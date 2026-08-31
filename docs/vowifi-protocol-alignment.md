# VoWiFi 协议对齐与验证范围

本文记录 HiDeck VoWiFi 实现与规范条款、代码入口和自动化测试的对应关系。它用于工程追踪，不替代 GCF、PTCRB 或运营商实验室认证。

## 规范基线

- 本次 SIP/SDP 对齐基线固定为 GSMA IR.51 v10.0 与 GSMA IR.92 v21.0；后续版本升级必须同步更新本表和对应回归测试。
- GSMA IR.51：IMS over untrusted Wi-Fi，VoWiFi IMS 功能继承 IR.92 要求。
- GSMA IR.92：IMS voice and SMS profile，包含 SIP precondition、会话定时器、语音媒体和 SMS over IP 要求。
- 3GPP TS 24.302：EPC via non-3GPP access。
- 3GPP TS 24.229：IMS SIP/SDP Stage 3。
- 3GPP TS 24.341：SMS over IP Stage 3。
- 3GPP TS 33.402、TS 33.203：非 3GPP 接入和 IMS 接入安全。
- IETF RFC 3261、3262、3311、3312、4028：SIP、100rel/PRACK、UPDATE、precondition 和 session timer。

具体部署仍以 SIM 归属运营商的 IMS 配置和启用策略为准；规范允许运营商关闭部分可选能力。

## 追踪矩阵

| 能力 | 规范依据 | 实现入口 | 自动化证据 | 状态 |
| --- | --- | --- | --- | --- |
| ePDG 发现与非 3GPP 接入 | TS 24.302、IR.51 | `startup.SelectEmergencyEPDG`、preset FQDN、A/AAAA | 对应包测试 | **部分实现**：preset 或 override 的 FQDN 加 A/AAAA，没有 DNS NAPTR 动态发现。`internal/vowifi/dns` 只服务 IMS Registrar。AAAA-only ePDG 加 IPv4-only SOCKS5 代理不可达，属已知约束。 |
| IKEv2、EAP-AKA/AKA'、IPsec SA | TS 33.402、RFC 7296、RFC 4187/5448 | `ikev2`、`ipsec`、`swu` | 对应包测试 | 已实现 |
| IMS AKA 注册与安全协商 | TS 24.229、TS 33.203 | `imscore/register*`、`sec_agree*` | 注册、AKA、UDP/TCP/IPsec 回归测试 | 已实现 |
| 可靠临时响应 | RFC 3262、TS 24.229 | `voice/outbound_provisional.go` | `TestAgentPRACKsReliableProvisionalBeforeFinalInvite` | 已实现 |
| VoWiFi SIP precondition 状态更新 | IR.51 2.4.1、IR.92 2.4.1、RFC 3312 | `voice/outbound_precondition_update.go` | `TestAgentSendsPreconditionUpdateAfterReliableProvisional`、`TestLocalClientOwnsReliableProvisionalPRACK` | 已实现 |
| precondition UPDATE 编码收敛 | IR.92 2.4.1 | `voice/sdp_codec_selection.go` | `TestBuildPreconditionStatusSDPUsesSelectedCodecsAndQoS` | 已实现 |
| SIP OPTIONS 与 Allow 能力声明 | RFC 3261 11、20.5 | `imscore/inbound_options.go`、`transport_runtime.go` | `TestBuildInboundOPTIONSResponseAdvertisesCapabilities` | 已实现 |
| 呼叫、ACK/CANCEL、保持与恢复 | TS 24.229、TS 24.610、RFC 3261 | `voice` | voice dialog、hold、CSeq 回归测试 | **部分实现**：hold re-INVITE 已有，电话页仍映射 `ErrHoldNotAligned`；多路并发未完成（DEV-27）。CANCEL/BYE 已带 `Reason: RELEASE_CAUSE`。 |
| SIP session timer | RFC 4028、IR.92 2.2.8 | `voice/session_timer.go` | session timer 与 422/UPDATE/re-INVITE 测试 | 已实现 |
| SMS over IMS | TS 24.341 | `sms`、`imscore/sms*` | RP-DATA、RP-ACK、状态报告和 SMMA 测试 | **部分实现**：MT 在 SMSC 缺失时仍可 Ready；MO 需要 SMSC。TP-SRR 可置位。XCAP Ut 不是 USSI。 |
| IMS 紧急注册、紧急 URI 和 emergency ePDG 选择 | TS 24.229、IR.51 | `imscore/emergency_register.go`、`voice/emergency*`、`startup.SelectEmergencyEPDG` | emergency 单元测试 | 协议构造已实现但默认禁用；普通启动和拨号路径不调用 |
| P-Access-Network-Info `i-wlan-node-id` | TS 24.229 7.2A.4.3 NOTE 3 | `imscore/helpers.go` `GenerateStableWlanNodeID` | `TestGenerateStableWlanNodeID`、`TestGeneratePAccessNetworkInfoPrefersRealBSSID` | **已知偏差**：本形态无 802.11 关联，使用身份派生的本地管理 MAC；若宿主能读到真实 BSSID 则优先使用。不得写成已对齐。该值也是紧急定位标识，合成值不可用于 PSAP 定位。 |
| Cellular-Network-Info | TS 24.229 R.3.1.1A / 7.2.15.1 | `imscore/helpers.go` `GenerateDefaultCellularNetworkInfo` | `TestGenerateDefaultCellularNetworkInfoOmitsSyntheticCell` | VoWiFi 关射频时省略该头；有真实小区时用 `FormatCellularNetworkInfo`。禁止随机 TAC/CellID。 |
| P-CSCF 503 failover | IR.92 2.2.1 / IR.51 4.9 | `imscore/pcscf_recovery.go` `decidePCSCF503Recovery` | `TestDecidePCSCF503RecoveryFollowsTimerB` | 无 Retry-After 时换 P-CSCF 并重新初始注册。Retry-After ≤ Timer B 则等待。Retry-After > Timer B 时仍切换并标记不可用，避免主叫 INVITE 卡死；这是刻意容错，不是规范最小集。 |
| INVITE forking 早期对话 | TS 24.229 5.1.3 / IR.92 2.2 | `imscore/sip_client_legacy.go` `retainClientInviteEarlyDialog` | `TestRetainClientInviteEarlyDialogKeepsForkedTags`、`Test199ClosesOnlyMatchingEarlyDialog` | 不同 To-tag 的 18x 并行保留早期对话，不再 latest-wins。199 只关对应 To-tag。2xx 仍会收拢到确认对话。 |

