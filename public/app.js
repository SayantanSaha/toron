(()=>{
'use strict';

/* =====================================================================
   Toron Edge Gateway · High-Density Observability Control Center
   Zero External Dependencies · Native SVG Graphics & Tracing
   ===================================================================== */

/* ---------- DOM & string utilities ---------- */
const $=(s,r=document)=>r.querySelector(s), $$=(s,r=document)=>[...r.querySelectorAll(s)];
const esc=s=>String(s==null?'':s).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const clamp=(v,a,b)=>Math.min(b,Math.max(a,v));
const sum=a=>(a||[]).reduce((x,y)=>x+y,0), avg=a=>(a&&a.length)?sum(a)/a.length:0;
const p2=n=>String(n).padStart(2,'0');
const hms=d=>`${p2(d.getHours())}:${p2(d.getMinutes())}:${p2(d.getSeconds())}`;
const hm=d=>`${p2(d.getHours())}:${p2(d.getMinutes())}`;
const dtFmt=d=>{if(!d)return'';const t=(d instanceof Date)?d:new Date(d);if(isNaN(t.getTime()))return String(d);return`${t.getFullYear()}-${p2(t.getMonth()+1)}-${p2(t.getDate())} ${p2(t.getHours())}:${p2(t.getMinutes())}:${p2(t.getSeconds())}`;};
const ago=s=>s<90?`${Math.round(s)} s`:s<5400?`${Math.round(s/60)} min`:s<129600?`${Math.round(s/3600)} h`:`${Math.round(s/86400)} d`;
const fmt={
  n:v=>v>=1e6?(v/1e6).toFixed(2)+'M':v>=1e4?(v/1e3).toFixed(1)+'k':v>=1e3?(v/1e3).toFixed(2)+'k':v>=100?String(Math.round(v)):v>=10?v.toFixed(0):v.toFixed(1),
  ms:v=>v>=1000?(v/1000).toFixed(2)+' s':v>=100?Math.round(v)+' ms':v>=10?v.toFixed(0)+' ms':v.toFixed(1)+' ms',
  pct:(v,d=2)=>(v*100).toFixed(d)+'%',
  bytes:b=>b>=1048576?(b/1048576).toFixed(1)+' MB':b>=1024?(b/1024).toFixed(1)+' KB':Math.round(b)+' B'
};
const axis=v=>v>=1000?(+(v/1000).toFixed(1))+'k':String(+v.toFixed(v<10?2:v<100?1:0));
const ICON=(id,c='ic')=>`<svg class="${c}" aria-hidden="true" focusable="false"><use href="#${id}"/></svg>`;
const TI={ok:'i-check',warn:'i-alert',err:'i-x',mute:'i-dash'};
const pill=(tone,text,title)=>`<span class="pill ${tone}"${title?` title="${esc(title)}"`:''}>${ICON(TI[tone]||'i-check')}${esc(text)}</span>`;
const TONE={ok:'var(--flow-ok)',warn:'var(--flow-warn)',err:'var(--flow-err)'};
const toneErr=e=>e>=.02?'err':e>=.012?'warn':'ok';

/* ---------- application state ---------- */
const N=60;
const RANGES={'15m':{bucket:15,label:'Last 15 minutes'},'1h':{bucket:60,label:'Last hour'},'6h':{bucket:360,label:'Last 6 hours'},'24h':{bucket:1440,label:'Last 24 hours'}};
const state={
  view:'overview',
  range:'1h',
  instance:'all',
  live:true,
  step:0,
  sort:{k:'rps',dir:-1},
  q:'',
  lq:'',
  lst:new Set(['2','3','4','5']),
  lroute:'all',
  lslow:false
};

let rawApiStatus = null;
let rawApiRoutes = [];
let rawApiUpstreams = [];
let rawApiIncidents = [];
let rawApiLogs = [];
let rawApiBannedIps = [];

let cache=null, FORCE=false;
const invalidate=()=>{cache=null;};

/* ---------- default seed mock data (used when server is idle) ---------- */
const H=(a,b=0)=>{const x=Math.sin(a*12.9898+b*78.233)*43758.5453;return x-Math.floor(x);};

/* ---------- DATA ADAPTER LAYER ---------- */
async function fetchBackendData() {
  try {
    const [statusRes, routesRes, upstreamsRes, incidentsRes, logsRes, bansRes] = await Promise.allSettled([
      fetch('/internal/api/status'),
      fetch('/internal/api/routes'),
      fetch('/internal/api/upstreams/health'),
      fetch('/internal/api/security/incidents'),
      fetch('/internal/api/logs'),
      fetch('/internal/api/security/banned-ips')
    ]);

    if (statusRes.status === 'fulfilled' && statusRes.value.ok) {
      rawApiStatus = await statusRes.value.json();
      if (rawApiStatus.version) {
        const eb = $('#envBadge');
        if (eb) eb.textContent = `v${rawApiStatus.version}`;
      }
    }
    if (routesRes.status === 'fulfilled' && routesRes.value.ok) {
      const data = await routesRes.value.json();
      rawApiRoutes = data.routes || [];
    }
    if (upstreamsRes.status === 'fulfilled' && upstreamsRes.value.ok) {
      const data = await upstreamsRes.value.json();
      rawApiUpstreams = data.upstreams || [];
    }
    if (incidentsRes.status === 'fulfilled' && incidentsRes.value.ok) {
      const data = await incidentsRes.value.json();
      rawApiIncidents = data.incidents || [];
    }
    if (logsRes.status === 'fulfilled' && logsRes.value.ok) {
      const data = await logsRes.value.json();
      rawApiLogs = data.logs || [];
    }
    if (bansRes.status === 'fulfilled' && bansRes.value.ok) {
      const data = await bansRes.value.json();
      rawApiBannedIps = data.banned_ips || [];
    }
    invalidate();
    refresh(false);
  } catch (err) {
    console.warn('Backend polling error:', err);
  }
}

function buildDataModel() {
  if (cache) return cache;
  const B = RANGES[state.range].bucket;

  // 1. Build routes list
  let routes = (rawApiRoutes && rawApiRoutes.length > 0) ? rawApiRoutes.map((r, idx) => {
    const id = (r.prefix || '/').replace(/[^a-zA-Z0-9]/g, '_') || `route_${idx}`;
    const short = (r.host ? r.host + ' ' : '') + r.prefix;
    const pool = (r.targets && r.targets.length > 0) ? r.targets[0] : (r.dir ? `static (${r.dir})` : 'in-process');

    let curRPS = 0;
    let lat = 2.5;
    let err = 0;
    if (rawApiStatus && rawApiStatus.metrics) {
      const m = rawApiStatus.metrics;
      if (m.by_route && m.by_route[r.prefix]) {
        curRPS = m.by_route[r.prefix];
      } else if (m.rate_per_sec) {
        curRPS = Math.round(m.rate_per_sec / rawApiRoutes.length);
      }
      if (m.latency_p95_ms > 0) lat = m.latency_p95_ms;
      if (m.err_rate > 0) err = m.err_rate;
    }

    const slo = Math.max(50, Math.round(lat * 2 + 20));
    const mw = [];
    if (r.type === 'static') mw.push('static-cache', 'gzip', 'rfc9111');
    else mw.push('waf-guard', 'cors', 'request-id', 'keep-alive');
    if (r.algorithm) mw.push(r.algorithm);
    if (r.headers && Object.keys(r.headers).length > 0) mw.push('header-match');

    const s = { rps: [], p50: [], p95: [], p99: [], e3: [], e4: [], e5: [] };
    if (rawApiStatus && rawApiStatus.timeseries && rawApiStatus.timeseries.A && rawApiStatus.timeseries.A.rps && rawApiStatus.timeseries.A.rps.length > 0) {
      const tsA = rawApiStatus.timeseries.A;
      const routeFactor = 1 / Math.max(1, rawApiRoutes.length);
      for (let i = 0; i < N; i++) {
        s.rps.push((tsA.rps[i] || 0) * routeFactor);
        s.p50.push(tsA.p50[i] || lat * 0.5);
        s.p95.push(tsA.p95[i] || lat);
        s.p99.push(tsA.p99[i] || lat * 1.5);
        s.e3.push((tsA.e3[i] || 0) * routeFactor);
        s.e4.push((tsA.e4[i] || 0) * routeFactor);
        s.e5.push((tsA.e5[i] || 0) * routeFactor);
      }
    } else {
      for (let i = 0; i < N; i++) {
        s.rps.push(curRPS);
        s.p50.push(lat * 0.5);
        s.p95.push(lat);
        s.p99.push(lat * 1.5);
        s.e3.push(0);
        s.e4.push(0);
        s.e5.push(0);
      }
    }
    return {
      id,
      short,
      host: r.host || '*',
      path: r.prefix,
      pool,
      type: r.type || 'proxy',
      algorithm: r.algorithm || 'Round-Robin',
      headers: r.headers || {},
      targets: r.targets || [],
      dir: r.dir || '',
      base: curRPS,
      lat,
      slo,
      err,
      mw,
      timeout: '10 s',
      s,
      cur: { rps: curRPS, p50: lat * 0.5, p95: lat, p99: lat * 1.5, e4: 0, e5: 0, err5: err, err4: 0 }
    };
  }) : [
    { id: 'orders', short: 'api /v1/orders', host: 'api.example.com', path: '/v1/orders/*', pool: 'orders-svc', base: 420, lat: 62, slo: 120, err: .004, mw: ['jwt', 'rate-limit 600/min', 'cors', 'request-id'], timeout: '8 s' },
    { id: 'auth', short: 'api /v1/auth', host: 'api.example.com', path: '/v1/auth/*', pool: 'auth-svc', base: 310, lat: 34, slo: 80, err: .002, mw: ['rate-limit 120/min', 'cors', 'request-id'], timeout: '4 s' },
    { id: 'search', short: 'api /v1/search', host: 'api.example.com', path: '/v1/search/*', pool: 'search-svc', base: 260, lat: 140, slo: 250, err: .009, mw: ['jwt', 'cache 30 s', 'cors'], timeout: '10 s' },
    { id: 'payments', short: 'api /v1/payments', host: 'api.example.com', path: '/v1/payments/*', pool: 'payments-svc', base: 95, lat: 210, slo: 300, err: .004, mw: ['jwt', 'rate-limit 60/min', 'retry ×1', 'request-id'], timeout: '12 s' },
    { id: 'app', short: 'app /', host: 'app.example.com', path: '/*', pool: 'web-static', base: 640, lat: 14, slo: 40, err: .001, mw: ['gzip', 'cache 5 min', 'security-headers'], timeout: '5 s' }
  ].map((rt, k) => {
    const s = { rps: [], p50: [], p95: [], p99: [], e3: [], e4: [], e5: [] };
    for (let i = 0; i < N; i++) {
      s.rps.push(rt.base);
      s.p50.push(rt.lat * .42);
      s.p95.push(rt.lat);
      s.p99.push(rt.lat * 2);
      s.e3.push(rt.base * .02);
      s.e4.push(rt.base * .01);
      s.e5.push(rt.base * rt.err);
    }
    return { ...rt, s, cur: { rps: rt.base, p50: rt.lat * .42, p95: rt.lat, p99: rt.lat * 2, e4: rt.base * .01, e5: rt.base * rt.err, err5: rt.err, err4: .01 } };
  });

  // 2. Build time series
  const now = Date.now();
  let labels = Array.from({ length: N }, (_, i) => {
    const d = new Date(now - (N - 1 - i) * B * 1000);
    return B < 60 ? hms(d) : hm(d);
  });

  let A = { rps: [], p50: [], p95: [], p99: [], e3: [], e4: [], e5: [], e2: [], err: [] };
  let X = { conns: [], egress: [], gor: [], heap: [], gc: [], fd: [], cpu: [], ev: [] };

  if (rawApiStatus && rawApiStatus.timeseries && rawApiStatus.timeseries.A && rawApiStatus.timeseries.A.rps && rawApiStatus.timeseries.A.rps.length > 0) {
    const tsA = rawApiStatus.timeseries.A;
    const tsX = rawApiStatus.timeseries.X || {};
    const tsLabels = rawApiStatus.timeseries.labels || [];
    const len = tsA.rps.length;

    if (len < N) {
      const padCount = N - len;
      const padZeros = Array(padCount).fill(0);
      const padP50 = Array(padCount).fill(tsA.p50[0] || 1);
      const padP95 = Array(padCount).fill(tsA.p95[0] || 2.5);
      const padP99 = Array(padCount).fill(tsA.p99[0] || 5);
      const padGor = Array(padCount).fill(tsX.gor ? tsX.gor[0] : 10);
      const padHeap = Array(padCount).fill(tsX.heap ? tsX.heap[0] : 1.5);
      const padCpu = Array(padCount).fill(tsX.cpu ? tsX.cpu[0] : 5);
      const padFd = Array(padCount).fill(tsX.fd ? tsX.fd[0] : 20);
      const padGc = Array(padCount).fill(tsX.gc ? tsX.gc[0] : 0.12);

      A.rps = [...padZeros, ...tsA.rps];
      A.p50 = [...padP50, ...tsA.p50];
      A.p95 = [...padP95, ...tsA.p95];
      A.p99 = [...padP99, ...tsA.p99];
      A.e2 = [...padZeros, ...(tsA.e2 || padZeros)];
      A.e3 = [...padZeros, ...(tsA.e3 || padZeros)];
      A.e4 = [...padZeros, ...(tsA.e4 || padZeros)];
      A.e5 = [...padZeros, ...(tsA.e5 || padZeros)];
      A.err = [...padZeros, ...(tsA.err || padZeros)];

      X.conns = [...padZeros, ...(tsX.conns || padZeros)];
      X.egress = [...padZeros, ...(tsX.egress || padZeros)];
      X.gor = [...padGor, ...(tsX.gor || padGor)];
      X.heap = [...padHeap, ...(tsX.heap || padHeap)];
      X.gc = [...padGc, ...(tsX.gc || padGc)];
      X.cpu = [...padCpu, ...(tsX.cpu || padCpu)];
      X.fd = [...padFd, ...(tsX.fd || padFd)];
      X.ev = [...padZeros, ...(tsX.ev || padZeros)];
    } else {
      A = tsA;
      X = tsX;
      if (tsLabels.length > 0) labels = tsLabels;
    }
  } else {
    for (let i = 0; i < N; i++) {
      const R = sum(routes.map(r => r.s.rps[i]));
      A.rps.push(R);
      A.p50.push(avg(routes.map(r => r.s.p50[i])));
      A.p95.push(avg(routes.map(r => r.s.p95[i])));
      A.p99.push(avg(routes.map(r => r.s.p99[i])));
      A.e3.push(sum(routes.map(r => r.s.e3[i])));
      A.e4.push(sum(routes.map(r => r.s.e4[i])));
      A.e5.push(sum(routes.map(r => r.s.e5[i])));
      A.e2.push(R - sum(routes.map(r => r.s.e4[i] + r.s.e5[i])));
      A.err.push(R > 0 ? sum(routes.map(r => r.s.e5[i])) / R : 0);

      X.conns.push(10);
      X.egress.push(R * 0.34);
      X.gor.push(20);
      X.heap.push(2);
      X.gc.push(0.12);
      X.fd.push(30);
      X.cpu.push(5);
      X.ev.push(R * 3.3);
    }
  }

  // 3. Build upstream pools with rich routing combinations
  const poolMap = new Map();
  if (rawApiUpstreams && rawApiUpstreams.length > 0) {
    rawApiUpstreams.forEach(u => {
      let targetAddr = u.name || `target:${u.port || 80}`;
      if (!targetAddr.includes(':') && u.port) {
        targetAddr = `${targetAddr}:${u.port}`;
      }
      targetAddr = targetAddr.replace(/:(\d+):\1$/, ':$1');

      // Find matching routes for this upstream node
      const matchingRoutes = routes.filter(r => {
        if (u.route && (r.short.includes(u.route) || (r.host + r.path).includes(u.route) || u.route.includes(r.path) || u.route.includes(r.short))) return true;
        if (r.pool && (r.pool.includes(targetAddr) || targetAddr.includes(r.pool) || r.pool.includes(u.name))) return true;
        if (r.targets && r.targets.some(t => t.includes(targetAddr) || targetAddr.includes(t) || t.includes(u.name))) return true;
        return false;
      });

      const primaryRoute = matchingRoutes[0];
      const host = primaryRoute ? (primaryRoute.host || '*') : '*';
      const path = primaryRoute ? primaryRoute.path : (u.route ? (u.route.includes('/') ? '/' + u.route.split('/').slice(1).join('/') : u.route) : '/');
      const displayName = primaryRoute ? primaryRoute.short : (u.route || targetAddr);
      const poolKey = primaryRoute ? primaryRoute.id : (u.route || targetAddr);

      if (!poolMap.has(poolKey)) {
        const headersList = (primaryRoute && primaryRoute.headers) ? Object.entries(primaryRoute.headers).map(([k, v]) => `${k}=${v}`).join(', ') : '';
        poolMap.set(poolKey, {
          id: poolKey,
          displayName,
          host,
          path,
          type: primaryRoute ? primaryRoute.type : 'proxy',
          algo: u.algo || (primaryRoute ? primaryRoute.algorithm : 'Round-Robin') || 'Round-Robin',
          hc: `HTTP probe on port ${u.port || 80}`,
          headersSummary: headersList,
          insts: [],
          routes: matchingRoutes.length > 0 ? matchingRoutes : (primaryRoute ? [primaryRoute] : [])
        });
      }

      const pool = poolMap.get(poolKey);
      matchingRoutes.forEach(mr => {
        if (!pool.routes.some(r => r.id === mr.id)) pool.routes.push(mr);
      });

      if (!pool.insts.some(i => i.a === targetAddr)) {
        pool.insts.push({
          a: targetAddr,
          route: u.route || (primaryRoute ? primaryRoute.short : ''),
          state: (u.status === 'HEALTHY' || u.status === 'UP') ? 'up' : 'down',
          httpCode: u.http_code,
          latency: u.latency_ms,
          history: u.history && u.history.length ? u.history : Array(48).fill(u.status === 'HEALTHY' || u.status === 'UP')
        });
      }
    });

    // Also register any configured routes not directly mapped
    routes.forEach(r => {
      let found = false;
      for (const p of poolMap.values()) {
        if (p.routes.some(pr => pr.id === r.id) || p.insts.some(i => r.pool.includes(i.a) || i.a.includes(r.pool))) {
          if (!p.routes.some(pr => pr.id === r.id)) p.routes.push(r);
          found = true;
        }
      }
      if (!found) {
        const poolKey = r.id || r.short;
        const headersList = r.headers ? Object.entries(r.headers).map(([k, v]) => `${k}=${v}`).join(', ') : '';
        poolMap.set(poolKey, {
          id: poolKey,
          displayName: r.short || poolKey,
          host: r.host || '*',
          path: r.path || '/',
          type: r.type || 'proxy',
          algo: r.algorithm || 'Round-Robin',
          hc: 'Passive health check',
          headersSummary: headersList,
          insts: [{ a: r.pool, route: r.short, state: 'up', history: Array(48).fill(true) }],
          routes: [r]
        });
      }
    });
  } else {
    routes.forEach(r => {
      const poolKey = r.id || r.short;
      const headersList = r.headers ? Object.entries(r.headers).map(([k, v]) => `${k}=${v}`).join(', ') : '';
      poolMap.set(poolKey, {
        id: poolKey,
        displayName: r.short || poolKey,
        host: r.host || '*',
        path: r.path || '/',
        type: r.type || 'proxy',
        algo: r.algorithm || 'Least connections',
        hc: 'GET /healthz every 5 s',
        headersSummary: headersList,
        insts: [{ a: r.pool, route: r.short, state: 'up', history: Array(48).fill(true) }],
        routes: [r]
      });
    });
  }

  const pools = Array.from(poolMap.values()).map(p => {
    const rs = p.routes;
    const rps = sum(rs.map(r => r.cur.rps));
    const p95 = avg(rs.map(r => r.cur.p95));
    const err5 = avg(rs.map(r => r.cur.err5));
    const insts = (p.insts && p.insts.length > 0) ? p.insts : [
      { a: `${p.id}`, state: 'up', history: Array(48).fill(true) }
    ];

    const up = insts.filter(i => i.state === 'up').length;
    const down = insts.filter(i => i.state === 'down').length;
    const tone = err5 >= 0.02 ? 'err' : (down ? 'warn' : 'ok');
    return { ...p, insts, rps, p95, err5, up, down, tone };
  });

  const total = sum(routes.map(r => r.cur.rps));
  const certs = (rawApiStatus && rawApiStatus.certs) || [
    { id: 'sayantansaha', domain: 'sayantansaha.in', chal: 'HTTP-01', days: 85, state: 'valid' },
    { id: 'toron', domain: 'toron.in', chal: 'HTTP-01', days: 85, state: 'valid' }
  ];

  const modules = (rawApiStatus && rawApiStatus.modules) || [
    { id: 'listener.http', kind: 'Listener', w: .18, state: 'running', note: 'Listening on :443 / :80' },
    { id: 'router', kind: 'Routing', w: .18, state: 'running', note: `${routes.length} routes active` },
    { id: 'waf.owasp', kind: 'Security', w: .09, state: 'running', note: 'OWASP Core Rules active' },
    { id: 'proxy.reverse', kind: 'Proxy', w: .2, state: 'running', note: 'Zero-allocation reverse proxy' }
  ];

  const alerts = [];

  // 1. Upstream pool health alerts
  pools.forEach(p => {
    if (p.down > 0) {
      alerts.push({
        id: `upstream_${p.id}`,
        sev: 'critical',
        title: `Upstream Degradation: ${p.displayName || p.id}`,
        timestamp: Date.now() - 60000,
        go: 'upstreams',
        detail: () => `${p.down} of ${p.insts.length} instances are failing health checks.`
      });
    }
  });

  // 2. High error rate route alerts
  routes.forEach(r => {
    if (r.cur.err5 >= 0.02) {
      alerts.push({
        id: `route_${r.id}`,
        sev: 'critical',
        title: `High 5xx Error Rate: ${r.short}`,
        timestamp: Date.now() - 60000,
        go: 'routes',
        detail: () => `5xx error rate (${fmt.pct(r.cur.err5, 1)}) exceeds 2% threshold.`
      });
    }
  });

  // 3. Certificate renewal alerts
  certs.forEach(c => {
    if (c.state === 'failing') {
      alerts.push({
        id: `cert_${c.id}`,
        sev: 'critical',
        title: `Certificate Renewal Failed: ${c.domain}`,
        timestamp: Date.now() - 300000,
        go: 'certs',
        detail: () => `Automated Let's Encrypt renewal failed for domain ${c.domain}.`
      });
    }
  });

  // 4. WAF Security Incidents
  if (rawApiIncidents && rawApiIncidents.length > 0) {
    rawApiIncidents.forEach((inc, i) => {
      const incTs = inc.timestamp ? new Date(inc.timestamp).getTime() : Date.now();
      alerts.push({
        id: `inc_${i}`,
        sev: 'warning',
        title: `WAF Security Anomaly: ${inc.category || inc.rule_id || 'Threat'} on ${inc.path}`,
        timestamp: incTs,
        go: 'alerts',
        detail: () => `Blocked malicious threat from client IP ${inc.client_ip || 'unknown'}`
      });
    });
  }

  // 5. Active Banned Threat Actors
  if (rawApiBannedIps && rawApiBannedIps.length > 0) {
    rawApiBannedIps.forEach((ban, i) => {
      const banTs = ban.created_at ? new Date(ban.created_at).getTime() : Date.now();
      alerts.push({
        id: `ban_${i}`,
        sev: ban.type === 'permanent' ? 'critical' : 'warning',
        title: `Banned Threat Actor: ${ban.ip} (${ban.type})`,
        timestamp: banTs,
        go: 'alerts',
        detail: () => ban.reason || `IP address has been banned due to repeated security violations`
      });
    });
  }

  // Sort alerts: Latest first
  alerts.sort((a, b) => (b.timestamp || 0) - (a.timestamp || 0));

  cache = { routes, pools, A, X, labels, B, total, redir: total * 0.06, certs, modules, alerts, bannedIps: rawApiBannedIps };
  return cache;
}

/* ---------- Pure SVG Chart Engine ---------- */
const hidden = {};
function niceMax(v) {
  const p = Math.pow(10, Math.floor(Math.log10(v || 1))), f = (v || 1) / p;
  return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 4 ? 4 : f <= 8 ? 8 : 10) * p;
}

function spark(vals, o = {}) {
  if (!vals || vals.length === 0) return '';
  const w = 100, h = o.h || 34, n = vals.length, lo = o.zero ? 0 : Math.min(...vals), hi = Math.max(...vals), r = (hi - lo) || 1;
  const pts = vals.map((v, i) => `${((n > 1 ? i / (n - 1) : 0) * w).toFixed(2)},${(h - 2 - (v - lo) / r * (h - 5)).toFixed(2)}`);
  const col = o.color || 'var(--brand)';
  return `<svg class="spark" viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" aria-hidden="true"><path d="M0,${h} L${pts.join(' L')} L${w},${h}Z" fill="${col}" opacity=".13"/><polyline points="${pts.join(' ')}" fill="none" stroke="${col}" stroke-width="1.5" stroke-linejoin="round" vector-effect="non-scaling-stroke"/></svg>`;
}

function chart(host, cfg, force) {
  if (!host) return;
  if (!force && !FORCE && host.matches(':hover')) return;
  host._cfg = cfg;
  const W = Math.max(260, host.clientWidth || 600), Hh = cfg.height || 220;
  const m = { l: cfg.ml || 44, r: 10, t: 10, b: 24 }, iw = W - m.l - m.r, ih = Hh - m.t - m.b;
  const off = hidden[cfg.id] || (hidden[cfg.id] = new Set());
  const vis = cfg.series.filter(s => !off.has(s.key)), n = (cfg.labels || []).length;
  if (n === 0) return;

  let top = 0;
  if (cfg.stacked) { for (let i = 0; i < n; i++) top = Math.max(top, sum(vis.map(s => s.values[i]))); }
  else vis.forEach(s => s.values.forEach(v => { if (v > top) top = v; }));
  (cfg.refs || []).forEach(r => { top = Math.max(top, r.y * 1.15); });
  const max = niceMax((top || 1) * 1.06);
  const x = i => m.l + (cfg.bars ? (i + .5) / n * iw : (n > 1 ? i / (n - 1) * iw : 0)), y = v => m.t + ih - ((v || 0) / max) * ih;

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
  const legend = cfg.series.length > 1 ? `<div class="legend">${cfg.series.map(s => `<button class="lg" data-k="${s.key}" aria-pressed="${!off.has(s.key)}"><i style="background:${s.color}"></i>${esc(s.label)}</button>`).join('')}</div>` : '';
  host.innerHTML = `${legend}<div class="cwrap"><svg viewBox="0 0 ${W} ${Hh}" role="img" aria-label="${esc(cfg.aria || 'Chart')}">${g}</svg><div class="tip" hidden></div></div>`;

  const svg = host.querySelector('svg'), tip = host.querySelector('.tip'), cur = host.querySelector('.cur');
  const move = ev => {
    const r = svg.getBoundingClientRect(), sx = r.width / W, px = (ev.clientX - r.left) / sx;
    let i = cfg.bars ? Math.floor((px - m.l) / iw * n) : Math.round((px - m.l) / iw * (n - 1));
    i = clamp(i, 0, n - 1);
    let c = '';
    if (cfg.bars) c = `<rect class="col" x="${x(i) - iw / n / 2}" y="${m.t}" width="${iw / n}" height="${ih}"/>`;
    else c = `<line class="xl" x1="${x(i)}" x2="${x(i)}" y1="${m.t}" y2="${m.t + ih}"/>` + vis.map(s => `<circle cx="${x(i)}" cy="${y(s.values[i])}" r="3.5" fill="${s.color}" stroke="var(--surface)" stroke-width="1.5"/>`).join('');
    cur.innerHTML = c; cur.style.display = '';
    let rows = vis.slice().reverse().map(s => `<div><i style="background:${s.color}"></i><span>${esc(s.label)}</span><em>${cfg.fmt(s.values[i])}</em></div>`).join('');
    if (cfg.stacked && vis.length > 1) rows += `<div><i style="background:transparent"></i><span>Total</span><em>${cfg.fmt(sum(vis.map(s => s.values[i])))}</em></div>`;
    tip.hidden = false; tip.innerHTML = `<b>${cfg.labels[i]}</b>${rows}`;
    const tw = tip.offsetWidth, px2 = x(i) * sx; let left = px2 + 14;
    if (left + tw > r.width) left = px2 - tw - 14;
    tip.style.left = Math.max(0, left) + 'px';
  };
  svg.onpointermove = move; svg.onpointerdown = move;
  svg.onpointerleave = () => { cur.style.display = 'none'; tip.hidden = true; };
  host.onclick = e => {
    const b = e.target.closest('.lg'); if (!b) return;
    const k = b.dataset.k;
    if (off.has(k)) off.delete(k); else if (vis.length > 1) off.add(k);
    chart(host, host._cfg, true);
  };
}

/* ---------- Traffic Flow (Sankey Diagram) ---------- */
function flow(host, D) {
  if (!host || (!FORCE && host.matches(':hover'))) return;
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
    const matchingRoutes = routeNodes.filter(r => r.pool && (r.pool.includes(p.id) || p.id.includes(r.pool) || (r.pool === p.id)));
    const rps = sum(matchingRoutes.map(r => r.v)) || p.rps || 0;
    const tone = matchingRoutes.some(r => r.tone === 'err') ? 'err' : (p.tone || 'ok');
    const rids = matchingRoutes.map(r => r.id);
    return {
      id: p.id,
      label: p.id,
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
    let targetPool = poolMap.get(r.pool) || poolNodes.find(p => r.pool && (r.pool.includes(p.id) || p.id.includes(r.pool))) || poolNodes[0];
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

/* ---------- View 1: Overview ---------- */
function bannerHTML(D) {
  const bad = D.routes.filter(r => r.cur.err5 >= .02).sort((a, b) => b.cur.err5 - a.cur.err5);
  const down = D.pools.filter(p => p.down > 0);
  const cert = D.certs.filter(c => c.state === 'failing');
  const alertCount = (D.alerts || []).length;

  let lvl = 'ok', title = 'All systems operational', text = `All ${D.routes.length} routes are within thresholds and upstream pools are healthy.`, go = '';

  if (bad.length > 0) {
    const r = bad[0]; lvl = 'err'; title = `Degraded: ${r.short}`; go = `routes`;
    text = `${fmt.pct(r.cur.err5, 1)} of requests to ${r.short} are failing, above the 2% threshold.`;
  } else if (down.length > 0) {
    lvl = 'warn'; title = 'Needs attention: Upstreams Unhealthy'; go = 'upstreams';
    text = `${down.length} upstream pool has failing backend instances.`;
  } else if (cert.length > 0) {
    lvl = 'warn'; title = 'Needs attention: Certificate Renewal'; go = 'certs';
    text = `${cert.length} certificate renewal is failing.`;
  } else if (alertCount > 0) {
    lvl = 'warn'; title = `${alertCount} Active Alert${alertCount > 1 ? 's' : ''}`; go = 'alerts';
    text = `${alertCount} operational or security event${alertCount > 1 ? 's require' : ' requires'} administrative attention.`;
  }

  const ver = rawApiStatus ? rawApiStatus.version : '1.5.29';
  const meta = [['Instances', 'Primary Node'], ['Version', `v${ver}`], ['Engine', 'Zero-Allocation Reactor'], ['Uptime', 'Healthy']];
  const primaryBtnLabel = go === 'upstreams' ? 'View upstreams' : (go === 'certs' ? 'View certificates' : (go === 'routes' ? 'View routes' : 'View alerts'));
  return `<div class="banner ${lvl}"><div class="bn-main"><span class="bn-ic">${ICON(lvl === 'ok' ? 'i-check' : 'i-alert')}</span><div><h2>${esc(title)}</h2><p>${esc(text)}</p>${lvl !== 'ok' ? `<div class="bn-act">${go ? `<button class="btn primary" data-go="${go}">${esc(primaryBtnLabel)}</button>` : ''}<button class="btn" data-go="alerts">Open alerts</button></div>` : ''}</div></div><dl class="meta">${meta.map(([k, v]) => `<div><dt>${esc(k)}</dt><dd>${esc(v)}</dd></div>`).join('')}</dl></div>`;
}

function signals(host, D) {
  const A = D.A, X = D.X, cur = a => a && a.length ? a[a.length - 1] : 0;
  const cells = [
    { k: 'Requests', vals: A.rps, show: v => `${fmt.n(v)}<small>req/s</small>`, bad: null, x: `Peak ${fmt.n(Math.max(...(A.rps.length ? A.rps : [0])))} req/s` },
    { k: 'p95 latency', vals: A.p95, show: v => `${fmt.ms(v)}`, bad: 'up', x: `p99 ${fmt.ms(cur(A.p99))}` },
    { k: '5xx rate', vals: A.err, show: v => `${(v * 100).toFixed(2)}<small>%</small>`, bad: 'up', zero: true, x: `Error budget within target` },
    { k: 'Open connections', vals: X.conns, show: v => Math.round(v).toLocaleString('en-US'), bad: 'up', x: `Active TCP client pool` },
    { k: 'Egress', vals: X.egress, show: v => `${fmt.n(v)}<small>Mb/s</small>`, bad: null, x: `Streaming proxy bandwidth` }
  ];
  host.innerHTML = cells.map(c => {
    const v = cur(c.vals), mean = avg(c.vals), d = mean ? (v - mean) / mean : 0;
    const tone = c.bad === 'up' ? (d > .08 ? 'bad' : d < -.08 ? 'good' : '') : '';
    return `<div class="sig"><div class="k">${esc(c.k)}</div><div class="v">${c.show(v)}</div><div class="d"><span class="delta ${tone}">${d >= 0 ? '▲' : '▼'} ${Math.abs(d * 100).toFixed(1)}%</span> <span class="mut">vs range average</span></div><div class="x">${esc(c.x)}</div>${spark(c.vals, { zero: c.zero, color: tone === 'bad' ? 'var(--c5)' : 'var(--brand)' })}</div>`;
  }).join('');
}

function ovShell() {
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

function ovUpdate(D) {
  const bEl = $('#banner'); if (bEl) bEl.innerHTML = bannerHTML(D);
  flow($('#flow'), D);
  signals($('#signals'), D);
  chart($('#chLat'), {
    id: 'lat', height: 240, labels: D.labels, fmt: fmt.ms, aria: 'Latency percentiles over time', series: [
      { key: 'p50', label: 'p50', color: 'var(--s1)', values: D.A.p50 },
      { key: 'p95', label: 'p95', color: 'var(--s2)', values: D.A.p95 },
      { key: 'p99', label: 'p99', color: 'var(--s3)', values: D.A.p99 }
    ]
  });
  chart($('#chErr'), {
    id: 'err', height: 240, bars: true, stacked: true, labels: D.labels, fmt: v => fmt.n(v) + '/s', aria: '4xx and 5xx responses per second', series: [
      { key: 'e4', label: '4xx', color: 'var(--c4)', values: D.A.e4 },
      { key: 'e5', label: '5xx', color: 'var(--c5)', values: D.A.e5 }
    ]
  });
  const prEl = $('#poolRows');
  if (prEl) {
    prEl.innerHTML = `<li class="hd"><span>Healthy</span><span>Pool / Route</span><span class="r">p95</span><span class="r">5xx</span></li>` + D.pools.map(p => `<li data-go="upstreams:${p.id}" tabindex="0" role="link"><span>${pill(p.tone, `${p.up}/${p.insts.length}`)}</span><b>${esc(p.displayName || p.id)}</b><span class="mut num r">${fmt.ms(p.p95)}</span><span class="num r ${p.err5 >= .02 ? 't-err' : ''}">${fmt.pct(p.err5, 1)}</span></li>`).join('');
  }
  const alEl = $('#alertRows');
  if (alEl) {
    alEl.innerHTML = (D.alerts || []).slice(0, 4).map(a => `<li data-go="${a.go}" tabindex="0" role="link"><span class="${a.sev === 'critical' ? 't-err' : 't-warn'}">${ICON(a.sev === 'critical' ? 'i-x' : 'i-alert')}</span><div><div class="al-t">${esc(a.title)}</div><div class="al-d">${esc(a.detail(D))}</div></div><span class="mut num" style="white-space:nowrap;font-size:11.5px">${dtFmt(a.timestamp)}</span></li>`).join('') || `<li><span class="t-ok">${ICON('i-check')}</span><div><div class="al-t">Zero Active Incidents</div></div></li>`;
  }
  const crEl = $('#certRows');
  if (crEl) {
    crEl.innerHTML = D.certs.slice(0, 4).map(c => `<li data-go="certs" tabindex="0" role="link"><div><div class="al-t">${esc(c.domain)}</div><div class="bar"><i style="width:${clamp(c.days / 90 * 100, 5, 100)}%;background:var(--ok)"></i></div></div><span class="num ${c.state === 'failing' ? 't-err' : 'mut'}">${c.days} days</span>${pill(c.state === 'failing' ? 'err' : 'ok', c.state === 'failing' ? 'Failing' : 'Valid')}</li>`).join('');
  }
}

/* ---------- View 2: Routes ---------- */
function rtHead() {
  const cols = [['name', 'Route', ''], [null, 'Upstream', 'hide-md'], ['rps', 'Requests', 'num'], ['p95', 'p95 latency', 'num'], ['err', '5xx rate', 'num'], [null, 'Status', 'hide-sm']];
  return `<tr>${cols.map(([k, l, c]) => { const on = k && state.sort.k === k; return `<th class="${c}" aria-sort="${on ? (state.sort.dir > 0 ? 'ascending' : 'descending') : 'none'}">${k ? `<button class="th" data-sort="${k}">${l}${on ? (state.sort.dir > 0 ? ' ▲' : ' ▼') : ''}</button>` : l}</th>`; }).join('')}</tr>`;
}

function rtShell() {
  return `<section class="card">
    <div class="toolbar"><div class="search">${ICON('i-search')}<input class="field" id="rtQ" type="search" placeholder="Filter routes..." aria-label="Filter routes" value="${esc(state.q)}"></div><span class="sp"></span><span class="mut" id="rtCount"></span></div>
    <div class="tscroll"><table class="tbl"><thead id="rtHead">${rtHead()}</thead><tbody id="rtBody"></tbody></table></div></section>`;
}

function rtInit() {
  const qEl = $('#rtQ'); if (qEl) qEl.addEventListener('input', e => { state.q = e.target.value.trim().toLowerCase(); rtUpdate(buildDataModel()); });
  const hEl = $('#rtHead'); if (hEl) hEl.addEventListener('click', e => {
    const b = e.target.closest('[data-sort]'); if (!b) return;
    const k = b.dataset.sort;
    state.sort = state.sort.k === k ? { k, dir: -state.sort.dir } : { k, dir: k === 'name' ? 1 : -1 };
    hEl.innerHTML = rtHead(); rtUpdate(buildDataModel());
  });
}

function rtUpdate(D) {
  const body = $('#rtBody'); if (!body) return;
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
      <td class="hide-sm">${pill(t, t === 'err' ? 'Degraded' : t === 'warn' ? 'Elevated' : 'Healthy')}</td></tr>`;
  }).join('') || `<tr><td colspan="6" class="empty">No routes match "${esc(state.q)}".</td></tr>`;
}

/* ---------- View 3: Upstreams ---------- */
function poolCard(p) {
  const rows = p.insts.map(i => {
    const st = i.state || 'up', tone = st === 'down' ? 'err' : st === 'draining' ? 'mute' : 'ok';
    const label = st === 'down' ? 'Down' : st === 'draining' ? 'Draining' : (i.httpCode ? `Healthy (HTTP ${i.httpCode})` : 'Healthy');
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
        <span>Latency: <b>${i.latency ? i.latency.toFixed(1) + ' ms' : fmt.ms(p.p95)}</b></span>
        <span>Probe Status: ${st === 'down' ? '<b style="color:var(--err)">Unreachable</b>' : '<b style="color:var(--ok)">HTTP 200 OK</b>'}</span>
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
    <dl class="pool-sum"><div><dt>Requests</dt><dd>${fmt.n(p.rps)}/s</dd></div><div><dt>p95 Latency</dt><dd>${fmt.ms(p.p95)}</dd></div><div><dt>5xx Error Rate</dt><dd class="${p.err5 >= .02 ? 't-err' : ''}">${fmt.pct(p.err5, 1)}</dd></div></dl>
    <div style="margin-top:4px">${rows}</div></section>`;
}

function upShell() { return `<div class="pools" id="poolGrid"></div><p class="hc-legend">Each tick is a health check probe (oldest left, newest right). Red ticks failed.</p>`; }
function upUpdate(D) { const el = $('#poolGrid'); if (el) el.innerHTML = D.pools.map(poolCard).join(''); }

/* ---------- View 4: Live Requests & Logs ---------- */
function stCls(s) { return s === 101 ? '2' : String(s)[0]; }
function lgShell() {
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

function lgSync() { const b = $('#lgPause'); if (b) b.innerHTML = state.live ? `${ICON('i-pause')}Pause Tail` : `${ICON('i-play')}Resume Tail`; }
function lgInit() {
  const upd = () => lgUpdate(buildDataModel(), true);
  const qEl = $('#lgQ'); if (qEl) qEl.addEventListener('input', e => { state.lq = e.target.value.trim().toLowerCase(); upd(); });
  const rtEl = $('#lgRoute'); if (rtEl) rtEl.addEventListener('change', e => { state.lroute = e.target.value; upd(); });
  const slEl = $('#lgSlow'); if (slEl) slEl.addEventListener('click', e => { state.lslow = !state.lslow; e.currentTarget.setAttribute('aria-pressed', String(state.lslow)); upd(); });
  $$('[data-st]').forEach(b => b.addEventListener('click', () => { const c = b.dataset.st; state.lst.has(c) ? state.lst.delete(c) : state.lst.add(c); b.setAttribute('aria-pressed', String(state.lst.has(c))); upd(); }));
  const psEl = $('#lgPause'); if (psEl) psEl.addEventListener('click', () => setLive(!state.live));
  const bEl = $('#lgBody');
  if (bEl) {
    bEl.addEventListener('click', e => { const r = e.target.closest('[data-log]'); if (r) openDrawer('log', +r.dataset.log); });
  }
  lgSync();
}

function lgUpdate(D, force) {
  const body = $('#lgBody'); if (!body) return;
  const logs = (rawApiLogs && rawApiLogs.length > 0) ? rawApiLogs : [
    { id: 101, ts: Date.now() - 400, method: 'GET', path: '/v1/orders/8f2a91', route: 'orders', short: 'api /v1/orders', status: 200, ms: 14.2, up: '10.0.1.11:8080', trace: 'a4b1c8f0e2d4', ip: '192.168.1.10', bytes: 1240 },
    { id: 102, ts: Date.now() - 900, method: 'POST', path: '/v1/payments/charge', route: 'payments', short: 'api /v1/payments', status: 502, ms: 210.5, up: '10.0.4.13:8080', trace: 'b9e3d1a8c7f2', ip: '192.168.1.15', bytes: 420, err: 'connect: connection refused' }
  ];

  const f = logs.filter(e => state.lst.has(stCls(e.status)) && (state.lroute === 'all' || e.route === state.lroute) && (!state.lq || (e.path + e.up + e.trace + (e.short || '') + (e.ip || '')).toLowerCase().includes(state.lq)));
  const sumEl = $('#lgSum');
  if (sumEl) sumEl.textContent = `Showing ${Math.min(f.length, 70)} of ${f.length} requests in live tail.` + (state.live ? '' : ' Tail is paused.');

  body.innerHTML = f.slice(0, 70).map(e => `<tr class="click" tabindex="0" data-log="${e.id}"><td class="tm" style="white-space:nowrap">${dtFmt(e.ts)}</td><td><span class="st s${stCls(e.status)}">${e.status}</span></td><td class="path"><span class="meth">${esc(e.method)}</span>${esc(e.path)}</td><td class="hide-sm"><code style="font-size:11.5px">${esc(e.ip || e.client_ip || '127.0.0.1')}</code></td><td class="hide-md">${esc(e.short || e.route)}</td><td class="hide-md"><code>${esc(e.up)}</code></td><td class="num">${fmt.ms(e.ms)}</td><td class="hide-sm"><code>${esc((e.trace || '').slice(0, 8))}</code></td></tr>`).join('') || `<tr><td colspan="8" class="empty">No requests match active filters.</td></tr>`;
}

/* ---------- View 5: Certificates ---------- */
function ceShell() {
  return `<section class="card"><div class="tscroll"><table class="tbl"><thead><tr><th>Domain</th><th class="hide-sm">Challenge</th><th>Expires</th><th class="hide-md" style="width:22%">Time remaining</th><th>Renewal</th></tr></thead><tbody id="ceBody"></tbody></table></div></section>
    <p class="hc-legend">Certificates are automatically issued and renewed by the ACME zero-touch engine 30 days before expiry.</p>`;
}
function ceUpdate(D) {
  const body = $('#ceBody'); if (!body) return;
  body.innerHTML = D.certs.map(c => {
    const t = c.state === 'failing' ? 'err' : c.days <= 30 ? 'warn' : 'ok';
    const d = new Date(Date.now() + c.days * 864e5).toLocaleDateString('en-US', { day: 'numeric', month: 'short', year: 'numeric' });
    return `<tr><td><div class="rt"><b>${esc(c.domain)}</b><span class="mut">Let's Encrypt</span></div></td><td class="hide-sm">${esc(c.chal)}</td><td><div class="rt"><b>${d}</b><span class="${t === 'err' ? 't-err' : 'mut'}">in ${c.days} days</span></div></td><td class="hide-md"><div class="bar"><i style="width:${clamp(c.days / 90 * 100, 5, 100)}%;background:${t === 'ok' ? 'var(--ok)' : t === 'warn' ? 'var(--c4)' : 'var(--c5)'}"></i></div></td><td>${c.state === 'failing' ? pill('err', 'Renewal Failing') : pill('ok', `Renews in ${Math.max(0, c.days - 30)}d`)}</td></tr>` + (c.err ? `<tr class="dtl"><td colspan="5"><div class="note err">${esc(c.err)}</div></td></tr>` : '');
  }).join('');
}

/* ---------- View 6: Modules & Runtime ---------- */
function mdShell() {
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

function mdUpdate(D) {
  const X = D.X, last = a => a && a.length ? a[a.length - 1] : 0, ev = avg(X.ev);
  const mBody = $('#mdBody');
  if (mBody) {
    mBody.innerHTML = D.modules.map(m => `<tr><td><div class="rt"><b>${esc(m.id)}</b><span class="mut">${esc(m.kind)}</span></div></td><td>${m.state === 'running' ? pill('ok', 'Running') : pill('warn', 'Degraded')}</td><td class="num">${fmt.n(ev * (m.w || 0.1))}/s</td><td class="hide-md mut">${esc(m.note)}</td></tr>`).join('');
  }
  const one = (id, key, vals, f, extra = {}) => chart($('#' + id), { id, height: extra.h || 160, ml: extra.ml || 44, labels: D.labels, fmt: f, aria: key, series: [{ key, label: key, color: extra.c || 'var(--s2)', values: vals, area: true }] });
  if ($('#cvEv')) $('#cvEv').textContent = fmt.n(ev) + '/s';
  if ($('#cvCpu')) $('#cvCpu').textContent = last(X.cpu).toFixed(0) + '%';
  if ($('#cvGor')) $('#cvGor').textContent = Math.round(last(X.gor)).toLocaleString('en-US');
  if ($('#cvHeap')) $('#cvHeap').textContent = Math.round(last(X.heap)) + ' MB';
  if ($('#cvGc')) $('#cvGc').textContent = last(X.gc).toFixed(2) + ' ms';
  if ($('#cvFd')) $('#cvFd').textContent = Math.round(last(X.fd)).toLocaleString('en-US');

  one('chEv', 'Events/s', X.ev, v => fmt.n(v) + '/s', { h: 200 });
  one('chCpu', 'CPU', X.cpu, v => v.toFixed(0) + '%', { h: 200, c: 'var(--s1)' });
  one('chGor', 'Goroutines', X.gor, v => Math.round(v).toLocaleString('en-US'));
  one('chHeap', 'Heap', X.heap, v => Math.round(v) + ' MB');
  one('chGc', 'GC', X.gc, v => v.toFixed(2) + ' ms');
  one('chFd', 'FDs', X.fd, v => Math.round(v).toLocaleString('en-US'));
}

/* ---------- View 7: Alerts & Threat Defense ---------- */
function alShell() {
  return `<div class="stack">
    <section class="card">
      <div class="card-h">
        <div>
          <h2>Active Alerts & WAF Incidents</h2>
          <p class="sub">Security events, anomalies, and operational alerts requiring attention</p>
        </div>
      </div>
      <ul class="rows al" id="alActive" style="margin-top:8px"></ul>
    </section>

    <section class="card" style="margin-top:16px">
      <div class="card-h" style="display:flex;justify-content:space-between;align-items:center">
        <div>
          <h2>Dynamic 2-Stage Auto-Ban & Blocked IPs</h2>
          <p class="sub">Automated threat defense reactor and persistent IP firewall entries</p>
        </div>
        <div style="display:flex;gap:8px;align-items:center">
          <input type="text" id="manualBanIP" class="field" placeholder="IP to ban (e.g. 1.2.3.4)" style="width:160px;font-size:12px;padding:4px 8px">
          <select id="manualBanType" class="field" style="font-size:12px;padding:4px 8px">
            <option value="temporary">Stage 1 (1h Temp)</option>
            <option value="permanent">Stage 2 (Permanent)</option>
          </select>
          <button class="btn primary" id="manualBanBtn" style="font-size:12px;padding:4px 10px">Ban IP</button>
        </div>
      </div>
      <div class="card-b" style="padding:0;overflow-x:auto">
        <table class="tbl" style="width:100%;text-align:left;border-collapse:collapse;font-size:13px">
          <thead>
            <tr style="border-bottom:1px solid var(--line);background:var(--surface-2)">
              <th style="padding:10px 14px">Client IP</th>
              <th style="padding:10px 14px">Ban Tier</th>
              <th style="padding:10px 14px">Created At</th>
              <th style="padding:10px 14px">Temp Bans</th>
              <th style="padding:10px 14px">Reason / Category</th>
              <th style="padding:10px 14px">TTL / Expiry</th>
              <th style="padding:10px 14px;text-align:right">Action</th>
            </tr>
          </thead>
          <tbody id="bannedIpsTable">
            <tr><td colspan="7" style="padding:16px;text-align:center;color:var(--text-muted)">Loading threat table...</td></tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>`;
}

function alUpdate(D) {
  const alEl = $('#alActive');
  if (alEl) {
    alEl.innerHTML = D.alerts.map(a => `<li data-go="${a.go}" tabindex="0" role="link"><span class="${a.sev === 'critical' ? 't-err' : 't-warn'}">${ICON(a.sev === 'critical' ? 'i-x' : 'i-alert')}</span><div><div class="al-t">${esc(a.title)}</div><div class="al-d">${esc(a.detail(D))}</div></div><span class="mut num" style="white-space:nowrap;font-size:12px">${dtFmt(a.timestamp)}</span></li>`).join('') || `<li><span class="t-ok">${ICON('i-check')}</span><div><div class="al-t">All Systems Operational</div><div class="al-d">Zero unresolved security incidents.</div></div></li>`;
  }

  const tb = $('#bannedIpsTable');
  if (tb) {
    const bans = (D.bannedIps || []).slice().sort((a, b) => new Date(b.created_at || 0).getTime() - new Date(a.created_at || 0).getTime());
    if (bans.length === 0) {
      tb.innerHTML = `<tr><td colspan="7" style="padding:20px;text-align:center;color:var(--text-muted)">${ICON('i-check')} Zero active IP bans in effect.</td></tr>`;
    } else {
      tb.innerHTML = bans.map(b => {
        const isPerm = b.type === 'permanent';
        const tierBadge = isPerm 
          ? `<span class="st s4" style="background:rgba(239,68,68,0.15);color:#ef4444;font-weight:600;padding:2px 8px;border-radius:4px">Stage 2: Permanent</span>`
          : `<span class="st s3" style="background:rgba(245,158,11,0.15);color:#f59e0b;font-weight:600;padding:2px 8px;border-radius:4px">Stage 1: Temporary</span>`;
        
        let ttlStr = 'Never (Permanent)';
        if (!isPerm && b.remaining_seconds >= 0) {
          const m = Math.floor(b.remaining_seconds / 60);
          const s = b.remaining_seconds % 60;
          ttlStr = `${m}m ${s}s remaining`;
        }

        return `<tr style="border-bottom:1px solid var(--line)">
          <td style="padding:10px 14px;font-family:var(--mono);font-weight:600">${esc(b.ip)}</td>
          <td style="padding:10px 14px">${tierBadge}</td>
          <td style="padding:10px 14px;font-family:var(--mono);font-size:12px;white-space:nowrap">${dtFmt(b.created_at)}</td>
          <td style="padding:10px 14px;font-family:var(--mono)">${b.temp_ban_count || 0}</td>
          <td style="padding:10px 14px;max-width:280px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title="${esc(b.reason || '')}">${esc(b.reason || b.last_category || 'WAF violation')}</td>
          <td style="padding:10px 14px;font-size:12px;color:var(--text-muted)">${esc(ttlStr)}</td>
          <td style="padding:10px 14px;text-align:right">
            <button class="btn" style="padding:2px 8px;font-size:11px" onclick="unbanIpAddress('${esc(b.ip)}')">Unban</button>
          </td>
        </tr>`;
      }).join('');
    }
  }

  const banBtn = $('#manualBanBtn');
  if (banBtn && !banBtn._bound) {
    banBtn._bound = true;
    banBtn.onclick = async () => {
      const ipIn = $('#manualBanIP');
      const typeIn = $('#manualBanType');
      if (!ipIn || !ipIn.value.trim()) return;
      const ip = ipIn.value.trim();
      const type = typeIn ? typeIn.value : 'temporary';
      try {
        const res = await fetch('/internal/api/security/ban', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ ip, type, reason: 'Manually banned via dashboard' })
        });
        if (res.ok) {
          ipIn.value = '';
          await fetchBackendData();
        } else {
          const err = await res.json();
          alert('Failed to ban IP: ' + (err.message || 'Unknown error'));
        }
      } catch (e) {
        alert('Ban request error: ' + e.message);
      }
    };
  }
}

window.unbanIpAddress = async function(ip) {
  if (!confirm(`Are you sure you want to unban IP ${ip}?`)) return;
  try {
    const res = await fetch('/internal/api/security/unban', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ip })
    });
    if (res.ok) {
      await fetchBackendData();
    } else {
      const err = await res.json();
      alert('Failed to unban IP: ' + (err.message || 'Unknown error'));
    }
  } catch (e) {
    alert('Unban request error: ' + e.message);
  }
};

