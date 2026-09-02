import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
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
  assert.match(globalStyles, /:root \{[\s\S]*--ui-nav: #FFFFFF;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-nav-surface: #FFFFFF;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-nav-text: #0B3558;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-nav-muted: #5B6B7A;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-nav-active: #0B3558;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-console: #0B3558;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-text: #0B3558;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-primary-solid: #0B3558;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-accent: #006BFF;/)
  assert.match(globalStyles, /:root \{[\s\S]*--ui-muted: #5B6B7A;/)
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
  assert.match(globalStyles, /:root \{[\s\S]*--ui-shadow-md: none;/)
  assert.match(globalStyles, /:root \{[\s\S]*--el-color-primary: #0B3558;/)
  assert.doesNotMatch(globalStyles, /:root \{[\s\S]*?--ui-nav: #0B3558;/)
  assert.doesNotMatch(globalStyles, /:root \{[\s\S]*?--ui-nav-surface: #0B3558;/)
})

test('navy-night keeps the same language without neon primary', () => {
  const darkBlock = globalStyles.match(/html\.dark \{[\s\S]*?\n\}/)?.[0] || ''
  assert.match(darkBlock, /--ui-bg: #0B1420;/)
  assert.match(darkBlock, /--ui-accent: #006BFF;/)
  assert.match(darkBlock, /--ui-muted: #8A96A3;/)
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
  assert.match(switchDark, /v-if="isDark"><WeatherMoon24Regular/)
  assert.match(switchDark, /v-else><WeatherSunny24Regular/)
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

test('vue and class helpers no longer use generic Tailwind gray palette classes', () => {
  const leftover = /\b(?:text|bg|border|divide|placeholder|ring|from|to|via)-gray-\d{2,3}\b|\bdark:text-white\b|\bdark:border-white\/|\bdark:bg-white\/|\bdark:text-gray-|\bdark:bg-gray-|\bdark:border-gray-|\bdark:divide-white|\bhover:bg-gray-/
  const srcRoot = new URL('../src/', import.meta.url)
  const hits: string[] = []

  function walk(dir: URL) {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const url = new URL(entry.name + (entry.isDirectory() ? '/' : ''), dir)
      if (entry.isDirectory()) {
        walk(url)
        continue
      }
      if (!/\.(vue|ts|css)$/.test(entry.name)) continue
      const text = readFileSync(url, 'utf8')
      if (leftover.test(text)) hits.push(join('src', decodeURIComponent(url.pathname.slice(srcRoot.pathname.length))))
    }
  }

  walk(srcRoot)
  assert.deepEqual(hits, [])
})

test('dashboard devices phone proxy commands and tasks use defined text tokens', () => {
  const files = [
    '../src/views/Dashboard.vue',
    '../src/views/Devices.vue',
    '../src/views/Phone.vue',
    '../src/styles/phone.css',
    '../src/views/Proxy.vue',
    '../src/views/Commands.vue',
    '../src/views/AutomaticTasks.vue',
    '../src/components/PhoneCallHistory.vue',
    '../src/components/commands/CommandChat.vue',
    '../src/components/commands/CommandTimeline.vue',
    '../src/components/commands/BalancePanel.vue',
    '../src/components/automation/AutomaticTaskList.vue'
  ]
  for (const path of files) {
    const text = source(path)
    assert.doesNotMatch(text, /\b(?:text|bg|border)-gray-\d{2,3}\b/, path)
    assert.doesNotMatch(text, /--ui-text-subtle/, path)
  }

  const dashboard = source('../src/views/Dashboard.vue')
  assert.doesNotMatch(dashboard, /WorkspacePreviewCard|信号日历|选择模组/)
  const phoneCss = source('../src/styles/phone.css')
  const history = source('../src/components/PhoneCallHistory.vue')
  const tasks = source('../src/views/AutomaticTasks.vue')
  assert.match(dashboard, /\.device-overview-toolbar h2 \{[^}]*color: var\(--ui-text\)/)
  assert.match(phoneCss, /\.console-header h2 \{[^}]*color: var\(--ui-text\)/)
  assert.match(history, /\.history-panel h2 \{[^}]*color: var\(--ui-text\)/)
  assert.match(tasks, /\.page-heading h1 \{[^}]*color: var\(--ui-text\)/)
  assert.match(source('../src/components/commands/CommandTimeline.vue'), /color: var\(--ui-muted\)/)
  assert.match(source('../src/components/automation/AutomaticTaskList.vue'), /color: var\(--ui-muted\)/)
})

test('secondary copy and timestamps use --ui-muted instead of navy --ui-text', () => {
  const settings = source('../src/views/Settings.vue')
  const empty = source('../src/components/EmptyState.vue')
  const fieldRow = source('../src/components/FieldRow.vue')
  const debug = source('../src/components/DebugPanel.vue')
  const app = source('../src/App.vue')
  assert.match(settings, /<h3 class="text-lg font-bold text-\[var\(--ui-text\)\]">安全<\/h3>/)
  assert.match(settings, /更新访问凭证<\/p>/)
  assert.match(settings, /text-\[var\(--ui-muted\)\]">更新访问凭证/)
  assert.match(empty, /text-\[var\(--ui-text\)\]">\{\{ title \}\}<\/div>/)
  assert.match(empty, /text-\[var\(--ui-muted\)\]">\{\{ subtitle \}\}<\/div>/)
  assert.match(fieldRow, /text-\[var\(--ui-muted\)\] shrink-0/)
  assert.match(debug, /text-\[var\(--ui-muted\)\]">\{\{ fmtTs/)
  assert.match(app, /text-\[var\(--ui-text\)\] tracking-tight/)
  assert.match(app, /text-\[14px\] text-\[var\(--ui-muted\)\]/)
  assert.match(globalStyles, /html\.classic \{[\s\S]*--ui-text-muted: #8f9b95;[\s\S]*--ui-muted: var\(--ui-text-muted\);/)
})

test('login, logs, and AT terminal no longer leak the old teal palette', () => {
  const login = source('../src/views/Login.vue')
  const logs = source('../src/views/Logs.vue')
  const atTerminal = source('../src/components/DeviceAtTab.vue')
  const leftover = /#071014|#67d2ca|#8ff7cb|#263a40|#081014|#050706|#c8d5ce|#66e9ad|#38bdb4|#102a2e|#06120e|rgba\(\s*92,\s*234,\s*177/
  assert.doesNotMatch(login, leftover)
  assert.doesNotMatch(logs, leftover)
  assert.doesNotMatch(atTerminal, leftover)
  assert.match(login, /background:[\s\S]*var\(--ui-bg\)/)
  assert.match(login, /class="login-page"/)
  assert.match(login, /class="login-identity"/)
  assert.doesNotMatch(login, /login-landing|WorkspacePreviewCard|信号日历|选择模组/)
  assert.doesNotMatch(login, /linear-gradient\(145deg, var\(--ui-nav\)/)
  assert.match(logs, /background: var\(--ui-console\);/)
  assert.match(atTerminal, /background: var\(--ui-console\);/)
  assert.match(shell, /background: var\(--ui-nav-surface\);/)
  assert.doesNotMatch(shell, /text-white/)
})
