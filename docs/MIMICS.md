# Protocol Mimicry (Arsenal & Camouflage)

Hedioum disguises each physical tunnel connection as an ordinary internet
service. A foreign (egress) node can run **several mimics at once**, each on its
own port, and the Iran (hub) node spreads its pool of physical pipes across them
with a **fluctuating, per-install distribution** — so no two deployments show the
same on-wire signature, and the mix drifts over time as the pool scales.

All mimics share one design:

```
[ camouflage handshake ] → [ single-layer auth + crypto ] → [ yamux multiplexing ]
```

Unauthenticated peers (active probes, scanners, a curious browser) never see an
error or a dropped connection — they are transparently handed to a
**protocol-appropriate decoy**, so the port is indistinguishable from a genuine
service.

---

## The mimics

| Mimic  | Default port | On the wire looks like        | Decoy for unauthorized peers        |
|--------|--------------|-------------------------------|-------------------------------------|
| `ssh`  | 22           | An OpenSSH server             | The host's **real `sshd`** (banner mirrored byte-for-byte) |
| `tls`  | 443          | An HTTPS web server           | A built-in `nginx`-style **web page** (or a real backend)  |
| `smtp` | 587          | A mail server doing STARTTLS  | Same web/TLS decoy after the STARTTLS upgrade              |
| `imap` | 143          | A mail server doing STARTTLS  | Same web/TLS decoy after the STARTTLS upgrade              |

### SSH mimic
Presents a real `SSH-2.0-...` banner mirrored **byte-for-byte** from the host's
own `sshd` (SSH binds the banner into its key-exchange hash, so this must match
exactly). Relocate the real `sshd` to a decoy port first (`--move-ssh` moves it
to `--decoy-port`, default 2022); unauthorized probes are proxied there and
complete a genuine SSH handshake.

### TLS mimic
Performs a **real TLS handshake** using a self-signed ECDSA P-256 certificate
that the node generates on first start (no domain, no CA, no user involvement).
That TLS session is the *single* crypto layer — there is **no second encryption
inside it**, so throughput stays uniform.

The peer is then authenticated with a **channel-bound token**: an
`HMAC-SHA256(token, serverCertFP ‖ nonce ‖ direction)`. This proves both sides
know the shared token **without ever putting it on the wire**, and because the
HMAC is bound to the live certificate fingerprint, a man-in-the-middle that
terminates TLS with its own cert cannot pass. A wrong token — or a real
browser — fails this check and is served the built-in web page instead.

The **client** side uses [uTLS](https://github.com/refraction-networking/utls)
with a real Chrome `ClientHello`, so the JA3 fingerprint matches a mainstream
browser rather than a Go program. The server certificate is pinned **TOFU**
(trust-on-first-use, warn-only — the token auth is the real gate).

### SMTP / IMAP mimics (STARTTLS)
These emit a short plaintext mail-protocol prologue (`220 ... ESMTP` / `EHLO` /
`STARTTLS`, or `* OK ... STARTTLS`) and then upgrade into the **exact same TLS
mimic** described above. On the wire the connection looks like a mail client
negotiating STARTTLS with a mail server — no domain required.

> Note: the post-STARTTLS decoy is the TLS/web decoy. A determined prober that
> completes STARTTLS and then speaks SMTP/IMAP inside TLS will get an HTTP
> response rather than a mail dialog. For the strongest mail decoy, point the
> listener's `decoy` at a real mail server. The plaintext prologue is the primary
> camouflage.

---

## Configuring mimics

### Foreign (egress) — declare which camouflages to listen behind

```bash
# SSH + TLS, relocating the real sshd to the decoy port
hedioum-tunnel setup-foreign --mimics ssh,tls --move-ssh

# Everything, custom ports
hedioum-tunnel setup-foreign \
  --mimics ssh,tls,smtp,imap \
  --listen-port 22 --tls-port 443 --smtp-port 587 --imap-port 143 \
  --move-ssh
```

`--mimics all` is shorthand for `ssh,tls`. The command prints the **auth token**
— you need it on the Iran side.

### Iran (hub) — reach a node over one or more mimics

A node has **one local SOCKS port** but may be reached over several endpoints;
the dialer picks among them per new pipe with a fluctuating weighting.

```bash
# One node reachable over ssh + tls + smtp, all at the same foreign IP
hedioum-tunnel setup-iran \
  --alias de1 --target-ip 203.0.113.9 \
  --mimics ssh,tls,smtp \
  --ssh-port 22 --tls-port 443 --smtp-port 587 \
  --socks-port 40001 --token <TOKEN> \
  --profile high-speed
```

The legacy single-endpoint form still works:
`--target 203.0.113.9:22` (implies the `ssh` mimic).

---

## Measuring throughput

`speedtest` opens an **un-shaped** stream to a node's egress and measures real
end-to-end Mbps (bypasses the per-connection bandwidth shaper), so you can verify
a link independent of the pool's DPI-evasion caps:

```bash
hedioum-tunnel speedtest --node de1                    # down + up, 10s each
hedioum-tunnel speedtest --node de1 --dir down          # download only
hedioum-tunnel speedtest --node de1 --mimic tls --seconds 20  # test a specific endpoint
```

BBR + `fq` and 16 MB Yamux windows (both enabled automatically) are what let a
single pipe approach line rate on high-latency links.
