/**
 * Toron Dashboard - SVG Charts, Sparklines & Histograms Engine
 */
import { sum, clamp, axis, esc } from '../utils.js';
import { isForce } from '../state.js';

const hidden = {};

export function niceMax(v) {
  const p = Math.pow(10, Math.floor(Math.log10(v || 1)));
  const f = (v || 1) / p;
  return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 4 ? 4 : f <= 8 ? 8 : 10) * p;
}

export function spark(vals, o = {}) {
  if (!vals || vals.length === 0) return '';
  const w = 100, h = o.h || 34, n = vals.length;
  const lo = o.zero ? 0 : Math.min(...vals);
  const hi = Math.max(...vals);
  const r = (hi - lo) || 1;
  const pts = vals.map((v, i) => `${((n > 1 ? i / (n - 1) : 0) * w).toFixed(2)},${(h - 2 - (v - lo) / r * (h - 5)).toFixed(2)}`);
  const col = o.color || 'var(--brand)';
  return `<svg class="spark" viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" aria-hidden="true"><path d="M0,${h} L${pts.join(' L')} L${w},${h}Z" fill="${col}" opacity=".13"/><polyline points="${pts.join(' ')}" fill="none" stroke="${col}" stroke-width="1.5" stroke-linejoin="round" vector-effect="non-scaling-stroke"/></svg>`;
}

