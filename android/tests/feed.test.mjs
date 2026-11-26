import assert from 'node:assert/strict';
import { test } from 'node:test';
import { Feed } from '../app/src/main/assets/feed.mjs';
import { fuelView } from '../../web/fuel.mjs';
import { presentation, viewState } from '../../web/model.mjs';

const time = '2026-11-17T12:00:00Z';
const snapshot = {
  source: 'simulator', health: 'connected', captured_at: time, received_at: time,
  server_time: time, stale_after_ms: 3000, speed_kmh: 0, rpm: 850, coolant_c: 78, voltage_v: null,
};

function fixture() {
  const host = new EventTarget();
  const dataset = {};
  host.document = { documentElement: { dataset } };
  const events = [], states = [], timers = new Map();
  let clock = 0, reloads = 0;
  let now = Date.parse(time), monotonic = 0;
  const feed = new Feed({
    host, onEvent: data => events.push(data), onState: state => states.push(state.status),
    now: () => now, monotonic: () => monotonic,
    setTimer: fn => { timers.set(++clock, fn); return clock; },
    clearTimer: id => timers.delete(id),
    reload: () => { reloads++; },
  });
  const signal = (data, ports = [], source = null, origin = '') =>
    host.dispatchEvent(Object.assign(new Event('message'), { data, ports, source, origin }));
  const attach = (live = false) => {
    const port = { closed: false, replies: [], close() { this.closed = true; },
      postMessage(value) { this.replies.push(value); } };
    signal(live ? 'lanos:telemetry:live' : 'lanos:telemetry', [port]);
    return port;
  };
  const expire = () => {
    for (const [id, fn] of [...timers]) {
      timers.delete(id);
      fn();
    }
  };
  const sample = (port, data = snapshot) => port.onmessage({ data: JSON.stringify(data) });
  return { feed, events, states, attach, expire, sample, signal, dataset,
    advance: (milliseconds, wall = milliseconds) => { now += wall; monotonic += milliseconds; }, reloads: () => reloads };
}

test('local samples preserve zero, missing voltage, stale expiration and simulation label', () => {
  const f = fixture();
  f.feed.start();
  const port = f.attach();
  f.sample(port);
  assert.deepEqual(port.replies, ['ready', 'ready']);
  assert.equal(f.states.at(-1), 'open');
  const view = viewState(f.events[0], 0, f.states.at(-1));
  assert.equal(view.source, 'SIMULATOR');
  assert.equal(view.speed, '0');
  assert.equal(view.voltage, '—');
  assert.equal(viewState(f.events[0], 3000, 'open').rpm, '—');
  f.sample(port, {
    ...snapshot, health: 'disconnected', captured_at: null, rpm: null, speed_kmh: null, coolant_c: null,
  });
  assert.equal(viewState(f.events[1], 0, 'open').health, 'disconnected');
});

test('ignores malformed snapshots, live payloads and other window messages', () => {
  const f = fixture();
  f.feed.start();
  f.signal('unrelated', [{ close() {} }]);
  f.signal('lanos:telemetry', [{}], {});
  f.signal('lanos:telemetry', [{}], null, 'https://example.org');
  const port = f.attach();
  port.onmessage({ data: '{' });
  f.sample(port, { ...snapshot, source: 'live' });
  f.sample(port, { ...snapshot, health: 'connected', captured_at: null });
  assert.deepEqual(f.events, []);
  assert.equal(f.states.at(-1), 'connecting');
  assert.deepEqual(port.replies, ['ready']);
});

test('replacing, pausing or stopping a channel fences late messages', () => {
  const f = fixture();
  f.feed.start();
  const first = f.attach();
  const late = first.onmessage;
  f.sample(first);
  const second = f.attach();
  assert.equal(first.closed, true);
  assert.equal(f.states.at(-1), 'connecting');
  late({ data: JSON.stringify(snapshot) });
  assert.equal(f.events.length, 1);
  f.signal('lanos:paused');
  assert.equal(second.closed, true);
  assert.equal(f.states.at(-1), 'disconnected');
  const third = f.attach();
  f.sample(third);
  assert.equal(f.events.length, 2);
  f.feed.stop();
  assert.equal(third.closed, true);
  assert.equal(f.states.at(-1), 'disconnected');
});

