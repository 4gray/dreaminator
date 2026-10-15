import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

// Load the browser ES module without requiring package.json or a DOM package.
const source = await readFile(new URL('./static/receiver.js', import.meta.url), 'utf8');
const { receiverUI } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`);

class Element {
  constructor(tag, props = {}, ...children) {
    this.tag = tag;
    this.disabled = false;
    this.parentElement = null;
    Object.assign(this, props);
    this.replaceChildren(...children);
  }

  get isConnected() {
    return this.connectedRoot || !!this.parentElement?.isConnected;
  }

  replaceChildren(...children) {
    for (const child of this.children || []) {
      if (child instanceof Element) child.parentElement = null;
    }
    this.children = children.flat(Infinity).filter((child) => child != null && child !== false);
    for (const child of this.children) {
      if (child instanceof Element) child.parentElement = this;
    }
  }

  get textContent() {
    return this.children.map((child) => child instanceof Element ? child.textContent : String(child)).join('');
  }

  set textContent(text) {
    this.replaceChildren(String(text));
  }

  contains(target) {
    return this === target || this.children.some((child) => child instanceof Element && child.contains(target));
  }
}

const h = (tag, props, ...children) => new Element(tag, props, ...children);

function find(root, predicate) {
  if (!(root instanceof Element)) return undefined;
  if (predicate(root)) return root;
  for (const child of root.children) {
    const match = find(child, predicate);
    if (match) return match;
  }
  return undefined;
}

function button(root, text) {
  const found = find(root, (el) => el.tag === 'button' && el.textContent.includes(text));
  assert.ok(found, `Button not found: ${text}`);
  return found;
}

test('reopening a bouquet during save updates and re-enables the visible editor', async (t) => {
  const root = h('main');
  root.connectedRoot = true;
  const previousWindow = globalThis.window;
  const previousDocument = globalThis.document;
  globalThis.window = { addEventListener() {} };
  globalThis.document = { contains: (element) => root.contains(element) };
  t.after(() => {
    if (previousWindow === undefined) delete globalThis.window;
    else globalThis.window = previousWindow;
    if (previousDocument === undefined) delete globalThis.document;
    else globalThis.document = previousDocument;
  });

  const id = 'userbouquet.example.tv';
  let saved = { id, name: 'Before', revision: 'revision-before', canUndo: false, channels: [] };
  let resolveSave;
  let submitted;
  let saveCount = 0;
  const api = {
    get: async (path) => {
      assert.equal(path, `/api/receiver-bouquets/${id}`);
      return structuredClone(saved);
    },
    put: (path, body) => {
      assert.equal(path, `/api/receiver-bouquets/${id}`);
      submitted = body;
      saveCount++;
      return new Promise((resolve) => { resolveSave = resolve; });
    },
  };
  const errors = [];
  let ui;
  let visibleID = id;
  let renderPending = Promise.resolve();
  const render = () => {
    renderPending = (async () => {
      root.replaceChildren(visibleID ? await ui.detail(visibleID) : h('section', {}, 'Other view'));
    })();
    return renderPending;
  };
  ui = receiverUI({
    h, api,
    icon: () => null,
    grip: () => null,
    makeSortable() {},
    go() { visibleID = null; return render(); },
    render,
    toast() {},
    fail: (error) => errors.push(error),
    promptDialog: async () => null,
    confirmDialog: async () => true,
    emptyState: (title) => h('div', {}, title),
    plural: (count) => String(count),
  });

  await render();
  const originalSection = root.children[0];
  const input = find(root, (el) => el.id === 'receiver-name');
  input.value = 'After';
  input.oninput({ target: input });
  const saveButton = button(root, 'Сохранить на ТВ');
  assert.equal(saveButton.disabled, false);
  const saving = saveButton.onclick();
  assert.deepEqual(submitted, { revision: 'revision-before', name: 'After', order: [] });

  // Navigate away and then back while the original request is still pending.
  find(root, (el) => el['aria-label'] === 'Все списки').onclick();
  await renderPending;
  assert.equal(originalSection.isConnected, false);
  visibleID = id;
  await render();
  assert.equal(find(root, (el) => el.id === 'receiver-name').disabled, true);

  saved = { ...saved, name: 'After', revision: 'revision-after', canUndo: true };
  resolveSave({ bouquet: structuredClone(saved) });
  await saving;
  await renderPending;
  await new Promise(setImmediate);

  assert.deepEqual(errors, []);
  assert.equal(saveCount, 1, 'navigation must not repeat the mutation');
  const visibleInput = find(root, (el) => el.id === 'receiver-name');
  assert.equal(visibleInput.disabled, false, 'the current editor must re-enable after the save');
  assert.equal(visibleInput.value, 'After');
  assert.equal(find(root, (el) => el.tag === 'h1').textContent, 'After');
  assert.equal(button(root, 'Создать копию').disabled, false);
  assert.equal(button(root, 'Отменить последнее сохранение').disabled, false);

  // A further edit must use the committed revision, not the detached draft.
  visibleInput.value = 'Another edit';
  visibleInput.oninput({ target: visibleInput });
  const nextSave = button(root, 'Сохранить на ТВ');
  assert.equal(nextSave.disabled, false);
  const savingAgain = nextSave.onclick();
  assert.equal(submitted.revision, 'revision-after');
  saved = { ...saved, name: 'Another edit', revision: 'revision-final' };
  resolveSave({ bouquet: structuredClone(saved) });
  await savingAgain;
  await renderPending;
  assert.deepEqual(errors, []);
});

test('renaming from the list keeps an open draft; deleting drops it', async (t) => {
  const root = h('main');
  root.connectedRoot = true;
  const previousWindow = globalThis.window;
  globalThis.window = { addEventListener() {} };
  t.after(() => {
    if (previousWindow === undefined) delete globalThis.window;
    else globalThis.window = previousWindow;
  });

  const id = 'userbouquet.example.tv';
  const channels = [{ id: '0', name: 'A', kind: 'channel' }, { id: '1', name: 'B', kind: 'channel' }];
  let current = { id, name: 'Before', revision: 'r1', count: 2, canUndo: false, channels };
  const calls = [];
  const api = {
    get: async (p) => {
      if (p === '/api/receiver-bouquets') {
        const { channels: _, ...summary } = current;
        return [structuredClone(summary)];
      }
      return structuredClone(current);
    },
    post: async (p, body) => {
      calls.push(['post', p, body]);
      assert.equal(p, `/api/receiver-bouquets/${id}/rename`);
      current = { ...current, name: body.name, revision: 'r2', canUndo: true };
      return { bouquet: structuredClone(current) };
    },
    put: async (p, body) => {
      calls.push(['put', p, body]);
      current = { ...current, revision: 'r3', channels: channels.filter((c) => body.order.includes(c.id)) };
      return { bouquet: structuredClone(current) };
    },
    del: async (p) => {
      calls.push(['del', p]);
      return { warning: '' };
    },
  };
  const confirms = [];
  const errors = [];
  let view = 'detail';
  let renderPending = Promise.resolve();
  let ui;
  const render = () => {
    renderPending = (async () => {
      root.replaceChildren(view === 'detail' ? await ui.detail(id) : await ui.list());
    })();
    return renderPending;
  };
  ui = receiverUI({
    h, api,
    icon: () => null,
    grip: () => null,
    makeSortable() {},
    go() { view = 'list'; return render(); },
    render,
    toast() {},
    fail: (error) => errors.push(error),
    promptDialog: async () => 'After',
    confirmDialog: async (title, text) => { confirms.push(text); return true; },
    emptyState: (title) => h('div', {}, title),
    plural: (count) => String(count),
  });
  const show = async (next) => { view = next; await render(); };
  const click = async (label) => {
    const el = find(root, (x) => x['aria-label'] === label);
    assert.ok(el, `Button not found: ${label}`);
    await el.onclick();
    await renderPending;
  };

  // An unsaved draft that drops channel B.
  await show('detail');
  await click('Убрать B');
  await show('list');
  await click('Переименовать Before');
  assert.deepEqual(calls.pop(), ['post', `/api/receiver-bouquets/${id}/rename`, { revision: 'r1', name: 'After' }]);

  await show('detail');
  assert.equal(find(root, (el) => el.tag === 'h1').textContent, 'After');
  assert.equal(find(root, (el) => el.id === 'receiver-name').value, 'After');
  await button(root, 'Сохранить на ТВ').onclick();
  assert.deepEqual(calls.pop(), ['put', `/api/receiver-bouquets/${id}`, { revision: 'r2', name: 'After', order: ['0'] }],
    'the draft must survive the rename and save against the new revision');

  // Dirty again, then delete from the list.
  current = { ...current, channels };
  await show('detail');
  await button(root, 'Перечитать с ресивера').onclick();
  await click('Убрать A');
  await show('list');
  await click('Удалить After');
  assert.match(confirms.at(-1), /Несохранённый черновик/);
  assert.deepEqual(calls.pop(), ['del', `/api/receiver-bouquets/${id}?revision=r3`]);

  current = { ...current, channels };
  await show('detail');
  assert.ok(find(root, (el) => el['aria-label'] === 'Убрать A'), 'the deleted bouquet\'s draft must not come back');
  assert.deepEqual(errors, []);
});

test('deleting from the editor recovers from errors and surfaces reload warnings in the list', async (t) => {
  const root = h('main');
  root.connectedRoot = true;
  const previousWindow = globalThis.window;
  globalThis.window = { addEventListener() {} };
  t.after(() => {
    if (previousWindow === undefined) delete globalThis.window;
    else globalThis.window = previousWindow;
  });

  const id = 'userbouquet.example.tv';
  const bouquet = { id, name: 'Sport', revision: 'r1', count: 0, canUndo: false, channels: [] };
  let deleteResult = Promise.reject(new Error('busy'));
  deleteResult.catch(() => {});
  const reloads = [];
  const api = {
    get: async (p) => (p === '/api/receiver-bouquets' ? [] : structuredClone(bouquet)),
    del: () => deleteResult,
    post: async (p) => { reloads.push(p); return { ok: true }; },
  };
  const errors = [];
  let view = 'detail';
  let renderPending = Promise.resolve();
  let ui;
  const render = () => {
    renderPending = (async () => {
      root.replaceChildren(view === 'detail' ? await ui.detail(id) : await ui.list());
    })();
    return renderPending;
  };
  ui = receiverUI({
    h, api,
    icon: () => null,
    grip: () => null,
    makeSortable() {},
    go() { view = 'list'; return render(); },
    render,
    toast() {},
    fail: (error) => errors.push(error.message),
    promptDialog: async () => null,
    confirmDialog: async () => true,
    emptyState: (title) => h('div', {}, title),
    plural: (count) => String(count),
  });
  const trash = () => find(root, (el) => el['aria-label'] === 'Удалить букет');

  await render();
  await trash().onclick();
  assert.deepEqual(errors, ['busy']);
  assert.equal(view, 'detail', 'a failed delete stays in the editor');
  assert.equal(trash().disabled, false, 'the editor must not stay busy after a failed delete');

  deleteResult = Promise.resolve({ warning: 'reload failed' });
  await trash().onclick();
  await renderPending;
  assert.equal(view, 'list');
  const retry = button(root, 'Повторить обновление на ТВ');
  assert.match(find(root, (el) => el.class === 'warn-box').textContent, /reload failed/);
  await retry.onclick();
  await renderPending;
  assert.deepEqual(reloads, ['/api/receiver-bouquets/reload']);
  assert.equal(find(root, (el) => el.class === 'warn-box'), undefined, 'a successful retry clears the warning');
});
