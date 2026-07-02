// Full-screen celebratory confetti burst for the "handoff succeeded" modal state.
// Pieces fall slowly from the top of the viewport rather than bursting out of the modal card.
const CONFETTI_COLORS = ['#d97757', '#0078d4', '#8b5cf6', '#4caf50', '#f5c451', '#ec4899'];
const PIECE_COUNT = 34;

function buildConfettiPieces() {
    const pieces = [];
    for (let i = 0; i < PIECE_COUNT; i++) {
        // Golden-angle spacing gives even, non-repeating horizontal coverage without Math.random()
        // (which would reshuffle on every re-render).
        const spread = (i * 137.5) % 100;
        const delay = (i * 83) % 1400;
        const duration = 3200 + ((i * 97) % 2200);
        const drift = ((i * 59) % 140) - 70;
        const rotate = 200 + ((i * 113) % 420);
        pieces.push({
            x: `${spread.toFixed(1)}%`,
            d: `${delay}ms`,
            dur: `${duration}ms`,
            dx: `${drift}px`,
            r: `${rotate}deg`,
            c: CONFETTI_COLORS[i % CONFETTI_COLORS.length],
            round: i % 3 === 0,
        });
    }
    return pieces;
}

const HANDOFF_CONFETTI_PIECES = buildConfettiPieces();

export function renderHandoffConfetti(html) {
    return html`
        <div class="handoff-confetti" aria-hidden="true">
            ${HANDOFF_CONFETTI_PIECES.map((piece, index) => html`
                <span
                    key=${index}
                    class="handoff-confetti-piece ${piece.round ? 'is-round' : ''}"
                    style="--x:${piece.x};--d:${piece.d};--dur:${piece.dur};--dx:${piece.dx};--r:${piece.r};--c:${piece.c};"
                ></span>
            `)}
        </div>
    `;
}
