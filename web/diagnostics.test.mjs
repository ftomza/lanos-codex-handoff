import test from 'node:test';
import assert from 'node:assert/strict';
import { milView, LiveMIL } from './diagnostics-model.mjs';
import { HistoryController, historyURL } from './diagnostics-history.mjs';

const at = '2026-11-07T12:00:00Z';
const old = '2026-11-07T11:59:00Z';
const future = '2026-11-07T12:00:01Z';
const sessionA = 'a'.repeat(32), sessionB = 'b'.repeat(32), incident = 'incident-000000000000000c';
const reply = (codes = []) => ({ status: 'available', captured_at: old, received_at: old, detail: { status: 'available', codes }, raw_response: '43 00' });
const event = () => ({ source: 'live', health: 'connected', server_time: at, captured_at: at, received_at: at, stale_after_ms: 3000, generation: 1,
  mil: { status: 'available', on: true, dtc_count: 0, captured_at: at, received_at: at, stale_after_ms: 15000 }, ecu_readings: { '03': reply(), '07': reply() } });
const catalog = (sessions = [{ id: sessionA, started_at: old, parts: 1, closed: true, aborted: false }]) => ({ version: 1, source: 'live', retention_truncated: false, sessions });
const report = (id = sessionA) => ({ version: 1, source: 'live', incomplete: true, sessions: [{ id, source: 'live', incomplete: true, events: [], diagnostics: [], incidents: [], findings: [] }], issues: [{ code: 'partial_tail' }], limitations: ['Sparse O2'] });
const json = (value, status = 200) => new Response(JSON.stringify(value), { status });
const deferred = () => { let resolve; const promise = new Promise(r => { resolve = r; }); return { promise, resolve }; };

test('fresh ON and OFF have validated count; unexplained ON has mandated summary', () => {
  let e = event(), v = milView(e, 0, 'open');
  assert.equal(v.state, 'ON'); assert.equal(v.count, 0);
  assert.equal(v.summary, 'Check Engine включён. Причина не определена стандартными диагностическими режимами');
  e.mil.on = false; assert.equal(milView(e, 0, 'open').state, 'OFF');
});
test('MIL expires independently of source even without SSE messages', () => {
  const e = event(); e.stale_after_ms = 60000;
  assert.equal(milView(e, 14999, 'open').state, 'ON');
  const v = milView(e, 15000, 'open'); assert.equal(v.state, 'UNKNOWN'); assert.equal(v.count, null);
});
test('source expiry and transport failure hide MIL immediately', () => {
  for (const [elapsed, transport] of [[3000, 'open'], [0, 'retrying'], [0, 'stopped']]) assert.equal(milView(event(), elapsed, transport).state, 'UNKNOWN');
  const e = event(); e.health = 'disconnected'; assert.equal(milView(e, 0, 'open').state, 'UNKNOWN');
});
test('future or invalid capture/receipt and invalid monotonic elapsed cannot be live', () => {
  for (const scope of ['mil', 'source']) for (const key of ['captured_at', 'received_at']) for (const time of [future, null]) {
    const e = event(); (scope === 'mil' ? e.mil : e)[key] = time;
    assert.equal(milView(e, 0, 'open').state, 'UNKNOWN');
  }
  for (const elapsed of [-1, NaN]) assert.equal(milView(event(), elapsed, 'open').state, 'UNKNOWN');
});
test('SIM and unknown or mismatched sources cannot present saved LIVE MIL as live', () => {
  for (const source of ['simulator', 'other']) { const e = event(); e.source = source; assert.equal(milView(e, 0, 'open').state, 'UNKNOWN'); }
  const e = event(); e.mil.source = 'simulator'; assert.equal(milView(e, 0, 'open').state, 'UNKNOWN');
  assert.match(milView({ source: 'simulator' }, 0, 'open').summary, /симулятором не предоставляется/);
});
test('invalid count and MIL status never produce a valid current reading', () => {
  for (const count of [-1, 128, 1.5, '1', null]) { const e = event(); e.mil.dtc_count = count; assert.equal(milView(e, 0, 'open').state, 'UNKNOWN'); }
  for (const status of ['stale', 'temporarily_unavailable', 'invalid_response']) { const e = event(); e.mil.status = status; assert.equal(milView(e, 0, 'open').count, null); }
});
test('DTC empty success requires matching decoded status and codes with valid own timestamps', () => {
  let v = milView(event(), 0, 'open'); assert.equal(v.dtcs[0].kind, 'empty'); assert.equal(v.dtcs[0].capturedAt, old); assert.equal(v.dtcs[2].kind, 'not_queried');
  for (const change of [r => { delete r.detail.codes; }, r => { r.detail.status = 'unsupported'; }, r => { r.detail.codes = ['garbage']; }, r => { r.received_at = future; }]) {
    const e = event(); change(e.ecu_readings['03']); assert.equal(milView(e, 0, 'open').dtcs[0].kind, 'invalid_response');
  }
  for (const status of ['unsupported', 'temporarily_unavailable', 'invalid_response']) { const e = event(); e.ecu_readings['03'].status = status; assert.equal(milView(e, 0, 'open').dtcs[0].kind, status); }
});
test('last-known DTC codes retain independent timestamp during fresh MIL and transport loss', () => {
  const e = event(); e.ecu_readings['03'] = reply(['P0420']);
  const v = milView(e, 0, 'retrying'); assert.equal(v.dtcs[0].codes[0], 'P0420'); assert.equal(v.dtcs[0].capturedAt, old); assert.equal(v.state, 'UNKNOWN');
});
test('page-scoped last success is not invented from a failed recording and survives expiry', () => {
  const live = new LiveMIL(); const failed = event(); failed.mil.on = null; failed.mil.status = 'temporarily_unavailable'; failed.mil_recording = { state: 'failed', error: 'disk full' };
  live.receive(failed); assert.equal(live.view(0, 'open').lastSuccess, null); assert.match(live.view(0, 'open').recording, /disk full/);
  live.receive(event()); assert.equal(live.view(0, 'open').lastSuccess, at); assert.equal(live.view(16000, 'open').lastSuccess, at);
});
test('a generation change does not promote the previous cached MIL to current', () => {
  const live = new LiveMIL(); live.receive(event()); const e = event(); e.generation = 2; live.receive(e);
  assert.equal(live.view(0, 'open').state, 'UNKNOWN');
  e.server_time = future; e.captured_at = e.received_at = future; e.mil.captured_at = e.mil.received_at = future; live.receive(e);
  assert.equal(live.view(0, 'open').state, 'ON');
});

