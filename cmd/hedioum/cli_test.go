package main

import "testing"

func TestValidPort(t *testing.T) {
	for _, p := range []int{1, 22, 443, 65535} {
		if err := validPort(p); err != nil {
			t.Errorf("validPort(%d) unexpected error: %v", p, err)
		}
	}
	for _, p := range []int{0, -1, 65536, 99999} {
		if err := validPort(p); err == nil {
			t.Errorf("validPort(%d) should error", p)
		}
	}
}

func TestValidIP(t *testing.T) {
	for _, s := range []string{"1.2.3.4", "::1", "2001:db8::1"} {
		if err := validIP(s); err != nil {
			t.Errorf("validIP(%q): %v", s, err)
		}
	}
	for _, s := range []string{"", "bad", "1.2.3.4.5", "example.com"} {
		if err := validIP(s); err == nil {
			t.Errorf("validIP(%q) should error", s)
		}
	}
}

func TestValidTarget(t *testing.T) {
	for _, s := range []string{"1.2.3.4:443", "[::1]:22", "example.com:80"} {
		if err := validTarget(s); err != nil {
			t.Errorf("validTarget(%q): %v", s, err)
		}
	}
	for _, s := range []string{"noport", "1.2.3.4:99999", "host:0", ":443", "1.2.3.4:abc"} {
		if err := validTarget(s); err == nil {
			t.Errorf("validTarget(%q) should error", s)
		}
	}
}

func TestValidToken(t *testing.T) {
	if err := validToken("acb329f4a1e30d0b"); err != nil {
		t.Errorf("valid hex token rejected: %v", err)
	}
	for _, s := range []string{"", "short", "zzzz1234", "not-hex!"} {
		if err := validToken(s); err == nil {
			t.Errorf("validToken(%q) should error", s)
		}
	}
}
