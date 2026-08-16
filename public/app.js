/**
 * Toron Web Server Dashboard — Mobile-First Logic
 * Controls tab navigation, metrics polling, upstream health matrix, and live API tester.
 */

document.addEventListener('DOMContentLoaded', () => {
  initTabs();
  initHealthMatrix();
  initSecurityTab();
  initTester();
  initPolling();
});

// --- 1. TAB NAVIGATION ---
function initTabs() {
  const desktopButtons = document.querySelectorAll('#desktop-tabs .tab-btn');
  const mobileButtons = document.querySelectorAll('#mobile-tabs .tab-btn-mobile');
  const tabContents = document.querySelectorAll('.tab-content');

  function switchTab(targetTabId) {
    // Hide all contents
    tabContents.forEach(content => {
      content.classList.add('hidden');
    });

    // Show target content
    const activeContent = document.getElementById(targetTabId);
    if (activeContent) {
      activeContent.classList.remove('hidden');
    }

    // Desktop buttons styling
    desktopButtons.forEach(btn => {
      if (btn.getAttribute('data-tab') === targetTabId) {
        btn.className = 'tab-btn active px-4 py-2 text-xs font-semibold rounded-lg text-indigo-400 bg-indigo-500/10 border border-indigo-500/20 transition';
      } else {
        btn.className = 'tab-btn px-4 py-2 text-xs font-medium rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800/60 transition';
      }
    });

    // Mobile buttons styling
    mobileButtons.forEach(btn => {
      if (btn.getAttribute('data-tab') === targetTabId) {
        btn.className = 'tab-btn-mobile active flex flex-col items-center justify-center p-1.5 rounded-lg text-indigo-400 bg-indigo-500/10';
      } else {
        btn.className = 'tab-btn-mobile flex flex-col items-center justify-center p-1.5 rounded-lg text-slate-400 hover:text-slate-200';
      }
    });
  }

  desktopButtons.forEach(btn => {
    btn.addEventListener('click', () => {
      const tabId = btn.getAttribute('data-tab');
      switchTab(tabId);
    });
  });

  mobileButtons.forEach(btn => {
    btn.addEventListener('click', () => {
      const tabId = btn.getAttribute('data-tab');
      switchTab(tabId);
    });
  });
}

// --- 2. UPSTREAM HEALTH MATRIX ---
const UPSTREAM_SERVICES = [
  { id: 1, port: 9001, name: "Dummy Service 1", route: "/api (v2)", algo: "Round-Robin" },
  { id: 2, port: 9002, name: "Dummy Service 2", route: "/api (v2)", algo: "Round-Robin" },
  { id: 3, port: 9003, name: "Dummy Service 3", route: "/api (v2)", algo: "Round-Robin" },
  { id: 4, port: 9004, name: "Dummy Service 4", route: "/api (v1)", algo: "Single Target" },
  { id: 5, port: 9005, name: "Dummy Service 5", route: "/services/cluster", algo: "Round-Robin" },
  { id: 6, port: 9006, name: "Dummy Service 6", route: "/services/cluster", algo: "Round-Robin" },
  { id: 7, port: 9007, name: "Dummy Service 7", route: "/services/cluster", algo: "Round-Robin" },
  { id: 8, port: 9008, name: "Dummy Service 8", route: "/services/auth", algo: "Single Target" },
  { id: 9, port: 9009, name: "Dummy Service 9", route: "/services/analytics", algo: "Round-Robin" },
  { id: 10, port: 9010, name: "Dummy Service 10", route: "/services/analytics", algo: "Round-Robin" }
];

function initHealthMatrix() {
  const container = document.getElementById('health-matrix-grid');
  if (!container) return;

  renderHealthGrid(container);
  probeAllNodes();

  const btnProbeAll = document.getElementById('btn-probe-all');
  if (btnProbeAll) {
    btnProbeAll.addEventListener('click', () => {
      probeAllNodes();
    });
  }
}

