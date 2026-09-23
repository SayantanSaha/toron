/**
 * Toron Dashboard - Backend Data Polling Adapter & HTTP Security Interceptor
 */
import { $ } from './utils.js';
import { state, invalidate, getAuthHeaders, setLive } from './state.js';

export let rawApiStatus = null;
export let rawApiRoutes = [];
export let rawApiUpstreams = [];
export let rawApiIncidents = [];
export let rawApiLogs = [];
export let rawApiBannedIps = [];

export function setRawApiStatus(val) { rawApiStatus = val; }
export function setRawApiRoutes(val) { rawApiRoutes = val; }
export function setRawApiUpstreams(val) { rawApiUpstreams = val; }
export function setRawApiIncidents(val) { rawApiIncidents = val; }
export function setRawApiLogs(val) { rawApiLogs = val; }
export function setRawApiBannedIps(val) { rawApiBannedIps = val; }

let updateCallback = null;
export function setUpdateCallback(fn) { updateCallback = fn; }

let authRequiredCallback = null;
export function setOnAuthRequired(fn) { authRequiredCallback = fn; }
export function getOnAuthRequired() { return authRequiredCallback; }

/**
 * Executes a fetch request with automatic authentication headers,
 * same-origin credentials for HTTP Basic auth, and HTTP 401 interception.
 */
export async function authenticatedFetch(url, options = {}) {
  const headers = Object.assign({}, getAuthHeaders(), options.headers || {});
  const fetchOpts = Object.assign({}, options, {
    headers,
    credentials: options.credentials || 'same-origin'
  });

  const res = await fetch(url, fetchOpts);
  if (res.status === 401) {
    if (typeof authRequiredCallback === 'function') {
      authRequiredCallback(url, res);
    }
  }
  return res;
}

export async function fetchBackendData(onSuccess) {
  // If operator chose demo mode, skip polling backend
  if (state.auth && state.auth.mode === 'demo') {
    if (typeof onSuccess === 'function') onSuccess();
    return;
  }

  try {
    const [statusRes, routesRes, upstreamsRes, incidentsRes, logsRes, bansRes] = await Promise.allSettled([
      authenticatedFetch('/internal/api/status'),
      authenticatedFetch('/internal/api/routes'),
      authenticatedFetch('/internal/api/upstreams/health'),
      authenticatedFetch('/internal/api/security/incidents'),
      authenticatedFetch('/internal/api/logs'),
      authenticatedFetch('/internal/api/security/banned-ips')
    ]);

    // Check if any request returned 401
    const results = [statusRes, routesRes, upstreamsRes, incidentsRes, logsRes, bansRes];
    const isUnauthorized = results.some(r => r.status === 'fulfilled' && r.value && r.value.status === 401);
    if (isUnauthorized) {
      setLive(false);
      return;
    }

    if (statusRes.status === 'fulfilled' && statusRes.value.ok) {
      rawApiStatus = await statusRes.value.json();
      if (rawApiStatus && rawApiStatus.version) {
        const eb = $('#envBadge');
        if (eb) eb.textContent = `v${rawApiStatus.version}`;
      }
    }
    if (routesRes.status === 'fulfilled' && routesRes.value.ok) {
      const data = await routesRes.value.json();
      rawApiRoutes = data.routes || [];
    }
    if (upstreamsRes.status === 'fulfilled' && upstreamsRes.value.ok) {
      const data = await upstreamsRes.value.json();
      rawApiUpstreams = data.upstreams || [];
    }
    if (incidentsRes.status === 'fulfilled' && incidentsRes.value.ok) {
      const data = await incidentsRes.value.json();
      rawApiIncidents = data.incidents || [];
    }
    if (logsRes.status === 'fulfilled' && logsRes.value.ok) {
      const data = await logsRes.value.json();
      rawApiLogs = data.logs || [];
    }
    if (bansRes.status === 'fulfilled' && bansRes.value.ok) {
      const data = await bansRes.value.json();
      rawApiBannedIps = data.banned_ips || [];
    }
    invalidate();
    if (typeof onSuccess === 'function') {
      onSuccess();
    } else if (typeof updateCallback === 'function') {
      updateCallback();
    }
  } catch (err) {
    console.warn('Backend polling error:', err);
  }
}

