#!/usr/bin/env python3
"""Publish a GitHub release from markdown notes with auto-version inference.

Usage:
  python3 scripts/release_gh.py --repo HexmosTech/git-lrc [--version vX.Y.Z]
"""

from __future__ import annotations

import argparse
import pathlib
import re
import subprocess
import sys
from typing import Iterable, Optional, Tuple

SEMVER_RE = re.compile(r"^v(\d+)\.(\d+)\.(\d+)$")


def run(cmd: Iterable[str], check: bool = True) -> str:
    result = subprocess.run(list(cmd), check=check, text=True, capture_output=True)
    return result.stdout.strip()


def parse_semver(version: str) -> Optional[Tuple[int, int, int]]:
    match = SEMVER_RE.match(version)
    if not match:
        return None
    return tuple(int(x) for x in match.groups())


def validate_version(version: str) -> str:
    if not version.startswith("v"):
        version = f"v{version}"
    if not SEMVER_RE.match(version):
        raise ValueError(f"invalid version '{version}' (expected vX.Y.Z)")
    return version


def semver_max(versions: Iterable[str]) -> Optional[str]:
    valid = [(parse_semver(v), v) for v in versions]
    valid = [(k, v) for (k, v) in valid if k is not None]
    if not valid:
        return None
    valid.sort(key=lambda item: item[0])
    return valid[-1][1]


def infer_from_head_tags() -> Optional[str]:
    tags = run(["git", "tag", "--points-at", "HEAD"], check=False)
    if not tags:
        return None
    return semver_max([t.strip() for t in tags.splitlines() if t.strip()])


def infer_from_main_go() -> Optional[str]:
    main_go = pathlib.Path("main.go")
    if not main_go.exists():
        return None
    pattern = re.compile(r'^const\\s+appVersion\\s*=\\s*"([^"]+)"')
    for line in main_go.read_text(encoding="utf-8").splitlines():
        match = pattern.match(line.strip())
        if match:
            candidate = match.group(1)
            try:
                return validate_version(candidate)
            except ValueError:
                return None
    return None


def infer_version(explicit: Optional[str]) -> str:
    if explicit:
        return validate_version(explicit)

    from_head = infer_from_head_tags()
    if from_head:
        return validate_version(from_head)

    from_source = infer_from_main_go()
    if from_source:
        return from_source

    raise ValueError(
        "unable to infer version automatically; pass --version vX.Y.Z"
    )


def ensure_local_tag(version: str) -> None:
    exists = subprocess.run(
        ["git", "rev-parse", "-q", "--verify", f"refs/tags/{version}"],
        check=False,
        text=True,
        capture_output=True,
    ).returncode == 0
    if exists:
        return
    run(["git", "tag", "-a", version, "-m", f"Release {version}"])


def ensure_remote_tag(version: str) -> None:
    result = subprocess.run(["git", "push", "origin", version], check=False)
    if result.returncode != 0:
        raise RuntimeError(f"failed to push tag {version} to origin")


def release_exists(repo: str, version: str) -> bool:
    result = subprocess.run(
        ["gh", "release", "view", version, "--repo", repo],
        check=False,
        text=True,
        capture_output=True,
    )
    return result.returncode == 0


def publish_release(repo: str, version: str, notes_file: pathlib.Path) -> None:
    if release_exists(repo, version):
        run(
            [
                "gh",
                "release",
                "edit",
                version,
                "--repo",
                repo,
                "--title",
                version,
                "--notes-file",
                str(notes_file),
            ]
        )
        return

    run(
        [
            "gh",
            "release",
            "create",
            version,
            "--repo",
            repo,
            "--title",
            version,
            "--notes-file",
            str(notes_file),
            "--verify-tag",
        ]
    )


def main() -> int:
    parser = argparse.ArgumentParser(description="Publish GitHub release from markdown notes")
    parser.add_argument("--repo", required=True, help="GitHub repo in owner/name form")
    parser.add_argument("--version", help="Version to publish, e.g. v1.2.3")
    args = parser.parse_args()

    try:
        version = infer_version(args.version)
    except ValueError as exc:
        print(f"❌ {exc}")
        return 2

    notes_file = pathlib.Path("docs") / "releases" / f"{version}.md"
    if not notes_file.exists() or notes_file.stat().st_size == 0:
        print(f"❌ Missing release notes file: {notes_file}")
        print(f"   Fix: make release-notes-init VERSION={version}")
        return 2

    try:
        ensure_local_tag(version)
        ensure_remote_tag(version)
        publish_release(args.repo, version, notes_file)
    except Exception as exc:  # noqa: BLE001
        print(f"❌ Failed to publish GitHub release: {exc}")
        return 1

    print(f"✅ Published GitHub release: {version}")
    print("ℹ️  SBOM generation+attachment will run from the release tag in CI.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
