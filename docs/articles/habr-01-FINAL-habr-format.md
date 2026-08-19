# Один интерфейс — девять LLM-провайдеров: как мы подружили Kimi K3, GLM-5.2, Claude и Ollama в одном терминальном агенте

Привет, Хабр! Я делаю execai — терминальный AI-агент на Go (bubbletea), в духе Claude Code. Он читает файлы, гоняет shell-команды, ходит в kubernetes и стримит ответы в TUI.

В какой-то момент выяснилось, что пользователям нужен не «агент с одной моделью», а мультитул: у кого-то подписка Kimi Code за $19, у кого-то GLM Coding Plan за $18, у кого-то корпоративный ключ Anthropic, а кто-то хочет гонять Ollama локально и не платить вообще. И всё это — в одном чате, с общей историей, с переключением на лету.

Под катом — как устроена мультипровайдерная архитектура: один интерфейс из пяти строк, два несовместимых мира API (Anthropic-compat и OpenAI-compat), SSE-парсеры с аккумуляцией tool calls, динамические каталоги моделей, автодетект тарифа подписки и делегирование в чужие CLI. С реальным кодом и граблями, на которые мы наступили.

<cut />

## Задача

Хотелось вот такого UX:

<source>
/source          ← меню: execai, zai, kimi, kimi-api, anthropic, openai,
                   claude-cli, codex-cli, ollama
/source kimi     ← переключились на Kimi Code
объясни этот код ← отвечает Kimi K3
/source zai      ← переключились на GLM-5.2
продолжай        ← GLM видит ВСЮ историю разговора, включая ответы Kimi
</source>

<img src="SHOT:final-1-source-picker.png" alt="/source — пикер провайдеров: 9 источников со статусами подключения"/>

Ключевые требования:

1. **Общая история.** Переключение источника не сбрасывает контекст — следующая модель видит всё, что было до неё.
2. **Общий агентный цикл.** Инструменты (Bash, Read, Edit, Grep…), подтверждения опасных команд, лимиты итераций — всё работает одинаково поверх любого провайдера.
3. **Биллинг изолирован.** Подписка пользователя — его подписка. Наш бэкенд в запросах к чужим API не участвует вообще.

## Интерфейс из пяти строк

Весь зоопарк провайдеров прячется за одним интерфейсом:

<source lang="go">
// StreamingLLM is the standard LLM-provider contract for the tool-use loop.
type StreamingLLM interface {
    Stream(ctx context.Context, messages []AIMessage,
        tools []map[string]any, cb StreamCallbacks) (*StreamResult, error)
}
</source>

На вход — история сообщений и JSON-схемы инструментов. На выход — результат:

<source lang="go">
type StreamResult struct {
    Content      string
    ToolCalls    []ToolCall
    FinishReason string
}

// StreamCallbacks — UI callbacks (showing text deltas and tool_call starts).
type StreamCallbacks struct {
    OnText      func(string)      // инкремент видимого текста
    OnToolCall  func(name string) // модель начала вызывать инструмент — UI покажет "▶ Bash…"
    OnReasoning func(string)      // chain-of-thought (thinking-модели) — рисуем приглушённо
}
</source>

Агентный цикл (tool-use loop) держит `StreamingLLM` и не знает, куда физически уходят запросы. Переключение источника — это буквально замена одного поля:

<source lang="go">
m.cli = m.makeLLMClient() // пересоздать клиент под активную подписку
</source>

Звучит тривиально. Дьявол, как обычно, в реализациях.

## Два мира: Anthropic-compat и OpenAI-compat

Все девять провайдеров сводятся к двум диалектам HTTP API.

**OpenAI-compat** (`POST /v1/chat/completions`, авторизация `Bearer`):
модель отвечает SSE-чанками вида `choices[].delta.content`, инструменты приходят как `delta.tool_calls[]` с индексами.

