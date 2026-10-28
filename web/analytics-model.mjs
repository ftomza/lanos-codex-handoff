export const number = (value, digits = 2) => Number.isFinite(value) && value >= 0 ? value.toFixed(digits) : '—';
export const timeLabel = value => Number.isFinite(Date.parse(value)) ? new Date(value).toLocaleString('ru-RU') : '—';

export function graphSegments(points = []) {
  const result = [];
  let line = [], last = -Infinity;
  const flush = () => { if (line.length) result.push(line); line = []; last = -Infinity; };
  for (const point of points.slice(0, 4000)) {
    const at = Date.parse(point.at);
    if (point.break || at < last) flush();
    if (!Number.isFinite(at) || !Number.isFinite(point.value)) { flush(); continue; }
    line.push({ at, value: point.value });
    last = at;
  }
  flush();
  return result;
}

export function historyView(report) {
  const f = report?.last?.sample?.fuel;
  const empty = report?.samples === 0;
  const notes = [];
  if (report?.incomplete) notes.push('Есть пропуски: отчёт охватывает только сохранённые наблюдаемые участки.');
  if (report?.retention_truncated) notes.push('Часть старой истории удалена при ротации.');
  if (report?.downsampled) notes.push('Графики прорежены с сохранением экстремумов и разрывов.');
  notes.push('Качество входов и покрытие не доказывают точность расхода.');
  return {
    source: `${report?.source === 'live' ? 'LIVE' : report?.source === 'simulator' ? 'SIMULATOR' : 'UNKNOWN'} · ИСТОРИЯ`,
    liters: number(empty ? null : report?.liters, 3), distance: number(empty ? null : report?.distance_km, 3),
    average: number(report?.average_l100, 2), speed: number(report?.average_speed_kmh, 1),
    maxSpeed: number(report?.max_speed_kmh, 1), duration: number(report?.duration_seconds, 1),
    coverage: number(report?.coverage_percent, 1), gaps: number(report?.gap_seconds, 1),
    model: f ? `${f.method || 'модель неизвестна'} · ${f.quality || 'качество не указано'} · ${f.factor === 1 ? 'не откалибровано' : `поправка ×${number(f.factor, 3)}`}` : 'Нет сохранённых измерений',
    note: notes.join(' '), range: empty ? 'В выбранном диапазоне нет сохранённых измерений' : `${timeLabel(report?.from)} — ${timeLabel(report?.to)}`,
  };
}
