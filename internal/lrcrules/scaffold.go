package lrcrules

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/HexmosTech/git-lrc/storage"
)

const rootReadmeContent = `# .lrc/ — Repository Rules

This directory teaches the LiveReview AI reviewer about this repository:
its conventions, what's intentionally off-limits, and which files it
shouldn't review at all.

## Layout

    .lrc/
    ├── README.md            — this file
    ├── ignore               — gitignore-style exclude patterns
    ├── rules/
    │   ├── INSTRUCTIONS.md  — entry point, sent to the reviewer first
    │   ├── design.md
    │   ├── security.md
    │   └── style.md         — and any other *.md files you add
    └── policy/
        └── tools.toml       — not yet enforced

## rules/

Every ` + "`*.md`" + ` file in ` + "`rules/`" + ` is concatenated into a single
instruction bundle for the AI reviewer, each preceded by a
` + "`## rules/<file>.md`" + ` header. Empty or whitespace-only files are
skipped entirely.

` + "`rules/INSTRUCTIONS.md`" + `, if present and non-empty, is always placed
first — use it as the entry point for the most important guidance. Every
other ` + "`*.md`" + ` file follows after it, in lexicographic order.

The combined bundle is limited to ` + "`CharLimit`" + ` (%d characters);
anything over the limit is truncated. Run ` + "`lrc config check`" + ` to
verify you're within the limit and ` + "`lrc config preview`" + ` to see the
exact bundle that will be sent.

Keep it short: capture the handful of ideas that repeatedly affect review
decisions (e.g. "prefer direct SQL over ORM abstractions", "avoid new
infrastructure dependencies").

## ignore

` + "`.lrc/ignore`" + ` uses gitignore syntax (comments with ` + "`#`" + `,
blank lines, ` + "`**`" + `, negation with ` + "`!`" + `, etc.) to list paths
that should be excluded from AI review entirely, matched against each
changed file's path relative to the repository root. Excluded files are not
sent to the reviewer and don't count toward billable lines of code.

## policy/

Machine-readable settings consumed directly by git-lrc. Not yet enforced.

## Commands

- ` + "`lrc config init`" + `    — scaffold this directory (idempotent)
- ` + "`lrc config check`" + `   — validate structure, ignore syntax, and rules
  bundle size, entirely offline
- ` + "`lrc config preview`" + ` — show the exact instruction bundle that will
  be sent to the reviewer
`

const ignoreContent = `# .lrc/ignore — gitignore-style patterns
#
# Paths are matched relative to the repository root, using the same syntax
# as .gitignore (comments, blank lines, "**", negation with "!", etc.).
# Files matching a pattern here are excluded from AI review.
`

const policyToolsTomlContent = `# .lrc/policy/tools.toml — Repository Tool Configuration
# Configure static-analysis security & quality tools for this repository.
# Tools configured here will run during code reviews even if disabled in Organization settings.

[gitleaks]
enabled = true
category = "secret-scanning"
include = ["*"]
# exclude = ["tests/fixtures/**", "*.md"]

# [ruff]
# enabled = true
# category = "python-sast"
# include = ["**/*.py"]

# [bandit]
# enabled = true
# category = "python-sast"
`

// scaffoldFile describes one file Init may create.
type scaffoldFile struct {
	relPath string // relative to .lrc/
	content string
}

func scaffoldFiles() []scaffoldFile {
	return []scaffoldFile{
		{"README.md", fmt.Sprintf(rootReadmeContent, CharLimit)},
		{"ignore", ignoreContent},
		{"rules/INSTRUCTIONS.md", ""},
		{"rules/design.md", ""},
		{"rules/security.md", ""},
		{"rules/style.md", ""},
		{"policy/tools.toml", policyToolsTomlContent},
	}
}

// Init scaffolds .lrc/ under repoRoot idempotently: existing files and
// directories are left untouched. Returns the list of paths (relative to
// repoRoot, using "/" separators) that were created.
func Init(repoRoot string) ([]string, error) {
	lrcDir := filepath.Join(repoRoot, ".lrc")
	var created []string

	// Pre-create .lrc/ and its subdirectories with 0o755 so they're
	// readable/listable by others, matching the 0o644 files written into
	// them below. storage.WriteFileAtomically always creates missing parent
	// directories with 0o700; pre-creating them here makes that a no-op.
	for _, dir := range []string{lrcDir, filepath.Join(lrcDir, "rules"), filepath.Join(lrcDir, "policy")} {
		if err := storage.MkdirAll(dir, 0o755); err != nil {
			return created, err
		}
	}

	for _, f := range scaffoldFiles() {
		fullPath := filepath.Join(lrcDir, f.relPath)
		if _, err := os.Stat(fullPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return created, err
		}

		if err := storage.WriteFileAtomically(fullPath, []byte(f.content), 0o644); err != nil {
			return created, err
		}
		created = append(created, filepath.ToSlash(filepath.Join(".lrc", f.relPath)))
	}

	return created, nil
}
