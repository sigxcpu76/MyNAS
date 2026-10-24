<script setup lang="ts">
import { reactive, ref } from 'vue';
import {
  mdiClose,
  mdiCogOutline,
  mdiDatabase,
  mdiDelete,
  mdiFlash,
  mdiHarddisk,
  mdiPlus,
  mdiRefresh,
  mdiShieldCheckOutline,
} from '@mdi/js';
import { services } from '../../container';
import {
  formatBytes,
  type ZfsPool,
} from '../../domain/models';
import MdiIcon from '../components/MdiIcon.vue';
import { nasState, notify, refreshAllData, refreshZfsSubresources } from '../store';
import { navigateTo } from '../router';

const showCreatePool = ref(false);
const poolForm = reactive({
  poolId: '',
  vdevLayout: 'raidz1',
  devicesText: '/dev/sdb, /dev/sdc, /dev/sdd',
  compression: 'lz4',
  mountpoint: '',
});

// Pool Operations
async function handleCreatePool() {
  try {
    const devices = poolForm.devicesText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    const created = await services.zfs.createPool(poolForm.poolId, {
      vdevLayout: poolForm.vdevLayout,
      devices,
      compression: poolForm.compression,
      mountpoint: poolForm.mountpoint || `/${poolForm.poolId.trim()}`,
    });
    notify('success', `Created ZFS pool ${created.poolName || created.name}`);
    poolForm.poolId = '';
    showCreatePool.value = false;
    await refreshAllData();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create ZFS pool');
  }
}

async function handleScrubPool(poolName: string) {
  try {
    const updated = await services.zfs.scrubPool(poolName);
    const idx = nasState.pools.findIndex((p) => p.name === poolName);
    if (idx !== -1) nasState.pools[idx] = updated;
    notify('success', `Initiated ZFS scrub on ${updated.poolName || updated.name}`);
  } catch (err: any) {
    notify('error', err?.message || 'Scrub failed');
  }
}

