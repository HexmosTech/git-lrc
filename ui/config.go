package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/HexmosTech/git-lrc/configpath"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// LoadRuntimeConfig reads ~/.lrc.toml and returns UI runtime config values.
func LoadRuntimeConfig(defaultAPIURL string) (*RuntimeConfig, error) {
	configPath, err := configpath.ResolveConfigPath()
	if err != nil {
		return nil, err
	}

	cfg := &RuntimeConfig{
		APIURL:        defaultAPIURL,
		ConfigPath:    configPath,
		ConfigMissing: false,
	}

	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			cfg.ConfigErr = fmt.Sprintf("config file not found at %s", configPath)
			cfg.ConfigMissing = true
			return cfg, nil
		}
		cfg.ConfigErr = fmt.Sprintf("failed to read config file %s: %v", configPath, err)
		return cfg, nil
	}

	k := koanf.New(".")
	if err := k.Load(file.Provider(configPath), toml.Parser()); err != nil {
		cfg.ConfigErr = fmt.Sprintf("failed to load config file %s: %v", configPath, err)
		return cfg, nil
	}

	apiURL := strings.TrimSpace(k.String("api_url"))
	if apiURL == "" {
		apiURL = defaultAPIURL
	}

	cfg.APIURL = apiURL
	cfg.APIKey = strings.TrimSpace(k.String("api_key"))
	cfg.JWT = strings.TrimSpace(k.String("jwt"))
	cfg.RefreshJWT = strings.TrimSpace(k.String("refresh_token"))
	cfg.OrgID = strings.TrimSpace(k.String("org_id"))
	cfg.UserEmail = strings.TrimSpace(k.String("user_email"))
	cfg.UserID = strings.TrimSpace(k.String("user_id"))
	cfg.FirstName = strings.TrimSpace(k.String("user_first_name"))
	cfg.LastName = strings.TrimSpace(k.String("user_last_name"))
	cfg.AvatarURL = strings.TrimSpace(k.String("avatar_url"))
	cfg.OrgName = strings.TrimSpace(k.String("org_name"))

	return cfg, nil
}
