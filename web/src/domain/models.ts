/**
 * Pure Domain Layer for AegisNAS Management Interface.
 * Defines Google AIP-compliant resource entities, value objects,
 * resource name builders/parsers, and formatting helpers.
 */

// ---------------------------------------------------------------------------
// Authentication & Session
// ---------------------------------------------------------------------------

export interface AuthSession {
  accessToken: string;
  tokenType: string;
  expiresIn: number;
  user: User;
}

// ---------------------------------------------------------------------------
// System Prerequisites (`prerequisites/{prerequisite}`)
// ---------------------------------------------------------------------------

export type PrerequisiteStatus = 'INSTALLED' | 'MISSING' | 'DEGRADED' | 'FALLBACK_ACTIVE';
export type PrerequisiteCategory = 'STORAGE' | 'VIRTUALIZATION' | 'CONTAINERS' | 'SHARING' | 'SECURITY';
export type PrerequisiteKind = 'BINARY' | 'LIBRARY' | 'KERNEL_MODULE' | 'DAEMON';

export interface Prerequisite {
  name: string; // prerequisites/{prerequisite}
  displayName: string;
  category: PrerequisiteCategory | string;
  kind: PrerequisiteKind | string;
  binaryOrPath: string;
  status: PrerequisiteStatus | string;
  version: string;
  details: string;
  installHint: string;
  requiredBy: string[];
  fallbackMode: boolean;
  checkedTime?: string;
}

export interface PrerequisiteSummary {
  prerequisites: Prerequisite[];
  installedCount: number;
  missingCount: number;
  hybridFallbackEnabled: boolean;
}

// ---------------------------------------------------------------------------
// Physical Disks & Storage Hardware (`disks/{disk}`)
// ---------------------------------------------------------------------------

export interface DiskPartition {
  name: string;
  devicePath: string;
  sizeBytes: number;
  fstype: string;
  mountpoint: string;
  label: string;
  owningZfsPool?: string;
}

export interface SmartParameter {
  id: number;
  name: string;
  value: string;
  worst: string;
  threshold: string;
  rawValue: string;
  status: string;
  type: string;
  description?: string;
}

export interface Disk {
  name: string; // disks/{disk}
  diskId: string;
  devicePath: string;
  sizeBytes: number;
  model: string;
  serial: string;
  vendor: string;
  transport: string;
  rotational: boolean;
  type: string; // "NVME", "SSD", "HDD", "VIRTUAL"
  firmwareRev: string;
  state: string;
  removable: boolean;
  logicalBlockSize: number;
  physicalBlockSize: number;
  temperatureCelsius: number;
  smartStatus: 'PASSED' | 'FAILED' | 'UNKNOWN' | string;
  healthPercentage: number;
  owningZfsPool: string;
  partitions: DiskPartition[];
  smartParameters?: SmartParameter[];
  powerOnHours?: number;
  powerCycleCount?: number;
  unsafeShutdowns?: number;
  mediaErrors?: number;
  totalBytesRead?: number;
  totalBytesWritten?: number;
  reallocatedSectors?: number;
  pendingSectors?: number;
}

// ---------------------------------------------------------------------------
// ZFS Storage (`zfsPools/{pool}`, `.../datasets/{dataset}`, `.../volumes/{volume}`)
// ---------------------------------------------------------------------------

export type ZfsPoolState = 'ONLINE' | 'DEGRADED' | 'FAULTED';
export type ZfsVdevLayout = 'stripe' | 'mirror' | 'raidz1' | 'raidz2';
export type ZfsCompression = 'lz4' | 'zstd' | 'off' | 'gzip';

export interface ZfsVdev {
  name: string;
  type: 'data' | 'cache' | 'log' | 'special' | 'dedup' | 'spare' | string;
  layout: string;
  state: string;
  path?: string;
  readErrors?: string;
  writeErrors?: string;
  checksumErrors?: string;
  children?: ZfsVdev[];
}

