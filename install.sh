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

# --- FIX: Dynamic Release Downloader (GitHub API) ---
echo "[*] Fetching the latest release from GitHub..."

# Use GitHub API to find the download URL of the latest release asset
LATEST_URL=$(curl -s https://api.github.com/repos/hedioum/Hedioum-Pool-Tunnel/releases/latest | grep "browser_download_url" | grep "hedioum-tunnel" | cut -d '"' -f 4)

# Fallback URLs in case GitHub API is rate-limited or blocked
if [ -z "$LATEST_URL" ]; then
    echo "[-] GitHub API rate-limited or blocked. Falling back to direct raw link..."
    LATEST_URL="https://raw.githubusercontent.com/hedioum/Hedioum-Pool-Tunnel/main/bin/hedioum-tunnel"
fi

URL_PROXY="https://ghp.ci/$LATEST_URL"

if curl -f -L -s -o /usr/local/bin/hedioum-tunnel "$LATEST_URL"; then
    echo "[✓] Binary downloaded successfully (Direct Release)."
elif curl -f -L -s -o /usr/local/bin/hedioum-tunnel "$URL_PROXY"; then
    echo "[✓] Binary downloaded successfully (Proxy Fallback for Iran Hub)."
else
    echo "[x] ERROR: Failed to download the binary. Network is severely restricted."
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