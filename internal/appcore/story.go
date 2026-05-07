package appcore

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/story"
	"github.com/HexmosTech/git-lrc/story/plugins/vscodecopilot"
	"github.com/urfave/cli/v2"
)

func RunStorySources(c *cli.Context) error {
	provider, output, err := storyCommandContext(c)
	if err != nil {
		return err
	}

	sources, err := provider.Discover(story.DiscoverOptions{UserDataDir: strings.TrimSpace(c.String("user-data-dir"))})
	if err != nil {
		return err
	}
	if output == "json" {
		return writeJSON(os.Stdout, sources)
	}
	if len(sources) == 0 {
		fmt.Printf("No %s sources found.\n", provider.ID())
		return nil
	}
	for _, source := range sources {
		fmt.Printf("workspace=%s transcripts=%d\n", source.WorkspaceID, source.TranscriptCount)
		fmt.Printf("  user_data_root: %s\n", source.UserDataRoot)
		fmt.Printf("  workspace_storage: %s\n", source.WorkspaceStorageDir)
		fmt.Printf("  transcript_dir: %s\n", source.TranscriptDir)
	}
	return nil
}

func RunStorySessions(c *cli.Context) error {
	provider, output, err := storyCommandContext(c)
	if err != nil {
		return err
	}

	sessions, err := provider.ListSessions(story.ListSessionsOptions{UserDataDir: strings.TrimSpace(c.String("user-data-dir"))})
	if err != nil {
		return err
	}
	if output == "json" {
		return writeJSON(os.Stdout, sessions)
	}
	if len(sessions) == 0 {
		fmt.Printf("No %s sessions found.\n", provider.ID())
		return nil
	}
	for _, session := range sessions {
		fmt.Printf("session=%s workspace=%s events=%d\n", session.SessionID, session.WorkspaceID, session.EventCount)
		if session.StartedAt != nil {
			fmt.Printf("  started_at: %s\n", session.StartedAt.Format(time.RFC3339))
		}
		if session.UpdatedAt != nil {
			fmt.Printf("  updated_at: %s\n", session.UpdatedAt.Format(time.RFC3339))
		}
		if session.Preview != "" {
			fmt.Printf("  preview: %s\n", session.Preview)
		}
		fmt.Printf("  transcript_path: %s\n", session.TranscriptPath)
		for _, warning := range session.Warnings {
			fmt.Printf("  warning: %s\n", warning)
		}
	}
	return nil
}

func RunStoryInspect(c *cli.Context) error {
	provider, output, err := storyCommandContext(c)
	if err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.String("session-id"))
	if sessionID == "" {
		return fmt.Errorf("story inspect requires --session-id")
	}

	inspection, err := provider.InspectSession(story.InspectSessionOptions{
		UserDataDir: strings.TrimSpace(c.String("user-data-dir")),
		SessionID:   sessionID,
	})
	if err != nil {
		return err
	}
	if output == "json" {
		return writeJSON(os.Stdout, inspection)
	}
	fmt.Printf("session=%s workspace=%s\n", inspection.Summary.SessionID, inspection.Summary.WorkspaceID)
	fmt.Printf("  visible_messages: %d\n", inspection.VisibleMessages)
	fmt.Printf("  tool_request_count: %d\n", inspection.ToolRequestCount)
	if len(inspection.EventTypes) > 0 {
		fmt.Printf("  event_types: %s\n", strings.Join(inspection.EventTypes, ", "))
	}
	for _, warning := range inspection.Warnings {
		fmt.Printf("  warning: %s\n", warning)
	}
	return nil
}

func RunStoryExport(c *cli.Context) error {
	provider, output, err := storyCommandContext(c)
	if err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.String("session-id"))
	if sessionID == "" {
		return fmt.Errorf("story export requires --session-id")
	}

	chat, err := provider.ExportSession(story.ExportSessionOptions{
		UserDataDir: strings.TrimSpace(c.String("user-data-dir")),
		SessionID:   sessionID,
		Now:         time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	if output == "json" {
		return writeJSON(os.Stdout, chat)
	}
	fmt.Printf("exported session=%s provider=%s events=%d\n", chat.SessionID, chat.ProviderID, len(chat.Events))
	if chat.SourcePath != "" {
		fmt.Printf("  source_path: %s\n", chat.SourcePath)
	}
	for _, warning := range chat.Warnings {
		fmt.Printf("  warning: %s\n", warning)
	}
	return nil
}

func storyCommandContext(c *cli.Context) (story.Provider, string, error) {
	providerID := strings.TrimSpace(c.String("provider"))
	if providerID == "" {
		providerID = vscodecopilot.ProviderID
	}
	output := strings.TrimSpace(strings.ToLower(c.String("output")))
	if output == "" {
		output = "pretty"
	}
	if output != "pretty" && output != "json" {
		return nil, "", fmt.Errorf("invalid output format: %s (must be pretty or json)", output)
	}
	provider, err := storyRegistry().Provider(providerID)
	if err != nil {
		return nil, "", err
	}
	return provider, output, nil
}

func storyRegistry() *story.Registry {
	registry, err := story.NewRegistry(vscodecopilot.NewAdapter())
	if err != nil {
		panic(err)
	}
	return registry
}

func writeJSON(output *os.File, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
