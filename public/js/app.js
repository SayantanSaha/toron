/**
 * Toron Dashboard - Native ECMAScript Modules Application Entrypoint
 * Router, View Registry, Chrome Coordinator & Event Dispatcher
 */
import { $, $$, esc, hms } from './utils.js';
import { state, invalidate, isForce, setForce, setLive, setOnLiveChange, themeIcon, toggleTheme, clearAuth } from './state.js';
import { ICON } from './components/icons.js';
import { fetchBackendData, setUpdateCallback } from './api.js';
import { buildDataModel } from './model.js';
import { dw, openDrawer, closeDrawer, renderDrawer } from './components/drawer.js';
import { showAuthModal, hideAuthModal, initAuthModal } from './components/authModal.js';

import { ovShell, ovUpdate } from './views/overview.js';
import { rtShell, rtInit, rtUpdate } from './views/routes.js';
import { upShell, upUpdate } from './views/upstreams.js';
import { lgShell, lgInit, lgUpdate, lgSync } from './views/logs.js';
import { ceShell, ceUpdate } from './views/certs.js';
import { mdShell, mdUpdate } from './views/modules.js';
import { alShell, alUpdate } from './views/alerts.js';
import { consoleShell, consoleInit, consoleUpdate } from './views/console.js';

/* ---------- Navigation & App Shell Metadata ---------- */
export const NAV = [
  { id: 'overview', label: 'Overview', icon: 'i-overview' },
  { id: 'routes', label: 'Routes', icon: 'i-routes' },
  { id: 'upstreams', label: 'Upstreams', icon: 'i-server' },
  { id: 'logs', label: 'Requests', icon: 'i-list' },
  { id: 'certs', label: 'Certificates', icon: 'i-shield' },
  { id: 'modules', label: 'Modules', icon: 'i-cube' },
  { id: 'alerts', label: 'Alerts', icon: 'i-bell' },
  { id: 'console', label: 'API Console', icon: 'i-terminal' }
];

export const META = {
  overview: { t: 'Overview', s: 'Gateway health, real-time traffic flow, and primary telemetry signals', range: 1, inst: 1 },
  routes: { t: 'Routes', s: 'Traffic, latency percentiles, and error rate breakdown per route', range: 1, inst: 1 },
  upstreams: { t: 'Upstreams', s: 'Target node health, connection load, and circuit breaker status', inst: 1 },
  logs: { t: 'Requests', s: 'Live request tailing with end-to-end trace waterfalls', inst: 1 },
  certs: { t: 'Certificates', s: 'ACME zero-touch TLS certificates and renewal lifecycle' },
  modules: { t: 'Modules & Runtime', s: 'Compiled-in reactors, event bus, and Go runtime internals', range: 1, inst: 1 },
  alerts: { t: 'Alerts & Incidents', s: 'WAF security violations and operational degradation events' },
  console: { t: 'API Console', s: 'Interactive endpoint probe execution and response inspector' }
};

export const VIEWS = {
  overview: { shell: ovShell, update: ovUpdate },
  routes: { shell: rtShell, init: rtInit, update: rtUpdate },
  upstreams: { shell: upShell, update: upUpdate },
  logs: { shell: lgShell, init: lgInit, update: lgUpdate },
  certs: { shell: ceShell, update: ceUpdate },
  modules: { shell: mdShell, update: mdUpdate },
  alerts: { shell: alShell, update: alUpdate },
  console: { shell: consoleShell, init: consoleInit, update: consoleUpdate }
};

export function renderNav() {
  const nEl = $('#nav');
  if (!nEl) return;
  const D = buildDataModel();
  const alertCount = (D.alerts || []).length;
  nEl.innerHTML = NAV.map(n => {
    const badge = (n.id === 'alerts' && alertCount > 0) ? `<span class="badge">${alertCount}</span>` : '';
    return `<button class="nv" data-nav="${n.id}" title="${esc(n.label)}"${n.id === state.view ? ' aria-current="page"' : ''}>${ICON(n.icon)}<span class="lbl">${esc(n.label)}</span>${badge}</button>`;
  }).join('');
}

export function head() {
  const m = META[state.view] || META.overview;
  const titleEl = $('#title');
  if (titleEl) titleEl.textContent = m.t;
  const subEl = $('#sub');
  if (subEl) subEl.textContent = m.s;
  const rangeSeg = $('#rangeSeg');
  if (rangeSeg) rangeSeg.hidden = !m.range;
  const instWrap = $('#instWrap');
  if (instWrap) instWrap.hidden = !m.inst;
  $$('#rangeSeg button').forEach(b => b.setAttribute('aria-pressed', String(b.dataset.r === state.range)));
  if (typeof document !== 'undefined') {
    document.title = `${m.t} · Toron Gateway`;
  }
}

export function mountView() {
  const v = VIEWS[state.view] || VIEWS.overview;
  const viewEl = $('#view');
  if (viewEl) viewEl.innerHTML = v.shell();
  if (v.init) v.init();
  renderNav();
  head();
  refresh(true);
}

export function updateAuthIcon() {
  if (typeof document === 'undefined') return;
  const isAuthed = state.auth && state.auth.authenticated;
  const iconHref = isAuthed ? '#i-unlock' : '#i-lock';
  const title = isAuthed ? 'Authenticated as Admin (Click to Lock)' : 'Lock / Authenticate Admin';
  $$('#authLockBtn use, #authLockBtn2 use').forEach(u => u.setAttribute('href', iconHref));
  const b1 = $('#authLockBtn');
  if (b1) b1.setAttribute('title', title);
  const b2 = $('#authLockBtn2');
  if (b2) b2.setAttribute('title', title);
}

