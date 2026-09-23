/**
 * Automated Verification Suite for REQ-139 / TASK-162 / TC-139 (v1.1)
 * Pure Node.js runner with simulated DOM scope.
 */
const assert = require('assert');
const fs = require('fs');
const path = require('path');
const vm = require('vm');

// 1. Canonical Fixtures from TC-139 v1.1
const fixtureRoutes = [
  {
    id: "_kite_callback",
    prefix: "/kite/callback",
    host: "sayantansaha.in",
    type: "proxy",
    targets: ["http://127.0.0.1:8080"],
    algorithm: "Round-Robin",
    headers: { "X-Service": "kite-callback" }
  },
  {
    id: "_kite_api",
    prefix: "/kite/api",
    host: "sayantansaha.in",
    type: "proxy",
    targets: ["http://127.0.0.1:8080"],
    algorithm: "Round-Robin",
    headers: { "X-Service": "kite-api" }
  },
  {
    id: "_kite_broker",
    prefix: "/kite/broker",
    host: "sayantansaha.in",
    type: "proxy",
    targets: ["http://127.0.0.1:8080"],
    algorithm: "Round-Robin",
    headers: { "X-Service": "kite-broker" }
  },
  {
    id: "_kite_auth",
    prefix: "/kite/auth",
    host: "sayantansaha.in",
    type: "proxy",
    targets: ["http://127.0.0.1:8080"],
    algorithm: "Round-Robin",
    headers: { "X-Service": "kite-auth" }
  },
  {
    id: "_kite_orders",
    prefix: "/kite/orders",
    host: "sayantansaha.in",
    type: "proxy",
    targets: ["http://10.0.2.20:8080"],
    algorithm: "Least-Connections",
    headers: {}
  }
];

const fixtureUpstreams = [
  {
    name: "127.0.0.1:8080",
    port: 8080,
    route: "/kite/callback",
    status: "HEALTHY",
    http_code: 200,
    latency_ms: 3.42,
    history: Array(48).fill(true)
  },
  {
    name: "127.0.0.1:8080",
    port: 8080,
    route: "/kite/api",
    status: "HEALTHY",
    http_code: 200,
    latency_ms: 18.75,
    history: Array(48).fill(true)
  },
  {
    name: "127.0.0.1:8080",
    port: 8080,
    route: "/kite/broker",
    status: "UNREACHABLE",
    http_code: 502,
    latency_ms: 45.10,
    history: [...Array(36).fill(true), ...Array(12).fill(false)]
  },
  {
    name: "127.0.0.1:8080",
    port: 8080,
    route: "/kite/auth",
    status: "HEALTHY",
    http_code: 200,
    latency_ms: 1.20,
    history: Array(48).fill(true)
  }
];

const fixtureStatus = {
  version: "1.5.30",
  metrics: {
    total_requests: 58500,
    requests_by_route: {
      "/kite/callback": 5000,
      "/kite/api/v1/business": 1800,
      "/kite/api/v1/trades": 800,
      "/kite/api/v1/auth": 400,
      "/kite/broker": 2000,
      "/kite/auth": 0,
      "/internal/api/status": 25000,
      "/internal/api/routes": 12000,
      "/internal/api/upstreams/health": 11000,
      "/internal/api/logs": 500
    },
    requests_by_status: {
      "200": 58300,
      "502": 200
    }
  },
  timeseries: {
    labels: ["14:30:00", "14:30:02", "14:30:04", "14:30:06", "14:30:08"],
    A: {
      rps: [110.0, 130.0, 105.0, 135.0, 120.0],
      p50: [4.0, 4.2, 4.5, 4.3, 4.5],
      p95: [12.0, 13.5, 14.8, 14.2, 14.8],
      p99: [25.0, 28.0, 30.0, 29.0, 30.0],
      e2: [108.0, 128.0, 103.0, 133.0, 117.0],
      e3: [0.0, 0.0, 0.0, 0.0, 0.0],
      e4: [0.0, 0.0, 0.0, 0.0, 0.0],
      e5: [2.0, 2.0, 2.0, 2.0, 3.0],
      err: [0.018, 0.015, 0.019, 0.015, 0.025]
    },
    X: {
      conns: [45, 48, 50, 49, 50],
      egress: [15.2, 16.0, 16.8, 16.5, 16.8],
      gor: [32, 32, 34, 34, 34],
      heap: [12.4, 12.8, 13.1, 13.0, 13.1],
      gc: [0.15, 0.14, 0.16, 0.15, 0.16],
      cpu: [6.2, 6.5, 7.0, 6.8, 7.0],
      fd: [64, 64, 68, 66, 68],
      ev: [350, 360, 380, 370, 380]
    }
  }
};

