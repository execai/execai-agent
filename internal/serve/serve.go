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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/auth"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/fleet"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/llmpick"
	"github.com/velesbsdllc/agent-vbai/internal/machine"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
	"github.com/velesbsdllc/agent-vbai/internal/subagent"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// Task is a task from the inbox (the shared type of the "web → agent"
// channel, see internal/inbox).
type Task = inbox.Task

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
		PollWait:    inbox.PollWait,
		TaskTimeout: inbox.TaskTimeout,
		// 0 — взять из конфига (/max-iterations). Своё число тут означало бы,
		// что локальная настройка человека в фоне игнорируется.
		MaxIterations: 0,
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

	// Источник показываем явно: теперь фон идёт по активной подписке, и
	// человек должен видеть, чей баланс будет тратиться, ДО первой задачи.
	src := "execai"
	if subs, _ := subscriptions.Load(); subs != nil {
		if a := subs.ActiveSubscription(); a != nil {
			src = a.Provider
		}
	}
	fmt.Printf("execai serve · машина %s · источник %s · %s\n", displayName(cr), src, cfg.APIBase)
	fmt.Println("Слушаю задачи из веб-чата. Ctrl+C — выход.")
	switch {
	case opts.ReadOnly:
		fmt.Println("🔒 Режим только чтение: файлы не меняются, команды не запускаются.")
	case strict:
		fmt.Printf("🔒 Разрешено по твоему permissions.json: %s. Про остальное спрошу в веб-чате.\n",
			strings.Join(perms.Tools, ", "))
	default:
		fmt.Println("⚠ permissions.json пуст — инструменты выполняются БЕЗ подтверждения,")
		fmt.Println("  рядом нет человека, который мог бы отказать. Ограничить: execai serve --read-only")
	}
	if p := AuditPath(); p != "" {
		fmt.Printf("Журнал выполненного: %s\n", p)
	}

	// Plan gate: a task on the ExecAI models asks it before the model is
	// called. Billing is refreshed in the background and daily only while
	// the ExecAI models are the active source (my_plan may renew a period);
	// if the gate already says no, the person learns it now.
	i18n.Setup(cfg.Locale)
	gate := plan.New(cfg.APIBase)
	gate.Start(ctx, plan.OnExecAI)
	startSubs, _ := subscriptions.Load()
	if v := gate.Check(ctx, plan.RouteOf(startSubs)); !v.Allowed {
		fmt.Println(i18n.T("plan.serve.startWarn"))
		fmt.Println(v.Message())
	}
	// Tasks from the site come with the Work and Max plans, whatever model
	// runs them. Without one every task is answered with the refusal; say so
	// now, not after the first task.
	if !inbox.Allowed(ctx, gate, inbox.Background()) {
		fmt.Println(i18n.T("inbox.plan.serveWarn"))
		fmt.Println(inbox.PlanRefusal())
	}
	// Самая частая жалоба на фоновый режим — «закрыл терминал, агент умер».
	// Процесс привязан к сессии, и подсказать про это дешевле, чем городить
	// собственную демонизацию.
	if !isDetached() {
		fmt.Println("Подсказка: терминал закроется — агент остановится. Чтобы пережил:")
		fmt.Println("  setsid nohup execai serve > ~/.execai-serve.log 2>&1 &")
	}

	// Subagents of tasks: one manager for the daemon; their records go to
	// the machine's card in the web panel. On exit everything still
	// running ends "cancelled" and the last report is sent.
	agentFleet := fleet.New()
	tools.SetSubagentDefault(cfg.SubagentIterations)
	agents := subagent.NewManager(agentFleet)
	daemonCwd, _ := os.Getwd()
	agentReport := fleet.StartReporter(agentFleet, fleet.HTTPSender(cfg.APIBase, fleet.SavedToken), daemonCwd, 0)
	defer func() {
		agents.Shutdown(2 * time.Second)
		agentReport.Close()
		agentFleet.Close()
	}()

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
			// 401 чаще всего означает не «сервер сломался», а «токен ротировался»:
			// на этой же машине перелогинились в TUI, credentials.json обновился,
			// а мы держим старый токен в памяти. Без перечитывания демон крутил бы
			// вечный цикл 401 до ручного рестарта, а в вебе агент выглядел бы
			// «подключён, но не отвечает».
			if isUnauthorized(err) && reloadCredentials(cr) {
				fmt.Fprintln(os.Stderr, "токен обновился на диске — продолжаю с новым")
				backoff = time.Second
				continue
			}
			hint := ""
			if isUnauthorized(err) {
				hint = " · токен отклонён — выполни на этой машине: execai → /login"
			}
			fmt.Fprintf(os.Stderr, "инбокс недоступен (%v)%s — повтор через %s\n", err, hint, backoff)
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
			runTask(ctx, cfg, cr, t, opts, strict, audit, gate, agents)
		}
	}
}

