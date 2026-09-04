<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Add24Regular,
  ArrowClockwise24Regular,
  Delete24Regular,
  Dismiss24Regular,
  Person24Regular,
  PersonEdit24Regular,
  Search24Regular
} from '@vicons/fluent'
import { usePhoneIdentity } from '../composables/usePhoneIdentity'
import { phoneContactsService, type PhoneIdentity } from '../services/phone-contacts'

const props = defineProps<{
  modelValue: boolean
  deviceId?: string
  kind?: 'dial' | 'sms'
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  select: [number: string]
}>()

const identities = usePhoneIdentity()
const query = ref('')
const adding = ref(false)
const searchInput = ref<{ focus?: () => void } | null>(null)
const nameInput = ref<HTMLInputElement | null>(null)
const draftName = ref('')
const draftNumber = ref('')
const saving = ref(false)

const NUMBER_PATTERN = /^\+?[0-9]{1,32}$/
const selectLabel = computed(() => props.kind === 'sms' ? '填入短信' : '填入拨号')
const overlayDialog = {
  zIndex: 5000,
  appendTo: typeof document === 'undefined' ? undefined : document.body
}

const normalizedNumber = computed(() => {
  const raw = draftNumber.value.trim()
  if (!raw) return ''
  const plus = raw.startsWith('+')
  const digits = raw.replace(/\D/g, '')
  return plus ? `+${digits}` : digits
})

const canSave = computed(() => {
  return !!draftName.value.trim() && NUMBER_PATTERN.test(normalizedNumber.value) && !saving.value
})

const filteredContacts = computed(() => {
  const needle = query.value.trim().toLowerCase()
  if (!needle) return identities.contacts
  return identities.contacts.filter((item) => contactHaystack(item).includes(needle))
})

watch(() => props.modelValue, (open) => {
  if (!open) {
    adding.value = false
    return
  }
  query.value = ''
  void identities.ensureContacts().catch(() => {})
  void nextTick(() => {
    if (window.matchMedia('(pointer: fine)').matches) searchInput.value?.focus?.()
  })
})

function contactHaystack(item: PhoneIdentity) {
  return [
    item.title,
    item.name,
    item.number,
    item.display_number,
    item.subtitle,
    item.carrier,
    item.region,
    item.country
  ].filter(Boolean).join(' ').toLowerCase()
}

function handleOpenChange(open: boolean) {
  emit('update:modelValue', open)
}

function choose(item: PhoneIdentity) {
  emit('select', item.number)
  window.setTimeout(() => emit('update:modelValue', false), 80)
}

function toggleAdd() {
  adding.value = !adding.value
  if (adding.value) {
    void nextTick(() => nameInput.value?.focus())
  }
}

async function saveManual() {
  if (!canSave.value) return
  saving.value = true
  try {
    const ident = await phoneContactsService.save(normalizedNumber.value, draftName.value.trim(), props.deviceId)
    identities.upsertLocal(ident, normalizedNumber.value, props.deviceId)
    draftName.value = ''
    draftNumber.value = ''
    adding.value = false
    ElMessage.success({ message: '已保存联系人', zIndex: 5100 })
  } catch (error) {
    ElMessage.error({ message: error instanceof Error ? error.message : '保存联系人失败', zIndex: 5100 })
  } finally {
    saving.value = false
  }
}

async function editContact(item: PhoneIdentity) {
  try {
    const { value } = await ElMessageBox.prompt('保存后，来电和短信通知会显示这个名字', '改联系人', {
      ...overlayDialog,
      confirmButtonText: '保存',
      cancelButtonText: '取消',
      inputPlaceholder: '联系人名字',
      inputValue: item.name || '',
      inputValidator: (v) => !!String(v || '').trim() || '请填写名字'
    })
    const ident = await phoneContactsService.save(item.number, String(value).trim(), props.deviceId)
    identities.upsertLocal(ident, item.number, props.deviceId)
    ElMessage.success({ message: '已更新联系人', zIndex: 5100 })
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    ElMessage.error({ message: error instanceof Error ? error.message : '保存联系人失败', zIndex: 5100 })
  }
}

async function deleteContact(item: PhoneIdentity) {
  try {
    await ElMessageBox.confirm(`确定删除「${item.name || item.number}」？删除后，来电和短信会重新只显示号码。`, '删除联系人', {
      ...overlayDialog,
      confirmButtonText: '确认删除',
      cancelButtonText: '取消',
      type: 'warning',
      distinguishCancelAndClose: true
    })
    await identities.removeContact(item.number, props.deviceId)
    ElMessage.success({ message: '已删除联系人', zIndex: 5100 })
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    ElMessage.error({ message: error instanceof Error ? error.message : '删除联系人失败', zIndex: 5100 })
  }
}

