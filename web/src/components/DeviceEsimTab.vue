<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { EsimChipInfo, EsimEUICCProfiles, EsimNotificationItem, EsimSpaceDelta } from '../types/api'
import { devicesService } from '../services/devices'
import { errorMessage } from '../services/http'
import { api } from '../stores/auth'
import { useSensitiveVisibility } from '../composables/useSensitiveVisibility'
import EsimCardPolicyInline from './EsimCardPolicyInline.vue'
import { applyOptimisticActiveState } from './deviceEsimOptimistic'
import { esimProfileActionForState } from './deviceEsimProfileAction'
import { pickNextDownloadAid } from './deviceEsimOverviewRefresh'
import { describeDeleteResultNotice, describeDownloadTerminalNotice, describeSpaceDelta } from './deviceEsimOperationNotice'
import { looksLikeEsimActivationCode, parseEsimActivationInput } from '../utils/esimActivationCode'
import {
  pickClipboardOrDropImage,
  pickImageFromClipboardData,
  readImageFromSystemClipboard,
  transferLooksLikeImageDrop
} from '../utils/esimQrImage'
import {
  formatEsimNotificationEvent,
  notificationDialogWidth,
  notificationListItemLayoutClass,
  notificationMetaContainerClass,
  notificationMetaItemClass,
  reconcileEsimNotificationDialogState,
  shouldShowEsimNotificationIcon,
  shouldShowEsimRefreshIcon
} from './deviceEsimNotifications'
import {
  Add24Regular,
  Alert24Regular,
  ArrowDownload24Regular,
  ArrowSync24Regular,
  Eye24Regular,
  EyeOff24Regular,
  Image24Regular,
  QrCode24Regular
} from '@vicons/fluent'

const props = defineProps<{
  deviceId: string
  deviceImei?: string
  isActive?: boolean
  deviceOnline?: boolean
}>()

// 数据状态
const loading = ref(false)
const profilesRefreshing = ref(false)
const chipInfo = ref<EsimChipInfo | null>(null)
const profiles = ref<EsimEUICCProfiles[]>([])

// 操作状态
const switching = ref<string | null>(null)
const deleting = ref<string | null>(null)
const renaming = ref<string | null>(null)
const showSensitive = useSensitiveVisibility()
const renameValue = ref('')
// 行内卡策略展开态（手风琴，一次只展开一行，按 iccid 记）
const expandedPolicyIccid = ref<string | null>(null)
function togglePolicyPanel(iccid: string) {
  expandedPolicyIccid.value = expandedPolicyIccid.value === iccid ? null : iccid
}
const notifications = ref<EsimNotificationItem[]>([])
const notificationsLoading = ref(false)
const notificationsDialogOpen = ref(false)
const retryingNotificationSequence = ref<number | null>(null)

// 下载表单
const downloadForm = ref({
  activationCode: '',
  smdp: '',
  matchingId: '',
  confirmationCode: '',
  aidHex: '',
  imei: ''
})
const confirmationRequired = ref(false)
const activationHint = ref('')
const qrFileInput = ref<HTMLInputElement | null>(null)
const qrReading = ref(false)
const qrDropActive = ref(false)
let qrDropDepth = 0
const downloading = ref(false)
const downloadProgress = ref(0)
const downloadMsg = ref('')
const downloadError = ref('')
const downloadSessionId = ref(0)
const recentSpaceDelta = ref<{ aidHex: string; message: string } | null>(null)
let recentSpaceDeltaTimer: number | null = null
let lastDeviceImeiDefault = ''

function defaultDeviceImei() {
  return (props.deviceImei || '').trim()
}

function applyDeviceImeiDefault(force = false) {
  const next = defaultDeviceImei()
  if (force || !downloadForm.value.imei || downloadForm.value.imei === lastDeviceImeiDefault) {
    downloadForm.value.imei = next
  }
  lastDeviceImeiDefault = next
}

function applyActivationInput(raw: string | number, announce?: boolean) {
  const text = String(raw ?? '')
  const shouldAnnounce = announce === true
  downloadForm.value.activationCode = text
  const parsed = parseEsimActivationInput(text)
  if (!parsed) {
    confirmationRequired.value = false
    const parts = text.replace(/\s+/g, '').split('$')
    activationHint.value = looksLikeEsimActivationCode(text) && parts.length >= 2 && parts[1]
      ? '无法识别这段内容。请贴 LPA:1$ 激活码，或包含 SM-DP+ 地址和激活码的文本'
      : ''
    return
  }
  downloadForm.value.smdp = parsed.smdp
  downloadForm.value.matchingId = parsed.matchingId
  downloadForm.value.confirmationCode = parsed.confirmationCode
  confirmationRequired.value = parsed.confirmationRequired
  activationHint.value = parsed.matchingId
    ? `已识别 ${parsed.smdp} / ${parsed.matchingId}`
    : `已识别 SM-DP+ ${parsed.smdp}`
  if (shouldAnnounce) ElMessage.success('已识别激活码')
}

function openQrFilePicker() {
  qrFileInput.value?.click()
}

function resetQrDropState() {
  qrDropDepth = 0
  qrDropActive.value = false
}

function onQrDragEnter(event: DragEvent) {
  if (!transferLooksLikeImageDrop(event.dataTransfer)) return
  event.preventDefault()
  qrDropDepth += 1
  qrDropActive.value = true
}

