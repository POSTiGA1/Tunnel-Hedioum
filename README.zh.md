# Hedioum 动态连接池隧道 (Chaos Mesh)

[![Downloads](https://img.shields.io/github/downloads/hedioum/Hedioum-Pool-Tunnel/total?color=brightgreen&label=downloads)](https://github.com/hedioum/Hedioum-Pool-Tunnel/releases)
[![Latest release](https://img.shields.io/github/v/release/hedioum/Hedioum-Pool-Tunnel?color=blue&label=release)](https://github.com/hedioum/Hedioum-Pool-Tunnel/releases/latest)
[![Stars](https://img.shields.io/github/stars/hedioum/Hedioum-Pool-Tunnel?style=flat)](https://github.com/hedioum/Hedioum-Pool-Tunnel/stargazers)

🌐 **[🇬🇧 English](README.md)** · **[🇮🇷 فارسی](README.fa.md)** · **[🇷🇺 Русский](README.ru.md)** · **🇨🇳 中文**

Hedioum 是一款高性能、企业级的连接多路复用隧道，用于**绕过严格的深度包检测（DPI）与网络审查（翻墙 / 科学上网）**，并在高负载下避免 TCP 雪崩。它把加密的 VLESS/Trojan 流量包裹进动态伸缩的连接池中，这些连接可以佩戴可互换的**伪装外衣**——**SSH、TLS/HTTPS、SMTP/IMAP，以及 DirectAdmin 控制面板**——运行在现代认证加密之上，非常难以被识别或封锁。

> 架构：客户端 → Xray 面板（境内）→ **Hedioum 中枢（境内）** → 伪装隧道 → **境外出口节点** → 自由互联网。
> 出口节点生成令牌；先安装出口节点，再安装境内节点。

## 🌟 主要特性

- **Chaos Mesh 动态均衡：** 用「最少负载」算法取代简单轮询，按实时带宽（Mbps）而非仅连接数来扩缩物理连接。
- **抗 DPI（浮动限速）：** 每条物理连接在随机浮动的带宽上限下运行，打破静态特征，使流量看起来像普通、嘈杂的互联网流量。
- **零中断连接排空（Draining）：** 缩容时空闲连接进入 `Draining` 状态，等待其上逻辑流自然结束再关闭，用户无掉线、无卡顿。
- **认证加密（ChaCha20-Poly1305）：** 传输层由 HKDF 从你的令牌派生密钥的现代 AEAD 流保护。**令牌从不在链路上发送**，同时用作密钥，被动观察者只能看到随机盐 + 密文。
- **真实证书（Let's Encrypt）或 DirectAdmin 伪装：** 为节点指定域名（`--domain`）即可获得**自动续期的真实 Let's Encrypt 证书**（ACME，带 DNS A/AAAA 预检、自愈续期、以及安全的自签名回退）；无域名时使用自签名。节点还可佩戴 **DirectAdmin 控制面板伪装**：在 **2222 端口**提供像素级还原的 DirectAdmin *Evolution* 登录页（明/暗主题，资源内嵌于二进制），此处自签名证书是「正常」的。
- **协议感知的连接生命周期：** 除原始流量外，最强的隧道特征是「协议 × 连接存活时间」。SSH 是长寿命的主干；每条非 SSH 伪装都是辅助连接，具有**随机的 5–60 分钟寿命 + 1–5 GB 传输额度**，随后排空并churn 到全新连接。每台服务器的时序由其密钥派生，彼此各不相同。
- **完整 TCP + UDP：** 单个本地 SOCKS5 端口同时服务 TCP（CONNECT）与 UDP（ASSOCIATE），因此 QUIC/HTTP3、DNS、语音/视频通话可用。UDP 走独立子连接池，与 TCP 隔离；节点间从不出现裸 UDP——它被封装在伪装的 TCP 链路里。
- **可选 IPv6：** 节点间链路与出口可用 IPv6（`egress_ip_mode`：`ipv4` 默认 / `ipv6` / `dual`），默认不泄露服务器的 IPv6 身份。
- **可插拔协议伪装（军火库）：** 一个节点可同时监听多种伪装——**SSH**（逐字节镜像主机自身 `sshd` 的真实 SSH-2.0 banner）、**TLS/HTTPS**、**STARTTLS SMTP/IMAP**（587/143）、**隐式 TLS SMTPS/IMAPS**（465/993），以及 **2222 端口的 DirectAdmin 面板**。中枢以**每台独特的浮动分布**在它们之间分散物理连接（Chaos Mesh v2）。未授权探测会被路由到与协议相符的诱饵（真实 `sshd`、DirectAdmin 登录页，或可用 `--decoy-style` 选择的 Apache/DirectAdmin 网页伪装，其 `Server`/`ETag`/`Last-Modified` **每台唯一**）；**80 端口**也运行诱饵，让裸 IP 在信誉扫描器眼里像普通网站主机。
- **设计上不泄露 DNS：** SOCKS5 入口把**目标域名**（而非本地解析出的 IP）透传进隧道，DNS 在**境外节点**解析——客户端设为远程 DNS（`domainStrategy: AsIs`）即可。
- **信道绑定令牌认证（无双重加密）：** TLS 伪装用绑定到服务器实时证书指纹的 HMAC 认证对端，**证明持有令牌却从不发送它**，同时只保留**单层**加密以获得统一高吞吐。客户端用 **uTLS**（真实 Chrome ClientHello）击败 JA3 指纹。
- **吞吐工程：** 安装时启用 BBR + `fq`，Yamux 流窗口放宽到 16MB，内置 `speedtest` 与 `probe` 命令。

## 🚀 安装与更新

先安装**境外（出口）节点**以生成境内节点所需的令牌。完整指南见 **[docs/DEPLOY.md](docs/DEPLOY.md)**。

### 一键安装（GitHub 可达时）
在每台 VPS 以 root 运行（先境外）：

    bash <(curl -fsSL https://raw.githubusercontent.com/hedioum/Hedioum-Pool-Tunnel/main/install.sh)

### 非交互式（自动化）

    hedioum-tunnel install
    hedioum-tunnel setup-foreign --mimics all --move-ssh --domain vpn.example.com --decoy-style directadmin   # 境外；打印令牌
    hedioum-tunnel setup-iran   --alias DE-01 --target-ip <IP> --mimics all --socks-port 40001 --token <hex>
    systemctl start hedioum.service

`--mimics all` 启用全部军火库（`ssh,tls,smtp,imap,smtps,imaps,directadmin`）。`--domain` 可选（真实 Let's Encrypt 证书），`--decoy-style` 为 `apache`（默认）或 `directadmin`。

**就地编辑节点**（仅改你传入的字段；令牌保持不变，除非 `--token`/`--rotate-token`）：

    hedioum-tunnel edit-foreign --tls-port 2087 --decoy-style directadmin   # 境外
    hedioum-tunnel edit-node --alias DE-01 --bw 12                          # 境内中枢

### 诊断

    hedioum-tunnel probe     --node DE-01       # 每种伪装的可达性（中枢）
    hedioum-tunnel speedtest --node DE-01       # 真实的无整形出口吞吐（中枢）
    hedioum-tunnel check-ip                     # 出口 IP 信誉：CLEAN / LIKELY-FLAGGED（境外）

## ⚙️ 管理面板

随时从终端运行交互式面板：

    hedioum-tunnel

可查看活动连接池与每节点端点、监控实时日志、动态**添加/编辑/删除**节点、运行 speedtest / probe / check-ip、安全自更新、彻底卸载。

## 🛠 从源码构建

    git clone https://github.com/hedioum/Hedioum-Pool-Tunnel.git
    cd Hedioum-Pool-Tunnel
    make all        # linux amd64 + arm64

## ☕ 支持

如果这个项目对维护自由开放的互联网有帮助，欢迎给团队买杯咖啡（USDT）：
- **TRC20:** TRhwZFoHRZ9oux4emFXTj63aib9nuC2J2J
- **BEP20 / ERC20:** 0x051e31cb70076854C0b62F816d5a89D3def4A22E
- **TON:** UQCqq0wYNDVhq9AXAZ5vOQ2ZgMmP6O0UTgvU1YhNeIpkUp1s

感谢支持！🙏
