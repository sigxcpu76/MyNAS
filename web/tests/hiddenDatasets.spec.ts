import { describe, expect, it, beforeEach, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { isDotDataset, type ZfsDataset, type ZfsPool } from '../src/domain/models';
import DatasetHierarchyTree from '../src/ui/components/DatasetHierarchyTree.vue';
import DatasetsView from '../src/ui/views/DatasetsView.vue';
import { nasState } from '../src/ui/store';

describe('Hidden Dot Datasets Filtering', () => {
  describe('isDotDataset helper', () => {
    it('returns true for dataset names or paths starting with a dot', () => {
      expect(isDotDataset('.nas-system')).toBe(true);
      expect(isDotDataset('.system')).toBe(true);
      expect(isDotDataset('.recycle')).toBe(true);
    });

    it('returns true for nested datasets having a segment starting with a dot', () => {
      expect(isDotDataset('tank/.nas-system')).toBe(true);
      expect(isDotDataset('tank/.system/cores')).toBe(true);
      expect(isDotDataset('tank/appdata/.cache')).toBe(true);
      expect(isDotDataset('zfsPools/tank/datasets/.nas-system')).toBe(true);
      expect(isDotDataset('zfsPools/tank/datasets/.system/cores')).toBe(true);
    });

    it('returns true for ZfsDataset objects with dot-prefixed datasetPath, name, id, or mountpoint', () => {
      expect(
        isDotDataset({
          name: 'zfsPools/tank/datasets/.nas-system',
          datasetPath: 'tank/.nas-system',
        } as ZfsDataset)
      ).toBe(true);

      expect(
        isDotDataset({
          name: 'zfsPools/tank/datasets/system/cores',
          datasetPath: 'tank/.system/cores',
        } as ZfsDataset)
      ).toBe(true);

      expect(
        isDotDataset({
          id: '.hidden',
        } as unknown as ZfsDataset)
      ).toBe(true);

      expect(
        isDotDataset({
          mountpoint: '/nas/tank/.nas-system',
        } as ZfsDataset)
      ).toBe(true);
    });

    it('returns false for normal datasets', () => {
      expect(isDotDataset('tank/media')).toBe(false);
      expect(isDotDataset('media')).toBe(false);
      expect(isDotDataset('tank/apps/nextcloud')).toBe(false);
      expect(isDotDataset('zfsPools/tank/datasets/apps')).toBe(false);
      expect(isDotDataset('tank/nextcloud.backup')).toBe(false);
      expect(
        isDotDataset({
          name: 'zfsPools/tank/datasets/media',
          datasetPath: 'tank/media',
          mountpoint: '/nas/tank/media',
        } as ZfsDataset)
      ).toBe(false);
    });

    it('returns false for null, undefined, or empty values', () => {
      expect(isDotDataset(null)).toBe(false);
      expect(isDotDataset(undefined)).toBe(false);
      expect(isDotDataset('')).toBe(false);
    });
  });

  describe('UI components exclude dot datasets', () => {
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
          mountpoint: '/nas/tank',
        } as unknown as ZfsPool,
      ];

      nasState.datasetsByPool['zfsPools/tank'] = [
        {
          name: 'zfsPools/tank/datasets/media',
          datasetPath: 'tank/media',
          mountpoint: '/nas/tank/media',
          usedBytes: 1024,
          availableBytes: 1024,
          compression: 'lz4',
        } as unknown as ZfsDataset,
        {
          name: 'zfsPools/tank/datasets/.nas-system',
          datasetPath: 'tank/.nas-system',
          mountpoint: '/nas/tank/.nas-system',
          usedBytes: 1024,
          availableBytes: 1024,
          compression: 'lz4',
        } as unknown as ZfsDataset,
        {
          name: 'zfsPools/tank/datasets/.system/cores',
          datasetPath: 'tank/.system/cores',
          mountpoint: '/nas/tank/.system/cores',
          usedBytes: 1024,
          availableBytes: 1024,
          compression: 'lz4',
        } as unknown as ZfsDataset,
      ];
    });

    it('DatasetHierarchyTree hides dot-prefixed datasets from tree view', () => {
      const wrapper = mount(DatasetHierarchyTree, {
        props: {
          searchQuery: '',
          selectedPool: 'all',
          selectedDatasetPath: null,
        },
      });

      const text = wrapper.text();
      expect(text).toContain('media');
      expect(text).not.toContain('.nas-system');
      expect(text).not.toContain('.system');
    });

    it('DatasetsView hides dot-prefixed datasets', async () => {
      const wrapper = mount(DatasetsView, {
        global: {
          stubs: {
            DatasetHierarchyTree: true,
            CodeMirrorEditor: true,
            PathSelector: true,
          },
        },
      });

      // Expand all tree nodes so child datasets are visible
      await wrapper.find('button[title="Expand All"]').trigger('click');

      const text = wrapper.text();
      expect(text).toContain('media');
      expect(text).not.toContain('.nas-system');
      expect(text).not.toContain('.system/cores');
    });
  });
});
