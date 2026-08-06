package mimic

import (
	"bytes"
	"io"
	"net"
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