function renderHealthGrid(container) {
  container.innerHTML = UPSTREAM_SERVICES.map(svc => `
    <div id="health-card-${svc.id}" class="p-3 rounded-lg bg-slate-950/70 border border-slate-800 space-y-2 hover:border-slate-700 transition">
      <div class="flex items-center justify-between">
        <span class="text-xs font-bold text-white font-mono">Port ${svc.port}</span>
        <span id="badge-status-${svc.id}" class="text-[10px] px-1.5 py-0.5 rounded font-medium bg-indigo-500/10 text-indigo-400 border border-indigo-500/20">
          INITIALIZING
        </span>
      </div>
      <div class="text-[11px] text-slate-400 leading-tight">
        <div class="font-medium text-slate-300 truncate">${svc.name}</div>
        <div class="text-[10px] text-indigo-400 font-mono mt-0.5">${svc.route}</div>
      </div>
      <div class="pt-1.5 border-t border-slate-900 flex justify-between items-center text-[10px] font-mono text-slate-400">
        <span>Latency:</span>
        <span id="latency-${svc.id}" class="text-slate-200">-- ms</span>
      </div>
    </div>
  `).join('');
}

async function probeAllNodes() {
  UPSTREAM_SERVICES.forEach(svc => {
    const badge = document.getElementById(`badge-status-${svc.id}`);
    if (badge) {
      badge.textContent = 'PROBING...';
      badge.className = 'text-[10px] px-1.5 py-0.5 rounded font-medium bg-indigo-500/10 text-indigo-400 border border-indigo-500/20 animate-pulse';
    }
  });

  try {
    const res = await fetch('/internal/api/upstreams/health');
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();

    if (data.upstreams && Array.isArray(data.upstreams)) {
      data.upstreams.forEach(svc => {
        const badge = document.getElementById(`badge-status-${svc.id}`);
        const latencySpan = document.getElementById(`latency-${svc.id}`);
        if (!badge || !latencySpan) return;

        latencySpan.textContent = `${svc.latency_ms.toFixed(1)}ms`;

        if (svc.status === 'CLOSED') {
          badge.textContent = 'CLOSED';
          badge.className = 'text-[10px] px-1.5 py-0.5 rounded font-medium bg-emerald-500/10 text-emerald-400 border border-emerald-500/20';
        } else if (svc.status.includes('OPEN')) {
          badge.textContent = svc.status;
          badge.className = 'text-[10px] px-1.5 py-0.5 rounded font-bold bg-rose-500/20 text-rose-400 border border-rose-500/40 animate-pulse';
        } else {
          badge.textContent = svc.status;
          badge.className = 'text-[10px] px-1.5 py-0.5 rounded font-medium bg-rose-500/20 text-rose-400 border border-rose-500/30';
        }
      });
    }
  } catch (err) {
    UPSTREAM_SERVICES.forEach(svc => {
      const badge = document.getElementById(`badge-status-${svc.id}`);
      const latencySpan = document.getElementById(`latency-${svc.id}`);
      if (badge) {
        badge.textContent = 'UNREACHABLE';
        badge.className = 'text-[10px] px-1.5 py-0.5 rounded font-medium bg-rose-500/20 text-rose-400 border border-rose-500/30';
      }
      if (latencySpan) latencySpan.textContent = '-- ms';
    });
  }
}

