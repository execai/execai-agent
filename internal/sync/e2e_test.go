package sync

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// machine — one «computer»: its own home and config directory. The
// environment is switched to it before each step (use), the way the agent
// finds its files.
type machine struct {
	t    *testing.T
	name string
	base string
	svc  *Service
}

func newMachine(t *testing.T, stub *stubServer, name string) *machine {
	t.Helper()
	m := &machine{t: t, name: name, base: t.TempDir()}
	m.use()
	if err := config.SaveCredentials(&config.Credentials{Token: "tok-" + name, Email: "user@example.com", AgentID: "agent-" + name}); err != nil {
		t.Fatal(err)
	}
	svc, err := New(stub.URL)
	if err != nil {
		t.Fatal(err)
	}
	m.svc = svc
	return m
}

func (m *machine) use() {
	m.t.Setenv("HOME", m.base)
	m.t.Setenv("USERPROFILE", m.base)
	m.t.Setenv("XDG_CONFIG_HOME", filepath.Join(m.base, ".config"))
	m.t.Setenv("APPDATA", filepath.Join(m.base, "AppData"))
}

func (m *machine) dir() string { return m.svc.Paths.ConfigDir }

func (m *machine) writeFile(rel, content string) {
	m.t.Helper()
	p := filepath.Join(m.dir(), rel)
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		m.t.Fatal(err)
	}
}

