#!/usr/bin/env python3
"""Сквозная проверка IDE-протокола: живой процесс, настоящий stdin/stdout.

Юниты проверяют функции, но протокол ломается на стыке — «команда не дошла»,
«событие не пришло», «упал на старте без логина». Ровно эти три класса и
уезжали к владельцу, поэтому здесь поднимается настоящий бинарь и с ним
разговаривают так же, как это делает плагин.

Ничего не спрашивает и ничего не требует от человека: только те команды,
которые работают без входа и без сети.
"""
import json
import os
import subprocess
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

BIN = os.environ.get("EXECAI_BIN", "./execai")
WS = os.environ.get("WORKDIR", "/tmp/execai-selftest-ws")
os.makedirs(WS, exist_ok=True)

checks = []          # (ok, название, подробность)


def check(ok, name, detail=""):
    checks.append((bool(ok), name, detail))
    print(("  ok   " if ok else "  ПРОВАЛ ") + name + (f" — {detail}" if detail else ""))


class Agent:
    """Обёртка над процессом: пишем строки, копим события."""

    def __init__(self):
        self.events = []
        self.p = subprocess.Popen(
            [BIN, "ide", "--cwd", WS],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, text=True, bufsize=1,
        )
        threading.Thread(target=self._pump, daemon=True).start()
        threading.Thread(target=self._pump_err, daemon=True).start()
        self.stderr = []

    def _pump(self):
        for line in self.p.stdout:
            line = line.strip()
            if not line:
                continue
            try:
                self.events.append(json.loads(line))
            except json.JSONDecodeError:
                # Протокол обязан быть чистым: любой мусор в stdout — дефект.
                self.events.append({"type": "__garbage__", "text": line[:200]})

    def _pump_err(self):
        for line in self.p.stderr:
            self.stderr.append(line.rstrip())

    def send(self, obj):
        self.p.stdin.write(json.dumps(obj, ensure_ascii=False) + "\n")
        self.p.stdin.flush()

    def wait_for(self, typ, timeout=15):
        end = time.time() + timeout
        while time.time() < end:
            for e in self.events:
                if e.get("type") == typ:
                    return e
            time.sleep(0.1)
        return None

    def last(self, typ):
        found = [e for e in self.events if e.get("type") == typ]
        return found[-1] if found else None

    def close(self):
        try:
            self.p.stdin.close()
        except Exception:
            pass
        try:
            self.p.wait(timeout=5)
        except Exception:
            self.p.kill()


