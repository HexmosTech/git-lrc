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
	commitMessageFile   = "livereview_commit_message"
	editorWrapperScript = "lrc_editor.sh"
	editorBackupFile    = ".lrc_editor_backup"
	pushRequestFile     = "livereview_push_request"
)

const (
	lrcMarkerBegin        = "# BEGIN lrc managed section - DO NOT EDIT"
	lrcMarkerEnd          = "# END lrc managed section"
	defaultGlobalHooksDir = ".git-hooks"
	hooksMetaFilename     = ".lrc-hooks-meta.json"
)

var managedHooks = []string{"pre-commit", "prepare-commit-msg", "commit-msg", "post-commit"}

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
			"%s\n\n## Synthetic Coverage\n\n- Generated %d synthetic comment(s) across %d file(s)\n- Includes deterministic Copy Issue scenarios: first-line (no prev), last-line (no next), interior-line (both prev/next)",
			strings.TrimSpace(result.Summary),
			totalComments,
			len(files),
		)
	}

	result.Files = files
	return result
}

func buildSyntheticCommentsByFile(files []reviewmodel.DiffReviewFileResult) map[string][]reviewmodel.DiffReviewComment {
	commentsByFile := make(map[string][]reviewmodel.DiffReviewComment)

	// Primary scenario: find one hunk that has enough lines to guarantee
	// first-line, last-line, and interior-line comments.
	for _, file := range files {
		for _, hunk := range file.Hunks {
			numbers := collectHunkAddedLineNumbers(hunk)
			if len(numbers) < 3 {
				continue
			}

			interiorIdx := 1
			if interiorIdx >= len(numbers)-1 {
				interiorIdx = len(numbers) / 2
			}
			if interiorIdx <= 0 {
				interiorIdx = 1
			}
			if interiorIdx >= len(numbers)-1 {
				interiorIdx = len(numbers) - 2
			}

			commentsByFile[file.FilePath] = []reviewmodel.DiffReviewComment{
				{
					Line:     numbers[0],
					Severity: "Warning",
					Category: "Context",
					Content:  "Hunk-start line: verify Copy Issue handles missing previous line context correctly.",
				},
				{
					Line:     numbers[len(numbers)-1],
					Severity: "Error",
					Category: "Context",
					Content:  "Hunk-end line: verify Copy Issue handles missing next line context correctly.",
				},
				{
					Line:     numbers[interiorIdx],
					Severity: "Info",
					Category: "Context",
					Content:  "Interior line: verify Copy Issue includes both previous and next lines in the code excerpt.",
				},
			}
			return commentsByFile
		}
	}

	// Fallback for very small diffs: emit whatever subset is possible.
	for _, file := range files {
		for _, hunk := range file.Hunks {
			numbers := collectHunkAddedLineNumbers(hunk)
			if len(numbers) == 0 {
				continue
			}

			comments := []reviewmodel.DiffReviewComment{
				{
					Line:     numbers[0],
					Severity: "Warning",
					Category: "Context",
					Content:  "Hunk-start line: verify Copy Issue handles missing previous line context correctly.",
				},
			}
			if len(numbers) > 1 {
				comments = append(comments, reviewmodel.DiffReviewComment{
					Line:     numbers[len(numbers)-1],
					Severity: "Error",
					Category: "Context",
					Content:  "Hunk-end line: verify Copy Issue handles missing next line context correctly.",
				})
			}
			if len(numbers) > 2 {
				comments = append(comments, reviewmodel.DiffReviewComment{
					Line:     numbers[1],
					Severity: "Info",
					Category: "Context",
					Content:  "Interior line: verify Copy Issue includes both previous and next lines in the code excerpt.",
				})
			}

			commentsByFile[file.FilePath] = comments
			return commentsByFile
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

func pollReviewFake(reviewID string, pollInterval, wait time.Duration, verbose bool, cancel <-chan struct{}, baseFiles []reviewmodel.DiffReviewFileResult) (*reviewmodel.DiffReviewResponse, error) {
	if pollInterval <= 0 {
		pollInterval = 1 * time.Second
	}

	start := time.Now()
	deadline := start.Add(wait)
	fmt.Printf("Waiting for fake review completion (poll every %s, delay %s)...\r\n", pollInterval, wait)
	syncFileSafely(os.Stdout)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		now := time.Now()
		if !now.Before(deadline) {
			statusLine := fmt.Sprintf("Status: completed | elapsed: %s", now.Sub(start).Truncate(time.Second))
			fmt.Printf("\r%-80s\r\n", statusLine)
			syncFileSafely(os.Stdout)
			if verbose {
				log.Printf("fake review %s completed", reviewID)
			}
			return buildFakeCompletedResultForFiles(baseFiles), nil
		}

		statusLine := fmt.Sprintf("Status: in_progress | elapsed: %s", now.Sub(start).Truncate(time.Second))
		fmt.Printf("\r%-80s", statusLine)
		syncFileSafely(os.Stdout)
		if verbose {
			log.Printf("fake review %s: %s", reviewID, statusLine)
		}

		select {
		case <-cancel:
			fmt.Printf("\r\n")
			syncFileSafely(os.Stdout)
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
