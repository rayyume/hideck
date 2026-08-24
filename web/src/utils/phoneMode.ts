/** 通话方式：wifi 软件 VoWiFi；cellular 软件 IMS 走蜂窝数据；volte 模组原生 IMS。 */
export function normalizePhoneMode(mode?: string | null): string {
  const value = (mode ?? 'wifi').trim()
  return value || 'wifi'
}

export function isNativeVoLTEMode(mode?: string | null): boolean {
  return normalizePhoneMode(mode) === 'volte'
}

export function phoneModeCampsOnCell(mode?: string | null): boolean {
  const value = normalizePhoneMode(mode)
  return value === 'cellular' || value === 'volte'
}

export function isWifiCallingEnabled(mode?: string | null, phoneEnabled?: boolean): boolean {
  return !!phoneEnabled && !phoneModeCampsOnCell(mode)
}

export function phoneModeLabel(mode?: string | null): string {
  const value = normalizePhoneMode(mode)
  if (value === 'volte') return 'VoLTE'
  if (value === 'cellular') return '蜂窝数据'
  return 'WiFi calling'
}
