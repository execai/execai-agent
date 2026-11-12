// Package ide — JSON-протокол для плагинов редакторов (VS Code / Cursor).
//
// Плагин запускает `execai ide --cwd <корень проекта>` и говорит с ним
// JSON-строками: stdin — от плагина, stdout — от агента, ПО ОДНОМУ JSON на
// строку. stdout принадлежит протоколу целиком; всё человеческое — в stderr,
// иначе любой случайный Printf ломает парсер на стороне редактора.
//
// Весь агентский цикл (инструменты, permissions, источники, память) живёт в
// CLI — плагин только рисует. Это то же разделение, что у веб-канала: обрыв
// протокола не имеет права давать агенту больше прав, чем явное решение
// человека, поэтому неотвеченный вопрос при закрытом редакторе = отказ.
//
// Чатов в одном процессе МНОГО. Раньше ход выполнялся прямо в цикле чтения
// stdin, и пока агент думал, вкладки не переключались: команда load_chat
// лежала в очереди до done. Теперь каждый чат — своя сессия с историей,
// разрешениями и горутиной хода; цикл чтения только раздаёт команды. Каждое
// событие несёт chat — идентификатор сессии, по которому панель раскладывает
// поток по вкладкам; события без chat — общие (state, chats, ошибки команд).
// Ходов одновременно — не больше parallelTurns: остальные ждут слот, и чат
// честно сообщает «в очереди», а не молчит.
//
// Версия протокола — Protocol. Плагин сверяет её из события ready и честно
// говорит «обнови execai», а не падает на незнакомом событии. Добавление
// поля chat и новых событий (wakeup, auto_user, queued) совместимо: старый
// плагин их не знает и пропускает.
package ide

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/catalog"
	"github.com/velesbsdllc/agent-vbai/internal/compact"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	subsconnect "github.com/velesbsdllc/agent-vbai/internal/connect"
	"github.com/velesbsdllc/agent-vbai/internal/fleet"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/llmpick"
	"github.com/velesbsdllc/agent-vbai/internal/machine"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
	"github.com/velesbsdllc/agent-vbai/internal/security"
	"github.com/velesbsdllc/agent-vbai/internal/sessions"
	"github.com/velesbsdllc/agent-vbai/internal/subagent"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
	"github.com/velesbsdllc/agent-vbai/internal/version"
)

// Protocol — версия контракта. Поднимать при НЕсовместимых изменениях;
// добавление нового поля или события совместимо и версию не меняет.
const Protocol = 1

// In — сообщение от плагина.
type In struct {
	Type string `json:"type"` // user | answer | stop | new_chat | ping | command
	// user
	Text    string     `json:"text,omitempty"`
	Context *EditorCtx `json:"context,omitempty"`
	// answer (на ask и ask_user)
	ID    string `json:"id,omitempty"`
	Value string `json:"value,omitempty"`
	// command: name = state | set_model | set_source | set_effort |
	// set_max_iterations | login | logout | connect | disconnect |
	// list_chats | load_chat | cancel_wakeup | wake_now | inbox_take |
	// bind | unbind | list_workspaces | list_agents | open_agent |
	// stop_agent | extend_agent | set_subagent_iterations | machine_state |
	// notify; value is the argument (extend_agent: "<id>" or "<id> <steps>",
	// notify: turn_done | approval | ask — see notify.go)
	Name string `json:"name,omitempty"`
	// notify approval: the tool asking for permission (a plain name only).
	Tool string `json:"tool,omitempty"`
	// bind: the project to bind this folder to (value works too).
	WorkspaceID string `json:"workspace_id,omitempty"`
	// connect: ключ провайдера и (опционально) свой base_url
	Key     string `json:"key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	// connect custom: protocol ("openai" | "anthropic"), the name the person
	// gave the endpoint, optional model IDs and extra headers.
	API     string            `json:"api,omitempty"`
	Label   string            `json:"label,omitempty"`
	Models  []string          `json:"models,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	// Chat — к какому чату относится сообщение (user, stop, load_chat,
	// cancel_wakeup, wake_now). Пусто — текущий (активный в панели).
	Chat string `json:"chat,omitempty"`

	// auto — the turn was started by the agent itself (a wake-up, a task
	// from the site), not by the person.
	auto bool
	// task — the turn is a task from the site (internal type web_task).
	task *webTask
}

// EditorCtx — что редактор знает о месте, где стоит человек.
type EditorCtx struct {
	Path      string `json:"path,omitempty"`      // активный файл (относительно cwd или абсолютный)
	Selection string `json:"selection,omitempty"` // выделенный текст
	Language  string `json:"language,omitempty"`
	// Files — приложенные кнопкой «+» файлы. Содержимое НЕ шлём: агент в том
	// же каталоге и прочитает Read'ом ровно то, что ему нужно.
	Files []string `json:"files,omitempty"`
}

