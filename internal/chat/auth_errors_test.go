package chat

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
)

// A refused sign-in is recognised in any interface language, also when the
// error is wrapped on its way up; other errors are not taken for it.
func TestUnauthorizedErr_ByTypeInAnyLanguage(t *testing.T) {
	defer i18n.SetLocale("en")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("no"))
	}))
	defer srv.Close()
	for _, loc := range []string{"en", "ru", "de", "es", "zh"} {
		i18n.SetLocale(loc)
		_, err := llm.FetchModels(t.Context(), srv.URL, "tok")
		if !unauthorizedErr(fmt.Errorf("boot: %w", err)) {
			t.Errorf("%s: %v not recognised", loc, err)
		}
	}
	if unauthorizedErr(errors.New("connection refused")) || unauthorizedErr(nil) {
		t.Error("another error taken for a refused sign-in")
	}
}
