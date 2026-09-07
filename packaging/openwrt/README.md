# OpenWrt

GitHub 上的 `hideck_*_linux_*` 是 glibc + UPX，OpenWrt（musl）不能跑。要用 **`hideck_*_openwrt_*`**：musl 静态链接，不压 UPX。

二进制大约 77M，小闪存路由请用 extroot。不提供 mips 包。

## 安装

```sh
opkg update
opkg install curl libqmi kmod-usb-net-qmi-wwan kmod-usb-serial-option

curl -fsSL https://raw.githubusercontent.com/yibaiba/hideck/main/deploy-binary.sh | sh
```

脚本在 OpenWrt 上会拉 `openwrt_amd64` / `openwrt_arm64` / `openwrt_armv7`，装到 `/usr/bin/hideck`，配置 `/etc/hideck/config.yaml`，数据 `/var/lib/hideck`，并用 procd 拉起。

手工下载：

| 文件 | 板子 |
| --- | --- |
| `hideck_vX.Y.Z_openwrt_amd64` | x86_64 |
| `hideck_vX.Y.Z_openwrt_arm64` | aarch64（树莓派、多数 ARM 网关） |
| `hideck_vX.Y.Z_openwrt_armv7` | 32 位 ARM |

```sh
cp hideck_vX.Y.Z_openwrt_arm64 /usr/bin/hideck
chmod +x /usr/bin/hideck
cp packaging/openwrt/hideck/files/config.yaml /etc/hideck/config.yaml
cp packaging/openwrt/hideck/files/hideck.init /etc/init.d/hideck
chmod +x /etc/init.d/hideck
/etc/init.d/hideck enable
/etc/init.d/hideck start
```

`qmi-proxy` 默认 `/usr/libexec/qmi-proxy`（`libqmi` 包）。把 `system.openwrt_dynamic_interfaces` 保持 `true`。

## 用 OpenWrt SDK 打 ipk

把 `packaging/openwrt/hideck` 拷进 SDK 的 `package/hideck`，`PKG_VERSION` 对齐 GitHub tag 后 `make package/hideck/compile`。
