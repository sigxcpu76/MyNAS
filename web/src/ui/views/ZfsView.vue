<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import {
  mdiAlertCircleOutline,
  mdiCamera,
  mdiCheck,
  mdiChevronDown,
  mdiChevronRight,
  mdiClose,
  mdiCrown,
  mdiDatabase,
  mdiDatabaseExportOutline,
  mdiDelete,
  mdiDragVariant,
  mdiFlash,
  mdiFormatListBulleted,
  mdiHarddisk,
  mdiHelpCircleOutline,
  mdiInformationOutline,
  mdiLayersOutline,
  mdiMagnify,
  mdiPencil,
  mdiPlus,
  mdiRefresh,
  mdiServer,
  mdiShieldCheckOutline,
  mdiTreeOutline,
  mdiTune,
} from '@mdi/js';
import { services } from '../../container';
import {
  bytesToGib,
  formatBytes,
  gibToBytes,
  type Disk,
  type ZfsDataset,
  type ZfsPool,
  type ZfsVdev,
  type ZfsVolume,
  isDotDataset,
} from '../../domain/models';
import MdiIcon from '../components/MdiIcon.vue';
import SortableTh from '../components/SortableTh.vue';
import PathSelector from '../components/PathSelector.vue';
import { sortHierarchy, useTableSort } from '../composables/useTableSort';
import ZfsPoolGauge from '../components/ZfsPoolGauge.vue';
import { askModal, confirmDelete } from '../modal';
import { confirmModal, nasState, notify, refreshAllData, refreshZfsSubresources } from '../store';

// ---------------------------------------------------------------------------
// 1. Top-Level State & Selection
// ---------------------------------------------------------------------------

export interface BuilderVdev {
  id: string;
  type: 'data' | 'cache' | 'log' | 'special' | 'dedup' | 'spare';
  layout: 'stripe' | 'mirror' | 'raidz1' | 'raidz2' | 'raidz3';
  devicePaths: string[];
}

const showCreatePool = ref(false);
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

function openCreatePool() {
  resetPoolBuilder();
  showCreatePool.value = true;
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

function getAssignedVdevName(devicePath: string): string | null {
  for (const vdev of builderVdevs.value) {
    if (vdev.devicePaths.includes(devicePath)) {
      return `${vdev.type.toUpperCase()} (${vdev.layout})`;
    }
  }
  return null;
}

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
  return { valid: true, message: `✓ ${vdev.type.toUpperCase()} VDEV (${count} drive(s)).`, state: 'ok' };
}

const isPoolBuilderValid = computed(() => {
  if (!poolForm.poolId.trim()) return false;
  const dataVdevs = builderVdevs.value.filter((v) => v.type === 'data');
  if (dataVdevs.length === 0 || dataVdevs.every((v) => v.devicePaths.length === 0)) return false;
  for (const v of builderVdevs.value) {
    if (v.devicePaths.length > 0) {
      const val = getVdevValidation(v);
      if (!val.valid) return false;
    }
  }
  return true;
});

function onDragStartDrive(e: DragEvent, devicePath: string) {
  draggedDevicePath.value = devicePath;
  if (e.dataTransfer) {
    e.dataTransfer.setData('text/plain', devicePath);
    e.dataTransfer.effectAllowed = 'move';
  }
}

function onDragEndDrive() {
  draggedDevicePath.value = null;
  hoveredVdevId.value = null;
}

function onDragOverVdev(e: DragEvent, vdevId: string) {
  e.preventDefault();
  if (e.dataTransfer) {
    e.dataTransfer.dropEffect = 'move';
  }
  hoveredVdevId.value = vdevId;
}

function onDragLeaveVdev(vdevId: string) {
  if (hoveredVdevId.value === vdevId) {
    hoveredVdevId.value = null;
  }
}

function onDropDriveToVdev(e: DragEvent, targetVdevId: string) {
  e.preventDefault();
  hoveredVdevId.value = null;
  const path = e.dataTransfer?.getData('text/plain') || draggedDevicePath.value;
  if (!path) return;
  assignDriveToVdev(path, targetVdevId);
  draggedDevicePath.value = null;
}

function assignDriveToVdev(devicePath: string, targetVdevId: string) {
  for (const vdev of builderVdevs.value) {
    vdev.devicePaths = vdev.devicePaths.filter((p) => p !== devicePath);
  }
  const target = builderVdevs.value.find((v) => v.id === targetVdevId);
  if (target && !target.devicePaths.includes(devicePath)) {
    target.devicePaths.push(devicePath);
  }
}

function removeDriveFromVdev(devicePath: string, vdevId: string) {
  const target = builderVdevs.value.find((v) => v.id === vdevId);
  if (target) {
    target.devicePaths = target.devicePaths.filter((p) => p !== devicePath);
  }
}

function addBuilderVdev(type: 'data' | 'cache' | 'log' | 'special' | 'dedup' | 'spare') {
  vdevCounter++;
  let defaultLayout: 'stripe' | 'mirror' | 'raidz1' | 'raidz2' = 'stripe';
  if (type === 'data') defaultLayout = 'mirror';
  if (type === 'log') defaultLayout = 'mirror';
  if (type === 'special') defaultLayout = 'mirror';

  builderVdevs.value.push({
    id: `vdev-${type}-${vdevCounter}`,
    type,
    layout: defaultLayout,
    devicePaths: [],
  });
}

function removeBuilderVdev(vdevId: string) {
  if (builderVdevs.value.length <= 1) {
    notify('error', 'A storage pool must have at least one VDEV');
    return;
  }
  builderVdevs.value = builderVdevs.value.filter((v) => v.id !== vdevId);
}

const activePool = computed(() =>
  nasState.pools.find((p) => p.name === nasState.selectedPoolName) || nasState.pools[0] || null
);

const activeDatasets = computed(() => {
  const list = activePool.value ? nasState.datasetsByPool[activePool.value.name] || [] : [];
  return list.filter((d) => !isDotDataset(d));
});

const activeVolumes = computed(() =>
  activePool.value ? nasState.volumesByPool[activePool.value.name] || [] : []
);

async function selectPool(poolName: string) {
  nasState.selectedPoolName = poolName;
  await refreshZfsSubresources(poolName);
}

// ---------------------------------------------------------------------------
// 2. Pool Operations (Create, Export, Scrub, Delete)
// ---------------------------------------------------------------------------

