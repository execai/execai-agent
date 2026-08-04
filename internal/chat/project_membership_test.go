package chat

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
)

const testAgentID = "0ba206a4-e546-4920-851d-b3adaa9632ca"

// wsWithTools собирает проект с произвольным набором инструментов.
func wsWithTools(id, name string, tools ...workspaceTool) workspace {
	w := workspace{ID: id, Name: name}
	for _, t := range tools {
		w.Tools = append(w.Tools, struct {
			ID      int    `json:"id"`
			ToolID  string `json:"tool_id"`
			Profile string `json:"profile"`
			Enabled bool   `json:"enabled"`
		}{ID: t.ID, ToolID: t.ToolID, Profile: t.Profile, Enabled: t.Enabled})
	}
	return w
}

// modelWithAPI подменяет базовый адрес на тестовый сервер.
func modelWithAPI(base string) *tuiModel {
	return &tuiModel{
		cfg:   &config.Config{APIBase: base},
		creds: &config.Credentials{Token: "stub-jwt", AgentID: testAgentID, Alias: "YasonTS1"},
	}
}

func TestFindAgentTool(t *testing.T) {
	w := wsWithTools("ws1", "huy",
		workspaceTool{ID: 1, ToolID: "ssh", Profile: "prod", Enabled: true},
		workspaceTool{ID: 2, ToolID: agentToolID, Profile: "другая-машина", Enabled: true},
		workspaceTool{ID: 3, ToolID: agentToolID, Profile: testAgentID, Enabled: false},
	)

	got, ok := findAgentTool(w, testAgentID)
	if !ok {
		t.Fatal("своя запись агента не найдена")
	}
	if got.ID != 3 {
		t.Errorf("найдена запись id=%d, ожидалась 3 (запись другой машины перепутана со своей)", got.ID)
	}
	// Выключенная запись — всё равно найденная. Иначе /project off по второму
	// разу сказал бы «агента нет в проекте» вместо «уже выключен».
	if got.Enabled {
		t.Error("Enabled=true у выключенной записи")
	}

	if _, ok := findAgentTool(w, "неизвестный-агент"); ok {
		t.Error("найдена запись для агента, которого в проекте нет")
	}
}

// Повторный bind не должен плодить дубликаты: POST в integrations-vbai
// не дедуплицирует и на каждый вызов создаёт новую строку.
func TestAddAgentToProject_Idempotent(t *testing.T) {
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	m := modelWithAPI(srv.URL)
	ws := wsWithTools("ws1", "huy",
		workspaceTool{ID: 7, ToolID: agentToolID, Profile: testAgentID, Enabled: true})

	if err := m.addAgentToProject(ws, testAgentID); err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if posts != 0 {
		t.Errorf("сделано %d POST-запросов, ожидалось 0 — агент уже в проекте", posts)
	}
}

func TestAddAgentToProject_Creates(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":42}`))
	}))
	defer srv.Close()

	m := modelWithAPI(srv.URL)
	ws := wsWithTools("ws1", "huy",
		workspaceTool{ID: 1, ToolID: "ssh", Profile: "prod", Enabled: true})

	if err := m.addAgentToProject(ws, testAgentID); err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if want := "/integrations-vbai/workspaces/ws1/tools"; gotPath != want {
		t.Errorf("путь %q, ожидался %q", gotPath, want)
	}
	if gotBody["tool_id"] != agentToolID {
		t.Errorf("tool_id=%v, ожидался %q", gotBody["tool_id"], agentToolID)
	}
	// В profile должен лежать стабильный agent_id, а не alias: alias
	// переименовывается пользователем, и привязка бы порвалась.
	if gotBody["profile"] != testAgentID {
		t.Errorf("profile=%v, ожидался agent_id %q", gotBody["profile"], testAgentID)
	}
	if gotBody["enabled"] != true {
		t.Errorf("enabled=%v, агента добавляем сразу включённым", gotBody["enabled"])
	}
}

