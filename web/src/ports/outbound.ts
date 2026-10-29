/**
 * Outbound Port Interfaces for the Hexagonal Frontend Architecture.
 * Application services depend strictly on these interfaces and domain types.
 */

import type {
  AuthSession,
  CatalogApp,
  CifsShare,
  CifsGlobalConfig,
  DirectoryList,
  Disk,
  DiscoveredPool,
  DockerApp,
  DockerConfig,
  DockerContainer,
  DockerNetworkInfo,
  Group,
  HostPortStatus,
  LxcCatalogItem,
  LxcContainer,
  Prerequisite,
  PrerequisiteSummary,
  StorageImage,
  SystemConfig,
  SystemTelemetry,
  UploadProgressCallback,
  User,
  VirtualMachine,
  VirtualMachineConsoleSession,
  VirtualMachineSnapshot,
  WebdavShare,
  ZfsDataset,
  ZfsPool,
  ZfsVolume,
  DockerRegistry,
  DockerRegistryInput,
} from '../domain/models';

export interface AuthPort {
  login(username: string, password: string): Promise<AuthSession>;
  getCurrentUser(): Promise<User>;
  logout(): void;
  getToken(): string | null;
  setToken(token: string | null): void;
}

export interface SystemPort {
  getSystemConfig(): Promise<SystemConfig>;
  updateSystemConfig(config: Partial<SystemConfig>): Promise<SystemConfig>;
  selectMainPool(poolName: string, autoImportExisting?: boolean): Promise<SystemConfig>;
  discoverPools(): Promise<DiscoveredPool[]>;
  getTelemetry(): Promise<SystemTelemetry>;
}

export interface PrerequisitePort {
  listPrerequisites(): Promise<Prerequisite[]>;
  getPrerequisite(name: string): Promise<Prerequisite>;
  checkPrerequisites(): Promise<PrerequisiteSummary>;
}

export interface DiskPort {
  listDisks(filter?: string): Promise<Disk[]>;
  getDisk(name: string): Promise<Disk>;
}

export interface ZfsPort {
  // ZFS Pools
  listPools(): Promise<ZfsPool[]>;
  getPool(name: string): Promise<ZfsPool>;
  createPool(poolId: string, pool: Partial<ZfsPool>): Promise<ZfsPool>;
  updatePool(name: string, update: Partial<ZfsPool>): Promise<ZfsPool>;
  deletePool(name: string, force?: boolean): Promise<void>;
  exportPool(name: string, force?: boolean): Promise<void>;
  importPool(poolName: string, force?: boolean, altroot?: string): Promise<ZfsPool>;
  scrubPool(name: string): Promise<ZfsPool>;
  addVdev(name: string, req: { type: string; layout?: string; devices: string[] }): Promise<ZfsPool>;
  removeVdev(name: string, device: string): Promise<ZfsPool>;

  // ZFS Datasets
  listDatasets(parentPoolName: string): Promise<ZfsDataset[]>;
  getDataset(name: string): Promise<ZfsDataset>;
  createDataset(parentPoolName: string, datasetId: string, dataset: Partial<ZfsDataset>): Promise<ZfsDataset>;
  updateDataset(name: string, dataset: Partial<ZfsDataset>, updateMask: string[]): Promise<ZfsDataset>;
  deleteDataset(name: string, recursive?: boolean): Promise<void>;
  snapshotDataset(name: string, snapshotName: string): Promise<ZfsDataset>;

  // ZFS Volumes (ZVols)
  listVolumes(parentPoolName: string): Promise<ZfsVolume[]>;
  getVolume(name: string): Promise<ZfsVolume>;
  createVolume(parentPoolName: string, volumeId: string, volume: Partial<ZfsVolume>): Promise<ZfsVolume>;
  updateVolume(name: string, volume: Partial<ZfsVolume>, updateMask: string[]): Promise<ZfsVolume>;
  deleteVolume(name: string): Promise<void>;
}

