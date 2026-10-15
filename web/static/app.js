import { receiverUI } from './receiver.js';

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
const channelsWord = (n) => plural(n, 'канал', 'канала', 'каналов');

function when(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  const diff = (Date.now() - d) / 60000;
  if (diff < 1) return 'только что';
  if (diff < 60) return `${Math.round(diff)} мин назад`;
  const time = d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
  const today = new Date();
  const yesterday = new Date(Date.now() - 86400000);
  if (d.toDateString() === today.toDateString()) return `сегодня в ${time}`;
  if (d.toDateString() === yesterday.toDateString()) return `вчера в ${time}`;
  return `${d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' })} в ${time}`;
}

const store = {
  get(k) { try { return localStorage.getItem(k); } catch { return null; } },
  set(k, v) { try { localStorage.setItem(k, v); } catch { /* private mode */ } },
};

// ---------------------------------------------------------------- api

async function request(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { /* not json */ }
  if (!res.ok) {
    const err = new Error((data && data.error) || `Ошибка ${res.status}`);
    err.status = res.status;
    err.json = data != null; // false: no such route on this receiver
    throw err;
  }
  return data;
}
// Multipart upload: text fields first, the file last (the receiver streams
// the file straight to disk once it reaches it).
async function upload(path, fields, file) {
  const form = new FormData();
  for (const [k, v] of Object.entries(fields)) form.append(k, v);
  form.append('file', file, file.name);
  const res = await fetch(path, { method: 'POST', body: form });
  const data = await res.json().catch(() => null);
  if (!res.ok) throw new Error((data && data.error) || `Ошибка ${res.status}`);
  return data;
}

const api = {
  get: (p) => request('GET', p),
  post: (p, b) => request('POST', p, b || {}),
  put: (p, b) => request('PUT', p, b),
  del: (p) => request('DELETE', p),
};

// ---------------------------------------------------------------- state

let S = null;          // server state
let dragging = false;
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
  renderChrome();
  if (!changed || !rerender || dragging || modalOpen) return;
  const [tab, sub] = route();
  if (tab === 'lists' && sub === 'receiver') return;
  if (tab === 'sources' || tab === 'lists') render();
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

async function pollBox() {
  const el = $('#box-status');
  try {
    const box = await api.get('/api/box');
    if (box.ok) {
      const what = box.standby ? 'в режиме ожидания' : box.current ? `сейчас: ${box.current}` : 'на связи';
      el.replaceChildren(h('i', { class: 'dot ok' }), h('span', {}, `Ресивер ${what}`));
    } else {
      el.replaceChildren(h('i', { class: 'dot bad' }), h('span', { title: box.error }, 'Нет связи с ресивером'));
    }
  } catch {
    el.replaceChildren(h('i', { class: 'dot bad' }), h('span', {}, 'Нет связи с Dreaminator'));
  }
  setTimeout(pollBox, 20000);
}

// ---------------------------------------------------------------- ui chrome

let toastTimer;
function toast(msg, kind = '') {
  const t = $('#toast');
  t.textContent = msg;
  t.className = `toast show ${kind}`;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { t.className = 'toast'; }, kind === 'error' ? 5000 : 2600);
}
const fail = (e) => toast(e.message || String(e), 'error');

function renderChrome() {
  const bar = $('#applybar');
  if (!S) return;
  if (S.dirty) {
    const btn = h('button', { class: 'btn btn-good btn-lg', onclick: () => applyNow(btn) }, icon('send'), 'Отправить на ТВ');
    bar.replaceChildren(h('div', { class: 'applybar' },
      h('div', { class: 'applybar-text' }, h('strong', {}, 'Есть изменения'), h('span', {}, 'Отправьте их — или они уйдут сами: ночью с обновлением плейлистов или когда на ТВ закроют каталог после правки «Избранного»')),
      btn));
  } else if (S.lastApplyError) {
    const btn = h('button', { class: 'btn btn-lg', onclick: () => applyNow(btn) }, icon('refresh'), 'Повторить');
    bar.replaceChildren(h('div', { class: 'applybar error' },
      h('div', { class: 'applybar-text' }, h('strong', {}, 'Не получилось отправить на ТВ'), h('span', {}, S.lastApplyError)),
      btn));
  } else {
    bar.replaceChildren();
  }
}

async function applyNow(btn) {
  btn.disabled = true;
  btn.replaceChildren(spinner(), 'Отправляем…');
  try {
    await api.post('/api/apply');
    toast('Готово! Списки обновлены на телевизоре');
  } catch (e) {
    fail(e);
  }
  await refreshState().catch(() => {});
}

function openModal(title, body, actions, onClose) {
  modalOpen = true;
  const close = () => {
    overlay.remove();
    modalOpen = false;
    document.removeEventListener('keydown', onKey);
    if (onClose) onClose();
  };
  const onKey = (e) => { if (e.key === 'Escape') close(); };
  const overlay = h('div', { class: 'modal-overlay', onclick: (e) => { if (e.target === overlay) close(); } },
    h('div', { class: 'modal', role: 'dialog', 'aria-modal': 'true', 'aria-label': title },
      h('div', { class: 'modal-head' }, h('h2', {}, title),
        h('button', { class: 'icon-btn', 'aria-label': 'Закрыть', onclick: close }, icon('x'))),
      body,
      actions && h('div', { class: 'modal-actions' }, actions)));
  document.addEventListener('keydown', onKey);
  document.body.append(overlay);
  return close;
}

function confirmDialog(title, text, okText = 'Удалить') {
  return new Promise((resolve) => {
    let answer = false;
    const close = openModal(title, h('p', {}, text), [
      h('button', { class: 'btn', onclick: () => close() }, 'Отмена'),
      h('button', { class: 'btn btn-danger solid', onclick: () => { answer = true; close(); } }, okText),
    ], () => resolve(answer));
  });
}

function promptDialog(title, label, value = '', okText = 'Сохранить') {
  return new Promise((resolve) => {
    let answer = null;
    const input = h('input', { type: 'text', value, autocomplete: 'off' });
    const submit = () => { if (input.value.trim()) { answer = input.value.trim(); close(); } };
    input.addEventListener('keydown', (e) => { if (e.key === 'Enter') submit(); });
    const close = openModal(title, h('div', { class: 'field' }, h('label', {}, label), input), [
      h('button', { class: 'btn', onclick: () => close() }, 'Отмена'),
      h('button', { class: 'btn btn-primary', onclick: submit }, okText),
    ], () => resolve(answer));
    setTimeout(() => input.focus(), 50);
  });
}

// Pointer-based drag & drop that works with fingers: grab the ⋮⋮ handle.
function makeSortable(list, onDone) {
  list.addEventListener('pointerdown', (e) => {
    const handle = e.target.closest('.grip');
    const item = handle && handle.closest('.sortable-item');
    if (!item || item.parentElement !== list) return;
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
      if (ids.join('|') !== before) onDone(ids);
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', up);
    handle.addEventListener('pointercancel', up);
  });
}

