package ide

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// modelFacing — functions whose Russian text goes to the model or a log,
// never to the panel: the wake-up prompt, the editor context block of a
// prompt, a stderr line.
var modelFacing = map[string]bool{"fireWake": true, "buildPrompt": true, "persistLocked": true}

// Every string the panel can show goes through i18n: no Cyrillic literal is
// left in internal/ide outside the model-facing functions.
func TestNoRussianLiteralsForThePanel(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if ok && modelFacing[fn.Name.Name] {
				continue
			}
			ast.Inspect(d, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if ok && lit.Kind == token.STRING && cyrillic.MatchString(lit.Value) {
					t.Errorf("%s: Russian text for the panel: %s", fset.Position(lit.Pos()), lit.Value)
				}
				return true
			})
		}
	}
}

// The state pickers: the security levels and the sources to connect.
func TestStatePickers_English(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("en")
	for _, it := range append(securityOptions(), ConnectableOptions()...) {
		if cyrillic.MatchString(it.Label) {
			t.Errorf("en: %q", it.Label)
		}
	}
	i18n.SetLocale("ru")
	if l := securityOptions()[0].Label; !cyrillic.MatchString(l) {
		t.Errorf("ru security level %q", l)
	}
}

// Signing in: a server that refuses, an expired code.
func TestLogin_English(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("en")
	isolateIDE(t)
	expired := false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/start") && !expired:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"down"}`))
		case strings.HasSuffix(r.URL.Path, "/start"):
			_, _ = w.Write([]byte(`{"link_token":"t","user_code":"ABCD","verify_uri":"https://web.example.org/link","expires_in":60,"poll_interval":1}`))
		default:
			_, _ = w.Write([]byte(`{"status":"expired"}`))
		}
	}))
	defer api.Close()
	cfg := &config.Config{APIBase: api.URL}
	var events []Out
	emit := func(o Out) { events = append(events, o) }

	_, err := login(context.Background(), cfg, emit)
	if err == nil || cyrillic.MatchString(err.Error()) {
		t.Fatalf("refused start: %v", err)
	}
	expired = true
	_, err = login(context.Background(), cfg, emit)
	if err == nil || err.Error() != "the confirmation code expired — try again" {
		t.Fatalf("expired: %v", err)
	}
	raw, _ := json.Marshal(events)
	if cyrillic.Match(raw) {
		t.Fatalf("events %s", raw)
	}
}

// Connecting a source: the refusals and the success the panel shows.
func TestConnect_English(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("en")
	isolateIDE(t)
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if strings.HasSuffix(r.URL.Path, "/models") {
			_, _ = w.Write([]byte(`{"data":[{"id":"qwen3-coder"},{"id":"llama-4"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer llmSrv.Close()

	bad := []In{
		{Value: "nope", Key: "k"},
		{Value: subscriptions.SourceOpenAI},
		{Value: subscriptions.SourceCustom, Label: "corp", API: "grpc", BaseURL: llmSrv.URL},
		{Value: subscriptions.SourceCustom, Label: "corp", API: "openai", BaseURL: ""},
		{Value: subscriptions.SourceCustom, Label: "corp", API: "openai", BaseURL: "ftp://x"},
		{Value: subscriptions.SourceCustom, Label: "!!!", API: "openai", BaseURL: llmSrv.URL},
		{Value: subscriptions.SourceCustom, Label: strings.Repeat("a", 41), API: "openai", BaseURL: llmSrv.URL},
		{Value: subscriptions.SourceOllama, BaseURL: "http://127.0.0.1:9"},
	}
	for _, in := range bad {
		subs, _ := subscriptions.Load()
		note, err := connect(subs, in)
		if err == nil {
			t.Errorf("%+v connected: %q", in, note)
			continue
		}
		if cyrillic.MatchString(err.Error()) {
			t.Errorf("%+v: %q", in, err.Error())
		}
	}
	subs, _ := subscriptions.Load()
	note, err := connect(subs, In{Value: subscriptions.SourceCustom, Label: "corp", API: "openai", BaseURL: llmSrv.URL + "/v1"})
	if err != nil || !strings.HasPrefix(note, "connected: ") || cyrillic.MatchString(note) {
		t.Fatalf("custom: %q %v", note, err)
	}
	if err := subs.Activate("not-there"); err == nil || cyrillic.MatchString(err.Error()) {
		t.Fatalf("activate: %v", err)
	}

	// The same refusal in Russian is Russian.
	i18n.SetLocale("ru")
	subs, _ = subscriptions.Load()
	if _, err := connect(subs, In{Value: subscriptions.SourceOpenAI}); err == nil || !cyrillic.MatchString(err.Error()) {
		t.Fatalf("ru: %v", err)
	}
}

// A permission question as the panel gets it, whole.
func TestQuestionEvent_English(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("en")
	_, _, out := testApprover().question("Bash", "ls -la")
	raw, _ := json.Marshal(out)
	if cyrillic.Match(raw) {
		t.Fatalf("event %s", raw)
	}
}
