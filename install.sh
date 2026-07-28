#!/bin/bash
# ==========================================================
# Hedioum Dynamic Pool Tunnel - minimal bootstrap
#
# This script only: (1) checks the CPU architecture, (2) downloads the matching
# release binary from GitHub, (3) runs it. The BINARY itself does everything else
# (self-copy to /usr/local/bin, systemd service, firewall, config), so installing
# via this script and running the binary by hand are equivalent.
# ==========================================================
set -euo pipefail

if [ "${EUID:-$(id -u)}" -ne 0 ]; then
  echo "[x] Please run as root (e.g. sudo bash install.sh)"
  exit 1
fi

case "$(uname -m)" in
  aarch64 | arm64) ASSET="hedioum-tunnel-arm64" ;;
  *) ASSET="hedioum-tunnel" ;;
esac
echo "[*] Architecture asset: $ASSET"

URL="https://github.com/hedioum/Hedioum-Pool-Tunnel/releases/latest/download/${ASSET}"
TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

echo "[*] Downloading the latest release from GitHub..."
ok=""
for attempt in 1 2 3; do
  if curl -fL --connect-timeout 15 -o "$TMP" "$URL"; then ok=1; break; fi
  echo "[-] Attempt $attempt failed; retrying..."
  sleep 2
done

if [ -z "$ok" ]; then
  cat <<EOF
[x] Download failed. GitHub may be blocked on this network.
    Download '${ASSET}' manually on another machine, copy it to this server, then:
        chmod +x ${ASSET} && ./${ASSET} install
    and configure with 'hedioum-tunnel' (wizard) or the setup-* subcommands.
EOF
  exit 1
fi

chmod +x "$TMP"

echo "[*] Installing (the binary sets up the service, firewall, and config paths)..."
"$TMP" install

echo "[*] Launching interactive setup..."
exec hedioum-tunnel
