# Real-review replay fixtures

These are blast-radius reports from actual `lrc review` runs, kept as
immutable test inputs. They let the scorer and the UI's Math Mode be checked
against what git-lrc really produced, not a hand-built approximation.

| File | Review | Size | Why it is here |
|---|---|---|---|
| `review_11632_report.json` | LiveReview repo, commit `1f92bf94`, 2026-08-23 | 2 files, 4 hunks, 6-symbol hunk | The artifact LiveReview's own Math Mode tests pin their numbers to. Exercises zero-signal, single-symbol, multi-symbol, dynamic step numbering, and every `IsSelf` branch. |
| `review_11632_current_report.json` | Same diff, re-scored by current git-lrc against a v0.9.0 `moderate` index | 2 files, 4 hunks | Captures the current saturating (`sqrt`) combine. Used to prove Math Mode follows the formula the generator actually used. |
| `../../../tools/uidev/fixtures/blastradius.json` | LiveReview repo, review 10566, 2026-08-01 | 31 files, 207 hunks, up to 15 symbols/hunk | The largest captured review. Exercises every hunk shape at scale and backs the design harness. |
| `../../../tools/uidev/fixtures/crossrepo-latest/blastradius.json` | Consumer repo, production review 14285, 2026-11-21 | 1 file, 7 hunks, 3 cross-repo impacts | The real latest cross-repo review. Its artifact predates `SymbolCombine`, so it also covers inferring `sqrt` after a post-scoring cross-repo bonus. |

## Provenance and reproduction

`review_11632_report.json` was copied unchanged from
`LiveReview:internal/blastradius/testdata/review_11632_report.json`, which is
the real S3 artifact for review 11632.

The re-run used the git-lrc code from the review's date (`7982e1f`) only to
build the diff scorer; the checked-in `*_current_report.json` was produced by
the current scorer. Recipe:

```bash
git worktree add --detach /tmp/lr-11632 1f92bf94
cd /tmp/lr-11632
git diff 1f92bf94^ 1f92bf94 > /tmp/lr-11632.diff

# The review used codebase-memory-mcp v0.9.0, which still emitted JSX
# component CALLS edges. Moderate mode also retains the SIMILAR_TO edges the
# duplication signal needs. v0.12.1's fast/full index no longer emits the
# JSX edges, so replaying against a fresh default index legitimately yields
# lower caller counts - the captured report is the ground truth, not a fresh
# index.
CBM_CACHE_DIR=/tmp/cbm-v090-cache \
  codebase-memory-mcp cli index_repository \
  --repo-path /tmp/lr-11632 --mode moderate --name lr11632v090mod

blastradius score --project lr11632v090mod --diff /tmp/lr-11632.diff
```

With that index every per-symbol score reproduces the original **exactly**.
The only delta in the saved re-run is the hunk's
`File co-change coupling` value (0.2456 vs 0.2598), because the worktree's
git history state is not byte-identical to the reviewing machine's. That is
why the re-run is a separate fixture: the original remains the authority for
"what the review showed", and the re-run is the authority for "what the
current formula computes".

## What these tests protect

1. **Formula parity.** For every hunk, `raw == rollup(symbols) + hunk
   signals`. Historical reports use `sum`; current reports use
   `sqrt(sum)`. A mismatch means the scorer and the Math Mode UI disagree.
2. **UI parity.** `blast_math_model.mjs` must derive the same totals, step
   numbers, and symbol order that the report recorded.
3. **No silent reinterpretation.** The legacy fixture has no
   `SymbolCombine` field. If the detector ever starts treating it as
   `sqrt`, the pinned totals in the tests fail.
4. **Cross-repo bonus inference.** Review 14285's impacted hunks have an
   8-point bonus added after the sqrt roll-up, making the raw score look
   closer to a linear sum. Detection must discount that bonus, and the Math
   Mode must show the adjustment step rather than misreading the hunk.
