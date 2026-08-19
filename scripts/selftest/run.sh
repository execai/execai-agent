#!/usr/bin/env bash
# Полный самопрогон: одна команда, никакого участия человека.
#
# Зачем. Проверки жили в голове и в разовых скриптах: часть гонялась руками,
# часть забывалась, а баги («стоп крутится всегда», паника без логина) уезжали
# к владельцу. Здесь всё собрано в один прогон с честным отчётом: что прошло,
# что упало, где артефакты.
#
# Железные правила прогона:
#   - конфиг владельца не трогается: всё в своём XDG_CONFIG_HOME;
#   - редактор поднимается в изолированном профиле и гасится за собой;
#   - падение любого этапа НЕ прерывает остальные — отчёт нужен целиком;
#   - код возврата ненулевой, если упал хоть один этап.
#
# Запуск:  scripts/selftest/run.sh [--fast] [--only <этап>]
#   --fast  пропустить UI-этап (он самый долгий: поднимает редактор)
#   --only  прогнать один этап:
#           units | security | ide | loop | tui | serve | plugin | ui
set -uo pipefail

AGENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PLUGIN_DIR="$(cd "$AGENT_DIR/../vscode-vbai" 2>/dev/null && pwd || echo "")"
OUT="${SELFTEST_OUT:-$AGENT_DIR/.selftest}"
RUN_ID="$(date +%Y%m%d-%H%M%S)"
ART="$OUT/$RUN_ID"
REPORT="$ART/report.md"
mkdir -p "$ART"

FAST=0
ONLY=""
while [ $# -gt 0 ]; do
  case "$1" in
    --fast) FAST=1 ;;
    --only) ONLY="${2:-}"; shift ;;
    *) echo "неизвестный ключ: $1"; exit 2 ;;
  esac
  shift
done

# Изолированное окружение: свой конфиг, чтобы прогон не менял модель, источник
# и уровень безопасности у владельца. Ловушка, проверенная на практике: тесты
# писали в живой ~/.config/execai и молча меняли effort.
export XDG_CONFIG_HOME="$ART/conf"
mkdir -p "$XDG_CONFIG_HOME/execai"
if [ -f "$HOME/.config/execai/config.json" ]; then
  # Берём api_base из настоящего конфига, остальное — умолчания.
  python3 - "$HOME/.config/execai/config.json" "$XDG_CONFIG_HOME/execai/config.json" <<'PY'
import json, sys
src, dst = sys.argv[1], sys.argv[2]
try:
    api = json.load(open(src)).get("api_base", "https://api.execai.ru")
except Exception:
    api = "https://api.execai.ru"
json.dump({"api_base": api}, open(dst, "w"), indent=2)
PY
fi

PASS=0; FAIL=0
declare -a ROWS

stage() { # stage <имя> <описание> <команда…>
  local name="$1" desc="$2"; shift 2
  [ -n "$ONLY" ] && [ "$ONLY" != "$name" ] && return 0
  local log="$ART/$name.log"
  echo "── $name: $desc"
  local t0=$SECONDS
  if "$@" >"$log" 2>&1; then
    local dt=$((SECONDS - t0))
    PASS=$((PASS + 1)); ROWS+=("| ✅ | $name | $desc | ${dt}с | \`$name.log\` |")
    echo "   ok (${dt}с)"
  else
    local dt=$((SECONDS - t0))
    FAIL=$((FAIL + 1)); ROWS+=("| ❌ | $name | $desc | ${dt}с | \`$name.log\` |")
    echo "   УПАЛО (${dt}с) → $log"
    tail -15 "$log" | sed 's/^/   │ /'
  fi
}

# ── Этапы ────────────────────────────────────────────────────────────────

s_units() { cd "$AGENT_DIR" && go build ./... && go vet ./... && go test ./internal/... ; }

