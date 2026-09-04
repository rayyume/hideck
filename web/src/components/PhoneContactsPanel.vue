<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Call24Regular, Delete24Regular, PersonEdit24Regular } from '@vicons/fluent'
import { usePhoneIdentity } from '../composables/usePhoneIdentity'
import { phoneContactsService } from '../services/phone-contacts'

const emit = defineEmits<{
  dial: [number: string]
}>()

const identities = usePhoneIdentity()
const draftName = ref('')
const draftNumber = ref('')
const saving = ref(false)
const nameInput = ref<HTMLInputElement | null>(null)

const NUMBER_PATTERN = /^\+?[0-9]{1,32}$/

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

onMounted(() => {
  void identities.ensureContacts()
})

async function saveManual() {
  if (!canSave.value) return
  saving.value = true
  try {
    const ident = await phoneContactsService.save(normalizedNumber.value, draftName.value.trim())
    identities.upsertLocal(ident)
    draftName.value = ''
    draftNumber.value = ''
    ElMessage.success('已保存联系人')
    nameInput.value?.focus()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存联系人失败')
  } finally {
    saving.value = false
  }
}

async function editContact(number: string, currentName?: string) {
  try {
    const { value } = await ElMessageBox.prompt('保存后，来电和短信通知会显示这个名字', '改联系人', {
      confirmButtonText: '保存',
      cancelButtonText: '取消',
      inputPlaceholder: '联系人名字',
      inputValue: currentName || '',
      inputValidator: (v) => !!String(v || '').trim() || '请填写名字'
    })
    identities.upsertLocal(await phoneContactsService.save(number, String(value).trim()))
    ElMessage.success('已更新联系人')
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    ElMessage.error(error instanceof Error ? error.message : '保存联系人失败')
  }
}

async function deleteContact(number: string, name?: string) {
  try {
    await ElMessageBox.confirm(`删除 ${name || number}？来电会重新只显示号码。`, '删除联系人', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await identities.removeContact(number)
    ElMessage.success('已删除联系人')
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    ElMessage.error(error instanceof Error ? error.message : '删除联系人失败')
  }
}
</script>

<template>
  <section class="contacts-panel" aria-labelledby="phone-contacts-title">
    <header>
      <div>
        <span>CONTACTS</span>
        <h2 id="phone-contacts-title">联系人</h2>
      </div>
      <div class="contacts-actions">
        <strong>{{ identities.contacts.length }}</strong>
      </div>
    </header>
    <form class="contacts-form" @submit.prevent="saveManual">
      <label>
        名字
        <input ref="nameInput" v-model="draftName" name="contact-name" autocomplete="name" maxlength="64" placeholder="张三" />
      </label>
      <label>
        号码
        <input v-model="draftNumber" type="tel" inputmode="tel" name="contact-number" autocomplete="tel" maxlength="32" placeholder="10086 或 +4478…" />
      </label>
      <button type="submit" :disabled="!canSave">{{ saving ? '保存中…' : '保存' }}</button>
      <small v-if="draftNumber.trim() && !NUMBER_PATTERN.test(normalizedNumber)">号码只能是数字，可带开头的 +</small>
    </form>
    <div v-if="identities.contacts.length" class="contacts-list">
      <article v-for="item in identities.contacts" :key="item.number" class="contacts-item">
        <button type="button" class="contacts-main" :aria-label="`拨打 ${item.title}`" @click="emit('dial', item.number)">
          <strong>{{ item.title }}</strong>
          <span>{{ item.display_number || item.number }}</span>
          <small v-if="item.subtitle">{{ item.subtitle }}</small>
        </button>
        <el-tooltip content="拨打" placement="left">
          <button type="button" class="icon-button" :aria-label="`拨打 ${item.title}`" @click="emit('dial', item.number)">
            <el-icon><Call24Regular /></el-icon>
          </button>
        </el-tooltip>
        <el-tooltip content="改名字" placement="left">
          <button type="button" class="icon-button" :aria-label="`修改 ${item.title}`" @click="editContact(item.number, item.name)">
            <el-icon><PersonEdit24Regular /></el-icon>
          </button>
        </el-tooltip>
        <el-tooltip content="删除" placement="left">
          <button type="button" class="icon-button" :aria-label="`删除 ${item.title}`" @click="deleteContact(item.number, item.name)">
            <el-icon><Delete24Regular /></el-icon>
          </button>
        </el-tooltip>
      </article>
    </div>
    <div v-else class="contacts-empty">还没有联系人。上面填名字和号码就能加。</div>
  </section>
</template>

<style scoped>
.contacts-panel { overflow: hidden; background: transparent; border-bottom: 1px solid var(--ui-border); }
.contacts-panel > header { min-height: 68px; padding: 14px 16px; display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.contacts-panel header span { color: var(--ui-primary); font-family: "v-mono", monospace; font-size: 9px; font-weight: 700; letter-spacing: .12em; }
.contacts-panel h2 { margin: 2px 0 0; color: var(--ui-text); font-size: 16px; }
.contacts-actions { display: flex; align-items: center; gap: 8px; }
.contacts-actions > strong { min-width: 28px; padding: 3px 8px; border-radius: 20px; background: var(--ui-surface-muted); color: var(--ui-text-muted); text-align: center; }
.contacts-form { padding: 0 16px 14px; display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.1fr) auto; gap: 8px; align-items: end; }
.contacts-form label { display: grid; gap: 4px; color: var(--ui-text-muted); font-size: 10px; font-weight: 600; }
.contacts-form input { min-height: 40px; padding: 0 10px; border: 1px solid var(--ui-border); border-radius: var(--ui-radius-md); background: var(--ui-surface); color: var(--ui-text); font-size: 13px; }
.contacts-form button { min-height: 40px; padding: 0 14px; border: 1px solid var(--ui-primary); border-radius: var(--ui-radius-pill); background: var(--ui-primary-solid); color: #fff; cursor: pointer; font-size: 13px; font-weight: 650; }
.contacts-form button:disabled { cursor: not-allowed; opacity: .45; }
.contacts-form small { grid-column: 1 / -1; color: var(--ui-danger); font-size: 11px; }
.contacts-list { max-height: 280px; overflow-y: auto; border-top: 1px solid var(--ui-border-muted); }
.contacts-item { min-height: 64px; padding: 8px 10px 8px 14px; display: flex; align-items: center; gap: 4px; border-top: 1px solid var(--ui-border-muted); }
.contacts-main { min-width: 0; flex: 1; padding: 6px 8px 6px 0; border: 0; background: transparent; color: inherit; text-align: left; cursor: pointer; }
.contacts-main strong { display: block; overflow: hidden; color: var(--ui-text); font-size: 14px; text-overflow: ellipsis; white-space: nowrap; }
.contacts-main span { display: block; margin-top: 2px; color: var(--ui-text-muted); font-family: "v-mono", monospace; font-size: 12px; }
.contacts-main small { display: block; margin-top: 2px; color: var(--ui-primary); font-size: 12px; }
.icon-button { width: 40px; height: 40px; flex: 0 0 40px; display: grid; place-items: center; border: 0; background: transparent; color: var(--ui-text-muted); cursor: pointer; }
.icon-button:hover { color: var(--ui-text); }
.contacts-empty { padding: 18px 16px 22px; color: var(--ui-text-muted); font-size: 13px; }
@media (max-width: 640px) {
  .contacts-form { grid-template-columns: 1fr; }
}
</style>
