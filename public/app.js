/**
 * Toron Web Server Dashboard — Traefik-Inspired Logic
 * Controls tab navigation, metrics polling, dynamic route rendering, and API tester.
 */

document.addEventListener('DOMContentLoaded', () => {
  initTabs();
  initHealthMatrix();
  initTester();
  pollStatus();
  setInterval(pollStatus, 4000);
});

// --- 1. TAB NAVIGATION ---
function initTabs() {
  const desktopButtons = document.querySelectorAll('#desktop-tabs .tab-btn');
  const tabContents = document.querySelectorAll('.tab-content');

  function switchTab(targetTabId) {
    tabContents.forEach(content => content.classList.add('hidden'));

    const activeContent = document.getElementById(targetTabId);
    if (activeContent) {
      activeContent.classList.remove('hidden');
    }

    desktopButtons.forEach(btn => {
      if (btn.getAttribute('data-tab') === targetTabId) {
        btn.className = 'tab-btn active px-4 py-2 text-xs font-semibold rounded-lg text-indigo-400 bg-indigo-500/10 border border-indigo-500/20 transition flex items-center space-x-2';
      } else {
        btn.className = 'tab-btn px-4 py-2 text-xs font-medium rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800/60 transition flex items-center space-x-2';
      }
    });
  }

  desktopButtons.forEach(btn => {
    btn.addEventListener('click', () => {
      switchTab(btn.getAttribute('data-tab'));
    });
  });

  const btnRefresh = document.getElementById('btn-refresh');
  if (btnRefresh) {
    btnRefresh.addEventListener('click', pollStatus);
  }
}

// --- 2. UPSTREAM HEALTH MATRIX ---
let TARGET_HEALTH_MAP = {};

function initHealthMatrix() {
  const probeBtn = document.getElementById('btn-probe-all');
  if (probeBtn) {
    probeBtn.addEventListener('click', probeAllNodes);
  }
}

async function probeAllNodes() {
  const container = document.getElementById('upstream-services-grid');
  const statUpstreams = document.getElementById('stat-upstreams-count');
  const tabBadgeServices = document.getElementById('tab-badge-services');

  try {
    const res = await fetch('/internal/api/upstreams/health');
    if (!res.ok) return;
    const data = await res.json();

    if (data.upstreams && Array.isArray(data.upstreams)) {
      if (statUpstreams) {
        statUpstreams.textContent = `${data.total_nodes || data.upstreams.length} Nodes`;
      }
      if (tabBadgeServices) {
        tabBadgeServices.textContent = data.total_nodes || data.upstreams.length;
      }

      data.upstreams.forEach(svc => {
        const isHealthy = svc.status === 'CLOSED' || svc.status === 'HEALTHY' || svc.status === 'OK';
        TARGET_HEALTH_MAP[svc.port] = isHealthy;
        TARGET_HEALTH_MAP[svc.name] = isHealthy;
      });

      if (container) {
        if (data.upstreams.length === 0) {
          container.innerHTML = `
            <div class="col-span-full p-6 text-center text-xs text-slate-500 italic traefik-card rounded-xl">
              No active upstream targets configured in routes.yaml.
            </div>
          `;
        } else {
          container.innerHTML = data.upstreams.map(svc => {
            const isHealthy = svc.status === 'CLOSED' || svc.status === 'HEALTHY' || svc.status === 'OK';
            const badgeClass = isHealthy ? 'traefik-badge-emerald' : 'traefik-badge-rose';
            const badgeText = isHealthy ? 'HEALTHY' : (svc.status || 'UNREACHABLE');
            const latencyStr = svc.latency_ms > 0 ? `${svc.latency_ms.toFixed(1)}ms` : 'offline';

            return `
              <div class="p-3 rounded-xl traefik-card space-y-2">
                <div class="flex items-center justify-between">
                  <span class="text-xs font-mono font-bold text-white">:${svc.port}</span>
                  <span class="text-[10px] px-1.5 py-0.5 rounded font-medium ${badgeClass}">
                    ${badgeText}
                  </span>
                </div>
                <div class="text-[11px] text-slate-400 leading-tight">
                  <div class="font-medium text-slate-300 truncate" title="${svc.name}">${svc.name}</div>
                  <div class="text-[10px] text-indigo-400 font-mono mt-0.5 truncate">${svc.route}</div>
                  <div class="text-[9px] text-slate-500 font-mono mt-0.5 flex justify-between">
                    <span>${svc.algo || 'Balancing'}</span>
                    <span class="text-cyan-400 font-mono">${latencyStr}</span>
                  </div>
                </div>
              </div>
            `;
          }).join('');
        }
      }
    }
  } catch (err) {
    console.error('Probe failed:', err);
  }
}

