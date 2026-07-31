#!/usr/bin/env bash
# execai installer — Linux / macOS (amd64 / arm64).
#
#   curl -fsSL https://raw.githubusercontent.com/execai/execai-agent/main/install.sh | bash
#
# Mirrors (binaries are identical, pick what is faster for you):
#   MIRROR=github  — GitHub Releases (default outside Russia)
#   MIRROR=yandex  — Yandex Object Storage (default for ru locales; faster in Russia/CIS)
#   MIRROR=auto    — pick by system locale, fall back to the other on failure (default)
#
#   MIRROR=yandex bash   ← usage with an explicit mirror:
#   curl -fsSL … | MIRROR=yandex bash
#
# Pin a version:  VERSION=5.136 curl -fsSL … | bash
#   (github: releases/download/v5.136, yandex: still the stable channel)
#
# Windows: use install.ps1 instead.

# Detect sourced (`source install.sh`) vs executed (`bash install.sh` / curl|bash).
_execai_sourced=0
if [ -n "${BASH_SOURCE:-}" ] && [ "${BASH_SOURCE[0]}" != "$0" ]; then
    _execai_sourced=1
fi

if [ "$_execai_sourced" = "0" ]; then
    set -euo pipefail
else
    set -e
fi

_execai_die() {
    echo "$1" >&2
    if [ "$_execai_sourced" = "1" ]; then
        return 1 2>/dev/null || kill -INT $$
    else
        exit 1
    fi
}

# Locale: RU messages if the system language is Russian, EN otherwise.
_L="${LC_ALL:-${LC_MESSAGES:-${LANG:-en}}}"
case "$_L" in
    ru*) _RU=1 ;;
    *)   _RU=0 ;;
esac
_m() { if [ "$_RU" = "1" ]; then printf '%s\n' "$1"; else printf '%s\n' "$2"; fi; }

# === Mirror selection ===
GH_REPO="execai/execai-agent"
YA_BASE="${BUCKET:-https://storage.yandexcloud.net/execai-agent-prod}/${PREFIX:-execai/stable}"
VERSION="${VERSION:-}"

gh_base() {
    if [ -n "$VERSION" ]; then
        echo "https://github.com/$GH_REPO/releases/download/v$VERSION"
    else
        echo "https://github.com/$GH_REPO/releases/latest/download"
    fi
}

MIRROR="${MIRROR:-auto}"
case "$MIRROR" in
    github) MIRRORS="github" ;;
    yandex) MIRRORS="yandex" ;;
    auto)
        # Russian locale → Yandex is usually much faster; otherwise GitHub.
        if [ "$_RU" = "1" ]; then MIRRORS="yandex github"; else MIRRORS="github yandex"; fi
        ;;
    *) _execai_die "MIRROR must be github|yandex|auto (got: $MIRROR)" ;;
esac

# === Install dir ===
SUDO=0
if [ -z "${INSTALL_DIR:-}" ]; then
    if [ -w /usr/local/bin ]; then
        INSTALL_DIR=/usr/local/bin
    elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null && [ -d /usr/local/bin ]; then
        INSTALL_DIR=/usr/local/bin
        SUDO=1
    else
        INSTALL_DIR="$HOME/.local/bin"
    fi
fi

case "$(uname -s)" in
    Linux*)   os=linux ;;
    Darwin*)  os=darwin ;;
    *) _execai_die "$(_m "неподдерживаемая ОС: $(uname -s). Для Windows используй install.ps1." "unsupported OS: $(uname -s). For Windows use install.ps1.")" ;;
esac

case "$(uname -m)" in
    x86_64|amd64)   arch=amd64 ;;
    aarch64|arm64)  arch=arm64 ;;
    *) _execai_die "$(_m "неподдерживаемая архитектура: $(uname -m)" "unsupported architecture: $(uname -m)")" ;;
esac

if command -v sha256sum >/dev/null 2>&1; then
    SHA256="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
    SHA256="shasum -a 256"
else
    _execai_die "$(_m "не найден sha256sum или shasum — не могу проверить контрольную сумму" "sha256sum or shasum not found — cannot verify the checksum")"
fi

