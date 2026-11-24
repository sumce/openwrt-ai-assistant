#!/bin/sh
# Build an OpenWrt 25.12+ .apk package (apk-tools v3) from a prebuilt
# wrtai binary — same invocation the OpenWrt buildroot itself uses.
#
# usage: mkapk.sh <binary> <arch> <version> <out-dir> [path-to-apk-tool]
#   <arch> is the OpenWrt package arch, e.g. x86_64, aarch64_generic,
#   mips_24kc, mipsel_24kc
#   [path-to-apk-tool] defaults to `apk` on PATH; for CI/PC use Alpine's
#   apk-tools-static (sbin/apk.static)
#
# Requires fakeroot (files are staged root-owned, like the buildroot does).
set -e

BIN="${1:?usage: mkapk.sh <binary> <arch> <version> <out-dir> [apk-tool]}"
ARCH="${2:?missing arch}"
VER="${3:?missing version}"
OUT="${4:?missing out dir}"
APK="${5:-apk}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

[ -f "$BIN" ] || { echo "binary not found: $BIN" >&2; exit 1; }
[ -x "$APK" ] || command -v "$APK" >/dev/null 2>&1 || { echo "apk tool not found: $APK" >&2; exit 1; }
mkdir -p "$OUT"

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
install -Dm755 "$BIN" "$STAGE/usr/bin/wrtai"
install -Dm755 "$ROOT/openwrt/wrtai/files/wrtai.init" "$STAGE/etc/init.d/wrtai"
install -Dm644 "$ROOT/openwrt/wrtai/files/wrtai.uci" "$STAGE/etc/config/wrtai"
install -dm755 "$STAGE/etc/wrtai"

# Unsigned package: install with `apk add --allow-untrusted`.
# Ownership matters (APKv3 records uid/gid): run as real root when
# possible; fakeroot only works for dynamically linked apk builds.
if [ "$(id -u)" = "0" ]; then
  WRAP=""
else
  WRAP="fakeroot"
fi
SOURCE_DATE_EPOCH=0 $WRAP "$APK" mkpkg \
  --info "name:wrtai" \
  --info "version:${VER}-r1" \
  --info "description:WRT AI - AI assistant that operates your OpenWRT router (status/logs/diagnostics/UCI config via chat)" \
  --info "arch:${ARCH}" \
  --info "license:MIT" \
  --info "origin:https://github.com/sumce/openwrt-ai-assistant" \
  --info "url:https://github.com/sumce/openwrt-ai-assistant" \
  --info "maintainer:sumce" \
  --files "$STAGE" \
  --output "$OUT/wrtai-${ARCH}-${VER}.apk"

echo "built $OUT/wrtai-${ARCH}-${VER}.apk"
