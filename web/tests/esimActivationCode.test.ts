import assert from 'node:assert/strict'
import test from 'node:test'
import { looksLikeEsimActivationCode, parseEsimActivationInput } from '../src/utils/esimActivationCode'

test('parses a standard LPA activation code used by VOXI and Giffgaff', () => {
  const parsed = parseEsimActivationInput('LPA:1$vfgb.esim.vodafone.com$JN-ABCDE-12345')
  assert.deepEqual(parsed, {
    smdp: 'vfgb.esim.vodafone.com',
    matchingId: 'JN-ABCDE-12345',
    oid: '',
    confirmationRequired: false,
    confirmationCode: ''
  })
  assert.deepEqual(parseEsimActivationInput('LPA:1$rsp.truphone.com$GG-MATCH-1')?.smdp, 'rsp.truphone.com')
})

test('parses Apple wrapped, host$token, and labeled carrier emails', () => {
  const apple = parseEsimActivationInput(
    'https://esimsetup.apple.com/esim_qrcode_provisioning?carddata=LPA:1%24rsp.truphone.com%24MATCH-1'
  )
  assert.equal(apple?.smdp, 'rsp.truphone.com')
  assert.equal(apple?.matchingId, 'MATCH-1')

  const hostToken = parseEsimActivationInput('rsp.redtea.io$RT-TOKEN-9')
  assert.equal(hostToken?.smdp, 'rsp.redtea.io')
  assert.equal(hostToken?.matchingId, 'RT-TOKEN-9')

  const labeled = parseEsimActivationInput('SM-DP+ Address: smdp.esim.wo.com.cn\n激活码: CU-TOKEN-22\n确认码: 654321')
  assert.equal(labeled?.smdp, 'smdp.esim.wo.com.cn')
  assert.equal(labeled?.matchingId, 'CU-TOKEN-22')
  assert.equal(labeled?.confirmationCode, '654321')
  assert.equal(labeled?.confirmationRequired, true)
})

test('parses a confirmation-required activation code', () => {
  const parsed = parseEsimActivationInput('LPA:1$smdp.example.com$token$$1')
  assert.equal(parsed?.smdp, 'smdp.example.com')
  assert.equal(parsed?.matchingId, 'token')
  assert.equal(parsed?.confirmationRequired, true)
})

test('parses two-line host plus matching ID', () => {
  const parsed = parseEsimActivationInput('consumer.e-sim.global\nAIRALO-TOKEN')
  assert.equal(parsed?.smdp, 'consumer.e-sim.global')
  assert.equal(parsed?.matchingId, 'AIRALO-TOKEN')
})

test('treats a plain SM-DP+ host as a download address', () => {
  const parsed = parseEsimActivationInput('https://rsp.truphone.com')
  assert.equal(parsed?.smdp, 'rsp.truphone.com')
  assert.equal(parsed?.matchingId, '')
})

test('prefers an LPA token over a labeled SM-DP+ host in the same paste', () => {
  const mixed = parseEsimActivationInput('SM-DP+ Address: rsp.truphone.com\n\nScan this QR:\nLPA:1$rsp.truphone.com$GG-MATCH-1')
  assert.equal(mixed?.smdp, 'rsp.truphone.com')
  assert.equal(mixed?.matchingId, 'GG-MATCH-1')

  const labeledLPA = parseEsimActivationInput('SM-DP+ Address: rsp.truphone.com\nActivation Code: LPA:1$rsp.truphone.com$GG-MATCH-1')
  assert.equal(labeledLPA?.matchingId, 'GG-MATCH-1')
})

test('rejects unrelated text', () => {
  assert.equal(parseEsimActivationInput('hello voxi'), null)
  assert.equal(looksLikeEsimActivationCode('LPA:1$smdp.example.com$abc'), true)
  assert.equal(looksLikeEsimActivationCode('rsp.truphone.com$abc'), true)
  assert.equal(looksLikeEsimActivationCode('consumer.e-sim.global\nAIRALO-TOKEN'), true)
  assert.equal(looksLikeEsimActivationCode('use the SM-DP+ and Matching ID below'), false)
  assert.equal(looksLikeEsimActivationCode('rsp.truphone.com'), false)
})
