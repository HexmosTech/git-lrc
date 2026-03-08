import { LOGO_DATA_URI } from '/static/components/utils.js';

const { html } = window.preact;

export function HeaderNav({ activePath }) {
  const homeActive = activePath === '/home';
  const connectorsActive = activePath.startsWith('/connectors');

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
      </div>

      <nav class="ui-nav" aria-label="git-lrc manager navigation">
        <span class="nav-label">Menu</span>
        <a href="#/home" class=${`nav-link ${homeActive ? 'active' : ''}`}>Home</a>
        <a href="#/connectors" class=${`nav-link ${connectorsActive ? 'active' : ''}`}>AI Connectors</a>
      </nav>
    </div>
  `;
}
