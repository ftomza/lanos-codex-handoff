import { viewState, MAX_RETRIES } from './model.mjs';
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

const labels = {
  connected: ['Симулятор подключён', 'Показания актуальны', 'Демонстрационные данные обновляются в реальном времени.'],
  connecting: ['Подключение…', 'Ожидаем данные', 'Устанавливаем соединение с локальной приборкой.'],
  stale: ['Данные устарели', 'Нет свежих измерений', 'Последние показания скрыты. Ожидаем обновления источника.'],
  disconnected: ['Нет связи', 'Источник недоступен', 'Показания скрыты до восстановления соединения.'],
};

function showTime(id, value) {
  const time = Date.parse(value);
  elements[id].textContent = Number.isFinite(time) && time > 0 ? new Date(time).toLocaleTimeString('ru-RU') : '—';
  elements[id].title = value ?? '';
}

function render() {
  const state = viewState(event, performance.now() - eventAt, transport.status);
  for (const id of ['source', 'speed', 'rpm', 'coolant', 'voltage']) elements[id].textContent = state[id];
  const text = labels[state.health] ?? labels.disconnected;
  elements.connection.textContent = text[0];
  elements.connection.dataset.health = state.health;
  elements['health-panel'].dataset.health = state.health;
  elements['health-title'].textContent = text[1];
  let detail = text[2];
  if (event?.health === 'disconnected' && transport.status === 'open') detail = 'Симулятор имитирует обрыв связи на 6 секунд. Восстановление автоматическое.';
  if (transport.status === 'retrying' || (transport.status === 'connecting' && transport.attempt > 0)) detail = `Восстанавливаем связь. Попытка ${transport.attempt} из ${MAX_RETRIES}.`;
  if (transport.status === 'stopped') detail = 'Автоматические попытки завершены. Проверьте сервер и повторите подключение.';
  elements['health-detail'].textContent = detail;
  elements.reconnect.hidden = transport.status !== 'stopped';
  elements['driving-state'].textContent = state.fresh ? (event.speed_kmh > 0 ? 'Демонстрация движения' : 'Демонстрация холостого хода') : 'Нет актуальных показаний';
  elements['coolant-note'].textContent = state.fresh && event.coolant_c !== null ? 'Смоделированная температура' : 'Нет актуальных данных';
  elements['voltage-note'].textContent = state.fresh && event.voltage_v !== null ? 'Смоделированное напряжение' : 'Напряжение недоступно';
  elements['speed-arc'].style.strokeDashoffset = String(100 - (state.fresh ? Math.min(160, Math.max(0, event.speed_kmh ?? 0)) / 160 * 100 : 0));
  bars.forEach((bar, i) => bar.classList.toggle('active', state.fresh && i < (event.rpm ?? 0) / 6000 * bars.length));
  showTime('captured', event?.captured_at);
  showTime('received', event?.received_at);
  elements.warnings.hidden = state.warnings.length === 0;
  elements.warnings.textContent = state.warnings.join(' · ');
}

const feed = new Feed({
  createEventSource: () => new EventSource('/events'),
  onEvent: data => { event = data; eventAt = performance.now(); },
  onState: state => { transport = state; render(); },
});
elements.reconnect.addEventListener('click', () => feed.start());
window.addEventListener('pagehide', () => feed.stop());
window.addEventListener('pageshow', e => { if (e.persisted) feed.start(); });
document.addEventListener('visibilitychange', render);
setInterval(render, 500); // Watchdog runs even when no telemetry messages arrive.
feed.start();
