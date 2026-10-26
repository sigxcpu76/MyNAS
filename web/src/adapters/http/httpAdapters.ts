/**
 * Concrete HTTP Outbound Adapters implementing all Port Interfaces
 * against the Google AIP `/v1/...` REST/JSON endpoints.
 * Handles both camelCase (`protojson` default) and snake_case field names,
 * plus `int64` string/number normalization via `normalizeInt64`.
 */

import type {
  AuthSession,
  CatalogApp,
  CatalogQuestion,
  CatalogQuestionGroup,
  CifsShare,
  DirectoryList,
  Disk,
  DiskPartition,
  DiscoveredPool,
  DockerApp,
  DockerAppContainer,
  DockerConfig,
  DockerContainer,
  Group,
  HostPortStatus,
  LinuxFlavor,
  LxcContainer,
  Prerequisite,
  PrerequisiteSummary,
  SmartParameter,
  SystemConfig,
  User,
  VirtualMachine,
  VirtualMachineCloudInit,
  VirtualMachineConsoleSession,
  VirtualMachineDisk,
  VirtualMachineNetworkInterface,
  VirtualMachineSnapshot,
  WebdavShare,
  ZfsDataset,
  ZfsPool,
  ZfsVdev,
  ZfsVolume,
} from '../../domain/models';
import type {
  AuthPort,
  CatalogPort,
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
} from '../../ports/outbound';
import { AipClient, normalizeInt64 } from './aipClient';

// ---------------------------------------------------------------------------
// Mappers (protojson camelCase + snake_case compatibility + int64 coercion)
// ---------------------------------------------------------------------------

export function mapUser(raw: any): User {
  if (!raw) {
    return {
      name: '',
      username: '',
      uid: 0,
      primaryGid: 0,
      primaryGroup: '',
      supplementaryGroups: [],
      fullName: '',
      homeDirectory: '',
      shell: '/bin/bash',
      sshEnabled: false,
      sshPasswordAuth: false,
      sshAuthorizedKeys: [],
      locked: false,
      systemAccount: false,
      isAdmin: false,
    };
  }
  return {
    name: raw.name ?? '',
    username: raw.username ?? '',
    uid: Number(raw.uid ?? 0),
    primaryGid: Number(raw.primaryGid ?? raw.primary_gid ?? 0),
    primaryGroup: raw.primaryGroup ?? raw.primary_group ?? '',
    supplementaryGroups: Array.isArray(raw.supplementaryGroups ?? raw.supplementary_groups)
      ? (raw.supplementaryGroups ?? raw.supplementary_groups)
      : [],
    fullName: raw.fullName ?? raw.full_name ?? '',
    homeDirectory: raw.homeDirectory ?? raw.home_directory ?? '',
    shell: raw.shell ?? '/bin/bash',
    sshEnabled: Boolean(raw.sshEnabled ?? raw.ssh_enabled ?? false),
    sshPasswordAuth: Boolean(raw.sshPasswordAuth ?? raw.ssh_password_auth ?? false),
    sshAuthorizedKeys: Array.isArray(raw.sshAuthorizedKeys ?? raw.ssh_authorized_keys)
      ? (raw.sshAuthorizedKeys ?? raw.ssh_authorized_keys)
      : [],
    locked: Boolean(raw.locked ?? false),
    systemAccount: Boolean(raw.systemAccount ?? raw.system_account ?? false),
    isAdmin: Boolean(raw.isAdmin ?? raw.is_admin ?? false),
  };
}

export function mapGroup(raw: any): Group {
  return {
    name: raw?.name ?? '',
    groupName: raw?.groupName ?? raw?.group_name ?? '',
    gid: Number(raw?.gid ?? 0),
    members: Array.isArray(raw?.members) ? raw.members : [],
    systemGroup: Boolean(raw?.systemGroup ?? raw?.system_group ?? false),
  };
}

export function mapPrerequisite(raw: any): Prerequisite {
  return {
    name: raw?.name ?? '',
    displayName: raw?.displayName ?? raw?.display_name ?? '',
    category: raw?.category ?? 'STORAGE',
    kind: raw?.kind ?? 'BINARY',
    binaryOrPath: raw?.binaryOrPath ?? raw?.binary_or_path ?? '',
    status: raw?.status ?? 'MISSING',
    version: raw?.version ?? '',
    details: raw?.details ?? '',
    installHint: raw?.installHint ?? raw?.install_hint ?? '',
    requiredBy: Array.isArray(raw?.requiredBy ?? raw?.required_by)
      ? (raw.requiredBy ?? raw.required_by)
      : [],
    fallbackMode: Boolean(raw?.fallbackMode ?? raw?.fallback_mode ?? false),
    checkedTime: raw?.checkedTime ?? raw?.checked_time ?? undefined,
  };
}

export function mapLinuxFlavor(raw: any): LinuxFlavor | undefined {
  if (!raw) return undefined;
  return {
    id: raw.id ?? '',
    name: raw.name ?? '',
    prettyName: raw.prettyName ?? raw.pretty_name ?? raw.name ?? '',
    versionId: raw.versionId ?? raw.version_id ?? '',
    packageManager: raw.packageManager ?? raw.package_manager ?? '',
    installCommandTemplate: raw.installCommandTemplate ?? raw.install_command_template ?? '',
  };
}

export function mapDiscoveredPool(raw: any): DiscoveredPool {
  return {
    poolName: raw?.poolName ?? raw?.pool_name ?? '',
    hasNasSystem: Boolean(raw?.hasNasSystem ?? raw?.has_nas_system ?? false),
    configVersion: raw?.configVersion ?? raw?.config_version ?? '1.0',
    lastModified: raw?.lastModified ?? raw?.last_modified ?? '',
    isImported: Boolean(raw?.isImported ?? raw?.is_imported ?? true),
    datasetPath: raw?.datasetPath ?? raw?.dataset_path ?? '',
    mountpoint: raw?.mountpoint ?? '',
  };
}

export function mapSystemConfig(raw: any): SystemConfig {
  const pools = (raw?.discoveredPools ?? raw?.discovered_pools ?? []).map(mapDiscoveredPool);
  return {
    mainPool: raw?.mainPool ?? raw?.main_pool ?? '',
    systemDataset: raw?.systemDataset ?? raw?.system_dataset ?? '',
    systemMountpoint: raw?.systemMountpoint ?? raw?.system_mountpoint ?? '',
    isInitialized: Boolean(raw?.isInitialized ?? raw?.is_initialized ?? false),
    enabledModules: Array.isArray(raw?.enabledModules ?? raw?.enabled_modules)
      ? (raw.enabledModules ?? raw.enabled_modules)
      : ['zfs'],
    configFilePath: raw?.configFilePath ?? raw?.config_file_path ?? '',
    autoDiscover: Boolean(raw?.autoDiscover ?? raw?.auto_discover ?? true),
    discoveredPools: pools,
  };
}

export function mapDiskPartition(raw: any): DiskPartition {
  if (!raw) return { name: '', devicePath: '', sizeBytes: 0, fstype: '', mountpoint: '', label: '', owningZfsPool: '', aliases: [] };
  return {
    name: raw.name ?? '',
    devicePath: raw.devicePath ?? raw.device_path ?? '',
    sizeBytes: normalizeInt64(raw.sizeBytes ?? raw.size_bytes),
    fstype: raw.fstype ?? '',
    mountpoint: raw.mountpoint ?? '',
    label: raw.label ?? '',
    owningZfsPool: raw.owningZfsPool ?? raw.owning_zfs_pool ?? '',
    aliases: Array.isArray(raw.aliases) ? raw.aliases : [],
    isSystem: Boolean(raw.isSystem ?? raw.is_system),
  };
}

export function mapSmartParameter(raw: any): SmartParameter {
  return {
    id: Number(raw?.id ?? 0),
    name: raw?.name ?? '',
    value: String(raw?.value ?? ''),
    worst: String(raw?.worst ?? ''),
    threshold: String(raw?.threshold ?? ''),
    rawValue: String(raw?.rawValue ?? raw?.raw_value ?? ''),
    status: raw?.status ?? 'OK',
    type: raw?.type ?? '',
    description: raw?.description ?? '',
  };
}

