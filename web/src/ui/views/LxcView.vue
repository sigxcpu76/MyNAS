<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import {
  mdiApps,
  mdiArchiveOutline,
  mdiClose,
  mdiConsole,
  mdiContentCopy,
  mdiDownload,
  mdiFileDocumentOutline,
  mdiFilterVariant,
  mdiFolder,
  mdiInformationOutline,
  mdiLayers,
  mdiMagnify,
  mdiOpenInNew,
  mdiPause,
  mdiPencil,
  mdiPlay,
  mdiPlus,
  mdiRefresh,
  mdiRestart,
  mdiServer,
  mdiSpeedometer,
  mdiStop,
  mdiTrashCan,
  mdiTrashCanOutline,
  mdiUpload,
} from '@mdi/js';
import MdiIcon from '../components/MdiIcon.vue';
import SkeletonBox from '../components/SkeletonBox.vue';
import SortableTh from '../components/SortableTh.vue';
import FileDropzone from '../components/FileDropzone.vue';
import PathSelector from '../components/PathSelector.vue';
import DockerTerminalModal, { type TerminalContainerOption } from '../components/DockerTerminalModal.vue';
import { useTableSort } from '../composables/useTableSort';
import { services } from '../../container';
import { nasState, notify, refreshLxcTemplates as storeRefreshTemplates } from '../store';
import { confirmDelete } from '../modal';
import { toZfsSafeSlug } from '../../domain/id';
import { formatBytes, type LxcCatalogItem, type LxcContainer, type LxcMountPoint, type StorageImage } from '../../domain/models';

/** Converts a dotted IPv4 address into a sortable integer (null when absent/invalid). */
function ipv4ToNumber(ip: string | undefined): number | null {
  const parts = (ip || '').trim().split('/')[0].split('.');
  if (parts.length !== 4) return null;
  let n = 0;
  for (const part of parts) {
    const octet = Number(part);
    if (!Number.isInteger(octet) || octet < 0 || octet > 255) return null;
    n = n * 256 + octet;
  }
  return n;
}

// =============================================================================
// Top Navigation Subtabs
// =============================================================================
const activeMainTab = ref<'containers' | 'catalog' | 'templates'>('containers');

function switchTab(tab: 'containers' | 'catalog' | 'templates') {
  activeMainTab.value = tab;
  if (tab === 'catalog' && catalogItems.value.length === 0 && !isLoadingCatalog.value) {
    loadCatalog();
  }
}

