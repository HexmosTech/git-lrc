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

function getSafeRenderedHtml(markdown) {
    const rawContent = markdown || '';
    const looksLikeHtml = /^\s*<(?:[a-z][\w:-]*|!doctype)\b/i.test(rawContent);
    const renderedHTML = looksLikeHtml || typeof marked === 'undefined'
        ? rawContent
        : marked.parse(rawContent, { mangle: false, headerIds: false, gfm: true, breaks: true });

    const parsed = new DOMParser().parseFromString(`<div id="summary-render-root">${renderedHTML}</div>`, 'text/html');
    const root = parsed.getElementById('summary-render-root');
    if (!root) {
        return '';
    }

    const container = document.createElement('div');

    for (const child of root.childNodes) {
        const sanitizedChild = sanitizeNode(child);
        if (sanitizedChild) {
            container.appendChild(sanitizedChild);
        }
    }

    return container.innerHTML;
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

function truncatePathDisplay(filePath, maxDisplayChars = 52) {
    const path = filePath || '';
    if (path.length <= maxDisplayChars) {
        return path;
    }
    // Take the last (maxDisplayChars-1) chars, then snap forward to the next slash
    // so we never show a partial directory segment.
    const suffix = path.slice(-(maxDisplayChars - 1));
    const slashPos = suffix.indexOf('/');
    const clean = slashPos >= 0 ? suffix.slice(slashPos) : `/${suffix}`;
    return `\u2026${clean}`;
}

function resolveSlideTypography(slide) {
    const kind = slide?.kind || 'detail';

    if (kind === 'intro') {
        return { fontSize: 'clamp(38px, 4.6vw, 54px)', lineHeight: '1.18', maxWidth: 'min(100%, 640px)' };
    }

    if (kind === 'sentence') {
        return { fontSize: 'clamp(31px, 3.5vw, 46px)', lineHeight: '1.28', maxWidth: 'min(100%, 800px)' };
    }

    if (kind === 'list') {
        return { fontSize: 'clamp(28px, 3.1vw, 40px)', lineHeight: '1.34', maxWidth: '100%' };
    }

    if (kind === 'file-point' || kind === 'label-point') {
        return { fontSize: 'clamp(31px, 3.5vw, 46px)', lineHeight: '1.28', maxWidth: 'min(100%, 800px)' };
    }

    if (kind === 'code') {
        return { fontSize: 'clamp(18px, 1.9vw, 24px)', lineHeight: '1.52', maxWidth: '100%' };
    }

    return { fontSize: 'clamp(22px, 2.3vw, 30px)', lineHeight: '1.46', maxWidth: '100%' };
}

const COMPLETE_TRACK_ITEM_KEY = 'complete';
const COMPLETE_TRACK_MARKER_KEY = 'complete::marker';
const COMPLETE_TRACK_TITLE = 'Complete';

function normalizeChapterLabel(text) {
    return String(text || '').trim();
}

function buildFallbackChapterKey(text, index) {
    const normalized = normalizeChapterLabel(text)
        .toLowerCase()
        .replace(/[^a-z0-9\s]/g, ' ')
        .replace(/\s+/g, '-')
        .replace(/^-+|-+$/g, '');
    return normalized || `chapter-${index + 1}`;
}

export function buildChapterNavigation(slides) {
    if (!Array.isArray(slides) || slides.length === 0) {
        return [];
    }

    const chapters = [];
    const firstExplicitChapter = slides.find((slide) => normalizeChapterLabel(slide?.chapter?.topLevelTitle));
    let currentTopTitle = normalizeChapterLabel(firstExplicitChapter?.chapter?.topLevelTitle);
    let currentTopKey = normalizeChapterLabel(firstExplicitChapter?.chapter?.topLevelKey);
    let currentChapter = null;
    let currentSubchapter = null;

    slides.forEach((slide, index) => {
        const explicitTopTitle = normalizeChapterLabel(slide?.chapter?.topLevelTitle);
        const explicitTopKey = normalizeChapterLabel(slide?.chapter?.topLevelKey);

        if (explicitTopTitle) {
            currentTopTitle = explicitTopTitle;
            currentTopKey = explicitTopKey || buildFallbackChapterKey(explicitTopTitle, index);
        }

        if (!currentTopTitle) {
            currentTopTitle = normalizeChapterLabel(slide?.title) || `Chapter ${chapters.length + 1}`;
            currentTopKey = buildFallbackChapterKey(currentTopTitle, index);
        }

        if (!currentChapter || currentChapter.key !== currentTopKey) {
            currentChapter = {
                key: currentTopKey,
                title: currentTopTitle,
                startIndex: index,
                endIndex: index,
                slideCount: 0,
                widthPct: 0,
                subchapters: []
            };
            chapters.push(currentChapter);
            currentSubchapter = null;
        }

        currentChapter.endIndex = index;
        currentChapter.slideCount += 1;

        const nestedTitle = normalizeChapterLabel(slide?.chapter?.nestedTitle);
        const nestedKey = normalizeChapterLabel(slide?.chapter?.nestedKey);
        const chapterSlideOrdinal = index - currentChapter.startIndex + 1;

        if (!nestedTitle) {
            currentChapter.subchapters.push({
                key: `${currentTopKey}::slide-${chapterSlideOrdinal}`,
                title: `${currentChapter.title} ${chapterSlideOrdinal}`,
                shortTitle: `${chapterSlideOrdinal}`,
                startIndex: index,
                endIndex: index,
                slideCount: 1,
                offsetPct: 0,
                widthPct: 0,
                isSynthetic: true
            });
            currentSubchapter = null;
            return;
        }

        if (!currentSubchapter || currentSubchapter.key !== nestedKey) {
            currentSubchapter = {
                key: nestedKey || `${currentTopKey}::section-${currentChapter.subchapters.length + 1}`,
                title: nestedTitle,
                shortTitle: nestedTitle,
                startIndex: index,
                endIndex: index,
                slideCount: 0,
                offsetPct: 0,
                widthPct: 0,
                isSynthetic: false
            };
            currentChapter.subchapters.push(currentSubchapter);
        }

        currentSubchapter.endIndex = index;
        currentSubchapter.slideCount += 1;
    });

    chapters.forEach((chapter) => {
        chapter.widthPct = (chapter.slideCount / slides.length) * 100;
        chapter.subchapters.forEach((subchapter) => {
            subchapter.offsetPct = chapter.slideCount > 0
                ? ((subchapter.startIndex - chapter.startIndex) / chapter.slideCount) * 100
                : 0;
            subchapter.widthPct = chapter.slideCount > 0
                ? (subchapter.slideCount / chapter.slideCount) * 100
                : 0;
        });
    });

    return chapters;
}

export function buildProgressTrackItems(chapterNavigation, slideCount) {
    const safeSlideCount = Number.isFinite(slideCount) ? Math.max(0, slideCount) : 0;
    const totalUnitCount = safeSlideCount + 1;
    const trackItems = Array.isArray(chapterNavigation)
        ? chapterNavigation.map((chapter) => ({
            key: chapter.key,
            kind: 'chapter',
            title: chapter.title,
            startIndex: chapter.startIndex,
            endIndex: chapter.endIndex,
            slideCount: chapter.slideCount,
            unitCount: chapter.slideCount,
            centerPct: totalUnitCount > 0
                ? ((chapter.startIndex + (chapter.slideCount / 2)) / totalUnitCount) * 100
                : 0,
            subchapters: chapter.subchapters.map((subchapter) => ({
                ...subchapter,
                tooltipLabel: subchapter.isSynthetic
                    ? subchapter.title
                    : `${chapter.title} -> ${subchapter.title}`,
                globalOffsetPct: totalUnitCount > 0
                    ? ((subchapter.startIndex + 0.5) / totalUnitCount) * 100
                    : 0,
                markerVariant: 'default'
            }))
        }))
        : [];

    trackItems.push({
        key: COMPLETE_TRACK_ITEM_KEY,
        kind: 'complete',
        title: COMPLETE_TRACK_TITLE,
        startIndex: safeSlideCount,
        endIndex: safeSlideCount,
        slideCount: 1,
        unitCount: 1,
        centerPct: totalUnitCount > 0
            ? ((safeSlideCount + 0.5) / totalUnitCount) * 100
            : 100,
        subchapters: [{
            key: COMPLETE_TRACK_MARKER_KEY,
            title: COMPLETE_TRACK_TITLE,
            shortTitle: COMPLETE_TRACK_TITLE,
            startIndex: safeSlideCount,
            endIndex: safeSlideCount,
            slideCount: 1,
            offsetPct: 0,
            widthPct: 100,
            isSynthetic: false,
            tooltipLabel: COMPLETE_TRACK_TITLE,
            globalOffsetPct: totalUnitCount > 0
                ? (safeSlideCount / totalUnitCount) * 100
                : 100,
            markerVariant: 'complete'
        }]
    });

    return trackItems;
}

export function getActiveProgressTrackItemKey(trackItems, currentSlide) {
    if (!Array.isArray(trackItems) || trackItems.length === 0) {
        return '';
    }

    const activeTrackItem = trackItems.find((trackItem) => currentSlide >= trackItem.startIndex && currentSlide <= trackItem.endIndex);
    return activeTrackItem ? activeTrackItem.key : trackItems[0].key;
}

export function getActiveProgressTrackMarkerKey(trackItems, currentSlide) {
    if (!Array.isArray(trackItems)) {
        return '';
    }

    for (const trackItem of trackItems) {
        const activeTrackMarker = trackItem.subchapters.find((subchapter) => currentSlide >= subchapter.startIndex && currentSlide <= subchapter.endIndex);
        if (activeTrackMarker) {
            return activeTrackMarker.key;
        }
    }

    return '';
}

function getProgressTrackFillPercent(trackItem, currentSlide) {
    if (!trackItem || trackItem.unitCount <= 0) {
        return 0;
    }

    if (currentSlide > trackItem.endIndex) {
        return 100;
    }

    if (currentSlide < trackItem.startIndex) {
        return 0;
    }

    const playedUnits = Math.max(0, Math.min(trackItem.unitCount, currentSlide - trackItem.startIndex + 1));
    return Math.max(0, Math.min(100, (playedUnits / trackItem.unitCount) * 100));
}

export function buildChapterExplorerCards(trackItems, currentSlide, activeTrackItemKey = '', activeTrackMarkerKey = '') {
    if (!Array.isArray(trackItems)) {
        return [];
    }

    return trackItems.map((trackItem) => ({
        key: trackItem.key,
        kind: trackItem.kind,
        title: trackItem.title,
        slideCount: trackItem.slideCount,
        startIndex: trackItem.startIndex,
        progressPercent: getProgressTrackFillPercent(trackItem, currentSlide),
        isActive: trackItem.key === activeTrackItemKey,
        subchapters: trackItem.kind === 'complete'
            ? []
            : trackItem.subchapters.map((subchapter) => ({
                key: subchapter.key,
                title: subchapter.title,
                tooltipLabel: subchapter.tooltipLabel,
                startIndex: subchapter.startIndex,
                slideCount: subchapter.slideCount,
                isSynthetic: subchapter.isSynthetic,
                isActive: subchapter.key === activeTrackMarkerKey
            }))
    }));
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
        const [isChapterExplorerOpen, setIsChapterExplorerOpen] = useState(false);
        const [highlightedTrackItemKey, setHighlightedTrackItemKey] = useState('');
        const [autoPlayRemainingMs, setAutoPlayRemainingMs] = useState(0);
        const bodyRef = useRef(null);
        const dialogRef = useRef(null);
        const lastFocusedElementRef = useRef(null);
        const autoPlayTimerRef = useRef(null);
        const autoPlayTickRef = useRef(null);
        const copyTimerRef = useRef(null);
        const chapterExplorerOpenTimerRef = useRef(null);
        const chapterExplorerCloseTimerRef = useRef(null);
        const chapterExplorerResetTimerRef = useRef(null);
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

        const clearChapterExplorerTimers = () => {
            if (chapterExplorerOpenTimerRef.current) {
                clearTimeout(chapterExplorerOpenTimerRef.current);
                chapterExplorerOpenTimerRef.current = null;
            }
            if (chapterExplorerCloseTimerRef.current) {
                clearTimeout(chapterExplorerCloseTimerRef.current);
                chapterExplorerCloseTimerRef.current = null;
            }
            if (chapterExplorerResetTimerRef.current) {
                clearTimeout(chapterExplorerResetTimerRef.current);
                chapterExplorerResetTimerRef.current = null;
            }
        };

        const openChapterExplorer = (trackItemKey = '', immediate = false) => {
            clearChapterExplorerTimers();
            if (trackItemKey) {
                setHighlightedTrackItemKey(trackItemKey);
            }

            if (immediate || isChapterExplorerOpen) {
                setIsChapterExplorerOpen(true);
                return;
            }

            chapterExplorerOpenTimerRef.current = setTimeout(() => {
                if (trackItemKey) {
                    setHighlightedTrackItemKey(trackItemKey);
                }
                setIsChapterExplorerOpen(true);
                chapterExplorerOpenTimerRef.current = null;
            }, 90);
        };

        const closeChapterExplorer = () => {
            clearChapterExplorerTimers();
            setIsChapterExplorerOpen(false);
            setHighlightedTrackItemKey('');
        };

        const closeChapterExplorerSoon = () => {
            clearChapterExplorerTimers();
            chapterExplorerCloseTimerRef.current = setTimeout(() => {
                setIsChapterExplorerOpen(false);
                chapterExplorerResetTimerRef.current = setTimeout(() => {
                    setHighlightedTrackItemKey('');
                    chapterExplorerResetTimerRef.current = null;
                }, 160);
                chapterExplorerCloseTimerRef.current = null;
            }, 200);
        };

        const isMovingWithinContainer = (event) => {
            const nextTarget = event?.relatedTarget;
            const currentTarget = event?.currentTarget;
            return Boolean(nextTarget && currentTarget && typeof currentTarget.contains === 'function' && currentTarget.contains(nextTarget));
        };

        const handleChapterExplorerRegionMouseLeave = (event) => {
            if (isMovingWithinContainer(event)) {
                return;
            }
            closeChapterExplorerSoon();
        };

        const handleChapterExplorerRegionBlur = (event) => {
            if (isMovingWithinContainer(event)) {
                return;
            }
            closeChapterExplorerSoon();
        };

        useEffect(() => {
            if (!isVisible || !markdown) {
                return;
            }

            const parsedSlides = parseMarkdownToSlides(markdown).map(slide => ({
                ...slide,
                renderedContent: getSafeRenderedHtml(slide.content)
            }));
            const startingIndex = clampSlideIndex(initialSlideIndex, parsedSlides.length);
            setSlides(parsedSlides);
            setCurrentSlide(startingIndex);
            setIsAutoPlay(false);
            setIsHelpShown(false);
            setCopied(false);
            setLiveMessage('');
            setIsChapterExplorerOpen(false);
            setHighlightedTrackItemKey('');
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

                if (isChapterExplorerOpen && key === 'escape') {
                    closeChapterExplorer();
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
        }, [isVisible, isModal, isShortcutActive, slides.length, currentSlide, isHelpShown, isAutoPlay, isChapterExplorerOpen]);

        useEffect(() => {
            if (!isVisible || !bodyRef.current) {
                return;
            }

            bodyRef.current.scrollTop = 0;
        }, [currentSlide, isVisible, slides.length]);

        useEffect(() => () => {
            clearAutoPlayTimers();
            if (copyTimerRef.current) {
                clearTimeout(copyTimerRef.current);
            }
            clearChapterExplorerTimers();
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

        const totalSlidesWithComplete = slides.length + 1;
        const isCompleteSlide = currentSlide >= slides.length;
        const slide = !isCompleteSlide ? slides[currentSlide] : null;
        const progressValue = totalSlidesWithComplete
            ? ((Math.min(currentSlide, totalSlidesWithComplete - 1) + 1) / totalSlidesWithComplete) * 100
            : 0;
        const chapterNavigation = buildChapterNavigation(slides);
        const progressTrackItems = buildProgressTrackItems(chapterNavigation, slides.length);
        const activeProgressTrackItemKey = getActiveProgressTrackItemKey(progressTrackItems, currentSlide);
        const activeProgressTrackMarkerKey = getActiveProgressTrackMarkerKey(progressTrackItems, currentSlide);
        const chapterExplorerCards = buildChapterExplorerCards(progressTrackItems, currentSlide, activeProgressTrackItemKey, activeProgressTrackMarkerKey);

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

        const jumpToSlide = (nextIndex, label = '') => {
            moveToSlide(nextIndex);
            if (label) {
                setLiveMessage(`Jumped to ${label}.`);
            }
        };

        const nextSlide = () => {
            if (currentSlide >= slides.length - 1) {
                moveToSlide(slides.length);
                return;
            }
            moveToSlide(currentSlide + 1);
        };

        const prevSlide = () => {
            if (isCompleteSlide) {
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

        const isIntro = !isCompleteSlide && slide?.kind === 'intro';
        const panelBg = isCompleteSlide ? '#1f2430' : (slide ? slide.color.surface : '#1f2430');
        const typography = slide ? resolveSlideTypography(slide) : null;
        const panelHeight = isModal ? 'clamp(540px, 78vh, 760px)' : 'clamp(440px, 62vh, 620px)';
        const progressAccent = slide ? slide.color.accent : (slides[slides.length - 1]?.color?.accent || '#3b82f6');
        const explorerFocusTrackItemKey = highlightedTrackItemKey || activeProgressTrackItemKey;

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
                    width: ${isModal ? 'min(1040px, calc(100vw - 48px))' : '100%'};
                    max-width: ${isModal ? 'calc(100vw - 48px)' : '100%'};
                    height: ${panelHeight};
                    max-height: ${isModal ? 'calc(100vh - 48px)' : '620px'};
                    aspect-ratio: ${isModal ? '16 / 10' : 'auto'};
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

                <div class="summary-slideshow-stage">
                    <button
                        class="summary-slideshow-nav-overlay summary-slideshow-nav-overlay-prev"
                        onClick=${prevSlide}
                        title="Previous slide (H / K / Left Arrow)"
                        aria-label="Previous slide"
                        disabled=${currentSlide === 0 && !isCompleteSlide}
                        tabIndex="-1"
                    >
                        <svg width="18" height="18" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
                        </svg>
                    </button>

                <div ref=${bodyRef} class="summary-slideshow-body" style="flex: 1; min-height: 0; overflow-y: auto; display: flex; flex-direction: column; justify-content: center; ${isIntro || isCompleteSlide ? 'align-items: center;' : ''} padding: 28px 32px;">
                    ${isCompleteSlide ? html`
                        <div class="summary-slideshow-complete" style="text-align: center; padding: 32px; max-width: 520px; width: 100%;">
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
                    ` : isIntro
                        ? html`
                            <div class="summary-slideshow-intro" style="text-align: center; max-width: min(820px, 100%); width: 100%;">
                                <h1 style="margin: 0; font-size: ${typography.fontSize}; line-height: ${typography.lineHeight}; color: ${slide.color.title}; font-weight: 760; letter-spacing: -0.034em; text-wrap: balance;">
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
                                                ${truncatePathDisplay(slide.meta.filePath)}${slide.meta.line ? `:${slide.meta.line}` : ''}
                                            </button>
                                        `
                                        : html`
                                            <code class="summary-file-inline-code" title="${slide.meta.filePath}${slide.meta.line ? `:${slide.meta.line}` : ''}">${truncatePathDisplay(slide.meta.filePath)}${slide.meta.line ? `:${slide.meta.line}` : ''}</code>
                                        `
                                    }
                                    <div
                                        class="summary-file-description summary-slideshow-content"
                                        style="font-size: ${typography.fontSize}; line-height: ${typography.lineHeight}; max-width: ${typography.maxWidth};"
                                        dangerouslySetInnerHTML=${{ __html: slide.renderedContent || '' }}
                                    ></div>
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
                                        <div
                                            class="summary-label-body summary-slideshow-content"
                                            style="font-size: ${typography.fontSize}; line-height: ${typography.lineHeight}; max-width: ${typography.maxWidth};"
                                            dangerouslySetInnerHTML=${{ __html: slide.renderedContent || '' }}
                                        ></div>
                                    </div>
                                `
                                : html`
                                    ${slide.title && html`
                                        <div style="margin-bottom: 16px; font-size: 14px; font-weight: 700; letter-spacing: 0.01em; color: ${slide.color.accent};">
                                            ${slide.title}
                                        </div>
                                    `}
                                    <div
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
                                        dangerouslySetInnerHTML=${{ __html: slide.renderedContent || '' }}
                                    ></div>
                                `}
                </div>
                    <button
                        class="summary-slideshow-nav-overlay summary-slideshow-nav-overlay-next"
                        onClick=${nextSlide}
                        title="Next slide (J / L / Right Arrow / Space)"
                        aria-label="Next slide"
                        disabled=${isCompleteSlide}
                        tabIndex="-1"
                    >
                        <svg width="18" height="18" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                        </svg>
                    </button>
                </div>

                <div class="summary-slideshow-controls" style="padding: 10px 16px 12px 16px; flex-shrink: 0;">
                    <div style="display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 8px;">
                        <div style="display: flex; align-items: center; gap: 6px;">
                            <button class="action-btn summary-slide-btn" onClick=${prevSlide} title="Previous slide (H / K / Left Arrow)" aria-label="Previous slide" disabled=${currentSlide === 0 && !isCompleteSlide}>
                                <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
                                </svg>
                                Prev
                            </button>
                            <button class="action-btn summary-slide-btn" onClick=${nextSlide} title="Next slide (J / L / Right Arrow / Space)" aria-label="Next slide" disabled=${isCompleteSlide}>
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
                            ${isCompleteSlide ? `${totalSlidesWithComplete}/${totalSlidesWithComplete} \u00b7 complete` : `${currentSlide + 1}/${totalSlidesWithComplete} \u00b7 ${formatRemainingTime(slides, currentSlide)} left`}
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

                    <div class="summary-chapter-progress-wrap">
                        <div
                            class="summary-chapter-progress-zone"
                            onMouseEnter=${() => openChapterExplorer(explorerFocusTrackItemKey)}
                            onMouseLeave=${handleChapterExplorerRegionMouseLeave}
                            onFocusCapture=${() => openChapterExplorer(explorerFocusTrackItemKey, true)}
                            onBlurCapture=${handleChapterExplorerRegionBlur}
                            style=${`--summary-chapter-accent: ${progressAccent};`}
                        >
                            <div class="summary-chapter-progress-shell">
                        <div class="summary-chapter-progress" role="group" aria-label="Slideshow chapter navigation">
                            ${progressTrackItems.map((trackItem) => {
                                const trackItemFillPercent = getProgressTrackFillPercent(trackItem, currentSlide);
                                const isActiveTrackItem = trackItem.key === activeProgressTrackItemKey;
                                const trackItemLabel = `${trackItem.title} · ${trackItem.slideCount} ${trackItem.slideCount === 1 ? 'slide' : 'slides'}`;

                                return html`
                                    <div
                                        class=${`summary-chapter-segment-wrap ${trackItem.kind === 'complete' ? 'is-complete-item' : ''}`}
                                        style=${`flex: ${trackItem.unitCount} 1 0;`}
                                    >
                                        <button
                                            type="button"
                                            class=${`summary-chapter-segment ${trackItem.kind === 'complete' ? 'is-complete-item' : ''} ${isActiveTrackItem ? 'is-active' : ''}`}
                                            title=${trackItemLabel}
                                            aria-label=${`Jump to ${trackItemLabel}`}
                                            aria-current=${isActiveTrackItem ? 'step' : null}
                                            onClick=${() => jumpToSlide(trackItem.startIndex, trackItem.title)}
                                            onMouseEnter=${() => openChapterExplorer(trackItem.key, true)}
                                            onFocus=${() => openChapterExplorer(trackItem.key, true)}
                                        >
                                            <span class="summary-chapter-segment-fill" style=${`width: ${trackItemFillPercent}%; background: ${progressAccent};`}></span>
                                        </button>

                                        ${trackItem.subchapters.map((trackMarker) => {
                                            const isActiveTrackMarker = activeProgressTrackMarkerKey === trackMarker.key;
                                            const markerStyle = `left: ${trackMarker.offsetPct}%; width: max(14px, ${trackMarker.widthPct}%);`;
                                            return html`
                                                <button
                                                    type="button"
                                                    class=${`summary-chapter-subsection-hit ${isActiveTrackMarker ? 'is-active' : ''} ${trackMarker.isSynthetic ? 'is-synthetic' : ''} ${trackMarker.markerVariant === 'complete' ? 'is-complete-marker' : ''}`}
                                                    style=${markerStyle}
                                                    title=${`${trackMarker.tooltipLabel} · ${trackMarker.slideCount} ${trackMarker.slideCount === 1 ? 'slide' : 'slides'}`}
                                                    aria-label=${`Jump to ${trackMarker.tooltipLabel}`}
                                                    onClick=${() => jumpToSlide(trackMarker.startIndex, trackMarker.tooltipLabel)}
                                                    onMouseEnter=${() => openChapterExplorer(trackItem.key, true)}
                                                    onFocus=${() => openChapterExplorer(trackItem.key, true)}
                                                >
                                                    <span class=${`summary-chapter-subsection-marker ${trackMarker.markerVariant === 'complete' ? 'is-complete-marker' : ''}`}></span>
                                                </button>
                                            `;
                                        })}
                                    </div>
                                `;
                            })}
                        </div>
                            </div>
                            <div class="summary-chapter-progress-readout" aria-hidden="true">${Math.round(progressValue)}%</div>
                        <div class=${`summary-chapter-explorer ${isChapterExplorerOpen ? 'is-open' : ''}`} aria-hidden=${isChapterExplorerOpen ? 'false' : 'true'}>
                            <div class="summary-chapter-explorer-grid">
                                ${chapterExplorerCards.map((card) => {
                                    const isEmphasizedCard = card.key === explorerFocusTrackItemKey;
                                    const shouldShowSubchapters = card.subchapters.length > 0 && (isEmphasizedCard || card.isActive);
                                    const cardCaption = card.kind === 'complete'
                                        ? 'Final slide'
                                        : `Starts at slide ${card.startIndex + 1}`;

                                    return html`
                                        <div
                                            class=${`summary-chapter-explorer-card ${card.isActive ? 'is-active' : ''} ${isEmphasizedCard ? 'is-emphasized' : ''}`}
                                            onMouseEnter=${() => openChapterExplorer(card.key, true)}
                                        >
                                            <button
                                                type="button"
                                                class="summary-chapter-explorer-card-main"
                                                onClick=${() => jumpToSlide(card.startIndex, card.title)}
                                                onFocus=${() => openChapterExplorer(card.key, true)}
                                                title=${`Jump to ${card.title}`}
                                                aria-label=${`Jump to ${card.title}`}
                                                tabIndex=${isChapterExplorerOpen ? 0 : -1}
                                            >
                                                <div class="summary-chapter-explorer-card-top">
                                                    <div class="summary-chapter-explorer-card-title">${card.title}</div>
                                                    <div class="summary-chapter-explorer-card-count">${card.slideCount} ${card.slideCount === 1 ? 'slide' : 'slides'}</div>
                                                </div>
                                                <div class="summary-chapter-explorer-card-progress">
                                                    <span class="summary-chapter-explorer-card-progress-fill" style=${`width: ${card.progressPercent}%; background: ${progressAccent};`}></span>
                                                </div>
                                                <div class="summary-chapter-explorer-card-caption">${cardCaption}</div>
                                            </button>

                                            ${shouldShowSubchapters && html`
                                                <div class="summary-chapter-explorer-subchapters">
                                                    ${card.subchapters.map((subchapter) => html`
                                                        <button
                                                            type="button"
                                                            class=${`summary-chapter-explorer-subchapter ${subchapter.isActive ? 'is-active' : ''}`}
                                                            onClick=${() => jumpToSlide(subchapter.startIndex, subchapter.tooltipLabel)}
                                                            onFocus=${() => openChapterExplorer(card.key, true)}
                                                            title=${`Jump to ${subchapter.tooltipLabel}`}
                                                            aria-label=${`Jump to ${subchapter.tooltipLabel}`}
                                                            tabIndex=${isChapterExplorerOpen ? 0 : -1}
                                                        >
                                                            <span class="summary-chapter-explorer-subchapter-title">${subchapter.title}</span>
                                                            <span class="summary-chapter-explorer-subchapter-count">${subchapter.slideCount}</span>
                                                        </button>
                                                    `)}
                                                </div>
                                            `}
                                        </div>
                                    `;
                                })}
                            </div>
                        </div>
                        <div role="status" aria-live="polite" class="summary-slideshow-status">
                            ${liveMessage || (isAutoPlay && !isCompleteSlide ? `Auto-play \u00b7 next in ${Math.max(1, Math.ceil(autoPlayRemainingMs / 1000))}s` : '')}
                        </div>
                        </div>
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
