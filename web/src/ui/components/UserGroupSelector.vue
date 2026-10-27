<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import {
  mdiAccountOutline,
  mdiAccountGroupOutline,
  mdiClose,
  mdiCheck,
  mdiChevronDown,
  mdiMagnify,
  mdiPlus,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import { nasState } from '../store';
import { services } from '../../container';
import type { User, Group } from '../../domain/models';

export interface SelectorOption {
  value: string;
  label: string;
  secondary?: string;
  badge?: string;
}

const props = withDefaults(
  defineProps<{
    modelValue: string | string[];
    type: 'user' | 'group';
    multiple?: boolean;
    placeholder?: string;
    disabled?: boolean;
    allowCustom?: boolean;
    includeSystem?: boolean;
  }>(),
  {
    multiple: false,
    placeholder: '',
    disabled: false,
    allowCustom: true,
    includeSystem: false,
  }
);

const emit = defineEmits<{
  (e: 'update:modelValue', value: any): void;
}>();

const containerRef = ref<HTMLDivElement | null>(null);
const searchInputRef = ref<HTMLInputElement | null>(null);
const isOpen = ref(false);
const searchQuery = ref('');
const highlightedIndex = ref(0);

// Load identity data if missing
onMounted(async () => {
  if (props.type === 'user' && (!nasState.users || nasState.users.length === 0)) {
    try {
      const users = await services.identity.listUsers(props.includeSystem);
      if (users) nasState.users = users;
    } catch {
      // Ignored
    }
  } else if (props.type === 'group' && (!nasState.groups || nasState.groups.length === 0)) {
    try {
      const groups = await services.identity.listGroups(props.includeSystem);
      if (groups) nasState.groups = groups;
    } catch {
      // Ignored
    }
  }
});

// Normalized selected values as array of strings
const selectedValues = computed<string[]>(() => {
  const v = props.modelValue;
  if (!v) return [];
  if (Array.isArray(v)) {
    return v.map((s) => String(s).trim()).filter(Boolean);
  }
  if (typeof v === 'string') {
    if (props.multiple) {
      return v.split(',').map((s) => s.trim()).filter(Boolean);
    }
    return v.trim() ? [v.trim()] : [];
  }
  return [];
});

// Base list of options from store
const rawOptions = computed<SelectorOption[]>(() => {
  if (props.type === 'user') {
    let list: User[] = nasState.users || [];
    if (!props.includeSystem) {
      list = list.filter((u) => u.uid >= 1000 && u.uid <= 10000 && !u.systemAccount);
    }
    return list.map((u) => ({
      value: u.username,
      label: u.username,
      secondary: u.fullName ? `${u.fullName} (UID: ${u.uid})` : `UID: ${u.uid}`,
      badge: u.systemAccount ? 'system' : undefined,
    }));
  } else {
    const list: Group[] = nasState.groups || [];
    return list.map((g) => ({
      value: g.groupName,
      label: g.groupName,
      secondary: `GID: ${g.gid}${g.members && g.members.length > 0 ? ` · ${g.members.length} member(s)` : ''}`,
      badge: g.systemGroup ? 'system' : undefined,
    }));
  }
});

// Fuzzy match algorithm
function fuzzyScore(target: string, query: string): number {
  target = target.toLowerCase();
  query = query.toLowerCase();

  if (target === query) return 1000;
  if (target.startsWith(query)) return 500 + (100 - target.length);
  if (target.includes(query)) return 200 + (100 - target.indexOf(query));

  // Character subsequence match
  let tIdx = 0;
  let matches = 0;
  for (let qIdx = 0; qIdx < query.length; qIdx++) {
    const qChar = query[qIdx];
    const found = target.indexOf(qChar, tIdx);
    if (found === -1) return 0;
    matches++;
    tIdx = found + 1;
  }
  return 50 + matches;
}

// Filtered & ranked options
const filteredOptions = computed<SelectorOption[]>(() => {
  const q = searchQuery.value.trim().toLowerCase();
  const all = rawOptions.value;

  if (!q) {
    return all;
  }

  const scored: { opt: SelectorOption; score: number }[] = [];
  for (const opt of all) {
    let score = fuzzyScore(opt.label, q);
    if (opt.secondary) {
      score = Math.max(score, fuzzyScore(opt.secondary, q) / 2);
    }
    if (score > 0) {
      scored.push({ opt, score });
    }
  }

  scored.sort((a, b) => b.score - a.score);
  return scored.map((s) => s.opt);
});

// Can custom typed query be added?
const canAddCustom = computed(() => {
  if (!props.allowCustom) return false;
  const q = searchQuery.value.trim();
  if (!q) return false;
  // If already in options with exact match
  const exact = rawOptions.value.some((o) => o.label.toLowerCase() === q.toLowerCase());
  // If already selected
  const alreadySelected = selectedValues.value.includes(q);
  return !exact && !alreadySelected;
});

function emitUpdate(newVals: string[]) {
  if (Array.isArray(props.modelValue)) {
    emit('update:modelValue', newVals);
  } else {
    if (props.multiple) {
      emit('update:modelValue', newVals.join(', '));
    } else {
      emit('update:modelValue', newVals[0] || '');
    }
  }
}

function selectOption(val: string) {
  if (props.multiple) {
    if (!selectedValues.value.includes(val)) {
      emitUpdate([...selectedValues.value, val]);
    }
    searchQuery.value = '';
    searchInputRef.value?.focus();
  } else {
    emitUpdate([val]);
    searchQuery.value = '';
    isOpen.value = false;
  }
}

function removeValue(val: string) {
  emitUpdate(selectedValues.value.filter((v) => v !== val));
}

function clearSingle() {
  emitUpdate([]);
  searchQuery.value = '';
}

function handleInputKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    isOpen.value = false;
    return;
  }

  if (e.key === 'ArrowDown') {
    e.preventDefault();
    if (!isOpen.value) {
      isOpen.value = true;
      return;
    }
    const max = filteredOptions.value.length + (canAddCustom.value ? 1 : 0);
    if (max > 0) {
      highlightedIndex.value = (highlightedIndex.value + 1) % max;
    }
    return;
  }

  if (e.key === 'ArrowUp') {
    e.preventDefault();
    if (!isOpen.value) {
      isOpen.value = true;
      return;
    }
    const max = filteredOptions.value.length + (canAddCustom.value ? 1 : 0);
    if (max > 0) {
      highlightedIndex.value = (highlightedIndex.value - 1 + max) % max;
    }
    return;
  }

  if (e.key === 'Enter' || e.key === ',') {
    e.preventDefault();
    if (!isOpen.value) {
      if (canAddCustom.value) {
        selectOption(searchQuery.value.trim());
        return;
      }
      const exactMatch = filteredOptions.value.find(
        (o) => o.label.toLowerCase() === searchQuery.value.trim().toLowerCase()
      );
      if (exactMatch) {
        selectOption(exactMatch.value);
        return;
      }
      isOpen.value = true;
      return;
    }

    if (canAddCustom.value && highlightedIndex.value === filteredOptions.value.length) {
      selectOption(searchQuery.value.trim());
      return;
    }

    const opt = filteredOptions.value[highlightedIndex.value];
    if (opt) {
      selectOption(opt.value);
    } else if (canAddCustom.value) {
      selectOption(searchQuery.value.trim());
    }
    return;
  }

  if (e.key === 'Backspace' && searchQuery.value === '' && props.multiple && selectedValues.value.length > 0) {
    // Remove the last selected value
    const last = selectedValues.value[selectedValues.value.length - 1];
    removeValue(last);
  }
}