export interface KvmPort {
  listVirtualMachines(): Promise<VirtualMachine[]>;
  getVirtualMachine(name: string): Promise<VirtualMachine>;
  createVirtualMachine(vmId: string, vm: Partial<VirtualMachine>): Promise<VirtualMachine>;
  updateVirtualMachine(name: string, vm: Partial<VirtualMachine>, updateMask: string[]): Promise<VirtualMachine>;
  deleteVirtualMachine(name: string): Promise<void>;
  startVirtualMachine(name: string): Promise<VirtualMachine>;
  stopVirtualMachine(name: string, force?: boolean): Promise<VirtualMachine>;
  rebootVirtualMachine(name: string, force?: boolean): Promise<VirtualMachine>;
  resetVirtualMachine(name: string): Promise<VirtualMachine>;
  pauseVirtualMachine(name: string): Promise<VirtualMachine>;
  resumeVirtualMachine(name: string): Promise<VirtualMachine>;
  cloneVirtualMachine(name: string, newId: string, newName?: string, targetPool?: string, linkedClone?: boolean): Promise<VirtualMachine>;
  attachMedia(name: string, isoPath: string, deviceId?: string): Promise<VirtualMachine>;
  ejectMedia(name: string, deviceId?: string): Promise<VirtualMachine>;
  listSnapshots(vmName: string): Promise<VirtualMachineSnapshot[]>;
  createSnapshot(vmName: string, snapshotId: string, description?: string, includeRam?: boolean): Promise<VirtualMachineSnapshot>;
  rollbackSnapshot(snapName: string): Promise<VirtualMachine>;
  deleteSnapshot(snapName: string): Promise<void>;
  openVirtualMachineConsole(name: string): Promise<VirtualMachineConsoleSession>;
  listIsos(): Promise<StorageImage[]>;
  uploadIso(file: File, onProgress?: UploadProgressCallback): Promise<StorageImage>;
  deleteIso(filename: string): Promise<void>;
}

export interface DockerPort {
  listContainers(): Promise<DockerContainer[]>;
  getContainer(name: string): Promise<DockerContainer>;
  createContainer(containerId: string, container: Partial<DockerContainer>): Promise<DockerContainer>;
  updateContainer(name: string, container: Partial<DockerContainer>, updateMask?: string[]): Promise<DockerContainer>;
  deleteContainer(name: string, force?: boolean): Promise<void>;
  startContainer(name: string): Promise<DockerContainer>;
  stopContainer(name: string): Promise<DockerContainer>;
  restartContainer(name: string): Promise<DockerContainer>;
  getContainerLogs(name: string, tail?: number): Promise<string>;
}

export interface DockerAppPort {
  listApps(): Promise<DockerApp[]>;
  getApp(name: string): Promise<DockerApp>;
  createApp(appId: string, composeYaml: string, title?: string): Promise<DockerApp>;
  updateApp(name: string, composeYaml: string, title?: string): Promise<DockerApp>;
  deleteApp(name: string, deleteVolumes?: boolean): Promise<void>;
  startApp(name: string): Promise<DockerApp>;
  stopApp(name: string): Promise<DockerApp>;
  restartApp(name: string): Promise<DockerApp>;
  upgradeApp(name: string, takeSnapshot?: boolean): Promise<DockerApp>;
  getAppLogs(name: string, tail?: number, serviceName?: string): Promise<{ logs: string; lineCount: number }>;
  getDockerConfig(): Promise<DockerConfig>;
  updateDockerConfig(dataset: string, pool?: string): Promise<DockerConfig>;
  listDirectories(path?: string): Promise<DirectoryList>;
  checkHostPort(port: number, protocol?: string, excludeAppId?: string): Promise<HostPortStatus>;
  checkHostPorts(ports: number[], protocol?: string, excludeAppId?: string): Promise<HostPortStatus[]>;
  listNetworks(): Promise<DockerNetworkInfo[]>;
  listRegistries(): Promise<DockerRegistry[]>;
  createRegistry(input: DockerRegistryInput): Promise<DockerRegistry>;
  updateRegistry(name: string, input: DockerRegistryInput): Promise<DockerRegistry>;
  deleteRegistry(name: string): Promise<void>;
}

