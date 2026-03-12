// Utility functions for LiveReview UI

// Wait for preact to be available
export function waitForPreact() {
    return new Promise((resolve) => {
        if (window.preact) {
            resolve(window.preact);
            return;
        }
        const check = setInterval(() => {
            if (window.preact) {
                clearInterval(check);
                resolve(window.preact);
            }
        }, 10);
    });
}

// Generate file ID from path
export function filePathToId(filePath) {
    return 'file_' + filePath.replace(/[^a-zA-Z0-9]/g, '_');
}

// Get badge class for severity
export function getBadgeClass(severity) {
    const sev = (severity || '').toLowerCase();
    if (sev === 'critical') return 'badge-critical';
    if (sev === 'error') return 'badge-error';
    if (sev === 'warning') return 'badge-warning';
    return 'badge-info';
}

// Format timestamp for display
export function formatTime(timestamp) {
    return new Date(timestamp).toLocaleTimeString();
}

// Copy text to clipboard
export async function copyToClipboard(text) {
    await navigator.clipboard.writeText(text);
}

// Build contextual code excerpt for a comment line.
// Includes previous/current/next lines (when available) with line numbers.
export function buildIssueCodeExcerpt(lines, commentLineIndex, contextLines = 1) {
    if (!Array.isArray(lines) || lines.length === 0) return '';
    if (!Number.isInteger(commentLineIndex) || commentLineIndex < 0 || commentLineIndex >= lines.length) return '';

    const radius = Number.isInteger(contextLines) && contextLines >= 0 ? contextLines : 1;
    const start = Math.max(0, commentLineIndex - radius);
    const end = Math.min(lines.length - 1, commentLineIndex + radius);

    const excerptLines = [];
    for (let i = start; i <= end; i++) {
        const line = lines[i] || {};
        const newNum = String(line.NewNum || '').trim();
        const oldNum = String(line.OldNum || '').trim();
        const lineNumber = newNum || oldNum || '?';
        const content = typeof line.Content === 'string' ? line.Content : '';
        const marker = i === commentLineIndex ? '>' : ' ';
        excerptLines.push(`${marker} ${lineNumber}: ${content}`);
    }

    return excerptLines.join('\n');
}

// Build a stable key for a comment so toggles stay in sync even if order changes.
export function getCommentVisibilityKey(filePath, comment) {
    const path = filePath || comment?.FilePath || '';
    const line = comment?.Line ?? comment?.line ?? '';
    const severity = (comment?.Severity || comment?.severity || '').toLowerCase();
    const category = (comment?.Category || comment?.category || '').toLowerCase();
    const content = (comment?.Content || comment?.content || '')
        .trim()
        .replace(/\s+/g, ' ');
    return `${path}::${line}::${severity}::${category}::${content}`;
}

// Count visible comments for a single file, filtered by visibleSeverities.
// Returns the count of comments whose severity is in the Set.
export function countVisibleComments(file, visibleSeverities, hiddenCommentKeys) {
    if (!file) return 0;
    const severitySet = visibleSeverities && visibleSeverities.size > 0
        ? visibleSeverities
        : new Set(['critical', 'error', 'warning', 'info']);
    const hiddenSet = hiddenCommentKeys || null;
    let count = 0;
    (file.Hunks || []).forEach(hunk => {
        (hunk.Lines || []).forEach(line => {
            if (!line.IsComment || !line.Comments) return;
            line.Comments.forEach((c) => {
                const sev = (c.Severity || '').toLowerCase();
                if (!severitySet.has(sev)) return;
                if (hiddenSet) {
                    const visibilityKey = getCommentVisibilityKey(file.FilePath, c);
                    if (hiddenSet.has(visibilityKey)) return;
                }
                count++;
            });
        });
    });
    return count;
}

// Count all issues by severity across files. Returns { critical, error, warning, info, total, visible }.
export function countIssuesBySeverity(files, visibleSeverities, hiddenCommentKeys) {
    let critical = 0, error = 0, warning = 0, info = 0, visible = 0;
    const hiddenSet = hiddenCommentKeys || null;
    const severitySet = visibleSeverities && visibleSeverities.size > 0
        ? visibleSeverities
        : new Set(['critical', 'error', 'warning', 'info']);
    files.forEach(file => {
        if (!file.HasComments) return;
        (file.Hunks || []).forEach(hunk => {
            (hunk.Lines || []).forEach(line => {
                if (line.IsComment && line.Comments) {
                    line.Comments.forEach((c) => {
                        const sev = (c.Severity || '').toLowerCase();
                        if (sev === 'critical') critical++;
                        else if (sev === 'error') error++;
                        else if (sev === 'warning') warning++;
                        else info++;
                        if (!severitySet.has(sev)) return;
                        if (hiddenSet) {
                            const visibilityKey = getCommentVisibilityKey(file.FilePath, c);
                            if (hiddenSet.has(visibilityKey)) return;
                        }
                        visible++;
                    });
                }
            });
        });
    });
    const total = critical + error + warning + info;
    return { critical, error, warning, info, total, visible };
}

