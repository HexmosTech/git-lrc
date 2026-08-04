// BlastRadiusPanel - the full, drillable "why this score" breakdown for a
// hunk, fed by the /api/blastradius report joined onto the hunk as
// BlastDetail. Principle: NOTHING from the report is discarded - it is
// presented prioritized and collapsed, with every layer expandable:
//
//   scores + impacted-packages summary
//   └─ hunk-level signals (always visible, ranked)
//   └─ two-column layout: text details (left) + sunburst call graph (right)
//        └─ symbol navigation slider ◀ N of M ▶
//        └─ selected symbol: signals, metrics, caller groups, packages
import { waitForPreact } from './utils.js';
import { renderIcon } from './icons.js';
import { blastRadiusTier, allSignals } from './blast_radius_sort_state.mjs';
import { getSunburstChart } from './SunburstChart.js';
import { getFlameGraph } from './FlameGraph.js';

const CALLERS_PREVIEW = 8;
const PKGS_PREVIEW = 10;
const HIDE_DELAY_MS = 180;
const SUNBURST_SIZE = 420;
// Popover width/height estimates used only to clamp position:fixed coords to
// the viewport (see ScoreChipWithHelp/MethodologyButton) - same technique as
// RiskBadge.js's HoverCard, which these popovers previously could not use
// since they lived inside .file/.diff-table td (both overflow:hidden), and
// position:absolute can't escape an ancestor's overflow clip regardless of
// z-index. position:fixed sidesteps that entirely.
const HELP_POPUP_WIDTH = 260;
const HELP_POPUP_EST_HEIGHT = 110;
const METHODOLOGY_POPUP_WIDTH = 420;
const METHODOLOGY_POPUP_EST_HEIGHT = 360;

const SCORE_HINTS = {
    combined: {
        title: 'Combined score: 0-100 rank within this diff',
        body: 'Blends Blast Radius and Review Priority (default 60/40). Use this number to sort. Read Blast and Priority to see why.',
    },
    blast: {
        title: 'Blast radius: how far this change can reach',
        body: 'Counts callers up to 3 hops. Adds points for HTTP routes, repository hotspots, architectural layers, and widely used interfaces. Adds points when the file touches auth, persistence, config, build, or schema.',
    },
    priority: {
        title: 'Review priority: how much attention this hunk needs',
        body: 'Goes up when a near-duplicate function exists in another file, or when this symbol has no direct tests. Adds small points for cyclomatic complexity, loop depth, and fan-out.',
    },
    hygiene: {
        title: 'Hygiene dampener: this hunk looks small or low-value',
        body: 'Detected from the diff content and file path. Multiplies the Combined score down. A small change to a key function must still rank low.',
    },
    coupling: {
        title: 'File co-change coupling bonus',
        body: 'Files that changed together in git history get a small bonus. Captures hidden coupling when no code reference connects them.',
    },
};

const METHODOLOGY_PARAGRAPHS = [
    'Each hunk gets two scores. Both range from 0 to 100 within this diff. 100 marks the highest-risk hunk in this diff, not a universal scale. Combined blends them at 60 percent Blast Radius and 40 percent Review Priority. Sort by Combined. Read Blast and Priority to see why.',
    'Blast radius measures how far the change can reach. Inputs: callers up to 3 hops, HTTP routes, repository hotspots, architectural layers, interface implementations, cross-package callers, and file paths near auth, persistence, config, build, or schema.',
    'Review priority measures how much attention this hunk needs. Main inputs: a near-duplicate function in another file, and missing direct tests. Secondary inputs: cyclomatic complexity, loop depth, and fan-out. Code complexity alone does not predict customer impact.',
    'File co-change coupling adds a small bonus to Blast Radius when git history shows files that change together. Hygiene signals (formatting, comments, generated code, logging, test-only files, dead code) multiply the Combined score down. A small change to a key function must still rank low.',
];

function shortName(qualifiedName) {
    const parts = (qualifiedName || '').split('.');
    return parts[parts.length - 1] || qualifiedName;
}

function sortedSignals(signals) {
    return [...(signals || [])].sort((a, b) => Math.abs(b.Points || 0) - Math.abs(a.Points || 0));
}

// Convention: a Signal object carries `_symbolName` (the symbol it came
// from) only when it was sourced from a SymbolContribution's own Signals -
// hunk-level signals (file coupling, arch role) never get one. This field
// is added client-side by allSignals() in blast_radius_sort_state.mjs (the
// one place that flattens hunk + symbol signals together) and is not part
// of the server's Signal JSON shape - every consumer here relies on that
// same convention rather than re-deriving it.
//
// A hunk touching several symbols can produce the exact same signal name
// several times over (e.g. "Caller reach" once per touched function) - with
// no grouping, that reads as confusing duplication rather than what it is.
// distinctSymbolCount decides whether a signal list needs per-symbol
// sections at all (a single-symbol list has nothing to disambiguate).
function distinctSymbolCount(signals) {
    return new Set((signals || []).map((s) => s._symbolName).filter(Boolean)).size;
}

