#!/usr/bin/env python3
"""Фоновый режим целиком: задача из веба → ход → вопрос в чат → результат.

Самый опасный режим и самый непроверенный: рядом с агентом нет человека,
задача приходит извне, а неотвеченный вопрос через полторы минуты становится
отказом. Юниты покрывали политику и веб-согласователь по отдельности, но не
путь целиком — а ломается обычно именно стык.

Здесь поддельны обе стороны: платформа (инбокс, канал вопросов, приём
результата) и провайдер модели. Поэтому прогон бесплатный, без сети и без
входа, а сценарий задаётся точно.
"""
import json
import os
import re
import subprocess
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

BIN = os.environ.get("EXECAI_BIN", "./execai")
WS = os.environ.get("WORKDIR", "/tmp/execai-serve-ws")
CONF = os.environ["XDG_CONFIG_HOME"]

checks = []


def check(ok, name, detail=""):
    checks.append((bool(ok), name, detail))
    print(("  ok   " if ok else "  ПРОВАЛ ") + name + (f" — {detail}" if detail else ""))


# ── Поддельная платформа ──────────────────────────────────────────────────
state = {
    "tasks": [],        # что отдать агенту следующим опросом
    "acked": [],        # какие задачи он подтвердил
    "questions": [],    # что он спросил у «человека в вебе»
    "answer": "deny",   # что человек отвечает
    "results": [],      # что агент прислал в итоге
    "paths": [],        # какие адреса он реально дёргал — первое, что нужно
                        # знать, если «ничего не произошло»
}
lock = threading.Lock()


class Platform(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *a):
        pass

    def _send(self, obj, code=200):
        body = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _read(self):
        n = int(self.headers.get("Content-Length", 0))
        raw = self.rfile.read(n) if n else b"{}"
        try:
            return json.loads(raw or b"{}")
        except json.JSONDecodeError:
            return {}

    def _poll(self):
        """Длинный опрос инбокса: отдаём задачу, как только она появилась."""
        for _ in range(50):
            with lock:
                if state["tasks"]:
                    self._send({"tasks": [state["tasks"].pop(0)]})
                    return
            time.sleep(0.2)
        self._send({"tasks": []})

    def do_GET(self):
        with lock:
            state["paths"].append("GET " + self.path)
        if self.path.startswith("/agents-vbai/inbox/poll"):
            self._poll()
            return
        if self.path.startswith("/agents-vbai/workspaces/bindings"):
            self._send({"bindings": [{"workspace_id": "ws-1", "local_path": WS}]})
            return
        if self.path.startswith("/billing-vbai/models_public"):
            # Каталог моделей платформы. Без него ход обрывается ещё до
            # обращения к провайдеру — и это выглядит как «задача выполнена»
            # с ошибкой внутри, а не как поломка.
            # Именно МАССИВ: обёртка {"models": [...]} отвергается разбором.
            self._send([{
                "id": "test-model", "provider": "openai", "display_name": "Тестовая модель",
                "description": "для самопрогона", "model_tier": "standard",
                "is_primary": True, "supports_tools": True, "show_in_picker": True,
            }])
            return
        self._send({}, 404)

    def do_POST(self):
        body = self._read()
        with lock:
            state["paths"].append("POST " + self.path)
        # Опрос инбокса приходит методом POST — это выяснилось только живым
        # прогоном: поддельная платформа отвечала на GET, и «ничего не
        # происходило» без единой ошибки в логе.
        if self.path.startswith("/agents-vbai/inbox/poll"):
            self._poll()
            return
        if self.path.startswith("/agents-vbai/workspaces/bindings"):
            self._send({"bindings": [{"workspace_id": "ws-1", "local_path": WS}]})
            return
        if self.path.endswith("/inbox/ack"):
            # Ответ обязан перечислить подтверждённые id: агент берёт в работу
            # ТОЛЬКО их. Пока платформа отвечала «ок», задача молча терялась —
            # ни ошибки, ни хода, ни следа в логе.
            ids = body.get("task_ids") or []
            with lock:
                state["acked"].append(body)
            self._send({"acked": ids})
            return
        if self.path.endswith("/ask"):
            # Вопрос человеку. Отвечаем сразу — сценарий задаёт, что именно.
            with lock:
                state["questions"].append(body)
                answer = state["answer"]
            self._send({"answer": answer})
            return
        if re.search(r"/tasks/[^/]+/result$", self.path):
            with lock:
                state["results"].append(body)
            self._send({"ok": True})
            return
        self._send({"ok": True})


