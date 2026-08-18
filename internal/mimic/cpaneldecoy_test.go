package mimic

import (
	"strings"
	"testing"
)

// TestCPanelLoginPage: the cPanel login carries the cpsrvd Server tell, the real
// branding/placeholders, and the exact failed-login text.
func TestCPanelLoginPage(t *testing.T) {
	got := drivePanel(t, cpsrvdRoute("cPanel"), "GET /login")
	for _, want := range []string{
		"200 OK",
		"Server: cpsrvd/",
		"<title>cPanel Login</title>",
		"Enter your username.",
		"Enter your account password.",
		"Log in",
		"/unprotected/whitelabel/images/cpanel-logo.svg",
		"The login is invalid.",
		"linear-gradient(90deg, #011a62, #01376b)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("cPanel login missing %q:\n%.400s", want, got)
		}
	}
}

// TestCpsrvdProductTitles: WHM and Webmail reuse the template with their own title.
func TestCpsrvdProductTitles(t *testing.T) {
	for _, c := range []struct{ product, title string }{
		{"WHM", "<title>WHM Login</title>"},
		{"Webmail", "<title>Webmail Login</title>"},
	} {
		got := drivePanel(t, cpsrvdRoute(c.product), "GET /login")
		if !strings.Contains(got, c.title) || !strings.Contains(got, "Server: cpsrvd/") {
			t.Fatalf("%s login wrong:\n%.200s", c.product, got)
		}
	}
}

// TestCPanelBadLogin: a POST is answered like a real cpsrvd bad login (never accepted).
func TestCPanelBadLogin(t *testing.T) {
	got := drivePanel(t, cpsrvdRoute("cPanel"), "POST /login/?login_only=1")
	if !strings.Contains(got, `"status":0`) || !strings.Contains(got, "The login is invalid.") {
		t.Fatalf("bad login should be rejected:\n%s", got)
	}
}

// TestCPanelAssets: the logo + field icons + favicon are served with correct types.
func TestCPanelAssets(t *testing.T) {
	for _, c := range []struct{ req, ct string }{
		{"GET /unprotected/whitelabel/images/cpanel-logo.svg", "image/svg+xml"},
		{"GET /unprotected/whitelabel/images/icon-username.png", "image/png"},
		{"GET /unprotected/whitelabel/images/icon-password.png", "image/png"},
		{"GET /favicon.ico", "image/x-icon"},
	} {
		got := drivePanel(t, cpsrvdRoute("cPanel"), c.req)
		if !strings.Contains(got, "200 OK") || !strings.Contains(got, "Content-Type: "+c.ct) {
			t.Fatalf("%s: want 200 %s:\n%.160s", c.req, c.ct, got)
		}
	}
}
