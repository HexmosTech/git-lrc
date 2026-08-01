// Fixture-driven integration test for the blast-radius UI pipeline.
//
// The pure-logic helpers in blast_radius_sort_state.test.mjs (attach, flatten,
// sort, summarize) use small synthetic fixtures — this test exercises the FULL
// data path against the real captured payloads the review UI consumes, the
// same ones served by `make design-ui` (tools/uidev/fixtures/):
//
//   review-state.json  ← GET /api/review      (snake_case, raw wire shape)
//   blastradius.json   ← GET /api/blastradius ({status, report}, camelCase)
//
// It reproduces the inlined /api/review → UI transformation app.js performs
// (file_path → FilePath, hunks → Hunks, new_start_line → NewStartLine, ...),
// then runs the same join + flatten + sort + hunkNav build app.js does, and
// asserts the invariants that catch real regressions:
//   - every /api/review hunk finds a /api/blastradius match (no orphan hunks);
//   - every joined hunk carries BlastRadius + BlastDetail;
//   - flattenFilesByRisk yields exactly one ranked entry per hunk, in
//     descending-score order, with RiskRank / SourceHunkNumber / ExpandKey
//     populated exactly as Sidebar.js / DiffTable.js expect;
//   - the hunkNav built for the Sidebar in flat-risk mode covers every file
//     and every hunk.

import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

import {
    attachBlastData,
    buildBlastLookup,
    flattenFilesByRisk,
    hasBlastRadiusData,
    hunkBlastKey,
    sortFilesByBlastRadius,
} from './blast_radius_sort_state.mjs';

const FIXTURES = path.resolve(import.meta.dirname, '../../../../tools/uidev/fixtures');
const reviewState = JSON.parse(fs.readFileSync(path.join(FIXTURES, 'review-state.json'), 'utf8'));
const blastPayload = JSON.parse(fs.readFileSync(path.join(FIXTURES, 'blastradius.json'), 'utf8'));

// Minimal mirror of app.js's inline transform: raw /api/review shape (snake)
// → the camelCase UI shape DiffTable/Sidebar/etc read. Only the fields the
// blast pipeline needs are normalized (FilePath, Hunks[].NewStartLine,
// NewLineCount, Lines) — see app.js:115-208 for the canonical version.
function transformReviewToUIFiles(reviewState) {
    return (reviewState.files || []).map((file) => ({
        FilePath: file.file_path,
        ID: file.file_path, // filePathToId-side equivalence for test keys
        HasComments: Array.isArray(file.comments) && file.comments.length > 0,
        CommentCount: Array.isArray(file.comments) ? file.comments.length : 0,
        Hunks: (file.hunks || []).map((hunk) => {
            const newStartLine = hunk.new_start_line || hunk.NewStartLine || 1;
            const newLineCount = hunk.new_line_count || hunk.NewLineCount || 0;
            // Light Lines shape: enough for hunkCommentCount to work in helpers.
            const lines = (hunk.content || hunk.Content || '')
                .split('\n')
                .filter((l) => l && !l.startsWith('@@'))
                .map((l) => ({
                    NewNum: l.startsWith('-') ? '' : '0',
                    OldNum: l.startsWith('+') ? '' : '0',
                    IsComment: false,
                    Comments: [],
                }));
            return {
                Header: hunk.header || hunk.Header || '',
                Lines: lines,
                NewStartLine: newStartLine,
                NewLineCount: newLineCount,
            };
        }),
    }));
}

test('fixture files load with the expected top-level shape', () => {
    assert.ok(Array.isArray(reviewState.files) && reviewState.files.length > 0,
        'review-state.json must contain files[] for the join to mean anything');
    assert.equal(blastPayload.status, 'ready');
    assert.ok(blastPayload.report && Array.isArray(blastPayload.report.Files));
});

test('every /api/review hunk joins to a /api/blastradius hunk via hunkBlastKey', () => {
    const reviewUIFiles = transformReviewToUIFiles(reviewState);
    const lookup = buildBlastLookup(blastPayload.report);
    let total = 0, joined = 0;
    reviewUIFiles.forEach((file) => {
        file.Hunks.forEach((hunk) => {
            total++;
            if (lookup.has(hunkBlastKey(file.FilePath, hunk.NewStartLine, hunk.NewLineCount))) {
                joined++;
            }
        });
    });
    assert.equal(total, joined,
        `${joined}/${total} hunks joined — every review hunk must have a blast match`);
});

test('attachBlastData stamps BlastRadius + BlastDetail on every joined hunk', () => {
    const reviewUIFiles = transformReviewToUIFiles(reviewState);
    const lookup = buildBlastLookup(blastPayload.report);
    const attached = attachBlastData(reviewUIFiles, lookup);
    assert.ok(hasBlastRadiusData(attached), 'hasBlastRadiusData must be true once data is attached');
    let withDetail = 0, withScore = 0;
    attached.forEach((file) => {
        file.Hunks.forEach((hunk) => {
            if (hunk.BlastDetail) withDetail++;
            if (typeof hunk.BlastRadius === 'number') withScore++;
        });
    });
    assert.ok(withDetail > 0, 'at least one hunk must carry a BlastDetail');
    assert.equal(withDetail, withScore, 'BlastDetail and BlastRadius go together');
});