test('a silent feed offers a page restart and can recover on channel replacement', () => {
  const f = fixture();
  f.feed.start();
  f.expire();
  assert.equal(f.states.at(-1), 'stopped');
  f.feed.start();
  assert.equal(f.reloads(), 1);
  const port = f.attach();
  f.sample(port);
  assert.equal(f.states.at(-1), 'open');
  f.expire();
  assert.equal(f.states.at(-1), 'stopped');
  assert.equal(port.closed, true);
});

test('WebView delivery delay counts toward expiry and the feed still acknowledges the sample', () => {
  const f = fixture();
  f.feed.start();
  const port = f.attach();
  f.advance(2500);
  f.sample(port);
  assert.equal(viewState(f.events[0], 499, 'open').rpm, '850');
  assert.equal(viewState(f.events[0], 500, 'open').rpm, '—');
  f.advance(1000);
  f.sample(port);
  assert.equal(viewState(f.events[1], 0, 'open').health, 'stale');
  assert.equal(port.replies.length, 3);
  const fresh = new Date(Date.parse(time) + 3500).toISOString();
  f.sample(port, { ...snapshot, captured_at: fresh, received_at: fresh, server_time: fresh });
  assert.equal(viewState(f.events[2], 0, 'open').rpm, '850');
});

test('a live channel never substitutes simulator data and expires each real reading separately', () => {
  const f = fixture();
  f.feed.start();
  const sim = f.attach();
  f.sample(sim);
  const live = f.attach(true);
  f.sample(live);
  assert.equal(f.events.length, 1);

  const fields = { rpm: 0, speed_kmh: null, coolant_c: 50, voltage_v: 14.2 };
  const signals = Object.fromEntries(Object.entries(fields).map(([key, value]) => [key, {
    value, support: 'supported', status: value === null ? 'temporarily_unavailable' : 'available',
    captured_at: time, received_at: time, stale_after_ms: key === 'rpm' ? 3000 : 10000,
  }]));
  const real = { ...snapshot, ...fields, source: 'live', stale_after_ms: 10000, signals };
  f.sample(live, real);
  assert.equal(viewState(f.events[1], 0, 'open').rpm, '0');
  const expiredRpm = viewState(f.events[1], 3000, 'open');
  assert.equal(expiredRpm.source, 'LIVE');
  assert.equal(expiredRpm.rpm, '—');
  assert.equal(expiredRpm.voltage, '14.2');
  assert.equal(expiredRpm.coolant, '50');

  const lost = Object.fromEntries(Object.keys(fields).map(key => [key, null]));
  f.sample(live, { ...real, ...lost, health: 'disconnected', captured_at: null, signals: {} });
  assert.equal(viewState(f.events[2], 0, 'open').source, 'LIVE');
  assert.equal(viewState(f.events[2], 0, 'open').voltage, '—');
});

test('delayed live delivery cannot extend monotonic limits after a backward wall-clock step', () => {
  const f = fixture();
  f.feed.start();
  const port = f.attach(true);
  const sent = '2026-11-17T12:00:01.900Z';
  const rpm = {
    value: 850, status: 'available', captured_at: time, received_at: time,
    stale_after_ms: 3000, remaining_ms: 100,
  };
  f.advance(1950);
  f.sample(port, { ...snapshot, source: 'live', server_time: sent, remaining_ms: 100, signals: { rpm } });
  assert.equal(viewState(f.events[0], 49, 'open').rpm, '850');
  assert.equal(viewState(f.events[0], 50, 'open').rpm, '—');
  const future = '2026-11-17T12:00:02.000Z';
  f.sample(port, { ...snapshot, source: 'live', server_time: future, remaining_ms: 100, signals: { rpm } });
  assert.equal(viewState(f.events[1], 0, 'open').rpm, '—');
});