export interface ZfsPool {
  name: string; // zfsPools/{zfs_pool}
  poolName: string;
  state: ZfsPoolState | string;
  sizeBytes: number;
  allocatedBytes: number;
  freeBytes: number;
  fragmentationPercent: number;
  capacityPercent: number;
  dedupRatio: number;
  vdevLayout: ZfsVdevLayout | string;
  devices: string[];
  mountpoint: string;
  compression: ZfsCompression | string;
  lastScrubStatus: string;
  healthDetails: string;
  sync?: 'standard' | 'always' | 'disabled' | string;
  autotrim?: 'on' | 'off' | string;
  vdevs?: ZfsVdev[];
}

export interface ZfsDataset {
  name: string; // zfsPools/{zfs_pool}/datasets/{dataset}
  datasetPath: string;
  mountpoint: string;
  usedBytes: number;
  availableBytes: number;
  referencedBytes: number;
  quotaBytes: number;
  compression: ZfsCompression | string;
  readonly: boolean;
  atime: boolean;
  recordsize: string;
  snapshotsCount: number;
  lastSnapshot: string;
  sync?: 'standard' | 'always' | 'disabled' | string;
  reservationBytes?: number;
  dedup?: boolean;
}

export interface ZfsVolume {
  name: string; // zfsPools/{zfs_pool}/volumes/{volume}
  volumePath: string;
  devicePath: string;
  sizeBytes: number;
  usedBytes: number;
  volblocksize: string;
  compression: ZfsCompression | string;
  thinProvisioned: boolean;
  sync?: 'standard' | 'always' | 'disabled' | string;
  readonly?: boolean;
  dedup?: boolean;
}

// ---------------------------------------------------------------------------
// KVM Virtual Machines (`virtualMachines/{virtual_machine}`)
// ---------------------------------------------------------------------------

export type VirtualMachineState = 'RUNNING' | 'STOPPED' | 'PAUSED';

export interface VirtualMachine {
  name: string; // virtualMachines/{virtual_machine}
  vmName: string;
  uuid: string;
  state: VirtualMachineState | string;
  vcpus: number;
  memoryMib: number;
  diskPath: string;
  diskSizeGib: number;
  isoImage: string;
  networkBridge: string;
  vncPort: number;
  vncDisplay: string;
  autostart: boolean;
  osVariant: string;
  description: string;
}

export interface VirtualMachineConsoleSession {
  vmName: string;
  token: string;
  websocketUrl: string;
  vncPort: number;
  resolution: string;
  expiresAt: string;
}

// ---------------------------------------------------------------------------
// Docker Containers (`dockerContainers/{docker_container}`)
// ---------------------------------------------------------------------------

export type DockerContainerState = 'RUNNING' | 'EXITED' | 'CREATED' | 'PAUSED';

export interface DockerContainer {
  name: string; // dockerContainers/{docker_container}
  containerId: string;
  containerName: string;
  image: string;
  state: DockerContainerState | string;
  statusText: string;
  ports: string[];
  volumes: string[];
  environment: string[];
  restartPolicy: string;
  networkMode: string;
  cpuPercent: number;
  memoryUsageMib: number;
  createdAt: string;
  memoryLimitMib?: number;
  command?: string[];
  privileged?: boolean;
  composeYaml?: string;
}

// ---------------------------------------------------------------------------
// Docker Apps (Compose Descriptors) (`dockerApps/{docker_app}`)
// ---------------------------------------------------------------------------

export type DockerAppState = 'RUNNING' | 'STOPPED' | 'PARTIALLY_RUNNING' | 'ERROR';

export interface DockerAppContainer {
  containerId: string;
  name: string;
  serviceName: string;
  image: string;
  state: string;
  status: string;
  cpuPercent: number;
  memoryMib: number;
  ports: string[];
  createdAt: string;
  netRxBytes?: number;
  netTxBytes?: number;
  netRxRate?: number;
  netTxRate?: number;
}

export interface DockerApp {
  name: string; // dockerApps/{docker_app}
  appId: string;
  title: string;
  state: DockerAppState | string;
  statusText: string;
  composeYaml: string;
  composeFilePath: string;
  services: string[];
  containers: DockerAppContainer[];
  portMappings: string[];
  volumes: string[];
  webUiUrl: string;
  totalCpuPercent: number;
  totalMemoryMib: number;
  totalNetRxBytes?: number;
  totalNetTxBytes?: number;
  totalNetRxRate?: number;
  totalNetTxRate?: number;
  createdAt: string;
  updatedAt: string;
}

