<script setup lang="ts">
import { ref, computed } from 'vue'
import type { DeviceOverviewItem } from '../types/api'
import { isControlOnline, isRadioRegistered, isRecoveryPhase, lifecycleStatusLabel } from '../utils/deviceLifecycle'
import { phoneModeCampsOnCell } from '../utils/phoneMode'
import { displaySignalDbm, hasValidSignalDbm } from '../utils/signalPresentation'
import StatusLight from './StatusLight.vue'
import OperatorSelectionDialog from './OperatorSelectionDialog.vue'
import { Settings24Regular } from '@vicons/fluent'
import type { StatusLightTone } from './statusLight'
import DeviceOverviewConnectionStage from './DeviceOverviewConnectionStage.vue'
import DeviceOverviewIdentityPanel from './DeviceOverviewIdentityPanel.vue'

const props = defineProps<{
  device: DeviceOverviewItem | null
  simOperatorDisplay: string
  simOperatorCountryCode: string
  trafficSpeedRx: string
  trafficSpeedTx: string
  trafficMinuteRx: string
  trafficMinuteTx: string
  e911Starting: boolean
}>()

const emit = defineEmits<{
  'setup-e911': []
  'refresh': []
}>()

const showOperatorSelection = ref(false)

const trafficStateLabel = computed(() => {
  const status = props.device?.traffic_meta?.status
  if (status === 'waiting_sample') return '等待采样'
  if (status === 'stale') return '采样中断'
  return ''
})

function trafficDisplay(value: string | undefined) {
  return trafficStateLabel.value || value
}

const trafficRxDisplay = computed(() => props.trafficMinuteRx || trafficDisplay(props.device?.traffic?.rx))
const trafficTxDisplay = computed(() => props.trafficMinuteTx || trafficDisplay(props.device?.traffic?.tx))
const trafficDownloadRateDisplay = computed(() => props.trafficSpeedRx || trafficDisplay(props.device?.traffic?.rate) || '--')
const trafficUploadRateDisplay = computed(() => props.trafficSpeedTx || trafficStateLabel.value || '--')

// ---- 蜂窝模式计算属性 ----

function signalMetricDisplay(value: number | null | undefined): number | '--' {
  return typeof value === 'number' && Number.isFinite(value) && value !== 0 && value !== -999
    ? value
    : '--'
}

const signalDbmValue = computed(() => displaySignalDbm(
  props.device?.modem?.signal_dbm,
  props.device?.modem?.signal_rsrp
))

const signalDbmDisplay = computed(() => {
  const dbm = signalDbmValue.value
  return dbm === undefined ? '--' : dbm
})

// 0-5 格
const signalLevel = computed<number>(() => {
  const dbm = signalDbmValue.value
  if (!hasValidSignalDbm(dbm)) return 0
  if (dbm >= -75)  return 5
  if (dbm >= -85)  return 4
  if (dbm >= -95)  return 3
  if (dbm >= -105) return 2
  return 1
})

const signalColor = computed<'green' | 'amber' | 'red' | 'gray'>(() => {
  const dbm = signalDbmValue.value
  if (!hasValidSignalDbm(dbm)) return 'gray'
  if (dbm >= -85)  return 'green'
  if (dbm >= -100) return 'amber'
  return 'red'
})

const signalColorClass = computed(() => ({
  green: 'text-[var(--ui-success)]',
  amber: 'text-[var(--ui-warning)]',
  red:   'text-[var(--ui-danger)]',
  gray:  'text-[var(--ui-muted)]',
}[signalColor.value]))

const signalBarColor = computed(() => ({
  green: 'bg-[var(--ui-success)]',
  amber: 'bg-[var(--ui-warning)]',
  red:   'bg-[var(--ui-danger)]',
  gray:  'bg-[var(--ui-border)]',
}[signalColor.value]))

const controlOnline = computed(() => isControlOnline(props.device))

const isRegistered = computed(() => isRadioRegistered(props.device))

const cellularStatusTone = computed<StatusLightTone>(() => {
  if (isRecoveryPhase(props.device?.lifecycle_phase)) return 'warning'
  if (!controlOnline.value) return 'danger'
  return isRegistered.value ? 'success' : 'warning'
})

