import { onUnmounted, ref } from 'vue'
import { usePollingScheduler } from './usePollingScheduler'
import { fetchSmsNotifications } from '../services/sms-notifications'
import { mergeSmsNotice, type SmsNotice } from '../utils/smsNotifications'

const SMS_NOTIFICATION_INTERVAL_MS = 5_000
const SMS_NOTIFICATION_BACKGROUND_INTERVAL_MS = 15_000

export function useSmsNotifications() {
  const unreadCount = ref<number | null>(null)
  const notice = ref<SmsNotice | null>(null)
  const error = ref('')
  const controller = new AbortController()
  let cursor: number | null = null

  async function refresh() {
    const result = await fetchSmsNotifications(cursor, controller.signal)
    if (controller.signal.aborted) return
    if (!result.ok) {
      error.value = result.error.message
      throw new Error(result.error.message)
    }
    error.value = ''
    unreadCount.value = result.data.unread_count
    notice.value = mergeSmsNotice(notice.value, result.data)
    cursor = result.data.cursor
    if (result.data.unread_count === 0) notice.value = null
  }

  const polling = usePollingScheduler(refresh, SMS_NOTIFICATION_INTERVAL_MS, {
    immediate: true, backgroundIntervalMs: SMS_NOTIFICATION_BACKGROUND_INTERVAL_MS
  })
  onUnmounted(() => controller.abort())

  return { unreadCount, notice, error, retry: polling.trigger, dismiss: () => { notice.value = null } }
}
