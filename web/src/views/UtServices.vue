<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import {
  ArrowClockwise24Regular,
  CallForward24Regular,
  EyeOff24Regular,
  Phone24Regular,
  CallProhibited24Regular,
  Save24Regular
} from '@vicons/fluent'
import ErrorState from '../components/ErrorState.vue'
import EmptyState from '../components/EmptyState.vue'
import PageHeader from '../components/PageHeader.vue'
import { devicesService } from '../services/devices'
import { utService, type UtSimservs } from '../services/ut'
import type { AppError } from '../types/domain'
import { createLatestRequestGate } from '../utils/latestRequestGate'
import { deviceSupportsUt } from '../utils/phoneMode'

const devices = ref<{ id: string; name?: string }[]>([])
const managedCount = ref(0)
const deviceId = ref('')
const loading = ref(false)
const saving = ref('')
const listError = ref<AppError | null>(null)
const loadError = ref<AppError | null>(null)
const doc = ref<UtSimservs | null>(null)
const diversionActive = ref(false)
const diversionTarget = ref('')
const restrictionActive = ref(false)
const restrictionOn = ref(false)
const incomingBarring = ref(false)
const outgoingBarring = ref(false)
const deviceRequest = createLatestRequestGate()
const simservsRequest = createLatestRequestGate()

const pageError = computed(() => listError.value || loadError.value)
const canSave = computed(() => !!deviceId.value && !!doc.value && !loading.value && !saving.value)
const identityLabel = computed(() => maskUtIdentity(doc.value?.xui))
const documentVersion = computed(() => doc.value?.etag || '无')
const emptyTitle = computed(() => managedCount.value ? '没有使用 WiFi calling 的设备' : '暂无设备')
const emptySubtitle = computed(() => managedCount.value
  ? '呼叫设置只对已开启 WiFi calling 的设备有效，未开启的不会出现在这里'
  : '接入模组并开启 WiFi calling 后才能读取网络上的呼叫设置')

onMounted(() => {
  void loadDevices()
})

watch(deviceId, () => {
  void loadSimservs()
})

async function retry() {
  if (listError.value || !devices.value.length) {
    await loadDevices()
    return
  }
  await loadSimservs()
}

async function loadDevices() {
  const request = deviceRequest.begin()
  loading.value = true
  listError.value = null
  const result = await devicesService.listManaged()
  if (!request.isCurrent()) return
  if (!result.ok) {
    loading.value = false
    listError.value = result.error
    return
  }
  const all = result.data.devices || []
  managedCount.value = all.length
  devices.value = all.filter(deviceSupportsUt).map((device) => ({
    id: device.id,
    name: device.name
  }))
  if (!devices.value.some((device) => device.id === deviceId.value)) {
    deviceId.value = devices.value[0]?.id || ''
    if (deviceId.value) return
  }
  loading.value = false
  if (deviceId.value) await loadSimservs()
}

async function loadSimservs() {
  if (!deviceId.value) {
    loading.value = false
    return
  }
  const request = simservsRequest.begin()
  loading.value = true
  loadError.value = null
  doc.value = null
  const result = await utService.getSimservs(deviceId.value)
  if (!request.isCurrent()) return
  loading.value = false
  if (!result.ok) {
    loadError.value = result.error
    return
  }
  applyDoc(result.data)
}

function maskUtIdentity(xui?: string) {
  const value = (xui || '').trim()
  if (!value) return '未注册'
  const at = value.lastIndexOf('@')
  if (at <= 4) return '已注册'
  return value.slice(0, 4) + '***' + value.slice(at)
}

function applyDoc(next: UtSimservs) {
  doc.value = next
  diversionActive.value = !!next.communication_diversion?.active
  diversionTarget.value = next.communication_diversion?.target || ''
  restrictionActive.value = !!next.identity_restriction?.active
  restrictionOn.value = !!next.identity_restriction?.restricted
  incomingBarring.value = !!next.incoming_barring?.active
  outgoingBarring.value = !!next.outgoing_barring?.active
}

async function save(kind: string, payload: Record<string, unknown>) {
  if (!doc.value || !deviceId.value) return
  saving.value = kind
  const result = await utService.putSimservs(deviceId.value, { etag: doc.value.etag, ...payload })
  saving.value = ''
  if (!result.ok) {
    ElMessage.error(utService.message(result.error))
    return
  }
  applyDoc(result.data)
  ElMessage.success('已按网络返回更新')
}
</script>

