<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import {
  mdiAlertCircle,
  mdiArrowLeft,
  mdiCheckCircle,
  mdiChevronDoubleDown,
  mdiClose,
  mdiConsole,
  mdiContentCopy,
  mdiDocker,
  mdiFileDocumentOutline,
  mdiInformationOutline,
  mdiLayers,
  mdiOpenInNew,
  mdiPlay,
  mdiPlus,
  mdiPower,
  mdiRefresh,
  mdiRestart,
  mdiServer,
  mdiStop,
  mdiTrashCanOutline,
  mdiUpdate,
} from '@mdi/js';
import MdiIcon from '../components/MdiIcon.vue';
import ComposeFormEditor from '../components/ComposeFormEditor.vue';
import EmbeddedTerminal from '../components/EmbeddedTerminal.vue';
import DockerTerminalModal, { type TerminalContainerOption } from '../components/DockerTerminalModal.vue';
import { useAutoScroll } from '../composables/useAutoScroll';
import { services } from '../../container';
import { composeYamlToForm, formToComposeYaml } from '../../domain/composeModel';
import {
  formatBytes,
  type DockerApp,
  type DockerAppContainer,
} from '../../domain/models';
import {
  nasState,
  notify,
  refreshAllData,
  runTrackedTask,
} from '../store';
import { confirmDelete } from '../modal';

const props = withDefaults(
  defineProps<{
    appName: string;
    activeSubTab?: 'summary' | 'console' | 'configuration' | 'containers' | 'compose' | 'logs';
  }>(),
  {
    activeSubTab: 'summary',
  }
);

const emit = defineEmits<{
  (e: 'back'): void;
  (e: 'update:activeSubTab', tab: 'summary' | 'console' | 'configuration' | 'containers' | 'compose' | 'logs'): void;
}>();
const isBusy = ref(false);

