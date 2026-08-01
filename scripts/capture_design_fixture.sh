#!/usr/bin/env bash
# Captures the two JSON payloads the review UI consumes - /api/review and
# /api/blastradius - from a REAL review run on a big diff, for use as design
# fixtures served by `go run ./tools/uidev` (make design-ui).
#
# This triggers an actual billed review against the configured LiveReview
# backend. Re-run only when the fixtures need refreshing.
#
# Env overrides:
#   REPO   - repository to review           (default /home/shrsv/bin/LiveReview)
#   RANGE  - git range for the diff         (default HEAD~8...HEAD)
#   PORT   - local serve port               (default 8130)
#   LRC    - lrc binary to use              (default: build from this repo)
#   OUT    - fixtures output dir            (default tools/uidev/fixtures)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

REPO="${REPO:-/home/shrsv/bin/LiveReview}"
RANGE="${RANGE:-HEAD~8...HEAD}"
PORT="${PORT:-8130}"
OUT="${OUT:-$REPO_ROOT/tools/uidev/fixtures}"
TIMEOUT_SECS="${TIMEOUT_SECS:-600}"

LOG="$(mktemp /tmp/lrc-capture-XXXX.log)"

if [[ -z "${LRC:-}" ]]; then
    LRC="$(mktemp /tmp/lrc-capture-bin-XXXX)"
    echo "Building lrc from $REPO_ROOT ..."
    (cd "$REPO_ROOT" && go build -o "$LRC" .)
fi

mkdir -p "$OUT"

echo "Starting review of $RANGE in $REPO on port $PORT (log: $LOG)"
(cd "$REPO" && "$LRC" review --range "$RANGE" --port "$PORT" >"$LOG" 2>&1) &
REVIEW_PID=$!
cleanup() {
    kill "$REVIEW_PID" 2>/dev/null || true
}
trap cleanup EXIT

# Extract the session token (?r=<reviewID>) from the serve banner.
RID=""
for _ in $(seq 1 60); do
    RID="$(grep -oE 'r=[A-Za-z0-9_-]+' "$LOG" | head -1 | cut -d= -f2 || true)"
    [[ -n "$RID" ]] && break
    if ! kill -0 "$REVIEW_PID" 2>/dev/null; then
        echo "Review process exited early; log follows:" >&2
        cat "$LOG" >&2
        exit 1
    fi
    sleep 1
done
if [[ -z "$RID" ]]; then
    echo "Could not find review session id in log:" >&2
    cat "$LOG" >&2
    exit 1
fi
echo "Review session: $RID"

BASE="http://localhost:$PORT"

# Wait for the review to complete AND the blast report to be ready (or
# declared unavailable), bounded by TIMEOUT_SECS.
deadline=$(( $(date +%s) + TIMEOUT_SECS ))
while :; do
    review_status="$(curl -sf "$BASE/api/review?r=$RID" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("status",""))' 2>/dev/null || echo "")"
    blast_status="$(curl -sf "$BASE/api/blastradius?r=$RID" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("status",""))' 2>/dev/null || echo "")"
    echo "  review=$review_status blast=$blast_status"
    if [[ "$review_status" == "completed" && ( "$blast_status" == "ready" || "$blast_status" == "unavailable" ) ]]; then
        break
    fi
    if [[ "$review_status" == "failed" ]]; then
        echo "Review failed; log follows:" >&2
        tail -40 "$LOG" >&2
        exit 1
    fi
    if (( $(date +%s) > deadline )); then
        echo "Timed out after ${TIMEOUT_SECS}s (review=$review_status blast=$blast_status)" >&2
        exit 1
    fi
    sleep 5
done

pretty() { python3 -m json.tool; }

curl -sf "$BASE/api/review?r=$RID" | pretty > "$OUT/review-state.json"
curl -sf "$BASE/api/blastradius?r=$RID" | pretty > "$OUT/blastradius.json"
curl -sf "$BASE/api/runtime/usage-chip?r=$RID" | pretty > "$OUT/usage-chip.json" || echo '{}' > "$OUT/usage-chip.json"

echo "Saved fixtures to $OUT:"
ls -la "$OUT"

python3 - "$OUT" <<'EOF'
import json, sys, os
out = sys.argv[1]
rs = json.load(open(os.path.join(out, 'review-state.json')))
files = rs.get('files') or []
comments = sum(len(f.get('comments') or []) for f in files)
hunks = sum(len(f.get('hunks') or []) for f in files)
scored = sum(1 for f in files for h in (f.get('hunks') or []) if h.get('blast_radius') is not None)
print(f"review-state: status={rs.get('status')} files={len(files)} hunks={hunks} comments={comments} scored_hunks={scored}")
b = json.load(open(os.path.join(out, 'blastradius.json')))
r = b.get('report') or {}
bh = [h for f in (r.get('Files') or []) for h in (f.get('Hunks') or [])]
with_symbols = sum(1 for h in bh if h.get('Symbols'))
print(f"blastradius: status={b.get('status')} hunks={len(bh)} hunks_with_symbols={with_symbols}")
EOF
