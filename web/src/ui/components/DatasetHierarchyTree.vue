<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import {
  mdiArrowCollapseAll,
  mdiArrowExpandAll,
  mdiCheck,
  mdiChevronDown,
  mdiChevronRight,
  mdiClose,
  mdiFolderOpenOutline,
  mdiFolderOutline,
  mdiHarddisk,
  mdiMagnify,
  mdiRefresh,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import { nasState, refreshAllData, refreshZfsSubresources } from '../store';
import { type ZfsDataset, isDotDataset } from '../../domain/models';

export interface DatasetTreeNode {
  id: string;
  name: string; // Short segment name (e.g. "apps" or "tank")
  path: string; // Full ZFS path (e.g. "tank/apps/appdata")
  mountpoint: string; // Filesystem path (e.g. "/nas/tank/apps/appdata")
  poolName: string; // Pool name (e.g. "tank")
  isPoolRoot: boolean;
  usedBytes?: number;
  availableBytes?: number;
  children: DatasetTreeNode[];
  dataset?: ZfsDataset;
}

export interface FlatTreeNode {
  node: DatasetTreeNode;
  depth: number;
  hasChildren: boolean;
  isExpanded: boolean;
}

const props = withDefaults(
  defineProps<{
    modelValue?: string;
    poolFilter?: string; // Specific pool or 'all'
    returnType?: 'path' | 'mountpoint' | 'dataset'; // What value to emit for v-model
    selectablePoolRoot?: boolean;
    title?: string;
    maxHeight?: string;
    autoExpandAll?: boolean;
  }>(),
  {
    modelValue: '',
    poolFilter: 'all',
    returnType: 'path',
    selectablePoolRoot: true,
    title: 'ZFS Dataset Hierarchy',
    maxHeight: '400px',
    autoExpandAll: true,
  }
);

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void;
  (e: 'select', node: DatasetTreeNode): void;
}>();

const searchQuery = ref('');
const isRefreshing = ref(false);
const expandedNodeIds = ref<Set<string>>(new Set());

// Track selected node by path or mountpoint
const selectedIdentifier = ref(props.modelValue || '');

watch(
  () => props.modelValue,
  (val) => {
    selectedIdentifier.value = val || '';
  }
);

// Format human-readable bytes
function formatBytes(bytes?: number): string {
  if (bytes === undefined || bytes === null || bytes === 0) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];
  let val = bytes;
  let idx = 0;
  while (val >= 1024 && idx < units.length - 1) {
    val /= 1024;
    idx++;
  }
  return `${val.toFixed(1)} ${units[idx]}`;
}

