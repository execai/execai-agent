// Периметр чтения и сети: кто из инструментов и когда спрашивает разрешение.
//
// Раньше «читающие» инструменты не спрашивали ничего и никогда, а путь не
// ограничивался рабочим каталогом. Из-за этого два безобидных вызова
// складывались в канал утечки: Read любого файла (`~/.ssh/id_rsa`) плюс
// WebFetch на любой адрес — и секрет уходил наружу без единого вопроса, даже
// без всякого Bash. Здесь это закрыто.
//
// Ключевая идея — SCOPED-РАЗРЕШЕНИЯ. Спрашивать про каждый файл невыносимо,
// поэтому ответ «навсегда» запоминается по КАТАЛОГУ (для чтения) и по ДОМЕНУ
// (для сети). Один ответ гасит целый класс будущих вопросов, и автономность
// агента сохраняется: после первых задач вопросы затухают.
//
// Исключение — секреты: там разрешение привязано к конкретному файлу.
// «Разрешил читать ~/.config» не должно означать «отдал креды».
package tools

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/security"
)

// Scoped — инструмент сам решает, чем является его «навсегда».
//
// Без этого интерфейса ключ разрешения строится из полных аргументов, и
// «навсегда» для `Read {"path":"/var/log/a.log"}` не покрывает соседний файл
// в том же каталоге — человек отвечал бы на один и тот же по смыслу вопрос
// бесконечно.
type Scoped interface {
	// PermissionKey возвращает ключ, под которым запоминается разрешение.
	// Пустая строка — использовать обычный ключ по аргументам.
	PermissionKey(args json.RawMessage) string
	// PermissionScope — человеческое описание того, что разрешается
	// («каталог /var/log», «домен github.com»). Показывается в вопросе.
	PermissionScope(args json.RawMessage) string
}

// argPath достаёт путь из аргументов инструмента и приводит к абсолютному.
func argPath(cwd string, args json.RawMessage, field string) string {
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		return cwd
	}
	p, _ := m[field].(string)
	if strings.TrimSpace(p) == "" {
		return cwd
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	return filepath.Clean(p)
}

// readKey — ключ разрешения для чтения.
//
// Секрет — по файлу, остальное — по каталогу.
func readKey(tool, path string) string {
	if security.IsSensitive(path) {
		return tool + "|file=" + path
	}
	return tool + "|dir=" + filepath.Dir(path)
}

func readScope(path string) string {
	if security.IsSensitive(path) {
		return "файл " + path + " (секрет — разрешение только на него)"
	}
	return "каталог " + filepath.Dir(path)
}

// ── Read ──────────────────────────────────────────────────────────────────

func (r *ReadTool) RequiresApproval(args json.RawMessage) bool {
	return security.AskRead(r.cwd, argPath(r.cwd, args, "path"))
}

func (r *ReadTool) PermissionKey(args json.RawMessage) string {
	return readKey("Read", argPath(r.cwd, args, "path"))
}

func (r *ReadTool) PermissionScope(args json.RawMessage) string {
	return "чтение: " + readScope(argPath(r.cwd, args, "path"))
}

// ── Grep / Glob / LS / Tree ───────────────────────────────────────────────
//
// Поиск по каталогу вне проекта — то же чтение: `grep -r 'AKIA' ~` выдаёт
// ключи ничуть не хуже, чем Read.

func (g *GrepTool) RequiresApproval(args json.RawMessage) bool {
	return security.AskRead(g.cwd, argPath(g.cwd, args, "path"))
}
func (g *GrepTool) PermissionKey(args json.RawMessage) string {
	return "Grep|dir=" + argPath(g.cwd, args, "path")
}
func (g *GrepTool) PermissionScope(args json.RawMessage) string {
	return "поиск в каталоге " + argPath(g.cwd, args, "path")
}

func (g *GlobTool) RequiresApproval(args json.RawMessage) bool {
	return security.AskRead(g.cwd, argPath(g.cwd, args, "path"))
}
func (g *GlobTool) PermissionKey(args json.RawMessage) string {
	return "Glob|dir=" + argPath(g.cwd, args, "path")
}
func (g *GlobTool) PermissionScope(args json.RawMessage) string {
	return "обход каталога " + argPath(g.cwd, args, "path")
}

func (l *LSTool) RequiresApproval(args json.RawMessage) bool {
	return security.AskRead(l.cwd, argPath(l.cwd, args, "path"))
}
func (l *LSTool) PermissionKey(args json.RawMessage) string {
	return "LS|dir=" + argPath(l.cwd, args, "path")
}
func (l *LSTool) PermissionScope(args json.RawMessage) string {
	return "список каталога " + argPath(l.cwd, args, "path")
}

func (t *TreeTool) RequiresApproval(args json.RawMessage) bool {
	return security.AskRead(t.cwd, argPath(t.cwd, args, "path"))
}
func (t *TreeTool) PermissionKey(args json.RawMessage) string {
	return "Tree|dir=" + argPath(t.cwd, args, "path")
}
func (t *TreeTool) PermissionScope(args json.RawMessage) string {
	return "дерево каталога " + argPath(t.cwd, args, "path")
}

// ── Сеть ──────────────────────────────────────────────────────────────────

// hostOf возвращает домен из URL; пустая строка, если разобрать не вышло.
func hostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func (*WebFetchTool) RequiresApproval(json.RawMessage) bool { return security.AskNetwork() }

// PermissionKey — по домену, а не по URL: иначе каждая страница того же сайта
// была бы новым вопросом.
func (*WebFetchTool) PermissionKey(args json.RawMessage) string {
	var p struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(args, &p)
	h := hostOf(p.URL)
	if h == "" {
		return "" // не разобрали — пусть работает обычный ключ по аргументам
	}
	return "WebFetch|domain=" + h
}

func (*WebFetchTool) PermissionScope(args json.RawMessage) string {
	var p struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(args, &p)
	if h := hostOf(p.URL); h != "" {
		return "сеть: домен " + h
	}
	return "сеть: " + p.URL
}

// WebSearch отправляет наружу текст запроса — а значит тоже может вынести
// секрет. Но домена у него нет, поэтому «навсегда» тут — на весь инструмент:
// один вопрос за всё время, дальше тишина.
func (*WebSearchTool) RequiresApproval(json.RawMessage) bool { return security.AskNetwork() }
func (*WebSearchTool) PermissionKey(json.RawMessage) string  { return "WebSearch" }
func (*WebSearchTool) PermissionScope(json.RawMessage) string {
	return "веб-поиск (текст запроса уходит поисковику)"
}