// --- 3. LIVE API TESTER ---
function initTester() {
  const form = document.getElementById('tester-form');
  const urlSelect = document.getElementById('tester-url');
  const presetSelect = document.getElementById('tester-header-preset');
  const customHeaderInput = document.getElementById('tester-custom-header');

  const resStatus = document.getElementById('res-status');
  const resLatency = document.getElementById('res-latency');
  const resHeaders = document.getElementById('res-headers');
  const resBody = document.getElementById('res-body');

  if (presetSelect && customHeaderInput) {
    presetSelect.addEventListener('change', () => {
      const val = presetSelect.value;
      if (val === 'none') {
        customHeaderInput.value = '';
      } else {
        customHeaderInput.value = val;
      }
    });
  }

  if (urlSelect && presetSelect && customHeaderInput) {
    urlSelect.addEventListener('change', () => {
      const selected = urlSelect.value;
      if (selected === '/api') {
        presetSelect.value = 'X-Version: v2';
        customHeaderInput.value = 'X-Version: v2';
      }
    });
  }

  if (form) {
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      
      const targetUrl = urlSelect.value;
      const method = document.getElementById('tester-method').value;
      const headerStr = customHeaderInput.value.trim();

      const headers = {};
      if (headerStr && headerStr.includes(':')) {
        const parts = headerStr.split(':');
        headers[parts[0].trim()] = parts.slice(1).join(':').trim();
      }

      resStatus.textContent = 'EXECUTING (INTERNAL)...';
      resStatus.className = 'text-xs px-2.5 py-0.5 rounded font-mono font-bold bg-amber-500/20 text-amber-300 animate-pulse';
      resHeaders.textContent = 'Waiting for internal response headers...';
      resBody.textContent = 'Routing through Toron proxy engine...';

      try {
        const response = await fetch('/internal/api/proxy-test', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            path: targetUrl,
            method: method,
            headers: headers
          })
        });

        if (!response.ok) {
          throw new Error(`Internal API returned HTTP ${response.status}`);
        }

        const data = await response.json();

        resLatency.textContent = `${data.latency_ms.toFixed(1)} ms`;

        // Format Headers
        let headerText = `HTTP/1.1 ${data.status_code} ${data.status_text}\n`;
        if (data.headers) {
          Object.entries(data.headers).forEach(([key, val]) => {
            headerText += `${key}: ${val}\n`;
          });
        }
        resHeaders.textContent = headerText;

        // Status Badge
        if (data.status_code >= 200 && data.status_code < 400) {
          resStatus.textContent = `${data.status_code} ${data.status_text || 'OK'}`;
          resStatus.className = 'text-xs px-2.5 py-0.5 rounded font-mono font-bold bg-emerald-500/20 text-emerald-300 border border-emerald-500/30';
        } else {
          resStatus.textContent = `${data.status_code} ${data.status_text || 'Error'}`;
          resStatus.className = 'text-xs px-2.5 py-0.5 rounded font-mono font-bold bg-rose-500/20 text-rose-300 border border-rose-500/30';
        }

        // Body Parsing
        const bodyText = data.body || '';
        try {
          const parsed = JSON.parse(bodyText);
          resBody.textContent = JSON.stringify(parsed, null, 2);
        } catch (e) {
          resBody.textContent = bodyText.length > 500 ? bodyText.substring(0, 500) + '\n... (truncated)' : bodyText;
        }

      } catch (err) {
        resLatency.textContent = '0 ms';
        resStatus.textContent = 'FETCH FAILED';
        resStatus.className = 'text-xs px-2.5 py-0.5 rounded font-mono font-bold bg-rose-500/20 text-rose-300 border border-rose-500/30';
        resHeaders.textContent = 'Error: Connection to /internal/api/proxy-test failed.';
        resBody.textContent = `Error details: ${err.message}`;
      }
    });
  }
}

// --- 4. SECURITY & WAF TAB ENGINE ---
function initSecurityTab() {
  const btnRefreshIncidents = document.getElementById('btn-refresh-incidents');
  if (btnRefreshIncidents) {
    btnRefreshIncidents.addEventListener('click', () => {
      fetchSecurityIncidents();
    });
  }
  fetchSecurityIncidents();
}

function escapeHTML(str) {
  if (typeof str !== 'string') return str == null ? '' : String(str);
  return str.replace(/[&<>"']/g, match => {
    switch (match) {
      case '&': return '&amp;';
      case '<': return '&lt;';
      case '>': return '&gt;';
      case '"': return '&quot;';
      case "'": return '&#39;';
      default: return match;
    }
  });
}

