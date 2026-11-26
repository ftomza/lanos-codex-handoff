import { dtcView, renderDtc } from './dtc.mjs';

const charts = [
  ['speed_kmh', 'Скорость', 'км/ч'], ['rpm', 'Обороты', 'об/мин'],
  ['map_kpa', 'Давление во впуске', 'кПа'], ['iat_c', 'Температура впуска', '°C'],
  ['flow_lph', 'Мгновенный расход, оценка', 'л/ч'], ['voltage_v', 'Напряжение адаптера', 'В'],
  ['liters', 'Топливо с момента сброса', 'л'], ['average_l100', 'Средний расход с момента сброса', 'л/100 км'],
];
const markerNames = new Map(Object.entries({
  gap: 'Пропуск данных', coverage_resumed: 'Учёт расхода возобновился',
  waiting_ecu: 'Ожидание ЭБУ', connection_lost: 'Потеря связи', live_returned: 'Связь восстановлена',
  mil_on: 'Check Engine загорелся', mil_already_on: 'Check Engine уже горел',
  mil_after_gap: 'Check Engine горит после пропуска', mil_off: 'ЭБУ сообщает, что Check Engine погас',
  mil_off_after_gap: 'Check Engine не горит после пропуска',
  mil_dtc_result: 'Сохранённые ответы ЭБУ по Check Engine',
  fuel_reset: 'Сброс счётчиков', fuel_changed: 'Изменение настроек расхода',
  vgate_power_cycle: 'Подтверждено отключение питания Vgate',
}));
const milOnMarkers = new Set(['mil_on', 'mil_already_on', 'mil_after_gap']);

export function elapsedLabel(ms) {
  const s = Math.max(0, Math.floor(ms / 1000));
  const minutes = String(Math.floor(s / 60) % 60).padStart(2, '0');
  const seconds = String(s % 60).padStart(2, '0');
  const hours = Math.floor(s / 3600);
  return (hours ? hours + ':' : '') + minutes + ':' + seconds;
}

export function historyDisplay(report) {
  const from = Date.parse(report.from), to = Date.parse(report.to);
  const notes = [];
  const metric = (value, digits) => Number.isFinite(value) && value >= 0 ?
    new Intl.NumberFormat('ru-RU', { minimumFractionDigits: digits, maximumFractionDigits: digits }).format(value) : '—';
  if (!report.samples) notes.push('В записи нет сохранённых измерений.');
  if (report.incomplete) notes.push('Запись неполная.');
  if (report.bad_rows > 0) notes.push('Пропущено повреждённых строк — ' + report.bad_rows + '.');
  if (report.omitted_markers > 0) notes.push('Пропущено событий из-за лимита — ' + report.omitted_markers + '.');
  if (report.downsampled) notes.push('Графики прорежены с сохранением выбросов и разрывов.');
  return {
    from, to, distance: metric(report.distance_km, 2), liters: metric(report.liters, 2),
    average: metric(report.average_l100, 1), coverage: metric(report.coverage_percent, 1), note: notes.join(' '),
    markers: (report.markers ?? []).map(marker => ({
      ...marker, elapsed: Date.parse(marker.at) - from, label: markerNames.get(marker.kind),
    })).filter(marker => marker.label && marker.elapsed >= 0 && marker.elapsed <= to - from),
  };
}

async function run() {
  const el = id => document.getElementById('trip-' + id);
  try {
    const [{ drawHistoryChart }, response] = await Promise.all([
      import('./history-chart.mjs'),
      fetch('/trip-history-report.json', { cache: 'no-store' }),
    ]);
    if (!response.ok) throw new Error('report unavailable');
    const report = await response.json(), view = historyDisplay(report);
    if (!Number.isFinite(view.from) || !Number.isFinite(view.to) || view.to < view.from) throw new Error('invalid report');
    el('range').textContent = new Date(view.from).toLocaleString('ru-RU') + ' · ' + elapsedLabel(view.to - view.from);
    for (const name of ['distance', 'liters', 'average', 'coverage', 'note']) el(name).textContent = view[name];

    const range = el('position');
    range.max = view.to - view.from;
    range.disabled = view.to === view.from;
    if (view.markers.length) {
      el('events').hidden = false;
      let expanded = null;
      for (const [index, marker] of view.markers.entries()) {
        const item = document.createElement('li'), button = document.createElement('button');
        button.type = 'button';
        button.textContent = elapsedLabel(marker.elapsed) + ' · ' + marker.label;
        item.append(button);
        const lampOn = milOnMarkers.has(marker.kind);
        const panel = lampOn || marker.kind === 'mil_dtc_result' ? document.createElement('div') : null;
        if (panel) {
          panel.id = 'trip-dtc-' + index;
          panel.className = 'trip-dtc';
          panel.hidden = true;
          button.setAttribute('aria-controls', panel.id);
          button.setAttribute('aria-expanded', 'false');
          item.append(panel);
        }
        button.addEventListener('click', () => {
          range.value = marker.elapsed;
          draw();
          const open = panel && expanded?.panel !== panel;
          if (expanded) {
            expanded.panel.hidden = true;
            expanded.button.setAttribute('aria-expanded', 'false');
            expanded = null;
          }
          if (open) {
            renderDtc(document, panel, dtcView(marker.dtc_replies, { lampOn }));
            panel.hidden = false;
            button.setAttribute('aria-expanded', 'true');
            expanded = { panel, button };
          }
        });
        el('markers').append(item);
      }
    }

    const nodes = charts.map(([key, title, unit]) => {
      const card = document.createElement('article'), heading = document.createElement('h2');
      const canvas = document.createElement('canvas'), caption = document.createElement('p');
      card.className = 'card history-chart';
      heading.textContent = title + ', ' + unit;
      canvas.setAttribute('role', 'img');
      canvas.setAttribute('aria-label', heading.textContent);
      card.append(heading, canvas, caption);
      el('charts').append(card);
      return { key, canvas, caption };
    });
    el('report').hidden = false;

    function draw() {
      const position = Number(range.value);
      el('elapsed').value = elapsedLabel(position);
      for (const { key, canvas, caption } of nodes) {
        drawHistoryChart(canvas, caption, report.series[key] ?? [], view.markers, {
          from: view.from, to: view.to, cursor: view.from + position, labelTime: t => elapsedLabel(t - view.from),
        });
      }
    }
    draw();
    let frame;
    function scheduleDraw() {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(draw);
    }
    range.addEventListener('input', scheduleDraw);
    window.addEventListener('resize', scheduleDraw);
  } catch {
    el('range').textContent = 'Поездка';
    el('error').textContent = 'Не удалось открыть отчёт. Вернитесь к списку и попробуйте ещё раз.';
    el('error').hidden = false;
  }
}

if (typeof document !== 'undefined') run();
