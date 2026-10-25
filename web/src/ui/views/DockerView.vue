<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import {
  mdiAlertCircleOutline,
  mdiCheck,
  mdiClose,
  mdiCloudSync,
  mdiContentCopy,
  mdiDelete,
  mdiDocker,
  mdiDownload,
  mdiEye,
  mdiFileDocumentOutline,
  mdiFolderOutline,
  mdiHarddisk,
  mdiLayers,
  mdiMagnify,
  mdiOpenInNew,
  mdiPencil,
  mdiPlay,
  mdiPlus,
  mdiRefresh,
  mdiRestart,
  mdiStop,
  mdiStorefrontOutline,
} from '@mdi/js';
import { services } from '../../container';
import { generateNanosecondId } from '../../domain/id';
import {
  APP_CATALOG_TEMPLATES,
  type AppCatalogTemplate,
  type CatalogApp,
  type DockerApp,
} from '../../domain/models';
import ComposeFormEditor from '../components/ComposeFormEditor.vue';
import CodeMirrorEditor from '../components/CodeMirrorEditor.vue';
import MdiIcon from '../components/MdiIcon.vue';
import { askModal, nasState, notify } from '../store';

// =============================================================================
// Filters & Search
// =============================================================================
const searchQuery = ref('');
const stateFilter = ref<'ALL' | 'RUNNING' | 'STOPPED' | 'PARTIALLY_RUNNING'>('ALL');
const isRefreshing = ref(false);

const runningCount = computed(
  () => nasState.dockerApps.filter((a) => a.state === 'RUNNING').length
);
const stoppedCount = computed(
  () => nasState.dockerApps.filter((a) => a.state === 'STOPPED').length
);
const partialCount = computed(
  () => nasState.dockerApps.filter((a) => a.state === 'PARTIALLY_RUNNING').length
);

const totalContainersCount = computed(() =>
  nasState.dockerApps.reduce((acc, a) => acc + (a.containers?.length || 0), 0)
);

const totalMemoryUsageMib = computed(() =>
  nasState.dockerApps.reduce((acc, a) => acc + (a.totalMemoryMib || 0), 0)
);

const filteredApps = computed(() => {
  return nasState.dockerApps.filter((a) => {
    if (stateFilter.value !== 'ALL' && a.state !== stateFilter.value) {
      return false;
    }
    if (!searchQuery.value.trim()) return true;
    const q = searchQuery.value.toLowerCase();
    const matchesId = a.appId.toLowerCase().includes(q);
    const matchesTitle = a.title.toLowerCase().includes(q);
    const matchesService = (a.services || []).some((s) => s.toLowerCase().includes(q));
    const matchesPort = (a.portMappings || []).some((p) => p.toLowerCase().includes(q));
    const matchesContainer = (a.containers || []).some(
      (c) => c.name.toLowerCase().includes(q) || c.image.toLowerCase().includes(q)
    );
    return matchesId || matchesTitle || matchesService || matchesPort || matchesContainer;
  });
});

// =============================================================================
// Realtime Telemetry & Network Rate Formatter
// =============================================================================
function formatNetworkRate(app: DockerApp): string {
  if (app.state !== 'RUNNING') return '—';
  const rx = app.totalNetRxRate || 0;
  const tx = app.totalNetTxRate || 0;
  if (rx + tx === 0) return '0 B/s';
  const rxStr = formatBytes(rx) + '/s ↓';
  const txStr = formatBytes(tx) + '/s ↑';
  return `${rxStr} · ${txStr}`;
}

function formatNetworkTotal(app: DockerApp): string {
  if (app.state !== 'RUNNING') return '';
  const rx = app.totalNetRxBytes || 0;
  const tx = app.totalNetTxBytes || 0;
  if (rx + tx === 0) return '';
  return `(${formatBytes(rx + tx)})`;
}

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

function displayPorts(app: DockerApp): string[] {
  const seen = new Set<string>();
  const res: string[] = [];
  for (const raw of app.portMappings || []) {
    let p = String(raw).trim();
    if (p.toLowerCase().endsWith('/tcp')) {
      p = p.slice(0, -4);
    }
    if (p.startsWith('0.0.0.0:')) {
      p = p.slice(8);
    }
    if (p && !seen.has(p)) {
      seen.add(p);
      res.push(p);
    }
  }
  return res;
}

function formatContainerPorts(ports?: string[]): string {
  if (!ports || ports.length === 0) return '—';
  const seen = new Set<string>();
  const res: string[] = [];
  for (const raw of ports) {
    let p = String(raw).trim();
    if (p.toLowerCase().endsWith('/tcp')) {
      p = p.slice(0, -4);
    }
    if (p.startsWith('0.0.0.0:')) {
      p = p.slice(8);
    }
    if (p && !seen.has(p)) {
      seen.add(p);
      res.push(p);
    }
  }
  return res.join(', ') || '—';
}

let isUpdatingTelemetry = false;

async function updateTelemetry() {
  if (isUpdatingTelemetry) return;
  isUpdatingTelemetry = true;
  try {
    const apps = await services.apps.listApps();
    nasState.dockerApps = apps;
  } catch {
    // background telemetry
  } finally {
    isUpdatingTelemetry = false;
  }
}

async function refreshApps() {
  isRefreshing.value = true;
  try {
    await Promise.all([updateTelemetry(), fetchDockerConfig()]);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to refresh Docker Apps');
  } finally {
    isRefreshing.value = false;
  }
}

let telemetryTimer: any = null;
let isViewActive = false;

async function runTelemetryLoop() {
  if (!isViewActive) return;
  await updateTelemetry();
  if (isViewActive) {
    telemetryTimer = setTimeout(runTelemetryLoop, 1000);
  }
}

// =============================================================================
// TrueNAS Application Catalog
// =============================================================================
const isCatalogModalOpen = ref(false);
const catalogApps = ref<CatalogApp[]>([]);
const isLoadingCatalog = ref(false);
const isSyncingCatalog = ref(false);
const catalogSearchQuery = ref('');
const catalogSelectedTrain = ref<'ALL' | 'stable' | 'community'>('ALL');
const catalogSelectedCategory = ref<string>('ALL');
const catalogLastSyncTime = ref<string>('');
const catalogTotalCount = ref<number>(0);
const failedIcons = ref<Record<string, boolean>>({});
const importingAppId = ref<string | null>(null);

const catalogCategories = computed(() => {
  const set = new Set<string>();
  for (const app of catalogApps.value) {
    if (app.category) set.add(app.category);
    if (app.categories) {
      for (const cat of app.categories) {
        if (cat) set.add(cat);
      }
    }
  }
  return Array.from(set).sort((a, b) => a.localeCompare(b));
});

const filteredCatalogApps = computed(() => {
  return catalogApps.value.filter((app) => {
    if (catalogSelectedTrain.value !== 'ALL') {
      if (app.train.toLowerCase() !== catalogSelectedTrain.value.toLowerCase()) {
        return false;
      }
    }
    if (catalogSelectedCategory.value !== 'ALL') {
      const matchCat =
        app.category.toLowerCase() === catalogSelectedCategory.value.toLowerCase() ||
        (app.categories || []).some(
          (c) => c.toLowerCase() === catalogSelectedCategory.value.toLowerCase()
        );
      if (!matchCat) return false;
    }
    if (!catalogSearchQuery.value.trim()) return true;
    const q = catalogSearchQuery.value.toLowerCase();
    const matchId = app.appId.toLowerCase().includes(q);
    const matchTitle = app.title.toLowerCase().includes(q);
    const matchDesc = (app.description || '').toLowerCase().includes(q);
    const matchTags = (app.tags || []).some((t) => t.toLowerCase().includes(q));
    return matchId || matchTitle || matchDesc || matchTags;
  });
});

