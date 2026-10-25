<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import {
  mdiCamera,
  mdiCheck,
  mdiChevronDown,
  mdiChevronRight,
  mdiClose,
  mdiCogOutline,
  mdiCrown,
  mdiDatabase,
  mdiDelete,
  mdiFlash,
  mdiFolderOutline,
  mdiFormatListBulleted,
  mdiHarddisk,
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
  type ZfsDataset,
  type ZfsPool,
  type ZfsVdev,
  type ZfsVolume,
} from '../../domain/models';
import MdiIcon from '../components/MdiIcon.vue';
import ZfsPoolGauge from '../components/ZfsPoolGauge.vue';
import { nasState, notify, refreshAllData, refreshZfsSubresources } from '../store';

// ---------------------------------------------------------------------------
// 1. Top-Level State & Selection
// ---------------------------------------------------------------------------

const showCreatePool = ref(false);
const poolForm = reactive({
  poolId: '',
  vdevLayout: 'raidz1',
  devicesText: '/dev/sdb, /dev/sdc, /dev/sdd',
  compression: 'lz4',
  mountpoint: '',
});

const activePool = computed(() =>
  nasState.pools.find((p) => p.name === nasState.selectedPoolName) || nasState.pools[0] || null
);

const activeDatasets = computed(() =>
  activePool.value ? nasState.datasetsByPool[activePool.value.name] || [] : []
);

const activeVolumes = computed(() =>
  activePool.value ? nasState.volumesByPool[activePool.value.name] || [] : []
);

async function selectPool(poolName: string) {
  nasState.selectedPoolName = poolName;
  await refreshZfsSubresources(poolName);
}

// ---------------------------------------------------------------------------
// 2. Pool Operations (Create, Scrub, Delete)
// ---------------------------------------------------------------------------

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
      mountpoint: poolForm.mountpoint || `/nas/${poolForm.poolId.trim()}`,
    });
    notify('success', `Created ZFS pool ${created.poolName || created.name}`);
    poolForm.poolId = '';
    showCreatePool.value = false;
    await refreshAllData();
    nasState.selectedPoolName = created.name;
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

