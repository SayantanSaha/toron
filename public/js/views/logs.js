/**
 * Toron Dashboard - View 4: Live Requests & Logs
 */
import { $, $$, esc, fmt, dtFmt, stCls } from '../utils.js';
import { ICON } from '../components/icons.js';
import { state, setLive } from '../state.js';
import { buildDataModel } from '../model.js';
import { rawApiLogs } from '../api.js';
import { openDrawer } from '../components/drawer.js';

export function lgShell() {
  const routes = buildDataModel().routes;
  return `<section class="card">
    <div class="toolbar">
      <div class="search">${ICON('i-search')}<input class="field" id="lgQ" type="search" placeholder="Filter path, upstream, or trace ID..." aria-label="Filter requests" value="${esc(state.lq)}"></div>
      ${['2', '3', '4', '5'].map(c => `<button class="chip" data-st="${c}" aria-pressed="${state.lst.has(c)}">${c}xx</button>`).join('')}
      <button class="chip" id="lgSlow" aria-pressed="${state.lslow}">Over SLO</button>
      <select class="field" id="lgRoute" aria-label="Route"><option value="all">All routes</option>${routes.map(r => `<option value="${r.id}"${state.lroute === r.id ? ' selected' : ''}>${esc(r.short)}</option>`).join('')}</select>
      <span class="sp"></span>
      <button class="btn" id="lgPause"></button>
    </div>
    <div class="tsum" id="lgSum"></div>
    <div class="tscroll"><table class="tbl lg-tbl"><thead><tr><th>Time</th><th>Status</th><th>Request</th><th class="hide-sm">Source IP</th><th class="hide-md">Route</th><th class="hide-md">Upstream</th><th class="num">Duration</th><th class="hide-sm">Trace</th></tr></thead><tbody id="lgBody"></tbody></table></div></section>`;
}

export function lgSync() {
  const b = $('#lgPause');
  if (b) b.innerHTML = state.live ? `${ICON('i-pause')}Pause Tail` : `${ICON('i-play')}Resume Tail`;
}

export function lgInit() {
  const upd = () => lgUpdate(buildDataModel(), true);
  const qEl = $('#lgQ');
  if (qEl) qEl.addEventListener('input', e => { state.lq = e.target.value.trim().toLowerCase(); upd(); });
  const rtEl = $('#lgRoute');
  if (rtEl) rtEl.addEventListener('change', e => { state.lroute = e.target.value; upd(); });
  const slEl = $('#lgSlow');
  if (slEl) slEl.addEventListener('click', e => { state.lslow = !state.lslow; e.currentTarget.setAttribute('aria-pressed', String(state.lslow)); upd(); });
  $$('[data-st]').forEach(b => b.addEventListener('click', () => {
    const c = b.dataset.st;
    state.lst.has(c) ? state.lst.delete(c) : state.lst.add(c);
    b.setAttribute('aria-pressed', String(state.lst.has(c)));
    upd();
  }));
  const psEl = $('#lgPause');
  if (psEl) psEl.addEventListener('click', () => setLive(!state.live));
  const bEl = $('#lgBody');
  if (bEl) {
    bEl.addEventListener('click', e => {
      const r = e.target.closest('[data-log]');
      if (r) openDrawer('log', +r.dataset.log);
    });
  }
  lgSync();
}

export function lgUpdate(D, force) {
  const body = $('#lgBody');
  if (!body) return;
  const logs = (rawApiLogs && rawApiLogs.length > 0) ? rawApiLogs : [
    { id: 101, ts: Date.now() - 400, method: 'GET', path: '/v1/orders/8f2a91', route: 'orders', short: 'api /v1/orders', status: 200, ms: 14.2, up: '10.0.1.11:8080', trace: 'a4b1c8f0e2d4', ip: '192.168.1.10', bytes: 1240 },
    { id: 102, ts: Date.now() - 900, method: 'POST', path: '/v1/payments/charge', route: 'payments', short: 'api /v1/payments', status: 502, ms: 210.5, up: '10.0.4.13:8080', trace: 'b9e3d1a8c7f2', ip: '192.168.1.15', bytes: 420, err: 'connect: connection refused' }
  ];

  const selR = (D && D.routes) ? D.routes.find(r => r.id === state.lroute) : null;
  const f = logs.filter(e => {
    if (!state.lst.has(stCls(e.status))) return false;
    if (state.lroute !== 'all') {
      const pfx = selR ? (selR.path || selR.prefix) : state.lroute;
      const matchRoute = (e.route === state.lroute) ||
                         (pfx && (e.route === pfx || (e.route && e.route.startsWith(pfx + '/')) || e.path === pfx || (e.path && e.path.startsWith(pfx + '/')))) ||
                         (e.short && (e.short === state.lroute || (selR && e.short === selR.short)));
      if (!matchRoute) return false;
    }
    if (state.lq) {
      const haystack = (e.path + ' ' + (e.up || '') + ' ' + (e.trace || '') + ' ' + (e.short || '') + ' ' + (e.ip || e.client_ip || '')).toLowerCase();
      if (!haystack.includes(state.lq)) return false;
    }
    return true;
  }).sort((a, b) => {
    const dt = (b.ts || 0) - (a.ts || 0);
    if (dt !== 0) return dt;
    const ipA = a.ip || a.client_ip || '';
    const ipB = b.ip || b.client_ip || '';
    const dip = ipA.localeCompare(ipB, undefined, { numeric: true });
    if (dip !== 0) return dip;
    return (a.path || '').localeCompare(b.path || '', undefined, { numeric: true });
  });
  const sumEl = $('#lgSum');
  if (sumEl) sumEl.textContent = `Showing ${Math.min(f.length, 70)} of ${f.length} requests in live tail.` + (state.live ? '' : ' Tail is paused.');

  body.innerHTML = f.slice(0, 70).map(e => `<tr class="click" tabindex="0" data-log="${e.id}"><td class="tm" style="white-space:nowrap">${dtFmt(e.ts)}</td><td><span class="st s${stCls(e.status)}">${e.status}</span></td><td class="path"><span class="meth">${esc(e.method)}</span>${esc(e.path)}</td><td class="hide-sm"><code style="font-size:11.5px">${esc(e.ip || e.client_ip || '127.0.0.1')}</code></td><td class="hide-md">${esc(e.short || e.route)}</td><td class="hide-md"><code>${esc(e.up)}</code></td><td class="num">${fmt.ms(e.ms)}</td><td class="hide-sm"><code>${esc((e.trace || '').slice(0, 8))}</code></td></tr>`).join('') || `<tr><td colspan="8" class="empty">No requests match active filters.</td></tr>`;
}
