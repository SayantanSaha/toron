import { $, esc, fmt, avg } from '../utils.js';
import { pill } from '../components/icons.js';
import { chart } from '../components/charts.js';

export function mdShell() {
  return `<div class="stack">
    <section class="card"><div class="card-h"><div><h2>Compiled Engine Modules</h2><p class="sub">Non-blocking modular reactors operating concurrently on the event loop.</p></div></div><div class="tscroll" style="margin-top:8px"><table class="tbl"><thead><tr><th>Module</th><th>State</th><th class="num">Events/s</th><th class="hide-md">Notes</th></tr></thead><tbody id="mdBody"></tbody></table></div></section>
    <div class="two">
      <section class="card mods"><div class="card-h"><div><h2>Event Bus Throughput<span class="cv" id="cvEv"></span></h2></div></div><div class="card-b chart" id="chEv"></div></section>
      <section class="card mods"><div class="card-h"><div><h2>Process CPU<span class="cv" id="cvCpu"></span></h2></div></div><div class="card-b chart" id="chCpu"></div></section>
    </div>
    <div class="four">
      <section class="card mods"><div class="card-h"><h2>Goroutines<span class="cv" id="cvGor"></span></h2></div><div class="card-b chart" id="chGor"></div></section>
      <section class="card mods"><div class="card-h"><h2>Heap in Use<span class="cv" id="cvHeap"></span></h2></div><div class="card-b chart" id="chHeap"></div></section>
      <section class="card mods"><div class="card-h"><h2>GC Pause p99<span class="cv" id="cvGc"></span></h2></div><div class="card-b chart" id="chGc"></div></section>
      <section class="card mods"><div class="card-h"><h2>File Descriptors<span class="cv" id="cvFd"></span></h2></div><div class="card-b chart" id="chFd"></div></section>
    </div></div>`;
}

export function mdUpdate(D) {
  const X = D.X, last = a => a?.length ? a[a.length - 1] : 0, ev = avg(X.ev), mBody = $('#mdBody');
  if (mBody) {
    mBody.innerHTML = D.modules.map(m => `<tr><td><div class="rt"><b>${esc(m.id)}</b><span class="mut">${esc(m.kind)}</span></div></td><td>${m.state === 'running' ? pill('ok', 'Running') : pill('warn', 'Degraded')}</td><td class="num">${fmt.n(ev * (m.w || 0.1))}/s</td><td class="hide-md mut">${esc(m.note)}</td></tr>`).join('');
  }
  const one = (id, key, vals, f, extra = {}) => chart($('#' + id), {
    id, height: extra.h || 160, ml: extra.ml || 44, labels: D.labels, fmt: f, aria: key,
    series: [{ key, label: key, color: extra.c || 'var(--s2)', values: vals, area: true }]
  });

  const sets = [
    ['cvEv', `${fmt.n(ev)}/s`, 'chEv', 'Events/s', X.ev, v => `${fmt.n(v)}/s`, { h: 200 }],
    ['cvCpu', `${last(X.cpu).toFixed(0)}%`, 'chCpu', 'CPU', X.cpu, v => `${v.toFixed(0)}%`, { h: 200, c: 'var(--s1)' }],
    ['cvGor', Math.round(last(X.gor)).toLocaleString('en-US'), 'chGor', 'Goroutines', X.gor, v => Math.round(v).toLocaleString('en-US')],
    ['cvHeap', `${Math.round(last(X.heap))} MB`, 'chHeap', 'Heap', X.heap, v => `${Math.round(v)} MB`],
    ['cvGc', `${last(X.gc).toFixed(2)} ms`, 'chGc', 'GC', X.gc, v => `${v.toFixed(2)} ms`],
    ['cvFd', Math.round(last(X.fd)).toLocaleString('en-US'), 'chFd', 'FDs', X.fd, v => Math.round(v).toLocaleString('en-US')]
  ];
  for (const [cvId, cvTxt, chId, k, vals, f, opt] of sets) {
    const el = $('#' + cvId); if (el) el.textContent = cvTxt;
    one(chId, k, vals, f, opt);
  }
}
