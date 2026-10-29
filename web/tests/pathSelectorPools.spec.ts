import { describe, expect, it, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { nextTick } from 'vue';
import { nasState } from '../src/ui/store';
import PathSelector from '../src/ui/components/PathSelector.vue';
import WidgetPools from '../src/ui/dashboard/widgets/WidgetPools.vue';
import { mapZfsPool, mapSystemConfig } from '../src/adapters/http/httpAdapters';

describe('Pool Mountpoints & Path Suggestions', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    nasState.systemConfig = {
      mainPool: 'zfsPools/fast',
      systemDataset: '.nas-system',
      systemMountpoint: '/mnt/fast/.nas-system',
      isInitialized: true,
      enabledModules: ['zfs', 'docker'],
      configFilePath: '/etc/nasctl.yaml',
      autoDiscover: true,
      discoveredPools: [],
    };
    nasState.pools = [
      {
        name: 'zfsPools/fast',
        poolName: 'fast',
        state: 'ONLINE',
        sizeBytes: 1000000000,
        allocatedBytes: 100000000,
        freeBytes: 900000000,
        fragmentationPercent: 0,
        capacityPercent: 10,
        dedupRatio: 1.0,
        vdevLayout: 'mirror',
        devices: ['/dev/sda'],
        mountpoint: '/mnt/fast',
        compression: 'lz4',
        lastScrubStatus: '',
        healthDetails: '',
        sync: 'standard',
        autotrim: 'off',
        vdevs: [],
      },
      {
        name: 'zfsPools/raid0',
        poolName: 'raid0',
        state: 'ONLINE',
        sizeBytes: 2000000000,
        allocatedBytes: 200000000,
        freeBytes: 1800000000,
        fragmentationPercent: 0,
        capacityPercent: 10,
        dedupRatio: 1.0,
        vdevLayout: 'stripe',
        devices: ['/dev/sdb'],
        mountpoint: '', // empty mountpoint should fallback to /mnt/raid0
        compression: 'lz4',
        lastScrubStatus: '',
        healthDetails: '',
        sync: 'standard',
        autotrim: 'off',
        vdevs: [],
      },
      {
        name: 'zfsPools/sas-pool',
        poolName: 'sas-pool',
        state: 'ONLINE',
        sizeBytes: 3000000000,
        allocatedBytes: 300000000,
        freeBytes: 2700000000,
        fragmentationPercent: 0,
        capacityPercent: 10,
        dedupRatio: 1.0,
        vdevLayout: 'raidz1',
        devices: ['/dev/sdc'],
        mountpoint: '/mnt/sas-pool',
        compression: 'lz4',
        lastScrubStatus: '',
        healthDetails: '',
        sync: 'standard',
        autotrim: 'off',
        vdevs: [],
      },
      {
        name: 'zfsPools/storage',
        poolName: 'storage',
        state: 'ONLINE',
        sizeBytes: 4000000000,
        allocatedBytes: 400000000,
        freeBytes: 3600000000,
        fragmentationPercent: 0,
        capacityPercent: 10,
        dedupRatio: 1.0,
        vdevLayout: 'mirror',
        devices: ['/dev/sdd'],
        mountpoint: '/mnt/storage',
        compression: 'lz4',
        lastScrubStatus: '',
        healthDetails: '',
        sync: 'standard',
        autotrim: 'off',
        vdevs: [],
      },
    ];
  });

  it('renders quick jump shortcuts rooted directly at /mnt/<pool> and NEVER /mnt/zfsPools/<pool>', async () => {
    const wrapper = mount(PathSelector, {
      props: {
        modelValue: '',
      },
    });

    // Open browser modal
    const browseBtn = wrapper.find('button[title="Browse Folders..."]');
    expect(browseBtn.exists()).toBe(true);
    await browseBtn.trigger('click');
    await nextTick();

    const text = wrapper.text();
    expect(text).toContain('Quick Jump:');
    expect(text).toContain('/mnt/fast');
    expect(text).toContain('/mnt/raid0');
    expect(text).toContain('/mnt/sas-pool');
    expect(text).toContain('/mnt/storage');

    // Crucial requirement: No zfsPools in any path
    expect(text).not.toContain('/mnt/zfsPools');
    expect(text).not.toContain('zfsPools/fast');
  });

  it('renders WidgetPools pool cards without zfsPools in header or mountpoint', async () => {
    const wrapper = mount(WidgetPools);
    await nextTick();

    const text = wrapper.text();
    expect(text).toContain('fast');
    expect(text).toContain('raid0');
    expect(text).toContain('sas-pool');
    expect(text).toContain('storage');

    expect(text).not.toContain('zfsPools/fast');
    expect(text).not.toContain('zfsPools/raid0');
    expect(text).not.toContain('/mnt/zfsPools');
  });

  it('mapZfsPool strips zfsPools from name and mountpoint', () => {
    const raw = {
      name: 'zfsPools/tank',
      poolName: 'zfsPools/tank',
      mountpoint: '/mnt/zfsPools/tank',
    };
    const mapped = mapZfsPool(raw);
    expect(mapped.poolName).toBe('tank');
    expect(mapped.mountpoint).toBe('/mnt/tank');
  });

  it('mapSystemConfig strips zfsPools from mainPool', () => {
    const raw = {
      mainPool: 'zfsPools/storage',
      systemDataset: '.nas-system',
    };
    const mapped = mapSystemConfig(raw);
    expect(mapped.mainPool).toBe('storage');
  });

  it('supports selectFiles and fileExtensions filtering in PathSelector', async () => {
    const wrapper = mount(PathSelector, {
      props: {
        modelValue: '',
        selectFiles: true,
        fileExtensions: ['.iso', '.img'],
      },
    });

    const browseBtn = wrapper.find('button[title="Browse Folders..."]');
    expect(browseBtn.exists()).toBe(true);
    await browseBtn.trigger('click');
    await nextTick();

    expect(wrapper.text()).toContain('Click any file row or "Select" to use it as boot / media image');
  });
});
