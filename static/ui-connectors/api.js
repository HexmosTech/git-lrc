export async function api(path, options = {}) {
  const resp = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  });

  const text = await resp.text();
  let data;
  try {
    data = text ? JSON.parse(text) : {};
  } catch {
    data = { error: text || `request failed (${resp.status})` };
  }

  if (!resp.ok) {
    throw new Error(data.error || data.message || `request failed (${resp.status})`);
  }

  return data;
}

export async function fetchGitHubReleases() {
  const resp = await fetch('https://api.github.com/repos/HexmosTech/git-lrc/releases?per_page=5', {
    headers: {
      'Accept': 'application/vnd.github+json',
    },
  });

  if (!resp.ok) {
    throw new Error(`GitHub releases request failed (${resp.status})`);
  }

  const data = await resp.json();
  if (!Array.isArray(data)) {
    return [];
  }

  return data;
}
