import { t } from '../i18n'
import type { DeviceOverviewItem, NativeVoLTEStatus } from '../types/api'
import {
  createDashboardStages,
  formatDashboardSignal,
  hasDashboardSignal
} from './dashboardPresentation'
import { displaySignalDbm } from './signalPresentation'
import { isNativeVoLTEMode, phoneModeCampsOnCell } from './phoneMode'

export type OverviewConnectionKind = 'wifi' | 'volte' | 'cellular'

export type OverviewConnectionStage = Readonly<{
  key: string
  ready: boolean | undefined
}>

export type OverviewConnectionMetric = Readonly<{
  label: string
  value: string
  hint: string
}>

export type OverviewConnectionPresentation = Readonly<{
  kind: OverviewConnectionKind
  eyebrow: string
  title: string
  detail: string
  tone: 'is-idle' | 'is-pending' | 'is-ready' | 'is-failed'
  pathIsFlowing: boolean
  stages: readonly OverviewConnectionStage[]
  metrics: readonly OverviewConnectionMetric[]
}>



export function createOverviewConnectionPresentation(
  device: DeviceOverviewItem | null
): OverviewConnectionPresentation {
  if (device && isNativeVoLTEMode(device.phone_mode)) {
    return createVoLTEPresentation(device)
  }
  return createVoWiFiOrCellularPresentation(device)
}

function createVoLTEPresentation(device: DeviceOverviewItem): OverviewConnectionPresentation {
  const status = device.native_volte
  const stages = createVoLTEStages(device, status)
  const state = volteServiceState(device, status)
  return Object.freeze({
    kind: 'volte',
    eyebrow: 'VOLTE',
    title: state.title,
    detail: state.detail,
    tone: state.tone,
    pathIsFlowing: device.healthy === true && volteRegistered(status) && state.tone !== 'is-failed',
    stages,
    metrics: Object.freeze([
      metric(t('overview.access'), 'VoLTE'),
      metric('PLMN', status?.plmn),
      metric('MBN', status?.mbn_name),
      metric('IMS PDN', status ? (status.ims_pdn_active ? t('overview.imsPdnOn') : t('overview.imsPdnOff')) : t('common.unavailable')),
      metric(t('devices.protocol'), device.backend_mode?.toUpperCase()),
      metric(t('devices.iface'), device.interface)
    ])
  })
}

function createVoWiFiOrCellularPresentation(
  device: DeviceOverviewItem | null
): OverviewConnectionPresentation {
  const stages = createDashboardStages(device?.vowifi_runtime)
  const hasFailedStage = stages.some((stage) => stage.ready === false)
  const hasReadyStage = stages.some((stage) => stage.ready === true)
  const allStagesReady = stages.every((stage) => stage.ready === true)
  const runtimeReason = device?.vowifi_runtime?.sms_ready_reason || device?.vowifi_runtime?.last_reason || ''
  const protocol = metric(t('devices.protocol'), device?.backend_mode?.toUpperCase())
  const deviceInterface = metric(t('devices.iface'), device?.interface)

  if (!device?.vowifi_enabled) {
    if (!phoneModeCampsOnCell(device?.phone_mode)) {
      return Object.freeze({
        kind: 'wifi',
        eyebrow: 'WI-FI CALLING',
        title: t('overview.wifiOff'),
        detail: t('overview.wifiOffHint'),
        tone: 'is-idle',
        pathIsFlowing: false,
        stages,
        metrics: Object.freeze([
          metric(t('overview.access'), 'Wi-Fi Calling', 'Wi-Fi Calling'),
          protocol,
          deviceInterface
        ])
      })
    }
    const signal = displaySignalDbm(device?.modem?.signal_dbm, device?.modem?.signal_rsrp)
    return Object.freeze({
      kind: 'cellular',
      eyebrow: 'CELLULAR',
      title: t('overview.softphoneOff'),
      detail: t('overview.softphoneOffHint'),
      tone: 'is-idle',
      pathIsFlowing: false,
      stages,
      metrics: Object.freeze([
        {
          label: t('overview.cellularSignal'),
          value: formatDashboardSignal(device?.modem?.signal_dbm, device?.modem?.signal_rsrp),
          hint: hasDashboardSignal(signal) ? signalQuality(signal) : ''
        },
        metric(t('dashboard.publicV4'), device?.public_ip, t('common.unassigned')),
        metric(t('dashboard.publicV6'), device?.public_ipv6, t('common.unassigned')),
        deviceInterface
      ])
    })
  }

  let tone: OverviewConnectionPresentation['tone'] = 'is-idle'
  let title = t('overview.vowifiWait')
  let detail = runtimeReason || t('overview.noLink')
  if (hasFailedStage) {
    tone = 'is-failed'
    title = t('overview.vowifiFailed')
    detail = runtimeReason || t('overview.checkFailed')
  } else if (device.vowifi_active && allStagesReady) {
    tone = 'is-ready'
    title = t('overview.vowifiConnected')
    detail = t('overview.vowifiConnectedHint')
  } else if (hasReadyStage) {
    tone = 'is-pending'
    title = t('overview.vowifiBuilding')
    detail = runtimeReason || t('overview.waitStages')
  }

  const runtime = device.vowifi_runtime
  return Object.freeze({
    kind: 'wifi',
    eyebrow: 'WI-FI CALLING',
    title,
    detail,
    tone,
    pathIsFlowing: device.healthy === true && device.vowifi_active === true && !hasFailedStage,
    stages,
    metrics: Object.freeze([
      metric(t('overview.access'), 'Wi-Fi Calling', 'Wi-Fi Calling'),
      metric(t('overview.dataplane'), runtime?.dataplane_mode),
      protocol,
      deviceInterface,
      metric(t('overview.lastReason'), runtime?.last_reason || runtime?.sms_ready_reason, t('common.none')),
      metric(t('overview.errorClass'), runtime?.last_error_class, t('common.none'))
    ])
  })
}

