import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const esimTab = await readFile(
  new URL('../src/components/DeviceEsimTab.vue', import.meta.url),
  'utf8'
)

test('eSIM install leads with QR activation code, not a bare SM-DP+ field', () => {
  assert.match(esimTab, /parseEsimActivationInput/)
  assert.match(esimTab, /decodeEsimActivation/)
  assert.match(esimTab, /esimQrImage/)
  assert.match(esimTab, /激活码 \/ 二维码内容/)
  assert.match(esimTab, /识别图片 \/ PDF/)
  assert.match(esimTab, /onQrDrop/)
  assert.match(esimTab, /onQrPaste/)
  assert.match(esimTab, /LPA:1\$/)
  assert.match(esimTab, /SM-DP\+ 地址和激活码/)
  assert.doesNotMatch(esimTab, /params\.set\('activation_code'/)
})
