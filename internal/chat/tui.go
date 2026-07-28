// TUI с tool-use loop через /aicore-vbai/agent-stream.
// Архитектура: bubbletea Model держит history (assistant/user/tool messages),
// agent.Agent гоняет цикл в горутине, события (text-дельты, tool-вызовы,
// результаты, запросы подтверждения) приходят через канал как tea.Msg.
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sort"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/auth"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/sessions"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
	"github.com/velesbsdllc/agent-vbai/internal/welcome"
)

// ===== стили =====

var (
	styleHeader = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).Background(lipgloss.Color("#5F5FD7")).Padding(0, 1)

	styleStatus    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888")).Background(lipgloss.Color("#222"))
	styleStatusKey = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD75F"))

	styleUserPrompt   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5FAFFF"))
	styleAssistPrefix = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A8FF60"))
	styleToolPrefix   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFD75F"))
	styleToolBody     = lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
	styleReasoning    = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#666"))
	styleSysHint      = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#888"))
	styleErr          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5F5F"))
	styleApproveBox   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFD75F")).Background(lipgloss.Color("#222")).Padding(0, 1)
	styleHelpKey      = lipgloss.NewStyle().Foreground(lipgloss.Color("#5FAFFF")).Bold(true)
	styleHelpDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
)

// ===== msg types =====

type agentTextMsg string
type agentReasoningMsg string
type agentToolCallMsg struct {
	name string
	args json.RawMessage
}
type agentToolResultMsg struct {
	name   string
	result string
	err    error
}
type agentToolChunkMsg struct {
	name  string
	chunk string
}
// Сигналы от device-flow polling в loginMode.
type agentLinkDoneMsg struct{ creds *config.Credentials }
type agentLinkErrMsg struct{ err error }
type agentLinkTickMsg struct{ n int } // тик поллинга device-flow, показываем прогресс

type agentApproveAskMsg struct {
	name    string
	args    json.RawMessage
	summary string
	reply   chan agent.ApproveDecision
}
type agentDoneMsg struct {
	history []llm.AIMessage
}
type agentErrMsg struct{ err error }

// ===== model =====

type tuiModel struct {
	cfg   *config.Config
	creds *config.Credentials

	// LLM-клиент и агент. Инициализируются после успешного login.
	cli      llm.StreamingLLM       // ExecAI или внешняя подписка — переключается /use
	subs     *subscriptions.Store   // подписки (Z.ai/Anthropic/OpenAI)
	registry *tools.Registry
	system   string

	// loginMode = true когда creds нет — TUI просит подтверждения в браузере
	// (device-flow) или fallback на paste-token.
	loginMode   bool
	pendingLink *auth.LinkStart // активный запрос device-flow, ждём подтверждения

	models       []llm.Model
	execAIModels []llm.Model // снимок исходного каталога ExecAI — для возврата из внешних подписок
	ollamaModels []llm.Model // кеш установленных в Ollama моделей (обновляется через /connect ollama и /model refresh)
	current      llm.Model
	lastLoginAt  time.Time // когда закончился последний device-flow. Защита от infinite-loop 401→device-flow→401.

	session    *sessions.Session // автосохраняется после каждого обмена
	history    []llm.AIMessage   // включая system
	uiSegments      []segment  // что отрисовать пользователю (user/assistant/tool/reasoning)
	lastEmittedIdx  int        // Ink-style: сколько uiSegments уже эмиттено через Program.Println (не рендерим повторно)
	streamBuf  strings.Builder  // текущая накопленная дельта assistant
	reasonBuf  strings.Builder  // текущий reasoning
	toolBuf    strings.Builder  // live-вывод текущего streaming-tool (Bash и т.п.)
	toolName   string           // имя tool который сейчас стримит
	streaming  bool

	// approve-режим
	approving       bool
	approveTool     string
	approveSummary  string
	approveReplyCh  chan agent.ApproveDecision
	approveFocus    int // 0=Раз, 1=Сессия, 2=Команда, 3=Всегда, 4=Отклонить

	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model
	renderer *glamour.TermRenderer

	width  int
	height int

	cancel context.CancelFunc
	once   sync.Once

	err           string
	statusMessage string
	cwd           string

	// === Autocomplete (как Claude Code) ===
	// Активно когда юзер начинает с "/" — показывает меню команд/аргументов.
	suggestActive bool
	suggestItems  []suggestItem
	suggestFocus  int

	// История вводов (как bash): ↑↓ листают.
	inputHistory   []string
	historyIndex   int    // 0 = текущий ввод; 1..len = инпут N шагов назад. -1 = нет навигации
	historyDraft   string // буфер несабмиченного ввода когда юзер ушёл в историю

	// Подтверждение выхода (Ctrl+C/Ctrl+D дважды в течение 2с).
	exitConfirmAt time.Time

	// Thinking-effort picker (modal overlay как в Claude Code).
	// Открывается Shift+Tab. ←/→ — выбор, Enter — confirm, Esc — cancel.
	thinkingPickerActive bool
	thinkingPickerIdx    int // 0..len(thinkingLevels)-1

	// Paste collapse (как в Claude Code): большие вставки заменяются
	// маркером "[Pasted #N — L lines, C chars]" в textarea и в
	// отображаемой user-сегменте. Реальный контент хранится в pasteStore
	// и разворачивается при отправке агенту.
	pasteStore   map[int]pasteEntry
	pasteCounter int

	// /loop — периодическое повторение промта (как в Claude Code).
	// Работает только пока TUI открыт. /loop stop — остановить.
	loopActive   bool
	loopInterval time.Duration
	loopPrompt   string
}

// loopTickMsg — событие тика для /loop.
type loopTickMsg struct{}

// autoWakeMsg — AI запланировал пробуждение через schedule_wakeup tool.
type autoWakeMsg struct {
	prompt string
}

// suggestItem — одна строка в меню подсказок.
type suggestItem struct {
	insert string // что подставить в textarea при выборе
	label  string // как отображать в меню (например "/model — выбор модели")
	hint   string // приглушённая правая часть (например ID/описание)
}

// pasteEntry — одна вставка большого текста, коллапсированная маркером
// "[Pasted #N — L lines, C chars]". marker уникальный, полное содержимое
// в text. При submit expandPasteMarkers разворачивает markers обратно.
type pasteEntry struct {
	id     int
	text   string
	lines  int
	chars  int
	marker string
}

type segment struct {
	kind string // "user" | "assistant" | "reasoning" | "tool_call" | "tool_result" | "system_hint"
	body string
	tool string // для tool_call/tool_result — имя
}

// ===== entry point =====

// agentVersion — версия бинаря, прокидывается из cmd/execai/main.go.
var agentVersion = "dev"

func RunTUI(_ context.Context, cfg *config.Config, ver string) error {
	if ver != "" {
		agentVersion = ver
	}
	if os.Getenv("COLORFGBG") == "" {
		_ = os.Setenv("COLORFGBG", "15;0")
	}

	cr, _ := config.LoadCredentials()
	loginMode := cr == nil || cr.Token == ""

	cwd, _ := os.Getwd()
	short := shortenCWD(cwd)
	wMsg := welcome.MaybeWelcome()
	registry := tools.Default(cwd)

	ta := textarea.New()
	if loginMode {
		ta.Placeholder = "Вставь JWT-токен из браузерной сессии ExecAI и нажми Enter"
	} else {
		ta.Placeholder = "Опиши задачу. Enter — отправить, /help — команды."
	}
	ta.Focus()
	ta.Prompt = "▌ "
	ta.ShowLineNumbers = false
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.CharLimit = 32768
	ta.KeyMap.InsertNewline.SetKeys("shift+enter", "ctrl+j")
	// Ink-style: статичный курсор, без мигания. Каждый blink → re-render →
	// терминал сбрасывает native selection. В classic (alt-screen) blink
	// ничему не мешает — restart в Update когда переключим режим.
	if !cfg.ClassicTUI {
		ta.Cursor.SetMode(cursor.CursorStatic)
	}

	vp := viewport.New(80, 20)
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD75F"))

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(76),
	)

	subs, _ := subscriptions.Load()
	m := &tuiModel{
		pasteStore:   map[int]pasteEntry{},
		cfg:          cfg,
		creds:        cr,
		registry:     registry,
		loginMode:    loginMode,
		viewport:     vp,
		textarea:     ta,
		spinner:      sp,
		renderer:     renderer,
		cwd:          short,
		inputHistory: loadInputHistory(),
		subs:         subs,
	}
	if loginMode {
		intro := "Привет! Чтобы войти — нужно подтвердить агента в браузере (как gh auth login).\n" +
			"Если по какой-то причине device-flow не работает — можешь вставить JWT-токен (eyJ…) сюда и нажать Enter."
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: intro})
	} else {
		// Сразу подгружаем модели и собираем агентский слой.
		// Если токен протух (401) — переходим в loginMode и стартуем device-flow,
		// а не падаем с ошибкой.
		if err := m.bootAfterLogin(); err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "401") || strings.Contains(errStr, "токен истёк") {
				_ = auth.Logout()
				m.creds = nil
				m.loginMode = true
				m.uiSegments = append(m.uiSegments, segment{kind: "system_hint",
					body: "Старый токен невалиден на " + cfg.APIBase + ". Запускаю device-flow для нового входа…"})
			} else {
				return err
			}
		} else {
			m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: m.welcomeText()})
			if wMsg != "" {
				m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: wMsg})
			}
		}
	}
	m.refreshViewport()

	// Ink-style rendering (как у Claude Code):
	// * История сообщений (assistant/tool_result/user submit) → tea.Println →
	//   становится обычным терминальным scrollback, native selection работает
	//   и не сбрасывается при re-render (эти строки уже не переписываются).
	// * View() рендерит только: текущий стрим + input + статус — это малая
	//   зона внизу, её re-render не мешает выделению текста выше.
	// * Alt-screen и mouse capture ОТКЛЮЧЕНЫ — терминал сам обрабатывает
	//   scroll wheel и drag-selection нативно.
	// Legacy classic-режим (m.cfg.ClassicTUI=true) можно вернуть /classic on.
	var p *tea.Program
	if m.cfg != nil && m.cfg.ClassicTUI {
		p = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	} else {
		p = tea.NewProgram(m)
	}
	teaProgramRef = p
	_, err := p.Run()
	return err
}

// ===== Init =====

func (m *tuiModel) Init() tea.Cmd {
	// В Ink-style не запускаем periodic blink/spinner — каждый tick вызывает
	// re-render View(), терминал видит output и сбрасывает native selection.
	// Cursor становится статичным (bubbles cursor mode = static).
	cmds := []tea.Cmd{tea.SetWindowTitle(m.titleString())}
	if m.cfg.ClassicTUI {
		cmds = append(cmds, textarea.Blink, m.spinner.Tick)
	}
	// В loginMode — сразу стартуем device-flow, не ждём ввода.
	if m.loginMode {
		cmds = append(cmds, func() tea.Msg {
			return autoStartLoginMsg{}
		})
	}
	// Auto-update check на старте (асинхронно, в фоне).
	cmds = append(cmds, m.checkForUpdateCmd())
	return tea.Batch(cmds...)
}

// titleString — для tea.SetWindowTitle. Формат: "execai · model · session-title".
func (m *tuiModel) titleString() string {
	parts := []string{"execai"}
	if m.current.ID != "" {
		parts = append(parts, m.current.ID)
	}
	if m.session != nil && m.session.Title != "" {
		t := m.session.Title
		if len(t) > 40 {
			t = t[:40] + "…"
		}
		parts = append(parts, t)
	}
	return strings.Join(parts, " · ")
}

type autoStartLoginMsg struct{}

