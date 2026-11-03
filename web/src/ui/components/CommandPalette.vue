<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
import {
  mdiClose,
  mdiConsole,
  mdiCubeOutline,
  mdiDatabase,
  mdiFolderNetwork,
  mdiInformationOutline,
  mdiLayers,
  mdiMagnify,
  mdiMonitor,
  mdiPlay,
  mdiPower,
  mdiRefresh,
  mdiRestart,
  mdiServer,
  mdiStop,
  mdiUpdate,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import { nasState, refreshAllData, selectResource } from '../store';
import { navigateTo } from '../router';

const props = defineProps<{
  modelValue: boolean;
}>();

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void;
  (e: 'open-host-terminal'): void;
  (e: 'open-system-details'): void;
  (e: 'open-prerequisites'): void;
}>();

const searchQuery = ref('');
const selectedIndex = ref(0);
const searchInput = ref<HTMLInputElement | null>(null);

interface CommandItem {
  id: string;
  category: 'Resources' | 'Navigation' | 'Actions' | 'System';
  title: string;
  subtitle: string;
  icon: string;
  badge?: string;
  action: () => void;
}

const allItems = computed<CommandItem[]>(() => {
  const items: CommandItem[] = [];

  // 1. VMs
  for (const vm of nasState.vms) {
    items.push({
      id: `vm:${vm.vmName}`,
      category: 'Resources',
      title: vm.vmName,
      subtitle: `Virtual Machine · ${vm.state || 'STOPPED'} · ${vm.vcpus} vCPUs · ${vm.memoryMib} MiB`,
      icon: mdiMonitor,
      badge: vm.state,
      action: () => {
        selectResource(`vm:${vm.vmName}`);
        nasState.activeTab = 'kvm';
        navigateTo('kvm');
        close();
      },
    });
  }

  // 2. LXC Containers
  for (const ct of nasState.lxcContainers) {
    const name = ct.containerName || ct.name.replace(/^lxcContainers\//, '');
    items.push({
      id: `lxc:${name}`,
      category: 'Resources',
      title: name,
      subtitle: `LXC Container · ${ct.state || 'STOPPED'} · IP: ${ct.ipv4Address || 'None'}`,
      icon: mdiCubeOutline,
      badge: ct.state,
      action: () => {
        selectResource(`lxc:${name}`);
        nasState.activeTab = 'lxc';
        navigateTo('lxc');
        close();
      },
    });
  }

  // 3. Docker Apps
  for (const app of nasState.dockerApps) {
    const title = app.title || app.appId || app.name.replace(/^dockerApps\//, '');
    items.push({
      id: `docker:${app.appId || title}`,
      category: 'Resources',
      title,
      subtitle: `Docker Application · ${app.state || 'STOPPED'} · ${app.containers?.length || 0} containers`,
      icon: mdiLayers,
      badge: app.state,
      action: () => {
        selectResource(`docker:${app.appId || title}`);
        nasState.activeTab = 'docker';
        navigateTo('docker');
        close();
      },
    });
  }

  // 4. Storage Pools
  for (const pool of nasState.pools) {
    const name = pool.poolName || pool.name.replace(/^zfsPools\//, '');
    items.push({
      id: `pool:${name}`,
      category: 'Resources',
      title: name,
      subtitle: `ZFS Pool · ${pool.state || 'ONLINE'} · Capacity: ${pool.capacityPercent ?? 0}%`,
      icon: mdiDatabase,
      badge: pool.state,
      action: () => {
        selectResource(`pool:${name}`);
        nasState.activeTab = 'storage-pools';
        navigateTo('storage-pools');
        close();
      },
    });
  }

  // 5. Navigation Sections
  items.push(
    {
      id: 'nav:overview',
      category: 'Navigation',
      title: 'Dashboard Overview',
      subtitle: 'System telemetry, KPI gauges, CPU/RAM/Network meters',
      icon: mdiServer,
      action: () => {
        nasState.activeTab = 'overview';
        selectResource('node:local');
        navigateTo('overview');
        close();
      },
    },
    {
      id: 'nav:vms',
      category: 'Navigation',
      title: 'Virtual Machines',
      subtitle: 'KVM hypervisor, noVNC consoles, and guest disks',
      icon: mdiMonitor,
      action: () => {
        nasState.activeTab = 'kvm';
        selectResource('node:vms');
        navigateTo('kvm');
        close();
      },
    },
    {
      id: 'nav:lxc',
      category: 'Navigation',
      title: 'LXC Containers',
      subtitle: 'System containers, OS templates, and mount points',
      icon: mdiCubeOutline,
      action: () => {
        nasState.activeTab = 'lxc';
        selectResource('node:lxc');
        navigateTo('lxc');
        close();
      },
    },
    {
      id: 'nav:docker',
      category: 'Navigation',
      title: 'Docker Applications',
      subtitle: 'Compose stacks, App Store catalog, container logs',
      icon: mdiLayers,
      action: () => {
        nasState.activeTab = 'docker';
        selectResource('node:docker');
        navigateTo('docker');
        close();
      },
    },
    {
      id: 'nav:storage-pools',
      category: 'Navigation',
      title: 'Storage Pools',
      subtitle: 'OpenZFS pool health, VDEV topologies, and scrub status',
      icon: mdiDatabase,
      action: () => {
        nasState.activeTab = 'storage-pools';
        selectResource('node:pools');
        navigateTo('storage-pools');
        close();
      },
    },
    {
      id: 'nav:storage-disks',
      category: 'Navigation',
      title: 'Physical Disks',
      subtitle: 'NVMe, SATA drives, S.M.A.R.T. health, and temperature',
      icon: mdiDatabase,
      action: () => {
        nasState.activeTab = 'storage-disks';
        selectResource('node:disks');
        navigateTo('storage-disks');
        close();
      },
    },
    {
      id: 'nav:shares',
      category: 'Navigation',
      title: 'Network File Shares',
      subtitle: 'Samba / CIFS shares and built-in WebDAV server',
      icon: mdiFolderNetwork,
      action: () => {
        nasState.activeTab = 'shares-cifs';
        selectResource('node:shares');
        navigateTo('shares-cifs');
        close();
      },
    }
  );

  // 6. Global System Actions
  items.push(
    {
      id: 'act:terminal',
      category: 'System',
      title: 'Open Host Web Terminal',
      subtitle: 'Launch detached interactive root bash terminal',
      icon: mdiConsole,
      action: () => {
        emit('open-host-terminal');
        close();
      },
    },
    {
      id: 'act:refresh',
      category: 'System',
      title: 'Refresh All System Telemetry',
      subtitle: 'Re-fetch pools, containers, VMs, and system metrics',
      icon: mdiRefresh,
      action: () => {
        refreshAllData();
        close();
      },
    },
    {
      id: 'act:prereq',
      category: 'System',
      title: 'System Prerequisites & Health Status',
      subtitle: 'Audit kernel modules, OpenZFS, Docker, and QEMU-KVM',
      icon: mdiInformationOutline,
      action: () => {
        emit('open-prerequisites');
        close();
      },
    }
  );

  return items;
});

const filteredItems = computed<CommandItem[]>(() => {
  const q = searchQuery.value.trim().toLowerCase();
  if (!q) return allItems.value;
  return allItems.value.filter(
    (item) =>
      item.title.toLowerCase().includes(q) ||
      item.subtitle.toLowerCase().includes(q) ||
      item.category.toLowerCase().includes(q)
  );
});

watch(filteredItems, () => {
  selectedIndex.value = 0;
});

function close() {
  emit('update:modelValue', false);
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'ArrowDown') {
    e.preventDefault();
    if (filteredItems.value.length > 0) {
      selectedIndex.value = (selectedIndex.value + 1) % filteredItems.value.length;
      scrollToSelected();
    }
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    if (filteredItems.value.length > 0) {
      selectedIndex.value =
        (selectedIndex.value - 1 + filteredItems.value.length) % filteredItems.value.length;
      scrollToSelected();
    }
  } else if (e.key === 'Enter') {
    e.preventDefault();
    const item = filteredItems.value[selectedIndex.value];
    if (item) {
      item.action();
    }
  } else if (e.key === 'Escape') {
    e.preventDefault();
    close();
  }
}

function scrollToSelected() {
  nextTick(() => {
    const el = document.querySelector(`.palette-item.active`);
    if (el) {
      el.scrollIntoView({ block: 'nearest' });
    }
  });
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      searchQuery.value = '';
      selectedIndex.value = 0;
      nextTick(() => {
        searchInput.value?.focus();
      });
    }
  }
);
</script>