export interface LxcPort {
  listContainers(): Promise<LxcContainer[]>;
  getContainer(name: string): Promise<LxcContainer>;
  createContainer(containerId: string, container: Partial<LxcContainer>): Promise<LxcContainer>;
  updateContainer(name: string, container: Partial<LxcContainer>, updateMask?: string[]): Promise<LxcContainer>;
  deleteContainer(name: string, force?: boolean): Promise<void>;
  startContainer(name: string): Promise<LxcContainer>;
  stopContainer(name: string): Promise<LxcContainer>;
  restartContainer(name: string): Promise<LxcContainer>;
  freezeContainer(name: string): Promise<LxcContainer>;
  getContainerLogs(name: string, tail?: number): Promise<string>;
  listTemplates(): Promise<StorageImage[]>;
  uploadTemplate(file: File, onProgress?: UploadProgressCallback): Promise<StorageImage>;
  deleteTemplate(filename: string): Promise<void>;
  listCatalogItems(source?: string, category?: string, query?: string): Promise<{ items: LxcCatalogItem[]; totalSize: number; lastSyncTime: string }>;
  getCatalogItem(id: string): Promise<LxcCatalogItem>;
  syncCatalog(force?: boolean): Promise<{ success: boolean; message: string; totalItems: number; lastSyncTime: string }>;
}

export interface SharePort {
  // CIFS / Samba
  listCifsShares(): Promise<CifsShare[]>;
  getCifsShare(name: string): Promise<CifsShare>;
  createCifsShare(shareId: string, share: Partial<CifsShare>): Promise<CifsShare>;
  updateCifsShare(name: string, share: Partial<CifsShare>, updateMask: string[]): Promise<CifsShare>;
  deleteCifsShare(name: string): Promise<void>;
  getCifsGlobalConfig(): Promise<CifsGlobalConfig>;
  updateCifsGlobalConfig(config: Partial<CifsGlobalConfig>, updateMask?: string[]): Promise<CifsGlobalConfig>;

  // WebDAV
  listWebdavShares(): Promise<WebdavShare[]>;
  getWebdavShare(name: string): Promise<WebdavShare>;
  createWebdavShare(shareId: string, share: Partial<WebdavShare>): Promise<WebdavShare>;
  updateWebdavShare(name: string, share: Partial<WebdavShare>, updateMask: string[]): Promise<WebdavShare>;
  deleteWebdavShare(name: string): Promise<void>;
}

export interface IdentityPort {
  // Linux Users
  listUsers(includeSystemAccounts?: boolean): Promise<User[]>;
  getUser(name: string): Promise<User>;
  createUser(userId: string, user: Partial<User>): Promise<User>;
  updateUser(name: string, user: Partial<User>, updateMask: string[]): Promise<User>;
  deleteUser(name: string, removeHome?: boolean): Promise<void>;

  // Linux Groups
  listGroups(includeSystemGroups?: boolean): Promise<Group[]>;
  getGroup(name: string): Promise<Group>;
  createGroup(groupId: string, group: Partial<Group>): Promise<Group>;
  updateGroup(name: string, group: Partial<Group>, updateMask: string[]): Promise<Group>;
  deleteGroup(name: string): Promise<void>;
}

export interface CatalogPort {
  listApps(train?: string, category?: string, query?: string): Promise<{ apps: CatalogApp[]; totalSize: number; lastSyncTime?: string }>;
  getApp(appId: string, train?: string): Promise<CatalogApp>;
  syncCatalog(force?: boolean): Promise<{ success: boolean; message: string; totalApps: number; lastSyncTime: string }>;
}
