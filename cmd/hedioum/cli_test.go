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

func TestExpandMimics(t *testing.T) {
	if got, _ := expandMimics("all"); len(got) != 6 || got[0] != "ssh" || got[5] != "imaps" {
		t.Fatalf("all => %v (want all 6 mimics)", got)
	}
	if got, _ := expandMimics("tls"); len(got) != 1 || got[0] != "tls" {
		t.Fatalf("tls => %v", got)
	}
	if got, _ := expandMimics("ssh,tls"); len(got) != 2 {
		t.Fatalf("ssh,tls => %v", got)
	}
	// Implicit-TLS and STARTTLS mail mimics are all valid selectable types.
	for _, m := range []string{"smtp", "imap", "smtps", "imaps"} {
		if got, err := expandMimics(m); err != nil || len(got) != 1 || got[0] != m {
			t.Fatalf("%s => %v, %v", m, got, err)
		}
	}
	if got, _ := expandMimics("ssh,tls,smtp,imap,smtps,imaps"); len(got) != 6 {
		t.Fatalf("all-explicit => %v", got)
	}
	if _, err := expandMimics("bogus"); err == nil {
		t.Fatal("bogus mimic should error")
	}
}

func TestSpeedProfile(t *testing.T) {
	hMin, hMax, hBw, _ := speedProfile("high-speed")
	if hMin <= 10 || hMax <= 20 || hBw <= 8 {
		t.Fatalf("high-speed too low: %d/%d/%d", hMin, hMax, hBw)
	}
	bMin, bMax, bBw, bJit := speedProfile("balanced")
	if bMin != 10 || bMax != 20 || bBw != 8 || bJit != 2 {
		t.Fatalf("balanced = %d/%d/%d/%d", bMin, bMax, bBw, bJit)
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
