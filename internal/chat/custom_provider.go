// /connect custom — a user-defined provider: any endpoint that speaks the
// OpenAI chat/completions protocol or the Anthropic messages protocol (a
// local LM Studio / vLLM / llama.cpp server, LiteLLM, a corporate gateway,
// DeepSeek, Groq…).
//
// Two ways in. The bare command asks the questions one at a time — protocol,
// address, key — which is how most people meet it. The one-line form is for
// those who already know the answers and for scripts:
//
//	/connect custom "LM Studio" openai http://localhost:1234/v1
//	/connect custom corp anthropic https://llm.corp.local sk-xxx --header "X-Tenant: dev"
//	/connect custom vllm openai http://gpu:8000 --models qwen3-coder,llama-4
package chat

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/velesbsdllc/agent-vbai/internal/connect"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// Wizard steps.
const (
	wizName = iota
	wizAPI
	wizURL
	wizKey
	wizModels // only when the endpoint reported no models
)

// customWizard holds the answers given so far.
type customWizard struct {
	step   int
	c      connect.Custom
	status int // HTTP status of the model request, for the wizModels question
}

// startCustomWizard opens the questions. The answers go through the chat
// input; Esc cancels at any step.
func (m *tuiModel) startCustomWizard() string {
	m.customWiz = &customWizard{step: wizName}
	m.textarea.Placeholder = i18n.T("custom.wiz.placeholder.name")
	return i18n.T("custom.wiz.intro") + "\n\n" + i18n.T("custom.wiz.name")
}

func (m *tuiModel) cancelCustomWizard() {
	m.customWiz = nil
	m.textarea.Placeholder = i18n.T("placeholder.chat")
	m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: i18n.T("custom.wiz.cancelled")})
	m.refreshViewport()
}

// customWizardInput takes one answer and asks the next question.
func (m *tuiModel) customWizardInput(line string) (tea.Model, tea.Cmd) {
	w := m.customWiz
	say := func(body string) (tea.Model, tea.Cmd) {
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: body})
		m.refreshViewport()
		return m, nil
	}
	echo := func(text string) {
		m.uiSegments = append(m.uiSegments, segment{kind: "user", body: text})
	}
	switch w.step {
	case wizName:
		echo(line)
		id := subscriptions.CustomID(line)
		if err := subscriptions.ValidateCustomID(id); err != nil {
			return say("✗ " + err.Error() + "\n\n" + i18n.T("custom.wiz.name"))
		}
		w.c.Name = line
		w.step = wizAPI
		m.textarea.Placeholder = i18n.T("custom.wiz.placeholder.api")
		return say(i18n.Tf("custom.wiz.idIs", id) + "\n\n" + i18n.T("custom.wiz.api"))
	case wizAPI:
		echo(line)
		api, err := connect.ParseAPI(line)
		if err != nil {
			return say("✗ " + err.Error() + "\n\n" + i18n.T("custom.wiz.api"))
		}
		w.c.API = api
		w.step = wizURL
		m.textarea.Placeholder = i18n.T("custom.wiz.placeholder.url")
		if api == subscriptions.APIAnthropic {
			return say(i18n.T("custom.wiz.urlAnthropic"))
		}
		return say(i18n.T("custom.wiz.urlOpenAI"))
	case wizURL:
		echo(line)
		if _, err := connect.NormalizeCustomBase(w.c.API, line); err != nil {
			return say("✗ " + err.Error())
		}
		w.c.BaseURL = line
		w.step = wizKey
		m.textarea.Placeholder = i18n.T("custom.wiz.placeholder.key")
		return say(i18n.T("custom.wiz.key"))
	case wizKey:
		// The key itself never reaches the transcript.
		if line == "" {
			echo(i18n.T("custom.wiz.noKey"))
		} else {
			echo("••••••••")
		}
		w.c.APIKey = line
		sub, status, err := connect.PrepareCustom(w.c)
		if err != nil {
			m.customWiz = nil
			m.textarea.Placeholder = i18n.T("placeholder.chat")
			return say("✗ " + err.Error())
		}
		if connect.KeyRejected(status) && status != 429 {
			w.step = wizKey
			return say(i18n.Tf("custom.wiz.keyRejected", status) + "\n\n" + i18n.T("custom.wiz.key"))
		}
		if len(sub.AvailableModels) == 0 {
			w.status = status
			w.step = wizModels
			m.textarea.Placeholder = i18n.T("custom.wiz.placeholder.models")
			return say(i18n.Tf("custom.wiz.models", status))
		}
		m.customWiz = nil
		m.textarea.Placeholder = i18n.T("placeholder.chat")
		return say(m.saveCustom(sub))
	case wizModels:
		echo(line)
		w.c.Models = []string{line}
		sub, _, err := connect.PrepareCustom(w.c)
		m.customWiz = nil
		m.textarea.Placeholder = i18n.T("placeholder.chat")
		if err != nil {
			return say("✗ " + err.Error())
		}
		return say(m.saveCustom(sub))
	}
	m.customWiz = nil
	return m, nil
}

// connectCustom is the one-line form: everything is known up front.
func (m *tuiModel) connectCustom(c connect.Custom) string {
	sub, status, err := connect.PrepareCustom(c)
	if err != nil {
		return "✗ " + err.Error()
	}
	if connect.KeyRejected(status) && status != 429 {
		return i18n.Tf("custom.keyRejected", status, sub.BaseURL)
	}
	return m.saveCustom(sub)
}

