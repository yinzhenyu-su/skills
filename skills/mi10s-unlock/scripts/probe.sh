#!/bin/bash
# Dump everything needed to decide the next step for a Xiaomi device.
#
#   scripts/probe.sh
#
# ADB=/path/to/adb scripts/probe.sh    # use a specific adb binary

set -uo pipefail

ADB=${ADB:-adb}

command -v "$ADB" >/dev/null 2>&1 || { echo "adb not found (set ADB=/path/to/adb)" >&2; exit 1; }

if ! "$ADB" get-state >/dev/null 2>&1; then
  echo "no device: enable USB debugging and accept the RSA prompt" >&2
  "$ADB" devices -l >&2
  exit 1
fi

g() { "$ADB" shell getprop "$1" 2>/dev/null | tr -d '\r'; }

product=$(g ro.product.name)
model=$(g ro.product.model)
build=$(g ro.build.version.incremental)
android=$(g ro.build.version.release)
patch=$(g ro.build.version.security_patch)
locked=$(g ro.boot.flash.locked)
vbs=$(g ro.boot.verifiedbootstate)
debuggable=$(g ro.debuggable)
kernel=$(g ro.kernel.version)
miui=$(g ro.miui.ui.version.name)

has_init_boot=no
"$ADB" shell "ls /dev/block/by-name/init_boot_a" >/dev/null 2>&1 && has_init_boot=yes

have_su=no
"$ADB" shell "which su" 2>/dev/null | grep -q . && have_su=yes

printf '%-16s %s\n' \
  "product"      "$product" \
  "model"        "$model" \
  "ROM"          "$build" \
  "Android"      "$android (patch $patch)" \
  "MIUI/HyperOS" "$miui" \
  "kernel"       "${kernel:-unknown}" \
  "flash.locked" "$locked  (0 = bootloader unlocked)" \
  "verifiedboot" "$vbs" \
  "ro.debuggable" "$debuggable" \
  "init_boot"    "$has_init_boot  (magisk target: $([[ $has_init_boot == yes ]] && echo init_boot || echo boot))" \
  "su present"   "$have_su"

echo
case "$locked" in
  0) echo "=> bootloader unlocked, skip to root" ;;
  1) echo "=> bootloader locked, unlock first" ;;
  *) echo "=> unknown lock state (device may be in a non-adb mode)" ;;
esac

if [ "$have_su" = yes ]; then
  echo "=> root already present: $("$ADB" shell su -v 2>/dev/null | tr -d '\r')"
fi
