import { calculateTotalReadTime, formatRemainingTime, parseMarkdownToSlides } from './slideshowParser.js';
import { copyToClipboard, waitForPreact } from '../utils.js';

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

    const rawContent = markdown || '';
    const looksLikeHtml = /^\s*<(?:[a-z][\w:-]*|!doctype)\b/i.test(rawContent);
    const renderedHTML = looksLikeHtml || typeof marked === 'undefined'
        ? rawContent
        : marked.parse(rawContent, { mangle: false, headerIds: false, gfm: true, breaks: true });

    const parsed = new DOMParser().parseFromString(`<div id="summary-render-root">${renderedHTML}</div>`, 'text/html');
    const root = parsed.getElementById('summary-render-root');
    const fragment = document.createDocumentFragment();

    for (const child of root.childNodes) {
        const sanitizedChild = sanitizeNode(child);
        if (sanitizedChild) {
            fragment.appendChild(sanitizedChild);
        }
    }

    container.replaceChildren(fragment);
}

function buildAutoplayLabel(isAutoPlay, remainingMs) {
    if (!isAutoPlay) {
        return 'Auto-play';
    }

    const seconds = Math.max(1, Math.ceil(remainingMs / 1000));
    return `Playing · ${seconds}s`;
}

function formatElapsed(slides) {
    const totalSeconds = calculateTotalReadTime(slides);
    if (totalSeconds < 60) {
        return `${totalSeconds}s`;
    }
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    return seconds ? `${minutes}m ${seconds}s` : `${minutes}m`;
}

function formatActualElapsed(startTime) {
    if (!startTime) {
        return '—';
    }
    const seconds = Math.round((Date.now() - startTime) / 1000);
    if (seconds < 60) {
        return `${seconds}s`;
    }
    const minutes = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return secs ? `${minutes}m ${secs}s` : `${minutes}m`;
}

