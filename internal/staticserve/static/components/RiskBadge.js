// RiskBadge - the face of Risk Assessment throughout the review UI.
// A compact tier-colored pill showing a hunk's 0-100 risk score that, on
// hover or keyboard focus, reveals a rich assessment card: headline score,
// blast-radius / review-priority dimension bars, the strongest signals with
// their signed contributions, and the hygiene dampener when active.
// Clicking invokes onOpen (the full signal breakdown panel).
//
// The card renders position:fixed at viewport coordinates (measured from the
// badge on reveal, flipping above the badge when there's no room below) so
// it can never be clipped by table rows, overflow containers, or stacking
// contexts - badges live deep inside the diff table.
//
// Used by hunk headers (size "large") and comment cards (size "small") so
// score and comment are always assessed together.
import { waitForPreact } from './utils.js';
import { renderIcon } from './icons.js';
import { blastRadiusTier, blastRadiusTierLabel, summarizeRiskDetail } from './blast_radius_sort_state.mjs';

const CARD_WIDTH = 300;
const CARD_EST_HEIGHT = 330; // used only for the above/below flip decision

export async function createRiskBadge() {
    const { html, useState, useRef, useCallback } = await waitForPreact();

    function DimensionBar({ label, value, hint }) {
        const pct = Math.max(0, Math.min(100, Math.round(value)));
        return html`
            <div class="risk-card-dim" title="${hint}">
                <span class="risk-card-dim-label">${label}</span>
                <span class="risk-card-dim-track"><span class="risk-card-dim-fill" style="width: ${pct}%"></span></span>
                <span class="risk-card-dim-value">${pct}</span>
            </div>
        `;
    }

    function HoverCard({ score, detail, pos }) {
        const summary = summarizeRiskDetail(detail);
        const tier = blastRadiusTier(score);
        const style = pos.placeAbove
            ? `left: ${pos.left}px; bottom: ${pos.bottom}px;`
            : `left: ${pos.left}px; top: ${pos.top}px;`;
        return html`
            <div class="risk-hover-card ${tier}" role="tooltip" style=${style}>
                <div class="risk-card-header">
                    <span class="risk-card-score">${Math.round(score)}</span>
                    <span class="risk-card-headline">
                        <span class="risk-card-tier">${blastRadiusTierLabel(score)}</span>
                        <span class="risk-card-kicker">Risk Assessment</span>
                    </span>
                </div>
                ${summary && html`
                    <div class="risk-card-dims">
                        <${DimensionBar} label="Blast Radius" value=${summary.blast} hint="How widely this change can propagate: callers, entry points, architectural role" />
                        <${DimensionBar} label="Review Priority" value=${summary.priority} hint="How much reviewer attention it deserves: duplication, complexity, test coverage" />
                    </div>
                    ${summary.hygiene !== null && html`
                        <div class="risk-card-hygiene">
                            Score dampened ×${summary.hygiene} — change looks low-value (formatting/comments/generated)
                        </div>
                    `}
                    ${summary.top.length > 0 && html`
                        <ul class="risk-card-signals">
                            ${summary.top.map((s, i) => html`
                                <li key=${i} class="risk-card-signal ${(s.Points || 0) < 0 ? 'negative' : ''}">
                                    <span class="risk-card-signal-pts">${(s.Points || 0) >= 0 ? '+' : ''}${(s.Points || 0).toFixed(1)}</span>
                                    <span class="risk-card-signal-name">${s.Name}</span>
                                </li>
                            `)}
                        </ul>
                    `}
                    <div class="risk-card-footer">
                        ${summary.moreCount > 0
                            ? `${summary.totalSignals} signals — click for the full breakdown`
                            : 'Click for the full breakdown'}
                    </div>
                `}
                ${!summary && html`
                    <div class="risk-card-footer">Relative importance of this hunk within the review</div>
                `}
            </div>
        `;
    }

    return function RiskBadge({ score, detail, size = 'small', expanded = false, onOpen }) {
        const [cardPos, setCardPos] = useState(null);
        const badgeRef = useRef(null);

        const showCard = useCallback(() => {
            const el = badgeRef.current;
            if (!el) return;
            const rect = el.getBoundingClientRect();
            const left = Math.max(8, Math.min(rect.left, window.innerWidth - CARD_WIDTH - 12));
            if (rect.bottom + CARD_EST_HEIGHT > window.innerHeight && rect.top > CARD_EST_HEIGHT) {
                setCardPos({ left, bottom: window.innerHeight - rect.top + 8, placeAbove: true });
            } else {
                setCardPos({ left, top: rect.bottom + 8, placeAbove: false });
            }
        }, []);
        const hideCard = useCallback(() => setCardPos(null), []);

        if (typeof score !== 'number') return null;
        const tier = blastRadiusTier(score);
        const clickable = Boolean(onOpen);
        return html`
            <span
                class="risk-badge-wrap risk-badge-${size}"
                onMouseEnter=${showCard}
                onMouseLeave=${hideCard}
            >
                <button
                    ref=${badgeRef}
                    class="risk-badge ${tier} ${clickable ? 'clickable' : ''}"
                    onClick=${clickable ? ((e) => { e.stopPropagation(); hideCard(); onOpen(); }) : undefined}
                    onFocus=${showCard}
                    onBlur=${hideCard}
                    aria-label="Risk score ${Math.round(score)} out of 100"
                    aria-expanded=${expanded}
                    type="button"
                >
                    ${renderIcon(html, 'blastRadius', { size: size === 'large' ? 11 : 10 })}
                    <span class="risk-badge-score">${Math.round(score)}</span>
                    ${size === 'large' && html`<span class="risk-badge-caret">${expanded ? '▾' : '▸'}</span>`}
                </button>
                ${cardPos && html`<${HoverCard} score=${score} detail=${detail} pos=${cardPos} />`}
            </span>
        `;
    };
}

let RiskBadgeComponent = null;
export async function getRiskBadge() {
    if (!RiskBadgeComponent) {
        RiskBadgeComponent = await createRiskBadge();
    }
    return RiskBadgeComponent;
}
