/**
 * Toron Dashboard - Core DOM, String & Mathematical Utilities
 */
export const $ = (s, r = (typeof document !== 'undefined' ? document : null)) => r ? r.querySelector(s) : null;
export const $$ = (s, r = (typeof document !== 'undefined' ? document : null)) => r ? [...r.querySelectorAll(s)] : [];

export const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;'
}[c]));

export const clamp = (v, a, b) => Math.min(b, Math.max(a, v));
export const sum = a => (a || []).reduce((x, y) => x + y, 0);
export const avg = a => (a && a.length) ? sum(a) / a.length : 0;
export const p2 = n => String(n).padStart(2, '0');

export const hms = d => `${p2(d.getHours())}:${p2(d.getMinutes())}:${p2(d.getSeconds())}`;
export const hm = d => `${p2(d.getHours())}:${p2(d.getMinutes())}`;

export const dtFmt = d => {
  if (!d) return '';
  const t = (d instanceof Date) ? d : new Date(d);
  if (isNaN(t.getTime())) return String(d);
  return `${t.getFullYear()}-${p2(t.getMonth() + 1)}-${p2(t.getDate())} ${p2(t.getHours())}:${p2(t.getMinutes())}:${p2(t.getSeconds())}`;
};

export const ago = s => s < 90 ? `${Math.round(s)} s` : s < 5400 ? `${Math.round(s / 60)} min` : s < 129600 ? `${Math.round(s / 3600)} h` : `${Math.round(s / 86400)} d`;

export const fmt = {
  n: v => v >= 1e6 ? (v / 1e6).toFixed(2) + 'M' : v >= 1e4 ? (v / 1e3).toFixed(1) + 'k' : v >= 1e3 ? (v / 1e3).toFixed(2) + 'k' : v >= 100 ? String(Math.round(v)) : v >= 10 ? v.toFixed(0) : v.toFixed(1),
  ms: v => v >= 1000 ? (v / 1000).toFixed(2) + ' s' : v >= 100 ? Math.round(v) + ' ms' : v.toFixed(1) + ' ms',
  pct: (v, d = 2) => (v * 100).toFixed(d) + '%',
  bytes: b => b >= 1048576 ? (b / 1048576).toFixed(1) + ' MB' : b >= 1024 ? (b / 1024).toFixed(1) + ' KB' : Math.round(b) + ' B'
};

export const axis = v => v >= 1000 ? (+(v / 1000).toFixed(1)) + 'k' : String(+v.toFixed(v < 10 ? 2 : v < 100 ? 1 : 0));

export const stCls = s => s === 101 ? '2' : String(s)[0];

export const H = (a, b = 0) => {
  const x = Math.sin(a * 12.9898 + b * 78.233) * 43758.5453;
  return x - Math.floor(x);
};