/* ---------- View 8: API Console ---------- */
function consoleShell() {
  const routes = buildDataModel().routes;
  return `<section class="card">
    <div class="card-h"><div><h2>API Console & Proxy Debugger</h2><p class="sub">Dispatch test probes against configured gateway routes and inspect headers & responses</p></div></div>
    <div class="card-b">
      <form id="consoleForm" style="display:flex;flex-direction:column;gap:12px">
        <div style="display:grid;grid-template-columns:120px 1fr 200px;gap:10px">
          <div><label class="mut" style="font-size:12px">Method</label><select id="csMethod" class="field" style="width:100%"><option value="GET">GET</option><option value="POST">POST</option><option value="HEAD">HEAD</option></select></div>
          <div><label class="mut" style="font-size:12px">Endpoint / Path</label><select id="csPath" class="field" style="width:100%"><option value="/health">GET /health (Server Health)</option><option value="/internal/api/status">GET /internal/api/status (Engine Metrics)</option>${routes.map(r => `<option value="${r.path}">ANY ${r.path} [${r.host}]</option>`).join('')}</select></div>
          <div><label class="mut" style="font-size:12px">Host Override</label><input type="text" id="csHost" class="field" style="width:100%" placeholder="e.g. api.example.com"></div>
        </div>
        <div style="display:flex;justify-content:flex-end"><button type="submit" class="btn primary">Execute Probe</button></div>
      </form>
      <div style="margin-top:16px;padding-top:14px;border-top:1px solid var(--line)">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">
          <span style="font-weight:600;font-size:13px">Response Output:</span>
          <span id="csStatus" class="st s2">Ready</span>
        </div>
        <pre id="csBody" style="background:var(--surface-2);border:1px solid var(--line);border-radius:8px;padding:12px;font-family:var(--mono);font-size:12px;max-height:300px;overflow:auto">Press "Execute Probe" to test route...</pre>
      </div>
    </div>
  </section>`;
}

