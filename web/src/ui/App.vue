<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import {
  mdiAlertCircle,
  mdiArrowLeft,
  mdiCamera,
  mdiCellphone,
  mdiClose,
  mdiCogOutline,
  mdiConsole,
  mdiCubeOutline,
  mdiDatabase,
  mdiDatabaseCheck,
  mdiDocker,
  mdiFileDocumentOutline,
  mdiFolderOutline,
  mdiHarddisk,
  mdiInformationOutline,
  mdiLayers,
  mdiLogout,
  mdiMagnify,
  mdiMenu,
  mdiMenuDown,
  mdiMonitor,
  mdiOpenInNew,
  mdiPageLayoutSidebarLeft,
  mdiPencil,
  mdiRefresh,
  mdiUpdate,
} from '@mdi/js';
import { services } from '../container';
import {
  dismissToast,
  getModuleAvailability,
  initSession,
  type ManagedModule,
  nasState,
  type NavTab,
  notify,
  pauseToast,
  refreshAllData,
  resumeToast,
  selectResource,
  startEventStream,
  stopEventStream,
  stopWatchStream,
} from './store';
import { navigateTo, getPathFromTab } from './router';
import type { VirtualMachine, LxcContainer, VirtualMachineConsoleSession } from '../domain/models';

import LoginModal from './components/LoginModal.vue';
import SystemPoolModal from './components/SystemPoolModal.vue';
import ConfirmModal from './components/ConfirmModal.vue';
import VncConsoleStandalone from './components/VncConsoleStandalone.vue';
import TerminalStandalone from './components/TerminalStandalone.vue';
import VncConsole from './components/VncConsole.vue';
import MdiIcon from './components/MdiIcon.vue';
import SortableTh from './components/SortableTh.vue';
import { useTableSort } from './composables/useTableSort';
import ThemeSelector from './components/ThemeSelector.vue';
import FontSizeSelector from './components/FontSizeSelector.vue';
import PrerequisitesTopBarButton from './components/PrerequisitesTopBarButton.vue';
import PrerequisitesModal from './components/PrerequisitesModal.vue';
import SystemDetailsTopBarButton from './components/SystemDetailsTopBarButton.vue';
import SystemDetailsModal from './components/SystemDetailsModal.vue';
import SystemUpdateModal from './components/SystemUpdateModal.vue';
import HostTerminalModal from './components/HostTerminalModal.vue';
import DockerTerminalModal, { type TerminalContainerOption } from './components/DockerTerminalModal.vue';
import NanLogo from './components/NanLogo.vue';
import OverviewSkeleton from './components/OverviewSkeleton.vue';
import ResourceTree from './components/ResourceTree.vue';
import ActionRibbon from './components/ActionRibbon.vue';
import TaskConsoleDock from './components/TaskConsoleDock.vue';
import CommandPalette from './components/CommandPalette.vue';

import OverviewView from './views/OverviewView.vue';
import PrerequisitesView from './views/PrerequisitesView.vue';
import DisksView from './views/DisksView.vue';
import PoolsView from './views/PoolsView.vue';
import PoolDetailView from './views/PoolDetailView.vue';
import DatasetsView from './views/DatasetsView.vue';
import VolumesView from './views/VolumesView.vue';
import KvmView from './views/KvmView.vue';
import VmDetailView from './views/VmDetailView.vue';
import DockerView from './views/DockerView.vue';
import DockerDetailView from './views/DockerDetailView.vue';
import LxcView from './views/LxcView.vue';
import LxcDetailView from './views/LxcDetailView.vue';
import SharesView from './views/SharesView.vue';
import CronJobsView from './views/CronJobsView.vue';
import IdentityView from './views/IdentityView.vue';
import { useDevice } from './composables/useDevice';
import MobileShell from './mobile/MobileShell.vue';
import MobileTerminal from './mobile/components/MobileTerminal.vue';
import PwaInstallSheet from './components/PwaInstallSheet.vue';

const { isMobile, viewMode, setViewMode } = useDevice();

async function handleMobileVmConsole(vmName: string) {
  const popup = typeof window !== 'undefined' ? window.open('about:blank', '_blank') : null;
  try {
    const s = await services.kvm.openConsole(vmName);
    const tokenParam = s.token ? `&token=${encodeURIComponent(s.token)}` : '';
    const portParam = s.vncPort ? `&port=${s.vncPort}` : '';
    const url = `/vnc-console?vm=${encodeURIComponent(vmName)}${tokenParam}${portParam}`;
    if (popup && !popup.closed) {
      popup.location.href = url;
    } else if (typeof window !== 'undefined') {
      window.location.href = url;
    }
  } catch (err: any) {
    if (popup && !popup.closed) {
      popup.close();
    }
    notify('error', `Failed to open console: ${err.message}`);
  }
}

