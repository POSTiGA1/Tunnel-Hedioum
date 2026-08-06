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

| Mimic   | Default port | On the wire looks like             | Decoy for unauthorized peers        |
|---------|--------------|------------------------------------|-------------------------------------|
| `ssh`   | 22           | An OpenSSH server                  | The host's **real `sshd`** (banner mirrored byte-for-byte) |
| `tls`   | 443          | An HTTPS web server                | A built-in `nginx`-style **web page** (or a real backend)  |
| `smtp`  | 587          | A mail server doing **STARTTLS**   | Web/TLS decoy after the STARTTLS upgrade              |
| `imap`  | 143          | A mail server doing **STARTTLS**   | Web/TLS decoy after the STARTTLS upgrade              |
| `smtps` | 465          | **Implicit-TLS** SMTP submission   | Built-in web page (same as `tls`)   |
| `imaps` | 993          | **Implicit-TLS** IMAP (IMAPS)      | Built-in web page (same as `tls`)   |

**STARTTLS vs implicit TLS.** `smtp`/`imap` (587/143) start in *plaintext* and
upgrade to TLS via a `STARTTLS` command — a mail client negotiating STARTTLS is
what a probe sees. `smtps`/`imaps` (465/993) are **TLS from the first byte**
(no plaintext prologue) — identical to the `tls` mimic, just on the conventional
mail-over-TLS ports. Pick whichever your network treats as least suspicious.

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

---

## Observing which mimics are actually working

Because the pool spreads pipes across mimics with a fluctuating distribution, it
helps to see which are connecting and which are blocked on the current path.

**Probe every endpoint of a node** (dials + pings each mimic end-to-end, reports
latency or the failure):

```bash
hedioum-tunnel probe --node de1
# ✓ ssh    203.0.113.9:22    OK  (61 ms)
# ✓ tls    203.0.113.9:443   OK  (63 ms)
# ✗ smtp   203.0.113.9:587   FAIL: dial tcp ...:587: i/o timeout
```

**Watch live pipe establishment** — the hub logs each new pipe with its mimic:

```bash
journalctl -u hedioum.service -f | grep "pipe established"
# INFO pipe established node=de1 mimic=tls target=203.0.113.9:443
```

On the foreign side, `journalctl -u hedioum | grep "authentic hub connection"`
shows the mimic mix from the egress's point of view.

> Which mimics work is a property of the **network path**, not the tool: a port
> may be blocked upstream, or a DPI box may be hostile to a specific protocol.
> `tls`/`imaps` on 443/993 are usually the safest; `smtp`/`imap` on 587/143 are
> more often blocked or scrutinised. Use `probe` to find out for a given network.

---

## DNS: no leak by design

DNS resolution happens on the **foreign** node, never on the Iran box — provided
the client sends a **domain** (not a pre-resolved IP) into SOCKS:

1. The SOCKS5 ingress reads the destination and, for a domain target, forwards the
   **domain string unchanged** — it never calls a resolver locally.
2. The domain travels through the tunnel to the egress, which resolves it there
   (with the same SSRF vetting applied to the result).
3. UDP DNS queries (port 53) ride the UDP-associate path and are likewise
   forwarded to the resolver from the foreign side.

So configure Xray/X-UI for **remote DNS** (e.g. `"domainStrategy": "AsIs"` and a
DNS server reached *through* the proxy). If Xray resolves names locally before
handing an IP to SOCKS, that lookup leaves the Iran box directly — that is a
client-side leak the tunnel cannot prevent.

**Verify no leak** from the hub:

```bash
# remote DNS (domain sent to us) — no local lookup:
curl --socks5-hostname 127.0.0.1:40001 https://example.com
# contrast with local resolution (leaks the query on the hub):
curl --socks5           127.0.0.1:40001 https://example.com
# and confirm nothing queries 53 outside the tunnel:
sudo tcpdump -ni any port 53
```
