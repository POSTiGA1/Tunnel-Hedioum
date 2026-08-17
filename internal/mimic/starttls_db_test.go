package mimic

import (
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"
)

// runProloguePair runs a server prologue and client prologue against each other over
// an in-memory pipe and returns both results.
func runProloguePair(proto string) (serverErr, clientErr error) {
	c, s := net.Pipe()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	_ = s.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan error, 1)
	go func() { done <- serverStartTLSPrologue(s, proto) }()
	clientErr = clientStartTLSPrologue(c, proto)
	serverErr = <-done
	c.Close()
	s.Close()
	return
}

// TestPostgresPrologueRoundTrip: our client's SSLRequest and the server's 'S' reply
// interoperate.
func TestPostgresPrologueRoundTrip(t *testing.T) {
	se, ce := runProloguePair("postgres")
	if se != nil || ce != nil {
		t.Fatalf("postgres prologue failed: server=%v client=%v", se, ce)
	}
}

// TestMySQLPrologueRoundTrip: the server greeting + client SSL-request interoperate.
func TestMySQLPrologueRoundTrip(t *testing.T) {
	se, ce := runProloguePair("mysql")
	if se != nil || ce != nil {
		t.Fatalf("mysql prologue failed: server=%v client=%v", se, ce)
	}
}

// TestPostgresProbeRejected: a non-Postgres probe (e.g. an HTTP GET) is bounced to
// the decoy, not accepted as a client.
func TestPostgresProbeRejected(t *testing.T) {
	c, s := net.Pipe()
	_ = s.SetDeadline(time.Now().Add(3 * time.Second))
	go func() {
		_, _ = c.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
	}()
	err := serverStartTLSPrologue(s, "postgres")
	c.Close()
	s.Close()
	if !errors.Is(err, errNoStartTLS) {
		t.Fatalf("HTTP probe should be rejected as non-Postgres, got %v", err)
	}
}

// TestMySQLGreetingShape: the greeting is a well-formed v10 packet — correct length
// header, protocol 10, version string, and CLIENT_SSL advertised.
func TestMySQLGreetingShape(t *testing.T) {
	g := buildMySQLGreeting()
	if len(g) < 4 {
		t.Fatal("greeting too short")
	}
	plen := int(g[0]) | int(g[1])<<8 | int(g[2])<<16
	if plen != len(g)-4 {
		t.Fatalf("length header %d != payload %d", plen, len(g)-4)
	}
	if g[3] != 0 {
		t.Fatalf("greeting sequence must be 0, got %d", g[3])
	}
	if g[4] != 0x0a {
		t.Fatalf("protocol version must be 10, got %d", g[4])
	}
	// version string starts at g[5], NUL-terminated
	end := 5
	for end < len(g) && g[end] != 0x00 {
		end++
	}
	if string(g[5:end]) != mysqlServerVersion {
		t.Fatalf("version = %q, want %q", g[5:end], mysqlServerVersion)
	}
	// capability lower is 2 bytes after: version NUL + 4 tid + 8 salt + 1 filler
	capLowOff := end + 1 + 4 + 8 + 1
	capLow := binary.LittleEndian.Uint16(g[capLowOff : capLowOff+2])
	if uint32(capLow)&mysqlClientSSL == 0 {
		t.Fatalf("greeting must advertise CLIENT_SSL, caps lower = %#x", capLow)
	}
}
