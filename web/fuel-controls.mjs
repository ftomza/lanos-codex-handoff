// All mutations change local trip state only. The browser supplies Origin itself.
export async function fuelRequest(path, body, fetcher = globalThis.fetch.bind(globalThis)) {
  const response = await fetcher(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
  return response.json();
}
export function bindFuelControls(elements, currentEvent) {
  let revision = 0;
  let preview = null;
  let busy = false;
  const error = message => { elements['fuel-error'].textContent = message; elements['fuel-error'].hidden = !message; for (const node of document.querySelectorAll('[data-fuel-error]')) { node.textContent = message; node.hidden = !message; } };
  const execute = async operation => {
    if (busy) return;
    busy = true; error(''); elements['fuel-panel'].setAttribute('aria-busy', 'true');
    try { await operation(); }
    catch (e) { error(`Не удалось сохранить: ${e.message}`); }
    finally { busy = false; elements['fuel-panel'].removeAttribute('aria-busy'); }
  };
  elements['fuel-reset'].addEventListener('click', () => {
    revision = currentEvent()?.fuel?.revision; elements['fuel-reset-dialog'].showModal();
  });
  elements['fuel-reset-confirm'].addEventListener('click', () => execute(async () => {
    await fuelRequest('/api/fuel/reset', { expected_revision: revision, default_factor: elements['fuel-reset-factor'].checked });
    elements['fuel-reset-dialog'].close();
  }));
  elements['fuel-calibrate'].addEventListener('click', () => {
    revision = currentEvent()?.fuel?.revision; preview = null;
    elements['fuel-apply'].disabled = true; elements['fuel-preview-result'].textContent = '';
    elements['fuel-calibration-dialog'].showModal();
  });
  for (const id of ['fuel-measured', 'fuel-full-to-full']) elements[id].addEventListener('input', () => {
    preview = null; elements['fuel-apply'].disabled = true;
  });
  elements['fuel-preview'].addEventListener('click', () => execute(async () => {
    const liters = Number(elements['fuel-measured'].value);
    const confirmed = elements['fuel-full-to-full'].checked;
    const result = await fuelRequest('/api/fuel/calibration/preview', { expected_revision: revision, measured_liters: liters, full_to_full: confirmed });
    preview = { liters, confirmed, token: result.preview_token };
    elements['fuel-preview-result'].textContent = `Поправка ${result.previous_factor.toFixed(3)} → ${result.factor.toFixed(3)}. Итог на момент проверки: ${result.adjusted_liters.toFixed(2)} л.`;
    elements['fuel-apply'].disabled = false;
  }));
  elements['fuel-apply'].addEventListener('click', () => execute(async () => {
    if (!preview) return;
    await fuelRequest('/api/fuel/calibration', { action: 'apply', expected_revision: revision, measured_liters: preview.liters, full_to_full: preview.confirmed, preview_token: preview.token });
    elements['fuel-calibration-dialog'].close();
  }));
  for (const [id, action] of [['fuel-undo', 'undo'], ['fuel-default', 'default']]) elements[id].addEventListener('click', () => execute(async () => {
    await fuelRequest('/api/fuel/calibration', { action, expected_revision: currentEvent()?.fuel?.revision });
  }));
  for (const id of ['fuel-reset-dialog', 'fuel-calibration-dialog']) {
    elements[id].querySelector('[data-close]').addEventListener('click', () => elements[id].close());
  }
}