// 2. Load app.js and create a sandbox environment
const appJsPath = path.resolve(__dirname, '../public/app.js');
const appCode = fs.readFileSync(appJsPath, 'utf8');

function createSandbox() {
  const domElements = new Map();
  function getEl(sel) {
    if (!domElements.has(sel)) {
      domElements.set(sel, {
        innerHTML: '',
        textContent: '',
        value: '',
        dataset: {},
        attributes: {},
        classList: {
          add: () => {},
          remove: () => {},
          toggle: () => {},
          contains: () => false
        },
        setAttribute(k, v) { this.attributes[k] = v; },
        getAttribute(k) { return this.attributes[k]; },
        querySelector(s) { return getEl(s); },
        querySelectorAll() { return []; },
        addEventListener() {},
        matches() { return false; },
        closest() { return null; }
      });
    }
    return domElements.get(sel);
  }

  const sandbox = {
    console,
    Date,
    Math,
    Array,
    Object,
    String,
    Number,
    Boolean,
    RegExp,
    Set,
    Map,
    Promise,
    document: {
      querySelector: getEl,
      querySelectorAll: () => [],
      documentElement: { getAttribute: () => 'dark', setAttribute: () => {} },
      body: { classList: { add: () => {}, remove: () => {} } },
      addEventListener: () => {}
    },
    window: {
      matchMedia: () => ({ matches: true }),
      addEventListener: () => {},
      scrollTo: () => {},
      location: { hash: '#/overview' },
      history: { replaceState: () => {} },
      performance: { now: () => Date.now() },
      requestAnimationFrame: fn => fn()
    },
    location: { hash: '#/overview' },
    localStorage: { setItem: () => {}, getItem: () => null },
    setInterval: () => {},
    setTimeout: () => {},
    fetch: () => Promise.resolve({ ok: true, json: () => Promise.resolve({}) })
  };

  // Rewrite closure to expose internals for testing
  let testCode = appCode;
  testCode = testCode.replace(
    /\}\)\(\);\s*$/,
    `
    globalThis.__test_exports__ = {
      buildDataModel,
      poolCard,
      flow,
      rtUpdate,
      upUpdate,
      lgUpdate,
      setRawApiData: (s, r, u, l) => {
        rawApiStatus = s;
        rawApiRoutes = r;
        rawApiUpstreams = u;
        if (l) rawApiLogs = l;
        invalidate();
      },
      getState: () => state
    };
  })();`
  );

  const ctx = vm.createContext(sandbox);
  vm.runInContext(testCode, ctx);
  return { exports: sandbox.__test_exports__, domElements, sandbox };
}

console.log('Running TC-139 (v1.1) Verification Suite...\n');

// -------------------------------------------------------------
// TC-139-01: Distinct Upstream Route Pool Rendering for Shared Target IP:Port
// -------------------------------------------------------------
{
  const { exports } = createSandbox();
  exports.setRawApiData(fixtureStatus, fixtureRoutes, fixtureUpstreams);

  const D = exports.buildDataModel();
  assert.ok(D.pools.length >= 4, `Expected >= 4 pools, got ${D.pools.length}`);

  const poolCallback = D.pools.find(p => p.id === '_kite_callback');
  const poolApi = D.pools.find(p => p.id === '_kite_api');
  const poolBroker = D.pools.find(p => p.id === '_kite_broker');
  const poolAuth = D.pools.find(p => p.id === '_kite_auth');

  assert.ok(poolCallback, 'Pool _kite_callback must exist');
  assert.ok(poolApi, 'Pool _kite_api must exist');
  assert.ok(poolBroker, 'Pool _kite_broker must exist');
  assert.ok(poolAuth, 'Pool _kite_auth must exist');

  assert.strictEqual(poolCallback.routes.length, 1, 'poolCallback must isolate its own route');
  assert.strictEqual(poolCallback.routes[0].id, '_kite_callback');
  assert.strictEqual(poolApi.routes.length, 1, 'poolApi must isolate its own route');
  assert.strictEqual(poolApi.routes[0].id, '_kite_api');
  assert.strictEqual(poolBroker.routes.length, 1, 'poolBroker must isolate its own route');
  assert.strictEqual(poolBroker.routes[0].id, '_kite_broker');
  assert.strictEqual(poolAuth.routes.length, 1, 'poolAuth must isolate its own route');
  assert.strictEqual(poolAuth.routes[0].id, '_kite_auth');

  // DOM check via poolCard
  const htmlCb = exports.poolCard(poolCallback);
  assert.ok(htmlCb.includes('id="pool-_kite_callback"'), 'DOM ID pool-_kite_callback must exist');
  assert.ok(htmlCb.includes('sayantansaha.in') && htmlCb.includes('/kite/callback'), 'Match host and prefix in header');
  assert.ok(htmlCb.includes('X-Service=kite-callback'), 'Header chip X-Service=kite-callback must exist');

  const htmlApi = exports.poolCard(poolApi);
  assert.ok(htmlApi.includes('id="pool-_kite_api"'));
  assert.ok(htmlApi.includes('sayantansaha.in') && htmlApi.includes('/kite/api'));
  assert.ok(htmlApi.includes('X-Service=kite-api'));

  console.log('  ✔ TC-139-01: Distinct Upstream Route Pool Rendering PASSED');
}