export function mapDisk(raw: any): Disk {
  if (!raw) {
    return {
      name: '',
      diskId: '',
      devicePath: '',
      sizeBytes: 0,
      model: '',
      serial: '',
      vendor: '',
      transport: '',
      rotational: false,
      type: 'SSD',
      firmwareRev: '',
      state: '',
      removable: false,
      logicalBlockSize: 512,
      physicalBlockSize: 512,
      temperatureCelsius: 0,
      smartStatus: 'UNKNOWN',
      healthPercentage: 100,
      owningZfsPool: '',
      partitions: [],
      aliases: [],
      smartParameters: [],
      powerOnHours: 0,
      powerCycleCount: 0,
      unsafeShutdowns: 0,
      mediaErrors: 0,
      totalBytesRead: 0,
      totalBytesWritten: 0,
      reallocatedSectors: 0,
      pendingSectors: 0,
    };
  }
  const parts = Array.isArray(raw.partitions) ? raw.partitions.map(mapDiskPartition) : [];
  const smartParams = Array.isArray(raw.smartParameters ?? raw.smart_parameters)
    ? (raw.smartParameters ?? raw.smart_parameters).map(mapSmartParameter)
    : [];
  return {
    name: raw.name ?? '',
    diskId: raw.diskId ?? raw.disk_id ?? '',
    devicePath: raw.devicePath ?? raw.device_path ?? '',
    sizeBytes: normalizeInt64(raw.sizeBytes ?? raw.size_bytes),
    model: raw.model ?? '',
    serial: raw.serial ?? '',
    vendor: raw.vendor ?? '',
    transport: raw.transport ?? '',
    rotational: Boolean(raw.rotational),
    type: raw.type ?? 'SSD',
    firmwareRev: raw.firmwareRev ?? raw.firmware_rev ?? '',
    state: raw.state ?? '',
    removable: Boolean(raw.removable),
    logicalBlockSize: normalizeInt64(raw.logicalBlockSize ?? raw.logical_block_size) || 512,
    physicalBlockSize: normalizeInt64(raw.physicalBlockSize ?? raw.physical_block_size) || 512,
    temperatureCelsius: Number(raw.temperatureCelsius ?? raw.temperature_celsius ?? 0),
    smartStatus: raw.smartStatus ?? raw.smart_status ?? 'UNKNOWN',
    healthPercentage: Number(raw.healthPercentage ?? raw.health_percentage ?? 100),
    owningZfsPool: raw.owningZfsPool ?? raw.owning_zfs_pool ?? '',
    partitions: parts,
    aliases: Array.isArray(raw.aliases) ? raw.aliases : [],
    smartParameters: smartParams,
    powerOnHours: normalizeInt64(raw.powerOnHours ?? raw.power_on_hours),
    powerCycleCount: normalizeInt64(raw.powerCycleCount ?? raw.power_cycle_count),
    unsafeShutdowns: normalizeInt64(raw.unsafeShutdowns ?? raw.unsafe_shutdowns),
    mediaErrors: normalizeInt64(raw.mediaErrors ?? raw.media_errors),
    totalBytesRead: normalizeInt64(raw.totalBytesRead ?? raw.total_bytes_read),
    totalBytesWritten: normalizeInt64(raw.totalBytesWritten ?? raw.total_bytes_written),
    reallocatedSectors: normalizeInt64(raw.reallocatedSectors ?? raw.reallocated_sectors),
    pendingSectors: normalizeInt64(raw.pendingSectors ?? raw.pending_sectors),
    isSystem: Boolean(raw.isSystem ?? raw.is_system),
  };
}

export function mapZfsVdev(raw: any): ZfsVdev {
  return {
    name: raw?.name ?? '',
    type: raw?.type ?? 'data',
    layout: raw?.layout ?? '',
    state: raw?.state ?? 'ONLINE',
    path: raw?.path ?? '',
    readErrors: raw?.readErrors ?? raw?.read_errors ?? '0',
    writeErrors: raw?.writeErrors ?? raw?.write_errors ?? '0',
    checksumErrors: raw?.checksumErrors ?? raw?.checksum_errors ?? '0',
    children: Array.isArray(raw?.children) ? raw.children.map(mapZfsVdev) : [],
  };
}

export function mapZfsPool(raw: any): ZfsPool {
  return {
    name: raw?.name ?? '',
    poolName: raw?.poolName ?? raw?.pool_name ?? '',
    state: raw?.state ?? 'ONLINE',
    sizeBytes: normalizeInt64(raw?.sizeBytes ?? raw?.size_bytes),
    allocatedBytes: normalizeInt64(raw?.allocatedBytes ?? raw?.allocated_bytes),
    freeBytes: normalizeInt64(raw?.freeBytes ?? raw?.free_bytes),
    fragmentationPercent: Number(raw?.fragmentationPercent ?? raw?.fragmentation_percent ?? 0),
    capacityPercent: Number(raw?.capacityPercent ?? raw?.capacity_percent ?? 0),
    dedupRatio: Number(raw?.dedupRatio ?? raw?.dedup_ratio ?? 1.0),
    vdevLayout: raw?.vdevLayout ?? raw?.vdev_layout ?? 'mirror',
    devices: Array.isArray(raw?.devices) ? raw.devices : [],
    mountpoint: raw?.mountpoint ?? '',
    compression: raw?.compression ?? 'lz4',
    lastScrubStatus: raw?.lastScrubStatus ?? raw?.last_scrub_status ?? '',
    healthDetails: raw?.healthDetails ?? raw?.health_details ?? '',
    sync: raw?.sync ?? 'standard',
    autotrim: raw?.autotrim ?? 'off',
    vdevs: Array.isArray(raw?.vdevs) ? raw.vdevs.map(mapZfsVdev) : [],
  };
}

export function mapZfsDataset(raw: any): ZfsDataset {
  return {
    name: raw?.name ?? '',
    datasetPath: raw?.datasetPath ?? raw?.dataset_path ?? '',
    mountpoint: raw?.mountpoint ?? '',
    usedBytes: normalizeInt64(raw?.usedBytes ?? raw?.used_bytes),
    availableBytes: normalizeInt64(raw?.availableBytes ?? raw?.available_bytes),
    referencedBytes: normalizeInt64(raw?.referencedBytes ?? raw?.referenced_bytes),
    quotaBytes: normalizeInt64(raw?.quotaBytes ?? raw?.quota_bytes),
    compression: raw?.compression ?? 'lz4',
    readonly: Boolean(raw?.readonly ?? false),
    atime: Boolean(raw?.atime ?? false),
    recordsize: raw?.recordsize ?? '128K',
    snapshotsCount: Number(raw?.snapshotsCount ?? raw?.snapshots_count ?? 0),
    lastSnapshot: raw?.lastSnapshot ?? raw?.last_snapshot ?? '',
    sync: raw?.sync ?? 'standard',
    reservationBytes: normalizeInt64(raw?.reservationBytes ?? raw?.reservation_bytes),
    dedup: Boolean(raw?.dedup ?? false),
  };
}

export function mapZfsVolume(raw: any): ZfsVolume {
  return {
    name: raw?.name ?? '',
    volumePath: raw?.volumePath ?? raw?.volume_path ?? '',
    devicePath: raw?.devicePath ?? raw?.device_path ?? '',
    sizeBytes: normalizeInt64(raw?.sizeBytes ?? raw?.size_bytes),
    usedBytes: normalizeInt64(raw?.usedBytes ?? raw?.used_bytes),
    volblocksize: raw?.volblocksize ?? '16K',
    compression: raw?.compression ?? 'lz4',
    thinProvisioned: Boolean(raw?.thinProvisioned ?? raw?.thin_provisioned ?? false),
    sync: raw?.sync ?? 'standard',
    readonly: Boolean(raw?.readonly ?? false),
    dedup: Boolean(raw?.dedup ?? false),
  };
}

export function mapVirtualMachineDisk(raw: any): VirtualMachineDisk {
  return {
    id: raw?.id ?? '',
    bus: raw?.bus ?? 'scsi',
    diskPath: raw?.diskPath ?? raw?.disk_path ?? '',
    sizeBytes: normalizeInt64(raw?.sizeBytes ?? raw?.size_bytes),
    cacheMode: raw?.cacheMode ?? raw?.cache_mode ?? 'none',
    discard: Boolean(raw?.discard ?? true),
    iothread: Boolean(raw?.iothread ?? true),
    readOnly: Boolean(raw?.readOnly ?? raw?.read_only ?? false),
    ssdEmulation: Boolean(raw?.ssdEmulation ?? raw?.ssd_emulation ?? false),
    backup: Boolean(raw?.backup ?? true),
    iopsLimitRead: normalizeInt64(raw?.iopsLimitRead ?? raw?.iops_limit_read),
    iopsLimitWrite: normalizeInt64(raw?.iopsLimitWrite ?? raw?.iops_limit_write),
    mbpsLimitRead: normalizeInt64(raw?.mbpsLimitRead ?? raw?.mbps_limit_read),
    mbpsLimitWrite: normalizeInt64(raw?.mbpsLimitWrite ?? raw?.mbps_limit_write),
  };
}

export function mapVirtualMachineNetworkInterface(raw: any): VirtualMachineNetworkInterface {
  return {
    id: raw?.id ?? '',
    model: raw?.model ?? 'virtio',
    bridge: raw?.bridge ?? 'virbr0',
    vlanTag: Number(raw?.vlanTag ?? raw?.vlan_tag ?? 0),
    macAddress: raw?.macAddress ?? raw?.mac_address ?? '',
    rateLimitMbps: normalizeInt64(raw?.rateLimitMbps ?? raw?.rate_limit_mbps),
    disconnected: Boolean(raw?.disconnected ?? false),
    firewall: Boolean(raw?.firewall ?? true),
  };
}

