// TUI with a tool-use loop via /aicore-vbai/agent-stream.
// Architecture: bubbletea Model holds history (assistant/user/tool messages),
// agent.Agent runs the loop in a goroutine, events (text deltas, tool calls,
// results, approval requests) arrive through a channel as tea.Msg.
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
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
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/sessions"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
	"github.com/velesbsdllc/agent-vbai/internal/welcome"
)

// ===== styles =====

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

// Signals from device-flow polling in loginMode.
type agentLinkDoneMsg struct{ creds *config.Credentials }
type agentLinkErrMsg struct{ err error }
type agentLinkTickMsg struct{ n int } // device-flow polling tick, shows progress

type agentApproveAskMsg struct {
	name    string
	args    json.RawMessage
	summary string
	reply   chan agent.ApproveDecision
}

// agentAskChoiceMsg — the AskUser tool asks the user to pick an option.
// reply carries the chosen label back to the blocked tool; an empty string
// means "dismissed".
type agentAskChoiceMsg struct {
	question string
	options  []tools.AskOption
	reply    chan string
}
type agentDoneMsg struct {
	history []llm.AIMessage
}
type agentErrMsg struct{ err error }

// ===== model =====

type tuiModel struct {
	cfg   *config.Config
	creds *config.Credentials

	// LLM client and agent. Initialized after successful login.
	cli      llm.StreamingLLM     // ExecAI or an external subscription — switched via /use
	subs     *subscriptions.Store // subscriptions (Z.ai/Anthropic/OpenAI)
	registry *tools.Registry
	system   string

	// loginMode = true when there are no creds — TUI asks for confirmation in the
	// browser (device-flow) or falls back to paste-token.
	loginMode   bool
	pendingLink *auth.LinkStart // active device-flow request, waiting for confirmation

	models            []llm.Model
	execAIModels      []llm.Model // snapshot of the original ExecAI catalog — for returning from external subscriptions
	ollamaModels      []llm.Model // cache of models installed in Ollama (refreshed via /connect ollama and /model refresh)
	ollamaCloudModels []llm.Model // session cache of the ollama.com cloud catalog (fetched via authed /api/tags)
	current           llm.Model
	lastLoginAt       time.Time // when the last device-flow finished. Guard against infinite-loop 401→device-flow→401.

	session        *sessions.Session // auto-saved after each exchange
	history        []llm.AIMessage   // including system
	uiSegments     []segment         // what to render for the user (user/assistant/tool/reasoning)
	lastEmittedIdx int               // Ink-style: how many uiSegments were already emitted via Program.Println (not re-rendered)
	streamBuf      strings.Builder   // currently accumulated assistant delta
	reasonBuf      strings.Builder   // current reasoning
	toolBuf        strings.Builder   // live output of the current streaming tool (Bash etc.)
	toolName       string            // name of the tool currently streaming
	streaming      bool

	// approve mode
	approving      bool
	approveTool    string
	approveSummary string
	approveReplyCh chan agent.ApproveDecision
	approveFocus   int // 0=Once, 1=Session, 2=Command, 3=Always, 4=Reject

	// ask mode — the model asked a question with fixed options (AskUser tool).
	// Same mechanics as approve: the tool blocks on a channel, the UI replies.
	asking      bool
	askQuestion string
	askOptions  []tools.AskOption
	askFocus    int
	askReplyCh  chan string

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

	// === Autocomplete (like Claude Code) ===
	// Active when the user starts with "/" — shows a menu of commands/arguments.
	suggestActive bool
	suggestItems  []suggestItem
	suggestFocus  int

	// Input history (like bash): ↑↓ navigate.
	inputHistory []string
	historyIndex int    // 0 = current input; 1..len = input N steps back. -1 = no navigation
	historyDraft string // buffer for unsubmitted input when the user went into history

	// Exit confirmation (Ctrl+C/Ctrl+D twice within 2s).
	exitConfirmAt time.Time

	// Thinking-effort picker (modal overlay like in Claude Code).
	// Opened with Shift+Tab. ←/→ — select, Enter — confirm, Esc — cancel.
	thinkingPickerActive bool
	thinkingPickerIdx    int // 0..len(thinkingLevels)-1

	// Paste collapse (like in Claude Code): large pastes are replaced with a
	// "[Pasted #N — L lines, C chars]" marker in the textarea and in the
	// displayed user segment. The real content is kept in pasteStore
	// and expanded when sent to the agent.
	pasteStore   map[int]pasteEntry
	pasteCounter int

	// Paste-burst (SSH clients without bracketed paste, e.g. Bitvise):
	// keystroke streams faster than 8ms/key are buffered and flushed after
	// an 80ms quiet period (pasteFlushMsg). Prevents per-line submits.
	lastKeyAt        time.Time
	pasteBurst       strings.Builder
	pasteBurstActive bool
	pasteFlushGen    int

	// /loop — periodic prompt repetition (like in Claude Code).
	// Works only while the TUI is open. /loop stop — stop it.
	loopActive   bool
	loopInterval time.Duration
	loopPrompt   string
}

// loopTickMsg — tick event for /loop.
type loopTickMsg struct{}

// autoWakeMsg — AI scheduled a wakeup via the schedule_wakeup tool.
type autoWakeMsg struct {
	prompt string
}

// suggestItem — a single row in the suggestions menu.
type suggestItem struct {
	insert string // what to insert into the textarea on selection
	label  string // how to display in the menu (e.g. "/model — выбор модели")
	hint   string // dimmed right-hand part (e.g. ID/description)
}

// pasteFlushMsg fires 80ms after the last burst keystroke; gen guards against
// stale timers (each keystroke bumps pasteFlushGen — only the latest fires).
type pasteFlushMsg struct {
	gen int
}

// pasteEntry — one large-text paste, collapsed into a
// "[Pasted #N — L lines, C chars]" marker. marker is unique, the full content
// is in text. On submit expandPasteMarkers expands markers back.
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
	tool string // for tool_call/tool_result — the name
}

// ===== entry point =====

// agentVersion — binary version, passed in from cmd/execai/main.go.
var agentVersion = "dev"

func RunTUI(_ context.Context, cfg *config.Config, ver string) error {
	if ver != "" {
		agentVersion = ver
	}
	if os.Getenv("COLORFGBG") == "" {
		_ = os.Setenv("COLORFGBG", "15;0")
	}

	// Locale: explicit from config, otherwise auto-detect from $LANG. Fallback → en.
	localeAuto, localeSrc := initI18nLocale(cfg)
	_ = localeAuto
	_ = localeSrc // TODO: emit boot-hint segment with this (fabricate in initModel)

	cr, _ := config.LoadCredentials()
	loginMode := cr == nil || cr.Token == ""

	cwd, _ := os.Getwd()
	short := shortenCWD(cwd)
	wMsg := welcome.MaybeWelcome()
	registry := tools.Default(cwd)

	ta := textarea.New()
	if loginMode {
		ta.Placeholder = i18n.T("placeholder.login")
	} else {
		ta.Placeholder = i18n.T("placeholder.chat")
	}
	ta.Focus()
	ta.Prompt = "▌ "
	ta.ShowLineNumbers = false
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.CharLimit = 32768
	ta.KeyMap.InsertNewline.SetKeys("shift+enter", "ctrl+j")
	// Ink-style: static cursor, no blinking. Every blink → re-render →
	// the terminal drops native selection. In classic (alt-screen) blink
	// hurts nothing — restart in Update when we switch modes.
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
	// Let tools report the active session to the backend: WebSearch passes it as
	// conversation_id so the search spend is attributed to this CLI session and
	// not booked anonymously.
	tools.SetSessionIDFunc(func() string {
		if m.session == nil {
			return ""
		}
		return m.session.ID
	})
	// AskUser: the tool blocks in the agent goroutine, the UI answers via the
	// bubbletea message loop — the same shape as the approval prompt.
	tools.SetAskUserFunc(func(ctx context.Context, question string, options []tools.AskOption) (string, error) {
		if teaProgramRef == nil {
			return "", tools.ErrAskUnavailable
		}
		reply := make(chan string, 1)
		teaProgramRef.Send(agentAskChoiceMsg{question: question, options: options, reply: reply})
		select {
		case answer := <-reply:
			if answer == "" {
				// Dismissed — tell the model plainly rather than fabricate a choice.
				return i18n.T("ask.dismissedForModel"), nil
			}
			return answer, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
	// Task: subagents run in-process with a read-only toolset.
	tools.SetSubagentRunner(m.runSubagent)
	// No-login boot: not logged in to ExecAI, but an external subscription is
	// active (kimi/zai/openai/…) — skip the login screen entirely and start
	// the chat on that subscription. The ExecAI account is optional.
	if loginMode && subs != nil && subs.ActiveSubscription() != nil {
		loginMode = false
		m.loginMode = false
		m.textarea.Placeholder = i18n.T("placeholder.chat")
		m.applySubscriptionSource()
		m.system = agent.SystemPrompt(cwd, registry.Names(), agent.LoadMemory(cwd))
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint",
			body: i18n.Tf("ui.boot.noLogin", subs.SourceLabel(), m.current.ID)})
		m.refreshViewport()
	} else if loginMode {
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: i18n.T("ui.login.intro")})
	} else {
		// Load models and assemble the agent layer right away.
		// If the token expired (401) — switch to loginMode and start device-flow
		// instead of failing with an error.
		if err := m.bootAfterLogin(); err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "401") || strings.Contains(errStr, "токен истёк") {
				_ = auth.Logout()
				m.creds = nil
				m.loginMode = true
				m.uiSegments = append(m.uiSegments, segment{kind: "system_hint",
					body: i18n.Tf("ui.login.staleToken", cfg.APIBase)})
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

	// Ink-style rendering (like Claude Code):
	// * Message history (assistant/tool_result/user submit) → tea.Println →
	//   becomes regular terminal scrollback, native selection works
	//   and is not reset on re-render (those lines are never rewritten).
	// * View() renders only: the current stream + input + status — a small
	//   zone at the bottom, its re-render doesn't disturb selection above.
	// * Alt-screen and mouse capture are DISABLED — the terminal handles
	//   scroll wheel and drag-selection natively itself.
	// Legacy classic mode (m.cfg.ClassicTUI=true) can be brought back with /classic on.
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
	// In Ink-style we don't start periodic blink/spinner — every tick causes
	// a View() re-render, the terminal sees output and drops native selection.
	// Cursor becomes static (bubbles cursor mode = static).
	cmds := []tea.Cmd{tea.SetWindowTitle(m.titleString())}
	if m.cfg.ClassicTUI {
		cmds = append(cmds, textarea.Blink, m.spinner.Tick)
	}
	// In loginMode — start device-flow immediately, don't wait for input.
	if m.loginMode {
		cmds = append(cmds, func() tea.Msg {
			return autoStartLoginMsg{}
		})
	}
	// Auto-update check on startup (async, in the background).
	cmds = append(cmds, m.checkForUpdateCmd())
	return tea.Batch(cmds...)
}

