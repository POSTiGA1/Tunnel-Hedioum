package mimic

import (
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/securestream"
	utls "github.com/refraction-networking/utls"
)

// TLSMimic disguises the tunnel as an HTTPS server: a real TLS handshake (with a
// self-signed cert) provides the single crypto layer, and a channel-bound token
// auth (tlsauth.go) proves the peer without sending the token and without a
// second encryption layer. Unauthenticated peers (browsers/probes) are served a
// plausible web response, so :443 looks like an ordinary web host.
type TLSMimic struct {
	Token     string
	Filter    *securestream.ReplayFilter
	TLSConfig *tls.Config // server config carrying the self-signed cert
	CertFP    [32]byte    // SHA-256 of our leaf cert (channel binding)
	DecoyAddr string      // web backend for unauth peers; "" = built-in page
}

func (m *TLSMimic) Accept(conn net.Conn) (net.Conn, net.Conn, error) {
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	tlsConn := tls.Server(conn, m.TLSConfig)
	if err := tlsConn.Handshake(); err != nil {
		// Not even a valid TLS ClientHello; nothing believable to serve.
		return nil, conn, err
	}
	_ = conn.SetDeadline(time.Time{})

	// Record the decrypted inner bytes so a non-client can be replayed to the web
	// decoy; read the channel-bound auth through the recorder.
	rec := &RecorderConn{Conn: tlsConn, recording: true}
	if err := serverVerifyTLS(tlsConn, rec, m.Token, m.CertFP, m.Filter); err != nil {
		return nil, buildReplayConn(tlsConn, rec), err // ErrAuth -> web decoy
	}
	rec.Stop()
	return rec, nil, nil // yamux runs directly over the TLS conn (single crypto layer)
}

// ProxyDecoy serves an unauthenticated peer a believable web response — a local
// backend if configured, otherwise a built-in minimal page.
func (m *TLSMimic) ProxyDecoy(replay net.Conn) {
	defer replay.Close()
	if m.DecoyAddr == "" {
		serveBuiltinWeb(replay)
		return
	}
	backend, err := net.DialTimeout("tcp", m.DecoyAddr, 5*time.Second)
	if err != nil {
		serveBuiltinWeb(replay) // backend down: still answer plausibly
		return
	}
	defer backend.Close()
	errCh := make(chan error, 2)
	go func() { _, e := io.Copy(backend, replay); errCh <- e }()
	go func() { _, e := io.Copy(replay, backend); errCh <- e }()
	<-errCh
}

func (m *TLSMimic) Name() string { return "tls" }

// serveBuiltinWeb answers a plausible HTTP response so an unauthenticated peer
// sees an ordinary web server.
func serveBuiltinWeb(conn net.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _ = conn.Read(make([]byte, 4096)) // consume (part of) the request
	const body = "<!doctype html><html><head><title>Welcome</title></head><body><h1>It works!</h1></body></html>"
	resp := fmt.Sprintf("HTTP/1.1 200 OK\r\nServer: nginx\r\nContent-Type: text/html\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, _ = conn.Write([]byte(resp))
}

// TLSClient is the client side of the TLS mimic. It uses uTLS to present a real
// browser's ClientHello (defeating JA3 fingerprinting) and pins the server cert
// TOFU (warn-only; the token auth is the real gate).
type TLSClient struct {
	Token      string
	ServerName string    // SNI
	Pin        *[32]byte // if set, warn on mismatch
	SavePin    func([32]byte)
}

func (c *TLSClient) Dial(conn net.Conn) (net.Conn, error) {
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	cfg := &utls.Config{ServerName: c.ServerName, InsecureSkipVerify: true}
	uconn := utls.UClient(conn, cfg, utls.HelloChrome_Auto)
	if err := uconn.Handshake(); err != nil {
		return nil, fmt.Errorf("tls handshake: %w", err)
	}
	_ = conn.SetDeadline(time.Time{})

	state := uconn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, fmt.Errorf("tls: server presented no certificate")
	}
	certFP := sha256.Sum256(state.PeerCertificates[0].Raw)

	if c.Pin != nil && *c.Pin != certFP {
		slog.Warn("TLS cert fingerprint changed (possible MITM); token auth still applies", "server", c.ServerName)
	} else if c.Pin == nil && c.SavePin != nil {
		c.SavePin(certFP) // TOFU: remember on first connect
	}

	if err := clientProveTLS(uconn, c.Token, certFP); err != nil {
		return nil, err
	}
	return uconn, nil
}

func (c *TLSClient) Name() string { return "tls" }
