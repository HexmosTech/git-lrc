package reviewapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/interactive"
	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/network"
)

func RunGitCommand(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git command failed: %s\nstderr: %s", err, string(exitErr.Stderr))
		}
		return nil, err
	}
	return output, nil
}

func CurrentTreeHash() (string, error) {
	out, err := RunGitCommand("write-tree")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func resolveGitPath(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to resolve git path via %v: %w", args, err)
	}

	gitPath := strings.TrimSpace(string(out))
	if gitPath == "" {
		return "", fmt.Errorf("git path is empty for %v", args)
	}

	if filepath.IsAbs(gitPath) {
		return gitPath, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to resolve working directory: %w", err)
	}

	return filepath.Join(cwd, gitPath), nil
}

// ResolveGitDir returns the absolute path to the repository's git dir.
func ResolveGitDir() (string, error) {
	return resolveGitPath("rev-parse", "--git-dir")
}

// ResolveGitCommonDir returns the absolute path to the repository's shared git dir.
func ResolveGitCommonDir() (string, error) {
	return resolveGitPath("rev-parse", "--git-common-dir")
}

// ResolveRepoRoot returns the absolute path to the current repository worktree root.
func ResolveRepoRoot() (string, error) {
	return resolveGitPath("rev-parse", "--show-toplevel")
}

func CreateZipArchive(diffContent []byte) ([]byte, error) {
	return CreateZipArchiveWithExtras(diffContent, nil)
}

