// Импорт памяти из файлов других агентов и экспорт своей.
//
// Зачем это раньше синка: мы выбрали схему, в которой ключ шифрования есть
// только у пользователя, а значит восстановления не существует. Импорт
// превращает потерю ключа из катастрофы в неудобство — память собирается
// заново из того, что и так лежит в репозитории.
//
// Побочная польза: человек, пришедший с Claude Code или Cursor, получает
// работающий контекст в первую минуту, а не начинает с чистого листа.
package agent

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/security"
)

// Agent ids accepted by `execai import --from`.
const (
	AgentClaude   = "claude"
	AgentCursor   = "cursor"
	AgentCodex    = "codex"
	AgentWindsurf = "windsurf"
	AgentGemini   = "gemini"
	AgentCopilot  = "copilot"
	// AgentExecai is execai's own EXECAI.md: picked up only without a filter.
	AgentExecai = "execai"
)

// ImportAgents lists the values --from accepts besides "all".
var ImportAgents = []string{AgentClaude, AgentCursor, AgentCodex, AgentWindsurf, AgentGemini, AgentCopilot}

// ImportSource is one memory/instructions file of another agent.
type ImportSource struct {
	// Path is the absolute path to the file.
	Path string
	// Tool is the product the file belongs to ("Claude Code", "Cursor"…).
	// Product names are not translated: they also go into the imported
	// file's frontmatter and must not depend on the UI language.
	Tool string
	// Kind qualifies the file within the tool (memory, rules, global
	// instructions…), already localized. Empty for the tool's main file.
	Kind string
	// Agent is the stable id used by --from (AgentClaude…).
	Agent string
	// Global marks user-level files that apply to every project
	// (~/.claude/CLAUDE.md, ~/.codex/AGENTS.md…).
	Global bool
	// Rel is the path shown to the user: relative to the project, or ~/…
	// for files outside it.
	Rel string
	// SizeBytes tells a real file from a stub.
	SizeBytes int64
	// Preview is the first lines, so the user sees what is being imported.
	// Empty when the file looks like it holds a secret.
	Preview string
	// Name, when set, is the preferred memory file name (without the
	// "imported_" prefix and ".md"); otherwise it is derived from Rel.
	Name string
	// SecretLine is the 1-based line that looks like a key, token or
	// password. Such a file is never imported: memory is synced. 0 = clean.
	SecretLine int
}

// Label is the human-readable origin: "Cursor (rules)".
func (s ImportSource) Label() string {
	if s.Kind == "" {
		return s.Tool
	}
	return s.Tool + " (" + s.Kind + ")"
}

// memoryLocation describes where another agent keeps its instructions.
type memoryLocation struct {
	rel   string // relative to the project; absolute for user-level files
	agent string
	tool  string
	kind  string // i18n key of the qualifier; "" for the tool's main file
	dir   bool   // a folder of markdown files, read one level deep
	name  string // memory file name for user-level files
}

// knownMemoryFiles — where other agents keep instructions inside a project.
// Only markdown/plain-text instruction files: settings, sessions and logs
// next to them are never read.
var knownMemoryFiles = []memoryLocation{
	{rel: "CLAUDE.md", agent: AgentClaude, tool: "Claude Code"},
	{rel: ".claude/CLAUDE.md", agent: AgentClaude, tool: "Claude Code"},
	{rel: "CLAUDE.local.md", agent: AgentClaude, tool: "Claude Code", kind: "import.kind.personal"},
	{rel: ".claude/memory", agent: AgentClaude, tool: "Claude Code", kind: "import.kind.memory", dir: true},
	{rel: "AGENTS.md", agent: AgentCodex, tool: "AGENTS.md", kind: "import.kind.agentsStd"},
	{rel: ".cursorrules", agent: AgentCursor, tool: "Cursor"},
	{rel: ".cursor/rules", agent: AgentCursor, tool: "Cursor", kind: "import.kind.rules", dir: true},
	{rel: ".github/copilot-instructions.md", agent: AgentCopilot, tool: "GitHub Copilot"},
	{rel: ".github/instructions", agent: AgentCopilot, tool: "GitHub Copilot", kind: "import.kind.pathRules", dir: true},
	{rel: "EXECAI.md", agent: AgentExecai, tool: "execai", kind: "import.kind.projectMemory"},
	{rel: ".windsurfrules", agent: AgentWindsurf, tool: "Windsurf"},
	{rel: ".windsurf/rules", agent: AgentWindsurf, tool: "Windsurf", kind: "import.kind.rules", dir: true},
	{rel: "GEMINI.md", agent: AgentGemini, tool: "Gemini CLI"},
}

