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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/catalog"
	"github.com/velesbsdllc/agent-vbai/internal/compact"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	subsconnect "github.com/velesbsdllc/agent-vbai/internal/connect"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/llmpick"
	"github.com/velesbsdllc/agent-vbai/internal/security"
	"github.com/velesbsdllc/agent-vbai/internal/sessions"
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
	// list_chats | load_chat | cancel_wakeup | wake_now; value — аргумент
	Name string `json:"name,omitempty"`
	// connect: ключ провайдера и (опционально) свой base_url
	Key     string `json:"key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	// Chat — к какому чату относится сообщение (user, stop, load_chat,
	// cancel_wakeup, wake_now). Пусто — текущий (активный в панели).
	Chat string `json:"chat,omitempty"`

	// auto — ход запущен самим агентом (пробуждение), а не человеком.
	auto bool
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
	Model    string `json:"model,omitempty"`
	Source   string `json:"source,omitempty"`
	Cwd      string `json:"cwd,omitempty"`

	// text_delta / reasoning_delta / notice / error / auto_user / wakeup (причина)
	Text string `json:"text,omitempty"`

	// tool_call / tool_chunk / tool_result
	ID      string `json:"id,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Summary string `json:"summary,omitempty"`
	Chunk   string `json:"chunk,omitempty"`
	OK      *bool  `json:"ok,omitempty"`
	Tail    string `json:"tail,omitempty"`

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
	Security    string      `json:"security,omitempty"`   // уровень доверия
	Securities  []NamedItem `json:"securities,omitempty"` // варианты для пикера
	// chat_loaded
	Msgs []ReplayMsg `json:"msgs,omitempty"`
	// chat_loaded: ход в этом чате ещё идёт (панель получит и его поток)
	Busy bool `json:"busy,omitempty"`
	// wakeup / chat_loaded: когда чат проснётся (RFC 3339)
	At string `json:"at,omitempty"`
}

// ReplayMsg — одна реплика восстановленного чата (упрощённый вид для UI).
type ReplayMsg struct {
	Role string `json:"role"` // user | assistant | tool
	Tool string `json:"tool,omitempty"`
	Text string `json:"text"`
}

// NamedItem — пункт списка для пикеров плагина.
type NamedItem struct {
	ID     string `json:"id"`
	Label  string `json:"label,omitempty"`
	Active bool   `json:"active,omitempty"`
	// chats: ход идёт / запланировано пробуждение (RFC 3339)
	Busy bool   `json:"busy,omitempty"`
	Wake string `json:"wake,omitempty"`
}

// AskOption — вариант ответа. Слово в слово как в TUI и веб-чате: человек
// должен узнавать кнопки, а не переучиваться в каждом интерфейсе.
type AskOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Значения ответов на разрешение — общий словарь с веб-каналом
// (agents-vbai/internal/ask). Незнакомое значение = отказ.
const (
	answerOnce    = "once"
	answerSession = "session"
	answerExact   = "exact"
	answerAlways  = "always"
	answerDeny    = "deny"
)

// Options — параметры запуска.
type Options struct {
	Cwd           string
	MaxIterations int
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
}