<template>
  <div class="app-page ut-page">
    <PageHeader title="呼叫设置" subtitle="设置呼叫前转、主叫号码显示和呼叫限制。一次只改一项。">
      <template #actions>
        <el-button :loading="loading" :disabled="loading" @click="retry">
          <el-icon v-if="!loading"><ArrowClockwise24Regular /></el-icon>
          刷新
        </el-button>
      </template>
    </PageHeader>

    <section class="ut-workspace ui-card ui-workspace-glow">
      <header class="ut-toolbar">
        <div class="ut-device">
          <label for="ut-device">设备</label>
          <el-select
            id="ut-device"
            v-model="deviceId"
            aria-label="呼叫设置设备"
            placeholder="选择设备"
            :disabled="!devices.length || loading"
          >
            <el-option v-if="!devices.length" label="无可用设备" value="" />
            <el-option
              v-for="device in devices"
              :key="device.id"
              :label="device.name || device.id"
              :value="device.id"
            />
          </el-select>
        </div>
        <div class="ut-facts">
          <span class="ut-fact">IMS 身份 <strong>{{ identityLabel }}</strong></span>
          <span class="ut-fact">文档版本 <strong>{{ documentVersion }}</strong></span>
        </div>
      </header>

      <ErrorState
        v-if="pageError"
        class="ut-alert"
        title="呼叫设置不可用"
        :message="pageError.message"
        :status-code="pageError.status"
        :request-method="pageError.method"
        :request-url="pageError.url"
        retry-text="重试"
        @retry="retry"
      />

      <EmptyState
        v-else-if="!loading && !devices.length"
        compact
        :title="emptyTitle"
        :subtitle="emptySubtitle"
      >
        <template #icon>
          <el-icon><Phone24Regular /></el-icon>
        </template>
      </EmptyState>

      <p v-else-if="loading && !doc" class="ut-loading">正在读取网络上的呼叫设置…</p>

      <div v-show="devices.length" class="ut-panels">
        <section class="ut-panel" aria-labelledby="ut-cdiv-title">
          <header class="ut-panel-header">
            <div class="section-icon section-icon-communication">
              <el-icon size="22"><CallForward24Regular /></el-icon>
            </div>
            <div>
              <h2 id="ut-cdiv-title">呼叫前转</h2>
              <p>把来电转到指定号码</p>
            </div>
          </header>
          <div class="ut-setting-list">
            <div class="ut-setting-row" :class="{ 'is-active': diversionActive }">
              <span>
                <strong>无条件前转</strong>
                <small>所有来电都转到下面的号码</small>
              </span>
              <el-switch v-model="diversionActive" :disabled="!canSave" aria-label="无条件前转" />
            </div>
            <div class="ut-setting-row">
              <span>
                <strong>前转号码</strong>
                <small>使用 tel:+ 国际格式</small>
              </span>
              <div class="ut-field-control">
                <el-input
                  id="ut-target"
                  v-model="diversionTarget"
                  placeholder="tel:+44…"
                  :disabled="!canSave"
                />
              </div>
            </div>
          </div>
          <footer class="ut-panel-footer">
            <small>保存后由运营商网络生效</small>
            <el-button type="primary" class="!border-0" :loading="saving === 'cdiv'" :disabled="!canSave" @click="save('cdiv', { communication_diversion: { active: diversionActive, target: diversionTarget } })">
              <el-icon v-if="saving !== 'cdiv'"><Save24Regular /></el-icon>
              {{ saving === 'cdiv' ? '保存中…' : '保存前转' }}
            </el-button>
          </footer>
        </section>

        <section class="ut-panel" aria-labelledby="ut-oir-title">
          <header class="ut-panel-header">
            <div class="section-icon section-icon-primary">
              <el-icon size="22"><EyeOff24Regular /></el-icon>
            </div>
            <div>
              <h2 id="ut-oir-title">主叫号码显示</h2>
              <p>对方是否看到你的号码</p>
            </div>
          </header>
          <div class="ut-setting-list">
            <div class="ut-setting-row" :class="{ 'is-active': restrictionActive }">
              <span>
                <strong>启用主叫限制</strong>
                <small>打开后可隐藏主叫号码</small>
              </span>
              <el-switch v-model="restrictionActive" :disabled="!canSave" aria-label="启用主叫限制" />
            </div>
            <div class="ut-setting-row" :class="{ 'is-active': restrictionOn }">
              <span>
                <strong>默认隐藏号码</strong>
                <small>限制呈现时对端看不到来电号码</small>
              </span>
              <el-switch v-model="restrictionOn" :disabled="!canSave" aria-label="默认隐藏号码" />
            </div>
          </div>
          <footer class="ut-panel-footer">
            <small>一次只改这一项</small>
            <el-button type="primary" class="!border-0" :loading="saving === 'oir'" :disabled="!canSave" @click="save('oir', { identity_restriction: { active: restrictionActive, restricted: restrictionOn } })">
              <el-icon v-if="saving !== 'oir'"><Save24Regular /></el-icon>
              {{ saving === 'oir' ? '保存中…' : '保存号码显示' }}
            </el-button>
          </footer>
        </section>

        <section class="ut-panel" aria-labelledby="ut-barring-title">
          <header class="ut-panel-header">
            <div class="section-icon section-icon-success">
              <el-icon size="22"><CallProhibited24Regular /></el-icon>
            </div>
            <div>
              <h2 id="ut-barring-title">呼叫限制</h2>
              <p>分别限制呼入或呼出</p>
            </div>
          </header>
          <div class="ut-setting-list">
            <div class="ut-setting-row" :class="{ 'is-active': incomingBarring }">
              <span>
                <strong>限制呼入</strong>
                <small>打开后不再接通来电</small>
              </span>
              <div class="ut-inline-action">
                <el-switch v-model="incomingBarring" :disabled="!canSave" aria-label="限制呼入" />
                <el-button type="primary" class="!border-0" :loading="saving === 'icb'" :disabled="!canSave" @click="save('icb', { incoming_barring: { active: incomingBarring } })">
                  {{ saving === 'icb' ? '保存中…' : '保存呼入' }}
                </el-button>
              </div>
            </div>
            <div class="ut-setting-row" :class="{ 'is-active': outgoingBarring }">
              <span>
                <strong>限制呼出</strong>
                <small>打开后不能再拨出电话</small>
              </span>
              <div class="ut-inline-action">
                <el-switch v-model="outgoingBarring" :disabled="!canSave" aria-label="限制呼出" />
                <el-button type="primary" class="!border-0" :loading="saving === 'ocb'" :disabled="!canSave" @click="save('ocb', { outgoing_barring: { active: outgoingBarring } })">
                  {{ saving === 'ocb' ? '保存中…' : '保存呼出' }}
                </el-button>
              </div>
            </div>
          </div>
        </section>
      </div>
    </section>
  </div>
