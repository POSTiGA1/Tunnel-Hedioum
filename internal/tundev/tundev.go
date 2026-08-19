// Package tundev brings up an OS-level TUN interface for one foreign node and
// wires its traffic, via a userspace network stack (gVisor), into that node's
// local SOCKS5 port — so the tunnel is reachable as a plain network interface,
// not only as a SOCKS proxy.
//
// It is deliberately per-node: every TUN-enabled foreign node gets its own
// interface name and /24, its own gVisor stack, and its own SOCKS-backed
// handler, so several foreign exits can run side by side without colliding.
//
// TUN is opt-in. A node with TunEnabled=false costs nothing here; only enabled
// nodes ever open an interface. The interface is never made the host default
// route — callers reach it through policy routing / Xray sendThrough / an
// upstream router — so the hub's own egress and SSH stay direct (no lockout).
//
// Only Linux has a real implementation; other platforms return an error from
// Start so the caller can fall back to SOCKS-only cleanly.
package tundev

// Node describes one TUN instance to bring up for a foreign node.
type Node struct {
	// Name is the interface name, e.g. "hedioum0".
	Name string
	// Addr is the gateway address in CIDR form, e.g. "10.200.0.1/24"; the host
	// part (.1) becomes this node's gateway IP and the interface address.
	Addr string
	// SocksAddr is the node's local SOCKS5 endpoint, e.g. "127.0.0.1:40001",
	// through which all TUN traffic egresses to the foreign exit.
	SocksAddr string
	// EnableDNS runs a small :53 forwarder bound to the gateway IP that resolves
	// through the tunnel (no local DNS leak), for gateway/router clients.
	EnableDNS bool
	// MTU overrides the interface MTU; 0 uses the default (1500).
	MTU uint32
}