**Anthropic-compat** (`POST /v1/messages`, заголовки `x-api-key` + `anthropic-version`):
события `message_start` / `content_block_delta` / `message_delta`, у thinking-моделей отдельные `thinking_delta`.

Раскладка получилась такой:

| Провайдер | Диалект | Endpoint |
|---|---|---|
| execai (наш gateway) | OpenAI-compat | api.execai.ru |
| Z.ai Coding Plan | **Anthropic-compat** | api.z.ai/api/anthropic |
| Kimi Code (подписка) | **Anthropic-compat** | api.kimi.com/coding |
| Moonshot Platform (pay-per-token) | OpenAI-compat | api.moonshot.ai/v1 |
| Anthropic API | Anthropic-compat | api.anthropic.com |
| OpenAI API | OpenAI-compat | api.openai.com/v1 |
| Ollama cloud | Anthropic-compat | ollama.com |
| Ollama local | OpenAI-compat | localhost:11434 |
| Claude Code CLI / Codex CLI | свой формат (об этом ниже) | локальный бинарь |

Сюрприз №1, стоивший нам вечера: **ключ Z.ai Coding Plan работает ТОЛЬКО через Anthropic-совместимый endpoint**. Тот же самый ключ в OpenAI-совместимый `/chat/completions` возвращает `429 Insufficient balance` — подписка и pay-per-token у них биллятся раздельно, и «баланс» на pay-per-token стороне нулевой. Мы долго думали, что ключ протух.

Сюрприз №2: у Kimi то же разделение, но жёстче. Kimi Code (kimi.com/code, подписка от $19/мес) и Moonshot Platform (platform.moonshot.ai, оплата за токены) — два разных продукта с разными ключами и разными endpoint'ами, ключи взаимно не подходят. Мы сделали их двумя разными источниками — `kimi` и `kimi-api`, чтобы пользователь не гадал.

В коде выбор клиента — обычный switch:

<source lang="go">
case subscriptions.SourceKimi:
    // Kimi Code Coding Plan subscription (kimi.com/code).
    // Endpoint: api.kimi.com/coding — Anthropic-compat + thinking.
    base := active.BaseURL
    if base == "" {
        base = "https://api.kimi.com/coding"
    }
    return llm.NewAnthropicClient(base, active.APIKey, m.current.ID, m.cfg.ThinkingBudget)

case subscriptions.SourceKimiAPI:
    // Moonshot Platform pay-per-token API key (platform.moonshot.ai).
    base := active.BaseURL
    if base == "" {
        base = "https://api.moonshot.ai/v1"
    }
    return llm.NewGLMClient(base, active.APIKey, m.current.ID)
</source>

Итого на девять провайдеров хватает **четырёх реализаций** `StreamingLLM`: клиент нашего gateway, generic OpenAI-compat клиент, generic Anthropic-compat клиент и обёртки над локальными CLI.

## SSE-парсер: аккумуляция tool calls по индексам

Самая противная часть стриминга — инструменты. Модель отдаёт вызов инструмента не целиком, а размазанным по чанкам: в первом чанке имя функции и кусочек аргументов, дальше — только дельты аргументов. Склеивать надо по индексу:

<source lang="go">
for _, tc := range ch.Delta.ToolCalls {
    idx := tc.Index
    existing, ok := toolByIdx[idx]
    if !ok {
        existing = &ToolCall{Index: idx, ID: tc.ID, Type: tc.Type, Function: tc.Function}
        toolByIdx[idx] = existing
        if cb.OnToolCall != nil && tc.Function.Name != "" {
            cb.OnToolCall(tc.Function.Name) // UI сразу показывает "▶ Bash…"
        }
    } else {
        // Accumulate the arguments deltas.
        existing.Function.Arguments += tc.Function.Arguments
    }
}
</source>

