<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { ElMessage } from 'element-plus'
import { ArrowDownload24Regular, Delete24Regular, Pause24Regular, Play24Regular } from '@vicons/fluent'
import { useLogsStore } from '../stores/logs'
import { useEventStream } from '../composables/useEventStream'
import { deviceNow, formatDeviceDate, formatDeviceDateTime } from '../utils/deviceTime'

// 日志条目类型
interface LogEntry {
  time: string
  level: string
  caller: string
  message: string
  fields?: string
}

const logsStore = useLogsStore()
const { logs } = storeToRefs(logsStore)

const connected = ref(false)
const paused = ref(false)
const autoScroll = ref(true)
const levelFilter = ref<'all' | 'debug' | 'info' | 'warn' | 'error'>('all')
const searchQuery = ref('')
const maxLogs = 1000 // 最大保留日志条数
const lastConnectError = ref<string>('')

// 日志容器引用
const logContainer = ref<HTMLElement | null>(null)

// 过滤后的日志
const filteredLogs = computed(() => {
  let result = logs.value

  // 级别过滤（精确匹配选中的级别）
  if (levelFilter.value !== 'all') {
    result = result.filter(log => 
      log.level.toLowerCase() === levelFilter.value.toLowerCase()
    )
  }

  // 搜索过滤
  if (searchQuery.value.trim()) {
    const q = searchQuery.value.toLowerCase()
    result = result.filter(log =>
      log.message.toLowerCase().includes(q) ||
      log.caller.toLowerCase().includes(q) ||
      (log.fields && log.fields.toLowerCase().includes(q))
    )
  }

  return result
})

const stream = useEventStream<LogEntry>({
  path: '/logs/stream',
  eventName: 'log',
  query: { level: '' },
  parse: (payload) => JSON.parse(payload) as LogEntry,
  onConnected: () => {
    connected.value = true
    lastConnectError.value = ''
  },
  onEvent: (entry) => {
    if (paused.value) return
    logsStore.append(entry, maxLogs)
    if (!autoScroll.value) return
    nextTick(() => {
      if (logContainer.value) logContainer.value.scrollTop = logContainer.value.scrollHeight
    })
  }
})

function connect() {
  connected.value = false
  stream.setPaused(false)
}

function disconnect() {
  stream.disconnect()
  connected.value = false
}

// 暂停/继续
function togglePause() {
  paused.value = !paused.value
  stream.setPaused(paused.value)
  if (!paused.value) connect()
}

// 清空日志
function clearLogs() {
  logsStore.clear()
}

