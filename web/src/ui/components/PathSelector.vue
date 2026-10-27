<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import {
  mdiArrowUp,
  mdiClose,
  mdiFolderOpenOutline,
  mdiFolderOutline,
  mdiCheck,
  mdiRefresh,
} from '@mdi/js';
import { services } from '../../container';
import type { DirectoryEntry } from '../../domain/models';
import MdiIcon from './MdiIcon.vue';

const props = withDefaults(
  defineProps<{
    modelValue: string;
    placeholder?: string;
    rootHint?: string;
  }>(),
  {
    placeholder: '/nas/tank/media',
    rootHint: '/nas',
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
  // If the path ends with a slash or is a directory, query it directly
  // Otherwise query the parent directory
  let lookupDir = path;
  if (!path.endsWith('/')) {
    const lastSlash = path.lastIndexOf('/');
    lookupDir = lastSlash === 0 ? '/' : path.substring(0, lastSlash);
  }
  try {
    const res = await services.apps.listDirectories(lookupDir);
    autocompleteSuggestions.value = (res.entries || []).map((e) => e.path);
  } catch {
    autocompleteSuggestions.value = [];
  }
}

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

async function openBrowser() {
  let initial = props.modelValue.trim();
  if (initial.startsWith('/tank')) {
    initial = '/nas' + initial;
  }
  if (!initial || !initial.startsWith('/')) {
    initial = props.rootHint || '/nas';
  }
  isBrowserOpen.value = true;
  await navigateTo(initial);
}

async function navigateTo(targetPath: string) {
  isLoadingBrowse.value = true;
  try {
    const res = await services.apps.listDirectories(targetPath);
    currentBrowsePath.value = res.currentPath;
    browseEntries.value = res.entries || [];
  } catch {
    // If target doesn't exist, try parent or root
    try {
      const fallback = await services.apps.listDirectories('/');
      currentBrowsePath.value = fallback.currentPath;
      browseEntries.value = fallback.entries || [];
    } catch {
      browseEntries.value = [];
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
  emit('update:modelValue', currentBrowsePath.value);
  isBrowserOpen.value = false;
}

function selectFolderEntry(entry: DirectoryEntry) {
  emit('update:modelValue', entry.path);
  isBrowserOpen.value = false;
}
</script>

<template>
  <div style="display: flex; gap: 6px; align-items: center; width: 100%">
    <!-- Text input with live directory datalist auto-completion -->
    <input
      :value="modelValue"
      type="text"
      class="form-input mono"
      :placeholder="placeholder"
      :list="datalistId"
      style="padding: 4px 8px; font-size: 12px; flex: 1"
      @input="handleInput"
      @focus="fetchSuggestions(modelValue)"
    />

    <datalist :id="datalistId">
      <option v-for="opt in autocompleteSuggestions" :key="opt" :value="opt" />
    </datalist>

    <!-- Visual Folder Browser Modal Button -->
    <button
      type="button"
      class="btn btn-sm btn-icon"
      style="padding: 4px 8px; flex-shrink: 0"
      title="Browse Host Folders..."
      @click="openBrowser"
    >
      <MdiIcon :path="mdiFolderOpenOutline" size="16" style="color: var(--accent-primary)" />
    </button>

    <!-- Graphical Folder Browser Modal -->
    <div
      v-if="isBrowserOpen"
      class="modal-backdrop"
      style="z-index: 150"
    >
      <div class="modal-card" style="max-width: 680px; width: 95vw; max-height: 80vh; display: flex; flex-direction: column">
        <!-- Header -->
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; padding-bottom: 10px; border-bottom: 1px solid var(--border-subtle)">
          <div style="display: flex; align-items: center; gap: 8px">
            <MdiIcon :path="mdiFolderOpenOutline" size="20" style="color: var(--accent-primary)" />
            <h4 style="font-size: 16px; font-weight: 700; color: var(--text-primary); margin: 0">
              Select Host Folder
            </h4>
          </div>
          <button class="btn btn-sm" style="padding: 2px 6px" @click="isBrowserOpen = false">
            <MdiIcon :path="mdiClose" size="18" />
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

        <!-- Directory List -->
        <div style="flex: 1; overflow-y: auto; border: 1px solid var(--border-subtle); border-radius: 6px; background: rgba(0,0,0,0.15); min-height: 220px; max-height: 360px">
          <div v-if="isLoadingBrowse" style="text-align: center; padding: 40px; color: var(--text-secondary)">
            <MdiIcon :path="mdiRefresh" size="24" class="spin-icon" style="margin-bottom: 8px" />
            <div>Loading directory contents...</div>
          </div>

          <div v-else-if="browseEntries.length === 0" style="text-align: center; padding: 40px; color: var(--text-secondary); font-size: 13px">
            No subdirectories found in this folder.
          </div>

          <div v-else style="display: flex; flex-direction: column">
            <div
              v-for="entry in browseEntries"
              :key="entry.path"
              style="display: flex; align-items: center; justify-content: space-between; padding: 8px 12px; border-bottom: 1px solid rgba(255,255,255,0.04); cursor: pointer; transition: background 0.1s"
              class="folder-row"
              @click="navigateTo(entry.path)"
            >
              <div style="display: flex; align-items: center; gap: 8px; overflow: hidden; text-overflow: ellipsis">
                <MdiIcon :path="mdiFolderOutline" size="18" style="color: var(--warning-text); flex-shrink: 0" />
                <span class="mono" style="font-size: 12.5px; color: var(--text-primary)">
                  {{ entry.name }}
                </span>
              </div>

              <button
                type="button"
                class="btn btn-sm"
                style="padding: 2px 8px; font-size: 11px; opacity: 0.8"
                @click.stop="selectFolderEntry(entry)"
              >
                Choose
              </button>
            </div>
          </div>
        </div>

        <!-- Footer / Confirmation -->
        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 14px; padding-top: 10px; border-top: 1px solid var(--border-subtle); flex-shrink: 0">
          <div class="mono" style="font-size: 12px; color: var(--text-secondary); overflow: hidden; text-overflow: ellipsis; max-width: 400px">
            Selected: <span style="color: var(--text-primary); font-weight: 600">{{ currentBrowsePath }}</span>
          </div>

          <div style="display: flex; gap: 8px">
            <button type="button" class="btn btn-sm" @click="isBrowserOpen = false">
              Cancel
            </button>
            <button
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
