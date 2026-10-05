// Pure helpers for the blast-radius sort modes. Hunks carry an optional
// BlastRadius score (0-100, computed locally when the graph engine is
// available); these helpers never assume it is present.

export const SORT_MODE_RISK_FLAT = 'risk-flat'; // whole-diff ranking, files dissolved
export const SORT_MODE_RISK_FILE = 'risk-file'; // hunks ranked within each file
export const SORT_MODE_DIFF = 'diff'; // original diff order (the classic view)

function normalizedScore(hunk) {
    const value = hunk?.BlastRadius;
    return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

// ===== Severity-aware blending =====
// The structural blast-radius score (Combined) is blind to the severity of
// the LLM findings attached to a hunk. A critical security bug in a
// low-blast-radius hunk must outrank a trivial typo in a slightly-higher
// one, so the sort score blends Combined with the hunk's most severe finding
// at SEVERITY_WEIGHT (default 10%): sortScore = 0.9 * Combined + 0.1 * severity.
export const SEVERITY_SCORE = { critical: 100, warning: 25, info: 5 };
export const SEVERITY_WEIGHT = 0.10;

// hunkSeverityInfo returns the hunk's finding-severity breakdown: the most
// severe finding mapped to 0-100 (score), its label ('critical' | 'warning'
// | 'info', or null when the hunk has no findings), and per-level counts.
// git-lrc attaches comments to hunk lines as
// { Severity: 'CRITICAL' | 'WARNING' | 'INFO' } (see app.js's
// convertFilesToUIFormat), so severity lives on hunk.Lines[].Comments[].
export function hunkSeverityInfo(hunk) {
    let score = 0;
    let label = null;
    const counts = { critical: 0, warning: 0, info: 0 };
    (hunk?.Lines || []).forEach((line) => {
        (line?.Comments || []).forEach((comment) => {
            const key = (comment?.Severity || comment?.severity || '').toLowerCase();
            if (!(key in SEVERITY_SCORE)) return;
            counts[key] += 1;
            const value = SEVERITY_SCORE[key];
            if (value > score) {
                score = value;
                label = key;
            }
        });
    });
    return { score, label, counts };
}

// hunkSeverityScore maps a hunk's most severe comment to a 0-100 value
// (0 when the hunk carries no findings).
export function hunkSeverityScore(hunk) {
    return hunkSeverityInfo(hunk).score;
}

// blendRiskScore mixes a structural 0-100 Combined score with a severity
// score at SEVERITY_WEIGHT. Missing Combined is treated as 0.
export function blendRiskScore(combined, severity) {
    const c = typeof combined === 'number' && Number.isFinite(combined) ? combined : 0;
    const s = typeof severity === 'number' && Number.isFinite(severity) ? severity : 0;
    return (1 - SEVERITY_WEIGHT) * c + SEVERITY_WEIGHT * s;
}

// blastRadiusTier maps a 0-100 score to a discrete severity tier (mirroring
// the badge-info/warning/critical scheme) - shared by the diff badge and the
// sidebar hunk chips so colors always agree.
export function blastRadiusTier(score) {
    if (score >= 66) return 'blast-radius-high';
    if (score >= 33) return 'blast-radius-medium';
    if (score > 0) return 'blast-radius-low';
    return 'blast-radius-none';
}

// blastRadiusTierLabel is the human word for a tier, used by the risk hover
// card ("High risk", "Moderate risk", ...).
export function blastRadiusTierLabel(score) {
    if (score >= 66) return 'High risk';
    if (score >= 33) return 'Moderate risk';
    if (score > 0) return 'Low risk';
    return 'Minimal risk';
}

// allSignals flattens a report hunk (BlastDetail) into every Signal that
// contributed to it - the hunk's own (file coupling, arch role) plus every
// touched symbol's - ranked by absolute contribution. This is the full set
// that BlastRadiusRaw/ReviewPriorityRaw are literally the sum of (see
// blastradius.go's sumSignalPoints); a UI showing only detail.Signals is
// showing a small fraction of what the headline score is built from.
//
// Symbol-sourced signals carry a `_symbolName` (the symbol they came from);
// hunk-level signals don't. A hunk touching several symbols can easily
// produce the *same* signal name several times over (e.g. "Caller reach" for
// each of 4 touched functions) - without _symbolName there's no way for a UI
// to show that these are 4 different symbols' scores, not one signal counted
// four times.
export function allSignals(detail) {
    if (!detail) return [];
    const all = [...(detail.Signals || [])];
    (detail.Symbols || []).forEach((sym) => {
        (sym.Signals || []).forEach((s) => all.push({ ...s, _symbolName: sym.Name || sym.QualifiedName }));
    });
    all.sort((a, b) => Math.abs(b.Points || 0) - Math.abs(a.Points || 0));
    return all;
}

// summarizeRiskDetail condenses a report hunk (BlastDetail) into what the
// hover card shows: the headline numbers plus the strongest signals across
// the hunk AND its touched symbols, ranked by absolute contribution.
export function summarizeRiskDetail(detail, limit = 4) {
    if (!detail) return null;
    const all = allSignals(detail);
    const top = all.slice(0, limit);
    return {
        score: detail.Combined || 0,
        blast: detail.BlastRadiusNorm || 0,
        priority: detail.ReviewPriorityNorm || 0,
        hygiene: typeof detail.HygieneMultiplier === 'number' && detail.HygieneMultiplier < 1
            ? detail.HygieneMultiplier
            : null,
        top,
        moreCount: Math.max(0, all.length - top.length),
        totalSignals: all.length,
    };
}

// hunkBlastKey is the join key between UI hunks and /api/blastradius report
// hunks: file path plus the new-side start line and line count.
export function hunkBlastKey(filePath, newStart, newLines) {
    return `${filePath}:${newStart}:${newLines}`;
}

// buildBlastLookup flattens a /api/blastradius report into a Map keyed by
// hunkBlastKey, valued with the full report hunk (signals, dimensions,
// hygiene multiplier). Returns an empty Map for a missing report.
export function buildBlastLookup(report) {
    const lookup = new Map();
    (report?.Files || []).forEach((file) => {
        (file.Hunks || []).forEach((hunk) => {
            lookup.set(hunkBlastKey(file.Path, hunk.NewStart, hunk.NewLines), hunk);
        });
    });
    return lookup;
}

// attachBlastData returns new file objects whose hunks carry BlastRadius
// (the Combined 0-100 score) and BlastDetail (the full report hunk) joined
// from the lookup. Hunks with no lookup entry keep whatever BlastRadius the
// server already stamped on them (or null). Inputs are never mutated.
export function attachBlastData(files, lookup) {
    if (!lookup || lookup.size === 0) {
        return files || [];
    }
    return (files || []).map((file) => ({
        ...file,
        Hunks: (file.Hunks || []).map((hunk) => {
            const detail = lookup.get(hunkBlastKey(file.FilePath, hunk.NewStartLine, hunk.NewLineCount));
            if (!detail) {
                return hunk;
            }
            // BlastRadius drives both the sort order and the hunk's headline
            // risk badge. Blend the structural Combined with the hunk's most
            // severe finding so ordering and the displayed number stay in
            // sync. BlastDetail keeps the full structural breakdown, plus the
            // severity breakdown (FindingSeverity*) so the detail panel can
            // explain the severity contribution in its own signal/Math Mode.
            const severity = hunkSeverityInfo(hunk);
            return {
                ...hunk,
                BlastRadius: blendRiskScore(detail.Combined, severity.score),
                BlastDetail: {
                    ...detail,
                    FindingSeverity: severity.score,
                    FindingSeverityLabel: severity.label,
                    FindingSeverityCounts: severity.counts,
                },
            };
        }),
    }));
}

function hunkCommentCount(hunk) {
    let count = 0;
    (hunk.Lines || []).forEach((line) => {
        if (line.IsComment && Array.isArray(line.Comments)) {
            count += line.Comments.length;
        }
    });
    return count;
}

// flattenFilesByRisk dissolves file boundaries into one globally ranked
// hunk list: each entry is a synthetic single-hunk "file" (so the existing
// FileBlock rendering works unchanged) ordered by descending BlastRadius.
// Unscored hunks keep their diff order after every scored one. ExpandKey
// points at the real file's ID so expand/collapse state (tracked per real
// file) applies to all of that file's hunks at once.
export function flattenFilesByRisk(files) {
    const entries = [];
    (files || []).forEach((file, fileIdx) => {
        (file.Hunks || []).forEach((hunk, hunkIdx) => {
            entries.push({ file, fileIdx, hunk, hunkIdx, score: normalizedScore(hunk) });
        });
    });
    entries.sort((a, b) => {
        if ((a.score === null) !== (b.score === null)) {
            return a.score === null ? 1 : -1;
        }
        if (a.score === null || a.score === b.score) {
            return a.fileIdx - b.fileIdx || a.hunkIdx - b.hunkIdx;
        }
        return b.score - a.score;
    });
    return entries.map(({ file, fileIdx, hunk, hunkIdx }, rank) => {
        const commentCount = hunkCommentCount(hunk);
        return {
            ...file,
            ID: `${file.ID}--hunk-${fileIdx}-${hunkIdx}`,
            ExpandKey: file.ID,
            Hunks: [hunk],
            HasComments: commentCount > 0,
            CommentCount: commentCount,
            SyntheticHunk: true,
            RiskRank: rank + 1,
            // 1-based position of this hunk within its original file, for
            // sidebar "Hunk n" submenu labels.
            SourceHunkNumber: hunkIdx + 1,
        };
    });
}

// hasBlastRadiusData reports whether any hunk across files carries a
// computed score - used to decide whether the sort toggle should render at
// all, since it's meaningless when --blast-radius wasn't used.
export function hasBlastRadiusData(files) {
    return (files || []).some((file) => (file.Hunks || []).some((hunk) => normalizedScore(hunk) !== null));
}

// sortHunksByBlastRadius returns a new array of hunks ordered by descending
// score; hunks with no score keep their original relative order and sort
// after every scored hunk. The input array is never mutated.
export function sortHunksByBlastRadius(hunks) {
    return [...(hunks || [])]
        .map((hunk, index) => ({ hunk, index, score: normalizedScore(hunk) }))
        .sort((a, b) => {
            if ((a.score === null) !== (b.score === null)) {
                return a.score === null ? 1 : -1;
            }
            if (a.score === null) {
                return a.index - b.index;
            }
            return b.score - a.score;
        })
        .map((entry) => entry.hunk);
}

// sortFilesByBlastRadius returns new file objects with their Hunks reordered
// by sortHunksByBlastRadius; files themselves keep their original order.
export function sortFilesByBlastRadius(files) {
    return (files || []).map((file) => ({
        ...file,
        Hunks: sortHunksByBlastRadius(file.Hunks),
    }));
}