const showPoolModal = ref(false);
const showPrerequisitesModal = ref(false);
const showSystemDetailsModal = ref(false);
const showUpdateModal = ref(false);
const showHostTerminalModal = ref(false);
const showCommandPalette = ref(false);
const isMobileDrawerOpen = ref(false);
const isSidebarCollapsed = ref(
  typeof window !== 'undefined' && window.innerWidth > 768 && window.innerWidth < 1024
);

function handleViewportResize() {
  if (typeof window !== 'undefined' && window.innerWidth > 768 && window.innerWidth < 1024) {
    isSidebarCollapsed.value = true;
  }
}

function handleGlobalKeydown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault();
    showCommandPalette.value = !showCommandPalette.value;
  }
}

onMounted(() => {
  if (typeof window !== 'undefined') {
    window.addEventListener('keydown', handleGlobalKeydown);
    window.addEventListener('resize', handleViewportResize);
  }
});

onUnmounted(() => {
  if (typeof window !== 'undefined') {
    window.removeEventListener('keydown', handleGlobalKeydown);
    window.removeEventListener('resize', handleViewportResize);
  }
});

// Register top overlay dismissal for mobile / Android back button
if (typeof window !== 'undefined') {
  (window as any).nanDismissTopOverlay = () => {
    if (showCommandPalette.value) {
      showCommandPalette.value = false;
      return true;
    }
    if (activeLxcTerminal.value) {
      activeLxcTerminal.value = null;
      return true;
    }
    if (showHostTerminalModal.value) {
      showHostTerminalModal.value = false;
      return true;
    }
    if (showPoolModal.value) {
      showPoolModal.value = false;
      return true;
    }
    if (showPrerequisitesModal.value) {
      showPrerequisitesModal.value = false;
      return true;
    }
    if (showSystemDetailsModal.value) {
      showSystemDetailsModal.value = false;
      return true;
    }
    if (showUpdateModal.value || nasState.updateModalOpen) {
      showUpdateModal.value = false;
      nasState.updateModalOpen = false;
      return true;
    }
    if (isMobileDrawerOpen.value) {
      isMobileDrawerOpen.value = false;
      return true;
    }
    return false;
  };
}

// Active terminal for LXC
const activeLxcTerminal = ref<LxcContainer | null>(null);

function handleOpenLxcTerminal(ct: LxcContainer) {
  activeLxcTerminal.value = ct;
}