// Format a single issue for clipboard copy.
export function formatIssueForCopy(filePath, comment) {
    const lineSuffix = comment.Line ? ':' + comment.Line : '';
    const sevLabel = comment.Severity
        ? ` (${comment.Severity}${comment.HasCategory ? ', ' + comment.Category : ''})`
        : '';
    return `${filePath}${lineSuffix} — ${comment.Content}${sevLabel}`;
}

// Transform backend event to display format
export function transformEvent(event) {
    let message = '';
    const eventData = event.data || {};
    
    switch (event.type) {
        case 'log':
            message = (eventData.message || '').replace(/\\n/g, '\n').replace(/\\t/g, '  ').replace(/\\"/g, '"');
            break;
        case 'batch':
            const batchId = event.batchId || 'unknown';
            if (eventData.status === 'processing') {
                const fileCount = eventData.fileCount || 0;
                message = `Batch ${batchId} started: processing ${fileCount} file${fileCount !== 1 ? 's' : ''}`;
            } else if (eventData.status === 'completed') {
                const commentCount = eventData.commentCount || 0;
                message = `Batch ${batchId} completed: generated ${commentCount} comment${commentCount !== 1 ? 's' : ''}`;
            } else {
                message = `Batch ${batchId}: ${eventData.status || 'unknown status'}`;
            }
            break;
        case 'status':
            message = `Status: ${eventData.status || 'unknown'}`;
            break;
        case 'artifact':
            message = eventData.url ? `Generated: ${eventData.kind || 'artifact'}` : `Artifact: ${eventData.kind || 'unknown'}`;
            break;
        case 'completion':
            const count = eventData.commentCount || 0;
            message = eventData.resultSummary || `Process completed with ${count} comment${count !== 1 ? 's' : ''}`;
            break;
        default:
            message = JSON.stringify(eventData);
    }
    
    return {
        id: event.id,
        type: event.type,
        time: event.time,
        level: event.level || 'info',
        batchId: event.batchId,
        data: eventData,
        message: message
    };
}

// Logo SVG as data URI
export const LOGO_DATA_URI = "data:image/svg+xml;base64,PD94bWwgdmVyc2lvbj0iMS4wIiBlbmNvZGluZz0iVVRGLTgiPz4KPHN2ZyB3aWR0aD0iNTEyIiBoZWlnaHQ9IjUxMiIgdmlld0JveD0iMCAwIDUxMiA1MTIiIGZpbGw9Im5vbmUiIHhtbG5zPSJodHRwOi8vd3d3LnczLm9yZy8yMDAwL3N2ZyI+CiAgPCEtLSBCYWNrZ3JvdW5kIGdsb3cgZWZmZWN0IC0tPgogIDxjaXJjbGUgY3g9IjI1NiIgY3k9IjI1NiIgcj0iMjQwIiBmaWxsPSIjMUU0MjlGIiBvcGFjaXR5PSIwLjIiIC8+CiAgCiAgPCEtLSBNYWluIGV5ZSBzaGFwZSAtLT4KICA8Y2lyY2xlIGN4PSIyNTYiIGN5PSIyNTYiIHI9IjIwMCIgZmlsbD0iIzExMTgyNyIgLz4KICA8Y2lyY2xlIGN4PSIyNTYiIGN5PSIyNTYiIHI9IjIwMCIgc3Ryb2tlPSIjM0I4MkY2IiBzdHJva2Utd2lkdGg9IjE2IiAvPgogIAogIDwhLS0gSXJpcyAtLT4KICA8Y2lyY2xlIGN4PSIyNTYiIGN5PSIyNTYiIHI9IjEwMCIgZmlsbD0iIzYwQTVGQSIgLz4KICAKICA8IS0tIFB1cGlsIC0tPgogIDxjaXJjbGUgY3g9IjI1NiIgY3k9IjI1NiIgcj0iNTAiIGZpbGw9IiMxRTQwQUYiIC8+CiAgCiAgPCEtLSBTaW5nbGUgbGlnaHQgcmVmbGVjdGlvbiAobW9yZSBzdWJ0bGUpIC0tPgogIDxwYXRoIGQ9Ik0yMzUgMjIwQzIzNSAyMjguMjg0IDIyOC4yODQgMjM1IDIyMCAyMzVDMjExLjcxNiAyMzUgMjA1IDIyOC4yODQgMjA1IDIyMEMyMDUgMjExLjcxNiAyMTEuNzE2IDIwNSAyMjAgMjA1QzIyOC4yODQgMjA1IDIzNSAyMTEuNzE2IDIzNSAyMjBaIiBmaWxsPSJ3aGl0ZSIgb3BhY2l0eT0iMC44IiAvPgogIAogIDwhLS0gT3V0ZXIgZ2xvdyAtLT4KICA8Y2lyY2xlIGN4PSIyNTYiIGN5PSIyNTYiIHI9IjIyMCIgc3Ryb2tleT0iIzkzQzVGRCIgc3Ryb2tlLXdpZHRoPSI0IiBvcGFjaXR5PSIwLjYiIC8+Cjwvc3ZnPgo=";
