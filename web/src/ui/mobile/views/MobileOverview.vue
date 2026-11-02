<script setup lang="ts">
import { computed, ref } from 'vue';
import {
  mdiCpu64Bit,
  mdiMemory,
  mdiDatabase,
  mdiDocker,
  mdiLayers,
  mdiHarddisk,
  mdiAlertCircle,
  mdiCheckCircle,
  mdiConsole,
  mdiArrowRight,
  mdiThermometer,
  mdiCellphoneArrowDown,
  mdiClose,
  mdiUpdate,
} from '@mdi/js';
import MdiIcon from '../../components/MdiIcon.vue';
import { nasState, refreshAllData } from '../../store';
import { navigateTo } from '../../router';
import { usePwaInstall } from '../../composables/usePwaInstall';

const emit = defineEmits<{
  (e: 'open-terminal'): void;
  (e: 'open-system-details'): void;
  (e: 'open-update'): void;
}>();

const { isStandalone, promptInstall } = usePwaInstall();
const bannerDismissed = ref(false);

const telem = computed(() => nasState.systemTelemetry);

const cpuPercent = computed(() => Math.round(telem.value?.cpu?.usagePercent || 0));
const cpuTemp = computed(() => {
  const t = telem.value?.cpu?.temperatureCelsius;
  return t && t > 0 ? Math.round(t) : null;
});

const memTotal = computed(() => telem.value?.memory?.totalBytes || 1);
const memUsed = computed(() => telem.value?.memory?.usedBytes || 0);
const memPercent = computed(() => Math.min(100, Math.round((memUsed.value / memTotal.value) * 100)));
const memUsedGib = computed(() => (memUsed.value / (1024 * 1024 * 1024)).toFixed(1));
const memTotalGib = computed(() => (memTotal.value / (1024 * 1024 * 1024)).toFixed(1));

