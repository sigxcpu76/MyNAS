<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import {
  mdiCamera,
  mdiChevronDown,
  mdiChevronRight,
  mdiClose,
  mdiDatabase,
  mdiDelete,
  mdiFolderOutline,
  mdiFormatListBulleted,
  mdiMagnify,
  mdiPencil,
  mdiPlus,
  mdiRefresh,
  mdiTreeOutline,
} from '@mdi/js';
import { services } from '../../container';
import {
  bytesToGib,
  formatBytes,
  gibToBytes,
  type ZfsDataset,
  isDotDataset,
} from '../../domain/models';
import MdiIcon from '../components/MdiIcon.vue';
import SkeletonBox from '../components/SkeletonBox.vue';
import SortableTh from '../components/SortableTh.vue';
import PathSelector from '../components/PathSelector.vue';
import { sortHierarchy, useTableSort } from '../composables/useTableSort';
import { confirmDelete } from '../modal';
import { nasState, notify, refreshAllData, refreshZfsSubresources } from '../store';

const selectedPool = ref<string>('all');
const searchQuery = ref('');
const viewMode = ref<'tree' | 'flat'>('tree');
const collapsedPaths = ref<Set<string>>(new Set());

// Modals
const showCreateDataset = ref(false);
const editingDataset = ref<ZfsDataset | null>(null);
const snapshotModalDataset = ref<ZfsDataset | null>(null);
const snapshotName = ref('');

// Dataset Form
const datasetForm = reactive({
  parentPoolOrDataset: '',
  datasetId: '',
  mountpoint: '',
  compression: 'lz4',
  quotaGib: 0,
  reservationGib: 0,
  sync: 'standard',
  atime: 'on',
  dedup: false,
});

const editDatasetForm = reactive({
  compression: 'lz4',
  quotaGib: 0,
  reservationGib: 0,
  sync: 'standard',
  atime: 'on',
  dedup: false,
});

