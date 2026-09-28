import { $, $$, esc, dtFmt } from '../utils.js';
import { ICON } from '../components/icons.js';
import { state } from '../state.js';
import { buildDataModel } from '../model.js';
import { openDrawer } from '../components/drawer.js';
import { fetchBackendData, authenticatedFetch } from '../api.js';

export function isValidIP(ip) {
  if (!ip || typeof ip !== 'string') return false;
  const s = ip.trim();
  const v4 = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/;
  if (v4.test(s)) return true;
  if (s.includes(':') && !s.includes(':::') && !/[^0-9a-fA-F:]/.test(s)) {
    const p = s.split(':');
    return p.length >= 3 && p.length <= 8 && p.every(x => x.length <= 4);
  }
  return false;
}

let toastTimer = null;
export function showToast(msg, tone = 'ok') {
  const toast = $('#alToast');
  if (!toast) return;
  if (toastTimer) clearTimeout(toastTimer);
  toast.textContent = msg;
  toast.className = `al-toast ${tone === 'err' ? 'al-toast-err' : 'al-toast-ok'}`;
  toast.style.display = 'block';
  toast.style.position = 'fixed';
  toast.style.bottom = '20px';
  toast.style.right = '20px';
  toast.style.zIndex = '100';
  toast.style.color = tone === 'err' ? 'var(--err)' : 'var(--ok)';
  toast.style.borderColor = tone === 'err' ? 'var(--err)' : 'var(--ok)';
  toastTimer = setTimeout(() => { if (toast) toast.style.display = 'none'; toastTimer = null; }, 4000);
}

export function openUnbanModal(ip) {
  state.alConfirmBan = ip;
  const m = $('#alConfirmModal'), el = $('#alConfirmIp');
  if (el) el.textContent = ip;
  if (m) { m.hidden = false; m.style.display = 'flex'; }
}

export function closeUnbanModal() {
  state.alConfirmBan = null;
  const m = $('#alConfirmModal');
  if (m) { m.hidden = true; m.style.display = 'none'; }
}

export async function executeUnban(targetIp) {
  const ip = targetIp || state.alConfirmBan;
  closeUnbanModal();
  if (!ip) return;
  try {
    const res = await authenticatedFetch('/internal/api/security/unban', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ip })
    });
    if (res.ok) {
      showToast(`IP ${ip} unbanned successfully`, 'ok');
      try { await fetchBackendData(); } catch (_) {}
    } else {
      let m = 'Failed to unban IP';
      try { const err = await res.json(); m = err.message || err.error || m; } catch (_) {}
      showToast(`Failed to unban IP ${ip}: ${m}`, 'err');
    }
  } catch (e) {
    showToast(`Unban request error: ${e.message}`, 'err');
  }
}

export function unbanIpAddress(ip) { openUnbanModal(ip); }
if (typeof window !== 'undefined') window.unbanIpAddress = unbanIpAddress;

