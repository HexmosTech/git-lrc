import test from 'node:test';
import assert from 'node:assert/strict';

import { clampUsagePercent, isEnterpriseSelfhostedPlan, normalizeUsagePayload, planLabel, usageTone } from './usage_chip_model.mjs';

test('planLabel maps known plan codes', () => {
  assert.equal(planLabel('free'), 'Free 30k');
  assert.equal(planLabel('free_30k'), 'Free 30k');
  assert.equal(planLabel('team'), 'Team 100k');
  assert.equal(planLabel('team_32usd'), 'Team 100k');
  assert.equal(planLabel('loc_1600k'), 'Team 1.6M');
  assert.equal(planLabel('custom_plan'), 'custom_plan');
  assert.equal(planLabel('enterprise-selfhosted'), 'Enterprise - Selfhosted');
});

test('isEnterpriseSelfhostedPlan detects enterprise-selfhosted plan', () => {
  assert.equal(isEnterpriseSelfhostedPlan('enterprise-selfhosted'), true);
  assert.equal(isEnterpriseSelfhostedPlan('free_30k'), false);
  assert.equal(isEnterpriseSelfhostedPlan('team_32usd'), false);
  assert.equal(isEnterpriseSelfhostedPlan(''), false);
});

test('clampUsagePercent normalizes numeric values', () => {
  assert.equal(clampUsagePercent(-10), 0);
  assert.equal(clampUsagePercent(45.2), 45);
  assert.equal(clampUsagePercent(180), 100);
  assert.equal(clampUsagePercent('not-a-number'), 0);
});

test('normalizeUsagePayload produces stable defaults', () => {
  const normalized = normalizeUsagePayload({
    available: true,
    usage_pct: 72,
    top_members: [{ label: 'a@b.com', loc: 55, share: 12.4, kind: 'user' }],
  });

  assert.equal(normalized.available, true);
  assert.equal(normalized.usagePct, 72);
  assert.equal(normalized.planCode, '');
  assert.equal(normalized.topMembers.length, 1);
  assert.equal(normalized.topMembers[0].label, 'a@b.com');
});

test('usageTone reflects critical, warning, and unavailable states', () => {
  assert.equal(usageTone({ available: false }, false), 'unavailable');
  assert.equal(usageTone({ available: true, blocked: true, customerState: '', usagePct: 10 }, false), 'critical');
  assert.equal(usageTone({ available: true, blocked: false, customerState: 'payment_failed', usagePct: 10 }, false), 'critical');
  assert.equal(usageTone({ available: true, blocked: false, customerState: '', usagePct: 88 }, false), 'warn');
  assert.equal(usageTone({ available: true, blocked: false, customerState: '', usagePct: 15 }, false), 'ok');
  assert.equal(usageTone({ available: true, blocked: false, customerState: '', usagePct: 15 }, true), 'loading');
});
