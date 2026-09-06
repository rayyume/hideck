import { api } from '../stores/auth'
import { callService } from './http'
import { parseSmsNotificationSnapshot } from '../utils/smsNotifications'

export function fetchSmsNotifications(cursor: number | null, signal: AbortSignal) {
  return callService(async () => {
    const response = await api.get<unknown>('/sms/notifications', {
      params: cursor === null ? {} : { after_id: cursor }, signal
    })
    return parseSmsNotificationSnapshot(response.data)
  })
}
