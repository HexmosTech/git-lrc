package lrcrules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func TestLoadMissing(t *testing.T) {
	dir := t.TempDir()
	_, ok, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false when .lrc/ does not exist")
	}
}

func TestLoadNotADirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".lrc"), "not a directory")

	_, _, err := Load(dir)
	if err == nil {
		t.Fatalf("expected error when .lrc exists but is not a directory")
	}
}

func TestBuildRulesBundle(t *testing.T) {
	dir := t.TempDir()
	lrcDir := filepath.Join(dir, ".lrc")

	writeFile(t, filepath.Join(lrcDir, "rules", "README.md"), "should be excluded")
	writeFile(t, filepath.Join(lrcDir, "rules", "design.md"), "  Use hexagonal architecture.  ")
	writeFile(t, filepath.Join(lrcDir, "rules", "empty.md"), "   \n  ")
	writeFile(t, filepath.Join(lrcDir, "rules", "security.md"), "No secrets in logs.")
	writeFile(t, filepath.Join(lrcDir, "rules", "notes.txt"), "ignored, not markdown")

	text, charCount, issues := BuildRulesBundle(lrcDir)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %v", issues)
	}

	want := "## rules/design.md\n\nUse hexagonal architecture.\n\n## rules/security.md\n\nNo secrets in logs."
	if text != want {
		t.Fatalf("unexpected bundle text:\ngot:  %q\nwant: %q", text, want)
	}
	if charCount != len(want) {
		t.Fatalf("charCount = %d, want %d", charCount, len(want))
	}
}

func TestBuildRulesBundleInstructionsFirst(t *testing.T) {
	dir := t.TempDir()
	lrcDir := filepath.Join(dir, ".lrc")

	writeFile(t, filepath.Join(lrcDir, "rules", "README.md"), "should be excluded")
	writeFile(t, filepath.Join(lrcDir, "rules", "design.md"), "Use hexagonal architecture.")
	writeFile(t, filepath.Join(lrcDir, "rules", "INSTRUCTIONS.md"), "Read this first.")

	text, _, issues := BuildRulesBundle(lrcDir)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %v", issues)
	}

	want := "## rules/INSTRUCTIONS.md\n\nRead this first.\n\n## rules/design.md\n\nUse hexagonal architecture."
	if text != want {
		t.Fatalf("unexpected bundle text:\ngot:  %q\nwant: %q", text, want)
	}
}

func TestBuildRulesBundleMissingRulesDir(t *testing.T) {
	dir := t.TempDir()
	lrcDir := filepath.Join(dir, ".lrc")
	if err := os.MkdirAll(lrcDir, 0o755); err != nil {
		t.Fatalf("failed to create .lrc: %v", err)
	}

	text, charCount, issues := BuildRulesBundle(lrcDir)
	if text != "" || charCount != 0 || len(issues) != 0 {
		t.Fatalf("expected empty result when rules/ is missing, got text=%q charCount=%d issues=%v", text, charCount, issues)
	}
}

func TestBuildRulesBundleOverLimit(t *testing.T) {
	dir := t.TempDir()
	lrcDir := filepath.Join(dir, ".lrc")
	writeFile(t, filepath.Join(lrcDir, "rules", "design.md"), strings.Repeat("x", CharLimit+100))

	_, charCount, issues := BuildRulesBundle(lrcDir)
	if charCount <= CharLimit {
		t.Fatalf("expected charCount > %d, got %d", CharLimit, charCount)
	}

	found := false
	for _, issue := range issues {
		if issue.Level == "error" && issue.Path == "rules" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an error issue for exceeding the char limit, got %v", issues)
	}
}

func TestValidateStructure(t *testing.T) {
	dir := t.TempDir()
	lrcDir := filepath.Join(dir, ".lrc")
	if err := os.MkdirAll(lrcDir, 0o755); err != nil {
		t.Fatalf("failed to create .lrc: %v", err)
	}

	issues := ValidateStructure(lrcDir)

	wantPaths := map[string]bool{"rules": false, "ignore": false}
	for _, issue := range issues {
		if issue.Level != "warning" {
			t.Fatalf("expected warning-level issue, got %v", issue)
		}
		if _, ok := wantPaths[issue.Path]; ok {
			wantPaths[issue.Path] = true
		}
	}
	for path, seen := range wantPaths {
		if !seen {
			t.Fatalf("expected an issue for %q, got %v", path, issues)
		}
	}
}

func TestValidateStructureComplete(t *testing.T) {
	dir := t.TempDir()
	lrcDir := filepath.Join(dir, ".lrc")
	writeFile(t, filepath.Join(lrcDir, "rules", "INSTRUCTIONS.md"), "")
	writeFile(t, filepath.Join(lrcDir, "ignore"), "")

	issues := ValidateStructure(lrcDir)
	if len(issues) != 0 {
		t.Fatalf("expected no issues for a complete structure, got %v", issues)
	}
}

