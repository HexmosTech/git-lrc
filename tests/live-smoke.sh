#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/installer-test-lib.sh"

require_claude_cli

if [[ "${LIVE_SMOKE:-${LIVE:-0}}" != "1" ]]; then
	skip_test "Skipping live smoke because LIVE_SMOKE=1 (or LIVE=1) was not set."
fi

run_public_installer_flow() {
	setup_installer_test_env "lrc-live-installer"

	bold ""
	bold "══ Live Smoke: Public Installer ═══════════════════════════"

	INSTALL_LOG="$TEST_LOG_DIR/live-installer.log"
	set +e
	HOME="$TEST_HOME" PATH="$TEST_BASE_PATH" bash -c 'curl -fsSL https://hexmos.com/lrc-install.sh | env LRC_CLAUDE_PLUGIN_MARKETPLACE_SOURCE=HexmosTech/claude-lrc LRC_CLAUDE_PLUGIN_MARKETPLACE_NAME=claude-lrc bash' >"$INSTALL_LOG" 2>&1
	LAST_STATUS=$?
	set -e
	assert_exit_code "public shell installer exits successfully" "0" "$LAST_STATUS"
	assert_path_exists "public shell installer writes lrc into temp home" "$TEST_HOME/.local/bin/lrc"
	assert_ok "public shell installer lrc binary reports a version" env HOME="$TEST_HOME" PATH="$TEST_RUNTIME_PATH" "$TEST_HOME/.local/bin/lrc" version

	PLUGIN_LIST_JSON="$(wait_for_plugin_install "lrc@claude-lrc" || true)"
	assert_contains "public shell installer bootstraps published plugin" '"id": "lrc@claude-lrc"' "$PLUGIN_LIST_JSON"
	assert_path_missing "public shell installer does not leave legacy Claude skill" "$TEST_HOME/.claude/skills/lrc"

	cleanup_installer_test_env
}

run_public_plugin_flow() {
	setup_installer_test_env "lrc-live-plugin"

	bold ""
	bold "══ Live Smoke: Public Marketplace Plugin ══════════════════"

	HOME="$TEST_HOME" PATH="$TEST_BASE_PATH" claude plugin marketplace add HexmosTech/claude-lrc >/dev/null
	HOME="$TEST_HOME" PATH="$TEST_BASE_PATH" claude plugin install lrc@claude-lrc >/dev/null

	PLUGIN_INSTALL_PATH="$(get_plugin_install_path "lrc@claude-lrc")"
	ENSURE_PATH="$PLUGIN_INSTALL_PATH/scripts/ensure-lrc.sh"
	assert_path_exists "published plugin install path exists" "$PLUGIN_INSTALL_PATH"
	assert_path_exists "published plugin exposes ensure-lrc.sh" "$ENSURE_PATH"

	ENSURE_LOG="$TEST_LOG_DIR/live-plugin-ensure.log"
	set +e
	HOME="$TEST_HOME" PATH="$TEST_BASE_PATH" "$ENSURE_PATH" >"$ENSURE_LOG" 2>&1
	ENSURE_STATUS=$?
	set -e
	assert_exit_code "published plugin bootstrap installs backend successfully" "0" "$ENSURE_STATUS"
	assert_path_exists "published plugin bootstrap writes lrc into temp home" "$TEST_HOME/.local/bin/lrc"
	assert_ok "published plugin bootstrap lrc binary reports a version" env HOME="$TEST_HOME" PATH="$TEST_RUNTIME_PATH" "$TEST_HOME/.local/bin/lrc" version

	cleanup_installer_test_env
}

trap cleanup_installer_test_env EXIT
run_public_installer_flow
run_public_plugin_flow
finish_tests