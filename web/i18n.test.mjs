import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const i18nSrc = await readFile(new URL('./static/i18n.js', import.meta.url), 'utf8');
const i18n = await import(`data:text/javascript;base64,${Buffer.from(i18nSrc).toString('base64')}`);

const receiverSrc = await readFile(new URL('./static/receiver.js', import.meta.url), 'utf8');
const { receiverUI } = await import(`data:text/javascript;base64,${Buffer.from(receiverSrc).toString('base64')}`);

const EXPECTED_CODES = ['pt', 'fr', 'he', 'ar', 'it', 'de', 'es', 'pl', 'en', 'ja', 'zh', 'th', 'tr', 'cs', 'ru'];

function placeholders(str) {
  return [...str.matchAll(/\{([a-zA-Z0-9_]+)\}/g)].map((m) => m[1]).sort();
}

function withGlobals(overrides, fn) {
  const saved = {};
  for (const [key, val] of Object.entries(overrides)) {
    saved[key] = Object.getOwnPropertyDescriptor(globalThis, key);
    Object.defineProperty(globalThis, key, { value: val, configurable: true, writable: true });
  }
  try {
    return fn();
  } finally {
    for (const [key, desc] of Object.entries(saved)) {
      if (desc) Object.defineProperty(globalThis, key, desc);
      else delete globalThis[key];
    }
  }
}

test('all 15 requested languages are registered and complete', () => {
  const codes = i18n.LANGUAGES.map((l) => l.code);
  for (const code of EXPECTED_CODES) {
    assert.ok(codes.includes(code), `missing language ${code} in LANGUAGES`);
    assert.ok(i18n.STRINGS[code], `missing dictionary STRINGS.${code}`);
    assert.ok(i18n.LOCALE_MAP[code], `missing locale LOCALE_MAP.${code}`);
  }
  assert.equal(codes.length, 15);

  const refKeys = Object.keys(i18n.STRINGS.ru);
  for (const code of EXPECTED_CODES) {
    const dict = i18n.STRINGS[code];
    for (const key of refKeys) {
      assert.equal(typeof dict[key], 'string', `${code}.${key} is missing`);
      if (!(code === 'en' && key === 'tmdb_notice_local')) {
        assert.ok(dict[key].length > 0, `${code}.${key} must be non-empty`);
      }
      assert.deepEqual(placeholders(dict[key]), placeholders(i18n.STRINGS.ru[key]), `${code}.${key} placeholder mismatch`);
    }
  }
});

test('normalizeLang handles regions and aliases', () => {
  const cases = {
    'pt-BR': 'pt', 'fr-FR': 'fr', 'he-IL': 'he', iw: 'he', 'ar-SA': 'ar',
    'it-IT': 'it', 'de-DE': 'de', 'es-419': 'es', 'pl-PL': 'pl', 'en-GB': 'en',
    'ja-JP': 'ja', jp: 'ja', 'zh-CN': 'zh', 'th-TH': 'th', 'tr-TR': 'tr',
    'cs-CZ': 'cs', cz: 'cs', 'ru-RU': 'ru', auto: '', xx: '', '': '',
  };
  for (const [input, want] of Object.entries(cases)) {
    assert.equal(i18n.normalizeLang(input), want, `normalizeLang(${input})`);
  }
});

test('isRTL only enables RTL for Hebrew and Arabic', () => {
  for (const code of EXPECTED_CODES) {
    assert.equal(i18n.isRTL(code), code === 'he' || code === 'ar', `isRTL(${code})`);
  }
});

test('browser language detection drives auto mode while explicit choice overrides', () => {
  const store = new Map();
  const fakeStorage = {
    getItem: (k) => (store.has(k) ? store.get(k) : null),
    setItem: (k, v) => store.set(k, String(v)),
  };

  withGlobals({ localStorage: fakeStorage, navigator: { languages: ['fr-FR', 'en-US'], language: 'en-US' } }, () => {
    i18n.setLanguagePref('auto');
    assert.equal(i18n.detectBrowserLanguage(), 'fr');
    assert.equal(i18n.getLanguage('ru'), 'fr');

    i18n.setLanguagePref('pt-BR');
    assert.equal(i18n.getLanguage('ru'), 'pt');

    i18n.setLanguagePref('auto');
    assert.equal(i18n.getLanguage('ru'), 'fr');
  });

  withGlobals({ localStorage: fakeStorage, navigator: { languages: ['ko-KR', 'he-IL'] } }, () => {
    store.clear();
    assert.equal(i18n.detectBrowserLanguage(), 'he');
    assert.equal(i18n.getLanguage('es'), 'he');
  });

  withGlobals({ localStorage: fakeStorage, navigator: { languages: ['ko-KR'], language: 'ko-KR' } }, () => {
    store.clear();
    assert.equal(i18n.detectBrowserLanguage(), '');
    assert.equal(i18n.getLanguage('de'), 'de');
    assert.equal(i18n.getLanguage(''), 'ru');
  });
});

test('formatting helpers and receiverUI localize into target languages', async () => {
  assert.equal(i18n.channelsWord(1, 'fr'), '1 chaîne');
  assert.equal(i18n.channelsWord(2, 'fr'), '2 chaînes');
  assert.equal(i18n.entriesWord(3, 'en'), '3 entries');
  assert.equal(i18n.entriesWord(3, 'pl'), '3 pozycje');
  assert.equal(i18n.daysWord(5, 'pt'), '5 dias');
  assert.equal(i18n.trServer('Избранное', 'fr'), 'Favoris');
  assert.equal(i18n.trServer('Все каналы', 'he'), 'כל הערוצים');
  assert.match(i18n.trServer('до 31 декабря 2027', 'en'), /^until .*2027/);
  assert.match(i18n.trServer('«Portal» до 31 декабря 2027', 'fr'), /^Portal · jusqu’au .*2027/);
  assert.equal(i18n.trServer('Ссылка открывается, 10 каналов', 'en'), 'Link is reachable, 10 channels');
  assert.match(i18n.trServer('Сейчас выбран стандартный проигрыватель GStreamer (4097).', 'en'), /GStreamer/);

  class El {
    constructor(tag, props = {}, ...children) {
      this.tag = tag;
      Object.assign(this, props);
      this.children = children.flat(Infinity).filter((c) => c != null && c !== false);
    }
    replaceChildren(...children) { this.children = children.flat(Infinity).filter((c) => c != null && c !== false); }
    get textContent() { return this.children.map((c) => (c instanceof El ? c.textContent : String(c))).join(''); }
  }
  const h = (tag, props, ...children) => new El(tag, props, ...children);
  const prevWin = globalThis.window;
  globalThis.window = { addEventListener() {} };
  try {
    const ui = receiverUI({
      h,
      api: { get: async () => [{ id: 'b1', name: 'Favoris', count: 2 }] },
      icon: () => null,
      grip: () => null,
      makeSortable() {},
      go() {},
      render() {},
      toast() {},
      fail() {},
      promptDialog: async () => null,
      confirmDialog: async () => true,
      emptyState: (title) => h('div', {}, title),
      t: (k, v) => i18n.t(k, v, 'fr'),
      entriesWord: (n) => i18n.entriesWord(n, 'fr'),
    });
    const listEl = await ui.list();
    assert.match(listEl.textContent, /Bouquets du récepteur/);
    assert.match(listEl.textContent, /2 entrées/);
  } finally {
    if (prevWin === undefined) delete globalThis.window;
    else globalThis.window = prevWin;
  }
});
