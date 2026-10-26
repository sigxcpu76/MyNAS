import { describe, expect, it, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { AipClient } from '../src/adapters/http/aipClient';
import { HttpKvmAdapter, HttpLxcAdapter } from '../src/adapters/http/httpAdapters';
import { KvmService, LxcService } from '../src/application/services';
import { services } from '../src/container';
import { nasState, refreshIsos, refreshLxcTemplates } from '../src/ui/store';
import type { StorageImage } from '../src/domain/models';
import KvmView from '../src/ui/views/KvmView.vue';
import LxcView from '../src/ui/views/LxcView.vue';

describe('Storage Repositories: ISOs and LXC Templates', () => {
  const sampleIsos: StorageImage[] = [
    {
      name: 'debian-12.iso',
      path: '/nas/tank/isos/debian-12.iso',
      sizeBytes: 654321000,
      sizeFormatted: '624.0 MiB',
      modTime: '2026-10-26T20:00:00Z',
    },
    {
      name: 'ubuntu-24.04.iso',
      path: '/nas/tank/isos/ubuntu-24.04.iso',
      sizeBytes: 2500000000,
      sizeFormatted: '2.3 GiB',
      modTime: '2026-10-26T21:00:00Z',
    },
  ];

  const sampleTemplates: StorageImage[] = [
    {
      name: 'debian-12.tar.xz',
      path: '/nas/tank/lxc-templates/debian-12.tar.xz',
      sizeBytes: 104857600,
      sizeFormatted: '100.0 MiB',
      modTime: '2026-10-26T19:00:00Z',
    },
    {
      name: 'alpine-3.20.tar.gz',
      path: '/nas/tank/lxc-templates/alpine-3.20.tar.gz',
      sizeBytes: 5242880,
      sizeFormatted: '5.0 MiB',
      modTime: '2026-10-26T19:30:00Z',
    },
  ];

  beforeEach(() => {
    vi.restoreAllMocks();
    nasState.isos = [];
    nasState.lxcTemplates = [];
    nasState.vms = [];
    nasState.lxcContainers = [];
    nasState.loading = false;
    nasState.systemConfig = {
      mainPool: 'tank',
      systemDataset: 'tank/.system',
      systemMountpoint: '/var/lib/nas',
      isInitialized: true,
      enabledModules: ['kvm', 'lxc'],
      configFilePath: '/etc/nas/config.yaml',
      autoDiscover: true,
      discoveredPools: [],
    };
  });

  describe('HttpKvmAdapter & KvmService ISO operations', () => {
    it('lists ISOs from backend', async () => {
      const mockFetch = vi.fn().mockImplementation(async (url: string) => {
        if (url.includes('/v1/storage/isos')) {
          return new Response(JSON.stringify({ isos: sampleIsos }), { status: 200 });
        }
        return new Response(JSON.stringify({}), { status: 200 });
      });

      const client = new AipClient('/v1', mockFetch as any);
      const adapter = new HttpKvmAdapter(client);
      const service = new KvmService(adapter);

      const isos = await service.listIsos();
      expect(isos).toHaveLength(2);
      expect(isos[0].name).toBe('debian-12.iso');
      expect(isos[1].name).toBe('ubuntu-24.04.iso');
    });

    it('uploads an ISO file via multipart/form-data', async () => {
      const uploadedImage: StorageImage = {
        name: 'archlinux.iso',
        path: '/nas/tank/isos/archlinux.iso',
        sizeBytes: 800000000,
        sizeFormatted: '762.9 MiB',
        modTime: '2026-10-26T22:00:00Z',
      };

      const mockFetch = vi.fn().mockImplementation(async (url: string, init?: RequestInit) => {
        if (url.includes('/v1/storage/isos/upload') && init?.method === 'POST') {
          return new Response(JSON.stringify({ iso: uploadedImage }), { status: 200 });
        }
        return new Response(JSON.stringify({}), { status: 200 });
      });

      const client = new AipClient('/v1', mockFetch as any);
      const adapter = new HttpKvmAdapter(client);
      const service = new KvmService(adapter);

      const dummyFile = new File(['mock content'], 'archlinux.iso', { type: 'application/x-iso9660-image' });
      const result = await service.uploadIso(dummyFile);
      expect(result.name).toBe('archlinux.iso');
      expect(result.sizeFormatted).toBe('762.9 MiB');
    });

    it('deletes an ISO file', async () => {
      let deletedName = '';
      const mockFetch = vi.fn().mockImplementation(async (url: string, init?: RequestInit) => {
        if (url.includes('/v1/storage/isos') && init?.method === 'DELETE') {
          const parsed = new URL(url, 'http://localhost');
          deletedName = parsed.searchParams.get('filename') || '';
          return new Response(JSON.stringify({ success: true }), { status: 200 });
        }
        return new Response(JSON.stringify({}), { status: 200 });
      });

      const client = new AipClient('/v1', mockFetch as any);
      const adapter = new HttpKvmAdapter(client);
      const service = new KvmService(adapter);

      await service.deleteIso('debian-12.iso');
      expect(deletedName).toBe('debian-12.iso');
    });
  });

  describe('HttpLxcAdapter & LxcService Template operations', () => {
    it('lists LXC templates from backend', async () => {
      const mockFetch = vi.fn().mockImplementation(async (url: string) => {
        if (url.includes('/v1/storage/lxc-templates')) {
          return new Response(JSON.stringify({ templates: sampleTemplates }), { status: 200 });
        }
        return new Response(JSON.stringify({}), { status: 200 });
      });

      const client = new AipClient('/v1', mockFetch as any);
      const adapter = new HttpLxcAdapter(client);
      const service = new LxcService(adapter);

      const templates = await service.listTemplates();
      expect(templates).toHaveLength(2);
      expect(templates[0].name).toBe('debian-12.tar.xz');
      expect(templates[1].name).toBe('alpine-3.20.tar.gz');
    });

    it('uploads an LXC template archive', async () => {
      const uploadedTmpl: StorageImage = {
        name: 'ubuntu-noble.tar.xz',
        path: '/nas/tank/lxc-templates/ubuntu-noble.tar.xz',
        sizeBytes: 150000000,
        sizeFormatted: '143.0 MiB',
        modTime: '2026-10-26T22:30:00Z',
      };

      const mockFetch = vi.fn().mockImplementation(async (url: string, init?: RequestInit) => {
        if (url.includes('/v1/storage/lxc-templates/upload') && init?.method === 'POST') {
          return new Response(JSON.stringify({ template: uploadedTmpl }), { status: 200 });
        }
        return new Response(JSON.stringify({}), { status: 200 });
      });

      const client = new AipClient('/v1', mockFetch as any);
      const adapter = new HttpLxcAdapter(client);
      const service = new LxcService(adapter);

      const dummyFile = new File(['fake archive'], 'ubuntu-noble.tar.xz', { type: 'application/x-xz' });
      const result = await service.uploadTemplate(dummyFile);
      expect(result.name).toBe('ubuntu-noble.tar.xz');
      expect(result.path).toBe('/nas/tank/lxc-templates/ubuntu-noble.tar.xz');
    });

    it('deletes an LXC template archive', async () => {
      let deletedName = '';
      const mockFetch = vi.fn().mockImplementation(async (url: string, init?: RequestInit) => {
        if (url.includes('/v1/storage/lxc-templates') && init?.method === 'DELETE') {
          const parsed = new URL(url, 'http://localhost');
          deletedName = parsed.searchParams.get('filename') || '';
          return new Response(JSON.stringify({ success: true }), { status: 200 });
        }
        return new Response(JSON.stringify({}), { status: 200 });
      });

      const client = new AipClient('/v1', mockFetch as any);
      const adapter = new HttpLxcAdapter(client);
      const service = new LxcService(adapter);

      await service.deleteTemplate('debian-12.tar.xz');
      expect(deletedName).toBe('debian-12.tar.xz');
    });
  });

  describe('UI Component Integration', () => {
    it('renders ISOs Repository section in KvmView and shows loaded images', async () => {
      vi.spyOn(services.kvm, 'listVirtualMachines').mockResolvedValue([]);
      vi.spyOn(services.kvm, 'listIsos').mockResolvedValue(sampleIsos);
      nasState.isos = [...sampleIsos];

      const wrapper = mount(KvmView, {
        global: {
          stubs: {
            NoVncModal: true,
          },
        },
      });

      expect(wrapper.text()).toContain('ISOs Repository');
      expect(wrapper.text()).toContain('debian-12.iso');
      expect(wrapper.text()).toContain('ubuntu-24.04.iso');
      expect(wrapper.text()).toContain('624.0 MiB');
      expect(wrapper.text()).toContain('2.3 GiB');
    });

    it('renders LXC Templates Repository section in LxcView and shows loaded templates', async () => {
      vi.spyOn(services.lxc, 'listContainers').mockResolvedValue([]);
      vi.spyOn(services.lxc, 'listTemplates').mockResolvedValue(sampleTemplates);
      nasState.lxcTemplates = [...sampleTemplates];

      const wrapper = mount(LxcView);

      expect(wrapper.text()).toContain('LXC Templates Repository');
      expect(wrapper.text()).toContain('debian-12.tar.xz');
      expect(wrapper.text()).toContain('alpine-3.20.tar.gz');
      expect(wrapper.text()).toContain('100.0 MiB');
      expect(wrapper.text()).toContain('5.0 MiB');
    });
  });
});
