package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/hedioum/Hedioum-Pool-Tunnel/config"
	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/sysutil"
)

func handleReset() {
	if err := os.Remove("/etc/hedioum/hedioum.json"); err != nil && !os.IsNotExist(err) {
		os.Remove("hedioum.json") // Fallback to local directory
	}
	color.Yellow("[-] Configuration purged. Resetting daemon state...\n")
	exec.Command("systemctl", "stop", "hedioum.service").Run()
}

func runSetupWizard() *config.AppConfig {
	var role string
	prompt := &survey.Select{
		Message: "Define the network role of this server instance:",
		Options: []string{"Foreign Egress Node (Target)", "Iran Hub Node (Ingress)"},
	}
	survey.AskOne(prompt, &role)

	cfg := &config.AppConfig{}

	if role == "Foreign Egress Node (Target)" {
		cfg.Role = "foreign"
		setupForeignNode(cfg)
	} else {
		cfg.Role = "iran"
		setupIranNode(cfg, true)
	}

	if err := config.SaveConfig(cfg); err != nil {
		color.Red("[x] Fatal: Failed to persist state: %v", err)
		os.Exit(1)
	}
	color.Green("\n[✓] State provisioned successfully.")
	return cfg
}

func setupForeignNode(cfg *config.AppConfig) {
	color.HiBlue("\n--- Foreign Egress Provisioning ---")
	detectedIP, _ := sysutil.GetPublicIPv4()

	var ip string
	survey.AskOne(&survey.Input{
		Message: "Confirm Server Public IPv4:",
		Default: detectedIP,
	}, &ip, survey.WithValidator(survey.Required))

	// Which camouflages this node listens behind (checkbox; "all" = whole arsenal).
	mimics := promptMimics()

	// SSH-specific prompts only matter when the SSH mimic is enabled.
	listenPort := 22
	decoyPort := 2022
	if containsStr(mimics, "ssh") {
		var listenPortStr string
		survey.AskOne(&survey.Input{Message: "SSH mimic public port:", Default: "22"}, &listenPortStr)
		listenPort = clampPort(listenPortStr, 22)
		if listenPort != 22 {
			color.Yellow("[!] SSH mimic on port %d: it is most convincing on 22.", listenPort)
		}

		var decoyPortStr string
		survey.AskOne(&survey.Input{Message: "Decoy sshd port (OpenSSH is moved here):", Default: "2022"}, &decoyPortStr)
		decoyPort = clampPort(decoyPortStr, 2022)

		changeSSH := false
		survey.AskOne(&survey.Confirm{
			Message: fmt.Sprintf("Move OpenSSH to port %d to free port %d for the SSH mimic?", decoyPort, listenPort),
			Default: true,
		}, &changeSSH)
		if changeSSH {
			if err := sysutil.ChangeSSHPort(strconv.Itoa(decoyPort)); err != nil {
				color.Red("[x] OpenSSH port relocation failed: %v", err)
			} else {
				color.Green("[✓] OpenSSH shifted to %d. Decoy port available.", decoyPort)
			}
		}
	}

	// Build one listener per selected mimic on its conventional port.
	var mimicList []config.MimicListener
	for _, ty := range mimics {
		ml := config.MimicListener{Type: ty, Port: mimicPort(ty, listenPort)}
		if ty == "ssh" {
			ml.Decoy = fmt.Sprintf("127.0.0.1:%d", decoyPort)
		}
		mimicList = append(mimicList, ml)
	}

	cfg.ForeignListenPort = listenPort
	cfg.DecoyPort = decoyPort
	cfg.Mimics = mimicList
	cfg.AuthToken = sysutil.GenerateSecureToken()

	// IPv6 egress is opt-in (default IPv4-only to avoid leaking the v6 identity).
	cfg.EgressIPMode = "ipv4"
	if v6, err := sysutil.GetPublicIPv6(); err == nil && v6 != "" {
		enableV6 := false
		survey.AskOne(&survey.Confirm{
			Message: fmt.Sprintf("Detected IPv6 (%s). Allow outbound traffic over IPv6 too?", v6),
			Default: false,
		}, &enableV6)
		if enableV6 {
			var mode string
			survey.AskOne(&survey.Select{
				Message: "Egress IP mode:",
				Options: []string{"Dual (prefer IPv4, fall back to IPv6)", "IPv6 only"},
			}, &mode)
			if strings.HasPrefix(mode, "IPv6") {
				cfg.EgressIPMode = "ipv6"
			} else {
				cfg.EgressIPMode = "dual"
			}
		}
	}

	// TLS certificate: a real domain (Let's Encrypt) vs self-signed.
	color.HiWhite("\n--- TLS Certificate ---")
	color.White("  A domain pointed at THIS server lets the tunnel present a genuine Let's")
	color.White("  Encrypt certificate — far harder to fingerprint than a self-signed one.")
	color.White("  Without a domain everything still works, using a self-signed certificate.")
	domain := ""
	survey.AskOne(&survey.Input{Message: "Domain for a real certificate (blank = self-signed):"}, &domain)
	cfg.Domain = strings.TrimSpace(domain)
	if cfg.Domain != "" {
		email := ""
		survey.AskOne(&survey.Input{Message: "Let's Encrypt account email (optional):"}, &email)
		cfg.ACMEEmail = strings.TrimSpace(email)
	} else {
		color.Yellow("  [!] Using a self-signed certificate. A domain is recommended when possible.")
	}

	// Camouflage persona shown to unauthorized probes / scanners.
	persona := ""
	survey.AskOne(&survey.Select{
		Message: "Decoy persona for unauthorized probes:",
		Options: []string{"apache (Apache default page)", "directadmin (DirectAdmin hosting box)"},
		Default: "apache (Apache default page)",
	}, &persona)
	if strings.HasPrefix(persona, "directadmin") {
		cfg.DecoyStyle = "directadmin"
	} else {
		cfg.DecoyStyle = "apache"
	}

	color.HiWhite("\n[INFO] Provisioning Summary:")
	labels := make([]string, len(mimicList))
	for i, m := range mimicList {
		labels[i] = fmt.Sprintf("%s:%d", m.Type, m.Port)
	}
	fmt.Printf(" - Mimics:     %s\n", strings.Join(labels, ", "))
	fmt.Printf(" - Auth Token: %s\n", color.HiYellowString(cfg.AuthToken))
	color.HiRed("   (CRITICAL: Retain this token for Iran Hub configuration)\n")
}

