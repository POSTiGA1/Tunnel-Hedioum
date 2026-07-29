package main

import (
	"flag"
	"fmt"
	"net"
	"os/exec"
	"strconv"

	"github.com/fatih/color"
	"github.com/hedioum/Hedioum-Pool-Tunnel/config"
	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/sysutil"
)

// cmdSetupForeign writes the foreign (egress) configuration non-interactively.
func cmdSetupForeign(args []string) {
	fs := flag.NewFlagSet("setup-foreign", flag.ExitOnError)
	listenPort := fs.Int("listen-port", 22, "public listen port")
	decoyPort := fs.Int("decoy-port", 2022, "local decoy sshd port")
	egressMode := fs.String("egress-mode", "ipv4", "egress family: ipv4|ipv6|dual")
	bindIP := fs.String("egress-bind-ip", "", "optional egress source IP")
	moveSSH := fs.Bool("move-ssh", false, "relocate OpenSSH to --decoy-port")
	token := fs.String("token", "", "auth token (generated if empty)")
	_ = fs.Parse(args)

	if err := validPort(*listenPort); err != nil {
		fail("--listen-port: %v", err)
	}
	if err := validPort(*decoyPort); err != nil {
		fail("--decoy-port: %v", err)
	}
	switch *egressMode {
	case "ipv4", "ipv6", "dual":
	default:
		fail("--egress-mode must be ipv4, ipv6, or dual")
	}
	if *bindIP != "" {
		if err := validIP(*bindIP); err != nil {
			fail("--egress-bind-ip: %v", err)
		}
	}
	tok := *token
	if tok == "" {
		tok = sysutil.GenerateSecureToken()
	} else if err := validToken(tok); err != nil {
		fail("--token: %v", err)
	}

	if *listenPort != 22 {
		color.Yellow("[!] Listen port %d: the SSH mimic is most convincing on 22; a non-22 port may be easier to fingerprint until port-appropriate mimics land.", *listenPort)
	}
	if *moveSSH {
		if err := sysutil.ChangeSSHPort(strconv.Itoa(*decoyPort)); err != nil {
			color.Red("[x] Failed to relocate OpenSSH to %d: %v", *decoyPort, err)
		} else {
			color.Green("[✓] OpenSSH relocated to %d (decoy).", *decoyPort)
		}
	}

	cfg := &config.AppConfig{
		Role:              "foreign",
		ForeignListenPort: *listenPort,
		DecoyPort:         *decoyPort,
		EgressIPMode:      *egressMode,
		EgressBindIP:      *bindIP,
		AuthToken:         tok,
	}
	if err := config.SaveConfig(cfg); err != nil {
		fail("failed to save config: %v", err)
	}
	color.Green("[✓] Foreign config written (listen %d, decoy %d, egress %s).", *listenPort, *decoyPort, *egressMode)
	fmt.Printf("Auth Token: %s\n", tok)
}

// cmdSetupIran writes the Iran (hub) config with its first node (same flags as add-node).
func cmdSetupIran(args []string) { cmdAddNode(args) }

// cmdAddNode appends a foreign node to the Iran hub config (creating it if absent).
func cmdAddNode(args []string) {
	fs := flag.NewFlagSet("add-node", flag.ExitOnError)
	alias := fs.String("alias", "", "node alias")
	target := fs.String("target", "", "foreign egress HOST:PORT")
	socksPort := fs.Int("socks-port", 0, "local SOCKS5 bind port")
	token := fs.String("token", "", "auth token from the foreign node")
	min := fs.Int("min", 10, "min warm-up connections")
	max := fs.Int("max", 20, "max connections")
	bw := fs.Int("bw", 8, "per-connection bandwidth cap (Mbps)")
	jitter := fs.Int("jitter", 2, "bandwidth jitter (Mbps)")
	_ = fs.Parse(args)

	if *alias == "" {
		fail("--alias is required")
	}
	if err := validTarget(*target); err != nil {
		fail("--target: %v", err)
	}
	if err := validPort(*socksPort); err != nil {
		fail("--socks-port: %v", err)
	}
	if err := validToken(*token); err != nil {
		fail("--token: %v", err)
	}
	host, portStr, _ := net.SplitHostPort(*target)
	tport, _ := strconv.Atoi(portStr)

	cfg, err := config.LoadConfig()
	if err != nil {
		cfg = &config.AppConfig{Role: "iran"}
	}
	if cfg.Role == "" {
		cfg.Role = "iran"
	}
	if cfg.Role != "iran" {
		fail("existing config role is %q, not iran", cfg.Role)
	}

	cfg.UpdateForeignNode(config.ForeignNode{
		Alias:               *alias,
		TargetIP:            host,
		TargetPort:          tport,
		LocalSocksPort:      *socksPort,
		AuthToken:           *token,
		MinConnections:      *min,
		MaxConnections:      *max,
		BandwidthLimitMbps:  *bw,
		BandwidthJitterMbps: *jitter,
	})
	if err := config.SaveConfig(cfg); err != nil {
		fail("failed to save config: %v", err)
	}
	color.Green("[✓] Node %q added (target %s, socks %d).", *alias, *target, *socksPort)
	restartDaemon()
}

// cmdRemoveNode removes a foreign node from the Iran hub config.
func cmdRemoveNode(args []string) {
	fs := flag.NewFlagSet("remove-node", flag.ExitOnError)
	alias := fs.String("alias", "", "node alias to remove")
	_ = fs.Parse(args)

	if *alias == "" {
		fail("--alias is required")
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		fail("failed to load config: %v", err)
	}
	if !cfg.RemoveForeignNode(*alias) {
		fail("node %q not found", *alias)
	}
	if err := config.SaveConfig(cfg); err != nil {
		fail("failed to save config: %v", err)
	}
	color.Green("[✓] Node %q removed.", *alias)
	restartDaemon()
}

// restartDaemon best-effort applies config changes via systemd.
func restartDaemon() {
	if err := exec.Command("systemctl", "restart", "hedioum.service").Run(); err != nil {
		color.Yellow("[-] Could not restart the daemon automatically; apply changes manually.")
	}
}
