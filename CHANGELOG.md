# Changelog

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