func setupIranNode(cfg *config.AppConfig, isFirstTime bool) {
	color.HiBlue("\n--- Egress Target Registration ---")

	node := config.ForeignNode{}
	suggestedSocksPort := getNextFreeSocksPort(cfg)

	questions := []*survey.Question{
		{
			Name:     "alias",
			Prompt:   &survey.Input{Message: "Server Alias (e.g., DE-Frankfurt-01):"},
			Validate: survey.Required,
		},
		{
			Name:     "targetip",
			Prompt:   &survey.Input{Message: "Foreign Egress IP (IPv4 or IPv6) or hostname:"},
			Validate: validateHost,
		},
		{
			Name:   "targetport",
			Prompt: &survey.Input{Message: "Foreign SSH mimic port (other mimics use standard ports):", Default: "22"},
		},
		{
			Name:   "localsocksport",
			Prompt: &survey.Input{Message: "Local SOCKS5 Bind Port (for X-UI Outbound mapping):", Default: suggestedSocksPort},
		},
		{
			Name:   "minconnections",
			Prompt: &survey.Input{Message: "Min Physical Connections (Warm-up pool baseline):", Default: "10"},
		},
		{
			Name:   "maxconnections",
			Prompt: &survey.Input{Message: "Max Physical Connections (Scale limit):", Default: "20"},
		},
		{
			Name:   "bandwidthlimit",
			Prompt: &survey.Input{Message: "Target Bandwidth Limit per Connection (Mbps):", Default: "8"},
		},
		{
			Name:   "bandwidthjitter",
			Prompt: &survey.Input{Message: "Bandwidth Jitter/Variance for DPI Evasion (Mbps):", Default: "2"},
		},
		{
			Name:     "authtoken",
			Prompt:   &survey.Input{Message: "Authentication Token (from egress server):"},
			Validate: survey.Required,
		},
	}

	answers := struct {
		Alias           string
		TargetIP        string
		TargetPort      string
		LocalSocksPort  string
		MinConnections  string
		MaxConnections  string
		BandwidthLimit  string
		BandwidthJitter string
		AuthToken       string
	}{}

	if err := survey.Ask(questions, &answers); err != nil {
		return
	}

	node.Alias = answers.Alias
	node.TargetIP = answers.TargetIP
	node.AuthToken = answers.AuthToken

	// Safely parse all integer inputs, falling back to defaults if empty or invalid (0)
	node.TargetPort = safeAtoi(answers.TargetPort, 22)
	node.LocalSocksPort = safeAtoi(answers.LocalSocksPort, safeAtoi(suggestedSocksPort, 40001))
	node.MinConnections = safeAtoi(answers.MinConnections, 10)
	node.MaxConnections = safeAtoi(answers.MaxConnections, 20)
	node.BandwidthLimitMbps = safeAtoi(answers.BandwidthLimit, 8)
	node.BandwidthJitterMbps = safeAtoi(answers.BandwidthJitter, 2)

	// Which mimics to reach this node over (checkbox; must match what the foreign
	// node runs). Populating Endpoints is what enables the multi-mimic arsenal —
	// without it the node falls back to a single synthesized SSH endpoint.
	for _, ty := range promptMimics() {
		node.Endpoints = append(node.Endpoints, config.Endpoint{
			Target: net.JoinHostPort(node.TargetIP, strconv.Itoa(mimicPort(ty, node.TargetPort))),
			Mimic:  ty,
		})
	}
	color.Green("[✓] Endpoints: %d mimic(s) toward %s", len(node.Endpoints), node.TargetIP)

	cfg.UpdateForeignNode(node)
}

