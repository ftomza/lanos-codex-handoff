export const MAX_RETRIES = 6;

export function retryDelay(attempt) {
  return attempt >= MAX_RETRIES ? null : Math.min(1000 * 2 ** attempt, 10000);
}

const number = (value, digits = 0) => Number.isFinite(value) ? value.toFixed(digits) : '—';
const freshTimes = (item, serverTime, elapsedMs, ttl) => Number.isFinite(ttl) && ttl > 0 &&
  [item?.captured_at, item?.received_at].every(value => {
    const age = serverTime - Date.parse(value);
    return Number.isFinite(age) && age >= 0 && age + elapsedMs < ttl;
  });

export function viewState(event, elapsedMs, transport) {
  let health = event?.health ?? 'connecting';
  const ttl = Number.isFinite(event?.stale_after_ms) && event.stale_after_ms > 0 ? event.stale_after_ms : 3000;
  const serverTime = Date.parse(event?.server_time);
  if (transport !== 'open') health = !event && transport === 'connecting' ? 'connecting' : 'disconnected';
  if (health === 'connected' && !freshTimes(event, serverTime, elapsedMs, ttl)) health = 'stale';
  const fresh = health === 'connected';
  const values = Object.fromEntries(['rpm', 'speed_kmh', 'coolant_c', 'voltage_v'].map(key => {
    const signal = event?.signals?.[key];
    const value = event?.signals ? signal?.value : event?.[key];
    const available = !event?.signals || (signal?.status === 'available' &&
      freshTimes(signal, serverTime, elapsedMs, signal.stale_after_ms));
    return [key, fresh && available && Number.isFinite(value) ? value : null];
  }));
  const warnings = [];
  if (values.coolant_c !== null && values.coolant_c >= 105) warnings.push('Высокая температура охлаждающей жидкости');
  if (values.voltage_v !== null && (values.voltage_v < 12 || values.voltage_v > 15.2)) warnings.push('Напряжение вне обычного диапазона');
  return {
    health, fresh, warnings, values,
    source: event?.source === 'simulator' ? 'SIMULATOR' : event?.source === 'live' ? 'LIVE' : 'UNKNOWN',
    speed: number(values.speed_kmh), rpm: number(values.rpm),
    coolant: number(values.coolant_c), voltage: number(values.voltage_v, 1),
  };
}

export function presentation(event, state, transport) {
  const sim = state.source === 'SIMULATOR';
  const live = state.source === 'LIVE';
  const labels = {
    connected: [sim ? 'Симулятор подключён' : live ? 'Источник подключён' : 'Источник неизвестен',
      'Показания обновляются', sim ? 'Демонстрационные данные.' : 'Не все сигналы обновляются одновременно.'],
    connecting: ['Подключение…', 'Ожидаем данные', 'Устанавливаем соединение с источником.'],
    stale: ['Данные устарели', 'Нет свежих измерений', 'Последние показания скрыты. Ожидаем обновления источника.'],
    disconnected: ['Нет связи', 'Источник недоступен', 'Показания скрыты до восстановления соединения.'],
  };
  const [connection, title, fallback] = labels[state.health] ?? labels.disconnected;
  let detail = fallback;
  if (transport.status === 'open' && ['connecting', 'disconnected'].includes(state.health)) {
    if (sim && state.health === 'disconnected') detail = 'Симулятор имитирует обрыв связи на 6 секунд. Восстановление автоматическое.';
    if (live && event?.message) detail = event.message;
  }
  if (transport.status === 'retrying' || (transport.status === 'connecting' && transport.attempt > 0)) detail = `Восстанавливаем связь с сервером. Попытка ${transport.attempt} из ${MAX_RETRIES}.`;
  if (transport.status === 'stopped') detail = 'Автоматические попытки завершены. Проверьте сервер и повторите подключение.';
  let driving = 'Нет актуальных показаний';
  if (state.values.speed_kmh !== null) driving = sim ?
    (state.values.speed_kmh > 0 ? 'Демонстрация движения' : 'Демонстрация холостого хода') :
    (state.values.speed_kmh > 0 ? 'Автомобиль движется' : 'Скорость 0 км/ч');
  return {
    connection, title, detail, driving,
    noticeTitle: sim ? 'Демонстрационный режим' : live ? 'Данные автомобиля' : 'Ожидание источника',
    noticeDetail: sim ? 'Все показания смоделированы. Автомобиль не подключён.' : live ?
      'Read-only OBD. Недоступные и устаревшие показания скрываются.' : 'Источник ещё не определён.',
    noticeTag: sim ? 'DEMO' : live ? 'LIVE' : 'WAIT',
    rpmNote: sim ? 'Данные демонстрационного цикла' : live ? 'Обороты по OBD' : 'Ожидание источника',
    coolantNote: state.values.coolant_c !== null ? (sim ? 'Смоделированная температура' : 'Температура по OBD') : unavailable(event, 'coolant_c'),
    voltageNote: state.values.voltage_v !== null ? (sim ? 'Смоделированное напряжение' : 'Питание адаптера в разъёме OBD') : unavailable(event, 'voltage_v'),
  };
}

function unavailable(event, key) {
  const signal = event?.signals?.[key];
  if (signal?.support === 'unsupported') return 'Не поддерживается ЭБУ';
  if (signal?.status === 'stale') return 'Данные устарели';
  if (['temporarily_unavailable', 'invalid_response'].includes(signal?.status)) return 'Временно недоступно';
  if (signal?.support === 'not_yet_tested') return 'Доступность ещё не проверена';
  return 'Нет актуальных данных';
}
