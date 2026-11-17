const DEFAULT_STRINGS = {
  rc_delete_title: 'Удалить букет с ресивера?',
  rc_delete_missing_text: 'Файл букета «{name}» недоступен. Если его нет на ресивере, запись о нём уберётся из списка букетов.',
  rc_delete_text: 'Букет «{name}» ({entries}) пропадёт с телевизора. Если его создал плагин провайдера, тот может вернуть букет при следующем обновлении.',
  rc_delete_draft_warn: ' Несохранённый черновик тоже пропадёт.',
  rc_deleted_reload: 'Удалено. Нужно повторить обновление на ТВ.',
  rc_deleted: 'Букет «{name}» удалён',
  rc_rename_title: 'Переименовать букет',
  new_name: 'Новое название',
  rc_renamed_reload: 'Переименовано. Нужно повторить обновление на ТВ.',
  rc_renamed: 'Букет переименован',
  rc_read_failed: 'Не удалось прочитать букеты ресивера: {msg}',
  rc_reloaded: 'Списки обновлены на телевизоре',
  rc_retry_reload: 'Повторить обновление на ТВ',
  receiver_bouquets: 'Букеты ресивера',
  rc_section_hint: 'Существующие списки из OpenWebif и других плагинов. Откройте букет для редактирования или создания своей копии.',
  rename_aria: 'Переименовать {name}',
  delete_aria: 'Удалить {name}',
  rc_has_draft: ' · есть черновик',
  rc_none: 'Других ТВ-букетов на ресивере пока нет.',
  rc_discard_title: 'Отбросить черновик?',
  rc_discard_text: 'Изменения в этой вкладке ещё не сохранены на ТВ.',
  rc_discard: 'Отбросить',
  rc_saved_reload: 'Сохранено. Нужно повторить обновление на ТВ.',
  rc_saved: 'Букет сохранён на телевизоре',
  rc_create_copy: 'Создать копию',
  rc_copy_prompt: 'Название отдельного букета',
  rc_copy_default: '{name} — копия',
  rc_create_on_tv: 'Создать на ТВ',
  rc_copy_created: 'Копия создана на телевизоре',
  rc_undo_title: 'Отменить последнее сохранение?',
  rc_undo_text: 'Предыдущая версия этого букета будет восстановлена на ТВ. Другие букеты останутся без изменений.',
  restore: 'Восстановить',
  rc_restored: 'Предыдущая версия восстановлена',
  saving: 'Сохраняем…',
  rc_save_to_tv: 'Сохранить на ТВ',
  rc_draft_dirty: 'Есть изменения в черновике',
  rc_all_saved: 'Все изменения сохранены',
  rc_undo_btn: 'Отменить последнее сохранение',
  rc_marker: 'Разделитель',
  rc_nested: 'Вложенный список',
  remove_aria: 'Убрать {name}',
  all_lists: 'Все списки',
  rc_delete_bouquet: 'Удалить букет',
  rc_editor_hint: 'Букет ресивера. Другой плагин может заменить его при обновлении — создайте копию, если хотите вести свой список.',
  rc_bouquet_name: 'Название букета',
  rc_draft_blocks_actions: 'Для копирования или отката сначала сохраните либо отбросьте черновик.',
  rc_discard_btn: 'Отбросить черновик',
  rc_reload_from_box: 'Перечитать с ресивера',
  rc_entries_hint: '{entries}. Перетащите за ⋮⋮, чтобы поменять порядок; × убирает запись из черновика.',
  rc_empty_title: 'Букет пуст',
  rc_empty_text: 'После сохранения в нём не останется каналов.',
};

function defaultT(key, vars) {
  let str = DEFAULT_STRINGS[key] || key;
  if (vars) {
    for (const [k, v] of Object.entries(vars)) {
      str = str.replaceAll(`{${k}}`, String(v));
    }
  }
  return str;
}

