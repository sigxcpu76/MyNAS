import { describe, expect, it, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import DatasetsView from '../src/ui/views/DatasetsView.vue';
import { nasState } from '../src/ui/store';
import type { ZfsPool, ZfsDataset } from '../src/domain/models';

describe('DatasetsView Tree Collapsed by Default', () => {
  beforeEach(() => {
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
        mountpoint: '/mnt/tank',
      } as unknown as ZfsPool,
    ];

    nasState.datasetsByPool['zfsPools/tank'] = [
      {
        name: 'zfsPools/tank/datasets/media',
        datasetPath: 'tank/media',
        mountpoint: '/mnt/tank/media',
        usedBytes: 1024 * 1024 * 1024,
        availableBytes: 10 * 1024 * 1024 * 1024,
        compression: 'lz4',
      } as unknown as ZfsDataset,
      {
        name: 'zfsPools/tank/datasets/media/movies',
        datasetPath: 'tank/media/movies',
        mountpoint: '/mnt/tank/media/movies',
        usedBytes: 500 * 1024 * 1024,
        availableBytes: 10 * 1024 * 1024 * 1024,
        compression: 'lz4',
      } as unknown as ZfsDataset,
    ];
  });

  it('starts with tree collapsed: only pool roots are visible', () => {
    const wrapper = mount(DatasetsView, {
      global: {
        stubs: {
          DatasetHierarchyTree: true,
          CodeMirrorEditor: true,
          PathSelector: true,
        },
      },
    });

    const text = wrapper.text();
    // Pool root should be visible
    expect(text).toContain('tank');
    // Child and grandchild datasets should be collapsed
    expect(text).not.toContain('media');
    expect(text).not.toContain('movies');

    // Chevron on pool root should indicate it is expandable
    const chevronBtn = wrapper.find('.chevron-btn');
    expect(chevronBtn.exists()).toBe(true);
    expect(chevronBtn.attributes('title')).toBe('Expand');
  });

  it('toggles expansion of a node when clicking its chevron', async () => {
    const wrapper = mount(DatasetsView, {
      global: {
        stubs: {
          DatasetHierarchyTree: true,
          CodeMirrorEditor: true,
          PathSelector: true,
        },
      },
    });

    // Expand tank root
    const rootChevron = wrapper.find('.chevron-btn');
    await rootChevron.trigger('click');

    let text = wrapper.text();
    expect(text).toContain('tank');
    expect(text).toContain('media');
    // movies is a child of media, which is still collapsed
    expect(text).not.toContain('movies');

    // Click root chevron again to collapse
    await rootChevron.trigger('click');
    text = wrapper.text();
    expect(text).toContain('tank');
    expect(text).not.toContain('media');
  });

  it('expands all and collapses all using tree control buttons', async () => {
    const wrapper = mount(DatasetsView, {
      global: {
        stubs: {
          DatasetHierarchyTree: true,
          CodeMirrorEditor: true,
          PathSelector: true,
        },
      },
    });

    // Initially collapsed
    expect(wrapper.text()).not.toContain('movies');

    // Click Expand All
    const expandAllBtn = wrapper.find('button[title="Expand All"]');
    expect(expandAllBtn.exists()).toBe(true);
    await expandAllBtn.trigger('click');

    expect(wrapper.text()).toContain('tank');
    expect(wrapper.text()).toContain('media');
    expect(wrapper.text()).toContain('movies');

    // Click Collapse All
    const collapseAllBtn = wrapper.find('button[title="Collapse All"]');
    expect(collapseAllBtn.exists()).toBe(true);
    await collapseAllBtn.trigger('click');

    expect(wrapper.text()).toContain('tank');
    expect(wrapper.text()).not.toContain('media');
    expect(wrapper.text()).not.toContain('movies');
  });

  it('displays all datasets when switching to Flat mode', async () => {
    const wrapper = mount(DatasetsView, {
      global: {
        stubs: {
          DatasetHierarchyTree: true,
          CodeMirrorEditor: true,
          PathSelector: true,
        },
      },
    });

    // Switch to Flat mode
    const flatBtn = wrapper.findAll('.toggle-btn').find((btn) => btn.text().includes('Flat'));
    expect(flatBtn).toBeDefined();
    await flatBtn!.trigger('click');

    const text = wrapper.text();
    expect(text).toContain('tank');
    expect(text).toContain('media');
    expect(text).toContain('movies');
  });
});