// titleString — for tea.SetWindowTitle. Format: "execai · model · session-title".
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
		// Reserve rows: header(1) + textarea(1) + status(1) + help(1) + slack(2)
		// for the optional bottom line (err/statusMessage) and potential status-bar wrap.
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
		// Paste-burst detection (SSH clients without bracketed paste, e.g.
		// Bitvise): a paste arrives as a rapid stream of individual keystrokes
		// with CR after every line — each Enter would submit a partial line to
		// the LLM. Humans type at >30ms/key; paste streams run at <8ms. When
		// two printable keys arrive within burstGap, we enter burst mode:
		// buffer everything (Enter becomes '\n'), and flush after a quiet
		// period via pasteFlushMsg — collapsing to a [Pasted #N] marker when
		// large. Terminals WITH bracketed paste deliver the whole paste as a
		// single KeyMsg, which never triggers this path (handled by
		// interceptPaste below).
		if !m.approving && !m.asking && !m.loginMode {
			now := time.Now()
			gap := now.Sub(m.lastKeyAt)
			m.lastKeyAt = now
			isPasteable := msg.Type == tea.KeyRunes || msg.Type == tea.KeyEnter ||
				msg.Type == tea.KeySpace || msg.Type == tea.KeyTab
			if m.pasteBurstActive && isPasteable {
				switch msg.Type {
				case tea.KeyEnter:
					m.pasteBurst.WriteByte('\n')
				case tea.KeySpace:
					m.pasteBurst.WriteByte(' ')
				case tea.KeyTab:
					m.pasteBurst.WriteByte('\t')
				default:
					m.pasteBurst.WriteString(string(msg.Runes))
				}
				m.pasteFlushGen++
				gen := m.pasteFlushGen
				return m, tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
					return pasteFlushMsg{gen: gen}
				})
			}
			if isPasteable && gap < 8*time.Millisecond && len(msg.Runes) <= 1 {
				// Burst start: the previous keystroke already went into the
				// textarea — pull it back into the buffer so the paste stays whole.
				m.pasteBurstActive = true
				m.pasteBurst.Reset()
				val := m.textarea.Value()
				if n := len(val); n > 0 {
					r := []rune(val)
					m.pasteBurst.WriteString(string(r[len(r)-1]))
					m.textarea.SetValue(string(r[:len(r)-1]))
					m.textarea.CursorEnd()
				}
				switch msg.Type {
				case tea.KeyEnter:
					m.pasteBurst.WriteByte('\n')
				case tea.KeySpace:
					m.pasteBurst.WriteByte(' ')
				case tea.KeyTab:
					m.pasteBurst.WriteByte('\t')
				default:
					m.pasteBurst.WriteString(string(msg.Runes))
				}
				m.pasteFlushGen++
				gen := m.pasteFlushGen
				return m, tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
					return pasteFlushMsg{gen: gen}
				})
			}
		}
		// ask mode: ↑↓ move, Enter picks, 1-4 pick directly, Esc dismisses.
		// Checked before approve mode — the two are never active together, but
		// the question is the more recent event if they somehow were.
		if m.asking {
			switch msg.String() {
			case "up", "shift+tab":
				if m.askFocus > 0 {
					m.askFocus--
				} else {
					m.askFocus = len(m.askOptions) - 1
				}
				m.refreshViewport()
				return m, nil
			case "down", "tab":
				m.askFocus = (m.askFocus + 1) % len(m.askOptions)
				m.refreshViewport()
				return m, nil
			case "enter":
				m.replyAsk(m.askOptions[m.askFocus].Label)
				return m, nil
			case "esc", "ctrl+c":
				// Dismissed: the tool reports it back so the model decides itself
				// instead of hanging or pretending an answer was given.
				m.replyAsk("")
				return m, nil
			case "1", "2", "3", "4":
				if n := int(msg.String()[0] - '1'); n < len(m.askOptions) {
					m.replyAsk(m.askOptions[n].Label)
				}
				return m, nil
			}
			return m, nil
		}
		// approve mode: arrows/Tab switch focus, Enter confirms,
		// Esc/Ctrl+C — reject. Hotkeys work too.
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
		// Any key EXCEPT Ctrl+C/Ctrl+D resets the "exit confirmation".
		if msg.Type != tea.KeyCtrlC && msg.Type != tea.KeyCtrlD && !m.exitConfirmAt.IsZero() {
			m.exitConfirmAt = time.Time{}
			m.statusMessage = ""
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			// While streaming — cancel generation, do NOT exit.
			if m.streaming {
				if m.cancel != nil {
					m.cancel()
				}
				m.statusMessage = i18n.T("stream.interrupted")
				return m, nil
			}
			// Not streaming — require confirmation: second Ctrl+C within 2s.
			if !m.exitConfirmAt.IsZero() && time.Since(m.exitConfirmAt) < 2*time.Second {
				return m, tea.Quit
			}
			m.exitConfirmAt = time.Now()
			m.statusMessage = i18n.T("quit.confirmCtrlC")
			return m, nil
		case tea.KeyCtrlD:
			// Same logic as Ctrl+C.
			if !m.exitConfirmAt.IsZero() && time.Since(m.exitConfirmAt) < 2*time.Second {
				return m, tea.Quit
			}
			m.exitConfirmAt = time.Now()
			m.statusMessage = i18n.T("quit.confirmCtrlD")
			return m, nil
		case tea.KeyCtrlR:
			// Ctrl+R — open fuzzy session search via the existing autocomplete menu.
			m.textarea.SetValue("/resume ")
			m.textarea.CursorEnd()
			return m, m.refreshSuggest()
		case tea.KeyShiftTab:
			// Shift+Tab — open/close the thinking-picker (modal overlay like in Claude Code).
			if m.thinkingPickerActive {
				// Already open — cycle right.
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
			m.uiSegments = nil
			m.lastEmittedIdx = 0
			m.statusMessage = i18n.T("sys.historyCleared")
			m.refreshViewport()
			return m, nil
		case tea.KeyTab:
			// Tab always triggers autocomplete if the line starts with "/".
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
			// Input history: navigate back. Save the drafted text if not yet in history.
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
			// Input history: navigate forward. At 0 — restore the draft.
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
				m.statusMessage = i18n.T("effort.pickerCancelled")
				return m, nil
			}
			if m.suggestActive {
				return m, m.closeSuggest()
			}
		case tea.KeyEnter:
			// If the thinking-picker is open — Enter confirms the selection.
			if m.thinkingPickerActive {
				lvl := thinkingLevels[m.thinkingPickerIdx]
				m.cfg.ThinkingBudget = lvl.budget
				_ = config.Save(m.cfg)
				m.cli = m.makeLLMClient()
				m.thinkingPickerActive = false
				m.statusMessage = i18n.Tf("effort.changed", lvl.label, lvl.budget)
				return m, nil
			}
			// If the menu is active — Enter:
			//   * if the text already exactly equals some suggestion → submit as usual
			//   * otherwise accept the focused suggestion (only replace the text)
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
			// exact match OR menu not active — close the menu (if active),
			// disable mouse capture and submit as usual. tea.DisableMouse
			// is collected into pendingCmd so it's not lost on return.
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
			// Input history: record unique submits (don't duplicate the same one back-to-back).
			if n := len(m.inputHistory); n == 0 || m.inputHistory[n-1] != line {
				m.inputHistory = append(m.inputHistory, line)
				if len(m.inputHistory) > 200 {
					m.inputHistory = m.inputHistory[len(m.inputHistory)-200:]
				}
				go saveInputHistory(line) // persist to disk, don't block the UI
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
		if m.streaming || m.approving || m.asking {
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
		// Finalize the accumulated assistant text as a segment
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
		// Reset the live buffer for the new tool call
		m.toolBuf.Reset()
		m.toolName = msg.name
		m.refreshViewport()
		return m, nil

	case agentToolChunkMsg:
		// Append to the live buffer, refresh the viewport — rendered on top of ▶ tool_call.
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
		// Click on the thinking slider sets the level. Layout: ... status (1) + thinking-slider (1) + help (1).
		// The slider sits on the second-to-last line from the bottom when visible.
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft &&
			m.subs != nil && m.subs.Active == subscriptions.SourceZAI {
			sliderY := m.height - 2 // status, slider, help — slider = height-2 (0-indexed)
			if msg.Y == sliderY {
				// "🧠 think: off · low · med · high · max    (Shift+Tab или клик)"
				// Determine the nearest label position by X.
				// Prefix "🧠 think: " ~ 11 visible cells (emoji takes 2).
				prefix := 11
				// Label lengths: off(3) · low(3) · med(3) · high(4) · max(3), separators " · " (3)
				positions := []struct {
					start, end int
					budget     int
				}{}
				x := prefix
				for _, lvl := range thinkingLevels {
					labelLen := len(lvl.label)
					// active adds 2 (padding) — but for clickable areas we count without padding
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
						m.statusMessage = i18n.Tf("effort.changed", label, p.budget)
						return m, nil
					}
				}
			}
		}
		// Click on a menu row — select + accept.
		if m.suggestActive && len(m.suggestItems) > 0 &&
			msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// Layout: header(1) + viewport(vpH) + bottom(0..1) + suggest_box → textarea → status → help
			headerH := 1
			vpH := m.viewport.Height
			bottomH := 0
			if m.err != "" || m.statusMessage != "" {
				bottomH = 1
			}
			y0 := headerH + vpH + bottomH // 0-indexed position of the box's top border
			maxRows := 8
			if len(m.suggestItems) < maxRows {
				maxRows = len(m.suggestItems)
			}
			// Inside the box: row 0 = border, rows 1..maxRows = items, row maxRows+1 = footer, row maxRows+2 = border.
			clicked := msg.Y - y0 - 1 // relative to the start of items
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
		// Wheel-scroll over the menu — focus navigation.
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
		// Menu NOT active — wheel should scroll the chat viewport.
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			var c tea.Cmd
			m.viewport, c = m.viewport.Update(msg)
			return m, c
		}
		return m, nil

	case pasteFlushMsg:
		// Quiet period after a keystroke burst — flush the buffered paste.
		if msg.gen != m.pasteFlushGen || !m.pasteBurstActive {
			return m, nil // stale timer, a newer keystroke arrived
		}
		text := m.pasteBurst.String()
		m.pasteBurst.Reset()
		m.pasteBurstActive = false
		if text == "" {
			return m, nil
		}
		hasNewline := strings.ContainsAny(text, "\n\r")
		if len([]rune(text)) >= pasteThresholdRunes || hasNewline {
			// Big/multiline → collapse to a [Pasted #N] marker (same as
			// bracketed-paste path in interceptPaste).
			m.pasteCounter++
			lines := strings.Count(text, "\n") + 1
			marker := fmt.Sprintf("[Pasted #%d — %d lines, %d chars]", m.pasteCounter, lines, len(text))
			m.pasteStore[m.pasteCounter] = pasteEntry{
				id: m.pasteCounter, text: text, lines: lines, chars: len(text), marker: marker,
			}
			m.textarea.InsertString(marker)
		} else {
			m.textarea.InsertString(text)
		}
		return m, m.refreshSuggest()

	case updateAvailableMsg:
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: msg.hint})
		m.refreshViewport()
		return m, nil

	case updateLatestMsg:
		m.statusMessage = i18n.Tf("update.latest", agentVersion)
		return m, nil

	case loopTickMsg:
		if !m.loopActive {
			return m, nil
		}
		// Schedule the next tick independently — even if it's streaming now,
		// we skip and check on the next tick.
		nextTick := tea.Tick(m.loopInterval, func(time.Time) tea.Msg { return loopTickMsg{} })
		if m.streaming || m.approving || m.asking || m.loginMode {
			return m, nextTick // skip, the agent is busy
		}
		// Emulate user input: add to the UI + start the agent.
		m.uiSegments = append(m.uiSegments, segment{kind: "user", body: "🔁 [loop] " + m.loopPrompt})
		m.refreshViewport()
		return m, tea.Batch(nextTick, m.startAgent(m.loopPrompt))

	case compactDoneMsg:
		if msg.err != nil {
			m.err = "compact: " + msg.err.Error()
			return m, nil
		}
		// Rebuild history: system + summary + last tail.
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
			Content: i18n.Tf("ui.compact.historyNote", msg.saved, msg.summary),
		})
		newHist = append(newHist, tail...)
		m.history = newHist
		// UI: add a compact hint about the compaction.
		m.uiSegments = append(m.uiSegments, segment{
			kind: "system_hint",
			body: i18n.Tf("ui.compact.done", msg.saved, len(msg.summary)),
		})
		m.statusMessage = ""
		m.refreshViewport()
		return m, nil

	case agentLinkDoneMsg:
		// successful login from the polling goroutine
		return m.finishLogin(msg.creds)

	case agentLinkTickMsg:
		// No-login mode: the user switched to an external subscription while
		// the device-flow poller is still running — don't clobber the status.
		if !m.loginMode {
			return m, nil
		}
		m.statusMessage = i18n.Tf("login.waitingPoll", msg.n)
		return m, nil

	case agentLinkErrMsg:
		m.err = i18n.Tf("login.deviceFlowFailed", msg.err.Error())
		m.statusMessage = ""
		m.pendingLink = nil
		m.refreshViewport()
		return m, nil

	case agentApproveAskMsg:
		m.approving = true
		m.approveTool = msg.name
		m.approveSummary = msg.summary
		m.approveReplyCh = msg.reply
		m.approveFocus = 0 // default — "Once"
		m.refreshViewport()
		return m, nil

	case agentAskChoiceMsg:
		m.asking = true
		m.askQuestion = msg.question
		m.askOptions = msg.options
		m.askReplyCh = msg.reply
		m.askFocus = 0 // first option is the recommended one by contract
		m.refreshViewport()
		return m, nil

	case agentDoneMsg:
		m.streaming = false
		// Finalize the last text stream, if any
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
		// Autoloop: check whether the AI requested a wakeup via the schedule_wakeup tool.
		if req := tools.TakeScheduledWakeup(); req != nil {
			delay := time.Duration(req.DelaySeconds) * time.Second
			prompt := req.PromptOnWake
			if prompt == "" {
				prompt = i18n.T("ui.autoloop.defaultPrompt")
			}
			m.uiSegments = append(m.uiSegments, segment{
				kind: "system_hint",
				body: i18n.Tf("ui.autoloop.wake", delay, req.Reason, prompt),
			})
			m.refreshViewport()
			return m, tea.Tick(delay, func(time.Time) tea.Msg {
				return autoWakeMsg{prompt: prompt}
			})
		}
		m.refreshViewport()
		return m, nil

	case autoWakeMsg:
		if m.streaming || m.approving || m.asking || m.loginMode {
			// If the user is busy right now — skip (the AI will re-schedule itself later).
			return m, nil
		}
		m.uiSegments = append(m.uiSegments, segment{kind: "user", body: "🌙 [autoloop] " + msg.prompt})
		m.refreshViewport()
		return m, m.startAgent(msg.prompt)

	case agentErrMsg:
		m.streaming = false
		if msg.err == context.Canceled {
			m.statusMessage = i18n.T("stream.aborted")
		} else {
			m.err = msg.err.Error()
			// 401 mid-session.
			// Strategy: if device-flow already ran in this CLI launch —
			// do NOT auto-trigger it again. auth-vbai issued a valid JWT,
			// api-vbai/aicore-vbai rejected it — that's an env misconfig
			// (or user session banned), device-flow won't change anything.
			// Show a clear error and suggest /login (manual start)
			// or /source to an external subscription.
			errStr := msg.err.Error()
			if strings.Contains(errStr, "401") || strings.Contains(errStr, "токен истёк") {
				if m.subs == nil || m.subs.ActiveSubscription() == nil {
					if !m.lastLoginAt.IsZero() {
						// Keep the original errStr (already contains the response body
						// from api-vbai) plus a hint on what to do.
						m.err = errStr + "\n" + i18n.T("ui.stream.tokenExpiredHint")
						m.streamBuf.Reset()
						m.reasonBuf.Reset()
						m.refreshViewport()
						return m, nil
					}
					// First 401 in the session, no previous login — start the flow.
					_ = auth.Logout()
					m.creds = nil
					m.loginMode = true
					m.err = ""
					m.uiSegments = append(m.uiSegments, segment{kind: "system_hint",
						body: i18n.T("ui.stream.tokenExpiredFlow")})
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

	// Paste-collapse: intercept large Runes inputs (Ctrl+V), replace with
	// a marker in the textarea. The full text expands on submit.
	if key, ok := msg.(tea.KeyMsg); ok {
		if m.interceptPaste(key) {
			// We already inserted the marker into the textarea ourselves — don't pass to textarea.Update.
			var refCmd tea.Cmd = m.refreshSuggest()
			return m, refCmd
		}
	}
	var c1, c2 tea.Cmd
	m.textarea, c1 = m.textarea.Update(msg)
	m.viewport, c2 = m.viewport.Update(msg)
	// Recompute the menu after the text changes.
	var refCmd tea.Cmd
	if _, ok := msg.(tea.KeyMsg); ok {
		refCmd = m.refreshSuggest()
	}
	return m, tea.Batch(c1, c2, refCmd)
}

// refreshSuggest rebuilds the suggestion list based on the current textarea
// value. Returns a tea.Cmd on the active↔inactive transition — toggles the mouse
// so text-select works when the menu is closed, and menu clicks work when open.
func (m *tuiModel) refreshSuggest() tea.Cmd {
	wasActive := m.suggestActive
	defer func() {
		// Toggle the mouse only on an edge (active changed).
	}()

	if m.streaming || m.loginMode || m.approving || m.asking {
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
			case cmd == "/lang":
				items = filterLangOptions(arg)
			}
			m.suggestItems = items
			m.suggestActive = len(items) > 0
			if m.suggestFocus >= len(items) {
				m.suggestFocus = 0
			}
		}
	}

	_ = wasActive // mouse is always on, no toggle needed
	return nil
}

