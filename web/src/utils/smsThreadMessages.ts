import type { SMSMessage } from '../types/api'

export const SMS_THREAD_PAGE_SIZE = 30

export function parseSmsTimestamp(value: string) {
  const ms = Date.parse(value)
  return Number.isFinite(ms) ? ms : 0
}

export function mergeSmsThreadPages(current: readonly SMSMessage[], incoming: readonly SMSMessage[]): SMSMessage[] {
  if (incoming.length === 0) return current.slice()
  if (current.length === 0) return incoming.slice()
  const byId = new Map<number, SMSMessage>()
  for (const message of current) byId.set(message.id, message)
  for (const message of incoming) byId.set(message.id, message)
  return [...byId.values()].sort((a, b) => (
    parseSmsTimestamp(a.timestamp) - parseSmsTimestamp(b.timestamp) || a.id - b.id
  ))
}

export function threadAlreadyHasLatest(messages: readonly Pick<SMSMessage, 'id'>[], lastSmsId: number) {
  if (!Number.isInteger(lastSmsId) || lastSmsId <= 0 || messages.length === 0) return false
  return messages.some(message => message.id === lastSmsId)
}
