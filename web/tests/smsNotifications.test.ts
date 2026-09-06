import assert from 'node:assert/strict'
import test from 'node:test'
import { mergeSmsNotice, parseSmsNotificationSnapshot, smsNotificationBadge, smsNotificationTarget } from '../src/utils/smsNotifications'
import type { SMSMessage } from '../src/types/api'

const message: SMSMessage = { id: 8, iccid: 'card-A', imsi: 'shared-imsi', peer: '+44123', sender: '+44123', content: '<script>text</script>', type: 1, timestamp: '2026-09-06T10:00:00Z' }

test('initial snapshot and repeated cursor do not create notifications', () => {
  const initial = { cursor: 8, unread_count: 4, new_count: 0 }
  assert.equal(mergeSmsNotice(null, initial), null)
  const current = { count: 2, latest: message }
  assert.equal(mergeSmsNotice(current, initial), current)
})

test('aggregates arrivals into one notice without dropping the count', () => {
  const next = mergeSmsNotice({ count: 2, latest: message }, { cursor: 10, unread_count: 207, new_count: 205, latest: { ...message, id: 10 } })
  assert.equal(next?.count, 207)
  assert.equal(next?.latest.id, 10)
  assert.equal(next?.latest.content, message.content)
})

test('links to the exact SIM conversation and falls back to the inbox when identity is absent', () => {
  assert.deepEqual(smsNotificationTarget(message), { path: '/sms', query: { contact: 'card-A|+44123' } })
  assert.deepEqual(smsNotificationTarget({ ...message, iccid: 'card-B' }), { path: '/sms', query: { contact: 'card-B|+44123' } })
  assert.deepEqual(smsNotificationTarget(), { path: '/sms', query: {} })
})

test('badge keeps unknown distinct from unread and abbreviates display only', () => {
  assert.equal(smsNotificationBadge(null), '')
  assert.equal(smsNotificationBadge(0), '')
  assert.equal(smsNotificationBadge(3), '3')
  assert.equal(smsNotificationBadge(120), '99+')
})

test('rejects malformed API snapshots instead of silently displaying an empty inbox', () => {
  for (const value of [null, {}, { cursor: -1, unread_count: 0, new_count: 0 }, { cursor: 1, unread_count: 1, new_count: 1 }]) {
    assert.throws(() => parseSmsNotificationSnapshot(value))
  }
  const snapshot = { cursor: 8, unread_count: 1, new_count: 1, latest: message }
  assert.equal(parseSmsNotificationSnapshot(snapshot), snapshot)
})