// ===== Update =====

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Резервируем по строкам: header(1) + textarea(1) + status(1) + help(1) + запас(2)
		// под опциональный bottom-line (err/statusMessage) и потенциальный wrap status-bar.
		taH := 1
		// 7 = header(1) + status(1) + help(1) + thinking-slider-reserve(1) + buffer(3)
		vpH := m.height - 7 - taH
		if vpH < 5 {
			vpH = 5
		}
		m.viewport.Width = m.width
		m.viewport.Height = vpH
		m.textarea.SetWidth(m.width - 2)
		ww := m.width - 4
		if ww < 40 {
			ww = 40
		}
		m.renderer, _ = glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(ww),
		)
		m.refreshViewport()
		return m, nil

	case tea.KeyMsg:
		// approve-режим: стрелки/Tab переключают фокус, Enter подтверждает,
		// Esc/Ctrl+C — отклоняют. Также горячие клавиши.
		if m.approving {
			switch msg.String() {
			case "left", "shift+tab":
				if m.approveFocus > 0 {
					m.approveFocus--
				} else {
					m.approveFocus = 4
				}
				m.refreshViewport()
				return m, nil
			case "right", "tab":
				m.approveFocus = (m.approveFocus + 1) % 5
				m.refreshViewport()
				return m, nil
			case "enter":
				m.replyApprove(approveDecisionFromFocus(m.approveFocus))
			case "y", "Y", "д":
				m.replyApprove(agent.ApproveOnce)
			case "a", "A":
				m.replyApprove(agent.ApproveTool)
			case "s", "S":
				m.replyApprove(agent.ApproveExactArgs)
			case "f", "F":
				m.replyApprove(agent.ApproveAlways)
			case "n", "N", "esc", "ctrl+c":
				m.replyApprove(agent.ApproveDeny)
			}
			return m, nil
		}
		// Любая клавиша КРОМЕ Ctrl+C/Ctrl+D — сбрасывает «подтверждение выхода».
		if msg.Type != tea.KeyCtrlC && msg.Type != tea.KeyCtrlD && !m.exitConfirmAt.IsZero() {
			m.exitConfirmAt = time.Time{}
			m.statusMessage = ""
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			// Во время стриминга — отменяем генерацию, НЕ выходим.
			if m.streaming {
				if m.cancel != nil {
					m.cancel()
				}
				m.statusMessage = "стрим прерван"
				return m, nil
			}
			// Не во время стрима — требуем подтверждения: второй Ctrl+C в течение 2с.
			if !m.exitConfirmAt.IsZero() && time.Since(m.exitConfirmAt) < 2*time.Second {
				return m, tea.Quit
			}
			m.exitConfirmAt = time.Now()
			m.statusMessage = "нажми Ctrl+C ещё раз (в течение 2с) чтобы выйти, или любую клавишу чтобы отменить"
			return m, nil
		case tea.KeyCtrlD:
			// Та же логика, что Ctrl+C.
			if !m.exitConfirmAt.IsZero() && time.Since(m.exitConfirmAt) < 2*time.Second {
				return m, tea.Quit
			}
			m.exitConfirmAt = time.Now()
			m.statusMessage = "нажми Ctrl+D ещё раз (в течение 2с) чтобы выйти, или любую клавишу чтобы отменить"
			return m, nil
		case tea.KeyCtrlR:
			// Ctrl+R — открыть fuzzy-поиск по сессиям через existing autocomplete-меню.
			m.textarea.SetValue("/resume ")
			m.textarea.CursorEnd()
			return m, m.refreshSuggest()
		case tea.KeyShiftTab:
			// Shift+Tab — открыть/закрыть thinking-picker (модальный overlay как в Claude Code).
			if m.thinkingPickerActive {
				// Уже открыт — циклим вправо.
				m.thinkingPickerIdx = (m.thinkingPickerIdx + 1) % len(thinkingLevels)
			} else {
				m.thinkingPickerActive = true
				m.thinkingPickerIdx = m.currentThinkingIdx()
			}
			return m, nil
		case tea.KeyLeft:
			if m.thinkingPickerActive {
				m.thinkingPickerIdx = (m.thinkingPickerIdx - 1 + len(thinkingLevels)) % len(thinkingLevels)
				return m, nil
			}
		case tea.KeyRight:
			if m.thinkingPickerActive {
				m.thinkingPickerIdx = (m.thinkingPickerIdx + 1) % len(thinkingLevels)
				return m, nil
			}
		case tea.KeyCtrlL:
			m.history = nil
			m.uiSegments = nil; m.lastEmittedIdx = 0
			m.statusMessage = "история очищена"
			m.refreshViewport()
			return m, nil
		case tea.KeyTab:
			// Tab всегда триггерит autocomplete если строка начинается с "/".
			refCmd := m.refreshSuggest()
			if len(m.suggestItems) > 0 {
				cmd := m.acceptSuggest(m.suggestFocus)
				return m, tea.Batch(refCmd, cmd)
			}
			return m, refCmd
		case tea.KeyUp:
			if m.suggestActive && len(m.suggestItems) > 0 {
				if m.suggestFocus > 0 {
					m.suggestFocus--
				} else {
					m.suggestFocus = len(m.suggestItems) - 1
				}
				return m, nil
			}
			// История вводов: листаем назад. Сохраняем drafted-текст если ещё не в истории.
			if len(m.inputHistory) > 0 && m.historyIndex < len(m.inputHistory) {
				if m.historyIndex == 0 {
					m.historyDraft = m.textarea.Value()
				}
				m.historyIndex++
				m.textarea.SetValue(m.inputHistory[len(m.inputHistory)-m.historyIndex])
				m.textarea.CursorEnd()
				return m, nil
			}
		case tea.KeyDown:
			if m.suggestActive && len(m.suggestItems) > 0 {
				m.suggestFocus = (m.suggestFocus + 1) % len(m.suggestItems)
				return m, nil
			}
			// История вводов: листаем вперёд. На 0 — возвращаем draft.
			if m.historyIndex > 0 {
				m.historyIndex--
				if m.historyIndex == 0 {
					m.textarea.SetValue(m.historyDraft)
					m.historyDraft = ""
				} else {
					m.textarea.SetValue(m.inputHistory[len(m.inputHistory)-m.historyIndex])
				}
				m.textarea.CursorEnd()
				return m, nil
			}
		case tea.KeyEsc:
			if m.thinkingPickerActive {
				m.thinkingPickerActive = false
				m.statusMessage = "effort picker — отменено"
				return m, nil
			}
			if m.suggestActive {
				return m, m.closeSuggest()
			}
		case tea.KeyEnter:
			// Если thinking-picker открыт — Enter подтверждает выбор.
			if m.thinkingPickerActive {
				lvl := thinkingLevels[m.thinkingPickerIdx]
				m.cfg.ThinkingBudget = lvl.budget
				_ = config.Save(m.cfg)
				m.cli = m.makeLLMClient()
				m.thinkingPickerActive = false
				m.statusMessage = fmt.Sprintf("🧠 effort → %s (%d токенов)", lvl.label, lvl.budget)
				return m, nil
			}
			// Если меню активно — Enter:
			//   * если текст уже точно равен какой-то подсказке → сабмитим как обычно
			//   * иначе принимаем focused подсказку (только заменяем текст)
			if m.suggestActive && len(m.suggestItems) > 0 {
				cur := strings.TrimSpace(m.textarea.Value())
				exact := false
				for _, it := range m.suggestItems {
					if cur == strings.TrimSpace(it.insert) {
						exact = true
						break
					}
				}
				if !exact {
					cmd := m.acceptSuggest(m.suggestFocus)
					return m, cmd
				}
			}
			// exact match ИЛИ меню не активно — закрываем меню (если активно),
			// отключаем захват мыши и сабмитим как обычно. tea.DisableMouse
			// собираем в pendingCmd чтобы не потерять при return.
			var pendingCmd tea.Cmd
			if m.suggestActive {
				pendingCmd = m.closeSuggest()
			}
			if m.streaming {
				return m, pendingCmd
			}
			line := strings.TrimSpace(m.textarea.Value())
			if line == "" {
				return m, pendingCmd
			}
			// История ввода: пишем уникальные сабмиты (не дублируем подряд тот же).
			if n := len(m.inputHistory); n == 0 || m.inputHistory[n-1] != line {
				m.inputHistory = append(m.inputHistory, line)
				if len(m.inputHistory) > 200 {
					m.inputHistory = m.inputHistory[len(m.inputHistory)-200:]
				}
				go saveInputHistory(line) // persist на диск, не блокируем UI
			}
			m.historyIndex = 0
			m.historyDraft = ""
			m.textarea.Reset()
			var sub tea.Model
			var subCmd tea.Cmd
			if m.loginMode {
				sub, subCmd = m.handleLoginInput(line)
			} else {
				sub, subCmd = m.handleInput(line)
			}
			return sub, tea.Batch(pendingCmd, subCmd)
		}

	case spinner.TickMsg:
		if m.streaming || m.approving {
			var c tea.Cmd
			m.spinner, c = m.spinner.Update(msg)
			return m, c
		}
		return m, nil

	case agentTextMsg:
		m.streamBuf.WriteString(string(msg))
		m.refreshViewport()
		return m, nil

	case agentReasoningMsg:
		m.reasonBuf.WriteString(string(msg))
		m.refreshViewport()
		return m, nil

	case agentToolCallMsg:
		// Финализируем накопленный текст assistant как сегмент
		if m.streamBuf.Len() > 0 {
			m.uiSegments = append(m.uiSegments, segment{kind: "assistant", body: m.streamBuf.String()})
			m.streamBuf.Reset()
		}
		if m.reasonBuf.Len() > 0 {
			m.uiSegments = append(m.uiSegments, segment{kind: "reasoning", body: m.reasonBuf.String()})
			m.reasonBuf.Reset()
		}
		body := summaryForArgs(msg.name, msg.args)
		m.uiSegments = append(m.uiSegments, segment{kind: "tool_call", tool: msg.name, body: body})
		// Сбрасываем live-буфер для нового tool вызова
		m.toolBuf.Reset()
		m.toolName = msg.name
		m.refreshViewport()
		return m, nil

	case agentToolChunkMsg:
		// Аппендим в live-буфер, обновляем viewport — отрисуется поверх ▶ tool_call.
		m.toolBuf.WriteString(msg.chunk)
		m.refreshViewport()
		return m, nil

	case agentToolResultMsg:
		body := msg.result
		if msg.err != nil {
			body = "ERROR: " + msg.err.Error()
			if msg.result != "" {
				body += "\n\n" + msg.result
			}
		}
		m.uiSegments = append(m.uiSegments, segment{kind: "tool_result", tool: msg.name, body: body})
		m.toolBuf.Reset()
		m.toolName = ""
		m.refreshViewport()
		return m, nil

	case autoStartLoginMsg:
		if m.loginMode && m.pendingLink == nil {
			return m.startDeviceLink()
		}
		return m, nil

	case tea.MouseMsg:
		// Клик по слайдеру thinking — задаёт уровень. Layout: ... status (1) + thinking-slider (1) + help (1).
		// Слайдер находится в предпоследней строке снизу когда видим.
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft &&
			m.subs != nil && m.subs.Active == subscriptions.SourceZAI {
			sliderY := m.height - 2 // status, slider, help — slider = height-2 (0-индексно)
			if msg.Y == sliderY {
				// "🧠 think: off · low · med · high · max    (Shift+Tab или клик)"
				// Определяем позицию ближайшего label по X.
				// Префикс "🧠 think: " ~ 11 видимых ячеек (эмодзи занимает 2).
				prefix := 11
				// Длины меток: off(3) · low(3) · med(3) · high(4) · max(3) разделители " · " (3)
				positions := []struct {
					start, end int
					budget     int
				}{}
				x := prefix
				for _, lvl := range thinkingLevels {
					labelLen := len(lvl.label)
					// active добавляет 2 (паддинг) — но для clickable считаем без паддинга
					positions = append(positions, struct {
						start, end int
						budget     int
					}{x, x + labelLen, lvl.budget})
					x += labelLen + 3 // ' · '
				}
				for _, p := range positions {
					if msg.X >= p.start-1 && msg.X <= p.end+1 {
						m.cfg.ThinkingBudget = p.budget
						_ = config.Save(m.cfg)
						m.cli = m.makeLLMClient()
						label := ""
						for _, lvl := range thinkingLevels {
							if lvl.budget == p.budget {
								label = lvl.label
							}
						}
						m.statusMessage = fmt.Sprintf("🧠 effort → %s (%d токенов)", label, p.budget)
						return m, nil
					}
				}
			}
		}
		// Клик по строке меню — выбрать + принять.
		if m.suggestActive && len(m.suggestItems) > 0 &&
			msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// Layout: header(1) + viewport(vpH) + bottom(0..1) + suggest_box → textarea → status → help
			headerH := 1
			vpH := m.viewport.Height
			bottomH := 0
			if m.err != "" || m.statusMessage != "" {
				bottomH = 1
			}
			y0 := headerH + vpH + bottomH // 0-индексная позиция верхнего border'а box'а
			maxRows := 8
			if len(m.suggestItems) < maxRows {
				maxRows = len(m.suggestItems)
			}
			// Внутри box: row 0 = border, rows 1..maxRows = items, row maxRows+1 = footer, row maxRows+2 = border.
			clicked := msg.Y - y0 - 1 // относительно начала items
			if clicked >= 0 && clicked < maxRows {
				start := 0
				if m.suggestFocus >= maxRows {
					start = m.suggestFocus - maxRows + 1
				}
				idx := start + clicked
				if idx >= 0 && idx < len(m.suggestItems) {
					m.suggestFocus = idx
					return m, m.acceptSuggest(idx)
				}
			}
		}
		// Wheel-scroll по меню — навигация фокусом.
		if m.suggestActive && len(m.suggestItems) > 0 {
			if msg.Button == tea.MouseButtonWheelUp {
				if m.suggestFocus > 0 {
					m.suggestFocus--
				}
				return m, nil
			}
			if msg.Button == tea.MouseButtonWheelDown {
				if m.suggestFocus < len(m.suggestItems)-1 {
					m.suggestFocus++
				}
				return m, nil
			}
		}
		// Меню НЕ активно — wheel должен скроллить viewport чата.
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			var c tea.Cmd
			m.viewport, c = m.viewport.Update(msg)
			return m, c
		}
		return m, nil

	case updateAvailableMsg:
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: msg.hint})
		m.refreshViewport()
		return m, nil

	case updateLatestMsg:
		m.statusMessage = "✓ execai " + agentVersion + " — последняя версия"
		return m, nil

	case loopTickMsg:
		if !m.loopActive {
			return m, nil
		}
		// Следующий тик планируем независимо — даже если сейчас стримит,
		// пропустим и проверим на следующем тике.
		nextTick := tea.Tick(m.loopInterval, func(time.Time) tea.Msg { return loopTickMsg{} })
		if m.streaming || m.approving || m.loginMode {
			return m, nextTick // пропускаем, агент занят
		}
		// Эмулируем юзерский ввод: добавляем в UI + запускаем агент.
		m.uiSegments = append(m.uiSegments, segment{kind: "user", body: "🔁 [loop] " + m.loopPrompt})
		m.refreshViewport()
		return m, tea.Batch(nextTick, m.startAgent(m.loopPrompt))

	case compactDoneMsg:
		if msg.err != nil {
			m.err = "compact: " + msg.err.Error()
			return m, nil
		}
		// Перестраиваем историю: system + summary + last tail.
		var sys llm.AIMessage
		if len(m.history) > 0 && m.history[0].Role == "system" {
			sys = m.history[0]
		}
		var tail []llm.AIMessage
		if len(m.history) > compactKeepTail {
			tail = m.history[len(m.history)-compactKeepTail:]
		}
		newHist := []llm.AIMessage{}
		if sys.Role != "" {
			newHist = append(newHist, sys)
		}
		newHist = append(newHist, llm.AIMessage{
			Role:    "assistant",
			Content: "[Сжатая история ранее (" + fmt.Sprintf("%d сообщений", msg.saved) + "): " + msg.summary + "]",
		})
		newHist = append(newHist, tail...)
		m.history = newHist
		// UI: добавим компактный hint про сжатие.
		m.uiSegments = append(m.uiSegments, segment{
			kind: "system_hint",
			body: fmt.Sprintf("📦 История сжата: %d сообщений → 1 summary (~%d симв.)", msg.saved, len(msg.summary)),
		})
		m.statusMessage = ""
		m.refreshViewport()
		return m, nil

	case agentLinkDoneMsg:
		// успешный логин из polling горутины
		return m.finishLogin(msg.creds)

	case agentLinkTickMsg:
		m.statusMessage = fmt.Sprintf("ждём подтверждения в браузере… (poll #%d)", msg.n)
		return m, nil

	case agentLinkErrMsg:
		m.err = "device-flow упал: " + msg.err.Error()
		m.statusMessage = ""
		m.pendingLink = nil
		m.refreshViewport()
		return m, nil

	case agentApproveAskMsg:
		m.approving = true
		m.approveTool = msg.name
		m.approveSummary = msg.summary
		m.approveReplyCh = msg.reply
		m.approveFocus = 0 // дефолт — "Раз"
		m.refreshViewport()
		return m, nil

	case agentDoneMsg:
		m.streaming = false
		// Финализируем последний поток текста, если был
		if m.streamBuf.Len() > 0 {
			m.uiSegments = append(m.uiSegments, segment{kind: "assistant", body: m.streamBuf.String()})
			m.streamBuf.Reset()
		}
		if m.reasonBuf.Len() > 0 {
			m.uiSegments = append(m.uiSegments, segment{kind: "reasoning", body: m.reasonBuf.String()})
			m.reasonBuf.Reset()
		}
		m.history = msg.history
		m.persistSession()
		// Autoloop: проверяем — не запросил ли AI пробуждение через schedule_wakeup tool.
		if req := tools.TakeScheduledWakeup(); req != nil {
			delay := time.Duration(req.DelaySeconds) * time.Second
			prompt := req.PromptOnWake
			if prompt == "" {
				prompt = "продолжай"
			}
			m.uiSegments = append(m.uiSegments, segment{
				kind: "system_hint",
				body: fmt.Sprintf("🌙 autoloop: пробуждение через %s (%s) → промт: %q", delay, req.Reason, prompt),
			})
			m.refreshViewport()
			return m, tea.Tick(delay, func(time.Time) tea.Msg {
				return autoWakeMsg{prompt: prompt}
			})
		}
		m.refreshViewport()
		return m, nil

	case autoWakeMsg:
		if m.streaming || m.approving || m.loginMode {
			// Если в данный момент юзер чем-то занят — пропускаем (AI потом сам себе пере-schedule).
			return m, nil
		}
		m.uiSegments = append(m.uiSegments, segment{kind: "user", body: "🌙 [autoloop] " + msg.prompt})
		m.refreshViewport()
		return m, m.startAgent(msg.prompt)

	case agentErrMsg:
		m.streaming = false
		if msg.err == context.Canceled {
			m.statusMessage = "(прервано)"
		} else {
			m.err = msg.err.Error()
			// 401 посреди сессии.
			// Стратегия: если device-flow уже был в этом запуске CLI —
			// НЕ авто-триггерим повторно. auth-vbai выдал валидный JWT,
			// api-vbai/aicore-vbai его не приняли — это env misconfig
			// (или user session banned), device-flow ничего не изменит.
			// Показываем понятную ошибку и предлагаем /login (ручной старт)
			// или /source на внешнюю подписку.
			errStr := msg.err.Error()
			if strings.Contains(errStr, "401") || strings.Contains(errStr, "токен истёк") {
				if m.subs == nil || m.subs.ActiveSubscription() == nil {
					if !m.lastLoginAt.IsZero() {
						// Сохраняем оригинальный errStr (уже содержит body ответа
						// от api-vbai) плюс подсказка что делать.
						m.err = errStr + "\n→ Смени /source zai|ollama|anthropic, или /login чтобы переподтвердить."
						m.streamBuf.Reset()
						m.reasonBuf.Reset()
						m.refreshViewport()
						return m, nil
					}
					// Первый 401 в сессии, без предыдущего логина — стартуем flow.
					_ = auth.Logout()
					m.creds = nil
					m.loginMode = true
					m.err = ""
					m.uiSegments = append(m.uiSegments, segment{kind: "system_hint",
						body: "Токен ExecAI истёк — стартую device-flow. Подтверди в браузере."})
					m.streamBuf.Reset()
					m.reasonBuf.Reset()
					m.refreshViewport()
					return m, func() tea.Msg { return autoStartLoginMsg{} }
				}
			}
		}
		m.streamBuf.Reset()
		m.reasonBuf.Reset()
		m.refreshViewport()
		return m, nil
	}

	// Paste-collapse: перехватываем большие Runes-вводы (Ctrl+V), заменяем
	// маркером в textarea. Полный текст расширится при submit.
	if key, ok := msg.(tea.KeyMsg); ok {
		if m.interceptPaste(key) {
			// Уже сами вставили маркер в textarea — не пропускаем в textarea.Update.
			var refCmd tea.Cmd = m.refreshSuggest()
			return m, refCmd
		}
	}
	var c1, c2 tea.Cmd
	m.textarea, c1 = m.textarea.Update(msg)
	m.viewport, c2 = m.viewport.Update(msg)
	// Пересчёт меню после изменения текста.
	var refCmd tea.Cmd
	if _, ok := msg.(tea.KeyMsg); ok {
		refCmd = m.refreshSuggest()
	}
	return m, tea.Batch(c1, c2, refCmd)
}

