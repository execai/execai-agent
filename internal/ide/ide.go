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
// Версия протокола — Protocol. Плагин сверяет её из события ready и честно
// говорит «обнови execai», а не падает на незнакомом событии.
package ide

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/catalog"
	"github.com/velesbsdllc/agent-vbai/internal/config"
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
	// list_chats | load_chat; value — аргумент
	Name string `json:"name,omitempty"`
	// connect: ключ провайдера и (опционально) свой base_url
	Key     string `json:"key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
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

	// ready
	Version  string `json:"version,omitempty"`
	Protocol int    `json:"protocol,omitempty"`
	Model    string `json:"model,omitempty"`
	Source   string `json:"source,omitempty"`
	Cwd      string `json:"cwd,omitempty"`

	// text_delta / reasoning_delta / notice / error
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

type session struct {
	mu  sync.Mutex
	enc *json.Encoder

	answers   map[string]chan string // id вопроса → канал ответа
	answersMu sync.Mutex
	nextID    int

	turnCancel context.CancelFunc // отмена текущего хода (stop)
}

func (s *session) emit(o Out) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.enc.Encode(o)
}

func (s *session) newQuestion() (string, chan string) {
	s.answersMu.Lock()
	defer s.answersMu.Unlock()
	s.nextID++
	id := fmt.Sprintf("q%d", s.nextID)
	ch := make(chan string, 1)
	s.answers[id] = ch
	return id, ch
}

func (s *session) deliver(id, value string) {
	s.answersMu.Lock()
	ch := s.answers[id]
	delete(s.answers, id)
	s.answersMu.Unlock()
	if ch != nil {
		ch <- value
	}
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

	s := &session{enc: json.NewEncoder(os.Stdout), answers: map[string]chan string{}}

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

	// AskUser: в редакторе ЕСТЬ кому ответить — вопрос уезжает кнопками.
	tools.SetAskUserFunc(func(ctx context.Context, question string, aos []tools.AskOption) (string, error) {
		id, ch := s.newQuestion()
		out := Out{Type: "ask_user", ID: id, Question: question}
		for _, o := range aos {
			out.Options = append(out.Options, AskOption{Value: o.Label, Label: o.Label, Description: o.Description})
		}
		s.emit(out)
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

	// Читатель stdin: user-сообщения в очередь, ответы — напрямую в каналы.
	// Отдельная горутина, потому что ответы должны доходить, пока ход идёт.
	userQ := make(chan In, 16)
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
				s.mu.Lock()
				cancel := s.turnCancel
				s.mu.Unlock()
				if cancel != nil {
					cancel()
				}
			case "ping":
				s.emit(Out{Type: "pong"})
			default:
				userQ <- in
			}
		}
		close(userQ)
	}()

	var history []llm.AIMessage
	// История — ОБЩИЙ стор с TUI (~/.config/execai/sessions): чат, начатый в
	// редакторе, виден в /resume терминала и наоборот. Файл создаётся лениво —
	// каждое открытие редактора не должно плодить пустые сессии.
	var sess *sessions.Session
	persist := func() {
		if len(history) == 0 {
			return
		}
		if sess == nil {
			sess = sessions.New(currentID, providerFor(currentID), opts.Cwd)
		}
		sess.Model = currentID
		sess.Messages = history
		if err := sess.Save(); err != nil {
			fmt.Fprintln(os.Stderr, "не сохранить сессию:", err)
		}
	}
	ap := &ideApprover{s: s, perms: perms, sessionTools: map[string]bool{}, sessionExact: map[string]bool{}}

	// Живой список моделей стареет: план могли повысить, вендор — выкатить
	// новую модель. Терминал перезапрашивает его сам, панель раньше нет —
	// после подключения список не обновлялся никогда. Обновляем в фоне: фетч
	// идёт на своей копии стора, а применяет результат основной цикл.
	refreshModels := func(provider string) {
		go func() {
			if subsconnect.RefreshModels(provider) {
				userQ <- In{Type: "command", Name: "models_refreshed"}
			}
		}()
	}
	if a := subs.ActiveSubscription(); a != nil && subsconnect.WantsLiveModels(a.Provider) && subsconnect.Stale(*a) {
		refreshModels(a.Provider)
	}

	for in := range userQ {
		switch in.Type {
		case "new_chat":
			persist()
			sess = nil
			history = nil
			// Сессионные разрешения умирают вместе с чатом: «в сессии»
			// значит «в этом разговоре», как в TUI.
			ap.sessionTools = map[string]bool{}
			ap.sessionExact = map[string]bool{}
			s.emit(Out{Type: "chat_reset"})
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
				o := Out{Type: "chats"}
				for _, c := range list {
					// Только чаты ЭТОГО воркспейса: история в редакторе
					// скоупится проектом, как у Claude Code.
					if c.CWD != opts.Cwd {
						continue
					}
					// Пустой заголовок отдаём как есть: подпись для чата без
					// темы — дело интерфейса, у него она на языке пользователя.
					title := c.Title
					o.Chats = append(o.Chats, NamedItem{
						ID: c.ID, Label: title,
						Active: sess != nil && c.ID == sess.ID,
					})
					if len(o.Chats) >= 30 {
						break
					}
				}
				s.emit(o)
			case "resume_last":
				// Продолжить последний чат ЭТОГО проекта.
				//
				// В терминале для этого есть /resume, а панель открывалась
				// каждый раз с чистого листа: чат никуда не девался, но
				// человек должен был искать его в истории. Редактор — место,
				// куда возвращаются, и разговор обязан продолжаться сам.
				list, _ := sessions.List()
				var last *sessions.Session
				for i := range list {
					if list[i].CWD != opts.Cwd {
						continue
					}
					if sess != nil && list[i].ID == sess.ID {
						break // уже открыт — восстанавливать нечего
					}
					last = list[i]
					break // List отсортирован по времени изменения
				}
				if last == nil {
					s.emit(Out{Type: "notice", Text: "продолжать нечего — это первый чат в проекте"})
					break
				}
				full, err := sessions.Load(last.ID)
				if err != nil {
					s.emit(Out{Type: "error", Text: "не открыть последний чат: " + err.Error()})
					break
				}
				persist()
				sess = full
				history = append([]llm.AIMessage(nil), full.Messages...)
				s.emit(replayOut(history))
			case "load_chat":
				full, err := sessions.Load(in.Value)
				if err != nil {
					s.emit(Out{Type: "error", Text: "не открыть чат: " + err.Error()})
					break
				}
				persist() // текущий не теряем
				sess = full
				history = append([]llm.AIMessage(nil), full.Messages...)
				s.emit(replayOut(history))
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

		turnCtx, cancel := context.WithCancel(ctx)
		s.mu.Lock()
		s.turnCancel = cancel
		s.mu.Unlock()

		st := &streamer{s: s, changed: map[string]bool{}, turnStart: time.Now()}
		a := agent.New(cli, registry, sys, ap, st)
		if opts.MaxIterations > 0 {
			a.MaxIterations = opts.MaxIterations
		} else {
			a.MaxIterations = cfg.GetMaxIterations()
		}

		start := time.Now()
		s.emit(Out{Type: "turn_start"})
		newHist, err := a.Run(turnCtx, history, buildPrompt(in))
		cancel()
		s.mu.Lock()
		s.turnCancel = nil
		s.mu.Unlock()

		if err != nil {
			if turnCtx.Err() != nil {
				s.emit(Out{Type: "done", Text: "stopped", Elapsed: time.Since(start).Seconds()})
			} else {
				s.emit(Out{Type: "error", Text: err.Error()})
				s.emit(Out{Type: "done", Text: "error", Elapsed: time.Since(start).Seconds()})
			}
			// Историю не теряем: частичный ход полезнее пустоты.
			if newHist != nil {
				history = newHist
				persist()
			}
			continue
		}
		history = newHist
		persist()
		if len(st.changedList()) > 0 {
			s.emit(Out{Type: "files_changed", Paths: st.changedList()})
		}
		s.emit(Out{Type: "done", Text: "ok", Elapsed: time.Since(start).Seconds()})
	}
	return nil
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
	s         *session
	toolSeq   int
	curID     string
	curTool   string
	curPath   string // путь из аргументов текущего Write/Edit
	turnStart time.Time
	changed   map[string]bool
}

func (st *streamer) OnText(delta string)      { st.s.emit(Out{Type: "text_delta", Text: delta}) }
func (st *streamer) OnReasoning(delta string) { st.s.emit(Out{Type: "reasoning_delta", Text: delta}) }

func (st *streamer) OnToolCall(name string, args json.RawMessage) {
	st.toolSeq++
	st.curID = fmt.Sprintf("t%d", st.toolSeq)
	st.curTool = name
	st.s.emit(Out{Type: "tool_call", ID: st.curID, Tool: name, Summary: agent.SummaryFor(name, args)})
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
	st.s.emit(Out{Type: "tool_chunk", ID: st.curID, Tool: name, Chunk: chunk})
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
	st.s.emit(Out{Type: "tool_result", ID: st.curID, Tool: name, OK: &ok, Tail: tail})
}

func (st *streamer) changedList() []string {
	out := make([]string, 0, len(st.changed))
	for p := range st.changed {
		out = append(out, p)
	}
	return out
}

// ideApprover — разрешения кнопками в редакторе. Значения и смысл — общий
// словарь с веб-каналом; «в сессии» = в этом разговоре (как в TUI), поэтому
// new_chat сбрасывает сессионные разрешения.
type ideApprover struct {
	s            *session
	perms        *agent.Permissions
	sessionTools map[string]bool
	sessionExact map[string]bool
}

func (a *ideApprover) AskApprove(toolName string, args json.RawMessage, summary string) agent.ApproveDecision {
	key := agent.ExactKey(toolName, args)
	if a.sessionTools[toolName] || a.sessionExact[key] {
		return agent.ApproveOnce
	}

	id, ch := a.s.newQuestion()
	a.s.emit(Out{
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
	})
	answer := <-ch

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

// OnIterationStart — плагину полезно показывать номер итерации в статусе.
func (st *streamer) OnIterationStart(n int) { st.s.emit(Out{Type: "iteration", N: n}) }

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
