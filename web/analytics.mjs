import { graphSegments, historyView, number, timeLabel } from './analytics-model.mjs';

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
  const { canvas, caption } = chartNodes.get(key), lines = graphSegments(points), all = lines.flat();
  const ratio = Math.min(devicePixelRatio || 1, 2), width = Math.max(250, canvas.clientWidth), height = 210;
  canvas.width = width * ratio; canvas.height = height * ratio;
  const ctx = canvas.getContext('2d'); ctx.scale(ratio, ratio); ctx.clearRect(0, 0, width, height);
  caption.textContent = all.length ? `${all.length} сохранённых точек · разрывы не соединяются` : 'Нет применимых измерений';
  if (!all.length) return;
  const x0 = Math.min(...all.map(p => p.at)), x1 = Math.max(...all.map(p => p.at)), y0 = Math.min(0, ...all.map(p => p.value)), y1 = Math.max(y0 + 1, ...all.map(p => p.value));
  const x = t => 48 + (t - x0) / Math.max(1, x1 - x0) * (width - 64), y = v => height - 30 - (v - y0) / (y1 - y0) * (height - 50);
  ctx.strokeStyle = '#34483e'; ctx.lineWidth = 1; ctx.beginPath(); ctx.moveTo(48, 12); ctx.lineTo(48, height - 30); ctx.lineTo(width - 16, height - 30); ctx.stroke();
  ctx.fillStyle = '#b9c8bb'; ctx.font = '11px system-ui'; ctx.fillText(y1.toFixed(1), 2, 20); ctx.fillText(y0.toFixed(0), 8, height - 29);
  ctx.fillText(new Date(x0).toLocaleTimeString('ru-RU'), 48, height - 8); ctx.fillText(new Date(x1).toLocaleTimeString('ru-RU'), Math.max(48, width - 78), height - 8);
  ctx.strokeStyle = '#97d687'; ctx.fillStyle = '#97d687'; ctx.lineWidth = 1.5;
  for (const line of lines) { ctx.beginPath(); line.forEach((p, i) => i ? ctx.lineTo(x(p.at), y(p.value)) : ctx.moveTo(x(p.at), y(p.value))); ctx.stroke(); if (line.length === 1) { ctx.beginPath(); ctx.arc(x(line[0].at), y(line[0].value), 2, 0, Math.PI * 2); ctx.fill(); } }
  ctx.strokeStyle = '#d9aa56'; ctx.setLineDash([3, 4]);
  for (const marker of markers) { const t = Date.parse(marker.at); if (t < x0 || t > x1 || !Number.isFinite(t)) continue; ctx.beginPath(); ctx.moveTo(x(t), 12); ctx.lineTo(x(t), height - 30); ctx.stroke(); }
  ctx.setLineDash([]);
}
el['history-form'].addEventListener('submit', e => { e.preventDefault(); load(false); });
el['history-refresh'].addEventListener('click', () => load(true));
window.addEventListener('resize', drawAll);
window.addEventListener('pagehide', () => pending?.abort());
load(true);