// refreshSuggest пересоберает список подсказок исходя из текущего значения
// textarea. Возвращает tea.Cmd при переходе active↔inactive — toggle мыши,
// чтобы текст-селект работал когда меню закрыто, а клик по меню — когда открыто.
func (m *tuiModel) refreshSuggest() tea.Cmd {
	wasActive := m.suggestActive
	defer func() {
		// Toggle мыши только на edge (active изменился).
	}()

	if m.streaming || m.loginMode || m.approving {
		m.suggestActive = false
		m.suggestItems = nil
	} else {
		raw := m.textarea.Value()
		line := raw
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		if !strings.HasPrefix(line, "/") {
			m.suggestActive = false
			m.suggestItems = nil
		} else {
			parts := strings.SplitN(line, " ", 2)
			cmd := parts[0]
			arg := ""
			if len(parts) == 2 {
				arg = parts[1]
			}
			var items []suggestItem
			switch {
			case len(parts) == 1:
				items = filterCommands(cmd)
			case cmd == "/model" || cmd == "/models":
				items = filterModels(m.models, arg)
			case cmd == "/resume":
				items = filterSessions(arg)
			case cmd == "/source":
				items = filterUseOptionsFor(m.subs, arg, "/source")
			case cmd == "/use":
				items = filterUseOptionsFor(m.subs, arg, "/use")
			case cmd == "/connect":
				items = filterConnectOptions(arg)
			case cmd == "/disconnect":
				items = filterDisconnectOptions(m.subs, arg)
			}
			m.suggestItems = items
			m.suggestActive = len(items) > 0
			if m.suggestFocus >= len(items) {
				m.suggestFocus = 0
			}
		}
	}

	_ = wasActive // мышь постоянно включена, toggle не нужен
	return nil
}

// acceptSuggest вставляет выбранный пункт в textarea (заменяя текущую строку).
// Если после вставки меню «сжалось» до самой себя (нет дальнейших argument'ов) —
// меню закрывается и мышь возвращается в native-режим.
//
// Эвристика auto-submit: insert БЕЗ trailing-space ('/help', '/source zai',
// '/model glm-5.2') → команда не ждёт аргумента → сразу сабмитим.
// С trailing-space ('/model ', '/source ', '/cd ') → ждём ввод аргумента.
func (m *tuiModel) acceptSuggest(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.suggestItems) {
		return nil
	}
	insert := m.suggestItems[idx].insert
	m.textarea.SetValue(insert)
	m.textarea.CursorEnd()
	if !strings.HasSuffix(insert, " ") {
		// Auto-submit: команда полностью готова, юзеру не нужно жать Enter ещё раз.
		closeCmd := m.closeSuggest()
		_, submitCmd := m.submitTextarea()
		return tea.Batch(closeCmd, submitCmd)
	}
	// Иначе — пересчёт меню (например /model → теперь модели).
	return m.refreshSuggest()
}