// --- Helper Functions ---

// getNextFreeSocksPort scans existing configurations for the highest used SOCKS5 port,
// increments it, and dynamically tests the OS to ensure the port is actually free.
func getNextFreeSocksPort(cfg *config.AppConfig) string {
	startPort := 40001
	for _, node := range cfg.ForeignNodes {
		if node.LocalSocksPort >= startPort {
			startPort = node.LocalSocksPort + 1
		}
	}

	// Keep testing ports until we find one that is actively free on the OS
	for {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", startPort))
		if err == nil {
			ln.Close() // The port is free, close the test listener
			break
		}
		startPort++
	}
	return strconv.Itoa(startPort)
}

// validateHost accepts a non-empty IPv4/IPv6 literal or a plausible hostname.
func validateHost(val interface{}) error {
	s, _ := val.(string)
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("a value is required")
	}
	if net.ParseIP(s) != nil {
		return nil // valid IPv4 or IPv6 literal
	}
	if strings.ContainsAny(s, " \t/\\") {
		return fmt.Errorf("not a valid IP or hostname")
	}
	return nil
}

// clampPort parses a port string, returning def if it is empty/invalid/out of range.
func clampPort(s string, def int) int {
	p := safeAtoi(s, def)
	if validPort(p) != nil {
		return def
	}
	return p
}

// safeAtoi parses strings to integers securely. It falls back to a provided default value
// if the input is empty, invalid, or zero (to prevent zero-port configuration bugs).
func safeAtoi(s string, defaultVal int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(s)
	if err != nil || val <= 0 { // Also prevents negative values and 0
		return defaultVal
	}
	return val
}

// allMimics is the full camouflage arsenal, in the order the wizard offers it.
// Shared with the CLI (mimicTypesAll) so the two never disagree on the set.
var allMimics = mimicTypesAll

// promptMimics shows a checkbox selection of camouflage protocols and returns the
// chosen types, defaulting to ssh+tls. The first option ("all") selects the whole
// arsenal. Never returns empty (falls back to ssh+tls).
func promptMimics() []string { return promptMimicsWithDefault([]string{"ssh", "tls"}) }

// promptMimicsWithDefault is promptMimics with a caller-supplied pre-selection (used
// by the edit flows so the current mimic set is pre-checked). Never returns empty.
func promptMimicsWithDefault(def []string) []string {
	const allOpt = "all (enable every mimic)"
	if len(def) == 0 {
		def = []string{"ssh", "tls"}
	}
	var sel []string
	survey.AskOne(&survey.MultiSelect{
		Message: "Select camouflage protocols (Space toggles, Enter confirms):",
		Options: append([]string{allOpt}, allMimics...),
		Default: def,
		Help:    "ssh=22, tls=HTTPS:443, https-alt=8443, smtp=587, imap=143, smtps=465, imaps=993, directadmin=2222, docker=5000, grafana=3000, prometheus=9090, postgres=5432, mysql=3306. Run several for a stronger, shifting signature.",
	}, &sel)

	chosen := map[string]bool{}
	for _, s := range sel {
		if s == allOpt {
			return append([]string{}, allMimics...)
		}
		chosen[s] = true
	}
	var out []string
	for _, t := range allMimics {
		if chosen[t] {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		out = []string{"ssh", "tls"} // never leave a node with no camouflage
	}
	return out
}

// mimicPort maps a mimic type to its conventional port; the SSH port is caller-set
// so the operator can relocate it when outbound :22 is blocked.
func mimicPort(ty string, sshPort int) int {
	if ty == "ssh" {
		return sshPort
	}
	return conventionalPorts()[ty] // 0 for an unknown type
}

// containsStr reports whether s is in list.
func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
