package mimic

import (
	"strings"
	"testing"
)

// TestPrometheusHealthy: /-/healthy returns the exact liveness string scanners key on.
func TestPrometheusHealthy(t *testing.T) {
	got := drivePanel(t, routePrometheus, "GET /-/healthy")
	if !strings.Contains(got, "200 OK") || !strings.Contains(got, "Prometheus Server is Healthy.") {
		t.Fatalf("/-/healthy:\n%s", got)
	}
}

// TestPrometheusMetrics: /metrics exposes the self-metrics that identify Prometheus.
func TestPrometheusMetrics(t *testing.T) {
	got := drivePanel(t, routePrometheus, "GET /metrics")
	for _, want := range []string{"200 OK", "version=0.0.4", "prometheus_build_info", `version="` + promVersion + `"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("/metrics missing %q:\n%.300s", want, got)
		}
	}
}

// TestPrometheusBuildInfo: the buildinfo API reports the version as JSON.
func TestPrometheusBuildInfo(t *testing.T) {
	got := drivePanel(t, routePrometheus, "GET /api/v1/status/buildinfo")
	if !strings.Contains(got, `"status":"success"`) || !strings.Contains(got, `"version":"`+promVersion) {
		t.Fatalf("buildinfo:\n%s", got)
	}
}

// TestPrometheusRootRedirect: "/" redirects to /graph like a real Prometheus.
func TestPrometheusRootRedirect(t *testing.T) {
	got := drivePanel(t, routePrometheus, "GET /")
	if !strings.Contains(got, "302 Found") || !strings.Contains(got, "Location: /graph") {
		t.Fatalf("root redirect:\n%s", got)
	}
}

// TestPrometheusGraphTitle: the UI shell carries Prometheus' exact <title>.
func TestPrometheusGraphTitle(t *testing.T) {
	got := drivePanel(t, routePrometheus, "GET /graph")
	if !strings.Contains(got, "Prometheus Time Series Collection and Processing Server") {
		t.Fatalf("graph UI title missing:\n%s", got)
	}
}
