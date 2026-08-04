// Package serve — фоновый режим: агент слушает задачи из веб-чата.
//
// Без него канал «веб → агент» обрывается на последнем шаге: задача
// создаётся, но забрать её некому, и в чате пользователь видит «агент не
// отвечает».
//
// Почему отдельный режим, а не работа внутри TUI: инбокс должен иметь ровно
// одного владельца. Если бы задачи забирал и TUI, и демон, они бы делили один
// поток задач случайным образом, и пользователь не понимал бы, куда ушла его
// задача и почему вывод появился не там.
//
// Что разрешено агенту — решает сам пользователь своим permissions.json
// (набитым в TUI кнопкой «Всегда»); пустой список означает «разрешено всё» с
// громким предупреждением при старте. Подробности и причины — policy.go.
package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/auth"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
	"github.com/velesbsdllc/agent-vbai/internal/version"
)

// Task — задача из инбокса.
type Task struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Payload     string `json:"payload"`
}

type inboxResponse struct {
	Tasks []Task `json:"tasks"`
}

type binding struct {
	WorkspaceID string `json:"workspace_id"`
	LocalPath   string `json:"local_path"`
}

// Options — параметры фонового режима.
type Options struct {
	// PollWait — сколько держать long-poll. Сервер ограничивает сверху.
	PollWait time.Duration
	// TaskTimeout — предел на одну задачу. Без него зависшая задача заняла бы
	// агента навсегда, и все следующие ждали бы её.
	TaskTimeout time.Duration
	// MaxIterations — предел цикла инструментов для задачи из веба. Меньше,
	// чем в интерактиве: рядом нет человека, который скажет «стоп».
	MaxIterations int
	// ReadOnly — не давать агенту менять файлы и запускать команды. Нужно,
	// когда демон пускают в чужой репозиторий.
	ReadOnly bool
}

// DefaultOptions — разумные значения.
func DefaultOptions() Options {
	return Options{
		PollWait:      60 * time.Second,
		TaskTimeout:   5 * time.Minute,
		MaxIterations: 30,
	}
}

// Run — основной цикл: забрать задачи, выполнить, вернуть результат.
// Возвращает управление только по отмене контекста.
func Run(ctx context.Context, cfg *config.Config, opts Options) error {
	cr, err := auth.Require()
	if err != nil {
		return err
	}
	if cr.AgentID == "" {
		return fmt.Errorf("эта сессия не агент — выполни на этой машине: execai, затем /login")
	}

	// Замок берём ДО всего остального: если демон уже работает, второй должен
	// умереть сразу, ничего не сломав и не начав опрашивать инбокс.
	release, err := acquireLock(cfg.APIBase)
	if err != nil {
		return err
	}
	defer release()

	// Политику определяем ОДИН раз при старте и сразу говорим вслух: человек
	// должен узнать, что разрешено его агенту, до первой задачи, а не после.
	perms, _ := agent.LoadPermissions()
	strict := perms != nil && (len(perms.Tools) > 0 || len(perms.Exact) > 0)
	audit := newAuditLog()
	defer audit.close()

	fmt.Printf("execai serve · машина %s · %s\n", displayName(cr), cfg.APIBase)
	fmt.Println("Слушаю задачи из веб-чата. Ctrl+C — выход.")
	switch {
	case opts.ReadOnly:
		fmt.Println("🔒 Режим только чтение: файлы не меняются, команды не запускаются.")
	case strict:
		fmt.Printf("🔒 Разрешено по твоему permissions.json: %s. Остальное будет отклонено.\n",
			strings.Join(perms.Tools, ", "))
	default:
		fmt.Println("⚠ permissions.json пуст — инструменты выполняются БЕЗ подтверждения,")
		fmt.Println("  рядом нет человека, который мог бы отказать. Ограничить: execai serve --read-only")
	}
	if p := AuditPath(); p != "" {
		fmt.Printf("Журнал выполненного: %s\n", p)
	}
	// Самая частая жалоба на фоновый режим — «закрыл терминал, агент умер».
	// Процесс привязан к сессии, и подсказать про это дешевле, чем городить
	// собственную демонизацию.
	if !isDetached() {
		fmt.Println("Подсказка: терминал закроется — агент остановится. Чтобы пережил:")
		fmt.Println("  setsid nohup execai serve > ~/.execai-serve.log 2>&1 &")
	}

	// Ошибки сети не должны крутить цикл вхолостую на полной скорости.
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nостановлен")
			return nil
		default:
		}

		tasks, err := pollInbox(ctx, cfg, cr.Token, opts.PollWait)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			fmt.Fprintf(os.Stderr, "инбокс недоступен (%v) — повтор через %s\n", err, backoff)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			if backoff < 60*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second

		// Выдача из инбокса — только предложение. Пока мы не подтвердили
		// получение, задача остаётся pending: так она не теряется, если наш
		// ответ ушёл в разорванное соединение. Выполняем строго то, что
		// сервер подтвердил нам обратно.
		tasks, err = ackTasks(ctx, cfg, cr.Token, tasks)
		if err != nil {
			fmt.Fprintf(os.Stderr, "не удалось подтвердить получение задач: %v\n", err)
			continue // задачи остались pending — приедут следующим poll'ом
		}

		for _, t := range tasks {
			runTask(ctx, cfg, cr, t, opts, strict, audit)
		}
	}
}

