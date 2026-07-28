// Device-flow login для CLI (как gh auth login / aws sso login).
// Заменяет старый paste-token подход в `execai login`.
//
//  1. StartAgentLink — CLI шлёт hostname/OS, получает user_code + link_token + verify_uri.
//  2. CLI показывает пользователю user_code/URL и опционально открывает браузер.
//  3. CLI поллит PollAgentLink(linkToken) — пока в браузере не подтвердят.
//  4. Возвращается JWT + agent_id + alias + user_email — сохраняются в credentials.json.
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

// LinkStart — ответ /agent-link/start.
type LinkStart struct {
	UserCode     string `json:"user_code"`
	LinkToken    string `json:"link_token"`
	VerifyURI    string `json:"verify_uri"`
	ExpiresIn    int    `json:"expires_in"`
	PollInterval int    `json:"poll_interval"`
}

// LinkPoll — ответ /agent-link/poll.
type LinkPoll struct {
	Status    string `json:"status"` // pending | linked | expired
	JWT       string `json:"jwt"`
	AgentID   string `json:"agent_id"`
	Alias     string `json:"alias"`
	UserEmail string `json:"user_email"`
}

// StartAgentLink инициализирует device-flow: шлёт agent_type/hostname/os +
// device_id (для reuse persistent-Session'а на бэке) + alias_suggestion
// (для гибридного UX — браузер преставит это имя в форме, юзер может править).
// Получает обратно user_code + link_token + verify_uri.
func StartAgentLink(ctx context.Context, apiBase string) (*LinkStart, error) {
	hostname, _ := os.Hostname()
	osInfo := runtime.GOOS + "/" + runtime.GOARCH
	deviceID, _ := hostid.Get() // если совсем не получилось — пустая строка (бэк создаст новую сессию)
	body, _ := json.Marshal(map[string]string{
		"agent_type":       "execai-cli",
		"hostname":         hostname,
		"os_info":          osInfo,
		"device_id":        deviceID,
		"alias_suggestion": hostname, // юзер сможет переопределить в браузере
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

// PollAgentLink один раз дёргает /agent-link/poll и возвращает текущий статус.
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

// WaitLinkUntilLinked поллит /agent-link/poll каждые start.PollInterval секунд
// пока сервер не вернёт linked (или expired/timeout). При успехе сохраняет
// credentials и возвращает их.
func WaitLinkUntilLinked(ctx context.Context, cfg *config.Config, start *LinkStart, deadline time.Duration, onTick func()) (*config.Credentials, error) {
	// Дефолт: 15 мин если ExpiresIn=0 (сервер не прислал).
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
			// Транзиентная ошибка (timeout, 5xx) — не рушим весь flow,
			// продолжаем поллить. Терпим до 10 подряд.
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

// logPollDiag — пишет строку в ~/.local/share/agent-vbai/auth-poll.log
// чтобы можно было понять что реально возвращает бэк во время device-flow.
// Не блокирующий, ошибки игнорируются.
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

// OpenBrowser — кросс-платформенный xdg-open / open / start. Возвращает ошибку
// если не получилось — тогда CLI просит пользователя открыть руками.
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

