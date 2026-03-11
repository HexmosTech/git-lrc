# lrc - LiveReview CLI

`lrc` is a command-line tool for submitting local code diffs to LiveReview for AI-powered code review.

## Installation

Build the binary:

```bash
make lrc
```

Or build directly:

```bash
go build -o lrc ./cmd/lrc
```

## Usage

### Basic Usage

Review staged changes:

```bash
lrc --api-key YOUR_API_KEY
```

### Git Hook Installation

Install pre-commit hooks for automatic code review:

```bash
# Install hooks
lrc install-hooks

# Uninstall hooks
lrc uninstall-hooks

# Force update existing lrc hooks
lrc install-hooks --force
```

The hooks will:
- Run `lrc review --staged` before each commit
- Never block commits (always exits 0)
- Can be skipped with Ctrl-C
- Can be bypassed with `git commit --no-verify`
- Add a commit trailer: `LiveReview Pre-Commit Check: [ran|skipped]`

### Diff Sources

- **Staged changes** (default):
  ```bash
  lrc --api-key YOUR_API_KEY --diff-source staged
  ```

- **Working tree changes**:
  ```bash
  lrc --api-key YOUR_API_KEY --diff-source working
  ```

- **Git range**:
  ```bash
  lrc --api-key YOUR_API_KEY --diff-source range --range HEAD~1..HEAD
  ```

- **From file**:
  ```bash
  lrc --api-key YOUR_API_KEY --diff-source file --diff-file my-changes.diff
  ```

### Configuration

You can provide configuration in three ways (in order of precedence):

1. **Command-line flags**: `--api-key YOUR_API_KEY --api-url https://your-instance.com`
2. **Environment variables**: `export LRC_API_KEY="your-api-key" LRC_API_URL="https://your-instance.com"`
3. **Config file**: Create `~/.lrc.toml` with:
   ```toml
   # Your LiveReview API key
   api_key = "lr_example_token"
   
   # Your LiveReview API endpoint (base URL, without /api suffix)
   # The CLI automatically appends /api/v1/diff-review
   api_url = "https://manual-talent.apps.hexmos.com"
   ```

#### Setting Up Your Config File

```bash
# Copy the sample config
cp cmd/lrc/.lrc.toml.sample ~/.lrc.toml

# Edit with your credentials
vim ~/.lrc.toml

# Protect your API key
chmod 600 ~/.lrc.toml

# Now you can run lrc without any flags
lrc
```

All other flags can be set via environment variables or command-line flags.

### Flags

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--repo-name` | `LRC_REPO_NAME` | current dir basename | Repository name |
| `--diff-source` | `LRC_DIFF_SOURCE` | `staged` | Diff source: `staged`, `working`, `range`, or `file` |
| `--range` | `LRC_RANGE` | | Git range (e.g., `HEAD~1..HEAD`) for `range` mode |
| `--diff-file` | `LRC_DIFF_FILE` | | Path to diff file for `file` mode |
| `--api-url` | `LRC_API_URL` | `http://localhost:8888` | LiveReview API base URL |
| `--api-key` | `LRC_API_KEY` | (from config) | API key for authentication |
| `--poll-interval` | `LRC_POLL_INTERVAL` | `2s` | Interval between status polls |
| `--timeout` | `LRC_TIMEOUT` | `5m` | Maximum wait time for review |
| `--output` | `LRC_OUTPUT` | `pretty` | Output format: `pretty` or `json` |
| `--save-bundle` | `LRC_SAVE_BUNDLE` | | Save bundle to file for inspection before sending |
| `--save-json` | `LRC_SAVE_JSON` | | Save JSON response to file after completion |
| `--save-text` | `LRC_SAVE_TEXT` | | Save formatted text with comment markers to file |
| `--save-html` | `LRC_SAVE_HTML` | | Save GitHub-style HTML review to file |
| `--verbose, -v` | `LRC_VERBOSE` | `false` | Enable verbose output |

## Examples

### Setup API key in config file

```bash
# Create config file with your API key and endpoint
cat > ~/.lrc.toml << EOF
# Your LiveReview API key
api_key = "lr_example_token"

# Your LiveReview API endpoint
api_url = "https://manual-talent.apps.hexmos.com"
EOF

# Protect your config file
chmod 600 ~/.lrc.toml

# Now you can run lrc without specifying the key or URL
lrc
```

### Inspect bundle before sending

```bash
# Save the bundle for inspection before submitting
lrc --save-bundle bundle.txt

# Review the bundle file to see what will be sent
less bundle.txt
```

### Save review results to files