// ackTasks подтверждает получение и возвращает только реально закреплённые
// за нами задачи.
func ackTasks(ctx context.Context, cfg *config.Config, token string, tasks []Task) ([]Task, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	body, _ := json.Marshal(map[string]any{"task_ids": ids})
	data, err := apiRequest(ctx, cfg, token, http.MethodPost, "/agents-vbai/inbox/ack", body)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Acked []string `json:"acked"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("не разобрать ответ ack: %w", err)
	}
	ackedSet := map[string]bool{}
	for _, id := range resp.Acked {
		ackedSet[id] = true
	}
	out := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		if ackedSet[t.ID] {
			out = append(out, t)
		}
	}
	return out, nil
}

func displayName(cr *config.Credentials) string {
	if cr.Alias != "" {
		return cr.Alias
	}
	return cr.AgentID
}

// runTask выполняет одну задачу и отправляет результат.
func runTask(ctx context.Context, cfg *config.Config, cr *config.Credentials, t Task, opts Options,
	strict bool, audit *auditLog) {
	prompt := extractPrompt(t.Payload)
	if prompt == "" {
		postResult(ctx, cfg, cr.Token, t.ID, "error", "пустая задача")
		return
	}
	fmt.Printf("\n▶ задача %s: %s\n", short(t.ID), firstLine(prompt))
	audit.setTask(t.ID)

	// Каталог задачи — тот, что привязан к её проекту. Выполнять в текущем
	// каталоге демона нельзя: он мог быть запущен где угодно, и задача
	// «поправь тесты» ушла бы не в тот репозиторий.
	cwd, err := workDirFor(ctx, cfg, cr.Token, t.WorkspaceID)
	if err != nil {
		postResult(ctx, cfg, cr.Token, t.ID, "error", err.Error())
		fmt.Printf("✗ %v\n", err)
		return
	}

	out, err := execute(ctx, cfg, cr, prompt, cwd, opts, strict, audit)
	if err != nil {
		postResult(ctx, cfg, cr.Token, t.ID, "error", err.Error())
		fmt.Printf("✗ %v\n", err)
		return
	}
	postResult(ctx, cfg, cr.Token, t.ID, "final", out)
	fmt.Printf("✓ задача %s выполнена (%d символов)\n", short(t.ID), len([]rune(out)))
}

// execute запускает агентный цикл в нужном каталоге.
func execute(ctx context.Context, cfg *config.Config, cr *config.Credentials,
	prompt, cwd string, opts Options, strict bool, audit *auditLog) (string, error) {

	models, err := llm.FetchModels(ctx, cfg.APIBase, cr.Token)
	if err != nil {
		return "", fmt.Errorf("не получить список моделей: %w", err)
	}
	current := llm.PickDefault(models, cfg.SelectedModelID)
	if current == nil {
		return "", fmt.Errorf("не выбрать модель")
	}
	// Именно AICoreClient, а не llm.New: агентный цикл требует потока с
	// поддержкой инструментов, простой Stream их не умеет.
	cli := llm.NewAICoreClient(cfg.APIBase, cr.Token, current.ID, current.Provider)

	registry := tools.Default(cwd)
	if opts.ReadOnly {
		// Тот же набор, которым работают субагенты: смотреть можно, менять нельзя.
		registry = tools.ReadOnly(cwd)
	}
	// Спрашивать некого: вопрос упрётся в ошибку и потратит итерацию. Модель
	// должна сразу выбирать между «сделать по разумному предположению» и
	// «объяснить в ответе, чего не хватило».
	registry.Unregister("AskUser")
	sys := agent.SystemPrompt(cwd, registry.Names(), agent.LoadMemory(cwd))

	collector := &textCollector{audit: audit}
	// Курированные разрешения проверяет сам agent.Agent ДО approver'а, так
	// что сюда доходит только то, чего в permissions.json нет.
	a := agent.New(cli, registry, sys, policyApprover{strict: strict, audit: audit}, collector)
	a.MaxIterations = opts.MaxIterations

	ctx, cancel := context.WithTimeout(ctx, opts.TaskTimeout)
	defer cancel()

	if _, err := a.Run(ctx, nil, prompt); err != nil {
		// Даже при ошибке отдаём то, что успели: частичный ответ полезнее
		// голого сообщения об ошибке.
		if partial := collector.String(); partial != "" {
			return partial + "\n\n(прервано: " + err.Error() + ")", nil
		}
		return "", err
	}
	res := collector.String()
	if strings.TrimSpace(res) == "" {
		return "(агент отработал, но текстового ответа не дал)", nil
	}
	return res, nil
}

// workDirFor находит каталог, привязанный к проекту задачи.
func workDirFor(ctx context.Context, cfg *config.Config, token, workspaceID string) (string, error) {
	if workspaceID == "" {
		// Задача вне проекта — работаем там, где запущен демон.
		return os.Getwd()
	}
	data, err := apiRequest(ctx, cfg, token, http.MethodGet, "/agents-vbai/workspaces/bindings", nil)
	if err != nil {
		return "", fmt.Errorf("не получить привязки проектов: %w", err)
	}
	var wrapped struct {
		Bindings []binding `json:"bindings"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return "", fmt.Errorf("не разобрать привязки: %w", err)
	}
	for _, b := range wrapped.Bindings {
		if b.WorkspaceID != workspaceID {
			continue
		}
		if b.LocalPath == "" {
			return "", fmt.Errorf("для этого проекта не указан каталог — выполни здесь: execai → /project bind")
		}
		if st, err := os.Stat(b.LocalPath); err != nil || !st.IsDir() {
			// Каталог мог переехать. Молча работать в другом месте нельзя.
			return "", fmt.Errorf("каталог проекта не найден: %s", b.LocalPath)
		}
		return b.LocalPath, nil
	}
	return "", fmt.Errorf("эта машина не привязана к проекту задачи — выполни: execai → /project bind")
}

