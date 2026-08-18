// Package persona groups the mimic library into coherent server identities. Instead
// of a random grab-bag of camouflage listeners, each install wears one persona — the
// SSH backbone plus a fixed set of non-SSH mimics that together look like one real,
// ordinary server (a cPanel host, a DirectAdmin host, or a DevOps/app host). This
// keeps the on-wire footprint internally consistent (a host never runs both cPanel
// and DirectAdmin, for example), while a per-install seed still makes two installs of
// the same persona differ in their optional ports.
package persona

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
)

// backbone is the long-lived mimic every persona is built on. Every server has SSH,
// so it is universally consistent; it is always added first.
const backbone = "ssh"

// A Persona is a coherent identity. Core mimics define it and are always present;
// the rest are filled deterministically from Pool (in seeded order) up to Size.
type Persona struct {
	Name string
	Core []string // always included (besides ssh); defines the identity
	Pool []string // seeded fill, drawn until Size is reached
	Size int      // number of non-ssh mimics (ssh is always added on top)
}

// Registry holds the coherent personas. Every persona's Core includes "tls" (:443),
// guaranteeing an always-reachable bootstrap path. "cpanel" and "directadmin" are
// mutually exclusive by construction (the panel-family coherence rule).
var Registry = map[string]Persona{
	"cpanel": {
		Name: "cpanel",
		Core: []string{"tls", "cpanel", "whm", "webmail"},
		Pool: []string{"https-alt", "smtp", "smtps", "imaps", "imap", "postgres", "mysql", "docker"},
		Size: 9,
	},
	"directadmin": {
		Name: "directadmin",
		Core: []string{"tls", "directadmin"},
		Pool: []string{"https-alt", "smtp", "smtps", "imaps", "imap", "postgres", "mysql", "docker", "grafana"},
		Size: 9,
	},
	"devops": {
		Name: "devops",
		Core: []string{"tls", "https-alt", "docker", "grafana", "prometheus"},
		Pool: []string{"postgres", "mysql", "smtp", "smtps", "imap", "imaps"},
		Size: 9,
	},
}

// order is the stable persona order for listing and deterministic Auto selection.
var order = []string{"cpanel", "directadmin", "devops"}

// Names lists the persona names in a stable order.
func Names() []string {
	out := make([]string, len(order))
	copy(out, order)
	return out
}

// Known reports whether name is a defined persona.
func Known(name string) bool {
	_, ok := Registry[name]
	return ok
}

// Auto deterministically selects a persona from the seed (the node's secret token),
// so a server always wears the same persona and the population spreads across all three.
func Auto(seed string) string {
	h := sha256.Sum256([]byte("hedioum-persona\x00" + seed))
	return order[int(h[0])%len(order)]
}

// Resolve returns the ordered mimic-type set for a persona: the SSH backbone first,
// then Core (in definition order), then a deterministic seeded fill from Pool up to
// Size. The result always has Size+1 entries (ssh + Size) and is coherent.
func Resolve(name, seed string) ([]string, error) {
	p, ok := Registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown persona %q (want one of %v)", name, order)
	}
	set := []string{backbone}
	seen := map[string]bool{backbone: true}
	for _, m := range p.Core {
		if !seen[m] {
			set = append(set, m)
			seen[m] = true
		}
	}
	for _, m := range seededOrder(p.Pool, seed) {
		if len(set)-1 >= p.Size {
			break
		}
		if !seen[m] {
			set = append(set, m)
			seen[m] = true
		}
	}
	if len(set)-1 != p.Size {
		return nil, fmt.Errorf("persona %q: pool too small to reach size %d", name, p.Size)
	}
	return set, nil
}

// CheckCoherence rejects an incoherent mimic set. A real server never runs both
// cPanel (cpanel/whm/webmail) and DirectAdmin, so they must not share one host.
func CheckCoherence(mimics []string) error {
	has := make(map[string]bool, len(mimics))
	for _, m := range mimics {
		has[m] = true
	}
	if (has["cpanel"] || has["whm"] || has["webmail"]) && has["directadmin"] {
		return fmt.Errorf("incoherent mimic set: cPanel family and DirectAdmin must not run on the same host")
	}
	return nil
}

// seededOrder returns items in a deterministic per-seed order: each item gets a
// hash-derived key from (seed, item), and the items are sorted by that key. Same seed
// → same order; different seeds → different fills, so same-persona installs vary.
func seededOrder(items []string, seed string) []string {
	type kv struct {
		k uint64
		v string
	}
	arr := make([]kv, len(items))
	for i, it := range items {
		h := sha256.Sum256([]byte("hedioum-fill\x00" + seed + "\x00" + it))
		arr[i] = kv{binary.BigEndian.Uint64(h[:8]), it}
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].k != arr[j].k {
			return arr[i].k < arr[j].k
		}
		return arr[i].v < arr[j].v
	})
	out := make([]string, len(items))
	for i, e := range arr {
		out[i] = e.v
	}
	return out
}
