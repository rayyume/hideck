export const THEME_STORAGE_KEY = 'theme'

export const THEME_MODES = ['navy-light', 'navy-night', 'classic'] as const

export type ThemeMode = (typeof THEME_MODES)[number]

export type NavyThemeMode = Exclude<ThemeMode, 'classic'>

export type ThemeClassNames = {
  dark: boolean
  classic: boolean
}

export function isThemeMode(value: string | null | undefined): value is ThemeMode {
  return value === 'navy-light' || value === 'navy-night' || value === 'classic'
}

export function resolveStoredTheme(value: string | null | undefined): ThemeMode {
  if (isThemeMode(value)) return value
  if (value === 'dark') return 'navy-night'
  if (value === 'light') return 'navy-light'
  return 'navy-light'
}

export function readStoredTheme(storage: Pick<Storage, 'getItem'> | null | undefined = defaultStorage()): ThemeMode {
  try {
    return resolveStoredTheme(storage?.getItem(THEME_STORAGE_KEY))
  } catch {
    return 'navy-light'
  }
}

export function persistTheme(
  mode: ThemeMode,
  storage: Pick<Storage, 'setItem'> | null | undefined = defaultStorage()
): void {
  try {
    storage?.setItem(THEME_STORAGE_KEY, mode)
  } catch {
    // Storage can be unavailable in hardened browser contexts.
  }
}

export function isDarkTheme(mode: ThemeMode): boolean {
  return mode === 'navy-night' || mode === 'classic'
}

export function themeClassNames(mode: ThemeMode): ThemeClassNames {
  return {
    dark: isDarkTheme(mode),
    classic: mode === 'classic'
  }
}

export function nextNavyTheme(mode: ThemeMode): NavyThemeMode {
  return mode === 'navy-light' ? 'navy-night' : 'navy-light'
}

export function applyThemeClass(
  mode: ThemeMode,
  root: Pick<HTMLElement, 'classList'> | null | undefined = defaultRoot()
): ThemeClassNames {
  const names = themeClassNames(mode)
  if (!root) return names
  root.classList.toggle('dark', names.dark)
  root.classList.toggle('classic', names.classic)
  return names
}

function defaultStorage(): Storage | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage
  } catch {
    return null
  }
}

function defaultRoot(): HTMLElement | null {
  return typeof document === 'undefined' ? null : document.documentElement
}
