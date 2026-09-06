<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { Dismiss24Regular, Mail24Regular } from '@vicons/fluent'
import { useSmsNotifications } from '../../composables/useSmsNotifications'
import { smsNotificationBadge, smsNotificationTarget } from '../../utils/smsNotifications'

const { unreadCount, notice, error, retry, dismiss } = useSmsNotifications()
const badge = computed(() => smsNotificationBadge(unreadCount.value))
const label = computed(() => error.value ? `短信提醒暂不可用：${error.value}`
  : unreadCount.value === null ? '短信中心，正在获取未读数量'
    : unreadCount.value > 0 ? `短信中心，${unreadCount.value} 条未读短信` : '短信中心，无未读短信')
</script>

<template>
  <RouterLink to="/sms" class="sms-notification-entry" :aria-label="label" :title="label">
    <Mail24Regular aria-hidden="true" />
    <span v-if="error" class="sms-notification-badge sms-notification-error">!</span>
    <span v-else-if="badge" class="sms-notification-badge">{{ badge }}</span>
  </RouterLink>
  <Teleport to="body">
    <section class="sms-notification-region" aria-live="polite" aria-atomic="true">
      <div v-if="notice" class="sms-notification-card">
        <div class="sms-notification-heading">
          <strong>{{ notice.count > 1 ? `收到 ${notice.count} 条新短信` : '收到新短信' }}</strong>
          <button type="button" class="sms-notification-close" aria-label="关闭短信提示，不标为已读" @click="dismiss">
            <Dismiss24Regular aria-hidden="true" />
          </button>
        </div>
        <RouterLink :to="smsNotificationTarget(notice.latest)" class="sms-notification-content" @click="dismiss">
          <strong class="sms-notification-sender">{{ notice.latest.sender || notice.latest.peer || '未知发件人' }}</strong>
          <p>{{ notice.latest.content }}</p>
          <span class="sms-notification-action">查看短信<span aria-hidden="true"> →</span></span>
        </RouterLink>
      </div>
      <div v-if="error" class="sms-notification-status">
        <span>短信提醒暂不可用：{{ error }}</span>
        <button type="button" @click="retry">重试</button>
      </div>
    </section>
  </Teleport>
</template>

<style scoped>
.sms-notification-entry {
  position: relative;
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  flex-shrink: 0;
  border-radius: var(--ui-radius-sm);
  color: var(--ui-text);
}
.sms-notification-entry > svg { width: 22px; height: 22px; }
.sms-notification-badge {
  position: absolute;
  top: 0;
  right: 0;
  min-width: 18px;
  padding: 0 4px;
  border-radius: var(--ui-radius-pill);
  background: var(--ui-primary-solid);
  color: var(--ui-on-primary, white);
  font-size: 12px;
  line-height: 18px;
  text-align: center;
}
.sms-notification-error { background: var(--ui-danger); }
.sms-notification-region {
  position: fixed;
  top: calc(76px + env(safe-area-inset-top, 0px));
  right: max(16px, env(safe-area-inset-right, 0px));
  width: min(360px, calc(100vw - 32px));
  z-index: 1999;
  pointer-events: none;
}
.sms-notification-card, .sms-notification-status {
  pointer-events: auto;
  border: 1px solid var(--ui-border);
  border-radius: var(--ui-radius-md);
  background: var(--ui-surface-strong);
  color: var(--ui-text);
  box-shadow: var(--ui-shadow-lg);
}
.sms-notification-heading { display: flex; align-items: center; justify-content: space-between; padding-left: 16px; font-size: 14px; }
.sms-notification-close { display: grid; place-items: center; width: 44px; height: 44px; color: var(--ui-muted); background: transparent; border: 0; cursor: pointer; border-radius: var(--ui-radius-sm); }
.sms-notification-close svg { width: 20px; height: 20px; }
.sms-notification-content { display: block; padding: 0 16px 16px; color: inherit; text-decoration: none; border-radius: var(--ui-radius-md); }
.sms-notification-sender { display: block; overflow-wrap: anywhere; font-size: 14px; }
.sms-notification-content p { margin: 6px 0 12px; color: var(--ui-muted); line-height: 1.5; font-size: 14px; overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.sms-notification-action { color: var(--ui-primary); font-weight: 600; font-size: 14px; }
.sms-notification-entry:hover, .sms-notification-close:hover, .sms-notification-content:hover { background: var(--ui-selected); }
.sms-notification-entry:focus-visible, .sms-notification-content:focus-visible, button:focus-visible { outline: 2px solid var(--ui-primary); outline-offset: 2px; }
.sms-notification-status { margin-top: 8px; padding: 12px 16px; font-size: 14px; display: flex; align-items: center; gap: 8px; }
.sms-notification-status span { flex: 1; overflow-wrap: anywhere; }
.sms-notification-status button { min-width: 44px; min-height: 44px; background: transparent; border: 0; color: var(--ui-primary); cursor: pointer; }
</style>
