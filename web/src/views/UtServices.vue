<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import PageHeader from '../components/PageHeader.vue'
import { devicesService } from '../services/devices'
import { utService, type UtSimservs } from '../services/ut'

const devices = ref<{ id: string; name?: string }[]>([])
const deviceId = ref('')
const loading = ref(false)
const saving = ref('')
const error = ref('')
const doc = ref<UtSimservs | null>(null)
const diversionActive = ref(false)
const diversionTarget = ref('')
const restrictionActive = ref(false)
const restrictionOn = ref(false)
const incomingBarring = ref(false)
const outgoingBarring = ref(false)

const canSave = computed(() => !!deviceId.value && !!doc.value && !loading.value && !saving.value)

onMounted(async () => {
  const result = await devicesService.listManaged()
  if (!result.ok) {
    error.value = result.error.message
    return
  }
  devices.value = (result.value.devices || []).map((device) => ({
    id: device.id, name: device.name
  }))
  if (!deviceId.value && devices.value[0]) deviceId.value = devices.value[0].id
  await load()
})

watch(deviceId, () => { load() })

async function load() {
  if (!deviceId.value) return
  loading.value = true
  error.value = ''
  const result = await utService.getSimservs(deviceId.value)
  loading.value = false
  if (!result.ok) {
    error.value = utService.message(result.error)
    return
  }
  applyDoc(result.value)
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
  error.value = ''
  const result = await utService.putSimservs(deviceId.value, { etag: doc.value.etag, ...payload })
  saving.value = ''
  if (!result.ok) {
    error.value = utService.message(result.error)
    ElMessage.error(error.value)
    return
  }
  applyDoc(result.value)
  ElMessage.success('已按网络返回更新')
}
</script>

<template>
  <div class="app-page ut-page">
    <PageHeader title="补充业务" subtitle="经 Ut/XCAP 读取和修改呼叫前转、主叫限制和呼叫限制。一次只改一项。" />

    <section class="ut-card">
      <label class="ut-label" for="ut-device">设备</label>
      <select id="ut-device" class="ut-input" v-model="deviceId">
        <option v-for="device in devices" :key="device.id" :value="device.id">
          {{ device.name || device.id }}
        </option>
      </select>
      <p v-if="error" class="ut-error" role="alert">{{ error }}</p>
      <p v-else-if="loading" class="ut-hint">正在读取网络上的 simservs…</p>
      <p v-else-if="doc" class="ut-hint">XUI {{ doc.xui || '未知' }} · 版本 {{ doc.etag || '无' }}</p>
    </section>

    <section class="ut-card">
      <h2>呼叫前转</h2>
      <label class="ut-check">
        <input type="checkbox" v-model="diversionActive" />
        无条件前转
      </label>
      <label class="ut-label" for="ut-target">前转号码</label>
      <input id="ut-target" class="ut-input" v-model="diversionTarget" placeholder="tel:+44…" />
      <button type="button" class="ut-btn" :disabled="!canSave" @click="save('cdiv', { communication_diversion: { active: diversionActive, target: diversionTarget } })">
        {{ saving === 'cdiv' ? '保存中…' : '保存前转' }}
      </button>
    </section>

    <section class="ut-card">
      <h2>主叫身份限制</h2>
      <label class="ut-check">
        <input type="checkbox" v-model="restrictionActive" />
        启用 OIR
      </label>
      <label class="ut-check">
        <input type="checkbox" v-model="restrictionOn" />
        默认限制呈现
      </label>
      <button type="button" class="ut-btn" :disabled="!canSave" @click="save('oir', { identity_restriction: { active: restrictionActive, restricted: restrictionOn } })">
        {{ saving === 'oir' ? '保存中…' : '保存身份限制' }}
      </button>
    </section>

    <section class="ut-card">
      <h2>呼叫限制</h2>
      <label class="ut-check">
        <input type="checkbox" v-model="incomingBarring" />
        限制呼入
      </label>
      <button type="button" class="ut-btn" :disabled="!canSave" @click="save('icb', { incoming_barring: { active: incomingBarring } })">
        {{ saving === 'icb' ? '保存中…' : '保存呼入限制' }}
      </button>
      <label class="ut-check">
        <input type="checkbox" v-model="outgoingBarring" />
        限制呼出
      </label>
      <button type="button" class="ut-btn" :disabled="!canSave" @click="save('ocb', { outgoing_barring: { active: outgoingBarring } })">
        {{ saving === 'ocb' ? '保存中…' : '保存呼出限制' }}
      </button>
    </section>
  </div>
</template>

<style scoped>
.ut-page { max-width: 720px; }
.ut-card {
  margin-bottom: 16px;
  padding: 16px;
  border: 1px solid var(--ui-border, #d4d4d8);
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.ut-card h2 { margin: 0; font-size: 16px; }
.ut-label { font-size: 13px; font-weight: 600; }
.ut-input, .ut-btn {
  min-height: 44px;
  padding: 0 12px;
  border-radius: 8px;
  border: 1px solid var(--ui-border, #d4d4d8);
}
.ut-btn { align-self: flex-start; cursor: pointer; }
.ut-btn:disabled { opacity: 0.6; cursor: not-allowed; }
.ut-check { display: flex; align-items: center; gap: 8px; min-height: 44px; }
.ut-error { color: #b42318; margin: 0; }
.ut-hint { margin: 0; color: #71717a; font-size: 13px; }
</style>
