<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import {
  mdiContentCopy,
  mdiFormatLineSpacing,
  mdiMagnify,
  mdiMenuDown,
  mdiMenuUp,
  mdiRefresh,
  mdiWrap,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import { ansiToHtml, stripAnsi } from '../utils/ansi';
import { notify } from '../store';

const props = withDefaults(
  defineProps<{
    logs: string;
    loading?: boolean;
    maxHeight?: string;
    showControls?: boolean;
    emptyText?: string;
    title?: string;
  }>(),
  {
    loading: false,
    maxHeight: '540px',
    showControls: true,
    emptyText: 'No log entries recorded.',
    title: 'Console Output',
  }
);

const emit = defineEmits<{
  (e: 'refresh'): void;
}>();

const logContainerRef = ref<HTMLDivElement | null>(null);
const searchQuery = ref('');
const isAutoScroll = ref(true);
const isWrapLines = ref(true);

// Processed HTML with ANSI colors
const renderedHtml = computed(() => {
  if (!props.logs) return '';
  const lines = props.logs.split('\n');

  // Filter lines if searchQuery is present
  const query = searchQuery.value.trim().toLowerCase();
  const filtered = query
    ? lines.filter((l) => stripAnsi(l).toLowerCase().includes(query))
    : lines;

  return ansiToHtml(filtered.join('\n'));
});

const matchCount = computed(() => {
  const query = searchQuery.value.trim().toLowerCase();
  if (!query || !props.logs) return 0;
  const lines = props.logs.split('\n');
  return lines.filter((l) => stripAnsi(l).toLowerCase().includes(query)).length;
});

function scrollToBottom() {
  if (!logContainerRef.value || !isAutoScroll.value) return;
  logContainerRef.value.scrollTop = logContainerRef.value.scrollHeight;
}

function handleScroll() {
  if (!logContainerRef.value) return;
  const el = logContainerRef.value;
  const threshold = 30;
  const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight <= threshold;
  if (!atBottom && isAutoScroll.value) {
    isAutoScroll.value = false;
  }
}

function toggleAutoScroll() {
  isAutoScroll.value = !isAutoScroll.value;
  if (isAutoScroll.value) {
    scrollToBottom();
  }
}

function copyLogs() {
  if (!props.logs) return;
  const clean = stripAnsi(props.logs);
  navigator.clipboard.writeText(clean);
  notify('info', 'Logs copied to clipboard (clean plain text)');
}

watch(
  () => props.logs,
  () => {
    if (isAutoScroll.value) {
      nextTick(scrollToBottom);
    }
  }
);
</script>

<template>
  <div class="log-viewer-wrapper">
    <!-- Log Viewer Controls Bar -->
    <div v-if="showControls" class="log-controls-bar">
      <!-- Optional Prepend Controls Slot -->
      <slot name="controls-start" />

      <!-- Search Filter -->
      <div class="log-search-box">
        <MdiIcon :path="mdiMagnify" size="14" class="search-icon" />
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Filter logs..."
          class="log-search-input mono"
        />
        <span v-if="searchQuery.trim()" class="match-pill mono font-xs">
          {{ matchCount }} {{ matchCount === 1 ? 'line' : 'lines' }}
        </span>
      </div>

      <!-- Quick Action Buttons -->
      <div class="log-actions">
        <slot name="actions-start" />

        <!-- Wrap Lines Toggle -->
        <button
          type="button"
          class="btn btn-xs log-btn"
          :class="{ active: isWrapLines }"
          :title="isWrapLines ? 'Line wrap enabled (click to disable)' : 'Line wrap disabled (click to wrap)'"
          @click="isWrapLines = !isWrapLines"
        >
          <MdiIcon :path="mdiWrap" size="13" />
          <span class="btn-label">Wrap</span>
        </button>

        <!-- Auto-Scroll Toggle -->
        <button
          type="button"
          class="btn btn-xs log-btn"
          :class="{ active: isAutoScroll }"
          :title="isAutoScroll ? 'Auto-scroll following output' : 'Click to enable auto-scroll'"
          @click="toggleAutoScroll"
        >
          <MdiIcon :path="mdiMenuDown" size="14" />
          <span class="btn-label">{{ isAutoScroll ? 'Following' : 'Follow' }}</span>
        </button>

        <!-- Copy Clean Plain Text -->
        <button
          type="button"
          class="btn btn-xs log-btn"
          :disabled="!logs"
          title="Copy clean logs to clipboard"
          @click="copyLogs"
        >
          <MdiIcon :path="mdiContentCopy" size="13" />
          <span class="btn-label">Copy</span>
        </button>

        <!-- Refresh Trigger -->
        <button
          type="button"
          class="btn btn-xs log-btn"
          :disabled="loading"
          title="Refresh log output"
          @click="emit('refresh')"
        >
          <MdiIcon :path="mdiRefresh" size="13" :class="{ 'spin-icon': loading }" />
        </button>
      </div>
    </div>

    <!-- Terminal Screen Pre Output -->
    <div
      ref="logContainerRef"
      class="terminal-viewport mono font-xs"
      :class="{ 'wrap-text': isWrapLines }"
      :style="{ maxHeight: maxHeight }"
      @scroll="handleScroll"
    >
      <div v-if="loading && !logs" class="loading-state">
        <MdiIcon :path="mdiRefresh" size="16" class="spin-icon text-accent" />
        <span>Loading logs...</span>
      </div>

      <div v-else-if="!logs || logs.trim() === ''" class="empty-state">
        {{ emptyText }}
      </div>

      <!-- ANSI Rendered Colored HTML -->
      <pre v-else class="ansi-terminal-content" v-html="renderedHtml"></pre>
    </div>
  </div>
</template>

<style scoped>
.log-viewer-wrapper {
  display: flex;
  flex-direction: column;
  width: 100%;
  border-radius: 6px;
  overflow: hidden;
  border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.08));
  background: #090d16;
}

