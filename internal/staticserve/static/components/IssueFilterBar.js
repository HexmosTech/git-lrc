import { waitForPreact } from './utils.js';
import { hasActiveIssueFilters } from './issue_filter_state.mjs';
import { getFeedbackPopup } from './FeedbackPopup.js';
import { getSendToAgentButton } from './SendToAgentButton.js';
import { renderIcon } from './icons.js';

function renderFacetSection(html, title, field, options, onToggleFilter) {
    if (!options || options.length === 0) {
        return null;
    }

    return html`
        <div class="issue-filter-group">
            <span class="issue-filter-group-label">${title}</span>
            <div class="issue-filter-chip-row">
                ${options.map((option) => html`
                    <button
                        class="issue-filter-chip ${field === 'severity' ? `severity-${option.value}` : ''} ${option.active ? 'active' : ''}"
                        onClick=${() => onToggleFilter(field, option.value)}
                        aria-pressed=${option.active ? 'true' : 'false'}
                        title="Toggle ${title.toLowerCase()} ${option.label}"
                    >
                        <span class="issue-filter-chip-label">${option.label}</span>
                        <span class="issue-filter-chip-count">${option.count}</span>
                    </button>
                `)}
            </div>
        </div>
    `;
}

function renderCategoryTree(html, categoryGroups, onToggleFilter) {
    if (!categoryGroups || categoryGroups.length === 0) {
        return null;
    }

    return html`
        <div class="issue-filter-group issue-filter-tree-group">
            <span class="issue-filter-group-label">Classification</span>
            <div class="issue-category-tree">
                ${categoryGroups.map((group) => html`
                    <div class="issue-category-branch ${group.active ? 'active' : ''}">
                        <button
                            class="issue-filter-chip issue-category-chip ${group.active ? 'active' : ''}"
                            onClick=${() => onToggleFilter('category', group.value)}
                            aria-pressed=${group.active ? 'true' : 'false'}
                            title="Toggle main category ${group.label}"
                        >
                            <span class="issue-filter-chip-label">${group.label}</span>
                            <span class="issue-filter-chip-count">${group.count}</span>
                        </button>
                        ${group.subcategories && group.subcategories.length > 0 && html`
                            <div class="issue-subcategory-tree">
                                ${group.subcategories.map((subcategory) => html`
                                    <button
                                        class="issue-filter-subchip ${subcategory.active ? 'active' : ''}"
                                        onClick=${() => onToggleFilter('subcategory', subcategory.value)}
                                        aria-pressed=${subcategory.active ? 'true' : 'false'}
                                        title="Toggle subcategory ${subcategory.label}"
                                    >
                                        <span class="issue-filter-chip-label">${subcategory.label}</span>
                                        <span class="issue-filter-chip-count">${subcategory.count}</span>
                                    </button>
                                `)}
                            </div>
                        `}
                    </div>
                `)}
            </div>
        </div>
    `;
}

