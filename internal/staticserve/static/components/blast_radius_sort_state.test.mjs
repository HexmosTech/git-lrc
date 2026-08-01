import test from 'node:test';
import assert from 'node:assert/strict';

import {
    attachBlastData,
    blastRadiusTierLabel,
    buildBlastLookup,
    flattenFilesByRisk,
    hasBlastRadiusData,
    hunkBlastKey,
    sortFilesByBlastRadius,
    summarizeRiskDetail,
    sortHunksByBlastRadius,
} from './blast_radius_sort_state.mjs';

test('hasBlastRadiusData is false with no files or no scores', () => {
    assert.equal(hasBlastRadiusData(null), false);
    assert.equal(hasBlastRadiusData([]), false);
    assert.equal(hasBlastRadiusData([{ Hunks: [{ Header: '@@' }] }]), false);
});

test('hasBlastRadiusData is true when any hunk has a score', () => {
    const files = [
        { Hunks: [{ Header: '@@' }] },
        { Hunks: [{ Header: '@@', BlastRadius: 0 }] },
    ];
    assert.equal(hasBlastRadiusData(files), true);
});

test('sortHunksByBlastRadius orders scored hunks descending, unscored last', () => {
    const hunks = [
        { id: 'low', BlastRadius: 5 },
        { id: 'unscored' },
        { id: 'high', BlastRadius: 90 },
        { id: 'mid', BlastRadius: 42 },
    ];
    const sorted = sortHunksByBlastRadius(hunks);
    assert.deepEqual(sorted.map((h) => h.id), ['high', 'mid', 'low', 'unscored']);
});

test('sortHunksByBlastRadius preserves relative order among unscored hunks', () => {
    const hunks = [{ id: 'a' }, { id: 'b' }, { id: 'c', BlastRadius: 1 }];
    const sorted = sortHunksByBlastRadius(hunks);
    assert.deepEqual(sorted.map((h) => h.id), ['c', 'a', 'b']);
});

test('sortHunksByBlastRadius does not mutate the input array', () => {
    const hunks = [{ id: 'a', BlastRadius: 1 }, { id: 'b', BlastRadius: 99 }];
    const original = [...hunks];
    sortHunksByBlastRadius(hunks);
    assert.deepEqual(hunks, original);
});

test('sortFilesByBlastRadius sorts each file\'s hunks and keeps file order', () => {
    const files = [
        { FilePath: 'a.go', Hunks: [{ id: 1, BlastRadius: 1 }, { id: 2, BlastRadius: 50 }] },
        { FilePath: 'b.go', Hunks: [{ id: 3, BlastRadius: 10 }] },
    ];
    const sorted = sortFilesByBlastRadius(files);
    assert.deepEqual(sorted.map((f) => f.FilePath), ['a.go', 'b.go']);
    assert.deepEqual(sorted[0].Hunks.map((h) => h.id), [2, 1]);
});

test('buildBlastLookup keys report hunks by path:start:lines', () => {
    const report = {
        Files: [
            { Path: 'a.go', Hunks: [{ NewStart: 5, NewLines: 3, Combined: 80 }] },
            { Path: 'b.go', Hunks: [{ NewStart: 1, NewLines: 1, Combined: 10 }] },
        ],
    };
    const lookup = buildBlastLookup(report);
    assert.equal(lookup.size, 2);
    assert.equal(lookup.get(hunkBlastKey('a.go', 5, 3)).Combined, 80);
    assert.equal(buildBlastLookup(null).size, 0);
});

test('attachBlastData joins scores and detail onto matching hunks only', () => {
    const files = [
        {
            FilePath: 'a.go',
            Hunks: [
                { NewStartLine: 5, NewLineCount: 3 },
                { NewStartLine: 40, NewLineCount: 2, BlastRadius: 33 }, // server-stamped, no lookup entry
            ],
        },
    ];
    const lookup = buildBlastLookup({
        Files: [{ Path: 'a.go', Hunks: [{ NewStart: 5, NewLines: 3, Combined: 77.5, Signals: [{ Name: 'x' }] }] }],
    });
    const joined = attachBlastData(files, lookup);
    assert.equal(joined[0].Hunks[0].BlastRadius, 77.5);
    assert.equal(joined[0].Hunks[0].BlastDetail.Signals.length, 1);
    assert.equal(joined[0].Hunks[1].BlastRadius, 33);
    assert.equal(joined[0].Hunks[1].BlastDetail, undefined);
    // Inputs must not be mutated.
    assert.equal(files[0].Hunks[0].BlastRadius, undefined);
});

