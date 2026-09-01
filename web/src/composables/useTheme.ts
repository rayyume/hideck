import { computed, ref } from 'vue'
import {
  applyThemeClass,
  nextNavyTheme,
  persistTheme,
  readStoredTheme,
  type ThemeMode
} from '../utils/theme'

const theme = ref<ThemeMode>(readStoredTheme())

applyThemeClass(theme.value)

export function useTheme() {
  const isDark = computed(() => theme.value !== 'navy-light')
  const isClassic = computed(() => theme.value === 'classic')

  function applyTheme(mode: ThemeMode) {
    theme.value = mode
    persistTheme(mode)
    applyThemeClass(mode)
  }

  function toggleTheme() {
    applyTheme(nextNavyTheme(theme.value))
  }

  function applyClassic() {
    applyTheme('classic')
  }

  function restoreNavyTheme(mode: Exclude<ThemeMode, 'classic'> = 'navy-night') {
    applyTheme(mode)
  }

  return {
    theme,
    isDark,
    isClassic,
    applyTheme,
    toggleTheme,
    applyClassic,
    restoreNavyTheme
  }
}
