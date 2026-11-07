import { Feed } from './feed.mjs';
import { LiveMIL, dtcView, timeLabel } from './diagnostics-model.mjs';
import { HistoryController, historyURL } from './diagnostics-history.mjs';

function node(document, tag, text, className) {
  const element = document.createElement(tag); if (text !== undefined) element.textContent = String(text);
  if (className) element.className = className; return element;
}
function details(document, parent, title, value) {
  const block = node(document, 'details'); block.append(node(document, 'summary', title), node(document, 'pre', JSON.stringify(value, null, 2))); parent.append(block);
}
function list(document, parent, title, values) {
  parent.append(node(document, 'h3', title)); const ul = node(document, 'ul');
  for (const value of values ?? []) ul.append(node(document, 'li', typeof value === 'string' ? value : JSON.stringify(value, null, 2)));
  if (!ul.children.length) ul.append(node(document, 'li', 'Нет сохранённых данных')); parent.append(ul);
}
function findings(document, parent, items) {
  if (!items?.length) parent.append(node(document, 'p', 'Находок не сохранено. Это не подтверждает исправность автомобиля.'));
  for (const finding of items ?? []) {
    const card = node(document, 'article', undefined, 'mil-finding');
    card.append(node(document, 'h3', `${finding.code} · ${finding.symptom}`), node(document, 'p', `${finding.confidence === 'observed' ? 'Наблюдаемый факт' : finding.confidence === 'limited_hypothesis' ? 'Гипотеза для проверки' : finding.confidence} · Наблюдений: ${finding.count} · ${timeLabel(finding.first_observed_at)} — ${timeLabel(finding.last_observed_at)}`));
    list(document, card, 'Подтверждающие измерения · file/line/sequence, время и сигнал', finding.supporting_evidence);
    list(document, card, 'Противоречащие измерения', finding.contradicting_evidence);
    list(document, card, 'Недостающие данные', finding.missing_data); list(document, card, 'Следующая проверка', finding.next_check);
    parent.append(card);
  }
}
function diagnostic(document, parent, item) {
  const block = node(document, 'details'); const freeze = item.command?.startsWith('02');
  const dtc = ['03', '07', '0A'].includes(item.command) ? dtcView(item.command, item.reading, Infinity) : null;
  block.append(node(document, 'summary', `${freeze ? 'Freeze frame · отдельно от live' : 'DTC'} · ${item.command} · ${dtc?.label ?? item.reading?.status ?? 'недоступно'} · ${timeLabel(item.reading?.captured_at)} · ${item.incident ? `окно ${item.incident}` : 'независимый поздний / сеансовый ответ без привязки к окну'}`),
    node(document, 'p', `Получено: ${timeLabel(item.reading?.received_at)}. Host response time не определяет время возникновения неисправности.`),
    node(document, 'pre', JSON.stringify(item, null, 2))); parent.append(block);
}

