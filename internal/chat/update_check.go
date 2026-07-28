package chat

import (
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// updateAvailableMsg — событие "в проде есть свежая версия".
type updateAvailableMsg struct {
	hint string
}

// updateLatestMsg — мы на актуальной версии (показывается коротко в status_message).
type updateLatestMsg struct{}

// updateChannel — какую ветку проверяем для обновлений. По умолчанию R5.
func updateChannel() string {
	if v := strings.TrimSpace(os.Getenv("EXECAI_UPDATE_CHANNEL")); v != "" {
		return v
	}
	return "R5"
}

func updateVersionURL() string {
	return "https://storage.yandexcloud.net/execai-agent-prod/execai/" + updateChannel() + "/latest/VERSION.txt"
}

func updateInstallURL() string {
	return "https://storage.yandexcloud.net/execai-agent-prod/execai/" + updateChannel() + "/latest/install.sh"
}

// checkForUpdateCmd — тянет VERSION.txt из prod-бакета и сравнивает с фактической
// версией запущенного бинаря (agentVersion, зашитый через -ldflags -X main.version).
//
// Раньше сравнивали SHA256SUMS архива с локальным installed_arch_sha stamp'ом,
// который писал install.sh. Проблема: если у юзера в PATH два бинаря (один
// свежий из install.sh, другой старый в другом каталоге), stamp был бы новый,
// а запускался старый — и UI врал "последняя версия" при устаревшем бинаре.
//
// Теперь сравниваем ФАКТИЧЕСКУЮ версию бинаря. Ошибка невозможна: agentVersion
// в бинаре и VERSION.txt на бакете одного релиза совпадут только если бинарь
// реально того же билда.
func (m *tuiModel) checkForUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 8 * time.Second}
		resp, err := client.Get(updateVersionURL())
		if err != nil || resp == nil {
			return nil
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil || len(body) == 0 {
			return nil
		}
		remoteVer := strings.TrimSpace(string(body))
		if remoteVer == "" {
			return nil
		}
		localVer := strings.TrimSpace(agentVersion)
		// dev-сборка (без ldflags) — не спамить обновлениями.
		if localVer == "" || localVer == "dev" {
			return nil
		}
		if localVer == remoteVer {
			return updateLatestMsg{}
		}
		return updateAvailableMsg{
			hint: "🔔 Доступна новая версия execai: " + localVer + " → " + remoteVer + "\n" +
				"   Обновить:  curl -fsSL " + updateInstallURL() + " | bash\n" +
				"   (Windows:  iwr -useb " + strings.Replace(updateInstallURL(), "install.sh", "install.ps1", 1) + " | iex)",
		}
	}
}
