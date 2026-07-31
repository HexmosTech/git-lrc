const ICON_SPECS = Object.freeze({
    folder: {
        paths: [
            { d: 'M3 7v10a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-6l-2-2H5a2 2 0 0 0-2 2z' },
        ],
    },
    listTree: {
        paths: [
            { d: 'M9 6h11' },
            { d: 'M9 12h11' },
            { d: 'M9 18h11' },
            { d: 'M4 6h.01' },
            { d: 'M4 12h.01' },
            { d: 'M4 18h.01' },
        ],
    },
    expandAll: {
        paths: [
            { d: 'M4 8V4' },
            { d: 'M4 4h4' },
            { d: 'M4 4l5 5' },
            { d: 'M20 8V4' },
            { d: 'M20 4h-4' },
            { d: 'M20 4l-5 5' },
            { d: 'M4 16v4' },
            { d: 'M4 20h4' },
            { d: 'M4 20l5-5' },
            { d: 'M20 20l-5-5' },
            { d: 'M20 20v-4' },
            { d: 'M20 20h-4' },
        ],
    },
    collapseAll: {
        paths: [
            { d: 'M20 12H4' },
        ],
    },
    arrowDown: {
        paths: [
            { d: 'M19 14l-7 7' },
            { d: 'M12 21l-7-7' },
            { d: 'M12 21V3' },
        ],
    },
    copy: {
        paths: [
            { d: 'M8 16H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v2' },
            { d: 'M10 10h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-8a2 2 0 0 1-2-2v-8a2 2 0 0 1 2-2z' },
        ],
    },
    check: {
        paths: [
            { d: 'M5 13l4 4L19 7' },
        ],
    },
    send: {
        paths: [
            { d: 'M22 2L11 13' },
            { d: 'M22 2L15 22l-4-9-9-4 20-7z' },
        ],
    },
    spark: {
        paths: [
            { d: 'M12 3l1.9 4.8L19 10l-5.1 2.2L12 17l-1.9-4.8L5 10l5.1-2.2L12 3z' },
            { d: 'M19 4v4' },
            { d: 'M21 6h-4' },
        ],
    },
    warning: {
        paths: [
            { d: 'M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z' },
            { d: 'M12 9v4' },
            { d: 'M12 17h.01' },
        ],
    },
    plus: {
        paths: [
            { d: 'M12 5v14' },
            { d: 'M5 12h14' },
        ],
    },
    x: {
        paths: [
            { d: 'M6 6l12 12' },
            { d: 'M18 6L6 18' },
        ],
    },
    upload: {
        paths: [
            { d: 'M12 16V4' },
            { d: 'M7 9l5-5 5 5' },
            { d: 'M4 20h16' },
        ],
    },
    shieldCheck: {
        paths: [
            { d: 'M12 3l7 4v5c0 5-3.5 8.7-7 10-3.5-1.3-7-5-7-10V7l7-4z' },
            { d: 'M9.5 12.5l1.8 1.8L15 10.6' },
        ],
    },
    refresh: {
        paths: [
            { d: 'M21 12a9 9 0 1 1-2.64-6.36' },
            { d: 'M21 3v6h-6' },
        ],
    },
    reorder: {
        paths: [
            { d: 'M8 6h8' },
            { d: 'M8 12h8' },
            { d: 'M8 18h8' },
            { d: 'M5 4v16' },
            { d: 'M3 6l2-2 2 2' },
            { d: 'M3 18l2 2 2-2' },
        ],
    },
    grip: {
        paths: [
            { d: 'M9 6h.01' },
            { d: 'M15 6h.01' },
            { d: 'M9 12h.01' },
            { d: 'M15 12h.01' },
            { d: 'M9 18h.01' },
            { d: 'M15 18h.01' },
        ],
    },
    chevronUp: {
        paths: [
            { d: 'M18 15l-6-6-6 6' },
        ],
    },
    chevronDown: {
        paths: [
            { d: 'M6 9l6 6 6-6' },
        ],
    },
    chevronLeft: {
        paths: [
            { d: 'M15 18l-6-6 6-6' },
        ],
    },
    chevronRight: {
        paths: [
            { d: 'M9 18l6-6-6-6' },
        ],
    },
    pencil: {
        paths: [
            { d: 'M12 20h9' },
            { d: 'M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4 12.5-12.5z' },
        ],
    },
    trash: {
        paths: [
            { d: 'M3 6h18' },
            { d: 'M8 6V4h8v2' },
            { d: 'M19 6l-1 14H6L5 6' },
            { d: 'M10 11v6' },
            { d: 'M14 11v6' },
        ],
    },
    save: {
        paths: [
            { d: 'M5 4h11l3 3v13H5z' },
            { d: 'M8 4v5h6V4' },
            { d: 'M8 20v-6h8v6' },
        ],
    },
    login: {
        paths: [
            { d: 'M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4' },
            { d: 'M10 17l5-5-5-5' },
            { d: 'M15 12H3' },
        ],
    },
    text: {
        paths: [
            { d: 'M4 7h16' },
            { d: 'M8 7v10' },
            { d: 'M16 7v10' },
        ],
    },
    slides: {
        paths: [
            { d: 'M4 4h10v10H4z' },
            { d: 'M10 10h10v10H10z' },
        ],
    },
    checkCircle: {
        paths: [
            { d: 'M22 11.08V12a10 10 0 1 1-5.93-9.14' },
            { d: 'M22 4L12 14.01l-3-3' },
        ],
    },
    xCircle: {
        paths: [
            { d: 'M12 22a10 10 0 1 0-10-10 10 10 0 0 0 10 10z' },
            { d: 'M15 9l-6 6' },
            { d: 'M9 9l6 6' },
        ],
    },
    terminal: {
        elements: [
            { tag: 'rect', attrs: { x: '2', y: '3', width: '20', height: '18', rx: '2', ry: '2' } },
            { tag: 'polyline', attrs: { points: '6 8 10 12 6 16' } },
            { tag: 'line', attrs: { x1: '14', y1: '16', x2: '18', y2: '16' } },
        ],
    },
    message: {
        paths: [
            { d: 'M21 11.5a8.5 8.5 0 0 1-8.5 8.5 8.36 8.36 0 0 1-4.1-1.06L3 20l1.06-5.4A8.36 8.36 0 0 1 3 11.5 8.5 8.5 0 0 1 11.5 3h1A8.5 8.5 0 0 1 21 11.5z' },
            { d: 'M8 12h.01' },
            { d: 'M12 12h.01' },
            { d: 'M16 12h.01' },
        ],
    },
    thumbsUp: {
        paths: [
            { d: 'M14 9V5a3 3 0 0 0-3-3l-4 9v11h11.28a2 2 0 0 0 2-1.7l1.38-9a2 2 0 0 0-2-2.3H14z' },
            { d: 'M7 22H4a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2h3' },
        ],
    },
    thumbsDown: {
        paths: [
            { d: 'M10 15v4a3 3 0 0 0 3 3l4-9V2H5.72a2 2 0 0 0-2 1.7l-1.38 9a2 2 0 0 0 2 2.3H10z' },
            { d: 'M17 2h2.67A2.31 2.31 0 0 1 22 4v7a2.31 2.31 0 0 1-2.33 2H17' },
        ],
    },
    eye: {
        elements: [
            { tag: 'path', attrs: { d: 'M1 12s4-7 11-7 11 7 11 7-4 7-11 7S1 12 1 12z' } },
            { tag: 'circle', attrs: { cx: '12', cy: '12', r: '3' } },
        ],
    },
    eyeOff: {
        elements: [
            { tag: 'path', attrs: { d: 'M10.58 10.58a2 2 0 0 0 2.84 2.84' } },
            { tag: 'path', attrs: { d: 'M9.36 5.37A10.94 10.94 0 0 1 12 5c7 0 10 7 10 7a13.03 13.03 0 0 1-3.08 4.25' } },
            { tag: 'path', attrs: { d: 'M6.61 6.61C3.61 8.13 2 12 2 12s3 7 10 7a9.76 9.76 0 0 0 4.39-1.02' } },
            { tag: 'line', attrs: { x1: '2', y1: '2', x2: '22', y2: '22' } },
        ],
    },
    externalLink: {
        paths: [
            { d: 'M10 6H6a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-4' },
            { d: 'M14 4h6v6' },
            { d: 'M10 14L20 4' },
        ],
    },
    play: {
        paths: [
            { d: 'M8 6.82v10.36a1 1 0 0 0 1.53.848l8.25-5.18a1 1 0 0 0 0-1.696L9.53 5.972A1 1 0 0 0 8 6.82z' },
        ],
    },
    pause: {
        paths: [
            { d: 'M10 9v6' },
            { d: 'M14 9v6' },
        ],
    },
    help: {
        elements: [
            { tag: 'circle', attrs: { cx: '12', cy: '12', r: '9' } },
            { tag: 'path', attrs: { d: 'M8.228 9c.549-1.165 1.918-2 3.522-2 2.071 0 3.75 1.343 3.75 3 0 1.268-.983 2.352-2.37 2.79-.488.154-.88.56-.88 1.012V15' } },
            { tag: 'path', attrs: { d: 'M12 18h.01' } },
        ],
    },
    info: {
        elements: [
            { tag: 'circle', attrs: { cx: '12', cy: '12', r: '9' } },
            { tag: 'path', attrs: { d: 'M12 16v-4' } },
            { tag: 'path', attrs: { d: 'M12 8h.01' } },
        ],
    },
    brandClaude: {
        type: 'brand-monogram',
        letter: 'C',
    },
    filter: {
        paths: [
            { d: 'M22 3H2l8 9.46V19l4 2v-8.54L22 3z' },
        ],
    },
});

