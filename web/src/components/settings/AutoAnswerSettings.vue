<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowUpload24Regular } from '@vicons/fluent'
import { devicesService } from '../../services/devices'
import {
  systemService,
  type AutoAnswerDeviceSettings,
  type AutoAnswerSettings
} from '../../services/system'

type DeviceRow = AutoAnswerDeviceSettings & { name: string }
const DEFAULT_AUTO_ANSWER_DELAY_SECONDS = 30

const settings = ref<AutoAnswerSettings>({ audio_ready: false, audio_name: '', devices: [] })
const rows = ref<DeviceRow[]>([])
const loading = ref(true)
const loaded = ref(false)
const savingDevices = ref(new Set<string>())
const uploading = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)

function isSaving(deviceId: string) {
  return savingDevices.value.has(deviceId)
}

function setSaving(deviceId: string, saving: boolean) {
  const next = new Set(savingDevices.value)
  if (saving) next.add(deviceId)
  else next.delete(deviceId)
  savingDevices.value = next
}

function applyDeviceSettings(next: AutoAnswerSettings) {
  settings.value = next
  const configured = new Map(next.devices.map(item => [item.device_id, item]))
  rows.value = rows.value.map(row => {
    const saved = configured.get(row.device_id)
    return saved ? { ...row, ...saved } : row
  })
}

async function load() {
  loading.value = true
  loaded.value = false
  const [phoneResult, devicesResult] = await Promise.all([
    systemService.getPhoneSettings(),
    devicesService.listManaged()
  ])
  loading.value = false
  if (!phoneResult.ok) {
    ElMessage.error(phoneResult.error.message || '自动接听设置加载失败')
    return
  }
  if (!devicesResult.ok) {
    ElMessage.error(devicesResult.error.message || '设备列表加载失败')
    return
  }
  settings.value = phoneResult.data.auto_answer
  const configured = new Map(settings.value.devices.map(item => [item.device_id, item]))
  rows.value = devicesResult.data.devices.map(device => {
    const saved = configured.get(device.id)
    return {
      device_id: device.id,
      name: device.name || device.id,
      enabled: saved?.enabled ?? false,
      delay_seconds: saved && saved.delay_seconds > 0 ? saved.delay_seconds : DEFAULT_AUTO_ANSWER_DELAY_SECONDS
    }
  })
  loaded.value = true
}

async function saveDevice(row: DeviceRow, enabled = row.enabled, delayOnly = false) {
  setSaving(row.device_id, true)
  const result = await systemService.saveAutoAnswerDevice(row.device_id, {
    enabled,
    delay_seconds: row.delay_seconds
  })
  setSaving(row.device_id, false)
  if (!result.ok) {
    ElMessage.error(result.error.message || '自动接听设置保存失败')
    await load()
    return false
  }
  row.enabled = enabled
  ElMessage.success(delayOnly ? '等待时间已保存' : enabled ? '该设备已启用自动接听' : '该设备已关闭自动接听')
  return true
}

async function toggleDevice(row: DeviceRow, value: string | number | boolean) {
  const enabled = value === true
  if (!enabled) {
    await saveDevice(row, false)
    return
  }
  if (!settings.value.audio_ready) {
    row.enabled = false
    ElMessage.warning('请先上传自动接听录音')
    return
  }
  try {
    await ElMessageBox.confirm(
      `${row.name} 来电持续 ${row.delay_seconds} 秒无人接听时，HiDeck 将自动接听、播放录音、保存对方语音，并发送到 QQ、飞书、Telegram、微信或企业微信机器人。确认启用？`,
      '启用自动接听',
      { type: 'warning', confirmButtonText: '启用', cancelButtonText: '取消' }
    )
  } catch {
    row.enabled = false
    return
  }
  await saveDevice(row, true)
}

async function upload(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file || uploading.value) return
  uploading.value = true
  const result = await systemService.uploadAutoAnswerAudio(file)
  uploading.value = false
  input.value = ''
  if (!result.ok) {
    ElMessage.error(result.error.message || '自动接听录音上传失败')
    return
  }
  applyDeviceSettings(result.data)
  ElMessage.success('自动接听录音已更新')
}

onMounted(load)
</script>

