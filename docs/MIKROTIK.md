# Running Hedioum on MikroTik / RouterOS

**Languages:** English · [فارسی](MIKROTIK.fa.md) · [Русский](MIKROTIK.ru.md) · [中文](MIKROTIK.zh.md)

This guide takes a **bare MikroTik** running RouterOS 7 and turns it into a working
Hedioum **hub (ingress)** that runs entirely inside a RouterOS *container* — with both
the SOCKS proxy and (optionally) an OS-level TUN interface. It has been validated on
RouterOS 7.18.2 (x86 CHR) end-to-end against a live foreign node.

> **What you get:** the router itself connects to your foreign egress node through the
> disguised tunnel, and exposes a SOCKS5 proxy (and/or a TUN interface + a leak-free DNS
> resolver) that clients on your LAN can use.

---

## 0. Requirements & safety

- **RouterOS 7.4+** (containers); **7.12+ recommended**, guide tested on **7.18.2**.
- **Architecture:** the image is published for `x86_64` (CHR), `arm64`, and `arm` (armv7).
  Pick the matching MikroTik.
- **Storage:** the image is ~17 MB and needs room to unpack. Devices with **only 16 MB
  flash will not fit** — use a model with a USB stick / NVMe / larger flash (hAP ax²,
  RB5009, CCR, CHR, …).
- **RAM:** ≥ 256 MB is comfortable.
- **A foreign node + its v2 pairing token** (from `hedioum-tunnel setup-foreign` on your
  egress server).

> ⚠️ **Containers are a security-relevant feature.** Enabling them lowers some RouterOS
> hardening. Only do this on a router you control, and keep the container's network
> (`172.20.0.0/24` below) isolated from untrusted hosts. Binding SOCKS to a non-loopback
> address exposes an open proxy on that network — keep it trusted.

> ⚠️ **Do NOT flash the raw CHR `.img` onto a cloud VM that boots UEFI** — CHR's image is
> BIOS/MBR and will drop to a UEFI shell. On a VPS, either pick a BIOS/legacy-boot plan or
> run CHR inside a local VM. This guide is about running the **container** on RouterOS,
> which is independent of that.

---

## 1. Enable container mode

Container support is gated behind *device-mode*, which requires a **cold power-cycle** to
confirm (a security measure — a soft reboot is not enough).

```rsc
/system/device-mode/print
/system/device-mode/update container=yes
```

RouterOS prints `turn off power in N to activate changes`. **Power the device off and on**
(on hardware: physically; on a VM: a full stop/start, not a soft reboot). After it boots:

```rsc
/system/device-mode/print   ;# container: yes
```

---

## 2. Install the `container` package

The container runtime ships as a separate package.

1. On mikrotik.com → Downloads, grab the **Extra packages** bundle for your arch and
   RouterOS version, e.g. `all_packages-x86-7.18.2.zip` (or `-arm-`, `-arm64-`).
2. Extract **`container-<version>.npk`** from it.
3. Upload it to the router (WinBox drag-and-drop into *Files*, `scp`, or
   `/tool/fetch url=http://<your-host>/container-7.18.2.npk`).
4. Install it by rebooting:

```rsc
/system/reboot
```

After it comes back:

```rsc
/system/package/print    ;# 'container' listed and enabled
/container/print         ;# the /container menu now exists
```

---

## 3. Container networking (veth + bridge + NAT)

Give the container its own address and a NAT'd path to the internet through the router's
existing WAN:

```rsc
/interface/veth/add name=veth1 address=172.20.0.2/24 gateway=172.20.0.1
/interface/bridge/add name=cbr
/ip/address/add address=172.20.0.1/24 interface=cbr
/interface/bridge/port/add bridge=cbr interface=veth1
/ip/firewall/nat/add chain=srcnat action=masquerade src-address=172.20.0.0/24
```

The container will be reachable at **`172.20.0.2`** and reach the internet via the router.

---

## 4. Create the Hedioum config

The hub reads `/etc/hedioum/hedioum.json`. We keep it on a RouterOS directory and mount it
into the container, so it survives container re-creation. Generate it with a **one-shot
container** driven by your foreign's **pairing token** — note `--socks-bind 172.20.0.2` so
the SOCKS port is reachable from your LAN (not just the container loopback):

```rsc
/container/config/set tmpdir=hedioum-tmp
/container/mounts/add name=hcfg src=hedioum-cfg dst=/etc/hedioum

# one-shot: write the config, then exit
/container/add file=hedioum-img.tar interface=veth1 mounts=hcfg root-dir=hedioum-setup \
    cmd="setup-iran --alias FR --token <PAIRING_TOKEN> --socks-port 40001 --socks-bind 172.20.0.2 --tun --dns" \
    logging=yes
/container/start [find where root-dir=hedioum-setup]
# wait a few seconds, confirm it wrote the config, then remove the one-shot:
/log/print where topics~"container"
/container/remove [find where root-dir=hedioum-setup]
```

