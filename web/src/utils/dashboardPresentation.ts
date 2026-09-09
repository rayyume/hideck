import { t } from '../i18n'
import type { DashboardDevice, VoWiFiRuntimeState } from '../types/api'
import { isNativeVoLTEMode } from './phoneMode'
import { displaySignalDbm, hasValidSignalDbm } from './signalPresentation'
import {
  createVoLTEStages,
  volteRegistered,
  volteServiceState
} from './volteConnectionPresentation'

export const DASHBOARD_UNAVAILABLE = '不可用'
export const DASHBOARD_UNASSIGNED = '未分配'

export type DashboardConnectionStage = Readonly<{
  key: string
  ready: boolean | undefined
}>

export type DashboardDevicePresentation = Readonly<{
  connectionKind: 'wifi' | 'volte' | 'cellular'
  connectionState: string
  connectionTitle: string
  connectionType: string
  connectionDetail: string
  displayName: string
  ipv4: string
  ipv6: string
  operator: string
  showsCellularFacts: boolean
  signal: string
  stages: readonly DashboardConnectionStage[]
  statusLabel: string
}>

export type DashboardDeviceFilter = Readonly<{
  query: string
  status: 'all' | 'online' | 'offline'
}>

export type DashboardOperatorSource = Readonly<{
  id: string
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
  if (isNativeVoLTEMode(device.phone_mode)) return 'VoLTE'
  const parts = [device.network_duplex, device.network_mode]
    .map((value) => String(value || '').trim())
    .filter(Boolean)
  return parts.join(' ') || t('common.unavailable')
}

export function formatDashboardSignal(value: unknown, rsrp?: unknown): string {
  const dbm = displaySignalDbm(
    typeof value === 'number' ? value : undefined,
    typeof rsrp === 'number' ? rsrp : undefined
  )
  return dbm === undefined ? t('common.unavailable') : `${dbm} dBm`
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
  if (!device.healthy) return false
  if (isNativeVoLTEMode(device.phone_mode)) {
    return volteRegistered(device.native_volte)
  }
  if (device.vowifi_active !== true) return false
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
  const operators = new Map(managedDevices.map((device) => [
    device.id,
    managedOperatorFallback(device.modem)
  ]))
  return devices.map((device) => {
    if (String(device.operator || '').trim()) return device
    const operator = operators.get(device.id)
    return operator ? { ...device, operator } : device
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
  const isVoLTE = isNativeVoLTEMode(device.phone_mode)
  const volte = isVoLTE ? volteServiceState(true, device.native_volte) : null

  return Object.freeze({
    connectionKind: isVoLTE ? 'volte' : isVoWiFi ? 'wifi' : 'cellular',
    connectionState: isVoLTE
      ? (isOnline ? volte!.detail : t('dashboard.deviceUnavailable'))
      : getConnectionState(isOnline, isVoWiFi, connectionType),
    connectionTitle: isVoLTE
      ? (isOnline ? volte!.title : t('dashboard.deviceOfflineShort'))
      : getConnectionTitle(device, isOnline, isVoWiFi),
    connectionType,
    connectionDetail: isVoLTE ? volte!.detail : '',
    displayName: String(device.name || device.id).trim() || device.id,
    ipv4: isVoWiFi ? '' : normalizeAddress(device.public_ip),
    ipv6: isVoWiFi ? '' : normalizeAddress(device.public_ipv6),
    operator: normalizeFact(device.operator),
    showsCellularFacts: !isVoWiFi,
    signal: formatDashboardSignal(device.signal_dbm),
    stages: isVoLTE
      ? createVoLTEStages(isOnline ? true : undefined, device.native_volte)
      : createDashboardStages(device.vowifi_runtime),
    statusLabel: isOnline ? t('common.online') : t('common.offline')
  })
}

function getConnectionState(
  isOnline: boolean,
  isVoWiFi: boolean,
  connectionType: string
): string {
  if (!isOnline) return t('dashboard.deviceUnavailable')
  if (isVoWiFi) return t('common.connected')
  return connectionType === t('common.unavailable') ? t('dashboard.controlOnline') : connectionType
}

function getConnectionTitle(
  device: DashboardDevice,
  isOnline: boolean,
  isVoWiFi: boolean
): string {
  if (!isOnline) return t('dashboard.deviceOfflineShort')
  if (isVoWiFi) return t('dashboard.wifiCalling')
  return normalizeFact(device.operator, t('dashboard.detectingNetwork'))
}

function normalizeAddress(value: unknown): string {
  return String(value || '').trim() || t('common.unassigned')
}

function normalizeFact(value: unknown, fallback = t('common.unavailable')): string {
  return String(value || '').trim() || fallback
}
