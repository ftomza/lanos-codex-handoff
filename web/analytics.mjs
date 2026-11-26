import { historyView, number, timeLabel } from './analytics-model.mjs';
import { drawHistoryChart } from './history-chart.mjs';

const el = Object.fromEntries([...document.querySelectorAll('[id]')].map(node => [node.id, node]));
const charts = [ ['rpm', 'Обороты', 'об/мин'], ['speed_kmh', 'Скорость', 'км/ч'],
  ['map_kpa', 'Давление во впуске', 'kPa'], ['flow_lph', 'Мгновенный расход, оценка', 'л/ч'],
  ['liters', 'Накопленное топливо с момента сброса', 'л'], ['average_l100', 'Средний с момента сброса', 'л/100 км'] ];
const modeNames = { moving: 'Движение', idle: 'Холостой ход', engine_off: 'Двигатель остановлен', unknown: 'Неизвестно / неполный интервал' };
const issues = { iat_constant: 'IAT долго не менялся', iat_jump: 'Резкий скачок IAT', iat_out_of_range: 'IAT вне диапазона модели',
  iat_cold_mismatch: 'Разница IAT и ОЖ при отмеченном холодном запуске', reset: 'Сброс счётчиков', availability_changed: 'Доступность данных изменилась',
  gap: 'Пропуск / граница измерений', sequence_gap: 'Пропущены строки записи', partial_tail: 'Незавершённая последняя строка', incomplete_session: 'Сессия завершена не полностью' };