// submitTextarea — общий submit-флоу (как KeyEnter): берёт значение, кладёт
// в input-history, ресетит textarea, вызывает handleInput. Возвращает
// результат как (model, cmd) для дальнейшей цепочки.
func (m *tuiModel) submitTextarea() (tea.Model, tea.Cmd) {
	line := strings.TrimSpace(m.textarea.Value())
	if line == "" {
		return m, nil
	}
	if n := len(m.inputHistory); n == 0 || m.inputHistory[n-1] != line {
		m.inputHistory = append(m.inputHistory, line)
		if len(m.inputHistory) > 200 {
			m.inputHistory = m.inputHistory[len(m.inputHistory)-200:]
		}
		go saveInputHistory(line)
	}
	m.historyIndex = 0
	m.historyDraft = ""
	m.textarea.Reset()
	if m.loginMode {
		return m.handleLoginInput(line)
	}
	return m.handleInput(line)
}

// closeSuggest закрывает меню. Мышь не трогаем — она всегда захвачена.
func (m *tuiModel) closeSuggest() tea.Cmd {
	m.suggestActive = false
	m.suggestItems = nil
	return nil
}

// allCommands — каноничный список slash-команд для меню.
var allCommands = []suggestItem{
	{insert: "/help",        label: "/help",        hint: "список команд"},
	{insert: "/model ",      label: "/model",       hint: "выбрать модель"},
	{insert: "/usage",       label: "/usage",       hint: "баланс + траты по моделям"},
	{insert: "/compact",     label: "/compact",     hint: "сжать историю в summary"},
	{insert: "/log",         label: "/log",         hint: "последние LLM-запросы (model_requested vs model_returned)"},
	{insert: "/loop ",       label: "/loop",        hint: "повторять промт периодически (/loop 5m <текст>, /loop stop)"},
	{insert: "/effort",      label: "/effort",      hint: "открыть picker уровня рассуждения (off/low/medium/high/xhigh/max)"},
	{insert: "/max-iterations ", label: "/max-iterations", hint: "лимит tool-use итераций за ход (дефолт 40)"},
	{insert: "/subscriptions", label: "/subscriptions", hint: "подключенные провайдеры (Z.ai/Anthropic/OpenAI)"},
	{insert: "/connect zai ",        label: "/connect zai",        hint: "подключить Z.ai GLM Coding Plan (нужен api-key)"},
	{insert: "/connect kimi ",       label: "/connect kimi",       hint: "Kimi Code Coding Plan (K3/K2.7, ключ kimi.com/code/console)"},
	{insert: "/connect kimi-api ",   label: "/connect kimi-api",   hint: "Moonshot Platform pay-per-token (moonshot-v1/kimi-latest, ключ platform.moonshot.ai)"},
	{insert: "/connect anthropic ",  label: "/connect anthropic",  hint: "подключить Anthropic API (sk-ant-... из console.anthropic.com)"},
	{insert: "/connect openai ",     label: "/connect openai",     hint: "OpenAI Platform pay-per-token (sk-… из platform.openai.com)"},
	{insert: "/connect codex-cli",   label: "/connect codex-cli",  hint: "локальный OpenAI Codex CLI (квота ChatGPT Plus/Pro, без ключа)"},
	{insert: "/connect claude-cli",  label: "/connect claude-cli", hint: "локальный Claude Code (квота Pro/Max-подписки, без ключа)"},
	{insert: "/connect ollama",      label: "/connect ollama",     hint: "локальный Ollama runner (localhost:11434, без ключа)"},
	{insert: "/source ",       label: "/source",        hint: "переключить источник (execai/zai/...)"},
	{insert: "/mouse off",     label: "/mouse off",     hint: "отключить захват мыши → выделение работает"},
	{insert: "/mouse on",      label: "/mouse on",      hint: "включить захват (колесо/клики по меню)"},
	{insert: "/inline on",     label: "/inline on",     hint: "TUI без alt-screen (native scroll, но выделение сбрасывается при стриме)"},
	{insert: "/paste",         label: "/paste",         hint: "список вставок (Ctrl+V больших кусков), /paste show <N> — содержимое"},
	{insert: "/cd ",         label: "/cd",          hint: "сменить рабочую папку"},
	{insert: "/sessions",    label: "/sessions",    hint: "список бесед"},
	{insert: "/resume ",     label: "/resume",      hint: "продолжить беседу"},
	{insert: "/new",         label: "/new",         hint: "новая беседа"},
	{insert: "/clear",       label: "/clear",       hint: "очистить историю"},
	{insert: "/whoami",      label: "/whoami",      hint: "кто залогинен"},
	{insert: "/config",      label: "/config",      hint: "путь к config + api_base"},
	{insert: "/permissions", label: "/permissions", hint: "persistent allow-list"},
	{insert: "/login",       label: "/login",       hint: "device-flow логин"},
	{insert: "/logout",      label: "/logout",      hint: "выйти"},
	{insert: "/quit",        label: "/quit",        hint: "выход"},
}

func filterCommands(prefix string) []suggestItem {
	out := make([]suggestItem, 0, len(allCommands))
	for _, c := range allCommands {
		if strings.HasPrefix(c.label, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func filterModels(models []llm.Model, prefix string) []suggestItem {
	out := make([]suggestItem, 0, len(models))
	low := strings.ToLower(prefix)
	for _, mod := range models {
		if low == "" || strings.Contains(strings.ToLower(mod.ID), low) || strings.Contains(strings.ToLower(mod.Name), low) {
			out = append(out, suggestItem{
				insert: "/model " + mod.ID,
				label:  mod.ID,
				hint:   mod.Name + " · " + mod.Provider,
			})
		}
		if len(out) >= 50 {
			break
		}
	}
	return out
}

func filterSessions(prefix string) []suggestItem {
	list, _ := sessions.List()
	out := make([]suggestItem, 0, len(list))
	low := strings.ToLower(prefix)
	for i, s := range list {
		title := s.Title
		if title == "" {
			title = "(без названия)"
		}
		match := low == "" ||
			strings.Contains(strings.ToLower(title), low) ||
			strings.Contains(strings.ToLower(s.ID), low)
		if !match && low != "" {
			// Fuzzy-поиск по содержимому: подгружаем сессию и ищем substring в user/assistant.
			if full, err := sessions.Load(s.ID); err == nil {
				for _, msg := range full.Messages {
					body := strings.ToLower(llm.ContentText(msg.Content))
					if strings.Contains(body, low) {
						match = true
						break
					}
				}
			}
		}
		if match {
			out = append(out, suggestItem{
				insert: fmt.Sprintf("/resume %d", i+1),
				label:  fmt.Sprintf("#%d %s", i+1, title),
				hint:   s.UpdatedAt.Local().Format("2006-01-02 15:04"),
			})
		}
		if len(out) >= 20 {
			break
		}
	}
	return out
}

// ===== handleInput / commands =====

var oscEcho = regexp.MustCompile(`^\]\d+;[^\\]*\\?$`)

// handleLoginInput в loginMode:
//  - "y"/Enter (когда есть pending link) — открываем браузер.
//  - "n" / новый ввод — отказ открывать, ждём пока юзер сам откроет URL.
//  - вставленный JWT (длинная строка из eyJ…) — fallback на старый paste-token.
//  - иначе — стартуем новый device-link (один раз показываем подсказку).
func (m *tuiModel) handleLoginInput(line string) (tea.Model, tea.Cmd) {
	m.err = ""
	m.statusMessage = ""

	// Fallback: пользователь вставил полный JWT — старый paste-token flow.
	if looksLikeJWT(line) {
		m.statusMessage = "Проверяю токен (paste-mode)…"
		m.refreshViewport()
		cr, err := auth.Login(context.Background(), m.cfg, line)
		if err != nil {
			m.err = "не удалось авторизоваться: " + err.Error()
			m.statusMessage = ""
			m.refreshViewport()
			return m, nil
		}
		return m.finishLogin(cr)
	}

	// Управление device-flow когда уже есть pending link.
	if m.pendingLink != nil {
		switch strings.ToLower(line) {
		case "y", "д", "yes", "да", "o", "open":
			if err := auth.OpenBrowser(m.pendingLink.VerifyURI); err != nil {
				m.statusMessage = "не получилось открыть браузер автоматически — открой URL руками (см. выше)"
			} else {
				m.statusMessage = "открываю браузер…"
			}
			m.refreshViewport()
			return m, nil
		case "n", "no", "нет":
			m.statusMessage = "ОК, открой URL вручную и подтверди в браузере"
			m.refreshViewport()
			return m, nil
		}
		return m, nil
	}

	// Иначе — стартуем device-link.
	return m.startDeviceLink()
}

func (m *tuiModel) startDeviceLink() (tea.Model, tea.Cmd) {
	m.statusMessage = "Связываюсь с сервером…"
	m.refreshViewport()

	start, err := auth.StartAgentLink(context.Background(), m.cfg.APIBase)
	if err != nil {
		m.err = "не удалось получить link: " + err.Error()
		m.refreshViewport()
		return m, nil
	}
	m.pendingLink = start

	m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: fmt.Sprintf(
		"Открой в браузере и подтверди:\n\n  %s\n\nКод (на случай если ввод вручную): %s\n\nНажми Enter / [y] чтобы открыть браузер автоматически. [n] чтобы открыть руками.",
		start.VerifyURI, start.UserCode)})
	m.statusMessage = "ждём подтверждения в браузере…"
	m.refreshViewport()

	// Стартуем polling в горутине, отправляя tea.Msg в TUI.
	go func(s *auth.LinkStart) {
		tick := 0
		onTick := func() {
			tick++
			if teaProgramRef != nil {
				teaProgramRef.Send(agentLinkTickMsg{n: tick})
			}
		}
		cr, err := auth.WaitLinkUntilLinked(context.Background(), m.cfg, s,
			time.Duration(s.ExpiresIn)*time.Second, onTick)
		if err != nil {
			if teaProgramRef != nil {
				teaProgramRef.Send(agentLinkErrMsg{err: err})
			}
			return
		}
		if teaProgramRef != nil {
			teaProgramRef.Send(agentLinkDoneMsg{creds: cr})
		}
	}(start)
	return m, nil
}

// finishLogin — общий конец login flow: пробуем bootAfterLogin и
// перерисовываем экран как обычный chat.
func (m *tuiModel) finishLogin(cr *config.Credentials) (tea.Model, tea.Cmd) {
	m.creds = cr
	m.pendingLink = nil
	m.lastLoginAt = time.Now()
	if err := m.bootAfterLogin(); err != nil {
		m.err = "вход успешен, но не удалось загрузить агента: " + err.Error()
		m.refreshViewport()
		return m, nil
	}
	greet := fmt.Sprintf("Вошёл как %s.", cr.Email)
	if cr.Alias != "" {
		greet = fmt.Sprintf("Вошёл как %s · agent: %s", cr.Email, cr.Alias)
	}
	greet += fmt.Sprintf(" · %s/%s · /help — команды.", m.current.Provider, m.current.ID)
	m.uiSegments = nil; m.lastEmittedIdx = 0
	m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: greet})
	m.statusMessage = ""
	m.refreshViewport()
	return m, nil
}

func looksLikeJWT(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "eyJ") && strings.Count(s, ".") == 2
}

// looksLikeImagePath — true если строка начинается с "/" но содержит
// расширение картинки (.png/.jpg/etc) — значит это путь к файлу, а не команда.
var imageExtInLineRE = regexp.MustCompile(`(?i)\.(png|jpe?g|gif|webp)(\s|$|['"])`)

