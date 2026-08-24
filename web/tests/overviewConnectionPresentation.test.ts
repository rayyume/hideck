import assert from 'node:assert/strict'
import test from 'node:test'
import type { DeviceOverviewItem } from '../src/types/api'
import { createOverviewConnectionPresentation } from '../src/utils/overviewConnectionPresentation'

function device(partial: Partial<DeviceOverviewItem> = {}): DeviceOverviewItem {
  return {
    id: 'wwan0',
    name: 'ct',
    running: true,
    healthy: true,
    network_connected: false,
    public_ip: '',
    network_enabled: false,
    modem: { iccid: '89860317840286845381' },
    backend_mode: 'qmi',
    interface: 'wwan1',
    ...partial
  }
}

test('volte registered is not shown as wifi calling waiting', () => {
  const presentation = createOverviewConnectionPresentation(device({
    phone_mode: 'volte',
    vowifi_enabled: true,
    vowifi_active: false,
    native_volte: {
      phase: 'registered',
      ims_registered: true,
      lte_registered: true,
      ims_pdn_active: true,
      voice_available: true,
      plmn: '460-11',
      mbn_name: 'VoLTE_OPNMKT_CT'
    }
  }))

  assert.equal(presentation.kind, 'volte')
  assert.equal(presentation.eyebrow, 'VOLTE')
  assert.equal(presentation.title, 'VoLTE 已注册')
  assert.equal(presentation.detail, '460-11 · VoLTE_OPNMKT_CT')
  assert.equal(presentation.tone, 'is-ready')
  assert.equal(presentation.pathIsFlowing, true)
  assert.deepEqual(presentation.metrics.find((item) => item.label === '接入方式'), {
    label: '接入方式',
    value: 'VoLTE',
    hint: ''
  })
  assert.deepEqual(presentation.stages.map((stage) => [stage.key, stage.ready]), [
    ['SIM', true],
    ['LTE', true],
    ['PDN', true],
    ['IMS', true],
    ['Voice', true]
  ])
})

test('volte still registering does not mark unfinished stages as failed', () => {
  const presentation = createOverviewConnectionPresentation(device({
    phone_mode: 'volte',
    vowifi_enabled: true,
    native_volte: {
      phase: 'registering',
      lte_registered: true,
      ims_pdn_active: false,
      ims_registered: false,
      provision_stage: 'verify'
    }
  }))

  assert.equal(presentation.title, 'VoLTE 正在注册')
  assert.equal(presentation.tone, 'is-pending')
  assert.deepEqual(presentation.stages.map((stage) => stage.ready), [true, true, undefined, undefined, undefined])
})

test('wifi calling presentation stays on the ePDG path', () => {
  const presentation = createOverviewConnectionPresentation(device({
    phone_mode: 'wifi',
    vowifi_enabled: true,
    vowifi_active: false
  }))

  assert.equal(presentation.kind, 'wifi')
  assert.equal(presentation.eyebrow, 'WI-FI CALLING')
  assert.equal(presentation.title, 'VoWiFi 等待连接')
  assert.equal(presentation.metrics.find((item) => item.label === '接入方式')?.value, 'Wi-Fi Calling')
})
