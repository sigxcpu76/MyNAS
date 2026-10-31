<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import {
  mdiAlertCircle,
  mdiArrowLeft,
  mdiCamera,
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
  mdiMenu,
  mdiMenuDown,
  mdiMonitor,
  mdiOpenInNew,
  mdiPencil,
  mdiRefresh,
} from '@mdi/js';
import { services } from '../container';
import {
  getModuleAvailability,
  initSession,
  type ManagedModule,
  nasState,
  type NavTab,
  notify,
  refreshAllData,
  selectResource,
  startEventStream,
  stopEventStream,
} from './store';
import { navigateTo, getPathFromTab } from './router';
import type { VirtualMachine, LxcContainer, VirtualMachineConsoleSession } from '../domain/models';

import LoginModal from './components/LoginModal.vue';
import SystemPoolModal from './components/SystemPoolModal.vue';
import ConfirmModal from './components/ConfirmModal.vue';
import VncConsoleStandalone from './components/VncConsoleStandalone.vue';
import VncConsole from './components/VncConsole.vue';
import MdiIcon from './components/MdiIcon.vue';
import SortableTh from './components/SortableTh.vue';
import { useTableSort } from './composables/useTableSort';
import ThemeSelector from './components/ThemeSelector.vue';
import PrerequisitesTopBarButton from './components/PrerequisitesTopBarButton.vue';
import PrerequisitesModal from './components/PrerequisitesModal.vue';
import SystemDetailsTopBarButton from './components/SystemDetailsTopBarButton.vue';
import SystemDetailsModal from './components/SystemDetailsModal.vue';
import HostTerminalModal from './components/HostTerminalModal.vue';
import DockerTerminalModal, { type TerminalContainerOption } from './components/DockerTerminalModal.vue';
import NanLogo from './components/NanLogo.vue';
import OverviewSkeleton from './components/OverviewSkeleton.vue';
import ResourceTree from './components/ResourceTree.vue';
import ActionRibbon from './components/ActionRibbon.vue';
import TaskConsoleDock from './components/TaskConsoleDock.vue';

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
import IdentityView from './views/IdentityView.vue';

