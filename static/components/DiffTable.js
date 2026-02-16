// DiffTable component - renders diff hunks with lines and comments
import { waitForPreact, getBadgeClass, filePathToId } from './utils.js';
import { getComment } from './Comment.js';

export async function createDiffTable() {
    const { html } = await waitForPreact();
    const Comment = await getComment();
    
    return function DiffTable({ hunks, filePath, fileId, visibleSeverities }) {
        if (!hunks || hunks.length === 0) {
            return html`
                <div style="padding: 20px; text-align: center; color: #57606a;">
                    No diff content available.
                </div>
            `;
        }
        
        // Use provided fileId or generate from filePath
        const resolvedFileId = fileId || filePathToId(filePath);
        
        return html`
            <table class="diff-table">
                ${hunks.map(hunk => html`
                    <tr>
                        <td colspan="3" class="hunk-header">${hunk.Header}</td>
                    </tr>
                    ${hunk.Lines.map((line, idx) => {
                        // Get previous line content for code excerpt (for comments)
                        const prevLine = idx > 0 ? hunk.Lines[idx - 1] : null;
                        const codeExcerpt = prevLine ? prevLine.Content : '';
                        
                        return html`
                            <tr class="diff-line ${line.Class}">
                                <td class="line-num">${line.OldNum}</td>
                                <td class="line-num">${line.NewNum}</td>
                                <td class="line-content">${line.Content}</td>
                            </tr>
                            ${line.IsComment && line.Comments && line.Comments.map((comment, commentIdx) => {
                                const sev = (comment.Severity || '').toLowerCase();
                                if (visibleSeverities && !visibleSeverities.has(sev)) return null;
                                return html`
                                    <${Comment} 
                                        comment=${comment} 
                                        filePath=${filePath}
                                        codeExcerpt=${codeExcerpt}
                                        commentId=${`comment-${resolvedFileId}-${comment.Line}-${commentIdx}`}
                                    />
                                `;
                            })}
                        `;
                    })}
                `)}
            </table>
        `;
    };
}

let DiffTableComponent = null;
export async function getDiffTable() {
    if (!DiffTableComponent) {
        DiffTableComponent = await createDiffTable();
    }
    return DiffTableComponent;
}
