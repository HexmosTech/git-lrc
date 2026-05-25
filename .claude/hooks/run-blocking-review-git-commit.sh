#!/usr/bin/env bash
set -euo pipefail

project_dir="${LRC_CLAUDE_PROJECT_DIR:-$PWD}"
original_command="${LRC_ORIGINAL_GIT_COMMIT:-}"
blocking_timeout="${LRC_BLOCKING_REVIEW_TIMEOUT:-20m}"

if [[ -z "$original_command" ]]; then
  echo "LiveReview: missing original git commit command for Claude wrapper" >&2
  exit 1
fi

if ! command -v lrc >/dev/null 2>&1; then
  echo "LiveReview: lrc is not available on PATH, so the blocking review gate cannot run" >&2
  exit 1
fi

lrc_bin="$(command -v lrc)"

lrc_review_mode="$($lrc_bin version 2>/dev/null | awk -F': ' '/Review mode/ {print $2; exit}')"

if [[ -z "$lrc_review_mode" ]]; then
  echo "LiveReview: unable to determine lrc review mode from $lrc_bin" >&2
  echo "LiveReview: rebuild the CLI with 'make build-local && lrc hooks install' before retrying git commit" >&2
  exit 1
fi

if [[ "$lrc_review_mode" == "fake" ]]; then
  echo "LiveReview: refusing to use fake-review lrc binary at $lrc_bin" >&2
  echo "LiveReview: rebuild the real CLI with 'make build-local && lrc hooks install' before retrying git commit" >&2
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "LiveReview: python3 is required for the local Claude blocking-review helper" >&2
  exit 1
fi

if ! initial_message=$(LRC_ORIGINAL_GIT_COMMIT="$original_command" python3 - <<'PY'
import os
import shlex

command = os.environ.get("LRC_ORIGINAL_GIT_COMMIT", "")

try:
    tokens = shlex.split(command, posix=True)
except ValueError:
    print("", end="")
    raise SystemExit(0)

message = ""
i = 0
while i < len(tokens):
    token = tokens[i]
    if token in ("-m", "--message") and i + 1 < len(tokens):
        message = tokens[i + 1]
        break
    if token.startswith("--message="):
        message = token.split("=", 1)[1]
        break
    if token.startswith("-m") and token != "-m" and len(token) > 2:
        message = token[2:]
        break
    i += 1

print(message, end="")
PY
); then
  echo "LiveReview: failed to parse the original git commit command" >&2
  exit 1
fi

cd "$project_dir"

echo "LiveReview: checking whether the current staged tree already has a valid review." >&2
echo "LiveReview: if not, a blocking browser review will open before git commit can continue." >&2

review_log=$(mktemp)
cleanup() {
  rm -f "$review_log"
}
trap cleanup EXIT

set +e
if [[ -n "$initial_message" ]]; then
  LRC_INITIAL_MESSAGE="$initial_message" lrc review --staged --blocking-review --blocking-review-timeout "$blocking_timeout" 2>&1 | tee "$review_log"
  review_status=${PIPESTATUS[0]}
else
  lrc review --staged --blocking-review --blocking-review-timeout "$blocking_timeout" 2>&1 | tee "$review_log"
  review_status=${PIPESTATUS[0]}
fi
set -e

case "$review_status" in
  0|2)
    exec env LRC_CLAUDE_REVIEW_HANDLED=1 bash -c "$original_command"
    ;;
  1)
    if grep -q "attestation already present for current tree" "$review_log"; then
      echo "LiveReview: current tree is already reviewed; proceeding with git commit." >&2
      exec env LRC_CLAUDE_REVIEW_HANDLED=1 bash -c "$original_command"
    fi
    if grep -q "Commit aborted by user" "$review_log"; then
    echo "LiveReview: commit intentionally aborted in the browser; git commit was not run." >&2
    exit 0
    fi
    echo "LiveReview: blocking review exited with code 1 before git commit could continue" >&2
    exit 1
    ;;
  *)
    echo "LiveReview: blocking review failed before git commit could continue" >&2
    exit 1
    ;;
esac