```bash
# Save both JSON and formatted text output
lrc --save-json review.json --save-text review.txt

# Save HTML output for viewing in browser
lrc --save-html review.html

# Save all formats
lrc --save-json review.json --save-text review.txt --save-html review.html

# Search for comments in the text file
grep -n ">>>COMMENT<<<" review.txt

# Or use your editor's search function to jump between comments
vim review.txt  # then search for: />>>COMMENT<<<

# Open HTML review in browser
xdg-open review.html  # Linux
open review.html      # macOS
start review.html     # Windows
```

### Complete workflow with all output options

```bash
# Run review with all inspection/output options
lrc \
  --save-bundle bundle.txt \
  --save-json review.json \
  --save-text review.txt \
  --save-html review.html \
  --verbose

# Review the bundle that was sent
cat bundle.txt

# Check the raw JSON response
jq . review.json

# Navigate through comments in text file
# Search for ">>>COMMENT<<<" to jump between comments
vim review.txt

# Open HTML review in browser (best for visual inspection)
xdg-open review.html
```

### Review uncommitted changes

```bash
# Review all staged changes
git add .
lrc --api-key YOUR_API_KEY

# Review working directory changes (unstaged)
lrc --api-key YOUR_API_KEY --diff-source working
```

### Review a specific commit

```bash
lrc --api-key YOUR_API_KEY --diff-source range --range HEAD~1..HEAD
```

### Review a saved diff

```bash
git diff main..feature-branch > changes.diff
lrc --api-key YOUR_API_KEY --diff-source file --diff-file changes.diff
```

### JSON output for scripting

```bash
lrc --api-key YOUR_API_KEY --output json > review-results.json
```

### Verbose mode for debugging

```bash
lrc --api-key YOUR_API_KEY --verbose
```

## Output Formats

### Pretty (default)

Human-readable output with file sections and colored severity levels:

```
================================================================================
LIVEREVIEW RESULTS
================================================================================

Summary:
The code looks good overall. Minor suggestions below.

2 file(s) with comments:

--------------------------------------------------------------------------------
FILE: src/main.go
--------------------------------------------------------------------------------

  [WARNING] Line 42 (best-practices)
    Consider using context.WithTimeout instead of time.Sleep

  [INFO] Line 89 (style)
    Variable name could be more descriptive

================================================================================
Review complete: 2 total comment(s)
================================================================================
```

### JSON

Machine-readable JSON output for automation:

```json
{
  "status": "completed",
  "summary": "The code looks good overall. Minor suggestions below.",
  "files": [
    {
      "file_path": "src/main.go",
      "hunks": [...],
      "comments": [
        {
          "line": 42,
          "content": "Consider using context.WithTimeout instead of time.Sleep",
          "severity": "warning",
          "category": "best-practices"
        }
      ]
    }
  ]
}
```

## API Requirements

The `lrc` tool requires:

1. A running LiveReview API server (default: `http://localhost:8888`)
2. A valid API key (obtain from your LiveReview account settings)

The tool communicates with these endpoints:

- `POST /api/v1/diff-review` - Submit diff for review
- `GET /api/v1/diff-review/:id` - Poll for review status/results

## Exit Codes

- `0` - Success
- `1` - Error (network failure, invalid input, review failed, timeout, etc.)

## Troubleshooting

### "API key not provided"

You can provide the API key in three ways:

```bash
# Option 1: Config file (recommended)
echo 'api_key = "your-key"' > ~/.lrc.toml

# Option 2: Environment variable
export LRC_API_KEY="your-key"

# Option 3: Command-line flag
lrc --api-key your-key
```

### Inspecting what gets sent to the API

Use `--save-bundle` to see exactly what will be transmitted:

```bash
lrc --save-bundle bundle.txt --verbose

# The bundle file contains:
# - Original diff content
# - Zip archive info
# - Base64 encoded payload (what the API receives)
```

### Analyzing review comments

Use `--save-text` to get a searchable text file with markers:

```bash
lrc --save-text review.txt

# Search for comments:
grep ">>>COMMENT<<<" review.txt

# Or use editor search to jump between comments:
vim review.txt  # then: />>>COMMENT<<<
code review.txt  # then: Ctrl+F ">>>COMMENT<<<"
```

### "no diff content collected"

Make sure you have uncommitted changes:

```bash
git status
git diff --staged  # Check if there are staged changes
```

### "API returned status 401"

Check your API key:

```bash
echo $LRC_API_KEY
# or
lrc --api-key YOUR_API_KEY --verbose
```

### "timeout waiting for review completion"

Increase the timeout:

```bash
lrc --api-key YOUR_API_KEY --timeout 10m
```

### Connection refused

Ensure the LiveReview API server is running:

```bash
# Start the API server
./livereview api

# Or check if it's already running
curl http://localhost:8888/health
```