function cleanPoolName(name: string | undefined | null): string {
  if (!name) return '';
  return name.replace(/^zfsPools\//, '').trim();
}

// Main storage pool and templates directory
const mainPool = computed(() => {
  const p =
    cleanPoolName(nasState.systemConfig?.mainPool) ||
    cleanPoolName(nasState.selectedPoolName) ||
    cleanPoolName(nasState.pools?.[0]?.poolName) ||
    cleanPoolName(nasState.pools?.[0]?.name) ||
    'tank';
  return p || 'tank';
});
const lxcTemplatesDir = computed(() => `/mnt/${mainPool.value}/lxc-templates`);

/**
 * Computes a unique, collision-free, ZFS-safe template archive filename for catalog items.
 * If upstream has generic names like "rootfs.tar.xz", names it after the template (e.g. debian-bookworm-amd64-rootfs.tar.xz).
 */
function computeSafeTemplateFilename(item: LxcCatalogItem): string {
  if (!item.downloadUrl) return '';

  const cleanUrl = item.downloadUrl.split('?')[0].split('#')[0];
  let ext = '.tar.xz';
  const matchExt = cleanUrl.match(/\.(tar\.(?:xz|gz|zst|bz2)|tgz|txz|tar)$/i);
  if (matchExt) {
    ext = matchExt[0].toLowerCase();
  }

  const parts = cleanUrl.split('/');
  const rawBase = parts[parts.length - 1] || '';
  const nameWithoutExt = rawBase.replace(/\.(tar\.(?:xz|gz|zst|bz2)|tgz|txz|tar)$/i, '');

  const genericNames = new Set(['rootfs', 'system', 'template', 'image', 'archive', 'download', 'base', 'default', '']);
  const isGeneric = genericNames.has(nameWithoutExt.toLowerCase().trim());

  if (isGeneric) {
    const baseSlug = toZfsSafeSlug(item.title || item.id || 'template');
    const arch = item.architecture ? toZfsSafeSlug(item.architecture) : '';
    const fullSlug = arch && !baseSlug.includes(arch) ? `${baseSlug}-${arch}` : baseSlug;
    return `${fullSlug}-rootfs${ext}`;
  }

  const safeSlug = toZfsSafeSlug(nameWithoutExt);
  return `${safeSlug}${ext}`;
}

interface BridgeOption {
  name: string;
  label: string;
}

const defaultBridge = computed<string>(() => {
  const net = nasState.systemTelemetry?.network;
  if (!net || !Array.isArray(net)) return 'br0';
  const br = net.find((n: any) => n.name === 'br0' || n.name === 'vmbr0');
  if (br) return br.name;
  const def = net.find((n: any) => n.isDefault);
  if (def) return def.name;
  const firstEth = net.find(
    (n: any) =>
      !n.name.startsWith('lo') &&
      !n.name.startsWith('docker') &&
      !n.name.startsWith('br-') &&
      !n.name.startsWith('veth') &&
      !n.name.startsWith('vnet') &&
      !n.name.startsWith('tap') &&
      !n.name.startsWith('lxc')
  );
  if (firstEth) return firstEth.name;
  return 'br0';
});

const hostBridgeInterfaces = computed<BridgeOption[]>(() => {
  const net = nasState.systemTelemetry?.network;
  if (!net || !Array.isArray(net)) {
    return [
      { name: 'br0', label: 'br0 (main bridge)' },
      { name: 'lxcbr0', label: 'lxcbr0' },
    ];
  }
  const list = net
    .filter(
      (n: any) =>
        !n.name.startsWith('lo') &&
        !n.name.startsWith('docker') &&
        !n.name.startsWith('br-') &&
        !n.name.startsWith('veth') &&
        !n.name.startsWith('vnet') &&
        !n.name.startsWith('tap')
    )
    .map((n: any) => {
      const ip = n.ipAddresses?.find((a: string) => !a.includes(':')) || n.ipAddresses?.[0];
      const parts = [n.isUp ? 'up' : 'down'];
      if (ip) parts.push(ip);
      if (n.isDefault) parts.push('default route');
      return { name: n.name, label: `${n.name} (${parts.join(', ')})` };
    });
  if (!list.some((b) => b.name === 'br0')) {
    list.unshift({ name: 'br0', label: 'br0 (main bridge)' });
  }
  return list;
});

function bridgeOptionsFor(current: string): BridgeOption[] {
  const opts = [...hostBridgeInterfaces.value];
  if (current && !opts.some((o) => o.name === current)) {
    opts.unshift({ name: current, label: `${current} (custom)` });
  }
  return opts;
}

const lxcSort = useTableSort(() => nasState.lxcContainers, {
  name: (lx) => lx.containerName,
  state: (lx) => lx.state,
  autostart: (lx) => (lx.autostart ? 1 : 0),
  distro: (lx) => `${lx.distribution} / ${lx.release} (${lx.architecture})`,
  ipv4: (lx) => ipv4ToNumber(lx.ipv4Address),
  // Primary key: CPU cores, secondary: memory limit (MiB)
  resources: (lx) => lx.cpuCores * 1e9 + lx.memoryLimitMib,
});

const templateSort = useTableSort(() => nasState.lxcTemplates, {
  name: (t) => t.name,
  size: (t) => t.sizeBytes,
  modified: (t) => (t.modTime ? new Date(t.modTime) : null),
});

// =============================================================================
// App Catalog State & Filters
// =============================================================================
const catalogItems = ref<LxcCatalogItem[]>([]);
const isLoadingCatalog = ref(false);
const isSyncingCatalog = ref(false);
const catalogSourceFilter = ref<string>('all');
const catalogCategoryFilter = ref<string>('all');
const catalogQuery = ref<string>('');
const catalogLastSync = ref<string>('');

const catalogCategories = computed(() => {
  const cats = new Set<string>();
  for (const item of catalogItems.value) {
    if (item.category) cats.add(item.category);
  }
  return Array.from(cats).sort();
});

const filteredCatalogItems = computed(() => {
  const q = catalogQuery.value.trim().toLowerCase();
  const src = catalogSourceFilter.value;
  const cat = catalogCategoryFilter.value;

  return catalogItems.value.filter((item) => {
    if (src !== 'all' && item.source !== src) return false;
    if (cat !== 'all' && item.category !== cat) return false;
    if (q) {
      const matchTitle = item.title.toLowerCase().includes(q);
      const matchDesc = item.description.toLowerCase().includes(q);
      const matchId = item.id.toLowerCase().includes(q);
      const matchBase = (item.baseDistribution || '').toLowerCase().includes(q);
      if (!matchTitle && !matchDesc && !matchId && !matchBase) return false;
    }
    return true;
  });
});

async function loadCatalog() {
  isLoadingCatalog.value = true;
  try {
    const res = await services.lxc.listCatalogItems();
    catalogItems.value = res?.items ?? (Array.isArray(res) ? res : []);
    catalogLastSync.value = res?.lastSyncTime ?? '';
  } catch (err: any) {
    console.error('Failed to load LXC catalog:', err);
  } finally {
    isLoadingCatalog.value = false;
  }
}

async function handleSyncCatalog() {
  isSyncingCatalog.value = true;
  try {
    const res = await services.lxc.syncCatalog(true);
    notify('success', res.message || `Successfully synced ${res.totalItems} catalog items`);
    catalogLastSync.value = res.lastSyncTime;
    await loadCatalog();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to sync LXC catalog');
  } finally {
    isSyncingCatalog.value = false;
  }
}

// =============================================================================
// Create / Deploy Container State & Modal
// =============================================================================
const showCreate = ref(false);
const isDeploying = ref(false);
const deployingCatalogItem = ref<LxcCatalogItem | null>(null);

const form = reactive({
  containerName: '',
  containerId: '',
  distribution: 'debian',
  release: 'bookworm',
  architecture: 'amd64',
  memoryLimitMib: 1024,
  cpuCores: 2,
  networkBridge: defaultBridge.value || 'br0',
  unprivileged: true,
  autostart: true,
  gpuEnabled: false,
  description: '',
  templateArchive: '',
  mountPoints: [] as LxcMountPoint[],
});

function onContainerNameInput() {
  form.containerId = toZfsSafeSlug(form.containerName);
}

function onDistributionChange() {
  if (!form.containerName) {
    form.containerId = toZfsSafeSlug(`${form.distribution}-${form.release}`);
  }
}

function addCreateMount() {
  form.mountPoints.push({
    hostPath: '',
    containerPath: '',
    readOnly: false,
  });
}

function removeCreateMount(index: number) {
  form.mountPoints.splice(index, 1);
}

function toggleCreate() {
  if (showCreate.value) {
    showCreate.value = false;
    deployingCatalogItem.value = null;
  } else {
    deployingCatalogItem.value = null;
    form.containerName = 'Debian Container';
    form.distribution = 'debian';
    form.release = 'bookworm';
    form.containerId = toZfsSafeSlug(form.containerName);
    form.architecture = 'amd64';
    form.memoryLimitMib = 1024;
    form.cpuCores = 2;
    form.networkBridge = defaultBridge.value || 'br0';
    form.unprivileged = true;
    form.autostart = true;
    form.gpuEnabled = false;
    form.description = '';
    form.templateArchive = '';
    form.mountPoints = [];
    showCreate.value = true;
  }
}

function openDeployModal(item: LxcCatalogItem) {
  deployingCatalogItem.value = item;
  form.containerName = item.title;
  form.containerId = toZfsSafeSlug(item.title) || toZfsSafeSlug(item.id.replace(/^(community|turnkey|lxc)-/, ''));
  form.distribution = item.baseDistribution || 'debian';
  form.release = item.baseRelease || 'bookworm';
  form.architecture = item.architecture || 'amd64';
  form.cpuCores = item.defaultCpuCores || 2;
  form.memoryLimitMib = item.defaultMemoryLimitMib || 1024;
  form.networkBridge = defaultBridge.value || 'br0';
  form.unprivileged = !item.privilegedRequired;
  form.autostart = true;
  form.gpuEnabled = Boolean(item.gpuRecommended);
  form.description = item.description || '';
  form.templateArchive = '';

  // Look up matching template if already downloaded
  if (nasState.lxcTemplates.length > 0) {
    const matched = nasState.lxcTemplates.find(
      (t) =>
        t.name.toLowerCase().includes(item.id.toLowerCase()) ||
        t.name.toLowerCase().includes(item.baseDistribution.toLowerCase())
    );
    if (matched) {
      form.templateArchive = matched.path;
    }
  }

  // Pre-fill archive filename in lxc-templates directory if downloadUrl is provided
  if (!form.templateArchive && item.downloadUrl) {
    const safeFname = computeSafeTemplateFilename(item);
    form.templateArchive = `${lxcTemplatesDir.value}/${safeFname}`;
  }

  // Pre-fill recommended mount points with the active main pool
  form.mountPoints = (item.recommendedMounts || []).map((cp) => ({
    hostPath: `/mnt/${mainPool.value}${cp}`,
    containerPath: cp,
    readOnly: false,
  }));

  showCreate.value = true;
  switchTab('containers');
}

async function handleCreate() {
  isDeploying.value = true;
  try {
    const targetId = toZfsSafeSlug(form.containerId || form.containerName || `${form.distribution}-${form.release}`) || 'container';
    const validMounts = form.mountPoints.filter((mp) => mp.hostPath.trim() && mp.containerPath.trim());
    const created = await services.lxc.createContainer(targetId, {
      containerName: form.containerName || targetId,
      distribution: form.distribution,
      release: form.release,
      architecture: form.architecture,
      memoryLimitMib: form.memoryLimitMib,
      cpuCores: form.cpuCores,
      networkBridge: form.networkBridge.trim() || defaultBridge.value || 'br0',
      unprivileged: form.unprivileged,
      autostart: form.autostart,
      gpuEnabled: form.gpuEnabled,
      description: form.description.trim(),
      templateArchive: form.templateArchive.trim() || undefined,
      mountPoints: validMounts,
      installRecipe: deployingCatalogItem.value?.installRecipe || undefined,
      installScriptUrl: deployingCatalogItem.value?.installScriptUrl || undefined,
      templateDownloadUrl: deployingCatalogItem.value?.downloadUrl || undefined,
    });

    // Start container automatically upon deployment
    try {
      await services.lxc.startContainer(created.name);
    } catch {
      // Ignored if start requires manual configuration
    }

    notify('success', `Deployed and initialized LXC container ${created.containerName || targetId}`);
    form.containerName = '';
    form.containerId = '';
    form.templateArchive = '';
    form.mountPoints = [];
    showCreate.value = false;
    deployingCatalogItem.value = null;
    activeMainTab.value = 'containers';
    await refreshLxc();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create LXC container');
  } finally {
    isDeploying.value = false;
  }
}

// =============================================================================
// Autostart Toggle
// =============================================================================
const togglingAutostart = ref<Record<string, boolean>>({});

async function toggleAutostart(lx: LxcContainer) {
  if (togglingAutostart.value[lx.name]) return;
  togglingAutostart.value[lx.name] = true;
  try {
    const updated = await services.lxc.updateContainer(
      lx.name,
      { autostart: !lx.autostart },
      ['autostart']
    );
    lx.autostart = updated.autostart;
    notify('success', `Autostart ${updated.autostart ? 'enabled' : 'disabled'} for ${lx.containerName}`);
    if (inspectedContainer.value && inspectedContainer.value.name === lx.name) {
      inspectedContainer.value.autostart = updated.autostart;
    }
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update autostart setting');
  } finally {
    togglingAutostart.value[lx.name] = false;
  }
}

// =============================================================================
// Container Lifecycle & Actions
// =============================================================================
async function handleAction(name: string, verb: 'start' | 'stop' | 'freeze') {
  try {
    if (verb === 'start') await services.lxc.startContainer(name);
    else if (verb === 'stop') await services.lxc.stopContainer(name);
    else await services.lxc.freezeContainer(name);
    notify('success', `LXC :${verb} succeeded on ${name}`);
    await refreshLxc();
  } catch (err: any) {
    notify('error', err?.message || `Failed to ${verb} LXC container`);
  }
}

async function handleRestart(lx: LxcContainer) {
  try {
    await services.lxc.restartContainer(lx.name);
    notify('success', `Restarted LXC container ${lx.containerName}`);
    await refreshLxc();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to restart LXC container');
  }
}

async function handleDelete(lx: LxcContainer) {
  const name = lx.name;
  const shortName = name.replace(/^lxcContainers\//, '');
  const res = await confirmDelete({
    kind: 'Container',
    name: lx.containerName || shortName,
    details: 'The container will be stopped and its rootfs and configuration deleted.',
    confirmText: 'Delete Container',
  });
  if (!res.confirmed) return;

  try {
    await services.lxc.deleteContainer(name, true);
    notify('info', `Deleted LXC container ${shortName}`);
    if (inspectedContainer.value && inspectedContainer.value.name === name) {
      isInspectModalOpen.value = false;
    }
    await refreshLxc();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete LXC container');
  }
}

// =============================================================================
// Edit Container Modal
// =============================================================================
const isEditModalOpen = ref(false);
const isSavingEdit = ref(false);
const editingContainer = ref<LxcContainer | null>(null);
const editForm = reactive({
  containerName: '',
  description: '',
  cpuCores: 2,
  memoryLimitMib: 1024,
  networkBridge: defaultBridge.value || 'br0',
  autostart: true,
  unprivileged: true,
  gpuEnabled: false,
  mountPoints: [] as LxcMountPoint[],
});

function openEditModal(lx: LxcContainer) {
  editingContainer.value = lx;
  editForm.containerName = lx.containerName || lx.name.replace(/^lxcContainers\//, '');
  editForm.description = lx.description || '';
  editForm.cpuCores = lx.cpuCores || 2;
  editForm.memoryLimitMib = lx.memoryLimitMib || 1024;
  editForm.networkBridge = lx.networkBridge || defaultBridge.value || 'br0';
  editForm.autostart = Boolean(lx.autostart);
  editForm.unprivileged = Boolean(lx.unprivileged);
  editForm.gpuEnabled = Boolean(lx.gpuEnabled);
  editForm.mountPoints = (lx.mountPoints || []).map((mp) => ({
    hostPath: mp.hostPath,
    containerPath: mp.containerPath,
    readOnly: Boolean(mp.readOnly),
  }));
  isEditModalOpen.value = true;
}

function addEditMount() {
  editForm.mountPoints.push({
    hostPath: '',
    containerPath: '',
    readOnly: false,
  });
}

function removeEditMount(index: number) {
  editForm.mountPoints.splice(index, 1);
}

async function handleSaveEdit() {
  if (!editingContainer.value) return;
  isSavingEdit.value = true;
  try {
    const validMounts = editForm.mountPoints.filter((mp) => mp.hostPath.trim() && mp.containerPath.trim());
    const updated = await services.lxc.updateContainer(editingContainer.value.name, {
      containerName: editForm.containerName.trim(),
      description: editForm.description.trim(),
      cpuCores: editForm.cpuCores,
      memoryLimitMib: editForm.memoryLimitMib,
      networkBridge: editForm.networkBridge.trim() || defaultBridge.value || 'br0',
      autostart: editForm.autostart,
      unprivileged: editForm.unprivileged,
      gpuEnabled: editForm.gpuEnabled,
      mountPoints: validMounts,
    });
    notify('success', `Updated configuration for ${updated.containerName}`);
    isEditModalOpen.value = false;
    await refreshLxc();
    if (inspectedContainer.value && inspectedContainer.value.name === updated.name) {
      inspectedContainer.value = updated;
    }
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update container configuration');
  } finally {
    isSavingEdit.value = false;
  }
}

// =============================================================================
// Container Inspector Modal & Logs
// =============================================================================
const isInspectModalOpen = ref(false);
const inspectedContainer = ref<LxcContainer | null>(null);
const inspectActiveTab = ref<'overview' | 'mounts' | 'logs'>('overview');
const logTail = ref(100);
const logsContent = ref('');
const isLoadingLogs = ref(false);

function openInspectModal(lx: LxcContainer, defaultTab: 'overview' | 'mounts' | 'logs' = 'overview') {
  inspectedContainer.value = lx;
  inspectActiveTab.value = defaultTab;
  isInspectModalOpen.value = true;
  if (defaultTab === 'logs') {
    fetchLogs();
  }
}

async function fetchLogs() {
  if (!inspectedContainer.value) return;
  isLoadingLogs.value = true;
  try {
    const logs = await services.lxc.getContainerLogs(inspectedContainer.value.name, logTail.value);
    logsContent.value = logs || 'No log entries available for this container.';
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

// =============================================================================
// Interactive Web Terminal Modal
// =============================================================================
const isTerminalOpen = ref(false);
const activeTerminalContainer = ref<LxcContainer | null>(null);

function openTerminal(lx: LxcContainer) {
  if (lx.state !== 'RUNNING') {
    notify('info', `Container "${lx.containerName}" is ${lx.state}. Start the container to access console.`);
    return;
  }
  activeTerminalContainer.value = lx;
  isTerminalOpen.value = true;
}

const terminalContainerOptions = computed<TerminalContainerOption[]>(() => {
  if (!activeTerminalContainer.value) return [];
  const id = activeTerminalContainer.value.name.replace(/^lxcContainers\//, '');
  return [
    {
      id,
      name: activeTerminalContainer.value.containerName || id,
      state: activeTerminalContainer.value.state,
    },
  ];
});

// =============================================================================
// Polling & Lifecycle
// =============================================================================
const isRefreshingContainers = ref(false);

async function refreshLxc() {
  if (isRefreshingContainers.value) return;
  isRefreshingContainers.value = true;
  try {
    nasState.lxcContainers = await services.lxc.listContainers();
  } catch (err: any) {
    console.error('Failed to list LXC containers:', err);
  } finally {
    isRefreshingContainers.value = false;
  }
}

let isViewActive = false;
let isPolling = false;
let pollTimer: ReturnType<typeof setTimeout> | null = null;

async function runContainerLoop() {
  if (!isViewActive) return;
  if (!document.hidden && activeMainTab.value === 'containers' && !isPolling) {
    isPolling = true;
    try {
      nasState.lxcContainers = await services.lxc.listContainers();
      if (inspectedContainer.value) {
        const found = nasState.lxcContainers.find((c) => c.name === inspectedContainer.value?.name);
        if (found) inspectedContainer.value = found;
      }
    } catch {
      // background polling error ignored
    } finally {
      isPolling = false;
    }
  }
  if (isViewActive) {
    pollTimer = setTimeout(runContainerLoop, 5000);
  }
}

onMounted(async () => {
  isViewActive = true;
  await refreshLxc();
  refreshTemplates().catch(() => {});
  runContainerLoop();
});

onUnmounted(() => {
  isViewActive = false;
  if (pollTimer) {
    clearTimeout(pollTimer);
    pollTimer = null;
  }
});

// =============================================================================
// Templates Repository
// =============================================================================
const templateFileInput = ref<HTMLInputElement | null>(null);
const templateDropzoneRef = ref<any>(null);
const isUploadingTemplate = ref(false);
const templateUploadProgress = ref<{ percent: number; loaded: number; total: number; filename: string } | null>(null);

function triggerTemplateUpload() {
  if (templateDropzoneRef.value) {
    templateDropzoneRef.value.openFileDialog();
  } else {
    templateFileInput.value?.click();
  }
}

async function refreshTemplates() {
  try {
    await storeRefreshTemplates();
  } catch (err: any) {
    console.error('Failed to refresh templates:', err);
  }
}

async function uploadTemplateFile(file: File) {
  if (!file) return;

  isUploadingTemplate.value = true;
  templateUploadProgress.value = {
    percent: 0,
    loaded: 0,
    total: file.size,
    filename: file.name,
  };

  try {
    const uploaded = await services.lxc.uploadTemplate(file, (p) => {
      templateUploadProgress.value = {
        percent: p.percent,
        loaded: p.loaded,
        total: p.total,
        filename: file.name,
      };
    });
    notify('success', `Uploaded template ${uploaded.name} (${uploaded.sizeFormatted})`);
    await refreshTemplates();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to upload LXC template');
  } finally {
    isUploadingTemplate.value = false;
    templateUploadProgress.value = null;
    if (templateFileInput.value) templateFileInput.value.value = '';
  }
}

async function handleUploadTemplate(e: Event) {
  const target = e.target as HTMLInputElement;
  const file = target.files?.[0];
  if (file) {
    await uploadTemplateFile(file);
  }
}

function handleInvalidTemplateFile(filename: string, reason: string) {
  notify('error', reason);
}

async function handleDeleteTemplate(tmpl: StorageImage) {
  const res = await confirmDelete({
    kind: 'Template',
    name: tmpl.name,
    details: `This will permanently remove ${tmpl.path} (${tmpl.sizeFormatted}) from the main pool.`,
    confirmText: 'Delete Template',
  });
  if (!res.confirmed) return;

  try {
    await services.lxc.deleteTemplate(tmpl.name);
    notify('info', `Deleted LXC template ${tmpl.name}`);
    await refreshTemplates();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete LXC template');
  }
}

function useTemplateForContainer(tmpl: StorageImage) {
  form.templateArchive = tmpl.path;
  if (!showCreate.value) {
    toggleCreate();
    form.templateArchive = tmpl.path;
  }
  notify('info', `Selected template: ${tmpl.name}`);
}

function copyTemplatePath(path: string) {
  navigator.clipboard.writeText(path);
  notify('info', `Copied path to clipboard: ${path}`);
}
</script>

<template>
  <!-- Top View Navigation Subtabs -->
  <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; flex-wrap: wrap; gap: 12px">
    <div style="display: flex; gap: 6px; background: var(--bg-surface); padding: 4px; border-radius: 8px; border: 1px solid var(--border-subtle)">
      <button
        class="btn btn-sm"
        :class="activeMainTab === 'containers' ? 'btn-primary' : 'btn-ghost'"
        style="padding: 5px 14px; font-weight: 600"
        @click="switchTab('containers')"
      >
        <MdiIcon :path="mdiServer" size="14" style="margin-right: 6px" />
        Containers ({{ nasState.lxcContainers.length }})
      </button>

      <button
        class="btn btn-sm"
        :class="activeMainTab === 'catalog' ? 'btn-primary' : 'btn-ghost'"
        style="padding: 5px 14px; font-weight: 600"
        @click="switchTab('catalog')"
      >
        <MdiIcon :path="mdiApps" size="14" style="margin-right: 6px" />
        App Catalog ({{ catalogItems?.length || 0 }})
      </button>

      <button
        class="btn btn-sm"
        :class="activeMainTab === 'templates' ? 'btn-primary' : 'btn-ghost'"
        style="padding: 5px 14px; font-weight: 600"
        @click="switchTab('templates')"
      >
        <MdiIcon :path="mdiArchiveOutline" size="14" style="margin-right: 6px" />
        Templates ({{ nasState.lxcTemplates.length }})
      </button>
    </div>

    <div v-if="activeMainTab === 'containers'" style="display: flex; gap: 8px">
      <button class="btn btn-sm" :disabled="isRefreshingContainers" @click="refreshLxc">
        <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" />
        {{ isRefreshingContainers ? 'Refreshing...' : 'Refresh' }}
      </button>
      <button class="btn btn-primary" @click="toggleCreate">
        {{ showCreate ? 'Cancel' : '+ Create Container' }}
      </button>
    </div>
  </div>

  <!-- ======================================================================= -->
  <!-- TAB 1: Containers Management View                                      -->
  <!-- ======================================================================= -->
  <div v-if="activeMainTab === 'containers'">
    <div class="card">
      <div class="card-header">
        <div>
          <h2 class="card-title" style="font-size: 17px">LXC Containers</h2>
        </div>
      </div>

      <!-- Create / Deploy Container Form -->
      <form
        v-if="showCreate"
        @submit.prevent="handleCreate"
        style="
          background: var(--bg-base);
          border: 1px solid var(--border-strong);
          border-radius: 8px;
          padding: 16px;
          margin: 0 16px 18px 16px;
        "
      >
        <div v-if="deployingCatalogItem" style="display: flex; align-items: center; gap: 10px; margin-bottom: 14px; padding-bottom: 12px; border-bottom: 1px solid var(--border-subtle)">
          <img v-if="deployingCatalogItem.iconUrl" :src="deployingCatalogItem.iconUrl" style="width: 32px; height: 32px; object-fit: contain; border-radius: 6px" alt="" />
          <div>
            <div style="font-size: 14px; font-weight: 700; color: var(--text-primary)">
              Deploying {{ deployingCatalogItem.title }}
            </div>
            <div style="font-size: 12px; color: var(--text-muted)">
              Source: {{ deployingCatalogItem.source }} · Pre-configured with recommended resources and rootfs
            </div>
          </div>
        </div>

        <div class="form-grid">
          <div class="form-group">
            <label>Container Name</label>
            <input
              v-model="form.containerName"
              class="input"
              placeholder="e.g. Debian Web Server"
              @input="onContainerNameInput"
            />
          </div>
          <div class="form-group">
            <label>Container ID</label>
            <input
              v-model="form.containerId"
              class="input mono"
              placeholder="e.g. debian-web-server"
              required
            />
            <small style="color: var(--text-muted); font-size: 11px">Derived from container name</small>
          </div>
          <div class="form-group">
            <label>Distribution</label>
            <select v-model="form.distribution" class="select" @change="onDistributionChange">
              <option value="debian">debian</option>
              <option value="ubuntu">ubuntu</option>
              <option value="alpine">alpine</option>
              <option value="archlinux">archlinux</option>
            </select>
          </div>
          <div class="form-group">
            <label>Release</label>
            <input v-model="form.release" class="input mono" placeholder="bookworm / noble / 3.20" required />
          </div>
          <div class="form-group">
            <label>Architecture</label>
            <select v-model="form.architecture" class="select">
              <option value="amd64">amd64</option>
              <option value="arm64">arm64</option>
            </select>
          </div>
          <div class="form-group">
            <label>CPU Cores</label>
            <input v-model.number="form.cpuCores" type="number" min="1" max="64" class="input mono" />
          </div>
          <div class="form-group">
            <label>Memory Limit (MiB)</label>
            <input v-model.number="form.memoryLimitMib" type="number" min="128" step="128" class="input mono" />
          </div>
          <div class="form-group">
            <label>Network Bridge</label>
            <select v-model="form.networkBridge" class="select mono">
              <option v-for="b in bridgeOptionsFor(form.networkBridge)" :key="b.name" :value="b.name">
                {{ b.label }}
              </option>
            </select>
          </div>
          <div class="form-group" style="grid-column: span 2">
            <label>Description (Optional)</label>
            <input v-model="form.description" class="input" placeholder="e.g. Production microservice container" />
          </div>
          <div class="form-group" style="grid-column: span 2">
            <label>Template Archive (.tar.gz / .tar.xz / .tar.zst) (Optional)</label>
            <div v-if="nasState.lxcTemplates.length > 0" style="margin-bottom: 6px">
              <select
                class="select mono"
                :value="form.templateArchive"
                @change="(e) => form.templateArchive = (e.target as HTMLSelectElement).value"
              >
                <option value="">-- Select a local template or leave empty for base distro --</option>
                <option v-for="tmpl in nasState.lxcTemplates" :key="tmpl.path" :value="tmpl.path">
                  {{ tmpl.name }} ({{ tmpl.sizeFormatted }})
                </option>
              </select>
            </div>
            <PathSelector
              v-model="form.templateArchive"
              :placeholder="lxcTemplatesDir + '/debian-12.tar.xz or custom archive path'"
              :root-hint="lxcTemplatesDir"
            />
          </div>
        </div>

        <!-- Storage Mounts Section -->
        <div style="margin-top: 14px; padding-top: 12px; border-top: 1px solid var(--border-subtle)">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
            <div>
              <h4 style="font-size: 13px; font-weight: 600; margin: 0; color: var(--text-primary)">
                Storage Bind Mounts
              </h4>
              <span style="font-size: 11px; color: var(--text-muted)">
                Map host ZFS datasets or directories into the container filesystem
              </span>
            </div>
            <button type="button" class="btn btn-xs" @click="addCreateMount">
              <MdiIcon :path="mdiPlus" size="13" style="margin-right: 4px" /> Add Mount Point
            </button>
          </div>

          <div v-if="form.mountPoints.length === 0" style="font-size: 12px; color: var(--text-muted); font-style: italic; padding: 4px 0">
            No bind mounts configured.
          </div>

          <div v-else style="display: flex; flex-direction: column; gap: 8px; margin-bottom: 8px">
            <div
              v-for="(mp, idx) in form.mountPoints"
              :key="idx"
              style="display: grid; grid-template-columns: 1fr 1fr auto auto; gap: 8px; align-items: center"
            >
              <input
                v-model="mp.hostPath"
                class="input mono"
                style="font-size: 12px"
                placeholder="Host Path, e.g. /mnt/tank/media"
                required
              />
              <input
                v-model="mp.containerPath"
                class="input mono"
                style="font-size: 12px"
                placeholder="Container Path, e.g. /media"
                required
              />
              <label style="display: flex; align-items: center; gap: 4px; font-size: 12px; white-space: nowrap">
                <input v-model="mp.readOnly" type="checkbox" /> Read Only
              </label>
              <button
                type="button"
                class="btn btn-xs btn-danger"
                style="padding: 4px"
                title="Remove Mount"
                @click="removeCreateMount(idx)"
              >
                <MdiIcon :path="mdiTrashCanOutline" size="14" />
              </button>
            </div>
          </div>
        </div>

        <!-- Flags & Actions -->
        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 14px; padding-top: 10px; border-top: 1px solid var(--border-subtle)">
          <div style="display: flex; gap: 16px; font-size: 12.5px; flex-wrap: wrap">
            <label style="display: flex; align-items: center; gap: 6px; cursor: pointer">
              <input v-model="form.unprivileged" type="checkbox" /> Unprivileged
            </label>
            <label style="display: flex; align-items: center; gap: 6px; cursor: pointer">
              <input v-model="form.autostart" type="checkbox" /> Autostart on boot
            </label>
            <label style="display: flex; align-items: center; gap: 6px; cursor: pointer">
              <input v-model="form.gpuEnabled" type="checkbox" /> GPU Acceleration (/dev/dri, nvidia)
            </label>
          </div>
          <button type="submit" class="btn btn-primary" :disabled="isDeploying">
            {{ isDeploying ? 'Deploying...' : 'Deploy Container' }}
          </button>
        </div>
      </form>

      <!-- LXC Containers Table -->
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <SortableTh :sort="lxcSort" sort-key="name">Name</SortableTh>
              <SortableTh :sort="lxcSort" sort-key="state">State</SortableTh>
              <SortableTh :sort="lxcSort" sort-key="autostart">Autostart</SortableTh>
              <SortableTh :sort="lxcSort" sort-key="distro">Distro / Release / Arch</SortableTh>
              <SortableTh :sort="lxcSort" sort-key="ipv4">IPv4 Address</SortableTh>
              <SortableTh :sort="lxcSort" sort-key="resources">Resources</SortableTh>
              <th style="text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <!-- Skeleton Placeholders when Loading -->
            <template v-if="nasState.loading && nasState.lxcContainers.length === 0">
              <tr v-for="r in 3" :key="r">
                <td>
                  <div style="display: flex; flex-direction: column; gap: 4px">
                    <SkeletonBox width="130px" height="15px" />
                    <SkeletonBox width="90px" height="11px" />
                  </div>
                </td>
                <td><SkeletonBox width="65px" height="20px" rounded="10px" /></td>
                <td><SkeletonBox width="60px" height="20px" rounded="10px" /></td>
                <td><SkeletonBox width="100px" height="14px" /></td>
                <td><SkeletonBox width="95px" height="14px" /></td>
                <td><SkeletonBox width="110px" height="14px" /></td>
                <td style="text-align: right">
                  <div style="display: inline-flex; gap: 6px">
                    <SkeletonBox width="26px" height="26px" rounded="4px" />
                    <SkeletonBox width="26px" height="26px" rounded="4px" />
                  </div>
                </td>
              </tr>
            </template>

            <tr v-else-if="nasState.lxcContainers.length === 0">
              <td colspan="7" style="text-align: center; color: var(--text-secondary); padding: 36px">
                No containers yet. Click "+ Create Container" above or browse the <strong>App Catalog</strong>.
              </td>
            </tr>

            <tr v-for="lx in lxcSort.sorted.value" :key="lx.name">
              <!-- Name & ID -->
              <td>
                <div
                  style="font-weight: 600; cursor: pointer; color: var(--text-primary)"
                  @click="openInspectModal(lx, 'overview')"
                >
                  {{ lx.containerName }}
                </div>
                <div class="mono" style="font-size: 11px; color: var(--text-muted)">
                  {{ lx.name.replace('lxcContainers/', '') }}
                </div>
                <div v-if="lx.description" style="font-size: 11px; color: var(--text-secondary); margin-top: 2px">
                  {{ lx.description }}
                </div>
              </td>

              <!-- State & Badges -->
              <td>
                <div style="display: flex; align-items: center; gap: 4px; flex-wrap: wrap">
                  <span
                    class="badge"
                    :class="
                      lx.state === 'RUNNING'
                        ? 'badge-success'
                        : lx.state === 'FROZEN'
                        ? 'badge-warning'
                        : 'badge-neutral'
                    "
                  >
                    {{ lx.state }}
                  </span>
                  <span v-if="lx.unprivileged" class="badge badge-info" title="Unprivileged user namespace">
                    Unpriv
                  </span>
                  <span v-if="lx.gpuEnabled" class="badge" style="background: rgba(16, 185, 129, 0.15); color: #10b981; border: 1px solid rgba(16, 185, 129, 0.3)" title="GPU Acceleration Enabled">
                    GPU
                  </span>
                </div>
              </td>

              <!-- Autostart Toggle Badge -->
              <td>
                <button
                  type="button"
                  class="badge"
                  :class="lx.autostart ? 'badge-success' : 'badge-neutral'"
                  style="cursor: pointer; border: 1px solid var(--border-subtle); display: inline-flex; align-items: center; gap: 4px; font-size: 11px; padding: 2px 8px; transition: all 0.15s ease"
                  :title="`Click to ${lx.autostart ? 'disable' : 'enable'} autostart on host boot`"
                  :disabled="togglingAutostart[lx.name]"
                  @click="toggleAutostart(lx)"
                >
                  <span
                    style="display: inline-block; width: 6px; height: 6px; border-radius: 50%"
                    :style="{ background: lx.autostart ? 'var(--success)' : 'var(--text-muted)' }"
                  ></span>
                  {{ lx.autostart ? 'On Boot' : 'Manual' }}
                </button>
              </td>

              <!-- Distro / Release / Arch -->
              <td class="mono">
                {{ lx.distribution }} / {{ lx.release }} ({{ lx.architecture }})
              </td>

              <!-- IPv4 Address -->
              <td class="mono" style="color: var(--success-text)">
                {{ lx.ipv4Address || '—' }}
              </td>

              <!-- Resources -->
              <td class="mono" style="font-size: 12px">
                <div>{{ lx.cpuCores }} Cores · {{ lx.memoryLimitMib }} MiB</div>
                <div v-if="lx.mountPoints && lx.mountPoints.length > 0" style="font-size: 11px; color: var(--text-muted)">
                  {{ lx.mountPoints.length }} bind mount{{ lx.mountPoints.length > 1 ? 's' : '' }}
                </div>
              </td>

              <!-- Actions Toolbar -->
              <td style="text-align: right">
                <div style="display: inline-flex; gap: 4px; flex-wrap: wrap; justify-content: flex-end">
                  <!-- Start Button -->
                  <button
                    v-if="lx.state !== 'RUNNING'"
                    class="btn btn-xs btn-success"
                    title="Start Container"
                    @click="handleAction(lx.name, 'start')"
                  >
                    <MdiIcon :path="mdiPlay" size="13" />
                  </button>

                  <!-- Restart Button -->
                  <button
                    v-if="lx.state === 'RUNNING'"
                    class="btn btn-xs"
                    title="Restart Container"
                    @click="handleRestart(lx)"
                  >
                    <MdiIcon :path="mdiRestart" size="13" />
                  </button>

                  <!-- Freeze / Unfreeze -->
                  <button
                    v-if="lx.state === 'RUNNING'"
                    class="btn btn-xs"
                    title="Freeze Container (pause execution)"
                    @click="handleAction(lx.name, 'freeze')"
                  >
                    <MdiIcon :path="mdiPause" size="13" />
                  </button>
                  <button
                    v-else-if="lx.state === 'FROZEN'"
                    class="btn btn-xs btn-success"
                    title="Unfreeze Container"
                    @click="handleAction(lx.name, 'start')"
                  >
                    <MdiIcon :path="mdiPlay" size="13" />
                  </button>

                  <!-- Stop Button -->
                  <button
                    v-if="lx.state !== 'STOPPED'"
                    class="btn btn-xs"
                    title="Stop Container"
                    @click="handleAction(lx.name, 'stop')"
                  >
                    <MdiIcon :path="mdiStop" size="13" />
                  </button>

                  <!-- Terminal Button -->
                  <button
                    class="btn btn-xs btn-primary"
                    :disabled="lx.state !== 'RUNNING'"
                    title="Open Web Terminal Console"
                    @click="openTerminal(lx)"
                  >
                    <MdiIcon :path="mdiConsole" size="13" />
                  </button>

                  <!-- Logs Button -->
                  <button
                    class="btn btn-xs"
                    title="View Container Logs"
                    @click="openInspectModal(lx, 'logs')"
                  >
                    <MdiIcon :path="mdiFileDocumentOutline" size="13" />
                  </button>

                  <!-- Edit Button -->
                  <button
                    class="btn btn-xs"
                    title="Edit Hardware & Settings"
                    @click="openEditModal(lx)"
                  >
                    <MdiIcon :path="mdiPencil" size="13" />
                  </button>

                  <!-- Inspect Button -->
                  <button
                    class="btn btn-xs"
                    title="Inspect Container Details"
                    @click="openInspectModal(lx, 'overview')"
                  >
                    <MdiIcon :path="mdiInformationOutline" size="13" />
                  </button>

                  <!-- Delete Button -->
                  <button
                    class="btn btn-xs btn-danger"
                    title="Delete Container"
                    @click="handleDelete(lx)"
                  >
                    <MdiIcon :path="mdiTrashCan" size="13" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>

  <!-- ======================================================================= -->
  <!-- TAB 2: LXC Application & Template Catalog                              -->
  <!-- ======================================================================= -->
  <div v-else-if="activeMainTab === 'catalog'">
    <div class="card" style="margin-bottom: 16px">
      <div class="card-header" style="flex-wrap: wrap; gap: 12px">
        <div>
          <h2 class="card-title" style="font-size: 17px">LXC Application & Template Catalog</h2>
          <p style="font-size: 12px; color: var(--text-secondary); margin: 4px 0 0 0">
            Deploy native system containers from Community-Scripts (Proxmox), TurnKey Linux appliances, or LinuxContainers OS images
          </p>
        </div>

        <div style="display: flex; gap: 8px; align-items: center">
          <span v-if="catalogLastSync" style="font-size: 11px; color: var(--text-muted)">
            Synced: {{ new Date(catalogLastSync).toLocaleDateString() }}
          </span>
          <button class="btn btn-sm" :disabled="isSyncingCatalog" @click="handleSyncCatalog">
            <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" />
            {{ isSyncingCatalog ? 'Syncing...' : 'Sync Catalog' }}
          </button>
        </div>
      </div>

      <!-- Filters & Search Toolbar -->
      <div style="padding: 12px 16px; background: var(--bg-base); border-top: 1px solid var(--border-subtle); display: flex; flex-wrap: wrap; justify-content: space-between; gap: 12px; align-items: center">
        <!-- Source Filter Pills -->
        <div style="display: flex; gap: 6px; flex-wrap: wrap">
          <button
            class="btn btn-xs"
            :class="catalogSourceFilter === 'all' ? 'btn-primary' : 'btn-secondary'"
            @click="catalogSourceFilter = 'all'"
          >
            All Sources ({{ catalogItems.length }})
          </button>
          <button
            class="btn btn-xs"
            :class="catalogSourceFilter === 'community-scripts' ? 'btn-primary' : 'btn-secondary'"
            @click="catalogSourceFilter = 'community-scripts'"
          >
            Community Apps ({{ catalogItems.filter(i => i.source === 'community-scripts').length }})
          </button>
          <button
            class="btn btn-xs"
            :class="catalogSourceFilter === 'turnkey' ? 'btn-primary' : 'btn-secondary'"
            @click="catalogSourceFilter = 'turnkey'"
          >
            TurnKey Appliances ({{ catalogItems.filter(i => i.source === 'turnkey').length }})
          </button>
          <button
            class="btn btn-xs"
            :class="catalogSourceFilter === 'linuxcontainers' ? 'btn-primary' : 'btn-secondary'"
            @click="catalogSourceFilter = 'linuxcontainers'"
          >
            LinuxContainers OS ({{ catalogItems.filter(i => i.source === 'linuxcontainers').length }})
          </button>
        </div>

        <!-- Search & Category Filter -->
        <div style="display: flex; gap: 8px; align-items: center; flex: 1; max-width: 440px">
          <select v-model="catalogCategoryFilter" class="select" style="max-width: 160px; font-size: 12px; height: 32px">
            <option value="all">All Categories</option>
            <option v-for="c in catalogCategories" :key="c" :value="c">{{ c }}</option>
          </select>
          <div style="position: relative; flex: 1">
            <input
              v-model="catalogQuery"
              class="input"
              style="padding-left: 28px; font-size: 12px; height: 32px"
              placeholder="Search applications, appliances, distros..."
            />
            <MdiIcon
              :path="mdiMagnify"
              size="15"
              style="position: absolute; left: 8px; top: 8px; color: var(--text-muted)"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- Catalog Grid of Cards -->
    <div v-if="isLoadingCatalog" style="display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 16px">
      <div v-for="n in 6" :key="n" class="card" style="padding: 16px">
        <SkeletonBox width="60%" height="20px" style="margin-bottom: 8px" />
        <SkeletonBox width="100%" height="36px" style="margin-bottom: 12px" />
        <SkeletonBox width="40%" height="16px" />
      </div>
    </div>

    <div v-else-if="filteredCatalogItems.length === 0" class="card" style="padding: 48px; text-align: center; color: var(--text-secondary)">
      No applications found matching your criteria. Try adjusting the search query or source filter.
    </div>

    <div v-else style="display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 16px">
      <div
        v-for="item in filteredCatalogItems"
        :key="item.id"
        class="card"
        style="display: flex; flex-direction: column; justify-content: space-between; padding: 16px; background: var(--bg-surface); transition: transform 0.15s ease, border-color 0.15s ease"
      >
        <div>
          <!-- Card Header -->
          <div style="display: flex; align-items: flex-start; gap: 12px; margin-bottom: 10px">
            <div
              style="
                width: 44px;
                height: 44px;
                border-radius: 8px;
                background: var(--bg-base);
                border: 1px solid var(--border-subtle);
                display: flex;
                align-items: center;
                justify-content: center;
                overflow: hidden;
                flex-shrink: 0;
              "
            >
              <img
                v-if="item.iconUrl"
                :src="item.iconUrl"
                style="width: 32px; height: 32px; object-fit: contain"
                alt=""
                @error="(e) => (e.target as HTMLElement).style.display = 'none'"
              />
              <MdiIcon v-else :path="item.source === 'linuxcontainers' ? mdiServer : mdiApps" size="24" style="color: var(--accent-primary)" />
            </div>

            <div style="flex: 1; min-width: 0">
              <div style="display: flex; align-items: center; justify-content: space-between; gap: 6px">
                <h3 style="font-size: 15px; font-weight: 700; color: var(--text-primary); margin: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis">
                  {{ item.title }}
                </h3>
              </div>

              <div style="display: flex; align-items: center; gap: 6px; margin-top: 4px; flex-wrap: wrap">
                <!-- Source Badge -->
                <span
                  class="badge"
                  :style="{
                    background:
                      item.source === 'community-scripts'
                        ? 'rgba(59, 130, 246, 0.15)'
                        : item.source === 'turnkey'
                        ? 'rgba(245, 158, 11, 0.15)'
                        : 'rgba(16, 185, 129, 0.15)',
                    color:
                      item.source === 'community-scripts'
                        ? '#3b82f6'
                        : item.source === 'turnkey'
                        ? '#f59e0b'
                        : '#10b981',
                    border: `1px solid ${
                      item.source === 'community-scripts'
                        ? 'rgba(59, 130, 246, 0.3)'
                        : item.source === 'turnkey'
                        ? 'rgba(245, 158, 11, 0.3)'
                        : 'rgba(16, 185, 129, 0.3)'
                    }`,
                    fontSize: '10px',
                    padding: '1px 6px',
                  }"
                >
                  {{ item.source === 'community-scripts' ? 'Community' : item.source === 'turnkey' ? 'TurnKey' : 'LinuxContainers' }}
                </span>

                <!-- Category -->
                <span class="mono" style="font-size: 11px; color: var(--text-muted)">
                  {{ item.category }}
                </span>
              </div>
            </div>
          </div>

          <!-- Description -->
          <p style="font-size: 12.5px; color: var(--text-secondary); line-height: 1.45; margin: 0 0 12px 0; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden">
            {{ item.description }}
          </p>

          <!-- Specifications Chips -->
          <div style="display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 14px">
            <span class="badge badge-neutral" style="font-size: 10.5px">
              {{ item.defaultCpuCores }} vCPU{{ item.defaultCpuCores > 1 ? 's' : '' }}
            </span>
            <span class="badge badge-neutral" style="font-size: 10.5px">
              {{ item.defaultMemoryLimitMib }} MiB RAM
            </span>
            <span v-if="item.defaultPort" class="badge badge-neutral mono" style="font-size: 10.5px">
              Port :{{ item.defaultPort }}
            </span>
            <span v-if="item.gpuRecommended" class="badge" style="background: rgba(16, 185, 129, 0.12); color: #10b981; font-size: 10.5px">
              GPU Ready
            </span>
            <span class="badge badge-neutral mono" style="font-size: 10.5px">
              {{ item.baseDistribution }}/{{ item.baseRelease }}
            </span>
          </div>
        </div>

        <!-- Card Footer Actions -->
        <div style="display: flex; justify-content: space-between; align-items: center; padding-top: 10px; border-top: 1px solid var(--border-subtle)">
          <div style="display: flex; gap: 8px">
            <a
              v-if="item.website"
              :href="item.website"
              target="_blank"
              class="btn btn-xs"
              style="padding: 3px 6px"
              title="Official Website"
            >
              <MdiIcon :path="mdiOpenInNew" size="12" />
            </a>
            <a
              v-if="item.documentationUrl"
              :href="item.documentationUrl"
              target="_blank"
              class="btn btn-xs"
              style="padding: 3px 6px"
              title="Documentation"
            >
              <MdiIcon :path="mdiFileDocumentOutline" size="12" />
            </a>
          </div>

          <button
            class="btn btn-xs btn-primary"
            style="font-weight: 600; padding: 4px 12px"
            :title="`Deploy ${item.title} as LXC container`"
            @click="openDeployModal(item)"
          >
            + Deploy Container
          </button>
        </div>
      </div>
    </div>
  </div>

  <!-- ======================================================================= -->
  <!-- TAB 3: Local Templates Repository View                                  -->
  <!-- ======================================================================= -->
  <div v-else-if="activeMainTab === 'templates'">
    <div class="card">
      <div class="card-header">
        <div>
          <h2 class="card-title" style="font-size: 17px">Template Archives</h2>
          <span style="font-size: 12px; color: var(--text-secondary)">
            Local template images stored on main pool ZFS dataset: {{ lxcTemplatesDir }}
          </span>
        </div>

        <div style="display: flex; gap: 10px; align-items: center">
          <input
            ref="templateFileInput"
            type="file"
            accept=".tar.gz,.tar.xz,.tar.zst,.tar,.tgz"
            style="display: none"
            @change="handleUploadTemplate"
          />
          <button class="btn btn-sm" @click="refreshTemplates">
            <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" /> Refresh
          </button>
          <button
            class="btn btn-sm btn-primary"
            :disabled="isUploadingTemplate"
            @click="triggerTemplateUpload"
          >
            <MdiIcon :path="mdiUpload" size="14" style="margin-right: 4px" />
            {{ isUploadingTemplate && templateUploadProgress ? `Uploading ${templateUploadProgress.percent}%...` : 'Upload Template' }}
          </button>
        </div>
      </div>

      <!-- Drag & Drop Template Dropzone -->
      <div style="margin: 0 16px 16px 16px">
        <FileDropzone
          ref="templateDropzoneRef"
          accept=".tar.gz,.tar.xz,.tar.zst,.tar,.tgz"
          label="Drag & drop template archive here, or click to browse"
          sublabel="Supports .tar.gz, .tar.xz, .tar.zst, .tar, .tgz templates"
          :icon="mdiArchiveOutline"
          :is-uploading="isUploadingTemplate"
          :progress="templateUploadProgress"
          @file-selected="uploadTemplateFile"
          @invalid-file="handleInvalidTemplateFile"
        />
      </div>

      <!-- Templates Table -->
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <SortableTh :sort="templateSort" sort-key="name">Template Archive Name</SortableTh>
              <SortableTh :sort="templateSort" sort-key="size">File Size</SortableTh>
              <SortableTh :sort="templateSort" sort-key="modified">Last Modified</SortableTh>
              <th style="text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <!-- Active Upload Row -->
            <tr v-if="isUploadingTemplate && templateUploadProgress">
              <td colspan="4" style="padding: 14px 16px; background: rgba(59, 130, 246, 0.04)">
                <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; font-size: 12px">
                  <span class="mono" style="font-weight: 600">{{ templateUploadProgress.filename }} (Uploading)</span>
                  <span class="mono">{{ formatBytes(templateUploadProgress.loaded) }} / {{ formatBytes(templateUploadProgress.total) }} ({{ templateUploadProgress.percent }}%)</span>
                </div>
                <div class="progress-bar" style="height: 6px">
                  <div class="progress-fill" :style="{ width: `${templateUploadProgress.percent}%` }"></div>
                </div>
              </td>
            </tr>

            <!-- Skeleton Placeholders when Loading -->
            <template v-if="nasState.loading && nasState.lxcTemplates.length === 0">
              <tr v-for="r in 2" :key="r">
                <td><SkeletonBox width="160px" height="15px" /></td>
                <td><SkeletonBox width="70px" height="15px" /></td>
                <td><SkeletonBox width="120px" height="15px" /></td>
                <td style="text-align: right"><SkeletonBox width="60px" height="24px" rounded="4px" /></td>
              </tr>
            </template>

            <tr v-else-if="nasState.lxcTemplates.length === 0">
              <td colspan="4" style="text-align: center; color: var(--text-secondary); padding: 36px">
                No local templates yet. Upload a .tar.gz, .tar.xz or .tar.zst archive or deploy from the <strong>App Catalog</strong>.
              </td>
            </tr>

            <tr v-for="tmpl in templateSort.sorted.value" :key="tmpl.name">
              <td style="font-weight: 600">
                <div style="display: flex; align-items: center; gap: 8px">
                  <MdiIcon :path="mdiArchiveOutline" size="16" style="color: var(--accent-primary)" />
                  <span class="mono" :title="tmpl.path">{{ tmpl.name }}</span>
                </div>
              </td>
              <td class="mono">{{ tmpl.sizeFormatted }}</td>
              <td class="mono" style="font-size: 11px; color: var(--text-muted)">
                {{ tmpl.modTime ? new Date(tmpl.modTime).toLocaleString() : '—' }}
              </td>
              <td style="text-align: right">
                <div style="display: inline-flex; gap: 6px">
                  <button
                    class="btn btn-xs btn-primary"
                    title="Use template to create container"
                    @click="useTemplateForContainer(tmpl)"
                  >
                    Use Template
                  </button>
                  <button
                    class="btn btn-xs"
                    title="Copy absolute path"
                    @click="copyTemplatePath(tmpl.path)"
                  >
                    <MdiIcon :path="mdiContentCopy" size="13" />
                  </button>
                  <button
                    class="btn btn-xs btn-danger"
                    title="Delete Template"
                    @click="handleDeleteTemplate(tmpl)"
                  >
                    <MdiIcon :path="mdiTrashCan" size="13" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>

  <!-- ======================================================================= -->
  <!-- MODAL 1: Edit LXC Container                                            -->
  <!-- ======================================================================= -->
  <div v-if="isEditModalOpen && editingContainer" class="modal-backdrop">
    <div class="modal-card modal-card-large">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px">
        <div>
          <h3 style="font-size: 18px; font-weight: 700; color: var(--text-primary); margin: 0">
            Edit Container: {{ editingContainer.containerName }}
          </h3>
          <span class="mono" style="font-size: 11px; color: var(--text-muted)">
            {{ editingContainer.name }}
          </span>
        </div>
        <button class="btn btn-sm" style="padding: 2px 6px" @click="isEditModalOpen = false">
          <MdiIcon :path="mdiClose" size="18" />
        </button>
      </div>

      <form @submit.prevent="handleSaveEdit">
        <div class="form-grid">
          <div class="form-group">
            <label>Container Name</label>
            <input v-model="editForm.containerName" class="input" required />
          </div>
          <div class="form-group">
            <label>Network Bridge</label>
            <select v-model="editForm.networkBridge" class="select mono">
              <option v-for="b in bridgeOptionsFor(editForm.networkBridge)" :key="b.name" :value="b.name">
                {{ b.label }}
              </option>
            </select>
          </div>
          <div class="form-group">
            <label>CPU Cores</label>
            <input v-model.number="editForm.cpuCores" type="number" min="1" max="64" class="input mono" required />
          </div>
          <div class="form-group">
            <label>Memory Limit (MiB)</label>
            <input v-model.number="editForm.memoryLimitMib" type="number" min="128" step="128" class="input mono" required />
          </div>
          <div class="form-group" style="grid-column: span 2">
            <label>Description</label>
            <input v-model="editForm.description" class="input" placeholder="e.g. Media server container" />
          </div>
        </div>

        <!-- Storage Mounts in Edit Modal -->
        <div style="margin-top: 14px; padding-top: 12px; border-top: 1px solid var(--border-subtle)">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
            <div>
              <h4 style="font-size: 13px; font-weight: 600; margin: 0; color: var(--text-primary)">
                Storage Bind Mounts
              </h4>
              <span style="font-size: 11px; color: var(--text-muted)">
                Map host datasets or folders into container mount paths
              </span>
            </div>
            <button type="button" class="btn btn-xs" @click="addEditMount">
              <MdiIcon :path="mdiPlus" size="13" style="margin-right: 4px" /> Add Mount Point
            </button>
          </div>

          <div v-if="editForm.mountPoints.length === 0" style="font-size: 12px; color: var(--text-muted); font-style: italic; padding: 4px 0">
            No bind mounts configured.
          </div>

          <div v-else style="display: flex; flex-direction: column; gap: 8px; margin-bottom: 12px">
            <div
              v-for="(mp, idx) in editForm.mountPoints"
              :key="idx"
              style="display: grid; grid-template-columns: 1fr 1fr auto auto; gap: 8px; align-items: center"
            >
              <input
                v-model="mp.hostPath"
                class="input mono"
                style="font-size: 12px"
                placeholder="Host Path, e.g. /mnt/tank/media"
                required
              />
              <input
                v-model="mp.containerPath"
                class="input mono"
                style="font-size: 12px"
                placeholder="Container Path, e.g. /media"
                required
              />
              <label style="display: flex; align-items: center; gap: 4px; font-size: 12px; white-space: nowrap">
                <input v-model="mp.readOnly" type="checkbox" /> Read Only
              </label>
              <button
                type="button"
                class="btn btn-xs btn-danger"
                style="padding: 4px"
                title="Remove Mount"
                @click="removeEditMount(idx)"
              >
                <MdiIcon :path="mdiTrashCanOutline" size="14" />
              </button>
            </div>
          </div>
        </div>

        <!-- Checkboxes -->
        <div style="display: flex; gap: 16px; margin-top: 14px; padding-top: 12px; border-top: 1px solid var(--border-subtle); flex-wrap: wrap">
          <label style="display: flex; align-items: center; gap: 6px; font-size: 13px; cursor: pointer">
            <input v-model="editForm.autostart" type="checkbox" /> Autostart on boot
          </label>
          <label style="display: flex; align-items: center; gap: 6px; font-size: 13px; cursor: pointer">
            <input v-model="editForm.unprivileged" type="checkbox" /> Unprivileged container
          </label>
          <label style="display: flex; align-items: center; gap: 6px; font-size: 13px; cursor: pointer">
            <input v-model="editForm.gpuEnabled" type="checkbox" /> GPU Passthrough (/dev/dri, nvidia)
          </label>
        </div>

        <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 18px">
          <button type="button" class="btn btn-secondary" @click="isEditModalOpen = false">
            Cancel
          </button>
          <button type="submit" class="btn btn-primary" :disabled="isSavingEdit">
            {{ isSavingEdit ? 'Saving...' : 'Save Changes' }}
          </button>
        </div>
      </form>
    </div>
  </div>

  <!-- ======================================================================= -->
  <!-- MODAL 2: LXC Container Inspector (Overview, Mounts, Logs)               -->
  <!-- ======================================================================= -->
  <div v-if="isInspectModalOpen && inspectedContainer" class="modal-backdrop">
    <div class="modal-card modal-card-large">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px; flex-shrink: 0">
        <div>
          <div style="display: flex; align-items: center; gap: 8px">
            <h3 style="font-size: 18px; font-weight: 700; color: var(--text-primary); margin: 0">
              {{ inspectedContainer.containerName }}
            </h3>
            <span
              class="badge"
              :class="
                inspectedContainer.state === 'RUNNING'
                  ? 'badge-success'
                  : inspectedContainer.state === 'FROZEN'
                  ? 'badge-warning'
                  : 'badge-neutral'
              "
            >
              {{ inspectedContainer.state }}
            </span>
            <button
              type="button"
              class="badge"
              :class="inspectedContainer.autostart ? 'badge-success' : 'badge-neutral'"
              style="cursor: pointer; border: 1px solid var(--border-subtle); display: inline-flex; align-items: center; gap: 4px; font-size: 11px; padding: 2px 8px; transition: all 0.15s ease"
              :title="`Click to ${inspectedContainer.autostart ? 'disable' : 'enable'} autostart on host boot`"
              @click="toggleAutostart(inspectedContainer)"
            >
              <span
                style="display: inline-block; width: 6px; height: 6px; border-radius: 50%"
                :style="{ background: inspectedContainer.autostart ? 'var(--success)' : 'var(--text-muted)' }"
              ></span>
              {{ inspectedContainer.autostart ? 'Autostart: On Boot' : 'Autostart: Manual' }}
            </button>
          </div>
          <p class="mono" style="font-size: 12px; color: var(--text-secondary); margin-top: 4px; margin-bottom: 0">
            {{ inspectedContainer.name }}
          </p>
        </div>

        <div style="display: flex; gap: 8px; align-items: center">
          <button
            class="btn btn-sm btn-primary"
            :disabled="inspectedContainer.state !== 'RUNNING'"
            title="Open Interactive Terminal"
            @click="openTerminal(inspectedContainer)"
          >
            <MdiIcon :path="mdiConsole" size="14" style="margin-right: 4px" /> Terminal
          </button>
          <button class="btn btn-sm" style="padding: 2px 6px" @click="isInspectModalOpen = false">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>
      </div>

      <!-- Inspector Tabs -->
      <div style="display: flex; gap: 8px; border-bottom: 1px solid var(--border-subtle); padding-bottom: 10px; margin-bottom: 14px; flex-shrink: 0">
        <button
          class="btn btn-sm"
          :class="inspectActiveTab === 'overview' ? 'btn-primary' : ''"
          @click="inspectActiveTab = 'overview'"
        >
          <MdiIcon :path="mdiSpeedometer" size="14" style="margin-right: 4px" />
          Overview
        </button>
        <button
          class="btn btn-sm"
          :class="inspectActiveTab === 'mounts' ? 'btn-primary' : ''"
          @click="inspectActiveTab = 'mounts'"
        >
          <MdiIcon :path="mdiFolder" size="14" style="margin-right: 4px" />
          Mount Points ({{ inspectedContainer.mountPoints?.length || 0 }})
        </button>
        <button
          class="btn btn-sm"
          :class="inspectActiveTab === 'logs' ? 'btn-primary' : ''"
          @click="inspectActiveTab = 'logs'; fetchLogs()"
        >
          <MdiIcon :path="mdiFileDocumentOutline" size="14" style="margin-right: 4px" />
          Logs
        </button>
      </div>

      <!-- Tab 1: Overview -->
      <div v-if="inspectActiveTab === 'overview'" style="display: flex; flex-direction: column; gap: 14px">
        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 12px">
          <div class="card" style="padding: 12px; background: var(--bg-surface)">
            <div style="font-size: 11px; color: var(--text-muted); text-transform: uppercase">Distribution</div>
            <div class="mono" style="font-size: 14px; font-weight: 600; margin-top: 4px">
              {{ inspectedContainer.distribution }} / {{ inspectedContainer.release }}
            </div>
            <div class="mono" style="font-size: 11px; color: var(--text-secondary)">{{ inspectedContainer.architecture }}</div>
          </div>

          <div class="card" style="padding: 12px; background: var(--bg-surface)">
            <div style="font-size: 11px; color: var(--text-muted); text-transform: uppercase">IPv4 Address</div>
            <div class="mono" style="font-size: 14px; font-weight: 600; margin-top: 4px" :style="{ color: inspectedContainer.ipv4Address ? 'var(--success-text)' : 'var(--text-muted)' }">
              {{ inspectedContainer.ipv4Address || 'No IP assigned' }}
            </div>
            <div class="mono" style="font-size: 11px; color: var(--text-secondary)">Bridge: {{ inspectedContainer.networkBridge || defaultBridge || 'br0' }}</div>
          </div>

          <div class="card" style="padding: 12px; background: var(--bg-surface)">
            <div style="font-size: 11px; color: var(--text-muted); text-transform: uppercase">Hardware Resources</div>
            <div class="mono" style="font-size: 14px; font-weight: 600; margin-top: 4px">
              {{ inspectedContainer.cpuCores }} vCPUs · {{ inspectedContainer.memoryLimitMib }} MiB RAM
            </div>
            <div style="font-size: 11px; color: var(--text-secondary)">
              {{ inspectedContainer.unprivileged ? 'Unprivileged Container' : 'Privileged Container' }}
            </div>
          </div>

          <div class="card" style="padding: 12px; background: var(--bg-surface)">
            <div style="font-size: 11px; color: var(--text-muted); text-transform: uppercase">GPU Passthrough</div>
            <div style="font-size: 14px; font-weight: 600; margin-top: 4px" :style="{ color: inspectedContainer.gpuEnabled ? 'var(--success-text)' : 'var(--text-muted)' }">
              {{ inspectedContainer.gpuEnabled ? 'Enabled (/dev/dri, nvidia)' : 'Disabled' }}
            </div>
          </div>
        </div>

        <div class="card" style="padding: 14px; background: var(--bg-surface)">
          <div style="font-weight: 600; font-size: 13px; margin-bottom: 8px">Storage Configuration</div>
          <div style="display: flex; flex-direction: column; gap: 6px; font-size: 12px">
            <div>
              <span style="color: var(--text-muted)">Rootfs Path: </span>
              <span class="mono">{{ inspectedContainer.rootfsPath }}</span>
            </div>
            <div v-if="inspectedContainer.templateArchive">
              <span style="color: var(--text-muted)">Base Template: </span>
              <span class="mono">{{ inspectedContainer.templateArchive }}</span>
            </div>
            <div v-if="inspectedContainer.description">
              <span style="color: var(--text-muted)">Description: </span>
              <span>{{ inspectedContainer.description }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Tab 2: Mount Points -->
      <div v-else-if="inspectActiveTab === 'mounts'">
        <div v-if="!inspectedContainer.mountPoints || inspectedContainer.mountPoints.length === 0" style="text-align: center; color: var(--text-secondary); padding: 32px">
          No bind mount points configured for this container.
        </div>
        <div v-else class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>Host Storage Path</th>
                <th>Container Mount Path</th>
                <th>Access Mode</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(mp, i) in inspectedContainer.mountPoints" :key="i">
                <td class="mono" style="font-size: 12px">{{ mp.hostPath }}</td>
                <td class="mono" style="font-size: 12px">{{ mp.containerPath }}</td>
                <td>
                  <span class="badge" :class="mp.readOnly ? 'badge-warning' : 'badge-success'">
                    {{ mp.readOnly ? 'Read Only' : 'Read / Write' }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Tab 3: Container Logs -->
      <div v-else-if="inspectActiveTab === 'logs'" style="display: flex; flex-direction: column; height: 480px">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
          <div style="display: flex; gap: 8px; align-items: center">
            <span style="font-size: 12px; color: var(--text-muted)">Lines:</span>
            <select v-model.number="logTail" class="select" style="padding: 2px 8px; font-size: 12px; height: 28px" @change="fetchLogs">
              <option :value="50">50</option>
              <option :value="100">100</option>
              <option :value="200">200</option>
              <option :value="500">500</option>
            </select>
          </div>
          <div style="display: flex; gap: 8px">
            <button class="btn btn-sm" @click="fetchLogs" :disabled="isLoadingLogs">
              <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" />
              {{ isLoadingLogs ? 'Refreshing...' : 'Refresh' }}
            </button>
            <button class="btn btn-sm" @click="copyLogs">
              <MdiIcon :path="mdiContentCopy" size="14" style="margin-right: 4px" /> Copy
            </button>
          </div>
        </div>

        <pre
          class="mono"
          style="
            flex: 1;
            background: #090d16;
            color: #d1d5db;
            border: 1px solid var(--border-subtle);
            border-radius: 6px;
            padding: 12px;
            font-size: 12px;
            line-height: 1.5;
            overflow-y: auto;
            white-space: pre-wrap;
            margin: 0;
          "
        >{{ logsContent }}</pre>
      </div>
    </div>
  </div>

  <!-- ======================================================================= -->
  <!-- MODAL 3: Interactive Container Web Terminal                            -->
  <!-- ======================================================================= -->
  <DockerTerminalModal
    v-if="isTerminalOpen && activeTerminalContainer"
    :app-title="`LXC Terminal: ${activeTerminalContainer.containerName || activeTerminalContainer.name.replace('lxcContainers/', '')}`"
    :containers="terminalContainerOptions"
    :initial-container-id="activeTerminalContainer.name.replace('lxcContainers/', '')"
    endpoint-type="lxc"
    @close="isTerminalOpen = false; activeTerminalContainer = null"
  />
</template>
