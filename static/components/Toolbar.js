// Toolbar component - tabs and action buttons
import { waitForPreact } from './utils.js';

export async function createToolbar() {
    const { html } = await waitForPreact();
    
    return function Toolbar({ 
        activeTab, 
        onTabChange, 
        allExpanded, 
        onToggleAll, 
        eventCount,
        showEventBadge,
        onTailLog,
        isTailing,
        onCopyLogs,
        logsCopied
    }) {
        return html`
            <div class="toolbar-row">
                <div class="view-tabs">
                    <button 
                        class="tab-btn ${activeTab === 'files' ? 'active' : ''}"
                        data-tab="files"
                        onClick=${() => onTabChange('files')}
                    >
                        <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
                        </svg>
                        Files & Comments
                    </button>
                    <button 
                        class="tab-btn ${activeTab === 'events' ? 'active' : ''}"
                        data-tab="events"
                        onClick=${() => onTabChange('events')}
                    >
                        <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
                        </svg>
                        Event Log
                        ${showEventBadge && eventCount > 0 && html`
                            <span class="notification-badge">${eventCount}</span>
                        `}
                    </button>
                </div>
                
                ${activeTab === 'files' && html`
                    <div class="tab-actions">
                        <button class="action-btn" onClick=${onToggleAll} title="${allExpanded ? 'Collapse all file blocks' : 'Expand all file blocks'}">
                            <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                ${allExpanded 
                                    ? html`<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12H4" />`
                                    : html`<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 8V4m0 0h4M4 4l5 5m11-1V4m0 0h-4m4 0l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4" />`
                                }
                            </svg>
                            ${allExpanded ? 'Collapse All' : 'Expand All'}
                        </button>
                    </div>
                `}
                
                ${activeTab === 'events' && html`
                    <div class="tab-actions">
                        <button class="action-btn ${isTailing ? 'active' : ''}" onClick=${onTailLog} title="Scroll to bottom and follow new logs">
                            <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 14l-7 7m0 0l-7-7m7 7V3" />
                            </svg>
                            ${isTailing ? 'Tailing...' : 'Tail Log'}
                        </button>
                        <button class="action-btn ${logsCopied ? 'copied' : ''}" onClick=${onCopyLogs} title="Copy all logs to clipboard">
                            <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                ${logsCopied 
                                    ? html`<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />`
                                    : html`<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />`
                                }
                            </svg>
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
