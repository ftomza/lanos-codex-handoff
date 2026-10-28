import test from 'node:test';
import assert from 'node:assert/strict';
import { viewState, retryDelay, MAX_RETRIES } from './model.mjs';

const event = {
  source: 'simulator', health: 'connected', speed_kmh: 0, rpm: 850,
  coolant_c: 88, voltage_v: null, stale_after_ms: 3000,
  server_time: '2026-10-21T12:00:00Z', captured_at: '2026-10-21T12:00:00Z',
  received_at: '2026-10-21T12:00:00Z',
};

test('zero is available and missing voltage stays unavailable', () => {
  const state = viewState(event, 0, 'open');
  assert.equal(state.health, 'connected');
  assert.equal(state.speed, '0');
  assert.equal(state.voltage, '—');
  assert.equal(state.source, 'SIMULATOR');
});

test('watchdog expires readings even without a server event', () => {
  const state = viewState(event, 3000, 'open');
  assert.equal(state.health, 'stale');
  assert.equal(state.speed, '—');
});

test('old captures cannot become fresh after transport reconnect', () => {
  assert.equal(viewState({ ...event, captured_at: '2026-10-21T11:59:00Z' }, 0, 'open').health, 'stale');
});

test('broken transport and disconnected source hide numeric readings', () => {
  for (const state of [viewState(event, 0, 'retrying'), viewState({ ...event, health: 'disconnected' }, 0, 'open')]) {
    assert.equal(state.health, 'disconnected');
    assert.equal(state.rpm, '—');
  }
});

test('unknown source is never labeled LIVE', () => {
  assert.equal(viewState({ ...event, source: 'unknown' }, 0, 'open').source, 'UNKNOWN');
  assert.equal(viewState({ ...event, source: 'live' }, 0, 'open').source, 'LIVE');
});

test('initial state and non-finite data cannot fabricate readings', () => {
  assert.equal(viewState(null, 0, 'connecting').health, 'connecting');
  assert.equal(viewState({ ...event, speed_kmh: NaN }, 0, 'open').speed, '—');
});

test('retry budget is finite and exponential delay is capped', () => {
  assert.equal(retryDelay(0), 1000);
  assert.equal(retryDelay(1), 2000);
  assert.equal(retryDelay(4), 10000);
  assert.equal(retryDelay(MAX_RETRIES), null);
});

const signal = (value, ageMs, ttl = 35000) => ({
  value, status: 'available', support: 'supported', stale_after_ms: ttl,
  captured_at: new Date(Date.parse(event.server_time) - ageMs).toISOString(),
  received_at: new Date(Date.parse(event.server_time) - ageMs).toISOString(),
});

test('live signals expire independently and do not drive stale warnings or arcs', () => {
  const live = { ...event, source: 'live', speed_kmh: 80, coolant_c: 110, voltage_v: 11,
    signals: { rpm: signal(900, 0, 3000), speed_kmh: signal(80, 3000, 3000),
      coolant_c: signal(110, 35000), voltage_v: signal(11, 130000, 130000) } };
  const state = viewState(live, 0, 'open');
  assert.equal(state.health, 'connected');
  assert.equal(state.rpm, '900');
  assert.equal(state.coolant, '—');
  assert.equal(state.speed, '—');
  assert.equal(state.values.speed_kmh, null);
  assert.deepEqual(state.warnings, []);
});

test('browser watchdog expires an individual signal before the envelope', () => {
  const live = { ...event, signals: { rpm: signal(950, 2500, 3000), coolant_c: signal(85, 0) } };
  assert.equal(viewState(live, 499, 'open').rpm, '950');
  const state = viewState(live, 500, 'open');
  assert.equal(state.health, 'connected');
  assert.equal(state.rpm, '—');
  assert.equal(state.coolant, '85');
  assert.equal(state.values.rpm, null);
  assert.equal(state.speed, '—'); // Never fall back to the flat field when metadata exists.
});

test('unavailable or nonfinite signals never become zero', () => {
  const state = viewState({ ...event, signals: { rpm: { ...signal(900, 0), status: 'temporarily_unavailable' },
    speed_kmh: signal(Infinity, 0), coolant_c: signal(null, 0) } }, 0, 'open');
  assert.equal(state.rpm, '—');
  assert.equal(state.speed, '—');
  assert.equal(state.coolant, '—');
});

test('live presentation identifies real source and adapter supply without demo text', async () => {
  const { presentation } = await import('./model.mjs');
  assert.equal(typeof presentation, 'function');
  const live = { ...event, source: 'live', health: 'disconnected', message: 'Перезапустите приборку.' };
  const text = presentation(live, viewState(live, 0, 'open'), { status: 'open', attempt: 0 });
  assert.equal(text.noticeTag, 'LIVE');
  assert.match(text.detail, /Перезапустите/);
  assert.doesNotMatch(JSON.stringify(text), /Симулятор|Смоделирован|Демонстрац|6 секунд/);
  const ready = { ...event, source: 'live', voltage_v: 14 };
  assert.match(presentation(ready, viewState(ready, 0, 'open'), { status: 'open' }).voltageNote, /адаптера.*OBD/);
});

test('initial HTML and presentation wait for source identity', async () => {
  const { readFile } = await import('node:fs/promises');
  const html = await readFile(new URL('./index.html', import.meta.url), 'utf8');
  assert.match(html, /id="source"[^>]*>UNKNOWN</);
  assert.doesNotMatch(html, /показания симулятора|Демонстрационный режим|Данные демонстрационного/);
  const { presentation } = await import('./model.mjs');
  const text = presentation(null, viewState(null, 0, 'connecting'), { status: 'connecting' });
  assert.equal(text.noticeTag, 'WAIT');
});

test('availability notes distinguish unsupported, untested and temporary failure', async () => {
  const { presentation } = await import('./model.mjs');
  for (const [status, support, expected] of [
    ['unsupported', 'unsupported', /Не поддерживается/],
    ['not_yet_tested', 'not_yet_tested', /ещё не проверена/],
    ['temporarily_unavailable', 'not_yet_tested', /Временно недоступно/],
    ['stale', 'supported', /устарели/],
  ]) {
    const sample = { ...event, source: 'live', signals: { coolant_c: { ...signal(null, 0), status, support } } };
    assert.match(presentation(sample, viewState(sample, 0, 'open'), { status: 'open' }).coolantNote, expected);
  }
});

test('ignition standby stays LIVE, hides readings and displays the source explanation', async () => {
  const { presentation } = await import('./model.mjs');
  for (const message of [
    'Адаптер подключён. ЭБУ не отвечает — возможно, зажигание выключено. Ожидаем…',
    'Нет связи с адаптером. Ожидаем подключения…',
  ]) {
    const waiting = { ...event, source: 'live', health: 'connecting', message };
    const state = viewState(waiting, 60000, 'open');
    const text = presentation(waiting, state, { status: 'open', attempt: 0 });
    assert.equal(state.rpm, '—');
    assert.equal(state.speed, '—');
    assert.equal(state.fresh, false);
    assert.equal(text.noticeTag, 'LIVE');
    assert.equal(text.detail, message);
    assert.doesNotMatch(text.detail, /Перезапустите|Симулятор/);
  }
});
