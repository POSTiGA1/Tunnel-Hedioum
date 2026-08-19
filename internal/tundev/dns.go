//go:build linux

package tundev

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"golang.org/x/net/proxy"
)

// dnsForwarder is a tiny UDP+TCP resolver bound to the TUN gateway IP:53 that
// forwards every query over the node's SOCKS proxy as DNS-over-TCP to a public
// resolver. Because the upstream dial goes through SOCKS, resolution happens at
// the foreign exit — there is no local DNS leak — and clients of the gateway
// (routers, LAN hosts) get a working resolver on the tunnel's own address.
type dnsForwarder struct {
	udp      net.PacketConn
	tcp      net.Listener
	dialer   proxy.Dialer
	upstream string
}

// dnsUpstreams are the resolvers we forward to (reached through the tunnel).
var dnsUpstreams = []string{"1.1.1.1:53", "8.8.8.8:53"}

func startDNSForwarder(gatewayIP, socksAddr string) (*dnsForwarder, error) {
	d, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("socks dialer: %w", err)
	}
	f := &dnsForwarder{dialer: d, upstream: dnsUpstreams[0]}

	uc, err := net.ListenPacket("udp", net.JoinHostPort(gatewayIP, "53"))
	if err != nil {
		return nil, fmt.Errorf("listen udp %s:53: %w", gatewayIP, err)
	}
	f.udp = uc
	go f.serveUDP()

	// TCP :53 is best-effort (large responses / zone transfers). A failure here
	// does not sink the forwarder; UDP is what resolvers use first.
	if tl, err := net.Listen("tcp", net.JoinHostPort(gatewayIP, "53")); err == nil {
		f.tcp = tl
		go f.serveTCP()
	}
	return f, nil
}

func (f *dnsForwarder) Close() {
	if f == nil {
		return
	}
	if f.udp != nil {
		_ = f.udp.Close()
	}
	if f.tcp != nil {
		_ = f.tcp.Close()
	}
}

// serveUDP reads datagram queries and answers each by forwarding over SOCKS.
func (f *dnsForwarder) serveUDP() {
	buf := make([]byte, 4096) // room for EDNS0-sized queries
	for {
		n, addr, err := f.udp.ReadFrom(buf)
		if err != nil {
			return // listener closed
		}
		query := make([]byte, n)
		copy(query, buf[:n])
		go func(q []byte, a net.Addr) {
			resp, err := f.forward(q)
			if err != nil {
				return
			}
			_ = f.udp.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_, _ = f.udp.WriteTo(resp, a)
		}(query, addr)
	}
}

// serveTCP answers length-prefixed TCP DNS queries.
func (f *dnsForwarder) serveTCP() {
	for {
		conn, err := f.tcp.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(10 * time.Second))
			query, err := readDNSMessage(c)
			if err != nil {
				return
			}
			resp, err := f.forward(query)
			if err != nil {
				return
			}
			_ = writeDNSMessage(c, resp)
		}(conn)
	}
}

// forward relays one raw DNS query to an upstream resolver over the SOCKS proxy
// using DNS-over-TCP, and returns the raw response.
func (f *dnsForwarder) forward(query []byte) ([]byte, error) {
	var lastErr error
	for _, up := range dnsUpstreams {
		conn, err := f.dialer.Dial("tcp", up)
		if err != nil {
			lastErr = err
			continue
		}
		_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
		if err := writeDNSMessage(conn, query); err != nil {
			conn.Close()
			lastErr = err
			continue
		}
		resp, err := readDNSMessage(conn)
		conn.Close()
		if err != nil {
			lastErr = err
			continue
		}
		return resp, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no upstream resolver reachable")
	}
	return nil, lastErr
}

// readDNSMessage reads a 2-byte length-prefixed DNS message (RFC 1035 §4.2.2).
func readDNSMessage(r io.Reader) ([]byte, error) {
	var lenBuf [2]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint16(lenBuf[:])
	if n == 0 {
		return nil, fmt.Errorf("empty dns message")
	}
	msg := make([]byte, n)
	if _, err := io.ReadFull(r, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// writeDNSMessage writes a 2-byte length-prefixed DNS message.
func writeDNSMessage(w io.Writer, msg []byte) error {
	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(msg)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err := w.Write(msg)
	return err
}
