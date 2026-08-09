# Threat model — what a nation-state censor can actually do

A reference for the `censorship-audit` skill. Assume a censor with the
capabilities of Iran's DPI or the GFW: line-rate passive inspection on backbone
links, a fleet of active probers, per-IP/ASN state, and the willingness to block
by IP, SNI, port, or protocol-heuristic. Everything below is a *capability the
adversary has*, mapped to how it bites Hedioum.

## A. Passive fingerprinting (no interaction)

### A0. Session shape vs protocol semantics — the dominant real-world signal
After raw data volume, the strongest discriminator is **how a connection lives**
versus **what the protocol it claims to be is used for**. A censor does not need to
break the crypto; it watches duration + volume + directionality per protocol:
- **HTTPS / mail** connections are short and bursty in the wild — fetch a page, an
  update, a file; a handful of seconds to minutes; then idle/close. A "TLS" or
  "SMTP" flow that stays open for **hours** and moves **tens of GB** is anomalous
  **no matter how perfect its JA3, cert, or SNI**. Perfect handshakes do not save a
  wrong session shape.
- **SSH** is the exception: interactive/forwarding SSH is naturally long-lived,
  SSH is trusted infrastructure that datacenters and the censors themselves run, and
  it is *known* to be awkward for tunneling — so it draws **less** scrutiny.
  **Operator-proven:** an SSH-fronted Hedioum deployment carried TB for hundreds of
  users across multiple full Iran shutdowns without being filtered; equivalent
  long-lived high-volume non-SSH did not survive.
- **Design consequence:** SSH = persistent backbone; every **non-SSH** mimic must be
  **auxiliary** — randomized short lifetime (≈5–60 min) + a transfer budget
  (≈1–5 GB), then churn to a fresh pipe. This is the single most important stealth
  property for Hedioum and it is *independent of* handshake perfection.

### A1. TLS fingerprinting (JA3 / JA4 / JA3S / JA4S)
- The ClientHello (cipher suites, extensions, curves, ALPN, order) hashes to a
  **JA3/JA4** fingerprint. A Go-crypto/tls ClientHello is instantly non-browser.
  Hedioum uses **uTLS `HelloChrome_Auto`** ⇒ matches Chrome — good, *if current*.
- The **ServerHello + Certificate** hash to **JA3S/JA4S** and expose the cert.
  This is where Hedioum leaks: a **self-signed cert** (issuer==subject, CN often
  `localhost`) on a bare IP with **no real SNI** is not what any real HTTPS site
  presents. Passive cert capture alone is enough to flag it.
