#!/bin/bash
# publish-release.sh — публикация билда из S3 (velesbsdllc dev) как GitHub Release
# в публичном репо github.com/execai/execai-agent.
#
# Usage:
#   scripts/publish-release.sh 5.112 [--draft] [--prerelease]
#   scripts/publish-release.sh 5.112 --title "R5.112 — какая-то фича"
#
# Что делает:
#   1. Скачивает execai-{os}-{arch}.{tar.gz|zip} + SHA256SUMS из S3-прода
#      s3://execai-agent-prod/execai/R5/latest/ (или /N/ если задать N)
#   2. Создаёт git tag v5.NN на execai/main (если ещё нет)
#   3. gh release create v5.NN --repo execai/execai-agent с 7 assets и notes
#
# Требования:
#   * ~/.local/share/agent-vbai/yc-s3-credentials.env — AWS-ключи для S3
#   * gh CLI залогинен (github.com/nwaddon имеет write на execai/execai-agent)
#   * git remote 'execai' указывает на github.com:execai/execai-agent
#
# Notes:
# * По умолчанию берёт из S3 /latest/. Флаг --build N возьмёт /N/.
# * Notes-файл: scripts/release-notes/v<VERSION>.md (создать вручную заранее).
#   Если нет — используется --generate-notes от gh (авто-changelog из коммитов).
#
# See project_execai_public_repo_2026_07_20.md в auto-memory для workflow.

set -euo pipefail

if [ -z "${1:-}" ]; then
    echo "Usage: $0 <version> [--build N] [--draft] [--prerelease] [--title 'text']" >&2
    echo "Example: $0 5.113" >&2
    exit 1
fi

VERSION="$1"
shift

BUILD="latest"
DRAFT=""
PRERELEASE=""
CUSTOM_TITLE=""
while [ $# -gt 0 ]; do
    case "$1" in
        --build) BUILD="$2"; shift 2 ;;
        --draft) DRAFT="--draft"; shift ;;
        --prerelease) PRERELEASE="--prerelease"; shift ;;
        --title) CUSTOM_TITLE="$2"; shift 2 ;;
        *) echo "unknown flag: $1" >&2; exit 1 ;;
    esac
done

TAG="v${VERSION}"
TITLE="${CUSTOM_TITLE:-R${VERSION}}"
REPO="execai/execai-agent"
S3_PREFIX="s3://execai-agent-prod/execai/R5/${BUILD}/"

echo "==> Publishing execai R${VERSION} to ${REPO} (tag ${TAG})"

# 1. Скачать бинари из S3
SCRATCH=$(mktemp -d /tmp/execai-release-${VERSION}.XXXXXX)
trap "rm -rf $SCRATCH" EXIT

if [ ! -f "$HOME/.local/share/agent-vbai/yc-s3-credentials.env" ]; then
    echo "✗ Нет ~/.local/share/agent-vbai/yc-s3-credentials.env — не смогу скачать бинари" >&2
    exit 1
fi

# shellcheck source=/dev/null
source "$HOME/.local/share/agent-vbai/yc-s3-credentials.env"
# The env file sets shell vars without `export` — aws-cli reads only the
# environment, so export them explicitly (otherwise it falls back to
# ~/.aws credentials and fails with SignatureDoesNotMatch).
export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY

echo "==> Downloading from ${S3_PREFIX}"
aws --endpoint-url=https://storage.yandexcloud.net s3 cp --recursive \
    "$S3_PREFIX" "$SCRATCH/" \
    --exclude "install.*" \
    --exclude "VERSION.txt" \
    >/dev/null

# Проверка что все 6 бинарей + SHA256SUMS есть
for f in \
    execai-linux-amd64.tar.gz execai-linux-arm64.tar.gz \
    execai-windows-amd64.zip  execai-windows-arm64.zip \
    execai-darwin-amd64.tar.gz execai-darwin-arm64.tar.gz \
    SHA256SUMS; do
    if [ ! -f "$SCRATCH/$f" ]; then
        echo "✗ Missing artifact: $f" >&2
        exit 1
    fi
done
echo "==> Downloaded 6 binaries + SHA256SUMS"

# 2. Тег на execai/main
git fetch execai main --tags >/dev/null 2>&1 || true
if git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null; then
    echo "==> Tag ${TAG} уже существует локально — пропускаю создание"
else
    MAIN_SHA=$(git rev-parse execai/main)
    git tag "${TAG}" "${MAIN_SHA}"
    git push execai "${TAG}"
    echo "==> Tag ${TAG} создан и запушен → ${MAIN_SHA:0:12}"
fi

# 3. Создать release
REPO_ROOT=$(git rev-parse --show-toplevel)
NOTES_FILE="${REPO_ROOT}/scripts/release-notes/${TAG}.md"
NOTES_ARG=""
if [ -f "$NOTES_FILE" ]; then
    NOTES_ARG="--notes-file $NOTES_FILE"
    echo "==> Using notes: ${NOTES_FILE}"
else
    NOTES_ARG="--generate-notes"
    echo "==> No custom notes file (${NOTES_FILE}) — using auto-generated"
fi

cd "$SCRATCH"
# shellcheck disable=SC2086
gh release create "${TAG}" \
    --repo "${REPO}" \
    --title "${TITLE}" \
    ${NOTES_ARG} \
    ${DRAFT} \
    ${PRERELEASE} \
    SHA256SUMS \
    execai-linux-amd64.tar.gz  execai-linux-arm64.tar.gz  \
    execai-darwin-amd64.tar.gz execai-darwin-arm64.tar.gz \
    execai-windows-amd64.zip   execai-windows-arm64.zip

echo
echo "✓ Published: https://github.com/${REPO}/releases/tag/${TAG}"