// Out — событие агенту наружу. Одна структура на все типы: плагину так проще
// (один парсер), а пустые поля JSON не раздувают.
type Out struct {
	Type string `json:"type"`

	// Chat — сессия, к которой относится событие. Пусто у общих событий
	// (state, chats, pong, ответы на команды без чата).
	Chat string `json:"chat,omitempty"`

	// ready
	Version  string `json:"version,omitempty"`
	Protocol int    `json:"protocol,omitempty"`
	// Features — what this agent offers beyond the protocol version:
	// "notify" — desktop notifications on request (notify.go).
	Features []string `json:"features,omitempty"`
	Model    string   `json:"model,omitempty"`
	Source   string   `json:"source,omitempty"`
	Cwd      string   `json:"cwd,omitempty"`

	// text_delta / reasoning_delta / notice / error / auto_user / wakeup (причина)
	Text string `json:"text,omitempty"`

	// tool_call / tool_chunk / tool_result
	ID      string `json:"id,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Summary string `json:"summary,omitempty"`
	Chunk   string `json:"chunk,omitempty"`
	OK      *bool  `json:"ok,omitempty"`
	Tail    string `json:"tail,omitempty"`
	// tool_result: the person refused the call — it did not run. OK stays
	// true for it, as it always was: a panel released before this field
	// draws what it drew, a newer one tells a refusal from a call that ran.
	Denied bool `json:"denied,omitempty"`

	// ask / ask_user
	Question string      `json:"question,omitempty"`
	Options  []AskOption `json:"options,omitempty"`

	// files_changed
	Paths []string `json:"paths,omitempty"`

	// done
	Elapsed float64 `json:"elapsed,omitempty"`
	// iteration
	N int `json:"n,omitempty"`
	// state / chats
	Models      []NamedItem `json:"models,omitempty"`
	Sources     []NamedItem `json:"sources,omitempty"`
	Chats       []NamedItem `json:"chats,omitempty"`
	Efforts     []NamedItem `json:"efforts,omitempty"`
	Connectable []NamedItem `json:"connectable,omitempty"`
	User        string      `json:"user,omitempty"`   // email вошедшего, пусто = не вошёл
	Effort      string      `json:"effort,omitempty"` // текущий уровень
	MaxIter     int         `json:"max_iter,omitempty"`
	// SubagentIter — steps a new subagent gets by default.
	SubagentIter int         `json:"subagent_iter,omitempty"`
	Security     string      `json:"security,omitempty"`   // уровень доверия
	Securities   []NamedItem `json:"securities,omitempty"` // варианты для пикера
	// chat_loaded
	Msgs []ReplayMsg `json:"msgs,omitempty"`
	// chat_loaded: ход в этом чате ещё идёт (панель получит и его поток)
	Busy bool `json:"busy,omitempty"`
	// wakeup / chat_loaded: когда чат проснётся (RFC 3339)
	At string `json:"at,omitempty"`

	// turn_start / auto_user / chat_loaded: who started the chat — "web" (a
	// task from the site) or "user".
	Origin string `json:"origin,omitempty"`
	// inbox: the inbox state (listening | other_serve | other_ide | no_plan |
	// disabled | not_logged_in); in the state event the same goes to Inbox.
	State string `json:"state,omitempty"`
	Inbox string `json:"inbox,omitempty"`
	// inbox / state: who owns the inbox when it is not this window.
	OwnerPID int    `json:"owner_pid,omitempty"`
	OwnerCwd string `json:"owner_cwd,omitempty"`
	// inbox / state: the project this folder is bound to; absent — not bound.
	BoundWorkspace *WorkspaceRef `json:"bound_workspace,omitempty"`
	// ask_resolved: the question answered elsewhere, and where (web | panel |
	// cancel).
	AskID string `json:"ask_id,omitempty"`
	By    string `json:"by,omitempty"`
	// workspaces: the account's projects (an empty list is sent as []).
	Items *[]WorkspaceItem `json:"items,omitempty"`

	// tool_call: the key of the call in the turn; a subagent started by a
	// Task call carries it as parent_tool_call (see agents.go).
	CallID string `json:"call_id,omitempty"`
	// ask: the subagent that asks (absent — the chat's own agent).
	Agent *AgentRef `json:"agent,omitempty"`
	// agent_loaded: the subagent's title and transcript (State — its state).
	Title    string      `json:"title,omitempty"`
	Messages []ReplayMsg `json:"messages,omitempty"`
	// agents: the registry snapshot, written as "items" (Out.MarshalJSON).
	Agents *[]fleet.Record `json:"-"`

	// machine_state: the "My machine" card, the getting-started steps and
	// the link that calls this machine from the phone ("" without an agent
	// id). Sent on the machine_state command and by the agent itself after
	// login, logout, connect, disconnect and a change of the sync state.
	Machine  *machine.State    `json:"machine,omitempty"`
	Tutorial *machine.Tutorial `json:"tutorial,omitempty"`
	PhoneURL *string           `json:"phone_url,omitempty"`
}

// machineStateOut — the machine_state event for a snapshot.
func machineStateOut(snap machine.Snapshot) Out {
	m, t, u := snap.Machine, snap.Tutorial, snap.PhoneURL
	return Out{Type: "machine_state", Machine: &m, Tutorial: &t, PhoneURL: &u}
}

// machineWatchEvery — how often the panel looks at this machine's own files
// (sync.json, the inbox lock, the marks) for a change worth announcing.
var machineWatchEvery = 3 * time.Second

// machineChanged asks the main loop to send machine_state again. It never
// blocks: after the loop has ended nobody reads the queue.
func (s *server) machineChanged() {
	select {
	case s.userQ <- In{Type: "machine_changed"}:
	default:
	}
}

// watchMachine announces a change of what this machine shows about itself
// that happens outside the panel: sync switched on in a terminal, an
// exchange done, execai serve started or stopped. Local files only.
func (s *server) watchMachine(ctx context.Context) {
	sig := localMachineSig()
	t := time.NewTicker(machineWatchEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if cur := localMachineSig(); cur != sig {
				sig = cur
				s.machineChanged()
			}
		}
	}
}

// localMachineSig — a fingerprint of the local part of the snapshot.
func localMachineSig() string {
	in := machine.Local("", nil, nil, nil)
	sy := machine.Build(in).Machine.Sync
	listener := ""
	if in.Listener != nil {
		listener = fmt.Sprintf("%s/%d", in.Listener.Kind, in.Listener.PID)
	}
	return fmt.Sprintf("%+v|%s|%v|%v", sy, listener, in.Marks.PhoneShownAt, in.Marks.WebCallAt)
}

// ReplayMsg — одна реплика восстановленного чата (упрощённый вид для UI).
type ReplayMsg struct {
	Role string `json:"role"` // user | assistant | tool
	Tool string `json:"tool,omitempty"`
	Text string `json:"text"`
	// user, when the message went out with files or an editor context: what
	// the person typed (Said), the files attached (Files, full paths) and how
	// many pictures the model was given (Images). Text stays the whole
	// prompt, context block included — a panel that does not know these
	// fields shows what it always did.
	Said   *string  `json:"said,omitempty"`
	Files  []string `json:"files,omitempty"`
	Images int      `json:"images,omitempty"`
}

// NamedItem — пункт списка для пикеров плагина.
type NamedItem struct {
	ID     string `json:"id"`
	Label  string `json:"label,omitempty"`
	Active bool   `json:"active,omitempty"`
	// chats: ход идёт / запланировано пробуждение (RFC 3339)
	Busy bool   `json:"busy,omitempty"`
	Wake string `json:"wake,omitempty"`
	// chats: "web" — started by a task from the site; "user" — in the panel.
	Origin string `json:"origin,omitempty"`
}

// AskOption — вариант ответа. Слово в слово как в TUI и веб-чате: человек
// должен узнавать кнопки, а не переучиваться в каждом интерфейсе.
type AskOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Permission answers are one vocabulary shared with the web channel
// (agents-vbai/internal/ask); in the agent it lives in one place,
// inbox.Answer*. An unknown value means deny.

// Options — параметры запуска.
type Options struct {
	Cwd           string
	MaxIterations int
	// NoInbox — do not accept tasks from the site in this window (also
	// EXECAI_IDE_INBOX=off); the panel shows the inbox as disabled.
	NoInbox bool
	// In and Out carry the protocol: one JSON message per line. Nil means
	// os.Stdin / os.Stdout (the editor plugin); execai ui passes a pipe that
	// stays open while the process lives and a fan-out to browser tabs.
	In  io.Reader
	Out io.Writer
	// SyncNotices — lines of the background sync for the panel (nil — none).
	SyncNotices *SyncNotices
}

// inboxOff — accepting tasks from the site is turned off by the environment.
func inboxOff() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("EXECAI_IDE_INBOX"))) {
	case "off", "0", "false", "no":
		return true
	}
	return false
}

// parallelTurns — сколько чатов могут отвечать одновременно. Больше — не
// быстрее: лимиты провайдера общие на ключ, а два хода в одном каталоге и
// так рискуют править один файл. EXECAI_IDE_PARALLEL переопределяет.
func parallelTurns() int {
	if v, err := strconv.Atoi(os.Getenv("EXECAI_IDE_PARALLEL")); err == nil && v > 0 {
		return v
	}
	return 3
}

// wakeRetry — через сколько переспросить занятый чат, когда пришло время
// пробуждения, а ход ещё идёт.
const wakeRetry = 15 * time.Second

// turnDeps — снимок общего состояния на момент старта хода. Ход живёт в
// своей горутине, а модель/источник человек может сменить, пока он идёт:
// смена действует со СЛЕДУЮЩЕГО хода, текущий доезжает на том, с чем начал.
type turnDeps struct {
	cli      llm.StreamingLLM
	sys      string
	registry *tools.Registry
	maxIter  int
	modelID  string
	provider string
	// autoCompactPct — порог автосжатия истории (% окна контекста, 0 = выкл).
	autoCompactPct int
	// gate — the plan gate, asked before the model is called (nil = none);
	// planRoute — where the turn's calls go (plan.RouteOf at turn start).
	gate      *plan.Checker
	planRoute plan.Route
	// source — the source label; picker — the model choice of subagents
	// ("fast", an ID), from the same snapshot.
	source string
	picker *llmpick.Picker
}

// server — процесс целиком: писатель stdout, вопросы, реестр чатов.
type server struct {
	mu  sync.Mutex
	enc *json.Encoder

	answers   map[string]chan string // id вопроса → канал ответа
	answersMu sync.Mutex
	nextID    int

	chatsMu sync.Mutex
	chats   map[string]*chatSession
	active  *chatSession

	slots chan struct{} // занятые слоты параллельных ходов
	userQ chan In       // очередь главного цикла (пробуждения кладут сюда)
	cwd   string

	// taskTimeout — the limit for a task from the site (inbox.TaskTimeout).
	taskTimeout time.Duration

	// agents runs the chats' subagents (nil in tests that do not need them).
	agents *subagent.Manager

	// desktop shows the panel's desktop notifications (notify.go); nil when
	// this machine cannot or the setting is off.
	desktop *desktop
}

func newServer(enc *json.Encoder, cwd string) *server {
	return &server{
		enc: enc, answers: map[string]chan string{},
		chats: map[string]*chatSession{},
		slots: make(chan struct{}, parallelTurns()),
		userQ: make(chan In, 64), cwd: cwd,
		taskTimeout: inbox.TaskTimeout,
	}
}

func (s *server) emit(o Out) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.enc.Encode(o)
}

func (s *server) newQuestion() (string, chan string) {
	s.answersMu.Lock()
	defer s.answersMu.Unlock()
	s.nextID++
	id := fmt.Sprintf("q%d", s.nextID)
	ch := make(chan string, 1)
	s.answers[id] = ch
	return id, ch
}

// drop forgets a question answered elsewhere: a late answer from the panel
// then goes nowhere.
func (s *server) drop(id string) {
	s.answersMu.Lock()
	delete(s.answers, id)
	s.answersMu.Unlock()
}

func (s *server) deliver(id, value string) {
	s.answersMu.Lock()
	ch := s.answers[id]
	delete(s.answers, id)
	s.answersMu.Unlock()
	if ch != nil {
		ch <- value
	}
}

// chatSession — один чат: история, разрешения, ход (если идёт), пробуждение.
type chatSession struct {
	id  string
	srv *server

	mu         sync.Mutex
	sess       *sessions.Session
	history    []llm.AIMessage
	busy       bool
	turnCancel context.CancelFunc
	ap         *ideApprover

	// live — события текущего хода. Панель показывает ОДИН чат; когда человек
	// возвращается в тот, что ещё отвечает, поток хода нужно показать с
	// начала, а не с той точки, где он переключился. Соседние дельты
	// склеиваются, чтобы долгий ход не держал тысячи крошечных событий.
	live []Out
	// pendingAsk — вопрос (разрешение или AskUser), на который ещё нет ответа:
	// при возврате в чат его надо показать снова, иначе кнопки потеряны.
	pendingAsk *Out

	wakeReq   *tools.ScheduleRequest // запрос пробуждения из текущего хода
	wakeTimer *time.Timer
	// firstText — первое сообщение человека, пока чат ещё не сохранён: файл
	// появляется после первого хода, а вкладка нужна с первой секунды.
	firstText string
	// task — the task from the site this chat's current turn is running;
	// cleared when its result is sent.
	task *webTask
	// asks — one permission question of this chat at a time (askSem).
	asks chan struct{}
}

type chatKey struct{}

func withChat(ctx context.Context, c *chatSession) context.Context {
	return context.WithValue(ctx, chatKey{}, c)
}

func chatFrom(ctx context.Context) *chatSession {
	c, _ := ctx.Value(chatKey{}).(*chatSession)
	return c
}

// emit помечает событие чатом и пишет его в протокол; события хода ещё и
// запоминаются для повторного показа.
func (c *chatSession) emit(o Out) {
	o.Chat = c.id
	c.mu.Lock()
	if c.busy {
		c.record(o)
	}
	c.mu.Unlock()
	c.srv.emit(o)
}

// record — под c.mu.
func (c *chatSession) record(o Out) {
	switch o.Type {
	case "text_delta", "reasoning_delta":
		if n := len(c.live); n > 0 && c.live[n-1].Type == o.Type {
			c.live[n-1].Text += o.Text
			return
		}
	case "tool_chunk":
		if n := len(c.live); n > 0 && c.live[n-1].Type == "tool_chunk" && c.live[n-1].ID == o.ID {
			c.live[n-1].Chunk += o.Chunk
			return
		}
	case "tool_call", "tool_result", "iteration", "notice", "error", "auto_user":
	default:
		return // turn_start/done/ask — воспроизводятся отдельно
	}
	c.live = append(c.live, o)
}

func (s *server) getChat(id string) *chatSession {
	s.chatsMu.Lock()
	defer s.chatsMu.Unlock()
	return s.chats[id]
}

func (s *server) activeChat() *chatSession {
	s.chatsMu.Lock()
	defer s.chatsMu.Unlock()
	return s.active
}

func (s *server) setActive(c *chatSession) {
	s.chatsMu.Lock()
	s.active = c
	s.chatsMu.Unlock()
}

// newChat создаёт сессию с идентификатором сразу: панели нужен chat в каждом
// событии с первой же секунды. Файл появится с первым сообщением.
func (s *server) newChat(modelID, provider string) *chatSession {
	sess := sessions.New(modelID, provider, s.cwd)
	c := &chatSession{id: sess.ID, srv: s, sess: sess}
	c.ap = &ideApprover{c: c, sessionTools: map[string]bool{}, sessionExact: map[string]bool{}}
	s.chatsMu.Lock()
	s.chats[c.id] = c
	s.chatsMu.Unlock()
	return c
}

// loadChat поднимает чат с диска, если он ещё не в памяти.
func (s *server) loadChat(id string) (*chatSession, error) {
	if c := s.getChat(id); c != nil {
		return c, nil
	}
	full, err := sessions.Load(id)
	if err != nil {
		return nil, err
	}
	c := &chatSession{id: full.ID, srv: s, sess: full, history: append([]llm.AIMessage(nil), full.Messages...)}
	c.ap = &ideApprover{c: c, sessionTools: map[string]bool{}, sessionExact: map[string]bool{}}
	s.chatsMu.Lock()
	if prev := s.chats[id]; prev != nil { // гонка с параллельной загрузкой
		s.chatsMu.Unlock()
		return prev, nil
	}
	s.chats[id] = c
	s.chatsMu.Unlock()
	return c, nil
}

// persist сохраняет чат. Под c.mu.
func (c *chatSession) persistLocked(modelID string) {
	if len(c.history) == 0 {
		return
	}
	if modelID != "" {
		c.sess.Model = modelID
	}
	c.sess.Messages = c.history
	if err := c.sess.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "не сохранить сессию:", err)
	}
}

// loadedOut — событие chat_loaded для этого чата с текущим состоянием.
func (c *chatSession) loadedOut() Out {
	c.mu.Lock()
	defer c.mu.Unlock()
	o := replayOut(c.history)
	o.Busy = c.busy
	o.Origin = originOf(c.sess.Origin)
	if c.sess.WakeAt != nil {
		o.At = c.sess.WakeAt.Format(time.RFC3339)
		o.Text = c.sess.WakeReason
	}
	return o
}

// replayLive повторяет панели поток идущего хода и неотвеченный вопрос.
// Вызывается ПОСЛЕ chat_loaded, когда чат стал активным в панели.
func (c *chatSession) replayLive() {
	c.mu.Lock()
	busy := c.busy
	live := append([]Out(nil), c.live...)
	var ask *Out
	if c.pendingAsk != nil {
		cp := *c.pendingAsk
		ask = &cp
	}
	c.mu.Unlock()
	if !busy {
		return
	}
	c.srv.emit(Out{Type: "turn_start", Chat: c.id, Origin: c.origin()})
	for _, o := range live {
		c.srv.emit(o)
	}
	if ask != nil {
		c.srv.emit(*ask)
	}
}

// startTurn запускает ход в горутине. Возвращает false, если чат занят.
func (c *chatSession) startTurn(in In, deps turnDeps) bool {
	return c.startTurnThen(in, deps, nil)
}

// startTurnThen is startTurn with a hook that runs once the chat is busy but
// before the turn emits anything — to announce a new chat (its tab) ahead of
// its first events.
func (c *chatSession) startTurnThen(in In, deps turnDeps, before func()) bool {
	c.mu.Lock()
	if c.busy {
		c.mu.Unlock()
		return false
	}
	c.busy = true
	c.live = nil
	c.pendingAsk = nil
	c.wakeReq = nil
	if len(c.history) == 0 && c.firstText == "" {
		c.firstText = in.Text
	}
	hist := append([]llm.AIMessage(nil), c.history...)
	var ctx context.Context
	var cancel context.CancelFunc
	if in.task != nil {
		// A task from the site has the same limit as in execai serve: the
		// web chat must not wait for a stuck turn forever.
		ctx, cancel = context.WithTimeout(context.Background(), c.srv.taskTimeout)
		c.task = in.task
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	c.turnCancel = cancel
	c.mu.Unlock()
	if before != nil {
		before()
	}
	go c.runTurn(ctx, cancel, in, deps, hist)
	return true
}

func (c *chatSession) runTurn(ctx context.Context, cancel context.CancelFunc, in In, deps turnDeps, hist []llm.AIMessage) {
	// Слот: ждём честно и говорим об этом, а не молчим.
	select {
	case c.srv.slots <- struct{}{}:
	default:
		c.emit(Out{Type: "queued", Text: i18n.T("ide.turn.queued")})
		select {
		case c.srv.slots <- struct{}{}:
		case <-ctx.Done():
			c.finishTurn(cancel, nil, ctx.Err(), ctx, time.Now(), nil, deps)
			return
		}
	}
	defer func() { <-c.srv.slots }()

	start := time.Now()
	c.emit(Out{Type: "turn_start", Origin: c.origin()})
	if in.auto {
		// Человек должен видеть, что ход начал агент сам и с каким промтом.
		o := Out{Type: "auto_user", Text: in.Text}
		if in.task != nil {
			o.Origin = originWeb // "from the site", not a wake-up
		}
		c.emit(o)
	}
	// Plan gate: a refusal ends the turn with its text, the model is not called.
	if deps.gate != nil {
		if v := deps.gate.Check(ctx, deps.planRoute); !v.Allowed {
			c.finishTurn(cancel, nil, errors.New(v.Message()), ctx, start, nil, deps)
			return
		}
	}
	ctx = withChat(ctx, c)
	ctx = tools.WithWakeupSink(ctx, func(r tools.ScheduleRequest) {
		c.mu.Lock()
		c.wakeReq = &r
		c.mu.Unlock()
	})
	st := &streamer{c: c, changed: map[string]bool{}, turnStart: start}
	var ap agent.Approver = c.ap
	if in.task != nil {
		// The person who started the task may answer from the site or from
		// the panel — whichever comes first.
		ap = newDualApprover(ctx, c.ap, in.task)
	}
	a := agent.New(deps.cli, deps.registry, deps.sys, ap, st)
	a.MaxIterations = deps.maxIter
	var runner *subagent.Runner
	if c.srv.agents != nil {
		// Subagents inherit this turn's snapshot and its approver: a write
		// subagent asks in this chat (and, for a task from the site, in the
		// web chat too), marked with its title.
		runner = c.srv.agents.Runner(subagent.Deps{
			Client: deps.cli, Source: deps.source, Model: deps.modelID, Picker: deps.picker,
			Cwd: c.srv.cwd, Approver: ap, ParentChat: c.id, Notify: c.srv.subagentNotify,
		})
		ctx = tools.WithSubagentRunner(ctx, runner)
	}
	var attached []string
	if in.Context != nil {
		attached = in.Context.Files
	}
	newHist, err := a.RunWithFiles(ctx, hist, buildPrompt(in), attached)
	if in.task != nil && runner != nil {
		// A task from the site has no later turn to receive background
		// results: wait for them and let the agent finish with them.
		for err == nil && ctx.Err() == nil {
			note := runner.Drain(ctx)
			if note == "" {
				break
			}
			c.emit(Out{Type: "auto_user", Text: note})
			newHist, err = a.RunWithFiles(ctx, newHist, note, nil)
		}
	}
	if errors.Is(err, llm.ErrPaymentRequired) && deps.gate != nil {
		deps.gate.RefreshAsync() // the plan has most likely just ended
	}
	if err == nil && ctx.Err() == nil {
		newHist = c.maybeAutoCompact(ctx, newHist, deps, a.LastPromptTokens)
	}
	c.finishTurn(cancel, newHist, err, ctx, start, st, deps)
}

// maybeAutoCompact сжимает историю чата, когда контекст перевалил порог.
// В панели нет ручного /compact — без этого длинный чат в редакторе тихо
// упирался в потолок окна модели и умирал с ошибкой провайдера.
func (c *chatSession) maybeAutoCompact(ctx context.Context, hist []llm.AIMessage, deps turnDeps, promptTokens int) []llm.AIMessage {
	if deps.autoCompactPct == 0 || len(hist) <= compact.KeepTail+2 {
		return hist
	}
	pct := compact.Percent(promptTokens, hist, deps.modelID)
	if pct < deps.autoCompactPct {
		return hist
	}
	c.emit(Out{Type: "notice", Text: i18n.Tf("ui.compact.autoStart", pct)})
	cut := compact.Cut(hist, compact.KeepTail)
	summary, err := compact.Summarize(ctx, deps.cli, hist[1:cut])
	if err != nil {
		// Сжать не вышло — работаем с полной историей, но человеку говорим:
		// следующий ход может упереться в потолок контекста.
		c.emit(Out{Type: "notice", Text: "compact: " + err.Error()})
		return hist
	}
	out := compact.Rebuild(hist, cut, summary, cut-1)
	c.emit(Out{Type: "notice", Text: i18n.Tf("ui.compact.done", cut-1, len(summary))})
	return out
}

func (c *chatSession) finishTurn(cancel context.CancelFunc, newHist []llm.AIMessage, err error, ctx context.Context, start time.Time, st *streamer, deps turnDeps) {
	// Снять ДО cancel(): после него ctx.Err() non-nil всегда, и любая ошибка
	// провайдера (неверный ключ, лимиты, кончился контекст, сеть) уходила в
	// панель как «stopped» — молча, без текста. Пользователь видел просто
	// остановившийся ход.
	ctxErr := ctx.Err()
	stopped := ctxErr != nil
	cancel()
	if err != nil {
		if stopped {
			c.emit(Out{Type: "done", Text: "stopped", Elapsed: time.Since(start).Seconds()})
		} else {
			c.emit(Out{Type: "error", Text: err.Error()})
			c.emit(Out{Type: "done", Text: "error", Elapsed: time.Since(start).Seconds()})
		}
	} else {
		if st != nil && len(st.changedList()) > 0 {
			c.emit(Out{Type: "files_changed", Paths: st.changedList()})
		}
		c.emit(Out{Type: "done", Text: "ok", Elapsed: time.Since(start).Seconds()})
	}
	c.mu.Lock()
	task := c.task
	c.task = nil
	c.mu.Unlock()
	if task != nil {
		c.reportTask(task, st, err, ctxErr)
	}
	c.mu.Lock()
	c.turnCancel = nil
	// Историю не теряем: частичный ход полезнее пустоты.
	if newHist != nil {
		c.history = newHist
		c.persistLocked(deps.modelID)
	}
	c.busy = false
	c.live = nil
	c.pendingAsk = nil
	wake := c.wakeReq
	c.wakeReq = nil
	c.mu.Unlock()
	if wake != nil && err == nil {
		c.armWake(time.Now().Add(time.Duration(wake.DelaySeconds)*time.Second), wake.Reason, wake.PromptOnWake)
	}
	// Background subagents that finished during the turn: their results
	// come as the next, automatic turn.
	if err == nil && c.srv.agents != nil && c.srv.agents.HasFinished(c.id) {
		c.srv.subagentNotify(c.id)
	}
}

// reportTask sends the outcome of a task from the site back to the web chat:
// the answer text, or the text so far with the reason it stopped, or the error.
func (c *chatSession) reportTask(task *webTask, st *streamer, err, ctxErr error) {
	text := ""
	if st != nil {
		text = st.text()
	}
	switch {
	case errors.Is(ctxErr, context.DeadlineExceeded):
		err = errors.New(i18n.Tf("inbox.task.timeout", c.srv.taskTimeout))
	case ctxErr != nil:
		err = errors.New(i18n.T("inbox.task.stopped"))
	}
	chunk, data := inbox.Outcome(text, err)
	go task.finish(chunk, data)
}

// origin — who started the chat, for the panel.
func (c *chatSession) origin() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return originOf(c.sess.Origin)
}

// pendingTask — the task from the site the chat is running, if any.
func (c *chatSession) pendingTask() *webTask {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.task
}

func originOf(o string) string {
	if o == originWeb {
		return originWeb
	}
	return originUser
}

// armWake ставит таймер пробуждения и записывает его в сессию: закрытый
// редактор не отменяет обещание «проверю через пять минут».
func (c *chatSession) armWake(at time.Time, reason, prompt string) {
	c.mu.Lock()
	if c.wakeTimer != nil {
		c.wakeTimer.Stop()
	}
	c.sess.WakeAt = &at
	c.sess.WakeReason = reason
	c.sess.WakePrompt = prompt
	c.persistLocked("")
	d := time.Until(at)
	if d < 0 {
		d = 0
	}
	c.wakeTimer = time.AfterFunc(d, c.fireWake)
	c.mu.Unlock()
	c.srv.emit(Out{Type: "wakeup", Chat: c.id, Text: reason, Summary: prompt, At: at.Format(time.RFC3339)})
}

func (c *chatSession) cancelWake() bool {
	c.mu.Lock()
	had := c.sess.WakeAt != nil || c.wakeTimer != nil
	if c.wakeTimer != nil {
		c.wakeTimer.Stop()
		c.wakeTimer = nil
	}
	c.sess.WakeAt, c.sess.WakeReason, c.sess.WakePrompt = nil, "", ""
	c.persistLocked("")
	c.mu.Unlock()
	if had {
		c.srv.emit(Out{Type: "wakeup_cancelled", Chat: c.id})
	}
	return had
}

// fireWake — таймер сработал: ход запускается через главный цикл, как
// обычное сообщение, чтобы снимок модели/источника брался там же, где его
// меняют. Занятый чат переспрашивается позже, а не теряет пробуждение.
func (c *chatSession) fireWake() {
	c.mu.Lock()
	if c.busy {
		c.wakeTimer = time.AfterFunc(wakeRetry, c.fireWake)
		c.mu.Unlock()
		return
	}
	prompt := c.sess.WakePrompt
	if strings.TrimSpace(prompt) == "" {
		prompt = "продолжай"
	}
	if c.sess.WakeReason != "" {
		prompt = prompt + "\n\n[пробуждение по таймеру: " + c.sess.WakeReason + "]"
	}
	c.wakeTimer = nil
	c.sess.WakeAt, c.sess.WakeReason, c.sess.WakePrompt = nil, "", ""
	c.persistLocked("")
	c.mu.Unlock()
	c.srv.userQ <- In{Type: "user", Text: prompt, Chat: c.id, auto: true}
}

func (c *chatSession) stop() {
	c.mu.Lock()
	cancel := c.turnCancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	// Stopping a chat stops its subagents, background ones too.
	if c.srv.agents != nil {
		c.srv.agents.CancelParent(c.id)
	}
}

func (c *chatSession) isBusy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.busy
}

func (c *chatSession) wakeAt() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sess.WakeAt == nil {
		return ""
	}
	return c.sess.WakeAt.Format(time.RFC3339)
}

// Run — главный цикл. Возвращается на EOF stdin (редактор закрыл процесс).
func Run(ctx context.Context, cfg *config.Config, opts Options) error {
	if opts.Cwd == "" {
		opts.Cwd, _ = os.Getwd()
	}
	if err := os.Chdir(opts.Cwd); err != nil {
		return fmt.Errorf("%s: %w", i18n.Tf("ide.err.chdir", opts.Cwd), err)
	}

	cr, err := config.LoadCredentials()
	// ВАЖНО: при отсутствии файла LoadCredentials возвращает (nil, nil) —
	// проверки одной ошибки мало, и панику ловил бы КАЖДЫЙ новый человек,
	// поставивший плагин и ещё не вошедший.
	if err != nil || cr == nil {
		// Без токена ExecAI можно жить на подписках; llmpick разберётся.
		cr = &config.Credentials{}
	}

	protoIn, protoOut := opts.In, opts.Out
	if protoIn == nil {
		protoIn = os.Stdin
	}
	if protoOut == nil {
		protoOut = os.Stdout
	}
	s := newServer(json.NewEncoder(protoOut), opts.Cwd)
	s.desktop = newDesktop(cfg)

	// Plan gate: each turn on the ExecAI models asks it; billing is refreshed
	// in the background only while the ExecAI models are the active source.
	i18n.Setup(cfg.Locale)
	gate := plan.New(cfg.APIBase)
	gate.Start(ctx, plan.OnExecAI)

	// Tasks from the site: this window may own the machine's inbox (see
	// inbox.go). The lock is given back on every way out — stdin closed,
	// SIGTERM, an error — or the daemon could never take over.
	ctl := newInboxCtl(s, cfg.APIBase, opts.Cwd, gate, opts.NoInbox || inboxOff())
	ctl.setCreds(cr)
	defer ctl.shutdown()
	inboxCtx, stopInbox := context.WithCancel(ctx)
	defer stopInbox()

	// Модели — cache-first, как TUI: редактор должен подняться и без сети.
	res := llm.FetchModelsCached(cfg.APIBase, cr.Token)
	current := llm.PickDefault(res.Models, cfg.SelectedModelID)
	if current == nil {
		s.emit(Out{Type: "error", Text: i18n.T("ide.err.noModel")})
		return errors.New(i18n.T("ide.err.noModelShort"))
	}
	subs, subsErr := subscriptions.Load()

	// Модели зависят от ИСТОЧНИКА — те же правила, что у /model в TUI
	// (см. applySubscriptionSource и «source switch инварианты»): на подписке
	// каталог ExecAI не действует, у неё свой список.
	sourceModels := func() []NamedItem {
		active := subs.ActiveSubscription()
		if active == nil {
			items := make([]NamedItem, 0, len(res.Models))
			for _, m := range res.Models {
				items = append(items, NamedItem{ID: m.ID, Label: m.Name})
			}
			return items
		}
		// Каталог строит общий пакет: панель показывает те же имена и тот же
		// порядок, что и терминал. Раньше здесь отдавались сырые ID с сервера
		// без имён — отсюда «странный список моделей» в плагине.
		ms := catalog.For(*active)
		items := make([]NamedItem, 0, len(ms))
		for _, m := range ms {
			// У claude-cli пустой ID — легальный «Default из конфига claude»:
			// без подписи в статусе была пустота (поймано на скрине владельца).
			items = append(items, NamedItem{ID: m.ID, Label: m.Name})
		}
		return items
	}
	modelLabel := func(id string) string {
		for _, it := range sourceModels() {
			if it.ID == id {
				if it.Label != "" {
					return it.Label
				}
				return it.ID
			}
		}
		return id
	}

	// currentID — выбранная модель. На смене источника выбор пересобирается:
	// чужой id в чужом каталоге — прямой путь к 401 (инвариант из TUI).
	currentID := current.ID
	providerFor := func(id string) string {
		for _, m := range res.Models {
			if m.ID == id {
				return m.Provider
			}
		}
		return ""
	}
	reconcileModel := func() {
		// A subscription whose list was never fetched (the provider was
		// unreachable at connect, or the store predates live lists): ask now,
		// otherwise the panel has nothing to offer.
		if a := subs.ActiveSubscription(); a != nil && len(a.AvailableModels) == 0 &&
			(a.APIKey != "" || !subsconnect.NeedsKey(*a)) {
			sub := *a
			if subsconnect.Fill(&sub) == 200 {
				subs.Subscriptions[sub.Provider] = sub
				_ = subs.Save()
			}
		}
		// The same rule as execai serve and the terminal (llmpick.Model): the
		// selected model when the source offers it, else the source's primary
		// one, and none at all rather than a model of the previous source
		// (source switch invariant 2).
		id, _ := llmpick.Model(cfg.SelectedModelID, subs, res.Models)
		currentID = id
		if cfg.SelectedModelID != id {
			cfg.SetAutoModel(id) // picked here, not by the person
			_ = config.Save(cfg)
		}
	}
	reconcileModel()
	cli := llmpick.Client(cfg, subs, currentID, providerFor(currentID), cr.Token)

	// state — снимок для пикеров плагина: модели ТЕКУЩЕГО источника и все
	// источники, с пометкой активного.
	stateOut := func() Out {
		o := Out{Type: "state", Model: modelLabel(currentID), Source: subs.SourceLabel(),
			User: cr.Email, Effort: effortName(cfg.ThinkingBudget), MaxIter: cfg.GetMaxIterations(),
			SubagentIter: tools.SubagentDefault(),
			Security:     security.Current().String(), Securities: securityOptions(),
			Efforts: EffortOptions(cfg.ThinkingBudget), Connectable: ConnectableOptions()}
		for _, it := range sourceModels() {
			it.Active = it.ID == currentID
			o.Models = append(o.Models, it)
		}
		ctl.decorate(&o)
		o.Sources = append(o.Sources, NamedItem{ID: "execai", Label: "ExecAI",
			Active: subs.ActiveSubscription() == nil})
		for _, sub := range subs.List() {
			// Label, not just the id: the panel shows this in a picker, and
			// "kimi" there said nothing about which of the two Kimi paths it is.
			o.Sources = append(o.Sources, NamedItem{
				ID:     sub.Provider,
				Label:  subscriptions.DisplayName(sub),
				Active: subs.ActiveSubscription() != nil && subs.ActiveSubscription().Provider == sub.Provider})
		}
		return o
	}

	// machineOut — the machine_state event: the card and the tutorial from
	// what this process holds (credentials, sources, the plan answer) and
	// this machine's files; the folder bindings as the inbox last saw them.
	machineOut := func() Out {
		in := machine.Local(cfg.APIBase, cr, subs, gate.Current())
		in.Bindings = ctl.bindings()
		return machineStateOut(machine.Build(in))
	}
	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	go s.watchMachine(watchCtx)

	// Subagents: one manager per process; the registry goes to the panel
	// (agents) and to the server (the machine's card in the web panel).
	agentFleet := fleet.New()
	tools.SetSubagentDefault(cfg.SubagentIterations)
	stopAgentsFeed := s.attachAgents(subagent.NewManager(agentFleet))
	agentReport := fleet.StartReporter(agentFleet, fleet.HTTPSender(cfg.APIBase, fleet.SavedToken), opts.Cwd, 0)
	defer func() {
		s.agents.Shutdown(shutdownGrace)
		agentReport.Close()
		stopAgentsFeed()
		agentFleet.Close()
	}()

	registry := tools.Default(opts.Cwd)
	perms, _ := agent.LoadPermissions()

	// AskUser: в редакторе ЕСТЬ кому ответить — вопрос уезжает кнопками в
	// ТОТ чат, чей ход спросил (сессия лежит в контексте хода).
	tools.SetAskUserFunc(func(ctx context.Context, question string, aos []tools.AskOption) (string, error) {
		id, ch := s.newQuestion()
		out := Out{Type: "ask_user", ID: id, Question: question}
		for _, o := range aos {
			out.Options = append(out.Options, AskOption{Value: o.Label, Label: o.Label, Description: o.Description})
		}
		c := chatFrom(ctx)
		if c == nil {
			c = s.activeChat()
		}
		if c != nil {
			c.mu.Lock()
			cp := out
			cp.Chat = c.id
			c.pendingAsk = &cp
			c.mu.Unlock()
			c.emit(out)
		} else {
			s.emit(out)
		}
		defer func() {
			if c != nil {
				c.mu.Lock()
				if c.pendingAsk != nil && c.pendingAsk.ID == id {
					c.pendingAsk = nil
				}
				c.mu.Unlock()
			}
		}()
		select {
		case v := <-ch:
			return v, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})

	sys := agent.SystemPrompt(opts.Cwd, registry.Names(), agent.LoadMemory(opts.Cwd))
	// A task from the site runs without AskUser, as in execai serve: the
	// person may be at the phone, and a question only the panel can answer
	// would hang the task until its timeout. The model chooses between a
	// reasonable assumption and saying in the answer what was missing.
	webRegistry := tools.Default(opts.Cwd)
	webRegistry.Unregister("AskUser")
	webSys := agent.SystemPrompt(opts.Cwd, webRegistry.Names(), agent.LoadMemory(opts.Cwd))
	if notice := res.OfflineNotice(); notice != "" {
		s.emit(Out{Type: "notice", Text: notice})
	}
	s.emit(Out{
		Type: "ready", Version: version.Get(), Protocol: Protocol, Features: readyFeatures(cfg),
		Model: modelLabel(currentID), Source: subs.SourceLabel(), Cwd: opts.Cwd,
	})
	s.emit(agentsOut(nil))
	// What sync says goes to the panel as notices (notices.go): from now
	// on, held until the panel's first command or for a few seconds.
	noticeCtx, stopNotices := context.WithCancel(ctx)
	defer stopNotices()
	go opts.SyncNotices.pump(noticeCtx, s.emit)
	releaseNotices := time.AfterFunc(syncNoticeDelay, opts.SyncNotices.release)
	defer releaseNotices.Stop()

	// Снимок зависимостей хода — берётся в главном цикле, где их и меняют.
	deps := func() turnDeps {
		maxIter := opts.MaxIterations
		if maxIter <= 0 {
			maxIter = cfg.GetMaxIterations()
		}
		return turnDeps{cli: cli, sys: sys, registry: registry, maxIter: maxIter,
			modelID: currentID, provider: providerFor(currentID),
			autoCompactPct: cfg.GetAutoCompactPercent(),
			gate:           gate, planRoute: plan.RouteOf(subs),
			source: subs.SourceLabel(), picker: llmpick.NewPicker(cfg, subs, res.Models, cr.Token)}
	}
	_ = perms

	// Читатель stdin: команды в очередь главного цикла, ответы и стоп —
	// напрямую: они должны доходить, пока ходы идут.
	go func() {
		sc := bufio.NewScanner(protoIn)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<20) // выделение может быть большим
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var in In
			if err := json.Unmarshal([]byte(line), &in); err != nil {
				s.emit(Out{Type: "error", Text: i18n.Tf("ide.err.badMessage", err.Error())})
				continue
			}
			switch in.Type {
			case "answer":
				s.deliver(in.ID, in.Value)
			case "stop":
				var c *chatSession
				if in.Chat != "" {
					c = s.getChat(in.Chat)
				} else {
					c = s.activeChat()
				}
				if c != nil {
					c.stop()
				}
			case "ping":
				s.emit(Out{Type: "pong"})
			default:
				s.userQ <- in
			}
		}
		// Not close(): the inbox and wake-up timers also send here, and a
		// send on a closed channel panics.
		s.userQ <- In{Type: "eof"}
	}()

	go ctl.refresh(inboxCtx, refreshOpts{binding: true, force: true, ask: inbox.Background()})
	go ctl.watch(inboxCtx)

	// Пробуждения, пережившие закрытие редактора: чаты этого проекта с
	// wake_at поднимаются в память и получают таймер; просроченные — срабатывают
	// сразу. Человек закрыл Studio — обещание «проверю через пять минут»
	// никуда не делось.
	if list, err := sessions.List(); err == nil {
		for _, meta := range list {
			if meta.CWD != opts.Cwd || meta.WakeAt == nil {
				continue
			}
			if c, err := s.loadChat(meta.ID); err == nil {
				c.armWake(*meta.WakeAt, meta.WakeReason, meta.WakePrompt)
			}
		}
	}

	// Живой список моделей стареет: план могли повысить, вендор — выкатить
	// новую модель. Терминал перезапрашивает его сам, панель раньше нет —
	// после подключения список не обновлялся никогда. Обновляем в фоне: фетч
	// идёт на своей копии стора, а применяет результат основной цикл.
	refreshModels := func(provider string) {
		go func() {
			if subsconnect.RefreshModels(provider) {
				s.userQ <- In{Type: "command", Name: "models_refreshed"}
			}
		}()
	}
	if a := subs.ActiveSubscription(); a != nil && subsconnect.WantsLiveModels(a.Provider) && subsconnect.Stale(*a) {
		refreshModels(a.Provider)
	}

	// target — чат, к которому относится команда: явный chat или активный.
	target := func(in In) *chatSession {
		if in.Chat != "" {
			c, err := s.loadChat(in.Chat)
			if err != nil {
				s.emit(Out{Type: "error", Chat: in.Chat, Text: i18n.Tf("ide.err.noChat", err.Error())})
				return nil
			}
			return c
		}
		return s.activeChat()
	}
	// open делает чат активным и отдаёт панели его историю и идущий ход.
	open := func(c *chatSession) {
		s.setActive(c)
		c.emit(c.loadedOut())
		c.replayLive()
	}

loop:
	for {
		var in In
		select {
		case in = <-s.userQ:
		case <-ctx.Done():
			break loop // SIGTERM: give the inbox back and leave
		}
		switch in.Type {
		case "eof":
			break loop
		case "subagent_done":
			s.subagentDone(in, deps())
			continue
		case "machine_changed":
			s.emit(machineOut())
			continue
		case "web_task":
			// The machine was called from the web or the phone: step 5 of
			// the getting-started list is done from now on.
			if machine.MarkWebCall() {
				s.emit(machineOut())
			}
			// A task from the site: a new chat, not made active — the person
			// may be typing in another one; the panel shows a tab and a toast.
			c := s.newChat(currentID, providerFor(currentID))
			c.mu.Lock()
			c.sess.Origin = originWeb
			c.mu.Unlock()
			d := deps()
			d.registry, d.sys = webRegistry, webSys
			// The tab first (the updated chat list), then the turn's events.
			c.startTurnThen(in, d, func() { s.emit(s.chatsOut(s.activeChat())) })
			continue
		case "new_chat":
			// Сессионные разрешения живут в чате: новый чат — новые.
			c := s.newChat(currentID, providerFor(currentID))
			s.setActive(c)
			c.emit(Out{Type: "chat_reset"})
			opts.SyncNotices.release()
			continue
		case "command":
			switch in.Name {
			case "state":
				if subsErr != nil {
					// Молчать нельзя: человек увидит «источник ExecAI» и решит, что его
					// подписки исчезли, а файл просто не разобрался.
					s.emit(Out{Type: "notice", Text: i18n.Tf("ide.subs.unreadable", subsErr.Error())})
				}
				s.emit(stateOut())
			case "set_model":
				ok := false
				for _, it := range sourceModels() {
					if it.ID == in.Value {
						ok = true
						break
					}
				}
				if !ok {
					s.emit(Out{Type: "error", Text: i18n.Tf("ide.err.noSuchModel", in.Value)})
					break
				}
				currentID = in.Value
				cli = llmpick.Client(cfg, subs, currentID, providerFor(currentID), cr.Token)
				// Выбор персистится, как в TUI: перезапуск плагина не должен
				// молча возвращать старую модель.
				cfg.ChooseModel(currentID)
				_ = config.Save(cfg)
				s.emit(stateOut())
			case "set_source":
				if err := subs.Activate(in.Value); err != nil {
					s.emit(Out{Type: "error", Text: err.Error()})
					break
				}
				_ = subs.Save()
				// Инвариант смены источника: модель пересобирается под новый
				// каталог, клиент пересоздаётся.
				reconcileModel()
				if a := subs.ActiveSubscription(); a != nil && subsconnect.WantsLiveModels(a.Provider) && subsconnect.Stale(*a) {
					refreshModels(a.Provider)
				}
				cli = llmpick.Client(cfg, subs, currentID, providerFor(currentID), cr.Token)
				s.emit(stateOut())
			case "set_security":
				lvl := security.Parse(in.Value)
				if lvl.String() != strings.ToLower(strings.TrimSpace(in.Value)) {
					s.emit(Out{Type: "error", Text: i18n.Tf("ide.err.unknownLevel", in.Value)})
					break
				}
				security.Set(lvl)
				cfg.SecurityLevel = lvl.String()
				if err := config.Save(cfg); err != nil {
					s.emit(Out{Type: "error", Text: err.Error()})
					break
				}
				s.emit(Out{Type: "notice", Text: i18n.Tf("ide.security.set", lvl.Title())})
				s.emit(stateOut())
			case "set_effort":
				msg, err := setEffort(cfg, in.Value)
				if err != nil {
					s.emit(Out{Type: "error", Text: err.Error()})
					break
				}
				// Бюджет размышлений зашит в клиента — пересоздаём, иначе
				// настройка «применилась» бы только со следующего запуска.
				cli = llmpick.Client(cfg, subs, currentID, providerFor(currentID), cr.Token)
				s.emit(Out{Type: "notice", Text: msg})
				s.emit(stateOut())
			case "set_subagent_iterations":
				msg, err := setSubagentIterations(cfg, in.Value)
				if err != nil {
					s.emit(Out{Type: "error", Text: err.Error()})
					break
				}
				s.emit(Out{Type: "notice", Text: msg})
				s.emit(stateOut())
			case "set_max_iterations":
				msg, err := setMaxIterations(cfg, in.Value)
				if err != nil {
					s.emit(Out{Type: "error", Text: err.Error()})
					break
				}
				s.emit(Out{Type: "notice", Text: msg})
				s.emit(stateOut())
			case "login":
				newCr, err := login(ctx, cfg, s.emit)
				if err != nil {
					s.emit(Out{Type: "error", Text: err.Error()})
					break
				}
				cr = newCr
				ctl.setCreds(cr)
				go ctl.refresh(inboxCtx, refreshOpts{binding: true, force: true, ask: true})
				// После входа список моделей ExecAI становится доступен —
				// перечитываем каталог и пересобираем клиента.
				res = llm.FetchModelsCached(cfg.APIBase, cr.Token)
				reconcileModel()
				cli = llmpick.Client(cfg, subs, currentID, providerFor(currentID), cr.Token)
				s.emit(Out{Type: "login_done", Text: cr.Email})
				s.emit(stateOut())
				s.emit(machineOut())
			case "logout":
				if err := config.DeleteCredentials(); err != nil {
					s.emit(Out{Type: "error", Text: err.Error()})
					break
				}
				cr = &config.Credentials{}
				ctl.setCreds(cr)
				go ctl.refresh(inboxCtx, refreshOpts{binding: true, force: true, ask: true})
				s.emit(Out{Type: "notice", Text: i18n.T("ide.logout.done")})
				s.emit(stateOut())
				s.emit(machineOut())
			case "connect":
				msg, err := connect(subs, in)
				if err != nil {
					s.emit(Out{Type: "error", Text: err.Error()})
					break
				}
				reconcileModel()
				cli = llmpick.Client(cfg, subs, currentID, providerFor(currentID), cr.Token)
				s.emit(Out{Type: "notice", Text: msg})
				s.emit(stateOut())
				s.emit(machineOut())
			case "disconnect":
				subs.Remove(in.Value)
				if err := subs.Save(); err != nil {
					s.emit(Out{Type: "error", Text: err.Error()})
					break
				}
				reconcileModel()
				cli = llmpick.Client(cfg, subs, currentID, providerFor(currentID), cr.Token)
				s.emit(Out{Type: "notice", Text: i18n.Tf("ide.disconnect.done", in.Value)})
				s.emit(stateOut())
				s.emit(machineOut())
			case "models_refreshed":
				// Фоновое обновление уже записало список на диск — перечитываем
				// и показываем панели свежий каталог.
				if st, err := subscriptions.Load(); err == nil && st != nil {
					subs = st
				}
				s.emit(stateOut())
			case "machine_state":
				s.emit(machineOut())
			case "notify":
				s.desktopNotify(in)
			case "list_chats":
				s.emit(s.chatsOut(s.activeChat()))
			case "list_agents", "open_agent", "stop_agent":
				s.agentCommand(in)
			case "inbox_take":
				go ctl.take(inboxCtx)
			case "bind":
				go ctl.bind(inboxCtx, strings.TrimSpace(firstNonEmpty(in.WorkspaceID, in.Value)))
			case "unbind":
				go ctl.unbind(inboxCtx)
			case "list_workspaces":
				go ctl.listWorkspaces(inboxCtx)
			case "resume_last":
				// Продолжить последний чат ЭТОГО проекта.
				//
				// В терминале для этого есть /resume, а панель открывалась
				// каждый раз с чистого листа: чат никуда не девался, но
				// человек должен был искать его в истории. Редактор — место,
				// куда возвращаются, и разговор обязан продолжаться сам.
				list, _ := sessions.List()
				active := s.activeChat()
				var last *sessions.Session
				for i := range list {
					if list[i].CWD != opts.Cwd {
						continue
					}
					if active != nil && list[i].ID == active.id {
						break // уже открыт — восстанавливать нечего
					}
					last = list[i]
					break // List отсортирован по времени изменения
				}
				if last == nil {
					if active == nil {
						// The first chat of the project is opened now, not
						// when the first message comes. A chat made for a
						// message is announced with chat_reset after the
						// panel has already drawn that message — the reset
						// clears the feed, and the person's first message
						// vanished from the screen. With a chat in place the
						// panel addresses the message to it and nothing is
						// reset under it.
						c := s.newChat(currentID, providerFor(currentID))
						s.setActive(c)
						c.emit(Out{Type: "chat_reset"})
					}
					s.emit(Out{Type: "notice", Text: i18n.T("ide.resume.none")})
					break
				}
				c, err := s.loadChat(last.ID)
				if err != nil {
					s.emit(Out{Type: "error", Text: i18n.Tf("ide.resume.failed", err.Error())})
					break
				}
				open(c)
			case "load_chat":
				c, err := s.loadChat(in.Value)
				if err != nil {
					s.emit(Out{Type: "error", Text: i18n.Tf("ide.chat.openFailed", err.Error())})
					break
				}
				open(c)
			case "cancel_wakeup":
				c := target(In{Chat: firstNonEmpty(in.Value, in.Chat)})
				if c == nil {
					break
				}
				if !c.cancelWake() {
					s.emit(Out{Type: "notice", Chat: c.id, Text: i18n.T("ide.wake.none")})
				}
			case "wake_now":
				c := target(In{Chat: firstNonEmpty(in.Value, in.Chat)})
				if c == nil {
					break
				}
				c.mu.Lock()
				has := c.sess.WakeAt != nil
				c.mu.Unlock()
				if !has {
					s.emit(Out{Type: "notice", Chat: c.id, Text: i18n.T("ide.wake.none")})
					break
				}
				c.fireWake()
			default:
				s.emit(Out{Type: "error", Text: i18n.Tf("ide.err.unknownCommand", in.Name)})
			}
			if in.Name == "resume_last" || in.Name == "load_chat" {
				// The panel has its chat: the sync lines said meanwhile
				// may follow (notices.go).
				opts.SyncNotices.release()
			}
			continue
		case "user":
			// ок, ниже
		default:
			s.emit(Out{Type: "error", Text: i18n.Tf("ide.err.unknownType", in.Type)})
			continue
		}

		c := target(in)
		if c == nil {
			if in.Chat != "" {
				continue
			}
			c = s.newChat(currentID, providerFor(currentID))
			s.setActive(c)
			c.emit(Out{Type: "chat_reset"})
		}
		if !c.startTurn(in, deps()) {
			if in.auto {
				// Пробуждение пришло в занятый чат — вернуть позже, не терять.
				c.armWake(time.Now().Add(wakeRetry), "", in.Text)
				continue
			}
			c.emit(Out{Type: "error", Text: i18n.T("ide.chat.busy")})
		}
	}
	// Редактор закрыл канал: ходы в полёте отменяем — отвечать некому.
	// The inbox goes first, so that execai serve can take over at once;
	// tasks from the site in flight are answered before the process exits
	// (the editor kills it shortly after closing stdin).
	stopInbox()
	ctl.shutdown()
	s.chatsMu.Lock()
	all := make([]*chatSession, 0, len(s.chats))
	for _, c := range s.chats {
		all = append(all, c)
	}
	s.chatsMu.Unlock()
	var pending []*webTask
	for _, c := range all {
		if t := c.pendingTask(); t != nil {
			pending = append(pending, t)
			go t.finish(inbox.ChunkError, i18n.T("inbox.task.editorClosed"))
		}
		c.stop()
	}
	// Tasks taken from the inbox that never reached a chat.
	for drained := false; !drained; {
		select {
		case in := <-s.userQ:
			if in.task != nil {
				pending = append(pending, in.task)
				go in.task.finish(inbox.ChunkError, i18n.T("inbox.task.editorClosed"))
			}
		default:
			drained = true
		}
	}
	deadline := time.After(time.Second)
	for _, t := range pending {
		select {
		case <-t.done:
		case <-deadline:
		}
	}
	return nil
}

// chatsOut — the chats of this folder for the panel's tab list: saved ones
// (up to 30, newest first) plus those not saved yet whose first turn is still
// running, so the person can always come back to a chat that is answering.
func (s *server) chatsOut(active *chatSession) Out {
	list, _ := sessions.List()
	o := Out{Type: "chats"}
	for _, c := range list {
		// Only this folder's chats: the editor's history is per project.
		if c.CWD != s.cwd {
			continue
		}
		// An empty title goes as is: labelling an untitled chat is the
		// panel's job, it speaks the person's language.
		item := NamedItem{ID: c.ID, Label: c.Title, Active: active != nil && c.ID == active.id,
			Origin: originOf(c.Origin)}
		if live := s.getChat(c.ID); live != nil {
			item.Busy = live.isBusy()
			item.Wake = live.wakeAt()
		} else if c.WakeAt != nil {
			item.Wake = c.WakeAt.Format(time.RFC3339)
		}
		o.Chats = append(o.Chats, item)
		if len(o.Chats) >= 30 {
			break
		}
	}
	seen := map[string]bool{}
	for _, it := range o.Chats {
		seen[it.ID] = true
	}
	s.chatsMu.Lock()
	for id, c := range s.chats {
		if seen[id] {
			continue
		}
		if !c.isBusy() && c != active {
			continue
		}
		c.mu.Lock()
		label := c.sess.Title
		if label == "" {
			label = shortTitle(c.firstText)
		}
		origin := originOf(c.sess.Origin)
		c.mu.Unlock()
		item := NamedItem{ID: id, Label: label, Active: c == active, Busy: c.isBusy(), Wake: c.wakeAt(),
			Origin: origin}
		o.Chats = append([]NamedItem{item}, o.Chats...)
	}
	s.chatsMu.Unlock()
	return o
}

// shortTitle — подпись вкладки из первой реплики, пока у чата нет заголовка.
func shortTitle(text string) string {
	t := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	r := []rune(t)
	if len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return t
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// buildPrompt приклеивает контекст редактора к сообщению так, чтобы модель
// отличала «что сказал человек» от «где он стоит».
func buildPrompt(in In) string {
	if in.Context == nil || (in.Context.Path == "" && in.Context.Selection == "" && len(in.Context.Files) == 0) {
		return in.Text
	}
	var b strings.Builder
	b.WriteString(in.Text)
	b.WriteString("\n\n" + ctxHead)
	if in.Context.Path != "" {
		b.WriteString(": файл " + in.Context.Path)
	}
	if in.Context.Language != "" {
		b.WriteString(" (" + in.Context.Language + ")")
	}
	b.WriteString("]")
	if in.Context.Selection != "" {
		b.WriteString("\nВыделенный фрагмент:\n```\n" + in.Context.Selection + "\n```")
	}
	if len(in.Context.Files) > 0 {
		// Картинки — В ТЕКСТ в кавычках: BuildUserContent найдёт их и
		// приложит vision-блоками (тот же механизм, что Ctrl+V в TUI).
		// Остальные — списком путей, содержимое агент читает сам.
		var imgs, files []string
		for _, f := range in.Context.Files {
			switch strings.ToLower(filepath.Ext(f)) {
			case ".png", ".jpg", ".jpeg", ".gif", ".webp":
				imgs = append(imgs, `"`+f+`"`)
			default:
				files = append(files, f)
			}
		}
		if len(imgs) > 0 {
			b.WriteString(ctxImages + strings.Join(imgs, " "))
		}
		if len(files) > 0 {
			b.WriteString(ctxFiles + strings.Join(files, ", "))
		}
	}
	return b.String()
}

// The parts of the block buildPrompt appends. promptParts takes a stored
// prompt apart by the same words, so the two cannot drift.
const (
	ctxHead   = "[контекст редактора"
	ctxImages = "\nПриложенные изображения: "
	ctxFiles  = "\nПриложенные файлы (прочитай нужные инструментом Read): "
)

var ctxQuoted = regexp.MustCompile(`"([^"\n]+)"`)

