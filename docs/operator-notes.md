# 运营商实测与预设备注

记录 2026-08-18 会话结论，避免和「能上网」「手机能收短信」混在一起。

## EC25 和短信 / 电话

- 这块现场模组是 `2c7c:0125` Quectel EC25（大疆改身份一类）。QMI WMS、NAS、VOICE 能分配；原生 IMS/IMSA/IMSP 分配失败（`qmi service not supported by hardware`）。
- **上网**：EC25 本职，QMI 拨号即可。别人改模组上网走的是这条，不是 VoLTE。
- **国内卡驻自家网**：短信可以走原版驻网（关软件电话、射频开、QMI/AT）。国内 VoLTE / 公网 VoWiFi 打电话，这块 EC25 做不了可靠方案。代码里 MCC `460`/`461` 会直接拒绝 VoWiFi。
- **手机能收、棒子驻网收不到**：认证手机有 VoLTE/IMS 短信；EC25 关掉软件 IMS 后只剩 QMI。英国卡漫游中国移动时，刚才实测 WMS 发送超时、入站没有 URC。

## giffgaff（234/10）

- 有完整预设 `giffgaff_23410`。WiFi calling（关射频 + 软件 IMS）能 REGISTER 200、`contact_smsip=true`。
- 关射频时发 `INFO`→`85075`：O2 IP-SM-GW 回 **RP-ERROR 69 facility not implemented**。短号 IMS 被网元拒，不是没注册。
- 切成「蜂窝 + on_demand」会空闲拆隧道（`VOWIFI_DESIRED_RECOVER_SKIPPED_CELLULAR_IDLE`），此时既没有 IMS 也没有可靠的 QMI 入站。
- 原版驻网收短信：关 VoWiFi → 射频 Online → QMI 听/发，没有射频抑制、没有蜂窝空闲。原版没有另一套短信协议。
- 2026-08-18 在国内把 giffgaff 收成干净驻网（关 IMS、驻中国移动、不开流量）后：自发 `INFO`/自发短信 QMI 超时；手机发来的短信没有进模组。当时状态不是原版日常「从未开过 IMS 的驻网」。
- CTEUK 不要给 giffgaff 发短信（收费）。CTEUK 只测免费查询（`BAL`→`888`）和打电话。giffgaff 可以自己发 `INFO`。

## 英国沃达丰 / VOXI（234/15）

- VOXI 是 Vodafone Limited 的品牌，和沃达丰 UK 同一张网，IMSI 一般是 `23415…`。
- 原版 vohive / 当时的预设清单里 **没有** `vodafone_uk`，只有荷兰 `vodafone_nl_20404`。
- 现已加入预设 `vodafone_uk_23415`：标准 ePDG `epdg.epc.mnc015.mcc234.pub.3gppnetwork.org`，`device_model=rmx3366`，IKE/ESP 先用英国已通提案加宽列表，REGISTER 带 `smsip`。
- `23415` 的 port-s 恢复策略按 Vodafone UK IMS 网络生效，同时覆盖 Vodafone 和 VOXI：连接被对端明确 RST 后只等待 5 秒；运行超过 2 分钟的连接未恢复时先刷新当前 P-CSCF，REGISTER 成功后再切换一次 P-CSCF。普通 EOF、超时和其他运营商仍走通用 RFC 5626 恢复。
- 切换后的 REGISTER 200 只代表注册成功；新 port-s 建立或收到有效下行 SIP 请求后才算恢复完成。验证超时的 P-CSCF 会降权 30 分钟并触发 runtime 重建；共享降权状态确保每轮只尝试一次候选，重建 IMS/ePDG 后也不会提前解除。
- 国家代理仍按 MCC `234` 走 GB 规则。VOXI 官方写明漫游不支持 WiFi calling，人在国内需走代理。
- 真卡验收前：IKE/ESP 仍可能要按日志改。余额查询未做自动短码（走 App）。

## Lebara UK NextGen（234/87）

- 分享 eSIM，LPA 显示名常见 `Lebara UK`，有的列表会写成 `0 Lebara UK`（前面的数字是序号，不是 PLMN）。认卡靠 IMSI `23487…`，不要靠卡名里的 `0`。
- 不是旧的 Vodafone-hosted Lebara（234/15 + GID 90）。234/15 仍走 `vodafone_uk_23415`。
- 自有 MNC `87`，宿主网仍是 Vodafone UK。预设 `lebara_uk_23487`：标准 ePDG `epdg.epc.mnc087.mcc234.pub.3gppnetwork.org`，`device_model=rmx3366`，IKE/ESP 先用英国已通提案加宽列表，REGISTER 带 `smsip`。
- 双 IMSI：一驻国内 460 常会切到 `20404`。切过去之后不要当荷兰沃达丰去连；英国 WFC 不可用。分享卡运行时锁射频、拒绝开网络/关飞行/蜂窝模式，避免踩这条路。
- `20404` 不是 Profile 损坏。同一 ICCID 做一次 Disable→Enable（或经停车 Profile 中转）可以把活动身份拉回 `23487`，不必删卡重写。VoHive 在活 IMSI 变成 `20404` 时自动清污，连续 3 次读到 `23487` 后再开 VoWiFi。射频锁不变。模组重启本身清不掉。
- `20404` 且没有 Lebara 证据时，仍是 `vodafone_nl_20404`。
- 国家代理按活 IMSI 的 MCC：`23487` 走 GB。人在国内必须走英国代理。
- 真卡验收前：IKE/ESP 仍可能要按日志改。余额查询未做自动短码（走 Lebara App）。

