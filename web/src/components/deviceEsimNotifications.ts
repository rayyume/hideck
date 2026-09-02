import type { EsimNotificationItem } from '../types/api'

export type EsimNotificationDialogState = {
  isOpen: boolean
  items: EsimNotificationItem[]
  retryingSequenceNumber: number | null
  lastRetriedSequenceNumber: number | null
}

export type ReconcileEsimNotificationDialogInput = {
  isOpen: boolean
  items: EsimNotificationItem[]
  refreshedItems: EsimNotificationItem[]
  retriedSequenceNumber: number | null
}

export function countEsimNotifications(items: EsimNotificationItem[]) {
  return items.length
}

export function formatEsimNotificationEvent(event: string) {
  switch (event) {
    case 'install':
      return '安装'
    case 'enable':
      return '启用'
    case 'disable':
      return '禁用'
    case 'delete':
      return '删除'
    case '':
      return '未知'
    default:
      return event
  }
}

export function reconcileEsimNotificationDialogState(input: ReconcileEsimNotificationDialogInput): EsimNotificationDialogState {
  return {
    isOpen: input.isOpen,
    items: input.refreshedItems,
    retryingSequenceNumber: null,
    lastRetriedSequenceNumber: input.retriedSequenceNumber
  }
}

export function shouldShowEsimNotificationIcon(loading: boolean) {
  return !loading
}

export function shouldShowEsimRefreshIcon(loading: boolean) {
  return !loading
}

export function notificationDialogWidth() {
  return 'min(500px, 80vw)'
}

export function notificationListItemLayoutClass() {
  return 'rounded-[var(--ui-radius-lg)] border border-[var(--ui-border)] p-3 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'
}

export function notificationMetaContainerClass() {
  return 'flex flex-col gap-1 pt-1'
}

export function notificationMetaItemClass() {
  return 'w-full min-w-0 rounded-[var(--ui-radius-sm)] bg-[var(--ui-surface-muted)] px-2 py-0.5 text-xs leading-[18px] text-[var(--ui-muted)]'
}
