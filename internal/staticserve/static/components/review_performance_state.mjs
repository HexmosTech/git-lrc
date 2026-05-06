const FALLBACK_ACTIVITY_LINES = [
    'Preparing review workspace',
    'Scanning changed files',
    'Analyzing diff structure',
    'Streaming review activity',
];

export function getPerformanceNow() {
    if (typeof performance !== 'undefined' && typeof performance.now === 'function') {
        return performance.now();
    }
    return Date.now();
}

export function formatDurationShort(durationMs) {
    if (!Number.isFinite(durationMs) || durationMs < 0) {
        return '';
    }

    const totalSeconds = Math.max(0, Math.round(durationMs / 1000));
    const hours = Math.floor(totalSeconds / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    const seconds = totalSeconds % 60;

    if (hours > 0) {
        return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
    }
    if (minutes > 0) {
        return seconds > 0 ? `${minutes}m ${seconds}s` : `${minutes}m`;
    }
    return `${seconds}s`;
}

export function recordFirstRenderTime(existingTimings, commentKey, renderMs) {
    const current = existingTimings || {};
    if (!commentKey || !Number.isFinite(renderMs)) {
        return current;
    }
    if (Object.prototype.hasOwnProperty.call(current, commentKey)) {
        return current;
    }
    return {
        ...current,
        [commentKey]: renderMs,
    };
}

export function getFirstRenderTime(renderTimings) {
    const values = Object.values(renderTimings || {}).filter((value) => Number.isFinite(value));
    if (values.length === 0) {
        return null;
    }
    return Math.min(...values);
}

export function buildPerformanceSnapshot({ baselineMs, nowMs, firstCommentMs, totalComments, completedMs }) {
    const effectiveNow = Number.isFinite(completedMs) ? completedMs : nowMs;
    const elapsedMs = Number.isFinite(baselineMs) && Number.isFinite(effectiveNow)
        ? Math.max(0, effectiveNow - baselineMs)
        : 0;
    const firstCommentElapsedMs = Number.isFinite(firstCommentMs) && Number.isFinite(baselineMs)
        ? Math.max(0, firstCommentMs - baselineMs)
        : null;
    const normalizedComments = Number.isFinite(totalComments) ? Math.max(0, totalComments) : 0;

    return {
        elapsedMs,
        elapsedLabel: formatDurationShort(elapsedMs),
        firstCommentElapsedMs,
        firstCommentLabel: firstCommentElapsedMs === null ? 'Waiting' : formatDurationShort(firstCommentElapsedMs),
        totalComments: normalizedComments,
        summaryItems: [
            {
                key: 'first-comment',
                label: 'First comment',
                value: firstCommentElapsedMs === null ? 'Waiting' : formatDurationShort(firstCommentElapsedMs),
            },
            {
                key: 'elapsed',
                label: Number.isFinite(completedMs) ? 'Final time' : 'Elapsed',
                value: formatDurationShort(elapsedMs),
            },
            {
                key: 'comments',
                label: 'Comments',
                value: String(normalizedComments),
            },
        ],
    };
}

export function getCommentRenderLabel(baselineMs, renderedAtMs) {
    if (!Number.isFinite(baselineMs) || !Number.isFinite(renderedAtMs)) {
        return '';
    }
    return `Appeared in ${formatDurationShort(Math.max(0, renderedAtMs - baselineMs))}`;
}

export function getLoadingActivityMessage(events, elapsedMs) {
    const latestMessage = [...(events || [])]
        .reverse()
        .map((event) => String(event?.message || '').trim())
        .find(Boolean);

    if (latestMessage) {
        return latestMessage;
    }

    const safeElapsedMs = Number.isFinite(elapsedMs) ? Math.max(0, elapsedMs) : 0;
    const index = Math.floor(safeElapsedMs / 3000) % FALLBACK_ACTIVITY_LINES.length;
    return FALLBACK_ACTIVITY_LINES[index];
}