export async function createSummarySlideshow() {
    const { html, useEffect, useRef, useState } = await waitForPreact();

    return function SummarySlideshow({ markdown, isOpen, onClose }) {
        const [slides, setSlides] = useState([]);
        const [currentSlide, setCurrentSlide] = useState(0);
        const [isAutoPlay, setIsAutoPlay] = useState(false);
        const [isHelpShown, setIsHelpShown] = useState(false);
        const [copied, setCopied] = useState(false);
        const [liveMessage, setLiveMessage] = useState('');
        const [autoPlayRemainingMs, setAutoPlayRemainingMs] = useState(0);
        const contentRef = useRef(null);
        const dialogRef = useRef(null);
        const lastFocusedElementRef = useRef(null);
        const autoPlayTimerRef = useRef(null);
        const autoPlayTickRef = useRef(null);
        const copyTimerRef = useRef(null);
        const sessionStartRef = useRef(null);

        const clearAutoPlayTimers = () => {
            if (autoPlayTimerRef.current) {
                clearTimeout(autoPlayTimerRef.current);
                autoPlayTimerRef.current = null;
            }
            if (autoPlayTickRef.current) {
                clearInterval(autoPlayTickRef.current);
                autoPlayTickRef.current = null;
            }
            setAutoPlayRemainingMs(0);
        };

        useEffect(() => {
            if (!isOpen || !markdown) {
                return;
            }

            const parsedSlides = parseMarkdownToSlides(markdown);
            setSlides(parsedSlides);
            setCurrentSlide(0);
            setIsAutoPlay(false);
            setIsHelpShown(false);
            setCopied(false);
            setLiveMessage('');
            sessionStartRef.current = Date.now();
        }, [markdown, isOpen]);

        useEffect(() => {
            if (!isOpen || !dialogRef.current) {
                return;
            }
            lastFocusedElementRef.current = document.activeElement;
            dialogRef.current.focus();
        }, [isOpen, slides.length]);

        useEffect(() => {
            if (!isOpen) {
                return;
            }

            const isEditableTarget = (target) => {
                if (!target || !target.tagName) {
                    return false;
                }

                const tag = target.tagName.toLowerCase();
                return tag === 'input' || tag === 'textarea' || tag === 'select' || target.isContentEditable;
            };

            const handler = (event) => {
                if (!dialogRef.current || !dialogRef.current.contains(event.target)) {
                    return;
                }

                if (isEditableTarget(event.target)) {
                    return;
                }

                const key = event.key.toLowerCase();
                if (!/^[1-9]$/.test(key) && !['arrowleft', 'arrowright', 'arrowup', 'arrowdown', 'h', 'j', 'k', 'l', ' ', '?', 'a', 'c', 'escape', 'q'].includes(key)) {
                    return;
                }

                event.preventDefault();
                event.stopPropagation();

                if (isHelpShown && (key === '?' || key === 'escape')) {
                    setIsHelpShown(false);
                    return;
                }

                switch (key) {
                    case 'arrowleft':
                    case 'h':
                    case 'k':
                        prevSlide();
                        break;
                    case 'arrowright':
                    case 'l':
                    case 'j':
                    case ' ':
                    case 'arrowdown':
                        nextSlide();
                        break;
                    case 'a':
                        toggleAutoPlay();
                        break;
                    case 'c':
                        handleCopy();
                        break;
                    case '?':
                        setIsHelpShown(true);
                        break;
                    case 'escape':
                    case 'q':
                        handleClose();
                        break;
                    default:
                        moveToSlide(Math.min(parseInt(key, 10) - 1, slides.length - 1));
                        break;
                }
            };

            document.addEventListener('keydown', handler, true);
            return () => document.removeEventListener('keydown', handler, true);
        }, [isOpen, slides.length, currentSlide, isHelpShown, isAutoPlay]);

        useEffect(() => {
            if (!slides.length || currentSlide >= slides.length) {
                return;
            }

            const slide = slides[currentSlide];
            renderSafeMarkdown(contentRef.current, slide.content);
        }, [slides, currentSlide]);

        useEffect(() => () => {
            clearAutoPlayTimers();
            if (copyTimerRef.current) {
                clearTimeout(copyTimerRef.current);
            }
        }, []);

        useEffect(() => {
            if (!isOpen || !isAutoPlay || !slides.length || currentSlide >= slides.length) {
                clearAutoPlayTimers();
                return;
            }

            const delayMs = slides[currentSlide].readTime * 1000;
            const deadline = Date.now() + delayMs;
            setAutoPlayRemainingMs(delayMs);

            autoPlayTimerRef.current = setTimeout(() => {
                setCurrentSlide(prev => {
                    if (prev >= slides.length - 1) {
                        return slides.length;
                    }
                    return prev + 1;
                });
            }, delayMs);

            autoPlayTickRef.current = setInterval(() => {
                setAutoPlayRemainingMs(Math.max(0, deadline - Date.now()));
            }, 250);

            return () => clearAutoPlayTimers();
        }, [isAutoPlay, isOpen, slides, currentSlide]);

        const isAppreciation = currentSlide >= slides.length;
        const slide = !isAppreciation ? slides[currentSlide] : null;
        const progressValue = slides.length ? (isAppreciation ? 100 : ((currentSlide + 1) / slides.length) * 100) : 0;

        const handleClose = () => {
            clearAutoPlayTimers();
            setIsAutoPlay(false);
            setIsHelpShown(false);
            if (lastFocusedElementRef.current && typeof lastFocusedElementRef.current.focus === 'function') {
                lastFocusedElementRef.current.focus();
            }
            onClose();
        };

        const moveToSlide = (nextIndex) => {
            clearAutoPlayTimers();
            setCurrentSlide(nextIndex);
        };

        const nextSlide = () => {
            if (currentSlide >= slides.length - 1) {
                moveToSlide(slides.length);
                return;
            }
            moveToSlide(currentSlide + 1);
        };

        const prevSlide = () => {
            if (isAppreciation) {
                moveToSlide(slides.length - 1);
                return;
            }
            moveToSlide(Math.max(0, currentSlide - 1));
        };

        const handleCopy = async () => {
            if (!slide) {
                return;
            }

            const copyParts = [];
            if (slide.title) {
                copyParts.push(slide.title);
            }
            if (slide.content) {
                copyParts.push(slide.content);
            }

            try {
                await copyToClipboard(copyParts.join('\n\n'));
                setCopied(true);
                setLiveMessage('Copied current slide to clipboard.');
                if (copyTimerRef.current) {
                    clearTimeout(copyTimerRef.current);
                }
                copyTimerRef.current = setTimeout(() => setCopied(false), 2000);
            } catch (error) {
                console.error('Failed to copy slide:', error);
                setLiveMessage('Copy failed.');
            }
        };

        const toggleAutoPlay = () => {
            setIsAutoPlay(prev => !prev);
            setLiveMessage(isAutoPlay ? 'Auto-play paused.' : 'Auto-play started.');
        };

        if (!isOpen || !slides.length) {
            return null;
        }

        const isIntro = !isAppreciation && slide?.kind === 'intro';
        const panelBg = isAppreciation ? '#f8fafc' : (slide ? slide.color.surface : '#f8fafc');

        return html`
            <div
                ref=${dialogRef}
                role="dialog"
                aria-modal="true"
                aria-label="Review summary slideshow"
                style="
                    position: fixed; inset: 0; z-index: 10000;
                    display: flex; align-items: center; justify-content: center;
                    background: rgba(2, 6, 23, 0.68);
                    padding: 28px;
                "
                onClick=${(event) => event.target === event.currentTarget && handleClose()}
                tabIndex="-1"
            >
                ${isHelpShown && html`
                    <div
                        style="
                            position: absolute; inset: auto auto 32px 32px;
                            max-width: 360px; padding: 16px 18px;
                            border-radius: 12px; border: 1px solid rgba(148, 163, 184, 0.18);
                            background: rgba(14, 23, 42, 0.96); color: var(--text-secondary);
                            box-shadow: 0 18px 34px rgba(0, 0, 0, 0.28);
                        "
                        onClick=${(event) => event.stopPropagation()}
                    >
                        <div style="display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 10px;">
                            <strong style="color: var(--text-primary); font-size: 13px;">Keyboard shortcuts</strong>
                            <button class="action-btn" onClick=${() => setIsHelpShown(false)} title="Close keyboard help" aria-label="Close keyboard help">
                                <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                                </svg>
                            </button>
                        </div>
                        <div style="font-size: 12px; line-height: 1.8; color: var(--text-secondary);">
                            <div>Previous: \u2190 / H / K</div>
                            <div>Next: \u2192 / L / J / Space</div>
                            <div>Jump: 1-9</div>
                            <div>Auto-play: A</div>
                            <div>Copy: C</div>
                            <div>Close: Q / Esc</div>
                        </div>
                    </div>
                `}

                <div
                    style="
                        width: min(960px, calc(100vw - 56px));
                        min-height: min(640px, calc(100vh - 56px));
                        max-height: calc(100vh - 56px);
                        display: flex; flex-direction: column;
                        border-radius: 12px; overflow: hidden;
                        background: ${panelBg};
                        transition: background 220ms ease;
                        box-shadow: 0 24px 64px rgba(15, 23, 42, 0.22);
                    "
                    onClick=${(event) => event.stopPropagation()}
                >
                    <div style="display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 10px 16px; background: rgba(255,255,255,0.72); border-bottom: 1px solid rgba(0,0,0,0.07); flex-shrink: 0;">
                        <div style="font-size: 12px; color: #64748b; font-weight: 600; letter-spacing: 0.01em;">
                            Review slideshow
                        </div>
                        <button class="action-btn" onClick=${handleClose} title="Close slideshow (Esc)" aria-label="Close slideshow">
                            <svg width="16" height="16" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                            </svg>
                        </button>
                    </div>

                    <div style="flex: 1; overflow-y: auto; display: flex; flex-direction: column; ${isIntro || isAppreciation ? 'align-items: center; justify-content: center;' : 'padding: 40px 48px;'}">
                        ${isAppreciation ? html`
                            <div style="text-align: center; padding: 48px 40px; max-width: 520px;">
                                <div style="font-size: 32px; font-weight: 700; color: #0f172a; letter-spacing: -0.02em; margin-bottom: 12px;">
                                    Review complete
                                </div>
                                <div style="font-size: 16px; color: #475569; margin-bottom: 36px; line-height: 1.6;">
                                    You finished all ${slides.length} slides.
                                </div>
                                <div style="margin-bottom: 6px;">
                                    <span style="font-size: 28px; font-weight: 700; color: #0f172a; letter-spacing: -0.02em;">${formatActualElapsed(sessionStartRef.current)}</span>
                                    <span style="font-size: 14px; color: #64748b; margin-left: 8px;">actual</span>
                                </div>
                                <div style="font-size: 13px; color: #94a3b8; margin-bottom: 40px;">
                                    Planned: ${formatElapsed(slides)}
                                </div>
                                <button class="action-btn" onClick=${handleClose} title="Close and return to review">
                                    <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M12 5l7 7-7 7" />
                                    </svg>
                                    Back to Review
                                </button>
                            </div>
                        ` : html`
                            ${isIntro
                                ? html`
                                    <div style="text-align: center; padding: 48px 40px; max-width: 640px;">
                                        <h1 style="margin: 0; font-size: clamp(30px, 4vw, 48px); line-height: 1.16; color: ${slide.color.title}; font-weight: 700; letter-spacing: -0.03em;">
                                            ${slide.title}
                                        </h1>
                                    </div>
                                `
                                : html`
                                    ${slide.title && html`
                                        <div style="margin-bottom: 16px; font-size: 12px; font-weight: 700; letter-spacing: 0.01em; color: ${slide.color.accent};">
                                            ${slide.title}
                                        </div>
                                    `}
                                    <div
                                        ref=${contentRef}
                                        style="
                                            color: ${slide.color.text};
                                            font-size: ${slide.kind === 'sentence' ? '28px' : slide.kind === 'list' ? '20px' : '18px'};
                                            line-height: ${slide.kind === 'sentence' ? '1.38' : '1.68'};
                                            letter-spacing: -0.01em;
                                            overflow-wrap: break-word;
                                            word-break: break-word;
                                            max-width: 100%;
                                        "
                                    ></div>
                                `}
                        `}
                    </div>

                    <div style="padding: 10px 16px 12px 16px; border-top: 1px solid rgba(0,0,0,0.07); background: rgba(255,255,255,0.72); flex-shrink: 0;">
                        <div style="display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 8px;">
                            <div style="display: flex; align-items: center; gap: 6px;">
                                <button class="action-btn" onClick=${prevSlide} title="Previous slide (H / K / Left Arrow)" aria-label="Previous slide" disabled=${currentSlide === 0 && !isAppreciation}>
                                    <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
                                    </svg>
                                    Prev
                                </button>
                                <button class="action-btn" onClick=${nextSlide} title="Next slide (J / L / Right Arrow / Space)" aria-label="Next slide" disabled=${isAppreciation}>
                                    <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                                    </svg>
                                    Next
                                </button>
                                <button class="action-btn ${isAutoPlay ? 'active' : ''}" onClick=${toggleAutoPlay} title="Toggle auto-play (A)" aria-label="Toggle auto-play">
                                    <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                        ${isAutoPlay
                                            ? html`<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 9v6m4-6v6" />`
                                            : html`<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 6.82v10.36a1 1 0 001.53.848l8.25-5.18a1 1 0 000-1.696L9.53 5.972A1 1 0 008 6.82z" />`}
                                    </svg>
                                    ${buildAutoplayLabel(isAutoPlay, autoPlayRemainingMs)}
                                </button>
                            </div>

                            <div style="font-size: 12px; color: #64748b; min-width: 0; text-align: center;">
                                ${isAppreciation ? `${slides.length}/${slides.length} \u00b7 complete` : `${currentSlide + 1}/${slides.length} \u00b7 ${formatRemainingTime(slides, currentSlide)} left`}
                            </div>

                            <div style="display: flex; align-items: center; gap: 6px;">
                                <button class="action-btn ${copied ? 'copied' : ''}" onClick=${handleCopy} title="Copy current slide (C)" aria-label="Copy current slide">
                                    <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                        ${copied
                                            ? html`<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />`
                                            : html`<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />`}
                                    </svg>
                                    ${copied ? 'Copied!' : 'Copy'}
                                </button>
                                <button class="action-btn" onClick=${() => setIsHelpShown(true)} title="Show keyboard shortcuts (?)" aria-label="Show keyboard shortcuts">
                                    <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8.228 9c.549-1.165 1.918-2 3.522-2 2.071 0 3.75 1.343 3.75 3 0 1.268-.983 2.352-2.37 2.79-.488.154-.88.56-.88 1.012V15" />
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 18h.01" />
                                        <circle cx="12" cy="12" r="9" stroke-width="2" />
                                    </svg>
                                    Help
                                </button>
                            </div>
                        </div>

                        <div style="height: 3px; border-radius: 999px; overflow: hidden; background: rgba(148, 163, 184, 0.2);">
                            <div style="height: 100%; width: ${progressValue}%; background: ${slide ? slide.color.accent : '#3b82f6'}; transition: width 180ms ease; border-radius: 999px;"></div>
                        </div>

                        <div role="status" aria-live="polite" style="min-height: 16px; margin-top: 6px; font-size: 11px; color: #94a3b8;">
                            ${liveMessage || (isAutoPlay && !isAppreciation ? `Auto-play \u00b7 next in ${Math.max(1, Math.ceil(autoPlayRemainingMs / 1000))}s` : '')}
                        </div>
                    </div>
                </div>
            </div>
        `;
    };
}

let SummarySlideshowComponent = null;
export async function getSummarySlideshow() {
    if (!SummarySlideshowComponent) {
        SummarySlideshowComponent = await createSummarySlideshow();
    }
    return SummarySlideshowComponent;
}