# ── Поддельный провайдер ──────────────────────────────────────────────────
script = []


class Provider(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *a):
        pass

    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        self.rfile.read(n)
        with lock:
            step = script.pop(0) if script else {"text": "готово"}
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Transfer-Encoding", "chunked")
        self.end_headers()

        def sse(obj):
            data = b"data: " + json.dumps(obj).encode() + b"\n\n"
            self.wfile.write(b"%x\r\n" % len(data) + data + b"\r\n")
            self.wfile.flush()

        if "tool" in step:
            sse({"choices": [{"delta": {"tool_calls": [{
                "index": 0, "id": "c1", "type": "function",
                "function": {"name": step["tool"], "arguments": json.dumps(step["args"])},
            }]}}]})
            sse({"choices": [{"delta": {}, "finish_reason": "tool_calls"}]})
        else:
            sse({"choices": [{"delta": {"content": step["text"]}}]})
            sse({"choices": [{"delta": {}, "finish_reason": "stop"}]})
        done = b"data: [DONE]\n\n"
        self.wfile.write(b"%x\r\n" % len(done) + done + b"\r\n")
        self.wfile.write(b"0\r\n\r\n")
        self.wfile.flush()


def serve_http(handler):
    srv = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return srv, srv.server_address[1]


def setup(api_port, llm_port):
    os.makedirs(f"{CONF}/execai", exist_ok=True)
    json.dump({"api_base": f"http://127.0.0.1:{api_port}", "security_level": "deep"},
              open(f"{CONF}/execai/config.json", "w"))
    json.dump({"token": "тест-токен", "email": "selftest@example.com",
               "agent_id": "ag-1", "alias": "selftest", "agent_type": "execai-cli"},
              open(f"{CONF}/execai/credentials.json", "w"))
    json.dump({"subscriptions": {"openai": {
        "provider": "openai", "api_key": "test",
        "base_url": f"http://127.0.0.1:{llm_port}/v1",
        "available_models": ["test-model"], "plan": "selftest"}},
        "active": "openai"}, open(f"{CONF}/execai/subscriptions.json", "w"))
    # Список НЕ пустой намеренно: пустой в фоне означает «разрешено всё», и
    # тогда вопросов не будет вовсе — проверять было бы нечего. Разрешаем
    # только чтение, чтобы Bash обязательно спросил.
    json.dump({"always_allowed_tools": ["Read"], "always_allowed_exact": None},
              open(f"{CONF}/execai/permissions.json", "w"))


def wait_for(cond, timeout=45):
    end = time.time() + timeout
    while time.time() < end:
        with lock:
            if cond():
                return True
        time.sleep(0.2)
    return False


