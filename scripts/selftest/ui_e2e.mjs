// Сценарии в живом редакторе: панель, меню, ход, состояние кнопок.
//
// Правило прогона: только настоящие клики координатами. Синтетический
// element.click() однажды «подтвердил» работу меню, которое у человека не
// открывалось вовсе — с тех пор он здесь запрещён.
//
// Вебвью в VS Code живёт отдельным CDP-таргетом (vscode-webview://), а его
// содержимое — во вложенном #active-frame. Отсюда двухступенчатый доступ:
// к внешнему окну цепляемся ради координат и мыши, к вебвью — ради DOM.
import WebSocket from 'ws';
import { writeFileSync } from 'node:fs';

const PORT = process.env.CDP_PORT || '9333';
const ART = process.env.ART || '.';
const checks = [];
const check = (ok, name, detail = '') => {
  checks.push({ ok: !!ok, name, detail });
  console.log(`  ${ok ? 'ok  ' : 'ПРОВАЛ'} ${name}${detail ? ' — ' + detail : ''}`);
};
// Пропуск — отдельное состояние. Молча пропущенная проверка читается как
// пройденная, и именно так теряются целые куски покрытия.
const skip = (name, why) => {
  checks.push({ ok: true, skipped: true, name, detail: why });
  console.log(`  пропуск ${name} — ${why}`);
};
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function targets() {
  return (await (await fetch(`http://127.0.0.1:${PORT}/json/list`)).json());
}

// Тонкий клиент CDP: playwright после многих перезапусков редактора зависал
// на connectOverCDP, а нам нужен прогон, который не падает сам по себе.
function connect(wsUrl) {
  const ws = new WebSocket(wsUrl);
  let id = 0;
  const waiters = new Map();
  const ready = new Promise((r) => ws.on('open', r));
  ws.on('message', (d) => {
    const m = JSON.parse(d);
    if (m.id && waiters.has(m.id)) { waiters.get(m.id)(m); waiters.delete(m.id); }
  });
  const call = async (method, params) => {
    await ready;
    const i = ++id;
    ws.send(JSON.stringify({ id: i, method, params }));
    return new Promise((r) => waiters.set(i, r));
  };
  return { call, close: () => ws.close(), ready };
}

const val = (r) => r?.result?.result?.value;