// promptParts takes a prompt built by buildPrompt apart again, as the chat
// history keeps it: what the person typed and the files that went with it.
// Pictures the model was given are cut out of the stored text (the history
// keeps them as content blocks — see imageBlocks); a picture that could not
// be read stays in the text by its path and is returned among the files.
// ok is false for a message without the block: it is shown as it is.
func promptParts(full string) (said string, files []string, ok bool) {
	// llm.ContentText marks every picture after the text.
	body := strings.TrimSpace(full)
	for strings.HasSuffix(body, "[image]") {
		body = strings.TrimSpace(strings.TrimSuffix(body, "[image]"))
	}
	var tail string
	if i := strings.LastIndex(body, "\n\n"+ctxHead); i >= 0 {
		said, tail = body[:i], body[i+2+len(ctxHead):]
	} else if strings.HasPrefix(body, ctxHead) {
		// Nothing typed: the text was trimmed down to the block itself.
		tail = body[len(ctxHead):]
	} else {
		return "", nil, false
	}
	// The header is one line and closes with a bracket: anything else is the
	// person's own text that happens to hold these words.
	if head, _, _ := strings.Cut(tail, "\n"); !strings.HasSuffix(strings.TrimSpace(head), "]") {
		return "", nil, false
	}
	// A selected fragment comes fenced before the files and may hold any
	// words, these included: the files are looked for after its last fence.
	if i := strings.LastIndex(tail, "\n```"); i >= 0 {
		tail = tail[i:]
	}
	if i := strings.Index(tail, ctxImages); i >= 0 {
		line, _, _ := strings.Cut(tail[i+len(ctxImages):], "\n")
		for _, m := range ctxQuoted.FindAllStringSubmatch(line, -1) {
			files = append(files, m[1])
		}
	}
	if i := strings.Index(tail, ctxFiles); i >= 0 {
		line, _, _ := strings.Cut(tail[i+len(ctxFiles):], "\n")
		for _, f := range strings.Split(line, ", ") {
			if f = strings.TrimSpace(f); f != "" {
				files = append(files, f)
			}
		}
	}
	return said, files, true
}

