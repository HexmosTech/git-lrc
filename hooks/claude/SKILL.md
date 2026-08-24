---
name: lrc
description: >
  Manage LiveReview code review for staged changes in this repo. Run /lrc review to open a
  browser-based AI review of staged changes before committing. Use /lrc skip to bypass review and
  write an attestation, or /lrc vouch to manually approve without AI. Check and toggle hook state
  with /lrc hooks status, hooks disable, hooks enable, hooks install, or hooks uninstall. Invoke
  when the user explicitly wants to review code, explicitly skip a review, vouch for changes,
  check if LiveReview is active, turn off hooks, or reinstall the Claude git-commit gate.
argument-hint: "review | skip | vouch | hooks [status|enable|disable|install|uninstall] [--surface claude|git]"
---

# lrc

Run commands from the current repository root. Map $ARGUMENTS to the matching command below.
Translate plain natural-language requests to the nearest command before running.

## Review commands

| Intent | Command |
|--------|---------|
| Review staged changes | `lrc review --staged` |
| Review staged + block until browser decision | `lrc review --staged --blocking-review` |
| Skip review (write attestation, no AI) | `lrc review --staged --skip` |
| Vouch for changes manually | `lrc review --staged --vouch` |
| Review a specific prior commit | `lrc review --commit HEAD` |
| Non-interactive review (agent/CI) | `lrc review --no-serve --output json --staged` |
| Non-interactive range review | `lrc review --no-serve --output json --range main...feature` |

## Hooks commands

| Intent | Command |
|--------|---------|
| Check hook status | `lrc hooks status` |
| Check Claude hook status only | `lrc hooks status --surface claude` |
| Disable all hooks in this repo | `lrc hooks disable` |
| Disable only the Claude gate | `lrc hooks disable --surface claude` |
| Re-enable hooks in this repo | `lrc hooks enable` |
| Re-enable only the Claude gate | `lrc hooks enable --surface claude` |
| Install global Claude hook | `lrc hooks install --surface claude` |
| Remove global Claude hook | `lrc hooks uninstall --surface claude` |

## Rules

- Prefer lrc hooks status before mutating hook state when intent is ambiguous.
- Use `lrc review --staged --skip` only when the user explicitly asks to skip or bypass review. Never use skip as a fallback after a hook, wrapper, or review failure.
- Use `lrc hooks disable` or `lrc hooks disable --surface claude` only when the user explicitly asks to disable hooks. Never disable hooks as a fallback for a failing review flow.
- Repo-local disable/enable uses marker files under .git/lrc/: disabled, disabled-git, disabled-claude.
- Global Claude integration lives in ~/.lrc/claude/hooks/ and ~/.claude/settings.json — manage only via lrc hooks install/uninstall.
- Never edit .claude/settings.local.json directly; it is not the control plane when the global install is active.