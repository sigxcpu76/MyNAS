<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import {
  mdiAccountGroup,
  mdiCalendarClock,
  mdiCheckCircle,
  mdiChevronDown,
  mdiChevronRight,
  mdiCircle,
  mdiClose,
  mdiCubeOutline,
  mdiDatabase,
  mdiDisc,
  mdiFolderNetwork,
  mdiHarddisk,
  mdiLayers,
  mdiMagnify,
  mdiMonitor,
  mdiServer,
  mdiSnowflake,
  mdiWeb,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import {
  getModuleAvailability,
  nasState,
  type NavTab,
  selectResource,
} from '../store';
import { navigateTo } from '../router';

const searchQuery = ref('');
const searchInputRef = ref<HTMLInputElement | null>(null);

// Tree collapse states (start all collapsed by default)
const openSections = ref<Record<string, boolean>>({
  storage: false,
  vms: false,
  lxc: false,
  docker: false,
  shares: false,
  access: false,
});

function toggleSection(sec: string) {
  openSections.value[sec] = !openSections.value[sec];
}

const hostname = computed(() => {
  return nasState.systemTelemetry?.host?.hostname || 'nas01';
});

const defaultIp = computed(() => {
  const net = nasState.systemTelemetry?.network || [];
  const def = net.find((n) => n.isDefault);
  if (def && def.ipAddresses && def.ipAddresses.length > 0) {
    return def.ipAddresses[0].split('/')[0];
  }
  for (const n of net) {
    if (n.isUp && n.ipAddresses && n.ipAddresses.length > 0 && n.name !== 'lo') {
      return n.ipAddresses[0].split('/')[0];
    }
  }
  return '';
});

// Keydown handler for Ctrl+K
function handleKeyDown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault();
    searchInputRef.value?.focus();
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeyDown);
});

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeyDown);
});

// Selection handlers
function handleSelect(resourceId: string, tab: NavTab) {
  selectResource(resourceId);
  navigateTo(tab);
}

function handleHeaderSelect(sec: string, resourceId: string, tab: NavTab) {
  openSections.value[sec] = true;
  handleSelect(resourceId, tab);
}

// Filter matching helpers
const query = computed(() => searchQuery.value.trim().toLowerCase());

function matches(text: string): boolean {
  if (!query.value) return true;
  return text.toLowerCase().includes(query.value);
}

const filteredPools = computed(() => {
  if (!query.value) return nasState.pools;
  return nasState.pools.filter((p) => matches(p.name) || matches(p.poolName || ''));
});

const filteredVms = computed(() => {
  if (!query.value) return nasState.vms;
  return nasState.vms.filter((v) => matches(v.name) || matches(v.vmName));
});

const filteredLxc = computed(() => {
  if (!query.value) return nasState.lxcContainers;
  return nasState.lxcContainers.filter(
    (c) => matches(c.name) || matches(c.containerName || '')
  );
});

