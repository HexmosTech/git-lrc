import test from 'node:test';
import assert from 'node:assert/strict';

import {
    appendStreamedCommentsToFiles,
    buildEventsURL,
    extractExternalCommentsFromEvents,
    extractNewEvents,
    inferReviewStatusFromEvents,
    normalizeStreamedComment,
} from './review_stream_state.mjs';

test('normalizeStreamedComment drops internal comments', () => {
    const result = normalizeStreamedComment({
        FilePath: 'a.go',
        Line: 12,
        Content: 'internal only',
        Severity: 'warning',
        IsInternal: true,
    });

    assert.equal(result, null);
});

test('normalizeStreamedComment returns external comment in diff-view shape', () => {
    const result = normalizeStreamedComment({
        FilePath: 'a.go',
        Line: 12,
        Content: 'handle error',
        Severity: 'WARNING',
        Category: '',
        IsInternal: false,
    });

    assert.deepEqual(result, {
        file_path: 'a.go',
        line: 12,
        content: 'handle error',
        severity: 'warning',
        category: 'review',
    });
});

test('extractExternalCommentsFromEvents keeps only external completed batch comments', () => {
    const comments = extractExternalCommentsFromEvents([
        {
            type: 'batch',
            data: {
                status: 'completed',
                comments: [
                    { FilePath: 'a.go', Line: 10, Content: 'visible', Severity: 'info', IsInternal: false },
                    { FilePath: 'a.go', Line: 11, Content: 'hidden', Severity: 'info', IsInternal: true },
                ],
            },
        },
        {
            type: 'batch',
            data: {
                status: 'processing',
                comments: [
                    { FilePath: 'b.go', Line: 7, Content: 'ignore', Severity: 'warning', IsInternal: false },
                ],
            },
        },
    ]);

    assert.deepEqual(comments, [
        {
            file_path: 'a.go',
            line: 10,
            content: 'visible',
            severity: 'info',
            category: 'review',
        },
    ]);
});

test('extractNewEvents returns only unseen events and advances bookmark time', () => {
    const seenEventIds = new Set(['1']);
    const { newEvents, nextSeenEventIds, lastSeenAt } = extractNewEvents([
        { id: 1, type: 'log', time: '2026-05-06T10:00:00.000Z' },
        { id: 2, type: 'batch', time: '2026-05-06T10:00:00.000Z' },
        { id: 3, type: 'completion', time: '2026-05-06T10:00:05.000Z' },
    ], seenEventIds);

    assert.deepEqual(newEvents.map(event => event.id), [2, 3]);
    assert.ok([...nextSeenEventIds].every((eventId) => typeof eventId === 'string'));
    assert.deepEqual([...nextSeenEventIds].sort(), ['1', '2', '3']);
    assert.equal(lastSeenAt, '2026-05-06T10:00:05.000Z');
});

test('appendStreamedCommentsToFiles appends matching comments without mutating input', () => {
    const files = [
        { file_path: 'a.go', comments: [{ line: 1, content: 'existing', severity: 'info', category: 'review' }] },
        { file_path: 'b.go', comments: [] },
    ];

    const nextFiles = appendStreamedCommentsToFiles(files, [
        { file_path: 'a.go', line: 2, content: 'new-a', severity: 'warning', category: 'review' },
        { file_path: 'b.go', line: 3, content: 'new-b', severity: 'error', category: 'review' },
        { file_path: 'missing.go', line: 9, content: 'ignored', severity: 'info', category: 'review' },
    ]);

    assert.equal(files[0].comments.length, 1);
    assert.equal(nextFiles[0].comments.length, 2);
    assert.equal(nextFiles[1].comments.length, 1);
    assert.equal(nextFiles[0].comments[1].content, 'new-a');
    assert.equal(nextFiles[1].comments[0].content, 'new-b');
});

test('inferReviewStatusFromEvents prefers explicit status and falls back to completion', () => {
    assert.equal(inferReviewStatusFromEvents([
        { type: 'completion', data: { resultSummary: 'ok' } },
    ]), 'completed');

    assert.equal(inferReviewStatusFromEvents([
        { type: 'completion', data: { errorSummary: 'boom' } },
    ]), 'failed');

    assert.equal(inferReviewStatusFromEvents([
        { type: 'completion', data: { resultSummary: 'ok' } },
        { type: 'status', data: { status: 'in_progress' } },
    ]), 'in_progress');
});

test('buildEventsURL includes overlapped since cursor when bookmark exists', () => {
    const url = buildEventsURL('r-123', '2026-05-06T10:00:05.000Z');

    assert.match(url, /^\/api\/v1\/diff-review\/r-123\/events\?/);
    assert.match(url, /limit=1000/);
    assert.match(url, /since=2026-05-06T10%3A00%3A04.000Z/);
});