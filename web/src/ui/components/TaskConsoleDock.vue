<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import {
  mdiAlertCircle,
  mdiCheckCircle,
  mdiChevronDown,
  mdiChevronUp,
  mdiClipboardTextClockOutline,
  mdiClose,
  mdiContentCopy,
  mdiLoading,
  mdiTrashCanOutline,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import {
  clearTasks,
  formatDurationMs,
  nasState,
  notify,
  setTaskConsoleCollapsed,
  type TaskRecord,
} from '../store';

const filter = ref<'all' | 'running' | 'errors'>('all');
const activeTaskModal = ref<TaskRecord | null>(null);
const nowTimestamp = ref(Date.now());
let timerInterval: any = null;

onMounted(() => {
  timerInterval = setInterval(() => {
    nowTimestamp.value = Date.now();
  }, 500);
});

onUnmounted(() => {
  if (timerInterval) clearInterval(timerInterval);
});

const filteredTasks = computed(() => {
  const list = nasState.tasks;
  if (filter.value === 'running') {
    return list.filter((t) => t.status === 'RUNNING');
  }
  if (filter.value === 'errors') {
    return list.filter((t) => t.status === 'ERROR');
  }
  return list;
});

const runningCount = computed(
  () => nasState.tasks.filter((t) => t.status === 'RUNNING').length
);
const errorCount = computed(
  () => nasState.tasks.filter((t) => t.status === 'ERROR').length
);

function toggleCollapse() {
  setTaskConsoleCollapsed(!nasState.isTaskConsoleCollapsed);
}

function getLiveDuration(task: TaskRecord): string {
  if (task.status === 'RUNNING') {
    return formatDurationMs(nowTimestamp.value - task.startTimestamp);
  }
  return task.duration || '0s';
}

function openTaskDetails(task: TaskRecord) {
  activeTaskModal.value = task;
}

function closeTaskDetails() {
  activeTaskModal.value = null;
}

function copyLogs(task: TaskRecord) {
  const content = task.logs.join('\n');
  if (navigator?.clipboard) {
    navigator.clipboard.writeText(content).then(() => {
      notify('success', 'Task logs copied to clipboard');
    });
  }
}
</script>

<template>
  <div
    class="task-console-dock"
    :class="{ collapsed: nasState.isTaskConsoleCollapsed }"
  >
    <!-- Dock Header Toolbar -->
    <div class="dock-header" @click.self="toggleCollapse">
      <div class="dock-header-left" @click="toggleCollapse">
        <MdiIcon
          :path="mdiClipboardTextClockOutline"
          size="16"
          style="color: var(--accent-primary)"
        />
        <span class="dock-title">Task & Audit Log</span>

        <span
          v-if="runningCount > 0"
          class="badge badge-warning pulse-anim mono"
          style="font-size: 10px; display: inline-flex; align-items: center; gap: 4px"
        >
          <MdiIcon :path="mdiLoading" size="11" class="spin-icon" />
          {{ runningCount }} RUNNING
        </span>
        <span
          v-if="errorCount > 0"
          class="badge badge-danger mono"
          style="font-size: 10px"
        >
          {{ errorCount }} ERRORS
        </span>
        <span class="dock-count-muted mono">({{ nasState.tasks.length }})</span>
      </div>

      <div class="dock-header-center">
        <!-- Quick Filters -->
        <div class="dock-filter-group" v-if="!nasState.isTaskConsoleCollapsed">
          <button
            class="filter-btn"
            :class="{ active: filter === 'all' }"
            @click.stop="filter = 'all'"
          >
            All
          </button>
          <button
            class="filter-btn"
            :class="{ active: filter === 'running' }"
            @click.stop="filter = 'running'"
          >
            Running ({{ runningCount }})
          </button>
          <button
            class="filter-btn"
            :class="{ active: filter === 'errors' }"
            @click.stop="filter = 'errors'"
          >
            Errors ({{ errorCount }})
          </button>
        </div>
      </div>

      <div class="dock-header-right">
        <button
          v-if="!nasState.isTaskConsoleCollapsed"
          class="dock-action-btn"
          title="Clear Completed Tasks"
          @click.stop="clearTasks"
        >
          <MdiIcon :path="mdiTrashCanOutline" size="14" />
          <span class="hide-mobile">Clear</span>
        </button>

        <button
          class="dock-action-btn toggle-btn"
          :title="nasState.isTaskConsoleCollapsed ? 'Expand Task Console' : 'Collapse Task Console'"
          @click.stop="toggleCollapse"
        >
          <MdiIcon
            :path="nasState.isTaskConsoleCollapsed ? mdiChevronUp : mdiChevronDown"
            size="17"
          />
          <span class="hide-mobile">
            {{ nasState.isTaskConsoleCollapsed ? 'Expand' : 'Collapse' }}
          </span>
        </button>
      </div>
    </div>

    <!-- Dock Table Body -->
    <div v-show="!nasState.isTaskConsoleCollapsed" class="dock-body">
      <div v-if="filteredTasks.length === 0" class="dock-empty">
        No {{ filter !== 'all' ? filter : '' }} tasks recorded.
      </div>
      <table v-else class="dock-table">
        <thead>
          <tr>
            <th style="width: 80px">Start</th>
            <th style="width: 80px">End</th>
            <th style="width: 90px">Node</th>
            <th style="width: 85px">User</th>
            <th>Description & Parameters</th>
            <th style="width: 95px">Status</th>
            <th style="width: 80px">Duration</th>
            <th style="width: 65px; text-align: center">Action</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="task in filteredTasks"
            :key="task.id"
            class="task-row"
            :class="{ 'row-error': task.status === 'ERROR', 'row-running': task.status === 'RUNNING' }"
            @dblclick="openTaskDetails(task)"
          >
            <td class="mono font-sm">{{ task.startTime }}</td>
            <td class="mono font-sm text-muted">
              <span v-if="task.endTime">{{ task.endTime }}</span>
              <span v-else class="pulse-anim" style="color: var(--accent-primary)">running</span>
            </td>
            <td class="mono font-sm text-secondary">{{ task.node }}</td>
            <td class="mono font-sm text-secondary">{{ task.user }}</td>
            <td class="task-desc-cell">
              <div class="task-desc-text font-sm">{{ task.description }}</div>
              <div v-if="task.details" class="task-details-sub mono font-xs">{{ task.details }}</div>
            </td>
            <td>
              <span
                class="badge mono font-xs"
                :class="{
                  'badge-success': task.status === 'OK',
                  'badge-danger': task.status === 'ERROR',
                  'badge-warning pulse-anim': task.status === 'RUNNING',
                }"
              >
                <MdiIcon
                  v-if="task.status === 'RUNNING'"
                  :path="mdiLoading"
                  size="11"
                  class="spin-icon"
                  style="margin-right: 3px"
                />
                <MdiIcon
                  v-else-if="task.status === 'OK'"
                  :path="mdiCheckCircle"
                  size="11"
                  style="margin-right: 3px"
                />
                <MdiIcon
                  v-else
                  :path="mdiAlertCircle"
                  size="11"
                  style="margin-right: 3px"
                />
                {{ task.status }}
              </span>
            </td>
            <td class="mono font-sm text-muted">{{ getLiveDuration(task) }}</td>
            <td style="text-align: center">
              <button
                class="btn btn-xs btn-ghost"
                title="Inspect Task Log Output"
                @click.stop="openTaskDetails(task)"
              >
                Log
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Task Log Inspection Modal / Drawer -->
    <div
      v-if="activeTaskModal"
      class="task-modal-backdrop"
      @click.self="closeTaskDetails"
    >
      <div class="task-modal-box">
        <div class="task-modal-header">
          <div style="display: flex; align-items: center; gap: 8px">
            <MdiIcon :path="mdiClipboardTextClockOutline" size="18" style="color: var(--accent-primary)" />
            <h3 class="task-modal-title">Task Inspector: {{ activeTaskModal.description }}</h3>
          </div>
          <div style="display: flex; align-items: center; gap: 8px">
            <button
              class="btn btn-xs btn-secondary"
              title="Copy all logs to clipboard"
              @click="copyLogs(activeTaskModal)"
            >
              <MdiIcon :path="mdiContentCopy" size="13" style="margin-right: 4px" />
              Copy
            </button>
            <button
              class="btn btn-xs btn-icon"
              style="border: none; background: transparent; cursor: pointer"
              @click="closeTaskDetails"
            >
              <MdiIcon :path="mdiClose" size="18" />
            </button>
          </div>
        </div>

        <div class="task-modal-meta">
          <span class="meta-tag mono">ID: {{ activeTaskModal.id }}</span>
          <span class="meta-tag mono">Node: {{ activeTaskModal.node }}</span>
          <span class="meta-tag mono">User: {{ activeTaskModal.user }}</span>
          <span class="meta-tag mono">Started: {{ activeTaskModal.startTime }}</span>
          <span v-if="activeTaskModal.endTime" class="meta-tag mono">Ended: {{ activeTaskModal.endTime }}</span>
          <span class="meta-tag mono">Duration: {{ getLiveDuration(activeTaskModal) }}</span>
          <span
            class="badge mono"
            :class="{
              'badge-success': activeTaskModal.status === 'OK',
              'badge-danger': activeTaskModal.status === 'ERROR',
              'badge-warning pulse-anim': activeTaskModal.status === 'RUNNING',
            }"
          >
            {{ activeTaskModal.status }}
          </span>
        </div>

        <div class="task-modal-terminal">
          <div
            v-for="(logLine, idx) in activeTaskModal.logs"
            :key="idx"
            class="terminal-line"
            :class="{
              'line-error': logLine.includes('[ERROR]') || logLine.includes('Failed'),
              'line-warn': logLine.includes('[WARN]'),
              'line-ok': logLine.includes('[OK]'),
            }"
          >
            {{ logLine }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.task-console-dock {
  background: var(--bg-surface);
  border-top: 1px solid var(--border-strong);
  display: flex;
  flex-direction: column;
  height: 175px;
  flex-shrink: 0;
  transition: height 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  z-index: 40;
}

.task-console-dock.collapsed {
  height: 32px;
}

.dock-header {
  height: 32px;
  background: var(--sidebar-bg);
  border-bottom: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  cursor: pointer;
  user-select: none;
}

.dock-header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.dock-title {
  font-size: 12px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: -0.01em;
}

.dock-count-muted {
  font-size: 11px;
  color: var(--text-muted);
}

.dock-header-center {
  display: flex;
  align-items: center;
}

.dock-filter-group {
  display: flex;
  gap: 2px;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  padding: 1px;
}

.filter-btn {
  background: transparent;
  border: none;
  font-size: 11px;
  color: var(--text-secondary);
  padding: 2px 8px;
  border-radius: 3px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.filter-btn.active {
  background: var(--accent-primary);
  color: var(--accent-contrast, #fff);
  font-weight: 600;
}

.dock-header-right {
  display: flex;
  align-items: center;
  gap: 6px;
}

.dock-action-btn {
  background: transparent;
  border: 1px solid transparent;
  color: var(--text-secondary);
  font-size: 11.5px;
  padding: 2px 6px;
  border-radius: 4px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.dock-action-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
  border-color: var(--border-subtle);
}

.dock-body {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
  background: var(--bg-base);
}

.dock-empty {
  padding: 24px;
  text-align: center;
  color: var(--text-muted);
  font-size: 12px;
}

.dock-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.dock-table th {
  position: sticky;
  top: 0;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border-subtle);
  color: var(--text-muted);
  font-weight: 600;
  text-align: left;
  padding: 4px 8px;
  font-size: 11px;
  z-index: 10;
}

.dock-table td {
  padding: 4px 8px;
  border-bottom: 1px solid var(--border-subtle);
  white-space: nowrap;
}

.task-row {
  cursor: pointer;
  transition: background-color 0.1s ease;
}

.task-row:hover {
  background: var(--bg-hover);
}

.row-error {
  background: rgba(247, 118, 142, 0.05);
}

.task-desc-cell {
  max-width: 450px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-desc-text {
  font-weight: 500;
  color: var(--text-primary);
}

.task-details-sub {
  color: var(--text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
}

.font-sm {
  font-size: 11.5px;
}

.font-xs {
  font-size: 10px;
}

/* Inspector Modal / Slide-over */
.task-modal-backdrop {
  position: fixed;
  inset: 0;
  background: var(--modal-backdrop, rgba(0, 0, 0, 0.6));
  z-index: 9999;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
}

.task-modal-box {
  width: 760px;
  max-width: 95vw;
  max-height: 85vh;
  background: var(--bg-surface);
  border: 1px solid var(--border-strong);
  border-radius: 8px;
  box-shadow: var(--modal-shadow);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.task-modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-subtle);
  background: var(--sidebar-bg);
}

.task-modal-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
}

.task-modal-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  background: var(--bg-base);
  border-bottom: 1px solid var(--border-subtle);
}

.meta-tag {
  font-size: 11px;
  background: var(--bg-elevated);
  padding: 2px 6px;
  border-radius: 4px;
  color: var(--text-secondary);
  border: 1px solid var(--border-subtle);
}

.task-modal-terminal {
  flex: 1;
  background: #0d1117;
  color: #c9d1d9;
  padding: 12px 16px;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.6;
  overflow-y: auto;
  max-height: 480px;
}

.terminal-line {
  white-space: pre-wrap;
  word-break: break-all;
}

.line-error {
  color: #f85149;
}

.line-warn {
  color: #d29922;
}

.line-ok {
  color: #3fb950;
}
</style>
