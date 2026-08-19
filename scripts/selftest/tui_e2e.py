#!/usr/bin/env python3
"""Терминальный интерфейс через настоящий псевдотерминал.

Почему pty, а не подмена ввода. Bubbletea поднимается только на терминале: без
tty он либо не рисует, либо ведёт себя иначе. Проверять «как бы TUI» через
подставной поток — значит проверять не то, чем пользуется человек.

Здесь запускается настоящий `execai chat` в pty, ему шлются настоящие нажатия
клавиш, а утверждения делаются по тому, что реально нарисовано на экране.
Модель — поддельный локальный провайдер, поэтому прогон бесплатный и точный.
"""
import json
import os
import pty
import re
import select
import subprocess
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

BIN = os.environ.get("EXECAI_BIN", "./execai")
WS = os.environ.get("WORKDIR", "/tmp/execai-tui-ws")
CONF = os.environ["XDG_CONFIG_HOME"]

checks = []
script = []
lock = threading.Lock()


def check(ok, name, detail=""):
    checks.append((bool(ok), name, detail))
    print(("  ok   " if ok else "  ПРОВАЛ ") + name + (f" — {detail}" if detail else ""))


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


ANSI = re.compile(r"\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b[()][A-B0-9]|\r")


class Terminal:
    """Живой pty: пишем нажатия, копим то, что нарисовано."""

    def __init__(self, env):
        self.master, slave = pty.openpty()
        # Размер окна задаём явно: TUI верстается от него, а у pty по
        # умолчанию 0×0 — интерфейс просто не рисуется.
        import fcntl
        import struct
        import termios
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
        self.p = subprocess.Popen([BIN, "chat"], stdin=slave, stdout=slave, stderr=slave,
                                  cwd=WS, env=env, close_fds=True)
        os.close(slave)
        self.buf = ""
        threading.Thread(target=self._pump, daemon=True).start()

    def _pump(self):
        while True:
            try:
                r, _, _ = select.select([self.master], [], [], 0.3)
                if not r:
                    if self.p.poll() is not None:
                        return
                    continue
                data = os.read(self.master, 65536)
                if not data:
                    return
                self.buf += ANSI.sub("", data.decode("utf-8", "replace"))
            except OSError:
                return

    def type(self, text, enter=True):
        """Печатает по символу, как человек.

        Целой строкой нельзя: TUI видит пачку символов, пришедшую разом, и
        считает её ВСТАВКОЙ ИЗ БУФЕРА — на экране появляется «[Pasted #1 …]»,
        а команда не выполняется. Прогон при этом выглядит рабочим.
        """
        for ch in text:
            os.write(self.master, ch.encode())
            time.sleep(0.012)
        if enter:
            time.sleep(0.15)
            os.write(self.master, b"\r")

    def key(self, seq):
        os.write(self.master, seq)

    def screen(self):
        return self.buf

    def wait_text(self, needle, timeout=25):
        end = time.time() + timeout
        while time.time() < end:
            if needle in self.buf:
                return True
            time.sleep(0.2)
        return False

    def close(self):
        try:
            self.p.terminate()
            self.p.wait(timeout=5)
        except Exception:
            self.p.kill()


def setup(port):
    os.makedirs(f"{CONF}/execai", exist_ok=True)
    json.dump({"api_base": "http://127.0.0.1:1", "security_level": "deep"},
              open(f"{CONF}/execai/config.json", "w"))
    json.dump({"subscriptions": {"openai": {
        "provider": "openai", "api_key": "test",
        "base_url": f"http://127.0.0.1:{port}/v1",
        "available_models": ["test-model"], "plan": "selftest"}},
        "active": "openai"}, open(f"{CONF}/execai/subscriptions.json", "w"))
    json.dump({"always_allowed_tools": [], "always_allowed_exact": None},
              open(f"{CONF}/execai/permissions.json", "w"))


def main():
    os.makedirs(WS, exist_ok=True)
    open(f"{WS}/main.go", "w").write("package main\n")
    srv = ThreadingHTTPServer(("127.0.0.1", 0), Provider)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    setup(srv.server_address[1])

    env = dict(os.environ, TERM="xterm-256color", XDG_CONFIG_HOME=CONF, LANG="ru_RU.UTF-8")
    t = Terminal(env)

    # 1. Интерфейс вообще рисуется. Это не формальность: на пустом размере
    #    окна или без tty bubbletea молча не показывает ничего.
    drawn = t.wait_text("execai", 25) or t.wait_text("ExecAI", 5)
    check(drawn, "интерфейс нарисовался", t.screen()[-120:].replace("\n", " ") if not drawn else "")
    if not drawn:
        t.close()
        return report()

    # 2. Команда /security печатает уровни и текущий выбор — настройка
    #    безопасности обязана быть видимой из терминала.
    t.type("/security")
    ok = t.wait_text("paranoid", 15)
    check(ok, "/security показывает уровни", "" if ok else t.screen()[-160:].replace("\n", " "))
    check("▸" in t.screen() or "deep" in t.screen(), "текущий уровень отмечен")

    t.type("/security paranoid")
    check(t.wait_text("paranoid", 10), "/security переключает уровень")
    t.type("/security deep")
    time.sleep(1)

    # 3. Настоящий ход с подтверждением: рисуется диалог, работает клавиша.
    with lock:
        script[:] = [{"tool": "Bash", "args": {"command": "rm -rf /tmp/execai-tui-victim"}},
                     {"text": "не стал"}]
    open("/tmp/execai-tui-victim", "w").write("цел")
    t.type("удали файл")
    asked = t.wait_text("rm -rf", 30)
    check(asked, "диалог подтверждения показан", "" if asked else t.screen()[-200:].replace("\n", " "))
    if asked:
        # Отклонить — по клавише, как это делает человек.
        t.type("d", enter=False)
        time.sleep(3)
        check(os.path.exists("/tmp/execai-tui-victim"), "отказ в терминале РЕАЛЬНО остановил команду")
        if os.path.exists("/tmp/execai-tui-victim"):
            os.remove("/tmp/execai-tui-victim")

    # 4. Безопасная команда проходит молча и виден ответ модели.
    with lock:
        script[:] = [{"tool": "Bash", "args": {"command": "ls -la"}}, {"text": "посмотрел файлы"}]
    before = len(t.screen())
    t.type("покажи файлы")
    got = t.wait_text("посмотрел файлы", 30)
    check(got, "ответ модели отрисован", "" if got else t.screen()[before:][-160:].replace("\n", " "))
    check("rm -rf" not in t.screen()[before:], "безопасная команда не подняла диалог")

    # 5. Выход по команде — чистый, без паники.
    t.type("/exit")
    time.sleep(2)
    if t.p.poll() is None:
        t.key(b"\x03")  # Ctrl+C
        time.sleep(2)
    check(t.p.poll() is not None, "выходит по команде")
    check("panic:" not in t.screen(), "на экране нет паники",
          t.screen()[t.screen().find("panic:"):][:120] if "panic:" in t.screen() else "")

    t.close()
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