// CreateZipArchiveWithExtras builds the diff-review zip containing
// "diff.txt" plus any additional named entries (e.g. the raw .lrc/ tree,
// keyed by repo-relative path such as ".lrc/rules/security.md"). extras
// may be nil.
func CreateZipArchiveWithExtras(diffContent []byte, extras map[string][]byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	fileWriter, err := zipWriter.Create("diff.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create zip entry: %w", err)
	}

	if _, err := fileWriter.Write(diffContent); err != nil {
		return nil, fmt.Errorf("failed to write to zip: %w", err)
	}

	// Sort keys for deterministic zip output.
	names := make([]string, 0, len(extras))
	for name := range extras {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		extraWriter, err := zipWriter.Create(name)
		if err != nil {
			return nil, fmt.Errorf("failed to create zip entry %q: %w", name, err)
		}
		if _, err := extraWriter.Write(extras[name]); err != nil {
			return nil, fmt.Errorf("failed to write zip entry %q: %w", name, err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// formatJSONParseError creates a helpful error message when JSON parsing fails.
// It includes hints about common causes like wrong API URL/port.
func formatJSONParseError(body []byte, contentType string, parseErr error) error {
	bodyStr := string(body)
	preview := bodyStr
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}

	if strings.HasPrefix(strings.TrimSpace(bodyStr), "<") || strings.Contains(contentType, "text/html") {
		return fmt.Errorf("received HTML instead of JSON (Content-Type: %s).\n"+
			"This usually means api_url in ~/.lrc.toml points to the frontend UI instead of the API.\n"+
			"Check that api_url uses the correct port (default API port is 8888, not 8081).\n"+
			"Response preview: %s", contentType, preview)
	}

	return fmt.Errorf("failed to parse response as JSON: %w\nContent-Type: %s\nResponse preview: %s",
		parseErr, contentType, preview)
}

func SubmitReview(apiURL, apiKey, base64Diff, repoName string, toolsOnly bool, verbose bool) (reviewmodel.DiffReviewCreateResponse, error) {
	payload := reviewmodel.DiffReviewRequest{
		DiffZipBase64: base64Diff,
		RepoName:      repoName,
		ToolsOnly:     toolsOnly,
	}

	if verbose {
		log.Printf("POST %s", network.ReviewSubmitURL(apiURL))
	}

	client := network.NewReviewAPIClient(30 * time.Second)
	resp, err := network.ReviewSubmit(client, apiURL, payload, apiKey)
	if err != nil {
		return reviewmodel.DiffReviewCreateResponse{}, fmt.Errorf("failed to send request: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")

	if resp.StatusCode != http.StatusOK {
		return reviewmodel.DiffReviewCreateResponse{}, &reviewmodel.APIError{StatusCode: resp.StatusCode, Body: string(resp.Body)}
	}

	var result reviewmodel.DiffReviewCreateResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return reviewmodel.DiffReviewCreateResponse{}, formatJSONParseError(resp.Body, contentType, err)
	}

	if result.ReviewID == "" {
		return reviewmodel.DiffReviewCreateResponse{}, fmt.Errorf("review_id not found in response")
	}

	return result, nil
}

// trackCLIUsage sends a telemetry ping to the backend to track CLI usage
// This is a best-effort call and failures are silently ignored
func TrackCLIUsage(apiURL, apiKey string, verbose bool) {
	client := network.NewReviewAPIClient(5 * time.Second)
	resp, err := network.ReviewTrackCLIUsage(client, apiURL, apiKey)
	if err != nil {
		if verbose {
			log.Printf("Failed to send telemetry: %v", err)
		}
		return
	}

	if verbose && resp.StatusCode == http.StatusOK {
		log.Println("CLI usage tracked successfully")
	}
}

var ErrPollCancelled = errors.New("poll cancelled")
var ErrInputCancelled = errors.New("terminal input cancelled")

func PollReview(apiURL, apiKey, reviewID string, pollInterval, timeout time.Duration, verbose bool, cancel <-chan struct{}, statusSink func(string)) (*reviewmodel.DiffReviewResponse, error) {
	deadline := time.Now().Add(timeout)
	start := time.Now()
	if statusSink != nil {
		statusSink(fmt.Sprintf("waiting | poll=%s timeout=%s", pollInterval, timeout))
	} else {
		fmt.Printf("Waiting for review completion (poll every %s, timeout %s)...\r\n", pollInterval, timeout)
		interactive.SyncFileSafely(os.Stdout)
	}

	if verbose {
		log.Printf("Polling for review completion (timeout: %v)...", timeout)
	}

	client := network.NewReviewAPIClient(30 * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-cancel:
			if statusSink == nil {
				fmt.Printf("\r\n")
				interactive.SyncFileSafely(os.Stdout)
			}
			return nil, ErrPollCancelled
		default:
		}

		resp, err := network.ReviewPoll(client, apiURL, reviewID, apiKey)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}

		contentType := resp.Header.Get("Content-Type")

		if resp.StatusCode != http.StatusOK {
			return nil, &reviewmodel.APIError{StatusCode: resp.StatusCode, Body: string(resp.Body)}
		}

		var result reviewmodel.DiffReviewResponse
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			return nil, formatJSONParseError(resp.Body, contentType, err)
		}

		statusLine := fmt.Sprintf("Status: %s | elapsed: %s", result.Status, time.Since(start).Truncate(time.Second))
		if statusSink != nil {
			statusSink(statusLine)
		} else {
			fmt.Printf("\r%-80s", statusLine)
			interactive.SyncFileSafely(os.Stdout)
		}
		if verbose {
			log.Printf("%s", statusLine)
		}

		if result.Status == "completed" {
			if statusSink == nil {
				fmt.Printf("\r%-80s\r\n", statusLine)
				interactive.SyncFileSafely(os.Stdout)
			}
			return &result, nil
		}

		if result.Status == "failed" {
			if statusSink == nil {
				fmt.Printf("\r%-80s\r\n", statusLine)
				interactive.SyncFileSafely(os.Stdout)
			}
			reason := strings.TrimSpace(result.Message)
			if reason == "" {
				reason = "no additional details provided"
			}
			result.Summary = fmt.Sprintf("Review failed: %s", reason)
			return &result, fmt.Errorf("review failed: %s", reason)
		}

		select {
		case <-cancel:
			return nil, ErrPollCancelled
		case <-time.After(pollInterval):
		}
	}

	if statusSink == nil {
		fmt.Println()
	}
	return nil, fmt.Errorf("timeout waiting for review completion")
}