// ackTasks подтверждает получение и возвращает только реально закреплённые
// за нами задачи.
func ackTasks(ctx context.Context, cfg *config.Config, token string, tasks []Task) ([]Task, error) {
	return inbox.NewClient(cfg.APIBase, token).Ack(ctx, tasks)
}

func displayName(cr *config.Credentials) string {
	if cr.Alias != "" {
		return cr.Alias
	}
	return cr.AgentID
}

// runTask выполняет одну задачу и отправляет результат.
func runTask(ctx context.Context, cfg *config.Config, cr *config.Credentials, t Task, opts Options,
	strict bool, audit *auditLog, gate *plan.Checker, agents *subagent.Manager) {
	prompt := extractPrompt(t.Payload)
	if prompt == "" {
		postResult(ctx, cfg, cr.Token, t.ID, "error", "пустая задача")
		return
	}
	fmt.Printf("\n▶ задача %s: %s\n", short(t.ID), firstLine(prompt))
	audit.setTask(t.ID)
	if !boardTask(ctx, cfg, cr.Token, t, gate) {
		return
	}
	if !gateTask(ctx, cfg, cr.Token, t, gate) {
		return
	}
	// The machine was called from the web or the phone: the getting-started
	// step "call the agent from your phone" is done.
	machine.MarkWebCall()

	// Каталог задачи — тот, что привязан к её проекту. Выполнять в текущем
	// каталоге демона нельзя: он мог быть запущен где угодно, и задача
	// «поправь тесты» ушла бы не в тот репозиторий.
	cwd, err := workDirFor(ctx, cfg, cr, t.WorkspaceID)
	if err != nil {
		postResult(ctx, cfg, cr.Token, t.ID, "error", err.Error())
		fmt.Printf("✗ %v\n", err)
		return
	}

	out, err := execute(ctx, cfg, cr, prompt, cwd, t.ID, opts, strict, audit, agents)
	if err != nil {
		if errors.Is(err, llm.ErrPaymentRequired) && gate != nil {
			gate.RefreshAsync() // the plan has most likely just ended
		}
		postResult(ctx, cfg, cr.Token, t.ID, "error", err.Error())
		fmt.Printf("✗ %v\n", err)
		return
	}
	postResult(ctx, cfg, cr.Token, t.ID, "final", out)
	fmt.Printf("✓ задача %s выполнена (%d символов)\n", short(t.ID), len([]rune(out)))
}

// boardTask asks whether the plan includes tasks from the site at all (the
// Work and Max plans). A refusal is the task's result, as with gateTask.
func boardTask(ctx context.Context, cfg *config.Config, token string, t Task, gate *plan.Checker) bool {
	// A task from the site is the person's action: billing may be asked.
	if gate == nil || inbox.Allowed(ctx, gate, true) {
		return true
	}
	postResult(ctx, cfg, token, t.ID, inbox.ChunkError, inbox.PlanRefusal())
	fmt.Println(i18n.Tf("inbox.serve.refused", short(t.ID)))
	return false
}

// gateTask asks the plan gate before a task reaches the model. A refusal is
// the task's result: the task is already acked, the web chat shows why, and
// the model is not called. Returns whether the task may run.
func gateTask(ctx context.Context, cfg *config.Config, token string, t Task, gate *plan.Checker) bool {
	if gate == nil {
		return true
	}
	subs, _ := subscriptions.Load()
	v := gate.Check(ctx, plan.RouteOf(subs))
	if v.Allowed {
		return true
	}
	postResult(ctx, cfg, token, t.ID, "error", v.Message())
	fmt.Println(i18n.Tf("plan.serve.refused", short(t.ID)))
	return false
}

