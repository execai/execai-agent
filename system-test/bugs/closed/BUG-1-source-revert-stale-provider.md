# BUG-1: возврат на ExecAI оставлял чужой provider/model → 401

**Discovered:** 2026-06-30
**Closed:** 2026-06-30 (R5 коммит — fix applySubscriptionSource)
**Severity:** High (главный CLI flow ломается, нужен restart)

## Симптом

1. `/source zai` (или anthropic/claude-cli) — переключиться на внешнюю
   подписку. Каталог моделей → GLM, primary = glm-5.2. ОК.
2. `/source execai` — вернуться на дефолт. **Status bar:**
   `src:ExecAI provider:zai model:glm-5.2` ← provider/model устарели!
3. Послать сообщение → `401 от /aicore-vbai/agent-stream — токен
   истёк (execai login) или endpoint ещё не открыт`.
4. На самом деле токен валиден; реальная причина — наш gateway
   получил `model=glm-5.2 provider=zai` и отказался.

## Root cause

`applySubscriptionSource()` в `internal/chat/subs_commands.go` при
`active==nil` (=ExecAI) только пересоздавал `m.cli`, но НЕ:

- Восстанавливал `m.models` обратно в исходный каталог из
  `/billing-vbai/models_public` (там оставались GLMModels с прошлого
  переключения)
- Проверял что `m.current.ID` есть в каталоге → не подбирал primary

## Fix

1. В `tuiModel` добавлено поле `execAIModels []llm.Model` — снапшот
   исходного каталога, заполняется в `init()` сразу после
   `FetchModels()`.
2. В `applySubscriptionSource` при `active==nil`:
   - `m.models = m.execAIModels`
   - Если `m.current` не из этого каталога — `pickPrimary()`
3. Вынесены хелперы `modelInCatalog`, `pickPrimary` (DRY с веткой
   внешней подписки).

## Regression test

T01.C — обязателен в каждом прогоне system-test.
