<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import {
  mdiChevronDown,
  mdiChevronRight,
  mdiClose,
  mdiDatabase,
  mdiDelete,
  mdiFormatListBulleted,
  mdiHarddisk,
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
  type ZfsVolume,
} from '../../domain/models';
import MdiIcon from '../components/MdiIcon.vue';
import SkeletonBox from '../components/SkeletonBox.vue';
import SortableTh from '../components/SortableTh.vue';
import { sortHierarchy, useTableSort } from '../composables/useTableSort';
import { confirmDelete } from '../modal';
import { nasState, notify, refreshAllData, refreshZfsSubresources } from '../store';

const selectedPool = ref<string>('all');
const searchQuery = ref('');
const viewMode = ref<'tree' | 'flat'>('tree');
const expandedPaths = ref<Set<string>>(new Set());

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
    if (selectedPool.value !== 'all' && selectedPool.value !== pName && selectedPool.value !== shortName) {
      continue;
    }
    const vols = nasState.volumesByPool[pName] || [];
    for (const v of vols) {
      result.push({ ...v, poolName: shortName });
    }
  }
  return result;
});

// Tree Hierarchy Node for Volumes View
export interface VolumeNode {
  id: string;
  name: string;
  fullName: string;
  poolName: string;
  devicePath: string;
  sizeBytes: number;
  usedBytes: number;
  volblocksize: string;
  thinProvisioned: boolean;
  compression: string;
  sync?: string;
  readonly?: boolean;
  dedup?: boolean;
  depth: number;
  isPoolRoot: boolean;
  poolState?: string;
  raw?: ZfsVolume;
  children: VolumeNode[];
}

