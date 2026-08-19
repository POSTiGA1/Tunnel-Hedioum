# اجرای Hedioum روی MikroTik / RouterOS

**زبان‌ها:** [English](MIKROTIK.md) · فارسی · [Русский](MIKROTIK.ru.md) · [中文](MIKROTIK.zh.md)

این راهنما یک **میکروتیکِ خام** با RouterOS 7 را می‌گیرد و تبدیلش می‌کند به یک **هابِ (ورودیِ)**
Hedioum که کاملاً داخلِ یک *کانتینرِ* RouterOS اجرا می‌شود — هم پراکسیِ SOCKS و هم (به‌اختیار)
یک اینترفیسِ TUN در سطحِ سیستم‌عامل. این مسیر روی RouterOS 7.18.2 (x86 CHR) به‌صورت
سرتاسری و در برابرِ یک نودِ خارجیِ زنده تست شده است.

> **چه چیزی به‌دست می‌آید:** خودِ روتر از دلِ تونلِ استتارشده به نودِ خروجیِ خارجی وصل می‌شود و
> یک پراکسیِ SOCKS5 (و/یا یک اینترفیسِ TUN + یک resolverِ DNSِ بدونِ نشت) در اختیار می‌گذارد که
> کلاینت‌های شبکهٔ محلی می‌توانند از آن استفاده کنند.

---

## ۰. پیش‌نیازها و ایمنی

- **RouterOS 7.4 به بالا** (برای کانتینر)؛ **7.12+ توصیه می‌شود**، این راهنما روی **7.18.2** تست شده.
- **معماری:** ایمیج برای `x86_64` (CHR)، `arm64` و `arm` (armv7) منتشر شده. میکروتیکِ متناظر را انتخاب کنید.
- **فضای ذخیره‌سازی:** ایمیج حدود ۱۷MB است و برای باز شدن جا لازم دارد. دستگاه‌هایی با **فلشِ فقط ۱۶MB
  جا نمی‌دهند** — از مدلی با USB / NVMe / فلشِ بزرگ‌تر استفاده کنید (hAP ax²، RB5009، CCR، CHR، …).
- **رم:** ۲۵۶MB به بالا راحت است.
- **یک نودِ خارجی + توکنِ pairingِ v2 آن** (خروجیِ `hedioum-tunnel setup-foreign` روی سرورِ خروجی).

> ⚠️ **کانتینرها یک قابلیتِ حساس از نظر امنیتی‌اند.** فعال‌کردنشان بخشی از سخت‌سازیِ RouterOS را کم می‌کند.
> فقط روی روتری که کنترلش با شماست این کار را بکنید و شبکهٔ کانتینر (`172.20.0.0/24` پایین) را از میزبان‌های
> نامطمئن جدا نگه دارید. بایند کردنِ SOCKS به آدرسِ غیرِ loopback یعنی یک پراکسیِ باز روی آن شبکه — آن شبکه باید مورد اعتماد باشد.

> ⚠️ **ایمیجِ خامِ CHR را روی VMِ ابری‌ای که با UEFI بوت می‌شود ننویسید** — ایمیجِ CHR بایوس/MBR است و
> می‌افتد توی شلِ UEFI. روی VPS یا پلنی با بوتِ BIOS/legacy بگیرید یا CHR را داخلِ یک VMِ محلی اجرا کنید.
> این راهنما دربارهٔ اجرای **کانتینر** روی RouterOS است که مستقل از آن موضوع است.

---

## ۱. فعال‌کردنِ حالتِ کانتینر

پشتیبانیِ کانتینر پشتِ *device-mode* قفل است و برای تأیید نیاز به **خاموش/روشنِ سختِ برق** دارد (یک اقدامِ
امنیتی — ریبوتِ نرم کافی نیست).

```rsc
/system/device-mode/print
/system/device-mode/update container=yes
```

RouterOS پیام می‌دهد `turn off power in N to activate changes`. **دستگاه را خاموش و روشن کنید** (روی سخت‌افزار:
فیزیکی؛ روی VM: یک stop/start کامل، نه ریبوتِ نرم). بعد از بالا آمدن:

```rsc
/system/device-mode/print   ;# container: yes
```

---

## ۲. نصبِ پکیجِ `container`

رانتایمِ کانتینر به‌صورتِ یک پکیجِ جدا عرضه می‌شود.

1. از mikrotik.com → Downloads، بستهٔ **Extra packages** را برای معماری و نسخهٔ RouterOSِ خود بگیرید،
   مثلاً `all_packages-x86-7.18.2.zip` (یا `-arm-`، `-arm64-`).