async function reload() {
  try {
    await identities.reloadContacts()
  } catch {
    // contactsError is rendered in the list.
  }
}
</script>

<template>
  <el-drawer
    :model-value="modelValue"
    class="phone-contacts-drawer"
    modal-class="phone-contacts-drawer-scrim"
    direction="rtl"
    size="min(420px, 100vw)"
    append-to-body
    :lock-scroll="false"
    :z-index="4200"
    :show-close="false"
    @update:model-value="handleOpenChange"
  >
    <template #header>
      <div class="picker-header">
        <div class="picker-icon" aria-hidden="true"><el-icon><Person24Regular /></el-icon></div>
        <div>
          <span>CONTACTS</span>
          <h2>全部联系人 · {{ identities.contacts.length }}</h2>
        </div>
        <el-button circle aria-label="关闭联系人" @click="handleOpenChange(false)">
          <el-icon><Dismiss24Regular /></el-icon>
        </el-button>
      </div>
    </template>

    <div class="picker-toolbar">
      <el-input
        ref="searchInput"
        v-model="query"
        clearable
        placeholder="搜索名字、号码、归属地"
        aria-label="搜索联系人"
      >
        <template #prefix><el-icon><Search24Regular /></el-icon></template>
      </el-input>
      <button type="button" class="picker-add-toggle" :aria-pressed="adding" @click="toggleAdd">
        <el-icon><Add24Regular /></el-icon>
        {{ adding ? '取消' : '添加' }}
      </button>
    </div>

    <form v-if="adding" class="picker-form" @submit.prevent="saveManual">
      <label>
        名字
        <input ref="nameInput" v-model="draftName" name="drawer-contact-name" autocomplete="name" maxlength="64" placeholder="张三" />
      </label>
      <label>
        号码
        <input v-model="draftNumber" type="tel" inputmode="tel" name="drawer-contact-number" autocomplete="tel" maxlength="32" placeholder="+86138… 或 10086" />
      </label>
      <button type="submit" :disabled="!canSave">{{ saving ? '保存中…' : '保存' }}</button>
      <small v-if="draftNumber.trim() && !NUMBER_PATTERN.test(normalizedNumber)">号码只能是数字，可带开头的 +</small>
    </form>

    <div v-if="identities.contactsError" class="picker-state is-error" role="alert">
      <span>{{ identities.contactsError }}</span>
      <button type="button" class="icon-button" aria-label="重新加载联系人" @click="reload">
        <el-icon><ArrowClockwise24Regular /></el-icon>
      </button>
    </div>
    <div v-else-if="identities.contactsLoading && !identities.contacts.length" class="picker-state">正在加载联系人</div>
    <div v-else-if="filteredContacts.length" class="picker-list" role="list">
      <article v-for="item in filteredContacts" :key="item.number" class="picker-item" role="listitem">
        <button type="button" class="picker-main" :aria-label="`${selectLabel} ${item.title}`" @click="choose(item)">
          <strong>{{ item.title }}</strong>
          <span>{{ item.display_number || item.number }}</span>
          <small v-if="item.subtitle">{{ item.subtitle }}</small>
        </button>
        <button type="button" class="icon-button" :aria-label="`修改 ${item.title}`" @click.stop="editContact(item)">
          <el-icon><PersonEdit24Regular /></el-icon>
        </button>
        <button type="button" class="icon-button is-danger" :aria-label="`删除 ${item.title}`" @click.stop="deleteContact(item)">
          <el-icon><Delete24Regular /></el-icon>
        </button>
      </article>
    </div>
    <div v-else-if="identities.contacts.length" class="picker-state">没有匹配「{{ query.trim() }}」的联系人</div>
    <div v-else class="picker-state">还没有联系人。点右上角添加。</div>
  </el-drawer>
</template>