// All archive values enter the DOM through textContent; never parse saved HTML.
export function renderSaved(document, parent, report) {
  parent.replaceChildren();
  const overview = node(document, 'section', undefined, 'card history-table');
  overview.append(node(document, 'h2', report.incomplete ? 'НЕПОЛНАЯ ИСТОРИЯ · сохранившиеся доказательства' : 'Сохранённый отчёт'), node(document, 'p', 'Отсутствие находок не означает исправность. Редкие O2 не дают заключения о катализаторе; задержки не устанавливают причину CPU/BLE/ЭБУ.'));
  list(document, overview, 'Проблемы архива и пропуски', report.issues); list(document, overview, 'Ограничения анализа', report.limitations); parent.append(overview);
  for (const session of report.sessions) {
    const card = node(document, 'section', undefined, 'card history-table');
    card.append(node(document, 'h2', `Сеанс ${session.id}`), node(document, 'p', session.summary ?? 'Нет сохранённого заключения'));
    details(document, card, 'Границы и полнота сеанса', { source: session.source, started_at: session.started_at, ended_at: session.ended_at, closed: session.closed, incomplete: session.incomplete, end_reason: session.end_reason, error: session.error });
    const events = node(document, 'details'); events.append(node(document, 'summary', `История MIL · событий: ${session.events.length}`));
    for (const item of session.events) details(document, events, `${timeLabel(item.event?.at ?? item.at)} · ${item.event?.kind} · ${item.event?.state} · предыдущая граница: ${timeLabel(item.event?.previous_captured_at)}`, item);
    if (!session.events.length) events.append(node(document, 'p', 'Событий нет в сохранившейся выборке. Состояние двигателя не установлено.'));
    card.append(events);
    const replies = node(document, 'details'); replies.append(node(document, 'summary', `Сохранённые DTC и freeze replies · ${session.diagnostics.length}`));
    for (const item of session.diagnostics) diagnostic(document, replies, item);
    card.append(replies); const observations = node(document, 'details'); observations.append(node(document, 'summary', 'Наблюдения по сеансу')); findings(document, observations, session.findings); card.append(observations);
    for (const incident of session.incidents) {
      const block = node(document, 'article', undefined, 'mil-incident');
      block.append(node(document, 'h3', `Окно ${incident.id} · ${incident.incomplete ? 'неполный контекст' : 'сохранённый контекст'}`), node(document, 'p', `Кадров: ${incident.frame_count}; ${incident.closed ? 'есть граница завершения' : 'открыто / нет границы завершения'}`));
      try { const link = node(document, 'a', 'Снимок события JSON', 'history-link'); link.href = historyURL('incident', session.id, incident.id); link.download = 'mil-incident.json'; block.append(link); } catch { block.append(node(document, 'p', 'Некорректный идентификатор: скачивание недоступно.')); }
      details(document, block, 'Сохранённые границы окна, retained / had_gaps и ограничения', incident.window);
      const checks = node(document, 'details'); checks.append(node(document, 'summary', 'Проверенные правила и недостаток данных'));
      for (const check of incident.rule_checks ?? []) checks.append(node(document, 'p', `${check.status === 'insufficient_data' ? 'Недостаточно данных' : check.status} · ${check.code} · ${check.detail}`));
      block.append(checks); const observations = node(document, 'details'); observations.append(node(document, 'summary', 'Наблюдения по окну')); findings(document, observations, incident.findings); block.append(observations); card.append(block);
    }
    parent.append(card);
  }
}

const dtcSignatures = new WeakMap();
export function renderDTCs(document, parent, dtcs) {
  const signature = JSON.stringify(dtcs);
  if (dtcSignatures.get(parent) === signature) return;
  dtcSignatures.set(parent, signature); parent.replaceChildren();
  for (const dtc of dtcs) {
    const card = node(document, 'article'); card.append(node(document, 'h3', `${({ '03': 'Stored', '07': 'Pending', '0A': 'Permanent' })[dtc.command]} · ${dtc.label}`),
      node(document, 'p', dtc.codes.join(', ') || '—'), node(document, 'p', `Последний известный ответ: ${timeLabel(dtc.capturedAt)} · получено: ${timeLabel(dtc.receivedAt)}`));
    details(document, card, 'Сохранённый raw', dtc.raw); parent.append(card);
  }
}

