<script setup lang="ts">
import { computed, ref } from 'vue';
import {
  mdiArrowUp,
  mdiClose,
  mdiFolderOpenOutline,
  mdiFolderOutline,
  mdiCheck,
  mdiRefresh,
  mdiFolderPlusOutline,
  mdiDisc,
  mdiFileDocumentOutline,
} from '@mdi/js';
import { services } from '../../container';
import { nasState } from '../store';
import type { DirectoryEntry } from '../../domain/models';
import MdiIcon from './MdiIcon.vue';

const props = withDefaults(
  defineProps<{
    modelValue: string;
    placeholder?: string;
    rootHint?: string;
    required?: boolean;
    disabled?: boolean;
    buttonLabel?: string;
    inputClass?: string;
    modalTitle?: string;
    selectFiles?: boolean;
    fileExtensions?: string[];
  }>(),
  {
    placeholder: '/mnt/tank/media',
    rootHint: '/mnt',
    required: false,
    disabled: false,
    buttonLabel: 'Browse',
    inputClass: 'input mono',
    modalTitle: 'Select Host Folder',
    selectFiles: false,
    fileExtensions: () => [],
  }
);

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void;
}>();

// Datalist auto-completion suggestions
const datalistId = `path-datalist-${Math.random().toString(36).substring(2, 9)}`;
const autocompleteSuggestions = ref<string[]>([]);
let autocompleteTimer: any = null;

async function fetchSuggestions(path: string) {
  if (!path || !path.startsWith('/')) {
    path = '/';
  }
  let lookupDir = path;
  if (!path.endsWith('/')) {
    const lastSlash = path.lastIndexOf('/');
    lookupDir = lastSlash === 0 ? '/' : path.substring(0, lastSlash);
  }
  try {
    const res = await services.apps.listDirectories(lookupDir);
    let entries = res.entries || [];
    if (!props.selectFiles) {
      entries = entries.filter((e) => e.isDir);
    } else if (props.fileExtensions && props.fileExtensions.length > 0) {
      const exts = props.fileExtensions.map((x) => x.toLowerCase());
      entries = entries.filter((e) => e.isDir || exts.some((ext) => e.name.toLowerCase().endsWith(ext)));
    }
    autocompleteSuggestions.value = entries.map((e) => e.path);
  } catch {
    autocompleteSuggestions.value = [];
  }
}

const filteredEntries = computed(() => {
  if (!props.selectFiles) {
    return browseEntries.value.filter((e) => e.isDir);
  }
  if (props.fileExtensions && props.fileExtensions.length > 0) {
    const exts = props.fileExtensions.map((x) => x.toLowerCase());
    return browseEntries.value.filter((e) => {
      if (e.isDir) return true;
      const lower = e.name.toLowerCase();
      return exts.some((ext) => lower.endsWith(ext));
    });
  }
  return browseEntries.value;
});

function handleInput(e: Event) {
  const target = e.target as HTMLInputElement;
  const val = target.value;
  emit('update:modelValue', val);

  clearTimeout(autocompleteTimer);
  autocompleteTimer = setTimeout(() => {
    fetchSuggestions(val);
  }, 200);
}

// =============================================================================
// Graphical Folder Browser Modal
// =============================================================================
const isBrowserOpen = ref(false);
const currentBrowsePath = ref('');
const browseEntries = ref<DirectoryEntry[]>([]);
const isLoadingBrowse = ref(false);
const subfolderInput = ref('');

const pathBreadcrumbs = computed(() => {
  const p = currentBrowsePath.value;
  if (!p || p === '/') return [{ name: '/', path: '/' }];
  const parts = p.split('/').filter(Boolean);
  const crumbs = [{ name: 'root (/)', path: '/' }];
  let accum = '';
  for (const part of parts) {
    accum += '/' + part;
    crumbs.push({ name: part, path: accum });
  }
  return crumbs;
});

const finalSelectedPath = computed(() => {
  const base = (currentBrowsePath.value || '/').replace(/\/+$/, '');
  const sub = subfolderInput.value.trim().replace(/^\/+/, '');
  if (!sub) return currentBrowsePath.value || '/';
  return (base === '' ? '' : base) + '/' + sub;
});