const grip = () => h('span', { class: 'icon-btn grip', 'aria-label': 'Перетащить' }, icon('grip'));

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
  document.querySelectorAll('.nav a').forEach((a) => a.classList.toggle('active', a.dataset.tab === tab));
  let el;
  try {
    switch (tab) {
      case 'lists': el = rest[0] === 'receiver' ? await receiver.detail(rest[1]) : rest[0] ? viewBouquet(rest[0]) : await viewBouquets(); break;
      case 'sources': el = viewSources(); break;
      case 'settings': el = rest[0] === 'tmdb' ? await viewTMDBMatch() : viewSettings(); break;
      default: el = await viewChannels(rest[0], rest[1]);
    }
  } catch (e) {
    el = emptyState('Что-то пошло не так', e.message, h('button', { class: 'btn', onclick: render }, 'Повторить'));
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
    return emptyState('Пока нет ни одного плейлиста',
      'Добавьте ссылку на плейлист M3U или данные Xtream — каналы появятся здесь.',
      h('button', { class: 'btn btn-primary btn-lg', onclick: () => go('sources', 'new') }, icon('plus'), 'Добавить плейлист'));
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
      el = emptyState('Не удалось загрузить каналы', e.message);
    }
    if (my === token) content.replaceChildren(el);
  };

  const input = h('input', { type: 'search', placeholder: 'Поиск канала', value: ui.q, 'aria-label': 'Поиск канала' });
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
  const select = h('select', { 'aria-label': 'В какой список добавлять каналы' },
    S.bouquets.map((b) => h('option', { value: b.id, selected: current && b.id === current.id }, `${b.name} · ${b.channels.length}`)),
    h('option', { value: '__new' }, '+ Новый список…'));
  select.addEventListener('change', async () => {
    if (select.value === '__new') {
      const b = await createBouquetFlow();
      if (b) setTarget(b.id);
    } else {
      setTarget(select.value);
    }
    render();
  });
  return h('label', { class: 'target' }, h('span', { class: 'target-label' }, 'Добавлять в'), select);
}

function updatePickerCounts() {
  const select = $('.target select');
  if (!select) return;
  for (const b of S.bouquets) {
    const opt = [...select.options].find((o) => o.value === b.id);
    if (opt) opt.textContent = `${b.name} · ${b.channels.length}`;
  }
}

async function groupGrid(src) {
  const groups = await api.get(`/api/sources/${src.id}/groups`);
  if (!groups.length) {
    if (src.refreshing) return h('div', { class: 'loading' }, spinner(), ' Загружаем каналы…');
    return emptyState('Каналов пока нет', src.error || 'Плейлист ещё не загружен.',
      h('button', { class: 'btn', onclick: () => go('sources') }, 'К плейлистам'));
  }
  return h('div', { class: 'groups' }, groups.map((g) =>
    h('button', { class: 'group', onclick: () => go('channels', src.id, g.name) },
      h('span', {}, g.name), h('span', { class: 'count' }, g.count))));
}

async function groupChannels(src, group) {
  const page = await api.get(`/api/channels?source=${encodeURIComponent(src.id)}&group=${encodeURIComponent(group)}&limit=2000`);
  const target = targetBouquet();
  const addAll = async () => {
    let b = targetBouquet();
    if (!b) b = await createBouquetFlow();
    if (!b) return;
    try {
      await api.post(`/api/bouquets/${b.id}/channels`, { add: page.items.map(refOf) });
      await refreshState(false);
      toast(`Группа добавлена в «${b.name}»`);
      render();
    } catch (e) { fail(e); }
  };
  const asList = async () => {
    try {
      const b = await api.post('/api/bouquets', { name: group, channels: page.items.map(refOf) });
      setTarget(b.id);
      await refreshState(false);
      toast(`Создан список «${group}»`);
      render();
    } catch (e) { fail(e); }
  };
  return h('div', {},
    h('div', { class: 'view-head' },
      h('button', { class: 'icon-btn back', 'aria-label': 'Все группы', onclick: () => go('channels', src.id) }, icon('back')),
      h('h1', {}, group), h('span', { class: 'count' }, page.total)),
    h('div', { class: 'group-actions' },
      h('button', { class: 'btn btn-sm', onclick: addAll }, icon('plus'), target ? `Все в «${target.name}»` : 'Добавить все'),
      h('button', { class: 'btn btn-sm', onclick: asList }, icon('list'), 'Сделать отдельным списком')),
    channelList(page, false));
}

async function searchResults(q) {
  const page = await api.get(`/api/channels?q=${encodeURIComponent(q)}&limit=300`);
  if (!page.items.length) return emptyState('Ничего не нашлось', `Нет каналов, похожих на «${q}».`);
  return channelList(page, true);
}

function channelList(page, showGroup) {
  const srcName = (id) => (S.sources.find((s) => s.id === id) || {}).name || '';
  return h('div', {},
    h('div', { class: 'list' }, page.items.map((ch) => channelRow(ch, showGroup && `${ch.group}${S.sources.length > 1 ? ' · ' + srcName(ch.source) : ''}`))),
    page.total > page.items.length && h('p', { class: 'more' }, `Показаны первые ${page.items.length} из ${page.total}. Уточните поиск.`));
}

function starButton(ch) {
  const btn = h('button', { class: 'icon-btn star' });
  const paint = () => {
    const on = inBouquet(targetBouquet(), ch);
    btn.classList.toggle('on', on);
    btn.setAttribute('aria-label', on ? 'Убрать из списка' : 'Добавить в список');
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
      toast(had ? `Убрано из «${b.name}»` : `Добавлено в «${b.name}»`);
    } catch (e) { fail(e); }
  });
  paint();
  return btn;
}

function channelRow(ch, sub) {
  return h('div', { class: 'row' },
    logo(ch),
    h('div', { class: 'row-main' }, h('div', { class: 'row-title' }, ch.name), sub && h('div', { class: 'row-sub' }, sub)),
    h('button', { class: 'btn btn-sm btn-ghost', onclick: () => zap(ch), 'aria-label': `Включить ${ch.name} на телевизоре` }, icon('play'), 'На ТВ'),
    probeButton(ch),
    starButton(ch));
}

const probeButton = (ch) => h('button', { class: 'icon-btn', title: 'Проверить поток', 'aria-label': `Проверить поток ${ch.name}`, onclick: () => probeDialog(ch) }, icon('pulse'));

async function probeDialog(ch) {
  const body = h('div', {},
    h('div', { class: 'loading' }, spinner(), ` Проверяю «${ch.name}»…`),
    h('p', { class: 'hint' }, 'Займёт до 15 секунд. На это время проверка занимает одно подключение к провайдеру.'));
  const close = openModal('Проверка канала', body, [h('button', { class: 'btn', onclick: () => close() }, 'Закрыть')]);
  try {
    const r = await api.post('/api/probe', { source: ch.source, id: ch.id });
    body.replaceChildren(probeView(ch, r));
  } catch (e) {
    body.replaceChildren(h('div', { class: 'form-error' }, e.message));
  }
}

function probeView(ch, r) {
  const row = (label, value) => value && h('div', { class: 'kv' }, h('span', { class: 'kv-label' }, label), h('span', {}, value));
  return h('div', {},
    r.ok
      ? h('div', { class: 'status-line ok big' }, icon('check'), `«${ch.name}» открывается`)
      : h('div', { class: 'form-error' }, r.error || 'Поток не открылся'),
    h('div', { class: 'kv-list' },
      row('Ответ сервера', r.firstByteMs ? `${(r.firstByteMs / 1000).toFixed(1)} с` : ''),
      row('Формат', r.format),
      row('Видео', r.video),
      row('Звук', (r.audio || []).join(', ')),
      row('Скорость загрузки', r.mbps ? `${r.mbps.toFixed(1)} Мбит/с` : ''),
      row('Сервер', r.host)),
    (r.warnings || []).map((w) => h('div', { class: 'warn-box' }, icon('alert'), w)),
    r.ok && !(r.warnings || []).length && h('p', { class: 'hint', style: 'margin-top:12px' }, 'Всё в порядке: ресивер должен показать этот канал.'));
}

