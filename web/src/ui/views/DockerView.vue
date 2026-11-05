<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import {
  mdiAlertCircleOutline,
  mdiArrowUpBoldCircleOutline,
  mdiCheck,
  mdiChevronDoubleDown,
  mdiChevronDown,
  mdiClose,
  mdiCloudSync,
  mdiConsole,
  mdiContentCopy,
  mdiDelete,
  mdiDocker,
  mdiDownload,
  mdiEye,
  mdiFileDocumentOutline,
  mdiFolderOutline,
  mdiHarddisk,
  mdiInformationOutline,
  mdiKeyVariant,
  mdiLayers,
  mdiLoading,
  mdiMagnify,
  mdiOpenInNew,
  mdiPencil,
  mdiPlay,
  mdiPlus,
  mdiRefresh,
  mdiRestart,
  mdiStop,
  mdiStorefrontOutline,
  mdiUpdate,
} from '@mdi/js';
import { services } from '../../container';
import { generateNanosecondId, toZfsSafeSlug } from '../../domain/id';
import {
  APP_CATALOG_TEMPLATES,
  type AppCatalogTemplate,
  type CatalogApp,
  type DockerApp,
  isDotDataset,
} from '../../domain/models';
import { composeYamlToForm, formToComposeYaml } from '../../domain/composeModel';
import ComposeFormEditor from '../components/ComposeFormEditor.vue';
import CodeMirrorEditor from '../components/CodeMirrorEditor.vue';
import CatalogQuestionWizard from '../components/CatalogQuestionWizard.vue';
import DockerRegistriesModal from '../components/DockerRegistriesModal.vue';
import MdiIcon from '../components/MdiIcon.vue';
import SkeletonBox from '../components/SkeletonBox.vue';
import { getDockerAppIcon } from '../utils/appIcons';
import SortableTh from '../components/SortableTh.vue';
import { useTableSort } from '../composables/useTableSort';
import { checkAndPollDeployingApps, nasState, notify, selectResource } from '../store';
import { confirmDelete } from '../modal';

// =============================================================================
// Filters & Search
// =============================================================================
const searchQuery = ref('');
const stateFilter = ref<'ALL' | 'RUNNING' | 'STOPPED' | 'PARTIALLY_RUNNING'>('ALL');
const isRefreshing = ref(false);
const failedAppIcons = ref<Record<string, boolean>>({});
const transitioningApps = reactive<Record<string, 'starting' | 'stopping' | 'restarting'>>({});

const runningCount = computed(
  () => (nasState.dockerApps || []).filter((a) => a?.state === 'RUNNING').length
);
const stoppedCount = computed(
  () => (nasState.dockerApps || []).filter((a) => a?.state === 'STOPPED').length
);
const partialCount = computed(
  () => (nasState.dockerApps || []).filter((a) => a?.state === 'PARTIALLY_RUNNING').length
);

const totalContainersCount = computed(() =>
  (nasState.dockerApps || []).reduce((acc, a) => acc + (a?.containers?.length || 0), 0)
);

const totalMemoryUsageMib = computed(() =>
  (nasState.dockerApps || []).reduce((acc, a) => acc + (Number(a?.totalMemoryMib) || 0), 0)
);

const filteredApps = computed(() => {
  return (nasState.dockerApps || []).filter((a) => {
    if (!a) return false;
    if (stateFilter.value !== 'ALL' && a.state !== stateFilter.value) {
      return false;
    }
    if (!searchQuery.value.trim()) return true;
    const q = searchQuery.value.toLowerCase();
    const matchesId = String(a.appId || '').toLowerCase().includes(q);
    const matchesTitle = String(a.title || '').toLowerCase().includes(q);
    const matchesService = (a.services || []).some((s) => String(s || '').toLowerCase().includes(q));
    const matchesPort = (a.portMappings || []).some((p) => String(p || '').toLowerCase().includes(q));
    const matchesContainer = (a.containers || []).some(
      (c) => String(c?.name || '').toLowerCase().includes(q) || String(c?.image || '').toLowerCase().includes(q)
    );
    return matchesId || matchesTitle || matchesService || matchesPort || matchesContainer;
  });
});

const appSort = useTableSort(filteredApps, {
  name: (a) => a?.title || a?.appId || '',
  autostart: (a) => (a?.autoStart ? 1 : 0),
  webui: (a) =>
    getPrimaryWebUiUrl(a) ? (a?.portals && a.portals.length > 0 ? a.portals[0]?.name || 'Open UI' : 'Open UI') : null,
  cpu: (a) => Number(a?.totalCpuPercent) || 0,
  memory: (a) => Number(a?.totalMemoryMib) || 0,
  network: (a) => (a?.state === 'RUNNING' ? (Number(a?.totalNetRxRate) || 0) + (Number(a?.totalNetTxRate) || 0) : null),
});

