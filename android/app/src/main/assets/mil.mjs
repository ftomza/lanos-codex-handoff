import { dtcView, renderDtc } from './dtc.mjs';

export function milBadge(event, elapsedMs, transport) {
  if (event?.source !== 'live') return null;
  const mil = event.mil;
  const fresh = event.health === 'connected' && transport === 'open' && !event.standby_reason &&
    Number.isFinite(elapsedMs) && elapsedMs >= 0 && Number.isFinite(mil?.remaining_ms) &&
    mil.remaining_ms > elapsedMs && Number.isInteger(mil.dtc_count) && mil.dtc_count >= 0 && mil.dtc_count <= 127;
  if (fresh && mil.state === 'on') return { state: 'on', label: 'Check Engine · Горит', count: mil.dtc_count };
  if (fresh && mil.state === 'off') return { state: 'off', label: 'Check Engine · Не горит', count: mil.dtc_count };
  return { state: 'unknown', label: 'Check Engine · Нет данных', count: null };
}

export function renderMil(element, event, elapsedMs, transport) {
  const badge = milBadge(event, elapsedMs, transport);
  element.hidden = badge === null;
  if (badge === null) return null;
  const text = badge.count !== null ? badge.label + ' · DTC ' + badge.count : badge.label;
  if (element.textContent !== text) element.textContent = text;
  element.dataset.mil = badge.state;
  return badge;
}


export function milDetails(event, badge) {
  if (badge?.state === 'on') return dtcView(event?.mil?.dtc_replies, { live: true, lampOn: true });
  const summary = badge?.state === 'off' ? 'Коды читаются при включении Check Engine.'
    : 'Дождитесь свежего ответа ЭБУ о состоянии Check Engine.';
  return { summary, modes: [] };
}

export function bindMilDetails(button, dialog) {
  const output = dialog.querySelector('#mil-dtc-results');
  const heading = dialog.querySelector('#mil-details-title');
  let event, badge, rendered;

  function draw() {
    if (!dialog.open || !badge) return;
    if (heading.textContent !== badge.label) heading.textContent = badge.label;
    const view = milDetails(event, badge);
    const key = JSON.stringify(view);
    if (key === rendered) return;
    renderDtc(dialog.ownerDocument, output, view);
    rendered = key;
  }

  button.addEventListener('click', () => {
    if (!badge || dialog.open) return;
    dialog.showModal();
    draw();
  });
  dialog.querySelector('[data-close]').addEventListener('click', () => dialog.close());
  return (next, elapsedMs, transport) => {
    event = next;
    badge = renderMil(button, event, elapsedMs, transport);
    if (!badge && dialog.open) dialog.close();
    draw();
  };
}
