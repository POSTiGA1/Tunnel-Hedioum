//go:build linux

package tundev

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// Gateway mode turns a TUN node into a transparent L3 gateway: transit traffic
// arriving on the LAN-facing interface is policy-routed into the TUN (and so out
// the tunnel), exactly like a WireGuard/L2TP egress interface. The upstream router
// decides WHAT reaches us (via routing marks); we tunnel whatever transit arrives.
//
// The rule is keyed on the INCOMING interface (`iif`), not the source subnet, so
// that (a) we never need to enumerate LAN subnets, and (b) the return path does not
// loop: LAN→internet enters on the LAN iface → TUN; internet→LAN replies enter on
// the TUN → main table → back out the LAN iface; our own tunnel dials are locally
// generated → main table. All via netlink (rtnetlink) — no external `ip` binary.

const (
	// gwTableBase/gwRuleBase are offset by the TUN index so multiple gateway nodes
	// never collide on a routing table id or rule priority. 0x6864 == "hd".
	gwTableBase = 0x6864
	gwRuleBase  = 0x6864
)

// gatewayState records what enableGateway created, for a clean teardown.
type gatewayState struct {
	table           int
	rules           []*netlink.Rule
	restoreIPv4Fwd  bool // we flipped net.ipv4.ip_forward 0→1; restore on disable
	ipForwardV4Path string
}

// enableGateway configures ip_forward + a policy route so transit traffic on
// gwIface is steered into tunName's netstack. lanSubnets, if non-empty, scopes the
// rule to those source ranges (defense-in-depth); otherwise all transit is forwarded.
func enableGateway(tunName, gwIface string, lanSubnets []string) (*gatewayState, error) {
	link, err := netlink.LinkByName(tunName)
	if err != nil {
		return nil, fmt.Errorf("gateway: tun %q not found: %w", tunName, err)
	}

	if gwIface == "" {
		gwIface, err = detectLANIface(tunName)
		if err != nil {
			return nil, fmt.Errorf("gateway: could not auto-detect LAN interface: %w", err)
		}
	}
	if _, err := net.InterfaceByName(gwIface); err != nil {
		return nil, fmt.Errorf("gateway: LAN interface %q not found: %w", gwIface, err)
	}

	idx := tunIndex(tunName)
	table := gwTableBase + idx

	gs := &gatewayState{table: table, ipForwardV4Path: "/proc/sys/net/ipv4/ip_forward"}

	// 1. Ensure IPv4 forwarding. Best-effort: in a container /proc/sys/net is often
	//    read-only (Docker default), but forwarding may already be on (RouterOS, or
	//    `docker run --sysctl net.ipv4.ip_forward=1`). Warn — don't abort — so the
	//    routes/rules below still get installed and the gateway works once forwarding
	//    is on by whatever means.
	changed, on := ensureIPForward(gs.ipForwardV4Path)
	gs.restoreIPv4Fwd = changed
	if !on && !changed {
		slog.Warn("gateway: ip_forward is off and could not be set here; " +
			"start the container with --sysctl net.ipv4.ip_forward=1 (RouterOS enables it itself)")
	}

	// 2. default route via the TUN in our dedicated table (a dev/link route — the TUN
	//    is point-to-point, no next-hop gateway). netlink needs an explicit 0.0.0.0/0
	//    Dst rather than a nil one.
	route := &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       &net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)},
		Table:     table,
		Scope:     netlink.SCOPE_LINK,
	}
	if err := netlink.RouteReplace(route); err != nil {
		gs.disable()
		return nil, fmt.Errorf("gateway: add default route in table %d: %w", table, err)
	}

	// 3. RouterOS 7.22 regression guard: normalize consecutive 1/2/3 rule priorities
	//    so our injected rule is actually consulted. No-op on sane systems.
	normalizeRulePrioritiesIfBroken()

	// 4. Policy rule(s): iif gwIface [from subnet] lookup table.
	priority := gwRuleBase + idx
	if len(lanSubnets) == 0 {
		if err := gs.addRule(gwIface, "", priority); err != nil {
			gs.disable()
			return nil, err
		}
	} else {
		for i, sub := range lanSubnets {
			if err := gs.addRule(gwIface, sub, priority+i); err != nil {
				gs.disable()
				return nil, err
			}
		}
	}

	slog.Info("gateway mode active", "tun", tunName, "iface", gwIface, "table", table,
		"scoped", len(lanSubnets) > 0)
	return gs, nil
}

