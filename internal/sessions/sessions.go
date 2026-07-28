// Persistent storage сессий чата. Каждая сессия — отдельный JSON-файл в
// os.UserConfigDir()/execai/sessions/<id>.json. После каждого обмена
// (user → assistant + tool results) сессия сохраняется. При старте execai
// автоматически продолжает последнюю активную сессию; для нового чата —
// /new, для переключения — /sessions + /resume <id|номер>.
package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/velesbsdllc/agent-vbai/internal/llm"
)

type Session struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	StartedAt time.Time       `json:"started_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Model     string          `json:"model"`
	Provider  string          `json:"provider"`
	CWD       string          `json:"cwd"`
	Messages  []llm.AIMessage `json:"messages"`
}

func dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(base, "execai", "sessions")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	return d, nil
}

// New — создать новую сессию (без файла).
func New(model, provider, cwd string) *Session {
	now := time.Now().UTC()
	return &Session{
		ID:        uuid.NewString(),
		StartedAt: now,
		UpdatedAt: now,
		Model:     model,
		Provider:  provider,
		CWD:       cwd,
	}
}

// Save сохраняет сессию атомарно (через .tmp + rename).
func (s *Session) Save() error {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.StartedAt.IsZero() {
		s.StartedAt = time.Now().UTC()
	}
	s.UpdatedAt = time.Now().UTC()
	if s.Title == "" {
		s.Title = s.deriveTitle()
	}
	d, err := dir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	full := filepath.Join(d, s.ID+".json")
	tmp := full + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, full)
}

// Load — прочитать сессию по id.
func Load(id string) (*Session, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(d, id+".json"))
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// List возвращает все сессии, отсортированные по UpdatedAt от свежих к старым.
func List() ([]*Session, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(d)
	if err != nil {
		return nil, err
	}
	out := make([]*Session, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(d, name))
		if err != nil {
			continue
		}
		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		out = append(out, &s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// MostRecent — последняя обновлённая сессия (для авто-resume), либо nil.
func MostRecent() *Session {
	list, err := List()
	if err != nil || len(list) == 0 {
		return nil
	}
	return list[0]
}

// Delete удаляет сессию (используется командой /sessions delete <id>).
func Delete(id string) error {
	d, err := dir()
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(d, id+".json"))
}

func (s *Session) deriveTitle() string {
	for _, m := range s.Messages {
		if m.Role == "user" {
			t := strings.TrimSpace(llm.ContentText(m.Content))
			if t == "" {
				continue
			}
			if len(t) > 60 {
				t = t[:60] + "…"
			}
			t = strings.ReplaceAll(t, "\n", " ")
			return t
		}
	}
	return "(новая беседа)"
}