func (m *machine) readFile(rel string) string {
	b, err := os.ReadFile(filepath.Join(m.dir(), rel))
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

// converse runs a flow with the given answers and returns every step.
func (m *machine) converse(f Flow, answers ...string) []Step {
	m.t.Helper()
	m.use()
	ctx := context.Background()
	steps := []Step{f.Start(ctx)}
	for _, a := range answers {
		last := steps[len(steps)-1]
		if last.Done() {
			m.t.Fatalf("flow ended before answer %q: %+v", a, steps)
		}
		steps = append(steps, f.Answer(ctx, a))
	}
	return steps
}

func allText(steps []Step) string {
	var b strings.Builder
	for _, s := range steps {
		b.WriteString(s.Text + "\n" + s.Prompt + "\n")
	}
	return b.String()
}

const password = "correct horse battery"

// TestTwoComputers — two config directories against the stub. First
// computer: /sync on with a password, memory and the whole config go up.
// Second: /sync on with the same password — the key arrives, memory,
// settings and provider keys are there, the active source is the first
// one's; a wrong password says so. Provider keys are inside the encrypted
// config blob and nowhere in the server's bytes or the manifest. keys
// export / import still works without sync as the fallback.
func TestTwoComputers(t *testing.T) {
	i18n.SetLocale("ru")
	defer i18n.SetLocale("en")
	stub := newStub(t)
	const zaiKey = "sk-zai-SECRET-0123456789"

	// ---- computer A ----
	a := newMachine(t, stub, "a")
	a.writeFile("config.json", `{"api_base":"`+stub.URL+`","selected_model_id":"glm-5.3","thinking_budget":4096,"locale":"ru"}`)
	a.writeFile("permissions.json", `{"always_allowed_tools":["Read","Grep"],"always_allowed_exact":null}`)
	a.writeFile("memory/MEMORY.md", "- [Роль](user_role.md) — кто я\n")
	a.writeFile("memory/user_role.md", "Пишу на Go\n")
	subsA := &subscriptions.Store{Subscriptions: map[string]subscriptions.Subscription{}}
	subsA.Add(subscriptions.Subscription{Provider: subscriptions.SourceZAI, APIKey: zaiKey, Plan: "coding"})
	_ = subsA.Activate(subscriptions.SourceZAI)
	if err := subsA.Save(); err != nil {
		t.Fatal(err)
	}

	steps := a.converse(a.svc.OnFlow(), "short", password, "not the same password", password, password)
	if !steps[0].Secret || !strings.Contains(steps[0].Prompt, "Придумайте пароль") {
		t.Fatalf("first step: %+v", steps[0])
	}
	if !strings.Contains(steps[1].Text, "Слишком короткий") {
		t.Fatalf("short password accepted: %+v", steps[1])
	}
	if !strings.Contains(steps[3].Text, "не совпали") {
		t.Fatalf("mismatch not caught: %+v", steps[3])
	}
	done := steps[len(steps)-1]
	if !done.Done() || !strings.Contains(done.Text,
		"Синхронизируется: память (файлов: 2), настройки и ключи провайдеров — всё зашифровано на этом компьютере") {
		t.Fatalf("summary: %q", done.Text)
	}
	if strings.Contains(done.Text, "остаются на этом компьютере") {
		t.Fatalf("old wording: %q", done.Text)
	}
	if fi, err := os.Stat(crypto.KeyPath(a.dir())); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("sync.key: %v %v", fi, err)
	}
	if stub.identity == nil || len(stub.blobs) != 3 {
		t.Fatalf("server: identity=%v blobs=%d (want memory×2 + config)", stub.identity != nil, len(stub.blobs))
	}

	// The keys ARE in the decrypted config blob, and NOT in any byte the
	// server holds, nor in the manifest.
	keyA, _ := crypto.LoadIdentity(a.dir())
	for _, c := range stub.allContent() {
		if strings.Contains(string(c), zaiKey) {
			t.Fatal("a provider key is readable on the server")
		}
	}
	_, _, data, err := unseal(keyA, stub.blob(KindConfig, NameConfig).content)
	if err != nil {
		t.Fatal(err)
	}
	files, err := unpackFiles(data)
	if err != nil || !strings.Contains(string(files["subscriptions.json"]), zaiKey) ||
		!strings.Contains(string(files["permissions.json"]), "Grep") || !strings.Contains(string(files["config.json"]), "glm-5.3") {
		t.Fatalf("config blob: %v %q", err, files)
	}
	man, _ := json.Marshal(stub.machines["agent-a"])
	if strings.Contains(string(man), zaiKey) || strings.Contains(string(man), "api_key") {
		t.Fatalf("manifest leaks: %s", man)
	}
	if !strings.Contains(string(man), `"Z.ai Coding Plan"`) || !strings.Contains(string(man), `"ExecAI"`) {
		t.Fatalf("manifest sources: %s", man)
	}

	// ---- computer B ----
	b := newMachine(t, stub, "b")
	b.writeFile("config.json", `{"api_base":"`+stub.URL+`","selected_model_id":"other-model","classic_tui":true}`)
	subsB := &subscriptions.Store{Subscriptions: map[string]subscriptions.Subscription{}}
	subsB.Add(subscriptions.Subscription{Provider: subscriptions.SourceKimiAPI, APIKey: "sk-moonshot-B"})
	_ = subsB.Activate(subscriptions.SourceKimiAPI)
	_ = subsB.Save()

	steps = b.converse(b.svc.OnFlow(), "wrong password here", password)
	if !strings.Contains(steps[0].Prompt, "пароль синхронизации с первого компьютера") || !steps[0].Secret {
		t.Fatalf("join prompt: %+v", steps[0])
	}
	if !strings.Contains(steps[1].Text, "Пароль не подошёл") || steps[1].Done() {
		t.Fatalf("wrong password: %+v", steps[1])
	}
	if strings.Contains(allText(steps), password) {
		t.Fatal("the password is echoed back")
	}
	done = steps[2]
	if !done.Done() || !strings.Contains(done.Text, "Ключ получен") {
		t.Fatalf("join: %q", done.Text)
	}
	keyB, err := crypto.LoadIdentity(b.dir())
	if err != nil || keyB.String() != keyA.String() {
		t.Fatalf("key did not arrive: %v", err)
	}
	if got := b.readFile("memory/user_role.md"); got != "Пишу на Go\n" {
		t.Fatalf("memory on B: %q", got)
	}
	var cfgB map[string]any
	_ = json.Unmarshal([]byte(b.readFile("config.json")), &cfgB)
	if cfgB["selected_model_id"] != "glm-5.3" || cfgB["api_base"] != stub.URL {
		t.Fatalf("settings on B: %v", cfgB)
	}
	if !strings.Contains(b.readFile("permissions.json"), `"Grep"`) {
		t.Fatalf("permissions on B: %s", b.readFile("permissions.json"))
	}
	b.use()
	subsB, _ = subscriptions.Load()
	if subsB.Subscriptions[subscriptions.SourceZAI].APIKey != zaiKey {
		t.Fatal("A's provider key did not arrive on B")
	}
	if subsB.Active != subscriptions.SourceZAI {
		t.Fatalf("active on B = %q, want A's (the config is global)", subsB.Active)
	}
	if subsB.Subscriptions[subscriptions.SourceKimiAPI].APIKey != "sk-moonshot-B" {
		t.Fatal("B's own source was lost on joining")
	}

	// ---- an edit on A reaches B; B's own source reaches A ----
	a.use()
	a.writeFile("memory/user_role.md", "Пишу на Go и Rust\n")
	_ = os.Remove(filepath.Join(a.dir(), "memory", "MEMORY.md"))
	if out := a.svc.SyncNow(context.Background()); !strings.Contains(out, "Готово") {
		t.Fatalf("now on A: %s", out)
	}
	if !strings.Contains(a.readFile("subscriptions.json"), "sk-moonshot-B") {
		t.Fatal("B's source did not reach A")
	}
	b.use()
	b.svc.SyncNow(context.Background())
	if got := b.readFile("memory/user_role.md"); got != "Пишу на Go и Rust\n" {
		t.Fatalf("edit not on B: %q", got)
	}
	if b.readFile("memory/MEMORY.md") != "<missing>" {
		t.Fatal("deletion not on B")
	}

	// ---- the fallback without sync: keys export on A, import on C ----
	a.use()
	out := a.svc.KeysExport()
	code := regexp.MustCompile(`execai-keys-1:\S+`).FindString(out)
	phrase := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "    ") {
			phrase = strings.TrimSpace(l)
		}
	}
	if code == "" || len(strings.Fields(phrase)) != PhraseWords || strings.Contains(out, zaiKey) ||
		!strings.Contains(out, "без сервера и без тарифа") {
		t.Fatalf("export: %s", out)
	}
	c := newMachine(t, stub, "c")
	subsC := &subscriptions.Store{Subscriptions: map[string]subscriptions.Subscription{}}
	subsC.Add(subscriptions.Subscription{Provider: subscriptions.SourceOpenAI, APIKey: "sk-openai-C"})
	_ = subsC.Activate(subscriptions.SourceOpenAI)
	_ = subsC.Save()
	wrappedCode := code[:20] + "\n" + code[20:] // a messenger may wrap it
	steps = c.converse(c.svc.KeysImportFlow(""), wrappedCode, "wrong words here", phrase)
	if !steps[0].Quiet || !strings.Contains(steps[2].Text, "Фраза не подошла") {
		t.Fatalf("import steps: %+v", steps)
	}
	if !steps[3].Done() || !strings.Contains(steps[3].Text, "Z.ai Coding Plan") {
		t.Fatalf("import: %q", steps[3].Text)
	}
	c.use()
	subsC, _ = subscriptions.Load()
	if subsC.Subscriptions[subscriptions.SourceZAI].APIKey != zaiKey || subsC.Active != subscriptions.SourceOpenAI {
		t.Fatalf("import on C: active=%q", subsC.Active)
	}

	// ---- status ----
	b.use()
	st := b.svc.Status()
	if !strings.Contains(st, "Синхронизация включена") || !strings.Contains(st, "ключи провайдеров") {
		t.Fatalf("status: %s", st)
	}
}

