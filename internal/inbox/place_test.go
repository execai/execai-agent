package inbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// The editor window takes only its own project's tasks (and tasks outside
// any project); another project's task is left for whoever works there; a
// task nobody on this machine can run is taken and answered with the reason.
func TestPlace_ByFolder(t *testing.T) {
	here := t.TempDir()
	there := t.TempDir()
	binds := []Binding{
		{AgentID: "me", WorkspaceID: "ws-here", LocalPath: here},
		{AgentID: "me", WorkspaceID: "ws-there", LocalPath: there},
		{AgentID: "me", WorkspaceID: "ws-nodir", LocalPath: ""},
		{AgentID: "me", WorkspaceID: "ws-moved", LocalPath: filepath.Join(there, "gone")},
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
			got, dir, err := Place(Task{ID: "t", WorkspaceID: c.ws}, binds, "me", here)
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
	if got, _, _ := Place(Task{WorkspaceID: "w"}, []Binding{{AgentID: "me", WorkspaceID: "w", LocalPath: link}}, "me", real); got != Here {
		t.Errorf("bound through a link: %v", got)
	}
}

func TestWorkDir(t *testing.T) {
	dir := t.TempDir()
	binds := []Binding{{AgentID: "me", WorkspaceID: "ws", LocalPath: dir}, {AgentID: "me", WorkspaceID: "empty"}}
	if got, err := WorkDir(binds, "me", "ws", "/fallback"); err != nil || got != dir {
		t.Errorf("bound: %q %v", got, err)
	}
	if got, err := WorkDir(nil, "me", "", "/fallback"); err != nil || got != "/fallback" {
		t.Errorf("outside a project: %q %v", got, err)
	}
	for _, ws := range []string{"empty", "unknown"} {
		if _, err := WorkDir(binds, "me", ws, "/fallback"); err == nil {
			t.Errorf("%s: no error", ws)
		}
	}
}

// Two machines of one account in one project: the server lists both
// bindings. A task on this machine runs only in this machine's directory —
// never in the other one's, even when that path exists here too — and a
// project bound only on the other machine is refused, not run "somewhere".
func TestWorkDir_IgnoresOtherMachinesBinding(t *testing.T) {
	mine, theirs := t.TempDir(), t.TempDir()
	binds := []Binding{
		{AgentID: "other", WorkspaceID: "ws-both", LocalPath: theirs},
		{AgentID: "me", WorkspaceID: "ws-both", LocalPath: mine},
		{AgentID: "other", WorkspaceID: "ws-theirs", LocalPath: theirs},
		{AgentID: "other", WorkspaceID: "ws-theirs-nodir", LocalPath: ""},
	}
	if got, err := WorkDir(binds, "me", "ws-both", "/fallback"); err != nil || got != mine {
		t.Errorf("project bound on both machines: (%q, %v), want this machine's %q", got, err, mine)
	}
	for _, ws := range []string{"ws-theirs", "ws-theirs-nodir"} {
		got, err := WorkDir(binds, "me", ws, "/fallback")
		if err == nil {
			t.Errorf("%s: ran in %q — the project is bound only on the other machine", ws, got)
			continue
		}
		if err.Error() != i18n.T("inbox.task.notBound") {
			t.Errorf("%s: %v, want the not-bound refusal", ws, err)
		}
	}
	// Signed in without a machine id: no binding is this machine's.
	if got, err := WorkDir(binds, "", "ws-both", "/fallback"); err == nil {
		t.Errorf("no agent id: ran in %q", got)
	}
}

// The other machine keeps the project at the very path this window works
// in (the same layout on two computers): the task is still not this
// window's — this machine has not bound the project.
func TestPlace_SamePathOtherMachine(t *testing.T) {
	here, elsewhere := t.TempDir(), t.TempDir()
	binds := []Binding{
		{AgentID: "other", WorkspaceID: "ws-a", LocalPath: here},
		{AgentID: "other", WorkspaceID: "ws-b", LocalPath: elsewhere},
		{AgentID: "me", WorkspaceID: "ws-mine", LocalPath: here},
	}
	for _, ws := range []string{"ws-a", "ws-b"} {
		got, dir, err := Place(Task{ID: "t", WorkspaceID: ws}, binds, "me", here)
		if got != Broken || err == nil {
			t.Errorf("%s: Place = (%v, %q, %v), want Broken: bound only on the other machine", ws, got, dir, err)
		}
	}
	if got, dir, _ := Place(Task{ID: "t", WorkspaceID: "ws-mine"}, binds, "me", here); got != Here || dir != here {
		t.Errorf("own project: (%v, %q)", got, dir)
	}
}

func TestOwn(t *testing.T) {
	binds := []Binding{{AgentID: "a", WorkspaceID: "1"}, {AgentID: "b", WorkspaceID: "2"}, {WorkspaceID: "3"}}
	if got := Own(binds, "a"); len(got) != 1 || got[0].WorkspaceID != "1" {
		t.Errorf("Own(a) = %+v", got)
	}
	if got := Own(binds, ""); len(got) != 0 {
		t.Errorf("Own without an agent id = %+v", got)
	}
}
