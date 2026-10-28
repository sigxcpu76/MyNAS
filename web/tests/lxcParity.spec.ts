import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { nextTick } from 'vue';
import { mapLxcContainer, HttpLxcAdapter } from '../src/adapters/http/httpAdapters';
import type { AipClient } from '../src/adapters/http/aipClient';
import LxcView from '../src/ui/views/LxcView.vue';
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
        {
          lxcContainer: expect.objectContaining({
            name: 'lxcContainers/ct-test',
            containerName: 'Updated Name',
            autostart: false,
            cpuCores: 8,
            memoryLimitMib: 4096,
          }),
        },
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

    it('renders Autostart column and clickable toggle badge', async () => {
      vi.spyOn(services.lxc, 'listContainers').mockResolvedValue(nasState.lxcContainers);
      const updateSpy = vi.spyOn(services.lxc, 'updateContainer').mockResolvedValue({
        ...nasState.lxcContainers[0],
        autostart: false,
      });

      const wrapper = mount(LxcView);
      await nextTick();

      // Find Autostart column header
      expect(wrapper.text()).toContain('Autostart');

      // Find the On Boot badge
      const autostartBtn = wrapper.find('button[title*="autostart"]');
      expect(autostartBtn.exists()).toBe(true);
      expect(autostartBtn.text()).toContain('On Boot');

      // Click to toggle autostart
      await autostartBtn.trigger('click');
      expect(updateSpy).toHaveBeenCalledWith(
        'lxcContainers/ct-media',
        { autostart: false },
        ['autostart']
      );
    });

    it('renders action buttons: restart, terminal, logs, edit, inspect', async () => {
      vi.spyOn(services.lxc, 'listContainers').mockResolvedValue(nasState.lxcContainers);
      const restartSpy = vi.spyOn(services.lxc, 'restartContainer').mockResolvedValue(nasState.lxcContainers[0]);

      const wrapper = mount(LxcView);
      await nextTick();

      // Find restart button and trigger
      const restartBtn = wrapper.find('button[title="Restart Container"]');
      expect(restartBtn.exists()).toBe(true);
      await restartBtn.trigger('click');
      expect(restartSpy).toHaveBeenCalledWith('lxcContainers/ct-media');

      // Check terminal button
      const terminalBtn = wrapper.find('button[title="Open Web Terminal Console"]');
      expect(terminalBtn.exists()).toBe(true);

      // Check logs button
      const logsBtn = wrapper.find('button[title="View Container Logs"]');
      expect(logsBtn.exists()).toBe(true);

      // Check edit button
      const editBtn = wrapper.find('button[title="Edit Hardware & Settings"]');
      expect(editBtn.exists()).toBe(true);

      // Check inspect button
      const inspectBtn = wrapper.find('button[title="Inspect Container Details"]');
      expect(inspectBtn.exists()).toBe(true);
    });

    it('opens Edit Container modal when clicking edit button', async () => {
      vi.spyOn(services.lxc, 'listContainers').mockResolvedValue(nasState.lxcContainers);

      const wrapper = mount(LxcView);
      await nextTick();

      const editBtn = wrapper.find('button[title="Edit Hardware & Settings"]');
      await editBtn.trigger('click');

      expect(wrapper.text()).toContain('Edit Container: Media Server');
      expect(wrapper.text()).toContain('GPU Passthrough (/dev/dri, nvidia)');
      expect(wrapper.text()).toContain('Storage Bind Mounts');
      const hostPathInput = wrapper.find<HTMLInputElement>('input[placeholder*="Host Path"]');
      expect(hostPathInput.element.value).toBe('/mnt/tank/media');
    });

    it('opens Container Inspector modal when clicking inspect button', async () => {
      vi.spyOn(services.lxc, 'listContainers').mockResolvedValue(nasState.lxcContainers);
      vi.spyOn(services.lxc, 'getContainerLogs').mockResolvedValue('[lxc-status] Container RUNNING');

      const wrapper = mount(LxcView);
      await nextTick();

      const inspectBtn = wrapper.find('button[title="Inspect Container Details"]');
      await inspectBtn.trigger('click');

      expect(wrapper.text()).toContain('Overview');
      expect(wrapper.text()).toContain('Mount Points (1)');
      expect(wrapper.text()).toContain('Logs');
      expect(wrapper.text()).toContain('Plex and media services');
      expect(wrapper.text()).toContain('Enabled (/dev/dri, nvidia)');
    });
  });
});
