<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import {
  mdiAlertCircleOutline,
  mdiCheckCircleOutline,
  mdiClose,
  mdiCogOutline,
  mdiCrown,
  mdiDatabase,
  mdiDatabaseExportOutline,
  mdiDelete,
  mdiDragVariant,
  mdiFlash,
  mdiHarddisk,
  mdiLayersOutline,
  mdiMagnify,
  mdiPlus,
  mdiRefresh,
  mdiShieldCheckOutline,
  mdiStarOutline,
  mdiTreeOutline,
} from '@mdi/js';
import { services } from '../../container';
import {
  formatBytes,
  type Disk,
  type ZfsPool,
  type ZfsVdev,
} from '../../domain/models';
import MdiIcon from '../components/MdiIcon.vue';
import { confirmModal, nasState, notify, refreshAllData } from '../store';
import { navigateTo } from '../router';

onMounted(() => {
  if (nasState.disks.length === 0 || nasState.pools.length === 0) {
    refreshAllData();
  }
});

// ---------------------------------------------------------------------------
// Drag-and-Drop ZFS Pool Infrastructure Designer State
// ---------------------------------------------------------------------------
interface BuilderVdev {
  id: string;
  type: 'data' | 'cache' | 'log' | 'spare' | 'special';
  layout: 'stripe' | 'mirror' | 'raidz1' | 'raidz2' | 'raidz3';
  devicePaths: string[];
}

const showCreatePool = ref(false);
const creatingPool = ref(false);
const driveSearchQuery = ref('');
const draggedDevicePath = ref<string | null>(null);
const hoveredVdevId = ref<string | null>(null);
let vdevCounter = 1;

const poolForm = reactive({
  poolId: '',
  compression: 'lz4',
  mountpoint: '',
});

const builderVdevs = ref<BuilderVdev[]>([
  {
    id: 'vdev-data-0',
    type: 'data',
    layout: 'mirror',
    devicePaths: [],
  },
]);

function resetPoolBuilder() {
  poolForm.poolId = '';
  poolForm.compression = 'lz4';
  poolForm.mountpoint = '';
  driveSearchQuery.value = '';
  draggedDevicePath.value = null;
  hoveredVdevId.value = null;
  vdevCounter = 1;
  builderVdevs.value = [
    {
      id: 'vdev-data-0',
      type: 'data',
      layout: 'mirror',
      devicePaths: [],
    },
  ];
}

function openCreatePoolModal() {
  resetPoolBuilder();
  if (nasState.disks.length === 0) {
    refreshAllData();
  }
  showCreatePool.value = true;
}

// Drives not attached to an imported pool (excluding system OS disks and zvols)
const unassignedDisks = computed(() => {
  return nasState.disks.filter(
    (d) =>
      !d.owningZfsPool &&
      !d.isSystem &&
      !d.diskId.startsWith('zd') &&
      d.transport !== 'zvol' &&
      !d.partitions?.some((p) => p.isSystem || p.mountpoint === '/')
  );
});

const builderAssignedPaths = computed(() => {
  return new Set(builderVdevs.value.flatMap((v) => v.devicePaths));
});

const filteredUnassignedDisks = computed(() => {
  const q = driveSearchQuery.value.trim().toLowerCase();
  return unassignedDisks.value.filter((d) => {
    if (!q) return true;
    return (
      d.devicePath.toLowerCase().includes(q) ||
      d.diskId.toLowerCase().includes(q) ||
      (d.model || '').toLowerCase().includes(q) ||
      (d.serial || '').toLowerCase().includes(q) ||
      (d.type || '').toLowerCase().includes(q) ||
      (d.transport || '').toLowerCase().includes(q)
    );
  });
});