function onQrDragOver(event: DragEvent) {
  if (!transferLooksLikeImageDrop(event.dataTransfer)) return
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
}

function onQrDragLeave(event: DragEvent) {
  if (!transferLooksLikeImageDrop(event.dataTransfer)) return
  event.preventDefault()
  qrDropDepth = Math.max(0, qrDropDepth - 1)
  if (qrDropDepth === 0) qrDropActive.value = false
}

async function onQrDrop(event: DragEvent) {
  event.preventDefault()
  resetQrDropState()
  await decodeQrImage(pickClipboardOrDropImage(event.dataTransfer))
}

async function onQrPaste(event: ClipboardEvent) {
  const syncImage = pickClipboardOrDropImage(event.clipboardData)
  if (syncImage) {
    event.preventDefault()
    await decodeQrImage(syncImage)
    return
  }
  const image = await pickImageFromClipboardData(event.clipboardData) || await readImageFromSystemClipboard()
  if (!image) return
  event.preventDefault()
  await decodeQrImage(image)
}

async function onQrFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  await decodeQrImage(file || null)
}

async function decodeQrImage(file: File | null) {
  if (!file) {
    ElMessage.warning('请放入二维码图片或 PDF')
    return
  }
  qrReading.value = true
  try {
    const result = await devicesService.decodeEsimActivation(props.deviceId, file)
    if (!result.ok) throw result.error
    applyActivationInput(result.data.text, true)
  } catch (e: unknown) {
    ElMessage.error(errorMessage(e, '识别二维码失败'))
  } finally {
    qrReading.value = false
  }
}

function resetDownloadForm(aidHex: string, imei: string) {
  downloadForm.value = {
    activationCode: '',
    smdp: '',
    matchingId: '',
    confirmationCode: '',
    aidHex,
    imei
  }
  confirmationRequired.value = false
  activationHint.value = ''
}

let fetchAbortController: AbortController | null = null
let fetchOverviewRequestId = 0

function normalizeAidHex(aidHex: string | undefined | null): string {
  return (aidHex || '').trim().toUpperCase()
}

function clearRecentSpaceDelta() {
  if (recentSpaceDeltaTimer !== null) {
    window.clearTimeout(recentSpaceDeltaTimer)
    recentSpaceDeltaTimer = null
  }
  recentSpaceDelta.value = null
}

function showRecentSpaceDelta(aidHex: string, spaceDelta?: EsimSpaceDelta) {
  const normalizedAidHex = normalizeAidHex(aidHex)
  const message = describeSpaceDelta(spaceDelta)
  if (!normalizedAidHex || !message) return
  clearRecentSpaceDelta()
  recentSpaceDelta.value = { aidHex: normalizedAidHex, message }
  recentSpaceDeltaTimer = window.setTimeout(() => {
    recentSpaceDelta.value = null
    recentSpaceDeltaTimer = null
  }, 75000)
}

async function fetchNotifications() {
  notificationsLoading.value = true
  const result = await devicesService.getEsimNotifications(props.deviceId)
  try {
    if (!result.ok) throw result.error
    notifications.value = result.data
  } catch (e: unknown) {
    ElMessage.error(errorMessage(e, '获取当前通知列表失败'))
  } finally {
    notificationsLoading.value = false
  }
}

async function openNotificationsDialog() {
  notificationsDialogOpen.value = true
  await fetchNotifications()
}

async function retryNotification(item: EsimNotificationItem) {
  if (!item.can_retry || retryingNotificationSequence.value !== null) return
  retryingNotificationSequence.value = item.sequence_number
  const result = await devicesService.retryEsimNotification(props.deviceId, item.sequence_number, item.aid_hex)
  try {
    if (!result.ok) throw result.error
    retryingNotificationSequence.value = null
    ElMessage.success(result.data.message)
    const refreshed = await devicesService.getEsimNotifications(props.deviceId)
    if (!refreshed.ok) {
      ElMessage.warning(refreshed.error.message || '通知已发送，但刷新通知列表失败')
      return
    }
    const nextState = reconcileEsimNotificationDialogState({
      isOpen: notificationsDialogOpen.value,
      items: notifications.value,
      refreshedItems: refreshed.data,
      retriedSequenceNumber: item.sequence_number
    })
    notificationsDialogOpen.value = nextState.isOpen
    notifications.value = nextState.items
    retryingNotificationSequence.value = nextState.retryingSequenceNumber
  } catch (e: unknown) {
    const nextState = reconcileEsimNotificationDialogState({
      isOpen: notificationsDialogOpen.value,
      items: notifications.value,
      refreshedItems: notifications.value,
      retriedSequenceNumber: null
    })
    notificationsDialogOpen.value = nextState.isOpen
    notifications.value = nextState.items
    retryingNotificationSequence.value = nextState.retryingSequenceNumber
    ElMessage.error(errorMessage(e, '通知重试发送失败'))
  }
}

