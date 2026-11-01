<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import {
  mdiCheck,
  mdiFormatFontSizeDecrease,
  mdiFormatFontSizeIncrease,
  mdiFormatSize,
  mdiMonitorEye,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import {
  activeFontLabel,
  activeFontSizeId,
  applyFontSize,
  currentFontOption,
  detectHiDpiRecommendation,
  FONT_SIZE_OPTIONS,
  type FontSizeId,
  stepFontSize,
} from '../fontSize';

const isOpen = ref(false);
const dropdownRef = ref<HTMLDivElement | null>(null);

const recommendedId = computed<FontSizeId>(() => {
  return detectHiDpiRecommendation();
});

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

function selectFontSize(id: FontSizeId) {
  applyFontSize(id);
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
  <div ref="dropdownRef" class="font-size-selector-container">
    <button
      type="button"
      class="btn btn-sm btn-icon font-size-toggle-btn"
      :class="{ 'btn-open': isOpen }"
      :title="`UI Font Size: ${activeFontLabel} — Click to adjust text size`"
      aria-label="Adjust UI Font Size"
      @click="toggleDropdown"
    >
      <MdiIcon :path="mdiFormatSize" size="16" class="font-size-icon" />
      <span class="sr-only">Font Size: {{ activeFontLabel }}</span>
    </button>

    <!-- Font Size Dropdown Popover -->
    <transition name="dropdown-fade">
      <div v-if="isOpen" class="font-size-dropdown-menu">
        <div class="font-size-menu-header">
          <MdiIcon :path="mdiFormatSize" size="16" style="color: var(--accent-primary)" />
          <span>UI Font Size</span>
          <span class="active-badge mono">{{ currentFontOption.percent }}%</span>
        </div>

        <!-- Quick Stepper Actions -->
        <div class="stepper-row">
          <button
            type="button"
            class="btn btn-xs btn-secondary stepper-btn"
            title="Decrease font size"
            @click.stop="stepFontSize('decrease')"
          >
            <MdiIcon :path="mdiFormatFontSizeDecrease" size="14" />
            <span>Smaller</span>
          </button>
          <button
            type="button"
            class="btn btn-xs btn-secondary stepper-btn"
            title="Increase font size"
            @click.stop="stepFontSize('increase')"
          >
            <MdiIcon :path="mdiFormatFontSizeIncrease" size="14" />
            <span>Larger</span>
          </button>
        </div>

        <div class="font-size-options-list">
          <button
            v-for="opt in FONT_SIZE_OPTIONS"
            :key="opt.id"
            type="button"
            class="font-size-option"
            :class="{ selected: activeFontSizeId === opt.id }"
            @click="selectFontSize(opt.id)"
          >
            <div class="font-size-info-box">
              <div class="font-size-info-row">
                <span class="font-size-name">{{ opt.label }}</span>
                <span
                  v-if="recommendedId === opt.id && opt.id !== 'default'"
                  class="font-size-pill-accent"
                >
                  Detected Screen
                </span>
                <span
                  v-else-if="opt.tag"
                  class="font-size-pill"
                >
                  {{ opt.tag }}
                </span>
              </div>
              <div class="font-size-desc">{{ opt.description }}</div>
            </div>
            <MdiIcon
              v-if="activeFontSizeId === opt.id"
              :path="mdiCheck"
              size="16"
              class="font-size-check-icon"
            />
          </button>
        </div>

        <div class="font-size-divider"></div>

        <!-- Screen Resolution Footer -->
        <div class="font-size-menu-footer">
          <MdiIcon :path="mdiMonitorEye" size="13" style="color: var(--text-muted)" />
          <span class="mono font-xs text-muted">{{ currentResolutionInfo }}</span>
        </div>
      </div>
    </transition>
  </div>
</template>

<style scoped>
.font-size-selector-container {
  position: relative;
  display: inline-flex;
}

.font-size-toggle-btn {
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

.font-size-toggle-btn:hover,
.font-size-toggle-btn.btn-open {
  background: var(--bg-hover);
  color: var(--text-primary);
  border-color: var(--border-strong);
}

.font-size-icon {
  pointer-events: none;
}

.font-size-dropdown-menu {
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

.font-size-menu-header {
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

.stepper-row {
  display: flex;
  gap: 6px;
  padding: 0 4px 4px 4px;
}

.stepper-btn {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  padding: 4px 8px;
  font-size: 11.5px;
}

.font-size-options-list {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.font-size-option {
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

.font-size-option:hover {
  background: var(--bg-hover);
}

.font-size-option.selected {
  background: var(--accent-bg-subtle);
  border-color: var(--accent-primary);
}

.font-size-info-box {
  flex: 1;
  min-width: 0;
}

.font-size-info-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.font-size-name {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-primary);
}

.font-size-pill {
  font-size: 9.5px;
  font-weight: 600;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--badge-neutral-bg);
  color: var(--badge-neutral-text);
}

.font-size-pill-accent {
  font-size: 9.5px;
  font-weight: 600;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--accent-bg-subtle);
  color: var(--accent-primary);
  border: 1px solid var(--accent-primary);
}

.font-size-desc {
  font-size: 11px;
  color: var(--text-secondary);
  line-height: 1.3;
  margin-top: 2px;
  white-space: normal;
}

.font-size-check-icon {
  color: var(--accent-primary);
  flex-shrink: 0;
}

.font-size-divider {
  height: 1px;
  background: var(--border-subtle);
  margin: 2px 0;
}

.font-size-menu-footer {
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