// acceptSuggest inserts the selected item into the textarea (replacing the current line).
// If after insertion the menu "collapsed" onto itself (no further arguments) —
// the menu closes and the mouse returns to native mode.
//
// Auto-submit heuristic: insert WITHOUT a trailing space ('/help', '/source zai',
// '/model glm-5.2') → the command expects no argument → submit right away.
// With a trailing space ('/model ', '/source ', '/cd ') → wait for the argument.
func (m *tuiModel) acceptSuggest(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.suggestItems) {
		return nil
	}
	insert := m.suggestItems[idx].insert
	m.textarea.SetValue(insert)
	m.textarea.CursorEnd()
	if !strings.HasSuffix(insert, " ") {
		// Auto-submit: the command is fully ready, the user doesn't need to press Enter again.
		closeCmd := m.closeSuggest()
		_, submitCmd := m.submitTextarea()
		return tea.Batch(closeCmd, submitCmd)
	}
	// Otherwise — recompute the menu (e.g. /model → now models).
	return m.refreshSuggest()
}

// submitTextarea — shared submit flow (like KeyEnter): takes the value, puts it
// into input-history, resets the textarea, calls handleInput. Returns
// the result as (model, cmd) for further chaining.
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

// closeSuggest closes the menu. Don't touch the mouse — it's always captured.
func (m *tuiModel) closeSuggest() tea.Cmd {
	m.suggestActive = false
	m.suggestItems = nil
	return nil
}

