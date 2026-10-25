import { describe, expect, it, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import DatasetHierarchyTree from '../src/ui/components/DatasetHierarchyTree.vue';
import { nasState } from '../src/ui/store';
import type { ZfsPool, ZfsDataset } from '../src/domain/models';

describe('DatasetHierarchyTree Component', () => {
  beforeEach(() => {
    // Populate store with test pool and datasets
    nasState.pools = [
      {
        name: 'zfsPools/tank',
        poolName: 'tank',
        state: 'ONLINE',
        health: 'HEALTHY',
        scanStatus: 'none',
        allocatedBytes: 10 * 1024 * 1024 * 1024,
        freeBytes: 90 * 1024 * 1024 * 1024,
        sizeBytes: 100 * 1024 * 1024 * 1024,
        fragmentationPercent: 0,
        readErrors: 0,
        writeErrors: 0,
        checksumErrors: 0,
        vdevs: [],
        mountpoint: '/nas/tank',
      } as unknown as ZfsPool,
    ];

    nasState.datasetsByPool['zfsPools/tank'] = [
      {
        name: 'zfsPools/tank/datasets/apps',
        datasetPath: 'tank/apps',
        mountpoint: '/nas/tank/apps',
        usedBytes: 2 * 1024 * 1024 * 1024,
        availableBytes: 88 * 1024 * 1024 * 1024,
        referencedBytes: 0,
        quotaBytes: 0,
        compression: 'lz4',
        readonly: false,
        atime: true,
        recordsize: '128k',
        snapshotsCount: 0,
        lastSnapshot: '',
      } as unknown as ZfsDataset,
      {
        name: 'zfsPools/tank/datasets/apps/appdata',
        datasetPath: 'tank/apps/appdata',
        mountpoint: '/nas/tank/apps/appdata',
        usedBytes: 1 * 1024 * 1024 * 1024,
        availableBytes: 87 * 1024 * 1024 * 1024,
        referencedBytes: 0,
        quotaBytes: 0,
        compression: 'lz4',
        readonly: false,
        atime: true,
        recordsize: '128k',
        snapshotsCount: 0,
        lastSnapshot: '',
      } as unknown as ZfsDataset,
      {
        name: 'zfsPools/tank/datasets/media',
        datasetPath: 'tank/media',
        mountpoint: '/nas/tank/media',
        usedBytes: 5 * 1024 * 1024 * 1024,
        availableBytes: 85 * 1024 * 1024 * 1024,
        referencedBytes: 0,
        quotaBytes: 0,
        compression: 'lz4',
        readonly: false,
        atime: true,
        recordsize: '128k',
        snapshotsCount: 0,
        lastSnapshot: '',
      } as unknown as ZfsDataset,
    ];
  });

  it('renders dataset hierarchy tree with pools and datasets', () => {
    const wrapper = mount(DatasetHierarchyTree, {
      props: {
        modelValue: '',
      },
    });

    expect(wrapper.text()).toContain('ZFS Dataset Hierarchy');
    expect(wrapper.text()).toContain('tank');
    expect(wrapper.text()).toContain('/nas/tank');
    expect(wrapper.text()).toContain('apps');
    expect(wrapper.text()).toContain('media');

    wrapper.unmount();
  });

  it('selects a dataset when clicked and emits update:modelValue', async () => {
    const wrapper = mount(DatasetHierarchyTree, {
      props: {
        modelValue: '',
        returnType: 'path',
      },
    });

    // Find the 'apps' dataset row
    const rows = wrapper.findAll('.tree-node-row');
    const appsRow = rows.find((r) => r.text().includes('apps'));
    expect(appsRow).toBeDefined();

    await appsRow!.trigger('click');

    expect(wrapper.emitted('update:modelValue')).toBeTruthy();
    expect(wrapper.emitted('update:modelValue')![0]).toEqual(['tank/apps']);
    expect(wrapper.emitted('select')).toBeTruthy();

    wrapper.unmount();
  });

  it('returns mountpoint when returnType="mountpoint"', async () => {
    const wrapper = mount(DatasetHierarchyTree, {
      props: {
        modelValue: '',
        returnType: 'mountpoint',
      },
    });

    const rows = wrapper.findAll('.tree-node-row');
    const mediaRow = rows.find((r) => r.text().includes('media'));
    expect(mediaRow).toBeDefined();

    await mediaRow!.trigger('click');

    expect(wrapper.emitted('update:modelValue')).toBeTruthy();
    expect(wrapper.emitted('update:modelValue')![0]).toEqual(['/nas/tank/media']);

    wrapper.unmount();
  });

  it('performs fuzzy search filtering and highlights matches', async () => {
    const wrapper = mount(DatasetHierarchyTree, {
      props: {
        modelValue: '',
      },
    });

    const searchInput = wrapper.find('input[type="text"]');
    await searchInput.setValue('app');

    // Should match apps and appdata, but not media
    expect(wrapper.text()).toContain('apps');
    expect(wrapper.text()).not.toContain('media');

    // Clear search
    await searchInput.setValue('');
    expect(wrapper.text()).toContain('media');

    wrapper.unmount();
  });

  it('filters by pool when poolFilter prop is set', () => {
    // Add another pool
    nasState.pools.push({
      name: 'zfsPools/fastpool',
      poolName: 'fastpool',
      state: 'ONLINE',
      health: 'HEALTHY',
      mountpoint: '/nas/fastpool',
    } as unknown as ZfsPool);
    nasState.datasetsByPool['zfsPools/fastpool'] = [
      {
        name: 'zfsPools/fastpool/datasets/vms',
        datasetPath: 'fastpool/vms',
        mountpoint: '/nas/fastpool/vms',
      } as unknown as ZfsDataset,
    ];

    const wrapper = mount(DatasetHierarchyTree, {
      props: {
        poolFilter: 'fastpool',
      },
    });

    expect(wrapper.text()).toContain('fastpool');
    expect(wrapper.text()).toContain('vms');
    expect(wrapper.text()).not.toContain('tank');

    wrapper.unmount();
  });

  it('expands and collapses nodes when toggle buttons are clicked', async () => {
    const wrapper = mount(DatasetHierarchyTree, {
      props: {
        modelValue: '',
      },
    });

    // Both apps and media are visible because autoExpandAll is true
    expect(wrapper.text()).toContain('apps');
    expect(wrapper.text()).toContain('media');

    // Click collapse all
    const collapseBtn = wrapper.find('button[title="Collapse All Folders"]');
    await collapseBtn.trigger('click');

    // Now only root pool node is visible in flat visible list
    expect(wrapper.text()).toContain('tank');
    expect(wrapper.text()).not.toContain('apps');

    // Click expand all
    const expandBtn = wrapper.find('button[title="Expand All Folders"]');
    await expandBtn.trigger('click');

    expect(wrapper.text()).toContain('apps');
    expect(wrapper.text()).toContain('media');

    wrapper.unmount();
  });
});