export const ICON_ALIASES = Object.freeze({
    filesTab: 'folder',
    eventsTab: 'listTree',
    expandFiles: 'expandAll',
    collapseFiles: 'collapseAll',
    tailLog: 'arrowDown',
    copyLogs: 'copy',
    copied: 'check',
    sendToAgent: 'send',
    claudeAgentAction: 'send',
    claudeBrand: 'brandClaude',
    issueWarning: 'warning',
    aiAssist: 'spark',
    commit: 'check',
    commitPush: 'upload',
    skip: 'x',
    vouch: 'shieldCheck',
    abort: 'x',
    refresh: 'refresh',
    reorder: 'reorder',
    add: 'plus',
    drag: 'grip',
    moveUp: 'chevronUp',
    moveDown: 'chevronDown',
    previous: 'chevronLeft',
    next: 'chevronRight',
    edit: 'pencil',
    delete: 'trash',
    save: 'save',
    cancel: 'x',
    signIn: 'login',
    reauthenticate: 'login',
    slidesView: 'slides',
    textView: 'text',
    openSlides: 'slides',
    successStatus: 'checkCircle',
    errorStatus: 'xCircle',
    handoffSuccess: 'terminal',
    handoffNotice: 'warning',
    feedback: 'message',
    helpful: 'thumbsUp',
    notHelpful: 'thumbsDown',
    showComment: 'eye',
    hideComment: 'eyeOff',
    close: 'x',
    dropdownOpen: 'chevronUp',
    dropdownClosed: 'chevronDown',
    external: 'externalLink',
    play: 'play',
    pause: 'pause',
    help: 'help',
});