// allCommands — canonical list of slash commands for the menu.
var allCommands = []suggestItem{
	// hint: an i18n key; renderSuggestPanel translates via i18n.T() on the fly
	// (dynamic hints — model descriptions etc. — pass through,
	// since T() returns the input unchanged when the key is not found).
	{insert: "/help", label: "/help", hint: "hint.help"},
	{insert: "/model ", label: "/model", hint: "hint.model"},
	{insert: "/usage", label: "/usage", hint: "hint.usage"},
	{insert: "/compact", label: "/compact", hint: "hint.compact"},
	{insert: "/log", label: "/log", hint: "hint.log"},
	{insert: "/loop ", label: "/loop", hint: "hint.loop"},
	{insert: "/effort", label: "/effort", hint: "hint.effort"},
	{insert: "/max-iterations ", label: "/max-iterations", hint: "hint.maxIterations"},
	{insert: "/subscriptions", label: "/subscriptions", hint: "hint.subscriptions"},
	{insert: "/connect zai ", label: "/connect zai", hint: "hint.connect.zai"},
	{insert: "/connect zai-api ", label: "/connect zai-api", hint: "hint.connect.zaiapi"},
	{insert: "/connect kimi ", label: "/connect kimi", hint: "hint.connect.kimi"},
	{insert: "/connect kimi-api ", label: "/connect kimi-api", hint: "hint.connect.kimiapi"},
	{insert: "/connect anthropic ", label: "/connect anthropic", hint: "hint.connect.anthropic"},
	{insert: "/connect openai ", label: "/connect openai", hint: "hint.connect.openai"},
	{insert: "/connect codex-cli", label: "/connect codex-cli", hint: "hint.connect.codexcli"},
	{insert: "/connect claude-cli", label: "/connect claude-cli", hint: "hint.connect.claudecli"},
	{insert: "/connect ollama", label: "/connect ollama", hint: "hint.connect.ollama"},
	{insert: "/source ", label: "/source", hint: "hint.source"},
	{insert: "/mouse off", label: "/mouse off", hint: "hint.mouseOff"},
	{insert: "/mouse on", label: "/mouse on", hint: "hint.mouseOn"},
	{insert: "/inline on", label: "/inline on", hint: "hint.inlineOn"},
	{insert: "/lang ", label: "/lang", hint: "hint.lang"},
	{insert: "/paste", label: "/paste", hint: "hint.paste"},
	{insert: "/cd ", label: "/cd", hint: "hint.cd"},
	{insert: "/sessions", label: "/sessions", hint: "hint.sessions"},
	{insert: "/resume ", label: "/resume", hint: "hint.resume"},
	{insert: "/new", label: "/new", hint: "hint.new"},
	{insert: "/clear", label: "/clear", hint: "hint.clear"},
	{insert: "/whoami", label: "/whoami", hint: "hint.whoami"},
	{insert: "/config", label: "/config", hint: "hint.config"},
	{insert: "/permissions", label: "/permissions", hint: "hint.permissions"},
	{insert: "/login", label: "/login", hint: "hint.login"},
	{insert: "/logout", label: "/logout", hint: "hint.logout"},
	{insert: "/quit", label: "/quit", hint: "hint.quit"},
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
			title = i18n.T("session.untitled")
		}
		match := low == "" ||
			strings.Contains(strings.ToLower(title), low) ||
			strings.Contains(strings.ToLower(s.ID), low)
		if !match && low != "" {
			// Fuzzy search over content: load the session and look for a substring in user/assistant.
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

// handleLoginInput in loginMode:
//   - "y"/Enter (when there is a pending link) — open the browser.
//   - "n" / new input — refuse to open, wait for the user to open the URL themselves.
//   - a pasted JWT (long eyJ… string) — fallback to the old paste-token flow.
//   - otherwise — start a new device-link (show the hint once).
func (m *tuiModel) handleLoginInput(line string) (tea.Model, tea.Cmd) {
	m.err = ""
	m.statusMessage = ""

	// Slash commands work even before login: an ExecAI account is OPTIONAL.
	// /connect kimi|zai|openai|... + /source <provider> lets the CLI run
	// entirely on the user's own subscription, no ExecAI login needed.
	if strings.HasPrefix(strings.TrimSpace(line), "/") {
		return m.handleCommand(strings.TrimSpace(line))
	}

	// Fallback: the user pasted a full JWT — old paste-token flow.
	if looksLikeJWT(line) {
		m.statusMessage = i18n.T("login.checkingToken")
		m.refreshViewport()
		cr, err := auth.Login(context.Background(), m.cfg, line)
		if err != nil {
			m.err = i18n.Tf("login.authFailed", err.Error())
			m.statusMessage = ""
			m.refreshViewport()
			return m, nil
		}
		return m.finishLogin(cr)
	}

	// Device-flow control when there is already a pending link.
	if m.pendingLink != nil {
		switch strings.ToLower(line) {
		case "y", "д", "yes", "да", "o", "open":
			if err := auth.OpenBrowser(m.pendingLink.VerifyURI); err != nil {
				m.statusMessage = i18n.T("login.browserOpenFailed")
			} else {
				m.statusMessage = i18n.T("login.openingBrowser")
			}
			m.refreshViewport()
			return m, nil
		case "n", "no", "нет":
			m.statusMessage = i18n.T("login.openManually")
			m.refreshViewport()
			return m, nil
		}
		return m, nil
	}

	// Otherwise — start a device-link.
	return m.startDeviceLink()
}

func (m *tuiModel) startDeviceLink() (tea.Model, tea.Cmd) {
	m.statusMessage = i18n.T("login.contactingServer")
	m.refreshViewport()

	start, err := auth.StartAgentLink(context.Background(), m.cfg.APIBase)
	if err != nil {
		m.err = i18n.Tf("login.linkFailed", err.Error())
		m.refreshViewport()
		return m, nil
	}
	m.pendingLink = start

	m.uiSegments = append(m.uiSegments, segment{kind: "system_hint",
		body: i18n.Tf("ui.login.deviceFlowOpen", start.VerifyURI, start.UserCode)})
	m.statusMessage = i18n.T("login.waitingBrowser")
	m.refreshViewport()

	// Start polling in a goroutine, sending tea.Msg into the TUI.
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

// finishLogin — shared end of the login flow: try bootAfterLogin and
// redraw the screen as a regular chat.
func (m *tuiModel) finishLogin(cr *config.Credentials) (tea.Model, tea.Cmd) {
	m.creds = cr
	m.pendingLink = nil
	m.lastLoginAt = time.Now()
	if err := m.bootAfterLogin(); err != nil {
		m.err = i18n.Tf("login.loadAgentFailed", err.Error())
		m.refreshViewport()
		return m, nil
	}
	greet := i18n.Tf("login.greet", cr.Email)
	if cr.Alias != "" {
		greet = i18n.Tf("login.greetAlias", cr.Email, cr.Alias)
	}
	greet += i18n.Tf("login.greetSuffix", m.current.Provider, m.current.ID)
	m.uiSegments = nil
	m.lastEmittedIdx = 0
	m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: greet})
	m.statusMessage = ""
	m.refreshViewport()
	return m, nil
}