2. **`container-<version>.npk`** را از آن استخراج کنید.
3. آن را روی روتر آپلود کنید (کشیدن‌ودرکردن در WinBox به *Files*، `scp`، یا
   `/tool/fetch url=http://<host-شما>/container-7.18.2.npk`).
4. با ریبوت نصبش کنید:

```rsc
/system/reboot
```

بعد از بالا آمدن:

```rsc
/system/package/print    ;# 'container' در فهرست و فعال
/container/print         ;# منوی /container حالا وجود دارد
```

---

## ۳. شبکهٔ کانتینر (veth + bridge + NAT)

به کانتینر آدرسِ خودش و یک مسیرِ NATشده به اینترنت از طریقِ WANِ موجودِ روتر بدهید:

```rsc
/interface/veth/add name=veth1 address=172.20.0.2/24 gateway=172.20.0.1
/interface/bridge/add name=cbr
/ip/address/add address=172.20.0.1/24 interface=cbr
/interface/bridge/port/add bridge=cbr interface=veth1
/ip/firewall/nat/add chain=srcnat action=masquerade src-address=172.20.0.0/24
```

کانتینر روی **`172.20.0.2`** در دسترس خواهد بود و از طریقِ روتر به اینترنت می‌رسد.

---

## ۴. ساختِ کانفیگِ Hedioum

هاب `/etc/hedioum/hedioum.json` را می‌خواند. آن را روی یک دایرکتوریِ RouterOS نگه می‌داریم و داخلِ کانتینر
mount می‌کنیم تا با بازساختِ کانتینر از بین نرود. با یک **کانتینرِ یک‌بارمصرف** که با **توکنِ pairing** رانده
می‌شود بسازیدش — به `--socks-bind 172.20.0.2` دقت کنید تا پورتِ SOCKS از شبکهٔ محلی در دسترس باشد (نه فقط
loopbackِ کانتینر):

```rsc
/container/config/set tmpdir=hedioum-tmp
/container/mounts/add name=hcfg src=hedioum-cfg dst=/etc/hedioum

# یک‌بارمصرف: کانفیگ را بنویس، بعد خارج شو
/container/add file=hedioum-img.tar interface=veth1 mounts=hcfg root-dir=hedioum-setup \
    cmd="setup-iran --alias FR --token <PAIRING_TOKEN> --socks-port 40001 --socks-bind 172.20.0.2 --tun --dns" \
    logging=yes
/container/start [find where root-dir=hedioum-setup]
# چند ثانیه صبر، نوشته‌شدنِ کانفیگ را تأیید، بعد یک‌بارمصرف را حذف کنید:
/log/print where topics~"container"
/container/remove [find where root-dir=hedioum-setup]
```

*(رساندنِ `hedioum-img.tar` به روتر در مرحلهٔ بعد توضیح داده شده — می‌توانید این را بعد از fetchِ مرحلهٔ ۵ اجرا
کنید. برای حالتِ فقط-SOCKS، `--tun --dns` را حذف کنید.)*

جایگزین: `hedioum.json` را روی کامپیوترِ خود با `hedioum-tunnel setup-iran … --socks-bind 172.20.0.2` بسازید و
در `hedioum-cfg/` آپلود کنید.

---

## ۵. گرفتنِ ایمیج و افزودنِ کانتینر

**گزینهٔ الف — pull از GHCR (بعد از اینکه پکیج public شد):**

```rsc
/container/config/set registry-url=https://ghcr.io tmpdir=hedioum-tmp
/container/add remote-image=hedioum/pool-tunnel:latest interface=veth1 mounts=hcfg \
    root-dir=hedioum logging=yes start-on-boot=yes
```

**گزینهٔ ب — ایمپورتِ tar (بدونِ نیاز به registry):** روی یک ماشینِ دارای Docker،
`docker save ghcr.io/hedioum/pool-tunnel:latest -o hedioum-img.tar`، آن را روی روتر آپلود کنید
(`/tool/fetch`، scp یا WinBox)، بعد:

```rsc
/container/add file=hedioum-img.tar interface=veth1 mounts=hcfg \
    root-dir=hedioum logging=yes start-on-boot=yes
```

اجرا:

```rsc
/container/start [find where root-dir=hedioum]
```

---

## ۶. تأیید

```rsc
/container/print detail          ;# status=running, os=linux, arch=…
/log/print where topics~"container"
```

باید خطوطی مثلِ این ببینید:

```
INFO hedioum daemon starting version=v0.10.1 role=iran
INFO SOCKS5 ingress active node=FR addr=172.20.0.2:40001
INFO TUN egress active node=FR iface=hedioum0 addr=10.200.0.1/24 dns=true
INFO pipe established node=FR mimic=tls target=<foreign-ip>:443
```