// --- 3. DYNAMIC STATUS & ROUTE POLLING ---
async function pollStatus() {
  try {
    await probeAllNodes();

    const res = await fetch('/internal/api/status');
    if (!res.ok) return;
    const data = await res.json();

    const headerVersion = document.getElementById('header-version');
    if (headerVersion && data.version) {
      headerVersion.textContent = `v${data.version}`;
    }

    const statWaf = document.getElementById('stat-waf-status');
    if (statWaf && data.security) {
      statWaf.textContent = (data.security.waf_mode || 'ENFORCE').toUpperCase();
    }

    const statWafRules = document.getElementById('stat-waf-rules');
    if (statWafRules && data.security) {
      const totalRules = (data.security.waf_rules_count || 10) + (data.security.waf_custom_rules_count || 0);
      statWafRules.textContent = `${totalRules} Rules`;
    }

    const statWafThreshold = document.getElementById('stat-waf-threshold');
    if (statWafThreshold && data.security) {
      statWafThreshold.textContent = `${data.security.waf_anomaly_threshold || 5} Score`;
    }

    const statMesh = document.getElementById('stat-mesh-status');
    if (statMesh && data.security) {
      statMesh.textContent = data.security.mtls_enabled ? 'mTLS Active' : 'mTLS Ready';
    }

    const statOci = document.getElementById('stat-oci-count');
    if (statOci && data.discovery) {
      statOci.textContent = `${data.discovery.containers_count || 0} Containers`;
    }

    fetchAndRenderRoutes();
    fetchSecurityIncidents();
  } catch (e) {
    console.error('Failed to poll engine status:', e);
  }
}

