package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/hedioum/Hedioum-Pool-Tunnel/config"
	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/egress"
	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/ingress"
	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/sysutil"
)

func main() {
	resetCfg := flag.Bool("reset", false, "Wipe the current configuration database and restart the setup wizard")
	flag.Parse()

	if *resetCfg {
		handleReset()
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		// No config means first launch. Force terminal wizard regardless of environment.
		printHeader()
		color.Yellow("[!] Initializing Setup Wizard for fresh installation...\n")
		cfg = runSetupWizard()
	}

	// Detect execution context: Human (Terminal) vs Systemd (Daemon)
	fileInfo, _ := os.Stdout.Stat()
	isInteractive := (fileInfo.Mode() & os.ModeCharDevice) != 0

	if isInteractive {
		runInteractiveDashboard(cfg)
	} else {
		// Headless Daemon Execution (Systemd)
		if cfg.Role == "foreign" {
			egress.StartForeignDaemon(cfg)
		} else if cfg.Role == "iran" {
			ingress.StartIranHub(cfg)
		} else {
			os.Exit(1)
		}
	}
}

// --- CORE BOOTSTRAP & WIZARD ---

func handleReset() {
	if err := os.Remove("/etc/hedioum/hedioum.json"); err != nil && !os.IsNotExist(err) {
		os.Remove("hedioum.json") // Fallback to local directory
	}
	color.Yellow("[-] Configuration purged. Resetting daemon state...\n")
	exec.Command("systemctl", "stop", "hedioum.service").Run()
}

func printHeader() {
	color.Cyan("=========================================================")
	color.HiCyan("   Hedioum Dynamic Pool Tunnel - Management Dashboard")
	color.Cyan("=========================================================\n")
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

	changeSSH := false
	survey.AskOne(&survey.Confirm{
		Message: "Move OpenSSH daemon to port 2022 to free Port 22 for DPI Decoy?",
		Default: true,
	}, &changeSSH)

	if changeSSH {
		if err := sysutil.ChangeSSHPort("2022"); err != nil {
			color.Red("[x] OpenSSH port relocation failed: %v", err)
		} else {
			color.Green("[✓] OpenSSH shifted to 2022. Decoy port available.")
		}
	}

	cfg.ForeignListenPort = 22
	cfg.AuthToken = sysutil.GenerateSecureToken()

	color.HiWhite("\n[INFO] Provisioning Summary:")
	fmt.Printf(" - Listen Port: %d\n", cfg.ForeignListenPort)
	fmt.Printf(" - Auth Token:  %s\n", color.HiYellowString(cfg.AuthToken))
	color.HiRed("   (CRITICAL: Retain this token for Iran Hub configuration)\n")
}

