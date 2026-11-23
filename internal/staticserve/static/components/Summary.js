// Summary component - renders markdown summary
import { renderIcon } from './icons.js';
import { waitForPreact } from './utils.js';
import { getSummarySlideshow } from './SummarySlideshow/SummarySlideshow.js';
import { getQuiz } from './Quiz.js';

const ALLOWED_TAGS = new Set([
    'A', 'BLOCKQUOTE', 'BR', 'CAPTION', 'CODE', 'COL', 'COLGROUP', 'EM', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6',
    'HR', 'LI', 'OL', 'P', 'PRE', 'STRONG', 'TABLE', 'TBODY', 'TD', 'TFOOT', 'TH', 'THEAD', 'TR', 'UL'
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

function sanitizeError(err) {
    if (!err) return { short: '', full: '' };
    const full = typeof err === 'string' ? err : String(err);
    let short = full;
    if (full.startsWith('{') || full.startsWith('[')) {
        try {
            const parsed = JSON.parse(full);
            short = parsed.error || parsed.message || parsed.envelope?.error || 'Unknown server error';
        } catch { /* not valid JSON, show as-is */ }
    }
    if (short.length > 500) short = short.slice(0, 500) + '…';
    return { short, full };
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
    if (!filePath || !/\.[A-Za-z0-9]+$/.test(filePath)) {
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

    const candidateNodes = Array.from(container.querySelectorAll('code, strong'));
    candidateNodes.forEach((node) => {
        if (node.tagName === 'CODE' && node.closest('pre')) {
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
    const Quiz = await getQuiz();

    return function Summary({ markdown, status, errorSummary, showAllClear, slidesEnabled = true, isSlideshowModalOpen, onOpenSlideshowModal, onEmbeddedShortcutActiveChange, slideIndex = 0, onSlideIndexChange = () => {}, onOpenFileFromSlide = () => {}, canOpenFileFromSlide = () => false, quiz = [], onViewModeChange = () => {} }) {
        const contentRef = useRef(null);
        const summaryRootRef = useRef(null);
        const [summaryViewMode, setSummaryViewMode] = useState(slidesEnabled ? 'slides' : 'text');
        const [isSummaryInView, setIsSummaryInView] = useState(false);
        const [showViewToggleAttention, setShowViewToggleAttention] = useState(false);
        const [errorExpanded, setErrorExpanded] = useState(false);
        const hasPlayedAttentionRef = useRef(false);
        const hasSummaryMarkdown = Boolean(markdown && markdown.trim());
        const hasQuiz = Array.isArray(quiz) && quiz.length > 0;

        useEffect(() => {
            renderSafeMarkdown(contentRef.current, markdown, { onOpenFileFromSlide, canOpenFileFromSlide });
        }, [markdown, onOpenFileFromSlide, canOpenFileFromSlide]);

        useEffect(() => {
            if (hasSummaryMarkdown) {
                setSummaryViewMode(slidesEnabled ? 'slides' : 'text');
            }
        }, [markdown, hasSummaryMarkdown, slidesEnabled]);

        useEffect(() => {
            onViewModeChange(summaryViewMode);
        }, [summaryViewMode, onViewModeChange]);

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

        // Draw attention to the Slides/Text/Quiz toggle once, the first
        // time it's actually scrolled into view — not on mount, since the
        // summary is often below the fold when a review first completes.
        useEffect(() => {
            if (!isSummaryInView || hasPlayedAttentionRef.current) {
                return;
            }
            if (!hasSummaryMarkdown || !(slidesEnabled || hasQuiz)) {
                return;
            }
            hasPlayedAttentionRef.current = true;
            setShowViewToggleAttention(true);
            const timer = setTimeout(() => setShowViewToggleAttention(false), 1800);
            return () => clearTimeout(timer);
        }, [isSummaryInView, hasSummaryMarkdown, slidesEnabled, hasQuiz]);

        const embeddedShortcutsActive = Boolean(
            hasSummaryMarkdown
            && slidesEnabled
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
                            ${(slidesEnabled || hasQuiz)
                                ? html`
                                    <div class="summary-view-toggle ${showViewToggleAttention ? 'summary-view-toggle-attention' : ''}" role="group" aria-label="Summary display mode">
                                        ${slidesEnabled && html`
                                            <button
                                                class="action-btn summary-view-btn ${summaryViewMode === 'slides' ? 'active' : ''}"
                                                onClick=${() => setSummaryViewMode('slides')}
                                                title="Show slides view"
                                                aria-label="Show slides view"
                                                aria-pressed=${summaryViewMode === 'slides'}
                                            >
                                                ${renderIcon(html, 'slidesView', { className: 'btn-icon' })}
                                                Slides
                                            </button>
                                        `}
                                        <button
                                            class="action-btn summary-view-btn ${summaryViewMode === 'text' ? 'active' : ''}"
                                            onClick=${() => setSummaryViewMode('text')}
                                            title="Show text view"
                                            aria-label="Show text view"
                                            aria-pressed=${summaryViewMode === 'text'}
                                        >
                                            ${renderIcon(html, 'textView', { className: 'btn-icon' })}
                                            Text
                                        </button>
                                        ${hasQuiz && html`
                                            <button
                                                class="action-btn summary-view-btn ${summaryViewMode === 'quiz' ? 'active' : ''}"
                                                onClick=${() => setSummaryViewMode('quiz')}
                                                title="Show comprehension quiz"
                                                aria-label="Show comprehension quiz"
                                                aria-pressed=${summaryViewMode === 'quiz'}
                                            >
                                                ${renderIcon(html, 'help', { className: 'btn-icon' })}
                                                Quiz
                                            </button>
                                        `}
                                    </div>
                                `
                                : html`<span style="font-size: 12px; color: var(--text-muted);">Text view</span>`
                            }
                        </div>
                        <div class="summary-header-center" aria-hidden="true">
                            Summary
                        </div>
                        <div class="summary-actions">
                            ${slidesEnabled && html`
                                <button class="action-btn summary-play-btn" onClick=${onOpenSlideshowModal} title="Open slides in dialog" aria-label="Open slides in dialog">
                                    ${renderIcon(html, 'openSlides')}
                                    Open Slides
                                </button>
                            `}
                        </div>
                    </div>
                `}
                ${showAllClear && html`
                    <div class="summary-all-clear" role="status" aria-live="polite">
                        <div class="summary-all-clear-icon" aria-hidden="true">${renderIcon(html, 'successStatus', { size: 18 })}</div>
                        <div class="summary-all-clear-copy">
                            <strong class="summary-all-clear-title">Good to go</strong>
                            <p class="summary-all-clear-text">This review finished without any review comments. No issues were found in the reviewed diff.</p>
                        </div>
                    </div>
                `}
                ${isError && (() => {
                    const err = sanitizeError(errorSummary) || { short: 'Review failed', full: 'Review failed' };
                    return html`
                        <div style="padding: 16px; background: #fef2f2; border: 1px solid #fecaca; border-radius: 6px; color: #991b1b; margin-bottom: 16px;">
                            <strong style="display: block; margin-bottom: 8px; font-size: 16px;">${renderIcon(html, 'errorStatus', { className: 'btn-icon', size: 16 })}Error Details:</strong>
                            <pre style="white-space: pre-wrap; font-family: monospace; font-size: 13px; margin: 0;">${errorExpanded ? err.full : err.short}</pre>
                            ${err.full !== err.short && html`
                                <button
                                    style="display: inline-block; margin-top: 8px; padding: 0; background: none; border: none; color: #3b82f6; font-size: 12px; cursor: pointer; text-decoration: underline; text-underline-offset: 2px;"
                                    onClick=${() => setErrorExpanded(v => !v)}
                                    type="button"
                                >${errorExpanded ? 'Show less' : 'Show full error'}</button>
                            `}
                        </div>
                    `;
                })()}

                ${hasSummaryMarkdown && slidesEnabled && summaryViewMode === 'slides' && html`
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
                            hasQuiz=${hasQuiz}
                            onTakeQuiz=${() => setSummaryViewMode('quiz')}
                        />
                    </div>
                `}

                ${hasSummaryMarkdown && hasQuiz && summaryViewMode === 'quiz' && html`
                    <div class="summary-quiz-container">
                        <${Quiz} quiz=${quiz} />
                    </div>
                `}

                <div
                    ref=${contentRef}
                    class="summary-text-content"
                    style=${hasSummaryMarkdown && summaryViewMode === 'text' ? '' : 'display: none;'}
                ></div>
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