test('opening loads catalog then latest session once; archive source stays LIVE', async () => {
  const requests = []; const c = new HistoryController({ fetch: async url => { requests.push(url); return json(url.endsWith('sessions') ? catalog([{ id: sessionA, started_at: old }, { id: sessionB, started_at: at }]) : report(sessionB)); } });
  await c.refresh(); assert.equal(c.state.session, sessionB); assert.equal(c.state.source, 'LIVE'); assert.equal(c.state.status, 'ready'); assert.equal(requests.length, 2);
  assert.equal(c.state.reportURL, `/api/mil/history/report?session=${sessionB}`);
});
test('new selection clears old report and links immediately; stale completion is ignored', async () => {
  const pending = deferred(); const c = new HistoryController({ fetch: async url => url.endsWith('sessions') ? json(catalog([{ id: sessionA, started_at: old }, { id: sessionB, started_at: at }])) : url.includes(sessionA) ? pending.promise : json(report(sessionB)) });
  const opening = c.refresh(); await new Promise(r => setImmediate(r));
  const first = c.select(sessionA); assert.equal(c.state.report, null); assert.equal(c.state.reportURL, null);
  await c.select(sessionB); pending.resolve(json(report(sessionA))); await first; await opening;
  assert.equal(c.state.session, sessionB); assert.equal(c.state.report.sessions[0].id, sessionB);
});
test('failed report after selection clears previous content and export links', async () => {
  let fail = false; const c = new HistoryController({ fetch: async url => url.endsWith('sessions') ? json(catalog()) : fail ? json({}, 409) : json(report()) });
  await c.refresh(); fail = true; await c.select(sessionA); assert.equal(c.state.status, 'error'); assert.equal(c.state.report, null); assert.equal(c.state.reportURL, null); assert.match(c.state.error, /Обновите/);
});
test('cancellation invalidates late fetch and clears export affordances', async () => {
  const pending = deferred(); const c = new HistoryController({ fetch: async url => url.endsWith('sessions') ? json(catalog()) : pending.promise });
  const loading = c.refresh(); await new Promise(r => setImmediate(r)); c.cancel(); pending.resolve(json(report())); await loading;
  assert.equal(c.state.status, 'canceled'); assert.equal(c.state.reportURL, null); assert.equal(c.state.report, null);
});
test('empty and disabled archives are explicit states without reports', async () => {
  for (const [response, status] of [[json(catalog([])), 'empty'], [json({}, 503), 'disabled']]) {
    const c = new HistoryController({ fetch: async () => response }); await c.refresh(); assert.equal(c.state.status, status); assert.equal(c.state.reportURL, null);
  }
});
test('corrupt history, unsupported schema, source mismatch and wrong selection cannot create empty success', async () => {
  for (const broken of [{ ...report(), version: 2 }, { ...report(), source: 'simulator' }, { ...report(), sessions: [] }, report(sessionB)]) {
    const c = new HistoryController({ fetch: async url => json(url.endsWith('sessions') ? catalog() : broken) }); await c.refresh(); assert.equal(c.state.status, 'error'); assert.equal(c.state.reportURL, null);
  }
  const c = new HistoryController({ fetch: async () => json({ version: 1, source: 'live', sessions: [{}] }) }); await c.refresh(); assert.equal(c.state.status, 'error');
});
test('recoverable issues retain surviving evidence and exact session/incident links', async () => {
  const c = new HistoryController({ fetch: async url => json(url.endsWith('sessions') ? catalog() : report()) }); await c.refresh();
  assert.equal(c.state.report.incomplete, true); assert.equal(c.state.report.issues[0].code, 'partial_tail');
  assert.equal(historyURL('incident', sessionA, incident), `/api/mil/history/incident?session=${sessionA}&incident=${incident}`);
  assert.throws(() => historyURL('incident', '../secret', incident));
});
test('oversized response is rejected before it can become a download', async () => {
  const c = new HistoryController({ fetch: async () => new Response('x', { headers: { 'content-length': String(8 * 1024 * 1024 + 1) } }) }); await c.refresh();
  assert.equal(c.state.status, 'error'); assert.match(c.state.error, /8 MiB/); assert.equal(c.state.reportURL, null);
});