// TestNoPlan — without Work/Max the setup stops at once and says where to
// get it; nothing reaches the server.
func TestNoPlan(t *testing.T) {
	i18n.SetLocale("ru")
	defer i18n.SetLocale("en")
	for _, tariff := range []string{"", "week_start"} {
		stub := newStub(t)
		stub.tariff = tariff
		c := newMachine(t, stub, "c")
		steps := c.converse(c.svc.OnFlow())
		if !steps[0].Done() || !strings.Contains(steps[0].Text, "«Работа»") || !strings.Contains(steps[0].Text, "«Макс»") {
			t.Fatalf("%q: refusal: %+v", tariff, steps[0])
		}
		if stub.identity != nil || len(stub.blobs) != 0 {
			t.Fatalf("%q: something reached the server", tariff)
		}
		if out := c.svc.SyncNow(context.Background()); !strings.Contains(out, "«Работа»") {
			t.Fatalf("%q: now: %s", tariff, out)
		}
	}
}

// TestNotSignedIn — no token, no sync, a hint to sign in.
func TestNotSignedIn(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	c := newMachine(t, stub, "c")
	_ = config.DeleteCredentials()
	if v := c.svc.Gate.Check(context.Background(), true); v != NotSignedIn {
		t.Fatalf("verdict %v", v)
	}
	if s := c.converse(c.svc.OnFlow()); !strings.Contains(s[0].Text, "sign in") {
		t.Fatalf("%+v", s[0])
	}
}

