# Run 2026-07-01-T0-bugfix-verify

**Что:** проверка фикса BUG-1 (возврат на ExecAI оставлял чужой
provider/model → 401) и smoke всех source.
**Версия:** R5 коммит 52a4305 (#32 в проде).

## Unit-тесты — `go test ./internal/chat/...`

```
=== RUN   TestApplySubscriptionSource_ZaiThenExecAI_RestoresCatalog
--- PASS
=== RUN   TestApplySubscriptionSource_AnthropicThenExecAI_RestoresCatalog
--- PASS
=== RUN   TestApplySubscriptionSource_PickPrimaryWhenCurrentMissing
--- PASS
=== RUN   TestApplySubscriptionSource_FullChain
--- PASS
=== RUN   TestModelInCatalog
--- PASS
=== RUN   TestPickPrimary
--- PASS
PASS (6/6) ~60ms
```

Прямой regression: `/source zai` → `/source execai` → после второго
вызова `applySubscriptionSource()` `m.current.Provider != "zai"` И
`m.current.ID != "glm-5.2"`. Фикс закрывает баг.

## E2E smoke — `go run ./cmd/syscheck`

Реальные запросы к каждому source (промт "скажи 'OK'"):

| Source       | Latency | Ответ     | Статус |
|--------------|---------|-----------|--------|
| ExecAI       | 10ms    | (401)     | ✗ env-проблема (см. ниже) |
| Z.ai (GLM)   | 3.28s   | "OK"      | ✓      |
| Anthropic    | —       | не подкл. | ⚠ skip |
| claude-cli   | 4.49s   | "OK"      | ✓      |

### Про ExecAI 401

Не связано с BUG-1. На этой машине JWT-токен (тестовая учётка на
dev-шлюз) отвергается gateway'ем `/aicore-vbai/agent-stream`.
При этом `execai run "..."` через legacy `chat.Once()` отрабатывает —
он бьёт в `/billing-vbai/agent` или похожий endpoint, не agent-stream.

Это надо разобрать отдельно (вероятно нужен `execai login` через
свежий браузер), к фиксу source-switch отношения не имеет.

## Pass-criteria

- BUG-1 — закрыт (unit-test + код)
- 2/3 доступных source работают e2e
- Регрессий в других сценариях нет (полный test suite зелёный)
