#!/usr/bin/env python3
"""Scan a diff file for high-confidence secret patterns and print JSON results.

Usage:
  python scripts/detect_secrets.py --file secret.diff
  cat secret.diff | python scripts/detect_secrets.py

Exit code: 0 = no secrets, 2 = secrets found
"""
import sys
import re
import json
import argparse

PATTERNS = {
    "AWS Access Key ID": re.compile(r"AKIA[0-9A-Z]{16}"),
    "AWS Secret Access Key": re.compile(r"(?<![A-Za-z0-9/+=])[A-Za-z0-9/+=]{40}(?![A-Za-z0-9/+=])"),
    "Google API Key": re.compile(r"AIza[0-9A-Za-z\-_]{35}"),
    "Slack Token": re.compile(r"xox[baprs]-[0-9A-Za-z\-]{10,}"),
    "Private RSA Key Header": re.compile(r"-----BEGIN RSA PRIVATE KEY-----"),
    "Private PEM Key Header": re.compile(r"-----BEGIN PRIVATE KEY-----"),
    "Heroku API Key": re.compile(r"[hH]eroku.?[aA]pi.?key[:=]?\s*[0-9a-fA-F]{32}"),
    # Generic assignment-style key detection (e.g. API_KEY=..., SECRET=...)
    # Capture the assigned value for reporting (group 1).
    "Key Assignment": re.compile(r"\b(?:API_KEY|API|SECRET_KEY|SECRET|ACCESS_KEY|TOKEN|PASSWORD|PWD)\b\s*[:=]\s*([A-Za-z0-9\-_.+/=]{8,200})", re.IGNORECASE),
}


def scan_text(text):
    findings = []
    seen = set()
    for label, pat in PATTERNS.items():
        for m in pat.finditer(text):
            # If regex defines capture groups, prefer the first group's text
            try:
                if m.groups():
                    snippet = m.group(1).strip()
                else:
                    snippet = m.group(0).strip()
            except Exception:
                snippet = m.group(0).strip()
            if not snippet:
                continue
            key = f"{label}|{snippet}"
            if key in seen:
                continue
            seen.add(key)
            findings.append({"label": label, "match": snippet})
    return findings


def main():
    parser = argparse.ArgumentParser(description="Detect secrets in a diff file")
    parser.add_argument("--file", "-f", help="Path to diff file (omits to read stdin)")
    args = parser.parse_args()

    if args.file:
        try:
            raw = open(args.file, "rb").read()
        except Exception as e:
            print(f"Failed to read {args.file}: {e}", file=sys.stderr)
            sys.exit(1)
    else:
        # Read raw bytes from stdin buffer to preserve encoding
        try:
            raw = sys.stdin.buffer.read()
        except Exception:
            # Fallback if buffer not available
            raw = sys.stdin.read().encode("utf-8", errors="ignore")

    # Try decoding intelligently: utf-8, then utf-16 (LE/BE), then latin-1 fallback
    for enc in ("utf-8", "utf-16", "utf-16-le", "utf-16-be", "latin-1"):
        try:
            data = raw.decode(enc)
            # if decoding successful and contains meaningful text, stop
            if len(data) > 0:
                break
        except Exception:
            data = None
            continue
    if data is None:
        print(f"Failed to decode input file with common encodings", file=sys.stderr)
        sys.exit(1)

    findings = scan_text(data)
    out = {"findings": findings, "count": len(findings)}
    print(json.dumps(out))

    if findings:
        # Also print human-friendly to stderr and exit non-zero
        print("High-confidence secret(s) detected:", file=sys.stderr)
        for f in findings:
            print(f"  - {f['label']}: `{f['match']}`", file=sys.stderr)
        sys.exit(2)


if __name__ == "__main__":
    main()
