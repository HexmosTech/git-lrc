# .lrc/ — Repository Rules

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

Every `*.md` file in `rules/` is concatenated into a single
instruction bundle for the AI reviewer, each preceded by a
`## rules/<file>.md` header. Empty or whitespace-only files are
skipped entirely.

`rules/INSTRUCTIONS.md`, if present and non-empty, is always placed
first — use it as the entry point for the most important guidance. Every
other `*.md` file follows after it, in lexicographic order.

The combined bundle is limited to `CharLimit` (3000 characters);
anything over the limit is truncated. Run `lrc config check` to
verify you're within the limit and `lrc config preview` to see the
exact bundle that will be sent.

Keep it short: capture the handful of ideas that repeatedly affect review
decisions (e.g. "prefer direct SQL over ORM abstractions", "avoid new
infrastructure dependencies").

## ignore

`.lrc/ignore` uses gitignore syntax (comments with `#`,
blank lines, `**`, negation with `!`, etc.) to list paths
that should be excluded from AI review entirely, matched against each
changed file's path relative to the repository root. Excluded files are not
sent to the reviewer and don't count toward billable lines of code.

## policy/

Machine-readable settings consumed directly by git-lrc. Not yet enforced.

## Commands

- `lrc config init`    — scaffold this directory (idempotent)
- `lrc config check`   — validate structure, ignore syntax, and rules
  bundle size, entirely offline
- `lrc config preview` — show the exact instruction bundle that will
  be sent to the reviewer
