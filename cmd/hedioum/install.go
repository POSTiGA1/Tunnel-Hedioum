package main

import (
	"bytes"
	_ "embed"
	"flag"
	"io"
	"os"
	"os/exec"
	"text/template"

	"github.com/fatih/color"
	"github.com/hedioum/Hedioum-Pool-Tunnel/config"
	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/sysutil"
)

//go:embed hedioum.service.tmpl
var systemdUnit string

// renderUnit fills the systemd unit template. tunCapable adds CAP_NET_ADMIN so
// the hub can bring up an opt-in TUN interface; the foreign leaves it off.
func renderUnit(tunCapable bool) string {
	t, err := template.New("unit").Parse(systemdUnit)
	if err != nil {
		return systemdUnit // template is compiled-in; parse cannot realistically fail
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, struct{ TunCapable bool }{tunCapable}); err != nil {
		return systemdUnit
	}
	return buf.String()
}

// reconcileUnit rewrites the installed unit to match the node's role (the hub is
// TUN-capable, the foreign is not) and reloads systemd. Best-effort: it is a no-op
// when the service is not installed or we are not privileged.
func reconcileUnit(role string) {
	if _, err := os.Stat(installUnitPath); err != nil {
		return // not installed under systemd (e.g. dev run)
	}
	if err := os.WriteFile(installUnitPath, []byte(renderUnit(role == "iran")), 0644); err != nil {
		return
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
}

const (
	installBinPath  = "/usr/local/bin/hedioum-tunnel"
	installUnitPath = "/etc/systemd/system/hedioum.service"
)

// cmdInstall self-installs the running binary + the systemd unit, with no network
// access required — deploy by copying the binary to the server and running this.
func cmdInstall(args []string) {
	if os.Geteuid() != 0 {
		fail("install must run as root")
	}

	// The config/working directory must exist before the (namespaced) service can
	// start — the hardened unit sets WorkingDirectory + ReadWritePaths to it.
	if err := os.MkdirAll("/etc/hedioum", 0755); err != nil {
		fail("cannot create /etc/hedioum: %v", err)
	}

	self, err := os.Executable()
	if err != nil {
		fail("cannot locate the running binary: %v", err)
	}

	// Copy self -> temp -> atomic rename, so replacing a running binary does not
	// hit "text file busy".
	if err := copyExecutable(self, installBinPath); err != nil {
		fail("failed to install binary: %v", err)
	}
	color.Green("[✓] Binary installed to %s", installBinPath)

	// Render the unit for the role already configured on this box, if any. A fresh
	// install with no config yet is TUN-capable by default (the hub is the TUN user);
	// setup-foreign later re-renders it locked-down. This keeps the foreign minimal
	// while ensuring the hub has CAP_NET_ADMIN ready for an opt-in TUN.
	tunCapable := true
	if cfg, err := config.LoadConfig(); err == nil && cfg.Role == "foreign" {
		tunCapable = false
	}
	if err := os.WriteFile(installUnitPath, []byte(renderUnit(tunCapable)), 0644); err != nil {
		fail("failed to write systemd unit: %v", err)
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	_ = exec.Command("systemctl", "enable", "hedioum.service").Run()
	color.Green("[✓] systemd service installed and enabled.")

	enableBBR()

	color.HiWhite("\nNext:")
	color.HiWhite("  Foreign: hedioum-tunnel setup-foreign [--move-ssh]")
	color.HiWhite("  Iran:    hedioum-tunnel setup-iran --alias ... --target IP:PORT --socks-port N --token HEX")
	color.HiWhite("  Then:    systemctl start hedioum.service")
}

// enableBBR turns on the BBR congestion control + fq qdisc, a large throughput
// win on high-latency/lossy links. Best-effort; ignored on kernels without BBR.
func enableBBR() {
	const conf = "net.core.default_qdisc=fq\nnet.ipv4.tcp_congestion_control=bbr\n"
	if err := os.WriteFile("/etc/sysctl.d/99-hedioum-bbr.conf", []byte(conf), 0644); err != nil {
		return
	}
	if err := exec.Command("sysctl", "-p", "/etc/sysctl.d/99-hedioum-bbr.conf").Run(); err == nil {
		color.Green("[✓] Enabled BBR congestion control (fq qdisc).")
	}
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst) // atomic replace
}

// cmdUpdate updates the binary from GitHub, or installs a locally-provided one
// (--file) when GitHub is blocked. Both use the backup/rollback flow.
func cmdUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	file := fs.String("file", "", "install from a local binary instead of downloading")
	_ = fs.Parse(args)
	if *file != "" {
		sysutil.UpdateFromFile(*file)
		return
	}
	sysutil.SelfUpdate(AppVersion)
}

// cmdUninstall stops, disables, and removes everything (non-interactive with --yes).
func cmdUninstall(args []string) {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	yes := fs.Bool("yes", false, "confirm removal of the daemon, config, and binary")
	_ = fs.Parse(args)
	if !*yes {
		fail("refusing without --yes (this removes the daemon, config, and binary)")
	}
	sysutil.Uninstall() // performs the removal and exits
}
