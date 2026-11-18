package ide

import (
	"encoding/json"
	"testing"
)

// The wire shape of the events added for tasks from the site (E100.6). The
// VS Code extension parses exactly these keys; Protocol stays 1 because every
// field is optional and an older panel skips what it does not know.
func TestProtocol_InboxEvents(t *testing.T) {
	empty := []WorkspaceItem{}
	items := []WorkspaceItem{{ID: "w1", Name: "site", Active: true, Path: "/src/site"}, {ID: "w2", Name: "api", Default: true}}
	cases := []struct {
		name string
		out  Out
		want string
	}{
		{"inbox: listening, bound",
			Out{Type: "inbox", State: inboxListening, BoundWorkspace: &WorkspaceRef{ID: "w1", Name: "site"}},
			`{"type":"inbox","state":"listening","bound_workspace":{"id":"w1","name":"site"}}`},
		{"inbox: serve owns it, not bound",
			Out{Type: "inbox", State: inboxOtherServe, OwnerPID: 4242},
			`{"type":"inbox","state":"other_serve","owner_pid":4242}`},
		{"inbox: another window",
			Out{Type: "inbox", State: inboxOtherIDE, OwnerPID: 7, OwnerCwd: "/src/api"},
			`{"type":"inbox","state":"other_ide","owner_pid":7,"owner_cwd":"/src/api"}`},
		{"inbox: no plan",
			Out{Type: "inbox", State: inboxNoPlan},
			`{"type":"inbox","state":"no_plan"}`},
		{"state carries the inbox as a string",
			Out{Type: "state", Inbox: inboxListening, BoundWorkspace: &WorkspaceRef{ID: "w1", Name: "site"}},
			`{"type":"state","inbox":"listening","bound_workspace":{"id":"w1","name":"site"}}`},
		{"ask_resolved",
			Out{Type: "ask_resolved", Chat: "c1", AskID: "q3", By: resolvedByWeb},
			`{"type":"ask_resolved","chat":"c1","ask_id":"q3","by":"web"}`},
		{"turn_start of a web chat",
			Out{Type: "turn_start", Chat: "c1", Origin: originWeb},
			`{"type":"turn_start","chat":"c1","origin":"web"}`},
		{"auto_user of a task",
			Out{Type: "auto_user", Chat: "c1", Text: "fix it", Origin: originWeb},
			`{"type":"auto_user","chat":"c1","text":"fix it","origin":"web"}`},
		{"chats with origins",
			Out{Type: "chats", Chats: []NamedItem{{ID: "c1", Label: "fix it", Busy: true, Origin: originWeb}, {ID: "c2", Origin: originUser}}},
			`{"type":"chats","chats":[{"id":"c1","label":"fix it","busy":true,"origin":"web"},{"id":"c2","origin":"user"}]}`},
		{"workspaces",
			Out{Type: "workspaces", Items: &items},
			`{"type":"workspaces","items":[{"id":"w1","name":"site","active":true,"path":"/src/site"},{"id":"w2","name":"api","default":true}]}`},
		{"workspaces: an empty list is still a list",
			Out{Type: "workspaces", Items: &empty},
			`{"type":"workspaces","items":[]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := json.Marshal(c.out)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != c.want {
				t.Errorf("\n got %s\nwant %s", b, c.want)
			}
		})
	}
}

// Commands from the panel: bind takes the project in workspace_id or value.
func TestProtocol_InboxCommands(t *testing.T) {
	for _, line := range []string{
		`{"type":"command","name":"bind","workspace_id":"w1"}`,
		`{"type":"command","name":"bind","value":"w1"}`,
	} {
		var in In
		if err := json.Unmarshal([]byte(line), &in); err != nil {
			t.Fatal(err)
		}
		if got := firstNonEmpty(in.WorkspaceID, in.Value); got != "w1" {
			t.Errorf("%s → %q", line, got)
		}
	}
}
