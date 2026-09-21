/**
 * 👑 Toron Edge Gateway — Control Center & Dashboard v2.0 (Traefik Edition)
 * Inspired by Traefik Proxy Dashboard Architecture & Design System
 * Minimalist, Mobile-First, Light/Dark/System Theme Synchronization
 * Pure Vanilla JavaScript (ES6+) — Zero External Dependencies
 */

document.addEventListener('DOMContentLoaded', () => {
  initThemeEngine();
  initTabNavigation();
  initRouteSearch();
  initApiTester();
  initRefreshButton();

  // Initial data hydration
  pollStatus();
  fetchUpstreamHealth();

// Periodic real-time telemetry polling (every 4 seconds)
  setInterval(() => {
    pollStatus();
  }, 4000);
});

// HTML entity escaping utility to prevent Stored & Reflected DOM XSS (CWE-79)
function escapeHTML(str) {
  if (str === null || str === undefined) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// ============================================================================
// 1. THREE-STATE THEME ENGINE (Light, Dark, System Sync)
// ============================================================================
function initThemeEngine() {
  const btnLight = document.getElementById('theme-btn-light');
  const btnDark = document.getElementById('theme-btn-dark');
  const btnSystem = document.getElementById('theme-btn-system');

  function getSavedTheme() {
    return localStorage.getItem('toron-theme') || 'system';
  }

  function applyTheme(mode) {
    const isDark = mode === 'dark' || (mode === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
    
    if (isDark) {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }

    localStorage.setItem('toron-theme', mode);
    updateThemeButtons(mode);
  }

  function updateThemeButtons(currentMode) {
    [btnLight, btnDark, btnSystem].forEach(btn => {
      if (!btn) return;
      btn.classList.remove('bg-white', 'dark:bg-slate-800', 'text-cyan-600', 'dark:text-cyan-400', 'shadow-xs');
      btn.classList.add('text-slate-500', 'dark:text-slate-400');
    });

    let activeBtn = btnSystem;
    if (currentMode === 'light') activeBtn = btnLight;
    if (currentMode === 'dark') activeBtn = btnDark;

    if (activeBtn) {
      activeBtn.classList.add('bg-white', 'dark:bg-slate-800', 'text-cyan-600', 'dark:text-cyan-400', 'shadow-xs');
      activeBtn.classList.remove('text-slate-500', 'dark:text-slate-400');
    }
  }

  if (btnLight) btnLight.addEventListener('click', () => applyTheme('light'));
  if (btnDark) btnDark.addEventListener('click', () => applyTheme('dark'));
  if (btnSystem) btnSystem.addEventListener('click', () => applyTheme('system'));

  // Listen to OS system color scheme changes in real time
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
    if (getSavedTheme() === 'system') {
      if (e.matches) {
        document.documentElement.classList.add('dark');
      } else {
        document.documentElement.classList.remove('dark');
      }
    }
  });

  // Apply initial theme state
  applyTheme(getSavedTheme());
}

// ============================================================================
// 2. TAB NAVIGATION
// ============================================================================
function initTabNavigation() {
  const tabs = document.querySelectorAll('#desktop-tabs .navitem');
  const sections = document.querySelectorAll('.tab-content');

  tabs.forEach(tab => {
    tab.addEventListener('click', () => {
      const targetId = tab.getAttribute('data-tab');

      tabs.forEach(t => t.classList.remove('active'));
      tab.classList.add('active');

      sections.forEach(sec => {
        if (sec.id === targetId) {
          sec.classList.remove('hidden');
        } else {
          sec.classList.add('hidden');
        }
      });

      if (targetId === 'tab-services') {
        fetchUpstreamHealth();
      }
    });
  });
}