// -------------------------------------------------------------
// TC-139-02: Fallback Route Pool Creation for Unmapped Upstream Routes
// -------------------------------------------------------------
{
  const { exports } = createSandbox();
  exports.setRawApiData(fixtureStatus, fixtureRoutes, fixtureUpstreams);

  const D = exports.buildDataModel();
  const pOrders = D.pools.find(p => p.id === '_kite_orders');
  assert.ok(pOrders, 'Fallback pool _kite_orders must exist');
  assert.strictEqual(pOrders.displayName, 'sayantansaha.in /kite/orders');
  assert.strictEqual(pOrders.path, '/kite/orders');
  assert.strictEqual(pOrders.algo, 'Least-Connections');
  assert.strictEqual(pOrders.hc, 'Passive health check');
  assert.strictEqual(pOrders.insts.length, 1);
  assert.strictEqual(pOrders.insts[0].a, 'http://10.0.2.20:8080');
  assert.strictEqual(pOrders.insts[0].state, 'up');
  assert.strictEqual(pOrders.insts[0].history.length, 48);

  const htmlOrders = exports.poolCard(pOrders);
  assert.ok(htmlOrders.includes('id="pool-_kite_orders"'));

  console.log('  ✔ TC-139-02: Fallback Route Pool Creation PASSED');
}

// -------------------------------------------------------------
// TC-139-03: Hierarchical Subpath Rollup and Prefix Aggregation
// -------------------------------------------------------------
{
  const { exports } = createSandbox();
  // Include a sibling prefix /kite/api-v2/other that should NOT be aggregated into /kite/api
  const statusWithSiblings = {
    ...fixtureStatus,
    metrics: {
      ...fixtureStatus.metrics,
      requests_by_route: {
        ...fixtureStatus.metrics.requests_by_route,
        "/kite/api-v2/other": 500
      }
    }
  };
  exports.setRawApiData(statusWithSiblings, fixtureRoutes, fixtureUpstreams);

  const D = exports.buildDataModel();
  const rApi = D.routes.find(r => r.path === '/kite/api');
  const rCb = D.routes.find(r => r.path === '/kite/callback');
  const rBr = D.routes.find(r => r.path === '/kite/broker');
  const rAu = D.routes.find(r => r.path === '/kite/auth');

  // /kite/api aggregates /kite/api/v1/business (1800) + /kite/api/v1/trades (800) + /kite/api/v1/auth (400) = 3000
  assert.strictEqual(rApi.totalReqs, 3000, `Expected 3000, got ${rApi.totalReqs}`);
  assert.strictEqual(rCb.totalReqs, 5000, `Expected 5000, got ${rCb.totalReqs}`);
  assert.strictEqual(rBr.totalReqs, 2000, `Expected 2000, got ${rBr.totalReqs}`);
  assert.strictEqual(rAu.totalReqs, 0, `Expected 0, got ${rAu.totalReqs}`);

  console.log('  ✔ TC-139-03: Hierarchical Subpath Rollup PASSED');
}

// -------------------------------------------------------------
// TC-139-04: Internal Telemetry Polling Isolation and Denominator Normalization
// -------------------------------------------------------------
{
  const { exports } = createSandbox();
  exports.setRawApiData(fixtureStatus, fixtureRoutes, fixtureUpstreams);

  const D = exports.buildDataModel();
  const rApi = D.routes.find(r => r.path === '/kite/api');
  const rCb = D.routes.find(r => r.path === '/kite/callback');
  const rBr = D.routes.find(r => r.path === '/kite/broker');

  // Total external requests = 5000 + 1800 + 800 + 400 + 2000 + 0 = 10000 (excluding 48500 internal requests)
  // smoothedRPS = avg(110, 130, 105, 135, 120) = 120.0
  // curRPS(callback) = 120 * (5000 / 10000) = 60.0
  // curRPS(api) = 120 * (3000 / 10000) = 36.0
  // curRPS(broker) = 120 * (2000 / 10000) = 24.0
  assert.strictEqual(rCb.cur.rps, 60.0, `Expected 60.0, got ${rCb.cur.rps}`);
  assert.strictEqual(rApi.cur.rps, 36.0, `Expected 36.0, got ${rApi.cur.rps}`);
  assert.strictEqual(rBr.cur.rps, 24.0, `Expected 24.0, got ${rBr.cur.rps}`);

  // Test Boundary: all requests are internal
  const statusOnlyInternal = {
    metrics: {
      total_requests: 1000,
      requests_by_route: { "/internal/api/status": 1000 }
    },
    timeseries: { A: { rps: [10.0] } }
  };
  exports.setRawApiData(statusOnlyInternal, fixtureRoutes, fixtureUpstreams);
  const DInternal = exports.buildDataModel();
  DInternal.routes.forEach(r => {
    assert.ok(!isNaN(r.cur.rps), 'curRPS must not be NaN when all requests are internal');
  });

  console.log('  ✔ TC-139-04: Internal Polling Isolation PASSED');
}

