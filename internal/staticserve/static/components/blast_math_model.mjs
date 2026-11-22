// blast_math_model.mjs - the arithmetic behind BlastRadiusPanel's "Math Mode".
//
// Extracted from the panel so the derivation can be tested against real
// review artifacts without mounting Preact. It mirrors blastradius.ScoreHunks:
// per-symbol signals are summed per dimension, then rolled into the hunk's
// raw score.
//
// Two roll-up formulas exist in real artifacts:
//
//   - linear (reports generated before 2026-10-01): raw = sum(symbols) + hunk
//   - saturating (current): raw = sqrt(sum(symbols)) + hunk
//
// Current reports carry `SymbolCombine: "sqrt"`. Older artifacts are silent,
// so the mode is inferred by fitting the recorded raw score; this keeps the
// checked-in historical reviews showing the arithmetic they were generated
// with while new reports show the saturating step. The two formulas differ
// by more than a rounding error for every multi-point hunk, so the fit is
// unambiguous (see blast_math_model.test.mjs).

import { SEVERITY_WEIGHT, blendRiskScore } from './blast_radius_sort_state.mjs';

// Mirror blastRadiusCategories/reviewPriorityCategories in blastradius.go.
// A signal in neither set (e.g. "diff-shape" hygiene) feeds no raw score.
export const BLAST_CATEGORIES = new Set(['architecture', 'graph']);
export const PRIORITY_CATEGORIES = new Set(['duplication', 'code-metrics']);

function shortName(qualifiedName) {
    const parts = (qualifiedName || '').split('.');
    return parts[parts.length - 1] || qualifiedName;
}

// sumExpression spells a list of numbers out as a literal "1.8 + 1.4 - 0.9"
// arithmetic expression plus its total. Math Mode's whole point is to show
// the addition, never just assert the result.
export function sumExpression(nums) {
    if (!nums || nums.length === 0) return { text: '0', total: 0 };
    const total = nums.reduce((a, b) => a + b, 0);
    const text = nums.reduce((acc, n, i) => {
        if (i === 0) return n.toFixed(1);
        return `${acc} ${n >= 0 ? '+' : '-'} ${Math.abs(n).toFixed(1)}`;
    }, '');
    return { text, total };
}

function filterByCategory(signals, categories) {
    return (signals || []).filter((s) => categories.has(s.Category));
}

function buildSymbolLines(symbols, categories) {
    return (symbols || [])
        .map((sym) => {
            const signals = filterByCategory(sym.Signals, categories);
            if (signals.length === 0) return null;
            return {
                name: sym.Name || shortName(sym.QualifiedName),
                signals,
                ...sumExpression(signals.map((s) => s.Points || 0)),
            };
        })
        .filter(Boolean);
}

// detectCombineMode picks the roll-up used by the artifact's generator.
// `SymbolCombine` is authoritative when present; otherwise the recorded raw
// score is the ground truth and we choose the formula that reproduces it.
export function detectCombineMode(declared, symbolSubtotal, hunkSubtotal, raw, crossRepoBonus = 0) {
    const linear = symbolSubtotal + hunkSubtotal;
    const saturating = Math.sqrt(symbolSubtotal) + hunkSubtotal;
    const hint = (declared || '').toLowerCase();
    if (hint === 'sqrt') {
        return { mode: 'sqrt', total: saturating, delta: raw - saturating };
    }
    if (hint === 'linear') {
        return { mode: 'linear', total: linear, delta: raw - linear };
    }
    // Cross-repo enrichment adds its bonus after the roll-up, to both the
    // symbol sum and the recorded raw score. Reconstruct what the sqrt
    // formula produced before that bonus so a historical cross-repo artifact
    // is not mistaken for linear.
    const base = Math.max(0, symbolSubtotal - crossRepoBonus);
    const saturatingWithBonus = Math.sqrt(base) + hunkSubtotal + crossRepoBonus;
    const saturatingDelta = Math.min(
        Math.abs(raw - saturating),
        Math.abs(raw - saturatingWithBonus),
    );
    if (saturatingDelta + 1e-9 < Math.abs(raw - linear)) {
        return { mode: 'sqrt', total: saturating, delta: raw - saturating };
    }
    return { mode: 'linear', total: linear, delta: raw - linear };
}