func looksLikeImagePath(line string) bool {
	return strings.HasPrefix(line, "/") && imageExtInLineRE.MatchString(line)
}

func (m *tuiModel) handleInput(line string) (tea.Model, tea.Cmd) {
	m.err = ""
	m.statusMessage = ""
	if oscEcho.MatchString(line) {
		return m, nil
	}
	if strings.HasPrefix(line, "/") && !looksLikeImagePath(line) {
		return m.handleCommand(line)
	}
	// Сообщение содержит путь к картинке, но ExtractImageAttachments не вытащил —
	// даём ВИДИМУЮ подсказку в чате (system_hint segment, не только статус-бар).
	if looksLikeImagePath(line) {
		if _, imgs := llm.ExtractImageAttachments(line); len(imgs) == 0 {
			m.uiSegments = append(m.uiSegments, segment{
				kind: "system_hint",
				body: "⚠ Путь к картинке не распознан (вероятно пробелы/кириллица в имени). " +
					"Оберни его в одинарные кавычки: '/home/yz/Pictures/Снимок экрана.png'",
			})
		}
	}
	if m.cli == nil {
		m.err = "не залогинен — /login"
		m.refreshViewport()
		return m, nil
	}
	// В user-сегменте оставляем маркеры [Pasted #N] для компактного отображения
	// в scrollback (иначе история засорится 47-строчными вставками). Агент
	// получает ПОЛНЫЙ текст через expandPasteMarkers.
	m.uiSegments = append(m.uiSegments, segment{kind: "user", body: line})
	m.refreshViewport()
	return m, m.startAgent(m.expandPasteMarkers(line))
}

func (m *tuiModel) handleCommand(line string) (tea.Model, tea.Cmd) {
	cmd := strings.TrimSpace(line)
	switch {
	case cmd == "/quit" || cmd == "/exit":
		return m, tea.Quit
	case cmd == "/clear" || cmd == "/reset":
		m.history = nil
		m.uiSegments = nil; m.lastEmittedIdx = 0
		m.statusMessage = "история очищена"
		m.refreshViewport()
		return m, nil
	case cmd == "/help" || cmd == "/?":
		help := "Команды:\n" +
			"  /model [num|id]    — список моделей или переключение\n" +
			"  /new               — новая беседа (текущая сохранена)\n" +
			"  /sessions, /list   — список сохранённых бесед\n" +
			"  /resume <num|id>   — открыть сохранённую беседу\n" +
			"  /title <text>      — переименовать текущую беседу\n" +
			"  /clear, Ctrl+L     — очистить историю текущей беседы\n" +
			"  /permissions       — persistent-разрешения tools\n" +
			"  /whoami            — текущий пользователь\n" +
			"  /config            — показать конфиг\n" +
			"  /logout            — выйти и заново попросить токен\n" +
			"  /quit, Ctrl+D      — выход\n\n" +
			"Хоткеи: Enter — отправить, Shift+Enter — новая строка, Ctrl+C — отменить стрим"
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: help})
		m.refreshViewport()
		return m, nil
	case cmd == "/paste" || strings.HasPrefix(cmd, "/paste "):
		arg := strings.TrimSpace(strings.TrimPrefix(cmd, "/paste"))
		if arg == "" || arg == "list" {
			var b strings.Builder
			if len(m.pasteStore) == 0 {
				b.WriteString("Вставок в этой сессии нет. Ctrl+V большой кусок текста → маркер.")
			} else {
				b.WriteString("Вставки (Ctrl+V ≥60 chars или c \\n):\n")
				ids := make([]int, 0, len(m.pasteStore))
				for id := range m.pasteStore {
					ids = append(ids, id)
				}
				sort.Ints(ids)
				for _, id := range ids {
					p := m.pasteStore[id]
					fmt.Fprintf(&b, "  #%d — %d lines, %d chars\n", p.id, p.lines, p.chars)
				}
				b.WriteString("\nПоказать: /paste show <N>")
			}
			m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: b.String()})
			m.refreshViewport()
			return m, nil
		}
		if strings.HasPrefix(arg, "show ") {
			idStr := strings.TrimSpace(strings.TrimPrefix(arg, "show"))
			var id int
			_, err := fmt.Sscanf(idStr, "%d", &id)
			if err != nil {
				m.err = "не число: " + idStr
				return m, nil
			}
			p, ok := m.pasteStore[id]
			if !ok {
				m.err = fmt.Sprintf("вставки #%d нет", id)
				return m, nil
			}
			body := fmt.Sprintf("=== Paste #%d (%d lines, %d chars) ===\n%s", p.id, p.lines, p.chars, p.text)
			m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: body})
			m.refreshViewport()
			return m, nil
		}
		m.err = "usage: /paste [list|show <N>]"
		return m, nil
	case cmd == "/whoami":
		if m.creds != nil {
			m.statusMessage = m.creds.Email + " @ " + m.cfg.APIBase
		} else {
			m.statusMessage = "(не залогинен — /login)"
		}
		return m, nil
	case cmd == "/config":
		dir, _ := config.Dir()
		body := fmt.Sprintf("config dir       : %s\napi_base         : %s\nselected_model   : %s",
			dir, m.cfg.APIBase, m.cfg.SelectedModelID)
		if m.creds != nil {
			body += "\nlogged in        : " + m.creds.Email
		}
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: body})
		m.refreshViewport()
		return m, nil
	case cmd == "/subscriptions" || cmd == "/subs" ||
		cmd == "/source" || cmd == "/use" || cmd == "/connect" || cmd == "/disconnect" ||
		strings.HasPrefix(cmd, "/connect ") || strings.HasPrefix(cmd, "/disconnect ") ||
		strings.HasPrefix(cmd, "/source ") || strings.HasPrefix(cmd, "/use "):
		_, msg := m.handleSubsCommand(cmd)
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: msg})
		m.refreshViewport()
		return m, nil
	case cmd == "/classic" || cmd == "/classic on" || cmd == "/classic off":
		// Toggle classic TUI (alt-screen + mouse). Дефолт — Ink-style
		// (история в scrollback, native selection/scroll).
		want := m.cfg.ClassicTUI
		switch cmd {
		case "/classic":
			want = !want
		case "/classic on":
			want = true
		case "/classic off":
			want = false
		}
		m.cfg.ClassicTUI = want
		if err := config.Save(m.cfg); err != nil {
			m.err = "config save: " + err.Error()
			return m, nil
		}
		if want {
			m.statusMessage = "✓ classic TUI ON — рестартни execai (/quit → execai). Alt-screen + прибитый статус-бар, Shift+drag для копирования."
		} else {
			m.statusMessage = "✓ Ink-style (default) — рестартни execai. История в scrollback, native selection и scroll."
		}
		return m, nil
	case cmd == "/mouse" || cmd == "/mouse off" || cmd == "/mouse on":
		// Toggle захвата мыши. Когда off — терминал отдаёт мышь под родное выделение
		// (для копирования). Когда on — мышь работает для скролла/клика по меню.
		// Без Shift+drag (если терминал не bypass'ит).
		if cmd == "/mouse" {
			cmd = "/mouse off" // /mouse без аргумента = выключить
		}
		if strings.HasSuffix(cmd, " off") {
			m.statusMessage = "🖱  захват мыши OFF — мышь выделяет текст, меню кликами не реагирует. Включить: /mouse on"
			return m, tea.DisableMouse
		}
		m.statusMessage = "🖱  захват мыши ON — колесо скроллит, клик по меню. Выделить текст: Shift+drag. Выкл: /mouse off"
		return m, tea.EnableMouseCellMotion
	case cmd == "/effort":
		// /effort без аргумента — открыть picker (если Shift+Tab не работает в твоём терминале).
		m.thinkingPickerActive = true
		m.thinkingPickerIdx = m.currentThinkingIdx()
		m.statusMessage = "effort picker: ←/→ выбрать, Enter подтвердить, Esc отмена"
		return m, nil
	case strings.HasPrefix(cmd, "/effort "):
		// /effort <off|low|medium|high|max|N>
		arg := strings.TrimSpace(strings.TrimPrefix(cmd, "/effort"))
		levels := map[string]int{
			"off": 0, "low": 1024, "medium": 4096, "high": 8192, "max": 32000,
		}
		if arg == "" {
			cur := "off"
			for name, n := range levels {
				if n == m.cfg.ThinkingBudget {
					cur = name
				}
			}
			m.uiSegments = append(m.uiSegments, segment{kind: "system_hint",
				body: fmt.Sprintf("Effort сейчас: %s (%d токенов)\nИзменить: /effort <off|low|medium|high|max>\n  off=0  low=1024  medium=4096  high=8192  max=32000\nРаботает для Anthropic-compat источников (Z.ai, Kimi, Anthropic, ollama-cloud, claude-cli).",
					cur, m.cfg.ThinkingBudget)})
			m.refreshViewport()
			return m, nil
		}
		budget, ok := levels[arg]
		if !ok {
			// Попробуем как число.
			if n, err := strconv.Atoi(arg); err == nil && n >= 0 {
				budget = n
			} else {
				m.err = "/effort <off|low|medium|high|max|N>"
				return m, nil
			}
		}
		m.cfg.ThinkingBudget = budget
		_ = config.Save(m.cfg)
		// Пересоздаём клиент чтобы новый бюджет применился.
		m.cli = m.makeLLMClient()
		m.statusMessage = fmt.Sprintf("✓ effort=%s (%d токенов)", arg, budget)
		return m, nil
	case cmd == "/max-iterations" || cmd == "/maxiter":
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint",
			body: fmt.Sprintf(
				"Max iterations сейчас: %d\nЛимит tool-use итераций за один ход. При исчерпании — мягкий stop, юзер может сказать 'продолжай'.\nИзменить: /max-iterations <N>  (рекомендуется 20-200; дефолт 40)",
				m.cfg.GetMaxIterations())})
		m.refreshViewport()
		return m, nil
	case strings.HasPrefix(cmd, "/max-iterations ") || strings.HasPrefix(cmd, "/maxiter "):
		arg := strings.TrimSpace(cmd)
		for _, p := range []string{"/max-iterations", "/maxiter"} {
			arg = strings.TrimSpace(strings.TrimPrefix(arg, p))
		}
		n, err := strconv.Atoi(arg)
		if err != nil || n <= 0 || n > 500 {
			m.err = "/max-iterations <N>  где N от 1 до 500 (рекомендуется 20-200)"
			return m, nil
		}
		m.cfg.MaxIterations = n
		_ = config.Save(m.cfg)
		m.statusMessage = fmt.Sprintf("✓ max-iterations = %d", n)
		return m, nil
	case cmd == "/loop":
		if m.loopActive {
			m.statusMessage = fmt.Sprintf("🔁 loop: каждые %s — %q. /loop stop чтобы остановить", m.loopInterval, m.loopPrompt)
		} else {
			m.statusMessage = "loop неактивен. Использование: /loop <интервал> <prompt>  (пример: /loop 5m проверь статус билда)"
		}
		return m, nil
	case cmd == "/loop stop" || cmd == "/loop off":
		if !m.loopActive {
			m.statusMessage = "loop и так не запущен"
			return m, nil
		}
		m.loopActive = false
		m.statusMessage = "🔁 loop остановлен"
		return m, nil
	case strings.HasPrefix(cmd, "/loop "):
		arg := strings.TrimSpace(strings.TrimPrefix(cmd, "/loop"))
		parts := strings.SplitN(arg, " ", 2)
		if len(parts) < 2 {
			m.err = "/loop <интервал> <prompt>  (например: /loop 5m проверь статус билда)"
			return m, nil
		}
		dur, err := time.ParseDuration(parts[0])
		if err != nil {
			m.err = "не парсится интервал " + parts[0] + " — нужно типа 30s, 5m, 1h"
			return m, nil
		}
		if dur < 5*time.Second {
			dur = 5 * time.Second
		}
		m.loopActive = true
		m.loopInterval = dur
		m.loopPrompt = parts[1]
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint",
			body: fmt.Sprintf("🔁 loop запущен: каждые %s — %q\nОстановить: /loop stop", dur, parts[1])})
		m.refreshViewport()
		// Первый тик — сразу, чтобы юзер увидел что работает.
		return m, tea.Tick(dur, func(time.Time) tea.Msg { return loopTickMsg{} })
	case cmd == "/log" || cmd == "/logs":
		// Последние 20 строк ~/.config/execai/requests.log — кто/что/какой источник.
		dir, _ := config.Dir()
		path := filepath.Join(dir, "requests.log")
		data, err := os.ReadFile(path)
		if err != nil {
			m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: "лога ещё нет: " + path})
			m.refreshViewport()
			return m, nil
		}
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if len(lines) > 20 {
			lines = lines[len(lines)-20:]
		}
		var b strings.Builder
		fmt.Fprintf(&b, "📜 Последние %d запросов (%s):\n", len(lines), path)
		for _, ln := range lines {
			var e struct {
				Ts             string `json:"ts"`
				Source         string `json:"source"`
				ModelRequested string `json:"model_requested"`
				ModelReturned  string `json:"model_returned"`
				Status         string `json:"status"`
				ContentLen     int    `json:"content_len"`
				ToolCalls      int    `json:"tool_calls"`
				Err            string `json:"err"`
			}
			if json.Unmarshal([]byte(ln), &e) != nil {
				continue
			}
			ts := e.Ts
			if len(ts) >= 19 {
				ts = ts[11:19]
			}
			mark := "✓"
			if e.Status != "ok" {
				mark = "✗"
			}
			ret := ""
			if e.ModelReturned != "" && e.ModelReturned != e.ModelRequested {
				ret = " → returned: " + e.ModelReturned
			} else if e.ModelReturned == e.ModelRequested {
				ret = " ✓ confirmed"
			}
			fmt.Fprintf(&b, "  %s %s  %s · %s%s  · %db · tools:%d", mark, ts, e.Source, e.ModelRequested, ret, e.ContentLen, e.ToolCalls)
			if e.Err != "" {
				fmt.Fprintf(&b, " · err: %s", e.Err)
			}
			b.WriteString("\n")
		}
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: b.String()})
		m.refreshViewport()
		return m, nil
	case cmd == "/compact":
		if m.cli == nil {
			m.err = "не залогинен — /login"
			return m, nil
		}
		m.statusMessage = "сжимаю историю…"
		fn := m.compactCmd()
		return m, func() tea.Msg {
			return fn()
		}
	case cmd == "/usage":
		if m.creds == nil {
			m.err = "не залогинен — /login"
			return m, nil
		}
		text, err := fetchUsageForSource(m.cfg.APIBase, m.creds.Token, m.subs)
		if err != nil {
			m.err = "не удалось получить usage: " + err.Error()
			return m, nil
		}
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: text})
		m.refreshViewport()
		return m, nil
	case strings.HasPrefix(cmd, "/cd "):
		target := strings.TrimSpace(strings.TrimPrefix(cmd, "/cd"))
		if strings.HasPrefix(target, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				target = home + target[1:]
			}
		}
		if target == "" {
			target = os.Getenv("HOME")
		}
		st, err := os.Stat(target)
		if err != nil || !st.IsDir() {
			m.err = "не папка: " + target
			return m, nil
		}
		if err := os.Chdir(target); err != nil {
			m.err = "chdir: " + err.Error()
			return m, nil
		}
		m.cwd = shortenCWD(target)
		// Пересоздать registry чтоб tools знали новый cwd.
		m.registry = tools.Default(target)
		m.statusMessage = "cwd → " + target
		return m, nil
	case cmd == "/cd":
		m.statusMessage = "cwd сейчас: " + m.cwd
		return m, nil
	case cmd == "/logout":
		_ = auth.Logout()
		m.creds = nil
		m.cli = nil
		m.loginMode = true
		m.pendingLink = nil
		m.history = nil
		m.uiSegments = nil; m.lastEmittedIdx = 0
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint",
			body: "Вышел. Стартую device-flow для нового входа…"})
		m.textarea.Placeholder = "Подтверди вход в браузере"
		m.statusMessage = ""
		m.refreshViewport()
		return m, func() tea.Msg { return autoStartLoginMsg{} }
	case cmd == "/new":
		m.history = nil
		m.uiSegments = nil; m.lastEmittedIdx = 0
		m.session = sessions.New(m.current.ID, m.current.Provider, getCWDForBoot())
		m.statusMessage = "новая беседа"
		m.refreshViewport()
		return m, nil
	case cmd == "/sessions" || cmd == "/list":
		list, _ := sessions.List()
		var b strings.Builder
		b.WriteString("\nСохранённые беседы (свежие сверху):\n")
		for i, s := range list {
			cur := " "
			if m.session != nil && s.ID == m.session.ID {
				cur = "•"
			}
			fmt.Fprintf(&b, "%s %2d. %s  [%s/%s]  %s\n",
				cur, i+1, s.Title, s.Provider, s.Model, s.UpdatedAt.Local().Format("2006-01-02 15:04"))
		}
		if len(list) == 0 {
			b.WriteString("(пусто)\n")
		}
		b.WriteString("\nПереключиться: /resume <номер|id>")
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: b.String()})
		m.refreshViewport()
		return m, nil
	case strings.HasPrefix(cmd, "/resume "):
		arg := strings.TrimSpace(strings.TrimPrefix(cmd, "/resume"))
		list, _ := sessions.List()
		var picked *sessions.Session
		if n, err := strconv.Atoi(arg); err == nil {
			if n >= 1 && n <= len(list) {
				picked = list[n-1]
			}
		} else {
			for _, s := range list {
				if s.ID == arg || strings.HasPrefix(s.ID, arg) {
					picked = s
					break
				}
			}
		}
		if picked == nil {
			m.err = fmt.Sprintf("беседа %q не найдена. /sessions — список", arg)
			return m, nil
		}
		// Перезагружаем
		full, err := sessions.Load(picked.ID)
		if err != nil {
			m.err = "не удалось загрузить: " + err.Error()
			return m, nil
		}
		m.session = full
		m.history = append([]llm.AIMessage(nil), full.Messages...)
		m.uiSegments = nil; m.lastEmittedIdx = 0
		m.replayHistoryToUI()
		m.statusMessage = "продолжаем: " + full.Title
		m.refreshViewport()
		return m, nil
	case strings.HasPrefix(cmd, "/title "):
		t := strings.TrimSpace(strings.TrimPrefix(cmd, "/title"))
		if t == "" || m.session == nil {
			return m, nil
		}
		m.session.Title = t
		_ = m.session.Save()
		m.statusMessage = "переименовано: " + t
		return m, nil
	case cmd == "/login":
		m.creds = nil
		m.cli = nil
		m.loginMode = true
		m.pendingLink = nil
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint",
			body: "Стартую device-flow для нового входа…"})
		m.refreshViewport()
		return m, func() tea.Msg { return autoStartLoginMsg{} }
	case cmd == "/permissions" || cmd == "/perms":
		perms, _ := agent.LoadPermissions()
		var b strings.Builder
		b.WriteString("\nPersistent permissions (~/.config/execai/permissions.json):\n")
		if perms != nil && len(perms.Tools) > 0 {
			b.WriteString("  always allowed tools: " + strings.Join(perms.Tools, ", ") + "\n")
		} else {
			b.WriteString("  always allowed tools: (пусто)\n")
		}
		if perms != nil && len(perms.Exact) > 0 {
			b.WriteString(fmt.Sprintf("  always allowed exact commands: %d записей\n", len(perms.Exact)))
		}
		b.WriteString("\nЧтобы сбросить — удали файл вручную или запусти: rm ~/.config/execai/permissions.json")
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: b.String()})
		m.refreshViewport()
		return m, nil
	case cmd == "/model" || cmd == "/models":
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: m.modelListText()})
		m.refreshViewport()
		return m, nil
	case strings.HasPrefix(cmd, "/model "):
		arg := strings.TrimSpace(strings.TrimPrefix(cmd, "/model"))
		next := pickByArg(m.models, arg)
		if next == nil {
			m.err = fmt.Sprintf("модель %q не найдена", arg)
			return m, nil
		}
		m.current = *next
		m.cfg.SelectedModelID = next.ID
		_ = config.Save(m.cfg)
		m.cli = m.makeLLMClient()
		m.statusMessage = fmt.Sprintf("переключено на %s/%s — %s (история сохранена)", next.Provider, next.ID, next.Name)
		return m, tea.SetWindowTitle(m.titleString())
	}
	m.err = "неизвестная команда: " + cmd
	return m, nil
}

