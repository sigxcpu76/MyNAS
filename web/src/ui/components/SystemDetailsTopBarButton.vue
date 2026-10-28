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
    title="Click to view detailed system hardware, processor, kernel, and platform specs"
    aria-label="System Details"
    @click="emit('click')"
  >
    <span class="indicator-icon">
      <MdiIcon :path="mdiServer" size="16" />
    </span>
    <span class="btn-text">
      {{ hostname }}
    </span>
    <span class="status-pill mono">
      {{ uptime }}
    </span>
  </button>
</template>

<style scoped>
.system-topbar-btn {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 5px 10px;
  border-radius: 8px;
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
  user-select: none;
  background: var(--bg-surface-elevated, var(--bg-surface));
  border: 1px solid var(--border-subtle);
  color: var(--text-primary);
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
