package ingress

import (
	"fmt"
	mrand "math/rand/v2"
	"net"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/hedioum/Hedioum-Pool-Tunnel/config"
	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/mimic"
)

// hubYamuxConfig tunes Yamux for high-latency, high-throughput WAN links.
func hubYamuxConfig() *yamux.Config {
	c := yamux.DefaultConfig()
	c.EnableKeepAlive = false        // we run a custom randomized heartbeat
	c.MaxStreamWindowSize = 16 << 20 // 16MB: headroom for 200-300 Mbps at high RTT
	c.StreamCloseTimeout = 3 * time.Minute
	return c
}

// clientMimicFor builds the client camouflage for an endpoint's mimic type.
func clientMimicFor(ep config.Endpoint, token string) mimic.ClientMimic {
	switch ep.Mimic {
	case "tls":
		return &mimic.TLSClient{Token: token, ServerName: ep.ServerName}
	default: // "ssh"
		return &mimic.SSHClient{Token: token}
	}
}

// DialEndpoint dials one endpoint: TCP connect, mimic handshake, Yamux client.
// Shared by the pool dialer and the speedtest CLI.
func DialEndpoint(ep config.Endpoint, token string, cfg *yamux.Config) (*yamux.Session, error) {
	conn, err := net.DialTimeout("tcp", ep.Target, 8*time.Second)
	if err != nil {
		return nil, err
	}
	secureConn, err := clientMimicFor(ep, token).Dial(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("%s mimic handshake failed: %w", ep.Mimic, err)
	}
	session, err := yamux.Client(secureConn, cfg)
	if err != nil {
		secureConn.Close()
		return nil, err
	}
	return session, nil
}

// endpointDialer spreads new physical pipes across a node's endpoints with random
// per-node weights, so each server instance runs a different, shifting mix of
// mimics (Chaos Mesh v2) — there is no fixed "10 SSH + 0 others" signature for DPI.
type endpointDialer struct {
	node    config.ForeignNode
	cfg     *yamux.Config
	weights []float64
}

func newEndpointDialer(node config.ForeignNode) *endpointDialer {
	w := make([]float64, len(node.Endpoints))
	for i := range w {
		w[i] = 0.3 + mrand.Float64() // per-node random bias
	}
	return &endpointDialer{node: node, cfg: hubYamuxConfig(), weights: w}
}

// pick chooses an endpoint by weighted random (drift emerges as the pool scales
// pipes up and down over time).
func (d *endpointDialer) pick() config.Endpoint {
	if len(d.node.Endpoints) == 1 {
		return d.node.Endpoints[0]
	}
	total := 0.0
	for _, w := range d.weights {
		total += w
	}
	r := mrand.Float64() * total
	for i, w := range d.weights {
		if r -= w; r <= 0 {
			return d.node.Endpoints[i]
		}
	}
	return d.node.Endpoints[len(d.node.Endpoints)-1]
}

func (d *endpointDialer) dial() (*yamux.Session, error) {
	session, err := DialEndpoint(d.pick(), d.node.AuthToken, d.cfg)
	if err != nil {
		return nil, err
	}
	go keepAlive(session)
	return session, nil
}

// keepAlive sends a randomized-interval Yamux ping to evade DPI periodicity checks.
func keepAlive(s *yamux.Session) {
	for {
		if s.IsClosed() {
			return
		}
		time.Sleep(time.Duration(mrand.IntN(26)+20) * time.Second)
		if _, err := s.Ping(); err != nil {
			return
		}
	}
}
