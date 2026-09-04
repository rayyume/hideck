<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import {
  ArrowClockwise24Regular,
  CheckmarkCircle24Regular,
  HeartPulse24Regular,
  Stop24Regular,
  Warning24Regular
} from '@vicons/fluent'
import type {
  WiFiCallingHealthEvent,
  WiFiCallingHealthSegment,
  WiFiCallingHealthSnapshot,
  WiFiCallingHealthState
} from '../types/api'
import {
  formatHealthAvailability,
  formatHealthDuration,
  formatHealthEventTime,
  healthSegmentDuration,
  wifiCallingHealthDetail,
  wifiCallingHealthEventLabel,
  wifiCallingHealthLabel,
  wifiCallingHealthStateLabel,
  wifiCallingHealthTone
} from '../utils/wifiCallingHealth'

const props = withDefaults(defineProps<{
  health?: WiFiCallingHealthSnapshot
  mode?: 'compact' | 'summary' | 'detail'
}>(), {
  mode: 'summary'
})

const HEALTH_CLOCK_INTERVAL_MS = 30_000
const tone = computed(() => wifiCallingHealthTone(props.health?.state))
const titleID = useId()
const receivedAt = ref(Date.now())
const now = ref(receivedAt.value)
let clockTimer: number | undefined
const elapsedSeconds = computed(() => {
  if (!props.health?.active || !props.health.measured || props.mode === 'compact') return 0
  return Math.max(0, Math.floor((now.value - receivedAt.value) / 1000))
})
const liveHealth = computed<WiFiCallingHealthSnapshot | undefined>(() => {
  const health = props.health
  if (!health || elapsedSeconds.value === 0) return health
  const healthyDelta = health.state === 'healthy' ? elapsedSeconds.value : 0
  const interruptedDelta = health.state === 'healthy' ? 0 : elapsedSeconds.value
  const currentInterruption = health.timeline
    ?.slice()
    .reverse()
    .find(segment => segment.current && segment.state !== 'healthy')
  const currentInterruptionSeconds = currentInterruption
    ? healthSegmentDuration(currentInterruption.started_at, currentInterruption.ended_at)
    : 0
  const sessionSeconds = health.session_seconds + elapsedSeconds.value
  const healthySeconds = health.healthy_seconds + healthyDelta
  return {
    ...health,
    session_seconds: sessionSeconds,
    healthy_seconds: healthySeconds,
    interrupted_seconds: health.interrupted_seconds + interruptedDelta,
    stable_seconds: health.state === 'healthy' ? health.stable_seconds + elapsedSeconds.value : 0,
    longest_interruption_seconds: health.state === 'healthy'
      ? health.longest_interruption_seconds
      : Math.max(health.longest_interruption_seconds, currentInterruptionSeconds + interruptedDelta),
    availability: sessionSeconds > 0 ? 100 * healthySeconds / sessionSeconds : 0
  }
})
const statusLabel = computed(() => wifiCallingHealthLabel(props.health))
const statusDetail = computed(() => wifiCallingHealthDetail(liveHealth.value))
const availabilityLabel = computed(() => formatHealthAvailability(liveHealth.value))
const availabilityValue = computed(() => {
  if (!liveHealth.value?.measured) return undefined
  return Math.max(0, Math.min(100, Number(liveHealth.value.availability) || 0))
})
const timeline = computed(() => props.health?.timeline || [])
const events = computed(() => [...(props.health?.events || [])].reverse().slice(0, 6))
const statusIcon = computed(() => iconForState(props.health?.state))

watch(() => props.health, () => {
  receivedAt.value = Date.now()
  now.value = receivedAt.value
})

onMounted(() => {
  if (props.mode === 'compact') return
  clockTimer = window.setInterval(() => { now.value = Date.now() }, HEALTH_CLOCK_INTERVAL_MS)
})

onBeforeUnmount(() => {
  if (clockTimer !== undefined) window.clearInterval(clockTimer)
})

function iconForState(state?: WiFiCallingHealthState) {
  switch (state) {
    case 'healthy': return CheckmarkCircle24Regular
    case 'recovering': return ArrowClockwise24Regular
    case 'unavailable': return Warning24Regular
    case 'stopped': return Stop24Regular
    default: return HeartPulse24Regular
  }
}

function iconForEvent(event: WiFiCallingHealthEvent) {
  if (event.kind === 'stopped') return Stop24Regular
  if (event.kind === 'interrupted') return Warning24Regular
  if (event.kind === 'recovered') return ArrowClockwise24Regular
  return CheckmarkCircle24Regular
}

function segmentStyle(segment: WiFiCallingHealthSegment) {
  const liveDuration = segment.current ? elapsedSeconds.value : 0
  return { flexGrow: healthSegmentDuration(segment.started_at, segment.ended_at) + liveDuration }
}

function segmentLabel(segment: WiFiCallingHealthSegment): string {
  const duration = healthSegmentDuration(segment.started_at, segment.ended_at) + (segment.current ? elapsedSeconds.value : 0)
  const label = wifiCallingHealthStateLabel(segment.state)
  return `${label}，${formatHealthDuration(duration)}${segment.reason ? `，${segment.reason}` : ''}`
}
</script>

