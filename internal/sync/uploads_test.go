package sync

import (
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// Files execai ui saved in .execai/uploads/ stay on their machine: they are
// not packed into the project blob, and a pull from another machine does not
// delete them as files the project no longer has.
func TestProjects_UploadsStayLocal(t *testing.T) {
	i18n.SetLocale("en")
	stub, a, b, pA, pB := projectPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "memory\n", time.Time{})
	writeProjectFile(t, pA, "uploads/1-shot.png", strings.Repeat("x", MaxProjectBlobBytes+1), time.Time{})
	now(a)
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted {
		t.Fatal("an upload made the project too big to send")
	}

	writeProjectFile(t, pB, "uploads/2-mine.txt", "b's file", time.Time{})
	now(b)
	if readProjectFile(pB, "EXECAI.md") != "memory\n" {
		t.Fatal("project did not arrive")
	}
	if readProjectFile(pB, "uploads/1-shot.png") != "<missing>" {
		t.Fatal("A's upload travelled")
	}
	if readProjectFile(pB, "uploads/2-mine.txt") != "b's file" {
		t.Fatal("a pull deleted B's own upload")
	}
	if !localOnlyRel("uploads/x.png") || !localOnlyRel("uploads") || localOnlyRel("notes/uploads.md") {
		t.Fatal("localOnlyRel")
	}
}
