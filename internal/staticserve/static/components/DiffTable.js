// DiffTable component - renders diff hunks with lines and comments
import { waitForPreact, filePathToId, getCommentVisibilityKey, buildIssueCodeExcerpt } from './utils.js';
import { matchesIssueFilters } from './issue_filter_state.mjs';
import { getComment } from './Comment.js';
import { getCommentRenderLabel } from './review_performance_state.mjs';
import { renderIcon } from './icons.js';

// Discrete tiers (mirroring the existing badge-info/warning/critical scheme)
// rather than a continuous color ramp, for visual consistency with the rest
// of the review UI.
function blastRadiusTier(score) {
    if (score >= 66) return 'blast-radius-high';
    if (score >= 33) return 'blast-radius-medium';
    if (score > 0) return 'blast-radius-low';
    return 'blast-radius-none';
}

export async function createDiffTable() {
    const { html } = await waitForPreact();
    const Comment = await getComment();
    
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
        
        return html`
            <table class="diff-table">
                ${hunks.map(hunk => html`
                    <tr>
                        <td colspan="3" class="hunk-header">
                            ${typeof hunk.BlastRadius === 'number' && html`
                                <span
                                    class="blast-radius-badge ${blastRadiusTier(hunk.BlastRadius)}"
                                    title="Blast radius: this hunk touches symbols with ${hunk.BlastRadius.toFixed(1)}/100 relative importance in this review"
                                >${renderIcon(html, 'blastRadius', { size: 11 })} ${Math.round(hunk.BlastRadius)}</span>
                            `}
                            ${hunk.Header}
                        </td>
                    </tr>
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
