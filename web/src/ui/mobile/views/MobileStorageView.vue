<script setup lang="ts">
import { computed, ref, watch, onMounted } from 'vue';
import {
  mdiDatabase,
  mdiHarddisk,
  mdiLayersTripleOutline,
  mdiRefresh,
  mdiAlertCircle,
  mdiCheckCircle,
  mdiThermometer,
  mdiShieldCheck,
  mdiPlay,
  mdiMagnify,
  mdiFolderOutline,
} from '@mdi/js';
import MdiIcon from '../../components/MdiIcon.vue';
import BottomSheet from '../../components/BottomSheet.vue';
import { nasState, notify, refreshAllData, refreshZfsSubresources } from '../../store';
import { services } from '../../../container';
import { navigateTo } from '../../router';
import type { ZfsPool, Disk, ZfsDataset } from '../../../domain/models';

const activeTab = ref<'pools' | 'disks' | 'datasets'>('pools');
const selectedPool = ref<ZfsPool | null>(null);
const selectedDataset = ref<ZfsDataset | null>(null);
const isPoolSheetOpen = ref(false);
const isDatasetSheetOpen = ref(false);
const scrubLoading = ref(false);
const datasetsLoading = ref(false);
const datasetSearch = ref('');

const pools = computed(() => nasState.pools || []);
const disks = computed(() => nasState.disks || []);

