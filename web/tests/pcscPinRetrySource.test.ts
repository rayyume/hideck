import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const component = await readFile(
  new URL('../src/components/DeviceConfigTab.vue', import.meta.url),
  'utf8'
)
const view = await readFile(new URL('../src/views/Devices.vue', import.meta.url), 'utf8')
const service = await readFile(new URL('../src/services/devices.ts', import.meta.url), 'utf8')

test('PCSC PIN retry is an explicit confirmed action', () => {
  assert.match(component, /v-if="isPCSCBackend"[^>]+@click="emit\('retryPin'\)"/)
  assert.match(view, /如果 PIN 仍不正确，可能消耗一次剩余尝试次数/)
  assert.match(view, /if \(editDirty\.value\)/)
  assert.match(service, /\/devices\/\$\{id\}\/actions\/retry-sim-pin/)
})