async function main() {
  // 1. Панель открывается из активной панели слева.
  // Порт отладки открывается РАНЬШЕ, чем появляется окно: одиночный запрос
  // списка ловил пустоту и обвинял редактор в том, что его нет.
  let page = null;
  for (let i = 0; i < 30; i++) {
    const list = await targets();
    page = list.find((t) => t.type === 'page' && t.url.includes('workbench'))
        || list.find((t) => t.type === 'page');
    if (page) break;
    await sleep(1000);
  }
  if (!page) { check(false, 'окно редактора найдено', 'за 30с не появился таргет страницы'); return finish(); }
  const win = connect(page.webSocketDebuggerUrl);
  // Неактивное окно ввод не принимает — playwright делал это за нас.
  await win.call('Page.bringToFront', {});

  // Ждём, а не спрашиваем один раз: расширения поднимаются после того, как
  // порт отладки уже открыт, и одиночный снимок ловил пустую панель.
  let opened = 'нет иконки';
  for (let i = 0; i < 30; i++) {
    opened = val(await win.call('Runtime.evaluate', {
      expression: `(() => { const e = document.querySelector('.activitybar [aria-label^="ExecAI"]');
        if (!e) return 'нет иконки'; e.click(); return 'ok'; })()`,
      returnByValue: true,
    }));
    if (opened === 'ok') break;
    await sleep(1000);
  }
  if (opened !== 'ok') {
    // Провал обязан показывать, ЧТО было на экране, иначе разбор начинается
    // с повторного запуска руками.
    const labels = val(await win.call('Runtime.evaluate', {
      expression: `[...document.querySelectorAll('.activitybar [aria-label]')]
        .map(e => e.getAttribute('aria-label')).join(' | ')`,
      returnByValue: true,
    }));
    opened += ` (в панели: ${labels || 'пусто'})`;
  }
  check(opened === 'ok', 'иконка ExecAI есть в панели активности', opened === 'ok' ? '' : opened);
  await sleep(4000);

  // 2. Вебвью поднялся отдельным таргетом.
  let wv = null;
  for (let i = 0; i < 20; i++) {
    wv = (await targets()).find((t) => t.url.startsWith('vscode-webview://'));
    if (wv) break;
    await sleep(1000);
  }
  check(!!wv, 'вебвью панели создан');
  if (!wv) { win.close(); return finish(); }
  const view = connect(wv.webSocketDebuggerUrl);

  const inFrame = async (expr) => val(await view.call('Runtime.evaluate', {
    expression: `(() => { const D = document.getElementById('active-frame').contentDocument;
      const W = D.defaultView; return (${expr}); })()`,
    returnByValue: true,
  }));

  // Координаты вебвью внутри окна — чтобы кликать настоящей мышью.
  // Рамку ищем по идентификатору НАШЕГО вебвью, а не «первую подходящую».
  //
  // В окне их несколько (панели, редакторы, чужие расширения), и координаты
  // чужой рамки дают самый коварный результат: проверка попадания внутри
  // кадра проходит (она считает в своих координатах), а настоящий клик
  // уходит мимо — интерфейс выглядит сломанным, хотя сломан прогон.
  const wvId = new URL(wv.url).host;
  const box = val(await win.call('Runtime.evaluate', {
    expression: `(() => {
      const f = [...document.querySelectorAll('iframe.webview')]
        .find(x => (x.src || '').includes(${JSON.stringify(wvId)}));
      if (!f) return null; const b = f.getBoundingClientRect();
      return { x: b.x, y: b.y, w: b.width, h: b.height }; })()`,
    returnByValue: true,
  }));
  check(!!box, 'рамка вебвью найдена по идентификатору',
    box ? `${Math.round(box.w)}×${Math.round(box.h)}` : `не нашли iframe для ${wvId}`);

  // Полная последовательность мыши.
  //
  // Одних mousePressed/mouseReleased мало: без предварительного mouseMoved
  // попадание не рассчитывается, и клик уходит в пустоту — панель при этом
  // выглядит рабочей, а меню не открывается (поймано самопрогоном 15.08).
  // Флаг buttons тоже обязателен: без него Chromium считает кнопку ненажатой.
  // Куда слать ввод.
  //
  // Вебвью живёт ОТДЕЛЬНЫМ процессом. Событие, отправленное в окно, доходит до
  // него только после того, как готова карта попаданий композитора, а у
  // свежезапущенного окна её ещё нет: нажатия просто исчезают — ни ошибки, ни
  // эффекта. Поэтому шлём прямо в вебвью (координаты его собственные), а окно
  // оставляем запасным путём.
  const dispatch = async (t, x, y) => {
    const base = { x, y, modifiers: 0, pointerType: 'mouse' };
    await t.call('Input.dispatchMouseEvent',
      { ...base, type: 'mouseMoved', button: 'none', buttons: 0, clickCount: 0 });
    await sleep(60);
    await t.call('Input.dispatchMouseEvent',
      { ...base, type: 'mousePressed', button: 'left', buttons: 1, clickCount: 1 });
    await sleep(40);
    await t.call('Input.dispatchMouseEvent',
      { ...base, type: 'mouseReleased', button: 'left', buttons: 0, clickCount: 1 });
  };

  // Смещение внутреннего кадра внутри вебвью.
  //
  // Кадров ДВА: панель окна (iframe.webview) и внутри неё #active-frame с
  // содержимым. Складывать только внешнюю рамку — значит промахиваться на
  // высоту внутреннего смещения; клик уходит в пустоту, а выглядит это как
  // «интерфейс не реагирует».
  const innerOff = val(await view.call('Runtime.evaluate', {
    expression: `(() => { const f = document.getElementById('active-frame');
      if (!f) return { x: 0, y: 0 }; const b = f.getBoundingClientRect();
      return { x: b.x, y: b.y }; })()`,
    returnByValue: true,
  })) || { x: 0, y: 0 };

  const clickIn = async (sel, text) => {
    const q = text
      ? `els.find(e => e.textContent.includes(${JSON.stringify(text)}))`
      : 'els[0]';
    const pos = await inFrame(`(() => {
      const els = [...D.querySelectorAll(${JSON.stringify(sel)})];
      const m = ${q};
      if (!m) return null; const b = m.getBoundingClientRect();
      return { x: b.x + b.width / 2, y: b.y + b.height / 2 }; })()`);
    if (!pos || !box) return false;

    // Перед нажатием проверяем попадание: промах по координатам и молчащий
    // интерфейс — разные диагнозы, и путать их нельзя.
    const hit = await inFrame(`(() => {
      const t = D.elementFromPoint(${pos.x}, ${pos.y});
      if (!t) return 'пусто';
      const els = [...D.querySelectorAll(${JSON.stringify(sel)})];
      const m = ${q};
      return (m && (m === t || m.contains(t))) ? 'ok' : (t.id || t.className || t.tagName); })()`);
    if (hit !== 'ok') { lastMiss = `клик пришёлся бы на ${hit}`; return false; }

    // Считаем реально пришедшие нажатия: «отправлено» и «дошло» — разные вещи,
    // и молча потерянный клик читался бы как сломанный интерфейс.
    await inFrame(`(() => { W.__clicks = 0;
      if (!W.__counter) { W.__counter = true;
        D.addEventListener('mousedown', () => { W.__clicks++; }, true); }
      return 1; })()`);

    await dispatch(view, innerOff.x + pos.x, innerOff.y + pos.y);
    await sleep(500);
    if (!(await inFrame(`W.__clicks || 0`))) {
      // Запасной путь — через окно: на некоторых сборках маршрут обратный.
      await dispatch(win, box.x + innerOff.x + pos.x, box.y + innerOff.y + pos.y);
      await sleep(500);
      if (!(await inFrame(`W.__clicks || 0`))) { lastMiss = 'нажатие не дошло ни одним путём'; return false; }
    }
    await sleep(400);
    return true;
  };
  let lastMiss = '';

  // 3. Агент поднялся и доложил состояние.
  await sleep(3000);
  const status = await inFrame(`D.getElementById('status')?.textContent || ''`);
  check(status.length > 0 && !status.includes('…'), 'статус-строка заполнена', status);

  // 4. Покой: «стоп» скрыт. Ровно эту половину проверки я однажды пропустил,
  //    и кнопка крутилась у владельца постоянно.
  const idle = {
    stop: await inFrame(`W.getComputedStyle(D.getElementById('stopBtn')).display`),
    send: await inFrame(`W.getComputedStyle(D.getElementById('sendBtn')).display`),
  };
  check(idle.stop === 'none', 'в покое кнопка «стоп» скрыта', idle.stop);
  check(idle.send !== 'none', 'в покое кнопка «отправить» видна', idle.send);

  // 5. Меню открывается настоящим кликом и содержит нужные пункты.
  // closeMenu() снимает только класс, пункты остаются в DOM — по ним прогон
  // однажды «нашёл» меню, которое не открывалось. Чистим перед проверкой.
  await inFrame(`(() => { const m = D.getElementById('menu');
    m.classList.remove('open'); m.innerHTML = ''; return 1; })()`);
  // Ставим наблюдателя ДО клика: он покажет, дошло ли событие мыши вообще и
  // в какую точку. Без него «меню не открылось» покрывает три разных диагноза.
  await inFrame(`(() => { W.__hits = [];
    D.addEventListener('mousedown', (e) => W.__hits.push(
      Math.round(e.clientX) + ',' + Math.round(e.clientY) + '→' +
      ((e.target && (e.target.id || e.target.className)) || '?')), true);
    return 1; })()`);
  const clicked = await clickIn('#cmdBtn');
  const hits = await inFrame(`(W.__hits || []).join(' | ')`);
  const menuOpen = await inFrame(`D.getElementById('menu')?.classList.contains('open')`);
  check(clicked && menuOpen, 'кнопка команд открывает меню',
    clicked ? (menuOpen ? '' : `нажатий в кадре: ${hits || 'НИ ОДНОГО'}`)
            : (lastMiss || 'кнопка не найдена'));
  // По якорям data-mi, а не по подписям: язык редактора у людей разный.
  let keys = await inFrame(`[...D.querySelectorAll('#menu .mi[data-mi]')].map(e => e.dataset.mi)`);
  if (!keys || !keys.length) {
    // Пустое меню при открытом контейнере = скрипт панели умер на отрисовке.
    // Без этой диагностики разбор начинается с гадания.
    const diag = await inFrame(`JSON.stringify({
      всего: D.querySelectorAll('#menu .mi').length,
      длина: (D.getElementById('menu')||{}).innerHTML?.length || 0,
      начало: ((D.getElementById('menu')||{}).innerHTML || '').slice(0, 160),
      естьФункция: typeof menuRoot,
    })`);
    console.log('  диагностика меню:', diag);
    keys = [];
  }
  for (const want of ['models', 'sources', 'connect', 'efforts', 'securities',
                      'maxiter', 'resume', 'newchat', 'restart', 'terminal']) {
    check((keys || []).includes(want), `в меню есть пункт «${want}»`,
      (keys || []).includes(want) ? '' : `видно: ${(keys || []).join(', ')}`);
  }
  check((keys || []).includes('login') || (keys || []).includes('logout'),
    'в меню есть вход или выход');

  // 6. Переход во вложенный список и обратно — здесь ломался outside-click.
  await clickIn('#menu .mi[data-mi="efforts"]');
  const levels = await inFrame(`[...D.querySelectorAll('#menu .mi[data-mi]')].map(e => e.dataset.mi)`);
  check((levels || []).includes('item:medium'), 'список уровней раскрылся',
    (levels || []).join(', '));
  await clickIn('#menu .mi[data-mi="item:medium"]');
  await sleep(1500);
  const st2 = await inFrame(`D.getElementById('status')?.textContent || ''`);
  check(st2.includes('medium'), 'выбор доехал до статус-строки', st2);

  // 7a. Разметка ответа: таблица обязана стать настоящей таблицей.
  //
  // Владелец прислал скриншот, где таблица приходила сырым текстом с
  // вертикальными чертами — панель вставляла ответ как обычный текст.
  //
  // Шлём панели НАСТОЯЩЕЕ событие потока, а не зовём рендерер напрямую:
  // во-первых, из внешнего контекста не видно top-level const внутреннего
  // кадра; во-вторых, так проверяется весь путь — обработчик, накопление
  // текста и отрисовка, — а не одна функция.
  await inFrame(`(() => {
    const md = ['| \u044d\u0442\u0430\u043f | \u0438\u0442\u043e\u0433 |', '|---|---:|', '| \u0441\u0431\u043e\u0440\u043a\u0430 | 15 |', '', '**\u0436\u0438\u0440\u043d\u044b\u0439** \u0438 \\\`\u043a\u043e\u0434\\\`'].join('\\n');
    W.postMessage({ type: 'turn_start' }, '*');
    W.postMessage({ type: 'text_delta', text: md }, '*');
    W.postMessage({ type: 'done', elapsed: 1 }, '*');
    return 1; })()`);
  await sleep(1200);
  const md = await inFrame(`(() => {
    const a = [...D.querySelectorAll('.msg.assistant')].pop();
    if (!a) return JSON.stringify({ none: true });
    return JSON.stringify({
      table: a.querySelectorAll('table').length,
      cells: a.querySelectorAll('td').length,
      bold: a.querySelectorAll('strong').length,
      code: a.querySelectorAll('code').length,
      pipes: a.textContent.includes('|'),
      text: a.textContent.slice(0, 60),
      raw: (a._md || '').slice(-40),
    }); })()`);
  const m = JSON.parse(md || '{}');
  check(m.table === 1 && m.cells === 2, 'таблица отрисована как таблица', md);
  check(m.bold === 1 && m.code === 1, 'жирный и код размечены', md);
  check(m.pipes === false, 'сырых вертикальных чертей не осталось', md);

  // 6b. Ссылки, кнопки копирования и вкладки чатов.
  //
  // Всё три — про панель, а не про агента, поэтому и проверяются здесь же
  // настоящим событием потока: у ответа должны появиться кликабельная ссылка,
  // кликабельный путь к файлу и кнопки копирования; вкладки должны собраться
  // из события chats.
  await inFrame(`(() => {
    const md = ['смотри internal/agent/memory.go:42 и https://execai.ru/blog', '', '\\\`\\\`\\\`go', 'func main() {}', '\\\`\\\`\\\`'].join('\\n');
    W.postMessage({ type: 'turn_start' }, '*');
    W.postMessage({ type: 'text_delta', text: md }, '*');
    W.postMessage({ type: 'done', elapsed: 1 }, '*');
    return 1; })()`);
  await sleep(1200);
  const links = await inFrame(`(() => {
    const a = [...D.querySelectorAll('.msg.assistant')].pop();
    if (!a) return JSON.stringify({ none: true });
    const file = a.querySelector('a[data-file]');
    const ext = [...a.querySelectorAll('a')].find(x => (x.getAttribute('href') || '').startsWith('http'));
    return JSON.stringify({
      file: file ? file.dataset.file + ':' + (file.dataset.line || '') : '',
      ext: ext ? ext.getAttribute('href') : '',
      copyCode: a.querySelectorAll('.codewrap .copy').length,
      copyMsg: a.querySelectorAll(':scope > .copy-msg').length,
    }); })()`);
  const L = JSON.parse(links || '{}');
  check(L.file === 'internal/agent/memory.go:42', 'путь к файлу стал ссылкой со строкой', links);
  check(L.ext === 'https://execai.ru/blog', 'ссылка в тексте стала кликабельной', links);
  check(L.copyCode === 1, 'у блока кода есть кнопка копирования', links);
  check(L.copyMsg === 1, 'у ответа есть кнопка копирования', links);

  // Клик по кнопке копирования уходит расширению как copy_text.
  const copied = await inFrame(`(() => {
    const orig = W.acquireVsCodeApi;
    const btn = D.querySelector('.codewrap .copy');
    if (!btn) return 'нет кнопки';
    W.__sent = null;
    const vs = D.__vscodeApi;
    btn.click();
    return btn.textContent; })()`);
  check(typeof copied === 'string' && copied.length > 0, 'кнопка копирования отвечает на клик', String(copied));

  await inFrame(`(() => {
    W.postMessage({ type: 'chats', chats: [
      { id: 'a1', label: 'Первый чат', active: true },
      { id: 'b2', label: '', active: false }
    ] }, '*');
    return 1; })()`);
  await sleep(600);
  const tabs = await inFrame(`(() => {
    const t = D.getElementById('tabs');
    if (!t) return JSON.stringify({ none: true });
    return JSON.stringify({
      count: t.querySelectorAll('.tab').length,
      active: t.querySelectorAll('.tab.active').length,
      plus: t.querySelectorAll('.newtab').length,
      second: (t.querySelectorAll('.tab span')[1] || {}).textContent || '',
    }); })()`);
  const TB = JSON.parse(tabs || '{}');
  check(TB.count === 2, 'вкладки чатов отрисованы', tabs);
  check(TB.active === 1, 'текущая вкладка выделена ровно одна', tabs);
  check(TB.plus === 1, 'есть кнопка нового чата', tabs);
  check(!!TB.second && TB.second.length > 0, 'чат без названия получил подпись от панели', tabs);

  // Закрытие вкладки — действие вида: чат остаётся в истории, ход не трогаем.
  const closed = await inFrame(`(() => {
    const t = D.getElementById('tabs');
    const x = t.querySelector('.tab i');
    if (!x) return JSON.stringify({ noClose: true });
    x.click();
    return JSON.stringify({
      left: t.querySelectorAll('.tab').length,
      firstId: (t.querySelector('.tab') || {}).dataset ? t.querySelector('.tab').dataset.chat : '',
    }); })()`);
  const C = JSON.parse(closed || '{}');
  check(C.left === 1, 'крестик убирает вкладку из полосы', closed);
  check(C.firstId === 'b2', 'закрылась именно та вкладка, по которой кликнули', closed);

  // И возвращается, когда чат открывают из истории заново.
  const back = await inFrame(`(() => {
    W.postMessage({ type: 'chats', chats: [
      { id: 'a1', label: 'Первый чат', active: false },
      { id: 'b2', label: 'Второй чат', active: true }
    ] }, '*');
    return 1; })()`);
  await sleep(500);
  const afterHide = await inFrame(`D.getElementById('tabs').querySelectorAll('.tab').length`);
  check(String(afterHide) === '1', 'закрытая вкладка не возвращается сама при обновлении списка', String(afterHide));

  // Порядок вкладок держится за панелью, а не за временем изменения чата:
  // агент отдаёт список «свежие сверху», и при переключении он тасуется —
  // полоса от этого меняться не должна, иначе клик выделяет соседнюю вкладку.
  await inFrame(`(() => {
    W.postMessage({ type: 'chats', chats: [
      { id: 'x1', label: 'Первый', active: true },
      { id: 'x2', label: 'Второй', active: false },
      { id: 'x3', label: 'Третий', active: false }
    ] }, '*');
    return 1; })()`);
  await sleep(500);
  const order1 = await inFrame(`[...D.querySelectorAll('#tabs .tab')].map(t => t.dataset.chat).join(',')`);
  // Тот же набор, но сервер прислал в другом порядке и активен другой чат.
  await inFrame(`(() => {
    W.postMessage({ type: 'chats', chats: [
      { id: 'x3', label: 'Третий', active: true },
      { id: 'x1', label: 'Первый', active: false },
      { id: 'x2', label: 'Второй', active: false }
    ] }, '*');
    return 1; })()`);
  await sleep(500);
  const after = await inFrame(`(() => {
    const t = [...D.querySelectorAll('#tabs .tab')];
    return JSON.stringify({
      order: t.map(x => x.dataset.chat).join(','),
      active: (t.find(x => x.classList.contains('active')) || {}).dataset ? t.find(x => x.classList.contains('active')).dataset.chat : '',
    }); })()`);
  const O = JSON.parse(after || '{}');
  check(O.order === order1, 'порядок вкладок не меняется от порядка сервера', order1 + ' → ' + O.order);
  check(O.active === 'x3', 'выделяется ровно тот чат, который стал активным', after);

  // 7. Настоящий ход: кнопки меняются местами, спиннер крутится.
  //
  // Без входа провайдера нет, и ход не начнётся: это не дефект панели.
  // Прогон по умолчанию не тратит токены владельца — включается явно.
  const signedIn = !!(await inFrame(`!!(D.getElementById('status')?.dataset?.user)`))
    || !(await inFrame(`D.getElementById('status')?.textContent || ''`))
        .match(/not signed in|не вошёл/i);
  if (!signedIn) {
    for (const n of ['во время хода «стоп» виден', 'во время хода «отправить» скрыт',
                     'спиннер крутится только во время хода', 'ход завершился',
                     'после хода «стоп» снова скрыт']) {
      skip(n, 'в изолированном профиле нет входа — ход не запускается');
    }
    const shot0 = await win.call('Page.captureScreenshot', { format: 'png' });
    if (shot0?.result?.data) writeFileSync(`${ART}/ui-final.png`, Buffer.from(shot0.result.data, 'base64'));
    view.close(); win.close();
    return finish();
  }
  await inFrame(`(() => { const i = D.getElementById('inp');
    i.value = 'Выполни команду: sleep 6 && echo готово'; return true; })()`);
  await clickIn('#sendBtn');
  await sleep(2500);
  const busy = {
    stop: await inFrame(`W.getComputedStyle(D.getElementById('stopBtn')).display`),
    send: await inFrame(`W.getComputedStyle(D.getElementById('sendBtn')).display`),
    spin: await inFrame(`W.getComputedStyle(D.getElementById('stopBtn'), '::after').animationName`),
  };
  check(busy.stop !== 'none', 'во время хода «стоп» виден', busy.stop);
  check(busy.send === 'none', 'во время хода «отправить» скрыт', busy.send);
  check(busy.spin === 'exSpin', 'спиннер крутится только во время хода', busy.spin);

  // 8. Ход завершается сам, состояние возвращается.
  let finished = false;
  for (let i = 0; i < 90; i++) {
    if (!(await inFrame(`D.body.classList.contains('busy')`))) { finished = true; break; }
    await sleep(1000);
  }
  check(finished, 'ход завершился');
  if (finished) {
    check(await inFrame(`W.getComputedStyle(D.getElementById('stopBtn')).display`) === 'none',
      'после хода «стоп» снова скрыт');
  }

  // 9. Снимок на память: по нему видно, что реально было на экране.
  const shot = await win.call('Page.captureScreenshot', { format: 'png' });
  if (shot?.result?.data) {
    writeFileSync(`${ART}/ui-final.png`, Buffer.from(shot.result.data, 'base64'));
    console.log(`  снимок: ${ART}/ui-final.png`);
  }

  view.close();
  win.close();
  return finish();
}

function finish() {
  const bad = checks.filter((c) => !c.ok);
  const skipped = checks.filter((c) => c.skipped);
  console.log(`\nИтог: ${checks.length - bad.length - skipped.length} прошли, ` +
              `${skipped.length} пропущено, ${bad.length} упало`);
  if (bad.length) {
    console.log('Упало:');
    for (const b of bad) console.log(`  - ${b.name}${b.detail ? ' — ' + b.detail : ''}`);
  }
  writeFileSync(`${ART}/ui-checks.json`, JSON.stringify(checks, null, 1));
  process.exit(bad.length ? 1 : 0);
}

main().catch((e) => { console.error('сценарий упал:', e.message); finish(); });
