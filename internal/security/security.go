// Package security — уровни доверия агенту и правила, из которых следует,
// что спрашивать у человека, а что делать молча.
//
// Зачем уровни. Агент ценен автономностью: если он спрашивает про каждый `ls`,
// им перестают пользоваться. Но у автономности есть цена — всё, что агент
// делает без вопроса, может сделать и текст, попавший к нему в контекст с
// чужого сайта или из чужого README. Один компромисс на всех не подходит:
// у человека, который правит свой проект, и у демона, который в три ночи
// выполняет задачу из веба, разная цена ошибки.
//
// Поэтому уровень — настройка, а не наша догадка. Мы отвечаем за то, чтобы:
//   - на любом уровне НЕ БЫЛО обходов (дыра ≠ уровень доверия);
//   - вопросы ЗАТУХАЛИ: ответ «всегда» запоминается по каталогу или домену,
//     а не по конкретному файлу, иначе человек утонет в подтверждениях.
//
// Чего уровни НЕ делают: не защищают от prompt injection «уговорами». Текст в
// системном промте модель обходит. Защищает только одно — эффект требует
// согласия человека; уровень задаёт, какие эффекты считаются опасными.
package security

import (
	"os"
	"path/filepath"
	"strings"
)

// Level — профиль доверия.
type Level int

const (
	// Light — «не мешай»: закрыты только дыры, периметры открыты.
	// Чтение и сеть без вопросов. Для своей машины и своего кода.
	Light Level = iota
	// Deep — по умолчанию: работа внутри проекта молчалива, выход за его
	// границы и сеть спрашиваются один раз на каталог/домен.
	Deep
	// Paranoid — спрашивается всё, что имеет эффект или выносит данные
	// наружу; «навсегда» для чувствительных путей не предлагается.
	Paranoid
)

// Parse разбирает имя уровня. Неизвестное значение — Deep: незнакомая
// строка в конфиге не должна молча ослаблять защиту.
func Parse(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "light", "лёгкий", "легкий", "relaxed":
		return Light
	case "paranoid", "strict", "строгий", "параноидальный":
		return Paranoid
	default:
		return Deep
	}
}

func (l Level) String() string {
	switch l {
	case Light:
		return "light"
	case Paranoid:
		return "paranoid"
	default:
		return "deep"
	}
}

// Title — человеческое имя для интерфейсов.
func (l Level) Title() string {
	switch l {
	case Light:
		return "лёгкий — не мешать"
	case Paranoid:
		return "строгий — спрашивать всё"
	default:
		return "обычный — проект молча, наружу с вопросом"
	}
}

// Levels — набор для пикеров (TUI, плагин, веб).
func Levels() []Level { return []Level{Light, Deep, Paranoid} }

// current — уровень процесса. Ставится один раз на старте (как язык или
// каталог): протаскивать его аргументом через каждый инструмент значило бы
// менять сигнатуру Tool ради настройки, которая за жизнь процесса не меняется.
var current = Deep

// Set устанавливает уровень процесса.
func Set(l Level) { current = l }

// Current возвращает текущий уровень.
func Current() Level { return current }

// sensitivePatterns — то, что спрашивается ВСЕГДА (кроме Light), даже внутри
// рабочего каталога: приватные ключи, токены, куки, .env. Утечка любого из
// них — не «неудобство», а компрометация, и «я же разрешил читать проект»
// не должно её покрывать.
var sensitivePatterns = []string{
	".ssh/", ".gnupg/", ".aws/", ".kube/", ".docker/config.json",
	".config/execai/credentials.json", ".config/execai/subscriptions.json",
	".netrc", ".npmrc", ".pypirc", ".git-credentials",
	"id_rsa", "id_ed25519", "id_ecdsa", ".pem", ".p12", ".pfx",
	"cookies.sqlite", "login data", "keychain",
	".env",
}

// IsSensitive — путь похож на секрет.
//
// Сравниваем по нормализованному пути в нижнем регистре: `.ENV`, `~/.SSH/` и
// `/home/u/proj/../.ssh/id_rsa` обязаны попадаться так же, как канонический
// вид, иначе проверка обходится написанием.
func IsSensitive(path string) bool {
	p := strings.ToLower(filepath.Clean(path))
	slashed := filepath.ToSlash(p)
	base := filepath.Base(slashed)
	for _, s := range sensitivePatterns {
		if strings.Contains(slashed, s) {
			return true
		}
		// `.env` как расширение (`prod.env`) и как имя файла.
		if strings.HasPrefix(s, ".") && !strings.Contains(s, "/") &&
			(base == s || strings.HasSuffix(base, s)) {
			return true
		}
	}
	return false
}

// Inside — путь лежит внутри корня (после разрешения символических ссылок и
// `..`). Сравниваем поэлементно, а не по префиксу строки: иначе `/home/u/proj2`
// сошёл бы за «внутри» `/home/u/proj`.
func Inside(root, path string) bool {
	r, err1 := resolve(root)
	p, err2 := resolve(path)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(r, p)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

// resolve приводит путь к абсолютному с раскрытыми симлинками. Симлинк —
// самый дешёвый обход границы каталога: `ln -s ~/.ssh ./s` и `Read s/id_rsa`
// формально «внутри проекта».
func resolve(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real, nil
	}
	// Файла может ещё не быть (Write) — тогда проверяем существующего родителя.
	dir := filepath.Dir(abs)
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		return filepath.Join(real, filepath.Base(abs)), nil
	}
	return abs, nil
}

// AskRead — нужно ли подтверждение на чтение пути.
func AskRead(cwd, path string) bool {
	switch current {
	case Light:
		return false
	case Paranoid:
		return !Inside(cwd, path) || IsSensitive(path)
	default: // Deep
		return !Inside(cwd, path) || IsSensitive(path)
	}
}

// AskNetwork — нужно ли подтверждение на сетевой запрос.
func AskNetwork() bool { return current != Light }

// AskReadOnlyShell — спрашивать ли даже про безопасные команды (`ls`).
// На строгом уровне у Bash нет «бесплатных» команд вовсе.
func AskReadOnlyShell() bool { return current == Paranoid }

// AllowPersist — можно ли предлагать «НАВСЕГДА» для этой цели.
// Для секретов на строгом уровне — нельзя: одно случайное нажатие открывает
// ключи навсегда, и человек об этом не вспомнит.
func AllowPersist(target string) bool {
	if current == Paranoid && IsSensitive(target) {
		return false
	}
	return true
}

// Home — домашний каталог (для подписей). Пустая строка, если не определён.
func Home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}
