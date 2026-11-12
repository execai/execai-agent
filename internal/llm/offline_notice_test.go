package llm

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages"
)

var cyrillicRe = regexp.MustCompile(`[А-Яа-яЁё]`)

// The notice about working without the model catalog is shown to the
// person (chat hint, panel notice): in their language, with the reason.
func TestOfflineNotice_InTheInterfaceLanguage(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("en")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down"))
	}))
	defer srv.Close()
	res := FetchModelsCached(srv.URL, "tok") // no cache yet: the built-in fallback
	if n := res.OfflineNotice(); !res.Fallback || n == "" || cyrillicRe.MatchString(n) {
		t.Errorf("fallback notice %q", n)
	}
	cached := FetchModelsResult{FromCache: true, CacheAge: time.Hour, Err: errors.New("dial tcp: refused")}
	if n := cached.OfflineNotice(); cyrillicRe.MatchString(n) {
		t.Errorf("cache notice %q", n)
	}
	none := FetchModelsResult{Fallback: true}
	none.Err = FetchModelsCached("http://127.0.0.1:1", "tok").Err
	if n := none.OfflineNotice(); cyrillicRe.MatchString(n) {
		t.Errorf("no network notice %q", n)
	}

	i18n.SetLocale("ru")
	if n := res.OfflineNotice(); !cyrillicRe.MatchString(n) {
		t.Errorf("ru notice %q", n)
	}
}