function formatSyncTime(iso?: string): string {
  if (!iso) return '';
  try {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toLocaleString();
  } catch {
    return iso;
  }
}

async function loadCatalog() {
  isLoadingCatalog.value = true;
  try {
    const res = await services.catalog.listApps();
    catalogApps.value = res.apps;
    catalogTotalCount.value = res.totalSize;
    if (res.lastSyncTime) {
      catalogLastSyncTime.value = res.lastSyncTime;
    }
  } catch (err: any) {
    notify('error', err?.message || 'Failed to load Application Catalog');
  } finally {
    isLoadingCatalog.value = false;
  }
}

async function syncCatalog() {
  isSyncingCatalog.value = true;
  try {
    notify('info', 'Synchronizing TrueNAS SCALE Application Catalog from upstream...');
    const res = await services.catalog.syncCatalog();
    catalogLastSyncTime.value = res.lastSyncTime;
    notify(
      'success',
      res.message || `Catalog synchronized: ${res.totalApps} applications available.`
    );
    await loadCatalog();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to sync catalog');
  } finally {
    isSyncingCatalog.value = false;
  }
}

function openCatalogModal() {
  isCatalogModalOpen.value = true;
  if (catalogApps.value.length === 0) {
    loadCatalog();
  }
}

async function importFromCatalog(app: CatalogApp) {
  importingAppId.value = app.appId;
  try {
    const detailed = await services.catalog.getApp(app.appId, app.train);
    installAppId.value = detailed.appId;
    installTitle.value = detailed.title || detailed.appId;
    installComposeYaml.value =
      detailed.composeYaml ||
      `services:\n  ${detailed.appId}:\n    image: ${detailed.appId}:latest\n`;
    activeInstallTemplate.value = null;
    installEditorKey.value++;
    isCatalogModalOpen.value = false;
    isInstallModalOpen.value = true;
    notify('success', `Imported "${detailed.title}" from TrueNAS Catalog into Compose Editor`);
  } catch (err: any) {
    notify('error', err?.message || `Failed to load template for ${app.title}`);
  } finally {
    importingAppId.value = null;
  }
}

// =============================================================================
// Docker Owned ZFS Storage Dataset Configuration
// =============================================================================
const isDatasetModalOpen = ref(false);
const selectedDatasetToSave = ref('tank/apps');
const isSavingDataset = ref(false);

