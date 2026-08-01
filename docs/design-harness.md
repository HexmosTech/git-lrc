# Review UI Design Harness

A local harness for iterating on the review UI design against a **fully populated, realistic
review** — big diff, real AI comments, real blast-radius/review-priority data — with no backend,
no billing, and no rebuild between edits.

## Why it exists

Fake-mode reviews (`make dev-ui` / `scripts/fake_review.sh`) exercise the plumbing but produce a
tiny sandbox diff with a handful of hardcoded synthetic comments — nowhere near enough content to
judge visual design, ranking behavior, or information density. The harness replays a **captured
real review** instead: every badge, signal panel, sort mode, and comment thread renders exactly as
it would for a real user.

## Quick start

```bash
make design-ui          # or: go run ./tools/uidev [--port 8130]
# open http://127.0.0.1:8130/?r=design
```

Then edit anything under `internal/staticserve/static/` — `app.js`, `components/*`, `styles.css` —
and **refresh the browser**. Files are served from disk (`LRC_STATIC_DEV_DIR` mechanism), so no
rebuild is needed. Stop/restart the server only when you change the fixtures or the server itself.

## How it works

```
scripts/capture_design_fixture.sh          (run once / when refreshing fixtures)
    └─ runs a REAL `lrc review --range …` in a big repo, waits for completion,
       then snapshots the exact JSON payloads the browser consumes:
         tools/uidev/fixtures/review-state.json   ← GET /api/review
         tools/uidev/fixtures/blastradius.json    ← GET /api/blastradius
         tools/uidev/fixtures/usage-chip.json     ← GET /api/runtime/usage-chip

tools/uidev/main.go                        (the replay server, make design-ui)
    ├─ /static/*            → internal/staticserve/static from DISK (no-store)
    ├─ /                    → index.html (app shell)
    ├─ /api/review          → review-state.json
    ├─ /api/blastradius     → blastradius.json
    ├─ /api/runtime/usage-chip → usage-chip.json
    └─ /api/v1/diff-review/*/events → {"events":[]}   (stub)
```

Because the fixture review has `status: "completed"`, the frontend renders the final state
immediately and never polls the events stream or re-fetches from the backend. The blast report is
joined onto hunks client-side by hunk key (`filePath:newStart:newLines`), the same way it is in a
live review.

## The fixtures

Captured from a real `lrc review --range HEAD~8...HEAD` of the LiveReview repository:
31 files, 207 hunks (all scored), 60 real AI comments, 45 hunks with per-symbol signal
breakdowns. They are committed so the harness works from a fresh clone.

To refresh them (note: this triggers a **real billed review** against the configured backend):

```bash
scripts/capture_design_fixture.sh                       # defaults below
RANGE=HEAD~15...HEAD scripts/capture_design_fixture.sh  # bigger diff (~108 files)
REPO=/path/to/repo PORT=8140 scripts/capture_design_fixture.sh
```

Defaults: `REPO=/home/shrsv/bin/LiveReview`, `RANGE=HEAD~8...HEAD`, `PORT=8130`,
`OUT=tools/uidev/fixtures`, 10-minute timeout. The script builds lrc from the working tree, so
captured payloads always match the current server-side field shapes.

## What the UI should show (current design intent)

- **Default view — "Risk Score (whole)"**: pure hunk-based ranked stream; file boundaries
  dissolve and every hunk renders as its own block, ordered highest→lowest Combined score. One
  scroll goes from the riskiest change to the most trivial.
- **Sidebar**: still lists files, and in the ranked view each file expands into `Hunk 1, Hunk 2…`
  entries (with score chips) that jump to that hunk's block in the stream.
- **Order By control** (toolbar): one grouped panel — `Risk Score (whole)` / `Risk Score (per
  file)` / `Natural`. Natural is the classic file+hunk diff-order view, fully preserved.
- **Comment navigator** (bottom right): always navigates **comments** — there is no separate
  hunk-navigation mode. The traversal order simply follows the active view, because the comment
  list is derived from the displayed order: risk view ⇒ comments visited highest-risk-hunk first;
  natural view ⇒ diff order.
- **Score badges**: each hunk header shows its 0-100 Combined score; clicking opens the
  "why this score" panel (signal list, Blast/Priority dimensions, hygiene dampener, per-symbol
  contributions).

## Known harness quirks

- The usage chip renders its "not authenticated" state — that's literally what the endpoint
  returned at capture time; replace `usage-chip.json` with a real payload if you need to style the
  authenticated chip.
- Only the completed-review state is exercised. The in-progress/streaming experience still needs
  `make dev-ui` (fake mode) or a real review.
- The server binds 127.0.0.1 and does no session checking; any `?r=` value works.

## Related

- `tools/uidev/README.md` — short operational notes next to the code.
- `make dev-ui` — fake-mode sandbox with live JS reload (streaming/in-progress states).
- `blastradius/explorer/` — standalone explorer for raw blastradius reports (scoring-methodology
  work, separate from the product review UI).