export async function createIssueFilterBar() {
    const { html, useState } = await waitForPreact();
    const FeedbackPopup = await getFeedbackPopup();
    const SendToAgentButton = await getSendToAgentButton();

    return function IssueFilterBar({
        issueFilters,
        filterOptions,
        categoryGroups,
        filterCounts,
        filterSummary,
        onToggleFilter,
        onResetFilters,
        onCopyVisibleIssues,
        copyFeedbackStatus,
        copyFeedbackMessage,
        onSendToAgent,
        visibleCount,
        prVote,
        onVote,
    }) {
        if (!filterCounts || filterCounts.total === 0) {
            return null;
        }

        const [isPinnedOpen, setIsPinnedOpen] = useState(false);

        const hasActiveFilters = hasActiveIssueFilters(issueFilters);
        const filterLabel = filterCounts.visible === filterCounts.total
            ? `${filterCounts.total} issues visible`
            : `${filterCounts.visible} of ${filterCounts.total} visible`;
        const buttonState = copyFeedbackStatus && copyFeedbackStatus !== 'idle' ? copyFeedbackStatus : '';
        const buttonLabel = copyFeedbackStatus === 'success'
            ? 'Copied!'
            : copyFeedbackStatus === 'empty'
                ? 'No Visible Issues'
                : copyFeedbackStatus === 'error'
                    ? 'Copy Failed'
                    : 'Copy Visible Issues';

        return html`
            <div class="issue-filter-bar ${isPinnedOpen ? 'expanded' : ''}">
                <div class="issue-filter-main-row">
                    <div class="issue-filter-summary-block issue-filter-summary-block-collapsed">
                        <span class="issue-filter-title">Issue Filters</span>
                        <span class="issue-filter-summary-text">${filterLabel}</span>
                        ${filterSummary && filterSummary.length > 0 && html`
                            <span class="issue-filter-active-count">${filterSummary.length} active</span>
                        `}
                    </div>
                    <div class="issue-filter-toolbar-actions">
                        <button
                            class="issue-filter-expand-btn ${isPinnedOpen ? 'active' : ''}"
                            onClick=${() => setIsPinnedOpen((prev) => !prev)}
                            aria-expanded=${isPinnedOpen ? 'true' : 'false'}
                            title="Toggle expanded issue filters"
                        >
                            ${isPinnedOpen ? 'Hide Filters' : 'Open Filters'}
                        </button>
                        ${hasActiveFilters && html`
                            <button class="issue-filter-reset-btn" onClick=${onResetFilters} title="Reset all issue filters">
                                Reset Filters
                            </button>
                        `}
                        <div class="issue-filter-votes">
                            <${FeedbackPopup}
                                type="up"
                                vote=${prVote}
                                onVote=${onVote}
                                visibilityKey="__pr_level__"
                                sourceType="pr_level"
                            />
                            <${FeedbackPopup}
                                type="down"
                                vote=${prVote}
                                onVote=${onVote}
                                visibilityKey="__pr_level__"
                                sourceType="pr_level"
                            />
                        </div>
                        <div class="copy-visible-wrapper issue-filter-copy-actions">
                            <button
                                class="btn btn-primary copy-visible-btn ${buttonState}"
                                onClick=${onCopyVisibleIssues}
                                title="Copy all visible issues to clipboard"
                            >
                                ${renderIcon(html, buttonLabel === 'Copied!' ? 'copied' : 'copyLogs')}
                                ${buttonLabel}
                            </button>
                            <${SendToAgentButton}
                                visibleCount=${visibleCount}
                                onSendToAgent=${onSendToAgent}
                            />
                            ${copyFeedbackMessage && html`
                                <div class="copy-feedback copy-feedback-${copyFeedbackStatus}" role="status" aria-live="polite">
                                    ${copyFeedbackMessage}
                                </div>
                            `}
                        </div>
                    </div>
                </div>
                <div class="issue-filter-secondary-row">
                    <div class="issue-filter-quick-row">
                        <span class="issue-filter-quick-label">Severity</span>
                        <div class="issue-filter-chip-row issue-filter-chip-row-compact">
                            ${(filterOptions?.severities || []).map((option) => html`
                                <button
                                    class="issue-filter-chip issue-filter-chip-compact severity-${option.value} ${option.active ? 'active' : ''}"
                                    onClick=${() => onToggleFilter('severity', option.value)}
                                    aria-pressed=${option.active ? 'true' : 'false'}
                                    title="Toggle severity ${option.label}"
                                >
                                    <span class="issue-filter-chip-label">${option.label}</span>
                                    <span class="issue-filter-chip-count">${option.count}</span>
                                </button>
                            `)}
                        </div>
                    </div>
                </div>
                <div class="issue-filter-details">
                    <div class="issue-filter-details-header">
                        <div class="issue-filter-summary-block">
                            <span class="issue-filter-title">Issue Filters</span>
                            <span class="issue-filter-summary-text">${filterLabel}</span>
                            <span class="issue-filter-hint">Hover or open to browse all filter options</span>
                        </div>
                    </div>
                    <div class="issue-filter-groups">
                        ${renderFacetSection(html, 'Confidence', 'confidence', filterOptions?.confidences, onToggleFilter)}
                        ${renderFacetSection(html, 'Type', 'type', filterOptions?.types, onToggleFilter)}
                        ${renderCategoryTree(html, categoryGroups, onToggleFilter)}
                    </div>
                </div>
            </div>
        `;
    };
}

let IssueFilterBarComponent = null;
export async function getIssueFilterBar() {
    if (!IssueFilterBarComponent) {
        IssueFilterBarComponent = await createIssueFilterBar();
    }
    return IssueFilterBarComponent;
}