<script setup lang="ts">
import { computed } from 'vue';
import { mdiServer } from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import { nasState } from '../store';
import { formatUptime } from '../../domain/models';

const emit = defineEmits<{
  (e: 'click'): void;
}>();

const telemetry = computed(() => nasState.systemTelemetry);
const hostname = computed(() => telemetry.value?.host?.hostname || 'System');
const uptime = computed(() => formatUptime(telemetry.value?.host?.uptimeSeconds || 0));
</script>

<template>
  <button
    class="system-topbar-btn"
    :title="`System Details: ${hostname} (Uptime: ${uptime}) — Click to view hardware, CPU, kernel & platform specs`"
    aria-label="System Details"
    @click="emit('click')"
  >
    <span class="indicator-icon">
      <MdiIcon :path="mdiServer" size="16" />
    </span>
    <span class="sr-only">{{ hostname }} {{ uptime }}</span>
  </button>
</template>

<style scoped>
.system-topbar-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 5px 7px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
  user-select: none;
  background: var(--bg-surface-elevated, var(--bg-surface));
  border: 1px solid var(--border-subtle);
  color: var(--text-primary);
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

.system-topbar-btn:hover {
  transform: translateY(-1px);
  border-color: var(--accent-primary);
  filter: brightness(1.06);
}

.system-topbar-btn:active {
  transform: translateY(0);
}

.indicator-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--accent-primary);
}

.btn-text {
  font-size: 12px;
  font-weight: 600;
}

.status-pill {
  font-size: 10.5px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: 10px;
  background: var(--bg-surface-hover, rgba(255, 255, 255, 0.08));
  color: var(--text-secondary);
}
</style>