// ============================================================================
// 3. ROUTE SEARCH & FILTERING
// ============================================================================
function initRouteSearch() {
  const searchInput = document.getElementById('route-search-input');
  if (!searchInput) return;

  searchInput.addEventListener('input', (e) => {
    const query = e.target.value.toLowerCase().trim();
    const routeCards = document.querySelectorAll('.route-card');

    routeCards.forEach(card => {
      const cardText = card.textContent.toLowerCase();
      if (cardText.includes(query)) {
        card.classList.remove('hidden');
      } else {
        card.classList.add('hidden');
      }
    });
  });
}

// ============================================================================
// 4. TELEMETRY & ENGINE STATUS POLLING
// ============================================================================
async function pollStatus() {
  try {
    const res = await fetch('/internal/api/status');
    if (!res.ok) return;
    const data = await res.json();

    // Version badge
    const headerVer = document.getElementById('header-version');
    if (headerVer && data.version) {
      headerVer.textContent = `v${data.version}`;
    }

    // Workers count
    const statWorkers = document.getElementById('stat-workers-count');
    if (statWorkers && data.worker_pool_size) {
      statWorkers.textContent = `${data.worker_pool_size} Workers`;
    }

    // OCI Container discovery count
    const statOci = document.getElementById('stat-oci-count');
    const providerOci = document.getElementById('provider-oci-count');
    const ociCount = data.discovery ? (data.discovery.containers_count || 0) : 0;
    if (statOci) statOci.textContent = `${ociCount} Containers`;
    if (providerOci) providerOci.textContent = `${ociCount}`;

    // Security & WAF metrics
    if (data.security) {
      const totalRules = (data.security.waf_rules_count || 10) + (data.security.waf_custom_rules_count || 0);
      const statWafRules = document.getElementById('stat-waf-rules');
      if (statWafRules) statWafRules.textContent = `${totalRules} OWASP Rules`;

      const statSecRules = document.getElementById('stat-sec-rules');
      if (statSecRules) statSecRules.textContent = `${totalRules} Rules`;

      const statSecThreshold = document.getElementById('stat-sec-threshold');
      if (statSecThreshold) statSecThreshold.textContent = `${data.security.waf_anomaly_threshold || 5} Score`;

      const statMesh = document.getElementById('stat-mesh-status');
      if (statMesh) statMesh.textContent = data.security.mtls_enabled ? 'mTLS Active' : 'mTLS Ready';
    }

    fetchAndRenderRoutes();
    fetchSecurityIncidents();
    fetchObservability();
  } catch (e) {
    console.error('Failed to poll engine status:', e);
  }
}

// ============================================================================
// 5. TRAEFIK-STYLE ROUTE RENDERING
// ============================================================================
let cachedRoutes = [];