test('flattenFilesByRisk yields one entry per hunk, ranked descending by score', () => {
    const reviewUIFiles = transformReviewToUIFiles(reviewState);
    const lookup = buildBlastLookup(blastPayload.report);
    const attached = attachBlastData(reviewUIFiles, lookup);
    const totalHunks = attached.reduce((n, f) => n + f.Hunks.length, 0);

    const flat = flattenFilesByRisk(attached);
    assert.equal(flat.length, totalHunks, 'flat stream has exactly one entry per hunk');

    // Required Sidebar/DiffTable hook fields.
    flat.forEach((entry, i) => {
        assert.ok(entry.ID, `entry ${i} missing ID`);
        assert.equal(typeof entry.RiskRank, 'number', `entry ${i} missing RiskRank`);
        assert.ok(entry.ExpandKey, `entry ${i} missing ExpandKey`);
        assert.equal(typeof entry.SourceHunkNumber, 'number', `entry ${i} missing SourceHunkNumber`);
        assert.equal(entry.Hunks.length, 1, `entry ${i} must hold exactly one hunk`);
        assert.equal(entry.SyntheticHunk, true);
    });

    // Descending-score order with scored-hunks-first, unscored after.
    const scores = flat.map((e) => e.Hunks[0].BlastRadius ?? null);
    let lastScore = Infinity;
    let crossedIntoUnscored = false;
    scores.forEach((s) => {
        if (s === null) {
            crossedIntoUnscored = true;
            return;
        }
        assert.ok(!crossedIntoUnscored,
            `scored hunk (${s}) appeared after an unscored one — sort contract broken`);
        assert.ok(s <= lastScore, `scores must be non-increasing: ${s} > ${lastScore}`);
        lastScore = s;
    });
    // RiskRank is 1-based, monotonic.
    flat.forEach((entry, i) => {
        assert.equal(entry.RiskRank, i + 1);
    });
});

test('hunkNav (as built by app.js in flat-risk mode) covers every file and hunk', () => {
    const reviewUIFiles = transformReviewToUIFiles(reviewState);
    const lookup = buildBlastLookup(blastPayload.report);
    const attached = attachBlastData(reviewUIFiles, lookup);
    const flat = flattenFilesByRisk(attached);

    // Mirror of app.js:912-928.
    const hunkNav = {};
    flat.forEach((entry) => {
        (hunkNav[entry.FilePath] = hunkNav[entry.FilePath] || []).push({
            targetId: entry.ID,
            expandKey: entry.ExpandKey,
            hunkNum: entry.SourceHunkNumber,
            score: entry.Hunks[0]?.BlastRadius ?? null,
        });
    });
    Object.values(hunkNav).forEach((list) => list.sort((a, b) => a.hunkNum - b.hunkNum));

    // Every original file must have an entry, and per-file hunk counts must
    // match the original files' hunk counts.
    assert.equal(Object.keys(hunkNav).length, reviewUIFiles.length,
        'every file should appear in hunkNav');
    reviewUIFiles.forEach((file) => {
        const nav = hunkNav[file.FilePath];
        assert.ok(nav, `missing hunkNav for ${file.FilePath}`);
        assert.equal(nav.length, file.Hunks.length,
            `hunkNav count mismatch for ${file.FilePath}`);
        // hunk numbers are 1-based, sorted ascending and contiguous.
        nav.forEach((entry, i) => {
            assert.equal(entry.hunkNum, i + 1);
            assert.ok(entry.targetId);
            assert.ok(entry.expandKey);
        });
    });
});

test('sortFilesByBlastRadius reorders hunks within each file but keeps file set', () => {
    const reviewUIFiles = transformReviewToUIFiles(reviewState);
    const lookup = buildBlastLookup(blastPayload.report);
    const attached = attachBlastData(reviewUIFiles, lookup);
    const sorted = sortFilesByBlastRadius(attached);
    assert.equal(sorted.length, reviewUIFiles.length, 'file count unchanged');
    sorted.forEach((file, i) => {
        assert.equal(file.Hunks.length, reviewUIFiles[i].Hunks.length,
            `hunk count must be preserved per file (${file.FilePath})`);
        // Within-file scores must be non-increasing.
        let last = Infinity;
        file.Hunks.forEach((h) => {
            const s = typeof h.BlastRadius === 'number' ? h.BlastRadius : -Infinity;
            assert.ok(s <= last, `within-file sort broken in ${file.FilePath}`);
            last = s;
        });
    });
});

test('degradation: empty blast lookup leaves hunks untouched and falls back to diff order', () => {
    const reviewUIFiles = transformReviewToUIFiles(reviewState);
    const untouched = attachBlastData(reviewUIFiles, new Map());
    // Zero hunk has BlastDetail — the engine is "missing", exactly like a
    // real review running without `lrc graph install` having been run.
    untouched.forEach((file) => {
        file.Hunks.forEach((h) => assert.equal(h.BlastDetail, undefined));
    });
    assert.equal(untouched, reviewUIFiles, 'empty lookup must short-circuit (return same array)');
    // flattenFilesByRisk with no scores: every hunk gets a RiskRank, in diff
    // order (unscored hunks retain their original sequence). Sidebar still
    // navigates them; the user just sees risk unranked, not a broken UI.
    const flat = flattenFilesByRisk(untouched);
    assert.equal(flat.length, reviewUIFiles.reduce((n, f) => n + f.Hunks.length, 0));
});