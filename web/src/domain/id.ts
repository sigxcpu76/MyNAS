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
 * E.g. "vm-1761411800123456789-disk-0"
 */
export function generateVMDiskName(vmId: string, diskIndex = 0): string {
  const cleanId = vmId.replace(/^virtualMachines\//, '').trim() || generateNanosecondId();
  return `vm-${cleanId}-disk-${diskIndex}`;
}

/**
 * Generates a standard ZFS ZVol block device path for a VM disk on a storage pool.
 * E.g. "/dev/zvol/mypool/vm-1761411800123456789-disk-0"
 */
export function generateZvolDiskPath(pool: string, vmId: string, diskIndex = 0): string {
  const cleanPool = pool.replace(/^zfsPools\//, '').trim() || 'tank';
  return `/dev/zvol/${cleanPool}/${generateVMDiskName(vmId, diskIndex)}`;
}

/**
 * Generates a standard LXC rootfs path on the main pool.
 * E.g. "/nas/mypool/lxc/1761411800123456789/rootfs"
 */
export function generateLxcRootfsPath(mainPool: string, containerId: string): string {
  const cleanPool = mainPool.replace(/^zfsPools\//, '').trim() || 'tank';
  const cleanId = containerId.replace(/^lxcContainers\//, '').trim() || generateNanosecondId();
  return `/nas/${cleanPool}/lxc/${cleanId}/rootfs`;
}

/**
 * Generates a standard application persistent storage path on the main pool.
 * E.g. "/nas/mypool/apps/appdata/1761411800123456789"
 */
export function generateAppdataPath(mainPool: string, appId: string): string {
  const cleanPool = mainPool.replace(/^zfsPools\//, '').trim() || 'tank';
  const cleanId = appId.replace(/^dockerApps\//, '').trim() || generateNanosecondId();
  return `/nas/${cleanPool}/apps/appdata/${cleanId}`;
}