// imageBlocks counts the pictures in the content of a message, in both shapes
// a history holds it: built in this run, or read back from disk.
func imageBlocks(content interface{}) int {
	n := 0
	switch v := content.(type) {
	case []llm.ContentBlock:
		for _, b := range v {
			if b.Type == "image_url" {
				n++
			}
		}
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok && m["type"] == "image_url" {
				n++
			}
		}
	}
	return n
}

// streamer транслирует события цикла в протокол.
//
// Parallel tool calls (several Task subagents) start and finish out of
// order, so cards are tracked per call key (agent.ToolCallStreamer); the
// key-less OnToolCall/OnToolResult (a delegated CLI) use the current card.
type streamer struct {
	c *chatSession
	// answer — the turn's text, for the result of a task from the site.
	answerMu  sync.Mutex
	answer    strings.Builder
	turnStart time.Time

	mu      sync.Mutex
	toolSeq int
	curID   string
	curTool string
	curPath string // путь из аргументов текущего Write/Edit
	calls   map[string]toolCard
	changed map[string]bool
}

// toolCard — a card in flight: its panel id and the file a Write/Edit names.
type toolCard struct {
	id   string
	path string
}

func (st *streamer) OnText(delta string) {
	st.answerMu.Lock()
	st.answer.WriteString(delta)
	st.answerMu.Unlock()
	st.c.emit(Out{Type: "text_delta", Text: delta})
}

