/**
 * Toron Dashboard - Overview View Controller
 */
import { $, esc, fmt, dtFmt, clamp, avg } from '../utils.js';
import { ICON, pill } from '../components/icons.js';
import { flow } from '../components/sankey.js';
import { spark, chart } from '../components/charts.js';
import { rawApiStatus } from '../api.js';

export function bannerHTML(D) {
  const bad = D.routes.filter(r => r.cur.err5 >= .02).sort((a, b) => b.cur.err5 - a.cur.err5);
  const down = D.pools.filter(p => p.down > 0);
  const cert = D.certs.filter(c => c.state === 'failing');
  const alertCount = (D.alerts || []).length;

  let lvl = 'ok', title = 'All systems operational', text = `All ${D.routes.length} routes are within thresholds and upstream pools are healthy.`, go = '';

  if (bad.length > 0) {
    const r = bad[0];
    lvl = 'err';
    title = `Degraded: ${r.short}`;
    go = 'routes';
    text = `${fmt.pct(r.cur.err5, 1)} of requests to ${r.short} are failing, above the 2% threshold.`;
  } else if (down.length > 0) {
    lvl = 'warn';
    title = 'Needs attention: Upstreams Unhealthy';
    go = 'upstreams';
    text = `${down.length} upstream pool has failing backend instances.`;
  } else if (cert.length > 0) {
    lvl = 'warn';
    title = 'Needs attention: Certificate Renewal';
    go = 'certs';
    text = `${cert.length} certificate renewal is failing.`;
  } else if (alertCount > 0) {
    lvl = 'warn';
    title = `${alertCount} Active Alert${alertCount > 1 ? 's' : ''}`;
    go = 'alerts';
    text = `${alertCount} operational or security event${alertCount > 1 ? 's require' : ' requires'} administrative attention.`;
  }

  const ver = rawApiStatus ? rawApiStatus.version : '1.5.29';
  const meta = [
    ['Instances', 'Primary Node'],
    ['Version', `v${ver}`],
    ['Engine', 'Zero-Allocation Reactor'],
    ['Uptime', 'Healthy']
  ];
  const primaryBtnLabel = go === 'upstreams' ? 'View upstreams' : (go === 'certs' ? 'View certificates' : (go === 'routes' ? 'View routes' : 'View alerts'));
  return `<div class="banner ${lvl}"><div class="bn-main"><span class="bn-ic">${ICON(lvl === 'ok' ? 'i-check' : 'i-alert')}</span><div><h2>${esc(title)}</h2><p>${esc(text)}</p>${lvl !== 'ok' ? `<div class="bn-act">${go ? `<button class="btn primary" data-go="${go}">${esc(primaryBtnLabel)}</button>` : ''}<button class="btn" data-go="alerts">Open alerts</button></div>` : ''}</div></div><dl class="meta">${meta.map(([k, v]) => `<div><dt>${esc(k)}</dt><dd>${esc(v)}</dd></div>`).join('')}</dl></div>`;
}

export function signals(host, D) {
  const A = D.A, X = D.X, cur = a => a && a.length ? a[a.length - 1] : 0;
  const cells = [
    { k: 'Requests', vals: A.rps, show: v => `${fmt.n(v)}<small>req/s</small>`, bad: null, x: `Peak ${fmt.n(Math.max(...(A.rps.length ? A.rps : [0])))} req/s` },
    { k: 'p95 latency', vals: A.p95, show: v => `${fmt.ms(v)}`, bad: 'up', x: `p99 ${fmt.ms(cur(A.p99))}` },
    { k: '5xx rate', vals: A.err, show: v => `${(v * 100).toFixed(2)}<small>%</small>`, bad: 'up', zero: true, x: 'Error budget within target' },
    { k: 'Open connections', vals: X.conns, show: v => Math.round(v).toLocaleString('en-US'), bad: 'up', x: 'Active TCP client pool' },
    { k: 'Egress', vals: X.egress, show: v => `${fmt.n(v)}<small>Mb/s</small>`, bad: null, x: 'Streaming proxy bandwidth' }
  ];
  host.innerHTML = cells.map(c => {
    const v = cur(c.vals), mean = avg(c.vals), d = mean ? (v - mean) / mean : 0;
    const tone = c.bad === 'up' ? (d > .08 ? 'bad' : d < -.08 ? 'good' : '') : '';
    return `<div class="sig"><div class="k">${esc(c.k)}</div><div class="v">${c.show(v)}</div><div class="d"><span class="delta ${tone}">${d >= 0 ? '▲' : '▼'} ${Math.abs(d * 100).toFixed(1)}%</span> <span class="mut">vs range average</span></div><div class="x">${esc(c.x)}</div>${spark(c.vals, { zero: c.zero, color: tone === 'bad' ? 'var(--c5)' : 'var(--brand)' })}</div>`;
  }).join('');
}