function createVoLTEStages(
  device: DeviceOverviewItem,
  status?: NativeVoLTEStatus
): readonly OverviewConnectionStage[] {
  return Object.freeze([
    Object.freeze({ key: 'SIM', ready: device.modem?.iccid ? true : undefined }),
    Object.freeze({ key: 'LTE', ready: volteStageReady(status?.lte_registered, status) }),
    Object.freeze({ key: 'PDN', ready: volteStageReady(status?.ims_pdn_active, status) }),
    Object.freeze({ key: 'IMS', ready: volteStageReady(status?.ims_registered, status) }),
    Object.freeze({ key: 'Voice', ready: volteStageReady(status?.voice_available, status) })
  ])
}

function volteStageReady(ok: boolean | undefined, status?: NativeVoLTEStatus): boolean | undefined {
  if (ok) return true
  if (status?.phase === 'failed') return false
  return undefined
}

function volteRegistered(status?: NativeVoLTEStatus): boolean {
  return status?.ims_registered === true || status?.phase === 'registered'
}

function volteServiceState(
  device: DeviceOverviewItem,
  status?: NativeVoLTEStatus
): { tone: OverviewConnectionPresentation['tone']; title: string; detail: string } {
  if (!device.vowifi_enabled) {
    return {
      tone: 'is-idle',
      title: 'VoLTE 未开启',
      detail: '打开卡策略里的电话后，模组会注册原生 IMS'
    }
  }
  if (status?.reboot_required) {
    return {
      tone: 'is-pending',
      title: 'VoLTE 需重启模组',
      detail: status.last_error || 'USB/UAC 变更后需要重启模组'
    }
  }
  if (status?.phase === 'failed') {
    return {
      tone: 'is-failed',
      title: 'VoLTE 失败',
      detail: status.last_error || '请检查模组 IMS 注册'
    }
  }
  if (volteRegistered(status)) {
    return {
      tone: 'is-ready',
      title: 'VoLTE 已注册',
      detail: volteReadyDetail(status)
    }
  }
  if (
    status?.phase === 'registering'
    || status?.phase === 'enabling'
    || status?.phase === 'ims_enabled_unverified'
  ) {
    return {
      tone: 'is-pending',
      title: 'VoLTE 正在注册',
      detail: status.last_error || status.provision_stage || '等待模组 IMS 注册'
    }
  }
  return {
    tone: 'is-idle',
    title: 'VoLTE 等待注册',
    detail: status?.last_error || '尚未收到 IMS 状态'
  }
}

function volteReadyDetail(status?: NativeVoLTEStatus): string {
  const parts = [status?.plmn, status?.mbn_name].map((value) => String(value || '').trim()).filter(Boolean)
  if (parts.length) return parts.join(' · ')
  return t('overview.volteRegistered')
}

function metric(label: string, value?: string | null, empty?: string): OverviewConnectionMetric {
  const text = String(value || '').trim()
  return Object.freeze({ label, value: text || empty || t('common.unavailable'), hint: '' })
}

function signalQuality(value: number): string {
  if (value >= -75) return '优秀'
  if (value >= -90) return '良好'
  if (value >= -105) return '一般'
  return '较弱'
}
