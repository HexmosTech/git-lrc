package appui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

func TestWriteFileAtomicallyReplacesExistingContent(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, ".lrc.toml")

	if err := os.WriteFile(targetPath, []byte("api_key = \"old\"\n"), 0600); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	newContent := []byte("api_key = \"new\"\norg_id = \"o1\"\n")
	if err := writeFileAtomically(targetPath, newContent, 0600); err != nil {
		t.Fatalf("write atomically: %v", err)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(got) != string(newContent) {
		t.Fatalf("unexpected content: %q", string(got))
	}
}

func TestBackupExistingConfigBacksUpNonEmptyConfig(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	configPath := filepath.Join(tmpHome, ".lrc.toml")
	configBody := "jwt = \"stale\"\norg_id = \"org-1\"\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	slog := newSetupLog()
	if err := backupExistingConfig(slog); err != nil {
		t.Fatalf("backup existing config: %v", err)
	}

	matches, err := filepath.Glob(configPath + ".bak.*")
	if err != nil {
		t.Fatalf("glob backup files: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one backup file, got %d", len(matches))
	}

	backupBody, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read backup file: %v", err)
	}
	if string(backupBody) != configBody {
		t.Fatalf("backup mismatch: got %q", string(backupBody))
	}
}

func TestBackupExistingConfigSkipsMissingConfig(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	slog := newSetupLog()
	if err := backupExistingConfig(slog); err != nil {
		t.Fatalf("backup existing config on first run: %v", err)
	}

	configPath := filepath.Join(tmpHome, ".lrc.toml")
	matches, err := filepath.Glob(configPath + ".bak.*")
	if err != nil {
		t.Fatalf("glob backup files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no backup file for missing config, got %d", len(matches))
	}
}

func TestWriteConfigIncludesSessionFields(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	result := &setupResult{
		PlainAPIKey:  "lr_key_123",
		Email:        "user@example.com",
		FirstName:    "Jane",
		LastName:     "Doe",
		AvatarURL:    "https://cdn.hexmos.com/u/jane.png",
		UserID:       "u-1",
		OrgID:        "o-1",
		OrgName:      "Acme Org",
		AccessToken:  "jwt-1",
		RefreshToken: "ref-1",
	}

	if err := writeConfig(result, cloudAPIURL); err != nil {
		t.Fatalf("write config: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpHome, ".lrc.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)

	for _, expected := range []string{
		`api_key = "lr_key_123"`,
		`api_url = "https://livereview.hexmos.com"`,
		`user_email = "user@example.com"`,
		`user_first_name = "Jane"`,
		`user_last_name = "Doe"`,
		`avatar_url = "https://cdn.hexmos.com/u/jane.png"`,
		`org_id = "o-1"`,
		`org_name = "Acme Org"`,
		`jwt = "jwt-1"`,
		`refresh_token = "ref-1"`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("config missing %s", expected)
		}
	}
}

