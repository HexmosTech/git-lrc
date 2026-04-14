import { LOGO_DATA_URI } from '/static/components/utils.js';
import { dedupeIdentityLines, getDisplayName, getInitials } from '/static/ui-connectors/session-utils.js';
import { UsageChip } from '/static/components/UsageChip.js';
import { UsageBanner } from '/static/components/UsageBanner.js';

const { html, useEffect, useState } = window.preact;

export function HeaderNav({ activePath, session, reauthInProgress, orgSwitching, onReauthenticate, onSwitchOrg }) {
  const homeActive = activePath === '/home';
  const connectorsActive = activePath.startsWith('/connectors');
  const authenticated = Boolean(session && session.authenticated);
  const displayName = getDisplayName(session);
  const sessionHint = session && session.user_email ? session.user_email : (session && session.message ? session.message : 'Sign in required');
  const orgLabel = (session && session.org_name) || (session && session.org_id ? `Org #${session.org_id}` : 'No organization');
  const avatarURL = session && session.avatar_url ? session.avatar_url : '';
  const initials = getInitials(session);
  const [avatarFailed, setAvatarFailed] = useState(false);
  const identityLines = dedupeIdentityLines([displayName, sessionHint, orgLabel]);
  const primaryLine = identityLines[0] || displayName;
  const secondaryLine = identityLines[1] || '';
  const tertiaryLine = identityLines[2] || '';
  const profileTitle = identityLines.length > 0 ? identityLines.join(' | ') : 'Profile';
  const organizations = Array.isArray(session && session.organizations) ? session.organizations : [];
  const selectedOrgID = String((session && session.org_id) || '');

  useEffect(() => {
    setAvatarFailed(false);
  }, [avatarURL]);

  return html`
    <div>
      <div class="header">
        <div class="brand">
          <div class="logo-wrap">
            <img alt="git-lrc" src=${LOGO_DATA_URI} />
          </div>
          <div class="brand-text">
            <h1>git-lrc</h1>
            <div class="meta">Manage your git-lrc</div>
          </div>
        </div>

        <div class="header-right">
          ${authenticated ? html`
            <div class="header-right-stack">
              <div class="org-context-switcher">
                <label class="org-context-label" for="org-context-select">Org</label>
                <select
                  id="org-context-select"
                  class="org-context-select"
                  value=${selectedOrgID}
                  disabled=${orgSwitching || organizations.length === 0}
                  onChange=${(event) => onSwitchOrg && onSwitchOrg(event.target.value)}
                  title=${organizations.length === 0 ? 'No organizations available' : 'Switch organization context'}
                >
                  ${organizations.length === 0
                    ? html`<option value="">No organizations</option>`
                    : html`
                      ${selectedOrgID ? '' : html`<option value="">Select organization</option>`}
                      ${organizations.map((org) => html`
                        <option key=${String(org.id)} value=${String(org.id)}>${org.name}</option>
                      `)}
                    `}
                </select>
              </div>

              <${UsageChip} key=${`usage-${selectedOrgID || 'none'}`} endpoint="/api/ui/usage-chip" />
              <a class="profile-chip" href="#/profile" title=${profileTitle}>
                ${avatarURL && !avatarFailed
                  ? html`<img class="profile-chip-avatar" src=${avatarURL} alt=${displayName} onError=${() => setAvatarFailed(true)} />`
                  : html`<div class="profile-chip-avatar profile-chip-fallback">${initials}</div>`}
                <div class="profile-chip-text">
                  <div class="profile-chip-name">${primaryLine}</div>
                  ${secondaryLine ? html`<div class="profile-chip-meta">${secondaryLine}</div>` : ''}
                  ${tertiaryLine ? html`<div class="profile-chip-org">${tertiaryLine}</div>` : ''}
                </div>
              </a>
            </div>
          ` : html`
            <div class="header-auth-actions">
              <${UsageChip} endpoint="/api/ui/usage-chip" />
              <div class="session-pill session-bad" title=${sessionHint}>Not Authenticated</div>
              <button class="secondary" disabled=${reauthInProgress} onClick=${onReauthenticate}>
                ${reauthInProgress ? 'Signing in...' : 'Sign in'}
              </button>
            </div>
          `}
        </div>
      </div>

      <${UsageBanner} endpoint="/api/ui/usage-chip" />

      <nav class="ui-nav" aria-label="git-lrc manager navigation">
        <span class="nav-label">Menu</span>
        <a href="#/home" class=${`nav-link ${homeActive ? 'active' : ''}`}>Home</a>
        <a href="#/connectors" class=${`nav-link ${connectorsActive ? 'active' : ''}`}>AI Connectors</a>
      </nav>
    </div>
  `;
}
