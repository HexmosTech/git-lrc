__LRC_MARKER_BEGIN__
# lrc_version: __LRC_VERSION__
# This section is managed by LiveReview CLI (lrc)
# Manual changes within markers will be lost on hook updates

COMMIT_MSG_FILE="$1"
COMMIT_SOURCE="${2:-}"
SKIP_REVIEW="${LRC_SKIP_REVIEW:-}" 
GIT_DIR="$(git rev-parse --git-dir 2>/dev/null || echo .git)"
COMMIT_MSG_OVERRIDE="$GIT_DIR/__LRC_COMMIT_MESSAGE_FILE__"
EDITOR_OVERRIDE_STATE="$GIT_DIR/livereview_editor_override"
LRC_DIR="$GIT_DIR/lrc"
ATTEST_DIR="$LRC_DIR/attestations"
DISABLED_FILE="$LRC_DIR/disabled"
DISABLED_GIT_FILE="$LRC_DIR/disabled-git"
STATE_FILE="$GIT_DIR/livereview_state"
LOCK_DIR="$GIT_DIR/livereview_state.lock"
INITIAL_MSG_FILE="$GIT_DIR/livereview_initial_message.$$"

rm -f "$EDITOR_OVERRIDE_STATE" 2>/dev/null || true

apply_commit_message_override() {
	if [ -n "$COMMIT_MSG_FILE" ] && [ -f "$COMMIT_MSG_OVERRIDE" ] && [ -s "$COMMIT_MSG_OVERRIDE" ]; then
		cat "$COMMIT_MSG_OVERRIDE" > "$COMMIT_MSG_FILE" 2>/dev/null || true
	fi
}

enable_noop_editor_for_prefilled_commit_message() {
	if [ -n "$COMMIT_SOURCE" ]; then
		return
	fi
	if [ -z "$COMMIT_MSG_FILE" ] || [ ! -f "$COMMIT_MSG_FILE" ] || [ ! -s "$COMMIT_MSG_FILE" ]; then
		return
	fi
	printf 'prefilled\n' > "$EDITOR_OVERRIDE_STATE"
}

prepare_prefilled_commit_message() {
	apply_commit_message_override
	enable_noop_editor_for_prefilled_commit_message
}

if [ -f "$DISABLED_FILE" ] || [ -f "$DISABLED_GIT_FILE" ]; then
	exit 0
fi

# Skip during Git sequencer operations to avoid re-triggering on rebase/merge/cherry-pick
if [ -d "$GIT_DIR/rebase-apply" ] || [ -d "$GIT_DIR/rebase-merge" ] || [ -f "$GIT_DIR/MERGE_HEAD" ] || [ -f "$GIT_DIR/CHERRY_PICK_HEAD" ]; then
	echo "LiveReview: skipping during rebase/merge/cherry-pick" >&2
	exit 0
fi

# Allow explicit bypass (analogous to --no-verify)
if [ "$LRC_SKIP_REVIEW" = "1" ]; then
	exit 0
fi

# Claude Code local hook already ran the blocking review gate; avoid re-running it here.
if [ "${LRC_CLAUDE_REVIEW_HANDLED:-}" = "1" ]; then
	prepare_prefilled_commit_message
	echo "LiveReview: Claude-managed review already handled; prepare-commit-msg no-op" >&2
	exit 0
fi

# Detect if running in TTY (check stdout, not stdin - Git redirects stdin)
if [ -t 1 ]; then
	LRC_INTERACTIVE=1
else
	LRC_INTERACTIVE=0
fi

# Non-interactive: require attestation for current staged tree; do not run review here
if [ "$LRC_INTERACTIVE" = "0" ]; then
	TREE_HASH="$(git write-tree 2>/dev/null || true)"
	ATTEST_FILE="$ATTEST_DIR/$TREE_HASH.json"
	if [ -z "$TREE_HASH" ]; then
		echo "LiveReview: failed to compute staged tree hash. Run 'lrc review --staged' and retry commit." >&2
		exit 1
	fi
	if [ ! -f "$ATTEST_FILE" ]; then
		echo "LiveReview: review attestation missing for staged changes. Run 'lrc review --staged' (or --skip/--vouch) and retry commit." >&2
		exit 1
	fi
	prepare_prefilled_commit_message
	echo "LiveReview prepare-commit-msg: attestation present for $TREE_HASH; proceeding" >&2
	exit 0
fi