Если склеить неаккуратно — получите невалидный JSON аргументов и инструмент, который «не парсится» через раз. Отдельная радость — `reasoning_content`: DeepSeek, GLM и Kimi шлют размышления в разных полях (`reasoning`, `reasoning_content`, `thinking_delta` у Anthropic-диалекта), и всё это надо сводить в один колбэк `OnReasoning`.

<img src="SHOT:final-2-glm52-interface.png" alt="GLM-5.2 через Ollama Cloud отвечает на вопрос про interface в Go — виден reasoning"/>

## Динамические каталоги моделей

Хардкодить список моделей провайдера — путь к вечно протухшему каталогу. Для OpenAI-совместимых источников мы при подключении дёргаем `GET /v1/models` и строим каталог из того, что реально доступно этому ключу:

<source lang="go">
// buildOpenAIDynamicCatalog — primary picker takes the best of the available ones.
primaryOrder := []string{"gpt-5", "gpt-5-mini", "o3", "gpt-4.1", "gpt-4o", "o4-mini"}
idsSet := map[string]bool{}
for _, id := range ids {
    idsSet[id] = true
}
primaryID := ""
for _, want := range primaryOrder {
    if idsSet[want] {          // первый по ПРИОРИТЕТУ, а не первый по списку сервера
        primaryID = want
        break
    }
}
</source>

Грабля, которую поймал юнит-тест: первая версия итерировала по списку сервера и брала первый приоритетный — в итоге при `ids = [gpt-4o, gpt-5]` primary становился gpt-4o. Итерировать надо по списку приоритетов.

## Автодетект тарифа: что на самом деле даёт подписка

У Kimi Code есть приятная особенность: `GET /coding/v1/models` возвращает только те модели, которые доступны на вашем тарифе. Мы этим пользуемся — при подключении и при переключении источника агент сам определяет уровень подписки:

<source lang="go">
switch {
case has["k3"] && has["kimi-for-coding-highspeed"]:
    return "K3 + HighSpeed"
case has["k3"]:
    return "K3"
case has["kimi-for-coding"]:
    return "K2.7 Code"
}
</source>

И статус-бар честно показывает `src : kimi (K3 + HighSpeed)` — пользователь видит, что его тариф реально включает флагманский Kimi K3, без походов в личный кабинет.

Тем же способом `/usage` показывает живые квоты: у Kimi Code есть недокументированный-но-стабильный `GET /coding/v1/usages` с недельным лимитом и rolling-окнами (мы нашли его в исходниках их собственного CLI на GitHub). Из смешного: числа `used`/`limit` сервер отдаёт то как int, то как строку — в зависимости от версии. Парсим через `json.RawMessage` и «гибкий» декодер.

<img src="SHOT:final-4-usage-kimi.png" alt="/usage — живые квоты Kimi Code: тариф K3+HighSpeed, недельный лимит 48%, rolling 5ч окно"/>

## На чём это реально работает: Kimi K3 и GLM-5.2

Пара слов о моделях, ради которых всё затевалось — оба флагмана из «дешёвых» подписок оказались рабочими лошадками, а не компромиссом.

**Kimi K3** (Moonshot AI, подписка Kimi Code: Moderato $19/мес, дальше Allegretto $39, Allegro $99, Vivace $199 — тарифы отличаются множителем квоты) — флагман с нативным thinking mode и контекстом до 256K (до 1M на старших тарифах). В агентных задачах ведёт себя очень уверенно: сам решает, когда дёрнуть kubectl, аккуратно строит цепочки инструментов, reasoning читается осмысленно. Важная деталь для интеграции: **ID моделей строгие** — сервер принимает `k3`, `kimi-for-coding`, `kimi-for-coding-highspeed`, а привычное `kimi-k3` отдаёт 401, что при отладке выглядит как «ключ не подходит» и знатно путает.

