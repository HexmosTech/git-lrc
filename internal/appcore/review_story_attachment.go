package appcore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/internal/reviewapi"
	"github.com/HexmosTech/git-lrc/storage"
	"github.com/HexmosTech/git-lrc/story"
)

type reviewStoryAttachRequest struct {
	ProviderID string `json:"provider_id"`
	SessionID  string `json:"session_id"`
}

type reviewStoryAttachmentResponse struct {
	Attachment *reviewStoryAttachmentSummary `json:"attachment,omitempty"`
}

func handleReviewStoryAttach(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeReviewStoryAttachRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	attachment, err := attachReviewStorySession(request.ProviderID, request.SessionID, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(reviewStoryAttachmentResponse{Attachment: attachment}); err != nil {
		http.Error(w, "failed to write story attachment", http.StatusInternalServerError)
	}
}

func handleReviewStoryDetach(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if err := detachReviewStorySession(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(reviewStoryAttachmentResponse{}); err != nil {
		http.Error(w, "failed to write story attachment response", http.StatusInternalServerError)
	}
}

func decodeReviewStoryAttachRequest(r *http.Request) (*reviewStoryAttachRequest, error) {
	if r.Body == nil {
		return nil, fmt.Errorf("story attach requires request body")
	}
	defer r.Body.Close()

	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read story attach request: %w", err)
	}

	var request reviewStoryAttachRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("invalid story attach request JSON: %w", err)
	}

	request.ProviderID = strings.TrimSpace(request.ProviderID)
	request.SessionID = strings.TrimSpace(request.SessionID)
	if request.ProviderID == "" {
		return nil, fmt.Errorf("story attach requires provider_id")
	}
	if request.SessionID == "" {
		return nil, fmt.Errorf("story attach requires session_id")
	}

	return &request, nil
}

func readReviewStoryAttachmentSummary() (*reviewStoryAttachmentSummary, error) {
	gitDir, err := reviewapi.ResolveGitDir()
	if err != nil {
		return nil, nil
	}

	state, err := storage.ReadStoryAttachmentState(storage.StoryAttachmentMetadataPath(gitDir))
	if err != nil {
		return nil, fmt.Errorf("read story attachment state: %w", err)
	}
	if state == nil {
		return nil, nil
	}

	return reviewStoryAttachmentSummaryFromState(state), nil
}

func reviewStoryAttachmentSummaryFromState(state *storage.StoryAttachmentState) *reviewStoryAttachmentSummary {
	if state == nil {
		return nil
	}

	return &reviewStoryAttachmentSummary{
		ProviderID:     state.ProviderID,
		SessionID:      state.SessionID,
		ReviewTreeHash: state.ReviewTreeHash,
		Path:           storyAttachmentRelativePath(state),
		DisplayTitle:   state.DisplayTitle,
		AttachedAt:     state.AttachedAt,
	}
}