export function mapVirtualMachineCloudInit(raw: any): VirtualMachineCloudInit {
  if (!raw) return {};
  return {
    user: raw?.user ?? '',
    password: raw?.password ?? '',
    sshPublicKeys: Array.isArray(raw?.sshPublicKeys ?? raw?.ssh_public_keys)
      ? (raw?.sshPublicKeys ?? raw?.ssh_public_keys)
      : [],
    ipConfig: raw?.ipConfig ?? raw?.ip_config ?? '',
    dnsNameservers: raw?.dnsNameservers ?? raw?.dns_nameservers ?? '',
    dnsSearch: raw?.dnsSearch ?? raw?.dns_search ?? '',
    customUserData: raw?.customUserData ?? raw?.custom_user_data ?? '',
  };
}

export function mapVirtualMachineSnapshot(raw: any): VirtualMachineSnapshot {
  return {
    name: raw?.name ?? '',
    snapshotId: raw?.snapshotId ?? raw?.snapshot_id ?? '',
    description: raw?.description ?? '',
    createTime: raw?.createTime ?? raw?.create_time ?? '',
    includeRam: Boolean(raw?.includeRam ?? raw?.include_ram ?? false),
    parentSnapshot: raw?.parentSnapshot ?? raw?.parent_snapshot ?? '',
    vmState: raw?.vmState ?? raw?.vm_state ?? 'STOPPED',
  };
}

export function mapVirtualMachine(raw: any): VirtualMachine {
  const disks = Array.isArray(raw?.disks) ? raw.disks.map(mapVirtualMachineDisk) : [];
  const ifaces = Array.isArray(raw?.networkInterfaces ?? raw?.network_interfaces)
    ? (raw.networkInterfaces ?? raw.network_interfaces).map(mapVirtualMachineNetworkInterface)
    : [];
  return {
    name: raw?.name ?? '',
    vmName: raw?.vmName ?? raw?.vm_name ?? '',
    uuid: raw?.uuid ?? '',
    state: raw?.state ?? 'STOPPED',
    vcpus: Number(raw?.vcpus ?? 2),
    memoryMib: normalizeInt64(raw?.memoryMib ?? raw?.memory_mib),
    diskPath: raw?.diskPath ?? raw?.disk_path ?? '',
    diskSizeGib: normalizeInt64(raw?.diskSizeGib ?? raw?.disk_size_gib),
    isoImage: raw?.isoImage ?? raw?.iso_image ?? '',
    cdromIsoImage: raw?.cdromIsoImage ?? raw?.cdrom_iso_image ?? '',
    networkBridge: raw?.networkBridge ?? raw?.network_bridge ?? 'virbr0',
    vncPort: Number(raw?.vncPort ?? raw?.vnc_port ?? 0),
    vncDisplay: raw?.vncDisplay ?? raw?.vnc_display ?? '',
    autostart: Boolean(raw?.autostart ?? false),
    osVariant: raw?.osVariant ?? raw?.os_variant ?? '',
    description: raw?.description ?? '',

    sockets: Number(raw?.sockets ?? 1),
    cores: Number(raw?.cores ?? (raw?.vcpus ?? 2)),
    cpuType: raw?.cpuType ?? raw?.cpu_type ?? 'host',
    cpuUnits: Number(raw?.cpuUnits ?? raw?.cpu_units ?? 1024),
    cpuLimit: Number(raw?.cpuLimit ?? raw?.cpu_limit ?? 0),
    numa: Boolean(raw?.numa ?? false),
    balloonMinMib: normalizeInt64(raw?.balloonMinMib ?? raw?.balloon_min_mib),
    hugepages: raw?.hugepages ?? 'off',
    machineType: raw?.machineType ?? raw?.machine_type ?? 'q35',
    biosType: raw?.biosType ?? raw?.bios_type ?? 'seabios',
    tpmVersion: raw?.tpmVersion ?? raw?.tpm_version ?? 'none',
    secureBoot: Boolean(raw?.secureBoot ?? raw?.secure_boot ?? false),

    disks,
    networkInterfaces: ifaces,
    cloudInit: mapVirtualMachineCloudInit(raw?.cloudInit ?? raw?.cloud_init),

    agentEnabled: Boolean(raw?.agentEnabled ?? raw?.agent_enabled ?? false),
    agentRunning: Boolean(raw?.agentRunning ?? raw?.agent_running ?? false),
    guestIpAddresses: Array.isArray(raw?.guestIpAddresses ?? raw?.guest_ip_addresses)
      ? (raw.guestIpAddresses ?? raw.guest_ip_addresses)
      : [],
    guestOsName: raw?.guestOsName ?? raw?.guest_os_name ?? '',
    guestCpuPercent: Number(raw?.guestCpuPercent ?? raw?.guest_cpu_percent ?? 0),
    guestMemoryUsedMib: normalizeInt64(raw?.guestMemoryUsedMib ?? raw?.guest_memory_used_mib),

    startupDelaySeconds: Number(raw?.startupDelaySeconds ?? raw?.startup_delay_seconds ?? 0),
    bootOrder: raw?.bootOrder ?? raw?.boot_order ?? '',
    watchdogModel: raw?.watchdogModel ?? raw?.watchdog_model ?? 'none',
    watchdogAction: raw?.watchdogAction ?? raw?.watchdog_action ?? 'reset',
    displayType: raw?.displayType ?? raw?.display_type ?? 'virtio-gpu',
    isTemplate: Boolean(raw?.isTemplate ?? raw?.is_template ?? false),
    qemuCommandlineArgs: Array.isArray(raw?.qemuCommandlineArgs ?? raw?.qemu_commandline_args)
      ? (raw.qemuCommandlineArgs ?? raw.qemu_commandline_args)
      : [],
    serialConsoleUrl: raw?.serialConsoleUrl ?? raw?.serial_console_url ?? '',
  };
}

export function mapDockerContainer(raw: any): DockerContainer {
  return {
    name: raw?.name ?? '',
    containerId: raw?.containerId ?? raw?.container_id ?? '',
    containerName: raw?.containerName ?? raw?.container_name ?? '',
    image: raw?.image ?? '',
    state: raw?.state ?? 'STOPPED',
    statusText: raw?.statusText ?? raw?.status_text ?? '',
    ports: Array.isArray(raw?.ports) ? raw.ports : [],
    volumes: Array.isArray(raw?.volumes) ? raw.volumes : [],
    environment: Array.isArray(raw?.environment) ? raw.environment : [],
    restartPolicy: raw?.restartPolicy ?? raw?.restart_policy ?? 'unless-stopped',
    networkMode: raw?.networkMode ?? raw?.network_mode ?? 'bridge',
    cpuPercent: Number(raw?.cpuPercent ?? raw?.cpu_percent ?? 0),
    memoryUsageMib: normalizeInt64(raw?.memoryUsageMib ?? raw?.memory_usage_mib),
    createdAt: raw?.createdAt ?? raw?.created_at ?? '',
    memoryLimitMib: normalizeInt64(raw?.memoryLimitMib ?? raw?.memory_limit_mib),
    command: Array.isArray(raw?.command) ? raw.command : [],
    privileged: Boolean(raw?.privileged ?? false),
    composeYaml: raw?.composeYaml ?? raw?.compose_yaml ?? '',
  };
}

