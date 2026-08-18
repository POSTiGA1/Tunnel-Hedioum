//go:build linux

package tundev

import (
	"bytes"
	"testing"
)

func TestDNSMessageRoundTrip(t *testing.T) {
	cases := [][]byte{
		{0x12, 0x34, 0x01, 0x00}, // a tiny header
		bytes.Repeat([]byte{0xAB}, 512),
		bytes.Repeat([]byte{0x00, 0xFF}, 600), // > 512, EDNS-sized
	}
	for _, msg := range cases {
		var buf bytes.Buffer
		if err := writeDNSMessage(&buf, msg); err != nil {
			t.Fatalf("write: %v", err)
		}
		if buf.Len() != len(msg)+2 {
			t.Fatalf("framed length = %d, want %d", buf.Len(), len(msg)+2)
		}
		got, err := readDNSMessage(&buf)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if !bytes.Equal(got, msg) {
			t.Fatalf("round-trip mismatch: got %d bytes, want %d", len(got), len(msg))
		}
	}
}

func TestReadDNSMessageRejectsEmpty(t *testing.T) {
	// A zero length prefix must be rejected, not treated as a valid 0-byte message.
	if _, err := readDNSMessage(bytes.NewReader([]byte{0x00, 0x00})); err == nil {
		t.Fatal("expected error for zero-length DNS message")
	}
}

func TestReadDNSMessageShortBody(t *testing.T) {
	// Declares 4 bytes but only 2 follow.
	if _, err := readDNSMessage(bytes.NewReader([]byte{0x00, 0x04, 0xAA, 0xBB})); err == nil {
		t.Fatal("expected error for truncated DNS message body")
	}
}
