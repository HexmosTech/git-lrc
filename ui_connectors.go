package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/urfave/cli/v2"
)

const defaultUIPort = 8090

const (
	aiConnectorsSectionBegin = "# BEGIN lrc managed ai_connectors"
	aiConnectorsSectionEnd   = "# END lrc managed ai_connectors"
)

type uiRuntimeConfig struct {
	APIURL     string
	JWT        string
	RefreshJWT string
	OrgID      string
	ConfigPath string
}

type aiConnectorRemote struct {
	ID            int64  `json:"id"`
	ProviderName  string `json:"provider_name"`
	ConnectorName string `json:"connector_name"`
	APIKey        string `json:"api_key"`
	BaseURL       string `json:"base_url"`
	SelectedModel string `json:"selected_model"`
	DisplayOrder  int    `json:"display_order"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type connectorManagerServer struct {
	cfg    *uiRuntimeConfig
	client *http.Client
	mu     sync.Mutex
}

type authRefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type authRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func runUI(c *cli.Context) error {
	cfg, err := loadUIRuntimeConfig()
	if err != nil {
		return err
	}

	ln, port, err := pickServePort(defaultUIPort, 20)
	if err != nil {
		return fmt.Errorf("failed to reserve UI port: %w", err)
	}

	srv := &connectorManagerServer{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", getStaticHandler()))
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/api/ui/connectors/reorder", srv.handleReorder)
	mux.HandleFunc("/api/ui/connectors/validate-key", srv.handleValidateKey)
	mux.HandleFunc("/api/ui/connectors/ollama/models", srv.handleOllamaModels)
	mux.HandleFunc("/api/ui/connectors/", srv.handleConnectorByID)
	mux.HandleFunc("/api/ui/connectors", srv.handleConnectors)

	httpServer := &http.Server{Handler: mux}
	go func() {
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("ui server error: %v", err)
		}
	}()

	url := fmt.Sprintf("http://localhost:%d", port)
	fmt.Printf("\n🌐 git-lrc Manager UI available at: %s\n\n", highlightURL(url))
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = openURL(url)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpServer.Shutdown(ctx)
}

func loadUIRuntimeConfig() (*uiRuntimeConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".lrc.toml")
	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found at %s. Run `lrc setup` first", configPath)
		}
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	k := koanf.New(".")
	if err := k.Load(file.Provider(configPath), toml.Parser()); err != nil {
		return nil, fmt.Errorf("failed to load config file %s: %w", configPath, err)
	}

	apiURL := strings.TrimSpace(k.String("api_url"))
	if apiURL == "" {
		apiURL = defaultAPIURL
	}

	jwt := strings.TrimSpace(k.String("jwt"))
	refreshJWT := strings.TrimSpace(k.String("refresh_token"))
	orgID := strings.TrimSpace(k.String("org_id"))
	if jwt == "" || orgID == "" {
		return nil, fmt.Errorf("missing jwt/org_id in %s. Run `lrc setup` to authenticate", configPath)
	}

	return &uiRuntimeConfig{
		APIURL:     apiURL,
		JWT:        jwt,
		RefreshJWT: refreshJWT,
		OrgID:      orgID,
		ConfigPath: configPath,
	}, nil
}

func (s *connectorManagerServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	htmlBytes, err := staticFiles.ReadFile("static/ui-connectors.html")
	if err != nil {
		http.Error(w, "failed to load UI", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(htmlBytes)
}

func (s *connectorManagerServer) handleConnectors(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status, body, err := s.proxyJSONRequest(http.MethodGet, "/api/v1/aiconnectors", nil)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}

		if status >= 200 && status < 300 {
			var connectors []aiConnectorRemote
			if err := json.Unmarshal(body, &connectors); err != nil {
				log.Printf("failed to decode connectors response for config persistence: %v", err)
			} else {
				if err := persistConnectorsToConfig(s.cfg.ConfigPath, connectors); err != nil {
					log.Printf("failed to persist connectors to config: %v", err)
				}
			}
		}

		writeRawJSON(w, status, body)
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		status, respBody, err := s.proxyJSONRequest(http.MethodPost, "/api/v1/aiconnectors", body)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeRawJSON(w, status, respBody)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *connectorManagerServer) handleConnectorByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/ui/connectors/")
	if id == "" || strings.Contains(id, "/") {
		writeJSONError(w, http.StatusNotFound, "connector not found")
		return
	}

	apiPath := "/api/v1/aiconnectors/" + id

	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		status, respBody, err := s.proxyJSONRequest(http.MethodPut, apiPath, body)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeRawJSON(w, status, respBody)
	case http.MethodDelete:
		status, respBody, err := s.proxyJSONRequest(http.MethodDelete, apiPath, nil)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeRawJSON(w, status, respBody)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *connectorManagerServer) handleReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	status, respBody, err := s.proxyJSONRequest(http.MethodPut, "/api/v1/aiconnectors/reorder", body)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeRawJSON(w, status, respBody)
}

func (s *connectorManagerServer) handleValidateKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	status, respBody, err := s.proxyJSONRequest(http.MethodPost, "/api/v1/aiconnectors/validate-key", body)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeRawJSON(w, status, respBody)
}

func (s *connectorManagerServer) handleOllamaModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	status, respBody, err := s.proxyJSONRequest(http.MethodPost, "/api/v1/aiconnectors/ollama/models", body)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeRawJSON(w, status, respBody)
}

func (s *connectorManagerServer) proxyJSONRequest(method, apiPath string, payload []byte) (int, []byte, error) {
	url := buildLiveReviewURL(s.cfg.APIURL, apiPath)

	s.mu.Lock()
	jwt := s.cfg.JWT
	orgID := s.cfg.OrgID
	s.mu.Unlock()

	status, respBody, err := s.forwardJSONRequest(method, url, payload, jwt, orgID)
	if err != nil {
		return status, nil, err
	}

	if status == http.StatusUnauthorized {
		refreshed, refreshErr := s.refreshAccessToken(jwt)
		if refreshErr != nil {
			log.Printf("failed to refresh lrc ui token: %v", refreshErr)
			return status, respBody, nil
		}
		if refreshed {
			s.mu.Lock()
			newJWT := s.cfg.JWT
			s.mu.Unlock()

			status, retryBody, retryErr := s.forwardJSONRequest(method, url, payload, newJWT, orgID)
			if retryErr != nil {
				return status, nil, retryErr
			}
			return status, retryBody, nil
		}
	}

	return status, respBody, nil
}

func (s *connectorManagerServer) forwardJSONRequest(method, url string, payload []byte, jwt string, orgID string) (int, []byte, error) {

	var bodyReader io.Reader
	if payload != nil {
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return http.StatusInternalServerError, nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("X-Org-Context", orgID)

	resp, err := s.client.Do(req)
	if err != nil {
		return http.StatusBadGateway, nil, fmt.Errorf("failed to call LiveReview API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return http.StatusBadGateway, nil, fmt.Errorf("failed to read LiveReview API response: %w", err)
	}

	return resp.StatusCode, respBody, nil
}

func (s *connectorManagerServer) refreshAccessToken(failedJWT string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(s.cfg.JWT) != strings.TrimSpace(failedJWT) {
		return true, nil
	}

	if strings.TrimSpace(s.cfg.RefreshJWT) == "" {
		return false, fmt.Errorf("refresh_token missing in %s", s.cfg.ConfigPath)
	}

	refreshURL := buildLiveReviewURL(s.cfg.APIURL, "/api/v1/auth/refresh")
	reqBody, err := json.Marshal(authRefreshRequest{RefreshToken: s.cfg.RefreshJWT})
	if err != nil {
		return false, fmt.Errorf("failed to marshal refresh request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, refreshURL, bytes.NewReader(reqBody))
	if err != nil {
		return false, fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read refresh response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("refresh failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var refreshResp authRefreshResponse
	if err := json.Unmarshal(body, &refreshResp); err != nil {
		return false, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	if strings.TrimSpace(refreshResp.AccessToken) == "" {
		return false, fmt.Errorf("refresh response missing access token")
	}

	s.cfg.JWT = strings.TrimSpace(refreshResp.AccessToken)
	if strings.TrimSpace(refreshResp.RefreshToken) != "" {
		s.cfg.RefreshJWT = strings.TrimSpace(refreshResp.RefreshToken)
	}

	if err := persistAuthTokensToConfig(s.cfg.ConfigPath, s.cfg.JWT, s.cfg.RefreshJWT); err != nil {
		log.Printf("warning: refreshed token obtained but failed to update %s: %v", s.cfg.ConfigPath, err)
	}

	return true, nil
}

func buildLiveReviewURL(baseURL, apiPath string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	base = strings.TrimSuffix(base, "/api/v1")
	base = strings.TrimSuffix(base, "/api")
	return base + apiPath
}

func writeRawJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeRawJSON(w, status, []byte(fmt.Sprintf(`{"error":%q}`, message)))
}

func persistConnectorsToConfig(configPath string, connectors []aiConnectorRemote) error {
	originalBytes, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read config for connector snapshot: %w", err)
		}
		originalBytes = []byte{}
	}

	originalContent := string(originalBytes)
	cleanedContent := stripManagedAIConnectorsSection(originalContent)
	managedSection := renderManagedAIConnectorsSection(connectors)

	trimmed := strings.TrimRight(cleanedContent, "\n\r\t ")
	var updatedContent string
	if trimmed == "" {
		updatedContent = managedSection + "\n"
	} else {
		updatedContent = trimmed + "\n\n" + managedSection + "\n"
	}

	tmpPath := configPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(updatedContent), 0600); err != nil {
		return fmt.Errorf("failed to write temporary config file: %w", err)
	}

	if err := os.Rename(tmpPath, configPath); err != nil {
		return fmt.Errorf("failed to replace config file: %w", err)
	}

	return nil
}

func persistAuthTokensToConfig(configPath string, jwt string, refreshToken string) error {
	originalBytes, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config for token update: %w", err)
	}

	content := string(originalBytes)
	updated := upsertQuotedConfigValue(content, "jwt", jwt)
	if strings.TrimSpace(refreshToken) != "" {
		updated = upsertQuotedConfigValue(updated, "refresh_token", refreshToken)
	}

	tmpPath := configPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(updated), 0600); err != nil {
		return fmt.Errorf("failed to write temporary config file: %w", err)
	}

	if err := os.Rename(tmpPath, configPath); err != nil {
		return fmt.Errorf("failed to replace config file: %w", err)
	}

	return nil
}

func upsertQuotedConfigValue(content string, key string, value string) string {
	lines := strings.Split(content, "\n")
	prefix := key + " = "
	replacement := prefix + strconv.Quote(value)
	replaced := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			lines[i] = replacement
			replaced = true
			break
		}
	}

	if !replaced {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, replacement)
	}

	return strings.Join(lines, "\n")
}

func stripManagedAIConnectorsSection(content string) string {
	start := strings.Index(content, aiConnectorsSectionBegin)
	if start == -1 {
		return content
	}

	endRelative := strings.Index(content[start:], aiConnectorsSectionEnd)
	if endRelative == -1 {
		return content[:start]
	}

	end := start + endRelative + len(aiConnectorsSectionEnd)
	if end < len(content) {
		if content[end] == '\r' {
			end++
		}
		if end < len(content) && content[end] == '\n' {
			end++
		}
	}

	return content[:start] + content[end:]
}

func renderManagedAIConnectorsSection(connectors []aiConnectorRemote) string {
	var builder strings.Builder
	builder.WriteString(aiConnectorsSectionBegin)
	builder.WriteString("\n")
	builder.WriteString("# Generated by lrc ui. This section is auto-managed and will be replaced.\n")

	for _, connector := range connectors {
		builder.WriteString("\n[[ai_connectors]]\n")
		builder.WriteString("id = ")
		builder.WriteString(strconv.FormatInt(connector.ID, 10))
		builder.WriteString("\n")
		builder.WriteString("provider_name = ")
		builder.WriteString(strconv.Quote(connector.ProviderName))
		builder.WriteString("\n")
		builder.WriteString("connector_name = ")
		builder.WriteString(strconv.Quote(connector.ConnectorName))
		builder.WriteString("\n")
		builder.WriteString("api_key = ")
		builder.WriteString(strconv.Quote(connector.APIKey))
		builder.WriteString("\n")
		if connector.BaseURL != "" {
			builder.WriteString("base_url = ")
			builder.WriteString(strconv.Quote(connector.BaseURL))
			builder.WriteString("\n")
		}
		if connector.SelectedModel != "" {
			builder.WriteString("selected_model = ")
			builder.WriteString(strconv.Quote(connector.SelectedModel))
			builder.WriteString("\n")
		}
		builder.WriteString("display_order = ")
		builder.WriteString(strconv.Itoa(connector.DisplayOrder))
		builder.WriteString("\n")
		if connector.CreatedAt != "" {
			builder.WriteString("created_at = ")
			builder.WriteString(strconv.Quote(connector.CreatedAt))
			builder.WriteString("\n")
		}
		if connector.UpdatedAt != "" {
			builder.WriteString("updated_at = ")
			builder.WriteString(strconv.Quote(connector.UpdatedAt))
			builder.WriteString("\n")
		}
	}

	builder.WriteString("\n")
	builder.WriteString(aiConnectorsSectionEnd)
	return builder.String()
}