export function mapDockerApp(raw: any): DockerApp {
  const containers: DockerAppContainer[] = (raw?.containers ?? []).map((c: any) => ({
    containerId: c.containerId ?? c.container_id ?? '',
    name: c.name ?? '',
    serviceName: c.serviceName ?? c.service_name ?? '',
    image: c.image ?? '',
    state: c.state ?? '',
    status: c.status ?? '',
    cpuPercent: Number(c.cpuPercent ?? c.cpu_percent ?? 0),
    memoryMib: normalizeInt64(c.memoryMib ?? c.memory_mib ?? 0),
    ports: c.ports ?? [],
    createdAt: c.createdAt ?? c.created_at ?? '',
    netRxBytes: normalizeInt64(c.netRxBytes ?? c.net_rx_bytes ?? 0),
    netTxBytes: normalizeInt64(c.netTxBytes ?? c.net_tx_bytes ?? 0),
    netRxRate: normalizeInt64(c.netRxRate ?? c.net_rx_rate ?? 0),
    netTxRate: normalizeInt64(c.netTxRate ?? c.net_tx_rate ?? 0),
  }));

  return {
    name: raw?.name ?? '',
    appId: raw?.appId ?? raw?.app_id ?? '',
    title: raw?.title ?? '',
    state: raw?.state ?? 'STOPPED',
    statusText: raw?.statusText ?? raw?.status_text ?? '',
    composeYaml: raw?.composeYaml ?? raw?.compose_yaml ?? '',
    composeFilePath: raw?.composeFilePath ?? raw?.compose_file_path ?? '',
    services: raw?.services ?? [],
    containers,
    portMappings: raw?.portMappings ?? raw?.port_mappings ?? [],
    volumes: raw?.volumes ?? [],
    webUiUrl: raw?.webUiUrl ?? raw?.web_ui_url ?? '',
    portals: (raw?.portals || []).map((p: any) => ({
      name: p?.name || 'Web UI',
      url: p?.url || '',
      port: Number(p?.port || 0),
      protocol: p?.protocol || 'http',
      path: p?.path || '/',
    })),
    totalCpuPercent: Number(raw?.totalCpuPercent ?? raw?.total_cpu_percent ?? 0),
    totalMemoryMib: normalizeInt64(raw?.totalMemoryMib ?? raw?.total_memory_mib ?? 0),
    totalNetRxBytes: normalizeInt64(raw?.totalNetRxBytes ?? raw?.total_net_rx_bytes ?? 0),
    totalNetTxBytes: normalizeInt64(raw?.totalNetTxBytes ?? raw?.total_net_tx_bytes ?? 0),
    totalNetRxRate: normalizeInt64(raw?.totalNetRxRate ?? raw?.total_net_rx_rate ?? 0),
    totalNetTxRate: normalizeInt64(raw?.totalNetTxRate ?? raw?.total_net_tx_rate ?? 0),
    createdAt: raw?.createdAt ?? raw?.created_at ?? '',
    updatedAt: raw?.updatedAt ?? raw?.updated_at ?? '',
  };
}

export function mapLxcContainer(raw: any): LxcContainer {
  return {
    name: raw?.name ?? '',
    containerName: raw?.containerName ?? raw?.container_name ?? '',
    state: raw?.state ?? 'STOPPED',
    distribution: raw?.distribution ?? 'debian',
    release: raw?.release ?? 'bookworm',
    architecture: raw?.architecture ?? 'amd64',
    ipv4Address: raw?.ipv4Address ?? raw?.ipv4_address ?? '',
    autostart: Boolean(raw?.autostart ?? false),
    unprivileged: Boolean(raw?.unprivileged ?? true),
    rootfsPath: raw?.rootfsPath ?? raw?.rootfs_path ?? '',
    memoryLimitMib: normalizeInt64(raw?.memoryLimitMib ?? raw?.memory_limit_mib),
    cpuCores: Number(raw?.cpuCores ?? raw?.cpu_cores ?? 1),
    templateArchive: raw?.templateArchive ?? raw?.template_archive ?? undefined,
  };
}

export function mapCifsShare(raw: any): CifsShare {
  return {
    name: raw?.name ?? '',
    shareName: raw?.shareName ?? raw?.share_name ?? '',
    path: raw?.path ?? '',
    comment: raw?.comment ?? '',
    readOnly: Boolean(raw?.readOnly ?? raw?.read_only ?? false),
    guestOk: Boolean(raw?.guestOk ?? raw?.guest_ok ?? false),
    browseable: Boolean(raw?.browseable ?? true),
    validUsers: Array.isArray(raw?.validUsers ?? raw?.valid_users)
      ? (raw.validUsers ?? raw.valid_users)
      : [],
    validGroups: Array.isArray(raw?.validGroups ?? raw?.valid_groups)
      ? (raw.validGroups ?? raw.valid_groups)
      : [],
    createMask: raw?.createMask ?? raw?.create_mask ?? '0664',
    directoryMask: raw?.directoryMask ?? raw?.directory_mask ?? '0775',
    enabled: Boolean(raw?.enabled ?? true),
  };
}

export function mapWebdavShare(raw: any): WebdavShare {
  return {
    name: raw?.name ?? '',
    shareName: raw?.shareName ?? raw?.share_name ?? '',
    path: raw?.path ?? '',
    urlPrefix: raw?.urlPrefix ?? raw?.url_prefix ?? '',
    description: raw?.description ?? '',
    readOnly: Boolean(raw?.readOnly ?? raw?.read_only ?? false),
    allowedUsers: Array.isArray(raw?.allowedUsers ?? raw?.allowed_users)
      ? (raw.allowedUsers ?? raw.allowed_users)
      : [],
    allowedGroups: Array.isArray(raw?.allowedGroups ?? raw?.allowed_groups)
      ? (raw.allowedGroups ?? raw.allowed_groups)
      : [],
    enabled: Boolean(raw?.enabled ?? true),
  };
}

// ---------------------------------------------------------------------------
// 1. HttpAuthAdapter
// ---------------------------------------------------------------------------

export class HttpAuthAdapter implements AuthPort {
  constructor(private readonly client: AipClient) {}

  async login(username: string, password: string): Promise<AuthSession> {
    const raw = await this.client.customMethod<any>('auth', 'login', {
      username,
      password,
    });
    return {
      accessToken: raw.accessToken ?? raw.access_token ?? '',
      tokenType: raw.tokenType ?? raw.token_type ?? 'Bearer',
      expiresIn: normalizeInt64(raw.expiresIn ?? raw.expires_in ?? 86400),
      user: mapUser(raw.user),
    };
  }

  async getCurrentUser(): Promise<User> {
    const raw = await this.client.get<any>('auth:me');
    return mapUser(raw);
  }

  logout(): void {
    this.client.setToken(null);
  }

  getToken(): string | null {
    return this.client.getToken();
  }

  setToken(token: string | null): void {
    this.client.setToken(token);
  }
}

// ---------------------------------------------------------------------------
// 1b. HttpSystemAdapter
// ---------------------------------------------------------------------------

export class HttpSystemAdapter implements SystemPort {
  constructor(private readonly client: AipClient) {}

  async getSystemConfig(): Promise<SystemConfig> {
    const raw = await this.client.get<any>('systemConfig');
    return mapSystemConfig(raw);
  }

  async updateSystemConfig(config: Partial<SystemConfig>): Promise<SystemConfig> {
    const raw = await this.client.patch<any>('systemConfig', { system_config: config });
    return mapSystemConfig(raw);
  }

  async selectMainPool(poolName: string, autoImportExisting = true): Promise<SystemConfig> {
    const raw = await this.client.customMethod<any>('systemConfig', 'selectMainPool', {
      poolName: poolName,
      autoImportExisting: autoImportExisting,
    });
    return mapSystemConfig(raw);
  }

  async discoverPools(): Promise<DiscoveredPool[]> {
    const raw = await this.client.get<any>('systemConfig:discoverPools');
    return (raw?.pools ?? []).map(mapDiscoveredPool);
  }
}

// ---------------------------------------------------------------------------
// 2. HttpPrerequisiteAdapter
// ---------------------------------------------------------------------------

export class HttpPrerequisiteAdapter implements PrerequisitePort {
  constructor(private readonly client: AipClient) {}

  async listPrerequisites(): Promise<Prerequisite[]> {
    const res = await this.client.get<any>('prerequisites');
    const items = res.prerequisites ?? [];
    return items.map(mapPrerequisite);
  }

  async getPrerequisite(name: string): Promise<Prerequisite> {
    const raw = await this.client.get<any>(name);
    return mapPrerequisite(raw);
  }

  async checkPrerequisites(): Promise<PrerequisiteSummary> {
    const raw = await this.client.customMethod<any>('prerequisites', 'check', {});
    const items = (raw.prerequisites ?? []).map(mapPrerequisite);
    const missingItems = (raw.missingPrerequisites ?? raw.missing_prerequisites ?? []).map(mapPrerequisite);
    const installedItems = (raw.installedPrerequisites ?? raw.installed_prerequisites ?? []).map(mapPrerequisite);
    return {
      prerequisites: items,
      installedCount: Number(raw.installedCount ?? raw.installed_count ?? installedItems.length),
      missingCount: Number(raw.missingCount ?? raw.missing_count ?? missingItems.length),
      linuxFlavor: mapLinuxFlavor(raw.linuxFlavor ?? raw.linux_flavor),
      missingPrerequisites: missingItems.length > 0 ? missingItems : items.filter((p: Prerequisite) => p.status !== 'INSTALLED'),
      installedPrerequisites: installedItems.length > 0 ? installedItems : items.filter((p: Prerequisite) => p.status === 'INSTALLED'),
      remediationScript: raw.remediationScript ?? raw.remediation_script ?? '',
      isFullyCompliant: Boolean(raw.isFullyCompliant ?? raw.is_fully_compliant ?? (missingItems.length === 0 && items.length > 0)),
    };
  }
}

// ---------------------------------------------------------------------------
// 2b. HttpDiskAdapter
// ---------------------------------------------------------------------------

export class HttpDiskAdapter implements DiskPort {
  constructor(private readonly client: AipClient) {}

