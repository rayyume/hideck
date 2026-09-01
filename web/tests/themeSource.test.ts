import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

function source(relativePath: string): string {
  return readFileSync(new URL(relativePath, import.meta.url), 'utf8')
}

const globalStyles = source('../src/style.css')
const app = source('../src/App.vue')
const switchDark = source('../src/components/SwitchDark.vue')
const settings = source('../src/views/Settings.vue')
const shell = source('../src/layouts/AuthenticatedShell.vue')
const unauth = source('../src/layouts/UnauthenticatedShell.vue')
const themeUtil = source('../src/utils/theme.ts')
const themeComposable = source('../src/composables/useTheme.ts')
const indexHtml = source('../index.html')

test('navy-light tokens match the approved palette', () => {
  assert.match(globalStyles, /:root \{[\s\S]*--ui-bg: #FBF9F6;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-text: #0B3558;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-primary-solid: #0B3558;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-accent: #006BFF;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-text-muted: #5B6B7A;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-surface: #ffffff;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-border: #E3EAF0;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-surface-muted: #F3F6F9;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-selected: #E8F1FF;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-success: #1F7A4D;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-success-surface: #E6F4EC;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-warning: #9A6700;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-warning-surface: #F8F1DE;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-danger: #B42318;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-danger-surface: #FDECEC;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-radius-md: 12px;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-radius-sm: 8px;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-radius-pill: 999px;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-font-weight-body: 500;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-font-weight-title: 700;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-shadow-sm: none;/)
  assert.match(globalStyles, /:root \{[\s\S]*--el-color-primary: #0B3558;/)
})

test('navy-night keeps the same language without neon primary', () => {
  const darkBlock = globalStyles.match(/html\.dark \{[\s\S]*?\n\}/)?.[0] || ''
  assert.match(darkBlock, /--ui-bg: #0B1420;/)
  assert.match(darkBlock, /--ui-accent: #006BFF;/)
  assert.match(darkBlock, /--ui-selected: #163050;/)
  assert.match(darkBlock, /--ui-primary-solid: #0B3558;/)
  assert.doesNotMatch(darkBlock, /#66e9ad/)
  assert.match(globalStyles, /html\.dark \.ui-workspace-glow::before/)
})

test('classic preserves the previous dark theme tokens', () => {
  assert.match(globalStyles, /html\.classic \{[\s\S]*--ui-bg: #050807;/)
  assert.match(globalStyles, /html\.classic \{[\s\S]*--ui-primary: #66e9ad;/)
  assert.match(globalStyles, /html\.classic \{[\s\S]*--ui-primary-solid: #1a8b62;/)
  assert.match(globalStyles, /html\.classic \{[\s\S]*--el-color-primary: #1a8b62;/)
  assert.match(globalStyles, /html\.classic body \{[\s\S]*radial-gradient\(circle at 69% 12%/)
})

test('primary buttons are navy pills and selected nav uses the accent bar token', () => {
  assert.match(globalStyles, /\.el-button \{[\s\S]*border-radius: var\(--ui-radius-pill\);/)
  assert.match(globalStyles, /\.el-button--primary:not\(\.is-plain\):not\(\.is-text\):not\(\.is-link\) \{[\s\S]*var\(--ui-primary-solid\)/)
  assert.match(globalStyles, /\.el-menu-item\.is-active \{[\s\S]*var\(--ui-selected\)/)
  assert.match(shell, /--sidebar-menu-active-bg: var\(--ui-selected\);/)
  assert.match(shell, /background: var\(--ui-accent\);/)
})

test('sun button wiring stays a two-state navy toggle', () => {
  assert.match(app, /const \{ isDark, toggleTheme \} = useTheme\(\)/)
  assert.match(themeComposable, /applyTheme\(nextNavyTheme\(theme\.value\)\)/)
  assert.doesNotMatch(themeComposable, /function toggleTheme\(\) \{[^}]*classic/)
  assert.match(switchDark, /isDark: boolean/)
  assert.doesNotMatch(switchDark, /classic|navy-night|three/)
  assert.match(shell, /<SwitchDark :is-dark="isDark" @toggle="\(e\) => emit\('toggle-theme', e\)" \/>/)
  assert.match(unauth, /<SwitchDark :is-dark="isDark" @toggle="\(e\) => emit\('toggle-theme', e\)" \/>/)
})

test('classic is applied from settings and never from the header control', () => {
  assert.match(settings, /经典主题/)
  assert.match(settings, /applyClassicTheme/)
  assert.match(settings, /restoreNavyTheme\('navy-night'\)/)
  assert.doesNotMatch(shell, /applyClassic|经典/)
  assert.doesNotMatch(switchDark, /经典/)
  assert.match(themeUtil, /nextNavyTheme/)
})

test('boot script understands navy and classic storage keys', () => {
  assert.match(indexHtml, /theme === 'navy-night' \|\| theme === 'classic'/)
  assert.match(indexHtml, /classList.toggle\('classic', theme === 'classic'\)/)
  assert.match(indexHtml, /background: #FBF9F6;/)
})

test('login, logs, and AT terminal no longer leak the old teal palette', () => {
  const login = source('../src/views/Login.vue')
  const logs = source('../src/views/Logs.vue')
  const atTerminal = source('../src/components/DeviceAtTab.vue')
  const leftover = /#071014|#67d2ca|#8ff7cb|#263a40|#081014|#050706|#c8d5ce|#66e9ad|#38bdb4|#102a2e|#06120e|rgba\(\s*92,\s*234,\s*177/
  assert.doesNotMatch(login, leftover)
  assert.doesNotMatch(logs, leftover)
  assert.doesNotMatch(atTerminal, leftover)
  assert.match(login, /background:[\s\S]*var\(--ui-nav\)/)
  assert.match(logs, /background: var\(--ui-nav\);/)
  assert.match(atTerminal, /background: var\(--ui-nav\);/)
})
