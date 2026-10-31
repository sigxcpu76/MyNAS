/**
 * Global Reactive NAS State Store (Vue 3 Composition API)
 * Orchestrates Application Services and exposes reactive collections to UI Views.
 */

import { reactive } from 'vue';
import { aipClient, services } from '../container';
import { isDotDataset } from '../domain/models';
import {
  mapDockerApp,
  mapDockerContainer,
  mapLxcContainer,
  mapSystemTelemetry,
  mapVirtualMachine,
} from '../adapters/http/httpAdapters';
import type {
  CifsShare,
  CifsGlobalConfig,
  CatalogApp,
  Disk,
  DockerApp,
  DockerConfig,
  DockerContainer,
  Group,
  LxcCatalogItem,
  LxcContainer,
  Prerequisite,
  StorageImage,
  SystemConfig,
  SystemTelemetry,
  User,
  VirtualMachine,
  WebdavShare,
  ZfsDataset,
  ZfsPool,
  ZfsVolume,
} from '../domain/models';

export type NavTab =
  | 'overview'
  | 'prerequisites'
  | 'storage-disks'
  | 'storage-pools'
  | 'storage-datasets'
  | 'storage-volumes'
  | 'zfs'
  | 'kvm'
  | 'docker'
  | 'lxc'
  | 'shares'
  | 'shares-cifs'
  | 'shares-webdav'
  | 'identity'
  | 'terminal'
  | 'vnc-console';

export type ManagedModule = 'zfs' | 'kvm' | 'docker' | 'lxc' | 'cifs';

const MODULE_PREREQ_IDS: Record<ManagedModule, string[]> = {
  zfs: ['zpool', 'zfs', 'libzfs'],
  kvm: ['kvm-dev', 'qemu-system-x86_64', 'libvirt', 'virsh'],
  docker: ['docker'],
  lxc: ['lxc', 'liblxc'],
  cifs: ['smbd'],
};

export interface ModuleStatus {
  enabled: boolean;
  missing: Prerequisite[];
}

export interface ToastNotification {
  id: number;
  type: 'success' | 'error' | 'info' | 'warning';
  message: string;
}

export interface TaskRecord {
  id: string;
  startTime: string;
  endTime?: string;
  startTimestamp: number;
  endTimestamp?: number;
  node: string;
  user: string;
  description: string;
  status: 'RUNNING' | 'OK' | 'ERROR';
  duration?: string;
  details?: string;
  logs: string[];
}

export interface TrafficPoint {
  time: string;
  rxRate: number;
  txRate: number;
}

let nextToastId = 1;

interface LocalCachePayload {
  systemConfig?: SystemConfig | null;
  systemTelemetry?: SystemTelemetry | null;
  prerequisites?: Prerequisite[];
  disks?: Disk[];
  pools?: ZfsPool[];
  dockerApps?: DockerApp[];
  lastRefreshed?: string;
}

const STORAGE_CACHE_KEY = 'nan_cache_v1';
const LEGACY_CACHE_KEY = 'aegis_nas_cache_v1';
export const STORAGE_RESOURCE_KEY = 'nan_last_active_resource';
export const STORAGE_TASK_COLLAPSED_KEY = 'nan_task_console_collapsed';

function loadSavedResource(): string {
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      return window.localStorage.getItem(STORAGE_RESOURCE_KEY) || 'node:local';
    }
  } catch {
    // ignore
  }
  return 'node:local';
}

function loadTaskConsoleCollapsed(): boolean {
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      const v = window.localStorage.getItem(STORAGE_TASK_COLLAPSED_KEY);
      if (v !== null) return v === 'true';
    }
  } catch {
    // ignore
  }
  return false;
}

function loadLocalCache(): LocalCachePayload | null {
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      const raw = window.localStorage.getItem(STORAGE_CACHE_KEY) || window.localStorage.getItem(LEGACY_CACHE_KEY);
      if (raw) return JSON.parse(raw);
    }
  } catch {
    // ignore
  }
  return null;
}

export function saveLocalCache(): void {
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      const data: LocalCachePayload = {
        systemConfig: nasState.systemConfig,
        systemTelemetry: nasState.systemTelemetry,
        prerequisites: nasState.prerequisites,
        disks: nasState.disks,
        pools: nasState.pools,
        dockerApps: nasState.dockerApps,
        lastRefreshed: nasState.lastRefreshed,
      };
      window.localStorage.setItem(STORAGE_CACHE_KEY, JSON.stringify(data));
    }
  } catch {
    // ignore
  }
}