func looksLikeJWT(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "eyJ") && strings.Count(s, ".") == 2
}

// looksLikeImagePath — true if the line starts with "/" but contains an
// image extension (.png/.jpg/etc) — meaning it's a file path, not a command.
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
	// The message contains an image path, but ExtractImageAttachments didn't pick it up —
	// give a VISIBLE hint in the chat (system_hint segment, not just the status bar).
	if looksLikeImagePath(line) {
		if _, imgs := llm.ExtractImageAttachments(line); len(imgs) == 0 {
			m.uiSegments = append(m.uiSegments, segment{
				kind: "system_hint",
				body: i18n.T("sys.imgHintPath"),
			})
		}
	}
	if m.cli == nil {
		m.err = i18n.T("err.notLoggedIn")
		m.refreshViewport()
		return m, nil
	}
	// Keep [Pasted #N] markers in the user segment for compact display
	// in scrollback (otherwise history gets cluttered with 47-line pastes). The agent
	// receives the FULL text via expandPasteMarkers.
	m.uiSegments = append(m.uiSegments, segment{kind: "user", body: line})
	m.refreshViewport()
	return m, m.startAgent(m.expandPasteMarkers(line))
}

func (m *tuiModel) handleCommand(line string) (tea.Model, tea.Cmd) {
	// Expand [Pasted #N] markers BEFORE parsing: a user pasting an API key
	// into '/connect kimi <Ctrl+V>' gets the paste collapsed into a marker,
	// and without expansion the marker itself was saved as the key
	// (api_key="[Pasted", base_url="#2" — real bug).
	cmd := strings.TrimSpace(m.expandPasteMarkers(line))
	switch {
	case cmd == "/quit" || cmd == "/exit":
		return m, tea.Quit
	case cmd == "/clear" || cmd == "/reset":
		m.history = nil
		m.uiSegments = nil
		m.lastEmittedIdx = 0
		m.statusMessage = i18n.T("sys.historyCleared")
		m.refreshViewport()
		return m, nil
	case cmd == "/help" || cmd == "/?":
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: i18n.T("help.body")})
		m.refreshViewport()
		return m, nil
	case cmd == "/paste" || strings.HasPrefix(cmd, "/paste "):
		arg := strings.TrimSpace(strings.TrimPrefix(cmd, "/paste"))
		if arg == "" || arg == "list" {
			var b strings.Builder
			if len(m.pasteStore) == 0 {
				b.WriteString(i18n.T("ui.paste.empty"))
			} else {
				b.WriteString(i18n.T("ui.paste.header"))
				ids := make([]int, 0, len(m.pasteStore))
				for id := range m.pasteStore {
					ids = append(ids, id)
				}
				sort.Ints(ids)
				for _, id := range ids {
					p := m.pasteStore[id]
					fmt.Fprintf(&b, "  #%d — %d lines, %d chars\n", p.id, p.lines, p.chars)
				}
				b.WriteString(i18n.T("ui.paste.showHint"))
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
				m.err = i18n.Tf("ui.paste.notNumber", idStr)
				return m, nil
			}
			p, ok := m.pasteStore[id]
			if !ok {
				m.err = i18n.Tf("ui.paste.notFound", id)
				return m, nil
			}
			body := fmt.Sprintf("=== Paste #%d (%d lines, %d chars) ===\n%s", p.id, p.lines, p.chars, p.text)
			m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: body})
			m.refreshViewport()
			return m, nil
		}
		m.err = i18n.T("ui.paste.usage")
		return m, nil
	case cmd == "/whoami":
		if m.creds != nil {
			m.statusMessage = m.creds.Email + " @ " + m.cfg.APIBase
		} else {
			m.statusMessage = i18n.T("ui.whoami.notLoggedIn")
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
	case cmd == "/lang" || strings.HasPrefix(cmd, "/lang "):
		// /lang         → show current + available
		// /lang <code>  → set and save to config
		arg := strings.TrimSpace(strings.TrimPrefix(cmd, "/lang"))
		if arg == "" {
			m.statusMessage = i18n.Tf("lang.current", i18n.Locale(), availableLocalesStr())
			return m, nil
		}
		newLoc := i18n.SetLocale(arg)
		if newLoc != arg {
			m.err = i18n.Tf("lang.unknown", arg, availableLocalesStr())
			// Restore the previous one since SetLocale already switched to default.
			if m.cfg.Locale != "" {
				i18n.SetLocale(m.cfg.Locale)
			}
			return m, nil
		}
		m.cfg.Locale = newLoc
		if err := config.Save(m.cfg); err != nil {
			m.err = i18n.Tf("err.configSave", err.Error())
			return m, nil
		}
		m.statusMessage = i18n.Tf("lang.changed", newLoc)
		// Recompute the textarea placeholder — it's set once at init, update manually.
		if m.loginMode {
			m.textarea.Placeholder = i18n.T("placeholder.login")
		} else {
			m.textarea.Placeholder = i18n.T("placeholder.chat")
		}
		return m, nil
	case cmd == "/classic" || cmd == "/classic on" || cmd == "/classic off":
		// Toggle classic TUI (alt-screen + mouse). Default — Ink-style
		// (history in scrollback, native selection/scroll).
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
			m.statusMessage = i18n.T("ui.classic.on")
		} else {
			m.statusMessage = i18n.T("ui.classic.off")
		}
		return m, nil
	case cmd == "/mouse" || cmd == "/mouse off" || cmd == "/mouse on":
		// Toggle mouse capture. When off — the terminal keeps the mouse for native selection
		// (for copying). When on — the mouse works for scrolling/clicking the menu.
		// Without Shift+drag (unless the terminal bypasses it).
		if cmd == "/mouse" {
			cmd = "/mouse off" // /mouse without an argument = turn off
		}
		if strings.HasSuffix(cmd, " off") {
			m.statusMessage = i18n.T("ui.mouse.off")
			return m, tea.DisableMouse
		}
		m.statusMessage = i18n.T("ui.mouse.on")
		return m, tea.EnableMouseCellMotion
	case cmd == "/effort":
		// /effort without an argument — open the picker (if Shift+Tab doesn't work in your terminal).
		m.thinkingPickerActive = true
		m.thinkingPickerIdx = m.currentThinkingIdx()
		m.statusMessage = i18n.T("ui.effort.pickerHint")
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
				body: i18n.Tf("ui.effort.current", cur, m.cfg.ThinkingBudget)})
			m.refreshViewport()
			return m, nil
		}
		budget, ok := levels[arg]
		if !ok {
			// Try it as a number.
			if n, err := strconv.Atoi(arg); err == nil && n >= 0 {
				budget = n
			} else {
				m.err = "/effort <off|low|medium|high|max|N>"
				return m, nil
			}
		}
		m.cfg.ThinkingBudget = budget
		_ = config.Save(m.cfg)
		// Recreate the client so the new budget takes effect.
		m.cli = m.makeLLMClient()
		m.statusMessage = i18n.Tf("ui.effort.set", arg, budget)
		return m, nil
	case cmd == "/max-iterations" || cmd == "/maxiter":
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint",
			body: i18n.Tf("ui.maxIter.current", m.cfg.GetMaxIterations())})
		m.refreshViewport()
		return m, nil
	case strings.HasPrefix(cmd, "/max-iterations ") || strings.HasPrefix(cmd, "/maxiter "):
		arg := strings.TrimSpace(cmd)
		for _, p := range []string{"/max-iterations", "/maxiter"} {
			arg = strings.TrimSpace(strings.TrimPrefix(arg, p))
		}
		n, err := strconv.Atoi(arg)
		if err != nil || n <= 0 || n > 500 {
			m.err = i18n.T("ui.maxIter.usage")
			return m, nil
		}
		m.cfg.MaxIterations = n
		_ = config.Save(m.cfg)
		m.statusMessage = fmt.Sprintf("✓ max-iterations = %d", n)
		return m, nil
	case cmd == "/loop":
		if m.loopActive {
			m.statusMessage = i18n.Tf("ui.loop.status", m.loopInterval, m.loopPrompt)
		} else {
			m.statusMessage = i18n.T("ui.loop.inactive")
		}
		return m, nil
	case cmd == "/loop stop" || cmd == "/loop off":
		if !m.loopActive {
			m.statusMessage = i18n.T("ui.loop.notRunning")
			return m, nil
		}
		m.loopActive = false
		m.statusMessage = i18n.T("ui.loop.stopped")
		return m, nil
	case strings.HasPrefix(cmd, "/loop "):
		arg := strings.TrimSpace(strings.TrimPrefix(cmd, "/loop"))
		parts := strings.SplitN(arg, " ", 2)
		if len(parts) < 2 {
			m.err = i18n.T("ui.loop.usage")
			return m, nil
		}
		dur, err := time.ParseDuration(parts[0])
		if err != nil {
			m.err = i18n.Tf("ui.loop.badInterval", parts[0])
			return m, nil
		}
		if dur < 5*time.Second {
			dur = 5 * time.Second
		}
		m.loopActive = true
		m.loopInterval = dur
		m.loopPrompt = parts[1]
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint",
			body: i18n.Tf("ui.loop.started", dur, parts[1])})
		m.refreshViewport()
		// First tick — right away, so the user sees it's working.
		return m, tea.Tick(dur, func(time.Time) tea.Msg { return loopTickMsg{} })
	case cmd == "/log" || cmd == "/logs":
		// Last 20 lines of ~/.config/execai/requests.log — who/what/which source.
		dir, _ := config.Dir()
		path := filepath.Join(dir, "requests.log")
		data, err := os.ReadFile(path)
		if err != nil {
			m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: i18n.Tf("ui.log.none", path)})
			m.refreshViewport()
			return m, nil
		}
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if len(lines) > 20 {
			lines = lines[len(lines)-20:]
		}
		var b strings.Builder
		b.WriteString(i18n.Tf("ui.log.header", len(lines), path))
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
			m.err = i18n.T("err.notLoggedIn")
			return m, nil
		}
		m.statusMessage = i18n.T("ui.compact.working")
		fn := m.compactCmd()
		return m, func() tea.Msg {
			return fn()
		}
	case cmd == "/usage":
		if m.creds == nil {
			m.err = i18n.T("err.notLoggedIn")
			return m, nil
		}
		text, err := fetchUsageForSource(m.cfg.APIBase, m.creds.Token, m.subs)
		if err != nil {
			m.err = i18n.Tf("ui.usage.fetchFailed", err.Error())
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
			m.err = i18n.Tf("err.noFolder", target)
			return m, nil
		}
		if err := os.Chdir(target); err != nil {
			m.err = "chdir: " + err.Error()
			return m, nil
		}
		m.cwd = shortenCWD(target)
		// Recreate the registry so tools know the new cwd.
		m.registry = tools.Default(target)
		m.statusMessage = "cwd → " + target
		return m, nil
	case cmd == "/cd":
		m.statusMessage = i18n.Tf("ui.cd.current", m.cwd)
		return m, nil
	case cmd == "/logout":
		_ = auth.Logout()
		m.creds = nil
		m.cli = nil
		m.loginMode = true
		m.pendingLink = nil
		m.history = nil
		m.uiSegments = nil
		m.lastEmittedIdx = 0
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint",
			body: i18n.T("login.loggedOutStartingFlow")})
		m.textarea.Placeholder = i18n.T("placeholder.confirmBrowser")
		m.statusMessage = ""
		m.refreshViewport()
		return m, func() tea.Msg { return autoStartLoginMsg{} }
	case cmd == "/new":
		m.history = nil
		m.uiSegments = nil
		m.lastEmittedIdx = 0
		m.session = sessions.New(m.current.ID, m.current.Provider, getCWDForBoot())
		m.statusMessage = i18n.T("ui.session.new")
		m.refreshViewport()
		return m, nil
	case cmd == "/sessions" || cmd == "/list":
		list, _ := sessions.List()
		var b strings.Builder
		b.WriteString(i18n.T("ui.sessions.header"))
		for i, s := range list {
			cur := " "
			if m.session != nil && s.ID == m.session.ID {
				cur = "•"
			}
			fmt.Fprintf(&b, "%s %2d. %s  [%s/%s]  %s\n",
				cur, i+1, s.Title, s.Provider, s.Model, s.UpdatedAt.Local().Format("2006-01-02 15:04"))
		}
		if len(list) == 0 {
			b.WriteString(i18n.T("ui.sessions.empty"))
		}
		b.WriteString(i18n.T("ui.sessions.switchHint"))
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
			m.err = i18n.Tf("ui.resume.notFound", arg)
			return m, nil
		}
		// Reload
		full, err := sessions.Load(picked.ID)
		if err != nil {
			m.err = i18n.Tf("ui.resume.loadFailed", err.Error())
			return m, nil
		}
		m.session = full
		m.history = append([]llm.AIMessage(nil), full.Messages...)
		m.uiSegments = nil
		m.lastEmittedIdx = 0
		m.replayHistoryToUI()
		m.statusMessage = i18n.Tf("ui.resume.resumed", full.Title)
		m.refreshViewport()
		return m, nil
	case strings.HasPrefix(cmd, "/title "):
		t := strings.TrimSpace(strings.TrimPrefix(cmd, "/title"))
		if t == "" || m.session == nil {
			return m, nil
		}
		m.session.Title = t
		_ = m.session.Save()
		m.statusMessage = i18n.Tf("ui.title.renamed", t)
		return m, nil
	case cmd == "/login":
		m.creds = nil
		m.cli = nil
		m.loginMode = true
		m.pendingLink = nil
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint",
			body: i18n.T("ui.login.startFlow")})
		m.refreshViewport()
		return m, func() tea.Msg { return autoStartLoginMsg{} }
	case cmd == "/permissions" || cmd == "/perms":
		perms, _ := agent.LoadPermissions()
		var b strings.Builder
		b.WriteString("\nPersistent permissions (~/.config/execai/permissions.json):\n")
		if perms != nil && len(perms.Tools) > 0 {
			b.WriteString("  always allowed tools: " + strings.Join(perms.Tools, ", ") + "\n")
		} else {
			b.WriteString(i18n.T("ui.perms.toolsEmpty"))
		}
		if perms != nil && len(perms.Exact) > 0 {
			b.WriteString(i18n.Tf("ui.perms.exactCount", len(perms.Exact)))
		}
		b.WriteString(i18n.T("ui.perms.resetHint"))
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
			m.err = i18n.Tf("ui.model.notFound", arg)
			return m, nil
		}
		m.current = *next
		m.cfg.SelectedModelID = next.ID
		_ = config.Save(m.cfg)
		m.cli = m.makeLLMClient()
		m.statusMessage = i18n.Tf("ui.model.switched", next.Provider, next.ID, next.Name)
		return m, tea.SetWindowTitle(m.titleString())
	}
	m.err = i18n.Tf("err.unknownCommand", cmd)
	return m, nil
}

