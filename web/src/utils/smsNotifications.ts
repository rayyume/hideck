import type { SMSMessage } from '../types/api'

export type SmsNotificationSnapshot = {
  cursor: number
  unread_count: number
  new_count: number
  latest?: SMSMessage
}

export type SmsNotice = { count: number; latest: SMSMessage }

export function parseSmsNotificationSnapshot(value: unknown): SmsNotificationSnapshot {
  const data = value as Partial<SmsNotificationSnapshot> | null
  const validCount = (count: unknown) => typeof count === 'number' && Number.isSafeInteger(count) && count >= 0
  if (!data || !validCount(data.cursor) || !validCount(data.unread_count) || !validCount(data.new_count)) {
    throw new Error('短信提醒返回了无效的数量或游标')
  }
  if (data.new_count! > 0 && (!data.latest || !validCount(data.latest.id) || data.latest.type !== 1 ||
    typeof data.latest.content !== 'string' || typeof data.latest.sender !== 'string')) {
    throw new Error('短信提醒返回了无效的来信内容')
  }
  return data as SmsNotificationSnapshot
}

export function smsNotificationTarget(message?: SMSMessage) {
  const identity = message?.iccid || message?.imsi
  const peer = message?.peer || message?.sender
  return { path: '/sms', query: identity && peer && message ? { contact: `${identity}|${peer}`, message: String(message.id) } : {} }
}

export function mergeSmsNotice(current: SmsNotice | null, snapshot: SmsNotificationSnapshot): SmsNotice | null {
  if (!snapshot.latest || snapshot.new_count <= 0) return current
  return { count: (current?.count || 0) + snapshot.new_count, latest: snapshot.latest }
}

export function smsNotificationBadge(count: number | null) {
  if (count === null) return ''
  return count > 99 ? '99+' : count > 0 ? String(count) : ''
}
