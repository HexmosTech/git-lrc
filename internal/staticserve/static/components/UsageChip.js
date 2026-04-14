import { formatResetAt, normalizeUsagePayload, planLabel, usageTone } from '/static/components/usage_chip_model.mjs';

const { html, useEffect, useMemo, useRef, useState } = window.preact;

const DEFAULT_REFRESH_MS = 5 * 60 * 1000;

async function fetchUsagePayload(endpoint) {
  const response = await fetch(endpoint, {
    headers: { 'Content-Type': 'application/json' },
  });
  if (!response.ok) {
    let parsed = {};
    const contentType = String(response.headers.get('content-type') || '').toLowerCase();
    const contentLength = Number(response.headers.get('content-length') || '0');

    if (contentType.includes('application/json')) {
      try {
        parsed = await response.json();
      } catch {
        parsed = {};
      }
    } else if (!Number.isNaN(contentLength) && contentLength > 0 && contentLength <= 2048) {
      try {
        const text = await response.text();
        if (text) {
          parsed = { message: text };
        }
      } catch {
        parsed = {};
      }
    }

    const reason = String(parsed.error || parsed.message || `request failed (${response.status})`).trim();
    return normalizeUsagePayload({ available: false, unavailable_reason: reason }, reason);
  }

  const text = await response.text();
  let parsed = {};
  if (text) {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = { unavailable_reason: text };
    }
  }

  return normalizeUsagePayload(parsed);
}

export function UsageChip({ endpoint, refreshMs = DEFAULT_REFRESH_MS }) {
  const [chip, setChip] = useState(() => normalizeUsagePayload({ available: false, unavailable_reason: 'Loading usage data...' }));
  const [loading, setLoading] = useState(true);
  const [isOpen, setIsOpen] = useState(false);
  const closeTimerRef = useRef(null);

  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      setLoading(true);
      try {
        const payload = await fetchUsagePayload(endpoint);
        if (!cancelled) {
          setChip(payload);
        }
      } catch (error) {
        if (!cancelled) {
          const reason = String(error?.message || 'Usage data unavailable right now.').trim();
          setChip(normalizeUsagePayload({ available: false, unavailable_reason: reason }, reason));
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    load();
    const intervalID = setInterval(load, refreshMs);

    return () => {
      cancelled = true;
      clearInterval(intervalID);
      if (closeTimerRef.current) {
        clearTimeout(closeTimerRef.current);
        closeTimerRef.current = null;
      }
    };
  }, [endpoint, refreshMs]);

  const tone = useMemo(() => usageTone(chip, loading), [chip, loading]);

  const openPopup = () => {
    if (closeTimerRef.current) {
      clearTimeout(closeTimerRef.current);
      closeTimerRef.current = null;
    }
    setIsOpen(true);
  };

  const closePopupSoon = () => {
    if (closeTimerRef.current) {
      clearTimeout(closeTimerRef.current);
    }
    closeTimerRef.current = setTimeout(() => {
      setIsOpen(false);
      closeTimerRef.current = null;
    }, 220);
  };

  const buttonLabel = loading
    ? 'Usage...'
    : chip.available
      ? `${planLabel(chip.planCode)} ${chip.usagePct}%`
      : 'Usage unavailable';

  const title = chip.available
    ? 'Open billing and usage details'
    : chip.unavailableReason || 'Usage data unavailable right now.';

  return html`
    <div class="usage-chip-wrap" onMouseEnter=${openPopup} onMouseLeave=${closePopupSoon}>
      <button
        class=${`usage-chip-button usage-chip-tone-${tone}`}
        title=${title}
        onFocus=${openPopup}
        onBlur=${closePopupSoon}
        type="button"
      >
        ${buttonLabel}
      </button>

      ${isOpen && html`
        <div class="usage-chip-popover" onMouseEnter=${openPopup} onMouseLeave=${closePopupSoon}>
          ${loading
            ? html`
              <p class="usage-chip-title">Loading usage details...</p>
              <p class="usage-chip-help">Please wait while git-lrc fetches the latest organization billing usage.</p>
            `
            : chip.available
              ? html`
                <p class="usage-chip-title">Billing Usage Detail</p>
                <p class="usage-chip-help">
                  Scope: organization usage in current billing period. Attribution is charged to the triggering actor.
                </p>

                ${''}

                <div class="usage-chip-reset-card">
                  <p class="usage-chip-reset-title">Usage resets on ${formatResetAt(chip.resetAt)}</p>
                  <p class="usage-chip-reset-sub">Local timezone. New cycle usage starts immediately after this time.</p>
                </div>

                <div class="usage-chip-grid">
                  <div class="usage-chip-cell">
                    <p class="usage-chip-cell-label">Plan</p>
                    <p class="usage-chip-cell-value">${planLabel(chip.planCode)}</p>
                  </div>
                  <div class="usage-chip-cell">
                    <p class="usage-chip-cell-label">Org Usage</p>
                    <p class="usage-chip-cell-value">${chip.locUsed.toLocaleString()} / ${chip.locLimit > 0 ? chip.locLimit.toLocaleString() : 'Unlimited'} LOC</p>
                  </div>
                  <div class="usage-chip-cell">
                    <p class="usage-chip-cell-label">My Usage</p>
                    <p class="usage-chip-cell-value">${chip.myUsageLOC.toLocaleString()} LOC</p>
                  </div>
                  <div class="usage-chip-cell">
                    <p class="usage-chip-cell-label">My Activity Share</p>
                    ${chip.myOperationCount === 0 && chip.myUsageLOC === 0
                      ? html`<p class="usage-chip-cell-muted">No billable activity this cycle.</p>`
                      : html`
                        <p class="usage-chip-cell-value">${chip.myOperationCount.toLocaleString()} operations</p>
                        <p class="usage-chip-cell-sub">${chip.mySharePct.toFixed(1)}% of org usage</p>
                      `}
                  </div>
                </div>

                <p class="usage-chip-footnote">Operations are billable actions. Share is your LOC contribution percentage out of org usage.</p>

                ${chip.canViewTeamBreakdown && chip.topMembers.length > 0
                  ? html`
                    <div class="usage-chip-members">
                      <p class="usage-chip-members-title">Top Contributors</p>
                      <div class="usage-chip-members-list">
                        ${chip.topMembers.map((member) => html`
                          <div class="usage-chip-members-row" key=${`${member.label}-${member.kind}`}>
                            <span>${member.label || 'Unknown'}</span>
                            <span>${member.loc.toLocaleString()} LOC (${member.share.toFixed(1)}%)</span>
                          </div>
                        `)}
                      </div>
                    </div>
                  `
                  : ''}
              `
              : html`
                <p class="usage-chip-title">Usage unavailable</p>
                <p class="usage-chip-help">${chip.unavailableReason || 'Usage data unavailable right now.'}</p>
                <p class="usage-chip-footnote">Run lrc ui and re-authenticate to refresh session credentials.</p>
              `}
        </div>
      `}
    </div>
  `;
}
