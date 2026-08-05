package agent

import "testing"

// «Эту команду в сессии» обязана узнавать повтор той же команды, даже когда
// модель сменила подпись или переставила поля. Поймано живьём (WA13 05.08):
// два одинаковых `touch`, отличавшихся только description, дали два вопроса.
func TestExactKey_CosmeticsDontChangeIt(t *testing.T) {
	base := ExactKey("Bash", []byte(`{"command":"touch /tmp/x","description":"Create marker"}`))

	same := []string{
		`{"command":"touch /tmp/x","description":"Create marker (second call)"}`, // другая подпись
		`{"command":"touch /tmp/x"}`,                                             // без подписи вовсе
		`{"description":"whatever","command":"touch /tmp/x"}`,                    // другой порядок полей
		`{ "command" : "touch /tmp/x" }`,                                         // пробелы
	}
	for _, s := range same {
		if got := ExactKey("Bash", []byte(s)); got != base {
			t.Errorf("ключ для %s = %q — должен совпадать с %q", s, got, base)
		}
	}

	// А вот сама команда — значащая: другая команда = другой ключ.
	if ExactKey("Bash", []byte(`{"command":"rm /tmp/x"}`)) == base {
		t.Error("другая команда дала тот же ключ — «эту команду» разрешила бы лишнее")
	}
	// И таймаут значащий: он меняет поведение выполнения.
	if ExactKey("Bash", []byte(`{"command":"touch /tmp/x","timeout":600}`)) == base {
		t.Error("timeout проигнорирован — а он меняет выполнение")
	}
}

// Не-JSON не должен ни падать, ни склеиваться с чужими вызовами.
func TestExactKey_NonJSONFallsBackRaw(t *testing.T) {
	a := ExactKey("Bash", []byte(`не json`))
	b := ExactKey("Bash", []byte(`тоже не json`))
	if a == b {
		t.Error("разные сырые аргументы склеились в один ключ")
	}
}

// Старые записи в permissions.json — сырые, возможно с description и своим
// порядком полей. Канонизация не должна их обесценить.
func TestHasExact_LegacyEntriesStillMatch(t *testing.T) {
	p := &Permissions{Exact: []string{
		`Bash|{"description":"старая запись руками","command":"git status"}`,
	}}
	if !p.HasExact(ExactKey("Bash", []byte(`{"command":"git status","description":"fresh"}`))) {
		t.Error("легаси-запись перестала узнаваться после канонизации")
	}
	if p.HasExact(ExactKey("Bash", []byte(`{"command":"git push"}`))) {
		t.Error("узналась ЧУЖАЯ команда")
	}
}
