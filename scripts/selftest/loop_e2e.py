#!/usr/bin/env python3
"""Настоящий ход агента: инструмент → разрешение → выполнение → ответ.

Зачем отдельно от юнитов. `RequiresApproval` можно проверить вызовом функции,
но это проверка функции, а не поведения. Между ней и человеком лежит весь
цикл: агент решает, спрашивает, применяет ответ, запоминает разрешение,
отдаёт результат модели. Дефект в любом звене — и периметр, безупречный в
юнитах, на практике не работает.

Провайдер здесь поддельный: локальный сервер, отвечающий как OpenAI-совместимый
API. Поэтому прогон ничего не стоит, работает без сети и без входа, а сценарий
задаётся точно — модель «просит» ровно тот инструмент, который мы проверяем.
"""
import json
import os
import subprocess
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

BIN = os.environ.get("EXECAI_BIN", "./execai")
WS = os.environ.get("WORKDIR", "/tmp/execai-loop-ws")
CONF = os.environ["XDG_CONFIG_HOME"]

checks = []


def check(ok, name, detail=""):
    checks.append((bool(ok), name, detail))
    print(("  ok   " if ok else "  ПРОВАЛ ") + name + (f" — {detail}" if detail else ""))


# ── Поддельный провайдер ──────────────────────────────────────────────────
#
# Сценарий задаётся снаружи: список «ходов» модели. Каждый ход — либо вызов
# инструмента, либо финальный текст. Сервер отдаёт их по очереди.
script = []
seen_requests = []


class Provider(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass  # не засорять вывод прогона

    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(n) or b"{}")
        seen_requests.append(body)

        step = script.pop(0) if script else {"text": "готово"}
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.end_headers()

        def sse(obj):
            self.wfile.write(b"data: " + json.dumps(obj).encode() + b"\n\n")
            self.wfile.flush()

        if "tool" in step:
            sse({"choices": [{"delta": {"tool_calls": [{
                "index": 0, "id": "call_1", "type": "function",
                "function": {"name": step["tool"], "arguments": json.dumps(step["args"])},
            }]}}]})
            sse({"choices": [{"delta": {}, "finish_reason": "tool_calls"}]})
        else:
            sse({"choices": [{"delta": {"content": step["text"]}}]})
            sse({"choices": [{"delta": {}, "finish_reason": "stop"}]})
        self.wfile.write(b"data: [DONE]\n\n")
        self.wfile.flush()


def start_provider():
    srv = ThreadingHTTPServer(("127.0.0.1", 0), Provider)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return srv, srv.server_address[1]


# ── Агент ─────────────────────────────────────────────────────────────────
class Agent:
    def __init__(self):
        self.events = []
        self.p = subprocess.Popen(
            [BIN, "ide", "--cwd", WS],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, text=True, bufsize=1,
        )
        threading.Thread(target=self._pump, daemon=True).start()
        self.stderr = []
        threading.Thread(target=self._pump_err, daemon=True).start()

    def _pump(self):
        for line in self.p.stdout:
            line = line.strip()
            if line:
                try:
                    self.events.append(json.loads(line))
                except json.JSONDecodeError:
                    self.events.append({"type": "__garbage__", "text": line[:200]})

    def _pump_err(self):
        for line in self.p.stderr:
            self.stderr.append(line.rstrip())

    def send(self, obj):
        self.p.stdin.write(json.dumps(obj, ensure_ascii=False) + "\n")
        self.p.stdin.flush()

    def wait(self, typ, timeout=30, since=0):
        """Ждёт событие, начиная с указанной отметки.

        Без `since` проверка находила вопрос от ПРЕДЫДУЩЕГО сценария и
        проверяла не то — тихо и убедительно.
        """
        end = time.time() + timeout
        while time.time() < end:
            for e in self.events[since:]:
                if e.get("type") == typ:
                    return e
            time.sleep(0.1)
        return None

    def after(self, mark, typ):
        return [e for e in self.events[mark:] if e.get("type") == typ]

    def close(self):
        try:
            self.p.stdin.close()
            self.p.wait(timeout=5)
        except Exception:
            self.p.kill()


