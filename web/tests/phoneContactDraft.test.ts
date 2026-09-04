import assert from 'node:assert/strict'
import test from 'node:test'
import {
  isPhoneContactNumberValid,
  normalizePhoneContactNumber,
  phoneContactNumberError,
  validPhoneContactNumbers
} from '../src/utils/phoneContactDraft.ts'

test('contact numbers are trimmed without silently deleting invalid characters', () => {
  assert.equal(normalizePhoneContactNumber('  +447911123456  '), '+447911123456')
  assert.equal(normalizePhoneContactNumber('138abc'), '138abc')
  assert.equal(isPhoneContactNumberValid('138abc'), false)
  assert.equal(phoneContactNumberError(['138abc']), '号码只能是数字，可带开头的 +')
})

test('manual contact numbers reject invalid extras and remove exact duplicates', () => {
  assert.deepEqual(validPhoneContactNumbers('+447911123456', ['10086', '+447911123456', '']), [
    '+447911123456',
    '10086'
  ])
  assert.deepEqual(validPhoneContactNumbers('+447911123456', ['100-86']), [])
  assert.deepEqual(validPhoneContactNumbers('', ['10086']), [])
})
