// Package messages/es_agent — Spanish strings of the agent loop and
// the model catalog that the person sees: a declined tool, the scope of a
// permission, working without the catalog. Merged into the "es" catalog
// via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var esAgentMessages = map[string]string{
	"agent.tool.declined":          "El usuario rechazó ejecutar esta herramienta.",
	"agent.tool.noApprovalChannel": "execai no puede pedir confirmación para esta llamada aquí",
	"agent.approve.scope":          "el permiso abarcará: %s",
	"llm.offline.fallback":         "⚠ Sin conexión — el catálogo de modelos no está disponible, uso la lista integrada. Motivo: %v",
	"llm.offline.cache":            "ℹ Uso el catálogo de modelos en caché (antigüedad: %s). La red no está disponible: %v",
	"llm.models.unauthorized":      "401 de /billing-vbai/models_public — la sesión caducó, vuelve a iniciarla (en el chat: /login)",
	"llm.models.status":            "el catálogo de modelos respondió %d: %s",
	"llm.models.notJSON":           "la respuesta del catálogo de modelos no es una lista: %s",
	"llm.models.none":              "no hay red ni un catálogo de modelos guardado",
	"llm.err.unauthorized":         "401 de %s — %s",
	"llm.err.status":               "el servidor respondió %d: %s",
	"llm.err.server":               "el servidor devolvió un error: %v",
	"llm.err.aichatUnauthorized":   "401 de /aichat-vbai/stream — el token caducó, ejecuta execai login",
	"llm.err.sse":                  "error del servidor en el flujo SSE: %v",
	"llm.err.anthropicKey":         "401 de la API de Anthropic — clave incorrecta. Revisa /subscriptions",
	"llm.err.providerKey":          "401 de %s — clave de API incorrecta. Revisa /subscriptions y /connect",
	"llm.err.providerStatus":       "%s respondió %d: %s",
	"llm.billing.limit":            "⏱  LÍMITE AGOTADO",
	"llm.billing.funds":            "💰 SALDO INSUFICIENTE",
	"llm.billing.inactive":         "⚠  PLAN INACTIVO",
	"llm.billing.unavailable":      "El servicio no está disponible temporalmente por motivos de facturación.",
	"llm.billing.hint":             "Cambiar de plan: sección «Plan» de tu cuenta web de ExecAI (%s)",
	"llm.cli.claudeMissing":        "no se encontró claude CLI (PATH, ~/.local/bin, extensiones de VS Code/Cursor). Instalación: https://docs.claude.com/en/docs/claude-code/quickstart",
	"llm.cli.claudeNotFound":       "no se encontró claude CLI",
	"llm.cli.claudeNoModels":       "claude no devolvió ningún modelo",
	"llm.cli.codexMissing":         "no se encontró codex CLI en el PATH. Instalación: https://github.com/openai/codex\nDespués de instalarlo: codex login",
	"llm.cli.codexNotFound":        "no se encontró codex CLI en el PATH",
	"llm.cli.codexNoModels":        "codex no devolvió ningún modelo",
	"llm.cli.noAnswer":             "la CLI terminó sin responder",
	"llm.cli.emptyResult":          "resultado vacío",
	"llm.cli.emptyAnswer":          "respuesta vacía",
	"llm.cli.images":               "imágenes",
	"llm.cli.noToolResult":         "sin resultado: claude terminó antes",
	"llm.ollama.down":              "Ollama %s no está disponible",
	"llm.ollama.start":             "Inícialo: ollama serve",
	"llm.ollama.noModel":           "Ollama: el modelo %q no está instalado (¿ollama pull %s?). %s",
	"llm.ollama.status":            "Ollama respondió %d: %s",
	"llm.ollama.tagsStatus":        "Ollama /api/tags respondió %d: %s",
	"llm.ollama.tagsParse":         "Ollama /api/tags: no se pudo leer la respuesta",
	"llm.ollama.noModels":          "Ollama no tiene modelos. Instala alguno: `ollama pull llama3.2`",
	"llm.ollama.localModel":        "modelo local de Ollama",
	"agent.memory.readFailed":      "lectura de la memoria",
	"agent.memory.exportDirFailed": "creación de la carpeta de exportación",
	"agent.memory.writeFailed":     "escritura de %s",
	"agent.permissions.saveFailed": "no se pudo guardar el permiso:",
	"auth.notSignedIn":             "no has iniciado sesión. Ejecuta: execai login",
}

func init() { i18n.Register("es", esAgentMessages) }