function consoleInit() {
  const form = $('#consoleForm');
  if (form) {
    form.addEventListener('submit', async e => {
      e.preventDefault();
      const method = $('#csMethod').value;
      const path = $('#csPath').value;
      const host = $('#csHost').value;
      const stEl = $('#csStatus');
      const bodyEl = $('#csBody');

      if (stEl) stEl.textContent = 'Executing...';
      try {
        const headers = {};
        if (host) headers['Host'] = host;
        const res = await fetch('/internal/api/proxy-test', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ path, method, headers })
        });
        const data = await res.json();
        if (stEl) stEl.textContent = `HTTP ${data.status_code || res.status} (${(data.latency_ms || 0).toFixed(1)}ms)`;
        if (bodyEl) {
          try {
            bodyEl.textContent = JSON.stringify(JSON.parse(data.body), null, 2);
          } catch {
            bodyEl.textContent = data.body || JSON.stringify(data, null, 2);
          }
        }
      } catch (err) {
        if (stEl) stEl.textContent = 'Error';
        if (bodyEl) bodyEl.textContent = err.message;
      }
    });
  }
}

/* ---------- Slide-Over Drawer Engine ---------- */
const dw = { el: null, kind: null, id: null, last: null };
function histo(med, p95) {
  const edges = [10, 25, 50, 100, 250, 500, Infinity], labels = ['< 10ms', '10-25', '25-50', '50-100', '100-250', '250-500', '> 500'];
  return edges.map((e, i) => ({ l: labels[i], v: 10 + (i === 2 ? 40 : i === 1 ? 25 : 5) }));
}

