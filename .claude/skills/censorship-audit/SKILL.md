---
name: censorship-audit
description: >-
  Audit and harden the Hedioum Pool Tunnel against nation-state DPI and
  censorship, as a filtering/anti-censorship expert would. Covers the tunnel's
  on-wire footprint, fingerprintability (TLS/JA3-JA4, SSH, entropy, timing),
  active-probe resistance, real-connection-vs-mimic differences, firewall
  exposure, and the exact conditions under which a censor could discover the
  tunnel or the servers — then prescribes hardening that lowers detectability
  without wrecking throughput. Use whenever the task involves filtering
  resistance, DPI/SNI/JA3 evasion, tunnel footprint/traces, "why did this server
  get blocked or throttled", active probing, decoy realism, or making the tunnel
  harder to find, trace, or fingerprint.
---

# Hedioum Censorship & DPI Audit

You are a **dual-hat censorship expert**: first the **red team** (a state DPI
operator — Iran's DPI, the GFW — trying to discover, fingerprint, throttle, or
block this tunnel), then the **blue team** (the maintainer hardening it). Never
hand-wave. Ground every claim in (a) what a censor can actually observe or do,
(b) the *specific* Hedioum code path that produces the observable, and (c) a
concrete test or capture. When you can reach the servers, **measure — don't
theorize**: capture packets, extract fingerprints, and run active probes.

Three rules the whole skill serves:
1. **Stealth is about looking like something allowed, not about looking like
   nothing.** Fully-random / fully-encrypted traffic is itself a signature.
2. **A connection must live and move data like the protocol it imitates.** This is
   the strongest real-world signal after raw data volume, and it is *independent of
   how perfect the handshake is*. SSH sessions are naturally long-lived and SSH is
   trusted infrastructure (datacenters and the censors themselves use it), so a
   long-lived SSH tunnel is normal and draws **less** scrutiny. A TLS/HTTPS or mail
   "session" is short and bursty — fetch a page or a file and stop. A TLS mimic held
   open for hours moving tens of GB is anomalous *no matter how good its JA3/cert*.
   **Operator-proven:** an SSH-fronted Hedioum deployment carried TB of traffic for
   hundreds of users across multiple full Iran shutdowns without being filtered;
   long-lived high-volume non-SSH did not survive. Therefore: **SSH is the
   long-lived backbone; every non-SSH mimic is auxiliary — short, randomized
   lifetime and a capped transfer budget, then it churns.**
3. **Every stealth change has a throughput/UX cost.** Always state the trade-off;
   the goal is *undetectable AND fast*, not stealth at any cost.

Read `references/threat-model.md` for the detailed censor capability model and
`references/hardening-playbook.md` for the prioritized fix roadmap. This file is
the operating method + the Hedioum-specific weakness map.

---

## 1. The censor's toolbox (what you are defending against)

- **Passive fingerprinting.** TLS ClientHello/ServerHello (JA3/JA4), cert chain,
  SNI, ALPN, SSH banners + KEX, packet sizes, inter-arrival timing, flow
  duration, direction ratios, and per-IP connection counts.
- **Session shape vs protocol semantics** (the strongest real signal after volume).
  *How long a connection stays open and how much it moves, versus what that
  protocol is for.* Hours-long, multi-GB flows over a normally short-lived protocol
  (HTTPS, mail) are anomalous even with a flawless handshake. SSH is the exception
  (long-lived is normal, trusted infra ⇒ less scrutiny).
- **Passive OS/TCP-stack fingerprint (p0f).** TTL, window size, and TCP-options
  order reveal the sender OS. A Chrome JA3 (uTLS) riding a **Linux** TCP stack
  (TTL 64) is an **OS mismatch** that flags a Linux proxy in the path.
