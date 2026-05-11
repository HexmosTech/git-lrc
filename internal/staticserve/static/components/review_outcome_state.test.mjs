import test from 'node:test';
import assert from 'node:assert/strict';

import { shouldShowAllClear } from './review_outcome_state.mjs';

test('shouldShowAllClear returns true for completed reviews with zero comments and no error', () => {
    assert.equal(shouldShowAllClear({
        status: 'completed',
        totalComments: 0,
        errorSummary: '',
        summarySlidesEligibility: { eligible: true, reason: 'ok' },
    }), true);
});

test('shouldShowAllClear returns false when comments exist', () => {
    assert.equal(shouldShowAllClear({
        status: 'completed',
        totalComments: 1,
        errorSummary: '',
        summarySlidesEligibility: { eligible: true, reason: 'ok' },
    }), false);
});

test('shouldShowAllClear returns false when an error summary is present', () => {
    assert.equal(shouldShowAllClear({
        status: 'completed',
        totalComments: 0,
        errorSummary: 'Review failed',
        summarySlidesEligibility: { eligible: true, reason: 'ok' },
    }), false);
});

test('shouldShowAllClear returns false while review is still running', () => {
    assert.equal(shouldShowAllClear({
        status: 'in_progress',
        totalComments: 0,
        errorSummary: '',
        summarySlidesEligibility: { eligible: true, reason: 'ok' },
    }), false);
});

test('shouldShowAllClear normalizes string inputs and whitespace-only error summaries', () => {
    assert.equal(shouldShowAllClear({
        status: ' Completed ',
        totalComments: '0',
        errorSummary: '   ',
        summarySlidesEligibility: { eligible: true, reason: 'ok' },
    }), true);
});

test('shouldShowAllClear treats missing or non-numeric comment counts as zero', () => {
    assert.equal(shouldShowAllClear({
        status: 'completed',
        totalComments: undefined,
        errorSummary: undefined,
        summarySlidesEligibility: { eligible: true, reason: 'ok' },
    }), true);

    assert.equal(shouldShowAllClear({
        status: 'completed',
        totalComments: 'not-a-number',
        errorSummary: '',
        summarySlidesEligibility: { eligible: true, reason: 'ok' },
    }), true);
});

test('shouldShowAllClear rejects non-string error payloads', () => {
    assert.equal(shouldShowAllClear({
        status: 'completed',
        totalComments: 0,
        errorSummary: { message: 'boom' },
        summarySlidesEligibility: { eligible: true, reason: 'ok' },
    }), false);
});

test('shouldShowAllClear rejects missing status even when counts are zero', () => {
    assert.equal(shouldShowAllClear({
        status: null,
        totalComments: 0,
        errorSummary: '',
        summarySlidesEligibility: { eligible: true, reason: 'ok' },
    }), false);
});

test('shouldShowAllClear returns false when summary structure is invalid', () => {
    assert.equal(shouldShowAllClear({
        status: 'completed',
        totalComments: 0,
        errorSummary: '',
        summarySlidesEligibility: { eligible: false, reason: 'missing-required-sections' },
    }), false);
});

test('shouldShowAllClear returns false when summary validator result is missing', () => {
    assert.equal(shouldShowAllClear({
        status: 'completed',
        totalComments: 0,
        errorSummary: '',
    }), false);
});