// ===== agent loop in goroutine =====

func (m *tuiModel) startAgent(userMessage string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.streaming = true

	prog := teaProgramRef // глобальная ссылка, выставляется ниже в Run

	st := &tuiStreamer{prog: prog}
	ap := &tuiApprover{prog: prog}

	a := agent.New(m.cli, m.registry, m.system, ap, st)
	a.MaxIterations = m.cfg.GetMaxIterations()

	hist := append([]llm.AIMessage(nil), m.history...)

	go func() {
		out, err := a.Run(ctx, hist, userMessage)
		if err != nil {
			prog.Send(agentErrMsg{err: err})
			return
		}
		prog.Send(agentDoneMsg{history: out})
	}()

	return m.spinner.Tick
}

// ===== approve =====

func (m *tuiModel) replyApprove(d agent.ApproveDecision) {
	tool := m.approveTool
	if m.approveReplyCh != nil {
		select {
		case m.approveReplyCh <- d:
		default:
		}
	}
	m.approving = false
	m.approveTool = ""
	m.approveSummary = ""
	m.approveReplyCh = nil
	switch d {
	case agent.ApproveDeny:
		m.statusMessage = "отклонено"
	case agent.ApproveTool:
		m.statusMessage = "разрешено: все вызовы " + tool + " в этой сессии"
	case agent.ApproveExactArgs:
		m.statusMessage = "разрешено: эта команда в этой сессии"
	case agent.ApproveAlways:
		m.statusMessage = "разрешено НАВСЕГДА: " + tool + " (записано в ~/.config/execai/permissions.json)"
	}
	m.refreshViewport()
}

func approveDecisionFromFocus(f int) agent.ApproveDecision {
	switch f {
	case 0:
		return agent.ApproveOnce
	case 1:
		return agent.ApproveTool
	case 2:
		return agent.ApproveExactArgs
	case 3:
		return agent.ApproveAlways
	default:
		return agent.ApproveDeny
	}
}

// ===== render =====

// refreshViewport — в classic (alt-screen) режиме перерисовывает viewport
// целиком. В Ink-style (дефолт) — эмитит все НОВЫЕ сегменты с прошлого
// вызова в терминальный scrollback через Program.Println. Live-стрим
// рендерится в View() и в scrollback не попадает — попадёт только когда
// финализируется как сегмент.
func (m *tuiModel) refreshViewport() {
	if !m.cfg.ClassicTUI {
		// Ink-style: печатаем свежие сегменты в scrollback.
		for i := m.lastEmittedIdx; i < len(m.uiSegments); i++ {
			m.emitSegment(m.uiSegments[i])
		}
		m.lastEmittedIdx = len(m.uiSegments)
		return
	}
	var b strings.Builder
	for _, s := range m.uiSegments {
		b.WriteString(m.renderSegment(s))
	}
	if m.toolBuf.Len() > 0 {
		b.WriteString(m.renderSegment(segment{kind: "tool_result", tool: m.toolName, body: m.toolBuf.String()}))
	}
	if m.reasonBuf.Len() > 0 {
		b.WriteString(m.renderSegment(segment{kind: "reasoning", body: m.reasonBuf.String()}))
	}
	if m.streamBuf.Len() > 0 {
		b.WriteString(m.renderSegment(segment{kind: "assistant", body: m.streamBuf.String()}))
	}
	if m.streaming && m.streamBuf.Len() == 0 && m.reasonBuf.Len() == 0 && m.toolBuf.Len() == 0 {
		b.WriteString("\n" + styleAssistPrefix.Render("● ") + m.spinner.View() + "\n")
	}
	m.viewport.SetContent(b.String())
	m.viewport.GotoBottom()
}

