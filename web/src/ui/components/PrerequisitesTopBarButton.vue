<script setup lang="ts">
import { computed } from 'vue';
import {
  mdiShieldAlert,
  mdiShieldCheck,
  mdiShieldRemove,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import { nasState } from '../store';

const emit = defineEmits<{
  (e: 'click'): void;
}>();

const installedCount = computed(() => {
  return nasState.prerequisites.filter((p) => p.status === 'INSTALLED').length;
});

const missingList = computed(() => {
  return nasState.prerequisites.filter((p) => p.status !== 'INSTALLED');
});

const missingCount = computed(() => missingList.value.length);

const isStorageMissing = computed(() => {
  return missingList.value.some(
    (p) => p.category === 'STORAGE' || p.name.includes('zfs') || p.name.includes('zpool')
  );
});

const statusLevel = computed<'green' | 'amber' | 'red'>(() => {
  if (nasState.prerequisites.length === 0) {
    return nasState.initialLoaded ? 'red' : 'amber';
  }
  if (missingCount.value === 0 && installedCount.value > 0) {
    return 'green';
  }
  if (isStorageMissing.value || installedCount.value === 0) {
    return 'red';
  }
  return 'amber';
});

const tooltipText = computed(() => {
  if (statusLevel.value === 'green') {
    return `System Prerequisites: All ${installedCount.value} components verified & compliant. Click to view.`;
  }
  if (statusLevel.value === 'amber') {
    return `System Prerequisites: ${missingCount.value} missing (Click to view instructions to fix).`;
  }
  return `System Prerequisites: ${missingCount.value} missing (Critical Storage/System required! Click to fix).`;
});
</script>

<template>
  <button
    class="prereq-topbar-btn"
    :class="`btn-${statusLevel}`"
    :title="tooltipText"
    aria-label="System Prerequisites Status"
    @click="emit('click')"
  >
    <span class="indicator-icon">
      <MdiIcon
        :path="statusLevel === 'green' ? mdiShieldCheck : statusLevel === 'amber' ? mdiShieldAlert : mdiShieldRemove"
        size="16"
      />
    </span>

    <span class="btn-text">
      Prerequisites
    </span>

    <span class="status-pill" :class="`pill-${statusLevel}`">
      {{ statusLevel === 'green' ? 'OK' : `${missingCount} Missing` }}
    </span>
  </button>
</template>

<style scoped>
.prereq-topbar-btn {
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
  border: 1px solid transparent;
}

.prereq-topbar-btn:hover {
  transform: translateY(-1px);
  filter: brightness(1.08);
}

.prereq-topbar-btn:active {
  transform: translateY(0);
}

/* Green Status */
.btn-green {
  background: var(--success-bg, rgba(158, 206, 106, 0.15));
  border-color: var(--success-border, rgba(158, 206, 106, 0.35));
  color: var(--text-primary);
}
.btn-green .indicator-icon {
  color: var(--success-text, #9ece6a);
}
.pill-green {
  background: rgba(158, 206, 106, 0.25);
  color: var(--success-text, #9ece6a);
}

/* Amber Status */
.btn-amber {
  background: var(--warning-bg, rgba(224, 175, 104, 0.15));
  border-color: var(--warning-border, rgba(224, 175, 104, 0.35));
  color: var(--text-primary);
}
.btn-amber .indicator-icon {
  color: var(--warning-text, #e0af68);
}
.pill-amber {
  background: rgba(224, 175, 104, 0.25);
  color: var(--warning-text, #e0af68);
}

/* Red Status */
.btn-red {
  background: var(--danger-bg, rgba(247, 118, 142, 0.15));
  border-color: var(--danger-border, rgba(247, 118, 142, 0.35));
  color: var(--text-primary);
}
.btn-red .indicator-icon {
  color: var(--danger-text, #f7768e);
}
.pill-red {
  background: rgba(247, 118, 142, 0.25);
  color: var(--danger-text, #f7768e);
}

.indicator-icon {
  display: flex;
  align-items: center;
  justify-content: center;
}

.btn-text {
  font-size: 12px;
  font-weight: 600;
}

.status-pill {
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 10px;
  font-weight: 700;
  font-family: var(--font-mono, monospace);
}

@media (max-width: 768px) {
  .btn-text {
    display: none;
  }
}
</style>
