<script setup lang="ts">
import { formatBytes } from '../../../domain/models';
import { nasState } from '../../store';
import { navigateTo } from '../../router';

function getTempClass(temp?: number): string {
  if (!temp || temp <= 0) return 'badge-neutral';
  if (temp < 45) return 'badge-success';
  if (temp < 55) return 'badge-warning';
  return 'badge-danger';
}
</script>

<template>
  <div class="card disks-widget-card">
    <div class="card-header" style="border-bottom: 1px solid var(--border-subtle); padding-bottom: 8px; margin-bottom: 12px">
      <div style="display: flex; align-items: center; gap: 8px">
        <h3 class="card-title">Physical Disks</h3>
        <span class="badge badge-neutral mono">{{ nasState.disks.length }} Drives</span>
      </div>
      <button class="btn btn-sm btn-primary" @click="navigateTo('storage-disks')">
        Manage Disks
      </button>
    </div>

    <div v-if="nasState.disks.length === 0" class="card-subtitle" style="padding: 24px 0; text-align: center">
      No physical drives detected.
    </div>

    <div v-else class="disks-list">
      <div
        v-for="disk in nasState.disks"
        :key="disk.devicePath || disk.name"
        class="disk-item"
      >
        <div class="disk-meta">
          <div style="display: flex; align-items: center; gap: 6px">
            <strong class="mono" style="font-size: 13.5px; color: var(--accent-primary)">
              {{ disk.name.replace(/^disks\//, '') }}
            </strong>
            <span v-if="disk.transport" class="badge badge-neutral mono" style="font-size: 10px; padding: 1px 5px">
              {{ disk.transport }}
            </span>
            <span v-if="disk.type" class="badge badge-info" style="font-size: 10px; padding: 1px 5px">
              {{ disk.type }}
            </span>
          </div>
          <div class="card-subtitle" style="font-size: 11.5px; margin-top: 2px">
            {{ disk.model || disk.vendor || 'Generic Disk' }}
            <span v-if="disk.serial" class="mono" style="color: var(--text-muted); margin-left: 4px">
              ({{ disk.serial }})
            </span>
          </div>
        </div>

        <div class="disk-stats">
          <span class="mono" style="font-weight: 600; font-size: 13px">
            {{ formatBytes(disk.sizeBytes) }}
          </span>
          <div style="display: flex; align-items: center; gap: 6px; margin-top: 2px">
            <span
              v-if="(disk.temperatureCelsius || 0) > 0"
              class="badge mono"
              :class="getTempClass(disk.temperatureCelsius)"
              style="font-size: 10px; padding: 1px 5px"
            >
              {{ disk.temperatureCelsius }}°C
            </span>
            <span
              class="badge mono"
              :class="disk.smartStatus === 'PASSED' ? 'badge-success' : disk.smartStatus === 'FAILED' ? 'badge-danger' : 'badge-neutral'"
              style="font-size: 10px; padding: 1px 5px"
            >
              {{ disk.smartStatus || 'OK' }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.disks-widget-card {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.disks-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  overflow-y: auto;
  max-height: 280px;
}

.disk-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 10px;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  gap: 12px;
}

.disk-meta {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.disk-stats {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  flex-shrink: 0;
}
</style>