const hierarchyTree = computed<VolumeNode[]>(() => {
  const rootNodes: VolumeNode[] = [];

  for (const pool of nasState.pools) {
    const pName = pool.name;
    const shortName = pool.poolName || pName.replace(/^zfsPools\//, '');
    if (selectedPool.value !== 'all' && selectedPool.value !== pName && selectedPool.value !== shortName) {
      continue;
    }

    const poolNode: VolumeNode = {
      id: pool.name,
      name: shortName,
      fullName: shortName,
      poolName: shortName,
      devicePath: '',
      sizeBytes: pool.sizeBytes || 0,
      usedBytes: pool.allocatedBytes || 0,
      volblocksize: '',
      thinProvisioned: false,
      compression: (pool.compression as string) || 'lz4',
      sync: pool.sync || 'standard',
      readonly: false,
      dedup: false,
      depth: 0,
      isPoolRoot: true,
      poolState: pool.state || 'ONLINE',
      children: [],
    };

    const vols = nasState.volumesByPool[pName] || [];
    for (const v of vols) {
      const fullName = v.volumePath || v.name.replace(/^zfsPools\/[^/]+\/volumes\//, '');
      let leafName = fullName;
      if (leafName.startsWith(shortName + '/')) {
        leafName = leafName.substring(shortName.length + 1);
      }
      const childNode: VolumeNode = {
        id: v.name,
        name: leafName,
        fullName,
        poolName: shortName,
        devicePath: v.devicePath || '',
        sizeBytes: v.sizeBytes || 0,
        usedBytes: v.usedBytes || 0,
        volblocksize: v.volblocksize || '16k',
        thinProvisioned: Boolean(v.thinProvisioned),
        compression: (v.compression as string) || 'lz4',
        sync: v.sync || 'standard',
        readonly: Boolean(v.readonly),
        dedup: Boolean(v.dedup),
        depth: 1,
        isPoolRoot: false,
        raw: v,
        children: [],
      };
      poolNode.children.push(childNode);
    }

    // Sort child volumes alphabetically by fullName/name
    poolNode.children.sort((a, b) => a.fullName.localeCompare(b.fullName));
    rootNodes.push(poolNode);
  }

  // Sort pool roots alphabetically by name
  rootNodes.sort((a, b) => a.name.localeCompare(b.name));
  return rootNodes;
});

function flattenTree(nodes: VolumeNode[], result: VolumeNode[] = []): VolumeNode[] {
  for (const node of nodes) {
    result.push(node);
    if (expandedPaths.value.has(node.id) && node.children.length > 0) {
      flattenTree(node.children, result);
    }
  }
  return result;
}

const visibleNodes = computed<VolumeNode[]>(() => {
  const q = searchQuery.value.trim().toLowerCase();
  if (viewMode.value === 'flat' || q) {
    return allVolumes.value
      .filter((v) => {
        if (!q) return true;
        const fn = v.volumePath || v.name;
        return (
          fn.toLowerCase().includes(q) ||
          v.devicePath.toLowerCase().includes(q) ||
          v.poolName.toLowerCase().includes(q)
        );
      })
      .map((v) => {
        const fullName = v.volumePath || v.name.replace(/^zfsPools\/[^/]+\/volumes\//, '');
        let leafName = fullName;
        if (leafName.startsWith(v.poolName + '/')) {
          leafName = leafName.substring(v.poolName.length + 1);
        }
        return {
          id: v.name,
          name: leafName,
          fullName,
          poolName: v.poolName,
          devicePath: v.devicePath || '',
          sizeBytes: v.sizeBytes || 0,
          usedBytes: v.usedBytes || 0,
          volblocksize: v.volblocksize || '16k',
          thinProvisioned: Boolean(v.thinProvisioned),
          compression: (v.compression as string) || 'lz4',
          sync: v.sync || 'standard',
          readonly: Boolean(v.readonly),
          dedup: Boolean(v.dedup),
          depth: 0,
          isPoolRoot: false,
          raw: v,
          children: [],
        };
      });
  }
  return flattenTree(hierarchyTree.value);
});

/** Converts a ZFS block size string such as "16k" / "1M" into bytes for sorting. */
function blockSizeBytes(s: string | undefined): number | null {
  const m = /^(\d+(?:\.\d+)?)\s*([kmg]?)/i.exec((s || '16k').trim());
  if (!m) return null;
  const mult = { '': 1, k: 1024, m: 1024 ** 2, g: 1024 ** 3 }[m[2].toLowerCase() as '' | 'k' | 'm' | 'g'];
  return parseFloat(m[1]) * mult;
}

const volumeSort = useTableSort(visibleNodes, {
  name: (n) => n.name,
  pool: (n) => n.poolName,
  capacity: (n) => n.sizeBytes,
  blockSize: (n) => (n.isPoolRoot ? null : blockSizeBytes(n.volblocksize)),
  provisioning: (n) => (n.isPoolRoot ? n.poolState : n.thinProvisioned ? 'Thin' : 'Thick'),
});

// Child node id -> parent pool id derived from hierarchyTree
const volumeParentIds = computed(() => {
  const map = new Map<string, string>();
  const walk = (nodes: VolumeNode[]) => {
    for (const n of nodes) {
      for (const c of n.children) {
        map.set(c.id, n.id);
      }
      walk(n.children);
    }
  };
  walk(hierarchyTree.value);
  return map;
});

const sortedNodes = computed<VolumeNode[]>(() => {
  if (!volumeSort.key.value) return visibleNodes.value;
  if (viewMode.value === 'flat' || searchQuery.value.trim()) return volumeSort.sorted.value;
  return sortHierarchy(
    visibleNodes.value,
    (n) => n.id,
    (n) => volumeParentIds.value.get(n.id),
    volumeSort.compare
  );
});

function toggleCollapse(nodeId: string) {
  if (expandedPaths.value.has(nodeId)) {
    expandedPaths.value.delete(nodeId);
  } else {
    expandedPaths.value.add(nodeId);
  }
}

function expandAll() {
  const set = new Set<string>();
  const collect = (nodes: VolumeNode[]) => {
    for (const n of nodes) {
      if (n.children.length > 0) {
        set.add(n.id);
        collect(n.children);
      }
    }
  };
  collect(hierarchyTree.value);
  expandedPaths.value = set;
}

function collapseAll() {
  expandedPaths.value.clear();
}

function openCreateModal(targetPoolName?: string) {
  const defaultPool = nasState.pools[0]?.name || '';
  if (targetPoolName) {
    const matched = nasState.pools.find(
      (p) => p.name === targetPoolName || p.poolName === targetPoolName || p.name.replace(/^zfsPools\//, '') === targetPoolName
    );
    volumeForm.poolName = matched ? matched.name : (targetPoolName.startsWith('zfsPools/') ? targetPoolName : `zfsPools/${targetPoolName}`);
  } else {
    volumeForm.poolName = selectedPool.value !== 'all' ? selectedPool.value : defaultPool;
  }
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

async function handleDeleteVolume(vol: ZfsVolume) {
  const name = vol.name;
  const shortName = vol.volumePath || name.replace(/^zfsPools\/[^/]+\/volumes\//, '');
  const res = await confirmDelete({
    kind: 'Volume',
    name: shortName,
    details: 'All data on this volume will be destroyed.',
    confirmText: 'Delete Volume',
  });
  if (!res.confirmed) return;

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
    <!-- Header Card with Controls and "New Volume" Button -->
    <div class="card" style="padding: 18px 20px">
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 14px">
        <div>
          <div style="display: flex; align-items: center; gap: 10px">
            <h2 class="card-title" style="font-size: 19px; margin: 0">Volumes</h2>
            <span class="badge badge-purple" style="font-size: 12px">
              {{ allVolumes.length }} {{ allVolumes.length === 1 ? 'Volume' : 'Volumes' }}
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

          <!-- Tree Expand / Collapse All Controls -->
          <div v-if="viewMode === 'tree' && !searchQuery" class="tree-expand-controls">
            <button
              class="btn btn-sm btn-icon"
              title="Expand All"
              aria-label="Expand All"
              @click="expandAll"
            >
              <MdiIcon :path="mdiChevronDown" size="16" />
            </button>
            <button
              class="btn btn-sm btn-icon"
              title="Collapse All"
              aria-label="Collapse All"
              @click="collapseAll"
            >
              <MdiIcon :path="mdiChevronRight" size="16" />
            </button>
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
              <SortableTh :sort="volumeSort" sort-key="name" style="min-width: 200px">Volume Name</SortableTh>
              <SortableTh :sort="volumeSort" sort-key="pool" style="min-width: 90px">Pool</SortableTh>
              <SortableTh :sort="volumeSort" sort-key="capacity" style="min-width: 140px">Capacity & Usage</SortableTh>
              <SortableTh :sort="volumeSort" sort-key="blockSize" class="col-desktop-only">Block Size</SortableTh>
              <SortableTh :sort="volumeSort" sort-key="provisioning" class="col-desktop-only">Provisioning</SortableTh>
              <th class="col-desktop-only">Properties</th>
              <th style="min-width: 110px; text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <!-- Moving Skeleton Placeholders when Loading -->
            <template v-if="nasState.loading && visibleNodes.length === 0">
              <tr v-for="r in 4" :key="r">
                <td>
                  <div style="display: flex; align-items: center; gap: 8px">
                    <SkeletonBox width="18px" height="18px" rounded="3px" />
                    <SkeletonBox width="130px" height="15px" />
                  </div>
                </td>
                <td><SkeletonBox width="60px" height="20px" rounded="10px" /></td>
                <td><SkeletonBox width="90px" height="14px" /></td>
                <td class="col-desktop-only"><SkeletonBox width="60px" height="14px" /></td>
                <td class="col-desktop-only"><SkeletonBox width="70px" height="20px" rounded="10px" /></td>
                <td class="col-desktop-only"><SkeletonBox width="100px" height="14px" /></td>
                <td style="text-align: right">
                  <div style="display: inline-flex; gap: 6px; justify-content: flex-end">
                    <SkeletonBox width="26px" height="26px" rounded="4px" />
                    <SkeletonBox width="26px" height="26px" rounded="4px" />
                  </div>
                </td>
              </tr>
            </template>

            <tr v-else-if="visibleNodes.length === 0">
              <td colspan="7" style="text-align: center; color: var(--text-secondary); padding: 36px">
                <div v-if="searchQuery">
                  No volumes match filter "<strong>{{ searchQuery }}</strong>".
                </div>
                <div v-else>
                  No volumes.
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
                    :title="expandedPaths.has(node.id) ? 'Collapse' : 'Expand'"
                    :aria-label="expandedPaths.has(node.id) ? 'Collapse' : 'Expand'"
                    @click="toggleCollapse(node.id)"
                  >
                    <MdiIcon
                      :path="expandedPaths.has(node.id) ? mdiChevronDown : mdiChevronRight"
                      size="16"
                    />
                  </button>
                  <span v-else-if="viewMode === 'tree' && !searchQuery" class="chevron-spacer"></span>

                  <MdiIcon
                    :path="node.isPoolRoot ? mdiDatabase : mdiHarddisk"
                    size="18"
                    :style="{ color: node.isPoolRoot ? 'var(--accent-primary)' : 'var(--purple-text)', flexShrink: 0 }"
                  />

                  <div style="min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
                    <span class="mono font-semibold" style="font-size: 12px; color: var(--text-primary)" :title="node.fullName">
                      {{ node.name }}
                    </span>
                    <span v-if="node.isPoolRoot" class="badge badge-info mono" style="font-size: 10px; margin-left: 6px">
                      zpool
                    </span>
                    <span v-if="node.isPoolRoot" class="card-subtitle mono" style="font-size: 11px; margin-left: 6px">
                      ({{ node.children.length }} {{ node.children.length === 1 ? 'volume' : 'volumes' }})
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

              <!-- Capacity & Used -->
              <td>
                <div v-if="node.isPoolRoot">
                  <div class="mono" style="font-weight: 700; font-size: 12px">
                    {{ formatBytes(node.sizeBytes) }}
                  </div>
                  <div style="font-size: 11px; color: var(--text-secondary); margin-top: 1px">
                    Allocated: {{ formatBytes(node.usedBytes) }}
                  </div>
                </div>
                <div v-else>
                  <div class="mono" style="font-weight: 700; font-size: 12px">
                    {{ formatBytes(node.sizeBytes) }}
                  </div>
                  <div style="font-size: 11px; color: var(--text-secondary); margin-top: 1px">
                    Used: {{ formatBytes(node.usedBytes) }}
                  </div>
                </div>
              </td>

              <!-- Block Size -->
              <td class="col-desktop-only">
                <span v-if="!node.isPoolRoot" class="badge mono badge-neutral" style="font-size: 10.5px">
                  {{ node.volblocksize || '16k' }}
                </span>
                <span v-else style="color: var(--text-secondary); font-size: 11px">—</span>
              </td>

              <!-- Provisioning -->
              <td class="col-desktop-only">
                <span
                  v-if="node.isPoolRoot"
                  class="badge mono"
                  :class="node.poolState === 'ONLINE' ? 'badge-success' : 'badge-danger'"
                  style="font-size: 10.5px"
                >
                  {{ node.poolState }}
                </span>
                <span
                  v-else
                  class="badge mono"
                  :class="node.thinProvisioned ? 'badge-success' : 'badge-neutral'"
                  style="font-size: 10.5px"
                >
                  {{ node.thinProvisioned ? 'Thin' : 'Thick' }}
                </span>
              </td>

              <!-- Properties -->
              <td class="col-desktop-only">
                <div v-if="node.isPoolRoot" style="display: flex; gap: 4px; align-items: center; flex-wrap: wrap">
                  <span class="badge badge-info mono" style="font-size: 10px">{{ node.compression || 'lz4' }}</span>
                </div>
                <div v-else style="display: flex; gap: 4px; align-items: center; flex-wrap: wrap">
                  <span class="badge badge-info mono" style="font-size: 10px">{{ node.compression || 'lz4' }}</span>
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
                  <span v-if="node.readonly" class="badge badge-warning mono" style="font-size: 10px">RO</span>
                  <span v-if="node.dedup" class="badge badge-purple mono" style="font-size: 10px">Dedup</span>
                </div>
              </td>

              <!-- Actions -->
              <td style="text-align: right">
                <div v-if="node.isPoolRoot" style="display: inline-flex; gap: 6px; justify-content: flex-end">
                  <button
                    class="btn btn-sm"
                    title="Create Volume in this Pool"
                    @click="openCreateModal(node.poolName)"
                  >
                    <MdiIcon :path="mdiPlus" size="13" style="margin-right: 4px" />
                    New Vol
                  </button>
                </div>
                <div v-else-if="node.raw" style="display: inline-flex; gap: 6px; justify-content: flex-end">
                  <button
                    class="btn btn-sm"
                    title="Edit Settings & Resize Volsize"
                    @click="startEditVolume(node.raw)"
                  >
                    <MdiIcon :path="mdiPencil" size="13" style="margin-right: 4px" />
                    Resize
                  </button>
                  <button
                    class="btn btn-danger btn-sm btn-icon"
                    title="Delete Volume"
                    @click="handleDeleteVolume(node.raw)"
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
    <div v-if="showCreateVolume" class="modal-overlay">
      <div class="modal-container" style="max-width: 580px; width: 100%">
        <div class="modal-header">
          <div style="display: flex; align-items: center; gap: 10px">
            <h3 class="card-title" style="margin: 0">Create Volume</h3>
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
    <div v-if="editingVolume" class="modal-overlay">
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
            <div v-if="editingVolume.devicePath" style="font-size: 12px; color: var(--text-secondary)">
              Device Path: <span class="mono" style="color: var(--text-primary)">{{ editingVolume.devicePath }}</span>
            </div>
            <div class="form-group">
              <label>Provisioned Size (GiB)</label>
              <input v-model.number="editVolumeForm.volsizeGib" type="number" min="1" class="input mono" required />
              <span class="card-subtitle" style="font-size: 11px">
                Current: {{ formatBytes(editingVolume.sizeBytes) }}
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