test('ECU standby explains the wait and shows only independently fresh adapter voltage', () => {
  const f = fixture();
  f.feed.start();
  const port = f.attach(true);
  const reading = value => ({
    value, status: 'available', captured_at: time, received_at: time, stale_after_ms: 10000,
  });
  f.sample(port, {
    ...snapshot, source: 'live', stale_after_ms: 10000, standby_reason: 'ecu',
    message: 'ЭБУ не отвечает. Приборка сама повторит проверку.',
    signals: { rpm: reading(850), coolant_c: reading(79), voltage_v: reading(12.7) },
  });
  const event = f.events[0];
  const state = viewState(event, 0, 'open');
  assert.equal(state.source, 'LIVE');
  assert.equal(state.rpm, '—');
  assert.equal(state.coolant, '—');
  assert.equal(state.voltage, '12.7');
  assert.equal(viewState(event, 10000, 'open').voltage, '—');
  const text = presentation(event, state, { status: 'open' });
  assert.match(text.connection, /ЭБУ/);
  assert.match(text.detail, /повторит проверку/);
});

test('queued native fuel expires even when wall clock has barely advanced', () => {
  const f = fixture();
  f.feed.start();
  const port = f.attach();
  const fuel = {
    status: 'available', flow_lph: 5, flow_remaining_ms: 1000,
    speed_kmh: 40, speed_remaining_ms: 400, factor: 1,
  };
  f.advance(250, 20);
  f.sample(port, { ...snapshot, fuel });
  assert.equal(fuelView(f.events.at(-1), 149, 'open').unit, 'л/100 км');
  const expiredSpeed = fuelView(f.events.at(-1), 150, 'open');
  assert.equal(expiredSpeed.unit, 'л/ч');
  assert.equal(expiredSpeed.instant, '5.0');
  assert.equal(fuelView(f.events.at(-1), 750, 'open').instant, '—');
});

test('native fuel requests are allowlisted and replies belong to the current port', async () => {
  const f = fixture();
  f.feed.start();
  const port = f.attach();
  f.sample(port);
  const reset = { expected_revision: 1, default_factor: false };
  await assert.rejects(f.feed.fuelRequest('/api/ecu', reset));
  const result = f.feed.fuelRequest('/api/fuel/reset', reset);
  const request = JSON.parse(port.replies.at(-1));
  assert.deepEqual(request, { type: 'fuel', id: 1, operation: 'reset', body: reset });
  port.onmessage({ data: JSON.stringify({ type: 'fuel_reply', id: 1, result: {} }) });
  assert.deepEqual(await result, {});
  const pending = f.feed.fuelRequest('/api/fuel/reset', reset);
  const failed = assert.rejects(pending);
  const live = f.attach(true);
  await failed;
  port.onmessage({ data: JSON.stringify({ type: 'fuel_reply', id: 2, result: {} }) });
  assert.deepEqual(live.replies, ['ready']);
});

test('power confirmation appears only for the current LIVE connection and clears on pause', () => {
  const f = fixture();
  f.feed.start();
  const sim = f.attach();
  f.sample(sim, { ...snapshot, recovery_required: true });
  assert.equal(f.dataset.vgateRecovery, 'false');
  const live = f.attach(true);
  const waiting = {
    ...snapshot, source: 'live', health: 'connecting', signals: {},
    recovery_required: true,
  };
  f.sample(live, waiting);
  assert.equal(f.dataset.vgateRecovery, 'true');
  f.sample(live, { ...waiting, recovery_required: false });
  assert.equal(f.dataset.vgateRecovery, 'false');
  f.sample(live, waiting);
  f.signal('lanos:paused');
  assert.equal(f.dataset.vgateRecovery, undefined);
  f.sample(live, waiting);
  assert.equal(f.dataset.vgateRecovery, undefined);
});

test('a queued MIL frame loses lifetime despite a slow wall clock', () => {
  const f = fixture();
  f.feed.start();
  const port = f.attach(true);
  f.advance(300, 10);
  f.sample(port, {
    ...snapshot, source: 'live', signals: {},
    mil: { state: 'on', dtc_count: 0, remaining_ms: 250 },
  });
  assert.equal(f.events.at(-1).mil.remaining_ms, 0);
});
