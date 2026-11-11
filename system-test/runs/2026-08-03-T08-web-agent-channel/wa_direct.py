#!/usr/bin/env python3
# Прямые сценарии WA-листа (T08): зовём /agents-vbai/tasks/run как aihandler,
# разбираем SSE-ответ по контракту tool-сервисов (output/error, base64).
#
#   ./wa_direct.py wa3|wa4|wa5b|wa6-offline|wa9 [аргументы]
#
# Каждый шаг печатает JSON-вердикт — строки идут в journal.json прогона.
import base64
import json
import os
import ssl
import sys
import time
import urllib.request

# Контур задаётся окружением — один скрипт на dev и stage:
#   WA_GW=https://apistage.velesbsd.com WA_CRED=<путь>/credentials.json
#   WA_WS_A=<ws-id> WA_WS_B=<ws-id>   (второй нужен только для wa4)
GW = os.environ.get("WA_GW", "https://apidev.velesbsd.com")
CRED = json.load(open(os.path.expanduser(
    os.environ.get("WA_CRED", "~/.config/execai/credentials.json"))))
TOKEN, AG = CRED["token"], CRED["agent_id"]
DEF_WS = os.environ.get("WA_WS_A", "51f2d4e0-4d12-4a74-abc9-1b68393ac105")
HUY_WS = os.environ.get("WA_WS_B", "0ca9a4ca-a33b-4fd2-a5dd-7d7f118c80ed")
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


def run_task(agent, task, timeout=180):
    """POST /tasks/run, разобрать SSE. Возвращает (kind, text, seconds, keepalives)."""
    req = urllib.request.Request(GW + "/agents-vbai/tasks/run", method="POST",
                                 data=json.dumps({"agent": agent, "task": task}).encode())
    req.add_header("Authorization", "Bearer " + TOKEN)
    req.add_header("Content-Type", "application/json")
    t0 = time.time()
    keep = 0
    try:
        with urllib.request.urlopen(req, timeout=timeout, context=CTX) as r:
            for raw in r:
                line = raw.decode().strip()
                if not line.startswith("data:"):
                    continue
                chunk = json.loads(line[5:].strip())
                fr = chunk.get("function_result")
                if fr == "keepalive":
                    keep += 1
                    continue
                if fr in ("output", "error"):
                    text = base64.b64decode(chunk.get("content", "")).decode("utf-8", "replace")
                    return fr, text, round(time.time() - t0, 1), keep
    except Exception as e:  # HTTP-ошибка до SSE — тоже вердикт
        return "http", str(e), round(time.time() - t0, 1), keep
    return "none", "", round(time.time() - t0, 1), keep


def toggle(ws, rec_id, enabled):
    st, _ = api("PUT", f"/integrations-vbai/workspaces/{ws}/tools/{rec_id}",
                {"enabled": enabled})
    assert st == 200, f"toggle HTTP {st}"


def agent_records():
    _, body = api("GET", "/integrations-vbai/workspaces")
    out = {}
    for w in json.loads(body):
        for t in w.get("tools", []):
            if t["tool_id"] == "agent" and t.get("profile"):
                out.setdefault(w["id"], {})[t["profile"]] = t["id"]
    return out


def verdict(name, ok, detail):
    print(json.dumps({"step": name, "verdict": "PASS" if ok else "FAIL",
                      "detail": detail}, ensure_ascii=False))
    return ok


def wa3():
    recs = agent_records()
    rec = recs[HUY_WS][AG]
    # Активный проект — huy (страховка от чужого состояния).
    api("PUT", "/integrations-vbai/active-workspace", {"workspace_id": HUY_WS})
    toggle(HUY_WS, rec, False)
    try:
        kind, text, sec, _ = run_task(AG, "echo wa3")
        off_ok = verdict("wa3-off", kind == "error" and "выключен" in text and sec < 10,
                         f"{kind}/{sec}s: {text[:90]}")
    finally:
        toggle(HUY_WS, rec, True)
    kind, text, sec, _ = run_task(AG, "выведи ровно строку WA3-OK и больше ничего")
    on_ok = verdict("wa3-on", kind == "output" and "WA3-OK" in text, f"{kind}/{sec}s: {text[:90]}")
    return off_ok and on_ok


