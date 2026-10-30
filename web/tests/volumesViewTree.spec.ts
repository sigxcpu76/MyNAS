import { describe, expect, it, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import VolumesView from '../src/ui/views/VolumesView.vue';
import { nasState } from '../src/ui/store';
import type { ZfsPool, ZfsVolume } from '../src/domain/models';

describe('VolumesView Tree Collapsed by Default', () => {
  beforeEach(() => {
    nasState.pools = [
      {
        name: 'zfsPools/tank',
        poolName: 'tank',
        state: 'ONLINE',
        allocatedBytes: 20 * 1024 * 1024 * 1024,
        freeBytes: 80 * 1024 * 1024 * 1024,
        sizeBytes: 100 * 1024 * 1024 * 1024,
        compression: 'lz4',
        mountpoint: '/mnt/tank',
      } as unknown as ZfsPool,
    ];

    nasState.volumesByPool['zfsPools/tank'] = [
      {
        name: 'zfsPools/tank/volumes/vm-disk-1',
        volumePath: 'tank/vm-disk-1',
        devicePath: '/dev/zvol/tank/vm-disk-1',
        sizeBytes: 10 * 1024 * 1024 * 1024,
        usedBytes: 2 * 1024 * 1024 * 1024,
        volblocksize: '16k',
        compression: 'lz4',
        thinProvisioned: true,
      } as unknown as ZfsVolume,
      {
        name: 'zfsPools/tank/volumes/vm-disk-2',
        volumePath: 'tank/vm-disk-2',
        devicePath: '/dev/zvol/tank/vm-disk-2',
        sizeBytes: 20 * 1024 * 1024 * 1024,
        usedBytes: 5 * 1024 * 1024 * 1024,
        volblocksize: '16k',
        compression: 'zstd',
        thinProvisioned: false,
      } as unknown as ZfsVolume,
    ];
  });

  it('starts with tree collapsed: only pool roots are visible', () => {
    const wrapper = mount(VolumesView);

    const text = wrapper.text();
    // Pool root should be visible
    expect(text).toContain('tank');
    expect(text).toContain('zpool');
    expect(text).toContain('2 volumes');

    // Child volumes should be collapsed
    expect(text).not.toContain('vm-disk-1');
    expect(text).not.toContain('vm-disk-2');

    // Chevron on pool root should indicate it is expandable
    const chevronBtn = wrapper.find('.chevron-btn');
    expect(chevronBtn.exists()).toBe(true);
    expect(chevronBtn.attributes('title')).toBe('Expand');
  });

  it('toggles expansion of a pool root when clicking its chevron', async () => {
    const wrapper = mount(VolumesView);

    // Expand tank root
    const rootChevron = wrapper.find('.chevron-btn');
    await rootChevron.trigger('click');

    let text = wrapper.text();
    expect(text).toContain('tank');
    expect(text).toContain('vm-disk-1');
    expect(text).toContain('vm-disk-2');

    // Click root chevron again to collapse
    await rootChevron.trigger('click');
    text = wrapper.text();
    expect(text).toContain('tank');
    expect(text).not.toContain('vm-disk-1');
    expect(text).not.toContain('vm-disk-2');
  });

  it('expands all and collapses all using tree control buttons', async () => {
    const wrapper = mount(VolumesView);

    // Initially collapsed
    expect(wrapper.text()).not.toContain('vm-disk-1');

    // Click Expand All
    const expandAllBtn = wrapper.find('button[title="Expand All"]');
    expect(expandAllBtn.exists()).toBe(true);
    await expandAllBtn.trigger('click');

    expect(wrapper.text()).toContain('tank');
    expect(wrapper.text()).toContain('vm-disk-1');
    expect(wrapper.text()).toContain('vm-disk-2');

    // Click Collapse All
    const collapseAllBtn = wrapper.find('button[title="Collapse All"]');
    expect(collapseAllBtn.exists()).toBe(true);
    await collapseAllBtn.trigger('click');

    expect(wrapper.text()).toContain('tank');
    expect(wrapper.text()).not.toContain('vm-disk-1');
    expect(wrapper.text()).not.toContain('vm-disk-2');
  });

  it('displays all volumes when switching to Flat mode', async () => {
    const wrapper = mount(VolumesView);

    // Switch to Flat mode
    const flatBtn = wrapper.findAll('.toggle-btn').find((btn) => btn.text().includes('Flat'));
    expect(flatBtn).toBeDefined();
    await flatBtn!.trigger('click');

    const text = wrapper.text();
    expect(text).toContain('vm-disk-1');
    expect(text).toContain('vm-disk-2');
  });

  it('filters volumes when searching', async () => {
    const wrapper = mount(VolumesView);

    // Type in search query
    const searchInput = wrapper.find('.search-input');
    await searchInput.setValue('disk-2');

    const text = wrapper.text();
    expect(text).not.toContain('vm-disk-1');
    expect(text).toContain('vm-disk-2');
  });
});
