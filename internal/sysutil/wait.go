package sysutil

import (
	"os"
	"os/signal"
	"syscall"
)

// WaitForTerminationSignal blocks until the process receives SIGINT or SIGTERM.
// Daemons use it instead of a bare `select{}` so the process stays alive even when
// it has no active goroutines (e.g. an Iran hub configured with zero foreign
// nodes) — a bare `select{}` there trips Go's runtime deadlock detector and
// crash-loops the service. It also enables a clean shutdown under systemd.
func WaitForTerminationSignal() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}