function start() {
  const el = Object.fromEntries([...document.querySelectorAll('[id]')].map(element => [element.id, element]));
  const live = new LiveMIL(); let received = performance.now(), transport = { status: 'connecting', attempt: 0 }, catalog = null, rendered = null;
  function renderCurrent() {
    const view = live.view(performance.now() - received, transport.status);
    el['mil-source'].textContent = `${view.source} · ТЕКУЩИЙ ИСТОЧНИК`;
    el['mil-state'].textContent = view.state; el['mil-state'].dataset.state = view.state;
    el['mil-count'].textContent = view.count ?? '—'; el['mil-summary'].textContent = view.summary;
    el['mil-health'].textContent = `${view.health} · SSE: ${transport.status}${transport.attempt ? ` · попытка ${transport.attempt}/6` : ''}`;
    el['mil-time'].textContent = `Ответ MIL: ${timeLabel(view.capturedAt)} · получено: ${timeLabel(view.receivedAt)} · возраст: ${view.ageMs === null ? '—' : `${(view.ageMs / 1000).toFixed(1)} с`}`;
    el['mil-success'].textContent = `Последнее успешное чтение при этом открытии страницы: ${timeLabel(view.lastSuccess)} · возраст: ${view.lastSuccessAgeMs === null ? '—' : `${(view.lastSuccessAgeMs / 1000).toFixed(1)} с`}`;
    el['mil-recording'].textContent = view.recording; el['mil-retry'].hidden = transport.status !== 'stopped';
    renderDTCs(document, el['mil-dtcs'], view.dtcs);
  }
  const feed = new Feed({ onEvent: event => { received = performance.now(); live.receive(event); renderCurrent(); }, onState: state => { transport = state; renderCurrent(); } });
  const history = new HistoryController({ onState: state => {
    if (catalog !== state.catalog) {
      catalog = state.catalog; el['mil-history-session'].replaceChildren();
      for (const session of catalog?.sessions ?? []) { const option = node(document, 'option', `${timeLabel(session.started_at)} · ${session.closed ? 'закрыт' : 'активная / неполная запись'}${session.aborted ? ' · прервана' : ''} · частей: ${session.parts}`); option.value = session.id; el['mil-history-session'].append(option); }
    }
    el['mil-history-session'].value = state.session;
    el['mil-history-session'].disabled = !catalog?.sessions.length;
    el['mil-archive-source'].textContent = `${state.source} · АРХИВ${catalog?.retention_truncated ? ' · RETENTION: часть истории удалена' : ''}`;
    el['mil-history-load'].disabled = !state.session || state.status === 'loading'; el['mil-history-refresh'].disabled = state.status === 'loading';
    el['mil-history-cancel'].hidden = state.status !== 'loading';
    el['mil-history-status'].textContent = ({ loading: 'Читаем сохранённые данные…', ready: 'Сохранённый отчёт. Новые данные доступны после ручного обновления.', empty: 'Сохранённых сеансов пока нет.', disabled: 'История MIL не настроена.', error: 'История не загружена.', canceled: 'Чтение отменено.' })[state.status] ?? '';
    el['mil-history-error'].textContent = state.error; el['mil-history-error'].hidden = !state.error;
    const link = el['mil-history-export']; link.hidden = !state.reportURL; if (state.reportURL) link.href = state.reportURL; else link.removeAttribute('href');
    el['mil-history-report'].setAttribute('aria-busy', String(state.status === 'loading'));
    if (rendered !== state.report) { rendered = state.report; if (rendered) renderSaved(document, el['mil-history-report'], rendered); else el['mil-history-report'].replaceChildren(); }
  } });
  el['mil-history-form'].addEventListener('submit', event => { event.preventDefault(); history.select(el['mil-history-session'].value); });
  el['mil-history-session'].addEventListener('change', () => history.select(el['mil-history-session'].value));
  el['mil-history-refresh'].addEventListener('click', () => history.refresh()); el['mil-history-cancel'].addEventListener('click', () => history.cancel());
  el['mil-retry'].addEventListener('click', () => feed.start());
  let watchdog = setInterval(renderCurrent, 250);
  window.addEventListener('pagehide', () => {
    feed.stop(); transport = { status: 'disconnected', attempt: 0 }; renderCurrent();
    history.cancel(); clearInterval(watchdog); watchdog = null;
  });
  window.addEventListener('pageshow', event => {
    if (!event.persisted || watchdog !== null) return;
    transport = { status: 'connecting', attempt: 0 }; renderCurrent();
    feed.start(); watchdog = setInterval(renderCurrent, 250);
  });
  renderCurrent(); feed.start(); history.refresh();
}
if (typeof document !== 'undefined') start();
