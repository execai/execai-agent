#!/usr/bin/env bash
# Smoke-test для всего execai-стека на dev.
# Запускается без аргументов, не требует TTY, прогоняет все слои:
#   1. Infra        — kubectl deployments healthy + актуальные R4-теги
#   2. HTTP smoke   — public endpoints без auth (frontend, models_public, agent-link/start)
#   3. JWT auth    — генерим test-JWT через kubectl-secret, дёргаем /current-user
#   4. Device-flow — start → info (без подтверждения, нужен браузер)
#   5. CLI binary  — скачиваем последний R4 из prod-бакета, проверяем models fetch
#   6. AICore       — non-streaming тест /agent-stream alias с test-JWT
#   7. UI smoke     — curl frontend routes (полная Playwright-проверка отдельно)
#
# Использование:
#   bash scripts/smoke-test.sh            # все фазы
#   PHASE=2 bash scripts/smoke-test.sh    # только фаза 2
#   API_BASE=https://api.execai.ru bash scripts/smoke-test.sh   # против prod
set -uo pipefail

API_BASE="${API_BASE:-https://api.execai.ru}"
FRONT_BASE="${FRONT_BASE:-https://chat.execai.ru}"
TEST_EMAIL="${TEST_EMAIL:-alort5@yandex.com}"  # реальный юзер на dev
KUBECTX="${KUBECTX:-config-dev}"
PHASE="${PHASE:-all}"

C_RESET=$'\033[0m'; C_R=$'\033[31m'; C_G=$'\033[32m'; C_Y=$'\033[33m'; C_B=$'\033[34m'; C_DIM=$'\033[2m'

PASS=0; FAIL=0; SKIP=0
phase() { echo; echo "${C_B}=== Phase $1: $2 ===${C_RESET}"; }
ok()   { echo "  ${C_G}✓${C_RESET} $1"; PASS=$((PASS+1)); }
bad()  { echo "  ${C_R}✗${C_RESET} $1"; FAIL=$((FAIL+1)); }
warn() { echo "  ${C_Y}!${C_RESET} $1"; }
skip() { echo "  ${C_DIM}—${C_RESET} skip: $1"; SKIP=$((SKIP+1)); }

run_phase() { [ "$PHASE" = "all" ] || [ "$PHASE" = "$1" ]; }

# ============================================================
# Phase 1: Infra health
# ============================================================
if run_phase 1; then
    phase 1 "Infra health (k8s deployments)"
    KCFG="$HOME/.kube/$KUBECTX"
    if [ ! -f "$KCFG" ]; then
        skip "$KCFG нет — пропускаем (надо для kubectl)"
    else
        for svc in auth-vbai aicore-vbai billing-vbai-svc execaiui-vbai api-vbai-svc aichat-vbai; do
            img=$(KUBECONFIG="$KCFG" kubectl get deployment "$svc" -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || echo "")
            ready=$(KUBECONFIG="$KCFG" kubectl get deployment "$svc" -o jsonpath='{.status.readyReplicas}/{.status.replicas}' 2>/dev/null)
            if [ -z "$img" ]; then bad "$svc: deployment не найден"; continue; fi
            tag="${img##*:}"
            case "$tag" in
                R4.*) ok "$svc: $tag (ready $ready)" ;;
                R3.*) warn "$svc: $tag — НЕ R4 (ready $ready)" ;;
                *) warn "$svc: $tag (ready $ready)" ;;
            esac
        done
    fi
fi

