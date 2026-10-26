<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import {
  mdiAlertCircleOutline,
  mdiCheck,
  mdiCheckCircleOutline,
  mdiClose,
  mdiCogOutline,
  mdiDatabase,
  mdiDatabaseCheck,
  mdiDatabaseExportOutline,
  mdiDelete,
  mdiDragVariant,
  mdiFlash,
  mdiHarddisk,
  mdiLayersOutline,
  mdiLockOutline,
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

function onDropDrive(vdevId: string, evt?: DragEvent) {
  const path = draggedDevicePath.value || evt?.dataTransfer?.getData('text/plain');
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
// ---------------------------------------------------------------------------
// Existing Pool Layout & Topology Designer State (Drag & Drop)
// ---------------------------------------------------------------------------
const configuringPool = ref<ZfsPool | null>(null);
const isApplyingPoolConfig = ref(false);

const poolSettingsForm = reactive({
  sync: 'standard',
  autotrim: 'off',
  compression: 'lz4',
});

const editBuilderVdevs = ref<BuilderVdev[]>([]);
const editDriveSearchQuery = ref('');

interface ExistingVdevCard {
  name: string;
  type: 'data' | 'cache' | 'log' | 'spare' | 'special';
  layout: string;
  state: string;
  devicePaths: string[];
}

const existingActiveVdevs = computed<ExistingVdevCard[]>(() => {
  if (!configuringPool.value) return [];
  const pool = configuringPool.value;
  if (pool.vdevs && pool.vdevs.length > 0) {
    return pool.vdevs.map((v) => {
      const paths: string[] = [];
      if (v.children && v.children.length > 0) {
        for (const c of v.children) {
          if (c.path) paths.push(c.path);
          else if (c.name) paths.push(c.name);
        }
      } else if (v.path) {
        paths.push(v.path);
      }
      return {
        name: v.name,
        type: (v.type as any) || 'data',
        layout: v.layout || (v.type === 'mirror' ? 'mirror' : 'stripe'),
        state: v.state || 'ONLINE',
        devicePaths: paths,
      };
    });
  }
  if (pool.devices && pool.devices.length > 0) {
    return [
      {
        name: 'data-0',
        type: 'data',
        layout: (pool.vdevLayout as any) || 'mirror',
        state: 'ONLINE',
        devicePaths: [...pool.devices],
      },
    ];
  }
  return [];
});

function openPoolConfig(pool: ZfsPool) {
  configuringPool.value = pool;
  poolSettingsForm.sync = (pool.sync as any) || 'standard';
  poolSettingsForm.autotrim = (pool.autotrim as any) || 'off';
  poolSettingsForm.compression = pool.compression || 'lz4';
  editDriveSearchQuery.value = '';
  vdevCounter++;
  if (editAvailableDisks.value.length > 0) {
    editBuilderVdevs.value = [
      {
        id: `edit-vdev-data-${Date.now()}-${vdevCounter}`,
        type: 'data',
        layout: 'mirror',
        devicePaths: [],
      },
    ];
  } else {
    editBuilderVdevs.value = [];
  }
  if (nasState.disks.length === 0) {
    refreshAllData();
  }
}

// Device paths currently belonging to the existing pool
const existingPoolDevicePaths = computed(() => {
  if (!configuringPool.value) return new Set<string>();
  const set = new Set<string>();
  if (configuringPool.value.devices) {
    for (const d of configuringPool.value.devices) set.add(d);
  }
  if (configuringPool.value.vdevs) {
    for (const v of configuringPool.value.vdevs) {
      if (v.path) set.add(v.path);
      if (v.children) {
        for (const c of v.children) {
          if (c.path) set.add(c.path);
        }
      }
    }
  }
  return set;
});

// Available drives: unassigned host disks that are not already part of this pool
const editAvailableDisks = computed(() => {
  return unassignedDisks.value.filter((d) => !existingPoolDevicePaths.value.has(d.devicePath));
});

const editBuilderAssignedPaths = computed(() => {
  return new Set(editBuilderVdevs.value.flatMap((v) => v.devicePaths));
});

const filteredEditAvailableDisks = computed(() => {
  const q = editDriveSearchQuery.value.trim().toLowerCase();
  return editAvailableDisks.value.filter((d) => {
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

function addEditBuilderVdev(type: 'data' | 'cache' | 'log' | 'spare' | 'special') {
  vdevCounter++;
  const defaultLayout = type === 'data' ? 'mirror' : type === 'cache' || type === 'spare' ? 'stripe' : 'mirror';
  editBuilderVdevs.value.push({
    id: `edit-vdev-${type}-${Date.now()}-${vdevCounter}`,
    type,
    layout: defaultLayout as any,
    devicePaths: [],
  });
}

function removeEditBuilderVdev(vdevId: string) {
  editBuilderVdevs.value = editBuilderVdevs.value.filter((v) => v.id !== vdevId);
}

function assignDriveToEditVdev(devicePath: string, vdevId: string) {
  for (const v of editBuilderVdevs.value) {
    const idx = v.devicePaths.indexOf(devicePath);
    if (idx !== -1) v.devicePaths.splice(idx, 1);
  }
  const target = editBuilderVdevs.value.find((v) => v.id === vdevId);
  if (target && !target.devicePaths.includes(devicePath)) {
    target.devicePaths.push(devicePath);
  }
}

function removeDriveFromEditVdev(vdevId: string, devicePath: string) {
  const target = editBuilderVdevs.value.find((v) => v.id === vdevId);
  if (target) {
    const idx = target.devicePaths.indexOf(devicePath);
    if (idx !== -1) target.devicePaths.splice(idx, 1);
  }
}

function onDropEditDrive(vdevId: string, evt?: DragEvent) {
  const path = draggedDevicePath.value || evt?.dataTransfer?.getData('text/plain');
  if (path) {
    assignDriveToEditVdev(path, vdevId);
  }
  hoveredVdevId.value = null;
  draggedDevicePath.value = null;
}

const editPoolCapacitySummary = computed(() => {
  if (!configuringPool.value) {
    return {
      totalRawBytes: 0,
      estimatedUsableBytes: 0,
      dataDrivesCount: 0,
      cacheDrivesCount: 0,
      logDrivesCount: 0,
      spareDrivesCount: 0,
      specialDrivesCount: 0,
    };
  }

  const pool = configuringPool.value;
  let totalRawBytes = 0;
  let estimatedUsableBytes = pool.sizeBytes || 0;
  let dataDrivesCount = 0;
  let cacheDrivesCount = 0;
  let logDrivesCount = 0;
  let spareDrivesCount = 0;
  let specialDrivesCount = 0;

  // 1. Existing Active VDEVs
  for (const ev of existingActiveVdevs.value) {
    for (const dp of ev.devicePaths) {
      const disk = getDiskByPath(dp);
      totalRawBytes += disk?.sizeBytes || (pool.devices?.length ? pool.sizeBytes / pool.devices.length : 0);
      if (ev.type === 'data') dataDrivesCount++;
      else if (ev.type === 'cache') cacheDrivesCount++;
      else if (ev.type === 'log') logDrivesCount++;
      else if (ev.type === 'spare') spareDrivesCount++;
      else if (ev.type === 'special') specialDrivesCount++;
    }
  }

  // 2. New Expansion VDEVs
  for (const vdev of editBuilderVdevs.value) {
    const driveSizes = vdev.devicePaths.map((p) => getDiskByPath(p)?.sizeBytes || 0);
    const vdevRaw = driveSizes.reduce((acc, bytes) => acc + bytes, 0);
    totalRawBytes += vdevRaw;

    if (vdev.type === 'data') {
      dataDrivesCount += vdev.devicePaths.length;
      if (vdev.devicePaths.length > 0) {
        const minDrive = Math.min(...driveSizes);
        const count = vdev.devicePaths.length;
        if (vdev.layout === 'stripe') {
          estimatedUsableBytes += vdevRaw;
        } else if (vdev.layout === 'mirror') {
          estimatedUsableBytes += minDrive;
        } else if (vdev.layout === 'raidz1' && count >= 2) {
          estimatedUsableBytes += minDrive * (count - 1);
        } else if (vdev.layout === 'raidz2' && count >= 3) {
          estimatedUsableBytes += minDrive * (count - 2);
        } else if (vdev.layout === 'raidz3' && count >= 4) {
          estimatedUsableBytes += minDrive * (count - 3);
        }
      }
    } else if (vdev.type === 'cache') {
      cacheDrivesCount += vdev.devicePaths.length;
    } else if (vdev.type === 'log') {
      logDrivesCount += vdev.devicePaths.length;
    } else if (vdev.type === 'spare') {
      spareDrivesCount += vdev.devicePaths.length;
    } else if (vdev.type === 'special') {
      specialDrivesCount += vdev.devicePaths.length;
    }
  }

  return {
    totalRawBytes,
    estimatedUsableBytes,
    dataDrivesCount,
    cacheDrivesCount,
    logDrivesCount,
    spareDrivesCount,
    specialDrivesCount,
  };
});

const isEditPoolBuilderValid = computed(() => {
  if (!configuringPool.value) return false;
  const pool = configuringPool.value;
  const hasSettingsChanges =
    poolSettingsForm.sync !== ((pool.sync as any) || 'standard') ||
    poolSettingsForm.autotrim !== ((pool.autotrim as any) || 'off') ||
    poolSettingsForm.compression !== (pool.compression || 'lz4');

  const newVdevsWithDisks = editBuilderVdevs.value.filter((v) => v.devicePaths.length > 0);
  if (newVdevsWithDisks.length > 0) {
    return newVdevsWithDisks.every((v) => getVdevValidation(v).valid);
  }

  return hasSettingsChanges;
});

async function handleApplyPoolConfig() {
  if (!configuringPool.value) return;
  const pool = configuringPool.value;
  const poolName = pool.name;
  isApplyingPoolConfig.value = true;

  try {
    const hasSettingsChanges =
      poolSettingsForm.sync !== ((pool.sync as any) || 'standard') ||
      poolSettingsForm.autotrim !== ((pool.autotrim as any) || 'off') ||
      poolSettingsForm.compression !== (pool.compression || 'lz4');

    if (hasSettingsChanges) {
      await services.zfs.updatePool(poolName, {
        sync: poolSettingsForm.sync,
        autotrim: poolSettingsForm.autotrim,
        compression: poolSettingsForm.compression,
      });
    }

    const newVdevs = editBuilderVdevs.value.filter((v) => v.devicePaths.length > 0);
    for (const v of newVdevs) {
      const val = getVdevValidation(v);
      if (!val.valid) {
        notify('error', `VDEV ${v.type.toUpperCase()}: ${val.message}`);
        isApplyingPoolConfig.value = false;
        return;
      }
      await services.zfs.addVdev(poolName, {
        type: v.type,
        layout: v.layout,
        devices: v.devicePaths,
      });
    }

    notify(
      'success',
      newVdevs.length > 0
        ? `Successfully expanded pool "${pool.poolName || pool.name}" with ${newVdevs.length} new VDEV tier(s)`
        : `Updated settings for pool "${pool.poolName || pool.name}"`
    );
    configuringPool.value = null;
    await refreshAllData();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update pool layout');
  } finally {
    isApplyingPoolConfig.value = false;
  }
}

async function handleRemoveExistingDevice(devicePath: string) {
  if (!configuringPool.value) return;
  const confirmed = await confirmModal({
    title: 'Detach Device from Pool',
    tone: 'warning',
    message: `Remove device "${devicePath}" from pool "${configuringPool.value.poolName || configuringPool.value.name}"?`,
    details: 'The device will be safely removed from this VDEV tier and returned to the unassigned drive bank.',
    confirmText: 'Remove Device',
  });
  if (!confirmed) return;
  try {
    const updated = await services.zfs.removeVdev(configuringPool.value.name, devicePath);
    configuringPool.value = updated;
    const idx = nasState.pools.findIndex((p) => p.name === updated.name);
    if (idx !== -1) nasState.pools[idx] = updated;
    notify('info', `Removed device ${devicePath} from pool`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', err?.message || `Failed to remove device ${devicePath}`);
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

    <!-- Pools Grid Cards -->
    <div
      v-if="nasState.pools.length === 0"
      class="card"
      style="padding: 48px 24px; text-align: center; color: var(--text-secondary)"
    >
      <MdiIcon :path="mdiDatabase" size="44" style="opacity: 0.35; margin-bottom: 12px; color: var(--accent-primary)" />
      <div style="font-size: 16px; font-weight: 600; color: var(--text-primary)">No Storage Pools Found</div>
      <p style="margin: 6px auto 18px; font-size: 13px; max-width: 480px; line-height: 1.5">
        No OpenZFS storage pools are currently imported on the host. Click <strong>"New Pool"</strong> to design and create a storage pool.
      </p>
      <button class="btn btn-primary btn-sm" @click="openCreatePoolModal">
        <MdiIcon :path="mdiPlus" size="16" style="margin-right: 4px" />
        New Pool
      </button>
    </div>

    <div v-else class="pools-grid">
      <div
        v-for="p in nasState.pools"
        :key="p.name"
        class="card pool-card"
        :class="{ 'is-main-pool': (p.poolName || p.name.replace(/^zfsPools\//, '')) === nasState.systemConfig?.mainPool }"
      >
        <!-- Card Top Bar: Icon, Pool Name, Main Pool Badge, Health Badge -->
        <div class="pool-card-header">
          <div style="display: flex; align-items: center; gap: 10px; min-width: 0">
            <MdiIcon
              :path="(p.poolName || p.name.replace(/^zfsPools\//, '')) === nasState.systemConfig?.mainPool ? mdiDatabaseCheck : mdiDatabase"
              size="24"
              :style="{ color: (p.poolName || p.name.replace(/^zfsPools\//, '')) === nasState.systemConfig?.mainPool ? 'var(--accent-primary)' : 'var(--text-secondary)' }"
            />
            <div style="min-width: 0">
              <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap">
                <span class="mono" style="font-weight: 700; font-size: 16px; color: var(--text-primary)">
                  {{ p.poolName || p.name.replace(/^zfsPools\//, '') }}
                </span>
                <span
                  v-if="(p.poolName || p.name.replace(/^zfsPools\//, '')) === nasState.systemConfig?.mainPool"
                  class="badge badge-main-pool"
                  title="Designated Main Storage Pool"
                >
                  <MdiIcon :path="mdiDatabaseCheck" size="12" style="margin-right: 3px" />
                  MAIN POOL
                </span>
              </div>
              <div class="mono" style="font-size: 11.5px; color: var(--text-muted); margin-top: 2px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
                {{ p.mountpoint || `/nas/${p.poolName || p.name.replace(/^zfsPools\//, '')}` }}
              </div>
            </div>
          </div>

          <div style="display: flex; align-items: center; gap: 6px; flex-shrink: 0">
            <span
              class="badge mono"
              :class="p.state === 'ONLINE' ? 'badge-success' : 'badge-danger'"
              style="font-size: 11px; font-weight: 600"
            >
              {{ p.state }}
            </span>
          </div>
        </div>

        <!-- Layout & Devices Info Tag -->
        <div style="display: flex; align-items: center; justify-content: space-between; font-size: 11.5px; margin-top: 14px; padding: 6px 10px; background: var(--bg-tertiary); border-radius: 6px; border: 1px solid var(--border-subtle)">
          <div style="display: flex; align-items: center; gap: 6px">
            <span style="color: var(--text-muted)">Topology:</span>
            <span class="badge badge-info mono" style="font-size: 10px; padding: 1px 6px">
              {{ (p.vdevLayout || 'stripe').toUpperCase() }}
            </span>
          </div>
          <div
            v-if="p.devices && p.devices.length > 0"
            class="mono"
            style="color: var(--text-secondary); font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 200px"
            :title="p.devices.join(', ')"
          >
            {{ p.devices.length }} dev{{ p.devices.length === 1 ? '' : 's' }} ({{ p.devices.map(d => d.replace(/^\/dev\//, '')).join(', ') }})
          </div>
        </div>

        <!-- Capacity Section -->
        <div style="display: flex; flex-direction: column; gap: 6px; margin-top: 14px">
          <div style="display: flex; justify-content: space-between; align-items: baseline; font-size: 12px">
            <div>
              <span class="mono" style="font-weight: 700; color: var(--text-primary)">
                {{ formatBytes(p.allocatedBytes) }}
              </span>
              <span style="color: var(--text-muted); font-size: 11px; margin: 0 4px">used of</span>
              <span class="mono" style="color: var(--text-secondary)">
                {{ formatBytes(p.sizeBytes) }}
              </span>
            </div>
            <span
              class="mono"
              :style="{
                color: p.capacityPercent > 85 ? 'var(--danger)' : p.capacityPercent > 70 ? 'var(--warning)' : 'var(--accent-primary)',
                fontWeight: 700,
                fontSize: '13px'
              }"
            >
              {{ p.capacityPercent }}%
            </span>
          </div>

          <!-- Progress Bar -->
          <div class="capacity-track">
            <div
              class="capacity-fill"
              :style="{
                width: `${Math.min(100, p.capacityPercent)}%`,
                background: p.capacityPercent > 85 ? 'var(--danger)' : p.capacityPercent > 70 ? 'var(--warning)' : 'var(--accent-primary)'
              }"
            ></div>
          </div>

          <div style="display: flex; justify-content: space-between; font-size: 11px; color: var(--text-muted)">
            <span>Free: <strong class="mono" style="color: var(--text-secondary)">{{ formatBytes(p.freeBytes) }}</strong></span>
            <span>Frag: <strong class="mono" style="color: #c084fc">{{ p.fragmentationPercent }}%</strong></span>
          </div>
        </div>

        <!-- Key Metrics Grid -->
        <div class="pool-metrics-grid">
          <div class="metric-box">
            <div class="metric-label">Sync</div>
            <div class="metric-val mono">{{ p.sync || 'standard' }}</div>
          </div>
          <div class="metric-box">
            <div class="metric-label">Dedup</div>
            <div class="metric-val mono">{{ p.dedupRatio > 1 ? `${p.dedupRatio.toFixed(2)}x` : '1.00x' }}</div>
          </div>
          <div class="metric-box" style="grid-column: span 2">
            <div class="metric-label">Scrub</div>
            <div class="metric-val mono" style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap" :title="p.lastScrubStatus || 'Never scrubbed'">
              {{ p.lastScrubStatus || 'Never scrubbed' }}
            </div>
          </div>
        </div>

        <!-- Card Footer / Action Buttons -->
        <div class="pool-card-footer">
          <div style="display: flex; align-items: center; gap: 8px">
            <button
              class="btn btn-sm btn-primary"
              title="View Child Datasets in this pool"
              @click="viewPoolDatasets(p.name)"
            >
              <MdiIcon :path="mdiTreeOutline" size="14" style="margin-right: 4px" />
              Datasets
            </button>
            <button
              class="btn btn-sm"
              title="Scrub pool integrity"
              @click="handleScrubPool(p.name)"
            >
              <MdiIcon :path="mdiShieldCheckOutline" size="14" style="margin-right: 4px" />
              Scrub
            </button>
          </div>

          <div style="display: flex; align-items: center; gap: 6px">
            <button
              v-if="(p.poolName || p.name.replace(/^zfsPools\//, '')) !== nasState.systemConfig?.mainPool"
              class="btn btn-sm btn-ghost"
              title="Designate as Main Storage Pool"
              style="padding: 4px 8px; font-size: 11px"
              @click="handleSetMainPool(p.poolName || p.name.replace(/^zfsPools\//, ''))"
            >
              <MdiIcon :path="mdiDatabaseCheck" size="13" style="margin-right: 3px; color: var(--accent-primary)" />
              Set Main
            </button>
            <button
              class="btn btn-sm btn-icon"
              title="Configure Sync & VDEVs"
              @click="openPoolConfig(p)"
            >
              <MdiIcon :path="mdiCogOutline" size="14" />
            </button>
            <button
              class="btn btn-sm btn-icon"
              title="Export pool (safely unmount and detach)"
              @click="handleExportPool(p.name)"
            >
              <MdiIcon :path="mdiDatabaseExportOutline" size="14" />
            </button>
            <button
              class="btn btn-danger btn-sm btn-icon"
              title="Destroy Pool"
              @click="handleDeletePool(p.name)"
            >
              <MdiIcon :path="mdiDelete" size="14" />
            </button>
          </div>
        </div>
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
                  @drop="onDropDrive(vdev.id, $event)"
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

    <!-- Advanced Drag-and-Drop ZFS Storage Pool Layout & Infrastructure Designer Modal (Edit) -->
    <div v-if="configuringPool" class="modal-overlay" @click.self="configuringPool = null">
      <div class="modal-container builder-modal-container">
        <!-- Modal Header -->
        <div class="modal-header">
          <div>
            <div style="display: flex; align-items: center; gap: 10px">
              <MdiIcon :path="mdiLayersOutline" size="22" style="color: var(--accent-primary)" />
              <h3 class="card-title" style="margin: 0; font-size: 18px">
                ZFS Storage Pool Builder & Infrastructure Designer:
                <span class="mono" style="color: var(--accent-primary)">
                  {{ configuringPool.poolName || configuringPool.name.replace(/^zfsPools\//, '') }}
                </span>
              </h3>
            </div>
            <p class="card-subtitle" style="margin: 4px 0 0; font-size: 12.5px">
              Drag unassigned host drives directly into target VDEVs. Configure Data, Cache (L2ARC), Log (SLOG), and Spare tiers with enterprise redundancy.
            </p>
          </div>
          <button class="btn btn-sm btn-icon" @click="configuringPool = null">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>

        <!-- Modal Body (Scrollable Workspace) -->
        <div class="modal-body" style="padding: 20px; display: flex; flex-direction: column; gap: 18px">
          <!-- Top Settings Grid -->
          <div class="form-grid" style="grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 14px">
            <div class="form-group">
              <label>Pool Name (ID)</label>
              <input
                :value="configuringPool.poolName || configuringPool.name.replace(/^zfsPools\//, '')"
                class="input mono"
                disabled
                style="background: rgba(0, 0, 0, 0.25); color: var(--text-secondary); cursor: not-allowed"
              />
            </div>

            <div class="form-group">
              <label>Mountpoint</label>
              <input
                :value="configuringPool.mountpoint || `/nas/${configuringPool.poolName || configuringPool.name.replace(/^zfsPools\//, '')}`"
                class="input mono"
                disabled
                style="background: rgba(0, 0, 0, 0.25); color: var(--text-secondary); cursor: not-allowed"
              />
            </div>

            <div class="form-group">
              <label>Default Compression</label>
              <select v-model="poolSettingsForm.compression" class="select mono">
                <option value="lz4">lz4 (Fast, Recommended)</option>
                <option value="zstd">zstd (Higher ratio)</option>
                <option value="gzip">gzip (Legacy)</option>
                <option value="off">off</option>
              </select>
            </div>

            <div class="form-group">
              <label>Sync Policy</label>
              <select v-model="poolSettingsForm.sync" class="select mono">
                <option value="standard">standard (POSIX fsync)</option>
                <option value="always">always (Force SLOG/ZIL)</option>
                <option value="disabled">disabled (Highest throughput)</option>
              </select>
            </div>

            <div class="form-group">
              <label>Autotrim (SSD)</label>
              <select v-model="poolSettingsForm.autotrim" class="select mono">
                <option value="on">on (Automatic TRIM)</option>
                <option value="off">off (Disabled)</option>
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
                  {{ filteredEditAvailableDisks.length }} unattached
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
                  v-model="editDriveSearchQuery"
                  class="input"
                  placeholder="Filter device, model, serial..."
                  style="padding-left: 28px; font-size: 12px; height: 32px"
                />
              </div>

              <!-- Drive Cards List -->
              <div class="drives-list">
                <div
                  v-if="filteredEditAvailableDisks.length === 0"
                  style="text-align: center; padding: 28px 12px; color: var(--text-muted); font-size: 12px"
                >
                  <MdiIcon :path="mdiAlertCircleOutline" size="24" style="margin-bottom: 6px; opacity: 0.6" /><br />
                  No unassigned drives available on host. All host drives are currently attached to pools.
                </div>

                <div
                  v-for="d in filteredEditAvailableDisks"
                  :key="d.devicePath"
                  class="drive-card"
                  :class="{ 'is-assigned': editBuilderAssignedPaths.has(d.devicePath) }"
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

                    <span v-if="editBuilderAssignedPaths.has(d.devicePath)" class="badge badge-success" style="font-size: 9.5px">
                      In VDEV
                    </span>
                  </div>

                  <!-- Quick Assign Buttons for Direct Click Placement -->
                  <div
                    v-if="!editBuilderAssignedPaths.has(d.devicePath) && editBuilderVdevs.length > 0"
                    style="margin-top: 8px; border-top: 1px dashed var(--border-subtle); padding-top: 6px; display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 4px"
                  >
                    <button
                      v-for="(v, vIdx) in editBuilderVdevs"
                      :key="v.id"
                      type="button"
                      class="btn btn-xs"
                      style="font-size: 9.5px; padding: 2px 6px"
                      :title="`Assign to ${v.type.toUpperCase()}`"
                      @click="assignDriveToEditVdev(d.devicePath, v.id)"
                    >
                      + {{ v.type.toUpperCase() }} {{ existingActiveVdevs.filter(e => e.type === v.type).length + vIdx + 1 }}
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
                  <button type="button" class="btn btn-xs btn-primary-outline" @click="addEditBuilderVdev('data')">
                    <MdiIcon :path="mdiPlus" size="12" /> Data VDEV
                  </button>
                  <button type="button" class="btn btn-xs" style="background: rgba(168, 85, 247, 0.15); color: #c084fc; border: 1px solid rgba(168, 85, 247, 0.3)" @click="addEditBuilderVdev('cache')">
                    <MdiIcon :path="mdiFlash" size="12" /> + Cache (L2ARC)
                  </button>
                  <button type="button" class="btn btn-xs" style="background: rgba(234, 179, 8, 0.15); color: #facc15; border: 1px solid rgba(234, 179, 8, 0.3)" @click="addEditBuilderVdev('log')">
                    <MdiIcon :path="mdiPlus" size="12" /> + Log (SLOG)
                  </button>
                  <button type="button" class="btn btn-xs btn-neutral" @click="addEditBuilderVdev('spare')">
                    <MdiIcon :path="mdiPlus" size="12" /> + Hot Spare
                  </button>
                  <button type="button" class="btn btn-xs" style="background: rgba(16, 185, 129, 0.15); color: #34d399; border: 1px solid rgba(16, 185, 129, 0.3)" @click="addEditBuilderVdev('special')">
                    <MdiIcon :path="mdiPlus" size="12" /> + Special
                  </button>
                </div>
              </div>

              <!-- List of VDEVs with Drop Zones -->
              <div style="display: flex; flex-direction: column; gap: 12px">
                <!-- 1. Existing Active VDEVs -->
                <div
                  v-for="(vdev, evIdx) in existingActiveVdevs"
                  :key="vdev.name"
                  class="vdev-builder-card"
                  :class="{
                    'is-data': vdev.type === 'data',
                    'is-cache': vdev.type === 'cache',
                    'is-log': vdev.type === 'log',
                    'is-spare': vdev.type === 'spare',
                    'is-special': vdev.type === 'special',
                  }"
                >
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
                        {{ vdev.type.toUpperCase() }} TIER #{{ evIdx + 1 }}
                      </span>
                      <span class="badge badge-success mono" style="font-size: 10px">ACTIVE</span>
                      <span class="mono" style="font-size: 11px; color: var(--text-secondary)">
                        Layout: <strong class="mono" style="color: var(--text-primary)">{{ (vdev.layout || 'stripe').toUpperCase() }}</strong>
                      </span>
                    </div>

                    <div style="display: flex; align-items: center; gap: 8px">
                      <span class="mono" style="font-size: 11px; color: var(--text-secondary)">
                        {{ vdev.devicePaths.length }} drive{{ vdev.devicePaths.length === 1 ? '' : 's' }}
                      </span>
                    </div>
                  </div>

                  <!-- Existing Devices Chips -->
                  <div class="vdev-drop-target has-drives">
                    <div style="display: flex; flex-wrap: wrap; gap: 8px; width: 100%">
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
                          v-if="['cache', 'log', 'spare'].includes(vdev.type)"
                          type="button"
                          class="chip-remove-btn"
                          title="Detach device from pool"
                          @click.stop="handleRemoveExistingDevice(dp)"
                        >
                          <MdiIcon :path="mdiClose" size="12" />
                        </button>
                      </div>
                    </div>
                  </div>

                  <!-- Validation Feedback Alert -->
                  <div class="vdev-validation-bar ok">
                    <MdiIcon :path="mdiCheckCircleOutline" size="14" />
                    <span>Active operational pool tier ({{ vdev.state || 'ONLINE' }})</span>
                  </div>
                </div>

                <!-- 2. Staged Expansion VDEVs -->
                <div
                  v-for="(vdev, vIdx) in editBuilderVdevs"
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
                  @drop="onDropEditDrive(vdev.id, $event)"
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
                        NEW {{ vdev.type.toUpperCase() }} TIER #{{ existingActiveVdevs.filter(e => e.type === vdev.type).length + vIdx + 1 }}
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
                        type="button"
                        class="btn btn-xs btn-icon btn-danger"
                        title="Remove this expansion VDEV"
                        @click="removeEditBuilderVdev(vdev.id)"
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
                          @click.stop="removeDriveFromEditVdev(vdev.id, dp)"
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
                      {{ formatBytes(editPoolCapacitySummary.totalRawBytes) }}
                    </div>
                  </div>
                  <div>
                    <div style="font-size: 11px; color: var(--text-secondary)">Estimated Usable Space</div>
                    <div class="mono" style="font-weight: 700; font-size: 15px; color: var(--accent-primary)">
                      {{ formatBytes(editPoolCapacitySummary.estimatedUsableBytes) }}
                    </div>
                  </div>
                  <div>
                    <div style="font-size: 11px; color: var(--text-secondary)">Drive Tiers</div>
                    <div style="font-size: 12px; font-weight: 600; color: var(--text-primary)">
                      {{ editPoolCapacitySummary.dataDrivesCount }} Data · {{ editPoolCapacitySummary.cacheDrivesCount }} Cache · {{ editPoolCapacitySummary.logDrivesCount }} Log · {{ editPoolCapacitySummary.spareDrivesCount }} Spare
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Modal Footer -->
        <div class="modal-footer">
          <button type="button" class="btn btn-sm" @click="configuringPool = null">
            Cancel
          </button>
          <button
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="!isEditPoolBuilderValid || isApplyingPoolConfig"
            @click="handleApplyPoolConfig"
          >
            <MdiIcon v-if="isApplyingPoolConfig" :path="mdiRefresh" size="14" class="spin" style="margin-right: 4px" />
            <MdiIcon v-else :path="mdiCheck" size="16" style="margin-right: 4px" />
            {{ isApplyingPoolConfig ? 'Applying Changes...' : 'Apply Layout & Save' }}
          </button>
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

.pools-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 18px;
  width: 100%;
}

.pool-card {
  padding: 18px 20px;
  display: flex;
  flex-direction: column;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
  position: relative;
  background: var(--bg-card);
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
}

.pool-card:hover {
  border-color: rgba(6, 182, 212, 0.4);
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.2);
}

.pool-card.is-main-pool {
  border-color: rgba(6, 182, 212, 0.45);
  box-shadow: 0 0 14px rgba(6, 182, 212, 0.08);
}

.pool-card-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
}

.capacity-track {
  width: 100%;
  height: 6px;
  background: rgba(0, 0, 0, 0.35);
  border-radius: 3px;
  overflow: hidden;
  border: 1px solid var(--border-subtle);
}

.capacity-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 0.3s ease;
}

.pool-metrics-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 8px;
  margin-top: 14px;
}

.metric-box {
  background: var(--bg-tertiary);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 6px 10px;
}

.metric-label {
  font-size: 10px;
  text-transform: uppercase;
  color: var(--text-muted);
  letter-spacing: 0.04em;
  font-weight: 600;
}

.metric-val {
  font-size: 11.5px;
  color: var(--text-primary);
  margin-top: 2px;
}

.pool-card-footer {
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
}

.badge-purple {
  background: rgba(168, 85, 247, 0.15);
  color: #c084fc;
  border: 1px solid rgba(168, 85, 247, 0.3);
}

.badge-main-pool {
  background: rgba(6, 182, 212, 0.12);
  color: var(--accent-primary);
  border: 1px solid rgba(6, 182, 212, 0.35);
  font-weight: 700;
  letter-spacing: 0.03em;
  font-size: 10px;
  padding: 2px 7px;
  border-radius: 4px;
  display: inline-flex;
  align-items: center;
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
