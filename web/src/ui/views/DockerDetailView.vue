<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import {
  mdiAlertCircle,
  mdiArrowLeft,
  mdiCheckCircle,
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
import CodeMirrorEditor from '../components/CodeMirrorEditor.vue';
import DockerTerminalModal, { type TerminalContainerOption } from '../components/DockerTerminalModal.vue';
import { services } from '../../container';
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
    activeSubTab?: 'summary' | 'containers' | 'compose' | 'logs';
  }>(),
  {
    activeSubTab: 'summary',
  }
);

const emit = defineEmits<{
  (e: 'back'): void;
  (e: 'update:activeSubTab', tab: 'summary' | 'containers' | 'compose' | 'logs'): void;
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
    // In Docker apps, autostart is managed in compose/config
    const newSetting = !app.value.autoStart;
    await services.apps.updateApp(app.value.name, app.value.composeYaml, app.value.title);
    notify(
      'success',
      `Autostart setting updated for ${app.value.title || cleanAppName.value}`
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
    const running = terminalContainerOptions.value.find((c) => c.state === 'RUNNING');
    terminalDefaultContainerId.value = running?.id || terminalContainerOptions.value[0]?.id || '';
  }
  showTerminalModal.value = true;
}

// -----------------------------------------------------------------------------
// Compose YAML Editor
// -----------------------------------------------------------------------------
const isEditMode = ref(false);
const editableYaml = ref('');
const isSavingYaml = ref(false);

watch(
  app,
  (a) => {
    if (a && !isEditMode.value) {
      editableYaml.value = a.composeYaml || '';
    }
  },
  { immediate: true }
);

function toggleEditMode() {
  if (isEditMode.value) {
    // Cancel edit
    editableYaml.value = app.value?.composeYaml || '';
    isEditMode.value = false;
  } else {
    editableYaml.value = app.value?.composeYaml || '';
    isEditMode.value = true;
  }
}