<template>
  <div v-if="modelValue" class="palette-overlay" @click.self="close" @keydown="handleKeydown">
    <div class="palette-container">
      <!-- Search Input Bar -->
      <div class="palette-search-bar">
        <MdiIcon :path="mdiMagnify" size="20" style="color: var(--text-muted); flex-shrink: 0" />
        <input
          ref="searchInput"
          v-model="searchQuery"
          type="text"
          class="palette-input"
          placeholder="Search resources, apps, VMs, pools, or system actions..."
          spellcheck="false"
        />
        <kbd class="palette-kbd">ESC</kbd>
      </div>

      <!-- Results List -->
      <div class="palette-results">
        <div v-if="filteredItems.length === 0" class="palette-empty">
          <span>No resources or commands matching "{{ searchQuery }}"</span>
        </div>
        <div
          v-for="(item, idx) in filteredItems"
          :key="item.id"
          class="palette-item"
          :class="{ active: idx === selectedIndex }"
          @click="item.action"
          @mouseenter="selectedIndex = idx"
        >
          <div class="item-icon-box">
            <MdiIcon :path="item.icon" size="18" />
          </div>
          <div class="item-text">
            <div class="item-title-row">
              <span class="item-title">{{ item.title }}</span>
              <span v-if="item.badge" class="badge badge-sm" :class="{
                'badge-success': item.badge === 'RUNNING' || item.badge === 'ONLINE',
                'badge-warning': item.badge === 'PAUSED' || item.badge === 'PARTIALLY_RUNNING',
                'badge-secondary': item.badge === 'STOPPED'
              }">
                {{ item.badge }}
              </span>
            </div>
            <span class="item-subtitle">{{ item.subtitle }}</span>
          </div>
          <span class="item-category">{{ item.category }}</span>
        </div>
      </div>

      <!-- Footer Quick Tips -->
      <div class="palette-footer">
        <div class="footer-tip">
          <kbd>↑</kbd><kbd>↓</kbd> <span>Navigate</span>
        </div>
        <div class="footer-tip">
          <kbd>↵</kbd> <span>Select</span>
        </div>
        <div class="footer-tip">
          <kbd>ESC</kbd> <span>Close</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.palette-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.65);
  backdrop-filter: blur(4px);
  z-index: 9999;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding-top: 12vh;
}