function onClickOutside(e: PointerEvent) {
  if (containerRef.value && !containerRef.value.contains(e.target as Node)) {
    isOpen.value = false;
  }
}

watch(isOpen, (val) => {
  if (val) {
    document.addEventListener('pointerdown', onClickOutside);
    highlightedIndex.value = 0;
  } else {
    document.removeEventListener('pointerdown', onClickOutside);
  }
});

onUnmounted(() => {
  document.removeEventListener('pointerdown', onClickOutside);
});

function toggleOpen() {
  if (props.disabled) return;
  isOpen.value = !isOpen.value;
  if (isOpen.value) {
    searchInputRef.value?.focus();
  }
}

const defaultPlaceholder = computed(() => {
  if (props.placeholder) return props.placeholder;
  if (props.multiple) {
    return props.type === 'user' ? 'Select or type users...' : 'Select or type groups...';
  }
  return props.type === 'user' ? 'Select user...' : 'Select group...';
});
</script>

<template>
  <div
    ref="containerRef"
    class="user-group-selector"
    :class="{ 'is-open': isOpen, 'is-disabled': disabled, 'is-multiple': multiple }"
    @click="toggleOpen"
  >
    <!-- Multi-select chips and inline search -->
    <template v-if="multiple">
      <div class="chips-wrap">
        <span
          v-for="val in selectedValues"
          :key="val"
          class="selector-chip mono"
        >
          <MdiIcon
            :path="type === 'user' ? mdiAccountOutline : mdiAccountGroupOutline"
            size="13"
            style="color: var(--accent-primary); flex-shrink: 0"
          />
          <span>{{ val }}</span>
          <button
            type="button"
            class="chip-remove-btn"
            title="Remove"
            @click.stop="removeValue(val)"
          >
            <MdiIcon :path="mdiClose" size="12" />
          </button>
        </span>

        <input
          ref="searchInputRef"
          v-model="searchQuery"
          type="text"
          class="chip-search-input mono"
          :placeholder="selectedValues.length === 0 ? defaultPlaceholder : ''"
          :disabled="disabled"
          @keydown="handleInputKeydown"
          @focus="isOpen = true"
          @click.stop
        />
      </div>

      <div class="toggle-icon-wrap" @click.stop="toggleOpen">
        <MdiIcon :path="mdiChevronDown" size="16" class="chevron" :class="{ 'rotated': isOpen }" />
      </div>
    </template>

    <!-- Single-select display & trigger -->
    <template v-else>
      <div class="single-display">
        <MdiIcon
          :path="type === 'user' ? mdiAccountOutline : mdiAccountGroupOutline"
          size="16"
          style="color: var(--accent-primary); flex-shrink: 0; margin-right: 6px"
        />
        <span v-if="selectedValues[0]" class="selected-text mono">
          {{ selectedValues[0] }}
        </span>
        <span v-else class="placeholder-text">
          {{ defaultPlaceholder }}
        </span>
      </div>

      <div class="single-actions" @click.stop>
        <button
          v-if="selectedValues.length > 0 && !disabled"
          type="button"
          class="btn-clear"
          title="Clear"
          @click="clearSingle"
        >
          <MdiIcon :path="mdiClose" size="14" />
        </button>
        <button type="button" class="btn-toggle" @click="toggleOpen">
          <MdiIcon :path="mdiChevronDown" size="16" class="chevron" :class="{ 'rotated': isOpen }" />
        </button>
      </div>
    </template>

    <!-- Dropdown Menu -->
    <div v-if="isOpen" class="dropdown-menu" @click.stop>
      <!-- Search bar inside dropdown for single-select -->
      <div v-if="!multiple" class="dropdown-search-wrap">
        <MdiIcon :path="mdiMagnify" size="15" style="color: var(--text-muted); flex-shrink: 0" />
        <input
          ref="searchInputRef"
          v-model="searchQuery"
          type="text"
          class="dropdown-search-input mono"
          :placeholder="`Search ${type}s...`"
          @keydown="handleInputKeydown"
        />
        <button
          v-if="searchQuery"
          type="button"
          class="btn-clear"
          @click="searchQuery = ''"
        >
          <MdiIcon :path="mdiClose" size="13" />
        </button>
      </div>

      <!-- Options List -->
      <div class="options-list">
        <!-- Exact / Custom Add Option -->
        <div
          v-if="canAddCustom"
          class="option-item custom-option"
          :class="{ 'is-highlighted': highlightedIndex === filteredOptions.length }"
          @click="selectOption(searchQuery.trim())"
        >
          <div style="display: flex; align-items: center; gap: 8px">
            <MdiIcon :path="mdiPlus" size="16" style="color: var(--success); flex-shrink: 0" />
            <span class="mono" style="font-weight: 600; color: var(--accent-primary)">
              Use custom {{ type }}: "{{ searchQuery.trim() }}"
            </span>
          </div>
        </div>

        <template v-if="filteredOptions.length > 0">
          <div
            v-for="(opt, idx) in filteredOptions"
            :key="opt.value"
            class="option-item"
            :class="{
              'is-highlighted': idx === highlightedIndex,
              'is-selected': selectedValues.includes(opt.value),
            }"
            @click="selectOption(opt.value)"
            @mouseenter="highlightedIndex = idx"
          >
            <div style="display: flex; align-items: center; gap: 8px; overflow: hidden">
              <MdiIcon
                :path="type === 'user' ? mdiAccountOutline : mdiAccountGroupOutline"
                size="16"
                style="color: var(--accent-primary); flex-shrink: 0"
              />
              <span class="mono option-label">{{ opt.label }}</span>
              <span v-if="opt.secondary" class="option-secondary">{{ opt.secondary }}</span>
              <span v-if="opt.badge" class="option-badge">{{ opt.badge }}</span>
            </div>

            <MdiIcon
              v-if="selectedValues.includes(opt.value)"
              :path="mdiCheck"
              size="15"
              style="color: var(--success); flex-shrink: 0"
            />
          </div>
        </template>

        <div v-else-if="!canAddCustom" class="no-options">
          No matching {{ type }}s found.
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.user-group-selector {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 36px;
  background: var(--bg-surface);
  border: 1px solid var(--border-strong);
  border-radius: 6px;
  padding: 4px 8px;
  cursor: pointer;
  transition: all 0.15s ease;
  user-select: none;
  width: 100%;
}

