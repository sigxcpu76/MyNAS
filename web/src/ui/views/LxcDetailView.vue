<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import {
  mdiAlertCircle,
  mdiArrowLeft,
  mdiCheckCircle,
  mdiClose,
  mdiConsole,
  mdiContentCopy,
  mdiCubeOutline,
  mdiDelete,
  mdiFolder,
  mdiFolderOutline,
  mdiInformationOutline,
  mdiNetwork,
  mdiOpenInNew,
  mdiPencil,
  mdiPlay,
  mdiPlus,
  mdiPower,
  mdiRefresh,
  mdiRestart,
  mdiServer,
  mdiSnowflake,
  mdiStop,
  mdiTrashCanOutline,
} from '@mdi/js';
import MdiIcon from '../components/MdiIcon.vue';
import EmbeddedTerminal from '../components/EmbeddedTerminal.vue';
import DockerTerminalModal, { type TerminalContainerOption } from '../components/DockerTerminalModal.vue';
import { services } from '../../container';
import {
  type LxcContainer,
  type LxcMountPoint,
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
    containerName: string;
    activeSubTab?: 'summary' | 'terminal' | 'mounts' | 'configuration' | 'logs';
  }>(),
  {
    activeSubTab: 'summary',
  }
);

const emit = defineEmits<{
  (e: 'back'): void;
  (e: 'update:activeSubTab', tab: 'summary' | 'terminal' | 'mounts' | 'configuration' | 'logs'): void;
}>();

const isBusy = ref(false);

