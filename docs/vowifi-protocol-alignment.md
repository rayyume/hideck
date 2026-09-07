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
| 呼叫、ACK/CANCEL、保持与恢复 | TS 24.229、TS 24.610、RFC 3261 | `voice`、`voice/agent_calls.go` `SwitchCall`、`internal/api/phone.go` | `TestSecondOutboundCallAllowedWhenFirstIsConnected`、hold/CSeq 回归 | **部分实现**：hold/resume 发 TS 24.610 re-INVITE。DEV-27 已落地：最多两路已应答用户通话，`SwitchCall` 切换，第三路 busy；电话 API 有 `/phone/calls/:id/switch`。网关仍可能返回 `ErrHoldNotAligned`，电话页映射为 `ErrHoldUnavailable`。CANCEL/BYE 带 `Reason: RELEASE_CAUSE`。电话页没有 merge/transfer 端点。 |
| SIP session timer | RFC 4028、IR.92 2.2.8 | `voice/session_timer.go` | session timer 与 422/UPDATE/re-INVITE 测试 | 已实现 |
| SMS over IMS | TS 24.341 | `sms`、`imscore/sms*` | RP-DATA、RP-ACK、状态报告和 SMMA 测试 | **部分实现**：MT 在 SMSC 缺失时仍可 Ready；MO 需要 SMSC。TP-SRR 可置位。 |
| IMS 紧急注册、紧急 URI 和 emergency ePDG 选择 | TS 24.229、IR.51 | `imscore/emergency_register.go`、`voice/emergency*`、`startup.SelectEmergencyEPDG` | emergency 单元测试 | 协议构造已实现但默认禁用；普通启动和拨号路径不调用 |
| P-Access-Network-Info `i-wlan-node-id` | TS 24.229 7.2A.4.3 NOTE 3 | `imscore/helpers.go` `GenerateStableWlanNodeID` | `TestGenerateStableWlanNodeID`、`TestGeneratePAccessNetworkInfoPrefersRealBSSID` | **已知偏差**：本形态无 802.11 关联，使用身份派生的本地管理 MAC；若宿主能读到真实 BSSID 则优先使用。不得写成已对齐。该值也是紧急定位标识，合成值不可用于 PSAP 定位。 |
| Cellular-Network-Info | TS 24.229 R.3.1.1A / 7.2.15.1 | `imscore/helpers.go` `GenerateDefaultCellularNetworkInfo` | `TestGenerateDefaultCellularNetworkInfoOmitsSyntheticCell` | VoWiFi 关射频时省略该头；有真实小区时用 `FormatCellularNetworkInfo`。禁止随机 TAC/CellID。 |
| P-CSCF 503 failover | IR.92 2.2.1 / IR.51 4.9 | `imscore/pcscf_recovery.go` `decidePCSCF503Recovery` | `TestDecidePCSCF503RecoveryFollowsTimerB` | 无 Retry-After 时换 P-CSCF 并重新初始注册。Retry-After ≤ Timer B 则等待。Retry-After > Timer B 时仍切换并标记不可用，避免主叫 INVITE 卡死；这是刻意容错，不是规范最小集。 |
| INVITE forking 早期对话 | TS 24.229 5.1.3 / IR.92 2.2 | `imscore/sip_client_legacy.go` `retainClientInviteEarlyDialog` | `TestRetainClientInviteEarlyDialogKeepsForkedTags`、`Test199ClosesOnlyMatchingEarlyDialog` | 不同 To-tag 的 18x 并行保留早期对话，不再 latest-wins。199 只关对应 To-tag。2xx 仍会收拢到确认对话。 |