const cellularStatusText = computed(() => {
  const phaseText = lifecycleStatusLabel(props.device?.lifecycle_phase)
  if (phaseText && props.device?.lifecycle_phase !== 'online' && props.device?.lifecycle_phase !== 'offline') return phaseText
  if (!controlOnline.value) return props.device?.running ? '控制面恢复中' : '离线'
  if (isRegistered.value) return ''
  if (props.device?.registration_state_label === 'searching') return '搜索网络中'
  if (props.device?.registration_state_label === 'denied') return '驻网被拒'
  return '未驻网'
})

const isWifiPath = computed(() => !phoneModeCampsOnCell(props.device?.phone_mode))

const networkPanelMessage = computed(() => {
  if (isWifiPath.value) {
    return props.device?.vowifi_enabled ? 'WiFi calling 不使用蜂窝数据地址' : 'WiFi calling 未开启，不走蜂窝数据'
  }
  if (!props.device?.network_enabled) return '数据未开启（仍可驻网）'
  if (!props.device?.network_connected) return '数据网络未连接'
  return ''
})

</script>

<template>
  <div class="device-overview-stack">
    <DeviceOverviewConnectionStage :device="device" />

    <div class="device-overview-facts">

    <!-- ===== 蜂窝运行状态面板；VoWiFi 诊断已合并到主舞台 ===== -->
    <section v-if="!isWifiPath" class="overview-fact-panel ui-panel-muted p-4">
      <div class="overview-panel-title">蜂窝运行时</div>
        <!-- 运营商 hero（与 VoWiFi pill 统一样式） -->
        <div class="flex items-center gap-2.5 rounded-xl px-3.5 py-2.5 mb-3 border"
          :class="isRegistered
            ? 'bg-[var(--ui-success-surface)] border-[color-mix(in_srgb,var(--ui-success)_28%,var(--ui-border))]'
            : controlOnline
              ? 'bg-amber-50 border-amber-200 dark:bg-amber-500/10 dark:border-amber-500/25'
              : 'bg-[var(--ui-surface-muted)] border-[var(--ui-border)]'"
        >
          <StatusLight :tone="cellularStatusTone" size="sm" :animated="isRegistered" />
          <div class="flex-1 min-w-0">
            <div class="text-sm font-bold leading-tight"
              :class="isRegistered
                ? 'text-[var(--ui-success)]'
                : controlOnline
                  ? 'text-amber-700 dark:text-amber-300'
                  : 'text-[var(--ui-muted)]'"
            >
              <template v-if="isRegistered">
                {{ device?.modem?.operator || '--' }}
                <span v-if="device?.modem?.network_mode" class="opacity-70">· {{ [device?.modem?.network_duplex, device?.modem?.network_mode].filter(Boolean).join(' ') }}</span>
              </template>
              <template v-else>
                {{ cellularStatusText }}
              </template>
            </div>
          </div>
          <button @click="showOperatorSelection = true" class="p-1 rounded hover:bg-[var(--ui-selected)] transition-colors" title="网络选择设置">
            <Settings24Regular class="w-5 h-5 text-[var(--ui-muted)]" />
          </button>
        </div>

        <!-- 信号大字 -->
        <div class="rounded-xl border border-[var(--ui-border)] px-3.5 py-3 mb-3">
          <div class="text-xs font-bold text-[var(--ui-muted)] uppercase tracking-wider mb-1.5">信号强度</div>
          <div class="flex items-center gap-3">
            <div>
              <div class="flex items-baseline gap-1">
                <span class="text-2xl font-extrabold tabular-nums leading-none" :class="signalColorClass">
                  {{ signalDbmDisplay }}
                </span>
                <span class="text-xs text-[var(--ui-muted)]">dBm</span>
              </div>
              <div class="text-xs text-[var(--ui-muted)] mt-1">
                RSRP {{ signalMetricDisplay(device?.modem?.signal_rsrp) }}
                &nbsp;·&nbsp;
                RSRQ {{ signalMetricDisplay(device?.modem?.signal_rsrq) }}
                &nbsp;·&nbsp;
                SINR {{ signalMetricDisplay(device?.modem?.signal_sinr) }}
                <template v-if="device?.modem?.nr5g_signal_sinr !== undefined">
                  &nbsp;·&nbsp;NR5G SINR {{ signalMetricDisplay(device?.modem?.nr5g_signal_sinr) }}
                </template>
              </div>
            </div>
            <!-- 信号格 -->
            <div class="flex items-end gap-0.5 ml-auto" style="height: 28px">
              <div v-for="i in 5" :key="i"
                class="w-1.5 rounded-sm"
                :style="{ height: (i * 18 + 10) + '%' }"
                :class="i <= signalLevel ? signalBarColor : 'bg-[var(--ui-border)]'"
              />
            </div>
          </div>
        </div>

        <!-- 次要字段 -->
        <div class="space-y-1.5 text-sm text-[var(--ui-text)]">
          <FieldRow label="网络模式"  :value="[device?.modem?.network_duplex, device?.modem?.network_mode].filter(Boolean).join(' ') || '--'" monospace />
          <FieldRow label="频段"  :value="device?.modem?.radio_band || '--'" monospace />
          <FieldRow label="信道"  :value="device?.modem?.radio_channel ? String(device.modem.radio_channel) : '--'" monospace />
          <FieldRow label="注册状态"  :value="device?.modem?.reg_status_text || '--'" monospace />
        </div>
    </section>

    <DeviceOverviewIdentityPanel
      :class="{ 'is-wide': isWifiPath }"
      :device="device"
      :sim-operator-display="simOperatorDisplay"
      :sim-operator-country-code="simOperatorCountryCode"
      :e911-starting="e911Starting"
      @setup-e911="emit('setup-e911')"
    />

    <!-- ===== 流量面板（不变）===== -->
    <section class="overview-fact-panel overview-network-panel ui-panel-muted p-4">
      <div class="overview-panel-title mb-2">地址与实时网络</div>
      <div v-if="networkPanelMessage" class="flex items-center justify-center p-6 text-sm text-[var(--ui-muted)]">
        {{ networkPanelMessage }}
      </div>
      <div v-else class="text-sm space-y-1.5 text-[var(--ui-text)]">
        <FieldRow label="内网 IPv4"     :value="device?.private_ip"           monospace copyable />
        <FieldRow label="内网 IPv6"   :value="device?.private_ipv6"         monospace copyable wrap />
        <FieldRow label="外网 IPv4"     :value="device?.public_ip"            monospace copyable />
        <FieldRow label="外网 IPv6"   :value="device?.public_ipv6"          monospace copyable wrap />
        <FieldRow label="近1分钟上传" :value="trafficTxDisplay"             monospace />
        <FieldRow label="近1分钟下载" :value="trafficRxDisplay"             monospace />
        <FieldRow label="实时下载速率"    :value="trafficDownloadRateDisplay"   monospace />
        <FieldRow label="实时上传速率"    :value="trafficUploadRateDisplay"     monospace />
      </div>
    </section>
    </div>

    <!-- 运营商选择弹窗 -->
    <OperatorSelectionDialog
      v-if="device?.id"
      v-model="showOperatorSelection"
      :device-id="device.id"
      @updated="emit('refresh')"
    />
  </div>
</template>

<style scoped>
.device-overview-stack {
  display: grid;
  gap: 16px;
}

.device-overview-facts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.overview-fact-panel {
  min-width: 0;
  border-radius: 12px;
}

.overview-network-panel {
  grid-column: 1 / -1;
}

.overview-panel-title {
  color: var(--ui-text-muted);
  font: 700 var(--ui-font-caption) "v-mono", ui-monospace, monospace;
  letter-spacing: .12em;
  text-transform: uppercase;
}

.overview-network-panel > div:last-child {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 28px;
}

.overview-network-panel :deep(.font-mono) {
  overflow-wrap: anywhere;
}

@media (max-width: 880px) {
  .device-overview-facts {
    grid-template-columns: minmax(0, 1fr);
  }

  .overview-network-panel {
    grid-column: auto;
  }
}

@media (max-width: 520px) {
  .overview-network-panel > div:last-child {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
