#!/bin/bash
# End-to-end test: global hooks install → commit blocked → uninstall → commit succeeds
# This tests the full lifecycle of lrc global hook management.
#
# Usage: bash tests/global-hooks.sh
set -euo pipefail

# ─── Helpers ──────────────────────────────────────────────────────────────────

PASS=0
FAIL=0
CLEANUP_ITEMS=()

red()   { printf "\033[31m%s\033[0m\n" "$*"; }
green() { printf "\033[32m%s\033[0m\n" "$*"; }
bold()  { printf "\033[1m%s\033[0m\n" "$*"; }

assert_ok() {
    local desc="$1"; shift
    if "$@"; then
        green "  ✓ $desc"
        PASS=$((PASS + 1))
    else
        red "  ✗ $desc (command failed: $*)"
        FAIL=$((FAIL + 1))
    fi
}

assert_fail() {
    local desc="$1"; shift
    if "$@"; then
        red "  ✗ $desc (expected failure but succeeded: $*)"
        FAIL=$((FAIL + 1))
    else
        green "  ✓ $desc"
        PASS=$((PASS + 1))
    fi
}

assert_eq() {
    local desc="$1" expected="$2" actual="$3"
    if [[ "$expected" == "$actual" ]]; then
        green "  ✓ $desc"
        PASS=$((PASS + 1))
    else
        red "  ✗ $desc"
        red "    expected: '$expected'"
        red "    actual:   '$actual'"
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

assert_not_contains() {
    local desc="$1" needle="$2" haystack="$3"
    if [[ "$haystack" != *"$needle"* ]]; then
        green "  ✓ $desc"
        PASS=$((PASS + 1))
    else
        red "  ✗ $desc"
        red "    expected NOT to contain: '$needle'"
        red "    got: '$haystack'"
        FAIL=$((FAIL + 1))
    fi
}

assert_path_exists() {
    local desc="$1" path="$2"
    if [[ -e "$path" ]]; then
        green "  ✓ $desc"
        PASS=$((PASS + 1))
    else
        red "  ✗ $desc (path not found: $path)"
        FAIL=$((FAIL + 1))
    fi
}

assert_path_not_exists() {
    local desc="$1" path="$2"
    if [[ ! -e "$path" ]]; then
        green "  ✓ $desc"
        PASS=$((PASS + 1))
    else
        red "  ✗ $desc (path should not exist: $path)"
        FAIL=$((FAIL + 1))
    fi
}

cleanup() {
    bold ""
    bold "── Cleanup ─────────────────────────────────────────────"
    # Move to a safe directory before removing temp dirs
    cd /tmp
    for item in "${CLEANUP_ITEMS[@]}"; do
        if [[ -d "$item" ]]; then
            rm -rf "$item"
            echo "  removed dir: $item"
        elif [[ -f "$item" ]]; then
            rm -f "$item"
            echo "  removed file: $item"
        fi
    done

    # Restore core.hooksPath if we saved one
    if [[ -n "${ORIG_HOOKS_PATH:-}" ]]; then
        git config --global core.hooksPath "$ORIG_HOOKS_PATH"
        echo "  restored core.hooksPath to: $ORIG_HOOKS_PATH"
    elif [[ "${HAD_HOOKS_PATH:-}" == "no" ]]; then
        git config --global --unset core.hooksPath 2>/dev/null || true
        echo "  unset core.hooksPath (was not set before test)"
    fi
}
trap cleanup EXIT

# ─── Save original state ─────────────────────────────────────────────────────

ORIG_HOOKS_PATH="$(git config --global --get core.hooksPath 2>/dev/null || true)"
if [[ -n "$ORIG_HOOKS_PATH" ]]; then
    HAD_HOOKS_PATH="yes"
else
    HAD_HOOKS_PATH="no"
fi

LRC="$(command -v lrc)"
if [[ -z "$LRC" ]]; then
    red "ERROR: lrc not found in PATH. Build and install first."
    exit 1
fi

bold "Using lrc: $LRC"
bold "Original core.hooksPath: '${ORIG_HOOKS_PATH:-<unset>}'"

# Use a dedicated test hooks path so we don't clobber the user's real one
TEST_HOOKS_DIR="$(mktemp -d /tmp/lrc-test-hooks.XXXXXX)"
TEST_REPO="$(mktemp -d /tmp/lrc-test-repo.XXXXXX)"
CLEANUP_ITEMS+=("$TEST_HOOKS_DIR" "$TEST_REPO")

# ═════════════════════════════════════════════════════════════════════════════
bold ""
bold "══ Phase 1: Install global hooks ═══════════════════════════"

# Unset any existing core.hooksPath first for a clean slate
git config --global --unset core.hooksPath 2>/dev/null || true

install_output="$(lrc hooks install --path "$TEST_HOOKS_DIR" 2>&1)"
echo "$install_output"

assert_contains "install reports success" "LiveReview global hooks installed" "$install_output"

# Verify core.hooksPath is set
actual_path="$(git config --global --get core.hooksPath 2>/dev/null || true)"
assert_eq "core.hooksPath points to test dir" "$TEST_HOOKS_DIR" "$actual_path"

# Verify dispatchers exist
for hook in pre-commit prepare-commit-msg commit-msg post-commit; do
    assert_path_exists "dispatcher exists: $hook" "$TEST_HOOKS_DIR/$hook"
done

# Verify lrc managed scripts exist
for hook in pre-commit prepare-commit-msg commit-msg post-commit; do
    assert_path_exists "lrc script exists: lrc/$hook" "$TEST_HOOKS_DIR/lrc/$hook"
done

# Verify meta file
assert_path_exists "meta file exists" "$TEST_HOOKS_DIR/.lrc-hooks-meta.json"
meta_content="$(cat "$TEST_HOOKS_DIR/.lrc-hooks-meta.json")"
assert_contains "meta has set_by_lrc: true" '"set_by_lrc": true' "$meta_content"

# ═════════════════════════════════════════════════════════════════════════════
bold ""
bold "══ Phase 2: Create test repo and stage changes ═════════════"

cd "$TEST_REPO"
git init --initial-branch=main .
git config user.email "test@test.com"
git config user.name "Test User"

# Need an initial commit so the repo is usable
echo "initial" > README.md
git add README.md
# Initial commit - do it in non-interactive mode. Since there's no attestation
# yet, we need to bypass the hook for the initial commit. Use --no-verify.
git commit --no-verify -m "Initial commit"

# Now stage real changes
echo "hello world" > test.txt
git add test.txt

bold ""
bold "══ Phase 3: Commit should be BLOCKED (no attestation) ══════"

# The pre-commit hook blocks in non-interactive mode (when stdout is not a tty).
# Pipe through cat to simulate non-interactive (CI-like) environment.
commit_output="$(git commit -m "test commit without review" 2>&1 | cat)" || true
commit_exit=$?

# The hook should have blocked with attestation message
assert_contains "commit blocked with attestation message" \
    "You are using LiveReview" "$commit_output"

# Verify nothing was committed (test.txt still staged, not committed)
status_output="$(git status --porcelain)"
assert_contains "test.txt still staged (not committed)" "A  test.txt" "$status_output"

bold ""
bold "══ Phase 4: Uninstall global hooks ═════════════════════════"

uninstall_output="$(lrc hooks uninstall --path "$TEST_HOOKS_DIR" 2>&1)"
echo "$uninstall_output"

assert_contains "uninstall reports removal" "Removed LiveReview sections" "$uninstall_output"

# Verify core.hooksPath unset (lrc set it, so it should unset)
post_hooks_path="$(git config --global --get core.hooksPath 2>/dev/null || true)"
assert_eq "core.hooksPath is unset after uninstall" "" "$post_hooks_path"

# Verify dispatchers removed (they were lrc-only, so files should be gone)
for hook in pre-commit prepare-commit-msg commit-msg post-commit; do
    assert_path_not_exists "dispatcher removed: $hook" "$TEST_HOOKS_DIR/$hook"
done

# Verify lrc subdir removed
assert_path_not_exists "lrc/ directory removed" "$TEST_HOOKS_DIR/lrc"

# Verify meta file removed
assert_path_not_exists "meta file removed" "$TEST_HOOKS_DIR/.lrc-hooks-meta.json"

# Verify backups dir removed
assert_path_not_exists "backups dir removed" "$TEST_HOOKS_DIR/.lrc_backups"

# Verify hooks directory itself is cleaned (empty dir removed)
assert_path_not_exists "hooks dir cleaned up (empty)" "$TEST_HOOKS_DIR"

bold ""
bold "══ Phase 5: Commit should SUCCEED (hooks removed) ══════════"

cd "$TEST_REPO"

# Same non-interactive approach, but now hooks are gone
commit_output2="$(git commit -m "test commit after uninstall" 2>&1 | cat)" || true

assert_contains "commit succeeded" "test commit after uninstall" "$commit_output2"
assert_not_contains "no LiveReview message" "You are using LiveReview" "$commit_output2"

# Verify test.txt is committed
status_output2="$(git status --porcelain)"
assert_not_contains "test.txt no longer staged" "A  test.txt" "$status_output2"

# Verify in log
log_output="$(git log --oneline -1)"
assert_contains "commit appears in log" "test commit after uninstall" "$log_output"

# ═════════════════════════════════════════════════════════════════════════════
bold ""
bold "══ Phase 6: Recovery — uninstall without meta file ══════════"

# Re-install, delete meta, then uninstall - should still clean up
git config --global --unset core.hooksPath 2>/dev/null || true
TEST_HOOKS_DIR2="$(mktemp -d /tmp/lrc-test-hooks2.XXXXXX)"
CLEANUP_ITEMS+=("$TEST_HOOKS_DIR2")

lrc hooks install --path "$TEST_HOOKS_DIR2" >/dev/null 2>&1
rm -f "$TEST_HOOKS_DIR2/.lrc-hooks-meta.json"

recovery_output="$(lrc hooks uninstall --path "$TEST_HOOKS_DIR2" 2>&1)"
echo "$recovery_output"

post_path="$(git config --global --get core.hooksPath 2>/dev/null || true)"
assert_eq "core.hooksPath unset after recovery uninstall" "" "$post_path"
assert_contains "recovery detected and cleaned" "Unset core.hooksPath" "$recovery_output"

# ═════════════════════════════════════════════════════════════════════════════
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
