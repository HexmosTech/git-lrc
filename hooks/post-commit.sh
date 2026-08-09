__LRC_MARKER_BEGIN__
# lrc_version: __LRC_VERSION__
# This section is managed by LiveReview CLI (lrc)
# Manual changes within markers will be lost on hook updates

GIT_DIR="$(git rev-parse --git-dir 2>/dev/null || echo .git)"
PUSH_FLAG="$GIT_DIR/__LRC_PUSH_REQUEST_FILE__"
LRC_DIR="$GIT_DIR/lrc"
ATTEST_DIR="$LRC_DIR/attestations"
DISABLED_FILE="$LRC_DIR/disabled"
DISABLED_GIT_FILE="$LRC_DIR/disabled-git"
UPSTREAM=""
UPSTREAM_REMOTE=""
UPSTREAM_BRANCH=""

if [ -f "$DISABLED_FILE" ] || [ -f "$DISABLED_GIT_FILE" ]; then
	exit 0
fi

# Skip during Git sequencer operations to avoid re-triggering on rebase/merge/cherry-pick
if [ -d "$GIT_DIR/rebase-apply" ] || [ -d "$GIT_DIR/rebase-merge" ] || [ -f "$GIT_DIR/MERGE_HEAD" ] || [ -f "$GIT_DIR/CHERRY_PICK_HEAD" ]; then
	echo "LiveReview: skipping during rebase/merge/cherry-pick" >&2
	exit 0
fi

cleanup_flag() {
	rm -f "$PUSH_FLAG" 2>/dev/null || true
}

cleanup_attestation() {
	TREE_HASH="$(git rev-parse --verify HEAD^{tree} 2>/dev/null || true)"
	if [ -n "$TREE_HASH" ] && [ -f "$ATTEST_DIR/$TREE_HASH.json" ]; then
		rm -f "$ATTEST_DIR/$TREE_HASH.json" 2>/dev/null || true
		echo "lrc: cleared attestation for committed tree $TREE_HASH"
	fi
}

# Always clear attestation for the committed tree
cleanup_attestation

# Clean up review session DB for this branch (best-effort)
if command -v lrc >/dev/null 2>&1; then
	lrc review-cleanup 2>/dev/null || true
fi

# Queue this commit for offline-first sync to LiveReview's backend, if it
# corresponds to a review that was actually submitted (best-effort, never
# blocks or fails the commit -- the commit already happened).
if command -v lrc >/dev/null 2>&1; then
	lrc internal sync enqueue >/dev/null 2>&1 || true
fi

# If push was not requested, we're done
if [ ! -f "$PUSH_FLAG" ]; then
	exit 0
fi

echo "lrc: commit-and-push requested; verifying state and pushing if safe"

# 1. Abort if HEAD is detached
if ! git symbolic-ref -q HEAD >/dev/null; then
	echo "lrc: push skipped – detached HEAD"
	cleanup_flag
	exit 0
fi

# 2. Bootstrap upstream on first push
if ! git rev-parse --abbrev-ref --symbolic-full-name @{u} >/dev/null 2>&1; then
	if ! BRANCH_NAME=$(git rev-parse --abbrev-ref HEAD 2>/dev/null); then
		echo "lrc: push failed – unable to resolve current branch for upstream bootstrap"
		cleanup_flag
		exit 1
	fi
	if [ -z "$BRANCH_NAME" ] || [ "$BRANCH_NAME" = "HEAD" ]; then
		echo "lrc: push failed – unable to resolve a valid branch name for upstream bootstrap"
		cleanup_flag
		exit 1
	fi
	echo "lrc: no upstream configured for $BRANCH_NAME; creating origin upstream"
	if ! git push -u origin HEAD; then
		echo "lrc: push failed – could not create upstream on origin"
		cleanup_flag
		exit 1
	fi
	echo "lrc: push complete -> origin/$BRANCH_NAME"
	cleanup_flag
	exit 0
fi

UPSTREAM=$(git rev-parse --abbrev-ref --symbolic-full-name @{u} 2>/dev/null)
UPSTREAM_REMOTE=${UPSTREAM%%/*}
UPSTREAM_BRANCH=${UPSTREAM#*/}
if [ -z "$UPSTREAM_REMOTE" ] || [ -z "$UPSTREAM_BRANCH" ]; then
	echo "lrc: push skipped – unable to resolve upstream"
	cleanup_flag
	exit 0
fi
echo "lrc: upstream detected -> $UPSTREAM_REMOTE/$UPSTREAM_BRANCH"

# 3. Fetch upstream
echo "lrc: fetching $UPSTREAM_REMOTE"
if ! git fetch --prune "$UPSTREAM_REMOTE"; then
	echo "lrc: push skipped – fetch failed"
	cleanup_flag
	exit 0
fi

# 4. Fast-forward only
echo "lrc: attempting fast-forward merge"
if ! git merge --ff-only @{u}; then
	echo "lrc: push skipped – fast-forward merge failed (remote has diverged)"
	cleanup_flag
	exit 0
fi
echo "lrc: fast-forwarded to $UPSTREAM"

# 5. Push
echo "lrc: pushing to $UPSTREAM_REMOTE/$UPSTREAM_BRANCH"
if ! git push "$UPSTREAM_REMOTE" HEAD:"$UPSTREAM_BRANCH"; then
	echo "lrc: push failed"
	cleanup_flag
	exit 0
fi
echo "lrc: push complete -> $UPSTREAM_REMOTE/$UPSTREAM_BRANCH"
cleanup_flag
exit 0
__LRC_MARKER_END__