// execute запускает агентный цикл в нужном каталоге.
func execute(ctx context.Context, cfg *config.Config, cr *config.Credentials,
	prompt, cwd, taskID string, opts Options, strict bool, audit *auditLog, agents *subagent.Manager) (string, error) {

	// Клиент выбирается ТАК ЖЕ, как в интерактиве: по активной подписке из
	// subscriptions.json. Иначе задачи из веба шли бы через ExecAI мимо
	// подписки, которой человек пользуется локально, — другой биллинг и,
	// возможно, другая модель.
	//
	// The model follows the same rules as the terminal and the editor
	// (llmpick.Model): the selected one when the active source offers it,
	// otherwise that source's primary model. Before, the selected ExecAI
	// model ID went to the person's own subscription, and the ExecAI catalog
	// was fetched (and could fail the task) even when it was not used.
	subs, _ := subscriptions.Load()
	var execai []llm.Model
	if subs.ActiveSubscription() == nil {
		execai = llm.FetchModelsCached(cfg.APIBase, cr.Token).Models
	}
	modelID, provider := llmpick.Model(cfg.SelectedModelID, subs, execai)
	if modelID == "" && subs.ActiveSubscription() == nil {
		return "", fmt.Errorf("не выбрать модель")
	}
	cli := llmpick.Client(cfg, subs, modelID, provider, cr.Token)

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

	if opts.MaxIterations <= 0 {
		opts.MaxIterations = cfg.GetMaxIterations()
	}
	collector := &textCollector{audit: audit}
	// Курированные разрешения проверяет сам agent.Agent ДО approver'а, так
	// что сюда доходит только то, чего в permissions.json нет.
	// Approver: спрашиваем в чате вместо немого отказа. Курированные
	// разрешения проверяет сам agent.Agent ДО обращения сюда, так что
	// вопросом тревожим только то, чего в permissions.json нет.
	perms, _ := agent.LoadPermissions()
	ap := agent.Approver(newWebApprover(ctx, cfg, cr.Token, taskID, perms, audit))
	if !strict {
		// Список пуст — человек ничего не настраивал; спрашивать про каждый
		// шаг значило бы сделать режим неработоспособным из коробки.
		ap = policyApprover{strict: false, audit: audit}
	}
	a := agent.New(cli, registry, sys, ap, collector)
	a.MaxIterations = opts.MaxIterations

	ctx, cancel := context.WithTimeout(ctx, opts.TaskTimeout)
	defer cancel()

	var runner *subagent.Runner
	if agents != nil {
		// Subagents of the task: the same client and source, the task's
		// directory, and the task's approver — a write subagent asks in the
		// web chat with its title in the question.
		runner = agents.Runner(subagent.Deps{
			Client: cli, Source: subs.SourceLabel(), Model: modelID,
			Picker: llmpick.NewPicker(cfg, subs, execai, cr.Token),
			Cwd:    cwd, Approver: ap, ParentChat: taskID,
		})
		ctx = tools.WithSubagentRunner(ctx, runner)
	}
	hist, err := a.Run(ctx, nil, prompt)
	// Background subagents: the task has no later turn, so the agent waits
	// for them here and finishes with their results.
	for runner != nil && err == nil && ctx.Err() == nil {
		note := runner.Drain(ctx)
		if note == "" {
			break
		}
		hist, err = a.Run(ctx, hist, note)
	}
	if err != nil || ctx.Err() != nil {
		// The task is over: its subagents go with it.
		if agents != nil {
			agents.CancelParent(taskID)
		}
	}
	// Even on an error, give what was done: a partial answer beats a bare
	// error (inbox.Outcome — the same rule as in the editor).
	chunk, data := inbox.Outcome(collector.String(), err)
	if chunk == inbox.ChunkError {
		return "", err // keep the type: runTask tells a 402 apart
	}
	return data, nil
}

// workDirFor finds the directory this machine bound to the task's project
// (another machine's binding of the same project does not count); a task
// outside any project runs where the daemon was started.
func workDirFor(ctx context.Context, cfg *config.Config, cr *config.Credentials, workspaceID string) (string, error) {
	cwd, _ := os.Getwd()
	return inbox.NewClient(cfg.APIBase, cr.Token).WorkDirFor(ctx, cr.AgentID, workspaceID, cwd)
}

func pollInbox(ctx context.Context, cfg *config.Config, token string, wait time.Duration) ([]Task, error) {
	return inbox.NewClient(cfg.APIBase, token).Poll(ctx, wait, nil)
}

func postResult(ctx context.Context, cfg *config.Config, token, taskID, chunkType, data string) {
	if err := inbox.NewClient(cfg.APIBase, token).PostResult(ctx, taskID, chunkType, data); err != nil {
		fmt.Fprintf(os.Stderr, "не удалось отправить результат задачи %s: %v\n", short(taskID), err)
	}
}

// isUnauthorized — является ли ошибка ответом HTTP 401.
func isUnauthorized(err error) bool { return inbox.IsUnauthorized(err) }

// reloadCredentials перечитывает credentials.json и возвращает true, если на
// диске лежит другой токен. Обновляет cr на месте: указатель на эти креды
// уже разошёлся по циклу задач.
func reloadCredentials(cr *config.Credentials) bool {
	fresh, err := config.LoadCredentials()
	if err != nil || fresh == nil || fresh.Token == "" || fresh.Token == cr.Token {
		return false
	}
	*cr = *fresh
	return true
}

// extractPrompt достаёт текст задачи из payload.
func extractPrompt(payload string) string { return inbox.ExtractPrompt(payload) }

func short(id string) string { return inbox.Short(id) }

func firstLine(s string) string { return inbox.FirstLine(s) }

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