async function handleSetMainPool(poolName: string) {
  const cleanName = poolName.replace(/^zfsPools\//, '');
  const current = nasState.systemConfig?.mainPool;
  if (current === cleanName) return;

  const confirmMsg = current
    ? `Change Main Storage Pool from "${current}" to "${cleanName}"?\n\nConfiguration, apps root, and LXC root will now reside on "${cleanName}".`
    : `Designate "${cleanName}" as the Main Storage Pool?\n\nThis will initialize .nas-system datasets and enable all paused services.`;

  if (!confirm(confirmMsg)) return;

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
  if (!confirm(`Are you sure you want to remove device "${devicePath}" from pool?`)) {
    return;
  }
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

async function handleDeleteDataset(datasetName: string) {
  const shortName = datasetName.replace(/^zfsPools\/[^/]+\/datasets\//, '');
  if (!confirm(`Are you sure you want to delete dataset "${shortName}"? ALL DATA WILL BE LOST.`)) {
    return;
  }
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

async function handleDeleteVolume(volName: string) {
  const shortName = volName.replace(/^zfsPools\/[^/]+\/volumes\//, '');
  if (!confirm(`Are you sure you want to delete ZVol "${shortName}"?`)) {
    return;
  }
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
            <MdiIcon :path="mdiHarddisk" size="24" style="color: var(--accent-primary)" />
            <h2 class="card-title" style="font-size: 19px; margin: 0">
              Storage Pools
            </h2>
          </div>
          <p class="card-subtitle" style="margin-top: 4px; font-size: 13px">
            ZFS storage pools, redundancy layouts, capacity allocation, fragmentation, and storage tiers.
          </p>
        </div>

        <button class="btn btn-primary" @click="showCreatePool = !showCreatePool">
          <MdiIcon :path="showCreatePool ? mdiClose : mdiPlus" size="16" style="margin-right: 4px" />
          {{ showCreatePool ? 'Cancel' : 'Create Storage Pool' }}
        </button>
      </div>

      <!-- Create Pool Form Modal/Inline -->
      <form
        v-if="showCreatePool"
        @submit.prevent="handleCreatePool"
        style="
          background: var(--bg-base);
          border: 1px solid var(--border-strong);
          border-radius: 8px;
          padding: 16px;
          margin-top: 18px;
        "
      >
        <div class="form-grid">
          <div class="form-group">
            <label>Pool Name</label>
            <input v-model="poolForm.poolId" class="input mono" placeholder="tank" required />
          </div>
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
          <div class="form-group">
            <label>Backing Block Devices (comma-separated)</label>
            <input
              v-model="poolForm.devicesText"
              class="input mono"
              placeholder="/dev/sdb, /dev/sdc"
              required
            />
          </div>
        </div>
        <div style="margin-top: 14px; display: flex; justify-content: flex-end; gap: 8px">
          <button type="button" class="btn btn-sm" @click="showCreatePool = false">Cancel</button>
          <button type="submit" class="btn btn-primary btn-sm">Create Storage Pool</button>
        </div>
      </form>

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
                <div class="pool-mountpoint mono">
                  <MdiIcon :path="mdiFolderOutline" size="13" style="margin-right: 3px; opacity: 0.7" />
                  {{ pool.mountpoint || (`/nas/${pool.poolName || pool.name.replace(/^zfsPools\//, '')}`) }}
                </div>
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
                <MdiIcon :path="mdiCogOutline" size="14" style="margin-right: 4px" />
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
    <div v-if="configuringPool" class="modal-overlay" @click.self="closePoolConfig">
      <div class="modal-container" style="max-width: 720px; width: 95%">
        <!-- Modal Header -->
        <div class="modal-header">
          <div style="display: flex; align-items: center; gap: 10px">
            <div class="pool-icon-wrap" style="width: 32px; height: 32px">
              <MdiIcon :path="mdiCogOutline" size="20" style="color: var(--accent-primary)" />
            </div>
            <div>
              <h3 style="margin: 0; font-size: 17px; font-weight: 700">
                Configure Pool: <span class="mono" style="color: var(--accent-primary)">{{ configuringPool.poolName || configuringPool.name.replace(/^zfsPools\//, '') }}</span>
              </h3>
              <p style="margin: 2px 0 0; font-size: 12px; color: var(--text-secondary)">
                Tune synchronous write mode, SSD auto TRIM, compression, and manage VDEV storage tiers.
              </p>
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
            Pool Properties
          </button>
          <button
            class="tab-btn"
            :class="{ active: configTab === 'vdevs' }"
            @click="configTab = 'vdevs'"
          >
            <MdiIcon :path="mdiLayersOutline" size="15" style="margin-right: 6px" />
            VDEVs & Storage Tiers
          </button>
        </div>

        <!-- Modal Body: Tab 1 (Settings) -->
        <div v-if="configTab === 'settings'" class="modal-body">
          <form @submit.prevent="handleSavePoolSettings" style="display: flex; flex-direction: column; gap: 18px">
            <!-- Sync Mode -->
            <div class="config-section">
              <label class="config-label">
                <span>Sync Mode (ZFS Intent Log semantics)</span>
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
                <span>Auto TRIM (SSD / NVMe Space Reclamation)</span>
                <span class="badge badge-neutral mono">{{ poolSettingsForm.autotrim }}</span>
              </label>
              <select v-model="poolSettingsForm.autotrim" class="select mono" style="width: 100%">
                <option value="on">on - Automatically TRIM/UNMAP freed blocks continuously</option>
                <option value="off">off - Do not automatically issue TRIM commands</option>
              </select>
              <div class="config-hint">
                Enables automatic background trimming of unallocated space on SSD/NVMe devices to sustain write performance and flash longevity.
              </div>
            </div>

            <!-- Compression -->
            <div class="config-section">
              <label class="config-label">
                <span>Default Pool Compression</span>
                <span class="badge badge-info mono">{{ poolSettingsForm.compression }}</span>
              </label>
              <select v-model="poolSettingsForm.compression" class="select mono" style="width: 100%">
                <option value="lz4">lz4 (Fast, zero-overhead, highly recommended)</option>
                <option value="zstd">zstd (Higher ratio compression)</option>
                <option value="gzip">gzip (High ratio legacy)</option>
                <option value="off">off (No compression)</option>
              </select>
              <div class="config-hint">
                Default transparent inline compression applied to new datasets created under this storage pool.
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
              <MdiIcon :path="mdiLayersOutline" size="16" style="color: var(--accent-primary)" />
              Active Storage Tiers in Pool
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
              <MdiIcon :path="mdiPlus" size="16" style="color: var(--accent-primary)" />
              Add Storage Tier to Pool
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
            <MdiIcon :path="mdiDatabase" size="22" style="color: var(--accent-primary)" />
            <h3 class="card-title" style="font-size: 17px; margin: 0">
              Filesystems & Block Volumes in <span class="mono" style="color: var(--accent-primary)">{{ activePool.poolName || activePool.name.replace(/^zfsPools\//, '') }}</span>
            </h3>
          </div>
          <p class="card-subtitle" style="margin-top: 3px; font-size: 12.5px">
            Hierarchical view of ZFS datasets, nested filesystems, and raw ZVol block devices.
          </p>
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
            <span>Create Filesystem Dataset</span>
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
            <span>Create Block Volume (ZVol)</span>
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
            <span>Editing Dataset Properties:</span>
            <span class="mono" style="color: var(--accent-primary)">{{ editingDataset.datasetPath }}</span>
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
            <span>Editing Block Volume (ZVol):</span>
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
              <th style="min-width: 240px">Name & Hierarchy</th>
              <th style="min-width: 180px">Mount / Device Path</th>
              <th style="min-width: 140px">Space Usage</th>
              <th style="min-width: 200px">Properties</th>
              <th style="min-width: 100px">Snapshots</th>
              <th style="min-width: 140px; text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="visibleHierarchyNodes.length === 0">
              <td colspan="6" style="text-align: center; color: var(--text-secondary); padding: 32px">
                <div v-if="searchQuery">
                  No datasets or volumes match filter "<strong>{{ searchQuery }}</strong>".
                </div>
                <div v-else>
                  No child datasets or block volumes in pool "{{ activePool.poolName || activePool.name.replace(/^zfsPools\//, '') }}". Create one above.
                </div>
              </td>
            </tr>

            <tr
              v-for="node in visibleHierarchyNodes"
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

              <!-- Mountpoint or Device Path -->
              <td class="mount-cell">
                <span v-if="node.type === 'dataset'" class="mono" style="font-size: 12px; color: var(--text-secondary)" :title="node.mountpoint || '-'">
                  {{ node.mountpoint || '-' }}
                </span>
                <span v-else class="mono" style="font-size: 11.5px; color: #c084fc" :title="node.devicePath || '-'">
                  {{ node.devicePath || '-' }}
                </span>
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
                      @click="handleDeleteDataset(node.rawDataset.name)"
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
                      @click="handleDeleteVolume(node.rawVolume.name)"
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

.mount-cell {
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