async function zap(ch) {
  toast(`Включаю «${ch.name}» на телевизоре…`);
  try {
    await api.post('/api/zap', { source: ch.source, id: ch.id });
  } catch (e) { fail(e); }
}

// ---------------------------------------------------------------- bouquets

async function createBouquetFlow() {
  const name = await promptDialog('Новый список', 'Как назвать список на телевизоре?', '', 'Создать');
  if (!name) return null;
  try {
    const b = await api.post('/api/bouquets', { name, channels: [] });
    await refreshState(false);
    toast(`Список «${name}» создан`);
    return S.bouquets.find((x) => x.id === b.id) || null;
  } catch (e) { fail(e); return null; }
}

async function renameBouquet(b) {
  const name = await promptDialog('Переименовать список', 'Новое название', b.name);
  if (!name || name === b.name) return;
  try { await api.put(`/api/bouquets/${b.id}`, { name }); await refreshState(); } catch (e) { fail(e); }
}

// Resolves to true once the list is deleted.
async function deleteBouquet(b) {
  if (!await confirmDialog('Удалить список?', `Список «${b.name}» пропадёт с телевизора после отправки. Каналы в плейлисте останутся.`)) return false;
  try { await api.del(`/api/bouquets/${b.id}`); await refreshState(false); return true; } catch (e) { fail(e); return false; }
}

const receiver = receiverUI({ h, api, icon, grip, makeSortable, go, render, toast, fail, promptDialog, confirmDialog, emptyState, plural });

async function viewBouquets() {
  const native = await receiver.list();
  const list = h('div', { class: 'cards' }, S.bouquets.map((b) => {
    const missing = b.channels.filter((c) => c.missing).length;
    return h('div', { class: 'card list-card sortable-item', 'data-id': b.id },
      grip(),
      h('button', { class: 'list-open', onclick: () => go('lists', b.id) },
        h('div', { class: 'row-main' },
          h('div', { class: 'row-title' }, b.name),
          h('div', { class: 'row-sub' }, b.channels.length ? channelsWord(b.channels.length) : 'пустой — на ТВ не показывается',
            missing ? ` · ${missing} недоступно` : '')),
        icon('chevron')),
      h('button', { class: 'icon-btn', 'aria-label': `Переименовать ${b.name}`, onclick: () => renameBouquet(b) }, icon('edit')),
      h('button', { class: 'icon-btn danger', 'aria-label': `Удалить ${b.name}`, onclick: async () => { if (await deleteBouquet(b)) render(); } }, icon('trash')));
  }));
  makeSortable(list, async (ids) => {
    try { await api.put('/api/bouquet-order', { ids }); } catch (e) { fail(e); }
    refreshState().catch(() => {});
  });
  const create = async () => { const b = await createBouquetFlow(); if (b) go('lists', b.id); };
  return h('section', {},
    h('div', { class: 'view-head' }, h('h1', {}, 'Мои списки'),
      h('div', { class: 'actions' },
        h('button', { class: 'btn', onclick: () => native.scrollIntoView({ behavior: 'smooth', block: 'start' }) }, icon('tv'), 'Букеты ресивера'),
        h('button', { class: 'btn', onclick: create }, icon('plus'), 'Новый список'))),
    h('h2', {}, 'Dreaminator'),
    h('p', { class: 'hint' }, 'Списки из ваших плейлистов. После изменений нажмите «Отправить на ТВ». Порядок меняется перетаскиванием за ⋮⋮.'),
    S.bouquets.length ? list : emptyState('Списков нет', 'Создайте первый список, например «Избранное».'),
    native);
}

function viewBouquet(id) {
  const b = S.bouquets.find((x) => x.id === id);
  if (!b) return emptyState('Список не найден', 'Возможно, его уже удалили.', h('button', { class: 'btn', onclick: () => go('lists') }, 'К спискам'));

  const pick = () => { setTarget(b.id); go('channels'); };
  const removeChannel = async (c) => {
    try { await api.post(`/api/bouquets/${b.id}/channels`, { remove: [c] }); await refreshState(); } catch (e) { fail(e); }
  };

  const key = (c) => `${c.source}/${c.id}`;
  const list = h('div', { class: 'list' }, b.channels.map((c) =>
    h('div', { class: `row sortable-item ${c.missing ? 'missing' : ''}`, 'data-id': key(c) },
      grip(),
      logo(c),
      h('div', { class: 'row-main' }, h('div', { class: 'row-title' }, c.name),
        c.missing && h('div', { class: 'row-sub' }, 'Нет в плейлисте — на ТВ не попадёт')),
      !c.missing && h('button', { class: 'icon-btn', 'aria-label': `Включить ${c.name} на телевизоре`, onclick: () => zap(c) }, icon('play')),
      !c.missing && probeButton(c),
      h('button', { class: 'icon-btn danger', 'aria-label': `Убрать ${c.name}`, onclick: () => removeChannel(c) }, icon('x')))));
  makeSortable(list, async (ids) => {
    const byKey = new Map(b.channels.map((c) => [key(c), c]));
    const channels = ids.map((k) => byKey.get(k)).filter(Boolean).map((c) => ({ source: c.source, id: c.id, name: c.name }));
    try { await api.put(`/api/bouquets/${b.id}`, { channels }); } catch (e) { fail(e); }
    refreshState().catch(() => {});
  });

  return h('section', {},
    h('div', { class: 'view-head' },
      h('button', { class: 'icon-btn back', 'aria-label': 'Все списки', onclick: () => go('lists') }, icon('back')),
      h('h1', {}, b.name),
      h('button', { class: 'icon-btn', 'aria-label': 'Переименовать', onclick: () => renameBouquet(b) }, icon('edit')),
      h('button', { class: 'icon-btn danger', 'aria-label': 'Удалить список', onclick: async () => { if (await deleteBouquet(b)) go('lists'); } }, icon('trash'))),
    b.channels.length
      ? [h('p', { class: 'hint' }, `${channelsWord(b.channels.length)}. Порядок меняется перетаскиванием за ⋮⋮.`), list,
        h('div', { class: 'actions' }, h('button', { class: 'btn', onclick: pick }, icon('plus'), 'Добавить каналы'))]
      : emptyState('В списке пока пусто', 'Откройте каналы и нажмите звёздочку у тех, что нужны в этом списке.',
        h('button', { class: 'btn btn-primary btn-lg', onclick: pick }, icon('plus'), 'Выбрать каналы')));
}

// ---------------------------------------------------------------- sources

// Guides the playlist or provider advertises; detectedEpgUrl is the v0.1 field.
const detectedGuides = (s) => s.detectedEpgUrls || (s.detectedEpgUrl ? [s.detectedEpgUrl] : []);

// Where a source's guide comes from: the user's own links win over the ones
// the playlist or provider advertises.
function epgLine(s) {
  if (s.type === 'stalker' && !s.epgUrl) return 'Программа передач с портала';
  const own = s.epgUrl ? s.epgUrl.split(/[,\s]+/).filter(Boolean).length : 0;
  const n = own || detectedGuides(s).length;
  const count = n > 1 ? `: ${plural(n, 'источник', 'источника', 'источников')}, по порядку` : '';
  if (own) return `Программа передач указана вручную${count}`;
  if (n) return (s.type === 'xtream' ? 'Программа передач от провайдера Xtream' : 'Программа передач из плейлиста') + count;
  if (s.updatedAt && s.type !== 'xtream') return 'В плейлисте нет ссылки на программу передач — её можно указать в «Изменить»';
  return 'Программа передач не указана';
}

