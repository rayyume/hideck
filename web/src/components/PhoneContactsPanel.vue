<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Person24Regular } from '@vicons/fluent'
import PhoneContactsDrawer from './PhoneContactsDrawer.vue'
import { usePhoneIdentity } from '../composables/usePhoneIdentity'
import { useContactImport } from '../composables/useContactImport'

const emit = defineEmits<{
  dial: [number: string]
}>()
const props = defineProps<{ deviceId?: string }>()

const identities = usePhoneIdentity()
const drawerOpen = ref(false)
const {
  importing,
  dropActive,
  onDragEnter,
  onDragOver,
  onDragLeave,
  onDrop
} = useContactImport({
  deviceId: () => props.deviceId,
  reloadContacts: identities.reloadContacts,
  onComplete: () => { drawerOpen.value = true }
})

onMounted(() => {
  void identities.ensureContacts().catch(() => {})
})

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
.contacts-open span { color: var(--ui-primary); font-family: "v-mono", monospace; font-size: 12px; font-weight: 700; letter-spacing: 0; }
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
