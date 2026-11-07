const time = value => typeof value === 'string' && !value.startsWith('0001-') ? Date.parse(value) : NaN;
const validTime = (value, now) => Number.isFinite(time(value)) && time(value) <= now;
const fresh = (item, now, elapsed, ttl) => Number.isFinite(ttl) && ttl > 0 &&
  [item?.captured_at, item?.received_at].every(value => validTime(value, now) && now - time(value) + elapsed < ttl);
export const sourceLabel = source => source === 'live' ? 'LIVE' : ['simulator', 'sim'].includes(source) ? 'SIM' : 'UNKNOWN';
export const timeLabel = value => Number.isFinite(time(value)) ? new Date(value).toLocaleString('ru-RU', { timeZone: 'Europe/Moscow' }) + ' МСК' : '—';
const kinds = { empty: 'Доступный пустой ответ', codes: 'Сохранённые коды', not_queried: 'Нет чтения', unsupported: 'Не поддерживается',
  temporarily_unavailable: 'Временно недоступно', invalid_response: 'Некорректный ответ', stale: 'Исторический ответ' };

export function dtcView(command, reading, now) {
  let kind = reading ? reading.status : 'not_queried';
  let codes = [];
  if (kind === 'available' || kind === 'stale') {
    const detail = reading.detail;
    if (detail?.status !== 'available' || !Array.isArray(detail.codes) || !detail.codes.every(code => typeof code === 'string' && /^[PCBU][0-3][0-9A-F]{3}$/.test(code)) ||
      ![reading.captured_at, reading.received_at].every(value => validTime(value, now))) kind = 'invalid_response';
    else { codes = [...detail.codes]; if (kind === 'available') kind = codes.length ? 'codes' : 'empty'; }
  }
  if (reading && [reading.captured_at, reading.received_at].some(value => !validTime(value, now))) { kind = 'invalid_response'; codes = []; }
  return { command, kind, label: kinds[kind] ?? `Недоступно (${kind})`, codes, capturedAt: reading?.captured_at ?? null, receivedAt: reading?.received_at ?? null, raw: reading?.raw_response ?? '' };
}

export function milView(event, elapsedMs, transportStatus) {
  const now = time(event?.server_time), reading = event?.mil, source = sourceLabel(event?.source);
  const sourceFresh = transportStatus === 'open' && event?.health === 'connected' && Number.isFinite(elapsedMs) && elapsedMs >= 0 &&
    fresh(event, now, elapsedMs, event?.stale_after_ms);
  const available = source === 'LIVE' && (!reading?.source || reading.source === event.source) && sourceFresh && reading?.status === 'available' &&
    typeof reading.on === 'boolean' && Number.isInteger(reading.dtc_count) && reading.dtc_count >= 0 && reading.dtc_count <= 127 &&
    fresh(reading, now, elapsedMs, reading.stale_after_ms);
  const state = available ? reading.on ? 'ON' : 'OFF' : 'UNKNOWN';
  const dtcs = ['03', '07', '0A'].map(command => dtcView(command, event?.ecu_readings?.[command], now));
  const summary = source === 'SIM' ? 'MIL симулятором не предоставляется. Автомобиль не подключён.' : state === 'ON' && !dtcs.some(d => d.codes.length) ?
    'Check Engine включён. Причина не определена стандартными диагностическими режимами' : state === 'ON' ?
      'Check Engine включён. Последние известные DTC приведены отдельно; код не устанавливает неисправную деталь.' : state === 'OFF' ?
        'Свежий ответ 0101: Check Engine выключен. Это не заключение об исправности автомобиля.' : 'Нет достоверного свежего чтения MIL; UNKNOWN не означает выключение лампы.';
  const recording = event?.mil_recording;
  return { state, count: available ? reading.dtc_count : null, source, sourceFresh, summary, dtcs, capturedAt: reading?.captured_at,
    receivedAt: reading?.received_at, ageMs: Number.isFinite(now - time(reading?.captured_at) + elapsedMs) ? now - time(reading?.captured_at) + elapsedMs : null,
    recording: recording?.state === 'failed' ? `Ошибка записи MIL: ${recording.error ?? 'неизвестная ошибка'}` : recording?.state === 'recording' ? 'Запись MIL включена' : 'Запись MIL выключена',
    health: sourceFresh ? 'Свежий источник' : 'Источник или связь недоступны / устарели' };
}

// Only successes observed during this page opening may populate this label.
export class LiveMIL {
  constructor() { this.event = null; this.lastSuccess = null; this.generationFloor = -Infinity; }
  receive(event) {
    if (this.event && (event.generation !== this.event.generation || event.source !== this.event.source)) this.generationFloor = time(this.event.server_time);
    this.event = event;
    if (milView(this.current(), 0, 'open').state !== 'UNKNOWN') this.lastSuccess = event.mil.captured_at;
  }
  current() {
    if (this.event?.mil && time(this.event.mil.captured_at) <= this.generationFloor) return { ...this.event, mil: { ...this.event.mil, status: 'stale' } };
    return this.event;
  }
  view(elapsed, transport) {
    const age = time(this.event?.server_time) + elapsed - time(this.lastSuccess);
    return { ...milView(this.current(), elapsed, transport), lastSuccess: this.lastSuccess, lastSuccessAgeMs: Number.isFinite(age) && age >= 0 ? age : null };
  }
}
