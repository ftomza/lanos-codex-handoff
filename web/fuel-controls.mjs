// All mutations change local trip state only. The browser supplies Origin itself.
export async function fuelRequest(path, body, fetcher = globalThis.fetch.bind(globalThis)) {
  const response = await fetcher(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
  return response.json();
}
export function bindFuelControls(elements, currentEvent, request = fuelRequest) {
  let revision = 0;
  let preview = null;
  let busy = false;
  let scope = 0;
  let inputVersion = 0;
  const calibration = elements['fuel-calibration-dialog'];
  const discardPreview = () => {
    inputVersion++;
    preview = null;
    elements['fuel-apply'].disabled = true;
    elements['fuel-preview-result'].textContent = '';
  };
  const clearCalibration = () => {
    elements['fuel-measured'].value = '';
    elements['fuel-full-to-full'].checked = false;
    discardPreview();
  };
  const error = message => { elements['fuel-error'].textContent = message; elements['fuel-error'].hidden = !message; for (const node of elements['fuel-panel'].ownerDocument.querySelectorAll('[data-fuel-error]')) { node.textContent = message; node.hidden = !message; } };
  const execute = async operation => {
    if (busy) return;
    const currentScope = scope;
    busy = true; error(''); elements['fuel-panel'].setAttribute('aria-busy', 'true');
    try { await operation(currentScope); }
    catch (e) { if (currentScope === scope) error(`Не удалось сохранить: ${e.message}`); }
    finally { busy = false; elements['fuel-panel'].removeAttribute('aria-busy'); }
  };
  elements['fuel-reset'].addEventListener('click', () => {
    revision = currentEvent()?.fuel?.revision;
    elements['fuel-reset-factor'].checked = false;
    elements['fuel-reset-dialog'].showModal();
  });
  elements['fuel-reset-confirm'].addEventListener('click', () => execute(async currentScope => {
    await request('/api/fuel/reset', { expected_revision: revision, default_factor: elements['fuel-reset-factor'].checked });
    if (currentScope === scope) elements['fuel-reset-dialog'].close();
  }));
  elements['fuel-calibrate'].addEventListener('click', () => {
    revision = currentEvent()?.fuel?.revision;
    clearCalibration();
    calibration.showModal();
  });
  for (const id of ['fuel-measured', 'fuel-full-to-full']) elements[id].addEventListener('input', discardPreview);
  calibration.addEventListener('close', () => { if (!calibration.open) discardPreview(); });
  elements['fuel-preview'].addEventListener('click', () => execute(async () => {
    discardPreview();
    const version = inputVersion;
    const liters = Number(elements['fuel-measured'].value);
    const confirmed = elements['fuel-full-to-full'].checked;
    const result = await request('/api/fuel/calibration/preview', { expected_revision: revision, measured_liters: liters, full_to_full: confirmed });
    if (!calibration.open || version !== inputVersion) return;
    preview = { liters, confirmed, token: result.preview_token };
    elements['fuel-preview-result'].textContent = `Поправка ${result.previous_factor.toFixed(3)} → ${result.factor.toFixed(3)}. Итог на момент проверки: ${result.adjusted_liters.toFixed(2)} л.`;
    elements['fuel-apply'].disabled = false;
  }));
  elements['fuel-apply'].addEventListener('click', () => execute(async currentScope => {
    if (!preview) return;
    await request('/api/fuel/calibration', { action: 'apply', expected_revision: revision, measured_liters: preview.liters, full_to_full: preview.confirmed, preview_token: preview.token });
    if (currentScope === scope) calibration.close();
  }));
  for (const [id, action] of [['fuel-undo', 'undo'], ['fuel-default', 'default']]) elements[id].addEventListener('click', () => execute(async currentScope => {
    await request('/api/fuel/calibration', { action, expected_revision: currentEvent()?.fuel?.revision });
    if (currentScope === scope) calibration.close();
  }));
  for (const id of ['fuel-reset-dialog', 'fuel-calibration-dialog']) {
    elements[id].querySelector('[data-close]').addEventListener('click', () => elements[id].close());
  }
  return () => {
    scope++;
    clearCalibration();
    elements['fuel-reset-dialog'].close();
    calibration.close();
    error('');
  };
}
