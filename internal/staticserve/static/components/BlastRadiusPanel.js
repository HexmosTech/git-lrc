// BlastRadiusPanel - the full, drillable "why this score" breakdown for a
// hunk, fed by the /api/blastradius report joined onto the hunk as
// BlastDetail. Principle: NOTHING from the report is discarded - it is
// presented prioritized and collapsed, with every layer expandable:
//
//   scores + impacted-packages summary
//   └─ hunk-level signals (always visible, ranked)
//   └─ one card per touched symbol (collapsed: name, fan-in, contribution)
//        └─ full signal list, code metrics, impacted packages,
//           and the complete caller graph grouped by call depth
import { waitForPreact } from './utils.js';
import { renderIcon } from './icons.js';
import { blastRadiusTier } from './blast_radius_sort_state.mjs';

const CALLERS_PREVIEW = 8;
const PKGS_PREVIEW = 10;
const HIDE_DELAY_MS = 180;

// Brief, reviewer-pitched explanations for each score chip. Surfaced via the
// info icon's hover popup and the chip's native title attribute (for the
// classic browser tooltip path). Mirrors the standalone explorer's
// "How is this scored?" panel — see blastradius/explorer/index.html — kept
// short here since the full methodology is one click away.
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
    const { html, useState, useRef, useCallback } = await waitForPreact();

    function SignalList({ signals }) {
        const ranked = sortedSignals(signals);
        if (ranked.length === 0) {
            return html`<div class="blast-signal-empty">No signals recorded.</div>`;
        }
        return html`
            <ul class="blast-signal-list">
                ${ranked.map((s, idx) => html`
                    <li key=${idx} class="blast-signal ${(s.Points || 0) < 0 ? 'negative' : 'positive'}">
                        <span class="blast-signal-points">${(s.Points || 0) >= 0 ? '+' : ''}${(s.Points || 0).toFixed(1)}</span>
                        <span class="blast-signal-name">${s.Name}</span>
                        ${s.Detail && html`<span class="blast-signal-detail">${s.Detail}</span>`}
                        <span class="blast-signal-category">${s.Category}</span>
                    </li>
                `)}
            </ul>
        `;
    }

    // A score chip + an info icon that summons a hover popup with the brief
    // explanation from SCORE_HINTS. Same delayed-hover discipline as
    // RiskBadge's hover card (mouse can travel chip -> popup without it
    // vanishing, and the popup itself keeps the popup open while hovered).
    function ScoreChipWithHelp({ hintKey, chipClass, children }) {
        const [showHelp, setShowHelp] = useState(false);
        const hideTimerRef = useRef(null);
        const cancelHide = useCallback(() => {
            if (hideTimerRef.current) { clearTimeout(hideTimerRef.current); hideTimerRef.current = null; }
        }, []);
        const scheduleHide = useCallback(() => {
            cancelHide();
            hideTimerRef.current = setTimeout(() => setShowHelp(false), HIDE_DELAY_MS);
        }, [cancelHide]);
        const hint = SCORE_HINTS[hintKey] || {};
        return html`
            <span
                class="blast-score-chip-wrap"
                onMouseLeave=${scheduleHide}
            >
                <span class="${chipClass || ''}" title="${hint.title || ''}">${children}</span>
                <button
                    type="button"
                    class="blast-help-btn"
                    aria-label="What does this score mean?"
                    title="${hint.title || ''}"
                    onMouseEnter=${() => { cancelHide(); setShowHelp(true); }}
                    onMouseLeave=${scheduleHide}
                    onFocus=${() => { cancelHide(); setShowHelp(true); }}
                    onBlur=${scheduleHide}
                    onClick=${(e) => { e.stopPropagation(); cancelHide(); setShowHelp(true); }}
                >
                    ${renderIcon(html, 'info', { size: 12 })}
                </button>
                ${showHelp && html`
                    <span
                        class="blast-help-popup"
                        role="tooltip"
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

    // The "How is this scored?" button. Opens a single shared popup with the
    // full methodology text (mirrors the standalone explorer's explanation
    // panel — see blastradius/explorer/index.html).
    function MethodologyButton() {
        const [open, setOpen] = useState(false);
        const hideTimerRef = useRef(null);
        const cancelHide = useCallback(() => {
            if (hideTimerRef.current) { clearTimeout(hideTimerRef.current); hideTimerRef.current = null; }
        }, []);
        const scheduleHide = useCallback(() => {
            cancelHide();
            hideTimerRef.current = setTimeout(() => setOpen(false), HIDE_DELAY_MS);
        }, [cancelHide]);
        const show = useCallback(() => { cancelHide(); setOpen(true); }, [cancelHide]);
        return html`
            <span
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
                ${open && html`
                    <div
                        class="blast-methodology-popup"
                        role="tooltip"
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

    // Expandable chip list used for impacted packages: first N always shown,
    // the rest behind a "+N more" toggle - all data reachable, none dumped.
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

    // One depth level of the caller graph: preview list expandable to the
    // complete set (scrollable), so even 300+ callers stay reachable.
    function CallerGroup({ depth, callers }) {
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
                        <span key=${c.QualifiedName} class="blast-caller" title="${c.QualifiedName}">
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

    // One touched symbol: collapsed = identity + reach summary; expanded =
    // every signal, metric, impacted package, and the full caller graph.
    // Open state is controlled by the parent (accordion: at least one card
    // is always open) via isOpen/onToggle, not held per-card.
    function SymbolCard({ sym, isOpen, onToggle }) {
        const callerGroups = groupCallersByDepth(sym.Callers);
        const totalCallers = (sym.Callers || []).length;
        const reach = sym.Method === 'calls'
            ? `${sym.DirectCount || 0} direct · ${sym.TransitiveCount || 0} transitive callers`
            : 'matched by text reference';
        return html`
            <div class="blast-symbol ${isOpen ? 'open' : ''}">
                <button class="blast-symbol-header" onClick=${onToggle} aria-expanded=${isOpen}>
                    <span class="blast-symbol-toggle">${isOpen ? '▾' : '▸'}</span>
                    <span class="blast-symbol-name" title="${sym.QualifiedName}">${sym.Name || sym.QualifiedName}</span>
                    <span class="blast-symbol-kind">${sym.Label}</span>
                    <span class="blast-symbol-reach">${reach}</span>
                    <span class="blast-symbol-contrib" title="This symbol's contribution to the hunk's blast radius">
                        +${(sym.BlastRadiusRaw || 0).toFixed(1)}
                    </span>
                </button>
                ${isOpen && html`
                    <div class="blast-symbol-body">
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
                                    <${CallerGroup} key=${depth} depth=${depth} callers=${callers} />
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
                `}
            </div>
        `;
    }

    return function BlastRadiusPanel({ detail }) {
        if (!detail) return null;

        const hygiene = typeof detail.HygieneMultiplier === 'number' && detail.HygieneMultiplier < 1
            ? detail.HygieneMultiplier
            : null;
        const symbols = [...(detail.Symbols || [])].sort((a, b) => (b.BlastRadiusRaw || 0) - (a.BlastRadiusRaw || 0));

        // Accordion: at least one symbol card is always open. The first symbol
        // (highest contribution) defaults open; clicking an open card only
        // closes it if at least one other card is still open.
        const [openIdx, setOpenIdx] = useState(0);
        const toggle = (idx) => {
            if (idx === openIdx) {
                // Trying to close the currently-open card: allow only if
                // there's another card to fall open to (i.e. start the next
                // one). This keeps exactly one card open at all times, while
                // still letting the user switch focus freely.
                if (symbols.length > 1) {
                    setOpenIdx(idx === 0 ? 1 : 0);
                }
                // If there's only one symbol, it stays open.
            } else {
                setOpenIdx(idx);
            }
        };

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
                    ${hygiene !== null && html`
                        <${ScoreChipWithHelp}
                            hintKey="hygiene"
                            chipClass="blast-score-chip hygiene"
                        >
                            ×${hygiene}
                        </${ScoreChipWithHelp}>
                    `}
                    ${detail.FileCouplingBonus > 0 && html`
                        <${ScoreChipWithHelp}
                            hintKey="coupling"
                            chipClass="blast-score-chip"
                        >
                            Coupling +${detail.FileCouplingBonus.toFixed(1)}
                        </${ScoreChipWithHelp}>
                    `}
                    <${MethodologyButton} />
                </div>
                <${ChipList}
                    items=${detail.ImpactedPackages}
                    preview=${PKGS_PREVIEW}
                    chipClass="blast-pkg-chip"
                    label="Impacted packages (${(detail.ImpactedPackages || []).length})"
                />
                <${SignalList} signals=${detail.Signals} />
                ${symbols.length > 0 && html`
                    <div class="blast-panel-symbols">
                        <div class="blast-section-title">Symbols touched (${symbols.length}) — highest contribution first</div>
                        ${symbols.map((sym, idx) => html`
                            <${SymbolCard}
                                key=${sym.QualifiedName}
                                sym=${sym}
                                isOpen=${idx === openIdx}
                                onToggle=${() => toggle(idx)}
                            />
                        `)}
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
