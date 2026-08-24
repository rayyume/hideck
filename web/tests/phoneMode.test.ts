import assert from 'node:assert/strict'
import test from 'node:test'
import { isNativeVoLTEMode, isWifiCallingEnabled, phoneModeCampsOnCell, phoneModeLabel } from '../src/utils/phoneMode'

test('native volte is only the volte phone mode', () => {
  assert.equal(isNativeVoLTEMode('volte'), true)
  assert.equal(isNativeVoLTEMode('cellular'), false)
  assert.equal(isNativeVoLTEMode('wifi'), false)
})

test('volte and cellular camp on cell; wifi does not', () => {
  assert.equal(phoneModeCampsOnCell('volte'), true)
  assert.equal(phoneModeCampsOnCell('cellular'), true)
  assert.equal(phoneModeCampsOnCell('wifi'), false)
  assert.equal(phoneModeCampsOnCell(undefined), false)
})

test('wifi calling is only wifi mode with phone on', () => {
  assert.equal(isWifiCallingEnabled('wifi', true), true)
  assert.equal(isWifiCallingEnabled('volte', true), false)
  assert.equal(isWifiCallingEnabled('cellular', true), false)
  assert.equal(isWifiCallingEnabled('wifi', false), false)
})

test('phone mode labels', () => {
  assert.equal(phoneModeLabel('volte'), 'VoLTE')
  assert.equal(phoneModeLabel('cellular'), '蜂窝数据')
  assert.equal(phoneModeLabel('wifi'), 'WiFi calling')
})