async function fetchSecurityIncidents() {
  const tbody = document.getElementById('security-incidents-tbody');
  const badge = document.getElementById('incident-count-badge');
  if (!tbody) return;

  try {
    const res = await fetch('/internal/api/security/incidents');
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();

    const incidents = data.incidents || [];
    if (badge) badge.textContent = `${incidents.length} Event${incidents.length === 1 ? '' : 's'}`;

    if (incidents.length === 0) {
      tbody.innerHTML = `
        <tr>
          <td colspan="7" class="py-8 text-center text-slate-500 font-sans text-xs">
            No security incidents recorded. System active with clean traffic flow.
          </td>
        </tr>
      `;
      return;
    }

    tbody.innerHTML = incidents.map(item => {
      let catBadgeClass = 'bg-slate-800 text-slate-300 border-slate-700';
      const cat = (item.category || 'unknown').toLowerCase();
      if (cat.includes('sqli')) {
        catBadgeClass = 'bg-rose-500/20 text-rose-300 border-rose-500/30';
      } else if (cat.includes('xss')) {
        catBadgeClass = 'bg-amber-500/20 text-amber-300 border-amber-500/30';
      } else if (cat.includes('traversal')) {
        catBadgeClass = 'bg-purple-500/20 text-purple-300 border-purple-500/30';
      } else if (cat.includes('rce')) {
        catBadgeClass = 'bg-red-600/30 text-red-300 border-red-500/40 animate-pulse';
      } else if (cat.includes('ip_acl') || cat.includes('ip')) {
        catBadgeClass = 'bg-sky-500/20 text-sky-300 border-sky-500/30';
      } else if (cat.includes('protocol')) {
        catBadgeClass = 'bg-orange-500/20 text-orange-300 border-orange-500/30';
      }

      let actionClass = 'text-rose-400 font-bold bg-rose-500/20 border-rose-500/30';
      let actionLabel = 'BLOCKED (403)';
      if (item.action === 'logged' || item.action === 'detection') {
        actionClass = 'text-amber-300 font-semibold bg-amber-500/20 border-amber-500/30';
        actionLabel = 'LOGGED (ANOMALY)';
      } else if (item.action === 'ip_denied') {
        actionClass = 'text-rose-400 font-bold bg-rose-600/30 border-rose-600/40';
        actionLabel = 'IP DENIED (403)';
      }

      const formattedTime = item.timestamp ? item.timestamp.replace('T', ' ').replace('Z', '') : '--';

      return `
        <tr class="hover:bg-slate-800/30 transition">
          <td class="py-2.5 px-3 text-slate-400 text-[11px] font-mono whitespace-nowrap">${escapeHTML(formattedTime)}</td>
          <td class="py-2.5 px-3 text-slate-200 text-[11px] font-mono font-semibold">${escapeHTML(item.client_ip || '127.0.0.1')}</td>
          <td class="py-2.5 px-3">
            <span class="text-[10px] px-2 py-0.5 rounded font-mono font-bold border ${catBadgeClass}">
              ${escapeHTML((item.category || 'THREAT').toUpperCase())}
            </span>
          </td>
          <td class="py-2.5 px-3 text-indigo-400 font-mono text-[11px]">${escapeHTML(item.rule_id || 'RULE-000')}</td>
          <td class="py-2.5 px-3 text-slate-300 font-mono text-[11px]">
            <span class="text-slate-400 font-semibold">${escapeHTML(item.method || 'GET')}</span>
            <span class="text-slate-200 truncate max-w-xs inline-block align-bottom">${escapeHTML(item.path || '/')}</span>
          </td>
          <td class="py-2.5 px-3 text-center text-amber-400 font-mono font-bold">${escapeHTML(item.anomaly_score || 5)}</td>
          <td class="py-2.5 px-3 text-right">
            <span class="text-[10px] px-2 py-0.5 rounded font-mono border ${actionClass}">
              ${escapeHTML(actionLabel)}
            </span>
          </td>
        </tr>
      `;
    }).join('');

  } catch (err) {
    if (badge) badge.textContent = 'Error';
  }
}

