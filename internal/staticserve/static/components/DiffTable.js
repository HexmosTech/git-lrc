// DiffTable component - renders diff hunks with lines and comments
import { waitForPreact, filePathToId, getCommentVisibilityKey, buildIssueCodeExcerpt } from './utils.js';
import { matchesIssueFilters } from './issue_filter_state.mjs';
import { getComment } from './Comment.js';
import { getBlastRadiusPanel } from './BlastRadiusPanel.js';
import { getRiskBadge } from './RiskBadge.js';
import { getCommentRenderLabel } from './review_performance_state.mjs';

export async function createDiffTable() {
    const preact = await waitForPreact();
    const { html, useState, useEffect } = preact;
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
        // Track which hunk index was most recently opened so an effect can
        // scroll it into view (the panel renders below the header row, so
        // bringing the header to the top of the viewport keeps the whole
        // panel visible). Stale when its panel has since been closed.
        const [lastOpenedHunkIdx, setLastOpenedHunkIdx] = useState(null);
        const toggleBlastPanel = (idx) => {
            setOpenBlastPanels(prev => {
                const next = new Set(prev);
                if (next.has(idx)) {
                    next.delete(idx);
                } else {
                    next.add(idx);
                    setLastOpenedHunkIdx(idx);
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
                setLastOpenedHunkIdx(idx);
                return next;
            });
        };

        // When a blast panel opens, scroll the enclosing file's header (the
        // filename title bar) into view so both the title and the just-opened
        // breakdown below the hunk stay visible. Falling back to the hunk
        // header keeps the scroll working in synthetic whole-diff blocks
        // where the file title is the next sibling above. Honors the
        // floating filter bar via the same --severity-filter-sticky-offset
        // variable the rest of the app uses for scroll-margin-top.
        useEffect(() => {
            if (lastOpenedHunkIdx === null) return;
            if (!openBlastPanels.has(lastOpenedHunkIdx)) return; // stale
            const hunkHeaderEl = document.getElementById(`hunk-${resolvedFileId}-${lastOpenedHunkIdx}`);
            if (!hunkHeaderEl) return;
            // Walk up to the .file block and target its .file-header (the
            // filename title). Falls back to the hunk header when the file
            // wrapper isn't found (e.g. synthetic flat-risk blocks).
            const fileBlockEl = hunkHeaderEl.closest('.file');
            const targetEl = (fileBlockEl && fileBlockEl.querySelector('.file-header')) || hunkHeaderEl;
            const mainContent = document.querySelector('.main-content');
            if (!mainContent) return;
            const filterOffset = (() => {
                const v = getComputedStyle(document.documentElement)
                    .getPropertyValue('--severity-filter-sticky-offset');
                const n = parseFloat(v);
                return Number.isFinite(n) ? n : 56;
            })();
            const headerEl2 = document.querySelector('.header');
            const headerHeight = headerEl2 ? headerEl2.offsetHeight : 60;
            // +32px of breathing room so the file title is always fully clear
            // of both the floating header and the sticky severity filter —
            // avoids the recurring "half the title is hidden" trim issue.
            const topOffset = headerHeight + filterOffset + 32;
            const rect = targetEl.getBoundingClientRect();
            const mainRect = mainContent.getBoundingClientRect();
            const scrollTarget = mainContent.scrollTop + rect.top - mainRect.top - topOffset;
            mainContent.scrollTo({ top: scrollTarget, behavior: 'smooth' });
        }, [lastOpenedHunkIdx, openBlastPanels, resolvedFileId]);

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
