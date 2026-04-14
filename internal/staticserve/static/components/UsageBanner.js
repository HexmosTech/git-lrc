import { normalizeUsagePayload } from '/static/components/usage_chip_model.mjs';

const { html, useEffect, useState } = window.preact;

export function UsageBanner({ endpoint }) {
    const [chip, setChip] = useState(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        let cancelled = false;
        const load = async () => {
            try {
                const response = await fetch(endpoint);
                if (!response.ok) return;
                const data = await response.json();
                if (!cancelled) {
                    setChip(normalizeUsagePayload(data));
                }
            } catch (err) {
                console.error('Failed to fetch usage for banner:', err);
            } finally {
                if (!cancelled) setLoading(false);
            }
        };
        load();
        return () => { cancelled = true; };
    }, [endpoint]);

    if (loading || !chip || !chip.available) return '';

    const upgradeURL = `${chip.cloudURL}/#/settings-subscriptions-overview`;

    if (chip.blocked || chip.usagePct >= 100) {
        const limitStr = chip.locLimit > 0 ? chip.locLimit.toLocaleString() : 'N/A';

        return html`
            <div class="quota-banner-slate">
                <div class="qbs-flex">
                    <div class="qbs-icon-wrap">
                        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                            <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path>
                            <line x1="12" y1="9" x2="12" y2="13"></line>
                            <line x1="12" y1="17" x2="12.01" y2="17"></line>
                        </svg>
                    </div>
                    <div class="qbs-content">
                        <p class="qbs-title">You've reached your monthly limit</p>
                        <p class="qbs-text">
                            Your team used all <strong>${limitStr} LOC</strong> this month. 
                            Upgrade to a higher tier and continue reviewing code without any interruption to your workflow.
                        </p>
                        <a href="${upgradeURL}" target="_blank" class="qbs-btn">
                            Upgrade plan
                        </a>
                    </div>
                </div>
            </div>
        `;
    }

    if (chip.usagePct >= 90) {
        return html`
            <div class="main-alert main-alert-warn">
                <div class="main-alert-content">
                    <div class="main-alert-text">
                        <span class="main-alert-title">⚠️ LOC Usage Nearing Limit</span>
                        <span class="main-alert-sub">
                            You've used ${chip.locUsed.toLocaleString()} of ${chip.locLimit > 0 ? chip.locLimit.toLocaleString() : 'N/A'} LOC (${chip.usagePct}%). Upgrade to avoid interruption.
                        </span>
                    </div>
                    <a href="${upgradeURL}" target="_blank" class="main-alert-btn main-alert-btn-warn">
                        Upgrade Plan
                    </a>
                </div>
            </div>
        `;
    }

    return '';
}
