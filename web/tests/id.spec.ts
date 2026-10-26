import { describe, it, expect } from 'vitest';
import {
  generateNanosecondId,
  generateVMDiskFilePath,
  generateVMDiskName,
  generateZvolDiskPath,
  generateVMPath,
  generateLxcRootfsPath,
  generateAppdataPath,
  toZfsSafeSlug,
} from '../src/domain/id';

describe('Domain Slug and Resource Path Helpers (TypeScript)', () => {
  it('computes ZFS safe slugs from entity names', () => {
    expect(toZfsSafeSlug('Nextcloud Hub 28!')).toBe('nextcloud-hub-28');
    expect(toZfsSafeSlug('My Ubuntu VM (v2.0)')).toBe('my-ubuntu-vm-v2-0');
    expect(toZfsSafeSlug('---hello---world---')).toBe('hello-world');
    expect(toZfsSafeSlug('.hidden.app.')).toBe('hidden-app');
    expect(toZfsSafeSlug('My_VM_01')).toBe('my_vm_01');
    expect(toZfsSafeSlug('  Custom App Title  ')).toBe('custom-app-title');
    expect(toZfsSafeSlug('')).toBe('');
    expect(toZfsSafeSlug('   ')).toBe('');
    expect(toZfsSafeSlug('1761411800123456789')).toBe('1761411800123456789');
  });

  it('generates strictly increasing unique nanosecond IDs', () => {
    const ids = new Set<string>();
    for (let i = 0; i < 500; i++) {
      const id = generateNanosecondId();
      expect(ids.has(id)).toBe(false);
      ids.add(id);
      expect(/^\d{16,20}$/.test(id)).toBe(true);
    }
    expect(ids.size).toBe(500);
  });

  it('generates standardized VM disk names and zvol paths rooted to /vms/<vm_id>', () => {
    const vmName = 'Debian Server';
    const vmId = toZfsSafeSlug(vmName);
    expect(vmId).toBe('debian-server');

    const diskName = generateVMDiskName(vmId, 0);
    expect(diskName).toBe('vm-debian-server-disk-0');

    const disk1 = generateVMDiskName(`virtualMachines/${vmId}`, 1);
    expect(disk1).toBe('vm-debian-server-disk-1');

    const zvolPath = generateZvolDiskPath('mypool', vmId, 0);
    expect(zvolPath).toBe('/dev/zvol/mypool/vms/debian-server');

    const zvolPath1 = generateZvolDiskPath('mypool', vmId, 1);
    expect(zvolPath1).toBe('/dev/zvol/mypool/vms/debian-server-disk-1');

    const vmDir = generateVMPath('mypool', vmId);
    expect(vmDir).toBe('/nas/mypool/vms/debian-server');

    const vmFilePath = generateVMDiskFilePath('mypool', vmId, 0);
    expect(vmFilePath).toBe('/nas/mypool/vms/debian-server/disk-0.raw');
  });

  it('generates standardized LXC rootfs rooted to /containers/<container_id>', () => {
    const name = 'Ubuntu 24 Noble';
    const id = toZfsSafeSlug(name);
    expect(id).toBe('ubuntu-24-noble');

    const lxcPath = generateLxcRootfsPath('fastpool', id);
    expect(lxcPath).toBe('/nas/fastpool/containers/ubuntu-24-noble/rootfs');
  });

  it('generates standardized App persistent paths rooted to /apps/<app_id>', () => {
    const appTitle = 'Nextcloud Hub';
    const appId = toZfsSafeSlug(appTitle);
    expect(appId).toBe('nextcloud-hub');

    const appPath = generateAppdataPath('fastpool', appId);
    expect(appPath).toBe('/nas/fastpool/apps/nextcloud-hub');
  });
});
