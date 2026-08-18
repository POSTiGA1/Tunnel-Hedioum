package mimic

import (
	"strings"
	"testing"
)

// TestDockerV2Challenge: GET /v2/ answers exactly like a token-protected Registry
// v2 — 401 with the registry API-version marker and a Bearer challenge.
func TestDockerV2Challenge(t *testing.T) {
	got := drivePanel(t, routeDocker, "GET /v2/")
	for _, want := range []string{
		"401 Unauthorized",
		"Docker-Distribution-Api-Version: registry/2.0",
		"Www-Authenticate: Bearer realm=",
		`"code":"UNAUTHORIZED"`,
		"authentication required",
		"Content-Type: application/json; charset=utf-8",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("/v2/ challenge missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Server:") {
		t.Fatalf("registry must not add a Server header:\n%s", got)
	}
}

// TestDockerRepoChallenge: a repository request without a token is challenged too.
func TestDockerRepoChallenge(t *testing.T) {
	got := drivePanel(t, routeDocker, "GET /v2/library/alpine/manifests/latest")
	if !strings.Contains(got, "401 Unauthorized") || !strings.Contains(got, "registry/2.0") {
		t.Fatalf("repo request should be challenged:\n%s", got)
	}
}

// TestDockerRoot: "/" returns the Go default 404 that distribution serves.
func TestDockerRoot(t *testing.T) {
	got := drivePanel(t, routeDocker, "GET /")
	if !strings.Contains(got, "404 Not Found") || !strings.Contains(got, "404 page not found") {
		t.Fatalf("root should be 404 page not found:\n%s", got)
	}
}
