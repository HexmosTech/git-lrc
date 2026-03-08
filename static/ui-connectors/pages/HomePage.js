import { fetchGitHubReleases } from '/static/ui-connectors/api.js';

const { html, useEffect, useState } = window.preact;

function formatDate(iso) {
  if (!iso) return '';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleDateString();
}

export function HomePage() {
  const [releases, setReleases] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;

    async function loadReleases() {
      setLoading(true);
      setError('');
      try {
        const data = await fetchGitHubReleases();
        if (!cancelled) {
          setReleases(data);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err.message || String(err));
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    loadReleases();
    return () => {
      cancelled = true;
    };
  }, []);

  return html`
    <div class="single">
      <section class="card">
        <h2>Home</h2>
        <div class="sub">Quick links and project updates.</div>

        <div class="home-grid">
          <div class="home-card">
            <h3>GitHub Repository</h3>
            <p>Give the project a ⭐ if it helps your workflow.</p>
            <a class="home-link" href="https://github.com/HexmosTech/git-lrc" target="_blank" rel="noopener noreferrer">
              Open github.com/HexmosTech/git-lrc
            </a>
          </div>

          <div class="home-card">
            <h3>Learn More</h3>
            <p>Get latest product information and usage details.</p>
            <a class="home-link" href="https://hexmos.com/livereview/git-lrc/" target="_blank" rel="noopener noreferrer">
              Open hexmos.com/livereview/git-lrc/
            </a>
          </div>
        </div>
      </section>

      <section class="card">
        <h2>Latest Releases</h2>
        <div class="sub">Fetched from GitHub releases API.</div>

        ${loading ? html`<div class="page-empty">Loading releases...</div>` : ''}
        ${error ? html`<div class="err-banner">${error}</div>` : ''}

        ${!loading && !error && releases.length === 0 ? html`<div class="page-empty">No releases available yet.</div>` : ''}

        ${!loading && !error && releases.length > 0 ? html`
          <div class="release-list">
            ${releases.map((release) => html`
              <div class="release-item">
                <div class="release-main">
                  <a class="home-link" href=${release.html_url} target="_blank" rel="noopener noreferrer">
                    ${release.name || release.tag_name || 'Untitled release'}
                  </a>
                  <span class="badge">${release.tag_name || 'untagged'}</span>
                </div>
                <div class="muted">Published ${formatDate(release.published_at)}</div>
              </div>
            `)}
          </div>
        ` : ''}
      </section>
    </div>
  `;
}