func newServer(enc *json.Encoder, cwd string) *server {
	return &server{
		enc: enc, answers: map[string]chan string{},
		chats: map[string]*chatSession{},
		slots: make(chan struct{}, parallelTurns()),
		userQ: make(chan In, 64), cwd: cwd,
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
	c.srv.emit(Out{Type: "turn_start", Chat: c.id})
	for _, o := range live {
		c.srv.emit(o)
	}
	if ask != nil {
		c.srv.emit(*ask)
	}
}

// startTurn запускает ход в горутине. Возвращает false, если чат занят.
func (c *chatSession) startTurn(in In, deps turnDeps) bool {
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
	ctx, cancel := context.WithCancel(context.Background())
	c.turnCancel = cancel
	c.mu.Unlock()
	go c.runTurn(ctx, cancel, in, deps, hist)
	return true
}

func (c *chatSession) runTurn(ctx context.Context, cancel context.CancelFunc, in In, deps turnDeps, hist []llm.AIMessage) {
	// Слот: ждём честно и говорим об этом, а не молчим.
	select {
	case c.srv.slots <- struct{}{}:
	default:
		c.emit(Out{Type: "queued", Text: "ход в очереди: другие чаты ещё отвечают"})
		select {
		case c.srv.slots <- struct{}{}:
		case <-ctx.Done():
			c.finishTurn(cancel, nil, ctx.Err(), ctx, time.Now(), nil, deps)
			return
		}
	}
	defer func() { <-c.srv.slots }()

	start := time.Now()
	c.emit(Out{Type: "turn_start"})
	if in.auto {
		// Человек должен видеть, что ход начал агент сам и с каким промтом.
		c.emit(Out{Type: "auto_user", Text: in.Text})
	}
	ctx = withChat(ctx, c)
	ctx = tools.WithWakeupSink(ctx, func(r tools.ScheduleRequest) {
		c.mu.Lock()
		c.wakeReq = &r
		c.mu.Unlock()
	})
	st := &streamer{c: c, changed: map[string]bool{}, turnStart: start}
	a := agent.New(deps.cli, deps.registry, deps.sys, c.ap, st)
	a.MaxIterations = deps.maxIter
	var attached []string
	if in.Context != nil {
		attached = in.Context.Files
	}
	newHist, err := a.RunWithFiles(ctx, hist, buildPrompt(in), attached)
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
	stopped := ctx.Err() != nil
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
		return fmt.Errorf("не перейти в каталог %s: %w", opts.Cwd, err)
	}

	cr, err := config.LoadCredentials()
	// ВАЖНО: при отсутствии файла LoadCredentials возвращает (nil, nil) —
	// проверки одной ошибки мало, и панику ловил бы КАЖДЫЙ новый человек,
	// поставивший плагин и ещё не вошедший.
	if err != nil || cr == nil {
		// Без токена ExecAI можно жить на подписках; llmpick разберётся.
		cr = &config.Credentials{}
	}

	s := newServer(json.NewEncoder(os.Stdout), opts.Cwd)

	// Модели — cache-first, как TUI: редактор должен подняться и без сети.
	res := llm.FetchModelsCached(cfg.APIBase, cr.Token)
	current := llm.PickDefault(res.Models, cfg.SelectedModelID)
	if current == nil {
		s.emit(Out{Type: "error", Text: "не выбрать модель — проверь /model в execai"})
		return fmt.Errorf("нет модели")
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
		ms := catalog.For(active.Provider, active.AvailableModels)
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
		items := sourceModels()
		for _, it := range items {
			if it.ID == cfg.SelectedModelID {
				currentID = it.ID
				return
			}
		}
		if len(items) > 0 {
			currentID = items[0].ID
		}
		cfg.SelectedModelID = currentID
		_ = config.Save(cfg)
	}
	reconcileModel()
	cli := llmpick.Client(cfg, subs, currentID, providerFor(currentID), cr.Token)

	// state — снимок для пикеров плагина: модели ТЕКУЩЕГО источника и все
	// источники, с пометкой активного.
	stateOut := func() Out {
		o := Out{Type: "state", Model: modelLabel(currentID), Source: subs.SourceLabel(),
			User: cr.Email, Effort: effortName(cfg.ThinkingBudget), MaxIter: cfg.GetMaxIterations(),
			Security: security.Current().String(), Securities: securityOptions(),
			Efforts: EffortOptions(cfg.ThinkingBudget), Connectable: ConnectableOptions()}
		for _, it := range sourceModels() {
			it.Active = it.ID == currentID
			o.Models = append(o.Models, it)
		}
		o.Sources = append(o.Sources, NamedItem{ID: "execai", Label: "ExecAI",
			Active: subs.ActiveSubscription() == nil})
		for _, sub := range subs.List() {
			// Label, not just the id: the panel shows this in a picker, and
			// "kimi" there said nothing about which of the two Kimi paths it is.
			o.Sources = append(o.Sources, NamedItem{
				ID:     sub.Provider,
				Label:  subscriptions.ProviderName(sub.Provider),
				Active: subs.ActiveSubscription() != nil && subs.ActiveSubscription().Provider == sub.Provider})
		}
		return o
	}

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
	if notice := res.OfflineNotice(); notice != "" {
		s.emit(Out{Type: "notice", Text: notice})
	}
	s.emit(Out{
		Type: "ready", Version: version.Get(), Protocol: Protocol,
		Model: modelLabel(currentID), Source: subs.SourceLabel(), Cwd: opts.Cwd,
	})

	// Снимок зависимостей хода — берётся в главном цикле, где их и меняют.
	deps := func() turnDeps {
		maxIter := opts.MaxIterations
		if maxIter <= 0 {
			maxIter = cfg.GetMaxIterations()
		}
		return turnDeps{cli: cli, sys: sys, registry: registry, maxIter: maxIter,
			modelID: currentID, provider: providerFor(currentID),
			autoCompactPct: cfg.GetAutoCompactPercent()}
	}
	_ = perms

	// Читатель stdin: команды в очередь главного цикла, ответы и стоп —
	// напрямую: они должны доходить, пока ходы идут.
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<20) // выделение может быть большим
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var in In
			if err := json.Unmarshal([]byte(line), &in); err != nil {
				s.emit(Out{Type: "error", Text: "не разобрать сообщение: " + err.Error()})
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
		close(s.userQ)
	}()

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
				s.emit(Out{Type: "error", Chat: in.Chat, Text: "нет такого чата: " + err.Error()})
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

	for in := range s.userQ {
		switch in.Type {
		case "new_chat":
			// Сессионные разрешения живут в чате: новый чат — новые.
			c := s.newChat(currentID, providerFor(currentID))
			s.setActive(c)
			c.emit(Out{Type: "chat_reset"})
			continue
		case "command":
			switch in.Name {
			case "state":
				if subsErr != nil {
					// Молчать нельзя: человек увидит «источник ExecAI» и решит, что его
					// подписки исчезли, а файл просто не разобрался.
					s.emit(Out{Type: "notice", Text: "subscriptions.json не прочитан (" + subsErr.Error() +
						") — работаю на базовом источнике; подписки не потеряны, но файл нужно починить"})
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
					s.emit(Out{Type: "error", Text: "нет такой модели у текущего источника: " + in.Value})
					break
				}
				currentID = in.Value
				cli = llmpick.Client(cfg, subs, currentID, providerFor(currentID), cr.Token)
				// Выбор персистится, как в TUI: перезапуск плагина не должен
				// молча возвращать старую модель.
				cfg.SelectedModelID = currentID
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
					s.emit(Out{Type: "error", Text: "неизвестный уровень: " + in.Value})
					break
				}
				security.Set(lvl)
				cfg.SecurityLevel = lvl.String()
				if err := config.Save(cfg); err != nil {
					s.emit(Out{Type: "error", Text: err.Error()})
					break
				}
				s.emit(Out{Type: "notice", Text: "уровень безопасности: " + lvl.Title()})
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
				// После входа список моделей ExecAI становится доступен —
				// перечитываем каталог и пересобираем клиента.
				res = llm.FetchModelsCached(cfg.APIBase, cr.Token)
				reconcileModel()
				cli = llmpick.Client(cfg, subs, currentID, providerFor(currentID), cr.Token)
				s.emit(Out{Type: "login_done", Text: cr.Email})
				s.emit(stateOut())
			case "logout":
				if err := config.DeleteCredentials(); err != nil {
					s.emit(Out{Type: "error", Text: err.Error()})
					break
				}
				cr = &config.Credentials{}
				s.emit(Out{Type: "notice", Text: "вышли из ExecAI (подписки остались)"})
				s.emit(stateOut())
			case "connect":
				msg, err := connect(subs, in.Value, in.Key, in.BaseURL)
				if err != nil {
					s.emit(Out{Type: "error", Text: err.Error()})
					break
				}
				reconcileModel()
				cli = llmpick.Client(cfg, subs, currentID, providerFor(currentID), cr.Token)
				s.emit(Out{Type: "notice", Text: msg})
				s.emit(stateOut())
			case "disconnect":
				subs.Remove(in.Value)
				if err := subs.Save(); err != nil {
					s.emit(Out{Type: "error", Text: err.Error()})
					break
				}
				reconcileModel()
				cli = llmpick.Client(cfg, subs, currentID, providerFor(currentID), cr.Token)
				s.emit(Out{Type: "notice", Text: "отключено: " + in.Value})
				s.emit(stateOut())
			case "models_refreshed":
				// Фоновое обновление уже записало список на диск — перечитываем
				// и показываем панели свежий каталог.
				if st, err := subscriptions.Load(); err == nil && st != nil {
					subs = st
				}
				s.emit(stateOut())
			case "list_chats":
				list, _ := sessions.List()
				active := s.activeChat()
				o := Out{Type: "chats"}
				for _, c := range list {
					// Только чаты ЭТОГО воркспейса: история в редакторе
					// скоупится проектом, как у Claude Code.
					if c.CWD != opts.Cwd {
						continue
					}
					// Пустой заголовок отдаём как есть: подпись для чата без
					// темы — дело интерфейса, у него она на языке пользователя.
					item := NamedItem{ID: c.ID, Label: c.Title, Active: active != nil && c.ID == active.id}
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
				// Чаты, которых на диске ещё нет: первый ход идёт или только
				// что начался. Без них человек, переключившийся на другую
				// вкладку, не смог бы вернуться в чат, который ещё отвечает.
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
					c.mu.Unlock()
					item := NamedItem{ID: id, Label: label, Active: c == active, Busy: c.isBusy(), Wake: c.wakeAt()}
					o.Chats = append([]NamedItem{item}, o.Chats...)
				}
				s.chatsMu.Unlock()
				s.emit(o)
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
					s.emit(Out{Type: "notice", Text: "продолжать нечего — это первый чат в проекте"})
					break
				}
				c, err := s.loadChat(last.ID)
				if err != nil {
					s.emit(Out{Type: "error", Text: "не открыть последний чат: " + err.Error()})
					break
				}
				open(c)
			case "load_chat":
				c, err := s.loadChat(in.Value)
				if err != nil {
					s.emit(Out{Type: "error", Text: "не открыть чат: " + err.Error()})
					break
				}
				open(c)
			case "cancel_wakeup":
				c := target(In{Chat: firstNonEmpty(in.Value, in.Chat)})
				if c == nil {
					break
				}
				if !c.cancelWake() {
					s.emit(Out{Type: "notice", Chat: c.id, Text: "пробуждение не было запланировано"})
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
					s.emit(Out{Type: "notice", Chat: c.id, Text: "пробуждение не было запланировано"})
					break
				}
				c.fireWake()
			default:
				s.emit(Out{Type: "error", Text: "неизвестная команда: " + in.Name})
			}
			continue
		case "user":
			// ок, ниже
		default:
			s.emit(Out{Type: "error", Text: "неизвестный тип: " + in.Type})
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
			c.emit(Out{Type: "error", Text: "этот чат ещё отвечает — дождись конца хода или останови его"})
		}
	}
	// Редактор закрыл канал: ходы в полёте отменяем — отвечать некому.
	s.chatsMu.Lock()
	all := make([]*chatSession, 0, len(s.chats))
	for _, c := range s.chats {
		all = append(all, c)
	}
	s.chatsMu.Unlock()
	for _, c := range all {
		c.stop()
	}
	return nil
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
	b.WriteString("\n\n[контекст редактора")
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
			b.WriteString("\nПриложенные изображения: " + strings.Join(imgs, " "))
		}
		if len(files) > 0 {
			b.WriteString("\nПриложенные файлы (прочитай нужные инструментом Read): " +
				strings.Join(files, ", "))
		}
	}
	return b.String()
}