// TestKeyChoice — B has its own key already: taking the server's keeps the
// old one next to it; keeping its own overwrites the server, and A then
// pauses instead of fighting over the files.
func TestKeyChoice(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.writeFile("memory/a.md", "from A\n")
	a.converse(a.svc.OnFlow(), password, password)

	b := newMachine(t, stub, "b")
	own, _, _ := crypto.LoadOrGenerate(b.dir())
	b.writeFile("memory/b.md", "from B\n")

	steps := b.converse(b.svc.OnFlow(), password, "1")
	if !strings.Contains(steps[1].Prompt, "1 —") || !strings.Contains(steps[2].Text, "previous key is kept") {
		t.Fatalf("choice: %+v", steps)
	}
	if got, _ := crypto.LoadIdentity(b.dir()); got.String() == own.String() {
		t.Fatal("server key not taken")
	}
	if b.readFile("memory/a.md") != "from A\n" {
		t.Fatal("A's memory not on B after taking the key")
	}

	// C keeps its own key: the server is rewritten, A pauses.
	c := newMachine(t, stub, "c")
	_, _, _ = crypto.LoadOrGenerate(c.dir())
	c.writeFile("memory/c.md", "from C\n")
	steps = c.converse(c.svc.OnFlow(), password, "2")
	if !strings.Contains(steps[2].Text, "this computer's data") {
		t.Fatalf("keep mine: %+v", steps[2])
	}
	a.use()
	out := a.svc.SyncNow(context.Background())
	if !strings.Contains(out, "key was changed on another computer") {
		t.Fatalf("A after the key change: %s", out)
	}
	if a.readFile("memory/a.md") != "from A\n" {
		t.Fatal("A lost local memory")
	}
	// A enters the password again and takes the new key.
	steps = a.converse(a.svc.OnFlow(), password, "1")
	if !steps[len(steps)-1].Done() || a.readFile("memory/c.md") != "from C\n" {
		t.Fatalf("A rejoin: %+v", steps)
	}
}

// TestForgetAndOff — forget asks for the word, wipes the server; another
// machine notices and switches itself off.
func TestForgetAndOff(t *testing.T) {
	i18n.SetLocale("ru")
	defer i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.writeFile("memory/x.md", "x\n")
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)

	if s := a.converse(a.svc.ForgetFlow(), "нет"); !strings.Contains(s[1].Text, "Не удаляю") || stub.identity == nil {
		t.Fatalf("forget without the word: %+v", s)
	}
	s := a.converse(a.svc.ForgetFlow(), "удалить")
	if !strings.Contains(s[1].Text, "Удалено с сервера, файлов: 1") || stub.identity != nil || len(stub.blobs) != 0 {
		t.Fatalf("forget: %+v", s)
	}
	if a.readFile("memory/x.md") != "x\n" {
		t.Fatal("forget touched local files")
	}
	b.use()
	if out := b.svc.SyncNow(context.Background()); !strings.Contains(out, "удалены с сервера") {
		t.Fatalf("B after the wipe: %s", out)
	}
	if len(stub.blobs) != 0 {
		t.Fatal("B pushed the data back after the wipe")
	}
	st, _ := LoadState(b.svc.Paths)
	if st.Enabled {
		t.Fatal("B still enabled")
	}

	// off keeps the server data and drops the machine from the list.
	a.converse(a.svc.OnFlow(), password, password)
	if _, ok := stub.machines["agent-a"]; !ok {
		t.Fatal("manifest not sent")
	}
	a.use()
	if out := a.svc.Off(context.Background()); !strings.Contains(out, "выключена") {
		t.Fatal(out)
	}
	if _, ok := stub.machines["agent-a"]; ok || stub.identity == nil {
		t.Fatal("off: machine not removed or data deleted")
	}
}

