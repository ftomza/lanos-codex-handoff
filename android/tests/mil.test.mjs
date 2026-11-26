import assert from 'node:assert/strict';
import { test } from 'node:test';
import { milBadge } from '../app/src/main/assets/mil.mjs';

const reading = { state: 'on', dtc_count: 0, remaining_ms: 200 };
const live = { source: 'live', health: 'connected', mil: reading };

test('MIL ON with zero DTCs remains on and OFF needs a fresh valid reply', () => {
  assert.deepEqual(milBadge(live, 199, 'open'), { state: 'on', label: 'Check Engine · Горит', count: 0 });
  assert.deepEqual(milBadge({ ...live, mil: { ...reading, state: 'off' } }, 100, 'open'),
    { state: 'off', label: 'Check Engine · Не горит', count: 0 });
});

test('expired, disconnected, in-standby and malformed readings are unknown', () => {
  for (const [event, elapsed, status] of [
    [live, 200, 'open'],
    [live, 0, 'disconnected'],
    [{ ...live, health: 'disconnected' }, 0, 'open'],
    [{ ...live, standby_reason: 'ecu' }, 0, 'open'],
    [{ ...live, mil: { ...reading, dtc_count: null } }, 0, 'open'],
    [{ ...live, mil: { ...reading, dtc_count: 128 } }, 0, 'open'],
  ]) assert.equal(milBadge(event, elapsed, status).state, 'unknown');
  assert.equal(milBadge({ ...live, source: 'simulator' }, 0, 'open'), null);
});

test('the badge displays the ECU count even when the lamp is off', async () => {
  const { renderMil } = await import('../app/src/main/assets/mil.mjs');
  const el = { dataset: {} };
  renderMil(el, { ...live, mil: { ...reading, state: 'off', dtc_count: 2 } }, 0, 'open');
  assert.equal(el.textContent, 'Check Engine · Не горит · DTC 2');
});
