package ide

import (
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// The first message of a project used to vanish from the panel: with no chat
// yet, the agent made one for the message and announced it with chat_reset —
// after the panel had drawn the message, so the reset cleared it. Now the
// answer to resume_last in a project without chats opens the first chat
// itself: the panel has it before anything is typed, addresses the message
// to it, and nothing is reset under the message.
func TestResumeLast_OpensTheFirstChatOfAProject(t *testing.T) {
	out := &syncBuf{}
	in, _ := runWithNotices(t, NewSyncNotices(0), out)
	out.waitEvent(t, func(o Out) bool { return o.Type == "ready" })

	send(t, in, `{"type":"command","name":"resume_last"}`)
	out.waitEvent(t, isNotice(i18n.T("ide.resume.none")))
	evs := out.events()
	reset := indexOf(evs, func(o Out) bool { return o.Type == "chat_reset" })
	answer := indexOf(evs, isNotice(i18n.T("ide.resume.none")))
	if reset < 0 || evs[reset].Chat == "" {
		t.Fatalf("no chat_reset with a chat before the answer: %+v", evs)
	}
	if reset > answer {
		t.Errorf("chat_reset (%d) must come before the answer to resume_last (%d): the panel takes the notice for that answer", reset, answer)
	}
	chat := evs[reset].Chat

	// The chat is the active one in the list, though nothing is saved yet.
	send(t, in, `{"type":"command","name":"list_chats"}`)
	list := out.waitEvent(t, func(o Out) bool { return o.Type == "chats" })
	found := false
	for _, c := range list.Chats {
		if c.ID == chat && c.Active {
			found = true
		}
	}
	if !found {
		t.Errorf("the first chat %s is not the active one in %+v", chat, list.Chats)
	}

	// Asking again (another page of execai ui, the menu) opens nothing new.
	send(t, in, `{"type":"command","name":"resume_last"}`)
	// The first message, addressed to the chat as the panel does it: its
	// events belong to that chat, and no reset comes with them.
	send(t, in, `{"type":"user","text":"hello","chat":"`+chat+`"}`)
	out.waitEvent(t, func(o Out) bool {
		return o.Chat == chat && (o.Type == "turn_start" || o.Type == "error" || o.Type == "done")
	})
	time.Sleep(200 * time.Millisecond)
	resets := 0
	for _, o := range out.events() {
		if o.Type == "chat_reset" {
			resets++
		}
	}
	if resets != 1 {
		t.Errorf("chat_reset came %d times; once — when the chat was opened — is all there may be", resets)
	}
}

// A project that has chats is resumed as before: its last chat is loaded,
// no empty one is opened next to it. (A new chat asked for by hand is not a
// resume either: it already answers with its own chat_reset.)
func TestResumeLast_WithAChatOpenOpensNothing(t *testing.T) {
	out := &syncBuf{}
	in, _ := runWithNotices(t, NewSyncNotices(0), out)
	out.waitEvent(t, func(o Out) bool { return o.Type == "ready" })
	send(t, in, `{"type":"new_chat"}`)
	out.waitEvent(t, func(o Out) bool { return o.Type == "chat_reset" })
	send(t, in, `{"type":"command","name":"resume_last"}`)
	out.waitEvent(t, isNotice(i18n.T("ide.resume.none")))
	time.Sleep(200 * time.Millisecond)
	resets := 0
	for _, o := range out.events() {
		if o.Type == "chat_reset" {
			resets++
		}
	}
	if resets != 1 {
		t.Errorf("chat_reset came %d times, want only the one of new_chat", resets)
	}
}
