// Pure helpers for the optional "sort by blast radius" toggle. Hunks carry
// an optional BlastRadius score (0-100, set only when --blast-radius was
// used); these helpers never assume it is present.

function normalizedScore(hunk) {
    const value = hunk?.BlastRadius;
    return typeof value === 'number' && Number.isFinite(value) ? value : null;
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
