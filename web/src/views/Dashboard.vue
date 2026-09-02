<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import WorkspacePreviewCard from '../components/WorkspacePreviewCard.vue'
import ListSkeleton from '../components/ListSkeleton.vue'
import ErrorState from '../components/ErrorState.vue'
import RefreshButton from '../components/RefreshButton.vue'
import TrafficAnalysisPanel from '../components/TrafficAnalysisPanel.vue'
import { usePollingScheduler } from '../composables/usePollingScheduler'
import { useDashboardStore } from '../stores/dashboard'
import type { TrafficRange } from '../services/traffic'
import { formatPlmnOperatorLabel, getMccMncIndex, type MccMncRow } from '../utils/mcc-mnc'
import { createLiveWorkspacePreview } from '../utils/workspacePreview'

const dashboard = useDashboardStore()
const router = useRouter()
const {
  devices,
  devicesLoading: loading,
  devicesLastOkAt,
  devicesError,
  analysis,
  analysisLoading,
  analysisLastOkAt,
  analysisError
} = storeToRefs(dashboard)

const analysisRange = ref<TrafficRange>('day')
const selectedDeviceID = ref('')
const mccMncIndex = ref<Map<string, MccMncRow> | null>(null)

const labeledDevices = computed(() => devices.value.map((device) => {
  const operator = formatPlmnOperatorLabel(device.operator || '', mccMncIndex.value)
  if (!operator || operator === (device.operator || '')) return device
  return { ...device, operator }
}))
const selectedDevice = computed(() => {
  return labeledDevices.value.find((device) => device.id === selectedDeviceID.value)
    || labeledDevices.value.find((device) => device.healthy)
    || labeledDevices.value[0]
})
const preview = computed(() => createLiveWorkspacePreview(
  labeledDevices.value,
  selectedDevice.value?.id || '',
  new Date()
))

async function fetchDevices() {
  await dashboard.fetchDevices()
}

async function fetchTrafficAnalysis() {
  await dashboard.fetchAnalysis(analysisRange.value)
}

function handleAnalysisRangeChange(range: TrafficRange) {
  if (analysisRange.value === range) return
  analysisRange.value = range
  void fetchTrafficAnalysis()
}

function openDeviceOverview(id?: string) {
  const deviceID = String(id || selectedDevice.value?.id || '').trim()
  if (!deviceID) {
    void router.push({ name: 'Devices' })
    return
  }
  void router.push({
    name: 'Devices',
    query: {
      device: deviceID,
      tab: 'overview'
    }
  })
}

function selectDevice(id: string) {
  selectedDeviceID.value = id
}

usePollingScheduler(fetchDevices, 5000, {
  immediate: true,
  maxIntervalMs: 30000,
  backgroundIntervalMs: 15000
})
usePollingScheduler(fetchTrafficAnalysis, 60000, {
  immediate: false,
  maxIntervalMs: 300000,
  backgroundIntervalMs: 120000
})

onMounted(() => {
  void getMccMncIndex().then((index) => {
    mccMncIndex.value = index
  })
  const win = window as Window & {
    requestIdleCallback?: (cb: IdleRequestCallback, opts?: IdleRequestOptions) => number
  }
  if (typeof win.requestIdleCallback === 'function') {
    win.requestIdleCallback(() => fetchTrafficAnalysis(), { timeout: 1500 })
  } else {
    setTimeout(fetchTrafficAnalysis, 800)
  }
})
</script>

<template>
  <div class="app-page dashboard-page">
    <div class="dashboard-toolbar">
      <RefreshButton :loading="loading" @click="fetchDevices" />
    </div>

    <ErrorState
      v-if="devicesError"
      class="mb-6"
      title="设备列表加载失败"
      :message="devicesError.message"
      :status-code="devicesError.status"
      :request-method="devicesError.method"
      :request-url="devicesError.url"
      :last-success-at="devicesLastOkAt"
      retry-text="重试"
      @retry="fetchDevices"
    />

    <ListSkeleton v-if="loading && devices.length === 0 && !devicesError" :rows="4" />
    <WorkspacePreviewCard
      v-else
      :model="preview"
      @select="selectDevice"
      @open="openDeviceOverview()"
    />

    <button
      v-if="!loading && devices.length === 0 && !devicesError"
      type="button"
      class="dashboard-empty-action"
      @click="openDeviceOverview()"
    >
      管理设备
    </button>

    <TrafficAnalysisPanel
      v-if="devices.length > 0 || !loading"
      class="dashboard-traffic"
      :analysis="analysis"
      :loading="analysisLoading"
      :error="analysisError"
      :last-ok-at="analysisLastOkAt"
      :range="analysisRange"
      mode="global"
      @update:range="handleAnalysisRangeChange"
      @refresh="fetchTrafficAnalysis"
    />
  </div>
</template>

<style scoped>
.dashboard-page {
  width: min(860px, 100%);
}
.dashboard-toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 12px;
}
.dashboard-empty-action {
  width: 100%;
  min-height: 48px;
  margin-top: 14px;
  border: 1px solid var(--ui-border);
  border-radius: var(--ui-radius-pill);
  background: var(--ui-surface);
  color: var(--ui-text);
  font-size: 14px;
  font-weight: 650;
  cursor: pointer;
}
.dashboard-empty-action:hover {
  border-color: color-mix(in srgb, var(--ui-accent) 40%, var(--ui-border));
  color: var(--ui-accent);
}
.dashboard-traffic { margin-top: 18px !important; }
</style>