// text — everything the model said in this turn.
func (st *streamer) text() string {
	st.answerMu.Lock()
	defer st.answerMu.Unlock()
	return st.answer.String()
}

func (st *streamer) OnReasoning(delta string) { st.c.emit(Out{Type: "reasoning_delta", Text: delta}) }

func (st *streamer) OnToolCall(name string, args json.RawMessage) { st.start("", name, args) }

func (st *streamer) OnToolResult(name string, result string, err error) {
	st.end("", name, result, err)
}

// OnToolStart/OnToolEnd — agent.ToolCallStreamer.
func (st *streamer) OnToolStart(key, name string, args json.RawMessage) { st.start(key, name, args) }

func (st *streamer) OnToolEnd(key, name, result string, err error) { st.end(key, name, result, err) }

func (st *streamer) start(key, name string, args json.RawMessage) {
	// Write/Edit меняют файлы — путь запоминаем, но в changed попадёт только
	// то, что РЕАЛЬНО легло на диск (см. end): отказ пользователя
	// возвращается без ошибки, и по одному ok файл от «не файла» не отличить.
	path := ""
	if name == "Write" || name == "Edit" {
		var a struct {
			Path string `json:"path"`
			File string `json:"file_path"`
		}
		if json.Unmarshal(args, &a) == nil {
			if a.Path != "" {
				path = a.Path
			} else if a.File != "" {
				path = a.File
			}
		}
	}
	st.mu.Lock()
	st.toolSeq++
	id := fmt.Sprintf("t%d", st.toolSeq)
	st.curID, st.curTool, st.curPath = id, name, path
	if key != "" {
		if st.calls == nil {
			st.calls = map[string]toolCard{}
		}
		st.calls[key] = toolCard{id: id, path: path}
	}
	st.mu.Unlock()
	st.c.emit(Out{Type: "tool_call", ID: id, CallID: key, Tool: name, Summary: agent.SummaryFor(name, args)})
}