`pipe established` یعنی تونلِ استتارشده به نودِ خارجیِ شما بالا است.

---

## ۷. استفاده از تونل

- **SOCKS5:** کلاینت‌ها را به **`172.20.0.2:40001`** وصل کنید (همان آدرسی که با `--socks-bind` ست کردید).
  هر اپِ آگاه به SOCKS5، یا outboundِ Xray/sing-box می‌تواند از آن استفاده کند. DNS از راهِ دور resolve می‌شود (بدونِ نشت).
- **روتینگِ کلِ شبکهٔ محلی:** یک کانتینرِ transparent-proxy (Xray/sing-box در حالتِ `tproxy`) اجرا کنید که این
  SOCKS را مصرف کند، یا پراکسی را روی تک‌تکِ دستگاه‌ها ست کنید. (پیشرفته؛ خارج از این راهنما.)
- **TUN + DNS (اختیاری):** کانتینر همچنین `hedioum0` (10.200.0.1/24) و یک forwarderِ `:53` را **داخلِ** کانتینر
  عرضه می‌کند. این‌ها بیشتر وقتی مفیدند که یک کانتینرِ transparent-proxy شبکهٔ کانتینر را به‌اشتراک بگذارد؛ برای
  استفادهٔ ساده از SOCKS می‌توانید `--tun --dns` را حذف کنید.

---

## ۸. اجرای دستوراتِ خودِ ابزار (test, speedtest, edit)

ایمیج `FROM scratch` است (بدونِ شل)، پس `/container/shell` پرامپتی نمی‌دهد — اما همچنان می‌توانید هر
زیردستورِ Hedioum را به‌صورتِ یک **کانتینرِ یک‌بارمصرف** که همان mountِ کانفیگ را به‌اشتراک می‌گذارد اجرا کنید:

```rsc
/container/add file=hedioum-img.tar interface=veth1 mounts=hcfg root-dir=hedioum-cmd \
    cmd="test --node FR" logging=yes
/container/start [find where root-dir=hedioum-cmd]
/log/print where topics~"container"          ;# نتیجه را بخوانید
/container/remove [find where root-dir=hedioum-cmd]
```

`cmd=` را با `speedtest --node FR`، `probe --node FR`، `check-ip` و … عوض کنید.

**بعد از تغییرِ کانفیگ** (`add-node` / `edit-node`) داخلِ کانتینر، systemdای نیست که دیمن را خودکار ری‌استارت کند —
**کانتینر را ری‌استارت کنید** تا اعمال شود:

```rsc
/container/stop  [find where root-dir=hedioum]
/container/start [find where root-dir=hedioum]
```

*(روی Dockerِ معمولی معادل‌ها این‌اند: `docker exec <name> hedioum-tunnel test --node FR` و بعد از ویرایشِ
کانفیگ `docker restart <name>`.)*

---

## ۹. عیب‌یابی

| نشانه | راه‌حل |
|---|---|
| منوی `/container` ناشناخته است | پکیجِ container نصب نشده، یا device-mode روی `container: yes` نیست (نیاز به خاموش/روشنِ واقعی). |
| pullِ `remote-image` خطای احراز می‌دهد | پکیجِ GHCR هنوز **private** است — public کنید، یا از tar (گزینهٔ ب) استفاده کنید. |
| هیچ `pipe established`ای در لاگ نیست | کانتینر اینترنت ندارد — veth/bridge/NAT (مرحلهٔ ۳) و WANِ روتر را چک کنید. |
| `TUN not started …` در لاگ | اگر device-mode/کپبیلیتی‌ها محدود باشند انتظار می‌رود؛ SOCKS همچنان کار می‌کند. روی RouterOS کانتینر آنچه TUN لازم دارد را دارد، پس کانفیگ را دوباره چک کنید. |
| SOCKS از شبکهٔ محلی در دسترس نیست | آن را به `127.0.0.1` بایند کرده‌اید؛ با `--socks-bind 172.20.0.2` (آدرسِ veth) دوباره بسازید. |
| ایمیج باز نمی‌شود | فضای کافی نیست — از USB/NVMe یا مدلی با فلشِ بزرگ‌تر استفاده کنید. |

---

## پیوست — انتشارِ ایمیج (برای نگه‌دارندگان)

ایمیج به‌صورتِ خودکار توسطِ `.github/workflows/docker-publish.yml` روی هر تگِ `v*` بیلد و به GHCR پوش می‌شود.
**اولین** پوش پکیج را **private** می‌سازد؛ یک‌بار آن را در
`https://github.com/orgs/hedioum/packages/container/pool-tunnel/settings` → *Change visibility* → *Public* عمومی کنید.
