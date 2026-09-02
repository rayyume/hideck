<script setup lang="ts">
import type { WorkspacePreviewModel } from '../utils/workspacePreview'

defineProps<{
  model: WorkspacePreviewModel
}>()

const emit = defineEmits<{
  select: [id: string]
  open: []
}>()
</script>

<template>
  <article
    class="workspace-preview"
    :class="{ 'is-demo': model.demo, 'is-empty': model.empty }"
    :aria-label="model.demo ? '工作区预览（示例数据）' : '仪表盘'"
  >
    <header class="preview-head">
      <div>
        <span class="preview-kicker">{{ model.kicker }}</span>
        <button
          v-if="!model.demo && !model.empty"
          type="button"
          class="preview-title-button"
          @click="emit('open')"
        >
          <h2>{{ model.title }}</h2>
        </button>
        <h2 v-else>{{ model.title }}</h2>
      </div>
      <strong class="preview-signal">{{ model.signal }}</strong>
    </header>

    <p v-if="model.demo" class="preview-demo-note">示例数据，登录后显示本机模组</p>

    <div class="preview-body">
      <section class="preview-calendar" aria-label="信号日历">
        <span class="preview-section-label">{{ model.calendarLabel }}</span>
        <ol>
          <li
            v-for="day in model.days"
            :key="day.key"
            :class="{ 'is-selected': day.selected }"
          >
            <span class="calendar-day" :aria-current="day.selected ? 'date' : undefined">{{ day.day }}</span>
            <span class="calendar-weekday">{{ day.weekday }}</span>
            <span class="calendar-signal">{{ day.signal }}</span>
          </li>
        </ol>
      </section>

      <section class="preview-modules" aria-label="选择模组">
        <span class="preview-section-label">选择模组</span>
        <div v-if="model.modules.length" class="module-list">
          <button
            v-for="item in model.modules"
            :key="item.id"
            type="button"
            class="module-chip"
            :class="{ 'is-selected': item.selected, 'is-online': item.online }"
            :disabled="model.demo"
            :aria-pressed="item.selected"
            @click="emit('select', item.id)"
          >
            <strong>{{ item.label }}</strong>
            <span>{{ item.status }}</span>
          </button>
        </div>
        <p v-else class="module-empty">暂无模组。添加设备后会出现在这里。</p>
      </section>
    </div>
  </article>
</template>

<style scoped>
.workspace-preview {
  position: relative;
  padding: 28px 30px 26px;
  border: 1px solid var(--ui-border);
  border-radius: 28px;
  background: var(--ui-surface);
  box-shadow: var(--ui-shadow-lg);
  color: var(--ui-text);
}

.preview-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.preview-kicker,
.preview-section-label,
.preview-demo-note {
  color: var(--ui-muted);
  font-size: var(--ui-font-caption);
  font-weight: 600;
}

.preview-title-button {
  display: block;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.preview-head h2 {
  margin: 6px 0 0;
  color: var(--ui-text);
  font-size: 22px;
  font-weight: 700;
  line-height: 1.2;
}

.preview-signal {
  color: var(--ui-text);
  font-size: 28px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.03em;
}

.preview-demo-note {
  margin: 8px 0 0;
}

.preview-body {
  margin-top: 22px;
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(180px, 0.85fr);
  gap: 28px;
}

.preview-calendar ol {
  margin: 12px 0 0;
  padding: 0;
  display: grid;
  gap: 6px;
  list-style: none;
}

.preview-calendar li {
  min-height: 40px;
  padding: 4px 10px 4px 6px;
  display: grid;
  grid-template-columns: 32px minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  border-radius: 999px;
}

.preview-calendar li.is-selected {
  background: var(--ui-selected);
}

.calendar-day {
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  border-radius: 50%;
  color: var(--ui-text);
  font-size: 13px;
  font-weight: 700;
}

.preview-calendar li.is-selected .calendar-day {
  background: var(--ui-accent);
  color: #fff;
}

.calendar-weekday {
  color: var(--ui-text);
  font-size: 13px;
}

.calendar-signal {
  color: var(--ui-muted);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}

.preview-calendar li.is-selected .calendar-signal {
  color: var(--ui-accent);
  font-weight: 700;
}

.module-list {
  margin-top: 12px;
  display: grid;
  gap: 10px;
}

.module-chip {
  min-height: 56px;
  padding: 10px 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border: 1px solid var(--ui-border);
  border-radius: 16px;
  background: var(--ui-surface-muted);
  color: var(--ui-text);
  text-align: left;
  cursor: pointer;
}

.module-chip:disabled {
  cursor: default;
}

.module-chip strong {
  font-size: 15px;
  font-weight: 700;
}

.module-chip span {
  color: var(--ui-accent);
  font-size: 13px;
  font-weight: 650;
}

.module-chip.is-selected {
  border-color: transparent;
  background: var(--ui-accent);
  color: #fff;
  box-shadow: 0 10px 22px color-mix(in srgb, var(--ui-accent) 28%, transparent);
}

.module-chip.is-selected span {
  color: #fff;
}

.module-empty {
  margin: 14px 0 0;
  color: var(--ui-muted);
  font-size: 13px;
  line-height: 1.5;
}

@media (max-width: 720px) {
  .workspace-preview {
    padding: 22px 18px 20px;
    border-radius: 22px;
  }

  .preview-head {
    flex-direction: column;
  }

  .preview-signal {
    font-size: 24px;
  }

  .preview-body {
    grid-template-columns: 1fr;
    gap: 20px;
  }
}
</style>
