<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import {
  mdiLayers,
  mdiServer,
  mdiPlay,
  mdiStop,
  mdiPower,
  mdiRefresh,
  mdiMonitor,
  mdiMemory,
  mdiCpu64Bit,
  mdiConsole,
} from '@mdi/js';
import MdiIcon from '../../components/MdiIcon.vue';
import { nasState, notify, refreshAllData } from '../../store';
import { services } from '../../../container';
import type { VirtualMachine, LxcContainer } from '../../../domain/models';

const emit = defineEmits<{
  (e: 'open-vm-console', vmName: string): void;
  (e: 'open-lxc-terminal', c: LxcContainer): void;
}>();

const activeTab = ref<'vms' | 'lxc'>(nasState.activeTab === 'lxc' ? 'lxc' : 'vms');
const actionLoading = ref(false);

watch(
  () => nasState.activeTab,
  (tab) => {
    if (tab === 'lxc') activeTab.value = 'lxc';
    else if (tab === 'kvm') activeTab.value = 'vms';
  }
);

const vms = computed(() => nasState.vms || []);
const lxcContainers = computed(() => nasState.lxcContainers || []);

function isVmRunning(vm: VirtualMachine): boolean {
  return vm.state === 'RUNNING' || vm.state === 'running';
}

function isLxcRunning(c: LxcContainer): boolean {
  return c.state === 'RUNNING' || c.state === 'running';
}

async function handleStartVm(vm: VirtualMachine) {
  if (actionLoading.value) return;
  actionLoading.value = true;
  try {
    await services.kvm.startVirtualMachine(vm.vmName);
    notify('success', `Started VM ${vm.vmName}`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to start VM: ${err.message || err}`);
  } finally {
    actionLoading.value = false;
  }
}

async function handleStopVm(vm: VirtualMachine) {
  if (actionLoading.value) return;
  actionLoading.value = true;
  try {
    await services.kvm.stopVirtualMachine(vm.vmName);
    notify('success', `Stopped VM ${vm.vmName}`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to stop VM: ${err.message || err}`);
  } finally {
    actionLoading.value = false;
  }
}

async function handleRebootVm(vm: VirtualMachine) {
  if (actionLoading.value) return;
  actionLoading.value = true;
  try {
    await services.kvm.rebootVirtualMachine(vm.vmName);
    notify('success', `Rebooting VM ${vm.vmName}`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to reboot VM: ${err.message || err}`);
  } finally {
    actionLoading.value = false;
  }
}

async function handleStartLxc(c: LxcContainer) {
  if (actionLoading.value) return;
  actionLoading.value = true;
  try {
    await services.lxc.startContainer(c.name);
    notify('success', `Started container ${c.containerName || c.name}`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to start container: ${err.message || err}`);
  } finally {
    actionLoading.value = false;
  }
}

