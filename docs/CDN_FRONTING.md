# CDN fronting (hiding the foreign IP behind a CDN)

A datacenter egress IP can be blocked by address, and some IPs are pre-flagged
(check yours with `hedioum-tunnel check-ip`). Putting a CDN in front hides the real
foreign IP behind the CDN's edge, so clients connect to a hard-to-block CDN address
and the origin IP is never exposed on the wire.

## Recommended pattern (today: at the client/proxy layer)

Hedioum is an app-layer TCP tunnel: the Iran hub exposes a local SOCKS5 port that
your VLESS/Trojan proxy (Xray / 3x-ui / sing-box) uses as an **outbound**. CDN
fronting is done at that proxy layer, which already supports it well:

```
client → CDN edge (e.g. Cloudflare / ArvanCloud, WS or gRPC + TLS)
       → your VLESS inbound on the Iran hub
       → outbound: SOCKS5 127.0.0.1:<hub port>   (Hedioum)
       → masked tunnel → foreign egress → internet
```

- Point a domain at the CDN and enable the proxy (orange cloud). The CDN terminates
  TLS for the domain; the origin IP stays hidden.
- Use a **WebSocket** (or gRPC) transport on the VLESS inbound so it rides standard
  CDN HTTP(S); set the `Host`/SNI to your fronted domain and the path the CDN routes.
- Keep the Hedioum hub bound to `127.0.0.1` (default) — only the local proxy reaches
  it; nothing about Hedioum is exposed to the CDN.

This is the approach in production use (VLESS+WS behind a CDN, outbound to the
Hedioum SOCKS port) and needs no change to Hedioum itself.

## Notes

- The **foreign** server still benefits from a clean IP and a real domain/cert
  (`--domain`), and from `check-ip` before you commit to an address.
- A CDN in front of the **hub** protects the *hub's* reachability/identity; the
  foreign egress IP is protected by keeping it off any public DNS and, if needed,
  fronting the hub→foreign path through a domestic edge during shutdowns
  (see the reachability notes in the roadmap).

## Future work

A built-in WebSocket/gRPC transport inside Hedioum (so the hub↔foreign link itself
can traverse a CDN without an external proxy) is tracked as a later enhancement.
