import { api } from '../stores/auth'
import { callService } from './http'
import { normalizeThread } from './sms'
import { parseSmsMessageTargetID } from '../utils/smsMessageTarget'
import type { SMSMessage, SMSContact } from '../types/api'
import type { SmsThreadVM } from '../types/view-model'

export type SmsMessageTarget = {
  message: SMSMessage
  thread: SmsThreadVM
  messages: SMSMessage[]
  hasMore: boolean
}

export function fetchSmsMessageTarget(rawID: unknown) {
  return callService(async (): Promise<SmsMessageTarget> => {
    const id = parseSmsMessageTargetID(rawID)
    const { data } = await api.get<{ message: SMSMessage; contact: SMSContact; messages: SMSMessage[]; has_more: boolean }>(`/sms/messages/${id}`)
    return {
      message: data.message, thread: normalizeThread(data.contact), hasMore: data.has_more,
      messages: data.messages.slice().sort((a, b) => Date.parse(a.timestamp) - Date.parse(b.timestamp) || a.id - b.id)
    }
  })
}
