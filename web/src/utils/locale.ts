export const LOCALE_STORAGE_KEY = 'hideck_locale'

export const APP_LOCALES = ['zh-CN', 'en'] as const

export type AppLocale = (typeof APP_LOCALES)[number]

export function isAppLocale(value: string | null | undefined): value is AppLocale {
  return value === 'zh-CN' || value === 'en'
}

export function resolveStoredLocale(value: string | null | undefined): AppLocale {
  if (isAppLocale(value)) return value
  if (value === 'zh' || value === 'zh-TW' || value === 'zh-Hans' || value === 'zh-Hant') return 'zh-CN'
  if (value === 'en-US' || value === 'en-GB' || value === 'en-AU') return 'en'
  return 'zh-CN'
}

function defaultStorage(): Storage | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage
  } catch {
    return null
  }
}

export function readStoredLocale(storage: Pick<Storage, 'getItem'> | null | undefined = defaultStorage()): AppLocale {
  try {
    return resolveStoredLocale(storage?.getItem(LOCALE_STORAGE_KEY))
  } catch {
    return 'zh-CN'
  }
}

export function persistLocale(
  mode: AppLocale,
  storage: Pick<Storage, 'setItem'> | null | undefined = defaultStorage()
): void {
  try {
    storage?.setItem(LOCALE_STORAGE_KEY, mode)
  } catch {
    // Storage can be unavailable in hardened browser contexts.
  }
}

export function applyDocumentLocale(
  mode: AppLocale,
  root: Pick<HTMLElement, 'lang'> | null | undefined = typeof document === 'undefined' ? null : document.documentElement
): void {
  if (!root) return
  root.lang = mode === 'en' ? 'en' : 'zh-CN'
}

export function nextLocale(mode: AppLocale): AppLocale {
  return mode === 'en' ? 'zh-CN' : 'en'
}