const allDatasets = computed(() => {
  const list: ZfsDataset[] = [];
  for (const pool of pools.value) {
    const pName = pool.name;
    const shortName = pool.poolName || pName.replace(/^zfsPools\//, '');
    const dss = nasState.datasetsByPool[pName] || nasState.datasetsByPool[shortName] || [];
    for (const d of dss) {
      list.push(d);
    }
  }
  if (!datasetSearch.value.trim()) return list;
  const q = datasetSearch.value.toLowerCase();
  return list.filter(
    (d) =>
      d.datasetPath.toLowerCase().includes(q) ||
      (d.mountpoint && d.mountpoint.toLowerCase().includes(q))
  );
});

async function loadDatasets() {
  if (!pools.value || pools.value.length === 0) return;
  datasetsLoading.value = true;
  try {
    await Promise.all(
      pools.value.map((p) => refreshZfsSubresources(p.poolName).catch(() => {}))
    );
  } finally {
    datasetsLoading.value = false;
  }
}

function syncActiveSubTab() {
  if (nasState.activeTab === 'storage-disks') {
    activeTab.value = 'disks';
  } else if (nasState.activeTab === 'storage-datasets') {
    activeTab.value = 'datasets';
    loadDatasets();
  } else {
    activeTab.value = 'pools';
  }
}

watch(
  () => nasState.activeTab,
  () => {
    syncActiveSubTab();
  },
  { immediate: true }
);

function switchTab(tab: 'pools' | 'disks' | 'datasets') {
  activeTab.value = tab;
  if (tab === 'disks') {
    navigateTo('storage-disks');
  } else if (tab === 'datasets') {
    navigateTo('storage-datasets');
    loadDatasets();
  } else {
    navigateTo('storage-pools');
  }
}

onMounted(() => {
  if (activeTab.value === 'datasets') {
    loadDatasets();
  }
});

function formatBytes(bytes?: number): string {
  if (!bytes || bytes <= 0) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

function getPoolUsagePercent(pool: ZfsPool): number {
  if (!pool.sizeBytes || pool.sizeBytes <= 0) return 0;
  return Math.min(100, Math.round((pool.allocatedBytes / pool.sizeBytes) * 100));
}

function openPoolSheet(pool: ZfsPool) {
  selectedPool.value = pool;
  isPoolSheetOpen.value = true;
}

function openDatasetSheet(ds: ZfsDataset) {
  selectedDataset.value = ds;
  isDatasetSheetOpen.value = true;
}

async function handleScrub() {
  if (!selectedPool.value || scrubLoading.value) return;
  scrubLoading.value = true;
  try {
    await services.zfs.scrubPool(selectedPool.value.poolName);
    notify('success', `Scrub started for pool ${selectedPool.value.poolName}`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to start scrub: ${err.message || err}`);
  } finally {
    scrubLoading.value = false;
  }
}
</script>

<template>
  <div class="mobile-storage-view">
    <!-- Sub-Tab Switcher (Pools | Disks | Datasets) -->
    <div class="storage-tab-group">
      <button
        class="storage-tab"
        :class="{ active: activeTab === 'pools' }"
        @click="switchTab('pools')"
      >
        <MdiIcon :path="mdiDatabase" size="15" style="margin-right: 4px" />
        Pools ({{ pools.length }})
      </button>
      <button
        class="storage-tab"
        :class="{ active: activeTab === 'disks' }"
        @click="switchTab('disks')"
      >
        <MdiIcon :path="mdiHarddisk" size="15" style="margin-right: 4px" />
        Disks ({{ disks.length }})
      </button>
      <button
        class="storage-tab"
        :class="{ active: activeTab === 'datasets' }"
        @click="switchTab('datasets')"
      >
        <MdiIcon :path="mdiLayersTripleOutline" size="15" style="margin-right: 4px" />
        Datasets
      </button>
    </div>

    <!-- Pools Tab Content -->
    <div v-if="activeTab === 'pools'" class="storage-list">
      <div
        v-for="pool in pools"
        :key="pool.poolName"
        class="pool-card"
        @click="openPoolSheet(pool)"
      >
        <div class="card-top-row">
          <div class="title-with-icon">
            <MdiIcon :path="mdiDatabase" size="18" style="color: var(--accent-primary)" />
            <span class="pool-title">{{ pool.poolName }}</span>
            <span
              v-if="nasState.systemConfig?.mainPool?.replace(/^zfsPools\//, '') === pool.poolName"
              class="main-pool-chip"
            >
              Main
            </span>
          </div>

          <span
            class="health-chip"
            :class="pool.state === 'ONLINE' ? 'health-online' : 'health-degraded'"
          >
            {{ pool.state }}
          </span>
        </div>

        <div class="pool-usage-row">
          <span class="used-text">{{ formatBytes(pool.allocatedBytes) }} Used</span>
          <span class="total-text">of {{ formatBytes(pool.sizeBytes) }} ({{ getPoolUsagePercent(pool) }}%)</span>
        </div>

        <div class="progress-bar-track">
          <div
            class="progress-bar-fill"
            :style="{
              width: `${getPoolUsagePercent(pool)}%`,
              background: getPoolUsagePercent(pool) > 85 ? 'var(--accent-warning)' : 'var(--accent-primary)',
            }"
          />
        </div>

        <div class="pool-footer-row">
          <span class="scrub-status">
            {{ pool.lastScrubStatus ? `Scrub: ${pool.lastScrubStatus}` : 'No active scrub' }}
          </span>
          <span class="dataset-count">{{ (pool as any).datasetCount || '-' }} datasets</span>
        </div>
      </div>

      <div v-if="pools.length === 0" class="empty-state">
        No ZFS pools found on host.
      </div>
    </div>

    <!-- Disks Tab Content -->
    <div v-else-if="activeTab === 'disks'" class="storage-list">
      <div v-for="disk in disks" :key="disk.devicePath" class="disk-card">
        <div class="card-top-row">
          <div class="disk-title-group">
            <span class="disk-dev mono">{{ disk.devicePath }}</span>
            <span class="disk-model">{{ disk.model || 'Unknown Model' }}</span>
          </div>

          <span class="disk-size-badge">{{ formatBytes(disk.sizeBytes) }}</span>
        </div>

        <div class="disk-meta-row">
          <div class="disk-smart-badge">
            <MdiIcon
              :path="disk.smartStatus?.toLowerCase().includes('fail') ? mdiAlertCircle : mdiShieldCheck"
              size="15"
              :style="{ color: disk.smartStatus?.toLowerCase().includes('fail') ? '#ef4444' : '#4ade80' }"
            />
            <span>SMART: {{ disk.smartStatus || 'OK' }}</span>
          </div>

          <div v-if="disk.temperatureCelsius" class="disk-temp-badge">
            <MdiIcon :path="mdiThermometer" size="14" style="color: #fb923c" />
            <span>{{ disk.temperatureCelsius }}°C</span>
          </div>

          <span v-if="disk.serial" class="disk-serial mono font-xs">{{ disk.serial }}</span>
        </div>
      </div>

      <div v-if="disks.length === 0" class="empty-state">
        No storage drives detected.
      </div>
    </div>

    <!-- Datasets Tab Content -->
    <div v-else-if="activeTab === 'datasets'" class="storage-list">
      <!-- Search & Refresh Row -->
      <div class="dataset-search-row">
        <div class="dataset-search-box">
          <MdiIcon :path="mdiMagnify" size="16" style="color: var(--text-secondary)" />
          <input
            v-model="datasetSearch"
            type="search"
            placeholder="Search datasets or mountpoints..."
            class="dataset-search-input"
          />
        </div>
        <button
          class="btn btn-xs btn-ghost btn-icon"
          :disabled="datasetsLoading"
          title="Reload Datasets"
          @click="loadDatasets"
        >
          <MdiIcon :path="mdiRefresh" size="16" :class="{ 'spin-icon': datasetsLoading }" />
        </button>
      </div>

      <div
        v-for="ds in allDatasets"
        :key="ds.name"
        class="dataset-card"
        @click="openDatasetSheet(ds)"
      >
        <div class="card-top-row">
          <div class="title-with-icon min-w-0">
            <MdiIcon :path="mdiLayersTripleOutline" size="16" style="color: #4ade80; flex-shrink: 0" />
            <span class="dataset-path-text mono">{{ ds.datasetPath }}</span>
          </div>

          <span class="dataset-used-size">{{ formatBytes(ds.usedBytes) }}</span>
        </div>

        <div class="dataset-footer-row">
          <span class="dataset-mount-text mono">
            {{ ds.mountpoint && ds.mountpoint !== 'none' && ds.mountpoint !== 'legacy' ? ds.mountpoint : '-' }}
          </span>
          <div class="dataset-chips">
            <span class="prop-chip">{{ ds.compression || 'lz4' }}</span>
            <span v-if="ds.quotaBytes && ds.quotaBytes > 0" class="prop-chip quota-chip">
              quota: {{ formatBytes(ds.quotaBytes) }}
            </span>
          </div>
        </div>
      </div>

      <div v-if="allDatasets.length === 0 && !datasetsLoading" class="empty-state">
        No datasets found.
      </div>
    </div>

    <!-- Pool Details Bottom Sheet -->
    <BottomSheet
      v-if="selectedPool"
      v-model="isPoolSheetOpen"
      :title="`Pool: ${selectedPool.poolName}`"
      :subtitle="`State: ${selectedPool.state} • Size: ${formatBytes(selectedPool.sizeBytes)}`"
    >
      <div class="sheet-pool-details">
        <!-- Scrub Action -->
        <div class="action-card-box">
          <div class="action-info">
            <span class="action-title">ZFS Scrub Data Integrity</span>
            <span class="action-desc">
              {{ selectedPool.lastScrubStatus ? selectedPool.lastScrubStatus : 'Verifies all checksums across pool VDevs' }}
            </span>
          </div>
          <button
            class="btn btn-sm btn-primary"
            :disabled="scrubLoading"
            @click="handleScrub"
          >
            <MdiIcon :path="mdiPlay" size="14" />
            <span>Start Scrub</span>
          </button>
        </div>

        <!-- Pool Properties -->
        <div class="details-spec-group">
          <div class="spec-item">
            <span class="spec-label">Allocated</span>
            <span class="spec-val">{{ formatBytes(selectedPool.allocatedBytes) }}</span>
          </div>
          <div class="spec-item">
            <span class="spec-label">Free Capacity</span>
            <span class="spec-val">{{ formatBytes(selectedPool.freeBytes) }}</span>
          </div>
          <div class="spec-item">
            <span class="spec-label">Fragmentation</span>
            <span class="spec-val">{{ selectedPool.fragmentationPercent ?? 0 }}%</span>
          </div>
          <div class="spec-item">
            <span class="spec-label">Deduplication</span>
            <span class="spec-val">{{ selectedPool.dedupRatio ? `${selectedPool.dedupRatio.toFixed(2)}x` : '1.00x' }}</span>
          </div>
        </div>
      </div>
    </BottomSheet>

    <!-- Dataset Details Bottom Sheet -->
    <BottomSheet
      v-if="selectedDataset"
      v-model="isDatasetSheetOpen"
      :title="selectedDataset.datasetPath"
      :subtitle="`Mount: ${selectedDataset.mountpoint || 'none'} • Used: ${formatBytes(selectedDataset.usedBytes)}`"
    >
      <div class="sheet-pool-details">
        <div class="details-spec-group">
          <div class="spec-item">
            <span class="spec-label">Used Space</span>
            <span class="spec-val">{{ formatBytes(selectedDataset.usedBytes) }}</span>
          </div>
          <div class="spec-item">
            <span class="spec-label">Available</span>
            <span class="spec-val">{{ formatBytes(selectedDataset.availableBytes) }}</span>
          </div>
          <div class="spec-item">
            <span class="spec-label">Compression</span>
            <span class="spec-val">{{ selectedDataset.compression || 'lz4' }}</span>
          </div>
          <div class="spec-item">
            <span class="spec-label">Quota</span>
            <span class="spec-val">{{ selectedDataset.quotaBytes ? formatBytes(selectedDataset.quotaBytes) : 'None' }}</span>
          </div>
          <div class="spec-item">
            <span class="spec-label">Mountpoint</span>
            <span class="spec-val mono font-xs">{{ selectedDataset.mountpoint || 'none' }}</span>
          </div>
          <div class="spec-item">
            <span class="spec-label">Sync Setting</span>
            <span class="spec-val">{{ selectedDataset.sync || 'standard' }}</span>
          </div>
        </div>
      </div>
    </BottomSheet>
  </div>
</template>

<style scoped>
.mobile-storage-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px 12px calc(40px + env(safe-area-inset-bottom, 0px)) 12px;
}

.storage-tab-group {
  display: flex;
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 12px;
  padding: 4px;
  gap: 4px;
}

.storage-tab {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 8px 4px;
  border-radius: 8px;
  font-size: 11px;
  font-weight: 700;
  background: transparent;
  border: none;
  color: var(--text-secondary, #94a3b8);
  cursor: pointer;
  touch-action: manipulation;
  white-space: nowrap;
}

.storage-tab.active {
  background: var(--accent-primary, #38bdf8);
  color: #0f172a;
}

.storage-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.pool-card,
.disk-card,
.dataset-card {
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 14px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  cursor: pointer;
  touch-action: manipulation;
  transition: transform 0.1s ease, background 0.1s ease;
}

.pool-card:active,
.disk-card:active,
.dataset-card:active {
  transform: scale(0.98);
  background: var(--bg-hover, #334155);
}

.card-top-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.title-with-icon {
  display: flex;
  align-items: center;
  gap: 8px;
}

.min-w-0 {
  min-width: 0;
  overflow: hidden;
}

.pool-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
}

.main-pool-chip {
  font-size: 10px;
  font-weight: 700;
  padding: 1px 6px;
  border-radius: 999px;
  background: rgba(56, 189, 248, 0.15);
  color: var(--accent-primary, #38bdf8);
}

.health-chip {
  font-size: 10px;
  font-weight: 700;
  padding: 2px 6px;
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

.pool-usage-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-top: 2px;
}

.used-text {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
}

.total-text {
  font-size: 11px;
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

.pool-footer-row {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
  margin-top: 4px;
}

.disk-title-group {
  display: flex;
  flex-direction: column;
}

.disk-dev {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
}

.disk-model {
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
}

.disk-size-badge {
  font-size: 13px;
  font-weight: 700;
  color: var(--accent-primary, #38bdf8);
}

.disk-meta-row {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
}

.disk-smart-badge,
.disk-temp-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.disk-serial {
  margin-left: auto;
}

/* Datasets Styles */
.dataset-search-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.dataset-search-box {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 10px;
  padding: 0 10px;
  height: 38px;
}

.dataset-search-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  font-size: 13px;
  color: var(--text-primary, #f8fafc);
}

.dataset-path-text {
  font-size: 12px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.dataset-used-size {
  font-size: 12px;
  font-weight: 700;
  color: var(--accent-primary, #38bdf8);
  white-space: nowrap;
  margin-left: 8px;
}

.dataset-footer-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
}

.dataset-mount-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dataset-chips {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.prop-chip {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.08);
  color: var(--text-secondary, #94a3b8);
}

.quota-chip {
  background: rgba(251, 191, 36, 0.15);
  color: #fbbf24;
}

.sheet-pool-details {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.action-card-box {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--bg-surface-elevated, #243248);
  border: 1px solid var(--border-color, #334155);
  border-radius: 12px;
  padding: 12px;
}

.action-info {
  display: flex;
  flex-direction: column;
}

.action-title {
  font-size: 12px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
}

.action-desc {
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
  margin-top: 2px;
}

.details-spec-group {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}

.spec-item {
  background: var(--bg-surface-elevated, #243248);
  border: 1px solid var(--border-color, #334155);
  border-radius: 10px;
  padding: 10px;
  display: flex;
  flex-direction: column;
}

.spec-label {
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  color: var(--text-secondary, #94a3b8);
}

.spec-val {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
  margin-top: 2px;
}

.empty-state {
  text-align: center;
  padding: 30px;
  color: var(--text-secondary, #94a3b8);
  font-size: 13px;
}
</style>