func pollInbox(ctx context.Context, cfg *config.Config, token string, wait time.Duration) ([]Task, error) {
	path := fmt.Sprintf("/agents-vbai/inbox/poll?wait=%d", int(wait.Seconds()))
	data, err := apiRequest(ctx, cfg, token, http.MethodPost, path, []byte(`{}`))
	if err != nil {
		return nil, err
	}
	var resp inboxResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("не разобрать ответ инбокса: %w", err)
	}
	return resp.Tasks, nil
}

func postResult(ctx context.Context, cfg *config.Config, token, taskID, chunkType, data string) {
	body, _ := json.Marshal(map[string]string{"chunk_type": chunkType, "data": data})
	// Своим контекстом: если основной уже отменён (Ctrl+C), результат всё
	// равно надо доставить, иначе в чате останется висеть таймаут.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if _, err := apiRequest(ctx, cfg, token, http.MethodPost,
		"/agents-vbai/tasks/"+taskID+"/result", body); err != nil {
		fmt.Fprintf(os.Stderr, "не удалось отправить результат задачи %s: %v\n", short(taskID), err)
	}
}

func apiRequest(ctx context.Context, cfg *config.Config, token, method, path string, body []byte) ([]byte, error) {
	base := strings.TrimRight(cfg.APIBase, "/")
	var rdr *bytes.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", version.UserAgent())

	// Таймаут клиента должен превышать long-poll, иначе он рвал бы каждый
	// нормальный запрос к инбоксу.
	resp, err := (&http.Client{Timeout: 3 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(buf.String()))
	}
	return buf.Bytes(), nil
}

// extractPrompt достаёт текст задачи из payload.
func extractPrompt(payload string) string {
	var p struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		// payload мог прийти голой строкой — это тоже задача.
		return strings.TrimSpace(payload)
	}
	return strings.TrimSpace(p.Prompt)
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	r := []rune(s)
	if len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return string(r)
}

// textCollector собирает текст ответа и показывает ход работы в консоли
// демона — иначе непонятно, жив ли он и чем занят.
type textCollector struct {
	mu    sync.Mutex
	buf   strings.Builder
	audit *auditLog
}

func (c *textCollector) OnText(delta string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf.WriteString(delta)
}

func (c *textCollector) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

// Рассуждения в результат не попадают: пользователь ждёт ответ, а не ход мысли.
func (c *textCollector) OnReasoning(string) {}

func (c *textCollector) OnToolCall(name string, args json.RawMessage) {
	fmt.Printf("  · %s\n", name)
	// Пишем ПОПЫТКУ вызова: отказы фиксирует approver, а сюда попадает всё,
	// что модель вообще собиралась сделать.
	c.audit.record("call", name, string(args), "")
}

func (c *textCollector) OnToolChunk(string, string) {}

func (c *textCollector) OnToolResult(name string, _ string, err error) {
	if err != nil {
		fmt.Printf("  ✗ %s: %v\n", name, err)
	}
}

func (c *textCollector) OnIterationStart(int) {}
