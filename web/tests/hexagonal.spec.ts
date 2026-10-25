import { describe, expect, it, vi } from 'vitest';
import {
  bytesToGib,
  formatBytes,
  gibToBytes,
  isValidResourceId,
  ResourceNames,
  validateSshPublicKey,
} from '../src/domain/models';
import {
  AuthService,
  CatalogService,
  DiskService,
  DockerAppService,
  DockerService,
  IdentityService,
  KvmService,
  LxcService,
  PrerequisiteService,
  ShareService,
  ZfsService,
} from '../src/application/services';
import { AipClient, AipHttpError, normalizeInt64 } from '../src/adapters/http/aipClient';
import {
  HttpAuthAdapter,
  HttpCatalogAdapter,
  HttpDiskAdapter,
  HttpDockerAdapter,
  HttpDockerAppAdapter,
  HttpIdentityAdapter,
  HttpKvmAdapter,
  HttpLxcAdapter,
  HttpPrerequisiteAdapter,
  HttpShareAdapter,
  HttpZfsAdapter,
} from '../src/adapters/http/httpAdapters';
import { RfbConsoleAdapter } from '../src/adapters/novnc/rfbConsoleAdapter';

// ---------------------------------------------------------------------------
// 1. Domain Model & Google AIP Resource Name Tests
// ---------------------------------------------------------------------------

