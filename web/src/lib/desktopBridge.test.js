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

test('isExternalUrl: other http(s) origins only', async () => {
  const { isExternalUrl } = await import('./desktopBridge.js');
  fakeEnv('http://127.0.0.1:4321/#/');
  assert.equal(isExternalUrl('https://aprs.fi/'), true);
  assert.equal(isExternalUrl('http://127.0.0.1:8080/'), true); // different port = other origin
  assert.equal(isExternalUrl('/api/flare'), false);
  assert.equal(isExternalUrl('#/map'), false);
  assert.equal(isExternalUrl('http://127.0.0.1:4321/x'), false);
  assert.equal(isExternalUrl('mailto:a@b.c'), false);
  assert.equal(isExternalUrl('javascript:void 0'), false);
});

test('window.open of an external URL goes to the shell, not a popup', async () => {
  const { installExternalLinkHandler } = await import('./desktopBridge.js');
  fakeEnv('http://127.0.0.1:4321/#/');
  const posted = [];
  const popups = [];
  const savedFetch = globalThis.fetch;
  const savedOpen = globalThis.open;
  const savedDoc = globalThis.document;
  globalThis.fetch = async (url, init) => { posted.push([url, JSON.parse(init.body).url]); return { ok: true }; };
  globalThis.open = (url) => { popups.push(url); return {}; };
  globalThis.document = { addEventListener() {} };
  try {
    _setDesktopTokenForTests('tok');
    installExternalLinkHandler();
    assert.equal(globalThis.open('https://www.qrz.com/db/W0LAB'), null);
    globalThis.open('/#/map');
    await new Promise((r) => setTimeout(r, 0));
    assert.deepEqual(posted, [['/api/desktop/open-url', 'https://www.qrz.com/db/W0LAB']]);
    assert.deepEqual(popups, ['/#/map']);
  } finally {
    globalThis.fetch = savedFetch;
    if (savedOpen === undefined) delete globalThis.open; else globalThis.open = savedOpen;
    if (savedDoc === undefined) delete globalThis.document; else globalThis.document = savedDoc;
  }
});
