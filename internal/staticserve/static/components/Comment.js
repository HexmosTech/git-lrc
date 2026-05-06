// Comment component
import { waitForPreact, getBadgeClass, copyToClipboard } from './utils.js';

export async function createComment() {
    const { html, useState } = await waitForPreact();
    
    return function Comment({ comment, filePath, codeExcerpt, commentId, visibilityKey, isHidden, onToggleVisibility }) {
        const [copied, setCopied] = useState(false);

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
                                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 12s4-7 11-7 11 7 11 7-4 7-11 7S1 12 1 12z"/><circle cx="12" cy="12" r="3"/></svg>
                                            Show
                                        </button>
                                    </div>
                                    <span class="comment-hidden-title">Comment hidden</span>
                                    <span class="comment-hidden-meta">${filePath}${lineLabel}</span>
                                    <span class="comment-hidden-note">Hidden comments are excluded from Copy Visible Issues and Claude Agent.</span>
                                </div>
                            `
                            : html`
                                <div 
                                    class="comment-container"
                                    data-filepath="${filePath}"
                                    data-line="${comment.Line}"
                                    data-comment="${comment.Content}"
                                >
                                    <div class="comment-actions" style="display: flex; gap: 8px; position: absolute; right: 12px; top: 12px;">
                                        <button 
                                            class="comment-visibility-btn"
                                            title="Hide this comment from the AI Agent"
                                            onClick=${handleToggleVisibility}
                                            style="position: static; opacity: 1;"
                                        >
                                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17.94 17.94A10.07 10.07 0 0112 20c-5 0-9.27-3.11-11-7.5a11.8 11.8 0 012.89-4.11M9.88 9.88a3 3 0 104.24 4.24"/><path d="M1 1l22 22"/></svg>
                                            Hide
                                        </button>
                                        <button 
                                            class="comment-copy-btn ${copied ? 'copied' : ''}"
                                            title="Copy issue details"
                                            onClick=${handleCopy}
                                            style="position: static;"
                                        >
                                            ${copied ? 'Copied!' : 'Copy'}
                                        </button>
                                    </div>
                                    <div class="comment-header">
                                        <span class="comment-badge ${badgeClass}">${comment.Severity}</span>
                                        ${comment.HasCategory && html`
                                            <span class="comment-category">${comment.Category}</span>
                                        `}
                                    </div>
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
