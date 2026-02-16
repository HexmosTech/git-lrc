// Sidebar component
import { waitForPreact, filePathToId, countVisibleComments } from './utils.js';

export async function createSidebar() {
    const { html } = await waitForPreact();
    
    return function Sidebar({ files, activeFileId, onFileClick, visibleSeverities }) {
        const totalFiles = files.length;
        const totalComments = files.reduce((sum, file) => sum + countVisibleComments(file, visibleSeverities), 0);
        
        return html`
            <div class="sidebar">
                <div class="sidebar-header">
                    <h2>📂 FILES</h2>
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
                                    const badgeCount = countVisibleComments(file, visibleSeverities);
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