describe('Domain Layer: ResourceNames, Byte Formatters & Validators', () => {
  it('builds and parses Google AIP hierarchical resource names accurately', () => {
    expect(ResourceNames.prerequisite('zfs')).toBe('prerequisites/zfs');
    expect(ResourceNames.prerequisite('prerequisites/zfs')).toBe('prerequisites/zfs');

    expect(ResourceNames.zfsPool('tank')).toBe('zfsPools/tank');
    expect(ResourceNames.zfsDataset('tank', 'media')).toBe('zfsPools/tank/datasets/media');
    expect(ResourceNames.zfsVolume('zfsPools/tank', 'vm-disk')).toBe(
      'zfsPools/tank/volumes/vm-disk'
    );

    expect(ResourceNames.virtualMachine('debian-vm')).toBe('virtualMachines/debian-vm');
    expect(ResourceNames.dockerContainer('immich')).toBe('dockerContainers/immich');
    expect(ResourceNames.dockerApp('nextcloud')).toBe('dockerApps/nextcloud');
    expect(ResourceNames.lxcContainer('alpine-dns')).toBe('lxcContainers/alpine-dns');
    expect(ResourceNames.cifsShare('media')).toBe('cifsShares/media');
    expect(ResourceNames.webdavShare('docs')).toBe('webdavShares/docs');
    expect(ResourceNames.user('admin')).toBe('users/admin');
    expect(ResourceNames.group('nasusers')).toBe('groups/nasusers');

    expect(ResourceNames.extractLeafId('zfsPools/tank/datasets/photos')).toBe('photos');

    const parsed = ResourceNames.parseZfsSubResource('zfsPools/tank/volumes/vm-root');
    expect(parsed).toEqual({
      poolName: 'zfsPools/tank',
      poolId: 'tank',
      subType: 'volumes',
      itemId: 'vm-root',
    });
    expect(ResourceNames.parseZfsSubResource('invalid/path')).toBeNull();
  });

  it('formats byte sizes and converts GiB <-> Bytes', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(1024)).toBe('1 KiB');
    expect(formatBytes(gibToBytes(40))).toBe('40 GiB');
    expect(bytesToGib(gibToBytes(25))).toBe(25);
  });

  it('validates resource IDs and OpenSSH public keys', () => {
    expect(isValidResourceId('tank-01')).toBe(true);
    expect(isValidResourceId('../bad')).toBe(false);
    expect(isValidResourceId('')).toBe(false);

    expect(
      validateSshPublicKey('ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAINasApplianceKey admin@nas')
    ).toBe(true);
    expect(validateSshPublicKey('invalid-key-string')).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 2. Google AIP HTTP Adapter (`AipClient` & `HttpAdapters`) Tests
// ---------------------------------------------------------------------------

describe('Adapters Layer: Google AIP HTTP Client & Port Implementations', () => {
  it('normalizes protojson int64 strings and numbers safely', () => {
    expect(normalizeInt64('1099511627776')).toBe(1099511627776);
    expect(normalizeInt64(4096)).toBe(4096);
    expect(normalizeInt64(null)).toBe(0);
    expect(normalizeInt64(undefined)).toBe(0);
  });

  it('executes AIP standard methods, PATCH with updateMask, and custom :verb methods with Bearer token', async () => {
    const calls: Array<{ url: string; init?: RequestInit }> = [];
    const mockFetch = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
      const urlStr = String(url);
      calls.push({ url: urlStr, init });

      if (urlStr.endsWith('/v1/auth:login')) {
        return new Response(
          JSON.stringify({
            access_token: 'jwt-test-123',
            token_type: 'Bearer',
            expires_in: '3600',
            user: {
              name: 'users/admin',
              username: 'admin',
              uid: 1000,
              is_admin: true,
            },
          }),
          { status: 200 }
        );
      }

      if (urlStr.endsWith('/v1/zfsPools')) {
        return new Response(
          JSON.stringify({
            zfs_pools: [
              {
                name: 'zfsPools/tank',
                pool_name: 'tank',
                state: 'ONLINE',
                size_bytes: '4398046511104',
                allocated_bytes: '1099511627776',
                free_bytes: '3298534883328',
                capacity_percent: 25,
                fragmentation_percent: 3,
                dedup_ratio: 1.12,
                vdev_layout: 'raidz1',
                devices: ['/dev/sda', '/dev/sdb', '/dev/sdc'],
                mountpoint: '/tank',
                compression: 'lz4',
              },
            ],
          }),
          { status: 200 }
        );
      }

      if (urlStr.includes('/v1/zfsPools/tank/datasets/media?updateMask=')) {
        return new Response(
          JSON.stringify({
            name: 'zfsPools/tank/datasets/media',
            dataset_path: 'tank/media',
            mountpoint: '/tank/media',
            quota_bytes: '107374182400',
            compression: 'zstd',
            readonly: true,
          }),
          { status: 200 }
        );
      }

      if (urlStr.endsWith('/v1/zfsPools/tank/datasets/media:snapshot')) {
        return new Response(
          JSON.stringify({
            name: 'zfsPools/tank/datasets/media',
            dataset_path: 'tank/media',
            snapshots_count: 4,
            last_snapshot: 'tank/media@snap-01',
          }),
          { status: 200 }
        );
      }

      if (urlStr.endsWith('/v1/virtualMachines/debian-vm:openConsole')) {
        return new Response(
          JSON.stringify({
            vm_name: 'debian-vm',
            token: 'vnc-tok-99',
            websocket_url: '/ws/novnc/debian-vm?token=vnc-tok-99',
            vnc_port: 5901,
            resolution: '1024x768',
          }),
          { status: 200 }
        );
      }

      return new Response(JSON.stringify({}), { status: 200 });
    });

    const client = new AipClient('/v1', mockFetch as any);
    const authAdapter = new HttpAuthAdapter(client);
    const zfsAdapter = new HttpZfsAdapter(client);
    const kvmAdapter = new HttpKvmAdapter(client);

    // 1. Custom `:login` method
    const session = await authAdapter.login('admin', 'admin123');
    expect(session.accessToken).toBe('jwt-test-123');
    expect(session.expiresIn).toBe(3600);
    authAdapter.setToken(session.accessToken);

    // 2. List ZFS Pools with int64 string coercion
    const pools = await zfsAdapter.listPools();
    expect(pools).toHaveLength(1);
    expect(pools[0].sizeBytes).toBe(4398046511104);
    expect(pools[0].vdevLayout).toBe('raidz1');

    // Verify Authorization Bearer header was injected
    const poolCallHeaders = calls[1].init?.headers as Record<string, string>;
    expect(poolCallHeaders['Authorization']).toBe('Bearer jwt-test-123');

    // 3. PATCH Dataset with `updateMask` query param
    const updatedDs = await zfsAdapter.updateDataset(
      'zfsPools/tank/datasets/media',
      { compression: 'zstd', quotaBytes: 107374182400, readonly: true },
      ['compression', 'quota_bytes', 'readonly']
    );
    expect(updatedDs.compression).toBe('zstd');
    expect(updatedDs.quotaBytes).toBe(107374182400);
    expect(calls[2].url).toBe(
      '/v1/zfsPools/tank/datasets/media?updateMask=compression%2Cquota_bytes%2Creadonly'
    );
    expect(calls[2].init?.method).toBe('PATCH');

    // 4. Custom `:snapshot` method
    const snapDs = await zfsAdapter.snapshotDataset(
      'zfsPools/tank/datasets/media',
      'snap-01'
    );
    expect(snapDs.lastSnapshot).toBe('tank/media@snap-01');
    expect(calls[3].url).toBe('/v1/zfsPools/tank/datasets/media:snapshot');

    // 5. Custom `:openConsole` method on KVM VM
    const consoleSession = await kvmAdapter.openVirtualMachineConsole(
      'virtualMachines/debian-vm'
    );
    expect(consoleSession.token).toBe('vnc-tok-99');
    expect(consoleSession.vncPort).toBe(5901);
  });

  it('supports updating ZFS pool settings and adding/removing VDEVs', async () => {
    const calls: Array<{ url: string; method?: string; body?: any }> = [];
    const mockFetch = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
      const urlStr = String(url);
      const body = init?.body ? JSON.parse(String(init.body)) : undefined;
      calls.push({ url: urlStr, method: init?.method, body });

      if (urlStr.endsWith('/v1/zfsPools/tank') && init?.method === 'PATCH') {
        return new Response(
          JSON.stringify({
            name: 'zfsPools/tank',
            pool_name: 'tank',
            sync: 'always',
            autotrim: 'on',
            compression: 'zstd',
          }),
          { status: 200 }
        );
      }

      if (urlStr.endsWith('/v1/zfsPools/tank:addVdev')) {
        return new Response(
          JSON.stringify({
            name: 'zfsPools/tank',
            pool_name: 'tank',
            vdevs: [
              {
                name: 'cache-1',
                type: 'cache',
                layout: 'stripe',
                state: 'ONLINE',
                children: [{ name: '/dev/nvme0n1p2', type: 'cache', state: 'ONLINE' }],
              },
            ],
          }),
          { status: 200 }
        );
      }

      if (urlStr.endsWith('/v1/zfsPools/tank:removeVdev')) {
        return new Response(
          JSON.stringify({
            name: 'zfsPools/tank',
            pool_name: 'tank',
            vdevs: [],
          }),
          { status: 200 }
        );
      }

      return new Response(JSON.stringify({}), { status: 200 });
    });

    const client = new AipClient('/v1', mockFetch as any);
    const zfsAdapter = new HttpZfsAdapter(client);
    const zfsService = new ZfsService(zfsAdapter);

    // 1. updatePool
    const updated = await zfsService.updatePool('tank', {
      sync: 'always',
      autotrim: 'on',
      compression: 'zstd',
    });
    expect(updated.sync).toBe('always');
    expect(updated.autotrim).toBe('on');
    expect(updated.compression).toBe('zstd');
    expect(calls[0].url).toBe('/v1/zfsPools/tank');
    expect(calls[0].method).toBe('PATCH');

    // 2. addVdev
    const withVdev = await zfsService.addVdev('tank', {
      type: 'cache',
      layout: 'stripe',
      devices: ['/dev/nvme0n1p2'],
    });
    expect(withVdev.vdevs?.[0]?.type).toBe('cache');
    expect(calls[1].url).toBe('/v1/zfsPools/tank:addVdev');
    expect(calls[1].method).toBe('POST');

    // 3. removeVdev
    const removed = await zfsService.removeVdev('tank', '/dev/nvme0n1p2');
    expect(removed.vdevs).toHaveLength(0);
    expect(calls[2].url).toBe('/v1/zfsPools/tank:removeVdev');
    expect(calls[2].method).toBe('POST');
  });

  it('supports editing dataset and zvol sync, size, provisioning, and features', async () => {
    const calls: Array<{ url: string; method?: string; body?: any }> = [];
    const mockFetch = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
      const urlStr = String(url);
      const body = init?.body ? JSON.parse(String(init.body)) : undefined;
      calls.push({ url: urlStr, method: init?.method, body });

      if (urlStr.includes('/v1/zfsPools/tank/datasets/media')) {
        return new Response(
          JSON.stringify({
            name: 'zfsPools/tank/datasets/media',
            dataset_path: 'tank/media',
            mountpoint: '/tank/media',
            sync: 'always',
            reservation_bytes: '53687091200',
            dedup: true,
          }),
          { status: 200 }
        );
      }

      if (urlStr.includes('/v1/zfsPools/tank/volumes/vm-disk')) {
        return new Response(
          JSON.stringify({
            name: 'zfsPools/tank/volumes/vm-disk',
            volume_path: 'tank/vm-disk',
            device_path: '/dev/zvol/tank/vm-disk',
            size_bytes: '107374182400',
            thin_provisioned: false,
            sync: 'always',
            readonly: true,
            dedup: true,
          }),
          { status: 200 }
        );
      }

      return new Response(JSON.stringify({}), { status: 200 });
    });

    const client = new AipClient('/v1', mockFetch as any);
    const zfsAdapter = new HttpZfsAdapter(client);
    const zfsService = new ZfsService(zfsAdapter);

    // 1. Edit Dataset
    const updatedDs = await zfsService.updateDataset(
      'zfsPools/tank/datasets/media',
      {
        sync: 'always',
        reservationBytes: 53687091200,
        dedup: true,
      }
    );
    expect(updatedDs.sync).toBe('always');
    expect(updatedDs.reservationBytes).toBe(53687091200);
    expect(updatedDs.dedup).toBe(true);
    expect(calls[0].url).toContain('updateMask=sync%2Creservation_bytes%2Cdedup');

    // 2. Edit Volume (ZVol)
    const updatedVol = await zfsService.updateVolume(
      'zfsPools/tank/volumes/vm-disk',
      {
        sizeBytes: 107374182400,
        thinProvisioned: false,
        sync: 'always',
        readonly: true,
        dedup: true,
      }
    );
    expect(updatedVol.sync).toBe('always');
    expect(updatedVol.thinProvisioned).toBe(false);
    expect(updatedVol.readonly).toBe(true);
    expect(updatedVol.sizeBytes).toBe(107374182400);
    expect(calls[1].url).toContain('updateMask=size_bytes%2Cthin_provisioned%2Csync%2Creadonly%2Cdedup');
  });

  it('throws AipHttpError on non-2xx responses', async () => {
    const errorFetch = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          error: {
            code: 404,
            message: 'ZFS pool not found',
            status: 'NOT_FOUND',
          },
        }),
        { status: 404 }
      );
    });

    const client = new AipClient('/v1', errorFetch as any);
    await expect(client.get('zfsPools/missing')).rejects.toThrowError(AipHttpError);
  });
});