// userLevelLocations — instructions other agents apply to every project.
// Each lives in a folder that also holds credentials, sessions and logs;
// only the one named file is read.
func userLevelLocations(home string) []memoryLocation {
	codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if codexHome == "" {
		codexHome = filepath.Join(home, ".codex")
	}
	return []memoryLocation{
		{rel: filepath.Join(home, ".claude", "CLAUDE.md"), agent: AgentClaude, tool: "Claude Code", kind: "import.kind.global", name: "claude_global"},
		{rel: filepath.Join(codexHome, "AGENTS.md"), agent: AgentCodex, tool: "Codex CLI", kind: "import.kind.global", name: "codex_global"},
		{rel: filepath.Join(codexHome, "instructions.md"), agent: AgentCodex, tool: "Codex CLI", kind: "import.kind.globalLegacy", name: "codex_global_instructions"},
		{rel: filepath.Join(home, ".gemini", "GEMINI.md"), agent: AgentGemini, tool: "Gemini CLI", kind: "import.kind.global", name: "gemini_global"},
		{rel: filepath.Join(home, ".codeium", "windsurf", "memories", "global_rules.md"), agent: AgentWindsurf, tool: "Windsurf", kind: "import.kind.global", name: "windsurf_global"},
	}
}

// maxPreviewLines — сколько строк показывать в предпросмотре.
const maxPreviewLines = 3

// maxImportFileSize — файлы больше этого не импортируем: обычно это
// не память, а случайно попавший дамп или логи.
const maxImportFileSize = 256 * 1024

// discovery collects sources without duplicates.
type discovery struct {
	seen map[string]bool
	out  []ImportSource
}

func newDiscovery() *discovery { return &discovery{seen: map[string]bool{}} }

func (d *discovery) add(path string, loc memoryLocation, rel string, global bool, name string) {
	abs, err := filepath.Abs(path)
	if err != nil || d.seen[abs] {
		return
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() || info.Size() == 0 || info.Size() > maxImportFileSize {
		return
	}
	// A symlink named CLAUDE.md that points at a key file is still a key file.
	if real, err := filepath.EvalSymlinks(abs); err == nil && real != abs && security.IsSensitive(real) {
		return
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return
	}
	d.seen[abs] = true
	src := ImportSource{
		Path: abs, Tool: loc.tool, Agent: loc.agent, Global: global,
		Rel: rel, SizeBytes: info.Size(), Name: name,
	}
	if loc.kind != "" {
		src.Kind = i18n.T(loc.kind)
	}
	text := string(data)
	if src.SecretLine = secretLine(text); src.SecretLine == 0 {
		src.Preview = previewText(text)
	}
	d.out = append(d.out, src)
}

// expand turns a location into files: itself, or the markdown files of a
// folder one level deep — recursing into .claude/ or .cursor/ would drag in
// service files and sessions.
func expand(path string, dir bool) []string {
	if !dir {
		return []string{path}
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".mdc") {
			out = append(out, filepath.Join(path, name))
		}
	}
	return out
}

// DiscoverMemorySources finds other agents' instruction files inside a
// project directory. It reads the files only for the preview and the secret
// check; nothing is written.
func DiscoverMemorySources(dir string) []ImportSource {
	d := newDiscovery()
	project := projectSlug(dir)
	for _, loc := range knownMemoryFiles {
		for _, p := range expand(filepath.Join(dir, loc.rel), loc.dir) {
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				rel = p
			}
			d.add(p, loc, rel, false, project+"_"+rel)
		}
	}
	sortSources(d.out)
	return d.out
}

// DiscoverGlobalMemorySources finds what other agents keep outside the
// project: user-level instructions (~/.claude/CLAUDE.md, Codex CLI's
// AGENTS.md…) and Claude Code's memory for this project under ~/.claude.
func DiscoverGlobalMemorySources(cwd string) []ImportSource {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	d := newDiscovery()
	for _, loc := range userLevelLocations(home) {
		d.add(loc.rel, loc, displayPath(home, loc.rel), true, loc.name)
	}
	// Claude Code's per-project memory belongs to this project even though it
	// lives in the home folder. Its own MEMORY.md is an index of links, not a
	// fact: every fact file is imported on its own.
	project := projectSlug(cwd)
	loc := memoryLocation{agent: AgentClaude, tool: "Claude Code", kind: "import.kind.memory"}
	for _, mem := range claudeProjectMemoryDirs(cwd) {
		for _, p := range expand(mem, true) {
			if strings.EqualFold(filepath.Base(p), "MEMORY.md") {
				continue
			}
			d.add(p, loc, displayPath(home, p), false, "claude_"+project+"_"+filepath.Base(p))
		}
	}
	sortSources(d.out)
	return d.out
}