func (st *streamer) OnToolChunk(name string, chunk string) {
	st.mu.Lock()
	id := st.curID
	st.mu.Unlock()
	st.c.emit(Out{Type: "tool_chunk", ID: id, Tool: name, Chunk: chunk})
}

func (st *streamer) end(key, name string, result string, err error) {
	ok := err == nil
	st.mu.Lock()
	id, path := st.curID, st.curPath
	if card, found := st.calls[key]; key != "" && found {
		id, path = card.id, card.path
		delete(st.calls, key)
	}
	if id == st.curID {
		st.curPath = ""
	}
	// Файл считается изменённым по ФАКТУ: существует и тронут в этом ходу.
	// Люфт 2с: файловые часы грубее time.Now() (tmpfs штампует тиком и может
	// отстать на миллисекунды, FAT — на 2 секунды), без люфта свежая запись
	// выглядела бы «старой».
	if ok && path != "" {
		if fi, statErr := os.Stat(path); statErr == nil && !fi.ModTime().Before(st.turnStart.Add(-2*time.Second)) {
			if st.changed == nil {
				st.changed = map[string]bool{}
			}
			st.changed[path] = true
		}
	}
	st.mu.Unlock()
	tail := result
	if err != nil {
		tail = err.Error()
	}
	const max = 2000
	if len([]rune(tail)) > max {
		r := []rune(tail)
		tail = "…" + string(r[len(r)-max:])
	}
	// A refusal comes back from the tool loop as an ordinary result — the
	// model has to read it — so ok alone cannot tell it from a call that
	// ran. The text is the one the loop puts there itself (agent.runTool,
	// approveDelegated), in this process's language.
	denied := err == nil && result == i18n.T("agent.tool.declined")
	st.c.emit(Out{Type: "tool_result", ID: id, Tool: name, OK: &ok, Tail: tail, Denied: denied})
}

