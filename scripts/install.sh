#!/usr/bin/env bash
# Устанавливает execai из публичного prod-бакета Yandex Object Storage.
#
# Два способа запуска:
#   1) Обычный (нужен потом hash -r в текущем терминале):
#        curl -fsSL https://storage.yandexcloud.net/execai-agent-prod/execai/R5/latest/install.sh | bash
#   2) Sourced (всё автоматом — рекомендуется):
#        source <(curl -fsSL https://storage.yandexcloud.net/execai-agent-prod/execai/R5/latest/install.sh)
#
# Sourced-режим: скрипт работает в ТВОЁМ shell, поэтому может очистить hash-кеш
# и обновить $PATH — после установки просто набери 'execai' и всё работает.

# Определяем: нас sourced (`.`, `source`) или запущено (`bash`, `./install.sh`)?
# Если sourced — $0 будет 'bash'/'zsh'/etc, а BASH_SOURCE не совпадает с $0.
_execai_sourced=0
if [ -n "${BASH_SOURCE:-}" ] && [ "${BASH_SOURCE[0]}" != "$0" ]; then
    _execai_sourced=1
fi

# Используем set -e (не set -eu — 'unset var' в sourced-режиме может испортить
# юзерский shell).
if [ "$_execai_sourced" = "0" ]; then
    set -euo pipefail
else
    set -e
fi

# Универсальный exit: в sourced-режиме используем return (иначе закроем терминал).
_execai_die() {
    echo "$1" >&2
    if [ "$_execai_sourced" = "1" ]; then
        return 1 2>/dev/null || kill -INT $$
    else
        exit 1
    fi
}

BUCKET="${BUCKET:-https://storage.yandexcloud.net/execai-agent-prod}"
PREFIX="${PREFIX:-execai/R5/latest}"

# Если юзер не задал INSTALL_DIR — выбираем умно:
#  1. /usr/local/bin — если уже пишется без sudo (root / OWNED) ИЛИ есть passwordless-sudo
#  2. ~/.local/bin — fallback
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
    *)        _execai_die "неподдерживаемая ОС: $(uname -s). Для Windows используй install.ps1." ; return 1 2>/dev/null ;;
esac

case "$(uname -m)" in
    x86_64|amd64)   arch=amd64 ;;
    aarch64|arm64)  arch=arm64 ;;
    *) _execai_die "неподдерживаемая архитектура: $(uname -m)" ; return 1 2>/dev/null ;;
esac

# sha256 — на Linux 'sha256sum', на macOS 'shasum -a 256'.
if command -v sha256sum >/dev/null 2>&1; then
    SHA256="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
    SHA256="shasum -a 256"
else
    _execai_die "не найден sha256sum или shasum — не могу проверить контрольную сумму"
fi

archive="execai-${os}-${arch}.tar.gz"
base="$BUCKET/$PREFIX"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "==> Скачиваю $base/$archive"
curl -fsSL "$base/$archive" -o "$tmp/$archive"

echo "==> Проверяю SHA256"
curl -fsSL "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"
# Сравниваем хеши руками (не через --check --status) — так работает
# идентично на Linux (sha256sum) и macOS (shasum -a 256), не полагаясь
# на GNU-специфичные флаги.
expected=$(grep " $archive\$" "$tmp/SHA256SUMS" | awk '{print $1}')
actual=$($SHA256 "$tmp/$archive" | awk '{print $1}')
if [ -z "$expected" ]; then
    _execai_die "SHA256 не найден в SHA256SUMS для $archive"
fi
if [ "$expected" != "$actual" ]; then
    _execai_die "SHA256 не совпал: ожидали=$expected, вычислили=$actual"
fi

tar -xzf "$tmp/$archive" -C "$tmp"
if [ "$SUDO" = "1" ]; then
    sudo install -m 0755 "$tmp/execai-${os}-${arch}" "$INSTALL_DIR/execai"
else
    mkdir -p "$INSTALL_DIR"
    install -m 0755 "$tmp/execai-${os}-${arch}" "$INSTALL_DIR/execai"
fi
echo "==> Установлено: $INSTALL_DIR/execai"

# === Удаляем ВСЕ старые копии execai из PATH ===
# Юзерская проблема: install.sh кладёт свежий бинарь в /usr/local/bin,
# но в PATH была ещё одна копия в ~/.local/bin (или ~/bin, /usr/bin) —
# и PATH тянул старую. Результат: `execai version` показывает старую
# несмотря на успешную установку.
# Решение: пробегаем ВЕСЬ PATH, находим все execai (кроме нашего свежего)
# и сносим их. Это одноразовая операция; юзеру не надо разбираться сам.
IFS=: read -ra PATH_DIRS <<< "$PATH"
CLEANED=0
for d in "${PATH_DIRS[@]}"; do
    [ -z "$d" ] && continue
    p="$d/execai"
    [ ! -e "$p" ] && continue
    # Нашу свежую копию не трогаем
    if [ "$p" = "$INSTALL_DIR/execai" ]; then continue; fi
    # Симлинк на нашу свежую — оставляем (полезно, ссылается корректно)
    if [ -L "$p" ] && [ "$(readlink -f "$p" 2>/dev/null)" = "$(readlink -f "$INSTALL_DIR/execai" 2>/dev/null)" ]; then continue; fi
    # Удаляем
    if [ -w "$d" ] 2>/dev/null; then
        rm -f "$p" 2>/dev/null && { echo "==> Удалил старую копию: $p"; CLEANED=$((CLEANED+1)); }
    elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
        sudo rm -f "$p" 2>/dev/null && { echo "==> Удалил старую копию (sudo): $p"; CLEANED=$((CLEANED+1)); }
    else
        echo "⚠ Нашёл старую копию $p, но нет прав удалить. Убери сам: rm $p"
    fi
