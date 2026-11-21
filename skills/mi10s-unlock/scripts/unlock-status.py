#!/usr/bin/env python3
"""
Pre-flight query for Xiaomi bootloader unlock. Touches nothing.

Calls the same endpoint the official tool hits right after you click "Unlock"
(/api/v2/unlock/device/clear) and prints what the server says about wiping data.
It never calls /api/v3/ahaUnlock and never runs fastboot, so it cannot unlock or
wipe the device.

  pip install miunlock            # provides migate + the request signer
  scripts/unlock-status.py
  scripts/unlock-status.py --product thyme

The wait time itself ("please wait N hours") is only returned by ahaUnlock,
i.e. by running `miunlock` for real. That call is safe too: if the wait has not
expired it answers 20036 and no token quota is consumed.
"""

import argparse
import json
import random
import subprocess
import sys

from migate import get_passtoken, get_service, get_region
from miunlock.utils import _send

CN = "https://unlock.update.miui.com"
INTL = "https://unlock.update.intl.miui.com"
IN = "https://in-unlock.update.intl.miui.com"
RU = "https://ru-unlock.update.intl.miui.com"
EU = "https://eu-unlock.update.intl.miui.com"

EU_COUNTRIES = {
    "AT", "BE", "BG", "HR", "CY", "CZ", "DK", "EE", "FI", "FR", "DE", "EL", "HU",
    "IS", "IE", "IT", "LV", "LI", "LT", "LU", "MT", "NL", "NO", "PL", "PT", "RO",
    "SK", "SI", "ES", "SE", "UK",
}


def mask(value) -> str:
    """Account ids are personal data - never print them in full."""
    s = str(value or "")
    return s if len(s) < 7 else f"{s[:3]}***{s[-3:]}"


def domain_for(region: str | None) -> str:
    if region == "CN" or region is None:
        return CN
    if region == "IN":
        return IN
    if region == "RU":
        return RU
    if region in EU_COUNTRIES:
        return EU
    return INTL


def detect_product() -> str:
    try:
        out = subprocess.run(["adb", "shell", "getprop", "ro.product.name"],
                             capture_output=True, text=True, timeout=10)
        product = out.stdout.strip()
        if product:
            return product
    except Exception:
        pass
    return ""


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--product", help="device codename, e.g. thyme (default: ask adb)")
    a = ap.parse_args()

    product = a.product or detect_product()
    if not product:
        sys.exit("unknown product - pass --product (e.g. --product thyme)")

    param = {"sid": "unlockApi", "checkSafeAddress": True}
    pass_token = get_passtoken(param)
    if not pass_token:
        return 1

    service = get_service(pass_token, param)
    if not service:
        return 1

    region = get_region(pass_token)
    domain = domain_for(region)
    cookies = service["cookies"]

    print(f"\nuserId  : {mask(cookies.get('userId'))}")
    print(f"region  : {region}")
    print(f"server  : {domain}")
    print(f"product : {product}\n")

    nonce = _send("/api/v2/nonce",
                  {"r": "".join(random.choices("abcdefghijklmnopqrstuvwxyz", k=16))},
                  domain, service["servicedata"]["ssecurity"], cookies)
    if nonce.get("code") != 0:
        print("nonce request failed:", json.dumps(nonce, ensure_ascii=False, indent=2))
        return 1

    res = _send("/api/v2/unlock/device/clear",
                {"appId": "1", "data": {"product": product}, "nonce": nonce["nonce"]},
                domain, service["servicedata"]["ssecurity"], cookies)

    print(json.dumps(res, ensure_ascii=False, indent=2))

    code = res.get("code")
    hints = {
        0: "请求被接受（未做资格判定）",
        20031: "账号未绑定，先去开发者选项绑定",
        20045: "账号地区与设备地区不匹配",
        10013: "设备激活不足 7 天 / SIM 未激活",
    }
    print(f"\n==> {hints.get(code, res.get('descEN') or res.get('description'))}")
    return 0 if code == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
