import type { SmsThreadVM } from '../types/view-model'

export function parseSmsMessageTargetID(raw: unknown): number {
  if (typeof raw !== 'string' || !/^\d+$/.test(raw)) throw new Error('提醒链接中的短信 ID 无效')
  const id = Number(raw)
  if (!Number.isSafeInteger(id) || id <= 0) throw new Error('提醒链接中的短信 ID 无效')
  return id
}

export function includeSmsTargetThread(threads: readonly SmsThreadVM[], target: SmsThreadVM): SmsThreadVM[] {
  return [target, ...threads.filter(thread => thread.key !== target.key)]
}