// 导出日志
function exportLogs() {
  const content = filteredLogs.value.map(log => {
    const time = formatDeviceDateTime(log.time, { fallback: log.time })
    const fields = log.fields ? ` ${log.fields}` : ''
    return `[${time}] ${log.level.toUpperCase().padEnd(5)} ${log.caller} ${log.message}${fields}`
  }).join('\n')

  const blob = new Blob([content], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `logs-${formatDeviceDate(deviceNow())}.txt`
  a.click()
  URL.revokeObjectURL(url)
  ElMessage.success('已导出日志')
}

// 日志级别颜色
function getLevelClass(level: string): string {
  switch (level.toLowerCase()) {
    case 'debug': return 'log-level-debug'
    case 'info': return 'log-level-info'
    case 'warn': return 'log-level-warn'
    case 'error': return 'log-level-error'
    case 'fatal': return 'log-level-fatal'
    default: return 'log-level-debug'
  }
}

// 格式化日期时间
function formatDateTime(isoTime: string): string {
  return formatDeviceDateTime(isoTime, { fallback: isoTime })
}

// 加载历史日志
async function loadHistory() {
  const result = await logsStore.fetchHistory(500)
  if (!result.ok) return
  nextTick(() => {
    if (logContainer.value) logContainer.value.scrollTop = logContainer.value.scrollHeight
  })
}

onMounted(async () => {
  await loadHistory()
  connect()
})

onUnmounted(() => {
  disconnect()
})

watch(levelFilter, () => {
  stream.setQuery({ level: levelFilter.value === 'all' ? '' : levelFilter.value })
  if (!paused.value) {
    connect()
  }
})
</script>

<template>
  <div class="app-page logs-page">
    <div class="logs-workspace ui-card ui-workspace-glow">
      <aside class="logs-control-rail">
        <div class="logs-rail-status">
          <span class="logs-connection-dot" :class="connected ? 'is-connected' : 'is-disconnected'" />
          <div>
            <small>LIVE STREAM</small>
            <strong>{{ connected ? '实时连接' : '连接中断' }}</strong>
          </div>
        </div>
        <div class="logs-rail-count">
          <span>已缓存</span>
          <strong>{{ logs.length }}</strong>
          <small>条日志</small>
        </div>
        <span v-if="!connected && lastConnectError" class="logs-rail-error" :title="lastConnectError">{{ lastConnectError }}</span>
        <div class="logs-rail-filters">
          <label>日志级别</label>
        <el-select v-model="levelFilter" placeholder="日志级别" class="w-32">
          <el-option label="全部" value="all" />
          <el-option label="DEBUG" value="debug" />
          <el-option label="INFO" value="info" />
          <el-option label="WARN" value="warn" />
          <el-option label="ERROR" value="error" />
        </el-select>
        <el-input
          v-model="searchQuery"
          placeholder="搜索日志内容..."
          clearable
          class="w-full"
        />
          <div class="logs-rail-filter-count">显示 {{ filteredLogs.length }} / {{ logs.length }}</div>
          <el-checkbox v-model="autoScroll" label="自动追尾" />
        </div>
      </aside>

      <section class="log-frame overflow-hidden">
        <header class="log-console-header">
          <div><span>SYSTEM OUTPUT</span><strong>运行时输出</strong></div>
          <div class="log-console-actions">
            <small>{{ paused ? 'PAUSED' : 'FOLLOWING' }}</small>
            <el-button size="small" @click="togglePause" :type="paused ? 'success' : 'warning'" class="!border-0">
              <el-icon><component :is="paused ? Play24Regular : Pause24Regular" /></el-icon>
              {{ paused ? '继续' : '暂停' }}
            </el-button>
            <el-button size="small" @click="exportLogs" type="primary" class="!border-0">
              <el-icon><ArrowDownload24Regular /></el-icon>
              导出
            </el-button>
            <el-button size="small" @click="clearLogs" class="!border-0">
              <el-icon><Delete24Regular /></el-icon>
              清空
            </el-button>
          </div>
        </header>
      <div
        ref="logContainer"
        class="log-console h-[calc(100dvh-150px)] min-h-[470px] overflow-auto font-mono text-sm p-4"
      >
        <div v-if="filteredLogs.length === 0" class="log-empty text-center py-8">
          {{ connected ? '等待日志...' : '未连接到日志流' }}
        </div>
        <div
          v-for="(log, idx) in filteredLogs"
          :key="idx"
          class="log-row py-0.5 px-2 -mx-2 whitespace-nowrap"
        >
          <span class="log-time">[{{ formatDateTime(log.time) }}]</span>
          <span class="font-bold ml-1 inline-block w-14" :class="getLevelClass(log.level)">{{ log.level.toUpperCase().padEnd(5) }}</span>
          <span class="log-caller inline-block w-48 truncate align-bottom" :title="log.caller">{{ log.caller }}</span>
          <span class="log-message ml-1">{{ log.message }}</span>
          <span v-if="log.fields" class="log-fields ml-1">{{ log.fields }}</span>
        </div>
      </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.logs-workspace {
  display: grid;
  grid-template-columns: 250px minmax(0, 1fr);
  align-items: stretch;
  overflow: hidden;
  animation: logs-workspace-enter 240ms var(--ui-ease-out) both;
}

.logs-control-rail {
  padding: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-right: 1px solid var(--ui-border);
}

.logs-rail-status {
  min-height: 86px;
  padding: 18px;
  display: flex;
  align-items: center;
  gap: 11px;
  border-bottom: 1px solid var(--ui-border);
}

.logs-rail-status div,
.log-console-header div {
  display: grid;
  gap: 3px;
}

.logs-rail-status small,
.log-console-header span {
  color: var(--ui-primary);
  font: 700 9px "v-mono", monospace;
  letter-spacing: .13em;
}

.logs-rail-status strong,
.log-console-header strong {
  color: var(--ui-text);
  font-size: 15px;
}

.logs-rail-count {
  padding: 20px 18px;
  display: grid;
  gap: 2px;
  border-bottom: 1px solid var(--ui-border);
  color: var(--ui-text-muted);
  font-size: 10px;
}

.logs-rail-count strong {
  color: var(--ui-text);
  font: 32px "v-mono", monospace;
}

.logs-rail-error {
  margin: 12px;
  overflow: hidden;
  color: var(--ui-danger);
  font-size: 11px;
  text-overflow: ellipsis;
}

.logs-rail-filters {
  padding: 15px;
  display: grid;
  gap: 10px;
}

.logs-rail-filters label,
.logs-rail-filter-count {
  color: var(--ui-text-muted);
  font-size: 10px;
}

.log-console-header {
  height: 60px;
  padding: 0 18px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--ui-border);
  background: var(--ui-nav);
}

