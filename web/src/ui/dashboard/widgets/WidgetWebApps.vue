<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue';
import {
  mdiOpenInNew,
  mdiChevronRight,
  mdiWeb,
  mdiChevronDown,
} from '@mdi/js';
import { nasState } from '../../store';
import { navigateTo } from '../../router';
import SkeletonBox from '../../components/SkeletonBox.vue';
import MdiIcon from '../../components/MdiIcon.vue';
import {
  collectWebApps,
  openWebUiPortal,
  type WebAppLauncherItem,
  type WebPortalLink,
} from '../webAppsHelper';

const dockerApps = computed(() => nasState.dockerApps || []);
const catalogApps = computed(() => nasState.catalogApps || []);

const webApps = computed(() => collectWebApps(dockerApps.value, catalogApps.value));

const totalCount = computed(() => webApps.value.length);
const runningCount = computed(() => webApps.value.filter((a) => a.isRunning).length);

const failedIcons = ref<Record<string, boolean>>({});
const activePortalMenu = ref<string | null>(null);

function handleImageError(app: WebAppLauncherItem) {
  // If original icon failed, try fallback icon once
  if (app.iconUrl !== app.fallbackIconUrl && app.fallbackIconUrl) {
    app.iconUrl = app.fallbackIconUrl;
  } else {
    failedIcons.value[app.appId] = true;
  }
}

function handleTileClick(app: WebAppLauncherItem, e: MouseEvent) {
  // If clicking on the multi-port selector toggle, don't open primary directly
  if ((e.target as HTMLElement)?.closest('.multi-port-trigger')) {
    return;
  }
  openWebUiPortal(app.primaryUrl);
}

function togglePortalMenu(appId: string, e: MouseEvent) {
  e.stopPropagation();
  if (activePortalMenu.value === appId) {
    activePortalMenu.value = null;
  } else {
    activePortalMenu.value = appId;
  }
}

function openPortalLink(portal: WebPortalLink) {
  openWebUiPortal(portal.url);
  activePortalMenu.value = null;
}

function handleGlobalClick(e: MouseEvent) {
  if (
    !(e.target as HTMLElement)?.closest('.portal-dropdown-menu') &&
    !(e.target as HTMLElement)?.closest('.multi-port-trigger')
  ) {
    activePortalMenu.value = null;
  }
}

onMounted(() => {
  window.addEventListener('click', handleGlobalClick);
});

onUnmounted(() => {
  window.removeEventListener('click', handleGlobalClick);
});
</script>

