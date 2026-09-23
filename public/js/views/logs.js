import { $, $$, esc, fmt, dtFmt, stCls } from '../utils.js';
import { ICON } from '../components/icons.js';
import { state, setLive } from '../state.js';
import { buildDataModel } from '../model.js';
import { rawApiLogs } from '../api.js';
import { openDrawer } from '../components/drawer.js';

export function lgShell() {
  const routes = buildDataModel().routes;
  const arr = c => state.lsort === c ? (state.lsortDir === 'asc' ? ' ▲' : ' ▼') : '';
  const th = (c, l, cls = '') => `<th class="sortable click ${cls}" data-sort="${c}" tabindex="0">${l}<span class="s-arr" id="sArr-${c}">${arr(c)}</span></th>`;
  const tOpts = [['all','All Buffer'],['1m','Last 1 min'],['5m','Last 5 mins'],['1h','Last 1 hour'],['custom','Custom...']]
    .map(([v, l]) => `<option value="${v}"${state.ltime === v ? ' selected' : ''}>${l}</option>`).join('');
  const rOpts = '<option value="all">All routes</option>' + routes.map(r => `<option value="${r.id}"${state.lroute === r.id ? ' selected' : ''}>${esc(r.short)}</option>`).join('');
  const stChips = ['2', '3', '4', '5'].map(c => `<button class="chip" data-st="${c}" aria-pressed="${state.lst.has(c)}">${c}xx</button>`).join('');
  const pBtns = [['First','&laquo;'],['Prev','&lsaquo;'],['Next','&rsaquo;'],['Last','&raquo;']]
    .map(([k, ic]) => `<button class="btn icon sm" id="lg${k}" title="${k}" aria-label="${k}">${ic}</button>`);
  return `<section class="card">
    <div class="lg-analytics" id="lgAnalytics">
      <div class="lg-hist-wrap"><div class="lg-hist-h"><span class="mut">Status Distribution</span><span id="lgHistTotal" class="mut num"></span></div><div class="lg-hist" id="lgHist"></div></div>
      <div class="lg-facets" id="lgFacets"></div>
    </div>
    <div class="toolbar" style="flex-wrap:wrap;gap:8px">
      <div class="search">${ICON('i-search')}<input class="field" id="lgQ" type="search" placeholder="Filter path, upstream, trace, IP..." aria-label="Filter requests" value="${esc(state.lq)}"></div>
      <select class="field" id="lgTime" aria-label="Time Window">${tOpts}</select>
      <div id="lgCustomTime" class="lg-custom-time" style="display:${state.ltime === 'custom' ? 'inline-flex' : 'none'}">
        <input type="datetime-local" class="field lg-dt" id="lgFrom"><span class="mut lg-dt">to</span><input type="datetime-local" class="field lg-dt" id="lgTo"><button class="btn sm lg-dt-btn" id="lgCustomApply">Apply</button>
      </div>
      ${stChips}
      <button class="chip" id="lgSlow" aria-pressed="${state.lslow}">Over SLO</button>
      <button class="chip" id="lgNoHealth" aria-pressed="${state.lnohealth}">No /health</button>
      <select class="field" id="lgRoute" aria-label="Route">${rOpts}</select>
      <span class="sp"></span><button class="btn" id="lgPause"></button>
    </div>
    <div class="tscroll"><table class="tbl lg-tbl"><thead><tr>${th('ts','Time')}${th('status','Status')}${th('path','Request')}<th class="hide-sm">Source IP</th><th class="hide-md">Route</th><th class="hide-md">Upstream</th>${th('ms','Duration','num')}<th class="hide-sm">Trace</th></tr></thead><tbody id="lgBody"></tbody></table></div>
    <div class="lg-foot-bar">
      <div class="tsum" id="lgSum"></div>
      <div class="pagination" id="lgPaging">
        ${pBtns[0]}${pBtns[1]}<span class="page-ind" id="lgPageInd">Page 1/1</span>${pBtns[2]}${pBtns[3]}
        <select class="field sm lg-pg-sz" id="lgPageSize" aria-label="Page Size">
          ${[25, 50, 100].map(n => `<option value="${n}"${state.lpageSize === n ? ' selected' : ''}>${n} / page</option>`).join('')}
        </select>
      </div>
    </div>
  </section>`;
}

