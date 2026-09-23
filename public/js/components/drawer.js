/**
 * Toron Dashboard - Slide-out Inspector Drawer Controller
 */
import { $, esc, fmt, dtFmt, stCls } from '../utils.js';
import { ICON, pill, toneErr } from './icons.js';
import { chart, histo } from './charts.js';
import { buildDataModel } from '../model.js';
import { rawApiLogs } from '../api.js';

export const dw = {
  el: null,
  kind: null,
  id: null,
  last: null
};

export function openDrawer(kind, id) {
  dw.el = $('#drawer');
  if (!dw.el) return;
  dw.kind = kind;
  dw.id = id;
  dw.last = document.activeElement;
  dw.el.hidden = false;

  if (kind === 'route') {
    const D = buildDataModel();
    const r = D.routes.find(x => x.id === id) || D.routes[0];
    $('#dwTitle').textContent = r.short;
    $('#dwBody').innerHTML = `<div class="stats" id="dwStats"></div>
      <h3 class="dh">Latency (p95 against SLO)</h3><div class="chart" id="dwLat"></div>
      <h3 class="dh">Errors</h3><div class="chart" id="dwErr"></div>
      <h3 class="dh">Latency Distribution</h3><div class="hist" id="dwHist"></div>
      <h3 class="dh">Configuration</h3><dl class="kv"><dt>Match</dt><dd><code>${esc(r.host + r.path)}</code></dd><dt>Upstream</dt><dd><button class="linkbtn" data-go="upstreams:${r.pool}">${esc(r.pool)}</button></dd><dt>Timeout</dt><dd>${esc(r.timeout)}</dd><dt>SLO Target</dt><dd>p95 under ${r.slo} ms</dd><dt>Middlewares</dt><dd><div class="chips">${r.mw.map(m => `<span class="chipx">${esc(m)}</span>`).join('')}</div></dd></dl>`;
    renderDrawer();
  } else {
    const logs = (rawApiLogs && rawApiLogs.length > 0) ? rawApiLogs : [
      { id: 101, ts: Date.now(), method: 'GET', path: '/v1/orders/8f2a91', route: 'orders', short: 'api /v1/orders', status: 200, ms: 14.2, up: '10.0.1.11:8080', trace: 'a4b1c8f0e2d4', ip: '192.168.1.10', bytes: 1240 }
    ];
    const e = logs.find(x => x.id === id) || logs[0];
    $('#dwTitle').textContent = `${e.method} ${e.path}`;
    $('#dwSub').innerHTML = `${pill(stCls(e.status) === '5' ? 'err' : 'ok', String(e.status))}<span class="mut num">${fmt.ms(e.ms)} · ${dtFmt(e.ts)}</span>`;
    const spans = e.spans || [
      { name: 'Accept & Parse', mod: 'listener.http', d: 0.2 },
      { name: 'TLS Handshake', mod: 'listener.http', d: 2.1 },
      { name: 'Route Match', mod: 'router', d: 0.1 },
      { name: 'WAF Inspection', mod: 'waf.owasp', d: 0.3 },
      { name: 'Upstream Proxy', mod: 'proxy.reverse', d: Math.max(0.5, e.ms - 2.7) }
    ];
    let offset = 0;
    const spanRows = spans.map(s => {
      const left = (offset / e.ms * 100).toFixed(1);
      const width = Math.max(1, (s.d / e.ms * 100)).toFixed(1);
      offset += s.d;
      return `<div class="wf-row"><div class="wf-n"><b>${esc(s.name)}</b><span>${esc(s.mod)}</span></div><div class="wf-t"><i class="${s.bad ? 'bad' : ''}" style="left:${left}%;width:${width}%"></i></div><div class="wf-d">${s.d.toFixed(1)} ms</div></div>`;
    }).join('');

    $('#dwBody').innerHTML = `${e.err ? `<div class="callout err">${ICON('i-alert')}<div><b>Upstream Error</b><p>${esc(e.err)}</p></div></div>` : ''}
      <h3 class="dh">Trace Waterfall</h3>${spanRows}
      <h3 class="dh">Details</h3><dl class="kv"><dt>Trace ID</dt><dd><code>${esc(e.trace || 'none')}</code></dd><dt>Upstream Node</dt><dd><code>${esc(e.up)}</code></dd><dt>Client IP</dt><dd><code>${esc(e.ip)}</code></dd><dt>Response Size</dt><dd>${fmt.bytes(e.bytes || 0)}</dd></dl>`;
  }

  $('#scrim').classList.add('on');
  document.body.classList.add('lock');
  requestAnimationFrame(() => dw.el.classList.add('open'));
  const dwClose = $('#dwClose');
  if (dwClose) dwClose.focus();
}

export function renderDrawer() {
  if (dw.kind !== 'route') return;
  const D = buildDataModel();
  const r = D.routes.find(x => x.id === dw.id) || D.routes[0];
  const c = r.cur;
  const t = toneErr(c.err5);

  $('#dwSub').innerHTML = `${pill(t, t === 'err' ? 'Degraded' : 'Healthy')}<span class="mut">${esc(r.host + r.path)}</span>`;
  $('#dwStats').innerHTML = [
    ['Requests', fmt.n(c.rps) + '/s'],
    ['p50', fmt.ms(c.p50)],
    ['p95', fmt.ms(c.p95)],
    ['p99', fmt.ms(c.p99)],
    ['4xx rate', fmt.pct(c.err4, 2)],
    ['5xx rate', fmt.pct(c.err5, 2)]
  ].map(([k, v]) => `<div><dt>${esc(k)}</dt><dd>${esc(v)}</dd></div>`).join('');

  chart($('#dwLat'), {
    id: 'dwlat',
    height: 170,
    labels: D.labels,
    fmt: fmt.ms,
    aria: 'p95 latency curve',
    refs: [{ y: r.slo, label: `SLO ${r.slo}ms`, color: 'var(--warn)' }],
    series: [{ key: 'p95', label: 'p95', color: 'var(--s2)', values: r.s.p95, area: true }]
  });

  chart($('#dwErr'), {
    id: 'dwerr',
    height: 150,
    bars: true,
    stacked: true,
    labels: D.labels,
    fmt: v => v.toFixed(1) + '/s',
    aria: 'Error volume',
    series: [
      { key: 'e4', label: '4xx', color: 'var(--c4)', values: r.s.e4 },
      { key: 'e5', label: '5xx', color: 'var(--c5)', values: r.s.e5 }
    ]
  });

  const hs = histo(c.p50, c.p95);
  const mx = Math.max(...hs.map(h => h.v));
  $('#dwHist').innerHTML = hs.map(h => `<span class="mut">${esc(h.l)}</span><div class="bar"><i style="width:${(h.v / mx * 100).toFixed(1)}%;background:var(--s2)"></i></div><span class="r">${h.v}%</span>`).join('');
}

export function closeDrawer() {
  if (!dw.el || dw.el.hidden) return;
  dw.el.classList.remove('open');
  $('#scrim').classList.remove('on');
  document.body.classList.remove('lock');
  dw.kind = null;
  setTimeout(() => {
    if (!dw.kind && dw.el) dw.el.hidden = true;
  }, 230);
}
