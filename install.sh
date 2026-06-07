#!/bin/bash

# ==========================================================
# Hedioum Dynamic Pool Tunnel - 1-Click Installer & Updater
# ==========================================================

# 1. Enforce Root Privileges
if [ "$EUID" -ne 0 ]; then
  echo "[x] CRITICAL: Please run the installer as root (e.g., sudo bash install.sh)"
  exit 1
fi

echo "=================================================="
echo "  Deploying Hedioum Stealth Mesh Daemon..."
echo "=================================================="

# 2. Prepare System Directories
# We use /etc/hedioum as the secure working directory for the configuration file
mkdir -p /etc/hedioum
mkdir -p /usr/local/bin

# 3. Download the Latest Optimized Binary
# This fetches the statically compiled Go binary directly from your repository's bin directory
BINARY_URL="https://raw.githubusercontent.com/hedioum/Hedioum-Pool-Tunnel/main/bin/hedioum-tunnel"

echo "[*] Fetching the latest optimized binary..."
curl -L -s -o /usr/local/bin/hedioum-tunnel "$BINARY_URL"

if [ $? -ne 0 ]; then
    echo "[x] ERROR: Failed to download the binary. Check network connectivity or repository URL."
    exit 1
fi

# Make the binary executable across the system
chmod +x /usr/local/bin/hedioum-tunnel
echo "[✓] Binary installed successfully at /usr/local/bin/hedioum-tunnel"

# 4. Provision the Systemd Service
echo "[*] Configuring Systemd background service..."
cat << 'EOF' > /etc/systemd/system/hedioum.service
[Unit]
Description=Hedioum Dynamic Pool Tunnel Daemon
After=network.target

[Service]
Type=simple
User=root
# WorkingDirectory ensures hedioum.json is consistently read/saved here
WorkingDirectory=/etc/hedioum
ExecStart=/usr/local/bin/hedioum-tunnel
Restart=always
RestartSec=5
# Maximum open file descriptors optimized for high-concurrency connection pools
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

# Reload daemon to recognize the new/updated service file
systemctl daemon-reload
systemctl enable hedioum.service > /dev/null 2>&1

# Anti-Socket Activation Patch for Ubuntu 22.04+ / 24.04+
# This ensures Port 22 is completely released from systemd-allocated sockets
if systemctl is-active --quiet ssh.socket; then
    echo "[*] Modern Ubuntu socket activation detected. Disabling ssh.socket to free Port 22..."
    systemctl disable --now ssh.socket > /dev/null 2>&1
    systemctl restart ssh > /dev/null 2>&1
fi

# 5. Bootstrapping & Wizard Handling
echo "=================================================="
if [ ! -f "/etc/hedioum/hedioum.json" ]; then
    echo -e "[!] Fresh installation detected. Launching Initial Setup Wizard..."
    sleep 2

    # Enter the working directory and launch the wizard natively
    cd /etc/hedioum && hedioum-tunnel

    # Once the wizard exits successfully, start the background daemon
    systemctl start hedioum.service
    echo -e "\n[✓] Setup complete! Hedioum Daemon is now running in the background."
else
    # Seamless Update: Restart the service to apply the newly downloaded binary
    echo "[✓] Existing configuration found. Applying seamless update..."
    systemctl restart hedioum.service
    echo "[✓] Hedioum Daemon updated and restarted gracefully."
fi

echo "=================================================="
echo " [Ops] Management Dashboard Command:"
echo " Simply type 'hedioum-tunnel' anywhere in your terminal."
echo "=================================================="