// Standard OBD-II descriptions. Manufacturer-specific definitions vary by ECU.
const descriptions = new Map([
  ['P0105', 'Цепь датчика абсолютного давления во впуске или барометрического давления'],
  ['P0106', 'Сигнал датчика давления во впуске или барометрического давления вне рабочего диапазона'],
  ['P0107', 'Низкий сигнал датчика давления во впуске или барометрического давления'],
  ['P0108', 'Высокий сигнал датчика давления во впуске или барометрического давления'],
  ['P0110', 'Цепь датчика температуры впускного воздуха'],
  ['P0112', 'Низкий сигнал датчика температуры впускного воздуха'],
  ['P0113', 'Высокий сигнал датчика температуры впускного воздуха'],
  ['P0115', 'Цепь датчика температуры охлаждающей жидкости'],
  ['P0117', 'Низкий сигнал датчика температуры охлаждающей жидкости'],
  ['P0118', 'Высокий сигнал датчика температуры охлаждающей жидкости'],
  ['P0120', 'Цепь датчика положения дросселя или педали, канал A'],
  ['P0130', 'Цепь датчика кислорода, банк 1, датчик 1'],
  ['P0131', 'Низкий сигнал датчика кислорода, банк 1, датчик 1'],
  ['P0132', 'Высокий сигнал датчика кислорода, банк 1, датчик 1'],
  ['P0133', 'Медленный отклик датчика кислорода, банк 1, датчик 1'],
  ['P0135', 'Цепь подогревателя датчика кислорода, банк 1, датчик 1'],
  ['P0136', 'Цепь датчика кислорода, банк 1, датчик 2'],
  ['P0141', 'Цепь подогревателя датчика кислорода, банк 1, датчик 2'],
  ['P0171', 'Слишком бедная смесь, банк 1'],
  ['P0172', 'Слишком богатая смесь, банк 1'],
  ['P0201', 'Цепь форсунки цилиндра 1'],
  ['P0202', 'Цепь форсунки цилиндра 2'],
  ['P0203', 'Цепь форсунки цилиндра 3'],
  ['P0204', 'Цепь форсунки цилиндра 4'],
  ['P0300', 'Случайные или множественные пропуски воспламенения'],
  ['P0301', 'Пропуски воспламенения в цилиндре 1'],
  ['P0302', 'Пропуски воспламенения в цилиндре 2'],
  ['P0303', 'Пропуски воспламенения в цилиндре 3'],
  ['P0304', 'Пропуски воспламенения в цилиндре 4'],
  ['P0325', 'Цепь датчика детонации 1, банк 1 или единственный датчик'],
  ['P0335', 'Цепь датчика положения коленвала A'],
  ['P0340', 'Цепь датчика положения распредвала A, банк 1 или единственный датчик'],
  ['P0401', 'Недостаточный поток рециркуляции отработавших газов'],
  ['P0402', 'Избыточный поток рециркуляции отработавших газов'],
  ['P0420', 'Эффективность катализатора ниже порога, банк 1'],
  ['P0443', 'Цепь клапана продувки адсорбера'],
  ['P0455', 'Крупная утечка в системе улавливания паров топлива'],
  ['P0500', 'Сигнал датчика скорости автомобиля'],
  ['P0505', 'Неисправность системы регулирования холостого хода'],
  ['P0562', 'Низкое напряжение бортовой сети'],
  ['P0563', 'Высокое напряжение бортовой сети'],
  ['U0100', 'Потеря связи с блоком управления двигателем или силовой установкой'],
]);

const modes = [['03', 'Сохранённые коды'], ['07', 'Ожидающие коды']];
const messages = new Map([
  ['waiting', 'Ожидаем ответ ЭБУ'],
  ['missing', 'Ответ не записан'],
  ['empty', 'ЭБУ вернул пустой список'],
  ['unsupported', 'ЭБУ не поддерживает этот запрос'],
  ['interrupted', 'Чтение прервано'],
  ['failed', 'Не удалось прочитать коды'],
]);
const codePattern = /^[PCBU][0-3][0-9A-F]{3}$/;

function replyView(mode, label, reply, live) {
  const capturedAt = typeof reply?.captured_at === 'string' && Number.isFinite(Date.parse(reply.captured_at))
    ? reply.captured_at : null;
  const source = { mode, label, capturedAt, codes: [], truncated: false };
  const state = value => ({ ...source, state: value, message: messages.get(value) ?? '' });
  if (reply === undefined) return state(live ? 'waiting' : 'missing');
  if (reply?.status === 'unsupported' || reply?.status === 'interrupted') return state(reply.status);
  if (reply?.status !== 'available' || !capturedAt || !Array.isArray(reply.codes) ||
    reply.codes.length > 64 || reply.codes.some(code => typeof code !== 'string' || !codePattern.test(code)) ||
    ('codes_truncated' in reply && typeof reply.codes_truncated !== 'boolean')) return state('failed');
  if (!reply.codes.length) return state(reply.codes_truncated ? 'failed' : 'empty');
  return {
    ...state('codes'),
    codes: [...new Set(reply.codes)].map(code => ({
      code, description: descriptions.get(code) ?? 'Нет описания в справочнике',
    })),
    truncated: reply.codes_truncated === true,
  };
}

export function dtcView(replies, { live = false, lampOn = false } = {}) {
  const details = modes.map(([mode, label]) => replyView(mode, label, replies?.[mode], live));
  const summary = lampOn && details.every(mode => mode.state === 'empty')
    ? (live ? 'Check Engine горит. ЭБУ не вернул кодов.' : 'При включённом Check Engine ЭБУ не вернул кодов.')
    : '';
  return { summary, modes: details };
}

const formatTime = new Intl.DateTimeFormat('ru-RU', { dateStyle: 'short', timeStyle: 'medium' });

export function renderDtc(document, container, view) {
  const fragment = document.createDocumentFragment();
  const appendText = (parent, tag, text) => {
    const element = document.createElement(tag);
    element.textContent = text;
    parent.append(element);
    return element;
  };
  if (view.summary) appendText(fragment, 'p', view.summary);
  if (view.modes.length) {
    const grid = document.createElement('div');
    grid.className = 'dtc-modes';
    for (const mode of view.modes) {
      const section = document.createElement('section');
      section.className = 'dtc-mode';
      appendText(section, 'h3', mode.label + ' · ' + mode.mode);
      if (mode.capturedAt) {
        const time = appendText(section, 'time', formatTime.format(new Date(mode.capturedAt)));
        time.dateTime = mode.capturedAt;
      }
      if (mode.message) appendText(section, 'p', mode.message);
      if (mode.codes.length) {
        const list = document.createElement('ul');
        for (const code of mode.codes) appendText(list, 'li', code.code + ' — ' + code.description);
        section.append(list);
      }
      if (mode.truncated) appendText(section, 'p', 'Список кодов сохранён частично.');
      grid.append(section);
    }
    fragment.append(grid);
  }
  container.replaceChildren(fragment);
}