async function fetchAndRenderRoutes() {
  const container = document.getElementById('routes-container');
  if (!container) return;

  try {
    const res = await fetch('/internal/api/routes');
    if (!res.ok) return;
    const data = await res.json();

    if (!data.routes || !Array.isArray(data.routes)) return;
    cachedRoutes = data.routes;

    const routesCount = data.routes.length;
    const tabBadge = document.getElementById('tab-badge-routers');
    if (tabBadge) tabBadge.textContent = routesCount;

    const statRouters = document.getElementById('stat-routers-count');
    if (statRouters) statRouters.textContent = `${routesCount} Routes`;

    // Populate tester dropdown if needed
    populateTesterDropdown(data.routes);

    container.innerHTML = data.routes.map((r, idx) => {
      const isOci = r.source === 'oci' || Boolean(r.container_name) || (r.host && r.host.includes('.local'));
      const isStatic = r.type === 'static';
      const isTranscoder = r.prefix === '/v1/users/:id' || r.prefix.includes('/v1/users');

      // Provider badge
      let providerPill = `<span class="tr-badge tr-badge-emerald font-mono">📄 File</span>`;
      if (isOci) {
        providerPill = `<span class="tr-badge tr-badge-cyan font-mono">🐋 Docker (${escapeHTML(r.container_name || 'Container')})</span>`;
      } else if (r.source === 'k8s') {
        providerPill = `<span class="tr-badge tr-badge-indigo font-mono">☸️ Kubernetes</span>`;
      }

      // Build Traefik Rule Syntax: `Host(`example.com`) && PathPrefix(`/path`)`
      let ruleExpression = `PathPrefix(\`${escapeHTML(r.prefix)}\`)`;
      if (r.host) {
        ruleExpression = `Host(\`${escapeHTML(r.host)}\`) && ${ruleExpression}`;
      }
      if (r.headers && Object.keys(r.headers).length > 0) {
        const headerRules = Object.entries(r.headers).map(([k, v]) => `Header(\`${escapeHTML(k)}\`, \`${escapeHTML(v)}\`)`).join(' && ');
        ruleExpression = `${ruleExpression} && ${headerRules}`;
      }

      // Middlewares chain (WAF, StripPrefix, Transcoder, Load Balancer)
      const middlewarePills = [];
      middlewarePills.push(`<span class="text-[10px] px-2 py-0.5 rounded bg-rose-50 text-rose-700 dark:bg-rose-950/40 dark:text-rose-300 border border-rose-200 dark:border-rose-800/60 font-mono">🛡️ WAF Guard</span>`);

      if (isTranscoder) {
        middlewarePills.push(`<span class="text-[10px] px-2 py-0.5 rounded bg-sky-50 text-sky-700 dark:bg-sky-950/40 dark:text-sky-300 border border-sky-200 dark:border-sky-800/60 font-mono">🔀 REST-to-gRPC</span>`);
      }

      if (r.algorithm) {
        middlewarePills.push(`<span class="text-[10px] px-2 py-0.5 rounded bg-indigo-50 text-indigo-700 dark:bg-indigo-950/40 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800/60 font-mono">⚖️ ${escapeHTML(r.algorithm)}</span>`);
      }

      // Targets / Service representation
      let targetsHTML = '';
      if (r.targets && r.targets.length > 0) {
        targetsHTML = r.targets.map(t => `
          <div class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-slate-100 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 font-mono text-xs text-slate-800 dark:text-slate-200">
            <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
            <span>${escapeHTML(t)}</span>
          </div>
        `).join(' ');
      } else if (r.dir) {
        targetsHTML = `
          <div class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800/50 font-mono text-xs text-amber-800 dark:text-amber-300">
            <span>📁 ${escapeHTML(r.dir)}</span>
          </div>
        `;
      } else {
        targetsHTML = `<span class="text-xs text-slate-400 dark:text-slate-500 italic">Self-handled internal API</span>`;
      }

      const routerName = r.container_name ? `router-${escapeHTML(r.container_name)}@docker` : `router-${escapeHTML(r.prefix.replace(/[^a-zA-Z0-9]/g, '_'))}@file`;

      return `
        <div class="route-card p-4 sm:p-5 rounded-xl traefik-panel space-y-3.5">
          
          <!-- Top Row: Name, Protocol, Provider, Status -->
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5">
            <div class="flex items-center space-x-2.5">
              <span class="px-2 py-0.5 rounded text-[10px] font-bold font-mono ${isStatic ? 'bg-amber-100 text-amber-800 dark:bg-amber-900/60 dark:text-amber-300' : 'bg-cyan-100 text-cyan-800 dark:bg-cyan-900/60 dark:text-cyan-300'}">
                ${isStatic ? 'STATIC' : 'HTTP'}
              </span>
              <span class="font-bold text-xs sm:text-sm font-mono text-slate-800 dark:text-slate-200 truncate">${routerName}</span>
            </div>
            <div class="flex items-center space-x-2">
              ${providerPill}
              <span class="tr-badge tr-badge-emerald text-[10px]">SUCCESS</span>
            </div>
          </div>

          <!-- Traefik Rule Matcher Box -->
          <div class="p-2.5 sm:p-3 rounded-lg traefik-rule-box text-xs">
            <div class="text-[10px] font-bold uppercase tracking-wider text-slate-400 dark:text-slate-500 mb-1">Rule Matcher</div>
            <div class="text-cyan-700 dark:text-cyan-300 font-semibold break-all">${ruleExpression}</div>
          </div>

          <!-- Bottom: Service Target & Middlewares Chain -->
          <div class="pt-2 border-t border-slate-100 dark:border-slate-800/80 flex flex-col md:flex-row md:items-center justify-between gap-3">
            
            <!-- Service Targets -->
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-xs text-slate-400 font-medium flex items-center gap-1">➔ Target Service:</span>
              ${targetsHTML}
            </div>

            <!-- Middlewares -->
            <div class="flex flex-wrap items-center gap-1.5">
              <span class="text-xs text-slate-400 font-medium mr-1">Middlewares:</span>
              ${middlewarePills.join(' ')}
            </div>

          </div>

        </div>
      `;
    }).join('');
    const secondary = document.getElementById('routes-container-secondary');
    if (secondary) secondary.innerHTML = container.innerHTML;

  } catch (e) {
    console.error('Failed to fetch routes:', e);
  }
}