  async listDisks(filter?: string): Promise<Disk[]> {
    const params: Record<string, string> = { pageSize: '100' };
    if (filter) params.filter = filter;
    const res = await this.client.get<{ disks?: any[]; nextPageToken?: string }>('disks', params);
    return (res.disks ?? []).map(mapDisk);
  }

  async getDisk(name: string): Promise<Disk> {
    const res = await this.client.get<any>(name);
    return mapDisk(res);
  }
}

// ---------------------------------------------------------------------------
// 3. HttpZfsAdapter
// ---------------------------------------------------------------------------

export class HttpZfsAdapter implements ZfsPort {
  constructor(private readonly client: AipClient) {}

  async listPools(): Promise<ZfsPool[]> {
    const res = await this.client.get<any>('zfsPools');
    const items = res.zfsPools ?? res.zfs_pools ?? [];
    return items.map(mapZfsPool);
  }

  async getPool(name: string): Promise<ZfsPool> {
    const raw = await this.client.get<any>(name);
    return mapZfsPool(raw);
  }

  async createPool(poolId: string, pool: Partial<ZfsPool>): Promise<ZfsPool> {
    const payload = {
      vdevLayout: pool.vdevLayout,
      devices: pool.devices ?? [],
      mountpoint: pool.mountpoint,
      compression: pool.compression,
      vdevs: pool.vdevs ?? [],
    };
    const raw = await this.client.post<any>('zfsPools', payload, {
      zfsPoolId: poolId,
    });
    return mapZfsPool(raw);
  }

  async deletePool(name: string, force = false): Promise<void> {
    await this.client.delete(name, { force });
  }

  async exportPool(name: string, force = false): Promise<void> {
    await this.client.customMethod<any>(name, 'export', { force });
  }

  async scrubPool(name: string): Promise<ZfsPool> {
    const raw = await this.client.customMethod<any>(name, 'scrub', {});
    return mapZfsPool(raw);
  }

  async updatePool(name: string, update: Partial<ZfsPool>): Promise<ZfsPool> {
    const payload: any = {
      name,
    };
    if (update.sync !== undefined) payload.sync = update.sync;
    if (update.autotrim !== undefined) payload.autotrim = update.autotrim;
    if (update.compression !== undefined) payload.compression = update.compression;

    const raw = await this.client.patch<any>(name, payload);
    return mapZfsPool(raw);
  }

  async addVdev(
    name: string,
    req: { type: string; layout?: string; devices: string[] }
  ): Promise<ZfsPool> {
    const raw = await this.client.customMethod<any>(name, 'addVdev', req);
    return mapZfsPool(raw);
  }

  async removeVdev(name: string, device: string): Promise<ZfsPool> {
    const raw = await this.client.customMethod<any>(name, 'removeVdev', { device });
    return mapZfsPool(raw);
  }

  // Datasets
  async listDatasets(parentPoolName: string): Promise<ZfsDataset[]> {
    const res = await this.client.get<any>(`${parentPoolName}/datasets`);
    const items = res.zfsDatasets ?? res.zfs_datasets ?? [];
    return items.map(mapZfsDataset);
  }

  async getDataset(name: string): Promise<ZfsDataset> {
    const raw = await this.client.get<any>(name);
    return mapZfsDataset(raw);
  }

  async createDataset(
    parentPoolName: string,
    datasetId: string,
    dataset: Partial<ZfsDataset>
  ): Promise<ZfsDataset> {
    const payload = {
      mountpoint: dataset.mountpoint,
      quotaBytes: dataset.quotaBytes ?? 0,
      compression: dataset.compression,
      readonly: dataset.readonly,
      atime: dataset.atime,
      recordsize: dataset.recordsize,
      sync: dataset.sync,
      reservationBytes: dataset.reservationBytes,
      dedup: dataset.dedup,
    };
    const raw = await this.client.post<any>(`${parentPoolName}/datasets`, payload, {
      zfsDatasetId: datasetId,
    });
    return mapZfsDataset(raw);
  }

  async updateDataset(
    name: string,
    dataset: Partial<ZfsDataset>,
    updateMask: string[]
  ): Promise<ZfsDataset> {
    const payload = {
      name,
      mountpoint: dataset.mountpoint,
      quotaBytes: dataset.quotaBytes,
      compression: dataset.compression,
      readonly: dataset.readonly,
      atime: dataset.atime,
      recordsize: dataset.recordsize,
      sync: dataset.sync,
      reservationBytes: dataset.reservationBytes,
      dedup: dataset.dedup,
    };
    const raw = await this.client.patch<any>(name, payload, updateMask);
    return mapZfsDataset(raw);
  }

  async deleteDataset(name: string, recursive = false): Promise<void> {
    await this.client.delete(name, { recursive });
  }

  async snapshotDataset(name: string, snapshotName: string): Promise<ZfsDataset> {
    const raw = await this.client.customMethod<any>(name, 'snapshot', {
      snapshotName,
    });
    return mapZfsDataset(raw);
  }

  // Volumes (ZVols)
  async listVolumes(parentPoolName: string): Promise<ZfsVolume[]> {
    const res = await this.client.get<any>(`${parentPoolName}/volumes`);
    const items = res.zfsVolumes ?? res.zfs_volumes ?? [];
    return items.map(mapZfsVolume);
  }

  async getVolume(name: string): Promise<ZfsVolume> {
    const raw = await this.client.get<any>(name);
    return mapZfsVolume(raw);
  }

  async createVolume(
    parentPoolName: string,
    volumeId: string,
    volume: Partial<ZfsVolume>
  ): Promise<ZfsVolume> {
    const payload = {
      sizeBytes: volume.sizeBytes,
      volblocksize: volume.volblocksize,
      compression: volume.compression,
      thinProvisioned: volume.thinProvisioned,
      sync: volume.sync,
      readonly: volume.readonly,
      dedup: volume.dedup,
    };
    const raw = await this.client.post<any>(`${parentPoolName}/volumes`, payload, {
      zfsVolumeId: volumeId,
    });
    return mapZfsVolume(raw);
  }

  async updateVolume(
    name: string,
    volume: Partial<ZfsVolume>,
    updateMask: string[]
  ): Promise<ZfsVolume> {
    const payload = {
      name,
      sizeBytes: volume.sizeBytes,
      compression: volume.compression,
      thinProvisioned: volume.thinProvisioned,
      sync: volume.sync,
      readonly: volume.readonly,
      dedup: volume.dedup,
    };
    const raw = await this.client.patch<any>(name, payload, updateMask);
    return mapZfsVolume(raw);
  }

  async deleteVolume(name: string): Promise<void> {
    await this.client.delete(name);
  }
}

// ---------------------------------------------------------------------------
// 4. HttpKvmAdapter
// ---------------------------------------------------------------------------

export class HttpKvmAdapter implements KvmPort {
  constructor(private readonly client: AipClient) {}

  async listVirtualMachines(): Promise<VirtualMachine[]> {
    const res = await this.client.get<any>('virtualMachines');
    const items = res.virtualMachines ?? res.virtual_machines ?? [];
    return items.map(mapVirtualMachine);
  }

  async getVirtualMachine(name: string): Promise<VirtualMachine> {
    const raw = await this.client.get<any>(name);
    return mapVirtualMachine(raw);
  }

  async createVirtualMachine(
    vmId: string,
    vm: Partial<VirtualMachine>
  ): Promise<VirtualMachine> {
    const payload = {
      vcpus: vm.vcpus,
      memoryMib: vm.memoryMib,
      diskPath: vm.diskPath,
      diskSizeGib: vm.diskSizeGib,
      isoImage: vm.isoImage,
      cdromIsoImage: vm.cdromIsoImage,
      networkBridge: vm.networkBridge,
      autostart: vm.autostart,
      osVariant: vm.osVariant,
      description: vm.description,
      sockets: vm.sockets,
      cores: vm.cores,
      cpuType: vm.cpuType,
      cpuUnits: vm.cpuUnits,
      cpuLimit: vm.cpuLimit,
      numa: vm.numa,
      balloonMinMib: vm.balloonMinMib,
      hugepages: vm.hugepages,
      machineType: vm.machineType,
      biosType: vm.biosType,
      tpmVersion: vm.tpmVersion,
      secureBoot: vm.secureBoot,
      disks: vm.disks,
      networkInterfaces: vm.networkInterfaces,
      cloudInit: vm.cloudInit,
      bootOrder: vm.bootOrder,
      watchdogModel: vm.watchdogModel,
      watchdogAction: vm.watchdogAction,
      displayType: vm.displayType,
      qemuCommandlineArgs: vm.qemuCommandlineArgs,
    };
    const raw = await this.client.post<any>('virtualMachines', payload, {
      virtualMachineId: vmId,
    });
    return mapVirtualMachine(raw);
  }

