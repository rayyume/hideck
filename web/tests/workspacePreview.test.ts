import assert from 'node:assert/strict'
import test from 'node:test'
import type { DashboardDevice } from '../src/types/api.ts'
import {
  LOGIN_PREVIEW_DEMO,
  buildMonthSignalDays,
  createLiveWorkspacePreview,
  dashboardModuleLabel,
  formatMonthCalendarLabel
} from '../src/utils/workspacePreview.ts'

function createDevice(overrides: Partial<DashboardDevice> = {}): DashboardDevice {
  return {
    id: 'modem-1',
    name: 'QA Modem',
    healthy: true,
    operator: '中国移动',
    network_mode: 'LTE',
    network_duplex: 'TDD',
    signal_dbm: -68,
    ...overrides
  }
}

test('login preview demo matches the landing mock copy', () => {
  assert.equal(LOGIN_PREVIEW_DEMO.demo, true)
  assert.equal(LOGIN_PREVIEW_DEMO.title, '中国移动 · TDD LTE')
  assert.equal(LOGIN_PREVIEW_DEMO.signal, '-68 dBm')
  assert.equal(LOGIN_PREVIEW_DEMO.calendarLabel, '9月 · 信号日历')
  assert.equal(LOGIN_PREVIEW_DEMO.modules[0]?.label, 'wwan0')
  assert.equal(LOGIN_PREVIEW_DEMO.modules[1]?.status, '待命')
})

test('month signal calendar only fills today from live signal', () => {
  const days = buildMonthSignalDays(new Date(2026, 8, 2), '-68 dBm')
  assert.equal(days.length, 5)
  assert.equal(days[0]?.day, 1)
  assert.equal(days[0]?.signal, '—')
  assert.equal(days[1]?.day, 2)
  assert.equal(days[1]?.selected, true)
  assert.equal(days[1]?.signal, '-68 dBm')
  assert.equal(formatMonthCalendarLabel(new Date(2026, 8, 2)), '9月 · 信号日历')
})

test('live preview uses operator, network and module interface labels', () => {
  const preview = createLiveWorkspacePreview([
    createDevice({ iface: 'wwan0' }),
    createDevice({ id: 'modem-2', name: 'Standby', healthy: false, iface: 'wwan1', signal_dbm: -999 })
  ], 'modem-1', new Date(2026, 8, 2))

  assert.equal(preview.empty, false)
  assert.equal(preview.demo, false)
  assert.equal(preview.title, '中国移动 · TDD LTE')
  assert.equal(preview.signal, '-68 dBm')
  assert.equal(preview.modules[0]?.label, 'wwan0')
  assert.equal(preview.modules[0]?.selected, true)
  assert.equal(preview.modules[1]?.status, '待命')
  assert.equal(dashboardModuleLabel({ id: 'x', name: 'Named' }), 'Named')
})

test('empty live preview keeps the card language without inventing devices', () => {
  const preview = createLiveWorkspacePreview([], '', new Date(2026, 8, 2))
  assert.equal(preview.empty, true)
  assert.equal(preview.title, '等待设备接入')
  assert.equal(preview.signal, '—')
  assert.equal(preview.modules.length, 0)
})
