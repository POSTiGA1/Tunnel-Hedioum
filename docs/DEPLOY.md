# Deploying Hedioum

The binary is fully self-sufficient: it installs its own systemd service, opens
the firewall, and writes its config. Installing via `install.sh` and running the
binary by hand are equivalent — the script only detects the architecture,
downloads the matching release, and runs the binary.

## Install order

> Install the **Foreign (egress)** node first — it prints the auth token the Iran
> hub needs.

## Option A — one-line bootstrap (GitHub reachable)

```bash
bash <(curl -s https://raw.githubusercontent.com/hedioum/Hedioum-Pool-Tunnel/main/install.sh)
```

This downloads the right binary, runs `hedioum-tunnel install`, and launches the
interactive wizard.

## Option B — manual (GitHub blocked)

Download the matching asset (`hedioum-tunnel` or `hedioum-tunnel-arm64`) on any
machine, copy it to the server, then:

```bash
chmod +x hedioum-tunnel
./hedioum-tunnel install      # self-copies to /usr/local/bin + installs the service
hedioum-tunnel                # interactive setup wizard
```

## Option C — fully non-interactive (automation / Ansible)

No wizard, no prompts.

**Foreign node:**
```bash
./hedioum-tunnel install
hedioum-tunnel setup-foreign --listen-port 22 --decoy-port 2022 --move-ssh
#   prints: Auth Token: <hex>
systemctl start hedioum.service
```

**Iran hub:**
```bash
./hedioum-tunnel install
hedioum-tunnel setup-iran \
  --alias DE-01 --target <FOREIGN_IP>:22 --socks-port 40001 --token <hex> \
  --min 10 --max 20 --bw 8 --jitter 2
systemctl start hedioum.service
hedioum-tunnel add-node --alias NL-02 --target <IP>:22 --socks-port 40002 --token <hex>
hedioum-tunnel remove-node --alias NL-02
```

## Ports

- `--listen-port` (default **22**): the public tunnel port. A non-22 port helps
  where outbound :22 is blocked, but weakens the SSH mimic — a warning is shown.
- `--decoy-port` (default **2022**): where the real `sshd` is relocated; the
  tunnel routes unauthenticated probes (and real admin SSH) there.
- `--egress-mode ipv4|ipv6|dual` and `--egress-bind-ip IP` control how the foreign
  node reaches the internet (default `ipv4`, no IPv6 identity leak).

## Updating

```bash
hedioum-tunnel update                       # download the latest release + rollback-safe swap
hedioum-tunnel update --file ./hedioum-tunnel   # install a locally-provided binary (GitHub blocked)
```

## Logs & debugging

The daemon logs to journald (structured, color-free):

```bash
journalctl -u hedioum.service -f
journalctl -u hedioum.service -p warning     # warnings and errors only
```

Turn on verbose diagnostics (e.g. to see why a connection dropped):

```bash
systemctl edit hedioum.service   # add:  [Service]\nEnvironment=HEDIOUM_LOG_LEVEL=debug
systemctl restart hedioum.service
```

## Management

```bash
hedioum-tunnel version           # version + build (commit/date)
hedioum-tunnel uninstall --yes   # stop, disable, and remove everything
```