function openDrawer(kind, id) {
  dw.el = $('#drawer'); if (!dw.el) return;
  dw.kind = kind; dw.id = id; dw.last = document.activeElement;
  dw.el.hidden = false;

  if (kind === 'route') {
    const r = buildDataModel().routes.find(x => x.id === id) || buildDataModel().routes[0];
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

  $('#scrim').classList.add('on'); document.body.classList.add('lock');
  requestAnimationFrame(() => dw.el.classList.add('open'));
  $('#dwClose').focus();
}

function renderDrawer() {
  if (dw.kind !== 'route') return;
  const D = buildDataModel(), r = D.routes.find(x => x.id === dw.id) || D.routes[0], c = r.cur, t = toneErr(c.err5);
  $('#dwSub').innerHTML = `${pill(t, t === 'err' ? 'Degraded' : 'Healthy')}<span class="mut">${esc(r.host + r.path)}</span>`;
  $('#dwStats').innerHTML = [['Requests', fmt.n(c.rps) + '/s'], ['p50', fmt.ms(c.p50)], ['p95', fmt.ms(c.p95)], ['p99', fmt.ms(c.p99)], ['4xx rate', fmt.pct(c.err4, 2)], ['5xx rate', fmt.pct(c.err5, 2)]].map(([k, v]) => `<div><dt>${esc(k)}</dt><dd>${esc(v)}</dd></div>`).join('');
  chart($('#dwLat'), { id: 'dwlat', height: 170, labels: D.labels, fmt: fmt.ms, aria: 'p95 latency curve', refs: [{ y: r.slo, label: `SLO ${r.slo}ms`, color: 'var(--warn)' }], series: [{ key: 'p95', label: 'p95', color: 'var(--s2)', values: r.s.p95, area: true }] });
  chart($('#dwErr'), { id: 'dwerr', height: 150, bars: true, stacked: true, labels: D.labels, fmt: v => v.toFixed(1) + '/s', aria: 'Error volume', series: [{ key: 'e4', label: '4xx', color: 'var(--c4)', values: r.s.e4 }, { key: 'e5', label: '5xx', color: 'var(--c5)', values: r.s.e5 }] });
  const hs = histo(c.p50, c.p95), mx = Math.max(...hs.map(h => h.v));
  $('#dwHist').innerHTML = hs.map(h => `<span class="mut">${esc(h.l)}</span><div class="bar"><i style="width:${(h.v / mx * 100).toFixed(1)}%;background:var(--s2)"></i></div><span class="r">${h.v}%</span>`).join('');
}

function closeDrawer() {
  if (!dw.el || dw.el.hidden) return;
  dw.el.classList.remove('open'); $('#scrim').classList.remove('on'); document.body.classList.remove('lock');
  dw.kind = null;
  setTimeout(() => { if (!dw.kind && dw.el) dw.el.hidden = true; }, 230);
}

/* ---------- Navigation & App Shell ---------- */
const NAV = [
  { id: 'overview', label: 'Overview', icon: 'i-overview' },
  { id: 'routes', label: 'Routes', icon: 'i-routes' },
  { id: 'upstreams', label: 'Upstreams', icon: 'i-server' },
  { id: 'logs', label: 'Requests', icon: 'i-list' },
  { id: 'certs', label: 'Certificates', icon: 'i-shield' },
  { id: 'modules', label: 'Modules', icon: 'i-cube' },
  { id: 'alerts', label: 'Alerts', icon: 'i-bell' },
  { id: 'console', label: 'API Console', icon: 'i-terminal' }
];

const META = {
  overview: { t: 'Overview', s: 'Gateway health, real-time traffic flow, and primary telemetry signals', range: 1, inst: 1 },
  routes: { t: 'Routes', s: 'Traffic, latency percentiles, and error rate breakdown per route', range: 1, inst: 1 },
  upstreams: { t: 'Upstreams', s: 'Target node health, connection load, and circuit breaker status', inst: 1 },
  logs: { t: 'Requests', s: 'Live request tailing with end-to-end trace waterfalls', inst: 1 },
  certs: { t: 'Certificates', s: 'ACME zero-touch TLS certificates and renewal lifecycle' },
  modules: { t: 'Modules & Runtime', s: 'Compiled-in reactors, event bus, and Go runtime internals', range: 1, inst: 1 },
  alerts: { t: 'Alerts & Incidents', s: 'WAF security violations and operational degradation events' },
  console: { t: 'API Console', s: 'Interactive endpoint probe execution and response inspector' }
};

const VIEWS = {
  overview: { shell: ovShell, update: ovUpdate },
  routes: { shell: rtShell, init: rtInit, update: rtUpdate },
  upstreams: { shell: upShell, update: upUpdate },
  logs: { shell: lgShell, init: lgInit, update: lgUpdate },
  certs: { shell: ceShell, update: ceUpdate },
  modules: { shell: mdShell, update: mdUpdate },
  alerts: { shell: alShell, update: alUpdate },
  console: { shell: consoleShell, init: consoleInit, update: () => {} }
};

function renderNav() {
  const nEl = $('#nav'); if (!nEl) return;
  const D = buildDataModel();
  const alertCount = (D.alerts || []).length;
  nEl.innerHTML = NAV.map(n => {
    const badge = (n.id === 'alerts' && alertCount > 0) ? `<span class="badge">${alertCount}</span>` : '';
    return `<button class="nv" data-nav="${n.id}" title="${esc(n.label)}"${n.id === state.view ? ' aria-current="page"' : ''}>${ICON(n.icon)}<span class="lbl">${esc(n.label)}</span>${badge}</button>`;
  }).join('');
}

function head() {
  const m = META[state.view] || META.overview;
  $('#title').textContent = m.t; $('#sub').textContent = m.s;
  $('#rangeSeg').hidden = !m.range; $('#instWrap').hidden = !m.inst;
  $$('#rangeSeg button').forEach(b => b.setAttribute('aria-pressed', String(b.dataset.r === state.range)));
  document.title = `${m.t} · Toron Gateway`;
}

function mountView() {
  const v = VIEWS[state.view] || VIEWS.overview;
  $('#view').innerHTML = v.shell();
  if (v.init) v.init();
  renderNav(); head(); refresh(true);
}

function chrome() {
  $('#stamp').textContent = state.live ? `Updated ${hms(new Date())}` : 'Paused';
  const b = $('#liveBtn'); if (b) b.setAttribute('aria-pressed', String(state.live));
  const lt = $('#liveTxt'); if (lt) lt.textContent = state.live ? 'Live' : 'Paused';
  lgSync();
}

function refresh(force) {
  FORCE = !!force;
  try {
    const D = buildDataModel();
    const v = VIEWS[state.view] || VIEWS.overview;
    v.update(D);
    if (dw.kind === 'route') renderDrawer();
    chrome();
  } finally {
    FORCE = false;
  }
}

function go(view, id) {
  closeNav();
  if (dw.kind) closeDrawer();
  if (view !== state.view && VIEWS[view]) {
    state.view = view;
    mountView();
    window.scrollTo(0, 0);
  }
  try { history.replaceState(null, '', '#/' + view + (id ? '/' + id : '')); } catch (e) {}
  if (id && view === 'routes') openDrawer('route', id);
  if (id && view === 'upstreams') { const el = $('#pool-' + id); if (el) el.scrollIntoView({ block: 'center', behavior: 'smooth' }); }
}

function setLive(v) { state.live = v; chrome(); if (state.view === 'logs') lgUpdate(buildDataModel(), true); }

/* ---------- Theme Engine ---------- */
const mq = window.matchMedia('(prefers-color-scheme: dark)');
function effTheme() { return document.documentElement.getAttribute('data-theme') || (mq.matches ? 'dark' : 'light'); }
function themeIcon() { const t = effTheme(); $$('#themeBtn use, #themeBtn2 use').forEach(u => u.setAttribute('href', t === 'dark' ? '#i-sun' : '#i-moon')); }
function toggleTheme() {
  const n = effTheme() === 'dark' ? 'light' : 'dark';
  document.documentElement.setAttribute('data-theme', n);
  try { localStorage.setItem('toron-theme', n); } catch (e) {}
  themeIcon();
}

/* ---------- Mobile Navigation ---------- */
function openNav() { $('#rail').classList.add('open'); $('#scrim').classList.add('on'); $('#menuBtn').setAttribute('aria-expanded', 'true'); }
function closeNav() { if (!$('#rail').classList.contains('open')) return; $('#rail').classList.remove('open'); if (!dw.kind) $('#scrim').classList.remove('on'); $('#menuBtn').setAttribute('aria-expanded', 'false'); }

/* ---------- Event Listeners ---------- */
function activate(el) { const [v, id] = el.dataset.go.split(':'); go(v, id); }
document.addEventListener('click', e => {
  const g = e.target.closest('[data-go]'); if (g) { activate(g); return; }
  const n = e.target.closest('[data-nav]'); if (n) { go(n.dataset.nav); }
});

document.addEventListener('keydown', e => {
  if ((e.key === 'Enter' || e.key === ' ') && e.target.matches && e.target.matches('[data-go][tabindex]')) { e.preventDefault(); activate(e.target); return; }
  if (e.key === 'Escape') { if (dw.kind) closeDrawer(); else closeNav(); }
});

$('#scrim').addEventListener('click', () => { if (dw.kind) closeDrawer(); else closeNav(); });
$('#dwClose').addEventListener('click', closeDrawer);
const mBtn = $('#menuBtn'); if (mBtn) mBtn.addEventListener('click', () => $('#rail').classList.contains('open') ? closeNav() : openNav());
const tBtn = $('#themeBtn'); if (tBtn) tBtn.addEventListener('click', toggleTheme);
const tBtn2 = $('#themeBtn2'); if (tBtn2) tBtn2.addEventListener('click', toggleTheme);
const lBtn = $('#liveBtn'); if (lBtn) lBtn.addEventListener('click', () => setLive(!state.live));
const rSeg = $('#rangeSeg'); if (rSeg) rSeg.addEventListener('click', e => {
  const b = e.target.closest('button'); if (!b) return;
  state.range = b.dataset.r; invalidate(); head(); refresh(true);
});

/* ---------- Application Bootstrap ---------- */
(function boot() {
  const m = (location.hash || '').match(/^#\/(\w+)(?:\/([\w-]+))?/);
  if (m && VIEWS[m[1]]) state.view = m[1];
  themeIcon();
  mountView();
  fetchBackendData();
  setInterval(() => {
    if (state.live && !document.hidden) {
      state.step++;
      fetchBackendData();
    }
  }, 2000);
})();

})();
