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

`fixtures/crossrepo-latest/` is the real production review 14285 (one
consumer-repo file, 7 hunks, 7 comments, 3 cross-repo impacts). Use it to see
the same related-repo cards, friendly names, and visible route chips the hosted
UI shows, without running a review:

```bash
./uidev -fixtures tools/uidev/fixtures/crossrepo-latest -port 8132
```

`fixtures/crossrepo/` is the same shape but includes a **synthetic** two-hop
chain for exercising the chain UI; real reviews on this diff did not produce a
multi-hop chain. Use it only when iterating on `ChainList`:

```bash
./uidev -fixtures tools/uidev/fixtures/crossrepo -port 8134
```

`fixtures/review-11632-current/` replays review 11632's diff with the current
saturating (`sqrt`) scoring formula, so the Math Mode sqrt and file-level
signal steps can be inspected without running a review:

```bash
./uidev -fixtures tools/uidev/fixtures/review-11632-current -port 8133
```

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
