package appui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/network"
	setuptpl "github.com/HexmosTech/git-lrc/setup"
	"github.com/urfave/cli/v2"
)

// RunOnboard handles the "lrc onboard" command.
func RunOnboard(c *cli.Context) error {
	// 1. Resolve API URL
	apiURL := strings.TrimSpace(c.String("api-url"))
	if apiURL == "" {
		apiURL = os.Getenv("LRC_API_URL")
	}
	if apiURL == "" {
		apiURL = setuptpl.CloudAPIURL
	}
	apiURL = strings.TrimRight(apiURL, "/")

	// 2. Resolve Onboarding API Key
	onboardingKey := strings.TrimSpace(c.String("onboarding-key"))
	if onboardingKey == "" {
		return errors.New("onboarding API key cannot be empty; pass via --onboarding-key or LRC_API_KEY env var")
	}

	// 3. Check existing config details
	details, err := setuptpl.ReadExistingConfigDetails()
	if err != nil {
		return fmt.Errorf("failed to read existing config details: %w", err)
	}

	// If config exists and we are interactive / no "--yes" flag, ask the user
	if details.Exists && !c.Bool("yes") && isInteractiveSetupStdin() {
		replace, err := promptSetupYesNo("  Existing config file detected. Replace it?", false)
		if err != nil {
			return err
		}
		if !replace {
			fmt.Println("  Onboarding cancelled. Existing configuration preserved.")
			return nil
		}
	}

	fmt.Printf("  Onboarding to LiveReview server at %s...\n", apiURL)

	// 4. Hit the onboard endpoint
	client := network.NewClient(30 * time.Second)
	resp, err := network.SetupOnboard(client, apiURL, onboardingKey)
	if err != nil {
		return fmt.Errorf("failed to contact LiveReview API: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(resp.Body, &errorResp); err == nil && errorResp.Error != "" {
			return fmt.Errorf("onboarding failed (status %d): %s", resp.StatusCode, errorResp.Error)
		}
		return fmt.Errorf("onboarding failed (status %d)", resp.StatusCode)
	}

	// 5. Parse response
	var onboardResp struct {
		APIKey       string      `json:"api_key"`
		OrgID        json.Number `json:"org_id"`
		OrgName      string      `json:"org_name"`
		JWT          string      `json:"jwt"`
		RefreshToken string      `json:"refresh_token"`
	}
	if err := json.Unmarshal(resp.Body, &onboardResp); err != nil {
		return fmt.Errorf("failed to parse onboarding response: %w", err)
	}

	// 6. Backup config if it exists
	if details.Exists {
		backupPath, err := setuptpl.BackupExistingConfig(nil)
		if err != nil {
			return fmt.Errorf("failed to backup existing config: %w", err)
		}
		if backupPath != "" {
			fmt.Printf("  📦 Backed up existing config to %s\n", backupPath)
		}
	}

	// 7. Write new configuration
	result := &setuptpl.SetupResult{
		PlainAPIKey:  onboardResp.APIKey,
		OrgID:        onboardResp.OrgID.String(),
		OrgName:      onboardResp.OrgName,
		AccessToken:  onboardResp.JWT,
		RefreshToken: onboardResp.RefreshToken,
	}

	if err := setuptpl.WriteConfigWithOptions(result, setuptpl.WriteConfigOptions{APIURL: apiURL}); err != nil {
		return fmt.Errorf("failed to write configuration: %w", err)
	}

	fmt.Println("  ✅ Onboarding completed successfully!")
	fmt.Printf("     Configuration saved to %s\n", details.Path)
	return nil
}