| 紧急业务 | IR.51 5.3 / TS 24.229 5.1.6 | `emergency_register.go`、`AllowEmergencyRegistration` | emergency 单元测试 | **协议构造就绪，生产禁用**。默认不启紧急 ePDG、不拨 PSAP。PANI 为合成值，无 Geolocation/PIDF-LO，不得声称真实位置上报。 |
| XCAP Ut 补充业务 | IR.92 2.3.2 / TS 24.623 | `xcap`、`runtimehost/ut.go` `UtAccess`、`internal/api/ut.go`、`web/src/views/UtServices.vue` | `TestGetFallsBackToSecondXUIOn404`、`TestUtGetAndPutUseRealXCAPDocument`、`TestUtAccessDoesNotUseIMSWhenXCAPRequired` | XCAP GET/PUT simservs，If-Match，404 换 XUI。独立 `xcap_apn` 时 IMS 起来后启动第二条 SWu，Ut HTTP 经该 PDN 拨号，不回退 IMS。无 XCAP PDN 时页面显示真实错误，不用 USSI。桌面/移动浏览器证据缺失（MCP 不可用），仅有单元测试。 |
| 临时会议 / ECT / 入向 Replaces | IR.92 2.3.3 / 2.3.11、RFC 3891 | `voice/conference.go` `MergeConference`、`voice/ect.go` `TransferConsultative`、`voice/replaces.go` | `TestMergeConferenceInvitesFactoryAndRefersBoth`、`TestTransferConsultativeSendsREFERAndReleases`、`TestInboundReplacesTerminatesMatchedDialog` | **协议已实现**（DEV-27/DEV-26 之后）。会议：向工厂 URI INVITE、REFER 两路、SUBSCRIBE `conference-info`；缺工厂 URI 返回 `ErrConferenceFactoryUnavailable`。咨询式转接：出向 REFER 带 Replaces，等 NOTIFY sipfrag 后 BYE 两路；缺 tag 返回 `ErrECTRequiresReplaces`，不静默盲转。入向 INVITE Replaces 按 call-id/to-tag/from-tag 定位并终止匹配 dialog。Supported 含 `replaces` 与解析路径一致。电话 API 没有 merge/transfer 端点。 |
| 多 PDN / 第二 SWu | IR.51 4.5 / 4.7.4 | `swu.SessionManager.StartSlot`、`runtimecore.StartAdditionalPDNs`、`attachAdditionalPDNs` | `TestSessionManagerOverlappingSlotKeepsDefault`、`TestAdditionalPDNsOnlyWhenXCAPAPNDiffers`、`TestStartAdditionalPDNsWaitsThenStopsFailedSlot` | 默认仍是每设备一条 IMS 会话。配置了不同于 IMS APN 的 `xcap_apn` 时，IMS 起来后在同一 ePDG 上并行第二条 SWu；失败隔离，不拆 IMS。 |
| Annex B 动态下发 | IR.51 Annex B / TS.32 | 静态 preset `annex_b.go` | `TestMergeAnnexBAppliesValidFields` | **不支持 OMA-DM/ANDSF**。四个 Annex B 字段只能静态配置；非法值记入 `AnnexBRejection` 且不改默认。 |
| IKE 重叠重认证 | RFC 7296 2.8.3 | `legacy_lifecycle.go`、`runtime_reauth.go` | `TestOverlappingReauthKeepsOldSessionUntilSuccessorIsUp`、`TestOverlappingReauthOmitsInitialContact` | 主机在旧 SA 仍转发时启动新的 IKE_SA_INIT/IKE_AUTH；新 IKE 与 Child SA 起来后再 Delete 旧 SA；新 AUTH 省略 INITIAL_CONTACT 并携带 FastReauthID。新 runtime 失败则保留旧 SA。 |

## SHOULD / OPTIONAL 已知偏差

- 媒体为隧道内明文 RTP，不是 SRTP（OPTIONAL）。
- SIP 信令无 DSCP 差异化；仅 RTP 标 EF 46（SHOULD）。
- 无 RTCP-XR（OPTIONAL）。
- MO 短信经 `smsSendMu` 串行化。
- 入向 Privacy/TIR 未处理（TS 24.608）。

