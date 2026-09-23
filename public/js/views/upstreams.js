/**
 * Toron Dashboard - View 3: Upstreams & Health Checks
 */
import { $, esc, fmt } from '../utils.js';
import { pill } from '../components/icons.js';

export function poolCard(p) {
  const rows = p.insts.map(i => {
    const st = i.state || 'up', tone = st === 'down' ? 'err' : st === 'draining' ? 'mute' : (st === 'warn' ? 'warn' : 'ok');
    const label = st === 'down' ? 'Down' : st === 'draining' ? 'Draining' : (st === 'warn' ? (i.httpCode ? `Degraded (HTTP ${i.httpCode})` : 'Degraded') : (i.httpCode ? `Healthy (HTTP ${i.httpCode})` : 'Healthy'));
    const history = i.history && i.history.length ? i.history : Array(48).fill(st !== 'down');
    const fails = history.filter(v => !v).length;
    const strip = `<svg class="hc" viewBox="0 0 96 12" preserveAspectRatio="none" role="img" aria-label="Last 48 health checks, ${fails} failed">${history.map((ok, k) => `<rect class="${ok ? 'g' : 'b'}" x="${k * 2}" y="0" width="1.4" height="12" rx=".5"/>`).join('')}</svg>`;

    return `<div class="inst"><div class="inst-top">
      <div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap">
        <code style="font-weight:600;font-size:12.5px">${esc(i.a)}</code>
        ${i.route && i.route !== p.displayName ? `<span class="mut" style="font-size:11px">(${esc(i.route)})</span>` : ''}
      </div>
      ${pill(tone, label)}</div>
      <div class="inst-m">
        <span>Latency: <b>${(i.latency && i.latency > 0) ? i.latency.toFixed(1) + ' ms' : fmt.ms(p.p95)}</b></span>
        <span>Probe Status: ${st === 'down' ? '<b style="color:var(--err)">Unreachable</b>' : (st === 'warn' ? '<b style="color:var(--warn)">Degraded</b>' : '<b style="color:var(--ok)">HTTP 200 OK</b>')}</span>
      </div>${strip}
      ${st === 'down' ? `<p class="note err">Circuit breaker open. Connections routed away from failing node.</p>` : ''}
      ${st === 'draining' ? `<p class="note mute">Node is draining connections for deployment.</p>` : ''}</div>`;
  }).join('');

  const hostLabel = (p.host && p.host !== '*') ? p.host : 'Any Host (*)';
  const prefixLabel = p.path || '/';
  const mwList = (p.routes && p.routes.length > 0 && p.routes[0].mw) ? p.routes[0].mw : [];

  return `<section class="card" id="pool-${esc(p.id)}"><div class="card-h">
    <div>
      <h2>${esc(p.displayName || p.id)}</h2>
      <p class="sub"><span><b>Match:</b> <code>${esc(hostLabel)}</code> <code>${esc(prefixLabel)}</code></span> · <span><b>LB Algorithm:</b> ${esc(p.algo)}</span></p>
    </div>
    ${pill(p.tone, `${p.up} of ${p.insts.length} up`)}</div>
    <div style="padding:10px 16px;background:var(--surface-2);border-top:1px solid var(--line);border-bottom:1px solid var(--line);display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:8px;font-size:12px">
      <div style="display:flex;gap:12px;align-items:center;flex-wrap:wrap">
        <div><span class="mut">Host: </span><b>${esc(hostLabel)}</b></div>
        <div><span class="mut">Prefix: </span><b>${esc(prefixLabel)}</b></div>
        ${p.headersSummary ? `<div><span class="mut">Headers: </span><code>${esc(p.headersSummary)}</code></div>` : ''}
      </div>
      ${mwList.length > 0 ? `<div class="chips">${mwList.slice(0, 3).map(m => `<span class="chipx">${esc(m)}</span>`).join('')}</div>` : ''}
    </div>
    <dl class="pool-sum"><div><dt>Requests</dt><dd>${fmt.n(p.rps)}/s ${p.totalReqs > 0 ? `<span class="mut" style="font-size:11px;font-weight:400">(${fmt.n(p.totalReqs)} total)</span>` : ''}</dd></div><div><dt>p95 Latency</dt><dd>${fmt.ms(p.p95)}</dd></div><div><dt>5xx Error Rate</dt><dd class="${p.err5 >= .02 ? 't-err' : ''}">${fmt.pct(p.err5, 1)}</dd></div></dl>
    <div style="margin-top:4px">${rows}</div></section>`;
}

export function upShell() {
  return `<div class="pools" id="poolGrid"></div><p class="hc-legend">Each tick is a health check probe (oldest left, newest right). Red ticks failed.</p>`;
}

export function upUpdate(D) {
  const el = $('#poolGrid');
  if (el) el.innerHTML = D.pools.map(poolCard).join('');
}
