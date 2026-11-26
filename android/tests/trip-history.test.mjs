import assert from 'node:assert/strict';
import { test } from 'node:test';
import { historyDisplay, elapsedLabel } from '../app/src/main/assets/trip-history.mjs';

test('keeps zero, unknown and marked gaps distinct in the report', () => {
  const result = historyDisplay({
    from: '2026-11-25T08:00:00Z', to: '2026-11-25T08:02:00Z', distance_km: 0, liters: null,
    average_l100: 0, coverage_percent: 68, samples: 100, incomplete: true, bad_rows: 1,
    markers: [{at: '2026-11-25T08:01:00Z', kind: 'mil_after_gap'}, {at: 'broken', kind: 'gap'}],
  });
  assert.equal(result.distance, '0,00');
  assert.equal(result.liters, '—');
  assert.match(result.note, /неполн/);
  assert.match(result.note, /повреждённых строк.*1/);
  assert.equal(result.markers.length, 1);
  assert.equal(result.markers[0].elapsed, 60000);
  assert.equal(result.markers[0].label, 'Check Engine горит после пропуска');
});

test('elapsed time labels use the common monotonic axis', () => {
  assert.equal(elapsedLabel(61_000), '01:01');
  assert.equal(elapsedLabel(3_605_000), '1:00:05');
});
