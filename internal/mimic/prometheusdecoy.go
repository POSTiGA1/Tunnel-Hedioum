package mimic

import (
	"net"
	"net/http"
	"strconv"
	"strings"
)

// Prometheus persona decoy for the :9090 TLS mimic. Prometheus has no login page —
// it is a monitoring server — so an unauthorized probe is answered exactly as a real
// Prometheus does: the unauthenticated /-/healthy and /-/ready liveness endpoints,
// the /metrics self-exposition (build info + Go runtime metrics), the /api/v1
// query/status API, and the graph UI. These are the fingerprints a scanner uses to
// identify Prometheus, so the port reads as an ordinary monitoring instance.
//
// Display-only camouflage on the shared panel harness (Prometheus speaks HTTP/1.1).

// promVersion is advertised by /metrics and /api/v1/status/buildinfo (an LTS release).
const promVersion = "2.53.2"

// ServePrometheus handles one (keep-alive) connection as a Prometheus server would.
func ServePrometheus(conn net.Conn) { servePanel(conn, routePrometheus) }

func routePrometheus(req *http.Request) panelResp {
	switch path := req.URL.Path; {
	case path == "/-/healthy":
		return promText(200, "OK", "text/plain; charset=utf-8", "Prometheus Server is Healthy.\n")
	case path == "/-/ready":
		return promText(200, "OK", "text/plain; charset=utf-8", "Prometheus Server is Ready.\n")
	case path == "/metrics":
		return promText(200, "OK", "text/plain; version=0.0.4; charset=utf-8", promMetrics)
	case path == "/api/v1/status/buildinfo":
		return promJSON(200, "OK", `{"status":"success","data":{"version":"`+promVersion+`","revision":"","branch":"HEAD","goVersion":"go1.22.5"}}`)
	case path == "/api/v1/query" || path == "/api/v1/query_range":
		// A query with no expression — answered exactly as Prometheus does.
		return promJSON(400, "Bad Request", `{"status":"error","errorType":"bad_data","error":"invalid parameter \"query\": 1:1: parse error: no expression found in input"}`)
	case path == "/":
		return promRedirect("/graph")
	case path == "/graph" || path == "/targets" || path == "/alerts" || strings.HasPrefix(path, "/graph"):
		return promText(200, "OK", "text/html; charset=utf-8", promGraphHTML)
	default:
		return promText(404, "Not Found", "text/plain; charset=utf-8", "404 page not found\n")
	}
}

// promMetrics is a faithful slice of a Prometheus server's own /metrics output.
const promMetrics = `# HELP prometheus_build_info A metric with a constant '1' value labeled by version, revision, branch, goversion from which prometheus was built, and the goos and goarch for the build.
# TYPE prometheus_build_info gauge
prometheus_build_info{branch="HEAD",goarch="amd64",goos="linux",goversion="go1.22.5",revision="",tags="netgo,builtinassets,stringlabels",version="2.53.2"} 1
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 38
# HELP process_start_time_seconds Start time of the process since unix epoch in seconds.
# TYPE process_start_time_seconds gauge
process_start_time_seconds 1.71048192e+09
# HELP prometheus_tsdb_head_series Total number of series in the head block.
# TYPE prometheus_tsdb_head_series gauge
prometheus_tsdb_head_series 1247
`

// promGraphHTML is the Prometheus web-UI shell (the React app bootstrap), recognized
// by its exact title.
const promGraphHTML = `<!doctype html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Prometheus Time Series Collection and Processing Server</title><link rel="shortcut icon" href="/favicon.ico"></head><body><div id="root"></div></body></html>`

func promHeaders(contentType string, n int) [][2]string {
	return [][2]string{
		{"Content-Type", contentType},
		{"Content-Length", strconv.Itoa(n)},
		{"X-Content-Type-Options", "nosniff"},
	}
}

func promText(status int, reason, contentType, body string) panelResp {
	return panelResp{status: status, reason: reason, middle: promHeaders(contentType, len(body)), body: []byte(body)}
}

func promJSON(status int, reason, body string) panelResp {
	return panelResp{status: status, reason: reason, middle: promHeaders("application/json", len(body)), body: []byte(body)}
}

func promRedirect(location string) panelResp {
	return panelResp{
		status: 302, reason: "Found",
		middle: [][2]string{
			{"Location", location},
			{"Content-Type", "text/html; charset=utf-8"},
			{"Content-Length", "0"},
		},
	}
}
