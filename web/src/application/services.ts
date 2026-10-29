/**
 * Application Layer Services (Use-Case Orchestrators).
 * Strictly depend ONLY on `domain` and `ports` (Hexagonal Architecture).
 * Injected via constructor so they can be unit tested with mock ports.
 */

import {
  type AuthSession,
  type CifsShare,
  type CifsGlobalConfig,
  type DirectoryList,
  type Disk,
  type DockerApp,
  type DockerAppContainer,
  type DockerConfig,
  type DockerContainer,
  type DockerNetworkInfo,
  type Group,
  type HostPortStatus,
  isValidResourceId,
  type LxcCatalogItem,
  type LxcContainer,
  type Prerequisite,
  type PrerequisiteSummary,
  ResourceNames,
  type StorageImage,
  type SystemConfig,
  type SystemTelemetry,
  type UploadProgressCallback,
  type DiscoveredPool,
  type User,
  validateSshPublicKey,
  type VirtualMachine,
  type VirtualMachineConsoleSession,
  type VirtualMachineSnapshot,
  type WebdavShare,
  type ZfsDataset,
  type ZfsPool,
  type ZfsVolume,
  DockerRegistry,
  DockerRegistryInput,
} from '../domain/models';
import type {
  AuthPort,
  DiskPort,
  DockerAppPort,
  DockerPort,
  IdentityPort,
  KvmPort,
  LxcPort,
  PrerequisitePort,
  SharePort,
  SystemPort,
  ZfsPort,
} from '../ports/outbound';

// ---------------------------------------------------------------------------
// 1. AuthService
// ---------------------------------------------------------------------------

export class AuthService {
  constructor(private readonly authPort: AuthPort) {}

  async login(username: string, password: string): Promise<AuthSession> {
    if (!username.trim()) {
      throw new Error('Username is required');
    }
    if (!password) {
      throw new Error('Password is required');
    }
    const session = await this.authPort.login(username.trim(), password);
    this.authPort.setToken(session.accessToken);
    return session;
  }

  async getCurrentUser(): Promise<User> {
    return this.authPort.getCurrentUser();
  }

  logout(): void {
    this.authPort.logout();
  }

  isAuthenticated(): boolean {
    return Boolean(this.authPort.getToken());
  }
}

// ---------------------------------------------------------------------------
// 1b. SystemService
// ---------------------------------------------------------------------------

export class SystemService {
  constructor(private readonly systemPort: SystemPort) {}

  async getSystemConfig(): Promise<SystemConfig> {
    return this.systemPort.getSystemConfig();
  }

  async updateSystemConfig(config: Partial<SystemConfig>): Promise<SystemConfig> {
    return this.systemPort.updateSystemConfig(config);
  }

  async selectMainPool(poolName: string, autoImportExisting = true): Promise<SystemConfig> {
    if (!poolName.trim()) {
      throw new Error('Pool name is required');
    }
    return this.systemPort.selectMainPool(poolName.trim(), autoImportExisting);
  }

  async discoverPools(): Promise<DiscoveredPool[]> {
    return this.systemPort.discoverPools();
  }

  async getTelemetry(): Promise<SystemTelemetry> {
    return this.systemPort.getTelemetry();
  }
}

// ---------------------------------------------------------------------------
// 2. PrerequisiteService
// ---------------------------------------------------------------------------

export class PrerequisiteService {
  constructor(private readonly prerequisitePort: PrerequisitePort) {}

  async listPrerequisites(): Promise<Prerequisite[]> {
    return this.prerequisitePort.listPrerequisites();
  }

  async getPrerequisite(idOrName: string): Promise<Prerequisite> {
    return this.prerequisitePort.getPrerequisite(ResourceNames.prerequisite(idOrName));
  }

  async checkPrerequisites(): Promise<PrerequisiteSummary> {
    return this.prerequisitePort.checkPrerequisites();
  }
}

// ---------------------------------------------------------------------------
// 2b. DiskService
// ---------------------------------------------------------------------------

export class DiskService {
  constructor(private readonly diskPort: DiskPort) {}

  async listDisks(filter?: string): Promise<Disk[]> {
    return this.diskPort.listDisks(filter);
  }

