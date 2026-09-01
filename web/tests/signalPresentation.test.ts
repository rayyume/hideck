import assert from 'node:assert/strict'
import test from 'node:test'
import { displaySignalDbm, hasValidSignalDbm } from '../src/utils/signalPresentation.ts'

test('treats QMI LTE placeholder RSSI as missing and falls back to RSRP', () => {
  assert.equal(hasValidSignalDbm(-125), false)
  assert.equal(hasValidSignalDbm(-128), false)
  assert.equal(hasValidSignalDbm(0), false)
  assert.equal(hasValidSignalDbm(-86), true)
  assert.equal(displaySignalDbm(-125, -86), -86)
  assert.equal(displaySignalDbm(-72, -86), -72)
  assert.equal(displaySignalDbm(-125, 0), undefined)
})
