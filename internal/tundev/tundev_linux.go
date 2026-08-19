//go:build linux

package tundev

import (
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/xjasonlyu/tun2socks/v2/core"
	"github.com/xjasonlyu/tun2socks/v2/core/device"
	"github.com/xjasonlyu/tun2socks/v2/core/device/tun"
	t2log "github.com/xjasonlyu/tun2socks/v2/log"
	socks5proxy "github.com/xjasonlyu/tun2socks/v2/proxy/socks5"
	"github.com/xjasonlyu/tun2socks/v2/tunnel"
	"github.com/xjasonlyu/tun2socks/v2/tunnel/statistic"
	"golang.org/x/sys/unix"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

const defaultMTU = 1500

// quietOnce silences tun2socks' own zap logger the first time any TUN starts,
// so its per-connection chatter does not interleave with our slog/journald
// output. We keep warnings and errors.
var quietOnce sync.Once

// Instance is a running TUN interface plus its network stack, SOCKS-backed
// handler, and optional DNS forwarder. Close tears all of it down.
type Instance struct {
	name  string
	dev   device.Device
	stack *stack.Stack
	tnl   *tunnel.Tunnel
	dns   *dnsForwarder
	gw    *gatewayState
}

// Name returns the interface name.
func (in *Instance) Name() string {
	if in == nil {
		return ""
	}
	return in.name
}

// Start opens the TUN interface, assigns its gateway address, brings it up, and
// pipes its traffic through the node's SOCKS5 port via a gVisor userspace stack.
func Start(n Node) (*Instance, error) {
	quietOnce.Do(func() {
		if lg, err := t2log.NewLeveled(t2log.WarnLevel); err == nil {
			t2log.SetLogger(lg)
		}
	})

	ip, ipnet, err := net.ParseCIDR(n.Addr)
	if err != nil {
		return nil, fmt.Errorf("bad tun addr %q: %w", n.Addr, err)
	}
	gw := ip.To4()
	if gw == nil {
		return nil, fmt.Errorf("tun addr must be IPv4: %s", n.Addr)
	}

	mtu := n.MTU
	if mtu == 0 {
		mtu = defaultMTU
	}

	dev, err := tun.Open(n.Name, mtu)
	if err != nil {
		return nil, fmt.Errorf("open tun %s: %w", n.Name, err)
	}

	if err := configureIface(n.Name, gw, []byte(ipnet.Mask)); err != nil {
		dev.Close()
		return nil, fmt.Errorf("configure %s: %w", n.Name, err)
	}

	p, err := socks5proxy.New(n.SocksAddr, "", "")
	if err != nil {
		dev.Close()
		return nil, fmt.Errorf("socks proxy %s: %w", n.SocksAddr, err)
	}

	tnl := tunnel.New(p, statistic.DefaultManager)
	tnl.ProcessAsync()

	st, err := core.CreateStack(&core.Config{
		LinkEndpoint:     dev,
		TransportHandler: tnl,
	})
	if err != nil {
		tnl.Close()
		dev.Close()
		return nil, fmt.Errorf("create netstack: %w", err)
	}

	in := &Instance{name: n.Name, dev: dev, stack: st, tnl: tnl}

	if n.EnableDNS {
		f, err := startDNSForwarder(gw.String(), n.SocksAddr)
		if err != nil {
			// Non-fatal: TUN egress still works; DNS is an add-on for router clients.
			slog.Warn("TUN DNS forwarder not started", "iface", n.Name, "err", err)
		} else {
			in.dns = f
		}
	}

	if n.Gateway {
		gs, err := enableGateway(n.Name, n.GatewayIface, n.GatewayLAN)
		if err != nil {
			// Non-fatal: the TUN itself is up; gateway forwarding just isn't wired.
			slog.Warn("gateway mode not enabled (TUN still up)", "iface", n.Name, "err", err)
		} else {
			in.gw = gs
		}
	}

	return in, nil
}

// Close tears down the DNS forwarder, network stack, handler, and interface.
func (in *Instance) Close() error {
	if in == nil {
		return nil
	}
	if in.gw != nil {
		in.gw.disable()
	}
	if in.dns != nil {
		in.dns.Close()
	}
	if in.stack != nil {
		in.stack.Close()
	}
	if in.tnl != nil {
		in.tnl.Close()
	}
	if in.dev != nil {
		in.dev.Close()
	}
	return nil
}

// configureIface assigns the IPv4 address + netmask to the interface and brings
// it up, using ioctls directly so we depend on no external `ip`/iproute2 binary
// (works in a FROM-scratch image and on minimal container hosts).
func configureIface(name string, ip net.IP, mask []byte) error {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)

	// Address.
	ifr, err := unix.NewIfreq(name)
	if err != nil {
		return err
	}
	if err := ifr.SetInet4Addr(ip.To4()); err != nil {
		return err
	}
	if err := unix.IoctlIfreq(fd, unix.SIOCSIFADDR, ifr); err != nil {
		return fmt.Errorf("set addr: %w", err)
	}

	// Netmask.
	ifrMask, err := unix.NewIfreq(name)
	if err != nil {
		return err
	}
	if err := ifrMask.SetInet4Addr(mask); err != nil {
		return err
	}
	if err := unix.IoctlIfreq(fd, unix.SIOCSIFNETMASK, ifrMask); err != nil {
		return fmt.Errorf("set netmask: %w", err)
	}

	// Flags: UP + RUNNING.
	ifrFlags, err := unix.NewIfreq(name)
	if err != nil {
		return err
	}
	if err := unix.IoctlIfreq(fd, unix.SIOCGIFFLAGS, ifrFlags); err != nil {
		return fmt.Errorf("get flags: %w", err)
	}
	ifrFlags.SetUint16(ifrFlags.Uint16() | unix.IFF_UP | unix.IFF_RUNNING)
	if err := unix.IoctlIfreq(fd, unix.SIOCSIFFLAGS, ifrFlags); err != nil {
		return fmt.Errorf("set flags up: %w", err)
	}
	return nil
}
