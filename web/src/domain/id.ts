/**
 * Computes a ZFS-safe slug identifier from an arbitrary entity name.
 * ZFS dataset components can only contain alphanumeric characters, hyphens, underscores, colons, and periods,
 * and must NOT begin with a period or hyphen.
 * The slug is converted to lowercase, non-alphanumeric characters replaced by hyphens,
 * consecutive hyphens collapsed, leading/trailing punctuation trimmed, and length truncated to 63 chars.
 */
export function toZfsSafeSlug(name: string): string {
  if (!name) return '';
  return name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9_-]+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^[-_.]+|[-_.]+$/g, '')
    .slice(0, 63);
}

/**
 * Generates a strictly unique, monotonic numeric ID based on the creation timestamp at the nanosecond level.
 * E.g. "1761411800123456789"
 */
let lastNanosecondId = 0n;

export function generateNanosecondId(): string {
  const millisNanos = BigInt(Date.now()) * 1_000_000n;
  const perfNanos =
    typeof performance !== 'undefined'
      ? BigInt(Math.floor((performance.now() % 1) * 1_000_000))
      : 0n;

  let now = millisNanos + perfNanos;
  if (now <= lastNanosecondId) {
    now = lastNanosecondId + 1n;
  }
  lastNanosecondId = now;
  return now.toString();
}

/**
 * Generates a standardized disk name based on the VM ID and disk index.
 * E.g. "vm-my-vm-disk-0"
 */
export function generateVMDiskName(vmId: string, diskIndex = 0): string {
  const cleanId = toZfsSafeSlug(vmId.replace(/^virtualMachines\//, '').trim()) || 'vm';
  return `vm-${cleanId}-disk-${diskIndex}`;
}

/**
 * Generates a standard ZFS ZVol block device path for a VM disk on a storage pool.
 * All VMs are rooted to the main pool under /vms/<vm_id>.
 * E.g. "/dev/zvol/mypool/vms/my-vm" or "/dev/zvol/mypool/vms/my-vm-disk-1"
 */
export function generateZvolDiskPath(pool: string, vmId: string, diskIndex = 0): string {
  const cleanPool = pool.replace(/^zfsPools\//, '').trim() || 'tank';
  const cleanId = toZfsSafeSlug(vmId.replace(/^virtualMachines\//, '').trim()) || 'vm';
  if (diskIndex === 0) {
    return `/dev/zvol/${cleanPool}/vms/${cleanId}`;
  }
  return `/dev/zvol/${cleanPool}/vms/${cleanId}-disk-${diskIndex}`;
}

/**
 * Generates the root directory path for a VM on the main storage pool.
 * E.g. "/nas/mypool/vms/my-vm"
 */
export function generateVMPath(mainPool: string, vmId: string): string {
  const cleanPool = mainPool.replace(/^zfsPools\//, '').trim() || 'tank';
  const cleanId = toZfsSafeSlug(vmId.replace(/^virtualMachines\//, '').trim()) || 'vm';
  return `/nas/${cleanPool}/vms/${cleanId}`;
}

/**
 * Generates a standard LXC rootfs path on the main pool.
 * All LXC containers are rooted to the main pool under /containers/<container_id>.
 * E.g. "/nas/mypool/containers/my-container/rootfs"
 */
export function generateLxcRootfsPath(mainPool: string, containerId: string): string {
  const cleanPool = mainPool.replace(/^zfsPools\//, '').trim() || 'tank';
  const cleanId = toZfsSafeSlug(containerId.replace(/^lxcContainers\//, '').trim()) || 'container';
  return `/nas/${cleanPool}/containers/${cleanId}/rootfs`;
}

/**
 * Generates a standard application persistent storage path on the main pool.
 * All applications are rooted to the main pool under /apps/<app_id>.
 * E.g. "/nas/mypool/apps/nextcloud"
 */
export function generateAppdataPath(mainPool: string, appId: string): string {
  const cleanPool = mainPool.replace(/^zfsPools\//, '').trim() || 'tank';
  const cleanId = toZfsSafeSlug(appId.replace(/^dockerApps\//, '').trim()) || 'app';
  return `/nas/${cleanPool}/apps/${cleanId}`;
}