// addRule installs one ip rule (iif [from src] lookup table @ priority) and records it.
func (gs *gatewayState) addRule(iif, srcCIDR string, priority int) error {
	rule := netlink.NewRule()
	rule.IifName = iif
	rule.Table = gs.table
	rule.Priority = priority
	if srcCIDR != "" {
		_, ipnet, err := net.ParseCIDR(srcCIDR)
		if err != nil {
			return fmt.Errorf("gateway: bad gateway-lan %q: %w", srcCIDR, err)
		}
		rule.Src = ipnet
	}
	if err := netlink.RuleAdd(rule); err != nil {
		return fmt.Errorf("gateway: add rule iif %s table %d: %w", iif, gs.table, err)
	}
	gs.rules = append(gs.rules, rule)
	return nil
}

// disable tears down the rules, the table route, and restores ip_forward if we set it.
func (gs *gatewayState) disable() {
	if gs == nil {
		return
	}
	for _, r := range gs.rules {
		_ = netlink.RuleDel(r)
	}
	// Flush our table (removes the default-via-tun route).
	if routes, err := netlink.RouteListFiltered(unix.AF_INET,
		&netlink.Route{Table: gs.table}, netlink.RT_FILTER_TABLE); err == nil {
		for i := range routes {
			_ = netlink.RouteDel(&routes[i])
		}
	}
	if gs.restoreIPv4Fwd {
		_ = os.WriteFile(gs.ipForwardV4Path, []byte("0\n"), 0644)
	}
}

// ensureIPForward makes sure net.ipv4.ip_forward is 1. It returns (changed, on):
// changed=true if we flipped it 0→1 (so we restore it on teardown), on=true if
// forwarding is now enabled. A failed write (read-only /proc/sys in a container) is
// not fatal — forwarding may already be on, or be enabled out-of-band.
func ensureIPForward(path string) (changed, on bool) {
	cur := strings.TrimSpace(string(mustRead(path)))
	if cur == "1" {
		return false, true
	}
	if err := os.WriteFile(path, []byte("1\n"), 0644); err != nil {
		return false, false
	}
	return true, true
}

func mustRead(path string) []byte {
	b, _ := os.ReadFile(path)
	return b
}

// detectLANIface picks the primary non-loopback, non-TUN interface that is up
// (usually "eth0" inside a container).
func detectLANIface(tunName string) (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagLoopback != 0 || ifi.Flags&net.FlagUp == 0 {
			continue
		}
		if ifi.Name == tunName || strings.HasPrefix(ifi.Name, "hedioum") {
			continue
		}
		if addrs, _ := ifi.Addrs(); len(addrs) > 0 {
			return ifi.Name, nil
		}
	}
	return "", fmt.Errorf("no eligible LAN interface found")
}

// tunIndex parses the trailing number from "hedioumN" (0 on failure).
func tunIndex(name string) int {
	i := strings.LastIndexFunc(name, func(r rune) bool { return r < '0' || r > '9' })
	if i >= 0 && i+1 < len(name) {
		if n, err := strconv.Atoi(name[i+1:]); err == nil {
			return n
		}
	}
	return 0
}

// normalizeRulePrioritiesIfBroken works around RouterOS 7.22, which set container
// ip-rule priorities to the consecutive 1/2/3 (local/main/default) with no gap,
// leaving no room for a policy rule to take effect. If that exact pattern is
// present, move them back to sane priorities (local=200, main/default at the top).
// Reverted upstream in 7.23rc2; this is a safe no-op elsewhere.
func normalizeRulePrioritiesIfBroken() {
	rules, err := netlink.RuleList(unix.AF_INET)
	if err != nil {
		return
	}
	var local, main, def *netlink.Rule
	for i := range rules {
		switch {
		case rules[i].Priority == 1 && rules[i].Table == unix.RT_TABLE_LOCAL:
			local = &rules[i]
		case rules[i].Priority == 2 && rules[i].Table == unix.RT_TABLE_MAIN:
			main = &rules[i]
		case rules[i].Priority == 3 && rules[i].Table == unix.RT_TABLE_DEFAULT:
			def = &rules[i]
		}
	}
	if local == nil || main == nil || def == nil {
		return // not the broken 7.22 pattern
	}
	slog.Warn("gateway: normalizing RouterOS 7.22 container ip-rule priorities (1/2/3)")
	reprio := func(old *netlink.Rule, newPrio int) {
		_ = netlink.RuleDel(old)
		nr := netlink.NewRule()
		nr.Table = old.Table
		nr.Priority = newPrio
		_ = netlink.RuleAdd(nr)
	}
	reprio(local, 200)
	reprio(main, 2147483646)
	reprio(def, 2147483647)
}