// Existing Enigma2 bouquets stay outside Dreaminator playlist state. Keep a
// draft per file so polling/navigation cannot silently discard an edit.
export function receiverUI({ h, api, icon, grip, makeSortable, go, render, toast, fail, promptDialog, confirmDialog, emptyState, plural, t: tFn, entriesWord: entriesWordFn }) {
  const t = tFn || defaultT;
  const drafts = new Map();
  const views = new Map();
  const entries = (n) => (entriesWordFn ? entriesWordFn(n) : plural(n, 'запись', 'записи', 'записей'));
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
    return confirmDialog(t('rc_delete_title'), b.error
      ? t('rc_delete_missing_text', { name: b.name })
      : t('rc_delete_text', { name: b.name, entries: entries(b.count) }) + (d && changed(d) ? t('rc_delete_draft_warn') : ''));
  };
  // Resolves to true once the bouquet is gone from the receiver.
  const remove = async (b) => {
    try {
      const result = await api.del(`${path(b.id)}?revision=${encodeURIComponent(b.revision || '')}`);
      drafts.delete(b.id);
      views.delete(b.id);
      listWarning = result.warning || '';
      toast(listWarning ? t('rc_deleted_reload') : t('rc_deleted', { name: b.name }));
      return true;
    } catch (e) { fail(e); return false; }
  };
  const rename = async (b) => {
    const name = await promptDialog(t('rc_rename_title'), t('new_name'), b.name);
    if (!name || name === b.name) return;
    try {
      const result = await api.post(`${path(b.id)}/rename`, { revision: b.revision, name });
      // Renaming keeps entry ids, so a draft of the same revision stays valid.
      const d = drafts.get(b.id);
      if (d && d.saved.revision === b.revision) { d.saved = result.bouquet; d.name = result.bouquet.name; }
      listWarning = result.warning || '';
      toast(listWarning ? t('rc_renamed_reload') : t('rc_renamed'));
    } catch (e) { fail(e); }
    render();
  };

  async function list() {
    let bouquets;
    try { bouquets = await api.get('/api/receiver-bouquets'); }
    catch (e) { return h('div', { class: 'warn-box' }, t('rc_read_failed', { msg: e.message })); }
    const retry = h('button', { class: 'btn', onclick: async () => {
      retry.disabled = true;
      try { await api.post('/api/receiver-bouquets/reload'); listWarning = ''; toast(t('rc_reloaded')); }
      catch (e) { fail(e); }
      render();
    } }, t('rc_retry_reload'));
    return h('div', { class: 'receiver-section' },
      h('h2', {}, t('receiver_bouquets')),
      h('p', { class: 'hint' }, t('rc_section_hint')),
      listWarning && h('div', { class: 'warn-box' }, h('span', {}, listWarning), retry),
      bouquets.length ? h('div', { class: 'cards' }, bouquets.map((b) =>
        h('div', { class: 'card list-card' },
          h('button', { class: 'list-open', disabled: !!b.error, onclick: () => go('lists', 'receiver', b.id) },
            h('div', { class: 'row-main' },
              h('div', { class: 'row-title' }, b.name),
              h('div', { class: 'row-sub' }, b.error || `${entries(b.count)}${drafts.has(b.id) && changed(drafts.get(b.id)) ? t('rc_has_draft') : ''}`)), icon('chevron')),
          !b.error && h('button', { class: 'icon-btn', 'aria-label': t('rename_aria', { name: b.name }), onclick: () => rename(b) }, icon('edit')),
          h('button', { class: 'icon-btn danger', 'aria-label': t('delete_aria', { name: b.name }), onclick: async () => {
            if (!await confirmRemove(b)) return;
            await remove(b);
            render();
          } }, icon('trash')))))
        : h('p', { class: 'hint' }, t('rc_none')));
  }

  async function detail(id) {
    if (!drafts.has(id)) drafts.set(id, draft(await api.get(path(id))));
    const d = drafts.get(id);
    const redraw = () => views.get(id)?.();
    const section = h('section', { class: 'receiver-editor' });
    const reloadDraft = async () => {
      if (d.busy) return;
      if (changed(d) && !await confirmDialog(t('rc_discard_title'), t('rc_discard_text'), t('rc_discard'))) return;
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
        toast(result.warning ? t('rc_saved_reload') : message);
        if (b.id !== id) { d.busy = false; go('lists', 'receiver', b.id); }
      } catch (e) { fail(e); }
      finally { d.busy = false; redraw(); }
    };
    const save = () => run(() => api.put(path(id), { revision: d.saved.revision, name: d.name, order: d.channels.map((c) => c.id) }), t('rc_saved'));
    const copy = async () => {
      if (d.busy || changed(d)) return;
      const name = await promptDialog(t('rc_create_copy'), t('rc_copy_prompt'), t('rc_copy_default', { name: d.saved.name }), t('rc_create_on_tv'));
      if (name) await run(() => api.post(`${path(id)}/copy`, { revision: d.saved.revision, name }), t('rc_copy_created'));
    };
    const removeBouquet = async () => {
      if (d.busy || !await confirmRemove(d.saved)) return;
      d.busy = true; redraw();
      if (await remove(d.saved)) { go('lists'); return; }
      d.busy = false; redraw();
    };
    const undo = async () => {
      if (d.busy || changed(d)) return;
      if (await confirmDialog(t('rc_undo_title'), t('rc_undo_text'), t('restore'))) {
        await run(() => api.post(`${path(id)}/undo`, { revision: d.saved.revision }), t('rc_restored'));
      }
    };
    const draw = () => {
      const dirty = changed(d);
      const saveBtn = h('button', { class: 'btn btn-good', disabled: d.busy || !dirty, onclick: save }, icon('send'), d.busy ? t('saving') : t('rc_save_to_tv'));
      const status = h('span', { class: 'hint' }, dirty ? t('rc_draft_dirty') : t('rc_all_saved'));
      const copyBtn = h('button', { class: 'btn', disabled: d.busy || dirty, onclick: copy }, icon('plus'), t('rc_create_copy'));
      const undoBtn = h('button', { class: 'btn', disabled: d.busy || dirty || !d.saved.canUndo, onclick: undo }, icon('back'), t('rc_undo_btn'));
      const nameInput = h('input', { id: 'receiver-name', type: 'text', value: d.name, maxlength: 512, disabled: d.busy, oninput: (e) => {
        d.name = e.target.value;
        const dirty = changed(d);
        saveBtn.disabled = d.busy || !dirty;
        copyBtn.disabled = d.busy || dirty;
        undoBtn.disabled = d.busy || dirty || !d.saved.canUndo;
        status.textContent = dirty ? t('rc_draft_dirty') : t('rc_all_saved');
      } });
      const rows = h('div', { class: 'list', inert: d.busy }, d.channels.map((c) =>
        h('div', { class: 'row sortable-item', 'data-id': c.id }, grip(),
          h('div', { class: 'row-main' }, h('div', { class: 'row-title' }, c.name),
            c.kind !== 'channel' && h('div', { class: 'row-sub' }, c.kind === 'marker' ? t('rc_marker') : t('rc_nested'))),
          h('button', { class: 'icon-btn danger', disabled: d.busy, 'aria-label': t('remove_aria', { name: c.name }), onclick: () => {
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
          h('button', { class: 'icon-btn back', 'aria-label': t('all_lists'), onclick: () => go('lists') }, icon('back')),
          h('h1', {}, d.saved.name),
          h('button', { class: 'icon-btn danger', disabled: d.busy, 'aria-label': t('rc_delete_bouquet'), onclick: removeBouquet }, icon('trash'))),
        h('p', { class: 'hint' }, t('rc_editor_hint')),
        h('div', { class: 'card' },
          h('label', { for: 'receiver-name', class: 'field' }, h('span', {}, t('rc_bouquet_name')), nameInput),
          h('div', { class: 'actions' }, copyBtn, undoBtn),
          dirty && h('p', { class: 'hint' }, t('rc_draft_blocks_actions'))),
        d.warning ? h('div', { class: 'warn-box' }, h('span', {}, d.warning),
          h('button', { class: 'btn', disabled: d.busy, onclick: async () => {
            d.busy = true; redraw();
            try { await api.post('/api/receiver-bouquets/reload'); d.warning = ''; toast(t('rc_reloaded')); }
            catch (e) { fail(e); }
            finally { d.busy = false; redraw(); }
          } }, t('rc_retry_reload'))) : h('span'),
        h('div', { class: 'receiver-save actions' }, saveBtn,
          h('button', { class: 'btn', disabled: d.busy, onclick: reloadDraft }, icon('refresh'), dirty ? t('rc_discard_btn') : t('rc_reload_from_box')), status),
        h('p', { class: 'hint' }, t('rc_entries_hint', { entries: entries(d.channels.length) })),
        d.channels.length ? rows : emptyState(t('rc_empty_title'), t('rc_empty_text')));
    };
    views.set(id, draw);
    redraw();
    return section;
  }
  return { list, detail };
}
