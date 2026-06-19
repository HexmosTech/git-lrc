package appcore

import (
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/HexmosTech/git-lrc/internal/naming"
	"github.com/HexmosTech/git-lrc/internal/reviewapi"
	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/internal/reviewopts"
	reviewpkg "github.com/HexmosTech/git-lrc/review"
	"github.com/urfave/cli/v2"
)

type fakeReviewEvent struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Time    string         `json:"time"`
	Level   string         `json:"level,omitempty"`
	BatchID string         `json:"batchId,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

type fakeReviewEventsResponse struct {
	Events []fakeReviewEvent `json:"events"`
}

const (
	commitMessageFile          = "livereview_commit_message"
	activeCommitMessageFileEnv = "LRC_ACTIVE_COMMIT_MSG_FILE"
	editorWrapperScript        = "lrc_editor.sh"
	editorBackupFile           = ".lrc_editor_backup"
	pushRequestFile            = "livereview_push_request"
)

const (
	lrcMarkerBegin        = "# BEGIN lrc managed section - DO NOT EDIT"
	lrcMarkerEnd          = "# END lrc managed section"
	defaultGlobalHooksDir = ".git-hooks"
	hooksMetaFilename     = ".lrc-hooks-meta.json"
)

var managedHooks = []string{"pre-commit", "prepare-commit-msg", "commit-msg", "post-commit"}

// pushHooks are installed alongside managedHooks (as inert dispatchers) whenever the git
// hook surface is installed, but are only active once push review is explicitly enabled
// via the independent "push" surface (see pushReviewEnabled/setPushReviewEnabled).
var pushHooks = []string{"pre-push"}

var (
	version    = "unknown"
	reviewMode = "prod"

	currentReviewState *ReviewState
	reviewStateMu      sync.RWMutex
)

func Configure(versionValue, reviewModeValue string) {
	if strings.TrimSpace(versionValue) != "" {
		version = versionValue
	}
	if strings.TrimSpace(reviewModeValue) != "" {
		reviewMode = reviewModeValue
	}
}

func isFakeReviewBuild() bool {
	return reviewpkg.IsFakeReviewBuild(reviewMode)
}

func fakeReviewWaitDuration() (time.Duration, error) {
	return reviewpkg.FakeReviewWaitDuration(os.Getenv("LRC_FAKE_REVIEW_WAIT"))
}

func buildFakeSubmitResponse() reviewmodel.DiffReviewCreateResponse {
	resp := reviewpkg.BuildFakeSubmitResponse(time.Now(), naming.GenerateFriendlyName())
	return reviewmodel.DiffReviewCreateResponse{
		ReviewID:     resp.ReviewID,
		Status:       resp.Status,
		FriendlyName: resp.FriendlyName,
	}
}

func buildFakeCompletedResult() *reviewmodel.DiffReviewResponse {
	resp := reviewpkg.BuildFakeCompletedResult()
	return &reviewmodel.DiffReviewResponse{
		Status:  resp.Status,
		Summary: resp.Summary,
		Files:   []reviewmodel.DiffReviewFileResult{},
	}
}

func buildFakeCompletedResultForFiles(baseFiles []reviewmodel.DiffReviewFileResult) *reviewmodel.DiffReviewResponse {
	result := buildFakeCompletedResult()
	if len(baseFiles) == 0 {
		return result
	}

	files := make([]reviewmodel.DiffReviewFileResult, len(baseFiles))
	for i := range baseFiles {
		files[i] = baseFiles[i]
		files[i].Comments = nil
	}

	commentsByFile := buildSyntheticCommentsByFile(files)
	totalComments := 0
	for i := range files {
		if comments, ok := commentsByFile[files[i].FilePath]; ok {
			files[i].Comments = comments
			totalComments += len(comments)
		}
	}

	if totalComments > 0 {
		result.Summary = fmt.Sprintf(
			"%s\n\n## Synthetic Coverage\n\n- Generated %d synthetic comment(s) across %d file(s)\n- Covers Critical, Warning, and Info severities across targeted files",
			strings.TrimSpace(result.Summary),
			totalComments,
			len(files),
		)
	}

	result.Files = files
	return result
}

type syntheticCommentSpec struct {
	linePickIndex int // -1 = last available line, ≥0 = Nth available line
	severity      string
	confidence    string
	typeName      string
	category      string
	subcategory   string
	content       string
}

// perFileCommentSpecs maps each fake review file's base name to the comments to
// generate for it. Line numbers are resolved at runtime from actual diff hunks.
var perFileCommentSpecs = map[string][]syntheticCommentSpec{
	"README.md": {
		{
			linePickIndex: 0,
			severity:      "Critical",
			confidence:    "High",
			typeName:      "Best Practice",
			category:      "Documentation",
			subcategory:   "Missing Prerequisites",
			content:       "README is missing required Go version and platform prerequisites — document these before shipping.",
		},
	},
	"edge_cases.txt": {
		{
			linePickIndex: 0,
			severity:      "Critical",
			confidence:    "High",
			typeName:      "Bug",
			category:      "Logic",
			subcategory:   "Parser Mismatch",
			content:       "`alpha-updated` is inconsistent with downstream parser expectations; update the canonical test fixture.",
		},
		{
			linePickIndex: -1,
			severity:      "Warning",
			confidence:    "Medium",
			typeName:      "Bug",
			category:      "Logic",
			subcategory:   "Integration Drift",
			content:       "`delta-updated` does not match the expected integration test output — realign the test data.",
		},
	},
	"fake_large_config.toml": {
		{
			linePickIndex: 0,
			severity:      "Warning",
			confidence:    "Medium",
			typeName:      "Risk",
			category:      "Configuration",
			subcategory:   "Telemetry Leakage",
			content:       "`enable_telemetry = true` in a generated config risks leaking test data to analytics endpoints — disable for local runs.",
		},
	},
	"only_one_line.txt": {
		{
			linePickIndex: 0,
			severity:      "Info",
			confidence:    "Low",
			typeName:      "Technical Debt",
			category:      "Style",
			subcategory:   "Snapshot Stability",
			content:       "Single-line file — confirm the seed suffix is stable enough for snapshot testing.",
		},
	},
	"ui_connectors_handlers.go": {
		{
			linePickIndex: 0,
			severity:      "Info",
			confidence:    "Low",
			typeName:      "Optimization",
			category:      "Style",
			subcategory:   "String Processing",
			content:       "`normalizeConnectorName` chains three sequential string operations; consider combining into a single `strings.Map` pass for clarity.",
		},
	},
}

func buildSyntheticCommentsByFile(files []reviewmodel.DiffReviewFileResult) map[string][]reviewmodel.DiffReviewComment {
	commentsByFile := make(map[string][]reviewmodel.DiffReviewComment)

	for _, file := range files {
		base := file.FilePath
		if idx := strings.LastIndex(base, "/"); idx >= 0 {
			base = base[idx+1:]
		}
		specs, ok := perFileCommentSpecs[base]
		if !ok {
			continue
		}

		var allLines []int
		for _, hunk := range file.Hunks {
			allLines = append(allLines, collectHunkAddedLineNumbers(hunk)...)
		}
		if len(allLines) == 0 {
			continue
		}

		var comments []reviewmodel.DiffReviewComment
		for _, spec := range specs {
			idx := spec.linePickIndex
			if idx < 0 {
				idx = len(allLines) + idx
			}
			if idx < 0 {
				idx = 0
			}
			if idx >= len(allLines) {
				idx = len(allLines) - 1
			}
			comments = append(comments, reviewmodel.DiffReviewComment{
				Line:        allLines[idx],
				Severity:    spec.severity,
				Confidence:  spec.confidence,
				Type:        spec.typeName,
				Category:    spec.category,
				Subcategory: spec.subcategory,
				Content:     spec.content,
			})
		}
		if len(comments) > 0 {
			commentsByFile[file.FilePath] = comments
		}
	}

	return commentsByFile
}

func collectHunkAddedLineNumbers(hunk reviewmodel.DiffReviewHunk) []int {
	numbers := make([]int, 0, 16)
	newLine := hunk.NewStartLine

	for _, line := range strings.Split(hunk.Content, "\n") {
		if line == "" || strings.HasPrefix(line, "@@") {
			continue
		}

		switch {
		case strings.HasPrefix(line, "-"):
			// Removed line does not advance new line numbers.
		case strings.HasPrefix(line, "+"):
			numbers = append(numbers, newLine)
			newLine++
		default:
			newLine++
		}
	}

	return numbers
}

func collectHunkNewLineNumbers(hunk reviewmodel.DiffReviewHunk) []int {
	numbers := make([]int, 0, 16)
	oldLine := hunk.OldStartLine
	newLine := hunk.NewStartLine

	for _, line := range strings.Split(hunk.Content, "\n") {
		if line == "" || strings.HasPrefix(line, "@@") {
			continue
		}

		switch {
		case strings.HasPrefix(line, "-"):
			oldLine++
		case strings.HasPrefix(line, "+"):
			numbers = append(numbers, newLine)
			newLine++
		default:
			numbers = append(numbers, newLine)
			oldLine++
			newLine++
		}
	}

	return numbers
}

func buildFakeEventsResponse(snapshot ReviewStateSnapshot) fakeReviewEventsResponse {
	baseTime := snapshot.StartedAt
	if baseTime.IsZero() {
		baseTime = time.Now().Add(-3 * time.Second)
	}
	batchID := "fake-batch-1"

	events := []fakeReviewEvent{
		{
			ID:    "fake-log-1",
			Type:  "log",
			Time:  baseTime.Format(time.RFC3339),
			Level: "info",
			Data: map[string]any{
				"message": "Fake review mode is active. Generating synthetic logs and issues for UI testing.",
			},
		},
		{
			ID:      "fake-batch-1",
			Type:    "batch",
			Time:    baseTime.Add(1 * time.Second).Format(time.RFC3339),
			Level:   "info",
			BatchID: batchID,
			Data: map[string]any{
				"status":    "processing",
				"fileCount": snapshot.TotalFiles,
			},
		},
		{
			ID:    "fake-status-1",
			Type:  "status",
			Time:  baseTime.Add(2 * time.Second).Format(time.RFC3339),
			Level: "info",
			Data: map[string]any{
				"status": snapshot.Status,
			},
		},
	}

	if snapshot.Status == "completed" {
		events = append(events,
			fakeReviewEvent{
				ID:      "fake-batch-2",
				Type:    "batch",
				Time:    baseTime.Add(3 * time.Second).Format(time.RFC3339),
				Level:   "info",
				BatchID: batchID,
				Data: map[string]any{
					"status":       "completed",
					"commentCount": snapshot.TotalComments,
				},
			},
			fakeReviewEvent{
				ID:    "fake-artifact-1",
				Type:  "artifact",
				Time:  baseTime.Add(4 * time.Second).Format(time.RFC3339),
				Level: "info",
				Data: map[string]any{
					"kind": "inline-comments",
					"url":  "/api/review",
				},
			},
			fakeReviewEvent{
				ID:    "fake-completion-1",
				Type:  "completion",
				Time:  baseTime.Add(5 * time.Second).Format(time.RFC3339),
				Level: "info",
				Data: map[string]any{
					"commentCount":  snapshot.TotalComments,
					"resultSummary": "Fake review completed with synthetic comments and events for UI testing.",
				},
			},
		)
	}

	return fakeReviewEventsResponse{Events: events}
}

func pollReviewFake(reviewID string, pollInterval, wait time.Duration, verbose bool, cancel <-chan struct{}, baseFiles []reviewmodel.DiffReviewFileResult, statusSink func(string)) (*reviewmodel.DiffReviewResponse, error) {
	if pollInterval <= 0 {
		pollInterval = 1 * time.Second
	}

	start := time.Now()
	deadline := start.Add(wait)
	if statusSink != nil {
		statusSink(fmt.Sprintf("waiting | poll=%s delay=%s", pollInterval, wait))
	} else {
		fmt.Printf("Waiting for fake review completion (poll every %s, delay %s)...\r\n", pollInterval, wait)
		syncFileSafely(os.Stdout)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		now := time.Now()
		if !now.Before(deadline) {
			statusLine := fmt.Sprintf("Status: completed | elapsed: %s", now.Sub(start).Truncate(time.Second))
			if statusSink != nil {
				statusSink(statusLine)
			} else {
				fmt.Printf("\r%-80s\r\n", statusLine)
				syncFileSafely(os.Stdout)
			}
			if verbose {
				log.Printf("fake review %s completed", reviewID)
			}
			return buildFakeCompletedResultForFiles(baseFiles), nil
		}

		statusLine := fmt.Sprintf("Status: in_progress | elapsed: %s", now.Sub(start).Truncate(time.Second))
		if statusSink != nil {
			statusSink(statusLine)
		} else {
			fmt.Printf("\r%-80s", statusLine)
			syncFileSafely(os.Stdout)
		}
		if verbose {
			log.Printf("fake review %s: %s", reviewID, statusLine)
		}

		select {
		case <-cancel:
			if statusSink == nil {
				fmt.Printf("\r\n")
				syncFileSafely(os.Stdout)
			}
			return nil, reviewapi.ErrPollCancelled
		case <-ticker.C:
		}
	}
}

func highlightURL(url string) string {
	return "\033[36m" + url + "\033[0m"
}

func buildReviewURL(apiURL, reviewID string) string {
	base := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(apiURL, "/"), "/api"), "/api/v1")
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s/#/reviews/%s", base, reviewID)
}

func pickServePort(preferredPort, maxTries int) (net.Listener, int, error) {
	for i := 0; i < maxTries; i++ {
		candidate := preferredPort + i

		if runtime.GOOS == "windows" {
			lnLocal, errLocal := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", candidate))
			lnAll, errAll := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", candidate))

			if errLocal != nil || errAll != nil {
				if lnLocal != nil {
					lnLocal.Close()
				}
				if lnAll != nil {
					lnAll.Close()
				}
				continue
			}

			lnAll.Close()
			return lnLocal, candidate, nil
		}

		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", candidate))
		if err == nil {
			return ln, candidate, nil
		}
	}

	return nil, 0, fmt.Errorf("no available port found starting from %d", preferredPort)
}

func RunReviewWithOptions(opts reviewopts.Options) error {
	return runReviewWithOptions(opts)
}

func RunUninstall(c *cli.Context) error {
	return runUninstall(c)
}

func RunHooksInstall(c *cli.Context) error {
	return runHooksInstall(c)
}

func RunHooksUninstall(c *cli.Context) error {
	return runHooksUninstall(c)
}

func RunHooksEnable(c *cli.Context) error {
	return runHooksEnable(c)
}

func RunHooksDisable(c *cli.Context) error {
	return runHooksDisable(c)
}

func RunHooksStatus(c *cli.Context) error {
	return runHooksStatus(c)
}

func RunAttestationTrailer(c *cli.Context) error {
	return runAttestationTrailer(c)
}

func RunInternalClaudePreToolUse(c *cli.Context) error {
	return runInternalClaudePreToolUse(c)
}

func RunInternalClaudeRunCommit(c *cli.Context) error {
	return runInternalClaudeRunCommit(c)
}

func RunInternalClaudeSetupStart(c *cli.Context) error {
	return runInternalClaudeSetupStart(c)
}

func RunInternalClaudeSetupWorker(c *cli.Context) error {
	return runInternalClaudeSetupWorker(c)
}

func RunInternalClaudeSetupSubmitKey(c *cli.Context) error {
	return runInternalClaudeSetupSubmitKey(c)
}

func RunInternalClaudeSetupStatus(c *cli.Context) error {
	return runInternalClaudeSetupStatus(c)
}
