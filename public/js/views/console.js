import { $ } from '../utils.js';
import { buildDataModel } from '../model.js';
import { authenticatedFetch } from '../api.js';

export function consoleShell() {
  const routes = buildDataModel().routes;
  return `<section class="card">
    <div class="card-h"><div><h2>API Console & Proxy Debugger</h2><p class="sub">Dispatch test probes against configured gateway routes and inspect headers & responses</p></div></div>
    <div class="card-b">
      <form id="consoleForm" class="cs-form">
        <div class="cs-grid">
          <div><label class="mut cs-lbl">Method</label><select id="csMethod" class="field cs-w100"><option value="GET">GET</option><option value="POST">POST</option><option value="HEAD">HEAD</option></select></div>
          <div><label class="mut cs-lbl">Endpoint / Path</label><select id="csPath" class="field cs-w100"><option value="/health">GET /health (Server Health)</option><option value="/internal/api/status">GET /internal/api/status (Engine Metrics)</option>${routes.map(r => `<option value="${r.path}">ANY ${r.path} [${r.host}]</option>`).join('')}</select></div>
          <div><label class="mut cs-lbl">Host Override</label><input type="text" id="csHost" class="field cs-w100" placeholder="e.g. api.example.com"></div>
        </div>
        <div style="display:flex;justify-content:flex-end"><button type="submit" class="btn primary">Execute Probe</button></div>
      </form>
      <div class="cs-out-box">
        <div class="cs-out-h"><span style="font-weight:600;font-size:13px">Response Output:</span><span id="csStatus" class="st s2">Ready</span></div>
        <pre id="csBody" class="cs-pre">Press "Execute Probe" to test route...</pre>
      </div>
    </div>
  </section>`;
}

export function consoleInit() {
  const form = $('#consoleForm');
  if (form) {
    form.addEventListener('submit', async e => {
      e.preventDefault();
      const method = $('#csMethod').value, path = $('#csPath').value, host = $('#csHost').value;
      const stEl = $('#csStatus'), bodyEl = $('#csBody');
      if (stEl) stEl.textContent = 'Executing...';
      try {
        const headers = host ? { Host: host } : {};
        const res = await authenticatedFetch('/internal/api/proxy-test', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ path, method, headers })
        });
        const data = await res.json();
        if (stEl) stEl.textContent = `HTTP ${data.status_code || res.status} (${(data.latency_ms || 0).toFixed(1)}ms)`;
        if (bodyEl) {
          try { bodyEl.textContent = JSON.stringify(JSON.parse(data.body), null, 2); }
          catch { bodyEl.textContent = data.body || JSON.stringify(data, null, 2); }
        }
      } catch (err) {
        if (stEl) stEl.textContent = 'Error';
        if (bodyEl) bodyEl.textContent = err.message;
      }
    });
  }
}

export function consoleUpdate() {}
