<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import {
  mdiAccountGroup,
  mdiAlertCircle,
  mdiCheckCircle,
  mdiChevronDown,
  mdiChevronRight,
  mdiCubeOutline,
  mdiDatabase,
  mdiDatabaseCheck,
  mdiFolderNetwork,
  mdiFolderOutline,
  mdiHarddisk,
  mdiLayers,
  mdiLogout,
  mdiMenuDown,
  mdiMonitor,
  mdiRefresh,
  mdiShieldCheck,
  mdiViewDashboard,
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
  startEventStream,
  stopEventStream,
} from './store';
import { navigateTo, getPathFromTab } from './router';
import { isDotDataset } from '../domain/models';
import LoginModal from './components/LoginModal.vue';
import SystemPoolModal from './components/SystemPoolModal.vue';
import ConfirmModal from './components/ConfirmModal.vue';
import VncConsoleStandalone from './components/VncConsoleStandalone.vue';
import MdiIcon from './components/MdiIcon.vue';
import ThemeSelector from './components/ThemeSelector.vue';
import PrerequisitesTopBarButton from './components/PrerequisitesTopBarButton.vue';
import PrerequisitesModal from './components/PrerequisitesModal.vue';
import NanLogo from './components/NanLogo.vue';
import OverviewSkeleton from './components/OverviewSkeleton.vue';
import OverviewView from './views/OverviewView.vue';
import PrerequisitesView from './views/PrerequisitesView.vue';
import DisksView from './views/DisksView.vue';
import PoolsView from './views/PoolsView.vue';
import DatasetsView from './views/DatasetsView.vue';
import VolumesView from './views/VolumesView.vue';
import KvmView from './views/KvmView.vue';
import DockerView from './views/DockerView.vue';
import LxcView from './views/LxcView.vue';
import SharesView from './views/SharesView.vue';
import IdentityView from './views/IdentityView.vue';

const storageExpanded = ref(true);
const showPoolModal = ref(false);
const showPrerequisitesModal = ref(false);

watch(
  () => nasState.activeTab,
  (tab: string) => {
    if (tab === 'prerequisites') {
      showPrerequisitesModal.value = true;
      navigateTo('overview', true);
    }
  },
  { immediate: true }
);

const totalDatasetsCount = computed(() => {
  let count = 0;
  for (const list of Object.values(nasState.datasetsByPool)) {
    count += list.filter((d) => !isDotDataset(d)).length;
  }
  return count;
});

const totalVolumesCount = computed(() => {
  let count = 0;
  for (const list of Object.values(nasState.volumesByPool)) {
    count += list.length;
  }
  return count;
});

const isStorageActive = computed(() =>
  nasState.activeTab === 'zfs' ||
  nasState.activeTab === 'storage-disks' ||
  nasState.activeTab === 'storage-pools' ||
  nasState.activeTab === 'storage-datasets' ||
  nasState.activeTab === 'storage-volumes'
);

function toggleStorageTree() {
  storageExpanded.value = !storageExpanded.value;
  if (!isStorageActive.value) {
    navigateTo('storage-disks');
  }
}

interface NavChildItem {
  id: NavTab;
  label: string;
  icon: string;
  path: string;
  badge?: string | number;
  disabled?: boolean;
}

interface NavItem {
  id: NavTab;
  label: string;
  icon: string;
  path: string;
  badge?: string | number;
  disabled?: boolean;
  children?: NavChildItem[];
}

