package appui

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/HexmosTech/git-lrc/internal/reviewopts"
	"github.com/HexmosTech/git-lrc/internal/staticserve"
	"github.com/HexmosTech/git-lrc/network"
	uicfg "github.com/HexmosTech/git-lrc/ui"
	"github.com/urfave/cli/v2"
)

const defaultUIPort = 8090

const (
	aiConnectorsSectionBegin = uicfg.AIConnectorsSectionBegin
	aiConnectorsSectionEnd   = uicfg.AIConnectorsSectionEnd
)

type uiRuntimeConfig = uicfg.RuntimeConfig
type aiConnectorRemote = uicfg.ConnectorRemote

type connectorManagerServer struct {
	cfg    *uiRuntimeConfig
	client *network.Client
	mu     sync.Mutex
}

type authRefreshRequest = uicfg.AuthRefreshRequest
type authRefreshResponse = uicfg.AuthRefreshResponse

func RunUI(c *cli.Context) error {
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
		client: network.NewUIConnectorClient(30 * time.Second),
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", staticserve.GetStaticHandler()))
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/api/ui/session-status", srv.handleSessionStatus)
	mux.HandleFunc("/api/ui/auth/reauth", srv.handleReauthenticate)
	mux.HandleFunc("/api/ui/org-context", srv.handleSetOrgContext)
	mux.HandleFunc("/api/ui/connectors/reorder", srv.handleReorder)
	mux.HandleFunc("/api/ui/connectors/settings", srv.handleHelperSettings)
	mux.HandleFunc("/api/ui/connectors/validate-key", srv.handleValidateKey)
	mux.HandleFunc("/api/ui/connectors/ollama/models", srv.handleOllamaModels)
	mux.HandleFunc("/api/ui/connectors/bedrock/models", srv.handleBedrockModels)
	mux.HandleFunc("/api/ui/connectors/providers/", srv.handleProviderModels)
	mux.HandleFunc("/api/ui/usage-chip", srv.handleUsageChip)
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
	return uicfg.LoadRuntimeConfig(reviewopts.DefaultAPIURL)
}
