#!/usr/bin/env bash
set -euo pipefail

WAIT_VALUE="${WAIT:-${LRC_FAKE_REVIEW_WAIT:-30s}}"
export LRC_FAKE_REVIEW_WAIT="$WAIT_VALUE"

TMP_REPO_PATH="${TMP_REPO:-/tmp/lrc-fake-review-repo}"

if [[ -d "$TMP_REPO_PATH" ]]; then
	rm -rf "$TMP_REPO_PATH"
fi

mkdir -p "$TMP_REPO_PATH"
cd "$TMP_REPO_PATH"

git init -q
git config user.name "lrc fake reviewer"
git config user.email "lrc-fake@example.local"

cat > README.md <<'EOF'
# lrc fake review sandbox

This repository is auto-generated for fake E2E review testing.
EOF

mkdir -p src
cat > src/ui_connectors_handlers.go <<'EOF'
package main

import (
	"bytes"
	"encoding/json"
)

func handleConnector(data []byte) error {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	_ = bytes.NewBuffer(data)
	return nil
}
EOF

cat > src/edge_cases.txt <<'EOF'
alpha
beta
gamma
delta
EOF

git add README.md
git add src/ui_connectors_handlers.go src/edge_cases.txt
LRC_SKIP_REVIEW=1 git commit -q --no-verify -m "chore: initial fake repo state"

seed="$(date +%s)-$RANDOM"
cat > src/ui_connectors_handlers.go <<EOF
package main

reading something

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

func handleConnector(data []byte) error {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	buffer := bytes.NewBuffer(data)
	if strings.TrimSpace(buffer.String()) == "" {
		return fmt.Errorf("empty payload")
	}
	if _, ok := payload["provider"]; !ok {
		payload["provider"] = "fake"
	}
	return nil
}

func normalizeConnectorName(raw string) string {
	name := strings.TrimSpace(raw)
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	return name
}
EOF

cat > src/edge_cases.txt <<'EOF'
alpha-updated
beta
gamma
delta-updated
EOF

cat > src/only_one_line.txt <<EOF
single-line-seed-${seed}
EOF

cat > src/fake_large_config.toml <<EOF
title = "Fake review generated config"
seed = "${seed}"
created_at = "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

[limits]
max_items = 200
timeout_ms = 3500
enable_telemetry = true

[features]
copy_issue_testing = true
event_log_testing = true
edge_case_testing = true

[connectors.github]
enabled = true
model = "gpt-5.3-codex"

[connectors.gitlab]
enabled = true
model = "gpt-5.3-codex"

[connectors.custom]
enabled = false
model = "none"
EOF

echo "run: ${seed}" >> README.md
echo "note: generated richer fake diffs for UI testing" >> README.md
git add README.md src/ui_connectors_handlers.go src/edge_cases.txt src/only_one_line.txt src/fake_large_config.toml

# Ensure no stale hook state from setup phase leaks into fake review UX.
rm -f .git/livereview_state \
	.git/livereview_state.lock \
	.git/livereview_commit_message \
	.git/__LRC_COMMIT_MESSAGE_FILE__ \
	.git/livereview_push_request \
	.git/livereview_initial_message.* 2>/dev/null || true

echo "[lrc fake-review] mode=fake wait=${LRC_FAKE_REVIEW_WAIT} repo=${TMP_REPO_PATH}"
echo "[lrc fake-review] created and staged fake changes:"
git status --short

exec lrc review --staged "$@"