  async getDisk(idOrName: string): Promise<Disk> {
    const name = idOrName.startsWith('disks/') ? idOrName : `disks/${idOrName}`;
    return this.diskPort.getDisk(name);
  }
}

// ---------------------------------------------------------------------------
// 3. ZfsService
// ---------------------------------------------------------------------------

export class ZfsService {
  constructor(private readonly zfsPort: ZfsPort) {}

  // Pools
  async listPools(): Promise<ZfsPool[]> {
    return this.zfsPort.listPools();
  }

  async getPool(poolIdOrName: string): Promise<ZfsPool> {
    return this.zfsPort.getPool(ResourceNames.zfsPool(poolIdOrName));
  }

  async createPool(poolId: string, pool: Partial<ZfsPool>): Promise<ZfsPool> {
    if (!isValidResourceId(poolId)) {
      throw new Error(`Invalid ZFS pool ID: "${poolId}"`);
    }
    const hasDevices = pool.devices && pool.devices.length > 0;
    const hasVdevs = pool.vdevs && pool.vdevs.length > 0;
    if (!hasDevices && !hasVdevs) {
      throw new Error('At least one backing device path is required to create a ZFS pool');
    }
    const cleanId = poolId.trim().replace(/^zfsPools\//, '');
    return this.zfsPort.createPool(cleanId, {
      vdevLayout: pool.vdevLayout || 'mirror',
      compression: pool.compression || 'lz4',
      mountpoint: pool.mountpoint || `/mnt/${cleanId}`,
      ...pool,
    });
  }

  async deletePool(poolIdOrName: string, force = false): Promise<void> {
    return this.zfsPort.deletePool(ResourceNames.zfsPool(poolIdOrName), force);
  }

  async exportPool(poolIdOrName: string, force = false): Promise<void> {
    return this.zfsPort.exportPool(ResourceNames.zfsPool(poolIdOrName), force);
  }

  async importPool(poolName: string, force = true, altroot = '/mnt'): Promise<ZfsPool> {
    const cleanName = poolName.trim().replace(/^zfsPools\//, '');
    if (!cleanName) {
      throw new Error('Pool name cannot be empty');
    }
    return this.zfsPort.importPool(cleanName, force, altroot);
  }

  async scrubPool(poolIdOrName: string): Promise<ZfsPool> {
    return this.zfsPort.scrubPool(ResourceNames.zfsPool(poolIdOrName));
  }

  async updatePool(poolIdOrName: string, update: Partial<ZfsPool>): Promise<ZfsPool> {
    return this.zfsPort.updatePool(ResourceNames.zfsPool(poolIdOrName), update);
  }

  async addVdev(
    poolIdOrName: string,
    req: { type: string; layout?: string; devices: string[] }
  ): Promise<ZfsPool> {
    if (!req.devices || req.devices.length === 0) {
      throw new Error('At least one device path is required to add VDEV');
    }
    return this.zfsPort.addVdev(ResourceNames.zfsPool(poolIdOrName), req);
  }

  async removeVdev(poolIdOrName: string, device: string): Promise<ZfsPool> {
    if (!device || !device.trim()) {
      throw new Error('Device path is required to remove VDEV');
    }
    return this.zfsPort.removeVdev(ResourceNames.zfsPool(poolIdOrName), device.trim());
  }

  // Datasets
  async listDatasets(poolIdOrName: string): Promise<ZfsDataset[]> {
    return this.zfsPort.listDatasets(ResourceNames.zfsPool(poolIdOrName));
  }

  async createDataset(
    poolIdOrName: string,
    datasetId: string,
    dataset: Partial<ZfsDataset>
  ): Promise<ZfsDataset> {
    if (!isValidResourceId(datasetId)) {
      throw new Error(`Invalid ZFS dataset ID: "${datasetId}"`);
    }
    const parent = ResourceNames.zfsPool(poolIdOrName);
    return this.zfsPort.createDataset(parent, datasetId.trim(), {
      compression: dataset.compression || 'lz4',
      recordsize: dataset.recordsize || '128K',
      atime: dataset.atime ?? false,
      readonly: dataset.readonly ?? false,
      quotaBytes: dataset.quotaBytes ?? 0,
      ...dataset,
    });
  }

  async updateDataset(
    datasetName: string,
    updates: Partial<Pick<ZfsDataset, 'compression' | 'quotaBytes' | 'readonly' | 'atime' | 'recordsize' | 'mountpoint' | 'sync' | 'reservationBytes' | 'dedup'>>,
    updateMask?: string[]
  ): Promise<ZfsDataset> {
    const mask =
      updateMask && updateMask.length > 0
        ? updateMask
        : Object.keys(updates).map((k) => {
            if (k === 'quotaBytes') return 'quota_bytes';
            if (k === 'reservationBytes') return 'reservation_bytes';
            return k;
          });
    return this.zfsPort.updateDataset(datasetName, updates, mask);
  }

  async deleteDataset(datasetName: string, recursive = false): Promise<void> {
    return this.zfsPort.deleteDataset(datasetName, recursive);
  }

  async snapshotDataset(datasetName: string, snapshotName: string): Promise<ZfsDataset> {
    if (!isValidResourceId(snapshotName)) {
      throw new Error(`Invalid snapshot name: "${snapshotName}"`);
    }
    return this.zfsPort.snapshotDataset(datasetName, snapshotName.trim());
  }

  // Volumes (ZVols)
  async listVolumes(poolIdOrName: string): Promise<ZfsVolume[]> {
    return this.zfsPort.listVolumes(ResourceNames.zfsPool(poolIdOrName));
  }

  async createVolume(
    poolIdOrName: string,
    volumeId: string,
    volume: Partial<ZfsVolume>
  ): Promise<ZfsVolume> {
    if (!isValidResourceId(volumeId)) {
      throw new Error(`Invalid ZFS volume ID: "${volumeId}"`);
    }
    if (!volume.sizeBytes || volume.sizeBytes <= 0) {
      throw new Error('Volume size must be greater than 0 bytes');
    }
    const parent = ResourceNames.zfsPool(poolIdOrName);
    return this.zfsPort.createVolume(parent, volumeId.trim(), {
      volblocksize: volume.volblocksize || '16K',
      compression: volume.compression || 'lz4',
      thinProvisioned: volume.thinProvisioned ?? true,
      sync: volume.sync || 'standard',
      readonly: volume.readonly ?? false,
      dedup: volume.dedup ?? false,
      ...volume,
    });
  }

  async updateVolume(
    volumeName: string,
    updates: Partial<Pick<ZfsVolume, 'sizeBytes' | 'compression' | 'thinProvisioned' | 'sync' | 'readonly' | 'dedup'>>,
    updateMask?: string[]
  ): Promise<ZfsVolume> {
    const mask =
      updateMask && updateMask.length > 0
        ? updateMask
        : Object.keys(updates).map((k) => {
            if (k === 'sizeBytes') return 'size_bytes';
            if (k === 'thinProvisioned') return 'thin_provisioned';
            return k;
          });
    return this.zfsPort.updateVolume(volumeName, updates, mask);
  }

  async resizeVolume(
    volumeName: string,
    sizeBytes: number,
    compression?: string
  ): Promise<ZfsVolume> {
    if (!sizeBytes || sizeBytes <= 0) {
      throw new Error('New volume size must be greater than 0 bytes');
    }
    const payload: Partial<ZfsVolume> = { sizeBytes };
    if (compression) {
      payload.compression = compression;
    }
    return this.updateVolume(volumeName, payload);
  }

  async deleteVolume(volumeName: string): Promise<void> {
    return this.zfsPort.deleteVolume(volumeName);
  }
}

// ---------------------------------------------------------------------------
// 4. KvmService
// ---------------------------------------------------------------------------

export class KvmService {
  constructor(private readonly kvmPort: KvmPort) {}

  async listVirtualMachines(): Promise<VirtualMachine[]> {
    return this.kvmPort.listVirtualMachines();
  }

  async getVirtualMachine(vmIdOrName: string): Promise<VirtualMachine> {
    return this.kvmPort.getVirtualMachine(ResourceNames.virtualMachine(vmIdOrName));
  }

  async createVirtualMachine(vmId: string, vm: Partial<VirtualMachine>): Promise<VirtualMachine> {
    if (!isValidResourceId(vmId)) {
      throw new Error(`Invalid VM ID: "${vmId}"`);
    }
    if ((vm.vcpus ?? 2) < 1) {
      throw new Error('Virtual machine must have at least 1 vCPU');
    }
    if ((vm.memoryMib ?? 2048) < 256) {
      throw new Error('Virtual machine must have at least 256 MiB RAM');
    }
    return this.kvmPort.createVirtualMachine(vmId.trim(), {
      vcpus: vm.vcpus ?? 2,
      memoryMib: vm.memoryMib ?? 2048,
      diskSizeGib: vm.diskSizeGib ?? 20,
      networkBridge: vm.networkBridge || 'virbr0',
      osVariant: vm.osVariant || 'debian12',
      autostart: vm.autostart ?? false,
      ...vm,
    });
  }

  async updateVirtualMachine(
    vmIdOrName: string,
    updates: Partial<VirtualMachine>,
    updateMask: string[]
  ): Promise<VirtualMachine> {
    return this.kvmPort.updateVirtualMachine(
      ResourceNames.virtualMachine(vmIdOrName),
      updates,
      updateMask
    );
  }

  async deleteVirtualMachine(vmIdOrName: string): Promise<void> {
    return this.kvmPort.deleteVirtualMachine(ResourceNames.virtualMachine(vmIdOrName));
  }

  async startVirtualMachine(vmIdOrName: string): Promise<VirtualMachine> {
    return this.kvmPort.startVirtualMachine(ResourceNames.virtualMachine(vmIdOrName));
  }

  async stopVirtualMachine(vmIdOrName: string, force = false): Promise<VirtualMachine> {
    return this.kvmPort.stopVirtualMachine(ResourceNames.virtualMachine(vmIdOrName), force);
  }

  async rebootVirtualMachine(vmIdOrName: string, force = false): Promise<VirtualMachine> {
    return this.kvmPort.rebootVirtualMachine(ResourceNames.virtualMachine(vmIdOrName), force);
  }

  async resetVirtualMachine(vmIdOrName: string): Promise<VirtualMachine> {
    return this.kvmPort.resetVirtualMachine(ResourceNames.virtualMachine(vmIdOrName));
  }

  async pauseVirtualMachine(vmIdOrName: string): Promise<VirtualMachine> {
    return this.kvmPort.pauseVirtualMachine(ResourceNames.virtualMachine(vmIdOrName));
  }

  async resumeVirtualMachine(vmIdOrName: string): Promise<VirtualMachine> {
    return this.kvmPort.resumeVirtualMachine(ResourceNames.virtualMachine(vmIdOrName));
  }

  async cloneVirtualMachine(
    vmIdOrName: string,
    newId: string,
    newName?: string,
    targetPool?: string,
    linkedClone = true
  ): Promise<VirtualMachine> {
    if (!isValidResourceId(newId)) {
      throw new Error(`Invalid new VM ID: "${newId}"`);
    }
    return this.kvmPort.cloneVirtualMachine(
      ResourceNames.virtualMachine(vmIdOrName),
      newId.trim(),
      newName,
      targetPool,
      linkedClone
    );
  }

  async attachMedia(vmIdOrName: string, isoPath: string, deviceId = ''): Promise<VirtualMachine> {
    return this.kvmPort.attachMedia(ResourceNames.virtualMachine(vmIdOrName), isoPath, deviceId);
  }

  async ejectMedia(vmIdOrName: string, deviceId = ''): Promise<VirtualMachine> {
    return this.kvmPort.ejectMedia(ResourceNames.virtualMachine(vmIdOrName), deviceId);
  }

  async listSnapshots(vmIdOrName: string): Promise<VirtualMachineSnapshot[]> {
    return this.kvmPort.listSnapshots(ResourceNames.virtualMachine(vmIdOrName));
  }

  async createSnapshot(
    vmIdOrName: string,
    snapshotId: string,
    description = '',
    includeRam = false
  ): Promise<VirtualMachineSnapshot> {
    if (!isValidResourceId(snapshotId)) {
      throw new Error(`Invalid snapshot ID: "${snapshotId}"`);
    }
    return this.kvmPort.createSnapshot(
      ResourceNames.virtualMachine(vmIdOrName),
      snapshotId.trim(),
      description,
      includeRam
    );
  }

  async rollbackSnapshot(snapName: string): Promise<VirtualMachine> {
    return this.kvmPort.rollbackSnapshot(snapName);
  }

  async deleteSnapshot(snapName: string): Promise<void> {
    return this.kvmPort.deleteSnapshot(snapName);
  }

  async openConsole(vmIdOrName: string): Promise<VirtualMachineConsoleSession> {
    return this.kvmPort.openVirtualMachineConsole(ResourceNames.virtualMachine(vmIdOrName));
  }

  async listIsos(): Promise<StorageImage[]> {
    return this.kvmPort.listIsos();
  }

  async uploadIso(file: File, onProgress?: UploadProgressCallback): Promise<StorageImage> {
    return this.kvmPort.uploadIso(file, onProgress);
  }

  async deleteIso(filename: string): Promise<void> {
    return this.kvmPort.deleteIso(filename);
  }
}

// ---------------------------------------------------------------------------
// 5. DockerService
// ---------------------------------------------------------------------------

export class DockerService {
  constructor(private readonly dockerPort: DockerPort) {}

  async listContainers(): Promise<DockerContainer[]> {
    return this.dockerPort.listContainers();
  }

  async getContainer(idOrName: string): Promise<DockerContainer> {
    return this.dockerPort.getContainer(ResourceNames.dockerContainer(idOrName));
  }

  async createContainer(
    containerId: string,
    container: Partial<DockerContainer>
  ): Promise<DockerContainer> {
    if (!isValidResourceId(containerId)) {
      throw new Error(`Invalid Docker container ID: "${containerId}"`);
    }
    if (!container.image || !container.image.trim()) {
      throw new Error('Container OCI image is required');
    }
    return this.dockerPort.createContainer(containerId.trim(), {
      restartPolicy: container.restartPolicy || 'unless-stopped',
      networkMode: container.networkMode || 'bridge',
      ports: container.ports || [],
      volumes: container.volumes || [],
      environment: container.environment || [],
      ...container,
    });
  }

  async deleteContainer(idOrName: string, force = true): Promise<void> {
    return this.dockerPort.deleteContainer(ResourceNames.dockerContainer(idOrName), force);
  }

  async startContainer(idOrName: string): Promise<DockerContainer> {
    return this.dockerPort.startContainer(ResourceNames.dockerContainer(idOrName));
  }

  async stopContainer(idOrName: string): Promise<DockerContainer> {
    return this.dockerPort.stopContainer(ResourceNames.dockerContainer(idOrName));
  }

  async restartContainer(idOrName: string): Promise<DockerContainer> {
    return this.dockerPort.restartContainer(ResourceNames.dockerContainer(idOrName));
  }

  async updateContainer(
    idOrName: string,
    container: Partial<DockerContainer>,
    updateMask?: string[]
  ): Promise<DockerContainer> {
    const fullName = ResourceNames.dockerContainer(idOrName);
    return this.dockerPort.updateContainer(fullName, container, updateMask);
  }

  async getContainerLogs(idOrName: string, tail = 100): Promise<string> {
    const fullName = ResourceNames.dockerContainer(idOrName);
    return this.dockerPort.getContainerLogs(fullName, tail);
  }
}

// ---------------------------------------------------------------------------
// 5.1 DockerAppService (TrueNAS SCALE Compose Descriptors)
// ---------------------------------------------------------------------------

export class DockerAppService {
  constructor(private readonly appPort: DockerAppPort) {}

  async listApps(): Promise<DockerApp[]> {
    return this.appPort.listApps();
  }

  async getApp(idOrName: string): Promise<DockerApp> {
    return this.appPort.getApp(ResourceNames.dockerApp(idOrName));
  }

  async createApp(appId: string, composeYaml: string, title?: string): Promise<DockerApp> {
    if (!isValidResourceId(appId)) {
      throw new Error(`Invalid Docker App ID: "${appId}"`);
    }
    if (!composeYaml || !composeYaml.trim()) {
      throw new Error('Docker Compose specification (compose.yaml) cannot be empty');
    }
    return this.appPort.createApp(appId.trim(), composeYaml, title);
  }

  async updateApp(idOrName: string, composeYaml: string, title?: string): Promise<DockerApp> {
    if (!composeYaml || !composeYaml.trim()) {
      throw new Error('Docker Compose specification (compose.yaml) cannot be empty');
    }
    return this.appPort.updateApp(ResourceNames.dockerApp(idOrName), composeYaml, title);
  }

  async deleteApp(idOrName: string, deleteVolumes = false): Promise<void> {
    return this.appPort.deleteApp(ResourceNames.dockerApp(idOrName), deleteVolumes);
  }

  async startApp(idOrName: string): Promise<DockerApp> {
    return this.appPort.startApp(ResourceNames.dockerApp(idOrName));
  }

  async stopApp(idOrName: string): Promise<DockerApp> {
    return this.appPort.stopApp(ResourceNames.dockerApp(idOrName));
  }

  async restartApp(idOrName: string): Promise<DockerApp> {
    return this.appPort.restartApp(ResourceNames.dockerApp(idOrName));
  }

  async upgradeApp(idOrName: string, takeSnapshot = true): Promise<DockerApp> {
    return this.appPort.upgradeApp(ResourceNames.dockerApp(idOrName), takeSnapshot);
  }

  async getAppLogs(idOrName: string, tail = 100, serviceName?: string): Promise<{ logs: string; lineCount: number }> {
    return this.appPort.getAppLogs(ResourceNames.dockerApp(idOrName), tail, serviceName);
  }

  async getDockerConfig(): Promise<DockerConfig> {
    return this.appPort.getDockerConfig();
  }

  async updateDockerConfig(dataset: string, pool?: string): Promise<DockerConfig> {
    if (!dataset || !dataset.trim()) {
      throw new Error('ZFS dataset name cannot be empty');
    }
    return this.appPort.updateDockerConfig(dataset.trim(), pool?.trim());
  }

  async listDirectories(path?: string): Promise<DirectoryList> {
    return this.appPort.listDirectories(path);
  }

  async checkHostPort(port: number, protocol?: string, excludeAppId?: string): Promise<HostPortStatus> {
    return this.appPort.checkHostPort(port, protocol, excludeAppId);
  }

  async checkHostPorts(ports: number[], protocol?: string, excludeAppId?: string): Promise<HostPortStatus[]> {
    return this.appPort.checkHostPorts(ports, protocol, excludeAppId);
  }

  async listNetworks(): Promise<DockerNetworkInfo[]> {
    return this.appPort.listNetworks();
  }

  async listRegistries(): Promise<DockerRegistry[]> {
    return this.appPort.listRegistries();
  }

  async saveRegistry(input: DockerRegistryInput, existingName?: string): Promise<DockerRegistry> {
    const clean: DockerRegistryInput = {
      server: input.server.trim(),
      username: input.username.trim(),
      password: input.password ?? '',
      description: (input.description ?? '').trim(),
    };
    if (!clean.server) throw new Error('Registry server is required');
    if (!clean.username) throw new Error('Username is required');
    if (!existingName && !clean.password) throw new Error('Password or access token is required');
    return existingName ? this.appPort.updateRegistry(existingName, clean) : this.appPort.createRegistry(clean);
  }

  async deleteRegistry(name: string): Promise<void> {
    return this.appPort.deleteRegistry(name);
  }
}

// ---------------------------------------------------------------------------
// 6. LxcService
// ---------------------------------------------------------------------------

export class LxcService {
  constructor(private readonly lxcPort: LxcPort) {}

  async listContainers(): Promise<LxcContainer[]> {
    return this.lxcPort.listContainers();
  }

  async getContainer(idOrName: string): Promise<LxcContainer> {
    return this.lxcPort.getContainer(ResourceNames.lxcContainer(idOrName));
  }

  async createContainer(
    containerId: string,
    container: Partial<LxcContainer>
  ): Promise<LxcContainer> {
    if (!isValidResourceId(containerId)) {
      throw new Error(`Invalid LXC container ID: "${containerId}"`);
    }
    return this.lxcPort.createContainer(containerId.trim(), {
      distribution: container.distribution || 'debian',
      release: container.release || 'bookworm',
      architecture: container.architecture || 'amd64',
      memoryLimitMib: container.memoryLimitMib ?? 1024,
      cpuCores: container.cpuCores ?? 2,
      unprivileged: container.unprivileged ?? true,
      autostart: container.autostart ?? false,
      ...container,
    });
  }

  async deleteContainer(idOrName: string, force = true): Promise<void> {
    return this.lxcPort.deleteContainer(ResourceNames.lxcContainer(idOrName), force);
  }

  async startContainer(idOrName: string): Promise<LxcContainer> {
    return this.lxcPort.startContainer(ResourceNames.lxcContainer(idOrName));
  }

  async stopContainer(idOrName: string): Promise<LxcContainer> {
    return this.lxcPort.stopContainer(ResourceNames.lxcContainer(idOrName));
  }

  async freezeContainer(idOrName: string): Promise<LxcContainer> {
    return this.lxcPort.freezeContainer(ResourceNames.lxcContainer(idOrName));
  }

  async restartContainer(idOrName: string): Promise<LxcContainer> {
    return this.lxcPort.restartContainer(ResourceNames.lxcContainer(idOrName));
  }

  async updateContainer(
    idOrName: string,
    container: Partial<LxcContainer>,
    updateMask?: string[]
  ): Promise<LxcContainer> {
    return this.lxcPort.updateContainer(
      ResourceNames.lxcContainer(idOrName),
      container,
      updateMask
    );
  }

  async getContainerLogs(idOrName: string, tail = 100): Promise<string> {
    return this.lxcPort.getContainerLogs(ResourceNames.lxcContainer(idOrName), tail);
  }

  async listTemplates(): Promise<StorageImage[]> {
    return this.lxcPort.listTemplates();
  }

  async uploadTemplate(file: File, onProgress?: UploadProgressCallback): Promise<StorageImage> {
    return this.lxcPort.uploadTemplate(file, onProgress);
  }

  async deleteTemplate(filename: string): Promise<void> {
    return this.lxcPort.deleteTemplate(filename);
  }

  async listCatalogItems(source?: string, category?: string, query?: string): Promise<{ items: LxcCatalogItem[]; totalSize: number; lastSyncTime: string }> {
    return this.lxcPort.listCatalogItems(source, category, query);
  }

  async getCatalogItem(id: string): Promise<LxcCatalogItem> {
    return this.lxcPort.getCatalogItem(id);
  }

  async syncCatalog(force?: boolean): Promise<{ success: boolean; message: string; totalItems: number; lastSyncTime: string }> {
    return this.lxcPort.syncCatalog(force);
  }
}

// ---------------------------------------------------------------------------
// 7. ShareService
// ---------------------------------------------------------------------------

export class ShareService {
  constructor(private readonly sharePort: SharePort) {}

  // CIFS
  async listCifsShares(): Promise<CifsShare[]> {
    return this.sharePort.listCifsShares();
  }

  async createCifsShare(shareId: string, share: Partial<CifsShare>): Promise<CifsShare> {
    if (!isValidResourceId(shareId)) {
      throw new Error(`Invalid CIFS share ID: "${shareId}"`);
    }
    if (!share.path || !share.path.startsWith('/')) {
      throw new Error('Share filesystem path must be an absolute path starting with "/"');
    }
    return this.sharePort.createCifsShare(shareId.trim(), {
      readOnly: share.readOnly ?? false,
      guestOk: share.guestOk ?? false,
      browseable: share.browseable ?? true,
      createMask: share.createMask || '0664',
      directoryMask: share.directoryMask || '0775',
      enabled: share.enabled ?? true,
      validUsers: share.validUsers || [],
      validGroups: share.validGroups || [],
      ...share,
    });
  }

  async updateCifsShare(
    shareIdOrName: string,
    updates: Partial<CifsShare>,
    updateMask: string[]
  ): Promise<CifsShare> {
    return this.sharePort.updateCifsShare(
      ResourceNames.cifsShare(shareIdOrName),
      updates,
      updateMask
    );
  }

  async deleteCifsShare(shareIdOrName: string): Promise<void> {
    return this.sharePort.deleteCifsShare(ResourceNames.cifsShare(shareIdOrName));
  }

  async getCifsGlobalConfig(): Promise<CifsGlobalConfig> {
    return this.sharePort.getCifsGlobalConfig();
  }

  async updateCifsGlobalConfig(
    config: Partial<CifsGlobalConfig>,
    updateMask?: string[]
  ): Promise<CifsGlobalConfig> {
    return this.sharePort.updateCifsGlobalConfig(config, updateMask);
  }

  // WebDAV
  async listWebdavShares(): Promise<WebdavShare[]> {
    return this.sharePort.listWebdavShares();
  }

  async createWebdavShare(shareId: string, share: Partial<WebdavShare>): Promise<WebdavShare> {
    if (!isValidResourceId(shareId)) {
      throw new Error(`Invalid WebDAV share ID: "${shareId}"`);
    }
    if (!share.path || !share.path.startsWith('/')) {
      throw new Error('WebDAV filesystem path must be an absolute path starting with "/"');
    }
    return this.sharePort.createWebdavShare(shareId.trim(), {
      readOnly: share.readOnly ?? false,
      enabled: share.enabled ?? true,
      allowedUsers: share.allowedUsers || [],
      allowedGroups: share.allowedGroups || [],
      ...share,
    });
  }

  async updateWebdavShare(
    shareIdOrName: string,
    updates: Partial<WebdavShare>,
    updateMask: string[]
  ): Promise<WebdavShare> {
    return this.sharePort.updateWebdavShare(
      ResourceNames.webdavShare(shareIdOrName),
      updates,
      updateMask
    );
  }

  async deleteWebdavShare(shareIdOrName: string): Promise<void> {
    return this.sharePort.deleteWebdavShare(ResourceNames.webdavShare(shareIdOrName));
  }
}

// ---------------------------------------------------------------------------
// 8. IdentityService
// ---------------------------------------------------------------------------

export class IdentityService {
  constructor(private readonly identityPort: IdentityPort) {}

  async listUsers(includeSystemAccounts = false): Promise<User[]> {
    return this.identityPort.listUsers(includeSystemAccounts);
  }

  async getUser(userIdOrName: string): Promise<User> {
    return this.identityPort.getUser(ResourceNames.user(userIdOrName));
  }

  async createUser(userId: string, user: Partial<User>): Promise<User> {
    if (!isValidResourceId(userId)) {
      throw new Error(`Invalid username: "${userId}"`);
    }
    if (user.sshAuthorizedKeys) {
      for (const key of user.sshAuthorizedKeys) {
        if (key.trim() && !validateSshPublicKey(key)) {
          throw new Error(`Invalid SSH public key format: "${key.slice(0, 32)}..."`);
        }
      }
    }
    return this.identityPort.createUser(userId.trim(), {
      shell: user.shell || '/bin/bash',
      homeDirectory: user.homeDirectory || `/home/${userId.trim()}`,
      sshEnabled: user.sshEnabled ?? true,
      sshPasswordAuth: user.sshPasswordAuth ?? true,
      sshAuthorizedKeys: user.sshAuthorizedKeys || [],
      supplementaryGroups: user.supplementaryGroups || [],
      locked: user.locked ?? false,
      isAdmin: user.isAdmin ?? false,
      ...user,
    });
  }

  async updateUser(
    userIdOrName: string,
    updates: Partial<User>,
    updateMask: string[]
  ): Promise<User> {
    if (updates.sshAuthorizedKeys) {
      for (const key of updates.sshAuthorizedKeys) {
        if (key.trim() && !validateSshPublicKey(key)) {
          throw new Error(`Invalid SSH public key format: "${key.slice(0, 32)}..."`);
        }
      }
    }
    return this.identityPort.updateUser(
      ResourceNames.user(userIdOrName),
      updates,
      updateMask
    );
  }

  async deleteUser(userIdOrName: string, removeHome = false): Promise<void> {
    return this.identityPort.deleteUser(ResourceNames.user(userIdOrName), removeHome);
  }

  async listGroups(includeSystemGroups = false): Promise<Group[]> {
    return this.identityPort.listGroups(includeSystemGroups);
  }

  async createGroup(groupId: string, group: Partial<Group>): Promise<Group> {
    if (!isValidResourceId(groupId)) {
      throw new Error(`Invalid group name: "${groupId}"`);
    }
    return this.identityPort.createGroup(groupId.trim(), {
      members: group.members || [],
      ...group,
    });
  }

  async updateGroup(
    groupIdOrName: string,
    updates: Partial<Group>,
    updateMask: string[]
  ): Promise<Group> {
    return this.identityPort.updateGroup(
      ResourceNames.group(groupIdOrName),
      updates,
      updateMask
    );
  }

  async deleteGroup(groupIdOrName: string): Promise<void> {
    return this.identityPort.deleteGroup(ResourceNames.group(groupIdOrName));
  }
}

export { CatalogService } from './catalogService';
