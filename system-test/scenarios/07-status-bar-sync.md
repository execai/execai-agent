# T07 — Status bar consistency

**Что проверяем:** `src / provider / model / think` всегда отражают
РЕАЛЬНОЕ состояние клиента. Никаких stale-значений.

## Шаги

1. После каждого из переходов в T01 — снимать скриншот status bar.
2. **Инвариант 1:** `src` соответствует тому что вернёт
   `/subscriptions` → active.
3. **Инвариант 2:** `provider` == `m.current.Provider`. Если src=ExecAI
   и m.current это claude-sonnet — provider=anthropic. Если src=zai —
   provider=zai всегда.
4. **Инвариант 3:** `model` == `m.current.ID` (или дружелюбное имя).
5. **Инвариант 4:** `think` (off/low/medium/high/max) применяется к
   запросу — провернуть `/thinking max` и убедиться через requests.log
   что budget_tokens передан.

## Известный паттерн бага
При неудачном переключении источника status-bar показывает корректный
`src` но устаревший `provider/model`. Это значит `applySubscriptionSource`
не пересинхронизировал `m.current`. См. T01.C для регрессии.

## Pass-criteria
Все 4 инварианта держатся после любых переключений.