func TestWriteConfigUsesSelectedAPIURL(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	result := &setupResult{
		PlainAPIKey:  "lr_key_123",
		Email:        "user@example.com",
		FirstName:    "Jane",
		LastName:     "Doe",
		AvatarURL:    "https://cdn.hexmos.com/u/jane.png",
		UserID:       "u-1",
		OrgID:        "o-1",
		OrgName:      "Acme Org",
		AccessToken:  "jwt-1",
		RefreshToken: "ref-1",
	}

	selectedAPIURL := "http://localhost:8888"
	if err := writeConfig(result, selectedAPIURL); err != nil {
		t.Fatalf("write config: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpHome, ".lrc.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `api_url = "http://localhost:8888"`) {
		t.Fatalf("config did not retain selected api_url: %s", content)
	}
}

func TestWriteConfigPreservesExistingUnrelatedFields(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	configPath := filepath.Join(tmpHome, ".lrc.toml")
	original := `custom_setting = "keep-me"

# BEGIN lrc managed ai_connectors
[[ai_connectors]]
id = 7
provider_name = "gemini"
connector_name = "Existing Gemini"
api_key = "existing-key"
selected_model = "gemini-2.5-flash"
display_order = 1
# END lrc managed ai_connectors
`
	if err := os.WriteFile(configPath, []byte(original), 0600); err != nil {
		t.Fatalf("write original config: %v", err)
	}

	result := &setupResult{
		PlainAPIKey:  "lr_key_123",
		Email:        "user@example.com",
		FirstName:    "Jane",
		LastName:     "Doe",
		AvatarURL:    "https://cdn.hexmos.com/u/jane.png",
		UserID:       "u-1",
		OrgID:        "o-1",
		OrgName:      "Acme Org",
		AccessToken:  "jwt-1",
		RefreshToken: "ref-1",
	}

	if err := writeConfig(result, cloudAPIURL); err != nil {
		t.Fatalf("write config: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)
	for _, expected := range []string{
		`custom_setting = "keep-me"`,
		`provider_name = "gemini"`,
		`api_key = "lr_key_123"`,
		`jwt = "jwt-1"`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("config missing %s in %s", expected, content)
		}
	}
}

func TestResolveSetupAPIURLNonInteractiveRequiresExplicitChoice(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	configPath := filepath.Join(tmpHome, ".lrc.toml")
	if err := os.WriteFile(configPath, []byte("api_url = \"http://localhost:8888\"\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx := newSetupTestContext(t, []string{"--yes"})
	slog := newSetupLog()
	_, err := resolveSetupAPIURL(ctx, slog, cloudAPIURL)
	if err == nil {
		t.Fatalf("expected error when no explicit api_url choice provided in non-interactive mode")
	}
	if !strings.Contains(err.Error(), "--keep-api-url") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveSetupAPIURLKeepFlagPreservesExistingValue(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	configPath := filepath.Join(tmpHome, ".lrc.toml")
	if err := os.WriteFile(configPath, []byte("api_url = \"http://localhost:8888\"\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx := newSetupTestContext(t, []string{"--yes", "--keep-api-url"})
	slog := newSetupLog()
	apiURL, err := resolveSetupAPIURL(ctx, slog, "https://custom-target.example.com")
	if err != nil {
		t.Fatalf("resolve setup api url: %v", err)
	}
	if apiURL != "http://localhost:8888" {
		t.Fatalf("expected existing api_url, got %q", apiURL)
	}
}

func TestResolveSetupAPIURLReplaceFlagUsesSetupTarget(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	configPath := filepath.Join(tmpHome, ".lrc.toml")
	if err := os.WriteFile(configPath, []byte("api_url = \"http://localhost:8888\"\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx := newSetupTestContext(t, []string{"--yes", "--replace-api-url"})
	slog := newSetupLog()
	apiURL, err := resolveSetupAPIURL(ctx, slog, "https://custom-target.example.com")
	if err != nil {
		t.Fatalf("resolve setup api url: %v", err)
	}
	if apiURL != "https://custom-target.example.com" {
		t.Fatalf("expected setup target api_url %q, got %q", "https://custom-target.example.com", apiURL)
	}
}

func TestResolveSetupAPIURLNoExistingConfigUsesTarget(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	ctx := newSetupTestContext(t, []string{"--yes", "--api-url", "http://localhost:8899"})
	slog := newSetupLog()
	apiURL, err := resolveSetupAPIURL(ctx, slog, resolveSetupTargetAPIURL(ctx.String("api-url")))
	if err != nil {
		t.Fatalf("resolve setup api url: %v", err)
	}
	if apiURL != "http://localhost:8899" {
		t.Fatalf("expected target api_url %q, got %q", "http://localhost:8899", apiURL)
	}
}

func TestResolveSetupTargetAPIURLTrimsTrailingSlash(t *testing.T) {
	got := resolveSetupTargetAPIURL("https://livereview.hexmos.com/")
	if got != "https://livereview.hexmos.com" {
		t.Fatalf("unexpected normalized api_url: %q", got)
	}
}

func newSetupTestContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	fs := flag.NewFlagSet("setup-test", flag.ContinueOnError)
	fs.Bool("yes", false, "")
	fs.Bool("keep-api-url", false, "")
	fs.Bool("replace-api-url", false, "")
	fs.String("api-url", "", "")
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	return cli.NewContext(app, fs, nil)
}