func attachReviewStorySession(providerID, sessionID string, now time.Time) (*reviewStoryAttachmentSummary, error) {
	provider, err := storyRegistry().Provider(strings.TrimSpace(providerID))
	if err != nil {
		return nil, err
	}

	chat, err := provider.ExportSession(story.ExportSessionOptions{
		SessionID: strings.TrimSpace(sessionID),
		Now:       now,
	})
	if err != nil {
		return nil, fmt.Errorf("export story session: %w", err)
	}

	gitDir, err := reviewapi.ResolveGitDir()
	if err != nil {
		return nil, fmt.Errorf("resolve git dir: %w", err)
	}
	repoRoot, err := reviewapi.ResolveRepoRoot()
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	reviewTreeHash, err := reviewapi.CurrentTreeHash()
	if err != nil {
		return nil, fmt.Errorf("resolve staged tree hash: %w", err)
	}
	if strings.TrimSpace(reviewTreeHash) == "" {
		return nil, fmt.Errorf("staged tree hash is empty")
	}

	metadataPath := storage.StoryAttachmentMetadataPath(gitDir)
	existing, err := storage.ReadStoryAttachmentState(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("read existing story attachment state: %w", err)
	}

	markdownPath := storage.StoryAttachmentMarkdownPath(repoRoot, reviewTreeHash)
	markdownRelativePath, err := filepath.Rel(repoRoot, markdownPath)
	if err != nil {
		return nil, fmt.Errorf("resolve story attachment path: %w", err)
	}
	markdownRelativePath = filepath.ToSlash(markdownRelativePath)

	if existing != nil {
		existingMarkdownPath := storyAttachmentAbsolutePath(repoRoot, existing)
		if existingMarkdownPath != markdownPath {
			if err := storage.RemoveStoryAttachmentMarkdown(existingMarkdownPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("remove previous story attachment markdown: %w", err)
			}
		}
	}

	markdown := buildStoryAttachmentMarkdown(chat, now, reviewTreeHash)
	if err := storage.WriteStoryAttachmentMarkdown(markdownPath, []byte(markdown)); err != nil {
		return nil, err
	}

	state := storage.StoryAttachmentState{
		ProviderID:           chat.ProviderID,
		SessionID:            chat.SessionID,
		ReviewTreeHash:       reviewTreeHash,
		MarkdownRelativePath: markdownRelativePath,
		DisplayTitle:         firstNonEmpty(strings.TrimSpace(chat.DisplayTitle), strings.TrimSpace(chat.DraftInput), chat.SessionID),
		AttachedAt:           now,
	}
	if err := storage.WriteStoryAttachmentState(metadataPath, state); err != nil {
		return nil, err
	}

	return reviewStoryAttachmentSummaryFromState(&state), nil
}

func detachReviewStorySession() error {
	gitDir, err := reviewapi.ResolveGitDir()
	if err != nil {
		return fmt.Errorf("resolve git dir: %w", err)
	}
	repoRoot, err := reviewapi.ResolveRepoRoot()
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}

	metadataPath := storage.StoryAttachmentMetadataPath(gitDir)
	state, err := storage.ReadStoryAttachmentState(metadataPath)
	if err != nil {
		return fmt.Errorf("read story attachment state: %w", err)
	}
	if state == nil {
		return nil
	}

	if err := storage.RemoveStoryAttachmentMarkdown(storyAttachmentAbsolutePath(repoRoot, state)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove story attachment markdown: %w", err)
	}
	if err := storage.RemoveStoryAttachmentState(metadataPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove story attachment state: %w", err)
	}

	return nil
}

func finalizePendingStoryAttachment(verbose bool) error {
	gitDir, err := reviewapi.ResolveGitDir()
	if err != nil {
		return nil
	}
	repoRoot, err := reviewapi.ResolveRepoRoot()
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}

	metadataPath := storage.StoryAttachmentMetadataPath(gitDir)
	state, err := storage.ReadStoryAttachmentState(metadataPath)
	if err != nil {
		return fmt.Errorf("read story attachment state: %w", err)
	}
	if state == nil {
		return nil
	}

	currentTreeHash, err := reviewapi.CurrentTreeHash()
	if err != nil {
		return fmt.Errorf("resolve staged tree hash: %w", err)
	}
	if strings.TrimSpace(currentTreeHash) != strings.TrimSpace(state.ReviewTreeHash) {
		return fmt.Errorf("story attachment no longer matches the staged tree; reattach it from the Story tab")
	}

	markdownPath := storyAttachmentAbsolutePath(repoRoot, state)
	exists, err := storage.PathExists(markdownPath)
	if err != nil {
		return fmt.Errorf("stat story attachment markdown: %w", err)
	}
	if !exists {
		return fmt.Errorf("story attachment markdown is missing: %s", storyAttachmentRelativePath(state))
	}

	if _, err := reviewapi.RunGitCommand("-C", repoRoot, "add", "--", storyAttachmentRelativePath(state)); err != nil {
		return fmt.Errorf("stage story attachment markdown: %w", err)
	}

	payload, err := readAttestationForTree(state.ReviewTreeHash)
	if err != nil {
		return fmt.Errorf("read source attestation: %w", err)
	}
	if payload == nil {
		return fmt.Errorf("missing attestation for staged tree %s", state.ReviewTreeHash)
	}

	finalTreeHashBytes, err := reviewapi.RunGitCommand("-C", repoRoot, "write-tree")
	if err != nil {
		return fmt.Errorf("compute final tree hash: %w", err)
	}
	finalTreeHash := strings.TrimSpace(string(finalTreeHashBytes))
	if finalTreeHash == "" {
		return fmt.Errorf("final tree hash is empty")
	}

	if _, err := writeAttestationFullForTree(finalTreeHash, *payload); err != nil {
		return fmt.Errorf("write final tree attestation: %w", err)
	}
	if finalTreeHash != state.ReviewTreeHash {
		if err := deleteAttestationForTree(state.ReviewTreeHash); err != nil {
			return fmt.Errorf("remove superseded attestation: %w", err)
		}
	}

	if err := storage.RemoveStoryAttachmentState(metadataPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("clear story attachment state: %w", err)
	}

	if verbose {
		log.Printf("Story attachment finalized: %s", storyAttachmentRelativePath(state))
	}

	return nil
}

