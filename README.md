# Hedioum Dynamic Pool Tunnel (Chaos Mesh)

🌐 **[🇮🇷 راهنمای فارسی — Persian README](README.fa.md)** · **[Latest release](https://github.com/hedioum/Hedioum-Pool-Tunnel/releases/latest)**

Hedioum Pool Tunnel is a high-performance, enterprise-grade connection multiplexer designed to bypass strict Deep Packet Inspection (DPI) and thwart TCP Meltdown under heavy load. It operates as a Custom SDN Overlay, wrapping encrypted VLESS/Trojan traffic into highly obfuscated, dynamically scaling connection pools that wear interchangeable disguises — **SSH, TLS/HTTPS, and SMTP/IMAP** — over a modern authenticated-encryption transport.

## 🌟 Key Features

- **Chaos Mesh Dynamic Balancing:** Replaces traditional Round-Robin with a smart "Least Loaded" distribution algorithm. The system actively monitors real-time bandwidth (Mbps) and scales physical connections up or down based on actual traffic volume, not just stream count.
- **DPI Evasion (Fluctuating Caps):** Implements dynamic bandwidth jitter. Each physical connection operates under a randomized, fluctuating bandwidth limit (e.g., 8 Mbps ± 2 Mbps) to break static patterns, making the tunnel indistinguishable from organic, noisy internet traffic.
- **Zero-Downtime Connection Draining:** During scale-down events, idle physical connections are placed in a `Draining` state. They wait for active logical streams (like open socket connections) to finish naturally before closing, ensuring zero lag or disconnections for end-users.
- **Enterprise Lifecycle Management:** Features an interactive TUI dashboard equipped with a Blue-Green Self-Updater (with automatic rollback on failure) and a Clean Uninstaller that purges all traces without leaving orphaned files.
- **Authenticated Encryption (ChaCha20-Poly1305):** The transport is wrapped in a modern AEAD stream keyed via HKDF from your token. ChaCha20 needs no AES-NI, so overhead stays minimal on cheap/ARM VPS. The token is never sent on the wire and also keys the cipher, so a passive observer sees only random salts followed by ciphertext.
- **Full TCP + UDP:** A single local SOCKS5 port serves both TCP (CONNECT) and UDP (ASSOCIATE), so QUIC/HTTP3, DNS, and voice/video calls work. UDP rides its own dedicated (still SSH-masked) connection sub-pool, isolated from TCP so a bulk download can't stall a call, with drop-on-congestion so a slow link never buffers unboundedly. UDP never touches the wire between the nodes — it is tunnelled over the masked TCP link.
- **Optional IPv6:** The inter-node link and egress-to-internet can use IPv6 (`egress_ip_mode`: `ipv4` default / `ipv6` / `dual`), opt-in so the server's IPv6 identity is never leaked by default.
- **Pluggable Protocol Mimicry (Arsenal):** A single node can listen behind several camouflages at once — **SSH** (real SSH-2.0 banner mirrored byte-for-byte from the host's own `sshd`), **TLS/HTTPS** (a real TLS handshake with a self-signed cert; unauthenticated probes and browsers get a plausible `nginx` web page), **STARTTLS SMTP/IMAP** (587/143), and **implicit-TLS SMTPS/IMAPS** (465/993). The hub spreads its physical pipes across these with a **per-install fluctuating distribution** (Chaos Mesh v2), so two servers running the same build present different, shifting on-wire signatures. Every mimic routes unauthorized probes to a protocol-appropriate decoy, so each port looks like an ordinary service. See **[docs/MIMICS.md](docs/MIMICS.md)**.
- **No DNS Leak by Design:** the SOCKS5 ingress forwards the destination **domain** (never a locally-resolved IP) through the tunnel, so DNS is resolved on the **foreign** node — configure your client for remote DNS (`domainStrategy: AsIs`) and the Iran box emits no DNS query for tunneled destinations. See the verification steps in [docs/MIMICS.md](docs/MIMICS.md).
- **Channel-Bound Token Auth (no double crypto):** The TLS mimic authenticates the peer with an HMAC bound to the server's live certificate fingerprint — proving knowledge of the token *without ever sending it* and defeating MITM — while keeping a **single** crypto layer (the TLS session itself) for uniform, high-throughput performance. The client uses **uTLS** (a real Chrome ClientHello) to defeat JA3 fingerprinting.
- **Throughput Engineering:** BBR congestion control + `fq` qdisc are enabled on install, Yamux stream windows are widened to 16 MB for high bandwidth-delay-product links, and a built-in `speedtest` command measures real end-to-end egress throughput (un-shaped) per endpoint.

## 🏗 Architecture Topology

1. **X-UI (Iran):** Authenticates the user, splits domestic traffic, and forwards international traffic to the local SOCKS5 Bridge.
2. **Hedioum Hub (Iran):** Receives the SOCKS5 payload, evaluates pool health, and multiplexes the stream (via HashiCorp Yamux) over an authenticated, encrypted physical connection pool using the Chaos Mesh algorithm.
3. **Hedioum Egress (Foreign):** Authenticates the Hub via the channel-bound handshake (unauthenticated probes are diverted to a protocol-appropriate decoy — the real `sshd`, or a plausible web page for the TLS/mail mimics), enforces SSRF protections (resolve-once, blocks private/link-local/CGNAT), resolves the destination DNS here (no leak in Iran), and dials the open internet over IPv4/IPv6/dual per `egress_ip_mode`.

## 🚀 Installation & Updates

The binary is self-sufficient: it installs its own systemd service, opens the
firewall, and manages its config — so `install.sh` is just a thin bootstrap
(detect architecture → download → run the binary). See **[docs/DEPLOY.md](docs/DEPLOY.md)** for the full guide, including non-interactive (Ansible-friendly) setup and the GitHub-blocked path.

**Installation Order:** install the **Foreign (egress)** node first to generate the auth token the Iran node needs.

### One-line bootstrap (GitHub reachable)
Run on each VPS as root (foreign first):

    bash <(curl -fsSL https://raw.githubusercontent.com/hedioum/Hedioum-Pool-Tunnel/main/install.sh)

### Manual (GitHub blocked)
Copy the matching binary (`hedioum-tunnel` / `hedioum-tunnel-arm64`) to the server, then:

    chmod +x hedioum-tunnel && ./hedioum-tunnel install && hedioum-tunnel

### Non-interactive (automation)

    hedioum-tunnel install
    hedioum-tunnel setup-foreign --mimics all --move-ssh                                              # foreign; prints the token
    hedioum-tunnel setup-iran   --alias DE-01 --target-ip <IP> --mimics all --socks-port 40001 --token <hex>
    systemctl start hedioum.service

`--mimics all` enables SSH + TLS; list them explicitly for the full arsenal:
`--mimics ssh,tls,smtp,imap,smtps,imaps`. Updates are rollback-safe
(`hedioum-tunnel update`, or `update --file <path>` when GitHub is blocked).

### Diagnostics (on the Iran hub)

    hedioum-tunnel probe     --node DE-01                 # per-mimic reachability of every endpoint
    hedioum-tunnel speedtest --node DE-01 --mimic tls     # real un-shaped egress throughput

## ⚙️ Management Dashboard

To manage servers, view live connection status, or perform lifecycle operations, run the interactive dashboard from your terminal at any time:

    hedioum-tunnel

**Dashboard Capabilities:**
- View active egress pools, per-node endpoints (mimic → target), and live DPI Evasion dynamics (Limits & Jitter).
- Monitor real-time daemon logs for Scale-Up/Down events and bandwidth usage.
- Add or Remove foreign egress nodes dynamically.
- Run a **Speedtest** or **Probe** endpoints (per-mimic reachability) straight from the menu.
- Perform a safe Self-Update (fetches latest GitHub release).
- Completely Uninstall and purge the daemon.

## 🛠 Building from Source

If you wish to compile the static binary manually:

    git clone https://github.com/hedioum/Hedioum-Pool-Tunnel.git
    cd Hedioum-Pool-Tunnel
    make build-linux

## ☕ Support & Donate

If you found this project helpful for maintaining a free and open internet, and you want to support further development, consider buying the team a coffee!

**USDT (Tether) Donation Addresses:**
- **TRC20 (Tron):** TRhwZFoHRZ9oux4emFXTj63aib9nuC2J2J
- **BEP20 (BSC):** 0x051e31cb70076854C0b62F816d5a89D3def4A22E
- **ERC20 (Ethereum):** 0x051e31cb70076854C0b62F816d5a89D3def4A22E
- **TON (The Open Network):** UQCqq0wYNDVhq9AXAZ5vOQ2ZgMmP6O0UTgvU1YhNeIpkUp1s

Thank you for your support!