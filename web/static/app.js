import { receiverUI } from './receiver.js';
import {
  LANGUAGES,
  applyDocumentLanguage,
  channelsWord,
  daysWord,
  detectBrowserLanguage,
  entriesWord,
  fmtDate,
  getLanguage,
  getLanguagePref,
  noLogoWord,
  setLanguagePref,
  sourcesWord,
  t,
  trServer,
  when,
} from './i18n.js';

// Dreaminator — tablet UI. Plain ES modules, no build step: the files are
// embedded into the receiver-side binary as they are.

// ---------------------------------------------------------------- helpers

const $ = (sel, root = document) => root.querySelector(sel);

const PROPS = new Set(['value', 'checked', 'selected', 'disabled', 'type']);

function h(tag, props, ...children) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(props || {})) {
    if (v == null || v === false) continue;
    if (k === 'class') el.className = v;
    else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2).toLowerCase(), v);
    else if (PROPS.has(k)) el[k] = v;
    else el.setAttribute(k, v === true ? '' : v);
  }
  for (const c of children.flat(Infinity)) {
    if (c == null || c === false) continue;
    el.append(c instanceof Node ? c : String(c));
  }
  return el;
}

const ICONS = {
  tv: '<rect x="3" y="5" width="18" height="12" rx="2.5"/><path d="M8 21h8"/>',
  list: '<path d="M9 6h11M9 12h11M9 18h11"/><circle cx="4.5" cy="6" r="1" fill="currentColor"/><circle cx="4.5" cy="12" r="1" fill="currentColor"/><circle cx="4.5" cy="18" r="1" fill="currentColor"/>',
  link: '<path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1"/><path d="M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1"/>',
  sliders: '<path d="M4 6h9M17 6h3M4 12h3M11 12h9M4 18h11M19 18h1"/><circle cx="15" cy="6" r="2"/><circle cx="9" cy="12" r="2"/><circle cx="17" cy="18" r="2"/>',
  play: '<path d="M8 5.5v13l11-6.5z" fill="currentColor" stroke="none"/>',
  star: '<path d="M12 3.5l2.6 5.4 5.9.8-4.3 4.1 1 5.8L12 16.9l-5.2 2.7 1-5.8-4.3-4.1 5.9-.8z"/>',
  starFill: '<path d="M12 3.5l2.6 5.4 5.9.8-4.3 4.1 1 5.8L12 16.9l-5.2 2.7 1-5.8-4.3-4.1 5.9-.8z" fill="currentColor"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  trash: '<path d="M4 7h16M9 7V4.5h6V7M6.5 7l1 13h9l1-13"/>',
  edit: '<path d="M4 20h4L19 9l-4-4L4 16z"/><path d="M13.5 6.5l4 4"/>',
  refresh: '<path d="M20 12a8 8 0 1 1-2.4-5.7"/><path d="M20 4v5h-5"/>',
  grip: '<circle cx="9" cy="6" r="1.4" fill="currentColor" stroke="none"/><circle cx="15" cy="6" r="1.4" fill="currentColor" stroke="none"/><circle cx="9" cy="12" r="1.4" fill="currentColor" stroke="none"/><circle cx="15" cy="12" r="1.4" fill="currentColor" stroke="none"/><circle cx="9" cy="18" r="1.4" fill="currentColor" stroke="none"/><circle cx="15" cy="18" r="1.4" fill="currentColor" stroke="none"/>',
  back: '<path d="M15 5l-7 7 7 7"/>',
  chevron: '<path d="M9 5l7 7-7 7"/>',
  search: '<circle cx="11" cy="11" r="6.5"/><path d="M20 20l-4.2-4.2"/>',
  x: '<path d="M6 6l12 12M18 6L6 18"/>',
  check: '<path d="M5 12.5l4.5 4.5L19 7"/>',
  alert: '<circle cx="12" cy="12" r="9"/><path d="M12 7.5v5.5M12 16.5v.01"/>',
  send: '<path d="M4 12l16-8-6 16-2.5-6.5z"/>',
  pulse: '<path d="M3 12h4l2.5-6 4 12 2.5-6H21"/>',
  upload: '<path d="M12 16V4M7 9l5-5 5 5"/><path d="M4 16v3a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-3"/>',
  more: '<circle cx="5" cy="12" r="1.6" fill="currentColor" stroke="none"/><circle cx="12" cy="12" r="1.6" fill="currentColor" stroke="none"/><circle cx="19" cy="12" r="1.6" fill="currentColor" stroke="none"/>',
};