func storyAttachmentRelativePath(state *storage.StoryAttachmentState) string {
	if state == nil {
		return ""
	}
	if strings.TrimSpace(state.MarkdownRelativePath) != "" {
		return filepath.ToSlash(state.MarkdownRelativePath)
	}
	return filepath.ToSlash(filepath.Join(".lrc", fmt.Sprintf("%s.md", strings.TrimSpace(state.ReviewTreeHash))))
}

func storyAttachmentAbsolutePath(repoRoot string, state *storage.StoryAttachmentState) string {
	return filepath.Join(repoRoot, filepath.FromSlash(storyAttachmentRelativePath(state)))
}

func buildStoryAttachmentMarkdown(chat *story.CommonChat, now time.Time, reviewTreeHash string) string {
	assistantMessages := collectStoryAttachmentAssistantMessages(chat)
	prompt := strings.TrimSpace(storyAttachmentOpeningPrompt(chat))
	title := firstNonEmpty(strings.TrimSpace(chat.DisplayTitle), prompt, chat.SessionID, "Story")

	var builder strings.Builder
	builder.WriteString("# Git Story\n\n")
	builder.WriteString(fmt.Sprintf("- Title: %s\n", title))
	builder.WriteString(fmt.Sprintf("- Provider: %s\n", strings.TrimSpace(chat.ProviderID)))
	builder.WriteString(fmt.Sprintf("- Session ID: %s\n", strings.TrimSpace(chat.SessionID)))
	builder.WriteString(fmt.Sprintf("- Review tree hash: %s\n", strings.TrimSpace(reviewTreeHash)))
	builder.WriteString(fmt.Sprintf("- Attached at: %s\n", now.UTC().Format(time.RFC3339)))

	if prompt != "" {
		builder.WriteString("\n## User Prompt\n\n")
		builder.WriteString(prompt)
		builder.WriteString("\n")
	}

	for index, message := range assistantMessages {
		builder.WriteString(fmt.Sprintf("\n## Assistant Response %d\n\n", index+1))
		builder.WriteString(message)
		builder.WriteString("\n")
	}

	return builder.String()
}

func storyAttachmentOpeningPrompt(chat *story.CommonChat) string {
	if chat == nil {
		return ""
	}
	if strings.TrimSpace(chat.DraftInput) != "" {
		return strings.TrimSpace(chat.DraftInput)
	}
	for _, event := range chat.Events {
		if event.Message == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(event.Message.Role), "user") && strings.TrimSpace(event.Message.Content) != "" {
			return strings.TrimSpace(event.Message.Content)
		}
	}
	return ""
}

func collectStoryAttachmentAssistantMessages(chat *story.CommonChat) []string {
	if chat == nil {
		return nil
	}

	messages := make([]string, 0, len(chat.Events))
	lastMessage := ""
	for _, event := range chat.Events {
		if event.Message == nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(event.Message.Role), "assistant") {
			continue
		}
		content := strings.TrimSpace(event.Message.Content)
		if content == "" || content == lastMessage {
			continue
		}
		messages = append(messages, content)
		lastMessage = content
	}

	return messages
}
