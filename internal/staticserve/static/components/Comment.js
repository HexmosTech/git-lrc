// Comment component
import { renderIcon } from './icons.js';
import { waitForPreact, getBadgeClass, copyToClipboard } from './utils.js';
import { getFeedbackPopup } from './FeedbackPopup.js';
import { getRiskBadge } from './RiskBadge.js';

export async function createComment() {
    const { html, useEffect, useState } = await waitForPreact();
    const FeedbackPopup = await getFeedbackPopup();
    const RiskBadge = await getRiskBadge();

    const renderMetaItem = (label, value, extraClass = '') => {
        if (!value) {
            return null;
        }
        return html`
            <span class="comment-meta-item ${extraClass}">
                <span class="comment-meta-label">${label}</span>
                <span class="comment-meta-value">${value}</span>
            </span>
        `;
    };

    return function Comment({ comment, filePath, codeExcerpt, commentId, visibilityKey, isHidden, onToggleVisibility, onFirstRender, renderTimingLabel, vote, onVote, hunkRiskScore, hunkRiskDetail, onOpenRiskPanel }) {
        const [copied, setCopied] = useState(false);

        useEffect(() => {
            if (visibilityKey && onFirstRender) {
                onFirstRender(visibilityKey);
            }
        }, [visibilityKey, onFirstRender]);

        const handleCopy = async (e) => {
            e.stopPropagation();
            
            let copyText = '';
            if (filePath) {
                copyText += filePath;
                if (comment.Line) {
                    copyText += ':' + comment.Line;
                }
                copyText += '\n\n';
            }
            
            if (codeExcerpt) {
                copyText += 'Code excerpt:\n' + codeExcerpt + '\n\n';
            }
            
            copyText += 'Issue:\n' + comment.Content;
            
            try {
                await copyToClipboard(copyText);
                setCopied(true);
                setTimeout(() => setCopied(false), 2000);
            } catch (err) {
                console.error('Copy failed:', err);
            }
        };

        const handleToggleVisibility = (e) => {
            e.stopPropagation();
            if (!visibilityKey) {
                console.warn('Missing visibility key for comment toggle');
                return;
            }
            if (onToggleVisibility) {
                onToggleVisibility(visibilityKey);
            }
        };
        
        const badgeClass = getBadgeClass(comment.Severity);
        const lineLabel = comment.Line ? `:${comment.Line}` : '';
        const metaItems = [
            renderMetaItem('Confidence', comment.Confidence),
            renderMetaItem('Type', comment.Type),
            (comment.Category || comment.Subcategory)
                ? renderMetaItem('Classification', `${comment.Category || 'Uncategorized'}${comment.Subcategory ? ` / ${comment.Subcategory}` : ''}`, 'comment-meta-item-classification')
                : null,
        ].filter(Boolean);
        
        return html`
            <tr class="comment-row ${isHidden ? 'comment-row-hidden' : ''}" data-line="${comment.Line}" id="${commentId}">
                <td colspan="3">
                    <div class="comment-visibility-row">
                        ${isHidden
                            ? html`
                                <div class="comment-hidden-placeholder" style="position: relative;">
                                    <div class="comment-actions" style="display: flex; gap: 8px; position: absolute; right: 12px; top: 12px;">
                                        <button 
                                            class="comment-visibility-btn"
                                            title="Show this comment to the AI Agent"
                                            onClick=${handleToggleVisibility}
                                            style="position: static; opacity: 1;"
                                        >
                                            ${renderIcon(html, 'showComment')}
                                            Show
                                        </button>
                                    </div>
                                    <span class="comment-hidden-title">Comment hidden</span>
                                    <span class="comment-hidden-meta">${filePath}${lineLabel}</span>
                                    <span class="comment-hidden-note">Hidden comments are excluded from Copy Visible Issues and the Claude Agent.</span>
                                </div>
                            `
                            : html`
                                <div 
                                    class="comment-container"
                                    data-filepath="${filePath}"
                                    data-line="${comment.Line}"
                                    data-comment="${comment.Content}"
                                >
                                    <div class="comment-actions">
                                        ${typeof hunkRiskScore === 'number' && html`
                                            <${RiskBadge}
                                                score=${hunkRiskScore}
                                                detail=${hunkRiskDetail}
                                                size="large"
                                                onOpen=${onOpenRiskPanel}
                                            />
                                        `}
                                        <${FeedbackPopup}
                                            type="up"
                                            vote=${vote}
                                            onVote=${onVote}
                                            visibilityKey=${visibilityKey}
                                            commentContent=${comment.Content}
                                            codeExcerpt=${codeExcerpt}
                                            filePath=${filePath}
                                            severity=${comment.Severity}
                                            sourceType="comment"
                                        />
                                        <${FeedbackPopup}
                                            type="down"
                                            vote=${vote}
                                            onVote=${onVote}
                                            visibilityKey=${visibilityKey}
                                            commentContent=${comment.Content}
                                            codeExcerpt=${codeExcerpt}
                                            filePath=${filePath}
                                            severity=${comment.Severity}
                                            sourceType="comment"
                                        />
                                        <button
                                            class="comment-visibility-btn comment-action-icon-btn"
                                            title="Hide this comment from the AI Agent"
                                            onClick=${handleToggleVisibility}
                                        >
                                            ${renderIcon(html, 'hideComment')}
                                        </button>
                                        <button 
                                            class="comment-copy-btn comment-action-icon-btn ${copied ? 'copied' : ''}"
                                            title="Copy issue details"
                                            onClick=${handleCopy}
                                        >
                                            ${renderIcon(html, copied ? 'copied' : 'copyLogs')}
                                        </button>
                                    </div>
                                    <div class="comment-header">
                                        <div class="comment-header-main">
                                            <span class="comment-badge ${badgeClass}">${comment.Severity}</span>
                                            <span class="comment-location">${filePath}${lineLabel}</span>
                                            ${renderTimingLabel && html`
                                                <span class="comment-arrival">${renderTimingLabel}</span>
                                            `}
                                        </div>
                                    </div>
                                    ${metaItems.length > 0 && html`
                                        <div class="comment-meta-line">
                                            ${metaItems.map((item, index) => html`
                                                ${item}
                                                ${index < metaItems.length - 1 && html`<span class="comment-meta-divider">•</span>`}
                                            `)}
                                        </div>
                                    `}
                                    <div class="comment-body">${comment.Content}</div>
                                </div>
                            `
                        }
                    </div>
                </td>
            </tr>
        `;
    };
}

let CommentComponent = null;
export async function getComment() {
    if (!CommentComponent) {
        CommentComponent = await createComment();
    }
    return CommentComponent;
}
