# Native VoLTE（模组 IMS）

`phone_mode=volte` 走模组原生 IMS / QMI VOICE，不建 ePDG/SWu，也不走软件 IMS。
`phone_mode=cellular` 仍是蜂窝数据上的软件 IMS；`wifi` 仍是 VoWiFi。

卡策略里的 `vowifi_enabled` 仍是“电话开”开关（库字段不改名）。蜂窝软件电话和原生 VoLTE 开电话时驻网、不强制飞行；只有 WiFi calling 会占射频。`GET` 设备状态里的 `native_volte` 是 `volte.Status`。

插卡前协议已经按下面顺序写完。插卡后只验证，不再改协议骨架。

## 启用顺序

1. 停软件 IMS
2. `AT+COPS?` 读驻网 PLMN
3. 只在 MBN 清单里有**唯一**画像时才 `QMBNCFG select/activate`（不猜 `ROW_Generic`）
4. 备份并写入 IMS / VoLTE / UAC（VID/PID 不变）；失败阶段不会声称已恢复
5. 等 `AT+CEREG` 注册到 LTE
6. 找到 APN 含 `ims` 的 PDP，必要时 `AT+CGACT=1,<cid>`
7. 按需分配 QMI IMS/IMSA/IMSP，订阅 IMSA 指示
8. 轮询 IMSA，直到 `registered` 或超时（超时保持 `registering`，不当成成功）

国内 PLMN 映射（必须出现在模块清单里）：

| PLMN | MBN |
|---|---|
| 460-00/02/04/07/08 | `Volte_OpenMkt-Commercial-CMCC` |
| 460-03/05/11 | `VoLTE_OPNMKT_CT` |
| 460-01/06/09 | `CU-VoLTE` |
| 460-15 广电 | 清单里的广电画像（`CBN-VoLTE` 等）；没有则用移动 `Volte_OpenMkt-Commercial-CMCC`（共建共享） |

234/xx 等没有唯一画像时启用失败，不会猜画像，也不会回退软件 IMS。

## 通话

- 仅 `IMS registered` 才允许拨号
- QMI VOICE 状态投影到现有 `/phone` 事件
- `CallSnapshot.ClientSDP` 是本机 PCMU 8 kHz 端点，网页 `MediaSession` 在接通后 Attach
- `Mode` 为 GSM/UMTS 时记 `cs_fallback`，不当成 VoLTE 接通
- 无 UAC 声卡时媒体仍给 PCMU 端点，上行是静音，录音错误说明缺声卡

## 回滚

`RestoreNativeVoLTE` 只还原 IMEI 且 VID/PID 匹配的备份。备份在 `data/volte-provision/`，文件名是 IMEI 的 SHA-256，权限 0600。不要按 ttyUSB 口写。

UAC 写入后通常要重枚举。本实现**不会**自动 `AT+CFUN=1,1`。

## 插卡后要看的

- `GET /api/phone/devices` 里 `native_volte.phase`、`plmn`、`mbn_name`、`lte_registered`、`ims_pdn_active`、`ims_registered`
- 模块 `AT+QCFG="ims"?`、`AT+QMBNCFG="list"`、`AT+CEREG?`、`AT+CGACT?`
- 通话接通时 QMI call mode 是否 LTE/WLAN