export interface AppCatalogTemplate {
  id: string;
  name: string;
  category: string;
  description: string;
  icon: string;
  defaultPort: number;
  composeYaml: string;
}

export interface DockerConfig {
  dataset: string;
  pool: string;
  mountpoint: string;
  appdataPath: string;
  dockerDataRoot: string;
  availableDatasets: string[];
  availablePools: string[];
  datasetSelected: boolean;
  appdataDataset?: string;
  poolSelected?: boolean;
}

export interface DirectoryEntry {
  name: string;
  path: string;
  isDir: boolean;
}

export interface DirectoryList {
  currentPath: string;
  parentPath: string;
  entries: DirectoryEntry[];
}

export interface CatalogApp {
  appId: string;
  title: string;
  description: string;
  category: string;
  categories: string[];
  iconUrl: string;
  home?: string;
  train: string;
  version?: string;
  appVersion?: string;
  tags?: string[];
  screenshots?: string[];
  composeYaml?: string;
  defaultPort?: number;
}

// ---------------------------------------------------------------------------
// LXC Containers (`lxcContainers/{lxc_container}`)
// ---------------------------------------------------------------------------

export type LxcContainerState = 'RUNNING' | 'STOPPED' | 'FROZEN';

export interface LxcContainer {
  name: string; // lxcContainers/{lxc_container}
  containerName: string;
  state: LxcContainerState | string;
  distribution: string;
  release: string;
  architecture: string;
  ipv4Address: string;
  autostart: boolean;
  unprivileged: boolean;
  rootfsPath: string;
  memoryLimitMib: number;
  cpuCores: number;
}

// ---------------------------------------------------------------------------
// File Shares (`cifsShares/{cifs_share}` & `webdavShares/{webdav_share}`)
// ---------------------------------------------------------------------------

export interface CifsShare {
  name: string; // cifsShares/{cifs_share}
  shareName: string;
  path: string;
  comment: string;
  readOnly: boolean;
  guestOk: boolean;
  browseable: boolean;
  validUsers: string[];
  validGroups: string[];
  createMask: string;
  directoryMask: string;
  enabled: boolean;
}

export interface WebdavShare {
  name: string; // webdavShares/{webdav_share}
  shareName: string;
  path: string;
  urlPrefix: string;
  description: string;
  readOnly: boolean;
  allowedUsers: string[];
  allowedGroups: string[];
  enabled: boolean;
}

// ---------------------------------------------------------------------------
// Linux Users & Groups (`users/{user}` & `groups/{group}`)
// ---------------------------------------------------------------------------

export interface User {
  name: string; // users/{user}
  username: string;
  uid: number;
  primaryGid: number;
  primaryGroup: string;
  supplementaryGroups: string[];
  fullName: string;
  homeDirectory: string;
  shell: string;
  sshEnabled: boolean;
  sshPasswordAuth: boolean;
  sshAuthorizedKeys: string[];
  locked: boolean;
  systemAccount: boolean;
  password?: string;
  isAdmin: boolean;
}

export interface Group {
  name: string; // groups/{group}
  groupName: string;
  gid: number;
  members: string[];
  systemGroup: boolean;
}

// ---------------------------------------------------------------------------
// Domain Helpers: Google AIP Resource Name Builders & Parsers
// ---------------------------------------------------------------------------

