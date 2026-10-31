<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import {
  mdiAlertCircle,
  mdiArrowLeft,
  mdiCamera,
  mdiCheckCircle,
  mdiClose,
  mdiCog,
  mdiDatabase,
  mdiDatabaseCheck,
  mdiDatabaseSync,
  mdiDelete,
  mdiFolderOutline,
  mdiHarddisk,
  mdiInformationOutline,
  mdiLayers,
  mdiPencil,
  mdiPlus,
  mdiRefresh,
  mdiTrashCanOutline,
} from '@mdi/js';
import MdiIcon from '../components/MdiIcon.vue';
import SkeletonBox from '../components/SkeletonBox.vue';
import { services } from '../../container';
import {
  formatBytes,
  bytesToGib,
  gibToBytes,
  type ZfsDataset,
  type ZfsPool,
  type ZfsVolume,
  isDotDataset,
} from '../../domain/models';
import {
  nasState,
  notify,
  refreshAllData,
  refreshZfsSubresources,
  runTrackedTask,
  selectResource,
} from '../store';
import { confirmDelete } from '../modal';

const props = withDefaults(
  defineProps<{
    poolName: string;
    activeSubTab?: 'summary' | 'datasets' | 'volumes' | 'topology' | 'config';
  }>(),
  {
    activeSubTab: 'summary',
  }
);

const emit = defineEmits<{
  (e: 'back'): void;
  (e: 'update:activeSubTab', tab: 'summary' | 'datasets' | 'volumes' | 'topology' | 'config'): void;
}>();

const isBusy = ref(false);