export function ovShell() {
  return `<div class="stack">
    <section id="banner" aria-label="Status"></section>
    <section class="card" aria-labelledby="hFlow"><div class="card-h"><div><h2 id="hFlow">Traffic flow</h2><p class="sub">Requests per second from listener to route to upstream pool. Colored ribbons carry elevated error rates.</p></div></div><div class="flow-scroll" id="flow"></div><p class="hint">Swipe sideways to see the full diagram.</p></section>
    <section class="signals" id="signals" aria-label="Key signals"></section>
    <div class="two">
      <section class="card"><div class="card-h"><div><h2>Latency</h2><p class="sub">Percentiles across all routes, weighted by traffic</p></div></div><div class="card-b chart" id="chLat"></div></section>
      <section class="card"><div class="card-h"><div><h2>Errors</h2><p class="sub">4xx and 5xx responses per second</p></div></div><div class="card-b chart" id="chErr"></div></section>
    </div>
    <div class="three">
      <section class="card"><div class="card-h"><div><h2>Upstream pools</h2></div><button class="linkbtn" data-go="upstreams">View all</button></div><ul class="rows pr" id="poolRows" style="margin-top:8px"></ul></section>
      <section class="card"><div class="card-h"><div><h2>Active alerts</h2></div><button class="linkbtn" data-go="alerts">View all</button></div><ul class="rows al" id="alertRows" style="margin-top:8px"></ul></section>
      <section class="card"><div class="card-h"><div><h2>Certificates</h2><p class="sub">Soonest to expire</p></div><button class="linkbtn" data-go="certs">View all</button></div><ul class="rows ce" id="certRows" style="margin-top:8px"></ul></section>
    </div></div>`;
}

export function ovUpdate(D) {
  const bEl = $('#banner');
  if (bEl) bEl.innerHTML = bannerHTML(D);
  flow($('#flow'), D);
  signals($('#signals'), D);
  chart($('#chLat'), {
    id: 'lat',
    height: 240,
    labels: D.labels,
    fmt: fmt.ms,
    aria: 'Latency percentiles over time',
    series: [
      { key: 'p50', label: 'p50', color: 'var(--s1)', values: D.A.p50 },
      { key: 'p95', label: 'p95', color: 'var(--s2)', values: D.A.p95 },
      { key: 'p99', label: 'p99', color: 'var(--s3)', values: D.A.p99 }
    ]
  });
  chart($('#chErr'), {
    id: 'err',
    height: 240,
    bars: true,
    stacked: true,
    labels: D.labels,
    fmt: v => fmt.n(v) + '/s',
    aria: '4xx and 5xx responses per second',
    series: [
      { key: 'e4', label: '4xx', color: 'var(--c4)', values: D.A.e4 },
      { key: 'e5', label: '5xx', color: 'var(--c5)', values: D.A.e5 }
    ]
  });
  const prEl = $('#poolRows');
  if (prEl) {
    prEl.innerHTML = `<li class="hd"><span>Healthy</span><span>Pool / Route</span><span class="r">p95</span><span class="r">5xx</span></li>` +
      D.pools.map(p => `<li data-go="upstreams:${p.id}" tabindex="0" role="link"><span>${pill(p.tone, `${p.up}/${p.insts.length}`)}</span><b>${esc(p.displayName || p.id)}</b><span class="mut num r">${fmt.ms(p.p95)}</span><span class="num r ${p.err5 >= .02 ? 't-err' : ''}">${fmt.pct(p.err5, 1)}</span></li>`).join('');
  }
  const alEl = $('#alertRows');
  if (alEl) {
    alEl.innerHTML = (D.alerts || []).slice(0, 4).map(a => `<li data-go="${a.go}" tabindex="0" role="link"><span class="${a.sev === 'critical' ? 't-err' : 't-warn'}">${ICON(a.sev === 'critical' ? 'i-x' : 'i-alert')}</span><div><div class="al-t">${esc(a.title)}</div><div class="al-d">${esc(a.detail(D))}</div></div><span class="mut num" style="white-space:nowrap;font-size:11.5px">${dtFmt(a.timestamp)}</span></li>`).join('') ||
      `<li><span class="t-ok">${ICON('i-check')}</span><div><div class="al-t">Zero Active Incidents</div></div></li>`;
  }
  const crEl = $('#certRows');
  if (crEl) {
    crEl.innerHTML = D.certs.slice(0, 4).map(c => `<li data-go="certs" tabindex="0" role="link"><div><div class="al-t">${esc(c.domain)}</div><div class="bar"><i style="width:${clamp(c.days / 90 * 100, 5, 100)}%;background:var(--ok)"></i></div></div><span class="num ${c.state === 'failing' ? 't-err' : 'mut'}">${c.days} days</span>${pill(c.state === 'failing' ? 'err' : 'ok', c.state === 'failing' ? 'Failing' : 'Valid')}</li>`).join('');
  }
}