function getDiskByPath(path: string): Disk | undefined {
  return nasState.disks.find(
    (d) => d.devicePath === path || d.diskId === path.replace(/^\/dev\//, '')
  );
}

// Drag & Drop Handlers
function onDragStartDrive(ev: DragEvent, path: string) {
  draggedDevicePath.value = path;
  if (ev.dataTransfer) {
    ev.dataTransfer.setData('text/plain', path);
    ev.dataTransfer.effectAllowed = 'move';
  }
}

function onDragEndDrive() {
  draggedDevicePath.value = null;
  hoveredVdevId.value = null;
}

function onDragOverVdev(ev: DragEvent, vdevId: string) {
  ev.preventDefault();
  hoveredVdevId.value = vdevId;
  if (ev.dataTransfer) {
    ev.dataTransfer.dropEffect = 'move';
  }
}

function onDragLeaveVdev(vdevId: string) {
  if (hoveredVdevId.value === vdevId) {
    hoveredVdevId.value = null;
  }
}

function onDropDrive(vdevId: string) {
  const path = draggedDevicePath.value;
  if (!path) return;
  assignDriveToVdev(path, vdevId);
  draggedDevicePath.value = null;
  hoveredVdevId.value = null;
}

function assignDriveToVdev(path: string, vdevId: string) {
  // Remove from any existing VDEV first
  for (const v of builderVdevs.value) {
    const idx = v.devicePaths.indexOf(path);
    if (idx !== -1) {
      v.devicePaths.splice(idx, 1);
    }
  }
  const target = builderVdevs.value.find((v) => v.id === vdevId);
  if (target && !target.devicePaths.includes(path)) {
    target.devicePaths.push(path);
  }
}

function removeDriveFromVdev(vdevId: string, path: string) {
  const target = builderVdevs.value.find((v) => v.id === vdevId);
  if (target) {
    const idx = target.devicePaths.indexOf(path);
    if (idx !== -1) {
      target.devicePaths.splice(idx, 1);
    }
  }
}

function addBuilderVdev(type: 'data' | 'cache' | 'log' | 'spare' | 'special') {
  let layout: BuilderVdev['layout'] = 'stripe';
  if (type === 'data') layout = 'mirror';
  else if (type === 'log') layout = 'mirror';
  else if (type === 'special') layout = 'mirror';

  builderVdevs.value.push({
    id: `vdev-${type}-${vdevCounter++}`,
    type,
    layout,
    devicePaths: [],
  });
}

function removeBuilderVdev(vdevId: string) {
  const idx = builderVdevs.value.findIndex((v) => v.id === vdevId);
  if (idx !== -1) {
    builderVdevs.value.splice(idx, 1);
  }
}

// Live Capacity & Architecture Summary
const poolCapacitySummary = computed(() => {
  let totalRawBytes = 0;
  let estimatedUsableBytes = 0;
  let dataDrivesCount = 0;
  let cacheDrivesCount = 0;
  let logDrivesCount = 0;
  let spareDrivesCount = 0;

  for (const vdev of builderVdevs.value) {
    const driveSizes = vdev.devicePaths
      .map((p) => getDiskByPath(p)?.sizeBytes || 0)
      .filter((s) => s > 0);

    const vdevRaw = driveSizes.reduce((acc, s) => acc + s, 0);
    totalRawBytes += vdevRaw;

    if (vdev.type === 'data') {
      dataDrivesCount += vdev.devicePaths.length;
      if (driveSizes.length > 0) {
        const minSize = Math.min(...driveSizes);
        if (vdev.layout === 'stripe') {
          estimatedUsableBytes += vdevRaw;
        } else if (vdev.layout === 'mirror') {
          estimatedUsableBytes += minSize;
        } else if (vdev.layout === 'raidz1') {
          estimatedUsableBytes += Math.max(0, (driveSizes.length - 1) * minSize);
        } else if (vdev.layout === 'raidz2') {
          estimatedUsableBytes += Math.max(0, (driveSizes.length - 2) * minSize);
        } else if (vdev.layout === 'raidz3') {
          estimatedUsableBytes += Math.max(0, (driveSizes.length - 3) * minSize);
        }
      }
    } else if (vdev.type === 'cache') {
      cacheDrivesCount += vdev.devicePaths.length;
    } else if (vdev.type === 'log') {
      logDrivesCount += vdev.devicePaths.length;
    } else if (vdev.type === 'spare') {
      spareDrivesCount += vdev.devicePaths.length;
    }
  }

  return {
    totalRawBytes,
    estimatedUsableBytes,
    dataDrivesCount,
    cacheDrivesCount,
    logDrivesCount,
    spareDrivesCount,
  };
});

function getVdevValidation(vdev: BuilderVdev): { valid: boolean; message: string; state: 'ok' | 'warn' | 'info' } {
  const count = vdev.devicePaths.length;
  if (count === 0) {
    return { valid: false, message: 'No drives assigned yet. Drag unassigned drives here.', state: 'info' };
  }
  if (vdev.type === 'data') {
    if (vdev.layout === 'mirror') {
      if (count < 2) return { valid: false, message: 'Mirror requires at least 2 drives (currently 1).', state: 'warn' };
      return { valid: true, message: `✓ Valid ${count}-way Mirror. Tolerates ${count - 1} drive failure(s).`, state: 'ok' };
    }
    if (vdev.layout === 'raidz1') {
      if (count < 3) return { valid: false, message: `RAIDZ1 requires at least 3 drives (currently ${count}).`, state: 'warn' };
      return { valid: true, message: `✓ Valid RAIDZ1 (${count} drives, 1 parity disk).`, state: 'ok' };
    }
    if (vdev.layout === 'raidz2') {
      if (count < 4) return { valid: false, message: `RAIDZ2 requires at least 4 drives (currently ${count}).`, state: 'warn' };
      return { valid: true, message: `✓ Valid RAIDZ2 (${count} drives, 2 parity disks).`, state: 'ok' };
    }
    if (vdev.layout === 'raidz3') {
      if (count < 5) return { valid: false, message: `RAIDZ3 requires at least 5 drives (currently ${count}).`, state: 'warn' };
      return { valid: true, message: `✓ Valid RAIDZ3 (${count} drives, 3 parity disks).`, state: 'ok' };
    }
    return { valid: true, message: `✓ Stripe layout (${count} concatenated drive(s), no parity).`, state: 'ok' };
  }
  if (vdev.type === 'log') {
    if (vdev.layout === 'mirror') {
      if (count < 2) return { valid: false, message: 'Mirrored SLOG requires at least 2 drives.', state: 'warn' };
      return { valid: true, message: `✓ Mirrored SLOG write accelerator (${count} drives).`, state: 'ok' };
    }
    return { valid: true, message: `SLOG write accelerator (${count} drive(s)). Mirrored SLOG is recommended for write safety.`, state: 'ok' };
  }
  if (vdev.type === 'cache') {
    return { valid: true, message: `✓ L2ARC Read Cache (${count} drive(s)).`, state: 'ok' };
  }
  if (vdev.type === 'spare') {
    return { valid: true, message: `✓ Hot Spare (${count} drive(s)).`, state: 'ok' };
  }
  if (vdev.type === 'special') {
    if (vdev.layout === 'mirror' && count < 2) {
      return { valid: false, message: 'Special metadata VDEV requires at least 2 drives for redundancy.', state: 'warn' };
    }
    return { valid: true, message: `✓ Special Metadata & Small Files VDEV (${count} drive(s)).`, state: 'ok' };
  }
  return { valid: true, message: `✓ ${(vdev.type as string).toUpperCase()} VDEV (${count} drive(s)).`, state: 'ok' };
}

const isPoolBuilderValid = computed(() => {
  if (!poolForm.poolId.trim()) return false;
  const dataVdevs = builderVdevs.value.filter((v) => v.type === 'data');
  if (dataVdevs.length === 0) return false;
  for (const v of builderVdevs.value) {
    if (v.devicePaths.length > 0) {
      if (!getVdevValidation(v).valid) return false;
    }
  }
  return dataVdevs.some((v) => v.devicePaths.length > 0);
});

// ---------------------------------------------------------------------------
// Pool Operations (Create, Export, Scrub, Delete, Set Main)
// ---------------------------------------------------------------------------
async function handleCreatePool() {
  if (!poolForm.poolId.trim()) {
    notify('error', 'Pool Name is required');
    return;
  }
  const dataVdevs = builderVdevs.value.filter((v) => v.type === 'data');
  if (dataVdevs.length === 0 || dataVdevs.every((v) => v.devicePaths.length === 0)) {
    notify('error', 'At least one Data VDEV with assigned drives is required');
    return;
  }

  for (const v of builderVdevs.value) {
    if (v.devicePaths.length > 0) {
      const val = getVdevValidation(v);
      if (!val.valid) {
        notify('error', val.message);
        return;
      }
    }
  }

  const vdevs: ZfsVdev[] = builderVdevs.value
    .filter((bv) => bv.devicePaths.length > 0)
    .map((bv, idx) => ({
      name: `${bv.type}-${idx}`,
      type: bv.type,
      layout: bv.layout,
      state: 'ONLINE',
      children: bv.devicePaths.map((dp) => ({
        name: dp,
        path: dp,
        type: bv.type,
        layout: 'disk',
        state: 'ONLINE',
      })),
    }));

  const allDevices = builderVdevs.value.flatMap((bv) => bv.devicePaths);
  const primaryDataVdev = dataVdevs[0];

  creatingPool.value = true;
  try {
    const created = await services.zfs.createPool(poolForm.poolId.trim(), {
      vdevLayout: primaryDataVdev?.layout || 'mirror',
      devices: allDevices,
      compression: poolForm.compression || 'lz4',
      mountpoint: poolForm.mountpoint || `/nas/${poolForm.poolId.trim()}`,
      vdevs,
    });
    notify(
      'success',
      `Created ZFS pool "${created.poolName || created.name}" with ${vdevs.length} VDEV tier(s)`
    );
    showCreatePool.value = false;
    resetPoolBuilder();
    await refreshAllData();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create ZFS pool');
  } finally {
    creatingPool.value = false;
  }
}

async function handleExportPool(poolName: string) {
  const shortName = poolName.replace(/^zfsPools\//, '');
  const currentMain = nasState.systemConfig?.mainPool;
  const isMain = shortName === currentMain;

  const confirmed = await confirmModal({
    title: isMain ? 'Export Main Storage Pool' : 'Export ZFS Pool',
    tone: isMain ? 'danger' : 'warning',
    message: isMain
      ? `WARNING: "${shortName}" is currently designated as the MAIN POOL!`
      : `Export ZFS storage pool "${shortName}"?`,
    details: isMain
      ? 'Exporting the main pool will immediately disconnect persistent storage for Docker Apps and LXC containers. The pool will be safely unmounted and detached.'
      : 'The pool will be safely unmounted and detached from the system without deleting any data. It can be re-imported at any time.',
    confirmText: 'Export Pool',
  });
  if (!confirmed) return;

  try {
    await services.zfs.exportPool(poolName, false);
    notify('info', `Successfully exported ZFS pool "${shortName}"`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', err?.message || `Failed to export pool "${shortName}"`);
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
  const confirmed = await confirmModal({
    title: 'Destroy ZFS Storage Pool',
    tone: 'danger',
    message: `Are you sure you want to permanently destroy pool "${shortName}"?`,
    details: 'ALL DATA, datasets, snapshots, and block volumes in this pool will be PERMANENTLY DESTROYED. This action cannot be undone.',
    confirmText: 'Destroy Pool',
  });
  if (!confirmed) return;

  try {
    await services.zfs.deletePool(poolName, true);
    notify('info', `Destroyed ZFS pool "${shortName}"`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete pool');
  }
}

async function handleSetMainPool(poolName: string) {
  const cleanName = poolName.replace(/^zfsPools\//, '');
  const current = nasState.systemConfig?.mainPool;
  if (current === cleanName) return;

  const confirmed = await confirmModal({
    title: current ? 'Switch Main Storage Pool' : 'Set Main Storage Pool',
    tone: current ? 'warning' : 'primary',
    message: current
      ? `Change Main Storage Pool from "${current}" to "${cleanName}"?`
      : `Designate "${cleanName}" as the Main Storage Pool?`,
    details: current
      ? `Configuration, Docker appdata root, and LXC containers will now reside on "${cleanName}".`
      : `This will initialize .nas-system datasets and enable all paused services.`,
    confirmText: current ? 'Switch Main Pool' : 'Set Main Pool',
  });
  if (!confirmed) return;

  try {
    const updated = await services.system.selectMainPool(cleanName, true);
    nasState.systemConfig = updated;
    notify('success', `"${cleanName}" is now the Main Storage Pool!`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', err?.message || `Failed to set ${cleanName} as main pool`);
  }
}

// ---------------------------------------------------------------------------
// Existing Pool Configuration & VDEV Management Modal
// ---------------------------------------------------------------------------
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

function toggleDeviceInAddVdev(devicePath: string) {
  const current = addVdevForm.devicesText
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
  const idx = current.indexOf(devicePath);
  if (idx !== -1) {
    current.splice(idx, 1);
  } else {
    current.push(devicePath);
  }
  addVdevForm.devicesText = current.join(', ');
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
      notify('error', 'Please specify or select at least one device path');
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
    <div class="card" style="padding: 20px 24px">
      <div style="display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: 16px">
        <div>
          <div style="display: flex; align-items: center; gap: 10px">
            <MdiIcon :path="mdiDatabase" size="24" style="color: var(--accent-primary)" />
            <h2 class="card-title" style="margin: 0; font-size: 20px">ZFS Storage Pools</h2>
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
          <button class="btn btn-primary btn-sm" @click="openCreatePoolModal">
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
              <th style="min-width: 260px; text-align: right">Actions</th>
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
                    <div style="display: flex; align-items: center; gap: 6px">
                      <span class="mono" style="font-weight: 700; font-size: 14px; color: var(--text-primary)">
                        {{ p.poolName || p.name.replace(/^zfsPools\//, '') }}
                      </span>
                      <span
                        v-if="(p.poolName || p.name.replace(/^zfsPools\//, '')) === nasState.systemConfig?.mainPool"
                        class="badge badge-main-pool"
                        style="font-size: 10px; padding: 1px 7px"
                        title="Active Main Storage Pool"
                      >
                        <MdiIcon :path="mdiCrown" size="12" /> MAIN POOL
                      </span>
                    </div>
                    <div class="mono" style="font-size: 11px; color: var(--text-secondary)">
                      {{ p.mountpoint || `/nas/${p.poolName || p.name.replace(/^zfsPools\//, '')}` }}
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
                <div style="display: inline-flex; gap: 6px; justify-content: flex-end; align-items: center">
                  <button
                    v-if="(p.poolName || p.name.replace(/^zfsPools\//, '')) !== nasState.systemConfig?.mainPool"
                    class="btn btn-sm btn-primary-outline"
                    title="Designate as Main Storage Pool"
                    @click="handleSetMainPool(p.poolName || p.name.replace(/^zfsPools\//, ''))"
                  >
                    <MdiIcon :path="mdiCrown" size="14" style="margin-right: 3px; color: #fbbf24" />
                    Set Main
                  </button>
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
                    class="btn btn-sm"
                    title="Export pool (safely unmount and detach)"
                    @click="handleExportPool(p.name)"
                  >
                    <MdiIcon :path="mdiDatabaseExportOutline" size="14" style="margin-right: 3px" />
                    Export
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

    <!-- Advanced Drag-and-Drop ZFS Storage Pool Infrastructure Builder Modal -->
    <div v-if="showCreatePool" class="modal-overlay" @click.self="showCreatePool = false">
      <div class="modal-container builder-modal-container">
        <!-- Modal Header -->
        <div class="modal-header">
          <div>
            <div style="display: flex; align-items: center; gap: 10px">
              <MdiIcon :path="mdiLayersOutline" size="22" style="color: var(--accent-primary)" />
              <h3 class="card-title" style="margin: 0; font-size: 18px">
                ZFS Storage Pool Builder & Infrastructure Designer
              </h3>
            </div>
            <p class="card-subtitle" style="margin: 4px 0 0; font-size: 12.5px">
              Drag unassigned host drives directly into target VDEVs. Configure Data, Cache (L2ARC), Log (SLOG), and Spare tiers with enterprise redundancy.
            </p>
          </div>
          <button class="btn btn-sm btn-icon" @click="showCreatePool = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>

        <!-- Modal Body (Scrollable Workspace) -->
        <div class="modal-body" style="padding: 20px; display: flex; flex-direction: column; gap: 18px">
          <!-- Top Settings Grid -->
          <div class="form-grid" style="grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 14px">
            <div class="form-group">
              <label>Pool Name (ID)</label>
              <input v-model="poolForm.poolId" class="input mono" placeholder="tank" autofocus required />
            </div>

            <div class="form-group">
              <label>Mountpoint</label>
              <input
                v-model="poolForm.mountpoint"
                class="input mono"
                :placeholder="poolForm.poolId ? `/nas/${poolForm.poolId.trim()}` : '/nas/<pool-name>'"
              />
            </div>

            <div class="form-group">
              <label>Default Compression</label>
              <select v-model="poolForm.compression" class="select mono">
                <option value="lz4">lz4 (Fast, Recommended)</option>
                <option value="zstd">zstd (Higher ratio)</option>
                <option value="gzip">gzip (Legacy)</option>
                <option value="off">off</option>
              </select>
            </div>
          </div>

          <!-- Main Drag & Drop Workspace: Two Columns -->
          <div class="builder-workspace">
            <!-- Left Column: Available Drives Bank -->
            <div class="drives-bank-panel">
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px">
                <div style="font-weight: 700; font-size: 13px; display: flex; align-items: center; gap: 6px">
                  <MdiIcon :path="mdiHarddisk" size="16" style="color: var(--accent-primary)" />
                  <span>Available Drives</span>
                </div>
                <span class="badge badge-neutral" style="font-size: 11px">
                  {{ unassignedDisks.length }} unattached
                </span>
              </div>

              <!-- Drive Search Filter -->
              <div style="position: relative; margin-bottom: 12px">
                <MdiIcon
                  :path="mdiMagnify"
                  size="15"
                  style="position: absolute; left: 8px; top: 50%; transform: translateY(-50%); color: var(--text-muted)"
                />
                <input
                  v-model="driveSearchQuery"
                  class="input"
                  placeholder="Filter device, model, serial..."
                  style="padding-left: 28px; font-size: 12px; height: 32px"
                />
              </div>

              <!-- Drive Cards List -->
              <div class="drives-list">
                <div
                  v-if="filteredUnassignedDisks.length === 0"
                  style="text-align: center; padding: 28px 12px; color: var(--text-muted); font-size: 12px"
                >
                  <MdiIcon :path="mdiAlertCircleOutline" size="24" style="margin-bottom: 6px; opacity: 0.6" /><br />
                  No unassigned drives available. All host drives are currently attached to imported ZFS pools.
                </div>

                <div
                  v-for="d in filteredUnassignedDisks"
                  :key="d.devicePath"
                  class="drive-card"
                  :class="{ 'is-assigned': builderAssignedPaths.has(d.devicePath) }"
                  draggable="true"
                  @dragstart="onDragStartDrive($event, d.devicePath)"
                  @dragend="onDragEndDrive"
                >
                  <div style="display: flex; justify-content: space-between; align-items: flex-start">
                    <div style="display: flex; align-items: center; gap: 6px">
                      <MdiIcon :path="mdiDragVariant" size="16" style="color: var(--text-muted); cursor: grab" />
                      <span class="mono" style="font-weight: 700; font-size: 13px; color: var(--text-primary)">
                        {{ d.devicePath }}
                      </span>
                    </div>
                    <span class="badge badge-info mono" style="font-weight: 700; font-size: 11px">
                      {{ formatBytes(d.sizeBytes) }}
                    </span>
                  </div>

                  <div style="font-size: 11.5px; color: var(--text-secondary); margin-top: 4px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap" :title="d.model || d.vendor">
                    {{ d.model || d.vendor || 'Physical Disk' }}
                  </div>

                  <div v-if="d.serial" class="mono" style="font-size: 10px; color: var(--text-muted); margin-top: 2px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
                    S/N: {{ d.serial }}
                  </div>

                  <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 6px; font-size: 10.5px">
                    <div style="display: flex; gap: 4px">
                      <span class="badge badge-neutral" style="font-size: 9.5px">{{ d.type }}</span>
                      <span class="badge badge-neutral" style="font-size: 9.5px">{{ (d.transport || 'disk').toUpperCase() }}</span>
                      <span v-if="d.smartStatus" class="badge badge-success" style="font-size: 9.5px">{{ d.smartStatus }}</span>
                    </div>

                    <span v-if="builderAssignedPaths.has(d.devicePath)" class="badge badge-success" style="font-size: 9.5px">
                      In VDEV
                    </span>
                  </div>

                  <!-- Quick Assign Buttons for Direct Click Placement -->
                  <div
                    v-if="!builderAssignedPaths.has(d.devicePath)"
                    style="margin-top: 8px; border-top: 1px dashed var(--border-subtle); padding-top: 6px; display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 4px"
                  >
                    <button
                      v-for="(v, vIdx) in builderVdevs"
                      :key="v.id"
                      type="button"
                      class="btn btn-xs"
                      style="font-size: 9.5px; padding: 2px 6px"
                      :title="`Assign to ${v.type.toUpperCase()}`"
                      @click="assignDriveToVdev(d.devicePath, v.id)"
                    >
                      + {{ v.type.toUpperCase() }} {{ vIdx + 1 }}
                    </button>
                  </div>
                </div>
              </div>
            </div>

            <!-- Right Column: VDEV Infrastructure Canvas -->
            <div class="vdev-canvas-panel">
              <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px">
                <div style="font-weight: 700; font-size: 13.5px; color: var(--text-primary); display: flex; align-items: center; gap: 6px">
                  <MdiIcon :path="mdiTreeOutline" size="16" style="color: var(--accent-primary)" />
                  <span>Configured VDEV Topology</span>
                </div>

                <!-- Add Tier VDEV Buttons -->
                <div style="display: flex; gap: 6px; flex-wrap: wrap">
                  <button type="button" class="btn btn-xs btn-primary-outline" @click="addBuilderVdev('data')">
                    <MdiIcon :path="mdiPlus" size="12" /> Data VDEV
                  </button>
                  <button type="button" class="btn btn-xs" style="background: rgba(168, 85, 247, 0.15); color: #c084fc; border: 1px solid rgba(168, 85, 247, 0.3)" @click="addBuilderVdev('cache')">
                    <MdiIcon :path="mdiFlash" size="12" /> + Cache (L2ARC)
                  </button>
                  <button type="button" class="btn btn-xs" style="background: rgba(234, 179, 8, 0.15); color: #facc15; border: 1px solid rgba(234, 179, 8, 0.3)" @click="addBuilderVdev('log')">
                    <MdiIcon :path="mdiPlus" size="12" /> + Log (SLOG)
                  </button>
                  <button type="button" class="btn btn-xs btn-neutral" @click="addBuilderVdev('spare')">
                    <MdiIcon :path="mdiPlus" size="12" /> + Hot Spare
                  </button>
                  <button type="button" class="btn btn-xs" style="background: rgba(16, 185, 129, 0.15); color: #34d399; border: 1px solid rgba(16, 185, 129, 0.3)" @click="addBuilderVdev('special')">
                    <MdiIcon :path="mdiPlus" size="12" /> + Special
                  </button>
                </div>
              </div>

              <!-- List of VDEVs with Drop Zones -->
              <div style="display: flex; flex-direction: column; gap: 12px">
                <div
                  v-for="(vdev, vIdx) in builderVdevs"
                  :key="vdev.id"
                  class="vdev-builder-card"
                  :class="{
                    'drag-hover': hoveredVdevId === vdev.id,
                    'is-data': vdev.type === 'data',
                    'is-cache': vdev.type === 'cache',
                    'is-log': vdev.type === 'log',
                    'is-spare': vdev.type === 'spare',
                    'is-special': vdev.type === 'special',
                  }"
                  @dragover="onDragOverVdev($event, vdev.id)"
                  @dragleave="onDragLeaveVdev(vdev.id)"
                  @drop="onDropDrive(vdev.id)"
                >
                  <!-- VDEV Card Header -->
                  <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; border-bottom: 1px solid rgba(255,255,255,0.06); padding-bottom: 8px">
                    <div style="display: flex; align-items: center; gap: 8px">
                      <span
                        class="badge mono font-semibold"
                        :class="
                          vdev.type === 'data'
                            ? 'badge-info'
                            : vdev.type === 'cache'
                            ? 'badge-purple'
                            : vdev.type === 'log'
                            ? 'badge-warning'
                            : vdev.type === 'special'
                            ? 'badge-success'
                            : 'badge-neutral'
                        "
                        style="font-size: 11px"
                      >
                        {{ vdev.type.toUpperCase() }} TIER #{{ vIdx + 1 }}
                      </span>

                      <!-- Layout Selector -->
                      <div style="display: flex; align-items: center; gap: 6px">
                        <span style="font-size: 11px; color: var(--text-secondary)">Layout:</span>
                        <select
                          v-if="vdev.type === 'data'"
                          v-model="vdev.layout"
                          class="select mono select-xs"
                        >
                          <option value="mirror">Mirror (RAID-1)</option>
                          <option value="raidz1">RAIDZ1 (Single Parity)</option>
                          <option value="raidz2">RAIDZ2 (Dual Parity)</option>
                          <option value="raidz3">RAIDZ3 (Triple Parity)</option>
                          <option value="stripe">Stripe (No Parity)</option>
                        </select>
                        <select
                          v-else-if="vdev.type === 'log'"
                          v-model="vdev.layout"
                          class="select mono select-xs"
                        >
                          <option value="mirror">Mirror (Recommended)</option>
                          <option value="stripe">Stripe (Single SLOG)</option>
                        </select>
                        <select
                          v-else-if="vdev.type === 'special'"
                          v-model="vdev.layout"
                          class="select mono select-xs"
                        >
                          <option value="mirror">Mirror (Redundant)</option>
                          <option value="stripe">Stripe</option>
                        </select>
                        <span v-else class="mono" style="font-size: 11px; color: var(--text-muted)">
                          STRIPE
                        </span>
                      </div>
                    </div>

                    <div style="display: flex; align-items: center; gap: 8px">
                      <span class="mono" style="font-size: 11px; color: var(--text-secondary)">
                        {{ vdev.devicePaths.length }} drive{{ vdev.devicePaths.length === 1 ? '' : 's' }}
                      </span>
                      <button
                        v-if="builderVdevs.length > 1"
                        type="button"
                        class="btn btn-xs btn-icon btn-danger"
                        title="Remove this VDEV"
                        @click="removeBuilderVdev(vdev.id)"
                      >
                        <MdiIcon :path="mdiClose" size="14" />
                      </button>
                    </div>
                  </div>

                  <!-- Drop Zone -->
                  <div
                    class="vdev-drop-target"
                    :class="{ 'has-drives': vdev.devicePaths.length > 0 }"
                  >
                    <div
                      v-if="vdev.devicePaths.length === 0"
                      class="drop-placeholder"
                    >
                      <MdiIcon :path="mdiDragVariant" size="20" style="opacity: 0.5; margin-bottom: 4px" />
                      <span>Drag unassigned drives here, or click quick-assign on the left.</span>
                    </div>

                    <!-- Dropped Drives Chips -->
                    <div
                      v-else
                      style="display: flex; flex-wrap: wrap; gap: 8px; width: 100%"
                    >
                      <div
                        v-for="dp in vdev.devicePaths"
                        :key="dp"
                        class="vdev-drive-chip"
                      >
                        <div style="display: flex; align-items: center; gap: 6px">
                          <MdiIcon :path="mdiHarddisk" size="14" style="color: var(--accent-primary)" />
                          <span class="mono font-semibold" style="font-size: 12px">{{ dp }}</span>
                          <span class="badge badge-neutral mono" style="font-size: 10px">
                            {{ formatBytes(getDiskByPath(dp)?.sizeBytes || 0) }}
                          </span>
                        </div>
                        <button
                          type="button"
                          class="chip-remove-btn"
                          title="Remove drive from VDEV"
                          @click.stop="removeDriveFromVdev(vdev.id, dp)"
                        >
                          <MdiIcon :path="mdiClose" size="12" />
                        </button>
                      </div>
                    </div>
                  </div>

                  <!-- Validation Feedback Alert -->
                  <div
                    class="vdev-validation-bar"
                    :class="getVdevValidation(vdev).state"
                  >
                    <MdiIcon
                      :path="getVdevValidation(vdev).state === 'ok' ? mdiCheckCircleOutline : mdiAlertCircleOutline"
                      size="14"
                    />
                    <span>{{ getVdevValidation(vdev).message }}</span>
                  </div>
                </div>
              </div>

              <!-- Topology Capacity & Usable Storage Summary Banner -->
              <div class="capacity-summary-card">
                <div style="font-weight: 700; font-size: 12.5px; color: var(--text-primary); margin-bottom: 8px; display: flex; align-items: center; gap: 6px">
                  <MdiIcon :path="mdiLayersOutline" size="16" style="color: var(--accent-primary)" />
                  <span>Estimated Storage Architecture</span>
                </div>
                <div class="summary-metrics-grid">
                  <div>
                    <div style="font-size: 11px; color: var(--text-secondary)">Total Raw Capacity</div>
                    <div class="mono" style="font-weight: 700; font-size: 15px; color: var(--text-primary)">
                      {{ formatBytes(poolCapacitySummary.totalRawBytes) }}
                    </div>
                  </div>
                  <div>
                    <div style="font-size: 11px; color: var(--text-secondary)">Estimated Usable Space</div>
                    <div class="mono" style="font-weight: 700; font-size: 15px; color: var(--accent-primary)">
                      {{ formatBytes(poolCapacitySummary.estimatedUsableBytes) }}
                    </div>
                  </div>
                  <div>
                    <div style="font-size: 11px; color: var(--text-secondary)">Drive Tiers</div>
                    <div style="font-size: 12px; font-weight: 600; color: var(--text-primary)">
                      {{ poolCapacitySummary.dataDrivesCount }} Data · {{ poolCapacitySummary.cacheDrivesCount }} Cache · {{ poolCapacitySummary.logDrivesCount }} Log · {{ poolCapacitySummary.spareDrivesCount }} Spare
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Modal Footer -->
        <div class="modal-footer">
          <button type="button" class="btn btn-sm" @click="showCreatePool = false">
            Cancel
          </button>
          <button
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="!isPoolBuilderValid || creatingPool"
            @click="handleCreatePool"
          >
            <MdiIcon v-if="creatingPool" :path="mdiRefresh" size="14" class="spin" style="margin-right: 4px" />
            <MdiIcon v-else :path="mdiPlus" size="16" style="margin-right: 4px" />
            {{ creatingPool ? 'Creating Pool...' : 'Create Storage Pool' }}
          </button>
        </div>
      </div>
    </div>

    <!-- Configure Pool Modal (Settings & VDEVs) -->
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
                placeholder="/dev/sdb, /dev/sdc"
              />
              <!-- Unassigned drives quick-toggle chips -->
              <div v-if="unassignedDisks.length > 0" style="margin-top: 6px; display: flex; flex-wrap: wrap; gap: 6px">
                <span class="card-subtitle" style="font-size: 11px; width: 100%">Click unassigned drive to toggle:</span>
                <button
                  v-for="d in unassignedDisks"
                  :key="d.devicePath"
                  type="button"
                  class="badge mono"
                  :class="addVdevForm.devicesText.includes(d.devicePath) ? 'badge-info' : 'badge-neutral'"
                  style="cursor: pointer; padding: 2px 7px; font-size: 11px"
                  @click="toggleDeviceInAddVdev(d.devicePath)"
                >
                  + {{ d.devicePath }} ({{ formatBytes(d.sizeBytes) }})
                </button>
              </div>
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
  background: rgba(0, 0, 0, 0.78);
  backdrop-filter: blur(5px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 16px;
}

.modal-container {
  background: var(--bg-surface);
  border: 1px solid var(--border-strong);
  border-radius: 12px;
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.6);
  display: flex;
  flex-direction: column;
  max-height: 88vh;
  overflow: hidden;
}

.builder-modal-container {
  max-width: 1180px;
  width: 96vw;
  max-height: 92vh;
}

.modal-header {
  padding: 16px 22px;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  border-bottom: 1px solid var(--border-subtle);
  background: var(--bg-base);
}

.modal-footer {
  padding: 14px 22px;
  border-top: 1px solid var(--border-subtle);
  background: var(--bg-base);
  display: flex;
  justify-content: flex-end;
  gap: 10px;
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

/* Builder Layout Grid */
.builder-workspace {
  display: grid;
  grid-template-columns: 360px 1fr;
  gap: 18px;
}

@media (max-width: 920px) {
  .builder-workspace {
    grid-template-columns: 1fr;
  }
}

.drives-bank-panel {
  background: rgba(0, 0, 0, 0.22);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  max-height: 560px;
}

.drives-list {
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-right: 4px;
}

.drive-card {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 10px 12px;
  cursor: grab;
  user-select: none;
  transition: all 0.15s ease;
}

.drive-card:hover {
  border-color: var(--accent-primary);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.25);
}

.drive-card.is-assigned {
  opacity: 0.55;
  border-style: dashed;
}

.vdev-canvas-panel {
  display: flex;
  flex-direction: column;
  gap: 14px;
  overflow-y: auto;
  max-height: 560px;
  padding-right: 4px;
}

.vdev-builder-card {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 14px;
  transition: all 0.2s ease;
}

.vdev-builder-card.is-data {
  border-left: 3px solid #38bdf8;
}

.vdev-builder-card.is-cache {
  border-left: 3px solid #c084fc;
}

.vdev-builder-card.is-log {
  border-left: 3px solid #facc15;
}

.vdev-builder-card.is-spare {
  border-left: 3px solid #94a3b8;
}

.vdev-builder-card.is-special {
  border-left: 3px solid #34d399;
}

.vdev-builder-card.drag-hover {
  border-color: var(--accent-primary);
  background: rgba(56, 189, 248, 0.08);
  box-shadow: 0 0 16px rgba(56, 189, 248, 0.25);
}

.vdev-drop-target {
  min-height: 58px;
  border: 2px dashed rgba(255, 255, 255, 0.12);
  border-radius: 6px;
  padding: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.15);
  transition: border-color 0.15s ease;
}

.vdev-drop-target.has-drives {
  border-style: solid;
  border-color: rgba(255, 255, 255, 0.08);
}

.drop-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  color: var(--text-muted);
  font-size: 11.5px;
  text-align: center;
  pointer-events: none;
}

.vdev-drive-chip {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  padding: 6px 10px;
  display: inline-flex;
  align-items: center;
  gap: 10px;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.2);
}

.chip-remove-btn {
  background: transparent;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  padding: 0;
  display: flex;
  align-items: center;
  transition: color 0.15s ease;
}

.chip-remove-btn:hover {
  color: var(--danger);
}

.vdev-validation-bar {
  margin-top: 8px;
  padding: 6px 10px;
  border-radius: 4px;
  font-size: 11px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.vdev-validation-bar.ok {
  background: rgba(16, 185, 129, 0.12);
  color: #34d399;
}

.vdev-validation-bar.warn {
  background: rgba(234, 179, 8, 0.12);
  color: #facc15;
}

.vdev-validation-bar.info {
  background: rgba(148, 163, 184, 0.08);
  color: var(--text-muted);
}

.capacity-summary-card {
  background: rgba(0, 0, 0, 0.25);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 12px 16px;
}

.summary-metrics-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 14px;
}

.select-xs {
  font-size: 11px;
  height: 26px;
  padding: 2px 20px 2px 8px;
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

.badge-main-pool {
  background: rgba(251, 191, 36, 0.15);
  color: #fbbf24;
  border: 1px solid rgba(251, 191, 36, 0.35);
  font-weight: 700;
  letter-spacing: 0.03em;
}

.spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
</style>
