import { graphSegments } from './analytics-model.mjs';

const timeLabel = value => new Date(value).toLocaleTimeString('ru-RU');

export function drawHistoryChart(canvas, caption, points, markers, { from, to, cursor, labelTime = timeLabel } = {}) {
  const lines = graphSegments(points), all = lines.flat();
  const ratio = Math.min(devicePixelRatio || 1, 2), width = Math.max(250, canvas.clientWidth), height = 210;
  canvas.width = width * ratio; canvas.height = height * ratio;
  const ctx = canvas.getContext('2d'); ctx.scale(ratio, ratio); ctx.clearRect(0, 0, width, height);
  caption.textContent = all.length ? `${all.length} сохранённых точек · разрывы не соединяются` : 'Нет применимых измерений';
  if (!all.length) return;
  const x0 = Number.isFinite(from) ? from : Math.min(...all.map(p => p.at));
  const x1 = Number.isFinite(to) ? to : Math.max(...all.map(p => p.at));
  const y0 = Math.min(0, ...all.map(p => p.value)), y1 = Math.max(y0 + 1, ...all.map(p => p.value));
  const x = t => 48 + (t - x0) / Math.max(1, x1 - x0) * (width - 64), y = v => height - 30 - (v - y0) / (y1 - y0) * (height - 50);
  ctx.strokeStyle = '#34483e'; ctx.lineWidth = 1; ctx.beginPath(); ctx.moveTo(48, 12); ctx.lineTo(48, height - 30); ctx.lineTo(width - 16, height - 30); ctx.stroke();
  ctx.fillStyle = '#b9c8bb'; ctx.font = '11px system-ui'; ctx.fillText(y1.toFixed(1), 2, 20); ctx.fillText(y0.toFixed(0), 8, height - 29);
  ctx.fillText(labelTime(x0), 48, height - 8); ctx.fillText(labelTime(x1), Math.max(48, width - 78), height - 8);
  ctx.strokeStyle = '#97d687'; ctx.fillStyle = '#97d687'; ctx.lineWidth = 1.5;
  for (const line of lines) { ctx.beginPath(); line.forEach((p, i) => i ? ctx.lineTo(x(p.at), y(p.value)) : ctx.moveTo(x(p.at), y(p.value))); ctx.stroke(); if (line.length === 1) { ctx.beginPath(); ctx.arc(x(line[0].at), y(line[0].value), 2, 0, Math.PI * 2); ctx.fill(); } }
  function lineAt(t) {
    ctx.beginPath(); ctx.moveTo(x(t), 12); ctx.lineTo(x(t), height - 30); ctx.stroke();
  }
  ctx.setLineDash([3, 4]);
  for (const marker of markers) {
    const t = Date.parse(marker.at);
    if (t < x0 || t > x1 || !Number.isFinite(t)) continue;
    ctx.strokeStyle = marker.kind === 'mil_on' || marker.kind === 'mil_after_gap' || marker.kind === 'mil_already_on' ? '#ef8068' : '#d9aa56';
    lineAt(t);
  }
  ctx.setLineDash([]);
  if (Number.isFinite(cursor) && cursor >= x0 && cursor <= x1) {
    ctx.strokeStyle = '#e4f4dc';
    lineAt(cursor);
  }
}
