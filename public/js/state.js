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
  lslow: false,
  ltime: 'all',
  lfrom: null,
  lto: null,
  lnohealth: false,
  lsort: 'ts',
  lsortDir: 'desc',
  lpage: 1,
  lpageSize: 25,
  lhover: false,
  auth: {
    token: null,
    authenticated: false,
    mode: 'live' // 'live' | 'demo'
  }
};

/* ---------- Authentication & Credential Store ---------- */
const AUTH_STORAGE_KEY = 'toron_admin_key';

export function getAuthToken() {
  return state.auth.token;
}

export function isAuth() {
  return state.auth.authenticated;
}

export function initAuth() {
  if (typeof sessionStorage !== 'undefined') {
    try {
      const stored = sessionStorage.getItem(AUTH_STORAGE_KEY);
      if (stored) {
        state.auth.token = stored;
        state.auth.authenticated = true;
      }
    } catch (e) {}
  }
}

export function setAuthToken(token) {
  state.auth.token = token || null;
  state.auth.authenticated = !!token;
  state.auth.mode = 'live';
  if (typeof sessionStorage !== 'undefined') {
    try {
      if (token) {
        sessionStorage.setItem(AUTH_STORAGE_KEY, token);
      } else {
        sessionStorage.removeItem(AUTH_STORAGE_KEY);
      }
    } catch (e) {}
  }
}

export function clearAuth() {
  state.auth.token = null;
  state.auth.authenticated = false;
  if (typeof sessionStorage !== 'undefined') {
    try {
      sessionStorage.removeItem(AUTH_STORAGE_KEY);
    } catch (e) {}
  }
}

export function setAuthDemoMode() {
  state.auth.mode = 'demo';
}

export function getAuthHeaders() {
  const headers = {};
  if (state.auth.token) {
    headers['X-Toron-Admin-Key'] = state.auth.token;
    headers['Authorization'] = `Bearer ${state.auth.token}`;
  }
  return headers;
}

// Hydrate auth state on initial module load
initAuth();


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