<template>
  <div class="card workload-card webapps-card">
    <!-- Header -->
    <div class="card-header workload-header">
      <div class="header-left">
        <div class="header-icon-box">
          <svg viewBox="0 0 24 24" class="header-icon"><path :d="mdiOpenInNew" /></svg>
        </div>
        <h3 class="card-title">Web Applications</h3>
        <span class="badge badge-neutral mono count-badge" :title="`${runningCount} online out of ${totalCount} web applications`">
          {{ runningCount }}/{{ totalCount }}
        </span>
      </div>
      <button class="header-action-btn" @click="navigateTo('docker')" title="Manage Apps & Containers">
        <span>Manage</span>
        <svg viewBox="0 0 24 24" class="chevron-svg"><path :d="mdiChevronRight" /></svg>
      </button>
    </div>

    <!-- Loading Skeleton -->
    <div v-if="nasState.loading && totalCount === 0" class="webapps-grid">
      <div v-for="s in 4" :key="s" class="webapp-tile skeleton-tile">
        <SkeletonBox width="46px" height="46px" rounded="10px" />
        <SkeletonBox width="52px" height="11px" rounded="3px" style="margin-top: 6px" />
        <SkeletonBox width="38px" height="10px" rounded="3px" style="margin-top: 4px" />
      </div>
    </div>

    <!-- Empty State -->
    <div v-else-if="totalCount === 0" class="empty-state">
      <div class="empty-icon-wrap">
        <svg viewBox="0 0 24 24" class="empty-icon"><path :d="mdiWeb" /></svg>
      </div>
      <span>No installed applications exposing Web UI ports found.</span>
      <button class="btn btn-xs btn-secondary" @click="navigateTo('docker')">Explore Catalog</button>
    </div>

    <!-- App Launcher Grid -->
    <div v-else class="webapps-grid">
      <div
        v-for="app in webApps"
        :key="app.appId"
        class="webapp-tile"
        :class="{ 'app-stopped': !app.isRunning }"
        @click="handleTileClick(app, $event)"
        role="button"
        tabindex="0"
        @keydown.enter="openWebUiPortal(app.primaryUrl)"
        @keydown.space.prevent="openWebUiPortal(app.primaryUrl)"
      >
        <!-- App Icon Box with Live Status Badge -->
        <div class="icon-wrapper" :title="app.tooltip">
          <div class="icon-box">
            <img
              v-if="app.iconUrl && !failedIcons[app.appId]"
              :src="app.iconUrl"
              :alt="app.title"
              class="app-img"
              loading="lazy"
              @error="handleImageError(app)"
            />
            <div v-else class="icon-fallback">
              <MdiIcon :path="mdiWeb" size="24" class="fallback-svg" />
            </div>
          </div>

          <!-- Status Indicator Dot -->
          <span
            class="status-indicator"
            :class="app.isRunning ? 'status-online' : 'status-offline'"
            :title="app.isRunning ? 'Running' : app.state"
          />

          <!-- Quick Open External Indicator on Hover -->
          <div class="hover-launch-badge" title="Open in new tab">
            <MdiIcon :path="mdiOpenInNew" size="11" />
          </div>
        </div>

        <!-- Name with Tooltip -->
        <span
          class="app-name"
          :title="app.tooltip"
        >
          {{ app.title }}
        </span>

        <!-- Web Port Badge / Multi-Portal Selector -->
        <div class="port-badge-wrap">
          <span
            v-if="app.portals.length <= 1"
            class="port-badge mono"
            :title="`Exposed Port: ${app.primaryPort || 'Web'}`"
          >
            {{ app.primaryPort ? `:${app.primaryPort}` : 'Web' }}
          </span>

          <!-- Multi-portal dropdown trigger -->
          <button
            v-else
            type="button"
            class="port-badge port-badge-multi mono multi-port-trigger"
            :title="`Multiple Web UIs available: ${app.displayPorts}. Click to select.`"
            @click="togglePortalMenu(app.appId, $event)"
          >
            <span>:{{ app.primaryPort }}</span>
            <span class="multi-indicator">+{{ app.portals.length - 1 }}</span>
            <svg viewBox="0 0 24 24" class="drop-chevron"><path :d="mdiChevronDown" /></svg>
          </button>

          <!-- Portal Selection Dropdown Menu -->
          <div
            v-if="activePortalMenu === app.appId && app.portals.length > 1"
            class="portal-dropdown-menu"
            @click.stop
          >
            <div class="dropdown-header">Web UI Portals</div>
            <button
              v-for="portal in app.portals"
              :key="portal.url"
              type="button"
              class="portal-menu-item"
              @click="openPortalLink(portal)"
              :title="`Open ${portal.name} (${portal.url})`"
            >
              <div class="portal-item-meta">
                <span class="portal-item-name">{{ portal.name }}</span>
                <span class="portal-item-port mono">{{ portal.port ? `:${portal.port}` : '' }}</span>
              </div>
              <MdiIcon :path="mdiOpenInNew" size="12" class="portal-item-icon" />
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.webapps-card {
  height: auto;
  display: flex;
  flex-direction: column;
}

