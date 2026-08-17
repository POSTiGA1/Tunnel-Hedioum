package mimic

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

// Docker Registry (distribution) decoy for the :5000 TLS mimic. A probe or a
// `docker`/registry client that reaches the port without the tunnel token is
// answered exactly as a real token-protected Registry v2 does: the well-known
// `GET /v2/` API check returns 401 with the `Docker-Distribution-Api-Version:
// registry/2.0` marker and a Bearer `WWW-Authenticate` challenge, so the port
// reads as an ordinary private container registry to scanners and tooling.
//
// It never authenticates, stores, or serves any image data — display-only
// camouflage, reusing the shared panel harness (panel.go) since Registry v2 is
// plain HTTP/1.1 + JSON.

// ServeDockerRegistry handles one (keep-alive) connection as a Registry v2 API would.
func ServeDockerRegistry(conn net.Conn) { servePanel(conn, routeDocker) }

func routeDocker(req *http.Request) panelResp {
	host := req.Host
	if host == "" {
		host, _ = os.Hostname()
	}
	switch {
	case req.URL.Path == "/v2" || req.URL.Path == "/v2/":
		// The canonical registry API version check — challenged when auth is on.
		return dockerUnauthorized(host)
	case strings.HasPrefix(req.URL.Path, "/v2/"):
		// Any repository/manifest/blob request without a token is challenged too.
		return dockerUnauthorized(host)
	default:
		// distribution has no browser root; it returns Go's default 404.
		return dockerNotFound()
	}
}

// dockerUnauthorized reproduces distribution's token-auth challenge for /v2/*.
func dockerUnauthorized(host string) panelResp {
	body := `{"errors":[{"code":"UNAUTHORIZED","message":"authentication required","detail":null}]}` + "\n"
	realm := fmt.Sprintf("Bearer realm=%q,service=%q", "https://"+host+"/auth", host)
	return panelResp{
		status: 401, reason: "Unauthorized",
		middle: [][2]string{
			{"Content-Type", "application/json; charset=utf-8"},
			{"Docker-Distribution-Api-Version", "registry/2.0"},
			{"Www-Authenticate", realm},
			{"X-Content-Type-Options", "nosniff"},
			{"Content-Length", fmt.Sprintf("%d", len(body))},
		},
		body: []byte(body),
	}
}

// dockerNotFound mirrors the Go net/http default 404 that distribution serves off /.
func dockerNotFound() panelResp {
	body := "404 page not found\n"
	return panelResp{
		status: 404, reason: "Not Found",
		middle: [][2]string{
			{"Content-Type", "text/plain; charset=utf-8"},
			{"X-Content-Type-Options", "nosniff"},
			{"Content-Length", fmt.Sprintf("%d", len(body))},
		},
		body: []byte(body),
	}
}
