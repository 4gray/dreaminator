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
  if (!res.ok) throw new Error((data && data.error) || `Ошибка ${res.status}`);
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
  const [tab] = route();
  if (tab === 'sources' || tab === 'lists') render();
  else if (tab === 'channels' && wasBusy && !busy()) render();
  else if (tab === 'settings') { const card = $('#epg-card'); if (card) card.replaceWith(epgCard()); }
}

const busy = () => !!S && (S.sources.some((s) => s.refreshing) || S.epg.running);

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
      h('div', { class: 'applybar-text' }, h('strong', {}, 'Есть изменения'), h('span', {}, 'Они появятся на телевизоре после отправки')),
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
      case 'lists': el = rest[0] ? viewBouquet(rest[0]) : viewBouquets(); break;
      case 'sources': el = viewSources(); break;
      case 'settings': el = viewSettings(); break;
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
    starButton(ch));
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

function viewBouquets() {
  const list = h('div', { class: 'cards' }, S.bouquets.map((b) => {
    const missing = b.channels.filter((c) => c.missing).length;
    return h('div', { class: 'card list-card sortable-item', 'data-id': b.id },
      grip(),
      h('button', { class: 'list-open', onclick: () => go('lists', b.id) },
        h('div', { class: 'row-main' },
          h('div', { class: 'row-title' }, b.name),
          h('div', { class: 'row-sub' }, b.channels.length ? channelsWord(b.channels.length) : 'пустой — на ТВ не показывается',
            missing ? ` · ${missing} недоступно` : '')),
        icon('chevron')));
  }));
  makeSortable(list, async (ids) => {
    try { await api.put('/api/bouquet-order', { ids }); } catch (e) { fail(e); }
    refreshState().catch(() => {});
  });
  const create = async () => { const b = await createBouquetFlow(); if (b) go('lists', b.id); };
  return h('section', {},
    h('div', { class: 'view-head' }, h('h1', {}, 'Мои списки'),
      h('button', { class: 'btn', onclick: create }, icon('plus'), 'Новый список')),
    h('p', { class: 'hint' }, 'Так списки будут выглядеть на телевизоре. Чтобы поменять порядок, потяните за ⋮⋮.'),
    S.bouquets.length ? list : emptyState('Списков нет', 'Создайте первый список, например «Избранное».'));
}

function viewBouquet(id) {
  const b = S.bouquets.find((x) => x.id === id);
  if (!b) return emptyState('Список не найден', 'Возможно, его уже удалили.', h('button', { class: 'btn', onclick: () => go('lists') }, 'К спискам'));

  const pick = () => { setTarget(b.id); go('channels'); };
  const rename = async () => {
    const name = await promptDialog('Переименовать список', 'Новое название', b.name);
    if (!name) return;
    try { await api.put(`/api/bouquets/${b.id}`, { name }); await refreshState(); } catch (e) { fail(e); }
  };
  const remove = async () => {
    if (!await confirmDialog('Удалить список?', `Список «${b.name}» пропадёт с телевизора после отправки. Каналы в плейлисте останутся.`)) return;
    try { await api.del(`/api/bouquets/${b.id}`); await refreshState(false); go('lists'); } catch (e) { fail(e); }
  };
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
      h('button', { class: 'icon-btn', 'aria-label': 'Переименовать', onclick: rename }, icon('edit')),
      h('button', { class: 'icon-btn danger', 'aria-label': 'Удалить список', onclick: remove }, icon('trash'))),
    b.channels.length
      ? [h('p', { class: 'hint' }, `${channelsWord(b.channels.length)}. Порядок меняется перетаскиванием за ⋮⋮.`), list,
        h('div', { class: 'actions' }, h('button', { class: 'btn', onclick: pick }, icon('plus'), 'Добавить каналы'))]
      : emptyState('В списке пока пусто', 'Откройте каналы и нажмите звёздочку у тех, что нужны в этом списке.',
        h('button', { class: 'btn btn-primary btn-lg', onclick: pick }, icon('plus'), 'Выбрать каналы')));
}

// ---------------------------------------------------------------- sources

