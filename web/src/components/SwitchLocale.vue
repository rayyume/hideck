<script setup lang="ts">
import { computed } from 'vue'
import { nextLocale } from '../utils/locale'
import { t, useLocale } from '../i18n'

const { locale, setLocale } = useLocale()
const label = computed(() => (locale.value === 'en' ? '中' : 'EN'))
const hint = computed(() => (locale.value === 'en' ? t('locale.switchToZh') : t('locale.switchToEn')))

function onToggle() {
  setLocale(nextLocale(locale.value))
}
</script>

<template>
  <el-tooltip :content="hint" placement="bottom">
    <el-button
      circle
      class="locale-toggle"
      :aria-label="hint"
      @click="onToggle"
    >
      <span class="locale-toggle-label">{{ label }}</span>
    </el-button>
  </el-tooltip>
</template>

<style scoped>
.locale-toggle {
  width: 38px;
  height: 38px;
  border-color: var(--ui-border);
  background: color-mix(in srgb, var(--ui-surface-muted) 82%, transparent);
  color: var(--ui-text-muted);
}

.locale-toggle:hover {
  border-color: color-mix(in srgb, var(--ui-primary) 48%, var(--ui-border));
  color: var(--ui-primary);
}

.locale-toggle-label {
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.02em;
}
</style>
