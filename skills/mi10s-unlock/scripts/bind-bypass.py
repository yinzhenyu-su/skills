#!/usr/bin/env python3
"""
Bypass the HyperOS "Mi Community qualification" gate on bootloader-unlock account binding.

Background and the exact weakness being abused: references/internals.md

  scripts/bind-bypass.py                # capture a bind request, rewrite, ask, replay
  scripts/bind-bypass.py --dry-run      # capture + analyse only, never send
  scripts/bind-bypass.py --send cap.json
  scripts/bind-bypass.py --global       # non-mainland ROM (intl endpoint)

What it does NOT do: it does not shorten the 168h wait, does not unlock anything,
and does not touch the device's partitions. It only makes the server record the
account/device binding the way an MIUI-era client would.
"""

import argparse
import base64
import hashlib
import hmac
import json
import os
import re
import shutil
import subprocess
import sys
import time
import urllib.parse
import urllib.request

# shipped inside com.android.settings, used to decrypt the payload it logs
AES_KEY = b"20nr1aobv2xi8ax4"
AES_IV = b"0102030405060708"
# shipped inside com.android.settings, used to sign the request it sends
SIGN_KEY = "10f29ff413c89c8de02349cb3eb9a5f510f29ff413c89c8de02349cb3eb9a5f5"

SID = "miui_sec_android"
TAG = "CloudDeviceStatus"
BIND_PATH = "/v1/unlock/applyBind"

CODES = {
    0: ("bound", "账号绑定成功"),
    401: ("error", "账号凭据过期，需在手机上重新登录小米账号"),
    20086: ("error", "设备凭据过期，重启手机后重试"),
    30001: ("error", "设备被强制校验账号资格 —— 这条路封了"),
    86015: ("error", "设备签名无效 / 该报文已被使用"),
}


def adb_bin() -> str:
    for cand in (os.environ.get("ADB"), shutil.which("adb"), shutil.which("adb-darwin")):
        if cand:
            return cand
    sys.exit("adb not found (set ADB=/path/to/adb)")


def sh(cmd, timeout=60):
    return subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=timeout)


def log(msg, kind="INFO"):
    print(f"[{kind}] {msg}", flush=True)


def mask(value) -> str:
    """Account ids are personal data - never print them in full."""
    s = str(value or "")
    return s if len(s) < 7 else f"{s[:3]}***{s[-3:]}"


# Captures contain a live session token, so they must never land inside the repo.
STATE_DIR = os.environ.get("MI10S_STATE") or os.path.join(
    os.path.expanduser("~/.local/state"), "mi10s-unlock"
)


# ----------------------------------------------------------------- crypto

def aes_decrypt(blob_b64: str) -> str:
    out = subprocess.run(
        ["openssl", "enc", "-d", "-aes-128-cbc",
         "-K", AES_KEY.hex(), "-iv", AES_IV.hex(), "-nopad"],
        input=base64.b64decode(blob_b64), capture_output=True,
    )
    plain = out.stdout
    if not plain:
        raise ValueError(f"openssl failed: {out.stderr.decode(errors='replace')[:200]}")
    pad = plain[-1]
    if 1 <= pad <= 16:
        plain = plain[:-pad]
    return plain.decode(errors="replace")


def sign(data_json: str, path: str = BIND_PATH) -> str:
    msg = f"POST\n{path}\ndata={data_json}&sid={SID}"
    return hmac.new(SIGN_KEY.encode(), msg.encode(), hashlib.sha1).hexdigest()


# ----------------------------------------------------------------- capture

