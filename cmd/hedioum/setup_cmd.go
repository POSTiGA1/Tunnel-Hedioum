package main

import (
	"flag"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/hedioum/Hedioum-Pool-Tunnel/config"
	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/sysutil"
)

// expandMimics turns "all" or a comma list into an ordered, validated mimic list.
func expandMimics(spec string) ([]string, error) {
	if spec == "all" {
		return []string{"ssh", "tls"}, nil
	}
	var out []string
	for _, p := range strings.Split(spec, ",") {
		p = strings.TrimSpace(p)
		switch p {
		case "ssh", "tls", "smtp", "imap", "smtps", "imaps":
			out = append(out, p)
		case "":
		default:
			return nil, fmt.Errorf("unknown mimic %q (want ssh|tls|smtp|imap|smtps|imaps|all)", p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no mimics selected")
	}
	return out, nil
}

// speedProfile resolves min/max/bw/jitter defaults for a named throughput profile.
func speedProfile(name string) (min, max, bw, jitter int) {
	switch name {
	case "high-speed":
		return 15, 40, 60, 15 // ~ up to ~40*60 Mbps aggregate
	default: // "balanced" / ""
		return 10, 20, 8, 2
	}
}

// cmdSetupForeign writes the foreign (egress) configuration non-interactively.
func cmdSetupForeign(args []string) {
	fs := flag.NewFlagSet("setup-foreign", flag.ExitOnError)
	sshPort := fs.Int("listen-port", 22, "SSH mimic public port")
	decoyPort := fs.Int("decoy-port", 2022, "local decoy sshd port")
	tlsPort := fs.Int("tls-port", 443, "TLS mimic public port")
	smtpPort := fs.Int("smtp-port", 587, "SMTP (STARTTLS) mimic public port")
	imapPort := fs.Int("imap-port", 143, "IMAP (STARTTLS) mimic public port")
	smtpsPort := fs.Int("smtps-port", 465, "SMTPS (implicit TLS) mimic public port")
	imapsPort := fs.Int("imaps-port", 993, "IMAPS (implicit TLS) mimic public port")
	tlsServerName := fs.String("tls-servername", "", "TLS SNI/CN (optional)")
	mimics := fs.String("mimics", "ssh", "camouflage: ssh|tls|smtp|imap|smtps|imaps|all or a comma list")
	egressMode := fs.String("egress-mode", "ipv4", "egress family: ipv4|ipv6|dual")
	bindIP := fs.String("egress-bind-ip", "", "optional egress source IP")
	moveSSH := fs.Bool("move-ssh", false, "relocate OpenSSH to --decoy-port")
	token := fs.String("token", "", "auth token (generated if empty)")
	_ = fs.Parse(args)

	for label, p := range map[string]int{"listen-port": *sshPort, "decoy-port": *decoyPort, "tls-port": *tlsPort, "smtp-port": *smtpPort, "imap-port": *imapPort, "smtps-port": *smtpsPort, "imaps-port": *imapsPort} {
		if err := validPort(p); err != nil {
			fail("--%s: %v", label, err)
		}
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
	types, err := expandMimics(*mimics)
	if err != nil {
		fail("--mimics: %v", err)
	}
	tok := *token
	if tok == "" {
		tok = sysutil.GenerateSecureToken()
	} else if err := validToken(tok); err != nil {
		fail("--token: %v", err)
	}

	var mimicList []config.MimicListener
	for _, ty := range types {
		switch ty {
		case "ssh":
			if *sshPort != 22 {
				color.Yellow("[!] SSH mimic on port %d: most convincing on 22.", *sshPort)
			}
			mimicList = append(mimicList, config.MimicListener{Type: "ssh", Port: *sshPort, Decoy: fmt.Sprintf("127.0.0.1:%d", *decoyPort)})
		case "tls":
			mimicList = append(mimicList, config.MimicListener{Type: "tls", Port: *tlsPort, ServerName: *tlsServerName})
		case "smtp":
			mimicList = append(mimicList, config.MimicListener{Type: "smtp", Port: *smtpPort, ServerName: *tlsServerName})
		case "imap":
			mimicList = append(mimicList, config.MimicListener{Type: "imap", Port: *imapPort, ServerName: *tlsServerName})
		case "smtps":
			mimicList = append(mimicList, config.MimicListener{Type: "smtps", Port: *smtpsPort, ServerName: *tlsServerName})
		case "imaps":
			mimicList = append(mimicList, config.MimicListener{Type: "imaps", Port: *imapsPort, ServerName: *tlsServerName})
		}
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
		ForeignListenPort: *sshPort,
		DecoyPort:         *decoyPort,
		EgressIPMode:      *egressMode,
		EgressBindIP:      *bindIP,
		AuthToken:         tok,
		Mimics:            mimicList,
	}
	if err := config.SaveConfig(cfg); err != nil {
		fail("failed to save config: %v", err)
	}
	color.Green("[✓] Foreign config written (mimics: %s, egress %s).", strings.Join(types, ","), *egressMode)
	fmt.Printf("Auth Token: %s\n", tok)
}

// cmdSetupIran writes the Iran (hub) config with its first node.
func cmdSetupIran(args []string) { cmdAddNode(args) }

// cmdAddNode appends a foreign node (one SOCKS port + endpoints) to the hub config.
func cmdAddNode(args []string) {
	fs := flag.NewFlagSet("add-node", flag.ExitOnError)
	alias := fs.String("alias", "", "node alias")
	targetIP := fs.String("target-ip", "", "foreign IP (with --mimics)")
	target := fs.String("target", "", "single foreign HOST:PORT (legacy, SSH)")
	mimics := fs.String("mimics", "", "endpoints: ssh|tls|all (needs --target-ip)")
	sshPort := fs.Int("ssh-port", 22, "foreign SSH mimic port")
	tlsPort := fs.Int("tls-port", 443, "foreign TLS mimic port")
	smtpPort := fs.Int("smtp-port", 587, "foreign SMTP (STARTTLS) mimic port")
	imapPort := fs.Int("imap-port", 143, "foreign IMAP (STARTTLS) mimic port")
	smtpsPort := fs.Int("smtps-port", 465, "foreign SMTPS (implicit TLS) mimic port")
	imapsPort := fs.Int("imaps-port", 993, "foreign IMAPS (implicit TLS) mimic port")
	tlsServerName := fs.String("tls-servername", "", "TLS SNI")
	socksPort := fs.Int("socks-port", 0, "local SOCKS5 bind port")
	token := fs.String("token", "", "auth token from the foreign node")
	profile := fs.String("profile", "balanced", "throughput profile: balanced|high-speed")
	min := fs.Int("min", 0, "min warm-up connections (0 = profile)")
	max := fs.Int("max", 0, "max connections (0 = profile)")
	bw := fs.Int("bw", 0, "per-connection Mbps cap (0 = profile)")
	jitter := fs.Int("jitter", -1, "bandwidth jitter Mbps (-1 = profile)")
	_ = fs.Parse(args)

	if *alias == "" {
		fail("--alias is required")
	}
	if err := validPort(*socksPort); err != nil {
		fail("--socks-port: %v", err)
	}
	if err := validToken(*token); err != nil {
		fail("--token: %v", err)
	}

	var endpoints []config.Endpoint
	switch {
	case *mimics != "":
		if *targetIP == "" {
			fail("--mimics requires --target-ip")
		}
		types, err := expandMimics(*mimics)
		if err != nil {
			fail("--mimics: %v", err)
		}
		for label, p := range map[string]int{"ssh-port": *sshPort, "tls-port": *tlsPort, "smtp-port": *smtpPort, "imap-port": *imapPort, "smtps-port": *smtpsPort, "imaps-port": *imapsPort} {
			if err := validPort(p); err != nil {
				fail("--%s: %v", label, err)
			}
		}
		portFor := map[string]int{"ssh": *sshPort, "tls": *tlsPort, "smtp": *smtpPort, "imap": *imapPort, "smtps": *smtpsPort, "imaps": *imapsPort}
		for _, ty := range types {
			sni := ""
			if ty != "ssh" {
				sni = *tlsServerName
			}
			endpoints = append(endpoints, config.Endpoint{
				Target:     net.JoinHostPort(*targetIP, strconv.Itoa(portFor[ty])),
				Mimic:      ty,
				ServerName: sni,
			})
		}
	case *target != "":
		if err := validTarget(*target); err != nil {
			fail("--target: %v", err)
		}
		endpoints = []config.Endpoint{{Target: *target, Mimic: "ssh"}}
	default:
		fail("provide --target-ip --mimics ... or --target HOST:PORT")
	}

	pMin, pMax, pBw, pJit := speedProfile(*profile)
	if *min > 0 {
		pMin = *min
	}
	if *max > 0 {
		pMax = *max
	}
	if *bw > 0 {
		pBw = *bw
	}
	if *jitter >= 0 {
		pJit = *jitter
	}

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
		LocalSocksPort:      *socksPort,
		AuthToken:           *token,
		Endpoints:           endpoints,
		MinConnections:      pMin,
		MaxConnections:      pMax,
		BandwidthLimitMbps:  pBw,
		BandwidthJitterMbps: pJit,
	})
	if err := config.SaveConfig(cfg); err != nil {
		fail("failed to save config: %v", err)
	}
	labels := make([]string, len(endpoints))
	for i, e := range endpoints {
		labels[i] = e.Mimic + "@" + e.Target
	}
	color.Green("[✓] Node %q added (socks %d, endpoints: %s).", *alias, *socksPort, strings.Join(labels, ", "))
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
