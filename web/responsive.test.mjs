import assert from 'node:assert/strict';
import { execFile, spawn } from 'node:child_process';
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { promisify } from 'node:util';
import test from 'node:test';

const exec = promisify(execFile);
const binary = process.env.DREAMINATOR_WEB_BINARY;

// Optional real-browser check: make web-test. No receiver or provider is used.
test('channel names and actions remain usable across viewport sizes', { skip: !binary, timeout: 240000 }, async (t) => {
  const dir = await mkdtemp(join(tmpdir(), 'dreaminator-responsive-'));
  const session = `dreaminator-responsive-${process.pid}`;
  const browser = async (...args) => {
    const { stdout } = await exec('npx', ['--yes', 'agent-browser@0.41.2', '--session', session, ...args], { timeout: 30000 });
    return stdout;
  };
  const evaluate = async (script) => {
    const output = await browser('--json', 'eval', '-b', Buffer.from(script).toString('base64'));
    const result = JSON.parse(output);
    assert.equal(result.success, true, output);
    return result.data.result;
  };
  let service;
  t.after(async () => {
    if (service && service.exitCode === null && service.signalCode === null) {
      const exited = new Promise((done) => service.once('exit', done));
      service.kill('SIGTERM');
      await exited;
    }
    await browser('close').catch(() => {});
    await rm(dir, { recursive: true, force: true });
  });

  const listener = createServer();
  await new Promise((done) => listener.listen(0, '127.0.0.1', done));
  const port = listener.address().port;
  await new Promise((done) => listener.close(done));
  const base = `http://127.0.0.1:${port}`;
  const names = ['Луна', 'Северный маяк — путешествия и открытия HD',
    'Amber Field Documentary International HD', 'Город облаков — истории и музыка',
    'Moss & Meadow', 'Мастерская далёких миров Ultra HD',
    'AReallyLongUnbrokenInventedChannelNameForLayoutChecks'];
  const channels = names.map((name, i) => ({ id: `demo-${i}`, name, group: 'Открытия и истории', url: `${base}/demo-stream-${i}.ts` }));
  const refs = channels.map(({ id, name }) => ({ source: 'demo-source', id, name }));
  const state = {
    sources: [{ id: 'demo-source', name: 'Учебный плейлист — открытия и истории', type: 'm3u',
      url: 'https://playlist.example.test/demo.m3u', channelCount: channels.length, refreshing: false }],
    bouquets: [{ id: 'demo-favorites', name: 'Избранное', channels: refs },
      { id: 'demo-empty', name: 'Для тихого вечера — новые открытия', channels: [] },
      { id: 'demo-missing', name: 'Недоступные каналы', channels: [{ source: 'demo-source', id: 'demo-removed', name: 'Забытый сад — недоступный канал' }] }],
    settings: { serviceType: 4097, streamFormat: 'ts', refreshHour: -1, bouquetsOnTop: true, webifUrl: '', picons: false },
    dirty: false, epg: {}, picons: {},
  };
  await mkdir(join(dir, 'data/channels'), { recursive: true });
  for (const folder of ['enigma2', 'epgimport', 'picons']) await mkdir(join(dir, folder));
  await writeFile(join(dir, 'data/state.json'), JSON.stringify(state));
  await writeFile(join(dir, 'data/channels/demo-source.json'), JSON.stringify(channels));
  service = spawn(resolve(binary), ['-listen', `127.0.0.1:${port}`, '-data', join(dir, 'data'),
    '-e2dir', join(dir, 'enigma2'), '-epgimport-dir', join(dir, 'epgimport'), '-plugin-dir', '',
    '-picon-dir', join(dir, 'picons'), '-webif', ''], { stdio: 'ignore' });
  await new Promise((done, reject) => {
    service.once('error', reject);
    let tries = 0;
    const check = async () => {
      try { if ((await fetch(`${base}/api/state`)).ok) return done(); } catch { /* starting */ }
      if (++tries > 100) return reject(new Error('Demo service did not start'));
      setTimeout(check, 100);
    };
    check();
  });

  await browser('open', `${base}/#/lists/demo-favorites`);
  await browser('set', 'media', 'light');
  const open = async (path, selector) => {
    await browser('open', `${base}/#/${path}`);
    await browser('wait', selector);
  };
  const screenshot = async (name) => {
    if (!process.env.DREAMINATOR_WEB_SCREENSHOTS) return;
    const output = resolve(process.env.DREAMINATOR_WEB_SCREENSHOTS);
    await mkdir(output, { recursive: true });
    await browser('screenshot', join(output, `${name}.png`));
  };
  const layout = async (width, channelRows = false) => {
    const metrics = await evaluate(`(() => {
      const box = e => { const r = e.getBoundingClientRect(); return { x:r.x, y:r.y, right:r.right, bottom:r.bottom, width:r.width, height:r.height }; };
      return {
        overflow: document.documentElement.scrollWidth > innerWidth,
        rows: [...document.querySelectorAll('.list > .row')].map(row => ({
          title: box(row.querySelector('.row-title')),
          controls: [...row.querySelectorAll('button, .grip')].map(box)
        }))
      };
    })()`);
    assert.equal(metrics.overflow, false, `horizontal overflow at ${width}px`);
    if (!channelRows) return;
    assert.ok(metrics.rows.length > 0);
    for (const row of metrics.rows) {
      assert.ok(row.title.width >= Math.min(150, width * 0.4), `title only ${row.title.width}px at ${width}px`);
      for (const control of row.controls) {
        assert.ok(control.width >= 44 && control.height >= 44, 'touch target below 44px');
        assert.ok(control.right <= width && control.x >= 0, 'control outside viewport');
        const overlaps = row.title.x < control.right && row.title.right > control.x && row.title.y < control.bottom && row.title.bottom > control.y;
        assert.equal(overlaps, false, 'control overlaps title');
      }
    }
  };
  for (const width of [360, 390, 768, 1024, 1440]) {
    await browser('set', 'viewport', String(width), width < 600 ? '844' : '1024');
    await open('lists/demo-favorites', '.list > .row');
    await layout(width, true);
    if ([390, 768, 1440].includes(width)) await screenshot(`favorites-${width}`);
    await open('channels/demo-source/Открытия%20и%20истории', '.list > .row');
    await layout(width, true);
    if (width === 390) await screenshot('channels-390');
    for (const [path, selector] of [['lists', '.list-card'], ['sources', '.card'], ['settings', '.section-card'],
      ['lists/demo-empty', '.empty'], ['lists/demo-missing', '.missing']]) {
      await open(path, selector);
      await layout(width, path.endsWith('demo-missing'));
    }
    t.diagnostic(`layout verified at ${width}px`);
  }

  await browser('set', 'viewport', '390', '844');
  await open('channels/demo-source/Открытия%20и%20истории', '.list > .row');
  await browser('click', '.row:first-child .star');
  await browser('wait', '.row:first-child .star:not(.on)');
  assert.equal((await (await fetch(`${base}/api/state`)).json()).bouquets[0].channels.length, refs.length - 1);
  await browser('click', '.row:first-child .star');
  await browser('wait', '.row:first-child .star.on');
  await browser('fill', 'input[type=search]', 'Amber');
  await browser('wait', '--fn', 'document.querySelectorAll(".list > .row").length === 1');
  await layout(390, true);
  await browser('click', '.row-actions button[title="Проверить поток"]');
  await browser('wait', '.modal .form-error');
  await browser('click', '.modal-head button');

  await open('lists/demo-favorites', '.list > .row');
  const before = await evaluate('[...document.querySelectorAll(".sortable-item")].map(e => e.dataset.id)');
  const positions = await evaluate('[...document.querySelectorAll(".sortable-item")].slice(0,2).map(e => { const r = e.querySelector(".grip").getBoundingClientRect(); return {x:Math.round(r.x+r.width/2),y:Math.round(r.y+r.height/2)}; })');
  await browser('mouse', 'move', String(positions[0].x), String(positions[0].y));
  await browser('mouse', 'down', 'left');
  await browser('mouse', 'move', String(positions[1].x), String(positions[1].y + 30), '--steps', '12');
  await browser('mouse', 'up', 'left');
  await browser('wait', '--fn', `document.querySelector('.sortable-item').dataset.id !== ${JSON.stringify(before[0])}`);
  const saved = (await (await fetch(`${base}/api/state`)).json()).bouquets[0].channels;
  assert.equal(saved[0].id, before[1].split('/')[1], 'drag order persisted');
  await browser('focus', '.row:first-child .grip');
  await browser('press', 'ArrowDown');
  await browser('wait', '--fn', `document.querySelector('.sortable-item').dataset.id === ${JSON.stringify(before[0])}`);
  await browser('wait', '--fn', 'document.activeElement.matches(".row:nth-child(2) .grip")');
  assert.equal((await (await fetch(`${base}/api/state`)).json()).bouquets[0].channels[0].id, before[0].split('/')[1], 'keyboard order persisted');
  await browser('click', '.row:first-child .danger');
  await browser('wait', '--fn', `document.querySelectorAll('.list > .row').length === ${refs.length - 1}`);
  assert.equal((await (await fetch(`${base}/api/state`)).json()).bouquets[0].channels.length, refs.length - 1);
  await layout(390, true);
  await browser('scroll', 'down', '5000');
  assert.equal(await evaluate('document.querySelector("#view .actions:last-child").getBoundingClientRect().bottom <= document.querySelector("#applybar").getBoundingClientRect().top'), true, 'save panel covers the final action');
  await browser('set', 'media', 'dark');
  await browser('scroll', 'up', '5000');
  await layout(390, true);
  await screenshot('favorites-390-dark');
  await open('lists', '.list-card');
  await browser('focus', '.list-card:first-child .grip');
  await browser('press', 'ArrowDown');
  await browser('wait', '--fn', 'document.querySelector(".list-card").dataset.id === "demo-empty" && document.activeElement.matches(".list-card:nth-child(2) .grip")');
  assert.equal((await (await fetch(`${base}/api/state`)).json()).bouquets[0].id, 'demo-empty');
});
