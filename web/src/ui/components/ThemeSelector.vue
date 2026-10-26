<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import {
  mdiCheck,
  mdiChevronDown,
  mdiPaletteOutline,
  mdiWeatherNight,
  mdiWeatherSunny,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import {
  activeTheme,
  activeThemeId,
  applyTheme,
  darkThemes,
  isDarkMode,
  lightThemes,
  type ThemeId,
} from '../theme';

const isOpen = ref(false);
const dropdownRef = ref<HTMLDivElement | null>(null);

function toggleDropdown() {
  isOpen.value = !isOpen.value;
}

function selectTheme(themeId: ThemeId) {
  applyTheme(themeId);
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
  <div ref="dropdownRef" class="theme-selector-container">
    <button
      type="button"
      class="btn btn-sm theme-toggle-btn"
      :class="{ 'btn-open': isOpen }"
      :title="`Current theme: ${activeTheme.name} (${activeTheme.mode}). Click to change.`"
      @click="toggleDropdown"
    >
      <MdiIcon
        :path="isDarkMode ? mdiWeatherNight : mdiWeatherSunny"
        size="15"
        class="theme-icon"
      />
      <span class="theme-active-name">{{ activeTheme.name }}</span>
      <MdiIcon
        :path="mdiChevronDown"
        size="14"
        class="theme-chevron"
        :class="{ rotated: isOpen }"
      />
    </button>

    <!-- Theme Dropdown Popover -->
    <transition name="dropdown-fade">
      <div v-if="isOpen" class="theme-dropdown-menu">
        <div class="theme-menu-header">
          <MdiIcon :path="mdiPaletteOutline" size="16" style="color: var(--accent-primary)" />
          <span>Color Themes</span>
        </div>

        <!-- Dark Themes Section -->
        <div class="theme-group">
          <div class="theme-group-label">
            <MdiIcon :path="mdiWeatherNight" size="13" />
            <span>Dark Themes</span>
          </div>
          <div class="theme-options-list">
            <button
              v-for="t in darkThemes"
              :key="t.id"
              type="button"
              class="theme-option"
              :class="{ selected: activeThemeId === t.id }"
              @click="selectTheme(t.id)"
            >
              <div class="theme-preview-dots">
                <span class="preview-dot" :style="{ backgroundColor: t.preview.bg }"></span>
                <span class="preview-dot" :style="{ backgroundColor: t.preview.surface }"></span>
                <span class="preview-dot" :style="{ backgroundColor: t.preview.accent }"></span>
              </div>
              <div class="theme-info-box">
                <div class="theme-info-row">
                  <span class="theme-name">{{ t.name }}</span>
                  <span v-if="t.id === 'tokyo-night'" class="theme-pill">Featured</span>
                </div>
                <div class="theme-desc">{{ t.description }}</div>
              </div>
              <MdiIcon
                v-if="activeThemeId === t.id"
                :path="mdiCheck"
                size="16"
                class="theme-check-icon"
              />
            </button>
          </div>
        </div>

        <div class="theme-divider"></div>

        <!-- Light Themes Section -->
        <div class="theme-group">
          <div class="theme-group-label">
            <MdiIcon :path="mdiWeatherSunny" size="13" />
            <span>Light Themes (High Contrast)</span>
          </div>
          <div class="theme-options-list">
            <button
              v-for="t in lightThemes"
              :key="t.id"
              type="button"
              class="theme-option"
              :class="{ selected: activeThemeId === t.id }"
              @click="selectTheme(t.id)"
            >
              <div class="theme-preview-dots">
                <span class="preview-dot" :style="{ backgroundColor: t.preview.bg }"></span>
                <span class="preview-dot" :style="{ backgroundColor: t.preview.surface }"></span>
                <span class="preview-dot" :style="{ backgroundColor: t.preview.accent }"></span>
              </div>
              <div class="theme-info-box">
                <div class="theme-info-row">
                  <span class="theme-name">{{ t.name }}</span>
                  <span v-if="t.id === 'github-light'" class="theme-pill high-contrast">WCAG AAA</span>
                </div>
                <div class="theme-desc">{{ t.description }}</div>
              </div>
              <MdiIcon
                v-if="activeThemeId === t.id"
                :path="mdiCheck"
                size="16"
                class="theme-check-icon"
              />
            </button>
          </div>
        </div>
      </div>
    </transition>
  </div>
</template>

<style scoped>
.theme-selector-container {
  position: relative;
  display: inline-block;
}

.theme-toggle-btn {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 5px 11px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-strong);
  color: var(--text-primary);
  border-radius: 7px;
  font-size: 12.5px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
  user-select: none;
}

.theme-toggle-btn:hover,
.theme-toggle-btn.btn-open {
  background: var(--bg-hover);
  border-color: var(--accent-primary);
}

.theme-icon {
  color: var(--accent-primary);
  flex-shrink: 0;
}

.theme-active-name {
  font-weight: 600;
  white-space: nowrap;
}

.theme-chevron {
  color: var(--text-secondary);
  transition: transform 0.2s ease;
}

.theme-chevron.rotated {
  transform: rotate(180deg);
}

.theme-dropdown-menu {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  width: 310px;
  background: var(--bg-surface);
  border: 1px solid var(--border-strong);
  border-radius: 10px;
  box-shadow: var(--modal-shadow);
  z-index: 1000;
  padding: 8px;
  display: flex;
  flex-direction: column;
}

.theme-menu-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px 10px 10px;
  font-size: 12.5px;
  font-weight: 700;
  color: var(--text-primary);
  border-bottom: 1px solid var(--border-subtle);
  margin-bottom: 6px;
}

.theme-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.theme-group-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-secondary);
  padding: 6px 8px 4px 8px;
}

.theme-options-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.theme-option {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 7px 10px;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  cursor: pointer;
  text-align: left;
  transition: all 0.12s ease;
  color: var(--text-primary);
}

.theme-option:hover {
  background: var(--bg-hover);
  border-color: var(--border-subtle);
}

.theme-option.selected {
  background: var(--accent-bg-subtle);
  border-color: var(--accent-primary);
}

.theme-preview-dots {
  display: flex;
  align-items: center;
  gap: 3px;
  flex-shrink: 0;
}

.preview-dot {
  display: inline-block;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  border: 1px solid rgba(0, 0, 0, 0.2);
}

.theme-info-box {
  flex: 1;
  min-width: 0;
}

.theme-info-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.theme-name {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-primary);
}

.theme-pill {
  font-size: 9.5px;
  font-weight: 700;
  text-transform: uppercase;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--accent-bg-subtle);
  color: var(--accent-primary);
  border: 1px solid var(--accent-primary);
}

.theme-pill.high-contrast {
  background: var(--badge-success-bg);
  color: var(--success-text);
  border: 1px solid var(--success-border);
}

.theme-desc {
  font-size: 11px;
  color: var(--text-secondary);
  line-height: 1.3;
  margin-top: 1px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.theme-check-icon {
  color: var(--accent-primary);
  flex-shrink: 0;
}

.theme-divider {
  height: 1px;
  background: var(--border-subtle);
  margin: 6px 0;
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
</style>