const cached = loadLocalCache();

export const nasState = reactive({
  activeTab: 'overview' as NavTab,
  selectedResourceId: loadSavedResource() as string,
  isTaskConsoleCollapsed: loadTaskConsoleCollapsed() as boolean,
  activeTaskDetail: null as TaskRecord | null,
  tasks: [
    {
      id: 'task-init',
      startTime: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
      startTimestamp: Date.now(),
      node: 'nas01',
      user: 'system',
      description: 'System: Initialized Proxmox-Style Hypervisor & Storage Workbench',
      status: 'OK',
      duration: '0.1s',
      details: 'Appliance session established',
      logs: [
        `[${new Date().toLocaleTimeString()}] [INFO] Proxmox-style appliance shell initialized`,
        `[${new Date().toLocaleTimeString()}] [INFO] Connected to NAS API backend`,
        `[${new Date().toLocaleTimeString()}] [OK] Resource tree & audit console ready`,
      ],
    },
  ] as TaskRecord[],
  authenticated: services.auth.isAuthenticated(),
  currentUser: null as User | null,
  loading: false,
  initialLoaded: Boolean(cached?.systemConfig),
  lastRefreshed: cached?.lastRefreshed || '',
  liveSyncActive: false,

  systemConfig: cached?.systemConfig || null,
  systemTelemetry: cached?.systemTelemetry ? mapSystemTelemetry(cached.systemTelemetry) : (null as SystemTelemetry | null),
  prerequisites: cached?.prerequisites || [],
  disks: cached?.disks || [],
  pools: cached?.pools || [],
  datasetsByPool: {} as Record<string, ZfsDataset[]>,
  volumesByPool: {} as Record<string, ZfsVolume[]>,
  selectedPoolName: (cached?.pools && cached.pools.length > 0 ? cached.pools[0].name : '') as string,

  vms: [] as VirtualMachine[],
  isos: [] as StorageImage[],
  lxcTemplates: [] as StorageImage[],
  dockerContainers: [] as DockerContainer[],
  dockerApps: cached?.dockerApps || [],
  dockerConfig: null as DockerConfig | null,
  lxcContainers: [] as LxcContainer[],
  lxcCatalogItems: [] as LxcCatalogItem[],
  lxcCatalogLastSync: '' as string,
  catalogApps: [] as CatalogApp[],
  catalogLastSync: '' as string,
  cifsShares: [] as CifsShare[],
  cifsGlobalConfig: null as CifsGlobalConfig | null,
  webdavShares: [] as WebdavShare[],
  users: [] as User[],
  groups: [] as Group[],
  includeSystemAccounts: false,
  includeSystemGroups: false,
  networkTrafficHistory: [] as TrafficPoint[],

  toasts: [] as ToastNotification[],
});

// Kick off polling if any cached app was in DEPLOYING state
setTimeout(() => checkAndPollDeployingApps(), 100);

export function getModuleAvailability(module: ManagedModule): ModuleStatus {
  // Gating: Only ZFS is enabled until a main pool is selected
  // Do not report missing main pool if initial fetch is still in flight and no config is available yet
  if (module !== 'zfs') {
    if (nasState.initialLoaded && !nasState.systemConfig?.mainPool) {
      return {
        enabled: false,
        missing: [
          {
            name: 'system/main-pool',
            displayName: 'Main ZFS Storage Pool (.nas-system)',
            category: 'STORAGE',
            kind: 'DAEMON',
            binaryOrPath: 'zpool',
            status: 'MISSING',
            version: 'unconfigured',
            details: 'A main ZFS storage pool must be designated to host system configuration and enable non-ZFS services',
            installHint: 'Select or create a ZFS pool in the Storage tab to initialize the system dataset',
            requiredBy: [module.toUpperCase()],
            fallbackMode: false,
          },
        ],
      };
    }
  }

  if (nasState.prerequisites.length === 0) {
    return { enabled: true, missing: [] };
  }
  const reqIds = MODULE_PREREQ_IDS[module] || [];
  const missing = nasState.prerequisites.filter((p) => {
    const shortId = p.name.replace(/^prerequisites\//, '');
    return (
      reqIds.includes(shortId) &&
      p.status !== 'INSTALLED' &&
      p.status !== 'FALLBACK_ACTIVE'
    );
  });
  return {
    enabled: missing.length === 0,
    missing,
  };
}

export function notify(type: 'success' | 'error' | 'info' | 'warning', message: string): void {
  const id = nextToastId++;
  nasState.toasts.push({ id, type, message });
  setTimeout(() => {
    const idx = nasState.toasts.findIndex((t) => t.id === id);
    if (idx !== -1) nasState.toasts.splice(idx, 1);
  }, 4200);
}

let nextTaskIdCounter = 1;

export function selectResource(resourceId: string): void {
  nasState.selectedResourceId = resourceId;
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      window.localStorage.setItem(STORAGE_RESOURCE_KEY, resourceId);
    }
  } catch {
    // ignore
  }
}

