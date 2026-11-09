import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { nextTick } from 'vue';
import { mapLxcContainer, mapLxcCatalogItem, HttpLxcAdapter } from '../src/adapters/http/httpAdapters';
import type { AipClient } from '../src/adapters/http/aipClient';
import type { LxcCatalogItem } from '../src/domain/models';
import LxcView from '../src/ui/views/LxcView.vue';
import LxcDetailView from '../src/ui/views/LxcDetailView.vue';
import ActionRibbon from '../src/ui/components/ActionRibbon.vue';
import { nasState } from '../src/ui/store';
import { services } from '../src/container';

describe('LXC Feature Parity with VMs and Apps', () => {
  describe('mapLxcContainer', () => {
    it('maps all extended fields: mount points, GPU passthrough, network bridge, and description', () => {
      const raw = {
        name: 'lxcContainers/ct-test',
        container_name: 'Test Container',
        state: 'RUNNING',
        distribution: 'debian',
        release: 'bookworm',
        architecture: 'amd64',
        ipv4_address: '10.10.20.15',
        autostart: true,
        unprivileged: true,
        rootfs_path: '/mnt/tank/containers/ct-test/rootfs',
        memory_limit_mib: 2048,
        cpu_cores: 4,
        template_archive: '/mnt/tank/lxc-templates/debian-12.tar.xz',
        mount_points: [
          {
            host_path: '/mnt/tank/media',
            container_path: '/media',
            read_only: true,
          },
        ],
        gpu_enabled: true,
        description: 'Test description',
        network_bridge: 'br0',
        cpu_percent: 2.5,
        memory_used_mib: 512,
      };

      const mapped = mapLxcContainer(raw);
      expect(mapped.name).toBe('lxcContainers/ct-test');
      expect(mapped.containerName).toBe('Test Container');
      expect(mapped.autostart).toBe(true);
      expect(mapped.unprivileged).toBe(true);
      expect(mapped.gpuEnabled).toBe(true);
      expect(mapped.description).toBe('Test description');
      expect(mapped.networkBridge).toBe('br0');
      expect(mapped.mountPoints).toEqual([
        {
          hostPath: '/mnt/tank/media',
          containerPath: '/media',
          readOnly: true,
        },
      ]);
      expect(mapped.cpuPercent).toBe(2.5);
      expect(mapped.memoryUsedMib).toBe(512);
    });
  });

  describe('HttpLxcAdapter', () => {
    it('sends correct PATCH payload when updating an LXC container', async () => {
      const mockClient = {
        patch: vi.fn().mockResolvedValue({
          name: 'lxcContainers/ct-test',
          containerName: 'Updated Name',
          autostart: false,
          cpuCores: 8,
          memoryLimitMib: 4096,
        }),
      } as unknown as AipClient;

      const adapter = new HttpLxcAdapter(mockClient);
      const res = await adapter.updateContainer(
        'lxcContainers/ct-test',
        {
          containerName: 'Updated Name',
          autostart: false,
          cpuCores: 8,
          memoryLimitMib: 4096,
        },
        ['container_name', 'autostart', 'cpu_cores', 'memory_limit_mib']
      );

      expect(mockClient.patch).toHaveBeenCalledWith(
        'lxcContainers/ct-test',
        expect.objectContaining({
          name: 'lxcContainers/ct-test',
          containerName: 'Updated Name',
          autostart: false,
          cpuCores: 8,
          memoryLimitMib: 4096,
        }),
        ['container_name', 'autostart', 'cpu_cores', 'memory_limit_mib']
      );
      expect(res.containerName).toBe('Updated Name');
    });

    it('invokes restart custom method on container', async () => {
      const mockClient = {
        customMethod: vi.fn().mockResolvedValue({
          name: 'lxcContainers/ct-test',
          state: 'RUNNING',
        }),
      } as unknown as AipClient;

      const adapter = new HttpLxcAdapter(mockClient);
      const res = await adapter.restartContainer('lxcContainers/ct-test');
      expect(mockClient.customMethod).toHaveBeenCalledWith('lxcContainers/ct-test', 'restart', {});
      expect(res.state).toBe('RUNNING');
    });

    it('fetches container logs with tail parameter', async () => {
      const mockClient = {
        get: vi.fn().mockResolvedValue({
          logs: '[lxc-init] Container ct-test started\n[lxc-network] IP assigned: 10.10.20.15',
        }),
      } as unknown as AipClient;

      const adapter = new HttpLxcAdapter(mockClient);
      const logs = await adapter.getContainerLogs('lxcContainers/ct-test', 50);
      expect(mockClient.get).toHaveBeenCalledWith('lxcContainers/ct-test:logs', { tail: 50 });
      expect(logs).toContain('[lxc-init] Container ct-test started');
    });
  });

  describe('LxcView UI', () => {
    beforeEach(() => {
      nasState.lxcContainers = [
        {
          name: 'lxcContainers/ct-media',
          containerName: 'Media Server',
          state: 'RUNNING',
          distribution: 'debian',
          release: 'bookworm',
          architecture: 'amd64',
          ipv4Address: '10.10.20.20',
          autostart: true,
          unprivileged: true,
          rootfsPath: '/mnt/tank/containers/ct-media/rootfs',
          memoryLimitMib: 2048,
          cpuCores: 4,
          gpuEnabled: true,
          description: 'Plex and media services',
          networkBridge: 'lxcbr0',
          mountPoints: [
            {
              hostPath: '/mnt/tank/media',
              containerPath: '/media',
              readOnly: true,
            },
          ],
        },
      ];
      nasState.lxcTemplates = [];
    });

    it('renders realtime CPU, Memory, and Network columns without redundant State and Autostart columns', async () => {
      vi.spyOn(services.lxc, 'listContainers').mockResolvedValue(nasState.lxcContainers);

      const wrapper = mount(LxcView);
      await nextTick();

      // Check realtime metric column headers
      expect(wrapper.text()).toContain('CPU');
      expect(wrapper.text()).toContain('Memory');
      expect(wrapper.text()).toContain('Network');

      // Verify redundant columns are eliminated from overview table
      const thead = wrapper.find('thead');
      expect(thead.text()).not.toContain('State');
      expect(thead.text()).not.toContain('Autostart');

      // Check values rendered
      expect(wrapper.text()).toContain('Media Server');
      expect(wrapper.text()).toContain('0.0%');
      expect(wrapper.text()).toContain('0 MiB');
      expect(wrapper.text()).toContain('0 B/s ↓ · 0 B/s ↑');
    });

    it('renders quick power toggle in table and navigates to LxcDetailView on row click', async () => {
      vi.spyOn(services.lxc, 'listContainers').mockResolvedValue(nasState.lxcContainers);
      const stopSpy = vi.spyOn(services.lxc, 'stopContainer').mockResolvedValue(nasState.lxcContainers[0]);

      const wrapper = mount(LxcView);
      await nextTick();

      // Quick Stop button in the row
      const stopBtn = wrapper.find('button[title="Stop Container"]');
      expect(stopBtn.exists()).toBe(true);
      await stopBtn.trigger('click');
      expect(stopSpy).toHaveBeenCalledWith('lxcContainers/ct-media');

      // Clicking the row opens the unified detail view
      const row = wrapper.find('tbody tr');
      await row.trigger('click');
      expect(nasState.selectedResourceId).toBe('lxc:ct-media');
    });

    it('renders unified Configuration tab in LxcDetailView with CPU, RAM, GPU, and Bind Mounts in a single card', async () => {
      const updateSpy = vi.spyOn(services.lxc, 'updateContainer').mockResolvedValue(nasState.lxcContainers[0]);
      nasState.selectedResourceId = 'lxc:ct-media';
      const wrapper = mount(LxcDetailView, {
        props: {
          containerName: 'ct-media',
          activeSubTab: 'configuration',
        },
      });
      await nextTick();

      expect(wrapper.findAll('.detail-card').length).toBe(1);
      expect(wrapper.findAll('.lxc-config-zone').length).toBe(2);
      expect(wrapper.text()).toContain('Container Settings & Resources');
      expect(wrapper.text()).toContain('CPU Cores');
      expect(wrapper.text()).toContain('Memory Limit (MiB)');
      expect(wrapper.text()).toContain('GPU Acceleration Passthrough');
      expect(wrapper.text()).toContain('Storage Bind Mounts');
      expect(wrapper.text()).toContain('/mnt/tank/media');
      expect(wrapper.text()).toContain('/media');

      await wrapper.find('form.lxc-config-form').trigger('submit.prevent');
      expect(updateSpy).toHaveBeenCalledWith(
        'lxcContainers/ct-media',
        expect.objectContaining({
          mountPoints: [
            {
              hostPath: '/mnt/tank/media',
              containerPath: '/media',
              readOnly: true,
            },
          ],
        })
      );
    });

    it('renders unified ActionRibbon and Logs tab in LxcDetailView', async () => {
      vi.spyOn(services.lxc, 'getContainerLogs').mockResolvedValue('[lxc-status] Container RUNNING');

      nasState.selectedResourceId = 'lxc:ct-media';
      const ribbon = mount(ActionRibbon);
      expect(ribbon.text()).toContain('Media Server');
      expect(ribbon.text()).toContain('Summary');
      expect(ribbon.text()).toContain('Console');
      expect(ribbon.text()).toContain('Configuration');
      expect(ribbon.text()).toContain('Logs');
      expect(ribbon.find('[aria-label="Freeze"]').exists()).toBe(true);
      expect(ribbon.find('[aria-label="Stop"]').exists()).toBe(true);
      expect(ribbon.find('[aria-label="Reboot"]').exists()).toBe(true);

      const detail = mount(LxcDetailView, {
        props: {
          containerName: 'ct-media',
          activeSubTab: 'logs',
        },
      });
      await nextTick();
      expect(detail.text()).toContain('Container Console Logs');
    });
  });

  describe('LXC Catalog Integration', () => {
    const sampleCatalogItem: LxcCatalogItem = {
      id: 'community-plex',
      title: 'Plex Media Server',
      description: 'Organize and stream your video and music libraries.',
      category: 'Media',
      categories: ['Media'],
      source: 'community-scripts',
      baseDistribution: 'debian',
      baseRelease: 'bookworm',
      architecture: 'amd64',
      defaultCpuCores: 2,
      defaultMemoryLimitMib: 2048,
      defaultPort: 32400,
      website: 'https://www.plex.tv',
      installScriptUrl: 'https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/ct/plex.sh',
      downloadUrl: 'http://images.linuxcontainers.org/images/debian/bookworm/amd64/default/rootfs.tar.xz',
      privilegedRequired: false,
      recommendedMounts: ['/media'],
      gpuRecommended: true,
      iconUrl: 'https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/plex.png',
    };

    describe('mapLxcCatalogItem', () => {
      it('maps snake_case catalog payload to camelCase domain model', () => {
        const raw = {
          id: 'turnkey-lamp',
          title: 'TurnKey LAMP',
          description: 'Linux, Apache, MySQL, PHP web stack appliance',
          category: 'Web',
          categories: ['Web', 'Database'],
          source: 'turnkey',
          base_distribution: 'debian',
          base_release: 'bullseye',
          architecture: 'amd64',
          default_cpu_cores: 1,
          default_memory_limit_mib: 1024,
          default_port: 80,
          website: 'https://www.turnkeylinux.org/lamp',
          download_url: 'https://releases.turnkeylinux.org/debian/turnkey-lamp.tar.gz',
          privileged_required: false,
          gpu_recommended: false,
          icon_url: 'https://www.turnkeylinux.org/files/lamp.png',
        };

        const item = mapLxcCatalogItem(raw);
        expect(item.id).toBe('turnkey-lamp');
        expect(item.title).toBe('TurnKey LAMP');
        expect(item.source).toBe('turnkey');
        expect(item.baseDistribution).toBe('debian');
        expect(item.baseRelease).toBe('bullseye');
        expect(item.architecture).toBe('amd64');
        expect(item.defaultCpuCores).toBe(1);
        expect(item.defaultMemoryLimitMib).toBe(1024);
        expect(item.defaultPort).toBe(80);
        expect(item.downloadUrl).toBe('https://releases.turnkeylinux.org/debian/turnkey-lamp.tar.gz');
        expect(item.gpuRecommended).toBe(false);
      });
    });

    describe('HttpLxcAdapter catalog operations', () => {
      it('calls get with query parameters for listCatalogItems', async () => {
        const mockClient = {
          get: vi.fn().mockResolvedValue({
            items: [
              {
                id: 'community-plex',
                title: 'Plex Media Server',
                source: 'community-scripts',
                category: 'Media',
              },
            ],
            total_size: 1,
            last_sync_time: '2026-10-28T22:00:00Z',
          }),
        } as unknown as AipClient;

        const adapter = new HttpLxcAdapter(mockClient);
        const res = await adapter.listCatalogItems('community-scripts', 'Media', 'plex');
        expect(mockClient.get).toHaveBeenCalledWith('lxcCatalog/items', {
          source: 'community-scripts',
          category: 'Media',
          query: 'plex',
        });
        expect(res.items).toHaveLength(1);
        expect(res.items[0].id).toBe('community-plex');
        expect(res.totalSize).toBe(1);
      });

      it('calls get for getCatalogItem', async () => {
        const mockClient = {
          get: vi.fn().mockResolvedValue({
            id: 'community-plex',
            title: 'Plex Media Server',
            source: 'community-scripts',
          }),
        } as unknown as AipClient;

        const adapter = new HttpLxcAdapter(mockClient);
        const item = await adapter.getCatalogItem('community-plex');
        expect(mockClient.get).toHaveBeenCalledWith('lxcCatalog/items/community-plex');
        expect(item.title).toBe('Plex Media Server');
      });

      it('calls customMethod for syncCatalog', async () => {
        const mockClient = {
          customMethod: vi.fn().mockResolvedValue({
            success: true,
            message: 'Catalog synchronized successfully',
            totalItems: 92,
            lastSyncTime: '2026-10-28T22:00:00Z',
          }),
        } as unknown as AipClient;

        const adapter = new HttpLxcAdapter(mockClient);
        const summary = await adapter.syncCatalog(true);
        expect(mockClient.customMethod).toHaveBeenCalledWith('lxcCatalog', 'sync', { force: true });
        expect(summary.totalItems).toBe(92);
        expect(summary.message).toBe('Catalog synchronized successfully');
      });
    });

    describe('LxcView App Catalog tab', () => {
      it('switches to App Catalog tab, displays catalog items, and prefills deployment modal', async () => {
        vi.spyOn(services.lxc, 'listContainers').mockResolvedValue([]);
        vi.spyOn(services.lxc, 'listTemplates').mockResolvedValue([]);
        vi.spyOn(services.lxc, 'listCatalogItems').mockResolvedValue({
          items: [
            sampleCatalogItem,
            {
              id: 'turnkey-nextcloud',
              title: 'TurnKey Nextcloud',
              description: 'Cloud storage and file sync',
              category: 'Storage',
              categories: ['Storage'],
              source: 'turnkey',
              baseDistribution: 'debian',
              baseRelease: 'bookworm',
              architecture: 'amd64',
              defaultCpuCores: 2,
              defaultMemoryLimitMib: 2048,
              defaultPort: 443,
              website: 'https://nextcloud.com',
              downloadUrl: 'https://releases.turnkeylinux.org/debian/turnkey-nextcloud.tar.gz',
              privilegedRequired: false,
              recommendedMounts: [],
              gpuRecommended: false,
            },
          ],
          totalSize: 2,
          lastSyncTime: '2026-10-28T22:00:00Z',
        });

        const wrapper = mount(LxcView);
        await nextTick();

        // Switch to App Catalog tab
        const catalogTab = wrapper.findAll('button').find(b => b.text().includes('App Catalog'));
        expect(catalogTab).toBeDefined();
        await catalogTab!.trigger('click');
        await nextTick();

        // Verify catalog items rendered
        expect(wrapper.text()).toContain('Plex Media Server');
        expect(wrapper.text()).toContain('TurnKey Nextcloud');
        expect(wrapper.text()).toContain('Community Apps');
        expect(wrapper.text()).toContain('TurnKey Appliances');

        // Test filter pill: click Community Apps
        const communityPill = wrapper.findAll('button').find(b => b.text().includes('Community Apps'));
        expect(communityPill).toBeDefined();
        await communityPill!.trigger('click');
        await nextTick();

        // Filtered view should show Plex and not Nextcloud
        expect(wrapper.text()).toContain('Plex Media Server');
        expect(wrapper.text()).not.toContain('TurnKey Nextcloud');

        // Click Deploy Container on Plex
        const deployBtn = wrapper.find('button[title="Deploy Plex Media Server as LXC container"]');
        expect(deployBtn.exists()).toBe(true);
        await deployBtn.trigger('click');
        await nextTick();

        // Modal should open with Plex specs pre-filled
        expect(wrapper.text()).toContain('Deploying Plex Media Server');
        const nameInput = wrapper.find<HTMLInputElement>('input[placeholder="e.g. Debian Web Server"]');
        expect(nameInput.element.value).toBe('Plex Media Server');

        const numberInputs = wrapper.findAll<HTMLInputElement>('input[type="number"]');
        const coresInput = numberInputs[0];
        expect(coresInput.element.value).toBe('2');

        const memInput = numberInputs[1];
        expect(memInput.element.value).toBe('2048');

        // GPU toggle should be checked for Plex
        const checkboxes = wrapper.findAll<HTMLInputElement>('input[type="checkbox"]');
        const gpuCheckbox = checkboxes.find(c => c.element.parentElement?.textContent?.includes('GPU Acceleration'));
        expect(gpuCheckbox?.element.checked).toBe(true);
      });

      it('populates template archive path with safe slug filename and cleans zfsPools prefix from pool', async () => {
        nasState.selectedPoolName = 'zfsPools/fast';
        vi.spyOn(services.lxc, 'listContainers').mockResolvedValue([]);
        vi.spyOn(services.lxc, 'listTemplates').mockResolvedValue([]);
        vi.spyOn(services.lxc, 'listCatalogItems').mockResolvedValue({
          items: [
            {
              id: 'linuxcontainers-debian-bookworm-amd64',
              title: 'Debian Bookworm',
              description: 'Minimal Debian Bookworm rootfs',
              category: 'Operating System',
              categories: ['Operating System'],
              source: 'linuxcontainers',
              baseDistribution: 'debian',
              baseRelease: 'bookworm',
              architecture: 'amd64',
              defaultCpuCores: 1,
              defaultMemoryLimitMib: 512,
              defaultPort: 22,
              website: 'https://linuxcontainers.org',
              downloadUrl: 'https://images.linuxcontainers.org/images/debian/bookworm/amd64/default/20261028_05:24/rootfs.tar.xz',
              privilegedRequired: false,
              recommendedMounts: [],
              gpuRecommended: false,
            },
          ],
          totalSize: 1,
          lastSyncTime: '2026-10-28T22:00:00Z',
        });

        const wrapper = mount(LxcView);
        await nextTick();

        // Switch to catalog and deploy Debian Bookworm
        const catalogTab = wrapper.findAll('button').find(b => b.text().includes('App Catalog'));
        await catalogTab!.trigger('click');
        await nextTick();

        const deployBtn = wrapper.find('button[title="Deploy Debian Bookworm as LXC container"]');
        expect(deployBtn.exists()).toBe(true);
        await deployBtn.trigger('click');
        await nextTick();

        // Find the PathSelector input for template archive
        const pathInput = wrapper.find<HTMLInputElement>('input[placeholder*="lxc-templates"]');
        expect(pathInput.exists()).toBe(true);
        expect(pathInput.element.value).toBe('/mnt/fast/lxc-templates/debian-bookworm-amd64-rootfs.tar.xz');
        expect(pathInput.element.value).not.toContain('zfsPools');
        expect(pathInput.element.value).not.toBe('/mnt/fast/lxc-templates/rootfs.tar.xz');
      });
    });
  });
});