test('streamed response beyond 8 MiB is rejected without content-length', async () => {
  const c = new HistoryController({ fetch: async () => new Response(new ReadableStream({ start(controller) { controller.enqueue(new Uint8Array(8 * 1024 * 1024 + 1)); controller.close(); } })) });
  await c.refresh(); assert.equal(c.state.status, 'error'); assert.match(c.state.error, /8 MiB/);
});

test('saved rendering keeps every finding field, independent reply and safe literal evidence', async () => {
  const { renderSaved } = await import('./diagnostics.mjs');
  class Node {
    constructor(tag) { this.tag = tag; this.children = []; this.textContent = ''; this.attributes = {}; }
    append(...nodes) { this.children.push(...nodes); }
    setAttribute(key, value) { this.attributes[key] = value; }
    replaceChildren(...nodes) { this.children = nodes; }
    text() { return this.textContent + this.children.map(n => n.text()).join(' '); }
    all() { return [this, ...this.children.flatMap(n => n.all())]; }
  }
  const document = { createElement: tag => new Node(tag) }, parent = new Node('section'), r = report();
  r.sessions[0].findings = [{ code: 'o2', symptom: '<img src=x onerror=bad()>', confidence: 'limited_hypothesis', count: 2, first_observed_at: old, last_observed_at: at,
    supporting_evidence: [{ file: 'part.jsonl', line: 17, sequence: 42, signal: 'o2', value: 0.4 }], contradicting_evidence: [{ detail: 'counterexample' }], missing_data: ['dense O2'], next_check: ['inspect exhaust'] }];
  r.sessions[0].diagnostics = [{ command: '020200', incident: '', reading: { ...reply(['P0420']), received_at: at }, file: 'part.jsonl', line: 18 }];
  r.sessions[0].incidents = [{ id: incident, incomplete: true, closed: false, frame_count: 1, window: { pre_window_retained: false, had_gaps: true }, findings: [], rule_checks: [{ code: 'o2', status: 'insufficient_data', detail: 'sparse' }] }];
  renderSaved(document, parent, r);
  for (const literal of ['<img src=x onerror=bad()>', 'Гипотеза для проверки', 'part.jsonl', 'counterexample', 'dense O2', 'inspect exhaust', 'Недостаточно данных', 'независимый', 'pre_window_retained', 'Sparse O2']) assert.ok(parent.text().includes(literal), literal);
  assert.ok(parent.all().every(n => n.tag !== 'img'));
  assert.equal(parent.all().find(n => n.tag === 'a').href, `/api/mil/history/incident?session=${sessionA}&incident=${incident}`);
});

test('stale DTC retains valid last-known codes but never claims an empty available response', () => {
  const e = event(); e.ecu_readings['03'] = reply(['P0420']); e.ecu_readings['03'].status = 'stale';
  const dtc = milView(e, 0, 'open').dtcs[0]; assert.equal(dtc.kind, 'stale'); assert.deepEqual(dtc.codes, ['P0420']);
  e.ecu_readings['03'].received_at = future; assert.equal(milView(e, 0, 'open').dtcs[0].kind, 'invalid_response');
});
test('page-scoped last success keeps its age independently of a later failed MIL response', () => {
  const live = new LiveMIL(); live.receive(event()); const e = event(); e.server_time = future; e.mil.captured_at = future; e.mil.status = 'temporarily_unavailable'; live.receive(e);
  assert.equal(live.view(1000, 'open').lastSuccess, at); assert.equal(live.view(1000, 'open').lastSuccessAgeMs, 2000);
});

test('watchdog redraw retains DTC details and focus when the saved replies have not changed', async () => {
  const { renderDTCs } = await import('./diagnostics.mjs');
  class Node { constructor() { this.children = []; } append(...nodes) { this.children.push(...nodes); } replaceChildren(...nodes) { this.children = nodes; } }
  const document = { createElement: () => new Node() }, parent = new Node(), e = event();
  renderDTCs(document, parent, milView(e, 0, 'open').dtcs); const card = parent.children[0];
  renderDTCs(document, parent, milView(e, 1000, 'open').dtcs); assert.equal(parent.children[0], card);
  e.ecu_readings['03'] = reply(['P0420']); renderDTCs(document, parent, milView(e, 0, 'open').dtcs); assert.notEqual(parent.children[0], card);
});
