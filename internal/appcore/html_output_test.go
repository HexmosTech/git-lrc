package appcore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
)

// TestHTMLOutputConsistency tests that refactored HTML generation produces identical output
func TestHTMLOutputConsistency(t *testing.T) {
	// Create test data
	result := &reviewmodel.DiffReviewResponse{
		Status:  "completed",
		Summary: "# Test Summary\n\nThis is a **test** summary with:\n- Item 1\n- Item 2\n\n## Code Example\n\n```go\nfunc test() {\n    return true\n}\n```",
		Files: []reviewmodel.DiffReviewFileResult{
			{
				FilePath: "test/file.go",
				Hunks: []reviewmodel.DiffReviewHunk{
					{
						OldStartLine: 10,
						OldLineCount: 5,
						NewStartLine: 10,
						NewLineCount: 6,
						Content: `@@ -10,5 +10,6 @@
 func example() {
-    old line
+    new line
+    another new line
     context line
 }`,
					},
				},
				Comments: []reviewmodel.DiffReviewComment{
					{
						Line:     11,
						Content:  "This is a test comment with\nmultiple lines",
						Severity: "warning",
						Category: "style",
					},
					{
						Line:     12,
						Content:  "Another comment",
						Severity: "error",
						Category: "bug",
					},
				},
			},
			{
				FilePath: "test/another.go",
				Hunks: []reviewmodel.DiffReviewHunk{
					{
						OldStartLine: 1,
						OldLineCount: 3,
						NewStartLine: 1,
						NewLineCount: 4,
						Content: `@@ -1,3 +1,4 @@
 package test
+import "fmt"
 
 func main() {`,
					},
				},
				Comments: []reviewmodel.DiffReviewComment{
					{
						Line:     2,
						Content:  "Consider using a different import",
						Severity: "info",
						Category: "suggestion",
					},
				},
			},
		},
	}

	// Create temp directory for test output
	tmpDir := t.TempDir()

	// Generate HTML using the current implementation
	outputPath := filepath.Join(tmpDir, "output.html")
	err := saveHTMLOutput(outputPath, result, false, false, false, "", "", "", "")
	if err != nil {
		t.Fatalf("Failed to generate HTML: %v", err)
	}

	// Read the generated HTML
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read generated HTML: %v", err)
	}

	// Verify basic structure
	html := string(content)

	// Check for essential shell components.
	// The current frontend loads review data dynamically from /api/review,
	// so file names/comments are not embedded into static HTML output.
	essentialStrings := []string{
		"<!DOCTYPE html>",
		"LiveReview Results",
		"id=\"app\"",
		"Loading LiveReview",
		"/static/app.js",
		"/api/review",
		"marked.min.js",
		"preact.umd.js",
	}

	for _, str := range essentialStrings {
		if !containsString(html, str) {
			t.Errorf("Generated HTML missing expected string: %s", str)
		}
	}

	// Check that it's a valid HTML structure
	if len(content) == 0 {
		t.Error("Generated HTML is empty")
	}

	t.Logf("Generated HTML: %d bytes", len(content))
}

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) &&
		(haystack == needle || len(haystack) > len(needle) &&
			(haystack[:len(needle)] == needle ||
				haystack[len(haystack)-len(needle):] == needle ||
				containsSubstring(haystack, needle)))
}

func containsSubstring(haystack, needle string) bool {
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestHTMLTemplateWithEmptyData tests edge cases
func TestHTMLTemplateWithEmptyData(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name   string
		result *reviewmodel.DiffReviewResponse
	}{
		{
			name: "no files",
			result: &reviewmodel.DiffReviewResponse{
				Status:  "completed",
				Summary: "No changes",
				Files:   []reviewmodel.DiffReviewFileResult{},
			},
		},
		{
			name: "file with no comments",
			result: &reviewmodel.DiffReviewResponse{
				Status: "completed",
				Files: []reviewmodel.DiffReviewFileResult{
					{
						FilePath: "test.go",
						Hunks: []reviewmodel.DiffReviewHunk{
							{
								OldStartLine: 1,
								OldLineCount: 1,
								NewStartLine: 1,
								NewLineCount: 1,
								Content:      " unchanged",
							},
						},
						Comments: []reviewmodel.DiffReviewComment{},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputPath := filepath.Join(tmpDir, tt.name+".html")
			err := saveHTMLOutput(outputPath, tt.result, false, false, false, "", "", "", "")
			if err != nil {
				t.Errorf("Failed to generate HTML for %s: %v", tt.name, err)
			}

			// Verify file was created
			if _, err := os.Stat(outputPath); os.IsNotExist(err) {
				t.Errorf("HTML file was not created for %s", tt.name)
			}
		})
	}
}