## IMS 订阅拒绝与注册生命周期

- REGISTER 刷新不重建已开始的 `reg` / `message-summary` 订阅，也不清除拒绝原因。有效订阅沿用对话并按协商有效期刷新；`reg` 初始注册新 Contact 后建立新订阅，见 [TS 24.229 §5.1.1.3](https://www.etsi.org/deliver/etsi_ts/124200_124299/124229/18.10.00_60/ts_124229v181000p.pdf)。短有效期采用半周期刷新，避免提前量大于有效期时立即循环发送。
- MWI 收到 405/489 后不重试，直到公共用户身份注销，见 [TS 24.606 §4.7.2.1](https://www.etsi.org/deliver/etsi_ts/124600_124699/124606/18.00.00_60/ts_124606v180000p.pdf)。记录按设备、IMS 身份隔离，在同一次运行的自动恢复和 IKE 重鉴权间共享；TCP 关闭或 Service 销毁不等于注销。确认注销对应绑定后移除绑定；仍有其他已知有效绑定则保留拒绝记录，最后一个已知绑定注销或到期后才结束该记录。不写入永久运营商黑名单。
- `reg` 初始拒绝不随普通 REGISTER 刷新重试。403、超时或 5xx 不写入 MWI 的“不支持”记录；403 保留原有本地拒绝处理，超时和 5xx 保留原有重试处理。订阅结果带注册上下文校验，旧响应不得覆盖新对话状态。
- 失败原因与跳过原因保留在诊断中；本项不改变 port-s 普通 EOF、2degrees 按需下行、VOXI RST/短信回执 488 或 P-CSCF 退避策略。回归入口：`subscription_lifecycle_test.go`、`subscription_registration_store_test.go`。

### SUBSCRIBE / NOTIFY 状态与计时

- `reg` / `message-summary` 的 NOTIFY 按注册上下文、Call-ID、双方 tag 和 Event usage 匹配；不匹配返回 481，不再修改当前订阅或处理其中的注册/语音信箱状态。其他 Event 的原有分发路径保持不变。待发送事务与实际发送状态分开，允许合法 NOTIFY 先于 SUBSCRIBE 200 到达。首个 NOTIFY 确认订阅对话，后到的其他分支响应不得覆盖它。
- 新的对话内 SUBSCRIBE 在分派前保留递增 CSeq，失败也不复用；同一 SIP 事务重传仍使用原请求。电话 ACK/CANCEL 的序号处理不受影响，见 [RFC 3261 §12.2.1.1](https://www.rfc-editor.org/rfc/rfc3261.html#section-12.2.1.1)。
- `active` / `pending` NOTIFY 的 `expires` 是有效期依据；后到的 200 不覆盖同次尝试已收到的 NOTIFY 有效期，0 不回退为正数。可恢复刷新失败保留原协商到期时间，订阅终止类响应则结束该 usage。
- 初始、刷新和取消订阅在出队发送时启动 `Timer N = 64 × T1`，收到匹配 NOTIFY 后取消。定时器到期只结束订阅并记录错误，不清除 IMS/SMS 就绪状态、不触发 runtime 重建。主动取消后保留最终 NOTIFY 的接收窗口，不自动恢复该订阅。
- `terminated;reason=deactivated/timeout` 可立即以新对话重订阅；`probation/giveup` 和其他原因遵守有效的 `retry-after`；`rejected/noresource/invariant` 不自动重订阅。MWI 405/489 的身份级拒绝记录仍优先，普通 REGISTER 刷新和 EOF 不能绕过。未给出重试时间时沿用原有订阅周期重试间隔，这是本地调度策略，不是 RFC 规定的固定重试时长。
- 订阅本身终止与 reginfo 报告当前 Contact 注销是不同事件；后者在确认属于当前上下文后仍进入原来的 IMS 注册恢复流程。
- 已接纳的 NOTIFY 正文按接收顺序处理，不能因后一条到达而丢弃前一条 reginfo 增量，见 [RFC 3680 §5.2](https://www.rfc-editor.org/rfc/rfc3680.html#section-5.2)。`reg` / MWI 队列独立，旧注册上下文的正文仍被隔离；重复通知和重复回调不重复应用。先尝试回复 SIP，再处理正文；回复写失败仍向调用方返回原错误，但不会丢弃已接纳的状态或阻塞后续通知。回归入口：`subscription_notify_queue_test.go`、`subscription_notify_reply_test.go`。

生命周期依据：[RFC 6665 §4.1](https://www.rfc-editor.org/rfc/rfc6665.html#section-4.1)。回归入口：`subscription_protocol_test.go`、`subscription_timer_protocol_test.go`、`subscription_wire_protocol_test.go`。本次未启用最终 IKE_AUTH 的全局严格认证校验；该兼容选项仍需单独验证，不能据本次订阅修复宣称全部 VoWiFi 协议已对齐。

## Vodafone UK / VOXI 的 P-CSCF 恢复策略

此节是 `vodafone_uk_23415` 预设的经验性兼容策略，不是所有运营商必须采用的协议行为。明确 port-s RST 仍使用 5 秒宽限；RP 报告 488 仍触发换路径。普通 EOF 和其他运营商的恢复分支不因此改变。

- **优先级与重试资格分离**：异常节点保留 30 分钟的 `deprioritizedUntil`，有其他合格节点时优先选择其他节点；该记录本身不禁止重试，也不因候选耗尽被删除。
- **独立的 `retryNotBefore`**：替代路径建立失败使用 RFC 5626 §4.5 随机指数退避，Retry-After 只能延长等待。节点尚未到允许重试时间时，即使没有其他候选也不会提前使用。
- **跨重建保留历史**：新隧道使用新下发的 P-CSCF 列表，同时保留重试时间与连续失败次数。没有可选节点时等待最早的重试时间，再进入新的连接尝试；不通过清空记录连续重建。替代 runtime 的初始 REGISTER 失败也参与该恢复计数。
- **下行验证与成功注册分开**：REGISTER 成功并不直接表示 VOXI 短信下行已恢复；提前建立 port-s 也不能在 REGISTER 成功前结束恢复或清零失败次数。两项条件满足后才结束本轮恢复。REGISTER 失败的退避和 Retry-After 归属实际尝试的节点，不归属失败处理后选出的下一候选。验证超时不再导致固定禁止选择 30 分钟，而是保留低优先级并调度恢复退避。退避期内，同一退役路径的重复报告不会按短信数量累计恢复失败次数。其他节点的降权历史仍保留，不把历史记录当作下一次普通断开的恢复触发条件。
- **诊断日志**：`IMS P-CSCF configuration from new tunnel` 记录下发的 IPv4/IPv6 列表；`IMS P-CSCF candidates resolved` 记录前后候选与来源；`IMS P-CSCF candidate eligibility`、`IMS P-CSCF recovery preference and retry scheduled` 分别记录选择结果、降权原因及允许重试时间。重新获取可能仍返回相同节点，不保证产生新的 P-CSCF。

回归入口：`registrar_selection_test.go`、`registrar_recovery_completion_test.go`、`pcscf_recovery_test.go`、`port_s_session_test.go`、`runtimecore_test.go`。退避参考：[RFC 5626 §4.5](https://www.rfc-editor.org/rfc/rfc5626.html#section-4.5)。30 分钟偏好与 VOXI 下行验证仍属于实现策略，不应写成协议规定的黑名单期限。

## 验证边界

自动化测试验证消息构造、事务时序、状态迁移和主要失败路径。真实网络还需要分别验证运营商策略、P-CSCF 行为、NAT、IPv4/IPv6、媒体编码和超时参数。只有完成目标运营商的实验室一致性用例后，才能声明通过该运营商认证。
