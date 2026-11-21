#!/bin/bash
# Flash Magisk to a Xiaomi device whose bootloader is already unlocked.
#
#   scripts/root-magisk.sh --prep --rom <fastboot-rom.tgz> [--md5 <hex>] [--magisk <apk>]
#   scripts/root-magisk.sh --flash
#   scripts/root-magisk.sh --verify
#   scripts/root-magisk.sh --restore
#
# The patch step itself must happen in the Magisk app on the phone:
#   Magisk -> 安装 -> 选择并修补一个文件 -> Download/<part>.img
#   output: Download/magisk_patched-*.img
#
# ROM build and device build are compared before anything is written: a boot
# image from a different build is the most common way to soft-brick this.

set -euo pipefail

ADB=${ADB:-adb}
FASTBOOT=${FASTBOOT:-fastboot}
DIR=${WORKDIR:-$HOME/xiaomi-unlock}

ROM=""
ROM_MD5=""
APK=""
MODE=""

while [ $# -gt 0 ]; do
  case "$1" in
    --prep|--flash|--verify|--restore) MODE="${1#--}" ;;
    --rom)    ROM="$2"; shift ;;
    --md5)    ROM_MD5="$2"; shift ;;
    --magisk) APK="$2"; shift ;;
    --dir)    DIR="$2"; shift ;;
    -h|--help) sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
  shift
done

say() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
die() { printf '\033[31m!! %s\033[0m\n' "$*" >&2; exit 1; }

g() { "$ADB" shell getprop "$1" 2>/dev/null | tr -d '\r'; }

require_device() {
  [ "$("$ADB" get-state 2>/dev/null)" = "device" ] || die "no adb device"
}

# V816.0.4.0.TGACNXM (build prop) and OS1.0.4.0.TGACNXM (rom filename) are the same build:
# "V816." and "OS1." are the HyperOS prefixes, everything after them must match
norm() { echo "$1" | sed -e 's/^V816\.//' -e 's/^OS1\.//'; }

detect_part() {
  if "$ADB" shell "ls /dev/block/by-name/init_boot_a" >/dev/null 2>&1; then
    echo init_boot
  else
    echo boot
  fi
}

prep() {
  require_device
  [ -n "$ROM" ] || die "--prep needs --rom <fastboot-rom.tgz>"
  [ -f "$ROM" ] || die "no such file: $ROM"
  mkdir -p "$DIR"

  # a Xiaomi fastboot rom is one top-level dir: <codename>_images_<version>_<android>/
  local prefix rom_build build
  prefix=$(tar -tzf "$ROM" 2>/dev/null | head -1 | cut -d/ -f1)
  rom_build=$(echo "$prefix" | grep -oE '(OS|V)[0-9.]+[A-Z]+XM' | head -1 || true)
  build=$(g ro.build.version.incremental)

  say "build check"
  echo "device : $build"
  echo "rom    : ${rom_build:-unknown}  ($prefix)"
  if [ -z "$rom_build" ]; then
    die "cannot read the build version from the archive - refusing to guess"
  fi
  [ "$(norm "$build")" = "$(norm "$rom_build")" ] \
    || die "ROM does not match the device build - flashing it would brick the boot image"

  if [ -n "$ROM_MD5" ]; then
    say "md5"
    local got
    got=$(md5 -q "$ROM" 2>/dev/null || md5sum "$ROM" | cut -d' ' -f1)
    echo "$got"
    [ "$got" = "$ROM_MD5" ] || die "md5 mismatch (expected $ROM_MD5)"
  fi

  local part
  part=$(detect_part)
  say "extracting $prefix/images/$part.img"
  tar -xzf "$ROM" -C "$DIR" "$prefix/images/$part.img"
  mkdir -p "$DIR/images"
  mv -f "$DIR/$prefix/images/$part.img" "$DIR/images/$part.img"
  rm -rf "$DIR/$prefix"
  ls -l "$DIR/images/$part.img"
  shasum -a 256 "$DIR/images/$part.img" 2>/dev/null || sha256sum "$DIR/images/$part.img"

  if [ -z "$APK" ]; then
    APK=$(ls -t "$DIR"/Magisk*.apk "$HOME"/Downloads/Magisk*.apk 2>/dev/null | head -1 || true)
  fi
  if [ -n "$APK" ] && [ -f "$APK" ]; then
    say "installing $(basename "$APK")"
    "$ADB" install -r "$APK" | tail -1
  else
    echo "!! no Magisk apk found, install it manually" >&2
  fi

  say "pushing stock $part.img"
  "$ADB" push "$DIR/images/$part.img" "/sdcard/Download/$part.img" | tail -1

  cat <<EOF

next, on the phone:
  1. open Magisk
  2. 安装 -> 选择并修补一个文件
  3. pick Download/$part.img
  4. wait for "All done", remember the output file name

then run:  $0 --flash
EOF
}

flash() {
  require_device
  local part patched
  part=$(detect_part)

  patched=$(ls -t "$DIR"/magisk_patched*.img 2>/dev/null | head -1 || true)
  if [ -z "$patched" ]; then
    say "looking for the patched image on the phone"
    local remote
    remote=$("$ADB" shell "ls -t /sdcard/Download/magisk_patched*.img 2>/dev/null | head -1" | tr -d '\r')
    [ -n "$remote" ] || die "no magisk_patched*.img found - did the Magisk patch finish?"
    "$ADB" pull "$remote" "$DIR/" | tail -1
    patched=$(ls -t "$DIR"/magisk_patched*.img | head -1)
  fi

  say "patched image"
  echo "$patched"
  shasum -a 256 "$patched" 2>/dev/null || sha256sum "$patched"

  local locked
  locked=$(g ro.boot.flash.locked)
  [ "$locked" = "0" ] || die "bootloader is locked ($locked) - flash would fail"

  say "rebooting to fastboot"
  "$ADB" reboot bootloader
  sleep 8
  "$FASTBOOT" getvar product 2>&1 | grep -q product || die "device did not enter fastboot"

  say "flashing $part"
  "$FASTBOOT" flash "$part" "$patched"
  "$FASTBOOT" reboot
  echo "done - device is rebooting"
}

verify() {
  require_device
  say "waiting for boot"
  for _ in $(seq 60); do
    [ "$(g sys.boot_completed)" = "1" ] && break
    sleep 2
  done
  say "root"
  local v
  v=$("$ADB" shell su -v 2>/dev/null | tr -d '\r' || true)
  [ -n "$v" ] || die "no su - root is not active"
  echo "su version : $v"
  "$ADB" shell "su -c id" | tr -d '\r'
}

restore() {
  require_device
  local part
  part=$(detect_part)
  [ -f "$DIR/images/$part.img" ] || die "no stock image at $DIR/images/$part.img (run --prep --rom ... first)"

  say "restoring stock $part"
  "$ADB" reboot bootloader
  sleep 8
  "$FASTBOOT" flash "$part" "$DIR/images/$part.img"
  "$FASTBOOT" reboot
}

case "$MODE" in
  prep) prep ;;
  flash) flash ;;
  verify) verify ;;
  restore) restore ;;
  *) sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//' ;;
esac