func TestSetAgentEnabled_NotInProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("неожиданный запрос %s %s — записи агента в проекте нет", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	m := modelWithAPI(srv.URL)
	ws := wsWithTools("ws1", "huy",
		workspaceTool{ID: 1, ToolID: "ssh", Profile: "prod", Enabled: true})

	err := m.setAgentEnabled(ws, testAgentID, true)
	if err == nil {
		t.Fatal("ожидалась ошибка «агента нет в проекте»")
	}
	if !strings.Contains(err.Error(), "huy") {
		t.Errorf("в тексте ошибки нет имени проекта: %v", err)
	}
}

func TestSetAgentEnabled_TogglesAndSkipsNoOp(t *testing.T) {
	puts := 0
	var gotBody map[string]any
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			puts++
			gotPath = r.URL.Path
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	m := modelWithAPI(srv.URL)
	ws := wsWithTools("ws1", "huy",
		workspaceTool{ID: 9, ToolID: agentToolID, Profile: testAgentID, Enabled: true})

	// Уже включён — запроса быть не должно.
	if err := m.setAgentEnabled(ws, testAgentID, true); err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if puts != 0 {
		t.Errorf("сделано %d PUT, ожидалось 0 — состояние не менялось", puts)
	}

	// Выключаем — запрос по id записи.
	if err := m.setAgentEnabled(ws, testAgentID, false); err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if puts != 1 {
		t.Fatalf("сделано %d PUT, ожидался 1", puts)
	}
	if want := "/integrations-vbai/workspaces/ws1/tools/9"; gotPath != want {
		t.Errorf("путь %q, ожидался %q", gotPath, want)
	}
	if gotBody["enabled"] != false {
		t.Errorf("enabled=%v, ожидалось false", gotBody["enabled"])
	}
}

func TestRemoveAgentFromProject_MissingIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("неожиданный запрос %s %s — удалять нечего", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	m := modelWithAPI(srv.URL)
	ws := wsWithTools("ws1", "huy")
	// Запись могли удалить из веб-интерфейса — для unbind это норма.
	if err := m.removeAgentFromProject(ws, testAgentID); err != nil {
		t.Errorf("отсутствие записи считается ошибкой: %v", err)
	}
}

// uuid агента в сводке инструментов ничего не сообщает — показываем количество.
func TestToolsSummary_CollapsesAgents(t *testing.T) {
	ws := wsWithTools("ws1", "huy",
		workspaceTool{ID: 1, ToolID: "ssh", Profile: "prod", Enabled: true},
		workspaceTool{ID: 2, ToolID: agentToolID, Profile: testAgentID, Enabled: true},
		workspaceTool{ID: 3, ToolID: agentToolID, Profile: "вторая-машина", Enabled: true},
		workspaceTool{ID: 4, ToolID: agentToolID, Profile: "выключенная", Enabled: false},
		// Добавлен из веба, машина не выбрана — не машина.
		workspaceTool{ID: 5, ToolID: agentToolID, Profile: "", Enabled: true},
	)

	got := toolsSummary(ws)
	if strings.Contains(got, testAgentID) {
		t.Errorf("uuid агента попал в сводку: %q", got)
	}
	if !strings.Contains(got, "agent×2") {
		t.Errorf("сводка %q — ожидалось agent×2 (выключенный не считается)", got)
	}
	if !strings.Contains(got, "ssh/prod") {
		t.Errorf("обычный инструмент пропал из сводки: %q", got)
	}
}

// Браузерная сессия — не агент: добавлять в проект нечего.
func TestSelfAgentID_EmptyForNonAgent(t *testing.T) {
	m := &tuiModel{creds: &config.Credentials{Token: "browser-jwt"}}
	if got := m.selfAgentID(); got != "" {
		t.Errorf("selfAgentID()=%q для не-агента, ожидалась пустая строка", got)
	}
	// И без учётных данных вовсе.
	m2 := &tuiModel{}
	if got := m2.selfAgentID(); got != "" {
		t.Errorf("selfAgentID()=%q без креденшелов", got)
	}
}