func (st *streamer) changedList() []string {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make([]string, 0, len(st.changed))
	for p := range st.changed {
		out = append(out, p)
	}
	return out
}

// OnIterationStart — плагину полезно показывать номер итерации в статусе.
func (st *streamer) OnIterationStart(n int) { st.c.emit(Out{Type: "iteration", N: n}) }

// ideApprover — разрешения кнопками в редакторе. Значения и смысл — общий
// словарь с веб-каналом; «в сессии» = в этом разговоре (как в TUI), поэтому
// у каждого чата свой approver и новый чат начинает с чистого листа.
type ideApprover struct {
	c *chatSession
	// mu guards the session maps: the chat's agent and its write subagents
	// ask from different goroutines.
	mu           sync.Mutex
	sessionTools map[string]bool
	sessionExact map[string]bool
}

func (a *ideApprover) AskApprove(toolName string, args json.RawMessage, summary string) agent.ApproveDecision {
	return a.ask(context.Background(), nil, toolName, args, summary)
}

// ask puts one question on the panel — from the chat's agent (who nil) or a
// subagent — and waits for the answer or for ctx to end.
func (a *ideApprover) ask(ctx context.Context, who *agent.Asker, toolName string, args json.RawMessage, summary string) agent.ApproveDecision {
	key := agent.ExactKey(toolName, args)
	if a.allowed(toolName, key) {
		return agent.ApproveOnce
	}
	if !a.c.acquireAsk(ctx) {
		return agent.ApproveDeny
	}
	defer a.c.releaseAsk()
	if a.allowed(toolName, key) { // answered "session" while we waited
		return agent.ApproveOnce
	}
	id, ch, out := a.question(toolName, summary)
	a.show(withAgent(out, who))
	select {
	case answer := <-ch:
		a.clear(id)
		return a.decide(answer, toolName, key)
	case <-ctx.Done():
		a.cancelQuestion(id)
		return agent.ApproveDeny
	}
}

