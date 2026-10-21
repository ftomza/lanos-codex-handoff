export const MAX_RETRIES = 6;

export function retryDelay(attempt) {
  return attempt >= MAX_RETRIES ? null : Math.min(1000 * 2 ** attempt, 10000);
}

const number = (value, digits = 0) => Number.isFinite(value) ? value.toFixed(digits) : '—';

export function viewState(event, elapsedMs, transport) {
  let health = event?.health ?? 'connecting';
  const ttl = Number.isFinite(event?.stale_after_ms) && event.stale_after_ms > 0 ? event.stale_after_ms : 3000;
  if (transport !== 'open') health = !event && transport === 'connecting' ? 'connecting' : 'disconnected';
  if (health === 'connected') {
    const serverTime = Date.parse(event.server_time);
    const ages = [event.captured_at, event.received_at].map(value => serverTime - Date.parse(value));
    if (elapsedMs >= ttl || ages.some(age => !Number.isFinite(age) || age < 0 || age + elapsedMs >= ttl)) health = 'stale';
  }
  const fresh = health === 'connected';
  const warnings = [];
  if (fresh && Number.isFinite(event.coolant_c) && event.coolant_c >= 105) warnings.push('Высокая температура охлаждающей жидкости');
  if (fresh && Number.isFinite(event.voltage_v) && (event.voltage_v < 12 || event.voltage_v > 15.2)) warnings.push('Напряжение вне обычного диапазона');
  return {
    health, fresh, warnings,
    source: event?.source === 'simulator' ? 'SIMULATOR' : event?.source === 'live' ? 'LIVE' : 'UNKNOWN',
    speed: fresh ? number(event.speed_kmh) : '—',
    rpm: fresh ? number(event.rpm) : '—',
    coolant: fresh ? number(event.coolant_c) : '—',
    voltage: fresh ? number(event.voltage_v, 1) : '—',
  };
}