// -------------------------------------------------------------
// TC-139-05: Instantaneous RPS Temporal Smoothing via Moving Average Window
// -------------------------------------------------------------
{
  const { exports } = createSandbox();

  // Scenario A: Active Traffic with 5 points [110.0, 130.0, 105.0, 135.0, 120.0] -> avg = 120.0
  exports.setRawApiData(fixtureStatus, fixtureRoutes, fixtureUpstreams);
  const DA = exports.buildDataModel();
  const sumRpsA = DA.routes.reduce((acc, r) => acc + r.cur.rps, 0);
  assert.strictEqual(Math.round(sumRpsA), 120, `Expected sum 120, got ${sumRpsA}`);

  // Scenario B: Oscillating Low-Traffic test [0.0, 6.0, 0.0, 6.0, 0.0] -> avg = 2.4
  const statusOscillating = {
    ...fixtureStatus,
    timeseries: {
      ...fixtureStatus.timeseries,
      A: {
        ...fixtureStatus.timeseries.A,
        rps: [0.0, 6.0, 0.0, 6.0, 0.0]
      }
    }
  };
  exports.setRawApiData(statusOscillating, fixtureRoutes, fixtureUpstreams);
  const DB = exports.buildDataModel();
  const sumRpsB = DB.routes.reduce((acc, r) => acc + r.cur.rps, 0);
  assert.ok(Math.abs(sumRpsB - 2.4) < 1e-6, `Expected sum 2.4, got ${sumRpsB}`);

  // Scenario C: Fewer than 5 points [100.0, 120.0] -> avg = 110.0
  const statusShort = {
    ...fixtureStatus,
    timeseries: {
      ...fixtureStatus.timeseries,
      A: {
        ...fixtureStatus.timeseries.A,
        rps: [100.0, 120.0]
      }
    }
  };
  exports.setRawApiData(statusShort, fixtureRoutes, fixtureUpstreams);
  const DC = exports.buildDataModel();
  const sumRpsC = DC.routes.reduce((acc, r) => acc + r.cur.rps, 0);
  assert.ok(Math.abs(sumRpsC - 110.0) < 1e-6, `Expected sum 110.0, got ${sumRpsC}`);

  console.log('  ✔ TC-139-05: Moving Average RPS Temporal Smoothing PASSED');
}

// -------------------------------------------------------------
// TC-139-06: Dual Rate and Cumulative Volume Display in Upstream Pool Cards
// -------------------------------------------------------------
{
  const { exports } = createSandbox();
  exports.setRawApiData(fixtureStatus, fixtureRoutes, fixtureUpstreams);

  const D = exports.buildDataModel();
  const pCb = D.pools.find(p => p.id === '_kite_callback');
  const pApi = D.pools.find(p => p.id === '_kite_api');
  const pBr = D.pools.find(p => p.id === '_kite_broker');
  const pAu = D.pools.find(p => p.id === '_kite_auth');

  assert.strictEqual(pCb.totalReqs, 5000);
  assert.strictEqual(pApi.totalReqs, 3000);
  assert.strictEqual(pBr.totalReqs, 2000);
  assert.strictEqual(pAu.totalReqs, 0);

  const cardCb = exports.poolCard(pCb);
  assert.ok(cardCb.includes('60/s') || cardCb.includes('60.0/s'), `Expected rate in card: ${cardCb}`);
  assert.ok(cardCb.includes('total)'), `Expected cumulative volume in card: ${cardCb}`);
  assert.ok(cardCb.includes('5.00k total') || cardCb.includes('5.0k total'));

  const cardApi = exports.poolCard(pApi);
  assert.ok(cardApi.includes('36/s') || cardApi.includes('36.0/s'));
  assert.ok(cardApi.includes('3.00k total') || cardApi.includes('3.0k total'));

  // Test idle pool with historical traffic (e.g. 1700 total requests)
  const idlePool = { ...pAu, id: '_kite_legacy', displayName: 'legacy-service', rps: 0.0, totalReqs: 1700 };
  const cardLegacy = exports.poolCard(idlePool);
  assert.ok(cardLegacy.includes('0.0/s'));
  assert.ok(cardLegacy.includes('1.70k total') || cardLegacy.includes('1.7k total'), `Expected 1.7k total in ${cardLegacy}`);

  // Test brand new pool with 0 requests
  const cardAu = exports.poolCard(pAu);
  assert.ok(cardAu.includes('0.0/s'));
  assert.ok(!cardAu.includes('(0 total)'), 'Should not show (0 total) for 0 requests');

  console.log('  ✔ TC-139-06: Dual Rate and Cumulative Volume Display PASSED');
}