  async updateVirtualMachine(
    name: string,
    vm: Partial<VirtualMachine>,
    updateMask: string[]
  ): Promise<VirtualMachine> {
    const payload = {
      name,
      vcpus: vm.vcpus,
      memoryMib: vm.memoryMib,
      autostart: vm.autostart,
      description: vm.description,
      isoImage: vm.isoImage,
      cdromIsoImage: vm.cdromIsoImage,
      sockets: vm.sockets,
      cores: vm.cores,
      cpuType: vm.cpuType,
      balloonMinMib: vm.balloonMinMib,
      machineType: vm.machineType,
      biosType: vm.biosType,
      tpmVersion: vm.tpmVersion,
      secureBoot: vm.secureBoot,
      disks: vm.disks,
      networkInterfaces: vm.networkInterfaces,
      cloudInit: vm.cloudInit,
      bootOrder: vm.bootOrder,
      watchdogModel: vm.watchdogModel,
      watchdogAction: vm.watchdogAction,
      displayType: vm.displayType,
      qemuCommandlineArgs: vm.qemuCommandlineArgs,
    };
    const raw = await this.client.patch<any>(name, payload, updateMask);
    return mapVirtualMachine(raw);
  }

  async deleteVirtualMachine(name: string): Promise<void> {
    await this.client.delete(name);
  }

  async startVirtualMachine(name: string): Promise<VirtualMachine> {
    const raw = await this.client.customMethod<any>(name, 'start', {});
    return mapVirtualMachine(raw);
  }

  async stopVirtualMachine(name: string, force = false): Promise<VirtualMachine> {
    const raw = await this.client.customMethod<any>(name, 'stop', { force });
    return mapVirtualMachine(raw);
  }

  async rebootVirtualMachine(name: string): Promise<VirtualMachine> {
    const raw = await this.client.customMethod<any>(name, 'reboot', {});
    return mapVirtualMachine(raw);
  }

  async resetVirtualMachine(name: string): Promise<VirtualMachine> {
    const raw = await this.client.customMethod<any>(name, 'reset', {});
    return mapVirtualMachine(raw);
  }

  async pauseVirtualMachine(name: string): Promise<VirtualMachine> {
    const raw = await this.client.customMethod<any>(name, 'pause', {});
    return mapVirtualMachine(raw);
  }

  async resumeVirtualMachine(name: string): Promise<VirtualMachine> {
    const raw = await this.client.customMethod<any>(name, 'resume', {});
    return mapVirtualMachine(raw);
  }

  async cloneVirtualMachine(
    name: string,
    newId: string,
    newName?: string,
    targetPool?: string,
    linkedClone = true
  ): Promise<VirtualMachine> {
    const raw = await this.client.customMethod<any>(name, 'clone', {
      newVirtualMachineId: newId,
      newName: newName || newId,
      targetPool: targetPool || 'tank',
      linkedClone,
    });
    return mapVirtualMachine(raw);
  }

  async attachMedia(name: string, isoPath: string, deviceId = 'hdc'): Promise<VirtualMachine> {
    const raw = await this.client.customMethod<any>(name, 'attachMedia', {
      deviceId,
      isoPath,
    });
    return mapVirtualMachine(raw);
  }

  async ejectMedia(name: string, deviceId = 'hdc'): Promise<VirtualMachine> {
    const raw = await this.client.customMethod<any>(name, 'ejectMedia', {
      deviceId,
    });
    return mapVirtualMachine(raw);
  }

  async listSnapshots(vmName: string): Promise<VirtualMachineSnapshot[]> {
    const res = await this.client.get<any>(`${vmName}/snapshots`);
    const items = res.snapshots ?? [];
    return items.map(mapVirtualMachineSnapshot);
  }

  async createSnapshot(
    vmName: string,
    snapshotId: string,
    description = '',
    includeRam = false
  ): Promise<VirtualMachineSnapshot> {
    const raw = await this.client.post<any>(`${vmName}/snapshots`, {
      snapshot: {
        description,
        includeRam,
      },
    }, {
      snapshotId,
    });
    return mapVirtualMachineSnapshot(raw);
  }

  async rollbackSnapshot(snapName: string): Promise<VirtualMachine> {
    const raw = await this.client.customMethod<any>(snapName, 'rollback', {});
    return mapVirtualMachine(raw);
  }

  async deleteSnapshot(snapName: string): Promise<void> {
    await this.client.delete(snapName);
  }

  async openVirtualMachineConsole(name: string): Promise<VirtualMachineConsoleSession> {
    const raw = await this.client.customMethod<any>(name, 'openConsole', {});
    return {
      vmName: raw.vmName ?? raw.vm_name ?? '',
      token: raw.token ?? '',
      websocketUrl: raw.websocketUrl ?? raw.websocket_url ?? '',
      vncPort: Number(raw.vncPort ?? raw.vnc_port ?? 5900),
      resolution: raw.resolution ?? '1024x768',
      expiresAt: raw.expiresAt ?? raw.expires_at ?? '',
    };
  }
}

// ---------------------------------------------------------------------------
// 5. HttpDockerAdapter
// ---------------------------------------------------------------------------

export class HttpDockerAdapter implements DockerPort {
  constructor(private readonly client: AipClient) {}

  async listContainers(): Promise<DockerContainer[]> {
    const res = await this.client.get<any>('dockerContainers');
    const items = res.dockerContainers ?? res.docker_containers ?? [];
    return items.map(mapDockerContainer);
  }

  async getContainer(name: string): Promise<DockerContainer> {
    const raw = await this.client.get<any>(name);
    return mapDockerContainer(raw);
  }

  async createContainer(
    containerId: string,
    container: Partial<DockerContainer>
  ): Promise<DockerContainer> {
    const payload = {
      image: container.image,
      ports: container.ports ?? [],
      volumes: container.volumes ?? [],
      environment: container.environment ?? [],
      restartPolicy: container.restartPolicy,
      networkMode: container.networkMode,
      memoryLimitMib: container.memoryLimitMib,
      command: container.command ?? [],
      privileged: container.privileged ?? false,
      composeYaml: container.composeYaml,
    };
    const raw = await this.client.post<any>('dockerContainers', payload, {
      dockerContainerId: containerId,
    });
    return mapDockerContainer(raw);
  }

  async updateContainer(
    name: string,
    container: Partial<DockerContainer>,
    updateMask?: string[]
  ): Promise<DockerContainer> {
    const payload = {
      name,
      image: container.image,
      ports: container.ports,
      volumes: container.volumes,
      environment: container.environment,
      restartPolicy: container.restartPolicy,
      networkMode: container.networkMode,
      memoryLimitMib: container.memoryLimitMib,
      command: container.command,
      privileged: container.privileged,
      composeYaml: container.composeYaml,
    };
    const raw = await this.client.patch<any>(name, payload, updateMask ?? []);
    return mapDockerContainer(raw);
  }

  async getContainerLogs(name: string, tail = 100): Promise<string> {
    const res = await this.client.get<any>(`${name}:logs`, { tail });
    return res.logs ?? '';
  }

  async deleteContainer(name: string, force = true): Promise<void> {
    await this.client.delete(name, { force });
  }

  async startContainer(name: string): Promise<DockerContainer> {
    const raw = await this.client.customMethod<any>(name, 'start', {});
    return mapDockerContainer(raw);
  }

  async stopContainer(name: string): Promise<DockerContainer> {
    const raw = await this.client.customMethod<any>(name, 'stop', {});
    return mapDockerContainer(raw);
  }

  async restartContainer(name: string): Promise<DockerContainer> {
    const raw = await this.client.customMethod<any>(name, 'restart', {});
    return mapDockerContainer(raw);
  }
}

// ---------------------------------------------------------------------------
// 5.1 HttpDockerAppAdapter (TrueNAS SCALE Compose Descriptors)
// ---------------------------------------------------------------------------

export class HttpDockerAppAdapter implements DockerAppPort {
  constructor(private readonly client: AipClient) {}

  async listApps(): Promise<DockerApp[]> {
    const res = await this.client.get<{ dockerApps?: any[]; docker_apps?: any[] }>('dockerApps');
    const items = res.dockerApps ?? res.docker_apps ?? [];
    return items.map(mapDockerApp);
  }

  async getApp(name: string): Promise<DockerApp> {
    const raw = await this.client.get<any>(name);
    return mapDockerApp(raw);
  }

  async createApp(appId: string, composeYaml: string, title?: string): Promise<DockerApp> {
    const payload = {
      name: `dockerApps/${appId}`,
      id: appId,
      title: title || appId,
      composeYaml,
    };
    const raw = await this.client.post<any>('dockerApps', payload, {
      dockerAppId: appId,
    });
    return mapDockerApp(raw);
  }

  async updateApp(name: string, composeYaml: string, title?: string): Promise<DockerApp> {
    const payload = {
      name,
      title,
      composeYaml,
    };
    const raw = await this.client.patch<any>(name, payload, ['compose_yaml', 'title']);
    return mapDockerApp(raw);
  }

