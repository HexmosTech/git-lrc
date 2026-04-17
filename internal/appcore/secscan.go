package appcore

import (
	"fmt"
	"regexp"
	"strings"
)

// SecretPattern represents a regex rule to match known sensitive patterns
type SecretPattern struct {
	Name    string
	Pattern *regexp.Regexp
}

// Pre-compiled high-confidence secret patterns
var secretPatterns = []SecretPattern{
	{
		Name:    "AWS Access Key ID",
		Pattern: regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	},
	{
		Name:    "GitHub Personal Access Token",
		Pattern: regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`),
	},
	{
		Name:    "Slack Token",
		Pattern: regexp.MustCompile(`xox[baprs]-[0-9]{10,}-[0-9]{10,}-[a-zA-Z0-9]{24}`),
	},
	{
		Name:    "RSA / OpenSSH Private Key",
		Pattern: regexp.MustCompile(`-----BEGIN (?:RSA|OPENSSH) PRIVATE KEY-----`),
	},
	{
		Name:    "Generic High Entropy Secret",
		Pattern: regexp.MustCompile(`(?i)(?:sk|api_key|token|secret)[-_]?(?:key|token)?(?:[\s:=]+)(['"]?)([a-zA-Z0-9_\-\.]{20,})\1`),
	},
}

// ScanDiffForSecrets scans the provided git diff content for high-confidence secrets
// Returns an error detailing the found secrets, or nil if safe.
func ScanDiffForSecrets(diffContent []byte) error {
	if len(diffContent) == 0 {
		return nil
	}

	contentStr := string(diffContent)
	var foundSecrets []string

	for _, sp := range secretPatterns {
		if sp.Pattern.MatchString(contentStr) {
			// Find all matches for reporting
			matches := sp.Pattern.FindAllString(contentStr, -1)
			for _, match := range matches {
				redacted := redactSecretMatch(match)
				foundSecrets = append(foundSecrets, fmt.Sprintf("%s (%s)", sp.Name, redacted))
			}
		}
	}

	if len(foundSecrets) > 0 {
		return fmt.Errorf("local security check failed. Found %d potentially sensitive credential(s) in the staged diff:\n - %s\n\nAborting review. If you must commit this, please bypass using the `--skip` flag.",
			len(foundSecrets), strings.Join(foundSecrets, "\n - "))
	}

	return nil
}

// redactSecretMatch masks all but the first 4 and last 4 characters of the matched secret
func redactSecretMatch(secret string) string {
	if len(secret) <= 8 {
		return strings.Repeat("*", len(secret))
	}
	return secret[:4] + "...." + secret[len(secret)-4:]
}