func TestCheckIgnoreSyntax(t *testing.T) {
	dir := t.TempDir()
	lrcDir := filepath.Join(dir, ".lrc")
	writeFile(t, filepath.Join(lrcDir, "ignore"), "# comment\n\nnode_modules/\n**/**\n*.log\n")

	issues := CheckIgnoreSyntax(lrcDir)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %v", issues)
	}
	if !strings.Contains(issues[0].Message, "**/**") {
		t.Fatalf("expected issue about '**/**', got %v", issues[0])
	}
}

func TestCheckIgnoreSyntaxMissingFile(t *testing.T) {
	dir := t.TempDir()
	lrcDir := filepath.Join(dir, ".lrc")
	if err := os.MkdirAll(lrcDir, 0o755); err != nil {
		t.Fatalf("failed to create .lrc: %v", err)
	}

	issues := CheckIgnoreSyntax(lrcDir)
	if issues != nil {
		t.Fatalf("expected no issues when ignore file is absent, got %v", issues)
	}
}

func TestInitCreatesScaffold(t *testing.T) {
	repoRoot := t.TempDir()

	created, err := Init(repoRoot)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	wantFiles := []string{
		".lrc/README.md",
		".lrc/ignore",
		".lrc/rules/INSTRUCTIONS.md",
		".lrc/rules/design.md",
		".lrc/rules/security.md",
		".lrc/rules/style.md",
		".lrc/policy/tools.toml",
	}
	if len(created) != len(wantFiles) {
		t.Fatalf("created = %v, want %v", created, wantFiles)
	}
	for _, f := range wantFiles {
		if _, err := os.Stat(filepath.Join(repoRoot, f)); err != nil {
			t.Fatalf("expected %s to be created: %v", f, err)
		}
	}
}

func TestInitIsIdempotent(t *testing.T) {
	repoRoot := t.TempDir()

	if _, err := Init(repoRoot); err != nil {
		t.Fatalf("first Init failed: %v", err)
	}

	customContent := "custom rules"
	securityPath := filepath.Join(repoRoot, ".lrc", "rules", "security.md")
	writeFile(t, securityPath, customContent)

	created, err := Init(repoRoot)
	if err != nil {
		t.Fatalf("second Init failed: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("expected no files created on second Init, got %v", created)
	}

	content, err := os.ReadFile(securityPath)
	if err != nil {
		t.Fatalf("failed to read security.md: %v", err)
	}
	if string(content) != customContent {
		t.Fatalf("Init overwrote existing file: got %q, want %q", string(content), customContent)
	}
}

func TestCollectZipExtras(t *testing.T) {
	repoRoot := t.TempDir()
	if _, err := Init(repoRoot); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	extras, warnings, err := CollectZipExtras(repoRoot)
	if err != nil {
		t.Fatalf("CollectZipExtras failed: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	wantKeys := []string{
		".lrc/README.md",
		".lrc/ignore",
		".lrc/rules/INSTRUCTIONS.md",
		".lrc/rules/design.md",
		".lrc/rules/security.md",
		".lrc/rules/style.md",
		".lrc/policy/tools.toml",
	}
	if len(extras) != len(wantKeys) {
		t.Fatalf("extras = %v, want keys %v", extras, wantKeys)
	}
	for _, k := range wantKeys {
		if _, ok := extras[k]; !ok {
			t.Fatalf("expected extras to contain %q, got %v", k, extras)
		}
	}
}

func TestCollectZipExtrasNoLRC(t *testing.T) {
	repoRoot := t.TempDir()

	extras, warnings, err := CollectZipExtras(repoRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(extras) != 0 {
		t.Fatalf("expected no extras when .lrc/ is absent, got %v", extras)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings when .lrc/ is absent, got %v", warnings)
	}
}

// TestCollectZipExtrasUnreadableFile verifies that a single unreadable file
// under .lrc/ is reported as a warning rather than aborting collection of
// the rest of the .lrc/ tree.
func TestCollectZipExtrasUnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: file permissions are not enforced")
	}

	repoRoot := t.TempDir()
	if _, err := Init(repoRoot); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	secretPath := filepath.Join(repoRoot, ".lrc", "rules", "security.md")
	if err := os.Chmod(secretPath, 0o000); err != nil {
		t.Fatalf("failed to chmod %s: %v", secretPath, err)
	}
	defer os.Chmod(secretPath, 0o644)

	extras, warnings, err := CollectZipExtras(repoRoot)
	if err != nil {
		t.Fatalf("CollectZipExtras failed: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected a warning about the unreadable file, got none")
	}

	if _, ok := extras[".lrc/rules/security.md"]; ok {
		t.Fatalf("expected unreadable file to be excluded from extras, got %v", extras)
	}
	if _, ok := extras[".lrc/rules/design.md"]; !ok {
		t.Fatalf("expected readable files to still be collected, got %v", extras)
	}
}
