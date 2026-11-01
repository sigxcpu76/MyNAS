<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import {
  mdiCheck,
  mdiChevronDown,
  mdiFormatSize,
  mdiMonitorEye,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import {
  activeScaleLabel,
  activeScaleMode,
  applyScale,
  detectedFactor,
  effectiveScaleFactor,
  UI_SCALE_OPTIONS,
  type UiScaleMode,
} from '../scale';

const isOpen = ref(false);
const dropdownRef = ref<HTMLDivElement | null>(null);

const currentResolutionInfo = computed(() => {
  if (typeof window === 'undefined') return '';
  const dpr = window.devicePixelRatio || 1;
  const w = (window.screen && window.screen.width) || window.innerWidth;
  const h = (window.screen && window.screen.height) || window.innerHeight;
  return `${w}×${h} • DPR: ${dpr.toFixed(2)}x`;
});

function toggleDropdown() {
  isOpen.value = !isOpen.value;
}

function selectScale(mode: UiScaleMode) {
  applyScale(mode);
  isOpen.value = false;
}

function handleClickOutside(event: MouseEvent) {
  if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) {
    isOpen.value = false;
  }
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && isOpen.value) {
    isOpen.value = false;
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside);
  document.addEventListener('keydown', handleKeydown);
});

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside);
  document.removeEventListener('keydown', handleKeydown);
});
</script>

<template>
  <div ref="dropdownRef" class="scale-selector-container">
    <button
      type="button"
      class="btn btn-sm btn-icon scale-toggle-btn"
      :class="{ 'btn-open': isOpen }"
      :title="`Display Scale: ${activeScaleLabel} — Click to adjust UI & font size for HiDPI/4K`"
      aria-label="Select Display Scale"
      @click="toggleDropdown"
    >
      <MdiIcon
        :path="mdiFormatSize"
        size="16"
        class="scale-icon"
      />
      <span class="sr-only">Display Scale: {{ activeScaleLabel }}</span>
    </button>

    <!-- Scale Dropdown Popover -->
    <transition name="dropdown-fade">
      <div v-if="isOpen" class="scale-dropdown-menu">
        <div class="scale-menu-header">
          <MdiIcon :path="mdiFormatSize" size="16" style="color: var(--accent-primary)" />
          <span>UI & Font Scale</span>
          <span class="active-badge mono">{{ activeScaleLabel }}</span>
        </div>

        <div class="scale-options-list">
          <button
            v-for="opt in UI_SCALE_OPTIONS"
            :key="opt.id"
            type="button"
            class="scale-option"
            :class="{ selected: activeScaleMode === opt.id }"
            @click="selectScale(opt.id)"
          >
            <div class="scale-info-box">
              <div class="scale-info-row">
                <span class="scale-name">{{ opt.label }}</span>
                <span
                  v-if="opt.id === 'auto'"
                  class="scale-pill"
                >
                  Detected {{ Math.round(detectedFactor * 100) }}%
                </span>
                <span
                  v-else-if="opt.id === '100'"
                  class="scale-pill-neutral"
                >
                  Standard
                </span>
                <span
                  v-else-if="opt.id === '120'"
                  class="scale-pill-accent"
                >
                  Recommended for 2K
                </span>
                <span
                  v-else-if="opt.id === '130'"
                  class="scale-pill-accent"
                >
                  Recommended for 4K
                </span>
              </div>
              <div class="scale-desc">{{ opt.description }}</div>
            </div>
            <MdiIcon
              v-if="activeScaleMode === opt.id"
              :path="mdiCheck"
              size="16"
              class="scale-check-icon"
            />
          </button>
        </div>

        <div class="scale-divider"></div>

        <!-- Hardware & Display Detection Info Footer -->
        <div class="scale-menu-footer">
          <MdiIcon :path="mdiMonitorEye" size="13" style="color: var(--text-muted)" />
          <span class="mono font-xs text-muted">{{ currentResolutionInfo }}</span>
        </div>
      </div>
    </transition>
  </div>
</template>

<style scoped>
.scale-selector-container {
  position: relative;
  display: inline-flex;
}

.scale-toggle-btn {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
  width: 28px;
  height: 28px;
  padding: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.scale-toggle-btn:hover,
.scale-toggle-btn.btn-open {
  background: var(--bg-hover);
  color: var(--text-primary);
  border-color: var(--border-strong);
}

.scale-icon {
  pointer-events: none;
}

.scale-dropdown-menu {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  width: 310px;
  background: var(--bg-surface);
  border: 1px solid var(--border-strong);
  border-radius: 8px;
  box-shadow: 0 10px 25px rgba(0, 0, 0, 0.45);
  z-index: 1000;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.scale-menu-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px 4px 8px;
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary);
}

.active-badge {
  margin-left: auto;
  font-size: 11px;
  background: var(--accent-bg-subtle);
  color: var(--accent-primary);
  border: 1px solid var(--accent-primary);
  padding: 1px 6px;
  border-radius: 4px;
}

.scale-options-list {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.scale-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  padding: 6px 10px;
  cursor: pointer;
  text-align: left;
  transition: all 0.15s ease;
  gap: 10px;
}

.scale-option:hover {
  background: var(--bg-hover);
}

.scale-option.selected {
  background: var(--accent-bg-subtle);
  border-color: var(--accent-primary);
}

.scale-info-box {
  flex: 1;
  min-width: 0;
}

.scale-info-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.scale-name {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-primary);
}

.scale-pill {
  font-size: 9.5px;
  font-weight: 700;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--badge-pool-bg);
  color: var(--badge-pool-text);
  border: 1px solid var(--badge-pool-border);
}

.scale-pill-neutral {
  font-size: 9.5px;
  font-weight: 600;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--badge-neutral-bg);
  color: var(--badge-neutral-text);
}

.scale-pill-accent {
  font-size: 9.5px;
  font-weight: 600;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--accent-bg-subtle);
  color: var(--accent-primary);
}

.scale-desc {
  font-size: 11px;
  color: var(--text-secondary);
  line-height: 1.3;
  margin-top: 2px;
  white-space: normal;
}

.scale-check-icon {
  color: var(--accent-primary);
  flex-shrink: 0;
}

.scale-divider {
  height: 1px;
  background: var(--border-subtle);
  margin: 2px 0;
}

.scale-menu-footer {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  background: var(--bg-elevated);
  border-radius: 4px;
}

/* Animations */
.dropdown-fade-enter-active,
.dropdown-fade-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.dropdown-fade-enter-from,
.dropdown-fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
</style>