// DiscoverAllMemorySources is the project's files plus everything outside it.
// When the project is the home folder, user-level files keep their user-level
// meaning.
func DiscoverAllMemorySources(cwd string) []ImportSource {
	outside := DiscoverGlobalMemorySources(cwd)
	seen := map[string]bool{}
	out := make([]ImportSource, 0, len(outside))
	for _, s := range outside {
		seen[s.Path] = true
		out = append(out, s)
	}
	for _, s := range DiscoverMemorySources(cwd) {
		if !seen[s.Path] {
			out = append(out, s)
		}
	}
	sortSources(out)
	return out
}

// sortSources puts project files first, user-level ones last.
func sortSources(srcs []ImportSource) {
	sort.SliceStable(srcs, func(i, j int) bool {
		if srcs[i].Global != srcs[j].Global {
			return !srcs[i].Global
		}
		return srcs[i].Rel < srcs[j].Rel
	})
}

// claudeProjectMemoryDirs — candidate folders of Claude Code's memory for a
// project. Older builds replaced only path separators, newer ones every
// non-alphanumeric character; both spellings are tried.
func claudeProjectMemoryDirs(cwd string) []string {
	first := ClaudeProjectMemoryDir(cwd)
	if first == "" {
		return nil
	}
	home, _ := os.UserHomeDir()
	alnum := nonAlnum.ReplaceAllString(filepath.Clean(cwd), "-")
	second := filepath.Join(home, ".claude", "projects", alnum, "memory")
	if second == first {
		return []string{first}
	}
	return []string{first, second}
}

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

// projectSlug — the project folder's name, used to tell apart the same
// CLAUDE.md imported from different projects.
func projectSlug(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	base := filepath.Base(abs)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "project"
	}
	return base
}

// displayPath shows a path under the home folder as ~/….
func displayPath(home, p string) string {
	if r, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(r, "..") {
		return "~/" + filepath.ToSlash(r)
	}
	return p
}

// previewOf возвращает первые содержательные строки файла.
func previewOf(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return previewText(string(data))
}

func previewText(text string) string {
	// Frontmatter is metadata (Cursor's globs, Claude's name/type), not
	// what the file says.
	if fm, ok := frontmatterOf(text); ok {
		if i := strings.Index(text, fm); i >= 0 {
			text = strings.TrimPrefix(strings.TrimLeft(text[i+len(fm):], "\r\n"), frontmatterFence)
		}
	}
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		// Пропускаем пустые и разделители frontmatter — они ничего не говорят.
		if l == "" || l == "---" {
			continue
		}
		lines = append(lines, l)
		if len(lines) >= maxPreviewLines {
			break
		}
	}
	return strings.Join(lines, " · ")
}

// secretPatterns — shapes of well-known keys and tokens.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{30,}`),
	regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}`),
	regexp.MustCompile(`\by0_[A-Za-z0-9_-]{30,}`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),
}

// assignedSecret — "password = …", "api_key: …" with a value that looks real.
var assignedSecret = regexp.MustCompile(`(?i)\b(?:api[_-]?key|secret|token|passw(?:or)?d|pwd)["']?\s*[:=]\s*["']?([A-Za-z0-9_\-/+=.]{12,})`)

// secretLine returns the 1-based line that looks like a key, token or
// password, or 0. Placeholders ("sk-xxx", "<your token>") do not count.
func secretLine(text string) int {
	for i, line := range strings.Split(text, "\n") {
		for _, re := range secretPatterns {
			if re.MatchString(line) {
				return i + 1
			}
		}
		if m := assignedSecret.FindStringSubmatch(line); m != nil && hasLetterAndDigit(m[1]) {
			return i + 1
		}
	}
	return 0
}

func hasLetterAndDigit(s string) bool {
	var letter, digit bool
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			letter = true
		}
	}
	return letter && digit
}

