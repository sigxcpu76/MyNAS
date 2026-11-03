<script setup lang="ts">
import { computed } from 'vue';
import { mdiDocker, mdiChevronRight } from '@mdi/js';
import { nasState, selectResource } from '../../store';
import { navigateTo } from '../../router';
import SkeletonBox from '../../components/SkeletonBox.vue';
import type { DockerApp, LxcContainer } from '../../../domain/models';

const dockerApps = computed(() => nasState.dockerApps || []);
const lxcContainers = computed(() => nasState.lxcContainers || []);

function openApp(app: DockerApp) {
  const clean = app.name.replace(/^dockerApps\//, '');
  navigateTo('docker', false, `docker:${clean}`);
}

function openLxc(lx: LxcContainer) {
  const clean = lx.name.replace(/^lxcContainers\//, '');
  navigateTo('lxc', false, `lxc:${clean}`);
}

const totalCount = computed(() => dockerApps.value.length + lxcContainers.value.length);
const runningCount = computed(() => {
  const dRun = dockerApps.value.filter((a) => a.state === 'RUNNING').length;
  const lRun = lxcContainers.value.filter((l) => l.state === 'RUNNING').length;
  return dRun + lRun;
});

function getAppStateClass(state?: string): string {
  switch (state) {
    case 'RUNNING':
      return 'chip-running';
    case 'DEPLOYING':
      return 'chip-info';
    case 'PARTIALLY_RUNNING':
      return 'chip-warning';
    case 'ERROR':
      return 'chip-danger';
    default:
      return 'chip-stopped';
  }
}

function getAppStateLabel(state?: string): string {
  switch (state) {
    case 'RUNNING':
      return 'Running';
    case 'DEPLOYING':
      return 'Deploying';
    case 'PARTIALLY_RUNNING':
      return 'Partial';
    case 'ERROR':
      return 'Error';
    default:
      return 'Stopped';
  }
}
</script>

<template>
  <div class="card workload-card">
    <div class="card-header workload-header">
      <div class="header-left">
        <div class="header-icon-box">
          <svg viewBox="0 0 24 24" class="header-icon"><path :d="mdiDocker" /></svg>
        </div>
        <h3 class="card-title">Apps & Containers</h3>
        <span class="badge badge-neutral mono count-badge">
          {{ runningCount }}/{{ totalCount }}
        </span>
      </div>
      <button class="header-action-btn" @click="navigateTo('docker')" title="Manage Apps & Containers">
        <span>Manage</span>
        <svg viewBox="0 0 24 24" class="chevron-svg"><path :d="mdiChevronRight" /></svg>
      </button>
    </div>

    <!-- Loading Skeleton -->
    <div v-if="nasState.loading && totalCount === 0" class="workload-list">
      <div v-for="s in 3" :key="s" class="workload-item skeleton-item">
        <div class="item-meta">
          <SkeletonBox width="130px" height="14px" />
          <SkeletonBox width="140px" height="11px" />
        </div>
        <SkeletonBox width="60px" height="20px" rounded="4px" />
      </div>
    </div>

    <!-- Empty State -->
    <div v-else-if="totalCount === 0" class="empty-state">
      <span>No apps or containers running.</span>
      <button class="btn btn-xs btn-secondary" @click="navigateTo('docker')">Explore Catalog</button>
    </div>

    <!-- Apps & Containers List -->
    <div v-else class="workload-list">
      <!-- Docker Compose Apps -->
      <div
        v-for="app in dockerApps"
        :key="app.name"
        class="workload-item"
        @click="openApp(app)"
        role="button"
        tabindex="0"
      >
        <div class="item-meta">
          <div class="item-row-top">
            <span class="type-pill pill-app">APP</span>
            <span class="item-title" :title="app.title || app.appId">
              {{ app.title || app.appId }}
            </span>
          </div>
          <div class="item-subtitle mono" :title="`${app.containers.length} containers · ${app.statusText}`">
            {{ app.containers.length }} containers · {{ app.statusText }}
          </div>
        </div>

        <span class="status-chip" :class="getAppStateClass(app.state)">
          <span class="status-dot"></span>
          {{ getAppStateLabel(app.state) }}
        </span>
      </div>

      <!-- LXC System Containers -->
      <div
        v-for="lx in lxcContainers"
        :key="lx.name"
        class="workload-item"
        @click="openLxc(lx)"
        role="button"
        tabindex="0"
      >
        <div class="item-meta">
          <div class="item-row-top">
            <span class="type-pill pill-lxc">LXC</span>
            <span class="item-title" :title="lx.containerName">
              {{ lx.containerName }}
            </span>
          </div>
          <div class="item-subtitle mono" :title="`${lx.distribution} (${lx.ipv4Address || 'DHCP'})`">
            {{ lx.distribution }} · {{ lx.ipv4Address || 'DHCP' }}
          </div>
        </div>

        <span class="status-chip" :class="lx.state === 'RUNNING' ? 'chip-running' : 'chip-stopped'">
          <span class="status-dot"></span>
          {{ lx.state === 'RUNNING' ? 'Running' : 'Stopped' }}
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

.item-row-top {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.type-pill {
  font-size: 9px;
  font-weight: 700;
  padding: 1px 4px;
  border-radius: 3px;
  font-family: var(--font-mono);
  flex-shrink: 0;
  letter-spacing: 0.3px;
}

.pill-app {
  background: rgba(56, 189, 248, 0.14);
  color: #38bdf8;
  border: 1px solid rgba(56, 189, 248, 0.28);
}

.pill-lxc {
  background: rgba(168, 85, 247, 0.14);
  color: #c084fc;
  border: 1px solid rgba(168, 85, 247, 0.28);
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

.chip-info {
  background: rgba(56, 189, 248, 0.12);
  color: #38bdf8;
  border: 1px solid rgba(56, 189, 248, 0.25);
}
.chip-info .status-dot {
  background: #38bdf8;
}

.chip-warning {
  background: rgba(245, 158, 11, 0.12);
  color: #f59e0b;
  border: 1px solid rgba(245, 158, 11, 0.25);
}
.chip-warning .status-dot {
  background: #f59e0b;
}

.chip-danger {
  background: rgba(244, 63, 94, 0.12);
  color: #f43f5e;
  border: 1px solid rgba(244, 63, 94, 0.25);
}
.chip-danger .status-dot {
  background: #f43f5e;
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