**GLM-5.2** (Z.ai, GLM Coding Plan: Lite $18/мес, Pro $72, Max $160; при годовой оплате заметно дешевле) — MoE 753B/40B с dual thinking, заточенный под кодинг. Из необычного — суффикс-модификатор контекста прямо в ID модели: `glm-5.2[1m]` включает окно в 1M токенов. По нашему опыту GLM-5.2 — лучший «рефакторщик» в этой ценовой категории: длинные правки по многим файлам держит стабильно.

Обе подписки покупаются без карты западного банка, что для части нашей аудитории решающий фактор.

## Чужой CLI как LLM-провайдер

Отдельный трюк — источники `claude-cli` и `codex-cli`. Если у пользователя уже стоит Claude Code с подпиской Pro/Max или Codex CLI с ChatGPT-подпиской, мы не просим API-ключ: делегируем запросы в локальный бинарь, который сам ходит со своей OAuth-сессией.

<source lang="go">
cmd := exec.CommandContext(ctx, c.Path,
    "--print", "--output-format", "stream-json", "--include-partial-messages")
cmd.Stdin = strings.NewReader(flattenMessagesToPrompt(messages))
</source>

`claude -p --output-format stream-json` отдаёт JSONL, внутри которого — знакомые Anthropic-события (`message_start`, `content_block_delta`…). Парсим их тем же кодом, что и прямой API. История разговора уходит одним плоским промтом с тегами ролей — session-id у чужого CLI нам недоступен, но для «продолжи мысль» этого достаточно.

Ограничение честно показываем пользователю: наши инструменты (Bash/Read/Write) через делегирование не работают — у Claude Code свои и своя система разрешений.

## Грабли переключения источников

Самое коварное в мультипровайдерности — не подключение, а **переключение**. Три инварианта, выстраданные багами:

**1. Одинаковые ID в разных каталогах.** `glm-5.2` есть и у Z.ai, и в Ollama cloud. Если при переключении искать модель только по ID — можно взять запись из чужого каталога с чужим `Provider` и уйти запросом не туда. Правило: при смене источника модель ищется в НОВОМ каталоге, и берётся именно его запись:

<source lang="go">
func pickForNewCatalog(catalog []llm.Model, current llm.Model) llm.Model {
    for _, mm := range catalog {
        if mm.ID == current.ID {
            return mm // same ID — but THIS catalog's entry (правильный Provider)
        }
    }
    // ID нет в новом каталоге — берём primary
    ...
}
</source>

**2. Возврат на дефолтный источник должен восстанавливать снапшот.** После `/source zai → /source execai` в каталоге не должно остаться GLM-моделей: иначе запрос уйдёт в наш gateway с provider=zai и получит 401. Держим снимок исходного каталога и восстанавливаем его.

**3. Клиент пересоздаётся всегда.** Ленивая оптимизация «клиент тот же, поменяю только модель» ломается на смене типа клиента (Anthropic-compat ↔ OpenAI-compat). Пересоздание — копеечное, багов — на вечер.

<img src="SHOT:final-3-kimi-k3-kubectl.png" alt="Kimi K3 проверяет живой Kubernetes-кластер: 5 нод Ready, 89 подов Running"/>

## Что в итоге

Ядро — интерфейс из пяти строк и четыре его реализации. Всё остальное — аккуратная сантехника: два диалекта SSE, склейка tool calls, динамические каталоги, инварианты переключения. Зато пользователь получает одну команду `/source` и свободу: Kimi K3 по подписке Kimi Code, GLM-5.2 по GLM Coding Plan, Claude по ключу, Ollama бесплатно локально — в одном чате с общей историей.

Код открыт (Business Source License): **github.com/execai/execai-agent** — там же README на пяти языках и бинарники под Linux/macOS/Windows. Поставить:

<source lang="bash">
curl -fsSL https://raw.githubusercontent.com/execai/execai-agent/main/install.sh | bash
</source>

Вопросы по архитектуре с удовольствием отвечу в комментариях. В следующей статье — сага о том, как мы делали выделение текста «как в Claude Code» и поймали deadlock в bubbletea на ровном месте.