export function lgSync() {
  const b = $('#lgPause');
  if (b) b.innerHTML = state.live ? `${ICON('i-pause')}Pause Tail` : `${ICON('i-play')}Resume Tail`;
}

function renderHist(items) {
  const el = $('#lgHist'), tot = $('#lgHistTotal');
  if (!el) return;
  if (!items.length) {
    el.innerHTML = '<span class="mut lg-hist-empty">No requests in window.</span>';
    if (tot) tot.textContent = '0 reqs';
    return;
  }
  const B = 24, now = Date.now(), l = items.length;
  let minT = items[0].ts || now, maxT = minT;
  for (let i = 1; i < l; i++) {
    const t = items[i].ts || now;
    if (t < minT) minT = t; else if (t > maxT) maxT = t;
  }
  const span = Math.max(1000, maxT - minT);
  const bks = Array.from({ length: B }, () => ({ 2: 0, 3: 0, 4: 0, 5: 0, tot: 0 }));
  for (const e of items) {
    const idx = Math.min(B - 1, Math.max(0, Math.floor((((e.ts || now) - minT) / span) * B)));
    const c = stCls(e.status);
    if (bks[idx][c] !== undefined) bks[idx][c]++;
    bks[idx].tot++;
  }
  const max = Math.max(1, ...bks.map(b => b.tot)), H = 34, bw = 3.4, gap = 0.76;
  const clrs = [['5', 'var(--c5)'], ['4', 'var(--c4)'], ['3', 'var(--s2)'], ['2', 'var(--ok)']];
  el.innerHTML = `<svg viewBox="0 0 100 ${H}" preserveAspectRatio="none" style="width:100%;height:${H}px;display:block">${bks.map((b, i) => {
    const x = (i * (bw + gap)).toFixed(1);
    if (!b.tot) return `<rect x="${x}%" y="${H - 1}" width="${bw}%" height="1" fill="var(--line-2)"/>`;
    let y = H, p = '';
    for (const [k, clr] of clrs) {
      if (b[k] > 0) {
        const sh = Math.max(1.5, (b[k] / max) * (H - 2));
        y -= sh;
        p += `<rect x="${x}%" y="${y.toFixed(1)}" width="${bw}%" height="${sh.toFixed(1)}" fill="${clr}"/>`;
      }
    }
    return p;
  }).join('')}</svg>`;
  if (tot) tot.textContent = `${l} reqs`;
}

function renderFacets(items) {
  const el = $('#lgFacets'); if (!el) return;
  if (!items.length) { el.innerHTML = ''; return; }
  const rc = {}, lats = []; let ec = 0, maxMs = 0, slow = '';
  for (const e of items) {
    const r = e.short || e.route || 'unknown'; rc[r] = (rc[r] || 0) + 1;
    if (['4', '5'].includes(stCls(e.status))) ec++;
    const ms = Number(e.ms) || 0; lats.push(ms);
    if (ms > maxMs) { maxMs = ms; slow = `${e.method} ${e.path}`; }
  }
  lats.sort((a, b) => a - b);
  const p95 = lats.length ? lats[Math.floor(lats.length * 0.95)] : 0;
  const rate = ((ec / items.length) * 100).toFixed(1);
  const top = Object.entries(rc).sort((a, b) => b[1] - a[1]).slice(0, 3);
  const pills = top.map(([r, c]) => `<button class="facet-pill click" data-setroute="${esc(r)}">${esc(r)} <b class="num">${c}</b></button>`).join('');
  const rCls = Number(rate) > 5 ? '5' : (Number(rate) > 1 ? '4' : '2');
  el.innerHTML = `<div class="facet-col"><span class="mut">Top Routes</span><div class="facet-pills">${pills}</div></div><div class="facet-col"><span class="mut">P95 / Slowest</span><div class="facet-stat"><span class="num">${fmt.ms(p95)}</span><span class="mut facet-slow" title="${esc(slow)}">${esc(slow || 'none')}</span></div></div><div class="facet-col"><span class="mut">Error Rate</span><div class="facet-stat"><span class="st s${rCls} facet-rate">${rate}%</span></div></div>`;
}

