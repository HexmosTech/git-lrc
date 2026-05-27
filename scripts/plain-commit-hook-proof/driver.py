#!/usr/bin/env python3
"""Disposable proof rig for plain `git commit` hook behavior.

This script creates a fully isolated temp repo under /tmp, writes hand-authored
local hooks plus helper processes, runs plain `git commit` under a PTY, and
classifies whether the editor opened or the commit completed directly with the
requested message.
"""

from __future__ import annotations

import argparse
import json
import os
import shlex
import shutil
import subprocess
import sys
import tempfile
import textwrap
import time
from pathlib import Path
from typing import Any


POLL_INTERVAL_SECONDS = 0.05
BLOCKER_WAIT_SECONDS = 5.0
DEFAULT_MESSAGE = "feat: isolated proof"
TEMP_ROOT_PREFIX = "git-lrc-plain-commit-proof-"


def sanitized_git_env(extra: dict[str, str] | None = None) -> dict[str, str]:
    env = os.environ.copy()
    for key in ("GIT_EDITOR", "EDITOR", "VISUAL"):
        env.pop(key, None)
    if extra:
        env.update(extra)
    return env


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run an isolated plain git commit hook proof under /tmp."
    )
    parser.add_argument(
        "--message",
        default=DEFAULT_MESSAGE,
        help="commit message to deliver in the passing scenario",
    )
    parser.add_argument(
        "--keep-temp",
        action="store_true",
        help="keep the /tmp workspace after the run",
    )
    parser.add_argument(
        "--keep-temp-on-failure",
        action="store_true",
        help="keep the /tmp workspace only when a check fails",
    )
    return parser.parse_args()


def run_command(
    args: list[str],
    *,
    cwd: Path | None = None,
    check: bool = True,
    env: dict[str, str] | None = None,
) -> subprocess.CompletedProcess[str]:
    completed = subprocess.run(
        args,
        cwd=cwd,
        env=env,
        text=True,
        capture_output=True,
    )
    if check and completed.returncode != 0:
        raise RuntimeError(
            f"command failed: {' '.join(args)}\nstdout:\n{completed.stdout}\nstderr:\n{completed.stderr}"
        )
    return completed


def write_file(path: Path, content: str, executable: bool = False) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")
    if executable:
        path.chmod(0o755)


def wait_for_path(path: Path, timeout_seconds: float) -> bool:
    deadline = time.time() + timeout_seconds
    while time.time() < deadline:
        if path.exists():
            return True
        time.sleep(POLL_INTERVAL_SECONDS)
    return False


def setup_repo(repo_dir: Path, editor_wrapper: Path) -> None:
    base_env = sanitized_git_env()
    run_command(["git", "init", "--initial-branch=main", str(repo_dir)], env=base_env)
    run_command(["git", "config", "user.email", "proof@example.com"], cwd=repo_dir, env=base_env)
    run_command(["git", "config", "user.name", "Proof Driver"], cwd=repo_dir, env=base_env)
    run_command(["git", "config", "core.editor", str(editor_wrapper)], cwd=repo_dir, env=base_env)

    tracked_file = repo_dir / "note.txt"
    tracked_file.write_text("isolated proof\n", encoding="utf-8")
    run_command(["git", "add", "."], cwd=repo_dir, env=base_env)


def helper_script_text() -> str:
    return "\n".join(
        [
            "#!/usr/bin/env python3",
            "import json",
            "import sys",
            "import time",
            "from pathlib import Path",
            "",
            "state_dir = Path(sys.argv[1])",
            "commit_msg_path = Path(sys.argv[2])",
            "timeout_seconds = float(sys.argv[3])",
            'started_marker = state_dir / "blocker-started"',
            'decision_file = state_dir / "decision.json"',
            'result_file = state_dir / "result.json"',
            "",
            'started_marker.write_text("started\\n", encoding="utf-8")',
            "deadline = time.time() + timeout_seconds",
            "while time.time() < deadline:",
            "    if decision_file.exists():",
            '        payload = json.loads(decision_file.read_text(encoding="utf-8"))',
            '        message = str(payload.get("message", "")).rstrip("\\n")',
            "        if message:",
            '            commit_msg_path.write_text(message + "\\n", encoding="utf-8")',
            "            result_file.write_text(",
            '                json.dumps({"status": "message-written", "message": message}),',
            '                encoding="utf-8",',
            "            )",
            "            raise SystemExit(0)",
            "        result_file.write_text(",
            '            json.dumps({"status": "invalid-message"}),',
            '            encoding="utf-8",',
            "        )",
            "        raise SystemExit(1)",
            "    time.sleep(0.05)",
            "",
            'result_file.write_text(json.dumps({"status": "timeout"}), encoding="utf-8")',
            "raise SystemExit(0)",
            "",
        ]
    )


