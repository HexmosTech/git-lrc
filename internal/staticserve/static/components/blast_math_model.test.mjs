import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { buildMathMode, detectCombineMode, sumExpression } from './blast_math_model.mjs';

// Real reviews, not hand-built hunks. See
// blastradius/testdata/reviews/README.md for provenance and reproduction.
function loadReport(relativePath) {
    return JSON.parse(readFileSync(new URL(relativePath, import.meta.url), 'utf8'));
}

const golden = loadReport('../../../../blastradius/testdata/reviews/review_11632_report.json');
const current = loadReport('../../../../blastradius/testdata/reviews/review_11632_current_report.json');
const largest = loadReport('../../../../tools/uidev/fixtures/blastradius.json').report;
const crossRepo = loadReport('../../../../tools/uidev/fixtures/crossrepo-latest/blastradius.json').report;

function hunk(report, file, newStart) {
    const f = report.Files.find((x) => x.Path === file);
    assert.ok(f, `file ${file} not found`);
    const h = f.Hunks.find((x) => x.NewStart === newStart);
    assert.ok(h, `hunk ${file}:${newStart} not found`);
    return h;
}

const near = (got, want, tolerance, label) => {
    assert.ok(
        Math.abs(got - want) <= tolerance,
        `${label}: got ${got}, want ${want} (±${tolerance})`,
    );
};

test('legacy review 11632 multi-symbol hunk matches LiveReview golden numbers', () => {
    const mm = buildMathMode(hunk(golden, 'ui/src/pages/Chatbot/rebucketChart.ts', 557));

    assert.equal(mm.blast.mode, 'linear');
    assert.deepEqual(
        mm.blast.perSymbol.map((line) => line.name),
        ['SlopeEncodingParts', 'SlopeStats', 'computeHeatmapStats', 'computeSlopeStats', 'getSlopeEncoding', 'isSlopeGraph'],
    );
    near(mm.blast.symbolSubtotal, 30.442234425508587, 1e-9, 'blast symbol subtotal');
    near(mm.blast.hunkSubtotal, 0.25980762113533157, 1e-9, 'hunk signal subtotal');
    near(mm.blast.total, 30.702042046643918, 1e-9, 'blast total');
    near(mm.blast.raw, 30.702042046643918, 1e-9, 'recorded blast raw');
    near(mm.blast.artifactDelta, 0, 1e-9, 'blast artifact delta');

    // Linear artifacts keep the original step layout: add signals, add every
    // subtotal, scale. No sqrt step.
    assert.deepEqual(
        [mm.blast.stepAdd, mm.blast.stepSum, mm.blast.stepCombine, mm.blast.stepHunk, mm.blast.stepScale],
        [1, 2, 0, 0, 3],
    );
    assert.equal(mm.blast.isSelf, true);
    near(mm.blast.norm, 100, 1e-9, 'blast norm');

    near(mm.priority.total, 69.7, 1e-9, 'priority total');
    assert.deepEqual(
        [mm.priority.stepAdd, mm.priority.stepSum, mm.priority.stepCombine, mm.priority.stepHunk, mm.priority.stepScale],
        [4, 5, 0, 0, 6],
    );
    assert.equal(mm.stepBlend, 7);
    assert.equal(mm.stepHygiene, 8);
    near(mm.structural, 100, 1e-9, 'structural score');
});

test('legacy single-symbol hunk skips the sum step and still lands on Combined', () => {
    const mm = buildMathMode(hunk(golden, 'ui/src/pages/Chatbot/ChatConversation.tsx', 841));

    assert.equal(mm.blast.mode, 'linear');
    assert.equal(mm.blast.perSymbol.length, 1);
    assert.deepEqual(
        [mm.blast.stepAdd, mm.blast.stepSum, mm.blast.stepScale],
        [1, 0, 2],
        'one symbol means no redundant "add every subtotal" step',
    );
    near(mm.blast.total, 6.980266605821071, 1e-9, 'blast total');
    near(mm.blast.norm, 22.735512495280727, 1e-9, 'blast norm');
    near(mm.blast.artifactDelta, 0, 1e-9, 'blast artifact delta');

    // Priority numbering continues where Blast Radius left off.
    assert.equal(mm.priority.stepAdd, 3);
    assert.equal(mm.priority.stepScale, 4);
    assert.equal(mm.stepBlend, 5);

    // The load-bearing invariant: the derivation reproduces the stored
    // pre-severity Combined score.
    near(mm.structural, 35.50644379559024, 1e-9, 'structural == Combined');
});

