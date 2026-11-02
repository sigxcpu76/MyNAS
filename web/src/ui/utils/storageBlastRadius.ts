import { nasState } from '../store';

export interface StorageBlastRadius {
  isMainPool: boolean;
  affectedVms: string[];
  affectedLxc: string[];
  affectedApps: string[];
  affectedCifs: string[];
  affectedWebdav: string[];
  hasDependencies: boolean;
  summaryWarning: string;
}

export function computeStorageBlastRadius(target: string): StorageBlastRadius {
  const cleanTarget = target.replace(/^(zfsPools\/|pool:)/, '').trim();
  const poolName = cleanTarget.split('/')[0];

  const isMainPool = Boolean(
    nasState.systemConfig?.mainPool &&
      (nasState.systemConfig.mainPool === cleanTarget ||
        nasState.systemConfig.mainPool.replace(/^zfsPools\//, '') === cleanTarget ||
        (cleanTarget === poolName &&
          nasState.systemConfig.mainPool.replace(/^zfsPools\//, '') === poolName))
  );

  const targetPrefix = `/mnt/${cleanTarget}`;
  const zvolPrefix = `/dev/zvol/${cleanTarget}`;

  // Check VMs
  const affectedVms = (nasState.vms || [])
    .filter((vm) => {
      const inDisks = vm.disks?.some((d: any) => {
        const p = d.diskPath || d.path || '';
        return (
          p.includes(cleanTarget) ||
          p.startsWith(targetPrefix) ||
          p.startsWith(zvolPrefix)
        );
      });
      return inDisks;
    })
    .map((vm) => vm.name.replace(/^virtualMachines\//, ''));

  // Check LXC containers
  const affectedLxc = (nasState.lxcContainers || [])
    .filter((ct: any) => {
      const rootfs = ct.rootfsPath || ct.rootfs || '';
      const inRootfs = rootfs.includes(cleanTarget) || rootfs.startsWith(targetPrefix);
      const mounts = ct.mountPoints || ct.mounts || [];
      const inMounts = mounts.some(
        (m: any) => m.hostPath?.includes(cleanTarget) || m.hostPath?.startsWith(targetPrefix)
      );
      return inRootfs || inMounts;
    })
    .map((ct) => ct.containerName || ct.name.replace(/^lxcContainers\//, ''));

  // Check Docker Apps
  const affectedApps = (nasState.dockerApps || [])
    .filter((app) => {
      const yaml = app.composeYaml || '';
      return yaml.includes(cleanTarget) || yaml.includes(targetPrefix);
    })
    .map((app) => app.title || app.appId);

  // Check CIFS shares
  const affectedCifs = (nasState.cifsShares || [])
    .filter((s) => s.path?.includes(cleanTarget) || s.path?.startsWith(targetPrefix))
    .map((s) => s.shareName || s.name.replace(/^cifsShares\//, ''));

  // Check WebDAV shares
  const affectedWebdav = (nasState.webdavShares || [])
    .filter((s) => s.path?.includes(cleanTarget) || s.path?.startsWith(targetPrefix))
    .map((s) => s.shareName || s.name.replace(/^webdavShares\//, ''));

  const hasDependencies =
    isMainPool ||
    affectedVms.length > 0 ||
    affectedLxc.length > 0 ||
    affectedApps.length > 0 ||
    affectedCifs.length > 0 ||
    affectedWebdav.length > 0;

  const lines: string[] = [];
  if (isMainPool) {
    lines.push(
      '⚠️ DESIGNATED MAIN SYSTEM POOL: Destroying or exporting this pool will disable non-ZFS services (Docker, KVM, LXC, Shares).'
    );
  }
  if (affectedVms.length > 0) {
    lines.push(`• Dependent Virtual Machines (${affectedVms.length}): ${affectedVms.join(', ')}`);
  }
  if (affectedLxc.length > 0) {
    lines.push(`• Dependent LXC Containers (${affectedLxc.length}): ${affectedLxc.join(', ')}`);
  }
  if (affectedApps.length > 0) {
    lines.push(`• Dependent Docker Apps (${affectedApps.length}): ${affectedApps.join(', ')}`);
  }
  if (affectedCifs.length > 0) {
    lines.push(`• SMB Shares (${affectedCifs.length}): ${affectedCifs.join(', ')}`);
  }
  if (affectedWebdav.length > 0) {
    lines.push(`• WebDAV Shares (${affectedWebdav.length}): ${affectedWebdav.join(', ')}`);
  }

  return {
    isMainPool,
    affectedVms,
    affectedLxc,
    affectedApps,
    affectedCifs,
    affectedWebdav,
    hasDependencies,
    summaryWarning: lines.join('\n'),
  };
}
