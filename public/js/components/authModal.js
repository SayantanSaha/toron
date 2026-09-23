/**
 * Toron Dashboard - Admin Authentication Modal Component
 * Intercepts HTTP 401 challenges and manages session credential submission
 */
import { $ } from '../utils.js';
import { state, setAuthToken, setAuthDemoMode, setLive } from '../state.js';
import { fetchBackendData, setOnAuthRequired } from '../api.js';

let modalSuccessCallback = null;

export function isAuthModalOpen() {
  const modal = $('#authModal');
  return modal ? !modal.hidden : false;
}

export function showAuthModal(onSuccess) {
  modalSuccessCallback = onSuccess || null;
  const modal = $('#authModal');
  if (!modal) return;
  modal.hidden = false;

  const keyInput = $('#adminKeyInput');
  if (keyInput) {
    keyInput.value = state.auth.token || '';
    keyInput.focus();
  }
  const alertEl = $('#authAlert');
  if (alertEl) alertEl.style.display = 'none';
}

export function hideAuthModal() {
  const modal = $('#authModal');
  if (modal) modal.hidden = true;
  const alertEl = $('#authAlert');
  if (alertEl) alertEl.style.display = 'none';
}

export function initAuthModal() {
  if (typeof document === 'undefined') return;

  const form = $('#authForm');
  const keyInput = $('#adminKeyInput');
  const alertEl = $('#authAlert');
  const alertMsg = $('#authAlertMsg');
  const maskBtn = $('#toggleKeyMask');
  const demoBtn = $('#demoModeBtn');

  // Toggle mask/reveal
  if (maskBtn && !maskBtn._bound) {
    maskBtn._bound = true;
    maskBtn.addEventListener('click', () => {
      if (!keyInput) return;
      keyInput.type = (keyInput.type === 'password') ? 'text' : 'password';
    });
  }

  // Demo mode button
  if (demoBtn && !demoBtn._bound) {
    demoBtn._bound = true;
    demoBtn.addEventListener('click', () => {
      setAuthDemoMode();
      hideAuthModal();
      if (typeof modalSuccessCallback === 'function') {
        modalSuccessCallback();
      }
    });
  }

  // Submit credentials
  if (form && !form._bound) {
    form._bound = true;
    form.addEventListener('submit', async e => {
      e.preventDefault();
      const token = (keyInput ? keyInput.value : '').trim();
      if (!token) return;

      const submitBtn = $('#authSubmitBtn');
      if (submitBtn) {
        submitBtn.disabled = true;
        submitBtn.textContent = 'Verifying...';
      }
      if (alertEl) alertEl.style.display = 'none';

      try {
        // Direct probe verification against /internal/api/status
        const headers = {
          'X-Toron-Admin-Key': token,
          'Authorization': `Bearer ${token}`
        };
        const probeRes = await fetch('/internal/api/status', {
          headers,
          credentials: 'same-origin'
        });

        if (probeRes.ok) {
          // Authentication succeeded
          setAuthToken(token);
          hideAuthModal();
          setLive(true);
          await fetchBackendData();
          if (typeof modalSuccessCallback === 'function') {
            modalSuccessCallback();
          }
        } else {
          // Server rejected credentials
          let errMsg = 'Invalid Admin Key or Token.';
          try {
            const errData = await probeRes.json();
            if (errData && errData.message) errMsg = errData.message;
          } catch (_) {}
          if (alertMsg) alertMsg.textContent = errMsg;
          if (alertEl) alertEl.style.display = 'block';
        }
      } catch (err) {
        if (alertMsg) alertMsg.textContent = `Verification failed: ${err.message}`;
        if (alertEl) alertEl.style.display = 'block';
      } finally {
        if (submitBtn) {
          submitBtn.disabled = false;
          submitBtn.textContent = 'Authenticate';
        }
      }
    });
  }

  // Register 401 callback in API layer
  setOnAuthRequired(() => {
    showAuthModal();
  });
}