export function chrome() {
  if (typeof document === 'undefined') return;
  const stamp = $('#stamp');
  if (stamp) stamp.textContent = state.live ? `Updated ${hms(new Date())}` : 'Paused';
  const b = $('#liveBtn');
  if (b) b.setAttribute('aria-pressed', String(state.live));
  const lt = $('#liveTxt');
  if (lt) lt.textContent = state.live ? 'Live' : 'Paused';
  updateAuthIcon();
  lgSync();
}

export function refresh(force) {
  if (typeof document === 'undefined') return;
  setForce(!!force);
  try {
    const D = buildDataModel();
    const v = VIEWS[state.view] || VIEWS.overview;
    v.update(D);
    if (dw.kind === 'route') renderDrawer();
    chrome();
  } finally {
    setForce(false);
  }
}

export function go(view, id) {
  closeNav();
  if (dw.kind) closeDrawer();
  if (view !== state.view && VIEWS[view]) {
    state.view = view;
    mountView();
    if (typeof window !== 'undefined' && window.scrollTo) window.scrollTo(0, 0);
  }
  try {
    if (typeof history !== 'undefined' && history.replaceState) {
      history.replaceState(null, '', '#/' + view + (id ? '/' + id : ''));
    }
  } catch (e) {}
  if (id && view === 'routes') openDrawer('route', id);
  if (id && view === 'upstreams') {
    const el = $('#pool-' + id);
    if (el && el.scrollIntoView) el.scrollIntoView({ block: 'center', behavior: 'smooth' });
  }
}

export function openNav() {
  const rail = $('#rail');
  if (rail) rail.classList.add('open');
  const scrim = $('#scrim');
  if (scrim) scrim.classList.add('on');
  const menuBtn = $('#menuBtn');
  if (menuBtn) menuBtn.setAttribute('aria-expanded', 'true');
}

export function closeNav() {
  const rail = $('#rail');
  if (!rail || !rail.classList.contains('open')) return;
  rail.classList.remove('open');
  if (!dw.kind) {
    const scrim = $('#scrim');
    if (scrim) scrim.classList.remove('on');
  }
  const menuBtn = $('#menuBtn');
  if (menuBtn) menuBtn.setAttribute('aria-expanded', 'false');
}

function activate(el) {
  const [v, id] = el.dataset.go.split(':');
  go(v, id);
}

/* ---------- Global Event Listeners ---------- */
if (typeof document !== 'undefined') {
  document.addEventListener('click', e => {
    const g = e.target.closest('[data-go]');
    if (g) { activate(g); return; }
    const n = e.target.closest('[data-nav]');
    if (n) { go(n.dataset.nav); }
  });

  document.addEventListener('keydown', e => {
    if ((e.key === 'Enter' || e.key === ' ') && e.target.matches && e.target.matches('[data-go][tabindex]')) {
      e.preventDefault();
      activate(e.target);
      return;
    }
    if (e.key === 'Escape') {
      if (dw.kind) closeDrawer();
      else closeNav();
    }
  });

  const scrim = $('#scrim');
  if (scrim) scrim.addEventListener('click', () => { if (dw.kind) closeDrawer(); else closeNav(); });

  const dwClose = $('#dwClose');
  if (dwClose) dwClose.addEventListener('click', closeDrawer);

  const mBtn = $('#menuBtn');
  if (mBtn) mBtn.addEventListener('click', () => {
    const rail = $('#rail');
    if (rail && rail.classList.contains('open')) closeNav();
    else openNav();
  });

  const tBtn = $('#themeBtn');
  if (tBtn) tBtn.addEventListener('click', toggleTheme);
  const tBtn2 = $('#themeBtn2');
  if (tBtn2) tBtn2.addEventListener('click', toggleTheme);

  const handleLockClick = () => {
    clearAuth();
    updateAuthIcon();
    showAuthModal();
  };
  const lkBtn = $('#authLockBtn');
  if (lkBtn) lkBtn.addEventListener('click', handleLockClick);
  const lkBtn2 = $('#authLockBtn2');
  if (lkBtn2) lkBtn2.addEventListener('click', handleLockClick);

  const lBtn = $('#liveBtn');
  if (lBtn) lBtn.addEventListener('click', () => setLive(!state.live));

  const rSeg = $('#rangeSeg');
  if (rSeg) rSeg.addEventListener('click', e => {
    const b = e.target.closest('button');
    if (!b) return;
    state.range = b.dataset.r;
    invalidate();
    head();
    refresh(true);
  });
}

// Wire state and polling listeners
setOnLiveChange(() => {
  chrome();
  if (state.view === 'logs') lgUpdate(buildDataModel(), true);
});

setUpdateCallback(() => {
  refresh(false);
});

/* ---------- Application Bootstrap ---------- */
export function boot() {
  if (typeof location !== 'undefined') {
    const m = (location.hash || '').match(/^#\/(\w+)(?:\/([\w-]+))?/);
    if (m && VIEWS[m[1]]) state.view = m[1];
  }
  themeIcon();
  initAuthModal();
  updateAuthIcon();
  mountView();
  fetchBackendData();
  if (typeof setInterval !== 'undefined') {
    setInterval(() => {
      if (state.live && (typeof document === 'undefined' || !document.hidden)) {
        state.step++;
        fetchBackendData();
      }
    }, 2000);
  }
}

// Auto-boot if running in browser
if (typeof window !== 'undefined' && typeof document !== 'undefined') {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
}