// pasteThresholdRunes — минимальный размер вставки для коллапса.
// Меньше — считаем обычным вводом. Также срабатывает если есть \n.
const pasteThresholdRunes = 60

// interceptPaste — если msg это большой ввод текста (Ctrl+V из буфера или
// bracketed paste от терминала), сохраняем содержимое в pasteStore и
// вставляем в textarea короткий маркер вместо всего текста. Возвращает true
// если перехватили — тогда caller не должен пропускать msg в textarea.Update.
func (m *tuiModel) interceptPaste(msg tea.KeyMsg) bool {
	// Работает только для Runes-ввода (KeyRunes = обычный текст).
	if msg.Type != tea.KeyRunes {
		return false
	}
	runes := msg.Runes
	if len(runes) == 0 {
		return false
	}
	// Порог: либо длинный кусок, либо есть перенос строки внутри одного ввода.
	hasNewline := false
	for _, r := range runes {
		if r == '\n' || r == '\r' {
			hasNewline = true
			break
		}
	}
	if len(runes) < pasteThresholdRunes && !hasNewline {
		return false
	}
	// Сохраняем и вставляем маркер.
	text := string(runes)
	m.pasteCounter++
	lines := strings.Count(text, "\n") + 1
	marker := fmt.Sprintf("[Pasted #%d — %d lines, %d chars]", m.pasteCounter, lines, len(text))
	m.pasteStore[m.pasteCounter] = pasteEntry{
		id:     m.pasteCounter,
		text:   text,
		lines:  lines,
		chars:  len(text),
		marker: marker,
	}
	m.textarea.InsertString(marker)
	return true
}

// expandPasteMarkers — заменяет [Pasted #N ...] маркеры на реальные тексты
// из pasteStore. Используется при отправке инпута агенту.
func (m *tuiModel) expandPasteMarkers(input string) string {
	if len(m.pasteStore) == 0 {
		return input
	}
	for _, p := range m.pasteStore {
		input = strings.ReplaceAll(input, p.marker, p.text)
	}
	return input
}

// emitQueue + drainer — Program.Println блокирующе шлёт в p.msgs; если
// вызвать из Update(), deadlock (event loop не читает пока Update идёт).
// Поэтому шлём через буфер, а один goroutine последовательно вызывает
// Println уже вне Update-контекста. Порядок сохраняется (channel FIFO).
var emitQueue = make(chan string, 256)
var emitStarted sync.Once

// emitSegment — в Ink-style кладёт сегмент в очередь на Program.Println.
// В classic — no-op.
func (m *tuiModel) emitSegment(s segment) {
	if m.cfg.ClassicTUI {
		return
	}
	rendered := strings.TrimRight(m.renderSegment(s), "\n")
	if rendered == "" {
		return
	}
	if teaProgramRef == nil {
		return
	}
	emitStarted.Do(func() {
		go func() {
			for r := range emitQueue {
				if teaProgramRef != nil {
					teaProgramRef.Println(r)
				}
			}
		}()
	})
	// Неблокирующая отправка: если буфер полон, дропаем (крайне маловероятно
	// на 256 строк, но лучше уронить один сегмент чем зависнуть весь TUI).
	select {
	case emitQueue <- rendered:
	default:
	}
}

func (m *tuiModel) renderSegment(s segment) string {
	var b strings.Builder
	switch s.kind {
	case "user":
		b.WriteString("\n")
		b.WriteString(styleUserPrompt.Render("› "))
		b.WriteString(s.body)
		b.WriteString("\n")
	case "assistant":
		b.WriteString("\n")
		b.WriteString(styleAssistPrefix.Render("● "))
		b.WriteString("\n")
		b.WriteString(m.renderMD(s.body))
	case "reasoning":
		b.WriteString("\n")
		b.WriteString(styleReasoning.Render("· (reasoning) " + truncate(s.body, 400)))
		b.WriteString("\n")
	case "tool_call":
		b.WriteString("\n")
		b.WriteString(styleToolPrefix.Render("▶ " + s.tool))
		b.WriteString("  ")
		b.WriteString(styleToolBody.Render(truncate(s.body, 200)))
		b.WriteString("\n")
	case "tool_result":
		// результат компактно, ограничим
		body := truncate(s.body, 800)
		b.WriteString(styleToolBody.Render("  " + strings.ReplaceAll(body, "\n", "\n  ")))
		b.WriteString("\n")
	case "system_hint":
		b.WriteString("\n")
		b.WriteString(styleSysHint.Render(s.body))
		b.WriteString("\n")
	}
	return b.String()
}

func (m *tuiModel) renderMD(s string) string {
	if m.renderer == nil || s == "" {
		return s
	}
	out, err := m.renderer.Render(s)
	if err != nil {
		return s
	}
	return out
}

// ===== view =====

func (m *tuiModel) View() string {
	headerName := m.current.Name
	if headerName == "" {
		headerName = "login"
	}
	header := styleHeader.Render(fmt.Sprintf(" execai · %s ", headerName))

	statusKV := func(k, v string) string {
		return styleStatusKey.Render(k+":") + styleStatus.Render(v)
	}
	userLabel := "(не залогинен)"
	if m.creds != nil {
		userLabel = m.creds.Email
		if m.creds.Alias != "" {
			userLabel = m.creds.Email + " · " + m.creds.Alias
		}
	}
	provider := m.current.Provider
	model := m.current.ID
	if provider == "" {
		provider = "—"
	}
	if model == "" {
		model = "—"
	}
	source := "ExecAI"
	if m.subs != nil {
		source = m.subs.SourceLabel()
	}
	items := []string{
		statusKV(" src ", " "+source+" "),
		statusKV(" provider ", " "+provider+" "),
		statusKV(" model ", " "+model+" "),
	}
	if m.cfg.ThinkingBudget > 0 {
		levels := map[int]string{1024: "low", 4096: "medium", 8192: "high", 16384: "xhigh", 32000: "max"}
		lab := levels[m.cfg.ThinkingBudget]
		if lab == "" {
			lab = fmt.Sprintf("%d", m.cfg.ThinkingBudget)
		}
		items = append(items, statusKV(" effort ", " "+lab+" "))
	}
	if m.loopActive {
		items = append(items, statusKV(" 🔁 loop ", " "+m.loopInterval.String()+" "))
	}
	items = append(items,
		statusKV(" user ", " "+userLabel+" "),
		statusKV(" cwd ", " "+m.cwd+" "),
	)
	if m.streaming {
		items = append([]string{styleStatusKey.Render(" ⏵ working ")}, items...)
	}
	status := styleStatus.Width(m.width).MaxHeight(1).Render(strings.Join(items, " "))

	var bottom strings.Builder
	if m.approving {
		bottom.WriteString(m.renderApproveBlock())
		bottom.WriteString("\n")
	}
	if m.err != "" {
		bottom.WriteString(styleErr.Render("✗ " + m.err))
		bottom.WriteString("\n")
	} else if m.statusMessage != "" {
		bottom.WriteString(styleSysHint.Render(m.statusMessage))
		bottom.WriteString("\n")
	}

	help := styleHelpDim.MaxHeight(1).MaxWidth(m.width).Render(
		styleHelpKey.Render("Enter") + " send · " +
			styleHelpKey.Render("↑↓") + " history · " +
			styleHelpKey.Render("Shift+Tab") + " effort · " +
			styleHelpKey.Render("Shift+drag") + " copy · " +
			styleHelpKey.Render("/help") + " · " +
			styleHelpKey.Render("Ctrl+C") + " stop · " +
			styleHelpKey.Render("Ctrl+D") + " exit")

	// Меню подсказок (autocomplete) — над textarea если активно.
	suggestPanel := m.renderSuggestPanel()
	thinkingSlider := m.renderThinkingSlider()

	// Ink-style: viewport НЕ рендерим (история в scrollback), только
	// живой стрим текущего ответа (если есть) + input + статус.
	var liveArea string
	if !m.cfg.ClassicTUI {
		var live strings.Builder
		if m.toolBuf.Len() > 0 {
			live.WriteString(m.renderSegment(segment{kind: "tool_result", tool: m.toolName, body: m.toolBuf.String()}))
		}
		if m.reasonBuf.Len() > 0 {
			live.WriteString(m.renderSegment(segment{kind: "reasoning", body: m.reasonBuf.String()}))
		}
		if m.streamBuf.Len() > 0 {
			live.WriteString(m.renderSegment(segment{kind: "assistant", body: m.streamBuf.String()}))
		}
		if m.streaming && m.streamBuf.Len() == 0 && m.reasonBuf.Len() == 0 && m.toolBuf.Len() == 0 {
			live.WriteString(styleAssistPrefix.Render("● ") + m.spinner.View() + "\n")
		}
		liveArea = live.String()
	} else {
		liveArea = m.viewport.View()
	}
	parts := []string{
		header,
		liveArea,
		bottom.String() + suggestPanel + m.textarea.View(),
		status,
	}
	if thinkingSlider != "" {
		parts = append(parts, thinkingSlider)
	}
	parts = append(parts, help)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// thinkingLevels — позиции effort-слайдера. Faster ←→ Smarter.
var thinkingLevels = []struct {
	budget int
	label  string
}{
	{0, "off"},
	{1024, "low"},
	{4096, "medium"},
	{8192, "high"},
	{16384, "xhigh"},
	{32000, "max"},
}

// currentThinkingIdx — индекс текущего бюджета в thinkingLevels (-1 если custom).
func (m *tuiModel) currentThinkingIdx() int {
	for i, lvl := range thinkingLevels {
		if lvl.budget == m.cfg.ThinkingBudget {
			return i
		}
	}
	return 0
}

// renderThinkingSlider — компактный одно-строчный индикатор (когда picker закрыт)
// или полноценный модальный picker (когда thinkingPickerActive). Только для
// Anthropic-compat подписок.
func (m *tuiModel) renderThinkingSlider() string {
	if m.subs == nil {
		return ""
	}
	// Thinking поддерживают источники на базе AnthropicClient или Claude CLI.
	if !sourceSupportsThinking(m.subs) {
		return ""
	}
	if m.thinkingPickerActive {
		return m.renderThinkingPicker()
	}
	// Компактная строка-индикатор когда picker закрыт.
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#777"))
	cur := m.currentThinkingIdx()
	return dim.Render(fmt.Sprintf("🧠 effort: %s   (Shift+Tab чтобы поменять)", thinkingLevels[cur].label))
}

// renderThinkingPicker — модальный effort-picker в стиле Claude Code.
func (m *tuiModel) renderThinkingPicker() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#777"))
	active := lipgloss.NewStyle().Foreground(lipgloss.Color("#A8FF60")).Bold(true)
	normal := lipgloss.NewStyle().Foreground(lipgloss.Color("#CCC"))
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5F5FD7")).
		Padding(0, 1).
		MaxWidth(m.width - 2)

	// Строка 1: подпись Faster / Smarter
	faster := dim.Render("Faster")
	smarter := dim.Render("Smarter")
	// Строка 2: позиции (current — зелёная жирная, прочие — серые)
	parts := []string{}
	for i, lvl := range thinkingLevels {
		if i == m.thinkingPickerIdx {
			parts = append(parts, active.Render(lvl.label))
		} else {
			parts = append(parts, normal.Render(lvl.label))
		}
	}
	row := strings.Join(parts, "  ")
	// Указатель ▲ под текущей позицией
	cursor := ""
	for i, lvl := range thinkingLevels {
		var seg string
		if i == m.thinkingPickerIdx {
			seg = strings.Repeat(" ", (len(lvl.label)-1)/2) + "▲" + strings.Repeat(" ", len(lvl.label)/2)
		} else {
			seg = strings.Repeat(" ", len(lvl.label))
		}
		if i > 0 {
			seg = "  " + seg
		}
		cursor += seg
	}
	footer := dim.Render("←/→ adjust · Enter confirm · Esc cancel · Shift+Tab next")
	body := title.Render("Effort") + "    " + faster + strings.Repeat(" ", max(0, m.width-30-lipgloss.Width(faster)-lipgloss.Width(smarter))) + smarter + "\n" +
		row + "\n" +
		active.Render(cursor) + "\n" +
		footer
	return box.Render(body)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// renderSuggestPanel — компактное меню под list-стиль Claude Code.
// Возвращает пустую строку если меню не активно.
func (m *tuiModel) renderSuggestPanel() string {
	if !m.suggestActive || len(m.suggestItems) == 0 {
		return ""
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5F5FD7")).
		Padding(0, 1).
		MaxWidth(m.width - 2)
	focused := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#5F5FD7")).
		Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#777"))
	normal := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))

	maxRows := 8
	if len(m.suggestItems) < maxRows {
		maxRows = len(m.suggestItems)
	}
	// Окно прокрутки вокруг focus.
	start := 0
	if m.suggestFocus >= maxRows {
		start = m.suggestFocus - maxRows + 1
	}
	end := start + maxRows
	if end > len(m.suggestItems) {
		end = len(m.suggestItems)
	}

	var lines []string
	for i := start; i < end; i++ {
		it := m.suggestItems[i]
		label := it.label
		hint := ""
		if it.hint != "" {
			hint = "  " + dim.Render(it.hint)
		}
		row := label + hint
		if i == m.suggestFocus {
			row = focused.Render(" " + label + " ") + hint
		} else {
			row = normal.Render("  " + label) + hint
		}
		lines = append(lines, row)
	}
	footer := dim.Render(fmt.Sprintf(" %d/%d · ↑↓ выбор · Tab/Enter подтвердить · Esc закрыть",
		m.suggestFocus+1, len(m.suggestItems)))
	body := strings.Join(lines, "\n") + "\n" + footer
	return box.Render(body) + "\n"
}