def wa4():
    results = {}
    for ws, want in ((DEF_WS, "agent-vbai"), (HUY_WS, "aicore-vbai")):
        api("PUT", "/integrations-vbai/active-workspace", {"workspace_id": ws})
        kind, text, sec, _ = run_task(AG, "выведи только pwd, одной строкой, без пояснений")
        results[want] = (kind, text.strip(), sec)
    api("PUT", "/integrations-vbai/active-workspace", {"workspace_id": HUY_WS})
    ok = True
    for want, (kind, text, sec) in results.items():
        ok &= verdict(f"wa4-{want}", kind == "output" and text.endswith(want),
                      f"{kind}/{sec}s: {text[-60:]}")
    return ok


def wa5b():
    phantom = "wa-phantom-" + time.strftime("%H%M%S")
    # Машина «существует» для проекта: запись инструмента + привязка каталога.
    st, body = api("POST", f"/integrations-vbai/workspaces/{HUY_WS}/tools",
                   {"tool_id": "agent", "profile": phantom, "enabled": True})
    rec = json.loads(body)["id"]
    st, _ = api("POST", "/agents-vbai/workspaces/bind",
                {"workspace_id": HUY_WS, "agent_id": phantom, "local_path": "/tmp"})
    assert st in (200, 201), f"bind phantom HTTP {st}"
    try:
        kind, text, sec, _ = run_task(phantom, "echo wa5b", timeout=40)
        # Фантом никогда не заберёт задачу → отказ «агент не отвечает» с task_id.
        got_offline = kind == "error" and "не отвечает" in text
        task_id = text.split("Задача ")[-1].split()[0] if "Задача " in text else ""
        ok1 = verdict("wa5b-offline-verdict", got_offline and bool(task_id),
                      f"{kind}/{sec}s task={task_id[:8]}")
        # Результат от НАШЕЙ машины для задачи, адресованной фантому → 403.
        st, body = api("POST", f"/agents-vbai/tasks/{task_id}/result",
                       {"chunk_type": "final", "data": "подделка"})
        ok2 = verdict("wa5b-foreign-result-403", st == 403, f"HTTP {st}: {body[:80]}")
        return ok1 and ok2
    finally:
        api("POST", "/agents-vbai/workspaces/unbind",
            {"workspace_id": HUY_WS, "agent_id": phantom})
        api("DELETE", f"/integrations-vbai/workspaces/{HUY_WS}/tools/{rec}")


def wa6_offline():
    kind, text, sec, _ = run_task(AG, "выведи ровно строку WA6-DONE", timeout=40)
    task_id = text.split("Задача ")[-1].split()[0] if "Задача " in text else ""
    return verdict("wa6-offline", kind == "error" and "не отвечает" in text and sec < 16,
                   f"{kind}/{sec}s task={task_id}") and print(task_id) is None


def wa9():
    t0 = time.time()
    kind, text, sec, keep = run_task(
        AG, "выполни команду: sleep 150 && echo WA9-DONE — и покажи её вывод", timeout=200)
    ok1 = verdict("wa9-timeout-verdict",
                  kind == "error" and "не уложился" in text and 100 < sec < 140 and keep >= 10,
                  f"{kind}/{sec}s keepalives={keep}: {text[:100]}")
    task_id = text.split("Задача ")[-1].split()[0] if "Задача " in text else ""
    print(json.dumps({"wa9_task": task_id}))
    return ok1


if __name__ == "__main__":
    fn = {"wa3": wa3, "wa4": wa4, "wa5b": wa5b, "wa6-offline": wa6_offline, "wa9": wa9}[sys.argv[1]]
    sys.exit(0 if fn() else 1)
