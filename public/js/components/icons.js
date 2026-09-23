/**
 * Toron Dashboard - SVG Icon & Tone Badge Helpers
 */
import { esc } from '../utils.js';

export const ICON = (id, c = 'ic') => `<svg class="${c}" aria-hidden="true" focusable="false"><use href="#${id}"/></svg>`;

export const TI = {
  ok: 'i-check',
  warn: 'i-alert',
  err: 'i-x',
  mute: 'i-dash'
};

export const pill = (tone, text, title) => `<span class="pill ${tone}"${title ? ` title="${esc(title)}"` : ''}>${ICON(TI[tone] || 'i-check')}${esc(text)}</span>`;

export const TONE = {
  ok: 'var(--flow-ok)',
  warn: 'var(--flow-warn)',
  err: 'var(--flow-err)'
};

export const toneErr = e => e >= .02 ? 'err' : e >= .012 ? 'warn' : 'ok';