// 获取 eSIM 总览数据
async function fetchOverview(refresh = false) {
  fetchOverviewRequestId += 1
  const requestId = fetchOverviewRequestId

  if (fetchAbortController) {
    fetchAbortController.abort()
  }
  const controller = new AbortController()
  fetchAbortController = controller

  if (refresh) {
    profilesRefreshing.value = true
  } else {
    loading.value = true
  }

  const currentAidHex = downloadForm.value.aidHex
  const result = await devicesService.getEsimOverview(props.deviceId, {
    refresh,
    signal: controller.signal
  })
  let shouldResetLoading = true
  try {
    if (requestId !== fetchOverviewRequestId) {
      shouldResetLoading = false
      return
    }
    if (!result.ok) throw result.error
    chipInfo.value = result.data.chipInfo
    profiles.value = result.data.profiles
    downloadForm.value.aidHex = pickNextDownloadAid(chipInfo.value, currentAidHex)
  } catch (e: unknown) {
    if (result.ok === false && result.error.code === 'ERR_CANCELED') {
      return
    }
    ElMessage.error(errorMessage(e, '获取 eSIM 信息失败'))
  } finally {
    if (shouldResetLoading) {
      if (refresh) {
        profilesRefreshing.value = false
      } else {
        loading.value = false
      }
    }
  }
}

async function fetchProfiles(refresh = false) {
  profilesRefreshing.value = true
  const result = await devicesService.getEsimProfiles(props.deviceId, { refresh })
  try {
    if (!result.ok) throw result.error
    profiles.value = result.data
  } catch (e: unknown) {
    ElMessage.error(errorMessage(e, '获取 eSIM Profiles 失败'))
  } finally {
    profilesRefreshing.value = false
  }
}

function applyOptimisticActive(targetICCID: string, aidHex: string) {
  profiles.value = applyOptimisticActiveState(profiles.value, targetICCID, aidHex)
}

// Profile 启用与停用是两个独立的 LPA 操作。
async function changeProfileState(iccid: string, currentState: number, aidHex: string) {
  const operation = esimProfileActionForState(currentState)
  const action = operation === 'disable' ? '停用' : '启用'
  const confirmed = await ElMessageBox.confirm(
    `确定要${action}此 Profile (${iccid}) 吗？切换后设备会短暂断网。`,
    `${action} Profile`,
    { confirmButtonText: action, cancelButtonText: '取消', type: 'warning' }
  ).then(() => true).catch(() => false)
  if (!confirmed) return

  switching.value = iccid
  try {
    const payload = {
      iccid,
      aid_hex: aidHex
    }
    const result = operation === 'disable'
      ? await devicesService.disableEsimProfile(props.deviceId, payload)
      : await devicesService.switchEsimProfile(props.deviceId, payload)
    if (!result.ok) throw new Error(result.error.message || `${action}失败`)
    ElMessage.success(`Profile ${action}成功`)
    if (operation === 'disable') {
      await fetchProfiles(true)
    } else {
      applyOptimisticActive(iccid, aidHex)
    }
  } catch (e: unknown) {
    ElMessage.error(errorMessage(e, `${action}失败`))
  } finally {
    switching.value = null
  }
}

// 开始编辑名称
function startRename(iccid: string, currentName: string) {
  renaming.value = iccid
  renameValue.value = currentName
}

// 保存名称
async function saveRename(iccid: string, aidHex: string) {
  const name = renameValue.value.trim()
  if (!name) {
    ElMessage.warning('名称不能为空')
    return
  }
  try {
    const result = await devicesService.renameEsimProfile(props.deviceId, iccid, { name, aid_hex: aidHex })
    if (!result.ok) throw new Error(result.error.message || '修改名称失败')
    ElMessage.success('名称修改成功')
    renaming.value = null
    await fetchProfiles(true)
  } catch (e: unknown) {
    ElMessage.error(errorMessage(e, '修改名称失败'))
  }
}

// 取消编辑
function cancelRename() {
  renaming.value = null
  renameValue.value = ''
}

// 删除 profile（需要输入 ICCID 后 4 位确认）
async function deleteProfile(iccid: string, name: string, aidHex: string) {
  const last4 = iccid.slice(-4)
  const { value: input } = await ElMessageBox.prompt(
    `此操作不可逆！请输入 ICCID 后 4 位「${last4}」以确认删除 Profile「${name}」`,
    '⚠️ 删除 Profile',
    {
      confirmButtonText: '确认删除',
      cancelButtonText: '取消',
      inputPattern: new RegExp(`^${last4}$`),
      inputErrorMessage: `请输入 ${last4} 以确认`,
      inputPlaceholder: `输入 ${last4}`,
      type: 'error',
      confirmButtonClass: '!bg-red-600 !border-red-600 hover:!bg-red-700'
    }
  ).catch(() => ({ value: '' }))
  if (input !== last4) return

  deleting.value = iccid
  try {
    const result = await devicesService.deleteEsimProfile(props.deviceId, iccid, aidHex)
    if (!result.ok) throw new Error(result.error.message || '删除失败')
    showRecentSpaceDelta(aidHex, result.data.space_delta)
    const notice = describeDeleteResultNotice(result.data)
    if (notice.tone === 'warning') {
      ElMessage.warning(notice.message)
    } else {
      ElMessage.success(notice.message)
    }
    await fetchOverview(true)
  } catch (e: unknown) {
    ElMessage.error(errorMessage(e, '删除失败'))
  } finally {
    deleting.value = null
  }
}