// -------------------------------------------------------------
// TC-139-07: Live Health Probe Latency Binding
// -------------------------------------------------------------
{
  const { exports } = createSandbox();
  exports.setRawApiData(fixtureStatus, fixtureRoutes, fixtureUpstreams);

  const D = exports.buildDataModel();
  const pCb = D.pools.find(p => p.id === '_kite_callback');
  const pApi = D.pools.find(p => p.id === '_kite_api');
  const pBr = D.pools.find(p => p.id === '_kite_broker');
  const pAu = D.pools.find(p => p.id === '_kite_auth');

  assert.strictEqual(pCb.insts[0].latency, 3.42);
  assert.strictEqual(pApi.insts[0].latency, 18.75);
  assert.strictEqual(pBr.insts[0].latency, 45.10);
  assert.strictEqual(pAu.insts[0].latency, 1.20);

  const cardCb = exports.poolCard(pCb);
  assert.ok(cardCb.includes('Latency: <b>3.4 ms</b>'), `Got: ${cardCb}`);
  const cardApi = exports.poolCard(pApi);
  assert.ok(cardApi.includes('Latency: <b>18.8 ms</b>'), `Got: ${cardApi}`);
  const cardBr = exports.poolCard(pBr);
  assert.ok(cardBr.includes('Latency: <b>45.1 ms</b>'), `Got: ${cardBr}`);
  const cardAu = exports.poolCard(pAu);
  assert.ok(cardAu.includes('Latency: <b>1.2 ms</b>'), `Got: ${cardAu}`);

  console.log('  ✔ TC-139-07: Live Health Probe Latency Binding PASSED');
}

// -------------------------------------------------------------
// TC-139-08: Dynamic p95 Latency Derivation Without Static Fallbacks
// -------------------------------------------------------------
{
  // TC-139-08a: Probe Latencies Available
  const { exports: exA } = createSandbox();
  exA.setRawApiData(fixtureStatus, fixtureRoutes, fixtureUpstreams);
  const DA = exA.buildDataModel();
  const rApi = DA.routes.find(r => r.path === '/kite/api');
  const pApi = DA.pools.find(p => p.id === '_kite_api');
  assert.strictEqual(rApi.cur.p95, 18.75);
  assert.strictEqual(pApi.p95, 18.75);
  assert.notStrictEqual(pApi.p95, 2.5);

  // TC-139-08b: Probe Latency Absent, Gateway Time-Series Available
  const { exports: exB } = createSandbox();
  const upstreamsNoLat = fixtureUpstreams.map(u => ({ ...u, latency_ms: 0 }));
  exB.setRawApiData(fixtureStatus, fixtureRoutes, upstreamsNoLat);
  const DB = exB.buildDataModel();
  const pApiB = DB.pools.find(p => p.id === '_kite_api');
  assert.strictEqual(pApiB.p95, 14.8, `Expected 14.8, got ${pApiB.p95}`);

  // TC-139-08c: Cold Start Initial Nominal Fallback
  const { exports: exC } = createSandbox();
  const statusEmpty = {
    metrics: {},
    timeseries: { A: { rps: [], p95: [], err: [] } }
  };
  exC.setRawApiData(statusEmpty, fixtureRoutes, upstreamsNoLat);
  const DC = exC.buildDataModel();
  const pApiC = DC.pools.find(p => p.id === '_kite_api');
  assert.strictEqual(pApiC.p95, 2.5);

  console.log('  ✔ TC-139-08: Dynamic p95 Latency Derivation PASSED');
}

