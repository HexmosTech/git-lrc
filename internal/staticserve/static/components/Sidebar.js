// Sidebar component
import { renderIcon } from './icons.js';
import { waitForPreact, filePathToId } from './utils.js';
import { countFileVisibleIssues } from './issue_filter_state.mjs';

export async function createSidebar() {
    const { html } = await waitForPreact();
    
    return function Sidebar({ files, activeFileId, onFileClick, issueFilters, hiddenCommentKeys, open, onClose }) {
        const totalFiles = files.length;
        const totalComments = files.reduce((sum, file) => sum + countFileVisibleIssues(file, issueFilters, hiddenCommentKeys), 0);

        return html`
            ${open && html`<div class="sidebar-backdrop" onClick=${onClose}></div>`}
            <div class="sidebar ${open ? 'open' : ''}">
                <div class="sidebar-header">
                    <h2>
                        ${renderIcon(html, 'filesTab', { size: 12, className: 'btn-icon' })}
                        Files
                    </h2>
                    <div class="sidebar-stats">
                        ${totalFiles} file${totalFiles !== 1 ? 's' : ''} • ${totalComments} comment${totalComments !== 1 ? 's' : ''}
                    </div>
                </div>
                <div class="sidebar-content">
                    ${files.map(file => {
                        const fileId = filePathToId(file.FilePath);
                        const isActive = activeFileId === fileId;
                        
                        return html`
                            <div 
                                class="sidebar-file ${isActive ? 'active' : ''}"
                                data-file-id="${fileId}"
                                onClick=${() => onFileClick(fileId)}
                            >
                                <span class="sidebar-file-name" title="${file.FilePath}">
                                    ${file.FilePath}
                                </span>
                                ${(() => {
                                    const badgeCount = countFileVisibleIssues(file, issueFilters, hiddenCommentKeys);
                                    return badgeCount > 0 && html`
                                        <span class="sidebar-file-badge">${badgeCount}</span>
                                    `;
                                })()}
                            </div>
                        `;
                    })}
                </div>
            </div>
        `;
    };
}

let SidebarComponent = null;
export async function getSidebar() {
    if (!SidebarComponent) {
        SidebarComponent = await createSidebar();
    }
    return SidebarComponent;
}