// saveCustom stores a prepared custom subscription, switches to it and
// reports what came of it.
func (m *tuiModel) saveCustom(sub subscriptions.Subscription) string {
	if m.subs == nil {
		m.subs, _ = subscriptions.Load()
	}
	m.subs.Add(sub)
	if err := m.subs.Activate(sub.Provider); err != nil {
		return "✗ " + err.Error()
	}
	if err := m.subs.Save(); err != nil {
		return i18n.Tf("subs.saveError", err)
	}
	m.applySubscriptionSource()
	if m.loginMode {
		m.loginMode = false
		m.textarea.Placeholder = i18n.T("placeholder.chat")
	}
	msg := i18n.Tf("custom.connected", subscriptions.DisplayName(sub), sub.Provider, sub.API, sub.BaseURL)
	if n := len(m.models); n > 0 {
		msg += "\n" + i18n.Tf("custom.connectedModels", n, m.current.ID)
	} else {
		msg += "\n" + i18n.T("subs.connect.noModels")
	}
	return msg + "\n" + i18n.Tf("custom.connectedHint", sub.Provider)
}

// typedModel accepts a model ID that is not in the catalog — for a
// subscription whose provider listed nothing, a custom endpoint that does not
// list everything it serves, or a CLI given a pinned ID. The ID is remembered
// in the subscription so it is still offered after a restart. Built-in API
// providers with a live list keep rejecting unknown IDs: there it is almost
// always a typo.
func (m *tuiModel) typedModel(id string) (*llm.Model, bool) {
	if m.subs == nil || strings.ContainsAny(id, " \t") || id == "" {
		return nil, false
	}
	active := m.subs.ActiveSubscription()
	// The CLIs validate --model themselves and accept pinned IDs their list
	// leaves out (claude-opus-5-5 next to the "opus" alias).
	cli := active != nil && (active.Provider == subscriptions.SourceClaudeCLI || active.Provider == subscriptions.SourceCodexCLI)
	if active == nil || (len(m.models) > 0 && !active.IsCustom() && !cli) {
		return nil, false
	}
	sub := m.subs.Subscriptions[active.Provider]
	sub.ManualModels = append(sub.ManualModels, id)
	m.subs.Subscriptions[active.Provider] = sub
	_ = m.subs.Save()
	m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: i18n.Tf("ui.model.typed", id)})
	return &llm.Model{ID: id, Provider: active.Provider, Name: id, Tier: "standard", HasTools: true}, true
}

// parseCustomArgs reads the one-line form:
//
//	<name> <openai|anthropic> <base_url> [api_key] [--models a,b] [--model a] [--header "K: V"]
func parseCustomArgs(s string) (connect.Custom, error) {
	args, err := splitArgs(s)
	if err != nil {
		return connect.Custom{}, err
	}
	var c connect.Custom
	var pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--models" || a == "--model" || a == "-m":
			if i+1 >= len(args) {
				return c, fmt.Errorf("%s: нужен список моделей", a)
			}
			i++
			c.Models = append(c.Models, args[i])
		case strings.HasPrefix(a, "--models=") || strings.HasPrefix(a, "--model="):
			c.Models = append(c.Models, a[strings.Index(a, "=")+1:])
		case a == "--header" || a == "-H":
			if i+1 >= len(args) {
				return c, fmt.Errorf("%s: нужен заголовок вида \"Имя: значение\"", a)
			}
			i++
			k, v, err := parseHeader(args[i])
			if err != nil {
				return c, err
			}
			if c.Headers == nil {
				c.Headers = map[string]string{}
			}
			c.Headers[k] = v
		case a == "--key":
			if i+1 >= len(args) {
				return c, errors.New("--key: нужен ключ")
			}
			i++
			c.APIKey = args[i]
		case strings.HasPrefix(a, "--"):
			return c, fmt.Errorf("неизвестный флаг %s", a)
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) < 3 {
		return c, errors.New("нужны имя, тип интерфейса и адрес")
	}
	if len(pos) > 4 {
		return c, errors.New("лишние аргументы — имя с пробелами возьми в кавычки")
	}
	c.Name, c.API, c.BaseURL = pos[0], pos[1], pos[2]
	if len(pos) == 4 {
		c.APIKey = pos[3]
	}
	return c, nil
}

func parseHeader(h string) (string, string, error) {
	i := strings.IndexAny(h, ":=")
	if i <= 0 {
		return "", "", fmt.Errorf("заголовок %q: нужен вид \"Имя: значение\"", h)
	}
	k, v := strings.TrimSpace(h[:i]), strings.TrimSpace(h[i+1:])
	if k == "" || strings.ContainsAny(k, " \t\r\n") || strings.ContainsAny(v, "\r\n") {
		return "", "", fmt.Errorf("заголовок %q некорректен", h)
	}
	return k, v, nil
}

// splitArgs splits like a shell does for the cases that matter here: spaces
// separate, "double" and 'single' quotes group.
func splitArgs(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	quote := rune(0)
	inArg := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
			inArg = true
		case r == ' ' || r == '\t':
			if inArg {
				out = append(out, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if quote != 0 {
		return nil, errors.New("незакрытая кавычка")
	}
	if inArg {
		out = append(out, cur.String())
	}
	return out, nil
}
