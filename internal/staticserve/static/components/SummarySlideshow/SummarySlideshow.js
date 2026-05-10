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
    const seconds = Math.max(1, Math.round((Date.now() - startTime) / 1000));
    if (seconds < 60) {
        return `${seconds}s`;
    }
    const minutes = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return secs ? `${minutes}m ${secs}s` : `${minutes}m`;
}

function stripHtmlTags(input) {
    return (input || '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();
}

function resolveSlideTypography(slide) {
    const kind = slide?.kind || 'detail';
    const length = stripHtmlTags(slide?.content || '').length;

    if (kind === 'sentence') {
        if (length < 120) {
            return { fontSize: 'clamp(34px, 4.1vw, 52px)', lineHeight: '1.3', maxWidth: '28ch' };
        }
        return { fontSize: 'clamp(30px, 3.6vw, 44px)', lineHeight: '1.34', maxWidth: '34ch' };
    }

    if (kind === 'list') {
        return { fontSize: 'clamp(20px, 2.2vw, 27px)', lineHeight: '1.7', maxWidth: '64ch' };
    }

    if (kind === 'file-point' || kind === 'label-point') {
        return { fontSize: 'clamp(19px, 1.95vw, 25px)', lineHeight: '1.68', maxWidth: '70ch' };
    }

    if (kind === 'intro') {
        return { fontSize: 'clamp(36px, 4.5vw, 56px)', lineHeight: '1.14', maxWidth: '20ch' };
    }

    return { fontSize: 'clamp(18px, 1.8vw, 24px)', lineHeight: '1.72', maxWidth: '72ch' };
}

export async function createSummarySlideshow() {
    const { html, useEffect, useRef, useState } = await waitForPreact();

    return function SummarySlideshow({ markdown, isOpen = true, onClose = () => {}, mode = 'modal', isShortcutActive = false, className = '', initialSlideIndex = 0, onSlideIndexChange = () => {}, onOpenFileFromSlide = () => {}, canOpenFileFromSlide = () => false }) {
        const isModal = mode === 'modal';
        const isVisible = isModal ? isOpen : true;
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

        const clampSlideIndex = (value, length) => {
            if (!Number.isFinite(value)) {
                return 0;
            }
            const maxIndex = Math.max(0, length);
            return Math.max(0, Math.min(Math.floor(value), maxIndex));
        };

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
            if (!isVisible || !markdown) {
                return;
            }

            const parsedSlides = parseMarkdownToSlides(markdown);
            const startingIndex = clampSlideIndex(initialSlideIndex, parsedSlides.length);
            setSlides(parsedSlides);
            setCurrentSlide(startingIndex);
            setIsAutoPlay(false);
            setIsHelpShown(false);
            setCopied(false);
            setLiveMessage('');
            sessionStartRef.current = Date.now();
        }, [markdown, isVisible]);

        useEffect(() => {
            if (!slides.length) {
                return;
            }
            const nextIndex = clampSlideIndex(initialSlideIndex, slides.length);
            if (nextIndex !== currentSlide) {
                setCurrentSlide(nextIndex);
            }
        }, [initialSlideIndex, slides.length]);

        useEffect(() => {
            if (!isVisible || !slides.length) {
                return;
            }
            onSlideIndexChange(currentSlide);
        }, [currentSlide, isVisible, slides.length, onSlideIndexChange]);

        useEffect(() => {
            if (!isModal || !isVisible || !dialogRef.current) {
                return;
            }
            lastFocusedElementRef.current = document.activeElement;
            dialogRef.current.focus();
        }, [isModal, isVisible, slides.length]);

        useEffect(() => {
            if (!isVisible || (!isModal && !isShortcutActive)) {
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
                if (isModal && (!dialogRef.current || !dialogRef.current.contains(event.target))) {
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
                        if (isModal) {
                            handleClose();
                        } else {
                            setIsHelpShown(false);
                        }
                        break;
                    default:
                        moveToSlide(Math.min(parseInt(key, 10) - 1, slides.length - 1));
                        break;
                }
            };

            document.addEventListener('keydown', handler, true);
            return () => document.removeEventListener('keydown', handler, true);
        }, [isVisible, isModal, isShortcutActive, slides.length, currentSlide, isHelpShown, isAutoPlay]);

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
            if (!isVisible || !isAutoPlay || !slides.length || currentSlide >= slides.length) {
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
        }, [isAutoPlay, isVisible, slides, currentSlide]);

        const isAppreciation = currentSlide >= slides.length;
        const slide = !isAppreciation ? slides[currentSlide] : null;
        const progressValue = slides.length ? (isAppreciation ? 100 : ((currentSlide + 1) / slides.length) * 100) : 0;

        const handleClose = () => {
            clearAutoPlayTimers();
            setIsAutoPlay(false);
            setIsHelpShown(false);
            if (isModal && lastFocusedElementRef.current && typeof lastFocusedElementRef.current.focus === 'function') {
                lastFocusedElementRef.current.focus();
            }
            if (isModal) {
                onClose();
            }
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
            if (slide.meta?.kind === 'file-point') {
                const location = slide.meta.line ? `${slide.meta.filePath}:${slide.meta.line}` : slide.meta.filePath;
                copyParts.push(location);
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

        const handleOpenFile = (meta) => {
            if (!meta || !meta.filePath || typeof onOpenFileFromSlide !== 'function') {
                return;
            }

            const opened = onOpenFileFromSlide(meta.filePath, meta.line || null);
            if (!opened) {
                setLiveMessage('File path was not found in the current diff.');
            }
        };

        if (!isVisible || !slides.length) {
            return null;
        }

        const isIntro = !isAppreciation && slide?.kind === 'intro';
        const panelBg = isAppreciation ? '#1f2430' : (slide ? slide.color.surface : '#1f2430');
        const typography = slide ? resolveSlideTypography(slide) : null;

        const panel = html`
            ${isHelpShown && html`
                <div
                    class="summary-slideshow-help"
                    style="
                        position: absolute; inset: auto auto 32px 32px;
                        max-width: 360px; padding: 14px 16px;
                        border-radius: 12px; border: 1px solid rgba(148, 163, 184, 0.18);
                        background: rgba(14, 23, 42, 0.96); color: var(--text-secondary);
                        box-shadow: 0 18px 34px rgba(0, 0, 0, 0.28);
                        z-index: 2;
                    "
                    onClick=${(event) => event.stopPropagation()}
                >
                    <div style="display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 10px;">
                        <strong style="color: var(--text-primary); font-size: 13px;">Keyboard shortcuts</strong>
                        <button class="action-btn summary-slide-btn" onClick=${() => setIsHelpShown(false)} title="Close keyboard help" aria-label="Close keyboard help">
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
                        <div>${isModal ? 'Close: Q / Esc' : 'Hide help: Esc'}</div>
                    </div>
                </div>
            `}

            <div
                class="summary-slideshow-surface ${isModal ? '' : 'summary-slideshow-embedded-panel'}"
                style="
                    width: ${isModal ? 'min(960px, calc(100vw - 56px))' : '100%'};
                    min-height: ${isModal ? 'min(640px, calc(100vh - 56px))' : '380px'};
                    max-height: ${isModal ? 'calc(100vh - 56px)' : '700px'};
                    display: flex; flex-direction: column;
                    border-radius: 14px; overflow: hidden;
                    background: ${panelBg};
                    transition: background 220ms ease;
                    box-shadow: ${isModal ? '0 28px 72px rgba(0, 0, 0, 0.4)' : 'inset 0 0 0 1px rgba(0,0,0,0.06)'};
                    position: relative;
                "
                onClick=${(event) => event.stopPropagation()}
            >
                ${isModal && html`
                    <div class="summary-slideshow-chrome" style="display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 10px 16px; flex-shrink: 0;">
                        <div style="font-size: 12px; color: var(--text-muted); font-weight: 600; letter-spacing: 0.01em;">
                            Review slideshow
                        </div>
                        <button class="action-btn summary-slide-btn" onClick=${handleClose} title="Close slideshow (Esc)" aria-label="Close slideshow">
                            <svg width="16" height="16" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                            </svg>
                        </button>
                    </div>
                `}

                <div class="summary-slideshow-body" style="flex: 1; overflow-y: auto; display: flex; flex-direction: column; ${isIntro || isAppreciation ? 'align-items: center; justify-content: center;' : 'padding: 28px 32px;'}">
                    ${isAppreciation ? html`
                        <div class="summary-slideshow-complete" style="text-align: center; padding: 40px 32px; max-width: 520px;">
                            <div class="summary-slideshow-celebration" aria-hidden="true">
                                <svg viewBox="0 0 240 84" width="220" height="76">
                                    <circle cx="32" cy="24" r="5" fill="#4f8cff"/>
                                    <circle cx="58" cy="14" r="4" fill="#38b28a"/>
                                    <circle cx="86" cy="28" r="4" fill="#f5a524"/>
                                    <circle cx="152" cy="18" r="5" fill="#9a7bff"/>
                                    <circle cx="188" cy="30" r="4" fill="#ff6b94"/>
                                    <circle cx="212" cy="16" r="5" fill="#4f8cff"/>
                                    <rect x="106" y="14" width="28" height="28" rx="14" fill="#233046" stroke="#7fb3ff" stroke-width="2"/>
                                    <path d="M112 28l6 6 10-12" fill="none" stroke="#9ed8ff" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/>
                                </svg>
                            </div>
                            <div style="font-size: 34px; font-weight: 700; color: var(--text-primary); letter-spacing: -0.02em; margin-bottom: 12px;">
                                Review complete
                            </div>
                            <div style="font-size: 18px; color: var(--text-secondary); margin-bottom: 30px; line-height: 1.6;">
                                You finished all ${slides.length} slides.
                            </div>
                            <div style="font-size: 15px; color: var(--text-muted); margin-bottom: 20px; line-height: 1.6;">
                                Your commitment to higher engineering standards made this review possible.
                            </div>
                            <div style="margin-bottom: 6px;">
                                <span style="font-size: 30px; font-weight: 700; color: var(--text-primary); letter-spacing: -0.02em;">${formatActualElapsed(sessionStartRef.current)}</span>
                                <span style="font-size: 15px; color: var(--text-muted); margin-left: 8px;">actual</span>
                            </div>
                            <div style="font-size: 14px; color: var(--text-muted); margin-bottom: 40px;">
                                Planned: ${formatElapsed(slides)}
                            </div>
                            ${isModal && html`
                                <button class="action-btn summary-slide-btn" onClick=${handleClose} title="Close and return to review">
                                    <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M12 5l7 7-7 7" />
                                    </svg>
                                    Back to Review
                                </button>
                            `}
                        </div>
                    ` : html`
                        ${isIntro
                            ? html`
                                <div class="summary-slideshow-intro" style="text-align: center; padding: 42px 28px; max-width: min(760px, 100%);">
                                    <h1 style="margin: 0; font-size: ${typography.fontSize}; line-height: ${typography.lineHeight}; color: ${slide.color.title}; font-weight: 760; letter-spacing: -0.034em; max-width: ${typography.maxWidth}; margin-inline: auto; text-wrap: balance;">
                                        ${slide.title}
                                    </h1>
                                </div>
                            `
                            : slide.kind === 'file-point' && slide.meta
                                ? html`
                                    <div class="summary-file-point" style="max-width: ${typography.maxWidth}; width: 100%;">
                                        ${slide.title && html`
                                            <div class="summary-point-title" style="margin-bottom: 4px; font-size: 14px; font-weight: 700; letter-spacing: 0.01em; color: ${slide.color.accent};">
                                                ${slide.title}
                                            </div>
                                        `}
                                        ${canOpenFileFromSlide(slide.meta.filePath)
                                            ? html`
                                                <button
                                                    class="summary-file-chip summary-file-chip-interactive summary-path-chip"
                                                    data-tooltip="Open in diff: ${slide.meta.filePath}${slide.meta.line ? `:${slide.meta.line}` : ''}"
                                                    title="${slide.meta.filePath}${slide.meta.line ? `:${slide.meta.line}` : ''}"
                                                    onClick=${() => handleOpenFile(slide.meta)}
                                                >
                                                    ${slide.meta.filePath}${slide.meta.line ? `:${slide.meta.line}` : ''}
                                                </button>
                                            `
                                            : html`
                                                <code class="summary-file-inline-code">${slide.meta.filePath}${slide.meta.line ? `:${slide.meta.line}` : ''}</code>
                                            `
                                        }
                                        <div class="summary-file-description">${slide.content}</div>
                                    </div>
                                `
                                : slide.kind === 'label-point' && slide.meta
                                    ? html`
                                        <div class="summary-label-point" style="max-width: ${typography.maxWidth}; width: 100%;">
                                            ${slide.title && html`
                                                <div class="summary-point-title" style="margin-bottom: 4px; font-size: 14px; font-weight: 700; letter-spacing: 0.01em; color: ${slide.color.accent};">
                                                    ${slide.title}
                                                </div>
                                            `}
                                            <div class="summary-label-chip">${slide.meta.label}</div>
                                            <div class="summary-label-body">${slide.content}</div>
                                        </div>
                                    `
                            : html`
                                ${slide.title && html`
                                    <div style="margin-bottom: 16px; font-size: 14px; font-weight: 700; letter-spacing: 0.01em; color: ${slide.color.accent};">
                                        ${slide.title}
                                    </div>
                                `}
                                <div
                                    ref=${contentRef}
                                    class="summary-slideshow-content"
                                    style="
                                        color: ${slide.color.text};
                                        font-size: ${typography.fontSize};
                                        line-height: ${typography.lineHeight};
                                        letter-spacing: -0.01em;
                                        overflow-wrap: break-word;
                                        word-break: break-word;
                                        max-width: ${typography.maxWidth};
                                    "
                                ></div>
                            `}
                    `}
                </div>

                <div class="summary-slideshow-controls" style="padding: 10px 16px 12px 16px; flex-shrink: 0;">
                    <div style="display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 8px;">
                        <div style="display: flex; align-items: center; gap: 6px;">
                            <button class="action-btn summary-slide-btn" onClick=${prevSlide} title="Previous slide (H / K / Left Arrow)" aria-label="Previous slide" disabled=${currentSlide === 0 && !isAppreciation}>
                                <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
                                </svg>
                                Prev
                            </button>
                            <button class="action-btn summary-slide-btn" onClick=${nextSlide} title="Next slide (J / L / Right Arrow / Space)" aria-label="Next slide" disabled=${isAppreciation}>
                                <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                                </svg>
                                Next
                            </button>
                            <button class="action-btn summary-slide-btn ${isAutoPlay ? 'active' : ''}" onClick=${toggleAutoPlay} title="Toggle auto-play (A)" aria-label="Toggle auto-play">
                                <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                    ${isAutoPlay
                                        ? html`<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 9v6m4-6v6" />`
                                        : html`<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 6.82v10.36a1 1 0 001.53.848l8.25-5.18a1 1 0 000-1.696L9.53 5.972A1 1 0 008 6.82z" />`}
                                </svg>
                                ${buildAutoplayLabel(isAutoPlay, autoPlayRemainingMs)}
                            </button>
                        </div>

                        <div class="summary-slideshow-counter" style="font-size: 13px; min-width: 0; text-align: center;">
                            ${isAppreciation ? `${slides.length}/${slides.length} \u00b7 complete` : `${currentSlide + 1}/${slides.length} \u00b7 ${formatRemainingTime(slides, currentSlide)} left`}
                        </div>

                        <div style="display: flex; align-items: center; gap: 6px;">
                            <button class="action-btn summary-slide-btn ${copied ? 'copied' : ''}" onClick=${handleCopy} title="Copy current slide (C)" aria-label="Copy current slide">
                                <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                    ${copied
                                        ? html`<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />`
                                        : html`<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />`}
                                </svg>
                                ${copied ? 'Copied!' : 'Copy'}
                            </button>
                            <button class="action-btn summary-slide-btn" onClick=${() => setIsHelpShown(true)} title="Show keyboard shortcuts (?)" aria-label="Show keyboard shortcuts">
                                <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8.228 9c.549-1.165 1.918-2 3.522-2 2.071 0 3.75 1.343 3.75 3 0 1.268-.983 2.352-2.37 2.79-.488.154-.88.56-.88 1.012V15" />
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 18h.01" />
                                    <circle cx="12" cy="12" r="9" stroke-width="2" />
                                </svg>
                                Help
                            </button>
                        </div>
                    </div>

                    <div style="height: 4px; border-radius: 999px; overflow: hidden; background: rgba(148, 163, 184, 0.2);">
                        <div style="height: 100%; width: ${progressValue}%; background: ${slide ? slide.color.accent : '#3b82f6'}; transition: width 180ms ease; border-radius: 999px;"></div>
                    </div>

                    <div role="status" aria-live="polite" class="summary-slideshow-status" style="min-height: 16px; margin-top: 6px; font-size: 12px;">
                        ${liveMessage || (isAutoPlay && !isAppreciation ? `Auto-play \u00b7 next in ${Math.max(1, Math.ceil(autoPlayRemainingMs / 1000))}s` : '')}
                    </div>
                </div>
            </div>
        `;

        if (!isModal) {
            return html`
                <div
                    ref=${dialogRef}
                    class="summary-slideshow-embedded ${className}" 
                    tabIndex="0"
                >
                    ${panel}
                </div>
            `;
        }

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
                ${panel}
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