// Primary pool metrics
const mainPool = computed(() => {
  const pools = nasState.pools || [];
  if (pools.length === 0) return null;
  const configured = nasState.systemConfig?.mainPool?.replace(/^zfsPools\//, '');
  return pools.find((p) => p.poolName === configured) || pools[0];
});

const mainPoolPercent = computed(() => {
  if (!mainPool.value || !mainPool.value.sizeBytes) return 0;
  return Math.min(100, Math.round((mainPool.value.allocatedBytes / mainPool.value.sizeBytes) * 100));
});

const mainPoolUsedTib = computed(() => {
  if (!mainPool.value) return '0';
  return (mainPool.value.allocatedBytes / (1024 * 1024 * 1024 * 1024)).toFixed(1);
});

const mainPoolTotalTib = computed(() => {
  if (!mainPool.value) return '0';
  return (mainPool.value.sizeBytes / (1024 * 1024 * 1024 * 1024)).toFixed(1);
});

// App / VM counters
const dockerRunningCount = computed(() => {
  return (nasState.dockerApps || []).filter((a) => a.state === 'RUNNING' || a.state === 'running').length;
});
const dockerTotalCount = computed(() => (nasState.dockerApps || []).length);

const vmRunningCount = computed(() => {
  return (nasState.vms || []).filter((v) => v.state === 'RUNNING' || v.state === 'running').length;
});
const vmTotalCount = computed(() => (nasState.vms || []).length);

const lxcRunningCount = computed(() => {
  return (nasState.lxcContainers || []).filter((c) => c.state === 'RUNNING' || c.state === 'running').length;
});
const lxcTotalCount = computed(() => (nasState.lxcContainers || []).length);

const diskHealthyCount = computed(() => {
  const disks = nasState.disks || [];
  if (disks.length === 0) return 0;
  return disks.filter((d) => !d.smartStatus || d.smartStatus.toLowerCase().includes('pass') || d.smartStatus.toLowerCase().includes('ok')).length;
});
const diskTotalCount = computed(() => (nasState.disks || []).length);

const isDegraded = computed(() => {
  return (nasState.pools || []).some((p) => p.state && p.state !== 'ONLINE');
});
</script>

<template>
  <div class="mobile-overview-container">
    <!-- PWA Install Quick Banner -->
    <div v-if="!isStandalone && !bannerDismissed" class="pwa-quick-banner">
      <div class="banner-left" @click="promptInstall">
        <div class="banner-icon">
          <MdiIcon :path="mdiCellphoneArrowDown" size="18" />
        </div>
        <div class="banner-text">
          <span class="banner-title">Install NaN App</span>
          <span class="banner-sub">Add to home screen for native feel</span>
        </div>
      </div>
      <div class="banner-actions">
        <button class="btn btn-xs btn-primary" @click="promptInstall">Install</button>
        <button class="btn btn-xs btn-ghost btn-icon" aria-label="Dismiss banner" @click="bannerDismissed = true">
          <MdiIcon :path="mdiClose" size="14" />
        </button>
      </div>
    </div>

    <!-- Software Update Available Banner -->
    <div
      v-if="nasState.systemUpdate?.updateAvailable"
      class="update-quick-banner"
      @click="emit('open-update')"
    >
      <div class="banner-left">
        <div class="banner-icon-update">
          <MdiIcon :path="mdiUpdate" size="18" />
        </div>
        <div class="banner-text">
          <span class="banner-title">Update Available: {{ nasState.systemUpdate.latestVersion }}</span>
          <span class="banner-sub">Tap to view changelog & update</span>
        </div>
      </div>
      <button class="btn btn-xs btn-warning" @click.stop="emit('open-update')">Update</button>
    </div>

    <!-- System Health Hero Card -->
    <div class="hero-health-card" :class="{ 'hero-warning': isDegraded }">
      <div class="hero-header">
        <div class="health-badge">
          <MdiIcon
            :path="isDegraded ? mdiAlertCircle : mdiCheckCircle"
            size="20"
            :style="{ color: isDegraded ? '#f87171' : '#4ade80' }"
          />
          <span class="health-text">
            {{ isDegraded ? 'Storage Attention Required' : 'All Core Systems Operational' }}
          </span>
        </div>
        <span class="host-pill">{{ telem?.host?.hostname || 'nas01' }}</span>
      </div>

      <div class="hero-meta">
        <span>{{ telem?.host?.osName || 'Debian GNU/Linux' }}</span>
        <span v-if="telem?.host?.kernelVersion"> • {{ telem.host.kernelVersion }}</span>
        <span> • nasctl {{ nasState.systemUpdate?.currentVersion || 'v1.6.8' }}</span>
      </div>
    </div>

    <!-- Live Telemetry Meters -->
    <div class="metric-cards-grid">
      <!-- CPU Metric -->
      <div class="metric-card">
        <div class="metric-header">
          <div class="metric-icon" style="color: #38bdf8">
            <MdiIcon :path="mdiCpu64Bit" size="18" />
          </div>
          <span class="metric-title">CPU</span>
          <span v-if="cpuTemp" class="metric-temp">
            <MdiIcon :path="mdiThermometer" size="13" />
            {{ cpuTemp }}°C
          </span>
        </div>
        <div class="metric-value-row">
          <span class="metric-value">{{ cpuPercent }}%</span>
          <span class="metric-sub">{{ telem?.cpu?.physicalCores || 0 }} cores</span>
        </div>
        <div class="progress-bar-track">
          <div
            class="progress-bar-fill"
            :style="{
              width: `${cpuPercent}%`,
              background: cpuPercent > 85 ? 'var(--accent-danger)' : 'var(--accent-primary)',
            }"
          />
        </div>
      </div>

      <!-- RAM Metric -->
      <div class="metric-card">
        <div class="metric-header">
          <div class="metric-icon" style="color: #c084fc">
            <MdiIcon :path="mdiMemory" size="18" />
          </div>
          <span class="metric-title">Memory</span>
        </div>
        <div class="metric-value-row">
          <span class="metric-value">{{ memPercent }}%</span>
          <span class="metric-sub">{{ memUsedGib }} / {{ memTotalGib }} GB</span>
        </div>
        <div class="progress-bar-track">
          <div
            class="progress-bar-fill"
            :style="{
              width: `${memPercent}%`,
              background: memPercent > 90 ? 'var(--accent-danger)' : '#a855f7',
            }"
          />
        </div>
      </div>
    </div>

    <!-- Storage Pool Card -->
    <div class="storage-hero-card" @click="navigateTo('storage-pools')">
      <div class="storage-card-header">
        <div class="storage-icon-title">
          <MdiIcon :path="mdiDatabase" size="18" style="color: var(--accent-primary)" />
          <span class="storage-pool-name">{{ mainPool?.poolName || 'Primary Storage' }}</span>
          <span
            v-if="mainPool"
            class="pool-health-tag"
            :class="mainPool.state === 'ONLINE' ? 'health-online' : 'health-degraded'"
          >
            {{ mainPool.state }}
          </span>
        </div>
        <MdiIcon :path="mdiArrowRight" size="18" style="color: var(--text-secondary)" />
      </div>

      <div class="storage-usage-row">
        <div>
          <span class="storage-used-val">{{ mainPoolUsedTib }}</span>
          <span class="storage-used-unit"> TiB Used</span>
        </div>
        <span class="storage-total-val">of {{ mainPoolTotalTib }} TiB ({{ mainPoolPercent }}%)</span>
      </div>

      <div class="progress-bar-track" style="height: 8px; margin-top: 8px">
        <div
          class="progress-bar-fill"
          :style="{
            width: `${mainPoolPercent}%`,
            background: mainPoolPercent > 85 ? 'var(--accent-warning)' : 'var(--accent-primary)',
          }"
        />
      </div>
    </div>

    <!-- Workloads & Components Quick Grid -->
    <div class="section-heading">Workloads & Status</div>
    <div class="workloads-grid">
      <!-- Docker Apps -->
      <button class="workload-card" @click="navigateTo('docker')">
        <div class="workload-top">
          <div class="workload-icon" style="color: #38bdf8; background: rgba(56, 189, 248, 0.12)">
            <MdiIcon :path="mdiDocker" size="20" />
          </div>
          <span class="workload-count">{{ dockerRunningCount }} / {{ dockerTotalCount }}</span>
        </div>
        <div class="workload-title">Docker Apps</div>
        <div class="workload-status-sub">{{ dockerRunningCount }} active</div>
      </button>

      <!-- Virtual Machines -->
      <button class="workload-card" @click="navigateTo('kvm')">
        <div class="workload-top">
          <div class="workload-icon" style="color: #818cf8; background: rgba(129, 140, 248, 0.12)">
            <MdiIcon :path="mdiLayers" size="20" />
          </div>
          <span class="workload-count">{{ vmRunningCount }} / {{ vmTotalCount }}</span>
        </div>
        <div class="workload-title">Virtual Machines</div>
        <div class="workload-status-sub">{{ vmRunningCount }} running</div>
      </button>

      <!-- Physical Disks -->
      <button class="workload-card" @click="navigateTo('storage-disks')">
        <div class="workload-top">
          <div class="workload-icon" style="color: #4ade80; background: rgba(74, 222, 128, 0.12)">
            <MdiIcon :path="mdiHarddisk" size="20" />
          </div>
          <span class="workload-count">{{ diskHealthyCount }} / {{ diskTotalCount }}</span>
        </div>
        <div class="workload-title">Disks & SMART</div>
        <div class="workload-status-sub">All reporting OK</div>
      </button>

      <!-- Host Shell -->
      <button class="workload-card" @click="emit('open-terminal')">
        <div class="workload-top">
          <div class="workload-icon" style="color: #fbbf24; background: rgba(251, 191, 36, 0.12)">
            <MdiIcon :path="mdiConsole" size="20" />
          </div>
          <span class="workload-count">CLI</span>
        </div>
        <div class="workload-title">Host Terminal</div>
        <div class="workload-status-sub">Root Shell</div>
      </button>
    </div>
  </div>