const showPoolModal = ref(false);
const showPrerequisitesModal = ref(false);
const showSystemDetailsModal = ref(false);
const showHostTerminalModal = ref(false);
const isMobileDrawerOpen = ref(false);

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
    return raw.replace(/^lxcContainers\//, '').trim() || null;
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

// Resource Detail View Active Subtabs
const vmActiveSubTab = ref<'summary' | 'hardware' | 'console' | 'snapshots'>('summary');
const lxcActiveSubTab = ref<'summary' | 'terminal' | 'mounts' | 'configuration' | 'logs'>('summary');
const dockerActiveSubTab = ref<'summary' | 'containers' | 'compose' | 'logs'>('summary');
const poolActiveSubTab = ref<'summary' | 'datasets' | 'volumes' | 'topology' | 'config'>('summary');

watch(activeSelectedVm, () => {
  vmActiveSubTab.value = 'summary';
});
watch(activeSelectedLxc, () => {
  lxcActiveSubTab.value = 'summary';
});
watch(activeSelectedDocker, () => {
  dockerActiveSubTab.value = 'summary';
});
watch(activeSelectedPool, () => {
  poolActiveSubTab.value = 'summary';
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
  () => nasState.activeTab,
  (tab: string) => {
    if (tab === 'prerequisites') {
      showPrerequisitesModal.value = true;
      navigateTo('overview', true);
    } else if (tab === 'terminal') {
      showHostTerminalModal.value = true;
      navigateTo('overview', true);
    }
  },
  { immediate: true }
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
  services.auth.logout();
  nasState.authenticated = false;
  nasState.currentUser = null;
  notify('info', 'Logged out of NAS session');
}

// Restore last active resource across reloads (only for default overview navigation)
onMounted(() => {
  initSession();
  const currentPath = typeof window !== 'undefined' ? window.location.pathname : '';
  if (
    nasState.activeTab === 'vnc-console' ||
    nasState.activeTab === 'terminal' ||
    currentPath.startsWith('/vnc-console') ||
    currentPath.startsWith('/terminal')
  ) {
    return;
  }
  const saved = nasState.selectedResourceId;
  if (saved) {
    if (saved.startsWith('vm:')) {
      navigateTo('kvm');
    } else if (saved.startsWith('lxc:')) {
      navigateTo('lxc');
    } else if (saved.startsWith('docker:')) {
      navigateTo('docker');
    } else if (saved.startsWith('pool:')) {
      navigateTo('storage-pools');
    } else if (saved === 'node:disks') {
      navigateTo('storage-disks');
    } else if (saved === 'node:shares' || saved.startsWith('shares:')) {
      navigateTo(saved === 'shares:webdav' ? 'shares-webdav' : 'shares-cifs');
    } else if (saved === 'node:identity') {
      navigateTo('identity');
    }
  }
});
</script>

<template>
  <!-- Detached Standalone Console View (when ?vm=... is opened in its own window) -->
  <div v-if="nasState.activeTab === 'vnc-console'" class="detached-console-view">
    <VncConsoleStandalone />
    <LoginModal v-if="!nasState.authenticated" />
    <ConfirmModal />
    <div class="toast-stack">
      <div
        v-for="t in nasState.toasts"
        :key="t.id"
        class="toast"
        :class="`toast-${t.type}`"
      >
        {{ t.message }}
      </div>
    </div>
  </div>

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

        <NanLogo :size="26" subtitle="Proxmox-Style Hypervisor & Storage" />

        <span class="appliance-node-chip hide-mobile mono">
          {{ nasState.systemTelemetry?.host?.hostname || 'nas01' }}
        </span>

        <!-- Main Pool Badge Indicator -->
        <button
          v-if="nasState.systemConfig?.mainPool"
          class="badge badge-main-pool interactive hide-mobile"
          title="Designated Main Storage Pool. Click to switch main pool."
          @click="showPoolModal = true"
        >
          <MdiIcon :path="mdiDatabaseCheck" size="13" style="color: var(--accent-primary)" />
          <span>MAIN POOL: {{ nasState.systemConfig.mainPool.replace(/^zfsPools\//, '') }}</span>
          <MdiIcon :path="mdiMenuDown" size="12" style="opacity: 0.75" />
        </button>
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
        <span v-if="hasFallbackPrereqs" class="badge badge-warning hide-mobile mono font-xs">
          FALLBACK
        </span>

        <span v-if="nasState.lastRefreshed" class="header-sync-text mono hide-mobile font-xs">
          Synced {{ nasState.lastRefreshed }}
        </span>

        <button
          class="btn btn-xs btn-secondary hide-mobile"
          title="Open Root Host Shell Terminal"
          @click="showHostTerminalModal = true"
        >
          <MdiIcon :path="mdiConsole" size="13" style="margin-right: 4px" />
          <span>Host Shell</span>
        </button>

        <SystemDetailsTopBarButton class="hide-mobile" @click="showSystemDetailsModal = true" />
        <PrerequisitesTopBarButton class="hide-mobile" @click="showPrerequisitesModal = true" />
        <ThemeSelector class="hide-mobile" />

        <button
          class="btn btn-xs btn-ghost"
          :disabled="nasState.loading"
          title="Refresh Telemetry & Data"
          @click="refreshAllData"
        >
          <MdiIcon :path="mdiRefresh" size="14" :class="{ 'spin-icon': nasState.loading }" />
        </button>

        <template v-if="nasState.authenticated && nasState.currentUser">
          <span class="badge badge-info hide-mobile mono font-xs">
            {{ nasState.currentUser.username }}
          </span>
          <button class="btn btn-xs btn-ghost hide-mobile" title="Sign Out" @click="handleLogout">
            <MdiIcon :path="mdiLogout" size="13" />
          </button>
        </template>
      </div>
    </header>

    <!-- Main Split Pane Workbench -->
    <div class="workbench-split">
      <!-- Left: Hierarchical Resource Tree (Desktop) -->
      <aside class="sidebar-tree hide-mobile">
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
        <!-- Top Action Ribbon -->
        <ActionRibbon
          @open-host-terminal="showHostTerminalModal = true"
          @open-system-details="showSystemDetailsModal = true"
          @open-prerequisites="showPrerequisitesModal = true"
          @open-vm-console="openEmbeddedVmConsole"
          @open-lxc-terminal="handleOpenLxcTerminal"
        />

        <!-- Contextual Subtabs Bar -->
        <div class="workspace-subtabs-bar">
          <!-- Overview Tab -->
          <template v-if="nasState.activeTab === 'overview'">
            <button class="subtab-btn active">Summary Dashboard</button>
          </template>

          <!-- Storage Subtabs -->
          <template
            v-else-if="
              nasState.activeTab === 'storage-pools' ||
              nasState.activeTab === 'storage-disks' ||
              nasState.activeTab === 'storage-datasets' ||
              nasState.activeTab === 'storage-volumes' ||
              nasState.activeTab === 'zfs'
            "
          >
            <template v-if="activeSelectedPool && (nasState.activeTab === 'storage-pools' || nasState.activeTab === 'zfs')">
              <button
                class="subtab-btn"
                title="Back to all storage pools"
                @click="selectResource('node:pools'); handleNav('storage-pools')"
              >
                <MdiIcon :path="mdiArrowLeft" size="14" />
                All Pools
              </button>
              <div class="subtab-divider"></div>
              <button
                class="subtab-btn"
                :class="{ active: poolActiveSubTab === 'summary' }"
                @click="poolActiveSubTab = 'summary'"
              >
                <MdiIcon :path="mdiInformationOutline" size="14" />
                Summary
              </button>
              <button
                class="subtab-btn"
                :class="{ active: poolActiveSubTab === 'datasets' }"
                @click="poolActiveSubTab = 'datasets'"
              >
                <MdiIcon :path="mdiFolderOutline" size="14" />
                Datasets
              </button>
              <button
                class="subtab-btn"
                :class="{ active: poolActiveSubTab === 'volumes' }"
                @click="poolActiveSubTab = 'volumes'"
              >
                <MdiIcon :path="mdiHarddisk" size="14" />
                Volumes
              </button>
              <button
                class="subtab-btn"
                :class="{ active: poolActiveSubTab === 'topology' }"
                @click="poolActiveSubTab = 'topology'"
              >
                <MdiIcon :path="mdiLayers" size="14" />
                Topology & Disks
              </button>
              <button
                class="subtab-btn"
                :class="{ active: poolActiveSubTab === 'config' }"
                @click="poolActiveSubTab = 'config'"
              >
                <MdiIcon :path="mdiCogOutline" size="14" />
                Configuration
              </button>
            </template>
            <template v-else>
              <button
                class="subtab-btn"
                :class="{ active: nasState.activeTab === 'storage-pools' || nasState.activeTab === 'zfs' }"
                @click="handleNav('storage-pools')"
              >
                Pools ({{ nasState.pools.length }})
              </button>
              <button
                class="subtab-btn"
                :class="{ active: nasState.activeTab === 'storage-disks' }"
                @click="handleNav('storage-disks')"
              >
                Disks ({{ nasState.disks.length }})
              </button>
              <button
                class="subtab-btn"
                :class="{ active: nasState.activeTab === 'storage-datasets' }"
                @click="handleNav('storage-datasets')"
              >
                Datasets
              </button>
              <button
                class="subtab-btn"
                :class="{ active: nasState.activeTab === 'storage-volumes' }"
                @click="handleNav('storage-volumes')"
              >
                Volumes
              </button>
            </template>
          </template>

          <!-- KVM Subtabs -->
          <template v-else-if="nasState.activeTab === 'kvm'">
            <template v-if="activeSelectedVm">
              <button
                class="subtab-btn"
                title="Back to all virtual machines"
                @click="selectResource('node:vms'); handleNav('kvm')"
              >
                <MdiIcon :path="mdiArrowLeft" size="14" />
                All VMs
              </button>
              <div class="subtab-divider"></div>
              <button
                class="subtab-btn"
                :class="{ active: vmActiveSubTab === 'summary' }"
                @click="vmActiveSubTab = 'summary'"
              >
                <MdiIcon :path="mdiInformationOutline" size="14" />
                Summary
              </button>
              <button
                class="subtab-btn"
                :class="{ active: vmActiveSubTab === 'hardware' }"
                @click="vmActiveSubTab = 'hardware'"
              >
                <MdiIcon :path="mdiHarddisk" size="14" />
                Hardware & Disks
              </button>
              <button
                class="subtab-btn"
                :class="{ active: vmActiveSubTab === 'console' }"
                @click="vmActiveSubTab = 'console'"
              >
                <MdiIcon :path="mdiMonitor" size="14" />
                Console
              </button>
              <button
                class="subtab-btn"
                :class="{ active: vmActiveSubTab === 'snapshots' }"
                @click="vmActiveSubTab = 'snapshots'"
              >
                <MdiIcon :path="mdiCamera" size="14" />
                Snapshots
              </button>
            </template>
            <template v-else>
              <button
                class="subtab-btn"
                :class="{ active: vmWorkspaceMode === 'view' }"
                @click="vmWorkspaceMode = 'view'"
              >
                Virtual Machines ({{ nasState.vms.length }})
              </button>
              <button
                v-if="embeddedConsoleSession"
                class="subtab-btn"
                :class="{ active: vmWorkspaceMode === 'console' }"
                @click="vmWorkspaceMode = 'console'"
              >
                <MdiIcon :path="mdiMonitor" size="14" />
                Console: {{ embeddedConsoleSession.vmName }}
              </button>
            </template>
          </template>

          <!-- LXC Subtabs -->
          <template v-else-if="nasState.activeTab === 'lxc'">
            <template v-if="activeSelectedLxc">
              <button
                class="subtab-btn"
                title="Back to all LXC containers"
                @click="selectResource('node:lxc'); handleNav('lxc')"
              >
                <MdiIcon :path="mdiArrowLeft" size="14" />
                All Containers
              </button>
              <div class="subtab-divider"></div>
              <button
                class="subtab-btn"
                :class="{ active: lxcActiveSubTab === 'summary' }"
                @click="lxcActiveSubTab = 'summary'"
              >
                <MdiIcon :path="mdiInformationOutline" size="14" />
                Summary
              </button>
              <button
                class="subtab-btn"
                :class="{ active: lxcActiveSubTab === 'terminal' }"
                @click="lxcActiveSubTab = 'terminal'"
              >
                <MdiIcon :path="mdiConsole" size="14" />
                Terminal
              </button>
              <button
                class="subtab-btn"
                :class="{ active: lxcActiveSubTab === 'mounts' }"
                @click="lxcActiveSubTab = 'mounts'"
              >
                <MdiIcon :path="mdiFolderOutline" size="14" />
                Storage Mounts
              </button>
              <button
                class="subtab-btn"
                :class="{ active: lxcActiveSubTab === 'configuration' }"
                @click="lxcActiveSubTab = 'configuration'"
              >
                <MdiIcon :path="mdiPencil" size="14" />
                Configuration
              </button>
              <button
                class="subtab-btn"
                :class="{ active: lxcActiveSubTab === 'logs' }"
                @click="lxcActiveSubTab = 'logs'"
              >
                <MdiIcon :path="mdiFileDocumentOutline" size="14" />
                Logs
              </button>
            </template>
            <template v-else>
              <button class="subtab-btn active">
                LXC Containers ({{ nasState.lxcContainers.length }})
              </button>
            </template>
          </template>

          <!-- Docker Apps Subtabs -->
          <template v-else-if="nasState.activeTab === 'docker'">
            <template v-if="activeSelectedDocker">
              <button
                class="subtab-btn"
                title="Back to all Docker applications"
                @click="selectResource('node:docker'); handleNav('docker')"
              >
                <MdiIcon :path="mdiArrowLeft" size="14" />
                All Apps
              </button>
              <div class="subtab-divider"></div>
              <button
                class="subtab-btn"
                :class="{ active: dockerActiveSubTab === 'summary' }"
                @click="dockerActiveSubTab = 'summary'"
              >
                <MdiIcon :path="mdiInformationOutline" size="14" />
                Summary
              </button>
              <button
                class="subtab-btn"
                :class="{ active: dockerActiveSubTab === 'containers' }"
                @click="dockerActiveSubTab = 'containers'"
              >
                <MdiIcon :path="mdiLayers" size="14" />
                Containers
              </button>
              <button
                class="subtab-btn"
                :class="{ active: dockerActiveSubTab === 'compose' }"
                @click="dockerActiveSubTab = 'compose'"
              >
                <MdiIcon :path="mdiFileDocumentOutline" size="14" />
                Compose YAML
              </button>
              <button
                class="subtab-btn"
                :class="{ active: dockerActiveSubTab === 'logs' }"
                @click="dockerActiveSubTab = 'logs'"
              >
                <MdiIcon :path="mdiDocker" size="14" />
                Logs
              </button>
            </template>
            <template v-else>
              <button class="subtab-btn active">
                Docker Applications ({{ nasState.dockerApps.length }})
              </button>
            </template>
          </template>

          <!-- Shares Subtabs -->
          <template
            v-else-if="
              nasState.activeTab === 'shares' ||
              nasState.activeTab === 'shares-cifs' ||
              nasState.activeTab === 'shares-webdav'
            "
          >
            <button
              class="subtab-btn"
              :class="{ active: nasState.activeTab !== 'shares-webdav' }"
              @click="handleNav('shares-cifs')"
            >
              CIFS (SMB) ({{ nasState.cifsShares.length }})
            </button>
            <button
              class="subtab-btn"
              :class="{ active: nasState.activeTab === 'shares-webdav' }"
              @click="handleNav('shares-webdav')"
            >
              WebDAV ({{ nasState.webdavShares.length }})
            </button>
          </template>

          <!-- Identity Subtabs -->
          <template v-else-if="nasState.activeTab === 'identity'">
            <button class="subtab-btn active">
              Users & Groups ({{ nasState.users.length }})
            </button>
          </template>

          <template v-else>
            <button class="subtab-btn active">View</button>
          </template>
        </div>

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
                  :active-sub-tab="poolActiveSubTab"
                  @update:active-sub-tab="poolActiveSubTab = $event"
                  @back="selectResource('node:pools')"
                />
                <PoolsView v-else-if="nasState.activeTab === 'storage-pools' || nasState.activeTab === 'zfs'" />
                <DatasetsView v-else-if="nasState.activeTab === 'storage-datasets'" />
                <VolumesView v-else-if="nasState.activeTab === 'storage-volumes'" />
                <VmDetailView
                  v-if="nasState.activeTab === 'kvm' && activeSelectedVm"
                  :vm-name="activeSelectedVm"
                  :active-sub-tab="vmActiveSubTab"
                  @update:active-sub-tab="vmActiveSubTab = $event"
                  @back="selectResource('node:vms')"
                />
                <KvmView v-else-if="nasState.activeTab === 'kvm'" />
                <DockerDetailView
                  v-if="nasState.activeTab === 'docker' && activeSelectedDocker"
                  :app-name="activeSelectedDocker"
                  :active-sub-tab="dockerActiveSubTab"
                  @update:active-sub-tab="dockerActiveSubTab = $event"
                  @back="selectResource('node:docker')"
                />
                <DockerView v-else-if="nasState.activeTab === 'docker'" />
                <LxcDetailView
                  v-if="nasState.activeTab === 'lxc' && activeSelectedLxc"
                  :container-name="activeSelectedLxc"
                  :active-sub-tab="lxcActiveSubTab"
                  @update:active-sub-tab="lxcActiveSubTab = $event"
                  @back="selectResource('node:lxc')"
                />
                <LxcView v-else-if="nasState.activeTab === 'lxc'" />
                <SharesView
                  v-else-if="nasState.activeTab === 'shares' || nasState.activeTab === 'shares-cifs' || nasState.activeTab === 'shares-webdav'"
                  :active-subtab="nasState.activeTab === 'shares-webdav' ? 'webdav' : 'cifs'"
                />
                <IdentityView v-else-if="nasState.activeTab === 'identity'" />
              </template>
            </template>
          </template>
        </main>
      </div>
    </div>

    <!-- Docked Bottom Task & Audit Console (Proxmox-Style) -->
    <TaskConsoleDock />

    <!-- Modals -->
    <LoginModal v-if="!nasState.authenticated" />
    <SystemPoolModal v-if="showPoolModal" @close="showPoolModal = false" />
    <PrerequisitesModal v-if="showPrerequisitesModal" @close="showPrerequisitesModal = false" />
    <SystemDetailsModal
      v-if="showSystemDetailsModal"
      @close="showSystemDetailsModal = false"
      @open-terminal="showSystemDetailsModal = false; showHostTerminalModal = true"
    />
    <HostTerminalModal
      v-if="showHostTerminalModal"
      @close="showHostTerminalModal = false"
    />
    <DockerTerminalModal
      v-if="activeLxcTerminal"
      :app-title="`LXC Terminal: ${activeLxcTerminal.containerName || activeLxcTerminal.name.replace('lxcContainers/', '')}`"
      :containers="lxcTerminalOptions"
      :initial-container-id="activeLxcTerminal.name.replace('lxcContainers/', '')"
      endpoint-type="lxc"
      @close="activeLxcTerminal = null"
    />
    <ConfirmModal />

    <!-- Toast Notifications Stack -->
    <div class="toast-stack">
      <div
        v-for="t in nasState.toasts"
        :key="t.id"
        class="toast"
        :class="`toast-${t.type}`"
      >
        {{ t.message }}
      </div>
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
  font-size: 11px;
  background: var(--bg-elevated);
  color: var(--text-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  padding: 2px 7px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.header-sync-text {
  font-size: 11px;
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
  height: 32px;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  padding: 0 12px;
  gap: 2px;
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
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 500;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.15s ease;
  height: 100%;
  display: inline-flex;
  align-items: center;
  gap: 5px;
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
}

.system-pool-banner {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  background: var(--warning-bg);
  border: 1px solid var(--warning-border);
  border-left: 4px solid var(--warning);
  border-radius: 6px;
  padding: 10px 14px;
  margin-bottom: 14px;
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
  min-height: 550px;
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
  font-size: 10px;
}
</style>
