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
