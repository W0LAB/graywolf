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

// Test-only.
export function _setDesktopTokenForTests(v) {
  token = v;
}