// ===== agent loop in goroutine =====

func (m *tuiModel) startAgent(userMessage string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.streaming = true

	prog := teaProgramRef // global reference, set below in Run

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

// replyAsk hands the chosen label back to the blocked AskUser tool and records
// the exchange in the transcript, so scrollback shows what was asked and picked.
// An empty answer means the user dismissed the question.
func (m *tuiModel) replyAsk(answer string) {
	q := m.askQuestion
	if m.askReplyCh != nil {
		select {
		case m.askReplyCh <- answer:
		default:
		}
	}
	m.asking = false
	m.askQuestion = ""
	m.askOptions = nil
	m.askReplyCh = nil
	m.askFocus = 0

	shown := answer
	if shown == "" {
		shown = i18n.T("ask.dismissed")
	}
	m.emitSegment(segment{kind: "system_hint", body: i18n.Tf("ask.answered", q, shown)})
	m.refreshViewport()
}

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
		m.statusMessage = i18n.T("ui.approve.denied")
	case agent.ApproveTool:
		m.statusMessage = i18n.Tf("ui.approve.allowedTool", tool)
	case agent.ApproveExactArgs:
		m.statusMessage = i18n.T("ui.approve.allowedExact")
	case agent.ApproveAlways:
		m.statusMessage = i18n.Tf("approve.savedForever", tool)
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

// refreshViewport — in classic (alt-screen) mode redraws the viewport
// entirely. In Ink-style (default) — emits all NEW segments since the last
// call into terminal scrollback via Program.Println. The live stream
// is rendered in View() and doesn't go to scrollback — it gets there only
// when finalized as a segment.
func (m *tuiModel) refreshViewport() {
	if !m.cfg.ClassicTUI {
		// Ink-style: print fresh segments into scrollback.
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

// pasteThresholdRunes — minimum SINGLE-LINE paste size to collapse into a
// [Pasted #N] marker. Multiline pastes always collapse (newline check).
// 200 keeps API keys (40-160 chars: sk-ant-…, sk-ki-…, JWT fragments) visible
// and directly editable — collapsing them only obscured /connect input.
// Only long single-line blobs (minified JSON, base64, huge URLs) collapse.
// Smaller — treated as regular input. Also triggers if there is a \n.
const pasteThresholdRunes = 200

// interceptPaste — if msg is a large text input (Ctrl+V from the clipboard or
// bracketed paste from the terminal), save the content into pasteStore and
// insert a short marker into the textarea instead of the whole text. Returns true
// if intercepted — then the caller must not pass msg into textarea.Update.
func (m *tuiModel) interceptPaste(msg tea.KeyMsg) bool {
	// Works only for Runes input (KeyRunes = plain text).
	if msg.Type != tea.KeyRunes {
		return false
	}
	runes := msg.Runes
	if len(runes) == 0 {
		return false
	}
	// Threshold: either a long chunk, or a newline within a single input.
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
	// Save and insert the marker.
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

// expandPasteMarkers — replaces [Pasted #N ...] markers with the real texts
// from pasteStore. Used when sending input to the agent.
func (m *tuiModel) expandPasteMarkers(input string) string {
	if len(m.pasteStore) == 0 {
		return input
	}
	for _, p := range m.pasteStore {
		input = strings.ReplaceAll(input, p.marker, p.text)
	}
	return input
}

// emitQueue + drainer — Program.Println sends to p.msgs blockingly; calling it
// from Update() means deadlock (the event loop doesn't read while Update runs).
// So we send through a buffer, and a single goroutine sequentially calls
// Println outside the Update context. Order is preserved (channel FIFO).
var emitQueue = make(chan string, 256)
var emitStarted sync.Once

// emitSegment — in Ink-style puts the segment into the queue for Program.Println.
// In classic — no-op.
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
	// Non-blocking send: if the buffer is full, drop it (extremely unlikely
	// with 256 lines, but better to lose one segment than freeze the whole TUI).
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
		// result kept compact, cap it
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
	userLabel := i18n.T("status.notLogin")
	if m.creds != nil {
		userLabel = m.creds.Email
		if m.creds.Alias != "" {
			userLabel = m.creds.Email + " · " + m.creds.Alias
		}
	}
	provider := m.current.Provider
	model := m.current.ID
	if provider == "" {
		provider = i18n.T("status.unknown")
	}
	if model == "" {
		model = i18n.T("status.unknown")
	}
	source := "ExecAI"
	if m.subs != nil {
		source = m.subs.SourceLabel()
	}
	items := []string{
		statusKV(i18n.T("status.src"), " "+source+" "),
		statusKV(i18n.T("status.provider"), " "+provider+" "),
		statusKV(i18n.T("status.model"), " "+model+" "),
	}
	if m.cfg.ThinkingBudget > 0 {
		levels := map[int]string{1024: "low", 4096: "medium", 8192: "high", 16384: "xhigh", 32000: "max"}
		lab := levels[m.cfg.ThinkingBudget]
		if lab == "" {
			lab = fmt.Sprintf("%d", m.cfg.ThinkingBudget)
		}
		items = append(items, statusKV(i18n.T("status.effort"), " "+lab+" "))
	}
	if m.loopActive {
		items = append(items, statusKV(i18n.T("status.loop"), " "+m.loopInterval.String()+" "))
	}
	items = append(items,
		statusKV(i18n.T("status.user"), " "+userLabel+" "),
		statusKV(i18n.T("status.cwd"), " "+m.cwd+" "),
	)
	if m.streaming {
		items = append([]string{styleStatusKey.Render(i18n.T("status.working"))}, items...)
	}
	status := styleStatus.Width(m.width).MaxHeight(1).Render(strings.Join(items, " "))

	var bottom strings.Builder
	if m.asking {
		bottom.WriteString(m.renderAskBlock())
		bottom.WriteString("\n")
	}
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
		styleHelpKey.Render("Enter") + " " + i18n.T("help.enter") + " · " +
			styleHelpKey.Render("↑↓") + " " + i18n.T("help.history") + " · " +
			styleHelpKey.Render("Shift+Tab") + " " + i18n.T("help.effort") + " · " +
			styleHelpKey.Render("Shift+drag") + " " + i18n.T("help.copy") + " · " +
			styleHelpKey.Render(i18n.T("help.slashHelp")) + " · " +
			styleHelpKey.Render("Ctrl+C") + " " + i18n.T("help.stop") + " · " +
			styleHelpKey.Render("Ctrl+D") + " " + i18n.T("help.exit"))

	// Suggestions menu (autocomplete) — above the textarea if active.
	suggestPanel := m.renderSuggestPanel()
	thinkingSlider := m.renderThinkingSlider()

	// Ink-style: do NOT render the viewport (history is in scrollback), only
	// the live stream of the current reply (if any) + input + status.
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

// thinkingLevels — effort slider positions. Faster ←→ Smarter.
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

// currentThinkingIdx — index of the current budget in thinkingLevels (-1 if custom).
func (m *tuiModel) currentThinkingIdx() int {
	for i, lvl := range thinkingLevels {
		if lvl.budget == m.cfg.ThinkingBudget {
			return i
		}
	}
	return 0
}

// renderThinkingSlider — compact one-line indicator (when the picker is closed)
// or a full modal picker (when thinkingPickerActive). Only for
// Anthropic-compat subscriptions.
func (m *tuiModel) renderThinkingSlider() string {
	if m.subs == nil {
		return ""
	}
	// Thinking is supported by sources based on AnthropicClient or Claude CLI.
	if !sourceSupportsThinking(m.subs) {
		return ""
	}
	if m.thinkingPickerActive {
		return m.renderThinkingPicker()
	}
	// Compact indicator line when the picker is closed.
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#777"))
	cur := m.currentThinkingIdx()
	return dim.Render(fmt.Sprintf("🧠 effort: %s   %s", thinkingLevels[cur].label, i18n.T("effort.sliderHint")))
}

// renderThinkingPicker — modal effort-picker in Claude Code style.
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

	// Line 1: Faster / Smarter caption
	faster := dim.Render("Faster")
	smarter := dim.Render("Smarter")
	// Line 2: positions (current — green bold, others — gray)
	parts := []string{}
	for i, lvl := range thinkingLevels {
		if i == m.thinkingPickerIdx {
			parts = append(parts, active.Render(lvl.label))
		} else {
			parts = append(parts, normal.Render(lvl.label))
		}
	}
	row := strings.Join(parts, "  ")
	// ▲ pointer under the current position
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

// renderSuggestPanel — compact menu in the Claude Code list style.
// Returns an empty string if the menu is not active.
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
	// Scroll window around the focus.
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
			// i18n: hint contains either a key ("hint.help") or dynamic
			// text (model description). T() translates keys, returns text as-is.
			hint = "  " + dim.Render(i18n.T(it.hint))
		}
		row := label + hint
		if i == m.suggestFocus {
			row = focused.Render(" "+label+" ") + hint
		} else {
			row = normal.Render("  "+label) + hint
		}
		lines = append(lines, row)
	}
	footer := dim.Render(fmt.Sprintf(" %d/%d · %s",
		m.suggestFocus+1, len(m.suggestItems), i18n.T("suggest.footer")))
	body := strings.Join(lines, "\n") + "\n" + footer
	return box.Render(body) + "\n"
}