def setup_config(port):
    """Подписка на поддельного провайдера — так же, как это делает /connect."""
    os.makedirs(f"{CONF}/execai", exist_ok=True)
    json.dump({"api_base": "http://127.0.0.1:1", "security_level": "deep"},
              open(f"{CONF}/execai/config.json", "w"))
    # Формат — как у настоящего файла: карта по провайдеру, а не список.
    json.dump({
        "subscriptions": {
            "openai": {
                "provider": "openai", "api_key": "test",
                "base_url": f"http://127.0.0.1:{port}/v1",
                "available_models": ["test-model"], "plan": "selftest",
            },
        },
        "active": "openai",
    }, open(f"{CONF}/execai/subscriptions.json", "w"))
    # Разрешений нет: каждый вопрос должен быть задан честно.
    json.dump({"always_allowed_tools": [], "always_allowed_exact": None},
              open(f"{CONF}/execai/permissions.json", "w"))


def main():
    os.makedirs(WS, exist_ok=True)
    open(f"{WS}/main.go", "w").write("package main\n")
    srv, port = start_provider()
    setup_config(port)

    a = Agent()
    if a.wait("ready", 20) is None:
        check(False, "агент поднялся", "; ".join(a.stderr[:3]))
        return report()
    check(True, "агент поднялся с поддельным провайдером")

    # 1. Безопасная команда выполняется молча и доходит до ответа.
    script[:] = [{"tool": "Bash", "args": {"command": "ls -la"}}, {"text": "посмотрел"}]
    mark = len(a.events)
    a.send({"type": "user", "text": "покажи файлы"})
    done = a.wait("done", 40, since=mark)
    check(done is not None, "ход завершился")
    check(not a.after(mark, "ask"), "простая команда не спрашивает разрешения",
          str([e.get("summary") for e in a.after(mark, "ask")])[:90])
    calls = a.after(mark, "tool_call")
    check(any(c.get("tool") == "Bash" for c in calls), "инструмент был вызван")
    res = a.after(mark, "tool_result")
    check(res and res[0].get("ok"), "команда выполнилась успешно",
          (res[0].get("tail") or "")[:60] if res else "результата нет")

    # 2. Опасная команда обязана спросить — и отказ обязан её остановить.
    script[:] = [{"tool": "Bash", "args": {"command": "rm -rf /tmp/execai-loop-victim"}},
                 {"text": "не стал"}]
    open("/tmp/execai-loop-victim", "w").write("цел")
    mark = len(a.events)
    a.send({"type": "user", "text": "удали файл"})
    ask = a.wait("ask", 30, since=mark)
    check(ask is not None, "опасная команда спрашивает разрешение")
    if ask:
        a.send({"type": "answer", "id": ask.get("id"), "value": "deny"})
        a.wait("done", 40, since=mark)
        check(os.path.exists("/tmp/execai-loop-victim"), "отказ РЕАЛЬНО остановил команду")
        os.remove("/tmp/execai-loop-victim")

    # 3. Чтение вне проекта спрашивает, и разрешение даётся на КАТАЛОГ.
    outside = "/tmp/execai-loop-outside"
    os.makedirs(outside, exist_ok=True)
    open(f"{outside}/a.txt", "w").write("первый")
    open(f"{outside}/b.txt", "w").write("второй")
    script[:] = [{"tool": "Read", "args": {"path": f"{outside}/a.txt"}}, {"text": "прочитал"}]
    mark = len(a.events)
    a.send({"type": "user", "text": "прочитай файл снаружи"})
    ask = a.wait("ask", 30, since=mark)
    check(ask is not None, "чтение вне проекта спрашивает")
    if ask:
        summary = (ask.get("summary") or "")
        check("каталог" in summary, "в вопросе видно, ЧТО именно разрешается", summary[:80])
        a.send({"type": "answer", "id": ask.get("id"), "value": "always"})
        a.wait("done", 40, since=mark)

    # 3b. Соседний файл того же каталога больше не спрашивает — вопросы
    #     обязаны затухать, иначе автономность агента умирает.
    script[:] = [{"tool": "Read", "args": {"path": f"{outside}/b.txt"}}, {"text": "и этот"}]
    mark = len(a.events)
    a.send({"type": "user", "text": "прочитай соседний"})
    a.wait("done", 40, since=mark)
    check(not a.after(mark, "ask"), "разрешение на каталог покрыло соседний файл")

    # 4. Секрет спрашивает ОТДЕЛЬНО, даже когда каталог уже разрешён.
    open(f"{outside}/.env", "w").write("SECRET=1")
    script[:] = [{"tool": "Read", "args": {"path": f"{outside}/.env"}}, {"text": "…"}]
    mark = len(a.events)
    a.send({"type": "user", "text": "прочитай .env"})
    ask = a.wait("ask", 30, since=mark)
    check(ask is not None, "секрет спрашивает, хотя каталог разрешён")
    if ask:
        a.send({"type": "answer", "id": ask.get("id"), "value": "deny"})
        a.wait("done", 40, since=mark)

    # 5. Уровень light открывает периметр — настройка обязана работать.
    a.send({"type": "command", "name": "set_security", "value": "light"})
    time.sleep(1.0)
    script[:] = [{"tool": "Read", "args": {"path": f"{outside}/a.txt"}}, {"text": "…"}]
    mark = len(a.events)
    a.send({"type": "user", "text": "ещё раз снаружи"})
    a.wait("done", 40, since=mark)
    check(not a.after(mark, "ask"), "на light чтение снаружи не спрашивается")
    a.send({"type": "command", "name": "set_security", "value": "deep"})
    time.sleep(0.8)

    # 6. Продолжение чата: панель открывается заново и подхватывает разговор.
    #
    # Проверяем не показ, а ПАМЯТЬ: после восстановления модель должна
    # получить прежние сообщения, иначе «чат продолжился» только на экране.
    a.send({"type": "user", "text": "запомни слово ЯБЛОКО"})
    script[:] = [{"text": "запомнил"}]
    a.wait("done", 40, since=len(a.events) - 1)
    time.sleep(1.0)

    b = Agent()  # новый процесс — как повторное открытие панели
    if b.wait("ready", 20):
        mark = len(b.events)
        b.send({"type": "command", "name": "resume_last"})
        loaded = b.wait("chat_loaded", 20, since=mark)
        check(loaded is not None, "последний чат восстанавливается сам")
        if loaded:
            texts = " ".join(m.get("text", "") for m in loaded.get("msgs", []))
            check("ЯБЛОКО" in texts, "в восстановленном чате прежняя переписка", texts[:80])

        # И главное: продолжение уходит модели ВМЕСТЕ с историей.
        seen_requests.clear()
        script[:] = [{"text": "яблоко"}]
        mark = len(b.events)
        b.send({"type": "user", "text": "какое слово я просил запомнить?"})
        b.wait("done", 40, since=mark)
        sent = json.dumps(seen_requests, ensure_ascii=False) if seen_requests else ""
        check("ЯБЛОКО" in sent, "история ушла модели — чат продолжается, а не только показан",
              "в запросе прежних сообщений нет" if "ЯБЛОКО" not in sent else "")
        b.close()

    # 8. Провайдер вообще получал запросы — страховка от «всё зелено, потому
    #    что ничего не происходило».
    check(len(seen_requests) > 0, "провайдер получал запросы", str(len(seen_requests)))

    a.close()
    check(not [l for l in a.stderr if "panic:" in l], "в stderr нет паник",
          next((l for l in a.stderr if "panic:" in l), ""))
    srv.shutdown()
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