test('legacy zero-signal hunk derives zero without inventing steps', () => {
    const mm = buildMathMode(hunk(golden, 'ui/src/pages/Chatbot/ChatConversation.tsx', 21));

    assert.equal(mm.blast.mode, 'linear');
    assert.equal(mm.blast.perSymbol.length, 0);
    assert.equal(mm.blast.total, 0);
    assert.deepEqual([mm.blast.stepAdd, mm.blast.stepSum, mm.blast.stepScale], [1, 0, 2]);
    assert.equal(mm.structural, 0);
});

test('current review 11632 renders the saturating combine the scorer used', () => {
    const mm = buildMathMode(hunk(current, 'ui/src/pages/Chatbot/rebucketChart.ts', 557));

    assert.equal(mm.blast.mode, 'sqrt');
    assert.equal(mm.blast.declaredCombine, 'sqrt');
    near(mm.blast.symbolSubtotal, 30.442234425508587, 1e-9, 'symbol subtotal');
    near(mm.blast.combinedSymbols, 5.51744818059115, 1e-9, 'sqrt of symbol subtotal');
    near(mm.blast.hunkSubtotal, 0.24556058315617352, 1e-9, 'hunk subtotal');
    near(mm.blast.total, 5.763008763747323, 1e-9, 'blast total');
    near(mm.blast.artifactDelta, 0, 1e-9, 'artifact delta');

    // add signals -> sum -> sqrt -> add hunk signals -> scale
    assert.deepEqual(
        [mm.blast.stepAdd, mm.blast.stepSum, mm.blast.stepCombine, mm.blast.stepHunk, mm.blast.stepScale],
        [1, 2, 3, 4, 5],
    );
    near(mm.blast.raw, mm.blast.total, 1e-9, 'rederived total == recorded raw');

    const single = buildMathMode(hunk(current, 'ui/src/pages/Chatbot/ChatConversation.tsx', 841));
    assert.equal(single.blast.mode, 'sqrt');
    // A single symbol still gets the sqrt step: the scorer applies it to the
    // sum even when only one contribution is present.
    assert.deepEqual(
        [single.blast.stepAdd, single.blast.stepSum, single.blast.stepCombine, single.blast.stepHunk, single.blast.stepScale],
        [1, 0, 2, 0, 3],
    );
    near(single.blast.total, 2.6420194181385326, 1e-9, 'single-symbol sqrt total');
});

test('largest captured review (10566) replays as legacy linear for every hunk', () => {
    let hunks = 0;
    let withSymbols = 0;
    for (const file of largest.Files) {
        for (const h of file.Hunks) {
            hunks += 1;
            if ((h.Symbols || []).length > 0) withSymbols += 1;
            const mm = buildMathMode(h);
            assert.equal(mm.blast.mode, 'linear', `${file.Path}:${h.NewStart} blast mode`);
            assert.equal(mm.priority.mode, 'linear', `${file.Path}:${h.NewStart} priority mode`);
            near(mm.blast.total, h.BlastRadiusRaw || 0, 1e-6, `${file.Path}:${h.NewStart} blast total`);
            near(mm.priority.total, h.ReviewPriorityRaw || 0, 1e-6, `${file.Path}:${h.NewStart} priority total`);
            near(mm.structural, h.Combined || 0, 1e-6, `${file.Path}:${h.NewStart} structural == Combined`);
        }
    }
    assert.equal(hunks, 207);
    assert.equal(withSymbols, 45);

    const nav = buildMathMode(hunk(largest, 'ui/src/components/Navbar/NavMegaMenu.tsx', 1));
    assert.equal(nav.blast.perSymbol.length, 15);
    near(nav.blast.total, 45.57003102438757, 1e-9, '15-symbol hunk total');

    // This capture predates MaxBlastRadiusRaw, so the divisor must be
    // recovered from the recorded normalized rank instead of claiming no
    // hunk scored above zero.
    const top = buildMathMode(hunk(largest, 'ui/src/components/UIPrimitives.tsx', 700));
    assert.equal(top.blast.maxInferred, true);
    near(top.blast.max, 183.75178077156346, 1e-6, 'recovered blast max');
    near(top.blast.norm, 100, 1e-9, 'recorded blast norm');
});

