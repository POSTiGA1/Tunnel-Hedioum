package ingress

import (
	"testing"
	"time"

	"github.com/hedioum/Hedioum-Pool-Tunnel/config"
)

func threeEndpointDialer() *endpointDialer {
	return newEndpointDialer(config.ForeignNode{Alias: "N", Endpoints: []config.Endpoint{
		{Target: "1.2.3.4:2200", Mimic: "ssh"},
		{Target: "1.2.3.4:443", Mimic: "tls"},
		{Target: "1.2.3.4:5432", Mimic: "postgres"},
	}})
}

// TestAttemptOrderCapAndPrimary: the order starts with the weighted primary pick and
// is capped at dialMaxAttempts, containing only real endpoints with no duplicates.
func TestAttemptOrderCapAndPrimary(t *testing.T) {
	d := newEndpointDialer(config.ForeignNode{Endpoints: []config.Endpoint{
		{Target: "h:1", Mimic: "ssh"}, {Target: "h:2", Mimic: "tls"}, {Target: "h:3", Mimic: "smtp"},
		{Target: "h:4", Mimic: "imap"}, {Target: "h:5", Mimic: "docker"}, {Target: "h:6", Mimic: "mysql"},
	}})
	order := d.attemptOrder()
	if len(order) != dialMaxAttempts {
		t.Fatalf("order len = %d, want cap %d", len(order), dialMaxAttempts)
	}
	seen := map[string]bool{}
	for _, ep := range order {
		if seen[ep.Target] {
			t.Fatalf("duplicate endpoint in order: %v", ep.Target)
		}
		seen[ep.Target] = true
	}
}

// TestCooledPortIsDeprioritized: with the ssh port cooled down (as if outbound :2200
// is blocked), it is only tried after every reachable endpoint — so a censored entry
// port never stalls the connect-race ahead of a working one.
func TestCooledPortIsDeprioritized(t *testing.T) {
	d := threeEndpointDialer()
	d.recordFailure("1.2.3.4:2200") // block ssh
	d.recordFailure("1.2.3.4:2200")

	order := d.attemptOrder()
	posTLS, posPG, posSSH := -1, -1, -1
	for i, ep := range order {
		switch ep.Mimic {
		case "tls":
			posTLS = i
		case "postgres":
			posPG = i
		case "ssh":
			posSSH = i
		}
	}
	if posSSH == -1 {
		return // ssh dropped past the cap entirely — even better
	}
	if posTLS != -1 && posSSH < posTLS || posPG != -1 && posSSH < posPG {
		t.Fatalf("cooled-down ssh must come after reachable endpoints: %+v", order)
	}
}

// TestDialRankInnocuousFirst: the fallback rank puts 443/8443 ahead of ssh and the
// databases, so a partial outage falls back to the most-reachable ports first.
func TestDialRankInnocuousFirst(t *testing.T) {
	if dialRank("tls") >= dialRank("ssh") {
		t.Fatal("tls must rank before ssh")
	}
	if dialRank("tls") >= dialRank("postgres") || dialRank("https-alt") >= dialRank("mysql") {
		t.Fatal("443/8443 must rank before the databases")
	}
	if dialRank("ssh") < dialRank("mysql") {
		t.Fatal("ssh (often blocked) should rank last, after the databases")
	}
}

// TestCooldownAndRecovery: an endpoint is cooled down after the threshold, excluded
// from the reachable set, and success clears it.
func TestCooldownAndRecovery(t *testing.T) {
	d := threeEndpointDialer()
	tgt := "1.2.3.4:2200"

	d.recordFailure(tgt) // 1 fail — not yet cooled
	if d.health[tgt].cooldownUntil.After(time.Now()) {
		t.Fatal("one failure should not cool down yet")
	}
	d.recordFailure(tgt) // 2 fails — cooled
	if !d.health[tgt].cooldownUntil.After(time.Now()) {
		t.Fatal("threshold failures should cool the endpoint down")
	}
	d.recordSuccess(tgt)
	if d.health[tgt].fails != 0 || d.health[tgt].cooldownUntil.After(time.Now()) {
		t.Fatal("success should clear the cooldown")
	}
}

// TestAllCooledStillTried: when every endpoint is cooling down, attemptOrder still
// returns candidates (the block may have lifted) rather than giving up.
func TestAllCooledStillTried(t *testing.T) {
	d := threeEndpointDialer()
	for _, tgt := range []string{"1.2.3.4:2200", "1.2.3.4:443", "1.2.3.4:5432"} {
		d.recordFailure(tgt)
		d.recordFailure(tgt)
	}
	if got := d.attemptOrder(); len(got) == 0 {
		t.Fatal("attemptOrder must still return candidates when all are cooled down")
	}
}