// ParseImportFrom turns --from into agent ids. nil means no filter.
func ParseImportFrom(v string) ([]string, error) {
	var out []string
	for _, p := range strings.Split(strings.ToLower(v), ",") {
		p = strings.TrimSpace(p)
		switch {
		case p == "":
			continue
		case p == "all":
			return nil, nil
		case !slices.Contains(ImportAgents, p):
			return nil, errors.New(i18n.Tf("import.badFrom", p, strings.Join(ImportAgents, ", ")))
		}
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out, nil
}

// FilterSourcesByAgent keeps the sources of the given agents; nil keeps all.
func FilterSourcesByAgent(srcs []ImportSource, agents []string) []ImportSource {
	if agents == nil {
		return srcs
	}
	var out []ImportSource
	for _, s := range srcs {
		if slices.Contains(agents, s.Agent) {
			out = append(out, s)
		}
	}
	return out
}

// ImportResult — что получилось после импорта.
type ImportResult struct {
	Imported []string // имена созданных файлов памяти
	Skipped  []string // что пропущено и почему
}

// ImportSourceFromPath делает источник импорта из абсолютного пути — для
// находок вне каталога проекта (глобальная память Claude Code). Rel считаем
// от домашнего каталога: имя импортированного файла должно намекать, откуда
// оно взялось, а не выглядеть как файл из проекта.
func ImportSourceFromPath(path, tool string) ImportSource {
	rel := path
	if home, err := os.UserHomeDir(); err == nil {
		rel = displayPath(home, path)
	}
	src := ImportSource{Path: path, Tool: tool, Rel: rel}
	if tool == "Claude Code" {
		src.Agent = AgentClaude
	}
	if st, err := os.Stat(path); err == nil {
		src.SizeBytes = st.Size()
	}
	if data, err := os.ReadFile(path); err == nil {
		text := string(data)
		if src.SecretLine = secretLine(text); src.SecretLine == 0 {
			src.Preview = previewText(text)
		}
	}
	return src
}

// PlanItem is what will happen to one source: Target is the memory file it
// becomes, or Skip says why it stays where it is.
type PlanItem struct {
	Source ImportSource
	Target string
	Skip   string
}

// PlanImport decides, without writing anything, what an import would do.
// --dry-run shows exactly this, and ImportSources follows it.
func PlanImport(srcs []ImportSource) ([]PlanItem, error) {
	dir, err := MemoryDirPath()
	if err != nil {
		return nil, err
	}
	origins, existing := scanImported(dir)
	taken := map[string]string{} // names claimed in this batch → source path
	plan := make([]PlanItem, 0, len(srcs))
	for _, s := range srcs {
		item := PlanItem{Source: s}
		switch {
		case s.SecretLine > 0:
			item.Skip = i18n.Tf("import.skip.secret", s.SecretLine)
		case origins[s.Path] != "":
			item.Skip = i18n.Tf("import.skip.already", origins[s.Path])
		default:
			name := importedFileName(s)
			owner, exists := existing[name]
			if exists && owner == "" {
				// A file of ours with no recorded origin: the user may have
				// edited it — never touch it.
				item.Skip = i18n.Tf("import.skip.exists", name)
				break
			}
			if other, ok := taken[name]; exists || (ok && other != s.Path) {
				name = suffixed(name, s.Path)
			}
			if _, dup := existing[name]; dup || taken[name] != "" {
				item.Skip = i18n.Tf("import.skip.exists", name)
				break
			}
			item.Target = name
			taken[name] = s.Path
		}
		plan = append(plan, item)
	}
	return plan, nil
}

// suffixed disambiguates a name taken by another origin.
func suffixed(name, origin string) string {
	sum := sha1.Sum([]byte(origin))
	return strings.TrimSuffix(name, ".md") + "_" + hex.EncodeToString(sum[:3]) + ".md"
}

// scanImported reads the memory folder: which origins were already imported
// (origin → file) and which file names exist (file → origin, "" if unknown).
func scanImported(dir string) (map[string]string, map[string]string) {
	origins := map[string]string{}
	existing := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return origins, existing
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		existing[e.Name()] = ""
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		fm, ok := frontmatterOf(string(data))
		if !ok {
			continue
		}
		for _, line := range strings.Split(fm, "\n") {
			v, ok := strings.CutPrefix(strings.TrimSpace(line), "imported_from:")
			if !ok {
				continue
			}
			v = strings.TrimSpace(v)
			if u, err := strconv.Unquote(v); err == nil {
				v = u
			}
			if v != "" {
				existing[e.Name()] = v
				origins[v] = e.Name()
			}
			break
		}
	}
	return origins, existing
}