const filteredDocker = computed(() => {
  if (!query.value) return nasState.dockerApps;
  return nasState.dockerApps.filter((a) => {
    const clean = a.appId || a.name.replace(/^dockerApps\//, '');
    return matches(clean) || matches(a.title || '') || matches(a.name);
  });
});

function isSectionVisible(section: 'storage' | 'vms' | 'lxc' | 'docker' | 'shares' | 'cron' | 'access'): boolean {
  if (!query.value) return true;
  if (section === 'storage') {
    return matches('storage') || matches('disks') || matches('pools') || filteredPools.value.length > 0;
  }
  if (section === 'vms') {
    return matches('virtual machines') || matches('kvm') || matches('vms') || filteredVms.value.length > 0;
  }
  if (section === 'lxc') {
    return matches('lxc') || matches('containers') || filteredLxc.value.length > 0;
  }
  if (section === 'docker') {
    return matches('docker') || matches('apps') || filteredDocker.value.length > 0;
  }
  if (section === 'shares') {
    return matches('shares') || matches('smb') || matches('cifs') || matches('webdav');
  }
  if (section === 'cron') {
    return matches('cron') || matches('tasks') || matches('scheduled') || matches('jobs') || matches('backup');
  }
  if (section === 'access') {
    return matches('users') || matches('groups') || matches('identity') || matches('access');
  }
  return true;
}
</script>

<template>
  <div class="resource-tree-pane">
    <!-- Top Filter Box (Proxmox style) -->
    <div class="tree-search-bar">
      <MdiIcon :path="mdiMagnify" size="14" class="search-icon" />
      <input
        ref="searchInputRef"
        v-model="searchQuery"
        type="text"
        class="tree-search-input"
        placeholder="Filter (Ctrl+K)..."
        aria-label="Filter resource tree"
      />
      <button
        v-if="searchQuery"
        class="search-clear-btn"
        @click="searchQuery = ''"
      >
        <MdiIcon :path="mdiClose" size="13" />
      </button>
    </div>

    <!-- Tree Structure Container -->
    <div class="tree-content">
      <!-- Root Node: Server Appliance -->
      <div class="tree-node-item root-node">
        <div
          class="tree-row"
          :class="{ selected: nasState.selectedResourceId === 'node:local' && nasState.activeTab === 'overview' }"
          @click="handleSelect('node:local', 'overview')"
        >
          <MdiIcon :path="mdiServer" size="16" style="color: var(--accent-primary)" />
          <span class="tree-label node-name">
            {{ hostname }}
            <span v-if="defaultIp" class="tree-ip mono font-xs">({{ defaultIp }})</span>
          </span>
          <MdiIcon
            :path="mdiCircle"
            size="9"
            style="color: var(--success); margin-left: auto"
            title="Node Online"
          />
        </div>

        <!-- Node Children -->
        <div class="tree-branch">
          <!-- 1. Storage Folder -->
          <div v-show="isSectionVisible('storage')" class="tree-group">
            <div
              class="tree-row tree-header-row"
              :class="{ selected: nasState.selectedResourceId === 'node:pools' && nasState.activeTab === 'storage-pools' }"
              @click="handleHeaderSelect('storage', 'node:pools', 'storage-pools')"
            >
              <span
                class="chevron-btn"
                :title="openSections.storage || query ? 'Collapse' : 'Expand'"
                @click.stop="toggleSection('storage')"
              >
                <MdiIcon
                  :path="openSections.storage || query ? mdiChevronDown : mdiChevronRight"
                  size="14"
                  class="chevron-icon"
                />
              </span>
              <MdiIcon :path="mdiDatabase" size="15" style="color: var(--warning-text)" />
              <span class="tree-label">Storage</span>
              <span class="nav-badge font-xs">{{ nasState.pools.length }}</span>
            </div>

            <div v-show="openSections.storage || query" class="tree-sub-branch">
              <!-- Individual Storage Pools -->
              <div
                v-for="p in filteredPools"
                :key="p.name"
                class="tree-row tree-leaf"
                :class="{ selected: nasState.selectedResourceId === `pool:${p.poolName || p.name.replace(/^zfsPools\//, '')}` }"
                @click="nasState.selectedPoolName = p.name; handleSelect(`pool:${p.poolName || p.name.replace(/^zfsPools\//, '')}`, 'storage-pools')"
              >
                <MdiIcon
                  :path="mdiCircle"
                  size="9"
                  :style="{
                    color: p.state === 'ONLINE' ? 'var(--success)' : 'var(--danger)',
                    opacity: p.state === 'ONLINE' ? 1 : 0.85,
                  }"
                  :title="`Status: ${(p.state || 'ONLINE').toLowerCase()}`"
                />
                <span class="tree-label font-sm">{{ p.poolName || p.name.replace(/^zfsPools\//, '') }}</span>
              </div>

              <!-- Physical Disks -->
              <div
                v-if="matches('disks')"
                class="tree-row tree-leaf"
                :class="{ selected: nasState.selectedResourceId === 'node:disks' && nasState.activeTab === 'storage-disks' }"
                @click="handleSelect('node:disks', 'storage-disks')"
              >
                <MdiIcon :path="mdiHarddisk" size="13" style="color: var(--text-secondary)" />
                <span class="tree-label font-sm">Disks</span>
                <span class="nav-badge font-xs" style="margin-left: auto">
                  {{ nasState.disks.length }}
                </span>
              </div>

              <!-- ISOs & Images -->
              <div
                v-if="matches('iso') || matches('templates')"
                class="tree-row tree-leaf"
                :class="{ selected: nasState.selectedResourceId === 'node:isos' }"
                @click="handleSelect('node:isos', 'kvm')"
              >
                <MdiIcon :path="mdiDisc" size="13" style="color: var(--text-secondary)" />
                <span class="tree-label font-sm">ISOs & Templates</span>
              </div>
            </div>
          </div>

          <!-- 2. Virtual Machines (KVM) -->
          <div v-show="isSectionVisible('vms')" class="tree-group">
            <div
              class="tree-row tree-header-row"
              :class="{ selected: nasState.selectedResourceId === 'node:vms' && nasState.activeTab === 'kvm' }"
              @click="handleHeaderSelect('vms', 'node:vms', 'kvm')"
            >
              <span
                class="chevron-btn"
                :title="openSections.vms || query ? 'Collapse' : 'Expand'"
                @click.stop="toggleSection('vms')"
              >
                <MdiIcon
                  :path="openSections.vms || query ? mdiChevronDown : mdiChevronRight"
                  size="14"
                  class="chevron-icon"
                />
              </span>
              <MdiIcon :path="mdiMonitor" size="15" style="color: var(--accent-primary)" />
              <span class="tree-label">Virtual Machines</span>
              <span class="nav-badge font-xs">{{ nasState.vms.length }}</span>
            </div>

            <div v-show="openSections.vms || query" class="tree-sub-branch">
              <div
                v-if="filteredVms.length === 0"
                class="tree-empty-hint"
              >
                No VMs found
              </div>
              <div
                v-for="vm in filteredVms"
                :key="vm.name"
                class="tree-row tree-leaf"
                :class="{
                  selected:
                    nasState.selectedResourceId === `vm:${vm.name}` ||
                    nasState.selectedResourceId === `vm:${vm.vmName}` ||
                    nasState.selectedResourceId === `vm:${vm.name.replace(/^virtualMachines\//, '')}`
                }"
                @click="handleSelect(`vm:${vm.vmName || vm.name.replace(/^virtualMachines\//, '')}`, 'kvm')"
              >
                <!-- Power State Glyph -->
                <MdiIcon
                  :path="mdiCircle"
                  size="9"
                  :style="{
                    color: vm.state === 'RUNNING' ? 'var(--success)' : 'var(--text-muted)',
                    opacity: vm.state === 'RUNNING' ? 1 : 0.6,
                  }"
                  :title="`Status: ${vm.state === 'RUNNING' ? 'running' : 'stopped'}`"
                />
                <span class="tree-label font-sm" :title="vm.vmName">
                  {{ vm.vmName }}
                </span>
              </div>
            </div>
          </div>

          <!-- 3. LXC Containers -->
          <div v-show="isSectionVisible('lxc')" class="tree-group">
            <div
              class="tree-row tree-header-row"
              :class="{ selected: nasState.selectedResourceId === 'node:lxc' && nasState.activeTab === 'lxc' }"
              @click="handleHeaderSelect('lxc', 'node:lxc', 'lxc')"
            >
              <span
                class="chevron-btn"
                :title="openSections.lxc || query ? 'Collapse' : 'Expand'"
                @click.stop="toggleSection('lxc')"
              >
                <MdiIcon
                  :path="openSections.lxc || query ? mdiChevronDown : mdiChevronRight"
                  size="14"
                  class="chevron-icon"
                />
              </span>
              <MdiIcon :path="mdiCubeOutline" size="15" style="color: var(--accent-secondary)" />
              <span class="tree-label">LXC Containers</span>
              <span class="nav-badge font-xs">{{ nasState.lxcContainers.length }}</span>
            </div>

            <div v-show="openSections.lxc || query" class="tree-sub-branch">
              <div
                v-if="filteredLxc.length === 0"
                class="tree-empty-hint"
              >
                No LXC containers
              </div>
              <div
                v-for="c in filteredLxc"
                :key="c.name"
                class="tree-row tree-leaf"
                :class="{
                  selected:
                    nasState.selectedResourceId === `lxc:${c.name}` ||
                    nasState.selectedResourceId === `lxc:${c.containerName}` ||
                    nasState.selectedResourceId === `lxc:${c.name.replace(/^lxcContainers\//, '')}`
                }"
                @click="handleSelect(`lxc:${c.name.replace(/^lxcContainers\//, '')}`, 'lxc')"
              >
                <MdiIcon
                  v-if="c.state === 'FROZEN'"
                  :path="mdiSnowflake"
                  size="11"
                  style="color: var(--info)"
                />
                <MdiIcon
                  v-else
                  :path="mdiCircle"
                  size="9"
                  :style="{
                    color: c.state === 'RUNNING' ? 'var(--success)' : 'var(--text-muted)',
                    opacity: c.state === 'RUNNING' ? 1 : 0.6,
                  }"
                  :title="`Status: ${(c.state || 'stopped').toLowerCase()}`"
                />
                <span class="tree-label font-sm" :title="c.containerName || c.name">
                  {{ c.containerName || c.name.replace('lxcContainers/', '') }}
                </span>
              </div>
            </div>
          </div>

          <!-- 4. Docker Applications -->
          <div v-show="isSectionVisible('docker')" class="tree-group">
            <div
              class="tree-row tree-header-row"
              :class="{ selected: nasState.selectedResourceId === 'node:docker' && nasState.activeTab === 'docker' }"
              @click="handleHeaderSelect('docker', 'node:docker', 'docker')"
            >
              <span
                class="chevron-btn"
                :title="openSections.docker || query ? 'Collapse' : 'Expand'"
                @click.stop="toggleSection('docker')"
              >
                <MdiIcon
                  :path="openSections.docker || query ? mdiChevronDown : mdiChevronRight"
                  size="14"
                  class="chevron-icon"
                />
              </span>
              <MdiIcon :path="mdiLayers" size="15" style="color: var(--info-text)" />
              <span class="tree-label">Docker Apps</span>
              <span class="nav-badge font-xs">{{ nasState.dockerApps.length }}</span>
            </div>

            <div v-show="openSections.docker || query" class="tree-sub-branch">
              <div
                v-if="filteredDocker.length === 0"
                class="tree-empty-hint"
              >
                No Docker apps
              </div>
              <div
                v-for="a in filteredDocker"
                :key="a.name"
                class="tree-row tree-leaf"
                :class="{
                  selected:
                    nasState.selectedResourceId === `docker:${a.name}` ||
                    nasState.selectedResourceId === `docker:${a.appId}` ||
                    nasState.selectedResourceId === `docker:${a.name.replace(/^dockerApps\//, '')}`
                }"
                @click="handleSelect(`docker:${a.appId || a.name.replace(/^dockerApps\//, '')}`, 'docker')"
              >
                <MdiIcon
                  :path="mdiCircle"
                  size="9"
                  :style="{
                    color: a.state === 'RUNNING' ? 'var(--success)' : 'var(--text-muted)',
                    opacity: a.state === 'RUNNING' ? 1 : 0.6,
                  }"
                  :title="`Status: ${(a.state || 'stopped').toLowerCase()}`"
                />
                <span
                  class="tree-label font-sm"
                  :title="a.title || a.appId || a.name.replace(/^dockerApps\//, '')"
                >
                  {{ a.title || a.appId || a.name.replace(/^dockerApps\//, '') }}
                </span>
              </div>
            </div>
          </div>

          <!-- 5. Shares -->
          <div v-show="isSectionVisible('shares')" class="tree-group">
            <div
              class="tree-row tree-header-row"
              :class="{ selected: (nasState.selectedResourceId === 'shares:cifs' || nasState.selectedResourceId === 'shares:webdav') && (nasState.activeTab === 'shares-cifs' || nasState.activeTab === 'shares-webdav') }"
              @click="handleHeaderSelect('shares', 'shares:cifs', 'shares-cifs')"
            >
              <span
                class="chevron-btn"
                :title="openSections.shares || query ? 'Collapse' : 'Expand'"
                @click.stop="toggleSection('shares')"
              >
                <MdiIcon
                  :path="openSections.shares || query ? mdiChevronDown : mdiChevronRight"
                  size="14"
                  class="chevron-icon"
                />
              </span>
              <MdiIcon :path="mdiFolderNetwork" size="15" style="color: var(--accent-primary)" />
              <span class="tree-label">Shares</span>
              <span class="nav-badge font-xs">
                {{ nasState.cifsShares.length + nasState.webdavShares.length }}
              </span>
            </div>

            <div v-show="openSections.shares || query" class="tree-sub-branch">
              <div
                class="tree-row tree-leaf"
                :class="{ selected: nasState.selectedResourceId === 'shares:cifs' && nasState.activeTab === 'shares-cifs' }"
                @click="handleSelect('shares:cifs', 'shares-cifs')"
              >
                <MdiIcon :path="mdiFolderNetwork" size="13" style="color: var(--text-secondary)" />
                <span class="tree-label font-sm">CIFS (SMB)</span>
                <span class="nav-badge font-xs" style="margin-left: auto">{{ nasState.cifsShares.length }}</span>
              </div>
              <div
                class="tree-row tree-leaf"
                :class="{ selected: nasState.selectedResourceId === 'shares:webdav' && nasState.activeTab === 'shares-webdav' }"
                @click="handleSelect('shares:webdav', 'shares-webdav')"
              >
                <MdiIcon :path="mdiWeb" size="13" style="color: var(--text-secondary)" />
                <span class="tree-label font-sm">WebDAV</span>
                <span class="nav-badge font-xs" style="margin-left: auto">{{ nasState.webdavShares.length }}</span>
              </div>
            </div>
          </div>

          <!-- 6. Scheduled Tasks (Native CronJobs) -->
          <div v-show="isSectionVisible('cron')" class="tree-group">
            <div
              class="tree-row tree-leaf"
              :class="{ selected: nasState.selectedResourceId === 'node:cron' && nasState.activeTab === 'cron' }"
              @click="handleSelect('node:cron', 'cron')"
            >
              <MdiIcon :path="mdiCalendarClock" size="15" style="color: var(--accent-primary)" />
              <span class="tree-label font-sm">Scheduled Tasks</span>
              <span class="nav-badge font-xs" style="margin-left: auto">{{ nasState.cronJobs.length }}</span>
            </div>
          </div>

          <!-- 7. Access & Identity -->
          <div v-show="isSectionVisible('access')" class="tree-group">
            <div
              class="tree-row tree-leaf"
              :class="{ selected: nasState.selectedResourceId === 'node:identity' && nasState.activeTab === 'identity' }"
              @click="handleSelect('node:identity', 'identity')"
            >
              <MdiIcon :path="mdiAccountGroup" size="15" style="color: var(--text-secondary)" />
              <span class="tree-label font-sm">Users & Groups</span>
              <span class="nav-badge font-xs" style="margin-left: auto">{{ nasState.users.length }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.resource-tree-pane {
  width: 260px;
  min-width: 220px;
  background: var(--sidebar-bg);
  border-right: 1px solid var(--border-subtle);
  display: flex;
  flex-direction: column;
  height: 100%;
  flex-shrink: 0;
  user-select: none;
}

.tree-search-bar {
  padding: 8px 10px;
  border-bottom: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  position: relative;
  background: var(--bg-surface);
}

.search-icon {
  position: absolute;
  left: 18px;
  color: var(--text-muted);
  pointer-events: none;
}

.tree-search-input {
  width: 100%;
  background: var(--input-bg);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  padding: 4px 26px 4px 28px;
  font-size: 11.5px;
  color: var(--text-primary);
  outline: none;
  transition: border-color 0.15s ease;
}

.tree-search-input:focus {
  border-color: var(--accent-primary);
}

.search-clear-btn {
  position: absolute;
  right: 16px;
  background: transparent;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  display: flex;
  align-items: center;
  padding: 2px;
}

.tree-content {
  flex: 1;
  overflow-y: auto;
  padding: 6px 4px;
}

.tree-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 12.5px;
  color: var(--text-primary);
  transition: background-color 0.1s ease;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tree-row:hover {
  background: var(--bg-hover);
}

.tree-row.selected {
  background: var(--accent-bg-subtle);
  color: var(--accent-primary);
  font-weight: 600;
}

.tree-header-row {
  font-weight: 600;
  font-size: 12px;
  color: var(--text-secondary);
  padding: 5px 6px;
}

.chevron-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 3px;
  margin-right: 1px;
  flex-shrink: 0;
  cursor: pointer;
  color: var(--text-muted);
  transition: background-color 0.12s ease, color 0.12s ease;
}

.chevron-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.tree-row.selected .chevron-btn {
  color: var(--accent-primary);
}

.chevron-icon {
  color: inherit;
}

.tree-branch {
  padding-left: 10px;
  margin-top: 2px;
}

.tree-sub-branch {
  padding-left: 14px;
  margin-top: 1px;
  border-left: 1px solid var(--border-subtle);
  margin-left: 10px;
}

.tree-leaf {
  padding: 3px 8px;
}

.tree-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-name {
  font-weight: 700;
  font-size: 13px;
}

.tree-ip {
  color: var(--text-muted);
  margin-left: 4px;
}

.tree-empty-hint {
  padding: 4px 8px;
  font-size: 11px;
  color: var(--text-muted);
  font-style: italic;
}

.font-sm {
  font-size: 11.5px;
}

.font-xs {
  font-size: 10.5px;
}
</style>
