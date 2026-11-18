// Per-launch bearer token for the native desktop shell
// (cmd/graywolf-desktop). The shell opens the window on
//   http://127.0.0.1:<port>/?gwdesktop=<token>
// and this module captures the token once at boot, keeps it in
// sessionStorage so an in-window reload still authenticates, and
// strips it from the address bar.
//
// Deliberately separate from androidBridge.js / GraywolfWebInterface:
// platform.js treats the presence of that Android bridge as "running
// on Android", and the desktop shell must not light up Android-only UI.
//
// Returns null in a normal browser, so the bearer wiring stays a no-op
// there and cookie-session login applies as usual.

export const DESKTOP_TOKEN_PARAM = 'gwdesktop';
const STORAGE_KEY = 'graywolf.desktopToken';

let token = null;

export function captureDesktopToken() {
  try {
    const params = new URLSearchParams(globalThis.location?.search || '');
    const fromUrl = params.get(DESKTOP_TOKEN_PARAM);
    if (fromUrl) {
      token = fromUrl;
      try { globalThis.sessionStorage?.setItem(STORAGE_KEY, fromUrl); } catch { /* storage blocked */ }
      params.delete(DESKTOP_TOKEN_PARAM);
      const qs = params.toString();
      const loc = globalThis.location;
      globalThis.history?.replaceState?.(
        globalThis.history.state,
        '',
        `${loc.pathname}${qs ? `?${qs}` : ''}${loc.hash}`,
      );
      return token;
    }
    const stored = globalThis.sessionStorage?.getItem(STORAGE_KEY);
    token = stored || null;
  } catch {
    token = null;
  }
  return token;
}

export function getDesktopToken() {
  return token;
}

// isExternalUrl reports whether href points at an http(s) origin other
// than the station's own. Relative and same-origin links stay in-app.
export function isExternalUrl(href) {
  try {
    const base = globalThis.location?.href || 'http://127.0.0.1/';
    const u = new URL(href, base);
    if (u.protocol !== 'http:' && u.protocol !== 'https:') return false;
    return u.origin !== new URL(base).origin;
  } catch {
    return false;
  }
}

// openExternal asks the desktop shell to open url in the system browser
// (POST /api/desktop/open-url; the bearer is added by secureFetch).
export function openExternal(url) {
  return globalThis.fetch('/api/desktop/open-url', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url }),
  }).catch(() => {});
}

let linkHandlerInstalled = false;

// installExternalLinkHandler routes external links and window.open calls
// to the system browser when running inside the desktop app. Neither
// webview does that on its own: WebView2 opens a bare popup window and
// WebKitGTK ignores target=_blank. No-op in a normal browser.
export function installExternalLinkHandler() {
  if (linkHandlerInstalled || !token) return;
  const doc = globalThis.document;
  if (!doc?.addEventListener) return;
  linkHandlerInstalled = true;

  doc.addEventListener('click', (ev) => {
    if (ev.defaultPrevented || ev.button > 1) return;
    const a = ev.target?.closest?.('a[href]');
    if (!a) return;
    const href = a.getAttribute('href');
    if (!isExternalUrl(href)) return;
    ev.preventDefault();
    openExternal(new URL(href, globalThis.location.href).href);
  }, true);

  const originalOpen = globalThis.open?.bind(globalThis);
  globalThis.open = function (url, ...rest) {
    if (url != null && isExternalUrl(String(url))) {
      openExternal(new URL(String(url), globalThis.location.href).href);
      return null;
    }
    return originalOpen ? originalOpen(url, ...rest) : null;
  };
}

// Test-only.
export function _setDesktopTokenForTests(v) {
  token = v;
}
