package main

import (
	"testing"

	"github.com/hedioum/Hedioum-Pool-Tunnel/config"
)

func TestFirewallPortsOpensEveryMimic(t *testing.T) {
	cfg := &config.AppConfig{Mimics: []config.MimicListener{
		{Type: "ssh", Port: 22},
		{Type: "tls", Port: 443},
		{Type: "smtp", Port: 587},
		{Type: "imap", Port: 143},
	}}
	got := firewallPorts(cfg)
	want := map[int]bool{22: true, 443: true, 587: true, 143: true}
	if len(got) != len(want) {
		t.Fatalf("firewallPorts = %v, want %d ports", got, len(want))
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected port %d", p)
		}
	}
}

func TestFirewallPortsLegacyFallback(t *testing.T) {
	cfg := &config.AppConfig{ForeignListenPort: 8443}
	if got := firewallPorts(cfg); len(got) != 1 || got[0] != 8443 {
		t.Fatalf("legacy fallback = %v, want [8443]", got)
	}
	if got := firewallPorts(&config.AppConfig{}); len(got) != 1 || got[0] != 22 {
		t.Fatalf("empty fallback = %v, want [22]", got)
	}
}

func TestFirewallPortsIncludesHTTPDecoy(t *testing.T) {
	cfg := &config.AppConfig{
		Mimics:        []config.MimicListener{{Type: "tls", Port: 443}},
		HTTPDecoyPort: 80,
	}
	got := firewallPorts(cfg)
	has := func(p int) bool {
		for _, x := range got {
			if x == p {
				return true
			}
		}
		return false
	}
	if !has(443) || !has(80) {
		t.Fatalf("expected 443 and 80 open, got %v", got)
	}
	// Disabled decoy (-1) must not add a rule.
	cfg.HTTPDecoyPort = -1
	for _, p := range firewallPorts(cfg) {
		if p < 1 {
			t.Fatalf("disabled decoy leaked a bad port %d", p)
		}
	}
}
