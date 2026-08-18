package persona

import (
	"strings"
	"testing"
)

// TestResolveShapeAndCoherence: every persona resolves to ssh + exactly 9, includes
// its core (with tls), is coherent, and has no duplicates.
func TestResolveShapeAndCoherence(t *testing.T) {
	for _, name := range Names() {
		set, err := Resolve(name, "seed-"+name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(set) != 10 {
			t.Fatalf("%s: got %d mimics, want 10 (ssh + 9): %v", name, len(set), set)
		}
		if set[0] != "ssh" {
			t.Fatalf("%s: backbone must be first, got %q", name, set[0])
		}
		seen := map[string]bool{}
		for _, m := range set {
			if seen[m] {
				t.Fatalf("%s: duplicate %q in %v", name, m, set)
			}
			seen[m] = true
		}
		if !seen["tls"] {
			t.Fatalf("%s: every persona must include tls (:443) for bootstrap: %v", name, set)
		}
		for _, c := range Registry[name].Core {
			if !seen[c] {
				t.Fatalf("%s: core mimic %q missing: %v", name, c, set)
			}
		}
		if err := CheckCoherence(set); err != nil {
			t.Fatalf("%s: resolved set is incoherent: %v", name, err)
		}
	}
}

// TestDeterministic: same (persona, seed) → identical set; the order is stable.
func TestDeterministic(t *testing.T) {
	a, _ := Resolve("cpanel", "token-A")
	b, _ := Resolve("cpanel", "token-A")
	if strings.Join(a, ",") != strings.Join(b, ",") {
		t.Fatalf("not deterministic:\n%v\n%v", a, b)
	}
}

// TestSeedVariation: two different seeds of the same persona differ in their fill
// (the pool is larger than the slack), so same-persona installs are not identical.
func TestSeedVariation(t *testing.T) {
	seen := map[string]int{}
	for _, s := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		set, _ := Resolve("cpanel", s)
		seen[strings.Join(set, ",")]++
	}
	if len(seen) < 2 {
		t.Fatalf("expected fill variation across seeds, got only %d distinct sets", len(seen))
	}
}

// TestAutoSpreads: Auto distributes across all three personas over many seeds and is
// deterministic per seed.
func TestAutoSpreads(t *testing.T) {
	if Auto("x") != Auto("x") {
		t.Fatal("Auto must be deterministic per seed")
	}
	got := map[string]bool{}
	for i := 0; i < 200; i++ {
		got[Auto(string(rune('a'+i%26))+string(rune('0'+i/26)))] = true
	}
	for _, name := range Names() {
		if !got[name] {
			t.Fatalf("Auto never selected persona %q over 200 seeds", name)
		}
	}
}

// TestCoherenceValidator: the validator flags cPanel-family + DirectAdmin together.
func TestCoherenceValidator(t *testing.T) {
	if err := CheckCoherence([]string{"ssh", "tls", "cpanel", "directadmin"}); err == nil {
		t.Fatal("cpanel + directadmin should be rejected as incoherent")
	}
	if err := CheckCoherence([]string{"ssh", "tls", "whm", "postgres"}); err != nil {
		t.Fatalf("coherent cPanel set wrongly rejected: %v", err)
	}
}

// TestUnknownPersona: Resolve errors on an unknown name.
func TestUnknownPersona(t *testing.T) {
	if _, err := Resolve("nope", "s"); err == nil {
		t.Fatal("unknown persona should error")
	}
}