async function handleStopLxc(c: LxcContainer) {
  if (actionLoading.value) return;
  actionLoading.value = true;
  try {
    await services.lxc.stopContainer(c.name);
    notify('success', `Stopped container ${c.containerName || c.name}`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to stop container: ${err.message || err}`);
  } finally {
    actionLoading.value = false;
  }
}
</script>

<template>
  <div class="mobile-kvm-view">
    <!-- Sub-tab switch -->
    <div class="workload-tab-group">
      <button
        class="workload-tab"
        :class="{ active: activeTab === 'vms' }"
        @click="activeTab = 'vms'"
      >
        <MdiIcon :path="mdiLayers" size="16" style="margin-right: 4px" />
        Virtual Machines ({{ vms.length }})
      </button>
      <button
        class="workload-tab"
        :class="{ active: activeTab === 'lxc' }"
        @click="activeTab = 'lxc'"
      >
        <MdiIcon :path="mdiServer" size="16" style="margin-right: 4px" />
        LXC Containers ({{ lxcContainers.length }})
      </button>
    </div>

    <!-- KVM VMs List -->
    <div v-if="activeTab === 'vms'" class="items-list">
      <div v-for="vm in vms" :key="vm.vmName" class="item-card">
        <div class="item-card-top">
          <div class="item-identity">
            <span class="status-dot" :class="isVmRunning(vm) ? 'dot-running' : 'dot-stopped'" />
            <span class="item-name">{{ vm.vmName }}</span>
          </div>

          <span
            class="status-badge"
            :class="isVmRunning(vm) ? 'badge-running' : 'badge-stopped'"
          >
            {{ vm.state }}
          </span>
        </div>

        <div class="item-specs-row">
          <div class="spec-tag">
            <MdiIcon :path="mdiCpu64Bit" size="14" />
            <span>{{ vm.vcpus || 1 }} vCPU</span>
          </div>
          <div class="spec-tag">
            <MdiIcon :path="mdiMemory" size="14" />
            <span>{{ Math.round((vm.memoryMib || 1024) / 1024) }} GB RAM</span>
          </div>
          <span v-if="vm.osVariant" class="spec-text font-xs">{{ vm.osVariant }}</span>
        </div>

        <!-- Action Bar -->
        <div class="item-actions-row">
          <button
            v-if="!isVmRunning(vm)"
            class="btn btn-xs btn-success action-btn"
            :disabled="actionLoading"
            @click="handleStartVm(vm)"
          >
            <MdiIcon :path="mdiPlay" size="14" />
            <span>Start</span>
          </button>

          <template v-else>
            <button
              class="btn btn-xs btn-danger action-btn"
              :disabled="actionLoading"
              @click="handleStopVm(vm)"
            >
              <MdiIcon :path="mdiStop" size="14" />
              <span>Stop</span>
            </button>
            <button
              class="btn btn-xs btn-secondary action-btn"
              :disabled="actionLoading"
              @click="handleRebootVm(vm)"
            >
              <MdiIcon :path="mdiRefresh" size="14" />
              <span>Reboot</span>
            </button>
            <button
              class="btn btn-xs btn-primary action-btn"
              @click="emit('open-vm-console', vm.vmName)"
            >
              <MdiIcon :path="mdiMonitor" size="14" />
              <span>Console</span>
            </button>
          </template>
        </div>
      </div>

      <div v-if="vms.length === 0" class="empty-state">
        No KVM virtual machines configured.
      </div>
    </div>

    <!-- LXC Containers List -->
    <div v-else class="items-list">
      <div v-for="c in lxcContainers" :key="c.name" class="item-card">
        <div class="item-card-top">
          <div class="item-identity">
            <span class="status-dot" :class="isLxcRunning(c) ? 'dot-running' : 'dot-stopped'" />
            <span class="item-name">{{ c.containerName || c.name.replace('lxcContainers/', '') }}</span>
          </div>

          <span
            class="status-badge"
            :class="isLxcRunning(c) ? 'badge-running' : 'badge-stopped'"
          >
            {{ c.state }}
          </span>
        </div>

        <div class="item-specs-row">
          <div v-if="c.ipv4Address" class="spec-tag mono font-xs">
            {{ c.ipv4Address }}
          </div>
          <span v-if="c.memoryLimitMib" class="spec-tag font-xs">
            {{ Math.round(c.memoryLimitMib / 1024) }} GB
          </span>
        </div>

        <!-- Action Bar -->
        <div class="item-actions-row">
          <button
            v-if="!isLxcRunning(c)"
            class="btn btn-xs btn-success action-btn"
            :disabled="actionLoading"
            @click="handleStartLxc(c)"
          >
            <MdiIcon :path="mdiPlay" size="14" />
            <span>Start</span>
          </button>
          <template v-else>
            <button
              class="btn btn-xs btn-danger action-btn"
              :disabled="actionLoading"
              @click="handleStopLxc(c)"
            >
              <MdiIcon :path="mdiStop" size="14" />
              <span>Stop</span>
            </button>
            <button
              class="btn btn-xs btn-primary action-btn"
              @click="emit('open-lxc-terminal', c)"
            >
              <MdiIcon :path="mdiConsole" size="14" />
              <span>Terminal</span>
            </button>
          </template>
        </div>
      </div>

      <div v-if="lxcContainers.length === 0" class="empty-state">
        No LXC containers configured.
      </div>
    </div>
  </div>
</template>

<style scoped>
.mobile-kvm-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px 12px calc(40px + env(safe-area-inset-bottom, 0px)) 12px;
}

.workload-tab-group {
  display: flex;
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 12px;
  padding: 4px;
  gap: 4px;
}

.workload-tab {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 8px;
  border-radius: 8px;
  font-size: 12px;
  font-weight: 700;
  background: transparent;
  border: none;
  color: var(--text-secondary, #94a3b8);
  cursor: pointer;
  touch-action: manipulation;
}

.workload-tab.active {
  background: var(--accent-primary, #38bdf8);
  color: #0f172a;
}

.items-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.item-card {
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 14px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.item-card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.item-identity {
  display: flex;
  align-items: center;
  gap: 8px;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.dot-running {
  background: #4ade80;
  box-shadow: 0 0 8px #4ade80;
}

.dot-stopped {
  background: #64748b;
}

.item-name {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
}

.status-badge {
  font-size: 10px;
  font-weight: 700;
  padding: 2px 6px;
  border-radius: 4px;
}

.badge-running {
  background: rgba(74, 222, 128, 0.15);
  color: #4ade80;
}

.badge-stopped {
  background: rgba(148, 163, 184, 0.15);
  color: #94a3b8;
}

.item-specs-row {
  display: flex;
  align-items: center;
  gap: 12px;
  color: var(--text-secondary, #94a3b8);
}

.spec-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
}

.item-actions-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 4px;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.action-btn {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  height: 32px;
}

.empty-state {
  text-align: center;
  padding: 30px;
  color: var(--text-secondary, #94a3b8);
  font-size: 13px;
}
</style>
