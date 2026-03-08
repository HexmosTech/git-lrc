const { html } = window.preact;

export function Breadcrumbs({ route, connectorName }) {
  const crumbs = [];

  if (route.name === 'home') {
    crumbs.push({ label: 'Home' });
  }

  if (route.name === 'connectors') {
    crumbs.push({ label: 'Home', path: '/home' });
    crumbs.push({ label: 'AI Connectors' });
  }

  if (route.name === 'new') {
    crumbs.push({ label: 'Home', path: '/home' });
    crumbs.push({ label: 'AI Connectors', path: '/connectors' });
    crumbs.push({ label: 'Add Connector' });
  }

  if (route.name === 'edit') {
    crumbs.push({ label: 'Home', path: '/home' });
    crumbs.push({ label: 'AI Connectors', path: '/connectors' });
    crumbs.push({ label: connectorName || `Edit Connector #${route.connectorID || ''}`.trim() });
  }

  return html`
    <nav class="breadcrumbs" aria-label="Breadcrumb">
      ${crumbs.map((crumb, index) => html`
        <span class="crumb-wrap">
          ${crumb.path
            ? html`<a href=${`#${crumb.path}`} class="crumb-link">${crumb.label}</a>`
            : html`<span class="crumb-current">${crumb.label}</span>`}
          ${index < crumbs.length - 1 ? html`<span class="crumb-sep" aria-hidden="true">›</span>` : ''}
        </span>
      `)}
    </nav>
  `;
}