.log-console-header small {
  color: var(--ui-text-muted);
  font: 10px "v-mono", monospace;
}

.log-console-actions {
  display: flex !important;
  align-items: center;
  gap: 7px !important;
}

.logs-connection-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.logs-connection-dot.is-connected {
  background: var(--ui-success);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--ui-success) 14%, transparent);
}

.logs-connection-dot.is-disconnected {
  background: var(--ui-danger);
}

.logs-toolbar {
  box-shadow: none;
  border-radius: 14px;
}

.log-frame {
  min-width: 0;
}

.log-console {
  background:
    linear-gradient(90deg, color-mix(in srgb, var(--ui-accent) 6%, transparent) 1px, transparent 1px),
    var(--ui-nav);
  background-size: 36px 100%;
  color: var(--ui-nav-text);
  font-variant-numeric: tabular-nums;
}

.log-empty,
.log-time {
  color: var(--ui-nav-muted);
}

.log-caller {
  color: var(--ui-accent);
}

.log-message {
  color: var(--ui-nav-text);
}

.log-fields {
  color: color-mix(in srgb, var(--ui-warning) 70%, var(--ui-nav-text));
}

.log-level-debug { color: var(--ui-nav-muted); }
.log-level-info { color: var(--ui-communication); }
.log-level-warn { color: var(--ui-warning); }
.log-level-error,
.log-level-fatal { color: var(--ui-danger); }

.log-row {
  border-left: 2px solid transparent;
  border-radius: 2px;
}

.log-row:hover {
  border-left-color: var(--ui-accent);
  background: color-mix(in srgb, var(--ui-nav-text) 4.5%, transparent);
}

@keyframes logs-workspace-enter {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: translateY(0); }
}

@media (prefers-reduced-motion: reduce) {
  .logs-workspace {
    animation-name: logs-workspace-fade;
  }

  @keyframes logs-workspace-fade {
    from { opacity: 0; }
    to { opacity: 1; }
  }
}

@media (max-width: 640px) {
  .logs-workspace {
    grid-template-columns: minmax(0, 1fr);
  }

  .logs-control-rail {
    min-height: 0;
    border-right: 0;
    border-bottom: 1px solid var(--ui-border);
  }

  .log-console {
    height: calc(100dvh - 360px);
    min-height: 360px;
  }
}
</style>
