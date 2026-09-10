import assert from 'node:assert/strict'
import test from 'node:test'
import type { SMSMessage } from '../src/types/api'
import {
  SMS_THREAD_PAGE_SIZE,
  mergeSmsThreadPages,
  threadAlreadyHasLatest
} from '../src/utils/smsThreadMessages'

function message(id: number, timestamp: string, content = `sms-${id}`): SMSMessage {
  return { id, sender: 'Vodafone', content, type: 1, status: 1, timestamp }
}

test('SMS thread pages stay small enough to open without rendering the full history', () => {
  assert.equal(SMS_THREAD_PAGE_SIZE, 30)
  assert.ok(SMS_THREAD_PAGE_SIZE < 80)
})

test('merge keeps older loaded history and replaces the same id with the newer copy', () => {
  const older = [message(1, '2026-09-08T15:00:00Z'), message(2, '2026-09-08T16:00:00Z')]
  const incoming = [message(2, '2026-09-08T16:00:00Z', 'updated'), message(3, '2026-09-08T17:00:00Z')]
  const merged = mergeSmsThreadPages(older, incoming)
  assert.deepEqual(merged.map(item => [item.id, item.content]), [
    [1, 'sms-1'],
    [2, 'updated'],
    [3, 'sms-3']
  ])
})

test('a silent poll skips the thread request when the latest id is already on screen', () => {
  const page = [message(8, '2026-09-08T18:00:00Z'), message(9, '2026-09-08T19:00:00Z')]
  assert.equal(threadAlreadyHasLatest(page, 9), true)
  assert.equal(threadAlreadyHasLatest(page, 10), false)
  assert.equal(threadAlreadyHasLatest([], 9), false)
  assert.equal(threadAlreadyHasLatest(page, 0), false)
})
