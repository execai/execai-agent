// Альфа-авторизация: пользователь берёт JWT из веб-сессии ExecAI и
// вставляет его в CLI. Дальше CLI ходит к api-vbai с Authorization Bearer.
// Когда в auth-vbai появится OAuth Authorization Code Flow с PKCE и
// loopback-redirect (как у claude code / gh / gcloud) — заменим этот файл.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
)

// Verify дёргает /auth-vbai/current-user. Возвращает email пользователя.
func Verify(ctx context.Context, apiBase, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/auth-vbai/current-user", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	cli := &http.Client{Timeout: 15 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("запрос /current-user: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return "", errors.New("токен отклонён сервером (401). Возможно истёк или неверный")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("неожиданный ответ %d: %s", resp.StatusCode, truncate(string(body), 300))
	}

	// Сервер возвращает что-то вроде {"email":"...","name":"..."} — берём email.
	var u struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return "", fmt.Errorf("ответ не-JSON: %s", truncate(string(body), 300))
	}
	if u.Email == "" {
		return "", fmt.Errorf("в ответе нет email: %s", truncate(string(body), 300))
	}
	return u.Email, nil
}

// Login сохраняет токен после проверки. token может быть с префиксом "Bearer ".
func Login(ctx context.Context, cfg *config.Config, token string) (*config.Credentials, error) {
	token = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(token), "Bearer "))
	if token == "" {
		return nil, errors.New("пустой токен")
	}
	email, err := Verify(ctx, cfg.APIBase, token)
	if err != nil {
		return nil, err
	}
	cr := &config.Credentials{
		Token:   token,
		Email:   email,
		SavedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := config.SaveCredentials(cr); err != nil {
		return nil, err
	}
	return cr, nil
}

// Logout удаляет credentials.json, если есть.
func Logout() error {
	return config.DeleteCredentials()
}

// Require возвращает текущие credentials, или ошибку с подсказкой про login.
func Require() (*config.Credentials, error) {
	cr, err := config.LoadCredentials()
	if err != nil {
		return nil, err
	}
	if cr == nil || cr.Token == "" {
		return nil, errors.New("вы не залогинены. Запустите: agent-vbai login")
	}
	return cr, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
