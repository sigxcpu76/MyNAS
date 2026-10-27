<script setup lang="ts">
import { computed } from 'vue';
import { formatBytes } from '../../../domain/models';
import { nasState } from '../../store';

const telemetry = computed(() => nasState.systemTelemetry);

const totalStorageBytes = computed(() =>
  nasState.pools.reduce((acc, p) => acc + (p.sizeBytes || 0), 0)
);
const usedStorageBytes = computed(() =>
  nasState.pools.reduce((acc, p) => acc + (p.allocatedBytes || 0), 0)
);
const runningVms = computed(() =>
  nasState.vms.filter((v) => v.state === 'RUNNING').length
);
const runningApps = computed(() =>
  nasState.dockerApps.filter((a) => a.state === 'RUNNING').length
);
const runningLxc = computed(() =>
  nasState.lxcContainers.filter((c) => c.state === 'RUNNING').length
);
const activeShares = computed(
  () =>
    nasState.cifsShares.filter((s) => s.enabled).length +
    nasState.webdavShares.filter((s) => s.enabled).length
);

function getTempClass(temp: number): string {
  if (temp <= 0) return 'badge-neutral';
  if (temp < 65) return 'badge-success';
  if (temp < 80) return 'badge-warning';
  return 'badge-danger';
}
</script>

<template>
  <div class="kpi-grid">
    <div class="card kpi-card">
      <div class="card-header" style="margin-bottom: 6px">
        <span class="card-subtitle">Storage</span>
        <span class="badge badge-success">{{ nasState.pools.length }} Pools</span>
      </div>
      <div style="font-size: 22px; font-weight: 700">
        {{ formatBytes(usedStorageBytes) }}
        <span style="font-size: 13px; color: var(--text-secondary); font-weight: 400">
          / {{ formatBytes(totalStorageBytes) }}
        </span>
      </div>
    </div>

    <div class="card kpi-card">
      <div class="card-header" style="margin-bottom: 6px">
        <span class="card-subtitle">Processor & Load</span>
        <span
          v-if="(telemetry?.cpu?.temperatureCelsius || 0) > 0"
          class="badge"
          :class="getTempClass(telemetry?.cpu?.temperatureCelsius || 0)"
        >
          {{ telemetry?.cpu?.temperatureCelsius.toFixed(1) }} °C
        </span>
      </div>
      <div style="font-size: 22px; font-weight: 700">
        {{ (telemetry?.cpu?.usagePercent || 0).toFixed(1) }}%
        <span style="font-size: 13px; color: var(--text-secondary); font-weight: 400">
          load avg: {{ telemetry?.cpu?.load1?.toFixed(2) || '0.00' }}
        </span>
      </div>
      <p class="card-subtitle" style="margin-top: 6px">
        {{ telemetry?.cpu?.logicalCores || 0 }} Logical Threads ({{ telemetry?.cpu?.physicalCores || 0 }} Cores)
      </p>
    </div>

    <div class="card kpi-card">
      <div class="card-header" style="margin-bottom: 6px">
        <span class="card-subtitle">Memory (RAM)</span>
        <span class="badge badge-info">
          {{ formatBytes(telemetry?.memory?.totalBytes || 0) }} Total
        </span>
      </div>
      <div style="font-size: 22px; font-weight: 700">
        {{ formatBytes(telemetry?.memory?.usedBytes || 0) }}
        <span style="font-size: 13px; color: var(--text-secondary); font-weight: 400">
          Used ({{ telemetry?.memory?.totalBytes ? Math.round(((telemetry.memory.usedBytes || 0) / telemetry.memory.totalBytes) * 100) : 0 }}%)
        </span>
      </div>
      <p class="card-subtitle" style="margin-top: 6px">
        ZFS ARC: {{ formatBytes(telemetry?.memory?.zfsArcBytes || 0) }} · Free: {{ formatBytes(telemetry?.memory?.freeBytes || 0) }}
      </p>
    </div>

    <div class="card kpi-card">
      <div class="card-header" style="margin-bottom: 6px">
        <span class="card-subtitle">Workloads & Shares</span>
        <span class="badge badge-success">
          {{ runningVms + runningApps + runningLxc }} Active
        </span>
      </div>
      <div style="font-size: 22px; font-weight: 700">
        {{ nasState.vms.length }} VMs · {{ nasState.dockerApps.length }} Apps
      </div>
      <p class="card-subtitle" style="margin-top: 6px">
        {{ activeShares }} Shares · {{ nasState.users.length }} Users
      </p>
    </div>
  </div>
</template>

<style scoped>
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 14px;
  width: 100%;
}

.kpi-card {
  padding: 16px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
}
</style>
