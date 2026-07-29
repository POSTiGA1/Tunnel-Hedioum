package sysutil

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		cand, cur string
		want      bool
	}{
		{"v0.6.0", "v0.3.2", true},      // the real upgrade path
		{"v0.6.0", "v0.6.0", false},     // equal
		{"v0.6.1", "v0.6.0", true},      // patch bump
		{"v0.5.0", "v0.6.0", false},     // older release than current
		{"v1.0.0", "v0.9.9", true},      // major bump
		{"0.6.0", "v0.6.0", false},      // missing 'v' still equal
		{"v0.6", "v0.6.0", false},       // v0.6 == v0.6.0
		{"v0.6.0-rc1", "v0.6.0", false}, // prerelease suffix ignored -> equal
		{"v0.6.0", "dev", true},         // unparseable current -> update to a real release
		{"", "v0.6.0", false},           // empty candidate never newer
	}
	for _, c := range cases {
		if got := IsNewer(c.cand, c.cur); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.cand, c.cur, got, c.want)
		}
	}
}
