import assert from 'node:assert/strict';
import { test } from 'node:test';
import { dtcView } from '../app/src/main/assets/dtc.mjs';
import { milBadge, milDetails } from '../app/src/main/assets/mil.mjs';

const at = '2026-11-25T08:00:00Z';
const reply = codes => ({ status: 'available', codes, captured_at: at });

test('shows known and unknown codes with their ECU mode and timestamp', () => {
  const view = dtcView({ '03': reply(['P0420', 'P1999']), '07': reply(['P0300']) });
  assert.deepEqual(view.modes.map(m => m.mode), ['03', '07']);
  assert.equal(view.modes[0].capturedAt, at);
  assert.match(view.modes[0].codes[0].description, /катализатор.*банк 1/);
  assert.equal(view.modes[0].codes[1].code, 'P1999');
  assert.match(view.modes[0].codes[1].description, /нет.*справочник/i);
  assert.match(view.modes[1].codes[0].description, /пропуск/);
});

test('requires two successful empty replies before saying no codes came back', () => {
  const options = { live: true, lampOn: true };
  const empty = dtcView({ '03': reply([]), '07': reply([]) }, options);
  assert.equal(empty.summary, 'Check Engine горит. ЭБУ не вернул кодов.');
  assert.deepEqual(empty.modes.map(m => m.state), ['empty', 'empty']);
  for (const mode07 of [undefined, { status: 'unsupported' }, { status: 'interrupted' },
    { status: 'temporarily_unavailable' }, { status: 'available', codes: [] },
    reply(['not-a-code']), { ...reply([]), codes_truncated: true }]) {
    assert.equal(dtcView({ '03': reply([]), '07': mode07 }, options).summary, '');
  }
  assert.deepEqual(dtcView({}, options).modes.map(m => m.state), ['waiting', 'waiting']);
  assert.deepEqual(dtcView({}).modes.map(m => m.state), ['missing', 'missing']);
});

test('expired and disconnected live indicators cannot display prior codes', () => {
  const event = { source: 'live', health: 'connected', mil: {
    state: 'on', dtc_count: 1, remaining_ms: 15000, dtc_replies: { '03': reply(['P0420']) },
  } };
  assert.equal(milDetails(event, milBadge(event, 0, 'open')).modes[0].codes[0].code, 'P0420');
  assert.deepEqual(milDetails(event, milBadge(event, 15000, 'open')).modes, []);
  assert.deepEqual(milDetails(event, milBadge(event, 0, 'retrying')).modes, []);
});
