package messages

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// English lines quote names and words with plain double quotes ("Alpha"),
// never with «», which are Russian (and Spanish) quotes.
func TestEnglish_NoGuillemets(t *testing.T) {
	files, _ := filepath.Glob("en*.go")
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			v, _ := strconv.Unquote(lit.Value)
			if strings.ContainsAny(v, "«»") {
				t.Errorf("%s: %q", fset.Position(lit.Pos()), v)
			}
			return true
		})
	}
}