*(Getting `hedioum-img.tar` onto the router is covered in the next step — you can run this
after step 5's fetch. Drop `--tun --dns` for a SOCKS-only setup.)*

Alternatively, generate `hedioum.json` on your PC with
`hedioum-tunnel setup-iran … --socks-bind 172.20.0.2` and upload it to `hedioum-cfg/`.

---

## 5. Get the image and add the container

**Option A — pull from GHCR (once the package is public):**

```rsc
/container/config/set registry-url=https://ghcr.io tmpdir=hedioum-tmp
/container/add remote-image=hedioum/pool-tunnel:v0.10.1 interface=veth1 mounts=hcfg \
    root-dir=hedioum logging=yes start-on-boot=yes
```

**Option B — import a tar (no registry needed):** on a machine with Docker,
`docker save ghcr.io/hedioum/pool-tunnel:v0.10.1 -o hedioum-img.tar`, upload it to the
router (`/tool/fetch`, scp, or WinBox), then:

```rsc
/container/add file=hedioum-img.tar interface=veth1 mounts=hcfg \
    root-dir=hedioum logging=yes start-on-boot=yes
```

Start it:

```rsc
/container/start [find where root-dir=hedioum]
```

---

## 6. Verify

```rsc
/container/print detail          ;# status=running, os=linux, arch=…
/log/print where topics~"container"
```

You should see lines like:

```
INFO hedioum daemon starting version=v0.10.1 role=iran
INFO SOCKS5 ingress active node=FR addr=172.20.0.2:40001
INFO TUN egress active node=FR iface=hedioum0 addr=10.200.0.1/24 dns=true
INFO pipe established node=FR mimic=tls target=<foreign-ip>:443
```

`pipe established` means the disguised tunnel to your foreign is up.

---

## 7. Use the tunnel

- **SOCKS5:** point clients at **`172.20.0.2:40001`** (the address you set with
  `--socks-bind`). Any SOCKS5-aware app, or an Xray/sing-box outbound, can use it. DNS is
  resolved remotely (no leak).
- **Whole-LAN routing:** run a transparent-proxy container (Xray/sing-box in `tproxy` mode)
  that consumes this SOCKS, or set the proxy on individual devices. (Advanced; beyond this
  guide.)
- **TUN + DNS (optional):** the container also exposes `hedioum0` (10.200.0.1/24) and a
  `:53` forwarder **inside** the container. These are most useful when a transparent-proxy
  container shares the container's network; for plain SOCKS use you can skip `--tun --dns`.

---

## 8. Running the tool's own commands (test, speedtest, edit)

The image is `FROM scratch` (no shell), so `/container/shell` won't give you a prompt — but
you can still run any Hedioum subcommand as a **one-shot container** that shares the config
mount:

```rsc
/container/add file=hedioum-img.tar interface=veth1 mounts=hcfg root-dir=hedioum-cmd \
    cmd="test --node FR" logging=yes
/container/start [find where root-dir=hedioum-cmd]
/log/print where topics~"container"          ;# read the result
/container/remove [find where root-dir=hedioum-cmd]
```

Swap `cmd=` for `speedtest --node FR`, `probe --node FR`, `check-ip`, etc.

**After changing config** (`add-node` / `edit-node`) inside a container, there is no systemd
to auto-restart the daemon — **restart the container** to apply:

```rsc
/container/stop  [find where root-dir=hedioum]
/container/start [find where root-dir=hedioum]
```

*(On plain Docker the equivalents are `docker exec <name> hedioum-tunnel test --node FR`,
and `docker restart <name>` after a config edit.)*

---

## 9. Troubleshooting

| Symptom | Fix |
|---|---|
| `/container` menu is unknown | Container package not installed, or device-mode not `container: yes` (needs a real power-cycle). |
| `remote-image` pull fails with auth error | The GHCR package is still **private** — make it public, or use the tar (Option B). |
| No `pipe established` in the log | The container has no internet — check the veth/bridge/NAT (step 3) and the router's WAN. |
| `TUN not started …` in the log | Expected if device-mode/caps are limited; SOCKS still works. On RouterOS the container has what TUN needs, so re-check the config. |
| SOCKS unreachable from LAN | You bound it to `127.0.0.1`; re-create with `--socks-bind 172.20.0.2` (the veth IP). |
| Image won't unpack | Not enough storage — use USB/NVMe or a larger-flash model. |

---

## Appendix — publishing the image (maintainers)

The image is built and pushed to GHCR automatically by
`.github/workflows/docker-publish.yml` on every `v*` tag. The **first** push creates the
package as **private**; make it public once at
`https://github.com/orgs/hedioum/packages/container/pool-tunnel/settings` → *Change
visibility* → *Public*.
