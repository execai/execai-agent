// Package messages/es_custom — Spanish strings for live model lists and the
// custom provider (/connect custom). Merged into the "es" catalog.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var esCustomMessages = map[string]string{
	"catalog.cliDefault":               "Predeterminado",
	"catalog.cliDefaultDesc":           "sin --model: el modelo de la configuración del propio CLI",
	"subs.connect.modelsCount":         "Modelos que informa el proveedor: %d — elige con /model.",
	"subs.connect.noModels":            "El proveedor no informó ningún modelo. Indícalo tú: /model <id> — se recordará para esta fuente.",
	"subs.connect.ollamaLocalFailed":   "✗ Ollama en %s no responde (HTTP %d). ¿Está en marcha? `ollama serve`",
	"subs.connect.ollamaCloudRejected": "✗ ollama.com rechazó la clave (HTTP %d). Claves: https://ollama.com/settings/keys",
	"ui.model.listEmpty":               "Esta fuente aún no ha informado ningún modelo. Indica uno: /model <id>",
	"ui.model.typed":                   "El modelo %q no está en la lista del proveedor — se usa tal como se escribió y se recuerda para esta fuente.",
	"hint.connect.custom":              "proveedor propio: cualquier dirección compatible con OpenAI o Anthropic",
	"subs.connectHint.custom":          "proveedor propio: protocolo, dirección, clave — paso a paso",
	"subs.hint.custom":                 "propio (%s) · %s",
	"subs.custom.usage": "Uso:\n" +
		"  /connect custom                                — paso a paso\n" +
		"  /connect custom <nombre> <openai|anthropic> <base_url> [api_key] [--models a,b] [--header \"Nombre: valor\"]\n" +
		"Ejemplos:\n" +
		"  /connect custom \"LM Studio\" openai http://localhost:1234/v1\n" +
		"  /connect custom deepseek openai https://api.deepseek.com sk-xxx\n" +
		"  /connect custom corp anthropic https://llm.corp.local sk-xxx --header \"X-Tenant: dev\"",

	"custom.wiz.intro":              "Conectando tu propio proveedor — cualquier servidor con API al estilo OpenAI o Anthropic. Esc cancela.",
	"custom.wiz.name":               "Paso 1/4. ¿Cómo se llamará? Por ejemplo: LM Studio, vLLM, deepseek, corp-gateway (letras latinas — el nombre se convierte en el id para /source).",
	"custom.wiz.idIs":               "id: %s  (luego cambia con /source <id>)",
	"custom.wiz.api":                "Paso 2/4. Tipo de interfaz:\n  1 — compatible con OpenAI (POST …/chat/completions): LM Studio, vLLM, llama.cpp, LiteLLM, DeepSeek, Groq, Together, Mistral…\n  2 — compatible con Anthropic (POST …/v1/messages): LiteLLM, pasarelas a Claude, MiniMax, DeepSeek…\nEscribe 1 o 2.",
	"custom.wiz.urlOpenAI":          "Paso 3/4. Dirección del servidor (base URL), p. ej. http://localhost:1234/v1, http://gpu-box:8000/v1, https://api.deepseek.com",
	"custom.wiz.urlAnthropic":       "Paso 3/4. Dirección del servidor (base URL) sin /v1/messages, p. ej. https://llm.corp.local, https://api.deepseek.com/anthropic",
	"custom.wiz.key":                "Paso 4/4. Clave API — Enter si no hay (los servidores locales no suelen tenerla). Se guarda solo en esta máquina y nunca va al historial de entrada.",
	"custom.wiz.noKey":              "(sin clave)",
	"custom.wiz.keyRejected":        "✗ El servidor rechazó la clave (HTTP %d). Escríbela de nuevo o Esc para cancelar.",
	"custom.wiz.models":             "El servidor no devolvió la lista de modelos (GET …/models: HTTP %d). Escribe los ID de modelos separados por comas, o Enter para omitir y fijarlo luego con /model <id>.",
	"custom.wiz.cancelled":          "Conexión de proveedor propio cancelada.",
	"custom.wiz.placeholder.name":   "nombre del proveedor, p. ej. LM Studio",
	"custom.wiz.placeholder.api":    "1 — compatible con OpenAI, 2 — compatible con Anthropic",
	"custom.wiz.placeholder.url":    "base URL, p. ej. http://localhost:1234/v1",
	"custom.wiz.placeholder.key":    "clave API, o Enter si no hay",
	"custom.wiz.placeholder.models": "ID de modelos separados por comas, o Enter",
	"custom.keyRejected":            "✗ %[2]s rechazó la clave (HTTP %[1]d).",
	"custom.connected":              "✓ Conectado: %s (id %s, protocolo %s, %s)",
	"custom.connectedModels":        "Modelos: %d, actual: %s — elige con /model.",
	"custom.connectedHint":          "Volver luego: /source %[1]s · quitar: /disconnect %[1]s",
}

func init() { i18n.Register("es", esCustomMessages) }
