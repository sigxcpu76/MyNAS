import { nasState } from '../store';

export interface StorageBlastRadius {
  isMainPool: boolean;
  affectedVms: string[];
  affectedLxc: string[];
  affectedApps: string[];
  affectedContainers: string[];
  affectedCronJobs: string[];
  affectedCifs: string[];
  affectedWebdav: string[];
  hasDependencies: boolean;
  summaryWarning: string;
}

function escapeRegex(str: string): string {
  return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function normalizePath(p: string | undefined): string {
  if (!p) return '';
  const trimmed = p.trim().replace(/\/+$/, '');
  return trimmed === '-' || trimmed === 'none' || trimmed === 'legacy' ? '' : trimmed;
}

function collectCandidateMounts(cleanTarget: string, poolName: string): string[] {
  const candidates = new Set<string>([
    cleanTarget,
    `/${cleanTarget}`,
    `/mnt/${cleanTarget}`,
  ]);

  const pool = (nasState.pools || []).find(
    (p) => (p.poolName || p.name.replace(/^zfsPools\//, '')) === poolName
  );
  const poolMp = normalizePath(pool?.mountpoint);
  if (poolMp) {
    const rel = cleanTarget === poolName ? '' : cleanTarget.slice(poolName.length + 1);
    candidates.add(rel ? `${poolMp}/${rel}` : poolMp);
  }

  for (const key of [`zfsPools/${poolName}`, poolName]) {
    const dsList = nasState.datasetsByPool?.[key] || [];
    for (const ds of dsList) {
      const dsPath = (ds.datasetPath || '').trim().replace(/\/+$/, '');
      const mp = normalizePath(ds.mountpoint);
      if (!dsPath || !mp) continue;
      if (dsPath === cleanTarget || dsPath.startsWith(`${cleanTarget}/`)) {
        candidates.add(mp);
      }
    }
  }

  return Array.from(candidates).filter(Boolean);
}

function matchesStoragePath(
  rawPath: string | undefined,
  candidateMounts: string[],
  zvolPrefix: string
): boolean {
  const p = normalizePath(rawPath);
  if (!p) return false;
  if (p === zvolPrefix || p.startsWith(`${zvolPrefix}/`)) {
    return true;
  }
  return candidateMounts.some((cand) => p === cand || p.startsWith(`${cand}/`));
}

function matchesTextReference(text: string | undefined, candidateMounts: string[], zvolPrefix: string): boolean {
  if (!text) return false;
  const allTargets = [...candidateMounts, zvolPrefix];
  return allTargets.some((target) => {
    const re = new RegExp(`(?:^|[\\s"':=])${escapeRegex(target)}(?:/|[\\s"':]|$)`);
    return re.test(text);
  });
}

export function computeStorageBlastRadius(target: string): StorageBlastRadius {
  const cleanTarget = target.replace(/^(zfsPools\/|pool:)/, '').trim().replace(/\/+$/, '');
  const poolName = cleanTarget.split('/')[0];

  const mainPoolClean = (nasState.systemConfig?.mainPool || '').replace(/^zfsPools\//, '').trim();
  const isMainPool = Boolean(
    mainPoolClean &&
      (mainPoolClean === cleanTarget ||
        (cleanTarget === poolName && mainPoolClean === poolName))
  );

  const candidateMounts = collectCandidateMounts(cleanTarget, poolName);
  const zvolPrefix = `/dev/zvol/${cleanTarget}`;

  // Check VMs
  const affectedVms = (nasState.vms || [])
    .filter((vm) => {
      return vm.disks?.some((d: any) => {
        const p = d.diskPath || d.path || '';
        return matchesStoragePath(p, candidateMounts, zvolPrefix);
      });
    })
    .map((vm) => vm.name.replace(/^virtualMachines\//, ''));

  // Check LXC containers
  const affectedLxc = (nasState.lxcContainers || [])
    .filter((ct: any) => {
      const rootfs = ct.rootfsPath || ct.rootfs || '';
      const inRootfs = matchesStoragePath(rootfs, candidateMounts, zvolPrefix);
      const mounts = ct.mountPoints || ct.mounts || [];
      const inMounts = mounts.some((m: any) =>
        matchesStoragePath(m.hostPath, candidateMounts, zvolPrefix)
      );
      return inRootfs || inMounts;
    })
    .map((ct) => ct.containerName || ct.name.replace(/^lxcContainers\//, ''));

  // Check Docker Apps (Compose stacks)
  const affectedApps = (nasState.dockerApps || [])
    .filter((app) => matchesTextReference(app.composeYaml || '', candidateMounts, zvolPrefix))
    .map((app) => app.title || app.appId);

  // Check standalone Docker Containers + Docker data-root config
  const affectedContainers = (nasState.dockerContainers || [])
    .filter((c: any) => {
      const vols: string[] = Array.isArray(c.volumes)
        ? c.volumes
        : Array.isArray(c.mounts)
        ? c.mounts
        : [];
      return vols.some((v) => {
        const hostSide = String(v || '').split(':')[0];
        return matchesStoragePath(hostSide, candidateMounts, zvolPrefix);
      });
    })
    .map((c: any) => c.containerName || String(c.name || '').replace(/^dockerContainers\//, ''));

  const dockerCfg: any = (nasState as any).dockerConfig;
  if (dockerCfg) {
    const dockerPool = String(dockerCfg.pool || '').replace(/^zfsPools\//, '').trim();
    const dockerPaths = [
      dockerCfg.dockerDataRoot,
      dockerCfg.dataRoot,
      dockerCfg.appdataPath,
      dockerCfg.mountpoint,
      dockerCfg.dataset,
    ]
      .map((v) => String(v || '').trim())
      .filter(Boolean);
    if (
      (dockerPool && cleanTarget === dockerPool) ||
      dockerPaths.some((dp) => matchesStoragePath(dp, candidateMounts, zvolPrefix))
    ) {
      if (!affectedContainers.includes('Docker Engine data-root')) {
        affectedContainers.unshift('Docker Engine data-root');
      }
    }
  }

  // Check Cron Jobs (container volume mounts + command/args)
  const affectedCronJobs = (nasState.cronJobs || [])
    .filter((job: any) => {
      const volMounts: any[] = Array.isArray(job.volumes) ? job.volumes : [];
      if (volMounts.some((m) => matchesStoragePath(m?.hostPath, candidateMounts, zvolPrefix))) {
        return true;
      }
      const cmdParts = Array.isArray(job.command) ? job.command : [job.command || ''];
      const argParts = Array.isArray(job.args) ? job.args : [];
      const cmdLine = [...cmdParts, ...argParts, job.workingDir || ''].join(' ');
      return matchesTextReference(cmdLine, candidateMounts, zvolPrefix);
    })
    .map((job: any) => job.title || job.displayName || job.id || job.jobId || String(job.name || '').replace(/^cronJobs\//, ''));

  // Check CIFS shares
  const affectedCifs = (nasState.cifsShares || [])
    .filter((s) => matchesStoragePath(s.path, candidateMounts, zvolPrefix))
    .map((s) => s.shareName || s.name.replace(/^cifsShares\//, ''));

  // Check WebDAV shares
  const affectedWebdav = (nasState.webdavShares || [])
    .filter((s) => matchesStoragePath(s.path, candidateMounts, zvolPrefix))
    .map((s) => s.shareName || s.name.replace(/^webdavShares\//, ''));

  const hasDependencies =
    isMainPool ||
    affectedVms.length > 0 ||
    affectedLxc.length > 0 ||
    affectedApps.length > 0 ||
    affectedContainers.length > 0 ||
    affectedCronJobs.length > 0 ||
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
  if (affectedContainers.length > 0) {
    lines.push(`• Dependent Docker Containers (${affectedContainers.length}): ${affectedContainers.join(', ')}`);
  }
  if (affectedCronJobs.length > 0) {
    lines.push(`• Scheduled Cron Jobs (${affectedCronJobs.length}): ${affectedCronJobs.join(', ')}`);
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
    affectedContainers,
    affectedCronJobs,
    affectedCifs,
    affectedWebdav,
    hasDependencies,
    summaryWarning: lines.join('\n'),
  };
}