</template>

<style scoped>
.ut-page { min-width: 0; }
.ut-workspace { min-width: 0; overflow: hidden; }
.ut-toolbar {
  min-height: 82px;
  padding: 16px 18px;
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid var(--ui-border);
}
.ut-device {
  min-width: 0;
  width: min(360px, 100%);
  display: grid;
  gap: 6px;
}
.ut-device label,
.ut-fact {
  color: var(--ui-primary);
  font: 700 var(--ui-font-caption) "v-mono", ui-monospace, monospace;
  letter-spacing: .14em;
  text-transform: uppercase;
}
.ut-facts {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}
.ut-fact {
  min-height: 34px;
  padding: 0 12px;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  border: 1px solid var(--ui-border);
  border-radius: 10px;
  background: color-mix(in srgb, var(--ui-surface-strong) 84%, transparent);
}
.ut-fact strong {
  color: var(--ui-text);
  font: var(--ui-font-body-sm) "v-mono", ui-monospace, monospace;
  letter-spacing: 0;
  text-transform: none;
}
.ut-alert { margin: 16px 18px 0; }
.ut-loading {
  margin: 0;
  padding: 28px 18px;
  color: var(--ui-text-muted);
  font-size: var(--ui-font-body-sm);
}
.ut-panels {
  padding: 16px 18px 18px;
  display: grid;
  gap: 12px;
}
.ut-panel {
  overflow: hidden;
  border: 1px solid var(--ui-border);
  border-radius: 12px;
  background: var(--ui-surface-strong);
  animation: ut-panel-enter 240ms var(--ui-ease-out) both;
}
.ut-panel-header {
  min-height: 82px;
  padding: 14px 16px;
  display: flex;
  align-items: center;
  gap: 12px;
  border-bottom: 1px solid var(--ui-border);
}
.ut-panel-header h2 {
  margin: 3px 0 0;
  color: var(--ui-text);
  font-size: 20px;
  font-weight: 650;
}
.ut-panel-header p {
  margin: 3px 0 0;
  color: var(--ui-text-muted);
  font-size: var(--ui-font-body-sm);
}
.ut-setting-list { padding: 0 16px; }
.ut-setting-row {
  min-height: 72px;
  padding: 13px 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  border-bottom: 1px solid var(--ui-border-muted);
}
.ut-setting-row:last-child { border-bottom: 0; }
.ut-setting-row.is-active {
  background: linear-gradient(90deg, transparent, color-mix(in srgb, var(--ui-primary) 5%, transparent));
}
.ut-setting-row > span {
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.ut-setting-row > span strong { color: var(--ui-text); font-size: 13px; }
.ut-setting-row > span small {
  margin-top: 3px;
  color: var(--ui-text-muted);
  font-size: var(--ui-font-body-sm);
}
.ut-field-control { width: min(360px, 52%); }
.ut-inline-action {
  display: flex;
  align-items: center;
  gap: 10px;
}
.ut-setting-row :deep(.el-switch) {
  min-width: 44px;
  min-height: 44px;
  justify-content: center;
}
.ut-page :deep(.page-actions .el-button),
.ut-panel-footer :deep(.el-button) {
  min-height: 40px;
}
.ut-panel-footer {
  min-height: 74px;
  padding: 13px 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  background: var(--ui-surface-muted);
}
.ut-panel-footer small {
  color: var(--ui-text-muted);
  font-size: var(--ui-font-caption);
}

@keyframes ut-panel-enter {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: translateY(0); }
}

@media (max-width: 820px) {
  .ut-toolbar,
  .ut-setting-row,
  .ut-panel-footer {
    align-items: stretch;
    flex-direction: column;
  }
  .ut-device,
  .ut-field-control { width: 100%; }
  .ut-facts { justify-content: flex-start; }
  .ut-inline-action { justify-content: space-between; }
  .ut-panel-footer small { text-align: left; }
}

@media (prefers-reduced-motion: reduce) {
  .ut-panel {
    animation: none;
  }
}
</style>
