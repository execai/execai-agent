package sessions

import (
	"testing"
	"time"
)

// A subagent's transcript is stored like any session but is not a chat:
// /sessions, /resume, the panel's tabs and auto-resume must not offer it,
// and the agents list finds it by its parent.
func TestList_HidesSubagentSessions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)

	parent := New("m", "p", "/proj")
	parent.Title = "main chat"
	if err := parent.Save(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	for i, title := range []string{"first", "second"} {
		sub := New("m", "p", "/proj")
		sub.Origin = OriginSubagent
		sub.ParentID = parent.ID
		sub.ParentToolCall = "call-1-" + string(rune('0'+i))
		sub.Title = title
		sub.State = "running"
		if err := sub.Save(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != parent.ID {
		t.Errorf("List = %d sessions, want only the chat", len(list))
	}
	if mr := MostRecent(); mr == nil || mr.ID != parent.ID {
		t.Errorf("MostRecent picked a subagent transcript: %+v", mr)
	}
	all, _ := ListAll()
	if len(all) != 3 {
		t.Errorf("ListAll = %d, want 3", len(all))
	}
	kids, err := Children(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(kids) != 2 || kids[0].Title != "first" || kids[1].Title != "second" {
		t.Errorf("Children = %+v", kids)
	}
	if !kids[0].IsSubagent() || kids[0].ParentToolCall != "call-1-0" || kids[0].State != "running" {
		t.Errorf("subagent fields lost: %+v", kids[0])
	}
}