</template>

<style scoped>
.mobile-overview-container {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 12px 12px calc(40px + env(safe-area-inset-bottom, 0px)) 12px;
}

.pwa-quick-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: linear-gradient(135deg, rgba(56, 189, 248, 0.12) 0%, rgba(129, 140, 248, 0.12) 100%);
  border: 1px solid rgba(56, 189, 248, 0.3);
  border-radius: 14px;
  padding: 10px 12px;
  gap: 8px;
}

.update-quick-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: linear-gradient(135deg, rgba(245, 158, 11, 0.15) 0%, rgba(217, 119, 6, 0.1) 100%);
  border: 1px solid rgba(245, 158, 11, 0.4);
  border-radius: 14px;
  padding: 10px 12px;
  gap: 8px;
  cursor: pointer;
}

.banner-icon-update {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: rgba(245, 158, 11, 0.25);
  color: #fbbf24;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.banner-left {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  min-width: 0;
  cursor: pointer;
}

.banner-icon {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: rgba(56, 189, 248, 0.2);
  color: var(--accent-primary, #38bdf8);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.banner-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.banner-title {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
}

.banner-sub {
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.banner-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.hero-health-card {
  background: linear-gradient(135deg, rgba(30, 41, 59, 0.8) 0%, rgba(15, 23, 42, 0.9) 100%);
  border: 1px solid rgba(56, 189, 248, 0.2);
  border-radius: 16px;
  padding: 14px 16px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.3);
}

.hero-health-card.hero-warning {
  border-color: rgba(239, 68, 68, 0.4);
}

.hero-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.health-badge {
  display: flex;
  align-items: center;
  gap: 8px;
}

.health-text {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
}

.host-pill {
  font-size: 11px;
  font-family: ui-monospace, monospace;
  padding: 2px 8px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.08);
  color: var(--accent-primary, #38bdf8);
}

.hero-meta {
  margin-top: 8px;
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
}

.metric-cards-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

.metric-card {
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 14px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.metric-header {
  display: flex;
  align-items: center;
  gap: 6px;
}

.metric-title {
  font-size: 12px;
  font-weight: 700;
  color: var(--text-secondary, #94a3b8);
  flex: 1;
}

.metric-temp {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 11px;
  font-weight: 600;
  color: #fb923c;
}

.metric-value-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
}

.metric-value {
  font-size: 20px;
  font-weight: 800;
  color: var(--text-primary, #f8fafc);
}

.metric-sub {
  font-size: 10px;
  color: var(--text-secondary, #94a3b8);
}

.progress-bar-track {
  width: 100%;
  height: 6px;
  background: rgba(255, 255, 255, 0.08);
  border-radius: 999px;
  overflow: hidden;
}

.progress-bar-fill {
  height: 100%;
  border-radius: 999px;
  transition: width 0.3s ease;
}

.storage-hero-card {
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 14px;
  padding: 14px;
  cursor: pointer;
  touch-action: manipulation;
  transition: background 0.15s ease;
}

.storage-hero-card:active {
  background: var(--bg-hover, #334155);
}

.storage-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.storage-icon-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.storage-pool-name {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
}

.pool-health-tag {
  font-size: 10px;
  font-weight: 700;
  padding: 1px 6px;
  border-radius: 4px;
}

.health-online {
  background: rgba(74, 222, 128, 0.15);
  color: #4ade80;
}

.health-degraded {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
}

.storage-usage-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-top: 10px;
}

.storage-used-val {
  font-size: 18px;
  font-weight: 800;
  color: var(--text-primary, #f8fafc);
}

.storage-used-unit {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary, #94a3b8);
}

.storage-total-val {
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
}

.section-heading {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--text-secondary, #94a3b8);
  margin-top: 4px;
}

.workloads-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

.workload-card {
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 14px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  text-align: left;
  cursor: pointer;
  touch-action: manipulation;
  transition: transform 0.15s ease, background 0.15s ease;
}

.workload-card:active {
  transform: scale(0.97);
  background: var(--bg-hover, #334155);
}

.workload-top {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.workload-icon {
  width: 34px;
  height: 34px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.workload-count {
  font-size: 12px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
}

.workload-title {
  font-size: 12px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
}

.workload-status-sub {
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
  margin-top: 2px;
}
</style>
