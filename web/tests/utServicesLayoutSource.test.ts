import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const utServices = await readFile(
  new URL('../src/views/UtServices.vue', import.meta.url),
  'utf8'
)
const shell = await readFile(
  new URL('../src/layouts/AuthenticatedShell.vue', import.meta.url),
  'utf8'
)
const router = await readFile(
  new URL('../src/router/index.ts', import.meta.url),
  'utf8'
)

test('Ut services reads callService payloads from data, not value', () => {
  assert.match(utServices, /result\.data\.devices/)
  assert.match(utServices, /applyDoc\(result\.data\)/)
  assert.doesNotMatch(utServices, /result\.value/)
})

test('Ut services uses the shared workspace, Element Plus controls, and error surface', () => {
  assert.match(utServices, /<PageHeader title="呼叫设置"/)
  assert.match(utServices, /class="ut-workspace ui-card ui-workspace-glow"/)
  assert.match(utServices, /<ErrorState/)
  assert.match(utServices, /title="呼叫设置不可用"/)
  assert.match(utServices, /<el-select/)
  assert.match(utServices, /<el-switch/)
  assert.match(utServices, /<el-input/)
  assert.match(utServices, /<el-button type="primary"/)
  assert.match(utServices, /createLatestRequestGate/)
  assert.doesNotMatch(utServices, /补充业务/)
  assert.doesNotMatch(utServices, /<select |<input type="checkbox"|class="ut-btn"|class="ut-card"/)
})

test('sidebar hides 呼叫设置 until Ut/XCAP is reachable', () => {
  assert.doesNotMatch(shell, /index: '\/ut', label: '呼叫设置'/)
  assert.doesNotMatch(shell, /补充业务/)
  assert.match(router, /path: '\/ut'[\s\S]*?redirect: '\/'/)
})

test('Ut services only lists software IMS WiFi calling devices', () => {
  assert.match(utServices, /deviceSupportsUt/)
  assert.match(utServices, /没有使用 WiFi calling 的设备/)
  assert.match(utServices, /all\.filter\(deviceSupportsUt\)/)
})

test('Ut setting rows stay touch-safe and reduced-motion compatible', () => {
  assert.match(utServices, /\.ut-setting-row \{[^}]*min-height: 72px/)
  assert.match(utServices, /\.ut-setting-row :deep\(\.el-switch\) \{[^}]*min-width: 44px;[^}]*min-height: 44px/)
  assert.match(utServices, /@media \(prefers-reduced-motion: reduce\)[\s\S]*animation: none/)
  assert.match(utServices, /var\(--ui-text\)/)
  assert.match(utServices, /var\(--ui-surface-strong\)/)
})
