import { computed, ref } from 'vue'
import { applyDocumentLocale, persistLocale, readStoredLocale, type AppLocale } from '../utils/locale'
import { en } from './en'
import { zhCN } from './zh-CN'

const catalogs = {
  'zh-CN': zhCN,
  en
} as const

export const locale = ref<AppLocale>(readStoredLocale())
applyDocumentLocale(locale.value)

function lookup(tree: unknown, path: string): string | undefined {
  let current: unknown = tree
  for (const part of path.split('.')) {
    if (current == null || typeof current !== 'object') return undefined
    current = (current as Record<string, unknown>)[part]
  }
  return typeof current === 'string' ? current : undefined
}

export function t(key: string, params?: Record<string, string | number>): string {
  const table = catalogs[locale.value]
  let text = lookup(table, key) ?? lookup(zhCN, key) ?? key
  if (params) {
    for (const [name, value] of Object.entries(params)) {
      text = text.replaceAll(`{${name}}`, String(value))
    }
  }
  return text
}

export function setLocale(next: AppLocale): void {
  locale.value = next
  persistLocale(next)
  applyDocumentLocale(next)
}

export function useLocale() {
  return {
    locale,
    t,
    setLocale,
    isEnglish: computed(() => locale.value === 'en')
  }
}
