import test from 'node:test';
import assert from 'node:assert/strict';
import { Feed } from './feed.mjs';
import { MAX_RETRIES } from './model.mjs';

function harness() {
  const connections = [], states = [], messages = [], timers = new Map();
  let timerID = 0;
  const feed = new Feed({
    createEventSource: () => {
      const source = { closed: false, close() { this.closed = true; }, addEventListener(type, handler) { this[type] = handler; } };
      connections.push(source);
      return source;
    },
    setTimer: (fn, delay) => { const id = ++timerID; timers.set(id, { fn, delay }); return id; },
    clearTimer: id => timers.delete(id),
    onState: state => states.push(state), onEvent: event => messages.push(event),
  });
  const tick = () => { const [id, timer] = timers.entries().next().value; timers.delete(id); timer.fn(); };
  return { feed, connections, states, messages, timers, tick };
}

test('closes native reconnect and exhausts a bounded retry budget', () => {
  const h = harness();
  h.feed.start();
  for (let i = 0; i <= MAX_RETRIES; i++) {
    const source = h.connections.at(-1);
    source.onerror();
    assert.equal(source.closed, true);
    if (i < MAX_RETRIES) h.tick();
  }
  assert.equal(h.connections.length, MAX_RETRIES + 1);
  assert.equal(h.states.at(-1).status, 'stopped');
  assert.equal(h.timers.size, 0);
});

test('healthy events recover transport; stop cancels pending work', () => {
  const h = harness();
  h.feed.start();
  h.connections.at(-1).onerror();
  h.tick();
  h.connections.at(-1).telemetry({ data: JSON.stringify({ source: 'simulator', health: 'connected' }) });
  assert.equal(h.states.at(-1).status, 'open');
  assert.equal(h.messages.length, 1);
  h.feed.stop();
  assert.equal(h.timers.size, 0);
  assert.equal(h.connections.at(-1).closed, true);
});

test('silent connection times out and malformed data cannot appear healthy', () => {
  const h = harness();
  h.feed.start();
  h.connections.at(-1).telemetry({ data: 'invalid json' });
  assert.equal(h.messages.length, 0);
  h.tick();
  assert.equal(h.states.at(-1).status, 'retrying');
  h.feed.stop();
  assert.equal(h.timers.size, 0);
});

test('native browser timers retain the global receiver', t => {
  t.mock.method(globalThis, 'clearTimeout', function () { assert.equal(this, globalThis); });
  t.mock.method(globalThis, 'setTimeout', function () { assert.equal(this, globalThis); return 1; });
  const feed = new Feed({
    createEventSource: () => ({ close() {}, addEventListener() {} }),
    onEvent() {}, onState() {},
  });
  feed.start();
  feed.stop();
});
