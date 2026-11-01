<script setup lang="ts">
import { computed, ref } from 'vue';
import {
  mdiMonitor,
  mdiDocker,
  mdiDatabase,
  mdiLayers,
  mdiMenu,
  mdiRefresh,
  mdiConsole,
  mdiPower,
  mdiAlertCircle,
  mdiCheckCircle,
} from '@mdi/js';
import MdiIcon from '../components/MdiIcon.vue';
import NanLogo from '../components/NanLogo.vue';
import { nasState, refreshAllData } from '../store';
import { navigateTo } from '../router';
import MobileOverview from './views/MobileOverview.vue';
import MobileAppsView from './views/MobileAppsView.vue';
import MobileStorageView from './views/MobileStorageView.vue';
import MobileKvmView from './views/MobileKvmView.vue';
import MobilePowerSheet from './components/MobilePowerSheet.vue';
import MobileNavDrawer from './components/MobileNavDrawer.vue';
import type { LxcContainer } from '../../domain/models';

// Import views for fallback tabs
import SharesView from '../views/SharesView.vue';
import CronJobsView from '../views/CronJobsView.vue';
import IdentityView from '../views/IdentityView.vue';
import DatasetsView from '../views/DatasetsView.vue';
import VolumesView from '../views/VolumesView.vue';
import PrerequisitesView from '../views/PrerequisitesView.vue';

const emit = defineEmits<{
  (e: 'open-terminal'): void;
  (e: 'open-system-details'): void;
  (e: 'open-prerequisites'): void;
  (e: 'open-pool-modal'): void;
  (e: 'open-vm-console', vmName: string): void;
  (e: 'open-lxc-terminal', c: LxcContainer): void;
  (e: 'logout'): void;
}>();

const isPowerSheetOpen = ref(false);
const isDrawerOpen = ref(false);

const activeTab = computed(() => nasState.activeTab);

const isOverview = computed(() => activeTab.value === 'overview');
const isApps = computed(() => activeTab.value === 'docker');
const isStorage = computed(() => {
  return (
    activeTab.value === 'storage-pools' ||
    activeTab.value === 'storage-disks' ||
    activeTab.value === 'storage-datasets' ||
    activeTab.value === 'zfs'
  );
});
const isKvm = computed(() => activeTab.value === 'kvm' || activeTab.value === 'lxc');

const isDegraded = computed(() => {
  return (nasState.pools || []).some((p) => p.state && p.state !== 'ONLINE');
});
</script>

