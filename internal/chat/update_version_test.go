package chat

import "testing"

// The version banner lies if it only checks inequality: a dev build, or a
// channel rolled back, would tell the user to "update" to something older.
func TestRemoteIsNewer(t *testing.T) {
	cases := []struct {
		local, remote string
		want          bool
		why           string
	}{
		{"R5.150", "R5.152", true, "обычное обновление"},
		{"R5.152", "R5.152", false, "та же версия"},
		{"R5.152", "R5.150", false, "откат канала — не предлагать"},
		{"5.155-dev", "R5.152", false, "дев-сборка новее прода"},
		{"5.150-dev", "R5.152", true, "дев-сборка старее прода"},
		{"5.152", "R5.152", false, "разный формат, одна версия"},
		{"R5.9", "R5.10", true, "числовое сравнение, не строковое"},
		{"R5.152", "", false, "пустой ответ сервера"},
		{"R5.152", "мусор", false, "нечисловая версия — молчим"},
		{"dev", "R5.152", false, "локальная версия без номера"},
	}
	for _, c := range cases {
		if got := remoteIsNewer(c.local, c.remote); got != c.want {
			t.Errorf("remoteIsNewer(%q, %q) = %v, ожидалось %v — %s",
				c.local, c.remote, got, c.want, c.why)
		}
	}
}

// Cyrillic arguments in a tool_call were cut mid-character and the terminal
// rendered the tail as garbage.
func TestTruncate_RuneSafe(t *testing.T) {
	s := "Ты — субагент, выполни задачу"
	got := truncate(s, 5)
	if want := "Ты — …"; got != want {
		t.Errorf("truncate = %q, ожидалось %q", got, want)
	}
	for _, r := range got {
		if r == '�' {
			t.Fatalf("обрезка разрезала символ: %q", got)
		}
	}
	if truncate("короткая", 100) != "короткая" {
		t.Error("короткая строка не должна меняться")
	}
}
