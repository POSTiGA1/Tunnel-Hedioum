package main

import (
	"testing"

	"github.com/hedioum/Hedioum-Pool-Tunnel/config"
)

func TestCurrentForeignPorts(t *testing.T) {
	cfg := &config.AppConfig{Role: "foreign", Mimics: []config.MimicListener{
		{Type: "tls", Port: 2087}, {Type: "directadmin", Port: 2222},
	}}
	p := currentForeignPorts(cfg)
	if p["tls"] != 2087 {
		t.Fatalf("tls port = %d, want the configured 2087", p["tls"])
	}
	if p["smtp"] != 587 {
		t.Fatalf("absent mimic smtp should default to 587, got %d", p["smtp"])
	}
	if p["directadmin"] != 2222 {
		t.Fatalf("directadmin port = %d", p["directadmin"])
	}
}

func TestBuildForeignMimicList(t *testing.T) {
	ports := map[string]int{"ssh": 2200, "tls": 443, "directadmin": 2222}
	list := buildForeignMimicList([]string{"ssh", "tls", "directadmin"}, ports, 2022, "vpn.example.com")
	if len(list) != 3 {
		t.Fatalf("len = %d", len(list))
	}
	// ssh -> decoy set, no SNI, caller port.
	if list[0].Type != "ssh" || list[0].Port != 2200 || list[0].Decoy != "127.0.0.1:2022" || list[0].ServerName != "" {
		t.Fatalf("ssh listener wrong: %+v", list[0])
	}
	// non-ssh -> SNI set, no decoy.
	if list[1].Type != "tls" || list[1].Port != 443 || list[1].ServerName != "vpn.example.com" || list[1].Decoy != "" {
		t.Fatalf("tls listener wrong: %+v", list[1])
	}
	if list[2].Type != "directadmin" || list[2].Port != 2222 {
		t.Fatalf("directadmin listener wrong: %+v", list[2])
	}
}

func TestNodePortsAndEndpoints(t *testing.T) {
	node := config.ForeignNode{Endpoints: []config.Endpoint{
		{Target: "1.2.3.4:2200", Mimic: "ssh"},
		{Target: "1.2.3.4:8443", Mimic: "tls", ServerName: "d.example.com"},
	}}
	p := currentNodePorts(node)
	if p["ssh"] != 2200 || p["tls"] != 8443 {
		t.Fatalf("ports from endpoints wrong: %v", p)
	}
	if p["imaps"] != 993 {
		t.Fatalf("absent mimic should default: %v", p["imaps"])
	}
	if got := nodeTargetIP(node); got != "1.2.3.4" {
		t.Fatalf("nodeTargetIP = %q", got)
	}
	if got := nodeMimicTypesCSV(node); got != "ssh,tls" {
		t.Fatalf("nodeMimicTypesCSV = %q", got)
	}
	if got := firstEndpointSNI(node); got != "d.example.com" {
		t.Fatalf("firstEndpointSNI = %q", got)
	}

	eps := buildEndpoints("9.9.9.9", []string{"ssh", "tls"}, map[string]int{"ssh": 22, "tls": 443}, "sni.example.com")
	if eps[0].Target != "9.9.9.9:22" || eps[0].ServerName != "" { // ssh has no SNI
		t.Fatalf("ssh endpoint wrong: %+v", eps[0])
	}
	if eps[1].Target != "9.9.9.9:443" || eps[1].ServerName != "sni.example.com" {
		t.Fatalf("tls endpoint wrong: %+v", eps[1])
	}
}

func TestExtractArgValue(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--alias", "DE-01", "--bw", "12"}, "DE-01"},
		{[]string{"--alias=DE-02"}, "DE-02"},
		{[]string{"-alias", "DE-03"}, "DE-03"},
		{[]string{"--bw", "12"}, ""},
	}
	for _, c := range cases {
		if got := extractArgValue(c.args, "alias"); got != c.want {
			t.Errorf("extractArgValue(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}

func TestResolveTokenEdit(t *testing.T) {
	// keep by default
	if tok, note, err := resolveTokenEdit("current-tok", "", false); err != nil || tok != "current-tok" || note != "unchanged" {
		t.Fatalf("keep: %q %q %v", tok, note, err)
	}
	// explicit wins
	if tok, _, err := resolveTokenEdit("current-tok", "abcdef0123456789", false); err != nil || tok != "abcdef0123456789" {
		t.Fatalf("explicit: %q %v", tok, err)
	}
	// invalid explicit errors
	if _, _, err := resolveTokenEdit("current-tok", "nothex!!", false); err == nil {
		t.Fatal("invalid explicit token should error")
	}
	// rotate generates a fresh, different, valid token
	tok, note, err := resolveTokenEdit("current-tok", "", true)
	if err != nil || tok == "current-tok" || note != "rotated (regenerated)" {
		t.Fatalf("rotate: %q %q %v", tok, note, err)
	}
	if err := validToken(tok); err != nil {
		t.Fatalf("rotated token invalid: %v", err)
	}
}