def editor_wrapper_text(editor_marker: Path) -> str:
    return "\n".join(
        [
            "#!/bin/sh",
            f"printf 'editor-opened\\n' > {shlex.quote(str(editor_marker))}",
            "exit 0",
            "",
        ]
    )


def prepare_hook_text(helper_path: Path, state_dir: Path, helper_timeout: float) -> str:
    return "\n".join(
        [
            "#!/bin/sh",
            "set -eu",
            "",
            'COMMIT_MSG_FILE="$1"',
            f"STATE_DIR={shlex.quote(str(state_dir))}",
            f"HELPER={shlex.quote(str(helper_path))}",
            'RESULT_FILE="$STATE_DIR/result.json"',
            "",
            f'python3 "$HELPER" "$STATE_DIR" "$COMMIT_MSG_FILE" {helper_timeout} &',
            'HELPER_PID=$!',
            "",
            "waited=0",
            "limit=120",
            'while [ "$waited" -lt "$limit" ]; do',
            '    if [ -f "$RESULT_FILE" ]; then',
            "        break",
            "    fi",
            "    sleep 0.05",
            '    waited=$((waited + 1))',
            "done",
            "",
            'wait "$HELPER_PID" 2>/dev/null || true',
            "exit 0",
            "",
        ]
    )


def commit_hook_text(state_dir: Path) -> str:
    return "\n".join(
        [
            "#!/bin/sh",
            "set -eu",
            f"printf 'commit-msg-ran\\n' > {shlex.quote(str(state_dir / 'commit-msg-ran'))}",
            "exit 0",
            "",
        ]
    )


def create_case_layout(case_root: Path, helper_timeout: float) -> dict[str, Path]:
    repo_dir = case_root / "repo"
    hooks_dir = case_root / "repo-hooks"
    bin_dir = case_root / "bin"
    state_dir = case_root / "state"
    logs_dir = case_root / "logs"
    for path in (repo_dir, hooks_dir, bin_dir, state_dir, logs_dir):
        path.mkdir(parents=True, exist_ok=True)

    editor_marker = state_dir / "editor-invoked"
    helper_path = bin_dir / "blocker-helper.py"
    editor_path = bin_dir / "editor-wrapper.sh"
    prepare_hook_path = hooks_dir / "prepare-commit-msg"
    commit_hook_path = hooks_dir / "commit-msg"

    write_file(helper_path, helper_script_text(), executable=True)
    write_file(editor_path, editor_wrapper_text(editor_marker), executable=True)
    write_file(
        prepare_hook_path,
        prepare_hook_text(helper_path, state_dir, helper_timeout),
        executable=True,
    )
    write_file(commit_hook_path, commit_hook_text(state_dir), executable=True)
    setup_repo(repo_dir, editor_path)

    return {
        "case_root": case_root,
        "repo_dir": repo_dir,
        "hooks_dir": hooks_dir,
        "state_dir": state_dir,
        "logs_dir": logs_dir,
        "editor_marker": editor_marker,
    }


def start_commit_process(repo_dir: Path, hooks_dir: Path) -> subprocess.Popen[str]:
    hook_arg = shlex.quote(str(hooks_dir))
    cmd_text = f"git -c core.hooksPath={hook_arg} -c commit.cleanup=strip commit"
    return subprocess.Popen(
        ["script", "-qfec", cmd_text, "/dev/null"],
        cwd=repo_dir,
        env=sanitized_git_env(),
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )


