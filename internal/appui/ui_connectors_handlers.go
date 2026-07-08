package appui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/HexmosTech/git-lrc/internal/reviewopts"
	"github.com/HexmosTech/git-lrc/internal/staticserve"
	"github.com/HexmosTech/git-lrc/network"
	setuptpl "github.com/HexmosTech/git-lrc/setup"
	"github.com/HexmosTech/git-lrc/storage"
	uicfg "github.com/HexmosTech/git-lrc/ui"
)

type uiSessionStatusResponse = uicfg.SessionStatusResponse

func (s *connectorManagerServer) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	s.mu.Lock()
	jwt := strings.TrimSpace(s.cfg.JWT)
	orgID := strings.TrimSpace(s.cfg.OrgID)
	apiURL := strings.TrimSpace(s.cfg.APIURL)
	userEmail := strings.TrimSpace(s.cfg.UserEmail)
	userID := strings.TrimSpace(s.cfg.UserID)
	firstName := strings.TrimSpace(s.cfg.FirstName)
	lastName := strings.TrimSpace(s.cfg.LastName)
	avatarURL := strings.TrimSpace(s.cfg.AvatarURL)
	orgName := strings.TrimSpace(s.cfg.OrgName)
	configErr := strings.TrimSpace(s.cfg.ConfigErr)
	configMissing := s.cfg.ConfigMissing
	s.mu.Unlock()

	if apiURL == "" {
		apiURL = reviewopts.DefaultAPIURL
	}

	claims := decodeJWTClaims(jwt)
	if userEmail == "" {
		userEmail = strings.TrimSpace(claims["email"])
	}
	if firstName == "" {
		firstName = firstNonEmpty(claims["given_name"], claims["first_name"])
	}
	if lastName == "" {
		lastName = firstNonEmpty(claims["family_name"], claims["last_name"])
	}
	if avatarURL == "" {
		avatarURL = firstNonEmpty(claims["picture"], claims["avatar_url"])
	}
	displayName := strings.TrimSpace(claims["name"])
	if displayName == "" {
		displayName = strings.TrimSpace(strings.TrimSpace(firstName + " " + lastName))
	}
	if displayName == "" {
		displayName = firstNonEmpty(userEmail, userID)
	}

	status := uiSessionStatusResponse{
		Authenticated:  false,
		SessionExpired: false,
		MissingConfig:  configMissing,
		DisplayName:    displayName,
		FirstName:      firstName,
		LastName:       lastName,
		AvatarURL:      avatarURL,
		UserEmail:      userEmail,
		UserID:         userID,
		OrgID:          orgID,
		OrgName:        orgName,
		Organizations:  make([]uicfg.SessionOrganization, 0),
		APIURL:         apiURL,
	}

	orgsResult, orgsErr := s.fetchSessionOrganizations()
	if orgsErr == nil && orgsResult != nil {
		status.Organizations = orgsResult.Organizations
		if strings.TrimSpace(orgsResult.JWT) != "" {
			jwt = strings.TrimSpace(orgsResult.JWT)
		}
		if status.OrgName == "" && strings.TrimSpace(status.OrgID) != "" {
			for _, org := range orgsResult.Organizations {
				if status.OrgID == strconv.FormatInt(org.ID, 10) {
					status.OrgName = strings.TrimSpace(org.Name)
					break
				}
			}
		}
	}

	if jwt == "" {
		if configErr != "" {
			status.Message = configErr
		} else if orgsErr != nil {
			status.Message = orgsErr.Error()
		} else {
			status.Message = "not authenticated"
		}
		writeJSON(w, http.StatusOK, status)
		return
	}

	if strings.TrimSpace(orgID) == "" {
		status.Authenticated = true
		if len(status.Organizations) > 0 {
			status.Message = "organization context required. Select organization in header."
		} else if orgsErr != nil {
			status.Authenticated = false
			status.Message = orgsErr.Error()
		} else {
			status.Message = "organization context missing"
		}
		writeJSON(w, http.StatusOK, status)
		return
	}

	probeURL := network.ReviewNormalizedAPIURL(apiURL, "/api/v1/aiconnectors")
	probeStatus, _, err := s.forwardJSONRequest(http.MethodGet, probeURL, nil, jwt, orgID)
	if err != nil {
		status.Message = err.Error()
		writeJSON(w, http.StatusOK, status)
		return
	}

	if probeStatus == http.StatusUnauthorized {
		refreshed, refreshErr := s.refreshAccessToken(jwt)
		if refreshErr != nil || !refreshed {
			status.SessionExpired = true
			if refreshErr != nil {
				status.Message = refreshErr.Error()
			} else {
				status.Message = "session expired"
			}
			writeJSON(w, http.StatusOK, status)
			return
		}

		s.mu.Lock()
		jwt = strings.TrimSpace(s.cfg.JWT)
		s.mu.Unlock()
		probeStatus, _, err = s.forwardJSONRequest(http.MethodGet, probeURL, nil, jwt, orgID)
		if err != nil {
			status.Message = err.Error()
			writeJSON(w, http.StatusOK, status)
			return
		}
	}

	if probeStatus >= 200 && probeStatus < 300 {
		status.Authenticated = true
		status.SessionExpired = false
		status.Message = "authenticated"
		status.OrgID = strings.TrimSpace(orgID)
		writeJSON(w, http.StatusOK, status)
		return
	}

	status.Message = fmt.Sprintf("session check failed with status %d", probeStatus)
	writeJSON(w, http.StatusOK, status)
}

