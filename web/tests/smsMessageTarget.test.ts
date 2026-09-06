import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { includeSmsTargetThread, parseSmsMessageTargetID } from '../src/utils/smsMessageTarget'
import type { SmsThreadVM } from '../src/types/view-model'

const thread = (key: string): SmsThreadVM => ({ key, imsi: 'shared', iccid: key, peer: 'sender', lastTs: 0, lastSmsId: 1, lastMessage: '', unreadCount: 1, peerLower: 'sender', lastMessageLower: '' })

test('a target missing from the first 200 conversations remains available', () => {
  const page = Array.from({ length: 200 }, (_, index) => thread(String(index)))
  const target = thread('late-card')
  const next = includeSmsTargetThread(page, target)
  assert.equal(next[0], target)
  assert.equal(next.length, 201)
  assert.equal(page.length, 200)
  assert.equal(includeSmsTargetThread(next, { ...target, unreadCount: 0 }).length, 201)
})

test('target IDs reject malformed and unsafe URLs', () => {
  for (const raw of [undefined, '', '0', '-1', '1junk', '1.5', ['8'], '9007199254740992']) assert.throws(() => parseSmsMessageTargetID(raw))
  assert.equal(parseSmsMessageTargetID('8'), 8)
})

test('SMS page keeps target context through polling and offers explicit latest navigation', () => {
  const source = readFileSync(new URL('../src/views/Sms.vue', import.meta.url), 'utf8')
  assert.match(source, /fetchSmsMessageTarget\(targetMessageQuery.value\)/)
  assert.match(source, /includeSmsTargetThread\(result.data, messageTarget.value.thread\)/)
  assert.match(source, /threadMessages.value = messageTarget.value.messages/)
  assert.match(source, /返回最新短信/)
  assert.match(source, /:highlighted-id=/)
})