export function lgInit() {
  const upd = force => lgUpdate(buildDataModel(), force);
  const on = (id, evt, fn) => $(id)?.addEventListener(evt, fn);
  on('#lgQ', 'input', e => { state.lq = e.target.value.trim().toLowerCase(); state.lpage = 1; upd(true); });
  on('#lgTime', 'change', e => {
    state.ltime = e.target.value;
    const ct = $('#lgCustomTime'); if (ct) ct.style.display = state.ltime === 'custom' ? 'inline-flex' : 'none';
    state.lpage = 1; upd(true);
  });
  on('#lgCustomApply', 'click', () => {
    state.lfrom = $('#lgFrom')?.value ? new Date($('#lgFrom').value).getTime() : null;
    state.lto = $('#lgTo')?.value ? new Date($('#lgTo').value).getTime() : null;
    state.lpage = 1; upd(true);
  });
  on('#lgRoute', 'change', e => { state.lroute = e.target.value; state.lpage = 1; upd(true); });
  on('#lgSlow', 'click', e => { state.lslow = !state.lslow; e.currentTarget.setAttribute('aria-pressed', String(state.lslow)); state.lpage = 1; upd(true); });
  on('#lgNoHealth', 'click', e => { state.lnohealth = !state.lnohealth; e.currentTarget.setAttribute('aria-pressed', String(state.lnohealth)); state.lpage = 1; upd(true); });
  $$('[data-st]').forEach(b => b.addEventListener('click', () => {
    const c = b.dataset.st; state.lst.has(c) ? state.lst.delete(c) : state.lst.add(c);
    b.setAttribute('aria-pressed', String(state.lst.has(c))); state.lpage = 1; upd(true);
  }));
  on('#lgPause', 'click', () => setLive(!state.live));
  const bEl = $('#lgBody');
  if (bEl) {
    bEl.addEventListener('mouseenter', () => { state.lhover = true; });
    bEl.addEventListener('mouseleave', () => { state.lhover = false; });
    bEl.addEventListener('click', e => { const r = e.target.closest('[data-log]'); if (r) openDrawer('log', +r.dataset.log); });
  }
  on('#lgFacets', 'click', e => {
    const p = e.target.closest('[data-setroute]');
    if (p) { state.lroute = p.dataset.setroute; const re = $('#lgRoute'); if (re) re.value = state.lroute; state.lpage = 1; upd(true); }
  });
  $$('th.sortable').forEach(th => th.addEventListener('click', () => {
    const col = th.dataset.sort;
    state.lsortDir = (state.lsort === col) ? (state.lsortDir === 'asc' ? 'desc' : 'asc') : ((col === 'path') ? 'asc' : 'desc');
    state.lsort = col;
    ['ts', 'status', 'path', 'ms'].forEach(c => {
      const sa = $(`#sArr-${c}`); if (sa) sa.textContent = state.lsort === c ? (state.lsortDir === 'asc' ? ' ▲' : ' ▼') : '';
    });
    upd(true);
  }));
  ['#lgFirst', '#lgPrev', '#lgNext', '#lgLast'].forEach((id, i) => {
    on(id, 'click', () => {
      state.lpage = i === 0 ? 1 : (i === 1 ? Math.max(1, state.lpage - 1) : (i === 2 ? state.lpage + 1 : 999999));
      upd(true);
    });
  });
  on('#lgPageSize', 'change', e => { state.lpageSize = Number(e.target.value) || 25; state.lpage = 1; upd(true); });
  lgSync();
}