## 德国 O2（262/03、262/07）

- 预设已有：`O2_de_26203`、`O2_de_26207_alias`。ePDG 用 3GPP 名 `epdg.epc.mnc003.mcc262.pub.3gppnetwork.org` / `mnc007`。
- IKE 先出 `aes256-sha256-prfsha1-modp2048`（iPhone 侧已通过的提案），设备身份 `iphone15,4`。
- 国家代理按 MCC `262` 走 **DE**。人在国内要配德国前置，和 GB 同一套规则表。
- 余额查询码按套餐不同（`*105#` / `*101#`），命令中心标成不支持自动查。

## 荷兰 Vodafone（204/04）

- 预设已有：`vodafone_nl_20404`。ePDG `epdg.epc.mnc004.mcc204.pub.3gppnetwork.org`。
- **不要**把 Lebara UK 切到 `20404` 的分享卡当成这家。光秃 204/04 且没有 Lebara 证据时才走这条。
- 国家代理按 MCC `204` 走 **NL**。
- 余额短码：`STATUS` → `4000`。

## 菲律宾 DITO（515/66）

- 新增预设 `dito_51566`。标准 ePDG `epdg.epc.mnc066.mcc515.pub.3gppnetwork.org`，`device_model=rmx3366`，IKE/ESP 用宽列表，REGISTER 带 `smsip`，403/500 允许 fallback。
- DITO 公开页只保证 VoLTE；VoWiFi 按机型开通，白名单不公开。真卡 IKE/REGISTER 仍可能要按日志改提案或 `device_model`。
- 国家代理按 MCC `515` 走 **PH**。人在国内需配菲律宾前置。
- 余额走 DITO App / `*143#` 菜单，没有可审计的统一免费短信码。

## 市面常见 eSIM（按 home PLMN，不是品牌名）

Airalo / Holafly / Nomad 一类旅行流量卡通常没有 IMS 开户。即使宿主 IMSI 落在下面这些 PLMN，REGISTER 仍会被拒。下面加的是带号码、官方写过 WiFi calling 的宿主网。匹配键是 SIM 的 home MCC+MNC，不是包装上的品牌。

IKE/ESP 先用宽列表，`device_model=rmx3366`，REGISTER 带 `smsip`，403/500 允许 fallback。真卡验收前仍可能要按日志改提案或设备身份。

| 运营商 | PLMN | 预设 | 国家前置 |
| --- | --- | --- | --- |
| 1GLOBAL / 原 Truphone | 234/25 | `oneglobal_23425` | GB |
| Lycamobile UK | 234/26 | `lycamobile_uk_23426` | GB |
| EE UK | 234/30、234/31、234/32 | `ee_uk_23430` / `23431` / `23432` | GB |
| Orange France | 208/01 | `orange_fr_20801` | FR |
| Telekom Germany | 262/01 | `telekom_de_26201` | DE |
| Vodafone Germany | 262/02 | `vodafone_de_26202` | DE |
| 中国移动香港 CMHK | 454/12、454/13 | `cmhk_45412` / `cmhk_45413` | HK |
| 乌龟卡 / TravelSIM / Elisa | 248/02 | `elisa_ee_24802` | EE |

- EE 的 234/33 仍是 `CTEUK_23433`，不要当成 EE 官方卡。
- Lycamobile UK 是 234/26。美国 Lyca 走 AT&T 的 `310/410`，两套不要混。
- 1GLOBAL 若 DNS 不到标准 3GPP ePDG，再按 IKE 日志改自定义主机名。
- 人在国内必须配对应国家前置：GB / FR / DE / HK / JP / EE / NL / MY / PH / TH / NG。UI 里还没有该国规则时，WFC 连不上 ePDG。
- 余额查询资料不足，命令中心标成不支持自动查。

香港 MCC `454` 的 PANI 国家码现在是 **HK**（以前掉进默认 `XX`）。CSL / 3HK / CMHK 都会用到。

## 日本（440）官方 Wi-Fi通話

