package appcore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	neturl "net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/internal/reviewapi"
	"github.com/HexmosTech/git-lrc/story"
)

var reviewStoryRunGitCommand = reviewapi.RunGitCommand

type reviewStoryCommitContext struct {
	HeadCommit   string     `json:"head_commit,omitempty"`
	WindowStart  *time.Time `json:"window_start,omitempty"`
	WindowEnd    time.Time  `json:"window_end"`
	ChangedFiles []string   `json:"changed_files,omitempty"`
}

type reviewStorySessionRef struct {
	ProviderID string `json:"provider_id"`
	SessionID  string `json:"session_id"`
}

type reviewStorySession struct {
	ProviderID          string     `json:"provider_id"`
	SessionID           string     `json:"session_id"`
	WorkspaceID         string     `json:"workspace_id,omitempty"`
	SessionScope        string     `json:"session_scope,omitempty"`
	DisplayTitle        string     `json:"display_title,omitempty"`
	Preview             string     `json:"preview,omitempty"`
	DraftInput          string     `json:"draft_input,omitempty"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	UpdatedAt           *time.Time `json:"updated_at,omitempty"`
	EventCount          int        `json:"event_count"`
	VisibleMessageCount int        `json:"visible_message_count,omitempty"`
	ToolRequestCount    int        `json:"tool_request_count,omitempty"`
	Warnings            []string   `json:"warnings,omitempty"`
	MatchScore          int        `json:"match_score"`
	MatchReasons        []string   `json:"match_reasons,omitempty"`
	Recommended         bool       `json:"recommended,omitempty"`
	WithinCommitWindow  bool       `json:"within_commit_window,omitempty"`
	MatchedFiles        []string   `json:"matched_files,omitempty"`
	SourcePath          string     `json:"source_path,omitempty"`
	UserDataRoot        string     `json:"user_data_root,omitempty"`
	ChatSessionPath     string     `json:"chat_session_path,omitempty"`
	Transcript          string     `json:"transcript_path,omitempty"`
	ProviderRef         string     `json:"provider_ref,omitempty"`
}

type reviewStorySessionsResponse struct {
	Commit      reviewStoryCommitContext `json:"commit"`
	Sessions    []reviewStorySession     `json:"sessions"`
	Recommended *reviewStorySessionRef   `json:"recommended,omitempty"`
}

func handleReviewStorySessions(w http.ResponseWriter, r *http.Request, state *ReviewState) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	response, err := buildReviewStorySessionsResponse(state, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "failed to write story sessions", http.StatusInternalServerError)
	}
}

func handleReviewStorySession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	providerID := strings.TrimSpace(r.URL.Query().Get("provider"))
	if providerID == "" {
		http.Error(w, "story session requires provider", http.StatusBadRequest)
		return
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		http.Error(w, "story session requires session_id", http.StatusBadRequest)
		return
	}

	provider, err := storyRegistry().Provider(providerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	chat, err := provider.ExportSession(story.ExportSessionOptions{
		SessionID: sessionID,
		Now:       time.Now().UTC(),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(chat); err != nil {
		http.Error(w, "failed to write story session", http.StatusInternalServerError)
	}
}

func buildReviewStorySessionsResponse(state *ReviewState, now time.Time) (*reviewStorySessionsResponse, error) {
	commitContext := buildReviewStoryCommitContext(state, now)
	response := &reviewStorySessionsResponse{Commit: commitContext}

	providers := storyRegistry().ProviderIDs()
	for _, providerID := range providers {
		provider, err := storyRegistry().Provider(providerID)
		if err != nil {
			return nil, err
		}
		sessions, err := provider.ListSessions(story.ListSessionsOptions{})
		if err != nil {
			return nil, fmt.Errorf("list story sessions for %s: %w", providerID, err)
		}
		for _, session := range sessions {
			candidate := reviewStorySession{
				ProviderID:          session.ProviderID,
				SessionID:           session.SessionID,
				WorkspaceID:         session.WorkspaceID,
				SessionScope:        session.SessionScope,
				DisplayTitle:        session.DisplayTitle,
				Preview:             session.Preview,
				DraftInput:          session.DraftInput,
				StartedAt:           session.StartedAt,
				UpdatedAt:           session.UpdatedAt,
				EventCount:          session.EventCount,
				VisibleMessageCount: session.VisibleMessageCount,
				ToolRequestCount:    session.ToolRequestCount,
				Warnings:            append([]string(nil), session.Warnings...),
				SourcePath:          firstNonEmpty(session.ChatSessionPath, session.TranscriptPath),
				UserDataRoot:        session.UserDataRoot,
				ChatSessionPath:     session.ChatSessionPath,
				Transcript:          session.TranscriptPath,
				ProviderRef:         session.ProviderID + ":" + session.SessionID,
			}

			candidate.MatchScore, candidate.MatchReasons, candidate.WithinCommitWindow = scoreStorySessionAgainstCommitWindow(session, commitContext, now)
			if len(commitContext.ChangedFiles) > 0 {
				extraScore, extraReasons, matchedFiles := scoreStorySessionSummaryAgainstChangedFiles(session, commitContext.ChangedFiles)
				candidate.MatchScore += extraScore
				candidate.MatchReasons = append(candidate.MatchReasons, extraReasons...)
				candidate.MatchedFiles = matchedFiles
				if candidate.WithinCommitWindow && len(matchedFiles) > 0 {
					candidate.MatchScore += 28
					candidate.MatchReasons = append(candidate.MatchReasons, "in commit window and mentions changed files")
				}
			}

			candidate.MatchReasons = uniqueStoryStrings(candidate.MatchReasons)
			response.Sessions = append(response.Sessions, candidate)
		}
	}

	sort.SliceStable(response.Sessions, func(i, j int) bool {
		left := response.Sessions[i]
		right := response.Sessions[j]
		if left.MatchScore != right.MatchScore {
			return left.MatchScore > right.MatchScore
		}
		if left.WithinCommitWindow != right.WithinCommitWindow {
			return left.WithinCommitWindow
		}
		if left.UpdatedAt != nil && right.UpdatedAt != nil && !left.UpdatedAt.Equal(*right.UpdatedAt) {
			return left.UpdatedAt.After(*right.UpdatedAt)
		}
		if left.UpdatedAt != nil && right.UpdatedAt == nil {
			return true
		}
		if left.UpdatedAt == nil && right.UpdatedAt != nil {
			return false
		}
		if left.ProviderID != right.ProviderID {
			return left.ProviderID < right.ProviderID
		}
		return left.SessionID < right.SessionID
	})

	if len(response.Sessions) > 0 {
		response.Recommended = &reviewStorySessionRef{
			ProviderID: response.Sessions[0].ProviderID,
			SessionID:  response.Sessions[0].SessionID,
		}
		response.Sessions[0].Recommended = true
	}

	return response, nil
}

func buildReviewStoryCommitContext(state *ReviewState, now time.Time) reviewStoryCommitContext {
	context := reviewStoryCommitContext{
		WindowEnd:    now,
		ChangedFiles: changedFilesFromReviewState(state),
	}

	headCommit, commitTime, err := readLatestCommitMetadata()
	if err == nil {
		context.HeadCommit = headCommit
		context.WindowStart = &commitTime
	}

	return context
}

func changedFilesFromReviewState(state *ReviewState) []string {
	if state == nil {
		return nil
	}

	state.mu.RLock()
	defer state.mu.RUnlock()

	paths := make([]string, 0, len(state.Files))
	seen := make(map[string]struct{}, len(state.Files))
	for _, file := range state.Files {
		path := strings.TrimSpace(file.FilePath)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func readLatestCommitMetadata() (string, time.Time, error) {
	out, err := reviewStoryRunGitCommand("log", "-1", "--format=%H%n%cI", "HEAD")
	if err != nil {
		return "", time.Time{}, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return "", time.Time{}, fmt.Errorf("unexpected git log output for latest commit metadata")
	}

	headCommit := strings.TrimSpace(lines[0])
	if headCommit == "" {
		return "", time.Time{}, fmt.Errorf("latest commit hash is empty")
	}
	commitTime, err := time.Parse(time.RFC3339, strings.TrimSpace(lines[1]))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("parse latest commit timestamp: %w", err)
	}

	return headCommit, commitTime.UTC(), nil
}

func scoreStorySessionAgainstCommitWindow(session story.SessionSummary, context reviewStoryCommitContext, now time.Time) (int, []string, bool) {
	score := 0
	reasons := make([]string, 0, 3)
	if context.WindowStart == nil {
		if session.UpdatedAt != nil {
			age := now.Sub(*session.UpdatedAt)
			switch {
			case age <= 30*time.Minute:
				score += 10
				reasons = append(reasons, "recent session")
			case age <= 6*time.Hour:
				score += 4
			}
		}
		return score, reasons, false
	}

	windowStart := *context.WindowStart
	windowEnd := context.WindowEnd
	sessionStart := session.StartedAt
	sessionEnd := session.UpdatedAt
	if sessionStart == nil {
		sessionStart = sessionEnd
	}
	if sessionEnd == nil {
		sessionEnd = sessionStart
	}

	overlapsWindow := false
	if sessionStart != nil && sessionEnd != nil {
		overlapsWindow = !sessionEnd.Before(windowStart) && !sessionStart.After(windowEnd)
	} else if session.UpdatedAt != nil {
		overlapsWindow = !session.UpdatedAt.Before(windowStart) && !session.UpdatedAt.After(windowEnd)
	}

	if overlapsWindow {
		score += 140
		reasons = append(reasons, "in current commit window")
		if sessionStart != nil && sessionEnd != nil && !sessionStart.Before(windowStart) && !sessionEnd.After(windowEnd) {
			score += 24
			reasons = append(reasons, "fully inside commit window")
		}
	} else if sessionEnd != nil {
		delta := windowStart.Sub(*sessionEnd)
		if delta < 0 {
			delta = -delta
		}
		if delta <= 30*time.Minute {
			score += 16
			reasons = append(reasons, "near commit window")
		}
	}

	if session.UpdatedAt != nil {
		age := now.Sub(*session.UpdatedAt)
		switch {
		case age <= 30*time.Minute:
			score += 8
			reasons = append(reasons, "recent activity")
		case age <= 6*time.Hour:
			score += 3
		}
	}

	return score, reasons, overlapsWindow
}

func scoreStorySessionSummaryAgainstChangedFiles(session story.SessionSummary, changedFiles []string) (int, []string, []string) {
	if len(changedFiles) == 0 {
		return 0, nil, nil
	}

	combined := strings.ToLower(strings.Join([]string{
		session.DisplayTitle,
		session.Preview,
		session.DraftInput,
		session.ChatSessionPath,
		session.TranscriptPath,
	}, "\n"))
	if combined == "" {
		return 0, nil, nil
	}

	score := 0
	reasons := make([]string, 0, 2)
	matchedFiles := make([]string, 0)

	for _, changedFile := range changedFiles {
		pathNeedle := strings.ToLower(strings.TrimSpace(changedFile))
		if pathNeedle == "" {
			continue
		}
		baseNeedle := strings.ToLower(filepath.Base(pathNeedle))
		switch {
		case strings.Contains(combined, pathNeedle):
			score += 48
			matchedFiles = append(matchedFiles, changedFile)
		case baseNeedle != "" && baseNeedle != pathNeedle && strings.Contains(combined, baseNeedle):
			score += 18
			matchedFiles = append(matchedFiles, changedFile)
		}
	}

	matchedFiles = uniqueStoryStrings(matchedFiles)
	if len(matchedFiles) > 0 {
		reasons = append(reasons, fmt.Sprintf("mentions %d changed file(s)", len(matchedFiles)))
	}

	return score, uniqueStoryStrings(reasons), matchedFiles
}

func allowLocalStoryRequest(w http.ResponseWriter, r *http.Request) bool {
	host := stripPort(strings.TrimSpace(r.Host))
	if !isLoopbackHost(host) {
		http.Error(w, "story endpoints are only available via localhost", http.StatusForbidden)
		return false
	}
	if !sameLocalHostHeader(r.Header.Get("Origin"), host) || !sameLocalHostHeader(r.Header.Get("Referer"), host) {
		http.Error(w, "story endpoints reject non-local origins", http.StatusForbidden)
		return false
	}
	return true
}

func stripPort(host string) string {
	if strings.HasPrefix(host, "[") {
		if end := strings.Index(host, "]"); end != -1 {
			return host[1:end]
		}
	}
	if value, err := netip.ParseAddrPort(host); err == nil {
		return value.Addr().String()
	}
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		return host[:idx]
	}
	return host
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "localhost" {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

func sameLocalHostHeader(value string, expectedHost string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	parsed, err := neturl.Parse(value)
	if err != nil {
		return false
	}
	return isLoopbackHost(parsed.Hostname()) && isLoopbackHost(expectedHost)
}

func uniqueStoryStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