function buildDimension(opts) {
    const {
        startStep, title, symbols, hunkSignalList, categories,
        raw, max, norm, maxSource, self, declaredCombine,
    } = opts;

    const perSymbol = buildSymbolLines(symbols, categories);
    const hunkSignals = filterByCategory(hunkSignalList, categories);
    const hunkExpression = hunkSignals.length > 0
        ? sumExpression(hunkSignals.map((s) => s.Points || 0))
        : null;
    const symbolSubtotal = perSymbol.reduce((sum, line) => sum + line.total, 0);
    const hunkSubtotal = hunkExpression ? hunkExpression.total : 0;

    let crossRepoBonus = 0;
    if (categories.has('graph')) {
        (symbols || []).forEach((sym) => {
            (sym.Signals || []).forEach((sig) => {
                if (sig.Name === 'Cross-repo impact' || sig.Name === 'Cross-repo chain') {
                    crossRepoBonus += sig.Points || 0;
                }
            });
        });
    }

    const combined = detectCombineMode(declaredCombine, symbolSubtotal, hunkSubtotal, raw, crossRepoBonus);

    // Linear artifacts are rendered exactly as they always were: hunk-level
    // signals are just one more subtotal in "add every subtotal together".
    // Saturating artifacts split that into sum -> sqrt -> add hunk signals,
    // which is what the backend actually computes.
    const linearParts = [...perSymbol.map((line) => line.total)];
    if (hunkExpression) linearParts.push(hunkSubtotal);
    const sumExpressionParts = combined.mode === 'linear'
        ? sumExpression(linearParts)
        : sumExpression(perSymbol.map((line) => line.total));

    const combinedSymbols = Math.sqrt(symbolSubtotal);

    let n = startStep;
    const stepAdd = n++;
    const stepSum = (combined.mode === 'linear'
        ? linearParts.length > 1
        : perSymbol.length > 1)
        ? n++
        : 0;
    const stepCombine = combined.mode === 'sqrt' && symbolSubtotal > 0 ? n++ : 0;
    const stepHunk = combined.mode === 'sqrt' && hunkExpression ? n++ : 0;
    // Cross-repo enrichment (RecordCrossRepoImpact) adds its bonus after the
    // structural roll-up, so the recorded raw score can be slightly higher
    // than the formula above. Show that delta as a real step rather than
    // hiding it behind an "artifact mismatch" note.
    const stepAdjust = Math.abs(combined.delta) > 0.05 ? n++ : 0;
    const stepScale = n++;

    // Older artifacts (review 10566, for example) recorded the normalized
    // score but not the raw maximum it was divided by. Recover the divisor
    // from raw/norm so the scale step shows real arithmetic instead of
    // claiming no hunk scored above zero.
    const recordedMax = max || 0;
    const effectiveMax = recordedMax > 0
        ? recordedMax
        : (raw > 0 && norm > 0 ? raw / (norm / 100) : 0);
    const isSelf = Boolean(recordedMax > 0 && self && maxSource
        && maxSource.file === self.file && maxSource.header === self.header);

    return {
        title,
        mode: combined.mode,
        declaredCombine: declaredCombine || '',
        perSymbol,
        hunkSignals,
        hunkExpression,
        symbolSubtotal,
        crossRepoBonus,
        hunkSubtotal,
        combinedSymbols,
        sumExpression: sumExpressionParts,
        total: combined.total,
        // How far the recorded raw score is from the signals shown. Real
        // artifacts land at 0; a nonzero value means the generator included
        // something the report did not expose as a signal, and the panel
        // says so instead of pretending the arithmetic is complete.
        artifactDelta: combined.delta,
        raw: raw || 0,
        max: effectiveMax,
        maxInferred: recordedMax <= 0 && effectiveMax > 0,
        norm: norm || 0,
        maxSource: maxSource || { file: '', header: '' },
        isSelf,
        stepAdd,
        stepSum,
        stepCombine,
        stepHunk,
        stepAdjust,
        stepScale,
        nextStep: n,
    };
}

// buildMathMode returns every number and step number the panel renders. The
// caller does no arithmetic of its own, so a parity test can assert the same
// derivation the UI shows.
export function buildMathMode(detail) {
    if (!detail) return null;

    const symbols = detail.Symbols || [];
    const weights = detail.Weights && typeof detail.Weights.BlastRadius === 'number'
        ? detail.Weights
        : { BlastRadius: 0.6, ReviewPriority: 0.4 };
    const self = { file: detail.FilePath, header: detail.Header };

    const blast = buildDimension({
        startStep: 1,
        title: 'Blast Radius',
        symbols,
        hunkSignalList: detail.Signals,
        categories: BLAST_CATEGORIES,
        raw: detail.BlastRadiusRaw,
        max: detail.MaxBlastRadiusRaw,
        norm: detail.BlastRadiusNorm,
        maxSource: { file: detail.MaxBlastRadiusHunkFile, header: detail.MaxBlastRadiusHunkHeader },
        self,
        declaredCombine: detail.SymbolCombine,
    });
    const priority = buildDimension({
        startStep: blast.nextStep,
        title: 'Review Priority',
        symbols,
        hunkSignalList: detail.Signals,
        categories: PRIORITY_CATEGORIES,
        raw: detail.ReviewPriorityRaw,
        max: detail.MaxReviewPriorityRaw,
        norm: detail.ReviewPriorityNorm,
        maxSource: { file: detail.MaxReviewPriorityHunkFile, header: detail.MaxReviewPriorityHunkHeader },
        self,
        declaredCombine: detail.SymbolCombine,
    });

    const blastShare = weights.BlastRadius * blast.norm;
    const priorityShare = weights.ReviewPriority * priority.norm;
    const blended = blastShare + priorityShare;
    const hygieneMultiplier = typeof detail.HygieneMultiplier === 'number' ? detail.HygieneMultiplier : 1.0;
    const structural = blended * hygieneMultiplier;
    const severityScore = typeof detail.FindingSeverity === 'number' ? detail.FindingSeverity : 0;
    const severityLabel = detail.FindingSeverityLabel || null;
    const severityCounts = detail.FindingSeverityCounts || { critical: 0, warning: 0, info: 0 };
    const severityShare = SEVERITY_WEIGHT * severityScore;
    const final = blendRiskScore(structural, severityScore);

    const stepBlend = priority.nextStep;
    const stepHygiene = stepBlend + 1;
    const stepSeverity = stepBlend + 2;
    const stepFinalBlend = stepBlend + 3;

    return {
        blast,
        priority,
        weights,
        blastShare,
        priorityShare,
        blended,
        hygieneMultiplier,
        structural,
        severityScore,
        severityLabel,
        severityCounts,
        severityShare,
        final,
        stepBlend,
        stepHygiene,
        stepSeverity,
        stepFinalBlend,
    };
}
