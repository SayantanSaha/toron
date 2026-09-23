/**
 * Toron Dashboard - View 8: API Console & Proxy Debugger
 */
import { $ } from '../utils.js';
import { buildDataModel } from '../model.js';

export function consoleShell() {
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

export function consoleInit() {
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

export function consoleUpdate() {}