// ============================================================================
// 6. TRAEFIK-STYLE UPSTREAM HEALTH MATRIX
// ============================================================================
async function fetchUpstreamHealth() {
  const container = document.getElementById('upstream-services-grid');
  if (!container) return;

  try {
    const res = await fetch('/internal/api/upstreams/health');
    if (!res.ok) return;
    const data = await res.json();

    const upstreams = data.upstreams || [];
    const totalCount = data.total_nodes || upstreams.length;
    const healthyCount = data.healthy_nodes || 0;

    const tabBadge = document.getElementById('tab-badge-services');
    if (tabBadge) tabBadge.textContent = totalCount;

    const statUpstreams = document.getElementById('stat-upstreams-count');
    if (statUpstreams) statUpstreams.textContent = `${totalCount} Nodes`;

    if (upstreams.length === 0) {
      container.innerHTML = `
        <div class="col-span-full p-8 text-center rounded-xl traefik-panel text-slate-400 italic text-xs">
          No upstream targets configured. Add proxy targets in routes.yaml or run OCI containers to see live health matrix.
        </div>
      `;
      return;
    }

    container.innerHTML = upstreams.map(svc => {
      const isHealthy = svc.status === 'HEALTHY';
      const isOpen = svc.status === 'OPEN';
      const statusColor = isHealthy ? 'text-emerald-600 dark:text-emerald-400' : (isOpen ? 'text-amber-600 dark:text-amber-400' : 'text-rose-600 dark:text-rose-400');
      const badgeStyle = isHealthy ? 'tr-badge-emerald' : (isOpen ? 'tr-badge-amber' : 'tr-badge-rose');
      const pulseColor = isHealthy ? 'bg-emerald-500' : (isOpen ? 'bg-amber-500' : 'bg-rose-500');

      return `
        <div class="p-4 rounded-xl traefik-panel space-y-3">
          <div class="flex items-center justify-between">
            <div class="flex items-center space-x-2 min-w-0">
              <span class="w-2.5 h-2.5 rounded-full ${pulseColor} animate-pulse flex-shrink-0"></span>
              <span class="font-bold text-xs truncate text-slate-900 dark:text-white">${escapeHTML(svc.name || ('Port ' + svc.port))}</span>
            </div>
            <span class="tr-badge ${badgeStyle} text-[10px] font-mono">${escapeHTML(svc.status)}</span>
          </div>

          <div class="text-[11px] space-y-1 text-slate-500 dark:text-slate-400 font-mono">
            <div class="flex justify-between">
              <span>Port:</span>
              <span class="font-bold text-slate-700 dark:text-slate-300">:${escapeHTML(svc.port)}</span>
            </div>
            <div class="flex justify-between">
              <span>Route:</span>
              <span class="truncate max-w-[120px] text-slate-700 dark:text-slate-300">${escapeHTML(svc.route || '/')}</span>
            </div>
            <div class="flex justify-between">
              <span>Latency:</span>
              <span class="font-bold ${statusColor}">${svc.latency_ms > 0 ? (svc.latency_ms.toFixed(2) + ' ms') : '---'}</span>
            </div>
            <div class="flex justify-between">
              <span>HTTP Code:</span>
              <span class="font-bold text-slate-700 dark:text-slate-300">${svc.http_code > 0 ? escapeHTML(svc.http_code) : 'N/A'}</span>
            </div>
          </div>
        </div>
      `;
    }).join('');

  } catch (e) {
    console.error('Failed to fetch upstream health:', e);
  }
}

