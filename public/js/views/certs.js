import { $, esc, clamp } from '../utils.js';
import { pill } from '../components/icons.js';

export function ceShell() {
  return `<section class="card"><div class="tscroll"><table class="tbl"><thead><tr><th>Domain</th><th class="hide-sm">Challenge</th><th>Expires</th><th class="hide-md" style="width:22%">Time remaining</th><th>Renewal</th></tr></thead><tbody id="ceBody"></tbody></table></div></section>
    <p class="hc-legend">Certificates are automatically issued and renewed by the ACME zero-touch engine 30 days before expiry.</p>`;
}

export function ceUpdate(D) {
  const body = $('#ceBody');
  if (!body) return;
  body.innerHTML = D.certs.map(c => {
    const t = c.state === 'failing' ? 'err' : (c.days <= 30 ? 'warn' : 'ok');
    const d = new Date(Date.now() + c.days * 864e5).toLocaleDateString('en-US', { day: 'numeric', month: 'short', year: 'numeric' });
    return `<tr><td><div class="rt"><b>${esc(c.domain)}</b><span class="mut">Let's Encrypt</span></div></td><td class="hide-sm">${esc(c.chal)}</td><td><div class="rt"><b>${d}</b><span class="${t === 'err' ? 't-err' : 'mut'}">in ${c.days} days</span></div></td><td class="hide-md"><div class="bar"><i style="width:${clamp(c.days / 90 * 100, 5, 100)}%;background:${t === 'ok' ? 'var(--ok)' : (t === 'warn' ? 'var(--c4)' : 'var(--c5)')}"></i></div></td><td>${c.state === 'failing' ? pill('err', 'Renewal Failing') : pill('ok', `Renews in ${Math.max(0, c.days - 30)}d`)}</td></tr>` + (c.err ? `<tr class="dtl"><td colspan="5"><div class="note err">${esc(c.err)}</div></td></tr>` : '');
  }).join('');
}
