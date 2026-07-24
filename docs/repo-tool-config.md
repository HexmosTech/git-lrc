# Repository-Level Tool Configuration (`.lrc/policy/tools.toml`)

`git-lrc` supports repository-specific tool configurations via a single file: `.lrc/policy/tools.toml`. Each static-analysis tool (`gitleaks`, `ruff`, `bandit`, `detect-secrets`, etc.) is configured as a TOML table (object) in this file.

---

## Configuration File Format

Location: `.lrc/policy/tools.toml`

```toml
# .lrc/policy/tools.toml

[gitleaks]
enabled = true
category = "secret-scanning"
include = ["*"]
# exclude = ["tests/fixtures/**", "*.md"]

[ruff]
enabled = true
category = "python-sast"
include = ["**/*.py"]
exclude = ["tests/**"]

[golangci-lint]
enabled = false
category = "go-sast"
```

Each `[tool-name]` section header is the tool name (lowercase). Fields:

| Field | Type | Description |
|---|---|---|
| `enabled` | bool | Enable (`true`) or disable (`false`) this tool for reviews |
| `category` | string | Classification e.g. `secret-scanning`, `python-sast`, `go-sast` |
| `include` | string[] | Gitignore-style globs — tool only runs when a diff file matches |
| `exclude` | string[] | Gitignore-style globs — matching diff files are excluded from this tool |

If `include` is omitted, all files are considered. `exclude` takes priority over `include`.

---

## Scaffolding with `lrc config init`

Running `lrc config init` inside your repository scaffolds `.lrc/policy/tools.toml` with a ready-to-go `[gitleaks]` configuration:

```bash
lrc config init
```

Output:
```
Created:
  .lrc/README.md
  .lrc/ignore
  .lrc/rules/INSTRUCTIONS.md
  .lrc/rules/design.md
  .lrc/rules/security.md
  .lrc/rules/style.md
  .lrc/policy/tools.toml
```

---

## How It Works During Reviews

When running `lrc r` (or `lrc r --tools`):

1. `git-lrc` bundles `.lrc/policy/tools.toml` into the review payload ZIP archive sent to the LiveReview backend (`POST /api/v1/diff-review`).
2. The LiveReview backend parses the single `policy/tools.toml` file, reading each tool's table.
3. **Tool Activation**: Tools set to `enabled = true` run for the review even if disabled in organization settings.
4. **Per-Tool Diff Filtering**: Each tool evaluates its `include` and `exclude` gitignore-style glob patterns against the changed files in the review diff. If all changed files are excluded for a tool, the backend skips invoking the Lambda for that tool to save credits.
5. **Per-Tool Diff Slicing**: Only the diff content for matching files is sent to each tool's Lambda — other files are stripped from the payload.