export function chart(host, cfg, force) {
  if (!host) return;
  if (!force && !isForce() && host.matches && host.matches(':hover')) return;
  host._cfg = cfg;
  const W = Math.max(260, host.clientWidth || 600), Hh = cfg.height || 220;
  const m = { l: cfg.ml || 44, r: 10, t: 10, b: 24 }, iw = W - m.l - m.r, ih = Hh - m.t - m.b;
  const off = hidden[cfg.id] || (hidden[cfg.id] = new Set());
  const vis = cfg.series.filter(s => !off.has(s.key)), n = (cfg.labels || []).length;
  if (n === 0) return;

  let top = 0;
  if (cfg.stacked) {
    for (let i = 0; i < n; i++) top = Math.max(top, sum(vis.map(s => s.values[i])));
  } else {
    vis.forEach(s => s.values.forEach(v => { if (v > top) top = v; }));
  }
  (cfg.refs || []).forEach(r => { top = Math.max(top, r.y * 1.15); });
  const max = niceMax((top || 1) * 1.06);
  const x = i => m.l + (cfg.bars ? (i + .5) / n * iw : (n > 1 ? i / (n - 1) * iw : 0));
  const y = v => m.t + ih - ((v || 0) / max) * ih;

  let g = '';
  for (let k = 0; k <= 4; k++) {
    const yy = m.t + ih - ih * k / 4;
    g += `<line class="gl" x1="${m.l}" x2="${W - m.r}" y1="${yy}" y2="${yy}"/><text class="ax" x="${m.l - 8}" y="${yy + 4}" text-anchor="end">${axis(max * k / 4)}</text>`;
  }
  for (let k = 0; k < 5; k++) {
    const i = Math.round(k * (n - 1) / 4);
    g += `<text class="ax" x="${x(i)}" y="${Hh - 6}" text-anchor="${k === 0 ? 'start' : k === 4 ? 'end' : 'middle'}">${cfg.labels[i] || ''}</text>`;
  }
  (cfg.refs || []).forEach(r => {
    const yy = y(r.y);
    g += `<line class="ref" x1="${m.l}" x2="${W - m.r}" y1="${yy}" y2="${yy}" stroke="${r.color || 'var(--warn)'}"/><text class="reflb" x="${W - m.r - 4}" y="${yy - 5}" text-anchor="end" fill="${r.color || 'var(--warn)'}">${esc(r.label)}</text>`;
  });

  if (cfg.bars) {
    const bw = Math.max(2, iw / n * .68);
    for (let i = 0; i < n; i++) {
      let acc = 0;
      vis.forEach(s => {
        const v = s.values[i] || 0, y0 = y(acc), y1 = y(acc + v);
        g += `<rect x="${(x(i) - bw / 2).toFixed(1)}" y="${y1.toFixed(1)}" width="${bw.toFixed(1)}" height="${Math.max(0, y0 - y1).toFixed(1)}" fill="${s.color}"/>`;
        acc += v;
      });
    }
  } else {
    vis.forEach(s => {
      const d = s.values.map((v, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join('');
      if (s.area) g += `<path d="${d}L${x(n - 1).toFixed(1)},${y(0)}L${x(0).toFixed(1)},${y(0)}Z" fill="${s.color}" opacity=".12"/>`;
      g += `<path d="${d}" fill="none" stroke="${s.color}" stroke-width="${s.w || 1.75}" stroke-linejoin="round"/>`;
    });
  }

  g += `<g class="cur" style="display:none"></g><rect class="hit" x="${m.l}" y="${m.t}" width="${iw}" height="${ih}"/>`;
  const legend = cfg.series.length > 1
    ? `<div class="legend">${cfg.series.map(s => `<button class="lg" data-k="${s.key}" aria-pressed="${!off.has(s.key)}"><i style="background:${s.color}"></i>${esc(s.label)}</button>`).join('')}</div>`
    : '';
  host.innerHTML = `${legend}<div class="cwrap"><svg viewBox="0 0 ${W} ${Hh}" role="img" aria-label="${esc(cfg.aria || 'Chart')}">${g}</svg><div class="tip" hidden></div></div>`;

  const svg = host.querySelector('svg');
  const tip = host.querySelector('.tip');
  const cur = host.querySelector('.cur');
  if (!svg || !tip || !cur) return;

  const move = ev => {
    const r = svg.getBoundingClientRect(), sx = r.width / W, px = (ev.clientX - r.left) / sx;
    let i = cfg.bars ? Math.floor((px - m.l) / iw * n) : Math.round((px - m.l) / iw * (n - 1));
    i = clamp(i, 0, n - 1);
    let c = '';
    if (cfg.bars) {
      c = `<rect class="col" x="${x(i) - iw / n / 2}" y="${m.t}" width="${iw / n}" height="${ih}"/>`;
    } else {
      c = `<line class="xl" x1="${x(i)}" x2="${x(i)}" y1="${m.t}" y2="${m.t + ih}"/>` +
          vis.map(s => `<circle cx="${x(i)}" cy="${y(s.values[i])}" r="3.5" fill="${s.color}" stroke="var(--surface)" stroke-width="1.5"/>`).join('');
    }
    cur.innerHTML = c;
    cur.style.display = '';
    let rows = vis.slice().reverse().map(s => `<div><i style="background:${s.color}"></i><span>${esc(s.label)}</span><em>${cfg.fmt(s.values[i])}</em></div>`).join('');
    if (cfg.stacked && vis.length > 1) {
      rows += `<div><i style="background:transparent"></i><span>Total</span><em>${cfg.fmt(sum(vis.map(s => s.values[i])))}</em></div>`;
    }
    tip.hidden = false;
    tip.innerHTML = `<b>${cfg.labels[i]}</b>${rows}`;
    const tw = tip.offsetWidth, px2 = x(i) * sx;
    let left = px2 + 14;
    if (left + tw > r.width) left = px2 - tw - 14;
    tip.style.left = Math.max(0, left) + 'px';
  };

  svg.onpointermove = move;
  svg.onpointerdown = move;
  svg.onpointerleave = () => { cur.style.display = 'none'; tip.hidden = true; };
  host.onclick = e => {
    const b = e.target.closest('.lg');
    if (!b) return;
    const k = b.dataset.k;
    if (off.has(k)) off.delete(k);
    else if (vis.length > 1) off.add(k);
    chart(host, host._cfg, true);
  };
}

export function histo(med, p95) {
  const labels = ['< 10ms', '10-25', '25-50', '50-100', '100-250', '250-500', '> 500'];
  return [10, 25, 50, 100, 250, 500, Infinity].map((e, i) => ({
    l: labels[i],
    v: 10 + (i === 2 ? 40 : i === 1 ? 25 : 5)
  }));
}
