// Summary component - renders markdown summary
import { waitForPreact } from './utils.js';

const ALLOWED_TAGS = new Set([
    'A', 'BLOCKQUOTE', 'BR', 'CODE', 'EM', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6',
    'HR', 'LI', 'OL', 'P', 'PRE', 'STRONG', 'UL'
]);

const SAFE_URL_PROTOCOLS = new Set(['http:', 'https:', 'mailto:']);

function isSafeHref(href) {
    if (!href) {
        return false;
    }
    try {
        const parsed = new URL(href, window.location.origin);
        return SAFE_URL_PROTOCOLS.has(parsed.protocol);
    } catch {
        return false;
    }
}

function copyAllowedAttributes(source, target) {
    if (source.tagName === 'A') {
        const href = source.getAttribute('href') || '';
        if (isSafeHref(href)) {
            target.setAttribute('href', href);
            target.setAttribute('rel', 'noopener noreferrer');
            target.setAttribute('target', '_blank');
        }
    }

    if (source.tagName === 'CODE') {
        const className = source.getAttribute('class') || '';
        if (/^[a-z0-9 _-]+$/i.test(className)) {
            target.setAttribute('class', className);
        }
    }
}

function sanitizeNode(node) {
    if (node.nodeType === Node.TEXT_NODE) {
        return document.createTextNode(node.textContent || '');
    }

    if (node.nodeType !== Node.ELEMENT_NODE) {
        return null;
    }

    const source = node;
    if (!ALLOWED_TAGS.has(source.tagName)) {
        return document.createTextNode(source.textContent || '');
    }

    const target = document.createElement(source.tagName.toLowerCase());
    copyAllowedAttributes(source, target);

    for (const child of source.childNodes) {
        const sanitizedChild = sanitizeNode(child);
        if (sanitizedChild) {
            target.appendChild(sanitizedChild);
        }
    }

    return target;
}

function renderSafeMarkdown(container, markdown) {
    if (!container) {
        return;
    }

    const rawMarkdown = markdown || '';
    if (typeof marked === 'undefined') {
        container.textContent = rawMarkdown;
        return;
    }

    const renderedHTML = marked.parse(rawMarkdown, { mangle: false, headerIds: false });
    const parsed = new DOMParser().parseFromString(renderedHTML, 'text/html');
    const fragment = document.createDocumentFragment();

    for (const child of parsed.body.childNodes) {
        const sanitizedChild = sanitizeNode(child);
        if (sanitizedChild) {
            fragment.appendChild(sanitizedChild);
        }
    }

    container.replaceChildren(fragment);
}

export async function createSummary() {
    const { html, useEffect, useRef } = await waitForPreact();
    
    return function Summary({ markdown, status, errorSummary, showAllClear, showPlayAction, onPlaySlideshow }) {
        const contentRef = useRef(null);
        
        useEffect(() => {
            renderSafeMarkdown(contentRef.current, markdown);
        }, [markdown]);
        
        const isError = status === 'failed' || errorSummary;
        
        return html`
            <div class="summary" id="summary-content">
                ${showPlayAction && html`
                    <div class="summary-header-row">
                        <div class="summary-header-spacer"></div>
                        <div class="summary-actions">
                            <button class="action-btn summary-play-btn" onClick=${onPlaySlideshow} title="View summary as presentation slides" aria-label="View summary as presentation slides">
                                <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 6.82v10.36a1 1 0 001.53.848l8.25-5.18a1 1 0 000-1.696L9.53 5.972A1 1 0 008 6.82z" />
                                </svg>
                                Play Review
                            </button>
                        </div>
                    </div>
                `}
                ${showAllClear && html`
                    <div class="summary-all-clear" role="status" aria-live="polite">
                        <div class="summary-all-clear-icon" aria-hidden="true">✓</div>
                        <div class="summary-all-clear-copy">
                            <strong class="summary-all-clear-title">Good to go</strong>
                            <p class="summary-all-clear-text">This review finished without any review comments. No issues were found in the reviewed diff.</p>
                        </div>
                    </div>
                `}
                ${isError && html`
                    <div style="padding: 16px; background: #fef2f2; border: 1px solid #fecaca; border-radius: 6px; color: #991b1b; margin-bottom: 16px;">
                        <strong style="display: block; margin-bottom: 8px; font-size: 16px;">⚠️ Error Details:</strong>
                        <pre style="white-space: pre-wrap; font-family: monospace; font-size: 13px; margin: 0;">
                            ${errorSummary || 'Review failed'}
                        </pre>
                    </div>
                `}
                <div ref=${contentRef} style=${markdown && markdown.trim() ? '' : 'display: none;'}></div>
            </div>
        `;
    };
}

let SummaryComponent = null;
export async function getSummary() {
    if (!SummaryComponent) {
        SummaryComponent = await createSummary();
    }
    return SummaryComponent;
}
