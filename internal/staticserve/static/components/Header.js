// Header component
import { renderIcon } from './icons.js';
import { waitForPreact, LOGO_DATA_URI } from './utils.js';
import { UsageChip } from '/static/components/UsageChip.js';
import { fetchImpactStats, buildLinkedinText } from '/static/components/FeedbackPopup.js';
import { getReviewMeta } from '/static/components/reviewMeta.mjs';

const SESSION_REVIEW_ID = new URLSearchParams(window.location.search).get('r') || '';

const GITHUB_URL = 'https://github.com/HexmosTech/git-lrc';
const LIVEREVIEW_URL = 'https://hexmos.com/livereview/';
const REPOSITORY_PATH_MAX_WIDTH_VIEWPORT = '56vw';
const REPOSITORY_PATH_MAX_WIDTH_PIXELS = '780px';
const REPOSITORY_PATH_MAX_WIDTH = `min(${REPOSITORY_PATH_MAX_WIDTH_VIEWPORT}, ${REPOSITORY_PATH_MAX_WIDTH_PIXELS})`;

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
                                ${renderIcon(html, 'external', { size: 13 })}
                                Star on GitHub
                            </a>
                        </${Popover}>
                    </div>
                `}
            </div>
        `;
    }

    // ── brand text click popup ────────────────────────────────────────────────

    function BrandButton({ friendlyName, generatedTime, repositoryPath }) {
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
                ${repositoryPath && html`
                    <div
                        title=${repositoryPath}
                        style=${`margin-top:4px;max-width:${REPOSITORY_PATH_MAX_WIDTH};color:#5d708d;font-size:10px;line-height:1.35;font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,Liberation Mono,Courier New,monospace;word-break:break-all;`}
                    >
                        Repo: ${repositoryPath}
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
                                        ${renderIcon(html, 'external', { className: 'btn-icon', size: 11 })}
                                    hexmos.com/livereview
                                </a>
                                <button
                                    onClick=${copyURL}
                                    style="flex-shrink:0;padding:7px 9px;background:rgba(255,255,255,0.06);border:1px solid rgba(255,255,255,0.1);border-radius:7px;color:${copied ? '#4ade80' : '#6a7a99'};font-size:11px;cursor:pointer;transition:all 0.15s;white-space:nowrap;"
                                    title="Copy link"
                                >${renderIcon(html, copied ? 'copied' : 'copyLogs', { className: 'btn-icon', size: 11 })}${copied ? 'Copied' : 'Copy'}</button>
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
            ? html`<div style="font-size:12px;color:#4a5a6a;padding:4px 0;user-select:none;display:flex;align-items:center;gap:5px;">${renderIcon(html, 'aiAssist', { size: 12 })}Want to see your impact stats?</div>`
            : html`<div style="display:flex;align-items:center;gap:5px;color:#7aadff;cursor:pointer;font-size:12px;font-weight:500;padding:4px 0;user-select:none;" onMouseEnter=${() => setStatsExpanded(true)}>
                ${renderIcon(html, 'aiAssist', { size: 12 })}
                Want to see your impact stats?
                ${renderIcon(html, 'next', { size: 11 })}
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
                            ${renderIcon(html, 'aiAssist', { size: 12 })}
                            <span>Stand out by showing your impact stats to your peers</span>
                            ${renderIcon(html, 'next', { size: 12 })}
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
                    ${renderIcon(html, 'feedback', { size: 12 })}
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
                                <div style="margin-bottom:6px;display:flex;justify-content:center;">${renderIcon(html, 'successStatus', { size: 24 })}</div>
                                <p style="font-weight:600;color:#e8f0ff;font-size:13px;margin:0 0 3px;">Thanks!</p>
                                <p style="color:#4a6080;font-size:11px;margin:0;">Closing shortly...</p>
                            </div>
                        ` : html`
                            <p style="font-weight:600;font-size:12px;color:#c9d5e8;margin:0 0 10px;">How's LiveReview working for you?</p>
                            <div style="display:flex;gap:6px;margin-bottom:10px;">
                                <button onClick=${() => handleVote('up')} style="flex:1;padding:6px;border-radius:7px;border:1px solid ${upActive ? '#22c55e' : 'rgba(255,255,255,0.1)'};background:${upActive ? 'rgba(34,197,94,0.15)' : 'rgba(255,255,255,0.04)'};color:${upActive ? '#22c55e' : '#6a7a99'};font-size:12px;cursor:pointer;transition:all 0.15s;display:flex;align-items:center;justify-content:center;gap:4px;">
                                    ${renderIcon(html, 'helpful', { size: 12 })}
                                    Helpful
                                </button>
                                <button onClick=${() => handleVote('down')} style="flex:1;padding:6px;border-radius:7px;border:1px solid ${downActive ? '#ef4444' : 'rgba(255,255,255,0.1)'};background:${downActive ? 'rgba(239,68,68,0.15)' : 'rgba(255,255,255,0.04)'};color:${downActive ? '#ef4444' : '#6a7a99'};font-size:12px;cursor:pointer;transition:all 0.15s;display:flex;align-items:center;justify-content:center;gap:4px;">
                                    ${renderIcon(html, 'notHelpful', { size: 12 })}
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
                            <button onClick=${closeLinkedin} title="Close (Esc)" style="position:absolute;top:16px;right:16px;background:rgba(255,255,255,0.07);border:1px solid rgba(255,255,255,0.12);border-radius:6px;color:#8899bb;cursor:pointer;padding:4px 9px;font-size:14px;line-height:1;">${renderIcon(html, 'close', { size: 14 })}</button>
                            <div style="font-weight:700;font-size:17px;color:#e8f0ff;margin-bottom:4px;">Share your impact with your peers</div>
                            <div style="font-size:12px;color:#5a7aaa;margin-bottom:18px;">Edit and post on LinkedIn to showcase your engineering impact.</div>
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
                                ${renderIcon(html, snackbar ? 'copied' : 'copyLogs', { size: 14 })}
                                ${snackbar ? 'Copied!' : 'Copy to clipboard'}
                            </button>
                        </div>
                    </div>
                `}
            </div>
        `;
    }

    // ── blast-radius upload chip ─────────────────────────────────────────────
    // Persistent status chip for the blast-radius report's upload to
    // LiveReview (see internal/appcore/blastradius_bridge.go's
    // uploadBlastRadiusReport, and GET /api/blastradius's "upload" field).
    // Unlike a transient banner, this never disappears once blast-radius has
    // something to report - it just changes tone/icon as the upload moves
    // through idle -> uploading -> uploaded/failed, mirroring UsageChip's
    // hover-for-details pattern right next to it.

    function formatBytes(bytes) {
        if (!bytes || bytes <= 0) return '0 B';
        const units = ['B', 'KB', 'MB', 'GB'];
        let value = bytes;
        let unitIndex = 0;
        while (value >= 1024 && unitIndex < units.length - 1) {
            value /= 1024;
            unitIndex += 1;
        }
        return `${value.toFixed(unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
    }

    function formatDuration(ms) {
        if (!ms || ms <= 0) return '0 ms';
        if (ms < 1000) return `${ms} ms`;
        return `${(ms / 1000).toFixed(1)} s`;
    }

    const BLAST_CHIP_CONFIG = {
        idle: { icon: 'upload', tone: 'idle', label: 'Not uploaded yet', spin: false },
        uploading: { icon: 'upload', tone: 'pending', label: 'Uploading…', spin: true },
        uploaded: { icon: 'check', tone: 'ok', label: 'Uploaded to LiveReview', spin: false },
        failed: { icon: 'issueWarning', tone: 'critical', label: 'Upload failed', spin: false },
    };

    function BlastUploadChip({ blastRadius }) {
        const { isOpen, open, closeSoon } = useHoverPopover();

        const upload = blastRadius && blastRadius.upload;
        const scoringKnown = blastRadius && blastRadius.status && blastRadius.status !== 'unavailable';
        const uploadStarted = upload && upload.status && upload.status !== 'idle';
        if (!scoringKnown && !uploadStarted) return '';

        const status = (upload && upload.status) || 'idle';
        const cfg = BLAST_CHIP_CONFIG[status] || BLAST_CHIP_CONFIG.idle;

        return html`
            <div class="blast-chip-wrap" onMouseEnter=${open} onMouseLeave=${closeSoon}>
                <button
                    class=${`blast-chip-button blast-chip-tone-${cfg.tone}`}
                    title=${cfg.label}
                    onFocus=${open}
                    onBlur=${closeSoon}
                    type="button"
                >
                    <span class=${cfg.spin ? 'blast-chip-icon blast-chip-icon-spin' : 'blast-chip-icon'}>
                        ${renderIcon(html, cfg.icon, { size: 13 })}
                    </span>
                    Blast Radius
                </button>

                ${isOpen && html`
                    <div class="blast-chip-popover" onMouseEnter=${open} onMouseLeave=${closeSoon}>
                        <p class="blast-chip-title">Blast-Radius Upload</p>
                        <p class="blast-chip-help">${cfg.label}</p>

                        ${(status === 'uploaded' || status === 'failed') && html`
                            <div class="blast-chip-grid">
                                <div class="blast-chip-cell">
                                    <p class="blast-chip-cell-label">Size</p>
                                    <p class="blast-chip-cell-value">${formatBytes(upload.size_bytes)}</p>
                                </div>
                                <div class="blast-chip-cell">
                                    <p class="blast-chip-cell-label">Transfer time</p>
                                    <p class="blast-chip-cell-value">${formatDuration(upload.duration_ms)}</p>
                                </div>
                            </div>
                        `}

                        ${status === 'failed' && upload.error && html`
                            <p class="blast-chip-error">${upload.error}</p>
                        `}
                    </div>
                `}
            </div>
        `;
    }

    // ── header ────────────────────────────────────────────────────────────────

    return function Header({ generatedTime, friendlyName, repositoryPath, onToggleSidebar, blastRadius }) {
        return html`
            <div class="header">
                <div class="header-top-row">
                    <div class="brand">
                        <${LogoButton} />
                        <${BrandButton} friendlyName=${friendlyName} generatedTime=${generatedTime} repositoryPath=${repositoryPath} />
                    </div>
                    <div class="header-actions">
                        ${onToggleSidebar && html`
                            <button
                                class="sidebar-toggle"
                                onClick=${onToggleSidebar}
                                title="Files"
                                aria-label="Toggle file list"
                                style="align-items:center;justify-content:center;width:32px;height:32px;border-radius:6px;background:var(--bg-tertiary);border:1px solid var(--border-medium);color:var(--text-primary);cursor:pointer;"
                            >
                                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                    <line x1="4" y1="6" x2="20" y2="6"/>
                                    <line x1="4" y1="12" x2="20" y2="12"/>
                                    <line x1="4" y1="18" x2="20" y2="18"/>
                                </svg>
                            </button>
                        `}
                        <${BlastUploadChip} blastRadius=${blastRadius} />
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
