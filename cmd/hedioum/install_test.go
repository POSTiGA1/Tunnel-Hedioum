package main

import (
	"strings"
	"testing"
)

// TestRenderUnitCapabilities verifies the hub gains CAP_NET_ADMIN (for TUN) while
// the foreign stays bound to just CAP_NET_BIND_SERVICE.
func TestRenderUnitCapabilities(t *testing.T) {
	hub := renderUnit(true)
	if !strings.Contains(hub, "CapabilityBoundingSet=CAP_NET_BIND_SERVICE CAP_NET_ADMIN") {
		t.Errorf("hub unit missing CAP_NET_ADMIN in bounding set:\n%s", hub)
	}
	if !strings.Contains(hub, "AmbientCapabilities=CAP_NET_BIND_SERVICE CAP_NET_ADMIN") {
		t.Errorf("hub unit missing CAP_NET_ADMIN in ambient set:\n%s", hub)
	}

	foreign := renderUnit(false)
	// The directive lines (not the explanatory comment) must be bind-service only.
	if !strings.Contains(foreign, "CapabilityBoundingSet=CAP_NET_BIND_SERVICE\n") {
		t.Errorf("foreign bounding set must be CAP_NET_BIND_SERVICE only:\n%s", foreign)
	}
	if !strings.Contains(foreign, "AmbientCapabilities=CAP_NET_BIND_SERVICE\n") {
		t.Errorf("foreign ambient set must be CAP_NET_BIND_SERVICE only:\n%s", foreign)
	}
	if strings.Contains(foreign, "CAP_NET_BIND_SERVICE CAP_NET_ADMIN") {
		t.Errorf("foreign unit must NOT grant CAP_NET_ADMIN on a directive line:\n%s", foreign)
	}
	// The template must not leak Go template syntax into the rendered unit.
	for _, u := range []string{hub, foreign} {
		if strings.Contains(u, "{{") || strings.Contains(u, "}}") {
			t.Errorf("unrendered template directive left in unit:\n%s", u)
		}
	}
}
