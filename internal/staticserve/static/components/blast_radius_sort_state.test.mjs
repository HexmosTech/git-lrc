import test from 'node:test';
import assert from 'node:assert/strict';

import {
    hasBlastRadiusData,
    sortFilesByBlastRadius,
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
