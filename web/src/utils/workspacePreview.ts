import type { DashboardDevice } from '../types/api'
import {
  DASHBOARD_UNAVAILABLE,
  formatDashboardNetworkType,
  formatDashboardSignal,
  hasDashboardSignal
} from './dashboardPresentation'

export type WorkspacePreviewDay = Readonly<{
  key: string
  day: number
  weekday: string
  signal: string
  selected: boolean
}>

export type WorkspacePreviewModule = Readonly<{
  id: string
  label: string
  status: string
  selected: boolean
  online: boolean
}>

export type WorkspacePreviewModel = Readonly<{
  kicker: string
  title: string
  signal: string
  calendarLabel: string
  days: readonly WorkspacePreviewDay[]
  modules: readonly WorkspacePreviewModule[]
  demo: boolean
  empty: boolean
}>

const WEEKDAYS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'] as const

export const LOGIN_PREVIEW_DEMO: WorkspacePreviewModel = Object.freeze({
  kicker: '仪表盘',
  title: '中国移动 · TDD LTE',
  signal: '-68 dBm',
  calendarLabel: '9月 · 信号日历',
  days: Object.freeze([
    Object.freeze({ key: 'demo-1', day: 1, weekday: '周一', signal: '-72 dBm', selected: false }),
    Object.freeze({ key: 'demo-2', day: 2, weekday: '周二', signal: '-68 dBm', selected: true }),
    Object.freeze({ key: 'demo-3', day: 3, weekday: '周三', signal: '-70 dBm', selected: false }),
    Object.freeze({ key: 'demo-4', day: 4, weekday: '周四', signal: '-75 dBm', selected: false }),
    Object.freeze({ key: 'demo-5', day: 5, weekday: '周五', signal: '-71 dBm', selected: false })
  ]),
  modules: Object.freeze([
    Object.freeze({ id: 'wwan0', label: 'wwan0', status: '在线', selected: true, online: true }),
    Object.freeze({ id: 'wwan1', label: 'wwan1', status: '待命', selected: false, online: false })
  ]),
  demo: true,
  empty: false
})

export function weekdayLabel(date: Date): string {
  return WEEKDAYS[date.getDay()] || ''
}

export function formatMonthCalendarLabel(date: Date): string {
  return `${date.getMonth() + 1}月 · 信号日历`
}

export function dashboardModuleLabel(device: Pick<DashboardDevice, 'id' | 'name' | 'iface'>): string {
  return String(device.iface || device.name || device.id).trim() || device.id
}

export function buildMonthSignalDays(
  now: Date,
  todaySignal: string,
  count = 5
): WorkspacePreviewDay[] {
  const year = now.getFullYear()
  const month = now.getMonth()
  const today = now.getDate()
  const start = today <= count ? 1 : today - count + 1
  const days: WorkspacePreviewDay[] = []

  for (let day = start; day < start + count; day++) {
    const date = new Date(year, month, day)
    const isToday = day === today
    days.push(Object.freeze({
      key: `${year}-${month + 1}-${day}`,
      day,
      weekday: weekdayLabel(date),
      signal: isToday ? todaySignal : '—',
      selected: isToday
    }))
  }

  return days
}

export function createLiveWorkspacePreview(
  devices: readonly DashboardDevice[],
  selectedId: string,
  now = new Date()
): WorkspacePreviewModel {
  const selected = devices.find((device) => device.id === selectedId)
    || devices.find((device) => device.healthy)
    || devices[0]
  const empty = devices.length === 0
  const signal = selected && hasDashboardSignal(selected.signal_dbm)
    ? formatDashboardSignal(selected.signal_dbm)
    : '—'
  const operator = String(selected?.operator || '').trim()
  const network = selected ? formatDashboardNetworkType(selected) : ''
  const title = empty
    ? '等待设备接入'
    : [operator || '网络检测中', network && network !== DASHBOARD_UNAVAILABLE ? network : '']
      .filter(Boolean)
      .join(' · ')

  return Object.freeze({
    kicker: '仪表盘',
    title,
    signal,
    calendarLabel: formatMonthCalendarLabel(now),
    days: Object.freeze(buildMonthSignalDays(now, signal)),
    modules: Object.freeze(devices.map((device) => Object.freeze({
      id: device.id,
      label: dashboardModuleLabel(device),
      status: device.healthy ? '在线' : '待命',
      selected: device.id === selected?.id,
      online: device.healthy
    }))),
    demo: false,
    empty
  })
}