三大运营商官方套餐和线上品牌 eSIM 有 3GPP Wi-Fi通話。匹配仍按 home PLMN。IIJmio / mineo 一类 MVNO、以及日本旅行流量卡，通常没有 IMS 开户。

| 运营商 | PLMN | 预设 | 常见 eSIM |
| --- | --- | --- | --- |
| NTT Docomo | 440/10 | `docomo_44010` | ドコモ官方 eSIM |
| SoftBank / LINEMO | 440/20 | `softbank_44020` | SoftBank、LINEMO |
| Y!mobile | 440/00 | `ymobile_44000` | 旧 eAccess IMSI；新卡常见 440/20 |
| KDDI au / povo / UQ | 440/51 | `kddi_44051` | au、povo、UQ VoLTE 卡 |

- 没加 440/50（非 VoLTE）、440/52（IoT）、440/53（乐天漫游 KDDI）、Starlink / JPN-ROAM 号段。
- **乐天 440/11 不加**。语音走 Rakuten Link，不是 ePDG VoWiFi。
- 国家代理按 MCC `440`/`441` 走 **JP**。人在国内需配日本前置。
- ePDG 先用标准 3GPP 名。Y!mobile `440/00` 若 DNS 不到，再按日志看要不要改 SoftBank `mnc020`。
- 余额走各家 App，命令中心标成不支持自动查。

## 乌龟卡 / 爱沙尼亚 Elisa（248/02）

市面「乌龟卡」是 esim.gg / Nekoko Telecom，TravelSIM 的代理，+372 号码，宿主网是 **Elisa Eesti 248/02**，不是英国卡，也不是中国移动。

- 新增预设 `elisa_ee_24802`。标准 ePDG `epdg.epc.mnc002.mcc248.pub.3gppnetwork.org`，`device_model=rmx3366`，IKE/ESP 用宽列表，REGISTER 带 `smsip`，403/500 允许 fallback。
- Elisa 官方页写明商务套餐有 VoWiFi。TravelSIM/乌龟卡是 MVNO，IMS 开户不保证；没开户时 IKE/REGISTER 会被拒。
- 国家代理按 MCC `248` 走 **EE**。人在国内需配爱沙尼亚前置。
- 248/02 也是 Elisa 官方卡。余额短码不能自动选：乌龟卡/TravelSIM 文档有 `*146*099#`，Elisa 官方走账户页。

## 本轮核对过的 eSIM

| 品牌 | 结论 | 原因 |
| --- | --- | --- |
| Lycamobile UK | 已有 | `lycamobile_uk_23426` |
| 新西兰 2degrees | 已有 | `2degrees_nz_53024` |
| CMLink UK | 不另加 | 宿主 EE `234/30`，客服多次说不支持 WFC；IMSI 已走 EE 预设 |
| Asda Mobile | 不另加 | 官方有 WFC，宿主 VF `234/15`，且没有 eSIM |
| Saily | 不加 | 流量卡 + App 虚拟美号，不是 ePDG VoWiFi |
| 克罗地亚 A1 | 不加 | A1 HR 没有 VoWiFi；旅游 eSIM 偏流量 |
| 格鲁吉亚 Cellfie | 不加 | 只公开 VoLTE，没有 VoWiFi |
| 摩尔多瓦 eSIM | 不加 | 旅行卡多是流量；Orange/Moldcell 没有可审计的 VoWiFi |
| Hotlink MY | 新增 | 官方 VoWiFi，`hotlink_my_50212`（502/12） |
| AIS TH | 新增 | 官方 Wi-Fi Calling，`ais_th_52001` / `52003` |
| Smart PH | 新增 | 已商用 VoWiFi，`smart_ph_51503` |
| GlobeOne / Globe | 新增 | 官方 VoWiFi，`globe_ph_51502` |
| Simyo NL | 新增 | 官方 wifi bellen，宿主 KPN `kpn_nl_20408` |
| MTN 尼日利亚 | 新增 | 有 eSIM 和 VoWiFi 报道，`mtn_ng_62130` |

Hotlink 官方写明**漫游时 VoWiFi 不可用**。人在国内仍先走 MY 前置试 ePDG；账户没开国际 WFC 时会被拒。

尼日利亚 MCC `621` 的国家码是 **NG**。

## 英国侧已有预设

| 运营商 | PLMN | 预设 |
| --- | --- | --- |
| giffgaff | 234/10 | `giffgaff_23410` |
| Vodafone UK / VOXI | 234/15 | `vodafone_uk_23415` |
| Three UK | 234/20 | `three_uk_234020` |
| 1GLOBAL / Truphone | 234/25 | `oneglobal_23425` |
| Lycamobile UK | 234/26 | `lycamobile_uk_23426` |
| EE UK | 234/30、31、32 | `ee_uk_23430` 及别名 |
| CTExcel | 234/33 | `CTEUK_23433` |
| Lebara UK NextGen | 234/87 | `lebara_uk_23487` |