const navItems = computed<NavItem[]>(() => {
  const zfs = getModuleAvailability('zfs');
  const kvm = getModuleAvailability('kvm');
  const docker = getModuleAvailability('docker');
  const lxc = getModuleAvailability('lxc');
  return [
    { id: 'overview', label: 'Overview Dashboard', icon: mdiViewDashboard, path: '/' },
    {
      id: 'storage-pools',
      label: 'Storage',
      icon: mdiDatabase,
      path: '/storage/pools',
      badge: zfs.enabled ? nasState.pools.length : 'OFF',
      disabled: !zfs.enabled,
      children: [
        {
          id: 'storage-disks',
          label: 'Disks',
          icon: mdiHarddisk,
          path: '/storage/disks',
          badge: nasState.disks.filter(
            (d) =>
              !d.isSystem &&
              !d.diskId.startsWith('zd') &&
              d.transport !== 'zvol' &&
              !d.partitions?.some((p) => p.isSystem || p.mountpoint === '/')
          ).length,
        },
        {
          id: 'storage-pools',
          label: 'Pools',
          icon: mdiDatabase,
          path: '/storage/pools',
          badge: nasState.pools.length,
        },
        {
          id: 'storage-datasets',
          label: 'Datasets',
          icon: mdiFolderOutline,
          path: '/storage/datasets',
          badge: totalDatasetsCount.value,
        },
        {
          id: 'storage-volumes',
          label: 'Volumes',
          icon: mdiHarddisk,
          path: '/storage/volumes',
          badge: totalVolumesCount.value,
        },
      ],
    },
    {
      id: 'kvm',
      label: 'KVM Hypervisor + noVNC',
      icon: mdiMonitor,
      badge: kvm.enabled ? nasState.vms.length : 'OFF',
      disabled: !kvm.enabled,
      path: getPathFromTab('kvm'),
    },
    {
      id: 'docker',
      label: 'Apps',
      icon: mdiLayers,
      badge: docker.enabled ? (nasState.dockerApps?.length ?? 0) : 'OFF',
      disabled: !docker.enabled,
      path: getPathFromTab('docker'),
    },
    {
      id: 'lxc',
      label: 'LXC Containers',
      icon: mdiCubeOutline,
      badge: lxc.enabled ? nasState.lxcContainers.length : 'OFF',
      disabled: !lxc.enabled,
      path: getPathFromTab('lxc'),
    },
    {
      id: 'shares',
      label: 'CIFS & WebDAV Shares',
      icon: mdiFolderNetwork,
      badge: nasState.cifsShares.length + nasState.webdavShares.length,
      path: getPathFromTab('shares'),
    },
    {
      id: 'identity',
      label: 'Users, Groups & SSH',
      icon: mdiAccountGroup,
      badge: nasState.users.length,
      path: getPathFromTab('identity'),
    },
  ];
});

const currentTabLabel = computed(() => {
  for (const item of navItems.value) {
    if (item.id === nasState.activeTab) return item.label;
    if (item.children) {
      const child = item.children.find((c) => c.id === nasState.activeTab);
      if (child) return `Storage · ${child.label}`;
    }
  }
  if (nasState.activeTab === 'zfs') return 'Storage · Pools';
  return 'Overview Dashboard';
});

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
  return null;
});

const activeModuleStatus = computed(() => {
  if (!activeTabModule.value) return { enabled: true, missing: [] };
  return getModuleAvailability(activeTabModule.value);
});

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

onMounted(() => {
  initSession();
});
</script>

