import { $, esc, fmt } from '../utils.js';
import { state } from '../state.js';
import { ICON, pill, toneErr } from '../components/icons.js';
import { spark } from '../components/charts.js';
import { buildDataModel } from '../model.js';

export function rtHead() {
  const cols = [['name', 'Route', ''], [null, 'Upstream', 'hide-md'], ['rps', 'Requests', 'num'], ['p95', 'p95 latency', 'num'], ['err', '5xx rate', 'num'], [null, 'Status', 'hide-sm']];
  return `<tr>${cols.map(([k, l, c]) => {
    const on = k && state.sort.k === k;
    return `<th class="${c}" aria-sort="${on ? (state.sort.dir > 0 ? 'ascending' : 'descending') : 'none'}">${k ? `<button class="th" data-sort="${k}">${l}${on ? (state.sort.dir > 0 ? ' ▲' : ' ▼') : ''}</button>` : l}</th>`;
  }).join('')}</tr>`;
}

export function rtShell() {
  return `<section class="card">
    <div class="toolbar"><div class="search">${ICON('i-search')}<input class="field" id="rtQ" type="search" placeholder="Filter routes..." aria-label="Filter routes" value="${esc(state.q)}"></div><span class="sp"></span><span class="mut" id="rtCount"></span></div>
    <div class="tscroll"><table class="tbl"><thead id="rtHead">${rtHead()}</thead><tbody id="rtBody"></tbody></table></div></section>`;
}

export function rtInit() {
  $('#rtQ')?.addEventListener('input', e => {
    state.q = e.target.value.trim().toLowerCase();
    rtUpdate(buildDataModel());
  });
  $('#rtHead')?.addEventListener('click', e => {
    const b = e.target.closest('[data-sort]');
    if (!b) return;
    const k = b.dataset.sort;
    state.sort = state.sort.k === k ? { k, dir: -state.sort.dir } : { k, dir: k === 'name' ? 1 : -1 };
    $('#rtHead').innerHTML = rtHead();
    rtUpdate(buildDataModel());
  });
}

export function rtUpdate(D) {
  const body = $('#rtBody');
  if (!body) return;
  const rows = D.routes.filter(r => !state.q || (r.short + r.host + r.path + r.pool).toLowerCase().includes(state.q));
  const key = { name: r => r.short, rps: r => r.cur.rps, p95: r => r.cur.p95, err: r => r.cur.err5 }[state.sort.k] || (r => r.cur.rps);
  rows.sort((a, b) => (key(a) > key(b) ? 1 : key(a) < key(b) ? -1 : 0) * state.sort.dir);
  const bad = D.routes.filter(r => toneErr(r.cur.err5) === 'err').length;
  const countEl = $('#rtCount');
  if (countEl) countEl.textContent = `${rows.length} of ${D.routes.length} routes${bad ? ` · ${bad} degraded` : ''}`;

  body.innerHTML = rows.map(r => {
    const t = toneErr(r.cur.err5);
    return `<tr class="click" tabindex="0" data-go="routes:${r.id}">
      <td><div class="rt"><b>${esc(r.short)}</b><span class="mut">${esc(r.host + r.path)}</span></div></td>
      <td class="hide-md mut">${esc(r.pool)}</td>
      <td class="num"><div class="rps"><span>${fmt.n(r.cur.rps)}/s</span><span class="hide-sm">${spark(r.s.rps, { h: 24 })}</span></div></td>
      <td class="num">${fmt.ms(r.cur.p95)}</td>
      <td class="num ${t === 'ok' ? '' : 't-' + t}">${fmt.pct(r.cur.err5, 2)}</td>
      <td class="hide-sm">${pill(t, t === 'err' ? 'Degraded' : (t === 'warn' ? 'Elevated' : 'Healthy'))}</td></tr>`;
  }).join('') || `<tr><td colspan="6" class="empty">No routes match "${esc(state.q)}".</td></tr>`;
}