async function fetchAndRenderRoutes() {
  const container = document.getElementById('routes-container');
  if (!container) return;

  try {
    const res = await fetch('/internal/api/routes');
    if (!res.ok) return;
    const data = await res.json();

    if (!data.routes || !Array.isArray(data.routes)) return;

    const routesCount = data.routes.length;
    const tabBadge = document.getElementById('tab-badge-routers');
    if (tabBadge) tabBadge.textContent = routesCount;

    const statRouters = document.getElementById('stat-routers-count');
    if (statRouters) statRouters.textContent = `${routesCount} Routes`;

    let ociCount = 0;
    data.routes.forEach(r => {
      if (r.source === 'oci' || r.container_name || r.host === 'container.toron.local' || r.host === 'auto-discovered.local' || (r.host && r.host.includes('.local'))) {
        ociCount++;
      }
    });

    const statOci = document.getElementById('stat-oci-count');
    if (statOci) statOci.textContent = `${ociCount} Containers`;

    container.innerHTML = data.routes.map(r => {
      const isAutoDiscovered = r.source === 'oci' || Boolean(r.container_name) || r.host === 'container.toron.local' || r.host === 'auto-discovered.local' || (r.host && r.host.includes('.local'));
      const isTranscoder = r.prefix === '/v1/users/:id' || r.prefix.includes('/v1/users');
      const containerLabel = r.container_name ? `🐋 OCI: ${r.container_name}` : `🐋 OCI Auto-Discovered`;

      let badgeHTML = `<span class="text-xs px-2.5 py-1 rounded-md traefik-badge-indigo font-semibold">${r.algorithm || 'Round-Robin'}</span>`;
      if (isAutoDiscovered) {
        badgeHTML = `<span class="text-xs px-2.5 py-1 rounded-md traefik-badge-cyan font-semibold flex items-center gap-1">${containerLabel}</span>`;
      } else if (isTranscoder) {
        badgeHTML = `<span class="text-xs px-2.5 py-1 rounded-md traefik-badge-sky font-semibold flex items-center gap-1">🔀 REST-to-gRPC</span>`;
      } else if (r.algorithm === 'weighted_round_robin') {
        badgeHTML = `<span class="text-xs px-2.5 py-1 rounded-md traefik-badge-cyan font-semibold flex items-center gap-1">⚖️ Weighted Round-Robin</span>`;
      } else if (r.algorithm === 'weighted_random') {
        badgeHTML = `<span class="text-xs px-2.5 py-1 rounded-md traefik-badge-violet font-semibold flex items-center gap-1">🎲 Weighted Random</span>`;
      } else if (r.algorithm === 'least_conn') {
        badgeHTML = `<span class="text-xs px-2.5 py-1 rounded-md traefik-badge-emerald font-semibold flex items-center gap-1">⚡ Least Connections</span>`;
      } else if (r.algorithm === 'weighted_least_conn') {
        badgeHTML = `<span class="text-xs px-2.5 py-1 rounded-md traefik-badge-emerald font-semibold flex items-center gap-1">⚡ Weighted Least Conn</span>`;
      } else if (r.algorithm === 'least_latency') {
        badgeHTML = `<span class="text-xs px-2.5 py-1 rounded-md traefik-badge-amber font-semibold flex items-center gap-1">⏱️ Lowest Latency</span>`;
      }

      const hostHTML = r.host ? `<span class="text-xs px-2 py-0.5 rounded-full bg-slate-800 text-slate-300 border border-slate-700 font-mono">Host: <code class="text-indigo-400">${r.host}</code></span>` : '';
      const headerHTML = r.headers ? Object.entries(r.headers).map(([k, v]) => `<span class="text-xs px-2 py-0.5 rounded-full bg-slate-800 text-slate-300 border border-slate-700 font-mono">Header: <code class="text-indigo-400">${k}: ${v}</code></span>`).join(' ') : '';

      const targetsHTML = (r.targets && r.targets.length > 0)
        ? r.targets.map(t => {
            let portStr = '';
            const match = t.match(/:(\d+)/);
            if (match) portStr = match[1];

            const isHealthy = portStr ? (TARGET_HEALTH_MAP[portStr] !== false) : true;
            const targetBadge = isHealthy
              ? `<span class="inline-flex items-center gap-1 text-[11px] px-2 py-0.5 rounded traefik-badge-emerald">
                  <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
                  Healthy
                 </span>`
              : `<span class="inline-flex items-center gap-1 text-[11px] px-2 py-0.5 rounded traefik-badge-rose font-bold">
                  <span class="w-1.5 h-1.5 rounded-full bg-rose-500"></span>
                  Unreachable
                 </span>`;

            return `
              <div class="p-3 rounded-lg bg-slate-950/60 border border-slate-800 flex items-center justify-between">
                <div>
                  <div class="text-xs font-mono text-white">${t}</div>
                  <div class="text-[11px] text-slate-400">${isAutoDiscovered ? 'OCI Microservice Container' : 'Upstream Target'}</div>
                </div>
                ${targetBadge}
              </div>
            `;
          }).join('')
        : `<div class="p-3 rounded-lg bg-slate-950/60 border border-slate-800 text-xs text-sky-400 font-mono">Dynamic Upstream Engine Stream</div>`;

      return `
        <div class="p-5 rounded-xl traefik-card space-y-4">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-3 border-b border-slate-800/60">
            <div class="flex items-center space-x-2 flex-wrap gap-y-1">
              <span class="px-2 py-1 text-xs font-bold rounded bg-indigo-500/20 text-indigo-300 font-mono">${r.type === 'static' ? 'STATIC' : 'ANY'}</span>
              <span class="text-base font-bold text-white font-mono">${r.prefix}</span>
              ${hostHTML}
              ${headerHTML}
            </div>
            <div class="flex items-center space-x-2">
              ${badgeHTML}
            </div>
          </div>
          <div class="space-y-2">
            <h4 class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Upstream Targets & Clusters</h4>
            <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
              ${targetsHTML}
            </div>
          </div>
        </div>
      `;
    }).join('');
    // Dynamic Tester URL Dropdown Population
    const urlSelect = document.getElementById('tester-url');
    if (urlSelect) {
      const currentSelected = urlSelect.value;
      let optionsHTML = `
        <option value="/health" data-host="">GET /health (Server Health)</option>
        <option value="/internal/api/status" data-host="">GET /internal/api/status (Engine Metrics)</option>
        <option value="/internal/api/routes" data-host="">GET /internal/api/routes (Route Table)</option>
        <option value="/internal/api/upstreams/health" data-host="">GET /internal/api/upstreams/health (Upstream Probes)</option>
      `;

      data.routes.forEach(r => {
        if (r.prefix && r.prefix !== '/' && !r.prefix.startsWith('/internal')) {
          const hostAttr = r.host || '';
          const label = `GET ${r.prefix} ${r.host ? '(Host: ' + r.host + ')' : ''}`;
          optionsHTML += `<option value="${r.prefix}" data-host="${hostAttr}">${label}</option>`;
        }
      });

      urlSelect.innerHTML = optionsHTML;
      if (currentSelected) {
        urlSelect.value = currentSelected;
      }
    }
  } catch (e) {
    console.error('Failed to render dynamic routes:', e);
  }
}