.user-group-selector:hover:not(.is-disabled) {
  border-color: var(--accent-primary);
}

.user-group-selector.is-open {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 2px rgba(122, 162, 247, 0.2);
}

.user-group-selector.is-disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

/* Multi-select chips container */
.chips-wrap {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 0;
}

.selector-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: var(--accent-bg-subtle);
  border: 1px solid rgba(122, 162, 247, 0.25);
  border-radius: 4px;
  padding: 2px 6px;
  font-size: 11.5px;
  color: var(--text-primary);
}

.chip-remove-btn {
  background: none;
  border: none;
  padding: 0;
  display: flex;
  align-items: center;
  cursor: pointer;
  color: var(--text-secondary);
  opacity: 0.7;
}

.chip-remove-btn:hover {
  opacity: 1;
  color: var(--danger);
}

.chip-search-input {
  border: none;
  outline: none;
  background: transparent;
  color: var(--text-primary);
  font-size: 12.5px;
  min-width: 80px;
  flex: 1;
  padding: 2px 4px;
}

/* Single-select display */
.single-display {
  display: flex;
  align-items: center;
  flex: 1;
  overflow: hidden;
  font-size: 13px;
}

.selected-text {
  color: var(--text-primary);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.placeholder-text {
  color: var(--text-muted);
  font-size: 12.5px;
}

.single-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}

