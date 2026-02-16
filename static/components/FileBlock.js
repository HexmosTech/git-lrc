// FileBlock component - collapsible file with diff
import { waitForPreact, filePathToId, countVisibleComments } from './utils.js';
import { getDiffTable } from './DiffTable.js';

export async function createFileBlock() {
    const { html } = await waitForPreact();
    const DiffTable = await getDiffTable();
    
    return function FileBlock({ file, expanded, onToggle, visibleSeverities }) {
        // Use file.ID if available (set by convertFilesToUIFormat), otherwise generate
        const fileId = file.ID || filePathToId(file.FilePath);
        
        const visibleCount = countVisibleComments(file, visibleSeverities);
        
        return html`
            <div 
                class="file ${expanded ? 'expanded' : 'collapsed'}"
                id="${fileId}"
                data-has-comments="${file.HasComments}"
                data-filepath="${file.FilePath}"
            >
                <div class="file-header" onClick=${() => onToggle(fileId)}>
                    <span class="toggle"></span>
                    <span class="filename">${file.FilePath}</span>
                    ${visibleCount > 0 && html`
                        <span class="comment-count">${visibleCount}</span>
                    `}
                </div>
                <div class="file-content">
                    <${DiffTable} hunks=${file.Hunks} filePath=${file.FilePath} fileId=${fileId} visibleSeverities=${visibleSeverities} />
                </div>
            </div>
        `;
    };
}

let FileBlockComponent = null;
export async function getFileBlock() {
    if (!FileBlockComponent) {
        FileBlockComponent = await createFileBlock();
    }
    return FileBlockComponent;
}
