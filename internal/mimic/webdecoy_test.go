package mimic

import (
	"bytes"
	"io"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestServeWebDecoy(t *testing.T) {
	c, s := net.Pipe()
	go func() {
		defer s.Close()
		ServeWebDecoy(s)
	}()
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(4 * time.Second))
	if _, err := c.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(c)
	for _, want := range []string{"HTTP/1.1 200 OK", "Server: Apache/2.4.52 (Ubuntu)", "It works", "Apache2 Ubuntu Default Page"} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("decoy missing %q in: %q", want, got)
		}
	}
}

// TestServeDirectAdminWeb verifies the DirectAdmin web-port default: the exact
// 47-byte body and the Apache/2 signature a real DA server presents.
func TestServeDirectAdminWeb(t *testing.T) {
	c, s := net.Pipe()
	go func() {
		defer s.Close()
		ServeDirectAdminWeb(s)
	}()
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(4 * time.Second))
	if _, err := c.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(c)
	for _, want := range []string{
		"HTTP/1.1 200 OK",
		"Server: Apache/2\r\n",
		"Vary: User-Agent",
		"Accept-Ranges: bytes",
		"<html>webserver is functioning normally</html>",
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("DA web decoy missing %q in: %q", want, got)
		}
	}
	// The body must be the exact 47-byte file (Content-Length correctness).
	if !bytes.Contains(got, []byte("Content-Length: 47")) {
		t.Fatalf("DA web decoy Content-Length wrong: %q", got)
	}
}

// TestWebDecoyFor maps persona styles to the right raw decoy.
func TestWebDecoyFor(t *testing.T) {
	ptr := func(f func(net.Conn)) uintptr { return reflect.ValueOf(f).Pointer() }
	if ptr(WebDecoyFor("directadmin")) != ptr(ServeDirectAdminWeb) {
		t.Fatal("directadmin style should map to ServeDirectAdminWeb")
	}
	if ptr(WebDecoyFor("apache")) != ptr(ServeWebDecoy) {
		t.Fatal("apache style should map to ServeWebDecoy")
	}
	if ptr(WebDecoyFor("")) != ptr(ServeWebDecoy) {
		t.Fatal("empty style should fall back to ServeWebDecoy")
	}
}