const cleanAppName = computed(() => {
  return props.appName.replace(/^dockerApps\//, '').trim();
});

const app = computed<DockerApp | null>(() => {
  const target = cleanAppName.value;
  return (
    nasState.dockerApps.find(
      (a) =>
        a.name === target ||
        a.name === `dockerApps/${target}` ||
        a.appId === target ||
        a.title === target
    ) || null
  );
});

// -----------------------------------------------------------------------------
// Power Controls
// -----------------------------------------------------------------------------
async function handleStart() {
  if (!app.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`App "${app.value.title || cleanAppName.value}": Start`, async () => {
      await services.apps.startApp(app.value!.name);
    });
    notify('success', `Application "${app.value.title || cleanAppName.value}" started`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Start failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleStop() {
  if (!app.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`App "${app.value.title || cleanAppName.value}": Stop`, async () => {
      await services.apps.stopApp(app.value!.name);
    });
    notify('info', `Application "${app.value.title || cleanAppName.value}" stopped`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Stop failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleRestart() {
  if (!app.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`App "${app.value.title || cleanAppName.value}": Restart`, async () => {
      await services.apps.restartApp(app.value!.name);
    });
    notify('info', `Application "${app.value.title || cleanAppName.value}" restarted`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Restart failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleDeleteApp() {
  if (!app.value || isBusy.value) return;
  const confirmed = await confirmDelete({
    kind: 'Docker Application',
    name: app.value.title || cleanAppName.value,
    details: 'The Docker Compose service stack, containers and associated virtual network will be removed.',
    confirmText: 'Delete App',
  });
  if (!confirmed.confirmed) return;

  isBusy.value = true;
  try {
    await runTrackedTask(`App "${app.value.title || cleanAppName.value}": Delete`, async () => {
      await services.apps.deleteApp(app.value!.name, false);
    });
    notify('warning', `Docker application "${cleanAppName.value}" deleted`);
    emit('back');
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Delete failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

// -----------------------------------------------------------------------------
// Upgrade App
// -----------------------------------------------------------------------------
const isUpgrading = ref(false);

async function handleUpgradeApp() {
  if (!app.value || isUpgrading.value) return;
  const ok = window.confirm(
    `Upgrade application "${app.value.title || cleanAppName.value}"?\nThis will pull the latest images and recreate containers.`
  );
  if (!ok) return;

  isUpgrading.value = true;
  try {
    await runTrackedTask(`App "${app.value.title || cleanAppName.value}": Upgrade`, async () => {
      await services.apps.upgradeApp(app.value!.name, true);
    });
    notify('success', `Upgrade completed for "${app.value.title || cleanAppName.value}"`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Upgrade failed: ${err.message}`);
  } finally {
    isUpgrading.value = false;
  }
}

// -----------------------------------------------------------------------------
// Autostart Toggle
// -----------------------------------------------------------------------------
const isTogglingAutostart = ref(false);

async function toggleAutostart() {
  if (!app.value || isTogglingAutostart.value) return;
  isTogglingAutostart.value = true;
  try {
    const parsed = composeYamlToForm(app.value.composeYaml || '');
    const newAutoStart = !app.value.autoStart;
    parsed.autoStart = newAutoStart;
    const targetRestart = newAutoStart ? 'unless-stopped' : 'no';
    for (const s of parsed.services) {
      s.restart = targetRestart;
    }
    const newYaml = formToComposeYaml(parsed);
    await services.apps.updateApp(app.value.name, newYaml, app.value.title);
    notify(
      'success',
      `Autostart ${newAutoStart ? 'enabled' : 'disabled'} for ${app.value.title || cleanAppName.value}`
    );
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to update autostart: ${err.message}`);
  } finally {
    isTogglingAutostart.value = false;
  }
}

// -----------------------------------------------------------------------------
// Terminal Modal
// -----------------------------------------------------------------------------
const showTerminalModal = ref(false);
const terminalDefaultContainerId = ref('');

const terminalContainerOptions = computed<TerminalContainerOption[]>(() => {
  if (!app.value?.containers) return [];
  return app.value.containers.map((c) => ({
    id: c.containerId || c.name,
    name: c.name || c.containerId,
    serviceName: c.serviceName,
    state: c.state,
  }));
});

function openTerminal(containerId?: string) {
  if (containerId) {
    terminalDefaultContainerId.value = containerId;
  } else {
    const running = terminalContainerOptions.value.find((c) => c.state === 'RUNNING' || c.state === 'running');
    terminalDefaultContainerId.value = running?.id || terminalContainerOptions.value[0]?.id || '';
  }
  emit('update:activeSubTab', 'console');
}

const activeConsoleContainerId = computed(() => {
  if (terminalDefaultContainerId.value) return terminalDefaultContainerId.value;
  const running = terminalContainerOptions.value.find((c) => c.state === 'RUNNING' || c.state === 'running');
  return running?.id || terminalContainerOptions.value[0]?.id || '';
});

function popoutDockerTerminal() {
  const cid = activeConsoleContainerId.value;
  if (!cid) return;
  const url = `/terminal?container=${encodeURIComponent(cid)}`;
  window.open(
    url,
    `docker_terminal_${cid}`,
    'width=1000,height=650,location=no,toolbar=no,menubar=no,status=no,resizable=yes'
  );
}

// -----------------------------------------------------------------------------
// Compose Configuration & Form/YAML Editor
// -----------------------------------------------------------------------------
const isSavingConfig = ref(false);

async function handleSaveFormConfig(data: { appId: string; title: string; composeYaml: string }) {
  if (!app.value || isSavingConfig.value) return;
  isSavingConfig.value = true;
  try {
    await runTrackedTask(`App "${data.title || cleanAppName.value}": Update Configuration`, async () => {
      await services.apps.updateApp(app.value!.name, data.composeYaml, data.title);
    });
    notify('success', `Configuration saved and stack re-deployed`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to update configuration: ${err.message}`);
  } finally {
    isSavingConfig.value = false;
  }
}

// -----------------------------------------------------------------------------
// Logs Viewer
// -----------------------------------------------------------------------------
const logTail = ref(100);
const selectedService = ref('');
const logsContent = ref('');
const isLoadingLogs = ref(false);
let logPollTimer: any = null;

const {
  containerRef: logContainerRef,
  autoScroll,
  scrollToBottom,
  onScroll: onLogScroll,
  toggleAutoScroll,
} = useAutoScroll();

function startLogPolling() {
  stopLogPolling();
  logPollTimer = setInterval(() => {
    if (props.activeSubTab === 'logs') {
      fetchLogs(true);
    } else {
      stopLogPolling();
    }
  }, 2500);
}

function stopLogPolling() {
  if (logPollTimer) {
    clearInterval(logPollTimer);
    logPollTimer = null;
  }
}

async function fetchLogs(silent = false) {
  if (!app.value) return;
  if (!silent) isLoadingLogs.value = true;
  try {
    const res = await services.apps.getAppLogs(
      app.value.name,
      logTail.value,
      selectedService.value || undefined
    );
    logsContent.value = res.logs || 'No log entries recorded for this application.';
    scrollToBottom();
  } catch (err: any) {
    if (!silent) logsContent.value = `Failed to fetch logs: ${err.message || 'Unknown error'}`;
  } finally {
    if (!silent) {
      isLoadingLogs.value = false;
      scrollToBottom();
    }
  }
}

function copyLogs() {
  navigator.clipboard.writeText(logsContent.value);
  notify('info', 'Logs copied to clipboard');
}

watch(
  () => props.activeSubTab,
  (tab) => {
    if (tab === 'logs') {
      autoScroll.value = true;
      fetchLogs(false);
      startLogPolling();
    } else {
      stopLogPolling();
    }
  },
  { immediate: true }
);

onUnmounted(() => {
  stopLogPolling();
});
</script>

<template>
  <div class="docker-detail-view">
    <!-- Subtab 1: Summary -->
    <div v-if="props.activeSubTab === 'summary'" class="docker-tab-content">
      <div class="grid-2">
        <!-- Live Real-Time Telemetry Card -->
        <div class="card detail-card">
          <div class="card-header">
            <h3 class="card-title">Application Status & Telemetry</h3>
          </div>
          <div class="card-body">
            <div class="metrics-grid">
              <div class="metric-item">
                <span class="metric-label">Status</span>
                <span
                  class="badge font-xs"
                  :class="
                    app?.state === 'RUNNING'
                      ? 'badge-success'
                      : app?.state === 'PARTIALLY_RUNNING'
                      ? 'badge-warning'
                      : 'badge-neutral'
                  "
                  style="align-self: flex-start"
                >
                  {{ app?.state || 'STOPPED' }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Total Real CPU</span>
                <span class="metric-val">
                  {{ app?.state === 'RUNNING' && app?.totalCpuPercent != null ? (Number(app.totalCpuPercent) || 0).toFixed(1) + '%' : '-' }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Total Real Memory (RSS)</span>
                <span class="metric-val">
                  {{
                    app?.state === 'RUNNING' && app?.totalMemoryMib
                      ? (Number(app.totalMemoryMib) || 0) >= 1024
                        ? ((Number(app.totalMemoryMib) || 0) / 1024).toFixed(2) + ' GiB'
                        : app.totalMemoryMib + ' MiB'
                      : '-'
                  }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Containers</span>
                <span class="metric-val">{{ app?.containers?.length || 0 }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Services</span>
                <span class="metric-val">{{ app?.services?.length || 0 }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Autostart on Boot</span>
                <button
                  type="button"
                  class="badge interactive font-xs"
                  :class="app?.autoStart ? 'badge-success' : 'badge-neutral'"
                  style="align-self: flex-start; cursor: pointer"
                  :disabled="isTogglingAutostart"
                  @click="toggleAutostart"
                >
                  {{ app?.autoStart ? 'Enabled (On Boot)' : 'Disabled (Manual)' }}
                </button>
              </div>
            </div>

            <!-- Web UI Portals & Ports -->
            <div style="margin-top: 18px; padding-top: 14px; border-top: 1px solid var(--border-subtle)">
              <span class="form-label" style="display: block; margin-bottom: 8px">Exposed Portals & Network Ports</span>
              <div v-if="!app?.portMappings || app.portMappings.length === 0" class="text-muted font-xs">
                No external host ports exposed.
              </div>
              <div v-else style="display: flex; flex-wrap: wrap; gap: 8px">
                <span v-for="port in app.portMappings" :key="port" class="badge badge-info mono font-xs">
                  {{ port }}
                </span>
              </div>
            </div>

            <!-- Service Badges -->
            <div v-if="app?.services && app.services.length > 0" style="margin-top: 14px">
              <span class="form-label" style="display: block; margin-bottom: 8px">Stack Services</span>
              <div style="display: flex; flex-wrap: wrap; gap: 6px">
                <span v-for="s in app.services" :key="s" class="badge badge-neutral font-xs">
                  {{ s }}
                </span>
              </div>
            </div>
          </div>
        </div>

        <!-- Quick Containers Status Card -->
        <div class="card detail-card">
          <div class="card-header" style="display: flex; justify-content: space-between; align-items: center">
            <h3 class="card-title">Stack Containers</h3>
            <button class="btn btn-xs btn-secondary" @click="emit('update:activeSubTab', 'containers')">
              View Full Table
            </button>
          </div>
          <div class="card-body">
            <div v-if="!app?.containers || app.containers.length === 0" class="empty-state">
              No containers currently deployed for this stack.
            </div>
            <table v-else class="data-table">
              <thead>
                <tr>
                  <th>Container</th>
                  <th>State</th>
                  <th>CPU</th>
                  <th>RAM</th>
                  <th style="text-align: right">Console</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="c in app.containers" :key="c.containerId || c.name">
                  <td>
                    <div class="font-sm" style="font-weight: 600">{{ c.name }}</div>
                    <div class="text-muted font-xs">{{ c.serviceName }}</div>
                  </td>
                  <td>
                    <span
                      class="badge font-xs"
                      :class="c.state === 'running' || c.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'"
                    >
                      {{ c.state }}
                    </span>
                  </td>
                  <td class="font-xs">
                    {{ (c.cpuPercent || 0).toFixed(1) }}%
                  </td>
                  <td class="font-xs">
                    {{ c.memoryMib ? c.memoryMib + ' MiB' : '-' }}
                  </td>
                  <td style="text-align: right">
                    <button
                      class="btn btn-xs btn-secondary"
                      :disabled="c.state !== 'running' && c.state !== 'RUNNING'"
                      title="Open interactive shell console"
                      @click="openTerminal(c.containerId || c.name)"
                    >
                      <MdiIcon :path="mdiConsole" size="13" />
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    <!-- Subtab 2: Console / Interactive Shell -->
    <div v-else-if="props.activeSubTab === 'console'" class="docker-tab-content">
      <div v-if="app?.state !== 'RUNNING' && app?.state !== 'PARTIALLY_RUNNING'" class="card detail-card" style="padding: 48px; text-align: center">
        <MdiIcon :path="mdiAlertCircle" size="48" style="color: var(--warning-text); margin-bottom: 12px" />
        <h4 style="font-size: 16px; font-weight: 600; margin-bottom: 6px">Application is not running</h4>
        <p class="text-muted" style="font-size: 13px; max-width: 400px; margin: 0 auto 16px">
          Start the application stack to attach an interactive container shell.
        </p>
        <button class="btn btn-success" :disabled="isBusy" @click="handleStart">
          <MdiIcon :path="mdiPlay" size="14" style="margin-right: 4px" />
          Start Application
        </button>
      </div>
      <div v-else class="card detail-card" style="padding: 0; overflow: hidden">
        <div
          v-if="terminalContainerOptions.length > 1"
          style="display: flex; align-items: center; gap: 10px; padding: 8px 14px; background: var(--bg-surface); border-bottom: 1px solid var(--border-subtle)"
        >
          <span class="font-xs" style="font-weight: 600; color: var(--text-secondary)">Target Container:</span>
          <select
            :value="activeConsoleContainerId"
            class="form-select font-xs"
            @change="terminalDefaultContainerId = ($event.target as HTMLSelectElement).value"
          >
            <option v-for="opt in terminalContainerOptions" :key="opt.id" :value="opt.id">
              {{ opt.name }} ({{ opt.serviceName || 'service' }})
            </option>
          </select>
        </div>
        <EmbeddedTerminal
          v-if="activeConsoleContainerId"
          :key="activeConsoleContainerId"
          :container-id="activeConsoleContainerId"
          :container-name="app?.title || cleanAppName"
          endpoint-type="docker"
          @popout="popoutDockerTerminal"
        />
      </div>
    </div>

    <!-- Subtab 3: Containers Table -->
    <div v-else-if="props.activeSubTab === 'containers'" class="docker-tab-content">
      <div class="card detail-card">
        <div class="card-header">
          <h3 class="card-title">Containers Inventory</h3>
          <p class="card-subtitle">Active and stopped containers managed under the {{ cleanAppName }} stack</p>
        </div>
        <div class="card-body">
          <div v-if="!app?.containers || app.containers.length === 0" class="empty-state">
            No containers found in this application stack.
          </div>
          <div v-else class="table-wrap">
            <table class="data-table">
              <thead>
                <tr>
                  <th>Container Name</th>
                  <th>Service</th>
                  <th>Image</th>
                  <th>State</th>
                  <th>Real CPU</th>
                  <th>Real Memory</th>
                  <th>Ports</th>
                  <th style="text-align: right">Actions</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="c in app.containers" :key="c.containerId || c.name">
                  <td>
                    <div class="font-sm" style="font-weight: 600">{{ c.name }}</div>
                    <div class="text-muted font-xs mono" style="opacity: 0.7">{{ c.containerId?.slice(0, 12) }}</div>
                  </td>
                  <td><span class="badge badge-neutral font-xs">{{ c.serviceName }}</span></td>
                  <td class="mono font-xs" style="max-width: 220px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap" :title="c.image">
                    {{ c.image }}
                  </td>
                  <td>
                    <span
                      class="badge font-xs"
                      :class="c.state === 'running' || c.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'"
                    >
                      {{ c.state }}
                    </span>
                  </td>
                  <td class="font-xs">{{ (c.cpuPercent || 0).toFixed(1) }}%</td>
                  <td class="font-xs">{{ c.memoryMib ? c.memoryMib + ' MiB' : '-' }}</td>
                  <td class="mono font-xs">
                    <span v-if="c.ports && c.ports.length > 0">{{ c.ports.join(', ') }}</span>
                    <span v-else class="text-muted">—</span>
                  </td>
                  <td style="text-align: right">
                    <button
                      class="btn btn-xs btn-primary"
                      :disabled="c.state !== 'running' && c.state !== 'RUNNING'"
                      title="Open interactive web console"
                      @click="openTerminal(c.containerId || c.name)"
                    >
                      <MdiIcon :path="mdiConsole" size="13" style="margin-right: 4px" />
                      <span>Console</span>
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    <!-- Subtab 4: Configuration (Form & YAML) -->
    <div v-else-if="props.activeSubTab === 'configuration' || props.activeSubTab === 'compose'" class="docker-tab-content">
      <div class="card detail-card" style="padding: 16px">
        <ComposeFormEditor
          v-if="app"
          :key="app.name"
          :initial-app-id="app.appId || cleanAppName"
          :initial-title="app.title"
          :initial-compose-yaml="app.composeYaml"
          :is-edit-mode="true"
          :is-saving="isSavingConfig"
          @save="handleSaveFormConfig"
        />
      </div>
    </div>

    <!-- Subtab 5: Logs -->
    <div v-else-if="props.activeSubTab === 'logs'" class="docker-tab-content">
      <div class="card detail-card">
        <div class="card-header" style="display: flex; justify-content: space-between; align-items: center">
          <div>
            <h3 class="card-title">Compose Application Logs</h3>
            <p class="card-subtitle">Real-time aggregated stdout/stderr streams from stack services</p>
          </div>
          <div style="display: flex; align-items: center; gap: 8px">
            <!-- Service Selector -->
            <select v-if="app?.services && app.services.length > 0" v-model="selectedService" class="form-select font-xs" @change="fetchLogs(false)">
              <option value="">All Services</option>
              <option v-for="s in app.services" :key="s" :value="s">{{ s }}</option>
            </select>
            <select v-model.number="logTail" class="form-select font-xs" @change="fetchLogs(false)">
              <option :value="50">Last 50 lines</option>
              <option :value="100">Last 100 lines</option>
              <option :value="250">Last 250 lines</option>
              <option :value="500">Last 500 lines</option>
            </select>
            <button
              class="btn btn-sm"
              :class="autoScroll ? 'btn-primary' : 'btn-secondary'"
              :title="autoScroll ? 'Auto-scroll enabled (click to pause)' : 'Click to follow logs automatically'"
              @click="toggleAutoScroll"
            >
              <MdiIcon :path="mdiChevronDoubleDown" size="14" style="margin-right: 4px" />
              <span>Auto-scroll</span>
            </button>
            <button class="btn btn-sm btn-secondary" :disabled="isLoadingLogs" @click="fetchLogs(false)">
              <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" :class="{ 'spin-icon': isLoadingLogs }" />
              <span>Refresh</span>
            </button>
            <button class="btn btn-sm btn-secondary" :disabled="!logsContent" @click="copyLogs">
              <MdiIcon :path="mdiContentCopy" size="14" style="margin-right: 4px" />
              <span>Copy</span>
            </button>
          </div>
        </div>
        <div class="card-body">
          <pre
            ref="logContainerRef"
            class="mono font-xs"
            style="
              background: var(--bg-base);
              padding: 16px;
              border-radius: 6px;
              border: 1px solid var(--border-subtle);
              color: var(--text-secondary);
              max-height: 520px;
              overflow-y: auto;
              white-space: pre-wrap;
              word-break: break-all;
            "
            @scroll="onLogScroll"
          >{{ isLoadingLogs ? 'Loading logs...' : logsContent }}</pre>
        </div>
      </div>
    </div>

    <!-- Terminal Modal Component -->
    <DockerTerminalModal
      v-if="showTerminalModal"
      :app-title="app?.title || cleanAppName"
      :containers="terminalContainerOptions"
      :initial-container-id="terminalDefaultContainerId"
      :endpoint-type="'docker'"
      @close="showTerminalModal = false"
    />
  </div>
</template>

<style scoped>
.docker-detail-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
}

.docker-header-card {
  padding: 14px 18px 0 18px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
}

.docker-header-main {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 12px;
}

.docker-title {
  font-size: 17px;
  font-weight: 700;
  color: var(--text-primary);
  margin: 0;
}

.docker-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.docker-subtabs {
  display: flex;
  gap: 4px;
  border-top: 1px solid var(--border-subtle);
  padding-top: 8px;
  padding-bottom: 8px;
  overflow-x: auto;
}

.subtab-item {
  background: transparent;
  border: 1px solid transparent;
  color: var(--text-secondary);
  font-size: 12px;
  padding: 5px 12px;
  border-radius: 4px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.subtab-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.subtab-item.active {
  background: var(--accent-bg-subtle);
  color: var(--accent-primary);
  border-color: var(--accent-primary);
  font-weight: 600;
}

.docker-tab-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.grid-2 {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 16px;
}

.detail-card {
  padding: 18px 20px;
}

.metrics-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

.metric-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.metric-label {
  font-size: 11px;
  color: var(--text-muted);
}

.metric-val {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.empty-state {
  padding: 32px;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
}

.form-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
}

.form-select {
  background: var(--input-bg);
  border: 1px solid var(--border-subtle);
  color: var(--text-primary);
  border-radius: 4px;
  padding: 6px 10px;
  font-size: 13px;
  outline: none;
}

.form-select:focus {
  border-color: var(--accent-primary);
}

.font-sm {
  font-size: 12px;
}

.font-xs {
  font-size: 10.5px;
}
</style>
