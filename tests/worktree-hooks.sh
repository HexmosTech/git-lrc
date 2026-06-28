#!/bin/bash
set -euo pipefail

PASS=0
FAIL=0

red()   { printf "\033[31m%s\033[0m\n" "$*"; }
green() { printf "\033[32m%s\033[0m\n" "$*"; }
bold()  { printf "\033[1m%s\033[0m\n" "$*"; }

assert_ok() {
	local desc="$1"
	if "$@" >/dev/null 2>&1; then
		green "  ✓ $desc"
		PASS=$((PASS + 1))
	else
		red "  ✗ $desc"
		FAIL=$((FAIL + 1))
	fi
}

assert_contains() {
	local desc="$1" needle="$2" haystack="$3"
	if [[ "$haystack" == *"$needle"* ]]; then
		green "  ✓ $desc"
		PASS=$((PASS + 1))
	else
		red "  ✗ $desc"
		red "    expected to contain: '$needle'"
		red "    got: '$haystack'"
		FAIL=$((FAIL + 1))
	fi
}

assert_file_contains() {
	local desc="$1" needle="$2" file="$3"
	if grep -q "$needle" "$file"; then
		green "  ✓ $desc"
		PASS=$((PASS + 1))
	else
		red "  ✗ $desc"
		red "    expected to find: $needle"
		red "    in file: $file"
		FAIL=$((FAIL + 1))
	fi
}

assert_path_exists() {
	local desc="$1" path="$2"
	if [[ -e "$path" ]]; then
		green "  ✓ $desc"
		PASS=$((PASS + 1))
	else
		red "  ✗ $desc"
		red "    missing path: $path"
		FAIL=$((FAIL + 1))
	fi
}

assert_file_not_contains_or_missing() {
	local desc="$1" needle="$2" file="$3"
	if [[ ! -f "$file" ]]; then
		green "  ✓ $desc"
		PASS=$((PASS + 1))
		return
	fi
	if grep -q "$needle" "$file"; then
		red "  ✗ $desc"
		red "    did not expect to find: $needle"
		red "    in file: $file"
		FAIL=$((FAIL + 1))
	else
		green "  ✓ $desc"
		PASS=$((PASS + 1))
	fi
}

assert_exit_code() {
	local desc="$1" want="$2" actual="$3"
	if [[ "$want" == "$actual" ]]; then
		green "  ✓ $desc"
		PASS=$((PASS + 1))
	else
		red "  ✗ $desc"
		red "    expected exit code: $want"
		red "    actual exit code:   $actual"
		FAIL=$((FAIL + 1))
	fi
}

cleanup() {
	cd /tmp
	if [[ -n "${HOOKS_DIR:-}" ]]; then
		lrc hooks uninstall --path "$HOOKS_DIR" >/dev/null 2>&1 || true
	fi
	if [[ -n "${ORIG_HOOKS_PATH:-}" ]]; then
		git config --global core.hooksPath "$ORIG_HOOKS_PATH" || true
	elif [[ "${HAD_HOOKS_PATH:-no}" == "no" ]]; then
		git config --global --unset core.hooksPath >/dev/null 2>&1 || true
	fi
	rm -rf "${TMP_ROOT:-}" \
		/tmp/lrc-main-block.out \
		/tmp/lrc-main-pass.out \
		/tmp/lrc-wt-skip.out \
		/tmp/lrc-wt-vouch.out \
		/tmp/lrc-wt-disabled.out
}
trap cleanup EXIT

LRC="$(command -v lrc)"
if [[ -z "$LRC" ]]; then
	red "ERROR: lrc not found in PATH. Build and install first."
	exit 1
fi

ORIG_HOOKS_PATH="$(git config --global --get core.hooksPath 2>/dev/null || true)"
if [[ -n "$ORIG_HOOKS_PATH" ]]; then
	HAD_HOOKS_PATH=yes
else
	HAD_HOOKS_PATH=no
fi

TMP_ROOT="$(mktemp -d /tmp/lrc-worktree-test.XXXXXX)"
HOOKS_DIR="$TMP_ROOT/hooks"
REPO_DIR="$TMP_ROOT/main"
WT_SKIP_DIR="$TMP_ROOT/wt-skip"
WT_VOUCH_DIR="$TMP_ROOT/wt-vouch"
LOCAL_REPO_DIR="$TMP_ROOT/local-main"
WT_LOCAL_DIR="$TMP_ROOT/wt-local"

