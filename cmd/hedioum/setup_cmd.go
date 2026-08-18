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
	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/pairing"
	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/persona"
	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/sysutil"
)

// expandMimics turns "all" or a comma list into an ordered, validated mimic list.
// "all" means the whole arsenal — the same as the interactive wizard's "all"
// checkbox, so CLI and wizard agree (a mismatch left the two ends of a link on
// different mimic sets).
// mimicTypesAll is the whole implemented arsenal, in a stable order. "all" expands
// to this; the wizard and validators share it so CLI and wizard never disagree.
var mimicTypesAll = []string{
	"ssh", "tls", "https-alt", "smtp", "imap", "smtps", "imaps",
	"directadmin", "docker", "grafana", "prometheus",
	"cpanel", "whm", "webmail", "postgres", "mysql",
}

func validMimicType(t string) bool {
	for _, m := range mimicTypesAll {
		if m == t {
			return true
		}
	}
	return false
}

func expandMimics(spec string) ([]string, error) {
	if spec == "all" {
		out := make([]string, len(mimicTypesAll))
		copy(out, mimicTypesAll)
		return out, nil
	}
	var out []string
	for _, p := range strings.Split(spec, ",") {
		p = strings.TrimSpace(p)
		switch {
		case p == "":
		case validMimicType(p):
			out = append(out, p)
		default:
			return nil, fmt.Errorf("unknown mimic %q (want one of %s|all)", p, strings.Join(mimicTypesAll, "|"))
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
	daPort := fs.Int("directadmin-port", 2222, "DirectAdmin panel mimic port")
	httpsAltPort := fs.Int("https-alt-port", 8443, "alt-HTTPS (implicit TLS) mimic port")
	dockerPort := fs.Int("docker-port", 5000, "Docker Registry (implicit TLS) mimic port")
	grafanaPort := fs.Int("grafana-port", 3000, "Grafana (implicit TLS) mimic port")
	promPort := fs.Int("prometheus-port", 9090, "Prometheus (implicit TLS) mimic port")
	cpanelPort := fs.Int("cpanel-port", 2083, "cPanel (implicit TLS) mimic port")
	whmPort := fs.Int("whm-port", 2087, "WHM (implicit TLS) mimic port")
	webmailPort := fs.Int("webmail-port", 2096, "Webmail (implicit TLS) mimic port")
	pgPort := fs.Int("postgres-port", 5432, "PostgreSQL (STARTTLS) mimic port")
	mysqlPort := fs.Int("mysql-port", 3306, "MySQL/MariaDB (STARTTLS) mimic port")
	tlsServerName := fs.String("tls-servername", "", "TLS SNI/CN (optional)")
	personaFlag := fs.String("persona", "auto", "server persona: auto|cpanel|directadmin|devops (picks a coherent SSH+9 mimic set)")
	mimics := fs.String("mimics", "", "explicit camouflage set (overrides --persona): comma list or 'all'")
	domain := fs.String("domain", "", "real domain for a Let's Encrypt cert (recommended; empty = self-signed)")
	acmeEmail := fs.String("acme-email", "", "Let's Encrypt account email (optional)")
	decoyStyle := fs.String("decoy-style", "apache", "camouflage persona: apache|directadmin")
	egressMode := fs.String("egress-mode", "ipv4", "egress family: ipv4|ipv6|dual")
	bindIP := fs.String("egress-bind-ip", "", "optional egress source IP")
	moveSSH := fs.Bool("move-ssh", false, "relocate OpenSSH to --decoy-port")
	httpDecoyPort := fs.Int("http-decoy-port", 80, "plaintext web decoy port (0 to disable)")
	token := fs.String("token", "", "auth token (generated if empty)")
	publicIP := fs.String("public-ip", "", "public IP to embed in the pairing token (auto-detected if empty)")
	_ = fs.Parse(args)

	switch *decoyStyle {
	case "apache", "directadmin":
	default:
		fail("--decoy-style must be apache or directadmin")
	}

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
	tok := *token
	if tok == "" {
		tok = sysutil.GenerateSecureToken()
	} else if err := validToken(tok); err != nil {
		fail("--token: %v", err)
	}

	// Choose the mimic set: an explicit --mimics wins (power user); otherwise resolve
	// a coherent persona (SSH + 9), seeded from the token so the set is deterministic
	// per server and spreads across personas fleet-wide.
	var types []string
	var chosenPersona string
	if *mimics != "" {
		var err error
		if types, err = expandMimics(*mimics); err != nil {
			fail("--mimics: %v", err)
		}
		if cerr := persona.CheckCoherence(types); cerr != nil {
			color.Yellow("[!] %v (an explicit --mimics set — proceeding anyway).", cerr)
		}
	} else {
		chosenPersona = *personaFlag
		if chosenPersona == "auto" || chosenPersona == "" {
			chosenPersona = persona.Auto(tok)
		} else if !persona.Known(chosenPersona) {
			fail("--persona must be auto|%s", strings.Join(persona.Names(), "|"))
		}
		var err error
		if types, err = persona.Resolve(chosenPersona, tok); err != nil {
			fail("--persona: %v", err)
		}
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
		case "directadmin":
			if err := validPort(*daPort); err != nil {
				fail("--directadmin-port: %v", err)
			}
			mimicList = append(mimicList, config.MimicListener{Type: "directadmin", Port: *daPort, ServerName: *tlsServerName})
		case "https-alt":
			if err := validPort(*httpsAltPort); err != nil {
				fail("--https-alt-port: %v", err)
			}
			mimicList = append(mimicList, config.MimicListener{Type: "https-alt", Port: *httpsAltPort, ServerName: *tlsServerName})
		case "docker":
			if err := validPort(*dockerPort); err != nil {
				fail("--docker-port: %v", err)
			}
			mimicList = append(mimicList, config.MimicListener{Type: "docker", Port: *dockerPort, ServerName: *tlsServerName})
		case "grafana":
			if err := validPort(*grafanaPort); err != nil {
				fail("--grafana-port: %v", err)
			}
			mimicList = append(mimicList, config.MimicListener{Type: "grafana", Port: *grafanaPort, ServerName: *tlsServerName})
		case "prometheus":
			if err := validPort(*promPort); err != nil {
				fail("--prometheus-port: %v", err)
			}
			mimicList = append(mimicList, config.MimicListener{Type: "prometheus", Port: *promPort, ServerName: *tlsServerName})
		case "cpanel":
			if err := validPort(*cpanelPort); err != nil {
				fail("--cpanel-port: %v", err)
			}
			mimicList = append(mimicList, config.MimicListener{Type: "cpanel", Port: *cpanelPort, ServerName: *tlsServerName})
		case "whm":
			if err := validPort(*whmPort); err != nil {
				fail("--whm-port: %v", err)
			}
			mimicList = append(mimicList, config.MimicListener{Type: "whm", Port: *whmPort, ServerName: *tlsServerName})
		case "webmail":
			if err := validPort(*webmailPort); err != nil {
				fail("--webmail-port: %v", err)
			}
			mimicList = append(mimicList, config.MimicListener{Type: "webmail", Port: *webmailPort, ServerName: *tlsServerName})
		case "postgres":
			if err := validPort(*pgPort); err != nil {
				fail("--postgres-port: %v", err)
			}
			mimicList = append(mimicList, config.MimicListener{Type: "postgres", Port: *pgPort, ServerName: *tlsServerName})
		case "mysql":
			if err := validPort(*mysqlPort); err != nil {
				fail("--mysql-port: %v", err)
			}
			mimicList = append(mimicList, config.MimicListener{Type: "mysql", Port: *mysqlPort, ServerName: *tlsServerName})
		}
	}
	if *moveSSH {
		if err := sysutil.ChangeSSHPort(strconv.Itoa(*decoyPort)); err != nil {
			color.Red("[x] Failed to relocate OpenSSH to %d: %v", *decoyPort, err)
		} else {
			color.Green("[✓] OpenSSH relocated to %d (decoy).", *decoyPort)
		}
	}

	// -1 sentinel disables the decoy (config 0 would be re-defaulted to 80 on load).
	httpDecoy := *httpDecoyPort
	if httpDecoy == 0 {
		httpDecoy = -1
	} else if err := validPort(httpDecoy); err != nil {
		fail("--http-decoy-port: %v", err)
	}

	cfg := &config.AppConfig{
		Role:              "foreign",
		ForeignListenPort: *sshPort,
		DecoyPort:         *decoyPort,
		HTTPDecoyPort:     httpDecoy,
		Domain:            *domain,
		ACMEEmail:         *acmeEmail,
		DecoyStyle:        *decoyStyle,
		EgressIPMode:      *egressMode,
		EgressBindIP:      *bindIP,
		AuthToken:         tok,
		Persona:           chosenPersona,
		Mimics:            mimicList,
	}
	if err := config.SaveConfig(cfg); err != nil {
		fail("failed to save config: %v", err)
	}
	if chosenPersona != "" {
		color.Green("[✓] Persona: %s (SSH backbone + %d coherent mimics).", chosenPersona, len(types)-1)
	}
	color.Green("[✓] Foreign config written (mimics: %s, egress %s).", strings.Join(types, ","), *egressMode)
	if *domain == "" {
		color.Yellow("[!] No domain set: the TLS mimic will use a SELF-SIGNED certificate.")
		color.Yellow("    For much stronger camouflage, point a domain's A/AAAA record at this")
		color.Yellow("    server and re-run with --domain <name> to get a real Let's Encrypt cert.")
	} else {
		color.Green("[✓] Domain %s: a real Let's Encrypt cert will be obtained once DNS points here.", *domain)
	}
	// Emit a self-contained v2 pairing token (exit IP + ports + persona + key) so the
	// hub onboards by pasting one string — no --target-ip/--mimics to match by hand.
	exitIP := *publicIP
	if exitIP == "" {
		if ip, err := sysutil.GetPublicIPv4(); err == nil {
			exitIP = ip
		}
	}
	if exitIP != "" {
		eps := make(map[string]int, len(mimicList))
		for _, m := range mimicList {
			eps[m.Type] = m.Port
		}
		sni := *tlsServerName
		if sni == "" {
			sni = *domain
		}
		pt := pairing.Encode(pairing.Token{ExitIP: exitIP, AuthKey: tok, Persona: chosenPersona, SNI: sni, Endpoints: eps})
		color.HiGreen("\n━━━ Hub onboarding ━━━")
		color.HiGreen("Paste THIS pairing token into the hub — it already carries the exit IP, every")
		color.HiGreen("port, and the persona, so the hub needs no --target-ip/--persona/--mimics:")
		fmt.Println(pt)
		color.HiBlack("(Raw auth key, for advanced/legacy setups only: %s)", tok)
	} else {
		color.Yellow("[!] Public IP not detected: on the hub use the raw token below with --target-ip,")
		color.Yellow("    or re-run with --public-ip <ip> to emit a paste-only pairing token.")
		fmt.Printf("Auth Token: %s\n", tok)
	}
	// Apply immediately: without a restart the daemon keeps its old config (old
	// mimics AND old token), which silently breaks the link after re-provisioning.
	restartDaemon()
}

// cmdSetupIran writes the Iran (hub) config with its first node.
func cmdSetupIran(args []string) { cmdAddNode(args) }

// cmdAddNode appends a foreign node (one SOCKS port + endpoints) to the hub config.
func cmdAddNode(args []string) {
	fs := flag.NewFlagSet("add-node", flag.ExitOnError)
	alias := fs.String("alias", "", "node alias")
	targetIP := fs.String("target-ip", "", "foreign IP (with --mimics)")
	target := fs.String("target", "", "single foreign HOST:PORT (legacy, SSH)")
	personaFlag := fs.String("persona", "", "match the foreign's persona: auto|cpanel|directadmin|devops (derives the endpoint set from the token; needs --target-ip)")
	mimics := fs.String("mimics", "", "explicit endpoint set (overrides --persona): comma list or 'all' (needs --target-ip)")
	sshPort := fs.Int("ssh-port", 22, "foreign SSH mimic port")
	tlsPort := fs.Int("tls-port", 443, "foreign TLS mimic port")
	smtpPort := fs.Int("smtp-port", 587, "foreign SMTP (STARTTLS) mimic port")
	imapPort := fs.Int("imap-port", 143, "foreign IMAP (STARTTLS) mimic port")
	smtpsPort := fs.Int("smtps-port", 465, "foreign SMTPS (implicit TLS) mimic port")
	imapsPort := fs.Int("imaps-port", 993, "foreign IMAPS (implicit TLS) mimic port")
	daPort := fs.Int("directadmin-port", 2222, "foreign DirectAdmin panel mimic port")
	httpsAltPort := fs.Int("https-alt-port", 8443, "foreign alt-HTTPS mimic port")
	dockerPort := fs.Int("docker-port", 5000, "foreign Docker Registry mimic port")
	grafanaPort := fs.Int("grafana-port", 3000, "foreign Grafana mimic port")
	promPort := fs.Int("prometheus-port", 9090, "foreign Prometheus mimic port")
	cpanelPort := fs.Int("cpanel-port", 2083, "foreign cPanel mimic port")
	whmPort := fs.Int("whm-port", 2087, "foreign WHM mimic port")
	webmailPort := fs.Int("webmail-port", 2096, "foreign Webmail mimic port")
	pgPort := fs.Int("postgres-port", 5432, "foreign PostgreSQL (STARTTLS) mimic port")
	mysqlPort := fs.Int("mysql-port", 3306, "foreign MySQL (STARTTLS) mimic port")
	tlsServerName := fs.String("tls-servername", "", "TLS SNI (set to the foreign's domain for a real cert)")
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
	// A v2 pairing token is self-contained (IP + ports + persona + key); a v1 token is
	// a bare hex secret used with --target-ip and --persona/--mimics.
	pairTok, isV2, perr := pairing.Decode(*token)
	if perr != nil {
		fail("--token: %v", perr)
	}
	authKey := *token
	if isV2 {
		authKey = pairTok.AuthKey
	}
	if err := validToken(authKey); err != nil {
		fail("--token: %v", err)
	}

	var endpoints []config.Endpoint
	switch {
	case isV2:
		if *targetIP != "" || *mimics != "" || *personaFlag != "" {
			color.Yellow("[!] v2 pairing token is self-contained; ignoring --target-ip/--mimics/--persona.")
		}
		for _, ty := range mimicTypesAll {
			port, ok := pairTok.Endpoints[ty]
			if !ok {
				continue
			}
			sni := ""
			if ty != "ssh" {
				sni = pairTok.SNI
			}
			endpoints = append(endpoints, config.Endpoint{
				Target:     net.JoinHostPort(pairTok.ExitIP, strconv.Itoa(port)),
				Mimic:      ty,
				ServerName: sni,
			})
		}
		label := pairTok.Persona
		if label == "" {
			label = "custom"
		}
		color.Green("[✓] Pairing token: %s persona, %d endpoints @ %s.", label, len(endpoints), pairTok.ExitIP)
	case *mimics != "" || *personaFlag != "":
		if *targetIP == "" {
			fail("--mimics/--persona requires --target-ip")
		}
		// The endpoint set comes from an explicit --mimics (power user) or, since the
		// persona is deterministic from the token, from --persona resolved against the
		// shared token — so the hub matches the foreign's persona without hand-listing.
		var types []string
		var err error
		if *mimics != "" {
			if types, err = expandMimics(*mimics); err != nil {
				fail("--mimics: %v", err)
			}
		} else {
			name := *personaFlag
			if name == "auto" {
				name = persona.Auto(authKey)
			} else if !persona.Known(name) {
				fail("--persona must be auto|%s", strings.Join(persona.Names(), "|"))
			}
			if types, err = persona.Resolve(name, authKey); err != nil {
				fail("--persona: %v", err)
			}
			color.Green("[✓] Persona %q: dialing the foreign across %d endpoints.", name, len(types))
		}
		for label, p := range map[string]int{
			"ssh-port": *sshPort, "tls-port": *tlsPort, "smtp-port": *smtpPort, "imap-port": *imapPort,
			"smtps-port": *smtpsPort, "imaps-port": *imapsPort, "directadmin-port": *daPort,
			"https-alt-port": *httpsAltPort, "docker-port": *dockerPort, "grafana-port": *grafanaPort,
			"prometheus-port": *promPort, "cpanel-port": *cpanelPort, "whm-port": *whmPort,
			"webmail-port": *webmailPort, "postgres-port": *pgPort, "mysql-port": *mysqlPort,
		} {
			if err := validPort(p); err != nil {
				fail("--%s: %v", label, err)
			}
		}
		portFor := map[string]int{
			"ssh": *sshPort, "tls": *tlsPort, "smtp": *smtpPort, "imap": *imapPort,
			"smtps": *smtpsPort, "imaps": *imapsPort, "directadmin": *daPort,
			"https-alt": *httpsAltPort, "docker": *dockerPort, "grafana": *grafanaPort,
			"prometheus": *promPort, "cpanel": *cpanelPort, "whm": *whmPort, "webmail": *webmailPort,
			"postgres": *pgPort, "mysql": *mysqlPort,
		}
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
		fail("provide a v2 pairing token, or --target-ip with --persona/--mimics, or --target HOST:PORT")
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
		AuthToken:           authKey,
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