  async deleteApp(name: string, deleteVolumes = false): Promise<void> {
    await this.client.delete(name, { deleteVolumes });
  }

  async startApp(name: string): Promise<DockerApp> {
    const raw = await this.client.customMethod<any>(name, 'start', {});
    return mapDockerApp(raw);
  }

  async stopApp(name: string): Promise<DockerApp> {
    const raw = await this.client.customMethod<any>(name, 'stop', {});
    return mapDockerApp(raw);
  }

  async restartApp(name: string): Promise<DockerApp> {
    const raw = await this.client.customMethod<any>(name, 'restart', {});
    return mapDockerApp(raw);
  }

  async getAppLogs(name: string, tail = 100, serviceName?: string): Promise<{ logs: string; lineCount: number }> {
    const params: Record<string, any> = { tail };
    if (serviceName) {
      params.serviceName = serviceName;
      params.service_name = serviceName;
    }
    const res = await this.client.get<any>(`${name}:logs`, params);
    return {
      logs: res.logs ?? '',
      lineCount: res.lineCount ?? res.line_count ?? 0,
    };
  }

  async getDockerConfig(): Promise<DockerConfig> {
    const raw = await this.client.get<any>('dockerConfig');
    const dataset = raw?.dataset || (raw?.pool ? `${raw.pool}/apps` : 'tank/apps');
    const pool = raw?.pool || (dataset.includes('/') ? dataset.split('/')[0] : 'tank');
    const mountpoint = raw?.mountpoint ?? `/${dataset}`;
    const appdataPath = raw?.appdataPath ?? raw?.appdata_path ?? raw?.appdataDataset ?? `${mountpoint}/appdata`;
    const dockerDataRoot = raw?.dockerDataRoot ?? raw?.docker_data_root ?? `${mountpoint}/docker`;
    const isSelected = Boolean(raw?.datasetSelected ?? raw?.dataset_selected ?? raw?.poolSelected ?? raw?.pool_selected ?? false);
    return {
      dataset,
      pool,
      mountpoint,
      appdataPath,
      dockerDataRoot,
      availableDatasets: raw?.availableDatasets ?? raw?.available_datasets ?? [dataset],
      availablePools: raw?.availablePools ?? raw?.available_pools ?? [pool],
      datasetSelected: isSelected,
      appdataDataset: appdataPath,
      poolSelected: isSelected,
    };
  }

  async updateDockerConfig(dataset: string, pool?: string): Promise<DockerConfig> {
    const targetPool = pool || (dataset.includes('/') ? dataset.split('/')[0] : 'tank');
    const raw = await this.client.patch<any>('dockerConfig', {
      dockerConfig: { dataset, pool: targetPool },
    });
    const resDataset = raw?.dataset || dataset;
    const resPool = raw?.pool || targetPool;
    const mountpoint = raw?.mountpoint ?? `/${resDataset}`;
    const appdataPath = raw?.appdataPath ?? raw?.appdata_path ?? `${mountpoint}/appdata`;
    const dockerDataRoot = raw?.dockerDataRoot ?? raw?.docker_data_root ?? `${mountpoint}/docker`;
    return {
      dataset: resDataset,
      pool: resPool,
      mountpoint,
      appdataPath,
      dockerDataRoot,
      availableDatasets: raw?.availableDatasets ?? raw?.available_datasets ?? [resDataset],
      availablePools: raw?.availablePools ?? raw?.available_pools ?? [resPool],
      datasetSelected: Boolean(raw?.datasetSelected ?? raw?.dataset_selected ?? true),
      appdataDataset: appdataPath,
      poolSelected: Boolean(raw?.poolSelected ?? raw?.pool_selected ?? true),
    };
  }

  async listDirectories(path?: string): Promise<DirectoryList> {
    const params: Record<string, any> = {};
    if (path) params.path = path;
    const raw = await this.client.get<any>('filesystem:directories', params);
    const rawEntries = raw?.entries ?? [];
    return {
      currentPath: raw?.currentPath ?? raw?.current_path ?? (path || '/'),
      parentPath: raw?.parentPath ?? raw?.parent_path ?? '/',
      entries: rawEntries.map((e: any) => ({
        name: e.name ?? '',
        path: e.path ?? '',
        isDir: Boolean(e.isDir ?? e.is_dir ?? true),
      })),
    };
  }

  async checkHostPort(port: number, protocol = 'tcp', excludeAppId = ''): Promise<HostPortStatus> {
    const params: Record<string, any> = { port, protocol };
    if (excludeAppId) params.exclude_app_id = excludeAppId;
    const raw = await this.client.get<any>('docker/ports/check', params);
    return {
      port: Number(raw?.port ?? port),
      protocol: String(raw?.protocol ?? protocol),
      available: Boolean(raw?.available ?? false),
      inUseBy: String(raw?.inUseBy ?? raw?.in_use_by ?? ''),
    };
  }

  async checkHostPorts(ports: number[], protocol = 'tcp', excludeAppId = ''): Promise<HostPortStatus[]> {
    if (!ports || ports.length === 0) return [];
    const body: Record<string, any> = {
      ports,
      protocol,
    };
    if (excludeAppId) body.exclude_app_id = excludeAppId;
    const raw = await this.client.post<any>('docker/ports/check', body);
    const rawPorts = raw?.ports ?? [];
    return rawPorts.map((p: any) => ({
      port: Number(p.port),
      protocol: String(p.protocol || protocol),
      available: Boolean(p.available),
      inUseBy: String(p.inUseBy ?? p.in_use_by ?? ''),
    }));
  }
}

// ---------------------------------------------------------------------------
// 6. HttpLxcAdapter
// ---------------------------------------------------------------------------

export class HttpLxcAdapter implements LxcPort {
  constructor(private readonly client: AipClient) {}

  async listContainers(): Promise<LxcContainer[]> {
    const res = await this.client.get<any>('lxcContainers');
    const items = res.lxcContainers ?? res.lxc_containers ?? [];
    return items.map(mapLxcContainer);
  }

  async getContainer(name: string): Promise<LxcContainer> {
    const raw = await this.client.get<any>(name);
    return mapLxcContainer(raw);
  }

  async createContainer(
    containerId: string,
    container: Partial<LxcContainer>
  ): Promise<LxcContainer> {
    const payload = {
      distribution: container.distribution,
      release: container.release,
      architecture: container.architecture,
      autostart: container.autostart,
      unprivileged: container.unprivileged,
      rootfsPath: container.rootfsPath,
      memoryLimitMib: container.memoryLimitMib,
      cpuCores: container.cpuCores,
      templateArchive: container.templateArchive,
    };
    const raw = await this.client.post<any>('lxcContainers', payload, {
      lxcContainerId: containerId,
    });
    return mapLxcContainer(raw);
  }

  async deleteContainer(name: string, force = true): Promise<void> {
    await this.client.delete(name, { force });
  }

  async startContainer(name: string): Promise<LxcContainer> {
    const raw = await this.client.customMethod<any>(name, 'start', {});
    return mapLxcContainer(raw);
  }

  async stopContainer(name: string): Promise<LxcContainer> {
    const raw = await this.client.customMethod<any>(name, 'stop', {});
    return mapLxcContainer(raw);
  }

  async freezeContainer(name: string): Promise<LxcContainer> {
    const raw = await this.client.customMethod<any>(name, 'freeze', {});
    return mapLxcContainer(raw);
  }
}

// ---------------------------------------------------------------------------
// 7. HttpShareAdapter
// ---------------------------------------------------------------------------

export class HttpShareAdapter implements SharePort {
  constructor(private readonly client: AipClient) {}

  // CIFS
  async listCifsShares(): Promise<CifsShare[]> {
    const res = await this.client.get<any>('cifsShares');
    const items = res.cifsShares ?? res.cifs_shares ?? [];
    return items.map(mapCifsShare);
  }

  async getCifsShare(name: string): Promise<CifsShare> {
    const raw = await this.client.get<any>(name);
    return mapCifsShare(raw);
  }

  async createCifsShare(shareId: string, share: Partial<CifsShare>): Promise<CifsShare> {
    const payload = {
      path: share.path,
      comment: share.comment,
      readOnly: share.readOnly,
      guestOk: share.guestOk,
      browseable: share.browseable,
      validUsers: share.validUsers ?? [],
      validGroups: share.validGroups ?? [],
      createMask: share.createMask,
      directoryMask: share.directoryMask,
      enabled: share.enabled,
    };
    const raw = await this.client.post<any>('cifsShares', payload, {
      cifsShareId: shareId,
    });
    return mapCifsShare(raw);
  }