archive="execai-${os}-${arch}.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# fetch <mirror> — download archive + SHA256SUMS from one mirror. Returns non-zero on failure.
fetch_from() {
    local mirror="$1" base
    case "$mirror" in
        github) base="$(gh_base)" ;;
        yandex) base="$YA_BASE" ;;
    esac
    _m "==> Скачиваю ($mirror) $base/$archive" "==> Downloading ($mirror) $base/$archive"
    curl -fsSL --connect-timeout 10 "$base/$archive" -o "$tmp/$archive" || return 1
    curl -fsSL --connect-timeout 10 "$base/SHA256SUMS" -o "$tmp/SHA256SUMS" || return 1
    return 0
}

FETCHED=0
for mirror in $MIRRORS; do
    if fetch_from "$mirror"; then
        FETCHED=1
        break
    fi
    _m "⚠ Зеркало $mirror недоступно — пробую другое…" "⚠ Mirror $mirror unavailable — trying another…"
done
[ "$FETCHED" = "1" ] || _execai_die "$(_m "не удалось скачать ни с одного зеркала" "failed to download from any mirror")"

_m "==> Проверяю SHA256" "==> Verifying SHA256"
expected=$(grep " $archive\$" "$tmp/SHA256SUMS" | awk '{print $1}')
actual=$($SHA256 "$tmp/$archive" | awk '{print $1}')
if [ -z "$expected" ]; then
    _execai_die "$(_m "SHA256 не найден в SHA256SUMS для $archive" "SHA256 for $archive not found in SHA256SUMS")"
fi
if [ "$expected" != "$actual" ]; then
    _execai_die "$(_m "SHA256 не совпал: ожидали=$expected, вычислили=$actual" "SHA256 mismatch: expected=$expected actual=$actual")"
fi

tar -xzf "$tmp/$archive" -C "$tmp"
if [ "$SUDO" = "1" ]; then
    sudo install -m 0755 "$tmp/execai-${os}-${arch}" "$INSTALL_DIR/execai"
else
    mkdir -p "$INSTALL_DIR"
    install -m 0755 "$tmp/execai-${os}-${arch}" "$INSTALL_DIR/execai"
fi
_m "==> Установлено: $INSTALL_DIR/execai" "==> Installed: $INSTALL_DIR/execai"

# === Remove ALL stale execai copies from PATH ===
IFS=: read -ra PATH_DIRS <<< "$PATH"
CLEANED=0
for d in "${PATH_DIRS[@]}"; do
    [ -z "$d" ] && continue
    p="$d/execai"
    [ ! -e "$p" ] && continue
    if [ "$p" = "$INSTALL_DIR/execai" ]; then continue; fi
    if [ -L "$p" ] && [ "$(readlink -f "$p" 2>/dev/null)" = "$(readlink -f "$INSTALL_DIR/execai" 2>/dev/null)" ]; then continue; fi
    if [ -w "$d" ] 2>/dev/null; then
        rm -f "$p" 2>/dev/null && { echo "$(_m "==> Удалил старую копию: $p" "==> Removed stale copy: $p")"; CLEANED=$((CLEANED+1)); }
    elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
        sudo rm -f "$p" 2>/dev/null && { echo "$(_m "==> Удалил старую копию (sudo): $p" "==> Removed stale copy (sudo): $p")"; CLEANED=$((CLEANED+1)); }
    else
        echo "$(_m "⚠ Нашёл старую копию $p, но нет прав удалить. Убери сам: rm $p" "⚠ Found a stale copy at $p but no permission to remove it. Remove manually: rm $p")"
    fi
done
hash -r 2>/dev/null || true

"$INSTALL_DIR/execai" version

mkdir -p "$HOME/.config/execai"
grep " $archive\$" "$tmp/SHA256SUMS" | awk '{print $1}' > "$HOME/.config/execai/installed_arch_sha" 2>/dev/null || true