.workload-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding-bottom: 8px;
  margin-bottom: 12px;
  border-bottom: 1px solid var(--border-subtle);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.header-icon-box {
  width: 26px;
  height: 26px;
  border-radius: 6px;
  background: var(--accent-bg-subtle, rgba(56, 189, 248, 0.1));
  border: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.header-icon {
  width: 15px;
  height: 15px;
  fill: var(--accent-primary, #38bdf8);
}

.card-title {
  margin: 0;
  font-size: 13.5px;
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.count-badge {
  font-size: 10px;
  padding: 1px 6px;
  flex-shrink: 0;
}

.header-action-btn {
  background: transparent;
  border: 1px solid var(--border-subtle);
  border-radius: 5px;
  padding: 2px 7px;
  font-size: 11px;
  font-weight: 500;
  color: var(--text-secondary);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  flex-shrink: 0;
  transition: all 0.15s ease;
}

.header-action-btn:hover {
  color: var(--text-primary);
  border-color: var(--border-strong);
  background: var(--bg-hover, rgba(255, 255, 255, 0.04));
}

.chevron-svg {
  width: 14px;
  height: 14px;
  fill: currentColor;
  transition: transform 0.15s ease;
}

.header-action-btn:hover .chevron-svg {
  transform: translateX(1px);
}

/* Launcher Grid */
.webapps-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(78px, 1fr));
  gap: 10px 8px;
  padding: 2px;
  max-height: 420px;
  overflow-y: auto;
}

.webapp-tile {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 8px 4px 6px;
  border-radius: 8px;
  background: var(--bg-surface-elevated, rgba(255, 255, 255, 0.02));
  border: 1px solid var(--border-subtle);
  cursor: pointer;
  text-align: center;
  position: relative;
  transition: all 0.18s cubic-bezier(0.16, 1, 0.3, 1);
  user-select: none;
}

.webapp-tile:hover {
  background: var(--bg-hover, rgba(255, 255, 255, 0.07));
  border-color: var(--accent-primary, #38bdf8);
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.25);
}

.webapp-tile:active {
  transform: translateY(0);
}

.webapp-tile.app-stopped {
  opacity: 0.72;
}

.webapp-tile.app-stopped:hover {
  opacity: 1;
}

.webapp-tile.skeleton-tile {
  cursor: default;
  pointer-events: none;
}

/* Icon Box & Status */
.icon-wrapper {
  position: relative;
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 6px;
}

.icon-box {
  width: 100%;
  height: 100%;
  border-radius: 10px;
  background: var(--bg-surface, rgba(0, 0, 0, 0.2));
  border: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  box-shadow: inset 0 1px 1px rgba(255, 255, 255, 0.04);
  transition: border-color 0.15s ease;
}

.webapp-tile:hover .icon-box {
  border-color: rgba(56, 189, 248, 0.4);
}

.app-img {
  width: 36px;
  height: 36px;
  object-fit: contain;
  border-radius: 4px;
}

.icon-fallback {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(56, 189, 248, 0.08);
}

.fallback-svg {
  color: var(--accent-primary, #38bdf8);
}

/* Live Status Dot */
.status-indicator {
  position: absolute;
  bottom: -1px;
  right: -1px;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  border: 2px solid var(--bg-surface-elevated, #18181b);
  box-sizing: content-box;
}

.status-online {
  background: #10b981;
  box-shadow: 0 0 6px rgba(16, 185, 129, 0.7);
}

.status-offline {
  background: var(--text-muted, #71717a);
  opacity: 0.6;
}

/* Hover Launch Overlay Badge */
.hover-launch-badge {
  position: absolute;
  top: -4px;
  right: -4px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--accent-primary, #38bdf8);
  color: #0f172a;
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transform: scale(0.6);
  transition: all 0.18s ease;
  pointer-events: none;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.3);
}

.webapp-tile:hover .hover-launch-badge {
  opacity: 1;
  transform: scale(1);
}

/* Title & Ports */
.app-name {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--text-primary);
  width: 100%;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  padding: 0 2px;
  line-height: 1.25;
}

.port-badge-wrap {
  position: relative;
  margin-top: 3px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.port-badge {
  font-size: 10px;
  font-weight: 600;
  padding: 1px 5px;
  border-radius: 3px;
  background: rgba(255, 255, 255, 0.05);
  color: var(--text-secondary);
  border: 1px solid var(--border-subtle);
  letter-spacing: 0.2px;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  max-width: 74px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.webapp-tile:hover .port-badge {
  color: var(--accent-primary, #38bdf8);
  background: rgba(56, 189, 248, 0.08);
  border-color: rgba(56, 189, 248, 0.25);
}

.port-badge-multi {
  cursor: pointer;
  background: rgba(56, 189, 248, 0.08);
  color: var(--accent-primary, #38bdf8);
  border-color: rgba(56, 189, 248, 0.25);
}

.port-badge-multi:hover {
  background: rgba(56, 189, 248, 0.16);
  border-color: var(--accent-primary, #38bdf8);
}

.multi-indicator {
  font-size: 9px;
  opacity: 0.85;
}

.drop-chevron {
  width: 10px;
  height: 10px;
  fill: currentColor;
}

/* Multi-Portal Popover Menu */
.portal-dropdown-menu {
  position: absolute;
  top: calc(100% + 4px);
  left: 50%;
  transform: translateX(-50%);
  min-width: 140px;
  background: var(--bg-surface-floating, #1f2937);
  border: 1px solid var(--border-strong, #374151);
  border-radius: 6px;
  padding: 4px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.5);
  z-index: 50;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.dropdown-header {
  font-size: 10px;
  font-weight: 700;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  padding: 3px 6px;
  text-align: left;
  border-bottom: 1px solid var(--border-subtle);
  margin-bottom: 2px;
}

.portal-menu-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 5px 7px;
  border-radius: 4px;
  background: transparent;
  border: none;
  color: var(--text-primary);
  font-size: 11px;
  cursor: pointer;
  text-align: left;
  transition: all 0.12s ease;
}

.portal-menu-item:hover {
  background: var(--bg-hover, rgba(255, 255, 255, 0.08));
  color: var(--accent-primary, #38bdf8);
}

.portal-item-meta {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.portal-item-name {
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.portal-item-port {
  font-size: 9.5px;
  color: var(--text-muted);
}

.portal-item-icon {
  flex-shrink: 0;
  color: var(--text-secondary);
}

/* Empty State */
.empty-state {
  padding: 24px 12px;
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: var(--text-muted);
  font-size: 12px;
}

.empty-icon-wrap {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.04);
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 2px;
}

.empty-icon {
  width: 20px;
  height: 20px;
  fill: var(--text-muted);
}
</style>
