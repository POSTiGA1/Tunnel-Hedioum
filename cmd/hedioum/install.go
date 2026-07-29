package main

import (
	_ "embed"
	"flag"
	"io"
	"os"
	"os/exec"

	"github.com/fatih/color"
	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/sysutil"
)

//go:embed hedioum.service.tmpl
var systemdUnit string

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

	if err := os.WriteFile(installUnitPath, []byte(systemdUnit), 0644); err != nil {
		fail("failed to write systemd unit: %v", err)
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	_ = exec.Command("systemctl", "enable", "hedioum.service").Run()
	color.Green("[✓] systemd service installed and enabled.")

	color.HiWhite("\nNext:")
	color.HiWhite("  Foreign: hedioum-tunnel setup-foreign [--move-ssh]")
	color.HiWhite("  Iran:    hedioum-tunnel setup-iran --alias ... --target IP:PORT --socks-port N --token HEX")
	color.HiWhite("  Then:    systemctl start hedioum.service")
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
