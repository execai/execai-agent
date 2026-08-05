// Канонический ключ «точно этот вызов».
//
// Зачем канонизация. Сырой ключ name+"|"+json(args) рвётся на косметике:
// модель между двумя ОДИНАКОВЫМИ командами меняет поле description
// («…timestamp» → «…timestamp (second call)»), и «Эту команду в сессии»
// перестаёт узнавать повтор — человек отвечает на тот же вопрос дважды.
// Поймано живьём прогоном WA13 05.08: два вызова `touch /tmp/wa13-exact`,
// отличавшихся только description, дали два вопроса.
//
// Канонизируем три вещи:
//   - выбрасываем description — это подпись для человека, на выполнение не
//     влияет (см. схему Bash: «пользователь увидит при подтверждении»);
//   - сортируем ключи и убираем пробелы (json.Marshal map — стабильный
//     порядок), чтобы перестановка полей не выглядела «другой командой»;
//   - при неразбираемом JSON честно откатываемся на сырую строку.
//
// Сравнение в HasExact канонизирует ОБЕ стороны, поэтому старые записи в
// permissions.json (сырые, возможно с description) продолжают совпадать.
package agent

import (
	"encoding/json"
	"strings"
)

// ExactKey строит канонический ключ вызова для сессионных и постоянных
// «точных» разрешений. Формат: name + "|" + канонический JSON аргументов.
func ExactKey(name string, args []byte) string {
	return name + "|" + canonicalArgs(string(args))
}

// canonicalExisting канонизирует уже сохранённый ключ (например, строку из
// permissions.json), чтобы сравнение шло каноника-с-каноникой.
func canonicalExisting(key string) string {
	name, rawArgs, ok := strings.Cut(key, "|")
	if !ok {
		return strings.TrimSpace(key)
	}
	return name + "|" + canonicalArgs(rawArgs)
}

func canonicalArgs(raw string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		// Не JSON-объект — сравниваем как есть, без выдумок.
		return strings.TrimSpace(raw)
	}
	delete(m, "description")
	b, err := json.Marshal(m) // map → ключи отсортированы, пробелов нет
	if err != nil {
		return strings.TrimSpace(raw)
	}
	return string(b)
}
