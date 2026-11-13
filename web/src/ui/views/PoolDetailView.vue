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
  mdiDragVariant,
  mdiMagnify,
  mdiFlash,
  mdiCheckCircleOutline,
  mdiAlertCircleOutline,
  mdiCheck,
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
  type Disk,
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
import { confirmDelete, confirmModal } from '../modal';
import { computeStorageBlastRadius } from '../utils/storageBlastRadius';
import DatasetsView from './DatasetsView.vue';
import VolumesView from './VolumesView.vue';

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
      await services.system.selectMainPool(cleanName.value, true);
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
  const blast = computeStorageBlastRadius(cleanName.value);
  const message = isMain
    ? 'WARNING: This is the MAIN POOL. Exporting it will pause Apps and Containers!'
    : blast.hasDependencies
    ? `The pool will be unmounted and safely detached.\n\nACTIVE DEPENDENCIES DETECTED:\n${blast.summaryWarning}`
    : 'The pool will be unmounted and safely detached. You can re-import it anytime.';
  const confirmed = await confirmModal({
    title: `Export ZFS Pool "${cleanName.value}"`,
    message,
    tone: isMain ? 'danger' : 'warning',
    confirmText: 'Export Pool',
  });
  if (!confirmed) return;

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
  const blast = computeStorageBlastRadius(cleanName.value);
  const baseDetails = 'PERMANENT DATA LOSS: All datasets, snapshots, and volumes in this pool will be deleted permanently.';
  const details = blast.hasDependencies
    ? `${baseDetails}\n\nACTIVE DEPENDENCIES DETECTED:\n${blast.summaryWarning}`
    : baseDetails;
  const confirmed = await confirmDelete({
    kind: 'Destroy Pool',
    name: cleanName.value,
    details,
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
// Drag-and-Drop VDEV Infrastructure Designer State (Pool Entity Level)
// -----------------------------------------------------------------------------
interface BuilderVdev {
  id: string;
  type: 'data' | 'cache' | 'log' | 'spare' | 'special';
  layout: 'stripe' | 'mirror' | 'raidz1' | 'raidz2' | 'raidz3';
  devicePaths: string[];
}

interface ExistingVdevCard {
  name: string;
  type: 'data' | 'cache' | 'log' | 'spare' | 'special';
  layout: string;
  state: string;
  devicePaths: string[];
}

const editBuilderVdevs = ref<BuilderVdev[]>([]);
const editDriveSearchQuery = ref('');
const draggedDevicePath = ref<string | null>(null);
const hoveredVdevId = ref<string | null>(null);
let vdevCounter = 1;

// Active VDEVs currently in this pool
const existingActiveVdevs = computed<ExistingVdevCard[]>(() => {
  if (!pool.value) return [];
  const p = pool.value;
  if (p.vdevs && p.vdevs.length > 0) {
    return p.vdevs.map((v) => {
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
  if (p.devices && p.devices.length > 0) {
    return [
      {
        name: 'data-0',
        type: 'data',
        layout: (p.vdevLayout as any) || 'mirror',
        state: 'ONLINE',
        devicePaths: [...p.devices],
      },
    ];
  }
  return [];
});

const existingPoolDevicePaths = computed(() => {
  if (!pool.value) return new Set<string>();
  const set = new Set<string>();
  if (pool.value.devices) {
    for (const d of pool.value.devices) set.add(d);
  }
  if (pool.value.vdevs) {
    for (const v of pool.value.vdevs) {
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

const unassignedDisks = computed(() => {
  return nasState.disks.filter(
    (d) =>
      !d.owningZfsPool &&
      !d.isSystem &&
      !d.diskId.startsWith('zd') &&
      !d.devicePath.startsWith('/dev/zd')
  );
});

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

function getDiskByPath(path: string): Disk | undefined {
  return nasState.disks.find((d) => d.devicePath === path || d.diskId === path);
}

function appendDriveMixWarnings(
  vdev: BuilderVdev,
  base: { valid: boolean; message: string; state: 'ok' | 'warn' | 'info' }
): { valid: boolean; message: string; state: 'ok' | 'warn' | 'info' } {
  if (!base.valid || vdev.devicePaths.length < 2) return base;
  const disks = vdev.devicePaths.map((p) => getDiskByPath(p)).filter(Boolean);
  if (disks.length < 2) return base;

  const warnings: string[] = [];
  const hasRotational = disks.some((d) => Boolean(d?.rotational));
  const hasSolidState = disks.some((d) => d && !d.rotational);
  if (hasRotational && hasSolidState) {
    warnings.push('Mixed HDD and SSD/NVMe drives in one VDEV degrades latency to HDD speed');
  }

  if (vdev.layout !== 'stripe') {
    const sizes = disks.map((d) => d?.sizeBytes || 0).filter((s) => s > 0);
    if (sizes.length >= 2) {
      const minS = Math.min(...sizes);
      const maxS = Math.max(...sizes);
      if (maxS > minS * 1.1) {
        warnings.push(`Mismatched drive sizes (${formatBytes(minS)} vs ${formatBytes(maxS)}): usable capacity capped by smallest drive`);
      }
    }
  }

  if (warnings.length === 0) return base;
  return {
    valid: base.valid,
    state: 'warn',
    message: `${base.message} ⚠ ${warnings.join(' · ')}`,
  };
}

function getVdevValidation(vdev: BuilderVdev): { valid: boolean; message: string; state: 'ok' | 'warn' | 'info' } {
  const count = vdev.devicePaths.length;
  if (count === 0) {
    return { valid: false, message: 'No drives assigned yet. Drag unassigned drives here.', state: 'info' };
  }
  if (vdev.type === 'data') {
    if (vdev.layout === 'mirror') {
      if (count < 2) return { valid: false, message: 'Mirror requires at least 2 drives (currently 1).', state: 'warn' };
      return appendDriveMixWarnings(vdev, { valid: true, message: `✓ Valid ${count}-way Mirror. Tolerates ${count - 1} drive failure(s).`, state: 'ok' });
    }
    if (vdev.layout === 'raidz1') {
      if (count < 3) return { valid: false, message: `RAIDZ1 requires at least 3 drives (currently ${count}).`, state: 'warn' };
      return appendDriveMixWarnings(vdev, { valid: true, message: `✓ Valid RAIDZ1 (${count} drives, 1 parity disk).`, state: 'ok' });
    }
    if (vdev.layout === 'raidz2') {
      if (count < 4) return { valid: false, message: `RAIDZ2 requires at least 4 drives (currently ${count}).`, state: 'warn' };
      return appendDriveMixWarnings(vdev, { valid: true, message: `✓ Valid RAIDZ2 (${count} drives, 2 parity disks).`, state: 'ok' });
    }
    if (vdev.layout === 'raidz3') {
      if (count < 5) return { valid: false, message: `RAIDZ3 requires at least 5 drives (currently ${count}).`, state: 'warn' };
      return appendDriveMixWarnings(vdev, { valid: true, message: `✓ Valid RAIDZ3 (${count} drives, 3 parity disks).`, state: 'ok' });
    }
    return appendDriveMixWarnings(vdev, { valid: true, message: `⚠ Stripe layout (${count} concatenated drive(s), no parity — single drive failure destroys pool).`, state: 'warn' });
  }
  if (vdev.type === 'log') {
    if (vdev.layout === 'mirror') {
      if (count < 2) return { valid: false, message: 'Mirrored SLOG requires at least 2 drives.', state: 'warn' };
      return appendDriveMixWarnings(vdev, { valid: true, message: `✓ Mirrored SLOG write accelerator (${count} drives).`, state: 'ok' });
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
      return { valid: false, message: 'Special metadata vdev strongly recommends at least 2 drives in mirror.', state: 'warn' };
    }
    if (vdev.layout === 'stripe') {
      return { valid: true, message: `⚠ Unmirrored Special Metadata VDEV (${count} drive(s)): failure of a Special VDEV destroys the entire pool!`, state: 'warn' };
    }
    return appendDriveMixWarnings(vdev, { valid: true, message: `✓ Special Metadata / Small-Blocks VDEV (${count} drive(s)).`, state: 'ok' });
  }
  return { valid: true, message: 'Custom configuration', state: 'ok' };
}

function onDragStartDrive(ev: DragEvent, devicePath: string) {
  draggedDevicePath.value = devicePath;
  if (ev.dataTransfer) {
    ev.dataTransfer.setData('text/plain', devicePath);
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

function onDropEditDrive(vdevId: string, evt?: DragEvent) {
  const path = draggedDevicePath.value || evt?.dataTransfer?.getData('text/plain');
  if (path) {
    assignDriveToEditVdev(path, vdevId);
  }
  hoveredVdevId.value = null;
  draggedDevicePath.value = null;
}

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

const editPoolCapacitySummary = computed(() => {
  if (!pool.value) {
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
  const currentPool = pool.value;
  let totalRawBytes = 0;
  let estimatedUsableBytes = currentPool.sizeBytes || (currentPool.freeBytes + currentPool.allocatedBytes) || 0;
  let dataDrivesCount = 0;
  let cacheDrivesCount = 0;
  let logDrivesCount = 0;
  let spareDrivesCount = 0;
  let specialDrivesCount = 0;

  for (const ev of existingActiveVdevs.value) {
    for (const dp of ev.devicePaths) {
      const disk = getDiskByPath(dp);
      totalRawBytes += disk?.sizeBytes || (currentPool.devices?.length ? currentPool.sizeBytes / currentPool.devices.length : 0);
      if (ev.type === 'data') dataDrivesCount++;
      else if (ev.type === 'cache') cacheDrivesCount++;
      else if (ev.type === 'log') logDrivesCount++;
      else if (ev.type === 'spare') spareDrivesCount++;
      else if (ev.type === 'special') specialDrivesCount++;
    }
  }

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
        } else if (vdev.layout === 'raidz1' && count >= 3) {
          estimatedUsableBytes += minDrive * (count - 1);
        } else if (vdev.layout === 'raidz2' && count >= 4) {
          estimatedUsableBytes += minDrive * (count - 2);
        } else if (vdev.layout === 'raidz3' && count >= 5) {
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
  if (!pool.value) return false;
  const p = pool.value;
  const hasSettingsChanges =
    configForm.sync !== ((p.sync as any) || 'standard') ||
    configForm.autotrim !== ((p.autotrim as any) || 'off') ||
    configForm.compression !== (p.compression || 'lz4');

  const newVdevsWithDisks = editBuilderVdevs.value.filter((v) => v.devicePaths.length > 0);
  if (newVdevsWithDisks.length > 0) {
    return newVdevsWithDisks.every((v) => getVdevValidation(v).valid);
  }

  return hasSettingsChanges;
});

async function handleRemoveExistingDevice(devicePath: string) {
  if (!pool.value) return;
  const confirmed = await confirmModal({
    title: 'Remove Device from Pool',
    tone: 'danger',
    message: `Remove device "${devicePath}" from pool "${cleanName.value}"?`,
    details: 'This device will be detached from the pool. Ensure data redundancy is preserved before detaching.',
    confirmText: 'Remove Device',
  });
  if (!confirmed) return;

  isBusy.value = true;
  try {
    await services.zfs.removeVdev(pool.value.name, devicePath);
    notify('success', `Removed device "${devicePath}" from pool "${cleanName.value}"`);
    await refreshAllData();
    await refreshZfsSubresources(cleanName.value);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to remove device');
  } finally {
    isBusy.value = false;
  }
}

async function handleApplyPoolConfig() {
  if (!pool.value || isBusy.value) return;
  const p = pool.value;
  const newVdevs = editBuilderVdevs.value.filter((v) => v.devicePaths.length > 0);
  for (const v of newVdevs) {
    const val = getVdevValidation(v);
    if (!val.valid) {
      notify('error', `VDEV ${v.type.toUpperCase()}: ${val.message}`);
      return;
    }
  }

  if (newVdevs.length > 0) {
    const hasRedundantExisting = existingActiveVdevs.value.some(
      (ev) => ev.type === 'data' && ev.layout !== 'stripe'
    ) || (p.vdevLayout && p.vdevLayout !== 'stripe');
    const addingStripeData = newVdevs.some(
      (v) => v.type === 'data' && v.layout === 'stripe'
    );
    const vdevSummary = newVdevs
      .map((v) => `${v.type.toUpperCase()} (${v.layout.toUpperCase()}: ${v.devicePaths.join(', ')})`)
      .join('; ');
    const confirmed = await confirmModal({
      title: addingStripeData && hasRedundantExisting
        ? 'Warning: Adding Non-Redundant Stripe VDEV to Redundant Pool'
        : `Confirm Pool Expansion (${cleanName.value})`,
      tone: addingStripeData && hasRedundantExisting ? 'danger' : 'warning',
      message: addingStripeData && hasRedundantExisting
        ? `You are about to add a non-redundant STRIPE Data VDEV to redundant pool "${cleanName.value}"!`
        : `Add ${newVdevs.length} new VDEV tier(s) to pool "${cleanName.value}"?`,
      details: addingStripeData && hasRedundantExisting
        ? `Adding a single-disk or stripe Data VDEV (${vdevSummary}) permanently removes pool-level redundancy: if that stripe drive fails, the ENTIRE pool "${cleanName.value}" will be lost and cannot be detached.`
        : `The following VDEV tier(s) will be permanently added to "${cleanName.value}": ${vdevSummary}. Data VDEVs cannot be easily removed from RAIDZ pools once added.`,
      confirmText: addingStripeData && hasRedundantExisting ? 'Add Stripe VDEV Anyway' : 'Expand Pool',
    });
    if (!confirmed) return;
  }

  isBusy.value = true;

  try {
    const hasSettingsChanges =
      configForm.sync !== ((p.sync as any) || 'standard') ||
      configForm.autotrim !== ((p.autotrim as any) || 'off') ||
      configForm.compression !== (p.compression || 'lz4');

    if (hasSettingsChanges) {
      await services.zfs.updatePool(p.name, {
        sync: configForm.sync,
        autotrim: configForm.autotrim,
        compression: configForm.compression,
      });
    }

    for (const v of newVdevs) {
      await services.zfs.addVdev(p.name, {
        type: v.type,
        layout: v.layout,
        devices: v.devicePaths,
      });
    }

    notify(
      'success',
      newVdevs.length > 0
        ? `Expanded pool "${cleanName.value}" with ${newVdevs.length} new VDEV tier(s)`
        : `Updated properties for pool "${cleanName.value}"`
    );
    editBuilderVdevs.value = [];
    await refreshAllData();
    await refreshZfsSubresources(cleanName.value);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update pool layout');
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

function getDiskRole(d: Disk): string {
  if (pool.value?.vdevs) {
    for (const v of pool.value.vdevs) {
      const vType = v.type || 'data';
      const vLayout = v.layout || pool.value.vdevLayout || 'mirror';
      const roleLabel = vType === 'data' ? vLayout : `${vType}${vLayout && vLayout !== 'stripe' ? ' (' + vLayout + ')' : ''}`;
      if (v.path === d.devicePath || v.name === d.devicePath || v.name === d.diskId) {
        return roleLabel;
      }
      if (v.children) {
        for (const c of v.children) {
          if (c.path === d.devicePath || c.name === d.devicePath || c.name === d.diskId) {
            return roleLabel;
          }
        }
      }
    }
  }
  return pool.value?.vdevLayout || 'mirror';
}

function getDiskState(d: Disk): string {
  if (pool.value?.vdevs) {
    for (const v of pool.value.vdevs) {
      if (v.path === d.devicePath || v.name === d.devicePath || v.name === d.diskId) {
        if (v.state) return v.state;
      }
      if (v.children) {
        for (const c of v.children) {
          if (c.path === d.devicePath || c.name === d.devicePath || c.name === d.diskId) {
            if (c.state) return c.state;
          }
        }
      }
    }
  }
  if (d.smartStatus && d.smartStatus.toUpperCase() === 'FAILED') {
    return 'FAULTED';
  }
  return pool.value?.state || 'ONLINE';
}

function getStateBadgeClass(state: string): string {
  const st = (state || 'ONLINE').toUpperCase();
  if (st === 'ONLINE' || st === 'AVAIL' || st === 'INUSE') return 'badge-success';
  if (st === 'DEGRADED' || st === 'RESILVERING') return 'badge-warning';
  return 'badge-danger';
}

function getDiskStateBadgeClass(d: Disk): string {
  return getStateBadgeClass(getDiskState(d));
}

function isNonZeroErr(v?: string | number): boolean {
  if (v === undefined || v === null) return false;
  const s = String(v).trim();
  return s !== '' && s !== '0';
}

interface TopologyRow {
  key: string;
  label: string;
  isGroup: boolean;
  role: string;
  model: string;
  sizeBytes: number;
  transport: string;
  state: string;
  readErrors: string;
  writeErrors: string;
  checksumErrors: string;
  hasErrors: boolean;
}

const poolTopologyRows = computed<TopologyRow[]>(() => {
  const rows: TopologyRow[] = [];
  const p = pool.value;
  if (!p) return rows;

  if (p.vdevs && p.vdevs.length > 0) {
    p.vdevs.forEach((v, idx) => {
      const vType = v.type || 'data';
      const vLayout = v.layout || p.vdevLayout || 'stripe';
      const roleLabel = vType === 'data' ? vLayout : `${vType}${vLayout && vLayout !== 'stripe' ? ' (' + vLayout + ')' : ''}`;

      if (v.children && v.children.length > 0) {
        const vr = String(v.readErrors ?? '0');
        const vw = String(v.writeErrors ?? '0');
        const vc = String(v.checksumErrors ?? '0');
        rows.push({
          key: `vdev-${idx}-${v.name}`,
          label: v.name || `${vType}-${idx}`,
          isGroup: true,
          role: roleLabel.toUpperCase(),
          model: `${v.children.length} member drive(s)`,
          sizeBytes: 0,
          transport: '-',
          state: v.state || p.state || 'ONLINE',
          readErrors: vr,
          writeErrors: vw,
          checksumErrors: vc,
          hasErrors: isNonZeroErr(vr) || isNonZeroErr(vw) || isNonZeroErr(vc),
        });
        v.children.forEach((c, cIdx) => {
          const devKey = c.path || c.name || `dev-${idx}-${cIdx}`;
          const disk = nasState.disks.find(
            (d) => d.devicePath === devKey || d.diskId === devKey || d.name === devKey || (c.name && d.diskId === c.name)
          );
          const cr = String(c.readErrors ?? '0');
          const cw = String(c.writeErrors ?? '0');
          const cc = String(c.checksumErrors ?? '0');
          rows.push({
            key: `child-${idx}-${cIdx}-${devKey}`,
            label: devKey,
            isGroup: false,
            role: roleLabel,
            model: disk ? disk.model || disk.vendor || '-' : 'Missing / Detached Disk',
            sizeBytes: disk?.sizeBytes || 0,
            transport: disk?.transport || '-',
            state: c.state || (disk ? getDiskState(disk) : 'UNAVAIL'),
            readErrors: cr,
            writeErrors: cw,
            checksumErrors: cc,
            hasErrors: isNonZeroErr(cr) || isNonZeroErr(cw) || isNonZeroErr(cc),
          });
        });
      } else {
        const devKey = v.path || v.name || `vdev-${idx}`;
        const disk = nasState.disks.find(
          (d) => d.devicePath === devKey || d.diskId === devKey || d.name === devKey
        );
        const vr = String(v.readErrors ?? '0');
        const vw = String(v.writeErrors ?? '0');
        const vc = String(v.checksumErrors ?? '0');
        rows.push({
          key: `leaf-${idx}-${devKey}`,
          label: devKey,
          isGroup: false,
          role: roleLabel,
          model: disk ? disk.model || disk.vendor || '-' : 'Missing / Detached Disk',
          sizeBytes: disk?.sizeBytes || 0,
          transport: disk?.transport || '-',
          state: v.state || (disk ? getDiskState(disk) : p.state || 'ONLINE'),
          readErrors: vr,
          writeErrors: vw,
          checksumErrors: vc,
          hasErrors: isNonZeroErr(vr) || isNonZeroErr(vw) || isNonZeroErr(vc),
        });
      }
    });
    return rows;
  }

  for (const d of poolDisks.value) {
    rows.push({
      key: d.devicePath,
      label: d.devicePath,
      isGroup: false,
      role: getDiskRole(d),
      model: d.model || d.vendor || '-',
      sizeBytes: d.sizeBytes,
      transport: d.transport || 'SATA/NVMe',
      state: getDiskState(d),
      readErrors: '0',
      writeErrors: '0',
      checksumErrors: '0',
      hasErrors: false,
    });
  }
  return rows;
});

const poolLeafDriveRows = computed(() => poolTopologyRows.value.filter((r) => !r.isGroup));

const scrubNeverRun = computed(() => {
  const s = (pool.value?.lastScrubStatus || '').trim().toLowerCase();
  return !s || s === 'never scrubbed' || s === 'none';
});

const scrubHasWarningOrError = computed(() => {
  const s = (pool.value?.lastScrubStatus || '').toLowerCase();
  if (!s) return false;
  if (s.includes('canceled') || s.includes('cancelled') || s.includes('aborted')) return true;
  const errMatch = /(\d+)\s+errors?/.exec(s);
  if (errMatch && parseInt(errMatch[1], 10) > 0) return true;
  if (s.includes('error') && !s.includes('0 errors') && !s.includes('no scrub errors') && !s.includes('no known data errors')) {
    return true;
  }
  return false;
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
                <span class="metric-val badge badge-neutral">{{ (pool?.vdevLayout || 'stripe').toUpperCase() }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Deduplication</span>
                <span class="metric-val">{{ pool?.dedupRatio ? pool.dedupRatio.toFixed(2) + 'x' : '1.00x' }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Compression</span>
                <span class="metric-val">{{ pool?.compression || 'lz4' }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Sync Policy</span>
                <span class="metric-val">{{ pool?.sync || 'standard' }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Autotrim</span>
                <span class="metric-val">{{ pool?.autotrim || 'off' }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Health State</span>
                <span
                  class="metric-val"
                  :style="{
                    color:
                      !pool?.state || pool.state === 'ONLINE'
                        ? 'var(--success-text)'
                        : pool.state === 'DEGRADED'
                        ? 'var(--warning-text, var(--warning))'
                        : 'var(--danger-text, var(--danger))',
                  }"
                >
                  {{ pool?.state || 'ONLINE' }}
                </span>
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
                <MdiIcon
                  :path="scrubHasWarningOrError ? mdiAlertCircle : scrubNeverRun ? mdiInformationOutline : mdiCheckCircle"
                  size="20"
                  :style="{
                    color: scrubHasWarningOrError
                      ? 'var(--warning)'
                      : scrubNeverRun
                      ? 'var(--text-secondary)'
                      : 'var(--success)',
                  }"
                />
                <span style="font-weight: 600">{{ pool?.lastScrubStatus || 'Never scrubbed' }}</span>
              </div>
              <div v-if="pool?.healthDetails" class="font-xs" style="margin-top: 6px; color: var(--text-secondary)">
                Health details: <strong>{{ pool.healthDetails }}</strong>
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
              <div v-if="poolLeafDriveRows.length === 0" class="text-muted font-sm">
                No physical drive records linked.
              </div>
              <div v-else class="table-wrap">
                <table class="data-table">
                  <thead>
                    <tr>
                      <th>Device</th>
                      <th>Model</th>
                      <th>Capacity</th>
                      <th>Errors (R/W/C)</th>
                      <th>Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="row in poolLeafDriveRows" :key="row.key">
                      <td class="mono font-sm" style="font-weight: 600">{{ row.label }}</td>
                      <td class="font-sm text-secondary">{{ row.model }}</td>
                      <td class="font-sm">{{ row.sizeBytes > 0 ? formatBytes(row.sizeBytes) : '-' }}</td>
                      <td
                        class="mono font-xs"
                        :class="row.hasErrors ? 'text-red' : 'text-muted'"
                      >
                        {{ row.readErrors }} / {{ row.writeErrors }} / {{ row.checksumErrors }}
                      </td>
                      <td>
                        <span class="badge font-xs" :class="getStateBadgeClass(row.state)">{{ row.state }}</span>
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
      <DatasetsView :initial-pool="cleanName" :embedded="true" :auto-expand-roots="true" />
    </div>

    <!-- Subtab 3: Pool Volumes (ZVols) -->
    <div v-else-if="props.activeSubTab === 'volumes'" class="pool-tab-content">
      <VolumesView :initial-pool="cleanName" :embedded="true" :auto-expand-roots="true" />
    </div>

    <!-- Subtab 4: Topology & Disks -->
    <div v-else-if="props.activeSubTab === 'topology'" class="pool-tab-content">
      <div class="card">
        <div class="card-header">
          <div>
            <h3 class="card-title">VDEV Architecture & Physical Disk Topology</h3>
            <p class="card-subtitle">Storage devices and VDEV error counters currently forming pool {{ cleanName }}</p>
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
                  <th>Errors (R / W / Cksum)</th>
                  <th>State</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in poolTopologyRows" :key="row.key">
                  <td
                    class="mono font-sm"
                    :style="{
                      fontWeight: row.isGroup ? 700 : 600,
                      paddingLeft: row.isGroup ? '12px' : '28px',
                    }"
                  >
                    {{ row.label }}
                  </td>
                  <td class="font-sm"><span class="badge" :class="row.isGroup ? 'badge-info' : 'badge-neutral'">{{ row.role }}</span></td>
                  <td class="font-sm text-secondary">{{ row.model }}</td>
                  <td class="font-sm">{{ row.sizeBytes > 0 ? formatBytes(row.sizeBytes) : '-' }}</td>
                  <td class="font-xs text-muted">{{ row.transport }}</td>
                  <td
                    class="mono font-xs"
                    :class="row.hasErrors ? 'text-red' : 'text-muted'"
                  >
                    {{ row.readErrors }} / {{ row.writeErrors }} / {{ row.checksumErrors }}
                  </td>
                  <td><span class="badge font-xs" :class="getStateBadgeClass(row.state)">{{ row.state }}</span></td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    <!-- Subtab 5: Configuration & VDEV Infrastructure Designer -->
    <div v-else-if="props.activeSubTab === 'config'" class="pool-tab-content" style="display: flex; flex-direction: column; gap: 20px">
      <!-- 1. Pool Properties Card -->
      <div class="card">
        <div class="card-header">
          <div>
            <h3 class="card-title">Storage Policies &amp; Properties</h3>
            <p class="card-subtitle">Configure synchronization, compression, and TRIM policies for {{ cleanName }}</p>
          </div>
          <button class="btn btn-primary btn-sm" :disabled="isBusy" @click="handleSaveConfig">
            <MdiIcon :path="mdiCheck" size="14" style="margin-right: 4px" />
            Save Properties
          </button>
        </div>

        <div class="card-body">
          <div class="form-grid" style="grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 16px">
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
          </div>
        </div>
      </div>

      <!-- 2. VDEV Layout & Storage Expansion Drag & Drop Workspace -->
      <div class="card">
        <div class="card-header">
          <div>
            <h3 class="card-title">VDEV Infrastructure &amp; Storage Expansion</h3>
            <p class="card-subtitle">Drag and drop unattached physical drives to expand storage or add cache, log, and spare tiers</p>
          </div>
          <div style="display: flex; gap: 8px">
            <button
              class="btn btn-primary btn-sm"
              :disabled="!isEditPoolBuilderValid || isBusy"
              @click="handleApplyPoolConfig"
            >
              <MdiIcon v-if="isBusy" :path="mdiRefresh" size="14" class="spin" style="margin-right: 4px" />
              <MdiIcon v-else :path="mdiCheck" size="14" style="margin-right: 4px" />
              {{ isBusy ? 'Applying...' : 'Apply Layout & Expand' }}
            </button>
          </div>
        </div>

        <div class="card-body">
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
                  class="form-input"
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
                  No unassigned drives available on this host.
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
                    <div style="display: flex; gap: 4px; flex-wrap: wrap">
                      <span class="badge badge-neutral" style="font-size: 9.5px">{{ d.type }}</span>
                      <span class="badge badge-neutral" style="font-size: 9.5px">{{ (d.transport || 'disk').toUpperCase() }}</span>
                      <span
                        v-if="d.smartStatus"
                        class="badge"
                        :class="d.smartStatus === 'PASSED' ? 'badge-success' : d.smartStatus === 'FAILED' ? 'badge-danger' : 'badge-warning'"
                        style="font-size: 9.5px"
                      >{{ d.smartStatus }}</span>
                      <span
                        v-if="d.partitions && d.partitions.length > 0"
                        class="badge badge-warning"
                        style="font-size: 9.5px"
                        :title="`Warning: Disk has ${d.partitions.length} existing partition(s) that will be overwritten`"
                      >
                        {{ d.partitions.length }} part{{ d.partitions.length === 1 ? '' : 's' }} (will wipe)
                      </span>
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
                  <span>Pool VDEV Tiers</span>
                </div>

                <!-- Add Tier VDEV Buttons -->
                <div style="display: flex; gap: 6px; flex-wrap: wrap">
                  <button type="button" class="btn btn-xs btn-primary-outline" @click="addEditBuilderVdev('data')">
                    <MdiIcon :path="mdiPlus" size="12" /> Data VDEV
                  </button>
                  <button type="button" class="btn btn-xs" style="background: var(--accent-bg-subtle); color: var(--purple-text); border: 1px solid var(--border-subtle)" @click="addEditBuilderVdev('cache')">
                    <MdiIcon :path="mdiFlash" size="12" /> + Cache (L2ARC)
                  </button>
                  <button type="button" class="btn btn-xs" style="background: var(--warning-bg); color: var(--warning-text); border: 1px solid var(--warning-border)" @click="addEditBuilderVdev('log')">
                    <MdiIcon :path="mdiPlus" size="12" /> + Log (SLOG)
                  </button>
                  <button type="button" class="btn btn-xs btn-neutral" @click="addEditBuilderVdev('spare')">
                    <MdiIcon :path="mdiPlus" size="12" /> + Hot Spare
                  </button>
                  <button type="button" class="btn btn-xs" style="background: var(--success-bg); color: var(--success-text); border: 1px solid var(--success-border)" @click="addEditBuilderVdev('special')">
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
                  <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; border-bottom: 1px solid var(--border-subtle); padding-bottom: 8px">
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
                    <span>Active ({{ vdev.state || 'ONLINE' }})</span>
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
                  <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; border-bottom: 1px solid var(--border-subtle); padding-bottom: 8px">
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
                          class="form-select mono"
                          style="font-size: 11px; height: 26px; padding: 2px 20px 2px 8px"
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
                          class="form-select mono"
                          style="font-size: 11px; height: 26px; padding: 2px 20px 2px 8px"
                        >
                          <option value="mirror">Mirror (Recommended)</option>
                          <option value="stripe">Stripe (Single SLOG)</option>
                        </select>
                        <select
                          v-else-if="vdev.type === 'special'"
                          v-model="vdev.layout"
                          class="form-select mono"
                          style="font-size: 11px; height: 26px; padding: 2px 20px 2px 8px"
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

                <!-- Empty Builder State if No Expansion VDEVs -->
                <div
                  v-if="editBuilderVdevs.length === 0"
                  style="text-align: center; padding: 24px; border: 1px dashed var(--border-subtle); border-radius: 8px; color: var(--text-muted); font-size: 12px"
                >
                  <span>No expansion VDEVs staged. Click <strong>+ Data VDEV</strong>, <strong>+ Cache</strong>, or <strong>+ Log</strong> above to add more tiers.</span>
                </div>
              </div>

              <!-- Topology Capacity & Usable Storage Summary Banner -->
              <div class="capacity-summary-card" style="margin-top: 8px">
                <div style="font-weight: 700; font-size: 12.5px; color: var(--text-primary); margin-bottom: 8px; display: flex; align-items: center; gap: 6px">
                  <span>Estimated Total Capacity After Expansion</span>
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
  font-size: 13px;
}

.font-xs {
  font-size: 11.5px;
}

/* -------------------------------------------------------------------------- */
/* Builder Workspace Styles                                                  */
/* -------------------------------------------------------------------------- */
.builder-workspace {
  display: grid;
  grid-template-columns: 360px 1fr;
  gap: 18px;
}

@media (max-width: 960px) {
  .builder-workspace {
    grid-template-columns: 1fr;
  }
}

.drives-bank-panel {
  background: var(--bg-card);
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  max-height: 580px;
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
  border-radius: 8px;
  padding: 10px 12px;
  cursor: grab;
  user-select: none;
  box-shadow: none;
  transition: border-color 0.12s ease;
}

.drive-card:hover {
  border-color: var(--accent-primary);
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
  max-height: 580px;
  padding-right: 4px;
}

.vdev-builder-card {
  background: var(--bg-card);
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  padding: 14px;
  transition: border-color 0.15s ease;
}

.vdev-builder-card.is-data {
  border-left: 3px solid var(--accent-primary);
}

.vdev-builder-card.is-cache {
  border-left: 3px solid var(--purple-text, #a855f7);
}

.vdev-builder-card.is-log {
  border-left: 3px solid var(--warning-text, #f59e0b);
}

.vdev-builder-card.is-spare {
  border-left: 3px solid var(--text-secondary);
}

.vdev-builder-card.is-special {
  border-left: 3px solid var(--success-text, #22c55e);
}

.vdev-builder-card.drag-hover {
  border-color: var(--accent-primary);
  background: var(--accent-bg-subtle, rgba(2, 132, 199, 0.08));
}

.vdev-drop-target {
  min-height: 64px;
  border: 1px dashed var(--border-strong, #475569);
  border-radius: 8px;
  padding: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  transition: border-color 0.15s ease, background-color 0.15s ease;
}

.vdev-drop-target.has-drives {
  border-style: solid;
  border-color: var(--border-subtle);
  background: var(--bg-surface);
}

.drop-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  color: var(--text-muted);
  font-size: 11.5px;
  text-align: center;
  pointer-events: none;
  gap: 4px;
}

.vdev-drive-chip {
  background: var(--bg-card);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 6px 10px;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  box-shadow: none;
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
  color: var(--danger-text, #ef4444);
}

.vdev-validation-bar {
  margin-top: 8px;
  padding: 6px 10px;
  border-radius: 6px;
  font-size: 11px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.vdev-validation-bar.ok {
  background: var(--success-bg, rgba(34, 197, 94, 0.12));
  color: var(--success-text, #4ade80);
  border: 1px solid var(--success-border, rgba(34, 197, 94, 0.25));
}

.vdev-validation-bar.warn {
  background: var(--warning-bg, rgba(245, 158, 11, 0.12));
  color: var(--warning-text, #fbbf24);
  border: 1px solid var(--warning-border, rgba(245, 158, 11, 0.25));
}

.vdev-validation-bar.info {
  background: var(--bg-surface-elevated, #1e293b);
  color: var(--text-muted);
  border: 1px solid var(--border-subtle);
}

.capacity-summary-card {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  padding: 14px 16px;
}

.summary-metrics-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 14px;
}
</style>
