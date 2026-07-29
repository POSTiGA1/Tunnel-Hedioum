package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/fatih/color"
)

// runSubcommand dispatches a non-flag first argument to a management command.
// The no-argument invocation (dashboard on a TTY / daemon under systemd) is
// handled by main() and never reaches here.
func runSubcommand(name string, args []string) {
	switch name {
	case "version":
		printVersion()
	case "setup-foreign":
		cmdSetupForeign(args)
	case "setup-iran":
		cmdSetupIran(args)
	case "add-node":
		cmdAddNode(args)
	case "remove-node":
		cmdRemoveNode(args)
	case "install":
		cmdInstall(args)
	case "uninstall":
		cmdUninstall(args)
	case "update":
		cmdUpdate(args)
	case "speedtest":
		cmdSpeedtest(args)
	case "help", "-h", "--help":
		printUsage()
	default:
		color.Red("[x] Unknown command %q", name)
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Print(`Hedioum Dynamic Pool Tunnel

Usage:
  hedioum-tunnel                      Interactive dashboard (TTY) or daemon (systemd)
  hedioum-tunnel version              Print version and build info
  hedioum-tunnel install              Install/enable the systemd service (self-copy)
  hedioum-tunnel uninstall [--yes]    Stop, disable, and remove everything
  hedioum-tunnel update [--file PATH]  Update from GitHub, or from a local binary

  hedioum-tunnel setup-foreign [flags]   Write the foreign (egress) config
      --listen-port N (default 22)  --decoy-port N (default 2022)
      --egress-mode ipv4|ipv6|dual  --egress-bind-ip IP  --move-ssh  --token HEX

  hedioum-tunnel setup-iran   [flags]    Write the Iran (hub) config with one node
  hedioum-tunnel add-node     [flags]    Append a foreign node to the hub config
      --alias NAME  --target HOST:PORT  --socks-port N  --token HEX
      [--min N --max N --bw N --jitter N]
  hedioum-tunnel remove-node --alias NAME
  hedioum-tunnel speedtest [--node NAME] [--mimic ssh|tls] [--seconds N] [--dir down|up|both]

Flags for the default mode:
  --reset            Wipe the config and re-run the setup wizard
  --open-firewall    Open the listen port on the host firewall and exit
`)
}

// --- validation helpers (shared with the wizard) ---

func validPort(p int) error {
	if p < 1 || p > 65535 {
		return fmt.Errorf("port %d out of range (1..65535)", p)
	}
	return nil
}

func validIP(s string) error {
	if net.ParseIP(s) == nil {
		return fmt.Errorf("invalid IP address %q", s)
	}
	return nil
}

// validTarget checks a HOST:PORT where HOST is an IPv4/IPv6 literal or hostname.
func validTarget(s string) error {
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		return fmt.Errorf("target must be HOST:PORT: %w", err)
	}
	if host == "" {
		return fmt.Errorf("target host is empty")
	}
	if net.ParseIP(host) == nil && strings.ContainsAny(host, " \t/\\") {
		return fmt.Errorf("invalid target host %q", host)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid target port %q", portStr)
	}
	return validPort(port)
}

// validToken requires a non-trivial hex string (the tokens we generate are 32 hex).
func validToken(s string) error {
	if len(s) < 8 {
		return fmt.Errorf("token too short (need >= 8 chars)")
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return fmt.Errorf("token must be hexadecimal")
		}
	}
	return nil
}

// fail prints an error and exits non-zero (for CLI command handlers).
func fail(format string, a ...interface{}) {
	color.Red("[x] "+format, a...)
	os.Exit(1)
}