export function alShell() {
  const arr = col => state.alBanSort === col ? (state.alBanSortDir === 'asc' ? ' ▲' : ' ▼') : '';
  const th = (col, label, cls = '') => `<th class="sortable click ${cls}" data-sort="${col}" tabindex="0">${label}<span class="s-arr" id="alArr-${col}">${arr(col)}</span></th>`;
  const kpi = (id, cid, title, sub, icon, col = '') => `<div class="al-kpi-card" id="${cid}"><div class="al-kpi-h"><span class="mut">${title}</span><span class="al-kpi-badge">${ICON(icon)}</span></div><div class="al-kpi-val" id="${id}"${col ? ` style="color:${col}"` : ''}>0</div><div class="al-kpi-sub">${sub}</div></div>`;
  const pgn = (pfx, sz, opts) => `<div class="al-foot-bar" id="${pfx}Pagination"><div class="tsum" id="${pfx}Summary"><span id="${pfx}Sum"></span></div><div class="pagination"><button class="btn icon sm" id="${pfx}First" title="First" aria-label="First">&laquo;</button><button class="btn icon sm" id="${pfx}Prev" title="Previous" aria-label="Previous">&lsaquo;</button><span class="page-ind" id="${pfx}PageInd">Page 1 / 1</span><button class="btn icon sm" id="${pfx}Next" title="Next" aria-label="Next">&rsaquo;</button><button class="btn icon sm" id="${pfx}Last" title="Last" aria-label="Last">&raquo;</button><select class="field sm lg-pg-sz" id="${pfx}PageSize" aria-label="Page Size">${opts.map(o => `<option value="${o}"${sz === o ? ' selected' : ''}>${o} / page</option>`).join('')}</select></div></div>`;

  return `<div class="stack">
    <div class="al-kpi-grid">
      ${kpi('alKpiActive', 'alCardActive', 'Active Incidents / Alerts', 'Unresolved operational degradations', 'i-alert')}
      ${kpi('alKpiBlocks', 'alCardBlocks', 'Recent WAF Blocks', 'Threats intercepted in telemetry window', 'i-shield', 'var(--c5)')}
      ${kpi('alKpiTemp', 'alCardTemp', 'Stage 1 Temp Bans', 'Actors under temporary 1-hour quarantine', 'i-dash', 'var(--warn)')}
      ${kpi('alKpiPerm', 'alCardPerm', 'Stage 2 Permanent Bans', 'Repeat offenders permanently firewalled', 'i-x', 'var(--err)')}
    </div>

    <section class="card al-ban-card">
      <div class="card-h"><div><h2>Manual Threat Actor Quarantine</h2><p class="sub">Manually blacklist malicious IPs, automated scanners, or abusive bots</p></div></div>
      <div class="al-ban-box">
        <div style="display:flex;flex-wrap:wrap;gap:8px;align-items:center;width:100%">
          <input type="text" id="alBanIp" class="field ban-fld" placeholder="IPv4 or IPv6 address" aria-label="IP Address">
          <select id="alBanTier" class="field ban-sel" aria-label="Ban Duration Preset">
            <option value="1h" selected>Stage 1: 1h Temp</option>
            <option value="15m">15 Minutes</option>
            <option value="6h">6 Hours</option>
            <option value="24h">24 Hours</option>
            <option value="7d">7 Days</option>
            <option value="Permanent">Stage 2: Permanent</option>
            <option value="custom">Custom Duration...</option>
          </select>
          <input type="text" id="alBanCustomDuration" class="field ban-fld" placeholder="Duration (e.g. 30m, 48h)" style="display:none;width:140px" aria-label="Custom Duration">
          <input type="text" id="alBanReason" class="field" style="flex:1;min-width:180px" placeholder="Context / Reason (e.g. Scanning probe on /v1/auth)" aria-label="Ban Reason">
          <button class="btn primary ban-btn" id="alBanBtn">Ban Threat Actor</button>
        </div>
        <div id="alBanError" class="al-ban-err" style="display:none"></div>
      </div>
    </section>

    <section class="card" style="margin-top:16px">
      <div class="card-h"><div><h2>Active Alerts &amp; WAF Incidents</h2><p class="sub">Security events, anomalies, and operational alerts requiring attention</p></div></div>
      <div class="al-toolbar">
        <div class="search">${ICON('i-search')}<input class="field" id="alIncSearch" type="search" placeholder="Search incidents (IP, rule, path, payload)..." aria-label="Search Incidents" value="${esc(state.alIncQ)}"></div>
        <div class="chips" id="alSevChips">
          <button class="chip al-sev-chip" data-sev="all" aria-pressed="${state.alSev === 'all'}">All</button>
          <button class="chip al-sev-chip" data-sev="critical" aria-pressed="${state.alSev === 'critical'}">Critical</button>
          <button class="chip al-sev-chip" data-sev="warning" aria-pressed="${state.alSev === 'warning'}">Warning</button>
        </div>
      </div>
      <div id="alActive"><ul class="rows al" id="alIncList" style="margin-top:0"></ul></div>
      ${pgn('alInc', state.alIncPageSize, [10, 25, 50])}
    </section>

    <section class="card" style="margin-top:16px">
      <div class="card-h"><div><h2>Dynamic 2-Stage Auto-Ban &amp; Blocked IPs</h2><p class="sub">Automated threat defense reactor and persistent IP firewall entries</p></div></div>
      <div class="al-toolbar">
        <div class="search">${ICON('i-search')}<input class="field" id="alBanSearch" type="search" placeholder="Search bans (IP, reason, category)..." aria-label="Search Banned IPs" value="${esc(state.alBanQ)}"></div>
        <div class="chips" id="alTierChips">
          <button class="chip al-tier-chip" data-tier="all" aria-pressed="${state.alTier === 'all'}">All</button>
          <button class="chip al-tier-chip" data-tier="temporary" aria-pressed="${state.alTier === 'temporary'}">Stage 1 (1h Temp)</button>
          <button class="chip al-tier-chip" data-tier="permanent" aria-pressed="${state.alTier === 'permanent'}">Stage 2 (Permanent)</button>
        </div>
      </div>
      <div class="card-b tscroll" style="padding:0">
        <table class="tbl" id="bannedIpsTable">
          <thead id="alBanHead"><tr style="background:var(--surface-2)">${th('ip', 'Client IP')}${th('type', 'Ban Tier')}${th('created_at', 'Created At')}${th('temp_ban_count', 'Temp Bans', 'num')}${th('reason', 'Reason / Category')}${th('remaining_seconds', 'TTL / Expiry')}<th style="text-align:right">Action</th></tr></thead>
          <tbody id="alBanBody"><tr><td colspan="7" style="padding:16px;text-align:center;color:var(--ink-2)">Loading threat table...</td></tr></tbody>
        </table>
      </div>
      ${pgn('alBan', state.alBanPageSize, [10, 25, 50])}
    </section>

    <div id="alConfirmModal" class="modal-scrim" hidden style="display:none">
      <div class="modal-card">
        <div class="modal-h"><h2>Confirm Threat Actor Unban</h2><button class="btn icon sm" id="alConfirmClose" aria-label="Close modal">${ICON('i-x')}</button></div>
        <div class="modal-b">
          <p style="margin-bottom:12px">Are you sure you want to remove the firewall ban for client IP <b id="alConfirmIp" class="td-mono"></b>?</p>
          <p class="sub" style="font-size:12px;color:var(--ink-2)">Traffic from this address will no longer be dropped by automated edge filters.</p>
          <div class="modal-foot"><button class="btn" id="alConfirmCancel">Cancel</button><button class="btn primary" id="alConfirmOk">Confirm Unban</button></div>
        </div>
      </div>
    </div>
    <div id="alToast" class="al-toast" style="display:none;position:fixed;bottom:20px;right:20px;z-index:100"></div>
  </div>`;
}

