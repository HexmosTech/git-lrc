export function parseRoute(hashValue) {
  const cleaned = (hashValue || '').replace(/^#/, '').trim();
  const path = cleaned === '' ? '/home' : cleaned;
  const parts = path.split('/').filter(Boolean);

  if (parts[0] === 'connectors') {
    if (parts[1] === 'new') {
      return { name: 'new' };
    }
    if (parts[1] === 'edit' && parts[2]) {
      return { name: 'edit', connectorID: parts[2] };
    }
    return { name: 'connectors' };
  }

  return { name: 'home' };
}

export function routePath(route) {
  if (route.name === 'connectors') return '/connectors';
  if (route.name === 'new') return '/connectors/new';
  if (route.name === 'edit' && route.connectorID) return `/connectors/edit/${route.connectorID}`;
  return '/home';
}

export function navigate(path) {
  window.location.hash = path;
}