def main():
    os.makedirs(WS, exist_ok=True)
    open(f"{WS}/main.go", "w").write("package main\n")
    api, api_port = serve_http(Platform)
    llm, llm_port = serve_http(Provider)
    setup(api_port, llm_port)

    p = subprocess.Popen([BIN, "serve"], stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                         text=True, bufsize=1, cwd=WS)
    out = []
    threading.Thread(target=lambda: [out.append(l.rstrip()) for l in p.stdout], daemon=True).start()

    started = wait_for(lambda: any("permissions.json" in l or "задач" in l.lower() or
                                   "жду" in l.lower() for l in out), 25)
    check(started, "serve поднялся и ждёт задачи", "; ".join(out[:3])[:120])
    if not started:
        p.terminate()
        return report()

    # Пустой permissions.json = «разрешено всё» — про это обязано быть громко
    # сказано: молчаливый режим без подтверждений опаснее всего.
    check(any("permissions.json" in l for l in out),
          "про режим разрешений сказано вслух", next((l for l in out if "permissions" in l), "")[:100])

    # 1. Задача, требующая подтверждения. Человек в вебе отвечает отказом.
    with lock:
        state["answer"] = "deny"
        script[:] = [{"tool": "Bash", "args": {"command": "rm -rf /tmp/execai-serve-victim"}},
                     {"text": "не стал удалять"}]
        state["tasks"].append({"id": "t-1", "workspace_id": "ws-1", "payload": "удали файл"})
    open("/tmp/execai-serve-victim", "w").write("цел")

    asked = wait_for(lambda: state["questions"], 45)
    check(asked, "вопрос ушёл в веб-чат",
          "" if asked else "агент дёргал: " + "; ".join(dict.fromkeys(state["paths"]))[:220])
    if asked:
        q = state["questions"][0]
        check("rm" in json.dumps(q, ensure_ascii=False), "в вопросе видно саму команду",
              json.dumps(q, ensure_ascii=False)[:90])

    got = wait_for(lambda: state["results"], 45)
    check(got, "результат задачи отправлен обратно",
          "" if got else "вывод: " + " | ".join(out[-6:])[:200])
    if got:
        # Задача, завершившаяся ОШИБКОЙ, — тоже «результат». Без разбора
        # содержимого прогон зеленел бы на сломанном ходе.
        r = json.dumps(state["results"][0], ensure_ascii=False)
        check('"error"' not in r, "задача завершилась без ошибки", r[:160])
    check(os.path.exists("/tmp/execai-serve-victim"), "отказ из веба РЕАЛЬНО остановил команду")
    if os.path.exists("/tmp/execai-serve-victim"):
        os.remove("/tmp/execai-serve-victim")
    check(state["acked"], "задача подтверждена (ack), иначе придёт повторно")

    # 2. Разрешение из веба: «навсегда» обязано записаться на машину, иначе
    #    следующая задача спросит снова и автономность не наступит.
    with lock:
        state["answer"] = "always"
        state["questions"].clear()
        state["results"].clear()
        script[:] = [{"tool": "Bash", "args": {"command": "mkdir -p /tmp/execai-serve-made"}},
                     {"text": "сделал"}]
        state["tasks"].append({"id": "t-2", "workspace_id": "ws-1", "payload": "создай каталог"})

    check(wait_for(lambda: state["results"], 45), "вторая задача завершилась")
    perms = json.load(open(f"{CONF}/execai/permissions.json"))
    check("Bash" in (perms.get("always_allowed_tools") or []),
          "«навсегда» из веба записалось в permissions.json", json.dumps(perms, ensure_ascii=False))
    check(os.path.isdir("/tmp/execai-serve-made"), "разрешённая команда выполнилась")
    if os.path.isdir("/tmp/execai-serve-made"):
        os.rmdir("/tmp/execai-serve-made")

    # 3. Аудит: каждое решение обязано остаться в журнале.
    audit = f"{CONF}/execai/serve-audit.log"
    alt = os.path.join(WS, "serve-audit.log")
    path = audit if os.path.exists(audit) else alt
    if os.path.exists(path):
        body = open(path, encoding="utf-8", errors="replace").read()
        check("deny" in body and "allow" in body,
              "в журнале есть и отказ, и разрешение", body[-90:].replace("\n", " "))
    else:
        check(False, "журнал решений найден", f"нет ни {audit}, ни {alt}")

    p.terminate()
    try:
        p.wait(timeout=8)
    except Exception:
        p.kill()
    check(not any("panic:" in l for l in out), "в выводе нет паник",
          next((l for l in out if "panic:" in l), ""))
    api.shutdown()
    llm.shutdown()
    return report()


def report():
    bad = [c for c in checks if not c[0]]
    print(f"\nИтог: {len(checks) - len(bad)} из {len(checks)} проверок прошли")
    if bad:
        print("Упало:")
        for _, name, detail in bad:
            print(f"  - {name}{(' — ' + detail) if detail else ''}")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