test('attachBlastData with empty lookup returns files unchanged', () => {
    const files = [{ FilePath: 'a.go', Hunks: [{ NewStartLine: 1, NewLineCount: 1 }] }];
    assert.equal(attachBlastData(files, new Map()), files);
});

test('flattenFilesByRisk ranks hunks globally across files', () => {
    const files = [
        {
            ID: 'file-a', FilePath: 'a.go',
            Hunks: [
                { id: 'a1', BlastRadius: 10, Lines: [] },
                { id: 'a2', BlastRadius: 95, Lines: [] },
            ],
        },
        { ID: 'file-b', FilePath: 'b.go', Hunks: [{ id: 'b1', BlastRadius: 50, Lines: [] }] },
        { ID: 'file-c', FilePath: 'c.go', Hunks: [{ id: 'c1', Lines: [] }] }, // unscored
    ];
    const flat = flattenFilesByRisk(files);
    assert.deepEqual(flat.map((f) => f.Hunks[0].id), ['a2', 'b1', 'a1', 'c1']);
    assert.deepEqual(flat.map((f) => f.RiskRank), [1, 2, 3, 4]);
    // Every entry is a single-hunk pseudo-file keyed back to its real file.
    assert.equal(flat[0].ExpandKey, 'file-a');
    assert.equal(flat[1].ExpandKey, 'file-b');
    assert.ok(flat.every((f) => f.Hunks.length === 1 && f.SyntheticHunk));
    // Synthetic IDs must be unique.
    assert.equal(new Set(flat.map((f) => f.ID)).size, flat.length);
});

test('flattenFilesByRisk computes per-hunk comment counts', () => {
    const files = [
        {
            ID: 'f', FilePath: 'f.go',
            Hunks: [
                { BlastRadius: 5, Lines: [{ IsComment: true, Comments: [{}, {}] }, { IsComment: false }] },
                { BlastRadius: 9, Lines: [{ IsComment: false }] },
            ],
        },
    ];
    const flat = flattenFilesByRisk(files);
    assert.equal(flat[0].CommentCount, 0); // score 9 hunk first, no comments
    assert.equal(flat[1].CommentCount, 2);
    assert.equal(flat[1].HasComments, true);
});

test('flattenFilesByRisk keeps diff order among equal or unscored hunks', () => {
    const files = [
        { ID: 'a', FilePath: 'a.go', Hunks: [{ id: 'a1', Lines: [] }, { id: 'a2', Lines: [] }] },
        { ID: 'b', FilePath: 'b.go', Hunks: [{ id: 'b1', Lines: [] }] },
    ];
    const flat = flattenFilesByRisk(files);
    assert.deepEqual(flat.map((f) => f.Hunks[0].id), ['a1', 'a2', 'b1']);
});

test('summarizeRiskDetail merges hunk and symbol signals ranked by |points|', () => {
    const detail = {
        Combined: 73.3,
        BlastRadiusNorm: 88,
        ReviewPriorityNorm: 51,
        HygieneMultiplier: 1,
        Signals: [{ Name: 'Persistence layer', Points: 1.5 }],
        Symbols: [
            { Signals: [{ Name: 'Caller reach', Points: 9.2 }, { Name: 'Test coverage', Points: 3.0 }] },
            { Signals: [{ Name: 'Formatting only', Points: -0.95 }] },
        ],
    };
    const s = summarizeRiskDetail(detail, 2);
    assert.equal(s.score, 73.3);
    assert.equal(s.blast, 88);
    assert.equal(s.priority, 51);
    assert.equal(s.hygiene, null);
    assert.deepEqual(s.top.map((x) => x.Name), ['Caller reach', 'Test coverage']);
    assert.equal(s.moreCount, 2);
    assert.equal(s.totalSignals, 4);
});

test('summarizeRiskDetail reports an active hygiene dampener', () => {
    const s = summarizeRiskDetail({ Combined: 5, HygieneMultiplier: 0.05, Signals: [] });
    assert.equal(s.hygiene, 0.05);
    assert.equal(summarizeRiskDetail(null), null);
});

test('blastRadiusTierLabel maps scores to words', () => {
    assert.equal(blastRadiusTierLabel(80), 'High risk');
    assert.equal(blastRadiusTierLabel(40), 'Moderate risk');
    assert.equal(blastRadiusTierLabel(5), 'Low risk');
    assert.equal(blastRadiusTierLabel(0), 'Minimal risk');
});