bold "Using lrc: $LRC"
bold "Temp root: $TMP_ROOT"

git config --global --unset core.hooksPath >/dev/null 2>&1 || true

mkdir -p "$REPO_DIR"
cd "$REPO_DIR"
git init --initial-branch=main . >/dev/null
git config user.email test@example.com
git config user.name "Test User"
printf 'a\n' > a.txt
git add a.txt
git commit -m "init" >/dev/null

lrc hooks install --path "$HOOKS_DIR" >/dev/null

bold ""
bold "══ Main Repo Regression ═════════════════════════════════════"

printf 'main-path\n' > main.txt
git add main.txt
set +e
timeout 30s git commit -m "main blocked without attestation" >/tmp/lrc-main-block.out 2>&1
main_block_status=$?
set -e
if [[ $main_block_status -eq 0 ]]; then
	red "  ✗ main repo commit unexpectedly succeeded without attestation"
	cat /tmp/lrc-main-block.out
	FAIL=$((FAIL + 1))
else
	green "  ✓ main repo commit blocked without attestation"
	PASS=$((PASS + 1))
fi
assert_file_contains "main repo shows missing-attestation message" "review attestation missing for staged changes" /tmp/lrc-main-block.out

lrc review --staged --skip >/dev/null
MAIN_TREE="$(git write-tree)"
MAIN_GIT_DIR="$(git rev-parse --git-dir)"
MAIN_ATTEST="$MAIN_GIT_DIR/lrc/attestations/$MAIN_TREE.json"
assert_path_exists "main repo attestation written under git-dir" "$MAIN_ATTEST"

set +e
timeout 30s git commit -m "main skip works" >/tmp/lrc-main-pass.out 2>&1
main_commit_status=$?
set -e
assert_exit_code "main repo commit succeeds after --skip" "0" "$main_commit_status"

bold ""
bold "══ Linked Worktree Skip ═════════════════════════════════════"

git worktree add "$WT_SKIP_DIR" -b feature-skip >/dev/null
cd "$WT_SKIP_DIR"
git config user.email test@example.com
git config user.name "Test User"
printf 'b\n' > b.txt
git add b.txt
lrc review --staged --skip >/dev/null
WT_SKIP_TREE="$(git write-tree)"
WT_SKIP_GIT_DIR="$(git rev-parse --git-dir)"
WT_SKIP_ATTEST="$WT_SKIP_GIT_DIR/lrc/attestations/$WT_SKIP_TREE.json"
assert_path_exists "worktree skip attestation written under per-worktree git-dir" "$WT_SKIP_ATTEST"
assert_file_contains "worktree skip attestation records skipped action" '"action":"skipped"' "$WT_SKIP_ATTEST"

set +e
timeout 30s git commit -m "worktree skip works" >/tmp/lrc-wt-skip.out 2>&1
wt_skip_status=$?
set -e
assert_exit_code "linked worktree commit succeeds after --skip" "0" "$wt_skip_status"

bold ""
bold "══ Linked Worktree Vouch ════════════════════════════════════"

git -C "$REPO_DIR" worktree add "$WT_VOUCH_DIR" -b feature-vouch >/dev/null
cd "$WT_VOUCH_DIR"
git config user.email test@example.com
git config user.name "Test User"
printf 'c\n' > c.txt
git add c.txt
lrc review --staged --vouch >/dev/null
WT_VOUCH_TREE="$(git write-tree)"
WT_VOUCH_GIT_DIR="$(git rev-parse --git-dir)"
WT_VOUCH_ATTEST="$WT_VOUCH_GIT_DIR/lrc/attestations/$WT_VOUCH_TREE.json"
assert_path_exists "worktree vouch attestation written under per-worktree git-dir" "$WT_VOUCH_ATTEST"
assert_file_contains "worktree vouch attestation records vouched action" '"action":"vouched"' "$WT_VOUCH_ATTEST"

set +e
timeout 30s git commit -m "worktree vouch works" >/tmp/lrc-wt-vouch.out 2>&1
wt_vouch_status=$?
set -e
assert_exit_code "linked worktree commit succeeds after --vouch" "0" "$wt_vouch_status"