- **Mitigation reality:** only REALITY (borrow a real site's live cert) or a real
  domain + CA cert removes this. uTLS fixes the *client* half; it does nothing for
  the *server* cert.

### A2. The fully-encrypted-traffic heuristic (GFW, Nov 2021 →; mirrored in Iran)
- The censor does **not** decrypt. It **exempts** flows that look like a known
  protocol and blocks the rest. Exemption tests run on the **first ~6 bytes**:
  - **Ex1 — popcount:** if the fraction of set bits is ≈ 0.5 (looks random),
    *not* exempted.
  - **Ex2–Ex5 — printable ASCII:** enough printable-ASCII bytes, or a known
    protocol prefix (HTTP verbs, TLS record, SSH banner), ⇒ exempted.
- **Implication for Hedioum:** a raw high-entropy prefix would get blocked. Hedioum
  is currently safe because every mimic's first bytes are protocol-shaped: SSH
  sends `SSH-2.0-…` (ASCII), TLS sends a `0x16 0x03` record. **The audit must
  re-verify this for every code path** — any mimic that ever emits a random prefix
  before an ASCII/TLS envelope is immediately blockable.
- Source: "How the GFW Detects and Blocks Fully Encrypted Traffic", USENIX
  Security 2023 (gfw.report/publications/usenixsecurity23).

### A3. SSH fingerprinting (and why HASSH does *not* apply to Hedioum)
- **HASSH** fingerprints the SSH `KEXINIT` (KEX/cipher/MAC/compression list+order).
  A Go-vs-OpenSSH KEXINIT hashes differently. **But Hedioum's SSH mimic never sends
  a KEXINIT** — it emits the real `SSH-2.0-…` banner (mirrored, good) then
  `securestream` bytes. So HASSH has nothing to fingerprint. The **real** footprint
  is the *inverse*: a protocol-aware DPI parsing SSH expects a KEXINIT after the
  banner and finds non-SSH bytes ⇒ anomaly. Fix by making the handshake
  protocol-complete or leaning on the decoy for active probes.
- The pool's *parallelism* (A4) is the other SSH tell — real SSH is a few long
  sessions, not a scaling swarm.

### A6. Passive OS / TCP-stack fingerprint (p0f)
- **TTL, window size, and TCP-options order** (MSS/SACK/Timestamps/WS) reveal the
  sender OS. Windows TTL≈128 + option order `MSS,NOP,WS,NOP,NOP,SACK`; Linux TTL≈64
  + `MSS,SACK,TS,NOP,WS`. A **Chrome JA3 (uTLS) over a Linux TCP stack (TTL 64)** is
  an **OS mismatch** — a strong "Linux proxy in the path" signal.
- **Note (correction to a common claim):** MSS/MTU *reduction* (e.g. 1380 vs 1460)
  is an **L3-tunnel** (WireGuard/IPsec) signature. Hedioum is an **app-layer TCP**
  tunnel, so the outer MSS stays normal — this signal does **not** apply. Do not
  "fix" it by adding an L3 layer.
- Mitigation: optional NFQUEUE "TCP persona" rewriting TTL/WS/options to a Windows
  profile — removes the mismatch without changing the base OS config.

### A4. Traffic analysis (the hardest to hide, mimic-independent)
- **Connection count to one IP over time.** A browser opens a handful of short
  flows to *many* IPs. Hedioum opens **N long-lived flows to one** foreign IP and
  scales N up/down. That shape is unusual for any single legitimate app.
- **Establishment timing correlation:** pool warm-up dials several pipes within
  seconds — correlated SYN bursts to one IP.
- **Flow duration & byte-ratio:** tunnels are long-lived and roughly symmetric or
  download-heavy in a steady way; the fluctuating-cap jitter helps but the
  *aggregate* is still analysable.
- **Packet-size distribution:** securestream per-frame padding helps; yamux
  framing + 16MB windows produce their own distribution — compare to real HTTPS.

### A5. QUIC / UDP
- Since Apr 2024 the GFW decrypts QUIC Initial packets for SNI-based blocking.
  Hedioum tunnels UDP over TCP (no UDP on the inter-node wire), so this does not
  hit the tunnel directly — but a client that leaks QUIC *outside* the tunnel is
  exposed.

## B. Active probing (the censor connects to you)

- **Protocol-speak probes:** HTTP to :443, SSH to :22, EHLO/STARTTLS to :587. The
  server must respond exactly like the real service. Hedioum routes these to
  decoys — verify byte- and timing-fidelity, and that a wrong-token/replayed
  handshake is **routed to decoy, not errored** (an error or reset is itself a
  signal).
- **SNI-mismatch probe (the REALITY test):** connect with an unexpected SNI. A real
  fronting setup forwards to the real site and returns its cert. Hedioum's TLS
  mimic returns *its own* self-signed cert regardless of SNI ⇒ detectable.
- **Entropy probe:** send `/dev/urandom`; a high-entropy *reply* is a fast block
  trigger. Hedioum's decoys reply with ASCII (HTTP/SSH) — good — but confirm no
  path replies with raw ciphertext to an unauthenticated peer.
- **Replay probe:** capture a real client handshake and replay it. Must be dropped
  by the replay filter *and* handled like a decoy.

## C. Residual / out-of-band signals

- **IP/ASN reputation & geo:** datacenter ranges (OVH, cheap "unlimited" hosts) are
  pre-flagged; Google AI / OpenAI 403s are a cheap proxy for "known-bad exit".
- **Cross-user correlation:** many Iranian users hitting one foreign IP, or one
  `danyarai.ir`-style CDN edge, concentrates signal.
- **Certificate transparency / scanning:** a real domain with a valid cert is
  logged in CT; a bare IP with a self-signed cert stands out to internet-wide
  scanners (Censys/Shodan-style) that censors also run.

## C2. Extreme mode — total shutdown / white-IP only
Iran has hit the **"international internet" kill switch** repeatedly (e.g. Jun 2025,
Jan 2026, and a ~3-month cut from late Feb 2027, war-driven). In that mode almost
all foreign connectivity is severed and **only white-listed IPs remain reachable**.
- No DPI-evasion technique helps here — the pipe itself is cut. Survival depends on
  the *reachability* of the path, not its stealth: a **domestic-CDN-fronted** route
  (e.g. an Iran-hosted CDN edge that stays whitelisted) or a foreign endpoint that
  lands on an allow-listed address/edge.
- Treat this as a distinct requirement: "stay *reachable* during a shutdown" is a
  different goal from "stay *undetected* during normal filtering", and needs a
  fronting/whitelist strategy, not obfuscation.

## D. What Hedioum already does right (don't "fix" these)

- Real TLS handshake + uTLS Chrome ClientHello (A1 client half, A2 exemption).
- ASCII SSH banner before ciphertext (A2 exemption for the SSH path).
- Per-frame random padding (A4 size fingerprinting).
- Decoy routing for unauthorized/replayed probes (B), incl. mirrored sshd banner.
- Channel-bound auth *inside* TLS — the `HEDIOTLS` magic is never on the wire.
- AEAD (ChaCha20-Poly1305 + HKDF), token never transmitted.

The audit's job is to attack everything else — chiefly the **self-signed cert**
(A1/B), the **pool traffic shape** (A4), and **exit-IP reputation** (C).