// ============================================================================
// 7. SECURITY INCIDENTS AUDIT STREAM
// ============================================================================
async function fetchSecurityIncidents() {
  const tbody = document.getElementById('incidents-table-body');
  if (!tbody) return;

  try {
    const res = await fetch('/internal/api/security/incidents');
    if (!res.ok) return;
    const data = await res.json();

    const blockedCountEl = document.getElementById('sec-blocked-count');
    if (blockedCountEl) {
      blockedCountEl.textContent = `${data.total_blocked || 0}`;
    }

    if (!data.incidents || data.incidents.length === 0) {
      tbody.innerHTML = `
        <tr>
          <td colspan="5" class="py-6 text-center text-slate-400 italic">No security violations detected. Server operating securely.</td>
        </tr>
      `;
      return;
    }

    tbody.innerHTML = data.incidents.map(inc => `
      <tr class="hover:bg-slate-50 dark:hover:bg-slate-800/40 transition">
        <td class="py-2.5 px-3 text-slate-500 dark:text-slate-400">${escapeHTML(new Date(inc.timestamp).toLocaleTimeString())}</td>
        <td class="py-2.5 px-3 text-slate-700 dark:text-slate-300">${escapeHTML(inc.client_ip)}</td>
        <td class="py-2.5 px-3 font-semibold text-slate-900 dark:text-white">${escapeHTML(inc.path)}</td>
        <td class="py-2.5 px-3 text-amber-600 dark:text-amber-400">${escapeHTML(inc.rule_id)}</td>
        <td class="py-2.5 px-3 text-right">
          <span class="tr-badge tr-badge-rose text-[10px]">BLOCKED</span>
        </td>
      </tr>
    `).join('');

  } catch (e) {
    // Audit stream is silent when idle
  }
}

// ============================================================================
// 8. INTERACTIVE API CONSOLE
// ============================================================================
function populateTesterDropdown(routes) {
  const select = document.getElementById('tester-url');
  if (!select || select.children.length > 2) return;

  const currentVal = select.value;
  select.innerHTML = '';

  const defaultEndpoints = [
    { prefix: '/health', host: '', name: 'GET /health (Server Health)' },
    { prefix: '/internal/api/status', host: '', name: 'GET /internal/api/status (Engine Metrics)' },
  ];

  const addedPrefixes = new Set();

  defaultEndpoints.forEach(ep => {
    addedPrefixes.add(ep.prefix);
    const opt = document.createElement('option');
    opt.value = ep.prefix;
    opt.setAttribute('data-host', ep.host);
    opt.textContent = ep.name;
    select.appendChild(opt);
  });

  routes.forEach(r => {
    if (!addedPrefixes.has(r.prefix) && r.prefix !== '/metrics') {
      addedPrefixes.add(r.prefix);
      const opt = document.createElement('option');
      opt.value = r.prefix;
      opt.setAttribute('data-host', r.host || '');
      const hostLabel = r.host ? ` [Host: ${r.host}]` : '';
      opt.textContent = `ANY ${r.prefix}${hostLabel}`;
      select.appendChild(opt);
    }
  });

  select.value = currentVal || '/health';

  select.addEventListener('change', () => {
    const selectedOption = select.options[select.selectedIndex];
    const host = selectedOption ? selectedOption.getAttribute('data-host') : '';
    const hostInput = document.getElementById('tester-host-header');
    if (hostInput) {
      hostInput.value = host || '';
    }
  });
}

