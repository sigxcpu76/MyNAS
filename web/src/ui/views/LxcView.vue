<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import {
  mdiApps,
  mdiArchiveOutline,
  mdiCircle,
  mdiClose,
  mdiChevronDoubleDown,
  mdiConsole,
  mdiContentCopy,
  mdiCubeOutline,
  mdiDownload,
  mdiFileDocumentOutline,
  mdiFilterVariant,
  mdiFolder,
  mdiInformationOutline,
  mdiLayers,
  mdiLoading,
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
import { useTableSort } from '../composables/useTableSort';
import { services } from '../../container';
import { nasState, notify, refreshLxcTemplates as storeRefreshTemplates, selectResource, uploadLxcTemplateTracked } from '../store';
import { confirmDelete } from '../modal';
import { generateLxcRootfsPath, toZfsSafeSlug } from '../../domain/id';
import { formatBytes, type LxcCatalogItem, type LxcContainer, type LxcMountPoint, type StorageImage } from '../../domain/models';
import { getLxcContainerIcon } from '../utils/appIcons';
import { useNetworkBridges } from '../utils/networkBridges';

const failedLxcIcons = ref<Record<string, boolean>>({});

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
function getInitialLxcTab(): 'containers' | 'catalog' | 'templates' {
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      const saved = window.localStorage.getItem('lxc_active_tab');
      if (saved === 'catalog' || saved === 'templates' || saved === 'containers') {
        return saved;
      }
    }
  } catch {}
  return 'containers';
}

const activeMainTab = ref<'containers' | 'catalog' | 'templates'>(getInitialLxcTab());

function switchTab(tab: 'containers' | 'catalog' | 'templates') {
  activeMainTab.value = tab;
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      window.localStorage.setItem('lxc_active_tab', tab);
    }
  } catch {}
  if (tab === 'catalog' && catalogItems.value.length === 0 && !isLoadingCatalog.value) {
    loadCatalog();
  }
}

