# 在 MikroTik / RouterOS 上运行 Hedioum

**语言：** [English](MIKROTIK.md) · [فارسی](MIKROTIK.fa.md) · [Русский](MIKROTIK.ru.md) · 中文

本指南把一台运行 RouterOS 7 的**全新 MikroTik** 变成一个可用的 Hedioum **枢纽（入口）**，
完全运行在 RouterOS *容器* 内——同时提供 SOCKS 代理和（可选的）操作系统级 TUN 接口。已在
RouterOS 7.18.2（x86 CHR）上针对一个真实的境外节点做了端到端验证。

> **你将获得：** 路由器本身通过伪装隧道连接到你的境外出口节点，并对外提供一个 SOCKS5 代理
> （和/或一个 TUN 接口 + 无泄漏的 DNS 解析器），供你局域网内的客户端使用。

---

## 0. 前置条件与安全

- **RouterOS 7.4+**（容器）；**推荐 7.12+**，本指南在 **7.18.2** 上测试。
- **架构：** 镜像发布了 `x86_64`（CHR）、`arm64` 和 `arm`（armv7）。请选择匹配的 MikroTik。
- **存储：** 镜像约 17 MB，解包需要空间。**仅有 16 MB 闪存的设备装不下**——请使用带 USB /
  NVMe / 更大闪存的型号（hAP ax²、RB5009、CCR、CHR……）。
- **内存：** ≥ 256 MB 较为宽裕。
- **一个境外节点及其 v2 配对令牌**（在出口服务器上运行 `hedioum-tunnel setup-foreign` 得到）。

> ⚠️ **容器是与安全相关的功能。** 启用它会降低 RouterOS 的部分加固。请只在你掌控的路由器上
> 这样做，并让容器网络（下方的 `172.20.0.0/24`）与不受信任的主机隔离。把 SOCKS 绑定到非回环
> 地址会在该网络上暴露一个开放代理——该网络必须可信。

> ⚠️ **不要把 CHR 的原始 `.img` 刷到以 UEFI 引导的云 VM 上**——CHR 镜像是 BIOS/MBR，会掉进
> UEFI shell。在 VPS 上请选择 BIOS/legacy 引导的套餐，或在本地 VM 中运行 CHR。本指南讲的是在
> RouterOS 上运行**容器**，与此无关。

---

## 1. 启用容器模式

容器支持被 *device-mode* 门控，需要一次**冷断电重启**来确认（一项安全措施——软重启不够）。

```rsc
/system/device-mode/print
/system/device-mode/update container=yes
```

RouterOS 会打印 `turn off power in N to activate changes`。**给设备断电再上电**（硬件上是物理
断电；VM 上是完整的 stop/start，而非软重启）。启动后：

```rsc
/system/device-mode/print   ;# container: yes
```

---

## 2. 安装 `container` 软件包

容器运行时作为独立软件包提供。

1. 在 mikrotik.com → Downloads 下载与你的架构和 RouterOS 版本对应的 **Extra packages** 包，
   例如 `all_packages-x86-7.18.2.zip`（或 `-arm-`、`-arm64-`）。
2. 从中解压出 **`container-<version>.npk`**。
3. 上传到路由器（在 WinBox 中拖放到 *Files*、`scp`，或
   `/tool/fetch url=http://<你的主机>/container-7.18.2.npk`）。
4. 通过重启来安装：

```rsc
/system/reboot
```

回来之后：

```rsc
/system/package/print    ;# 列表中出现 'container' 且已启用
/container/print         ;# /container 菜单现在存在了
```

---

## 3. 容器网络（veth + bridge + NAT）

给容器分配自己的地址，并通过路由器现有的 WAN 提供一条 NAT 上网路径：

```rsc
/interface/veth/add name=veth1 address=172.20.0.2/24 gateway=172.20.0.1
/interface/bridge/add name=cbr
/ip/address/add address=172.20.0.1/24 interface=cbr
/interface/bridge/port/add bridge=cbr interface=veth1
/ip/firewall/nat/add chain=srcnat action=masquerade src-address=172.20.0.0/24
```

容器将可通过 **`172.20.0.2`** 访问，并经由路由器连上互联网。

---

## 4. 创建 Hedioum 配置

枢纽读取 `/etc/hedioum/hedioum.json`。我们把它放在 RouterOS 的一个目录里并挂载进容器，这样
在重建容器后依然保留。用一个由**配对令牌**驱动的**一次性容器**来生成它——注意
`--socks-bind 172.20.0.2`，让 SOCKS 端口从局域网可达（而不仅是容器回环）：

```rsc
/container/config/set tmpdir=hedioum-tmp
/container/mounts/add name=hcfg src=hedioum-cfg dst=/etc/hedioum

# 一次性：写入配置，然后退出
/container/add file=hedioum-img.tar interface=veth1 mounts=hcfg root-dir=hedioum-setup \
    cmd="setup-iran --alias FR --token <PAIRING_TOKEN> --socks-port 40001 --socks-bind 172.20.0.2 --tun --dns" \
    logging=yes
/container/start [find where root-dir=hedioum-setup]
# 等几秒，确认配置已写入，然后删除这个一次性容器：
/log/print where topics~"container"
/container/remove [find where root-dir=hedioum-setup]
```