<template>
  <span
    v-if="mode === 'compact'"
    class="wifi-health wifi-health--compact"
    :class="`is-${tone}`"
    :aria-label="`WiFi Calling 健康度：${statusLabel}，可用率 ${availabilityLabel}`"
  >
    <span class="wifi-health-compact-row">
      <span class="wifi-health-title">WiFi Calling 健康</span>
      <strong>{{ statusLabel }}</strong>
    </span>
    <span
      class="wifi-health-meter"
      role="progressbar"
      aria-label="本次会话可用率"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-valuenow="availabilityValue"
      :aria-valuetext="availabilityLabel"
    >
      <span :style="{ transform: `scaleX(${(availabilityValue || 0) / 100})` }" />
    </span>
    <span class="wifi-health-compact-meta">
      <span>{{ availabilityLabel }}</span>
      <span>{{ health?.interruption_count || 0 }} 次中断</span>
    </span>
  </span>

  <section
    v-else
    class="wifi-health"
    :class="[`wifi-health--${mode}`, `is-${tone}`]"
    :aria-labelledby="titleID"
  >
    <header class="wifi-health-header">
      <span class="wifi-health-icon" aria-hidden="true"><component :is="statusIcon" /></span>
      <span class="wifi-health-heading">
        <span :id="titleID" class="wifi-health-title">WiFi Calling 健康度</span>
        <strong>{{ statusLabel }}</strong>
        <small :title="health?.last_reason">{{ statusDetail }}</small>
      </span>
      <span class="wifi-health-score">
        <strong>{{ availabilityLabel }}</strong>
        <small>本次会话可用率</small>
      </span>
    </header>

    <div
      class="wifi-health-meter"
      role="progressbar"
      aria-label="本次会话可用率"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-valuenow="availabilityValue"
      :aria-valuetext="availabilityLabel"
    >
      <span :style="{ transform: `scaleX(${(availabilityValue || 0) / 100})` }" />
    </div>

    <div v-if="mode === 'summary'" class="wifi-health-summary-meta">
      <span>稳定 {{ liveHealth?.measured ? formatHealthDuration(liveHealth.stable_seconds) : '--' }}</span>
      <span>{{ health?.interruption_count || 0 }} 次中断</span>
    </div>

    <template v-else>
      <dl class="wifi-health-metrics">
        <div>
          <dt>监测时长</dt>
          <dd>{{ liveHealth?.measured ? formatHealthDuration(liveHealth.session_seconds) : '--' }}</dd>
        </div>
        <div>
          <dt>当前稳定</dt>
          <dd>{{ liveHealth?.measured ? formatHealthDuration(liveHealth.stable_seconds) : '--' }}</dd>
        </div>
        <div>
          <dt>中断次数</dt>
          <dd>{{ health?.interruption_count || 0 }} 次</dd>
        </div>
        <div>
          <dt>最长中断</dt>
          <dd>{{ liveHealth?.measured ? formatHealthDuration(liveHealth.longest_interruption_seconds) : '--' }}</dd>
        </div>
      </dl>

      <div class="wifi-health-timeline-block">
        <div class="wifi-health-section-heading">
          <strong>连接时间轴</strong>
          <span>本次运行会话</span>
        </div>
        <div
          class="wifi-health-timeline"
          :class="{ 'is-empty': timeline.length === 0 }"
          role="img"
          :aria-label="timeline.length ? `连接时间轴，共 ${timeline.length} 个状态区间` : '尚无连接区间'"
        >
          <span
            v-for="(segment, index) in timeline"
            :key="`${segment.started_at}-${index}`"
            :class="`is-${wifiCallingHealthTone(segment.state)}`"
            :style="segmentStyle(segment)"
            :title="segmentLabel(segment)"
          />
        </div>
        <div class="wifi-health-legend" aria-label="时间轴图例">
          <span><i class="is-success" aria-hidden="true" />稳定</span>
          <span><i class="is-warning" aria-hidden="true" />恢复中</span>
          <span><i class="is-danger" aria-hidden="true" />不可用</span>
        </div>
      </div>

      <div class="wifi-health-events">
        <div class="wifi-health-section-heading">
          <strong>近期事件</strong>
          <span>含主动关闭记录</span>
        </div>
        <ol v-if="events.length">
          <li v-for="(event, index) in events" :key="`${event.at}-${event.kind}-${index}`">
            <span class="wifi-health-event-icon" :class="`is-${wifiCallingHealthTone(event.state)}`" aria-hidden="true">
              <component :is="iconForEvent(event)" />
            </span>
            <span class="wifi-health-event-copy">
              <strong>{{ wifiCallingHealthEventLabel(event) }}</strong>
              <small v-if="event.reason" :title="event.reason">{{ event.reason }}</small>
            </span>
            <time :datetime="event.at">{{ formatHealthEventTime(event.at) }}</time>
          </li>
        </ol>
        <p v-else class="wifi-health-empty">暂无状态事件</p>
      </div>
    </template>
  </section>
</template>

<style scoped src="../styles/wifiCallingHealth.css"></style>