export const ResourceNames = {
  prerequisite: (id: string): string =>
    id.startsWith('prerequisites/') ? id : `prerequisites/${id}`,

  zfsPool: (poolId: string): string =>
    poolId.startsWith('zfsPools/') ? poolId : `zfsPools/${poolId}`,

  zfsDataset: (poolIdOrName: string, datasetId: string): string => {
    const pool = ResourceNames.zfsPool(poolIdOrName);
    return `${pool}/datasets/${datasetId}`;
  },

  zfsVolume: (poolIdOrName: string, volumeId: string): string => {
    const pool = ResourceNames.zfsPool(poolIdOrName);
    return `${pool}/volumes/${volumeId}`;
  },

  virtualMachine: (vmId: string): string =>
    vmId.startsWith('virtualMachines/') ? vmId : `virtualMachines/${vmId}`,

  dockerContainer: (containerId: string): string =>
    containerId.startsWith('dockerContainers/') ? containerId : `dockerContainers/${containerId}`,

  dockerApp: (appId: string): string =>
    appId.startsWith('dockerApps/') ? appId : `dockerApps/${appId}`,

  lxcContainer: (containerId: string): string =>
    containerId.startsWith('lxcContainers/') ? containerId : `lxcContainers/${containerId}`,

  cifsShare: (shareId: string): string =>
    shareId.startsWith('cifsShares/') ? shareId : `cifsShares/${shareId}`,

  webdavShare: (shareId: string): string =>
    shareId.startsWith('webdavShares/') ? shareId : `webdavShares/${shareId}`,

  user: (userId: string): string =>
    userId.startsWith('users/') ? userId : `users/${userId}`,

  group: (groupId: string): string =>
    groupId.startsWith('groups/') ? groupId : `groups/${groupId}`,

  extractLeafId: (resourceName: string): string => {
    const parts = resourceName.split('/').filter(Boolean);
    return parts.length > 0 ? parts[parts.length - 1] : resourceName;
  },

  parseZfsSubResource: (
    resourceName: string
  ): { poolName: string; poolId: string; subType: 'datasets' | 'volumes'; itemId: string } | null => {
    const match = /^zfsPools\/([^/]+)\/(datasets|volumes)\/([^/]+)$/.exec(resourceName);
    if (!match) return null;
    return {
      poolName: `zfsPools/${match[1]}`,
      poolId: match[1],
      subType: match[2] as 'datasets' | 'volumes',
      itemId: match[3],
    };
  },
};

// ---------------------------------------------------------------------------
// Domain Helpers: Byte Size Formatting & Validation Rules
// ---------------------------------------------------------------------------

export function formatBytes(bytes: number, decimals = 1): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(dm))} ${sizes[i]}`;
}

export function gibToBytes(gib: number): number {
  return Math.round(gib * 1024 * 1024 * 1024);
}

export function bytesToGib(bytes: number, decimals = 1): number {
  if (!bytes || bytes <= 0) return 0;
  return parseFloat((bytes / (1024 * 1024 * 1024)).toFixed(decimals));
}

const RESOURCE_ID_PATTERN = /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$/;

export function isValidResourceId(id: string): boolean {
  return RESOURCE_ID_PATTERN.test(id.trim());
}

export function validateSshPublicKey(keyLine: string): boolean {
  const trimmed = keyLine.trim();
  if (!trimmed) return false;
  return /^(ssh-(rsa|ed25519|dss)|ecdsa-sha2-nistp(256|384|521)|sk-ssh-ed25519@openssh\.com)\s+[A-Za-z0-9+/=]+/.test(
    trimmed
  );
}

import {
  mdiCloud,
  mdiFilmstrip,
  mdiHomeAutomation,
  mdiServerNetwork,
  mdiShieldCheckOutline,
  mdiShieldKeyOutline,
  mdiWeb,
} from '@mdi/js';

export const APP_CATALOG_TEMPLATES: AppCatalogTemplate[] = [
  {
    id: 'nextcloud',
    name: 'Nextcloud Hub',
    category: 'Productivity & Cloud',
    description: 'Complete private cloud collaboration suite with PostgreSQL database and Redis caching.',
    icon: mdiCloud,
    defaultPort: 8080,
    composeYaml: `services:
  app:
    image: nextcloud:apache
    container_name: nextcloud-app
    restart: always
    ports:
      - "8080:80"
    environment:
      - POSTGRES_HOST=db
      - POSTGRES_DB=nextcloud
      - POSTGRES_USER=nextcloud
      - POSTGRES_PASSWORD=nextcloud
      - REDIS_HOST=redis
    volumes:
      - /tank/appdata/nextcloud:/var/www/html
    depends_on:
      - db
      - redis
  db:
    image: postgres:16-alpine
    container_name: nextcloud-db
    restart: always
    environment:
      - POSTGRES_DB=nextcloud
      - POSTGRES_USER=nextcloud
      - POSTGRES_PASSWORD=nextcloud
    volumes:
      - /tank/appdata/nextcloud-db:/var/lib/postgresql/data
  redis:
    image: redis:alpine
    container_name: nextcloud-redis
    restart: always
