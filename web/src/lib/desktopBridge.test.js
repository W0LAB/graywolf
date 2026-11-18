import { test, beforeEach, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import { _resetForTests as resetBridge } from './androidBridge.js';
import {
  captureDesktopToken,
  getDesktopToken,
  _setDesktopTokenForTests,
} from './desktopBridge.js';
import { Platform } from './platform.js';

const saved = {
  location: globalThis.location,
  history: globalThis.history,
  sessionStorage: globalThis.sessionStorage,
};

function fakeEnv(href) {
  const u = new URL(href);
  const store = new Map();
  const replaced = [];
  globalThis.location = {
    href: u.href, origin: u.origin, pathname: u.pathname, search: u.search, hash: u.hash,
  };
  globalThis.history = {
    state: null,
    replaceState: (_s, _t, url) => { replaced.push(url); },
  };
  globalThis.sessionStorage = {
    getItem: (k) => (store.has(k) ? store.get(k) : null),
    setItem: (k, v) => { store.set(k, String(v)); },
  };
  return { store, replaced };
}

beforeEach(() => {
  resetBridge();
  delete globalThis.GraywolfWebInterface;
  _setDesktopTokenForTests(null);
});
afterEach(() => {
  resetBridge();
  delete globalThis.GraywolfWebInterface;
  _setDesktopTokenForTests(null);
  for (const [k, v] of Object.entries(saved)) {
    if (v === undefined) delete globalThis[k]; else globalThis[k] = v;
  }
});

test('captures token from ?gwdesktop=, stores it, and strips it from the URL', () => {
  const { store, replaced } = fakeEnv('http://127.0.0.1:4321/?gwdesktop=abc123&x=1#/map');
  assert.equal(captureDesktopToken(), 'abc123');
  assert.equal(getDesktopToken(), 'abc123');
  assert.equal(store.get('graywolf.desktopToken'), 'abc123');
  assert.deepEqual(replaced, ['/?x=1#/map']);
});

test('falls back to sessionStorage on reload (no param)', () => {
  const { store, replaced } = fakeEnv('http://127.0.0.1:4321/#/');
  store.set('graywolf.desktopToken', 'kept');
  assert.equal(captureDesktopToken(), 'kept');
  assert.deepEqual(replaced, []);
});

test('plain browser: no param, no storage -> null', () => {
  fakeEnv('http://127.0.0.1:8080/#/');
  assert.equal(captureDesktopToken(), null);
  assert.equal(Platform.kind, 'desktop');
  assert.equal(Platform.hasLaunchToken, false);
});

test('desktop token -> kind desktop-app, not android', () => {
  _setDesktopTokenForTests('tok');
  assert.equal(Platform.kind, 'desktop-app');
  assert.equal(Platform.isDesktopApp, true);
  assert.equal(Platform.isAndroid, false);
  assert.equal(Platform.hasLaunchToken, true);
});

test('Android bridge wins over a desktop token', () => {
  _setDesktopTokenForTests('tok');
  globalThis.GraywolfWebInterface = { getBearerToken: () => 'android' };
  assert.equal(Platform.kind, 'android');
  assert.equal(Platform.hasLaunchToken, true);
});