// Build list of raw datasets ensuring pool roots exist
const rawDatasetsList = computed(() => {
  const result: Array<ZfsDataset & { poolName: string; isPoolRoot: boolean }> = [];
  const targetPools = nasState.pools.filter((p) => {
    if (props.poolFilter && props.poolFilter !== 'all') {
      const pName = p.poolName || p.name.replace(/^zfsPools\//, '');
      return pName === props.poolFilter;
    }
    return true;
  });

  for (const pool of targetPools) {
    const pName = pool.name;
    const shortName = pool.poolName || pName.replace(/^zfsPools\//, '');
    const dss = nasState.datasetsByPool[pName] || [];

    const poolMount = pool.mountpoint && pool.mountpoint !== 'none'
      ? pool.mountpoint
      : `/nas/${shortName}`;

    // Look for root dataset
    const rootDs = dss.find(
      (d) => d.datasetPath === shortName || d.datasetPath === pool.poolName || d.name.endsWith('/datasets/root')
    );

    if (rootDs) {
      result.push({
        ...rootDs,
        datasetPath: shortName,
        mountpoint: rootDs.mountpoint || poolMount,
        poolName: shortName,
        isPoolRoot: true,
      });
    } else {
      result.push({
        name: `${pName}/datasets/root`,
        datasetPath: shortName,
        mountpoint: poolMount,
        usedBytes: pool.allocatedBytes,
        availableBytes: pool.freeBytes,
        referencedBytes: 0,
        quotaBytes: 0,
        compression: 'lz4',
        readonly: false,
        atime: true,
        recordsize: '128k',
        snapshotsCount: 0,
        lastSnapshot: '',
        poolName: shortName,
        isPoolRoot: true,
      });
    }

    for (const ds of dss) {
      if (ds === rootDs || ds.datasetPath === shortName || ds.name.endsWith('/datasets/root')) {
        continue;
      }
      if (isDotDataset(ds)) {
        continue;
      }
      // Normalize mountpoint if needed
      let mp = ds.mountpoint;
      if (!mp || mp === 'none' || mp === 'legacy') {
        mp = `/nas/${ds.datasetPath}`;
      }
      result.push({
        ...ds,
        mountpoint: mp,
        poolName: shortName,
        isPoolRoot: false,
      });
    }
  }

  return result;
});

// Build the hierarchical tree from the datasets
const hierarchyTree = computed<DatasetTreeNode[]>(() => {
  const nodesMap = new Map<string, DatasetTreeNode>();
  const rootNodes: DatasetTreeNode[] = [];

  for (const ds of rawDatasetsList.value) {
    const fullPath = ds.datasetPath || ds.name.replace(/^zfsPools\/[^/]+\/datasets\//, '');
    const segments = fullPath.split('/');
    const shortName = segments[segments.length - 1];

    const node: DatasetTreeNode = {
      id: ds.name,
      name: shortName,
      path: fullPath,
      mountpoint: ds.mountpoint || `/nas/${fullPath}`,
      poolName: ds.poolName,
      isPoolRoot: ds.isPoolRoot,
      usedBytes: ds.usedBytes,
      availableBytes: ds.availableBytes,
      children: [],
      dataset: ds,
    };

    nodesMap.set(fullPath, node);
  }

  // Link children to parents
  for (const ds of rawDatasetsList.value) {
    const fullPath = ds.datasetPath || ds.name.replace(/^zfsPools\/[^/]+\/datasets\//, '');
    const node = nodesMap.get(fullPath);
    if (!node) continue;

    if (node.isPoolRoot) {
      rootNodes.push(node);
      continue;
    }

    const segments = fullPath.split('/');
    if (segments.length > 1) {
      const parentPath = segments.slice(0, -1).join('/');
      const parent = nodesMap.get(parentPath);
      if (parent) {
        parent.children.push(node);
        continue;
      }
    }

    // Fallback: attach to pool root if parent was not found
    const poolRoot = nodesMap.get(node.poolName);
    if (poolRoot && poolRoot !== node) {
      poolRoot.children.push(node);
    } else {
      rootNodes.push(node);
    }
  }

  // Sort nodes alphabetically, keeping pool roots at top
  function sortTree(nodes: DatasetTreeNode[]) {
    nodes.sort((a, b) => {
      if (a.isPoolRoot && !b.isPoolRoot) return -1;
      if (!a.isPoolRoot && b.isPoolRoot) return 1;
      return a.name.localeCompare(b.name);
    });
    for (const child of nodes) {
      if (child.children.length > 0) {
        sortTree(child.children);
      }
    }
  }

  sortTree(rootNodes);
  return rootNodes;
});

// Watch hierarchyTree to initialize expansion
watch(
  hierarchyTree,
  (tree) => {
    const set = new Set<string>();
    if (props.autoExpandAll) {
      function addIds(nodes: DatasetTreeNode[]) {
        for (const n of nodes) {
          if (n.children.length > 0) {
            set.add(n.id);
            addIds(n.children);
          }
        }
      }
      addIds(tree);
    } else {
      for (const root of tree) {
        set.add(root.id);
      }
    }
    expandedNodeIds.value = set;
  },
  { immediate: true }
);

// Fuzzy Search Algorithm
function fuzzyMatch(pattern: string, text: string): boolean {
  if (!pattern) return true;
  pattern = pattern.toLowerCase();
  text = text.toLowerCase();
  if (text.includes(pattern)) return true;

  let pIdx = 0;
  for (let tIdx = 0; tIdx < text.length && pIdx < pattern.length; tIdx++) {
    if (text[tIdx] === pattern[pIdx]) {
      pIdx++;
    }
  }
  return pIdx === pattern.length;
}

// Determine if a node or any of its descendants matches the search
function nodeMatchesSearch(node: DatasetTreeNode, query: string): boolean {
  if (!query) return true;
  const matchSelf =
    fuzzyMatch(query, node.name) ||
    fuzzyMatch(query, node.path) ||
    fuzzyMatch(query, node.mountpoint) ||
    fuzzyMatch(query, node.poolName);

  if (matchSelf) return true;
  return node.children.some((c) => nodeMatchesSearch(c, query));
}

// Filter the tree based on search query
function filterTree(nodes: DatasetTreeNode[], query: string): DatasetTreeNode[] {
  if (!query) return nodes;
  const result: DatasetTreeNode[] = [];

  for (const node of nodes) {
    if (nodeMatchesSearch(node, query)) {
      const filteredChildren = filterTree(node.children, query);
      result.push({
        ...node,
        children: filteredChildren,
      });
    }
  }

  return result;
}

const filteredTree = computed(() => {
  const q = searchQuery.value.trim();
  const tree = filterTree(hierarchyTree.value, q);

  // Auto-expand all matching parent branches when searching
  if (q) {
    const toExpand = new Set<string>();
    function markExpand(nodes: DatasetTreeNode[]) {
      for (const n of nodes) {
        if (n.children.length > 0) {
          toExpand.add(n.id);
          markExpand(n.children);
        }
      }
    }
    markExpand(tree);
    expandedNodeIds.value = toExpand;
  }

  return tree;
});

// Flattened visible list supporting arbitrary dataset nesting depths
const flattenedVisibleNodes = computed<FlatTreeNode[]>(() => {
  const result: FlatTreeNode[] = [];
  function traverse(nodes: DatasetTreeNode[], depth: number) {
    for (const node of nodes) {
      const isExpanded = expandedNodeIds.value.has(node.id);
      result.push({
        node,
        depth,
        hasChildren: node.children.length > 0,
        isExpanded,
      });
      if (node.children.length > 0 && isExpanded) {
        traverse(node.children, depth + 1);
      }
    }
  }
  traverse(filteredTree.value, 0);
  return result;
});

const totalDatasetsCount = computed(() => rawDatasetsList.value.length);

// Tree Expand / Collapse actions
function toggleNode(nodeId: string) {
  const next = new Set(expandedNodeIds.value);
  if (next.has(nodeId)) {
    next.delete(nodeId);
  } else {
    next.add(nodeId);
  }
  expandedNodeIds.value = next;
}

function expandAll() {
  const set = new Set<string>();
  function addIds(nodes: DatasetTreeNode[]) {
    for (const n of nodes) {
      if (n.children.length > 0) {
        set.add(n.id);
        addIds(n.children);
      }
    }
  }
  addIds(hierarchyTree.value);
  expandedNodeIds.value = set;
}

function collapseAll() {
  expandedNodeIds.value = new Set();
}

// Select a node
function selectNode(node: DatasetTreeNode) {
  if (!props.selectablePoolRoot && node.isPoolRoot) return;

  let valToEmit: string;
  if (props.returnType === 'mountpoint') {
    valToEmit = node.mountpoint;
  } else if (props.returnType === 'dataset') {
    valToEmit = node.id;
  } else {
    valToEmit = node.path;
  }

  selectedIdentifier.value = valToEmit;
  emit('update:modelValue', valToEmit);
  emit('select', node);
}

function isNodeSelected(node: DatasetTreeNode): boolean {
  if (!selectedIdentifier.value) return false;
  return (
    selectedIdentifier.value === node.path ||
    selectedIdentifier.value === node.mountpoint ||
    selectedIdentifier.value === node.id ||
    selectedIdentifier.value === `/${node.path}` ||
    selectedIdentifier.value === node.dataset?.name
  );
}

// Refresh action
async function handleRefresh() {
  if (isRefreshing.value) return;
  isRefreshing.value = true;
  try {
    const pools = nasState.pools;
    await Promise.all([
      refreshAllData().catch(() => {}),
      ...pools.map((p) => refreshZfsSubresources(p.name).catch(() => {})),
    ]);
  } finally {
    isRefreshing.value = false;
  }
}
</script>

<template>
  <div class="dataset-hierarchy-container">
    <!-- Header: Title, Total Badge, Expand/Collapse & Refresh -->
    <div class="dataset-tree-header">
      <div style="display: flex; align-items: center; gap: 8px">
        <MdiIcon :path="mdiFolderOpenOutline" size="18" style="color: var(--accent-primary)" />
        <span style="font-weight: 600; font-size: 13px; color: var(--text-primary)">
          {{ title }}
        </span>
        <span class="badge badge-neutral" style="font-size: 10.5px; padding: 1px 6px">
          {{ totalDatasetsCount }} datasets
        </span>
      </div>

      <div style="display: flex; align-items: center; gap: 4px">
        <button
          type="button"
          class="btn btn-sm btn-icon"
          title="Expand All Folders"
          aria-label="Expand All Folders"
          @click="expandAll"
        >
          <MdiIcon :path="mdiArrowExpandAll" size="14" />
        </button>

        <button
          type="button"
          class="btn btn-sm btn-icon"
          title="Collapse All Folders"
          aria-label="Collapse All Folders"
          @click="collapseAll"
        >
          <MdiIcon :path="mdiArrowCollapseAll" size="14" />
        </button>

        <button
          type="button"
          class="btn btn-sm btn-icon"
          :class="{ 'spin-icon': isRefreshing }"
          :disabled="isRefreshing"
          title="Refresh Datasets"
          aria-label="Refresh Datasets"
          @click="handleRefresh"
        >
          <MdiIcon :path="mdiRefresh" size="15" />
        </button>
      </div>
    </div>

    <!-- Search Bar with Fuzzy Matching -->
    <div class="dataset-search-bar">
      <MdiIcon :path="mdiMagnify" size="16" style="color: var(--text-secondary); flex-shrink: 0" />
      <input
        v-model="searchQuery"
        type="text"
        class="dataset-search-input"
        placeholder="Fuzzy search datasets, paths, or mountpoints (e.g. apps, media, tank/doc)..."
      />
      <button
        v-if="searchQuery"
        type="button"
        class="clear-search-btn"
        title="Clear Search"
        @click="searchQuery = ''"
      >
        <MdiIcon :path="mdiClose" size="14" />
      </button>
    </div>

    <!-- Hierarchy Tree Viewport -->
    <div class="dataset-tree-viewport" :style="{ maxHeight }">
      <div v-if="flattenedVisibleNodes.length === 0" class="empty-tree-state">
        <div v-if="searchQuery">
          No datasets matching "<span class="mono" style="color: var(--accent-primary)">{{ searchQuery }}</span>"
        </div>
        <div v-else>
          No ZFS pools or datasets found on the system.
        </div>
      </div>

      <div v-else class="tree-nodes-list">
        <!-- Render Flattened Tree Nodes with proper depth indentation -->
        <div
          v-for="item in flattenedVisibleNodes"
          :key="item.node.id"
          class="tree-node-row"
          :class="{
            'node-selected': isNodeSelected(item.node),
            'node-pool-root': item.node.isPoolRoot,
          }"
          :style="{ paddingLeft: `${item.depth * 18 + 10}px` }"
          @click="selectNode(item.node)"
        >
          <!-- Chevron for Expand/Collapse -->
          <button
            v-if="item.hasChildren"
            type="button"
            class="chevron-btn"
            @click.stop="toggleNode(item.node.id)"
          >
            <MdiIcon
              :path="item.isExpanded ? mdiChevronDown : mdiChevronRight"
              size="16"
            />
          </button>
          <span v-else class="chevron-placeholder" />

          <!-- Icon: Pool Harddisk vs Folder -->
          <span class="node-icon">
            <MdiIcon
              :path="item.node.isPoolRoot ? mdiHarddisk : (item.isExpanded ? mdiFolderOpenOutline : mdiFolderOutline)"
              size="16"
              :style="{ color: item.node.isPoolRoot ? 'var(--accent-primary)' : (isNodeSelected(item.node) ? 'var(--accent-primary)' : 'var(--text-secondary)') }"
            />
          </span>

          <!-- Main Name & Path -->
          <div class="node-details">
            <div class="node-name-line">
              <span class="node-name mono" :class="{ 'font-semibold': item.node.isPoolRoot }">{{ item.node.name }}</span>
              <span v-if="item.node.isPoolRoot" class="badge badge-info" style="font-size: 9.5px; padding: 0 4px">
                POOL
              </span>
              <span v-if="item.node.mountpoint" class="node-mountpoint mono" :title="item.node.mountpoint">
                {{ item.node.mountpoint }}
              </span>
            </div>
          </div>

          <!-- Capacity Usage (if available) -->
          <div v-if="item.node.usedBytes !== undefined" class="node-usage mono">
            {{ formatBytes(item.node.usedBytes) }}
          </div>

          <!-- Selected Checkmark -->
          <div v-if="isNodeSelected(item.node)" class="node-selected-check">
            <MdiIcon :path="mdiCheck" size="14" />
          </div>
        </div>
      </div>
    </div>

    <!-- Active Selection Footer -->
    <div v-if="selectedIdentifier" class="dataset-tree-footer">
      <span style="font-size: 11px; color: var(--text-secondary)">Selected Dataset:</span>
      <span class="mono font-semibold" style="color: var(--accent-primary); font-size: 11.5px">
        {{ selectedIdentifier }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.dataset-hierarchy-container {
  display: flex;
  flex-direction: column;
  background: var(--bg-surface, #0f172a);
  border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.08));
  border-radius: 8px;
  overflow: hidden;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2);
}

.dataset-tree-header {
  padding: 10px 12px;
  background: rgba(255, 255, 255, 0.02);
  border-bottom: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.dataset-search-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  background: rgba(0, 0, 0, 0.25);
  border-bottom: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
}

.dataset-search-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  font-family: inherit;
  font-size: 12px;
  color: var(--text-primary, #f8fafc);
}

.dataset-search-input::placeholder {
  color: var(--text-secondary, #64748b);
}

.clear-search-btn {
  background: transparent;
  border: none;
  color: var(--text-secondary, #94a3b8);
  cursor: pointer;
  padding: 2px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
}

.clear-search-btn:hover {
  color: #fff;
}

.dataset-tree-viewport {
  overflow-y: auto;
  overflow-x: hidden;
  background: rgba(0, 0, 0, 0.1);
}

.empty-tree-state {
  padding: 32px 16px;
  text-align: center;
  font-size: 12.5px;
  color: var(--text-secondary, #94a3b8);
}

.tree-nodes-list {
  display: flex;
  flex-direction: column;
}

.tree-node-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.03);
  cursor: pointer;
  user-select: none;
  transition: background 0.1s ease, color 0.1s ease;
}

.tree-node-row:hover {
  background: var(--accent-bg-subtle);
}

.node-selected {
  background: var(--accent-bg-subtle) !important;
  border-left: 3px solid var(--accent-primary);
}

.node-pool-root {
  background: var(--table-hover);
}

.chevron-btn {
  background: transparent;
  border: none;
  padding: 0;
  width: 16px;
  height: 16px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-secondary);
}

.chevron-btn:hover {
  color: var(--text-primary);
}

.chevron-placeholder {
  width: 16px;
  height: 16px;
  display: inline-block;
}

.node-icon {
  display: flex;
  align-items: center;
  justify-content: center;
}

.node-details {
  flex: 1;
  min-width: 0;
}

.node-name-line {
  display: flex;
  align-items: center;
  gap: 6px;
  overflow: hidden;
}

.node-name {
  font-size: 12px;
  white-space: nowrap;
  color: var(--text-primary);
}

.node-mountpoint {
  font-size: 11px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.node-usage {
  font-size: 11px;
  color: var(--text-secondary);
  white-space: nowrap;
  padding-right: 4px;
}

.node-selected-check {
  color: var(--accent-primary);
  display: flex;
  align-items: center;
}

.tree-children-container {
  padding-left: 18px;
  border-left: 1px dashed var(--border-subtle);
  margin-left: 18px;
}

.tree-grandchildren-container {
  padding-left: 18px;
  border-left: 1px dashed rgba(255, 255, 255, 0.08);
  margin-left: 16px;
}

.dataset-tree-footer {
  padding: 6px 12px;
  background: rgba(0, 0, 0, 0.3);
  border-top: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