`,
  },
  {
    id: 'jellyfin',
    name: 'Jellyfin Media Server',
    category: 'Media & Streaming',
    description: 'The Free Software Media System with hardware transcoding support and web portal.',
    icon: mdiFilmstrip,
    defaultPort: 8096,
    composeYaml: `services:
  jellyfin:
    image: jellyfin/jellyfin:latest
    container_name: jellyfin-media
    restart: unless-stopped
    ports:
      - "8096:8096"
    volumes:
      - /tank/media:/media:ro
      - /tank/appdata/jellyfin:/config
`,
  },
  {
    id: 'vaultwarden',
    name: 'Vaultwarden',
    category: 'Security & Passwords',
    description: 'Lightweight, self-hosted Bitwarden-compatible password manager written in Rust.',
    icon: mdiShieldKeyOutline,
    defaultPort: 8085,
    composeYaml: `services:
  vaultwarden:
    image: vaultwarden/server:latest
    container_name: vaultwarden
    restart: unless-stopped
    ports:
      - "8085:80"
    environment:
      - SIGNUPS_ALLOWED=true
      - WEBSOCKET_ENABLED=true
    volumes:
      - /tank/appdata/vaultwarden:/data
`,
  },
  {
    id: 'pihole',
    name: 'Pi-hole & Unbound DNS',
    category: 'Networking & DNS',
    description: 'Network-wide ad blocking DNS sinkhole paired with Unbound recursive resolver.',
    icon: mdiShieldCheckOutline,
    defaultPort: 8081,
    composeYaml: `services:
  pihole:
    image: pihole/pihole:latest
    container_name: pihole-dns
    restart: unless-stopped
    ports:
      - "8081:80/tcp"
      - "53:53/tcp"
      - "53:53/udp"
    environment:
      - TZ=UTC
      - WEBPASSWORD=admin
    volumes:
      - /tank/appdata/pihole:/etc/pihole
      - /tank/appdata/dnsmasq:/etc/dnsmasq.d
`,
  },
  {
    id: 'plex',
    name: 'Plex Media Server',
    category: 'Media & Streaming',
    description: 'Organize personal video, music, and photo collections and stream to all your devices.',
    icon: mdiFilmstrip,
    defaultPort: 32400,
    composeYaml: `services:
  plex:
    image: lscr.io/linuxserver/plex:latest
    container_name: plex-media
    restart: unless-stopped
    network_mode: host
    environment:
      - PUID=1000
      - PGID=1000
      - VERSION=docker
    volumes:
      - /tank/appdata/plex:/config
      - /tank/media:/media:ro
`,
  },
  {
    id: 'nginx-proxy-manager',
    name: 'Nginx Proxy Manager',
    category: 'Networking & Reverse Proxy',
    description: 'Expose your self-hosted apps securely with free SSL certificates and clean GUI.',
    icon: mdiWeb,
    defaultPort: 81,
    composeYaml: `services:
  app:
    image: jc21/nginx-proxy-manager:latest
    container_name: npm-proxy
    restart: unless-stopped
    ports:
      - "80:80"
      - "81:81"
      - "443:443"
    volumes:
      - /tank/appdata/npm/data:/data
      - /tank/appdata/npm/letsencrypt:/etc/letsencrypt
`,
  },
  {
    id: 'homeassistant',
    name: 'Home Assistant',
    category: 'Smart Home & Automation',
    description: 'Open source home automation that puts local control and privacy first.',
    icon: mdiHomeAutomation,
    defaultPort: 8123,
    composeYaml: `services:
  homeassistant:
    image: ghcr.io/home-assistant/home-assistant:stable
    container_name: homeassistant
    restart: unless-stopped
    network_mode: host
    privileged: true
    volumes:
      - /tank/appdata/homeassistant:/config
      - /etc/localtime:/etc/localtime:ro
`,
  },
];
