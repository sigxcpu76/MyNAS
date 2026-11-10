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

function escapeRegex(str: string): string {
  return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function matchesStoragePath(rawPath: string | undefined, cleanTarget: string, targetPrefix: string, zvolPrefix: string): boolean {
  if (!rawPath) return false;
  const p = rawPath.trim().replace(/\/+$/, '');
  if (!p) return false;
  const slashTarget = `/${cleanTarget}`;
  return (
    p === cleanTarget ||
    p.startsWith(`${cleanTarget}/`) ||
    p === slashTarget ||
    p.startsWith(`${slashTarget}/`) ||
    p === targetPrefix ||
    p.startsWith(`${targetPrefix}/`) ||
    p === zvolPrefix ||
    p.startsWith(`${zvolPrefix}/`)
  );
}

function matchesComposeText(yaml: string, cleanTarget: string, targetPrefix: string): boolean {
  if (!yaml) return false;
  const prefixRe = new RegExp(`(?:^|[\\s"':=])${escapeRegex(targetPrefix)}(?:/|[\\s"':]|$)`);
  const targetRe = new RegExp(`(?:^|[\\s"':=/])${escapeRegex(cleanTarget)}(?:/|[\\s"':]|$)`);
  return prefixRe.test(yaml) || targetRe.test(yaml);
}

export function computeStorageBlastRadius(target: string): StorageBlastRadius {
  const cleanTarget = target.replace(/^(zfsPools\/|pool:)/, '').trim().replace(/\/+$/, '');
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
        return matchesStoragePath(p, cleanTarget, targetPrefix, zvolPrefix);
      });
      return inDisks;
    })
    .map((vm) => vm.name.replace(/^virtualMachines\//, ''));

  // Check LXC containers
  const affectedLxc = (nasState.lxcContainers || [])
    .filter((ct: any) => {
      const rootfs = ct.rootfsPath || ct.rootfs || '';
      const inRootfs = matchesStoragePath(rootfs, cleanTarget, targetPrefix, zvolPrefix);
      const mounts = ct.mountPoints || ct.mounts || [];
      const inMounts = mounts.some((m: any) =>
        matchesStoragePath(m.hostPath, cleanTarget, targetPrefix, zvolPrefix)
      );
      return inRootfs || inMounts;
    })
    .map((ct) => ct.containerName || ct.name.replace(/^lxcContainers\//, ''));

  // Check Docker Apps
  const affectedApps = (nasState.dockerApps || [])
    .filter((app) => {
      const yaml = app.composeYaml || '';
      return matchesComposeText(yaml, cleanTarget, targetPrefix);
    })
    .map((app) => app.title || app.appId);

  // Check CIFS shares
  const affectedCifs = (nasState.cifsShares || [])
    .filter((s) => matchesStoragePath(s.path, cleanTarget, targetPrefix, zvolPrefix))
    .map((s) => s.shareName || s.name.replace(/^cifsShares\//, ''));

  // Check WebDAV shares
  const affectedWebdav = (nasState.webdavShares || [])
    .filter((s) => matchesStoragePath(s.path, cleanTarget, targetPrefix, zvolPrefix))
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
