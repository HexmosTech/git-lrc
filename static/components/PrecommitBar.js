// PrecommitBar component - commit/push/skip actions
import { waitForPreact } from './utils.js';

// Extract the first markdown heading as a commit message suggestion
function extractTitleFromSummary(markdown) {
    if (!markdown) return '';
    const lines = markdown.split('\n');
    for (const line of lines) {
        const trimmed = line.trim();
        // Match any markdown heading (# Title, ## Title, etc.)
        const match = trimmed.match(/^#+\s+(.+)$/);
        if (match) {
            return match[1].trim();
        }
    }
    return '';
}

export async function createPrecommitBar() {
    const { html, useState, useEffect, useRef } = await waitForPreact();
    
    return function PrecommitBar({ interactive, isPostCommitReview, initialMsg, summary, status: reviewStatus }) {
        const [message, setMessage] = useState(initialMsg || '');
        const [status, setStatus] = useState('');
        const [disabled, setDisabled] = useState(false);
        const userHasTyped = useRef(!!(initialMsg && initialMsg.trim()));
        
        // Auto-fill commit message from AI summary title when review completes
        useEffect(() => {
            if (reviewStatus === 'completed' && !userHasTyped.current && !message.trim()) {
                const title = extractTitleFromSummary(summary);
                if (title) {
                    setMessage(title);
                }
            }
        }, [reviewStatus, summary]);
        
        if (!interactive) return null;
        
        // Post-commit review mode - just show info
        if (isPostCommitReview) {
            return html`
                <div class="precommit-bar">
                    <div style="padding: 12px; color: var(--text-muted); font-size: 13px;">
                        <p>📖 Viewing historical commit review. Press <strong>Ctrl-C</strong> in the terminal to exit.</p>
                    </div>
                </div>
            `;
        }
        
        const postDecision = async (path, successText, requireMessage) => {
            if (requireMessage && !message.trim()) {
                setStatus('Commit message is required');
                return;
            }
            
            setDisabled(true);
            setStatus('Sending decision...');
            
            try {
                const res = await fetch(path, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ message })
                });
                
                if (!res.ok) throw new Error('Request failed: ' + res.status);
                setStatus(successText + ' — you can now return to the terminal.');
            } catch (err) {
                setStatus('Failed: ' + err.message);
                setDisabled(false);
            }
        };
        
        return html`
            <div class="precommit-bar">
                <div class="precommit-bar-left">
                    <div class="precommit-bar-title">Git action</div>
                    <div class="precommit-actions">
                        <button 
                            class="btn btn-primary"
                            disabled=${disabled}
                            onClick=${() => postDecision('/commit', 'Commit requested', true)}
                        >
                            <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                            </svg>
                            Commit
                        </button>
                        <button 
                            class="btn btn-primary"
                            disabled=${disabled}
                            onClick=${() => postDecision('/commit-push', 'Commit and push requested', true)}
                        >
                            <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
                            </svg>
                            Commit & Push
                        </button>
                        <button 
                            class="btn btn-ghost"
                            disabled=${disabled}
                            onClick=${() => postDecision('/skip', 'Skip requested', false)}
                        >
                            <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                            </svg>
                            Skip
                        </button>
                    </div>
                    <div class="precommit-status">${status}</div>
                </div>
                <div class="precommit-message">
                    <label for="commit-message">Commit message</label>
                    <textarea 
                        id="commit-message"
                        placeholder="Enter your commit message"
                        value=${message}
                        disabled=${disabled}
                        onInput=${(e) => {
                            setMessage(e.target.value);
                            userHasTyped.current = true;
                        }}
                    ></textarea>
                    <div class="precommit-message-hint">Required for commit actions; ignored on Skip.</div>
                </div>
            </div>
        `;
    };
}

let PrecommitBarComponent = null;
export async function getPrecommitBar() {
    if (!PrecommitBarComponent) {
        PrecommitBarComponent = await createPrecommitBar();
    }
    return PrecommitBarComponent;
}