async function handleDeletePool(poolName: string) {
  const shortName = poolName.replace(/^zfsPools\//, '');
  if (!confirm(`Are you sure you want to destroy pool "${shortName}"? ALL DATA WILL BE LOST.`)) {
    return;
  }
  try {
    await services.zfs.deletePool(poolName, true);
    notify('info', `Deleted ZFS pool ${shortName}`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete pool');
  }
}

// Pool Configuration & VDEV Management Modal
const configuringPool = ref<ZfsPool | null>(null);
const configTab = ref<'settings' | 'vdevs'>('settings');

const poolSettingsForm = reactive({
  sync: 'standard',
  autotrim: 'off',
  compression: 'lz4',
});

const addVdevForm = reactive({
  type: 'cache',
  layout: 'stripe',
  devicesText: '',
});

function openPoolConfig(pool: ZfsPool) {
  configuringPool.value = pool;
  configTab.value = 'settings';
  poolSettingsForm.sync = (pool.sync as any) || 'standard';
  poolSettingsForm.autotrim = (pool.autotrim as any) || 'off';
  poolSettingsForm.compression = pool.compression || 'lz4';
  addVdevForm.type = 'cache';
  addVdevForm.layout = 'stripe';
  addVdevForm.devicesText = '';
}

async function handleSavePoolSettings() {
  if (!configuringPool.value) return;
  try {
    const updated = await services.zfs.updatePool(configuringPool.value.name, {
      sync: poolSettingsForm.sync,
      autotrim: poolSettingsForm.autotrim,
      compression: poolSettingsForm.compression,
    });
    const idx = nasState.pools.findIndex((p) => p.name === configuringPool.value?.name);
    if (idx !== -1) nasState.pools[idx] = updated;
    notify('success', `Updated settings for pool ${updated.poolName || updated.name}`);
    configuringPool.value = null;
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update pool settings');
  }
}

async function handleAddVdev() {
  if (!configuringPool.value) return;
  try {
    const devices = addVdevForm.devicesText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    if (devices.length === 0) {
      notify('error', 'Please specify at least one device path');
      return;
    }
    const updated = await services.zfs.addVdev(configuringPool.value.name, {
      type: addVdevForm.type,
      layout: addVdevForm.layout,
      devices,
    });
    const idx = nasState.pools.findIndex((p) => p.name === configuringPool.value?.name);
    if (idx !== -1) nasState.pools[idx] = updated;
    configuringPool.value = updated;
    addVdevForm.devicesText = '';
    notify('success', `Added ${addVdevForm.type} VDEV to pool`);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to add VDEV');
  }
}

function viewPoolDatasets(poolName: string) {
  nasState.selectedPoolName = poolName;
  navigateTo('storage-datasets');
}
</script>

<template>
  <div style="display: flex; flex-direction: column; gap: 20px; width: 100%; max-width: 100%; min-width: 0">
    <!-- Header Card with "New Pool" Button -->
    <div class="card" style="padding: 18px 20px">
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 14px">
        <div>
          <div style="display: flex; align-items: center; gap: 10px">
            <MdiIcon :path="mdiDatabase" size="24" style="color: var(--accent-primary)" />
            <h2 class="card-title" style="font-size: 19px; margin: 0">ZFS Storage Pools</h2>
            <span class="badge badge-info" style="font-size: 12px">
              {{ nasState.pools.length }} {{ nasState.pools.length === 1 ? 'Pool' : 'Pools' }}
            </span>
          </div>
          <p class="card-subtitle" style="margin-top: 4px; font-size: 13px">
            Storage pool topologies, RAID redundancy, capacity allocations, scrub tasks, and tier VDEV management.
          </p>
        </div>

        <div style="display: flex; align-items: center; gap: 10px">
          <button class="btn btn-sm" :disabled="nasState.loading" @click="refreshAllData">
            <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" />
            Refresh
          </button>
          <button class="btn btn-primary btn-sm" @click="showCreatePool = true">
            <MdiIcon :path="mdiPlus" size="16" style="margin-right: 4px" />
            New Pool
          </button>
        </div>
      </div>
    </div>

    <!-- Pools Table Card -->
    <div class="card" style="padding: 18px 20px">
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th style="min-width: 140px">Pool Name</th>
              <th style="min-width: 90px">Health</th>
              <th style="min-width: 130px">Layout</th>
              <th style="min-width: 220px">Capacity & Usage</th>
              <th style="min-width: 90px">Frag</th>
              <th style="min-width: 110px">Sync / Dedup</th>
              <th style="min-width: 170px">Scrub Status</th>
              <th style="min-width: 200px; text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="nasState.pools.length === 0">
              <td colspan="8" style="text-align: center; color: var(--text-secondary); padding: 36px">
                No storage pools found on the host. Click <strong>"New Pool"</strong> above to create one.
              </td>
            </tr>

            <tr v-for="p in nasState.pools" :key="p.name">
              <!-- Pool Name -->
              <td>
                <div style="display: flex; align-items: center; gap: 8px">
                  <MdiIcon :path="mdiDatabase" size="18" style="color: var(--accent-primary)" />
                  <div>
                    <div class="mono" style="font-weight: 700; font-size: 14px; color: var(--text-primary)">
                      {{ p.poolName || p.name.replace(/^zfsPools\//, '') }}
                    </div>
                    <div class="mono" style="font-size: 11px; color: var(--text-secondary)">
                      {{ p.mountpoint || `/${p.poolName || p.name.replace(/^zfsPools\//, '')}` }}
                    </div>
                  </div>
                </div>
              </td>

              <!-- Health -->
              <td>
                <span
                  class="badge mono"
                  :class="p.state === 'ONLINE' ? 'badge-success' : 'badge-danger'"
                  style="font-size: 11px"
                >
                  {{ p.state }}
                </span>
              </td>

              <!-- Layout -->
              <td>
                <div style="display: flex; flex-direction: column; gap: 3px">
                  <span class="badge badge-info mono" style="font-size: 10.5px">
                    {{ (p.vdevLayout || 'stripe').toUpperCase() }}
                  </span>
                  <div
                    v-if="p.devices && p.devices.length > 0"
                    class="mono"
                    style="font-size: 10.5px; color: var(--text-secondary); max-width: 140px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap"
                    :title="p.devices.join(', ')"
                  >
                    {{ p.devices.length }} dev{{ p.devices.length === 1 ? '' : 's' }}: {{ p.devices.join(', ') }}
                  </div>
                </div>
              </td>

              <!-- Capacity & Usage -->
              <td>
                <div style="display: flex; flex-direction: column; gap: 5px">
                  <div style="display: flex; justify-content: space-between; font-size: 11.5px">
                    <span class="mono" style="font-weight: 600">
                      {{ formatBytes(p.allocatedBytes) }} / {{ formatBytes(p.sizeBytes) }}
                    </span>
                    <span class="mono" style="color: var(--accent-primary); font-weight: 700">
                      {{ p.capacityPercent }}%
                    </span>
                  </div>
                  <!-- Progress Bar -->
                  <div style="width: 100%; height: 6px; background: rgba(0, 0, 0, 0.3); border-radius: 3px; overflow: hidden; border: 1px solid var(--border-subtle)">
                    <div
                      :style="{
                        width: `${Math.min(100, p.capacityPercent)}%`,
                        background: p.capacityPercent > 85 ? 'var(--danger)' : p.capacityPercent > 70 ? 'var(--warning)' : 'var(--accent-primary)'
                      }"
                      style="height: 100%; border-radius: 2px; transition: width 0.3s ease"
                    ></div>
                  </div>
                  <div style="font-size: 10.5px; color: var(--text-secondary)">
                    Free: {{ formatBytes(p.freeBytes) }}
                  </div>
                </div>
              </td>

              <!-- Fragmentation -->
              <td>
                <span class="mono" style="font-weight: 600; font-size: 12px; color: #c084fc">
                  {{ p.fragmentationPercent }}%
                </span>
              </td>

              <!-- Sync / Dedup -->
              <td>
                <div style="display: flex; flex-direction: column; gap: 3px; align-items: flex-start">
                  <span
                    class="badge mono"
                    :class="p.sync === 'always' ? 'badge-success' : p.sync === 'disabled' ? 'badge-danger' : 'badge-neutral'"
                    style="font-size: 10px"
                  >
                    sync:{{ p.sync || 'std' }}
                  </span>
                  <span class="mono" style="font-size: 10.5px; color: var(--text-secondary)">
                    dedup: {{ p.dedupRatio > 1 ? `${p.dedupRatio.toFixed(2)}x` : '1.00x' }}
                  </span>
                </div>
              </td>

              <!-- Scrub Status -->
              <td>
                <div class="mono" style="font-size: 11px; color: var(--text-secondary); max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap" :title="p.lastScrubStatus || 'Never scrubbed'">
                  {{ p.lastScrubStatus || 'Never scrubbed' }}
                </div>
              </td>

              <!-- Actions -->
              <td style="text-align: right">
                <div style="display: inline-flex; gap: 6px; justify-content: flex-end">
                  <button
                    class="btn btn-sm"
                    title="View Child Datasets in this pool"
                    @click="viewPoolDatasets(p.name)"
                  >
                    Datasets
                  </button>
                  <button
                    class="btn btn-sm"
                    title="Scrub pool integrity"
                    @click="handleScrubPool(p.name)"
                  >
                    <MdiIcon :path="mdiShieldCheckOutline" size="14" style="margin-right: 3px" />
                    Scrub
                  </button>
                  <button
                    class="btn btn-sm"
                    title="Configure Sync & VDEVs"
                    @click="openPoolConfig(p)"
                  >
                    <MdiIcon :path="mdiCogOutline" size="14" />
                  </button>
                  <button
                    class="btn btn-danger btn-sm btn-icon"
                    title="Destroy Pool"
                    @click="handleDeletePool(p.name)"
                  >
                    <MdiIcon :path="mdiDelete" size="14" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Create Pool Modal -->
    <div v-if="showCreatePool" class="modal-overlay" @click.self="showCreatePool = false">
      <div class="modal-container" style="max-width: 580px; width: 100%">
        <div class="modal-header">
          <div style="display: flex; align-items: center; gap: 10px">
            <MdiIcon :path="mdiDatabase" size="20" style="color: var(--accent-primary)" />
            <h3 class="card-title" style="margin: 0">Create New Storage Pool</h3>
          </div>
          <button class="btn btn-sm btn-icon" @click="showCreatePool = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>

        <form @submit.prevent="handleCreatePool">
          <div class="modal-body" style="display: flex; flex-direction: column; gap: 14px">
            <div class="form-group">
              <label>Pool Name</label>
              <input v-model="poolForm.poolId" class="input mono" placeholder="tank" required />
            </div>

            <div class="form-grid">
              <div class="form-group">
                <label>Redundancy Layout</label>
                <select v-model="poolForm.vdevLayout" class="select">
                  <option value="mirror">Mirror (RAID-1)</option>
                  <option value="raidz1">RAIDZ1 (Single Parity)</option>
                  <option value="raidz2">RAIDZ2 (Dual Parity)</option>
                  <option value="stripe">Stripe (No Parity / RAID-0)</option>
                </select>
              </div>

              <div class="form-group">
                <label>Default Compression</label>
                <select v-model="poolForm.compression" class="select">
                  <option value="lz4">lz4 (Recommended)</option>
                  <option value="zstd">zstd</option>
                  <option value="gzip">gzip</option>
                  <option value="off">off</option>
                </select>
              </div>
            </div>

            <div class="form-group">
              <label>Mountpoint (Optional)</label>
              <input v-model="poolForm.mountpoint" class="input mono" placeholder="/tank" />
            </div>

            <div class="form-group">
              <label>Backing Block Devices (comma-separated)</label>
              <input
                v-model="poolForm.devicesText"
                class="input mono"
                placeholder="/dev/sdb, /dev/sdc"
                required
              />
              <span class="card-subtitle" style="font-size: 11px">
                Use whole drive paths (e.g. /dev/sdb) or partition paths.
              </span>
            </div>
          </div>

          <div style="padding: 14px 20px; border-top: 1px solid var(--border-subtle); display: flex; justify-content: flex-end; gap: 8px">
            <button type="button" class="btn btn-sm" @click="showCreatePool = false">Cancel</button>
            <button type="submit" class="btn btn-primary btn-sm">Create Storage Pool</button>
          </div>
        </form>
      </div>
    </div>

    <!-- Configure Pool Modal -->
    <div v-if="configuringPool" class="modal-overlay" @click.self="configuringPool = null">
      <div class="modal-container" style="max-width: 620px; width: 100%">
        <div class="modal-header">
          <div style="display: flex; align-items: center; gap: 10px">
            <MdiIcon :path="mdiCogOutline" size="20" style="color: var(--accent-primary)" />
            <h3 class="card-title" style="margin: 0">
              Configure Pool: <span class="mono">{{ configuringPool.poolName || configuringPool.name.replace(/^zfsPools\//, '') }}</span>
            </h3>
          </div>
          <button class="btn btn-sm btn-icon" @click="configuringPool = null">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>

        <div class="modal-tabs">
          <button
            class="tab-btn"
            :class="{ active: configTab === 'settings' }"
            @click="configTab = 'settings'"
          >
            Settings & Sync
          </button>
          <button
            class="tab-btn"
            :class="{ active: configTab === 'vdevs' }"
            @click="configTab = 'vdevs'"
          >
            Tier VDEVs (Cache / SLOG)
          </button>
        </div>

        <!-- Tab 1: Settings -->
        <div v-if="configTab === 'settings'" class="modal-body" style="display: flex; flex-direction: column; gap: 16px">
          <div class="config-section">
            <label class="config-label">
              <span>Sync Policy</span>
              <span class="badge badge-info mono">{{ poolSettingsForm.sync }}</span>
            </label>
            <select v-model="poolSettingsForm.sync" class="select">
              <option value="standard">standard (Honor POSIX fsync requests)</option>
              <option value="always">always (Force all synchronous writes via SLOG/ZIL)</option>
              <option value="disabled">disabled (Ignore sync requests - Highest throughput, risk on power loss)</option>
            </select>
          </div>

          <div class="config-section">
            <label class="config-label">
              <span>Autotrim</span>
              <span class="badge badge-neutral mono">{{ poolSettingsForm.autotrim }}</span>
            </label>
            <select v-model="poolSettingsForm.autotrim" class="select">
              <option value="on">on (Automatic background TRIM on SSDs)</option>
              <option value="off">off (Disabled)</option>
            </select>
          </div>

          <div class="config-section">
            <label class="config-label">
              <span>Pool Compression</span>
            </label>
            <select v-model="poolSettingsForm.compression" class="select">
              <option value="lz4">lz4 (Fast, recommended)</option>
              <option value="zstd">zstd (Higher ratio)</option>
              <option value="gzip">gzip</option>
              <option value="off">off</option>
            </select>
          </div>

          <div style="display: flex; justify-content: flex-end; gap: 8px; margin-top: 10px">
            <button type="button" class="btn btn-sm" @click="configuringPool = null">Cancel</button>
            <button type="button" class="btn btn-primary btn-sm" @click="handleSavePoolSettings">
              Save Settings
            </button>
          </div>
        </div>

        <!-- Tab 2: VDEVs -->
        <div v-else class="modal-body" style="display: flex; flex-direction: column; gap: 16px">
          <div>
            <div style="font-weight: 600; font-size: 13px; margin-bottom: 8px">Existing VDEVs</div>
            <div v-if="configuringPool.vdevs && configuringPool.vdevs.length > 0" class="vdev-tier-list">
              <div v-for="vdev in configuringPool.vdevs" :key="vdev.name" class="vdev-card">
                <div class="vdev-header">
                  <span class="mono font-semibold">{{ vdev.name }}</span>
                  <span class="badge mono tier-badge" :class="vdev.type === 'data' ? 'badge-info' : 'badge-purple'">
                    {{ vdev.type.toUpperCase() }}
                  </span>
                </div>
              </div>
            </div>
            <div v-else class="card-subtitle" style="font-size: 12px">
              Primary data vdevs active.
            </div>
          </div>

          <!-- Add VDEV Form -->
          <div class="add-vdev-box">
            <div style="font-weight: 600; font-size: 13px; margin-bottom: 10px">Attach Special Tier VDEV</div>
            <div class="form-grid">
              <div class="form-group">
                <label>Tier Type</label>
                <select v-model="addVdevForm.type" class="select">
                  <option value="cache">Cache (L2ARC Read SSD)</option>
                  <option value="log">Log / SLOG (ZIL Write SSD)</option>
                  <option value="special">Special (Metadata & Small Blocks)</option>
                  <option value="spare">Hot Spare</option>
                </select>
              </div>
              <div class="form-group">
                <label>Device Layout</label>
                <select v-model="addVdevForm.layout" class="select">
                  <option value="stripe">Stripe (Single / Multi disk)</option>
                  <option value="mirror">Mirror (Redundant)</option>
                </select>
              </div>
            </div>

            <div class="form-group" style="margin-top: 10px">
              <label>Device Paths</label>
              <input
                v-model="addVdevForm.devicesText"
                class="input mono"
                placeholder="/dev/nvme0n1p3"
              />
            </div>

            <div style="display: flex; justify-content: flex-end; margin-top: 12px">
              <button class="btn btn-primary btn-sm" @click="handleAddVdev">
                Attach VDEV
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.75);
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

.modal-tabs {
  display: flex;
  border-bottom: 1px solid var(--border-subtle);
  background: var(--bg-base);
  padding: 0 16px;
}

.tab-btn {
  display: inline-flex;
  align-items: center;
  padding: 12px 16px;
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}

.tab-btn.active {
  color: var(--accent-primary);
  border-bottom-color: var(--accent-primary);
  background: rgba(56, 189, 248, 0.05);
}

.modal-body {
  padding: 20px;
  overflow-y: auto;
}

.config-section {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.config-label {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 13px;
  font-weight: 600;
}

.vdev-tier-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.vdev-card {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 10px 14px;
}

.vdev-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.add-vdev-box {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 14px;
}

.badge-purple {
  background: rgba(168, 85, 247, 0.15);
  color: #c084fc;
  border: 1px solid rgba(168, 85, 247, 0.3);
}
</style>
