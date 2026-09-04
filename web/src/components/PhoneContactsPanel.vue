<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Person24Regular } from '@vicons/fluent'
import PhoneContactsDrawer from './PhoneContactsDrawer.vue'
import { usePhoneIdentity } from '../composables/usePhoneIdentity'
import { phoneContactsService } from '../services/phone-contacts'
import { contactImportFilesFromDataTransfer, dataTransferHasFiles } from '../utils/contactImportFile'

const emit = defineEmits<{
  dial: [number: string]
}>()
const props = defineProps<{ deviceId?: string }>()

const identities = usePhoneIdentity()
const drawerOpen = ref(false)
const dropActive = ref(false)
const importing = ref(false)
let dropDepth = 0

onMounted(() => {
  void identities.ensureContacts().catch(() => {})
})

function resetDropState() {
  dropDepth = 0
  dropActive.value = false
}

function onDragEnter(event: DragEvent) {
  if (!dataTransferHasFiles(event.dataTransfer) || importing.value) return
  event.preventDefault()
  dropDepth += 1
  dropActive.value = true
}

function onDragOver(event: DragEvent) {
  if (!dataTransferHasFiles(event.dataTransfer) || importing.value) return
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
}

function onDragLeave(event: DragEvent) {
  if (!dataTransferHasFiles(event.dataTransfer)) return
  event.preventDefault()
  dropDepth = Math.max(0, dropDepth - 1)
  if (dropDepth === 0) dropActive.value = false
}

async function onDrop(event: DragEvent) {
  event.preventDefault()
  resetDropState()
  const files = contactImportFilesFromDataTransfer(event.dataTransfer)
  if (!files.length) {
    ElMessage.error({ message: '请拖入 vcf 或 csv（iOS、Google、三星、小米、华为、OPPO、vivo 通讯录导出）', zIndex: 5100 })
    return
  }
  importing.value = true
  let imported = 0
  let skipped = 0
  let failed = 0
  let lastError = ''
  try {
    for (const file of files) {
      try {
        const result = await phoneContactsService.importFile(file, props.deviceId)
        imported += result.imported
        skipped += result.skipped
      } catch (error) {
        failed += 1
        lastError = (error as { response?: { data?: { message?: string } } })?.response?.data?.message
          || (error instanceof Error ? error.message : '导入失败')
      }
    }
    await identities.reloadContacts()
    drawerOpen.value = true
    if (imported || skipped) {
      ElMessage.success({
        message: `已导入 ${imported} 个号码`
          + (skipped ? `，跳过 ${skipped} 条` : '')
          + (failed ? `，${failed} 个文件失败` : ''),
        zIndex: 5100
      })
      return
    }
    ElMessage.error({ message: lastError || '文件里没有识别到联系人。请导出 vcf 或 csv', zIndex: 5100 })
  } finally {
    importing.value = false
  }
}
</script>

<template>
  <section
    class="contacts-bar"
    :class="{ 'is-drop': dropActive, 'is-busy': importing }"
    aria-labelledby="phone-contacts-title"
    @dragenter="onDragEnter"
    @dragover="onDragOver"
    @dragleave="onDragLeave"
    @drop="onDrop"
  >
    <button type="button" class="contacts-open" aria-labelledby="phone-contacts-title" @click="drawerOpen = true">
      <div class="contacts-icon" aria-hidden="true"><el-icon><Person24Regular /></el-icon></div>
      <div>
        <span>CONTACTS</span>
        <h2 id="phone-contacts-title">联系人</h2>
      </div>
      <strong>{{ identities.contacts.length }}</strong>
      <em>{{ dropActive ? '放开导入' : (importing ? '导入中…' : '查看全部') }}</em>
    </button>
    <PhoneContactsDrawer v-model="drawerOpen" :device-id="props.deviceId" kind="dial" @select="emit('dial', $event)" />
  </section>
</template>

<style scoped>
.contacts-bar { border-bottom: 1px solid var(--ui-border); }
.contacts-bar.is-drop {
  background: color-mix(in srgb, var(--ui-primary) 8%, var(--ui-surface));
  outline: 1px dashed color-mix(in srgb, var(--ui-primary) 55%, var(--ui-border));
  outline-offset: -1px;
}
.contacts-open {
  width: 100%;
  min-height: 68px;
  padding: 12px 16px;
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 10px;
  border: 0;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
}
.contacts-icon {
  width: 36px;
  height: 36px;
  display: grid;
  place-items: center;
  border: 1px solid color-mix(in srgb, var(--ui-primary) 42%, var(--ui-border));
  border-radius: var(--ui-radius-md);
  background: color-mix(in srgb, var(--ui-primary) 9%, transparent);
  color: var(--ui-primary);
}
.contacts-open span { color: var(--ui-primary); font-family: "v-mono", monospace; font-size: 9px; font-weight: 700; letter-spacing: .12em; }
.contacts-open h2 { margin: 2px 0 0; color: var(--ui-text); font-size: 16px; font-weight: 650; }
.contacts-open strong { min-width: 28px; padding: 3px 8px; border-radius: 20px; background: var(--ui-surface-muted); color: var(--ui-text-muted); text-align: center; font-size: 12px; }
.contacts-open em {
  min-height: 36px;
  padding: 0 12px;
  display: inline-flex;
  align-items: center;
  border: 1px solid var(--ui-border);
  border-radius: var(--ui-radius-pill);
  font-style: normal;
  font-size: 12px;
  color: var(--ui-text);
}
.contacts-bar.is-drop .contacts-open em {
  border-color: color-mix(in srgb, var(--ui-primary) 45%, var(--ui-border));
  color: var(--ui-primary);
}
.contacts-open:active { background: var(--ui-surface-muted); }
@media (hover: hover) and (pointer: fine) {
  .contacts-open:hover { background: color-mix(in srgb, var(--ui-primary) 5%, var(--ui-surface)); }
}
</style>
