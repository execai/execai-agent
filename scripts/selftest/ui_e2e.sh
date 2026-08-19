#!/usr/bin/env bash
# UI-этап: настоящий редактор, настоящая мышь, без человека.
#
# Почему именно так. Синтетический .click() ЛЖЁТ — он срабатывает на уже
# закрытом меню и на элементах, до которых человек не дотянется. Поэтому здесь
# кликается координатами через CDP, как пальцем.
#
# Грабли, из-за которых этап когда-то не запускался:
#   - `--remote-debugging-port` игнорируется, если процесс с тем же
#     --user-data-dir ещё жив: новое окно открывается в СТАРОМ процессе.
#     Значит сначала гасим главный процесс профиля;
#   - pkill по своему же шаблону убивает сам скрипт → шаблон в скобках;
#   - .vsix одной и той же версии не переустанавливается — версия бампается
#     самим прогоном во временной копии, а не в репозитории.
set -uo pipefail

ART="${ART:?}"
PLUGIN_DIR="${PLUGIN_DIR:?}"
AGENT_BIN="${AGENT_BIN:?}"
PORT="${SELFTEST_CDP_PORT:-9333}"
PROFILE="$ART/vsc-profile"
EXTDIR="$ART/vsc-ext"
WS="$ART/ui-ws"
mkdir -p "$PROFILE" "$EXTDIR" "$WS"

command -v code >/dev/null || { echo "VS Code не установлен — UI-этап пропущен"; exit 0; }
[ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ] || { echo "нет графической сессии — UI-этап пропущен"; exit 0; }

echo "Готовлю рабочее пространство"
# Этап обязан запускаться в одиночку (--only ui): бинарь собираем сами, если
# предыдущие этапы не гонялись.
if [ ! -x "$AGENT_BIN" ]; then
  ( cd "$(dirname "${BASH_SOURCE[0]}")/../.." && go build -o "$AGENT_BIN" ./cmd/execai ) \
    || { echo "не собрался агент"; exit 1; }
fi
cp "$AGENT_BIN" "$WS/.execai-dev"
cat > "$WS/README.md" <<'EOF'
Пространство самопрогона. Файл нужен, чтобы в панели было что показать.
EOF
cat > "$PROFILE/settings.json" <<EOF
{ "execai.binaryPath": "$WS/.execai-dev", "execai.autoInstall": false }
EOF
mkdir -p "$PROFILE/User"
cp "$PROFILE/settings.json" "$PROFILE/User/settings.json"

echo "Собираю плагин"
# Собираем В САМОМ репозитории, а не в копии.
#
# Копия выглядела аккуратнее (репозиторий не трогаем), но vsce паковал
# лежавший в ней СТАРЫЙ dist/: пересборка не запускалась, и прогон проверял
# вчерашний плагин, показывая зелёное. Здесь пересборка гарантирована тем же
# путём, каким собирается релиз.
# Порядок важен: у плагина нет vscode:prepublish, компиляцию делает
# `npm run build`. Позвать один vsce — значит упаковать прошлую сборку.
( cd "$PLUGIN_DIR" && npm run build && npx --yes @vscode/vsce package --no-dependencies -o "$ART/selftest.vsix" ) \
  >"$ART/plugin-build.log" 2>&1 \
  || { echo "не собрался .vsix — см. plugin-build.log"; tail -20 "$ART/plugin-build.log"; exit 1; }

# И проверяем, что в сборку действительно попал текущий исходник: молчаливая
# сборка вчерашнего кода — худший вид зелёного прогона.
if command -v unzip >/dev/null && grep -q "data-mi" "$PLUGIN_DIR/src/webviewHtml.ts"; then
  unzip -p "$ART/selftest.vsix" extension/dist/extension.js | grep -q "data-mi" \
    || { echo "в .vsix попал устаревший код (нет тест-якорей) — сборка не пересобралась"; exit 1; }
fi

code --user-data-dir "$PROFILE" --extensions-dir "$EXTDIR" \
     --install-extension "$ART/selftest.vsix" --force >/dev/null 2>&1 \
  || { echo "не установился плагин"; exit 1; }

# Гасим редакторы ПРЕДЫДУЩИХ прогонов, а не только своего.
#
# Порт отладки один на всех, и живой редактор от вчерашнего прогона забирает
# его себе: новый процесс поднимается, а CDP разговаривает со СТАРОЙ сборкой.
# Так прогон полдня проверял вчерашний плагин и честно показывал провалы,
# которых в текущем коде уже не было.
kill_selftest_editors() {
  local pids
  pids="$(ps -eo pid,args | grep "[c]ode --user-data-dir .*selftest\|[c]ode --user-data-dir $PROFILE" \
          | grep -v "type=" | awk '{print $1}')"
  for pid in $pids; do kill -TERM "$pid" 2>/dev/null; done
  [ -n "$pids" ] && sleep 4
  return 0
}
kill_selftest_editors
trap kill_selftest_editors EXIT

# И убеждаемся, что порт свободен: занят — значит чужой редактор жив, и всё
# дальнейшее было бы проверкой не того, что мы собрали.
if curl -sS -o /dev/null "http://127.0.0.1:$PORT/json/version" 2>/dev/null; then
  echo "порт $PORT занят чужим редактором — прогон остановлен, чтобы не проверять чужую сборку"
  exit 1
fi

echo "Поднимаю редактор"
nohup code --user-data-dir "$PROFILE" --extensions-dir "$EXTDIR" \
  --remote-debugging-port="$PORT" --disable-workspace-trust --disable-gpu \
  --new-window "$WS" >"$ART/editor.log" 2>&1 < /dev/null &

for i in $(seq 1 40); do
  curl -sS -o /dev/null "http://127.0.0.1:$PORT/json/version" && break
  sleep 1
done
curl -sS -o /dev/null "http://127.0.0.1:$PORT/json/version" || { echo "редактор не отдал порт отладки за 40с"; exit 1; }

# Зависимости прогона ставятся сами: «сначала сделай npm i» — это участие
# человека, которого здесь быть не должно.
SELFTEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ ! -d "$SELFTEST_DIR/node_modules/ws" ]; then
  echo "Ставлю зависимости прогона"
  ( cd "$SELFTEST_DIR" && npm install --silent --no-audit --no-fund ) || {
    echo "не поставились зависимости для CDP"; exit 1; }
fi

echo "Гоняю сценарии"
CDP_PORT="$PORT" ART="$ART" node "$(dirname "${BASH_SOURCE[0]}")/ui_e2e.mjs"