func setupIranNode(cfg *config.AppConfig, isFirstTime bool) {
	color.HiBlue("\n--- Egress Target Registration ---")

	node := config.ForeignNode{}

	questions := []*survey.Question{
		{
			Name:     "alias",
			Prompt:   &survey.Input{Message: "Server Alias (e.g., DE-Frankfurt-01):"},
			Validate: survey.Required,
		},
		{
			Name:     "targetip",
			Prompt:   &survey.Input{Message: "Foreign Egress IPv4 Address:"},
			Validate: survey.Required,
		},
		{
			Name:   "localsocksport",
			Prompt: &survey.Input{Message: "Local SOCKS5 Bind Port (for X-UI Outbound mapping):", Default: "40001"},
		},
		{
			Name:   "maxconnections",
			Prompt: &survey.Input{Message: "Max Physical Connections in Pool (Scale limit):", Default: "20"},
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

	// Capture responses via intermediate struct to handle integer conversion cleanly
	answers := struct {
		Alias           string
		TargetIP        string
		LocalSocksPort  string
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

	node.LocalSocksPort, _ = strconv.Atoi(answers.LocalSocksPort)
	node.MaxConnections, _ = strconv.Atoi(answers.MaxConnections)
	node.BandwidthLimitMbps, _ = strconv.Atoi(answers.BandwidthLimit)
	node.BandwidthJitterMbps, _ = strconv.Atoi(answers.BandwidthJitter)

	cfg.UpdateForeignNode(node)
}

// --- INTERACTIVE OPERATIONS DASHBOARD ---

func runInteractiveDashboard(cfg *config.AppConfig) {
	printHeader()

	for {
		var action string
		options := []string{
			"1. Show Live Service Status & Configuration",
			"2. View Real-time Logs (Journalctl)",
		}

		if cfg.Role == "iran" {
			options = append(options,
				"3. Add New Foreign Egress Node",
				"4. Remove Existing Egress Node",
			)
		} else {
			options = append(options, "3. Rotate Authentication Token")
		}
		options = append(options, "0. Start Daemon Foreground (Debug)", "Exit")

		prompt := &survey.Select{
			Message:  "Select an operational task:",
			Options:  options,
			PageSize: 10,
		}
		survey.AskOne(prompt, &action)

		switch action {
		case "1. Show Live Service Status & Configuration":
			runSystemCmd("systemctl", "status", "hedioum.service", "--no-pager")

			if cfg.Role == "iran" && len(cfg.ForeignNodes) > 0 {
				color.HiCyan("\n--- Active Egress Pools Configuration ---")
				for _, n := range cfg.ForeignNodes {
					fmt.Printf(" Alias: %s | Target: %s:%d | SOCKS: %d\n", color.HiWhiteString(n.Alias), n.TargetIP, n.TargetPort, n.LocalSocksPort)
					fmt.Printf(" Dynamics: Limit %dMbps (±%dMbps Jitter) | Max Conns: %d\n\n", n.BandwidthLimitMbps, n.BandwidthJitterMbps, n.MaxConnections)
				}
				color.Yellow("Note: Run 'journalctl -u hedioum.service -f' for live Scale-Up/Down and Mbps monitoring.")
			}

		case "2. View Real-time Logs (Journalctl)":
			color.Cyan("\n[*] Tailing logs. Press Ctrl+C to return to dashboard.\n")
			runSystemCmd("journalctl", "-u", "hedioum.service", "-f", "-n", "50")

		case "3. Add New Foreign Egress Node":
			setupIranNode(cfg, false)
			saveAndRestart(cfg)

		case "4. Remove Existing Egress Node":
			if len(cfg.ForeignNodes) == 0 {
				color.Yellow("No egress nodes registered.")
				continue
			}
			var aliases []string
			for _, n := range cfg.ForeignNodes {
				aliases = append(aliases, n.Alias)
			}
			var selected string
			survey.AskOne(&survey.Select{Message: "Select node to terminate and remove:", Options: aliases}, &selected)

			if cfg.RemoveForeignNode(selected) {
				saveAndRestart(cfg)
			}

		case "3. Rotate Authentication Token":
			cfg.AuthToken = sysutil.GenerateSecureToken()
			color.Green("\n[✓] Token Rotated. New Token: %s", color.HiYellowString(cfg.AuthToken))
			color.Red("WARNING: You must update this token on your Iran Hub immediately.")
			saveAndRestart(cfg)

		case "0. Start Daemon Foreground (Debug)":
			color.Magenta("\n[*] Bootstrapping Daemon in foreground. Ctrl+C to abort.")
			if cfg.Role == "foreign" {
				egress.StartForeignDaemon(cfg)
			} else {
				ingress.StartIranHub(cfg)
			}

		case "Exit":
			fmt.Println("Exiting dashboard...")
			os.Exit(0)
		}
		fmt.Println("\n---------------------------------------------------------")
	}
}

// saveAndRestart commits config changes and performs a graceful systemd restart
func saveAndRestart(cfg *config.AppConfig) {
	if err := config.SaveConfig(cfg); err != nil {
		color.Red("[x] Failed to commit changes to storage: %v", err)
		return
	}
	color.Green("[✓] Configuration saved.")

	// Execute non-blocking service restart if managed by systemd
	color.HiBlue("[*] Restarting background daemon to apply state changes...")
	err := exec.Command("systemctl", "restart", "hedioum.service").Run()
	if err != nil {
		color.Yellow("[-] Systemd restart failed (Are you running as root?). Apply changes manually.")
	} else {
		color.Green("[✓] Daemon reloaded successfully.")
	}
}

// runSystemCmd acts as a bridge to execute system binaries directly within the TUI
func runSystemCmd(name string, arg ...string) {
	cmd := exec.Command(name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	_ = cmd.Run()
}
