export const QMI_UNAVAILABLE_RSSI_DBM = -125
export const QMI_INVALID_SIGNAL_DBM = -128
export const LEGACY_INVALID_SIGNAL_DBM = -999

const SIGNAL_SENTINELS = new Set([
  0,
  QMI_UNAVAILABLE_RSSI_DBM,
  QMI_INVALID_SIGNAL_DBM,
  LEGACY_INVALID_SIGNAL_DBM
])

export function hasValidSignalDbm(value: unknown): value is number {
  return typeof value === 'number'
    && Number.isFinite(value)
    && !SIGNAL_SENTINELS.has(value)
}

export function displaySignalDbm(
  rssi?: number | null,
  rsrp?: number | null
): number | undefined {
  if (hasValidSignalDbm(rssi)) return rssi
  if (hasValidSignalDbm(rsrp)) return rsrp
  return undefined
}
