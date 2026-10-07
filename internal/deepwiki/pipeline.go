package deepwiki

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Completer is the minimal LLM seam the pipeline depends on. The network
// package provides the implementation (see network/deepwiki_llm.go).
type Completer interface {
	Complete(ctx context.Context, cwd, system, prompt string) (string, error)
}

// Options configures a generation run for one repo at one ref.
type Options struct {
	RepoPath      string // absolute path to the (checked-out) repository
	Ref           string // branch/tag/commit being documented
	Owner         string // display owner (defaults to "local")
	Repo          string // display repo name (defaults to basename of RepoPath)
	Comprehensive bool
	Language      string // e.g. "en"; empty means English
	TreeHash      string // optional git tree hash, reserved for incremental updates
	Provider      string // informational, persisted into the artifact
	Model         string // informational, persisted into the artifact

	// MaxFileBytes caps a single file's inlined content; 0 uses the default.
	MaxFileBytes int
	// MaxPageBytes caps total inlined content per page; 0 uses the default.
	MaxPageBytes int
}

// Progress receives coarse phase updates plus per-page completion.
// done/total counts generated pages; pageID identifies the just-finished page
// (empty for non-page phases).
type Progress func(status TaskStatus, done, total int, pageID string)

const (
	defaultMaxFileBytes = 40 * 1024
	defaultMaxPageBytes = 160 * 1024
)

// Generate runs the full wiki pipeline: file tree -> structure (LLM) ->
// page generation (LLM) -> citation post-processing. It returns a Wiki with
// every page's Content populated.
func Generate(ctx context.Context, llm Completer, opts Options, progress Progress) (*Wiki, error) {
	if llm == nil {
		return nil, errors.New("deepwiki: nil completer")
	}
	if opts.RepoPath == "" {
		return nil, errors.New("deepwiki: empty RepoPath")
	}
	if opts.Repo == "" {
		opts.Repo = filepath.Base(filepath.Clean(opts.RepoPath))
	}
	if opts.Owner == "" {
		opts.Owner = "local"
	}
	if opts.Language == "" {
		opts.Language = "en"
	}
	if opts.MaxFileBytes == 0 {
		opts.MaxFileBytes = defaultMaxFileBytes
	}
	if opts.MaxPageBytes == 0 {
		opts.MaxPageBytes = defaultMaxPageBytes
	}

	notify := func(status TaskStatus, done, total int, pageID string) {
		if progress != nil {
			progress(status, done, total, pageID)
		}
	}

	fileTree, readme, err := readRepoFileTree(opts.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("deepwiki: reading repo tree: %w", err)
	}

	notify(TaskDeterminingStructure, 0, 0, "")
	structurePrompt := buildStructurePrompt(opts.Owner, opts.Repo, strings.Join(fileTree, "\n"), readme, opts.Comprehensive, opts.Language)
	structureText, err := llm.Complete(ctx, opts.RepoPath, "", structurePrompt)
	if err != nil {
		return nil, fmt.Errorf("deepwiki: determining structure: %w", err)
	}
	structure, err := parseWikiStructure(structureText, opts.Comprehensive)
	if err != nil {
		return nil, fmt.Errorf("deepwiki: parsing structure: %w", err)
	}

	total := len(structure.Pages)
	notify(TaskGenerating, 0, total, "")

	for i := range structure.Pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page := &structure.Pages[i]

		fileLinks := buildFileLinks(page.FilePaths)
		fileContents := readFileContents(opts.RepoPath, page.FilePaths, opts.MaxFileBytes, opts.MaxPageBytes)

		pagePrompt := buildPagePrompt(page.Title, fileLinks, fileContents, opts.Language)
		content, err := llm.Complete(ctx, opts.RepoPath, "", pagePrompt)
		if err != nil {
			return nil, fmt.Errorf("deepwiki: generating page %q: %w", page.ID, err)
		}
		content = stripMarkdownFences(content)
		page.Content = postProcessWikiContent(content, page.FilePaths)

		notify(TaskGenerating, i+1, total, page.ID)
	}

	wiki := &Wiki{
		Structure:   *structure,
		Ref:         opts.Ref,
		TreeHash:    opts.TreeHash,
		GeneratedAt: nowMillis(),
		Provider:    opts.Provider,
		Model:       opts.Model,
	}
	notify(TaskCompleted, total, total, "")
	return wiki, nil
}

// buildFileLinks renders the "- [path](url)" markdown list used to seed the
// page prompt's <details> block. For local repos the paths are rendered as
// code spans (no host URL).
func buildFileLinks(filePaths []string) string {
	if len(filePaths) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range filePaths {
		b.WriteString("- `" + p + "`\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// readFileContents inlines the content of filePaths, bounded per-file and
// per-page, prefixing each file with a header.
func readFileContents(root string, filePaths []string, maxFile, maxPage int) string {
	if len(filePaths) == 0 {
		return "(no source files)"
	}
	var b strings.Builder
	used := 0
	for _, rel := range filePaths {
		if used >= maxPage {
			b.WriteString("\n[... additional files truncated]\n")
			break
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		text := string(data)
		if len(text) > maxFile {
			text = text[:maxFile] + "\n[... truncated]"
		}
		b.WriteString("===== " + rel + " =====\n")
		b.WriteString(text)
		if !strings.HasSuffix(text, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
		used += len(text)
	}
	if b.Len() == 0 {
		return "(no readable source files)"
	}
	return b.String()
}

// nowMillis returns the current Unix time in milliseconds.
func nowMillis() int64 {
	return time.Now().UnixMilli()
}
