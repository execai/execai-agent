#!/usr/bin/env bash
# Copies the browser build of the editor panel from vscode-vbai into the
# agent, where execai ui embeds it (internal/uiserve/assets, go:embed).
#
# Usage:
#   bash scripts/sync-panel.sh                 # from ../vscode-vbai/dist next to the repo
#   bash scripts/sync-panel.sh /path/to/dist   # from a given dist folder
#   VSCODE_VBAI=/path/to/vscode-vbai bash scripts/sync-panel.sh
#
# Build the panel first in vscode-vbai (npm run build); it writes
# dist/panel-browser.en.html and dist/panel-browser.ru.html. Each file must
# carry the {{NONCE}} and {{CONFIG}} placeholders the agent fills in.
set -euo pipefail

root="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
# In a worktree (_wt/<name>) the sibling repos sit next to the main checkout.
main="$(cd "$(git -C "$root" rev-parse --git-common-dir)/.." && pwd)"

if [[ $# -ge 1 ]]; then
  dist="$1"
elif [[ -n "${VSCODE_VBAI:-}" ]]; then
  dist="$VSCODE_VBAI/dist"
elif [[ -d "$root/../vscode-vbai/dist" ]]; then
  dist="$root/../vscode-vbai/dist"
else
  dist="$main/../vscode-vbai/dist"
fi

dest="$root/internal/uiserve/assets"
copied=0
for lang in en ru; do
  src="$dist/panel-browser.$lang.html"
  if [[ ! -f "$src" ]]; then
    echo "missing: $src" >&2
    exit 1
  fi
  for ph in '{{NONCE}}' '{{CONFIG}}'; do
    if ! grep -qF "$ph" "$src"; then
      echo "$src: no $ph placeholder — not a browser build of the panel" >&2
      exit 1
    fi
  done
  cp "$src" "$dest/panel-browser.$lang.html"
  echo "panel-browser.$lang.html ← $src ($(wc -c <"$src") bytes)"
  copied=$((copied + 1))
done
echo "copied $copied files into $dest; rebuild execai to embed them"
