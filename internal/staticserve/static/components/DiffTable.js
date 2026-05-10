// DiffTable component - renders diff hunks with lines and comments
import { waitForPreact, filePathToId, getCommentVisibilityKey, buildIssueCodeExcerpt } from './utils.js';
import { getComment } from './Comment.js';
import { getCommentRenderLabel } from './review_performance_state.mjs';

export async function createDiffTable() {
    const { html } = await waitForPreact();
    const Comment = await getComment();
    
    return function DiffTable({ 
        hunks, 
        filePath, 
        fileId, 
        visibleSeverities,
        hiddenCommentKeys,
        onToggleCommentVisibility,
        reviewStartMs,
        commentRenderTimes,
        onCommentRendered
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
                        <td colspan="3" class="hunk-header">${hunk.Header}</td>
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
                                const sev = (comment.Severity || '').toLowerCase();
                                if (visibleSeverities && !visibleSeverities.has(sev)) return null;
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
