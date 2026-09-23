/**
 * Toron Dashboard - Vector SVG Sankey Traffic Flow Visualizer
 */
import { sum, fmt, esc } from '../utils.js';
import { isForce } from '../state.js';
import { toneErr, TONE } from './icons.js';

export function flow(host, D) {
  if (!host || (!isForce() && host.matches && host.matches(':hover'))) return;
  if (!D || !D.routes || D.routes.length === 0) {
    host.innerHTML = `<div class="empty" style="padding:40px;text-align:center;color:var(--ink-3)">No active traffic routes configured.</div>`;
    return;
  }

  const W = 1160, top = 34, bw = 8, gap = 12, minH = 22;
  const X = [170, 530, 910];

  const routeNodes = D.routes.map(r => ({
    id: r.id,
    label: r.short,
    pool: r.pool,
    v: (r.cur && r.cur.rps) || 0,
    err5: (r.cur && r.cur.err5) || 0,
    rids: [r.id],
    tone: toneErr((r.cur && r.cur.err5) || 0),
    meta: `${fmt.n((r.cur && r.cur.rps) || 0)}/s · ${(((r.cur && r.cur.err5) || 0) * 100).toFixed(1)}% 5xx`,
    go: `routes:${r.id}`
  }));

  const redirVal = D.redir || 0;
  const redirNode = {
    id: 'redir',
    label: '301 to HTTPS',
    pool: 'in-process',
    v: redirVal,
    err5: 0,
    rids: ['redir'],
    tone: 'ok',
    meta: `${fmt.n(redirVal)}/s · Redirect`,
    go: ''
  };

  const col1Nodes = [...routeNodes, redirNode];
  const totalCol1Val = sum(col1Nodes.map(n => n.v)) || 0;

  // Compute Col 1 Node Heights
  col1Nodes.forEach(n => {
    const extra = totalCol1Val > 0 ? (n.v / totalCol1Val) * 90 : 0;
    n.h = Math.max(minH, Math.min(64, minH + extra));
  });
  const col1H = sum(col1Nodes.map(n => n.h)) + (col1Nodes.length - 1) * gap;
  const Hh = Math.max(380, top + col1H + 34);

  // Position Col 1
  let y1 = top + Math.max(0, (Hh - top - 34 - col1H) / 2);
  col1Nodes.forEach(n => { n.y = y1; n.oy = 0; n.iy = 0; y1 += n.h + gap; });

  // Col 0: Listeners (:443 HTTPS and :80 HTTP)
  const l443_v = Math.max(0, sum(routeNodes.map(r => r.v)));
  const l443_h = Math.max(minH * 2, Math.min(col1H - redirNode.h - gap, sum(routeNodes.map(r => r.h))));
  const l80_h = Math.max(minH, redirNode.h);
  const col0H = l443_h + gap + l80_h;
  let y0 = top + Math.max(0, (Hh - top - 34 - col0H) / 2);

  const l443 = {
    id: 'l443',
    label: ':443 HTTPS',
    v: l443_v,
    h: l443_h,
    y: y0,
    oy: 0,
    iy: 0,
    rids: D.routes.map(r => r.id),
    tone: 'ok',
    meta: `${fmt.n(l443_v)}/s`
  };
  const l80 = {
    id: 'l80',
    label: ':80 HTTP',
    v: redirVal,
    h: l80_h,
    y: y0 + l443_h + gap,
    oy: 0,
    iy: 0,
    rids: ['redir'],
    tone: 'warn',
    meta: `${fmt.n(redirVal)}/s`
  };
  const col0Nodes = [l443, l80];

  // Col 2: Upstream Pools
  const poolMap = new Map();
  const rawPools = (D.pools && D.pools.length > 0) ? D.pools : [{ id: 'in-process', rps: 0, tone: 'ok', routes: [] }];
  const poolNodes = rawPools.map(p => {
    const matchingRoutes = routeNodes.filter(r => r.id === p.id || (p.routes && p.routes.some(pr => pr.id === r.id)) || (r.pool && (r.pool === p.id || r.pool.includes(p.id) || p.id.includes(r.pool))));
    const rps = (p.rps !== undefined && p.rps > 0) ? p.rps : (sum(matchingRoutes.map(r => r.v)) || 0);
    const tone = (p.tone && p.tone !== 'ok') ? p.tone : (matchingRoutes.some(r => r.tone === 'err') ? 'err' : (matchingRoutes.some(r => r.tone === 'warn') ? 'warn' : 'ok'));
    const rids = matchingRoutes.map(r => r.id);
    return {
      id: p.id,
      label: p.displayName || p.id,
      v: rps,
      routes: matchingRoutes,
      rids: rids.length > 0 ? rids : [p.id],
      tone,
      meta: `${fmt.n(rps)}/s`,
      go: `upstreams:${p.id}`
    };
  });

  poolNodes.forEach(p => {
    poolMap.set(p.id, p);
    const routeHSum = sum(p.routes.map(r => r.h * 0.85));
    p.h = Math.max(minH, Math.min(80, routeHSum || minH));
  });
  const col2H = sum(poolNodes.map(p => p.h)) + (poolNodes.length - 1) * gap;
  let y2 = top + Math.max(0, (Hh - top - 34 - col2H) / 2);
  poolNodes.forEach(p => { p.y = y2; p.oy = 0; p.iy = 0; y2 += p.h + gap; });

  // Build Links
  const links = [];
  const routeSumV = sum(routeNodes.map(r => r.v)) || 0;

  // 1. Listeners -> Routes
  routeNodes.forEach(r => {
    const fraction = routeSumV > 0 ? (r.v / routeSumV) : (1 / routeNodes.length);
    const ts = Math.max(3, l443.h * fraction);
    const tt = r.h;
    const sy = l443.y + l443.oy;
    const ty = r.y;
    l443.oy += ts;
    links.push({
      sX: X[0] + bw,
      tX: X[1],
      sY: sy,
      tY: ty,
      sH: ts,
      tH: tt,
      rid: r.id,
      tone: r.tone,
      lab: `${r.label} (Listener → Route)`,
      v: r.v
    });
  });

  // Redirect Link (:80 -> 301 to HTTPS)
  links.push({
    sX: X[0] + bw,
    tX: X[1],
    sY: l80.y,
    tY: redirNode.y,
    sH: l80.h,
    tH: redirNode.h,
    rid: 'redir',
    tone: 'warn',
    lab: '301 to HTTPS',
    v: redirVal
  });

  // 2. Routes -> Upstream Pools
  routeNodes.forEach(r => {
    let targetPool = poolMap.get(r.id) || poolMap.get(r.pool) || poolNodes.find(p => p.id === r.id || (p.routes && p.routes.some(pr => pr.id === r.id)) || (r.pool && (r.pool.includes(p.id) || p.id.includes(r.pool)))) || poolNodes[0];
    if (!targetPool && poolNodes.length > 0) targetPool = poolNodes[0];
    if (targetPool) {
      const poolRoutes = targetPool.routes || [];
      const poolRoutesV = sum(poolRoutes.map(x => x.v)) || 0;
      const fraction = poolRoutesV > 0 ? (r.v / poolRoutesV) : (1 / Math.max(1, poolRoutes.length));
      const ts = r.h;
      const tt = Math.max(3, targetPool.h * fraction);
      const sy = r.y;
      const ty = targetPool.y + targetPool.iy;
      targetPool.iy += tt;
      links.push({
        sX: X[1] + bw,
        tX: X[2],
        sY: sy,
        tY: ty,
        sH: ts,
        tH: tt,
        rid: r.id,
        tone: r.tone,
        lab: `${r.label} → ${targetPool.label}`,
        v: r.v
      });
    }
  });

  // Render SVG Elements
  let paths = '', nodes = '', labels = '';

  links.forEach(l => {
    const mx = (l.sX + l.tX) / 2;
    const y0 = l.sY, y0b = l.sY + l.sH;
    const y1 = l.tY, y1b = l.tY + l.tH;
    const color = TONE[l.tone] || 'var(--flow-ok)';
    paths += `<path class="rb ${l.tone}" data-r="${l.rid}" fill="${color}" d="M${l.sX},${y0.toFixed(1)} C${mx},${y0.toFixed(1)} ${mx},${y1.toFixed(1)} ${l.tX},${y1.toFixed(1)} L${l.tX},${y1b.toFixed(1)} C${mx},${y1b.toFixed(1)} ${mx},${y0b.toFixed(1)} ${l.sX},${y0b.toFixed(1)} Z"><title>${esc(l.lab)}: ${fmt.n(l.v)} req/s</title></path>`;
  });

  const allColumns = [
    { nodes: col0Nodes, x: X[0], align: 'end', labelX: X[0] - 12 },
    { nodes: col1Nodes, x: X[1], align: 'start', labelX: X[1] + bw + 12 },
    { nodes: poolNodes, x: X[2], align: 'start', labelX: X[2] + bw + 12 }
  ];

  allColumns.forEach((c, ci) => {
    c.nodes.forEach(n => {
      const go = n.go ? ` data-go="${n.go}" tabindex="0" role="link" aria-label="${esc(n.label)}, ${esc(n.meta || '')}"` : '';
      const nodeFill = ci === 0 ? (n.id === 'l80' ? 'var(--warn)' : 'var(--brand)') : (TONE[n.tone] || 'var(--flow-ok)');
      nodes += `<rect class="nd ${n.tone || 'ok'}" data-r="${n.rids.join(' ')}"${go} x="${c.x}" y="${n.y.toFixed(1)}" width="${bw}" height="${n.h.toFixed(1)}" rx="3" fill="${nodeFill}"><title>${esc(n.label)}${n.meta ? ': ' + esc(n.meta) : ''}</title></rect>`;

      const ly = (n.y + n.h / 2).toFixed(1);
      if (c.align === 'end') {
        labels += `<text class="lb" data-r="${n.rids.join(' ')}" x="${c.labelX}" y="${ly}" text-anchor="end" dominant-baseline="central"><tspan class="t">${esc(n.label)}</tspan><tspan class="m" dx="8">${esc(n.meta)}</tspan></text>`;
      } else {
        const metaToneClass = (n.tone && n.tone !== 'ok') ? ` ${n.tone}` : '';
        labels += `<text class="lb" data-r="${n.rids.join(' ')}" x="${c.labelX}" y="${ly}" dominant-baseline="central"><tspan class="t">${esc(n.label)}</tspan><tspan class="m${metaToneClass}" dx="8">${esc(n.meta)}</tspan></text>`;
      }
    });
  });

  const heads = `
    <text class="ch" x="${X[0] - 12}" y="16" text-anchor="end">LISTENERS</text>
    <text class="ch" x="${X[1]}" y="16">ROUTES</text>
    <text class="ch" x="${X[2]}" y="16">UPSTREAM POOLS</text>
  `;

  host.innerHTML = `<svg class="flow" viewBox="0 0 ${W} ${Hh}" role="img" aria-label="Traffic flow from listeners through routes to upstream pools.">${heads}<g class="links-g">${paths}</g><g class="nodes-g">${nodes}</g><g class="labels-g">${labels}</g></svg>`;

  const svg = host.querySelector('svg.flow');
  if (!svg) return;
  svg.onpointerover = e => {
    const t = e.target.closest('[data-r]'); if (!t) return;
    const ids = t.dataset.r.split(' ');
    svg.classList.add('hl');
    svg.querySelectorAll('[data-r]').forEach(el => el.classList.toggle('on', el.dataset.r.split(' ').some(id => ids.includes(id))));
  };
  svg.onpointerleave = () => {
    svg.classList.remove('hl');
    svg.querySelectorAll('[data-r].on').forEach(el => el.classList.remove('on'));
  };
}
