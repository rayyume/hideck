<script setup lang="ts">
import LoadingScreen from '../components/LoadingScreen.vue'
import SwitchDark from '../components/SwitchDark.vue'

defineProps({
  isDark: {
    type: Boolean,
    required: true
  }
})

const emit = defineEmits(['toggle-theme'])
</script>

<template>
  <div class="unauthenticated-shell">
    <div class="unauthenticated-theme">
      <SwitchDark :is-dark="isDark" @toggle="(e) => emit('toggle-theme', e)" />
    </div>
    <router-view v-slot="{ Component }">
      <Suspense>
        <template #default>
          <component :is="Component" />
        </template>
        <template #fallback>
          <LoadingScreen />
        </template>
      </Suspense>
    </router-view>
  </div>
</template>

<style scoped>
.unauthenticated-shell {
  position: relative;
  min-height: 100%;
  overflow: auto;
  background: var(--ui-bg);
}

.unauthenticated-theme {
  position: absolute;
  top: 18px;
  right: 20px;
  z-index: 50;
}

@media (max-width: 640px) {
  .unauthenticated-theme {
    top: 12px;
    right: 12px;
  }
}
</style>
