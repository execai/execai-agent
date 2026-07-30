// Package messages/es_subs — cadenas en español para los comandos de
// suscripciones/fuentes (/connect, /disconnect, /source, /subscriptions).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var esSubsMessages = map[string]string{
	// === /disconnect ===
	"subs.noSubs":              "no hay suscripciones",
	"subs.disconnect.notFound": "suscripción %q no encontrada",
	"subs.disconnect.none":     "No hay suscripciones conectadas.",
	"subs.disconnect.header":   "/disconnect <provider>. Conectadas:\n",
	"subs.saveError":           "error al guardar: %s",
	"subs.disconnected":        "✓ %s desconectado",

	// === /source (lista) ===
	"subs.useDeprecated":     "ℹ /use — obsoleto, usa /source\n\n",
	"subs.source.listHeader": "Fuentes disponibles (/source <name>):\n",
	"subs.source.listExecai": "  • execai  — nuestro billing (por defecto)\n",
	"subs.source.listItem":   "  • %-8s — suscripción %s\n",
	"subs.source.listFooter": "\nConsejo: escribe '/source ' y pulsa Tab para ver el menú.",

	// === /source <name> ayuda en caso de fallo ===
	"subs.howto.zai": "\n\nCómo conectar Z.ai:\n" +
		"  1. Obtén la clave en https://z.ai/manage-apikey/apikey-list\n" +
		"     (sección Individual Coding Plan → Plan Overview)\n" +
		"  2. /connect zai sk-zai-XXXXXX\n" +
		"  3. /source zai",
	"subs.howto.anthropic": "\n\nCómo conectar Anthropic:\n" +
		"  1. Obtén una API key en https://console.anthropic.com/settings/keys\n" +
		"     (formato sk-ant-... billing pay-per-token)\n" +
		"  2. /connect anthropic sk-ant-XXXXXX\n" +
		"  3. /source anthropic",
	"subs.sourceSwitched": "✓ fuente: %s · modelo: %s",

	// === /connect (sin argumentos) ===
	"subs.connect.usageShort": "Uso: /connect <provider> <api_key>\n" +
		"Soportado: zai (Z.ai Coding Plan)\n" +
		"Consejo: '/connect ' + Tab muestra el menú de proveedores.",

	// === /subscriptions ===
	"subs.list.empty": "No hay suscripciones conectadas.\n" +
		"Conectar: /connect zai (Z.ai Coding Plan)\n" +
		"Fuente actual: ExecAI (nuestro billing)",
	"subs.list.header":         "Suscripciones conectadas (activa: %s):\n",
	"subs.list.switchHint":     "\nCambiar:      /source <provider>  (o /source execai → nuestro billing)\n",
	"subs.list.disconnectHint": "Desconectar: /disconnect <provider>",

	// === Descripciones de proveedores (menú /source) ===
	"subs.provider.execai":    "nuestro billing (por defecto)",
	"subs.provider.zai":       "Z.ai GLM Coding Plan",
	"subs.provider.kimi":      "Kimi Code Coding Plan (K3/K2.7, suscripción kimi.com/code)",
	"subs.provider.kimiapi":   "Moonshot Platform (pay-per-token, platform.moonshot.ai)",
	"subs.provider.anthropic": "Anthropic API (sk-ant-…)",
	"subs.provider.openai":    "OpenAI Platform (sk-… de platform.openai.com, pay-per-token)",
	"subs.provider.codexcli":  "OpenAI Codex CLI local (cuota de ChatGPT Plus/Pro)",
	"subs.provider.claudecli": "Claude Code local (cuota de la suscripción Pro/Max)",
	"subs.provider.ollama":    "Ollama runner local (localhost:11434)",

	// === Hints del menú ===
	"subs.hint.connected":     "conectado",
	"subs.hint.connectedPlan": "conectado · %s",
	"subs.hint.notConnected":  "no conectado — /connect %s",
	"subs.hint.remove":        "eliminar suscripción",

	// === Hints del menú /connect ===
	"subs.connectHint.zai":       "Z.ai GLM Coding Plan (API key del Coding Plan)",
	"subs.connectHint.kimi":      "Kimi Code Coding Plan (K3/K2.7, clave de kimi.com/code/console)",
	"subs.connectHint.kimiapi":   "Moonshot Platform pay-per-token (clave de platform.moonshot.ai)",
	"subs.connectHint.anthropic": "Claude API (sk-ant-... de console.anthropic.com)",
	"subs.connectHint.openai":    "OpenAI Platform pay-per-token (sk-… de platform.openai.com)",
	"subs.connectHint.codexcli":  "OpenAI Codex CLI local (sin clave, requiere `codex` instalado)",
	"subs.connectHint.claudecli": "Claude Code local (sin clave, requiere `claude` instalado)",
	"subs.connectHint.ollama":    "Ollama runner local (localhost:11434, sin clave)",

	// === Flujo /connect ===
	"subs.connect.usage": "Uso: /connect <provider> <api_key> [base_url]\n" +
		"Soportado: zai (Z.ai Coding Plan)\n" +
		"Ejemplo:  /connect zai sk-zai-XXXXX\n" +
		"          /connect zai sk-zai-XXXXX https://open.bigmodel.cn/api/paas/v4  (para CN)",
	"subs.connect.claudecliOK": "✓ claude-cli conectado (cuota de tu suscripción Pro/Max vía OAuth de Claude Code).\n" +
		"Cambia con: /source claude-cli\n" +
		"\n" +
		"Limitaciones:\n" +
		"  • execai-tools (Bash/Read/Write) NO funcionan — el CLI de claude ejecuta SUS PROPIOS tools.\n" +
		"  • Gestión del modelo — mediante `claude config set defaultModel <id>` por fuera.\n" +
		"  • El historial se pasa como prompt de texto plano (sin session-id).",
	"subs.connect.codexcliOK": "✓ codex-cli conectado (cuota de tu suscripción ChatGPT Plus/Pro vía OAuth de OpenAI Codex).\n" +
		"Cambia con: /source codex-cli\n" +
		"\n" +
		"Requisitos:\n" +
		"  • `codex` instalado en PATH (github.com/openai/codex).\n" +
		"  • `codex login` hecho con tu cuenta de ChatGPT.\n" +
		"\n" +
		"Limitaciones:\n" +
		"  • execai-tools NO funcionan — codex ejecuta SUS PROPIOS tools.\n" +
		"  • Sin streaming — codex exec devuelve el texto final.",
	"subs.connect.ollamaHelp": "Ollama — 2 modos de conexión:\n" +
		"\n" +
		"🌩  CLOUD (ollama.com):\n" +
		"   Los modelos glm-5.2, qwen3-coder-30b y otros corren en sus servidores.\n" +
		"   Endpoint compatible con Anthropic. Requiere API key de https://ollama.com/settings/keys\n" +
		"   Uso:  /connect ollama <api-key>\n" +
		"\n" +
		"🏠  LOCAL (tu propio Ollama):\n" +
		"   Modelos vía `ollama pull <name>`, corren en local, gratis.\n" +
		"   Endpoint compatible con OpenAI. Sin clave.\n" +
		"   Uso:  /connect ollama local\n" +
		"         /connect ollama local http://192.168.1.10:11434  (URL propia)",
	"subs.connect.ollamaLocalOK": "✓ ollama (local) conectado: %s\nModelos (%d):  %s\n\nCambia con: /source ollama",
	"subs.connect.ollamaCloudOK": "✓ ollama (cloud) conectado: %s\n" +
		"Modelos: glm-5.2, qwen3-coder-30b, kimi-k2 y más (ver https://ollama.com/library)\n" +
		"\nCambia con: /source ollama\n" +
		"Cambiar modelo:  /model",
	"subs.connect.unsupported": "el proveedor %q aún no está soportado. Disponible: zai, anthropic, openai, kimi (Kimi Code Coding Plan), kimi-api (Moonshot Platform pay-per-token), claude-cli, codex-cli, ollama",

	"subs.connect.example.zai":       "Ejemplo: /connect zai sk-zai-XXXXX",
	"subs.connect.example.anthropic": "Ejemplo: /connect anthropic sk-ant-XXXXX  (clave de https://console.anthropic.com/settings/keys)",
	"subs.connect.example.kimi":      "Ejemplo: /connect kimi sk-XXXXX  (Kimi Code Coding Plan de https://www.kimi.com/code/console)",
	"subs.connect.example.kimiapi":   "Ejemplo: /connect kimi-api sk-XXXXX  (Moonshot Platform pay-per-token de https://platform.moonshot.ai/console/api-keys)",
	"subs.connect.example.openai":    "Ejemplo: /connect openai sk-proj-XXXXX  (clave de https://platform.openai.com/api-keys)",
	"subs.connect.keyRequired":       "La API key es obligatoria. %s",

	"subs.connect.kimiApiRejected": "✗ clave rechazada por api.moonshot.ai (HTTP %d).\n" +
		"Puede que hayas usado una clave de Kimi Code en lugar de una de Moonshot Platform.\n\n" +
		"Las claves son distintas:\n" +
		"  • Kimi Code (suscripción):   /connect kimi <key>  — de https://www.kimi.com/code/console\n" +
		"  • Moonshot pay-per-token:    /connect kimi-api <key> — de https://platform.moonshot.ai/console/api-keys",
	"subs.connect.openaiRejected": "✗ clave rechazada por api.openai.com (HTTP %d).\n" +
		"Verifica: que la clave esté copiada completa y siga vigente (no revocada).\n" +
		"Obtener una clave: https://platform.openai.com/api-keys",
	"subs.connect.availableModels": "\n  Modelos disponibles: %s",
	"subs.connected.via":           "✓ %s conectado vía %s.%s\nCambia con:  /source %s",
	"subs.connected":               "✓ %s conectado. Cambia con:  /source %s",
}

func init() { i18n.Register("es", esSubsMessages) }