const cleanContainerName = computed(() => {
  return props.containerName.replace(/^lxcContainers\//, '').trim();
});

const container = computed<LxcContainer | null>(() => {
  const target = cleanContainerName.value;
  return (
    nasState.lxcContainers.find(
      (c) =>
        c.name === target ||
        c.name === `lxcContainers/${target}` ||
        c.containerName === target ||
        c.name.replace(/^lxcContainers\//, '') === target
    ) || null
  );
});

const technicalContainerId = computed(() => {
  if (container.value?.name) {
    return container.value.name.replace(/^lxcContainers\//, '').trim();
  }
  return cleanContainerName.value.toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9._-]/g, '');
});

// -----------------------------------------------------------------------------
// Power Controls
// -----------------------------------------------------------------------------
async function handleStart() {
  if (!container.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`LXC "${cleanContainerName.value}": Start`, async () => {
      await services.lxc.startContainer(container.value!.name);
    });
    notify('success', `Container "${cleanContainerName.value}" started`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Start failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleStop() {
  if (!container.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`LXC "${cleanContainerName.value}": Stop`, async () => {
      await services.lxc.stopContainer(container.value!.name);
    });
    notify('info', `Container "${cleanContainerName.value}" stopped`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Stop failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleRestart() {
  if (!container.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`LXC "${cleanContainerName.value}": Restart`, async () => {
      await services.lxc.restartContainer(container.value!.name);
    });
    notify('info', `Container "${cleanContainerName.value}" restarted`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Restart failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleFreezeToggle() {
  if (!container.value || isBusy.value) return;
  const isFrozen = container.value.state === 'FROZEN';
  isBusy.value = true;
  try {
    await runTrackedTask(
      `LXC "${cleanContainerName.value}": ${isFrozen ? 'Unfreeze' : 'Freeze'}`,
      async () => {
        if (isFrozen) {
          await services.lxc.startContainer(container.value!.name);
        } else {
          await services.lxc.freezeContainer(container.value!.name);
        }
      }
    );
    notify('info', `Container "${cleanContainerName.value}" ${isFrozen ? 'unfrozen' : 'frozen'}`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Action failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleDeleteContainer() {
  if (!container.value || isBusy.value) return;
  const confirmed = await confirmDelete({
    kind: 'LXC Container',
    name: cleanContainerName.value,
    details: 'The container root filesystem, state, and host configuration will be destroyed.',
    confirmText: 'Delete Container',
  });
  if (!confirmed.confirmed) return;

  isBusy.value = true;
  try {
    await runTrackedTask(`LXC "${cleanContainerName.value}": Delete`, async () => {
      await services.lxc.deleteContainer(container.value!.name);
    });
    notify('warning', `LXC container "${cleanContainerName.value}" deleted`);
    emit('back');
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Delete failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

// -----------------------------------------------------------------------------
// Autostart Toggle
// -----------------------------------------------------------------------------
const isTogglingAutostart = ref(false);

async function toggleAutostart() {
  if (!container.value || isTogglingAutostart.value) return;
  isTogglingAutostart.value = true;
  try {
    await services.lxc.updateContainer(container.value.name, {
      autostart: !container.value.autostart,
    });
    notify(
      'success',
      `Autostart ${!container.value.autostart ? 'enabled' : 'disabled'} for ${cleanContainerName.value}`
    );
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to toggle autostart: ${err.message}`);
  } finally {
    isTogglingAutostart.value = false;
  }
}

// -----------------------------------------------------------------------------
// Terminal & Console
// -----------------------------------------------------------------------------
const showTerminalModal = ref(false);

const terminalContainerOptions = computed<TerminalContainerOption[]>(() => {
  if (!container.value) return [];
  const id = technicalContainerId.value;
  return [
    {
      id,
      name: container.value.containerName || id,
      state: container.value.state,
    },
  ];
});

function popoutTerminal() {
  const id = technicalContainerId.value;
  const url = `/terminal?lxc=${encodeURIComponent(id)}`;
  window.open(
    url,
    `lxc_terminal_${id}`,
    'width=1000,height=650,location=no,toolbar=no,menubar=no,status=no,resizable=yes'
  );
}

// -----------------------------------------------------------------------------
// Mount Points Management
// -----------------------------------------------------------------------------
const mountsList = computed<LxcMountPoint[]>(() => {
  return container.value?.mountPoints || [];
});

const isSavingMounts = ref(false);
const showAddMountModal = ref(false);
const newMount = reactive<LxcMountPoint>({
  hostPath: '',
  containerPath: '',
  readOnly: false,
});

function openAddMountModal() {
  newMount.hostPath = '';
  newMount.containerPath = '';
  newMount.readOnly = false;
  showAddMountModal.value = true;
}

async function handleAddMount() {
  if (!container.value || !newMount.hostPath.trim() || !newMount.containerPath.trim()) return;
  const updated = [
    ...mountsList.value,
    {
      hostPath: newMount.hostPath.trim(),
      containerPath: newMount.containerPath.trim(),
      readOnly: Boolean(newMount.readOnly),
    },
  ];

  isSavingMounts.value = true;
  try {
    await runTrackedTask(`LXC "${cleanContainerName.value}": Add Mount`, async () => {
      await services.lxc.updateContainer(container.value!.name, {
        mountPoints: updated,
      });
    });
    notify('success', `Added mount point ${newMount.containerPath}`);
    showAddMountModal.value = false;
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to add mount: ${err.message}`);
  } finally {
    isSavingMounts.value = false;
  }
}

async function handleRemoveMount(index: number) {
  if (!container.value) return;
  const target = mountsList.value[index];
  if (!target) return;
  const ok = window.confirm(`Remove bind mount "${target.containerPath}"?`);
  if (!ok) return;

  const updated = mountsList.value.filter((_, idx) => idx !== index);
  isSavingMounts.value = true;
  try {
    await runTrackedTask(`LXC "${cleanContainerName.value}": Remove Mount`, async () => {
      await services.lxc.updateContainer(container.value!.name, {
        mountPoints: updated,
      });
    });
    notify('info', `Removed mount point ${target.containerPath}`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to remove mount: ${err.message}`);
  } finally {
    isSavingMounts.value = false;
  }
}

// -----------------------------------------------------------------------------
// Configuration Editor
// -----------------------------------------------------------------------------
const isSavingConfig = ref(false);
const configForm = reactive({
  containerName: '',
  description: '',
  networkBridge: '',
  autostart: false,
  unprivileged: true,
  gpuEnabled: false,
});

watch(
  container,
  (ct) => {
    if (ct) {
      configForm.containerName = ct.containerName || cleanContainerName.value;
      configForm.description = ct.description || '';
      configForm.networkBridge = ct.networkBridge || 'br0';
      configForm.autostart = Boolean(ct.autostart);
      configForm.unprivileged = Boolean(ct.unprivileged);
      configForm.gpuEnabled = Boolean(ct.gpuEnabled);
    }
  },
  { immediate: true }
);

async function handleSaveConfig() {
  if (!container.value || isSavingConfig.value) return;
  isSavingConfig.value = true;
  try {
    await runTrackedTask(`LXC "${cleanContainerName.value}": Save Configuration`, async () => {
      await services.lxc.updateContainer(container.value!.name, {
        containerName: configForm.containerName.trim(),
        description: configForm.description.trim(),
        networkBridge: configForm.networkBridge.trim() || 'br0',
        autostart: configForm.autostart,
        unprivileged: configForm.unprivileged,
        gpuEnabled: configForm.gpuEnabled,
      });
    });
    notify('success', `Saved configuration for ${cleanContainerName.value}`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to save configuration: ${err.message}`);
  } finally {
    isSavingConfig.value = false;
  }
}

// -----------------------------------------------------------------------------
// Logs Viewer
// -----------------------------------------------------------------------------
const logTail = ref(100);
const logsContent = ref('');
const isLoadingLogs = ref(false);

async function fetchLogs() {
  if (!container.value) return;
  isLoadingLogs.value = true;
  try {
    const raw = await services.lxc.getContainerLogs(container.value.name, logTail.value);
    logsContent.value = raw || 'No log entries recorded for this container.';
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
  },
  { immediate: true }
);
</script>

<template>
  <div class="lxc-detail-view">
    <!-- Subtab 1: Summary -->
    <div v-if="props.activeSubTab === 'summary'" class="lxc-tab-content">
      <div class="grid-2">
        <!-- Live Real-Time Telemetry Card -->
        <div class="card detail-card">
          <div class="card-header">
            <h3 class="card-title">Real-Time Container Telemetry</h3>
          </div>
          <div class="card-body">
            <div class="metrics-grid">
              <div class="metric-item">
                <span class="metric-label">Power State</span>
                <span
                  class="badge mono font-xs"
                  :class="
                    container?.state === 'RUNNING'
                      ? 'badge-success'
                      : container?.state === 'FROZEN'
                      ? 'badge-warning'
                      : 'badge-neutral'
                  "
                  style="align-self: flex-start"
                >
                  {{ container?.state || 'STOPPED' }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Real CPU Usage</span>
                <span class="metric-val mono">
                  {{
                    container?.state === 'RUNNING' && container?.cpuPercent !== undefined
                      ? container.cpuPercent.toFixed(1) + '%'
                      : '-'
                  }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Real Memory Used</span>
                <span class="metric-val mono">
                  {{
                    container?.state === 'RUNNING' && container?.memoryUsedMib
                      ? container.memoryUsedMib >= 1024
                        ? (container.memoryUsedMib / 1024).toFixed(2) + ' GiB'
                        : container.memoryUsedMib + ' MiB'
                      : '-'
                  }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">IPv4 Address</span>
                <span class="metric-val mono" style="color: var(--success-text)">
                  {{ container?.ipv4Address || 'None assigned / querying...' }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Distribution</span>
                <span class="metric-val">{{ container?.distribution }} / {{ container?.release }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Architecture</span>
                <span class="metric-val mono">{{ container?.architecture }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Network Bridge</span>
                <span class="metric-val mono">{{ container?.networkBridge || 'br0' }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Autostart on Boot</span>
                <button
                  type="button"
                  class="badge interactive mono font-xs"
                  :class="container?.autostart ? 'badge-success' : 'badge-neutral'"
                  style="align-self: flex-start; cursor: pointer"
                  :disabled="isTogglingAutostart"
                  @click="toggleAutostart"
                >
                  {{ container?.autostart ? 'Enabled (On Boot)' : 'Disabled (Manual)' }}
                </button>
              </div>
            </div>

            <!-- Privileges & Hardware Acceleration Flags -->
            <div style="margin-top: 18px; padding-top: 14px; border-top: 1px solid var(--border-subtle); display: flex; gap: 12px; flex-wrap: wrap">
              <span class="badge" :class="container?.unprivileged ? 'badge-info' : 'badge-warning'">
                {{ container?.unprivileged ? 'Unprivileged Namespace' : 'Privileged Container' }}
              </span>
              <span class="badge" :class="container?.gpuEnabled ? 'badge-success' : 'badge-neutral'">
                GPU Acceleration: {{ container?.gpuEnabled ? 'Enabled' : 'Disabled' }}
              </span>
            </div>
          </div>
        </div>

        <!-- Bind Mounts Overview Card -->
        <div class="card detail-card">
          <div class="card-header" style="display: flex; justify-content: space-between; align-items: center">
            <h3 class="card-title">Storage Bind Mounts</h3>
            <button class="btn btn-xs btn-secondary" @click="emit('update:activeSubTab', 'mounts')">
              Manage Mounts
            </button>
          </div>
          <div class="card-body">
            <div v-if="!mountsList || mountsList.length === 0" class="empty-state">
              No bind mounts configured for this container.
            </div>
            <table v-else class="data-table">
              <thead>
                <tr>
                  <th>Host Path</th>
                  <th>Container Mount</th>
                  <th>Mode</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(m, idx) in mountsList" :key="idx">
                  <td class="mono font-xs" style="font-weight: 600">{{ m.hostPath }}</td>
                  <td class="mono font-xs" style="color: var(--accent-primary)">{{ m.containerPath }}</td>
                  <td>
                    <span class="badge font-xs" :class="m.readOnly ? 'badge-warning' : 'badge-neutral'">
                      {{ m.readOnly ? 'RO' : 'RW' }}
                    </span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    <!-- Subtab 2: Terminal -->
    <div v-else-if="props.activeSubTab === 'terminal'" class="lxc-tab-content">
      <div v-if="container?.state !== 'RUNNING'" class="card detail-card" style="padding: 48px; text-align: center">
        <MdiIcon :path="mdiAlertCircle" size="48" style="color: var(--warning-text); margin-bottom: 12px" />
        <h4 style="font-size: 16px; font-weight: 600; margin-bottom: 6px">Container is not running</h4>
        <p class="text-muted" style="font-size: 13px; max-width: 400px; margin: 0 auto 16px">
          Start the container to establish an interactive terminal connection.
        </p>
        <button class="btn btn-success" :disabled="isBusy" @click="handleStart">
          <MdiIcon :path="mdiPlay" size="14" style="margin-right: 4px" />
          Start Container
        </button>
      </div>
      <div v-else class="card detail-card" style="padding: 0; overflow: hidden">
        <EmbeddedTerminal
          :container-id="technicalContainerId"
          :container-name="container?.containerName || technicalContainerId"
          endpoint-type="lxc"
          @popout="popoutTerminal"
        />
      </div>
    </div>

    <!-- Subtab 3: Storage Mounts -->
    <div v-else-if="props.activeSubTab === 'mounts'" class="lxc-tab-content">
      <div class="card detail-card">
        <div class="card-header" style="display: flex; justify-content: space-between; align-items: center">
          <div>
            <h3 class="card-title">Storage Bind Mounts</h3>
            <p class="card-subtitle">ZFS datasets and host directories mapped into the container</p>
          </div>
          <button class="btn btn-sm btn-primary" @click="openAddMountModal">
            <MdiIcon :path="mdiPlus" size="14" style="margin-right: 4px" />
            <span>Add Mount Point</span>
          </button>
        </div>
        <div class="card-body">
          <div v-if="mountsList.length === 0" class="empty-state">
            No bind mounts configured. Click "Add Mount Point" to map host storage.
          </div>
          <div v-else class="table-wrap">
            <table class="data-table">
              <thead>
                <tr>
                  <th>Host Path</th>
                  <th>Container Mount Path</th>
                  <th>Access Mode</th>
                  <th style="text-align: right">Actions</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(m, idx) in mountsList" :key="idx">
                  <td class="mono font-sm" style="font-weight: 600">{{ m.hostPath }}</td>
                  <td class="mono font-sm" style="color: var(--accent-primary)">{{ m.containerPath }}</td>
                  <td>
                    <span class="badge font-xs" :class="m.readOnly ? 'badge-warning' : 'badge-success'">
                      {{ m.readOnly ? 'Read Only' : 'Read / Write' }}
                    </span>
                  </td>
                  <td style="text-align: right">
                    <button
                      class="btn btn-xs btn-danger"
                      title="Remove bind mount"
                      :disabled="isSavingMounts"
                      @click="handleRemoveMount(idx)"
                    >
                      <MdiIcon :path="mdiTrashCanOutline" size="13" />
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    <!-- Subtab 4: Configuration -->
    <div v-else-if="props.activeSubTab === 'configuration'" class="lxc-tab-content">
      <div class="card detail-card" style="max-width: 720px">
        <div class="card-header">
          <h3 class="card-title">Container Settings</h3>
          <p class="card-subtitle">Manage runtime behavior, networking and security profiles</p>
        </div>
        <div class="card-body">
          <form style="display: flex; flex-direction: column; gap: 16px" @submit.prevent="handleSaveConfig">
            <div class="form-group">
              <label class="form-label">Container Name</label>
              <input v-model="configForm.containerName" type="text" class="form-input" required />
            </div>

            <div class="form-group">
              <label class="form-label">Description</label>
              <input v-model="configForm.description" type="text" class="form-input" placeholder="Optional notes or service description" />
            </div>

            <div class="form-group">
              <label class="form-label">Network Bridge</label>
              <input v-model="configForm.networkBridge" type="text" class="form-input mono" placeholder="e.g. br0" />
            </div>

            <div style="display: flex; flex-direction: column; gap: 10px; margin-top: 8px">
              <label style="display: flex; align-items: center; gap: 8px; cursor: pointer; font-size: 13px">
                <input v-model="configForm.autostart" type="checkbox" />
                <span>Autostart on host boot</span>
              </label>

              <label style="display: flex; align-items: center; gap: 8px; cursor: pointer; font-size: 13px">
                <input v-model="configForm.unprivileged" type="checkbox" />
                <span>Unprivileged Container (maps UID/GID to high unprivileged ranges)</span>
              </label>

              <label style="display: flex; align-items: center; gap: 8px; cursor: pointer; font-size: 13px">
                <input v-model="configForm.gpuEnabled" type="checkbox" />
                <span>GPU Acceleration Passthrough (/dev/dri, nvidia)</span>
              </label>
            </div>

            <div style="margin-top: 14px; padding-top: 14px; border-top: 1px solid var(--border-subtle); display: flex; justify-content: flex-end">
              <button type="submit" class="btn btn-primary" :disabled="isSavingConfig">
                {{ isSavingConfig ? 'Saving Changes...' : 'Save Configuration' }}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>

    <!-- Subtab 5: Logs -->
    <div v-else-if="props.activeSubTab === 'logs'" class="lxc-tab-content">
      <div class="card detail-card">
        <div class="card-header" style="display: flex; justify-content: space-between; align-items: center">
          <div>
            <h3 class="card-title">Container Console Logs</h3>
            <p class="card-subtitle">Recent boot, console and kernel messages for {{ cleanContainerName }}</p>
          </div>
          <div style="display: flex; align-items: center; gap: 8px">
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
      :app-title="container?.containerName || cleanContainerName"
      :containers="terminalContainerOptions"
      :endpoint-type="'lxc'"
      @close="showTerminalModal = false"
    />

    <!-- Add Mount Point Modal -->
    <div v-if="showAddMountModal" class="modal-backdrop" @click.self="showAddMountModal = false">
      <div class="modal card" style="max-width: 520px">
        <div class="modal-header">
          <h3 class="card-title">Add Storage Bind Mount</h3>
          <button class="btn btn-ghost btn-xs" @click="showAddMountModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <div class="modal-body">
          <form style="display: flex; flex-direction: column; gap: 14px" @submit.prevent="handleAddMount">
            <div class="form-group">
              <label class="form-label">Host Path</label>
              <input
                v-model="newMount.hostPath"
                type="text"
                class="form-input mono"
                placeholder="e.g. /mnt/tank/media"
                required
              />
            </div>

            <div class="form-group">
              <label class="form-label">Container Target Path</label>
              <input
                v-model="newMount.containerPath"
                type="text"
                class="form-input mono"
                placeholder="e.g. /media"
                required
              />
            </div>

            <label style="display: flex; align-items: center; gap: 8px; cursor: pointer; font-size: 13px">
              <input v-model="newMount.readOnly" type="checkbox" />
              <span>Read-Only mount</span>
            </label>

            <div class="modal-actions" style="margin-top: 12px; display: flex; justify-content: flex-end; gap: 8px">
              <button type="button" class="btn btn-secondary" @click="showAddMountModal = false">
                Cancel
              </button>
              <button type="submit" class="btn btn-primary" :disabled="isSavingMounts">
                {{ isSavingMounts ? 'Adding...' : 'Add Mount' }}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.lxc-detail-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
}

.lxc-header-card {
  padding: 14px 18px 0 18px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
}

.lxc-header-main {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 12px;
}

.lxc-title {
  font-size: 17px;
  font-weight: 700;
  color: var(--text-primary);
  margin: 0;
}

.lxc-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.lxc-subtabs {
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

.lxc-tab-content {
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

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
}

.form-input,
.form-select {
  background: var(--input-bg);
  border: 1px solid var(--border-subtle);
  color: var(--text-primary);
  border-radius: 4px;
  padding: 6px 10px;
  font-size: 13px;
  outline: none;
}

.form-input:focus,
.form-select:focus {
  border-color: var(--accent-primary);
}

.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.65);
  backdrop-filter: blur(2px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal {
  width: 90%;
  max-width: 500px;
  background: var(--bg-surface);
  border: 1px solid var(--border-default);
  border-radius: 8px;
  padding: 20px;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.font-sm {
  font-size: 12px;
}

.font-xs {
  font-size: 10.5px;
}
</style>
