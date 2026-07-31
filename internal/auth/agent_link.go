// Device-flow login for the CLI (like gh auth login / aws sso login).
// Replaces the old paste-token approach in `execai login`.
//
//  1. StartAgentLink — the CLI sends hostname/OS, receives user_code + link_token + verify_uri.
//  2. The CLI shows the user_code/URL to the user and optionally opens the browser.
//  3. The CLI polls PollAgentLink(linkToken) until confirmed in the browser.
//  4. JWT + agent_id + alias + user_email are returned and saved to credentials.json.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/hostid"
)

// LinkStart is the /agent-link/start response.
type LinkStart struct {
	UserCode     string `json:"user_code"`
	LinkToken    string `json:"link_token"`
	VerifyURI    string `json:"verify_uri"`
	ExpiresIn    int    `json:"expires_in"`
	PollInterval int    `json:"poll_interval"`
}

// LinkPoll is the /agent-link/poll response.
type LinkPoll struct {
	Status    string `json:"status"` // pending | linked | expired
	JWT       string `json:"jwt"`
	AgentID   string `json:"agent_id"`
	Alias     string `json:"alias"`
	UserEmail string `json:"user_email"`
}

// StartAgentLink initiates the device flow: sends agent_type/hostname/os +
// device_id (to reuse a persistent Session on the backend) + alias_suggestion
// (for the hybrid UX — the browser pre-fills this name in the form, the user
// can edit it). Receives back user_code + link_token + verify_uri.
func StartAgentLink(ctx context.Context, apiBase string) (*LinkStart, error) {
	hostname, _ := os.Hostname()
	osInfo := runtime.GOOS + "/" + runtime.GOARCH
	deviceID, _ := hostid.Get() // if it fails entirely — empty string (the backend creates a new session)
	body, _ := json.Marshal(map[string]string{
		"agent_type":       "execai-cli",
		"hostname":         hostname,
		"os_info":          osInfo,
		"device_id":        deviceID,
		"alias_suggestion": hostname, // the user can override it in the browser
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiBase+"/auth-vbai/agent-link/start", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	cli := &http.Client{Timeout: 15 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("agent-link/start вернул %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	var out LinkStart
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("agent-link/start: %w; body=%s", err, truncate(string(data), 300))
	}
	if out.PollInterval <= 0 {
		out.PollInterval = 3
	}
	return &out, nil
}

// PollAgentLink calls /agent-link/poll once and returns the current status.
func PollAgentLink(ctx context.Context, apiBase, linkToken string) (*LinkPoll, error) {
	body, _ := json.Marshal(map[string]string{"link_token": linkToken})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiBase+"/auth-vbai/agent-link/poll", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	cli := &http.Client{Timeout: 15 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("agent-link/poll вернул %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	var out LinkPoll
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("agent-link/poll: %w", err)
	}
	return &out, nil
}

// WaitLinkUntilLinked polls /agent-link/poll every start.PollInterval seconds
// until the server returns linked (or expired/timeout). On success it saves
// the credentials and returns them.
func WaitLinkUntilLinked(ctx context.Context, cfg *config.Config, start *LinkStart, deadline time.Duration, onTick func()) (*config.Credentials, error) {
	// Default: 15 min if ExpiresIn=0 (server did not send it).
	if deadline <= 0 {
		deadline = 15 * time.Minute
	}
	interval := time.Duration(start.PollInterval) * time.Second
	if interval <= 0 {
		interval = 3 * time.Second
	}
	until := time.Now().Add(deadline)
	consecutiveErrors := 0
	for time.Now().Before(until) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		p, err := PollAgentLink(ctx, cfg.APIBase, start.LinkToken)
		if err != nil {
			logPollDiag(cfg, fmt.Sprintf("ERR: %v", err))
			// Transient error (timeout, 5xx) — do not break the whole flow,
			// keep polling. Tolerate up to 10 in a row.
			consecutiveErrors++
			if consecutiveErrors >= 10 {
				return nil, fmt.Errorf("polling упал 10 раз подряд: %w", err)
			}
			if onTick != nil {
				onTick()
			}
			time.Sleep(interval)
			continue
		}
		logPollDiag(cfg, fmt.Sprintf("status=%s", p.Status))
		consecutiveErrors = 0
		switch p.Status {
		case "linked":
			cr := &config.Credentials{
				Token:     p.JWT,
				Email:     p.UserEmail,
				AgentID:   p.AgentID,
				Alias:     p.Alias,
				AgentType: "execai-cli",
				SavedAt:   time.Now().UTC().Format(time.RFC3339),
			}
			if err := config.SaveCredentials(cr); err != nil {
				return nil, err
			}
			return cr, nil
		case "expired":
			return nil, fmt.Errorf("link expired — запусти /login заново")
		}
		if onTick != nil {
			onTick()
		}
		time.Sleep(interval)
	}
	return nil, fmt.Errorf("timeout ожидания подтверждения в браузере (15 мин)")
}

// logPollDiag writes a line to ~/.local/share/agent-vbai/auth-poll.log
// so one can see what the backend actually returns during the device flow.
// Non-blocking, errors are ignored.
func logPollDiag(cfg *config.Config, msg string) {
	dir, err := config.Dir()
	if err != nil {
		return
	}
	f, err := os.OpenFile(dir+"/auth-poll.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s [%s] %s\n", time.Now().Format(time.RFC3339), cfg.APIBase, msg)
}

// OpenBrowser is a cross-platform xdg-open / open / start. Returns an error
// on failure — the CLI then asks the user to open the URL manually.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported OS")
	}
	return cmd.Start()
}

