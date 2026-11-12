# T01 — Source/Model switching

**Что проверяем:** переключение между ExecAI / Z.ai / Anthropic / claude-cli
и обратно. После каждого переключения должно ВСЁ синхронизироваться:
src + provider + model в status bar, m.cli, m.models в /model picker.

**Регрессии которые сценарий ловит:**
- BUG: после `/source zai` → `/source execai` оставался `provider:zai`
  и `model:glm-5.2` → 401 от `/aicore-vbai/agent-stream`. Фикс
  R5 коммит 2026-06-30.

## Шаги

### A. Boot
1. `execai` → залогиниться (если еще не залогинен)
2. Status bar: `src:ExecAI provider:<тариф> model:<твоя primary>`
3. **Проверка:** `/model` показывает каталог из биллинга (claude-sonnet/opus,
   gpt-5, gemini, qwen и т.д.)

### B. ExecAI → Z.ai
1. `/connect zai <api-key>` (если ещё не подключен)
2. `/source zai`
3. Status bar: `src:zai (coding) provider:zai model:glm-5.2`
4. `/model` → видны GLMModels (glm-5.2/5.2[1m]/4.7)
5. Послать "привет" → ответ от GLM, в `~/.local/share/agent-vbai/requests.log`
   запись `source=zai-anthropic` или `zai`

### C. Z.ai → ExecAI (КРИТИЧНО — баг был тут)
1. `/source execai`
2. **Проверка статус-бара:** `src:ExecAI provider:<ваш-провайдер>
   model:<ваша primary>` (НЕ `zai` / `glm-5.2`!)
3. `/model` показывает ИСХОДНЫЙ каталог ExecAI (не GLMModels)
4. Послать "привет" → НЕ должно быть 401. Ответ приходит через
   `/aicore-vbai/agent-stream`.
5. В `requests.log` запись `source=execai`

### D. ExecAI → Anthropic → ExecAI
Повторить B+C, но через `/source anthropic`. Те же проверки.

### E. ExecAI → claude-cli → ExecAI
Повторить B+C, но через `/source claude-cli`. Доп. проверка:
- В `/model` алиасы sonnet/opus/haiku + pinned
- Выбрать `sonnet` → запрос реально идёт через локальный `claude --model sonnet`
- После возврата на ExecAI каталог снова исходный

### F. Цепочка: zai → anthropic → claude-cli → execai
Без рестарта CLI. После последнего перехода — `/model` показывает
ExecAI каталог, status bar корректный, реальный запрос отрабатывает.

## Pass-criteria
Все 6 шагов прошли БЕЗ необходимости перезапуска CLI и БЕЗ 401.