export function lgUpdate(D, force) {
  const body = $('#lgBody'); if (!body) return;
  const logs = (rawApiLogs && rawApiLogs.length > 0) ? rawApiLogs : [
    { id: 101, ts: Date.now() - 400, method: 'GET', path: '/v1/orders/8f2a91', route: 'orders', short: 'api /v1/orders', status: 200, ms: 14.2, up: '10.0.1.11:8080', trace: 'a4b1c8f0', ip: '127.0.0.1' }
  ];
  const now = Date.now(), selR = (D && D.routes) ? D.routes.find(r => r.id === state.lroute) : null;
  const dWin = { '1m': 60000, '5m': 300000, '1h': 3600000 };
  const f = logs.filter(e => {
    const ts = e.ts || 0;
    if (!state.lst.has(stCls(e.status))) return false;
    if (state.lslow && (Number(e.ms) || 0) < 100) return false;
    if (state.lnohealth && (e.path === '/health' || e.path?.startsWith('/health/'))) return false;
    if (dWin[state.ltime] && (now - ts) > dWin[state.ltime]) return false;
    if (state.ltime === 'custom' && ((state.lfrom && ts < state.lfrom) || (state.lto && ts > state.lto))) return false;
    if (state.lroute !== 'all') {
      const pfx = selR ? (selR.path || selR.prefix) : state.lroute;
      const m = (e.route === state.lroute) || (pfx && (e.route === pfx || e.route?.startsWith(pfx + '/') || e.path === pfx || e.path?.startsWith(pfx + '/'))) || (e.short && (e.short === state.lroute || e.short === selR?.short));
      if (!m) return false;
    }
    return !state.lq || `${e.path} ${e.up || ''} ${e.trace || ''} ${e.short || ''} ${e.ip || e.client_ip || ''}`.toLowerCase().includes(state.lq);
  });

  renderHist(f);
  renderFacets(f);

  const mult = state.lsortDir === 'asc' ? 1 : -1;
  f.sort((a, b) => {
    let diff = 0;
    if (state.lsort === 'status') diff = (a.status || 0) - (b.status || 0);
    else if (state.lsort === 'ms') diff = (a.ms || 0) - (b.ms || 0);
    else if (state.lsort === 'path') diff = (a.path || '').localeCompare(b.path || '', undefined, { numeric: true });
    else diff = (b.ts || 0) - (a.ts || 0);
    return diff !== 0 ? diff * mult : (b.ts || 0) - (a.ts || 0);
  });

  const totalPages = Math.max(1, Math.ceil(f.length / state.lpageSize));
  if (state.lpage > totalPages) state.lpage = totalPages;
  if (state.lpage < 1) state.lpage = 1;
  const start = (state.lpage - 1) * state.lpageSize;
  const end = Math.min(f.length, start + state.lpageSize);
  const pageItems = f.slice(start, end);

  const sumEl = $('#lgSum');
  if (sumEl) sumEl.textContent = `Showing ${f.length ? start + 1 : 0}–${end} of ${f.length} requests.` + (state.lhover ? ' Tail paused (hover/inspect).' : (state.live ? '' : ' Tail paused.'));
  const pgInd = $('#lgPageInd'); if (pgInd) pgInd.textContent = `Page ${state.lpage}/${totalPages}`;
  $('#lgFirst') && ($('#lgFirst').disabled = state.lpage <= 1);
  $('#lgPrev') && ($('#lgPrev').disabled = state.lpage <= 1);
  $('#lgNext') && ($('#lgNext').disabled = state.lpage >= totalPages);
  $('#lgLast') && ($('#lgLast').disabled = state.lpage >= totalPages);

  if (state.lhover && !force) return;

  body.innerHTML = pageItems.map(e => `<tr class="click" tabindex="0" data-log="${e.id}"><td class="tm lg-nowrap">${dtFmt(e.ts)}</td><td><span class="st s${stCls(e.status)}">${e.status}</span></td><td class="path"><span class="meth">${esc(e.method)}</span>${esc(e.path)}</td><td class="hide-sm"><code>${esc(e.ip || e.client_ip || '127.0.0.1')}</code></td><td class="hide-md">${esc(e.short || e.route)}</td><td class="hide-md"><code>${esc(e.up)}</code></td><td class="num">${fmt.ms(e.ms)}</td><td class="hide-sm"><code>${esc((e.trace || '').slice(0, 8))}</code></td></tr>`).join('') || `<tr><td colspan="8" class="empty">No requests match active filters.</td></tr>`;
}