// streamer транслирует события цикла в протокол.
type streamer struct {
	c         *chatSession
	toolSeq   int
	curID     string
	curTool   string
	curPath   string // путь из аргументов текущего Write/Edit
	turnStart time.Time
	changed   map[string]bool
}

func (st *streamer) OnText(delta string)      { st.c.emit(Out{Type: "text_delta", Text: delta}) }
func (st *streamer) OnReasoning(delta string) { st.c.emit(Out{Type: "reasoning_delta", Text: delta}) }

func (st *streamer) OnToolCall(name string, args json.RawMessage) {
	st.toolSeq++
	st.curID = fmt.Sprintf("t%d", st.toolSeq)
	st.curTool = name
	st.c.emit(Out{Type: "tool_call", ID: st.curID, Tool: name, Summary: agent.SummaryFor(name, args)})
	// Write/Edit меняют файлы — путь запоминаем, но в changed попадёт только
	// то, что РЕАЛЬНО легло на диск (см. OnToolResult): отказ пользователя
	// возвращается без ошибки, и по одному ok файл от «не файла» не отличить.
	st.curPath = ""
	if name == "Write" || name == "Edit" {
		var a struct {
			Path string `json:"path"`
			File string `json:"file_path"`
		}
		if json.Unmarshal(args, &a) == nil {
			if a.Path != "" {
				st.curPath = a.Path
			} else if a.File != "" {
				st.curPath = a.File
			}
		}
	}
}

