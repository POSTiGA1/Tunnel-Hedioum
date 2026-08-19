# Запуск Hedioum на MikroTik / RouterOS

**Языки:** [English](MIKROTIK.md) · [فارسی](MIKROTIK.fa.md) · Русский · [中文](MIKROTIK.zh.md)

Это руководство превращает **чистый MikroTik** с RouterOS 7 в рабочий **хаб (вход)**
Hedioum, полностью работающий внутри *контейнера* RouterOS — с SOCKS-прокси и
(опционально) TUN-интерфейсом уровня ОС. Проверено сквозным тестом на RouterOS 7.18.2
(x86 CHR) против живого зарубежного узла.

> **Что вы получаете:** сам маршрутизатор подключается к вашему зарубежному узлу выхода
> через замаскированный туннель и предоставляет SOCKS5-прокси (и/или TUN-интерфейс +
> DNS-резолвер без утечек), которым могут пользоваться клиенты в вашей локальной сети.

---

## 0. Требования и безопасность

- **RouterOS 7.4+** (контейнеры); **рекомендуется 7.12+**, руководство проверено на **7.18.2**.
- **Архитектура:** образ публикуется для `x86_64` (CHR), `arm64` и `arm` (armv7). Выберите
  соответствующий MikroTik.
- **Хранилище:** образ ~17 МБ и требует места для распаковки. Устройства **только с 16 МБ
  флеш-памяти не подойдут** — используйте модель с USB / NVMe / бо́льшим флешем (hAP ax²,
  RB5009, CCR, CHR, …).
- **ОЗУ:** ≥ 256 МБ комфортно.
- **Зарубежный узел + его pairing-токен v2** (вывод `hedioum-tunnel setup-foreign` на сервере выхода).

> ⚠️ **Контейнеры — это функция, важная для безопасности.** Их включение снижает часть
> защиты RouterOS. Делайте это только на маршрутизаторе, который контролируете, и держите
> сеть контейнера (`172.20.0.0/24` ниже) изолированной от недоверенных хостов. Привязка SOCKS
> к не-loopback адресу открывает прокси в этой сети — она должна быть доверенной.

> ⚠️ **Не записывайте сырой образ CHR `.img` на облачную ВМ с загрузкой UEFI** — образ CHR
> BIOS/MBR и упадёт в UEFI-shell. На VPS выберите план с BIOS/legacy-загрузкой или запустите
> CHR внутри локальной ВМ. Это руководство о запуске **контейнера** на RouterOS, что от этого не зависит.

---

## 1. Включение режима контейнеров

Поддержка контейнеров закрыта за *device-mode* и требует **холодного перезапуска питания**
для подтверждения (мера безопасности — мягкой перезагрузки недостаточно).

```rsc
/system/device-mode/print
/system/device-mode/update container=yes
```

RouterOS напишет `turn off power in N to activate changes`. **Выключите и включите питание**
(на железе — физически; на ВМ — полный stop/start, не мягкий reboot). После загрузки:

```rsc
/system/device-mode/print   ;# container: yes
```

---

## 2. Установка пакета `container`

Среда выполнения контейнеров поставляется отдельным пакетом.

1. На mikrotik.com → Downloads возьмите набор **Extra packages** для вашей архитектуры и
   версии RouterOS, например `all_packages-x86-7.18.2.zip` (или `-arm-`, `-arm64-`).
2. Извлеките из него **`container-<version>.npk`**.
3. Загрузите на маршрутизатор (drag-and-drop в WinBox в *Files*, `scp` или
   `/tool/fetch url=http://<ваш-хост>/container-7.18.2.npk`).
4. Установите перезагрузкой:

```rsc
/system/reboot
```

После загрузки:

```rsc
/system/package/print    ;# 'container' в списке и включён
/container/print         ;# меню /container теперь существует
```

---

## 3. Сеть контейнера (veth + bridge + NAT)

Дайте контейнеру собственный адрес и NAT-путь в интернет через существующий WAN маршрутизатора:

```rsc
/interface/veth/add name=veth1 address=172.20.0.2/24 gateway=172.20.0.1
/interface/bridge/add name=cbr
/ip/address/add address=172.20.0.1/24 interface=cbr
/interface/bridge/port/add bridge=cbr interface=veth1
/ip/firewall/nat/add chain=srcnat action=masquerade src-address=172.20.0.0/24
```

Контейнер будет доступен по **`172.20.0.2`** и выйдет в интернет через маршрутизатор.

---

## 4. Создание конфигурации Hedioum

Хаб читает `/etc/hedioum/hedioum.json`. Держим его в каталоге RouterOS и монтируем в
контейнер, чтобы он пережил пересоздание контейнера. Сгенерируйте его **одноразовым
контейнером** на основе **pairing-токена** — обратите внимание на `--socks-bind 172.20.0.2`,
чтобы порт SOCKS был доступен из LAN (а не только на loopback контейнера):

```rsc
/container/config/set tmpdir=hedioum-tmp
/container/mounts/add name=hcfg src=hedioum-cfg dst=/etc/hedioum

# одноразовый: записать конфиг и выйти
/container/add file=hedioum-img.tar interface=veth1 mounts=hcfg root-dir=hedioum-setup \
    cmd="setup-iran --alias FR --token <PAIRING_TOKEN> --socks-port 40001 --socks-bind 172.20.0.2 --tun --dns" \
    logging=yes
/container/start [find where root-dir=hedioum-setup]
# подождите пару секунд, убедитесь, что конфиг записан, затем удалите одноразовый:
/log/print where topics~"container"
/container/remove [find where root-dir=hedioum-setup]
```

