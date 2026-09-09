import { t } from '../i18n'
import type { DeviceOverviewItem } from '../types/api'
import {
  createDashboardStages,
  formatDashboardSignal,
  hasDashboardSignal
} from './dashboardPresentation'
import { displaySignalDbm } from './signalPresentation'
import { isNativeVoLTEMode, phoneModeCampsOnCell } from './phoneMode'
import {
  createVoLTEStages,
  volteRegistered,
  volteServiceState
} from './volteConnectionPresentation'

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
  const stages = createVoLTEStages(device.modem?.iccid ? true : undefined, status)
  const state = volteServiceState(device.vowifi_enabled === true, status)
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
