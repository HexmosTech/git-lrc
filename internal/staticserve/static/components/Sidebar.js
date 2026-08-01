// Sidebar component
import { renderIcon } from './icons.js';
import { waitForPreact, filePathToId } from './utils.js';
import { countFileVisibleIssues } from './issue_filter_state.mjs';
import { blastRadiusTier } from './blast_radius_sort_state.mjs';

export async function createSidebar() {
    const preact = await waitForPreact();
    const { html, useState, useCallback } = preact;

    // In the whole-diff risk view a file's hunks are scattered through the
    // ranked stream, so each file expands into "Hunk n" entries that jump to
    // the corresponding block. hunkNav: FilePath -> [{targetId, expandKey,
    // hunkNum, score}] (null/empty outside that view).
    //
    // Each file header is collapsible when it has hunk entries (default
    // collapsed), keeping the list scannable for big diffs. Clicking the
    // file header still jumps to the file; the caret toggles the submenu.
    return function Sidebar({ files, activeFileId, onFileClick, onHunkClick, hunkNav, issueFilters, hiddenCommentKeys, open, onClose }) {
        const totalFiles = files.length;
        const totalComments = files.reduce((sum, file) => sum + countFileVisibleIssues(file, issueFilters, hiddenCommentKeys), 0);
        // Set of expanded file paths; entries only apply when hunkNav is
        // populated (whole-diff risk view). Default expanded = empty set so
        // the hunk submenus start collapsed.
        const [expandedFiles, setExpandedFiles] = useState(() => new Set());
        const toggleFile = useCallback((filePath, e) => {
            e.stopPropagation();
            setExpandedFiles(prev => {
                const next = new Set(prev);
                if (next.has(filePath)) {
                    next.delete(filePath);
                } else {
                    next.add(filePath);
                }
                return next;
            });
        }, []);

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
                        const hunkEntries = (hunkNav && hunkNav[file.FilePath]) || [];
                        const isExpanded = expandedFiles.has(file.FilePath);

                        return html`
                            <div
                                class="sidebar-file ${isActive ? 'active' : ''} ${hunkEntries.length > 0 ? 'sidebar-file-collapsible' : ''} ${isExpanded ? 'expanded' : ''}"
                                data-file-id="${fileId}"
                                onClick=${() => onFileClick(fileId)}
                            >
                                ${hunkEntries.length > 0 && html`
                                    <span
                                        class="sidebar-file-caret"
                                        onClick=${(e) => toggleFile(file.FilePath, e)}
                                        aria-expanded=${isExpanded}
                                        title="${isExpanded ? 'Collapse hunks' : 'Expand hunks'}"
                                    >${isExpanded ? '▾' : '▸'}</span>
                                `}
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
                            ${hunkEntries.length > 0 && isExpanded && html`
                                <div class="sidebar-hunk-list">
                                    ${hunkEntries.map(entry => html`
                                        <div
                                            key=${entry.targetId}
                                            class="sidebar-hunk"
                                            onClick=${() => onHunkClick && onHunkClick(entry.targetId, entry.expandKey)}
                                            title="Jump to hunk ${entry.hunkNum} of ${file.FilePath} — risk ${typeof entry.score === 'number' ? Math.round(entry.score) : '–'}/100${entry.commentCount ? `, ${entry.commentCount} comment${entry.commentCount !== 1 ? 's' : ''}` : ''}"
                                        >
                                            <span class="sidebar-hunk-name">Hunk ${entry.hunkNum}</span>
                                            <span class="sidebar-hunk-meta">
                                                ${entry.commentCount > 0 && html`
                                                    <span class="sidebar-file-badge sidebar-hunk-comments">${entry.commentCount}</span>
                                                `}
                                                ${typeof entry.score === 'number' && html`
                                                    <span class="sidebar-hunk-score ${blastRadiusTier(entry.score)}">
                                                        ${renderIcon(html, 'blastRadius', { size: 9 })} ${Math.round(entry.score)}
                                                    </span>
                                                `}
                                            </span>
                                        </div>
                                    `)}
                                </div>
                            `}
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