// 下载新 profile（SSE 流式进度）
async function downloadProfile() {
  const parsedCode = parseEsimActivationInput(downloadForm.value.activationCode)
  const smdp = (downloadForm.value.smdp || parsedCode?.smdp || '').trim()
  const matchingId = (downloadForm.value.matchingId || parsedCode?.matchingId || '').trim()
  const { confirmationCode, aidHex, imei } = downloadForm.value
  const targetAidHex = aidHex || pickNextDownloadAid(chipInfo.value, '')
  if (!smdp) {
    ElMessage.warning('请粘贴二维码里的激活码，或上传二维码图片')
    return
  }
  if (confirmationRequired.value && !confirmationCode.trim()) {
    ElMessage.warning('这张卡需要确认码')
    return
  }

  downloadSessionId.value++
  downloading.value = true
  downloadProgress.value = 0
  downloadMsg.value = '正在连接...'
  downloadError.value = ''

  const params = new URLSearchParams({ smdp })
  if (matchingId) params.set('matching_id', matchingId)
  if (confirmationCode) params.set('confirmation_code', confirmationCode)
  if (targetAidHex) params.set('aid_hex', targetAidHex)
  if (imei.trim()) params.set('imei', imei.trim())

  const base = api.defaults.baseURL || ''
  const url = `${base}/devices/${props.deviceId}/esim/actions/download?${params}`
  const token = localStorage.getItem('token') || ''
  const controller = new AbortController()

  try {
    const res = await fetch(url, {
      method: 'GET',
      headers: { Authorization: `Bearer ${token}`, Accept: 'text/event-stream' },
      signal: controller.signal
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(text || `HTTP ${res.status}`)
    }
    if (!res.body) throw new Error('No stream body')

    const reader = res.body.getReader()
    const decoder = new TextDecoder('utf-8')
    let buffer = ''

    outer: while (true) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })

      while (true) {
        const nl = buffer.indexOf('\n')
        if (nl < 0) break
        let line = buffer.slice(0, nl)
        buffer = buffer.slice(nl + 1)
        if (line.endsWith('\r')) line = line.slice(0, -1)
        if (!line.startsWith('data:')) continue

        const payload = line.slice('data:'.length).trim()
        try {
          const evt = JSON.parse(payload) as { step: string; msg: string; pct: number; code?: string; warning?: string; space_delta?: EsimSpaceDelta }
          if (evt.step === 'error') {
            downloadError.value = evt.code === 'euicc_insufficient_memory'
              ? 'eUICC 安装 profile 时空间不足，请删除未使用的 profile 后重试。'
              : evt.msg
            break outer
          }
          downloadProgress.value = evt.pct
          downloadMsg.value = evt.msg
          if (evt.step === 'done') {
            showRecentSpaceDelta(targetAidHex, evt.space_delta)
            const notice = describeDownloadTerminalNotice(evt)
            if (notice.tone === 'warning') {
              ElMessage.warning(notice.message)
            } else {
              ElMessage.success(notice.message)
            }
            resetDownloadForm(targetAidHex, imei)
            await fetchOverview(true)
            break outer
          }
        } catch { /* 非 JSON 行，忽略 */ }
      }
    }
  } catch (e: unknown) {
    if (!downloadError.value) {
      downloadError.value = errorMessage(e, '下载失败')
    }
  } finally {
    downloading.value = false
  }
}

// 切换设备或改换 tab 时重新获取数据
watch(
  [() => props.deviceId, () => props.isActive],
  ([newId, newActive]) => {
    if (fetchAbortController) {
      fetchAbortController.abort()
    }
    expandedPolicyIccid.value = null
    if (!newId || !newActive) return

    clearRecentSpaceDelta()
    chipInfo.value = null
    profiles.value = []
    downloadForm.value.aidHex = ''
    applyDeviceImeiDefault(true)
    fetchOverview()
  },
  { immediate: true }
)

watch(() => props.deviceImei, () => {
  applyDeviceImeiDefault(false)
})

onBeforeUnmount(() => {
  resetQrDropState()
  clearRecentSpaceDelta()
  if (fetchAbortController) {
    fetchAbortController.abort()
  }
})
</script>