<template>
  <div class="auto-answer-settings" :aria-busy="loading || uploading || savingDevices.size > 0">
    <div class="auto-answer-heading">
      <div>
        <div class="text-sm font-bold text-[var(--ui-text)]">未接来电自动应答</div>
        <div class="text-xs leading-relaxed text-[var(--ui-muted)]">
          每台设备独立设置等待时间；默认关闭，不依赖浏览器保持在线。通话结束后，录音会发送到支持附件的 Bot 渠道。
        </div>
      </div>
    </div>

    <div class="auto-answer-audio">
      <div class="auto-answer-audio-copy">
        <strong>共用播放录音</strong>
        <p>单声道 PCM16 WAV，8 kHz 或 16 kHz，最大 16 MiB。</p>
        <span :class="settings.audio_ready ? 'is-ready' : 'is-missing'" aria-live="polite">
          {{ settings.audio_ready ? `已就绪：${settings.audio_name}` : '尚未上传录音' }}
        </span>
      </div>
      <el-button :loading="uploading" :disabled="!loaded || loading || savingDevices.size > 0" @click="fileInput?.click()">
        <el-icon><ArrowUpload24Regular /></el-icon>
        {{ uploading ? '正在上传' : '选择 WAV' }}
      </el-button>
      <input
        ref="fileInput"
        class="auto-answer-file"
        type="file"
        accept=".wav,audio/wav,audio/x-wav"
        :disabled="!loaded || loading || uploading || savingDevices.size > 0"
        @change="upload"
      >
    </div>

    <div v-if="loaded && rows.length" class="auto-answer-devices">
      <div v-for="row in rows" :key="row.device_id" class="auto-answer-device">
        <div class="auto-answer-device-heading">
          <div class="min-w-0">
            <strong>{{ row.name }}</strong>
            <span>{{ row.device_id }}</span>
          </div>
          <el-switch
            v-model="row.enabled"
            :loading="isSaving(row.device_id)"
            :disabled="loading || uploading || isSaving(row.device_id)"
            :aria-label="`${row.name} 自动接听`"
            @change="toggleDevice(row, $event)"
          />
        </div>
        <div class="auto-answer-delay">
          <label :for="`auto-answer-delay-${row.device_id}`">无人接听等待时间</label>
          <div class="auto-answer-delay-input">
            <el-input-number
              :id="`auto-answer-delay-${row.device_id}`"
              v-model="row.delay_seconds"
              :min="1"
              :step="5"
              :disabled="loading || uploading || isSaving(row.device_id)"
              controls-position="right"
            />
            <span>秒</span>
            <el-button
              :loading="isSaving(row.device_id)"
              :disabled="loading || uploading || isSaving(row.device_id)"
              @click="saveDevice(row, row.enabled, true)"
            >
              保存
            </el-button>
          </div>
        </div>
      </div>
    </div>
    <div v-else-if="loaded" class="auto-answer-empty">暂无可配置设备</div>
  </div>
</template>

<style scoped>
.auto-answer-settings { display: grid; gap: 16px; }
.auto-answer-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.auto-answer-audio, .auto-answer-device { min-width: 0; padding: 16px; border: 1px solid var(--ui-border); border-radius: var(--ui-radius-md); background: var(--ui-surface-muted); }
.auto-answer-audio { display: flex; align-items: center; gap: 12px; }
.auto-answer-audio-copy { flex: 1; min-width: 0; }
.auto-answer-audio strong, .auto-answer-device strong, .auto-answer-delay label { font-size: var(--ui-font-body-sm); font-weight: 650; color: var(--ui-text); }
.auto-answer-audio p { margin: 4px 0 0; color: var(--ui-text-muted); font-size: 13px; line-height: 1.5; }
.auto-answer-audio span { display: block; margin-top: 8px; font-size: 12px; overflow-wrap: anywhere; }
.auto-answer-audio .is-ready { color: var(--ui-success); }
.auto-answer-audio .is-missing { color: var(--ui-warning); }
.auto-answer-devices { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 12px; }
.auto-answer-device { display: grid; gap: 14px; }
.auto-answer-device-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.auto-answer-device-heading span { display: block; margin-top: 3px; color: var(--ui-text-muted); font-size: 12px; overflow-wrap: anywhere; }
.auto-answer-delay { display: grid; gap: 8px; }
.auto-answer-delay-input { display: flex; align-items: center; gap: 8px; }
.auto-answer-delay-input span { color: var(--ui-text-muted); font-size: 13px; }
.auto-answer-empty { padding: 20px; border: 1px dashed var(--ui-border); border-radius: var(--ui-radius-md); color: var(--ui-text-muted); text-align: center; }
.auto-answer-file { display: none; }
:deep(.el-button), :deep(.el-input-number) { min-height: 44px; }
@media (max-width: 720px) {
  .auto-answer-audio { align-items: stretch; flex-direction: column; }
  .auto-answer-audio .el-button { width: 100%; }
  .auto-answer-delay-input { align-items: stretch; flex-wrap: wrap; }
  .auto-answer-delay-input .el-input-number { flex: 1; min-width: 160px; }
}
</style>
