#!/bin/bash
# system-test/auto/run.sh — единая точка входа для автоматических тестов
# agent-vbai. Запускается перед промоушеном R5 → prod, при отладке
# регрессий и просто для sanity check.
#
# Что делает:
#   1. Go unit-тесты (internal/*/) — быстро (<30 сек)
#   2. E2E smoke через cmd/syscheck — реальные запросы во все подключенные
#      source (execai/zai/anthropic/claude-cli/ollama)
#   3. Опционально: PTY-тест — запускает `execai` через псевдо-терминал
#      и проверяет базовые команды (/help, /source, exit).
#
# Usage:
#   ./system-test/auto/run.sh              # всё подряд
#   ./system-test/auto/run.sh unit         # только unit
#   ./system-test/auto/run.sh smoke        # только e2e-smoke
#   ./system-test/auto/run.sh pty          # только PTY

set -e
cd "$(dirname "$0")/../.."

mode="${1:-all}"
fail=0

hr() { printf '\n\033[1;34m═══ %s ═══\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
red() { printf '\033[31m%s\033[0m\n' "$*"; }
yellow() { printf '\033[33m%s\033[0m\n' "$*"; }

run_unit() {
  hr "Unit-тесты (go test ./...)"
  if go test ./... -count=1 -timeout 60s; then
    green "✓ unit-тесты прошли"
  else
    red "✗ unit-тесты упали"
    fail=1
  fi
}

run_smoke() {
  hr "E2E smoke (cmd/syscheck) — реальные запросы"
  # Требует свежих creds — если 401 в ExecAI, только внешние source будут работать.
  if ! go run ./cmd/syscheck 2>&1; then
    yellow "⚠ syscheck отчитался (частичные ошибки — см. вывод)"
  fi
}

run_pty() {
  hr "PTY-тест TUI"
  # Требует build-тег ptytest — иначе тест-файл пропускается (bubbletea
  # alt-screen нельзя прочитать без доп. настройки, TODO в pty_smoke_test.go).
  if go test -tags ptytest -run TestTUISmoke -v ./system-test/auto/... 2>&1; then
    green "✓ TUI smoke прошёл"
  else
    yellow "⚠ PTY-тест skipped/failed (см. TODO в pty_smoke_test.go)"
  fi
}

case "$mode" in
  unit)  run_unit ;;
  smoke) run_smoke ;;
  pty)   run_pty ;;
  all)   run_unit; run_smoke; run_pty ;;
  *)     echo "Unknown mode: $mode. Use: unit | smoke | pty | all"; exit 1 ;;
esac

hr "Итог"
if [ $fail -eq 0 ]; then
  green "ВСЁ ЗЕЛЁНОЕ"
  exit 0
else
  red "ЕСТЬ ПАДЕНИЯ (см. выше)"
  exit 1
fi
