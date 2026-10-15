// Existing Enigma2 bouquets stay outside Dreaminator playlist state. Keep a
// draft per file so polling/navigation cannot silently discard an edit.
export function receiverUI({ h, api, icon, grip, makeSortable, go, render, toast, fail, promptDialog, confirmDialog, emptyState, plural }) {
  const drafts = new Map();
  const views = new Map();
  const entries = (n) => plural(n, 'запись', 'записи', 'записей');
  const path = (id) => `/api/receiver-bouquets/${encodeURIComponent(id)}`;
  const changed = (d) => d.name !== d.saved.name || JSON.stringify(d.channels.map((c) => c.id)) !== JSON.stringify((d.saved.channels || []).map((c) => c.id));
  const draft = (b, warning = '') => ({ saved: b, name: b.name, channels: [...(b.channels || [])], warning, busy: false });
  // Set when a list action changed files but Enigma2 did not re-read them.
  let listWarning = '';
  window.addEventListener('beforeunload', (event) => {
    if ([...drafts.values()].some(changed)) { event.preventDefault(); event.returnValue = ''; }
  });

  const confirmRemove = (b) => {
    const d = drafts.get(b.id);
    return confirmDialog('Удалить букет с ресивера?', b.error
      ? `Файл букета «${b.name}» недоступен. Если его нет на ресивере, запись о нём уберётся из списка букетов.`
      : `Букет «${b.name}» (${entries(b.count)}) пропадёт с телевизора. Если его создал плагин провайдера, тот может вернуть букет при следующем обновлении.${d && changed(d) ? ' Несохранённый черновик тоже пропадёт.' : ''}`);
  };
  // Resolves to true once the bouquet is gone from the receiver.
  const remove = async (b) => {
    try {
      const result = await api.del(`${path(b.id)}?revision=${encodeURIComponent(b.revision || '')}`);
      drafts.delete(b.id);
      views.delete(b.id);
      listWarning = result.warning || '';
      toast(listWarning ? 'Удалено. Нужно повторить обновление на ТВ.' : `Букет «${b.name}» удалён`);
      return true;
    } catch (e) { fail(e); return false; }
  };
  const rename = async (b) => {
    const name = await promptDialog('Переименовать букет', 'Новое название', b.name);
    if (!name || name === b.name) return;
    try {
      const result = await api.post(`${path(b.id)}/rename`, { revision: b.revision, name });
      // Renaming keeps entry ids, so a draft of the same revision stays valid.
      const d = drafts.get(b.id);
      if (d && d.saved.revision === b.revision) { d.saved = result.bouquet; d.name = result.bouquet.name; }
      listWarning = result.warning || '';
      toast(listWarning ? 'Переименовано. Нужно повторить обновление на ТВ.' : 'Букет переименован');
    } catch (e) { fail(e); }
    render();
  };

  async function list() {
    let bouquets;
    try { bouquets = await api.get('/api/receiver-bouquets'); }
    catch (e) { return h('div', { class: 'warn-box' }, `Не удалось прочитать букеты ресивера: ${e.message}`); }
    const retry = h('button', { class: 'btn', onclick: async () => {
      retry.disabled = true;
      try { await api.post('/api/receiver-bouquets/reload'); listWarning = ''; toast('Списки обновлены на телевизоре'); }
      catch (e) { fail(e); }
      render();
    } }, 'Повторить обновление на ТВ');
    return h('div', { class: 'receiver-section' },
      h('h2', {}, 'Букеты ресивера'),
      h('p', { class: 'hint' }, 'Существующие списки из OpenWebif и других плагинов. Откройте букет для редактирования или создания своей копии.'),
      listWarning && h('div', { class: 'warn-box' }, h('span', {}, listWarning), retry),
      bouquets.length ? h('div', { class: 'cards' }, bouquets.map((b) =>
        h('div', { class: 'card list-card' },
          h('button', { class: 'list-open', disabled: !!b.error, onclick: () => go('lists', 'receiver', b.id) },
            h('div', { class: 'row-main' },
              h('div', { class: 'row-title' }, b.name),
              h('div', { class: 'row-sub' }, b.error || `${entries(b.count)}${drafts.has(b.id) && changed(drafts.get(b.id)) ? ' · есть черновик' : ''}`)), icon('chevron')),
          !b.error && h('button', { class: 'icon-btn', 'aria-label': `Переименовать ${b.name}`, onclick: () => rename(b) }, icon('edit')),
          h('button', { class: 'icon-btn danger', 'aria-label': `Удалить ${b.name}`, onclick: async () => {
            if (!await confirmRemove(b)) return;
            await remove(b);
            render();
          } }, icon('trash')))))
        : h('p', { class: 'hint' }, 'Других ТВ-букетов на ресивере пока нет.'));
  }

  async function detail(id) {
    if (!drafts.has(id)) drafts.set(id, draft(await api.get(path(id))));
    const d = drafts.get(id);
    const redraw = () => views.get(id)?.();
    const section = h('section', { class: 'receiver-editor' });
    const reloadDraft = async () => {
      if (d.busy) return;
      if (changed(d) && !await confirmDialog('Отбросить черновик?', 'Изменения в этой вкладке ещё не сохранены на ТВ.', 'Отбросить')) return;
      d.busy = true; redraw();
      try { Object.assign(d, draft(await api.get(path(id)), d.warning)); }
      catch (e) { fail(e); }
      finally { d.busy = false; redraw(); }
    };
    const run = async (action, message) => {
      if (d.busy) return;
      d.busy = true; redraw();
      try {
        const result = await action();
        // Save succeeded even when OpenWebif could not reload. Keep that
        // distinction visible; retry only the reload, never the mutation.
        let b = result.bouquet;
        try { b = await api.get(path(b.id)); } catch { /* use saved response */ }
        const next = draft(b, result.warning || '');
        if (b.id === id) Object.assign(d, next);
        else drafts.set(b.id, next);
        toast(result.warning ? 'Сохранено. Нужно повторить обновление на ТВ.' : message);
        if (b.id !== id) { d.busy = false; go('lists', 'receiver', b.id); }
      } catch (e) { fail(e); }
      finally { d.busy = false; redraw(); }
    };
    const save = () => run(() => api.put(path(id), { revision: d.saved.revision, name: d.name, order: d.channels.map((c) => c.id) }), 'Букет сохранён на телевизоре');
    const copy = async () => {
      if (d.busy || changed(d)) return;
      const name = await promptDialog('Создать копию', 'Название отдельного букета', `${d.saved.name} — копия`, 'Создать на ТВ');
      if (name) await run(() => api.post(`${path(id)}/copy`, { revision: d.saved.revision, name }), 'Копия создана на телевизоре');
    };
    const removeBouquet = async () => {
      if (d.busy || !await confirmRemove(d.saved)) return;
      d.busy = true; redraw();
      if (await remove(d.saved)) { go('lists'); return; }
      d.busy = false; redraw();
    };
    const undo = async () => {
      if (d.busy || changed(d)) return;
      if (await confirmDialog('Отменить последнее сохранение?', 'Предыдущая версия этого букета будет восстановлена на ТВ. Другие букеты останутся без изменений.', 'Восстановить')) {
        await run(() => api.post(`${path(id)}/undo`, { revision: d.saved.revision }), 'Предыдущая версия восстановлена');
      }
    };
    const draw = () => {
      const dirty = changed(d);
      const saveBtn = h('button', { class: 'btn btn-good', disabled: d.busy || !dirty, onclick: save }, icon('send'), d.busy ? 'Сохраняем…' : 'Сохранить на ТВ');
      const status = h('span', { class: 'hint' }, dirty ? 'Есть изменения в черновике' : 'Все изменения сохранены');
      const copyBtn = h('button', { class: 'btn', disabled: d.busy || dirty, onclick: copy }, icon('plus'), 'Создать копию');
      const undoBtn = h('button', { class: 'btn', disabled: d.busy || dirty || !d.saved.canUndo, onclick: undo }, icon('back'), 'Отменить последнее сохранение');
      const nameInput = h('input', { id: 'receiver-name', type: 'text', value: d.name, maxlength: 512, disabled: d.busy, oninput: (e) => {
        d.name = e.target.value;
        const dirty = changed(d);
        saveBtn.disabled = d.busy || !dirty;
        copyBtn.disabled = d.busy || dirty;
        undoBtn.disabled = d.busy || dirty || !d.saved.canUndo;
        status.textContent = dirty ? 'Есть изменения в черновике' : 'Все изменения сохранены';
      } });
      const rows = h('div', { class: 'list', inert: d.busy }, d.channels.map((c) =>
        h('div', { class: 'row sortable-item', 'data-id': c.id }, grip(),
          h('div', { class: 'row-main' }, h('div', { class: 'row-title' }, c.name),
            c.kind !== 'channel' && h('div', { class: 'row-sub' }, c.kind === 'marker' ? 'Разделитель' : 'Вложенный список')),
          h('button', { class: 'icon-btn danger', disabled: d.busy, 'aria-label': `Убрать ${c.name}`, onclick: () => {
            d.channels = d.channels.filter((x) => x.id !== c.id); redraw();
          } }, icon('x')))));
      makeSortable(rows, (ids) => {
        if (!d.busy) {
          const byID = new Map(d.channels.map((c) => [c.id, c]));
          d.channels = ids.map((key) => byID.get(key));
        }
        redraw();
      });
      section.replaceChildren(
        h('div', { class: 'view-head' },
          h('button', { class: 'icon-btn back', 'aria-label': 'Все списки', onclick: () => go('lists') }, icon('back')),
          h('h1', {}, d.saved.name),
          h('button', { class: 'icon-btn danger', disabled: d.busy, 'aria-label': 'Удалить букет', onclick: removeBouquet }, icon('trash'))),
        h('p', { class: 'hint' }, 'Букет ресивера. Другой плагин может заменить его при обновлении — создайте копию, если хотите вести свой список.'),
        h('div', { class: 'card' },
          h('label', { for: 'receiver-name', class: 'field' }, h('span', {}, 'Название букета'), nameInput),
          h('div', { class: 'actions' }, copyBtn, undoBtn),
          dirty && h('p', { class: 'hint' }, 'Для копирования или отката сначала сохраните либо отбросьте черновик.')),
        d.warning ? h('div', { class: 'warn-box' }, h('span', {}, d.warning),
          h('button', { class: 'btn', disabled: d.busy, onclick: async () => {
            d.busy = true; redraw();
            try { await api.post('/api/receiver-bouquets/reload'); d.warning = ''; toast('Списки обновлены на телевизоре'); }
            catch (e) { fail(e); }
            finally { d.busy = false; redraw(); }
          } }, 'Повторить обновление на ТВ')) : h('span'),
        h('div', { class: 'receiver-save actions' }, saveBtn,
          h('button', { class: 'btn', disabled: d.busy, onclick: reloadDraft }, icon('refresh'), dirty ? 'Отбросить черновик' : 'Перечитать с ресивера'), status),
        h('p', { class: 'hint' }, `${entries(d.channels.length)}. Перетащите за ⋮⋮, чтобы поменять порядок; × убирает запись из черновика.`),
        d.channels.length ? rows : emptyState('Букет пуст', 'После сохранения в нём не останется каналов.'));
    };
    views.set(id, draw);
    redraw();
    return section;
  }
  return { list, detail };
}
