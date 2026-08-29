#!/usr/bin/env python3
"""Несколько чатов в одном процессе `execai ide`.

До этого ход выполнялся прямо в цикле чтения stdin: пока агент отвечал, ни
переключить вкладку, ни открыть другой чат было нельзя — команды лежали в
очереди до done. Здесь проверяется новая модель: ход — горутина на чат,
каждое событие несёт chat, переключение во время хода мгновенное, стоп бьёт
по своему чату, вопрос приходит из своего чата, пробуждение по таймеру
стартует ход само и переживает перезапуск процесса.

Провайдер поддельный (OpenAI-совместимый SSE): отвечает по СОДЕРЖАНИЮ
последнего сообщения, а не по очереди запросов — два параллельных хода не
имеют детерминированного порядка обращений.
"""
import json
import os
import subprocess
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

BIN = os.environ.get("EXECAI_BIN", "./execai")
WS = os.environ.get("WORKDIR", "/tmp/execai-multi-ws")
CONF = os.environ["XDG_CONFIG_HOME"]

checks = []


def check(ok, name, detail=""):
    checks.append((bool(ok), name, detail))
    print(("  ok   " if ok else "  ПРОВАЛ ") + name + (f" — {detail}" if detail else ""))


# ── Поддельный провайдер ──────────────────────────────────────────────────
def text_of(content):
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return " ".join(b.get("text", "") for b in content if isinstance(b, dict))
    return ""


class Provider(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(n) or b"{}")
        msgs = body.get("messages", [])
        last = msgs[-1] if msgs else {}
        last_user = ""
        for m in reversed(msgs):
            if m.get("role") == "user":
                last_user = text_of(m.get("content"))
                break
        step = self.plan(last, last_user)
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.end_headers()

        def sse(obj):
            self.wfile.write(b"data: " + json.dumps(obj).encode() + b"\n\n")
            self.wfile.flush()

        if "sleep" in step:
            time.sleep(step["sleep"])
        if "tool" in step:
            sse({"choices": [{"delta": {"tool_calls": [{
                "index": 0, "id": "call_1", "type": "function",
                "function": {"name": step["tool"], "arguments": json.dumps(step["args"])},
            }]}}]})
            sse({"choices": [{"delta": {}, "finish_reason": "tool_calls"}]})
        else:
            for piece in step["text"].split(" "):
                sse({"choices": [{"delta": {"content": piece + " "}}]})
                time.sleep(step.get("pace", 0))
            sse({"choices": [{"delta": {}, "finish_reason": "stop"}]})
        self.wfile.write(b"data: [DONE]\n\n")
        self.wfile.flush()

    @staticmethod
    def plan(last, last_user):
        # После результата инструмента — всегда финальный текст.
        if last.get("role") == "tool":
            return {"text": "после инструмента готово"}
        u = last_user.lower()
        if "медленно" in u:
            return {"text": "медленный ответ одного двух трёх четырёх пяти", "pace": 0.5}
        if "стопни" in u:
            return {"text": "очень длинный ответ " + "слово " * 40, "pace": 0.3}
        if "спроси" in u:
            return {"tool": "AskUser", "args": {"question": "Какой вариант?",
                    "options": [{"label": "первый"}, {"label": "второй"}]}}
        if "усни" in u:
            return {"tool": "schedule_wakeup", "args": {"delay_seconds": 1,
                    "reason": "тест пробуждения", "prompt_on_wake": "после сна проверь"}}
        if "усни-надолго" in u or "надолго" in u:
            return {"tool": "schedule_wakeup", "args": {"delay_seconds": 4,
                    "reason": "переживёт рестарт", "prompt_on_wake": "после рестарта"}}
        if "подожди" in u:
            return {"tool": "wait", "args": {"seconds": 1, "reason": "тест ожидания"}}
        if "после сна" in u or "после рестарта" in u:
            return {"text": "проснулся и продолжаю"}
        return {"text": "быстрый ответ"}


def start_provider():
    srv = ThreadingHTTPServer(("127.0.0.1", 0), Provider)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return srv, srv.server_address[1]


# ── Агент ─────────────────────────────────────────────────────────────────
class Agent:
    def __init__(self):
        self.events = []
        self.stderr = []
        self.p = subprocess.Popen(
            [BIN, "ide", "--cwd", WS],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, text=True, bufsize=1,
        )
        threading.Thread(target=self._pump, daemon=True).start()
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

    def wait(self, typ, timeout=30, since=0, chat=None, pred=None):
        end = time.time() + timeout
        while time.time() < end:
            for i in range(since, len(self.events)):
                e = self.events[i]
                if e.get("type") != typ:
                    continue
                if chat is not None and e.get("chat") != chat:
                    continue
                if pred is not None and not pred(e):
                    continue
                return e
            time.sleep(0.05)
        return None

    def after(self, mark, typ, chat=None):
        return [e for e in self.events[mark:] if e.get("type") == typ and (chat is None or e.get("chat") == chat)]

    def close(self):
        try:
            self.p.stdin.close()
            self.p.wait(timeout=8)
        except Exception:
            self.p.kill()


