import { retryDelay } from './model.mjs';

// Closing each failed EventSource disables its unbounded native retry loop.
export class Feed {
  constructor({ createEventSource = () => new EventSource('/events'),
    setTimer = (fn, delay) => globalThis.setTimeout(fn, delay),
    clearTimer = timer => globalThis.clearTimeout(timer), onEvent, onState }) {
    Object.assign(this, { createEventSource, setTimer, clearTimer, onEvent, onState });
    this.source = null;
    this.retryTimer = null;
    this.deadline = null;
    this.attempt = 0;
  }

  start() {
    this.stop();
    this.attempt = 0;
    this.open();
  }

  stop() {
    this.source?.close();
    this.source = null;
    this.clearTimer(this.retryTimer);
    this.clearTimer(this.deadline);
    this.retryTimer = this.deadline = null;
  }

  open() {
    this.onState({ status: 'connecting', attempt: this.attempt });
    const source = this.createEventSource();
    this.source = source;
    this.deadline = this.setTimer(() => this.fail(source), 8000);
    source.onerror = () => this.fail(source);
    source.addEventListener('telemetry', message => {
      if (this.source !== source) return;
      let event;
      try { event = JSON.parse(message.data); } catch { return; }
      if (!event || typeof event.source !== 'string' || !['connecting', 'connected', 'stale', 'disconnected'].includes(event.health)) return;
      this.attempt = 0;
      this.clearTimer(this.deadline);
      this.deadline = this.setTimer(() => this.fail(source), Math.max(8000, (event.stale_after_ms || 0) * 2));
      this.onEvent(event);
      this.onState({ status: 'open', attempt: 0 });
    });
  }

  fail(source) {
    if (this.source !== source) return;
    source.close();
    this.source = null;
    this.clearTimer(this.deadline);
    this.deadline = null;
    const delay = retryDelay(this.attempt);
    if (delay === null) {
      this.onState({ status: 'stopped', attempt: this.attempt });
      return;
    }
    this.attempt++;
    this.onState({ status: 'retrying', attempt: this.attempt, delay });
    this.retryTimer = this.setTimer(() => { this.retryTimer = null; this.open(); }, delay);
  }
}