export function setTaskConsoleCollapsed(collapsed: boolean): void {
  nasState.isTaskConsoleCollapsed = collapsed;
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      window.localStorage.setItem(STORAGE_TASK_COLLAPSED_KEY, String(collapsed));
    }
  } catch {
    // ignore
  }
}

export function formatDurationMs(ms: number): string {
  if (ms < 1000) return `${Math.max(1, Math.round(ms))}ms`;
  const secs = ms / 1000;
  if (secs < 60) return `${secs.toFixed(1)}s`;
  const mins = Math.floor(secs / 60);
  const remSecs = Math.round(secs % 60);
  return `${mins}m ${remSecs}s`;
}

export function startTask(
  description: string,
  details = '',
  node = nasState.systemTelemetry?.host?.hostname || 'nas01',
  user = nasState.currentUser?.username || 'root'
): TaskRecord {
  const id = `task-${Date.now()}-${nextTaskIdCounter++}`;
  const now = Date.now();
  const timeStr = new Date(now).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  const task: TaskRecord = {
    id,
    startTime: timeStr,
    startTimestamp: now,
    node,
    user,
    description,
    status: 'RUNNING',
    details,
    logs: [
      `[${timeStr}] [INFO] Task initiated on node "${node}" by "${user}"`,
      `[${timeStr}] [INFO] Action: ${description}`,
    ],
  };
  if (details) {
    task.logs.push(`[${timeStr}] [INFO] Parameters: ${details}`);
  }
  nasState.tasks.unshift(task);
  if (nasState.tasks.length > 100) {
    nasState.tasks.pop();
  }
  return task;
}

export function appendTaskLog(taskId: string, message: string): void {
  const t = nasState.tasks.find((task) => task.id === taskId);
  if (!t) return;
  const timeStr = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  t.logs.push(`[${timeStr}] ${message}`);
}

export function finishTask(taskId: string, status: 'OK' | 'ERROR', message?: string): void {
  const t = nasState.tasks.find((task) => task.id === taskId);
  if (!t) return;
  const now = Date.now();
  t.endTimestamp = now;
  t.endTime = new Date(now).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  t.status = status;
  t.duration = formatDurationMs(now - t.startTimestamp);
  if (message) {
    appendTaskLog(taskId, `[${status}] ${message}`);
  }
  appendTaskLog(taskId, `[INFO] Task finished with status: ${status} (duration: ${t.duration})`);
}

export function clearTasks(): void {
  nasState.tasks = nasState.tasks.filter((t) => t.status === 'RUNNING');
}

export async function runTrackedTask<T>(
  description: string,
  fn: (taskId: string) => Promise<T>,
  details = '',
  node = nasState.systemTelemetry?.host?.hostname || 'nas01',
  user = nasState.currentUser?.username || 'root'
): Promise<T> {
  const task = startTask(description, details, node, user);
  try {
    const res = await fn(task.id);
    finishTask(task.id, 'OK', 'Command finished successfully');
    return res;
  } catch (err: any) {
    const errMsg = err?.message || String(err);
    finishTask(task.id, 'ERROR', `Error: ${errMsg}`);
    throw err;
  }
}

export async function refreshIsos(): Promise<void> {
  if (!getModuleAvailability('kvm').enabled) return;
  try {
    nasState.isos = await services.kvm.listIsos();
  } catch (err) {
    console.warn('Failed to refresh ISOs repository:', err);
  }
}

