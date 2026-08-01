// DiffTable component - renders diff hunks with lines and comments
import { waitForPreact, filePathToId, getCommentVisibilityKey, buildIssueCodeExcerpt } from './utils.js';
import { matchesIssueFilters } from './issue_filter_state.mjs';
import { getComment } from './Comment.js';
import { getBlastRadiusPanel } from './BlastRadiusPanel.js';
import { getRiskBadge } from './RiskBadge.js';
import { getCommentRenderLabel } from './review_performance_state.mjs';

export async function createDiffTable() {
    const { html, useState } = await waitForPreact();
    const Comment = await getComment();
    const BlastRadiusPanel = await getBlastRadiusPanel();
    const RiskBadge = await getRiskBadge();

    return function DiffTable({
        hunks,
        filePath,
        fileId,
        issueFilters,
        hiddenCommentKeys,
        onToggleCommentVisibility,
        reviewStartMs,
        commentRenderTimes,
        onCommentRendered,
        commentVotes,
        onVote
    }) {
        if (!hunks || hunks.length === 0) {
            return html`
                <div style="padding: 20px; text-align: center; color: #57606a;">
                    No diff content available.
                </div>
            `;
        }
        
        // Use provided fileId or generate from filePath
        const resolvedFileId = fileId || filePathToId(filePath);

        // Which hunks' "why this score" panels are open (keyed by index).
        const [openBlastPanels, setOpenBlastPanels] = useState(() => new Set());
        const toggleBlastPanel = (idx) => {
            setOpenBlastPanels(prev => {
                const next = new Set(prev);
                if (next.has(idx)) {
                    next.delete(idx);
                } else {
                    next.add(idx);
                }
                return next;
            });
        };
        // Ensure-open variant used from comment risk chips: never collapses.
        const openBlastPanel = (idx) => {
            setOpenBlastPanels(prev => {
                if (prev.has(idx)) return prev;
                const next = new Set(prev);
                next.add(idx);
                return next;
            });
        };

        return html`
            <table class="diff-table">
                ${hunks.map((hunk, hunkIdx) => html`
                    <tr id="hunk-${resolvedFileId}-${hunkIdx}">
                        <td colspan="3" class="hunk-header">
                            ${typeof hunk.BlastRadius === 'number' && html`
                                <${RiskBadge}
                                    score=${hunk.BlastRadius}
                                    detail=${hunk.BlastDetail || null}
                                    size="large"
                                    expanded=${openBlastPanels.has(hunkIdx)}
                                    onOpen=${hunk.BlastDetail ? (() => toggleBlastPanel(hunkIdx)) : undefined}
                                />
                            `}
                            ${hunk.Header}
                        </td>
                    </tr>
                    ${hunk.BlastDetail && openBlastPanels.has(hunkIdx) && html`
                        <tr class="blast-panel-row">
                            <td colspan="3">
                                <${BlastRadiusPanel} detail=${hunk.BlastDetail} />
                            </td>
                        </tr>
                    `}
                    ${hunk.Lines.map((line, idx) => {
                        // Build line-numbered code context for per-issue copy.
                        const codeExcerpt = buildIssueCodeExcerpt(hunk.Lines, idx, 1);
                        const rowLine = Number(line.NewNum) || Number(line.OldNum) || 0;
                        const rowId = rowLine > 0 ? `line-${resolvedFileId}-${rowLine}` : '';
                        
                        return html`
                            <tr
                                class="diff-line ${line.Class}"
                                id=${rowId || undefined}
                                data-file-id=${resolvedFileId}
                                data-old-line=${line.OldNum || ''}
                                data-new-line=${line.NewNum || ''}
                            >
                                <td class="line-num">${line.OldNum}</td>
                                <td class="line-num">${line.NewNum}</td>
                                <td class="line-content">${line.Content}</td>
                            </tr>
                            ${line.IsComment && line.Comments && line.Comments.map((comment, commentIdx) => {
                                if (!matchesIssueFilters(comment, issueFilters)) return null;
                                const commentId = `comment-${resolvedFileId}-${comment.Line}-${commentIdx}`;
                                const visibilityKey = getCommentVisibilityKey(filePath, comment);
                                const isHidden = hiddenCommentKeys && hiddenCommentKeys.has(visibilityKey);
                                const renderTimingLabel = getCommentRenderLabel(reviewStartMs, commentRenderTimes?.[visibilityKey]);
                                return html`
                                    <${Comment}
                                        key=${visibilityKey}
                                        comment=${comment}
                                        filePath=${filePath}
                                        codeExcerpt=${codeExcerpt}
                                        commentId=${commentId}
                                        isHidden=${isHidden}
                                        visibilityKey=${visibilityKey}
                                        onToggleVisibility=${onToggleCommentVisibility}
                                        onFirstRender=${onCommentRendered}
                                        renderTimingLabel=${renderTimingLabel}
                                        vote=${commentVotes && commentVotes[visibilityKey] || null}
                                        onVote=${onVote}
                                        hunkRiskScore=${typeof hunk.BlastRadius === 'number' ? hunk.BlastRadius : null}
                                        hunkRiskDetail=${hunk.BlastDetail || null}
                                        onOpenRiskPanel=${hunk.BlastDetail ? (() => openBlastPanel(hunkIdx)) : undefined}
                                    />
                                `;
                            })}
                        `;
                    })}
                `)}
            </table>
        `;
    };
}

let DiffTableComponent = null;
export async function getDiffTable() {
    if (!DiffTableComponent) {
        DiffTableComponent = await createDiffTable();
    }
    return DiffTableComponent;
}
