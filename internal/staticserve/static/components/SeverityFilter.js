// SeverityFilter component - always-visible severity toggle buttons with counts
import { waitForPreact, countIssuesBySeverity } from './utils.js';

export async function createSeverityFilter() {
    const { html } = await waitForPreact();

    return function SeverityFilter({ 
        files, 
        visibleSeverities, 
        onToggleSeverity, 
        onCopyVisibleIssues, 
        hiddenCommentKeys,
        copyFeedbackStatus,
        copyFeedbackMessage,
        onSendToAgent,
        visibleCount
    }) {
        const counts = countIssuesBySeverity(files, visibleSeverities, hiddenCommentKeys);
        if (counts.total === 0) return null;

        const filterLabel = counts.visible === counts.total
            ? `${counts.total} issues`
            : `${counts.visible} of ${counts.total} visible`;

        const buttonState = copyFeedbackStatus && copyFeedbackStatus !== 'idle' ? copyFeedbackStatus : '';
        const buttonLabel = copyFeedbackStatus === 'success'
            ? 'Copied!'
            : copyFeedbackStatus === 'empty'
                ? 'No Visible Issues'
                : copyFeedbackStatus === 'error'
                    ? 'Copy Failed'
                    : 'Copy Visible Issues';

        return html`
            <div class="severity-filter-bar">
                <div class="severity-filters">
                    <button
                        class="severity-filter-btn critical ${visibleSeverities.has('critical') ? 'active' : ''}"
                        onClick=${() => onToggleSeverity('critical')}
                        title="Toggle critical issues"
                    >
                        Critical
                        <span class="filter-badge">${counts.critical}</span>
                    </button>
                    <button
                        class="severity-filter-btn error ${visibleSeverities.has('error') ? 'active' : ''}"
                        onClick=${() => onToggleSeverity('error')}
                        title="Toggle error issues"
                    >
                        Error
                        <span class="filter-badge">${counts.error}</span>
                    </button>
                    <button
                        class="severity-filter-btn warning ${visibleSeverities.has('warning') ? 'active' : ''}"
                        onClick=${() => onToggleSeverity('warning')}
                        title="Toggle warning issues"
                    >
                        Warning
                        <span class="filter-badge">${counts.warning}</span>
                    </button>
                    <button
                        class="severity-filter-btn info ${visibleSeverities.has('info') ? 'active' : ''}"
                        onClick=${() => onToggleSeverity('info')}
                        title="Toggle info issues"
                    >
                        Info
                        <span class="filter-badge">${counts.info}</span>
                    </button>
                </div>
                <span class="severity-filter-summary">${filterLabel}</span>
                <div class="copy-visible-wrapper" style="flex-direction: row; align-items: center; gap: 8px;">
                    <button 
                        class="btn btn-primary copy-visible-btn ${buttonState}"
                        onClick=${onCopyVisibleIssues} 
                        title="Copy all visible issues to clipboard"
                    >
                        <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                        </svg>
                        ${buttonLabel}
                    </button>
                    <button
                        class="btn btn-primary"
                        onClick=${onSendToAgent}
                        title="Send visible issues to Claude"
                    >
                        Send to Claude (${visibleCount})
                    </button>
                    ${copyFeedbackMessage && html`
                        <div class="copy-feedback copy-feedback-${copyFeedbackStatus}" role="status" aria-live="polite">
                            ${copyFeedbackMessage}
                        </div>
                    `}
                </div>
            </div>
        `;
    };
}

let SeverityFilterComponent = null;
export async function getSeverityFilter() {
    if (!SeverityFilterComponent) {
        SeverityFilterComponent = await createSeverityFilter();
    }
    return SeverityFilterComponent;
}
