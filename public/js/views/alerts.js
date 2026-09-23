import { $, esc, dtFmt } from '../utils.js';
import { ICON } from '../components/icons.js';
import { fetchBackendData, authenticatedFetch } from '../api.js';

export function alShell() {
  return `<div class="stack">
    <section class="card">
      <div class="card-h"><div><h2>Active Alerts & WAF Incidents</h2><p class="sub">Security events, anomalies, and operational alerts requiring attention</p></div></div>
      <ul class="rows al" id="alActive" style="margin-top:8px"></ul>
    </section>
    <section class="card" style="margin-top:16px">
      <div class="card-h" style="display:flex;justify-content:space-between;align-items:center">
        <div><h2>Dynamic 2-Stage Auto-Ban & Blocked IPs</h2><p class="sub">Automated threat defense reactor and persistent IP firewall entries</p></div>
        <div style="display:flex;gap:8px;align-items:center">
          <input type="text" id="manualBanIP" class="field ban-fld" placeholder="IP to ban (e.g. 1.2.3.4)">
          <select id="manualBanType" class="field ban-sel"><option value="temporary">Stage 1 (1h Temp)</option><option value="permanent">Stage 2 (Permanent)</option></select>
          <button class="btn primary ban-btn" id="manualBanBtn">Ban IP</button>
        </div>
      </div>
      <div class="card-b" style="padding:0;overflow-x:auto">
        <table class="tbl">
          <thead><tr style="background:var(--surface-2)"><th>Client IP</th><th>Ban Tier</th><th>Created At</th><th>Temp Bans</th><th>Reason / Category</th><th>TTL / Expiry</th><th style="text-align:right">Action</th></tr></thead>
          <tbody id="bannedIpsTable"><tr><td colspan="7" style="padding:16px;text-align:center;color:var(--ink-2)">Loading threat table...</td></tr></tbody>
        </table>
      </div>
    </section>
  </div>`;
}

export async function unbanIpAddress(ip) {
  if (typeof confirm === 'function' && !confirm(`Are you sure you want to unban IP ${ip}?`)) return;
  try {
    const res = await authenticatedFetch('/internal/api/security/unban', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ip })
    });
    if (res.ok) await fetchBackendData();
    else {
      const err = await res.json();
      if (typeof alert === 'function') alert('Failed to unban IP: ' + (err.message || 'Unknown error'));
    }
  } catch (e) {
    if (typeof alert === 'function') alert('Unban request error: ' + e.message);
  }
}

if (typeof window !== 'undefined') window.unbanIpAddress = unbanIpAddress;

export function alUpdate(D) {
  const alEl = $('#alActive');
  if (alEl) {
    alEl.innerHTML = D.alerts.map(a => `<li data-go="${a.go}" tabindex="0" role="link"><span class="${a.sev === 'critical' ? 't-err' : 't-warn'}">${ICON(a.sev === 'critical' ? 'i-x' : 'i-alert')}</span><div><div class="al-t">${esc(a.title)}</div><div class="al-d">${esc(a.detail(D))}</div></div><span class="mut num" style="white-space:nowrap;font-size:12px">${dtFmt(a.timestamp)}</span></li>`).join('') || `<li><span class="t-ok">${ICON('i-check')}</span><div><div class="al-t">All Systems Operational</div><div class="al-d">Zero unresolved security incidents.</div></div></li>`;
  }

  const tb = $('#bannedIpsTable');
  if (tb) {
    const bans = (D.bannedIps || []).slice().sort((a, b) => {
      const dt = new Date(b.created_at || 0).getTime() - new Date(a.created_at || 0).getTime();
      return dt !== 0 ? dt : (a.ip || '').localeCompare(b.ip || '', undefined, { numeric: true });
    });
    if (bans.length === 0) {
      tb.innerHTML = `<tr><td colspan="7" style="padding:20px;text-align:center;color:var(--text-muted)">${ICON('i-check')} Zero active IP bans in effect.</td></tr>`;
    } else {
      tb.innerHTML = bans.map(b => {
        const isPerm = b.type === 'permanent';
        const tierBadge = isPerm 
          ? '<span class="st s4 tier-perm">Stage 2: Permanent</span>'
          : '<span class="st s3 tier-temp">Stage 1: Temporary</span>';
        let ttlStr = 'Never (Permanent)';
        if (!isPerm && b.remaining_seconds >= 0) {
          ttlStr = `${Math.floor(b.remaining_seconds / 60)}m ${b.remaining_seconds % 60}s remaining`;
        }
        return `<tr>
          <td class="td-mono" style="font-weight:600">${esc(b.ip)}</td>
          <td>${tierBadge}</td>
          <td class="td-mono" style="font-size:12px;white-space:nowrap">${dtFmt(b.created_at)}</td>
          <td class="td-mono">${b.temp_ban_count || 0}</td>
          <td class="td-reason" title="${esc(b.reason || '')}">${esc(b.reason || b.last_category || 'WAF violation')}</td>
          <td style="font-size:12px;color:var(--ink-2)">${esc(ttlStr)}</td>
          <td style="text-align:right"><button class="btn" style="padding:2px 8px;font-size:11px" onclick="unbanIpAddress('${esc(b.ip)}')">Unban</button></td>
        </tr>`;
      }).join('');
    }
  }

  const banBtn = $('#manualBanBtn');
  if (banBtn && !banBtn._bound) {
    banBtn._bound = true;
    banBtn.onclick = async () => {
      const ipIn = $('#manualBanIP'), typeIn = $('#manualBanType');
      if (!ipIn?.value?.trim()) return;
      const ip = ipIn.value.trim(), type = typeIn ? typeIn.value : 'temporary';
      try {
        const res = await authenticatedFetch('/internal/api/security/ban', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ ip, type, reason: 'Manually banned via dashboard' })
        });
        if (res.ok) { ipIn.value = ''; await fetchBackendData(); }
        else {
          const err = await res.json();
          if (typeof alert === 'function') alert('Failed to ban IP: ' + (err.message || 'Unknown error'));
        }
      } catch (e) {
        if (typeof alert === 'function') alert('Ban request error: ' + e.message);
      }
    };
  }
}
