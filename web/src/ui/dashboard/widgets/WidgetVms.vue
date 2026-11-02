<script setup lang="ts">
import { computed } from 'vue';
import { mdiServerNetwork, mdiChevronRight } from '@mdi/js';
import { nasState, selectResource } from '../../store';
import { navigateTo } from '../../router';
import SkeletonBox from '../../components/SkeletonBox.vue';
import type { VirtualMachine } from '../../../domain/models';

const vms = computed(() => nasState.vms || []);
const runningCount = computed(() => vms.value.filter((v) => v.state === 'RUNNING').length);

function openVm(vm: VirtualMachine) {
  const clean = vm.name.replace(/^virtualMachines\//, '');
  selectResource(`vm:${clean}`);
  navigateTo('kvm');
}
</script>

<template>
  <div class="card workload-card">
    <div class="card-header workload-header">
      <div class="header-left">
        <div class="header-icon-box">
          <svg viewBox="0 0 24 24" class="header-icon"><path :d="mdiServerNetwork" /></svg>
        </div>
        <h3 class="card-title">Virtual Machines</h3>
        <span class="badge badge-neutral mono count-badge">
          {{ runningCount }}/{{ vms.length }}
        </span>
      </div>
      <button class="header-action-btn" @click="navigateTo('kvm')" title="Manage Virtual Machines">
        <span>Manage</span>
        <svg viewBox="0 0 24 24" class="chevron-svg"><path :d="mdiChevronRight" /></svg>
      </button>
    </div>

    <!-- Loading Skeleton -->
    <div v-if="nasState.loading && vms.length === 0" class="workload-list">
      <div v-for="s in 3" :key="s" class="workload-item skeleton-item">
        <div class="item-meta">
          <SkeletonBox width="110px" height="14px" />
          <SkeletonBox width="150px" height="11px" />
        </div>
        <SkeletonBox width="60px" height="20px" rounded="4px" />
      </div>
    </div>

    <!-- Empty State -->
    <div v-else-if="vms.length === 0" class="empty-state">
      <span>No virtual machines created yet.</span>
      <button class="btn btn-xs btn-secondary" @click="navigateTo('kvm')">Create VM</button>
    </div>

    <!-- VM Items -->
    <div v-else class="workload-list">
      <div
        v-for="vm in vms"
        :key="vm.name"
        class="workload-item"
        @click="openVm(vm)"
        role="button"
        tabindex="0"
      >
        <div class="item-meta">
          <div class="item-title" :title="vm.vmName">
            {{ vm.vmName }}
          </div>
          <div class="item-subtitle mono" :title="`${vm.vcpus} vCPU · ${vm.memoryMib} MiB RAM · VNC :${vm.vncPort}`">
            {{ vm.vcpus }} vCPU · {{ vm.memoryMib }} MiB · VNC :{{ vm.vncPort }}
          </div>
        </div>

        <span
          class="status-chip"
          :class="vm.state === 'RUNNING' ? 'chip-running' : vm.state === 'PAUSED' ? 'chip-warning' : 'chip-stopped'"
        >
          <span class="status-dot"></span>
          {{ vm.state === 'RUNNING' ? 'Running' : vm.state === 'PAUSED' ? 'Paused' : 'Stopped' }}
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.workload-card {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.workload-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding-bottom: 8px;
  margin-bottom: 10px;
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

.workload-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  overflow-y: auto;
  max-height: 260px;
  padding-right: 2px;
}

.workload-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 6px 8px;
  border-radius: 6px;
  background: var(--bg-surface-elevated, rgba(255, 255, 255, 0.02));
  border: 1px solid var(--border-subtle);
  cursor: pointer;
  transition: all 0.15s ease;
}

.workload-item:hover {
  background: var(--bg-hover, rgba(255, 255, 255, 0.05));
  border-color: var(--border-strong);
}

.workload-item.skeleton-item {
  cursor: default;
}

.item-meta {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.item-title {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.item-subtitle {
  font-size: 11px;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Status Chips */
.status-chip {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 2px 7px;
  border-radius: 4px;
  font-size: 10.5px;
  font-weight: 600;
  line-height: 1.3;
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}

.chip-running {
  background: rgba(16, 185, 129, 0.12);
  color: #10b981;
  border: 1px solid rgba(16, 185, 129, 0.25);
}
.chip-running .status-dot {
  background: #10b981;
  box-shadow: 0 0 6px rgba(16, 185, 129, 0.6);
}

.chip-stopped {
  background: transparent;
  color: var(--text-muted);
  border: none;
  font-weight: 500;
  padding: 2px 4px;
}
.chip-stopped .status-dot {
  background: var(--text-muted);
  opacity: 0.4;
}

.chip-warning {
  background: rgba(245, 158, 11, 0.12);
  color: #f59e0b;
  border: 1px solid rgba(245, 158, 11, 0.25);
}
.chip-warning .status-dot {
  background: #f59e0b;
}

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
</style>