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
import { type TerminalContainerOption } from '../components/DockerTerminalModal.vue';
import LogViewer from '../components/LogViewer.vue';
import { useAutoScroll } from '../composables/useAutoScroll';
import { services } from '../../container';
import { composeYamlToForm, formToComposeYaml } from '../../domain/composeModel';
import {
  type DockerApp,
} from '../../domain/models';
import {
  nasState,
  notify,
  refreshAllData,
  runTrackedTask,
} from '../store';
import { confirmModal } from '../modal';
import { getDockerAppIcon } from '../utils/appIcons';

const failedIcon = ref(false);

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

const WELL_KNOWN_WEB_PORTS = new Set([
  '80', '81', '443', '3000', '5000', '5001', '8000', '8080', '8081', '8088',
  '8096', '8123', '8384', '8443', '9000', '9443', '30020'
]);

interface FormattedPort {
  key: string;
  hostPort?: string;
  containerPort?: string;
  protocol: string;
  display: string;
  isWeb: boolean;
  webUrl?: string;
}

const formattedPorts = computed<FormattedPort[]>(() => {
  if (!app.value) return [];
  const rawList = [...(app.value.portMappings || [])];
  for (const c of app.value.containers || []) {
    for (const p of c.ports || []) {
      rawList.push(p);
    }
  }

  const result: FormattedPort[] = [];
  const seen = new Set<string>();

  for (const raw of rawList) {
    let s = String(raw || '').trim();
    if (!s) continue;

    // Handle raw stringified Go map from yaml deserialization, e.g.
    // "map[mode:ingress protocol:tcp published:30020 target:81]"
    if (s.startsWith('map[') && s.endsWith(']')) {
      const inner = s.slice(4, -1);
      const fields = inner.split(/\s+/);
      const m: Record<string, string> = {};
      for (const f of fields) {
        const idx = f.indexOf(':');
        if (idx !== -1) {
          m[f.slice(0, idx)] = f.slice(idx + 1);
        }
      }
      const pub = m['published'];
      const tgt = m['target'];
      const proto = (m['protocol'] || 'tcp').toLowerCase();
      if (pub && tgt) {
        s = `${pub}:${tgt}${proto !== 'tcp' ? '/' + proto : ''}`;
      } else if (tgt) {
        s = `${tgt}${proto !== 'tcp' ? '/' + proto : ''}`;
      } else {
        continue;
      }
    }

    let protocol = 'tcp';
    if (s.includes('/')) {
      const slash = s.lastIndexOf('/');
      protocol = s.slice(slash + 1).toLowerCase() || 'tcp';
      s = s.slice(0, slash);
    }

    // Strip 0.0.0.0: or [::]: or ::: prefix
    s = s.replace(/^(0\.0\.0\.0|\[::\]|:::):/, '');

    let hostPort = '';
    let containerPort = '';

    if (s.includes('->')) {
      const parts = s.split('->');
      const hp = parts[0].replace(/.*:/, '').trim();
      const cp = parts[1].trim();
      hostPort = hp;
      containerPort = cp;
    } else if (s.includes(':')) {
      const parts = s.split(':');
      if (parts.length === 2) {
        hostPort = parts[0].trim();
        containerPort = parts[1].trim();
      } else if (parts.length >= 3) {
        hostPort = parts[1].trim();
        containerPort = parts[2].trim();
      }
    } else {
      containerPort = s.trim();
    }

    if (!hostPort && !containerPort) continue;

    const dedupeKey = hostPort ? `${hostPort}:${containerPort}/${protocol}` : `:${containerPort}/${protocol}`;
    if (seen.has(dedupeKey)) continue;
    seen.add(dedupeKey);

    let display = '';
    if (hostPort && containerPort) {
      display = `${hostPort}:${containerPort}`;
    } else if (hostPort) {
      display = hostPort;
    } else {
      display = `:${containerPort}`;
    }

    if (protocol !== 'tcp') {
      display += `/${protocol.toUpperCase()}`;
    }

    const host = typeof window !== 'undefined' ? window.location.hostname : 'localhost';
    const isWeb = protocol === 'tcp' && Boolean(
      (hostPort && WELL_KNOWN_WEB_PORTS.has(hostPort)) ||
      (containerPort && WELL_KNOWN_WEB_PORTS.has(containerPort)) ||
      (hostPort && app.value.webUiUrl?.includes(':' + hostPort))
    );

    let webUrl = '';
    if (isWeb && hostPort) {
      const scheme = (hostPort === '443' || hostPort === '8443' || hostPort === '9443') ? 'https' : 'http';
      webUrl = `${scheme}://${host}:${hostPort}`;
    }

    result.push({
      key: dedupeKey,
      hostPort,
      containerPort,
      protocol,
      display,
      isWeb,
      webUrl,
    });
  }

  // Deduplicate bare container ports if a host-mapped port for the same container port exists
  const mappedContainerPorts = new Set(result.filter(r => r.hostPort).map(r => `${r.containerPort}/${r.protocol}`));
  return result.filter(r => {
    if (!r.hostPort && mappedContainerPorts.has(`${r.containerPort}/${r.protocol}`)) {
      return false;
    }
    return true;
  });
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

// -----------------------------------------------------------------------------
// Upgrade App
// -----------------------------------------------------------------------------
const isUpgrading = ref(false);

async function handleUpgradeApp() {
  if (!app.value || isUpgrading.value) return;
  const ok = await confirmModal({
    title: 'Upgrade Application',
    message: `Upgrade application "${app.value.title || cleanAppName.value}"?`,
    details: 'This will pull the latest container images and recreate containers.',
    confirmText: 'Upgrade Stack',
    tone: 'primary',
  });
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
// Console / Terminal
// -----------------------------------------------------------------------------
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
  <div class="docker-detail-view" :class="{ 'is-console-tab': props.activeSubTab === 'console' }">
    <!-- Subtab 1: Summary -->
    <div v-if="props.activeSubTab === 'summary'" class="docker-tab-content">
      <div class="grid-2">
        <!-- Live Real-Time Telemetry Card -->
        <div class="card detail-card">
          <div class="card-header" style="display: flex; align-items: center; gap: 12px">
            <div
              v-if="app && getDockerAppIcon(app) && !failedIcon"
              class="app-detail-header-icon-box"
            >
              <img
                :src="getDockerAppIcon(app)"
                :alt="app.title || cleanAppName"
                class="app-detail-header-img"
                @error="failedIcon = true"
              />
            </div>
            <div v-else class="app-detail-header-icon-box">
              <MdiIcon :path="mdiDocker" size="24" style="color: var(--accent-primary, #38bdf8)" />
            </div>
            <div>
              <h3 class="card-title">{{ app?.title || cleanAppName }}</h3>
              <span class="font-xs" style="color: var(--text-secondary)">Application Status & Telemetry</span>
            </div>
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
              <div v-if="formattedPorts.length === 0" class="text-muted font-xs">
                No external host ports exposed.
              </div>
              <div v-else style="display: flex; flex-wrap: wrap; gap: 8px">
                <template v-for="p in formattedPorts" :key="p.key">
                  <a
                    v-if="p.webUrl"
                    :href="p.webUrl"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="badge badge-info interactive mono font-xs"
                    style="display: inline-flex; align-items: center; gap: 5px; text-decoration: none"
                    :title="`Open Web UI at ${p.webUrl}`"
                  >
                    <span>{{ p.display }}</span>
                    <MdiIcon :path="mdiOpenInNew" size="12" style="opacity: 0.85" />
                  </a>
                  <span
                    v-else
                    class="badge badge-neutral mono font-xs"
                    :title="p.hostPort ? `Host port ${p.hostPort} mapped to container port ${p.containerPort}` : `Container port ${p.containerPort}`"
                  >
                    {{ p.display }}
                  </span>
                </template>
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
    <div v-else-if="props.activeSubTab === 'console'" class="docker-tab-content docker-console-tab-content">
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
      <div v-else class="card detail-card docker-terminal-card" style="padding: 0; overflow: hidden">
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
        <!-- Catalog Update Available Banner -->
        <div
          v-if="app?.updateAvailable"
          style="background: var(--warning-bg, rgba(234, 179, 8, 0.1)); border: 1px solid var(--warning-border, rgba(234, 179, 8, 0.3)); border-radius: 6px; padding: 10px 14px; margin-bottom: 14px; display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap"
        >
          <div style="font-size: 12.5px; color: var(--text-primary)">
            <strong>Catalog Update Available:</strong>
            <span v-if="app.latestVersion" style="margin-left: 4px">Version {{ app.latestVersion }}</span>
            <span v-if="app.latestImage" style="color: var(--text-secondary); margin-left: 8px">
              Recommended Image: <code class="mono" style="font-size: 11px">{{ app.latestImage }}</code>
            </span>
          </div>
          <button
            type="button"
            class="btn btn-sm btn-primary"
            style="font-size: 11.5px; padding: 3px 8px; flex-shrink: 0"
            :disabled="isUpgrading"
            @click="handleUpgradeApp"
          >
            {{ isUpgrading ? 'Upgrading...' : 'Upgrade Stack' }}
          </button>
        </div>

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
        <div class="card-header" style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px">
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
          </div>
        </div>
        <div class="card-body" style="padding: 12px">
          <LogViewer
            :logs="logsContent"
            :loading="isLoadingLogs"
            max-height="620px"
            empty-text="No log entries recorded for this application."
            @refresh="fetchLogs(false)"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.docker-detail-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
}

.docker-detail-view.is-console-tab {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.docker-console-tab-content {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.docker-terminal-card {
  flex: 1;
  min-height: max(520px, calc(100vh - 160px));
  display: flex;
  flex-direction: column;
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
  font-size: 13px;
}

.app-detail-header-icon-box {
  width: 42px;
  height: 42px;
  border-radius: 8px;
  background: var(--bg-surface-elevated, rgba(255, 255, 255, 0.05));
  border: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  overflow: hidden;
}

.app-detail-header-img {
  width: 32px;
  height: 32px;
  object-fit: contain;
}

.font-xs {
  font-size: 11.5px;
}
</style>
