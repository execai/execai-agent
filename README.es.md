[English](README.md) | [Русский](README.ru.md) | **Español** | [Deutsch](README.de.md) | [中文](README.zh.md)

🌐 **Sitio web:** [execai.ru](https://execai.ru) · 💬 Chat web: [chat.execai.ru](https://chat.execai.ru)

---

# execai — un agente de IA para la terminal

execai es un agente de codificación con IA nacido en la terminal que funciona con tus proveedores de LLM favoritos (Claude, GPT-5, Kimi K3, GLM-5.2 y más): trae tu propia suscripción o usa la nuestra. Un agente CLI al estilo de Claude Code que de verdad se ejecuta en tu máquina: lee archivos, ejecuta comandos, habla con kubernetes y te responde en el chat. Admite **10 fuentes**: nuestro backend de ExecAI + tus suscripciones de Z.ai / Kimi Code / Anthropic / OpenAI + tu Claude Code / OpenAI Codex CLI / Ollama local (en la nube o en local). Puedes cambiar entre ellas al vuelo compartiendo el historial de la conversación.

```
execai R5.136 · openai/gpt-5 · it@execai.ru · ~
Type a task. /model — models, /help — commands, /quit — exit.

› check free disk space on server dev01
```

![demo](scripts/demo.gif)

> **ℹ La cuenta de ExecAI es opcional.** La fuente predeterminada `execai` usa nuestro backend (registro en [execai.ru](https://execai.ru)), pero el CLI funciona por completo **sin iniciar sesión en ExecAI**: conecta tu propia suscripción desde la pantalla de login — `/connect kimi <key>`, `/connect zai <key>`, `/connect openai <key>`, `/connect anthropic <key>`, o los locales `claude-cli` / `codex-cli` / `ollama` — luego `/source <provider>` y a trabajar.

**Renderizado Ink-style** (predeterminado desde R5.107): el historial de mensajes se envía al scrollback nativo de la terminal (la selección nativa funciona y no se resetea al actualizar). El streaming en vivo y la entrada viven en un área dinámica al pie. Igual que Claude Code.

---

## Contenido

1. [Instalación](#instalación)
2. [Primera ejecución](#primera-ejecución)
3. [Hablar con el agente](#hablar-con-el-agente)
4. [Fuentes — ExecAI y suscripciones](#fuentes--execai-y-suscripciones)
5. [Modelos](#modelos)
6. [Imágenes y archivos](#imágenes-y-archivos)
7. [Effort — nivel de razonamiento](#effort--nivel-de-razonamiento)
8. [Loop y Autoloop — repetir y esperar](#loop-y-autoloop--repetir-y-esperar)
9. [Memoria del agente (EXECAI/MEMORY.md)](#memoria-del-agente)
10. [Proyectos y modo en segundo plano — web → agente](#proyectos-y-modo-en-segundo-plano--web--agente)
11. [Gasto / `/usage`](#gasto--usage)
12. [Comandos y atajos](#comandos-y-atajos)
13. [Sesiones e historial](#sesiones-e-historial)
14. [Dónde viven los archivos](#dónde-viven-los-archivos)
15. [Solución de problemas](#solución-de-problemas)

---

## Instalación

### Linux / macOS (Intel y Apple Silicon)

```bash
curl -fsSL https://raw.githubusercontent.com/execai/execai-agent/main/install.sh | bash
```

Los binarios vienen de **GitHub Releases** por defecto. En Rusia/CEI el espejo de Yandex suele ser más rápido — el script lo elige automáticamente para locales `ru`, o fuérzalo con `MIRROR=yandex`.

El script:
- detecta tu arquitectura (`linux-amd64`, `linux-arm64`, `darwin-amd64`, `darwin-arm64`),
- verifica el SHA256,
- coloca el binario en `/usr/local/bin/execai` (con sudo sin contraseña) o en `~/.local/bin/execai`,
- elimina copias obsoletas de execai del PATH y entradas muertas/`/tmp/execai*` de tus archivos rc,
- en macOS, quita `com.apple.quarantine` automáticamente (Gatekeeper).

Tras la instalación, en una **nueva terminal**, basta con escribir `execai`. En la actual, si ya tenías `execai` y bash cacheó la ruta antigua, `hash -r` lo arregla.

### Windows 10/11 (amd64 y ARM64)

```powershell
iwr -useb https://storage.yandexcloud.net/execai-agent-prod/execai/stable/install.ps1 | iex
```

Autodetecta la arquitectura (amd64 / arm64 para PCs Copilot+), pide **UAC** una vez para añadir la carpeta a las exclusiones de Defender (si no, el antivirus puede comerse el binario), descarga `execai.exe` a `%LOCALAPPDATA%\execai\` y lo registra en el PATH de usuario.

> Ejecútalo desde **Windows Terminal** (no desde el viejo `cmd.exe`/conhost): la TUI (bubbletea) se renderiza correctamente allí.

Tras instalar, verifica:
```bash
execai version
# execai R5.NN
```

### Fijar una versión concreta

```bash
curl -fsSL https://raw.githubusercontent.com/execai/execai-agent/main/install.sh | VERSION=5.136 bash
```

### Descarga manual

Si prefieres tomar el binario por tu cuenta sin ejecutar el instalador, todos los releases están en GitHub con checksums:

**https://github.com/execai/execai-agent/releases/latest**

Elige el archivo para tu plataforma (`execai-{linux,darwin,windows}-{amd64,arm64}.{tar.gz,zip}`), verifica contra `SHA256SUMS`, extráelo y añádelo al PATH.

---

## Primera ejecución

```bash
execai
```

Sin argumentos, se abre la TUI del chat.

**Si todavía no has iniciado sesión**, el agente te mostrará un enlace de confirmación en el navegador y lo abrirá por ti automáticamente:

```
👉 Open in your browser and confirm this is your agent:
   https://chat.execai.ru/agents/connect/U7XQ9F4P

(waiting, polling every 3s… once you confirm, we continue)
```

En el navegador:
1. Si no tienes sesión en ExecAI, entra (Yandex / VK / correo).
2. En la página "Connect agent", el campo de alias viene precargado con tu hostname. Acéptalo o cámbialo (por ejemplo "yz-laptop"). Pulsa "Confirm".

El agente recibirá un JWT y continuará al instante. El token dura 90 días y se renueva automáticamente en cada conexión.

### Qué es un agente persistente

Cada agente está atado a un **host-id estable** en tu máquina:
- Linux — `/etc/machine-id`
- macOS — `IOPlatformUUID`
- Windows — `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid`
- Fallback — `~/.config/execai/host_id` (UUID generado en la primera instalación)

Esto significa: si reinstalas execai, borras `credentials.json` y vuelves a iniciar sesión, **el backend reutiliza la sesión existente**, y agent_id se mantiene igual. Sin entradas duplicadas en tu lista de dispositivos.

Puedes ver todos tus agentes y desvincular cualquiera en `chat.execai.ru/settings/agents` (en UserMenu abajo a la izquierda → "Agents", icono de terminal).

---

## Hablar con el agente

Describe tu tarea en lenguaje natural, sin más:

```
› check how much space /var/log takes on server dev01
```

El agente:
- averiguará qué necesitas,
- decidirá qué herramienta usar (Bash, Read, Grep, …),
- te mostrará el comando antes de ejecutarlo y pedirá confirmación si es peligroso,
- lo ejecutará, leerá el resultado y responderá.

### Confirmar comandos peligrosos

Las acciones de solo lectura (`ls`, `cat`, `git status`, `kubectl get`, `grep`) se ejecutan automáticamente. Los comandos que modifican el estado (`rm`, `>`, `chmod`, `kubectl delete`, `git push`) preguntan:

```
⚠  Confirm Bash execution
   kubectl delete pod foo-xxx
   
   [y] Once  [a] All Bash this session  [s] This command this session  [f] FOREVER  [n] Deny
```

| Tecla | Significado |
|---|---|
| **y** | Permitir UNA VEZ |
| **a** | Permitir TODAS las invocaciones de Bash hasta que execai reinicie |
| **s** | Permitir este comando exacto (con sus argumentos) hasta reiniciar |
| **f** | Permitir PARA SIEMPRE (persistido en `~/.config/execai/permissions.json`) |
| **n** o Esc | Denegar — el agente recibe "user refused" de vuelta |

Navegación: ← → o Tab para mover el foco, Enter para confirmar, o los atajos `y/a/s/f/n`.

### Herramientas integradas

| Herramienta | Qué hace |
|---|---|
| **Bash** | Ejecuta un comando de shell con salida en streaming (las líneas aparecen a medida que se producen) |
| **Read** | Lee un archivo con offset/limit, autodetecta binarios |
| **Write** | Crea/sobreescribe un archivo (con confirmación) |
| **Edit** | Reemplazo preciso de cadenas en un archivo (con confirmación) |
| **Grep** | Búsqueda con regexp por un árbol de archivos |
| **Glob** | Encuentra archivos por un patrón como `**/*.go` |
| **LS** | Listado de directorio |
| **Tree** | Árbol de archivos hasta profundidad N |
| **WebFetch** | Abre una página: HTML → texto legible + enlaces para seguir (sin JS) |
| **WebSearch** | Búsqueda web con respuesta y fuentes citadas (requiere cuenta ExecAI) |
| **AskUser** | Hace una pregunta con 2–4 opciones cuando la decisión es tuya |
| **Task** | Delega una investigación autónoma a un subagente de solo lectura |
| **TodoWrite** | Planificador de tareas interno del agente |
| **schedule_wakeup** | La IA programa su propio despertar en N segundos (ver [Autoloop](#loop-y-autoloop--repetir-y-esperar)) |

---

## Fuentes — ExecAI y suscripciones

El agente puede hablar con **9 fuentes distintas**. El historial de la conversación se comparte: cambia al vuelo sin perder el contexto.

| Fuente | Qué es | Facturación |
|---|---|---|
| **execai** (por defecto) | Nuestro gateway → cualquiera de ~34 modelos | Plan ExecAI |
| **zai** | Z.ai GLM Coding Plan directo | Tu suscripción de Z.ai |
| **zai-api** | Z.ai open platform, pago por tokens — **la misma clave que la suscripción**, y sin la lista cerrada de herramientas del Coding Plan | Pago por tokens Z.ai |
| **kimi** | Kimi Code Coding Plan (kimi.com/code) | Tu suscripción de Kimi Code |
| **kimi-api** | Moonshot Platform, pago por token | Pago por token de Moonshot |
| **anthropic** | API directa de Claude (sk-ant-...) | Pago por token de Anthropic |
| **openai** | API directa de OpenAI (sk-proj-...) | Pago por token de OpenAI |
| **claude-cli** | Delega en tu CLI local `claude` | Tu OAuth de Claude Pro/Max/Team |
| **codex-cli** | Delega en tu CLI local `codex` | Tu OAuth de ChatGPT Plus/Pro/Team |
| **ollama** | Cloud (ollama.com) o local (`ollama serve`) | Tu suscripción / 0 ₽ en local |

### 1. ExecAI — por defecto

Tras el login, el agente usa nuestra facturación. Sin configuración adicional.

```
src : ExecAI   provider : anthropic   model : claude-sonnet-4-6
```

### 2. Z.ai Coding Plan

Tu propia suscripción de $3-60/mes → llegas directo a GLM-5.2 y nosotros no te cobramos nada.

1. Consigue una clave de **Coding Plan** (¡no una API-key normal!):
   - https://z.ai/manage-apikey/apikey-list → **Individual Coding Plan > Plan Overview**
   - Team: https://z.ai/manage-apikey/coding-plan/team/my-plan

   > ⚠️ Una API-key normal se cobra por token, NO desde tu suscripción.

2. `/connect zai sk-zai-XXXXX`
3. `/source zai` → modelo primario **GLM-5.2**

### 3. Kimi Code Coding Plan

Un producto independiente de Moonshot AI: su equivalente a Claude Code con su propia suscripción. Su insignia **K3** está disponible en todos los planes salvo los más bajos.

1. Obtén una clave en https://www.kimi.com/code/console (sección de API keys).
2. ```
   /connect kimi sk-XXXXX
   /source kimi
   ```
3. Tu plan se autodetecta vía `/coding/v1/models`: la barra de estado mostrará `kimi (K3)`, `kimi (K3 + HighSpeed)` o `kimi (K2.7 Code)` según lo que esté realmente disponible.

Modelos: `k3` (primario), `kimi-for-coding` (K2.7 Code), `kimi-for-coding-highspeed`. `/usage` muestra tu **cuota real**: el límite semanal más ventanas rodantes (5h) con barras de progreso.

### 4. Moonshot Platform (API de pago por token)

Una API key normal para facturación por token (no una suscripción). Si quieres modelos de Moonshot fuera de Kimi Code:

1. Obtén una clave en https://platform.moonshot.ai/console/api-keys.
2. ```
   /connect kimi-api sk-XXXXX
   /source kimi-api
   ```

El catálogo de modelos se extrae dinámicamente del `/v1/models` de tu cuenta. Primario → `kimi-latest` (alias automático al buque insignia actual). La clave se valida en el momento del `/connect`: ante un 401/403 recibes un rechazo inmediato con una pista sobre qué clave va dónde.

### 5. API de Anthropic

Clave directa desde https://console.anthropic.com/settings/keys — pago por token.

```
/connect anthropic sk-ant-XXXXX
/source anthropic
```
Modelos: claude-sonnet-4-6, claude-opus-4-8, claude-haiku-4-5.

### 6. OpenAI Platform (API de pago por token)

Clave directa desde https://platform.openai.com/api-keys — pago por token.

```
/connect openai sk-proj-XXXXX
/source openai
```

El catálogo se extrae del `/v1/models` de tu cuenta. Primario → **gpt-5** → o3 → gpt-4.1 → gpt-4o (orden de prioridad).

### 7. Claude Code CLI (OAuth de Claude Pro/Max/Team)

Si ya tienes `claude` (Claude Code) instalado y una suscripción **Pro/Max/Team**, usa su cuota mediante la sesión OAuth local, sin necesidad de una clave aparte.

```
/connect claude-cli    # comprueba que `claude` esté en el PATH
/source claude-cli
```

Modelos (aliases): sonnet / opus / haiku. La selección del modelo también se gestiona externamente vía `claude config set defaultModel <id>`.

⚠️ **Salvedades:**
- Las herramientas de execai (Bash/Read/Write) NO funcionan a través de claude-cli: éste lanza sus propias herramientas con sus propios permisos
- El historial se pasa como un prompt de texto plano (sin session-id)

### 8. OpenAI Codex CLI (OAuth de ChatGPT Plus/Pro/Team)

El equivalente de `claude-cli` para OpenAI. Usa la cuota de tu suscripción de ChatGPT (Plus/Pro/Team/Enterprise) a través del binario local `codex`.

**Instalar codex** (Linux/macOS):
```bash
curl -fsSL https://chatgpt.com/codex/install.sh | sh
codex login    # OAuth en el navegador con tu cuenta de ChatGPT
```
Windows:
```powershell
powershell -ExecutionPolicy ByPass -c "irm https://chatgpt.com/codex/install.ps1 | iex"
```

**En execai:**
```
/connect codex-cli
/source codex-cli
```

Modelos: gpt-5, o3, o4-mini. Mismas salvedades que claude-cli: las herramientas de execai no funcionan, codex lanza las suyas.

### 9. Ollama — nube o local

Dos modos bajo el mismo comando:

**Nube (ollama.com):**
```
/connect ollama <api-key>          # clave en https://ollama.com/settings/keys
/source ollama
```
Modelos en sus servidores: **glm-5.2** (primario), qwen3-coder:480b, kimi-k2:1t, deepseek-v3.1:671b, gpt-oss:120b. Endpoint compatible con Anthropic, effort soportado.

**Local (tu propio Ollama):**
```
ollama pull llama3.2               # aparte, lo que quieras ejecutar
/connect ollama local              # localhost:11434
/connect ollama local http://192.168.1.10:11434   # tu propia URL
/source ollama
```
El catálogo se extrae dinámicamente vía `/api/tags`. 0 ₽. Endpoint compatible con OpenAI.

### Sobre el cambio de fuente

- **El historial se comparte.** `/source zai` → `/source execai` → `/source ollama`: el contexto se conserva.
- **La facturación queda aislada.** Con una suscripción externa, la facturación de ExecAI NO te cobra.
- **`/subscriptions`**: qué está conectado, `/disconnect <provider>`: eliminar.
- **`/source` sin argumento**: selector rápido.

---

## Modelos

`/model` sin argumento: lista de lo disponible. `/model <id>` o `/model <num>`: cambia.

El autocompletado es más cómodo:
```
/model<Tab>   →  menú de todos los modelos con filtro
/model deeps  →  filtra por subcadena
```

### Qué hay en el catálogo (por fuente)

| Fuente | Modelos |
|---|---|
| **execai** | ~34 modelos — Claude/GPT/DeepSeek/Kimi/MiniMax/Qwen/GLM a través de nuestro backend |
| **zai** | glm-5.2 (primario), glm-5.2[1m], glm-4.7 |
| **kimi** | k3 (primario), kimi-for-coding (K2.7), kimi-for-coding-highspeed |
| **kimi-api** | kimi-latest (primario), kimi-k2-turbo-preview, moonshot-v1-* (dinámicamente desde /v1/models) |
| **anthropic** | claude-sonnet-4-6, claude-opus-4-8, claude-haiku-4-5 |
| **openai** | gpt-5 (primario), o3, gpt-4.1, gpt-4o, o4-mini (dinámicamente desde /v1/models) |
| **claude-cli** | sonnet / opus / haiku (aliases + versiones fijadas) |
| **codex-cli** | gpt-5, o3, o4-mini |
| **ollama cloud** | glm-5.2, qwen3-coder:480b, kimi-k2:1t, deepseek-v3.1:671b, gpt-oss:120b |
| **ollama local** | lo que hayas instalado con `ollama pull` (dinámicamente desde /api/tags) |

El primario se cambia automáticamente al cambiar de fuente. Precios y descripciones viven en `/model`.

---

## Imágenes y archivos

### Imágenes

**Arrastra y suelta** un PNG/JPG/GIF/WEBP en la ventana de tu terminal, o **escribe la ruta entre comillas**:

```
› '/home/yz/Pictures/Screenshot.png' what's in this image?
```

El agente:
- encuentra la ruta en tu mensaje (los espacios y caracteres no ASCII en el nombre están bien, si el archivo va entre comillas),
- lo codifica en base64,
- lo envía como `image_url` a un modelo de visión,
- devuelve una descripción.

**Modelos de visión** (que pueden ver): claude-sonnet-4-6, gpt-5.5, glm-5v-turbo, glm-4.5v. Si tienes seleccionado un modelo sin visión, cambia con `/model`.

### Archivos (texto/código)

Basta con nombrar la ruta: el agente la leerá con la herramienta `Read`:

```
› take a look at internal/chat/tui.go and suggest a refactor
```

Los archivos grandes se leen por trozos. Si la IA se salta algo importante, pídelo con más precisión ("lee las líneas 100-200" o "encuentra la función X").

### Adjuntar un archivo desde el portapapeles

Las terminales no pueden pasar imágenes desde el portapapeles (Ctrl+V): es una limitación de todas las TUIs. El drag & drop de archivos (la terminal pega la ruta) sí funciona.

---

## Effort — nivel de razonamiento

Soportado en fuentes compatibles con Anthropic: **zai, kimi, anthropic, ollama-cloud, claude-cli**. El modelo "piensa" internamente primero y luego responde. En tareas difíciles da resultados dramáticamente mejores, pero es más lento y consume más cuota.

### Niveles

```
/effort                      # abre el selector
```

Seis posiciones: **off** (0) · **low** (1024) · **medium** (4096) · **high** (8192) · **xhigh** (16384) · **max** (32000) tokens.

Controles del selector: **← →** para elegir, **Enter** para aplicar, **Esc** para cancelar.

**Shift+Tab** rota entre niveles de effort sin abrir el selector (no funciona en algunas terminales: en ese caso usa `/effort`).

Persistido en `~/.config/execai/config.json`, visible en la barra de estado como `effort : high`.

---

## Loop y Autoloop — repetir y esperar

### `/loop` — repetidor simple

Lanza un prompt en un temporizador:

```
/loop 5m check the CI build status
```

Cada 5 minutos, el agente reenvía el prompt. Útil para hacer polling — "esperar hasta que ocurra algo".

```
/loop          # muestra el estado actual
/loop stop     # detiene
```

La barra de estado muestra `🔁 loop: 5m`. Funciona solo mientras la TUI está abierta.

### Autoloop — la IA decide cuándo despertar

Más inteligente: a la IA se le entrega una herramienta `schedule_wakeup` y ella misma decide cuándo volver a una tarea.

Ejemplo:
```
› run `npm install`, wait for it to finish, then check the logs for errors
```

La IA:
1. Ejecuta `npm install` (streaming al chat)
2. Ve que la instalación tardará ~5 minutos
3. Llama a `schedule_wakeup(delay_seconds=300, reason="waiting for npm install", prompt_on_wake="check logs and errors")`
4. Termina la respuesta actual
5. **5 minutos después la TUI despierta al agente** con el prompt "check logs and errors"
6. La IA sigue por donde iba — ve todo el historial de la conversación

En la UI se ve así:
```
🌙 autoloop: wake in 5m (waiting for npm install) → prompt: "check logs and errors"
... 5 minutes later ...
› 🌙 [autoloop] check logs and errors
● ...AI continues...
```

Si la IA decide que la tarea está hecha, simplemente no llama a `schedule_wakeup` y el autoloop se detiene por sí solo.

**Cuándo usar cuál:**
- `/loop`: intervalo fijo, ya sabes cuánto hay que esperar
- Autoloop: la IA lo averigua, tú solo describes la tarea

---

## Memoria del agente

El agente recuerda hechos entre sesiones, como Claude Code, pero más simple. Se carga automáticamente en el system prompt (solo el índice, no todo a la vez): el contexto se mantiene ligero.

### Dónde va cada cosa

**Memoria de usuario** (transversal a proyectos, hechos personales sobre ti):
```
~/.config/execai/memory/
├── MEMORY.md              ← índice (siempre en el system prompt)
├── user_role.md           ← 1 archivo por hecho
├── feedback_style.md
├── project_alpha.md
└── reference_jenkins.md
```

Cada archivo: frontmatter + un cuerpo corto:
```markdown
---
name: user-role
description: senior Go engineer, live in EU
metadata:
  type: user
---

- Write in Go, less often Python
- Prefer bubbletea for TUI
- Don't use sudo unless absolutely necessary
```

En `MEMORY.md`, una línea que referencia el archivo:
```
- [🎯 My stack](user_role.md) — Go, bubbletea, EU timezone
```

**Memoria de proyecto** (específica de este repo):
```
<CWD>/EXECAI.md            ← archivo único
```

### 4 tipos de registro

| Tipo | Qué | Ejemplo |
|---|---|---|
| **user** | Rol, habilidades, preferencias | "Escribo en Go, tech lead" |
| **feedback** | Cómo quieres que se comporte | "No hagas resúmenes largos. Motivo: no los leo" |
| **project** | Iniciativas, plazos, decisiones | "R5 — nueva rama para suscripciones, deadline Q3" |
| **reference** | Punteros a sistemas externos | "Jenkins: jenkins.mycompany.com, dashboards en Grafana" |

### Cómo usarla

Basta con decirle al chat qué recordar:
```
› remember that I write in Go and prefer bubbletea
```
El modelo creará `user_role.md` (o actualizará el existente) + añadirá una línea a `MEMORY.md`.

```
› project R5 — new branch for subscriptions/GUI, don't merge into R4
```
Crea `project_r5.md` en la memoria de usuario. Si el hecho es sobre ESTE repo, actualiza `EXECAI.md` en CWD.

El modelo NO ensucia la memoria con hechos episódicos ("hoy arreglé un bug" — eso pertenece al git log). Solo anota lo duradero.

Funciona en **todas las fuentes** — el system prompt es el mismo.

---

## Proyectos y modo en segundo plano — web → agente

*Desde v6.17. La parte web requiere la herramienta «Agentes» en el catálogo del proyecto en execai.ru — se está activando gradualmente.*

Tu máquina puede ser **una herramienta dentro de un proyecto del chat web**: vincula un directorio al proyecto una vez, arranca el escucha en segundo plano — y pide al modelo en el navegador que haga cosas en tu máquina. El modelo llama al agente como a cualquier otra herramienta del proyecto (ssh, git), la tarea se ejecuta **en el directorio del proyecto** y la respuesta vuelve al chat.

### Vincular un directorio

```
/project              # tus proyectos; ● — vinculado a este directorio
/project bind <nombre>  # vincular el directorio actual + añadir esta máquina al proyecto
/project on|off       # el mismo interruptor que en la tarjeta del proyecto web
/project unbind       # quitar la vinculación y la máquina del proyecto
```

La vinculación hace dos cosas: recuerda «este directorio en esta máquina = aquel proyecto» y añade al proyecto un registro de herramienta normal — como un perfil ssh — así la máquina aparece en la tarjeta del proyecto con su interruptor. La interfaz muestra alias; el id estable de la máquina sobrevive a un nuevo login.

### `execai serve` — el escucha en segundo plano

```
execai serve
```

Escucha tareas del chat web y las ejecuta. Lo importante:

- La tarea se ejecuta **en el directorio vinculado a su proyecto**, no donde se lanzó `serve`. ¿Directorio desaparecido? → error claro, no ejecución en el lugar equivocado.
- **Lo permitido = tu `permissions.json`** (lo que aprobaste con «Siempre» en la TUI). Lo que no está en la lista se rechaza — no hay nadie cerca para confirmar. Archivo vacío → todo permitido, con un aviso bien visible al arrancar.
- `--read-only` — modo solo lectura: sin cambios de archivos ni comandos.
- Cada llamada de herramienta va al **registro de auditoría** `~/.config/execai/serve-audit.log` (rota a 8 MB) — se ve qué hizo el agente por la noche.
- Un demonio por máquina (pid-lock). `execai serve --status` muestra pid/tiempo/endpoint; `--stop` para suavemente (deja terminar la tarea actual); `--stop --force` mata tras 5 s — el resultado de la tarea actual se pierde y el chat verá un timeout.
- Agente offline → el chat recibe un honesto «el agente no responde» en ~12 s; la tarea queda en cola y se ejecuta en cuanto arranque `serve`. La entrega se confirma (ack): una tarea ofrecida a una conexión muerta no se pierde.
- Cerrar la terminal mata el proceso. Para que sobreviva:
  `setsid nohup execai serve > ~/.execai-serve.log 2>&1 &`

En este modo AskUser está desactivado (no hay quien responda) y el límite de iteraciones es menor (30 por tarea).

---

## Gasto / `/usage`

```
/usage
```

Muestra cosas distintas según el `/source` activo:

**ExecAI (por defecto):** tu plan + saldo + 4 ventanas de rate-limit (5h/día/semana/mes) con barras de progreso + las últimas 14 iteraciones (modelo, tokens, precio en ₽).

**Kimi Code (`/source kimi`):** cuota real desde `api.kimi.com/coding/v1/usages` — plan por modelos disponibles, límite semanal con hora de reset, ventanas rodantes (5h, etc.).

**Moonshot Platform (`/source kimi-api`), OpenAI (`/source openai`), Anthropic, Z.ai:** contadores locales de tokens + un enlace al dashboard del proveedor (la facturación real vive allí).

---

## Comandos y atajos

### Slash commands

| Comando | Qué hace |
|---|---|
| `/help` | Todos los comandos |
| `/model [id]` | Cambia el modelo |
| `/source [name]` | Cambia la fuente (execai/zai/kimi/kimi-api/anthropic/openai/claude-cli/codex-cli/ollama) |
| `/connect <provider> [args...]` | Conecta una suscripción. Sin args — ayuda |
| `/disconnect <provider>` | Desconecta una suscripción |
| `/subscriptions` (o `/subs`) | Lista de conexiones |
| `/usage` | Plan + gasto (con especificidades por fuente) |
| `/effort` | Selector de nivel de razonamiento (para fuentes compatibles con Anthropic) |
| `/max-iterations [N]` | Límite de iteraciones de tool-use por turno. Sin arg muestra el actual (por defecto 40). Al agotarse — parada suave |
| `/paste` | Lista de pegados (Ctrl+V de trozos grandes) — [Pasted #N — L lines, C chars] |
| `/paste show <N>` | Contenido del pegado #N |
| `/project [bind <nombre>\|on\|off\|unbind]` | Vincular el directorio a un proyecto del chat web; on/off — interruptor del proyecto (desde v6.17) |
| `/loop <interval> <prompt>` | Loop con temporizador fijo |
| `/loop stop` | Detener |
| `/log` | Últimas 20 peticiones LLM (para ver qué modelo respondió realmente) |
| `/new` | Nueva conversación (la actual se guarda) |
| `/sessions` | Lista de conversaciones |
| `/resume <num\|id>` | Abre una conversación guardada |
| `/compact` | Compacta el historial en un resumen vía LLM (para conversaciones largas) |
| `/cd <path>` | Cambia el directorio de trabajo |
| `/clear` | Limpia el historial (Ctrl+L) |
| `/whoami` | Quién ha iniciado sesión |
| `/config` | Muestra la config |
| `/permissions` | Permisos persistentes de herramientas |
| `/classic on\|off` | Alterna la TUI clásica (alt-screen+mouse) en lugar de Ink-style. Requiere reiniciar |
| `/mouse on\|off` | Captura del ratón (solo relevante en la TUI clásica) |
| `/login` | Volver a iniciar sesión |
| `/logout` | Cerrar sesión |
| `/quit` | Salir (Ctrl+D) |

> Tip: escribe `/` y el menú de autocompletado muestra todos los comandos. Para los comandos **sin argumentos**, elegir del menú ejecuta al momento (un solo Enter). Los comandos que toman un argumento separado por espacio (`/model `, `/source `, `/cd `) esperan la entrada.

### Atajos

| Tecla | Acción |
|---|---|
| **Enter** | Enviar |
| **Shift+Enter** | Nueva línea (multilínea) |
| **↑ / ↓** | Historial de tus entradas previas (como bash) |
| **Tab** | Aceptar una sugerencia de autocompletado |
| **Shift+Tab** | Rotar el nivel de effort (o abrir el selector) |
| **PgUp / PgDn** | Desplazar el chat |
| **Ctrl+R** | Búsqueda fuzzy por las sesiones |
| **Ctrl+C** | Cancelar el stream (o dos veces para salir) |
| **Ctrl+D** | Salir (dos veces para confirmar) |
| **Ctrl+L** | Limpiar historial |
| **Shift+drag** | Seleccionar texto (si el ratón está capturado — `/mouse off` para seleccionar normalmente) |
| **Rueda del ratón** | Desplazar la viewport |

---

## Sesiones e historial

Cada conversación se autoguarda en `~/.config/execai/sessions/<uuid>.json` tras cada intercambio. Al reiniciar:
- Si la misma carpeta tenía una conversación de menos de 24h, se reanuda
- Si no, arranca una nueva

`/sessions`: lista de todas las conversaciones. `/resume 3`: abre la tercera. `/resume <id>`: por ID.

`Ctrl+R`: búsqueda fuzzy — escribe cualquier palabra y busca tanto en títulos COMO en el contenido de cada conversación.

`/compact`: si una conversación se alarga demasiado y el contexto desborda, el agente comprime la parte más antigua en un único resumen manteniendo los últimos 6 turnos.

El historial de entrada (`↑/↓`) también se guarda en `~/.config/execai/input_history`: sobrevive a los reinicios.

---

## Dónde viven los archivos

| SO | Ruta |
|---|---|
| Linux   | `~/.config/execai/` |
| macOS   | `~/Library/Application Support/execai/` |
| Windows | `%APPDATA%\execai\` |

| Archivo | Qué |
|---|---|
| `config.json` | api_base, selected_model_id, thinking_budget (effort) |
| `credentials.json` | JWT (modo 0600) |
| `host_id` | machine-id estable (solo si /etc/machine-id, etc. no están disponibles — fallback) |
| `subscriptions.json` | Suscripciones conectadas (claves en texto plano — guarda con cuidado) |
| `permissions.json` | Allow-list persistente de herramientas |
| `sessions/<uuid>.json` | Historial de cada conversación |
| `memory/MEMORY.md` | Índice de la memoria de usuario (ver [Memoria](#memoria-del-agente)) |
| `memory/*.md` | Hechos individuales (user/feedback/project/reference) |
| `input_history` | Últimas 200 entradas tuyas |
| `requests.log` | Log de peticiones LLM (para `/log`) |
| `auth-poll.log` | Diagnósticos del login device-flow (para depurar) |
| `models_cache.json` | Caché del catálogo de modelos — se usa cuando la red está caída para que execai siga arrancando offline |
| `installed_arch_sha` | SHA de arquitectura para la comprobación de auto-update |
| `last_remote_sha` | Sello para comparación de versiones |

### Memoria — ver sección dedicada

[Memoria del agente](#memoria-del-agente) — estructura usuario + proyecto, autocargada en el system prompt en cada sesión.

---

## Solución de problemas

**`execai: command not found`** tras instalar:
- En la sesión actual: `exec bash -l` o abre una nueva terminal
- O manualmente: `export PATH="$HOME/.local/bin:$PATH"`

**"zai subscription is not connected"** al hacer `/source zai`:
- Primero `/connect zai <key>`. De dónde sacar la clave: ver la sección de [Fuentes](#fuentes--execai-y-suscripciones)

**429 "Insufficient balance"** en Z.ai:
- Estás usando una API-key normal, no una de Coding Plan. Consigue una de Coding Plan como es debido: https://z.ai/manage-apikey/apikey-list (sección Individual Coding Plan)

**404 "Not found"** durante el chat (ExecAI):
- Puede que el gateway no conozca el endpoint. Repórtalo a los desarrolladores: puede que haya que redesplegar aicore-vbai.

**La TUI no abre, pantalla en blanco**:
- En Windows usa **Windows Terminal**, no el viejo conhost. En Linux, cualquier moderna (gnome-terminal, kitty, alacritty).
- Defender: `Add-MpPreference -ExclusionPath "$env:LOCALAPPDATA\execai"` y reinstala.

**Se rompe la codificación** (`Ð�escribe` en lugar de `Describe`):
- Bug de una versión antigua de bubbles. Actualiza execai: `curl -fsSL .../install.sh | bash`.

**El auto-update nunca llega**:
- Al arrancar, la barra de estado abajo muestra `✓ execai R5.X — latest version` o `🔔 New version available`.
- Si no funciona — `EXECAI_UPDATE_CHANNEL=R5` fuerza el canal.

**Las imágenes no se envían** aunque la ruta esté ahí:
- Comprueba que la ruta va entre comillas (simples o dobles) si contiene espacios o no-ASCII: `'/path/to/Screenshot.png'`
- O sin comillas si la ruta no tiene espacios: `/path/to/foo.png`
- Si la IA sigue diciendo "no image", revisa el modelo — tiene que ser uno de visión (sonnet/gpt-5/glm-5v, etc.)

**Petición colgada**:
- `Ctrl+C` cancela la petición actual. El historial se conserva.

**"Reached N iterations limit"** en el chat:
- NO es un error: es una parada suave tras el límite de iteraciones de tool-use (por defecto 40).
- Basta con decir "continue" — el agente tomará otro lote del mismo tamaño y retomará donde lo dejó.
- Si la tarea es autónoma y grande, sube el límite: `/max-iterations 100` (rango 1-500).
- Se guarda en `~/.config/execai/config.json` y se aplica a los turnos siguientes.

**"context deadline exceeded" / los modelos no cargan**:
- Desde R5.67+ execai SIEMPRE arranca, incluso sin red.
- En la primera ejecución trae el catálogo desde la API y lo cachea en `~/.config/execai/models_cache.json`.
- Cuando la red se cae, usa la caché y muestra `ℹ Using cached catalog`.
- Si tampoco hay caché, entra un fallback integrado (Claude Sonnet 4.6), la TUI se abre, las peticiones reales pueden fallar pero la interfaz está viva.

---

## Soporte

- Bugs / peticiones de features: [github.com/execai/execai-agent/issues](https://github.com/execai/execai-agent/issues)
- Docs de ExecAI: https://chat.execai.ru/

---

**execai** — por ExecAI/VBAI. Licencia estilo MIT.
