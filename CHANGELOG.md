# Changelog

## 2.1.8 - 2026-09-03

### 打包

- Docker 镜像改回 UPX。UPX 5 的 stub 在 Alpine/gcompat 上会报 `Not a valid dynamic program`，发版底包改为 Debian bookworm（glibc），压缩包可以直接 exec。

## 2.1.7 - 2026-09-03

### VoWiFi / IMS

- 已注册后再发 REGISTER 被 503（2degrees）时，不再当成换传输、也不再拆掉还活着的 TCP。现有绑定保住，port-s 恢复停掉。

### 打包

- Docker 镜像改放未压缩二进制。2.1.6 把 UPX 包塞进 Alpine，容器会报 `Not a valid dynamic program` 一直重启。

## 2.1.6 - 2026-09-03

### VoWiFi / IMS

- port-s 不再发 CRLF。关掉后等 30s，对端不重连就补一次 REGISTER；这次 REGISTER 被拒（如 2degrees 503）就不再恢复。
- outbound 跟进 REGISTER 如果把信令流拆掉，这次注册算失败，马上重来，不再空等保活超时。
- 进程退出不再发 `Expires=0` / `Contact:*` 注销，避免把 IP-SM-GW 一起拆掉、重启后收不到短信。

## 2.1.5 - 2026-09-03

### VoWiFi / IMS

- P-CSCF 关掉 port-s 不再当成注册失败，也不再立刻重 REGISTER。监听还在，等对端重新连上来（#9）。
- RP-ACK 被拒时发 RP-SMMA 问 SMSC 队列；日志会标出哪条短信堵在队头、堵了多久。

## 2.1.4 - 2026-09-02

### 界面

- 默认主题改为 navy-light / navy-night（奶油画布、海军字、强调蓝）。原来的暗色主题仍可在设置里选「经典」。
- 卡片 24px、输入 16px、侧栏选中 12px、主按钮胶囊形。登录仍是左品牌栏 / 右表单，页面结构没改（#5）。

## 2.1.3 - 2026-09-02

### VoWiFi / IMS

- 入向短信按 RFC 3428 回 200。RP-ACK 按 TS 24.341：Request-URI 用 PAI，必带 In-Reply-To 和 binary CTE；488 不再换 URI 重试。
- port-s 入向 TCP 补 30s 套接字保活和 RFC 5626 双 CRLF；对端 RST 后先等重连，不立刻重注册把 Contact 换掉。
- 双栈只在没有可用 IPv6 P-CSCF、或地址不是单播时才跳过 IPv6 Contact。
- CFG_REPLY 能拆开 16390 里拼在一起的 IPv6 P-CSCF。
- 只在 `Require: outbound`、`Path;ob` 或 Contact 带回 `reg-id` 时才补 outbound REGISTER。2degrees 等只广告 `Supported: outbound` 的不再多发一次被 503 拆掉会话（#6 / #8）。
- REGISTER 超时会重试；IPsec SA 被重发时丢掉这次尝试；200 里还有旧 Contact 时先保住当前绑定。

### 修复

- 不再把 `go.work` / `go.work.sum` 纳入版本库。

## 2.1.2 - 2026-09-01

### 修复

- Docker 运行时镜像带上 `libqmi` / `qmi-proxy`，容器里可以走 QMI Proxy，不必再回退直接打开 `/dev/cdc-wdm*`。

## 2.1.1 - 2026-09-01

### eSIM

- Lebara UK 分享卡连过国内网变成 `204/04` 后，不必再删卡重写。同一 ICCID 做停用/启用，或经停车 Profile 中转，连续读到英国 `23487` 后再开 VoWiFi。射频锁不变，也不会把错误身份送去英国 ePDG。

### 原生 VoLTE

- 支持保持/恢复。
- 大疆/佰旺等 USB 声卡不能开时跳过，避免把 QMI 打挂；信令仍可走 VoLTE。
- 中国卡继续走原生 IMS，不走软件 WiFi calling。
- 启动失败会重试；LTE 信号优先显示 RSRP。

### VoWiFi

- 新增一批旅行/常用卡预设：Hotlink、AIS、Smart、Globe、KPN、MTN 等。
- 对端挂断后停止来电振铃。
- IMS REGISTER、短信 RP-ACK、IKE 重鉴权，以及会议 / 转接 / 呼叫等待等协议对齐。

### 修复

- WebRTC 在 NAT 后发布公网 ICE 候选。
- P-CSCF 运行时路径。
- Path 带 `ob` 时 REGISTER 尽快带 `reg-id`。

## 2.1.0 - 2026-08-28

### 原生 VoLTE

- 新增 `phone_mode=volte`：走模组 IMS / QMI VOICE，不建 ePDG，也不走软件 IMS。
- 国内 PLMN 有唯一 MBN 画像时才选择（移动 / 联通 / 电信 / 广电 460-15）；英国等没有唯一画像时不会乱选。
- 通话音频走模组 UAC / ALSA，网页侧仍是 PCMU；接通后可从悬浮栏打开 DTMF。
- 原生来电会通知渠道；挂断会发出 `call_ended`。不会自动 `AT+CFUN` 重启模组。

### VoWiFi 协议

- SMS over IP 按 TS 24.341：Request-URI 走 SMSC PSI，RP-ACK 带 In-Reply-To，对不上回 488；支持 RP-SMMA 和无号码 SMS（Contact 声明 `+g.3gpp.smsip-msisdn-less`）。
- 保活按 RFC 5626 / RFC 6223：TCP 发 `\r\n\r\n`，UDP 发 STUN Binding；只有 `keep=N` 或 `Require: outbound` 才等 pong。
- 语音 SDP 按 IR.92：AMR-WB bandwidth-efficient 在前，octet-align 另开 PT，带 telephone-event 和 `ptime:20`。
- 紧急呼叫只打包不主叫：REGISTER Contact `;sos`、`INVITE urn:service:sos`、`Priority: emergency`，以及配置里的 `sos.epdg...` FQDN。日常 IKE 仍走普通 ePDG，默认不打 PSAP。
- hideck 停止时会回收自己拉起的 qmi-proxy。

### 电话与界面

- 电话页可在模式选择下直接打开 WiFi calling。
- 仪表盘没有 serving operator 时显示 SIM 归属运营商。
- 设备头上的 IMEI / ICCID 可模糊显示。

### eSIM

- 服务端解析激活二维码图片和 PDF。
- 支持从市场激活码 / 拖入的 QR 安装 profile。

### 修复

- 来电/去电结束通知改为中文，与来电通知一致。
- 忽略过期 SIM 快照和 UIM IMSI 读取错误。
- QMI 握手失败时回退 AT 读 IMEI。
- 若干原生通话状态机、声卡枚举和挂断后幽灵振铃问题。