function initApiTester() {
  const form = document.getElementById('tester-form');
  const resStatus = document.getElementById('res-status');
  const resBody = document.getElementById('res-body');
  const btnCopy = document.getElementById('btn-copy-response');

  if (btnCopy) {
    btnCopy.addEventListener('click', () => {
      if (resBody) {
        navigator.clipboard.writeText(resBody.textContent).then(() => {
          btnCopy.textContent = 'Copied!';
          setTimeout(() => { btnCopy.textContent = 'Copy JSON'; }, 2000);
        });
      }
    });
  }

  if (form) {
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      const method = document.getElementById('tester-method')?.value || 'GET';
      const path = document.getElementById('tester-url')?.value || '/health';
      const hostHeader = document.getElementById('tester-host-header')?.value || '';

      if (resStatus) resStatus.textContent = 'Executing...';
      if (resBody) resBody.textContent = 'Waiting for response from gateway...';

      const startTime = performance.now();

      try {
        const headers = {};
        if (hostHeader) headers['Host'] = hostHeader;

        const res = await fetch('/internal/api/proxy-test', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ path, method, headers })
        });

        const elapsed = (performance.now() - startTime).toFixed(1);

        if (!res.ok) {
          if (resStatus) resStatus.textContent = `HTTP ${res.status} (${elapsed}ms)`;
          if (resBody) resBody.textContent = await res.text();
          return;
        }

        const data = await res.json();
        if (resStatus) {
          resStatus.textContent = `HTTP ${data.status_code || 200} (${elapsed}ms)`;
          resStatus.className = (data.status_code >= 200 && data.status_code < 300) ? 'font-mono text-xs font-bold text-emerald-600 dark:text-emerald-400' : 'font-mono text-xs font-bold text-rose-600 dark:text-rose-400';
        }

        try {
          const parsed = JSON.parse(data.body);
          if (resBody) resBody.textContent = JSON.stringify(parsed, null, 2);
        } catch {
          if (resBody) resBody.textContent = data.body || '(Empty Response Body)';
        }

      } catch (err) {
        if (resStatus) resStatus.textContent = 'Connection Error';
        if (resBody) resBody.textContent = `Failed to connect: ${err.message}`;
      }
    });
  }
}

// ============================================================================
// 9. REFRESH & MANUAL PROBE TRIGGERS
// ============================================================================
function initRefreshButton() {
  const btnRefresh = document.getElementById('btn-refresh');
  if (btnRefresh) {
    btnRefresh.addEventListener('click', () => {
      pollStatus();
      fetchUpstreamHealth();
    });
  }

  const btnProbe = document.getElementById('btn-probe-all');
  if (btnProbe) {
    btnProbe.addEventListener('click', () => {
      fetchUpstreamHealth();
    });
  }
}


// ============================================================================
// 10. OBSERVABILITY DASHBOARD
// ============================================================================
const obsHistory = { requests: [], errors: [], latency: [], connections: [], waf: [], health: [] };
let obsLastRequests = null;
let obsLastAt = null;

