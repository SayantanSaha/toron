/**
 * Toron Dashboard - Telemetry Data Model & Metrics Ingestion
 */
import { sum, avg, fmt, hms, hm } from './utils.js';
import { N, RANGES, state, getCache, setCache } from './state.js';
import {
  rawApiStatus,
  rawApiRoutes,
  rawApiUpstreams,
  rawApiIncidents,
  rawApiBannedIps
} from './api.js';

export function buildDataModel() {
  const cached = getCache();
  if (cached) return cached;

  const B = (RANGES[state.range] && RANGES[state.range].bucket) ? RANGES[state.range].bucket : 60;

  // 1. Build routes list
  let routes = (rawApiRoutes && rawApiRoutes.length > 0) ? rawApiRoutes.map((r, idx) => {
    const rawId = r.id || r.prefix || `route_${idx}`;
    const id = String(rawId).replace(/[^a-zA-Z0-9]/g, '_') || `route_${idx}`;
    const path = r.prefix || r.path || '/';
    const host = r.host || '*';
    const short = (r.host ? r.host + ' ' : '') + path;
    const pool = (r.targets && r.targets.length > 0) ? r.targets[0] : (r.dir ? `static (${r.dir})` : 'in-process');

    const tsA = rawApiStatus && rawApiStatus.timeseries && rawApiStatus.timeseries.A;
    const totalRPS = (tsA && tsA.rps && tsA.rps.length > 0) ? (tsA.rps[tsA.rps.length - 1] || 0) : 0;
    const reqByRoute = (rawApiStatus && rawApiStatus.metrics && rawApiStatus.metrics.requests_by_route) || {};
    const totalRequests = (rawApiStatus && rawApiStatus.metrics && rawApiStatus.metrics.total_requests) || 0;

    // Aggregate cumulative requests for this route prefix across all subpaths (e.g. /kite/api/v1/business under /kite/api)
    const prefix = r.prefix || path;
    function getRouteTotalReqs(pfx) {
      if (!pfx) return 0;
      if (pfx === '/') {
        return Object.entries(reqByRoute).reduce((acc, [k, count]) => {
          if (!k.startsWith('/internal/')) return acc + (Number(count) || 0);
          return acc;
        }, 0);
      }
      return Object.entries(reqByRoute).reduce((acc, [k, count]) => {
        if (k === pfx || k.startsWith(pfx + '/')) {
          return acc + (Number(count) || 0);
        }
        return acc;
      }, 0);
    }
    const routeCumulative = getRouteTotalReqs(prefix);

    // Exclude internal dashboard polling requests when computing external user traffic ratio
    const totalExternalRequests = Object.entries(reqByRoute).reduce((acc, [k, count]) => {
      if (!k.startsWith('/internal/')) {
        return acc + (Number(count) || 0);
      }
      return acc;
    }, 0);

    // Smooth instantaneous RPS via trailing 5-point moving average
    const recentRpsSamples = (tsA && tsA.rps && tsA.rps.length > 0) ? tsA.rps.slice(-5) : [];
    const avgRecentRps = recentRpsSamples.length > 0 ? avg(recentRpsSamples) : 0;
    const effectiveTotalRps = avgRecentRps > 0 ? avgRecentRps : totalRPS;

    let curRPS = 0;
    if (totalExternalRequests > 0 && effectiveTotalRps > 0) {
      curRPS = effectiveTotalRps * (routeCumulative / totalExternalRequests);
    } else if (effectiveTotalRps > 0 && rawApiRoutes.length > 0) {
      curRPS = effectiveTotalRps / rawApiRoutes.length;
    }

    // Find matching probes for this route: strict route match first, fallback to target address only if no route match
    let matchingUpstreams = (rawApiUpstreams || []).filter(u => {
      if (!u || !u.route) return false;
      const uRt = String(u.route).toLowerCase();
      const pfx = String(r.prefix || path || '').toLowerCase();
      const srt = String(short).toLowerCase();
      const hostPath = String((r.host || '') + path).toLowerCase();
      return (pfx && (uRt === pfx || uRt.includes(pfx) || pfx.includes(uRt))) ||
             (srt && (uRt === srt || uRt.includes(srt) || srt.includes(uRt))) ||
             (hostPath && (uRt === hostPath || uRt.includes(hostPath) || hostPath.includes(uRt)));
    });
    if (matchingUpstreams.length === 0) {
      matchingUpstreams = (rawApiUpstreams || []).filter(u => {
        if (!u) return false;
        if (r.targets && r.targets.some(t => u.name && (t.includes(u.name) || u.name.includes(t)))) return true;
        return false;
      });
    }

    const probeLatencies = matchingUpstreams.map(u => u.latency_ms).filter(l => typeof l === 'number' && l > 0);
    const tsP95 = (tsA && tsA.p95 && tsA.p95.length > 0) ? (tsA.p95[tsA.p95.length - 1] || 0) : 0;

    let lat = 2.5;
    if (probeLatencies.length > 0) {
      lat = avg(probeLatencies);
    } else if (tsP95 > 0) {
      lat = tsP95;
    }

    const statusCounts = (rawApiStatus && rawApiStatus.metrics && rawApiStatus.metrics.requests_by_status) || {};
    const err5xxCount = Object.entries(statusCounts).filter(([k]) => String(k).startsWith('5')).reduce((s, [, c]) => s + Number(c), 0);
    const errRateFromStatus = totalRequests > 0 ? (err5xxCount / totalRequests) : 0;
    const tsErr = (tsA && tsA.err && tsA.err.length > 0) ? (tsA.err[tsA.err.length - 1] || 0) : 0;
    const gatewayErr = tsErr > 0 ? tsErr : errRateFromStatus;

    let err = 0;
    if (matchingUpstreams.length > 0) {
      const failedTicks = matchingUpstreams.reduce((acc, u) => acc + (u.history ? u.history.filter(h => !h).length : (u.status === 'UNREACHABLE' || u.status === 'DOWN' || (u.http_code && u.http_code >= 500) ? 48 : 0)), 0);
      const totalTicks = matchingUpstreams.reduce((acc, u) => acc + (u.history ? u.history.length : 48), 0) || 1;
      err = failedTicks / totalTicks;
    } else {
      err = gatewayErr;
    }

    const slo = Math.max(50, Math.round(lat * 2 + 20));
    const mw = [];
    if (r.type === 'static') mw.push('static-cache', 'gzip', 'rfc9111');
    else mw.push('waf-guard', 'cors', 'request-id', 'keep-alive');
    if (r.algorithm) mw.push(r.algorithm);
    if (r.headers && Object.keys(r.headers).length > 0) mw.push('header-match');

    const s = { rps: [], p50: [], p95: [], p99: [], e3: [], e4: [], e5: [] };
    if (rawApiStatus && rawApiStatus.timeseries && rawApiStatus.timeseries.A && rawApiStatus.timeseries.A.rps && rawApiStatus.timeseries.A.rps.length > 0) {
      const routeShare = (totalExternalRequests > 0) ? (routeCumulative / totalExternalRequests) : (1 / Math.max(1, rawApiRoutes.length));
      for (let i = 0; i < N; i++) {
        s.rps.push(((tsA.rps && tsA.rps[i]) || 0) * routeShare);
        s.p50.push((tsA.p50 && tsA.p50[i]) || lat * 0.5);
        s.p95.push((tsA.p95 && tsA.p95[i]) || lat);
        s.p99.push((tsA.p99 && tsA.p99[i]) || lat * 1.5);
        s.e3.push(((tsA.e3 && tsA.e3[i]) || 0) * routeShare);
        s.e4.push(((tsA.e4 && tsA.e4[i]) || 0) * routeShare);
        s.e5.push(((tsA.e5 && tsA.e5[i]) || 0) * routeShare);
      }
    } else {
      for (let i = 0; i < N; i++) {
        s.rps.push(curRPS);
        s.p50.push(lat * 0.5);
        s.p95.push(lat);
        s.p99.push(lat * 1.5);
        s.e3.push(0);
        s.e4.push(0);
        s.e5.push(curRPS * err);
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
      totalReqs: routeCumulative,
      base: curRPS,
      lat,
      slo,
      err,
      mw,
      timeout: '10 s',
      s,
      cur: { rps: curRPS, totalReqs: routeCumulative, p50: lat * 0.5, p95: lat, p99: lat * 1.5, e4: 0, e5: curRPS * err, err5: err, err4: 0 }
    };
  }) : [
    ['orders', 'api /v1/orders', 'api.example.com', '/v1/orders/*', 'orders-svc', 420, 62, 120, .004],
    ['auth', 'api /v1/auth', 'api.example.com', '/v1/auth/*', 'auth-svc', 310, 34, 80, .002],
    ['search', 'api /v1/search', 'api.example.com', '/v1/search/*', 'search-svc', 260, 140, 250, .009],
    ['payments', 'api /v1/payments', 'api.example.com', '/v1/payments/*', 'payments-svc', 95, 210, 300, .004],
    ['app', 'app /', 'app.example.com', '/*', 'web-static', 640, 14, 40, .001]
  ].map(([id, short, host, path, pool, base, lat, slo, err]) => ({
    id, short, host, path, pool, base, lat, slo, err, mw: ['cors'], timeout: '8 s'
  })).map(rt => {
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
      const pad = (v, d = 0) => Array(padCount).fill(v !== undefined && v !== null ? v : d);
      ['rps', 'e2', 'e3', 'e4', 'e5', 'err'].forEach(k => { A[k] = [...pad(0), ...(tsA[k] || pad(0))]; });
      [['p50', 1], ['p95', 2.5], ['p99', 5]].forEach(([k, d]) => { A[k] = [...pad(tsA[k] && tsA[k][0], d), ...(tsA[k] || pad(d))]; });
      ['conns', 'egress', 'ev'].forEach(k => { X[k] = [...pad(0), ...(tsX[k] || pad(0))]; });
      [['gor', 10], ['heap', 1.5], ['cpu', 5], ['fd', 20], ['gc', 0.12]].forEach(([k, d]) => { X[k] = [...pad(tsX[k] && tsX[k][0], d), ...(tsX[k] || pad(d))]; });
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

      // 1. Strict route priority matching
      const routeMatches = routes.filter(r => {
        if (!u.route) return false;
        return r.short === u.route || r.path === u.route || (r.host + r.path) === u.route ||
               r.short.includes(u.route) || (r.host + r.path).includes(u.route) ||
               u.route.includes(r.path) || u.route.includes(r.short);
      });

      // Fallback matching on target address only if routeMatches is empty
      const fallbackMatches = routeMatches.length === 0 ? routes.filter(r => {
        if (r.pool && (r.pool.includes(targetAddr) || targetAddr.includes(r.pool) || r.pool.includes(u.name))) return true;
        if (r.targets && r.targets.some(t => t.includes(targetAddr) || targetAddr.includes(t) || t.includes(u.name))) return true;
        return false;
      }) : [];

      const primaryRoute = routeMatches.length > 0 ? routeMatches[0] : fallbackMatches[0];
      const poolKey = primaryRoute ? primaryRoute.id : (u.route ? u.route.replace(/[^a-zA-Z0-9]/g, '_') : targetAddr.replace(/[^a-zA-Z0-9]/g, '_'));
      const host = primaryRoute ? (primaryRoute.host || '*') : '*';
      const path = primaryRoute ? primaryRoute.path : (u.route ? (u.route.includes('/') ? '/' + u.route.split('/').slice(1).join('/') : u.route) : '/');
      const displayName = primaryRoute ? primaryRoute.short : (u.route || targetAddr);

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
          routes: primaryRoute ? [primaryRoute] : (fallbackMatches.length > 0 ? fallbackMatches : [])
        });
      }

      const pool = poolMap.get(poolKey);
      if (primaryRoute) {
        if (!pool.routes.some(r => r.id === primaryRoute.id)) pool.routes.push(primaryRoute);
      } else {
        fallbackMatches.forEach(mr => {
          if (!pool.routes.some(r => r.id === mr.id)) pool.routes.push(mr);
        });
      }

      // Instance health state classification
      const isDown = u.status === 'UNREACHABLE' || u.status === 'DOWN' || (u.http_code && u.http_code >= 500);
      const isUp = (u.status === 'HEALTHY' || u.status === 'UP') && (!u.http_code || u.http_code < 400);
      const instState = isDown ? 'down' : (isUp ? 'up' : 'warn');

      if (!pool.insts.some(i => i.a === targetAddr)) {
        pool.insts.push({
          a: targetAddr,
          route: u.route || (primaryRoute ? primaryRoute.short : ''),
          state: instState,
          httpCode: u.http_code,
          latency: (u.latency_ms !== undefined && u.latency_ms !== null) ? u.latency_ms : 0,
          history: (u.history && u.history.length) ? u.history : Array(48).fill(instState !== 'down')
        });
      }
    });

    // 4. Exhaustive Fallback Route Loop
    const addDefaultPool = (r, key) => {
      const headersList = r.headers ? Object.entries(r.headers).map(([k, v]) => `${k}=${v}`).join(', ') : '';
      const targetAddr = (r.targets && r.targets.length > 0) ? r.targets[0] : (r.pool || 'in-process');
      poolMap.set(key, {
        id: key,
        displayName: r.short || key,
        host: r.host || '*',
        path: r.path || '/',
        type: r.type || 'proxy',
        algo: r.algorithm || 'Round-Robin',
        hc: 'Passive health check',
        headersSummary: headersList,
        insts: [{ a: targetAddr, route: r.short, state: 'up', history: Array(48).fill(true) }],
        routes: [r]
      });
    };

    routes.forEach(r => {
      if (r.type === 'static' && (!r.targets || r.targets.length === 0)) return;
      if (!poolMap.has(r.id)) addDefaultPool(r, r.id);
    });
  } else {
    routes.forEach(r => {
      const key = r.id || r.short;
      const headersList = r.headers ? Object.entries(r.headers).map(([k, v]) => `${k}=${v}`).join(', ') : '';
      const targetAddr = (r.targets && r.targets.length > 0) ? r.targets[0] : (r.pool || 'in-process');
      poolMap.set(key, {
        id: key,
        displayName: r.short || key,
        host: r.host || '*',
        path: r.path || '/',
        type: r.type || 'proxy',
        algo: r.algorithm || 'Round-Robin',
        hc: 'Passive health check',
        headersSummary: headersList,
        insts: [{ a: targetAddr, route: r.short, state: 'up', history: Array(48).fill(true) }],
        routes: [r]
      });
    });
  }

  const pools = Array.from(poolMap.values()).map(p => {
    const rs = p.routes;
    const rps = sum(rs.map(r => r.cur.rps));
    const totalReqs = sum(rs.map(r => r.totalReqs || (r.cur && r.cur.totalReqs) || 0));

    const insts = (p.insts && p.insts.length > 0) ? p.insts : [
      { a: `${p.id}`, state: 'up', history: Array(48).fill(true) }
    ];

    // Derive p95 from active instance probe latencies, or route p95, or gateway time-series
    const activeProbeLats = insts.map(i => i.latency).filter(l => typeof l === 'number' && l > 0);
    const tsA = rawApiStatus && rawApiStatus.timeseries && rawApiStatus.timeseries.A;
    const tsP95 = (tsA && tsA.p95 && tsA.p95.length > 0) ? (tsA.p95[tsA.p95.length - 1] || 0) : 0;
    const p95 = activeProbeLats.length > 0
      ? Math.max(...activeProbeLats)
      : (rs.length > 0 ? avg(rs.map(r => r.cur.p95)) : (tsP95 || 2.5));

    // Derive 5xx error rate and state
    const up = insts.filter(i => i.state === 'up').length;
    const down = insts.filter(i => i.state === 'down').length;
    const warn = insts.filter(i => i.state === 'warn').length;

    const failedTicks = insts.reduce((acc, i) => acc + (i.history ? i.history.filter(h => !h).length : 0), 0);
    const totalTicks = insts.reduce((acc, i) => acc + (i.history ? i.history.length : 0), 0) || 1;
    const probeFailureRate = failedTicks / totalTicks;
    const routeErrAvg = rs.length > 0 ? avg(rs.map(r => r.cur.err5)) : 0;
    const err5 = Math.max(routeErrAvg, probeFailureRate);
    const tone = (down > 0 || err5 >= 0.02) ? 'err' : (warn > 0 || err5 > 0 ? 'warn' : 'ok');

    return { ...p, insts, rps, totalReqs, p95, err5, up, down, warn, tone };
  });

  const total = sum(routes.map(r => r.cur.rps));
  const certs = (rawApiStatus && rawApiStatus.certs) || ['sayantansaha.in', 'toron.in'].map(d => ({ id: d.split('.')[0], domain: d, chal: 'HTTP-01', days: 85, state: 'valid' }));
  const modules = (rawApiStatus && rawApiStatus.modules) || [
    ['listener.http', 'Listener', .18, 'Listening on :443 / :80'],
    ['router', 'Routing', .18, `${routes.length} routes active`],
    ['waf.owasp', 'Security', .09, 'OWASP Core Rules active'],
    ['proxy.reverse', 'Proxy', .2, 'Zero-allocation reverse proxy']
  ].map(([id, kind, w, note]) => ({ id, kind, w, state: 'running', note }));

  const alerts = [];

  // 1. Upstream pool health alerts
  pools.forEach(p => {
    if (p.down > 0) {
      alerts.push({
        id: `upstream_${p.id}`,
        sev: 'critical',
        title: `Upstream Degradation: ${p.displayName || p.id}`,
        timestamp: Date.now() - 60000,
        ip: '',
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
        ip: '',
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
        ip: '',
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
        ip: inc.client_ip || '',
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
        ip: ban.ip || '',
        go: 'alerts',
        detail: () => ban.reason || `IP address has been banned due to repeated security violations`
      });
    });
  }

  // Multi-level sort alerts: 1) timestamp descending (latest first), 2) client IP ascending
  alerts.sort((a, b) => {
    const dt = (b.timestamp || 0) - (a.timestamp || 0);
    if (dt !== 0) return dt;
    return (a.ip || '').localeCompare(b.ip || '', undefined, { numeric: true });
  });

  const computed = {
    routes,
    pools,
    A,
    X,
    labels,
    B,
    total,
    redir: total * 0.06,
    certs,
    modules,
    alerts,
    bannedIps: rawApiBannedIps
  };

  setCache(computed);
  return computed;
}
