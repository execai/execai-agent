#!/bin/bash
# WA2: задача агенту через НАСТОЯЩИЙ чат-конвейер (aichat→aihandler→tools-vbai→модель→agents-vbai→serve).
# Использование: ./wa2_stream.sh <run-dir>
set -u
R=${1:?run dir}
GW=https://apidev.velesbsd.com
TOKEN=$(python3 -c "import json,os;print(json.load(open(os.path.expanduser('~/.config/execai/credentials.json')))['token'])")
AG=$(python3 -c "import json,os;print(json.load(open(os.path.expanduser('~/.config/execai/credentials.json')))['agent_id'])")

# Пейлоад — ровно та форма, что собирает фронтовый requestBuilder:
# один агент в проекте → {"tool_id":"agent","alias":"<agent_id>"}.
cat > $R/wa2b_payload.json <<JSON
{"messages":[{"role":"user","content":"Спроси у агента, какая на машине версия ОС и сколько свободного места на диске. Перескажи мне его ответ."}],
 "tools":[{"tool_id":"agent","alias":"$AG"}],
 "system":[{"model":"MiniMax-M3","provider":"minimax"}]}
JSON

echo "[wa2b] стрим пошёл: $(date -u +%H:%M:%S)"
curl -sS -N --max-time 300 -X POST "$GW/aichat-vbai/stream" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d @$R/wa2b_payload.json > $R/wa2b_stream.log 2>&1
echo "[wa2b] стрим закрыт: $(date -u +%H:%M:%S), байт: $(wc -c < $R/wa2b_stream.log)"