function fmtNumber(n) {
  if (!Number.isFinite(n)) return '—';
  return Intl.NumberFormat(undefined, { notation: n >= 100000 ? 'compact' : 'standard', maximumFractionDigits: 1 }).format(n);
}
function setObsText(id, value) { const el=document.getElementById(id); if(el) el.textContent=value; }
function pushObs(key, value) { if(!Number.isFinite(value)) return; obsHistory[key].push(value); if(obsHistory[key].length>24) obsHistory[key].shift(); drawSpark('spark-'+key, obsHistory[key]); }
function drawSpark(id, values) {
  const svg=document.getElementById(id); if(!svg || !values.length) return;
  const w=220,h=52,p=3,min=Math.min(...values),max=Math.max(...values),span=max-min||1;
  const pts=values.map((v,i)=>`${p+(i/(Math.max(values.length-1,1)))*(w-p*2)},${h-p-((v-min)/span)*(h-p*2)}`).join(' ');
  svg.innerHTML=`<polyline points="${pts}" fill="none" stroke="currentColor" stroke-width="2.5" vector-effect="non-scaling-stroke" class="text-cyan-500"/>`;
}
function percentileFromHistogram(metrics, prefix, p=0.95) {
  const buckets=metrics.filter(x=>x.name===prefix+'_bucket').map(x=>({le:Number(x.labels.le),v:Number(x.value)})).filter(x=>Number.isFinite(x.le)).sort((a,b)=>a.le-b.le);
  const total=Number(metrics.find(x=>x.name===prefix+'_count')?.value||0); if(!total||!buckets.length) return NaN;
  const target=total*p; const hit=buckets.find(b=>b.v>=target); return hit?hit.le*1000:NaN;
}
function parsePrometheus(text) {
  const out=[]; for(const line of text.split(/\\r?\\n/)){ if(!line||line[0]==='#') continue; const m=line.match(/^([a-zA-Z_:][a-zA-Z0-9_:]*)(\\{([^}]*)\\})?\\s+([-+0-9.eE]+)$/); if(!m) continue; const labels={}; if(m[3]) for(const pair of m[3].matchAll(/([a-zA-Z_][a-zA-Z0-9_]*)="((?:\\\\.|[^"])*)"/g)) labels[pair[1]]=pair[2]; out.push({name:m[1],labels,value:Number(m[4])}); } return out;
}
function drawTraffic() {
  const req=obsHistory.requests, lat=obsHistory.latency; if(!req.length) return;
  const draw=(values)=>values.map((v,i)=>{const x=20+(i/(Math.max(values.length-1,1)))*860;const max=Math.max(...values)||1;const min=Math.min(...values);const y=205-((v-min)/(max-min||1))*175;return `${x.toFixed(1)},${y.toFixed(1)}`}).join(' ');
  const t=document.getElementById('traffic-line'), l=document.getElementById('latency-line'); if(t)t.setAttribute('points',draw(req)); if(l&&lat.length)l.setAttribute('points',draw(lat));
  const grid=document.getElementById('traffic-grid'); if(grid)grid.innerHTML=[35,80,125,170,205].map(y=>`<line x1="20" x2="880" y1="${y}" y2="${y}" stroke="currentColor" class="text-slate-200 dark:text-slate-800" stroke-width="1"/>`).join('');
}
function renderStatusBars(byStatus,total) {
  const el=document.getElementById('status-bars'); if(!el) return;
  const groups=[['2xx',Object.entries(byStatus).filter(([k])=>/^2/.test(k)).reduce((a,[,v])=>a+v,0),'bg-emerald-500'],['3xx',Object.entries(byStatus).filter(([k])=>/^3/.test(k)).reduce((a,[,v])=>a+v,0),'bg-sky-500'],['4xx',Object.entries(byStatus).filter(([k])=>/^4/.test(k)).reduce((a,[,v])=>a+v,0),'bg-amber-500'],['5xx',Object.entries(byStatus).filter(([k])=>/^5/.test(k)).reduce((a,[,v])=>a+v,0),'bg-rose-500']];
  el.innerHTML=groups.map(([name,val,cls])=>{const pct=total?val/total*100:0;return `<div><div class="flex justify-between text-xs mb-1"><span class="font-mono font-bold">${name}</span><span class="text-slate-500">${fmtNumber(val)} · ${pct.toFixed(1)}%</span></div><div class="h-2 rounded-full bg-slate-100 dark:bg-slate-900 overflow-hidden"><div class="h-full rounded-full ${cls}" style="width:${Math.min(pct,100)}%"></div></div></div>`}).join('');
}
function renderTopRoutes(byRoute) {
  const el=document.getElementById('top-routes'); if(!el)return; const rows=Object.entries(byRoute||{}).sort((a,b)=>b[1]-a[1]).slice(0,6); if(!rows.length){el.innerHTML='<div class="text-xs text-slate-500 py-6 text-center">No route telemetry yet.</div>';return;}
  const max=rows[0][1]||1; el.innerHTML=rows.map(([route,count])=>`<div class="space-y-1"><div class="flex justify-between gap-3 text-[11px]"><span class="font-mono truncate">${escapeHTML(route)}</span><strong class="font-mono">${fmtNumber(count)}</strong></div><div class="h-1.5 rounded bg-slate-100 dark:bg-slate-900"><div class="h-full rounded bg-cyan-500" style="width:${Math.max(3,count/max*100)}%"></div></div></div>`).join('');
}
async function fetchObservability() {
  try {
    const [metricsRes,statusRes,healthRes]=await Promise.all([fetch('/metrics'),fetch('/internal/api/status'),fetch('/internal/api/upstreams/health')]);
    if(!metricsRes.ok) return; const raw=await metricsRes.text(); const metrics=parsePrometheus(raw); const status= statusRes.ok ? await statusRes.json() : {};
    const total=metrics.filter(x=>x.name==='toron_http_requests_total').reduce((a,x)=>a+x.value,0);
    const errors=metrics.filter(x=>x.name==='toron_http_requests_total' && /^5/.test(x.labels.status||'')).reduce((a,x)=>a+x.value,0);
    const connections=Number(metrics.find(x=>x.name==='toron_tcp_connections_active')?.value||0);
    const quic=Number(metrics.find(x=>x.name==='toron_active_quic_streams')?.value||0);
    const waf=metrics.filter(x=>x.name==='toron_waf_blocked_requests_total').reduce((a,x)=>a+x.value,0);
    const anomalies=metrics.filter(x=>x.name==='toron_waf_anomalies_detected_total').reduce((a,x)=>a+x.value,0);
    const p95=percentileFromHistogram(metrics,'toron_http_request_duration_seconds',.95);
    const now=Date.now(); const rps=obsLastRequests===null||!obsLastAt?NaN:Math.max(0,(total-obsLastRequests)/((now-obsLastAt)/1000)); obsLastRequests=total;obsLastAt=now;
    setObsText('obs-total-requests',fmtNumber(total)); setObsText('obs-rps',Number.isFinite(rps)?fmtNumber(rps)+' req/s':'warming up…'); setObsText('obs-error-rate',total?((errors/total)*100).toFixed(2)+'%':'0.00%'); setObsText('obs-error-detail',fmtNumber(errors)+' 5xx / '+fmtNumber(total)); setObsText('obs-p95',Number.isFinite(p95)?p95.toFixed(2)+' ms':'—'); setObsText('obs-connections',fmtNumber(connections)); setObsText('obs-quic','QUIC '+fmtNumber(quic)); setObsText('obs-waf',fmtNumber(waf));setObsText('obs-waf-anomaly','anomalies '+fmtNumber(anomalies));
    setObsText('obs-workers',status.worker_pool_size||'—'); setObsText('obs-containers',status.discovery?.containers_count??'0'); setObsText('obs-mtls',status.security?.mtls_enabled?'Active':'Ready');
    pushObs('requests',Number.isFinite(rps)?rps:total); pushObs('errors',errors); pushObs('latency',Number.isFinite(p95)?p95:0); pushObs('connections',connections); pushObs('waf',waf);
    if(healthRes.ok){const h=await healthRes.json();const healthy=h.healthy_nodes||0,totalNodes=h.total_nodes||0;setObsText('obs-health',totalNodes?`${healthy}/${totalNodes}`:'0/0');setObsText('obs-health-detail',totalNodes?`${healthy} healthy · ${totalNodes-healthy} degraded`:'no targets');pushObs('health',totalNodes?healthy/totalNodes*100:100);}
    setObsText('obs-routers',cachedRoutes.length); setObsText('obs-services',document.getElementById('tab-badge-services')?.textContent||'0'); setObsText('last-updated','updated '+new Date().toLocaleTimeString());
    renderStatusBars(status.metrics?.requests_by_status||{},total); renderTopRoutes(status.metrics?.requests_by_route||{}); drawTraffic();
  } catch(e) { console.debug('Observability refresh failed',e); }
}
