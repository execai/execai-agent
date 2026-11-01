package inbox

import (
	"os"
	"path/filepath"
	"testing"
)

// The editor window takes only its own project's tasks (and tasks outside
// any project); another project's task is left for whoever works there; a
// task nobody on this machine can run is taken and answered with the reason.
func TestPlace_ByFolder(t *testing.T) {
	here := t.TempDir()
	there := t.TempDir()
	binds := []Binding{
		{WorkspaceID: "ws-here", LocalPath: here},
		{WorkspaceID: "ws-there", LocalPath: there},
		{WorkspaceID: "ws-nodir", LocalPath: ""},
		{WorkspaceID: "ws-moved", LocalPath: filepath.Join(there, "gone")},
	}
	cases := []struct {
		name string
		ws   string
		want Placement
		dir  string
	}{
		{"no project — runs here", "", Here, here},
		{"this folder's project", "ws-here", Here, here},
		{"another folder's project", "ws-there", Elsewhere, there},
		{"project without a directory", "ws-nodir", Broken, ""},
		{"project directory gone", "ws-moved", Broken, ""},
		{"project not bound on this machine", "ws-unknown", Broken, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, dir, err := Place(Task{ID: "t", WorkspaceID: c.ws}, binds, here)
			if got != c.want || dir != c.dir {
				t.Errorf("Place = (%v, %q, %v), want (%v, %q)", got, dir, err, c.want, c.dir)
			}
			if (got == Broken) != (err != nil) {
				t.Errorf("error %v for placement %v", err, got)
			}
		})
	}
}

// The editor may open a project through a link the terminal never used.
func TestSamePath_Symlink(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	if !SamePath(real, link) || !SamePath(real+"/", real) {
		t.Error("the same folder not recognised")
	}
	if SamePath(real, t.TempDir()) || SamePath("", real) {
		t.Error("different folders taken as one")
	}
	// A task bound through the link is this window's.
	if got, _, _ := Place(Task{WorkspaceID: "w"}, []Binding{{WorkspaceID: "w", LocalPath: link}}, real); got != Here {
		t.Errorf("bound through a link: %v", got)
	}
}

func TestWorkDir(t *testing.T) {
	dir := t.TempDir()
	binds := []Binding{{WorkspaceID: "ws", LocalPath: dir}, {WorkspaceID: "empty"}}
	if got, err := WorkDir(binds, "ws", "/fallback"); err != nil || got != dir {
		t.Errorf("bound: %q %v", got, err)
	}
	if got, err := WorkDir(nil, "", "/fallback"); err != nil || got != "/fallback" {
		t.Errorf("outside a project: %q %v", got, err)
	}
	for _, ws := range []string{"empty", "unknown"} {
		if _, err := WorkDir(binds, ws, "/fallback"); err == nil {
			t.Errorf("%s: no error", ws)
		}
	}
}
