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

// summarizeRiskDetail condenses a report hunk (BlastDetail) into what the
// hover card shows: the headline numbers plus the strongest signals across
// the hunk AND its touched symbols, ranked by absolute contribution.
export function summarizeRiskDetail(detail, limit = 4) {
    if (!detail) return null;
    const all = [...(detail.Signals || [])];
    (detail.Symbols || []).forEach((sym) => {
        (sym.Signals || []).forEach((s) => all.push(s));
    });
    all.sort((a, b) => Math.abs(b.Points || 0) - Math.abs(a.Points || 0));
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
            return { ...hunk, BlastRadius: detail.Combined, BlastDetail: detail };
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