// ---------------------------------------------------------------------------
// 3. Application Layer Services Unit Tests (with Mock Outbound Ports)
// ---------------------------------------------------------------------------

describe('Application Layer: Use-Case Services with Mock Ports', () => {
  it('validates and orchestrates ZfsService, KvmService, DockerService, LxcService, ShareService, and IdentityService', async () => {
    const client = new AipClient('/v1', vi.fn(async () => new Response('{}', { status: 200 })) as any);
    const zfsService = new ZfsService(new HttpZfsAdapter(client));
    const kvmService = new KvmService(new HttpKvmAdapter(client));
    const dockerService = new DockerService(new HttpDockerAdapter(client));
    const appService = new DockerAppService(new HttpDockerAppAdapter(client));
    const lxcService = new LxcService(new HttpLxcAdapter(client));
    const shareService = new ShareService(new HttpShareAdapter(client));
    const identityService = new IdentityService(new HttpIdentityAdapter(client));
    const prereqService = new PrerequisiteService(new HttpPrerequisiteAdapter(client));
    const authService = new AuthService(new HttpAuthAdapter(client));

    // Validation checks on invalid inputs
    await expect(authService.login('', 'secret')).rejects.toThrow('Username is required');
    await expect(zfsService.createPool('bad/name', { devices: ['/dev/sda'] })).rejects.toThrow(
      'Invalid ZFS pool ID'
    );
    await expect(zfsService.createPool('tank', { devices: [] })).rejects.toThrow(
      'At least one backing device path is required'
    );
    await expect(kvmService.createVirtualMachine('vm1', { vcpus: 0 })).rejects.toThrow(
      'at least 1 vCPU'
    );
    await expect(dockerService.createContainer('app1', { image: '' })).rejects.toThrow(
      'OCI image is required'
    );
    await expect(appService.createApp('nextcloud', '')).rejects.toThrow(
      'compose.yaml) cannot be empty'
    );
    await expect(appService.createApp('bad name', 'services: {}')).rejects.toThrow(
      'Invalid Docker App ID'
    );
    await expect(
      shareService.createCifsShare('share1', { path: 'relative/path' })
    ).rejects.toThrow('must be an absolute path');
    await expect(
      identityService.createUser('alice', {
        sshAuthorizedKeys: ['not-an-ssh-key'],
      })
    ).rejects.toThrow('Invalid SSH public key format');

    // Positive calls
    await prereqService.checkPrerequisites();
    await lxcService.createContainer('alpine1', { distribution: 'alpine' });
  });

  it('orchestrates CatalogService and HttpCatalogAdapter', async () => {
    const fakeCatalogResponse = {
      apps: [
        {
          appId: 'nextcloud',
          title: 'Nextcloud Hub',
          description: 'A safe home for all your data',
          category: 'Productivity',
          categories: ['Productivity', 'Storage'],
          iconUrl: 'https://media.sys.truenas.net/apps/nextcloud/icons/icon.png',
          train: 'stable',
          version: '1.0.0',
          appVersion: '29.0.0',
          tags: ['cloud', 'storage'],
          composeYaml: 'services:\n  nextcloud:\n    image: nextcloud:latest\n',
        },
      ],
      totalSize: 1,
      lastSyncTime: '2026-10-25T00:00:00Z',
    };

    const fetchMock = vi.fn(async (url: string, opts?: any) => {
      if (url.includes('/catalog:sync')) {
        return new Response(
          JSON.stringify({
            totalApps: 472,
            trains: ['stable', 'community'],
            syncTime: '2026-10-25T00:00:00Z',
          }),
          { status: 200 }
        );
      }
      if (url.includes('/catalog/apps/nextcloud')) {
        return new Response(JSON.stringify(fakeCatalogResponse.apps[0]), { status: 200 });
      }
      return new Response(JSON.stringify(fakeCatalogResponse), { status: 200 });
    });

    const client = new AipClient('/v1', fetchMock as any);
    const catalogAdapter = new HttpCatalogAdapter(client);
    const catalogService = new CatalogService(catalogAdapter);

    const listRes = await catalogService.listApps('stable', 'Productivity', 'nextcloud');
    expect(listRes.apps.length).toBe(1);
    expect(listRes.apps[0].appId).toBe('nextcloud');
    expect(listRes.apps[0].title).toBe('Nextcloud Hub');

    const appRes = await catalogService.getApp('nextcloud', 'stable');
    expect(appRes.appId).toBe('nextcloud');
    expect(appRes.composeYaml).toContain('services:');

    const syncRes = await catalogService.syncCatalog();
    expect(syncRes.totalApps).toBe(472);
    expect(syncRes.success).toBe(true);
  });

  it('orchestrates DockerConfig and ListDirectories via HttpDockerAppAdapter', async () => {
    const fetchMock = vi.fn(async (url: string, opts?: any) => {
      if (url.includes('/dockerConfig') && opts?.method === 'PATCH') {
        const body = JSON.parse(opts.body);
        const pool = body.dockerConfig?.pool || 'tank';
        return new Response(
          JSON.stringify({
            pool,
            appdataDataset: `/${pool}/appdata`,
            dockerDataRoot: `/${pool}/docker`,
            availablePools: [pool, 'fastpool'],
            poolSelected: true,
          }),
          { status: 200 }
        );
      }
      if (url.includes('/dockerConfig')) {
        return new Response(
          JSON.stringify({
            pool: 'tank',
            appdataDataset: '/tank/appdata',
            dockerDataRoot: '/tank/docker',
            availablePools: ['tank'],
            poolSelected: false,
          }),
          { status: 200 }
        );
      }
      if (url.includes('/filesystem:directories')) {
        return new Response(
          JSON.stringify({
            currentPath: '/tank',
            parentPath: '/',
            entries: [
              { name: 'appdata', path: '/tank/appdata', isDir: true },
              { name: 'media', path: '/tank/media', isDir: true },
            ],
          }),
          { status: 200 }
        );
      }
      return new Response('{}', { status: 200 });
    });

    const client = new AipClient('/v1', fetchMock as any);
    const adapter = new HttpDockerAppAdapter(client);
    const appService = new DockerAppService(adapter);

    const cfg = await appService.getDockerConfig();
    expect(cfg.pool).toBe('tank');
    expect(cfg.poolSelected).toBe(false);

    const updated = await appService.updateDockerConfig('tank');
    expect(updated.pool).toBe('tank');
    expect(updated.appdataDataset).toBe('/tank/appdata');
    expect(updated.poolSelected).toBe(true);

    const dirs = await appService.listDirectories('/tank');
    expect(dirs.currentPath).toBe('/tank');
    expect(dirs.entries.length).toBe(2);
    expect(dirs.entries[0].name).toBe('appdata');
  });

  it('orchestrates physical disks and SMART telemetry via HttpDiskAdapter', async () => {
    const fakeDisks = [
      {
        name: 'disks/nvme0n1',
        disk_id: 'nvme0n1',
        device_path: '/dev/nvme0n1',
        size_bytes: '2048408248320',
        model: 'Micron 7450 PRO Enterprise NVMe',
        type: 'NVME',
        temperature_celsius: 38.5,
        smart_status: 'PASSED',
        health_percentage: 99,
        owning_zfs_pool: 'tank',
        is_system: true,
        partitions: [
          { name: 'nvme0n1p1', device_path: '/dev/nvme0n1p1', size_bytes: '1073741824', mountpoint: '/boot', is_system: false },
          { name: 'nvme0n1p2', device_path: '/dev/nvme0n1p2', size_bytes: '107374182400', mountpoint: '/', is_system: true },
        ],
      },
    ];

    const fetchMock = vi.fn(async (url: string) => {
      if (url.includes('/disks/nvme0n1')) {
        return new Response(JSON.stringify(fakeDisks[0]), { status: 200 });
      }
      return new Response(JSON.stringify({ disks: fakeDisks }), { status: 200 });
    });

    const client = new AipClient('/v1', fetchMock as any);
    const diskAdapter = new HttpDiskAdapter(client);
    const diskService = new DiskService(diskAdapter);

    const disks = await diskService.listDisks();
    expect(disks.length).toBe(1);
    expect(disks[0].diskId).toBe('nvme0n1');
    expect(disks[0].sizeBytes).toBe(2048408248320);
    expect(disks[0].temperatureCelsius).toBe(38.5);
    expect(disks[0].smartStatus).toBe('PASSED');
    expect(disks[0].owningZfsPool).toBe('tank');
    expect(disks[0].isSystem).toBe(true);
    expect(disks[0].partitions.length).toBe(2);
    expect(disks[0].partitions[1].isSystem).toBe(true);
    expect(disks[0].partitions[1].mountpoint).toBe('/');

    const single = await diskService.getDisk('nvme0n1');
    expect(single.diskId).toBe('nvme0n1');
    expect(single.model).toContain('Micron');
  });

  it('builds WebSocket RFB noVNC URLs properly', () => {
    const wsUrl = RfbConsoleAdapter.buildWebSocketUrl(
      'debian-vm',
      'tok123',
      '/ws/novnc/debian-vm'
    );
    expect(wsUrl).toContain('/ws/novnc/debian-vm?token=tok123');
  });
});
