// Summary component - renders markdown summary
import { waitForPreact } from './utils.js';
import { getSummarySlideshow } from './SummarySlideshow/SummarySlideshow.js';

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

function parseFullPathToken(pathToken) {
    const trimmed = (pathToken || '').trim();
    const match = trimmed.match(/^(.*?)(?::(\d+))?$/);
    if (!match) {
        return null;
    }

    const filePath = (match[1] || '').trim();
    if (!filePath || !filePath.includes('/') || !/\.[A-Za-z0-9]+$/.test(filePath)) {
        return null;
    }

    const line = match[2] ? Number(match[2]) : null;
    return {
        filePath,
        line,
        display: line ? `${filePath}:${line}` : filePath
    };
}

function enhanceTextWithFileChips(container, handlers = {}) {
    if (!container) {
        return;
    }

    const onOpenFileFromSlide = handlers.onOpenFileFromSlide;
    const canOpenFileFromSlide = handlers.canOpenFileFromSlide;

    const codeNodes = Array.from(container.querySelectorAll('code'));
    codeNodes.forEach((node) => {
        if (node.closest('pre')) {
            return;
        }

        const parsed = parseFullPathToken(node.textContent || '');
        if (!parsed) {
            return;
        }

        if (typeof canOpenFileFromSlide !== 'function' || !canOpenFileFromSlide(parsed.filePath)) {
            return;
        }

        const chip = document.createElement('button');
        chip.setAttribute('type', 'button');
        chip.setAttribute('class', 'summary-file-chip summary-file-chip-interactive summary-inline-file-chip summary-path-chip');
        chip.setAttribute('data-tooltip', `Open in diff: ${parsed.display}`);
        chip.setAttribute('title', parsed.display);
        chip.textContent = parsed.display;
        chip.addEventListener('click', (event) => {
            event.preventDefault();
            event.stopPropagation();
            if (typeof onOpenFileFromSlide === 'function') {
                onOpenFileFromSlide(parsed.filePath, parsed.line || null);
            }
        });

        node.replaceWith(chip);
    });
}

function renderSafeMarkdown(container, markdown, handlers = {}) {
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
    enhanceTextWithFileChips(container, handlers);
}

export async function createSummary() {
    const { html, useEffect, useRef, useState } = await waitForPreact();
    const SummarySlideshow = await getSummarySlideshow();
    
    return function Summary({ markdown, status, errorSummary, showAllClear, isSlideshowModalOpen, onOpenSlideshowModal, onEmbeddedShortcutActiveChange, slideIndex = 0, onSlideIndexChange = () => {}, onOpenFileFromSlide = () => {}, canOpenFileFromSlide = () => false }) {
        const contentRef = useRef(null);
        const summaryRootRef = useRef(null);
        const [summaryViewMode, setSummaryViewMode] = useState('slides');
        const [isSummaryInView, setIsSummaryInView] = useState(false);
        const hasSummaryMarkdown = Boolean(markdown && markdown.trim());
        
        useEffect(() => {
            renderSafeMarkdown(contentRef.current, markdown, { onOpenFileFromSlide, canOpenFileFromSlide });
        }, [markdown, onOpenFileFromSlide, canOpenFileFromSlide]);

        useEffect(() => {
            if (hasSummaryMarkdown) {
                setSummaryViewMode('slides');
            }
        }, [markdown, hasSummaryMarkdown]);

        useEffect(() => {
            const element = summaryRootRef.current;
            if (!element || typeof IntersectionObserver === 'undefined') {
                setIsSummaryInView(true);
                return undefined;
            }

            const observer = new IntersectionObserver((entries) => {
                const entry = entries[0];
                setIsSummaryInView(Boolean(entry?.isIntersecting));
            }, { threshold: 0.35 });

            observer.observe(element);
            return () => observer.disconnect();
        }, []);

        const embeddedShortcutsActive = Boolean(
            hasSummaryMarkdown
            && summaryViewMode === 'slides'
            && !isSlideshowModalOpen
            && isSummaryInView
        );

        useEffect(() => {
            if (typeof onEmbeddedShortcutActiveChange === 'function') {
                onEmbeddedShortcutActiveChange(embeddedShortcutsActive);
            }
            return () => {
                if (typeof onEmbeddedShortcutActiveChange === 'function') {
                    onEmbeddedShortcutActiveChange(false);
                }
            };
        }, [embeddedShortcutsActive, onEmbeddedShortcutActiveChange]);
        
        const isError = status === 'failed' || errorSummary;
        
        return html`
            <div class="summary" id="summary-content" ref=${summaryRootRef}>
                ${hasSummaryMarkdown && html`
                    <div class="summary-header-row">
                        <div class="summary-header-left">
                            <div class="summary-view-toggle" role="group" aria-label="Summary display mode">
                                <button
                                    class="action-btn summary-view-btn ${summaryViewMode === 'slides' ? 'active' : ''}"
                                    onClick=${() => setSummaryViewMode('slides')}
                                    title="Show slides view"
                                    aria-label="Show slides view"
                                    aria-pressed=${summaryViewMode === 'slides'}
                                >
                                    Slides
                                </button>
                                <button
                                    class="action-btn summary-view-btn ${summaryViewMode === 'text' ? 'active' : ''}"
                                    onClick=${() => setSummaryViewMode('text')}
                                    title="Show text view"
                                    aria-label="Show text view"
                                    aria-pressed=${summaryViewMode === 'text'}
                                >
                                    Text
                                </button>
                            </div>
                        </div>
                        <div class="summary-header-center" aria-hidden="true">
                            Summary
                        </div>
                        <div class="summary-actions">
                            <button class="action-btn summary-play-btn" onClick=${onOpenSlideshowModal} title="Open slides in dialog" aria-label="Open slides in dialog">
                                <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 4h12v12M8 20h12M4 8v12h12" />
                                </svg>
                                Open Slides
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

                ${hasSummaryMarkdown && summaryViewMode === 'slides' && html`
                    <div class="summary-embedded-container">
                        <${SummarySlideshow}
                            markdown=${markdown}
                            mode="embedded"
                            isShortcutActive=${embeddedShortcutsActive}
                            initialSlideIndex=${slideIndex}
                            onSlideIndexChange=${onSlideIndexChange}
                            onOpenFileFromSlide=${onOpenFileFromSlide}
                            canOpenFileFromSlide=${canOpenFileFromSlide}
                            className="summary-embedded-slideshow"
                        />
                    </div>
                `}

                <div ref=${contentRef} style=${hasSummaryMarkdown && summaryViewMode === 'text' ? '' : 'display: none;'}></div>
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
