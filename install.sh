#!/bin/bash

# ==========================================================
# Hedioum Dynamic Pool Tunnel - 1-Click Installer & Updater
# ==========================================================

if [ "$EUID" -ne 0 ]; then
  echo "[x] CRITICAL: Please run the installer as root (e.g., sudo bash install.sh)"
  exit 1
fi

echo "=================================================="
echo "  Deploying Hedioum Stealth Mesh Daemon..."
echo "=================================================="

mkdir -p /etc/hedioum
mkdir -p /usr/local/bin

# --- FIX: Stop service and unlink binary to prevent 'Text file busy' error ---
if systemctl is-active --quiet hedioum.service; then
    echo "[*] Stopping existing daemon to apply update..."
    systemctl stop hedioum.service > /dev/null 2>&1
fi
rm -f /usr/local/bin/hedioum-tunnel
# -----------------------------------------------------------------------------

# --- FIX: Anti-Censorship Download mechanism (Direct + Proxy Fallback) ---
echo "[*] Fetching the latest optimized binary..."

URL_DIRECT="https://raw.githubusercontent.com/hedioum/Hedioum-Pool-Tunnel/main/bin/hedioum-tunnel"
URL_PROXY="https://ghp.ci/https://raw.githubusercontent.com/hedioum/Hedioum-Pool-Tunnel/main/bin/hedioum-tunnel"

if curl -f -L -s -o /usr/local/bin/hedioum-tunnel "$URL_DIRECT"; then
    echo "[✓] Binary downloaded successfully (Direct)."
elif curl -f -L -s -o /usr/local/bin/hedioum-tunnel "$URL_PROXY"; then
    echo "[✓] Binary downloaded successfully (Proxy Fallback for Iran Hub)."
else
    echo "[x] ERROR: Failed to download the binary from all sources. Network is severely restricted."
    exit 1
fi

chmod +x /usr/local/bin/hedioum-tunnel
# -------------------------------------------------------------------------

echo "[*] Configuring Systemd background service..."
cat << 'EOF' > /etc/systemd/system/hedioum.service
[Unit]
Description=Hedioum Dynamic Pool Tunnel Daemon
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/etc/hedioum
ExecStart=/usr/local/bin/hedioum-tunnel
Restart=always
RestartSec=5
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload

# --- FIX: Anti-Socket Activation Patch for Ubuntu 22.04+ to free Port 22 ---
if systemctl is-active --quiet ssh.socket; then
    systemctl disable --now ssh.socket > /dev/null 2>&1
    systemctl restart ssh > /dev/null 2>&1
fi
# ---------------------------------------------------------------------------

systemctl enable hedioum.service > /dev/null 2>&1

echo "=================================================="
if [ ! -f "/etc/hedioum/hedioum.json" ]; then
    echo -e "[!] Fresh installation detected. Launching Initial Setup Wizard..."
    sleep 2
    cd /etc/hedioum && hedioum-tunnel
    systemctl start hedioum.service
    echo -e "\n[✓] Setup complete! Hedioum Daemon is now running in the background."
else
    echo "[✓] Existing configuration found. Applying seamless update..."
    systemctl restart hedioum.service
    echo "[✓] Hedioum Daemon updated and restarted gracefully."
fi

echo "=================================================="
echo " [Ops] Management Dashboard Command:"
echo " Simply type 'hedioum-tunnel' anywhere in your terminal."
echo "=================================================="