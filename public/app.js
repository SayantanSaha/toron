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
const UPSTREAM_SERVICES = [
  { id: 1, port: 9001, name: "Dummy Container / Service 1", route: "/container-api" },
  { id: 2, port: 9002, name: "Dummy Container / Service 2", route: "/container-api" },
  { id: 3, port: 9003, name: "Dummy Container / Service 3", route: "/auto-discovered" },
  { id: 4, port: 9004, name: "Dummy Service 4", route: "/api (v1)" },
  { id: 5, port: 9005, name: "Dummy Service 5", route: "/services/cluster" },
  { id: 6, port: 9006, name: "Dummy Service 6", route: "/services/cluster" },
  { id: 7, port: 9007, name: "Dummy Service 7", route: "/services/cluster" },
  { id: 8, port: 9008, name: "Dummy Service 8", route: "/services/auth" },
  { id: 9, port: 9009, name: "Dummy Service 9", route: "/services/analytics" },
  { id: 10, port: 9010, name: "Dummy Service 10", route: "/services/analytics" }
];

function initHealthMatrix() {
  const container = document.getElementById('upstream-services-grid');
  if (!container) return;

  container.innerHTML = UPSTREAM_SERVICES.map(svc => `
    <div class="p-3 rounded-xl traefik-card space-y-2">
      <div class="flex items-center justify-between">
        <span class="text-xs font-mono font-bold text-white">:${svc.port}</span>
        <span id="badge-status-${svc.id}" class="text-[10px] px-1.5 py-0.5 rounded font-medium traefik-badge-emerald">
          HEALTHY
        </span>
      </div>
      <div class="text-[11px] text-slate-400 leading-tight">
        <div class="font-medium text-slate-300 truncate">${svc.name}</div>
        <div class="text-[10px] text-indigo-400 font-mono mt-0.5">${svc.route}</div>
      </div>
    </div>
  `).join('');

  const probeBtn = document.getElementById('btn-probe-all');
  if (probeBtn) {
    probeBtn.addEventListener('click', probeAllNodes);
  }
}

let TARGET_HEALTH_MAP = {};

async function probeAllNodes() {
  try {
    const res = await fetch('/internal/api/upstreams/health');
    if (!res.ok) return;
    const data = await res.json();
    if (data.upstreams && Array.isArray(data.upstreams)) {
      data.upstreams.forEach(svc => {
        const badge = document.getElementById(`badge-status-${svc.id}`);
        const isHealthy = svc.status === 'CLOSED' || svc.status === 'HEALTHY' || svc.status === 'OK';
        TARGET_HEALTH_MAP[svc.port] = isHealthy;

        if (badge) {
          if (isHealthy) {
            badge.textContent = 'HEALTHY';
            badge.className = 'text-[10px] px-1.5 py-0.5 rounded font-medium traefik-badge-emerald';
          } else {
            badge.textContent = 'UNREACHABLE';
            badge.className = 'text-[10px] px-1.5 py-0.5 rounded font-bold traefik-badge-rose';
          }
        }
      });
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

    const statWaf = document.getElementById('stat-waf-status');
    if (statWaf && data.security) {
      statWaf.textContent = (data.security.waf_mode || 'ENFORCE').toUpperCase();
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
      if (r.host === 'container.toron.local' || r.host === 'auto-discovered.local' || (r.host && r.host.includes('.local'))) {
        ociCount++;
      }
    });

    const statOci = document.getElementById('stat-oci-count');
    if (statOci) statOci.textContent = `${ociCount} Containers`;

    container.innerHTML = data.routes.map(r => {
      const isAutoDiscovered = r.host === 'container.toron.local' || r.host === 'auto-discovered.local' || (r.host && r.host.includes('.local'));
      const isTranscoder = r.prefix === '/v1/users/:id' || r.prefix.includes('/v1/users');

      let badgeHTML = `<span class="text-xs px-2.5 py-1 rounded-md traefik-badge-indigo font-semibold">${r.algorithm || 'Round-Robin'}</span>`;
      if (isAutoDiscovered) {
        badgeHTML = `<span class="text-xs px-2.5 py-1 rounded-md traefik-badge-cyan font-semibold flex items-center gap-1">🐋 OCI Auto-Discovered</span>`;
      } else if (isTranscoder) {
        badgeHTML = `<span class="text-xs px-2.5 py-1 rounded-md traefik-badge-sky font-semibold flex items-center gap-1">🔀 REST-to-gRPC</span>`;
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
      const val = urlSelect.value;
      if (val.includes('container-api')) {
        hostInput.value = 'container.toron.local';
      } else if (val.includes('auto-discovered')) {
        hostInput.value = 'auto-discovered.local';
      } else {
        hostInput.value = '';
      }
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