// groupSignalsBySymbol splits a signal list into hunk-level signals (no
// _symbolName - e.g. file coupling) and per-symbol groups, the groups
// ordered by that symbol's total contribution (sum of |Points|) to this
// particular list, descending - so the symbol driving the most of this
// dimension's score gets its section shown first.
function groupSignalsBySymbol(signals) {
    const ungrouped = [];
    const bySymbol = new Map();
    (signals || []).forEach((s) => {
        if (!s._symbolName) {
            ungrouped.push(s);
            return;
        }
        if (!bySymbol.has(s._symbolName)) bySymbol.set(s._symbolName, []);
        bySymbol.get(s._symbolName).push(s);
    });
    const weight = (sigs) => sigs.reduce((sum, s) => sum + Math.abs(s.Points || 0), 0);
    const groups = [...bySymbol.entries()].sort((a, b) => weight(b[1]) - weight(a[1]));
    return { ungrouped, groups };
}

// Mirrors blastRadiusCategories/reviewPriorityCategories in blastradius.go:
// every Signal's Category maps to exactly one of these two dimensions,
// except "diff-shape" (hygiene) signals, which are informational/dampening
// only and feed neither raw score - see splitSignalsByDimension.
const BLAST_CATEGORIES = new Set(['architecture', 'graph']);
const PRIORITY_CATEGORIES = new Set(['duplication', 'code-metrics']);

// splitSignalsByDimension groups a flat signal list the same way the Go
// backend sums them into BlastRadiusRaw/ReviewPriorityRaw, so the UI's
// grouping can never drift from the actual math - see sumSignalPoints in
// blastradius.go. `other` holds diff-shape (hygiene) signals, which are
// shown separately since they don't contribute to either raw score.
function splitSignalsByDimension(signals) {
    const blast = [];
    const priority = [];
    const other = [];
    (signals || []).forEach((s) => {
        if (BLAST_CATEGORIES.has(s.Category)) blast.push(s);
        else if (PRIORITY_CATEGORIES.has(s.Category)) priority.push(s);
        else other.push(s);
    });
    return { blast, priority, other };
}

// sumExpression spells a list of numbers out as a literal "1.8 + 1.4 - 0.9"
// arithmetic expression (Math Mode's whole point: never just assert a total,
// always show the addition that produces it) plus the total itself.
function sumExpression(nums) {
    if (!nums || nums.length === 0) return { text: '0', total: 0 };
    const total = nums.reduce((a, b) => a + b, 0);
    const text = nums.reduce((acc, n, i) => {
        if (i === 0) return n.toFixed(1);
        return `${acc} ${n >= 0 ? '+' : '-'} ${Math.abs(n).toFixed(1)}`;
    }, '');
    return { text, total };
}

function groupCallersByDepth(callers) {
    const groups = new Map();
    (callers || []).forEach((c) => {
        const depth = c.Depth || 1;
        if (!groups.has(depth)) groups.set(depth, []);
        groups.get(depth).push(c);
    });
    return [...groups.entries()].sort((a, b) => a[0] - b[0]);
}

function depthLabel(depth) {
    if (depth === 1) return 'Direct callers';
    return `${depth} calls away`;
}

