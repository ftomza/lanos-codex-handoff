const health = new Set(['connecting', 'connected', 'stale', 'disconnected']);
const readings = ['rpm', 'speed_kmh', 'coolant_c', 'voltage_v'];
const fuelOperations = new Map([
  ['/api/fuel/reset', 'reset'],
  ['/api/fuel/calibration/preview', 'preview'],
  ['/api/fuel/calibration', 'calibration'],
]);
const isTime = value => typeof value === 'string' && Number.isFinite(Date.parse(value));

function adjustRemaining(item, transitMs) {
  if (!Number.isFinite(item?.remaining_ms)) return item;
  return { ...item, remaining_ms: transitMs < 0 ? 0 : Math.max(0, item.remaining_ms - transitMs) };
}

function received(sample, now, roundTrip) {
  const transit = now - Date.parse(sample.server_time);
  const timed = adjustRemaining(sample, transit);
  if (sample.source === 'live') {
    timed.signals = Object.fromEntries(Object.entries(sample.signals)
      .map(([key, signal]) => [key, adjustRemaining(signal, transit)]));
  }
  if (sample.fuel || sample.mil) {
    // The frame was sent after the latest ready message. Its time in transit
    // cannot exceed the round trip on the WebView's monotonic clock.
    const delay = transit < 0 ? Infinity : Math.max(transit, Math.ceil(roundTrip));
    const remaining = ms => Number.isFinite(ms) ? Math.max(0, ms - delay) : 0;
    if (sample.mil) timed.mil = { ...sample.mil, remaining_ms: remaining(sample.mil.remaining_ms) };
    if (sample.fuel) timed.fuel = { ...sample.fuel,
      flow_remaining_ms: remaining(sample.fuel.flow_remaining_ms),
      speed_remaining_ms: remaining(sample.fuel.speed_remaining_ms),
    };
  }
  return { ...timed, server_time: new Date(now).toISOString() };
}

function snapshot(sample, source) {
  if (sample?.source !== source || !health.has(sample.health) ||
      !isTime(sample.server_time) || !isTime(sample.received_at) ||
      !(sample.stale_after_ms > 0 && Number.isFinite(sample.stale_after_ms)) ||
      !(isTime(sample.captured_at) || (sample.health !== 'connected' && sample.captured_at === null)) ||
      !readings.every(key => sample[key] === null || Number.isFinite(sample[key]))) return null;
  if (source === 'live' && (!sample.signals || typeof sample.signals !== 'object')) return null;
  return sample;
}

// Same interface as the browser's SSE Feed. The native host provides a port
// targeted at the bundled page for snapshots and local trip controls.
export class Feed {
  constructor({ onEvent, onState, host = window,
    now = () => Date.now(), monotonic = () => performance.now(),
    setTimer = (fn, delay) => setTimeout(fn, delay),
    clearTimer = id => clearTimeout(id), reload = () => host.location.reload() }) {
    Object.assign(this, { onEvent, onState, host, now, monotonic, setTimer, clearTimer, reload });
    this.listener = event => this.accept(event);
    this.status = 'disconnected';
    this.port = null;
    this.deadline = null;
    this.fuelId = 0;
    this.fuelPending = null;
  }

  start() {
    if (this.status === 'stopped') {
      this.reload();
      return;
    }
    this.closePort();
    this.host.addEventListener('message', this.listener);
    this.setState('connecting');
    this.watch();
  }

  stop() {
    this.host.removeEventListener('message', this.listener);
    this.closePort();
    this.setState('disconnected');
  }

  accept(event) {
    if (event.source !== null || (event.origin && event.origin !== this.host.location?.origin)) return;
    if (event.data === 'lanos:paused' && event.ports.length === 0) {
      this.closePort();
      this.setState('disconnected', true);
      return;
    }
    const source = event.data === 'lanos:telemetry:live' ? 'live' :
      event.data === 'lanos:telemetry' ? 'simulator' : null;
    if (source === null || event.ports.length !== 1) return;
    this.closePort();
    const [port] = event.ports;
    this.port = port;
    if (this.host.document) this.host.document.documentElement.dataset.telemetrySource = source;
    this.setState('connecting', true);
    this.watch();
    let readyAt;
    const requestSnapshot = () => {
      readyAt = this.monotonic();
      port.postMessage('ready');
    };
    port.onmessage = message => {
      if (this.port !== port || typeof message.data !== 'string' || message.data.length > 8192) return;
      let payload;
      try { payload = JSON.parse(message.data); } catch { return; }
      if (payload?.type === 'fuel_reply') {
        this.receiveFuelReply(payload);
        return;
      }
      const data = snapshot(payload, source);
      if (!data) return;
      const dataset = this.host.document?.documentElement.dataset;
      if (dataset) dataset.vgateRecovery = String(source === 'live' && data.recovery_required === true);
      // Both clocks are on the device. Include time spent waiting in the WebView
      // queue before the shared model starts its monotonic watchdog.
      this.onEvent(received(data, this.now(), this.monotonic() - readyAt));
      this.setState('open');
      this.watch();
      requestSnapshot();
    };
    requestSnapshot();
  }

  async fuelRequest(path, body) {
    const operation = fuelOperations.get(path);
    if (!operation || this.status !== 'open' || !this.port || this.fuelPending) {
      throw new Error('Действие недоступно');
    }
    const id = ++this.fuelId;
    return new Promise((resolve, reject) => {
      const timer = this.setTimer(() => {
        this.fuelPending = null;
        reject(new Error('Приборка не ответила'));
      }, 5000);
      this.fuelPending = { id, timer, resolve, reject };
      this.port.postMessage(JSON.stringify({ type: 'fuel', id, operation, body }));
    });
  }

  receiveFuelReply(reply) {
    const pending = this.fuelPending;
    if (!pending || reply.id !== pending.id) return;
    this.fuelPending = null;
    this.clearTimer(pending.timer);
    if (typeof reply.error === 'string') pending.reject(new Error(reply.error));
    else pending.resolve(reply.result);
  }

  watch() {
    this.clearTimer(this.deadline);
    this.deadline = this.setTimer(() => {
      this.closePort();
      this.setState('stopped', true);
    }, 8000);
  }

  closePort() {
    const dataset = this.host.document?.documentElement.dataset;
    if (dataset) delete dataset.vgateRecovery;
    this.clearTimer(this.deadline);
    this.deadline = null;
    if (this.fuelPending) {
      this.clearTimer(this.fuelPending.timer);
      this.fuelPending.reject(new Error('Соединение с приборкой закрыто'));
      this.fuelPending = null;
    }
    this.port?.close();
    this.port = null;
  }

  setState(status, channelChanged = false) {
    this.status = status;
    this.onState({ status, attempt: 0, channelChanged });
  }
}
