<script setup lang="ts">
import { computed, ref } from 'vue';
import {
  mdiArrowLeft,
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
import { nasState, refreshAllData, selectResource } from '../store';
import { navigateTo } from '../router';
import MobileOverview from './views/MobileOverview.vue';
import MobileAppsView from './views/MobileAppsView.vue';
import MobileStorageView from './views/MobileStorageView.vue';
import MobileKvmView from './views/MobileKvmView.vue';
import MobilePowerSheet from './components/MobilePowerSheet.vue';
import MobileNavDrawer from './components/MobileNavDrawer.vue';
import MobileAppConfigEditor from './components/MobileAppConfigEditor.vue';
import type { LxcContainer } from '../../domain/models';

// Import detail/configuration views for full editing on mobile
import DockerDetailView from '../views/DockerDetailView.vue';
import VmDetailView from '../views/VmDetailView.vue';
import LxcDetailView from '../views/LxcDetailView.vue';
import PoolDetailView from '../views/PoolDetailView.vue';

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
  (e: 'open-update'): void;
  (e: 'open-vm-console', vmName: string): void;
  (e: 'open-lxc-terminal', c: LxcContainer): void;
  (e: 'logout'): void;
}>();

const isPowerSheetOpen = ref(false);
const isDrawerOpen = ref(false);

// Register overlay dismissal for mobile / Android back button
if (typeof window !== 'undefined') {
  const prevDismiss = (window as any).nanDismissTopOverlay;
  (window as any).nanDismissTopOverlay = () => {
    if (isPowerSheetOpen.value) {
      isPowerSheetOpen.value = false;
      return true;
    }
    if (isDrawerOpen.value) {
      isDrawerOpen.value = false;
      return true;
    }
    if (typeof prevDismiss === 'function') {
      return prevDismiss();
    }
    return false;
  };
}

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

// Active selected resources for full configuration / detail editing on mobile
const activeSelectedPool = computed<string | null>(() => {
  if (
    (activeTab.value === 'storage-pools' || activeTab.value === 'zfs') &&
    nasState.selectedResourceId &&
    nasState.selectedResourceId.startsWith('pool:')
  ) {
    return nasState.selectedResourceId.replace(/^pool:/, '').trim() || null;
  }
  return null;
});

