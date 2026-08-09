# Hardening playbook — lowering detectability without wrecking speed

Reference for the `censorship-audit` skill. Ordered by impact-per-effort for the
Hedioum codebase. Each item: **what, why, how (in Hedioum terms), cost, verify.**

---

## 0. Protocol-aware connection lifecycle — SSH backbone, non-SSH churns  🔴 DO FIRST

**Why:** the operator-proven survival factor. A perfect handshake is wasted if the
*session shape* is wrong (threat-model A0). SSH is naturally long-lived and trusted
⇒ it can be the persistent backbone; TLS/mail sessions are short and bursty in the
wild ⇒ a long-lived, high-volume non-SSH flow is anomalous regardless of JA3/cert.

**How (Hedioum terms):**
- Keep **SSH** pipes long-lived (they carry the persistent bulk).
- Give every **non-SSH** pipe a **randomized lifetime** (≈5–60 min) **and a transfer
  budget** (≈1–5 GB); on hitting either, drain and close it and open a fresh pipe —
  ideally a new 5-tuple and possibly a different mimic/port. Non-SSH mimics are
  **auxiliary/overflow**, not the backbone.
- Touch points: `internal/pool` (per-connection lifetime + byte budget, keyed by
  the mimic type), `internal/ingress/dial.go` (churn + replacement), and the
  `Endpoint`/config so lifetimes are tunable.

**Cost:** more frequent handshakes on the non-SSH pipes (minor CPU/latency); a
churn event must not drop in-flight logical streams — reuse the existing
`Draining` state so streams finish before close.
**Verify:** a capture shows non-SSH flows lasting minutes (not hours) and capped in
volume, while SSH stays long; no user-visible stalls at churn boundaries.

---

## 1. Remove the self-signed-cert tell — REALITY or real-domain TLS  🔴 highest handshake leverage

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
  - **Caveat — REALITY is not a silver bullet.** Passive tools separate
    camouflage-TLS from a real browser↔site session via mishandled **post-handshake
    messages** (`NewSessionTicket`), **message-length** anomalies, and "HMAC
    tainting" (extra MAC bytes) — net4people/bbs #481. Implement it so post-handshake
    traffic is proxied **faithfully** from the real target and no extra bytes/length
    quirks are introduced; otherwise you have only moved the fingerprint.
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

## 9. IP / port rotation  🟠

**Why:** a stable single (IP, port) is the easiest thing to pin bulk flow to and to
add to a blocklist. Rotating breaks long-window correlation and raises the censor's
blocking cost.
**How:** scheduled rotation of the listener **port set** (the mimics already bind
several ports); multi-IP foreign **failover** so the hub re-homes if one IP is
throttled; optionally rotate which mimic/port a given user's non-SSH churn lands on.
**Cost:** operational (multiple IPs) + re-provisioning coordination.
**Verify:** no single (IP,port) carries a user's whole session over a long window.

## 10. Wire-level OS persona (NFQUEUE)  🟡 — optional

**Why:** removes the p0f **OS mismatch** (Chrome JA3 over a Linux TCP stack, TTL 64;
threat-model A6).
**How:** an NFQUEUE hook (ID-Spoofer style) rewrites TTL, window scale, and TCP
options on egress to a Windows profile. No base-OS change needed.
**Cost:** kernel packet-rewrite complexity + a small per-packet cost; Linux-only,
needs CAP_NET_ADMIN. Skip unless p0f is a proven vector on the target network.
**Verify:** p0f (or a TTL/options capture) reports Windows, matching the TLS JA3.

## 11. Total-shutdown reachability (white-IP)  🟠 — different goal

**Why:** during a full "international internet" cut only white-listed IPs stay
reachable (threat-model C2). This is *reachability*, not stealth.
**How:** a domestic-CDN-fronted path (an Iran-hosted edge that stays whitelisted),
or a foreign endpoint that lands on an allow-listed address. Document a
"shutdown mode" the operator can pre-stage.
**Cost:** infra (CDN account, extra endpoints). **Verify:** the path resolves and
connects when direct foreign IPs are cut (test against a known-blocked control).

## Cross-cutting rule

Pair every change with a **before/after**: a packet capture (fingerprint/probe
result) *and* a throughput/latency measurement through the full tunnel. Stealth
that halves the user's speed is not a win. The north star is **indistinguishable
from allowed traffic, at full speed.**
