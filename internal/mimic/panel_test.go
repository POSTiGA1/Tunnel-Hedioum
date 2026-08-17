package mimic

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// drivePanel runs one request through servePanel(route) over an in-memory pipe.
func drivePanel(t *testing.T, route panelRoute, reqLine string) string {
	t.Helper()
	c, s := net.Pipe()
	go func() {
		servePanel(s, route)
		s.Close()
	}()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	go func() {
		_, _ = c.Write([]byte(reqLine + " HTTP/1.1\r\nHost: panel\r\nConnection: close\r\n\r\n"))
	}()
	body, _ := io.ReadAll(c)
	c.Close()
	return string(body)
}

// TestWritePanelHeaderFrame: the shared writer injects Date first and Connection
// last, and emits the panel's middle headers verbatim in order.
func TestWritePanelHeaderFrame(t *testing.T) {
	var b strings.Builder
	r := panelResp{
		status: 200, reason: "OK",
		middle: [][2]string{
			{"Content-Type", "text/html; charset=utf-8"},
			{"Content-Length", "2"},
			{"X-Frame-Options", "sameorigin"},
		},
		body: []byte("hi"),
	}
	if err := writePanel(&b, r, true); err != nil {
		t.Fatalf("writePanel: %v", err)
	}
	got := b.String()
	// Order: status, Date, <middle...>, Connection, blank line, body.
	wantOrder := []string{
		"HTTP/1.1 200 OK\r\n",
		"Date: ",
		"Content-Type: text/html; charset=utf-8\r\n",
		"Content-Length: 2\r\n",
		"X-Frame-Options: sameorigin\r\n",
		"Connection: keep-alive\r\n\r\nhi",
	}
	idx := 0
	for _, w := range wantOrder {
		j := strings.Index(got[idx:], w)
		if j < 0 {
			t.Fatalf("missing/out-of-order %q in:\n%s", w, got)
		}
		idx += j + len(w)
	}
	if strings.Contains(got, "Server:") {
		t.Fatalf("writer must not add a Server header:\n%s", got)
	}
}

// TestPanelKeepAliveClose: Connection reflects the keep-alive decision.
func TestPanelConnectionHeader(t *testing.T) {
	for _, ka := range []bool{true, false} {
		var b strings.Builder
		_ = writePanel(&b, panelText(200, "OK", "text/plain", "x"), ka)
		want := "Connection: close"
		if ka {
			want = "Connection: keep-alive"
		}
		if !strings.Contains(b.String(), want) {
			t.Fatalf("keepAlive=%v: want %q", ka, want)
		}
	}
}

// TestPanelAssetMiss: a missing embedded asset yields a clean 404 (not a panic).
func TestPanelAssetMiss(t *testing.T) {
	r := panelAssetResp(daAssets, "daassets", "does-not-exist.svg", "image/svg+xml")
	if r.status != 404 {
		t.Fatalf("want 404 for missing asset, got %d", r.status)
	}
}

// TestServePanelRoutes: servePanel drives requests through the route and honors a
// route that varies by path (smoke test of the shared loop, independent of DA).
func TestServePanelRoutes(t *testing.T) {
	route := func(req *http.Request) panelResp {
		if req.URL.Path == "/health" {
			return panelText(200, "OK", "text/plain", "ok")
		}
		return panelText(404, "Not Found", "text/plain", "nope")
	}
	got := drivePanel(t, route, "GET /health")
	if !strings.Contains(got, "200 OK") || !strings.Contains(got, "ok") {
		t.Fatalf("/health: %s", got)
	}
	got = drivePanel(t, route, "GET /other")
	if !strings.Contains(got, "404 Not Found") {
		t.Fatalf("/other: %s", got)
	}
}
