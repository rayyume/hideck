import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'
import ts from 'typescript'
import { smsService } from '../src/services/sms'
import { api } from '../src/stores/auth'

// Execute the view's real read-state handler with only its reactive state and IO supplied.
const view = readFileSync(new URL('../src/views/Sms.vue', import.meta.url), 'utf8')
const script = view.split('<script setup lang="ts">')[1].split('</script>')[0]
const source = ts.createSourceFile('Sms.ts', script, ts.ScriptTarget.Latest, true)
const handler = source.statements.find(node => ts.isFunctionDeclaration(node) && node.name?.text === 'markThreadSeen')!
const handlerJS = ts.transpileModule(handler.getText(source), { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText

for (const historical of [true, false]) {
  test(`${historical ? 'historical target preserves' : 'latest thread clears'} the unread message outside an out-of-order window`, async () => {
    const messages = [
      { id: 1, timestamp: '2026-08-13T10:00:00Z', type: 1, status: 0 },
      { id: 2, timestamp: '2026-08-13T09:00:00Z', type: 1, status: 0 }
    ]
    const thread = { key: 'iccid-1|sender', iccid: 'iccid-1', peer: 'sender', unreadCount: 2 }
    const threads = { value: [thread] }
    const calls: unknown[] = []
    const originalPatch = api.patch
    api.patch = (async (_path: string, body: { through_id?: number; message_ids?: number[] }) => {
      calls.push(body)
      let marked = 0
      for (const message of messages) {
        const selected = body.message_ids ? body.message_ids.includes(message.id) : message.id <= body.through_id!
        if (selected && message.status === 0) { message.status = 1; marked += 1 }
      }
      return { data: { marked, unread_count: messages.filter(message => message.status === 0).length } }
    }) as typeof api.patch
    try {
      const context = vm.createContext({
        threadMessages: { value: [messages[1]] }, viewingTarget: { value: historical },
        messagesError: { value: null }, threads, smsStore: smsService
      })
      const markSeen = vm.runInContext(`${handlerJS}\nmarkThreadSeen`, context)
      assert.equal(await markSeen(thread), true)
      assert.deepEqual(JSON.parse(JSON.stringify(calls)), [historical ? { message_ids: [2] } : { through_id: 2 }])
      assert.equal(messages[0].status, historical ? 0 : 1)
      assert.equal(messages[1].status, 1)
      assert.equal(threads.value[0].unreadCount, historical ? 1 : 0)
    } finally {
      api.patch = originalPatch
    }
  })
}
