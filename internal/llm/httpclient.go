package llm

import (
	"net"
	"net/http"
	"time"
)

// newStreamHTTPClient — клиент для стриминговых запросов к провайдерам.
//
// Общий Timeout ограничивает ВЕСЬ запрос (генерация бывает долгой), но без
// таймаутов на connect и TLS недоступный провайдер (blackhole, файрвол)
// означал немой висяк на весь общий таймаут: ни ошибки, ни текста — в панели
// просто «думает» десять минут. Соединиться быстро обязан любой живой
// провайдер; думать долго — может.
func newStreamHTTPClient(total time.Duration) *http.Client {
	return &http.Client{
		Timeout: total,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 2 * time.Minute,
			ForceAttemptHTTP2:     true,
		},
	}
}