function viewSources() {
  const cards = S.sources.map((s) => {
    let status;
    if (s.refreshing) status = h('div', { class: 'status-line' }, spinner(), 'Загружаем каналы…');
    else if (s.error) status = h('div', { class: 'status-line error' }, icon('alert'), s.error);
    else if (s.updatedAt) status = h('div', { class: 'status-line ok' }, icon('check'),
      `${channelsWord(s.channelCount)} · ${s.type === 'file' ? 'загружен' : 'обновлён'} ${when(s.updatedAt)}`);
    const isFile = s.type === 'file';
    const refresh = async (btn) => {
      btn.disabled = true;
      try { await api.post(`/api/sources/${s.id}/refresh`); toast(`«${s.name}» обновлён`); } catch (e) { fail(e); }
      refreshState().catch(() => {});
    };
    const remove = async () => {
      if (!await confirmDialog('Удалить плейлист?', `Каналы из «${s.name}» пропадут из ваших списков.`)) return;
      try { await api.del(`/api/sources/${s.id}`); await refreshState(); } catch (e) { fail(e); }
    };
    const refreshBtn = h('button', { class: 'btn btn-sm', disabled: s.refreshing }, icon('refresh'), 'Обновить');
    refreshBtn.addEventListener('click', () => refresh(refreshBtn));
    const checkBtn = h('button', { class: 'btn btn-sm' }, icon('pulse'), 'Проверить');
    checkBtn.addEventListener('click', async () => {
      checkBtn.disabled = true;
      checkBtn.replaceChildren(spinner(), 'Проверяю…');
      try {
        const acc = await api.post(`/api/sources/${s.id}/check`);
        if (acc && acc.error) fail(new Error(acc.error));
        else toast(`«${s.name}» проверен`);
      } catch (e) { fail(e); }
      refreshState().catch(() => {});
    });
    return h('div', { class: 'card' },
      h('h2', {}, s.name, ' ', h('span', { class: 'badge' }, SOURCE_TYPES[s.type] || s.type)),
      status,
      h('div', { class: 'status-line' }, epgLine(s)),
      accountLine(s.account),
      (s.channelCount > 0 || s.type === 'stalker') && h('p', { class: 'hint' }, s.type === 'stalker' ? 'Портал доступен в дополнении Dreaminator: Live TV, фильмы и сериалы. Для обычного списка каналов выберите каналы или группы и нажмите «Отправить на ТВ».' : 'Для обычного списка каналов выберите каналы или группы и нажмите «Отправить на ТВ».'),
      isFile && h('div', { class: 'status-line' }, 'Сам не обновляется: если провайдер пришлёт новый файл, загрузите его сюда.'),
      h('div', { class: 'actions' },
        s.channelCount > 0 && h('button', { class: 'btn btn-sm btn-primary', onclick: () => go('channels', s.id) }, icon('list'), 'Выбрать группы и каналы'),
        isFile
          ? h('button', { class: 'btn btn-sm', onclick: () => sourceForm(s) }, icon('upload'), 'Загрузить новый файл')
          : refreshBtn,
        checkBtn,
        h('button', { class: 'btn btn-sm', onclick: () => sourceForm(s) }, icon('edit'), 'Изменить'),
        h('button', { class: 'btn btn-sm btn-danger', onclick: remove }, icon('trash'), 'Удалить')));
  });
  return h('section', {},
    h('div', { class: 'view-head' }, h('h1', {}, 'Плейлисты'),
      h('button', { class: 'btn btn-primary', onclick: () => sourceForm() }, icon('plus'), 'Добавить')),
    S.sources.length
      ? h('div', { class: 'cards' }, cards)
      : emptyState('Плейлистов пока нет', 'Понадобится ссылка на M3U, файл M3U, данные Xtream или адрес портала Stalker и MAC от вашего IPTV-провайдера.',
        h('button', { class: 'btn btn-primary btn-lg', onclick: () => sourceForm() }, icon('plus'), 'Добавить плейлист')));
}

const fmtDate = (iso) => new Date(iso).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' });

