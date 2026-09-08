<script setup lang="ts">
import { ArrowSync24Regular } from '@vicons/fluent'
import { t } from '../i18n'

defineProps<{
  loading?: boolean
  disabled?: boolean
}>()

defineEmits<{
  (e: 'click'): void
}>()
</script>

<template>
  <el-button :loading="loading" :disabled="disabled" class="!border-0 !bg-white/70 dark:!bg-white/5 ui-action-btn" @click="$emit('click')">
    <el-icon><ArrowSync24Regular /></el-icon>
    {{ t('common.refresh') }}
  </el-button>
</template>
