/**
 * Toron Dashboard - Backend Data Polling Adapter
 */
import { $ } from './utils.js';
import { invalidate } from './state.js';

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

export async function fetchBackendData(onSuccess) {
  try {
    const [statusRes, routesRes, upstreamsRes, incidentsRes, logsRes, bansRes] = await Promise.allSettled([
      fetch('/internal/api/status'),
      fetch('/internal/api/routes'),
      fetch('/internal/api/upstreams/health'),
      fetch('/internal/api/security/incidents'),
      fetch('/internal/api/logs'),
      fetch('/internal/api/security/banned-ips')
    ]);

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