// One line about the subscription: status, until when, plan, connections.
function accountLine(a) {
  if (!a) return null;
  if (a.error) return h('div', { class: 'status-line error' }, icon('alert'), `Проверка ${when(a.checkedAt)}: ${a.error}`);
  if (a.note) return h('div', { class: 'status-line ok' }, icon('check'), `${a.note} · проверено ${when(a.checkedAt)}`);
  let kind = 'ok';
  const parts = [a.active === false ? `Подписка: ${a.status || 'неактивна'}` : 'Подписка активна'];
  if (a.active === false) kind = 'error';
  if (a.unlimited) parts.push('без срока');
  else if (a.expires) {
    const days = Math.floor((new Date(a.expires) - Date.now()) / 86400000);
    if (days < 0) { kind = 'error'; parts[0] = `Подписка закончилась ${fmtDate(a.expires)}`; }
    else {
      parts.push(`до ${fmtDate(a.expires)}`);
      if (days <= 7) { kind = kind === 'error' ? kind : 'warn'; parts.push(`осталось ${plural(days, 'день', 'дня', 'дней')}`); }
    }
  } else if (a.rawExpiry) parts.push(`до ${a.rawExpiry}`);
  if (a.plan) parts.push(`тариф «${a.plan}»`);
  if (a.maxConnections) parts.push(`подключений ${a.connections || 0} из ${a.maxConnections}`);
  if (a.trial) parts.push('пробная');
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

const SOURCE_TYPES = { m3u: 'Ссылка M3U', file: 'Файл M3U', xtream: 'Xtream', stalker: 'Stalker' };

function sourceForm(existing) {
  let type = existing ? existing.type : 'm3u';
  // An uploaded playlist cannot turn into a link (and vice versa): there
  // would be nothing to read.
  const types = !existing ? ['m3u', 'file', 'xtream', 'stalker'] : existing.type === 'file' ? ['file'] : ['m3u', 'xtream', 'stalker'];
  const val = (key, onlyType) => (existing && (!onlyType || existing.type === onlyType) ? existing[key] || '' : '');
  let file = null;
  const f = {
    name: h('input', { type: 'text', value: existing ? existing.name : '', placeholder: 'Например: Основной' }),
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
    h('div', { class: 'field' }, h('label', {}, 'Ссылка на плейлист'), f.url, suggestBox));
  const fileInput = h('input', { type: 'file', class: 'visually-hidden' });
  const fileLabel = h('span', {});
  const paintFile = () => fileLabel.replaceChildren(file
    ? h('span', {}, h('strong', {}, file.name), ` · ${file.size < 1 << 20 ? `${Math.max(1, Math.round(file.size / 1024))} КБ` : `${(file.size / (1 << 20)).toFixed(1)} МБ`}`)
    : (existing ? 'Выбрать новый файл (необязательно)' : 'Выбрать файл .m3u'));
  paintFile();
  fileInput.addEventListener('change', () => {
    file = fileInput.files[0] || null;
    paintFile();
    paintEpg();
    if (file && !f.name.value.trim()) f.name.value = file.name.replace(/\.[^.]+$/, '');
  });
  const fileFields = h('div', {},
    h('div', { class: 'field' }, h('span', { class: 'label' }, 'Файл плейлиста'),
      h('label', { class: 'filepick' }, icon('upload'), fileLabel, fileInput),
      h('p', { class: 'hint' }, 'Файл сохранится на ресивере. Сам по ночам он не обновляется: если провайдер пришлёт новый, загрузите его здесь же.')));
  const xtreamFields = h('div', {},
    h('p', { class: 'hint' }, 'В этой версии Xtream используется для букетов Live TV. Каталог фильмов и сериалов внутри дополнения пока доступен только для Stalker.'),
    h('div', { class: 'field' }, h('label', {}, 'Адрес сервера'), f.server),
    h('div', { class: 'field-row' },
      h('div', { class: 'field' }, h('label', {}, 'Логин'), f.username),
      h('div', { class: 'field' }, h('label', {}, 'Пароль'), f.password)));

  const advancedOpen = !!(val('username', 'stalker') || val('serial', 'stalker') || val('deviceId', 'stalker'));
  const stalkerFields = h('div', {},
    h('p', { class: 'hint' }, 'После добавления портал появится в дополнении Dreaminator на телевизоре. Букеты — отдельная выборка Live TV: создайте списки каналов и нажмите «Отправить на ТВ». Фильмы и сериалы в букеты не добавляются.'),
    h('div', { class: 'field' }, h('label', {}, 'Адрес портала'), f.portal,
      h('p', { class: 'hint' }, 'Так, как его дал провайдер, обычно заканчивается на /c/.')),
    h('div', { class: 'field' }, h('label', {}, 'MAC-адрес'), f.mac,
      h('p', { class: 'hint' }, 'MAC, который зарегистрирован у провайдера.')),
    h('details', { class: 'howto', open: advancedOpen },
      h('summary', {}, 'Дополнительно — если провайдер выдал'),
      h('div', { class: 'field-row', style: 'margin-top:10px' },
        h('div', { class: 'field' }, h('label', {}, 'Логин'), f.slogin),
        h('div', { class: 'field' }, h('label', {}, 'Пароль'), f.spassword)),
      h('div', { class: 'field' }, h('label', {}, 'Серийный номер (SN)'), f.serial),
      h('div', { class: 'field-row' },
        h('div', { class: 'field' }, h('label', {}, 'Device ID'), f.deviceId),
        h('div', { class: 'field' }, h('label', {}, 'Device ID 2'), f.deviceId2)),
      h('p', { class: 'hint' }, 'Заполняйте только то, что дал провайдер: портал запоминает Device ID при первом входе.')));

  const epgHint = h('p', { class: 'hint' });
  const paintEpg = () => {
    // The playlist's own guides only stand for the playlist they came from.
    const same = existing && existing.type === type && (type === 'file' ? !file : type === 'm3u' && f.url.value.trim() === existing.url);
    const found = same ? detectedGuides(existing) : [];
    f.epgUrl.placeholder = found.length ? found.join(', ') : 'http://…/epg.xml.gz';
    epgHint.textContent = type === 'xtream'
      ? 'Можно не заполнять — возьмём программу у провайдера Xtream.'
      : type === 'stalker'
      ? 'Можно не заполнять — возьмём программу с портала.'
      : !found.length
      ? 'Необязательно: если ссылки есть в самом плейлисте, подхватим их сами. Можно несколько через запятую — каналы ищутся по порядку.'
      : f.epgUrl.value.trim()
      ? 'Своя ссылка заменяет программу из плейлиста. Очистите поле, чтобы снова брать её из плейлиста.'
      : `${found.length > 1 ? 'Ссылки нашлись' : 'Ссылка нашлась'} в ${type === 'file' ? 'файле плейлиста' : 'плейлисте'} — поле можно не заполнять.`
        + (type === 'm3u' ? ' Если провайдер сменит ссылку, подхватим новую сами.' : '');
  };
  f.epgUrl.addEventListener('input', paintEpg);
  const seg = h('div', { class: 'segmented', role: 'tablist' });
  const paint = () => {
    seg.hidden = types.length < 2;
    seg.replaceChildren(...types.map((t) =>
      h('button', { class: type === t ? 'on' : '', type: 'button', onclick: () => { type = t; paint(); } }, SOURCE_TYPES[t])));
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
    suggestBox.replaceChildren(x ? h('div', { class: 'suggest' }, 'Это ссылка Xtream. Лучше подключить как Xtream — будет надёжнее.',
      h('button', { class: 'btn btn-sm', type: 'button', onclick: () => {
        f.server.value = x.server; f.username.value = x.username; f.password.value = x.password;
        type = 'xtream'; suggestBox.replaceChildren(); paint();
      } }, 'Переключить')) : '');
  });

  const save = h('button', { class: 'btn btn-primary' }, existing ? 'Сохранить' : 'Добавить');
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
        if (!existing && !file) throw new Error('Выберите файл плейлиста');
        save.replaceChildren(spinner(), file ? 'Загружаем файл…' : 'Сохраняем…');
        let src;
        if (existing) {
          src = await api.put(`/api/sources/${existing.id}`, body);
          if (file) src = await upload(`/api/sources/${existing.id}/upload`, {}, file);
        } else {
          src = await upload('/api/sources/upload', { name: body.name, epgUrl: body.epgUrl }, file);
        }
        close();
        toast(file ? `Файл загружен: ${channelsWord(src.channelCount)}` : 'Сохранено');
      } else {
        if (existing) await api.put(`/api/sources/${existing.id}`, body);
        else await api.post('/api/sources', body);
        close();
        toast(existing ? 'Сохранено, обновляем каналы…' : 'Плейлист добавлен, загружаем каналы…');
      }
      await refreshState();
    } catch (e) {
      errorBox.replaceChildren(h('div', { class: 'form-error' }, e.message));
      save.replaceChildren(existing ? 'Сохранить' : 'Добавить');
      save.disabled = false;
    }
  });

  const close = openModal(existing ? 'Изменить плейлист' : 'Новый плейлист', h('div', {},
    seg,
    errorBox,
    m3uFields,
    fileFields,
    xtreamFields,
    stalkerFields,
    h('div', { class: 'field' }, h('label', {}, 'Ссылка на программу передач (EPG)'), f.epgUrl, epgHint),
    h('div', { class: 'field' }, h('label', {}, 'Название'), f.name)),
  [h('button', { class: 'btn', onclick: () => close() }, 'Отмена'), save]);
  setTimeout(() => { if (type !== 'file') ({ m3u: f.url, xtream: f.server, stalker: f.portal })[type].focus(); }, 50);
}

// ---------------------------------------------------------------- settings

function epgCard() {
  const e = S.epg;
  let status;
  if (e.running) status = h('div', { class: 'status-line' }, spinner(), 'Обновляем программу передач…');
  else if (e.updatedAt) {
    status = h('div', {},
      h('div', { class: `status-line ${e.error ? 'error' : 'ok'}` }, icon(e.error ? 'alert' : 'check'),
        `Обновлена ${when(e.updatedAt)}: программа есть у ${e.matched} из ${channelsWord(e.wanted)}`),
      e.error && h('div', { class: 'status-line error' }, e.error));
  } else status = h('div', { class: 'status-line' }, 'Ещё не обновлялась — обновится после первой отправки на ТВ.');

  const refresh = async (btn) => {
    btn.disabled = true;
    try { await api.post('/api/epg/refresh'); } catch (err) { fail(err); }
    setTimeout(() => refreshState().catch(() => {}), 500);
  };
  const btn = h('button', { class: 'btn btn-sm', disabled: e.running }, icon('refresh'), 'Обновить сейчас');
  btn.addEventListener('click', () => refresh(btn));

  return h('div', { class: 'card section-card', id: 'epg-card' },
    h('h2', {}, 'Программа передач'),
    !S.epgImportInstalled && h('div', { class: 'form-error' },
      'На ресивере не найден плагин EPGImport. Без него программа передач на ТВ не появится. Установите его: Меню → Плагины → зелёная кнопка «Загрузить» → Extensions → EPGImport.'),
    status,
    h('div', { class: 'actions' }, btn),
    h('details', { class: 'howto' },
      h('summary', {}, 'Одноразовая настройка на ресивере'),
      h('ol', {},
        h('li', {}, 'Меню → Плагины → EPG-Importer → синяя кнопка «Источники».'),
        h('li', {}, 'Убедитесь, что стоит галочка у «Dreaminator», и сохраните.'),
        h('li', {}, 'В настройках EPG-Importer включите автоматический импорт каждый день, например в 05:00 — после ночного обновления плейлистов.'))));
}