def capture(adb: str, timeout: int) -> dict:
    serial = sh(f"{adb} get-state").stdout.strip()
    if serial != "device":
        sys.exit("no adb device")

    sh(f"{adb} logcat -c")
    sh(f"{adb} shell am start -a android.settings.APPLICATION_DEVELOPMENT_SETTINGS")
    log("on the phone: 开发者选项 -> 设备解锁状态 -> 添加账号和设备 -> 同意", "TODO")

    proc = subprocess.Popen(f"{adb} logcat *:S {TAG}:V", shell=True,
                            stdout=subprocess.PIPE, text=True)
    args = headers = None
    deadline = time.time() + timeout
    try:
        while time.time() < deadline:
            line = proc.stdout.readline()
            if not line:
                continue
            if "args:" in line and args is None:
                args = re.search(r"args:(.*)", line).group(1).strip()
                log(f"captured args ({len(args)} b64 chars)")
            if "headers:" in line:
                headers = re.search(r"headers:(.*)", line).group(1).strip()
                log("captured headers")
                break
    finally:
        proc.kill()

    if not args:
        sys.exit("no bind request captured (did you tap 添加账号和设备?)")
    return {"args": args, "headers": headers or ""}


def analyse(cap: dict) -> tuple[str, dict]:
    payload = json.loads(aes_decrypt(cap["args"]))
    if "heartbeat_mode" in payload:
        log("captured a heartbeat, not a bind request - retry and tap 添加账号和设备", "ERROR")
        sys.exit(2)

    before = payload.get("rom_version")
    after = str(before).replace("V816", "V14")
    if before == after:
        log(f"rom_version {before} is not HyperOS - nothing to rewrite, sending as-is", "WARN")
    payload["rom_version"] = after

    log(f"rom_version : {before} -> {after}")
    log(f"userId      : {mask(payload.get('userId'))}")
    return json.dumps(payload, separators=(",", ":"), ensure_ascii=False), payload


# ----------------------------------------------------------------- send

def send(data_json: str, cap: dict, global_rom: bool) -> dict:
    api = ("https://unlock.update.intl.miui.com" if global_rom
           else "https://unlock.update.miui.com") + "/v1/"
    cookie = ""
    if cap.get("headers"):
        hdr = aes_decrypt(cap["headers"])
        m = re.search(r"Cookie=\[(.*?)\]", hdr)
        cookie = m.group(1) if m else ""

    body = urllib.parse.urlencode(
        {"data": data_json, "sid": SID, "sign": sign(data_json)}
    ).encode()
    req = urllib.request.Request(
        api + "unlock/applyBind", data=body, method="POST",
        headers={"Cookie": cookie,
                 "Content-Type": "application/x-www-form-urlencoded"},
    )
    with urllib.request.urlopen(req, timeout=20) as r:
        return json.loads(r.read().decode())


def report(res: dict) -> int:
    code = res.get("code")
    kind, msg = CODES.get(code, ("WARN", f"{res.get('descEN') or res.get('description')} ({code})"))
    log(msg, kind.upper())
    if code == 0:
        log(f"userId = {mask((res.get('data') or {}).get('userId'))}", "OK")
        log("接下来再跑一次正规解锁流程，等待期未到时服务器会返回 20036 并告诉你差多少小时", "OK")
        return 0
    return 1


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry-run", action="store_true", help="capture and analyse only")
    ap.add_argument("--send", metavar="FILE", help="replay a saved capture")
    ap.add_argument("--global", dest="global_rom", action="store_true",
                    help="use the international unlock endpoint")
    ap.add_argument("--timeout", type=int, default=240)
    a = ap.parse_args()

    adb = adb_bin()

    if a.send:
        cap = json.load(open(a.send))
        data_json, _ = analyse(cap)
        return report(send(data_json, cap, a.global_rom))

    cap = capture(adb, a.timeout)
    os.makedirs(STATE_DIR, mode=0o700, exist_ok=True)
    out = os.path.join(STATE_DIR, "capture.json")
    with open(out, "w") as f:
        json.dump(cap, f)
    os.chmod(out, 0o600)
    log(f"saved -> {out}  (contains your session token, keep it private)")

    data_json, _ = analyse(cap)
    if a.dry_run:
        log("dry-run: nothing sent", "OK")
        return 0
    if input("send the rewritten bind request now? [y/N] ").strip().lower() != "y":
        log("aborted")
        return 1
    return report(send(data_json, cap, a.global_rom))


if __name__ == "__main__":
    sys.exit(main())
