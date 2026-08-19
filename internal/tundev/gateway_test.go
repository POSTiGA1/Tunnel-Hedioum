//go:build linux

package tundev

import "testing"

func TestTunIndex(t *testing.T) {
	cases := map[string]int{
		"hedioum0":  0,
		"hedioum1":  1,
		"hedioum12": 12,
		"hedioum":   0, // no trailing number → 0
		"eth0":      0,
		"":          0,
	}
	for name, want := range cases {
		if got := tunIndex(name); got != want {
			t.Errorf("tunIndex(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestGatewayTableAndPriorityDistinctPerNode(t *testing.T) {
	// Two nodes on distinct TUN indices must land on distinct route tables and rule
	// priorities so their policy rules never collide.
	t0, p0 := gwTableBase+tunIndex("hedioum0"), gwRuleBase+tunIndex("hedioum0")
	t1, p1 := gwTableBase+tunIndex("hedioum1"), gwRuleBase+tunIndex("hedioum1")
	if t0 == t1 {
		t.Errorf("route tables collide: %d == %d", t0, t1)
	}
	if p0 == p1 {
		t.Errorf("rule priorities collide: %d == %d", p0, p1)
	}
	// Table id must stay clear of the reserved local/main/default (255/254/253).
	if t0 <= 255 {
		t.Errorf("route table %d overlaps reserved tables (<=255)", t0)
	}
}