func (s *connectorManagerServer) handleReauthenticate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	s.mu.Lock()
	configPath := strings.TrimSpace(s.cfg.ConfigPath)
	apiURL := strings.TrimSpace(s.cfg.APIURL)
	s.mu.Unlock()
	if apiURL == "" {
		apiURL = reviewopts.DefaultAPIURL
	}

	slog := newSetupLog()
	var result *setupResult
	var err error
	if setuptpl.IsCloudAPIURL(apiURL) {
		result, err = runHexmosLoginFlow(slog, apiURL)
	} else {
		result, err = promptSelfHostedLoginFlow(apiURL, slog)
	}
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("reauthentication failed: %v", err))
		return
	}

	if err := persistReauthSessionToConfig(configPath, apiURL, result); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to persist session: %v", err))
		return
	}

	_ = storage.RemoveReauthLogFile(slog.logFile)

	s.mu.Lock()
	s.cfg.APIURL = apiURL
	s.cfg.JWT = strings.TrimSpace(result.AccessToken)
	s.cfg.RefreshJWT = strings.TrimSpace(result.RefreshToken)
	s.cfg.OrgID = strings.TrimSpace(result.OrgID)
	s.cfg.UserEmail = strings.TrimSpace(result.Email)
	s.cfg.UserID = strings.TrimSpace(result.UserID)
	s.cfg.FirstName = strings.TrimSpace(result.FirstName)
	s.cfg.LastName = strings.TrimSpace(result.LastName)
	s.cfg.AvatarURL = strings.TrimSpace(result.AvatarURL)
	s.cfg.OrgName = strings.TrimSpace(result.OrgName)
	s.cfg.ConfigErr = ""
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, uiSessionStatusResponse{
		Authenticated:  true,
		SessionExpired: false,
		MissingConfig:  false,
		DisplayName:    strings.TrimSpace(strings.TrimSpace(result.FirstName + " " + result.LastName)),
		FirstName:      strings.TrimSpace(result.FirstName),
		LastName:       strings.TrimSpace(result.LastName),
		AvatarURL:      strings.TrimSpace(result.AvatarURL),
		UserEmail:      strings.TrimSpace(result.Email),
		UserID:         strings.TrimSpace(result.UserID),
		OrgID:          strings.TrimSpace(result.OrgID),
		OrgName:        strings.TrimSpace(result.OrgName),
		APIURL:         apiURL,
		Message:        "reauthentication complete",
	})
}

func (s *connectorManagerServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	htmlBytes, err := staticserve.ReadFile("ui-connectors.html")
	if err != nil {
		http.Error(w, "failed to load UI", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := io.Copy(w, bytes.NewReader(htmlBytes)); err != nil {
		log.Printf("failed to write UI index response: %v", err)
	}
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

func (s *connectorManagerServer) handleHelperSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status, respBody, err := s.proxyJSONRequest(http.MethodGet, "/api/v1/aiconnectors/settings", nil)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeRawJSON(w, status, respBody)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		status, respBody, err := s.proxyJSONRequest(http.MethodPut, "/api/v1/aiconnectors/settings", body)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeRawJSON(w, status, respBody)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
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

func (s *connectorManagerServer) handleBedrockModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	status, respBody, err := s.proxyJSONRequest(http.MethodPost, "/api/v1/aiconnectors/bedrock/models", body)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeRawJSON(w, status, respBody)
}

func (s *connectorManagerServer) handleProviderModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// URL format: /api/ui/connectors/providers/{provider}/models
	path := strings.TrimPrefix(r.URL.Path, "/api/ui/connectors/providers/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "models" {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	provider := parts[0]

	apiPath := fmt.Sprintf("/api/v1/aiconnectors/providers/%s/models", provider)
	status, respBody, err := s.proxyJSONRequest(http.MethodGet, apiPath, nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeRawJSON(w, status, respBody)
}