# Cleanup function
cleanup_lock() {
	rmdir "$LOCK_DIR" 2>/dev/null || true
}

# Set up cleanup trap
trap cleanup_lock EXIT INT TERM

# Check if attestation already exists for current tree (e.g., from prior lrc review)
TREE_HASH="$(git write-tree 2>/dev/null || true)"
ATTEST_FILE="$ATTEST_DIR/$TREE_HASH.json"
if [ -n "$TREE_HASH" ] && [ -f "$ATTEST_FILE" ]; then
	prepare_prefilled_commit_message
	echo "LiveReview: attestation already present for $TREE_HASH; proceeding with commit" >&2
	echo "ran:$$:$(date +%s)" > "${STATE_FILE}.tmp"
	mv "${STATE_FILE}.tmp" "$STATE_FILE" 2>/dev/null || true
	exit 0
fi

# Legacy env var check (kept for compatibility)
if [ "$SKIP_REVIEW" = "1" ]; then
	echo "LiveReview: skipping review (SKIP_REVIEW=1)" >&2
	echo "skipped_env:$$:$(date +%s)" > "${STATE_FILE}.tmp"
	mv "${STATE_FILE}.tmp" "$STATE_FILE" 2>/dev/null || true
	exit 0
fi

# Acquire lock with timeout (5 minutes)
MAX_WAIT=300
WAITED=0

# Check for stale locks (>5 minutes old)
if [ -d "$LOCK_DIR" ]; then
	if command -v stat >/dev/null 2>&1; then
		LOCK_AGE=$(($(date +%s) - $(stat -c %Y "$LOCK_DIR" 2>/dev/null || stat -f %m "$LOCK_DIR" 2>/dev/null || echo 0)))
		if [ "$LOCK_AGE" -gt 300 ]; then
			echo "Removing stale lock (${LOCK_AGE}s old)" >&2
			rmdir "$LOCK_DIR" 2>/dev/null || true
		fi
	fi
fi

while ! mkdir "$LOCK_DIR" 2>/dev/null; do
	if [ $WAITED -ge $MAX_WAIT ]; then
		echo "Could not acquire LiveReview lock after ${MAX_WAIT}s, skipping review" >&2
		echo "skipped_lock:$$:$(date +%s)" > "${STATE_FILE}.tmp"
		mv "${STATE_FILE}.tmp" "$STATE_FILE" 2>/dev/null || true
		exit 0
	fi
	sleep 1
	WAITED=$((WAITED + 1))
done

# Capture current commit message (available in prepare-commit-msg)
if [ -n "$COMMIT_MSG_FILE" ] && [ -f "$COMMIT_MSG_FILE" ]; then
	cat "$COMMIT_MSG_FILE" > "$INITIAL_MSG_FILE" 2>/dev/null || true
fi

# Run review
if [ "$LRC_INTERACTIVE" = "1" ]; then
	echo "Running LiveReview commit check..." >&2
	LRC_ACTIVE_COMMIT_MSG_FILE="$COMMIT_MSG_FILE" LRC_INITIAL_MESSAGE_FILE="$INITIAL_MSG_FILE" lrc review --staged --precommit
	REVIEW_EXIT=$?
else
	LRC_ACTIVE_COMMIT_MSG_FILE="$COMMIT_MSG_FILE" LRC_INITIAL_MESSAGE_FILE="$INITIAL_MSG_FILE" lrc review --staged --output json >/dev/null 2>&1
	REVIEW_EXIT=$?
fi

# Cleanup initial message file
rm -f "$INITIAL_MSG_FILE"

# Check exit code
if [ $REVIEW_EXIT -eq 0 ]; then
	prepare_prefilled_commit_message
	echo "ran:$$:$(date +%s)" > "${STATE_FILE}.tmp"
	mv "${STATE_FILE}.tmp" "$STATE_FILE" 2>/dev/null || true
	exit 0
elif [ $REVIEW_EXIT -eq 2 ]; then
	echo "skipped_manual:$$:$(date +%s)" > "${STATE_FILE}.tmp"
	mv "${STATE_FILE}.tmp" "$STATE_FILE" 2>/dev/null || true
	exit 0
else
	echo "skipped:$$:$(date +%s)" > "${STATE_FILE}.tmp"
	mv "${STATE_FILE}.tmp" "$STATE_FILE" 2>/dev/null || true
	exit 1
fi
__LRC_MARKER_END__