// ===== helpers =====

// renderApproveBlock — диалог с кнопками над textarea. Стрелки/Tab переключают
// фокус, Enter — выбор, Esc — отклонить. Также горячие клавиши y/a/s/f/n.
func (m *tuiModel) renderApproveBlock() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FFD75F")).
		Foreground(lipgloss.Color("#FAFAFA")).
		Padding(0, 1).
		Width(m.width - 2)

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFD75F")).
		Render(fmt.Sprintf("⚠  Подтвердите запуск %s", m.approveTool))

	body := m.approveSummary
	if len(body) > m.width*8 {
		body = body[:m.width*8] + "…"
	}
	cmd := lipgloss.NewStyle().Foreground(lipgloss.Color("#A8FF60")).Render(body)

	buttons := []struct{ label, hotkey string }{
		{"Раз", "y"},
		{"Весь " + m.approveTool + " в сессии", "a"},
		{"Эту команду в сессии", "s"},
		{"НАВСЕГДА", "f"},
		{"Отклонить", "n"},
	}
	btnDefault := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#444")).
		Foreground(lipgloss.Color("#CCC")).
		Padding(0, 1)
	btnFocus := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FFD75F")).
		Foreground(lipgloss.Color("#000")).
		Background(lipgloss.Color("#FFD75F")).
		Bold(true).
		Padding(0, 1)
	rendered := make([]string, 0, len(buttons))
	for i, b := range buttons {
		text := fmt.Sprintf("[%s] %s", b.hotkey, b.label)
		if i == m.approveFocus {
			rendered = append(rendered, btnFocus.Render(text))
		} else {
			rendered = append(rendered, btnDefault.Render(text))
		}
	}
	row := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)

	hint := styleHelpDim.Render("← → или Tab — переключение, Enter — выбор, Esc — отклонить")

	content := title + "\n\n" + cmd + "\n\n" + row + "\n" + hint
	return box.Render(content)
}

// bootAfterLogin тянет models, выбирает default, инициализирует cli.
// Вызывается либо при старте если creds есть, либо после успешного login.
func (m *tuiModel) bootAfterLogin() error {
	// Cache-first: агент ВСЕГДА стартует, даже если сеть/API/models_public
	// временно недоступны. Если сеть отвалилась — возьмём кеш; если и его
	// нет — встроенная заглушка (одна модель), юзер увидит warning в чате.
	res := llm.FetchModelsCached(m.cfg.APIBase, m.creds.Token)
	models := res.Models
	if notice := res.OfflineNotice(); notice != "" {
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: notice})
	}
	if len(models) == 0 {
		// Не должно случиться — embeddedFallback гарантирует >=1. На всякий случай.
		return fmt.Errorf("не смогли ни получить, ни собрать fallback-список моделей")
	}
	current := llm.PickDefault(models, m.cfg.SelectedModelID)
	if m.cfg.SelectedModelID != current.ID {
		m.cfg.SelectedModelID = current.ID
		_ = config.Save(m.cfg)
	}
	// Гарантируем что папка user-памяти и MEMORY.md-индекс существуют
	// перед подгрузкой — тогда LLM всегда видит куда писать.
	_, _ = agent.EnsureUserMemoryIndex()
	memory := agent.LoadMemory(getCWDForBoot())
	m.system = agent.SystemPrompt(getCWDForBoot(), m.registry.Names(), memory)
	m.models = models
	m.execAIModels = models
	m.current = *current
	m.loginMode = false
	// Если активна внешняя подписка — переопределяем m.models/m.current под её каталог.
	m.applySubscriptionSource()
	m.textarea.Placeholder = "Опиши задачу. Enter — отправить, /help — команды."

	// Восстановление последней сессии: если есть свежая (<24ч) и в той же
	// CWD — продолжаем её, иначе создаём новую.
	cwd := getCWDForBoot()
	if last := sessions.MostRecent(); last != nil && last.CWD == cwd && time.Since(last.UpdatedAt) < 24*time.Hour {
		m.session = last
		m.history = append([]llm.AIMessage(nil), last.Messages...)
		m.replayHistoryToUI()
	} else {
		m.session = sessions.New(current.ID, current.Provider, cwd)
	}
	return nil
}

// replayHistoryToUI — нарисовать сегменты для уже существующей истории
// (после resume сессии) — чтобы юзер видел контекст.
func (m *tuiModel) replayHistoryToUI() {
	for _, msg := range m.history {
		switch msg.Role {
		case "user":
			m.uiSegments = append(m.uiSegments, segment{kind: "user", body: llm.ContentText(msg.Content)})
		case "assistant":
			body := llm.ContentText(msg.Content)
			if body != "" {
				m.uiSegments = append(m.uiSegments, segment{kind: "assistant", body: body})
			}
			for _, tc := range msg.ToolCalls {
				b := summaryForArgs(tc.Function.Name, json.RawMessage(tc.Function.Arguments))
				m.uiSegments = append(m.uiSegments, segment{kind: "tool_call", tool: tc.Function.Name, body: b})
			}
		case "tool":
			m.uiSegments = append(m.uiSegments, segment{kind: "tool_result", tool: msg.Name, body: llm.ContentText(msg.Content)})
		}
	}
}

func getCWDForBoot() string {
	cwd, _ := os.Getwd()
	return cwd
}

// persistSession записывает текущую history в файл сессии. Вызывается после
// каждого agentDoneMsg и при переключении модели.
func (m *tuiModel) persistSession() {
	if m.session == nil {
		m.session = sessions.New(m.current.ID, m.current.Provider, getCWDForBoot())
	}
	m.session.Model = m.current.ID
	m.session.Provider = m.current.Provider
	m.session.Messages = append([]llm.AIMessage(nil), m.history...)
	_ = m.session.Save()
}

func (m *tuiModel) welcomeText() string {
	return fmt.Sprintf("execai %s · %s/%s · %s · %s\nВведите задачу. /model — модели, /help — команды, /quit — выход.",
		agentVersion, m.current.Provider, m.current.ID, m.creds.Email, m.cwd)
}

func (m *tuiModel) modelListText() string {
	var b strings.Builder
	maxName, maxID := 0, 0
	for _, mm := range m.models {
		if l := lipgloss.Width(mm.Name); l > maxName {
			maxName = l
		}
		if l := lipgloss.Width(mm.ID); l > maxID {
			maxID = l
		}
	}
	for i, mm := range m.models {
		marker := " "
		if mm.IsPrimary {
			marker = "★"
		}
		cur := " "
		if mm.ID == m.current.ID {
			cur = "•"
		}
		fmt.Fprintf(&b, "%s%s %2d. %-*s  %-*s  [%s/%s]\n",
			cur, marker, i+1, maxName, mm.Name, maxID, mm.ID, mm.Provider, mm.Tier)
	}
	return b.String()
}

func summaryForArgs(name string, args json.RawMessage) string {
	var generic map[string]any
	_ = json.Unmarshal(args, &generic)
	switch name {
	case "Bash":
		cmd, _ := generic["command"].(string)
		desc, _ := generic["description"].(string)
		if cmd != "" && desc != "" {
			return cmd + "\n# " + desc
		}
		if cmd != "" {
			return cmd
		}
	case "Write":
		p, _ := generic["path"].(string)
		c, _ := generic["content"].(string)
		size := len(c)
		if p != "" {
			return fmt.Sprintf("Write %s  (%d байт)", p, size)
		}
	case "Edit":
		p, _ := generic["path"].(string)
		old, _ := generic["old_string"].(string)
		newS, _ := generic["new_string"].(string)
		if p != "" {
			return fmt.Sprintf("Edit %s\n--- old (%d c) ---\n%s\n+++ new (%d c) +++\n%s",
				p, len(old), truncate(old, 200), len(newS), truncate(newS, 200))
		}
	case "Read":
		if p, _ := generic["path"].(string); p != "" {
			return "Read " + p
		}
	case "Grep":
		if pat, _ := generic["pattern"].(string); pat != "" {
			return "Grep " + pat
		}
	}
	raw, _ := json.Marshal(generic)
	return string(raw)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func shortenCWD(p string) string {
	if p == "" {
		return "?"
	}
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return filepath.Clean(p)
}

// ===== streamer/approver wiring =====

// teaProgramRef хранит ссылку на текущую tea.Program — нужна чтобы стример
// мог посылать сообщения из горутины через prog.Send. Перезаписывается при
// каждом RunTUI; для одной программы в процессе этого достаточно.
var (
	teaProgramRef *tea.Program
)

type tuiStreamer struct{ prog *tea.Program }

func (s *tuiStreamer) OnText(d string) {
	if s.prog != nil {
		s.prog.Send(agentTextMsg(d))
	}
}
func (s *tuiStreamer) OnReasoning(d string) {
	if s.prog != nil {
		s.prog.Send(agentReasoningMsg(d))
	}
}
func (s *tuiStreamer) OnToolCall(name string, args json.RawMessage) {
	if s.prog != nil {
		s.prog.Send(agentToolCallMsg{name: name, args: args})
	}
}
func (s *tuiStreamer) OnToolChunk(name, chunk string) {
	if s.prog != nil {
		s.prog.Send(agentToolChunkMsg{name: name, chunk: chunk})
	}
}
func (s *tuiStreamer) OnToolResult(name, result string, err error) {
	if s.prog != nil {
		s.prog.Send(agentToolResultMsg{name: name, result: result, err: err})
	}
}
func (s *tuiStreamer) OnIterationStart(int) {}

type tuiApprover struct{ prog *tea.Program }

func (a *tuiApprover) AskApprove(name string, args json.RawMessage, summary string) agent.ApproveDecision {
	reply := make(chan agent.ApproveDecision, 1)
	if a.prog == nil {
		return agent.ApproveDeny
	}
	a.prog.Send(agentApproveAskMsg{name: name, args: args, summary: summary, reply: reply})
	return <-reply
}
