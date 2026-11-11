package messages

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// A failure in Russian reads «не удалось начать вход», not the clipped «не
// начать вход». The few keys below are deliberate imperatives — an option
// or a flag that says what will not be done.
func TestRussian_NoClippedFailures(t *testing.T) {
	imperative := map[string]bool{
		"security.desc.light": true, "security.level.light": true, "ide.approve.denyDesc": true,
		"import.cmd.flagDryRun": true, "import.cmd.flagYes": true, "ui.flag.noOpen": true,
	}
	clipped := regexp.MustCompile(`(^|[^А-Яа-яЁё])[Нн]е [а-яё]+(ть|ти|чь)(ся)?([^а-яё]|$)`)
	modal := regexp.MustCompile(`(удалось|удаётся|получилось|можно|нужно|надо|стоит|могу|может|будет|нельзя|прошу|следует|должен|должна|должны)\s+[Нн]е [а-яё]+(ть|ти|чь)`)
	files, _ := filepath.Glob("ru*.go")
	fset := token.NewFileSet()
	for _, f := range files {
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			kv, ok := n.(*ast.KeyValueExpr)
			if !ok {
				return true
			}
			k, ok := kv.Key.(*ast.BasicLit)
			if !ok || k.Kind != token.STRING {
				return true
			}
			key, _ := strconv.Unquote(k.Value)
			if imperative[key] {
				return true
			}
			ast.Inspect(kv.Value, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				v, _ := strconv.Unquote(lit.Value)
				if clipped.MatchString(v) && !modal.MatchString(v) {
					t.Errorf("%s: %s: %q", fset.Position(lit.Pos()), key, v)
				}
				return true
			})
			return false
		})
	}
}
