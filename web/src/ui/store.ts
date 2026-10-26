/**
 * Global Reactive NAS State Store (Vue 3 Composition API)
 * Orchestrates Application Services and exposes reactive collections to UI Views.
 */

import { reactive } from 'vue';
import { aipClient, services } from '../container';
import type {
  CifsShare,
  Disk,
  DockerApp,
  DockerConfig,
  DockerContainer,
  Group,
  LxcContainer,
  Prerequisite,
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
  | 'identity';

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
  type: 'success' | 'error' | 'info';
  message: string;
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

function loadLocalCache(): LocalCachePayload | null {
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      const raw = window.localStorage.getItem('aegis_nas_cache_v1');
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
      window.localStorage.setItem('aegis_nas_cache_v1', JSON.stringify(data));
    }
  } catch {
    // ignore
  }
}

const cached = loadLocalCache();

export const nasState = reactive({
  activeTab: 'overview' as NavTab,
  authenticated: services.auth.isAuthenticated(),
  currentUser: null as User | null,
  loading: false,
  initialLoaded: Boolean(cached?.systemConfig),
  lastRefreshed: cached?.lastRefreshed || '',
  liveSyncActive: false,

  systemConfig: cached?.systemConfig || null,
  systemTelemetry: cached?.systemTelemetry || (null as SystemTelemetry | null),
  prerequisites: cached?.prerequisites || [],
  disks: cached?.disks || [],
  pools: cached?.pools || [],
  datasetsByPool: {} as Record<string, ZfsDataset[]>,
  volumesByPool: {} as Record<string, ZfsVolume[]>,
  selectedPoolName: (cached?.pools && cached.pools.length > 0 ? cached.pools[0].name : '') as string,

  vms: [] as VirtualMachine[],
  dockerContainers: [] as DockerContainer[],
  dockerApps: cached?.dockerApps || [],
  dockerConfig: null as DockerConfig | null,
  lxcContainers: [] as LxcContainer[],
  cifsShares: [] as CifsShare[],
  webdavShares: [] as WebdavShare[],
  users: [] as User[],
  groups: [] as Group[],
  includeSystemAccounts: false,
  includeSystemGroups: false,

  toasts: [] as ToastNotification[],
});

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

export function notify(type: 'success' | 'error' | 'info', message: string): void {
  const id = nextToastId++;
  nasState.toasts.push({ id, type, message });
  setTimeout(() => {
    const idx = nasState.toasts.findIndex((t) => t.id === id);
    if (idx !== -1) nasState.toasts.splice(idx, 1);
  }, 4200);
}

export async function refreshZfsSubresources(poolName: string): Promise<void> {
  if (!poolName || !getModuleAvailability('zfs').enabled) return;
  const [datasets, volumes] = await Promise.all([
    services.zfs.listDatasets(poolName).catch(() => []),
    services.zfs.listVolumes(poolName).catch(() => []),
  ]);
  nasState.datasetsByPool[poolName] = datasets;
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
      services.disk.listDisks().catch(() => []),
      zfsOk ? services.zfs.listPools().catch(() => []) : Promise.resolve([]),
      kvmOk ? services.kvm.listVirtualMachines().catch(() => []) : Promise.resolve([]),
      dockerOk ? services.docker.listContainers().catch(() => []) : Promise.resolve([]),
      dockerOk ? services.apps.listApps().catch(() => []) : Promise.resolve([]),
      lxcOk ? services.lxc.listContainers().catch(() => []) : Promise.resolve([]),
      cifsOk ? services.shares.listCifsShares().catch(() => []) : Promise.resolve([]),
      services.shares.listWebdavShares().catch(() => []),
      services.identity.listUsers(nasState.includeSystemAccounts).catch(() => []),
      services.identity.listGroups(nasState.includeSystemGroups).catch(() => []),
    ]);

    nasState.disks = disks;
    nasState.pools = pools;
    nasState.vms = vms;
    nasState.dockerContainers = dockerContainers;
    nasState.dockerApps = dockerApps;
    nasState.lxcContainers = lxcContainers;
    nasState.cifsShares = cifsShares;
    nasState.webdavShares = webdavShares;
    nasState.users = users;
    nasState.groups = groups;

    if (pools.length > 0 && zfsOk) {
      if (
        !nasState.selectedPoolName ||
        !pools.some((p) => p.name === nasState.selectedPoolName)
      ) {
        nasState.selectedPoolName = pools[0].name;
      }
      await Promise.all(pools.map((p) => refreshZfsSubresources(p.name)));
    } else {
      nasState.selectedPoolName = '';
    }

    nasState.lastRefreshed = new Date().toLocaleTimeString();
    saveLocalCache();
  } finally {
    nasState.initialLoaded = true;
    nasState.loading = false;
  }
}

export async function refreshDockerData(): Promise<void> {
  const dockerOk = getModuleAvailability('docker').enabled;
  if (!dockerOk) return;
  try {
    const [containers, apps] = await Promise.all([
      services.docker.listContainers().catch(() => []),
      services.apps.listApps().catch(() => []),
    ]);
    nasState.dockerContainers = containers;
    nasState.dockerApps = apps;
    nasState.lastRefreshed = new Date().toLocaleTimeString();
    saveLocalCache();
  } catch {
    // ignore
  }
}

export async function refreshTelemetry(): Promise<void> {
  try {
    const telem = await services.system.getTelemetry();
    if (telem) {
      nasState.systemTelemetry = telem;
    }
  } catch {
    // ignore
  }
}

let eventSource: EventSource | null = null;
let reconnectTimer: any = null;

export function startEventStream(): void {
  if (typeof window === 'undefined' || !window.EventSource) return;
  if (eventSource) return;

  const token = aipClient.getToken();
  const url = token ? `/v1/events?token=${encodeURIComponent(token)}` : '/v1/events';

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

export async function initSession(): Promise<void> {
  if (services.auth.isAuthenticated()) {
    try {
      nasState.currentUser = await services.auth.getCurrentUser();
      nasState.authenticated = true;
      await refreshAllData();
      startEventStream();
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
