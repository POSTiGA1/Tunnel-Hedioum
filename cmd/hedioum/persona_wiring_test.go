package main

import (
	"testing"

	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/persona"
)

// TestPersonasUseValidMimics guarantees every persona resolves to mimic types the CLI
// actually knows and can assign a port to — so a persona can never reference a mimic
// that isn't in the library.
func TestPersonasUseValidMimics(t *testing.T) {
	for _, name := range persona.Names() {
		set, err := persona.Resolve(name, "wiring-"+name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, m := range set {
			if !validMimicType(m) {
				t.Errorf("persona %s references unknown mimic %q", name, m)
			}
			if mimicPort(m, 22) == 0 {
				t.Errorf("persona %s mimic %q has no port mapping", name, m)
			}
		}
	}
}
