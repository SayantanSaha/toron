/**
 * Toron Dashboard - Application State & Theme Store
 */
import { $$ } from './utils.js';

export const N = 60;

export const RANGES = {
  '15m': { bucket: 15, label: 'Last 15 minutes' },
  '1h':  { bucket: 60, label: 'Last hour' },
  '6h':  { bucket: 360, label: 'Last 6 hours' },
  '24h': { bucket: 1440, label: 'Last 24 hours' }
};

export const state = {
  view: 'overview',
  range: '1h',
  instance: 'all',
  live: true,
  step: 0,
  sort: { k: 'rps', dir: -1 },
  q: '',
  lq: '',
  lst: new Set(['2', '3', '4', '5']),
  lroute: 'all',
  lslow: false
};

let cache = null;
let FORCE = false;

export const getCache = () => cache;
export const setCache = val => { cache = val; };
export const invalidate = () => { cache = null; };
export const isForce = () => FORCE;
export const setForce = val => { FORCE = !!val; };

let onLiveChangeHandler = null;
export const setOnLiveChange = fn => { onLiveChangeHandler = fn; };
export const setLive = v => {
  state.live = v;
  if (onLiveChangeHandler) onLiveChangeHandler(v);
};

/* ---------- Theme Engine ---------- */
const mq = (typeof window !== 'undefined' && window.matchMedia)
  ? window.matchMedia('(prefers-color-scheme: dark)')
  : { matches: false };

export function effTheme() {
  if (typeof document === 'undefined') return 'dark';
  return document.documentElement.getAttribute('data-theme') || (mq.matches ? 'dark' : 'light');
}

export function themeIcon() {
  if (typeof document === 'undefined') return;
  const t = effTheme();
  $$('#themeBtn use, #themeBtn2 use').forEach(u => u.setAttribute('href', t === 'dark' ? '#i-sun' : '#i-moon'));
}

export function toggleTheme() {
  if (typeof document === 'undefined') return;
  const n = effTheme() === 'dark' ? 'light' : 'dark';
  document.documentElement.setAttribute('data-theme', n);
  try { localStorage.setItem('toron-theme', n); } catch (e) {}
  themeIcon();
}