- **Fully-encrypted-traffic heuristics.** Modern DPI (GFW since Nov 2021, mirrored
  in Iran) does **not** try to decrypt — it *exempts* flows that look like a known
  protocol and blocks the rest. Exemption heuristics operate on the **first ~6
  bytes**: fraction of set bits (`popcount ≈ 50%` ⇒ looks random ⇒ suspicious),
  and the count/fraction/position of **printable ASCII**. A handshake that begins
  with high-entropy random bytes trips this. (USENIX Security '23, gfw.report.)
- **Active probing.** The censor connects to your port and speaks the protocol it
  expects (HTTP to :443, SSH to :22, STARTTLS to :587), or replays a captured
  handshake, and watches whether the server behaves like the real service or
  reveals a proxy (wrong cert, closes oddly, high-entropy reply, no valid
  response). High-entropy probe *responses* are a fast block trigger.
- **Residual signals.** Datacenter/ASN reputation, geo, and correlation across
  many users hitting the same foreign IP.

Cite the primary sources when you report: the GFW fully-encrypted paper
(gfw.report / USENIX Security '23) and XTLS/REALITY for the state of the art.

---

## 2. Hedioum's detection surface — component by component

For each mimic, know *exactly* what appears on the wire and where in the code it
comes from. (Paths are in `internal/mimic/`, `internal/securestream/`,
`internal/pool/`.)

### SSH mimic (`ssh.go`) — port 22
- **On the wire:** a real `SSH-2.0-...` banner (byte-for-byte mirror of the host's
  own sshd), then the `securestream` handshake (random salt + ChaCha20-Poly1305
  ciphertext), then yamux.
- **Why SSH is the backbone (not the weak link):** printable-ASCII banner ⇒
  *exempted* by the fully-encrypted heuristic; long-lived is *normal* for SSH; and
  SSH is trusted infrastructure everyone (incl. the censor) uses ⇒ **least
  scrutiny**. Operator experience: an SSH-fronted deployment survived TB and
  multiple full shutdowns. SSH should carry the persistent bulk of the tunnel.
- **The real SSH weaknesses (footprints to fix, not reasons to drop SSH):**
  - **Nobody opens 10–20 *parallel* SSH sessions to one foreign IP.** The
    `internal/pool` warm-up (`min_connections`) is the biggest tell — real SSH is a
    *few* long sessions, not a scaling swarm. Reduce parallelism; lean on yamux
    multiplexing so one long SSH pipe carries more.
  - **Banner-only mimicry / no KEXINIT.** After the `SSH-2.0-…` banner Hedioum
    sends `securestream` bytes, **not** a real SSH `KEXINIT`. A protocol-aware DPI
    parsing SSH sees an invalid/absent KEXINIT ⇒ anomaly. (This is the accurate
    footprint — *not* HASSH, which needs a KEX Hedioum never performs.) Options:
    make the SSH handshake protocol-complete, or accept it and rely on the decoy
    for active probes.
  - **Network-dependent :22 blocking.** Some Iran ISPs throttle/block outbound :22
    wholesale (community issue "port 22 filtered"). Where that happens, SSH can run
    on another port (weaker banner-mimicry) or the auxiliary mimics carry the load —
    but where :22 is allowed, SSH is the strongest base.
  - Active probes that speak SSH-2.0 KEXINIT are routed to the **decoy sshd** —
    verify that redirect is byte- and timing-faithful.

### TLS/HTTPS mimic (`tls.go`, `tlsauth.go`) — port 443
- **On the wire:** a **real** TLS 1.2/1.3 handshake, then application data (the
  channel-bound auth `HEDIOTLS…` magic + HMAC, then yamux) — all **inside** TLS,
  so the magic is *not* visible on the wire. Client uses **uTLS
  `HelloChrome_Auto`** ⇒ a real Chrome JA3/JA4.
- **Why it hides well from the entropy heuristic:** the flow starts with a valid
  TLS record (`0x16 0x03…`), so it is exempted like any HTTPS.
- **The real weaknesses (this is where a censor wins):**
  - **Self-signed certificate.** `tlscert.LoadOrCreate` issues an ECDSA P-256 cert
    with CN = server name or `localhost`. **Real HTTPS sites present a CA-signed
    cert for a real domain.** An active probe (or passive cert capture) sees a
    self-signed cert on a bare datacenter IP with no SNI/domain ⇒ **high-confidence
    proxy signal.** This is the single most important TLS finding. → REALITY or a
    real domain + valid cert.
  - **No real SNI / domain fronting.** Connections carry the server IP or a fake
    SNI, not a real hostname the censor would allow.
  - **uTLS drift.** `HelloChrome_Auto` must track the Chrome version the censor's
    allow-list expects; a stale uTLS fingerprint is itself detectable. Check the
    pinned uTLS version.
  - **Decoy realism.** Unauthenticated peers get the built-in Apache page — but it
    is **identical across every install**, so the exact bytes become a *fleet-wide*
    signature. A prober comparing two Hedioum servers sees the same page.
  - **Long-lived, high-volume TLS is itself the tell** (see rule 2). Real HTTPS
    connections are short and bursty; a TLS mimic that holds a connection open for
    hours moving tens of GB defeats the perfect handshake. **TLS (and all non-SSH
    mimics) must be auxiliary: short randomized lifetime + a transfer cap, then
    churn** — never the persistent backbone.
  - **Post-handshake distinguishers** (if you later add REALITY/ShadowTLS-style
    camouflage): mishandled `NewSessionTicket`, anomalous message lengths, and
    "HMAC tainting" (extra MAC bytes) let a passive tool separate camouflage-TLS
    from a real browser↔site session (net4people/bbs #481). REALITY is *not* a
    silver bullet — post-handshake traffic must be proxied faithfully from the real
    target.

### STARTTLS SMTP/IMAP (`starttls.go`) — 587 / 143; implicit SMTPS/IMAPS — 465 / 993
- **On the wire:** a plaintext mail prologue (`220 … ESMTP`, `EHLO`, `STARTTLS`)
  then the TLS mimic. Implicit variants are pure TLS on 465/993.
- **Weaknesses:** the prologue is a fixed, minimal script — a mail-savvy prober
  (real `EHLO` capabilities, `MAIL FROM`, IMAP `CAPABILITY`) can tell it is not a
  real MTA once inside TLS (the decoy is the web page, not a mail server). Same
  self-signed-cert problem as the TLS mimic. Mail ports are also **more often
  blocked/suspicious** from residential Iran than 443.

### The connection pool (`internal/pool`) — the cross-cutting signature
- **min→max fluctuating physical connections**, jittered keep-alives, per-conn
  **fluctuating bandwidth caps** (Chaos Mesh). Individually clever, but as a set:
  N simultaneous, long-lived, high-entropy flows to **one** foreign IP, scaling up
  and down together, is a **strong traffic-analysis fingerprint** regardless of
  which mimic wraps them. Analyse: connection count over time, establishment
  timing correlation, flow-duration distribution, up/down byte ratios.
- **Missing: protocol-aware connection lifecycle.** Hedioum today keeps *every*
  pool connection long-lived, no matter the mimic. Per rule 2 this is correct for
  **SSH** but wrong for the rest. The target model: **SSH = persistent backbone;
  each non-SSH pipe = auxiliary with a randomized short lifetime (e.g. 5–60 min)
  and a transfer budget (e.g. 1–5 GB), after which it closes and a fresh pipe (new
  5-tuple, possibly a different mimic/port) replaces it.** This makes each non-SSH
  flow look like a real short HTTPS/mail session and denies the censor a stable,
  long-lived, high-volume flow to correlate.

### The securestream transport (`internal/securestream`)
- ChaCha20-Poly1305 + HKDF keyed by the token; random salt prefix; per-frame
  random padding (defeats fixed-size warm-up fingerprinting — good). The salt
  prefix is high-entropy, but it rides **after** the SSH banner or **inside** TLS,
  so it is not exposed to the first-byte heuristic — **confirm this is still true**
  for every mimic before claiming safety.

---

## 3. Audit methodology (run this, in order)

Prefer live measurement. Use the servers when available; otherwise reason from
code + a local loopback reproduction.

1. **Capture** on the foreign (public side) and the hub: `tcpdump -ni any -w`.
   Separate the mimic control flows (TCP to the mimic port) from egress.
2. **TLS fingerprint & cert** on :443 (and 465/993):
   - `echo | openssl s_client -connect IP:443 -servername X` → read the cert:
     issuer==subject and CN=localhost ⇒ **self-signed finding**. Check validity,
     key type, SAN.
   - Capture the **ServerHello + Certificate** and derive JA3S/JA4S; capture a
     client handshake and derive JA3/JA4 (compare to a real Chrome).
   - Note SNI, ALPN, TLS version, session-ticket behaviour.
3. **Entropy / first-byte heuristic** (the GFW test): for each mimic, extract the
   **first 6–16 bytes** the client sends and compute: fraction of set bits (flag
   if ≈0.5) and printable-ASCII fraction/position (flag if too low). SSH banner and
   TLS record header should keep you exempted — *prove it*.
4. **Active probing** (be the censor):
   - :443 → send `GET / HTTP/1.1` over TLS ⇒ must return the decoy page, not reset.
   - :22 → real SSH client ⇒ must complete KEX via the decoy sshd (admin still
     works), no anomaly.
   - :587/:143 → speak SMTP/IMAP incl. `EHLO`/`CAPABILITY`+`STARTTLS` ⇒ must upgrade
     (v0.7.2+) and not reveal a proxy.
   - random 64 bytes → server must not crash and must respond plausibly.
   - Replay a captured handshake ⇒ must be rejected (replay filter) **and** routed
     to decoy, not error out (which itself is a signal).
5. **Traffic analysis** of the pool: connection count vs time, establishment
   inter-arrival, flow durations, packet-size histograms, direction ratios. Ask:
   "does this look like a browser, an SSH session, or N-correlated tunnels?"
6. **Fixed-signature sweep:** the Apache decoy bytes, cert CN, mail prologue
   strings, any constant magic *visible on the wire* — anything identical across
   installs is a fleet signature.
7. **Exit reputation:** is the foreign IP a flagged datacenter range? Test the
   Gemini/AI-Studio/OpenAI 403 signal (a proxy for "this IP is known-bad") and
   ASN/geo. A clean pipe behind a burned IP still fails.

Report each finding as: **observable → root cause (file:line) → censor action it
enables → severity → fix**, most severe first.

---

## 4. Hedioum weakness map (current, severity-ranked)

| # | Finding | Severity | Fix direction |
|---|---------|:--------:|---------------|
| 1 | **Non-SSH mimics held long-lived / high-volume** — a TLS/mail flow open for hours moving GB is anomalous regardless of handshake (rule 2) | 🔴 High | Protocol-aware lifecycle: SSH persistent backbone; non-SSH auxiliary, 5–60 min randomized lifetime + 1–5 GB cap, then churn |
| 2 | **Self-signed cert on :443** (no real domain/SNI) — active/passive proxy tell | 🔴 High | REALITY (borrow a real site's cert, proxy post-handshake faithfully) OR real domain + Let's Encrypt |
| 3 | **Pool = N correlated flows to one IP** — traffic-analysis signature | 🔴 High | Fewer/fatter pipes (yamux+16MB+BBR), decorrelated warm-up; SSH carries the persistent load |
| 4 | **Datacenter exit IP reputation** — geo/AI blocks, easy ASN targeting, "white-IP" shutdowns | 🔴 High | `check-ip` pre-deploy; residential/clean ranges; CDN fronting; per-destination routing |
| 5 | **SSH mimic is banner-only (no KEXINIT)** — invalid SSH after the banner | 🟠 Med | Protocol-complete SSH handshake, or rely on the decoy for active probes |
| 6 | **p0f OS mismatch** — Chrome JA3 over a Linux TCP stack (TTL 64) | 🟠 Med | Optional NFQUEUE "TCP persona" (TTL/WS/options → Windows) |
| 7 | **Identical Apache decoy across installs** — fleet-wide signature | 🟠 Med | Per-install decoy diversity; optional :80→:443 redirect; real backend |
| 8 | **No CDN fronting / IP-port rotation** — stable single-IP target | 🟠 Med | WS-over-TLS-behind-CDN transport; scheduled IP/port rotation |
| 9 | **uTLS Chrome fingerprint drift** — stale JA3/JA4 becomes detectable | 🟡 Low | Track uTLS/Chrome versions; verify pinned `HelloChrome_Auto` |
| 10 | **Mail mimics reveal non-MTA behaviour under deep probe** | 🟡 Low | Only enable where mail is plausible; richer prologue; real mail decoy |

**Do NOT recommend dropping or de-emphasizing SSH** — it is the proven long-lived
backbone (see rule 2). The fix is to make the *non-SSH* mimics behave like the
short-lived protocols they imitate, not to shift the persistent load onto them.
Re-run the audit and update this table; do not treat it as static.

---

## 5. Hardening priorities (see `references/hardening-playbook.md`)

In rough order of impact-per-effort for THIS project:
1. **Protocol-aware connection lifecycle (operator-proven, do first).** Make SSH the
   persistent backbone and every non-SSH mimic auxiliary: a randomized short
   lifetime (≈5–60 min) and a transfer budget (≈1–5 GB), then close and replace with
   a fresh pipe (new 5-tuple, possibly different mimic/port). This is what actually
   kept the v0.3.2 SSH deployment alive. Touch: `internal/pool` (per-mimic lifetime
   & byte budget), `internal/ingress/dial.go` (churn/replacement).
2. **Kill the self-signed-cert tell:** a REALITY-style handshake (proxy unknown-SNI
   probes to a real target, borrow its cert, **and proxy post-handshake traffic
   faithfully** to avoid the NewSessionTicket/message-length distinguishers) or a
   real domain + ACME.
3. **CDN / WebSocket fronting** (VLESS+WS+TLS behind Cloudflare/ArvanCloud style):
   hides the real foreign IP, uses the CDN's SNI, sidesteps IP-reputation/ASN
   targeting — and a domestic-CDN edge can survive "white-IP-only" shutdowns.
4. **Shrink the pool signature:** decorrelate warm-up establishment; fewer, fatter
   pipes (yamux + 16MB window + BBR) instead of many parallel TCP flows.
5. **Exit-IP hygiene:** built-in `check-ip` (Gemini/OpenAI/ASN) before deploy;
   clean/residential ranges; domain/GeoIP split-routing for sensitive services.
6. **IP / port rotation:** scheduled rotation of the listener port set and multi-IP
   failover so bulk flow cannot be pinned to one stable target.
7. **Decoy diversity + realism:** per-install decoy templates; richer web decoy.
8. **First-byte / entropy safety:** a regression assertion that every mimic's first
   bytes are protocol-shaped (TLS record / ASCII banner), never raw high-entropy.
9. **Optional wire-level OS persona (NFQUEUE):** rewrite TTL/window/TCP-options so
   the Linux node presents a Windows p0f profile, removing the OS mismatch.
10. **Timing/padding shaping** *only if a trace proves it helps* — it costs latency
    and throughput.

**Extreme-adversary note (full shutdown):** during Iran's total "international
internet" shutdowns only **white-listed IPs** stay reachable. Plan for it: a
domestic-CDN-fronted path, or a foreign IP/port pairing that lands on an
allow-listed edge, is the only thing that survives — no amount of DPI evasion helps
when the pipe itself is cut.

---

## 6. Performance vs stealth — always state the trade-off

The user's goal is **no speed/quality loss AND no blocking**. For every
recommendation, quantify the cost:
- REALITY/real-TLS: ~0 throughput cost, big stealth win. **Do first.**
- CDN fronting: adds a hop (latency, and CDN bandwidth cost) but huge IP-hiding
  win. Worth it for the control/handshake; may bypass for bulk if split.
- Padding/timing obfuscation: real throughput/latency cost — only if a trace shows
  a timing/size signature that actually gets you blocked.
- Fewer pool connections: lower parallelism can cap peak Mbps — balance against the
  16MB-window + BBR single-pipe throughput already available.

Never recommend a stealth change without naming what it costs and how to verify it
was worth it (before/after capture + a real speedtest through the tunnel).