*(Как доставить `hedioum-img.tar` на маршрутизатор — в следующем шаге; можно выполнить это
после fetch из шага 5. Для варианта только-SOCKS уберите `--tun --dns`.)*

Как вариант — сгенерируйте `hedioum.json` на своём ПК командой
`hedioum-tunnel setup-iran … --socks-bind 172.20.0.2` и загрузите в `hedioum-cfg/`.

---

## 5. Получение образа и добавление контейнера

**Вариант A — pull из GHCR (после того как пакет станет публичным):**

```rsc
/container/config/set registry-url=https://ghcr.io tmpdir=hedioum-tmp
/container/add remote-image=hedioum/pool-tunnel:latest interface=veth1 mounts=hcfg \
    root-dir=hedioum logging=yes start-on-boot=yes
```

**Вариант B — импорт tar (реестр не нужен):** на машине с Docker выполните
`docker save ghcr.io/hedioum/pool-tunnel:latest -o hedioum-img.tar`, загрузите на
маршрутизатор (`/tool/fetch`, scp или WinBox), затем:

```rsc
/container/add file=hedioum-img.tar interface=veth1 mounts=hcfg \
    root-dir=hedioum logging=yes start-on-boot=yes
```

Запуск:

```rsc
/container/start [find where root-dir=hedioum]
```

---

## 6. Проверка

```rsc
/container/print detail          ;# status=running, os=linux, arch=…
/log/print where topics~"container"
```

Вы должны увидеть строки вроде:

```
INFO hedioum daemon starting version=v0.10.1 role=iran
INFO SOCKS5 ingress active node=FR addr=172.20.0.2:40001
INFO TUN egress active node=FR iface=hedioum0 addr=10.200.0.1/24 dns=true
INFO pipe established node=FR mimic=tls target=<foreign-ip>:443
```

`pipe established` означает, что замаскированный туннель к зарубежному узлу поднят.

---

## 7. Использование туннеля

- **SOCKS5:** направьте клиентов на **`172.20.0.2:40001`** (адрес, заданный в `--socks-bind`).
  Любое приложение с поддержкой SOCKS5 или outbound Xray/sing-box может им пользоваться. DNS
  разрешается удалённо (без утечек).
- **Маршрутизация всей LAN:** запустите контейнер прозрачного прокси (Xray/sing-box в режиме
  `tproxy`), который использует этот SOCKS, либо задайте прокси на отдельных устройствах.
  (Продвинуто; за рамками руководства.)
- **TUN + DNS (опционально):** контейнер также предоставляет `hedioum0` (10.200.0.1/24) и
  форвардер `:53` **внутри** контейнера. Они полезнее всего, когда контейнер прозрачного
  прокси разделяет сеть контейнера; для простого SOCKS можно опустить `--tun --dns`.

---

## 8. Запуск собственных команд инструмента (test, speedtest, edit)

Образ `FROM scratch` (без shell), поэтому `/container/shell` не даст приглашения — но можно
запустить любую подкоманду Hedioum как **одноразовый контейнер**, разделяющий монтирование конфига:

```rsc
/container/add file=hedioum-img.tar interface=veth1 mounts=hcfg root-dir=hedioum-cmd \
    cmd="test --node FR" logging=yes
/container/start [find where root-dir=hedioum-cmd]
/log/print where topics~"container"          ;# прочитать результат
/container/remove [find where root-dir=hedioum-cmd]
```

Замените `cmd=` на `speedtest --node FR`, `probe --node FR`, `check-ip` и т.д.

**После изменения конфига** (`add-node` / `edit-node`) внутри контейнера нет systemd для
авто-перезапуска демона — **перезапустите контейнер**, чтобы применить:

```rsc
/container/stop  [find where root-dir=hedioum]
/container/start [find where root-dir=hedioum]
```

*(В обычном Docker эквиваленты: `docker exec <name> hedioum-tunnel test --node FR` и
`docker restart <name>` после правки конфига.)*

---

## 9. Устранение неполадок

| Симптом | Решение |
|---|---|
| Меню `/container` неизвестно | Пакет container не установлен или device-mode не `container: yes` (нужен реальный перезапуск питания). |
| Ошибка авторизации при pull `remote-image` | Пакет GHCR всё ещё **приватный** — сделайте публичным или используйте tar (Вариант B). |
| Нет `pipe established` в логе | У контейнера нет интернета — проверьте veth/bridge/NAT (шаг 3) и WAN маршрутизатора. |
| `TUN not started …` в логе | Ожидаемо при ограниченном device-mode/возможностях; SOCKS всё равно работает. На RouterOS у контейнера есть всё для TUN — перепроверьте конфиг. |
| SOCKS недоступен из LAN | Вы привязали к `127.0.0.1`; пересоздайте с `--socks-bind 172.20.0.2` (адрес veth). |
| Образ не распаковывается | Недостаточно места — используйте USB/NVMe или модель с бо́льшим флешем. |

---

## Приложение — публикация образа (для сопровождающих)

Образ автоматически собирается и пушится в GHCR через `.github/workflows/docker-publish.yml`
на каждый тег `v*`. **Первый** push создаёт пакет **приватным**; сделайте его публичным один
раз в `https://github.com/orgs/hedioum/packages/container/pool-tunnel/settings` → *Change
visibility* → *Public*.
