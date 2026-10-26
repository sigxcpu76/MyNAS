<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import {
  mdiClose,
  mdiDatabase,
  mdiDelete,
  mdiHarddisk,
  mdiMagnify,
  mdiPencil,
  mdiPlus,
  mdiRefresh,
} from '@mdi/js';
import { services } from '../../container';
import {
  bytesToGib,
  formatBytes,
  gibToBytes,
  type ZfsVolume,
} from '../../domain/models';
import MdiIcon from '../components/MdiIcon.vue';
import { confirmModal, nasState, notify, refreshAllData, refreshZfsSubresources } from '../store';

const selectedPool = ref<string>('all');
const searchQuery = ref('');

// Modals
const showCreateVolume = ref(false);
const editingVolume = ref<ZfsVolume | null>(null);

// Forms
const volumeForm = reactive({
  poolName: '',
  volumeId: '',
  volsizeGib: 20,
  thinProvisioned: true,
  volblocksize: '16k',
  compression: 'lz4',
  sync: 'standard',
  readonly: false,
  dedup: false,
});

const editVolumeForm = reactive({
  volsizeGib: 20,
  thinProvisioned: true,
  compression: 'lz4',
  sync: 'standard',
  readonly: false,
  dedup: false,
});

// All volumes combined or filtered by pool
const allVolumes = computed<Array<ZfsVolume & { poolName: string }>>(() => {
  const result: Array<ZfsVolume & { poolName: string }> = [];
  for (const pool of nasState.pools) {
    const pName = pool.name;
    const shortName = pool.poolName || pName.replace(/^zfsPools\//, '');
    if (selectedPool.value !== 'all' && selectedPool.value !== pName) {
      continue;
    }
    const vols = nasState.volumesByPool[pName] || [];
    for (const v of vols) {
      result.push({ ...v, poolName: shortName });
    }
  }
  return result;
});

const filteredVolumes = computed(() => {
  const q = searchQuery.value.trim().toLowerCase();
  if (!q) return allVolumes.value;
  return allVolumes.value.filter((v) => {
    const fn = v.volumePath || v.name;
    return fn.toLowerCase().includes(q) || v.devicePath.toLowerCase().includes(q) || v.poolName.toLowerCase().includes(q);
  });
});

function openCreateModal() {
  const defaultPool = nasState.pools[0]?.name || '';
  volumeForm.poolName = selectedPool.value !== 'all' ? selectedPool.value : defaultPool;
  volumeForm.volumeId = '';
  volumeForm.volsizeGib = 20;
  volumeForm.thinProvisioned = true;
  volumeForm.volblocksize = '16k';
  volumeForm.compression = 'lz4';
  volumeForm.sync = 'standard';
  volumeForm.readonly = false;
  volumeForm.dedup = false;
  showCreateVolume.value = true;
}

async function handleCreateVolume() {
  try {
    let pName = volumeForm.poolName;
    if (!pName.startsWith('zfsPools/')) {
      pName = `zfsPools/${pName}`;
    }
    const created = await services.zfs.createVolume(pName, volumeForm.volumeId.trim(), {
      sizeBytes: gibToBytes(volumeForm.volsizeGib),
      thinProvisioned: volumeForm.thinProvisioned,
      volblocksize: volumeForm.volblocksize,
      compression: volumeForm.compression,
      sync: volumeForm.sync,
      readonly: volumeForm.readonly,
      dedup: volumeForm.dedup,
    });
    notify('success', `Created ZFS Volume ${created.volumePath || created.name}`);
    showCreateVolume.value = false;
    await refreshZfsSubresources(pName);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create volume');
  }
}

function startEditVolume(vol: ZfsVolume) {
  editingVolume.value = vol;
  editVolumeForm.volsizeGib = bytesToGib(vol.sizeBytes || 0);
  editVolumeForm.thinProvisioned = Boolean(vol.thinProvisioned);
  editVolumeForm.compression = vol.compression || 'lz4';
  editVolumeForm.sync = vol.sync || 'standard';
  editVolumeForm.readonly = Boolean(vol.readonly);
  editVolumeForm.dedup = Boolean(vol.dedup);
}

async function handleSaveEditVolume() {
  if (!editingVolume.value) return;
  try {
    const targetSize = gibToBytes(editVolumeForm.volsizeGib);
    const updateMask: string[] = ['sizeBytes', 'thinProvisioned', 'compression', 'sync', 'readonly', 'dedup'];
    const updated = await services.zfs.updateVolume(
      editingVolume.value.name,
      {
        sizeBytes: targetSize,
        thinProvisioned: editVolumeForm.thinProvisioned,
        compression: editVolumeForm.compression,
        sync: editVolumeForm.sync,
        readonly: editVolumeForm.readonly,
        dedup: editVolumeForm.dedup,
      },
      updateMask
    );
    notify('success', `Updated volume ${updated.volumePath || updated.name}`);
    editingVolume.value = null;
    const poolPrefix = updated.name.split('/volumes/')[0];
    await refreshZfsSubresources(poolPrefix);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update volume');
  }
}

async function handleDeleteVolume(name: string) {
  const shortName = name.replace(/^zfsPools\/[^/]+\/volumes\//, '');
  const confirmed = await confirmModal({
    title: 'Delete ZVol Block Device',
    tone: 'danger',
    message: `Are you sure you want to delete volume "${shortName}"?`,
    details: 'ALL DATA ON THIS BLOCK DEVICE WILL BE PERMANENTLY DESTROYED.',
    confirmText: 'Delete Volume',
  });
  if (!confirmed) return;

  try {
    await services.zfs.deleteVolume(name);
    notify('info', `Deleted volume ${shortName}`);
    const poolPrefix = name.split('/volumes/')[0];
    await refreshZfsSubresources(poolPrefix);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete volume');
  }
}

onMounted(async () => {
  for (const pool of nasState.pools) {
    await refreshZfsSubresources(pool.name);
  }
});
</script>

<template>
  <div style="display: flex; flex-direction: column; gap: 20px; width: 100%; max-width: 100%; min-width: 0">
    <!-- Header Card with "New Volume" Button -->
    <div class="card" style="padding: 18px 20px">
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 14px">
        <div>
          <div style="display: flex; align-items: center; gap: 10px">
            <MdiIcon :path="mdiHarddisk" size="24" style="color: var(--purple-text)" />
            <h2 class="card-title" style="font-size: 19px; margin: 0">ZFS Block Volumes (ZVols)</h2>
            <span class="badge badge-purple" style="font-size: 12px">
              {{ allVolumes.length }} {{ allVolumes.length === 1 ? 'Volume' : 'Volumes' }}
            </span>
          </div>
          <p class="card-subtitle" style="margin-top: 4px; font-size: 13px">
            Emulated raw block devices exported to <code class="mono">/dev/zvol/...</code> for KVM virtual disks, databases, and raw block storage.
          </p>
        </div>

        <div style="display: flex; align-items: center; gap: 10px; flex-wrap: wrap">
          <!-- Pool Filter Selector -->
          <div style="display: flex; align-items: center; gap: 6px">
            <label style="font-size: 12px; color: var(--text-secondary)">Pool:</label>
            <select v-model="selectedPool" class="select" style="height: 32px; font-size: 12px; padding: 2px 8px">
              <option value="all">All Pools ({{ nasState.pools.length }})</option>
              <option v-for="p in nasState.pools" :key="p.name" :value="p.name">
                {{ p.poolName || p.name.replace(/^zfsPools\//, '') }}
              </option>
            </select>
          </div>

          <!-- Search Input -->
          <div class="search-input-wrap">
            <MdiIcon :path="mdiMagnify" size="16" class="search-icon" />
            <input
              v-model="searchQuery"
              class="input search-input"
              placeholder="Search volumes..."
            />
          </div>

          <button class="btn btn-sm" :disabled="nasState.loading" @click="refreshAllData">
            <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" />
            Refresh
          </button>

          <!-- Prominent "New Volume" Button -->
          <button class="btn btn-primary btn-sm" @click="openCreateModal()">
            <MdiIcon :path="mdiPlus" size="16" style="margin-right: 4px" />
            New Volume
          </button>
        </div>
      </div>
    </div>

    <!-- Volumes Table Card -->
    <div class="card" style="padding: 18px 20px">
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th style="min-width: 180px">Volume Name</th>
              <th style="min-width: 100px">Pool</th>
              <th style="min-width: 200px">Device Path</th>
              <th style="min-width: 140px">Capacity & Usage</th>
              <th style="min-width: 100px">Block Size</th>
              <th style="min-width: 100px">Provisioning</th>
              <th style="min-width: 180px">Properties</th>
              <th style="min-width: 120px; text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="filteredVolumes.length === 0">
              <td colspan="8" style="text-align: center; color: var(--text-secondary); padding: 36px">
                <div v-if="searchQuery">
                  No volumes match filter "<strong>{{ searchQuery }}</strong>".
                </div>
                <div v-else>
                  No ZFS volumes found. Click <strong>"New Volume"</strong> above to create one.
                </div>
              </td>
            </tr>

            <tr v-for="v in filteredVolumes" :key="v.name">
              <!-- Volume Name -->
              <td>
                <div style="display: flex; align-items: center; gap: 8px">
                  <MdiIcon :path="mdiHarddisk" size="18" style="color: var(--purple-text); flex-shrink: 0" />
                  <div>
                    <div class="mono font-semibold" style="font-size: 13.5px; color: var(--text-primary)">
                      {{ v.volumePath || v.name.replace(/^zfsPools\/[^/]+\/volumes\//, '') }}
                    </div>
                  </div>
                </div>
              </td>

              <!-- Pool -->
              <td>
                <span class="badge mono badge-neutral" style="font-size: 11px">
                  {{ v.poolName }}
                </span>
              </td>

              <!-- Device Path -->
              <td>
                <span class="mono" style="font-size: 12px; color: var(--purple-text)" :title="v.devicePath">
                  {{ v.devicePath || '-' }}
                </span>
              </td>

              <!-- Capacity & Used -->
              <td>
                <div>
                  <div class="mono" style="font-weight: 700; font-size: 13px">
                    {{ formatBytes(v.sizeBytes) }}
                  </div>
                  <div style="font-size: 11px; color: var(--text-secondary); margin-top: 1px">
                    Used: {{ formatBytes(v.usedBytes) }}
                  </div>
                </div>
              </td>

              <!-- Block Size -->
              <td>
                <span class="badge mono badge-neutral" style="font-size: 10.5px">
                  {{ v.volblocksize || '16k' }}
                </span>
              </td>

              <!-- Provisioning -->
              <td>
                <span
                  class="badge mono"
                  :class="v.thinProvisioned ? 'badge-success' : 'badge-neutral'"
                  style="font-size: 10.5px"
                >
                  {{ v.thinProvisioned ? 'Thin (Sparse)' : 'Thick (Reserved)' }}
                </span>
              </td>

              <!-- Properties -->
              <td>
                <div style="display: flex; gap: 4px; align-items: center; flex-wrap: wrap">
                  <span class="badge badge-info mono" style="font-size: 10px">{{ v.compression || 'lz4' }}</span>
                  <span
                    v-if="v.sync && v.sync !== 'standard'"
                    class="badge mono"
                    :class="v.sync === 'always' ? 'badge-success' : 'badge-danger'"
                    style="font-size: 10px"
                  >
                    sync:{{ v.sync }}
                  </span>
                  <span v-else class="badge badge-neutral mono" style="font-size: 10px; opacity: 0.75">
                    sync:std
                  </span>
                  <span v-if="v.readonly" class="badge badge-warning mono" style="font-size: 10px">RO</span>
                  <span v-if="v.dedup" class="badge badge-purple mono" style="font-size: 10px">Dedup</span>
                </div>
              </td>

              <!-- Actions -->
              <td style="text-align: right">
                <div style="display: inline-flex; gap: 6px; justify-content: flex-end">
                  <button
                    class="btn btn-sm"
                    title="Edit Settings & Resize Volsize"
                    @click="startEditVolume(v)"
                  >
                    <MdiIcon :path="mdiPencil" size="13" style="margin-right: 4px" />
                    Resize
                  </button>
                  <button
                    class="btn btn-danger btn-sm btn-icon"
                    title="Delete Volume"
                    @click="handleDeleteVolume(v.name)"
                  >
                    <MdiIcon :path="mdiDelete" size="13" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Create Volume Modal -->
    <div v-if="showCreateVolume" class="modal-overlay" @click.self="showCreateVolume = false">
      <div class="modal-container" style="max-width: 580px; width: 100%">
        <div class="modal-header">
          <div style="display: flex; align-items: center; gap: 10px">
            <MdiIcon :path="mdiHarddisk" size="20" style="color: var(--purple-text)" />
            <h3 class="card-title" style="margin: 0">Create New ZFS Volume (ZVol)</h3>
          </div>
          <button class="btn btn-sm btn-icon" @click="showCreateVolume = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>

        <form @submit.prevent="handleCreateVolume">
          <div class="modal-body" style="display: flex; flex-direction: column; gap: 14px">
            <div class="form-group">
              <label>Target Pool</label>
              <select v-model="volumeForm.poolName" class="select" required>
                <option v-for="p in nasState.pools" :key="p.name" :value="p.name">
                  {{ p.poolName || p.name.replace(/^zfsPools\//, '') }}
                </option>
              </select>
            </div>

            <div class="form-group">
              <label>Volume Identifier</label>
              <input v-model="volumeForm.volumeId" class="input mono" placeholder="vm-disk-01" required />
            </div>

            <div class="form-grid">
              <div class="form-group">
                <label>Volsize (GiB)</label>
                <input v-model.number="volumeForm.volsizeGib" type="number" min="1" class="input mono" required />
              </div>

              <div class="form-group">
                <label>Block Size (volblocksize)</label>
                <select v-model="volumeForm.volblocksize" class="select mono">
                  <option value="4k">4k</option>
                  <option value="8k">8k</option>
                  <option value="16k">16k (Recommended for VMs)</option>
                  <option value="64k">64k</option>
                  <option value="128k">128k</option>
                </select>
              </div>
            </div>

            <div class="form-grid">
              <div class="form-group">
                <label>Compression</label>
                <select v-model="volumeForm.compression" class="select">
                  <option value="lz4">lz4 (Fast, recommended)</option>
                  <option value="zstd">zstd</option>
                  <option value="off">off</option>
                </select>
              </div>

              <div class="form-group">
                <label>Sync Mode</label>
                <select v-model="volumeForm.sync" class="select">
                  <option value="standard">standard (Default)</option>
                  <option value="always">always (Highest data safety)</option>
                  <option value="disabled">disabled (Benchmark / Non-critical)</option>
                </select>
              </div>
            </div>

            <div class="form-grid" style="align-items: center">
              <label style="display: flex; align-items: center; gap: 8px; cursor: pointer">
                <input v-model="volumeForm.thinProvisioned" type="checkbox" />
                <span>Thin Provisioning (Sparse allocation)</span>
              </label>

              <label style="display: flex; align-items: center; gap: 8px; cursor: pointer">
                <input v-model="volumeForm.dedup" type="checkbox" />
                <span>Enable Deduplication</span>
              </label>
            </div>
          </div>

          <div style="padding: 14px 20px; border-top: 1px solid var(--border-subtle); display: flex; justify-content: flex-end; gap: 8px">
            <button type="button" class="btn btn-sm" @click="showCreateVolume = false">Cancel</button>
            <button type="submit" class="btn btn-primary btn-sm">Create Volume</button>
          </div>
        </form>
      </div>
    </div>

    <!-- Edit Volume Modal -->
    <div v-if="editingVolume" class="modal-overlay" @click.self="editingVolume = null">
      <div class="modal-container" style="max-width: 540px; width: 100%">
        <div class="modal-header">
          <div style="display: flex; align-items: center; gap: 10px">
            <MdiIcon :path="mdiPencil" size="18" style="color: var(--purple-text)" />
            <h3 class="card-title" style="margin: 0">
              Edit Volume: <span class="mono">{{ editingVolume.volumePath || editingVolume.name }}</span>
            </h3>
          </div>
          <button class="btn btn-sm btn-icon" @click="editingVolume = null">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>

        <form @submit.prevent="handleSaveEditVolume">
          <div class="modal-body" style="display: flex; flex-direction: column; gap: 14px">
            <div class="form-group">
              <label>Provisioned Size (GiB)</label>
              <input v-model.number="editVolumeForm.volsizeGib" type="number" min="1" class="input mono" required />
              <span class="card-subtitle" style="font-size: 11px">
                Current: {{ formatBytes(editingVolume.sizeBytes) }} (Resizing expands or shrinks the raw block device).
              </span>
            </div>

            <div class="form-grid">
              <div class="form-group">
                <label>Compression</label>
                <select v-model="editVolumeForm.compression" class="select">
                  <option value="lz4">lz4</option>
                  <option value="zstd">zstd</option>
                  <option value="off">off</option>
                </select>
              </div>

              <div class="form-group">
                <label>Sync Mode</label>
                <select v-model="editVolumeForm.sync" class="select">
                  <option value="standard">standard</option>
                  <option value="always">always</option>
                  <option value="disabled">disabled</option>
                </select>
              </div>
            </div>

            <div class="form-grid">
              <label style="display: flex; align-items: center; gap: 8px; cursor: pointer">
                <input v-model="editVolumeForm.thinProvisioned" type="checkbox" />
                <span>Thin Provisioning</span>
              </label>

              <label style="display: flex; align-items: center; gap: 8px; cursor: pointer">
                <input v-model="editVolumeForm.readonly" type="checkbox" />
                <span>Read-Only</span>
              </label>
            </div>
          </div>

          <div style="padding: 14px 20px; border-top: 1px solid var(--border-subtle); display: flex; justify-content: flex-end; gap: 8px">
            <button type="button" class="btn btn-sm" @click="editingVolume = null">Cancel</button>
            <button type="submit" class="btn btn-primary btn-sm">Save Changes</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<style scoped>
.search-input-wrap {
  position: relative;
  display: flex;
  align-items: center;
}

.search-icon {
  position: absolute;
  left: 9px;
  color: var(--text-secondary);
  pointer-events: none;
}

.search-input {
  padding-left: 30px;
  height: 32px;
  font-size: 12px;
  width: 170px;
}

.badge-purple {
  background: var(--accent-bg-subtle);
  color: var(--purple-text);
  border: 1px solid var(--border-subtle);
}

.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: var(--modal-backdrop);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal-container {
  background: var(--bg-surface);
  border: 1px solid var(--border-strong);
  border-radius: 12px;
  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
  max-height: 85vh;
  overflow: hidden;
}

.modal-header {
  padding: 16px 20px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid var(--border-subtle);
}

.modal-body {
  padding: 20px;
  overflow-y: auto;
}
</style>