export const ICON_SELECTION_GUIDANCE = Object.freeze({
    semanticFirst: 'Prefer the user action over the vendor named in the label.',
    brandForIdentity: 'Only use brand icons when the surface is representing vendor identity.',
    noForcedLogos: 'If no approved brand icon exists, keep a semantic icon plus text instead of inventing one.',
});

function resolveIconSpec(name) {
    const iconName = ICON_ALIASES[name] || name;
    return ICON_SPECS[iconName] || null;
}

function renderSvgIcon(html, spec, attributes, titleText) {
    return html`
        <span class=${attributes.className} aria-hidden=${attributes.ariaHidden}>
            <svg
                width=${attributes.pixelSize}
                height=${attributes.pixelSize}
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width=${attributes.strokeWidth}
                stroke-linecap="round"
                stroke-linejoin="round"
                role=${attributes.role}
                aria-label=${attributes.ariaLabel}
                focusable="false"
            >
                ${titleText ? html`<title>${titleText}</title>` : null}
                ${spec.elements
                    ? spec.elements.map((element) => renderSvgElement(html, element))
                    : spec.paths.map((path) => html`<path d=${path.d}></path>`)}
            </svg>
        </span>
    `;
}

function renderSvgElement(html, element) {
    const attrs = element.attrs || {};

    if (element.tag === 'rect') {
        return html`<rect x=${attrs.x} y=${attrs.y} width=${attrs.width} height=${attrs.height} rx=${attrs.rx} ry=${attrs.ry}></rect>`;
    }
    if (element.tag === 'polyline') {
        return html`<polyline points=${attrs.points}></polyline>`;
    }
    if (element.tag === 'line') {
        return html`<line x1=${attrs.x1} y1=${attrs.y1} x2=${attrs.x2} y2=${attrs.y2}></line>`;
    }
    if (element.tag === 'circle') {
        return html`<circle cx=${attrs.cx} cy=${attrs.cy} r=${attrs.r}></circle>`;
    }

    return html`<path d=${attrs.d}></path>`;
}

function renderBrandMonogram(html, spec, attributes, titleText) {
    return html`
        <span class=${`${attributes.className} icon-brand-monogram`} aria-hidden=${attributes.ariaHidden}>
            <span
                class="icon-brand-monogram-glyph"
                role=${attributes.role}
                aria-label=${attributes.ariaLabel}
                title=${titleText}
            >
                ${spec.letter}
            </span>
        </span>
    `;
}

export function renderIcon(html, name, options = {}) {
    const spec = resolveIconSpec(name);
    if (!spec) {
        throw new Error(`Unknown icon: ${name}`);
    }

    const decorative = options.decorative !== false;
    const pixelSize = Number.isFinite(options.size) ? options.size : 14;
    const className = ['ui-icon', options.className].filter(Boolean).join(' ');
    const titleText = options.title || '';
    const accessibleLabel = decorative ? undefined : (options.label || titleText || name);
    const attributes = {
        ariaHidden: decorative ? 'true' : undefined,
        ariaLabel: accessibleLabel,
        className,
        pixelSize,
        role: decorative ? 'presentation' : 'img',
        strokeWidth: options.strokeWidth || 1.8,
    };

    if (spec.type === 'brand-monogram') {
        return renderBrandMonogram(html, spec, attributes, titleText || accessibleLabel || spec.letter);
    }

    return renderSvgIcon(html, spec, attributes, titleText);
}

export function hasIcon(name) {
    return Boolean(resolveIconSpec(name));
}