def main():
    a = Agent()

    # 1. Поднимается вообще. Это не формальность: на этом падал каждый новый
    #    человек — LoadCredentials без файла отдаёт (nil, nil), и был SIGSEGV.
    ready = a.wait_for("ready", 20)
    check(ready is not None, "поднимается без логина",
          "" if ready else "события ready не было")
    if ready is None:
        for line in a.stderr[:10]:
            print("    stderr:", line)
        a.close()
        return report()

    check(not any(e.get("type") == "__garbage__" for e in a.events),
          "stdout содержит только протокол")

    # 2. Состояние: набор полей, который читает плагин.
    a.send({"type": "command", "name": "state"})
    time.sleep(1.0)
    st = a.last("state")
    check(st is not None, "команда state отвечает")
    if st:
        for field in ("model", "source", "models", "sources", "efforts",
                      "connectable", "effort", "max_iter"):
            check(field in st, f"в state есть поле {field}")
        check(len(st.get("efforts", [])) == 5, "уровней effort пять",
              str([e.get("id") for e in st.get("efforts", [])]))
        check(len(st.get("connectable", [])) >= 9,
              "источников для подключения не меньше девяти",
              str(len(st.get("connectable", []))))

    # 2b. Уровень безопасности: виден в состоянии и переключается.
    if st:
        check(st.get("security") in ("light", "deep", "paranoid"),
              "в state есть уровень безопасности", str(st.get("security")))
        check(len(st.get("securities", [])) == 3, "уровней безопасности три")
    a.send({"type": "command", "name": "set_security", "value": "paranoid"})
    time.sleep(0.8)
    check((a.last("state") or {}).get("security") == "paranoid",
          "уровень безопасности переключается")
    before = len(a.events)
    a.send({"type": "command", "name": "set_security", "value": "чепуха"})
    time.sleep(0.6)
    check(any(e.get("type") == "error" for e in a.events[before:]),
          "неизвестный уровень отбивается")
    a.send({"type": "command", "name": "set_security", "value": "deep"})
    time.sleep(0.6)

    # 3. Настройки хода применяются и доезжают обратно.
    a.send({"type": "command", "name": "set_effort", "value": "high"})
    time.sleep(0.8)
    check((a.last("state") or {}).get("effort") == "high", "effort переключается")

    a.send({"type": "command", "name": "set_max_iterations", "value": "55"})
    time.sleep(0.8)
    check((a.last("state") or {}).get("max_iter") == 55, "предел итераций переключается")

    # 4. Мусор отбивается ошибкой, а не молчанием и не падением.
    before = len(a.events)
    a.send({"type": "command", "name": "set_effort", "value": "мусор"})
    time.sleep(0.8)
    check(any(e.get("type") == "error" for e in a.events[before:]),
          "неверное значение отбивается ошибкой")

    # 5. Подписки: подключение без ключа обязано отказать честно.
    before = len(a.events)
    a.send({"type": "command", "name": "connect", "value": "kimi", "key": ""})
    time.sleep(1.0)
    said = " ".join(e.get("text", "") for e in a.events[before:])
    check("ключ" in said.lower(), "подключение без ключа отказано с объяснением", said[:80])

    # 5b. Свой провайдер из панели: имя, тип интерфейса, адрес, ключ,
    #     заголовки. Модели — из ответа сервера; источник виден под своим
    #     именем и активен. Сервер без /models — модели перечисляет панель.
    seen = {}

    class Models(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_GET(self):
            seen["auth"], seen["tenant"] = self.headers.get("Authorization"), self.headers.get("X-Tenant")
            if self.path == "/v1/models" and seen["auth"] == "Bearer sk-ide":
                body = json.dumps({"data": [{"id": "m-1", "name": "Model One"}, {"id": "nomic-embed-text"}]}).encode()
                self.send_response(200)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
                return
            self.send_response(404)
            self.send_header("Content-Length", "0")
            self.end_headers()

    srv = ThreadingHTTPServer(("127.0.0.1", 0), Models)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    base = f"http://127.0.0.1:{srv.server_address[1]}"
    check(any(c.get("id") == "custom" for c in (a.last("state") or {}).get("connectable", [])),
          "в connectable есть свой провайдер")
    before = len(a.events)
    a.send({"type": "command", "name": "connect", "value": "custom", "label": "Panel GW",
            "api": "openai", "base_url": base, "key": "sk-ide", "headers": {"X-Tenant": "t1"}})
    end = time.time() + 15
    st = None
    while time.time() < end:
        sts = [e for e in a.events[before:] if e.get("type") == "state"]
        if sts:
            st = sts[-1]
            break
        time.sleep(0.2)
    src = next((x for x in (st or {}).get("sources", []) if x.get("id") == "panel-gw"), None)
    check(src is not None and src.get("label") == "Panel GW" and src.get("active"),
          "свой провайдер подключён и активен под своим именем", str(src))
    ids = [m.get("id") for m in (st or {}).get("models", [])]
    check(ids == ["m-1"], "модели — из ответа сервера, без embedding", str(ids))
    check(seen.get("tenant") == "t1", "свой заголовок дошёл до сервера", str(seen))

    before = len(a.events)
    a.send({"type": "command", "name": "connect", "value": "custom", "label": "Bare",
            "api": "anthropic", "base_url": base + "/bare", "models": ["a-1", "a-2"]})
    time.sleep(2.0)
    st = a.last("state") or {}
    ids = [m.get("id") for m in st.get("models", [])]
    check(ids == ["a-1", "a-2"], "без /models — модели из панели", str(ids))

    a.send({"type": "command", "name": "disconnect", "value": "panel-gw"})
    time.sleep(1.0)
    st = a.last("state") or {}
    check(not any(x.get("id") == "panel-gw" for x in st.get("sources", [])), "свой провайдер отключается")
    srv.shutdown()

    # 6. Неизвестная команда не роняет процесс.
    a.send({"type": "command", "name": "такой-команды-нет"})
    time.sleep(0.6)
    check(a.p.poll() is None, "неизвестная команда не роняет агента")

    # 7. Чаты и новый чат — то, чем плагин пользуется постоянно.
    a.send({"type": "command", "name": "list_chats"})
    time.sleep(1.0)
    check(a.last("chats") is not None, "список чатов отвечает")

    a.send({"type": "new_chat"})
    time.sleep(0.6)
    check(a.last("chat_reset") is not None, "новый чат сбрасывает состояние")

    # 8. Пинг — признак живого цикла после всего вышеперечисленного.
    a.send({"type": "ping"})
    time.sleep(0.5)
    check(a.last("pong") is not None, "цикл жив после всех команд")

    a.close()
    check(a.p.returncode in (0, None), "выходит без паники",
          f"код возврата {a.p.returncode}")
    panics = [l for l in a.stderr if "panic:" in l]
    check(not panics, "в stderr нет паник", panics[0] if panics else "")

    return report()


def report():
    bad = [c for c in checks if not c[0]]
    print(f"\nИтог: {len(checks) - len(bad)} из {len(checks)} проверок прошли")
    if bad:
        print("Упало:")
        for _, name, detail in bad:
            print(f"  - {name} {('— ' + detail) if detail else ''}")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
