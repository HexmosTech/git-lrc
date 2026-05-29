package setup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	cfgutil "github.com/HexmosTech/git-lrc/config"
	"github.com/HexmosTech/git-lrc/configpath"
	"github.com/HexmosTech/git-lrc/storage"
)

type ExistingConfigDetails struct {
	Path   string
	Exists bool
	APIURL string
}

type WriteConfigOptions struct {
	APIURL string
}

// ReadExistingConfigDetails inspects ~/.lrc.toml and returns current api_url when present.
func ReadExistingConfigDetails() (ExistingConfigDetails, error) {
	configPath, err := configpath.ResolveConfigPath()
	if err != nil {
		return ExistingConfigDetails{}, err
	}

	details := ExistingConfigDetails{Path: configPath}
	data, err := storage.ReadConfigFile(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return details, nil
		}
		return ExistingConfigDetails{}, err
	}

	if strings.TrimSpace(string(data)) == "" {
		return details, nil
	}

	details.Exists = true
	if apiURL, ok := cfgutil.ReadQuotedConfigValue(string(data), "api_url"); ok {
		details.APIURL = strings.TrimSpace(apiURL)
	}

	return details, nil
}

// WriteConfig writes setup results to ~/.lrc.toml.
func WriteConfig(result *SetupResult) error {
	return WriteConfigWithOptions(result, WriteConfigOptions{APIURL: CloudAPIURL})
}

// WriteConfigWithOptions writes setup results to ~/.lrc.toml with explicit write controls.
func WriteConfigWithOptions(result *SetupResult, opts WriteConfigOptions) error {
	configPath, err := configpath.ResolveConfigPath()
	if err != nil {
		return err
	}

	apiURL := strings.TrimSpace(opts.APIURL)
	if apiURL == "" {
		apiURL = CloudAPIURL
	}

	originalBytes, err := storage.ReadConfigFile(configPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to read existing config file: %w", err)
		}
		originalBytes = []byte{}
	}

	content := string(originalBytes)
	updates := map[string]string{
		"api_key":         result.PlainAPIKey,
		"api_url":         apiURL,
		"user_email":      result.Email,
		"user_first_name": result.FirstName,
		"user_last_name":  result.LastName,
		"avatar_url":      result.AvatarURL,
		"user_id":         result.UserID,
		"org_id":          result.OrgID,
		"org_name":        result.OrgName,
		"jwt":             result.AccessToken,
		"refresh_token":   result.RefreshToken,
	}

	for key, value := range updates {
		content = cfgutil.UpsertQuotedConfigValue(content, key, strings.TrimSpace(value))
	}

	if err := storage.WriteFileAtomically(configPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// WriteFileAtomically remains for compatibility and delegates writes to storage.
func WriteFileAtomically(path string, data []byte, mode os.FileMode) error {
	return storage.WriteFileAtomically(path, data, mode)
}
