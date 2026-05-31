#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/installer-test-lib.sh"

trap cleanup_installer_test_env EXIT

POWERSHELL_CMD="$(find_powershell_command || true)"
if [[ -z "$POWERSHELL_CMD" ]]; then
	skip_test "Skipping PowerShell smoke test because PowerShell is not available in PATH."
fi

if ! is_windows_host; then
	skip_test "Skipping PowerShell smoke test because this host is not Windows/Git Bash."
fi

require_claude_cli
setup_installer_test_env "lrc-powershell-smoke"

bold ""
bold "══ PowerShell Smoke ════════════════════════════════════════"

WINDOWS_TEST_HOME="$(cygpath -w "$TEST_HOME")"
WINDOWS_LOCALAPPDATA="$(cygpath -w "$TEST_HOME/AppData/Local")"
WINDOWS_USERPROFILE="$(cygpath -w "$TEST_HOME")"
WINDOWS_INSTALLER_PATH="$(cygpath -w "$GIT_LRC_DIR/scripts/lrc-install.ps1")"

INSTALL_LOG="$TEST_LOG_DIR/powershell-install.log"
set +e
HOME="$TEST_HOME" USERPROFILE="$TEST_HOME" LOCALAPPDATA="$TEST_HOME/AppData/Local" PATH="$TEST_BASE_PATH" \
	LRC_INSTALL_SKIP_HOOKS=1 LRC_INSTALL_BOOTSTRAP_CLAUDE_PLUGIN=0 \
	"$POWERSHELL_CMD" -NoProfile -ExecutionPolicy Bypass -File "$WINDOWS_INSTALLER_PATH" >"$INSTALL_LOG" 2>&1
INSTALL_STATUS=$?
set -e

assert_exit_code "PowerShell installer smoke run succeeds" "0" "$INSTALL_STATUS"
assert_path_exists "PowerShell installer writes lrc.exe into LocalAppData" "$TEST_HOME/AppData/Local/Programs/lrc/lrc.exe"

PLUGIN_INSTALL_LOG="$TEST_LOG_DIR/powershell-plugin-install.log"
set +e
HOME="$TEST_HOME" PATH="$TEST_BASE_PATH" claude plugin marketplace add "$CLAUDE_LRC_DIR" >"$PLUGIN_INSTALL_LOG" 2>&1
MARKETPLACE_STATUS=$?
set -e
assert_exit_code "PowerShell smoke can add local marketplace" "0" "$MARKETPLACE_STATUS"

set +e
HOME="$TEST_HOME" PATH="$TEST_BASE_PATH" claude plugin install lrc@claude-lrc >>"$PLUGIN_INSTALL_LOG" 2>&1
PLUGIN_STATUS=$?
set -e
assert_exit_code "PowerShell smoke can install local plugin" "0" "$PLUGIN_STATUS"

PLUGIN_INSTALL_PATH="$(get_plugin_install_path "lrc@claude-lrc")"
POWERSHELL_ENSURE_PATH="$(cygpath -w "$PLUGIN_INSTALL_PATH/scripts/ensure-lrc.ps1")"
assert_path_exists "PowerShell smoke locates installed ensure-lrc.ps1" "$PLUGIN_INSTALL_PATH/scripts/ensure-lrc.ps1"

ENSURE_LOG="$TEST_LOG_DIR/powershell-ensure.log"
set +e
HOME="$TEST_HOME" USERPROFILE="$TEST_HOME" LOCALAPPDATA="$TEST_HOME/AppData/Local" PATH="$TEST_BASE_PATH" \
	"$POWERSHELL_CMD" -NoProfile -ExecutionPolicy Bypass -File "$POWERSHELL_ENSURE_PATH" >"$ENSURE_LOG" 2>&1
ENSURE_STATUS=$?
set -e
assert_exit_code "PowerShell ensure helper succeeds when backend is already installed" "0" "$ENSURE_STATUS"

finish_tests