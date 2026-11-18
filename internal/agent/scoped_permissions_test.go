package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/security"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// «Навсегда» на инструменте с масштабом НЕ должно выдавать весь инструмент.
//
// Так было до 15.08.2026: плагин и веб записывали разрешение сами —
// AddTool(name) и возврат ApproveOnce, — минуя цикл. Ответ «навсегда» на один
// каталог открывал чтение ЛЮБОГО файла на машине, включая ключи и .env, то
// есть делал ровно противоположное тому, что человек нажимал. Поймано
// самопрогоном: после «навсегда» на каталог перестал спрашиваться .env.
//
// Масштаб знает только цикл (tools.Scoped), поэтому и записывать обязан он.
func TestAlwaysOnScopedToolDoesNotGrantWholeTool(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)

	work := filepath.Join(dir, "проект")
	outside := filepath.Join(dir, "снаружи")
	mustDir(t, work)
	mustDir(t, outside)
	mustFile(t, filepath.Join(outside, "a.txt"), "текст")
	mustFile(t, filepath.Join(outside, "b.txt"), "ещё текст")
	mustFile(t, filepath.Join(outside, ".env"), "SECRET=1")

	security.Set(security.Deep)

	reg := tools.ReadOnly(work)
	asked := 0
	ag := New(nil, reg, "", &recordingApprover{decision: ApproveAlways, count: &asked}, nil)

	read := func(path string) {
		t.Helper()
		if _, err := ag.runTool(context.Background(), "Read",
			json.RawMessage(`{"path":`+quote(path)+`}`)); err != nil {
			t.Fatalf("Read(%s): %v", path, err)
		}
	}

	// 1. Первое чтение снаружи: вопрос, ответ «навсегда».
	read(filepath.Join(outside, "a.txt"))
	if asked != 1 {
		t.Fatalf("ожидался ровно один вопрос, было %d", asked)
	}
	if ag.Permissions.HasTool("Read") {
		t.Fatal("выдан ВЕСЬ Read — «навсегда» на каталог открыло бы всю машину")
	}

	// 2. Соседний файл того же каталога вопроса не поднимает: иначе человек
	//    утонет в подтверждениях и выключит агента.
	read(filepath.Join(outside, "b.txt"))
	if asked != 1 {
		t.Errorf("разрешение на каталог не сработало: вопросов стало %d", asked)
	}

	// 3. Секрет в том же каталоге обязан спросить отдельно.
	read(filepath.Join(outside, ".env"))
	if asked != 2 {
		t.Error("секрет не спросил, хотя разрешение выдавалось только на каталог")
	}
}

type recordingApprover struct {
	decision ApproveDecision
	count    *int
}

func (r *recordingApprover) AskApprove(string, json.RawMessage, string) ApproveDecision {
	*r.count++
	return r.decision
}

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }

func mustDir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
