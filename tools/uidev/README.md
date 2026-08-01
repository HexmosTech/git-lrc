# uidev — design harness for the review UI

> Full documentation: [docs/design-harness.md](../../docs/design-harness.md)

Serves the real Preact review UI from disk against **captured real-review
fixtures**, so the page renders fully populated (big diff, real AI comments,
real blast-radius/review-priority signal data) with zero backend calls.

## Run

```bash
make design-ui          # or: go run ./tools/uidev [--port 8130]
```

Open the printed URL. Then edit anything under `internal/staticserve/static/`
(app.js, components/, styles.css) and just refresh the browser — files are
served from disk, no rebuild.

## Fixtures (`fixtures/`)

| File | Source | Consumed by |
|---|---|---|
| `review-state.json` | `GET /api/review` of a real completed review | the whole app shell |
| `blastradius.json` | `GET /api/blastradius` of the same review | risk sort, score badges, signal panels, risk nav |
| `usage-chip.json` | `GET /api/runtime/usage-chip` (best-effort) | usage banner |

They were captured from a real `lrc review --range HEAD~8...HEAD` run in the
LiveReview repository. To refresh them (triggers a real billed review):

```bash
scripts/capture_design_fixture.sh          # defaults: LiveReview repo, HEAD~8...HEAD
RANGE=HEAD~15...HEAD scripts/capture_design_fixture.sh   # bigger diff
```

## Notes

- The server binds 127.0.0.1 only and does no session checking — any `?r=`
  value works (the default URL uses `?r=design`).
- The events endpoint is stubbed with an empty list; since the fixture review
  is `completed`, the app never polls further.