function cleanPoolName(name: string | undefined | null): string {
  if (!name) return '';
  return name.replace(/^zfsPools\//, '').trim();
}

function openLxcDetail(lx: LxcContainer) {
  const clean = lx.name.replace(/^lxcContainers\//, '');
  selectResource(`lxc:${clean}`);
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
const availablePools = computed(() => {
  const seen = new Set<string>();
  const list: Array<{ name: string; isMain: boolean }> = [];
  for (const p of nasState.pools || []) {
    const clean = cleanPoolName(p.poolName || p.name);
    if (clean && !seen.has(clean)) {
      seen.add(clean);
      list.push({ name: clean, isMain: clean === mainPool.value });
    }
  }
  if (mainPool.value && !seen.has(mainPool.value)) {
    seen.add(mainPool.value);
    list.push({ name: mainPool.value, isMain: true });
  }
  return list;
});
const lxcTemplatesDir = computed(() => `/mnt/${mainPool.value}/lxc-templates`);

function normalizeDistroName(raw?: string): string {
  const d = (raw || '').toLowerCase().trim();
  if (d === 'arch' || d === 'arch linux' || d === 'arch-linux' || d === 'archlinux') return 'archlinux';
  if (d === 'alpine' || d === 'alpine linux' || d === 'alpine-linux' || d === 'alpinelinux') return 'alpine';
  if (d === 'alma' || d === 'alma linux' || d === 'alma-linux' || d === 'almalinux') return 'almalinux';
  if (d === 'rocky' || d === 'rocky linux' || d === 'rocky-linux' || d === 'rockylinux') return 'rockylinux';
  if (d === 'centos' || d === 'centos stream' || d === 'centos-stream') return 'centos';
  if (d === 'void' || d === 'void linux' || d === 'void-linux' || d === 'voidlinux') return 'voidlinux';
  if (d === 'kali' || d === 'kali linux' || d === 'kali-linux' || d === 'kalilinux') return 'kali';
  if (d === 'oracle' || d === 'oracle linux' || d === 'oracle-linux' || d === 'oraclelinux') return 'oracle';
  if (d === 'amazon' || d === 'amazon linux' || d === 'amazon-linux' || d === 'amazonlinux') return 'amazonlinux';
  if (d === 'opensuse' || d === 'suse') return 'opensuse';
  return d.replace(/\s+/g, '');
}

function defaultReleaseForDistro(distro: string): string {
  switch (normalizeDistroName(distro)) {
    case 'debian': return 'bookworm';
    case 'ubuntu': return 'noble';
    case 'alpine': return '3.21';
    case 'archlinux': return 'current';
    case 'almalinux':
    case 'rockylinux': return '10';
    case 'fedora': return '41';
    case 'centos': return '9-Stream';
    case 'opensuse': return '15.6';
    case 'devuan': return 'daedalus';
    case 'openeuler': return '24.03';
    case 'oracle': return '9';
    case 'amazonlinux': return '2023';
    default: return 'current';
  }
}

function inferDistroFromTemplatePath(pathOrName: string): { distro: string; release: string; label: string } | null {
  const base = (pathOrName.split('/').pop() || '').toLowerCase();
  const candidates: Array<{ match: string; distro: string; label: string }> = [
    { match: 'arch', distro: 'archlinux', label: 'Arch Linux' },
    { match: 'almalinux', distro: 'almalinux', label: 'AlmaLinux' },
    { match: 'alma', distro: 'almalinux', label: 'AlmaLinux' },
    { match: 'rocky', distro: 'rockylinux', label: 'Rocky Linux' },
    { match: 'alpine', distro: 'alpine', label: 'Alpine' },
    { match: 'ubuntu', distro: 'ubuntu', label: 'Ubuntu' },
    { match: 'fedora', distro: 'fedora', label: 'Fedora' },
    { match: 'centos', distro: 'centos', label: 'CentOS' },
    { match: 'opensuse', distro: 'opensuse', label: 'openSUSE' },
    { match: 'gentoo', distro: 'gentoo', label: 'Gentoo' },
    { match: 'devuan', distro: 'devuan', label: 'Devuan' },
    { match: 'kali', distro: 'kali', label: 'Kali' },
    { match: 'void', distro: 'voidlinux', label: 'Void Linux' },
    { match: 'debian', distro: 'debian', label: 'Debian' },
  ];
  for (const c of candidates) {
    if (base.includes(c.match)) {
      return { distro: c.distro, release: defaultReleaseForDistro(c.distro), label: c.label };
    }
  }
  return null;
}

/**
 * Computes a unique, collision-free, ZFS-safe template archive filename for catalog items.
 * If upstream has generic names like "rootfs.tar.xz", names it after the template (e.g. debian-bookworm-amd64-rootfs.tar.xz).
 */
function computeSafeTemplateFilename(item: LxcCatalogItem, overrideUrl?: string): string {
  const url = overrideUrl || item.downloadUrl;
  if (!url) return '';

  const cleanUrl = url.split('?')[0].split('#')[0];
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
    if (item.source === 'community-scripts' && !item.downloadUrl) {
      const lcMatch = cleanUrl.match(/linuxcontainers\.org\/images\/([^/]+)\/([^/]+)\/([^/]+)/i);
      if (lcMatch) {
        const d = normalizeDistroName(lcMatch[1]);
        const r = toZfsSafeSlug(lcMatch[2]);
        const a = toZfsSafeSlug(lcMatch[3] || item.architecture || 'amd64');
        return `${d}-${r}-${a}-rootfs${ext}`;
      }
    }
    const baseSlug = toZfsSafeSlug(item.title || item.id || 'template');
    const arch = item.architecture ? toZfsSafeSlug(item.architecture) : '';
    const fullSlug = arch && !baseSlug.includes(arch) ? `${baseSlug}-${arch}` : baseSlug;
    return `${fullSlug}-rootfs${ext}`;
  }

  const safeSlug = toZfsSafeSlug(nameWithoutExt);
  return `${safeSlug}${ext}`;
}

const { defaultBridge, bridgeOptionsFor } = useNetworkBridges();

const containerQuery = ref('');

const filteredContainers = computed(() => {
  const q = containerQuery.value.trim().toLowerCase();
  if (!q) return nasState.lxcContainers;
  return nasState.lxcContainers.filter((lx) => {
    return (
      (lx.containerName || '').toLowerCase().includes(q) ||
      (lx.name || '').toLowerCase().includes(q) ||
      (lx.distribution || '').toLowerCase().includes(q) ||
      (lx.release || '').toLowerCase().includes(q) ||
      (lx.ipv4Address || '').toLowerCase().includes(q) ||
      (lx.description || '').toLowerCase().includes(q)
    );
  });
});

const runningContainersCount = computed(
  () => nasState.lxcContainers.filter((lx) => lx.state === 'RUNNING').length
);

const lxcSort = useTableSort(() => filteredContainers.value, {
  name: (lx) => lx.containerName,
  distro: (lx) => `${lx.distribution} / ${lx.release} (${lx.architecture})`,
  ipv4: (lx) => ipv4ToNumber(lx.ipv4Address),
  cpu: (lx) => (lx.state === 'RUNNING' ? Number(lx.cpuPercent) || 0 : -1),
  memory: (lx) => (lx.state === 'RUNNING' ? Number(lx.memoryUsedMib) || 0 : -1),
  network: (lx) => (lx.state === 'RUNNING' ? (Number(lx.netRxRate) || 0) + (Number(lx.netTxRate) || 0) : -1),
});

function formatNetworkRate(lx: LxcContainer): string {
  if (lx.state !== 'RUNNING') return '—';
  const rx = lx.netRxRate || 0;
  const tx = lx.netTxRate || 0;
  const rxStr = formatBytes(rx) + '/s ↓';
  const txStr = formatBytes(tx) + '/s ↑';
  return `${rxStr} · ${txStr}`;
}

function formatNetworkTotal(lx: LxcContainer): string {
  if (lx.state !== 'RUNNING') return '';
  const rx = lx.netRxBytes || 0;
  const tx = lx.netTxBytes || 0;
  if (rx + tx === 0) return '';
  return `(${formatBytes(rx + tx)})`;
}

const templateSort = useTableSort(() => nasState.lxcTemplates, {
  name: (t) => t.name,
  size: (t) => t.sizeBytes,
  modified: (t) => (t.modTime ? new Date(t.modTime) : null),
});

// =============================================================================
// App Catalog State & Filters
// =============================================================================
const catalogItems = ref<LxcCatalogItem[]>(nasState.lxcCatalogItems || []);
const isLoadingCatalog = ref(false);
const isSyncingCatalog = ref(false);
const catalogSourceFilter = ref<string>('all');
const catalogCategoryFilter = ref<string>('all');
const catalogQuery = ref<string>('');
const catalogLastSync = ref<string>(nasState.lxcCatalogLastSync || '');

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
    const items = res?.items ?? (Array.isArray(res) ? res : []);
    catalogItems.value = items;
    nasState.lxcCatalogItems = items;
    catalogLastSync.value = res?.lastSyncTime ?? '';
    nasState.lxcCatalogLastSync = catalogLastSync.value;
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
    nasState.lxcCatalogLastSync = res.lastSyncTime;
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
  storagePool: '',
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
  templateDownloadUrl: '',
  mountPoints: [] as LxcMountPoint[],
});

