import { describe, expect, it } from 'vitest';
import {
  getDockerAppIcon,
  getDockerContainerIcon,
  getLxcContainerIcon,
  LXC_DISTRO_ICONS,
} from '../src/ui/utils/appIcons';
import type { CatalogApp, DockerApp, LxcContainer } from '../src/domain/models';

describe('appIcons utility', () => {
  const mockCatalog = [
    {
      appId: 'nextcloud',
      title: 'Nextcloud Hub',
      category: 'Cloud',
      iconUrl: 'https://example.com/icons/nextcloud.png',
      description: 'Self-hosted cloud',
      train: 'community',
    },
    {
      appId: 'plex',
      title: 'Plex Media Server',
      category: 'Media',
      iconUrl: 'https://example.com/icons/plex.png',
      description: 'Media server',
      train: 'community',
    },
  ] as unknown as CatalogApp[];

  describe('getDockerContainerIcon', () => {
    it('returns explicit iconUrl if defined on container', () => {
      const icon = getDockerContainerIcon({
        name: 'custom-container',
        iconUrl: 'https://custom.site/icon.png',
      });
      expect(icon).toBe('https://custom.site/icon.png');
    });

    it('matches catalog apps by serviceName or stripped container name', () => {
      const fromService = getDockerContainerIcon(
        { serviceName: 'nextcloud', image: 'custom/image:v1' },
        mockCatalog
      );
      expect(fromService).toBe('https://example.com/icons/nextcloud.png');

      const fromContainerName = getDockerContainerIcon(
        { name: 'my-stack_plex_1', image: 'custom/image:v1' },
        mockCatalog
      );
      expect(fromContainerName).toBe('https://example.com/icons/plex.png');
    });

    it('extracts known dashboard icon slugs from image path and tags', () => {
      const qbit = getDockerContainerIcon({
        name: 'qbittorrent-1',
        image: 'lscr.io/linuxserver/qbittorrent:4.6.3',
      });
      expect(qbit).toBe('https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/qbittorrent.png');

      const pg = getDockerContainerIcon({
        name: 'db',
        image: 'postgres:16-alpine',
      });
      expect(pg).toBe('https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/postgres.png');

      const redis = getDockerContainerIcon({
        name: 'cache',
        image: 'redis:alpine',
      });
      expect(redis).toBe('https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/redis.png');
    });

    it('extracts meaningful identity when image last part is generic (e.g. server, app)', () => {
      const vw = getDockerContainerIcon({
        name: 'vaultwarden',
        image: 'vaultwarden/server:latest',
      });
      expect(vw).toBe('https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/vaultwarden.png');
    });

    it('falls back to serviceName when image is unspecified or clean', () => {
      const icon = getDockerContainerIcon({
        serviceName: 'mariadb',
      });
      expect(icon).toBe('https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/mariadb.png');
    });
  });

  describe('getDockerAppIcon', () => {
    it('returns explicit app iconUrl', () => {
      const app = {
        name: 'dockerApps/test',
        appId: 'test',
        title: 'Test App',
        iconUrl: 'https://test.com/app.png',
      } as DockerApp;
      expect(getDockerAppIcon(app)).toBe('https://test.com/app.png');
    });

    it('matches catalog iconUrl by appId or title', () => {
      const app = {
        name: 'dockerApps/nextcloud',
        appId: 'nextcloud',
        title: 'Nextcloud Hub',
      } as DockerApp;
      expect(getDockerAppIcon(app, mockCatalog)).toBe('https://example.com/icons/nextcloud.png');
    });

    it('falls back to container image icon when app ID is generic', () => {
      const app = {
        name: 'dockerApps/custom-media',
        appId: '',
        title: '',
        containers: [
          {
            containerId: 'c1',
            name: 'media_plex_1',
            serviceName: 'plex',
            image: 'plexinc/pms-docker:latest',
            state: 'RUNNING',
            status: 'Up',
            cpuPercent: 0,
            memoryMib: 128,
            ports: [],
            createdAt: '',
          },
        ],
      } as unknown as DockerApp;
      expect(getDockerAppIcon(app, mockCatalog)).toBe('https://example.com/icons/plex.png');
    });
  });

  describe('getLxcContainerIcon', () => {
    it('returns distro icon for known distributions', () => {
      const lx = {
        name: 'lxcContainers/101',
        containerName: 'ct-debian',
        distribution: 'debian',
      } as LxcContainer;
      expect(getLxcContainerIcon(lx)).toBe(LXC_DISTRO_ICONS.debian);
    });

    it('returns service icon if container name matches a known service', () => {
      const lx = {
        name: 'lxcContainers/102',
        containerName: 'pihole',
        distribution: 'debian',
      } as LxcContainer;
      expect(getLxcContainerIcon(lx)).toBe('https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/pihole.png');
    });
  });
});
