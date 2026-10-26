import { describe, it, expect, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import SkeletonBox from '../src/ui/components/SkeletonBox.vue';
import OverviewSkeleton from '../src/ui/components/OverviewSkeleton.vue';
import OverviewView from '../src/ui/views/OverviewView.vue';
import DisksView from '../src/ui/views/DisksView.vue';
import { nasState } from '../src/ui/store';

describe('Moving Placeholder Skeletons', () => {
  describe('SkeletonBox primitive', () => {
    it('renders with default shimmer class and custom dimensions', () => {
      const wrapper = mount(SkeletonBox, {
        props: {
          width: '120px',
          height: '24px',
        },
      });

      const el = wrapper.find('.skeleton-box');
      expect(el.exists()).toBe(true);
      expect(el.classes()).not.toContain('no-animation');
      expect(el.attributes('style')).toContain('width: 120px');
      expect(el.attributes('style')).toContain('height: 24px');
    });

    it('renders as a circle when circle=true', () => {
      const wrapper = mount(SkeletonBox, {
        props: {
          height: '40px',
          circle: true,
        },
      });

      const el = wrapper.find('.skeleton-box');
      expect(el.attributes('style')).toContain('border-radius: 50%');
      expect(el.attributes('style')).toContain('width: 40px');
      expect(el.attributes('style')).toContain('height: 40px');
    });

    it('renders multiple lines when count > 1', () => {
      const wrapper = mount(SkeletonBox, {
        props: {
          count: 3,
          height: '14px',
        },
      });

      const items = wrapper.findAll('.skeleton-box');
      expect(items.length).toBe(3);
      // Last item should have a shorter width (75%) for realistic text appearance
      expect(items[2].attributes('style')).toContain('width: 75%');
    });

    it('disables animation when animate=false', () => {
      const wrapper = mount(SkeletonBox, {
        props: {
          animate: false,
        },
      });

      const el = wrapper.find('.skeleton-box');
      expect(el.classes()).toContain('no-animation');
    });
  });

  describe('OverviewSkeleton compound screen', () => {
    it('renders full-fidelity dashboard placeholders with fastfetch, summary cards, pools, and traffic', () => {
      const wrapper = mount(OverviewSkeleton);

      expect(wrapper.find('.fastfetch-skeleton-card').exists()).toBe(true);
      expect(wrapper.find('.summary-cards-grid').exists()).toBe(true);
      expect(wrapper.find('.pool-cards-skeleton-grid').exists()).toBe(true);
      expect(wrapper.findAll('.pool-skeleton-card').length).toBe(2);
      expect(wrapper.find('.system-iface-skeleton-header').exists()).toBe(true);
      expect(wrapper.find('.traffic-chart-skeleton-box').exists()).toBe(true);
      expect(wrapper.find('.telemetry-details-skeleton-grid').exists()).toBe(true);

      const skeletonElements = wrapper.findAllComponents(SkeletonBox);
      expect(skeletonElements.length).toBeGreaterThan(20);
    });
  });

  describe('Data replacement in Views', () => {
    beforeEach(() => {
      nasState.loading = false;
      nasState.initialLoaded = false;
      nasState.systemTelemetry = null;
      nasState.pools = [];
      nasState.disks = [];
    });

    it('OverviewView displays OverviewSkeleton when telemetry is absent and loading', async () => {
      nasState.loading = true;
      nasState.initialLoaded = false;
      nasState.systemTelemetry = null;

      const wrapper = mount(OverviewView);
      expect(wrapper.findComponent(OverviewSkeleton).exists()).toBe(true);

      // Now simulate data arrival
      nasState.systemTelemetry = {
        host: {
          hostname: 'nan-nas',
          osName: 'Linux',
          osVersion: '6.6.0',
          kernelVersion: '6.6.0-arch',
          architecture: 'x86_64',
          uptimeSeconds: 3600,
          motherboardVendor: 'ASUS',
          motherboardName: 'Pro WS',
          motherboardVersion: 'Rev 1.0',
          biosVendor: 'American Megatrends',
          biosVersion: '1401',
          biosDate: '2024-01-01',
          gpus: ['NVIDIA RTX 4090'],
        },
        cpu: {
          model: 'AMD Ryzen 9 7950X',
          usagePercent: 12.5,
          temperatureCelsius: 45,
          physicalCores: 8,
          logicalCores: 16,
          load1: 1.2,
          load5: 1.0,
          load15: 0.8,
          coreTemperatures: [42, 45, 43, 44],
        },
        memory: {
          totalBytes: 32000000000,
          freeBytes: 15000000000,
          availableBytes: 20000000000,
          usedBytes: 12000000000,
          zfsArcBytes: 5000000000,
          zfsArcMaxBytes: 16000000000,
          buffersBytes: 1000000000,
          cachedBytes: 4000000000,
          servicesBytes: 7000000000,
        },
        network: [],
      };
      nasState.initialLoaded = true;
      nasState.loading = false;

      await wrapper.vm.$nextTick();

      // Placeholder is replaced by real content
      expect(wrapper.findComponent(OverviewSkeleton).exists()).toBe(false);
      expect(wrapper.find('.overview-dashboard').exists()).toBe(true);
    });

    it('DisksView displays skeleton rows when loading with empty disks, then replaces with actual rows', async () => {
      nasState.loading = true;
      nasState.disks = [];

      const wrapper = mount(DisksView);
      // Skeletons are rendered in the tbody
      const skeletons = wrapper.findAllComponents(SkeletonBox);
      expect(skeletons.length).toBeGreaterThan(0);

      // Simulate disks received
      nasState.loading = false;
      nasState.disks = [
        {
          name: 'disks/nvme0n1',
          diskId: 'nvme0n1',
          devicePath: '/dev/nvme0n1',
          model: 'Samsung SSD 980 PRO 1TB',
          vendor: 'Samsung',
          type: 'NVME',
          sizeBytes: 1000204886016,
          rotational: false,
          temperatureCelsius: 38,
          smartStatus: 'PASSED',
          owningZfsPool: 'zfsPools/tank',
          partitions: [
            {
              name: 'nvme0n1p1',
              devicePath: '/dev/nvme0n1p1',
              sizeBytes: 536870912,
              fstype: 'vfat',
              mountpoint: '/boot',
              label: 'EFI',
            },
          ],
        } as any,
      ];

      await wrapper.vm.$nextTick();

      // Placeholder rows replaced by the disk row
      expect(wrapper.text()).toContain('Samsung SSD 980 PRO 1TB');
    });
  });
});