async function handleCreatePool() {
  if (!poolForm.poolId.trim()) {
    notify('error', 'Pool ID is required');
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

  try {
    const cleanId = poolForm.poolId.replace(/^zfsPools\//, '').trim();
    const created = await services.zfs.createPool(cleanId, {
      vdevLayout: primaryDataVdev?.layout || 'mirror',
      devices: allDevices,
      compression: poolForm.compression || 'lz4',
      mountpoint: poolForm.mountpoint || `/mnt/${cleanId}`,
      vdevs,
    });
    notify(
      'success',
      `Created ZFS pool "${created.poolName || created.name.replace(/^zfsPools\//, '')}" with ${vdevs.length} VDEV tier(s)`
    );
    showCreatePool.value = false;
    resetPoolBuilder();
    await refreshAllData();
    nasState.selectedPoolName = created.name;
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create ZFS pool');
  }
}

async function handleExportPool(poolName: string) {
  const cleanId = poolName.replace(/^zfsPools\//, '');
  const isMain = cleanId === nasState.systemConfig?.mainPool;
  const confirmed = await confirmModal({
    title: isMain ? 'Export Main Storage Pool' : 'Export ZFS Pool',
    tone: isMain ? 'danger' : 'warning',
    message: isMain
      ? `WARNING: "${cleanId}" is currently designated as the MAIN POOL!`
      : `Are you sure you want to export ZFS pool "${cleanId}"?`,
    details: isMain
      ? 'Exporting the main pool will immediately detach storage for all Docker Apps and LXC containers. The pool will be safely unmounted and detached.'
      : 'The pool will be safely unmounted and detached from the system without deleting data.',
    confirmText: 'Export Pool',
  });
  if (!confirmed) return;

  try {
    await services.zfs.exportPool(poolName, false);
    notify('success', `Exported ZFS pool "${cleanId}" successfully`);
    if (isMain) {
      await services.system.selectMainPool('', false);
    }
    await refreshAllData();
  } catch (err: any) {
    notify('error', err?.message || `Failed to export pool "${cleanId}"`);
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
  const res = await confirmDelete({
    kind: 'Pool',
    name: shortName,
    details: 'All data, datasets and volumes in this pool will be destroyed.',
    confirmText: 'Destroy Pool',
  });
  if (!res.confirmed) return;

  try {
    await services.zfs.deletePool(poolName, true);
    notify('info', `Deleted ZFS pool ${shortName}`);
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
      ? `Configuration, Docker appdata, and LXC root will now reside on "${cleanName}".`
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
// 3. Pool Configuration & VDEV Management Modal
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

function closePoolConfig() {
  configuringPool.value = null;
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
    configuringPool.value = updated;
    notify('success', `Updated settings for pool "${updated.poolName || updated.name}"`);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update pool settings');
  }
}

async function handleAddVdev() {
  if (!configuringPool.value) return;
  const devices = addVdevForm.devicesText
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
  if (devices.length === 0) {
    notify('info', 'Please specify at least one device path');
    return;
  }
  try {
    const updated = await services.zfs.addVdev(configuringPool.value.name, {
      type: addVdevForm.type,
      layout: addVdevForm.layout,
      devices,
    });
    const idx = nasState.pools.findIndex((p) => p.name === configuringPool.value?.name);
    if (idx !== -1) nasState.pools[idx] = updated;
    configuringPool.value = updated;
    addVdevForm.devicesText = '';
    notify('success', `Added ${addVdevForm.type.toUpperCase()} storage tier to pool`);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to add VDEV to pool');
  }
}

async function handleRemoveVdev(devicePath: string) {
  if (!configuringPool.value) return;
  const res = await askModal({
    title: 'Remove VDEV Device',
    tone: 'danger',
    message: `Are you sure you want to remove device "${devicePath}" from pool?`,
    details: 'Removing a device or cache/log vdev requires ZFS device evacuation. Ensure sufficient redundancy.',
    confirmText: 'Remove Device',
    confirmPhrase: devicePath,
  });
  if (!res.confirmed) return;

  try {
    const updated = await services.zfs.removeVdev(configuringPool.value.name, devicePath);
    const idx = nasState.pools.findIndex((p) => p.name === configuringPool.value?.name);
    if (idx !== -1) nasState.pools[idx] = updated;
    configuringPool.value = updated;
    notify('info', `Removed device "${devicePath}" from pool`);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to remove device');
  }
}

// ---------------------------------------------------------------------------
// 4. Hierarchical Dataset & ZVol Tree / List View
// ---------------------------------------------------------------------------

export interface ZfsHierarchyNode {
  id: string;
  name: string;
  fullPath: string;
  type: 'dataset' | 'volume';
  mountpoint?: string;
  devicePath?: string;
  usedBytes: number;
  availableBytes?: number;
  quotaBytes?: number;
  reservationBytes?: number;
  sizeBytes?: number;
  compression: string;
  recordsize?: string;
  volblocksize?: string;
  readonly?: boolean;
  thinProvisioned?: boolean;
  sync?: string;
  dedup?: boolean;
  snapshotsCount?: number;
  lastSnapshot?: string;
  rawDataset?: ZfsDataset;
  rawVolume?: ZfsVolume;
  children: ZfsHierarchyNode[];
  depth: number;
}

const viewMode = ref<'tree' | 'list'>('tree');
const searchQuery = ref('');
const collapsedPaths = ref<Set<string>>(new Set());

function toggleCollapse(nodeId: string) {
  if (collapsedPaths.value.has(nodeId)) {
    collapsedPaths.value.delete(nodeId);
  } else {
    collapsedPaths.value.add(nodeId);
  }
}

function expandAll() {
  collapsedPaths.value.clear();
}

function collapseAll() {
  for (const node of allHierarchyNodes.value) {
    if (node.children.length > 0) {
      collapsedPaths.value.add(node.id);
    }
  }
}

// Build the hierarchical tree from datasets and volumes
const poolHierarchyTree = computed<ZfsHierarchyNode[]>(() => {
  if (!activePool.value) return [];
  const poolShort = activePool.value.poolName || activePool.value.name.replace(/^zfsPools\//, '');
  const nodeMap = new Map<string, ZfsHierarchyNode>();

  // 1. Add all datasets
  for (const ds of activeDatasets.value) {
    const path = ds.datasetPath;
    const parts = path.split('/');
    const leafName = parts[parts.length - 1] || path;
    nodeMap.set(path, {
      id: `ds:${path}`,
      name: leafName,
      fullPath: path,
      type: 'dataset',
      mountpoint: ds.mountpoint,
      usedBytes: ds.usedBytes,
      availableBytes: ds.availableBytes,
      quotaBytes: ds.quotaBytes,
      reservationBytes: ds.reservationBytes,
      compression: ds.compression,
      recordsize: ds.recordsize,
      readonly: ds.readonly,
      sync: ds.sync || 'standard',
      dedup: ds.dedup,
      snapshotsCount: ds.snapshotsCount,
      lastSnapshot: ds.lastSnapshot,
      rawDataset: ds,
      children: [],
      depth: 0,
    });
  }

  // 2. Add all volumes
  for (const vol of activeVolumes.value) {
    const path = vol.volumePath;
    const parts = path.split('/');
    const leafName = parts[parts.length - 1] || path;
    nodeMap.set(path, {
      id: `vol:${path}`,
      name: leafName,
      fullPath: path,
      type: 'volume',
      devicePath: vol.devicePath,
      usedBytes: vol.usedBytes,
      sizeBytes: vol.sizeBytes,
      compression: vol.compression,
      volblocksize: vol.volblocksize,
      thinProvisioned: vol.thinProvisioned,
      sync: vol.sync || 'standard',
      readonly: vol.readonly,
      dedup: vol.dedup,
      rawVolume: vol,
      children: [],
      depth: 0,
    });
  }

  // Ensure the pool root dataset exists in nodeMap
  if (!nodeMap.has(poolShort) && activePool.value) {
    nodeMap.set(poolShort, {
      id: `ds:${poolShort}`,
      name: poolShort,
      fullPath: poolShort,
      type: 'dataset',
      mountpoint: activePool.value.mountpoint || `/${poolShort}`,
      usedBytes: activePool.value.allocatedBytes,
      availableBytes: activePool.value.freeBytes,
      compression: activePool.value.compression || 'lz4',
      recordsize: '128k',
      readonly: false,
      sync: 'standard',
      dedup: false,
      snapshotsCount: 0,
      children: [],
      depth: 0,
    });
  }

  // 3. Assemble parent-child relationships
  const rootNodes: ZfsHierarchyNode[] = [];

  for (const [path, node] of nodeMap.entries()) {
    if (path === poolShort) {
      rootNodes.push(node);
      continue;
    }

    const lastSlash = path.lastIndexOf('/');
    if (lastSlash === -1) {
      if (nodeMap.has(poolShort)) {
        nodeMap.get(poolShort)!.children.push(node);
      } else {
        rootNodes.push(node);
      }
      continue;
    }

    const parentPath = path.substring(0, lastSlash);
    if (nodeMap.has(parentPath)) {
      nodeMap.get(parentPath)!.children.push(node);
    } else if (nodeMap.has(poolShort)) {
      nodeMap.get(poolShort)!.children.push(node);
    } else {
      // Orphan
      rootNodes.push(node);
    }
  }

  // 4. Sort and assign depths
  function processDepths(nodes: ZfsHierarchyNode[], depth: number) {
    nodes.sort((a, b) => {
      // datasets first, then volumes, then alphabetical
      if (a.type !== b.type) return a.type === 'dataset' ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
    for (const n of nodes) {
      n.depth = depth;
      if (n.children.length > 0) {
        processDepths(n.children, depth + 1);
      }
    }
  }

  processDepths(rootNodes, 0);
  return rootNodes;
});

// All nodes flattened for search / collapse operations
const allHierarchyNodes = computed<ZfsHierarchyNode[]>(() => {
  const list: ZfsHierarchyNode[] = [];
  function collect(nodes: ZfsHierarchyNode[]) {
    for (const n of nodes) {
      list.push(n);
      if (n.children.length > 0) collect(n.children);
    }
  }
  collect(poolHierarchyTree.value);
  return list;
});

// Visible nodes after applying viewMode (tree vs list) and search query
const visibleHierarchyNodes = computed<ZfsHierarchyNode[]>(() => {
  const query = searchQuery.value.trim().toLowerCase();

  // Search filter mode (always flat or auto-expanded)
  if (query) {
    return allHierarchyNodes.value.filter(
      (n) =>
        n.name.toLowerCase().includes(query) ||
        n.fullPath.toLowerCase().includes(query) ||
        (n.mountpoint && n.mountpoint.toLowerCase().includes(query)) ||
        (n.devicePath && n.devicePath.toLowerCase().includes(query))
    );
  }

  // Flat list mode
  if (viewMode.value === 'list') {
    return [...allHierarchyNodes.value].sort((a, b) => a.fullPath.localeCompare(b.fullPath));
  }

  // Tree view mode: pre-order depth-first traversal respecting collapsed states
  const result: ZfsHierarchyNode[] = [];
  function traverse(nodes: ZfsHierarchyNode[]) {
    for (const n of nodes) {
      result.push(n);
      if (n.children.length > 0 && !collapsedPaths.value.has(n.id)) {
        traverse(n.children);
      }
    }
  }
  traverse(poolHierarchyTree.value);
  return result;
});

// Column sorting for the hierarchy table (siblings are sorted in tree mode)
const hierarchySort = useTableSort(visibleHierarchyNodes, {
  name: (n) => n.name,
  usage: (n) => n.usedBytes,
  snapshots: (n) => (n.type === 'dataset' ? n.snapshotsCount || 0 : null),
});

// Child node id -> parent node id, derived from the assembled tree
const hierarchyParentIds = computed(() => {
  const map = new Map<string, string>();
  for (const n of allHierarchyNodes.value) {
    for (const c of n.children) map.set(c.id, n.id);
  }
  return map;
});

const sortedHierarchyNodes = computed<ZfsHierarchyNode[]>(() => {
  if (!hierarchySort.key.value) return visibleHierarchyNodes.value;
  // Flat list / search modes: plain sort
  if (viewMode.value === 'list' || searchQuery.value.trim()) return hierarchySort.sorted.value;
  // Tree mode: sort siblings while keeping children under their parent
  // (collapsed descendants are already excluded from visibleHierarchyNodes)
  return sortHierarchy(
    visibleHierarchyNodes.value,
    (n) => n.id,
    (n) => hierarchyParentIds.value.get(n.id),
    hierarchySort.compare
  );
});

// ---------------------------------------------------------------------------
// 5. Creation & Mutation Forms (Dataset & ZVol)
// ---------------------------------------------------------------------------

const showCreateDataset = ref(false);
const datasetParentPrefix = ref('');
const datasetForm = reactive({
  datasetId: '',
  compression: 'lz4',
  recordsize: '128K',
  quotaGib: 0,
  reservationGib: 0,
  sync: 'standard',
  readonly: false,
  atime: false,
  dedup: false,
});

function openCreateDataset(parentPrefix = '') {
  datasetParentPrefix.value = parentPrefix;
  datasetForm.datasetId = '';
  datasetForm.quotaGib = 0;
  datasetForm.reservationGib = 0;
  datasetForm.sync = 'standard';
  datasetForm.readonly = false;
  datasetForm.atime = false;
  datasetForm.dedup = false;
  showCreateDataset.value = true;
  showCreateVolume.value = false;
}

async function handleCreateDataset() {
  if (!activePool.value) return;
  try {
    const fullId = datasetParentPrefix.value
      ? `${datasetParentPrefix.value}/${datasetForm.datasetId}`
      : datasetForm.datasetId;
    const cleanId = fullId.replace(/\//g, '-');
    const ds = await services.zfs.createDataset(activePool.value.name, cleanId, {
      compression: datasetForm.compression,
      recordsize: datasetForm.recordsize,
      quotaBytes: datasetForm.quotaGib > 0 ? gibToBytes(datasetForm.quotaGib) : 0,
      reservationBytes: datasetForm.reservationGib > 0 ? gibToBytes(datasetForm.reservationGib) : 0,
      sync: datasetForm.sync,
      readonly: datasetForm.readonly,
      atime: datasetForm.atime,
      dedup: datasetForm.dedup,
    });
    notify('success', `Created ZFS dataset ${ds.datasetPath}`);
    showCreateDataset.value = false;
    await refreshZfsSubresources(activePool.value.name);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create dataset');
  }
}

const showCreateVolume = ref(false);
const volumeForm = reactive({
  volumeId: '',
  sizeGib: 20,
  volblocksize: '16K',
  compression: 'lz4',
  thinProvisioned: true,
  sync: 'standard',
  readonly: false,
  dedup: false,
});

function openCreateVolume() {
  volumeForm.volumeId = '';
  volumeForm.sizeGib = 20;
  volumeForm.volblocksize = '16K';
  volumeForm.compression = 'lz4';
  volumeForm.thinProvisioned = true;
  volumeForm.sync = 'standard';
  volumeForm.readonly = false;
  volumeForm.dedup = false;
  showCreateVolume.value = true;
  showCreateDataset.value = false;
}

async function handleCreateVolume() {
  if (!activePool.value) return;
  try {
    const vol = await services.zfs.createVolume(activePool.value.name, volumeForm.volumeId, {
      sizeBytes: gibToBytes(volumeForm.sizeGib),
      volblocksize: volumeForm.volblocksize,
      compression: volumeForm.compression,
      thinProvisioned: volumeForm.thinProvisioned,
      sync: volumeForm.sync,
      readonly: volumeForm.readonly,
      dedup: volumeForm.dedup,
    });
    notify('success', `Created ZVol block volume ${vol.devicePath}`);
    showCreateVolume.value = false;
    await refreshZfsSubresources(activePool.value.name);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create ZVol');
  }
}

// ---------------------------------------------------------------------------
// 6. Dataset / Volume Inline Actions (Edit, Resize, Snapshot, Delete)
// ---------------------------------------------------------------------------

const editingDataset = ref<ZfsDataset | null>(null);
const editDatasetForm = reactive({
  compression: 'lz4',
  recordsize: '128K',
  quotaGib: 0,
  reservationGib: 0,
  sync: 'standard',
  readonly: false,
  atime: false,
  dedup: false,
});

function startEditDataset(ds: ZfsDataset) {
  editingDataset.value = ds;
  editingVolume.value = null;
  editDatasetForm.compression = ds.compression || 'lz4';
  editDatasetForm.recordsize = ds.recordsize || '128K';
  editDatasetForm.quotaGib = bytesToGib(ds.quotaBytes);
  editDatasetForm.reservationGib = bytesToGib(ds.reservationBytes ?? 0);
  editDatasetForm.sync = (ds.sync as any) || 'standard';
  editDatasetForm.readonly = Boolean(ds.readonly);
  editDatasetForm.atime = Boolean(ds.atime);
  editDatasetForm.dedup = Boolean(ds.dedup);
}

async function handleSaveDatasetEdit() {
  if (!editingDataset.value || !activePool.value) return;
  try {
    await services.zfs.updateDataset(
      editingDataset.value.name,
      {
        compression: editDatasetForm.compression,
        recordsize: editDatasetForm.recordsize,
        quotaBytes: editDatasetForm.quotaGib > 0 ? gibToBytes(editDatasetForm.quotaGib) : 0,
        reservationBytes: editDatasetForm.reservationGib > 0 ? gibToBytes(editDatasetForm.reservationGib) : 0,
        sync: editDatasetForm.sync,
        readonly: editDatasetForm.readonly,
        atime: editDatasetForm.atime,
        dedup: editDatasetForm.dedup,
      },
      ['compression', 'recordsize', 'quota_bytes', 'reservation_bytes', 'sync', 'readonly', 'atime', 'dedup']
    );
    notify('success', `Updated dataset ${editingDataset.value.datasetPath}`);
    editingDataset.value = null;
    await refreshZfsSubresources(activePool.value.name);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update dataset');
  }
}

const editingVolume = ref<ZfsVolume | null>(null);
const editVolumeForm = reactive({
  sizeGib: 20,
  thinProvisioned: true,
  sync: 'standard',
  compression: 'lz4',
  readonly: false,
  dedup: false,
});

function startEditVolume(vol: ZfsVolume) {
  editingVolume.value = vol;
  editingDataset.value = null;
  editVolumeForm.sizeGib = Math.max(1, bytesToGib(vol.sizeBytes));
  editVolumeForm.thinProvisioned = Boolean(vol.thinProvisioned);
  editVolumeForm.sync = (vol.sync as any) || 'standard';
  editVolumeForm.compression = vol.compression || 'lz4';
  editVolumeForm.readonly = Boolean(vol.readonly);
  editVolumeForm.dedup = Boolean(vol.dedup);
}

async function handleSaveVolumeEdit() {
  if (!editingVolume.value || !activePool.value) return;
  try {
    await services.zfs.updateVolume(
      editingVolume.value.name,
      {
        sizeBytes: gibToBytes(editVolumeForm.sizeGib),
        thinProvisioned: editVolumeForm.thinProvisioned,
        sync: editVolumeForm.sync,
        compression: editVolumeForm.compression,
        readonly: editVolumeForm.readonly,
        dedup: editVolumeForm.dedup,
      },
      ['size_bytes', 'thin_provisioned', 'sync', 'compression', 'readonly', 'dedup']
    );
    notify('success', `Updated ZVol ${editingVolume.value.volumePath}`);
    editingVolume.value = null;
    await refreshZfsSubresources(activePool.value.name);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update ZVol');
  }
}

const snapshotModalDataset = ref<ZfsDataset | null>(null);
const snapshotName = ref('snap-daily');

async function handleTakeSnapshot() {
  if (!snapshotModalDataset.value || !activePool.value) return;
  try {
    const updated = await services.zfs.snapshotDataset(
      snapshotModalDataset.value.name,
      snapshotName.value
    );
    notify('success', `Created snapshot ${updated.lastSnapshot}`);
    snapshotModalDataset.value = null;
    await refreshZfsSubresources(activePool.value.name);
  } catch (err: any) {
    notify('error', err?.message || 'Snapshot failed');
  }
}

async function handleDeleteDataset(ds: ZfsDataset) {
  const datasetName = ds.name;
  const shortName = ds.datasetPath || datasetName.replace(/^zfsPools\/[^/]+\/datasets\//, '');
  const res = await confirmDelete({
    kind: 'Dataset',
    name: shortName,
    details: 'All data, child datasets and snapshots under it will be destroyed.',
    confirmText: 'Delete Dataset',
  });
  if (!res.confirmed) return;

  try {
    await services.zfs.deleteDataset(datasetName, true);
    notify('info', `Deleted dataset ${shortName}`);
    if (activePool.value) {
      await refreshZfsSubresources(activePool.value.name);
    }
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete dataset');
  }
}

async function handleDeleteVolume(vol: ZfsVolume) {
  const volName = vol.name;
  const shortName = vol.volumePath || volName.replace(/^zfsPools\/[^/]+\/volumes\//, '');
  const res = await confirmDelete({
    kind: 'Volume',
    name: shortName,
    details: 'All data and snapshots of this volume will be destroyed.',
    confirmText: 'Delete Volume',
  });
  if (!res.confirmed) return;

  try {
    await services.zfs.deleteVolume(volName);
    notify('info', `Deleted ZVol ${shortName}`);
    if (activePool.value) {
      await refreshZfsSubresources(activePool.value.name);
    }
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete ZVol');
  }
}
</script>

<template>
  <div class="zfs-view-container" style="display: flex; flex-direction: column; gap: 24px; width: 100%; max-width: 100%; min-width: 0">
    <!-- ===================================================================== -->
    <!-- Storage Pools Section Header & Pool Cards                             -->
    <!-- ===================================================================== -->
    <div class="card" style="padding: 20px 22px">
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px">
        <div>
          <div style="display: flex; align-items: center; gap: 10px">
            <h2 class="card-title" style="font-size: 19px; margin: 0">
              Pools
            </h2>
          </div>
        </div>

        <button class="btn btn-primary" @click="showCreatePool ? (showCreatePool = false) : openCreatePool()">
          <MdiIcon :path="showCreatePool ? mdiClose : mdiPlus" size="16" style="margin-right: 4px" />
          {{ showCreatePool ? 'Cancel' : 'Create Storage Pool' }}
        </button>
      </div>

      <!-- Advanced Drag-and-Drop ZFS Storage Pool Infrastructure Builder -->
      <div
        v-if="showCreatePool"
        class="pool-builder-card"
        style="
          background: var(--bg-base);
          border: 1px solid var(--border-strong);
          border-radius: 10px;
          padding: 20px;
          margin-top: 18px;
        "
      >
        <!-- Header -->
        <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 16px; border-bottom: 1px solid var(--border-subtle); padding-bottom: 12px">
          <div>
            <h3 style="margin: 0; font-size: 17px; font-weight: 700; color: var(--text-primary)">
              Create Pool
            </h3>
          </div>
          <button class="btn btn-icon btn-sm" @click="showCreatePool = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>

        <!-- Top Settings Grid -->
        <div class="form-grid" style="margin-bottom: 18px">
          <div class="form-group">
            <label>Pool Name</label>
            <input v-model="poolForm.poolId" class="input mono" placeholder="tank" required />
          </div>
          <div class="form-group">
            <label>Mountpoint</label>
            <PathSelector
              v-model="poolForm.mountpoint"
              :placeholder="poolForm.poolId ? `/mnt/${poolForm.poolId.trim()}` : '/mnt/<pool-name>'"
              root-hint="/mnt"
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

        <!-- Main Workspace (Drives Bank & VDEV Canvas) -->
        <div class="builder-workspace" style="display: grid; grid-template-columns: 340px 1fr; gap: 18px; margin-bottom: 18px">
          <!-- Left: Available Drives Bank -->
          <div class="drives-bank-panel" style="background: rgba(0,0,0,0.2); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 14px; display: flex; flex-direction: column">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px">
              <div style="font-weight: 700; font-size: 13px; display: flex; align-items: center; gap: 6px">
                <MdiIcon :path="mdiHarddisk" size="16" style="color: var(--accent-primary)" />
                <span>Available Drives</span>
              </div>
              <span class="badge badge-neutral" style="font-size: 11px">
                {{ unassignedDisks.length }} unattached
              </span>
            </div>

            <!-- Drive Search -->
            <div style="position: relative; margin-bottom: 12px">
              <MdiIcon
                :path="mdiMagnify"
                size="15"
                style="position: absolute; left: 8px; top: 50%; transform: translateY(-50%); color: var(--text-muted)"
              />
              <input
                v-model="driveSearchQuery"
                class="input"
                placeholder="Filter drives, model, serial..."
                style="padding-left: 28px; font-size: 12px; height: 32px"
              />
            </div>

            <!-- Drives List -->
            <div style="overflow-y: auto; max-height: 520px; display: flex; flex-direction: column; gap: 8px; padding-right: 2px">
              <div
                v-if="filteredUnassignedDisks.length === 0"
                style="text-align: center; padding: 24px 10px; color: var(--text-muted); font-size: 12px"
              >
                No unassigned drives.
              </div>

              <div
                v-for="d in filteredUnassignedDisks"
                :key="d.devicePath"
                class="drive-card"
                :class="{ 'is-assigned': builderAssignedPaths.has(d.devicePath) }"
                draggable="true"
                @dragstart="onDragStartDrive($event, d.devicePath)"
                @dragend="onDragEndDrive"
                style="
                  background: var(--bg-surface);
                  border: 1px solid var(--border-subtle);
                  border-radius: 6px;
                  padding: 10px;
                  cursor: grab;
                  user-select: none;
                  transition: all 0.15s ease;
                "
              >
                <div style="display: flex; justify-content: space-between; align-items: flex-start">
                  <div style="display: flex; align-items: center; gap: 6px">
                    <MdiIcon :path="mdiDragVariant" size="16" style="color: var(--text-muted); cursor: grab" />
                    <span class="mono" style="font-weight: 700; font-size: 13px">{{ d.devicePath }}</span>
                  </div>
                  <span class="badge badge-info mono" style="font-weight: 700; font-size: 11px">
                    {{ formatBytes(d.sizeBytes) }}
                  </span>
                </div>

                <div style="font-size: 11.5px; color: var(--text-secondary); margin-top: 4px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
                  {{ d.model || d.vendor || 'Generic Disk' }}
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

                <!-- Quick Assign Dropdown -->
                <div v-if="!builderAssignedPaths.has(d.devicePath)" style="margin-top: 8px; border-top: 1px dashed var(--border-subtle); padding-top: 6px; display: flex; justify-content: flex-end; gap: 4px">
                  <button
                    v-for="(v, vIdx) in builderVdevs"
                    :key="v.id"
                    type="button"
                    class="btn btn-xs"
                    style="font-size: 10px; padding: 1px 6px"
                    title="Click to assign to VDEV"
                    @click="assignDriveToVdev(d.devicePath, v.id)"
                  >
                    + {{ v.type.toUpperCase() }} {{ vIdx + 1 }}
                  </button>
                </div>
              </div>
            </div>
          </div>

          <!-- Right: VDEV Infrastructure Canvas -->
          <div class="vdev-canvas-panel" style="display: flex; flex-direction: column; gap: 14px">
            <div style="display: flex; justify-content: space-between; align-items: center">
              <div style="font-weight: 700; font-size: 13.5px; color: var(--text-primary); display: flex; align-items: center; gap: 6px">
                <span>VDEVs</span>
              </div>

              <!-- Add VDEV Buttons -->
              <div style="display: flex; gap: 6px; flex-wrap: wrap">
                <button type="button" class="btn btn-xs btn-primary-outline" @click="addBuilderVdev('data')">
                  <MdiIcon :path="mdiPlus" size="12" /> Data VDEV
                </button>
                <button type="button" class="btn btn-xs btn-primary-outline" @click="addBuilderVdev('cache')">
                  <MdiIcon :path="mdiPlus" size="12" /> Cache (L2ARC)
                </button>
                <button type="button" class="btn btn-xs btn-primary-outline" @click="addBuilderVdev('log')">
                  <MdiIcon :path="mdiPlus" size="12" /> Log (SLOG)
                </button>
                <button type="button" class="btn btn-xs btn-primary-outline" @click="addBuilderVdev('spare')">
                  <MdiIcon :path="mdiPlus" size="12" /> Hot Spare
                </button>
                <button type="button" class="btn btn-xs btn-primary-outline" @click="addBuilderVdev('special')">
                  <MdiIcon :path="mdiPlus" size="12" /> Special (Metadata)
                </button>
              </div>
            </div>

            <!-- VDEVs List -->
            <div style="display: flex; flex-direction: column; gap: 12px">
              <div
                v-for="(vdev, vIdx) in builderVdevs"
                :key="vdev.id"
                class="vdev-builder-card"
                :class="{
                  'is-dragover': hoveredVdevId === vdev.id,
                  'vdev-type-data': vdev.type === 'data',
                  'vdev-type-cache': vdev.type === 'cache',
                  'vdev-type-log': vdev.type === 'log',
                  'vdev-type-spare': vdev.type === 'spare',
                  'vdev-type-special': vdev.type === 'special' || vdev.type === 'dedup',
                }"
                style="
                  background: var(--bg-surface);
                  border: 2px dashed var(--border-strong);
                  border-radius: 8px;
                  padding: 14px;
                  transition: all 0.2s ease;
                "
                @dragover="onDragOverVdev($event, vdev.id)"
                @dragleave="onDragLeaveVdev(vdev.id)"
                @drop="onDropDriveToVdev($event, vdev.id)"
              >
                <!-- VDEV Card Header -->
                <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px">
                  <div style="display: flex; align-items: center; gap: 8px">
                    <span
                      class="badge"
                      :class="{
                        'badge-info': vdev.type === 'data',
                        'badge-success': vdev.type === 'cache',
                        'badge-warning': vdev.type === 'log',
                        'badge-neutral': vdev.type === 'spare',
                        'badge-purple': vdev.type === 'special' || vdev.type === 'dedup',
                      }"
                      style="font-weight: 700; font-size: 11px"
                    >
                      {{ vdev.type.toUpperCase() }}
                    </span>
                    <span style="font-weight: 700; font-size: 13.5px">VDEV #{{ vIdx + 1 }}</span>
                    <span style="font-size: 11px; color: var(--text-muted)">
                      ({{ vdev.devicePaths.length }} drive{{ vdev.devicePaths.length === 1 ? '' : 's' }})
                    </span>
                  </div>

                  <div style="display: flex; align-items: center; gap: 8px">
                    <div v-if="vdev.type === 'data'" style="display: flex; align-items: center; gap: 6px">
                      <label style="font-size: 11.5px; color: var(--text-secondary)">Layout:</label>
                      <select v-model="vdev.layout" class="select mono" style="height: 28px; font-size: 12px; padding: 2px 8px">
                        <option value="mirror">Mirror (RAID-1)</option>
                        <option value="raidz1">RAIDZ1 (Single Parity)</option>
                        <option value="raidz2">RAIDZ2 (Dual Parity)</option>
                        <option value="raidz3">RAIDZ3 (Triple Parity)</option>
                        <option value="stripe">Stripe (No Parity / RAID-0)</option>
                      </select>
                    </div>

                    <div v-else-if="vdev.type === 'log' || vdev.type === 'special'" style="display: flex; align-items: center; gap: 6px">
                      <label style="font-size: 11.5px; color: var(--text-secondary)">Layout:</label>
                      <select v-model="vdev.layout" class="select mono" style="height: 28px; font-size: 12px; padding: 2px 8px">
                        <option value="mirror">Mirror (Recommended)</option>
                        <option value="stripe">Stripe (Single)</option>
                      </select>
                    </div>

                    <button
                      v-if="builderVdevs.length > 1"
                      type="button"
                      class="btn btn-icon btn-sm btn-danger"
                      title="Remove this VDEV tier"
                      @click="removeBuilderVdev(vdev.id)"
                    >
                      <MdiIcon :path="mdiDelete" size="14" />
                    </button>
                  </div>
                </div>

                <!-- Drop Target / Drives Chips Container -->
                <div
                  style="
                    min-height: 64px;
                    border-radius: 6px;
                    background: rgba(0,0,0,0.15);
                    padding: 8px;
                    display: flex;
                    flex-wrap: wrap;
                    gap: 8px;
                    align-items: center;
                  "
                >
                  <div
                    v-if="vdev.devicePaths.length === 0"
                    style="color: var(--text-muted); font-size: 12px; padding: 12px; text-align: center; width: 100%"
                  >
                    Drag & drop unassigned drives from the left bank here, or click "+ {{ vdev.type.toUpperCase() }}" on a drive.
                  </div>

                  <!-- Assigned Drive Chips -->
                  <div
                    v-for="path in vdev.devicePaths"
                    :key="path"
                    style="
                      background: var(--bg-base);
                      border: 1px solid var(--border-strong);
                      border-radius: 6px;
                      padding: 6px 10px;
                      display: flex;
                      align-items: center;
                      gap: 8px;
                      font-size: 12px;
                    "
                  >
                    <MdiIcon :path="mdiHarddisk" size="14" style="color: var(--accent-primary)" />
                    <span class="mono" style="font-weight: 700">{{ path }}</span>
                    <span class="mono" style="color: var(--text-secondary); font-size: 11px">
                      {{ formatBytes(getDiskByPath(path)?.sizeBytes || 0) }}
                    </span>
                    <button
                      type="button"
                      class="btn-icon"
                      style="background: none; border: none; padding: 0; cursor: pointer; color: var(--danger); display: flex"
                      title="Remove drive from this VDEV"
                      @click="removeDriveFromVdev(path, vdev.id)"
                    >
                      <MdiIcon :path="mdiClose" size="14" />
                    </button>
                  </div>
                </div>

                <!-- Validation Hint Bar -->
                <div style="margin-top: 8px; font-size: 11.5px; display: flex; align-items: center; gap: 6px">
                  <span
                    :style="{
                      color:
                        getVdevValidation(vdev).state === 'ok'
                          ? 'var(--success)'
                          : getVdevValidation(vdev).state === 'warn'
                          ? '#f59e0b'
                          : 'var(--text-muted)',
                    }"
                  >
                    {{ getVdevValidation(vdev).message }}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Bottom Capacity & Validation Summary Bar -->
        <div
          style="
            background: rgba(0, 0, 0, 0.25);
            border: 1px solid var(--border-subtle);
            border-radius: 8px;
            padding: 14px 18px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            flex-wrap: wrap;
            gap: 12px;
          "
        >
          <div style="display: flex; gap: 24px; align-items: center; flex-wrap: wrap">
            <div>
              <div style="font-size: 11px; color: var(--text-secondary); text-transform: uppercase; font-weight: 600">
                Total Raw Storage
              </div>
              <div class="mono" style="font-size: 15px; font-weight: 700">
                {{ formatBytes(poolCapacitySummary.totalRawBytes) }}
              </div>
            </div>

            <div>
              <div style="font-size: 11px; color: var(--text-secondary); text-transform: uppercase; font-weight: 600">
                Estimated Usable Storage
              </div>
              <div class="mono text-emerald" style="font-size: 15px; font-weight: 700">
                {{ formatBytes(poolCapacitySummary.estimatedUsableBytes) }}
              </div>
            </div>

            <div>
              <div style="font-size: 11px; color: var(--text-secondary); text-transform: uppercase; font-weight: 600">
                Drive Tiers
              </div>
              <div style="font-size: 12px; color: var(--text-primary)">
                {{ poolCapacitySummary.dataDrivesCount }} Data
                <span v-if="poolCapacitySummary.cacheDrivesCount > 0">, {{ poolCapacitySummary.cacheDrivesCount }} L2ARC Cache</span>
                <span v-if="poolCapacitySummary.logDrivesCount > 0">, {{ poolCapacitySummary.logDrivesCount }} SLOG</span>
                <span v-if="poolCapacitySummary.spareDrivesCount > 0">, {{ poolCapacitySummary.spareDrivesCount }} Spare</span>
              </div>
            </div>
          </div>

          <div style="display: flex; gap: 10px; align-items: center">
            <button type="button" class="btn btn-sm" @click="showCreatePool = false">Cancel</button>
            <button
              type="button"
              class="btn btn-primary btn-sm"
              :disabled="!isPoolBuilderValid"
              @click="handleCreatePool"
            >
              <MdiIcon :path="mdiCheck" size="15" style="margin-right: 4px" />
              Create Storage Pool
            </button>
          </div>
        </div>
      </div>

      <!-- Pools Grid as Rich Cards with Gauges -->
      <div class="pools-grid" style="margin-top: 20px">
        <div
          v-for="pool in nasState.pools"
          :key="pool.name"
          class="pool-card"
          :class="{ 'is-active': activePool?.name === pool.name }"
          @click="selectPool(pool.name)"
        >
          <!-- Card Header -->
          <div class="pool-card-header">
            <div class="pool-title-group">
              <div class="pool-icon-wrap">
                <MdiIcon :path="mdiHarddisk" size="22" style="color: var(--accent-primary)" />
              </div>
              <div>
                <div class="pool-name mono">{{ pool.poolName || pool.name.replace(/^zfsPools\//, '') }}</div>
              </div>
            </div>

            <div class="pool-badges">
              <span
                v-if="(pool.poolName || pool.name.replace(/^zfsPools\//, '')) === nasState.systemConfig?.mainPool"
                class="badge badge-main-pool"
                title="Active Main Storage Pool (Root for Apps & LXC)"
              >
                <MdiIcon :path="mdiCrown" size="13" /> MAIN POOL
              </span>
              <span class="badge badge-neutral layout-badge">
                {{ pool.vdevLayout.toUpperCase() }}
              </span>
              <span
                class="badge"
                :class="pool.state === 'ONLINE' ? 'badge-success' : 'badge-warning'"
              >
                {{ pool.state }}
              </span>
            </div>
          </div>

          <!-- Card Body with Gauge & Metrics -->
          <div class="pool-card-body">
            <!-- Left: Dual Gauge (Usage & Fragmentation) -->
            <div class="gauge-pane">
              <ZfsPoolGauge
                :capacity-percent="pool.capacityPercent"
                :fragmentation-percent="pool.fragmentationPercent"
                :size="136"
              />
            </div>

            <!-- Right: Stats & Key Metrics -->
            <div class="info-pane">
              <!-- Three Main Capacity Metrics -->
              <div class="info-stat-row">
                <div class="stat-col">
                  <span class="stat-label">Allocated</span>
                  <span class="stat-value text-sky mono">{{ formatBytes(pool.allocatedBytes) }}</span>
                </div>
                <div class="stat-col">
                  <span class="stat-label">Free</span>
                  <span class="stat-value text-emerald mono">{{ formatBytes(pool.freeBytes) }}</span>
                </div>
                <div class="stat-col">
                  <span class="stat-label">Total Size</span>
                  <span class="stat-value mono">{{ formatBytes(pool.sizeBytes) }}</span>
                </div>
              </div>

              <!-- Property Details Grid -->
              <div class="info-props-table">
                <div class="prop-item">
                  <span class="prop-key">Sync:</span>
                  <span class="prop-val badge badge-neutral mono">{{ pool.sync || 'standard' }}</span>
                </div>
                <div class="prop-item">
                  <span class="prop-key">TRIM:</span>
                  <span class="prop-val badge badge-neutral mono">{{ pool.autotrim || 'off' }}</span>
                </div>
                <div class="prop-item">
                  <span class="prop-key">Compression:</span>
                  <span class="prop-val badge badge-info mono">{{ pool.compression }}</span>
                </div>
                <div class="prop-item">
                  <span class="prop-key">Dedup:</span>
                  <span class="prop-val mono">{{ pool.dedupRatio.toFixed(2) }}x</span>
                </div>
                <div class="prop-item full-width">
                  <span class="prop-key">Devices:</span>
                  <span class="prop-val mono dev-list" :title="(pool.devices || []).join(', ')">
                    {{ (pool.devices || []).join(', ') || 'None' }}
                  </span>
                </div>
                <div class="prop-item full-width">
                  <span class="prop-key">Scrub:</span>
                  <span class="prop-val scrub-text">{{ pool.lastScrubStatus }}</span>
                </div>
              </div>
            </div>
          </div>

          <!-- Card Footer Actions -->
          <div class="pool-card-footer" @click.stop>
            <div class="active-indicator">
              <span v-if="activePool?.name === pool.name" class="active-pill">
                <MdiIcon :path="mdiCheck" size="14" style="margin-right: 4px" />
                Selected Pool
              </span>
              <span v-else class="inactive-hint">
                Click to inspect datasets
              </span>
            </div>

            <div class="card-action-btns">
              <button
                v-if="(pool.poolName || pool.name.replace(/^zfsPools\//, '')) !== nasState.systemConfig?.mainPool"
                class="btn btn-sm btn-primary-outline"
                title="Designate as Main Storage Pool"
                @click="handleSetMainPool(pool.poolName || pool.name.replace(/^zfsPools\//, ''))"
              >
                <MdiIcon :path="mdiCrown" size="14" style="margin-right: 4px; color: #fbbf24" />
                Set Main
              </button>
              <button
                class="btn btn-sm btn-primary-outline"
                title="Configure pool settings and storage tiers"
                @click="openPoolConfig(pool)"
              >
                <MdiIcon :path="mdiPencil" size="14" style="margin-right: 4px" />
                Configure
              </button>
              <button
                class="btn btn-sm"
                title="Verify data integrity with scrub"
                @click="handleScrubPool(pool.name)"
              >
                <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" />
                Scrub
              </button>
              <button
                class="btn btn-sm btn-primary-outline"
                title="Export pool (unmount and detach from system)"
                @click="handleExportPool(pool.name)"
              >
                <MdiIcon :path="mdiDatabaseExportOutline" size="14" style="margin-right: 4px" />
                Export
              </button>
              <button
                class="btn btn-sm btn-danger"
                title="Destroy pool"
                @click="handleDeletePool(pool.name)"
              >
                <MdiIcon :path="mdiDelete" size="14" style="margin-right: 4px" />
                Destroy
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ===================================================================== -->
    <!-- Pool Configuration & VDEV Management Modal                            -->
    <!-- ===================================================================== -->
    <div v-if="configuringPool" class="modal-overlay">
      <div class="modal-container" style="max-width: 720px; width: 95%">
        <!-- Modal Header -->
        <div class="modal-header">
          <div style="display: flex; align-items: center; gap: 10px">
            <div class="pool-icon-wrap" style="width: 32px; height: 32px">
              <MdiIcon :path="mdiPencil" size="20" style="color: var(--accent-primary)" />
            </div>
            <div>
              <h3 style="margin: 0; font-size: 17px; font-weight: 700">
                Configure Pool: <span class="mono" style="color: var(--accent-primary)">{{ configuringPool.poolName || configuringPool.name.replace(/^zfsPools\//, '') }}</span>
              </h3>
            </div>
          </div>
          <button class="btn btn-icon btn-sm" @click="closePoolConfig">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>

        <!-- Modal Nav Tabs -->
        <div class="modal-tabs">
          <button
            class="tab-btn"
            :class="{ active: configTab === 'settings' }"
            @click="configTab = 'settings'"
          >
            <MdiIcon :path="mdiTune" size="15" style="margin-right: 6px" />
            Properties
          </button>
          <button
            class="tab-btn"
            :class="{ active: configTab === 'vdevs' }"
            @click="configTab = 'vdevs'"
          >
            <MdiIcon :path="mdiLayersOutline" size="15" style="margin-right: 6px" />
            VDEVs
          </button>
        </div>

        <!-- Modal Body: Tab 1 (Settings) -->
        <div v-if="configTab === 'settings'" class="modal-body">
          <form @submit.prevent="handleSavePoolSettings" style="display: flex; flex-direction: column; gap: 18px">
            <!-- Sync Mode -->
            <div class="config-section">
              <label class="config-label">
                <span>Sync Mode</span>
                <span class="badge badge-neutral mono">{{ poolSettingsForm.sync }}</span>
              </label>
              <select v-model="poolSettingsForm.sync" class="select mono" style="width: 100%">
                <option value="standard">standard - Respect application fsync / O_SYNC (Recommended)</option>
                <option value="always">always - Strict safety: every write committed synchronously</option>
                <option value="disabled">disabled - Extreme throughput: ignore sync (Risk of power loss data loss)</option>
              </select>
              <div class="config-hint">
                Controls synchronous transaction logging. Set to <strong>standard</strong> for balanced safety and throughput, or <strong>always</strong> when hosting database or virtual machine workloads.
              </div>
            </div>

            <!-- Auto TRIM -->
            <div class="config-section">
              <label class="config-label">
                <span>Auto TRIM (SSD)</span>
                <span class="badge badge-neutral mono">{{ poolSettingsForm.autotrim }}</span>
              </label>
              <select v-model="poolSettingsForm.autotrim" class="select mono" style="width: 100%">
                <option value="on">on - Automatically TRIM/UNMAP freed blocks continuously</option>
                <option value="off">off - Do not automatically issue TRIM commands</option>
              </select>
            </div>

            <!-- Compression -->
            <div class="config-section">
              <label class="config-label">
                <span>Default Compression</span>
                <span class="badge badge-info mono">{{ poolSettingsForm.compression }}</span>
              </label>
              <select v-model="poolSettingsForm.compression" class="select mono" style="width: 100%">
                <option value="lz4">lz4 (Fast, zero-overhead, highly recommended)</option>
                <option value="zstd">zstd (Higher ratio compression)</option>
                <option value="gzip">gzip (High ratio legacy)</option>
                <option value="off">off (No compression)</option>
              </select>
              <div class="config-hint">
                Applied to new datasets in this pool.
              </div>
            </div>

            <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 10px">
              <button type="button" class="btn btn-sm" @click="closePoolConfig">Cancel</button>
              <button type="submit" class="btn btn-primary btn-sm">
                <MdiIcon :path="mdiCheck" size="15" style="margin-right: 4px" />
                Save Pool Settings
              </button>
            </div>
          </form>
        </div>

        <!-- Modal Body: Tab 2 (VDEVs & Storage Tiers) -->
        <div v-else-if="configTab === 'vdevs'" class="modal-body">
          <!-- Active VDEVs & Storage Tiers List -->
          <div style="margin-bottom: 22px">
            <div style="font-weight: 700; font-size: 13.5px; margin-bottom: 8px; color: var(--text-primary); display: flex; align-items: center; gap: 6px">
              VDEVs
            </div>

            <!-- VDEV Tree / List -->
            <div v-if="(configuringPool.vdevs || []).length > 0" class="vdev-tier-list">
              <div
                v-for="vdev in configuringPool.vdevs"
                :key="vdev.name"
                class="vdev-card"
              >
                <div class="vdev-header">
                  <div style="display: flex; align-items: center; gap: 8px">
                    <span
                      class="badge tier-badge"
                      :class="{
                        'badge-info': vdev.type === 'data',
                        'badge-success': vdev.type === 'cache',
                        'badge-warning': vdev.type === 'log',
                        'badge-purple': vdev.type === 'special' || vdev.type === 'dedup',
                        'badge-neutral': vdev.type === 'spare',
                      }"
                    >
                      {{ vdev.type.toUpperCase() }}
                    </span>
                    <span class="mono" style="font-weight: 700; font-size: 13px">{{ vdev.name }}</span>
                    <span class="badge badge-neutral mono" style="font-size: 10px">{{ vdev.layout || 'stripe' }}</span>
                  </div>

                  <div style="display: flex; align-items: center; gap: 8px">
                    <span class="badge badge-success" style="font-size: 10.5px">{{ vdev.state }}</span>
                    <button
                      v-if="vdev.type !== 'data' || vdev.layout === 'stripe'"
                      class="btn btn-sm btn-danger btn-icon"
                      title="Remove VDEV from pool"
                      @click="handleRemoveVdev(vdev.name)"
                    >
                      <MdiIcon :path="mdiDelete" size="13" />
                    </button>
                  </div>
                </div>

                <!-- Child devices if composite -->
                <div v-if="vdev.children && vdev.children.length > 0" class="vdev-children">
                  <div
                    v-for="child in vdev.children"
                    :key="child.name"
                    class="vdev-child-item"
                  >
                    <div style="display: flex; align-items: center; gap: 6px">
                      <MdiIcon :path="mdiHarddisk" size="14" style="color: var(--text-secondary)" />
                      <span class="mono" style="font-size: 12px; font-weight: 600">{{ child.name }}</span>
                      <span v-if="child.path && child.path !== child.name" class="mono" style="font-size: 11px; opacity: 0.6">
                        ({{ child.path }})
                      </span>
                    </div>

                    <div style="display: flex; align-items: center; gap: 10px">
                      <span class="mono" style="font-size: 10.5px; color: var(--text-secondary)">
                        R:{{ child.readErrors || '0' }} W:{{ child.writeErrors || '0' }} C:{{ child.checksumErrors || '0' }}
                      </span>
                      <button
                        class="btn btn-icon btn-sm"
                        style="padding: 2px"
                        title="Remove device"
                        @click="handleRemoveVdev(child.name)"
                      >
                        <MdiIcon :path="mdiDelete" size="12" style="color: var(--danger)" />
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <!-- Fallback if vdevs tree not yet populated: show devices array -->
            <div v-else class="vdev-tier-list">
              <div
                v-for="dev in configuringPool.devices"
                :key="dev"
                class="vdev-card"
              >
                <div class="vdev-header">
                  <div style="display: flex; align-items: center; gap: 8px">
                    <span class="badge badge-info tier-badge">DATA</span>
                    <span class="mono" style="font-weight: 700; font-size: 13px">{{ dev }}</span>
                  </div>
                  <span class="badge badge-success" style="font-size: 10.5px">ONLINE</span>
                </div>
              </div>
            </div>
          </div>

          <!-- Add VDEV / Storage Tier Form -->
          <div class="add-vdev-box">
            <div style="font-weight: 700; font-size: 13.5px; margin-bottom: 12px; color: var(--text-primary); display: flex; align-items: center; gap: 6px">
              Add VDEV
            </div>

            <div class="form-grid">
              <div class="form-group">
                <label>Tier / VDEV Role</label>
                <select v-model="addVdevForm.type" class="select">
                  <option value="cache">cache - Read Cache (L2ARC SSD)</option>
                  <option value="log">log - Write Log (SLOG / ZIL NVMe)</option>
                  <option value="special">special - Metadata & Small Files</option>
                  <option value="dedup">dedup - Deduplication Tables</option>
                  <option value="spare">spare - Hot Spare Drive</option>
                  <option value="data">data - Expand Pool Capacity</option>
                </select>
              </div>

              <div class="form-group">
                <label>VDEV Layout</label>
                <select v-model="addVdevForm.layout" class="select">
                  <option value="stripe">Stripe (Single / JBOD)</option>
                  <option value="mirror">Mirror (Mirrored Redundancy)</option>
                  <option value="raidz1">RAIDZ1 (1 parity disk)</option>
                  <option value="raidz2">RAIDZ2 (2 parity disks)</option>
                </select>
              </div>

              <div class="form-group full-width">
                <label>Device Paths (comma-separated)</label>
                <div v-if="unassignedDisks.length > 0" style="margin-bottom: 8px">
                  <span style="font-size: 11px; color: var(--text-secondary); margin-bottom: 4px; display: block">
                    Unassigned Disks (click to add/remove):
                  </span>
                  <div style="display: flex; flex-wrap: wrap; gap: 6px">
                    <button
                      v-for="d in unassignedDisks"
                      :key="d.devicePath"
                      type="button"
                      class="chip"
                      :style="{
                        cursor: 'pointer',
                        fontSize: '11px',
                        padding: '3px 8px',
                        borderRadius: '4px',
                        border: addVdevForm.devicesText.split(',').map((s) => s.trim()).includes(d.devicePath)
                          ? '1px solid var(--accent-primary)'
                          : '1px solid var(--border-subtle)',
                        background: addVdevForm.devicesText.split(',').map((s) => s.trim()).includes(d.devicePath)
                          ? 'rgba(59, 130, 246, 0.2)'
                          : 'var(--bg-surface)',
                        color: addVdevForm.devicesText.split(',').map((s) => s.trim()).includes(d.devicePath)
                          ? 'var(--accent-primary)'
                          : 'var(--text-secondary)',
                      }"
                      @click="toggleDeviceInAddVdev(d.devicePath)"
                    >
                      <span class="mono">{{ d.devicePath }}</span>
                      <span style="opacity: 0.7; margin-left: 4px">({{ formatBytes(d.sizeBytes) }} - {{ d.type || 'Disk' }})</span>
                    </button>
                  </div>
                </div>
                <input
                  v-model="addVdevForm.devicesText"
                  class="input mono"
                  placeholder="e.g. /dev/nvme0n1p2 or /dev/sde, /dev/sdf"
                  required
                />
                <span style="font-size: 11px; color: var(--text-secondary); margin-top: 4px; display: block">
                  Example for SLOG/Cache: <code>/dev/nvme0n1p2</code>. Example for Mirrored Special: <code>/dev/sde, /dev/sdf</code>.
                </span>
              </div>
            </div>

            <div style="display: flex; justify-content: flex-end; margin-top: 14px">
              <button class="btn btn-primary btn-sm" @click="handleAddVdev">
                <MdiIcon :path="mdiPlus" size="15" style="margin-right: 4px" />
                Add Storage Tier
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ===================================================================== -->
    <!-- Selected Pool: Unified Datasets & ZVols Tree / List Explorer          -->
    <!-- ===================================================================== -->
    <div v-if="activePool" class="card" style="padding: 20px 22px">
      <!-- Section Header -->
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 14px; padding-bottom: 14px; border-bottom: 1px solid var(--border-subtle)">
        <div>
          <div style="display: flex; align-items: center; gap: 10px">
            <h3 class="card-title" style="font-size: 17px; margin: 0">
              Datasets & Volumes in <span class="mono" style="color: var(--accent-primary)">{{ activePool.poolName || activePool.name.replace(/^zfsPools\//, '') }}</span>
            </h3>
          </div>
        </div>

        <!-- Controls: Search, View Mode, Create Buttons -->
        <div style="display: flex; align-items: center; gap: 10px; flex-wrap: wrap">
          <!-- Search filter -->
          <div class="search-input-wrap">
            <MdiIcon :path="mdiMagnify" size="16" class="search-icon" />
            <input
              v-model="searchQuery"
              class="input mono search-input"
              placeholder="Filter datasets / zvols..."
            />
          </div>

          <!-- Tree vs List View Mode Toggle -->
          <div class="view-mode-toggle">
            <button
              class="toggle-btn"
              :class="{ active: viewMode === 'tree' }"
              title="Hierarchical Tree View"
              @click="viewMode = 'tree'"
            >
              <MdiIcon :path="mdiTreeOutline" size="16" />
              <span class="toggle-text">Tree</span>
            </button>
            <button
              class="toggle-btn"
              :class="{ active: viewMode === 'list' }"
              title="Flat List View"
              @click="viewMode = 'list'"
            >
              <MdiIcon :path="mdiFormatListBulleted" size="16" />
              <span class="toggle-text">Flat</span>
            </button>
          </div>

          <!-- Tree Expand / Collapse All -->
          <div v-if="viewMode === 'tree' && !searchQuery" class="tree-expand-controls">
            <button class="btn btn-sm btn-icon" title="Expand All" @click="expandAll">
              <MdiIcon :path="mdiChevronDown" size="16" />
            </button>
            <button class="btn btn-sm btn-icon" title="Collapse All" @click="collapseAll">
              <MdiIcon :path="mdiChevronRight" size="16" />
            </button>
          </div>

          <!-- Action Buttons -->
          <button class="btn btn-primary btn-sm" @click="openCreateDataset()">
            <MdiIcon :path="mdiPlus" size="15" style="margin-right: 4px" />
            New Dataset
          </button>
          <button class="btn btn-sm" style="border: 1px solid var(--border-strong)" @click="openCreateVolume()">
            <MdiIcon :path="mdiHarddisk" size="15" style="margin-right: 4px; color: #a855f7" />
            New ZVol
          </button>
        </div>
      </div>

      <!-- Create Dataset Collapsible Form -->
      <form
        v-if="showCreateDataset"
        @submit.prevent="handleCreateDataset"
        class="action-panel"
      >
        <div style="font-weight: 700; font-size: 13.5px; margin-bottom: 10px; color: var(--text-primary); display: flex; align-items: center; justify-content: space-between">
          <div style="display: flex; align-items: center; gap: 6px">
            <MdiIcon :path="mdiDatabase" size="16" style="color: var(--accent-primary)" />
            <span>Create Dataset</span>
            <span v-if="datasetParentPrefix" class="badge badge-info mono" style="font-size: 11px">
              Parent: {{ datasetParentPrefix }}
            </span>
          </div>
          <button type="button" class="btn btn-icon btn-sm" @click="showCreateDataset = false">
            <MdiIcon :path="mdiClose" size="15" />
          </button>
        </div>

        <div class="form-grid">
          <div class="form-group">
            <label>Dataset Name</label>
            <div style="display: flex; align-items: center; gap: 4px">
              <span v-if="datasetParentPrefix" class="mono" style="font-size: 12px; color: var(--text-secondary)">
                {{ datasetParentPrefix }}/
              </span>
              <input v-model="datasetForm.datasetId" class="input mono" placeholder="e.g. apps or media" required />
            </div>
          </div>
          <div class="form-group">
            <label>Sync Mode</label>
            <select v-model="datasetForm.sync" class="select mono">
              <option value="standard">standard (Default fsync safe)</option>
              <option value="always">always (Force sync transactions)</option>
              <option value="disabled">disabled (Ignore sync requests)</option>
            </select>
          </div>
          <div class="form-group">
            <label>Compression</label>
            <select v-model="datasetForm.compression" class="select">
              <option value="lz4">lz4 (Fast & Recommended)</option>
              <option value="zstd">zstd (Higher ratio)</option>
              <option value="gzip">gzip</option>
              <option value="off">off</option>
            </select>
          </div>
          <div class="form-group">
            <label>Recordsize</label>
            <select v-model="datasetForm.recordsize" class="select">
              <option value="16K">16K (Databases / VM)</option>
              <option value="32K">32K</option>
              <option value="64K">64K</option>
              <option value="128K">128K (Default General)</option>
              <option value="1M">1M (Large Media)</option>
            </select>
          </div>
          <div class="form-group">
            <label>Quota (GiB, 0 = unlimited)</label>
            <input v-model.number="datasetForm.quotaGib" type="number" min="0" class="input mono" />
          </div>
          <div class="form-group">
            <label>Space Reservation (GiB, 0 = none)</label>
            <input v-model.number="datasetForm.reservationGib" type="number" min="0" class="input mono" />
          </div>
        </div>

        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 14px; flex-wrap: wrap; gap: 10px">
          <div style="display: flex; gap: 16px; align-items: center">
            <label style="font-size: 12px; display: flex; align-items: center; gap: 6px; cursor: pointer">
              <input v-model="datasetForm.readonly" type="checkbox" /> Read-only Dataset
            </label>
            <label style="font-size: 12px; display: flex; align-items: center; gap: 6px; cursor: pointer">
              <input v-model="datasetForm.atime" type="checkbox" /> Update Access Times (atime)
            </label>
            <label style="font-size: 12px; display: flex; align-items: center; gap: 6px; cursor: pointer">
              <input v-model="datasetForm.dedup" type="checkbox" /> Inline Deduplication
            </label>
          </div>
          <div style="display: flex; gap: 8px">
            <button type="button" class="btn btn-sm" @click="showCreateDataset = false">Cancel</button>
            <button type="submit" class="btn btn-primary btn-sm">Create Dataset</button>
          </div>
        </div>
      </form>

      <!-- Create Volume Collapsible Form -->
      <form
        v-if="showCreateVolume"
        @submit.prevent="handleCreateVolume"
        class="action-panel"
      >
        <div style="font-weight: 700; font-size: 13.5px; margin-bottom: 10px; color: var(--text-primary); display: flex; align-items: center; justify-content: space-between">
          <div style="display: flex; align-items: center; gap: 6px">
            <MdiIcon :path="mdiHarddisk" size="16" style="color: #a855f7" />
            <span>Create Volume</span>
          </div>
          <button type="button" class="btn btn-icon btn-sm" @click="showCreateVolume = false">
            <MdiIcon :path="mdiClose" size="15" />
          </button>
        </div>

        <div class="form-grid">
          <div class="form-group">
            <label>ZVol Name</label>
            <input v-model="volumeForm.volumeId" class="input mono" placeholder="e.g. vm-disk0" required />
          </div>
          <div class="form-group">
            <label>Provisioned Size (GiB)</label>
            <input v-model.number="volumeForm.sizeGib" type="number" min="1" class="input mono" required />
          </div>
          <div class="form-group">
            <label>Volblocksize</label>
            <select v-model="volumeForm.volblocksize" class="select">
              <option value="8K">8K</option>
              <option value="16K">16K (KVM Optimal)</option>
              <option value="64K">64K</option>
              <option value="128K">128K</option>
            </select>
          </div>
          <div class="form-group">
            <label>Sync Mode</label>
            <select v-model="volumeForm.sync" class="select mono">
              <option value="standard">standard (Default fsync safe)</option>
              <option value="always">always (Force sync transactions)</option>
              <option value="disabled">disabled (Ignore sync requests)</option>
            </select>
          </div>
          <div class="form-group">
            <label>Compression</label>
            <select v-model="volumeForm.compression" class="select">
              <option value="lz4">lz4 (Recommended)</option>
              <option value="zstd">zstd</option>
              <option value="off">off</option>
            </select>
          </div>
          <div class="form-group">
            <label>Provisioning Mode</label>
            <select :value="volumeForm.thinProvisioned ? 'thin' : 'thick'" class="select" @change="volumeForm.thinProvisioned = ($event.target as HTMLSelectElement).value === 'thin'">
              <option value="thin">Thin Provisioned (Sparse / On-demand)</option>
              <option value="thick">Thick Provisioned (Pre-allocated reservation)</option>
            </select>
          </div>
        </div>

        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 14px; flex-wrap: wrap; gap: 10px">
          <div style="display: flex; gap: 16px; align-items: center">
            <label style="font-size: 12px; display: flex; align-items: center; gap: 6px; cursor: pointer">
              <input v-model="volumeForm.readonly" type="checkbox" /> Read-only Block Device
            </label>
            <label style="font-size: 12px; display: flex; align-items: center; gap: 6px; cursor: pointer">
              <input v-model="volumeForm.dedup" type="checkbox" /> Inline Deduplication
            </label>
          </div>
          <div style="display: flex; gap: 8px">
            <button type="button" class="btn btn-sm" @click="showCreateVolume = false">Cancel</button>
            <button type="submit" class="btn btn-primary btn-sm">Create ZVol</button>
          </div>
        </div>
      </form>

      <!-- Edit Dataset Inline / Modal Panel -->
      <div v-if="editingDataset" class="action-panel" style="border-color: var(--accent-primary)">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px">
          <div style="font-weight: 700; font-size: 14px; color: var(--text-primary); display: flex; align-items: center; gap: 8px">
            <MdiIcon :path="mdiTune" size="18" style="color: var(--accent-primary)" />
            <span>Edit Dataset:</span>
            <span class="mono" style="color: var(--accent-primary)">{{ editingDataset.datasetPath }}</span>
            <span v-if="editingDataset.mountpoint" class="mono" style="font-size: 11px; color: var(--text-secondary)">({{ editingDataset.mountpoint }})</span>
          </div>
          <button class="btn btn-icon btn-sm" @click="editingDataset = null">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>

        <div class="form-grid">
          <div class="form-group">
            <label>Sync Mode</label>
            <select v-model="editDatasetForm.sync" class="select mono">
              <option value="standard">standard - Respect application fsync / O_SYNC (Recommended)</option>
              <option value="always">always - Strict safety: commit every write synchronously</option>
              <option value="disabled">disabled - Highest throughput: ignore sync</option>
            </select>
          </div>
          <div class="form-group">
            <label>Compression</label>
            <select v-model="editDatasetForm.compression" class="select">
              <option value="lz4">lz4 (Fast & Recommended)</option>
              <option value="zstd">zstd (Higher ratio)</option>
              <option value="gzip">gzip</option>
              <option value="off">off</option>
            </select>
          </div>
          <div class="form-group">
            <label>Recordsize</label>
            <select v-model="editDatasetForm.recordsize" class="select">
              <option value="16K">16K (Databases / VM)</option>
              <option value="32K">32K</option>
              <option value="64K">64K</option>
              <option value="128K">128K (Default General)</option>
              <option value="1M">1M (Large Media)</option>
            </select>
          </div>
          <div class="form-group">
            <label>Quota (GiB, 0 = unlimited)</label>
            <input v-model.number="editDatasetForm.quotaGib" type="number" min="0" class="input mono" />
          </div>
          <div class="form-group">
            <label>Space Reservation (GiB, 0 = none)</label>
            <input v-model.number="editDatasetForm.reservationGib" type="number" min="0" class="input mono" />
          </div>
        </div>

        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 14px; flex-wrap: wrap; gap: 10px">
          <div style="display: flex; gap: 16px; align-items: center">
            <label style="font-size: 12px; display: flex; align-items: center; gap: 6px; cursor: pointer">
              <input v-model="editDatasetForm.readonly" type="checkbox" /> Read-only Dataset
            </label>
            <label style="font-size: 12px; display: flex; align-items: center; gap: 6px; cursor: pointer">
              <input v-model="editDatasetForm.atime" type="checkbox" /> Update Access Times (atime)
            </label>
            <label style="font-size: 12px; display: flex; align-items: center; gap: 6px; cursor: pointer">
              <input v-model="editDatasetForm.dedup" type="checkbox" /> Inline Deduplication
            </label>
          </div>
          <div style="display: flex; gap: 8px">
            <button class="btn btn-sm" @click="editingDataset = null">Cancel</button>
            <button class="btn btn-primary btn-sm" @click="handleSaveDatasetEdit">
              <MdiIcon :path="mdiCheck" size="14" style="margin-right: 4px" />
              Save Properties
            </button>
          </div>
        </div>
      </div>

      <!-- Edit ZVol Inline / Modal Panel -->
      <div v-if="editingVolume" class="action-panel" style="border-color: #a855f7">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px">
          <div style="font-weight: 700; font-size: 14px; color: var(--text-primary); display: flex; align-items: center; gap: 8px">
            <MdiIcon :path="mdiTune" size="18" style="color: #a855f7" />
            <span>Edit Volume:</span>
            <span class="mono" style="color: #c084fc">{{ editingVolume.volumePath }}</span>
            <span class="mono" style="font-size: 11px; color: var(--text-secondary)">({{ editingVolume.devicePath }})</span>
          </div>
          <button class="btn btn-icon btn-sm" @click="editingVolume = null">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>

        <div class="form-grid">
          <div class="form-group">
            <label>Provisioned Size (GiB)</label>
            <input v-model.number="editVolumeForm.sizeGib" type="number" min="1" class="input mono" required />
          </div>
          <div class="form-group">
            <label>Provisioning Mode</label>
            <select :value="editVolumeForm.thinProvisioned ? 'thin' : 'thick'" class="select" @change="editVolumeForm.thinProvisioned = ($event.target as HTMLSelectElement).value === 'thin'">
              <option value="thin">Thin Provisioned (Sparse / On-demand)</option>
              <option value="thick">Thick Provisioned (Guaranteed space reservation)</option>
            </select>
          </div>
          <div class="form-group">
            <label>Sync Mode</label>
            <select v-model="editVolumeForm.sync" class="select mono">
              <option value="standard">standard - Respect application fsync / O_SYNC (Recommended)</option>
              <option value="always">always - Strict safety: commit every write synchronously</option>
              <option value="disabled">disabled - Highest throughput: ignore sync</option>
            </select>
          </div>
          <div class="form-group">
            <label>Compression</label>
            <select v-model="editVolumeForm.compression" class="select">
              <option value="lz4">lz4 (Fast & Recommended)</option>
              <option value="zstd">zstd</option>
              <option value="gzip">gzip</option>
              <option value="off">off</option>
            </select>
          </div>
        </div>

        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 14px; flex-wrap: wrap; gap: 10px">
          <div style="display: flex; gap: 16px; align-items: center">
            <label style="font-size: 12px; display: flex; align-items: center; gap: 6px; cursor: pointer">
              <input v-model="editVolumeForm.readonly" type="checkbox" /> Read-only Block Device
            </label>
            <label style="font-size: 12px; display: flex; align-items: center; gap: 6px; cursor: pointer">
              <input v-model="editVolumeForm.dedup" type="checkbox" /> Inline Deduplication
            </label>
          </div>
          <div style="display: flex; gap: 8px">
            <button class="btn btn-sm" @click="editingVolume = null">Cancel</button>
            <button class="btn btn-primary btn-sm" @click="handleSaveVolumeEdit">
              <MdiIcon :path="mdiCheck" size="14" style="margin-right: 4px" />
              Save Volume Settings
            </button>
          </div>
        </div>
      </div>

      <!-- Snapshot Inline Prompt -->
      <div v-if="snapshotModalDataset" class="action-panel" style="border-color: var(--success); display: flex; align-items: center; gap: 10px">
        <MdiIcon :path="mdiCamera" size="20" style="color: var(--success); flex-shrink: 0" />
        <span class="mono" style="font-size: 12.5px; font-weight: 600">
          {{ snapshotModalDataset.datasetPath }}@
        </span>
        <input v-model="snapshotName" class="input mono" style="flex: 1" placeholder="snap-name" />
        <button class="btn btn-success btn-sm" @click="handleTakeSnapshot">
          Take Snapshot
        </button>
        <button class="btn btn-sm" @click="snapshotModalDataset = null">Cancel</button>
      </div>

      <!-- Hierarchical Tree / Flat List Table -->
      <div class="table-wrap" style="margin-top: 16px">
        <table class="data-table hierarchy-table">
          <thead>
            <tr>
              <SortableTh :sort="hierarchySort" sort-key="name" style="min-width: 240px">Name</SortableTh>
              <SortableTh :sort="hierarchySort" sort-key="usage" style="min-width: 140px">Space Usage</SortableTh>
              <th style="min-width: 200px">Properties</th>
              <SortableTh :sort="hierarchySort" sort-key="snapshots" style="min-width: 100px">Snapshots</SortableTh>
              <th style="min-width: 140px; text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="visibleHierarchyNodes.length === 0">
              <td colspan="5" style="text-align: center; color: var(--text-secondary); padding: 32px">
                <div v-if="searchQuery">
                  No datasets or volumes match filter "<strong>{{ searchQuery }}</strong>".
                </div>
                <div v-else>
                  No datasets or volumes.
                </div>
              </td>
            </tr>

            <tr
              v-for="node in sortedHierarchyNodes"
              :key="node.id"
              class="tree-row"
              :class="{ 'is-zvol-row': node.type === 'volume' }"
            >
              <!-- Name & Hierarchy Column -->
              <td>
                <div
                  class="tree-cell"
                  :style="{ paddingLeft: viewMode === 'tree' && !searchQuery ? `${node.depth * 20}px` : '0px' }"
                >
                  <!-- Expand/Collapse Chevron -->
                  <button
                    v-if="viewMode === 'tree' && !searchQuery && node.children.length > 0"
                    class="chevron-btn"
                    @click="toggleCollapse(node.id)"
                  >
                    <MdiIcon
                      :path="collapsedPaths.has(node.id) ? mdiChevronRight : mdiChevronDown"
                      size="16"
                    />
                  </button>
                  <span v-else-if="viewMode === 'tree' && !searchQuery" class="chevron-spacer"></span>

                  <!-- Node Type Icon -->
                  <MdiIcon
                    :path="node.type === 'dataset' ? mdiDatabase : mdiHarddisk"
                    size="17"
                    :style="{ color: node.type === 'dataset' ? 'var(--accent-primary)' : '#a855f7' }"
                    style="flex-shrink: 0"
                  />

                  <!-- Node Display Name -->
                  <div class="tree-name-wrap">
                    <span class="mono tree-name">{{ node.name }}</span>
                    <span
                      class="badge type-pill"
                      :class="node.type === 'dataset' ? 'badge-info' : 'badge-purple'"
                    >
                      {{ node.type.toUpperCase() }}
                    </span>
                  </div>
                </div>
              </td>

              <!-- Space Usage & Quotas -->
              <td>
                <div v-if="node.type === 'dataset'">
                  <div class="mono" style="font-weight: 600; font-size: 12.5px">{{ formatBytes(node.usedBytes) }}</div>
                  <div style="font-size: 11px; color: var(--text-secondary); margin-top: 1px">
                    Quota: {{ node.quotaBytes && node.quotaBytes > 0 ? formatBytes(node.quotaBytes) : 'Unlimited' }}
                    <span v-if="node.reservationBytes && node.reservationBytes > 0" style="margin-left: 4px; color: #38bdf8">
                      | Res: {{ formatBytes(node.reservationBytes) }}
                    </span>
                  </div>
                </div>
                <div v-else>
                  <div class="mono" style="font-weight: 600; font-size: 12.5px">
                    {{ node.sizeBytes ? formatBytes(node.sizeBytes) : '-' }}
                  </div>
                  <div style="font-size: 11px; color: var(--text-secondary); margin-top: 1px">
                    Used: {{ formatBytes(node.usedBytes) }}
                  </div>
                </div>
              </td>

              <!-- Properties -->
              <td>
                <div style="display: flex; gap: 4px; align-items: center; flex-wrap: wrap">
                  <span class="badge badge-info mono" style="font-size: 10px">{{ node.compression }}</span>
                  <span class="badge badge-neutral mono" style="font-size: 10px">
                    {{ node.type === 'dataset' ? node.recordsize : node.volblocksize }}
                  </span>
                  <!-- Sync mode pill -->
                  <span
                    v-if="node.sync && node.sync !== 'standard'"
                    class="badge mono"
                    :class="node.sync === 'always' ? 'badge-success' : 'badge-danger'"
                    style="font-size: 10px"
                    :title="`Sync mode: ${node.sync}`"
                  >
                    sync:{{ node.sync }}
                  </span>
                  <span
                    v-else-if="node.sync === 'standard'"
                    class="badge badge-neutral mono"
                    style="font-size: 10px; opacity: 0.75"
                    title="Sync mode: standard"
                  >
                    sync:std
                  </span>
                  <!-- Provisioning pill for ZVol -->
                  <span
                    v-if="node.type === 'volume'"
                    class="badge mono"
                    :class="node.thinProvisioned ? 'badge-success' : 'badge-neutral'"
                    style="font-size: 10px"
                  >
                    {{ node.thinProvisioned ? 'Thin' : 'Thick' }}
                  </span>
                  <span v-if="node.readonly" class="badge badge-warning" style="font-size: 10px">RO</span>
                  <span v-if="node.dedup" class="badge badge-purple mono" style="font-size: 10px">Dedup</span>
                </div>
              </td>

              <!-- Snapshots -->
              <td>
                <div v-if="node.type === 'dataset'">
                  <div class="mono" style="font-size: 12px">{{ node.snapshotsCount || 0 }} snap{{ node.snapshotsCount === 1 ? '' : 's' }}</div>
                  <div v-if="node.lastSnapshot" class="mono" style="font-size: 10.5px; color: #34d399; margin-top: 2px" :title="node.lastSnapshot">
                    {{ node.lastSnapshot.split('@')[1] || node.lastSnapshot }}
                  </div>
                </div>
                <div v-else class="mono" style="font-size: 11px; color: var(--text-muted)">-</div>
              </td>

              <!-- Actions -->
              <td style="text-align: right">
                <div style="display: inline-flex; gap: 4px; justify-content: flex-end">
                  <!-- Dataset Actions -->
                  <template v-if="node.type === 'dataset' && node.rawDataset">
                    <button
                      class="btn btn-sm btn-icon"
                      title="Create child dataset under this folder"
                      @click="openCreateDataset(node.name)"
                    >
                      <MdiIcon :path="mdiPlus" size="13" />
                    </button>
                    <button
                      class="btn btn-sm btn-icon"
                      title="Edit Properties"
                      @click="startEditDataset(node.rawDataset)"
                    >
                      <MdiIcon :path="mdiPencil" size="13" />
                    </button>
                    <button
                      class="btn btn-sm btn-icon"
                      title="Take Snapshot"
                      @click="snapshotModalDataset = node.rawDataset"
                    >
                      <MdiIcon :path="mdiCamera" size="13" />
                    </button>
                    <button
                      class="btn btn-danger btn-sm btn-icon"
                      title="Delete Dataset"
                      @click="handleDeleteDataset(node.rawDataset)"
                    >
                      <MdiIcon :path="mdiDelete" size="13" />
                    </button>
                  </template>

                  <!-- Volume Actions -->
                  <template v-else-if="node.type === 'volume' && node.rawVolume">
                    <button
                      class="btn btn-sm btn-icon"
                      title="Edit ZVol Settings & Resize"
                      @click="startEditVolume(node.rawVolume)"
                    >
                      <MdiIcon :path="mdiPencil" size="13" />
                    </button>
                    <button
                      class="btn btn-danger btn-sm btn-icon"
                      title="Delete Volume"
                      @click="handleDeleteVolume(node.rawVolume)"
                    >
                      <MdiIcon :path="mdiDelete" size="13" />
                    </button>
                  </template>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Pool Cards Grid Layout */
.pools-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 380px), 1fr));
  gap: 18px;
  width: 100%;
  min-width: 0;
}

@media (max-width: 640px) {
  .pools-grid {
    grid-template-columns: 1fr;
  }
}

.pool-card {
  background: var(--bg-surface);
  border: 2px solid var(--border-subtle);
  border-radius: 12px;
  padding: 18px 20px;
  display: flex;
  flex-direction: column;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  cursor: pointer;
  position: relative;
  overflow: hidden;
  min-width: 0;
}

.pool-card:hover {
  border-color: rgba(255, 255, 255, 0.22);
  transform: translateY(-2px);
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.25);
}

.pool-card.is-active {
  border-color: var(--accent-primary);
  background: linear-gradient(180deg, rgba(56, 189, 248, 0.05) 0%, rgba(0, 0, 0, 0) 100%), var(--bg-surface);
  box-shadow: 0 0 20px rgba(56, 189, 248, 0.12), 0 4px 16px rgba(0, 0, 0, 0.3);
}

/* Card Header */
.pool-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border-subtle);
}

.pool-title-group {
  display: flex;
  align-items: center;
  gap: 12px;
}

.pool-icon-wrap {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  background: rgba(56, 189, 248, 0.1);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.pool-name {
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: -0.2px;
}

.pool-mountpoint {
  font-size: 11.5px;
  color: var(--text-secondary);
  display: flex;
  align-items: center;
  margin-top: 1px;
}

.pool-badges {
  display: flex;
  align-items: center;
  gap: 6px;
}

.layout-badge {
  font-size: 10.5px;
  padding: 3px 7px;
  font-weight: 700;
  letter-spacing: 0.5px;
}

/* Card Body */
.pool-card-body {
  display: flex;
  gap: 20px;
  align-items: center;
  padding: 16px 0;
  flex: 1;
}

@media (max-width: 500px) {
  .pool-card-body {
    flex-direction: column;
    align-items: center;
  }
}

.gauge-pane {
  flex: 0 0 140px;
  display: flex;
  justify-content: center;
  align-items: center;
}

.info-pane {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

/* Stat row */
.info-stat-row {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
  background: rgba(0, 0, 0, 0.2);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 8px 10px;
}

.stat-col {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.stat-label {
  font-size: 10px;
  font-weight: 600;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.stat-value {
  font-size: 12.5px;
  font-weight: 700;
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.text-sky {
  color: #38bdf8;
}

.text-emerald {
  color: #34d399;
}

/* Props table */
.info-props-table {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px 12px;
  font-size: 11.5px;
}

.prop-item {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.prop-item.full-width {
  grid-column: span 2;
}

.prop-key {
  color: var(--text-secondary);
  flex-shrink: 0;
}

.prop-val {
  color: var(--text-primary);
  font-weight: 600;
}

.dev-list {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 11px;
}

.scrub-text {
  font-size: 11px;
  color: #38bdf8;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Card Footer */
.pool-card-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: 12px;
  border-top: 1px solid var(--border-subtle);
}

.active-pill {
  display: inline-flex;
  align-items: center;
  font-size: 11.5px;
  font-weight: 600;
  color: var(--accent-primary);
  background: rgba(56, 189, 248, 0.12);
  padding: 3px 8px;
  border-radius: 12px;
}

.inactive-hint {
  font-size: 11px;
  color: var(--text-muted);
}

.card-action-btns {
  display: flex;
  gap: 8px;
}

.btn-primary-outline {
  border: 1px solid var(--accent-primary);
  color: var(--accent-primary);
  background: rgba(56, 189, 248, 0.08);
}

.btn-primary-outline:hover {
  background: rgba(56, 189, 248, 0.2);
}

/* Modal Overlay & Styling */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.7);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  animation: fadeIn 0.15s ease-out;
}

@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
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
  transition: all 0.15s ease;
}

.tab-btn:hover {
  color: var(--text-primary);
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

/* Pool Config Form items */
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
  color: var(--text-primary);
}

.config-hint {
  font-size: 11.5px;
  color: var(--text-secondary);
  line-height: 1.4;
}

/* VDEV Tier List in Modal */
.vdev-tier-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
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

.tier-badge {
  font-size: 10px;
  padding: 2px 6px;
  font-weight: 700;
  letter-spacing: 0.5px;
}

.badge-purple {
  background: rgba(168, 85, 247, 0.15);
  color: #c084fc;
  border: 1px solid rgba(168, 85, 247, 0.3);
}

.vdev-children {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--border-subtle);
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.vdev-child-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 8px;
  background: rgba(0, 0, 0, 0.2);
  border-radius: 6px;
}

.add-vdev-box {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 16px;
}

/* Tree / List Controls */
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
  width: 190px;
}

.view-mode-toggle {
  display: inline-flex;
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  overflow: hidden;
  background: var(--bg-base);
}

.toggle-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 5px 10px;
  font-size: 12px;
  font-weight: 600;
  background: transparent;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
}

.toggle-btn.active {
  background: var(--accent-primary);
  color: #000;
}

.tree-expand-controls {
  display: inline-flex;
  gap: 4px;
}

/* Tree row & cell layout */
.tree-row {
  transition: background 0.1s ease;
}

.tree-row:hover {
  background: rgba(255, 255, 255, 0.03);
}

.tree-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.chevron-btn {
  background: transparent;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 2px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
}

.chevron-btn:hover {
  color: var(--text-primary);
  background: rgba(255, 255, 255, 0.08);
}

.chevron-spacer {
  width: 20px;
  display: inline-block;
}

.tree-name-wrap {
  display: flex;
  align-items: center;
  gap: 6px;
}

.tree-name {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary);
}

.type-pill {
  font-size: 9.5px;
  padding: 1px 5px;
  letter-spacing: 0.5px;
  font-weight: 700;
}

.action-panel {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 14px;
  margin-top: 14px;
}

.hierarchy-table {
  min-width: 760px;
  width: 100%;
}

/* Pool Builder & Drag-and-Drop Infrastructure */
.pool-builder-card {
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.25);
  animation: fadeIn 0.2s ease-in-out;
}

.builder-workspace {
  min-height: 480px;
}

@media (max-width: 900px) {
  .builder-workspace {
    grid-template-columns: 1fr !important;
  }
}

.drive-card:hover {
  border-color: var(--accent-primary) !important;
  transform: translateY(-1px);
}

.drive-card.is-assigned {
  opacity: 0.65;
  border-style: dashed !important;
}

.vdev-builder-card {
  border: 2px dashed var(--border-strong);
  transition: all 0.2s ease;
}

.vdev-builder-card.is-dragover {
  border-color: var(--accent-primary) !important;
  background: rgba(59, 130, 246, 0.08) !important;
  box-shadow: 0 0 12px rgba(59, 130, 246, 0.2);
}

.vdev-builder-card.vdev-type-data {
  border-left: 4px solid #3b82f6;
}

.vdev-builder-card.vdev-type-cache {
  border-left: 4px solid #10b981;
}

.vdev-builder-card.vdev-type-log {
  border-left: 4px solid #f59e0b;
}

.vdev-builder-card.vdev-type-spare {
  border-left: 4px solid #6b7280;
}

.vdev-builder-card.vdev-type-special {
  border-left: 4px solid #8b5cf6;
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(-6px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