.palette-container {
  width: 100%;
  max-width: 620px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  box-shadow: 0 16px 40px rgba(0, 0, 0, 0.45);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  animation: palette-slide-in 0.14s ease-out;
}

@keyframes palette-slide-in {
  from {
    opacity: 0;
    transform: translateY(-12px) scale(0.98);
  }
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

.palette-search-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 18px;
  border-bottom: 1px solid var(--border-subtle);
  background: var(--bg-surface);
}

.palette-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  font-size: 15px;
  color: var(--text-primary);
  font-family: inherit;
}

.palette-input::placeholder {
  color: var(--text-muted);
}

.palette-kbd {
  font-size: 10px;
  font-weight: 600;
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
}

.palette-results {
  max-height: 380px;
  overflow-y: auto;
  padding: 6px 0;
}

.palette-empty {
  padding: 28px;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
}

.palette-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 18px;
  cursor: pointer;
  transition: all 0.08s ease;
  border-left: 3px solid transparent;
}

.palette-item:hover,
.palette-item.active {
  background: var(--bg-hover);
  border-left-color: var(--accent-primary);
}

.item-icon-box {
  width: 32px;
  height: 32px;
  border-radius: 6px;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-secondary);
  flex-shrink: 0;
}

.palette-item.active .item-icon-box {
  color: var(--accent-primary);
  border-color: var(--accent-primary);
}

.item-text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.item-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.item-title {
  font-weight: 600;
  font-size: 13.5px;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.item-subtitle {
  font-size: 11.5px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.item-category {
  font-size: 10.5px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--text-muted);
  flex-shrink: 0;
}

.palette-footer {
  display: flex;
  align-items: center;
  gap: 18px;
  padding: 8px 18px;
  background: var(--bg-base);
  border-top: 1px solid var(--border-subtle);
  font-size: 11.5px;
  color: var(--text-muted);
}

.footer-tip {
  display: flex;
  align-items: center;
  gap: 4px;
}

.footer-tip kbd {
  font-size: 10px;
  padding: 1px 4px;
  border-radius: 3px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
}
</style>