const activeSelectedVm = computed<string | null>(() => {
  if (
    activeTab.value === 'kvm' &&
    nasState.selectedResourceId &&
    nasState.selectedResourceId.startsWith('vm:')
  ) {
    return nasState.selectedResourceId.replace(/^vm:/, '').replace(/^virtualMachines\//, '').trim() || null;
  }
  return null;
});

const activeSelectedLxc = computed<string | null>(() => {
  if (
    activeTab.value === 'lxc' &&
    nasState.selectedResourceId &&
    nasState.selectedResourceId.startsWith('lxc:')
  ) {
    const clean = nasState.selectedResourceId.replace(/^lxc:/, '').replace(/^lxcContainers\//, '').trim();
    const found = nasState.lxcContainers.find(
      (c) =>
        c.name === clean ||
        c.name === `lxcContainers/${clean}` ||
        c.containerName === clean ||
        c.name.replace(/^lxcContainers\//, '') === clean
    );
    if (found) {
      return found.name.replace(/^lxcContainers\//, '');
    }
    return clean || null;
  }
  return null;
});

const activeSelectedDocker = computed<string | null>(() => {
  if (
    activeTab.value === 'docker' &&
    nasState.selectedResourceId &&
    nasState.selectedResourceId.startsWith('docker:')
  ) {
    return nasState.selectedResourceId.replace(/^docker:/, '').replace(/^dockerApps\//, '').trim() || null;
  }
  return null;
});

const activeSelectedDockerApp = computed(() => {
  if (!activeSelectedDocker.value) return null;
  const target = activeSelectedDocker.value;
  return (
    nasState.dockerApps.find(
      (a) =>
        a.appId === target ||
        a.name === target ||
        a.name === `dockerApps/${target}`
    ) || null
  );
});

interface DetailSubTabItem {
  id: string;
  label: string;
}

const activeDetailContext = computed<{
  kind: 'docker' | 'vm' | 'lxc' | 'pool';
  title: string;
  badge: string;
  tabs: DetailSubTabItem[];
  currentTab: string;
  onBack: () => void;
  onSelectTab: (tabId: string) => void;
} | null>(() => {
  if (activeSelectedDocker.value) {
    const app = nasState.dockerApps.find(
      (a) =>
        a.appId === activeSelectedDocker.value ||
        a.name === activeSelectedDocker.value ||
        a.name === `dockerApps/${activeSelectedDocker.value}`
    );
    const current = nasState.dockerSubTab === 'compose' ? 'configuration' : nasState.dockerSubTab;
    return {
      kind: 'docker',
      title: app?.title || activeSelectedDocker.value,
      badge: 'Docker App',
      tabs: [
        { id: 'configuration', label: 'Configuration' },
        { id: 'summary', label: 'Summary' },
        { id: 'containers', label: 'Containers' },
        { id: 'logs', label: 'Logs' },
        { id: 'console', label: 'Console' },
      ],
      currentTab: current,
      onBack: () => selectResource('node:docker'),
      onSelectTab: (id) => {
        nasState.dockerSubTab = id as any;
      },
    };
  }
  if (activeSelectedVm.value) {
    const current = nasState.vmSubTab === 'hardware' ? 'configuration' : nasState.vmSubTab;
    return {
      kind: 'vm',
      title: activeSelectedVm.value,
      badge: 'KVM VM',
      tabs: [
        { id: 'configuration', label: 'Configuration' },
        { id: 'summary', label: 'Summary' },
        { id: 'snapshots', label: 'Snapshots' },
        { id: 'console', label: 'Console' },
      ],
      currentTab: current,
      onBack: () => selectResource('node:vms'),
      onSelectTab: (id) => {
        nasState.vmSubTab = id as any;
      },
    };
  }
  if (activeSelectedLxc.value) {
    const ct = nasState.lxcContainers.find(
      (c) =>
        c.name.replace(/^lxcContainers\//, '') === activeSelectedLxc.value ||
        c.containerName === activeSelectedLxc.value
    );
    const current =
      nasState.lxcSubTab === 'mounts'
        ? 'configuration'
        : nasState.lxcSubTab === 'terminal'
        ? 'console'
        : nasState.lxcSubTab;
    return {
      kind: 'lxc',
      title: ct?.containerName || activeSelectedLxc.value,
      badge: 'LXC',
      tabs: [
        { id: 'configuration', label: 'Configuration' },
        { id: 'summary', label: 'Summary' },
        { id: 'logs', label: 'Logs' },
        { id: 'console', label: 'Console' },
      ],
      currentTab: current,
      onBack: () => selectResource('node:lxc'),
      onSelectTab: (id) => {
        nasState.lxcSubTab = id as any;
      },
    };
  }
  if (activeSelectedPool.value) {
    return {
      kind: 'pool',
      title: activeSelectedPool.value,
      badge: 'ZFS Pool',
      tabs: [
        { id: 'config', label: 'Config' },
        { id: 'summary', label: 'Summary' },
        { id: 'datasets', label: 'Datasets' },
        { id: 'volumes', label: 'Volumes' },
        { id: 'topology', label: 'Topology' },
      ],
      currentTab: nasState.poolSubTab,
      onBack: () => selectResource('node:pools'),
      onSelectTab: (id) => {
        nasState.poolSubTab = id as any;
      },
    };
  }
  return null;
});

const isFaulted = computed(() => {
  return (nasState.pools || []).some(
    (p) => p.state && ['FAULTED', 'UNAVAIL', 'SUSPENDED'].includes(p.state.toUpperCase())
  );
});

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
        <span
          class="health-indicator"
          :class="isFaulted ? 'health-danger' : isDegraded ? 'health-warning' : 'health-ok'"
        />
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
      <!-- Full Resource Detail & Configuration Editor (reused from desktop with mobile shell) -->
      <template v-if="activeDetailContext">
        <div class="mobile-detail-header">
          <div class="mobile-detail-topbar">
            <button
              class="mobile-detail-back-btn"
              type="button"
              @click="activeDetailContext.onBack"
            >
              <MdiIcon :path="mdiArrowLeft" size="18" />
              <span>Back</span>
            </button>
            <div class="mobile-detail-title-wrap">
              <span class="mobile-detail-title">{{ activeDetailContext.title }}</span>
              <span class="mobile-detail-badge">{{ activeDetailContext.badge }}</span>
            </div>
          </div>
          <div class="mobile-detail-subtabs">
            <button
              v-for="tab in activeDetailContext.tabs"
              :key="tab.id"
              type="button"
              class="mobile-detail-tab"
              :class="{ active: activeDetailContext.currentTab === tab.id }"
              @click="activeDetailContext.onSelectTab(tab.id)"
            >
              {{ tab.label }}
            </button>
          </div>
        </div>

        <div class="mobile-detail-wrap">
          <MobileAppConfigEditor
            v-if="
              activeDetailContext.kind === 'docker' &&
              activeSelectedDockerApp &&
              (nasState.dockerSubTab === 'configuration' || nasState.dockerSubTab === 'compose')
            "
            :key="activeSelectedDockerApp.name"
            :app="activeSelectedDockerApp"
          />
          <DockerDetailView
            v-else-if="activeDetailContext.kind === 'docker' && activeSelectedDocker"
            :app-name="activeSelectedDocker"
            :active-sub-tab="nasState.dockerSubTab"
            @update:active-sub-tab="nasState.dockerSubTab = $event"
            @back="selectResource('node:docker')"
          />
          <VmDetailView
            v-else-if="activeDetailContext.kind === 'vm' && activeSelectedVm"
            :vm-name="activeSelectedVm"
            :active-sub-tab="nasState.vmSubTab"
            @update:active-sub-tab="nasState.vmSubTab = $event"
            @back="selectResource('node:vms')"
          />
          <LxcDetailView
            v-else-if="activeDetailContext.kind === 'lxc' && activeSelectedLxc"
            :container-name="activeSelectedLxc"
            :active-sub-tab="nasState.lxcSubTab"
            @update:active-sub-tab="nasState.lxcSubTab = $event"
            @back="selectResource('node:lxc')"
          />
          <PoolDetailView
            v-else-if="activeDetailContext.kind === 'pool' && activeSelectedPool"
            :pool-name="activeSelectedPool"
            :active-sub-tab="nasState.poolSubTab"
            @update:active-sub-tab="nasState.poolSubTab = $event"
            @back="selectResource('node:pools')"
          />
        </div>
      </template>

      <!-- High-Impact Dedicated Mobile Views -->
      <MobileOverview
        v-else-if="isOverview"
        @open-terminal="emit('open-terminal')"
        @open-system-details="emit('open-system-details')"
        @open-update="emit('open-update')"
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
        <SharesView
          v-if="activeTab === 'shares' || activeTab === 'shares-cifs' || activeTab === 'shares-webdav'"
          :active-subtab="activeTab === 'shares-webdav' ? 'webdav' : activeTab === 'shares-cifs' ? 'cifs' : 'all'"
        />
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
        @click="navigateTo('overview', false, 'node:local')"
      >
        <div class="nav-icon-wrap">
          <MdiIcon :path="mdiMonitor" size="20" />
        </div>
        <span class="nav-tab-label">Home</span>
      </button>

      <button
        class="nav-tab-btn"
        :class="{ active: isApps }"
        @click="navigateTo('docker', false, 'node:docker')"
      >
        <div class="nav-icon-wrap">
          <MdiIcon :path="mdiDocker" size="20" />
        </div>
        <span class="nav-tab-label">Apps</span>
      </button>

      <button
        class="nav-tab-btn"
        :class="{ active: isStorage }"
        @click="navigateTo('storage-pools', false, 'node:pools')"
      >
        <div class="nav-icon-wrap">
          <MdiIcon :path="mdiDatabase" size="20" />
        </div>
        <span class="nav-tab-label">Storage</span>
      </button>

      <button
        class="nav-tab-btn"
        :class="{ active: isKvm }"
        @click="navigateTo('kvm', false, 'node:vms')"
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
      @open-update="emit('open-update')"
      @logout="emit('logout')"
    />

    <!-- Secondary Navigation Drawer -->
    <MobileNavDrawer
      v-model="isDrawerOpen"
      @open-terminal="emit('open-terminal')"
      @open-system-details="emit('open-system-details')"
      @open-prerequisites="emit('open-prerequisites')"
      @open-update="emit('open-update')"
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
  overscroll-behavior-y: none;
  background: var(--bg-base, #0f172a);
  color: var(--text-primary, #f8fafc);
}

.mobile-top-bar {
  min-height: 52px;
  height: calc(52px + env(safe-area-inset-top, 0px));
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 14px;
  background: var(--bg-surface, #1e293b);
  border-bottom: 1px solid var(--border-color, #334155);
  padding-top: env(safe-area-inset-top, 0px);
  box-sizing: border-box;
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
  background: #f59e0b;
  box-shadow: 0 0 6px #f59e0b;
}

.health-indicator.health-danger {
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
  overscroll-behavior-y: contain;
  -webkit-overflow-scrolling: touch;
  padding-bottom: calc(80px + env(safe-area-inset-bottom, 0px));
  scroll-padding-bottom: calc(80px + env(safe-area-inset-bottom, 0px));
}

.mobile-fallback-wrap {
  padding: 12px;
  overscroll-behavior-y: contain;
}

.mobile-detail-header {
  position: sticky;
  top: 0;
  z-index: 40;
  background: var(--bg-surface, #1e293b);
  border-bottom: 1px solid var(--border-color, #334155);
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px;
}

.mobile-detail-topbar {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.mobile-detail-back-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 6px 10px;
  border-radius: 8px;
  background: var(--bg-base, #0f172a);
  border: 1px solid var(--border-color, #334155);
  color: var(--text-primary, #f8fafc);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  flex-shrink: 0;
  touch-action: manipulation;
}

.mobile-detail-back-btn:active {
  transform: scale(0.96);
}

.mobile-detail-title-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
}

.mobile-detail-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mobile-detail-badge {
  font-size: 10px;
  font-weight: 700;
  padding: 2px 6px;
  border-radius: 999px;
  background: rgba(56, 189, 248, 0.14);
  color: var(--accent-primary, #38bdf8);
  white-space: nowrap;
  flex-shrink: 0;
}

.mobile-detail-subtabs {
  display: flex;
  gap: 6px;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
  scrollbar-width: none;
  padding-bottom: 2px;
}

.mobile-detail-subtabs::-webkit-scrollbar {
  display: none;
}

.mobile-detail-tab {
  padding: 6px 12px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  background: var(--bg-base, #0f172a);
  border: 1px solid var(--border-color, #334155);
  color: var(--text-secondary, #94a3b8);
  white-space: nowrap;
  cursor: pointer;
  flex-shrink: 0;
  touch-action: manipulation;
}

.mobile-detail-tab.active {
  background: var(--accent-primary, #38bdf8);
  color: #0f172a;
  border-color: var(--accent-primary, #38bdf8);
}

.mobile-detail-wrap {
  padding: 12px;
  max-width: 100%;
  overflow-x: hidden;
}

.mobile-detail-wrap :deep(.summary-grid),
.mobile-detail-wrap :deep(.grid-2),
.mobile-detail-wrap :deep(.form-grid) {
  grid-template-columns: 1fr !important;
}

.mobile-detail-wrap :deep(.table-wrap) {
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}

.mobile-detail-wrap :deep(.card) {
  max-width: 100%;
  box-sizing: border-box;
}

.mobile-detail-wrap :deep(.card-header) {
  flex-wrap: wrap;
  gap: 8px;
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
