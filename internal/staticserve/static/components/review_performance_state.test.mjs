import test from 'node:test';
import assert from 'node:assert/strict';

import {
    buildPerformanceSnapshot,
    formatDurationShort,
    getCommentRenderLabel,
    getFirstRenderTime,
    getLoadingActivityMessage,
    recordFirstRenderTime,
} from './review_performance_state.mjs';

test('recordFirstRenderTime stores only the first render timestamp per comment', () => {
    const first = recordFirstRenderTime({}, 'a::1', 1200);
    const second = recordFirstRenderTime(first, 'a::1', 2400);

    assert.deepEqual(first, { 'a::1': 1200 });
    assert.equal(second, first);
});

test('getFirstRenderTime returns the earliest recorded comment render', () => {
    assert.equal(getFirstRenderTime({}), null);
    assert.equal(getFirstRenderTime({ one: 4200, two: 1800, three: 3600 }), 1800);
});

test('buildPerformanceSnapshot derives browser-perceived summary items', () => {
    const snapshot = buildPerformanceSnapshot({
        baselineMs: 1000,
        nowMs: 42000,
        firstCommentMs: 19000,
        totalComments: 7,
        completedMs: null,
    });

    assert.equal(snapshot.elapsedLabel, '41s');
    assert.equal(snapshot.firstCommentLabel, '18s');
    assert.deepEqual(snapshot.summaryItems, [
        { key: 'first-comment', label: 'First comment', value: '18s' },
        { key: 'elapsed', label: 'Elapsed', value: '41s' },
        { key: 'comments', label: 'Comments', value: '7' },
    ]);
});

test('buildPerformanceSnapshot freezes final elapsed time when completed', () => {
    const snapshot = buildPerformanceSnapshot({
        baselineMs: 500,
        nowMs: 60000,
        firstCommentMs: 16500,
        totalComments: 3,
        completedMs: 42500,
    });

    assert.equal(snapshot.elapsedLabel, '42s');
    assert.equal(snapshot.summaryItems[1].label, 'Final time');
});

test('getCommentRenderLabel returns a subtle relative timing label', () => {
    assert.equal(getCommentRenderLabel(1000, 19000), 'Appeared in 18s');
    assert.equal(getCommentRenderLabel(1000, null), '');
});

test('getLoadingActivityMessage prefers latest event message and falls back to rotating copy', () => {
    assert.equal(getLoadingActivityMessage([
        { message: 'Batch 1 completed: generated 2 comments' },
        { message: 'Status: in_progress' },
    ], 12000), 'Status: in_progress');

    assert.equal(getLoadingActivityMessage([], 0), 'Preparing review workspace');
    assert.equal(getLoadingActivityMessage([], 3100), 'Scanning changed files');
});

test('formatDurationShort formats short and multi-minute durations', () => {
    assert.equal(formatDurationShort(900), '1s');
    assert.equal(formatDurationShort(61000), '1m 1s');
    assert.equal(formatDurationShort(3600000), '1h');
});