function icon(name) {
  const t = document.createElement('template');
  t.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICONS[name]}</svg>`;
  return t.content.firstChild;
}

const spinner = () => h('span', { class: 'spinner', 'aria-hidden': 'true' });

function plural(n, one, few, many) {
  const m10 = n % 10, m100 = n % 100;
  const word = m10 === 1 && m100 !== 11 ? one : m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14) ? few : many;
  return `${n} ${word}`;
}

const store = {
  get(k) { try { return localStorage.getItem(k); } catch { return null; } },
  set(k, v) { try { localStorage.setItem(k, v); } catch { /* private mode */ } },
};

// ---------------------------------------------------------------- language

let S = null;          // server state
let lastBox = null;
let lastSyncedLang = '';

const currentLang = () => getLanguage(S && S.settings && S.settings.language);
const langLabel = (code) => (LANGUAGES.find((l) => l.code === code) || { label: code }).label;

function paintLanguageSelect(select, compact = false) {
  if (!select) return;
  const pref = getLanguagePref();
  const detected = detectBrowserLanguage() || currentLang();
  const autoLabel = compact
    ? `${t('language_auto_short')} (${langLabel(detected)})`
    : t('language_auto', { lang: langLabel(detected) });
  select.replaceChildren(
    h('option', { value: 'auto', selected: pref === 'auto' }, autoLabel),
    ...LANGUAGES.map((l) => h('option', { value: l.code, selected: pref === l.code }, l.label)),
  );
  select.setAttribute('aria-label', t('ui_language'));
}

async function syncLanguageToServer(force = false) {
  const lang = currentLang();
  if (!force && lang === lastSyncedLang) return;
  lastSyncedLang = lang;
  try {
    await fetch('/api/language', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ language: lang }),
    });
    if (S && S.settings) S.settings.language = lang;
  } catch { /* receiver offline */ }
}

async function onLanguageChange(nextPref) {
  setLanguagePref(nextPref);
  const lang = currentLang();
  applyDocumentLanguage(lang);
  paintLanguageSelect($('#lang-select'), true);
  renderBoxStatus();
  renderChrome();
  syncLanguageToServer(true);
  if (S) await render();
}

// ---------------------------------------------------------------- api

async function sendRequest(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { /* not json */ }
  if (!res.ok) {
    const err = new Error((data && trServer(data.error)) || t('err_http', { status: res.status }));
    err.status = res.status;
    err.json = data != null; // false: no such route on this receiver
    throw err;
  }
  return data;
}

let bouquetWrites = Promise.resolve();
function queueBouquetWrite(write) {
  const saved = bouquetWrites.then(write);
  bouquetWrites = saved.catch(() => {});
  return saved;
}

function request(method, path, body) {
  // Send to TV must include all earlier membership and order edits.
  if (method !== 'GET' && (path === '/api/apply' || path === '/api/bouquet-order' || path === '/api/bouquets' || path.startsWith('/api/bouquets/'))) {
    return queueBouquetWrite(() => sendRequest(method, path, body));
  }
  return sendRequest(method, path, body);
}
// Multipart upload: text fields first, the file last (the receiver streams
// the file straight to disk once it reaches it).
async function upload(path, fields, file) {
  const form = new FormData();
  for (const [k, v] of Object.entries(fields)) form.append(k, v);
  form.append('file', file, file.name);
  const res = await fetch(path, { method: 'POST', body: form });
  const data = await res.json().catch(() => null);
  if (!res.ok) throw new Error((data && trServer(data.error)) || t('err_http', { status: res.status }));
  return data;
}

const api = {
  get: (p) => request('GET', p),
  post: (p, b) => request('POST', p, b || {}),
  put: (p, b) => request('PUT', p, b),
  del: (p) => request('DELETE', p),
  reorderBouquet: (id, ids) => queueBouquetWrite(async () => {
    const state = await sendRequest('GET', '/api/state');
    const bouquet = state.bouquets.find(b => b.id === id);
    if (!bouquet) throw new Error(t('err_list_not_found'));
    const positions = new Map(ids.map((key, index) => [key, index]));
    const rank = c => positions.get(`${c.source}/${c.id}`) ?? ids.length;
    // Reorder current membership so stale rows cannot undo removals or additions.
    const channels = bouquet.channels.map(refOf).sort((a, b) => rank(a) - rank(b));
    return sendRequest('PUT', `/api/bouquets/${id}`, { channels });
  }),
};

// ---------------------------------------------------------------- state

let dragging = false;
// Keep optimistic row order visible until the queued saves finish.
const sortingLists = new Set();
let modalOpen = false;
const ui = { target: store.get('target'), q: '' };

function targetBouquet() {
  if (!S || !S.bouquets.length) return null;
  return S.bouquets.find((b) => b.id === ui.target) || S.bouquets[0];
}
function setTarget(id) { ui.target = id; store.set('target', id); }

const refOf = (ch) => ({ source: ch.source, id: ch.id, name: ch.name });
const sameRef = (a, b) => a.source === b.source && a.id === b.id;
const inBouquet = (b, ch) => !!b && b.channels.some((c) => sameRef(c, ch));

async function refreshState(rerender = true) {
  const wasBusy = busy();
  const next = await api.get('/api/state');
  const changed = JSON.stringify(next) !== JSON.stringify(S);
  S = next;
  if (S.settings && S.settings.language && !lastSyncedLang) {
    lastSyncedLang = S.settings.language;
  }
  renderChrome();
  if (!changed || !rerender || dragging || modalOpen || [...sortingLists].some((list) => list.isConnected)) return;
  const [tab, sub] = route();
  if (tab === 'lists' && sub === 'receiver') return;
  if (tab === 'sources' || tab === 'lists') await render();
  else if (tab === 'channels' && wasBusy && !busy()) render();
  else if (tab === 'settings') {
    const card = $('#epg-card'); if (card) card.replaceWith(epgCard());
    const pcard = $('#picon-card'); if (pcard) pcard.replaceWith(piconCard());
  }
}

const busy = () => !!S && (S.sources.some((s) => s.refreshing) || S.epg.running || (S.picons && S.picons.running));

function pollState() {
  setTimeout(async () => {
    try { await refreshState(); } catch { /* receiver busy or offline */ }
    pollState();
  }, busy() ? 2000 : 8000);
}

function renderBoxStatus() {
  const el = $('#box-status');
  if (!el || lastBox === null) return;
  if (lastBox.offline) {
    el.replaceChildren(h('i', { class: 'dot bad' }), h('span', {}, t('dreaminator_offline')));
  } else if (lastBox.ok) {
    const what = lastBox.standby
      ? t('box_standby')
      : lastBox.current ? t('box_current', { current: lastBox.current }) : t('box_online');
    el.replaceChildren(h('i', { class: 'dot ok' }), h('span', {}, t('box_status', { what })));
  } else {
    el.replaceChildren(h('i', { class: 'dot bad' }), h('span', { title: lastBox.error }, t('box_offline')));
  }
}

async function pollBox() {
  try {
    lastBox = await api.get('/api/box');
  } catch {
    lastBox = { offline: true };
  }
  renderBoxStatus();
  setTimeout(pollBox, 20000);
}

// ---------------------------------------------------------------- ui chrome

let toastTimer;
function toast(msg, kind = '') {
  const el = $('#toast');
  el.textContent = msg;
  el.className = `toast show ${kind}`;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.className = 'toast'; }, kind === 'error' ? 5000 : 2600);
}
const fail = (e) => toast(trServer(e.message || String(e)), 'error');

function renderChrome() {
  const bar = $('#applybar');
  if (!S) return;
  if (S.dirty) {
    const btn = h('button', { class: 'btn btn-good btn-lg', onclick: () => applyNow(btn) }, icon('send'), t('send_to_tv'));
    bar.replaceChildren(h('div', { class: 'applybar' },
      h('div', { class: 'applybar-text' }, h('strong', {}, t('changes_pending')), h('span', {}, t('changes_pending_desc'))),
      btn));
  } else if (S.lastApplyError) {
    const btn = h('button', { class: 'btn btn-lg', onclick: () => applyNow(btn) }, icon('refresh'), t('retry'));
    bar.replaceChildren(h('div', { class: 'applybar error' },
      h('div', { class: 'applybar-text' }, h('strong', {}, t('apply_failed')), h('span', {}, S.lastApplyError)),
      btn));
  } else {
    bar.replaceChildren();
  }
}

async function applyNow(btn) {
  btn.disabled = true;
  btn.replaceChildren(spinner(), t('sending'));
  try {
    await api.post('/api/apply');
    toast(t('apply_done'));
  } catch (e) {
    fail(e);
  }
  await refreshState().catch(() => {});
}

function openModal(title, body, actions, onClose) {
  const previousFocus = document.activeElement;
  modalOpen = true;
  const close = () => {
    overlay.remove();
    modalOpen = false;
    document.removeEventListener('keydown', onKey);
    if (previousFocus?.isConnected) previousFocus.focus({ preventScroll: true });
    if (onClose) onClose();
  };
  const onKey = (e) => {
    if (e.key === 'Escape') close();
    if (e.key !== 'Tab') return;
    const controls = [...overlay.querySelectorAll('button, input, select, textarea, a[href], [tabindex="0"]')]
      .filter((el) => !el.disabled && el.getClientRects().length);
    const first = controls[0], last = controls[controls.length - 1];
    if (e.shiftKey && (document.activeElement === first || !overlay.contains(document.activeElement))) {
      e.preventDefault(); last?.focus();
    } else if (!e.shiftKey && (document.activeElement === last || !overlay.contains(document.activeElement))) {
      e.preventDefault(); first?.focus();
    }
  };
  const overlay = h('div', { class: 'modal-overlay', onclick: (e) => { if (e.target === overlay) close(); } },
    h('div', { class: 'modal', role: 'dialog', 'aria-modal': 'true', 'aria-label': title },
      h('div', { class: 'modal-head' }, h('h2', {}, title),
        h('button', { class: 'icon-btn', 'aria-label': t('close'), onclick: close }, icon('x'))),
      body,
      actions && h('div', { class: 'modal-actions' }, actions)));
  document.addEventListener('keydown', onKey);
  document.body.append(overlay);
  overlay.querySelector('button')?.focus();
  return close;
}

function confirmDialog(title, text, okText = t('delete')) {
  return new Promise((resolve) => {
    let answer = false;
    const close = openModal(title, h('p', {}, text), [
      h('button', { class: 'btn', onclick: () => close() }, t('cancel')),
      h('button', { class: 'btn btn-danger solid', onclick: () => { answer = true; close(); } }, okText),
    ], () => resolve(answer));
  });
}

function promptDialog(title, label, value = '', okText = t('save')) {
  return new Promise((resolve) => {
    let answer = null;
    const input = h('input', { type: 'text', value, autocomplete: 'off' });
    const submit = () => { if (input.value.trim()) { answer = input.value.trim(); close(); } };
    input.addEventListener('keydown', (e) => { if (e.key === 'Enter') submit(); });
    const close = openModal(title, h('div', { class: 'field' }, h('label', {}, label), input), [
      h('button', { class: 'btn', onclick: () => close() }, t('cancel')),
      h('button', { class: 'btn btn-primary', onclick: submit }, okText),
    ], () => resolve(answer));
    setTimeout(() => input.focus(), 50);
  });
}

// Pointer-based drag & drop that works with fingers: grab the ⋮⋮ handle.
function makeSortable(list, onDone) {
  let pending = 0;
  const saveOrder = async (ids, handle) => {
    pending++;
    sortingLists.add(list);
    let restoreFocus = false;
    try { await onDone(ids); } catch (e) { fail(e); }
    finally {
      restoreFocus = !!handle && (document.activeElement === handle || (!list.isConnected && document.activeElement === document.body));
      if (--pending === 0) {
        sortingLists.delete(list);
        if (list.isConnected && !dragging && !modalOpen) await render();
      }
    }
    return restoreFocus;
  };
  list.addEventListener('keydown', async (e) => {
    const handle = e.target.closest('.grip');
    const item = handle && handle.closest('.sortable-item');
    if (!item || item.parentElement !== list || !['ArrowUp', 'ArrowDown'].includes(e.key)) return;
    closeChannelMenu();
    e.preventDefault();
    const sibling = e.key === 'ArrowUp' ? item.previousElementSibling : item.nextElementSibling;
    if (!sibling) return;
    list.insertBefore(item, e.key === 'ArrowUp' ? sibling : sibling.nextElementSibling);
    handle.focus({ preventScroll: true });
    const path = location.hash;
    const restoreFocus = await saveOrder([...list.children].map((el) => el.dataset.id), handle);
    if (!restoreFocus || location.hash !== path) return;
    const moved = [...document.querySelectorAll('.sortable-item')].find((el) => el.dataset.id === item.dataset.id);
    moved?.querySelector('.grip')?.focus();
  });
  list.addEventListener('pointerdown', (e) => {
    const handle = e.target.closest('.grip');
    const item = handle && handle.closest('.sortable-item');
    if (!item || item.parentElement !== list) return;
    closeChannelMenu();
    e.preventDefault();
    dragging = true;
    const before = [...list.children].map((el) => el.dataset.id).join('|');
    const startY = e.clientY + window.scrollY;
    const startTop = item.offsetTop;
    item.classList.add('dragging');
    handle.setPointerCapture(e.pointerId);

    const move = (ev) => {
      const y = ev.clientY;
      let prev = item.previousElementSibling;
      while (prev && y < prev.getBoundingClientRect().top + prev.offsetHeight / 2) {
        list.insertBefore(item, prev);
        prev = item.previousElementSibling;
      }
      let next = item.nextElementSibling;
      while (next && y > next.getBoundingClientRect().top + next.offsetHeight / 2) {
        list.insertBefore(next, item);
        next = item.nextElementSibling;
      }
      if (y < 110) window.scrollBy(0, -14);
      else if (y > window.innerHeight - 170) window.scrollBy(0, 14);
      item.style.transform = `translateY(${startTop + (ev.clientY + window.scrollY - startY) - item.offsetTop}px)`;
    };
    const up = () => {
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', up);
      handle.removeEventListener('pointercancel', up);
      item.classList.remove('dragging');
      item.style.transform = '';
      dragging = false;
      const ids = [...list.children].map((el) => el.dataset.id);
      if (ids.join('|') !== before) saveOrder(ids);
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', up);
    handle.addEventListener('pointercancel', up);
  });
}

const grip = () => h('button', { type: 'button', class: 'icon-btn grip', 'aria-label': t('reorder_aria'), title: t('reorder_title') }, icon('grip'));

function logo(ch) {
  const initials = (ch.name || '?').replace(/[^\p{L}\p{N} ]/gu, ' ').split(/\s+/).filter(Boolean)
    .slice(0, 2).map((w) => w[0]).join('').toUpperCase() || '•';
  const fallback = h('div', { class: 'logo logo-fallback', 'aria-hidden': 'true' }, initials);
  if (!ch.logo) return fallback;
  const img = h('img', { class: 'logo', src: ch.logo, alt: '', loading: 'lazy', referrerpolicy: 'no-referrer' });
  img.addEventListener('error', () => img.replaceWith(fallback));
  return img;
}

function emptyState(title, text, action) {
  return h('div', { class: 'empty' }, h('h2', {}, title), h('p', {}, text), action);
}

// ---------------------------------------------------------------- routing

function route() {
  const parts = location.hash.replace(/^#\/?/, '').split('/').filter(Boolean).map(decodeURIComponent);
  return parts.length ? parts : ['channels'];
}
function go(...parts) { location.hash = '#/' + parts.map(encodeURIComponent).join('/'); }

let renderToken = 0;
async function render() {
  const token = ++renderToken;
  const [tab, ...rest] = route();
  document.querySelectorAll('.nav a').forEach((a) => {
    const active = a.dataset.tab === tab;
    a.classList.toggle('active', active);
    if (active) a.setAttribute('aria-current', 'page');
    else a.removeAttribute('aria-current');
  });
  let el;
  try {
    switch (tab) {
      case 'lists': el = rest[0] === 'receiver' ? await receiver.detail(rest[1]) : rest[0] ? viewBouquet(rest[0]) : await viewBouquets(); break;
      case 'sources': el = viewSources(); break;
      case 'settings': el = rest[0] === 'tmdb' ? await viewTMDBMatch() : viewSettings(); break;
      default: el = await viewChannels(rest[0], rest[1]);
    }
  } catch (e) {
    el = emptyState(t('something_went_wrong'), trServer(e.message), h('button', { class: 'btn', onclick: render }, t('retry')));
  }
  if (token !== renderToken) return;
  $('#view').replaceChildren(el);
  if (tab === 'sources' && rest[0] === 'new') {
    history.replaceState(null, '', '#/sources');
    sourceForm();
  }
}

// ---------------------------------------------------------------- channels

async function viewChannels(sourceId, group) {
  if (!S.sources.length) {
    return emptyState(t('no_sources_title'),
      t('no_sources_text'),
      h('button', { class: 'btn btn-primary btn-lg', onclick: () => go('sources', 'new') }, icon('plus'), t('add_source')));
  }
  const src = S.sources.find((s) => s.id === sourceId) || S.sources[0];
  const content = h('div', {});
  let token = 0;
  const update = async () => {
    const my = ++token;
    let el;
    try {
      el = ui.q.trim() ? await searchResults(ui.q.trim()) : group ? await groupChannels(src, group) : await groupGrid(src);
    } catch (e) {
      el = emptyState(t('failed_load_channels'), trServer(e.message));
    }
    if (my === token) content.replaceChildren(el);
  };

  const input = h('input', { type: 'search', placeholder: t('search_channel'), value: ui.q, 'aria-label': t('search_channel') });
  let timer;
  input.addEventListener('input', () => {
    clearTimeout(timer);
    timer = setTimeout(() => { ui.q = input.value; update(); }, 250);
  });

  const chips = S.sources.length > 1 && !ui.q
    ? h('div', { class: 'chips' }, S.sources.map((s) =>
      h('button', { class: `chip ${s.id === src.id ? 'on' : ''}`, onclick: () => go('channels', s.id) }, s.name)))
    : null;

  await update();
  return h('section', {},
    h('div', { class: 'toolbar' }, targetPicker(), h('div', { class: 'search' }, icon('search'), input)),
    chips,
    content);
}

function targetPicker() {
  const current = targetBouquet();
  const select = h('select', { 'aria-label': t('target_picker_aria') },
    S.bouquets.map((b) => h('option', { value: b.id, selected: current && b.id === current.id }, `${trServer(b.name)} · ${b.channels.length}`)),
    h('option', { value: '__new' }, t('new_list_option')));
  select.addEventListener('change', async () => {
    if (select.value === '__new') {
      const b = await createBouquetFlow();
      if (b) setTarget(b.id);
    } else {
      setTarget(select.value);
    }
    render();
  });
  return h('label', { class: 'target' }, h('span', { class: 'target-label' }, t('add_to')), select);
}

function updatePickerCounts() {
  const select = $('.target select');
  if (!select) return;
  for (const b of S.bouquets) {
    const opt = [...select.options].find((o) => o.value === b.id);
    if (opt) opt.textContent = `${trServer(b.name)} · ${b.channels.length}`;
  }
}

async function groupGrid(src) {
  const groups = await api.get(`/api/sources/${src.id}/groups`);
  if (!groups.length) {
    if (src.refreshing) return h('div', { class: 'loading' }, spinner(), ' ', t('loading_channels'));
    return emptyState(t('no_channels_title'), src.error || t('playlist_not_loaded'),
      h('button', { class: 'btn', onclick: () => go('sources') }, t('to_playlists')));
  }
  return h('div', { class: 'groups' }, groups.map((g) =>
    h('button', { class: 'group', onclick: () => go('channels', src.id, g.name) },
      h('span', {}, trServer(g.name)), h('span', { class: 'count' }, g.count))));
}

async function groupChannels(src, group) {
  const page = await api.get(`/api/channels?source=${encodeURIComponent(src.id)}&group=${encodeURIComponent(group)}&limit=2000`);
  const target = targetBouquet();
  const groupTitle = trServer(group);
  const addAll = async () => {
    let b = targetBouquet();
    if (!b) b = await createBouquetFlow();
    if (!b) return;
    try {
      await api.post(`/api/bouquets/${b.id}/channels`, { add: page.items.map(refOf) });
      await refreshState(false);
      toast(t('group_added_to', { name: trServer(b.name) }));
      render();
    } catch (e) { fail(e); }
  };
  const asList = async () => {
    try {
      const b = await api.post('/api/bouquets', { name: groupTitle, channels: page.items.map(refOf) });
      setTarget(b.id);
      await refreshState(false);
      toast(t('list_created', { name: groupTitle }));
      render();
    } catch (e) { fail(e); }
  };
  return h('div', {},
    h('div', { class: 'view-head' },
      h('button', { class: 'icon-btn back', 'aria-label': t('all_groups'), onclick: () => go('channels', src.id) }, icon('back')),
      h('h1', {}, groupTitle), h('span', { class: 'count' }, page.total)),
    h('div', { class: 'group-actions' },
      h('button', { class: 'btn btn-sm', onclick: addAll }, icon('plus'), target ? t('all_to_target', { name: trServer(target.name) }) : t('add_all')),
      h('button', { class: 'btn btn-sm', onclick: asList }, icon('list'), t('make_separate_list'))),
    channelList(page, false));
}

async function searchResults(q) {
  const page = await api.get(`/api/channels?q=${encodeURIComponent(q)}&limit=300`);
  if (!page.items.length) return emptyState(t('nothing_found'), t('no_matching_channels', { q }));
  return channelList(page, true);
}

function channelList(page, showGroup) {
  const srcName = (id) => (S.sources.find((s) => s.id === id) || {}).name || '';
  return h('div', {},
    h('div', { class: 'list' }, page.items.map((ch) => channelRow(ch, showGroup && `${trServer(ch.group)}${S.sources.length > 1 ? ' · ' + srcName(ch.source) : ''}`))),
    page.total > page.items.length && h('p', { class: 'more' }, t('showing_first_of', { shown: page.items.length, total: page.total })));
}

function starButton(ch) {
  const btn = h('button', { class: 'icon-btn star' });
  const paint = () => {
    const on = inBouquet(targetBouquet(), ch);
    btn.classList.toggle('on', on);
    btn.setAttribute('aria-label', t(on ? 'remove_from_list_aria' : 'add_to_list_aria', { name: ch.name }));
    btn.setAttribute('aria-pressed', String(on));
    btn.replaceChildren(icon(on ? 'starFill' : 'star'));
  };
  btn.addEventListener('click', async () => {
    let b = targetBouquet();
    if (!b) b = await createBouquetFlow();
    if (!b) return;
    const had = inBouquet(b, ch);
    try {
      await api.post(`/api/bouquets/${b.id}/channels`, had ? { remove: [refOf(ch)] } : { add: [refOf(ch)] });
      await refreshState(false);
      paint();
      updatePickerCounts();
      toast(t(had ? 'removed_from_list' : 'added_to_list', { name: trServer(b.name) }));
    } catch (e) { fail(e); }
  });
  paint();
  return btn;
}

function channelRow(ch, sub) {
  return h('div', { class: 'row channel-row' },
    logo(ch),
    h('div', { class: 'row-main' }, h('div', { class: 'row-title' }, ch.name), sub && h('div', { class: 'row-sub' }, sub)),
    channelActions(ch));
}

const hasChannelPopovers = 'showPopover' in HTMLElement.prototype;
const channelPhone = matchMedia('(max-width: 640px)');
let channelMenuID = 0;

function closeChannelMenu() {
  if (hasChannelPopovers) $('.channel-secondary:popover-open')?.hidePopover();
}

function channelActions(ch, remove) {
  const compact = hasChannelPopovers && channelPhone.matches;
  const fullLabel = (text) => h('span', { class: 'channel-action-full' }, text);
  const menu = h('div', { class: 'channel-secondary',
    popover: compact ? 'auto' : null, id: compact ? `channel-menu-${++channelMenuID}` : null,
    role: compact ? 'group' : null, 'aria-label': compact ? t('actions_aria', { name: ch.name }) : null },
    !ch.missing && h('button', { class: 'btn btn-sm btn-ghost', onclick: () => zap(ch), 'aria-label': t('watch_on_tv_aria', { name: ch.name }) },
      icon('play'), h('span', { class: 'channel-action-short' }, t('on_tv_short')), fullLabel(t('watch_on_tv'))),
    !ch.missing && h('button', { class: 'icon-btn', title: t('probe_stream'), 'aria-label': t('probe_stream_aria', { name: ch.name }), onclick: () => probeDialog(ch) },
      icon('pulse'), fullLabel(t('probe_stream'))),
    remove && h('button', { class: 'icon-btn danger', 'aria-label': t('remove_aria', { name: ch.name }), onclick: remove }, icon('x'), fullLabel(t('remove_from_list'))));
  const more = compact && h('button', { class: 'icon-btn channel-menu-toggle', type: 'button',
    popovertarget: menu.id, 'aria-label': t('actions_aria', { name: ch.name }), 'aria-expanded': 'false', onclick: (e) => {
      e.preventDefault();
      if (menu.matches(':popover-open')) { menu.hidePopover(); return; }
      menu.showPopover();
      const rect = more.getBoundingClientRect();
      const ceiling = $('.topbar').getBoundingClientRect().bottom + 8;
      const floor = Math.min(innerHeight, $('.nav').getBoundingClientRect().top,
        $('#applybar').childElementCount ? $('#applybar').getBoundingClientRect().top : innerHeight);
      menu.style.maxHeight = `${Math.max(48, floor - ceiling - 8)}px`;
      menu.style.left = `${Math.max(12, Math.min(rect.right - menu.offsetWidth, innerWidth - menu.offsetWidth - 12))}px`;
      menu.style.top = `${Math.max(ceiling, Math.min(rect.bottom + 4, floor - menu.offsetHeight - 8))}px`;
      menu.querySelector('button')?.focus({ preventScroll: true });
    } }, icon('more'));
  if (compact) {
    menu.addEventListener('beforetoggle', (e) => {
      if (e.newState === 'closed' && menu.contains(document.activeElement)) more.focus({ preventScroll: true });
    });
    menu.addEventListener('toggle', () => more.setAttribute('aria-expanded', String(menu.matches(':popover-open'))));
    menu.addEventListener('click', (e) => { if (e.target.closest('button')) menu.hidePopover(); }, true);
  }
  return h('div', { class: 'row-actions' }, menu, !remove && starButton(ch), more);
}

async function probeDialog(ch) {
  const body = h('div', {},
    h('div', { class: 'loading' }, spinner(), ' ', t('checking_channel', { name: ch.name })),
    h('p', { class: 'hint' }, t('probe_hint')));
  const close = openModal(t('probe_title'), body, [h('button', { class: 'btn', onclick: () => close() }, t('close'))]);
  try {
    const r = await api.post('/api/probe', { source: ch.source, id: ch.id });
    body.replaceChildren(probeView(ch, r));
  } catch (e) {
    body.replaceChildren(h('div', { class: 'form-error' }, trServer(e.message)));
  }
}

function probeView(ch, r) {
  const row = (label, value) => value && h('div', { class: 'kv' }, h('span', { class: 'kv-label' }, label), h('span', {}, value));
  return h('div', {},
    r.ok
      ? h('div', { class: 'status-line ok big' }, icon('check'), t('probe_ok', { name: ch.name }))
      : h('div', { class: 'form-error' }, trServer(r.error) || t('probe_fail')),
    h('div', { class: 'kv-list' },
      row(t('probe_ttfb'), r.firstByteMs ? t('seconds_short', { val: (r.firstByteMs / 1000).toFixed(1) }) : ''),
      row(t('probe_format'), r.format),
      row(t('probe_video'), r.video),
      row(t('probe_audio'), (r.audio || []).join(', ')),
      row(t('probe_speed'), r.mbps ? t('mbps_unit', { val: r.mbps.toFixed(1) }) : ''),
      row(t('probe_server'), r.host)),
    (r.warnings || []).map((w) => h('div', { class: 'warn-box' }, icon('alert'), trServer(w))),
    r.ok && !(r.warnings || []).length && h('p', { class: 'hint', style: 'margin-top:12px' }, t('probe_all_good')));
}

async function zap(ch) {
  toast(t('zapping_channel', { name: ch.name }));
  try {
    await api.post('/api/zap', { source: ch.source, id: ch.id });
  } catch (e) { fail(e); }
}

// ---------------------------------------------------------------- bouquets

async function createBouquetFlow() {
  const name = await promptDialog(t('new_list'), t('new_list_prompt'), '', t('create'));
  if (!name) return null;
  try {
    const b = await api.post('/api/bouquets', { name, channels: [] });
    await refreshState(false);
    toast(t('list_created_named', { name }));
    return S.bouquets.find((x) => x.id === b.id) || null;
  } catch (e) { fail(e); return null; }
}

async function renameBouquet(b) {
  const name = await promptDialog(t('rename_list'), t('new_name'), trServer(b.name));
  if (!name || name === b.name) return;
  try { await api.put(`/api/bouquets/${b.id}`, { name }); await refreshState(); } catch (e) { fail(e); }
}

// Resolves to true once the list is deleted.
async function deleteBouquet(b) {
  if (!await confirmDialog(t('delete_list_confirm_title'), t('delete_list_confirm_text', { name: trServer(b.name) }))) return false;
  try { await api.del(`/api/bouquets/${b.id}`); await refreshState(false); return true; } catch (e) { fail(e); return false; }
}

const receiver = receiverUI({ h, api, icon, grip, makeSortable, go, render, toast, fail, promptDialog, confirmDialog, emptyState, plural, t, entriesWord });

async function viewBouquets() {
  const native = await receiver.list();
  const list = h('div', { class: 'cards' }, S.bouquets.map((b) => {
    const missing = b.channels.filter((c) => c.missing).length;
    const bName = trServer(b.name);
    return h('div', { class: 'card list-card sortable-item', 'data-id': b.id },
      grip(),
      h('button', { class: 'list-open', onclick: () => go('lists', b.id) },
        h('div', { class: 'row-main' },
          h('div', { class: 'row-title' }, bName),
          h('div', { class: 'row-sub' }, b.channels.length ? channelsWord(b.channels.length) : t('empty_list_hidden'),
            missing ? t('missing_count_suffix', { n: missing }) : '')),
        icon('chevron')),
      h('button', { class: 'icon-btn', 'aria-label': t('rename_aria', { name: bName }), onclick: () => renameBouquet(b) }, icon('edit')),
      h('button', { class: 'icon-btn danger', 'aria-label': t('delete_aria', { name: bName }), onclick: async () => { if (await deleteBouquet(b)) render(); } }, icon('trash')));
  }));
  makeSortable(list, async (ids) => {
    try { await api.put('/api/bouquet-order', { ids }); } catch (e) { fail(e); }
    await refreshState().catch(() => {});
  });
  const create = async () => { const b = await createBouquetFlow(); if (b) go('lists', b.id); };
  return h('section', {},
    h('div', { class: 'view-head' }, h('h1', {}, t('nav_lists')),
      h('div', { class: 'actions' },
        h('button', { class: 'btn', onclick: () => native.scrollIntoView({ behavior: 'smooth', block: 'start' }) }, icon('tv'), t('receiver_bouquets')),
        h('button', { class: 'btn', onclick: create }, icon('plus'), t('new_list'))),
    ),
    h('h2', {}, 'Dreaminator'),
    h('p', { class: 'hint' }, t('lists_hint')),
    S.bouquets.length ? list : emptyState(t('no_lists_title'), t('no_lists_text')),
    native);
}

function viewBouquet(id) {
  const b = S.bouquets.find((x) => x.id === id);
  if (!b) return emptyState(t('list_not_found'), t('list_maybe_deleted'), h('button', { class: 'btn', onclick: () => go('lists') }, t('to_lists')));

  const pick = () => { setTarget(b.id); go('channels'); };
  const removeChannel = async (c) => {
    try { await api.post(`/api/bouquets/${b.id}/channels`, { remove: [c] }); await refreshState(); } catch (e) { fail(e); }
  };

  const key = (c) => `${c.source}/${c.id}`;
  const list = h('div', { class: 'list' }, b.channels.map((c) =>
    h('div', { class: `row channel-row sortable-item ${c.missing ? 'missing' : ''}`, 'data-id': key(c) },
      grip(),
      logo(c),
      h('div', { class: 'row-main' }, h('div', { class: 'row-title' }, c.name),
        c.missing && h('div', { class: 'row-sub' }, t('not_in_playlist_skip'))),
      channelActions(c, () => removeChannel(c)))));
  makeSortable(list, async (ids) => {
    try { await api.reorderBouquet(b.id, ids); } catch (e) { fail(e); }
    await refreshState().catch(() => {});
  });

  return h('section', {},
    h('div', { class: 'view-head' },
      h('button', { class: 'icon-btn back', 'aria-label': t('all_lists'), onclick: () => go('lists') }, icon('back')),
      h('h1', {}, trServer(b.name)),
      h('button', { class: 'icon-btn', 'aria-label': t('rename'), onclick: () => renameBouquet(b) }, icon('edit')),
      h('button', { class: 'icon-btn danger', 'aria-label': t('delete_list'), onclick: async () => { if (await deleteBouquet(b)) go('lists'); } }, icon('trash'))),
    b.channels.length
      ? [h('p', { class: 'hint' }, t('list_channels_hint', { channels: channelsWord(b.channels.length) })), list,
        h('div', { class: 'actions' }, h('button', { class: 'btn', onclick: pick }, icon('plus'), t('add_channels')))]
      : emptyState(t('list_empty_title'), t('list_empty_text'),
        h('button', { class: 'btn btn-primary btn-lg', onclick: pick }, icon('plus'), t('pick_channels'))));
}

// ---------------------------------------------------------------- sources

// Guides the playlist or provider advertises; detectedEpgUrl is the v0.1 field.
const detectedGuides = (s) => s.detectedEpgUrls || (s.detectedEpgUrl ? [s.detectedEpgUrl] : []);

// Where a source's guide comes from: the user's own links win over the ones
// the playlist or provider advertises.
function epgLine(s) {
  if (s.type === 'stalker' && !s.epgUrl) return t('epg_from_portal');
  const own = s.epgUrl ? s.epgUrl.split(/[,\s]+/).filter(Boolean).length : 0;
  const n = own || detectedGuides(s).length;
  const count = n > 1 ? t('epg_sources_suffix', { sources: sourcesWord(n) }) : '';
  if (own) return t('epg_manual') + count;
  if (n) return (s.type === 'xtream' ? t('epg_xtream') : t('epg_playlist')) + count;
  if (s.updatedAt && s.type !== 'xtream') return t('epg_missing_hint');
  return t('epg_not_set');
}

const sourceTypeLabel = (type) => ({
  m3u: t('src_type_m3u'),
  file: t('src_type_file'),
  xtream: 'Xtream',
  stalker: 'Stalker',
}[type] || type);

function viewSources() {
  const cards = S.sources.map((s) => {
    let status;
    if (s.refreshing) status = h('div', { class: 'status-line' }, spinner(), t('loading_channels'));
    else if (s.error) status = h('div', { class: 'status-line error' }, icon('alert'), trServer(s.error));
    else if (s.updatedAt) status = h('div', { class: 'status-line ok' }, icon('check'),
      `${channelsWord(s.channelCount)} · ${s.type === 'file' ? t('source_uploaded') : t('source_updated')} ${when(s.updatedAt)}`);
    const isFile = s.type === 'file';
    const refresh = async (btn) => {
      btn.disabled = true;
      try { await api.post(`/api/sources/${s.id}/refresh`); toast(t('source_refreshed', { name: s.name })); } catch (e) { fail(e); }
      refreshState().catch(() => {});
    };
    const remove = async () => {
      if (!await confirmDialog(t('delete_source_confirm_title'), t('delete_source_confirm_text', { name: s.name }))) return;
      try { await api.del(`/api/sources/${s.id}`); await refreshState(); } catch (e) { fail(e); }
    };
    const refreshBtn = h('button', { class: 'btn btn-sm', disabled: s.refreshing }, icon('refresh'), t('refresh'));
    refreshBtn.addEventListener('click', () => refresh(refreshBtn));
    const checkBtn = h('button', { class: 'btn btn-sm' }, icon('pulse'), t('check'));
    checkBtn.addEventListener('click', async () => {
      checkBtn.disabled = true;
      checkBtn.replaceChildren(spinner(), t('checking'));
      try {
        const acc = await api.post(`/api/sources/${s.id}/check`);
        if (acc && acc.error) fail(new Error(acc.error));
        else toast(t('source_checked', { name: s.name }));
      } catch (e) { fail(e); }
      refreshState().catch(() => {});
    });
    return h('div', { class: 'card' },
      h('h2', {}, s.name, ' ', h('span', { class: 'badge' }, sourceTypeLabel(s.type))),
      status,
      h('div', { class: 'status-line' }, epgLine(s)),
      accountLine(s.account),
      (s.channelCount > 0 || s.type === 'stalker') && h('p', { class: 'hint' }, s.type === 'stalker' ? t('stalker_source_hint') : t('source_hint')),
      isFile && h('div', { class: 'status-line' }, t('file_source_hint')),
      h('div', { class: 'actions' },
        s.channelCount > 0 && h('button', { class: 'btn btn-sm btn-primary', onclick: () => go('channels', s.id) }, icon('list'), t('choose_groups_channels')),
        isFile
          ? h('button', { class: 'btn btn-sm', onclick: () => sourceForm(s) }, icon('upload'), t('upload_new_file'))
          : refreshBtn,
        checkBtn,
        h('button', { class: 'btn btn-sm', onclick: () => sourceForm(s) }, icon('edit'), t('edit')),
        h('button', { class: 'btn btn-sm btn-danger', onclick: remove }, icon('trash'), t('delete'))));
  });
  return h('section', {},
    h('div', { class: 'view-head' }, h('h1', {}, t('nav_sources')),
      h('button', { class: 'btn btn-primary', onclick: () => sourceForm() }, icon('plus'), t('add'))),
    S.sources.length
      ? h('div', { class: 'cards' }, cards)
      : emptyState(t('no_playlists_title'), t('no_playlists_text'),
        h('button', { class: 'btn btn-primary btn-lg', onclick: () => sourceForm() }, icon('plus'), t('add_source'))));
}

// One line about the subscription: status, until when, plan, connections.
function accountLine(a) {
  if (!a) return null;
  if (a.error) return h('div', { class: 'status-line error' }, icon('alert'), t('account_check_err', { when: when(a.checkedAt), err: trServer(a.error) }));
  if (a.note) return h('div', { class: 'status-line ok' }, icon('check'), t('account_note_checked', { note: trServer(a.note), when: when(a.checkedAt) }));
  let kind = 'ok';
  const parts = [a.active === false ? t('sub_inactive_status', { status: trServer(a.status) || t('sub_inactive') }) : t('sub_active')];
  if (a.active === false) kind = 'error';
  if (a.unlimited) parts.push(t('sub_unlimited'));
  else if (a.expires) {
    const days = Math.floor((new Date(a.expires) - Date.now()) / 86400000);
    if (days < 0) { kind = 'error'; parts[0] = t('sub_expired', { date: fmtDate(a.expires) }); }
    else {
      parts.push(t('sub_until', { date: fmtDate(a.expires) }));
      if (days <= 7) { kind = kind === 'error' ? kind : 'warn'; parts.push(t('sub_days_left', { days: daysWord(days) })); }
    }
  } else if (a.rawExpiry) parts.push(t('sub_until', { date: trServer(a.rawExpiry) }));
  if (a.plan) parts.push(t('sub_plan', { plan: a.plan }));
  if (a.maxConnections) parts.push(t('sub_connections', { used: a.connections || 0, max: a.maxConnections }));
  if (a.trial) parts.push(t('sub_trial'));
  return h('div', { class: `status-line ${kind}` }, icon(kind === 'ok' ? 'check' : 'alert'), parts.join(' · '));
}

// Recognises Xtream "get.php?username=…&password=…" links pasted as M3U.
function parseXtreamLink(value) {
  try {
    const u = new URL(value.trim());
    const user = u.searchParams.get('username');
    const pass = u.searchParams.get('password');
    if (!user || !pass || !/\/(get|player_api)\.php$/.test(u.pathname)) return null;
    return { server: u.origin, username: user, password: pass };
  } catch { return null; }
}

function sourceForm(existing) {
  let type = existing ? existing.type : 'm3u';
  // An uploaded playlist cannot turn into a link (and vice versa): there
  // would be nothing to read.
  const types = !existing ? ['m3u', 'file', 'xtream', 'stalker'] : existing.type === 'file' ? ['file'] : ['m3u', 'xtream', 'stalker'];
  const val = (key, onlyType) => (existing && (!onlyType || existing.type === onlyType) ? existing[key] || '' : '');
  let file = null;
  const f = {
    name: h('input', { type: 'text', value: existing ? existing.name : '', placeholder: t('source_name_placeholder') }),
    url: h('input', { type: 'url', value: existing ? existing.url || '' : '', placeholder: 'http://…/playlist.m3u', inputmode: 'url', autocomplete: 'off' }),
    server: h('input', { type: 'url', value: val('server', 'xtream'), placeholder: 'http://server:8080', inputmode: 'url', autocomplete: 'off' }),
    username: h('input', { type: 'text', value: val('username', 'xtream'), autocomplete: 'off', autocapitalize: 'off' }),
    password: h('input', { type: 'text', value: val('password', 'xtream'), autocomplete: 'off', autocapitalize: 'off' }),
    portal: h('input', { type: 'url', value: val('server', 'stalker'), placeholder: 'http://portal.example/stalker_portal/c/', inputmode: 'url', autocomplete: 'off' }),
    mac: h('input', { type: 'text', value: val('mac', 'stalker'), placeholder: '00:1A:79:XX:XX:XX', autocomplete: 'off', autocapitalize: 'characters', spellcheck: 'false' }),
    slogin: h('input', { type: 'text', value: val('username', 'stalker'), autocomplete: 'off', autocapitalize: 'off' }),
    spassword: h('input', { type: 'text', value: val('password', 'stalker'), autocomplete: 'off', autocapitalize: 'off' }),
    serial: h('input', { type: 'text', value: val('serial', 'stalker'), autocomplete: 'off', autocapitalize: 'characters' }),
    deviceId: h('input', { type: 'text', value: val('deviceId', 'stalker'), autocomplete: 'off', autocapitalize: 'characters' }),
    deviceId2: h('input', { type: 'text', value: val('deviceId2', 'stalker'), autocomplete: 'off', autocapitalize: 'characters' }),
    epgUrl: h('input', { type: 'text', value: existing ? existing.epgUrl || '' : '', placeholder: 'http://…/epg.xml.gz', inputmode: 'url', autocomplete: 'off', autocapitalize: 'off' }),
  };
  const errorBox = h('div', {});
  const suggestBox = h('div', {});

  const m3uFields = h('div', {},
    h('div', { class: 'field' }, h('label', {}, t('source_url_label')), f.url, suggestBox));
  const fileInput = h('input', { type: 'file', class: 'visually-hidden' });
  const fileLabel = h('span', {});
  const paintFile = () => fileLabel.replaceChildren(file
    ? h('span', {}, h('strong', {}, file.name), ` · ${file.size < 1 << 20 ? `${Math.max(1, Math.round(file.size / 1024))} ${t('kb_unit')}` : `${(file.size / (1 << 20)).toFixed(1)} ${t('mb_unit')}`}`)
    : (existing ? t('pick_new_file_opt') : t('pick_m3u_file')));
  paintFile();
  fileInput.addEventListener('change', () => {
    file = fileInput.files[0] || null;
    paintFile();
    paintEpg();
    if (file && !f.name.value.trim()) f.name.value = file.name.replace(/\.[^.]+$/, '');
  });
  const fileFields = h('div', {},
    h('div', { class: 'field' }, h('span', { class: 'label' }, t('playlist_file_label')),
      h('label', { class: 'filepick' }, icon('upload'), fileLabel, fileInput),
      h('p', { class: 'hint' }, t('file_upload_hint'))));
  const xtreamFields = h('div', {},
    h('p', { class: 'hint' }, t('xtream_hint')),
    h('div', { class: 'field' }, h('label', {}, t('server_url')), f.server),
    h('div', { class: 'field-row' },
      h('div', { class: 'field' }, h('label', {}, t('username')), f.username),
      h('div', { class: 'field' }, h('label', {}, t('password')), f.password)));

  const advancedOpen = !!(val('username', 'stalker') || val('serial', 'stalker') || val('deviceId', 'stalker'));
  const stalkerFields = h('div', {},
    h('p', { class: 'hint' }, t('stalker_hint')),
    h('div', { class: 'field' }, h('label', {}, t('portal_url')), f.portal,
      h('p', { class: 'hint' }, t('portal_url_hint'))),
    h('div', { class: 'field' }, h('label', {}, t('mac_address')), f.mac,
      h('p', { class: 'hint' }, t('mac_hint'))),
    h('details', { class: 'howto', open: advancedOpen },
      h('summary', {}, t('advanced_provider_fields')),
      h('div', { class: 'field-row', style: 'margin-top:10px' },
        h('div', { class: 'field' }, h('label', {}, t('username')), f.slogin),
        h('div', { class: 'field' }, h('label', {}, t('password')), f.spassword)),
      h('div', { class: 'field' }, h('label', {}, t('serial_number')), f.serial),
      h('div', { class: 'field-row' },
        h('div', { class: 'field' }, h('label', {}, t('device_id')), f.deviceId),
        h('div', { class: 'field' }, h('label', {}, t('device_id2')), f.deviceId2)),
      h('p', { class: 'hint' }, t('advanced_hint'))));

  const epgHint = h('p', { class: 'hint' });
  const paintEpg = () => {
    // The playlist's own guides only stand for the playlist they came from.
    const same = existing && existing.type === type && (type === 'file' ? !file : type === 'm3u' && f.url.value.trim() === existing.url);
    const found = same ? detectedGuides(existing) : [];
    f.epgUrl.placeholder = found.length ? found.join(', ') : 'http://…/epg.xml.gz';
    epgHint.textContent = type === 'xtream'
      ? t('epg_hint_xtream')
      : type === 'stalker'
      ? t('epg_hint_stalker')
      : !found.length
      ? t('epg_hint_none')
      : f.epgUrl.value.trim()
      ? t('epg_hint_override')
      : type === 'file'
      ? t(found.length > 1 ? 'epg_hint_detected_file_many' : 'epg_hint_detected_file_one')
      : t(found.length > 1 ? 'epg_hint_detected_m3u_many' : 'epg_hint_detected_m3u_one');
  };
  f.epgUrl.addEventListener('input', paintEpg);
  const seg = h('div', { class: 'segmented', role: 'tablist' });
  const paint = () => {
    seg.hidden = types.length < 2;
    seg.replaceChildren(...types.map((tKey) =>
      h('button', { class: type === tKey ? 'on' : '', type: 'button', onclick: () => { type = tKey; paint(); } }, sourceTypeLabel(tKey))));
    m3uFields.hidden = type !== 'm3u';
    fileFields.hidden = type !== 'file';
    xtreamFields.hidden = type !== 'xtream';
    stalkerFields.hidden = type !== 'stalker';
    paintEpg();
  };
  paint();

  f.url.addEventListener('input', () => {
    paintEpg();
    const x = parseXtreamLink(f.url.value);
    suggestBox.replaceChildren(x ? h('div', { class: 'suggest' }, t('xtream_link_suggest'),
      h('button', { class: 'btn btn-sm', type: 'button', onclick: () => {
        f.server.value = x.server; f.username.value = x.username; f.password.value = x.password;
        type = 'xtream'; suggestBox.replaceChildren(); paint();
      } }, t('switch_to_xtream'))) : '');
  });

  const save = h('button', { class: 'btn btn-primary' }, existing ? t('save') : t('add'));
  save.addEventListener('click', async () => {
    save.disabled = true;
    errorBox.replaceChildren();
    const body = type === 'stalker'
      ? {
        type, name: f.name.value, epgUrl: f.epgUrl.value, server: f.portal.value, mac: f.mac.value,
        username: f.slogin.value, password: f.spassword.value, serial: f.serial.value,
        deviceId: f.deviceId.value, deviceId2: f.deviceId2.value,
      }
      : {
        type, name: f.name.value, url: f.url.value, server: f.server.value,
        username: f.username.value, password: f.password.value, epgUrl: f.epgUrl.value,
      };
    try {
      if (type === 'file') {
        if (!existing && !file) throw new Error(t('err_choose_file'));
        save.replaceChildren(spinner(), file ? t('uploading_file') : t('saving'));
        let src;
        if (existing) {
          src = await api.put(`/api/sources/${existing.id}`, body);
          if (file) src = await upload(`/api/sources/${existing.id}/upload`, {}, file);
        } else {
          src = await upload('/api/sources/upload', { name: body.name, epgUrl: body.epgUrl }, file);
        }
        close();
        toast(file ? t('file_uploaded', { channels: channelsWord(src.channelCount) }) : t('saved'));
      } else {
        if (existing) await api.put(`/api/sources/${existing.id}`, body);
        else await api.post('/api/sources', body);
        close();
        toast(t(existing ? 'saved_refreshing' : 'source_added_loading'));
      }
      await refreshState();
    } catch (e) {
      errorBox.replaceChildren(h('div', { class: 'form-error' }, trServer(e.message)));
      save.replaceChildren(existing ? t('save') : t('add'));
      save.disabled = false;
    }
  });

  const close = openModal(t(existing ? 'edit_source' : 'new_source'), h('div', {},
    seg,
    errorBox,
    m3uFields,
    fileFields,
    xtreamFields,
    stalkerFields,
    h('div', { class: 'field' }, h('label', {}, t('epg_url_label')), f.epgUrl, epgHint),
    h('div', { class: 'field' }, h('label', {}, t('name_label')), f.name)),
  [h('button', { class: 'btn', onclick: () => close() }, t('cancel')), save]);
  setTimeout(() => { if (type !== 'file') ({ m3u: f.url, xtream: f.server, stalker: f.portal })[type].focus(); }, 50);
}

// ---------------------------------------------------------------- settings

function epgCard() {
  const e = S.epg;
  let status;
  if (e.running) status = h('div', { class: 'status-line' }, spinner(), t('epg_updating'));
  else if (e.updatedAt) {
    status = h('div', {},
      h('div', { class: `status-line ${e.error ? 'error' : 'ok'}` }, icon(e.error ? 'alert' : 'check'),
        t('epg_updated_stats', { when: when(e.updatedAt), matched: e.matched, wanted: channelsWord(e.wanted) })),
      e.error && h('div', { class: 'status-line error' }, trServer(e.error)));
  } else status = h('div', { class: 'status-line' }, t('epg_not_updated_yet'));

  const refresh = async (btn) => {
    btn.disabled = true;
    try { await api.post('/api/epg/refresh'); } catch (err) { fail(err); }
    setTimeout(() => refreshState().catch(() => {}), 500);
  };
  const btn = h('button', { class: 'btn btn-sm', disabled: e.running }, icon('refresh'), t('refresh_now'));
  btn.addEventListener('click', () => refresh(btn));

  return h('div', { class: 'card section-card', id: 'epg-card' },
    h('h2', {}, t('epg_title')),
    !S.epgImportInstalled && h('div', { class: 'form-error' }, t('epgimport_missing')),
    status,
    h('div', { class: 'actions' }, btn),
    h('details', { class: 'howto', open: true },
      h('summary', {}, t('epg_one_time_setup')),
      h('ol', {},
        h('li', {}, t('epg_setup_step1')),
        h('li', {}, t('epg_setup_step2')),
        h('li', {}, t('epg_setup_step3')),
        h('li', {}, t('epg_setup_step4')),
        h('li', {}, t('epg_setup_step5')))));
}

function piconCard() {
  const p = S.picons || {};
  let status;
  if (!S.settings.picons) status = h('div', { class: 'status-line' }, t('picons_disabled'));
  else if (p.running) status = h('div', { class: 'status-line' }, spinner(), t('picons_updating'));
  else if (p.updatedAt) {
    status = h('div', {},
      h('div', { class: `status-line ${p.error ? 'error' : 'ok'}` }, icon(p.error ? 'alert' : 'check'),
        t('picons_updated_stats', { done: p.done, channels: channelsWord(p.channels), when: when(p.updatedAt) })),
      p.noLogo > 0 && h('div', { class: 'status-line' }, t('picons_no_logo', { count: noLogoWord(p.noLogo) })),
      p.error && h('div', { class: 'status-line error' }, trServer(p.error)),
      (p.failed || []).length > 0 && h('details', { class: 'howto' },
        h('summary', {}, t('picons_failed_count', { n: (p.failed || []).length })),
        h('ul', { style: 'margin:6px 0 0;padding-left:20px;color:var(--muted);font-size:14px' }, p.failed.map((f) => h('li', {}, f)))),
      p.dir && h('div', { class: 'status-line' }, t('picons_folder', { dir: p.dir })));
  } else status = h('div', { class: 'status-line' }, t('picons_after_apply'));
  const btn = h('button', { class: 'btn btn-sm', disabled: p.running || !S.settings.picons }, icon('refresh'), t('refresh_picons'));
  btn.addEventListener('click', async () => {
    btn.disabled = true;
    try { await api.post('/api/picons/refresh'); } catch (e) { fail(e); }
    setTimeout(() => refreshState().catch(() => {}), 500);
  });
  return h('div', { class: 'card section-card', id: 'picon-card' },
    h('h2', {}, t('picons_title')),
    h('p', { class: 'hint' }, t('picons_hint')),
    status,
    h('div', { class: 'actions' }, btn));
}

function viewSettings() {
  const s = { ...S.settings };
  const sel = (value, options, onchange) => {
    const el = h('select', {}, options.map(([v, label]) => h('option', { value: String(v), selected: String(v) === String(value) }, label)));
    el.addEventListener('change', () => onchange(el.value));
    return el;
  };
  const text = (key, props = {}) => {
    const el = h('input', { type: 'text', value: s[key] || '', autocomplete: 'off', autocapitalize: 'off', ...props });
    el.addEventListener('input', () => { s[key] = el.value; });
    return el;
  };
  const hours = [[-1, t('disabled')], ...Array.from({ length: 24 }, (_, i) => [i, t('nightly_at', { time: `${String(i).padStart(2, '0')}:00` })])];
  const picons = h('input', { type: 'checkbox', checked: s.picons });
  picons.addEventListener('change', () => { s.picons = picons.checked; });
  const insecure = h('input', { type: 'checkbox', checked: s.insecureTls });
  insecure.addEventListener('change', () => { s.insecureTls = insecure.checked; });

  const langEl = h('select', {});
  paintLanguageSelect(langEl, false);
  langEl.addEventListener('change', () => {
    onLanguageChange(langEl.value);
    toast(t('language_changed', { lang: langLabel(currentLang()) }));
  });

  const save = h('button', { class: 'btn btn-primary btn-lg' }, t('save_settings'));
  save.addEventListener('click', async () => {
    save.disabled = true;
    s.language = currentLang();
    try { await api.put('/api/settings', s); await refreshState(false); toast(t('settings_saved')); } catch (e) { fail(e); }
    save.disabled = false;
  });

  return h('section', {},
    h('div', { class: 'view-head' }, h('h1', {}, t('nav_settings'))),
    h('div', { class: 'card section-card' },
      h('h2', {}, t('language_card')),
      h('div', { class: 'field' }, h('label', {}, t('ui_language')), langEl,
        h('p', { class: 'hint' }, t('language_hint')))),
    h('div', { class: 'card section-card' },
      h('h2', {}, t('playback_card')),
      h('div', { class: 'field' }, h('label', {}, t('player_on_receiver')),
        sel(s.serviceType, [[4097, t('player_4097')], [5001, t('player_5001')], [5002, t('player_5002')]],
          (v) => { s.serviceType = Number(v); }),
        h('p', { class: 'hint' }, t('player_hint'))),
      h('div', { class: 'field' }, h('label', {}, t('xtream_stream_format')),
        sel(s.streamFormat, [['ts', t('format_ts')], ['m3u8', t('format_m3u8')]], (v) => { s.streamFormat = v; }),
        h('p', { class: 'hint' }, t('applies_after_refresh'))),
      h('div', { class: 'field' }, h('label', {}, t('bouquets_position')),
        sel(s.bouquetsOnTop ? 'top' : 'bottom', [['top', t('bouquets_top')], ['bottom', t('bouquets_bottom')]],
          (v) => { s.bouquetsOnTop = v === 'top'; }))),
    h('div', { class: 'card section-card' },
      h('h2', {}, t('playlist_updates_card')),
      h('div', { class: 'field' }, h('label', {}, t('auto_refresh')), sel(s.refreshHour, hours, (v) => { s.refreshHour = Number(v); }),
        h('p', { class: 'hint' }, t('auto_refresh_hint')))),
    epgCard(),
    piconCard(),
    tmdbCard(),
    h('div', { class: 'card section-card' },
      h('h2', {}, t('picons_settings_card')),
      h('label', { class: 'check', style: 'margin-bottom:14px' }, picons, h('span', {}, t('show_picons'))),
      h('div', { class: 'field' }, h('label', {}, t('picon_dir')), text('piconDir', { placeholder: (S.picons && S.picons.dir) || t('auto_detect') }),
        h('p', { class: 'hint' }, t('picon_dir_hint')))),
    h('div', { class: 'card section-card' },
      h('h2', {}, t('receiver_conn_card')),
      S.tabletUrl && h('p', { class: 'hint' }, t('tablet_url_prefix'), h('strong', {}, S.tabletUrl),
        t('tablet_url_suffix')),
      h('div', { class: 'field' }, h('label', {}, t('webif_url')), text('webifUrl', { type: 'url', placeholder: 'http://127.0.0.1' }),
        h('p', { class: 'hint' }, t('webif_url_hint'))),
      h('div', { class: 'field-row' },
        h('div', { class: 'field' }, h('label', {}, t('webif_user')), text('webifUser', { placeholder: t('if_auth_enabled') })),
        h('div', { class: 'field' }, h('label', {}, t('webif_password')), text('webifPassword', { type: 'password' }))),
      h('label', { class: 'check' }, insecure,
        h('span', {}, t('insecure_tls'), h('span', { class: 'hint', style: 'display:block;margin:2px 0 0' },
          t('insecure_tls_hint'))))),
    h('div', { class: 'actions' }, save),
    h('p', { class: 'hint', style: 'margin-top:24px' }, `Dreaminator ${S.version}`));
}

// ---------------------------------------------------------------- TMDB

// The key never comes back from the receiver: the card only knows whether one
// is stored and its last characters.
const TMDB_LANGUAGES = [
  ['ru-RU', 'Русский'], ['uk-UA', 'Українська'], ['be-BY', 'Беларуская'], ['kk-KZ', 'Қазақ тілі'],
  ['en-US', 'English'], ['fr-FR', 'Français'], ['pt-PT', 'Português'], ['pt-BR', 'Português (Brasil)'],
  ['es-ES', 'Español'], ['it-IT', 'Italiano'], ['de-DE', 'Deutsch'], ['pl-PL', 'Polski'],
  ['cs-CZ', 'Čeština'], ['tr-TR', 'Türkçe'], ['he-IL', 'עברית'], ['ar-SA', 'العربية'],
  ['ja-JP', '日本語'], ['zh-CN', '中文'], ['th-TH', 'ไทย'],
  ['ro-RO', 'Română'], ['lv-LV', 'Latviešu'], ['lt-LT', 'Lietuvių'], ['et-EE', 'Eesti'],
];
const tmdbKindLabel = (kind) => ({
  movie: t('tmdb_kind_movie_lower'),
  series: t('tmdb_kind_series_lower'),
  season: t('tmdb_kind_season'),
  episode: t('tmdb_kind_episode'),
}[kind] || kind);

function tmdbNotice() {
  const local = t('tmdb_notice_local');
  return h('div', { class: 'tmdb-notice' },
    h('p', { lang: 'en' }, 'This product uses the TMDB API but is not endorsed or certified by TMDB.'),
    local && currentLang() !== 'en' && h('p', {}, local));
}

function tmdbCard() {
  const card = h('div', { class: 'card section-card', id: 'tmdb-card' },
    h('h2', {}, t('tmdb_title')), h('div', { class: 'status-line' }, spinner(), t('loading')));
  api.get('/api/tmdb')
    .then((st) => fillTMDBCard(card, st))
    .catch((e) => card.replaceChildren(h('h2', {}, t('tmdb_title')), h('div', { class: 'form-error' }, trServer(e.message))));
  return card;
}

function tmdbTestLine(res) {
  return res.ok
    ? h('div', { class: 'status-line ok' }, icon('check'), t('tmdb_connected'))
    : h('div', { class: 'status-line error' }, icon('alert'), trServer(res.error) || t('tmdb_check_failed'));
}

function fillTMDBCard(card, st, note) {
  const paint = (next, n) => fillTMDBCard(card, next, n);
  const langName = (TMDB_LANGUAGES.find(([v]) => v === st.language) || [st.language, st.language])[1];

  const enabled = h('input', { type: 'checkbox', checked: st.enabled });
  const lang = h('select', {}, TMDB_LANGUAGES.map(([v, label]) => h('option', { value: v, selected: v === st.language }, label)));
  if (!TMDB_LANGUAGES.some(([v]) => v === st.language)) lang.prepend(h('option', { value: st.language, selected: true }, st.language));
  // Never pre-filled: the receiver does not hand the key out.
  const key = h('input', {
    type: 'password', autocomplete: 'off', autocapitalize: 'off', spellcheck: 'false',
    placeholder: st.keySet ? t('tmdb_key_saved_hint', { hint: st.keyHint || '' }) : t('tmdb_key_placeholder'),
  });
  const result = h('div', {}, note);

  const put = async (body, okText) => {
    try {
      const next = await api.put('/api/tmdb', body);
      paint(next);
      if (okText) toast(okText);
      return next;
    } catch (e) {
      fail(e);
      paint(st);
      return null;
    }
  };
  enabled.addEventListener('change', () => put({ enabled: enabled.checked }, enabled.checked ? t('tmdb_enabled_toast') : t('tmdb_disabled_toast')));
  lang.addEventListener('change', () => put({ language: lang.value }, t('tmdb_lang_toast', { lang: lang.selectedOptions[0].textContent })));

  const saveKey = h('button', { class: 'btn btn-sm btn-primary' }, t('save_key'));
  saveKey.addEventListener('click', async () => {
    const value = key.value.trim();
    if (!value) { key.focus(); return; }
    saveKey.disabled = true;
    saveKey.replaceChildren(spinner(), t('saving'));
    let next;
    try { next = await api.put('/api/tmdb', { apiKey: value }); } catch (e) {
      fail(e);
      saveKey.disabled = false;
      saveKey.replaceChildren(t('save_key'));
      return;
    }
    const res = await api.post('/api/tmdb/test', {}).catch((e) => ({ ok: false, error: e.message }));
    paint(next, tmdbTestLine(res));
    toast(res.ok ? t('tmdb_key_ok') : t('tmdb_key_bad'), res.ok ? '' : 'error');
  });

  const test = h('button', { class: 'btn btn-sm' }, icon('pulse'), t('test_connection'));
  test.addEventListener('click', async () => {
    const value = key.value.trim();
    if (!value && !st.keySet) { key.focus(); return; }
    test.disabled = true;
    test.replaceChildren(spinner(), t('testing'));
    const res = await api.post('/api/tmdb/test', value ? { apiKey: value } : {}).catch((e) => ({ ok: false, error: e.message }));
    result.replaceChildren(tmdbTestLine(res));
    test.disabled = false;
    test.replaceChildren(icon('pulse'), t('test_connection'));
  });

  const remove = st.keySet && h('button', { class: 'btn btn-sm btn-danger' }, icon('trash'), t('delete_key'));
  if (remove) remove.addEventListener('click', async () => {
    if (!await confirmDialog(t('delete_tmdb_key_title'), t('delete_tmdb_key_text'))) return;
    put({ clearKey: true }, t('key_deleted'));
  });

  let status;
  if (!st.keySet) status = h('div', { class: 'status-line' }, t('tmdb_no_key'));
  else if (!st.enabled) status = h('div', { class: 'status-line' }, t('tmdb_off_key_saved', { hint: st.keyHint || '' }));
  else status = h('div', { class: 'status-line ok' }, icon('check'), t('tmdb_on_status', { hint: st.keyHint || '', lang: langName }));

  // Native replaceChildren would print `false`; h() skips it.
  card.replaceChildren(...[
    h('h2', {}, t('tmdb_title')),
    h('p', { class: 'hint' }, t('tmdb_intro_1'), t('tmdb_intro_2')),
    status,
    h('label', { class: 'check', style: 'margin:14px 0' }, enabled,
      h('span', {}, t('tmdb_enable_label'), h('span', { class: 'hint', style: 'display:block;margin:2px 0 0' },
        t('tmdb_enable_hint')))),
    h('div', { class: 'field' }, h('label', {}, t('tmdb_api_key_label')), key,
      h('p', { class: 'hint' }, t('tmdb_key_help_1'),
        h('a', { href: 'https://www.themoviedb.org/settings/api', target: '_blank', rel: 'noopener noreferrer' }, 'themoviedb.org'),
        t('tmdb_key_help_2'),
        st.keySet ? t('tmdb_key_help_replace') : '')),
    h('div', { class: 'field' }, h('label', {}, t('tmdb_desc_lang')), lang),
    result,
    h('div', { class: 'actions' }, saveKey, test, remove),
    st.enabled && st.keySet && h('details', { class: 'howto' },
      h('summary', {}, t('tmdb_wrong_match')),
      h('p', { class: 'hint', style: 'margin:8px 0' }, t('tmdb_wrong_match_hint')),
      h('button', { class: 'btn btn-sm', onclick: () => go('settings', 'tmdb') }, icon('search'), t('tmdb_match_manually'))),
    tmdbNotice(),
  ].filter(Boolean));
}

function posterThumb(url) {
  const empty = h('div', { class: 'poster-thumb empty', 'aria-hidden': 'true' });
  if (!url) return empty;
  const img = h('img', { class: 'poster-thumb', src: url, alt: '', loading: 'lazy', referrerpolicy: 'no-referrer' });
  img.addEventListener('error', () => img.replaceWith(empty));
  return img;
}

async function viewTMDBMatch() {
  const [st, all] = await Promise.all([api.get('/api/tmdb'), api.get('/api/catalog/sources')]);
  // Playlists have Live TV only; films and series come from Stalker portals.
  const sources = all.filter((s) => (s.sections || []).includes('vod'));
  const head = h('div', { class: 'view-head' },
    h('button', { class: 'icon-btn back', 'aria-label': t('back'), onclick: () => go('settings') }, icon('back')),
    h('h1', {}, t('tmdb_match_title')));
  if (!st.enabled || !st.keySet) {
    return h('section', {}, head, emptyState(t('tmdb_not_connected'), t('tmdb_enable_in_settings'),
      h('button', { class: 'btn btn-primary', onclick: () => go('settings') }, t('to_settings'))));
  }
  if (!sources.length) {
    return h('section', {}, head, emptyState(t('no_stalker_portals'), t('no_stalker_portals_hint')));
  }

  let source = sources.some((s) => s.id === store.get('tmdbSource')) ? store.get('tmdbSource') : sources[0].id;
  let type = store.get('tmdbType') === 'series' ? 'series' : 'vod';
  let token = 0; // only the latest search or entry may paint
  const results = h('div', {});
  const detail = h('div', { class: 'tmdb-detail' });
  const q = h('input', { type: 'search', placeholder: t('tmdb_search_portal_ph'), autocomplete: 'off', enterkeyhint: 'search' });
  const portal = h('select', {}, sources.map((s) => h('option', { value: s.id, selected: s.id === source }, s.name)));
  portal.addEventListener('change', () => {
    source = portal.value;
    store.set('tmdbSource', source);
    token++;
    results.replaceChildren();
    detail.replaceChildren();
  });
  const seg = h('div', { class: 'segmented', role: 'tablist' });
  const paintSeg = () => seg.replaceChildren(...[['vod', t('movies')], ['series', t('series')]].map(([tKey, label]) =>
    h('button', { type: 'button', class: type === tKey ? 'on' : '', onclick: () => {
      type = tKey;
      store.set('tmdbType', tKey);
      paintSeg();
      if (q.value.trim()) find();
    } }, label)));
  paintSeg();

  const reasonLine = (v) => {
    const how = v.current && { manual: t('match_manual'), explicit: t('match_explicit'), auto: t('match_auto') }[v.current.match];
    switch (v.reason) {
      case 'matched': return h('div', { class: 'status-line ok' }, icon('check'), t('matched_how', { how: how || '' }));
      case 'ambiguous': return h('div', { class: 'status-line warn' }, icon('alert'), t('match_ambiguous'));
      case 'none': return h('div', { class: 'status-line' }, t('match_none'));
      case 'disabled': return h('div', { class: 'status-line' }, t('tmdb_disabled'));
      default: return h('div', { class: 'status-line error' }, icon('alert'), v.error || t('tmdb_unavailable'));
    }
  };

  const currentCard = (cur, poster) => {
    const meta = [cur.year, tmdbKindLabel(cur.type === 'tv' ? 'series' : 'movie'), (cur.genres || []).join(', '),
      cur.runtime && t('minutes_short', { n: cur.runtime }), cur.rating && `★ ${cur.rating}`].filter(Boolean).join(' · ');
    return h('div', { class: 'tmdb-current' }, posterThumb(poster),
      h('div', { class: 'row-main' },
        h('div', { class: 'row-title' }, cur.title),
        cur.originalTitle && h('div', { class: 'row-sub' }, cur.originalTitle),
        h('div', { class: 'row-sub' }, meta),
        cur.overview && h('div', { class: 'tmdb-overview' }, cur.overview)));
  };

  const matchPanel = (it, v) => {
    const isCurrent = (c) => !!v.current && v.current.id === c.id && v.current.type === c.type;
    const cands = h('div', {});
    const candRow = (c) => {
      const choose = h('button', { class: `btn btn-sm${isCurrent(c) ? '' : ' btn-primary'}`, disabled: isCurrent(c) }, isCurrent(c) ? t('selected') : t('select'));
      choose.addEventListener('click', async () => {
        choose.disabled = true;
        try {
          await api.put('/api/tmdb/match', { source, key: it.key, id: c.id, type: c.type });
          toast(t('selected_toast', { title: c.title }));
          openEntry(it);
        } catch (e) {
          fail(e);
          choose.disabled = false;
        }
      });
      return h('div', { class: 'row tmdb-row' }, posterThumb(c.poster),
        h('div', { class: 'row-main' },
          h('div', { class: 'row-title' }, c.title, c.year ? ` (${c.year})` : ''),
          h('div', { class: 'row-sub' }, [c.originalTitle, tmdbKindLabel(c.type === 'tv' ? 'series' : 'movie'), t('match_score', { score: c.score })].filter(Boolean).join(' · ')),
          c.overview && h('div', { class: 'tmdb-overview' }, c.overview)),
        choose);
    };
    const paintCands = (list, title) => cands.replaceChildren(
      h('h3', { class: 'tmdb-subhead' }, title),
      list.length ? h('div', { class: 'list' }, list.map(candRow)) : h('p', { class: 'hint' }, t('tmdb_nothing_suggested')));
    paintCands(v.candidates, t('tmdb_candidates'));

    const fq = h('input', { type: 'search', value: v.item.originalName || v.item.name, autocomplete: 'off', enterkeyhint: 'search', placeholder: t('tmdb_search_ph') });
    const ftype = h('select', { 'aria-label': t('what_to_search') },
      h('option', { value: 'movie', selected: v.item.kind === 'movie' }, t('tmdb_kind_movie')),
      h('option', { value: 'tv', selected: v.item.kind !== 'movie' }, t('tmdb_kind_series')));
    const fbtn = h('button', { class: 'btn' }, icon('search'), t('find_in_tmdb'));
    const tmdbFind = async () => {
      const text = fq.value.trim();
      if (!text) { fq.focus(); return; }
      fbtn.disabled = true;
      try {
        const res = await api.get(`/api/tmdb/search?q=${encodeURIComponent(text)}&type=${ftype.value}&year=${encodeURIComponent(v.item.year || '')}`);
        paintCands(res.candidates || [], t('tmdb_query_results', { q: text }));
      } catch (e) { fail(e); }
      fbtn.disabled = false;
    };
    fbtn.addEventListener('click', tmdbFind);
    fq.addEventListener('keydown', (e) => { if (e.key === 'Enter') tmdbFind(); });

    const reset = v.manual && h('button', { class: 'btn btn-sm' }, icon('refresh'), t('reset_to_auto'));
    if (reset) reset.addEventListener('click', async () => {
      reset.disabled = true;
      try {
        await api.del(`/api/tmdb/match?source=${encodeURIComponent(source)}&key=${encodeURIComponent(it.key)}`);
        toast(t('reset_to_auto_toast'));
        openEntry(it);
      } catch (e) {
        fail(e);
        reset.disabled = false;
      }
    });
    const poster = v.current && (v.candidates.find(isCurrent) || {}).poster;
    return h('div', { class: 'card' },
      h('h2', {}, v.item.name),
      h('div', { class: 'row-sub' }, [v.item.originalName, v.item.year, tmdbKindLabel(v.item.kind)].filter(Boolean).join(' · ')),
      reasonLine(v),
      v.reason !== 'error' && v.error && h('div', { class: 'status-line error' }, icon('alert'), v.error),
      v.current && currentCard(v.current, poster),
      reset && h('div', { class: 'actions' }, reset),
      cands,
      h('div', { class: 'tmdb-search' }, fq, ftype, fbtn));
  };

  const openEntry = async (it) => {
    const my = ++token;
    detail.replaceChildren(h('div', { class: 'loading' }, spinner()));
    try {
      const v = await api.get(`/api/tmdb/match?source=${encodeURIComponent(source)}&key=${encodeURIComponent(it.key)}`);
      if (my !== token) return;
      detail.replaceChildren(matchPanel(it, v));
    } catch (e) {
      if (my !== token) return;
      detail.replaceChildren(h('div', { class: 'form-error' }, e.status === 410 ? t('portal_list_updated') : trServer(e.message)));
    }
    if (detail.scrollIntoView) detail.scrollIntoView({ behavior: 'smooth', block: 'start' });
  };

  const find = async () => {
    const text = q.value.trim();
    if (!text) { q.focus(); return; }
    const my = ++token;
    detail.replaceChildren();
    results.replaceChildren(h('div', { class: 'loading' }, spinner()));
    try {
      const res = await api.get(`/api/catalog/${encodeURIComponent(source)}/search?type=${type}&q=${encodeURIComponent(text)}`);
      if (my !== token) return;
      const items = (res && res.items) || [];
      results.replaceChildren(items.length
        ? h('div', { class: 'list' }, items.map((it) => h('button', { class: 'row row-button', type: 'button', onclick: () => openEntry(it) },
          h('div', { class: 'row-main' },
            h('div', { class: 'row-title' }, it.name),
            h('div', { class: 'row-sub' }, [tmdbKindLabel(it.kind), it.year].filter(Boolean).join(' · '))),
          icon('chevron'))))
        : h('p', { class: 'hint' }, t('portal_nothing_found')));
    } catch (e) {
      if (my !== token) return;
      results.replaceChildren(h('div', { class: 'form-error' }, e.status === 404 && !e.json
        ? t('portal_search_soon')
        : trServer(e.message)));
    }
  };
  q.addEventListener('keydown', (e) => { if (e.key === 'Enter') find(); });

  return h('section', {}, head,
    h('p', { class: 'hint' }, t('tmdb_match_intro')),
    h('div', { class: 'card section-card' },
      sources.length > 1 && h('div', { class: 'field' }, h('label', {}, t('portal')), portal),
      seg,
      h('div', { class: 'tmdb-search' }, q, h('button', { class: 'btn btn-primary', onclick: find }, icon('search'), t('find')))),
    results,
    detail,
    tmdbNotice());
}

// ---------------------------------------------------------------- boot

document.documentElement.classList.toggle('channel-popovers', hasChannelPopovers);
window.addEventListener('scroll', closeChannelMenu, { passive: true });
window.addEventListener('resize', closeChannelMenu);
channelPhone.addEventListener('change', () => {
  closeChannelMenu();
  const [tab, sub] = route();
  if (S && hasChannelPopovers && (tab === 'channels' || (tab === 'lists' && sub && sub !== 'receiver'))) render();
});

// Reserve the actual panel height, including wrapped copy and safe-area padding.
new ResizeObserver(() => {
  document.documentElement.style.setProperty('--apply-h', `${$('#applybar').getBoundingClientRect().height}px`);
}).observe($('#applybar'));

for (const a of document.querySelectorAll('.nav a')) a.prepend(icon(a.dataset.icon));

// Apply browser language immediately before state fetch completes.
applyDocumentLanguage(getLanguage());
const topLangSelect = $('#lang-select');
if (topLangSelect) {
  paintLanguageSelect(topLangSelect, true);
  topLangSelect.addEventListener('change', () => onLanguageChange(topLangSelect.value));
}
window.addEventListener('languagechange', () => {
  if (getLanguagePref() === 'auto') onLanguageChange('auto');
});

// A stale search would hide the group or playlist the user just opened.
window.addEventListener('hashchange', () => { closeChannelMenu(); ui.q = ''; window.scrollTo(0, 0); render(); });

(async function boot() {
  const loadingEl = $('#view .loading');
  if (loadingEl) loadingEl.textContent = t('loading');
  try {
    await refreshState(false);
    applyDocumentLanguage(currentLang());
    paintLanguageSelect($('#lang-select'), true);
    if (getLanguagePref() === 'auto' && detectBrowserLanguage()) {
      syncLanguageToServer();
    }
  } catch (e) {
    $('#view').replaceChildren(emptyState(t('dreaminator_offline'), t('dreaminator_offline_hint'),
      h('button', { class: 'btn btn-primary', onclick: () => location.reload() }, t('retry'))));
    return;
  }
  render();
  pollState();
  pollBox();
})();