*（如何把 `hedioum-img.tar` 送上路由器见下一步——可以在第 5 步 fetch 之后再执行本步。只要
SOCKS 而不要 TUN 时，去掉 `--tun --dns`。）*

或者，在你的电脑上用 `hedioum-tunnel setup-iran … --socks-bind 172.20.0.2` 生成 `hedioum.json`
再上传到 `hedioum-cfg/`。

---

## 5. 获取镜像并添加容器

**方案 A — 从 GHCR 拉取（在软件包设为公开之后）：**

```rsc
/container/config/set registry-url=https://ghcr.io tmpdir=hedioum-tmp
/container/add remote-image=hedioum/pool-tunnel:latest interface=veth1 mounts=hcfg \
    root-dir=hedioum logging=yes start-on-boot=yes
```

**方案 B — 导入 tar（无需镜像仓库）：** 在装有 Docker 的机器上执行
`docker save ghcr.io/hedioum/pool-tunnel:latest -o hedioum-img.tar`，上传到路由器
（`/tool/fetch`、scp 或 WinBox），然后：

```rsc
/container/add file=hedioum-img.tar interface=veth1 mounts=hcfg \
    root-dir=hedioum logging=yes start-on-boot=yes
```

启动：

```rsc
/container/start [find where root-dir=hedioum]
```

---

## 6. 验证

```rsc
/container/print detail          ;# status=running, os=linux, arch=…
/log/print where topics~"container"
```

你应当看到类似这样的日志：

```
INFO hedioum daemon starting version=v0.10.1 role=iran
INFO SOCKS5 ingress active node=FR addr=172.20.0.2:40001
INFO TUN egress active node=FR iface=hedioum0 addr=10.200.0.1/24 dns=true
INFO pipe established node=FR mimic=tls target=<foreign-ip>:443
```

`pipe established` 表示到你境外节点的伪装隧道已建立。

---

## 7. 使用隧道

- **SOCKS5：** 让客户端指向 **`172.20.0.2:40001`**（你用 `--socks-bind` 设定的地址）。任何支持
  SOCKS5 的应用，或 Xray/sing-box 的 outbound，都可以使用它。DNS 在远端解析（无泄漏）。
- **整个局域网路由：** 运行一个透明代理容器（`tproxy` 模式的 Xray/sing-box）来消费这个 SOCKS，
  或在各设备上设置代理。（进阶；超出本指南范围。）
- **TUN + DNS（可选）：** 容器还在**容器内部**提供 `hedioum0`（10.200.0.1/24）和一个 `:53`
  转发器。它们在透明代理容器共享容器网络时最有用；纯 SOCKS 用法可以省略 `--tun --dns`。

---

## 8. 运行工具自带的命令（test、speedtest、edit）

镜像是 `FROM scratch`（无 shell），所以 `/container/shell` 不会给你一个提示符——但你仍可把任意
Hedioum 子命令作为共享配置挂载的**一次性容器**来运行：

```rsc
/container/add file=hedioum-img.tar interface=veth1 mounts=hcfg root-dir=hedioum-cmd \
    cmd="test --node FR" logging=yes
/container/start [find where root-dir=hedioum-cmd]
/log/print where topics~"container"          ;# 读取结果
/container/remove [find where root-dir=hedioum-cmd]
```

把 `cmd=` 换成 `speedtest --node FR`、`probe --node FR`、`check-ip` 等。

**在容器内修改配置**（`add-node` / `edit-node`）之后，没有 systemd 来自动重启守护进程——
**重启容器**以生效：

```rsc
/container/stop  [find where root-dir=hedioum]
/container/start [find where root-dir=hedioum]
```

*（在普通 Docker 上，对应命令是 `docker exec <name> hedioum-tunnel test --node FR`，以及改配置后
`docker restart <name>`。）*

---

## 9. 故障排查

| 现象 | 处理 |
|---|---|
| `/container` 菜单未知 | 未安装 container 软件包，或 device-mode 不是 `container: yes`（需要真正的断电重启）。 |
| `remote-image` 拉取报授权错误 | GHCR 软件包仍是**私有**——设为公开，或改用 tar（方案 B）。 |
| 日志里没有 `pipe established` | 容器没有网络——检查 veth/bridge/NAT（第 3 步）和路由器 WAN。 |
| 日志出现 `TUN not started …` | 在 device-mode/权限受限时属预期；SOCKS 仍可用。RouterOS 上容器具备 TUN 所需条件，请复查配置。 |
| 局域网无法访问 SOCKS | 你绑定到了 `127.0.0.1`；用 `--socks-bind 172.20.0.2`（veth 地址）重建。 |
| 镜像无法解包 | 空间不足——使用 USB/NVMe 或更大闪存的型号。 |

---

## 附录 — 发布镜像（维护者）

镜像由 `.github/workflows/docker-publish.yml` 在每个 `v*` 标签上自动构建并推送到 GHCR。**首次**
推送会把软件包创建为**私有**；在
`https://github.com/orgs/hedioum/packages/container/pool-tunnel/settings` → *Change visibility*
→ *Public* 处一次性设为公开即可。
