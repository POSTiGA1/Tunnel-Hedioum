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

Two rules the whole skill serves:
1. **Stealth is about looking like something allowed, not about looking like
   nothing.** Fully-random / fully-encrypted traffic is itself a signature.
2. **Every stealth change has a throughput/UX cost.** Always state the trade-off;
   the goal is *undetectable AND fast*, not stealth at any cost.

Read `references/threat-model.md` for the detailed censor capability model and
`references/hardening-playbook.md` for the prioritized fix roadmap. This file is
the operating method + the Hedioum-specific weakness map.

---

## 1. The censor's toolbox (what you are defending against)

- **Passive fingerprinting.** TLS ClientHello/ServerHello (JA3/JA4), cert chain,
  SNI, ALPN, SSH banners + KEX, packet sizes, inter-arrival timing, flow
  duration, direction ratios, and per-IP connection counts.
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
- **Why the banner helps:** printable-ASCII start ⇒ *exempted* by the
  fully-encrypted heuristic, and the later high-entropy payload matches real SSH.
- **The real weaknesses:**
  - **Nobody opens 10–20 parallel SSH sessions to one foreign IP.** The
    `internal/pool` warm-up (`min_connections`) is a glaring anomaly — real SSH is
    1 long session. This is the SSH mimic's biggest tell (traffic analysis, not DPI).
  - **Outbound :22 to a random foreign datacenter IP** is rare from Iran and is
    frequently throttled/blocked wholesale (see community issue "port 22 filtered").
  - The securestream handshake after the banner never completes a *real* SSH KEX —
    an active prober that speaks SSH-2.0 KEXINIT is routed to the **decoy sshd**
    (good), but the *timing/behaviour* of that redirect can differ from a native
    sshd. Verify the decoy is byte- and timing-faithful.

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
| 1 | **Self-signed cert on :443** (no real domain/SNI) — active/passive proxy tell | 🔴 High | REALITY (steal a real site's cert) OR real domain + Let's Encrypt |
| 2 | **Pool = N correlated long-lived flows to one IP** — traffic-analysis signature | 🔴 High | Fewer/longer conns, decorrelated establishment, or multiplex more per pipe |
| 3 | **SSH mimic on :22 to a foreign datacenter IP** — rare, throttled, anomalous pool | 🟠 Med | Prefer TLS:443; treat SSH as secondary; never the only/primary mimic |
| 4 | **Identical Apache decoy across installs** — fleet-wide signature | 🟠 Med | Per-install decoy diversity; optional :80→:443 redirect; real backend |
| 5 | **Datacenter exit IP reputation** — geo/AI blocks, easy ASN targeting | 🟠 Med | IP-reputation pre-check; residential/clean ranges; per-destination routing |
| 6 | **uTLS Chrome fingerprint drift** — stale JA3 becomes detectable | 🟡 Low | Track uTLS/Chrome versions; verify pinned `HelloChrome_Auto` |
| 7 | **Mail mimics reveal non-MTA behaviour under deep probe** | 🟡 Low | Only enable where mail is plausible; richer prologue; real mail decoy |

Re-run the audit and update this table; do not treat it as static.

---

## 5. Hardening priorities (see `references/hardening-playbook.md`)

In rough order of impact-per-effort for THIS project:
1. **Kill the self-signed-cert tell:** add a REALITY-style handshake (proxy
   unknown-SNI probes to a real target, borrow its cert) or real-domain + ACME.
   This is the highest-leverage change.
2. **CDN / WebSocket fronting** (VLESS+WS+TLS behind Cloudflare/ArvanCloud style):
   hides the real foreign IP, uses the CDN's SNI, and sidesteps IP-reputation and
   ASN targeting.
3. **Shrink the pool signature:** decorrelate connection establishment, allow
   fewer but longer-lived pipes, and prefer heavier per-pipe multiplexing over
   many parallel TCP flows.
4. **First-byte / entropy safety:** keep every mimic's first bytes protocol-shaped
   (TLS record or ASCII banner). Never expose a raw high-entropy prefix.
5. **Decoy diversity + realism:** rotate decoy templates per install; make the web
   decoy respond to more paths/verbs like a real server.
6. **Exit-IP hygiene:** a built-in `check-ip` (Gemini/OpenAI/ASN) before deploy;
   domain/GeoIP split-routing so sensitive services use a clean exit.
7. **Timing/padding shaping** *only if measured to help* — it costs latency and
   throughput, so validate against a real trace before shipping.

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