<template>
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

  <div v-else class="app-shell">
    <!-- Sidebar Navigation -->
    <aside class="sidebar">
      <div class="brand-box">
        <NanLogo :size="38" subtitle="Not Another NAS" />
      </div>

      <nav class="nav-list">
        <template v-for="item in navItems" :key="item.id">
          <!-- Tree Branch with Children (e.g. Storage Section) -->
          <div v-if="item.children" class="nav-tree-group">
            <div
              class="nav-item nav-tree-header"
              :class="{ active: isStorageActive }"
              :style="item.disabled ? 'opacity: 0.65; pointer-events: none' : ''"
              @click="toggleStorageTree()"
            >
              <div style="display: flex; align-items: center; gap: 10px">
                <MdiIcon :path="item.icon" size="18" />
                <span>{{ item.label }}</span>
              </div>
              <div style="display: flex; align-items: center; gap: 6px">
                <span
                  v-if="item.badge !== undefined"
                  class="nav-badge"
                  :style="item.disabled ? 'background: var(--danger-bg); color: var(--danger-text)' : ''"
                >
                  {{ item.badge }}
                </span>
                <MdiIcon
                  :path="storageExpanded ? mdiChevronDown : mdiChevronRight"
                  size="16"
                  style="color: var(--text-secondary)"
                />
              </div>
            </div>

            <!-- Nested Sub-items (Disks, Pools, Datasets, Volumes) -->
            <div v-show="storageExpanded" class="nav-tree-children">
              <a
                v-for="child in item.children"
                :key="child.id"
                :href="child.path"
                class="nav-item nav-sub-item"
                :class="{ active: nasState.activeTab === child.id || (child.id === 'storage-pools' && nasState.activeTab === 'zfs') }"
                @click.prevent="navigateTo(child.id)"
              >
                <div style="display: flex; align-items: center; gap: 9px">
                  <MdiIcon :path="child.icon" size="16" />
                  <span>{{ child.label }}</span>
                </div>
                <span v-if="child.badge !== undefined" class="nav-badge" style="font-size: 10.5px">
                  {{ child.badge }}
                </span>
              </a>
            </div>
          </div>

          <!-- Standard Top-Level Nav Item -->
          <a
            v-else
            :href="item.path"
            class="nav-item"
            :class="{ active: nasState.activeTab === item.id }"
            :style="item.disabled ? 'opacity: 0.65; pointer-events: none' : ''"
            @click.prevent="!item.disabled && navigateTo(item.id)"
          >
            <div style="display: flex; align-items: center; gap: 10px">
              <MdiIcon :path="item.icon" size="18" />
              <span>{{ item.label }}</span>
            </div>
            <span
              v-if="item.badge !== undefined"
              class="nav-badge"
              :style="item.disabled ? 'background: var(--danger-bg); color: var(--danger-text)' : ''"
            >
              {{ item.badge }}
            </span>
          </a>
        </template>
      </nav>

      <div
        style="
          padding: 14px 16px;
          border-top: 1px solid var(--border-subtle);
          font-size: 11.5px;
          color: var(--text-secondary);
        "
      >
        <div style="color: var(--accent-primary); font-weight: 600">
          NaN System Core
        </div>
        <div>ZFS · KVM · Docker · LXC</div>
        <div>CIFS · WebDAV · Linux PAM</div>
      </div>
    </aside>

    <!-- Main Content Area -->
    <div class="main-area">
      <header class="top-header">
        <div style="display: flex; align-items: center; gap: 12px">
          <span style="font-weight: 600; font-size: 15px">
            {{ currentTabLabel }}
          </span>
          <span
            class="badge"
            :class="hasFallbackPrereqs ? 'badge-warning' : 'badge-success'"
          >
            {{ hasFallbackPrereqs ? 'Fallback Mode' : 'Host Environment' }}
          </span>
          <span v-if="nasState.lastRefreshed" class="card-subtitle mono">
            Synced {{ nasState.lastRefreshed }}
          </span>

          <button
            v-if="nasState.systemConfig?.mainPool"
            class="badge badge-main-pool interactive"
            title="Designated Main Storage Pool (Root for Apps & LXC, persistent config). Click to change main pool."
            @click="showPoolModal = true"
          >
            <MdiIcon :path="mdiDatabaseCheck" size="14" style="color: var(--accent-primary)" />
            <span>MAIN POOL: {{ nasState.systemConfig.mainPool }}</span>
            <MdiIcon :path="mdiMenuDown" size="13" style="opacity: 0.75" />
          </button>
          <button
            v-else-if="nasState.initialLoaded && !nasState.systemConfig?.mainPool"
            class="badge badge-warning interactive"
            style="cursor: pointer; display: inline-flex; align-items: center; gap: 5px; border: none; padding: 4px 10px; font-weight: 600"
            title="No main pool selected. Non-ZFS modules are paused. Click to select main pool."
            @click="showPoolModal = true"
          >
            <MdiIcon :path="mdiAlertCircle" size="14" />
            No Main Pool (Setup Required)
          </button>
          <span
            v-else-if="!nasState.initialLoaded"
            class="badge badge-neutral"
            style="display: inline-flex; align-items: center; gap: 6px; padding: 4px 10px; opacity: 0.8"
          >
            <span class="spinner" style="width: 10px; height: 10px; border-width: 2px"></span>
            Syncing System...
          </span>
        </div>

        <div style="display: flex; align-items: center; gap: 10px">
          <PrerequisitesTopBarButton @click="showPrerequisitesModal = true" />
          <ThemeSelector />

          <button
            class="btn btn-sm"
            :disabled="nasState.loading"
            @click="refreshAllData"
          >
            <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" />
            {{ nasState.loading ? 'Refreshing...' : 'Refresh Telemetry' }}
          </button>

          <template v-if="nasState.authenticated && nasState.currentUser">
            <span class="badge badge-info">
              {{ nasState.currentUser.username }} (UID {{ nasState.currentUser.uid }})
            </span>
            <button class="btn btn-sm" @click="handleLogout">
              <MdiIcon :path="mdiLogout" size="14" style="margin-right: 4px" />
              Sign Out
            </button>
          </template>
        </div>
      </header>

      <main class="content-container">
        <!-- Initial Loading State: Moving Skeleton Placeholders -->
        <div
          v-if="!nasState.initialLoaded && !nasState.systemConfig"
          class="initial-skeleton-wrap"
        >
          <div class="sync-status-chip">
            <div class="spinner" style="width: 14px; height: 14px; border-width: 2px"></div>
            <span>Connecting to NaN (Not Another NAS) · Synchronizing topology...</span>
          </div>
          <OverviewSkeleton />
        </div>

        <template v-else>
          <!-- Main Storage Pool Setup & Recovery Alert Banner -->
          <div
            v-if="nasState.initialLoaded && !nasState.systemConfig?.mainPool"
            class="system-pool-banner"
          >
            <div style="display: flex; align-items: center; gap: 12px">
              <MdiIcon :path="mdiAlertCircle" size="26" style="color: var(--warning-text); flex-shrink: 0" />
              <div>
                <div style="font-weight: 700; font-size: 14px; color: var(--warning-text)">
                  Main ZFS Storage Pool Unconfigured — Non-ZFS Modules Paused
                </div>
                <div style="font-size: 12.5px; color: var(--text-secondary); margin-top: 2px">
                  Until a main pool is selected, modules other than ZFS (KVM, Docker, LXC, CIFS, WebDAV) are paused. Select or import a pool to initialize <code>.nas-system</code> datasets and recover your configuration.
                </div>
              </div>
            </div>
            <button class="btn btn-sm btn-primary" style="white-space: nowrap" @click="showPoolModal = true">
              Configure Main Pool
            </button>
          </div>

          <div
            v-if="nasState.initialLoaded && !activeModuleStatus.enabled"
            class="card"
            style="border-color: var(--danger-border)"
          >
            <div class="card-header">
              <div>
                <h3 class="card-title" style="color: var(--danger-text)">
                  Module Disabled — Required Host Binaries / Libraries Missing
                </h3>
                <p class="card-subtitle">
                  This module is automatically disabled because one or more underlying Linux binaries,
                  shared libraries, or kernel modules were not detected on the host.
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
                    <th>Missing Prerequisite</th>
                    <th>Kind</th>
                    <th>Binary / Library Target</th>
                    <th>Installation Hint</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="m in activeModuleStatus.missing" :key="m.name">
                    <td style="font-weight: 600">{{ m.displayName }}</td>
                    <td><span class="badge badge-danger">{{ m.kind }}</span></td>
                    <td class="mono">{{ m.binaryOrPath }}</td>
                    <td class="mono" style="font-size: 11.5px; color: var(--accent-primary)">
                      {{ m.installHint }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <template v-else>
            <OverviewView v-if="nasState.activeTab === 'overview'" />
            <PrerequisitesView v-else-if="nasState.activeTab === 'prerequisites'" />
            <DisksView v-else-if="nasState.activeTab === 'storage-disks'" />
            <PoolsView v-else-if="nasState.activeTab === 'storage-pools' || nasState.activeTab === 'zfs'" />
            <DatasetsView v-else-if="nasState.activeTab === 'storage-datasets'" />
            <VolumesView v-else-if="nasState.activeTab === 'storage-volumes'" />
            <KvmView v-else-if="nasState.activeTab === 'kvm'" />
            <DockerView v-else-if="nasState.activeTab === 'docker'" />
            <LxcView v-else-if="nasState.activeTab === 'lxc'" />
            <SharesView v-else-if="nasState.activeTab === 'shares'" />
            <IdentityView v-else-if="nasState.activeTab === 'identity'" />
          </template>
        </template>
      </main>
    </div>

    <!-- Login Modal if not authenticated -->
    <LoginModal v-if="!nasState.authenticated" />

    <!-- System Storage Pool Configuration Modal -->
    <SystemPoolModal v-if="showPoolModal" @close="showPoolModal = false" />

    <!-- Host Prerequisites & Fix Instructions Modal -->
    <PrerequisitesModal
      v-if="showPrerequisitesModal"
      @close="showPrerequisitesModal = false"
    />

    <!-- Confirmation / Question / Alert Modal -->
    <ConfirmModal />

    <!-- Toast Notifications -->
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
.nav-tree-group {
  display: flex;
  flex-direction: column;
}

.nav-tree-header {
  cursor: pointer;
  user-select: none;
}

.nav-tree-children {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding-left: 8px;
  margin-top: 2px;
  margin-bottom: 4px;
  border-left: 2px solid var(--border-subtle);
  margin-left: 18px;
}

.nav-sub-item {
  padding: 7px 10px;
  font-size: 13px;
}

.system-pool-banner {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  background: var(--warning-bg);
  border: 1px solid var(--warning-border);
  border-left: 4px solid var(--warning);
  border-radius: 8px;
  padding: 14px 18px;
  margin-bottom: 18px;
}

.initial-skeleton-wrap {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
}

.sync-status-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 20px;
  padding: 6px 14px;
  font-size: 12.5px;
  color: var(--text-secondary);
  align-self: flex-start;
  box-shadow: var(--card-shadow);
}

.detached-console-view {
  width: 100vw;
  height: 100vh;
  margin: 0;
  padding: 0;
  overflow: hidden;
  background: var(--bg-primary, #0f172a);
}
</style>