function piconCard() {
  const p = S.picons || {};
  let status;
  if (!S.settings.picons) status = h('div', { class: 'status-line' }, 'Выключено — логотипы на ТВ не показываются.');
  else if (p.running) status = h('div', { class: 'status-line' }, spinner(), 'Обновляем логотипы…');
  else if (p.updatedAt) {
    status = h('div', {},
      h('div', { class: `status-line ${p.error ? 'error' : 'ok'}` }, icon(p.error ? 'alert' : 'check'),
        `Логотипы есть у ${p.done} из ${channelsWord(p.channels)}, обновлены ${when(p.updatedAt)}`),
      p.noLogo > 0 && h('div', { class: 'status-line' }, `У ${plural(p.noLogo, 'канала', 'каналов', 'каналов')} в плейлисте нет логотипа.`),
      p.error && h('div', { class: 'status-line error' }, p.error),
      (p.failed || []).length > 0 && h('details', { class: 'howto' },
        h('summary', {}, `Не получилось: ${(p.failed || []).length}`),
        h('ul', { style: 'margin:6px 0 0;padding-left:20px;color:var(--muted);font-size:14px' }, p.failed.map((f) => h('li', {}, f)))),
      p.dir && h('div', { class: 'status-line' }, `Папка: ${p.dir}`));
  } else status = h('div', { class: 'status-line' }, 'Появятся после первой отправки на ТВ.');
  const btn = h('button', { class: 'btn btn-sm', disabled: p.running || !S.settings.picons }, icon('refresh'), 'Обновить логотипы');
  btn.addEventListener('click', async () => {
    btn.disabled = true;
    try { await api.post('/api/picons/refresh'); } catch (e) { fail(e); }
    setTimeout(() => refreshState().catch(() => {}), 500);
  });
  return h('div', { class: 'card section-card', id: 'picon-card' },
    h('h2', {}, 'Логотипы каналов на ТВ'),
    h('p', { class: 'hint' }, 'Логотипы из плейлиста превращаются в пиконы — их показывает список каналов и инфобар на телевизоре.'),
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
  const hours = [[-1, 'Выключено'], ...Array.from({ length: 24 }, (_, i) => [i, `Каждую ночь в ${String(i).padStart(2, '0')}:00`])];
  const picons = h('input', { type: 'checkbox', checked: s.picons });
  picons.addEventListener('change', () => { s.picons = picons.checked; });
  const insecure = h('input', { type: 'checkbox', checked: s.insecureTls });
  insecure.addEventListener('change', () => { s.insecureTls = insecure.checked; });

  const save = h('button', { class: 'btn btn-primary btn-lg' }, 'Сохранить настройки');
  save.addEventListener('click', async () => {
    save.disabled = true;
    try { await api.put('/api/settings', s); await refreshState(false); toast('Настройки сохранены'); } catch (e) { fail(e); }
    save.disabled = false;
  });

  return h('section', {},
    h('div', { class: 'view-head' }, h('h1', {}, 'Настройки')),
    h('div', { class: 'card section-card' },
      h('h2', {}, 'Просмотр'),
      h('div', { class: 'field' }, h('label', {}, 'Проигрыватель на ресивере'),
        sel(s.serviceType, [[4097, 'Стандартный (GStreamer, 4097)'], [5001, 'ServiceApp: GStreamer (5001)'], [5002, 'ServiceApp: exteplayer3 (5002)']],
          (v) => { s.serviceType = Number(v); }),
        h('p', { class: 'hint' }, 'Если каналы не запускаются или заикаются — попробуйте другой. Для 5001 и 5002 нужен плагин ServiceApp.')),
      h('div', { class: 'field' }, h('label', {}, 'Формат потоков Xtream'),
        sel(s.streamFormat, [['ts', 'MPEG-TS (обычно лучше)'], ['m3u8', 'HLS (m3u8)']], (v) => { s.streamFormat = v; }),
        h('p', { class: 'hint' }, 'Применится после обновления плейлиста.')),
      h('div', { class: 'field' }, h('label', {}, 'Где показывать списки на ТВ'),
        sel(s.bouquetsOnTop ? 'top' : 'bottom', [['top', 'Перед остальными (спутник ниже)'], ['bottom', 'После остальных']],
          (v) => { s.bouquetsOnTop = v === 'top'; }))),
    h('div', { class: 'card section-card' },
      h('h2', {}, 'Обновление плейлистов'),
      h('div', { class: 'field' }, h('label', {}, 'Автоматически обновлять'), sel(s.refreshHour, hours, (v) => { s.refreshHour = Number(v); }),
        h('p', { class: 'hint' }, 'Ресивер сам скачает плейлисты и программу и обновит списки на ТВ. Работает, если ресивер не выключен полностью.'))),
    epgCard(),
    piconCard(),
    tmdbCard(),
    h('div', { class: 'card section-card' },
      h('h2', {}, 'Логотипы: настройки'),
      h('label', { class: 'check', style: 'margin-bottom:14px' }, picons, h('span', {}, 'Показывать логотипы каналов на ТВ')),
      h('div', { class: 'field' }, h('label', {}, 'Папка пиконов'), text('piconDir', { placeholder: (S.picons && S.picons.dir) || 'определится сама' }),
        h('p', { class: 'hint' }, 'Обычно оставьте пустым: Dreaminator положит логотипы туда, где Enigma2 уже ищет пиконы (например, рядом со спутниковыми на флешке).'))),
    h('div', { class: 'card section-card' },
      h('h2', {}, 'Связь с ресивером'),
      S.tabletUrl && h('p', { class: 'hint' }, 'Адрес для планшета и компьютера: ', h('strong', {}, S.tabletUrl),
        '. Его и QR-код можно посмотреть на телевизоре: Меню → Плагины → Dreaminator.'),
      h('div', { class: 'field' }, h('label', {}, 'Адрес OpenWebif'), text('webifUrl', { type: 'url', placeholder: 'http://127.0.0.1' }),
        h('p', { class: 'hint' }, 'Обычно менять не нужно: http://127.0.0.1')),
      h('div', { class: 'field-row' },
        h('div', { class: 'field' }, h('label', {}, 'Логин OpenWebif'), text('webifUser', { placeholder: 'если включён вход' })),
        h('div', { class: 'field' }, h('label', {}, 'Пароль OpenWebif'), text('webifPassword', { type: 'password' }))),
      h('label', { class: 'check' }, insecure,
        h('span', {}, 'Не проверять HTTPS-сертификаты', h('span', { class: 'hint', style: 'display:block;margin:2px 0 0' },
          'Включите, только если плейлист не скачивается из-за ошибки сертификата.')))),
    h('div', { class: 'actions' }, save),
    h('p', { class: 'hint', style: 'margin-top:24px' }, `Dreaminator ${S.version}`));
}

// ---------------------------------------------------------------- TMDB

// The key never comes back from the receiver: the card only knows whether one
// is stored and its last characters.
const TMDB_LANGUAGES = [
  ['ru-RU', 'Русский'], ['uk-UA', 'Українська'], ['be-BY', 'Беларуская'], ['kk-KZ', 'Қазақ тілі'],
  ['en-US', 'English'], ['de-DE', 'Deutsch'], ['fr-FR', 'Français'], ['es-ES', 'Español'],
  ['it-IT', 'Italiano'], ['pl-PL', 'Polski'], ['tr-TR', 'Türkçe'], ['ro-RO', 'Română'],
  ['lv-LV', 'Latviešu'], ['lt-LT', 'Lietuvių'], ['et-EE', 'Eesti'], ['he-IL', 'עברית'],
];
const TMDB_KIND = { movie: 'фильм', series: 'сериал', season: 'сезон', episode: 'серия' };

function tmdbNotice() {
  return h('div', { class: 'tmdb-notice' },
    h('p', { lang: 'en' }, 'This product uses the TMDB API but is not endorsed or certified by TMDB.'),
    h('p', {}, 'Этот продукт использует API TMDB, но не одобрен и не сертифицирован TMDB.'));
}

function tmdbCard() {
  const card = h('div', { class: 'card section-card', id: 'tmdb-card' },
    h('h2', {}, 'Описания и постеры TMDB'), h('div', { class: 'status-line' }, spinner(), 'Загружаем…'));
  api.get('/api/tmdb')
    .then((st) => fillTMDBCard(card, st))
    .catch((e) => card.replaceChildren(h('h2', {}, 'Описания и постеры TMDB'), h('div', { class: 'form-error' }, e.message)));
  return card;
}

function tmdbTestLine(res) {
  return res.ok
    ? h('div', { class: 'status-line ok' }, icon('check'), 'Подключено: TMDB принял ключ')
    : h('div', { class: 'status-line error' }, icon('alert'), res.error || 'Не удалось проверить');
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
    placeholder: st.keySet ? `Сохранён ключ ${st.keyHint || ''}` : 'Вставьте «Ключ API» (v3)',
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
  enabled.addEventListener('change', () => put({ enabled: enabled.checked }, enabled.checked ? 'TMDB включён' : 'TMDB выключен'));
  lang.addEventListener('change', () => put({ language: lang.value }, `Язык описаний: ${lang.selectedOptions[0].textContent}`));

  const saveKey = h('button', { class: 'btn btn-sm btn-primary' }, 'Сохранить ключ');
  saveKey.addEventListener('click', async () => {
    const value = key.value.trim();
    if (!value) { key.focus(); return; }
    saveKey.disabled = true;
    saveKey.replaceChildren(spinner(), 'Сохраняем…');
    let next;
    try { next = await api.put('/api/tmdb', { apiKey: value }); } catch (e) {
      fail(e);
      saveKey.disabled = false;
      saveKey.replaceChildren('Сохранить ключ');
      return;
    }
    const res = await api.post('/api/tmdb/test', {}).catch((e) => ({ ok: false, error: e.message }));
    paint(next, tmdbTestLine(res));
    toast(res.ok ? 'Ключ сохранён, TMDB подключён' : 'Ключ сохранён, но проверка не прошла', res.ok ? '' : 'error');
  });

  const test = h('button', { class: 'btn btn-sm' }, icon('pulse'), 'Проверить подключение');
  test.addEventListener('click', async () => {
    const value = key.value.trim();
    if (!value && !st.keySet) { key.focus(); return; }
    test.disabled = true;
    test.replaceChildren(spinner(), 'Проверяем…');
    const res = await api.post('/api/tmdb/test', value ? { apiKey: value } : {}).catch((e) => ({ ok: false, error: e.message }));
    result.replaceChildren(tmdbTestLine(res));
    test.disabled = false;
    test.replaceChildren(icon('pulse'), 'Проверить подключение');
  });

  const remove = st.keySet && h('button', { class: 'btn btn-sm btn-danger' }, icon('trash'), 'Удалить ключ');
  if (remove) remove.addEventListener('click', async () => {
    if (!await confirmDialog('Удалить ключ TMDB?', 'Описания и постеры TMDB перестанут показываться. Фильмы и сериалы останутся с данными провайдера.')) return;
    put({ clearKey: true }, 'Ключ удалён');
  });

  let status;
  if (!st.keySet) status = h('div', { class: 'status-line' }, 'Ключ не задан — TMDB не используется.');
  else if (!st.enabled) status = h('div', { class: 'status-line' }, `Выключено. Ключ ${st.keyHint || ''} сохранён.`);
  else status = h('div', { class: 'status-line ok' }, icon('check'), `Включено · ключ ${st.keyHint || ''} · ${langName}`);

  // Native replaceChildren would print `false`; h() skips it.
  card.replaceChildren(...[
    h('h2', {}, 'Описания и постеры TMDB'),
    h('p', { class: 'hint' }, 'Dreaminator может дополнять фильмы и сериалы порталов описаниями, постерами и оценками из TMDB (The Movie Database). ',
      'Данные используются только для описаний, постеров и оценок — смотреть фильмы и сериалы можно и без ключа.'),
    status,
    h('label', { class: 'check', style: 'margin:14px 0' }, enabled,
      h('span', {}, 'Показывать описания и постеры TMDB', h('span', { class: 'hint', style: 'display:block;margin:2px 0 0' },
        'Если TMDB недоступен или фильм не нашёлся, остаются данные провайдера.'))),
    h('div', { class: 'field' }, h('label', {}, 'Ключ API TMDB'), key,
      h('p', { class: 'hint' }, 'Бесплатный: зарегистрируйтесь на ',
        h('a', { href: 'https://www.themoviedb.org/settings/api', target: '_blank', rel: 'noopener noreferrer' }, 'themoviedb.org'),
        ' → Настройки → API и скопируйте «Ключ API». Ключ хранится только на ресивере и сюда не возвращается.',
        st.keySet ? ' Чтобы заменить его, вставьте новый.' : '')),
    h('div', { class: 'field' }, h('label', {}, 'Язык описаний'), lang),
    result,
    h('div', { class: 'actions' }, saveKey, test, remove),
    st.enabled && st.keySet && h('details', { class: 'howto' },
      h('summary', {}, 'Фильм сопоставился не с тем?'),
      h('p', { class: 'hint', style: 'margin:8px 0' }, 'Dreaminator выбирает запись TMDB, только если название и год совпали однозначно. Иначе — или если выбор неверный — сопоставьте вручную.'),
      h('button', { class: 'btn btn-sm', onclick: () => go('settings', 'tmdb') }, icon('search'), 'Сопоставить вручную')),
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
    h('button', { class: 'icon-btn back', 'aria-label': 'Назад', onclick: () => go('settings') }, icon('back')),
    h('h1', {}, 'Сопоставление с TMDB'));
  if (!st.enabled || !st.keySet) {
    return h('section', {}, head, emptyState('TMDB не подключён', 'Включите TMDB и сохраните ключ API в настройках.',
      h('button', { class: 'btn btn-primary', onclick: () => go('settings') }, 'К настройкам')));
  }
  if (!sources.length) {
    return h('section', {}, head, emptyState('Нет порталов Stalker', 'Сопоставление работает для фильмов и сериалов порталов Stalker. Добавьте портал во вкладке «Плейлисты».'));
  }

  let source = sources.some((s) => s.id === store.get('tmdbSource')) ? store.get('tmdbSource') : sources[0].id;
  let type = store.get('tmdbType') === 'series' ? 'series' : 'vod';
  let token = 0; // only the latest search or entry may paint
  const results = h('div', {});
  const detail = h('div', { class: 'tmdb-detail' });
  const q = h('input', { type: 'search', placeholder: 'Название фильма или сериала на портале', autocomplete: 'off', enterkeyhint: 'search' });
  const portal = h('select', {}, sources.map((s) => h('option', { value: s.id, selected: s.id === source }, s.name)));
  portal.addEventListener('change', () => {
    source = portal.value;
    store.set('tmdbSource', source);
    token++;
    results.replaceChildren();
    detail.replaceChildren();
  });
  const seg = h('div', { class: 'segmented', role: 'tablist' });
  const paintSeg = () => seg.replaceChildren(...[['vod', 'Фильмы'], ['series', 'Сериалы']].map(([t, label]) =>
    h('button', { type: 'button', class: type === t ? 'on' : '', onclick: () => {
      type = t;
      store.set('tmdbType', t);
      paintSeg();
      if (q.value.trim()) find();
    } }, label)));
  paintSeg();

  const reasonLine = (v) => {
    const how = v.current && { manual: 'вручную', explicit: 'по номеру TMDB от портала', auto: 'автоматически' }[v.current.match];
    switch (v.reason) {
      case 'matched': return h('div', { class: 'status-line ok' }, icon('check'), `Сопоставлено ${how || ''}`);
      case 'ambiguous': return h('div', { class: 'status-line warn' }, icon('alert'), 'Несколько похожих вариантов — выберите подходящий.');
      case 'none': return h('div', { class: 'status-line' }, 'Автоматически не нашлось — выберите вариант или поищите по другому названию.');
      case 'disabled': return h('div', { class: 'status-line' }, 'TMDB выключен.');
      default: return h('div', { class: 'status-line error' }, icon('alert'), v.error || 'TMDB сейчас недоступен — попробуйте позже.');
    }
  };

  const currentCard = (cur, poster) => {
    const meta = [cur.year, cur.type === 'tv' ? 'сериал' : 'фильм', (cur.genres || []).join(', '),
      cur.runtime && `${cur.runtime} мин`, cur.rating && `★ ${cur.rating}`].filter(Boolean).join(' · ');
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
      const choose = h('button', { class: `btn btn-sm${isCurrent(c) ? '' : ' btn-primary'}`, disabled: isCurrent(c) }, isCurrent(c) ? 'Выбрано' : 'Выбрать');
      choose.addEventListener('click', async () => {
        choose.disabled = true;
        try {
          await api.put('/api/tmdb/match', { source, key: it.key, id: c.id, type: c.type });
          toast(`Выбрано: ${c.title}`);
          openEntry(it);
        } catch (e) {
          fail(e);
          choose.disabled = false;
        }
      });
      return h('div', { class: 'row tmdb-row' }, posterThumb(c.poster),
        h('div', { class: 'row-main' },
          h('div', { class: 'row-title' }, c.title, c.year ? ` (${c.year})` : ''),
          h('div', { class: 'row-sub' }, [c.originalTitle, c.type === 'tv' ? 'сериал' : 'фильм', `совпадение ${c.score}`].filter(Boolean).join(' · ')),
          c.overview && h('div', { class: 'tmdb-overview' }, c.overview)),
        choose);
    };
    const paintCands = (list, title) => cands.replaceChildren(
      h('h3', { class: 'tmdb-subhead' }, title),
      list.length ? h('div', { class: 'list' }, list.map(candRow)) : h('p', { class: 'hint' }, 'TMDB ничего не предложил.'));
    paintCands(v.candidates, 'Варианты TMDB');

    const fq = h('input', { type: 'search', value: v.item.originalName || v.item.name, autocomplete: 'off', enterkeyhint: 'search', placeholder: 'Название или ссылка на themoviedb.org' });
    const ftype = h('select', { 'aria-label': 'Что искать' },
      h('option', { value: 'movie', selected: v.item.kind === 'movie' }, 'Фильм'),
      h('option', { value: 'tv', selected: v.item.kind !== 'movie' }, 'Сериал'));
    const fbtn = h('button', { class: 'btn' }, icon('search'), 'Найти в TMDB…');
    const tmdbFind = async () => {
      const text = fq.value.trim();
      if (!text) { fq.focus(); return; }
      fbtn.disabled = true;
      try {
        const res = await api.get(`/api/tmdb/search?q=${encodeURIComponent(text)}&type=${ftype.value}&year=${encodeURIComponent(v.item.year || '')}`);
        paintCands(res.candidates || [], `TMDB по запросу «${text}»`);
      } catch (e) { fail(e); }
      fbtn.disabled = false;
    };
    fbtn.addEventListener('click', tmdbFind);
    fq.addEventListener('keydown', (e) => { if (e.key === 'Enter') tmdbFind(); });

    const reset = v.manual && h('button', { class: 'btn btn-sm' }, icon('refresh'), 'Сбросить на автоматическое');
    if (reset) reset.addEventListener('click', async () => {
      reset.disabled = true;
      try {
        await api.del(`/api/tmdb/match?source=${encodeURIComponent(source)}&key=${encodeURIComponent(it.key)}`);
        toast('Вернули автоматический выбор');
        openEntry(it);
      } catch (e) {
        fail(e);
        reset.disabled = false;
      }
    });
    const poster = v.current && (v.candidates.find(isCurrent) || {}).poster;
    return h('div', { class: 'card' },
      h('h2', {}, v.item.name),
      h('div', { class: 'row-sub' }, [v.item.originalName, v.item.year, TMDB_KIND[v.item.kind]].filter(Boolean).join(' · ')),
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
      detail.replaceChildren(h('div', { class: 'form-error' }, e.status === 410 ? 'Список на портале обновился — найдите фильм заново.' : e.message));
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
            h('div', { class: 'row-sub' }, [TMDB_KIND[it.kind], it.year].filter(Boolean).join(' · '))),
          icon('chevron'))))
        : h('p', { class: 'hint' }, 'На портале ничего не нашлось. Попробуйте часть названия.'));
    } catch (e) {
      if (my !== token) return;
      results.replaceChildren(h('div', { class: 'form-error' }, e.status === 404 && !e.json
        ? 'Поиск по порталу появится в следующей версии Dreaminator на ресивере.'
        : e.message));
    }
  };
  q.addEventListener('keydown', (e) => { if (e.key === 'Enter') find(); });

  return h('section', {}, head,
    h('p', { class: 'hint' }, 'Найдите фильм или сериал портала и выберите для него запись TMDB. Выбор хранится на ресивере и переживает перезапуск.'),
    h('div', { class: 'card section-card' },
      sources.length > 1 && h('div', { class: 'field' }, h('label', {}, 'Портал'), portal),
      seg,
      h('div', { class: 'tmdb-search' }, q, h('button', { class: 'btn btn-primary', onclick: find }, icon('search'), 'Найти'))),
    results,
    detail,
    tmdbNotice());
}

// ---------------------------------------------------------------- boot

for (const a of document.querySelectorAll('.nav a')) a.prepend(icon(a.dataset.icon));
// A stale search would hide the group or playlist the user just opened.
window.addEventListener('hashchange', () => { ui.q = ''; window.scrollTo(0, 0); render(); });

(async function boot() {
  try {
    await refreshState(false);
  } catch (e) {
    $('#view').replaceChildren(emptyState('Нет связи с Dreaminator', 'Проверьте, что ресивер включён и планшет в той же сети Wi-Fi.',
      h('button', { class: 'btn btn-primary', onclick: () => location.reload() }, 'Повторить')));
    return;
  }
  render();
  pollState();
  pollBox();
})();
