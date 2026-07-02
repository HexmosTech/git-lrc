// SendToAgentButton — split button + dropdown for handing off visible issues to an AI coding agent.
import { waitForPreact } from './utils.js';
import { renderIcon } from './icons.js';

export const SEND_TO_AGENT_AGENTS = Object.freeze([
    {
        key: 'claude',
        label: 'Claude',
        // White wordmark — reads cleanly straight on the blue button/dark menu.
        logo: '/static/assets/claude-logo-white.svg',
        // Full-color mark for use on light surfaces (e.g. the handoff popup badge).
        logoOnLight: '/static/assets/claude-logo.svg',
        available: true,
    },
    {
        key: 'codex',
        label: 'Codex',
        logo: '/static/assets/codex-logo-white.svg',
        available: false,
    },
    {
        key: 'gemini',
        label: 'Gemini CLI',
        // Icon-only mark — no wordmark baked in, so the menu label is rendered alongside it.
        logo: '/static/assets/gemini-icon.png',
        showLabel: true,
        available: false,
    },
]);

export function getSendToAgentInfo(agentKey) {
    return SEND_TO_AGENT_AGENTS.find((agent) => agent.key === agentKey) || SEND_TO_AGENT_AGENTS[0];
}

export async function createSendToAgentButton() {
    const { html, useState, useRef, useEffect } = await waitForPreact();

    return function SendToAgentButton({ visibleCount, onSendToAgent }) {
        const [isOpen, setIsOpen] = useState(false);
        const [isLaunching, setIsLaunching] = useState(false);
        const wrapperRef = useRef(null);
        const launchTimerRef = useRef(null);

        useEffect(() => () => {
            if (launchTimerRef.current) clearTimeout(launchTimerRef.current);
        }, []);

        useEffect(() => {
            if (!isOpen) return;
            const handlePointerDown = (e) => {
                if (wrapperRef.current && !wrapperRef.current.contains(e.target)) {
                    setIsOpen(false);
                }
            };
            const handleKeyDown = (e) => {
                if (e.key === 'Escape') setIsOpen(false);
            };
            document.addEventListener('mousedown', handlePointerDown);
            document.addEventListener('keydown', handleKeyDown);
            return () => {
                document.removeEventListener('mousedown', handlePointerDown);
                document.removeEventListener('keydown', handleKeyDown);
            };
        }, [isOpen]);

        const primaryAgent = SEND_TO_AGENT_AGENTS.find((agent) => agent.available) || SEND_TO_AGENT_AGENTS[0];

        const selectAgent = (agentKey) => {
            setIsOpen(false);
            onSendToAgent(agentKey);
        };

        const handlePrimaryClick = () => {
            setIsLaunching(true);
            if (launchTimerRef.current) clearTimeout(launchTimerRef.current);
            launchTimerRef.current = setTimeout(() => setIsLaunching(false), 500);
            selectAgent(primaryAgent.key);
        };

        return html`
            <div ref=${wrapperRef} class="send-to-agent-group">
                <button
                    class="btn btn-primary send-to-agent-btn ${isLaunching ? 'launching' : ''}"
                    onClick=${handlePrimaryClick}
                    title="Send visible issues to ${primaryAgent.label}"
                >
                    <span class="send-to-agent-label">Send to</span>
                    <img class="send-to-agent-logo" src=${primaryAgent.logo} alt="${primaryAgent.label}" />
                    <span class="send-to-agent-count">(${visibleCount})</span>
                </button>
                <button
                    class="btn btn-primary send-to-agent-toggle ${isOpen ? 'open' : ''}"
                    onClick=${() => setIsOpen((prev) => !prev)}
                    aria-expanded=${isOpen ? 'true' : 'false'}
                    aria-label="Choose an AI agent"
                    title="Choose an AI agent"
                >
                    ${renderIcon(html, isOpen ? 'dropdownOpen' : 'dropdownClosed')}
                </button>
                <div class="send-to-agent-menu ${isOpen ? 'open' : ''}" role="menu">
                    ${SEND_TO_AGENT_AGENTS.map((agent) => html`
                        <button
                            class="send-to-agent-menu-item ${agent.available ? '' : 'unavailable'}"
                            role="menuitem"
                            onClick=${() => selectAgent(agent.key)}
                            title="${agent.available ? `Send visible issues to ${agent.label}` : `${agent.label} — not yet implemented`}"
                        >
                            <span class="send-to-agent-menu-item-row">
                                <span class="send-to-agent-menu-item-icon send-to-agent-menu-item-icon-${agent.key}">
                                    ${agent.logo
                                        ? html`<img class="send-to-agent-logo" src=${agent.logo} alt="${agent.label}" />`
                                        : renderIcon(html, agent.icon, { size: 14 })}
                                </span>
                                ${agent.showLabel && html`<span class="send-to-agent-menu-item-label">${agent.label}</span>`}
                            </span>
                            ${!agent.available && html`<span class="send-to-agent-menu-item-tag">Not yet implemented</span>`}
                        </button>
                    `)}
                </div>
            </div>
        `;
    };
}

let SendToAgentButtonComponent = null;
export async function getSendToAgentButton() {
    if (!SendToAgentButtonComponent) {
        SendToAgentButtonComponent = await createSendToAgentButton();
    }
    return SendToAgentButtonComponent;
}