export async function createBlastRadiusPanel() {
    const { html, useState, useRef, useCallback, useEffect } = await waitForPreact();
    const SunburstChart = await getSunburstChart();
    const FlameGraph = await getFlameGraph();

    // A Signal with Points exactly 0 means "checked, found nothing" (e.g.
    // "does not write persistent data") rather than "not evaluated" - kept
    // out of the ranked active list (which would otherwise be swamped with
    // zeros) but never dropped, since the user should always be able to see
    // every factor that was considered, not just the ones that fired.
    function SignalRow({ s, dormant }) {
        const cls = dormant ? 'dormant' : ((s.Points || 0) < 0 ? 'negative' : 'positive');
        return html`
            <li class="blast-signal ${cls}">
                <span class="blast-signal-points">${dormant ? '+0.0' : `${(s.Points || 0) >= 0 ? '+' : ''}${(s.Points || 0).toFixed(1)}`}</span>
                <span class="blast-signal-name">${s.Name}</span>
                ${s.Detail && html`<span class="blast-signal-detail">${s.Detail}</span>`}
            </li>
        `;
    }

    // Key includes _symbolName+Name (not just idx) so reordering the list
    // (e.g. re-sorting after a signal's Points changes, or switching which
    // symbol is selected) reconciles against the same row rather than a
    // stale one reused for different content. idx stays as a final
    // tiebreak: a symbol implementing several interfaces can legitimately
    // produce multiple same-named "Interface implementation" signals.
    function SignalRows({ signals, dormant, keyPrefix }) {
        return html`${signals.map((s, idx) => html`<${SignalRow} key=${`${keyPrefix}-${s._symbolName || 'hunk'}-${s.Name}-${idx}`} s=${s} dormant=${dormant} />`)}`;
    }

    // Renders a signal list either flat (one <ul>, the common single-symbol
    // case) or as per-symbol sections (a header per symbol, ordered by that
    // symbol's contribution to this list) whenever it mixes signals from
    // more than one symbol - see groupSignalsBySymbol. Hunk-level signals
    // (no symbol) render in a plain, unheaded list ahead of the sections.
    function SignalBlock({ signals, dormant, keyPrefix }) {
        if (distinctSymbolCount(signals) <= 1) {
            return html`<ul class="blast-signal-list"><${SignalRows} signals=${signals} dormant=${dormant} keyPrefix=${keyPrefix} /></ul>`;
        }
        const { ungrouped, groups } = groupSignalsBySymbol(signals);
        return html`
            <div class="blast-signal-sections">
                ${ungrouped.length > 0 && html`
                    <ul class="blast-signal-list"><${SignalRows} signals=${ungrouped} dormant=${dormant} keyPrefix=${`${keyPrefix}-u`} /></ul>
                `}
                ${groups.map(([name, sigs]) => html`
                    <div key=${name} class="blast-signal-section">
                        <div class="blast-signal-section-header">${shortName(name)}</div>
                        <ul class="blast-signal-list"><${SignalRows} signals=${sigs} dormant=${dormant} keyPrefix=${`${keyPrefix}-${name}`} /></ul>
                    </div>
                `)}
            </div>
        `;
    }

    function SignalList({ signals }) {
        const [showDormant, setShowDormant] = useState(false);
        const ranked = sortedSignals(signals);
        const active = ranked.filter((s) => (s.Points || 0) !== 0);
        const dormant = ranked.filter((s) => (s.Points || 0) === 0);
        if (active.length === 0 && dormant.length === 0) {
            return html`<div class="blast-signal-empty">No signals recorded.</div>`;
        }
        return html`
            <${SignalBlock} signals=${active} keyPrefix="active" />
            ${showDormant && html`<${SignalBlock} signals=${dormant} dormant=${true} keyPrefix="dormant" />`}
            ${dormant.length > 0 && html`
                <button class="blast-chip-more" onClick=${() => setShowDormant(!showDormant)}>
                    ${showDormant ? 'hide checked-but-inactive signals' : `${dormant.length} signal${dormant.length !== 1 ? 's' : ''} checked, not contributing`}
                </button>
            `}
        `;
    }

    // computePopupPos measures triggerEl against the viewport and returns
    // position:fixed coordinates, flipping above the trigger when there
    // isn't room below - same technique as RiskBadge.js's HoverCard. This
    // first pass relies on an *estimated* height (the real content isn't
    // rendered yet), so the flip decision can be wrong for content taller
    // than the estimate - see usePopupClamp, which corrects for that once
    // the popup has actually rendered.
    function computePopupPos(triggerEl, width, estHeight) {
        const rect = triggerEl.getBoundingClientRect();
        const left = Math.max(8, Math.min(rect.left, window.innerWidth - width - 12));
        if (rect.bottom + estHeight > window.innerHeight && rect.top > estHeight) {
            return { left, bottom: window.innerHeight - rect.top + 8, placeAbove: true };
        }
        return { left, top: rect.bottom + 8, placeAbove: false };
    }

    // usePopupClamp is a second, corrective pass: once the popup has
    // actually rendered (real content, real height - no longer an
    // estimate), nudge it back inside the viewport if it still overflows
    // top or bottom. Converges in one adjustment (the corrected position no
    // longer overflows, so the effect's own re-run is a no-op).
    function usePopupClamp(pos, setPos, popupRef) {
        useEffect(() => {
            if (!pos || !popupRef.current) return;
            const rect = popupRef.current.getBoundingClientRect();
            if (pos.placeAbove) {
                const shortfall = 8 - rect.top;
                if (shortfall > 0.5) {
                    setPos((p) => (p ? { ...p, bottom: p.bottom - shortfall } : p));
                }
            } else {
                const overflow = rect.bottom - (window.innerHeight - 8);
                if (overflow > 0.5) {
                    setPos((p) => (p ? { ...p, top: Math.max(8, p.top - overflow) } : p));
                }
            }
        }, [pos]);
    }

    function ScoreChipWithHelp({ hintKey, chipClass, children }) {
        const [pos, setPos] = useState(null);
        const triggerRef = useRef(null);
        const popupRef = useRef(null);
        const hideTimerRef = useRef(null);
        const cancelHide = useCallback(() => {
            if (hideTimerRef.current) { clearTimeout(hideTimerRef.current); hideTimerRef.current = null; }
        }, []);
        const scheduleHide = useCallback(() => {
            cancelHide();
            hideTimerRef.current = setTimeout(() => setPos(null), HIDE_DELAY_MS);
        }, [cancelHide]);
        const show = useCallback(() => {
            cancelHide();
            const el = triggerRef.current;
            if (!el) return;
            setPos(computePopupPos(el, HELP_POPUP_WIDTH, HELP_POPUP_EST_HEIGHT));
        }, [cancelHide]);
        usePopupClamp(pos, setPos, popupRef);
        const hint = SCORE_HINTS[hintKey] || {};
        const popupStyle = pos && (pos.placeAbove
            ? `left: ${pos.left}px; bottom: ${pos.bottom}px;`
            : `left: ${pos.left}px; top: ${pos.top}px;`);
        return html`
            <span
                class="blast-score-chip-wrap"
                onMouseLeave=${scheduleHide}
            >
                <span class="${chipClass || ''}" title="${hint.title || ''}">${children}</span>
                <button
                    ref=${triggerRef}
                    type="button"
                    class="blast-help-btn"
                    aria-label="What does this score mean?"
                    title="${hint.title || ''}"
                    onMouseEnter=${show}
                    onMouseLeave=${scheduleHide}
                    onFocus=${show}
                    onBlur=${scheduleHide}
                    onClick=${(e) => { e.stopPropagation(); show(); }}
                >
                    ${renderIcon(html, 'info', { size: 12 })}
                </button>
                ${pos && html`
                    <span
                        ref=${popupRef}
                        class="blast-help-popup"
                        role="tooltip"
                        style=${popupStyle}
                        onMouseEnter=${cancelHide}
                        onMouseLeave=${scheduleHide}
                    >
                        <span class="blast-help-popup-title">${hint.title || ''}</span>
                        <span class="blast-help-popup-body">${hint.body || ''}</span>
                    </span>
                `}
            </span>
        `;
    }

    function MethodologyButton() {
        const [pos, setPos] = useState(null);
        const triggerRef = useRef(null);
        const popupRef = useRef(null);
        const hideTimerRef = useRef(null);
        const cancelHide = useCallback(() => {
            if (hideTimerRef.current) { clearTimeout(hideTimerRef.current); hideTimerRef.current = null; }
        }, []);
        const scheduleHide = useCallback(() => {
            cancelHide();
            hideTimerRef.current = setTimeout(() => setPos(null), HIDE_DELAY_MS);
        }, [cancelHide]);
        const show = useCallback(() => {
            cancelHide();
            const el = triggerRef.current;
            if (!el) return;
            setPos(computePopupPos(el, METHODOLOGY_POPUP_WIDTH, METHODOLOGY_POPUP_EST_HEIGHT));
        }, [cancelHide]);
        usePopupClamp(pos, setPos, popupRef);
        const popupStyle = pos && (pos.placeAbove
            ? `left: ${pos.left}px; bottom: ${pos.bottom}px;`
            : `left: ${pos.left}px; top: ${pos.top}px;`);
        return html`
            <span
                ref=${triggerRef}
                class="blast-methodology-wrap"
                onMouseEnter=${show}
                onMouseLeave=${scheduleHide}
            >
                <button
                    type="button"
                    class="blast-methodology-btn"
                    aria-label="How is this scored?"
                    title="How is this scored?"
                    onClick=${(e) => { e.stopPropagation(); show(); }}
                    onFocus=${show}
                    onBlur=${scheduleHide}
                >
                    ${renderIcon(html, 'help', { size: 12 })} How is this scored?
                </button>
                ${pos && html`
                    <div
                        ref=${popupRef}
                        class="blast-methodology-popup"
                        role="tooltip"
                        style=${popupStyle}
                        onMouseEnter=${cancelHide}
                        onMouseLeave=${scheduleHide}
                    >
                        <div class="blast-methodology-title">How is this scored?</div>
                        ${METHODOLOGY_PARAGRAPHS.map((p, i) => html`
                            <p key=${i} class="blast-methodology-p">${p}</p>
                        `)}
                    </div>
                `}
            </span>
        `;
    }

    function ChipList({ items, preview, chipClass, label }) {
        const [showAll, setShowAll] = useState(false);
        if (!items || items.length === 0) return null;
        const visible = showAll ? items : items.slice(0, preview);
        const hidden = items.length - visible.length;
        return html`
            <div class="blast-chip-row">
                ${label && html`<span class="blast-chip-row-label">${label}</span>`}
                ${visible.map((item) => html`<span key=${item} class="${chipClass}">${item}</span>`)}
                ${hidden > 0 && html`
                    <button class="blast-chip-more" onClick=${() => setShowAll(true)}>+${hidden} more</button>
                `}
                ${showAll && items.length > preview && html`
                    <button class="blast-chip-more" onClick=${() => setShowAll(false)}>show less</button>
                `}
            </div>
        `;
    }

    function CallerGroup({ depth, callers, hoveredCaller, onHoverCaller }) {
        const [showAll, setShowAll] = useState(false);
        const visible = showAll ? callers : callers.slice(0, CALLERS_PREVIEW);
        const hidden = callers.length - visible.length;
        return html`
            <div class="blast-caller-group">
                <div class="blast-caller-group-header">
                    ${depthLabel(depth)}
                    <span class="blast-caller-count">${callers.length}</span>
                </div>
                <div class="blast-caller-list ${showAll ? 'expanded' : ''}">
                    ${visible.map((c) => html`
                        <span
                            key=${c.QualifiedName}
                            class="blast-caller ${hoveredCaller === c.QualifiedName ? 'highlighted' : ''}"
                            title="${c.QualifiedName}"
                            onMouseEnter=${() => onHoverCaller(c.QualifiedName)}
                            onMouseLeave=${() => onHoverCaller(null)}
                        >
                            ${shortName(c.QualifiedName)}
                        </span>
                    `)}
                    ${hidden > 0 && html`
                        <button class="blast-chip-more" onClick=${() => setShowAll(true)}>+${hidden} more</button>
                    `}
                    ${showAll && callers.length > CALLERS_PREVIEW && html`
                        <button class="blast-chip-more" onClick=${() => setShowAll(false)}>show less</button>
                    `}
                </div>
            </div>
        `;
    }

    function metricChips(sym) {
        const chips = [];
        if (sym.IsEntryPoint) chips.push({ label: 'entry point', title: 'This symbol is a service entry point' });
        if (sym.Complexity > 0) chips.push({ label: `complexity ${sym.Complexity}`, title: 'Cyclomatic complexity' });
        if (sym.Cognitive > 0) chips.push({ label: `cognitive ${sym.Cognitive}`, title: 'Cognitive complexity' });
        if (sym.LoopDepth > 0) chips.push({ label: `loop depth ${sym.LoopDepth}`, title: 'Deepest loop nesting' });
        if (sym.OutDegree > 0) chips.push({ label: `calls out ${sym.OutDegree}`, title: 'Functions this symbol calls (fan-out)' });
        chips.push(sym.TestCount > 0
            ? { label: `${sym.TestCount} test${sym.TestCount !== 1 ? 's' : ''}`, title: 'Direct test coverage' }
            : { label: 'no tests', title: 'No direct test coverage found', warn: true });
        return chips;
    }

    function SymbolNavBar({ symbols, selectedIdx, onSelect }) {
        if (!symbols || symbols.length <= 1) return null;
        const current = symbols[selectedIdx];
        return html`
            <div class="blast-symbol-nav">
                <button
                    class="blast-symbol-nav-btn"
                    onClick=${() => onSelect(selectedIdx === 0 ? symbols.length - 1 : selectedIdx - 1)}
                    title="Previous symbol"
                >
                    ${renderIcon(html, 'chevronLeft', { size: 14 })}
                </button>
                <span class="blast-symbol-nav-info">
                    <span class="blast-symbol-nav-name" title=${current.QualifiedName}>${current.Name || shortName(current.QualifiedName)}</span>
                    <span class="blast-symbol-nav-kind">${current.Label}</span>
                    <span class="blast-symbol-nav-pos">${selectedIdx + 1} of ${symbols.length}</span>
                </span>
                <button
                    class="blast-symbol-nav-btn"
                    onClick=${() => onSelect(selectedIdx === symbols.length - 1 ? 0 : selectedIdx + 1)}
                    title="Next symbol"
                >
                    ${renderIcon(html, 'chevronRight', { size: 14 })}
                </button>
            </div>
        `;
    }

    function SymbolDetail({ sym, hoveredCaller, onHoverCaller }) {
        const callerGroups = groupCallersByDepth(sym.Callers);
        const totalCallers = (sym.Callers || []).length;
        return html`
            <div class="blast-symbol-detail">
                <div class="blast-metric-chips">
                    ${metricChips(sym).map((chip) => html`
                        <span key=${chip.label} class="blast-metric-chip ${chip.warn ? 'warn' : ''}" title="${chip.title}">${chip.label}</span>
                    `)}
                </div>
                <${SignalList} signals=${sym.Signals} />
                ${totalCallers > 0 && html`
                    <div class="blast-callers">
                        <div class="blast-section-title">Reached from ${totalCallers} caller${totalCallers !== 1 ? 's' : ''}</div>
                        ${callerGroups.map(([depth, callers]) => html`
                            <${CallerGroup} key=${depth} depth=${depth} callers=${callers} hoveredCaller=${hoveredCaller} onHoverCaller=${onHoverCaller} />
                        `)}
                    </div>
                `}
                <${ChipList}
                    items=${sym.ImpactedPackages}
                    preview=${PKGS_PREVIEW}
                    chipClass="blast-pkg-chip"
                    label="Impacted packages (${(sym.ImpactedPackages || []).length})"
                />
            </div>
        `;
    }

    // normMathText spells out in plain language how a 0-100 score was
    // derived from its raw point total, so the number is traceable rather
    // than asserted - see HunkReport.MaxBlastRadiusRaw/MaxReviewPriorityRaw
    // in blastradius.go. max is 0 only when every hunk in the diff scored 0
    // raw points for this dimension (there's no "highest" to compare to).
    function normMathText(raw, max, norm) {
        const r = (raw || 0).toFixed(1);
        if (!max) return `No signals fired for this hunk, or any other hunk in this diff - scores 0/100.`;
        return `${r} points here. The highest-scoring hunk in this diff scored ${max.toFixed(1)}, so this one ranks ${Math.round(norm || 0)}/100 by comparison.`;
    }

    // renderMathDimension builds one dimension's (Blast Radius or Review
    // Priority) full worked derivation: every symbol's signals summed with
    // the addition spelled out, those subtotals summed into the raw total,
    // then the raw total scaled against this diff's highest-scoring hunk.
    // Steps are numbered dynamically (not fixed offsets) so a hunk with only
    // one contributing part skips the redundant "add subtotals together"
    // step without leaving a gap in the numbering - callers thread
    // startStep/nextStep through so numbering continues correctly across
    // Blast Radius, then Review Priority, then the final blend.
    function fileBaseName(path) {
        return (path || '').split('/').pop() || path;
    }

    function renderMathDimension(startStep, title, symbolLines, hunkSignals, raw, max, norm, maxSource, self) {
        const perSymbol = symbolLines.map((l) => ({ name: l.name, ...sumExpression(l.signals.map((s) => s.Points || 0)) }));
        const hunkExpr = hunkSignals.length > 0 ? sumExpression(hunkSignals.map((s) => s.Points || 0)) : null;
        const subtotalNums = [...perSymbol.map((l) => l.total), ...(hunkExpr ? [hunkExpr.total] : [])];
        const hasMultipleParts = subtotalNums.length > 1;
        const totalExpr = sumExpression(subtotalNums);

        let n = startStep;
        const stepAdd = n++;
        const stepSum = hasMultipleParts ? n++ : null;
        const stepScale = n++;

        // isSelf: this very hunk is the one that set the max - "divided by
        // itself" needs saying explicitly, since otherwise it looks like an
        // unexplained external reference the same way the plain number did.
        const isSelf = self && maxSource.file === self.file && maxSource.header === self.header;

        const node = html`
            <div class="math-section">
                <div class="math-section-title">${title}</div>
                <div class="math-step">
                    <div class="math-step-label">Step ${stepAdd} — add up each symbol's own signals</div>
                    ${perSymbol.length === 0 && !hunkExpr && html`<div class="math-step-line dim">No signals fired for this dimension.</div>`}
                    ${perSymbol.map((l, i) => html`
                        <div key=${i} class="math-step-line">
                            <span class="math-step-subject">${l.name}</span>: ${l.text} = <strong>${l.total.toFixed(1)}</strong>
                        </div>
                    `)}
                    ${hunkExpr && html`
                        <div class="math-step-line">
                            <span class="math-step-subject">This hunk (file-level signals)</span>: ${hunkExpr.text} = <strong>${hunkExpr.total.toFixed(1)}</strong>
                        </div>
                    `}
                </div>
                ${stepSum !== null && html`
                    <div class="math-step">
                        <div class="math-step-label">Step ${stepSum} — add every subtotal together</div>
                        <div class="math-step-line">${totalExpr.text} = <strong>${totalExpr.total.toFixed(1)}</strong></div>
                    </div>
                `}
                <div class="math-step">
                    <div class="math-step-label">Step ${stepScale} — scale against the highest-scoring hunk in this diff</div>
                    <div class="math-step-line">
                        ${max
                            ? html`${(raw || 0).toFixed(1)} ÷ ${max.toFixed(1)} × 100 = <strong>${(norm || 0).toFixed(1)}</strong>`
                            : html`no hunk in this diff scored above 0 for this dimension, so this is <strong>0</strong>`}
                    </div>
                    ${max > 0 && html`
                        <div class="math-step-line dim">
                            ${isSelf
                                ? `${max.toFixed(1)} is this very hunk's own score - it's the highest for this dimension anywhere in this diff.`
                                : `${max.toFixed(1)} is the highest score for this dimension anywhere in this diff, from ${maxSource.file ? fileBaseName(maxSource.file) : 'another hunk'}${maxSource.header ? ` (${maxSource.header})` : ''}.`}
                        </div>
                    `}
                </div>
            </div>
        `;
        return { node, nextStep: n };
    }

    // MathModeView is the "show your work" alternative to the dimension
    // cards: every arithmetic step from each symbol's individual signals
    // through to the final Combined score, numbered and labeled, so the
    // score can be checked by hand rather than trusted as a rollup.
    function MathModeView({ detail }) {
        const symbols = detail.Symbols || [];
        const weights = detail.Weights && typeof detail.Weights.BlastRadius === 'number'
            ? detail.Weights
            : { BlastRadius: 0.6, ReviewPriority: 0.4 };

        function dimensionInputs(categories) {
            const symbolLines = symbols
                .map((sym) => ({
                    name: sym.Name || shortName(sym.QualifiedName),
                    signals: (sym.Signals || []).filter((s) => categories.has(s.Category)),
                }))
                .filter((l) => l.signals.length > 0);
            const hunkSigs = (detail.Signals || []).filter((s) => categories.has(s.Category));
            return { symbolLines, hunkSigs };
        }

        const blastNorm = detail.BlastRadiusNorm || 0;
        const priorityNorm = detail.ReviewPriorityNorm || 0;
        const blastInputs = dimensionInputs(BLAST_CATEGORIES);
        const priorityInputs = dimensionInputs(PRIORITY_CATEGORIES);
        // self identifies *this* hunk so renderMathDimension can tell "the
        // highest score is this very hunk" apart from "it's some other hunk
        // over there" - see HunkReport.MaxBlastRadiusHunkFile/Header in
        // blastradius.go, which track exactly which hunk set each max.
        const self = { file: detail.FilePath, header: detail.Header };

        const { node: blastNode, nextStep: afterBlast } = renderMathDimension(
            1, 'Blast Radius', blastInputs.symbolLines, blastInputs.hunkSigs,
            detail.BlastRadiusRaw, detail.MaxBlastRadiusRaw, blastNorm,
            { file: detail.MaxBlastRadiusHunkFile, header: detail.MaxBlastRadiusHunkHeader }, self,
        );
        const { node: priorityNode, nextStep: afterPriority } = renderMathDimension(
            afterBlast, 'Review Priority', priorityInputs.symbolLines, priorityInputs.hunkSigs,
            detail.ReviewPriorityRaw, detail.MaxReviewPriorityRaw, priorityNorm,
            { file: detail.MaxReviewPriorityHunkFile, header: detail.MaxReviewPriorityHunkHeader }, self,
        );

        const hygiene = typeof detail.HygieneMultiplier === 'number' ? detail.HygieneMultiplier : 1.0;
        const blastShare = weights.BlastRadius * blastNorm;
        const priorityShare = weights.ReviewPriority * priorityNorm;
        const blended = blastShare + priorityShare;
        const final = blended * hygiene;
        const stepBlend = afterPriority;
        const stepHygiene = afterPriority + 1;

        return html`
            <div class="math-mode">
                ${blastNode}
                ${priorityNode}
                <div class="math-section">
                    <div class="math-section-title">Combine Into Final Score</div>
                    <div class="math-step">
                        <div class="math-step-label">Step ${stepBlend} — blend Blast Radius and Review Priority</div>
                        <div class="math-step-line">
                            (${weights.BlastRadius.toFixed(2)} × ${blastNorm.toFixed(1)}) + (${weights.ReviewPriority.toFixed(2)} × ${priorityNorm.toFixed(1)})
                            = ${blastShare.toFixed(1)} + ${priorityShare.toFixed(1)} = <strong>${blended.toFixed(1)}</strong>
                        </div>
                    </div>
                    <div class="math-step">
                        <div class="math-step-label">Step ${stepHygiene} — apply the hygiene multiplier</div>
                        <div class="math-step-line">
                            ${blended.toFixed(1)} × ${hygiene} = <strong>${final.toFixed(1)}</strong>
                        </div>
                    </div>
                    <div class="math-step final">
                        <div class="math-step-label">Final Score</div>
                        <div class="math-step-line">Rounded to the nearest whole number: <strong>${Math.round(detail.Combined || 0)}</strong> out of 100</div>
                    </div>
                </div>
            </div>
        `;
    }

    // DimensionCard makes the score hierarchy explicit at every level: the
    // card's own header shows exactly how its norm score was derived from
    // its raw sum (normMathText), and its signal list is exactly the
    // Signals that raw sum is made of - the same relationship SignalList's
    // active+dormant split already gives at the per-symbol level, one level
    // up. Two of these side by side (see .blast-dimension-grid) replace what
    // used to be one flat, unattributed signal list mixing both dimensions.
    function DimensionCard({ title, norm, raw, max, signals }) {
        return html`
            <div class="blast-dimension-card">
                <div class="blast-dimension-header">
                    <span class="blast-dimension-title">${title}</span>
                    <span class="blast-dimension-math">${normMathText(raw, max, norm)}</span>
                </div>
                <${SignalList} signals=${signals} />
            </div>
        `;
    }

    return function BlastRadiusPanel({ detail }) {
        if (!detail) return null;

        const hygieneMult = typeof detail.HygieneMultiplier === 'number' ? detail.HygieneMultiplier : 1.0;
        const couplingVal = typeof detail.FileCouplingBonus === 'number' ? detail.FileCouplingBonus : 0;
        const symbols = [...(detail.Symbols || [])].sort((a, b) => (b.BlastRadiusRaw || 0) - (a.BlastRadiusRaw || 0));
        const { blast: blastSignals, priority: prioritySignals, other: otherSignals } = splitSignalsByDimension(allSignals(detail));
        const [selectedIdx, setSelectedIdx] = useState(0);
        const [vizMode, setVizMode] = useState('sunburst');
        const [hoveredCaller, setHoveredCaller] = useState(null);
        const [scoreMode, setScoreMode] = useState('summary');

        return html`
            <div class="blast-panel">
                <div class="blast-panel-scores">
                    <${ScoreChipWithHelp}
                        hintKey="combined"
                        chipClass="blast-score-chip primary ${blastRadiusTier(detail.Combined || 0)}"
                    >
                        Score ${Math.round(detail.Combined || 0)}
                    </${ScoreChipWithHelp}>
                    <${ScoreChipWithHelp}
                        hintKey="blast"
                        chipClass="blast-score-chip"
                    >
                        Blast ${Math.round(detail.BlastRadiusNorm || 0)}
                    </${ScoreChipWithHelp}>
                    <${ScoreChipWithHelp}
                        hintKey="priority"
                        chipClass="blast-score-chip"
                    >
                        Priority ${Math.round(detail.ReviewPriorityNorm || 0)}
                    </${ScoreChipWithHelp}>
                    ${hygieneMult < 1
                        ? html`<${ScoreChipWithHelp} hintKey="hygiene" chipClass="blast-score-chip hygiene">×${hygieneMult}</${ScoreChipWithHelp}>`
                        : html`<${ScoreChipWithHelp} hintKey="hygiene" chipClass="blast-score-chip hygiene-dormant">Hygiene ×1.0</${ScoreChipWithHelp}>`
                    }
                    ${couplingVal > 0
                        ? html`<${ScoreChipWithHelp} hintKey="coupling" chipClass="blast-score-chip">Coupling +${couplingVal.toFixed(1)}</${ScoreChipWithHelp}>`
                        : html`<${ScoreChipWithHelp} hintKey="coupling" chipClass="blast-score-chip coupling-dormant">Coupling +0</${ScoreChipWithHelp}>`
                    }
                    <${MethodologyButton} />
                </div>
                ${otherSignals.length > 0 && html`
                    <div class="blast-hygiene-note">
                        ${otherSignals.map((s, i) => html`<span key=${i}>${s.Name}${s.Detail ? ` — ${s.Detail}` : ''}</span>`)}
                    </div>
                `}
                <${ChipList}
                    items=${detail.ImpactedPackages}
                    preview=${PKGS_PREVIEW}
                    chipClass="blast-pkg-chip"
                    label="Impacted packages (${(detail.ImpactedPackages || []).length})"
                />
                <div class="blast-score-mode-toggle">
                    <button class=${scoreMode === 'summary' ? 'active' : ''} onClick=${() => setScoreMode('summary')}>
                        Summary
                    </button>
                    <button class=${scoreMode === 'math' ? 'active' : ''} onClick=${() => setScoreMode('math')}>
                        ${renderIcon(html, 'help', { size: 12 })} Math Mode
                    </button>
                </div>
                ${scoreMode === 'math'
                    ? html`<${MathModeView} detail=${detail} />`
                    : html`
                        <div class="blast-dimension-grid">
                            <${DimensionCard}
                                title="Blast Radius"
                                norm=${detail.BlastRadiusNorm}
                                raw=${detail.BlastRadiusRaw}
                                max=${detail.MaxBlastRadiusRaw}
                                signals=${blastSignals}
                            />
                            <${DimensionCard}
                                title="Review Priority"
                                norm=${detail.ReviewPriorityNorm}
                                raw=${detail.ReviewPriorityRaw}
                                max=${detail.MaxReviewPriorityRaw}
                                signals=${prioritySignals}
                            />
                        </div>
                    `
                }
                ${symbols.length > 0 && html`
                    <div class="blast-panel-columns">
                        <div class="blast-panel-left">
                            <${SymbolNavBar}
                                symbols=${symbols}
                                selectedIdx=${selectedIdx}
                                onSelect=${(idx) => setSelectedIdx(idx)}
                            />
                            <${SymbolDetail} sym=${symbols[selectedIdx]} hoveredCaller=${hoveredCaller} onHoverCaller=${setHoveredCaller} />
                        </div>
                        <div class="blast-panel-right">
                            <div class="blast-viz-toggle">
                                <button class=${vizMode === 'sunburst' ? 'active' : ''} onClick=${() => setVizMode('sunburst')}>
                                    ${renderIcon(html, 'blastRadius', { size: 12 })} Sunburst
                                </button>
                                <button class=${vizMode === 'flamegraph' ? 'active' : ''} onClick=${() => setVizMode('flamegraph')}>
                                    ${renderIcon(html, 'reorder', { size: 12 })} Flamegraph
                                </button>
                            </div>
                            <div class="blast-viz-wrapper">
                                ${vizMode === 'sunburst'
                                    ? html`<${SunburstChart} symbol=${symbols[selectedIdx]} width=${SUNBURST_SIZE} height=${SUNBURST_SIZE} hoveredCaller=${hoveredCaller} onHoverCaller=${setHoveredCaller} />`
                                    : html`<${FlameGraph} symbol=${symbols[selectedIdx]} width=${SUNBURST_SIZE} height=${SUNBURST_SIZE} hoveredCaller=${hoveredCaller} onHoverCaller=${setHoveredCaller} />`
                                }
                            </div>
                        </div>
                    </div>
                `}
            </div>
        `;
    };
}

let BlastRadiusPanelComponent = null;
export async function getBlastRadiusPanel() {
    if (!BlastRadiusPanelComponent) {
        BlastRadiusPanelComponent = await createBlastRadiusPanel();
    }
    return BlastRadiusPanelComponent;
}