// -------------------------------------------------------------
// TC-139-09: Dynamic Error Rate Calculation and Visual Tone Escalation
// -------------------------------------------------------------
{
  const { exports } = createSandbox();
  exports.setRawApiData(fixtureStatus, fixtureRoutes, fixtureUpstreams);

  const D = exports.buildDataModel();
  const pCb = D.pools.find(p => p.id === '_kite_callback');
  assert.strictEqual(pCb.insts[0].state, 'up');
  assert.strictEqual(pCb.err5, 0.0);
  assert.strictEqual(pCb.tone, 'ok');

  const cardCb = exports.poolCard(pCb);
  assert.ok(cardCb.includes('1 of 1 up'), `Expected 1 of 1 up in ${cardCb}`);
  assert.ok(cardCb.includes('0.0%'), `Expected 0.0% in ${cardCb}`);
  assert.ok(cardCb.includes('HTTP 200 OK'));

  const pBr = D.pools.find(p => p.id === '_kite_broker');
  assert.strictEqual(pBr.insts[0].state, 'down');
  assert.strictEqual(pBr.down, 1);
  assert.strictEqual(pBr.err5, 0.25);
  assert.strictEqual(pBr.tone, 'err');

  const cardBr = exports.poolCard(pBr);
  assert.ok(cardBr.includes('0 of 1 up'));
  assert.ok(cardBr.includes('25.0%'));
  assert.ok(cardBr.includes('Unreachable'));
  assert.ok(cardBr.includes('Circuit breaker open'));

  // Alert generation
  const alertBr = D.alerts.find(a => a.id === 'upstream__kite_broker');
  assert.ok(alertBr, 'Upstream alert must be generated for failing broker pool');
  assert.strictEqual(alertBr.sev, 'critical');
  assert.strictEqual(alertBr.title, 'Upstream Degradation: sayantansaha.in /kite/broker');

  console.log('  ✔ TC-139-09: Dynamic Error Rate and Tone Escalation PASSED');
}

// -------------------------------------------------------------
// TC-139-10: Edge Case Resilience and Boundary Testing
// -------------------------------------------------------------
{
  const { exports } = createSandbox();

  // TC-139-10a: Gateway Cold Startup
  exports.setRawApiData(null, [], []);
  const Da = exports.buildDataModel();
  assert.ok(Da.routes.length > 0, 'Should fall back to seed mock routes');

  // TC-139-10b: Zero Recorded Requests
  exports.setRawApiData({ metrics: { total_requests: 0 }, timeseries: { A: { rps: [50.0] } } }, fixtureRoutes, fixtureUpstreams);
  const Db = exports.buildDataModel();
  Db.routes.forEach(r => {
    assert.ok(!isNaN(r.cur.rps), 'rps must not be NaN');
    assert.ok(isFinite(r.cur.rps), 'rps must be finite');
  });

  // TC-139-10c: Missing Metric Counter Map
  exports.setRawApiData({ metrics: {} }, fixtureRoutes, fixtureUpstreams);
  const Dc = exports.buildDataModel();
  assert.ok(Dc.routes.length > 0);

  // TC-139-10e: Special Route Characters
  const specialRoutes = [{ id: 'route_special', prefix: '/kite/broker-api/v2.1', host: 'api.io', type: 'proxy', targets: ['http://127.0.0.1:8080'] }];
  exports.setRawApiData(fixtureStatus, specialRoutes, [{ route: '/kite/broker-api/v2.1', name: '127.0.0.1:8080', port: 8080, latency_ms: 10, status: 'HEALTHY' }]);
  const De = exports.buildDataModel();
  assert.strictEqual(De.pools[0].id, 'route_special');

  // TC-139-10f: Null Probe Latency / Status
  exports.setRawApiData(fixtureStatus, fixtureRoutes, [{ route: '/kite/callback', latency_ms: null, status: '' }]);
  const Df = exports.buildDataModel();
  const cardDf = exports.poolCard(Df.pools[0]);
  assert.ok(!cardDf.includes('NaN'), 'card must not contain NaN');

  console.log('  ✔ TC-139-10: Edge Case Resilience PASSED');
}

// -------------------------------------------------------------
// TC-139-11: Overview Traffic Flow Sankey Diagram Rendering Parity
// -------------------------------------------------------------
{
  const { exports } = createSandbox();
  exports.setRawApiData(fixtureStatus, fixtureRoutes, fixtureUpstreams);

  const D = exports.buildDataModel();
  const host = { innerHTML: '', matches: () => false, querySelector: () => null };
  exports.flow(host, D);

  const svg = host.innerHTML;
  assert.ok(svg.includes('data-r="_kite_callback"'), 'Link for _kite_callback must exist in flow');
  assert.ok(svg.includes('data-r="_kite_api"'), 'Link for _kite_api must exist in flow');
  assert.ok(svg.includes('data-r="_kite_broker"'), 'Link for _kite_broker must exist in flow');
  assert.ok(svg.includes('data-r="_kite_auth"'), 'Link for _kite_auth must exist in flow');

  assert.ok(svg.includes('sayantansaha.in /kite/callback'), 'Label for callback pool');
  assert.ok(svg.includes('sayantansaha.in /kite/api'), 'Label for api pool');
  assert.ok(svg.includes('sayantansaha.in /kite/broker'), 'Label for broker pool');
  assert.ok(svg.includes('sayantansaha.in /kite/auth'), 'Label for auth pool');

  // Broker ribbon carries err tone
  assert.ok(svg.includes('class="rb err" data-r="_kite_broker"'), 'Broker ribbon must carry class "rb err"');

  console.log('  ✔ TC-139-11: Overview Traffic Flow Parity PASSED');
}