// MemoryDirPath is UserMemoryDir without creating it: for plans and dry runs.
func MemoryDirPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "execai", "memory"), nil
}

// ImportSources переносит выбранные файлы в память агента.
//
// Каждый источник становится отдельным файлом памяти с frontmatter, в котором
// записано происхождение: через полгода будет непонятно, откуда взялся текст,
// если этого не сохранить. Существующие файлы не перезаписываются.
func ImportSources(srcs []ImportSource) (ImportResult, error) {
	var res ImportResult
	plan, err := PlanImport(srcs)
	if err != nil {
		return res, err
	}
	dir, err := UserMemoryDir()
	if err != nil {
		return res, err
	}

	var done []importedEntry
	for _, it := range plan {
		s := it.Source
		if it.Skip != "" {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s (%s)", s.Rel, it.Skip))
			continue
		}
		body, err := os.ReadFile(s.Path)
		if err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s (%s)", s.Rel, i18n.Tf("import.skip.read", err)))
			continue
		}
		// The file may have changed since discovery: check what is copied.
		if n := secretLine(string(body)); n > 0 {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s (%s)", s.Rel, i18n.Tf("import.skip.secret", n)))
			continue
		}
		f, err := os.OpenFile(filepath.Join(dir, it.Target), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, err = f.WriteString(importedContent(it.Target, s, body))
			if cerr := f.Close(); err == nil {
				err = cerr
			}
		}
		if err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s (%s)", s.Rel, i18n.Tf("import.skip.write", err)))
			continue
		}
		res.Imported = append(res.Imported, it.Target)
		done = append(done, importedEntry{name: it.Target, src: s, hook: hookOf(string(body))})
	}

	if len(done) > 0 {
		if err := appendToIndex(dir, done); err != nil {
			return res, err
		}
	}
	return res, nil
}

// importedContent wraps a source into a memory file that records its origin.
func importedContent(name string, s ImportSource, body []byte) string {
	kind := "project"
	if s.Global {
		kind = "user"
	}
	return fmt.Sprintf(`---
name: %s
description: %s
metadata:
  type: %s
  imported_from: %s
  imported_tool: %s
---

%s`, strings.TrimSuffix(name, ".md"),
		strconv.Quote(i18n.Tf("import.fm.description", s.Label(), s.Rel)),
		kind, s.Path, s.Tool, string(body))
}

// hookOf — one line about what a file says, for the memory index.
func hookOf(text string) string {
	h := descriptionOf(text)
	if h == "" {
		h = previewText(text)
	}
	if r := []rune(h); len(r) > 100 {
		h = string(r[:100]) + "…"
	}
	return h
}

// importedFileName делает имя файла памяти из пути источника.
func importedFileName(s ImportSource) string {
	base := s.Name
	if base == "" {
		base = s.Rel
	}
	base = strings.ToLower(base)
	base = strings.TrimSuffix(base, ".md")
	base = strings.TrimSuffix(base, ".mdc")
	repl := strings.NewReplacer("/", "_", "\\", "_", ".", "_", " ", "_", "~", "_", ":", "_")
	base = strings.Trim(manyUnderscores.ReplaceAllString(repl.Replace(base), "_"), "_")
	if base == "" {
		base = "memory"
	}
	return "imported_" + base + ".md"
}

var manyUnderscores = regexp.MustCompile(`_{2,}`)

type importedEntry struct {
	name string
	src  ImportSource
	hook string
}

