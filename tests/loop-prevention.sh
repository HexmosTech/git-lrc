#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/installer-test-lib.sh"

trap cleanup_installer_test_env EXIT

require_claude_cli

plugin_first_scenario() {
	setup_installer_test_env "lrc-loop-plugin-first"
	install_local_shell_installer_curl_shim

	bold ""
	bold "══ Loop Prevention: Plugin First ═══════════════════════════"

	HOME="$TEST_HOME" PATH="$TEST_BASE_PATH" claude plugin marketplace add "$CLAUDE_LRC_DIR" >/dev/null
	HOME="$TEST_HOME" PATH="$TEST_BASE_PATH" claude plugin install lrc@claude-lrc >/dev/null
	PLUGIN_INSTALL_PATH="$(get_plugin_install_path "lrc@claude-lrc")"
	ENSURE_PATH="$PLUGIN_INSTALL_PATH/scripts/ensure-lrc.sh"

	HOME="$TEST_HOME" PATH="$TEST_BASE_PATH" "$ENSURE_PATH" >/dev/null
	PLUGIN_COUNT_BEFORE="$(get_plugin_count)"
	assert_contains "plugin-first flow installs exactly one plugin" '1' "$PLUGIN_COUNT_BEFORE"

	INSTALL_LOG="$TEST_LOG_DIR/loop-plugin-first-installer.log"
	run_installer_capture \
		"$INSTALL_LOG" \
		env LRC_CLAUDE_PLUGIN_MARKETPLACE_SOURCE="$CLAUDE_LRC_DIR" \
		"$GIT_LRC_DIR/scripts/lrc-install.sh"
	assert_exit_code "installer rerun succeeds after plugin-first bootstrap" "0" "$LAST_STATUS"
	assert_file_contains "installer rerun detects installed plugin and skips bootstrap" "Claude plugin 'lrc' already installed; skipping bootstrap" "$INSTALL_LOG"
	PLUGIN_COUNT_AFTER="$(get_plugin_count)"
	assert_contains "installer rerun keeps single plugin installation" '1' "$PLUGIN_COUNT_AFTER"
	assert_path_missing "plugin-first flow leaves no stale bootstrap lock" "$TEST_HOME/.claude/plugins/data/lrc/bootstrap.lock"

	cleanup_installer_test_env
}

installer_first_scenario() {
	setup_installer_test_env "lrc-loop-installer-first"

	bold ""
	bold "══ Loop Prevention: Installer First ════════════════════════"

	INSTALL_LOG="$TEST_LOG_DIR/loop-installer-first.log"
	run_installer_capture \
		"$INSTALL_LOG" \
		env LRC_CLAUDE_PLUGIN_MARKETPLACE_SOURCE="$CLAUDE_LRC_DIR" \
		"$GIT_LRC_DIR/scripts/lrc-install.sh"
	assert_exit_code "installer-first flow succeeds" "0" "$LAST_STATUS"

	PLUGIN_LIST_JSON="$(wait_for_plugin_install "lrc@claude-lrc" || true)"
	assert_contains "installer-first flow bootstraps plugin" '"id": "lrc@claude-lrc"' "$PLUGIN_LIST_JSON"
	PLUGIN_INSTALL_PATH="$(get_plugin_install_path "lrc@claude-lrc")"
	ENSURE_PATH="$PLUGIN_INSTALL_PATH/scripts/ensure-lrc.sh"
	assert_path_exists "installer-first flow exposes installed ensure helper" "$ENSURE_PATH"

	ENSURE_LOG="$TEST_LOG_DIR/loop-installer-first-ensure.log"
	set +e
	HOME="$TEST_HOME" PATH="$TEST_RUNTIME_PATH" "$ENSURE_PATH" >"$ENSURE_LOG" 2>&1
	ENSURE_STATUS=$?
	set -e
	assert_exit_code "ensure helper succeeds when backend is already installed" "0" "$ENSURE_STATUS"
	assert_not_contains "ensure helper does not emit bootstrap failure when backend already exists" 'failed to bootstrap git-lrc backend' "$(cat "$ENSURE_LOG")"
	assert_path_missing "installer-first flow leaves no stale bootstrap lock" "$TEST_HOME/.claude/plugins/data/lrc/bootstrap.lock"
	assert_path_missing "installer-first flow does not recreate legacy Claude skill" "$TEST_HOME/.claude/skills/lrc"

	cleanup_installer_test_env
}

plugin_first_scenario
installer_first_scenario
finish_tests