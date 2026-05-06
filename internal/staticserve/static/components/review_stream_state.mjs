const EVENT_POLL_LIMIT = 1000;
const EVENT_TIME_OVERLAP_MS = 1000;

function stableStringify(value) {
    if (value === null || value === undefined) {
        return '';
    }
    if (Array.isArray(value)) {
        return `[${value.map(stableStringify).join(',')}]`;
    }
    if (typeof value === 'object') {
        return `{${Object.keys(value).sort().map((key) => `${key}:${stableStringify(value[key])}`).join(',')}}`;
    }
    return String(value);
}

function normalizeEventId(event, index) {
    if (event?.id !== undefined && event?.id !== null) {
        return String(event.id);
    }
    return [
        event?.type || 'event',
        event?.time || 'unknown',
        event?.batchId || '',
        stableStringify(event?.data),
        index,
    ].join(':');
}

function normalizeSeverity(rawSeverity) {
    const severity = String(rawSeverity || 'info').trim().toLowerCase();
    if (severity === 'critical' || severity === 'error' || severity === 'warning') {
        return severity;
    }
    return 'info';
}

function normalizeFilePath(file) {
    return file?.file_path || file?.filePath || file?.FilePath || '';
}

export function buildEventsURL(reviewID, lastSeenAt) {
    const params = new URLSearchParams({ limit: String(EVENT_POLL_LIMIT) });
    if (lastSeenAt) {
        const parsed = new Date(lastSeenAt);
        if (!Number.isNaN(parsed.getTime())) {
            params.set('since', new Date(parsed.getTime() - EVENT_TIME_OVERLAP_MS).toISOString());
        }
    }
    return `/api/v1/diff-review/${reviewID}/events?${params.toString()}`;
}

export function extractNewEvents(events, seenEventIds) {
    const nextSeenEventIds = new Set(seenEventIds || []);
    const newEvents = [];
    let lastSeenAt = null;

    (events || []).forEach((event, index) => {
        const eventId = normalizeEventId(event, index);
        const eventTime = event?.time || null;

        if (eventTime) {
            if (!lastSeenAt || new Date(eventTime).getTime() > new Date(lastSeenAt).getTime()) {
                lastSeenAt = eventTime;
            }
        }

        if (nextSeenEventIds.has(eventId)) {
            return;
        }

        nextSeenEventIds.add(eventId);
        newEvents.push(event);
    });

    return { newEvents, nextSeenEventIds, lastSeenAt };
}

export function inferReviewStatusFromEvents(events) {
    for (let index = (events || []).length - 1; index >= 0; index--) {
        const event = events[index] || {};
        const eventData = event.data || {};
        if (event.type === 'status' && eventData.status) {
            return eventData.status;
        }
        if (event.type === 'completion') {
            return eventData.errorSummary ? 'failed' : 'completed';
        }
    }
    return '';
}

export function normalizeStreamedComment(rawComment) {
    if (!rawComment) {
        return null;
    }

    const isInternal = Boolean(rawComment.IsInternal ?? rawComment.isInternal);
    if (isInternal) {
        return null;
    }

    const filePath = rawComment.FilePath || rawComment.filePath || rawComment.file_path || '';
    const line = Number(rawComment.Line ?? rawComment.line ?? rawComment.lineNumber ?? rawComment.line_number);
    const content = String(rawComment.Content ?? rawComment.content ?? '').trim();

    if (!filePath || !Number.isFinite(line) || line <= 0 || !content) {
        return null;
    }

    return {
        file_path: filePath,
        line: line,
        content: content,
        severity: normalizeSeverity(rawComment.Severity ?? rawComment.severity),
        category: String(rawComment.Category ?? rawComment.category ?? 'review').trim() || 'review'
    };
}

export function extractExternalCommentsFromEvents(events) {
    const comments = [];

    (events || []).forEach((event) => {
        const eventData = event?.data || {};
        if (event?.type !== 'batch' || eventData.status !== 'completed' || !Array.isArray(eventData.comments)) {
            return;
        }
        eventData.comments.forEach((comment) => {
            const normalized = normalizeStreamedComment(comment);
            if (normalized) {
                comments.push(normalized);
            }
        });
    });

    return comments;
}

export function appendStreamedCommentsToFiles(files, incomingComments) {
    if (!Array.isArray(files) || files.length === 0 || !Array.isArray(incomingComments) || incomingComments.length === 0) {
        return files || [];
    }

    const nextFiles = files.map((file) => {
        const existingComments = Array.isArray(file?.comments) ? file.comments : [];
        return {
            ...file,
            comments: [...existingComments]
        };
    });

    const fileIndexByPath = new Map();
    nextFiles.forEach((file, index) => {
        const filePath = normalizeFilePath(file);
        if (filePath) {
            fileIndexByPath.set(filePath, index);
        }
    });

    incomingComments.forEach((comment) => {
        const fileIndex = fileIndexByPath.get(comment.file_path);
        if (fileIndex === undefined) {
            return;
        }
        nextFiles[fileIndex].comments.push(comment);
    });

    return nextFiles;
}