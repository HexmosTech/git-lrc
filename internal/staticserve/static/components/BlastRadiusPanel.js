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
import { blastRadiusTier } from './blast_radius_sort_state.mjs';

const CALLERS_PREVIEW = 8;
const PKGS_PREVIEW = 10;

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
    const { html, useState } = await waitForPreact();

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
    function SymbolCard({ sym }) {
        const [open, setOpen] = useState(false);
        const callerGroups = groupCallersByDepth(sym.Callers);
        const totalCallers = (sym.Callers || []).length;
        const reach = sym.Method === 'calls'
            ? `${sym.DirectCount || 0} direct · ${sym.TransitiveCount || 0} transitive callers`
            : 'matched by text reference';
        return html`
            <div class="blast-symbol ${open ? 'open' : ''}">
                <button class="blast-symbol-header" onClick=${() => setOpen(!open)} aria-expanded=${open}>
                    <span class="blast-symbol-toggle">${open ? '▾' : '▸'}</span>
                    <span class="blast-symbol-name" title="${sym.QualifiedName}">${sym.Name || sym.QualifiedName}</span>
                    <span class="blast-symbol-kind">${sym.Label}</span>
                    <span class="blast-symbol-reach">${reach}</span>
                    <span class="blast-symbol-contrib" title="This symbol's contribution to the hunk's blast radius">
                        +${(sym.BlastRadiusRaw || 0).toFixed(1)}
                    </span>
                </button>
                ${open && html`
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

        return html`
            <div class="blast-panel">
                <div class="blast-panel-scores">
                    <span class="blast-score-chip primary ${blastRadiusTier(detail.Combined || 0)}" title="Combined 0-100 ranking score for this hunk relative to the rest of the diff">
                        Score ${Math.round(detail.Combined || 0)}
                    </span>
                    <span class="blast-score-chip" title="Blast radius: how widely this change can propagate (fan-in, entry-point reachability, architectural role)">
                        Blast ${Math.round(detail.BlastRadiusNorm || 0)}
                    </span>
                    <span class="blast-score-chip" title="Review priority: how much reviewer attention this hunk deserves (duplication, complexity, test coverage)">
                        Priority ${Math.round(detail.ReviewPriorityNorm || 0)}
                    </span>
                    ${hygiene !== null && html`
                        <span class="blast-score-chip hygiene" title="Hygiene dampener: this hunk looks low-value (formatting/comments/generated/logging/test-only), so its score is multiplied down">
                            ×${hygiene}
                        </span>
                    `}
                    ${detail.FileCouplingBonus > 0 && html`
                        <span class="blast-score-chip" title="Files that historically change together with this one add coupling risk">
                            coupling +${detail.FileCouplingBonus.toFixed(1)}
                        </span>
                    `}
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
                        ${symbols.map((sym) => html`
                            <${SymbolCard} key=${sym.QualifiedName} sym=${sym} />
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
