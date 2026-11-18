package chat

import (
	"errors"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/llm"
)

// unauthorizedErr — the backend refused the sign-in (401). Decided by the
// error's type, llm.ErrUnauthorized, not by its words: those are in the
// interface language now. A "401" in a message from elsewhere still counts.
func unauthorizedErr(err error) bool {
	return err != nil && (errors.Is(err, llm.ErrUnauthorized) || strings.Contains(err.Error(), "401"))
}