// -------------------------------------------------------------
// TC-139-12: Routes View Table Metric Parity and Multi-Level Sorting
// -------------------------------------------------------------
{
  const { exports, domElements } = createSandbox();
  exports.setRawApiData(fixtureStatus, fixtureRoutes, fixtureUpstreams);

  const D = exports.buildDataModel();
  exports.rtUpdate(D);
  const bodyEl = domElements.get('#rtBody');
  const rowsHtml = bodyEl.innerHTML;

  assert.ok(rowsHtml.includes('60/s') || rowsHtml.includes('60.0/s'), 'Callback 60/s');
  assert.ok(rowsHtml.includes('3.4 ms'), 'Callback 3.4 ms');
  assert.ok(rowsHtml.includes('0.00%'), 'Callback 0.00%');

  assert.ok(rowsHtml.includes('36/s') || rowsHtml.includes('36.0/s'), 'Api 36/s');
  assert.ok(rowsHtml.includes('18.8 ms'), 'Api 18.8 ms');

  assert.ok(rowsHtml.includes('24/s') || rowsHtml.includes('24.0/s'), 'Broker 24/s');
  assert.ok(rowsHtml.includes('45.1 ms'), 'Broker 45.1 ms');
  assert.ok(rowsHtml.includes('25.00%'), 'Broker 25.00%');
  assert.ok(rowsHtml.includes('Degraded'), 'Broker status Degraded');

  // Test sorting by err descending
  const state = exports.getState();
  state.sort = { k: 'err', dir: -1 };
  exports.rtUpdate(D);
  const sortedHtml = bodyEl.innerHTML;
  const idxBroker = sortedHtml.indexOf('_kite_broker');
  const idxCallback = sortedHtml.indexOf('_kite_callback');
  assert.ok(idxBroker < idxCallback, 'Broker (25% err) must appear before Callback (0% err) when sorted by err desc');

  console.log('  ✔ TC-139-12: Routes View Table Metric Parity PASSED');
}

// -------------------------------------------------------------
// TC-139-13: Non-Functional Compliance: Performance Budget, Zero Dependencies & Relative Links
// -------------------------------------------------------------
{
  const { exports } = createSandbox();
  exports.setRawApiData(fixtureStatus, fixtureRoutes, fixtureUpstreams);

  // Performance benchmark (NFR-2): 100 runs
  const iters = 100;
  const start = Date.now();
  for (let i = 0; i < iters; i++) {
    exports.setRawApiData(fixtureStatus, fixtureRoutes, fixtureUpstreams);
    exports.buildDataModel();
  }
  const totalMs = Date.now() - start;
  const avgMs = totalMs / iters;
  assert.ok(avgMs < 2.0, `buildDataModel() average execution time ${avgMs}ms exceeds 2.0ms budget`);

  // Strictly Relative Markdown Links (NFR-4)
  const docFiles = [
    'docs/requirements/REQ-139.md',
    'docs/tasks/TASK-162.md',
    'docs/architecture/ADR-139.md',
    'docs/testCases/TC-139.md'
  ];

  const absPathPattern = /\]\(\/(?!\/)|href="\/(?!\/)|src="\/(?!\/)|file:\/\/\//g;
  for (const f of docFiles) {
    const fullPath = path.resolve(__dirname, '..', f);
    if (fs.existsSync(fullPath)) {
      const content = fs.readFileSync(fullPath, 'utf8');
      const matches = content.match(absPathPattern);
      assert.ok(!matches || matches.length === 0, `File ${f} contains absolute links: ${matches}`);
    }
  }

  console.log('  ✔ TC-139-13: Performance Budget & Relative Links PASSED');
}

// =============================================================
// TC-140 VERIFICATION SUITE: Dynamic Upstream Target & Route Logging
// =============================================================
console.log('\nRunning TC-140 Verification Suite (Dynamic Upstream & Route Fidelity)...\n');

