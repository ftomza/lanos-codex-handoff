const number = (value, digits) => Number.isFinite(value) && value >= 0 ? value.toFixed(digits) : '—';
const valid = (until, event, elapsed) => {
  const remaining = Date.parse(until) - Date.parse(event?.server_time);
  return Number.isFinite(remaining) && Number.isFinite(elapsed) && elapsed >= 0 && remaining > elapsed;
};
const reasons = {
  unverified_iat: 'IAT не проверен: LIVE-оценка выключена',
  iat_unavailable_or_out_of_range: 'IAT отсутствует или вне диапазона модели',
  model_changed_reset_required: 'Модель изменилась: нужен новый отсчёт',
  no_supported_model: 'Недостаточно поддерживаемых входов',
  rpm_unavailable: 'Нет свежих оборотов', map_unavailable: 'Нет свежего MAP',
  maf_unavailable: 'Нет свежего MAF', fuel_rate_unavailable: 'Нет свежего расхода ЭБУ',
  source_unavailable: 'Нет свежих данных источника', expired: 'Мгновенный расход устарел',
  flow_out_of_range: 'Расход вне допустимого диапазона модели',
};
export function fuelView(event, elapsedMs, transport) {
  const f = event?.fuel;
  if (!f) return { instant: '—', unit: 'л/ч', average: '—', liters: '—', distance: '—', model: 'Ожидание / модуль выключен', note: 'Нет данных топливного компьютера' };
  const live = event.source === 'live';
  const connected = ['live', 'simulator'].includes(event.source) && event.health === 'connected' && transport === 'open';
  const flowFresh = connected && f.status === 'available' && valid(f.flow_until, event, elapsedMs) && Number.isFinite(f.flow_lph) && f.flow_lph >= 0;
  const speedFresh = connected && valid(f.speed_until, event, elapsedMs) && Number.isFinite(f.speed_kmh) && f.speed_kmh >= 0;
  const perDistance = flowFresh && speedFresh && f.speed_kmh >= (f.low_speed_kmh ?? 5);
  const notes = [];
  if (f.incomplete) notes.push(`Частичное покрытие: ${number(f.covered_km, 2)} км с топливом`);
  if (f.settings?.unverified_iat) notes.push('IAT не проверен');
  if (!flowFresh) notes.push(reasons[f.reason] ?? 'Мгновенный расход недоступен');
  if (!speedFresh) notes.push('Скорость недоступна');
  if (f.persistence === 'persistence_error') notes.push('Ошибка сохранения — итоги не сохранены');
  if (f.persistence === 'pending') notes.push('Сохранение каждые 5 с');
  const method = { speed_density: 'Оценка MAP/RPM/IAT', maf: 'Оценка MAF', direct: 'Расход ЭБУ (точность не проверена)' }[f.method] ?? 'Ожидание модели';
  const calibration = f.factor === 1 ? 'не откалибровано' : `поправка ×${number(f.factor, 3)}`;
  return {
    instant: flowFresh ? number(perDistance ? f.flow_lph * 100 / f.speed_kmh : f.flow_lph, 1) : '—',
    unit: perDistance ? 'л/100 км' : 'л/ч', average: number(f.average_l100, 1),
    liters: number(f.liters, 2), distance: number(f.distance_km, 2),
    model: `${live ? 'LIVE' : event.source === 'simulator' ? 'SIMULATOR' : 'UNKNOWN'} · ${method} · ${calibration}`,
    note: notes.join(' · ') || 'Учтённые интервалы после сброса; не измерение уровня бака',
  };
}
