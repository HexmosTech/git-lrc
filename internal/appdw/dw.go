package appdw

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/HexmosTech/git-lrc/internal/deepwiki"
	"github.com/HexmosTech/git-lrc/network"
	"github.com/HexmosTech/git-lrc/storage"
	"github.com/urfave/cli/v2"
)

// RunDW is the action behind the `lrc dw` command. It generates (or loads) a
// deepwiki for the repository in the current directory and either serves the
// Preact UI (default) or prints a machine-readable export (--output json |
// markdown, or --no-serve).
func RunDW(c *cli.Context) error {
	repoPath, err := resolveRepo()
	if err != nil {
		return fmt.Errorf("deepwiki requires a git repository: %w", err)
	}

	ref := trimSpace(c.String("ref"))
	if ref == "" {
		ref = currentRef(repoPath)
	}

	provider := trimSpace(c.String("dw-provider"))
	if provider == "" {
		provider = "opencode"
	}
	model := trimSpace(c.String("dw-model"))
	baseURL := trimSpace(c.String("dw-base-url"))
	apiKey := trimSpace(c.String("dw-api-key"))
	comprehensive := !c.Bool("concise")
	regenerate := c.Bool("regenerate")
	output := trimSpace(c.String("output"))
	if output == "" {
		output = "ui"
	}

	llm := network.NewDeepwikiLLM(provider, model, baseURL, apiKey)

	cliMode := c.Bool("no-serve") || output == "json" || output == "markdown"
	if cliMode {
		return runCLI(repoPath, ref, llm, provider, model, comprehensive, regenerate, output)
	}

	port := c.Int("port")
	if port == 0 {
		port = defaultDWPort
	}
	return serveDW(repoPath, llm, provider, model, comprehensive, port, true)
}

// runCLI generates or loads the wiki and prints it, for scripts and agents.
func runCLI(repoPath, ref string, llm deepwiki.Completer, provider, model string, comprehensive, regenerate bool, output string) error {
	wiki, err := generateOrLoad(context.Background(), repoPath, ref, llm, provider, model, comprehensive, regenerate)
	if err != nil {
		return err
	}

	switch output {
	case "json":
		data, err := json.MarshalIndent(wiki, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	case "markdown":
		fmt.Print(wiki.Markdown())
	default:
		// --no-serve with no explicit output defaults to markdown.
		fmt.Print(wiki.Markdown())
	}
	return nil
}

// generateOrLoad returns the cached wiki when present (and not forced),
// otherwise generates, saves, and returns it.
func generateOrLoad(ctx context.Context, repoPath, ref string, llm deepwiki.Completer, provider, model string, comprehensive, regenerate bool) (*deepwiki.Wiki, error) {
	if !regenerate {
		if data, err := storage.DeepwikiLoad(repoPath, ref); err == nil {
			var w deepwiki.Wiki
			if json.Unmarshal(data, &w) == nil {
				return &w, nil
			}
		}
	}

	fmt.Fprintf(os.Stderr, "Generating deepwiki for %q (ref %q) using provider %q...\n", repoPath, ref, provider)

	workDir, cleanup, err := materializeRef(repoPath, ref)
	if err != nil {
		return nil, fmt.Errorf("materializing ref %q: %w", ref, err)
	}
	defer cleanup()

	wiki, err := deepwiki.Generate(ctx, llm, deepwiki.Options{
		RepoPath:      workDir,
		Ref:           ref,
		Comprehensive: comprehensive,
		Language:      "en",
		TreeHash:      treeHash(repoPath, ref),
		Provider:      provider,
		Model:         model,
	}, func(status deepwiki.TaskStatus, done, total int, pageID string) {
		switch status {
		case deepwiki.TaskDeterminingStructure:
			fmt.Fprintln(os.Stderr, "· determining wiki structure...")
		case deepwiki.TaskGenerating:
			if pageID != "" {
				fmt.Fprintf(os.Stderr, "· generated page %d/%d (%s)\n", done, total, pageID)
			}
		case deepwiki.TaskCompleted:
			fmt.Fprintf(os.Stderr, "· done (%d pages)\n", total)
		}
	})
	if err != nil {
		return nil, err
	}

	if data, err := json.MarshalIndent(wiki, "", "  "); err == nil {
		if err := storage.DeepwikiSave(repoPath, ref, data); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to cache wiki: %v\n", err)
		}
	}
	return wiki, nil
}

// DefaultServer builds a dwServer with default provider settings (opencode,
// comprehensive) for the repository in the current directory. It returns an
// error when not inside a git repository, so callers (e.g. `lrc ui`) can
// simply skip mounting the deepwiki routes.
func DefaultServer() (*dwServer, error) {
	repoPath, err := resolveRepo()
	if err != nil {
		return nil, err
	}
	llm := network.NewDeepwikiLLM("opencode", "opencode-go/deepseek-v4-flash", "", "")
	return NewServer(repoPath, llm, "opencode", "opencode-go/deepseek-v4-flash", true), nil
}