def collect_commit_message(repo_dir: Path) -> str:
    completed = run_command(["git", "log", "-1", "--format=%B"], cwd=repo_dir, env=sanitized_git_env())
    return completed.stdout.rstrip("\n")


def run_case(case_name: str, case_root: Path, message: str, send_decision: bool) -> dict[str, Any]:
    helper_timeout = 2.0
    layout = create_case_layout(case_root, helper_timeout)
    state_dir = layout["state_dir"]
    repo_dir = layout["repo_dir"]
    logs_dir = layout["logs_dir"]

    process = start_commit_process(repo_dir, layout["hooks_dir"])
    blocker_started = wait_for_path(state_dir / "blocker-started", BLOCKER_WAIT_SECONDS)
    if send_decision and blocker_started:
        (state_dir / "decision.json").write_text(
            json.dumps({"message": message}),
            encoding="utf-8",
        )

    try:
        output, _ = process.communicate(timeout=BLOCKER_WAIT_SECONDS)
    except subprocess.TimeoutExpired:
        process.kill()
        output, _ = process.communicate()

    (logs_dir / "commit-session.log").write_text(output, encoding="utf-8")

    commit_message = None
    if process.returncode == 0:
        commit_message = collect_commit_message(repo_dir)

    result_payload: dict[str, Any] = {
        "case": case_name,
        "blocker_started": blocker_started,
        "decision_sent": send_decision,
        "editor_invoked": layout["editor_marker"].exists(),
        "commit_exit_code": process.returncode,
        "commit_message": commit_message,
        "expected_message": message,
        "helper_result": None,
        "conditions": {},
        "passed": False,
    }

    helper_result_path = state_dir / "result.json"
    if helper_result_path.exists():
        result_payload["helper_result"] = json.loads(
            helper_result_path.read_text(encoding="utf-8")
        )

    helper_status = None
    if isinstance(result_payload["helper_result"], dict):
        helper_status = result_payload["helper_result"].get("status")

    result_payload["conditions"] = {
        "commit_triggered_via_temporary_api": bool(send_decision and helper_status == "message-written"),
        "editor_not_opened": not result_payload["editor_invoked"],
        "message_recorded_in_git_log": bool(process.returncode == 0 and commit_message == message),
    }

    if send_decision:
        result_payload["passed"] = (
            blocker_started
            and result_payload["conditions"]["commit_triggered_via_temporary_api"]
            and result_payload["conditions"]["editor_not_opened"]
            and result_payload["conditions"]["message_recorded_in_git_log"]
        )
    else:
        result_payload["passed"] = result_payload["editor_invoked"] or process.returncode != 0

    (state_dir / "case-summary.json").write_text(
        json.dumps(result_payload, indent=2, sort_keys=True),
        encoding="utf-8",
    )
    return result_payload


def main() -> int:
    args = parse_args()
    if shutil.which("git") is None:
        print("git not found in PATH", file=sys.stderr)
        return 1
    if shutil.which("script") is None:
        print("script command not found in PATH", file=sys.stderr)
        return 1

    temp_root = Path(tempfile.mkdtemp(prefix=TEMP_ROOT_PREFIX, dir="/tmp"))
    success = False
    summary: dict[str, Any] = {
        "temp_root": str(temp_root),
        "cases": [],
    }

    try:
        summary["cases"].append(
            run_case("commit_direct_without_editor", temp_root / "success-case", args.message, True)
        )
        summary["cases"].append(
            run_case("missing_decision_opens_editor_or_fails", temp_root / "negative-case", args.message, False)
        )
        success = all(case["passed"] for case in summary["cases"])
        summary["passed"] = success
        print(json.dumps(summary, indent=2, sort_keys=True))
        return 0 if success else 1
    finally:
        keep_temp = args.keep_temp or (args.keep_temp_on_failure and not success)
        if keep_temp:
            print(f"kept temp root: {temp_root}", file=sys.stderr)
        else:
            shutil.rmtree(temp_root, ignore_errors=True)


if __name__ == "__main__":
    raise SystemExit(main())