.log-controls-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 8px 12px;
  background: var(--bg-elevated, #111827);
  border-bottom: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.08));
  flex-wrap: wrap;
}

.log-search-box {
  display: flex;
  align-items: center;
  gap: 6px;
  background: var(--bg-base, #030712);
  border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.12));
  border-radius: 4px;
  padding: 2px 8px;
  min-width: 200px;
  flex: 1;
  max-width: 320px;
}

.search-icon {
  color: var(--text-muted, #94a3b8);
  flex-shrink: 0;
}

.log-search-input {
  background: transparent;
  border: none;
  color: var(--text-primary, #f8fafc);
  font-size: 11.5px;
  width: 100%;
  outline: none;
}

.match-pill {
  background: var(--bg-tertiary, #1f2937);
  color: var(--accent-primary, #38bdf8);
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 3px;
  white-space: nowrap;
}

.log-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.log-btn {
  background: var(--bg-tertiary, #1f2937);
  color: var(--text-secondary, #cbd5e1);
  border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.08));
  padding: 3px 8px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
}

.log-btn:hover {
  background: var(--bg-hover, rgba(255, 255, 255, 0.1));
  color: var(--text-primary, #ffffff);
}

.log-btn.active {
  background: rgba(56, 189, 248, 0.15);
  color: var(--accent-primary, #38bdf8);
  border-color: rgba(56, 189, 248, 0.4);
}

.btn-label {
  font-size: 11px;
}

/* Terminal Viewport */
.terminal-viewport {
  background: #090d16;
  color: #d1d5db;
  padding: 12px 16px;
  overflow-y: auto;
  overflow-x: auto;
  line-height: 1.5;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  font-size: 12px;
}

.terminal-viewport.wrap-text .ansi-terminal-content {
  white-space: pre-wrap;
  word-break: break-all;
}

.terminal-viewport:not(.wrap-text) .ansi-terminal-content {
  white-space: pre;
}

.ansi-terminal-content {
  margin: 0;
  padding: 0;
  font-family: inherit;
  font-size: inherit;
  line-height: inherit;
  background: transparent;
}

.loading-state,
.empty-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 36px 12px;
  color: var(--text-muted, #94a3b8);
  font-size: 12px;
}
</style>
