#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/installer-test-lib.sh"

trap cleanup_installer_test_env EXIT

require_claude_cli
setup_installer_test_env "lrc-plugin-hooks"
install_local_shell_installer_curl_shim

bold ""
bold "══ Plugin Hook Invocation ══════════════════════════════════"

HOME="$TEST_HOME" PATH="$TEST_BASE_PATH" claude plugin marketplace add "$CLAUDE_LRC_DIR" >/dev/null
HOME="$TEST_HOME" PATH="$TEST_BASE_PATH" claude plugin install lrc@claude-lrc >/dev/null

PLUGIN_INSTALL_PATH="$(get_plugin_install_path "lrc@claude-lrc")"
BASH_VALIDATOR_PATH="$PLUGIN_INSTALL_PATH/scripts/plugin-blocking-review-git-commit.sh"
POWERSHELL_VALIDATOR_PATH="$PLUGIN_INSTALL_PATH/scripts/plugin-blocking-review-git-commit.ps1"
BASH_ENSURE_PATH="$PLUGIN_INSTALL_PATH/scripts/ensure-lrc.sh"
HOOKS_JSON_PATH="$PLUGIN_INSTALL_PATH/hooks/hooks.json"

assert_path_exists "installed plugin exposes bash hook validator" "$BASH_VALIDATOR_PATH"
assert_path_exists "installed plugin exposes PowerShell hook validator" "$POWERSHELL_VALIDATOR_PATH"
assert_path_exists "installed plugin exposes bash bootstrap helper" "$BASH_ENSURE_PATH"
assert_file_contains "hook manifest includes Bash matcher" '"matcher": "Bash"' "$HOOKS_JSON_PATH"
assert_file_contains "hook manifest includes PowerShell matcher" '"matcher": "PowerShell"' "$HOOKS_JSON_PATH"

HOME="$TEST_HOME" PATH="$TEST_BASE_PATH" "$BASH_ENSURE_PATH" >/dev/null
assert_path_exists "hook test bootstraps backend lrc" "$TEST_HOME/.local/bin/lrc"

cd "$TEST_REPO_DIR"
printf 'hook-bash\n' > bash-hook.txt
git add bash-hook.txt
HOME="$TEST_HOME" PATH="$TEST_RUNTIME_PATH" "$TEST_HOME/.local/bin/lrc" review --staged --vouch >/dev/null

BASH_PAYLOAD="$(build_hook_payload "$TEST_REPO_DIR" "git commit -m 'plugin bash attested'")"
run_bash_hook_validator "$BASH_VALIDATOR_PATH" "$BASH_PAYLOAD"
assert_contains "bash validator allows git commit" '"permissionDecision": "allow"' "$LAST_VALIDATOR_OUTPUT"
assert_contains "bash validator rewrites command to installed wrapper" 'plugin-run-blocking-review-git-commit.sh' "$LAST_VALIDATOR_OUTPUT"

BASH_ATTESTED_OUTPUT="$TEST_LOG_DIR/plugin-bash-attested.out"
run_wrapped_bash_commit "$VALIDATOR_UPDATED_COMMAND" "$BASH_ATTESTED_OUTPUT"
assert_exit_code "bash wrapper reuses attestation and commits successfully" "0" "$LAST_STATUS"
assert_file_contains "bash wrapper reports attestation reuse" 'current tree is already reviewed; proceeding with git commit' "$BASH_ATTESTED_OUTPUT"

printf 'hook-disabled\n' > bash-hook-disabled.txt
git add bash-hook-disabled.txt
HOME="$TEST_HOME" PATH="$TEST_RUNTIME_PATH" "$TEST_HOME/.local/bin/lrc" hooks disable --surface claude >/dev/null

BASH_DISABLED_PAYLOAD="$(build_hook_payload "$TEST_REPO_DIR" "git commit -m 'plugin bash disabled'")"
run_bash_hook_validator "$BASH_VALIDATOR_PATH" "$BASH_DISABLED_PAYLOAD"
assert_contains "bash validator still allows disabled-marker commit path" '"permissionDecision": "allow"' "$LAST_VALIDATOR_OUTPUT"

BASH_DISABLED_OUTPUT="$TEST_LOG_DIR/plugin-bash-disabled.out"
run_wrapped_bash_commit "$VALIDATOR_UPDATED_COMMAND" "$BASH_DISABLED_OUTPUT"
assert_exit_code "bash wrapper bypasses review when claude surface is disabled" "0" "$LAST_STATUS"
assert_file_contains "bash wrapper reports disabled marker" 'Claude review hook disabled for this repository; proceeding with git commit.' "$BASH_DISABLED_OUTPUT"

POWERSHELL_CMD="$(find_powershell_command || true)"
if [[ -n "$POWERSHELL_CMD" ]] && is_windows_host; then
	POWERSHELL_PAYLOAD="$(build_hook_payload "$TEST_REPO_DIR" "git commit -m 'plugin powershell attested'")"
	LAST_VALIDATOR_OUTPUT="$(HOME="$TEST_HOME" PATH="$TEST_RUNTIME_PATH" CLAUDE_PROJECT_DIR="$TEST_REPO_DIR" "$POWERSHELL_CMD" -NoProfile -ExecutionPolicy Bypass -File "$POWERSHELL_VALIDATOR_PATH" <<<"$POWERSHELL_PAYLOAD")"
	assert_contains "PowerShell validator allows git commit" '"permissionDecision": "allow"' "$LAST_VALIDATOR_OUTPUT"
	assert_contains "PowerShell validator rewrites command to installed wrapper" 'plugin-run-blocking-review-git-commit.ps1' "$LAST_VALIDATOR_OUTPUT"
else
	bold ""
	printf '%s\n' "Skipping direct PowerShell hook invocation check because PowerShell is not available in PATH."
fi

finish_tests