test('post-scoring cross-repo bonuses appear as their own adjustment step', () => {
    // RecordCrossRepoImpact adds its points to the symbol and to the hunk's
    // recorded raw score *after* the sqrt roll-up, so the formula alone lands
    // short of the stored score. The derivation must show that delta instead
    // of silently disagreeing with it.
    const detail = {
        FilePath: 'src/service.ts',
        Header: '@@ -1,1 +1,2 @@',
        SymbolCombine: 'sqrt',
        BlastRadiusRaw: Math.sqrt(12) + 8,
        BlastRadiusNorm: 100,
        MaxBlastRadiusRaw: Math.sqrt(12) + 8,
        ReviewPriorityRaw: 0,
        ReviewPriorityNorm: 0,
        MaxReviewPriorityRaw: 0,
        Symbols: [{
            Name: 'readAadhaar',
            Signals: [
                { Name: 'Caller reach', Points: 12, Category: 'graph' },
                { Name: 'Cross-repo impact', Points: 8, Category: 'graph' },
            ],
        }],
    };
    const mm = buildMathMode(detail);
    near(mm.blast.symbolSubtotal, 20, 1e-9, 'symbol subtotal');
    near(mm.blast.combinedSymbols, Math.sqrt(20), 1e-9, 'sqrt roll-up');
    assert.equal(mm.blast.stepAdjust > 0, true, 'adjustment step must be present');
    near(mm.blast.raw, Math.sqrt(12) + 8, 1e-9, 'recorded raw');
    near(mm.blast.artifactDelta, (Math.sqrt(12) + 8) - Math.sqrt(20), 1e-9, 'adjustment delta');
    assert.equal(mm.blast.stepScale, mm.blast.stepAdjust + 1);
});

test('combine-mode detection prefers the recorded score when unmarked', () => {
    assert.equal(detectCombineMode(undefined, 30.442, 0.26, 30.702).mode, 'linear');
    assert.equal(detectCombineMode(undefined, 30.442, 0.26, 5.517 + 0.26).mode, 'sqrt');
    assert.equal(detectCombineMode('sqrt', 30.442, 0.26, 30.702).mode, 'sqrt');
    assert.equal(detectCombineMode('linear', 30.442, 0.26, 5.777).mode, 'linear');
    // Zero is identical under either formula; default to the historical one.
    assert.equal(detectCombineMode(undefined, 0, 0, 0).mode, 'linear');
});

test('combine-mode detection discounts a post-scoring cross-repo bonus', () => {
    // review 14285: symbol sum 14.21 includes an 8-point cross-repo bonus;
    // the raw score is sqrt(14.21 - 8) + 8 = 10.492, which is closer to the
    // linear total (14.21) unless the bonus is accounted for.
    assert.equal(detectCombineMode(undefined, 14.210, 0, 10.492, 8).mode, 'sqrt');
    // A genuine legacy linear artifact with the same bonus still matches
    // linear exactly.
    assert.equal(detectCombineMode(undefined, 14.210, 0, 14.210, 8).mode, 'linear');
});

test('real review 14285 cross-repo artifact detects sqrt and shows the adjustment', () => {
    let impacted = 0;
    let hunks = 0;
    for (const file of crossRepo.Files) {
        for (const h of file.Hunks) {
            hunks += 1;
            const mm = buildMathMode(h);
            assert.equal(mm.blast.mode, 'sqrt', `${file.Path}:${h.NewStart} mode`);
            near(mm.structural, h.Combined || 0, 1e-6, `${file.Path}:${h.NewStart} structural == Combined`);
            if ((h.ImpactedRelatedRepos || []).length > 0) {
                impacted += 1;
                assert.ok(mm.blast.stepAdjust > 0, `${file.Path}:${h.NewStart} adjustment step`);
                assert.ok(mm.blast.artifactDelta > 0, `${file.Path}:${h.NewStart} positive delta`);
            }
        }
    }
    assert.equal(hunks, 7);
    assert.equal(impacted, 3);
});

test('sumExpression keeps its literal arithmetic form', () => {
    const expr = sumExpression([1.8, 1.4, -0.9]);
    assert.equal(expr.text, '1.8 + 1.4 - 0.9');
    near(expr.total, 2.3, 1e-12, 'sum total');
    assert.deepEqual(sumExpression([]), { text: '0', total: 0 });
});
