import type { DashboardDevice, VoWiFiRuntimeState } from '../types/api'
import { displaySignalDbm, hasValidSignalDbm } from './signalPresentation'

export const DASHBOARD_UNAVAILABLE = '不可用'
export const DASHBOARD_UNASSIGNED = '未分配'

export type DashboardConnectionStage = Readonly<{
  key: 'SIM' | 'Access' | 'Tunnel' | 'IMS' | 'SMS'
  ready: boolean | undefined
}>

export type DashboardDevicePresentation = Readonly<{
  connectionState: string
  connectionTitle: string
  connectionType: string
  displayName: string
  ipv4: string
  ipv6: string
  operator: string
  showsCellularFacts: boolean
  signal: string
  stages: readonly DashboardConnectionStage[]
  statusLabel: '在线' | '离线'
}>

export type DashboardDeviceFilter = Readonly<{
  query: string
  status: 'all' | 'online' | 'offline'
}>

export type DashboardOperatorSource = Readonly<{
  id: string
  interface?: string
  modem?: Readonly<{
    operator?: string
    native_spn?: string
    native_mcc?: string
    native_mnc?: string
  }>
}>

export function hasDashboardSignal(value: unknown): value is number {
  return hasValidSignalDbm(value)
}

export function formatDashboardNetworkType(device: DashboardDevice): string {
  if (device.vowifi_active) return 'VoWiFi'
  const parts = [device.network_duplex, device.network_mode]
    .map((value) => String(value || '').trim())
    .filter(Boolean)
  return parts.join(' ') || DASHBOARD_UNAVAILABLE
}

export function formatDashboardSignal(value: unknown, rsrp?: unknown): string {
  const dbm = displaySignalDbm(
    typeof value === 'number' ? value : undefined,
    typeof rsrp === 'number' ? rsrp : undefined
  )
  return dbm === undefined ? DASHBOARD_UNAVAILABLE : `${dbm} dBm`
}

export function createDashboardStages(
  runtime?: VoWiFiRuntimeState
): readonly DashboardConnectionStage[] {
  return Object.freeze([
    Object.freeze({ key: 'SIM', ready: runtime?.sim_ready }),
    Object.freeze({ key: 'Access', ready: runtime?.access_ready }),
    Object.freeze({ key: 'Tunnel', ready: runtime?.tunnel_ready }),
    Object.freeze({ key: 'IMS', ready: runtime?.ims_ready }),
    Object.freeze({ key: 'SMS', ready: runtime?.sms_ready })
  ])
}

export function canAnimateDashboardConnection(device: DashboardDevice): boolean {
  if (!device.healthy || device.vowifi_active !== true) return false
  return !createDashboardStages(device.vowifi_runtime).some(stage => stage.ready === false)
}

export function filterDashboardDevices(
  devices: readonly DashboardDevice[],
  filter: DashboardDeviceFilter
): DashboardDevice[] {
  const query = filter.query.trim().toLocaleLowerCase()
  return devices.filter((device) => {
    if (filter.status === 'online' && !device.healthy) return false
    if (filter.status === 'offline' && device.healthy) return false
    if (!query) return true
    return [device.id, device.name, device.operator, device.public_ip, device.public_ipv6]
      .some(value => String(value || '').toLocaleLowerCase().includes(query))
  })
}

export function mergeDashboardDeviceOperators(
  devices: readonly DashboardDevice[],
  managedDevices: readonly DashboardOperatorSource[]
): DashboardDevice[] {
  const extras = new Map(managedDevices.map((device) => [
    device.id,
    {
      operator: managedOperatorFallback(device.modem),
      iface: String(device.interface || '').trim()
    }
  ]))
  return devices.map((device) => {
    const extra = extras.get(device.id)
    if (!extra) return device
    const next = { ...device }
    if (!String(next.operator || '').trim() && extra.operator) next.operator = extra.operator
    if (!String(next.iface || '').trim() && extra.iface) next.iface = extra.iface
    return next
  })
}

function managedOperatorFallback(modem?: DashboardOperatorSource['modem']): string {
  const serving = String(modem?.operator || '').trim()
  if (serving) return serving
  const spn = String(modem?.native_spn || '').trim()
  if (spn) return spn
  const mcc = String(modem?.native_mcc || '').trim()
  const mnc = String(modem?.native_mnc || '').trim()
  return mcc && mnc ? `${mcc}${mnc}` : ''
}

export function createDashboardDevicePresentation(
  device: DashboardDevice
): DashboardDevicePresentation {
  const connectionType = formatDashboardNetworkType(device)
  const isOnline = device.healthy
  const isVoWiFi = device.vowifi_active === true

  return Object.freeze({
    connectionState: getConnectionState(isOnline, isVoWiFi, connectionType),
    connectionTitle: getConnectionTitle(device, isOnline, isVoWiFi),
    connectionType,
    displayName: String(device.name || device.id).trim() || device.id,
    ipv4: isVoWiFi ? '' : normalizeAddress(device.public_ip),
    ipv6: isVoWiFi ? '' : normalizeAddress(device.public_ipv6),
    operator: normalizeFact(device.operator),
    showsCellularFacts: !isVoWiFi,
    signal: formatDashboardSignal(device.signal_dbm),
    stages: createDashboardStages(device.vowifi_runtime),
    statusLabel: isOnline ? '在线' : '离线'
  })
}

function getConnectionState(
  isOnline: boolean,
  isVoWiFi: boolean,
  connectionType: string
): string {
  if (!isOnline) return '当前设备不可用'
  if (isVoWiFi) return '已连接'
  return connectionType === DASHBOARD_UNAVAILABLE ? '控制面在线' : connectionType
}

function getConnectionTitle(
  device: DashboardDevice,
  isOnline: boolean,
  isVoWiFi: boolean
): string {
  if (!isOnline) return '设备离线'
  if (isVoWiFi) return 'Wi-Fi Calling'
  return normalizeFact(device.operator, '网络检测中')
}

function normalizeAddress(value: unknown): string {
  return String(value || '').trim() || DASHBOARD_UNASSIGNED
}

function normalizeFact(value: unknown, fallback = DASHBOARD_UNAVAILABLE): string {
  return String(value || '').trim() || fallback
}