bold ""
bold "══ Linked Worktree Disable ══════════════════════════════════"

lrc hooks disable >/dev/null
DISABLED_MARKER="$WT_VOUCH_GIT_DIR/lrc/disabled"
assert_path_exists "disabled marker written under per-worktree git-dir" "$DISABLED_MARKER"

printf 'd\n' > d.txt
git add d.txt
set +e
timeout 30s git commit -m "worktree disabled bypass works" >/tmp/lrc-wt-disabled.out 2>&1
wt_disabled_status=$?
set -e
assert_exit_code "linked worktree commit succeeds with hooks disabled" "0" "$wt_disabled_status"

bold ""
bold "══ Local Hook Management In Worktree ════════════════════════"

lrc hooks uninstall --path "$HOOKS_DIR" >/dev/null 2>&1 || true
git config --global --unset core.hooksPath >/dev/null 2>&1 || true

mkdir -p "$LOCAL_REPO_DIR"
cd "$LOCAL_REPO_DIR"
git init --initial-branch=main . >/dev/null
git config user.email test@example.com
git config user.name "Test User"
printf 'root\n' > README.md
git add README.md
git commit -m "init" >/dev/null

git worktree add "$WT_LOCAL_DIR" -b feature-local >/dev/null
cd "$WT_LOCAL_DIR"
git config user.email test@example.com
git config user.name "Test User"

lrc hooks install --local >/dev/null
WT_LOCAL_COMMON_DIR="$(git rev-parse --git-common-dir)"
WT_LOCAL_HOOKS_DIR="$WT_LOCAL_COMMON_DIR/hooks"
STATUS_OUTPUT="$(lrc hooks status 2>&1)"

for hook in pre-commit prepare-commit-msg commit-msg post-commit; do
	assert_file_contains "local worktree install adds managed section: $hook" "# BEGIN lrc managed section - DO NOT EDIT" "$WT_LOCAL_HOOKS_DIR/$hook"
done

assert_contains "local worktree status reports common-dir hooks path" "hooksPath: $WT_LOCAL_HOOKS_DIR" "$STATUS_OUTPUT"
assert_contains "local worktree status reports worktree root" "repo: $WT_LOCAL_DIR" "$STATUS_OUTPUT"

lrc hooks uninstall --local >/dev/null

for hook in pre-commit prepare-commit-msg commit-msg post-commit; do
	assert_file_not_contains_or_missing "local worktree uninstall removes managed section: $hook" "# BEGIN lrc managed section - DO NOT EDIT" "$WT_LOCAL_HOOKS_DIR/$hook"
done

bold ""
bold "══ Local Hook Management In Subdirectories ════════════════════"

mkdir -p "$LOCAL_REPO_DIR/subdir"
cd "$LOCAL_REPO_DIR/subdir"

lrc hooks install --local >/dev/null
STATUS_OUTPUT_SUBDIR="$(lrc hooks status 2>&1)"

for hook in pre-commit prepare-commit-msg commit-msg post-commit; do
	assert_file_contains "subdirectory install adds managed section: $hook" "# BEGIN lrc managed section - DO NOT EDIT" "$WT_LOCAL_HOOKS_DIR/$hook"
done

assert_contains "subdirectory status reports common-dir hooks path" "hooksPath: $WT_LOCAL_HOOKS_DIR" "$STATUS_OUTPUT_SUBDIR"
assert_contains "subdirectory status reports worktree root" "repo: $LOCAL_REPO_DIR" "$STATUS_OUTPUT_SUBDIR"

lrc hooks uninstall --local >/dev/null

for hook in pre-commit prepare-commit-msg commit-msg post-commit; do
	assert_file_not_contains_or_missing "subdirectory uninstall removes managed section: $hook" "# BEGIN lrc managed section - DO NOT EDIT" "$WT_LOCAL_HOOKS_DIR/$hook"
done

cd "$LOCAL_REPO_DIR"

bold ""
bold "══ Results ═══════════════════════════════════════════════════"
TOTAL=$((PASS + FAIL))
if [[ $FAIL -eq 0 ]]; then
	green "All $TOTAL tests passed."
	exit 0
else
	red "$FAIL of $TOTAL tests failed."
	exit 1
fi