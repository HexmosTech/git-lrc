// Header component
import { waitForPreact, LOGO_DATA_URI } from './utils.js';
import { UsageChip } from '/static/components/UsageChip.js';
import { fetchImpactStats, buildLinkedinText } from '/static/components/FeedbackPopup.js';
import { getReviewMeta } from '/static/components/reviewMeta.mjs';

const SESSION_REVIEW_ID = new URLSearchParams(window.location.search).get('r') || '';

const GITHUB_URL = 'https://github.com/HexmosTech/git-lrc';
const LIVEREVIEW_URL = 'https://hexmos.com/livereview/';

export async function createHeader() {
    const { html, useState, useEffect, useRef } = await waitForPreact();

    // ── shared hooks ──────────────────────────────────────────────────────────

    function useHoverPopover(delay = 180) {
        const [isOpen, setIsOpen] = useState(false);
        const timerRef = useRef(null);
        const open = () => {
            if (timerRef.current) { clearTimeout(timerRef.current); timerRef.current = null; }
            setIsOpen(true);
        };
        const closeSoon = () => {
            timerRef.current = setTimeout(() => setIsOpen(false), delay);
        };
        useEffect(() => () => { if (timerRef.current) clearTimeout(timerRef.current); }, []);
        return { isOpen, open, closeSoon, setIsOpen };
    }

    function useClickPopover() {
        const [isOpen, setIsOpen] = useState(false);
        const wrapRef = useRef(null);

        const toggle = () => setIsOpen(v => !v);
        const close = () => setIsOpen(false);

        useEffect(() => {
            if (!isOpen) return;
            const handler = (e) => {
                if (wrapRef.current && !wrapRef.current.contains(e.target)) close();
            };
            const esc = (e) => { if (e.key === 'Escape') close(); };
            document.addEventListener('mousedown', handler);
            document.addEventListener('keydown', esc);
            return () => {
                document.removeEventListener('mousedown', handler);
                document.removeEventListener('keydown', esc);
            };
        }, [isOpen]);

        return { isOpen, toggle, close, wrapRef };
    }

    // ── popover shell ─────────────────────────────────────────────────────────

    const POPUP_STYLE = 'background:#0d1520;border:1px solid rgba(99,130,180,0.22);border-radius:10px;box-shadow:0 10px 36px rgba(0,0,0,0.6);z-index:20000;';

    function Popover({ children }) {
        return html`
            <div style="position:absolute;left:0;top:calc(100% + 8px);${POPUP_STYLE}padding:14px 16px;width:240px;">
                ${children}
            </div>
        `;
    }

    // ── logo click popup ──────────────────────────────────────────────────────

    function LogoButton() {
        const { isOpen, open, closeSoon } = useHoverPopover();

        return html`
            <div style="position:relative;" onMouseEnter=${open} onMouseLeave=${closeSoon}>
                <div
                    class="logo-wrap"
                    style="cursor:default;"
                    title="About git-lrc"
                >
                    <img alt="LiveReview" src="${LOGO_DATA_URI}" />
                </div>
                ${isOpen && html`
                    <div onMouseEnter=${open} onMouseLeave=${closeSoon}>
                        <${Popover}>
                            <p style="font-size:13px;font-weight:600;color:#e8f0ff;margin:0 0 5px;">Thanks for choosing git-lrc!</p>
                            <p style="font-size:11px;color:#5a7aaa;margin:0 0 12px;line-height:1.5;">If it's been useful, a star on GitHub goes a long way.</p>
                            <a
                                href="${GITHUB_URL}"
                                target="_blank"
                                rel="noopener noreferrer"
                                style="display:flex;align-items:center;gap:6px;padding:7px 12px;background:rgba(255,255,255,0.07);border:1px solid rgba(255,255,255,0.12);border-radius:7px;color:#e8f0ff;font-size:12px;font-weight:500;text-decoration:none;transition:background 0.15s;width:fit-content;"
                            >
                                <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z"/></svg>
                                ⭐ Star on GitHub
                            </a>
                        </${Popover}>
                    </div>
                `}
            </div>
        `;
    }

    // ── brand text click popup ────────────────────────────────────────────────

    function BrandButton({ friendlyName, generatedTime }) {
        const { isOpen, open, closeSoon } = useHoverPopover();
        const [copied, setCopied] = useState(false);
        const copyTimer = useRef(null);

        useEffect(() => () => { if (copyTimer.current) clearTimeout(copyTimer.current); }, []);

        const copyURL = async (e) => {
            e.preventDefault();
            try {
                await navigator.clipboard.writeText(LIVEREVIEW_URL);
                setCopied(true);
                copyTimer.current = setTimeout(() => setCopied(false), 1500);
            } catch {}
        };

        return html`
            <div style="position:relative;">
                <h1 style="cursor:default;" title="Share LiveReview" onMouseEnter=${open} onMouseLeave=${closeSoon}>LiveReview Results</h1>
                ${(friendlyName || generatedTime) && html`
                    <div style="display:flex;align-items:center;gap:8px;margin-top:1px;">
                        ${friendlyName && html`<span style="color:#c9d5e8;font-size:11px;font-weight:600;">Run: ${friendlyName}</span>`}
                        ${friendlyName && generatedTime && html`<span style="color:#3a4a60;font-size:10px;">·</span>`}
                        ${generatedTime && html`<span style="color:#4a6080;font-size:11px;">${generatedTime}</span>`}
                    </div>
                `}
                ${isOpen && html`
                    <div onMouseEnter=${open} onMouseLeave=${closeSoon}>
                        <${Popover}>
                            <p style="font-size:13px;font-weight:600;color:#e8f0ff;margin:0 0 5px;">Share with friends & colleagues</p>
                            <p style="font-size:11px;color:#5a7aaa;margin:0 0 12px;line-height:1.5;">Know someone who'd benefit from AI-powered code reviews? Send them here.</p>
                            <div style="display:flex;align-items:center;gap:6px;">
                                <a
                                    href="${LIVEREVIEW_URL}"
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    style="flex:1;display:flex;align-items:center;gap:5px;padding:7px 10px;background:rgba(45,91,227,0.2);border:1px solid rgba(45,91,227,0.4);border-radius:7px;color:#93b4ff;font-size:11px;font-weight:500;text-decoration:none;transition:background 0.15s;overflow:hidden;white-space:nowrap;"
                                >
                                    <svg width="11" height="11" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"/></svg>
                                    hexmos.com/livereview
                                </a>
                                <button
                                    onClick=${copyURL}
                                    style="flex-shrink:0;padding:7px 9px;background:rgba(255,255,255,0.06);border:1px solid rgba(255,255,255,0.1);border-radius:7px;color:${copied ? '#4ade80' : '#6a7a99'};font-size:11px;cursor:pointer;transition:all 0.15s;white-space:nowrap;"
                                    title="Copy link"
                                >${copied ? '✓' : 'Copy'}</button>
                            </div>
                        </${Popover}>
                    </div>
                `}
            </div>
        `;
    }

    // ── feedback button ───────────────────────────────────────────────────────

    function AppFeedback() {
        const { isOpen, open, closeSoon, setIsOpen } = useHoverPopover();
        const [voteType, setVoteType] = useState(null);
        const [text, setText] = useState('');
        const [phase, setPhase] = useState('idle');
        const [statsExpanded, setStatsExpanded] = useState(false);
        const [impactStats, setImpactStats] = useState(null);
        const [linkedinOpen, setLinkedinOpen] = useState(false);
        const [linkedinOpacity, setLinkedinOpacity] = useState(0);
        const [linkedinText, setLinkedinText] = useState(() => buildLinkedinText(null));
        const [snackbar, setSnackbar] = useState(false);
        const snackTimer = useRef(null);

        const guardedCloseSoon = () => { if (linkedinOpen) return; closeSoon(); };

        const close = () => {
            if (linkedinOpen) return;
            setIsOpen(false);
            setVoteType(null);
            setText('');
            setPhase('idle');
            setStatsExpanded(false);
        };

        const handleVote = (v) => setVoteType(prev => prev === v ? null : v);
        const handleTextInput = (e) => setText(e.target.value);

        useEffect(() => {
            if (!isOpen) return;
            const { reviewID } = getReviewMeta();
            fetchImpactStats(reviewID, (data) => setImpactStats(data));
        }, [isOpen]);

        useEffect(() => {
            if (!isOpen) return;
            const handler = (e) => { if (e.key === 'Escape') { closeLinkedin(); close(); } };
            document.addEventListener('keydown', handler);
            return () => document.removeEventListener('keydown', handler);
        }, [isOpen, linkedinOpen]);

        useEffect(() => {
            if (phase !== 'done') return;
            const t = setTimeout(close, 2000);
            return () => clearTimeout(t);
        }, [phase]);

        useEffect(() => () => { if (snackTimer.current) clearTimeout(snackTimer.current); }, []);

        const openLinkedin = () => {
            setLinkedinText(buildLinkedinText(impactStats));
            setLinkedinOpen(true);
            setLinkedinOpacity(0);
            requestAnimationFrame(() => requestAnimationFrame(() => setLinkedinOpacity(1)));
        };
        const closeLinkedin = () => {
            setLinkedinOpacity(0);
            setTimeout(() => setLinkedinOpen(false), 200);
        };
        const handleCopyLinkedin = async (e) => {
            e.stopPropagation();
            try {
                await navigator.clipboard.writeText(linkedinText);
                setSnackbar(true);
                snackTimer.current = setTimeout(() => setSnackbar(false), 2200);
            } catch {}
        };

        const submit = async () => {
            if (!voteType || phase === 'submitting') return;
            setPhase('submitting');
            try {
                const feedbackURL = SESSION_REVIEW_ID ? `/api/v1/feedback?r=${SESSION_REVIEW_ID}` : '/api/v1/feedback';
                const res = await fetch(feedbackURL, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        vote_type: voteType,
                        source_type: 'general',
                        ...(text.trim() ? { feedback_text: text.trim() } : {}),
                    }),
                });
                setPhase(res.ok ? 'done' : 'error');
            } catch { setPhase('error'); }
        };

        const upActive = voteType === 'up';
        const downActive = voteType === 'down';
        const canSubmit = !!voteType && phase !== 'submitting';

        const ImpactLink = () => statsExpanded
            ? html`<div style="font-size:12px;color:#4a5a6a;padding:4px 0;user-select:none;">✨ Want to see your impact stats?</div>`
            : html`<div style="display:flex;align-items:center;gap:5px;color:#7aadff;cursor:pointer;font-size:12px;font-weight:500;padding:4px 0;user-select:none;" onMouseEnter=${() => setStatsExpanded(true)}>
                ✨ Want to see your impact stats?
                <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M9 18l6-6-6-6"/></svg>
            </div>`;

        const StatsGrid = () => {
            if (!impactStats) return html`<div style="font-size:12px;color:#4a5a6a;padding:8px 0;">Loading stats…</div>`;
            return html`
                <div style="margin-top:10px;">
                    <div style="display:grid;grid-template-columns:repeat(4,1fr);gap:5px;margin-bottom:10px;">
                        ${impactStats.map(s => html`
                            <div title=${s.tooltip} style="background:rgba(255,255,255,0.05);border:1px solid rgba(255,255,255,0.08);border-radius:8px;padding:8px 6px;text-align:center;">
                                <div style="font-size:17px;font-weight:700;color:#7aadff;line-height:1.2;">${s.value}</div>
                                <div style="font-size:10px;color:#6a88aa;margin-top:3px;line-height:1.3;">${s.label}</div>
                            </div>
                        `)}
                    </div>
                    <div style="border-top:1px solid rgba(255,255,255,0.07);padding-top:9px;">
                        <div
                            style="font-size:12px;font-weight:600;color:#7aadff;cursor:pointer;display:flex;align-items:center;gap:6px;"
                            onMouseEnter=${(e) => { e.currentTarget.style.color='#a8caff'; openLinkedin(); }}
                            onMouseLeave=${(e) => { e.currentTarget.style.color='#7aadff'; }}
                        >
                            <span>✦ Stand out by showing your impact stats to your peers</span>
                            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M9 18l6-6-6-6"/></svg>
                        </div>
                    </div>
                </div>
            `;
        };

        return html`
            <div style="position:relative;" onMouseEnter=${open} onMouseLeave=${guardedCloseSoon}>
                <button
                    style="display:flex;align-items:center;gap:5px;padding:6px 10px;background:rgba(99,130,180,0.08);border:1px solid rgba(99,130,180,0.2);border-radius:6px;color:#7a90b0;font-size:12px;font-weight:500;cursor:pointer;transition:all 0.15s;line-height:1;"
                    title="Share feedback"
                    type="button"
                >
                    <svg width="12" height="12" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
                    </svg>
                    Feedback
                </button>

                ${isOpen && html`
                    <div
                        style="position:absolute;right:0;top:calc(100% + 6px);${POPUP_STYLE}padding:14px 16px;width:340px;"
                        onMouseEnter=${open}
                        onMouseLeave=${guardedCloseSoon}
                        onClick=${(e) => e.stopPropagation()}
                    >
                        ${phase === 'done' ? html`
                            <div style="text-align:center;padding:6px 0;">
                                <div style="font-size:24px;margin-bottom:6px;">🎉</div>
                                <p style="font-weight:600;color:#e8f0ff;font-size:13px;margin:0 0 3px;">Thanks!</p>
                                <p style="color:#4a6080;font-size:11px;margin:0;">Closing shortly...</p>
                            </div>
                        ` : html`
                            <p style="font-weight:600;font-size:12px;color:#c9d5e8;margin:0 0 10px;">How's LiveReview working for you?</p>
                            <div style="display:flex;gap:6px;margin-bottom:10px;">
                                <button onClick=${() => handleVote('up')} style="flex:1;padding:6px;border-radius:7px;border:1px solid ${upActive ? '#22c55e' : 'rgba(255,255,255,0.1)'};background:${upActive ? 'rgba(34,197,94,0.15)' : 'rgba(255,255,255,0.04)'};color:${upActive ? '#22c55e' : '#6a7a99'};font-size:12px;cursor:pointer;transition:all 0.15s;display:flex;align-items:center;justify-content:center;gap:4px;">
                                    <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="1.5"><path d="M14 9V5a3 3 0 00-3-3l-4 9v11h11.28a2 2 0 002-1.7l1.38-9a2 2 0 00-2-2.3H14z"/><path d="M7 22H4a2 2 0 01-2-2v-7a2 2 0 012-2h3"/></svg>
                                    Helpful
                                </button>
                                <button onClick=${() => handleVote('down')} style="flex:1;padding:6px;border-radius:7px;border:1px solid ${downActive ? '#ef4444' : 'rgba(255,255,255,0.1)'};background:${downActive ? 'rgba(239,68,68,0.15)' : 'rgba(255,255,255,0.04)'};color:${downActive ? '#ef4444' : '#6a7a99'};font-size:12px;cursor:pointer;transition:all 0.15s;display:flex;align-items:center;justify-content:center;gap:4px;">
                                    <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="1.5"><path d="M10 15v4a3 3 0 003 3l4-9V2H5.72a2 2 0 00-2 1.7l-1.38 9a2 2 0 002 2.3H10z"/><path d="M17 2h2.67A2.31 2.31 0 0122 4v7a2.31 2.31 0 01-2.33 2H17"/></svg>
                                    Not helpful
                                </button>
                            </div>

                            <textarea
                                placeholder="Tell us more (optional)..."
                                onInput=${handleTextInput}
                                onFocus=${() => {}}
                                style="width:100%;box-sizing:border-box;background:rgba(255,255,255,0.04);border:1px solid rgba(255,255,255,0.09);border-radius:7px;color:#c9d5e8;font-size:12px;padding:7px 9px;resize:vertical;min-height:60px;font-family:inherit;outline:none;margin-bottom:10px;display:block;"
                                maxlength="1000"
                            ></textarea>
                            ${phase === 'error' && html`<p style="color:#ef4444;font-size:11px;margin:0 0 8px;">Something went wrong. Try again.</p>`}
                            <div style="display:flex;gap:6px;justify-content:flex-end;margin-bottom:10px;">
                                <button onClick=${close} style="padding:5px 12px;background:transparent;border:1px solid rgba(255,255,255,0.1);border-radius:5px;color:#6a7a99;font-size:11px;cursor:pointer;">Cancel</button>
                                <button onClick=${submit} style="padding:5px 12px;background:${canSubmit ? '#2d5be3' : 'rgba(45,91,227,0.25)'};border:none;border-radius:5px;color:${canSubmit ? 'white' : 'rgba(255,255,255,0.3)'};font-size:11px;font-weight:600;cursor:${canSubmit ? 'pointer' : 'not-allowed'};transition:all 0.15s;">${phase === 'submitting' ? 'Sending...' : 'Submit'}</button>
                            </div>

                            <div style="border-top:1px solid rgba(255,255,255,0.07);padding-top:9px;">
                                <${ImpactLink} />
                                ${statsExpanded && html`<${StatsGrid} />`}
                            </div>
                        `}
                    </div>
                `}

                ${linkedinOpen && html`
                    <div
                        style="position:fixed;inset:0;z-index:30000;background:rgba(0,0,0,0.72);backdrop-filter:blur(6px);display:flex;align-items:center;justify-content:center;opacity:${linkedinOpacity};transition:opacity 0.2s ease;"
                        onClick=${(e) => { if (e.target === e.currentTarget) closeLinkedin(); }}
                    >
                        <div
                            style="background:#151f2e;border:1px solid rgba(99,130,180,0.22);border-radius:16px;padding:32px;max-width:600px;width:calc(100vw - 48px);max-height:calc(100vh - 80px);overflow-y:auto;position:relative;box-shadow:0 24px 64px rgba(0,0,0,0.55);"
                            onClick=${(e) => e.stopPropagation()}
                        >
                            <button onClick=${closeLinkedin} title="Close (Esc)" style="position:absolute;top:16px;right:16px;background:rgba(255,255,255,0.07);border:1px solid rgba(255,255,255,0.12);border-radius:6px;color:#8899bb;cursor:pointer;padding:4px 9px;font-size:14px;line-height:1;">✕</button>
                            <div style="font-weight:700;font-size:17px;color:#e8f0ff;margin-bottom:4px;">Share your impact with your peers</div>
                            <div style="font-size:12px;color:#5a7aaa;margin-bottom:18px;">Edit and post on LinkedIn to showcase your engineering impact 🚀</div>
                            <textarea
                                value=${linkedinText}
                                onInput=${(e) => setLinkedinText(e.target.value)}
                                style="width:100%;box-sizing:border-box;background:rgba(255,255,255,0.04);border:1px solid rgba(255,255,255,0.1);border-radius:8px;color:#c9d5e8;font-size:13px;line-height:1.65;padding:14px 16px;min-height:300px;resize:vertical;font-family:inherit;outline:none;"
                                onClick=${(e) => e.stopPropagation()}
                            ></textarea>
                            <button
                                onClick=${handleCopyLinkedin}
                                style="margin-top:16px;padding:9px 22px;background:${snackbar ? '#22c55e' : '#2d5be3'};border:none;border-radius:8px;color:white;font-size:13px;font-weight:600;cursor:pointer;transition:background 0.2s;display:flex;align-items:center;gap:8px;"
                            >
                                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg>
                                ${snackbar ? '✓ Copied!' : 'Copy to clipboard'}
                            </button>
                        </div>
                    </div>
                `}
            </div>
        `;
    }

    // ── header ────────────────────────────────────────────────────────────────

    return function Header({ generatedTime, friendlyName }) {
        return html`
            <div class="header">
                <div class="header-top-row">
                    <div class="brand">
                        <${LogoButton} />
                        <${BrandButton} friendlyName=${friendlyName} generatedTime=${generatedTime} />
                    </div>
                    <div class="header-actions">
                        <${UsageChip} endpoint="/api/runtime/usage-chip" />
                        <${AppFeedback} />
                    </div>
                </div>
            </div>
        `;
    };
}

let HeaderComponent = null;
export async function getHeader() {
    if (!HeaderComponent) {
        HeaderComponent = await createHeader();
    }
    return HeaderComponent;
}