// -------------------------------------------------------------
// TC-140-01: Dynamic Upstream Socket Rendering in Request Logs
// -------------------------------------------------------------
{
  const { exports, domElements } = createSandbox();
  const sampleLogs = [
    { id: 101, ts: Date.now() - 500, method: 'GET', path: '/kite/api/v1/trades', route: '/kite/api', short: 'sayantansaha.in /kite/api', status: 200, ms: 12.4, up: '127.0.0.1:8080', trace: 'tr123456', ip: '192.168.1.10', bytes: 1024 },
    { id: 102, ts: Date.now() - 300, method: 'GET', path: '/assets/style.css', route: '/assets', short: 'static /assets', status: 200, ms: 1.2, up: 'static', trace: 'tr789012', ip: '192.168.1.15', bytes: 4096 },
    { id: 103, ts: Date.now() - 100, method: 'GET', path: '/health', route: '/health', short: 'in-process', status: 200, ms: 0.5, up: 'in-process', trace: 'tr345678', ip: '127.0.0.1', bytes: 128 }
  ];

  exports.setRawApiData(fixtureStatus, fixtureRoutes, fixtureUpstreams, sampleLogs);
  const D = exports.buildDataModel();
  exports.lgUpdate(D, true);

  const lgBody = domElements.get('#lgBody');
  assert.ok(lgBody && lgBody.innerHTML, 'Request log body must be populated');
  assert.ok(lgBody.innerHTML.includes('127.0.0.1:8080'), `Expected '127.0.0.1:8080' in table, got: ${lgBody.innerHTML}`);
  assert.ok(lgBody.innerHTML.includes('static'), `Expected 'static' in table`);
  assert.ok(lgBody.innerHTML.includes('in-process'), `Expected 'in-process' in table`);
  assert.ok(!lgBody.innerHTML.includes('<code>gateway</code>'), `Upstream column must NOT display generic 'gateway'`);

  console.log('  ✔ TC-140-01: Dynamic Upstream Socket Display PASSED');
}

// -------------------------------------------------------------
// TC-140-02: Hierarchical Route Subpath Filtering in Request Logs
// -------------------------------------------------------------
{
  const { exports, domElements } = createSandbox();
  const sampleLogs = [
    { id: 201, ts: Date.now() - 500, method: 'GET', path: '/kite/api/v1/trades', route: '/kite/api', short: 'sayantansaha.in /kite/api', status: 200, ms: 12.4, up: '127.0.0.1:8080', trace: 'tr201', ip: '192.168.1.10' },
    { id: 202, ts: Date.now() - 400, method: 'GET', path: '/kite/api/v1/business', route: '/kite/api', short: 'sayantansaha.in /kite/api', status: 200, ms: 15.1, up: '127.0.0.1:8080', trace: 'tr202', ip: '192.168.1.11' },
    { id: 203, ts: Date.now() - 300, method: 'GET', path: '/kite/broker/stream', route: '/kite/broker', short: 'sayantansaha.in /kite/broker', status: 200, ms: 22.0, up: '127.0.0.1:8080', trace: 'tr203', ip: '192.168.1.12' }
  ];

  exports.setRawApiData(fixtureStatus, fixtureRoutes, fixtureUpstreams, sampleLogs);
  const D = exports.buildDataModel();
  const state = exports.getState();

  // Filter by route ID for /kite/api
  state.lroute = '_kite_api';
  exports.lgUpdate(D, true);

  const lgBody = domElements.get('#lgBody');
  assert.ok(lgBody.innerHTML.includes('/kite/api/v1/trades'), 'Subpath /kite/api/v1/trades must be matched');
  assert.ok(lgBody.innerHTML.includes('/kite/api/v1/business'), 'Subpath /kite/api/v1/business must be matched');
  assert.ok(!lgBody.innerHTML.includes('/kite/broker/stream'), 'Unrelated route /kite/broker/stream must be filtered out');

  // Reset filter to 'all'
  state.lroute = 'all';
  exports.lgUpdate(D, true);
  assert.ok(lgBody.innerHTML.includes('/kite/broker/stream'), 'All routes must appear when lroute is "all"');

  console.log('  ✔ TC-140-02: Hierarchical Route Subpath Filtering PASSED');
}

// -------------------------------------------------------------
// TC-140-03: Relative Links Invariant in REQ-140 Documents
// -------------------------------------------------------------
{
  const docFiles = [
    'docs/requirements/REQ-140.md',
    'docs/tasks/TASK-163.md',
    'docs/architecture/ADR-140.md',
    'docs/testCases/TC-140.md'
  ];

  const absPathPattern = /\]\(\/(?!\/)|href="\/(?!\/)|src="\/(?!\/)|file:\/\/\//g;
  for (const f of docFiles) {
    const fullPath = path.resolve(__dirname, '..', f);
    assert.ok(fs.existsSync(fullPath), `Document ${f} must exist`);
    const content = fs.readFileSync(fullPath, 'utf8');
    const matches = content.match(absPathPattern);
    assert.ok(!matches || matches.length === 0, `File ${f} contains absolute links: ${matches}`);
  }

  console.log('  ✔ TC-140-03: Strictly Relative Links Invariant PASSED');
}

console.log('\n========================================');
console.log('🎉 ALL TC-139 & TC-140 TEST CASES PASSED SUCCESSFULLY!');
console.log('========================================\n');