done
# Сбросить кеш bash о путях команд — иначе `execai` всё ещё указывает на удалённый.
hash -r 2>/dev/null || true

"$INSTALL_DIR/execai" version

# Сохраняем SHA арки чтобы execai видел свою версию vs latest при запуске.
mkdir -p "$HOME/.config/execai"
grep " $archive\$" "$tmp/SHA256SUMS" | awk '{print $1}' > "$HOME/.config/execai/installed_arch_sha" 2>/dev/null || true

# === Чистим прежние '# execai installer'-блоки в rc-файлах если путь
# больше не существует (например /tmp/execai-test-install от давнего теста).
# Иначе rc-файл добавляет мёртвый путь в PATH при каждом входе, bash его
# кеширует, потом ищет там бинарь — не находит, "Нет такого файла".
clean_stale_installer_lines() {
    local rc="$1"
    [ ! -f "$rc" ] && return
    # Ищем строки export PATH="..." которые:
    # (а) идут после "# execai installer" И указывают на несуществующий каталог, либо
    # (б) добавляют /tmp/execai* пути (это ВСЕГДА мусор — /tmp мимолётный),
    #     ЛЮБОЙ такой export убираем + коммент вокруг него если он execai-installer'ский.
    # Иначе rc-файл добавляет мёртвый путь в PATH при каждом входе, bash его
    # кеширует, потом ищет там бинарь — не находит, "Нет такого файла".
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
    # (а) marker + export → удалить обе если dir не существует
    if stripped == "# execai installer" and i+1 < len(lines):
        m = re.match(r'export\s+PATH="([^:]+):\$PATH"', lines[i+1].strip())
        if m and not os.path.isdir(m.group(1)):
            i += 2
            changed = True
            continue
    # (б) любой export /tmp/execai*/PATH — просто сносим (плюс марker-коммент если он ЕСТЬ перед)
    m2 = re.match(r'export\s+PATH="(/tmp/execai[^":]*):\$PATH"', stripped)
    if m2:
        # Если пред. строка "# execai installer" — уберём и её.
        if out and out[-1].strip() == "# execai installer":
            out.pop()
        i += 1
        changed = True
        continue
    # (в) любой export dir/PATH где dir не существует и содержит 'execai' — тоже мусор
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

# === PATH: пишем в rc-файлы для НОВЫХ сессий ===
add_to_path_in() {
    local rc="$1"
    if [ -f "$rc" ] && grep -qsE "(^|:)$INSTALL_DIR(:|\$)" "$rc"; then
        return 0
    fi
    if [ -e "$rc" ] || touch "$rc" 2>/dev/null; then
        {
            echo ""
            echo "# execai installer"
            echo "export PATH=\"$INSTALL_DIR:\$PATH\""
        } >> "$rc"
    fi
}

# Если INSTALL_DIR не в PATH — добавляем в bashrc/zshrc.
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
        sh_name="$(basename "${SHELL:-bash}")"
        case "$sh_name" in
            zsh)  add_to_path_in "$HOME/.zshrc" ;;
            bash) add_to_path_in "$HOME/.bashrc"
                  [ -f "$HOME/.bash_profile" ] && add_to_path_in "$HOME/.bash_profile" ;;
            *)    add_to_path_in "$HOME/.profile" ;;
        esac
        ;;
esac

echo
# === ПОДНИМАЕМ execai В ТЕКУЩЕЙ СЕССИИ ===
# Делаем симлинк в одном из директорий что уже в PATH (если можем) —
# тогда execai будет виден сразу, без открытия нового терминала.
ensure_visible_now() {
    if command -v execai >/dev/null 2>&1 && [ "$(command -v execai)" = "$INSTALL_DIR/execai" ]; then
        return 0
    fi
    # Пробуем сделать симлинк в /usr/local/bin или в /usr/bin (если можем).
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

# macOS: Gatekeeper может заблочить неподписанный бинарь.
if [ "$os" = "darwin" ]; then
    xattr -d com.apple.quarantine "$INSTALL_DIR/execai" 2>/dev/null || true
fi

# === В sourced-режиме — доводим до полной готовности прямо в юзерском shell ===
if [ "$_execai_sourced" = "1" ]; then
    # 1. Обновляем PATH текущей сессии (если ещё не был)
    case ":$PATH:" in
        *":$INSTALL_DIR:"*) ;;
        *) export PATH="$INSTALL_DIR:$PATH" ;;
    esac
    # 2. Сбрасываем hash-кеш bash — забыть про удалённый старый бинарь
    hash -r 2>/dev/null || true
    # 3. Показываем что готово
    echo
    echo "✓ Готово. Набери:  execai"
    echo "(без аргументов — TUI чат; при первом запуске откроет браузер)"
    # Не return 0 — sourced-скрипт заканчивается сам
else
    # Запущен как обычный скрипт (curl | bash). Не запускаем execai
    # автоматически: exec из bash-подпроцесса ловит остатки escape-ответов
    # терминала на capability queries (OSC 11 / DA1), они прилетают в TUI
    # как мусорный текст. Проще попросить юзера набрать команду в чистом
    # родительском shell — не мусорит и надёжно.
    #
    # Проверяем не сидит ли в bash hash-кеше родительского shell'а старый
    # путь (из-за него `execai` даст "Нет такого файла"). Если он у нас
    # тоже кешировался (наследуется в env) — предупреждаем юзера.
    stale_hint=""
    if command -v bash >/dev/null 2>&1; then
        # hash в subshell не поможет, но мы можем ХОТЯ БЫ проверить есть ли
        # в открытых сессиях родительские пути (эвристика: смотрим hash в /proc
        # если parent bash доступен — сложно, пропустим). Просто дадим совет.
        :
    fi
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
fi
