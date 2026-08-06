package sysutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/firewall"
)

// GetPublicIPv4 safely resolves the server's public IPv4 address, forcing v4 transport
func GetPublicIPv4() (string, error) {
	// Force IPv4 dialer to prevent IPv6 leakage
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		DualStack: false,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp4", addr)
		},
	}

	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}

	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(ip), nil
}

// GetPublicIPv6 resolves the server's public IPv6 address, forcing v6 transport.
// Returns an error (not a value) when the host has no usable global IPv6.
func GetPublicIPv6() (string, error) {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp6", addr)
		},
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport}

	resp, err := client.Get("https://api6.ipify.org")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(ip)), nil
}

// ChangeSSHPort edits sshd_config safely, disables ssh.socket if present, updates UFW, and restarts the service
func ChangeSSHPort(newPort string) error {
	const sshdConfigPath = "/etc/ssh/sshd_config"
	backupPath := fmt.Sprintf("%s.bak.%d", sshdConfigPath, time.Now().Unix())

	// 1. Read existing config
	content, err := os.ReadFile(sshdConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read sshd_config: %w", err)
	}

	// 2. Create backup
	if err := os.WriteFile(backupPath, content, 0644); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// 3. Regex replacement for Port directive
	configStr := string(content)
	re := regexp.MustCompile(`(?m)^#?Port\s+\d+`)
	if re.MatchString(configStr) {
		configStr = re.ReplaceAllString(configStr, "Port "+newPort)
	} else {
		// If no port directive exists, append it
		configStr += fmt.Sprintf("\nPort %s\n", newPort)
	}

	// 4. Write new config
	if err := os.WriteFile(sshdConfigPath, []byte(configStr), 0644); err != nil {
		return fmt.Errorf("failed to write new sshd_config: %w", err)
	}

	// 5. Handle systemd ssh.socket overriding port 22 in modern Ubuntu systems
	_ = exec.Command("systemctl", "stop", "ssh.socket").Run()
	_ = exec.Command("systemctl", "disable", "ssh.socket").Run()

	// CRITICAL FIX: Ensure the actual SSH service is enabled for the next system reboot
	_ = exec.Command("systemctl", "enable", "ssh.service").Run()
	_ = exec.Command("systemctl", "enable", "sshd.service").Run()

	// 6. Open the new SSH (decoy) port on whatever firewall the host runs — ufw,
	// firewalld, or iptables/ip6tables — so the admin is not locked out on non-ufw
	// distros. This is the safety path to reach the box if the mimic ever stops.
	if p, convErr := strconv.Atoi(newPort); convErr == nil {
		switch backend, fwErr := firewall.EnsurePortOpen(p); {
		case fwErr != nil:
			color.Red("[!] Warning: could not open %s/tcp via %s automatically: %v", newPort, backend, fwErr)
			color.Red("    Please allow %s/tcp manually so you can still reach SSH.", newPort)
		case backend == "none":
			color.HiBlack("[i] No active host firewall; %s/tcp needs no rule.", newPort)
		default:
			color.Green("[✓] Opened %s/tcp via %s.", newPort, backend)
		}
	}

	// 7. Restart SSH service (handles both 'ssh' in Ubuntu and 'sshd' in RHEL)
	cmd := exec.Command("systemctl", "restart", "ssh")
	if err := cmd.Run(); err != nil {
		// Fallback for CentOS/AlmaLinux
		exec.Command("systemctl", "restart", "sshd").Run()
	}

	// Give the SSH daemon a second to bind
	time.Sleep(1 * time.Second)

	return nil
}

// isUFWActive checks if the Uncomplicated Firewall is running
func isUFWActive() bool {
	out, err := exec.Command("ufw", "status").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "Status: active")
}

// GenerateSecureToken creates a 32-character random hex string for authentication
func GenerateSecureToken() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "fallback-secure-token-12345"
	}
	return hex.EncodeToString(bytes)
}
