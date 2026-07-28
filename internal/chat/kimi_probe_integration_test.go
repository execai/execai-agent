package chat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Integration-тест каскада detectKimiPlan: мокает несколько endpoint'ов
// одновременно, проверяет что probe останавливается на первом принявшем ключ.
//
// Настоящий detectKimiPlan обращается к hardcoded URL'ам api.kimi.com и
// api.moonshot.ai — не то что мы хотим для теста. Поэтому проверяем
// поведение через probeKimiOne + вручную составленный attempts список,
// эмулируя тот же кастомный маршрутизатор что и в detectKimiPlan.

// TestProbeCascade_KimiCodingFirst — вариант где kimi.com/coding принял ключ.
// Ожидание: первое попадание останавливает каскад, moonshot не тестится.
func TestProbeCascade_KimiCodingFirst(t *testing.T) {
	var hits int
	kimiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(200) // ключ принят
	}))
	defer kimiSrv.Close()
	moonshotSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(401)
	}))
	defer moonshotSrv.Close()

	// Симуляция detectKimiPlan с custom URL'ами.
	attempts := []probeAttempt{
		{kimiSrv.URL + "/v1/messages", "coding", kimiSrv.URL, true},
		{moonshotSrv.URL + "/v1/messages", "coding", moonshotSrv.URL, true},
	}
	client := &http.Client{}
	var acceptedPlan, acceptedBase string
	for _, a := range attempts {
		code, err := probeKimiOne(client, a, "sk-test")
		if err == nil && (code == 200 || code == 400) {
			acceptedPlan = a.plan
			acceptedBase = a.base
			break
		}
	}
	if acceptedPlan != "coding" {
		t.Errorf("want plan=coding, got %q", acceptedPlan)
	}
	if acceptedBase != kimiSrv.URL {
		t.Errorf("want kimi accepted, got %q", acceptedBase)
	}
	if hits != 1 {
		t.Errorf("only kimi should be hit (cascade stops), got %d hits", hits)
	}
}

// TestProbeCascade_FallbackToMoonshot — kimi.com не отвечает, moonshot принимает.
func TestProbeCascade_FallbackToMoonshot(t *testing.T) {
	kimiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401) // ключ не подходит
	}))
	defer kimiSrv.Close()
	moonshotSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400) // "модель unknown" — принято
	}))
	defer moonshotSrv.Close()

	attempts := []probeAttempt{
		{kimiSrv.URL + "/v1/messages", "coding", kimiSrv.URL, true},
		{moonshotSrv.URL + "/v1/messages", "coding", moonshotSrv.URL, true},
	}
	client := &http.Client{}
	var acceptedBase string
	for _, a := range attempts {
		code, err := probeKimiOne(client, a, "sk-x")
		if err == nil && (code == 200 || code == 400) {
			acceptedBase = a.base
			break
		}
	}
	if acceptedBase != moonshotSrv.URL {
		t.Errorf("want fallback to moonshot, got %q", acceptedBase)
	}
}

// TestProbeCascade_AllReject — ни один endpoint не принял.
func TestProbeCascade_AllReject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer srv.Close()

	attempts := []probeAttempt{
		{srv.URL + "/a", "coding", srv.URL, true},
		{srv.URL + "/b", "api", srv.URL, false},
	}
	client := &http.Client{}
	var acceptedBase string
	var results []string
	for _, a := range attempts {
		code, _ := probeKimiOne(client, a, "sk-x")
		if code == 200 || code == 400 {
			acceptedBase = a.base
			break
		}
		results = append(results, a.url)
	}
	if acceptedBase != "" {
		t.Errorf("no endpoint should accept, got %q", acceptedBase)
	}
	if len(results) != 2 {
		t.Errorf("cascade should try all endpoints on reject, tried %d", len(results))
	}
}

// TestFetchKimiAvailableModels_MultipleContentTypes — сервер может вернуть
// JSON с разным Content-Type; главное что валидный body.
func TestFetchKimiAvailableModels_ContentTypeTolerant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Специально не ставим Content-Type — Go json.Decoder не проверяет заголовок.
		w.Write([]byte(`{"data":[{"id":"k3"}]}`))
	}))
	defer srv.Close()

	models := fetchKimiAvailableModels(srv.URL, "sk")
	if len(models) != 1 || models[0] != "k3" {
		t.Errorf("want [k3], got %v", models)
	}
}

// TestFetchKimiAvailableModels_MalformedJSON — сервер отдал невалидный JSON.
func TestFetchKimiAvailableModels_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{invalid`))
	}))
	defer srv.Close()

	if got := fetchKimiAvailableModels(srv.URL, "sk"); len(got) != 0 {
		t.Errorf("malformed JSON should give empty, got %v", got)
	}
}

// TestFetchKimiAvailableModels_HandlesSpaceEncodedURL — URL с trailing slash + parts.
func TestFetchKimiAvailableModels_URLNormalization(t *testing.T) {
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	// baseURL с /v1 суффиксом — не должно двоиться.
	fetchKimiAvailableModels(srv.URL+"/v1", "sk")
	if !strings.HasSuffix(seenPath, "/v1/models") {
		t.Errorf("with /v1 suffix: want /v1/models, got %s", seenPath)
	}
	// Без /v1 — должно добавиться.
	fetchKimiAvailableModels(srv.URL+"/coding", "sk")
	if !strings.HasSuffix(seenPath, "/v1/models") {
		t.Errorf("without /v1: want /v1/models, got %s", seenPath)
	}
}