export function alInit() { alUpdate(); }

export function alUpdate(D) {
  const model = D || buildDataModel();

  // 1. KPI Strip
  const activeCount = (model.alerts || []).length;
  const blockedCount = (model.incidents || []).filter(x => x.action === 'blocked').length;
  const tempCount = (model.bannedIps || []).filter(x => x.type === 'temporary').length;
  const permCount = (model.bannedIps || []).filter(x => x.type === 'permanent').length;

  const kpiActive = $('#alKpiActive');
  if (kpiActive) {
    kpiActive.textContent = String(activeCount);
    kpiActive.style.color = activeCount > 0 ? 'var(--err)' : 'var(--ok)';
  }
  const kpiBlocks = $('#alKpiBlocks'); if (kpiBlocks) kpiBlocks.textContent = String(blockedCount);
  const kpiTemp = $('#alKpiTemp'); if (kpiTemp) kpiTemp.textContent = String(tempCount);
  const kpiPerm = $('#alKpiPerm'); if (kpiPerm) kpiPerm.textContent = String(permCount);

  const setPgn = (pfx, cur, tot, len, start, end) => {
    const sum = $(`#${pfx}Summary`) || $(`#${pfx}Sum`);
    if (sum) sum.textContent = `Showing ${len ? start + 1 : 0}–${end} of ${len}`;
    const ind = $(`#${pfx}PageInd`);
    if (ind) ind.textContent = `Page ${cur} / ${tot}`;
    const f = $(`#${pfx}First`), p = $(`#${pfx}Prev`), n = $(`#${pfx}Next`), l = $(`#${pfx}Last`);
    if (f) f.disabled = cur <= 1;
    if (p) p.disabled = cur <= 1;
    if (n) n.disabled = cur >= tot;
    if (l) l.disabled = cur >= tot;
  };

  // 2. Incidents Filtering
  const incs = (model.incidents && model.incidents.length > 0) ? model.incidents : (model.alerts || []);
  const filteredIncidents = incs.filter(inc => {
    if (state.alSev === 'critical') {
      const isCrit = inc.action === 'blocked' || (inc.anomaly_score != null && Number(inc.anomaly_score) >= 10) || inc.sev === 'critical';
      if (!isCrit) return false;
    } else if (state.alSev === 'warning') {
      const isWarn = inc.action === 'logged' || (inc.anomaly_score != null && Number(inc.anomaly_score) < 10) || inc.sev === 'warning';
      if (!isWarn) return false;
    }

    if (state.alIncQ) {
      const q = state.alIncQ.toLowerCase();
      const cat = (inc.category || inc.title || '').toLowerCase();
      let aliases = '';
      if (cat.includes('sql') || inc.rule_id === '942100') aliases += ' sqli';
      if (cat.includes('cross-site') || inc.rule_id === '941100') aliases += ' xss';
      if (cat.includes('traversal') || inc.rule_id === '930100') aliases += ' traversal lfi';
      const targetStr = `${inc.client_ip || inc.ip || ''} ${inc.rule_id || ''} ${inc.path || ''} ${cat} ${inc.payload_snippet || ''} ${aliases}`.toLowerCase();
      if (!targetStr.includes(q)) return false;
    }
    return true;
  });

  const totalIncPages = Math.max(1, Math.ceil(filteredIncidents.length / state.alIncPageSize));
  if (state.alIncPage > totalIncPages) state.alIncPage = totalIncPages;
  if (state.alIncPage < 1) state.alIncPage = 1;
  const incStart = (state.alIncPage - 1) * state.alIncPageSize;
  const incEnd = Math.min(filteredIncidents.length, incStart + state.alIncPageSize);
  const incPageItems = filteredIncidents.slice(incStart, incEnd);

  const incList = $('#alIncList') || $('#alActive');
  if (incList) {
    if (incPageItems.length === 0) {
      incList.innerHTML = `<li class="empty" style="text-align:center;padding:24px 12px;color:var(--ink-2)">${ICON('i-check')}<span style="margin-left:6px">Zero security incidents match active filters.</span></li>`;
    } else {
      incList.innerHTML = incPageItems.map((inc, i) => {
        const idStr = inc.id || `inc-${incStart + i + 1}`;
        const isBlocked = inc.action === 'blocked';
        const fullTime = dtFmt(inc.timestamp);
        const scoreBadge = (inc.anomaly_score != null) ? `<span class="chipx" style="font-size:11px">Score: ${inc.anomaly_score}</span>` : '';
        const locationBadge = inc.location ? `<span class="chipx" style="font-size:11px">${esc(inc.location)}</span>` : '';

        return `<li id="${esc(idStr)}" class="al-inc-item click" data-inc-id="${esc(idStr)}" tabindex="0" role="link">
          <span class="${isBlocked ? 't-err' : 't-warn'}">${ICON(isBlocked ? 'i-x' : 'i-alert')}</span>
          <div style="min-width:0;flex:1">
            <div class="al-t" style="display:flex;align-items:center;gap:8px;flex-wrap:wrap">
              <b>${esc(inc.category || inc.title || 'Security Anomaly')}</b>
              <span class="st ${isBlocked ? 's5' : 's4'}" style="font-size:11px;padding:1px 6px">${isBlocked ? 'Blocked' : 'Logged'}</span>
              <code style="font-size:11.5px">${esc(inc.method || 'GET')} ${esc(inc.path || '/')}</code>
            </div>
            <div class="al-d" style="display:flex;align-items:center;gap:8px;margin-top:2px;flex-wrap:wrap">
              <span>Rule: <code>${esc(inc.rule_id || 'unknown')}</code></span>
              <span>IP: <code>${esc(inc.client_ip || inc.ip || 'unknown')}</code></span>
              ${scoreBadge}${locationBadge}
            </div>
          </div>
          <span class="mut num" style="white-space:nowrap;font-size:12px">${esc(fullTime)}</span>
        </li>`;
      }).join('');
    }
  }
  setPgn('alInc', state.alIncPage, totalIncPages, filteredIncidents.length, incStart, incEnd);

  // 3. Bans Filtering & Sorting
  const rawBans = model.bannedIps || [];
  const filteredBans = rawBans.filter(b => {
    if (state.alTier === 'temporary' && b.type !== 'temporary') return false;
    if (state.alTier === 'permanent' && b.type !== 'permanent') return false;
    if (state.alBanQ) {
      const q = state.alBanQ.toLowerCase();
      const targetStr = `${b.ip || ''} ${b.reason || ''} ${b.last_category || ''}`.toLowerCase();
      if (!targetStr.includes(q)) return false;
    }
    return true;
  });

  const mult = state.alBanSortDir === 'asc' ? 1 : -1;
  filteredBans.sort((a, b) => {
    const col = state.alBanSort;
    let diff = 0;
    if (col === 'ip') diff = (a.ip || '').localeCompare(b.ip || '', undefined, { numeric: true });
    else if (col === 'type') diff = (a.type || '').localeCompare(b.type || '');
    else if (col === 'created_at') diff = new Date(a.created_at || 0) - new Date(b.created_at || 0);
    else if (col === 'temp_ban_count') diff = (Number(a.temp_ban_count) || 0) - (Number(b.temp_ban_count) || 0);
    else if (col === 'remaining_seconds') diff = (Number(a.remaining_seconds) || 0) - (Number(b.remaining_seconds) || 0);
    else if (col === 'reason') diff = (a.reason || a.last_category || '').localeCompare(b.reason || b.last_category || '');
    return diff !== 0 ? diff * mult : (a.ip || '').localeCompare(b.ip || '', undefined, { numeric: true });
  });

  const totalBanPages = Math.max(1, Math.ceil(filteredBans.length / state.alBanPageSize));
  if (state.alBanPage > totalBanPages) state.alBanPage = totalBanPages;
  if (state.alBanPage < 1) state.alBanPage = 1;
  const banStart = (state.alBanPage - 1) * state.alBanPageSize;
  const banEnd = Math.min(filteredBans.length, banStart + state.alBanPageSize);
  const banPageItems = filteredBans.slice(banStart, banEnd);

  const tb = $('#alBanBody') || $('#bannedIpsTable');
  if (tb) {
    if (banPageItems.length === 0) {
      tb.innerHTML = `<tr><td colspan="7" style="padding:20px;text-align:center;color:var(--ink-2)">${ICON('i-check')} Zero active IP bans in effect.</td></tr>`;
    } else {
      tb.innerHTML = banPageItems.map(b => {
        const isPerm = b.type === 'permanent';
        const tierBadge = isPerm 
          ? '<span class="st s4 tier-perm">Stage 2: Permanent</span>'
          : '<span class="st s3 tier-temp">Stage 1: Temporary</span>';
        let ttlStr = 'Never (Permanent)';
        if (!isPerm && b.remaining_seconds != null && b.remaining_seconds >= 0) {
          ttlStr = `${Math.floor(b.remaining_seconds / 60)}m ${b.remaining_seconds % 60}s remaining`;
        }
        return `<tr>
          <td class="td-mono" style="font-weight:600">${esc(b.ip)}</td>
          <td>${tierBadge}</td>
          <td class="td-mono" style="font-size:12px;white-space:nowrap">${dtFmt(b.created_at)}</td>
          <td class="td-mono num">${b.temp_ban_count || 0}</td>
          <td class="td-reason" title="${esc(b.reason || '')}">${esc(b.reason || b.last_category || 'WAF violation')}</td>
          <td style="font-size:12px;color:var(--ink-2)">${esc(ttlStr)}</td>
          <td style="text-align:right">
            <button class="btn al-unban-btn" style="padding:2px 8px;font-size:11px" data-ip="${esc(b.ip)}">Unban</button>
          </td>
        </tr>`;
      }).join('');
    }
  }

  ['ip', 'type', 'created_at', 'temp_ban_count', 'reason', 'remaining_seconds'].forEach(c => {
    const sa = $(`#alArr-${c}`);
    if (sa) sa.textContent = state.alBanSort === c ? (state.alBanSortDir === 'asc' ? ' ▲' : ' ▼') : '';
    const thEl = $(`#alBanHead th[data-sort="${c}"]`);
    if (thEl && typeof thEl.querySelector === 'function' && !thEl.querySelector('.s-arr')) {
      const arrow = state.alBanSort === c ? (state.alBanSortDir === 'asc' ? ' ▲' : ' ▼') : '';
      thEl.textContent = thEl.textContent.replace(/[▲▼]/g, '').trim() + (arrow ? ' ' + arrow.trim() : '');
    }
  });

  setPgn('alBan', state.alBanPage, totalBanPages, filteredBans.length, banStart, banEnd);

  // Event handlers
  $$('.al-unban-btn').forEach(btn => {
    btn.onclick = () => openUnbanModal(btn.dataset.ip);
  });

  const banBtn = $('#alBanBtn') || $('#manualBanBtn');
  if (banBtn) {
    banBtn.onclick = async () => {
      const ipIn = $('#alBanIp') || $('#manualBanIP');
      const tierIn = $('#alBanTier') || $('#manualBanPreset');
      const customIn = $('#alBanCustomDuration');
      const reasonIn = $('#alBanReason') || $('#manualBanReason');
      const errEl = $('#alBanError');

      const ip = ipIn?.value ? ipIn.value.trim() : '';
      if (!isValidIP(ip)) {
        if (errEl) {
          errEl.textContent = 'Invalid IPv4 or IPv6 address';
          errEl.style.display = 'block';
        }
        return;
      }
      if (errEl) errEl.style.display = 'none';

      const preset = tierIn?.value || '1h';
      const isPerm = preset.toLowerCase() === 'permanent';
      const type = isPerm ? 'permanent' : 'temporary';
      const duration = isPerm ? undefined : (preset === 'custom' ? (customIn?.value?.trim() || '1h') : preset);
      const reason = reasonIn?.value?.trim() || 'Manually banned via dashboard';

      const payload = { ip, type, reason };
      if (duration) payload.duration = duration;

      try {
        const res = await authenticatedFetch('/internal/api/security/ban', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload)
        });
        if (res.ok) {
          if (ipIn) ipIn.value = '';
          if (reasonIn) reasonIn.value = '';
          showToast(`IP ${ip} banned successfully`, 'ok');
          try { await fetchBackendData(); } catch (_) {}
        } else {
          let msg = 'Failed to ban IP';
          try { const d = await res.json(); msg = d.error || d.message || msg; } catch (_) {}
          showToast(`Failed to ban IP: ${msg}`, 'err');
        }
      } catch (e) {
        showToast(`Ban request error: ${e.message}`, 'err');
      }
    };
  }

  const banTierSel = $('#alBanTier') || $('#manualBanPreset');
  if (banTierSel) {
    banTierSel.onchange = e => {
      const customIn = $('#alBanCustomDuration');
      if (customIn) customIn.style.display = e.target.value === 'custom' ? 'inline-block' : 'none';
    };
  }

  const incSearch = $('#alIncSearch');
  if (incSearch) {
    incSearch.oninput = e => {
      state.alIncQ = e.target.value.trim().toLowerCase();
      state.alIncPage = 1;
      alUpdate();
    };
  }
  $$('.al-sev-chip').forEach(btn => {
    btn.onclick = () => {
      state.alSev = btn.dataset.sev || 'all';
      state.alIncPage = 1;
      $$('.al-sev-chip').forEach(b => b.setAttribute('aria-pressed', String(b.dataset.sev === state.alSev)));
      alUpdate();
    };
  });

  const bind = (sel, fn) => { const el = $(sel); if (el) el.onclick = fn; };
  const bindPgn = (pfx, pageProp, szProp) => {
    bind(`#${pfx}First`, () => { state[pageProp] = 1; alUpdate(); });
    bind(`#${pfx}Prev`, () => { state[pageProp] = Math.max(1, state[pageProp] - 1); alUpdate(); });
    bind(`#${pfx}Next`, () => { state[pageProp]++; alUpdate(); });
    bind(`#${pfx}Last`, () => { state[pageProp] = 999999; alUpdate(); });
    const szEl = $(`#${pfx}PageSize`);
    if (szEl) szEl.onchange = e => { state[szProp] = Number(e.target.value) || 10; state[pageProp] = 1; alUpdate(); };
  };
  bindPgn('alInc', 'alIncPage', 'alIncPageSize');

  $$('.al-inc-item').forEach(item => {
    item.onclick = () => openDrawer('incident', item.dataset.incId);
  });

  const banSearch = $('#alBanSearch');
  if (banSearch) {
    banSearch.oninput = e => {
      state.alBanQ = e.target.value.trim().toLowerCase();
      state.alBanPage = 1;
      alUpdate();
    };
  }
  $$('.al-tier-chip').forEach(btn => {
    btn.onclick = () => {
      state.alTier = btn.dataset.tier || 'all';
      state.alBanPage = 1;
      $$('.al-tier-chip').forEach(b => b.setAttribute('aria-pressed', String(b.dataset.tier === state.alTier)));
      alUpdate();
    };
  });

  $$('#alBanHead th.sortable').forEach(thEl => {
    thEl.onclick = () => {
      const col = thEl.dataset.sort;
      state.alBanSortDir = (state.alBanSort === col) ? (state.alBanSortDir === 'asc' ? 'desc' : 'asc') : 'desc';
      state.alBanSort = col;
      alUpdate();
    };
  });

  bindPgn('alBan', 'alBanPage', 'alBanPageSize');

  bind('#alConfirmCancel', closeUnbanModal);
  bind('#alConfirmClose', closeUnbanModal);
  bind('#alConfirmOk', () => executeUnban());
}