// --- 5. AUTO POLLING ENGINE ---
function initPolling() {
  const btnRefresh = document.getElementById('btn-refresh');
  const refreshIcon = document.getElementById('refresh-icon');

  async function pollStatus() {
    if (refreshIcon) {
      refreshIcon.classList.add('rotate-180');
      setTimeout(() => refreshIcon.classList.remove('rotate-180'), 500);
    }

    try {
      const res = await fetch('/internal/api/status');
      if (res.ok) {
        const data = await res.json();
        const headerUptime = document.getElementById('header-uptime');
        if (headerUptime) headerUptime.textContent = `100% (${data.uptime})`;

        // Update Security Metrics Cards
        if (data.security) {
          const secWafMode = document.getElementById('sec-waf-mode');
          if (secWafMode) {
            const modeStr = (data.security.waf_mode || 'ENFORCE').toUpperCase();
            secWafMode.textContent = modeStr;
            secWafMode.className = modeStr === 'ENFORCE'
              ? 'text-lg sm:text-xl font-bold text-rose-400 font-mono'
              : 'text-lg sm:text-xl font-bold text-amber-400 font-mono';
          }

          const secWafSub = document.getElementById('sec-waf-sub');
          if (secWafSub) {
            secWafSub.textContent = data.security.waf_enabled ? 'Active Protection Engine' : 'Engine Disabled';
          }

          const secRulesCount = document.getElementById('sec-rules-count');
          if (secRulesCount) {
            secRulesCount.textContent = `${data.security.waf_rules_count || 10} Active`;
          }

          const secCustomRulesSub = document.getElementById('sec-custom-rules-sub');
          if (secCustomRulesSub) {
            secCustomRulesSub.textContent = `${data.security.waf_custom_rules_count || 0} Custom Regex`;
          }

          const secIpAclStatus = document.getElementById('sec-ip-acl-status');
          if (secIpAclStatus) {
            const allowN = data.security.waf_allowed_ips ? data.security.waf_allowed_ips.length : 0;
            const denyN = data.security.waf_denied_ips ? data.security.waf_denied_ips.length : 0;
            secIpAclStatus.textContent = allowN > 0 || denyN > 0 ? `${allowN} Allow / ${denyN} Deny` : 'Active';
          }

          const cfgWafDetails = document.getElementById('cfg-waf-details');
          if (cfgWafDetails) {
            cfgWafDetails.innerHTML = `
              <div>Mode: <span class="text-rose-400 font-mono font-bold">${(data.security.waf_mode || 'enforce').toUpperCase()} (403)</span></div>
              <div>Anomaly Threshold: <span class="text-slate-200 font-mono">${data.security.waf_anomaly_threshold || 5}</span></div>
              <div>Protocol Smuggling Guard: <span class="text-emerald-400 font-mono">Active</span></div>
            `;
          }

          const cfgIpDetails = document.getElementById('cfg-ip-details');
          if (cfgIpDetails) {
            const allowN = data.security.waf_allowed_ips ? data.security.waf_allowed_ips.length : 0;
            const denyN = data.security.waf_denied_ips ? data.security.waf_denied_ips.length : 0;
            cfgIpDetails.innerHTML = `
              <div>Allowed Subnets: <span class="text-emerald-400 font-mono font-semibold">${allowN} subnets</span></div>
              <div>Denied Subnets: <span class="text-rose-400 font-mono font-semibold">${denyN} subnets</span></div>
              <div>Auto Header Strip: <span class="text-emerald-400 font-mono">Active</span></div>
            `;
          }

          const cfgHeadersDetails = document.getElementById('cfg-headers-details');
          if (cfgHeadersDetails) {
            cfgHeadersDetails.innerHTML = `
              <div>HSTS Header: <span class="text-emerald-400 font-mono">${data.security.security_headers ? 'Enabled' : 'Disabled'}</span></div>
              <div>Frame-Options: <span class="text-emerald-400 font-mono">DENY</span></div>
              <div>X-Content-Type: <span class="text-emerald-400 font-mono">nosniff</span></div>
            `;
          }

          const cfgCorsDetails = document.getElementById('cfg-cors-details');
          if (cfgCorsDetails) {
            cfgCorsDetails.innerHTML = `
              <div>CORS Protection: <span class="text-indigo-400 font-mono">${data.security.cors_enabled ? 'Active' : 'Disabled'}</span></div>
              <div>Preflight MaxAge: <span class="text-slate-200 font-mono">86400s</span></div>
              <div>Mutual TLS (mTLS): <span class="text-emerald-400 font-mono font-semibold">${data.security.mtls_enabled ? 'Active' : 'Ready'}</span></div>
            `;
          }
        }

        if (data.metrics && data.metrics.waf) {
          const secBlockedCount = document.getElementById('sec-blocked-count');
          if (secBlockedCount) {
            secBlockedCount.textContent = `${data.metrics.waf.blocked_total || 0} Intercepted`;
          }
        }
      }
    } catch (e) {
      const headerUptime = document.getElementById('header-uptime');
      if (headerUptime) headerUptime.textContent = 'Offline';
    }

    fetchSecurityIncidents();
  }

  if (btnRefresh) {
    btnRefresh.addEventListener('click', pollStatus);
  }

  // Initial fetch and poll every 5 seconds for security reactivity
  pollStatus();
  setInterval(pollStatus, 5000);
}