<style scoped>
:global(.phone-contacts-drawer-scrim) {
  background: color-mix(in srgb, #000 54%, transparent);
  transition: opacity 200ms var(--ui-ease-out) !important;
}
:global(.phone-contacts-drawer) {
  border-radius: var(--ui-radius-lg) 0 0 var(--ui-radius-lg);
  background: var(--ui-surface);
  transition: transform 240ms var(--ui-ease-drawer) !important;
}
:global(.phone-contacts-drawer .el-drawer__header) { min-height: 70px; margin: 0; padding: 12px 18px; border-bottom: 1px solid var(--ui-border); color: var(--ui-text); }
:global(.phone-contacts-drawer .el-drawer__body) { min-height: 0; padding: 0; display: flex; flex-direction: column; overflow: hidden; }
.picker-header { width: 100%; min-width: 0; display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 10px; }
.picker-icon { width: 38px; height: 38px; display: grid; place-items: center; border: 1px solid color-mix(in srgb, var(--ui-primary) 42%, var(--ui-border)); border-radius: var(--ui-radius-md); background: color-mix(in srgb, var(--ui-primary) 9%, transparent); color: var(--ui-primary); }
.picker-header div > span { color: var(--ui-primary); font: var(--ui-font-caption)/1.2 "v-mono", monospace; letter-spacing: .14em; }
.picker-header h2 { margin: 4px 0 0; color: var(--ui-text); font-size: 18px; }
.picker-header :deep(.el-button) { width: 44px; height: 44px; }
.picker-toolbar { padding: 14px 18px; display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 8px; align-items: center; }
.picker-add-toggle {
  min-height: 40px;
  padding: 0 12px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--ui-border);
  border-radius: var(--ui-radius-pill);
  background: var(--ui-surface);
  color: var(--ui-text);
  cursor: pointer;
  font-size: 13px;
  white-space: nowrap;
}
.picker-add-toggle[aria-pressed="true"] {
  border-color: color-mix(in srgb, var(--ui-primary) 40%, var(--ui-border));
  color: var(--ui-primary);
}
.picker-form { padding: 0 18px 14px; display: grid; grid-template-columns: 1fr; gap: 10px; border-bottom: 1px solid var(--ui-border); flex: 0 0 auto; }
.picker-form label { min-width: 0; display: grid; gap: 6px; color: var(--ui-text-muted); font-size: 12px; font-weight: 600; }
.picker-form input { width: 100%; min-width: 0; min-height: 44px; box-sizing: border-box; padding: 0 12px; border: 1px solid var(--ui-border); border-radius: var(--ui-radius-md); background: var(--ui-surface); color: var(--ui-text); font-size: 14px; }
.picker-form button[type="submit"] { width: 100%; min-height: 44px; border: 1px solid var(--ui-primary); border-radius: var(--ui-radius-pill); background: var(--ui-primary-solid); color: #fff; cursor: pointer; font-size: 14px; font-weight: 650; }
.picker-form button:disabled { cursor: not-allowed; opacity: .45; }
.picker-form small { color: var(--ui-danger); font-size: 12px; }
.picker-list { flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; }
.picker-item { min-height: 64px; padding: 8px 8px 8px 18px; display: flex; align-items: center; gap: 4px; border-bottom: 1px solid var(--ui-border-muted); transition: background-color 120ms ease; }
.picker-item:active { background: var(--ui-surface-muted); }
.picker-main { min-width: 0; flex: 1; min-height: 44px; padding: 6px 8px 6px 0; border: 0; background: transparent; color: inherit; text-align: left; cursor: pointer; }
.picker-main strong { display: block; overflow: hidden; color: var(--ui-text); font-size: 14px; text-overflow: ellipsis; white-space: nowrap; }
.picker-main span { display: block; margin-top: 2px; overflow: hidden; color: var(--ui-text-muted); font-family: "v-mono", monospace; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.picker-main small { display: block; margin-top: 2px; overflow: hidden; color: var(--ui-primary); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.icon-button { width: 40px; height: 40px; flex: 0 0 40px; display: grid; place-items: center; border: 0; background: transparent; color: var(--ui-text-muted); cursor: pointer; transition: color 120ms ease, transform 140ms var(--ui-ease-out); }
.icon-button:active { transform: scale(0.94); }
.icon-button.is-danger:active { color: var(--ui-danger); }
@media (hover: hover) and (pointer: fine) {
  .icon-button:hover { color: var(--ui-text); }
  .icon-button.is-danger:hover { color: var(--ui-danger); }
  .picker-item:hover { background: color-mix(in srgb, var(--ui-primary) 6%, var(--ui-surface)); }
}
.picker-state { min-height: 120px; padding: 24px 18px; display: flex; align-items: center; justify-content: center; gap: 12px; color: var(--ui-text-muted); font-size: 13px; text-align: center; }
.picker-state.is-error { color: var(--ui-danger); justify-content: space-between; }
@media (max-width: 640px) {
  :global(.phone-contacts-drawer) { border-radius: 0; }
  :global(.phone-contacts-drawer .el-drawer__body) { padding-bottom: env(safe-area-inset-bottom); }
}
@media (prefers-reduced-motion: reduce) {
  :global(.phone-contacts-drawer),
  :global(.phone-contacts-drawer-scrim) {
    transition-duration: 1ms !important;
  }
}
</style>
