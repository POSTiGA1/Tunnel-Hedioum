package main

import "testing"

func TestMimicPort(t *testing.T) {
	// SSH port is caller-configurable; the rest are conventional.
	if got := mimicPort("ssh", 2200); got != 2200 {
		t.Errorf("ssh port = %d, want 2200 (caller-set)", got)
	}
	want := map[string]int{"tls": 443, "smtp": 587, "imap": 143, "smtps": 465, "imaps": 993, "directadmin": 2222}
	for ty, p := range want {
		if got := mimicPort(ty, 22); got != p {
			t.Errorf("mimicPort(%q) = %d, want %d", ty, got, p)
		}
	}
	if got := mimicPort("bogus", 22); got != 0 {
		t.Errorf("unknown mimic should map to 0, got %d", got)
	}
}

func TestContainsStr(t *testing.T) {
	list := []string{"ssh", "tls", "imaps"}
	if !containsStr(list, "tls") || containsStr(list, "smtp") {
		t.Fatalf("containsStr wrong: %v", list)
	}
	if containsStr(nil, "x") {
		t.Fatal("containsStr(nil) should be false")
	}
}

// TestAllMimicsComplete guards that the wizard's arsenal list stays in sync with
// the supported mimic types (so "all" really means all).
func TestAllMimicsComplete(t *testing.T) {
	want := map[string]bool{
		"ssh": true, "tls": true, "https-alt": true, "smtp": true, "imap": true,
		"smtps": true, "imaps": true, "directadmin": true, "docker": true, "grafana": true, "prometheus": true,
		"cpanel": true, "whm": true, "webmail": true, "postgres": true, "mysql": true,
	}
	if len(allMimics) != len(want) {
		t.Fatalf("allMimics = %v, want %d types", allMimics, len(want))
	}
	for _, m := range allMimics {
		if !want[m] {
			t.Errorf("unexpected mimic in allMimics: %q", m)
		}
		if mimicPort(m, 22) == 0 {
			t.Errorf("allMimics entry %q has no port mapping", m)
		}
	}
}
