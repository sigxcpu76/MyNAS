<script setup lang="ts">
import { computed } from 'vue';
import { formatBytes } from '../../../domain/models';
import { nasState } from '../../store';

const telemetry = computed(() => nasState.systemTelemetry);

const packageTemps = computed<number[]>(() => {
  const temps = telemetry.value?.cpu?.packageTemperatures;
  if (Array.isArray(temps) && temps.length > 0) {
    return temps;
  }
  const t = telemetry.value?.cpu?.temperatureCelsius;
  return t && t > 0 ? [t] : [];
});

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
      <div class="kpi-progress-track">
        <div
          class="kpi-progress-bar"
          :style="{
            width: `${totalStorageBytes > 0 ? Math.min((usedStorageBytes / totalStorageBytes) * 100, 100) : 0}%`,
            background: totalStorageBytes > 0 && (usedStorageBytes / totalStorageBytes) > 0.85 ? 'var(--danger)' : 'var(--success)'
          }"
        ></div>
      </div>
    </div>

    <div class="card kpi-card">
      <div class="card-header" style="margin-bottom: 6px">
        <span class="card-subtitle">Processor & Load</span>
        <div v-if="packageTemps.length > 1" style="display: flex; gap: 4px; align-items: center">
          <span
            v-for="(t, idx) in packageTemps"
            :key="idx"
            class="badge"
            :class="getTempClass(t)"
            style="font-size: 11px; font-weight: 600"
            :title="`CPU Socket ${idx} Temperature`"
          >
            CPU{{ idx }}: {{ t.toFixed(0) }}°C
          </span>
        </div>
        <span
          v-else-if="(telemetry?.cpu?.temperatureCelsius || 0) > 0"
          class="badge"
          :class="getTempClass(telemetry?.cpu?.temperatureCelsius || 0)"
        >
          {{ telemetry?.cpu?.temperatureCelsius.toFixed(1) }} °C
        </span>
      </div>
      <div style="font-size: 22px; font-weight: 700">
        {{ (telemetry?.cpu?.usagePercent || 0).toFixed(1) }}%
        <span style="font-size: 13px; color: var(--text-secondary); font-weight: 400">
          load: {{ telemetry?.cpu?.load1?.toFixed(2) || '0.00' }} · {{ telemetry?.cpu?.load5?.toFixed(2) || '0.00' }}
        </span>
      </div>
      <div class="kpi-progress-track">
        <div
          class="kpi-progress-bar"
          :style="{
            width: `${Math.min(Math.max(telemetry?.cpu?.usagePercent || 0, 0), 100)}%`,
            background: (telemetry?.cpu?.usagePercent || 0) > 85 ? 'var(--danger)' : (telemetry?.cpu?.usagePercent || 0) > 65 ? 'var(--warning)' : 'var(--accent-primary)'
          }"
        ></div>
      </div>
      <p class="card-subtitle" style="margin-top: 6px; margin-bottom: 0">
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
      <div class="kpi-progress-track" style="display: flex">
        <div
          title="Services & Apps"
          :style="{
            width: `${telemetry?.memory?.totalBytes ? Math.min(((telemetry.memory.servicesBytes || ((telemetry.memory.usedBytes || 0) - (telemetry.memory.zfsArcBytes || 0))) / telemetry.memory.totalBytes) * 100, 100) : 0}%`,
            background: 'var(--accent-primary)',
            height: '100%'
          }"
        ></div>
        <div
          title="ZFS ARC Cache"
          :style="{
            width: `${telemetry?.memory?.totalBytes ? Math.min(((telemetry.memory.zfsArcBytes || 0) / telemetry.memory.totalBytes) * 100, 100) : 0}%`,
            background: 'var(--info, #0284c7)',
            height: '100%'
          }"
        ></div>
      </div>
      <p class="card-subtitle" style="margin-top: 6px; margin-bottom: 0">
        Services: {{ formatBytes(telemetry?.memory?.servicesBytes || 0) }} · ARC: {{ formatBytes(telemetry?.memory?.zfsArcBytes || 0) }} · Free: {{ formatBytes(telemetry?.memory?.freeBytes || 0) }}
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
      <p class="card-subtitle" style="margin-top: 10px; margin-bottom: 0">
        {{ activeShares }} Shares · {{ nasState.users.length }} Users · {{ nasState.lxcContainers.length }} LXC
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

.kpi-progress-track {
  height: 4px;
  border-radius: 2px;
  background: var(--border-subtle);
  margin-top: 6px;
  overflow: hidden;
}

.kpi-progress-bar {
  height: 100%;
  border-radius: 2px;
  transition: width 0.3s ease;
}
</style>
