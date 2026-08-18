package mimic

import (
	"strings"
	"testing"
)

// TestGrafanaRootRedirect: "/" 302-redirects to /login like a real Grafana.
func TestGrafanaRootRedirect(t *testing.T) {
	got := drivePanel(t, routeGrafana, "GET /")
	if !strings.Contains(got, "302 Found") || !strings.Contains(got, "Location: /login") {
		t.Fatalf("root should redirect to /login:\n%s", got)
	}
}

// TestGrafanaLoginPage: /login serves the rendered dark-theme login with the real
// branding, form, and Grafana error text.
func TestGrafanaLoginPage(t *testing.T) {
	got := drivePanel(t, routeGrafana, "GET /login")
	for _, want := range []string{
		"200 OK",
		"<title>Grafana</title>",
		"Welcome to Grafana",
		"Email or username",
		"Log in",
		"/public/build/static/img/grafana_icon.svg",
		"Invalid username or password", // real failed-login text
		"X-Frame-Options: deny",
		"Cache-Control: no-store",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("login page missing %q:\n%.400s", want, got)
		}
	}
	if strings.Contains(got, "Server:") {
		t.Fatalf("login page must not send a Server header")
	}
}

// TestGrafanaHealth: /api/health is the unauthenticated JSON scanners fingerprint by.
func TestGrafanaHealth(t *testing.T) {
	got := drivePanel(t, routeGrafana, "GET /api/health")
	for _, want := range []string{"200 OK", "application/json", `"database":"ok"`, `"version":"` + grafanaVersion} {
		if !strings.Contains(got, want) {
			t.Fatalf("/api/health missing %q:\n%s", want, got)
		}
	}
}

// TestGrafanaAPIUnauth: other /api/* is 401 like a locked-down Grafana.
func TestGrafanaAPIUnauth(t *testing.T) {
	got := drivePanel(t, routeGrafana, "GET /api/dashboards/home")
	if !strings.Contains(got, "401 Unauthorized") || !strings.Contains(got, `"message":"Unauthorized"`) {
		t.Fatalf("protected API should be 401:\n%s", got)
	}
}

// TestGrafanaAssets: the logo + favicon are served with the right content types.
func TestGrafanaAssets(t *testing.T) {
	for _, c := range []struct{ req, ct string }{
		{"GET /public/build/static/img/grafana_icon.svg", "image/svg+xml"},
		{"GET /public/img/fav32.png", "image/png"},
	} {
		got := drivePanel(t, routeGrafana, c.req)
		if !strings.Contains(got, "200 OK") || !strings.Contains(got, "Content-Type: "+c.ct) {
			t.Fatalf("%s: want 200 %s:\n%.200s", c.req, c.ct, got)
		}
	}
}