// Clean short pool name (e.g. "fast" from "zfsPools/fast" or "fast")
const cleanName = computed(() => {
  return props.poolName.replace(/^zfsPools\//, '').trim();
});

// Resolve the active pool object from store
const pool = computed<ZfsPool | null>(() => {
  const target = cleanName.value;
  return (
    nasState.pools.find(
      (p) => (p.poolName || p.name.replace(/^zfsPools\//, '')) === target
    ) || null
  );
});

const isMainPool = computed(() => {
  if (!pool.value || !nasState.systemConfig?.mainPool) return false;
  return cleanName.value === nasState.systemConfig.mainPool;
});

// Pool Datasets and Volumes
const poolDatasets = computed<ZfsDataset[]>(() => {
  const list = nasState.datasetsByPool[cleanName.value] || [];
  return list.filter((d) => !isDotDataset(d));
});

const poolVolumes = computed<ZfsVolume[]>(() => {
  return nasState.volumesByPool[cleanName.value] || [];
});

// Load subresources when pool changes
watch(
  () => cleanName.value,
  async (name) => {
    if (name) {
      await refreshZfsSubresources(name);
    }
  },
  { immediate: true }
);

// -----------------------------------------------------------------------------
// Actions
// -----------------------------------------------------------------------------
async function handleScrub() {
  if (!pool.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`ZFS Pool "${cleanName.value}": Start Scrub`, async () => {
      await services.zfs.scrubPool(pool.value!.name);
    });
    notify('success', `Data scrub started on pool "${cleanName.value}"`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Scrub failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleSetMainPool() {
  if (!pool.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`System: Set Main Pool to "${cleanName.value}"`, async () => {
      await services.system.updateSystemConfig({
        mainPool: cleanName.value,
      });
    });
    notify('success', `Designated "${cleanName.value}" as main storage pool`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to set main pool: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleExportPool() {
  if (!pool.value || isBusy.value) return;
  const isMain = isMainPool.value;
  const confirmed = await confirmDelete({
    kind: 'Export ZFS Pool',
    name: cleanName.value,
    details: isMain
      ? 'WARNING: This is the MAIN POOL. Exporting it will pause Apps and Containers!'
      : 'The pool will be unmounted and safely detached. You can re-import it anytime.',
    confirmText: 'Export Pool',
  });
  if (!confirmed.confirmed) return;

  isBusy.value = true;
  try {
    await runTrackedTask(`ZFS Pool "${cleanName.value}": Export`, async () => {
      await services.zfs.exportPool(pool.value!.name);
    });
    notify('info', `Pool "${cleanName.value}" exported`);
    emit('back');
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Export failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleDestroyPool() {
  if (!pool.value || isBusy.value) return;
  const confirmed = await confirmDelete({
    kind: 'Destroy Pool',
    name: cleanName.value,
    details: 'PERMANENT DATA LOSS: All datasets, snapshots, and volumes in this pool will be deleted permanently.',
    confirmText: 'Destroy Pool Permanently',
  });
  if (!confirmed.confirmed) return;

  isBusy.value = true;
  try {
    await runTrackedTask(`ZFS Pool "${cleanName.value}": Destroy`, async () => {
      await services.zfs.deletePool(pool.value!.name);
    });
    notify('warning', `Pool "${cleanName.value}" destroyed`);
    emit('back');
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Destroy failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

// -----------------------------------------------------------------------------
// Pool Properties Configuration Form
// -----------------------------------------------------------------------------
const configForm = reactive({
  sync: 'standard',
  autotrim: 'off',
  compression: 'lz4',
});

watch(
  pool,
  (p) => {
    if (p) {
      configForm.sync = p.sync || 'standard';
      configForm.autotrim = p.autotrim || 'off';
      configForm.compression = p.compression || 'lz4';
    }
  },
  { immediate: true }
);

async function handleSaveConfig() {
  if (!pool.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`ZFS Pool "${cleanName.value}": Update Properties`, async () => {
      await services.zfs.updatePool(pool.value!.name, {
        sync: configForm.sync,
        autotrim: configForm.autotrim,
        compression: configForm.compression,
      });
    });
    notify('success', `Pool properties saved for "${cleanName.value}"`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to update pool: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

// -----------------------------------------------------------------------------
// Dataset Creation Modal State
// -----------------------------------------------------------------------------
const showCreateDatasetModal = ref(false);
const datasetForm = reactive({
  datasetId: '',
  mountpoint: '',
  compression: 'lz4',
  recordsize: 131072,
});

function openCreateDataset() {
  datasetForm.datasetId = '';
  datasetForm.mountpoint = `/mnt/${cleanName.value}/`;
  datasetForm.compression = pool.value?.compression || 'lz4';
  datasetForm.recordsize = 131072;
  showCreateDatasetModal.value = true;
}

async function submitCreateDataset() {
  if (!datasetForm.datasetId.trim() || isBusy.value) return;
  isBusy.value = true;
  const id = datasetForm.datasetId.trim().replace(/^\/+/, '');
  try {
    await runTrackedTask(`ZFS Dataset "${cleanName.value}/${id}": Create`, async () => {
      await services.zfs.createDataset(
        pool.value!.name,
        id,
        {
          mountpoint: datasetForm.mountpoint.trim() || undefined,
          compression: datasetForm.compression,
        }
      );
    });
    notify('success', `Dataset "${cleanName.value}/${id}" created`);
    showCreateDatasetModal.value = false;
    await refreshZfsSubresources(cleanName.value);
  } catch (err: any) {
    notify('error', `Failed to create dataset: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleDeleteDataset(ds: ZfsDataset) {
  const shortName = ds.name.replace(/^zfsPools\/[^/]+\/datasets\//, '');
  const confirmed = await confirmDelete({
    kind: 'Dataset',
    name: `${cleanName.value}/${shortName}`,
    details: 'All data and nested snapshots within this dataset will be destroyed.',
    confirmText: 'Delete Dataset',
  });
  if (!confirmed.confirmed) return;

  isBusy.value = true;
  try {
    await runTrackedTask(`ZFS Dataset "${cleanName.value}/${shortName}": Delete`, async () => {
      await services.zfs.deleteDataset(ds.name);
    });
    notify('info', `Dataset "${shortName}" deleted`);
    await refreshZfsSubresources(cleanName.value);
  } catch (err: any) {
    notify('error', `Failed to delete dataset: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

// -----------------------------------------------------------------------------
// Volume (ZVol) Creation Modal State
// -----------------------------------------------------------------------------
const showCreateVolumeModal = ref(false);
const volumeForm = reactive({
  volumeId: '',
  sizeGib: 20,
  volblocksize: 16384,
  sparse: true,
});

function openCreateVolume() {
  volumeForm.volumeId = '';
  volumeForm.sizeGib = 20;
  volumeForm.volblocksize = 16384;
  volumeForm.sparse = true;
  showCreateVolumeModal.value = true;
}

async function submitCreateVolume() {
  if (!volumeForm.volumeId.trim() || isBusy.value) return;
  isBusy.value = true;
  const id = volumeForm.volumeId.trim().replace(/^\/+/, '');
  try {
    await runTrackedTask(`ZFS Volume "${cleanName.value}/${id}": Create`, async () => {
      await services.zfs.createVolume(
        pool.value!.name,
        id,
        {
          sizeBytes: gibToBytes(volumeForm.sizeGib),
          volblocksize: String(volumeForm.volblocksize),
          thinProvisioned: volumeForm.sparse,
        }
      );
    });
    notify('success', `Volume "${cleanName.value}/${id}" created`);
    showCreateVolumeModal.value = false;
    await refreshZfsSubresources(cleanName.value);
  } catch (err: any) {
    notify('error', `Failed to create volume: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleDeleteVolume(vol: ZfsVolume) {
  const shortName = vol.name.replace(/^zfsPools\/[^/]+\/volumes\//, '');
  const confirmed = await confirmDelete({
    kind: 'Volume (zvol)',
    name: `${cleanName.value}/${shortName}`,
    details: 'The block device will be permanently destroyed. Virtual machines using this disk will lose access!',
    confirmText: 'Delete Volume',
  });
  if (!confirmed.confirmed) return;

  isBusy.value = true;
  try {
    await runTrackedTask(`ZFS Volume "${cleanName.value}/${shortName}": Delete`, async () => {
      await services.zfs.deleteVolume(vol.name);
    });
    notify('info', `Volume "${shortName}" deleted`);
    await refreshZfsSubresources(cleanName.value);
  } catch (err: any) {
    notify('error', `Failed to delete volume: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

// -----------------------------------------------------------------------------
// Disks belonging to this pool
// -----------------------------------------------------------------------------
const poolDisks = computed(() => {
  if (!pool.value) return [];
  const devPaths = new Set<string>();
  if (pool.value.devices) {
    for (const d of pool.value.devices) devPaths.add(d);
  }
  if (pool.value.vdevs) {
    for (const v of pool.value.vdevs) {
      if (v.path) devPaths.add(v.path);
      if (v.children) {
        for (const c of v.children) {
          if (c.path) devPaths.add(c.path);
          if (c.name) devPaths.add(c.name);
        }
      }
    }
  }

  return nasState.disks.filter((d) => {
    if (d.owningZfsPool === cleanName.value) return true;
    if (devPaths.has(d.devicePath)) return true;
    if (devPaths.has(d.name)) return true;
    return false;
  });
});
</script>

<template>
  <div class="pool-detail-view">
    <!-- Subtab 1: Pool Summary -->
    <div v-if="props.activeSubTab === 'summary'" class="pool-tab-content">
      <div class="grid-2">
        <!-- Card 1: Capacity & Allocation -->
        <div class="card detail-card">
          <div class="card-header">
            <h3 class="card-title">Storage Allocation & Capacity</h3>
          </div>
          <div class="card-body">
            <div v-if="pool" class="gauge-section">
              <div class="capacity-stats">
                <span class="capacity-used">
                  {{ formatBytes(pool.allocatedBytes) }} used
                </span>
                <span class="capacity-total text-muted">
                  of {{ formatBytes(pool.sizeBytes) }} ({{ pool.capacityPercent }}%)
                </span>
              </div>
              <div class="progress-bar-wrap">
                <div
                  class="progress-fill"
                  :style="{
                    width: `${Math.min(100, pool.capacityPercent)}%`,
                    backgroundColor: pool.capacityPercent > 85 ? 'var(--danger)' : pool.capacityPercent > 70 ? 'var(--warning)' : 'var(--accent-primary)',
                  }"
                ></div>
              </div>
              <div class="capacity-substats font-sm text-secondary">
                <span>Free: {{ formatBytes(pool.freeBytes) }}</span>
                <span>Fragmentation: {{ pool.fragmentationPercent }}%</span>
              </div>
            </div>

            <div class="metrics-grid">
              <div class="metric-item">
                <span class="metric-label">VDEV Topology</span>
                <span class="metric-val mono badge badge-neutral">{{ (pool?.vdevLayout || 'stripe').toUpperCase() }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Deduplication</span>
                <span class="metric-val mono">{{ pool?.dedupRatio ? pool.dedupRatio.toFixed(2) + 'x' : '1.00x' }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Compression</span>
                <span class="metric-val mono">{{ pool?.compression || 'lz4' }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Sync Policy</span>
                <span class="metric-val mono">{{ pool?.sync || 'standard' }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Autotrim</span>
                <span class="metric-val mono">{{ pool?.autotrim || 'off' }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Health State</span>
                <span class="metric-val mono" style="color: var(--success-text)">{{ pool?.state || 'ONLINE' }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Card 2: Scrub & Health Status -->
        <div class="card detail-card">
          <div class="card-header">
            <h3 class="card-title">Data Integrity & Scrub Status</h3>
          </div>
          <div class="card-body">
            <div class="scrub-status-box">
              <div style="display: flex; align-items: center; gap: 8px">
                <MdiIcon :path="mdiCheckCircle" size="20" style="color: var(--success)" />
                <span style="font-weight: 600">{{ pool?.lastScrubStatus || 'No scrub errors recorded' }}</span>
              </div>
              <p class="font-sm text-secondary" style="margin-top: 8px">
                ZFS scrubs verify checksum integrity on all data blocks to detect and heal silent bit rot.
              </p>
              <div style="margin-top: 14px">
                <button class="btn btn-sm btn-secondary" :disabled="isBusy" @click="handleScrub">
                  <MdiIcon :path="mdiDatabaseSync" size="14" style="margin-right: 4px" />
                  Trigger Scrub Now
                </button>
              </div>
            </div>

            <div style="margin-top: 20px">
              <h4 class="font-sm" style="font-weight: 600; margin-bottom: 8px">Physical Storage Drives</h4>
              <div v-if="poolDisks.length === 0" class="text-muted font-sm">
                No physical drive records linked.
              </div>
              <div v-else class="table-wrap">
                <table class="data-table">
                  <thead>
                    <tr>
                      <th>Device</th>
                      <th>Model</th>
                      <th>Capacity</th>
                      <th>Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="d in poolDisks" :key="d.devicePath">
                      <td class="mono font-sm" style="font-weight: 600">{{ d.devicePath }}</td>
                      <td class="font-sm text-secondary">{{ d.model || d.vendor || 'Unknown' }}</td>
                      <td class="mono font-sm">{{ formatBytes(d.sizeBytes) }}</td>
                      <td>
                        <span class="badge badge-success font-xs">ONLINE</span>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Subtab 2: Pool Datasets -->
    <div v-else-if="props.activeSubTab === 'datasets'" class="pool-tab-content">
      <div class="card">
        <div class="card-header">
          <div>
            <h3 class="card-title">Datasets in {{ cleanName }}</h3>
            <p class="card-subtitle">File systems hosted on pool {{ cleanName }}</p>
          </div>
          <button class="btn btn-sm btn-primary" @click="openCreateDataset">
            <MdiIcon :path="mdiPlus" size="15" style="margin-right: 4px" />
            Create Dataset
          </button>
        </div>

        <div class="table-wrap">
          <div v-if="poolDatasets.length === 0" class="empty-state">
            No child datasets created on pool {{ cleanName }}.
          </div>
          <table v-else class="data-table">
            <thead>
              <tr>
                <th>Dataset Name</th>
                <th>Used</th>
                <th>Available</th>
                <th>Referenced</th>
                <th>Mountpoint</th>
                <th>Compression</th>
                <th style="width: 80px; text-align: center">Action</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="ds in poolDatasets" :key="ds.name">
                <td style="font-weight: 600">
                  <div style="display: flex; align-items: center; gap: 6px">
                    <MdiIcon :path="mdiFolderOutline" size="15" style="color: var(--accent-primary)" />
                    <span>{{ ds.name.replace(/^zfsPools\/[^/]+\/datasets\//, '') }}</span>
                  </div>
                </td>
                <td class="mono font-sm">{{ formatBytes(ds.usedBytes) }}</td>
                <td class="mono font-sm text-muted">{{ formatBytes(ds.availableBytes) }}</td>
                <td class="mono font-sm text-muted">{{ formatBytes(ds.referencedBytes) }}</td>
                <td class="mono font-sm text-secondary">{{ ds.mountpoint || '-' }}</td>
                <td class="mono font-sm">{{ ds.compression || 'lz4' }}</td>
                <td style="text-align: center">
                  <button
                    class="btn btn-xs btn-ghost"
                    title="Delete Dataset"
                    @click="handleDeleteDataset(ds)"
                  >
                    <MdiIcon :path="mdiDelete" size="14" style="color: var(--danger-text)" />
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Subtab 3: Pool Volumes (ZVols) -->
    <div v-else-if="props.activeSubTab === 'volumes'" class="pool-tab-content">
      <div class="card">
        <div class="card-header">
          <div>
            <h3 class="card-title">Volumes (ZVols) in {{ cleanName }}</h3>
            <p class="card-subtitle">Block devices hosted on pool {{ cleanName }}</p>
          </div>
          <button class="btn btn-sm btn-primary" @click="openCreateVolume">
            <MdiIcon :path="mdiPlus" size="15" style="margin-right: 4px" />
            Create Volume
          </button>
        </div>

        <div class="table-wrap">
          <div v-if="poolVolumes.length === 0" class="empty-state">
            No block volumes (zvols) created on pool {{ cleanName }}.
          </div>
          <table v-else class="data-table">
            <thead>
              <tr>
                <th>Volume Name</th>
                <th>Volsize</th>
                <th>Used Space</th>
                <th>Block Size</th>
                <th style="width: 80px; text-align: center">Action</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="vol in poolVolumes" :key="vol.name">
                <td style="font-weight: 600">
                  <div style="display: flex; align-items: center; gap: 6px">
                    <MdiIcon :path="mdiHarddisk" size="15" style="color: var(--accent-secondary)" />
                    <span>{{ vol.name.replace(/^zfsPools\/[^/]+\/volumes\//, '') }}</span>
                  </div>
                </td>
                <td class="mono font-sm">{{ formatBytes(vol.sizeBytes) }}</td>
                <td class="mono font-sm text-muted">{{ formatBytes(vol.usedBytes) }}</td>
                <td class="mono font-sm text-secondary">{{ vol.volblocksize || '16K' }}</td>
                <td style="text-align: center">
                  <button
                    class="btn btn-xs btn-ghost"
                    title="Delete Volume"
                    @click="handleDeleteVolume(vol)"
                  >
                    <MdiIcon :path="mdiDelete" size="14" style="color: var(--danger-text)" />
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Subtab 4: Topology & Disks -->
    <div v-else-if="props.activeSubTab === 'topology'" class="pool-tab-content">
      <div class="card">
        <div class="card-header">
          <div>
            <h3 class="card-title">VDEV Architecture & Physical Disk Topology</h3>
            <p class="card-subtitle">Storage devices currently forming pool {{ cleanName }}</p>
          </div>
        </div>

        <div class="card-body">
          <div class="table-wrap">
            <table class="data-table">
              <thead>
                <tr>
                  <th>Device / VDEV</th>
                  <th>Type / Role</th>
                  <th>Model / Vendor</th>
                  <th>Capacity</th>
                  <th>Transport</th>
                  <th>State</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="d in poolDisks" :key="d.devicePath">
                  <td class="mono font-sm" style="font-weight: 600">{{ d.devicePath }}</td>
                  <td class="font-sm"><span class="badge badge-neutral mono">{{ pool?.vdevLayout || 'mirror' }}</span></td>
                  <td class="font-sm text-secondary">{{ d.model || d.vendor || '-' }}</td>
                  <td class="mono font-sm">{{ formatBytes(d.sizeBytes) }}</td>
                  <td class="mono font-xs text-muted">{{ d.transport || 'SATA/NVMe' }}</td>
                  <td><span class="badge badge-success font-xs">ONLINE</span></td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    <!-- Subtab 5: Configuration Properties -->
    <div v-else-if="props.activeSubTab === 'config'" class="pool-tab-content">
      <div class="card" style="max-width: 720px">
        <div class="card-header">
          <div>
            <h3 class="card-title">Pool Properties</h3>
            <p class="card-subtitle">Configure synchronization, compression, and trimming</p>
          </div>
        </div>

        <div class="card-body" style="display: flex; flex-direction: column; gap: 16px">
          <div class="form-group">
            <label class="form-label">Sync Policy</label>
            <select v-model="configForm.sync" class="form-select">
              <option value="standard">standard (Default POSIX fsync handling)</option>
              <option value="always">always (Force synchronous writes to SLOG/ZIL)</option>
              <option value="disabled">disabled (Ignore fsync, maximize write throughput)</option>
            </select>
          </div>

          <div class="form-group">
            <label class="form-label">Autotrim (SSD / NVMe TRIM)</label>
            <select v-model="configForm.autotrim" class="form-select">
              <option value="off">off (TRIM disabled)</option>
              <option value="on">on (Continuous background TRIM of freed LBAs)</option>
            </select>
          </div>

          <div class="form-group">
            <label class="form-label">Default Compression Algorithm</label>
            <select v-model="configForm.compression" class="form-select">
              <option value="lz4">lz4 (Recommended - High throughput, low CPU)</option>
              <option value="zstd">zstd (Higher ratio, modern Zstandard)</option>
              <option value="gzip">gzip (Legacy high compression)</option>
              <option value="off">off (No compression)</option>
            </select>
          </div>

          <div style="display: flex; justify-content: flex-end; margin-top: 10px">
            <button class="btn btn-primary" :disabled="isBusy" @click="handleSaveConfig">
              Save Properties
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Create Dataset Modal -->
    <div v-if="showCreateDatasetModal" class="modal-overlay" @click.self="showCreateDatasetModal = false">
      <div class="modal-container" style="max-width: 520px">
        <div class="modal-header">
          <h3 class="modal-title">Create Dataset in {{ cleanName }}</h3>
          <button class="btn btn-xs btn-ghost" @click="showCreateDatasetModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <div class="modal-body" style="display: flex; flex-direction: column; gap: 14px; padding: 16px 20px">
          <div class="form-group">
            <label class="form-label">Dataset Name</label>
            <input
              v-model="datasetForm.datasetId"
              type="text"
              class="form-input"
              placeholder="e.g. media, backups, data"
            />
          </div>
          <div class="form-group">
            <label class="form-label">Mountpoint</label>
            <input
              v-model="datasetForm.mountpoint"
              type="text"
              class="form-input"
              placeholder="/mnt/fast/..."
            />
          </div>
          <div class="form-group">
            <label class="form-label">Compression</label>
            <select v-model="datasetForm.compression" class="form-select">
              <option value="lz4">lz4</option>
              <option value="zstd">zstd</option>
              <option value="off">off</option>
            </select>
          </div>
        </div>
        <div class="modal-footer" style="padding: 12px 20px; display: flex; justify-content: flex-end; gap: 8px">
          <button class="btn btn-secondary btn-sm" @click="showCreateDatasetModal = false">Cancel</button>
          <button class="btn btn-primary btn-sm" :disabled="!datasetForm.datasetId.trim() || isBusy" @click="submitCreateDataset">
            Create Dataset
          </button>
        </div>
      </div>
    </div>

    <!-- Create Volume Modal -->
    <div v-if="showCreateVolumeModal" class="modal-overlay" @click.self="showCreateVolumeModal = false">
      <div class="modal-container" style="max-width: 520px">
        <div class="modal-header">
          <h3 class="modal-title">Create Volume (zvol) in {{ cleanName }}</h3>
          <button class="btn btn-xs btn-ghost" @click="showCreateVolumeModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <div class="modal-body" style="display: flex; flex-direction: column; gap: 14px; padding: 16px 20px">
          <div class="form-group">
            <label class="form-label">Volume Name</label>
            <input
              v-model="volumeForm.volumeId"
              type="text"
              class="form-input"
              placeholder="e.g. vm-101-disk-0"
            />
          </div>
          <div class="form-group">
            <label class="form-label">Volume Size (GiB)</label>
            <input
              v-model.number="volumeForm.sizeGib"
              type="number"
              min="1"
              class="form-input"
            />
          </div>
          <div class="form-group">
            <label class="form-label">Block Size</label>
            <select v-model.number="volumeForm.volblocksize" class="form-select">
              <option :value="8192">8 KiB</option>
              <option :value="16384">16 KiB (Recommended for VM Disks)</option>
              <option :value="32768">32 KiB</option>
              <option :value="65536">64 KiB</option>
            </select>
          </div>
          <div class="form-group" style="display: flex; align-items: center; gap: 8px">
            <input id="sparse-cb" v-model="volumeForm.sparse" type="checkbox" />
            <label for="sparse-cb" style="margin: 0; font-size: 13px; cursor: pointer">Sparse Volume (Thin Provisioning)</label>
          </div>
        </div>
        <div class="modal-footer" style="padding: 12px 20px; display: flex; justify-content: flex-end; gap: 8px">
          <button class="btn btn-secondary btn-sm" @click="showCreateVolumeModal = false">Cancel</button>
          <button class="btn btn-primary btn-sm" :disabled="!volumeForm.volumeId.trim() || isBusy" @click="submitCreateVolume">
            Create Volume
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pool-detail-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
}

.pool-header-card {
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.pool-header-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 14px;
}

.pool-title {
  margin: 0;
  font-size: 18px;
  font-weight: 700;
  color: var(--text-primary);
}

.pool-mountpoint {
  color: var(--text-muted);
  margin-top: 2px;
}

.pool-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.pool-subtabs {
  display: flex;
  align-items: center;
  gap: 4px;
  border-top: 1px solid var(--border-subtle);
  padding-top: 12px;
  margin-top: 4px;
  overflow-x: auto;
}

.subtab-item {
  background: transparent;
  border: 1px solid transparent;
  color: var(--text-secondary);
  font-size: 12.5px;
  font-weight: 500;
  padding: 5px 12px;
  border-radius: 4px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.subtab-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.subtab-item.active {
  background: var(--accent-bg-subtle);
  color: var(--accent-primary);
  border-color: var(--accent-primary);
  font-weight: 600;
}

.pool-tab-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.grid-2 {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 16px;
}

.detail-card {
  padding: 18px 20px;
}

.gauge-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 20px;
}

.capacity-stats {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.capacity-used {
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
}

.capacity-substats {
  display: flex;
  justify-content: space-between;
}

.progress-bar-wrap {
  width: 100%;
  height: 8px;
  background: var(--bg-elevated);
  border-radius: 4px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  border-radius: 4px;
  transition: width 0.3s ease;
}

.metrics-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
  padding-top: 14px;
  border-top: 1px solid var(--border-subtle);
}

.metric-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.metric-label {
  font-size: 11px;
  color: var(--text-muted);
}

.metric-val {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.scrub-status-box {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 14px;
}

.empty-state {
  padding: 32px;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
}

.form-input,
.form-select {
  background: var(--input-bg);
  border: 1px solid var(--border-subtle);
  color: var(--text-primary);
  border-radius: 4px;
  padding: 6px 10px;
  font-size: 13px;
  outline: none;
}

.form-input:focus,
.form-select:focus {
  border-color: var(--accent-primary);
}

.font-sm {
  font-size: 12px;
}

.font-xs {
  font-size: 10.5px;
}
</style>
