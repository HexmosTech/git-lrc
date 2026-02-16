// SeverityFilter component - always-visible severity toggle buttons with counts
import { waitForPreact, countIssuesBySeverity } from './utils.js';

export async function createSeverityFilter() {
    const { html, useCallback } = await waitForPreact();

    return function SeverityFilter({ files, visibleSeverities, onToggleSeverity, onCopyVisibleIssues }) {
        const counts = countIssuesBySeverity(files, visibleSeverities);
        if (counts.total === 0) return null;

        const filterLabel = counts.visible === counts.total
            ? `${counts.total} issues`
            : `${counts.visible} of ${counts.total} visible`;

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
                <button class="btn btn-primary copy-visible-btn" onClick=${onCopyVisibleIssues} title="Copy all visible issues to clipboard">
                    <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                    </svg>
                    Copy Visible Issues
                </button>
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