// appendToIndex дописывает строки в MEMORY.md. Индекс — единственное, что
// всегда попадает в системный промт, поэтому запись без строки в нём для
// агента невидима. The line names the origin path: memory is shared by all
// projects, and the agent must see which project a file is about.
func appendToIndex(dir string, entries []importedEntry) error {
	path := filepath.Join(dir, "MEMORY.md")

	var b strings.Builder
	existing, err := os.ReadFile(path)
	if err == nil {
		b.Write(existing)
		if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
			b.WriteString("\n")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("%s: %w", path, err)
	}

	for _, e := range entries {
		title := strings.TrimSuffix(strings.TrimPrefix(e.name, "imported_"), ".md")
		hook := e.hook
		if hook == "" {
			hook = e.src.Label()
		}
		b.WriteString(i18n.Tf("import.indexLine", title, e.name, hook, e.src.Label(), e.src.Path) + "\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// DescribeImportPlan renders a plan for the terminal: what goes where and
// what is skipped with the reason.
func DescribeImportPlan(plan []PlanItem) string {
	var b strings.Builder
	for _, it := range plan {
		s := it.Source
		if it.Skip != "" {
			fmt.Fprintf(&b, "  ~ %s — %s: %s\n", s.Rel, s.Label(), it.Skip)
			continue
		}
		fmt.Fprintf(&b, "  + %s — %s → %s\n", s.Rel, s.Label(), it.Target)
		if s.Preview != "" {
			fmt.Fprintf(&b, "      %s\n", s.Preview)
		}
	}
	return b.String()
}

// Importable returns the sources a plan will actually copy.
func Importable(plan []PlanItem) []ImportSource {
	var out []ImportSource
	for _, it := range plan {
		if it.Skip == "" {
			out = append(out, it.Source)
		}
	}
	return out
}

// ImportTools lists the distinct tools of a plan's importable sources, in
// order, for a one-line summary ("Claude Code, Cursor").
func ImportTools(plan []PlanItem) []string {
	var out []string
	for _, it := range plan {
		if it.Skip == "" && !slices.Contains(out, it.Source.Tool) {
			out = append(out, it.Source.Tool)
		}
	}
	return out
}

// ExportMemory складывает всю память в каталог обычными markdown-файлами.
// Нужно, чтобы память не была заложником нашего формата: её можно унести,
// положить в репозиторий или просто прочитать глазами.
func ExportMemory(targetDir string) ([]string, error) {
	src, err := UserMemoryDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil, fmt.Errorf("чтение памяти: %w", err)
	}
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return nil, fmt.Errorf("создание каталога выгрузки: %w", err)
	}

	var written []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(targetDir, e.Name()), data, 0o600); err != nil {
			return written, fmt.Errorf("запись %s: %w", e.Name(), err)
		}
		written = append(written, e.Name())
	}
	sort.Strings(written)
	return written, nil
}

// === Область записи: что синкается, а что остаётся на машине ===

// Запись памяти может быть помечена как локальная — тогда она НЕ уезжает на
// сервер ни в каком виде, даже зашифрованной. Это нужно для того, что человек
// не хочет отдавать наружу в принципе: пути, куски конфигов, заметки про
// инфраструктуру.
//
// Помечается полем во frontmatter:
//
//	---
//	name: local_notes
//	sync: false        # или local: true — понимаем оба написания
//	---
//
// Значение по умолчанию — синкать: иначе люди будут удивляться, почему память
// не появилась на второй машине. Отказ должен быть явным.
const (
	syncFalseMarker  = "sync: false"
	localTrueMarker  = "local: true"
	frontmatterFence = "---"
)

// IsLocalOnly сообщает, помечена ли запись как несинкаемая.
// Разбираем только frontmatter: слова «sync: false» в теле записи — это текст,
// а не директива, и реагировать на них нельзя.
func IsLocalOnly(content string) bool {
	fm, ok := frontmatterOf(content)
	if !ok {
		return false
	}
	for _, line := range strings.Split(fm, "\n") {
		l := strings.TrimSpace(line)
		if l == syncFalseMarker || l == localTrueMarker {
			return true
		}
	}
	return false
}

// frontmatterOf возвращает содержимое блока между первыми двумя «---».
func frontmatterOf(content string) (string, bool) {
	trimmed := strings.TrimLeft(content, "\r\n \t")
	if !strings.HasPrefix(trimmed, frontmatterFence) {
		return "", false
	}
	rest := trimmed[len(frontmatterFence):]
	end := strings.Index(rest, "\n"+frontmatterFence)
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// SyncableMemoryFiles — записи, которые можно синкать: всё из каталога памяти,
// кроме помеченных локальными. Индекс MEMORY.md синкается всегда: без него
// остальные записи для агента невидимы.
func SyncableMemoryFiles() ([]string, []string, error) {
	dir, err := UserMemoryDir()
	if err != nil {
		return nil, nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("чтение памяти: %w", err)
	}
	var syncable, local []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if IsLocalOnly(string(data)) {
			local = append(local, e.Name())
			continue
		}
		syncable = append(syncable, e.Name())
	}
	sort.Strings(syncable)
	sort.Strings(local)
	return syncable, local, nil
}
