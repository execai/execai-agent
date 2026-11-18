package writelock

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// writerAllowed — functions of the agent's writers (internal/tools,
// internal/agent) that change files without taking the lock themselves,
// and why they may.
var writerAllowed = map[string]string{
	"agent.Permissions.Save": "permissions.json in the config folder: a settings file, not the memory or a .execai/; " +
		"written through a temporary file renamed over it",
}

// writerCalls — the calls that write, rename, truncate or remove a file.
var writerCalls = map[string]bool{
	"os.WriteFile": true, "os.Create": true, "os.OpenFile": true, "os.Rename": true,
	"os.Remove": true, "os.RemoveAll": true, "os.Truncate": true, "ioutil.WriteFile": true,
}

// lockCalls — taking the lock (or writing through it).
var lockCalls = map[string]bool{
	"writelock.ForPath": true, "writelock.Hold": true, "writelock.WriteFile": true, "writelock.Update": true,
}

// The agent's own writers into the folders sync carries — the write and
// edit tools, the memory index template, /project memory, a memory import —
// take the short lock sync's replacements take: every call that changes a
// file in internal/tools and internal/agent is in a function that takes the
// lock (or writes through it), or is listed above with its reason.
func TestWriters_TakeTheLock(t *testing.T) {
	var bad []string
	seen := map[string]bool{}
	for _, pkg := range []string{"tools", "agent"} {
		files, _ := filepath.Glob(filepath.Join("..", pkg, "*.go"))
		fset := token.NewFileSet()
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			af, err := parser.ParseFile(fset, f, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range af.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				name := pkg + "." + fd.Name.Name
				if fd.Recv != nil && len(fd.Recv.List) == 1 {
					name = pkg + "." + recv(fd.Recv.List[0].Type) + "." + fd.Name.Name
				}
				var writes []token.Pos
				locks := false
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					id, ok := sel.X.(*ast.Ident)
					if !ok {
						return true
					}
					c := id.Name + "." + sel.Sel.Name
					if writerCalls[c] {
						writes = append(writes, call.Pos())
					}
					if lockCalls[c] {
						locks = true
					}
					return true
				})
				if len(writes) == 0 {
					continue
				}
				if _, ok := writerAllowed[name]; ok {
					seen[name] = true
					continue
				}
				if !locks {
					for _, p := range writes {
						bad = append(bad, fset.Position(p).String()+": "+name+" changes a file without the lock")
					}
				}
			}
		}
	}
	for name := range writerAllowed {
		if !seen[name] {
			bad = append(bad, "listed but not found (remove it): "+name)
		}
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Error(b)
	}
}

func recv(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return recv(x.X)
	case *ast.Ident:
		return x.Name
	}
	return "?"
}
