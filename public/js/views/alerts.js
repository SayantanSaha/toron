/**
 * Toron Dashboard - View 7: Alerts & Threat Defense
 */
import { $, esc, dtFmt } from '../utils.js';
import { ICON } from '../components/icons.js';
import { fetchBackendData } from '../api.js';

export function alShell() {
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

export async function unbanIpAddress(ip) {
  if (typeof confirm === 'function' && !confirm(`Are you sure you want to unban IP ${ip}?`)) return;
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
      if (typeof alert === 'function') alert('Failed to unban IP: ' + (err.message || 'Unknown error'));
    }
  } catch (e) {
    if (typeof alert === 'function') alert('Unban request error: ' + e.message);
  }
}

if (typeof window !== 'undefined') {
  window.unbanIpAddress = unbanIpAddress;
}

export function alUpdate(D) {
  const alEl = $('#alActive');
  if (alEl) {
    alEl.innerHTML = D.alerts.map(a => `<li data-go="${a.go}" tabindex="0" role="link"><span class="${a.sev === 'critical' ? 't-err' : 't-warn'}">${ICON(a.sev === 'critical' ? 'i-x' : 'i-alert')}</span><div><div class="al-t">${esc(a.title)}</div><div class="al-d">${esc(a.detail(D))}</div></div><span class="mut num" style="white-space:nowrap;font-size:12px">${dtFmt(a.timestamp)}</span></li>`).join('') || `<li><span class="t-ok">${ICON('i-check')}</span><div><div class="al-t">All Systems Operational</div><div class="al-d">Zero unresolved security incidents.</div></div></li>`;
  }

  const tb = $('#bannedIpsTable');
  if (tb) {
    const bans = (D.bannedIps || []).slice().sort((a, b) => {
      const dt = new Date(b.created_at || 0).getTime() - new Date(a.created_at || 0).getTime();
      if (dt !== 0) return dt;
      return (a.ip || '').localeCompare(b.ip || '', undefined, { numeric: true });
    });
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
          if (typeof alert === 'function') alert('Failed to ban IP: ' + (err.message || 'Unknown error'));
        }
      } catch (e) {
        if (typeof alert === 'function') alert('Ban request error: ' + e.message);
      }
    };
  }
}
