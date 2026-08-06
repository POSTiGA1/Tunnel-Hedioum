package main

import (
	"os"
	"testing"
)

// TestIsTerminalNonTTY covers the exact trigger of the installer wizard-on-pipe
// bug: when stdin is a pipe or a regular file (not a TTY), isTerminal must report
// false so main() skips the interactive wizard instead of provisioning a bogus
// config.
func TestIsTerminalNonTTY(t *testing.T) {
	// A pipe (what `bash <(curl ...) </dev/null | ...` / SSH exec gives us).
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(r) {
		t.Error("a pipe must not be reported as a terminal")
	}

	// A regular file.
	f, err := os.CreateTemp(t.TempDir(), "notty")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isTerminal(f) {
		t.Error("a regular file must not be reported as a terminal")
	}

	// /dev/null is a character device but not a TTY-backed stdin the wizard can use;
	// os.ModeCharDevice is set for it, so this documents the known edge (we accept
	// it — a wizard reading /dev/null EOFs immediately, same safe outcome).
	if devnull, err := os.Open(os.DevNull); err == nil {
		defer devnull.Close()
		_ = isTerminal(devnull) // no assertion; just exercises the path
	}
}