<template>
  <div class="esim-workspace space-y-5">
    <header class="esim-workspace-header">
      <div>
        <span>ESIM PROFILES</span>
        <h2>eSIM 档案管理</h2>
        <p>查看真实 eUICC 状态、管理档案。把运营商二维码或 LPA:1$ 激活码贴到下面即可</p>
      </div>
      <div class="esim-workspace-actions">
        <el-button :loading="profilesRefreshing" @click="fetchOverview(true)" class="ui-glass-border !border-0">
          <el-icon v-if="shouldShowEsimRefreshIcon(profilesRefreshing)"><ArrowSync24Regular /></el-icon>
          刷新
        </el-button>
        <el-button :loading="notificationsLoading" @click="openNotificationsDialog" class="ui-glass-border !border-0">
          <el-icon v-if="shouldShowEsimNotificationIcon(notificationsLoading)"><Alert24Regular /></el-icon>
          当前通知
        </el-button>
      </div>
    </header>

    <div v-if="loading" class="space-y-4">
      <div class="ui-panel-muted p-4 relative overflow-hidden esim-loading-hero">
        <div class="flex items-center gap-3">
          <div class="w-10 h-10 rounded-[var(--ui-radius-md)] esim-orbit flex items-center justify-center text-white text-xs font-bold">
            ESIM
          </div>
          <div class="space-y-2 flex-1">
            <div class="h-4 w-44 rounded-[var(--ui-radius-sm)] esim-skeleton-line" />
            <div class="h-3 w-64 rounded-[var(--ui-radius-sm)] esim-skeleton-line esim-skeleton-line-soft" />
          </div>
          <div class="flex items-center gap-1.5">
            <span class="esim-dot" />
            <span class="esim-dot" />
            <span class="esim-dot" />
          </div>
        </div>
        <div class="esim-skeleton-shimmer" />
      </div>

      <div class="ui-panel-muted p-4 space-y-3">
        <div class="h-3 w-28 rounded-[var(--ui-radius-sm)] esim-skeleton-line" />
        <div class="space-y-2">
          <div class="h-10 rounded-[var(--ui-radius-lg)] esim-skeleton-line" />
          <div class="h-10 rounded-[var(--ui-radius-lg)] esim-skeleton-line esim-skeleton-line-soft" />
          <div class="h-10 rounded-[var(--ui-radius-lg)] esim-skeleton-line" />
        </div>
      </div>
    </div>

    <template v-else>
      <!-- 芯片信息 -->
      <section v-if="chipInfo" class="esim-chip-strip ui-panel-muted p-4 relative">
      <div class="flex items-center justify-between gap-3">
        <div class="flex items-center gap-3 min-w-0">
          <div class="section-icon section-icon-success text-xs font-bold">
            ESIM
          </div>
          <div>
            <div class="text-base font-bold text-[var(--ui-text)]">
              {{ chipInfo.sku_name || 'eUICC' }}
            </div>
            <div class="text-xs text-[var(--ui-muted)] font-mono">
              <template v-if="chipInfo.firmware">固件 {{ chipInfo.firmware }}</template>
              <template v-if="chipInfo.serial_number">
                · SN: <span class="transition-all" :class="{ 'blur-sm select-none': !showSensitive }">{{ chipInfo.serial_number }}</span>
              </template>
            </div>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <span class="esim-ready-status"><i aria-hidden="true" />eUICC 就绪</span>
          <el-tooltip :content="showSensitive ? '隐藏敏感信息' : '显示敏感信息'" placement="top">
            <el-button circle text @click="showSensitive = !showSensitive">
              <el-icon size="18">
                <Eye24Regular v-if="showSensitive" />
                <EyeOff24Regular v-else />
              </el-icon>
            </el-button>
          </el-tooltip>
        </div>
      </div>
      </section>

      <!-- 按 eUICC 分组的 Profiles -->
      <section v-for="(group, gi) in profiles" :key="group.aid_hex || group.eid || ('group-' + gi)" class="esim-profile-group ui-panel-muted overflow-hidden">
      <!-- eUICC 头部 -->
      <div class="px-4 py-3 border-b border-[var(--ui-border)]">
        <div class="flex items-center justify-between">
          <div>
            <span class="text-sm font-bold text-[var(--ui-text)]">eUICC #{{ gi + 1 }}</span>
            <span class="text-xs text-[var(--ui-muted)] font-mono ml-2 transition-all" :class="{ 'blur-sm select-none': !showSensitive }">
              {{ group.eid }}
            </span>
          </div>
          <div v-if="chipInfo?.eids" class="text-xs text-[var(--ui-muted)]">
            <template v-for="eid in chipInfo.eids" :key="eid.eid">
              <span v-if="eid.eid === group.eid" class="inline-flex flex-col items-end gap-1">
                <span class="inline-flex items-center gap-1">
                  <span class="w-2 h-2 rounded-full" :class="eid.free_nvram_bytes > 100000 ? 'bg-[var(--ui-success)]' : 'bg-[var(--ui-warning)]'" />
                  可用 {{ eid.free_nvram }}
                </span>
                <span v-if="recentSpaceDelta && normalizeAidHex(group.aid_hex) === recentSpaceDelta.aidHex" class="text-xs text-[var(--ui-success)]">
                  {{ recentSpaceDelta.message }}
                </span>
              </span>
            </template>
          </div>
        </div>
        <!-- PKI 信息行 -->
        <template v-if="chipInfo?.eids">
          <template v-for="eid in chipInfo.eids" :key="'pki-' + eid.eid">
            <div v-if="eid.eid === group.eid && (eid.manufacturer || eid.certificates?.length || eid.default_smdp_address || eid.root_ds_address || eid.sas_accreditation_number || eid.info_source)" class="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--ui-muted)]">
              <span v-if="eid.manufacturer" class="inline-flex items-center gap-1">
                <span class="text-xs">生产商:</span> {{ eid.manufacturer }}
              </span>
              <span v-if="eid.certificates?.length" class="inline-flex items-center gap-1">
                <span class="text-xs">证书:</span> {{ eid.certificates.join(' · ') }}
              </span>
              <span v-if="eid.default_smdp_address" class="inline-flex items-center gap-1">
                <span class="text-xs">Default SM-DP+:</span> {{ eid.default_smdp_address }}
              </span>
              <span v-if="eid.root_ds_address" class="inline-flex items-center gap-1">
                <span class="text-xs">Root SM-DS:</span> {{ eid.root_ds_address }}
              </span>
              <span v-if="eid.sas_accreditation_number" class="inline-flex items-center gap-1">
                <span class="text-xs">SAS:</span> {{ eid.sas_accreditation_number }}
              </span>
              <span v-if="eid.info_source" class="inline-flex items-center gap-1">
                <span class="text-xs">来源:</span> {{ eid.info_source }}
              </span>
            </div>
          </template>
        </template>
      </div>

      <!-- Profile 列表 -->
      <div v-if="group.profiles?.length === 0" class="p-4 text-sm text-[var(--ui-muted)]">
        暂无 Profile
      </div>
      <div v-else class="divide-y divide-[var(--ui-border)]">
        <template v-for="p in group.profiles" :key="p.iccid">
        <div class="esim-profile-row px-4 py-3 hover:bg-[var(--ui-selected)] transition-colors">
          <div class="min-w-0 flex-1">
            <!-- 正常显示模式 -->
            <template v-if="renaming !== p.iccid">
              <div class="flex items-center gap-2">
                <span class="w-2 h-2 rounded-full flex-shrink-0" :class="p.state === 1 ? 'bg-[var(--ui-success)]' : 'bg-[var(--ui-border)]'" />
                <span class="font-medium text-sm text-[var(--ui-text)] truncate">{{ p.name || p.iccid }}</span>
                <el-tag size="small" :type="p.state === 1 ? 'success' : 'info'" class="flex-shrink-0">
                  {{ p.state_text }}
                </el-tag>
              </div>
              <div class="text-xs text-[var(--ui-muted)] mt-0.5 ml-4 flex flex-wrap items-center gap-x-2 gap-y-1 transition-all">
                <span>{{ p.service_provider_name }}</span>
                <span :class="{ 'blur-sm select-none': !showSensitive }">{{ p.iccid }}</span>
              </div>
            </template>
            <!-- 编辑名称模式 -->
            <template v-else>
              <div class="flex items-center gap-2">
                <el-input
                  v-model="renameValue"
                  size="small"
                  placeholder="输入新名称"
                  @keyup.enter="saveRename(p.iccid, group.aid_hex)"
                  @keyup.escape="cancelRename"
                  autofocus
                  class="!w-52"
                />
                <el-button size="small" type="primary" @click="saveRename(p.iccid, group.aid_hex)" class="!border-0">保存</el-button>
                <el-button size="small" @click="cancelRename" class="!border-0">取消</el-button>
              </div>
            </template>
          </div>

          <!-- 操作按钮 -->
          <div v-if="renaming !== p.iccid" class="esim-profile-actions">
            <el-button
              size="small"
              :type="p.state === 1 ? 'warning' : 'success'"
              :loading="switching === p.iccid"
              @click="changeProfileState(p.iccid, p.state, group.aid_hex)"
              plain
            >
              {{ p.state === 1 ? '停用' : '切换' }}
            </el-button>
            <el-button
              size="small"
              type="primary"
              @click="startRename(p.iccid, p.name)"
              plain
            >
              改名
            </el-button>
            <el-button
              size="small"
              :type="expandedPolicyIccid === p.iccid ? 'primary' : 'default'"
              @click="togglePolicyPanel(p.iccid)"
              plain
            >
              策略
            </el-button>
            <el-button
              size="small"
              type="danger"
              :disabled="p.state === 1"
              :loading="deleting === p.iccid"
              @click="deleteProfile(p.iccid, p.name, group.aid_hex)"
              plain
            >
              删除
            </el-button>
          </div>
        </div>
          <div v-if="expandedPolicyIccid === p.iccid" class="px-4 pb-3 border-t-0">
            <EsimCardPolicyInline
              :device-id="props.deviceId"
              :iccid="p.iccid"
              :is-active-card="p.state === 1"
              :device-online="props.deviceOnline === true"
              @policy-changed="fetchOverview(true)"
            />
          </div>
        </template>
      </div>
      </section>

      <el-dialog
        v-model="notificationsDialogOpen"
        title="当前通知列表"
        :width="notificationDialogWidth()"
        class="glass-modal"
      >
        <div v-if="notificationsLoading" class="py-10 text-sm text-center text-[var(--ui-muted)]">正在加载通知...</div>
        <div v-else-if="notifications.length === 0" class="py-10 text-sm text-center text-[var(--ui-muted)]">当前没有可展示的通知</div>
        <div v-else class="space-y-2 max-h-[420px] overflow-auto pr-1">
          <div
            v-for="item in notifications"
            :key="item.sequence_number"
            :class="notificationListItemLayoutClass()"
          >
            <div class="min-w-0 flex-1 space-y-1">
              <div class="flex items-center gap-2 text-sm font-medium text-[var(--ui-text)]">
                <span>#{{ item.sequence_number }}</span>
                <el-tag size="small" type="info">{{ formatEsimNotificationEvent(item.event) }}</el-tag>
              </div>
              <div :class="notificationMetaContainerClass()">
                <div v-if="item.iccid" :class="notificationMetaItemClass()">
                  <span class="mr-1 text-[var(--ui-muted)]">ICCID</span>
                  <span class="break-all">{{ item.iccid }}</span>
                </div>
                <div v-if="item.address" :class="notificationMetaItemClass()">
                  <span class="mr-1 text-[var(--ui-muted)]">地址</span>
                  <span class="break-all">{{ item.address }}</span>
                </div>
                <div v-if="item.aid_hex" :class="notificationMetaItemClass()">
                  <span class="mr-1 text-[var(--ui-muted)]">AID</span>
                  <span class="break-all">{{ item.aid_hex }}</span>
                </div>
              </div>
            </div>
            <el-button
              size="small"
              type="primary"
              class="self-start sm:self-auto"
              :disabled="!item.can_retry"
              :loading="retryingNotificationSequence === item.sequence_number"
              @click="retryNotification(item)"
            >
              重发
            </el-button>
          </div>
        </div>
      </el-dialog>

      <!-- 下载新 Profile -->
      <section v-if="chipInfo" class="esim-install-panel ui-panel-muted p-4">
      <div class="flex items-center gap-2 mb-3">
        <div class="w-7 h-7 rounded-[var(--ui-radius-sm)] bg-indigo-50 dark:bg-indigo-500/10 flex items-center justify-center text-indigo-600 dark:text-indigo-400">
          <el-icon size="16"><Add24Regular /></el-icon>
        </div>
        <div>
          <span class="esim-panel-eyebrow">INSTALL PROFILE</span>
          <div class="text-sm font-bold text-[var(--ui-text)]">安装新档案</div>
        </div>
      </div>
      <p class="esim-install-copy">
        市面常见 eSIM（VOXI、Giffgaff、Airalo、联通等）给的是一张二维码，或一行
        <span class="esim-install-code">LPA:1$</span>
        激活码。可以把二维码图片或 PDF 拖进来、粘贴或上传，由服务器识别。邮件里如果是分开的 SM-DP+ 地址和激活码，整段贴进来也可以。
      </p>
      <div class="space-y-3">
        <div class="space-y-1">
          <div class="text-xs font-bold text-[var(--ui-muted)] uppercase tracking-wider">激活码 / 二维码内容</div>
          <div
            class="esim-qr-drop"
            :class="{ 'is-active': qrDropActive, 'is-reading': qrReading }"
            tabindex="0"
            @dragenter="onQrDragEnter"
            @dragover="onQrDragOver"
            @dragleave="onQrDragLeave"
            @drop="onQrDrop"
            @paste.capture="onQrPaste"
          >
            <el-input
              v-model="downloadForm.activationCode"
              type="textarea"
              :autosize="{ minRows: 3, maxRows: 5 }"
              placeholder="LPA:1$smdp.example.com$匹配码，或把二维码图片 / PDF 拖到这里"
              @update:model-value="applyActivationInput"
            />
            <div class="flex flex-wrap items-center gap-2">
              <input
                ref="qrFileInput"
                type="file"
                accept="image/*,application/pdf,.pdf"
                class="esim-qr-file"
                @change="onQrFileChange"
              >
              <el-button :loading="qrReading" @click="openQrFilePicker">
                <el-icon><Image24Regular /></el-icon>
                识别图片 / PDF
              </el-button>
              <span class="esim-activation-hint">{{ qrDropActive ? '放开即可识别二维码' : (activationHint || '支持图片、PDF，可拖入或粘贴') }}</span>
            </div>
          </div>
        </div>
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-3">
          <div class="space-y-1">
            <div class="text-xs font-bold text-[var(--ui-muted)] uppercase tracking-wider">SM-DP+ 地址</div>
            <el-input v-model="downloadForm.smdp" placeholder="解析后自动填写，也可手填">
              <template #prefix>
                <el-icon><QrCode24Regular /></el-icon>
              </template>
            </el-input>
          </div>
          <div class="space-y-1">
            <div class="text-xs font-bold text-[var(--ui-muted)] uppercase tracking-wider">Matching ID</div>
            <el-input v-model="downloadForm.matchingId" placeholder="解析后自动填写" />
          </div>
          <div class="space-y-1">
            <div class="text-xs font-bold text-[var(--ui-muted)] uppercase tracking-wider">
              确认码{{ confirmationRequired ? ' *' : '' }}
            </div>
            <el-input
              v-model="downloadForm.confirmationCode"
              :placeholder="confirmationRequired ? '这张卡需要确认码' : '可选'"
            />
          </div>
          <div class="space-y-1">
            <div class="text-xs font-bold text-[var(--ui-muted)] uppercase tracking-wider">IMEI</div>
            <el-input v-model="downloadForm.imei" maxlength="15" placeholder="默认使用设备 IMEI，可修改" />
          </div>
          <div class="space-y-1">
            <div class="text-xs font-bold text-[var(--ui-muted)] uppercase tracking-wider">目标 eUICC</div>
            <el-select v-model="downloadForm.aidHex" placeholder="选择目标 eUICC">
              <el-option
                v-for="(eid, ei) in (chipInfo?.eids || [])"
                :key="eid.aid"
                :label="`eUICC #${Number(ei) + 1} (...${eid.eid.slice(-4)}) — ${eid.free_nvram} 可用`"
                :value="eid.aid"
              />
            </el-select>
          </div>
        </div>
      </div>
      <!-- 下载进度条 -->
      <div v-if="downloading || downloadError" class="mt-4 space-y-1.5">
        <el-progress
          :key="downloadSessionId"
          :percentage="downloadProgress"
          :status="downloadError ? 'exception' : downloadProgress >= 100 ? 'success' : undefined"
          :striped="downloading && downloadProgress < 100"
          :striped-flow="downloading && downloadProgress < 100"
          :duration="8"
          :stroke-width="10"
        />
        <div class="text-xs" :class="downloadError ? 'text-red-500' : 'text-[var(--ui-muted)]'">
          {{ downloadError || downloadMsg }}
        </div>
      </div>

      <div class="flex justify-end mt-4">
        <el-button type="primary" :loading="downloading" :disabled="downloading" @click="downloadProfile" class="!border-0">
          <el-icon><ArrowDownload24Regular /></el-icon>
          开始下载
        </el-button>
      </div>
      </section>

      <!-- 空状态 -->
      <EmptyState v-if="profiles.length === 0 && !chipInfo" title="未检测到 eUICC" subtitle="此SIM卡可能不支持 eUICC 功能" />
    </template>
  </div>
