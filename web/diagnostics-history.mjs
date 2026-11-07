import { sourceLabel } from './diagnostics-model.mjs';
const limit = 8 * 1024 * 1024;
const sessionID = id => typeof id === 'string' && /^[a-f0-9]{32}$/.test(id);
const incidentID = id => typeof id === 'string' && /^incident-[a-f0-9]{16}$/.test(id);
const errors = { 400: 'Неверный выбор истории. Обновите список.', 409: 'История недоступна или изменилась. Обновите список и повторите.',
  413: 'Ответ превышает 8 MiB. Выберите меньшее окно.', 429: 'Анализ занят. Повторите запрос позже.',
  503: 'История MIL не настроена. Укажите --mil-history-dir при запуске.', 504: 'Истекло время чтения истории. Повторите запрос.' };
export function historyURL(kind, session, incident) {
  if (!['report', 'incident'].includes(kind) || !sessionID(session) || (incident !== undefined && !incidentID(incident)) || kind === 'incident' && !incidentID(incident)) throw new Error('Неверный выбор истории');
  return `/api/mil/history/${kind}?${new URLSearchParams({ session, ...(incident ? { incident } : {}) })}`;
}
async function request(fetcher, url, signal) {
  const response = await fetcher(url, { signal, cache: 'no-store' });
  if (!response.ok) { const error = new Error(errors[response.status] ?? `Не удалось прочитать историю (HTTP ${response.status}).`); error.status = response.status; throw error; }
  if (Number(response.headers.get('content-length')) > limit) { await response.body?.cancel(); throw new Error('Ответ превышает 8 MiB.'); }
  const reader = response.body?.getReader();
  let bytes = 0, chunks = [];
  if (!reader) throw new Error('Пустой ответ истории.');
  try {
    while (true) { const { value, done } = await reader.read(); if (done) break; bytes += value.byteLength;
      if (bytes > limit) throw new Error('Ответ превышает 8 MiB.'); chunks.push(value); }
  } finally { if (bytes > limit) await reader.cancel(); reader.releaseLock(); }
  const data = new Uint8Array(bytes); let offset = 0;
  for (const chunk of chunks) { data.set(chunk, offset); offset += chunk.byteLength; }
  return JSON.parse(new TextDecoder().decode(data));
}
function validateCatalog(catalog) {
  if (catalog?.version !== 1 || sourceLabel(catalog.source) === 'UNKNOWN' || !Array.isArray(catalog.sessions) ||
    catalog.sessions.some(s => !sessionID(s.id) || !Number.isFinite(Date.parse(s.started_at))) || new Set(catalog.sessions.map(s => s.id)).size !== catalog.sessions.length) throw new Error('Неизвестная версия или повреждённый каталог MIL.');
}
function validateReport(report, session, source) {
  if (report?.version !== 1 || report.source !== source || !Array.isArray(report.sessions) || report.sessions.length !== 1 || report.sessions[0]?.id !== session ||
    report.sessions[0].source !== source || !Array.isArray(report.issues) || !Array.isArray(report.limitations) ||
    ['events', 'diagnostics', 'incidents', 'findings'].some(key => !Array.isArray(report.sessions[0][key]))) throw new Error('Неизвестная версия или повреждённый отчёт MIL. Обновите список.');
}

export class HistoryController {
  constructor({ fetch = globalThis.fetch, onState = () => {} } = {}) {
    this.fetch = fetch; this.onState = onState; this.epoch = 0; this.pending = null;
    this.state = { status: 'idle', catalog: null, session: '', source: 'UNKNOWN', report: null, reportURL: null, error: '' };
  }
  publish(patch) { this.state = { ...this.state, ...patch }; this.onState(this.state); }
  begin() {
    this.pending?.abort(); const control = new AbortController(); this.pending = control; const epoch = ++this.epoch;
    this.publish({ status: 'loading', report: null, reportURL: null, error: '' });
    return { control, epoch, timer: setTimeout(() => control.abort(), 10000) };
  }
  cancel() { this.epoch++; this.pending?.abort(); this.pending = null; this.publish({ status: 'canceled', report: null, reportURL: null, error: '' }); }
  async run(refresh, session) {
    const job = this.begin();
    try {
      if (refresh) {
        const catalog = await request(this.fetch, '/api/mil/history/sessions', job.control.signal); validateCatalog(catalog);
        if (job.epoch !== this.epoch) return;
        session = catalog.sessions.some(s => s.id === session) ? session : [...catalog.sessions].sort((a, b) => Date.parse(a.started_at) - Date.parse(b.started_at)).at(-1)?.id ?? '';
        this.publish({ catalog, source: sourceLabel(catalog.source), session });
        if (!session) { this.publish({ status: 'empty' }); return; }
      }
      if (!this.state.catalog?.sessions.some(s => s.id === session)) throw new Error('Выберите сеанс из каталога.');
      const url = historyURL('report', session);
      const report = await request(this.fetch, url, job.control.signal); validateReport(report, session, this.state.catalog.source);
      if (job.epoch !== this.epoch) return;
      if (job.control.signal.aborted) throw new DOMException('Timeout', 'AbortError');
      this.publish({ status: 'ready', report, reportURL: url, session });
    } catch (error) {
      if (job.epoch !== this.epoch) return;
      this.publish({ status: error.status === 503 ? 'disabled' : 'error', report: null, reportURL: null,
        error: error.name === 'AbortError' ? 'Чтение прервано или превысило 10 секунд. Повторите запрос.' : error.message });
    } finally { clearTimeout(job.timer); if (job.epoch === this.epoch) this.pending = null; }
  }
  refresh() { return this.run(true, this.state.session); }
  select(session) { this.publish({ session, report: null, reportURL: null }); return this.run(false, session); }
}