function openDockerDetail(app: DockerApp) {
  const clean = app.appId || app.name.replace(/^dockerApps\//, '');
  selectResource(`docker:${clean}`);
}

// =============================================================================
// Realtime Telemetry & Network Rate Formatter
// =============================================================================
function formatNetworkRate(app: DockerApp): string {
  if (app.state !== 'RUNNING') return '—';
  const rx = app.totalNetRxRate || 0;
  const tx = app.totalNetTxRate || 0;
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
  if (!bytes || bytes <= 0 || !Number.isFinite(bytes)) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  if (i < 0 || i >= sizes.length || !Number.isFinite(i)) return '0 B';
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
    if (apps && Array.isArray(apps)) {
      if (apps.length > 0 || nasState.dockerApps.length === 0) {
        nasState.dockerApps = apps;
        checkAndPollDeployingApps();
      }
    }
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

// =============================================================================
// TrueNAS Application Catalog
// =============================================================================
const isCatalogModalOpen = ref(false);
const catalogApps = ref<CatalogApp[]>(nasState.catalogApps || []);
const isLoadingCatalog = ref(false);
const isSyncingCatalog = ref(false);
const catalogSearchQuery = ref('');
const catalogSelectedTrain = ref<'ALL' | 'stable' | 'community' | 'casaos' | 'dokploy'>('ALL');
const catalogSelectedCategory = ref<string>('ALL');
const catalogLastSyncTime = ref<string>(nasState.catalogLastSync || '');
const catalogTotalCount = ref<number>(nasState.catalogApps?.length || 0);
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
    nasState.catalogApps = res.apps;
    catalogTotalCount.value = res.totalSize;
    if (res.lastSyncTime) {
      catalogLastSyncTime.value = res.lastSyncTime;
      nasState.catalogLastSync = res.lastSyncTime;
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
    notify('info', 'Checking upstream catalogs for updates...');
    const res = await services.catalog.syncCatalog(true);
    catalogLastSyncTime.value = res.lastSyncTime;
    nasState.catalogLastSync = res.lastSyncTime;
    await Promise.all([loadCatalog(), refreshApps()]);
    const updatesCount = nasState.dockerApps.filter((a) => a.updateAvailable).length;
    if (updatesCount > 0) {
      notify(
        'success',
        `Catalog synchronized (${res.totalApps} apps) — ${updatesCount} app update${updatesCount === 1 ? '' : 's'} available.`
      );
    } else {
      notify(
        'success',
        res.message || `Catalog synchronized (${res.totalApps} apps) — all installed apps are up to date.`
      );
    }
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

// =============================================================================
// TrueNAS SCALE App Configuration Wizard
// =============================================================================
const isQuestionWizardOpen = ref(false);
const wizardCatalogApp = ref<CatalogApp | null>(null);

function injectCatalogMetadata(yaml: string, catApp: CatalogApp): string {
  if (!catApp || !yaml) return yaml;
  let res = yaml;
  if (!res.includes('x-nas-catalog-id:')) {
    const meta = `x-nas-catalog-id: "${catApp.appId}"\nx-nas-catalog-source: "${catApp.train || 'stable'}"\nx-nas-catalog-version: "${catApp.version || catApp.appVersion || ''}"\n`;
    res = meta + res;
  }
  return res;
}

async function importFromCatalog(app: CatalogApp) {
  importingAppId.value = app.appId;
  try {
    const detailed = await services.catalog.getApp(app.appId, app.train);
    const hasQuestions =
      (detailed.questionGroups && detailed.questionGroups.length > 0) ||
      (detailed.questions && detailed.questions.length > 0);

    if (hasQuestions) {
      wizardCatalogApp.value = detailed;
      isCatalogModalOpen.value = false;
      isQuestionWizardOpen.value = true;
      notify('info', `Opened Setup Wizard for "${detailed.title || detailed.appId}"`);
    } else {
      installTitle.value = detailed.title || detailed.appId;
      const rawSlug = detailed.appId && detailed.appId.includes('.') ? detailed.appId.split('.').pop()! : (detailed.title || detailed.appId);
      installAppId.value = toZfsSafeSlug(detailed.title || rawSlug);
      const resolvedImg = detailed.containerImage || `${toZfsSafeSlug(rawSlug)}:latest`;
      const rawYaml =
        detailed.composeYaml ||
        `services:\n  ${installAppId.value}:\n    image: ${resolvedImg}\n`;
      installComposeYaml.value = injectCatalogMetadata(rawYaml, detailed);
      activeInstallTemplate.value = null;
      installEditorKey.value++;
      isCatalogModalOpen.value = false;
      isInstallModalOpen.value = true;
      notify('success', `Imported "${detailed.title}" from catalog`);
    }
  } catch (err: any) {
    notify('error', err?.message || `Failed to load template for ${app.title}`);
  } finally {
    importingAppId.value = null;
  }
}

function handleWizardOpenInEditor(data: { appId: string; title: string; composeYaml: string }) {
  installTitle.value = data.title;
  installAppId.value = toZfsSafeSlug(data.appId || data.title);
  installComposeYaml.value = wizardCatalogApp.value
    ? injectCatalogMetadata(data.composeYaml, wizardCatalogApp.value)
    : data.composeYaml;
  activeInstallTemplate.value = null;
  installEditorKey.value++;
  isQuestionWizardOpen.value = false;
  isInstallModalOpen.value = true;
  notify('success', `Configuration loaded into visual Compose Editor`);
}

function handleWizardDeploy(data: { appId: string; title: string; composeYaml: string }) {
  isQuestionWizardOpen.value = false;
  if (wizardCatalogApp.value) {
    data.composeYaml = injectCatalogMetadata(data.composeYaml, wizardCatalogApp.value);
  }
  handleDeployApp(data);
}

function handleWizardSkip() {
  if (!wizardCatalogApp.value) return;
  const app = wizardCatalogApp.value;
  installTitle.value = app.title || app.appId;
  const rawSlug = app.appId && app.appId.includes('.') ? app.appId.split('.').pop()! : (app.title || app.appId);
  installAppId.value = toZfsSafeSlug(app.title || rawSlug);
  const resolvedImg = app.containerImage || `${toZfsSafeSlug(rawSlug)}:latest`;
  const rawYaml =
    app.composeYaml ||
    `services:\n  ${installAppId.value}:\n    image: ${resolvedImg}\n`;
  installComposeYaml.value = injectCatalogMetadata(rawYaml, app);
  activeInstallTemplate.value = null;
  installEditorKey.value++;
  isQuestionWizardOpen.value = false;
  isInstallModalOpen.value = true;
  notify('info', `Loaded raw template for "${app.title || app.appId}" into Compose Editor`);
}

// =============================================================================
// Docker Owned ZFS Storage Dataset Configuration
// =============================================================================
const isDatasetModalOpen = ref(false);
const selectedDatasetToSave = ref('tank/apps');
const isSavingDataset = ref(false);

const availableDatasetNames = computed(() => {
  const rawMain = nasState.systemConfig?.mainPool || (nasState.dockerConfig?.availablePools?.[0] || 'tank');
  const mainPool = rawMain.replace(/^zfsPools\//, '').trim() || 'tank';
  if (nasState.dockerConfig?.availableDatasets && nasState.dockerConfig.availableDatasets.length > 0) {
    return nasState.dockerConfig.availableDatasets
      .map((d) => d.replace(/^zfsPools\//, '').trim())
      .filter((d) => (d.startsWith(`${mainPool}/`) || d === mainPool) && !isDotDataset(d));
  }
  const list: string[] = [];
  const poolDatasets = nasState.datasetsByPool[mainPool] || nasState.datasetsByPool[`zfsPools/${mainPool}`];
  if (poolDatasets && poolDatasets.length > 0) {
    for (const d of poolDatasets) {
      if (isDotDataset(d)) continue;
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
      selectedDatasetToSave.value = cfg.dataset.replace(/^zfsPools\//, '').trim();
    } else if (cfg.pool) {
      selectedDatasetToSave.value = `${cfg.pool.replace(/^zfsPools\//, '').trim()}/apps`;
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
    installAppId.value = toZfsSafeSlug(tmpl.name || tmpl.id);
    installTitle.value = tmpl.name;
    installComposeYaml.value = tmpl.composeYaml;
  } else {
    installTitle.value = 'Custom Web App';
    installAppId.value = toZfsSafeSlug(installTitle.value);
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

const isDeployingApp = ref(false);
const isRegistriesModalOpen = ref(false);

async function handleDeployApp(data: { appId: string; title: string; composeYaml: string }) {
  if (isDeployingApp.value) return;
  isDeployingApp.value = true;
  const targetId = toZfsSafeSlug(data.appId || data.title) || 'app';
  try {
    await services.apps.createApp(targetId, data.composeYaml, data.title || targetId);
    notify('info', `Deploying "${targetId}" in background. Pulling container images and starting stack...`);
    isInstallModalOpen.value = false;
    await refreshApps();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to deploy Docker App');
  } finally {
    isDeployingApp.value = false;
  }
}



// =============================================================================
// Upgrade App Modal
// =============================================================================
const isUpgradeModalOpen = ref(false);
const upgradingApp = ref<DockerApp | null>(null);
const upgradeTakeSnapshot = ref(true);
const isUpgradingApp = ref(false);

function openUpgradeModal(app: DockerApp) {
  upgradingApp.value = app;
  upgradeTakeSnapshot.value = true;
  isUpgradeModalOpen.value = true;
}

async function handleConfirmUpgrade() {
  if (!upgradingApp.value || isUpgradingApp.value) return;
  isUpgradingApp.value = true;
  const app = upgradingApp.value;
  try {
    await services.apps.upgradeApp(app.name, upgradeTakeSnapshot.value);
    notify(
      'info',
      `Upgrade initiated for "${app.title || app.appId}". Pulling latest images & recreating containers...`
    );
    isUpgradeModalOpen.value = false;
    await refreshApps();
  } catch (err: any) {
    notify('error', err?.message || `Failed to upgrade ${app.title || app.appId}`);
  } finally {
    isUpgradingApp.value = false;
  }
}

// =============================================================================
// App Lifecycle Actions
// =============================================================================
async function startApp(app: DockerApp) {
  if (transitioningApps[app.name]) return;
  transitioningApps[app.name] = 'starting';
  try {
    notify('info', `Starting "${app.appId}" in background (pulling images if needed)...`);
    await services.apps.startApp(app.name);
    await refreshApps();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to start App');
  } finally {
    delete transitioningApps[app.name];
  }
}

async function stopApp(app: DockerApp) {
  if (transitioningApps[app.name]) return;
  transitioningApps[app.name] = 'stopping';
  try {
    notify('info', `Stopping app "${app.appId}"...`);
    await services.apps.stopApp(app.name);
    notify('success', `App "${app.appId}" stopped`);
    await refreshApps();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to stop App');
  } finally {
    delete transitioningApps[app.name];
  }
}

async function restartApp(app: DockerApp) {
  if (transitioningApps[app.name]) return;
  transitioningApps[app.name] = 'restarting';
  try {
    notify('info', `Restarting app "${app.appId}"...`);
    await services.apps.restartApp(app.name);
    notify('success', `App "${app.appId}" restarted`);
    await refreshApps();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to restart App');
  } finally {
    delete transitioningApps[app.name];
  }
}

async function deleteApp(app: DockerApp) {
  const res = await confirmDelete({
    kind: 'App',
    name: app.appId,
    details: 'All containers and the compose project of this app will be stopped and removed.',
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

// Port selection for Open Web UI
const activePortMenuAppId = ref<string | null>(null);

interface AppPortEntry {
  hostPort: string;
  containerPort: string;
  protocol: string;
}

const WELL_KNOWN_WEB_PORTS = new Set(['80', '81', '443', '3000', '5000', '5001', '8000', '8080', '8088', '8096', '8123', '8384', '8443', '9000', '9443']);

function getAppPorts(app: DockerApp): AppPortEntry[] {
  const result: AppPortEntry[] = [];
  const seen = new Set<string>();

  const rawPorts = [...(app?.portMappings || [])];
  for (const c of app?.containers || []) {
    for (const p of c?.ports || []) {
      rawPorts.push(p);
    }
  }

  for (const p of rawPorts) {
    let clean = String(p || '').trim();
    if (!clean) continue;

    // Handle raw stringified Go map: e.g. "map[mode:ingress protocol:tcp published:30020 target:81]"
    if (clean.startsWith('map[') && clean.endsWith(']')) {
      const inner = clean.slice(4, -1);
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
      const pr = (m['protocol'] || 'tcp').toLowerCase();
      if (pub && tgt) {
        clean = `${pub}:${tgt}${pr !== 'tcp' ? '/' + pr : ''}`;
      } else if (tgt) {
        clean = `${tgt}${pr !== 'tcp' ? '/' + pr : ''}`;
      } else {
        continue;
      }
    }

    let proto = 'tcp';
    if (clean.includes('/')) {
      const parts = clean.split('/');
      clean = parts[0];
      proto = parts[1] || 'tcp';
    }
    // format 0.0.0.0:8080->80
    if (clean.includes('->')) {
      const parts = clean.split('->');
      const hostPart = parts[0].replace(/^(0\.0\.0\.0|\[::\]|:::):/, '');
      const contPart = parts[1];
      const key = `${hostPart}:${contPart}`;
      if (!seen.has(key) && hostPart && contPart) {
        seen.add(key);
        result.push({ hostPort: hostPart, containerPort: contPart, protocol: proto });
      }
      continue;
    }
    // format host:container
    const colons = clean.split(':');
    if (colons.length === 2) {
      const hostPart = colons[0];
      const contPart = colons[1];
      const key = `${hostPart}:${contPart}`;
      if (!seen.has(key) && hostPart && contPart) {
        seen.add(key);
        result.push({ hostPort: hostPart, containerPort: contPart, protocol: proto });
      }
    } else if (colons.length === 3) {
      const hostPart = colons[1];
      const contPart = colons[2];
      const key = `${hostPart}:${contPart}`;
      if (!seen.has(key) && hostPart && contPart) {
        seen.add(key);
        result.push({ hostPort: hostPart, containerPort: contPart, protocol: proto });
      }
    }
  }

  return result;
}

function getAppWebPorts(app: DockerApp): AppPortEntry[] {
  const ports = getAppPorts(app);
  return ports.filter(p => {
    if (p.protocol && p.protocol.toLowerCase() === 'udp') return false;
    return WELL_KNOWN_WEB_PORTS.has(p.containerPort) || WELL_KNOWN_WEB_PORTS.has(p.hostPort);
  });
}

function togglePortMenu(appId: string) {
  if (activePortMenuAppId.value === appId) {
    activePortMenuAppId.value = null;
  } else {
    activePortMenuAppId.value = appId;
  }
}

function openPortUrl(hostPort: string) {
  if (!hostPort) return;
  const hostname = window.location.hostname || 'localhost';
  const url = `http://${hostname}:${hostPort}`;
  window.open(url, '_blank', 'noopener,noreferrer');
  activePortMenuAppId.value = null;
}

function isPortActiveWebUi(app: DockerApp, p: AppPortEntry): boolean {
  if (!app.webUiUrl) return false;
  return app.webUiUrl.endsWith(`:${p.hostPort}`) || app.webUiUrl.endsWith(`:${p.hostPort}/`);
}

function handleGlobalClick(e: MouseEvent) {
  if (!(e.target as HTMLElement)?.closest('.webui-port-dropdown') && !(e.target as HTMLElement)?.closest('.webui-port-trigger')) {
    activePortMenuAppId.value = null;
  }
}

function getPrimaryWebUiUrl(app: DockerApp): string {
  if (app.portals && app.portals.length > 0 && app.portals[0].url) {
    return app.portals[0].url;
  }
  if (app.webUiUrl) return app.webUiUrl;
  const webPorts = getAppWebPorts(app);
  if (webPorts.length > 0 && webPorts[0].hostPort) {
    return `http://${window.location.hostname || 'localhost'}:${webPorts[0].hostPort}`;
  }
  return '';
}

function openWebPortal(url: string) {
  if (!url) return;
  let targetUrl = url;
  if (targetUrl.includes('localhost')) {
    targetUrl = targetUrl.replace('localhost', window.location.hostname);
  }
  window.open(targetUrl, '_blank', 'noopener,noreferrer');
}

let statsPollTimer: any = null;

onMounted(() => {
  window.addEventListener('click', handleGlobalClick);
  fetchDockerConfig();
  refreshApps();
  loadCatalog().catch(() => {});
  statsPollTimer = setInterval(() => {
    if (!nasState.liveSyncActive) {
      updateTelemetry();
    }
  }, 4000);
});

onUnmounted(() => {
  window.removeEventListener('click', handleGlobalClick);
  if (statsPollTimer) {
    clearInterval(statsPollTimer);
    statsPollTimer = null;
  }
});
</script>

<template>
  <div class="apps-container">
    <!-- Header & Action Bar -->
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; flex-wrap: wrap; gap: 12px">
      <div>
        <h2 style="font-size: 22px; font-weight: 700; color: var(--text-primary); margin: 0">
          Apps
        </h2>
      </div>

      <div style="display: flex; gap: 8px; align-items: center; flex-wrap: wrap">
        <button class="btn btn-sm" :disabled="isRefreshing" @click="refreshApps">
          <MdiIcon :path="mdiRefresh" size="14" :class="{ 'spin-icon': isRefreshing }" />
          <span class="hide-mobile" style="margin-left: 4px">{{ isRefreshing ? 'Refreshing...' : 'Refresh' }}</span>
        </button>

        <button
          class="btn btn-sm"
          style="display: flex; align-items: center; gap: 5px"
          :disabled="isSyncingCatalog"
          title="Sync upstream app catalogs and check installed apps for updates"
          @click="syncCatalog"
        >
          <MdiIcon :path="mdiUpdate" size="15" :class="{ 'spin-icon': isSyncingCatalog }" style="color: var(--accent-primary)" />
          <span class="hide-mobile">{{ isSyncingCatalog ? 'Checking...' : 'Check Updates' }}</span>
        </button>

        <button
          class="btn btn-sm"
          style="display: flex; align-items: center; gap: 6px"
          :class="(nasState.dockerConfig?.datasetSelected ?? nasState.dockerConfig?.poolSelected) ? '' : 'btn-warning'"
          title="Configure storage dataset"
          @click="openDatasetModal"
        >
          <MdiIcon :path="mdiFolderOutline" size="16" style="color: var(--accent-primary)" />
          <span class="hide-mobile">Dataset: {{ nasState.dockerConfig?.dataset || nasState.dockerConfig?.pool || 'None' }}</span>
          <span class="show-mobile-only">Storage</span>
        </button>

        <button
          class="btn btn-sm"
          style="display: flex; align-items: center; gap: 6px"
          title="Container registry credentials (Docker Hub, GHCR, private registries)"
          @click="isRegistriesModalOpen = true"
        >
          <MdiIcon :path="mdiKeyVariant" size="16" style="color: var(--accent-primary)" />
          <span class="hide-mobile">Registries</span>
        </button>

        <button class="btn btn-sm" style="display: flex; align-items: center; gap: 6px" @click="openCatalogModal">
          <MdiIcon :path="mdiStorefrontOutline" size="16" style="color: var(--accent-primary)" />
          <span>Catalog</span>
        </button>

        <button class="btn btn-primary btn-sm" @click="openInstallModal">
          <MdiIcon :path="mdiPlus" size="16" style="margin-right: 4px" />
          <span>Launch App</span>
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
        <MdiIcon :path="mdiAlertCircleOutline" size="24" style="color: var(--warning-text); flex-shrink: 0" />
        <div>
          <div style="font-weight: 700; font-size: 14px; color: var(--text-primary)">
            Docker Storage Dataset Not Configured
          </div>
          <div style="font-size: 12.5px; color: var(--text-secondary); margin-top: 2px">
            Select or create a dataset to store container images and app volumes.
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
        <div style="font-size: 26px; font-weight: 700; color: var(--success-text); margin-top: 4px">
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
        <div style="font-size: 26px; font-weight: 700; color: var(--accent-primary); margin-top: 4px">
          {{ totalMemoryUsageMib }} MiB
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

    <!-- Applications Loading Skeleton View -->
    <div v-if="nasState.loading && nasState.dockerApps.length === 0" class="card" style="padding: 0; overflow-x: auto">
      <table class="data-table" style="width: 100%; border-collapse: collapse">
        <thead>
          <tr style="border-bottom: 1px solid var(--border-subtle); text-align: left; font-size: 11.5px; color: var(--text-secondary); text-transform: uppercase">
            <th style="padding: 10px 14px; width: 280px">Application</th>
            <th style="padding: 10px 14px; width: 130px">Web UI</th>
            <th class="col-desktop-only" style="padding: 10px 14px; width: 85px">CPU</th>
            <th class="col-desktop-only" style="padding: 10px 14px; width: 105px">Memory</th>
            <th class="col-desktop-only" style="padding: 10px 14px; width: 210px">Network (1s)</th>
            <th style="padding: 10px 14px; min-width: 240px; text-align: right">Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in 4" :key="r" style="border-bottom: 1px solid var(--border-subtle)">
            <td style="padding: 10px 14px">
              <div style="display: flex; align-items: center; gap: 10px">
                <SkeletonBox width="16px" height="16px" circle />
                <div style="display: flex; flex-direction: column; gap: 4px">
                  <SkeletonBox width="140px" height="15px" />
                  <SkeletonBox width="90px" height="11px" />
                </div>
              </div>
            </td>
            <td style="padding: 10px 14px"><SkeletonBox width="75px" height="20px" rounded="10px" /></td>
            <td class="col-desktop-only" style="padding: 10px 14px"><SkeletonBox width="60px" height="14px" /></td>
            <td class="col-desktop-only" style="padding: 10px 14px"><SkeletonBox width="80px" height="14px" /></td>
            <td class="col-desktop-only" style="padding: 10px 14px"><SkeletonBox width="110px" height="14px" /></td>
            <td style="padding: 10px 14px; text-align: right">
              <div style="display: flex; gap: 6px; justify-content: flex-end">
                <SkeletonBox width="28px" height="28px" rounded="6px" />
                <SkeletonBox width="28px" height="28px" rounded="6px" />
                <SkeletonBox width="28px" height="28px" rounded="6px" />
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Empty State -->
    <div
      v-else-if="filteredApps.length === 0"
      class="card"
      style="text-align: center; padding: 48px 24px"
    >
      <div style="margin-bottom: 12px; color: var(--text-muted)">
        <MdiIcon :path="mdiLayers" size="44" />
      </div>
      <h3 style="font-size: 18px; font-weight: 700; color: var(--text-primary); margin-bottom: 18px">
        No Applications Found
      </h3>
      <div style="display: flex; gap: 10px; justify-content: center">
        <button class="btn btn-primary" @click="openInstallModal">
          <MdiIcon :path="mdiPlus" size="16" style="margin-right: 4px" />
          Launch App
        </button>
        <button class="btn" @click="openCatalogModal">
          <MdiIcon :path="mdiStorefrontOutline" size="16" style="margin-right: 4px; color: var(--accent-primary)" />
          Browse Catalog
        </button>
      </div>
    </div>

    <!-- Applications Table View -->
    <div v-else class="card" style="padding: 0; overflow-x: auto">
      <table class="data-table" style="width: 100%; border-collapse: collapse">
        <thead>
          <tr style="border-bottom: 1px solid var(--border-subtle); text-align: left; font-size: 12px; color: var(--text-secondary); text-transform: uppercase">
            <SortableTh :sort="appSort" sort-key="name" style="padding: 8px 10px; width: 280px; min-width: 220px; max-width: 340px">Application</SortableTh>
            <SortableTh :sort="appSort" sort-key="webui" style="padding: 8px 10px; width: 130px; min-width: 130px">Web UI</SortableTh>
            <SortableTh :sort="appSort" sort-key="cpu" class="col-desktop-only" style="padding: 8px 10px; width: 85px; min-width: 85px; max-width: 85px">CPU</SortableTh>
            <SortableTh :sort="appSort" sort-key="memory" class="col-desktop-only" style="padding: 8px 10px; width: 105px; min-width: 105px; max-width: 105px">Memory</SortableTh>
            <SortableTh :sort="appSort" sort-key="network" class="col-desktop-only" style="padding: 8px 10px; width: 210px; min-width: 210px; max-width: 210px">Network (1s)</SortableTh>
            <th style="padding: 8px 10px; min-width: 240px; text-align: right">Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="app in appSort.sorted.value"
            :key="app.name"
            style="border-bottom: 1px solid var(--border-subtle); cursor: pointer"
            :title="`Application: ${app.title || app.appId}\nServices: ${(app.services || []).join(', ') || 'none'}\nStatus: ${app.statusText || app.state}`"
            @click="openDockerDetail(app)"
          >
            <!-- Application (App Icon + Status Indicator Dot + Title + AppId with Services Tooltip) -->
            <td style="padding: 8px 10px; width: 280px; min-width: 220px; max-width: 340px">
              <div style="display: flex; align-items: center; gap: 10px">
                <!-- App Icon Container with Status Dot -->
                <div class="app-row-icon-wrap" :title="`Status: ${app.state} (${app.statusText})`">
                  <div class="app-row-icon-box">
                    <img
                      v-if="getDockerAppIcon(app) && !failedAppIcons[app.appId || app.name]"
                      :src="getDockerAppIcon(app)"
                      :alt="app.title || app.appId"
                      class="app-row-img"
                      loading="lazy"
                      @error="failedAppIcons[app.appId || app.name] = true"
                    />
                    <MdiIcon
                      v-else
                      :path="mdiDocker"
                      size="18"
                      style="color: var(--accent-primary, #38bdf8)"
                    />
                  </div>
                  <!-- Status indicator dot pinned to bottom-right of icon -->
                  <span
                    v-if="app.state === 'DEPLOYING'"
                    class="spinner-sm status-dot-badge"
                    style="width: 8px; height: 8px; border-width: 1.5px; border-color: var(--accent-primary); border-top-color: transparent"
                    title="Deploying stack in background..."
                  ></span>
                  <span
                    v-else
                    class="status-dot-badge"
                    :style="{
                      background:
                        app.state === 'RUNNING'
                          ? 'var(--success)'
                          : app.state === 'PARTIALLY_RUNNING'
                          ? 'var(--warning)'
                          : app.state === 'ERROR'
                          ? 'var(--danger)'
                          : 'var(--text-muted)'
                    }"
                  ></span>
                </div>

                <div style="min-width: 0; flex: 1">
                  <div style="display: flex; align-items: center; gap: 6px; flex-wrap: wrap">
                    <span
                      style="font-weight: 600; font-size: 13.5px; color: var(--text-primary)"
                      title="Open Application Details"
                    >
                      {{ app.title || app.appId }}
                    </span>
                    <button
                      v-if="app.updateAvailable"
                      type="button"
                      class="badge badge-warning"
                      style="font-size: 11px; padding: 1px 6px; cursor: pointer; display: inline-flex; align-items: center; gap: 3px; border: none"
                      :title="`Catalog Update Available: ${app.catalogVersion || 'installed'} → ${app.latestVersion || 'latest'}. Click to upgrade.`"
                      :aria-label="`Upgrade ${app.title || app.appId}`"
                      @click.stop="openUpgradeModal(app)"
                    >
                      <MdiIcon :path="mdiUpdate" size="11" />
                      Update: {{ app.latestVersion || 'Available' }}
                    </button>
                    <span
                      v-if="transitioningApps[app.name]"
                      class="badge badge-warning pulse-anim"
                      style="font-size: 11px; padding: 1px 6px; display: inline-flex; align-items: center; gap: 4px"
                    >
                      <MdiIcon :path="mdiLoading" size="10" class="spin-icon" />
                      {{ transitioningApps[app.name].toUpperCase() }}...
                    </span>
                    <span
                      v-else-if="app.state === 'DEPLOYING'"
                      class="badge badge-info"
                      style="font-size: 11px; padding: 1px 6px; display: inline-flex; align-items: center; gap: 4px"
                      :title="app.statusText"
                    >
                      <span class="spinner-sm" style="width: 10px; height: 10px; border-width: 1.5px"></span>
                      Deploying
                    </span>
                    <span
                      v-else-if="app.state === 'ERROR'"
                      class="badge badge-danger"
                      style="font-size: 11px; padding: 1px 6px"
                      :title="app.statusText"
                    >
                      Error
                    </span>
                  </div>
                  <div
                    v-if="app.state === 'DEPLOYING'"
                    style="font-size: 11.5px; color: var(--accent-primary); margin-top: 1px"
                    :title="app.statusText"
                  >
                    {{ app.statusText }}
                  </div>
                  <div
                    v-else-if="app.state === 'ERROR'"
                    style="font-size: 11.5px; color: var(--danger); margin-top: 1px"
                    :title="app.statusText"
                  >
                    {{ app.statusText }}
                  </div>
                  <div
                    v-else
                    style="font-size: 12px; color: var(--text-secondary)"
                    :title="`Services: ${(app.services || []).join(', ') || 'none'}`"
                  >
                    {{ app.appId }} · {{ (app.services || []).length }} service{{ (app.services || []).length === 1 ? '' : 's' }} · {{ app.statusText }}
                  </div>
                </div>
              </div>
            </td>

            <!-- Web Portal / Web UI -->
            <td style="padding: 10px 14px; width: 130px; min-width: 130px; position: relative" @click.stop>
              <div v-if="getPrimaryWebUiUrl(app)" style="display: inline-flex; align-items: center; gap: 2px">
                <button
                  class="btn btn-sm"
                  style="padding: 3px 8px; font-size: 12px; color: var(--success-text); border-color: var(--success-border); display: inline-flex; align-items: center; gap: 4px"
                  :title="`Open Web UI (${getPrimaryWebUiUrl(app)})`"
                  @click.stop="openWebPortal(getPrimaryWebUiUrl(app))"
                >
                  <MdiIcon :path="mdiOpenInNew" size="13" />
                  {{ app.portals && app.portals.length > 0 ? (app.portals[0].name || 'Open UI') : 'Open UI' }}
                </button>
                <button
                  v-if="(app.portals && app.portals.length > 1) || (!app.portals?.length && getAppWebPorts(app).length > 1)"
                  type="button"
                  class="btn btn-sm btn-icon webui-port-trigger"
                  style="padding: 3px 4px; font-size: 11.5px; color: var(--success-text); border-color: var(--success-border)"
                  title="View all portals and ports"
                  @click.stop="togglePortMenu(app.appId)"
                >
                  <MdiIcon :path="mdiChevronDown" size="14" />
                </button>
              </div>

              <span v-else style="color: var(--text-secondary); font-size: 12.5px">—</span>

              <!-- Port Selector / Portal Dropdown Menu -->
              <div
                v-if="activePortMenuAppId === app.appId"
                class="webui-port-dropdown"
                style="position: absolute; top: calc(100% - 4px); left: 16px; z-index: 100; min-width: 230px; background: var(--bg-surface); border: 1px solid var(--border-strong); border-radius: 6px; box-shadow: var(--modal-shadow); padding: 6px; display: flex; flex-direction: column; gap: 2px"
              >
                <!-- Application Portals from Manifest -->
                <template v-if="app.portals && app.portals.length > 0">
                  <div style="font-size: 11px; font-weight: 600; text-transform: uppercase; color: var(--text-secondary); padding: 4px 8px; border-bottom: 1px solid var(--border-subtle); margin-bottom: 2px">
                    Application Portals
                  </div>
                  <button
                    v-for="portal in app.portals"
                    :key="portal.name + portal.url"
                    type="button"
                    class="btn btn-sm btn-ghost"
                    style="display: flex; justify-content: space-between; align-items: center; width: 100%; padding: 6px 8px; text-align: left"
                    @click.stop="openWebPortal(portal.url)"
                  >
                    <div style="display: flex; flex-direction: column">
                      <span style="font-weight: 600; font-size: 12.5px; color: var(--text-primary)">
                        {{ portal.name }}
                      </span>
                      <span class="mono" style="font-size: 11.5px; color: var(--accent-primary)">
                        {{ portal.protocol }}://:{{ portal.port }}{{ portal.path }}
                      </span>
                    </div>
                    <MdiIcon :path="mdiOpenInNew" size="13" style="color: var(--text-secondary); flex-shrink: 0" />
                  </button>
                </template>

                <!-- Fallback Web UI Ports (when no explicit portals defined) -->
                <template v-else-if="getAppWebPorts(app).length > 0">
                  <div
                    style="font-size: 11px; font-weight: 600; text-transform: uppercase; color: var(--text-secondary); padding: 4px 8px; border-bottom: 1px solid var(--border-subtle); margin-top: 4px; margin-bottom: 2px"
                  >
                    Web UI Ports
                  </div>
                  <button
                    v-for="p in getAppWebPorts(app)"
                    :key="p.containerPort"
                    type="button"
                    class="btn btn-sm btn-ghost"
                    style="display: flex; justify-content: space-between; align-items: center; width: 100%; padding: 5px 8px; text-align: left; font-size: 12.5px"
                    @click.stop="openPortUrl(p.hostPort)"
                  >
                    <span class="mono font-semibold" style="color: var(--accent-primary)">
                      :{{ p.containerPort }}
                    </span>
                    <span style="color: var(--text-secondary); font-size: 11.5px">
                      → host :{{ p.hostPort }}
                    </span>
                    <span v-if="isPortActiveWebUi(app, p)" class="badge badge-success" style="font-size: 10.5px; padding: 1px 4px">
                      UI
                    </span>
                  </button>
                </template>
              </div>
            </td>

            <!-- CPU -->
            <td class="col-desktop-only" style="padding: 10px 14px; width: 85px; min-width: 85px; max-width: 85px; box-sizing: border-box; white-space: nowrap">
              <div style="font-size: 13.5px; font-weight: 600; color: var(--text-primary); font-variant-numeric: tabular-nums">
                {{ (Number(app.totalCpuPercent) || 0).toFixed(1) }}%
              </div>
            </td>

            <!-- Memory -->
            <td class="col-desktop-only" style="padding: 10px 14px; width: 105px; min-width: 105px; max-width: 105px; box-sizing: border-box; white-space: nowrap">
              <div style="font-size: 13.5px; color: var(--text-primary); font-variant-numeric: tabular-nums">
                {{ app.totalMemoryMib }} MiB
              </div>
            </td>

            <!-- Network (1s Live) -->
            <td class="col-desktop-only" style="padding: 10px 14px; width: 210px; min-width: 210px; max-width: 210px; box-sizing: border-box; white-space: nowrap">
              <div style="font-size: 12.5px; font-variant-numeric: tabular-nums">
                <div style="color: var(--text-primary); font-weight: 500">
                  {{ formatNetworkRate(app) }}
                </div>
                <div style="font-size: 11.5px; color: var(--text-secondary); min-height: 14px">
                  {{ formatNetworkTotal(app) || ' ' }}
                </div>
              </div>
            </td>

            <!-- Quick Power Toggle Action -->
            <td style="padding: 10px 14px; text-align: right" @click.stop>
              <div style="display: flex; gap: 6px; justify-content: flex-end; align-items: center; white-space: nowrap; flex-shrink: 0">
                <button
                  v-if="app.state === 'DEPLOYING'"
                  class="btn btn-xs"
                  style="flex-shrink: 0; opacity: 0.6; cursor: not-allowed"
                  title="Deployment in progress in background..."
                  disabled
                >
                  <span class="spinner-sm" style="width: 12px; height: 12px; border-width: 2px; margin-right: 4px"></span>
                  Deploying...
                </button>
                <button
                  v-else-if="app.state === 'RUNNING' || transitioningApps[app.name] === 'stopping'"
                  class="btn btn-xs"
                  :class="{ 'btn-loading': transitioningApps[app.name] === 'stopping' }"
                  :disabled="Boolean(transitioningApps[app.name])"
                  style="flex-shrink: 0"
                  :title="transitioningApps[app.name] === 'stopping' ? 'Stopping application...' : 'Stop Application'"
                  aria-label="Stop Application"
                  @click.stop="stopApp(app)"
                >
                  <MdiIcon
                    :path="transitioningApps[app.name] === 'stopping' ? mdiLoading : mdiStop"
                    :class="{ 'spin-icon': transitioningApps[app.name] === 'stopping' }"
                    size="13"
                    style="margin-right: 4px"
                  />
                  {{ transitioningApps[app.name] === 'stopping' ? 'Stopping...' : 'Stop' }}
                </button>
                <button
                  v-else
                  class="btn btn-xs btn-success"
                  :class="{ 'btn-loading': transitioningApps[app.name] === 'starting' }"
                  :disabled="Boolean(transitioningApps[app.name])"
                  style="flex-shrink: 0"
                  :title="transitioningApps[app.name] === 'starting' ? 'Starting application...' : 'Start Application'"
                  aria-label="Start Application"
                  @click.stop="startApp(app)"
                >
                  <MdiIcon
                    :path="transitioningApps[app.name] === 'starting' ? mdiLoading : mdiPlay"
                    :class="{ 'spin-icon': transitioningApps[app.name] === 'starting' }"
                    size="13"
                    style="margin-right: 4px"
                  />
                  {{ transitioningApps[app.name] === 'starting' ? 'Starting...' : 'Start' }}
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
    <div v-if="isInstallModalOpen" class="modal-backdrop">
      <div class="modal-card modal-card-xlarge">
        <!-- Header -->
        <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 14px; flex-shrink: 0">
          <div>
            <h3 style="font-size: 18px; font-weight: 700; color: var(--text-primary)">
              Launch App
            </h3>
          </div>
          <button class="btn btn-sm" style="padding: 2px 6px" @click="isInstallModalOpen = false">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>

        <!-- Quick Pick Catalog Templates Bar -->
        <div style="background: rgba(0, 0, 0, 0.25); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 10px 12px; margin-bottom: 14px; flex-shrink: 0">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
            <span style="font-size: 11.5px; color: var(--text-secondary); font-weight: 600; text-transform: uppercase">
              Templates
            </span>
            <button
              class="btn btn-sm"
              style="padding: 2px 8px; font-size: 11.5px; display: inline-flex; align-items: center; gap: 4px; color: var(--accent-primary)"
              @click="isInstallModalOpen = false; openCatalogModal()"
            >
              <MdiIcon :path="mdiStorefrontOutline" size="14" />
              <span>Browse Catalog...</span>
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
              <span>Blank</span>
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
            :is-saving="isDeployingApp"
            :show-cancel="true"
            @save="handleDeployApp"
            @cancel="isInstallModalOpen = false"
          />
        </div>
      </div>
    </div>

    <!-- ===================================================================== -->
    <!-- MODAL 2.5: Upgrade App Modal                                          -->
    <!-- ===================================================================== -->
    <div v-if="isUpgradeModalOpen && upgradingApp" class="modal-backdrop">
      <div class="modal-card" style="max-width: 520px">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px">
          <div style="display: flex; align-items: center; gap: 8px">
            <MdiIcon :path="mdiArrowUpBoldCircleOutline" size="22" style="color: var(--accent-primary)" />
            <h3 style="font-size: 18px; font-weight: 700; color: var(--text-primary); margin: 0">
              Upgrade: {{ upgradingApp.title || upgradingApp.appId }}
            </h3>
          </div>
          <button class="btn btn-sm btn-icon" @click="isUpgradeModalOpen = false">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>

        <div style="font-size: 13px; color: var(--text-secondary); line-height: 1.5; margin-bottom: 16px">
          This will pull the latest container images for the stack from the registry and restart the application with zero orphaned containers.
          <div v-if="upgradingApp.updateAvailable" style="margin-top: 10px; padding: 8px 12px; background: var(--bg-surface-elevated, var(--bg-hover)); border-radius: 6px; border: 1px solid var(--border-subtle)">
            <div style="font-size: 12px; font-weight: 600; color: var(--text-primary)">
              Catalog Version Information
            </div>
            <div style="font-size: 12px; margin-top: 2px">
              Installed: <span class="mono">{{ upgradingApp.catalogVersion || 'unspecified' }}</span> →
              Latest: <span class="mono" style="color: var(--accent-primary); font-weight: 600">{{ upgradingApp.latestVersion }}</span>
            </div>
            <div v-if="upgradingApp.latestImage" style="font-size: 11px; color: var(--text-muted); margin-top: 2px">
              Image: <span class="mono">{{ upgradingApp.latestImage }}</span>
            </div>
          </div>
        </div>

        <!-- Snapshot Checkbox -->
        <label style="display: flex; align-items: flex-start; gap: 8px; cursor: pointer; margin-bottom: 20px; padding: 10px 12px; border: 1px solid var(--border-subtle); border-radius: 6px; background: var(--bg-surface)">
          <input type="checkbox" v-model="upgradeTakeSnapshot" style="margin-top: 2px" />
          <div style="font-size: 12.5px">
            <span style="font-weight: 600; color: var(--text-primary)">Create pre-upgrade ZFS snapshot</span>
            <div style="font-size: 11px; color: var(--text-secondary); margin-top: 2px">
              Automatically snapshots app storage datasets on ZFS before pulling and restarting, allowing instant rollback if needed.
            </div>
          </div>
        </label>

        <!-- Actions -->
        <div style="display: flex; justify-content: flex-end; gap: 8px">
          <button class="btn btn-secondary" :disabled="isUpgradingApp" @click="isUpgradeModalOpen = false">
            Cancel
          </button>
          <button
            class="btn btn-primary"
            :disabled="isUpgradingApp"
            style="display: flex; align-items: center; gap: 6px"
            @click="handleConfirmUpgrade"
          >
            <MdiIcon v-if="isUpgradingApp" :path="mdiLoading" size="16" class="spin-icon" />
            <MdiIcon v-else :path="mdiArrowUpBoldCircleOutline" size="16" />
            <span>{{ isUpgradingApp ? 'Upgrading...' : 'Confirm Upgrade' }}</span>
          </button>
        </div>
      </div>
    </div>

    <!-- ===================================================================== -->
    <!-- MODAL: TrueNAS Application Catalog Modal                              -->
    <!-- ===================================================================== -->
    <div v-if="isCatalogModalOpen" class="modal-backdrop">
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
                  App Catalog
                </h3>
                <span class="badge badge-info" style="font-size: 11px">
                  {{ catalogApps.length }} Apps
                </span>
              </div>
              <p v-if="catalogLastSyncTime" style="font-size: 12.5px; color: var(--text-secondary); margin: 3px 0 0 0">
                Last synced: {{ formatSyncTime(catalogLastSyncTime) }}
              </p>
            </div>
          </div>

          <div style="display: flex; align-items: center; gap: 10px">
            <button
              class="btn btn-sm"
              :disabled="isSyncingCatalog"
              title="Sync latest app definitions"
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
              All Catalogs
            </button>
            <button
              class="btn btn-sm"
              :class="catalogSelectedTrain === 'stable' ? 'btn-primary' : ''"
              style="padding: 4px 10px; font-size: 12px"
              @click="catalogSelectedTrain = 'stable'"
            >
              TrueNAS
            </button>
            <button
              class="btn btn-sm"
              :class="catalogSelectedTrain === 'casaos' ? 'btn-primary' : ''"
              style="padding: 4px 10px; font-size: 12px"
              @click="catalogSelectedTrain = 'casaos'"
            >
              CasaOS
            </button>
            <button
              class="btn btn-sm"
              :class="catalogSelectedTrain === 'dokploy' ? 'btn-primary' : ''"
              style="padding: 4px 10px; font-size: 12px"
              @click="catalogSelectedTrain = 'dokploy'"
            >
              Dokploy
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
                        :class="{
                          'badge-success': app.train === 'stable',
                          'badge-info': app.train === 'community',
                          'badge-warning': app.train === 'casaos',
                          'badge-primary': app.train === 'dokploy',
                        }"
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
    <!-- MODAL: TrueNAS SCALE App Configuration Wizard                         -->
    <!-- ===================================================================== -->
    <DockerRegistriesModal v-if="isRegistriesModalOpen" @close="isRegistriesModalOpen = false" />

    <CatalogQuestionWizard
      v-if="isQuestionWizardOpen && wizardCatalogApp"
      :app="wizardCatalogApp"
      :apps-pool="(nasState.systemConfig?.mainPool || (nasState.dockerConfig?.availablePools?.[0] || 'tank')).replace(/^zfsPools\//, '').trim()"
      :is-deploying="isDeployingApp"
      @close="isQuestionWizardOpen = false"
      @open-in-editor="handleWizardOpenInEditor"
      @deploy="handleWizardDeploy"
      @skip-wizard="handleWizardSkip"
    />

    <!-- ===================================================================== -->
    <!-- MODAL: Docker Storage Dataset Configuration                           -->
    <!-- ===================================================================== -->
    <div v-if="isDatasetModalOpen" class="modal-backdrop">
      <div class="modal-card" style="max-width: 540px">
        <!-- Header -->
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; padding-bottom: 12px; border-bottom: 1px solid var(--border-subtle)">
          <div style="display: flex; align-items: center; gap: 10px">
            <MdiIcon :path="mdiFolderOutline" size="22" style="color: var(--accent-primary)" />
            <h3 style="font-size: 18px; font-weight: 700; color: var(--text-primary); margin: 0">
              Storage Dataset
            </h3>
          </div>
          <button class="btn btn-sm" style="padding: 2px 6px" @click="isDatasetModalOpen = false">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>

        <p style="font-size: 13px; color: var(--text-secondary); margin-bottom: 16px; line-height: 1.5">
          Dataset on the main pool (<span class="mono" style="color: var(--accent-primary)">{{ (nasState.systemConfig?.mainPool || 'tank').replace(/^zfsPools\//, '').trim() }}</span>) where container images and app volumes are stored.
        </p>

        <!-- Dataset Selector Form -->
        <div class="form-group" style="margin-bottom: 16px">
          <label style="display: block; font-size: 12px; font-weight: 600; color: var(--text-secondary); margin-bottom: 6px; text-transform: uppercase">
            Dataset
          </label>
          <select v-model="selectedDatasetToSave" class="form-input" style="width: 100%; margin-bottom: 10px">
            <option v-for="d in availableDatasetNames" :key="d" :value="d">
              {{ d }}
            </option>
          </select>

          <label style="display: block; font-size: 12px; font-weight: 600; color: var(--text-secondary); margin-bottom: 6px; text-transform: uppercase">
            Or Custom Path ({{ (nasState.systemConfig?.mainPool || 'tank').replace(/^zfsPools\//, '').trim() }}/dataset)
          </label>
          <input
            v-model="selectedDatasetToSave"
            type="text"
            class="form-input"
            style="width: 100%"
            :placeholder="`e.g. ${(nasState.systemConfig?.mainPool || 'tank').replace(/^zfsPools\//, '').trim()}/apps`"
          />
        </div>

        <!-- Path Preview Card -->
        <div style="background: rgba(0,0,0,0.25); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 12px 14px; margin-bottom: 20px">
          <div style="font-size: 11.5px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; margin-bottom: 8px">
            Paths
          </div>
          <div style="display: flex; flex-direction: column; gap: 6px; font-size: 12.5px" class="mono">
            <div>
              <span style="color: var(--text-secondary)">ZFS Dataset:</span>
              <span style="color: var(--accent-primary); margin-left: 6px">{{ selectedDatasetToSave.replace(/^zfsPools\//, '') }}</span>
            </div>
            <div>
              <span style="color: var(--text-secondary)">Mountpoint:</span>
              <span style="color: var(--accent-primary); margin-left: 6px">/mnt/{{ selectedDatasetToSave.replace(/^zfsPools\//, '').replace(/^\//, '') }}</span>
            </div>
            <div>
              <span style="color: var(--text-secondary)">App Volumes:</span>
              <span style="color: var(--accent-primary); margin-left: 6px">/mnt/{{ selectedDatasetToSave.replace(/^zfsPools\//, '').replace(/^\//, '') }}/appdata</span>
            </div>
            <div>
              <span style="color: var(--text-secondary)">Docker Data Root:</span>
              <span style="color: var(--accent-primary); margin-left: 6px">/mnt/{{ selectedDatasetToSave.replace(/^zfsPools\//, '').replace(/^\//, '') }}/docker</span>
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
            <span>{{ isSavingDataset ? 'Saving...' : 'Save' }}</span>
          </button>
        </div>
      </div>
    </div>

  </div>
</template>

<style scoped>
.app-row-icon-wrap {
  position: relative;
  width: 32px;
  height: 32px;
  flex-shrink: 0;
}

.app-row-icon-box {
  width: 32px;
  height: 32px;
  border-radius: 6px;
  background: var(--bg-surface-elevated, rgba(255, 255, 255, 0.04));
  border: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

.app-row-img {
  width: 24px;
  height: 24px;
  object-fit: contain;
  border-radius: 3px;
}

.status-dot-badge {
  position: absolute;
  bottom: -2px;
  right: -2px;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  border: 1.5px solid var(--bg-surface, #1e293b);
  box-sizing: content-box;
}
</style>