// Quick shortcut paths based on available pools and mount points
const quickShortcuts = computed(() => {
  const shortcuts: { label: string; path: string }[] = [{ label: '/mnt', path: '/mnt' }];
  const mainPool = nasState.systemConfig?.mainPool;
  if (mainPool) {
    const cleanMain = mainPool.replace(/^zfsPools\//, '').trim();
    if (cleanMain) {
      const pPath = `/mnt/${cleanMain}`;
      if (!shortcuts.some((s) => s.path === pPath)) {
        shortcuts.push({ label: pPath, path: pPath });
      }
    }
  }
  if (nasState.pools && nasState.pools.length > 0) {
    for (const p of nasState.pools) {
      const cleanName = (p.poolName || p.name.replace(/^zfsPools\//, '')).trim();
      let pPath = (p.mountpoint || '').trim();
      if (!pPath || pPath === 'none' || pPath === 'legacy') {
        pPath = cleanName ? `/mnt/${cleanName}` : '';
      }
      pPath = pPath.replace(/\/zfsPools\//g, '/').replace(/^zfsPools\//, '').replace(/^\/zfsPools/, '');
      if (pPath && pPath !== '/mnt' && !shortcuts.some((s) => s.path === pPath)) {
        shortcuts.push({ label: pPath, path: pPath });
      }
    }
  }
  return shortcuts;
});

async function openBrowser() {
  if (props.disabled) return;
  subfolderInput.value = '';
  let initial = (props.modelValue || '').trim();
  if (initial.startsWith('/tank')) {
    initial = '/mnt' + initial;
  }
  if (!initial || !initial.startsWith('/')) {
    if (props.rootHint) {
      initial = props.rootHint;
    } else if (nasState.systemConfig?.mainPool) {
      const cleanMain = nasState.systemConfig.mainPool.replace(/^zfsPools\//, '').trim();
      initial = `/mnt/${cleanMain}`;
    } else {
      initial = '/mnt';
    }
  }
  initial = initial.replace(/\/zfsPools\//g, '/').replace(/^zfsPools\//, '').replace(/^\/zfsPools/, '');
  if (!initial.startsWith('/')) initial = '/' + initial;
  isBrowserOpen.value = true;
  await navigateTo(initial);
}

async function navigateTo(targetPath: string) {
  isLoadingBrowse.value = true;
  subfolderInput.value = '';
  let cleanTarget = targetPath.replace(/\/zfsPools\//g, '/').replace(/^zfsPools\//, '').replace(/^\/zfsPools/, '');
  if (!cleanTarget.startsWith('/')) cleanTarget = '/' + cleanTarget;
  try {
    const res = await services.apps.listDirectories(cleanTarget);
    currentBrowsePath.value = res.currentPath;
    browseEntries.value = res.entries || [];
  } catch {
    try {
      const fallback = await services.apps.listDirectories('/mnt');
      currentBrowsePath.value = fallback.currentPath;
      browseEntries.value = fallback.entries || [];
    } catch {
      try {
        const rootFallback = await services.apps.listDirectories('/');
        currentBrowsePath.value = rootFallback.currentPath;
        browseEntries.value = rootFallback.entries || [];
      } catch {
        browseEntries.value = [];
      }
    }
  } finally {
    isLoadingBrowse.value = false;
  }
}

function navigateUp() {
  if (!currentBrowsePath.value || currentBrowsePath.value === '/') return;
  const lastSlash = currentBrowsePath.value.lastIndexOf('/');
  const parent = lastSlash <= 0 ? '/' : currentBrowsePath.value.substring(0, lastSlash);
  navigateTo(parent);
}

function selectCurrentFolder() {
  emit('update:modelValue', finalSelectedPath.value);
  isBrowserOpen.value = false;
}

function selectFolderEntry(entry: DirectoryEntry) {
  emit('update:modelValue', entry.path);
  isBrowserOpen.value = false;
}
</script>

<template>
  <div class="path-selector-container" style="display: flex; gap: 6px; align-items: center; width: 100%">
    <!-- Text input with live directory datalist auto-completion -->
    <input
      :value="modelValue"
      type="text"
      :class="inputClass"
      :placeholder="placeholder"
      :list="datalistId"
      :required="required"
      :disabled="disabled"
      style="flex: 1"
      @input="handleInput"
      @focus="fetchSuggestions(modelValue)"
    />

    <datalist :id="datalistId">
      <option v-for="opt in autocompleteSuggestions" :key="opt" :value="opt" />
    </datalist>

    <!-- Visual Folder Browser Modal Button -->
    <button
      type="button"
      class="btn btn-sm"
      :disabled="disabled"
      style="padding: 6px 10px; display: inline-flex; align-items: center; gap: 4px; flex-shrink: 0"
      title="Browse Folders..."
      @click="openBrowser"
    >
      <MdiIcon :path="mdiFolderOpenOutline" size="16" style="color: var(--accent-primary)" />
      <span v-if="buttonLabel">{{ buttonLabel }}</span>
    </button>

    <!-- Graphical Folder Browser Modal -->
    <div
      v-if="isBrowserOpen"
      class="modal-backdrop"
      style="z-index: 2000"
      @click.self="isBrowserOpen = false"
    >
      <div class="modal-card" style="max-width: 680px; width: 95vw; max-height: 85vh; display: flex; flex-direction: column">
        <!-- Header -->
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; padding-bottom: 10px; border-bottom: 1px solid var(--border-subtle)">
          <div style="display: flex; align-items: center; gap: 8px">
            <MdiIcon :path="mdiFolderOpenOutline" size="20" style="color: var(--accent-primary)" />
            <h4 style="font-size: 16px; font-weight: 700; color: var(--text-primary); margin: 0">
              {{ modalTitle }}
            </h4>
          </div>
          <button class="btn btn-sm" style="padding: 2px 6px" @click="isBrowserOpen = false">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>

        <!-- Shortcuts Bar -->
        <div v-if="quickShortcuts.length > 0" style="display: flex; align-items: center; gap: 6px; margin-bottom: 10px; flex-wrap: wrap">
          <span style="font-size: 11px; color: var(--text-secondary); font-weight: 600">Quick Jump:</span>
          <button
            v-for="sc in quickShortcuts"
            :key="sc.path"
            type="button"
            class="btn btn-xs"
            :class="{ 'btn-primary': currentBrowsePath === sc.path }"
            style="font-family: monospace; font-size: 11px; padding: 2px 6px"
            @click="navigateTo(sc.path)"
          >
            {{ sc.label }}
          </button>
        </div>

        <!-- Breadcrumbs Navigation Bar -->
        <div style="background: var(--bg-elevated); padding: 8px 12px; border-radius: 6px; border: 1px solid var(--border-subtle); display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; flex-shrink: 0">
          <div style="display: flex; align-items: center; gap: 4px; overflow-x: auto; font-size: 12.5px" class="mono">
            <template v-for="(crumb, idx) in pathBreadcrumbs" :key="crumb.path">
              <span v-if="idx > 0" style="color: var(--text-secondary); opacity: 0.5">/</span>
              <button
                type="button"
                class="btn-link"
                style="background: none; border: none; padding: 2px 4px; cursor: pointer; color: var(--accent-primary); font-family: monospace; font-size: 12px"
                @click="navigateTo(crumb.path)"
              >
                {{ crumb.name }}
              </button>
            </template>
          </div>

          <div style="display: flex; gap: 6px">
            <button
              type="button"
              class="btn btn-sm"
              :disabled="isLoadingBrowse"
              style="padding: 3px 6px"
              title="Refresh directory contents"
              @click="navigateTo(currentBrowsePath)"
            >
              <MdiIcon :path="mdiRefresh" size="14" :class="{ 'spin-icon': isLoadingBrowse }" />
            </button>
            <button
              type="button"
              class="btn btn-sm"
              :disabled="currentBrowsePath === '/' || isLoadingBrowse"
              style="padding: 3px 8px; font-size: 11.5px; display: inline-flex; align-items: center; gap: 4px"
              title="Go to parent directory"
              @click="navigateUp"
            >
              <MdiIcon :path="mdiArrowUp" size="14" />
              <span>Up</span>
            </button>
          </div>
        </div>

        <!-- Directory / File List -->
        <div style="flex: 1; overflow-y: auto; border: 1px solid var(--border-subtle); border-radius: 6px; background: rgba(0,0,0,0.15); min-height: 220px; max-height: 360px">
          <div v-if="isLoadingBrowse" style="text-align: center; padding: 40px; color: var(--text-secondary)">
            <MdiIcon :path="mdiRefresh" size="24" class="spin-icon" style="margin-bottom: 8px" />
            <div>Loading directory contents...</div>
          </div>

          <div v-else-if="filteredEntries.length === 0" style="text-align: center; padding: 40px; color: var(--text-secondary); font-size: 13px">
            {{ selectFiles ? 'No matching files or subdirectories found in this folder.' : 'No subdirectories found in this folder.' }}
          </div>

          <div v-else style="display: flex; flex-direction: column">
            <div
              v-for="entry in filteredEntries"
              :key="entry.path"
              style="display: flex; align-items: center; justify-content: space-between; padding: 8px 12px; border-bottom: 1px solid rgba(255,255,255,0.04); cursor: pointer; transition: background 0.1s"
              class="folder-row"
              @click="entry.isDir ? navigateTo(entry.path) : selectFolderEntry(entry)"
            >
              <div style="display: flex; align-items: center; gap: 8px; overflow: hidden; text-overflow: ellipsis">
                <MdiIcon
                  v-if="entry.isDir"
                  :path="mdiFolderOutline"
                  size="18"
                  style="color: var(--warning-text); flex-shrink: 0"
                />
                <MdiIcon
                  v-else-if="entry.name.toLowerCase().endsWith('.iso') || entry.name.toLowerCase().endsWith('.img')"
                  :path="mdiDisc"
                  size="18"
                  style="color: var(--accent-primary); flex-shrink: 0"
                />
                <MdiIcon
                  v-else
                  :path="mdiFileDocumentOutline"
                  size="18"
                  style="color: var(--text-secondary); flex-shrink: 0"
                />
                <span class="mono" style="font-size: 12.5px; color: var(--text-primary)">
                  {{ entry.name }}
                </span>
              </div>

              <div style="display: flex; gap: 6px; align-items: center">
                <button
                  v-if="entry.isDir"
                  type="button"
                  class="btn btn-sm"
                  style="padding: 2px 8px; font-size: 11px; opacity: 0.8"
                  @click.stop="navigateTo(entry.path)"
                >
                  Open
                </button>
                <button
                  type="button"
                  class="btn btn-sm"
                  :class="{ 'btn-primary': !entry.isDir }"
                  style="padding: 2px 8px; font-size: 11px"
                  @click.stop="selectFolderEntry(entry)"
                >
                  {{ entry.isDir ? 'Choose' : 'Select' }}
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- Optional Subfolder or New Subfolder field (only when selecting directories) -->
        <div v-if="!selectFiles" style="margin-top: 12px; display: flex; align-items: center; gap: 8px">
          <MdiIcon :path="mdiFolderPlusOutline" size="18" style="color: var(--accent-primary); flex-shrink: 0" />
          <span style="font-size: 12px; color: var(--text-secondary); white-space: nowrap">Subfolder (optional):</span>
          <input
            v-model="subfolderInput"
            type="text"
            class="input mono"
            placeholder="e.g. media or backups"
            style="font-size: 12px; padding: 4px 8px; flex: 1"
          />
        </div>

        <!-- Footer / Confirmation -->
        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 14px; padding-top: 10px; border-top: 1px solid var(--border-subtle); flex-shrink: 0">
          <div class="mono" style="font-size: 12px; color: var(--text-secondary); overflow: hidden; text-overflow: ellipsis; max-width: 380px">
            <span v-if="selectFiles" style="color: var(--text-muted); font-size: 11.5px">
              Click any file row or "Select" to use it as boot / media image.
            </span>
            <span v-else>
              Selected: <span style="color: var(--text-primary); font-weight: 600">{{ finalSelectedPath }}</span>
            </span>
          </div>

          <div style="display: flex; gap: 8px">
            <button type="button" class="btn btn-sm" @click="isBrowserOpen = false">
              Cancel
            </button>
            <button
              v-if="!selectFiles"
              type="button"
              class="btn btn-sm btn-primary"
              style="display: inline-flex; align-items: center; gap: 4px"
              @click="selectCurrentFolder"
            >
              <MdiIcon :path="mdiCheck" size="14" />
              <span>Select This Folder</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.folder-row:hover {
  background: rgba(255, 255, 255, 0.05);
}
</style>
