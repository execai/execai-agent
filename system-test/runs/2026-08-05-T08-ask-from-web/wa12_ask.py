#!/usr/bin/env python3
# WA12 — агент спрашивает разрешение, человек отвечает из веба.
#
# Ставит задачу, требующую инструмента ВНЕ permissions.json, затем играет роль
# веб-морды: видит открытый вопрос в GET /questions и отвечает.
#
#   WA_GW=... WA_CRED=... ./wa12_ask.py once|deny|timeout
import base64
import json
import os
import ssl
import sys
import threading
import time
import urllib.request

GW = os.environ.get("WA_GW", "https://apidev.velesbsd.com")
CRED = json.load(open(os.path.expanduser(
    os.environ.get("WA_CRED", "~/.config/execai/credentials.json"))))
TOKEN, AG = CRED["token"], CRED["agent_id"]
CTX = ssl.create_default_context()


def api(method, path, body=None, timeout=30):
    req = urllib.request.Request(GW + path, method=method,
                                 data=json.dumps(body).encode() if body is not None else None)
    req.add_header("Authorization", "Bearer " + TOKEN)
    req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=timeout, context=CTX) as r:
            return r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


def run_task(task, timeout=300):
    """POST /tasks/run, разобрать SSE-контракт tool-сервисов."""
    req = urllib.request.Request(GW + "/agents-vbai/tasks/run", method="POST",
                                 data=json.dumps({"agent": AG, "task": task}).encode())
    req.add_header("Authorization", "Bearer " + TOKEN)
    req.add_header("Content-Type", "application/json")
    t0, keep = time.time(), 0
    try:
        with urllib.request.urlopen(req, timeout=timeout, context=CTX) as r:
            for raw in r:
                line = raw.decode().strip()
                if not line.startswith("data:"):
                    continue
                c = json.loads(line[5:].strip())
                fr = c.get("function_result")
                if fr == "keepalive":
                    keep += 1
                    continue
                if fr in ("output", "error"):
                    text = base64.b64decode(c.get("content", "")).decode("utf-8", "replace")
                    return fr, text, round(time.time() - t0, 1), keep
    except Exception as e:
        return "http", str(e), round(time.time() - t0, 1), keep
    return "none", "", round(time.time() - t0, 1), keep


def verdict(step, ok, detail):
    print(json.dumps({"step": step, "verdict": "PASS" if ok else "FAIL",
                      "detail": detail}, ensure_ascii=False), flush=True)
    return ok


def web_side(answer, seen, deadline=180):
    """Играем веб-морду: ждём вопрос и отвечаем (или молчим при timeout)."""
    end = time.time() + deadline
    while time.time() < end:
        st, body = api("GET", "/agents-vbai/questions")
        if st == 200:
            d = json.loads(body)
            qs = d.get("questions") or []
            if qs:
                q = qs[0]
                seen.append({"question": q, "options": d.get("options")})
                if answer is None:      # сценарий «молчание»
                    return
                st2, b2 = api("POST", f"/agents-vbai/questions/{q['id']}/answer",
                              {"answer": answer})
                seen.append({"answer_http": st2, "answer_body": b2[:120]})
                return
        time.sleep(1)


def scenario(answer, task, marker, expect_done):
    """marker — файл, который команда создаёт; expect_done — должен ли он появиться.

    Отказ НЕ делает задачу ошибочной: агент доводит её до конца и объясняет,
    что выполнять не стал. Поэтому судим по факту (создан файл или нет), а не
    по типу чанка — первая версия теста ошибалась именно здесь.
    """
    try:
        os.remove(marker)
    except OSError:
        pass
    seen = []
    t = threading.Thread(target=web_side, args=(answer, seen), daemon=True)
    t.start()
    kind, text, sec, keep = run_task(task)
    t.join(timeout=5)

    name = f"wa12-{answer or 'timeout'}"
    # Вопрос обязан появиться в веб-морде — иначе человеку нечего нажимать.
    q = next((s["question"] for s in seen if "question" in s), None)
    ok_q = verdict(name + "-question-visible", q is not None,
                   f"инструмент={q['tool'] if q else '—'} summary={(q or {}).get('summary','')[:50]}")
    # Варианты приходят вместе с вопросом: интерфейс не должен знать их наизусть.
    opts = next((s["options"] for s in seen if "options" in s), None)
    ok_o = verdict(name + "-options-shipped",
                   bool(opts) and {o["value"] for o in opts} == {"once", "session", "exact", "always", "deny"},
                   f"варианты={[o['value'] for o in (opts or [])]}")
    done = os.path.exists(marker)
    ok_r = verdict(name + "-effect", done == expect_done,
                   f"{kind}/{sec}s keepalives={keep} файл={'создан' if done else 'нет'}: {text[:90]}")
    return ok_q and ok_o and ok_r


if __name__ == "__main__":
    mode = sys.argv[1] if len(sys.argv) > 1 else "once"
    if mode == "once":
        ok = scenario("once", "выполни команду: touch /tmp/wa12-allowed && echo done", "/tmp/wa12-allowed", True)
    elif mode == "deny":
        ok = scenario("deny", "выполни команду: touch /tmp/wa12-denied && echo done", "/tmp/wa12-denied", False)
    else:
        ok = scenario(None, "выполни команду: touch /tmp/wa12-silence && echo done", "/tmp/wa12-silence", False)
    sys.exit(0 if ok else 1)