// All datasets combined or filtered by selectedPool, ensuring each pool tree has the zpool dataset as root
const allDatasets = computed<Array<ZfsDataset & { poolName: string; isPoolRoot: boolean }>>(() => {
  const result: Array<ZfsDataset & { poolName: string; isPoolRoot: boolean }> = [];
  for (const pool of nasState.pools) {
    const pName = pool.name;
    const shortName = pool.poolName || pName.replace(/^zfsPools\//, '');
    if (selectedPool.value !== 'all' && selectedPool.value !== pName && selectedPool.value !== shortName) {
      continue;
    }
    const dss = nasState.datasetsByPool[pName] || [];

    // Find if the pool's root dataset is present in dss
    const poolRootDs = dss.find(
      (d) => d.datasetPath === shortName || d.datasetPath === pool.poolName || d.name.endsWith('/datasets/root')
    );

    if (poolRootDs) {
      result.push({
        ...poolRootDs,
        datasetPath: shortName,
        poolName: shortName,
        isPoolRoot: true,
      });
    } else {
      // Synthesize root dataset node for this pool from pool metadata so it always exists as tree root
      result.push({
        name: `${pName}/datasets/root`,
        datasetPath: shortName,
        mountpoint: pool.mountpoint || `/${shortName}`,
        usedBytes: pool.allocatedBytes,
        availableBytes: pool.freeBytes,
        referencedBytes: 0,
        quotaBytes: 0,
        compression: pool.compression || 'lz4',
        recordsize: '128k',
        readonly: false,
        atime: true,
        snapshotsCount: 0,
        lastSnapshot: '',
        poolName: shortName,
        isPoolRoot: true,
      });
    }

    for (const ds of dss) {
      // Do not duplicate the pool root dataset
      if (ds === poolRootDs || ds.datasetPath === shortName || ds.name.endsWith('/datasets/root')) {
        continue;
      }
      // Hide internal system datasets starting with a dot
      if (isDotDataset(ds)) {
        continue;
      }
      result.push({ ...ds, poolName: shortName, isPoolRoot: false });
    }
  }
  return result;
});

// Flat or Tree Hierarchy Node
interface DatasetNode {
  id: string;
  name: string;
  fullName: string;
  poolName: string;
  mountpoint: string;
  usedBytes: number;
  availableBytes: number;
  quotaBytes?: number;
  reservationBytes?: number;
  compression: string;
  recordsize: string;
  sync?: string;
  atime?: boolean;
  dedup?: boolean;
  snapshotsCount?: number;
  lastSnapshot?: string;
  depth: number;
  isPoolRoot: boolean;
  raw: ZfsDataset;
  children: DatasetNode[];
}

const hierarchyTree = computed<DatasetNode[]>(() => {
  const nodesMap = new Map<string, DatasetNode>();
  const rootNodes: DatasetNode[] = [];

  for (const ds of allDatasets.value) {
    const fullName = ds.datasetPath || ds.name.replace(/^zfsPools\/[^/]+\/datasets\//, '');
    const segments = fullName.split('/');
    const shortName = segments[segments.length - 1];

    const node: DatasetNode = {
      id: ds.name,
      name: shortName,
      fullName,
      poolName: ds.poolName,
      mountpoint: ds.mountpoint,
      usedBytes: ds.usedBytes,
      availableBytes: ds.availableBytes,
      quotaBytes: ds.quotaBytes,
      reservationBytes: ds.reservationBytes,
      compression: ds.compression || 'lz4',
      recordsize: ds.recordsize || '128k',
      sync: ds.sync,
      atime: ds.atime,
      dedup: ds.dedup,
      snapshotsCount: ds.snapshotsCount,
      lastSnapshot: ds.lastSnapshot,
      depth: ds.isPoolRoot ? 0 : 1,
      isPoolRoot: ds.isPoolRoot,
      raw: ds,
      children: [],
    };
    nodesMap.set(fullName, node);
  }

  // Link children
  for (const [, node] of nodesMap) {
    if (node.isPoolRoot) {
      rootNodes.push(node);
      continue;
    }

    const lastSlash = node.fullName.lastIndexOf('/');
    if (lastSlash !== -1) {
      const parentFullName = node.fullName.substring(0, lastSlash);
      const parent = nodesMap.get(parentFullName);
      if (parent) {
        parent.children.push(node);
        continue;
      }
    } else {
      // If single segment dataset not marked as root, attach to its pool root
      const poolRoot = nodesMap.get(node.poolName);
      if (poolRoot && poolRoot !== node) {
        poolRoot.children.push(node);
        continue;
      }
    }
    rootNodes.push(node);
  }

  // Sort: Pool root datasets first, then children recursively
  function sortNodes(nodes: DatasetNode[]) {
    nodes.sort((a, b) => {
      if (a.isPoolRoot && !b.isPoolRoot) return -1;
      if (!a.isPoolRoot && b.isPoolRoot) return 1;
      return a.fullName.localeCompare(b.fullName);
    });
    for (const n of nodes) {
      if (n.children.length > 0) {
        sortNodes(n.children);
      }
    }
  }
  sortNodes(rootNodes);

  // Compute depths based on tree nesting level
  function assignDepths(nodes: DatasetNode[], currentDepth: number) {
    for (const n of nodes) {
      n.depth = currentDepth;
      if (n.children.length > 0) {
        assignDepths(n.children, currentDepth + 1);
      }
    }
  }
  assignDepths(rootNodes, 0);

  return rootNodes;
});

function flattenTree(nodes: DatasetNode[], result: DatasetNode[] = []): DatasetNode[] {
  for (const node of nodes) {
    result.push(node);
    if (!collapsedPaths.value.has(node.id) && node.children.length > 0) {
      flattenTree(node.children, result);
    }
  }
  return result;
}

const visibleNodes = computed<DatasetNode[]>(() => {
  const q = searchQuery.value.trim().toLowerCase();
  if (viewMode.value === 'flat' || q) {
    return allDatasets.value
      .filter((ds) => {
        if (!q) return true;
        const fn = ds.datasetPath || ds.name;
        return fn.toLowerCase().includes(q) || ds.mountpoint.toLowerCase().includes(q) || ds.poolName.toLowerCase().includes(q);
      })
      .map((ds) => {
        const fn = ds.datasetPath || ds.name.replace(/^zfsPools\/[^/]+\/datasets\//, '');
        const segs = fn.split('/');
        return {
          id: ds.name,
          name: segs[segs.length - 1],
          fullName: fn,
          poolName: ds.poolName,
          mountpoint: ds.mountpoint,
          usedBytes: ds.usedBytes,
          availableBytes: ds.availableBytes,
          quotaBytes: ds.quotaBytes,
          reservationBytes: ds.reservationBytes,
          compression: ds.compression || 'lz4',
          recordsize: ds.recordsize || '128k',
          sync: ds.sync,
          atime: ds.atime,
          dedup: ds.dedup,
          snapshotsCount: ds.snapshotsCount,
          lastSnapshot: ds.lastSnapshot,
          depth: ds.isPoolRoot ? 0 : 1,
          isPoolRoot: ds.isPoolRoot,
          raw: ds,
          children: [],
        };
      });
  }
  return flattenTree(hierarchyTree.value);
});

// Column sorting (in tree mode siblings are sorted, keeping children under their parent)
const datasetSort = useTableSort(visibleNodes, {
  name: (n) => n.name,
  pool: (n) => n.poolName,
  usage: (n) => n.usedBytes,
  snapshots: (n) => n.snapshotsCount || 0,
});

// Child node id -> parent node id, derived from the assembled tree
const datasetParentIds = computed(() => {
  const map = new Map<string, string>();
  const walk = (nodes: DatasetNode[]) => {
    for (const n of nodes) {
      for (const c of n.children) map.set(c.id, n.id);
      walk(n.children);
    }
  };
  walk(hierarchyTree.value);
  return map;
});

const sortedNodes = computed<DatasetNode[]>(() => {
  if (!datasetSort.key.value) return visibleNodes.value;
  if (viewMode.value === 'flat' || searchQuery.value.trim()) return datasetSort.sorted.value;
  // Collapsed descendants are already excluded from visibleNodes
  return sortHierarchy(
    visibleNodes.value,
    (n) => n.id,
    (n) => datasetParentIds.value.get(n.id),
    datasetSort.compare
  );
});

function toggleCollapse(nodeId: string) {
  if (collapsedPaths.value.has(nodeId)) {
    collapsedPaths.value.delete(nodeId);
  } else {
    collapsedPaths.value.add(nodeId);
  }
}

// Open Create Dataset Dialog
function openCreateModal(parentPath?: string) {
  const defaultPool = nasState.pools[0]?.name || '';
  datasetForm.parentPoolOrDataset = parentPath || (selectedPool.value !== 'all' ? selectedPool.value : defaultPool);
  datasetForm.datasetId = '';
  datasetForm.mountpoint = '';
  datasetForm.compression = 'lz4';
  datasetForm.quotaGib = 0;
  datasetForm.reservationGib = 0;
  datasetForm.sync = 'standard';
  datasetForm.atime = 'on';
  datasetForm.dedup = false;
  showCreateDataset.value = true;
}

async function handleCreateDataset() {
  try {
    let rawParent = datasetForm.parentPoolOrDataset.trim();
    rawParent = rawParent.replace(/^zfsPools\//, '').replace(/\/datasets\/.*/, '');
    const slashIdx = rawParent.indexOf('/');
    const poolName = slashIdx === -1 ? rawParent : rawParent.substring(0, slashIdx);
    const parentPrefix = slashIdx === -1 ? '' : rawParent.substring(slashIdx + 1);

    let fullDatasetId = datasetForm.datasetId.trim();
    if (parentPrefix) {
      fullDatasetId = `${parentPrefix}/${fullDatasetId}`;
    }

    const payload: Partial<ZfsDataset> = {
      compression: datasetForm.compression,
      quotaBytes: gibToBytes(datasetForm.quotaGib),
      reservationBytes: gibToBytes(datasetForm.reservationGib),
      sync: datasetForm.sync,
      atime: datasetForm.atime === 'on',
      dedup: datasetForm.dedup,
    };
    if (datasetForm.mountpoint) payload.mountpoint = datasetForm.mountpoint;

    const created = await services.zfs.createDataset(`zfsPools/${poolName}`, fullDatasetId, payload);
    notify('success', `Created dataset ${created.datasetPath || created.name}`);
    showCreateDataset.value = false;
    await refreshZfsSubresources(`zfsPools/${poolName}`);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create dataset');
  }
}

function startEditDataset(ds: ZfsDataset) {
  editingDataset.value = ds;
  editDatasetForm.compression = ds.compression || 'lz4';
  editDatasetForm.quotaGib = bytesToGib(ds.quotaBytes || 0);
  editDatasetForm.reservationGib = bytesToGib(ds.reservationBytes || 0);
  editDatasetForm.sync = ds.sync || 'standard';
  editDatasetForm.atime = ds.atime ? 'on' : 'off';
  editDatasetForm.dedup = Boolean(ds.dedup);
}

async function handleSaveEditDataset() {
  if (!editingDataset.value) return;
  try {
    const updateMask: string[] = ['compression', 'quotaBytes', 'reservationBytes', 'sync', 'atime', 'dedup'];
    const updated = await services.zfs.updateDataset(
      editingDataset.value.name,
      {
        compression: editDatasetForm.compression,
        quotaBytes: gibToBytes(editDatasetForm.quotaGib),
        reservationBytes: gibToBytes(editDatasetForm.reservationGib),
        sync: editDatasetForm.sync,
        atime: editDatasetForm.atime === 'on',
        dedup: editDatasetForm.dedup,
      },
      updateMask
    );
    notify('success', `Updated dataset ${updated.datasetPath || updated.name}`);
    editingDataset.value = null;
    const poolPrefix = updated.name.split('/datasets/')[0];
    await refreshZfsSubresources(poolPrefix);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update dataset');
  }
}

async function handleTakeSnapshot() {
  if (!snapshotModalDataset.value || !snapshotName.value.trim()) return;
  try {
    await services.zfs.snapshotDataset(snapshotModalDataset.value.name, snapshotName.value.trim());
    notify('success', `Created snapshot @${snapshotName.value.trim()}`);
    const poolPrefix = snapshotModalDataset.value.name.split('/datasets/')[0];
    snapshotModalDataset.value = null;
    snapshotName.value = '';
    await refreshZfsSubresources(poolPrefix);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create snapshot');
  }
}

async function handleDeleteDataset(ds: ZfsDataset) {
  const name = ds.name;
  const shortName = ds.datasetPath || name.replace(/^zfsPools\/[^/]+\/datasets\//, '');
  const res = await confirmDelete({
    kind: 'Dataset',
    name: shortName,
    details: 'All data, child datasets and snapshots under it will be destroyed.',
    confirmText: 'Delete Dataset',
  });
  if (!res.confirmed) return;

  try {
    await services.zfs.deleteDataset(name, true);
    notify('info', `Deleted dataset ${shortName}`);
    const poolPrefix = name.split('/datasets/')[0];
    await refreshZfsSubresources(poolPrefix);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete dataset');
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
    <!-- Header Card with "New Dataset" Button -->
    <div class="card" style="padding: 18px 20px">
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 14px">
        <div>
          <div style="display: flex; align-items: center; gap: 10px">
            <h2 class="card-title" style="font-size: 19px; margin: 0">Datasets</h2>
            <span class="badge badge-info" style="font-size: 12px">
              {{ allDatasets.length }} {{ allDatasets.length === 1 ? 'Dataset' : 'Datasets' }}
            </span>
          </div>
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

          <!-- View Mode Toggle -->
          <div class="view-mode-toggle">
            <button
              class="toggle-btn"
              :class="{ active: viewMode === 'tree' }"
              @click="viewMode = 'tree'"
            >
              <MdiIcon :path="mdiTreeOutline" size="14" />
              Tree
            </button>
            <button
              class="toggle-btn"
              :class="{ active: viewMode === 'flat' }"
              @click="viewMode = 'flat'"
            >
              <MdiIcon :path="mdiFormatListBulleted" size="14" />
              Flat
            </button>
          </div>

          <!-- Search Input -->
          <div class="search-input-wrap">
            <MdiIcon :path="mdiMagnify" size="16" class="search-icon" />
            <input
              v-model="searchQuery"
              class="input search-input"
              placeholder="Search datasets..."
            />
          </div>

          <button class="btn btn-sm" :disabled="nasState.loading" @click="refreshAllData">
            <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" />
            Refresh
          </button>

          <!-- Prominent "New Dataset" Button -->
          <button class="btn btn-primary btn-sm" @click="openCreateModal()">
            <MdiIcon :path="mdiPlus" size="16" style="margin-right: 4px" />
            New Dataset
          </button>
        </div>
      </div>
    </div>

    <!-- Datasets Table Card -->
    <div class="card" style="padding: 18px 20px">
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <SortableTh :sort="datasetSort" sort-key="name" style="min-width: 260px">Name</SortableTh>
              <SortableTh :sort="datasetSort" sort-key="pool" style="min-width: 100px">Pool</SortableTh>
              <SortableTh :sort="datasetSort" sort-key="usage" style="min-width: 150px">Space Usage</SortableTh>
              <th style="min-width: 210px">Properties</th>
              <SortableTh :sort="datasetSort" sort-key="snapshots" style="min-width: 110px">Snapshots</SortableTh>
              <th style="min-width: 150px; text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <!-- Moving Skeleton Placeholders when Loading -->
            <template v-if="nasState.loading && visibleNodes.length === 0">
              <tr v-for="r in 5" :key="r">
                <td>
                  <div style="display: flex; align-items: center; gap: 8px" :style="{ paddingLeft: `${(r - 1) % 3 * 16}px` }">
                    <SkeletonBox width="16px" height="16px" rounded="3px" />
                    <SkeletonBox :width="`${120 + ((r * 25) % 60)}px`" height="15px" />
                  </div>
                </td>
                <td><SkeletonBox width="60px" height="20px" rounded="10px" /></td>
                <td><SkeletonBox width="90px" height="14px" /></td>
                <td><SkeletonBox width="110px" height="14px" /></td>
                <td><SkeletonBox width="45px" height="20px" rounded="10px" /></td>
                <td style="text-align: right">
                  <div style="display: inline-flex; gap: 6px; justify-content: flex-end">
                    <SkeletonBox width="26px" height="26px" rounded="4px" />
                    <SkeletonBox width="26px" height="26px" rounded="4px" />
                    <SkeletonBox width="26px" height="26px" rounded="4px" />
                  </div>
                </td>
              </tr>
            </template>

            <tr v-else-if="visibleNodes.length === 0">
              <td colspan="6" style="text-align: center; color: var(--text-secondary); padding: 36px">
                <div v-if="searchQuery">
                  No datasets match filter "<strong>{{ searchQuery }}</strong>".
                </div>
                <div v-else>
                  No datasets.
                </div>
              </td>
            </tr>

            <tr v-for="node in sortedNodes" :key="node.id" class="tree-row">
              <!-- Name & Hierarchy -->
              <td>
                <div
                  class="tree-cell"
                  :style="{ paddingLeft: viewMode === 'tree' && !searchQuery ? `${node.depth * 20}px` : '0px' }"
                >
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

                  <MdiIcon
                    :path="node.isPoolRoot ? mdiDatabase : mdiFolderOutline"
                    size="16"
                    :style="{ color: 'var(--accent-primary)', flexShrink: 0 }"
                  />

                  <div style="min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
                    <span class="mono font-semibold" style="font-size: 12px" :title="node.fullName">
                      {{ node.name }}
                    </span>
                    <span v-if="node.isPoolRoot" class="badge badge-info mono" style="font-size: 10px; margin-left: 6px">
                      zpool
                    </span>
                    <span v-if="viewMode === 'flat' && !node.isPoolRoot" class="card-subtitle mono" style="font-size: 11px; margin-left: 6px">
                      ({{ node.fullName }})
                    </span>
                  </div>
                </div>
              </td>

              <!-- Pool -->
              <td>
                <span class="badge mono badge-neutral" style="font-size: 11px">
                  {{ node.poolName }}
                </span>
              </td>

              <!-- Space Usage & Quotas -->
              <td>
                <div>
                  <div class="mono" style="font-weight: 700; font-size: 12px">{{ formatBytes(node.usedBytes) }}</div>
                  <div style="font-size: 11px; color: var(--text-secondary); margin-top: 1px">
                    Quota: {{ node.quotaBytes && node.quotaBytes > 0 ? formatBytes(node.quotaBytes) : 'Unlimited' }}
                    <span v-if="node.reservationBytes && node.reservationBytes > 0" style="margin-left: 4px; color: var(--accent-primary)">
                      | Res: {{ formatBytes(node.reservationBytes) }}
                    </span>
                  </div>
                </div>
              </td>

              <!-- Properties -->
              <td>
                <div style="display: flex; gap: 4px; align-items: center; flex-wrap: wrap">
                  <span class="badge badge-info mono" style="font-size: 10px">{{ node.compression }}</span>
                  <span class="badge badge-neutral mono" style="font-size: 10px">{{ node.recordsize }}</span>
                  <span
                    v-if="node.sync && node.sync !== 'standard'"
                    class="badge mono"
                    :class="node.sync === 'always' ? 'badge-success' : 'badge-danger'"
                    style="font-size: 10px"
                  >
                    sync:{{ node.sync }}
                  </span>
                  <span v-else class="badge badge-neutral mono" style="font-size: 10px; opacity: 0.75">
                    sync:std
                  </span>
                  <span v-if="node.atime === false" class="badge badge-warning mono" style="font-size: 10px">noatime</span>
                  <span v-if="node.dedup" class="badge badge-purple mono" style="font-size: 10px">Dedup</span>
                </div>
              </td>

              <!-- Snapshots -->
              <td>
                <div class="mono" style="font-size: 12px">{{ node.snapshotsCount || 0 }} snap{{ node.snapshotsCount === 1 ? '' : 's' }}</div>
                <div v-if="node.lastSnapshot" class="mono" style="font-size: 10.5px; color: var(--success-text); margin-top: 2px" :title="node.lastSnapshot">
                  {{ node.lastSnapshot.split('@')[1] || node.lastSnapshot }}
                </div>
              </td>

              <!-- Actions -->
              <td style="text-align: right">
                <div style="display: inline-flex; gap: 4px; justify-content: flex-end">
                  <button
                    class="btn btn-sm btn-icon"
                    title="Create child dataset under this folder"
                    @click="openCreateModal(node.fullName)"
                  >
                    <MdiIcon :path="mdiPlus" size="13" />
                  </button>
                  <button
                    class="btn btn-sm btn-icon"
                    title="Edit Properties"
                    @click="startEditDataset(node.raw)"
                  >
                    <MdiIcon :path="mdiPencil" size="13" />
                  </button>
                  <button
                    class="btn btn-sm btn-icon"
                    title="Take Snapshot"
                    @click="snapshotModalDataset = node.raw; snapshotName = ''"
                  >
                    <MdiIcon :path="mdiCamera" size="13" />
                  </button>
                  <button
                    v-if="!node.isPoolRoot"
                    class="btn btn-danger btn-sm btn-icon"
                    title="Delete Dataset"
                    @click="handleDeleteDataset(node.raw)"
                  >
                    <MdiIcon :path="mdiDelete" size="13" />
                  </button>
                  <button
                    v-else
                    class="btn btn-sm btn-icon disabled"
                    disabled
                    title="Root zpool dataset cannot be deleted directly"
                    style="opacity: 0.3; cursor: not-allowed"
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

    <!-- Create Dataset Modal -->
    <div v-if="showCreateDataset" class="modal-overlay">
      <div class="modal-container" style="max-width: 580px; width: 100%">
        <div class="modal-header">
          <div style="display: flex; align-items: center; gap: 10px">
            <h3 class="card-title" style="margin: 0">Create Dataset</h3>
          </div>
          <button class="btn btn-sm btn-icon" @click="showCreateDataset = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>

        <form @submit.prevent="handleCreateDataset">
          <div class="modal-body" style="display: flex; flex-direction: column; gap: 14px">
            <div class="form-group">
              <label>Parent Pool or Dataset</label>
              <input v-model="datasetForm.parentPoolOrDataset" class="input mono" placeholder="tank or tank/ds1" required />
            </div>

            <div class="form-group">
              <label>Dataset Name (Folder Name)</label>
              <input v-model="datasetForm.datasetId" class="input mono" placeholder="data or projects" required />
            </div>

            <div class="form-group">
              <label>Custom Mountpoint (Optional)</label>
              <PathSelector
                v-model="datasetForm.mountpoint"
                placeholder="/mnt/data"
                root-hint="/mnt"
              />
            </div>

            <div class="form-grid">
              <div class="form-group">
                <label>Compression</label>
                <select v-model="datasetForm.compression" class="select">
                  <option value="lz4">lz4 (Fast, recommended)</option>
                  <option value="zstd">zstd</option>
                  <option value="gzip">gzip</option>
                  <option value="off">off</option>
                </select>
              </div>

              <div class="form-group">
                <label>Sync Mode</label>
                <select v-model="datasetForm.sync" class="select">
                  <option value="standard">standard (Default)</option>
                  <option value="always">always (Force sync writes)</option>
                  <option value="disabled">disabled (Async high throughput)</option>
                </select>
              </div>
            </div>

            <div class="form-grid">
              <div class="form-group">
                <label>Quota (GiB, 0 = unlimited)</label>
                <input v-model.number="datasetForm.quotaGib" type="number" min="0" class="input" />
              </div>

              <div class="form-group">
                <label>Reservation (GiB, 0 = none)</label>
                <input v-model.number="datasetForm.reservationGib" type="number" min="0" class="input" />
              </div>
            </div>

            <div class="form-grid">
              <div class="form-group">
                <label>Access Time (Atime)</label>
                <select v-model="datasetForm.atime" class="select">
                  <option value="on">on (Update access time on reads)</option>
                  <option value="off">off (Faster performance, recommended)</option>
                </select>
              </div>

              <div class="form-group" style="justify-content: center">
                <label style="display: flex; align-items: center; gap: 8px; cursor: pointer; margin-top: 18px">
                  <input v-model="datasetForm.dedup" type="checkbox" />
                  <span>Enable Deduplication</span>
                </label>
              </div>
            </div>
          </div>

          <div style="padding: 14px 20px; border-top: 1px solid var(--border-subtle); display: flex; justify-content: flex-end; gap: 8px">
            <button type="button" class="btn btn-sm" @click="showCreateDataset = false">Cancel</button>
            <button type="submit" class="btn btn-primary btn-sm">Create Dataset</button>
          </div>
        </form>
      </div>
    </div>

    <!-- Edit Dataset Modal -->
    <div v-if="editingDataset" class="modal-overlay">
      <div class="modal-container" style="max-width: 540px; width: 100%">
        <div class="modal-header">
          <div style="display: flex; align-items: center; gap: 10px">
            <MdiIcon :path="mdiPencil" size="18" style="color: var(--accent-primary)" />
            <h3 class="card-title" style="margin: 0">
              Edit Dataset: <span class="mono">{{ editingDataset.datasetPath || editingDataset.name }}</span>
            </h3>
          </div>
          <button class="btn btn-sm btn-icon" @click="editingDataset = null">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>

        <form @submit.prevent="handleSaveEditDataset">
          <div class="modal-body" style="display: flex; flex-direction: column; gap: 14px">
            <div v-if="editingDataset.mountpoint" style="font-size: 12px; color: var(--text-secondary)">
              Mountpoint: <span class="mono" style="color: var(--text-primary)">{{ editingDataset.mountpoint }}</span>
            </div>
            <div class="form-grid">
              <div class="form-group">
                <label>Compression</label>
                <select v-model="editDatasetForm.compression" class="select">
                  <option value="lz4">lz4</option>
                  <option value="zstd">zstd</option>
                  <option value="gzip">gzip</option>
                  <option value="off">off</option>
                </select>
              </div>

              <div class="form-group">
                <label>Sync Mode</label>
                <select v-model="editDatasetForm.sync" class="select">
                  <option value="standard">standard</option>
                  <option value="always">always</option>
                  <option value="disabled">disabled</option>
                </select>
              </div>
            </div>

            <div class="form-grid">
              <div class="form-group">
                <label>Quota (GiB, 0 = unlimited)</label>
                <input v-model.number="editDatasetForm.quotaGib" type="number" min="0" class="input" />
              </div>

              <div class="form-group">
                <label>Reservation (GiB, 0 = none)</label>
                <input v-model.number="editDatasetForm.reservationGib" type="number" min="0" class="input" />
              </div>
            </div>

            <div class="form-grid">
              <div class="form-group">
                <label>Access Time (Atime)</label>
                <select v-model="editDatasetForm.atime" class="select">
                  <option value="on">on</option>
                  <option value="off">off</option>
                </select>
              </div>

              <div class="form-group" style="justify-content: center">
                <label style="display: flex; align-items: center; gap: 8px; cursor: pointer; margin-top: 18px">
                  <input v-model="editDatasetForm.dedup" type="checkbox" />
                  <span>Enable Deduplication</span>
                </label>
              </div>
            </div>
          </div>

          <div style="padding: 14px 20px; border-top: 1px solid var(--border-subtle); display: flex; justify-content: flex-end; gap: 8px">
            <button type="button" class="btn btn-sm" @click="editingDataset = null">Cancel</button>
            <button type="submit" class="btn btn-primary btn-sm">Save Changes</button>
          </div>
        </form>
      </div>
    </div>

    <!-- Take Snapshot Modal -->
    <div v-if="snapshotModalDataset" class="modal-overlay">
      <div class="modal-container" style="max-width: 440px; width: 100%">
        <div class="modal-header">
          <div style="display: flex; align-items: center; gap: 8px">
            <MdiIcon :path="mdiCamera" size="18" style="color: var(--success-text)" />
            <h3 class="card-title" style="margin: 0">Take Snapshot</h3>
          </div>
          <button class="btn btn-sm btn-icon" @click="snapshotModalDataset = null">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>

        <form @submit.prevent="handleTakeSnapshot">
          <div class="modal-body" style="display: flex; flex-direction: column; gap: 12px">
            <div class="card-subtitle mono" style="font-size: 12px">
              Target: {{ snapshotModalDataset.datasetPath || snapshotModalDataset.name }}
            </div>
            <div class="form-group">
              <label>Snapshot Name</label>
              <input v-model="snapshotName" class="input mono" placeholder="backup-2026-10-25" required />
            </div>
          </div>

          <div style="padding: 14px 20px; border-top: 1px solid var(--border-subtle); display: flex; justify-content: flex-end; gap: 8px">
            <button type="button" class="btn btn-sm" @click="snapshotModalDataset = null">Cancel</button>
            <button type="submit" class="btn btn-success btn-sm">Take Snapshot</button>
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