<template>
  <div class="mobile-shell">
    <!-- Top Mobile Appliance Bar -->
    <header class="mobile-top-bar">
      <div class="top-bar-left">
        <NanLogo :size="24" subtitle="Mobile" />
        <span class="host-pill">{{ nasState.systemTelemetry?.host?.hostname || 'nas01' }}</span>
        <span class="health-indicator" :class="isDegraded ? 'health-warning' : 'health-ok'" />
      </div>

      <div class="top-bar-right">
        <button
          class="btn btn-xs btn-ghost btn-icon"
          title="Refresh Data"
          :disabled="nasState.loading"
          @click="refreshAllData"
        >
          <MdiIcon :path="mdiRefresh" size="18" :class="{ 'spin-icon': nasState.loading }" />
        </button>

        <button
          class="btn btn-xs btn-ghost btn-icon"
          title="Host Root Shell"
          @click="emit('open-terminal')"
        >
          <MdiIcon :path="mdiConsole" size="18" style="color: var(--accent-primary)" />
        </button>

        <button
          class="btn btn-xs btn-ghost btn-icon"
          title="System & Power Menu"
          @click="isPowerSheetOpen = true"
        >
          <MdiIcon :path="mdiPower" size="18" style="color: #f87171" />
        </button>
      </div>
    </header>

    <!-- Scrollable Main Viewport -->
    <main class="mobile-main-viewport">
      <!-- High-Impact Dedicated Mobile Views -->
      <MobileOverview
        v-if="isOverview"
        @open-terminal="emit('open-terminal')"
        @open-system-details="emit('open-system-details')"
      />

      <MobileAppsView v-else-if="isApps" />

      <MobileStorageView v-else-if="isStorage" />

      <MobileKvmView
        v-else-if="isKvm"
        @open-vm-console="emit('open-vm-console', $event)"
        @open-lxc-terminal="emit('open-lxc-terminal', $event)"
      />

      <!-- Fallback tab views wrapped for touch scrolling -->
      <div v-else class="mobile-fallback-wrap">
        <SharesView v-if="activeTab === 'shares' || activeTab === 'shares-cifs' || activeTab === 'shares-webdav'" />
        <IdentityView v-else-if="activeTab === 'identity'" />
        <CronJobsView v-else-if="activeTab === 'cron'" />
        <DatasetsView v-else-if="activeTab === 'storage-datasets'" />
        <VolumesView v-else-if="activeTab === 'storage-volumes'" />
        <PrerequisitesView v-else-if="activeTab === 'prerequisites'" />
      </div>
    </main>

    <!-- Bottom Navigation Bar (5 Primary Touch Destinations) -->
    <nav class="mobile-bottom-nav">
      <button
        class="nav-tab-btn"
        :class="{ active: isOverview }"
        @click="navigateTo('overview')"
      >
        <div class="nav-icon-wrap">
          <MdiIcon :path="mdiMonitor" size="20" />
        </div>
        <span class="nav-tab-label">Home</span>
      </button>

      <button
        class="nav-tab-btn"
        :class="{ active: isApps }"
        @click="navigateTo('docker')"
      >
        <div class="nav-icon-wrap">
          <MdiIcon :path="mdiDocker" size="20" />
          <span v-if="(nasState.dockerApps || []).length > 0" class="nav-badge-dot" />
        </div>
        <span class="nav-tab-label">Apps</span>
      </button>

      <button
        class="nav-tab-btn"
        :class="{ active: isStorage }"
        @click="navigateTo('storage-pools')"
      >
        <div class="nav-icon-wrap">
          <MdiIcon :path="mdiDatabase" size="20" />
        </div>
        <span class="nav-tab-label">Storage</span>
      </button>

      <button
        class="nav-tab-btn"
        :class="{ active: isKvm }"
        @click="navigateTo('kvm')"
      >
        <div class="nav-icon-wrap">
          <MdiIcon :path="mdiLayers" size="20" />
        </div>
        <span class="nav-tab-label">VM / LXC</span>
      </button>

      <button
        class="nav-tab-btn"
        :class="{ active: isDrawerOpen }"
        @click="isDrawerOpen = true"
      >
        <div class="nav-icon-wrap">
          <MdiIcon :path="mdiMenu" size="20" />
        </div>
        <span class="nav-tab-label">More</span>
      </button>
    </nav>

    <!-- Power & Quick Actions Bottom Sheet -->
    <MobilePowerSheet
      v-model="isPowerSheetOpen"
      @open-terminal="emit('open-terminal')"
      @open-system-details="emit('open-system-details')"
      @open-pool-modal="emit('open-pool-modal')"
      @logout="emit('logout')"
    />

    <!-- Secondary Navigation Drawer -->
    <MobileNavDrawer
      v-model="isDrawerOpen"
      @open-terminal="emit('open-terminal')"
      @open-system-details="emit('open-system-details')"
      @open-prerequisites="emit('open-prerequisites')"
      @logout="emit('logout')"
    />
  </div>
</template>

<style scoped>
.mobile-shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  height: 100dvh;
  width: 100%;
  overflow: hidden;
  background: var(--bg-base, #0f172a);
  color: var(--text-primary, #f8fafc);
}

.mobile-top-bar {
  height: 52px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 14px;
  background: var(--bg-surface, #1e293b);
  border-bottom: 1px solid var(--border-color, #334155);
  padding-top: env(safe-area-inset-top, 0px);
  flex-shrink: 0;
  z-index: 100;
}

.top-bar-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.host-pill {
  font-size: 11px;
  font-family: ui-monospace, monospace;
  padding: 2px 7px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.08);
  color: var(--text-secondary, #94a3b8);
}

.health-indicator {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.health-indicator.health-ok {
  background: #4ade80;
  box-shadow: 0 0 6px #4ade80;
}

.health-indicator.health-warning {
  background: #f87171;
  box-shadow: 0 0 6px #f87171;
}

.top-bar-right {
  display: flex;
  align-items: center;
  gap: 4px;
}

.mobile-main-viewport {
  flex: 1;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
  padding-bottom: 16px;
}

.mobile-fallback-wrap {
  padding: 12px;
}

.mobile-bottom-nav {
  height: 58px;
  background: var(--bg-surface, #1e293b);
  border-top: 1px solid var(--border-color, #334155);
  display: flex;
  align-items: center;
  justify-content: space-around;
  padding: 0 6px calc(env(safe-area-inset-bottom, 0px)) 6px;
  box-sizing: content-box;
  flex-shrink: 0;
  z-index: 100;
}

.nav-tab-btn {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 6px 0;
  background: transparent;
  border: none;
  color: var(--text-secondary, #94a3b8);
  cursor: pointer;
  touch-action: manipulation;
  transition: color 0.15s ease, transform 0.1s ease;
  user-select: none;
  -webkit-user-select: none;
}

.nav-tab-btn:active {
  transform: scale(0.92);
}

.nav-tab-btn.active {
  color: var(--accent-primary, #38bdf8);
}

.nav-icon-wrap {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
}

.nav-badge-dot {
  position: absolute;
  top: -2px;
  right: -4px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #4ade80;
}

.nav-tab-label {
  font-size: 10px;
  font-weight: 700;
  margin-top: 2px;
  letter-spacing: 0.2px;
}
</style>