async function handleSaveYaml() {
  if (!app.value || isSavingYaml.value) return;
  isSavingYaml.value = true;
  try {
    await runTrackedTask(`App "${app.value.title || cleanAppName.value}": Redeploy Compose`, async () => {
      await services.apps.updateApp(app.value!.name, editableYaml.value, app.value!.title);
    });
    notify('success', `Saved Compose specification and re-deployed stack`);
    isEditMode.value = false;
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to update Compose: ${err.message}`);
  } finally {
    isSavingYaml.value = false;
  }
}

function copyYaml() {
  navigator.clipboard.writeText(editableYaml.value);
  notify('info', 'Compose YAML copied to clipboard');
}

// -----------------------------------------------------------------------------
// Logs Viewer
// -----------------------------------------------------------------------------
const logTail = ref(100);
const selectedService = ref('');
const logsContent = ref('');
const isLoadingLogs = ref(false);

async function fetchLogs() {
  if (!app.value) return;
  isLoadingLogs.value = true;
  try {
    const res = await services.apps.getAppLogs(
      app.value.name,
      logTail.value,
      selectedService.value || undefined
    );
    logsContent.value = res.logs || 'No log entries recorded for this application.';
  } catch (err: any) {
    logsContent.value = `Failed to fetch logs: ${err.message || 'Unknown error'}`;
  } finally {
    isLoadingLogs.value = false;
  }
}

function copyLogs() {
  navigator.clipboard.writeText(logsContent.value);
  notify('info', 'Logs copied to clipboard');
}

watch(
  () => props.activeSubTab,
  (tab) => {
    if (tab === 'logs' && !logsContent.value) {
      fetchLogs();
    }
  }
);
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
                  class="badge mono font-xs"
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
                <span class="metric-val mono">
                  {{ app?.state === 'RUNNING' && app?.totalCpuPercent != null ? (Number(app.totalCpuPercent) || 0).toFixed(1) + '%' : '-' }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Total Real Memory (RSS)</span>
                <span class="metric-val mono">
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
                <span class="metric-val mono">{{ app?.containers?.length || 0 }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Services</span>
                <span class="metric-val mono">{{ app?.services?.length || 0 }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Autostart on Boot</span>
                <span class="metric-val mono">{{ app?.autoStart ? 'Enabled' : 'Manual' }}</span>
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
                <span v-for="s in app.services" :key="s" class="badge badge-neutral mono font-xs">
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
                  <th style="text-align: right">Terminal</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="c in app.containers" :key="c.containerId || c.name">
                  <td>
                    <div class="mono font-sm" style="font-weight: 600">{{ c.name }}</div>
                    <div class="text-muted font-xs mono">{{ c.serviceName }}</div>
                  </td>
                  <td>
                    <span
                      class="badge font-xs mono"
                      :class="c.state === 'running' || c.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'"
                    >
                      {{ c.state }}
                    </span>
                  </td>
                  <td class="mono font-xs">
                    {{ (c.cpuPercent || 0).toFixed(1) }}%
                  </td>
                  <td class="mono font-xs">
                    {{ c.memoryMib ? c.memoryMib + ' MiB' : '-' }}
                  </td>
                  <td style="text-align: right">
                    <button
                      class="btn btn-xs btn-secondary"
                      :disabled="c.state !== 'running' && c.state !== 'RUNNING'"
                      title="Attach web terminal"
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

    <!-- Subtab 2: Containers Table -->
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
                    <div class="mono font-sm" style="font-weight: 600">{{ c.name }}</div>
                    <div class="text-muted font-xs mono" style="opacity: 0.7">{{ c.containerId?.slice(0, 12) }}</div>
                  </td>
                  <td><span class="badge badge-neutral mono font-xs">{{ c.serviceName }}</span></td>
                  <td class="mono font-xs" style="max-width: 220px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap" :title="c.image">
                    {{ c.image }}
                  </td>
                  <td>
                    <span
                      class="badge font-xs mono"
                      :class="c.state === 'running' || c.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'"
                    >
                      {{ c.state }}
                    </span>
                  </td>
                  <td class="mono font-xs">{{ (c.cpuPercent || 0).toFixed(1) }}%</td>
                  <td class="mono font-xs">{{ c.memoryMib ? c.memoryMib + ' MiB' : '-' }}</td>
                  <td class="mono font-xs">
                    <span v-if="c.ports && c.ports.length > 0">{{ c.ports.join(', ') }}</span>
                    <span v-else class="text-muted">—</span>
                  </td>
                  <td style="text-align: right">
                    <button
                      class="btn btn-xs btn-primary"
                      :disabled="c.state !== 'running' && c.state !== 'RUNNING'"
                      title="Attach interactive web terminal"
                      @click="openTerminal(c.containerId || c.name)"
                    >
                      <MdiIcon :path="mdiConsole" size="13" style="margin-right: 4px" />
                      <span>Attach</span>
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    <!-- Subtab 3: Compose YAML -->
    <div v-else-if="props.activeSubTab === 'compose'" class="docker-tab-content">
      <div class="card detail-card">
        <div class="card-header" style="display: flex; justify-content: space-between; align-items: center">
          <div>
            <h3 class="card-title">Docker Compose Specification</h3>
            <p class="card-subtitle">Source docker-compose.yml defining services, volumes and network topology</p>
          </div>
          <div style="display: flex; gap: 8px">
            <button class="btn btn-sm btn-secondary" @click="copyYaml">
              <MdiIcon :path="mdiContentCopy" size="14" style="margin-right: 4px" />
              <span>Copy YAML</span>
            </button>
            <button
              class="btn btn-sm"
              :class="isEditMode ? 'btn-secondary' : 'btn-primary'"
              @click="toggleEditMode"
            >
              <span>{{ isEditMode ? 'Cancel Edit' : 'Edit Compose' }}</span>
            </button>
            <button
              v-if="isEditMode"
              class="btn btn-sm btn-success"
              :disabled="isSavingYaml"
              @click="handleSaveYaml"
            >
              <span>{{ isSavingYaml ? 'Applying Changes...' : 'Save & Redeploy' }}</span>
            </button>
          </div>
        </div>
        <div class="card-body" style="padding: 0; min-height: 440px">
          <CodeMirrorEditor
            v-model="editableYaml"
            :read-only="!isEditMode"
            language="yaml"
            style="min-height: 440px; border-radius: 0 0 6px 6px"
          />
        </div>
      </div>
    </div>

    <!-- Subtab 4: Logs -->
    <div v-else-if="props.activeSubTab === 'logs'" class="docker-tab-content">
      <div class="card detail-card">
        <div class="card-header" style="display: flex; justify-content: space-between; align-items: center">
          <div>
            <h3 class="card-title">Compose Application Logs</h3>
            <p class="card-subtitle">Real-time aggregated stdout/stderr streams from stack services</p>
          </div>
          <div style="display: flex; align-items: center; gap: 8px">
            <!-- Service Selector -->
            <select v-if="app?.services && app.services.length > 0" v-model="selectedService" class="form-select font-xs" @change="fetchLogs">
              <option value="">All Services</option>
              <option v-for="s in app.services" :key="s" :value="s">{{ s }}</option>
            </select>
            <select v-model.number="logTail" class="form-select font-xs" @change="fetchLogs">
              <option :value="50">Last 50 lines</option>
              <option :value="100">Last 100 lines</option>
              <option :value="250">Last 250 lines</option>
              <option :value="500">Last 500 lines</option>
            </select>
            <button class="btn btn-sm btn-secondary" :disabled="isLoadingLogs" @click="fetchLogs">
              <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" />
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