// decide applies an answer under the chat's lock.
func (a *ideApprover) decide(answer, toolName, key string) agent.ApproveDecision {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sessionTools == nil {
		a.sessionTools = map[string]bool{}
	}
	if a.sessionExact == nil {
		a.sessionExact = map[string]bool{}
	}
	return decide(answer, toolName, key, a.sessionTools, a.sessionExact)
}

// allowed — the call is already allowed in this chat ("session"/"exact").
func (a *ideApprover) allowed(toolName, key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessionTools[toolName] || a.sessionExact[key]
}

// question registers a permission question and builds its event.
func (a *ideApprover) question(toolName, summary string) (string, chan string, Out) {
	id, ch := a.c.srv.newQuestion()
	out := Out{
		Type: "ask", ID: id, Tool: toolName, Summary: summary,
		// Labels in the panel's language (the process locale, i18n.Setup in
		// Run); the panel answers with the values, never the labels.
		Options: []AskOption{
			{inbox.AnswerOnce, i18n.T("approve.once"), i18n.T("ide.approve.onceDesc")},
			{inbox.AnswerSession, i18n.Tf("approve.allToolSession", toolName), i18n.T("ide.approve.sessionDesc")},
			{inbox.AnswerExact, i18n.T("approve.thisCmdSession"), i18n.T("ide.approve.exactDesc")},
			// Формулировка нарочно не обещает «весь инструмент»: у чтения и
			// сети «навсегда» означает КАТАЛОГ или ДОМЕН (см. tools.Scoped),
			// и точный масштаб напечатан в summary выше. Кнопка, обещающая
			// больше, чем делает, — обман в самом неподходящем месте.
			{inbox.AnswerAlways, i18n.T("approve.forever"), i18n.T("ide.approve.foreverDesc")},
			{inbox.AnswerDeny, i18n.T("approve.deny"), i18n.T("ide.approve.denyDesc")},
		},
	}
	return id, ch, out
}

// show remembers the question until it is answered (coming back to the tab,
// the person sees the buttons again) and sends it to the panel.
func (a *ideApprover) show(out Out) {
	a.c.mu.Lock()
	cp := out
	cp.Chat = a.c.id
	a.c.pendingAsk = &cp
	a.c.mu.Unlock()
	a.c.emit(out)
}

// clear forgets the question once it is answered.
func (a *ideApprover) clear(id string) {
	a.c.mu.Lock()
	if a.c.pendingAsk != nil && a.c.pendingAsk.ID == id {
		a.c.pendingAsk = nil
	}
	a.c.mu.Unlock()
}

// decide turns an answer into a decision, remembering "session" (the whole
// tool) and "exact" (exactly this call) in the given scope.
func decide(answer, toolName, key string, tools, exact map[string]bool) agent.ApproveDecision {
	switch answer {
	case inbox.AnswerOnce:
		return agent.ApproveOnce
	case inbox.AnswerSession:
		tools[toolName] = true
		return agent.ApproveOnce
	case inbox.AnswerExact:
		exact[key] = true
		return agent.ApproveOnce
	case inbox.AnswerAlways:
		// Записывает цикл, а не интерфейс.
		//
		// Раньше здесь стоял AddTool(toolName) — и ответ «навсегда» на ОДИН
		// каталог выдавал право читать что угодно где угодно, включая ключи и
		// .env. Масштаб разрешения знает только цикл: у него есть tools.Scoped
		// (каталог, домен, отдельный секретный файл). Поймано самопрогоном
		// 15.08: после «навсегда» на каталог перестал спрашиваться .env.
		return agent.ApproveAlways
	default:
		// Молчание, обрыв и мусор — отказ.
		return agent.ApproveDeny
	}
}

// replayOut собирает событие восстановления чата для панели.
//
// Общая для «открыть чат из истории» и «продолжить последний»: два вида
// восстановления обязаны показывать одно и то же, иначе человек увидит
// разную переписку в зависимости от того, каким путём он в неё вернулся.
func replayOut(history []llm.AIMessage) Out {
	o := Out{Type: "chat_loaded"}
	for _, msg := range history {
		switch msg.Role {
		case "user":
			m := ReplayMsg{Role: "user", Text: llm.ContentText(msg.Content)}
			if said, files, ok := promptParts(m.Text); ok {
				m.Said, m.Files, m.Images = &said, files, imageBlocks(msg.Content)
			}
			o.Msgs = append(o.Msgs, m)
		case "assistant":
			if body := llm.ContentText(msg.Content); body != "" {
				o.Msgs = append(o.Msgs, ReplayMsg{Role: "assistant", Text: body})
			}
			for _, tc := range msg.ToolCalls {
				o.Msgs = append(o.Msgs, ReplayMsg{Role: "tool", Tool: tc.Function.Name,
					Text: agent.SummaryFor(tc.Function.Name, json.RawMessage(tc.Function.Arguments))})
			}
		}
	}
	return o
}
