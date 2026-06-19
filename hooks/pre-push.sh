__LRC_MARKER_BEGIN__
# lrc_version: __LRC_VERSION__
# This section is managed by LiveReview CLI (lrc)
# Manual changes within markers will be lost on hook updates

REMOTE_NAME="$1"

GIT_DIR="$(git rev-parse --git-dir 2>/dev/null || echo .git)"
LRC_DIR="$GIT_DIR/lrc"
DISABLED_FILE="$LRC_DIR/disabled"
DISABLED_GIT_FILE="$LRC_DIR/disabled-git"
PUSH_ENABLED_FILE="$LRC_DIR/push-enabled"
ZERO_SHA="0000000000000000000000000000000000000000"

if [ -f "$DISABLED_FILE" ] || [ -f "$DISABLED_GIT_FILE" ]; then
	exit 0
fi

# Push-mode review is opt-in: no-op unless explicitly enabled for this repo.
if [ ! -f "$PUSH_ENABLED_FILE" ]; then
	exit 0
fi

# Detect interactive terminal (stdout check; git redirects stdin to the ref list below).
if [ -t 1 ]; then
	LRC_INTERACTIVE=1
else
	LRC_INTERACTIVE=0
fi

resolve_default_branch_base() {
	local_sha="$1"
	default_ref="$(git symbolic-ref --quiet --short "refs/remotes/$REMOTE_NAME/HEAD" 2>/dev/null)"
	if [ -z "$default_ref" ]; then
		for cand in main master; do
			if git show-ref --verify --quiet "refs/remotes/$REMOTE_NAME/$cand"; then
				default_ref="$REMOTE_NAME/$cand"
				break
			fi
		done
	fi
	if [ -n "$default_ref" ]; then
		git merge-base "$default_ref" "$local_sha" 2>/dev/null || true
	fi
}

# stdin carries one line per ref being pushed: "<local-ref> <local-sha> <remote-ref> <remote-sha>"
while read -r local_ref local_sha remote_ref remote_sha; do
	case "$local_ref" in
	refs/heads/*) : ;;
	*) continue ;;
	esac

	if [ "$local_sha" = "$ZERO_SHA" ]; then
		continue
	fi

	if [ "$remote_sha" = "$ZERO_SHA" ]; then
		base_sha="$(resolve_default_branch_base "$local_sha")"
		if [ -z "$base_sha" ]; then
			base_sha="$(git rev-list --max-parents=0 "$local_sha" 2>/dev/null | tail -1)"
		fi
	else
		base_sha="$remote_sha"
	fi

	if [ -z "$base_sha" ] || [ "$base_sha" = "$local_sha" ]; then
		continue
	fi

	ATTEST_FILE="$LRC_DIR/push_attestations/$local_sha.json"
	if [ -f "$ATTEST_FILE" ]; then
		echo "LiveReview pre-push: attestation present for $local_sha; proceeding" >&2
		continue
	fi

	if [ "$LRC_INTERACTIVE" = "0" ]; then
		echo "LiveReview: push review attestation missing for $local_ref. Run 'lrc review --push-range $base_sha..$local_sha' (or --skip/--vouch) interactively and retry push." >&2
		exit 1
	fi

	echo "Running LiveReview push check for $local_ref ($base_sha..$local_sha)..." >&2
	lrc review --push-range "$base_sha..$local_sha" --precommit < /dev/tty
	REVIEW_EXIT=$?

	# Exit codes mirror the commit-time decision flow: 0=approved, 2=skip/vouch
	# (still proceeds, just without/with manual AI review), anything else aborts.
	if [ $REVIEW_EXIT -eq 0 ] || [ $REVIEW_EXIT -eq 2 ]; then
		continue
	fi

	echo "LiveReview: push review for $local_ref was not approved; aborting push." >&2
	exit 1
done

exit 0
__LRC_MARKER_END__