</template>

<style scoped>
.esim-workspace-header {
  min-height: 74px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
}

.esim-workspace-header > div:first-child > span,
.esim-panel-eyebrow {
  color: var(--ui-primary);
  font: 700 var(--ui-font-caption) "v-mono", ui-monospace, monospace;
  letter-spacing: .15em;
}

.esim-workspace-header h2 {
  margin: 4px 0 0;
  color: var(--ui-text);
  font-size: 20px;
  font-weight: 650;
}

.esim-workspace-header p {
  margin: 3px 0 0;
  color: var(--ui-text-muted);
  font-size: var(--ui-font-body-sm);
}

.esim-workspace-actions,
.esim-profile-actions,
.esim-ready-status {
  display: flex;
  align-items: center;
}

.esim-workspace-actions,
.esim-profile-actions {
  gap: 8px;
}

.esim-workspace-actions :deep(.el-button + .el-button),
.esim-profile-actions :deep(.el-button + .el-button) {
  margin-left: 0;
}

.esim-chip-strip {
  border-color: color-mix(in srgb, var(--ui-primary) 28%, var(--ui-border));
  background: color-mix(in srgb, var(--ui-primary) 6%, var(--ui-surface-muted));
}

.esim-ready-status {
  gap: 6px;
  color: var(--ui-success);
  font-size: var(--ui-font-caption);
  white-space: nowrap;
}