async function fetchSecurityIncidents() {
  const tableBody = document.getElementById('incidents-table-body');
  if (!tableBody) return;

  try {
    const res = await fetch('/internal/api/security/incidents');
    if (!res.ok) return;
    const data = await res.json();

    if (data.incidents && Array.isArray(data.incidents) && data.incidents.length > 0) {
      tableBody.innerHTML = data.incidents.map(inc => `
        <tr class="hover:bg-slate-800/40">
          <td class="py-2.5 px-3 text-slate-400">${new Date(inc.timestamp).toLocaleTimeString()}</td>
          <td class="py-2.5 px-3 text-indigo-400 font-mono">${inc.client_ip}</td>
          <td class="py-2.5 px-3 text-white font-mono">${inc.path}</td>
          <td class="py-2.5 px-3 text-amber-400 font-mono">${inc.rule_id}</td>
          <td class="py-2.5 px-3 text-right"><span class="traefik-badge-rose px-2 py-0.5 rounded">${inc.action}</span></td>
        </tr>
      `).join('');
    }
  } catch (e) {
    console.error('Failed to fetch security incidents:', e);
  }
}

// --- 4. INTERACTIVE API TESTER ---
function initTester() {
  const form = document.getElementById('tester-form');
  const urlSelect = document.getElementById('tester-url');
  const hostInput = document.getElementById('tester-host-header');
  const resStatus = document.getElementById('res-status');
  const resBody = document.getElementById('res-body');

  if (urlSelect && hostInput) {
    urlSelect.addEventListener('change', () => {
      const selectedOption = urlSelect.options[urlSelect.selectedIndex];
      const hostVal = selectedOption ? selectedOption.getAttribute('data-host') : '';
      hostInput.value = hostVal || '';
    });
  }

  if (form) {
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      
      const targetUrl = urlSelect.value;
      const hostVal = hostInput.value.trim();

      resStatus.textContent = 'EXECUTING...';
      resStatus.className = 'font-mono text-indigo-400 font-bold animate-pulse';
      resBody.textContent = 'Sending request to Toron gateway...';

      const headers = {};
      if (hostVal) {
        headers['Host'] = hostVal;
      }

      const startTime = performance.now();

      try {
        const response = await fetch(targetUrl, { headers });
        const latency = (performance.now() - startTime).toFixed(1);
        
        resStatus.textContent = `${response.status} ${response.statusText} (${latency}ms)`;
        resStatus.className = response.ok ? 'font-mono text-emerald-400 font-bold' : 'font-mono text-rose-400 font-bold';

        const contentType = response.headers.get('content-type') || '';
        let bodyText = '';
        
        if (contentType.includes('json')) {
          const jsonObj = await response.json();
          bodyText = JSON.stringify(jsonObj, null, 2);
        } else {
          bodyText = await response.text();
        }

        resBody.textContent = bodyText;
      } catch (err) {
        resStatus.textContent = 'FAILED';
        resStatus.className = 'font-mono text-rose-400 font-bold';
        resBody.textContent = `Error executing request: ${err.message}`;
      }
    });
  }
}