# === Clean dead '# execai installer' PATH lines in rc files ===
clean_stale_installer_lines() {
    local rc="$1"
    [ ! -f "$rc" ] && return
    python3 - "$rc" <<'PY' 2>/dev/null || true
import os, re, sys
path = sys.argv[1]
try:
    with open(path) as f:
        lines = f.readlines()
except OSError:
    sys.exit()
out = []
i = 0
changed = False
while i < len(lines):
    ln = lines[i]
    stripped = ln.strip()
    if stripped == "# execai installer" and i+1 < len(lines):
        m = re.match(r'export\s+PATH="([^:]+):\$PATH"', lines[i+1].strip())
        if m and not os.path.isdir(m.group(1)):
            i += 2
            changed = True
            continue
    m2 = re.match(r'export\s+PATH="(/tmp/execai[^":]*):\$PATH"', stripped)
    if m2:
        if out and out[-1].strip() == "# execai installer":
            out.pop()
        i += 1
        changed = True
        continue
    m3 = re.match(r'export\s+PATH="([^":]+):\$PATH"', stripped)
    if m3 and "execai" in m3.group(1) and not os.path.isdir(m3.group(1)):
        if out and out[-1].strip() == "# execai installer":
            out.pop()
        i += 1
        changed = True
        continue
    out.append(ln)
    i += 1
if changed:
    with open(path, 'w') as f:
        f.writelines(out)
PY
}
for rc in "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.zshrc" "$HOME/.profile"; do
    clean_stale_installer_lines "$rc"
done

# === PATH for NEW sessions ===
add_to_path_in() {
    local rc="$1"
    if [ -f "$rc" ] && grep -qsE "(^|:)$INSTALL_DIR(:|\$)" "$rc"; then
        return 0
    fi
    if [ -f "$rc" ] || [ "$rc" = "$HOME/.bashrc" ]; then
        {
            echo ""
            echo "# execai installer"
            echo "export PATH=\"$INSTALL_DIR:\$PATH\""
        } >> "$rc"
    fi
}
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
        add_to_path_in "$HOME/.bashrc"
        [ -f "$HOME/.zshrc" ] && add_to_path_in "$HOME/.zshrc"
        ;;
esac

# Make `execai` visible right now via a symlink in a dir already on PATH.
ensure_visible_now() {
    command -v execai >/dev/null 2>&1 && return 0
    for d in /usr/local/bin /usr/bin "$HOME/bin"; do
        case ":$PATH:" in *":$d:"*) ;; *) continue ;; esac
        if [ -w "$d" ] 2>/dev/null; then
            ln -sf "$INSTALL_DIR/execai" "$d/execai" 2>/dev/null && return 0
        elif [ "$SUDO" = "1" ] && [ -d "$d" ]; then
            sudo ln -sf "$INSTALL_DIR/execai" "$d/execai" 2>/dev/null && return 0
        fi
    done
    return 1
}
ensure_visible_now || true

# macOS: Gatekeeper may block an unsigned binary.
if [ "$os" = "darwin" ]; then
    xattr -d com.apple.quarantine "$INSTALL_DIR/execai" 2>/dev/null || true
fi

if [ "$_execai_sourced" = "1" ]; then
    case ":$PATH:" in
        *":$INSTALL_DIR:"*) ;;
        *) export PATH="$INSTALL_DIR:$PATH" ;;
    esac
    hash -r 2>/dev/null || true
    echo
    _m "✓ Готово. Набери:  execai" "✓ Done. Type:  execai"
    _m "(без аргументов — TUI чат; при первом запуске откроет браузер)" "(no arguments — TUI chat; first run opens the browser)"
else
    if [ "$_RU" = "1" ]; then
        cat <<EOF

✓ execai установлен в $INSTALL_DIR/execai

Запусти:
  execai

Если получишь "bash: /tmp/…/execai: Нет такого файла" — bash в этом
терминале закешировал старый путь. Сделай одно из:
  hash -r && execai        # сбросить кеш и запустить
  exec bash                # перезапустить shell (сохраняя терминал)
  <открой новый терминал>  # там кеш пустой
EOF
    else
        cat <<EOF

✓ execai installed at $INSTALL_DIR/execai

Run:
  execai

If you get "bash: /tmp/…/execai: No such file or directory" — bash in
this terminal cached the old path. Do one of:
  hash -r && execai        # reset the cache and run
  exec bash                # restart the shell (keeps the terminal)
  <open a new terminal>    # fresh cache there
EOF
    fi
fi