function onContainerNameInput() {
  form.containerId = toZfsSafeSlug(form.containerName);
}

function onDistributionChange() {
  form.release = defaultReleaseForDistro(form.distribution);
  if (!form.containerName || / Container$/i.test(form.containerName)) {
    const title = form.distribution.charAt(0).toUpperCase() + form.distribution.slice(1);
    form.containerName = `${title} Container`;
    form.containerId = toZfsSafeSlug(form.containerName);
  }
}

function onTemplateArchiveSelect(path: string) {
  form.templateArchive = path;
  if (!path) return;
  form.templateDownloadUrl = '';
  const inferred = inferDistroFromTemplatePath(path);
  if (inferred) {
    form.distribution = inferred.distro;
    form.release = inferred.release;
    if (!deployingCatalogItem.value && (!form.containerName || / Container$/i.test(form.containerName))) {
      form.containerName = `${inferred.label} Container`;
      form.containerId = toZfsSafeSlug(form.containerName);
    }
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
    form.storagePool = mainPool.value;
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
    form.templateDownloadUrl = '';
    form.mountPoints = [];
    showCreate.value = true;
  }
}

function openDeployModal(item: LxcCatalogItem) {
  deployingCatalogItem.value = item;
  form.containerName = item.title;
  form.containerId = toZfsSafeSlug(item.title) || toZfsSafeSlug(item.id.replace(/^(community|turnkey|lxc)-/, ''));
  form.storagePool = mainPool.value;
  const normDistro = normalizeDistroName(item.baseDistribution || 'debian') || 'debian';
  form.distribution = normDistro;
  form.release = (item.baseRelease && !(normDistro !== 'debian' && item.baseRelease === 'bookworm'))
    ? item.baseRelease
    : defaultReleaseForDistro(normDistro);
  form.architecture = item.architecture || 'amd64';
  form.cpuCores = item.defaultCpuCores || 2;
  form.memoryLimitMib = item.defaultMemoryLimitMib || 1024;
  form.networkBridge = defaultBridge.value || 'br0';
  form.unprivileged = !item.privilegedRequired;
  form.autostart = true;
  form.gpuEnabled = Boolean(item.gpuRecommended);
  form.description = item.description || '';
  form.templateArchive = '';
  form.templateDownloadUrl = '';

  // If the catalog item does not have a direct downloadUrl (e.g. community-scripts app recipes),
  // resolve the base OS template image from the linuxcontainers catalog items
  let effectiveDownloadUrl = item.downloadUrl || '';
  if (!effectiveDownloadUrl) {
    const distroName = normDistro;
    const releaseName = (form.release || '').toLowerCase();
    const baseMatch = catalogItems.value.find((c) => {
      if (c.source !== 'linuxcontainers') return false;
      const cDistro = normalizeDistroName(c.baseDistribution || '');
      if (cDistro !== distroName) return false;
      const cRel = (c.baseRelease || '').toLowerCase();
      if (releaseName && (cRel === releaseName || (releaseName === '12' && cRel === 'bookworm') || (releaseName === '13' && cRel === 'trixie') || (releaseName === 'base' && cRel === 'current'))) {
        return true;
      }
      return false;
    }) || catalogItems.value.find((c) => c.source === 'linuxcontainers' && normalizeDistroName(c.baseDistribution || '') === distroName);

    if (baseMatch && baseMatch.downloadUrl) {
      effectiveDownloadUrl = baseMatch.downloadUrl;
    }
  }

  form.templateDownloadUrl = effectiveDownloadUrl;

  // Look up matching template if already downloaded
  if (nasState.lxcTemplates.length > 0) {
    const expectedFname = effectiveDownloadUrl ? computeSafeTemplateFilename(item, effectiveDownloadUrl).toLowerCase() : '';
    const matched = nasState.lxcTemplates.find((t) => {
      const tName = t.name.toLowerCase();
      if (expectedFname && tName === expectedFname) return true;
      if (tName.includes(item.id.toLowerCase())) return true;
      if (item.source !== 'turnkey') {
        const inferred = inferDistroFromTemplatePath(t.name);
        if (inferred && inferred.distro === normDistro && !tName.includes('turnkey')) return true;
      }
      return false;
    });
    if (matched) {
      form.templateArchive = matched.path;
    }
  }

  // Pre-fill archive filename in lxc-templates directory if downloadUrl is provided or resolved
  if (!form.templateArchive && effectiveDownloadUrl) {
    const safeFname = computeSafeTemplateFilename(item, effectiveDownloadUrl);
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
    const targetPool = (form.storagePool || mainPool.value).trim();
    const validMounts = form.mountPoints.filter((mp) => mp.hostPath.trim() && mp.containerPath.trim());
    const created = await services.lxc.createContainer(targetId, {
      containerName: form.containerName || targetId,
      rootfsPath: generateLxcRootfsPath(targetPool, targetId),
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
      templateDownloadUrl: form.templateDownloadUrl.trim() || deployingCatalogItem.value?.downloadUrl || undefined,
    });

    // Start container automatically upon deployment
    try {
      await services.lxc.startContainer(created.name);
      notify('success', `Deployed and initialized LXC container ${created.containerName || targetId}`);
    } catch (startErr: any) {
      notify('warning', `Container created, but failed to start: ${startErr?.message || startErr}`);
    }

    form.containerName = '';
    form.containerId = '';
    form.templateArchive = '';
    form.templateDownloadUrl = '';
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
// Container Lifecycle & Actions
// =============================================================================
const transitioningContainers = reactive<Record<string, 'starting' | 'stopping' | 'restarting' | 'freezing'>>({});

async function handleAction(name: string, verb: 'start' | 'stop' | 'freeze') {
  transitioningContainers[name] = verb === 'start' ? 'starting' : verb === 'stop' ? 'stopping' : 'freezing';
  try {
    if (verb === 'start') await services.lxc.startContainer(name);
    else if (verb === 'stop') await services.lxc.stopContainer(name);
    else await services.lxc.freezeContainer(name);
    notify('success', `Container ${verb === 'start' ? 'started' : verb === 'stop' ? 'stopped' : 'frozen'} successfully`);
    await refreshLxc();
  } catch (err: any) {
    notify('error', err?.message || `Failed to ${verb} LXC container`);
  } finally {
    delete transitioningContainers[name];
  }
}

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

onMounted(async () => {
  await refreshLxc();
  refreshTemplates().catch(() => {});
  loadCatalog().catch(() => {});
});

// =============================================================================
// Templates Repository
// =============================================================================
const templateFileInput = ref<HTMLInputElement | null>(null);
const templateDropzoneRef = ref<any>(null);
const isUploadingTemplate = computed(() => nasState.activeUploads.some((u) => u.kind === 'lxc-template'));
const templateUploadProgress = computed(() => nasState.activeUploads.find((u) => u.kind === 'lxc-template') || null);

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
  try {
    await uploadLxcTemplateTracked(file);
  } catch {
    // Already notified by uploadLxcTemplateTracked
  } finally {
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
  if (!showCreate.value) {
    toggleCreate();
  }
  onTemplateArchiveSelect(tmpl.path);
  showCreate.value = true;
  switchTab('containers');
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
      <div class="card-header" style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 10px">
        <div>
          <h2 class="card-title" style="font-size: 17px">LXC Containers</h2>
          <span v-if="nasState.lxcContainers.length > 0" style="font-size: 12px; color: var(--text-secondary)">
            {{ runningContainersCount }} running · {{ nasState.lxcContainers.length - runningContainersCount }} stopped
          </span>
        </div>
        <div v-if="nasState.lxcContainers.length > 0" style="position: relative; display: flex; align-items: center">
          <MdiIcon
            :path="mdiMagnify"
            size="15"
            style="position: absolute; left: 9px; color: var(--text-muted); pointer-events: none"
          />
          <input
            v-model="containerQuery"
            type="text"
            class="input"
            placeholder="Filter containers..."
            style="padding: 5px 28px 5px 30px; font-size: 12.5px; width: 220px"
          />
          <button
            v-if="containerQuery"
            type="button"
            style="position: absolute; right: 6px; background: none; border: none; color: var(--text-muted); cursor: pointer; padding: 2px; display: flex"
            aria-label="Clear container filter"
            @click="containerQuery = ''"
          >
            <MdiIcon :path="mdiClose" size="13" />
          </button>
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
              <option value="almalinux">almalinux</option>
              <option value="rockylinux">rockylinux</option>
              <option value="fedora">fedora</option>
              <option value="centos">centos</option>
              <option value="opensuse">opensuse</option>
              <option value="devuan">devuan</option>
              <option value="gentoo">gentoo</option>
              <option value="kali">kali</option>
              <option value="voidlinux">voidlinux</option>
              <option value="openeuler">openeuler</option>
              <option value="oracle">oracle</option>
              <option value="amazonlinux">amazonlinux</option>
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
          <div class="form-group">
            <label>Rootfs Storage Pool</label>
            <select v-model="form.storagePool" class="select mono">
              <option v-for="p in availablePools" :key="p.name" :value="p.name">
                {{ p.name }}{{ p.isMain ? ' (main)' : '' }}
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
                @change="(e) => onTemplateArchiveSelect((e.target as HTMLSelectElement).value)"
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
              :select-files="true"
              modal-title="Select LXC Template Archive"
              @update:model-value="onTemplateArchiveSelect"
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
              style="display: grid; grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr) auto auto; gap: 8px; align-items: center"
            >
              <PathSelector
                v-model="mp.hostPath"
                :placeholder="`/mnt/${mainPool}/media`"
                :root-hint="`/mnt/${mainPool}`"
                :select-files="true"
                button-label="Browse"
                modal-title="Select Host Directory or File for Bind Mount"
                :required="true"
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
              <SortableTh :sort="lxcSort" sort-key="distro" class="col-desktop-only">Distro / Release / Arch</SortableTh>
              <SortableTh :sort="lxcSort" sort-key="ipv4">IPv4 Address</SortableTh>
              <SortableTh :sort="lxcSort" sort-key="cpu" class="col-desktop-only">CPU</SortableTh>
              <SortableTh :sort="lxcSort" sort-key="memory" class="col-desktop-only">Memory</SortableTh>
              <SortableTh :sort="lxcSort" sort-key="network" class="col-desktop-only">Network</SortableTh>
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
                <td class="col-desktop-only"><SkeletonBox width="100px" height="14px" /></td>
                <td><SkeletonBox width="95px" height="14px" /></td>
                <td class="col-desktop-only"><SkeletonBox width="60px" height="14px" /></td>
                <td class="col-desktop-only"><SkeletonBox width="70px" height="14px" /></td>
                <td class="col-desktop-only"><SkeletonBox width="140px" height="14px" /></td>
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

            <tr v-else-if="lxcSort.sorted.value.length === 0">
              <td colspan="7" style="text-align: center; color: var(--text-secondary); padding: 36px">
                No containers match "{{ containerQuery }}".
              </td>
            </tr>

            <tr
              v-for="lx in lxcSort.sorted.value"
              :key="lx.name"
              style="cursor: pointer"
              title="Click to open container details"
              @click="openLxcDetail(lx)"
            >
              <!-- Name & Status Bullet with Icon -->
              <td>
                <div style="display: flex; align-items: center; gap: 10px">
                  <div class="lxc-row-icon-wrap" :title="`Status: ${lx.state}`">
                    <div class="lxc-row-icon-box">
                      <img
                        v-if="getLxcContainerIcon(lx) && !failedLxcIcons[lx.name]"
                        :src="getLxcContainerIcon(lx)"
                        :alt="lx.containerName"
                        class="lxc-row-img"
                        loading="lazy"
                        @error="failedLxcIcons[lx.name] = true"
                      />
                      <MdiIcon
                        v-else
                        :path="mdiCubeOutline"
                        size="18"
                        style="color: var(--accent-secondary, #c084fc)"
                      />
                    </div>
                    <!-- Status dot badge pinned to bottom-right of icon -->
                    <span
                      class="status-dot-badge"
                      :class="{ 'spin-icon': Boolean(transitioningContainers[lx.name]) }"
                      :style="{
                        background: transitioningContainers[lx.name]
                          ? 'var(--warning)'
                          : lx.state === 'RUNNING'
                          ? 'var(--success)'
                          : lx.state === 'FROZEN'
                          ? 'var(--warning)'
                          : 'var(--text-muted)'
                      }"
                    />
                  </div>

                  <div style="min-width: 0; flex: 1">
                    <div style="display: flex; align-items: center; gap: 6px; flex-wrap: wrap">
                      <span style="font-weight: 600; font-size: 13.5px; color: var(--text-primary)">
                        {{ lx.containerName }}
                      </span>
                      <span
                        v-if="transitioningContainers[lx.name]"
                        class="badge badge-warning pulse-anim"
                        style="display: inline-flex; align-items: center; gap: 4px; font-size: 11px; padding: 1px 6px"
                      >
                        <MdiIcon :path="mdiLoading" size="10" class="spin-icon" />
                        {{ transitioningContainers[lx.name].toUpperCase() }}...
                      </span>
                      <span v-if="lx.unprivileged" class="badge badge-info" style="font-size: 11px; padding: 1px 6px" title="Unprivileged user namespace">
                        Unpriv
                      </span>
                      <span v-if="lx.gpuEnabled" class="badge" style="background: rgba(16, 185, 129, 0.15); color: #10b981; border: 1px solid rgba(16, 185, 129, 0.3); font-size: 11px; padding: 1px 6px" title="GPU Acceleration Enabled">
                        GPU
                      </span>
                    </div>
                    <div v-if="lx.description" style="font-size: 11.5px; color: var(--text-secondary); margin-top: 2px">
                      {{ lx.description }}
                    </div>
                  </div>
                </div>
              </td>

              <!-- Distro / Release / Arch -->
              <td class="col-desktop-only">
                {{ lx.distribution }} / {{ lx.release }} ({{ lx.architecture }})
              </td>

              <!-- IPv4 Address -->
              <td class="mono" style="color: var(--success-text)">
                {{ lx.ipv4Address || '—' }}
              </td>

              <!-- CPU -->
              <td class="col-desktop-only" style="padding: 10px 14px; width: 85px; min-width: 85px; max-width: 85px; box-sizing: border-box; white-space: nowrap">
                <div v-if="lx.state === 'RUNNING'" style="font-size: 13.5px; font-weight: 600; color: var(--text-primary); font-variant-numeric: tabular-nums">
                  {{ (Number(lx.cpuPercent) || 0).toFixed(1) }}%
                </div>
                <div v-else style="color: var(--text-muted); font-size: 13.5px">—</div>
              </td>

              <!-- Memory -->
              <td class="col-desktop-only" style="padding: 10px 14px; width: 105px; min-width: 105px; max-width: 105px; box-sizing: border-box; white-space: nowrap">
                <div v-if="lx.state === 'RUNNING'" style="font-size: 13.5px; color: var(--text-primary); font-variant-numeric: tabular-nums">
                  {{ lx.memoryUsedMib || 0 }} MiB
                </div>
                <div v-else style="color: var(--text-muted); font-size: 13.5px">—</div>
              </td>

              <!-- Network (1s Live) -->
              <td class="col-desktop-only" style="padding: 10px 14px; width: 210px; min-width: 210px; max-width: 210px; box-sizing: border-box; white-space: nowrap">
                <div v-if="lx.state === 'RUNNING'" style="font-size: 12.5px; font-variant-numeric: tabular-nums">
                  <div style="color: var(--text-primary); font-weight: 500">
                    {{ formatNetworkRate(lx) }}
                  </div>
                  <div style="font-size: 11.5px; color: var(--text-secondary); min-height: 14px">
                    {{ formatNetworkTotal(lx) || ' ' }}
                  </div>
                </div>
                <div v-else style="color: var(--text-muted); font-size: 13.5px">—</div>
              </td>

              <!-- Quick Power Toggle Action -->
              <td style="text-align: right" @click.stop>
                <div style="display: inline-flex; gap: 4px; flex-wrap: wrap; justify-content: flex-end">
                  <!-- Start / Unfreeze Button -->
                  <button
                    v-if="lx.state !== 'RUNNING' || transitioningContainers[lx.name] === 'starting'"
                    class="btn btn-xs btn-success"
                    :class="{ 'btn-loading': transitioningContainers[lx.name] === 'starting' }"
                    :disabled="Boolean(transitioningContainers[lx.name])"
                    :title="transitioningContainers[lx.name] === 'starting' ? 'Starting container...' : lx.state === 'FROZEN' ? 'Unfreeze Container' : 'Start Container'"
                    @click.stop="handleAction(lx.name, 'start')"
                  >
                    <MdiIcon
                      :path="transitioningContainers[lx.name] === 'starting' ? mdiLoading : mdiPlay"
                      size="13"
                      :class="{ 'spin-icon': transitioningContainers[lx.name] === 'starting' }"
                      style="margin-right: 4px"
                    />
                    {{ transitioningContainers[lx.name] === 'starting' ? 'Starting...' : lx.state === 'FROZEN' ? 'Resume' : 'Start' }}
                  </button>

                  <!-- Stop Button -->
                  <button
                    v-if="lx.state === 'RUNNING' || transitioningContainers[lx.name] === 'stopping'"
                    class="btn btn-xs"
                    :class="{ 'btn-loading': transitioningContainers[lx.name] === 'stopping' }"
                    :disabled="Boolean(transitioningContainers[lx.name])"
                    :title="transitioningContainers[lx.name] === 'stopping' ? 'Stopping container...' : 'Stop Container'"
                    @click.stop="handleAction(lx.name, 'stop')"
                  >
                    <MdiIcon
                      :path="transitioningContainers[lx.name] === 'stopping' ? mdiLoading : mdiStop"
                      size="13"
                      :class="{ 'spin-icon': transitioningContainers[lx.name] === 'stopping' }"
                      style="margin-right: 4px"
                    />
                    {{ transitioningContainers[lx.name] === 'stopping' ? 'Stopping...' : 'Stop' }}
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
            Last synced: {{ formatSyncTime(catalogLastSync) }}
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
            <span class="badge badge-neutral" style="font-size: 11.5px">
              {{ item.defaultCpuCores }} vCPU{{ item.defaultCpuCores > 1 ? 's' : '' }}
            </span>
            <span class="badge badge-neutral" style="font-size: 11.5px">
              {{ item.defaultMemoryLimitMib }} MiB RAM
            </span>
            <span v-if="item.defaultPort" class="badge badge-neutral mono" style="font-size: 11.5px">
              Port :{{ item.defaultPort }}
            </span>
            <span v-if="item.gpuRecommended" class="badge" style="background: rgba(16, 185, 129, 0.12); color: #10b981; font-size: 11.5px">
              GPU Ready
            </span>
            <span class="badge badge-neutral mono" style="font-size: 11.5px">
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
              <td>{{ tmpl.sizeFormatted }}</td>
              <td style="font-size: 11px; color: var(--text-muted)">
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
                    <MdiIcon :path="mdiTrashCanOutline" size="13" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>

</template>

<style scoped>
.lxc-row-icon-wrap {
  position: relative;
  width: 32px;
  height: 32px;
  flex-shrink: 0;
}

.lxc-row-icon-box {
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

.lxc-row-img {
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
