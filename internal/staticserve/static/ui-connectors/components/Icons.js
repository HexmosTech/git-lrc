// Inline-SVG icon set for the git-lrc manager UI.
//
// These replace the inconsistent unicode glyphs (↻ ⇅ ＋ ↑ ↓ ✎ 🗑) that were
// previously used inside `.btn-icon` spans. Paths follow the Lucide style
// (24x24 viewBox, 2px round strokes, `currentColor`) so icons inherit the
// button's text color and stay crisp at any size. No external dependency —
// the markup is rendered directly through Preact + htm.

const { html } = window.preact;

// Each entry is the inner markup of a 0 0 24 24 stroke icon.
const PATHS = {
  refresh: html`
    <path d="M21 12a9 9 0 1 1-2.64-6.36" />
    <polyline points="21 3 21 9 15 9" />
  `,
  priority: html`
    <path d="M7 21V7" />
    <polyline points="3 11 7 7 11 11" />
    <path d="M17 3v14" />
    <polyline points="13 13 17 17 21 13" />
  `,
  plus: html`
    <line x1="12" y1="5" x2="12" y2="19" />
    <line x1="5" y1="12" x2="19" y2="12" />
  `,
  grip: html`
    <circle cx="9" cy="6" r="1.4" />
    <circle cx="15" cy="6" r="1.4" />
    <circle cx="9" cy="12" r="1.4" />
    <circle cx="15" cy="12" r="1.4" />
    <circle cx="9" cy="18" r="1.4" />
    <circle cx="15" cy="18" r="1.4" />
  `,
  'arrow-up': html`
    <line x1="12" y1="19" x2="12" y2="5" />
    <polyline points="6 11 12 5 18 11" />
  `,
  'arrow-down': html`
    <line x1="12" y1="5" x2="12" y2="19" />
    <polyline points="6 13 12 19 18 13" />
  `,
  edit: html`
    <path d="M12 20h9" />
    <path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z" />
  `,
  trash: html`
    <polyline points="3 6 5 6 21 6" />
    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
    <line x1="10" y1="11" x2="10" y2="17" />
    <line x1="14" y1="11" x2="14" y2="17" />
  `,
  external: html`
    <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
    <polyline points="15 3 21 3 21 9" />
    <line x1="10" y1="14" x2="21" y2="3" />
  `,
  star: html`
    <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
  `,
  cpu: html`
    <rect x="6" y="6" width="12" height="12" rx="2" />
    <path d="M9 2v3M15 2v3M9 19v3M15 19v3M2 9h3M2 15h3M19 9h3M19 15h3" />
  `,
  github: html`
    <path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22" />
  `,
  book: html`
    <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20" />
    <path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z" />
  `,
  clock: html`
    <circle cx="12" cy="12" r="9" />
    <polyline points="12 7 12 12 15 14" />
  `,
};

// Icon renders a named glyph. `size` controls the box; stroke width scales
// with it so small icons stay legible. Decorative by default (aria-hidden);
// pass a `label` to expose it to assistive tech.
export function Icon({ name, size = 16, strokeWidth, label, class: cls = '' }) {
  const inner = PATHS[name];
  if (!inner) return null;
  const sw = strokeWidth || (size <= 16 ? 2 : 1.75);
  const fillIcons = name === 'star';
  return html`
    <svg
      class=${`lrc-icon ${cls}`.trim()}
      width=${size}
      height=${size}
      viewBox="0 0 24 24"
      fill=${fillIcons ? 'currentColor' : 'none'}
      stroke="currentColor"
      stroke-width=${sw}
      stroke-linecap="round"
      stroke-linejoin="round"
      role=${label ? 'img' : undefined}
      aria-label=${label || undefined}
      aria-hidden=${label ? undefined : 'true'}
      focusable="false"
    >${inner}</svg>
  `;
}

export const ICON_NAMES = Object.keys(PATHS);
