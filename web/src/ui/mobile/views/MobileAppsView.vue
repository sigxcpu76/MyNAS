<script setup lang="ts">
import { computed, ref } from 'vue';
import {
  mdiPlay,
  mdiStop,
  mdiRefresh,
  mdiOpenInNew,
  mdiConsole,
  mdiInformationOutline,
  mdiCheckCircle,
  mdiAlertCircle,
  mdiMagnify,
  mdiServer,
  mdiTune,
} from '@mdi/js';
import MdiIcon from '../../components/MdiIcon.vue';
import BottomSheet from '../../components/BottomSheet.vue';
import { nasState, notify, refreshAllData } from '../../store';
import { services } from '../../../container';
import type { DockerApp } from '../../../domain/models';

const searchQuery = ref('');
const statusFilter = ref<'ALL' | 'RUNNING' | 'STOPPED'>('ALL');
const selectedApp = ref<DockerApp | null>(null);
const isSheetOpen = ref(false);

const actionLoading = ref(false);
const logsLoading = ref(false);
const appLogs = ref<string>('');
const activeDetailTab = ref<'details' | 'logs'>('details');

const apps = computed(() => nasState.dockerApps || []);

const filteredApps = computed(() => {
  return apps.value.filter((app) => {
    // Search query
    if (searchQuery.value.trim()) {
      const q = searchQuery.value.toLowerCase();
      const matchName = app.title?.toLowerCase().includes(q) || app.appId?.toLowerCase().includes(q);
      if (!matchName) return false;
    }

    // Status filter
    const isRunning = app.state === 'RUNNING' || app.state === 'running';
    if (statusFilter.value === 'RUNNING' && !isRunning) return false;
    if (statusFilter.value === 'STOPPED' && isRunning) return false;

    return true;
  });
});

const runningCount = computed(() => apps.value.filter((a) => a.state === 'RUNNING' || a.state === 'running').length);
const stoppedCount = computed(() => apps.value.length - runningCount.value);

function openAppSheet(app: DockerApp) {
  selectedApp.value = app;
  activeDetailTab.value = 'details';
  appLogs.value = '';
  isSheetOpen.value = true;
}

function isAppRunning(app: DockerApp): boolean {
  return app.state === 'RUNNING' || app.state === 'running';
}

function getAppPortalUrl(app: DockerApp): string | null {
  let target = '';
  if (app.webUiUrl) target = app.webUiUrl;
  else if (app.portals && app.portals.length > 0 && app.portals[0].url) target = app.portals[0].url;
  if (!target) return null;
  if (typeof window !== 'undefined' && window.location && window.location.hostname) {
    const host = window.location.hostname;
    return target.replace(/:\/\/(?:localhost|127\.0\.0\.1|0\.0\.0\.0)(?::|\/|$)/, (match) => {
      return match.replace(/localhost|127\.0\.0\.1|0\.0\.0\.0/, host);
    });
  }
  return target;
}

async function handleStart() {
  if (!selectedApp.value || actionLoading.value) return;
  actionLoading.value = true;
  try {
    await services.apps.startApp(selectedApp.value.appId);
    notify('success', `Started ${selectedApp.value.title || selectedApp.value.appId}`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to start: ${err.message || err}`);
  } finally {
    actionLoading.value = false;
  }
}

async function handleStop() {
  if (!selectedApp.value || actionLoading.value) return;
  actionLoading.value = true;
  try {
    await services.apps.stopApp(selectedApp.value.appId);
    notify('success', `Stopped ${selectedApp.value.title || selectedApp.value.appId}`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to stop: ${err.message || err}`);
  } finally {
    actionLoading.value = false;
  }
}

