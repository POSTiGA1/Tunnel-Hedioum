package mimic

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Reusable "TLS + HTTP login panel" decoy harness. Every hosting/ops panel we mimic
// (DirectAdmin, cPanel, WHM, Grafana, ...) is the same shape: a TLS listener whose
// unauthenticated peers get a faithful login page + branding assets + a few API
// stubs, so the port reads as an ordinary panel to scanners and humans. Only the
// route table, assets, and login HTML differ per product; this file owns the shared
// keep-alive HTTP/1.1 loop, the response writer, and embedded-asset serving.
//
// Fidelity note: each panel emits its real product's exact response headers, in the
// product's order. The writer injects only Date (first) and Connection (last); the
// panel's route supplies every header in between (Content-Type, Content-Length, and
// the product's security/cache headers) verbatim. So a profile can be byte-faithful
// to a captured real panel.

// panelResp is one HTTP/1.1 response. middle carries every header between the
// writer-injected Date (first) and Connection (last), in emission order.
type panelResp struct {
	status int
	reason string
	middle [][2]string
	body   []byte
}

// panelRoute produces the response for one request.
type panelRoute func(req *http.Request) panelResp

// servePanel handles one (keep-alive) connection as a login panel would, routing
// each request through route. It bounds the keep-alive loop (a browser fetches
// several assets) and mirrors the timeouts a real panel front-end uses.
func servePanel(conn net.Conn, route panelRoute) {
	br := bufio.NewReader(conn)
	for i := 0; i < 64; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		if req.Body != nil {
			_, _ = io.Copy(io.Discard, req.Body)
			_ = req.Body.Close()
		}
		keepAlive := req.ProtoAtLeast(1, 1) && !req.Close
		_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
		if err := writePanel(conn, route(req), keepAlive); err != nil {
			return
		}
		if !keepAlive {
			return
		}
	}
}

// writePanel emits Date first, the panel's ordered middle headers, then Connection
// last — the header frame shared by every panel; the values in between are the
// product's own.
func writePanel(w io.Writer, r panelResp, keepAlive bool) error {
	conn := "close"
	if keepAlive {
		conn = "keep-alive"
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "HTTP/1.1 %d %s\r\n", r.status, r.reason)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123))
	for _, h := range r.middle {
		fmt.Fprintf(&b, "%s: %s\r\n", h[0], h[1])
	}
	fmt.Fprintf(&b, "Connection: %s\r\n\r\n", conn)
	b.Write(r.body)
	_, err := w.Write(b.Bytes())
	return err
}

// panelText is a minimal response (Content-Type + Content-Length only) — used for
// fallbacks such as an asset miss.
func panelText(status int, reason, contentType, body string) panelResp {
	return panelResp{
		status: status, reason: reason,
		middle: [][2]string{
			{"Content-Type", contentType},
			{"Content-Length", fmt.Sprintf("%d", len(body))},
		},
		body: []byte(body),
	}
}

// panelAssetResp serves one embedded branding asset with the immutable-cache headers
// a real panel uses for its fingerprinted asset paths.
func panelAssetResp(fsys embed.FS, dir, name, contentType string) panelResp {
	b, err := fsys.ReadFile(dir + "/" + name)
	if err != nil {
		return panelText(404, "Not Found", "text/html; charset=utf-8", "Not Found")
	}
	return panelResp{
		status: 200, reason: "OK",
		middle: [][2]string{
			{"Content-Type", contentType},
			{"Content-Length", fmt.Sprintf("%d", len(b))},
			{"Cache-Control", "public, max-age=31536000, immutable"},
			{"X-Content-Type-Options", "nosniff"},
		},
		body: b,
	}
}
