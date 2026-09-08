import { t } from '../i18n'
import type { DeviceLifecyclePhase, DeviceMgmtListItem, DeviceOverviewItem } from '../types/api'

type DeviceLike = Pick<
  DeviceMgmtListItem | DeviceOverviewItem,
  'running' | 'healthy' | 'control_online' | 'lifecycle_phase' | 'modem'
>

export function isRecoveryPhase(phase?: DeviceLifecyclePhase) {
  return phase === 'rebooting' ||
    phase === 'usb_wait' ||
    phase === 'worker_starting' ||
    phase === 'qmi_starting' ||
    phase === 'recovering' ||
    phase === 'evicting'
}

export function isControlOnline(device: DeviceLike | null | undefined) {
  if (!device) return false
  if (isRecoveryPhase(device.lifecycle_phase)) return false
  return !!device.running && (device.control_online ?? device.healthy) === true
}

export function isRadioRegistered(device: DeviceLike | null | undefined) {
  const r = device?.modem?.reg_status
  return r === 1 || r === 5
}

export function lifecycleStatusLabel(phase?: DeviceLifecyclePhase) {
  switch (phase) {
    case 'rebooting':
      return t('lifecycle.rebooting')
    case 'usb_wait':
      return t('lifecycle.usbWait')
    case 'worker_starting':
      return t('lifecycle.workerStarting')
    case 'qmi_starting':
      return t('lifecycle.qmiStarting')
    case 'recovering':
      return t('lifecycle.recovering')
    case 'degraded':
      return t('lifecycle.degraded')
    case 'evicting':
      return t('lifecycle.evicting')
    case 'online':
      return t('lifecycle.online')
    case 'offline':
      return t('lifecycle.offline')
    default:
      return ''
  }
}

export function primaryLifecycleStatus(device: DeviceLike | null | undefined) {
  const phase = device?.lifecycle_phase
  if (isRecoveryPhase(phase)) {
    return { label: lifecycleStatusLabel(phase) || t('lifecycle.recoveringShort'), tag: 'warning' as const, tone: 'warning' as const, animated: true }
  }
  if (phase === 'degraded') {
    return { label: t('lifecycle.unstable'), tag: 'warning' as const, tone: 'warning' as const, animated: true }
  }
  if (!device?.running) {
    return { label: t('lifecycle.offline'), tag: 'danger' as const, tone: 'danger' as const, animated: false }
  }
  if (!isControlOnline(device)) {
    return { label: t('lifecycle.recoveringShort'), tag: 'warning' as const, tone: 'warning' as const, animated: true }
  }
  return { label: t('lifecycle.online'), tag: 'success' as const, tone: 'success' as const, animated: true }
}