def setup_config(port):
    os.makedirs(f"{CONF}/execai", exist_ok=True)
    json.dump({"api_base": "http://127.0.0.1:1", "security_level": "deep"},
              open(f"{CONF}/execai/config.json", "w"))
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
    json.dump({"always_allowed_tools": [], "always_allowed_exact": None},
              open(f"{CONF}/execai/permissions.json", "w"))


def main():
    os.makedirs(WS, exist_ok=True)
    srv, port = start_provider()
    setup_config(port)

    a = Agent()
    ready = a.wait("ready", 20)
    check(ready is not None, "поднимается на поддельном провайдере")
    if not ready:
        for line in a.stderr[:10]:
            print("    stderr:", line)
        a.close()
        return report()

    # 1. Два чата: A отвечает медленно, B — пока A занят.
    a.send({"type": "new_chat"})
    ra = a.wait("chat_reset", 5)
    check(ra is not None and ra.get("chat"), "новый чат получает идентификатор сразу")
    A = (ra or {}).get("chat")

    mark = len(a.events)
    a.send({"type": "user", "text": "ответь медленно", "chat": A})
    ts = a.wait("turn_start", 10, since=mark, chat=A)
    check(ts is not None, "ход в чате A стартовал и помечен chat")

    a.send({"type": "new_chat"})
    rb = a.wait("chat_reset", 5, since=mark, pred=lambda e: e.get("chat") != A)
    B = (rb or {}).get("chat")
    check(bool(B) and B != A, "второй чат создаётся, пока A отвечает")

    mark_b = len(a.events)
    a.send({"type": "user", "text": "ответь быстро", "chat": B})
    done_b = a.wait("done", 15, since=mark_b, chat=B)
    done_a_early = a.after(mark, "done", chat=A)
    check(done_b is not None and not done_a_early, "B ответил раньше, чем A закончил (ходы параллельны)",
          f"done B: {bool(done_b)}, done A уже: {bool(done_a_early)}")

    # 2. Переключение на A во время его хода: история + поток с начала.
    mark2 = len(a.events)
    a.send({"type": "command", "name": "load_chat", "value": A})
    loaded = a.wait("chat_loaded", 5, since=mark2, chat=A)
    check(loaded is not None and loaded.get("busy") is True, "chat_loaded занятого чата помечен busy")
    replay_ts = a.wait("turn_start", 3, since=mark2, chat=A)
    check(replay_ts is not None, "при возврате в занятый чат поток хода повторяется с turn_start")
    deltas = a.after(mark2, "text_delta", chat=A)
    check(len(deltas) >= 1 and "медленный" in "".join(d.get("text", "") for d in deltas),
          "повтор содержит уже полученный текст хода")

    # list_chats: A busy, B нет.
    mark3 = len(a.events)
    a.send({"type": "command", "name": "list_chats"})
    chats = a.wait("chats", 5, since=mark3)
    by_id = {c["id"]: c for c in (chats or {}).get("chats", [])}
    check(A in by_id and by_id[A].get("busy") is True, "в списке чатов A помечен занятым", str(by_id.get(A)))
    check(B in by_id and not by_id[B].get("busy"), "B в списке не занят")

    done_a = a.wait("done", 30, since=mark, chat=A)
    check(done_a is not None and done_a.get("text") == "ok", "A дошёл до конца сам")

    # 3. Стоп бьёт по своему чату.
    mark4 = len(a.events)
    a.send({"type": "user", "text": "стопни это", "chat": A})
    a.send({"type": "user", "text": "ответь быстро", "chat": B})
    a.wait("turn_start", 5, since=mark4, chat=A)
    a.wait("text_delta", 10, since=mark4, chat=A)
    a.send({"type": "stop", "chat": A})
    stopped = a.wait("done", 10, since=mark4, chat=A)
    check(stopped is not None and stopped.get("text") == "stopped", "stop с chat останавливает именно A",
          str((stopped or {}).get("text")))
    ok_b = a.wait("done", 15, since=mark4, chat=B)
    check(ok_b is not None and ok_b.get("text") == "ok", "B при этом доехал до ok")

    # 4. Занятый чат не принимает второе сообщение, но честно говорит об этом.
    mark5 = len(a.events)
    a.send({"type": "user", "text": "ответь медленно", "chat": A})
    a.wait("turn_start", 5, since=mark5, chat=A)
    a.send({"type": "user", "text": "ещё", "chat": A})
    err = a.wait("error", 5, since=mark5, chat=A)
    check(err is not None and "отвечает" in err.get("text", ""), "второе сообщение в занятый чат отбивается с объяснением")
    a.wait("done", 30, since=mark5, chat=A)

    # 5. Вопрос приходит из своего чата и ответ доходит.
    mark6 = len(a.events)
    a.send({"type": "user", "text": "спроси меня", "chat": B})
    ask = a.wait("ask_user", 10, since=mark6, chat=B)
    check(ask is not None and ask.get("id"), "AskUser помечен чатом B")
    # Возврат в B во время вопроса повторяет вопрос.
    mark6b = len(a.events)
    a.send({"type": "command", "name": "load_chat", "value": B})
    again = a.wait("ask_user", 5, since=mark6b, chat=B)
    check(again is not None and again.get("id") == (ask or {}).get("id"), "при возврате в чат неотвеченный вопрос показывается снова")
    a.send({"type": "answer", "id": (ask or {}).get("id", ""), "value": "первый"})
    d6 = a.wait("done", 15, since=mark6, chat=B)
    check(d6 is not None and d6.get("text") == "ok", "ответ на вопрос продолжает ход")

    # 6. wait внутри хода.
    mark7 = len(a.events)
    a.send({"type": "user", "text": "подожди немного", "chat": B})
    tc = a.wait("tool_call", 10, since=mark7, chat=B, pred=lambda e: e.get("tool") == "wait")
    check(tc is not None, "инструмент wait вызывается")
    d7 = a.wait("done", 20, since=mark7, chat=B)
    check(d7 is not None and d7.get("text") == "ok", "ход с wait завершается ok")

    # 7. Пробуждение: schedule_wakeup → wakeup → ход сам стартует.
    mark8 = len(a.events)
    a.send({"type": "user", "text": "усни", "chat": A})
    d8 = a.wait("done", 20, since=mark8, chat=A)
    wk = a.wait("wakeup", 10, since=mark8, chat=A)
    check(wk is not None and wk.get("at"), "после хода приходит wakeup с временем", str((wk or {}).get("at")))
    auto = a.wait("auto_user", 15, since=len(a.events) if not wk else mark8, chat=A,
                  pred=lambda e: "после сна" in e.get("text", ""))
    check(auto is not None, "по таймеру стартует ход с промтом пробуждения")
    d8b = a.wait("done", 20, since=(a.events.index(auto) if auto else mark8), chat=A)
    check(d8b is not None and d8b.get("text") == "ok", "проснувшийся ход доходит до ok")
    woke_text = "".join(e.get("text", "") for e in a.after(mark8, "text_delta", chat=A))
    check("проснулся" in woke_text, "модель получила промт пробуждения")

    # 8. Отмена пробуждения.
    mark9 = len(a.events)
    a.send({"type": "user", "text": "усни-надолго", "chat": B})
    a.wait("wakeup", 20, since=mark9, chat=B)
    a.send({"type": "command", "name": "cancel_wakeup", "chat": B})
    canc = a.wait("wakeup_cancelled", 5, since=mark9, chat=B)
    check(canc is not None, "cancel_wakeup отменяет пробуждение")
    time.sleep(5)
    check(not a.after(mark9, "auto_user", chat=B), "отменённое пробуждение не стреляет")

    # 9. Пробуждение переживает рестарт процесса.
    mark10 = len(a.events)
    a.send({"type": "user", "text": "усни-надолго", "chat": A})
    a.wait("wakeup", 20, since=mark10, chat=A)
    a.close()
    check(a.p.returncode in (0, None), "первый процесс вышел штатно", str(a.p.returncode))

    b = Agent()
    check(b.wait("ready", 20) is not None, "второй процесс поднялся")
    wk2 = b.wait("wakeup", 10, chat=A)
    check(wk2 is not None, "сохранённое пробуждение восстановлено после рестарта")
    auto2 = b.wait("auto_user", 20, chat=A)
    check(auto2 is not None, "и срабатывает в новом процессе")
    d10 = b.wait("done", 20, since=(b.events.index(auto2) if auto2 else 0), chat=A)
    check(d10 is not None and d10.get("text") == "ok", "ход после рестарта доходит до ok")

    # Гигиена протокола.
    garbage = [e for e in a.events + b.events if e.get("type") == "__garbage__"]
    check(not garbage, "stdout содержит только протокол", (garbage[0].get("text") if garbage else ""))
    b.close()
    panics = [l for l in a.stderr + b.stderr if "panic:" in l or "fatal error:" in l]
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