.btn-clear,
.btn-toggle {
  background: none;
  border: none;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  color: var(--text-secondary);
  padding: 2px;
  border-radius: 3px;
}

.btn-clear:hover,
.btn-toggle:hover {
  color: var(--text-primary);
}

.chevron {
  transition: transform 0.2s ease;
}

.chevron.rotated {
  transform: rotate(180deg);
}

/* Dropdown menu */
.dropdown-menu {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  right: 0;
  background: var(--bg-surface);
  border: 1px solid var(--border-strong);
  border-radius: 6px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
  z-index: 2100;
  max-height: 280px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  cursor: default;
}

.dropdown-search-wrap {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 10px;
  border-bottom: 1px solid var(--border-subtle);
  background: var(--bg-base);
}

.dropdown-search-input {
  border: none;
  outline: none;
  background: transparent;
  color: var(--text-primary);
  font-size: 12px;
  flex: 1;
}

.options-list {
  overflow-y: auto;
  max-height: 230px;
  display: flex;
  flex-direction: column;
}

.option-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 7px 10px;
  cursor: pointer;
  transition: background 0.1s ease;
  font-size: 12.5px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.03);
}

.option-item:hover,
.option-item.is-highlighted {
  background: var(--bg-hover);
}

.option-item.is-selected {
  background: var(--accent-bg-subtle);
}

.custom-option {
  border-bottom: 1px solid var(--border-subtle);
  background: rgba(122, 162, 247, 0.05);
}

.option-label {
  color: var(--text-primary);
  font-weight: 600;
}

.option-secondary {
  font-size: 11px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.option-badge {
  font-size: 10px;
  background: rgba(255, 255, 255, 0.07);
  padding: 1px 4px;
  border-radius: 3px;
  color: var(--text-muted);
}

.no-options {
  padding: 20px;
  text-align: center;
  color: var(--text-muted);
  font-size: 12px;
}
</style>