// TestPasswordChange — a new password re-wraps the same key; a new machine
// gets in with it, the old one no longer works.
func TestPasswordChange(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	s := a.converse(a.svc.PasswordFlow(), "new password 2", "new password 2")
	if !strings.Contains(s[2].Text, "Password changed") {
		t.Fatalf("%+v", s)
	}
	if _, err := UnwrapIdentity(stub.identity.wrapped, password); err == nil {
		t.Fatal("old password still opens the key")
	}
	b := newMachine(t, stub, "b")
	s = b.converse(b.svc.OnFlow(), "new password 2")
	if !s[1].Done() || !strings.Contains(s[1].Text, "Key received") {
		t.Fatalf("%+v", s)
	}
}

// TestRunnerPushesAndPauses — the background loop sends a change after the
// quiet delay, pushes the rest on exit, and pauses once when the plan goes.
func TestRunnerPushesAndPauses(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	a.svc.WatchEvery = 20 * time.Millisecond
	a.svc.PushDelay = 50 * time.Millisecond
	a.svc.PullEvery = time.Hour

	var notes []string
	notesCh := make(chan string, 10)
	stop := a.svc.Start(context.Background(), Hooks{Notify: func(s string) { notesCh <- s }})
	a.writeFile("memory/live.md", "written while running\n")
	key, _ := crypto.LoadIdentity(a.dir())
	name := memoryName(key, "live.md")
	deadline := time.Now().Add(5 * time.Second)
	for stub.blob(KindMemory, name) == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if stub.blob(KindMemory, name) == nil {
		t.Fatal("change not pushed by the runner")
	}
	stop()

	// A runner that would wait an hour: only the exit push can send it.
	slow, _ := New(stub.URL)
	slow.WatchEvery, slow.PushDelay, slow.PullEvery = 20*time.Millisecond, time.Hour, time.Hour
	stop = slow.Start(context.Background(), Hooks{})
	time.Sleep(60 * time.Millisecond)
	a.writeFile("memory/last.md", "just before exit\n")
	time.Sleep(60 * time.Millisecond)
	if stub.blob(KindMemory, memoryName(key, "last.md")) != nil {
		t.Fatal("pushed before the delay")
	}
	stop()
	if stub.blob(KindMemory, memoryName(key, "last.md")) == nil {
		t.Fatal("not pushed on exit")
	}

	// The plan is gone: one message, no traffic.
	stub.mu.Lock()
	stub.tariff = "week_start"
	stub.mu.Unlock()
	_ = os.Remove(filepath.Join(a.dir(), "plan.json"))
	svc, _ := New(stub.URL) // a fresh checker, without the answer kept in memory
	svc.WatchEvery, svc.PullEvery = 20*time.Millisecond, time.Hour
	stop = svc.Start(context.Background(), Hooks{Notify: func(s string) { notesCh <- s }})
	time.Sleep(100 * time.Millisecond)
	stop()
	close(notesCh)
	for n := range notesCh {
		notes = append(notes, n)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "paused") || !strings.Contains(notes[0], "«Work»") {
		t.Fatalf("notes: %q", notes)
	}
	st, _ := LoadState(svc.Paths)
	if st.Paused != PausedPlan {
		t.Fatalf("paused = %q", st.Paused)
	}
}

// TestPauseEndsWithRenewal — a plan pause ends with the first exchange that
// works; /sync now after renewing does it at once, and status says so.
func TestPauseEndsWithRenewal(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	a.svc.setPaused(PausedPlan)
	if !strings.Contains(a.svc.Status(), "Paused") {
		t.Fatal("status does not show the pause")
	}
	if out := a.svc.SyncNow(context.Background()); !strings.Contains(out, "Done") {
		t.Fatalf("now: %s", out)
	}
	if st, _ := LoadState(a.svc.Paths); st.Paused != "" {
		t.Fatalf("still paused: %q", st.Paused)
	}
}