.esim-ready-status i {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}

.esim-profile-group,
.esim-install-panel {
  border-radius: var(--ui-radius-xl);
}

.esim-profile-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.esim-profile-actions {
  flex: 0 0 auto;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.esim-install-panel {
  position: relative;
  background:
    linear-gradient(145deg, color-mix(in srgb, var(--ui-primary) 4%, transparent), transparent 48%),
    var(--ui-surface-muted);
}

.esim-install-copy {
  margin: 0 0 12px;
  color: var(--ui-text-muted);
  font-size: var(--ui-font-body-sm);
  line-height: 1.5;
}

.esim-install-code {
  color: var(--ui-text);
  font-family: "v-mono", ui-monospace, monospace;
}

.esim-qr-drop {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px;
  border: 1px dashed var(--ui-border);
  border-radius: var(--ui-radius-md);
  outline: none;
}

.esim-qr-drop.is-active {
  border-color: var(--ui-primary);
  background: color-mix(in srgb, var(--ui-primary) 8%, transparent);
}

.esim-qr-drop.is-reading {
  opacity: 0.85;
}

.esim-qr-drop:focus-visible {
  border-color: var(--ui-primary);
}

.esim-qr-file {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
}

.esim-activation-hint {
  color: var(--ui-text-muted);
  font-size: var(--ui-font-body-sm);
}

.esim-loading-hero {
  min-height: 88px;
}

.esim-orbit {
  background: linear-gradient(135deg, var(--ui-success), var(--ui-accent));
  animation: esim-orbit 2.2s ease-in-out infinite;
}

.esim-skeleton-line {
  background: linear-gradient(90deg, color-mix(in srgb, var(--ui-text-muted) 18%, transparent), color-mix(in srgb, var(--ui-text-muted) 34%, transparent), color-mix(in srgb, var(--ui-text-muted) 18%, transparent));
  background-size: 200% 100%;
  animation: esim-shimmer 1.4s linear infinite;
}

.esim-skeleton-line-soft {
  opacity: 0.8;
  animation-duration: 1.9s;
}

.esim-skeleton-shimmer {
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(120deg, transparent 0%, rgba(255, 255, 255, 0.24) 45%, transparent 75%);
  transform: translateX(-130%);
  animation: esim-sweep 2.1s ease-in-out infinite;
}

.esim-dot {
  width: 7px;
  height: 7px;
  border-radius: 9999px;
  background: var(--ui-accent);
  opacity: 0.3;
  animation: esim-dot-bounce 1.1s ease-in-out infinite;
}

.esim-dot:nth-child(2) {
  animation-delay: 0.16s;
}

.esim-dot:nth-child(3) {
  animation-delay: 0.32s;
}

@keyframes esim-shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}

@keyframes esim-sweep {
  0% { transform: translateX(-130%); }
  100% { transform: translateX(130%); }
}

@keyframes esim-dot-bounce {
  0%, 80%, 100% { opacity: 0.3; transform: translateY(0); }
  40% { opacity: 1; transform: translateY(-2px); }
}

@keyframes esim-orbit {
  0%, 100% { transform: scale(1); box-shadow: 0 8px 18px color-mix(in srgb, var(--ui-success) 25%, transparent); }
  50% { transform: scale(1.04); box-shadow: 0 10px 22px color-mix(in srgb, var(--ui-accent) 35%, transparent); }
}

@media (max-width: 720px) {
  .esim-workspace-header,
  .esim-profile-row {
    align-items: stretch;
    flex-direction: column;
  }

  .esim-workspace-actions,
  .esim-profile-actions {
    width: 100%;
    justify-content: flex-start;
  }

  .esim-profile-actions :deep(.el-button) {
    flex: 1 1 calc(50% - 4px);
  }
}

@media (prefers-reduced-motion: reduce) {
  .esim-orbit,
  .esim-skeleton-line,
  .esim-skeleton-shimmer,
  .esim-dot {
    animation: none;
  }
}
</style>