# ============================================================
# Phase 2: HTTP public endpoints
# ============================================================
if run_phase 2; then
    phase 2 "HTTP smoke (без auth)"

    code=$(curl -s -o /tmp/st_models.json -w "%{http_code}" "$API_BASE/billing-vbai/models_public")
    if [ "$code" = "200" ]; then
        n=$(python3 -c "import json; print(len(json.load(open('/tmp/st_models.json'))))" 2>/dev/null || echo 0)
        [ "$n" -gt 0 ] && ok "/billing-vbai/models_public → 200, $n моделей" || bad "/billing-vbai/models_public пустой массив"
    else
        bad "/billing-vbai/models_public → HTTP $code"
    fi

    code=$(curl -s -o /tmp/st_start.json -w "%{http_code}" -X POST "$API_BASE/auth-vbai/agent-link/start" \
        -H "Content-Type: application/json" -d '{"agent_type":"smoke-test","hostname":"runner","os_info":"linux/amd64"}')
    if [ "$code" = "200" ]; then
        uc=$(python3 -c "import json; d=json.load(open('/tmp/st_start.json')); print(d.get('user_code',''))")
        vu=$(python3 -c "import json; d=json.load(open('/tmp/st_start.json')); print(d.get('verify_uri',''))")
        [ -n "$uc" ] && ok "/agent-link/start → user_code=$uc verify=$vu" || bad "agent-link/start без user_code"
    else
        bad "/agent-link/start → HTTP $code"
    fi

    # alias /agent-stream должен существовать (401, НЕ 404)
    code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$API_BASE/aicore-vbai/agent-stream" \
        -H "Content-Type: application/json" -H "Authorization: Bearer fake" -d '{"model":"x","messages":[]}')
    case "$code" in
        401) ok "/aicore-vbai/agent-stream alias жив (401)" ;;
        404) bad "/aicore-vbai/agent-stream → 404 (alias отсутствует!)" ;;
        *)   warn "/aicore-vbai/agent-stream → HTTP $code (ожидался 401)" ;;
    esac

    code=$(curl -s -o /dev/null -w "%{http_code}" "$FRONT_BASE/auth/login")
    [ "$code" = "200" ] && ok "frontend /auth/login → 200" || bad "frontend /auth/login → $code"

    code=$(curl -s -o /dev/null -w "%{http_code}" "$FRONT_BASE/agents/connect/TEST1234")
    [ "$code" = "200" ] && ok "frontend /agents/connect/X → 200 (SPA)" || bad "frontend /agents/connect/X → $code"
fi

# ============================================================
# Phase 3: JWT auth (генерим из k8s-секрета, бьём /current-user)
# ============================================================
TEST_JWT=""
if run_phase 3; then
    phase 3 "JWT auth (test-JWT из k8s-secret → /current-user)"
    KCFG="$HOME/.kube/$KUBECTX"
    if [ ! -f "$KCFG" ]; then
        skip "$KCFG нет — без секрета не сгенерим JWT"
    else
        SECRET=$(KUBECONFIG="$KCFG" kubectl get secret auth-vbai-secret -o jsonpath='{.data.JWT_SECRET}' 2>/dev/null | base64 -d 2>/dev/null)
        if [ -z "$SECRET" ]; then
            skip "JWT_SECRET не достали из auth-vbai-secret"
        else
            exp=$(( $(date +%s) + 3600 ))
            header='{"alg":"HS256","typ":"JWT"}'
            payload="{\"authorized\":true,\"user_email\":\"$TEST_EMAIL\",\"session_id\":\"smoke-test\",\"exp\":$exp}"
            hb=$(printf '%s' "$header"  | openssl base64 -A | tr '/+' '_-' | tr -d '=')
            pb=$(printf '%s' "$payload" | openssl base64 -A | tr '/+' '_-' | tr -d '=')
            sg=$(printf '%s' "$hb.$pb"  | openssl dgst -sha256 -hmac "$SECRET" -binary | openssl base64 -A | tr '/+' '_-' | tr -d '=')
            TEST_JWT="$hb.$pb.$sg"

            code=$(curl -s -o /tmp/st_user.json -w "%{http_code}" "$API_BASE/auth-vbai/current-user" -H "Authorization: Bearer $TEST_JWT")
            if [ "$code" = "200" ]; then
                eml=$(python3 -c "import json; print(json.load(open('/tmp/st_user.json')).get('email',''))")
                pc=$(python3 -c "import json; print(json.load(open('/tmp/st_user.json')).get('is_profile_complete'))")
                ok "/current-user → 200 (email=$eml profile_complete=$pc)"
            else
                bad "/current-user → HTTP $code  body=$(cat /tmp/st_user.json | head -c 200)"
            fi
        fi
    fi
fi

# ============================================================
# Phase 4: Device-flow round-trip (start → info)
# ============================================================
if run_phase 4; then
    phase 4 "Device-flow (start → info)"
    if [ -s /tmp/st_start.json ]; then
        UC=$(python3 -c "import json; print(json.load(open('/tmp/st_start.json'))['user_code'])")
        code=$(curl -s -o /tmp/st_info.json -w "%{http_code}" "$API_BASE/auth-vbai/agent-link/info?user_code=$UC")
        if [ "$code" = "200" ]; then
            st=$(python3 -c "import json; print(json.load(open('/tmp/st_info.json'))['status'])")
            ok "/agent-link/info?user_code=$UC → status=$st"
            warn "  (для linked-проверки нужен браузер с реальным OAuth — отдельно)"
        else
            bad "/agent-link/info → HTTP $code"
        fi
    else
        skip "Phase 2 не дала user_code"
    fi