export async function refreshLxcTemplates(): Promise<void> {
  if (!getModuleAvailability('lxc').enabled) return;
  try {
    nasState.lxcTemplates = await services.lxc.listTemplates();
  } catch (err) {
    console.warn('Failed to refresh LXC templates repository:', err);
  }
}

export async function refreshZfsSubresources(poolName: string): Promise<void> {
  if (!poolName || !getModuleAvailability('zfs').enabled) return;
  const [datasets, volumes] = await Promise.all([
    services.zfs.listDatasets(poolName).catch(() => []),
    services.zfs.listVolumes(poolName).catch(() => []),
  ]);
  nasState.datasetsByPool[poolName] = datasets.filter((d) => !isDotDataset(d));
  nasState.volumesByPool[poolName] = volumes;
}

export async function refreshAllData(): Promise<void> {
  nasState.loading = true;
  try {
    const [sysCfg, prereqs, telem] = await Promise.all([
      services.system.getSystemConfig().catch(() => null),
      services.prerequisites.listPrerequisites().catch(() => []),
      services.system.getTelemetry().catch(() => null),
    ]);
    if (sysCfg) {
      if (sysCfg.mainPool) {
        sysCfg.mainPool = sysCfg.mainPool.replace(/^zfsPools\//, '').trim();
      }
      nasState.systemConfig = sysCfg;
    }
    if (prereqs && prereqs.length > 0) {
      nasState.prerequisites = prereqs;
    }
    if (telem) {
      nasState.systemTelemetry = telem;
    }
    nasState.initialLoaded = true;

    const zfsOk = getModuleAvailability('zfs').enabled;
    const kvmOk = getModuleAvailability('kvm').enabled;
    const dockerOk = getModuleAvailability('docker').enabled;
    const lxcOk = getModuleAvailability('lxc').enabled;
    const cifsOk = getModuleAvailability('cifs').enabled;

    const [
      disks,
      pools,
      vms,
      dockerContainers,
      dockerApps,
      lxcContainers,
      cifsShares,
      webdavShares,
      users,
      groups,
    ] = await Promise.all([
      services.disk.listDisks().catch(() => null),
      zfsOk ? services.zfs.listPools().catch(() => null) : Promise.resolve(null),
      kvmOk ? services.kvm.listVirtualMachines().catch(() => null) : Promise.resolve(null),
      dockerOk ? services.docker.listContainers().catch(() => null) : Promise.resolve(null),
      dockerOk ? services.apps.listApps().catch(() => null) : Promise.resolve(null),
      lxcOk ? services.lxc.listContainers().catch(() => null) : Promise.resolve(null),
      cifsOk ? services.shares.listCifsShares().catch(() => null) : Promise.resolve(null),
      services.shares.listWebdavShares().catch(() => null),
      services.identity.listUsers(nasState.includeSystemAccounts).catch(() => null),
      services.identity.listGroups(nasState.includeSystemGroups).catch(() => null),
    ]);

    if (disks !== null) nasState.disks = disks;
    if (pools !== null) {
      for (const p of pools) {
        p.poolName = (p.poolName || p.name.replace(/^zfsPools\//, '')).trim();
        if (!p.mountpoint || p.mountpoint === 'none' || p.mountpoint === 'legacy' || p.mountpoint.includes('zfsPools')) {
          p.mountpoint = `/mnt/${p.poolName}`;
        }
      }
      nasState.pools = pools;
    }
    if (vms !== null) nasState.vms = vms;
    if (dockerContainers !== null) nasState.dockerContainers = dockerContainers;
    if (dockerApps !== null) {
      nasState.dockerApps = dockerApps;
      checkAndPollDeployingApps();
    }
    if (lxcContainers !== null) nasState.lxcContainers = lxcContainers;
    if (cifsShares !== null) nasState.cifsShares = cifsShares;
    if (webdavShares !== null) nasState.webdavShares = webdavShares;
    if (users !== null) nasState.users = users;
    if (groups !== null) nasState.groups = groups;

    if (kvmOk) {
      refreshIsos().catch(() => {});
    }
    if (lxcOk) {
      refreshLxcTemplates().catch(() => {});
    }

    const effectivePools = pools ?? nasState.pools;
    if (effectivePools.length > 0 && zfsOk) {
      if (
        !nasState.selectedPoolName ||
        !effectivePools.some((p) => p.name === nasState.selectedPoolName)
      ) {
        nasState.selectedPoolName = effectivePools[0].name;
      }
      await Promise.all(effectivePools.map((p) => refreshZfsSubresources(p.name)));
    } else if (pools !== null) {
      nasState.selectedPoolName = '';
    }

    nasState.lastRefreshed = new Date().toLocaleTimeString();
    saveLocalCache();
  } finally {
    nasState.initialLoaded = true;
    nasState.loading = false;
  }
}

let deployingPollTimer: any = null;

export function checkAndPollDeployingApps(): void {
  if (typeof window === 'undefined') return;
  const hasDeploying = (nasState.dockerApps || []).some((a) => a?.state === 'DEPLOYING');
  if (hasDeploying) {
    if (!deployingPollTimer) {
      deployingPollTimer = setInterval(async () => {
        const stillDeploying = (nasState.dockerApps || []).some((a) => a?.state === 'DEPLOYING');
        if (!stillDeploying) {
          clearInterval(deployingPollTimer);
          deployingPollTimer = null;
          return;
        }
        await refreshDockerData();
      }, 2500);
    }
  } else if (deployingPollTimer) {
    clearInterval(deployingPollTimer);
    deployingPollTimer = null;
  }
}

export async function refreshDockerData(): Promise<void> {
  const dockerOk = getModuleAvailability('docker').enabled;
  if (!dockerOk) return;
  try {
    const [containers, apps] = await Promise.all([
      services.docker.listContainers().catch(() => null),
      services.apps.listApps().catch(() => null),
    ]);
    if (containers !== null) nasState.dockerContainers = containers;
    if (apps !== null) {
      nasState.dockerApps = apps;
      checkAndPollDeployingApps();
    }
    nasState.lastRefreshed = new Date().toLocaleTimeString();
    saveLocalCache();
  } catch {
    // ignore
  }
}

export const MAX_TRAFFIC_HISTORY_POINTS = 30;

export function recordNetworkTrafficSample(telem?: SystemTelemetry | null): void {
  const current = telem || nasState.systemTelemetry;
  if (!current?.network || current.network.length === 0) return;

  const list = current.network;
  const def = list.find((n) => n.isDefault);
  const activePhysical = list.find(
    (n) =>
      n.isUp &&
      n.ipAddresses &&
      n.ipAddresses.length > 0 &&
      !n.name.startsWith('docker') &&
      !n.name.startsWith('br-') &&
      !n.name.startsWith('vmnet') &&
      !n.name.startsWith('veth') &&
      !n.name.startsWith('virbr') &&
      n.name !== 'lo'
  );
  const activeWithIp = list.find(
    (n) => n.isUp && n.ipAddresses && n.ipAddresses.length > 0 && n.name !== 'lo'
  );
  const iface = def || activePhysical || activeWithIp || list.find((n) => n.name !== 'lo') || list[0];
  if (!iface) return;

  const now = new Date();
  const timeStr = now.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  const rx = iface.rxRateBytesPerSec || 0;
  const tx = iface.txRateBytesPerSec || 0;

  if (nasState.networkTrafficHistory.length === 0) {
    const ts = now.getTime();
    for (let i = MAX_TRAFFIC_HISTORY_POINTS - 1; i >= 0; i--) {
      const t = new Date(ts - i * 1000);
      nasState.networkTrafficHistory.push({
        time: t.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
        rxRate: rx,
        txRate: tx,
      });
    }
    return;
  }

  nasState.networkTrafficHistory.push({
    time: timeStr,
    rxRate: rx,
    txRate: tx,
  });

  if (nasState.networkTrafficHistory.length > MAX_TRAFFIC_HISTORY_POINTS) {
    nasState.networkTrafficHistory.shift();
  }
}

export async function refreshTelemetry(): Promise<void> {
  try {
    const telem = await services.system.getTelemetry();
    if (telem) {
      nasState.systemTelemetry = telem;
      recordNetworkTrafficSample(telem);
    }
  } catch {
    // ignore
  }
}

let eventSource: EventSource | null = null;
let reconnectTimer: any = null;
let watchAbortController: AbortController | null = null;
let watchReconnectTimer: any = null;

export function handleWatchResourceChange(msg: {
  resourceType: string;
  eventType: 'SNAPSHOT' | 'ADDED' | 'MODIFIED' | 'DELETED' | string;
  resourceName: string;
  jsonPayload: string;
}): void {
  if (!msg || !msg.resourceType) return;

  let payload: any = null;
  if (msg.jsonPayload) {
    try {
      payload = JSON.parse(msg.jsonPayload);
    } catch {
      payload = null;
    }
  }

  const { resourceType, eventType } = msg;

  if (eventType === 'SNAPSHOT' && payload) {
    switch (resourceType) {
      case 'dockerApps':
        nasState.dockerApps = Array.isArray(payload) ? payload.map(mapDockerApp) : [];
        checkAndPollDeployingApps();
        break;
      case 'dockerContainers':
        nasState.dockerContainers = Array.isArray(payload) ? payload.map(mapDockerContainer) : [];
        break;
      case 'virtualMachines':
        nasState.vms = Array.isArray(payload) ? payload.map(mapVirtualMachine) : [];
        break;
      case 'lxcContainers':
        nasState.lxcContainers = Array.isArray(payload) ? payload.map(mapLxcContainer) : [];
        break;
      case 'telemetry': {
        const mapped = mapSystemTelemetry(payload);
        nasState.systemTelemetry = mapped;
        recordNetworkTrafficSample(mapped);
        break;
      }
    }
    nasState.lastRefreshed = new Date().toLocaleTimeString();
    saveLocalCache();
    return;
  }

  // Delta updates (ADDED, MODIFIED, DELETED)
  switch (resourceType) {
    case 'dockerApps': {
      const app = payload ? mapDockerApp(payload) : null;
      const name = msg.resourceName || app?.name;
      const appId = app?.appId || (name ? name.replace(/^dockerApps\//, '') : '');
      const idx = nasState.dockerApps.findIndex(
        (a) => (name && a.name === name) || (appId && a.appId === appId)
      );
      if (eventType === 'DELETED') {
        if (idx !== -1) nasState.dockerApps.splice(idx, 1);
      } else if (app) {
        if (idx !== -1) {
          const existing = nasState.dockerApps[idx];
          nasState.dockerApps[idx] = {
            ...existing,
            ...app,
            services: (app.services && app.services.length > 0) ? app.services : (existing.services || []),
            containers: (app.containers && app.containers.length > 0) ? app.containers : (existing.containers || []),
            portMappings: (app.portMappings && app.portMappings.length > 0) ? app.portMappings : (existing.portMappings || []),
          };
        } else {
          nasState.dockerApps.push(app);
        }
      }
      checkAndPollDeployingApps();
      break;
    }
    case 'dockerContainers': {
      const container = payload ? mapDockerContainer(payload) : null;
      const name = msg.resourceName || container?.name;
      const idx = nasState.dockerContainers.findIndex((c) => c.name === name);
      if (eventType === 'DELETED') {
        if (idx !== -1) nasState.dockerContainers.splice(idx, 1);
      } else if (container) {
        if (idx !== -1) {
          nasState.dockerContainers[idx] = { ...nasState.dockerContainers[idx], ...container };
        } else {
          nasState.dockerContainers.push(container);
        }
      }
      break;
    }
    case 'virtualMachines': {
      const vm = payload ? mapVirtualMachine(payload) : null;
      const name = msg.resourceName || vm?.name;
      const idx = nasState.vms.findIndex((v) => v.name === name);
      if (eventType === 'DELETED') {
        if (idx !== -1) nasState.vms.splice(idx, 1);
      } else if (vm) {
        if (idx !== -1) {
          nasState.vms[idx] = { ...nasState.vms[idx], ...vm };
        } else {
          nasState.vms.push(vm);
        }
      }
      break;
    }
    case 'lxcContainers': {
      const ct = payload ? mapLxcContainer(payload) : null;
      const name = msg.resourceName || ct?.name;
      const idx = nasState.lxcContainers.findIndex((c) => c.name === name);
      if (eventType === 'DELETED') {
        if (idx !== -1) nasState.lxcContainers.splice(idx, 1);
      } else if (ct) {
        if (idx !== -1) {
          nasState.lxcContainers[idx] = { ...nasState.lxcContainers[idx], ...ct };
        } else {
          nasState.lxcContainers.push(ct);
        }
      }
      break;
    }
    case 'telemetry': {
      if (payload) {
        const mapped = mapSystemTelemetry(payload);
        nasState.systemTelemetry = mapped;
        recordNetworkTrafficSample(mapped);
      }
      break;
    }
  }

  nasState.lastRefreshed = new Date().toLocaleTimeString();
  saveLocalCache();
}

export function startWatchStream(): void {
  if (typeof window === 'undefined') return;
  if (watchAbortController) return;

  watchAbortController = new AbortController();
  const endpoint = 'resources:watch?resourceTypes=dockerApps,dockerContainers,virtualMachines,lxcContainers,telemetry';

  nasState.liveSyncActive = true;
  aipClient
    .watchStream(endpoint, handleWatchResourceChange, watchAbortController.signal)
    .catch((err) => {
      console.warn('Watch stream disconnected:', err);
    })
    .finally(() => {
      nasState.liveSyncActive = false;
      watchAbortController = null;
      clearTimeout(watchReconnectTimer);
      watchReconnectTimer = setTimeout(() => {
        if (nasState.authenticated) {
          startWatchStream();
        }
      }, 3000);
    });
}

export function stopWatchStream(): void {
  nasState.liveSyncActive = false;
  clearTimeout(watchReconnectTimer);
  if (watchAbortController) {
    watchAbortController.abort();
    watchAbortController = null;
  }
}

export function startEventStream(): void {
  if (typeof window === 'undefined' || !window.EventSource) return;
  if (eventSource) return;

  const token = aipClient.getToken();
  const url = token ? `/v1/events?access_token=${encodeURIComponent(token)}` : '/v1/events';

  try {
    eventSource = new EventSource(url);

    eventSource.onopen = () => {
      nasState.liveSyncActive = true;
    };

    eventSource.onmessage = (e) => {
      try {
        const evt = JSON.parse(e.data);
        handleIncomingEvent(evt);
      } catch {
        // Non-JSON ping/heartbeat
      }
    };

    eventSource.onerror = () => {
      nasState.liveSyncActive = false;
      if (eventSource && eventSource.readyState === EventSource.CLOSED) {
        stopEventStream();
        clearTimeout(reconnectTimer);
        reconnectTimer = setTimeout(() => {
          if (nasState.authenticated) {
            startEventStream();
          }
        }, 3000);
      }
    };
  } catch (err) {
    nasState.liveSyncActive = false;
    console.warn('Failed to start SSE event stream', err);
  }
}

export function stopEventStream(): void {
  nasState.liveSyncActive = false;
  clearTimeout(reconnectTimer);
  if (eventSource) {
    eventSource.close();
    eventSource = null;
  }
}

function handleIncomingEvent(evt: any): void {
  if (!evt || !evt.topic) return;

  switch (evt.topic) {
    case 'docker':
      refreshDockerData();
      break;
    case 'zfs':
      if (nasState.selectedPoolName) {
        refreshZfsSubresources(nasState.selectedPoolName);
      }
      services.zfs.listPools().then((pools) => {
        nasState.pools = pools;
      }).catch(() => {});
      break;
    case 'kvm':
      services.kvm.listVirtualMachines().then((vms) => {
        nasState.vms = vms;
      }).catch(() => {});
      break;
    case 'lxc':
      services.lxc.listContainers().then((c) => {
        nasState.lxcContainers = c;
      }).catch(() => {});
      break;
    default:
      refreshAllData();
      break;
  }
}

// An expired or revoked session drops the UI back to the login modal.
aipClient.onUnauthorized = () => {
  if (!nasState.authenticated) return;
  stopWatchStream();
  stopEventStream();
  services.auth.logout();
  nasState.authenticated = false;
  nasState.currentUser = null;
};

export async function initSession(): Promise<void> {
  if (services.auth.isAuthenticated()) {
    try {
      nasState.currentUser = await services.auth.getCurrentUser();
      nasState.authenticated = true;
      startWatchStream();
      startEventStream();
      await refreshAllData();
      return;
    } catch {
      services.auth.logout();
      nasState.authenticated = false;
      nasState.currentUser = null;
    }
  }
  try {
    nasState.prerequisites = await services.prerequisites.listPrerequisites();
  } catch {
    // ignore until login
  }
}

export { activeModal, askModal, confirmModal, alertModal, closeActiveModal } from './modal';
export type { ModalOptions, ModalResult, ModalTone } from './modal';