func (st *streamer) OnToolChunk(name string, chunk string) {
	st.c.emit(Out{Type: "tool_chunk", ID: st.curID, Tool: name, Chunk: chunk})
}

func (st *streamer) OnToolResult(name string, result string, err error) {
	ok := err == nil
	// Файл считается изменённым по ФАКТУ: существует и тронут в этом ходу.
	// Люфт 2с: файловые часы грубее time.Now() (tmpfs штампует тиком и может
	// отстать на миллисекунды, FAT — на 2 секунды), без люфта свежая запись
	// выглядела бы «старой».
	if ok && st.curPath != "" {
		if fi, statErr := os.Stat(st.curPath); statErr == nil && !fi.ModTime().Before(st.turnStart.Add(-2*time.Second)) {
			st.changed[st.curPath] = true
		}
		st.curPath = ""
	}
	tail := result
	if err != nil {
		tail = err.Error()
	}
	const max = 2000
	if len([]rune(tail)) > max {
		r := []rune(tail)
		tail = "…" + string(r[len(r)-max:])
	}
	st.c.emit(Out{Type: "tool_result", ID: st.curID, Tool: name, OK: &ok, Tail: tail})
}

func (st *streamer) changedList() []string {
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
	c            *chatSession
	sessionTools map[string]bool
	sessionExact map[string]bool
}