const availableDatasetNames = computed(() => {
  const mainPool = nasState.systemConfig?.mainPool || (nasState.dockerConfig?.availablePools?.[0] || 'tank');
  if (nasState.dockerConfig?.availableDatasets && nasState.dockerConfig.availableDatasets.length > 0) {
    return nasState.dockerConfig.availableDatasets.filter(d => d.startsWith(`${mainPool}/`) || d === mainPool);
  }
  const list: string[] = [];
  const poolDatasets = nasState.datasetsByPool[mainPool];
  if (poolDatasets && poolDatasets.length > 0) {
    for (const d of poolDatasets) {
      const dName = d.name.replace(/^zfsPools\/[^/]+\/datasets\//, '');
      list.push(`${mainPool}/${dName}`);
    }
  } else {
    list.push(`${mainPool}/apps`);
  }
  return list.length > 0 ? list : [`${mainPool}/apps`];
});

async function fetchDockerConfig() {
  try {
    const cfg = await services.apps.getDockerConfig();
    nasState.dockerConfig = cfg;
    if (cfg.dataset) {
      selectedDatasetToSave.value = cfg.dataset;
    } else if (cfg.pool) {
      selectedDatasetToSave.value = `${cfg.pool}/apps`;
    }
  } catch {
    // fallback
  }
}

function openDatasetModal() {
  selectedDatasetToSave.value = nasState.dockerConfig?.dataset || 'tank/apps';
  isDatasetModalOpen.value = true;
}

async function handleSaveDataset() {
  if (!selectedDatasetToSave.value) return;
  isSavingDataset.value = true;
  try {
    const updated = await services.apps.updateDockerConfig(selectedDatasetToSave.value);
    nasState.dockerConfig = updated;
    notify('success', `Docker storage configured on ZFS dataset "${updated.dataset}"`);
    isDatasetModalOpen.value = false;
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update Docker storage dataset');
  } finally {
    isSavingDataset.value = false;
  }
}

// =============================================================================
// Launch App Modal
// =============================================================================
const isInstallModalOpen = ref(false);
const activeInstallTemplate = ref<AppCatalogTemplate | null>(APP_CATALOG_TEMPLATES[0]);
const installAppId = ref('nextcloud');
const installTitle = ref('Nextcloud Hub');
const installComposeYaml = ref(APP_CATALOG_TEMPLATES[0].composeYaml);
const installEditorKey = ref(1);

function openInstallModal() {
  selectCatalogTemplate(APP_CATALOG_TEMPLATES[0]);
  isInstallModalOpen.value = true;
}

function selectCatalogTemplate(tmpl: AppCatalogTemplate | null) {
  activeInstallTemplate.value = tmpl;
  if (tmpl) {
    installAppId.value = tmpl.id;
    installTitle.value = tmpl.name;
    installComposeYaml.value = tmpl.composeYaml;
  } else {
    const nanoId = generateNanosecondId();
    installAppId.value = nanoId;
    installTitle.value = `custom-app-${nanoId}`;
    installComposeYaml.value = `services:
  web:
    image: nginx:alpine
    restart: unless-stopped
    ports:
      - "8080:80"
`;
  }
  installEditorKey.value++;
}

async function handleDeployApp(data: { appId: string; title: string; composeYaml: string }) {
  try {
    const targetId = data.appId.trim() || generateNanosecondId();
    await services.apps.createApp(targetId, data.composeYaml, data.title || targetId);
    notify('success', `App "${targetId}" deployed successfully`);
    isInstallModalOpen.value = false;
    await refreshApps();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to deploy Docker App');
  }
}

// =============================================================================
// Edit App Modal
// =============================================================================
const isEditModalOpen = ref(false);
const editingApp = ref<DockerApp | null>(null);

function openEditModal(app: DockerApp) {
  editingApp.value = app;
  isEditModalOpen.value = true;
}

async function handleSaveAppEdit(data: { appId: string; title: string; composeYaml: string }) {
  if (!editingApp.value) return;
  try {
    await services.apps.updateApp(editingApp.value.name, data.composeYaml, data.title);
    notify('success', `App "${data.appId}" updated`);
    isEditModalOpen.value = false;
    await refreshApps();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update Docker App');
  }
}

// =============================================================================
// App Inspection & Logs Modal
// =============================================================================
const isInspectModalOpen = ref(false);
const inspectedApp = ref<DockerApp | null>(null);
const inspectActiveTab = ref<'containers' | 'logs' | 'compose'>('containers');
const logTail = ref(100);
const selectedService = ref<string>('');
const logsContent = ref('');
const isLoadingLogs = ref(false);

function openInspectModal(app: DockerApp, defaultTab: 'containers' | 'logs' | 'compose' = 'containers') {
  inspectedApp.value = app;
  inspectActiveTab.value = defaultTab;
  selectedService.value = '';
  isInspectModalOpen.value = true;
  if (defaultTab === 'logs') {
    fetchLogs();
  }
}

async function fetchLogs() {
  if (!inspectedApp.value) return;
  isLoadingLogs.value = true;
  try {
    const res = await services.apps.getAppLogs(
      inspectedApp.value.name,
      logTail.value,
      selectedService.value || undefined
    );
    logsContent.value = res.logs || 'No log entries found for this application.';
  } catch (err: any) {
    logsContent.value = `Failed to fetch logs: ${err?.message || 'Unknown error'}`;
  } finally {
    isLoadingLogs.value = false;
  }
}

function copyLogs() {
  navigator.clipboard.writeText(logsContent.value);
  notify('info', 'Logs copied to clipboard');
}

function copyCompose(yaml: string) {
  navigator.clipboard.writeText(yaml);
  notify('info', 'Compose YAML copied to clipboard');
}

// =============================================================================
// App Lifecycle Actions
// =============================================================================
async function startApp(app: DockerApp) {
  try {
    notify('info', `Starting app "${app.appId}"...`);
    await services.apps.startApp(app.name);
    notify('success', `App "${app.appId}" started`);
    await refreshApps();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to start App');
  }
}

async function stopApp(app: DockerApp) {
  try {
    notify('info', `Stopping app "${app.appId}"...`);
    await services.apps.stopApp(app.name);
    notify('success', `App "${app.appId}" stopped`);
    await refreshApps();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to stop App');
  }
}

async function restartApp(app: DockerApp) {
  try {
    notify('info', `Restarting app "${app.appId}"...`);
    await services.apps.restartApp(app.name);
    notify('success', `App "${app.appId}" restarted`);
    await refreshApps();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to restart App');
  }
}

async function deleteApp(app: DockerApp) {
  const res = await askModal({
    title: 'Delete Docker Application',
    tone: 'danger',
    message: `Are you sure you want to delete App "${app.title || app.appId}"?`,
    details: 'This will stop and remove all associated containers and compose project configurations.',
    confirmText: 'Delete App',
    checkbox: {
      label: 'Purge persistent data volumes on disk (`docker compose down -v`)',
      checked: false,
      hint: 'If unchecked, your application data and config volumes will remain safely preserved on disk.',
    },
  });
  if (!res.confirmed) return;

  const removeVols = !!res.checkboxChecked;

  try {
    notify('info', `Tearing down app "${app.appId}"...`);
    await services.apps.deleteApp(app.name, removeVols);
    notify('success', `App "${app.appId}" deleted`);
    await refreshApps();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete App');
  }
}

function openWebPortal(url: string) {
  if (!url) return;
  let targetUrl = url;
  if (targetUrl.includes('localhost')) {
    targetUrl = targetUrl.replace('localhost', window.location.hostname);
  }
  window.open(targetUrl, '_blank', 'noopener,noreferrer');
}

onMounted(() => {
  isViewActive = true;
  fetchDockerConfig();
  if (nasState.dockerApps.length === 0) {
    refreshApps();
  }
  runTelemetryLoop();
});

onUnmounted(() => {
  isViewActive = false;
  if (telemetryTimer) {
    clearTimeout(telemetryTimer);
    telemetryTimer = null;
  }
});
</script>

<template>
  <div class="apps-container">
    <!-- Header & Action Bar -->
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px">
      <div>
        <h2 style="font-size: 24px; font-weight: 700; color: var(--text-primary)">
          Apps
        </h2>
        <p style="color: var(--text-secondary); font-size: 13.5px; margin-top: 3px">
          Manage containerized applications and services.
        </p>
      </div>

      <div style="display: flex; gap: 10px; align-items: center">
        <button class="btn btn-sm" :disabled="isRefreshing" @click="refreshApps">
          <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" />
          {{ isRefreshing ? 'Refreshing...' : 'Refresh' }}
        </button>

        <button
          class="btn btn-sm"
          style="display: flex; align-items: center; gap: 6px"
          :class="(nasState.dockerConfig?.datasetSelected ?? nasState.dockerConfig?.poolSelected) ? '' : 'btn-warning'"
          title="Configure Docker owned ZFS storage dataset for images and volumes"
          @click="openDatasetModal"
        >
          <MdiIcon :path="mdiFolderOutline" size="16" style="color: var(--accent-primary)" />
          <span>Dataset: {{ nasState.dockerConfig?.dataset || nasState.dockerConfig?.pool || 'None' }}</span>
        </button>

        <button class="btn btn-sm" style="display: flex; align-items: center; gap: 6px" @click="openCatalogModal">
          <MdiIcon :path="mdiStorefrontOutline" size="16" style="color: var(--accent-primary)" />
          TrueNAS Catalog
        </button>

        <button class="btn btn-primary" @click="openInstallModal">
          <MdiIcon :path="mdiPlus" size="16" style="margin-right: 4px" />
          Launch App
        </button>
      </div>
    </div>

    <!-- Apps Storage Dataset Warning Banner if unconfigured and no Main Pool exists -->
    <div
      v-if="!nasState.systemConfig?.mainPool && nasState.dockerConfig && !(nasState.dockerConfig.datasetSelected ?? nasState.dockerConfig.poolSelected)"
      class="card"
      style="
        background: rgba(245, 158, 11, 0.1);
        border: 1px solid rgba(245, 158, 11, 0.4);
        padding: 14px 18px;
        margin-bottom: 20px;
        display: flex;
        justify-content: space-between;
        align-items: center;
        border-radius: 8px;
      "
    >
      <div style="display: flex; align-items: center; gap: 12px">
        <MdiIcon :path="mdiAlertCircleOutline" size="24" style="color: #f59e0b; flex-shrink: 0" />
        <div>
          <div style="font-weight: 700; font-size: 14px; color: var(--text-primary)">
            Docker Storage Dataset Not Configured
          </div>
          <div style="font-size: 12.5px; color: var(--text-secondary); margin-top: 2px">
            Select or create a ZFS dataset (e.g. {{ nasState.dockerConfig.dataset || 'tank/apps' }}) to store container images (/{{ nasState.dockerConfig.dataset || 'tank/apps' }}/docker) and persistent app volumes (/{{ nasState.dockerConfig.dataset || 'tank/apps' }}/appdata).
          </div>
        </div>
      </div>
      <button class="btn btn-sm btn-primary" @click="openDatasetModal">
        Configure Storage Dataset
      </button>
    </div>

    <!-- Summary Metrics Cards -->
    <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 14px; margin-bottom: 22px">
      <div class="card" style="padding: 14px">
        <div style="font-size: 12px; color: var(--text-secondary); text-transform: uppercase; font-weight: 600">
          Installed Apps
        </div>
        <div style="font-size: 26px; font-weight: 700; color: var(--text-primary); margin-top: 4px">
          {{ nasState.dockerApps.length }}
        </div>
        <div style="font-size: 12px; color: var(--text-secondary); margin-top: 4px">
          {{ totalContainersCount }} total containers
        </div>
      </div>

      <div class="card" style="padding: 14px">
        <div style="font-size: 12px; color: var(--text-secondary); text-transform: uppercase; font-weight: 600">
          Running Apps
        </div>
        <div style="font-size: 26px; font-weight: 700; color: #10b981; margin-top: 4px">
          {{ runningCount }}
        </div>
        <div style="font-size: 12px; color: var(--text-secondary); margin-top: 4px">
          {{ stoppedCount }} stopped · {{ partialCount }} partial
        </div>
      </div>

      <div class="card" style="padding: 14px">
        <div style="font-size: 12px; color: var(--text-secondary); text-transform: uppercase; font-weight: 600">
          App Memory Usage
        </div>
        <div style="font-size: 26px; font-weight: 700; color: #3b82f6; margin-top: 4px">
          {{ totalMemoryUsageMib }} MiB
        </div>
        <div style="font-size: 12px; color: var(--text-secondary); margin-top: 4px">
          Aggregated across stacks
        </div>
      </div>
    </div>

    <!-- Filter & Search Controls -->
    <div style="display: flex; gap: 12px; align-items: center; justify-content: space-between; margin-bottom: 20px; flex-wrap: wrap">
      <!-- Search Input -->
      <div style="position: relative; flex: 1; min-width: 260px">
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search by app name, service, image, or port..."
          style="
            width: 100%;
            background: var(--bg-card);
            border: 1px solid var(--border-subtle);
            color: var(--text-primary);
            padding: 8px 12px 8px 34px;
            border-radius: 6px;
            font-size: 13.5px;
          "
        />
        <div style="position: absolute; left: 10px; top: 9px; color: var(--text-secondary); pointer-events: none">
          <MdiIcon :path="mdiMagnify" size="16" />
        </div>
      </div>

      <!-- State Filter Tabs -->
      <div style="display: flex; gap: 6px">
        <button
          class="btn btn-sm"
          :class="stateFilter === 'ALL' ? 'btn-primary' : ''"
          style="padding: 5px 12px"
          @click="stateFilter = 'ALL'"
        >
          All ({{ nasState.dockerApps.length }})
        </button>
        <button
          class="btn btn-sm"
          :class="stateFilter === 'RUNNING' ? 'btn-primary' : ''"
          style="padding: 5px 12px"
          @click="stateFilter = 'RUNNING'"
        >
          Running ({{ runningCount }})
        </button>
        <button
          class="btn btn-sm"
          :class="stateFilter === 'STOPPED' ? 'btn-primary' : ''"
          style="padding: 5px 12px"
          @click="stateFilter = 'STOPPED'"
        >
          Stopped ({{ stoppedCount }})
        </button>
        <button
          class="btn btn-sm"
          :class="stateFilter === 'PARTIALLY_RUNNING' ? 'btn-primary' : ''"
          style="padding: 5px 12px"
          @click="stateFilter = 'PARTIALLY_RUNNING'"
        >
          Partial ({{ partialCount }})
        </button>
      </div>
    </div>

    <!-- Empty State -->
    <div
      v-if="filteredApps.length === 0"
      class="card"
      style="text-align: center; padding: 48px 24px"
    >
      <div style="margin-bottom: 12px; color: var(--text-muted)">
        <MdiIcon :path="mdiLayers" size="44" />
      </div>
      <h3 style="font-size: 18px; font-weight: 700; color: var(--text-primary); margin-bottom: 6px">
        No Applications Found
      </h3>
      <p style="color: var(--text-secondary); font-size: 13.5px; max-width: 480px; margin: 0 auto 18px auto">
        Deploy your first application stack or select a template from the TrueNAS catalog.
      </p>
      <div style="display: flex; gap: 10px; justify-content: center">
        <button class="btn btn-primary" @click="openInstallModal">
          <MdiIcon :path="mdiPlus" size="16" style="margin-right: 4px" />
          Launch App
        </button>
        <button class="btn" @click="openCatalogModal">
          <MdiIcon :path="mdiStorefrontOutline" size="16" style="margin-right: 4px; color: var(--accent-primary)" />
          Browse TrueNAS Catalog
        </button>
      </div>
    </div>

    <!-- Applications Table View -->
    <div v-else class="card" style="padding: 0; overflow-x: auto">
      <table class="table" style="width: 100%; border-collapse: collapse">
        <thead>
          <tr style="border-bottom: 1px solid var(--border-subtle); text-align: left; font-size: 11.5px; color: var(--text-secondary); text-transform: uppercase">
            <th style="padding: 12px 16px">Application</th>
            <th style="padding: 12px 16px">Status</th>
            <th style="padding: 12px 16px">Services</th>
            <th style="padding: 12px 16px">Ports</th>
            <th style="padding: 12px 16px">Web UI</th>
            <th style="padding: 12px 16px">CPU</th>
            <th style="padding: 12px 16px">Memory</th>
            <th style="padding: 12px 16px">Network (1s)</th>
            <th style="padding: 12px 16px; text-align: right">Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="app in filteredApps"
            :key="app.name"
            style="border-bottom: 1px solid var(--border-subtle)"
          >
            <!-- Application -->
            <td style="padding: 12px 16px">
              <div style="display: flex; align-items: center; gap: 10px">
                <div style="width: 34px; height: 34px; border-radius: 6px; background: rgba(59, 130, 246, 0.12); display: flex; align-items: center; justify-content: center; color: #3b82f6; flex-shrink: 0">
                  <MdiIcon :path="mdiDocker" size="20" />
                </div>
                <div>
                  <div style="font-weight: 600; color: var(--text-primary)">
                    {{ app.title || app.appId }}
                  </div>
                  <div class="mono" style="font-size: 11.5px; color: var(--text-secondary)">
                    {{ app.appId }} · {{ app.services.length }} service{{ app.services.length === 1 ? '' : 's' }}
                  </div>
                </div>
              </div>
            </td>

            <!-- State -->
            <td style="padding: 12px 16px">
              <span
                class="badge"
                :class="{
                  'badge-success': app.state === 'RUNNING',
                  'badge-neutral': app.state === 'STOPPED',
                  'badge-warning': app.state === 'PARTIALLY_RUNNING',
                  'badge-danger': app.state === 'ERROR'
                }"
              >
                {{ app.state }}
              </span>
              <div style="font-size: 11px; color: var(--text-secondary); margin-top: 3px">
                {{ app.statusText }}
              </div>
            </td>

            <!-- Containers / Services -->
            <td style="padding: 12px 16px">
              <div style="display: flex; flex-wrap: wrap; gap: 4px; max-width: 220px">
                <span
                  v-for="c in app.containers"
                  :key="c.name"
                  class="badge"
                  :class="c.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'"
                  style="font-size: 10px; font-family: monospace"
                  :title="`${c.name} (${c.image})`"
                >
                  {{ c.serviceName }}
                </span>
              </div>
            </td>

            <!-- Ports -->
            <td style="padding: 12px 16px">
              <div v-if="displayPorts(app).length > 0" class="mono" style="font-size: 11.5px" :title="displayPorts(app).join(', ')">
                <div v-for="p in displayPorts(app).slice(0, 2)" :key="p">{{ p }}</div>
                <div v-if="displayPorts(app).length > 2" style="color: var(--text-secondary); font-size: 10.5px">
                  +{{ displayPorts(app).length - 2 }} more
                </div>
              </div>
              <span v-else style="color: var(--text-secondary); font-size: 12px">—</span>
            </td>

            <!-- Web Portal -->
            <td style="padding: 12px 16px">
              <button
                v-if="app.webUiUrl"
                class="btn btn-sm"
                style="padding: 3px 8px; font-size: 11.5px; color: #10b981; border-color: rgba(16, 185, 129, 0.3); display: inline-flex; align-items: center; gap: 4px"
                @click="openWebPortal(app.webUiUrl)"
              >
                <MdiIcon :path="mdiOpenInNew" size="13" />
                Open UI
              </button>
              <span v-else style="color: var(--text-secondary); font-size: 12px">—</span>
            </td>

            <!-- CPU -->
            <td style="padding: 12px 16px">
              <div class="mono" style="font-size: 12.5px; font-weight: 600; color: var(--text-primary)">
                {{ app.totalCpuPercent.toFixed(1) }}%
              </div>
            </td>

            <!-- Memory -->
            <td style="padding: 12px 16px">
              <div class="mono" style="font-size: 12.5px; color: var(--text-primary)">
                {{ app.totalMemoryMib }} MiB
              </div>
            </td>

            <!-- Network (1s Live) -->
            <td style="padding: 12px 16px">
              <div class="mono" style="font-size: 11.5px; white-space: nowrap">
                <div style="color: var(--text-primary); font-weight: 500">
                  {{ formatNetworkRate(app) }}
                </div>
                <div v-if="formatNetworkTotal(app)" style="font-size: 10.5px; color: var(--text-secondary)">
                  {{ formatNetworkTotal(app) }}
                </div>
              </div>
            </td>

            <!-- Actions (Icon-only buttons with tooltips) -->
            <td style="padding: 12px 16px; text-align: right">
              <div style="display: flex; gap: 6px; justify-content: flex-end; align-items: center">
                <button
                  v-if="app.state === 'RUNNING'"
                  class="btn btn-sm btn-icon btn-warning"
                  title="Stop Application"
                  aria-label="Stop Application"
                  @click="stopApp(app)"
                >
                  <MdiIcon :path="mdiStop" size="16" />
                </button>
                <button
                  v-if="app.state === 'RUNNING'"
                  class="btn btn-sm btn-icon"
                  title="Restart Application"
                  aria-label="Restart Application"
                  @click="restartApp(app)"
                >
                  <MdiIcon :path="mdiRestart" size="16" />
                </button>
                <button
                  v-if="app.state !== 'RUNNING'"
                  class="btn btn-sm btn-icon btn-success"
                  title="Start Application"
                  aria-label="Start Application"
                  @click="startApp(app)"
                >
                  <MdiIcon :path="mdiPlay" size="16" />
                </button>

                <button
                  class="btn btn-sm btn-icon"
                  title="View Application Logs"
                  aria-label="View Application Logs"
                  @click="openInspectModal(app, 'logs')"
                >
                  <MdiIcon :path="mdiFileDocumentOutline" size="16" />
                </button>

                <button
                  class="btn btn-sm btn-icon"
                  title="Edit Compose Configuration"
                  aria-label="Edit Compose Configuration"
                  @click="openEditModal(app)"
                >
                  <MdiIcon :path="mdiPencil" size="16" />
                </button>

                <button
                  class="btn btn-sm btn-icon btn-danger"
                  title="Delete Application"
                  aria-label="Delete Application"
                  @click="deleteApp(app)"
                >
                  <MdiIcon :path="mdiDelete" size="16" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- ===================================================================== -->
    <!-- MODAL 1: Launch App Modal                                             -->
    <!-- ===================================================================== -->
    <div v-if="isInstallModalOpen" class="modal-backdrop" @click.self="isInstallModalOpen = false">
      <div class="modal-card modal-card-xlarge">
        <!-- Header -->
        <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 14px; flex-shrink: 0">
          <div>
            <h3 style="font-size: 18px; font-weight: 700; color: var(--text-primary)">
              Launch Docker Application
            </h3>
            <p style="font-size: 12.5px; color: var(--text-secondary); margin-top: 2px">
              Configure services, containers, ports, environment, volumes, and networks.
            </p>
          </div>
          <button class="btn btn-sm" style="padding: 2px 6px" @click="isInstallModalOpen = false">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>

        <!-- Quick Pick Catalog Templates Bar -->
        <div style="background: rgba(0, 0, 0, 0.25); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 10px 12px; margin-bottom: 14px; flex-shrink: 0">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
            <span style="font-size: 11.5px; color: var(--text-secondary); font-weight: 600; text-transform: uppercase">
              Quick-Pick Pre-Configured Stack or Start Blank:
            </span>
            <button
              class="btn btn-sm"
              style="padding: 2px 8px; font-size: 11.5px; display: inline-flex; align-items: center; gap: 4px; color: var(--accent-primary)"
              @click="isInstallModalOpen = false; openCatalogModal()"
            >
              <MdiIcon :path="mdiStorefrontOutline" size="14" />
              <span>Browse TrueNAS Catalog (470+ Apps)...</span>
            </button>
          </div>
          <div style="display: flex; gap: 8px; overflow-x: auto; padding-bottom: 6px">
            <button
              v-for="tmpl in APP_CATALOG_TEMPLATES"
              :key="tmpl.id"
              class="btn btn-sm"
              :class="activeInstallTemplate?.id === tmpl.id ? 'btn-primary' : ''"
              style="font-size: 12px; padding: 5px 10px; display: flex; align-items: center; gap: 6px; white-space: nowrap"
              @click="selectCatalogTemplate(tmpl)"
            >
              <MdiIcon :path="tmpl.icon" size="16" />
              <span>{{ tmpl.name }}</span>
            </button>
            <button
              class="btn btn-sm"
              :class="activeInstallTemplate === null ? 'btn-primary' : ''"
              style="font-size: 12px; padding: 5px 10px; display: flex; align-items: center; gap: 6px; white-space: nowrap"
              @click="selectCatalogTemplate(null)"
            >
              <MdiIcon :path="mdiPlus" size="16" />
              <span>Custom Blank Stack</span>
            </button>
          </div>
        </div>

        <!-- Scrollable Body with Reusable Form-Based Compose Editor Component -->
        <div class="modal-scroll-body">
          <ComposeFormEditor
            :key="installEditorKey"
            :initial-app-id="installAppId"
            :initial-title="installTitle"
            :initial-compose-yaml="installComposeYaml"
            :is-edit-mode="false"
            @save="handleDeployApp"
            @cancel="isInstallModalOpen = false"
          />
        </div>
      </div>
    </div>

    <!-- ===================================================================== -->
    <!-- MODAL 2: Edit App Modal                                               -->
    <!-- ===================================================================== -->
    <div v-if="isEditModalOpen && editingApp" class="modal-backdrop" @click.self="isEditModalOpen = false">
      <div class="modal-card modal-card-xlarge">
        <!-- Header -->
        <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 14px; flex-shrink: 0">
          <div>
            <h3 style="font-size: 18px; font-weight: 700; color: var(--text-primary)">
              Edit App: {{ editingApp.title || editingApp.appId }}
            </h3>
            <p style="font-size: 12.5px; color: var(--text-secondary); margin-top: 2px">
              Modify application configuration and services.
            </p>
          </div>
          <button class="btn btn-sm" style="padding: 2px 6px" @click="isEditModalOpen = false">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>

        <!-- Scrollable Body with Reusable Form-Based Compose Editor Component in Edit Mode -->
        <div class="modal-scroll-body">
          <ComposeFormEditor
            :initial-app-id="editingApp.appId"
            :initial-title="editingApp.title"
            :initial-compose-yaml="editingApp.composeYaml"
            :is-edit-mode="true"
            @save="handleSaveAppEdit"
            @cancel="isEditModalOpen = false"
          />
        </div>
      </div>
    </div>

    <!-- ===================================================================== -->
    <!-- MODAL 3: App Inspector (Containers, Logs, YAML)                        -->
    <!-- ===================================================================== -->
    <div v-if="isInspectModalOpen && inspectedApp" class="modal-backdrop" @click.self="isInspectModalOpen = false">
      <div class="modal-card modal-card-large">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px; flex-shrink: 0">
          <div>
            <h3 style="font-size: 18px; font-weight: 700; color: var(--text-primary)">
              {{ inspectedApp.title || inspectedApp.appId }}
            </h3>
            <p class="mono" style="font-size: 12px; color: var(--text-secondary); margin-top: 2px">
              {{ inspectedApp.name }} · {{ inspectedApp.composeFilePath }}
            </p>
          </div>
          <button class="btn btn-sm" style="padding: 2px 6px" @click="isInspectModalOpen = false">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>

        <!-- Inspector Tabs -->
        <div style="display: flex; gap: 8px; border-bottom: 1px solid var(--border-subtle); padding-bottom: 10px; margin-bottom: 14px; flex-shrink: 0">
          <button
            class="btn btn-sm"
            :class="inspectActiveTab === 'containers' ? 'btn-primary' : ''"
            @click="inspectActiveTab = 'containers'"
          >
            <MdiIcon :path="mdiLayers" size="14" style="margin-right: 4px" />
            Child Containers ({{ inspectedApp.containers.length }})
          </button>
          <button
            class="btn btn-sm"
            :class="inspectActiveTab === 'logs' ? 'btn-primary' : ''"
            @click="openInspectModal(inspectedApp, 'logs')"
          >
            <MdiIcon :path="mdiFileDocumentOutline" size="14" style="margin-right: 4px" />
            Live Logs
          </button>
          <button
            class="btn btn-sm"
            :class="inspectActiveTab === 'compose' ? 'btn-primary' : ''"
            @click="inspectActiveTab = 'compose'"
          >
            <MdiIcon :path="mdiDocker" size="14" style="margin-right: 4px" />
            Compose YAML
          </button>
        </div>

        <!-- Tab 1: Containers Table -->
        <div v-if="inspectActiveTab === 'containers'" class="modal-scroll-body">
          <table class="table" style="width: 100%">
            <thead>
              <tr style="text-align: left; font-size: 11.5px; color: var(--text-secondary)">
                <th style="padding: 8px">CONTAINER</th>
                <th style="padding: 8px">SERVICE</th>
                <th style="padding: 8px">IMAGE</th>
                <th style="padding: 8px">STATE</th>
                <th style="padding: 8px">CPU</th>
                <th style="padding: 8px">RAM</th>
                <th style="padding: 8px">PORTS</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="c in inspectedApp.containers" :key="c.containerId" style="border-bottom: 1px solid var(--border-subtle); font-size: 12.5px">
                <td style="padding: 8px" class="mono font-semibold">{{ c.name }}</td>
                <td style="padding: 8px"><span class="badge badge-info">{{ c.serviceName }}</span></td>
                <td class="mono" style="padding: 8px; font-size: 11px">{{ c.image }}</td>
                <td style="padding: 8px">
                  <span class="badge" :class="c.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'">{{ c.state }}</span>
                </td>
                <td style="padding: 8px" class="mono">{{ c.cpuPercent.toFixed(1) }}%</td>
                <td style="padding: 8px" class="mono">{{ c.memoryMib }} MiB</td>
                <td class="mono" style="padding: 8px; font-size: 11px">
                  {{ formatContainerPorts(c.ports) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Tab 2: Aggregated Logs -->
        <div v-else-if="inspectActiveTab === 'logs'" class="modal-scroll-body" style="display: flex; flex-direction: column">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; gap: 10px; flex-wrap: wrap">
            <div style="display: flex; align-items: center; gap: 8px">
              <label style="font-size: 12px; color: var(--text-secondary)">Service:</label>
              <select v-model="selectedService" class="form-input" style="padding: 3px 8px; font-size: 12px; width: auto" @change="fetchLogs">
                <option value="">(All Services)</option>
                <option v-for="s in inspectedApp.services" :key="s" :value="s">{{ s }}</option>
              </select>

              <label style="font-size: 12px; color: var(--text-secondary); margin-left: 8px">Tail:</label>
              <select v-model="logTail" class="form-input" style="padding: 3px 8px; font-size: 12px; width: auto" @change="fetchLogs">
                <option :value="50">50 lines</option>
                <option :value="100">100 lines</option>
                <option :value="250">250 lines</option>
                <option :value="500">500 lines</option>
              </select>
            </div>

            <div style="display: flex; gap: 8px">
              <button class="btn btn-sm" :disabled="isLoadingLogs" @click="fetchLogs">
                <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" />
                {{ isLoadingLogs ? 'Fetching...' : 'Refresh' }}
              </button>
              <button class="btn btn-sm" @click="copyLogs">
                <MdiIcon :path="mdiContentCopy" size="14" style="margin-right: 4px" />
                Copy
              </button>
            </div>
          </div>

          <pre
            class="mono"
            style="
              flex: 1;
              background: #0d1117;
              color: #c9d1d9;
              padding: 12px;
              border-radius: 6px;
              font-size: 12px;
              line-height: 1.45;
              overflow: auto;
              max-height: 480px;
              white-space: pre-wrap;
              word-break: break-all;
              border: 1px solid var(--border-subtle);
            "
          >{{ logsContent }}</pre>
        </div>

        <!-- Tab 3: Compose Descriptor -->
        <div v-else-if="inspectActiveTab === 'compose'" class="modal-scroll-body" style="display: flex; flex-direction: column">
          <div style="display: flex; justify-content: flex-end; margin-bottom: 8px">
            <button class="btn btn-sm" @click="copyCompose(inspectedApp.composeYaml)">
              <MdiIcon :path="mdiContentCopy" size="14" style="margin-right: 4px" />
              Copy YAML
            </button>
          </div>
          <CodeMirrorEditor
            :model-value="inspectedApp.composeYaml"
            :read-only="true"
            min-height="420px"
          />
        </div>
      </div>
    </div>

    <!-- ===================================================================== -->
    <!-- MODAL: TrueNAS Application Catalog Modal                              -->
    <!-- ===================================================================== -->
    <div v-if="isCatalogModalOpen" class="modal-backdrop" @click.self="isCatalogModalOpen = false">
      <div class="modal-card modal-card-xlarge">
        <!-- Header -->
        <div
          style="
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 16px;
            flex-shrink: 0;
            padding-bottom: 14px;
            border-bottom: 1px solid var(--border-subtle);
          "
        >
          <div style="display: flex; align-items: center; gap: 12px">
            <div
              style="
                width: 40px;
                height: 40px;
                border-radius: 8px;
                background: rgba(59, 130, 246, 0.15);
                display: flex;
                align-items: center;
                justify-content: center;
                color: #3b82f6;
              "
            >
              <MdiIcon :path="mdiStorefrontOutline" size="24" />
            </div>
            <div>
              <div style="display: flex; align-items: center; gap: 8px">
                <h3 style="font-size: 19px; font-weight: 700; color: var(--text-primary); margin: 0">
                  TrueNAS SCALE App Catalog
                </h3>
                <span class="badge badge-info" style="font-size: 11px">
                  {{ catalogApps.length }} Apps
                </span>
              </div>
              <p style="font-size: 12.5px; color: var(--text-secondary); margin: 3px 0 0 0">
                Official and community Docker Compose applications ready to deploy.
                <span v-if="catalogLastSyncTime" style="margin-left: 6px; opacity: 0.8">
                  · Last synced: {{ formatSyncTime(catalogLastSyncTime) }}
                </span>
              </p>
            </div>
          </div>

          <div style="display: flex; align-items: center; gap: 10px">
            <button
              class="btn btn-sm"
              :disabled="isSyncingCatalog"
              title="Sync latest application definitions directly from TrueNAS upstream repository"
              @click="syncCatalog"
            >
              <MdiIcon
                :path="mdiCloudSync"
                size="16"
                :class="{ 'spin-icon': isSyncingCatalog }"
                style="margin-right: 5px"
              />
              {{ isSyncingCatalog ? 'Syncing...' : 'Sync Catalog' }}
            </button>

            <button class="btn btn-sm" style="padding: 4px 8px" @click="isCatalogModalOpen = false">
              <MdiIcon :path="mdiClose" size="18" />
            </button>
          </div>
        </div>

        <!-- Filter and Search Row -->
        <div
          style="
            display: flex;
            gap: 12px;
            align-items: center;
            margin-bottom: 14px;
            flex-shrink: 0;
            flex-wrap: wrap;
          "
        >
          <!-- Search box -->
          <div style="position: relative; flex: 1; min-width: 260px">
            <input
              v-model="catalogSearchQuery"
              type="text"
              class="form-input"
              placeholder="Search catalog by name, category, description, or tag..."
              style="width: 100%; padding-left: 36px; padding-right: 32px; height: 38px"
            />
            <MdiIcon
              :path="mdiMagnify"
              size="18"
              style="position: absolute; left: 10px; top: 10px; color: var(--text-secondary)"
            />
            <button
              v-if="catalogSearchQuery"
              class="btn-icon"
              style="position: absolute; right: 6px; top: 6px; background: none; border: none; padding: 4px; cursor: pointer"
              @click="catalogSearchQuery = ''"
            >
              <MdiIcon :path="mdiClose" size="14" />
            </button>
          </div>

          <!-- Train Filter Tabs -->
          <div
            style="
              display: flex;
              gap: 4px;
              background: var(--bg-surface-elevated, #161b22);
              padding: 3px;
              border-radius: 6px;
              border: 1px solid var(--border-subtle);
            "
          >
            <button
              class="btn btn-sm"
              :class="catalogSelectedTrain === 'ALL' ? 'btn-primary' : ''"
              style="padding: 4px 10px; font-size: 12px"
              @click="catalogSelectedTrain = 'ALL'"
            >
              All Trains
            </button>
            <button
              class="btn btn-sm"
              :class="catalogSelectedTrain === 'stable' ? 'btn-primary' : ''"
              style="padding: 4px 10px; font-size: 12px"
              @click="catalogSelectedTrain = 'stable'"
            >
              Stable
            </button>
            <button
              class="btn btn-sm"
              :class="catalogSelectedTrain === 'community' ? 'btn-primary' : ''"
              style="padding: 4px 10px; font-size: 12px"
              @click="catalogSelectedTrain = 'community'"
            >
              Community
            </button>
          </div>

          <!-- Category Dropdown -->
          <div style="min-width: 180px">
            <select
              v-model="catalogSelectedCategory"
              class="form-input"
              style="height: 38px; font-size: 13px"
            >
              <option value="ALL">All Categories ({{ catalogCategories.length }})</option>
              <option v-for="cat in catalogCategories" :key="cat" :value="cat">
                {{ cat }}
              </option>
            </select>
          </div>
        </div>

        <!-- Scrollable Catalog Grid Body -->
        <div class="modal-scroll-body" style="padding-top: 4px">
          <!-- Loading State -->
          <div v-if="isLoadingCatalog" style="text-align: center; padding: 60px 20px">
            <MdiIcon
              :path="mdiRefresh"
              size="36"
              class="spin-icon"
              style="color: var(--primary, #3b82f6); margin-bottom: 12px"
            />
            <div style="font-weight: 600; color: var(--text-primary)">
              Loading Catalog Applications...
            </div>
          </div>

          <!-- Empty Search State -->
          <div
            v-else-if="filteredCatalogApps.length === 0"
            style="
              text-align: center;
              padding: 60px 20px;
              background: rgba(0, 0, 0, 0.15);
              border-radius: 8px;
              border: 1px dashed var(--border-subtle);
            "
          >
            <MdiIcon
              :path="mdiStorefrontOutline"
              size="44"
              style="color: var(--text-muted, #6b7280); margin-bottom: 12px"
            />
            <h4 style="font-size: 16px; font-weight: 600; color: var(--text-primary); margin-bottom: 6px">
              No matching applications found
            </h4>
            <p style="font-size: 13px; color: var(--text-secondary); margin-bottom: 14px">
              Try adjusting your search terms or filters.
            </p>
            <button
              class="btn btn-sm"
              @click="
                catalogSearchQuery = '';
                catalogSelectedCategory = 'ALL';
                catalogSelectedTrain = 'ALL';
              "
            >
              Reset Filters
            </button>
          </div>

          <!-- App Cards Grid -->
          <div
            v-else
            style="
              display: grid;
              grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
              gap: 14px;
              padding-bottom: 16px;
            "
          >
            <div
              v-for="app in filteredCatalogApps"
              :key="app.appId + '-' + app.train"
              class="card"
              style="
                padding: 16px;
                display: flex;
                flex-direction: column;
                justify-content: space-between;
                border: 1px solid var(--border-subtle);
                border-radius: 8px;
                transition: border-color 0.15s, transform 0.15s;
              "
            >
              <div>
                <!-- Top Row: Icon + Title + Train Badge -->
                <div style="display: flex; gap: 12px; align-items: flex-start; margin-bottom: 10px">
                  <!-- App Icon -->
                  <div
                    style="
                      width: 44px;
                      height: 44px;
                      border-radius: 8px;
                      background: rgba(255, 255, 255, 0.06);
                      display: flex;
                      align-items: center;
                      justify-content: center;
                      flex-shrink: 0;
                      overflow: hidden;
                      border: 1px solid var(--border-subtle);
                    "
                  >
                    <img
                      v-if="app.iconUrl && !failedIcons[app.appId]"
                      :src="app.iconUrl"
                      :alt="app.title"
                      style="width: 36px; height: 36px; object-fit: contain"
                      @error="failedIcons[app.appId] = true"
                    />
                    <MdiIcon
                      v-else
                      :path="mdiDocker"
                      size="26"
                      style="color: var(--accent-primary, #38bdf8)"
                    />
                  </div>

                  <!-- Title & Category -->
                  <div style="flex: 1; min-width: 0">
                    <div style="display: flex; align-items: center; justify-content: space-between; gap: 6px">
                      <div
                        style="
                          font-weight: 700;
                          font-size: 15px;
                          color: var(--text-primary);
                          white-space: nowrap;
                          overflow: hidden;
                          text-overflow: ellipsis;
                        "
                        :title="app.title"
                      >
                        {{ app.title }}
                      </div>
                      <span
                        class="badge"
                        :class="app.train === 'stable' ? 'badge-success' : 'badge-info'"
                        style="font-size: 10.5px; text-transform: uppercase; padding: 2px 6px"
                      >
                        {{ app.train }}
                      </span>
                    </div>

                    <div
                      style="
                        display: flex;
                        align-items: center;
                        gap: 6px;
                        margin-top: 3px;
                        font-size: 12px;
                        color: var(--text-secondary);
                      "
                    >
                      <span style="white-space: nowrap; overflow: hidden; text-overflow: ellipsis">
                        {{ app.category || 'Utility' }}
                      </span>
                      <span v-if="app.appVersion" style="opacity: 0.6">· v{{ app.appVersion }}</span>
                    </div>
                  </div>
                </div>

                <!-- Description -->
                <p
                  style="
                    font-size: 12.5px;
                    color: var(--text-secondary);
                    line-height: 1.45;
                    margin: 0 0 12px 0;
                    display: -webkit-box;
                    -webkit-line-clamp: 3;
                    -webkit-box-orient: vertical;
                    overflow: hidden;
                    min-height: 48px;
                  "
                  :title="app.description"
                >
                  {{ app.description || 'No description provided.' }}
                </p>

                <!-- Tags / Port Info -->
                <div
                  v-if="app.tags && app.tags.length > 0"
                  style="display: flex; flex-wrap: wrap; gap: 4px; margin-bottom: 12px"
                >
                  <span
                    v-for="tag in app.tags.slice(0, 3)"
                    :key="tag"
                    class="badge badge-neutral"
                    style="font-size: 10px; padding: 1px 5px"
                  >
                    #{{ tag }}
                  </span>
                </div>
              </div>

              <!-- Card Action Bar -->
              <div
                style="
                  display: flex;
                  align-items: center;
                  justify-content: space-between;
                  padding-top: 10px;
                  border-top: 1px solid var(--border-subtle);
                  margin-top: auto;
                "
              >
                <a
                  v-if="app.home"
                  :href="app.home"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="btn btn-sm btn-icon"
                  style="color: var(--text-secondary)"
                  title="Visit Project Homepage"
                >
                  <MdiIcon :path="mdiOpenInNew" size="14" />
                </a>
                <div v-else />

                <button
                  class="btn btn-primary btn-sm"
                  :disabled="importingAppId === app.appId"
                  style="display: flex; align-items: center; gap: 6px; font-size: 12px; padding: 5px 12px"
                  @click="importFromCatalog(app)"
                >
                  <MdiIcon
                    :path="importingAppId === app.appId ? mdiRefresh : mdiDownload"
                    size="15"
                    :class="{ 'spin-icon': importingAppId === app.appId }"
                  />
                  <span>{{ importingAppId === app.appId ? 'Importing...' : 'Import to Editor' }}</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ===================================================================== -->
    <!-- MODAL: Docker Storage Dataset Configuration                           -->
    <!-- ===================================================================== -->
    <div v-if="isDatasetModalOpen" class="modal-backdrop" @click.self="isDatasetModalOpen = false">
      <div class="modal-card" style="max-width: 540px">
        <!-- Header -->
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; padding-bottom: 12px; border-bottom: 1px solid var(--border-subtle)">
          <div style="display: flex; align-items: center; gap: 10px">
            <MdiIcon :path="mdiFolderOutline" size="22" style="color: var(--accent-primary)" />
            <h3 style="font-size: 18px; font-weight: 700; color: var(--text-primary); margin: 0">
              Docker Storage Dataset
            </h3>
          </div>
          <button class="btn btn-sm" style="padding: 2px 6px" @click="isDatasetModalOpen = false">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>

        <p style="font-size: 13px; color: var(--text-secondary); margin-bottom: 16px; line-height: 1.5">
          Select or specify the ZFS dataset owned by Docker on the main pool (<span class="mono" style="color: var(--accent-primary)">{{ nasState.systemConfig?.mainPool || 'tank' }}</span>). Apps root is restricted to the main storage pool. Docker will store container images and persistent application volumes inside this dataset without taking over the entire pool.
        </p>

        <!-- Dataset Selector Form -->
        <div class="form-group" style="margin-bottom: 16px">
          <label style="display: block; font-size: 12px; font-weight: 600; color: var(--text-secondary); margin-bottom: 6px; text-transform: uppercase">
            Available Datasets on Main Pool ({{ nasState.systemConfig?.mainPool || 'tank' }})
          </label>
          <select v-model="selectedDatasetToSave" class="form-input" style="width: 100%; margin-bottom: 10px">
            <option v-for="d in availableDatasetNames" :key="d" :value="d">
              {{ d }}
            </option>
          </select>

          <label style="display: block; font-size: 12px; font-weight: 600; color: var(--text-secondary); margin-bottom: 6px; text-transform: uppercase">
            Or Custom Dataset Path ({{ nasState.systemConfig?.mainPool || 'tank' }}/dataset)
          </label>
          <input
            v-model="selectedDatasetToSave"
            type="text"
            class="form-input"
            style="width: 100%"
            :placeholder="`e.g. ${nasState.systemConfig?.mainPool || 'tank'}/apps`"
          />
        </div>

        <!-- Path Preview Card -->
        <div style="background: rgba(0,0,0,0.25); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 12px 14px; margin-bottom: 20px">
          <div style="font-size: 11.5px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; margin-bottom: 8px">
            Derived Paths on Host:
          </div>
          <div style="display: flex; flex-direction: column; gap: 6px; font-size: 12.5px" class="mono">
            <div>
              <span style="color: var(--text-secondary)">ZFS Dataset:</span>
              <span style="color: #38bdf8; margin-left: 6px">{{ selectedDatasetToSave }}</span>
            </div>
            <div>
              <span style="color: var(--text-secondary)">Mountpoint:</span>
              <span style="color: #38bdf8; margin-left: 6px">/{{ selectedDatasetToSave.replace(/^\//, '') }}</span>
            </div>
            <div>
              <span style="color: var(--text-secondary)">App Volumes:</span>
              <span style="color: #38bdf8; margin-left: 6px">/{{ selectedDatasetToSave.replace(/^\//, '') }}/appdata</span>
            </div>
            <div>
              <span style="color: var(--text-secondary)">Docker Data Root:</span>
              <span style="color: #38bdf8; margin-left: 6px">/{{ selectedDatasetToSave.replace(/^\//, '') }}/docker</span>
            </div>
          </div>
        </div>

        <!-- Footer -->
        <div style="display: flex; justify-content: flex-end; gap: 10px">
          <button class="btn btn-sm" @click="isDatasetModalOpen = false">
            Cancel
          </button>
          <button
            class="btn btn-sm btn-primary"
            :disabled="isSavingDataset || !selectedDatasetToSave"
            style="display: inline-flex; align-items: center; gap: 4px"
            @click="handleSaveDataset"
          >
            <MdiIcon :path="mdiCheck" size="16" />
            <span>{{ isSavingDataset ? 'Saving...' : 'Save Configuration' }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