const lxcTerminalOptions = computed<TerminalContainerOption[]>(() => {
  if (!activeLxcTerminal.value) return [];
  const id = activeLxcTerminal.value.name.replace(/^lxcContainers\//, '');
  return [
    {
      id,
      name: activeLxcTerminal.value.containerName || id,
      state: activeLxcTerminal.value.state,
    },
  ];
});

// Active selected pool for PoolDetailView
const activeSelectedPool = computed<string | null>(() => {
  if (
    (nasState.activeTab === 'storage-pools' || nasState.activeTab === 'zfs') &&
    nasState.selectedResourceId &&
    nasState.selectedResourceId.startsWith('pool:')
  ) {
    return nasState.selectedResourceId.replace('pool:', '');
  }
  return null;
});

// Active selected VM for VmDetailView
const activeSelectedVm = computed<string | null>(() => {
  if (
    nasState.activeTab === 'kvm' &&
    nasState.selectedResourceId &&
    nasState.selectedResourceId.startsWith('vm:')
  ) {
    const raw = nasState.selectedResourceId.replace(/^vm:/, '');
    return raw.replace(/^virtualMachines\//, '').trim() || null;
  }
  return null;
});

// Active selected LXC container for LxcDetailView
const activeSelectedLxc = computed<string | null>(() => {
  if (
    nasState.activeTab === 'lxc' &&
    nasState.selectedResourceId &&
    nasState.selectedResourceId.startsWith('lxc:')
  ) {
    const raw = nasState.selectedResourceId.replace(/^lxc:/, '');
    const clean = raw.replace(/^lxcContainers\//, '').trim();
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

// Active selected Docker application for DockerDetailView
const activeSelectedDocker = computed<string | null>(() => {
  if (
    nasState.activeTab === 'docker' &&
    nasState.selectedResourceId &&
    nasState.selectedResourceId.startsWith('docker:')
  ) {
    const raw = nasState.selectedResourceId.replace(/^docker:/, '');
    return raw.replace(/^dockerApps\//, '').trim() || null;
  }
  return null;
});

// Integrated VNC Console Session for selected VM
const embeddedConsoleSession = ref<VirtualMachineConsoleSession | null>(null);
const isLoadingEmbeddedConsole = ref(false);
const vmWorkspaceMode = ref<'view' | 'console'>('view');

function popoutVmConsole(vmName: string) {
  const clean = vmName.replace(/^virtualMachines\//, '');
  const tokenParam = embeddedConsoleSession.value?.token ? `&token=${encodeURIComponent(embeddedConsoleSession.value.token)}` : '';
  const portParam = embeddedConsoleSession.value?.vncPort ? `&port=${embeddedConsoleSession.value.vncPort}` : '';
  const url = `/vnc-console?vm=${encodeURIComponent(clean)}${tokenParam}${portParam}`;
  window.open(
    url,
    `vnc_${clean}`,
    'width=1024,height=768,location=no,toolbar=no,menubar=no,status=no,resizable=yes'
  );
}

async function openEmbeddedVmConsole(vm: VirtualMachine) {
  const clean = vm.name.replace(/^virtualMachines\//, '');
  isLoadingEmbeddedConsole.value = true;
  try {
    const s = await services.kvm.openConsole(clean);
    embeddedConsoleSession.value = s;
    vmWorkspaceMode.value = 'console';
  } catch (err: any) {
    notify('error', `Failed to open console: ${err.message}`);
  } finally {
    isLoadingEmbeddedConsole.value = false;
  }
}

function closeEmbeddedConsole() {
  vmWorkspaceMode.value = 'view';
  embeddedConsoleSession.value = null;
}

function toggleMobileDrawer() {
  isMobileDrawerOpen.value = !isMobileDrawerOpen.value;
}

function closeMobileDrawer() {
  isMobileDrawerOpen.value = false;
}

function handleNav(id: NavTab) {
  navigateTo(id);
  closeMobileDrawer();
}

watch(
  () => nasState.authenticated,
  (isAuth) => {
    if (!isAuth) {
      showCommandPalette.value = false;
      showHostTerminalModal.value = false;
      showPoolModal.value = false;
      showPrerequisitesModal.value = false;
      showSystemDetailsModal.value = false;
      showUpdateModal.value = false;
      activeLxcTerminal.value = null;
      isMobileDrawerOpen.value = false;
    }
  }
);

// Module availability check
const activeTabModule = computed<ManagedModule | null>(() => {
  if (
    nasState.activeTab === 'zfs' ||
    nasState.activeTab === 'storage-pools' ||
    nasState.activeTab === 'storage-datasets' ||
    nasState.activeTab === 'storage-volumes'
  ) {
    return 'zfs';
  }
  if (
    nasState.activeTab === 'kvm' ||
    nasState.activeTab === 'docker' ||
    nasState.activeTab === 'lxc'
  ) {
    return nasState.activeTab;
  }
  if (nasState.activeTab === 'shares' || nasState.activeTab === 'shares-cifs') {
    return 'cifs';
  }
  return null;
});

const activeModuleStatus = computed(() => {
  if (!activeTabModule.value) return { enabled: true, missing: [] };
  return getModuleAvailability(activeTabModule.value);
});

const missingPrereqSort = useTableSort(
  () => activeModuleStatus.value.missing,
  {
    name: (m) => m.displayName,
    kind: (m) => m.kind,
    target: (m) => m.binaryOrPath,
  }
);

const hasFallbackPrereqs = computed(() =>
  nasState.prerequisites.some((p) => p.status === 'FALLBACK_ACTIVE' || p.fallbackMode)
);

function handleLogout() {
  stopEventStream();
  stopWatchStream();
  services.auth.logout();
  nasState.authenticated = false;
  nasState.currentUser = null;
  if (typeof (window as any).NanAndroidNative?.clearSession === 'function') {
    (window as any).NanAndroidNative.clearSession();
  }
  notify('info', 'Logged out of NAS session');
}

// Restore last active resource across reloads (only when loading root '/' path)
onMounted(() => {
  initSession();
  const currentPath = typeof window !== 'undefined' ? window.location.pathname.replace(/\/+$/, '') || '/' : '/';
  if (
    nasState.activeTab === 'vnc-console' ||
    nasState.activeTab === 'terminal' ||
    currentPath.startsWith('/vnc-console') ||
    currentPath.startsWith('/terminal')
  ) {
    return;
  }
  if (currentPath === '/') {
    const saved = nasState.selectedResourceId;
    if (saved) {
      if (saved.startsWith('vm:')) {
        navigateTo('kvm', true);
      } else if (saved.startsWith('lxc:')) {
        navigateTo('lxc', true);
      } else if (saved.startsWith('docker:')) {
        navigateTo('docker', true);
      } else if (saved.startsWith('pool:')) {
        navigateTo('storage-pools', true);
      } else if (saved === 'node:disks') {
        navigateTo('storage-disks', true);
      } else if (saved === 'node:shares' || saved.startsWith('shares:')) {
        navigateTo(saved === 'shares:webdav' ? 'shares-webdav' : 'shares-cifs', true);
      } else if (saved === 'node:identity') {
        navigateTo('identity', true);
      }
    }
  }
});
</script>

<template>
  <!-- Detached Standalone Console View (when ?vm=... is opened in its own window) -->
  <div v-if="nasState.activeTab === 'vnc-console'" class="detached-console-view">
    <VncConsoleStandalone />
  </div>

  <!-- Detached Standalone Terminal View (for LXC, Docker, or Host terminal) -->
  <div v-else-if="nasState.activeTab === 'terminal'" class="detached-terminal-view">
    <TerminalStandalone />
  </div>

  <!-- Dedicated Mobile Shell (Phone / Tablet / Viewport <= 768px) -->
  <MobileShell
    v-else-if="isMobile"
    @open-terminal="showHostTerminalModal = true"
    @open-system-details="showSystemDetailsModal = true"
    @open-prerequisites="showPrerequisitesModal = true"
    @open-pool-modal="showPoolModal = true"
    @open-update="showUpdateModal = true"
    @open-vm-console="handleMobileVmConsole"
    @open-lxc-terminal="handleOpenLxcTerminal"
    @logout="handleLogout"
  />

  <!-- Enterprise Fixed-Viewport Desktop Workbench (Proxmox-Style 100vh) -->
  <div v-else class="enterprise-workbench">
    <!-- Top Compact Appliance Bar -->
    <header class="top-header-bar">
      <!-- Left: Appliance Identity -->
      <div class="header-left">
        <button
          class="mobile-nav-toggle show-mobile-only"
          aria-label="Toggle navigation menu"
          @click="toggleMobileDrawer"
        >
          <MdiIcon :path="mdiMenu" size="20" />
        </button>

        <button
          class="btn btn-xs btn-icon btn-ghost sidebar-toggle-btn hide-mobile"
          :title="isSidebarCollapsed ? 'Show Sidebar' : 'Hide Sidebar'"
          aria-label="Toggle Navigation Sidebar"
          @click="isSidebarCollapsed = !isSidebarCollapsed"
        >
          <MdiIcon :path="mdiPageLayoutSidebarLeft" size="16" />
        </button>

        <NanLogo :size="26" subtitle="Enterprise Hypervisor & Storage" />

        <span class="appliance-node-chip hide-mobile">
          {{ nasState.systemTelemetry?.host?.hostname || 'nas01' }}
        </span>

        <!-- Main Pool Badge Indicator -->
        <div
          v-if="nasState.systemConfig?.mainPool"
          class="badge badge-main-pool hide-mobile"
          :title="`Designated Main Storage Pool: ${nasState.systemConfig.mainPool.replace(/^zfsPools\//, '')}`"
        >
          <MdiIcon :path="mdiDatabaseCheck" size="13" style="color: var(--accent-primary)" />
          <span>Main Pool: {{ nasState.systemConfig.mainPool.replace(/^zfsPools\//, '') }}</span>
        </div>
        <button
          v-else-if="nasState.initialLoaded && !nasState.systemConfig?.mainPool"
          class="badge badge-warning interactive hide-mobile"
          title="No main pool selected. Click to configure."
          @click="showPoolModal = true"
        >
          <MdiIcon :path="mdiAlertCircle" size="13" />
          <span>No Main Pool</span>
        </button>
      </div>

      <!-- Right: Quick Shell, Specs, Theme & User -->
      <div class="header-right">
        <!-- Spotlight Command Palette Search Trigger Button -->
        <button
          class="btn btn-xs btn-secondary hide-mobile command-palette-trigger"
          title="Search Resources and Actions (Ctrl+K / Cmd+K)"
          style="display: inline-flex; align-items: center; gap: 6px; padding: 2px 8px; font-weight: 500"
          @click="showCommandPalette = true"
        >
          <MdiIcon :path="mdiMagnify" size="13" style="color: var(--text-muted)" />
          <span style="font-size: 11.5px; color: var(--text-secondary)">Search</span>
          <kbd style="font-size: 9.5px; padding: 1px 4px; background: var(--bg-surface); border: 1px solid var(--border-subtle); border-radius: 3px; color: var(--text-muted)">Ctrl+K</kbd>
        </button>

        <!-- Return to Mobile Shell if user forced Desktop mode on a mobile device -->
        <button
          v-if="viewMode === 'desktop'"
          class="btn btn-xs btn-primary forced-desktop-pill"
          style="display: inline-flex; align-items: center; gap: 4px; font-weight: 600"
          title="Return to Mobile Shell"
          @click="setViewMode('auto')"
        >
          <MdiIcon :path="mdiCellphone" size="14" />
          <span>Mobile View</span>
        </button>

        <button
          class="btn btn-xs btn-icon btn-secondary"
          :class="{ 'hide-mobile': viewMode !== 'desktop' }"
          title="Host Shell (Open Root Host Terminal)"
          aria-label="Host Shell"
          @click="showHostTerminalModal = true"
        >
          <MdiIcon :path="mdiConsole" size="14" />
        </button>

        <button
          class="btn btn-xs"
          :class="[
            nasState.systemUpdate?.updateAvailable ? 'btn-warning' : 'btn-secondary',
            { 'hide-mobile': viewMode !== 'desktop' }
          ]"
          style="display: inline-flex; align-items: center; gap: 4px; font-weight: 500"
          :title="nasState.systemUpdate?.updateAvailable ? 'Software update is available! Click to inspect and apply.' : 'Check for Software Updates'"
          @click="showUpdateModal = true"
        >
          <MdiIcon :path="mdiUpdate" size="14" />
          <span>{{ nasState.systemUpdate?.updateAvailable ? `Update ${nasState.systemUpdate.latestVersion}` : 'Updates' }}</span>
        </button>

        <SystemDetailsTopBarButton :class="{ 'hide-mobile': viewMode !== 'desktop' }" @click="showSystemDetailsModal = true" />
        <PrerequisitesTopBarButton :class="{ 'hide-mobile': viewMode !== 'desktop' }" @click="showPrerequisitesModal = true" />
        <ThemeSelector :class="{ 'hide-mobile': viewMode !== 'desktop' }" />
        <FontSizeSelector :class="{ 'hide-mobile': viewMode !== 'desktop' }" />

        <button
          class="btn btn-xs btn-ghost"
          :disabled="nasState.loading"
          title="Refresh Telemetry & Data"
          @click="refreshAllData"
        >
          <MdiIcon :path="mdiRefresh" size="14" :class="{ 'spin-icon': nasState.loading }" />
        </button>

        <template v-if="nasState.authenticated && nasState.currentUser">
          <span
            class="user-chip font-xs mono"
            :class="{ 'hide-mobile': viewMode !== 'desktop' }"
            style="color: var(--text-secondary); padding: 0 4px"
          >
            {{ nasState.currentUser.username }}
          </span>
          <button
            class="btn btn-xs btn-ghost"
            :class="{ 'hide-mobile': viewMode !== 'desktop' }"
            title="Sign Out"
            @click="handleLogout"
          >
            <MdiIcon :path="mdiLogout" size="13" />
          </button>
        </template>
      </div>
    </header>

    <!-- Main Split Pane Workbench -->
    <div class="workbench-split">
      <!-- Left: Hierarchical Resource Tree (Desktop) -->
      <aside class="sidebar-tree hide-mobile" :class="{ 'is-collapsed': isSidebarCollapsed }">
        <ResourceTree />
      </aside>

      <!-- Mobile Drawer for Tree (on smaller mobile viewports) -->
      <div
        v-if="isMobileDrawerOpen"
        class="mobile-drawer-backdrop"
        @click="closeMobileDrawer"
      >
        <div class="mobile-drawer-pane" @click.stop>
          <div class="mobile-drawer-header">
            <span style="font-weight: 700; font-size: 13px">Resource Tree</span>
            <button class="btn btn-xs btn-ghost" @click="closeMobileDrawer">
              <MdiIcon :path="mdiClose" size="16" />
            </button>
          </div>
          <ResourceTree />
        </div>
      </div>

      <!-- Center: Contextual Master-Detail Workspace -->
      <div class="center-workspace">
        <!-- Unified Top Header & Tab Bar -->
        <ActionRibbon
          @open-host-terminal="showHostTerminalModal = true"
          @open-system-details="showSystemDetailsModal = true"
          @open-prerequisites="showPrerequisitesModal = true"
          @open-vm-console="openEmbeddedVmConsole"
          @open-lxc-terminal="handleOpenLxcTerminal"
        />

        <!-- Detail Viewport Area (Internal scrolling only) -->
        <main class="workspace-viewport">
          <!-- Initial Skeleton Loading -->
          <div
            v-if="!nasState.initialLoaded && !nasState.systemConfig"
            class="initial-skeleton-wrap"
          >
            <div class="sync-status-chip">
              <div class="spinner" style="width: 14px; height: 14px; border-width: 2px"></div>
              <span>Connecting to system services...</span>
            </div>
            <OverviewSkeleton />
          </div>

          <template v-else>
            <!-- No Main Pool Warning Banner -->
            <div
              v-if="nasState.initialLoaded && !nasState.systemConfig?.mainPool"
              class="system-pool-banner"
            >
              <div style="display: flex; align-items: center; gap: 12px">
                <MdiIcon :path="mdiAlertCircle" size="24" style="color: var(--warning-text); flex-shrink: 0" />
                <div>
                  <div style="font-weight: 700; font-size: 13px; color: var(--warning-text)">
                    No Main Pool Selected — Non-ZFS Modules Paused
                  </div>
                  <div style="font-size: 12px; color: var(--text-secondary); margin-top: 2px">
                    Select or create a ZFS pool to enable KVM, Docker, LXC, and Shares.
                  </div>
                </div>
              </div>
              <button class="btn btn-sm btn-primary" style="white-space: nowrap" @click="showPoolModal = true">
                Configure Main Pool
              </button>
            </div>

            <!-- Missing Prerequisites Alert -->
            <div
              v-if="nasState.initialLoaded && !activeModuleStatus.enabled"
              class="card"
              style="border-color: var(--danger-border)"
            >
              <div class="card-header">
                <div>
                  <h3 class="card-title" style="color: var(--danger-text)">
                    Module Disabled — Missing Prerequisites
                  </h3>
                  <p class="card-subtitle">
                    Required binaries or kernel modules were not found on the host.
                  </p>
                </div>
                <button class="btn btn-primary btn-sm" @click="showPrerequisitesModal = true">
                  Inspect Prerequisites & Fix
                </button>
              </div>
              <div class="table-wrap">
                <table class="data-table">
                  <thead>
                    <tr>
                      <SortableTh :sort="missingPrereqSort" sort-key="name">Missing Prerequisite</SortableTh>
                      <SortableTh :sort="missingPrereqSort" sort-key="kind">Kind</SortableTh>
                      <SortableTh :sort="missingPrereqSort" sort-key="target">Binary / Library Target</SortableTh>
                      <th>Installation Hint</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="m in missingPrereqSort.sorted.value" :key="m.name">
                      <td style="font-weight: 600">{{ m.displayName }}</td>
                      <td><span class="badge badge-danger">{{ m.kind }}</span></td>
                      <td class="mono">{{ m.binaryOrPath }}</td>
                      <td class="mono font-xs" style="color: var(--accent-primary)">
                        {{ m.installHint }}
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- Active View Render -->
            <template v-else>
              <!-- Integrated VM Console Tab if active -->
              <div
                v-if="nasState.activeTab === 'kvm' && vmWorkspaceMode === 'console' && embeddedConsoleSession"
                class="embedded-vnc-container"
              >
                <VncConsole
                  :key="`${embeddedConsoleSession.vmName}-${embeddedConsoleSession.token}`"
                  :vm-name="embeddedConsoleSession.vmName"
                  :token="embeddedConsoleSession.token"
                  :vnc-port="embeddedConsoleSession.vncPort"
                  :websocket-url="embeddedConsoleSession.websocketUrl"
                  :standalone="false"
                  @close="closeEmbeddedConsole"
                  @detach="popoutVmConsole(embeddedConsoleSession.vmName)"
                />
              </div>

              <!-- Standard Views -->
              <template v-else>
                <OverviewView v-if="nasState.activeTab === 'overview'" />
                <PrerequisitesView v-else-if="nasState.activeTab === 'prerequisites'" />
                <DisksView v-else-if="nasState.activeTab === 'storage-disks'" />
                <PoolDetailView
                  v-if="(nasState.activeTab === 'storage-pools' || nasState.activeTab === 'zfs') && activeSelectedPool"
                  :pool-name="activeSelectedPool"
                  :active-sub-tab="nasState.poolSubTab"
                  @update:active-sub-tab="nasState.poolSubTab = $event"
                  @back="selectResource('node:pools')"
                />
                <PoolsView v-else-if="nasState.activeTab === 'storage-pools' || nasState.activeTab === 'zfs'" />
                <DatasetsView v-else-if="nasState.activeTab === 'storage-datasets'" />
                <VolumesView v-else-if="nasState.activeTab === 'storage-volumes'" />
                <VmDetailView
                  v-if="nasState.activeTab === 'kvm' && activeSelectedVm"
                  :vm-name="activeSelectedVm"
                  :active-sub-tab="nasState.vmSubTab"
                  @update:active-sub-tab="nasState.vmSubTab = $event"
                  @back="selectResource('node:vms')"
                />
                <KvmView v-else-if="nasState.activeTab === 'kvm'" />
                <DockerDetailView
                  v-if="nasState.activeTab === 'docker' && activeSelectedDocker"
                  :app-name="activeSelectedDocker"
                  :active-sub-tab="nasState.dockerSubTab"
                  @update:active-sub-tab="nasState.dockerSubTab = $event"
                  @back="selectResource('node:docker')"
                />
                <DockerView v-else-if="nasState.activeTab === 'docker'" />
                <LxcDetailView
                  v-if="nasState.activeTab === 'lxc' && activeSelectedLxc"
                  :container-name="activeSelectedLxc"
                  :active-sub-tab="nasState.lxcSubTab"
                  @update:active-sub-tab="nasState.lxcSubTab = $event"
                  @back="selectResource('node:lxc')"
                />
                <LxcView v-else-if="nasState.activeTab === 'lxc'" />
                <SharesView
                  v-else-if="nasState.activeTab === 'shares' || nasState.activeTab === 'shares-cifs' || nasState.activeTab === 'shares-webdav'"
                  :active-subtab="nasState.activeTab === 'shares-webdav' ? 'webdav' : 'cifs'"
                />
                <CronJobsView v-else-if="nasState.activeTab === 'cron'" />
                <IdentityView v-else-if="nasState.activeTab === 'identity'" />
              </template>
            </template>
          </template>
        </main>
      </div>
    </div>

    <!-- Docked Bottom Task & Audit Console (Proxmox-Style) -->
    <TaskConsoleDock />
  </div>

  <!-- Global Quick Spotlight Command Palette (Ctrl+K) -->
  <CommandPalette
    v-model="showCommandPalette"
    @open-host-terminal="showHostTerminalModal = true"
    @open-system-details="showSystemDetailsModal = true"
    @open-prerequisites="showPrerequisitesModal = true"
  />

  <!-- Shared Global Modals & Utilities (Accessible across Desktop & Mobile) -->
  <LoginModal v-if="!nasState.authenticated" />
  <SystemPoolModal v-if="showPoolModal" @close="showPoolModal = false" />
  <PrerequisitesModal
    v-if="showPrerequisitesModal"
    @close="showPrerequisitesModal = false"
    @open-terminal="showPrerequisitesModal = false; showHostTerminalModal = true"
  />
  <SystemDetailsModal
    v-if="showSystemDetailsModal"
    @close="showSystemDetailsModal = false"
    @open-terminal="showSystemDetailsModal = false; showHostTerminalModal = true"
    @open-update="showSystemDetailsModal = false; showUpdateModal = true"
  />
  <SystemUpdateModal
    v-if="showUpdateModal || nasState.updateModalOpen"
    @close="
      showUpdateModal = false;
      nasState.updateModalOpen = false;
    "
  />

  <!-- Host Terminal: Dedicated touch-enabled full screen on mobile, desktop modal on desktop -->
  <template v-if="showHostTerminalModal">
    <MobileTerminal
      v-if="isMobile"
      @close="showHostTerminalModal = false"
    />
    <HostTerminalModal
      v-else
      @close="showHostTerminalModal = false"
    />
  </template>

  <!-- LXC Terminal: Dedicated touch-enabled full screen on mobile, desktop modal on desktop -->
  <template v-if="activeLxcTerminal">
    <MobileTerminal
      v-if="isMobile"
      :lxc-id="activeLxcTerminal.name.replace(/^lxcContainers\//, '')"
      @close="activeLxcTerminal = null"
    />
    <DockerTerminalModal
      v-else
      :app-title="`LXC Terminal: ${activeLxcTerminal.containerName || activeLxcTerminal.name.replace('lxcContainers/', '')}`"
      :containers="lxcTerminalOptions"
      :initial-container-id="activeLxcTerminal.name.replace('lxcContainers/', '')"
      endpoint-type="lxc"
      @close="activeLxcTerminal = null"
    />
  </template>
  <ConfirmModal />
  <PwaInstallSheet />

  <!-- Toast Notifications Stack -->
  <div
    class="toast-stack"
    :style="{
      bottom: isMobile ? '76px' : nasState.isTaskConsoleCollapsed ? '42px' : '187px',
    }"
  >
    <div
      v-for="t in nasState.toasts"
      :key="t.id"
      class="toast"
      :class="`toast-${t.type}`"
      role="status"
      aria-live="polite"
      @mouseenter="pauseToast(t.id)"
      @mouseleave="resumeToast(t.id)"
    >
      <span style="flex: 1">{{ t.message }}</span>
      <button
        type="button"
        class="btn btn-xs btn-ghost toast-close-btn"
        aria-label="Dismiss notification"
        style="padding: 2px; margin-left: 8px; opacity: 0.75; flex-shrink: 0"
        @click="dismissToast(t.id)"
      >
        <MdiIcon :path="mdiClose" size="14" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.enterprise-workbench {
  height: 100vh;
  width: 100vw;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background-color: var(--bg-base);
}

.top-header-bar {
  height: 40px;
  background: var(--header-bg);
  border-bottom: 1px solid var(--header-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  flex-shrink: 0;
  z-index: 50;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.appliance-node-chip {
  font-size: 12px;
  background: var(--bg-surface);
  color: var(--text-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 2px 8px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.header-sync-text {
  font-size: 11.5px;
  color: var(--text-muted);
}

.workbench-split {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: row;
  overflow: hidden;
}

.sidebar-tree {
  width: 260px;
  flex-shrink: 0;
  height: 100%;
  transition: width 0.15s ease;
}

.sidebar-tree.is-collapsed {
  display: none !important;
}

@media (max-width: 1024px) {
  .sidebar-tree {
    width: 210px;
  }
}

.center-workspace {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
  background-color: var(--bg-base);
}

.workspace-subtabs-bar {
  height: 36px;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  padding: 0 12px;
  gap: 4px;
  flex-shrink: 0;
}

.subtab-divider {
  width: 1px;
  height: 16px;
  background: var(--border-subtle);
  margin: 0 6px;
}

.subtab-btn {
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  padding: 4px 12px;
  font-size: 13px;
  font-weight: 500;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.12s ease;
  height: 100%;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.subtab-btn:hover {
  color: var(--text-primary);
  background: var(--accent-bg-subtle);
}

.subtab-btn.active {
  color: var(--accent-primary);
  border-bottom-color: var(--accent-primary);
  font-weight: 600;
}

.workspace-viewport {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
}

.system-pool-banner {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  background: var(--warning-bg);
  border: 1px solid var(--warning-border);
  border-left: 3px solid var(--warning);
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 16px;
}

.initial-skeleton-wrap {
  display: flex;
  flex-direction: column;
  gap: 14px;
  width: 100%;
}

.sync-status-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 20px;
  padding: 4px 12px;
  font-size: 12px;
  color: var(--text-secondary);
  align-self: flex-start;
}

.detached-console-view {
  width: 100vw;
  height: 100vh;
  margin: 0;
  padding: 0;
  overflow: hidden;
  background: var(--bg-base);
}

.embedded-vnc-container {
  width: 100%;
  height: 100%;
  flex: 1;
  min-height: max(560px, calc(100vh - 160px));
  display: flex;
  flex-direction: column;
}

.mobile-drawer-pane {
  position: fixed;
  top: 0;
  left: 0;
  bottom: 0;
  width: 280px;
  background: var(--sidebar-bg);
  z-index: 100;
  display: flex;
  flex-direction: column;
  box-shadow: var(--modal-shadow);
}

.mobile-drawer-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  border-bottom: 1px solid var(--border-subtle);
}

.font-xs {
  font-size: 11.5px;
}
</style>