let lastReport = null, pending = null;
const chartNodes = new Map();
for (const [key, title, unit] of charts) {
  const card = document.createElement('article'); card.className = 'card history-chart';
  const heading = document.createElement('h2'); heading.textContent = `${title}, ${unit}`;
  const canvas = document.createElement('canvas'); canvas.setAttribute('role', 'img'); canvas.setAttribute('aria-label', `${title}: сохранённые значения с разрывами`);
  const caption = document.createElement('p'); caption.textContent = 'Нет измерений';
  card.append(heading, canvas, caption); el['history-charts'].append(card); chartNodes.set(key, { canvas, caption });
}
function row(parent, values) {
  const tr = document.createElement('tr');
  for (const value of values) { const td = document.createElement('td'); td.textContent = value; tr.append(td); }
  parent.append(tr);
}
function query() {
  const params = new URLSearchParams();
  if (el['history-session'].value) params.set('session', el['history-session'].value);
  for (const key of ['from', 'to']) if (el[`history-${key}`].value) {
    const value = new Date(el[`history-${key}`].value); if (!Number.isFinite(+value)) throw new Error('Проверьте границы времени.');
    params.set(key, value.toISOString());
  }
  return `/api/fuel/history/report?${params}`;
}
async function request(url, signal) {
  const response = await fetch(url, { signal, cache: 'no-store' });
  const text = await response.text();
  if (!response.ok) throw new Error(response.status === 503 ? 'История не настроена. Запустите приборку с каталогом записи или сохранённой истории.' : text.slice(0, 700));
  if (text.length > 8 * 1024 * 1024) throw new Error('Отчёт слишком велик. Выберите одну сессию или меньший диапазон.');
  return JSON.parse(text);
}
async function load(refresh) {
  pending?.abort(); const control = new AbortController(); pending = control;
  const timer = setTimeout(() => control.abort(), 12000);
  el['history-error'].hidden = true; el['history-state'].textContent = 'Читаем сохранённые измерения…'; el['history-load'].disabled = true;
  try {
    if (refresh) {
      const sessions = await request('/api/fuel/history/sessions', control.signal);
      const selected = el['history-session'].value; el['history-session'].replaceChildren();
      const all = document.createElement('option'); all.value = ''; all.textContent = 'Все сохранённые сессии'; el['history-session'].append(all);
      for (const session of sessions) { const option = document.createElement('option'); option.value = session.id;
        option.textContent = `${session.source === 'live' ? 'LIVE' : 'SIM'} · ${timeLabel(session.started_at)} · ${session.closed ? 'закрытые файлы' : 'идёт запись'}`; el['history-session'].append(option); }
      el['history-session'].value = sessions.some(s => s.id === selected) ? selected : sessions.at(-1)?.id ?? '';
      if (!sessions.length) throw new Error('Сохранённых сессий пока нет.');
    }
    const url = query(); const report = await request(url, control.signal);
    if (pending !== control) return;
    lastReport = report; render(report);
    el['history-export'].href = url; el['history-export'].hidden = false;
    el['history-state'].textContent = `Сохранённый отчёт · ${report.rows ?? 0} строк. Для новых данных нажмите «Показать».`;
  } catch (error) {
    if (pending !== control) return;
    el['history-error'].textContent = error.name === 'AbortError' ? 'Чтение истории прервано или превысило время ожидания.' : error.message;
    el['history-error'].hidden = false; el['history-state'].textContent = 'История не загружена.';
  } finally { clearTimeout(timer); if (pending === control) el['history-load'].disabled = false; }
}
function render(report) {
  const view = historyView(report);
  for (const key of ['source', 'model', 'range', 'liters', 'distance', 'average', 'coverage', 'note']) el[`history-${key}`].textContent = view[key];
  el['history-timing'].textContent = `Наблюдалось ${view.duration} с; пропуски ${view.gaps} с. Средняя / максимальная наблюдавшаяся скорость: ${view.speed} / ${view.maxSpeed} км/ч.`;
  for (const id of ['modes', 'bins', 'segments', 'issues', 'inputs']) el[`history-${id}`].replaceChildren();
  for (const [name, stats] of Object.entries(report.modes ?? {})) row(el['history-modes'], [modeNames[name] ?? name, number(stats.seconds, 1), number(stats.liters, 4), number(stats.distance_km, 3), number(stats.seconds > 0 ? stats.liters * 3600 / stats.seconds : null, 2)]);
  for (const [key, bins] of Object.entries(report.distributions ?? {})) for (const bin of bins) row(el['history-bins'], [`${key} · ${bin.label === 'unknown' ? 'неизвестно' : bin.label === 'mixed' ? 'переход между диапазонами' : bin.label}`, number(bin.seconds, 1), number(bin.liters, 4), number(bin.covered_seconds, 1)]);
  for (const segment of report.segments ?? []) row(el['history-segments'], [timeLabel(segment.from), number(segment.seconds, 1), number(segment.liters, 4), number(segment.distance_km, 3), number(segment.average_l100, 2)]);
  for (const item of [...(report.issues ?? []), ...(report.anomalies ?? []), ...(report.markers ?? [])]) { const li = document.createElement('li'); li.textContent = `${item.at ? `${timeLabel(item.at)} · ` : ''}${issues[item.code ?? item.reason] ?? item.code ?? item.reason}${item.detail ? ` · ${item.detail}` : ''}`; el['history-issues'].append(li); }
  el['history-limits'].textContent = `Признаков IAT: ${report.anomaly_count ?? 0}. Списки событий и деталей ограничены; полные сохранённые строки анализируются до прореживания графиков.`;
  const saved = report.last?.sample;
  el['history-comparison'].textContent = `Из счётчиков записи: ${number(report.liters, 5)} л. Интегрирование сохранённых точек для сравнения: ${number(report.sampled_liters, 5)} л. Разница может зависеть от частоты записи и покрытия.`;
  el['history-terms'].textContent = saved?.fuel?.intermediates ? JSON.stringify({settings:saved.fuel.settings,calculation:saved.fuel.intermediates,cold_start_check:saved.fuel.cold_start_check}, null, 2) : 'Промежуточный расчёт speed-density недоступен для последнего наблюдения.';
  for (const [key, signal] of Object.entries(saved?.snapshot?.signals ?? {})) {
    const age = saved?.signal_ages?.[key];
    row(el['history-inputs'], [key, `${Number.isFinite(signal.value) ? signal.value : '—'} ${signal.unit ?? ''}`, number(age?.captured_ms, 0), `${age?.applicable ? 'применим' : 'не применим'} · ${signal.status} · ${signal.source}`]);
  }
  drawAll();
}
function drawAll() { for (const [key] of charts) draw(key, lastReport?.series?.[key] ?? [], lastReport?.markers ?? []); }
function draw(key, points, markers) {
  const { canvas, caption } = chartNodes.get(key);
  drawHistoryChart(canvas, caption, points, markers);
}
el['history-form'].addEventListener('submit', e => { e.preventDefault(); load(false); });
el['history-refresh'].addEventListener('click', () => load(true));
window.addEventListener('resize', drawAll);
window.addEventListener('pagehide', () => pending?.abort());
load(true);
