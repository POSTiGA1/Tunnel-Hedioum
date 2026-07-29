package mimic

import (
	"io"
	"net"
	"testing"
)

// TestStartTLSRoundTrip: for SMTP and IMAP, a plaintext STARTTLS prologue followed
// by the TLS mimic (channel-bound auth) establishes an authenticated echo channel.
func TestStartTLSRoundTrip(t *testing.T) {
	for _, proto := range []string{"smtp", "imap"} {
		proto := proto
		t.Run(proto, func(t *testing.T) {
			const token = "starttls-round-trip-secret"
			tlsSrv, ln := newTLSServer(t, token)
			defer ln.Close()
			srv := &StartTLSMimic{Proto: proto, TLS: tlsSrv}

			srvErr := make(chan error, 1)
			go func() {
				conn, err := ln.Accept()
				if err != nil {
					srvErr <- err
					return
				}
				authed, _, err := srv.Accept(conn)
				if err != nil {
					srvErr <- err
					return
				}
				buf := make([]byte, 5)
				if _, err := io.ReadFull(authed, buf); err != nil {
					srvErr <- err
					return
				}
				_, err = authed.Write(buf)
				srvErr <- err
			}()

			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			cli := &StartTLSClient{Proto: proto, TLS: &TLSClient{Token: token, ServerName: "example"}}
			authed, err := cli.Dial(conn)
			if err != nil {
				t.Fatalf("client dial: %v", err)
			}
			if _, err := authed.Write([]byte("mail!")); err != nil {
				t.Fatal(err)
			}
			got := make([]byte, 5)
			if _, err := io.ReadFull(authed, got); err != nil {
				t.Fatal(err)
			}
			if string(got) != "mail!" {
				t.Fatalf("echo mismatch: %q", got)
			}
			if err := <-srvErr; err != nil {
				t.Fatalf("server side: %v", err)
			}
		})
	}
}

// TestStartTLSPrologueOnly verifies the server prologue emits the expected banner
// and advances to the TLS handshake point (a plain probe that never sends the TLS
// ClientHello simply stalls, which the caller treats as a decoy candidate).
func TestStartTLSPrologueOnly(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	done := make(chan error, 1)
	go func() { done <- serverStartTLSPrologue(s, "smtp") }()

	// Drive the client half of the SMTP STARTTLS exchange.
	if err := clientDrainSMTP(c); err != nil {
		t.Fatalf("client prologue: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("server prologue: %v", err)
	}
}

func clientDrainSMTP(c net.Conn) error {
	if _, err := stReadLine(c); err != nil { // 220 greeting
		return err
	}
	if err := stWrite(c, "EHLO test"); err != nil {
		return err
	}
	for {
		l, err := stReadLine(c)
		if err != nil {
			return err
		}
		if len(l) >= 4 && l[3] == ' ' { // final 250 line
			break
		}
	}
	if err := stWrite(c, "STARTTLS"); err != nil {
		return err
	}
	_, err := stReadLine(c) // 220 ready
	return err
}