async function handleRestart() {
  if (!selectedApp.value || actionLoading.value) return;
  actionLoading.value = true;
  try {
    await services.apps.restartApp(selectedApp.value.appId);
    notify('success', `Restarted ${selectedApp.value.title || selectedApp.value.appId}`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to restart: ${err.message || err}`);
  } finally {
    actionLoading.value = false;
  }
}

async function fetchLogs() {
  if (!selectedApp.value) return;
  logsLoading.value = true;
  try {
    const res = await services.apps.getAppLogs(selectedApp.value.appId, 150);
    appLogs.value = res.logs || 'No logs recorded.';
  } catch (err: any) {
    appLogs.value = `Failed to load logs: ${err.message || err}`;
  } finally {
    logsLoading.value = false;
  }
}

function switchDetailTab(tab: 'details' | 'logs') {
  activeDetailTab.value = tab;
  if (tab === 'logs' && !appLogs.value) {
    fetchLogs();
  }
}
</script>

<template>
  <div class="mobile-apps-view">
    <!-- Search & Filter Bar -->
    <div class="filter-header">
      <div class="search-input-box">
        <MdiIcon :path="mdiMagnify" size="18" style="color: var(--text-secondary)" />
        <input
          v-model="searchQuery"
          type="search"
          placeholder="Filter applications..."
          class="mobile-search-input"
        />
      </div>

      <div class="status-pill-group">
        <button
          class="filter-pill"
          :class="{ active: statusFilter === 'ALL' }"
          @click="statusFilter = 'ALL'"
        >
          All ({{ apps.length }})
        </button>
        <button
          class="filter-pill"
          :class="{ active: statusFilter === 'RUNNING' }"
          @click="statusFilter = 'RUNNING'"
        >
          Running ({{ runningCount }})
        </button>
        <button
          class="filter-pill"
          :class="{ active: statusFilter === 'STOPPED' }"
          @click="statusFilter = 'STOPPED'"
        >
          Stopped ({{ stoppedCount }})
        </button>
      </div>
    </div>

    <!-- Apps List Cards -->
    <div class="apps-card-list">
      <div
        v-for="app in filteredApps"
        :key="app.appId"
        class="app-card"
        @click="openAppSheet(app)"
      >
        <div class="app-card-left">
          <div class="app-status-dot" :class="isAppRunning(app) ? 'dot-running' : 'dot-stopped'" />
          <div class="app-name-wrap">
            <div class="app-title">{{ app.title || app.appId }}</div>
            <div class="app-meta">
              <span>{{ app.containers?.length || 0 }} containers</span>
              <span v-if="isAppRunning(app) && app.totalMemoryMib"> • {{ Math.round(app.totalMemoryMib) }} MB</span>
            </div>
          </div>
        </div>

        <div class="app-card-right" @click.stop>
          <a
            v-if="getAppPortalUrl(app)"
            :href="getAppPortalUrl(app)!"
            target="_blank"
            rel="noopener noreferrer"
            class="app-portal-btn"
            title="Open Web Portal"
          >
            <MdiIcon :path="mdiOpenInNew" size="16" />
          </a>
          <span
            class="app-badge"
            :class="isAppRunning(app) ? 'badge-running' : 'badge-stopped'"
          >
            {{ isAppRunning(app) ? 'RUNNING' : 'STOPPED' }}
          </span>
        </div>
      </div>

      <div v-if="filteredApps.length === 0" class="empty-state">
        No applications match your filter.
      </div>
    </div>

    <!-- App Detail Bottom Sheet -->
    <BottomSheet
      v-if="selectedApp"
      v-model="isSheetOpen"
      :title="selectedApp.title || selectedApp.appId"
      :subtitle="`App ID: ${selectedApp.appId}`"
    >
      <div class="sheet-tabs">
        <button
          class="sheet-tab"
          :class="{ active: activeDetailTab === 'details' }"
          @click="switchDetailTab('details')"
        >
          Overview & Actions
        </button>
        <button
          class="sheet-tab"
          :class="{ active: activeDetailTab === 'logs' }"
          @click="switchDetailTab('logs')"
        >
          Live Logs
        </button>
      </div>

      <!-- Details & Actions Tab -->
      <div v-if="activeDetailTab === 'details'" class="sheet-app-details">
        <!-- Power Controls Row -->
        <div class="power-controls-row">
          <button
            class="btn btn-sm btn-success flex-1"
            :disabled="actionLoading || isAppRunning(selectedApp)"
            @click="handleStart"
          >
            <MdiIcon :path="mdiPlay" size="16" />
            <span>Start</span>
          </button>
          <button
            class="btn btn-sm btn-danger flex-1"
            :disabled="actionLoading || !isAppRunning(selectedApp)"
            @click="handleStop"
          >
            <MdiIcon :path="mdiStop" size="16" />
            <span>Stop</span>
          </button>
          <button
            class="btn btn-sm btn-secondary flex-1"
            :disabled="actionLoading || !isAppRunning(selectedApp)"
            @click="handleRestart"
          >
            <MdiIcon :path="mdiRefresh" size="16" :class="{ 'spin-icon': actionLoading }" />
            <span>Restart</span>
          </button>
        </div>

        <!-- Web Portal Direct Link -->
        <div v-if="getAppPortalUrl(selectedApp)" class="portal-cta-box">
          <div class="portal-info">
            <span class="portal-title">Web Management Portal</span>
            <span class="portal-url">{{ getAppPortalUrl(selectedApp) }}</span>
          </div>
          <a
            :href="getAppPortalUrl(selectedApp)!"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-sm btn-primary"
          >
            <span>Open</span>
            <MdiIcon :path="mdiOpenInNew" size="14" />
          </a>
        </div>

        <!-- Containers Info -->
        <div class="spec-card">
          <div class="spec-title">Containers ({{ selectedApp.containers?.length || 0 }})</div>
          <div v-for="c in selectedApp.containers" :key="c.containerId" class="container-row">
            <div class="c-info">
              <span class="c-name">{{ c.name || c.serviceName }}</span>
              <span class="c-image mono font-xs">{{ c.image }}</span>
            </div>
            <span
              class="app-badge"
              :class="c.state === 'running' || c.state === 'RUNNING' ? 'badge-running' : 'badge-stopped'"
            >
              {{ c.state }}
            </span>
          </div>
        </div>

        <!-- Volume Mounts -->
        <div v-if="selectedApp.volumes && selectedApp.volumes.length > 0" class="spec-card">
          <div class="spec-title">Persistent Mounts</div>
          <div v-for="(v, i) in selectedApp.volumes" :key="i" class="mono font-xs mount-row">
            {{ v }}
          </div>
        </div>
      </div>

      <!-- Logs Tab -->
      <div v-else class="sheet-logs-tab">
        <div class="logs-actions">
          <button class="btn btn-xs btn-ghost" :disabled="logsLoading" @click="fetchLogs">
            <MdiIcon :path="mdiRefresh" size="14" :class="{ 'spin-icon': logsLoading }" />
            <span>Refresh Logs</span>
          </button>
        </div>
        <pre class="mobile-logs-pre">{{ appLogs || 'Loading logs...' }}</pre>
      </div>
    </BottomSheet>
  </div>
</template>

<style scoped>
.mobile-apps-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px;
}

.filter-header {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.search-input-box {
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 12px;
  padding: 0 12px;
  height: 42px;
}

.mobile-search-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  color: var(--text-primary, #f8fafc);
  font-size: 14px;
}

.status-pill-group {
  display: flex;
  gap: 6px;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}

.filter-pill {
  padding: 6px 12px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  color: var(--text-secondary, #94a3b8);
  white-space: nowrap;
  cursor: pointer;
  touch-action: manipulation;
}

.filter-pill.active {
  background: var(--accent-primary, #38bdf8);
  color: #0f172a;
  border-color: var(--accent-primary, #38bdf8);
}

.apps-card-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.app-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 14px;
  padding: 12px 14px;
  cursor: pointer;
  touch-action: manipulation;
  transition: transform 0.1s ease, background 0.1s ease;
}

.app-card:active {
  transform: scale(0.98);
  background: var(--bg-hover, #334155);
}

.app-card-left {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.app-status-dot {
  width: 10px;
  height: 10px;
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

.app-name-wrap {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.app-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.app-meta {
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
  margin-top: 2px;
}

.app-card-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.app-portal-btn {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(56, 189, 248, 0.12);
  color: var(--accent-primary, #38bdf8);
}

.app-badge {
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

.sheet-tabs {
  display: flex;
  background: var(--bg-base, #0f172a);
  border-radius: 8px;
  padding: 3px;
  gap: 4px;
  margin-bottom: 14px;
}

.sheet-tab {
  flex: 1;
  padding: 8px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 600;
  background: transparent;
  border: none;
  color: var(--text-secondary, #94a3b8);
  cursor: pointer;
}

.sheet-tab.active {
  background: var(--bg-surface, #1e293b);
  color: var(--text-primary, #f8fafc);
}

.sheet-app-details {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.power-controls-row {
  display: flex;
  gap: 8px;
}

.flex-1 {
  flex: 1;
}

.portal-cta-box {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: rgba(56, 189, 248, 0.08);
  border: 1px solid rgba(56, 189, 248, 0.25);
  border-radius: 12px;
  padding: 10px 14px;
}

.portal-info {
  display: flex;
  flex-direction: column;
}

.portal-title {
  font-size: 12px;
  font-weight: 700;
  color: var(--accent-primary, #38bdf8);
}

.portal-url {
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
  margin-top: 1px;
}

.spec-card {
  background: var(--bg-surface-elevated, #243248);
  border: 1px solid var(--border-color, #334155);
  border-radius: 12px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.spec-title {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--text-secondary, #94a3b8);
}

.container-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.container-row:last-child {
  border-bottom: none;
}

.c-info {
  display: flex;
  flex-direction: column;
}

.c-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary, #f8fafc);
}

.c-image {
  color: var(--text-secondary, #94a3b8);
}

.mount-row {
  padding: 4px 0;
  color: var(--text-secondary, #94a3b8);
  word-break: break-all;
}

.sheet-logs-tab {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.logs-actions {
  display: flex;
  justify-content: flex-end;
}

.mobile-logs-pre {
  background: #090d16;
  border: 1px solid #1f2937;
  border-radius: 8px;
  padding: 10px;
  font-family: ui-monospace, monospace;
  font-size: 11px;
  color: #e2e8f0;
  max-height: 45vh;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

.empty-state {
  text-align: center;
  padding: 30px;
  color: var(--text-secondary, #94a3b8);
  font-size: 13px;
}
</style>