  async updateCifsShare(
    name: string,
    share: Partial<CifsShare>,
    updateMask: string[]
  ): Promise<CifsShare> {
    const payload = {
      name,
      path: share.path,
      comment: share.comment,
      readOnly: share.readOnly,
      guestOk: share.guestOk,
      browseable: share.browseable,
      validUsers: share.validUsers,
      validGroups: share.validGroups,
      createMask: share.createMask,
      directoryMask: share.directoryMask,
      enabled: share.enabled,
    };
    const raw = await this.client.patch<any>(name, payload, updateMask);
    return mapCifsShare(raw);
  }

  async deleteCifsShare(name: string): Promise<void> {
    await this.client.delete(name);
  }

  // WebDAV
  async listWebdavShares(): Promise<WebdavShare[]> {
    const res = await this.client.get<any>('webdavShares');
    const items = res.webdavShares ?? res.webdav_shares ?? [];
    return items.map(mapWebdavShare);
  }

  async getWebdavShare(name: string): Promise<WebdavShare> {
    const raw = await this.client.get<any>(name);
    return mapWebdavShare(raw);
  }

  async createWebdavShare(
    shareId: string,
    share: Partial<WebdavShare>
  ): Promise<WebdavShare> {
    const payload = {
      path: share.path,
      description: share.description,
      readOnly: share.readOnly,
      allowedUsers: share.allowedUsers ?? [],
      allowedGroups: share.allowedGroups ?? [],
      enabled: share.enabled,
    };
    const raw = await this.client.post<any>('webdavShares', payload, {
      webdavShareId: shareId,
    });
    return mapWebdavShare(raw);
  }

  async updateWebdavShare(
    name: string,
    share: Partial<WebdavShare>,
    updateMask: string[]
  ): Promise<WebdavShare> {
    const payload = {
      name,
      path: share.path,
      description: share.description,
      readOnly: share.readOnly,
      allowedUsers: share.allowedUsers,
      allowedGroups: share.allowedGroups,
      enabled: share.enabled,
    };
    const raw = await this.client.patch<any>(name, payload, updateMask);
    return mapWebdavShare(raw);
  }

  async deleteWebdavShare(name: string): Promise<void> {
    await this.client.delete(name);
  }
}

// ---------------------------------------------------------------------------
// 8. HttpIdentityAdapter
// ---------------------------------------------------------------------------

export class HttpIdentityAdapter implements IdentityPort {
  constructor(private readonly client: AipClient) {}

  async listUsers(includeSystemAccounts = false): Promise<User[]> {
    const res = await this.client.get<any>('users', {
      includeSystemAccounts,
    });
    const items = res.users ?? [];
    return items.map(mapUser);
  }

  async getUser(name: string): Promise<User> {
    const raw = await this.client.get<any>(name);
    return mapUser(raw);
  }

  async createUser(userId: string, user: Partial<User>): Promise<User> {
    const payload = {
      uid: user.uid,
      primaryGid: user.primaryGid,
      primaryGroup: user.primaryGroup,
      supplementaryGroups: user.supplementaryGroups ?? [],
      fullName: user.fullName,
      homeDirectory: user.homeDirectory,
      shell: user.shell,
      sshEnabled: user.sshEnabled,
      sshPasswordAuth: user.sshPasswordAuth,
      sshAuthorizedKeys: user.sshAuthorizedKeys ?? [],
      locked: user.locked,
      password: user.password,
      isAdmin: user.isAdmin,
    };
    const raw = await this.client.post<any>('users', payload, {
      userId,
    });
    return mapUser(raw);
  }

  async updateUser(
    name: string,
    user: Partial<User>,
    updateMask: string[]
  ): Promise<User> {
    const payload = {
      name,
      primaryGroup: user.primaryGroup,
      supplementaryGroups: user.supplementaryGroups,
      fullName: user.fullName,
      homeDirectory: user.homeDirectory,
      shell: user.shell,
      sshEnabled: user.sshEnabled,
      sshPasswordAuth: user.sshPasswordAuth,
      sshAuthorizedKeys: user.sshAuthorizedKeys,
      locked: user.locked,
      password: user.password,
      isAdmin: user.isAdmin,
    };
    const raw = await this.client.patch<any>(name, payload, updateMask);
    return mapUser(raw);
  }

  async deleteUser(name: string, removeHome = false): Promise<void> {
    await this.client.delete(name, {
      removeHome,
    });
  }

  async listGroups(includeSystemGroups = false): Promise<Group[]> {
    const res = await this.client.get<any>('groups', {
      includeSystemGroups,
    });
    const items = res.groups ?? [];
    return items.map(mapGroup);
  }

  async getGroup(name: string): Promise<Group> {
    const raw = await this.client.get<any>(name);
    return mapGroup(raw);
  }

  async createGroup(groupId: string, group: Partial<Group>): Promise<Group> {
    const payload = {
      gid: group.gid,
      members: group.members ?? [],
    };
    const raw = await this.client.post<any>('groups', payload, {
      groupId,
    });
    return mapGroup(raw);
  }

  async updateGroup(
    name: string,
    group: Partial<Group>,
    updateMask: string[]
  ): Promise<Group> {
    const payload = {
      name,
      gid: group.gid,
      members: group.members,
    };
    const raw = await this.client.patch<any>(name, payload, updateMask);
    return mapGroup(raw);
  }

  async deleteGroup(name: string): Promise<void> {
    await this.client.delete(name);
  }
}

// ---------------------------------------------------------------------------
// 9. HttpCatalogAdapter (TrueNAS SCALE App Catalog)
// ---------------------------------------------------------------------------

function mapCatalogQuestion(raw: any): CatalogQuestion {
  return {
    variable: raw?.variable ?? '',
    label: raw?.label ?? '',
    description: raw?.description ?? '',
    type: raw?.type ?? 'string',
    defaultValue: raw?.defaultValue ?? raw?.default_value ?? '',
    required: Boolean(raw?.required ?? false),
    group: raw?.group ?? '',
    options: raw?.options ?? [],
    mountPath: raw?.mountPath ?? raw?.mount_path ?? '',
  };
}

function mapCatalogQuestionGroup(raw: any): CatalogQuestionGroup {
  return {
    name: raw?.name ?? '',
    description: raw?.description ?? '',
    questions: (raw?.questions ?? []).map(mapCatalogQuestion),
  };
}

function mapCatalogApp(raw: any): CatalogApp {
  const rawGroups = raw?.questionGroups ?? raw?.question_groups ?? [];
  const rawQuestions = raw?.questions ?? [];
  return {
    appId: raw?.appId ?? raw?.app_id ?? '',
    title: raw?.title ?? '',
    description: raw?.description ?? '',
    category: raw?.category ?? '',
    categories: raw?.categories ?? [],
    iconUrl: raw?.iconUrl ?? raw?.icon_url ?? '',
    home: raw?.home ?? '',
    train: raw?.train ?? 'community',
    version: raw?.version ?? '',
    appVersion: raw?.appVersion ?? raw?.app_version ?? '',
    tags: raw?.tags ?? [],
    screenshots: raw?.screenshots ?? [],
    sources: raw?.sources ?? [],
    containerImage: raw?.containerImage ?? raw?.container_image ?? '',
    composeYaml: raw?.composeYaml ?? raw?.compose_yaml ?? '',
    defaultPort: Number(raw?.defaultPort ?? raw?.default_port ?? 0),
    questionGroups: rawGroups.map(mapCatalogQuestionGroup),
    questions: rawQuestions.map(mapCatalogQuestion),
  };
}

export class HttpCatalogAdapter implements CatalogPort {
  constructor(private readonly client: AipClient) {}

  async listApps(train?: string, category?: string, query?: string): Promise<{ apps: CatalogApp[]; totalSize: number; lastSyncTime?: string }> {
    const params: Record<string, any> = {};
    if (train) params.train = train;
    if (category) params.category = category;
    if (query) params.query = query;

    const res = await this.client.get<any>('catalog/apps', params);
    const items = res?.apps ?? [];
    return {
      apps: items.map(mapCatalogApp),
      totalSize: Number(res?.totalSize ?? res?.total_size ?? items.length),
      lastSyncTime: res?.lastSyncTime ?? res?.last_sync_time ?? '',
    };
  }

  async getApp(appId: string, train?: string): Promise<CatalogApp> {
    const params: Record<string, any> = {};
    if (train) params.train = train;
    const raw = await this.client.get<any>(`catalog/apps/${appId}`, params);
    return mapCatalogApp(raw);
  }

  async syncCatalog(force = false): Promise<{ success: boolean; message: string; totalApps: number; lastSyncTime: string }> {
    const raw = await this.client.customMethod<any>('catalog', 'sync', { force });
    return {
      success: Boolean(raw?.success ?? true),
      message: String(raw?.message ?? ''),
      totalApps: Number(raw?.totalApps ?? raw?.total_apps ?? 0),
      lastSyncTime: String(raw?.lastSyncTime ?? raw?.last_sync_time ?? ''),
    };
  }
}
