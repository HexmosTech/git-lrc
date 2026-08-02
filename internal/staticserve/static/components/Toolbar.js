// Toolbar component - tabs and action buttons
import { renderIcon } from './icons.js';
import { waitForPreact } from './utils.js';
import { SORT_MODE_DIFF, SORT_MODE_RISK_FILE, SORT_MODE_RISK_FLAT } from './blast_radius_sort_state.mjs';

const SORT_MODE_OPTIONS = [
    { mode: SORT_MODE_RISK_FLAT, label: 'Score: Whole', title: 'One ranked stream: every hunk across the whole diff ordered by risk score, highest first' },
    { mode: SORT_MODE_RISK_FILE, label: 'Score: Per file', title: 'Keep files together; order hunks inside each file by risk score' },
    { mode: SORT_MODE_DIFF, label: 'Diff order', title: 'Original diff order: files and hunks as they appear in the diff' },
];

export async function createToolbar() {
    const { html } = await waitForPreact();

    return function Toolbar({
        activeTab,
        onTabChange,
        performanceItems,
        allExpanded,
        onToggleAll,
        showRiskSortControl,
        sortMode,
        onSortModeChange,
        eventCount,
        showEventBadge,
        onTailLog,
        isTailing,
        onCopyLogs,
        logsCopied,
    }) {
        return html`
            <div class="toolbar-row">
                <div class="view-tabs">
                    <button 
                        class="tab-btn ${activeTab === 'files' ? 'active' : ''}"
                        data-tab="files"
                        onClick=${() => onTabChange('files')}
                    >
                        ${renderIcon(html, 'filesTab')}
                        Files & Comments
                    </button>
                    <button 
                        class="tab-btn ${activeTab === 'events' ? 'active' : ''}"
                        data-tab="events"
                        onClick=${() => onTabChange('events')}
                    >
                        ${renderIcon(html, 'eventsTab')}
                        Event Log
                        ${showEventBadge && eventCount > 0 && html`
                            <span class="notification-badge">${eventCount}</span>
                        `}
                    </button>
                </div>

                ${Array.isArray(performanceItems) && performanceItems.length > 0 && html`
                    <div class="toolbar-performance" aria-label="Review performance summary">
                        ${performanceItems.map(item => html`
                            <div class="performance-pill" data-performance-key=${item.key}>
                                <span class="performance-pill-label">${item.label}</span>
                                <span class="performance-pill-value">${item.value}</span>
                            </div>
                        `)}
                    </div>
                `}
                
                ${activeTab === 'files' && html`
                    <div class="tab-actions">
                        ${showRiskSortControl && html`
                            <div class="sort-mode-group" role="group" aria-label="Order hunks by">
                                <span class="sort-mode-label">${renderIcon(html, 'blastRadius', { size: 12 })} Order By</span>
                                ${SORT_MODE_OPTIONS.map(opt => html`
                                    <button
                                        key=${opt.mode}
                                        class="sort-mode-btn ${sortMode === opt.mode ? 'active' : ''}"
                                        onClick=${() => onSortModeChange(opt.mode)}
                                        title="${opt.title}"
                                    >
                                        ${opt.label}
                                    </button>
                                `)}
                            </div>
                        `}
                        <button class="action-btn" onClick=${onToggleAll} title="${allExpanded ? 'Collapse all file blocks' : 'Expand all file blocks'}">
                            ${renderIcon(html, allExpanded ? 'collapseFiles' : 'expandFiles')}
                            ${allExpanded ? 'Collapse All' : 'Expand All'}
                        </button>
                    </div>
                `}
                
                ${activeTab === 'events' && html`
                    <div class="tab-actions">
                        <button class="action-btn ${isTailing ? 'active' : ''}" onClick=${onTailLog} title="Scroll to bottom and follow new logs">
                            ${renderIcon(html, 'tailLog')}
                            ${isTailing ? 'Tailing...' : 'Tail Log'}
                        </button>
                        <button class="action-btn ${logsCopied ? 'copied' : ''}" onClick=${onCopyLogs} title="Copy all logs to clipboard">
                            ${renderIcon(html, logsCopied ? 'copied' : 'copyLogs')}
                            ${logsCopied ? 'Copied!' : 'Copy Logs'}
                        </button>
                    </div>
                `}
            </div>
        `;
    };
}

let ToolbarComponent = null;
export async function getToolbar() {
    if (!ToolbarComponent) {
        ToolbarComponent = await createToolbar();
    }
    return ToolbarComponent;
}
