// Global light/dark theme controller — shared by every git-lrc web route
// (the review report viewer and the connectors manager).
//
// The actual `data-theme` attribute is set by a tiny inline script in each
// page's <head> BEFORE first paint (so there's no flash of the wrong theme).
// This module owns persistence, the system-preference default, and the
// floating sun/moon toggle button that lets the user switch at runtime.

const STORAGE_KEY = 'lrc-theme';

// Lucide-style icons (stroke = currentColor so they inherit the button color).
const SUN = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><circle cx="12" cy="12" r="4"/><line x1="12" y1="2" x2="12" y2="5"/><line x1="12" y1="19" x2="12" y2="22"/><line x1="2" y1="12" x2="5" y2="12"/><line x1="19" y1="12" x2="22" y2="12"/><line x1="4.9" y1="4.9" x2="7" y2="7"/><line x1="17" y1="17" x2="19.1" y2="19.1"/><line x1="4.9" y1="19.1" x2="7" y2="17"/><line x1="17" y1="7" x2="19.1" y2="4.9"/></svg>`;
const MOON = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>`;

export function getStoredTheme() {
  try {
    const t = localStorage.getItem(STORAGE_KEY);
    return t === 'light' || t === 'dark' ? t : null;
  } catch (e) {
    return null;
  }
}

export function systemPrefersDark() {
  try {
    return Boolean(window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches);
  } catch (e) {
    return false;
  }
}

export function activeTheme() {
  return document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light';
}

export function applyTheme(theme) {
  const next = theme === 'dark' ? 'dark' : 'light';
  document.documentElement.setAttribute('data-theme', next);
  try {
    localStorage.setItem(STORAGE_KEY, next);
  } catch (e) {
    /* storage unavailable — runtime toggle still works for this session */
  }
  updateButton();
  return next;
}

export function toggleTheme() {
  return applyTheme(activeTheme() === 'dark' ? 'light' : 'dark');
}

let button = null;

function updateButton() {
  if (!button) return;
  const dark = activeTheme() === 'dark';
  // In dark mode we offer the sun (→ switch to light); in light mode, the moon.
  button.innerHTML = dark ? SUN : MOON;
  const label = dark ? 'Switch to light theme' : 'Switch to dark theme';
  button.setAttribute('aria-label', label);
  button.title = label;
}

export function mountToggle() {
  if (typeof document === 'undefined' || document.querySelector('.lrc-theme-toggle')) return;
  button = document.createElement('button');
  button.type = 'button';
  button.className = 'lrc-theme-toggle';
  button.addEventListener('click', toggleTheme);
  updateButton();
  document.body.appendChild(button);
}

if (typeof document !== 'undefined') {
  if (document.readyState !== 'loading') {
    mountToggle();
  } else {
    document.addEventListener('DOMContentLoaded', mountToggle);
  }
}