func (a *ideApprover) AskApprove(toolName string, args json.RawMessage, summary string) agent.ApproveDecision {
	key := agent.ExactKey(toolName, args)
	if a.sessionTools[toolName] || a.sessionExact[key] {
		return agent.ApproveOnce
	}

	id, ch := a.c.srv.newQuestion()
	out := Out{
		Type: "ask", ID: id, Tool: toolName, Summary: summary,
		Options: []AskOption{
			{answerOnce, "Разово", "разрешить только этот вызов"},
			{answerSession, "Весь " + toolName + " в сессии", "до конца этого чата"},
			{answerExact, "Эту команду в сессии", "точно такой вызов до конца чата"},
			// Формулировка нарочно не обещает «весь инструмент»: у чтения и
			// сети «навсегда» означает КАТАЛОГ или ДОМЕН (см. tools.Scoped),
			// и точный масштаб напечатан в summary выше. Кнопка, обещающая
			// больше, чем делает, — обман в самом неподходящем месте.
			{answerAlways, "НАВСЕГДА", "запомнить это разрешение — запишется в permissions.json"},
			{answerDeny, "Отклонить", "не выполнять; агент получит отказ"},
		},
	}
	// Вопрос помнится до ответа: вернувшись во вкладку, человек увидит кнопки.
	a.c.mu.Lock()
	cp := out
	cp.Chat = a.c.id
	a.c.pendingAsk = &cp
	a.c.mu.Unlock()
	a.c.emit(out)
	answer := <-ch
	a.c.mu.Lock()
	if a.c.pendingAsk != nil && a.c.pendingAsk.ID == id {
		a.c.pendingAsk = nil
	}
	a.c.mu.Unlock()

	switch answer {
	case answerOnce:
		return agent.ApproveOnce
	case answerSession:
		a.sessionTools[toolName] = true
		return agent.ApproveOnce
	case answerExact:
		a.sessionExact[key] = true
		return agent.ApproveOnce
	case answerAlways:
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
			o.Msgs = append(o.Msgs, ReplayMsg{Role: "user", Text: llm.ContentText(msg.Content)})
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