// ===== helpers =====

// renderApproveBlock — dialog with buttons above the textarea. Arrows/Tab switch
// focus, Enter — select, Esc — reject. Hotkeys y/a/s/f/n too.
// renderAskBlock draws the question from the AskUser tool: options stacked
// vertically (unlike the approve buttons) because each carries a description,
// and a horizontal row would truncate them into uselessness.
func (m *tuiModel) renderAskBlock() string {
	// Width is unknown until the first WindowSizeMsg (and stays 0 in harnesses
	// that never set the tty size). Assume a classic 80 rather than clamping to
	// the minimum — otherwise every description collapses to one word.
	width := m.width
	if width <= 0 {
		width = 80
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5FAFFF")).
		Foreground(lipgloss.Color("#FAFAFA")).
		Padding(0, 1).
		Width(width - 2)

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5FAFFF")).
		Render(i18n.T("ask.title")))
	b.WriteString("\n")
	b.WriteString(m.askQuestion)
	b.WriteString("\n\n")

	focused := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5FAFFF"))
	normal := lipgloss.NewStyle().Foreground(lipgloss.Color("#CCC"))
	desc := lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))

	for i, o := range m.askOptions {
		marker, style := "  ", normal
		if i == m.askFocus {
			marker, style = "▸ ", focused
		}
		b.WriteString(fmt.Sprintf("%s%d. %s", marker, i+1, style.Render(o.Label)))
		if o.Description != "" {
			b.WriteString("\n     " + desc.Render(truncateLine(o.Description, width-10)))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(desc.Render(i18n.T("ask.hint")))
	return box.Render(b.String())
}

// truncateLine keeps a description on one line — the box has no room to wrap
// and a wrapped description pushes the options off screen.
func truncateLine(s string, max int) string {
	if max < 10 {
		max = 10
	}
	r := []rune(strings.ReplaceAll(s, "\n", " "))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max-1]) + "…"
}