| 紧急业务 | IR.51 5.3 / TS 24.229 5.1.6 | `emergency_register.go`、`AllowEmergencyRegistration` | emergency 单元测试 | **协议构造就绪，生产禁用**。默认不启紧急 ePDG、不拨 PSAP。PANI 为合成值，无 Geolocation/PIDF-LO，不得声称真实位置上报。 |
| XCAP Ut 补充业务 | IR.92 2.3.2 / TS 24.623 | `xcap`、`internal/api/ut.go`、`web/src/views/UtServices.vue` | `TestGetFallsBackToSecondXUIOn404`、`TestUtGetAndPutUseRealXCAPDocument` | XCAP GET/PUT simservs，If-Match，404 换 XUI。无 XCAP PDN 时页面显示真实错误，不用 USSI。 |
| 临时会议 / ECT | IR.92 2.3.3 / 2.3.11 | `voice/refer.go` | — | **未实现**。依赖多路 Agent（DEV-27）与入向 Replaces（DEV-26）。Supported 含 `replaces` 但入向 Replaces 尚未解析。 |
| 多 PDN / 第二 SWu | IR.51 4.5 / 4.7.4 | `swu.SessionManager.StartSlot`、`policy.AdditionalPDNs` | `TestSessionManagerOverlappingSlotKeepsDefault`、`TestAdditionalPDNsOnlyWhenXCAPAPNDiffers` | 默认仍是每设备一条 IMS 会话。配置了不同于 IMS APN 的 `xcap_apn` 时可在同一 ePDG 上并行第二条 SWu。 |
| Annex B 动态下发 | IR.51 Annex B / TS.32 | 静态 preset `annex_b.go` | `TestMergeAnnexBAppliesValidFields` | **不支持 OMA-DM/ANDSF**。四个 Annex B 字段只能静态配置；非法值记入 `AnnexBRejection` 且不改默认。 |
| IKE 重叠重认证 | RFC 7296 2.8.3 | `legacy_lifecycle.go`、`runtime_reauth.go` | `TestOverlappingReauthKeepsOldSessionUntilSuccessorIsUp`、`TestOverlappingReauthOmitsInitialContact` | 主机在旧 SA 仍转发时启动新的 IKE_SA_INIT/IKE_AUTH；新 IKE 与 Child SA 起来后再 Delete 旧 SA；新 AUTH 省略 INITIAL_CONTACT 并携带 FastReauthID。新 runtime 失败则保留旧 SA。 |

## SHOULD / OPTIONAL 已知偏差

- 媒体为隧道内明文 RTP，不是 SRTP（OPTIONAL）。
- SIP 信令无 DSCP 差异化；仅 RTP 标 EF 46（SHOULD）。
- 无 RTCP-XR（OPTIONAL）。
- MO 短信经 `smsSendMu` 串行化。
- 入向 Privacy/TIR 未处理（TS 24.608）。

## 验证边界

自动化测试验证消息构造、事务时序、状态迁移和主要失败路径。真实网络还需要分别验证运营商策略、P-CSCF 行为、NAT、IPv4/IPv6、媒体编码和超时参数。只有完成目标运营商的实验室一致性用例后，才能声明通过该运营商认证。
