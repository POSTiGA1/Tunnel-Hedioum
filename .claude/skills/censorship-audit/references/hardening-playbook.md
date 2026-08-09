# Hardening playbook — lowering detectability without wrecking speed

Reference for the `censorship-audit` skill. Ordered by impact-per-effort for the
Hedioum codebase. Each item: **what, why, how (in Hedioum terms), cost, verify.**

---

## 1. Remove the self-signed-cert tell — REALITY or real-domain TLS  🔴 highest leverage

**Why:** the #1 passive/active signal today is a self-signed cert on a bare IP with
no real SNI. Every serious modern circumvention transport solves exactly this.

**Two options:**
- **REALITY-style** (preferred; no domain needed): on the TLS mimic, when a peer
  connects with an unexpected/absent SNI or fails auth, **proxy the TLS handshake
  to a real allow-listed site** (e.g. a big CDN host) so the prober gets that
  site's *genuine* cert and page; only real clients (who know the short-id/token)
  get tunneled. This borrows a real cert and defeats the SNI-mismatch probe.
  Requires TLS 1.3 and matching the target's ALPN. Touch points:
  `internal/mimic/tls.go` (handshake handling + decoy routing), a new
  "borrowed-cert" path replacing `tlscert.LoadOrCreate` for probes.
- **Real domain + ACME:** let the operator set a domain; fetch a Let's Encrypt cert
  (auto, no manual step) so :443 presents a CA-signed cert for a real hostname.
  Simpler, but exposes a domain (CT logs) and needs DNS.

**Cost:** ~0 throughput. Implementation complexity is the only cost.
**Verify:** SNI-mismatch probe now returns a real cert; JA3S/cert no longer flags.

---

## 2. CDN / WebSocket fronting  🔴

**Why:** hides the real foreign IP entirely (censor sees only the CDN edge + the
CDN's SNI), sidesteps IP/ASN reputation and geo blocks, and inherits the CDN's
"too big to block" property. This is the danyarai.ir-style setup users already run
in front of Hedioum.

**How (Hedioum terms):** add a **VLESS/WS+TLS-style transport option** so a mimic
can run as WebSocket-over-TLS behind a CDN, or document/first-class the
"CDN → panel → Hedioum SOCKS" chain. A new mimic transport in `internal/mimic/`
that speaks WS.

**Cost:** one extra hop (latency + CDN bandwidth $$). Often used for control/light
traffic with a direct bulk path via split-routing.
**Verify:** the real foreign IP never appears in a client-side capture; SNI = CDN.

---

## 3. Shrink the connection-pool signature  🔴/🟠

**Why:** N correlated long-lived flows to one IP is a mimic-independent
traffic-analysis fingerprint (see threat-model A4).

**How:**
- **Decorrelate establishment:** the pool already staggers (`staggerDelay`) and
  jitters keep-alives — increase/ randomize the warm-up spread so SYNs don't burst.
- **Fewer, fatter pipes:** lean on yamux multiplexing + the 16MB window + BBR so
  one pipe carries more, reducing parallel-flow count. Lower default
  `min_connections`; scale up only under real load.
- **Vary per-install:** the Chaos-Mesh fluctuating distribution already makes two
  installs differ — extend it to pool *size/shape*, not just mimic mix.

**Cost:** fewer pipes can cap burst parallelism; balance against single-pipe
throughput (already high). Measure peak Mbps before/after.
**Verify:** connection-count-over-time and SYN-burst correlation look like a
browser/CDN client, not N tunnels.

---

## 4. First-byte / entropy safety (keep the exemption)  🟠

**Why:** the fully-encrypted heuristic blocks random-looking prefixes. Hedioum is
currently exempt (ASCII banner / TLS record first), but this is a *property to
protect*, not assume.

**How:** add a regression test/assertion that every mimic's first client bytes are
either a valid TLS record (`0x16 0x03`) or printable-ASCII, and that securestream's
random salt is never the first thing on the wire. Consider a small ASCII/protocol
preamble for any future raw-transport mimic.
**Cost:** ~0. **Verify:** popcount ≈ 0.5 test on the first 6 bytes must *fail*
(i.e. not look random) for every mimic.

---

## 5. Decoy diversity + realism  🟠

**Why:** the identical built-in Apache page is a fleet-wide signature; a shallow
decoy fails a determined prober.

**How:** ship several decoy templates (nginx/Apache/Caddy/IIS defaults) and pick
per-install (seeded by token) — the same idea as the mimic-distribution Chaos Mesh.
Optionally serve a real backend, or a plausible :80→:443 redirect. Make the web
decoy answer more paths/verbs like a real server. Touch: `internal/mimic/webdecoy.go`.
**Cost:** ~0. **Verify:** two installs serve *different* decoy bytes; probing more
paths still looks like a normal server.

---

## 6. Exit-IP hygiene  🟠

**Why:** a perfectly stealthy pipe behind a burned datacenter IP still fails
(Google-AI 403, ASN blocks). Cheap "unlimited" hosts recycle flagged ranges.

**How:**
- A built-in **`check-ip`** command: from the foreign, test Gemini/AI-Studio/OpenAI
  reachability + ASN/geo, and warn if the IP is flagged (the diagnostic already
  used ad-hoc in this project). Run before/at deploy.
- **Domain/GeoIP split-routing** in the SOCKS layer (it already sees the domain):
  send sensitive/AI traffic out a clean exit, bulk out a cheap one; keep `.ir` /
  Iran-GeoIP direct.
- Prefer residential/clean ranges for the primary exit; multi-node failover.
**Cost:** operational (choosing hosts). **Verify:** `check-ip` passes; AI services
return 200 through the tunnel.

---

## 7. uTLS / fingerprint maintenance  🟡

Track the uTLS library and the Chrome version its `HelloChrome_Auto` emulates;
a stale JA3 becomes a fingerprint. Add a note/CI check to bump uTLS periodically.
**Cost:** ~0. **Verify:** JA3/JA4 matches a current mainstream Chrome.

---

## 8. Timing / padding shaping  🟡 — only if measured

Traffic-shaping to mimic HTTPS timing/size *can* help against A4, but it costs
latency and throughput. **Do not add speculatively.** Only ship it if a real trace
shows a timing/size signature that actually triggers blocking, and re-measure a
real speedtest through the tunnel after.

---

## Cross-cutting rule

Pair every change with a **before/after**: a packet capture (fingerprint/probe
result) *and* a throughput/latency measurement through the full tunnel. Stealth
that halves the user's speed is not a win. The north star is **indistinguishable
from allowed traffic, at full speed.**
