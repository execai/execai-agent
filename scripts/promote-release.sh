#!/usr/bin/env bash
# promote-release.sh — продвинуть сборку CI из dev-бакета в прод-канал.
#
# Как устроен конвейер. Jenkins собирает ветку и кладёт артефакты в
# s3://execai-agent-dev/execai/<ветка>/<номер сборки>/ (версия = R<ветка>.<номер>).
# Прод — отдельный бакет: пользователи ставятся из него (install.sh читает
# execai-agent-prod/execai/stable). Продвижение всегда делается руками: сборка
# зелёная ещё не значит «выпускаем».
#
# Скрипт копирует одну сборку в три места прода:
#   execai/R<мажор>/<номер>/  — архив конкретной сборки (из него берёт publish-release.sh)
#   execai/R<мажор>/latest/   — последняя в этой мажорной ветке
#   execai/stable/            — то, что получают все, кто ставит агента
#
#   scripts/promote-release.sh <ветка> <номер сборки>
#   scripts/promote-release.sh R6 69
#
# Требует ~/.local/share/agent-vbai/yc-s3-credentials.env.

set -euo pipefail

BRANCH="${1:-}"
BUILD="${2:-}"
[[ -n "$BRANCH" && -n "$BUILD" ]] || { echo "Usage: $0 <ветка> <номер сборки>   (например: $0 R6 69)" >&2; exit 2; }

CREDS="$HOME/.local/share/agent-vbai/yc-s3-credentials.env"
[[ -f "$CREDS" ]] || { echo "нет $CREDS — без ключей S3 продвигать нечем" >&2; exit 2; }
set -a; source "$CREDS"; set +a

S3="aws --endpoint-url=https://storage.yandexcloud.net s3"
SRC="s3://execai-agent-dev/execai/${BRANCH}/${BUILD}/"
MAJOR="${BRANCH}"

echo "==> источник: $SRC"
VERSION="$($S3 cp "${SRC}VERSION.txt" - 2>/dev/null || true)"
[[ -n "$VERSION" ]] || { echo "в сборке нет VERSION.txt — такой сборки в dev нет?" >&2; exit 1; }
echo "==> версия сборки: $VERSION"

# Проверяем комплектность ДО копирования: половина артефактов в проде хуже,
# чем ничего — установщик выберет битую платформу и упадёт у пользователя.
NEED=(execai-linux-amd64.tar.gz execai-linux-arm64.tar.gz
      execai-windows-amd64.zip execai-windows-arm64.zip
      execai-darwin-amd64.tar.gz execai-darwin-arm64.tar.gz SHA256SUMS)
HAVE="$($S3 ls "$SRC" | awk '{print $4}')"
for f in "${NEED[@]}"; do
  grep -qx "$f" <<<"$HAVE" || { echo "в сборке нет $f — не продвигаю" >&2; exit 1; }
done
echo "==> все 6 бинарей и SHA256SUMS на месте"

for dst in "execai/${MAJOR}/${BUILD}/" "execai/${MAJOR}/latest/" "execai/stable/"; do
  echo "==> копирую в prod/$dst"
  $S3 cp --recursive --no-progress "$SRC" "s3://execai-agent-prod/$dst" >/dev/null
done

echo "==> прод обновлён:"
echo "    stable  = $(curl -fsS https://storage.yandexcloud.net/execai-agent-prod/execai/stable/VERSION.txt)"
echo "    ${MAJOR}/latest = $(curl -fsS https://storage.yandexcloud.net/execai-agent-prod/execai/${MAJOR}/latest/VERSION.txt)"
echo
echo "Дальше — GitHub Release:  scripts/publish-release.sh ${VERSION#R} --build ${BUILD}"
