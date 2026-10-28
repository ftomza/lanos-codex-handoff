import { viewState, presentation } from './model.mjs';
import { fuelView } from './fuel.mjs';
import { bindFuelControls } from './fuel-controls.mjs';
import { Feed } from './feed.mjs';

const elements = Object.fromEntries([...document.querySelectorAll('[id]')].map(el => [el.id, el]));
const bars = Array.from({ length: 24 }, (_, i) => {
  const bar = document.createElement('span');
  if (i >= 20) bar.className = 'redline';
  elements['rpm-bars'].append(bar);
  return bar;
});
let event = null;
let eventAt = performance.now();
let transport = { status: 'connecting', attempt: 0 };

function showTime(id, value) {
  const time = Date.parse(value);
  elements[id].textContent = Number.isFinite(time) && time > 0 ? new Date(time).toLocaleTimeString('ru-RU') : '—';
  elements[id].title = value ?? '';
}

function render() {
  const state = viewState(event, performance.now() - eventAt, transport.status);
  for (const id of ['source', 'speed', 'rpm', 'coolant', 'voltage']) elements[id].textContent = state[id];
  const text = presentation(event, state, transport);
  elements.connection.textContent = text.connection;
  elements.connection.dataset.health = state.health;
  elements['health-panel'].dataset.health = state.health;
  elements['health-title'].textContent = text.title;
  elements['health-detail'].textContent = text.detail;
  elements.reconnect.hidden = transport.status !== 'stopped';
  for (const [id, key] of Object.entries({ 'notice-title': 'noticeTitle', 'notice-detail': 'noticeDetail',
    'notice-tag': 'noticeTag', 'rpm-note': 'rpmNote', 'driving-state': 'driving',
    'coolant-note': 'coolantNote', 'voltage-note': 'voltageNote' })) elements[id].textContent = text[key];
  elements['speed-arc'].style.strokeDashoffset = String(100 - Math.min(160, Math.max(0, state.values.speed_kmh ?? 0)) / 160 * 100);
  bars.forEach((bar, i) => bar.classList.toggle('active', i < (state.values.rpm ?? 0) / 6000 * bars.length));
  const fuel = fuelView(event, performance.now() - eventAt, transport.status);
  for (const key of ['instant', 'unit', 'average', 'liters', 'distance', 'model', 'note']) elements['fuel-' + key].textContent = fuel[key];
  elements['fuel-reset'].disabled = !event?.fuel;
  elements['fuel-calibrate'].disabled = !event?.fuel;
  elements['fuel-undo'].disabled = event?.fuel?.previous_factor == null;
  showTime('captured' , event?.captured_at);
  showTime('received', event?.received_at);
  elements.warnings.hidden = state.warnings.length === 0;
  elements.warnings.textContent = state.warnings.join(' · ');
}

const feed = new Feed({
  createEventSource: () => new EventSource('/events'),
  onEvent: data => { event = data; eventAt = performance.now(); },
  onState: state => { transport = state; render(); },
});
bindFuelControls(elements, () => event);
elements.reconnect.addEventListener('click', () => feed.start());
window.addEventListener('pagehide', () => feed.stop());
window.addEventListener('pageshow', e => { if (e.persisted) feed.start(); });
document.addEventListener('visibilitychange', render);
setInterval(render, 500); // Watchdog runs even when no telemetry messages arrive.
feed.start();