func (m *tuiModel) renderApproveBlock() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FFD75F")).
		Foreground(lipgloss.Color("#FAFAFA")).
		Padding(0, 1).
		Width(m.width - 2)

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFD75F")).
		Render(i18n.Tf("approve.title", m.approveTool))

	body := m.approveSummary
	if len(body) > m.width*8 {
		body = body[:m.width*8] + "…"
	}
	cmd := lipgloss.NewStyle().Foreground(lipgloss.Color("#A8FF60")).Render(body)

	buttons := []struct{ label, hotkey string }{
		{i18n.T("approve.once"), "y"},
		{i18n.Tf("approve.allToolSession", m.approveTool), "a"},
		{i18n.T("approve.thisCmdSession"), "s"},
		{i18n.T("approve.forever"), "f"},
		{i18n.T("approve.deny"), "n"},
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

	hint := styleHelpDim.Render(i18n.T("ui.approve.navHint"))

	content := title + "\n\n" + cmd + "\n\n" + row + "\n" + hint
	return box.Render(content)
}

// bootAfterLogin fetches models, picks the default, initializes cli.
// Called either at startup if creds exist, or after a successful login.
func (m *tuiModel) bootAfterLogin() error {
	// Cache-first: the agent ALWAYS starts, even if the network/API/models_public
	// are temporarily unavailable. If the network dropped — use the cache; if that's
	// also missing — a built-in stub (one model), the user sees a warning in chat.
	res := llm.FetchModelsCached(m.cfg.APIBase, m.creds.Token)
	models := res.Models
	if notice := res.OfflineNotice(); notice != "" {
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: notice})
	}
	if len(models) == 0 {
		// Should not happen — embeddedFallback guarantees >=1. Just in case.
		return fmt.Errorf("%s", i18n.T("ui.boot.modelsFallbackFailed"))
	}
	current := llm.PickDefault(models, m.cfg.SelectedModelID)
	if m.cfg.SelectedModelID != current.ID {
		m.cfg.SelectedModelID = current.ID
		_ = config.Save(m.cfg)
	}
	// Guarantee the user-memory folder and MEMORY.md index exist
	// before loading — then the LLM always sees where to write.
	_, _ = agent.EnsureUserMemoryIndex()
	memory := agent.LoadMemory(getCWDForBoot())
	m.system = agent.SystemPrompt(getCWDForBoot(), m.registry.Names(), memory)
	m.models = models
	m.execAIModels = models
	m.current = *current
	m.loginMode = false
	// If an external subscription is active — override m.models/m.current with its catalog.
	m.applySubscriptionSource()
	m.textarea.Placeholder = i18n.T("placeholder.chat")

	// Restore the last session: if there's a fresh one (<24h) in the same
	// CWD — continue it, otherwise create a new one.
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

// replayHistoryToUI — draw segments for an already existing history
// (after a session resume) — so the user sees the context.
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

// persistSession writes the current history into the session file. Called after
// every agentDoneMsg and on model switch.
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
	return i18n.Tf("ui.welcome",
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
			return i18n.Tf("ui.toolSummary.write", p, size)
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

// truncate cuts to n runes, not bytes. Byte slicing split multi-byte
// characters in half and the terminal rendered the tail as garbage — visible
// on any tool_call whose arguments contain Cyrillic.
func truncate(s string, n int) string {
	if n < 1 {
		n = 1
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// langOption — a language for the /lang picker: code + native name.
var langOptions = []struct {
	code   string
	native string
}{
	{"en", "English"},
	{"ru", "Русский"},
	{"es", "Español"},
	{"de", "Deutsch"},
	{"zh", "中文 (简体)"},
}

// filterLangOptions — suggest menu for '/lang '. Selection → auto-submit
// (insert without trailing space). The current language is marked with a check.
func filterLangOptions(prefix string) []suggestItem {
	low := strings.ToLower(strings.TrimSpace(prefix))
	cur := i18n.Locale()
	out := []suggestItem{}
	for _, lo := range langOptions {
		if low != "" && !strings.Contains(lo.code, low) && !strings.Contains(strings.ToLower(lo.native), low) {
			continue
		}
		label := lo.code + " — " + lo.native
		if lo.code == cur {
			label += " ✓"
		}
		out = append(out, suggestItem{
			insert: "/lang " + lo.code,
			label:  label,
			hint:   "",
		})
	}
	return out
}

// initI18nLocale — sets the active i18n locale:
// 1) cfg.Locale if explicitly set — use as-is.
// 2) Otherwise — Detect from $LANG/$LC_ALL, if supported — use it.
// 3) Otherwise — default "en".
// Returns true if auto-detect kicked in (for the boot-hint).
func initI18nLocale(cfg *config.Config) (autoDetected bool, envValue string) {
	if cfg != nil && cfg.Locale != "" {
		i18n.SetLocale(cfg.Locale)
		return false, ""
	}
	src := ""
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(env); v != "" {
			src = env + "=" + v
			break
		}
	}
	detected := i18n.Detect()
	if detected != "" {
		got := i18n.SetLocale(detected)
		if got == detected {
			return true, src
		}
	}
	i18n.SetLocale(i18n.DefaultLocale)
	return false, ""
}

// availableLocalesStr — "en, ru, es, de, zh" for messages.
func availableLocalesStr() string {
	return strings.Join(i18n.Available(), ", ")
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

// teaProgramRef holds a reference to the current tea.Program — needed so the
// streamer can send messages from a goroutine via prog.Send. Overwritten on
// every RunTUI; for a single program per process that's enough.
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
