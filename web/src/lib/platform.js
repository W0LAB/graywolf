// Platform detection. Returns 'android' when running inside the
// Android WebView (signalled by the GraywolfWebInterface JS bridge),
// otherwise 'desktop'. Used by Svelte routes/components that gate
// surfaces on host platform.
//
// Read via Platform.kind. The kind getter is dynamic — it consults
// the bridge on every access — so test setups that toggle
// globalThis.GraywolfWebInterface between cases observe the change
// without re-importing.
//
// Companion modules: pkg/platform.Kind (Go) and Platform.KIND (Kotlin).

import { getDesktopToken } from './desktopBridge.js';

// Kinds: 'android' (Android WebView), 'desktop-app' (native desktop
// shell, cmd/graywolf-desktop), 'desktop' (plain browser). Code that
// gates Android-only surfaces must compare against 'android', never
// against 'desktop', so the desktop shell gets desktop behaviour.
function detectKind() {
  // Bypass androidBridge's cache — Platform.kind is documented as dynamic
  // so test setups that toggle globalThis.GraywolfWebInterface observe
  // the change without re-importing.
  try {
    const v = globalThis.GraywolfWebInterface?.getBearerToken?.();
    if (typeof v === 'string' && v.length > 0) return 'android';
  } catch {
    // fall through
  }
  return getDesktopToken() ? 'desktop-app' : 'desktop';
}

export const Platform = {
  get kind() { return detectKind(); },
  // Convenience for the common "are we on Android?" gate. Backed by
  // the same detectKind() call as `kind`, so toggling
  // globalThis.GraywolfWebInterface between accesses flips this too.
  get isAndroid() { return detectKind() === 'android'; },
  get isDesktopApp() { return detectKind() === 'desktop-app'; },
  // True when a host shell authenticates the SPA with a per-launch
  // bearer token (Android or desktop app): no cookie login page.
  get hasLaunchToken() { return detectKind() !== 'desktop'; },
};