function viewSources() {
  const cards = S.sources.map((s) => {
    let status;
    if (s.refreshing) status = h('div', { class: 'status-line' }, spinner(), 'Загружаем каналы…');
    else if (s.error) status = h('div', { class: 'status-line error' }, icon('alert'), s.error);
    else if (s.updatedAt) status = h('div', { class: 'status-line ok' }, icon('check'),
      `${channelsWord(s.channelCount)} · ${s.type === 'file' ? 'загружен' : 'обновлён'} ${when(s.updatedAt)}`);
    const isFile = s.type === 'file';
    const guides = s.epgUrl ? s.epgUrl.split(/[,\s]+/).filter(Boolean).length
      : (s.detectedEpgUrls || []).length || (s.detectedEpgUrl ? 1 : 0);
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
    return h('div', { class: 'card' },
      h('h2', {}, s.name, ' ', h('span', { class: 'badge' }, SOURCE_TYPES[s.type] || s.type)),
      status,
      h('div', { class: 'status-line' }, !guides ? 'Программа передач не указана'
        : guides === 1 ? 'Программа передач подключена'
        : `Программа передач: ${plural(guides, 'источник', 'источника', 'источников')}, по порядку`),
      isFile && h('div', { class: 'status-line' }, 'Сам не обновляется: если провайдер пришлёт новый файл, загрузите его сюда.'),
      h('div', { class: 'actions' },
        isFile
          ? h('button', { class: 'btn btn-sm', onclick: () => sourceForm(s) }, icon('upload'), 'Загрузить новый файл')
          : refreshBtn,
        h('button', { class: 'btn btn-sm', onclick: () => sourceForm(s) }, icon('edit'), 'Изменить'),
        h('button', { class: 'btn btn-sm btn-danger', onclick: remove }, icon('trash'), 'Удалить')));
  });
  return h('section', {},
    h('div', { class: 'view-head' }, h('h1', {}, 'Плейлисты'),
      h('button', { class: 'btn btn-primary', onclick: () => sourceForm() }, icon('plus'), 'Добавить')),
    S.sources.length
      ? h('div', { class: 'cards' }, cards)
      : emptyState('Плейлистов пока нет', 'Понадобится ссылка на M3U, файл M3U или адрес, логин и пароль Xtream от вашего IPTV-провайдера.',
        h('button', { class: 'btn btn-primary btn-lg', onclick: () => sourceForm() }, icon('plus'), 'Добавить плейлист')));
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

const SOURCE_TYPES = { m3u: 'Ссылка M3U', file: 'Файл M3U', xtream: 'Xtream' };

function sourceForm(existing) {
  let type = existing ? existing.type : 'm3u';
  // An uploaded playlist cannot turn into a link (and vice versa): there
  // would be nothing to read.
  const types = !existing ? ['m3u', 'file', 'xtream'] : existing.type === 'file' ? ['file'] : ['m3u', 'xtream'];
  let file = null;
  const f = {
    name: h('input', { type: 'text', value: existing ? existing.name : '', placeholder: 'Например: Основной' }),
    url: h('input', { type: 'url', value: existing ? existing.url || '' : '', placeholder: 'http://…/playlist.m3u', inputmode: 'url', autocomplete: 'off' }),
    server: h('input', { type: 'url', value: existing ? existing.server || '' : '', placeholder: 'http://server:8080', inputmode: 'url', autocomplete: 'off' }),
    username: h('input', { type: 'text', value: existing ? existing.username || '' : '', autocomplete: 'off', autocapitalize: 'off' }),
    password: h('input', { type: 'text', value: existing ? existing.password || '' : '', autocomplete: 'off', autocapitalize: 'off' }),
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
    if (file && !f.name.value.trim()) f.name.value = file.name.replace(/\.[^.]+$/, '');
  });
  const fileFields = h('div', {},
    h('div', { class: 'field' }, h('span', { class: 'label' }, 'Файл плейлиста'),
      h('label', { class: 'filepick' }, icon('upload'), fileLabel, fileInput),
      h('p', { class: 'hint' }, 'Файл сохранится на ресивере. Сам по ночам он не обновляется: если провайдер пришлёт новый, загрузите его здесь же.')));
  const xtreamFields = h('div', {},
    h('div', { class: 'field' }, h('label', {}, 'Адрес сервера'), f.server),
    h('div', { class: 'field-row' },
      h('div', { class: 'field' }, h('label', {}, 'Логин'), f.username),
      h('div', { class: 'field' }, h('label', {}, 'Пароль'), f.password)));

  const epgHint = h('p', { class: 'hint' });
  const seg = h('div', { class: 'segmented', role: 'tablist' });
  const paint = () => {
    seg.style.gridTemplateColumns = `repeat(${types.length}, 1fr)`;
    seg.hidden = types.length < 2;
    seg.replaceChildren(...types.map((t) =>
      h('button', { class: type === t ? 'on' : '', type: 'button', onclick: () => { type = t; paint(); } }, SOURCE_TYPES[t])));
    m3uFields.hidden = type !== 'm3u';
    fileFields.hidden = type !== 'file';
    xtreamFields.hidden = type !== 'xtream';
    epgHint.textContent = type === 'xtream'
      ? 'Можно не заполнять — возьмём программу у провайдера Xtream.'
      : 'Необязательно: если ссылки есть в самом плейлисте, подхватим их сами. Можно несколько через запятую — каналы ищутся по порядку.';
  };
  paint();

  f.url.addEventListener('input', () => {
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
    const body = {
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
    h('div', { class: 'field' }, h('label', {}, 'Ссылка на программу передач (EPG)'), f.epgUrl, epgHint),
    h('div', { class: 'field' }, h('label', {}, 'Название'), f.name)),
  [h('button', { class: 'btn', onclick: () => close() }, 'Отмена'), save]);
  setTimeout(() => { if (type !== 'file') (type === 'm3u' ? f.url : f.server).focus(); }, 50);
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
