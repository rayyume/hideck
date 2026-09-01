import assert from 'node:assert/strict'
import test from 'node:test'
import {
  applyThemeClass,
  isDarkTheme,
  nextNavyTheme,
  persistTheme,
  readStoredTheme,
  resolveStoredTheme,
  themeClassNames,
  THEME_STORAGE_KEY
} from '../src/utils/theme'

test('stored theme migrates the old light/dark pair onto navy', () => {
  assert.equal(resolveStoredTheme('light'), 'navy-light')
  assert.equal(resolveStoredTheme('dark'), 'navy-night')
  assert.equal(resolveStoredTheme('navy-light'), 'navy-light')
  assert.equal(resolveStoredTheme('navy-night'), 'navy-night')
  assert.equal(resolveStoredTheme('classic'), 'classic')
  assert.equal(resolveStoredTheme(null), 'navy-light')
  assert.equal(resolveStoredTheme('unknown'), 'navy-light')
})

test('sun-button next theme stays on the navy pair', () => {
  assert.equal(nextNavyTheme('navy-light'), 'navy-night')
  assert.equal(nextNavyTheme('navy-night'), 'navy-light')
  assert.equal(nextNavyTheme('classic'), 'navy-light')
})

test('classic is a dark document class but not a sun-button state', () => {
  assert.equal(isDarkTheme('navy-light'), false)
  assert.equal(isDarkTheme('navy-night'), true)
  assert.equal(isDarkTheme('classic'), true)
  assert.deepEqual(themeClassNames('navy-light'), { dark: false, classic: false })
  assert.deepEqual(themeClassNames('navy-night'), { dark: true, classic: false })
  assert.deepEqual(themeClassNames('classic'), { dark: true, classic: true })
})

test('theme class names only toggle dark and classic on the root', () => {
  const classes = new Set<string>()
  const root = {
    classList: {
      toggle(name: string, force?: boolean) {
        if (force) classes.add(name)
        else classes.delete(name)
        return force ?? false
      }
    }
  }

  applyThemeClass('navy-night', root)
  assert.deepEqual([...classes].sort(), ['dark'])
  applyThemeClass('classic', root)
  assert.deepEqual([...classes].sort(), ['classic', 'dark'])
  applyThemeClass('navy-light', root)
  assert.deepEqual([...classes], [])
})

test('theme persistence writes the resolved navy or classic key', () => {
  const store = new Map<string, string>()
  persistTheme('classic', { setItem: (key, value) => store.set(key, value) })
  assert.equal(store.get(THEME_STORAGE_KEY), 'classic')
  assert.equal(
    readStoredTheme({ getItem: (key) => (key === THEME_STORAGE_KEY ? 'dark' : null) }),
    'navy-night'
  )
})
