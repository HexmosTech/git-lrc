// Toolbar component - tabs and action buttons
import { renderIcon } from './icons.js';
import { waitForPreact } from './utils.js';

export async function createToolbar() {
    const { html } = await waitForPreact();
    
    return function Toolbar({
        activeTab,
        onTabChange,
        performanceItems,
        allExpanded,
        onToggleAll,
        showBlastRadiusToggle,
        sortByBlastRadius,
        onToggleSortByBlastRadius,
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
                        ${showBlastRadiusToggle && html`
                            <button
                                class="action-btn ${sortByBlastRadius ? 'active' : ''}"
                                onClick=${onToggleSortByBlastRadius}
                                title="${sortByBlastRadius ? 'Show hunks in diff order' : 'Sort hunks within each file by blast radius (highest first)'}"
                            >
                                ${renderIcon(html, 'blastRadius')}
                                ${sortByBlastRadius ? 'Diff Order' : 'Sort: Blast Radius'}
                            </button>
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