s_security() {
  cd "$AGENT_DIR"
  # Отдельным этапом: корпус обходов — самая дорогая ошибка, её результат
  # должен быть виден в отчёте одной строкой, а не тонуть среди юнитов.
  go test ./internal/tools/ ./internal/security/ -run 'Bypass|Useful|Subcommand|Levels|Perimeter|Scope|External|Sensitive|Inside' -v
}

s_ide() {
  cd "$AGENT_DIR"
  go build -o "$ART/execai" ./cmd/execai || return 1
  EXECAI_BIN="$ART/execai" WORKDIR="$ART/ws" python3 "$AGENT_DIR/scripts/selftest/ide_e2e.py"
}

s_loop() {
  cd "$AGENT_DIR"
  [ -x "$ART/execai" ] || go build -o "$ART/execai" ./cmd/execai || return 1
  # Свой конфиг у этапа: он подменяет провайдера и разрешения, и делать это
  # в общем окружении прогона значило бы портить соседние этапы.
  XDG_CONFIG_HOME="$ART/loop-conf" EXECAI_BIN="$ART/execai" WORKDIR="$ART/loop-ws" \
    python3 "$AGENT_DIR/scripts/selftest/loop_e2e.py"
}

s_tui() {
  cd "$AGENT_DIR"
  [ -x "$ART/execai" ] || go build -o "$ART/execai" ./cmd/execai || return 1
  XDG_CONFIG_HOME="$ART/tui-conf" EXECAI_BIN="$ART/execai" WORKDIR="$ART/tui-ws" \
    python3 "$AGENT_DIR/scripts/selftest/tui_e2e.py"
}

s_serve() {
  cd "$AGENT_DIR"
  [ -x "$ART/execai" ] || go build -o "$ART/execai" ./cmd/execai || return 1
  XDG_CONFIG_HOME="$ART/serve-conf" EXECAI_BIN="$ART/execai" WORKDIR="$ART/serve-ws" \
    python3 "$AGENT_DIR/scripts/selftest/serve_e2e.py"
}

s_plugin() {
  [ -z "$PLUGIN_DIR" ] && { echo "репозиторий плагина не найден рядом — пропуск"; return 0; }
  cd "$PLUGIN_DIR" && npm test
}

s_ui() {
  [ -z "$PLUGIN_DIR" ] && { echo "репозиторий плагина не найден — пропуск"; return 0; }
  [ "$FAST" = "1" ] && { echo "--fast: UI пропущен"; return 0; }
  ART="$ART" AGENT_BIN="$ART/execai" PLUGIN_DIR="$PLUGIN_DIR" \
    bash "$AGENT_DIR/scripts/selftest/ui_e2e.sh"
}

echo "Самопрогон $RUN_ID → $ART"
stage units    "сборка, go vet, все юнит-тесты"          s_units
stage security "корпус обходов и матрица уровней"        s_security
stage ide      "IDE-протокол сквозным пайпом"            s_ide
stage loop     "ход агента: инструмент, разрешение, отказ"  s_loop
stage tui      "терминал: диалоги и команды в pty"      s_tui
stage serve    "фоновый режим: задача из веба и вопрос"  s_serve
stage plugin   "тесты плагина (в т.ч. синтаксис вебвью)" s_plugin
stage ui       "живой редактор: панель, меню, ход"       s_ui

# ── Отчёт ────────────────────────────────────────────────────────────────
{
  echo "# Самопрогон $RUN_ID"
  echo
  echo "Итог: **$PASS прошло, $FAIL упало**."
  echo
  echo "| | этап | что проверяет | время | лог |"
  echo "|---|---|---|---|---|"
  printf '%s\n' "${ROWS[@]}"
  echo
  echo "Артефакты: \`$ART\`"
  echo
  echo "Окружение прогона изолировано (\`XDG_CONFIG_HOME=$XDG_CONFIG_HOME\`);"
  echo "конфиг владельца не изменялся."
} > "$REPORT"

echo
cat "$REPORT"
[ "$FAIL" -eq 0 ] || exit 1