fi

# ============================================================
# Phase 5: CLI binary (download + version + non-TTY models fetch)
# ============================================================
if run_phase 5; then
    phase 5 "CLI binary (download R4/latest + проверка моделей)"
    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT
    curl -fsSL "https://storage.yandexcloud.net/execai-agent-prod/execai/R4/latest/execai-linux-amd64.tar.gz" -o "$TMPDIR/x.tar.gz" \
        && tar -xzf "$TMPDIR/x.tar.gz" -C "$TMPDIR" \
        && mv "$TMPDIR/execai-linux-amd64" "$TMPDIR/execai" \
        && chmod +x "$TMPDIR/execai" \
        && ok "Скачан и распакован: $($TMPDIR/execai version 2>&1 | head -1)" \
        || { bad "Не скачался R4/latest"; }

    if [ -x "$TMPDIR/execai" ] && [ -n "$TEST_JWT" ]; then
        # Подсовываем test-creds в свой XDG_CONFIG_HOME — execai будет работать как залогиненный.
        export XDG_CONFIG_HOME="$TMPDIR/cfg"
        mkdir -p "$XDG_CONFIG_HOME/execai"
        cat > "$XDG_CONFIG_HOME/execai/config.json" <<EOF
{ "api_base": "$API_BASE" }
EOF
        cat > "$XDG_CONFIG_HOME/execai/credentials.json" <<EOF
{ "token": "$TEST_JWT", "email": "$TEST_EMAIL", "saved_at": "$(date -u +%FT%TZ)" }
EOF
        # whoami — простейшая проверка что creds читаются + /verify работает
        out=$("$TMPDIR/execai" whoami 2>&1)
        echo "$out" | grep -q "$TEST_EMAIL" && ok "execai whoami: $out" || bad "execai whoami: $out"
    else
        skip "TEST_JWT нет → models/whoami fetch не проверим"
    fi
fi

# ============================================================
# Phase 6: AICore agent-stream (минимальный round-trip)
# ============================================================
if run_phase 6; then
    phase 6 "AICore /agent-stream (минимальный non-streaming запрос)"
    if [ -z "$TEST_JWT" ]; then
        skip "Нет TEST_JWT (Phase 3 не отработала)"
    else
        # Минимальный запрос — посмотреть что сервер не падает с 5xx и принимает body.
        # Не ждём успешного ответа модели (модель может быть платной, баланс пуст).
        # aicore требует system: List[SystemConfig] (model+provider обязательны).
        body='{"messages":[{"role":"user","content":"ping"}],"tools":[],"system":[{"model":"qwen-max","provider":"alibaba","stream":true}],"stream":true}'
        code=$(curl -s -o /tmp/st_aicore.txt -w "%{http_code}" -X POST "$API_BASE/aicore-vbai/agent-stream" \
            -H "Content-Type: application/json" -H "Authorization: Bearer $TEST_JWT" -d "$body" --max-time 20)
        body_preview=$(head -c 200 /tmp/st_aicore.txt | tr -d '\n' | head -c 200)
        case "$code" in
            200) ok "/agent-stream → 200 (SSE ok, preview: ${body_preview:0:80})" ;;
            402) warn "/agent-stream → 402 insufficient funds (баланс пуст, but pipeline жив)" ;;
            400) warn "/agent-stream → 400 (формат запроса): $body_preview" ;;
            *)   bad "/agent-stream → HTTP $code, body=$body_preview" ;;
        esac
    fi
fi

# ============================================================
# Phase 7: UI smoke (curl-only — без playwright)
# ============================================================
if run_phase 7; then
    phase 7 "UI routes (curl-only smoke; для интерактива — playwright)"
    for path in / /auth/login /chat /agents/connect/X /sessions /billing /support; do
        code=$(curl -s -o /dev/null -w "%{http_code}" "$FRONT_BASE$path")
        [ "$code" = "200" ] && ok "$path → 200" || warn "$path → $code"
    done
fi

# ============================================================
# Summary
# ============================================================
echo
echo "${C_B}=== Summary ===${C_RESET}"
echo "  PASS: ${C_G}$PASS${C_RESET}   FAIL: ${C_R}$FAIL${C_RESET}   SKIP: ${C_DIM}$SKIP${C_RESET}"
[ $FAIL -eq 0 ] && exit 0 || exit 1
