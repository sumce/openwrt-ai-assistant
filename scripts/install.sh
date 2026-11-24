#!/bin/sh
# WRT AI quick installer — copies the prebuilt binary, procd init script
# and default UCI config to a router over SSH.
#
# Usage (repo root, after `make cross`):
#   ./scripts/install.sh root@192.168.1.1            # auto-detect arch
#   ./scripts/install.sh root@192.168.1.1 arm64      # force arch dir name
#
# The binary ships all dependencies; nothing else is installed on the
# router. /etc/wrtai holds password, sessions, conversations and backups.

set -e

HOST="${1:?usage: install.sh user@router [arch]}"
ARCH="${2:-}"

REPO="$(cd "$(dirname "$0")/.." && pwd)"

echo "==> detecting router architecture..."
if [ -z "$ARCH" ]; then
	case "$(ssh "$HOST" uname -m)" in
		x86_64)          ARCH=amd64 ;;
		aarch64|arm64)   ARCH=arm64 ;;
		armv7l|armv8l)   ARCH=armv7 ;;
		armv6l)          ARCH=armv6 ;;
		mips)            ARCH=mips ;;
		mipsel)          ARCH=mipsle ;;
		*) echo "unknown arch; pass one of: amd64 arm64 armv7 armv6 mips mipsle" >&2; exit 1 ;;
	esac
fi

BIN="$REPO/dist/linux-$ARCH/wrtai"
[ -f "$BIN" ] || { echo "missing $BIN — run: make cross" >&2; exit 1; }

echo "==> installing $BIN -> $HOST:/usr/bin/wrtai"
scp "$BIN" "$HOST:/tmp/wrtai.new"
ssh "$HOST" "mv /tmp/wrtai.new /usr/bin/wrtai && chmod +x /usr/bin/wrtai"

echo "==> installing init script + default config"
scp "$REPO/openwrt/wrtai/files/wrtai.init" "$HOST:/etc/init.d/wrtai"
ssh "$HOST" "chmod +x /etc/init.d/wrtai"
# never overwrite an existing config (it holds provider API keys)
ssh "$HOST" "test -f /etc/config/wrtai || scp '$REPO/openwrt/wrtai/files/wrtai.uci' '$HOST:/etc/config/wrtai'" 2>/dev/null || \
	scp "$REPO/openwrt/wrtai/files/wrtai.uci" "$HOST:/etc/config/wrtai.new"
ssh "$HOST" "test -f /etc/config/wrtai || mv /etc/config/wrtai.new /etc/config/wrtai"
ssh "$HOST" "mkdir -p /etc/wrtai && grep -q wrtai /etc/sysupgrade.conf 2>/dev/null || echo /etc/wrtai/ >> /etc/sysupgrade.conf"

echo "==> enabling + starting service"
ssh "$HOST" "/etc/init.d/wrtai enable && /etc/init.d/wrtai restart"

PORT=$(ssh "$HOST" ". /lib/functions.sh; config_load wrtai; config_get port main port 8765; echo \$port" 2>/dev/null || echo 8765)

echo
echo "✔ installed. open http://$(echo "$HOST" | sed 's/.*@//'):$PORT/"
echo "  (LuCI → 服务 → WRT AI 控制启用/端口)"
