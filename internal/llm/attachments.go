package llm

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// imageQuotedRE — путь в одинарных или двойных кавычках. Внутри может быть
// что угодно (пробелы, кириллица), кроме самой кавычки. Drag&drop в большинстве
// терминалов оборачивает путь в одинарные кавычки.
var imageQuotedRE = regexp.MustCompile(`(?i)'([~/][^']+\.(?:png|jpe?g|gif|webp))'|"([~/][^"]+\.(?:png|jpe?g|gif|webp))"`)

// imageUnquotedRE — путь без кавычек, упирается в пробел/кавычку/таб.
var imageUnquotedRE = regexp.MustCompile(`(?i)(?:^|\s)([~/][^\s'"]+\.(?:png|jpe?g|gif|webp))(?:\s|$|[,.;:!?])`)

// imageExtAnyRE — любое появление .png/.jpg/etc в строке (для fallback-эвристики
// поиска путей с пробелами без кавычек).
var imageExtAnyRE = regexp.MustCompile(`(?i)\.(png|jpe?g|gif|webp)(?:\s|$|[,.;:!?'"]|$)`)

// imageExt — все поддерживаемые расширения (нижний регистр).
var imageExt = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// imageMatch — внутренний тип для накопления match'ей из двух regex.
type imageMatch struct {
	matchStart, matchEnd int    // позиции в тексте для вырезки
	path                 string // распарсенный путь (raw)
}

// ExtractImageAttachments сканирует текст на пути к изображениям, читает файлы
// и возвращает (а) очищенный текст (без путей), (б) []ContentBlock с image_url'ами.
// Если ни одной картинки не найдено или ни один файл не читается — images=nil.
func ExtractImageAttachments(text string) (clean string, images []ContentBlock) {
	var all []imageMatch
	// Сначала ищем quoted-пути — они могут содержать пробелы/кириллицу.
	for _, m := range imageQuotedRE.FindAllStringSubmatchIndex(text, -1) {
		// m: [matchStart matchEnd g1Start g1End g2Start g2End]
		// g1 — одинарные кавычки, g2 — двойные. Один из них -1.
		var raw string
		switch {
		case m[2] != -1:
			raw = text[m[2]:m[3]]
		case m[4] != -1:
			raw = text[m[4]:m[5]]
		default:
			continue
		}
		all = append(all, imageMatch{m[0], m[1], raw})
	}
	// Fallback для unquoted-путей С ПРОБЕЛАМИ (типа "/home/.../Снимок экрана.png"):
	// regex'ы их не находят, но мы можем попробовать «жадно» взять от начала строки
	// или от первого '/' до позиции расширения и проверить os.Stat. Делаем это только
	// если строка целиком начинается с '/' или ' /' — иначе слишком много ложных срабатываний.
	if len(all) == 0 && (strings.HasPrefix(text, "/") || strings.HasPrefix(text, "~/")) {
		extMatches := imageExtAnyRE.FindAllStringIndex(text, -1)
		// Идём с конца — берём самое длинное возможное совпадение.
		for i := len(extMatches) - 1; i >= 0; i-- {
			em := extMatches[i]
			// em[1] — это конец вместе с trailing char (\s|punct), нам нужен конец расширения.
			// Найдём фактический конец .ext (без trailing разделителя).
			end := em[1]
			if end > 0 && (text[end-1] == ' ' || text[end-1] == '\n' || text[end-1] == '\t' ||
				text[end-1] == ',' || text[end-1] == '.' || text[end-1] == ';' ||
				text[end-1] == ':' || text[end-1] == '!' || text[end-1] == '?' ||
				text[end-1] == '\'' || text[end-1] == '"') {
				end--
			}
			candidate := strings.TrimSpace(text[:end])
			candidate = expandHome(candidate)
			if _, err := os.Stat(candidate); err == nil {
				all = append(all, imageMatch{0, end, candidate})
				break
			}
		}
	}

	// Потом unquoted — без захвата quoted-областей.
	for _, m := range imageUnquotedRE.FindAllStringSubmatchIndex(text, -1) {
		// Пропускаем если этот диапазон уже накрыт quoted-match'ем.
		overlap := false
		for _, q := range all {
			if m[2] < q.matchEnd && m[3] > q.matchStart {
				overlap = true
				break
			}
		}
		if overlap {
			continue
		}
		all = append(all, imageMatch{m[0], m[1], text[m[2]:m[3]]})
	}
	if len(all) == 0 {
		return text, nil
	}
	// Сортировка по началу match'а — для корректной вырезки.
	sortByStart(all)

	var keep []string
	last := 0
	seen := map[string]bool{}
	for _, im := range all {
		path := expandHome(im.path)
		if seen[path] {
			continue
		}
		ext := strings.ToLower(filepath.Ext(path))
		mime, ok := imageExt[ext]
		if !ok {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		images = append(images, ContentBlock{
			Type:     "image_url",
			ImageURL: fmt.Sprintf("data:%s;base64,%s", mime, b64),
		})
		seen[path] = true
		keep = append(keep, text[last:im.matchStart])
		last = im.matchEnd
	}
	keep = append(keep, text[last:])
	clean = strings.TrimSpace(strings.Join(keep, " "))
	return clean, images
}

func sortByStart(s []imageMatch) {
	// Простая insertion sort — диапазонов мало.
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j-1].matchStart > s[j].matchStart {
			s[j], s[j-1] = s[j-1], s[j]
			j--
		}
	}
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// BuildUserContent создаёт Content для user-сообщения: либо просто строку
// (если нет картинок), либо []ContentBlock с text + images.
func BuildUserContent(text string) interface{} {
	clean, images := ExtractImageAttachments(text)
	if len(images) == 0 {
		return text
	}
	blocks := make([]ContentBlock, 0, 1+len(images))
	if clean != "" {
		blocks = append(blocks, ContentBlock{Type: "text", Text: clean})
	}
	blocks = append(blocks, images...)
	return blocks
}
