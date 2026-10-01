#!/bin/bash
set -euo pipefail

INSTALLER_TEST_LIB_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
GIT_LRC_DIR="${GIT_LRC_DIR:-$(CDPATH= cd -- "$INSTALLER_TEST_LIB_DIR/.." && pwd)}"
DEFAULT_CLAUDE_LRC_DIR="$(CDPATH= cd -- "$GIT_LRC_DIR/../claude-lrc" 2>/dev/null && pwd || true)"
if [[ -z "$DEFAULT_CLAUDE_LRC_DIR" ]]; then
	DEFAULT_CLAUDE_LRC_DIR="/home/shrsv/bin/claude-lrc"
fi
CLAUDE_LRC_DIR="${CLAUDE_LRC_DIR:-$DEFAULT_CLAUDE_LRC_DIR}"

PASS=0
FAIL=0

red()   { printf "\033[31m%s\033[0m\n" "$*"; }
green() { printf "\033[32m%s\033[0m\n" "$*"; }
bold()  { printf "\033[1m%s\033[0m\n" "$*"; }

assert_ok() {
	local desc="$1"
	shift
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

assert_not_contains() {
	local desc="$1" needle="$2" haystack="$3"
	if [[ "$haystack" != *"$needle"* ]]; then
		green "  ✓ $desc"
		PASS=$((PASS + 1))
	else
		red "  ✗ $desc"
		red "    did not expect to contain: '$needle'"
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

assert_path_missing() {
	local desc="$1" path="$2"
	if [[ ! -e "$path" ]]; then
		green "  ✓ $desc"
		PASS=$((PASS + 1))
	else
		red "  ✗ $desc"
		red "    did not expect path to exist: $path"
		FAIL=$((FAIL + 1))
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

finish_tests() {
	local total
	total=$((PASS + FAIL))
	bold ""
	bold "══ Results ═══════════════════════════════════════════════════"
	if [[ $FAIL -eq 0 ]]; then
		green "All $total tests passed."
		return 0
	fi
	red "$FAIL of $total tests failed."
	return 1
}

setup_installer_test_env() {
	TEST_NAME="$1"
	TMP_ROOT="$(mktemp -d "/tmp/${TEST_NAME}.XXXXXX")"
	TEST_HOME="$TMP_ROOT/home"
	TEST_BIN_DIR="$TMP_ROOT/toolbin"
	TEST_REPO_DIR="$TMP_ROOT/repo"
	TEST_LOG_DIR="$TMP_ROOT/logs"
	TEST_EMPTY_HOOKS_DIR="$TMP_ROOT/empty-hooks"
	mkdir -p "$TEST_HOME" "$TEST_BIN_DIR" "$TEST_REPO_DIR" "$TEST_LOG_DIR" "$TEST_EMPTY_HOOKS_DIR"

	if command -v claude >/dev/null 2>&1; then
		ln -s "$(command -v claude)" "$TEST_BIN_DIR/claude"
	fi

	TEST_BASE_PATH="$TEST_BIN_DIR:/usr/bin:/bin"
	TEST_RUNTIME_PATH="$TEST_HOME/.local/bin:$TEST_BASE_PATH"

	git init --initial-branch=main "$TEST_REPO_DIR" >/dev/null
	git -C "$TEST_REPO_DIR" config user.email test@example.com
	git -C "$TEST_REPO_DIR" config user.name "Test User"
	printf 'seed\n' > "$TEST_REPO_DIR/seed.txt"
	git -C "$TEST_REPO_DIR" add seed.txt
	git -C "$TEST_REPO_DIR" -c core.hooksPath="$TEST_EMPTY_HOOKS_DIR" commit -m "init" >/dev/null

	export TEST_NAME TMP_ROOT TEST_HOME TEST_BIN_DIR TEST_REPO_DIR TEST_LOG_DIR TEST_EMPTY_HOOKS_DIR
	export GIT_LRC_DIR CLAUDE_LRC_DIR
	export TEST_BASE_PATH TEST_RUNTIME_PATH

	bold "Using temp root: $TMP_ROOT"
	bold "Using temp home: $TEST_HOME"
	bold "Using temp repo: $TEST_REPO_DIR"
}

cleanup_installer_test_env() {
	local root="${TMP_ROOT:-}"
	[[ -z "$root" ]] && return 0
	# claude's background update-checker can still be writing into TEST_HOME
	# (e.g. .claude.json) right after the test's foreground commands return,
	# racing this rm -rf with "Directory not empty". Retry briefly instead
	# of letting that transient race fail the whole test run.
	local attempt
	for attempt in 1 2 3 4 5; do
		rm -rf "$root" 2>/dev/null && return 0
		sleep 0.5
	done
	rm -rf "$root"
}

skip_test() {
	bold ""
	bold "══ Skipped ═══════════════════════════════════════════════════"
	printf '%s\n' "$1"
	exit 0
}

require_claude_cli() {
	if ! command -v claude >/dev/null 2>&1; then
		red "ERROR: claude CLI not found in PATH. Install Claude Code before running this harness."
		exit 1
	fi
}

get_plugin_install_path() {
	local plugin_id="$1"
	HOME="$TEST_HOME" PATH="$TEST_RUNTIME_PATH" claude plugin list --json | python3 -c '
import json
import sys

plugin_id = sys.argv[1]
payload = json.load(sys.stdin)
for entry in payload:
	if isinstance(entry, dict) and entry.get("id") == plugin_id:
		install_path = entry.get("installPath")
		if isinstance(install_path, str) and install_path:
			print(install_path, end="")
			raise SystemExit(0)

raise SystemExit(1)
' "$plugin_id"
}

get_plugin_count() {
	HOME="$TEST_HOME" PATH="$TEST_RUNTIME_PATH" claude plugin list --json | python3 -c '
import json
import sys

payload = json.load(sys.stdin)
print(sum(1 for entry in payload if isinstance(entry, dict) and entry.get("id")), end="")
'
}

install_local_shell_installer_curl_shim() {
	cat > "$TEST_BIN_DIR/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ge 2 ] && [ "$1" = "-fsSL" ] && [ "$2" = "https://hexmos.com/lrc-install.sh" ]; then
	cat "$GIT_LRC_DIR/scripts/lrc-install.sh"
	exit 0
fi

exec /usr/bin/curl "$@"
EOF
	chmod +x "$TEST_BIN_DIR/curl"
}

build_hook_payload() {
	local cwd="$1"
	local command="$2"
	python3 - "$cwd" "$command" <<'PY'
import json
import sys

print(json.dumps({
    "tool_input": {
        "command": sys.argv[2],
        "cwd": sys.argv[1],
    }
}))
PY
}

extract_hook_updated_command() {
	local validator_output="$1"
	VALIDATOR_UPDATED_COMMAND="$(VALIDATOR_OUTPUT="$validator_output" python3 - <<'PY'
import json
import os

payload = json.loads(os.environ["VALIDATOR_OUTPUT"])
hook_output = payload["hookSpecificOutput"]
if hook_output["permissionDecision"] != "allow":
    raise SystemExit("validator did not allow git commit")
print(hook_output["updatedInput"]["command"], end="")
PY
)"
}

run_bash_hook_validator() {
	local validator_path="$1"
	local payload="$2"
	LAST_VALIDATOR_OUTPUT="$(HOME="$TEST_HOME" PATH="$TEST_RUNTIME_PATH" CLAUDE_PROJECT_DIR="$TEST_REPO_DIR" "$validator_path" <<<"$payload")"
	extract_hook_updated_command "$LAST_VALIDATOR_OUTPUT"
}

run_wrapped_bash_commit() {
	local rewritten_command="$1"
	local output_file="$2"
	set +e
	(
		cd "$TEST_REPO_DIR"
		HOME="$TEST_HOME" PATH="$TEST_RUNTIME_PATH" CLAUDE_PROJECT_DIR="$TEST_REPO_DIR" timeout 30s bash -c "$rewritten_command"
	) >"$output_file" 2>&1
	LAST_STATUS=$?
	set -e
	LAST_OUTPUT_FILE="$output_file"
}

find_powershell_command() {
	local candidate
	local candidate_path
	for candidate in pwsh powershell pwsh.exe powershell.exe; do
		if command -v "$candidate" >/dev/null 2>&1; then
			candidate_path="$(command -v "$candidate")"
			if [[ -x "$candidate_path" ]]; then
				printf '%s\n' "$candidate_path"
				return 0
			fi
		fi
	done
	return 1
}

is_windows_host() {
	case "$(uname -s | tr '[:upper:]' '[:lower:]')" in
		msys*|mingw*|cygwin*)
			return 0
			;;
		*)
			return 1
			;;
	esac
}

wait_for_plugin_install() {
	local plugin_id="$1"
	local attempts=200
	local interval_seconds="0.1"
	local output
	local attempt

	for attempt in $(seq 1 "$attempts"); do
		output="$(HOME="$TEST_HOME" PATH="$TEST_RUNTIME_PATH" claude plugin list --json 2>/dev/null || true)"
		if [[ "$output" == *"\"id\": \"$plugin_id\""* ]]; then
			printf '%s\n' "$output"
			return 0
		fi
		sleep "$interval_seconds"
	done

	printf '%s\n' "$output"
	return 1
}

run_installer_capture() {
	local output_file="$1"
	shift
	set +e
	HOME="$TEST_HOME" PATH="$TEST_BASE_PATH" "$@" >"$output_file" 2>&1
	LAST_STATUS=$?
	set -e
	LAST_OUTPUT_FILE="$output_file"
}