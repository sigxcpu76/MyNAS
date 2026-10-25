import { describe, it, expect } from 'vitest';
import {
  generateNanosecondId,
  generateVMDiskName,
  generateZvolDiskPath,
  generateLxcRootfsPath,
  generateAppdataPath,
} from '../src/domain/id';

describe('Domain Nanosecond ID Generator (TypeScript)', () => {
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

  it('generates standardized VM disk names and zvol paths', () => {
    const vmId = '1761411800123456789';
    const diskName = generateVMDiskName(vmId, 0);
    expect(diskName).toBe('vm-1761411800123456789-disk-0');

    const disk1 = generateVMDiskName(`virtualMachines/${vmId}`, 1);
    expect(disk1).toBe('vm-1761411800123456789-disk-1');

    const zvolPath = generateZvolDiskPath('mypool', vmId, 0);
    expect(zvolPath).toBe('/dev/zvol/mypool/vm-1761411800123456789-disk-0');
  });

  it('generates standardized LXC rootfs and Appdata paths', () => {
    const id = '1761411800123456789';
    const lxcPath = generateLxcRootfsPath('fastpool', id);
    expect(lxcPath).toBe('/nas/fastpool/lxc/1761411800123456789/rootfs');

    const appPath = generateAppdataPath('fastpool', id);
    expect(appPath).toBe('/nas/fastpool/apps/appdata/1761411800123456789');
  });
});
