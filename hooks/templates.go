package hooks

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed *.sh
var templatesFS embed.FS

const (
	hookMarkerBeginPlaceholder       = "__LRC_MARKER_BEGIN__"
	hookMarkerEndPlaceholder         = "__LRC_MARKER_END__"
	hookVersionPlaceholder           = "__LRC_VERSION__"
	hookCommitMessageFilePlaceholder = "__LRC_COMMIT_MESSAGE_FILE__"
	hookPushRequestFilePlaceholder   = "__LRC_PUSH_REQUEST_FILE__"
	hookNamePlaceholder              = "__HOOK_NAME__"
)

// TemplateConfig provides replacement values for hook template placeholders.
type TemplateConfig struct {
	MarkerBegin       string
	MarkerEnd         string
	Version           string
	CommitMessageFile string
	PushRequestFile   string
}

func mustRenderTemplate(path string, replacements map[string]string) string {
	content, err := templatesFS.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("failed to load hook template %s: %v", path, err))
	}

	rendered := string(content)
	for placeholder, value := range replacements {
		rendered = strings.ReplaceAll(rendered, placeholder, value)
	}

	return rendered
}

func GeneratePreCommitHook(cfg TemplateConfig) string {
	return mustRenderTemplate("pre-commit.sh", map[string]string{
		hookMarkerBeginPlaceholder: cfg.MarkerBegin,
		hookMarkerEndPlaceholder:   cfg.MarkerEnd,
		hookVersionPlaceholder:     cfg.Version,
	})
}

func GeneratePrepareCommitMsgHook(cfg TemplateConfig) string {
	return mustRenderTemplate("prepare-commit-msg.sh", map[string]string{
		hookMarkerBeginPlaceholder: cfg.MarkerBegin,
		hookMarkerEndPlaceholder:   cfg.MarkerEnd,
		hookVersionPlaceholder:     cfg.Version,
	})
}

func GenerateCommitMsgHook(cfg TemplateConfig) string {
	return mustRenderTemplate("commit-msg.sh", map[string]string{
		hookMarkerBeginPlaceholder:       cfg.MarkerBegin,
		hookMarkerEndPlaceholder:         cfg.MarkerEnd,
		hookVersionPlaceholder:           cfg.Version,
		hookCommitMessageFilePlaceholder: cfg.CommitMessageFile,
	})
}

func GeneratePostCommitHook(cfg TemplateConfig) string {
	return mustRenderTemplate("post-commit.sh", map[string]string{
		hookMarkerBeginPlaceholder:     cfg.MarkerBegin,
		hookMarkerEndPlaceholder:       cfg.MarkerEnd,
		hookVersionPlaceholder:         cfg.Version,
		hookPushRequestFilePlaceholder: cfg.PushRequestFile,
	})
}

func GenerateDispatcherHook(hookName string, cfg TemplateConfig) string {
	return mustRenderTemplate("dispatcher.sh", map[string]string{
		hookMarkerBeginPlaceholder: cfg.MarkerBegin,
		hookMarkerEndPlaceholder:   cfg.MarkerEnd,
		hookVersionPlaceholder:     cfg.Version,
		hookNamePlaceholder:        hookName,
	})
}
