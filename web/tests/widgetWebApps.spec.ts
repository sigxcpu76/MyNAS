import { beforeEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import WidgetWebApps from '../src/ui/dashboard/widgets/WidgetWebApps.vue';
import OverviewView from '../src/ui/views/OverviewView.vue';
import { nasState } from '../src/ui/store';
import { WIDGET_CATALOG } from '../src/ui/dashboard/dashboardConfig';
import type { DockerApp, CatalogApp } from '../src/domain/models';

describe('WidgetWebApps - Dashboard Web UI Widget', () => {
  beforeEach(() => {
    nasState.dockerApps = [];
    nasState.catalogApps = [];
    nasState.loading = false;
    vi.restoreAllMocks();
  });

  it('is registered in WIDGET_CATALOG with proper description and workloads category', () => {
    const item = WIDGET_CATALOG.find((w) => w.type === 'webapps');
    expect(item).toBeDefined();
    expect(item?.name).toBe('Web Applications');
    expect(item?.category).toBe('workloads');
    expect(item?.description).toContain('Web UI');
  });

  it('renders empty state when no docker apps expose web UIs', () => {
    nasState.dockerApps = [
      {
        name: 'dockerApps/db-only',
        appId: 'db-only',
        title: 'PostgreSQL DB',
        state: 'RUNNING',
        statusText: 'Up 1 day',
        composeYaml: '',
        composeFilePath: '',
        services: ['db'],
        containers: [],
        portMappings: ['5432:5432/tcp'],
        volumes: [],
        webUiUrl: '',
        totalCpuPercent: 0,
        totalMemoryMib: 0,
        createdAt: '2026-01-01',
        updatedAt: '2026-01-01',
      } as DockerApp,
    ];

    const wrapper = mount(WidgetWebApps);
    expect(wrapper.text()).toContain('Web Applications');
    expect(wrapper.text()).toContain('0/0');
    expect(wrapper.text()).toContain('No installed applications exposing Web UI ports found');
  });

  it('renders application tiles with icons, port badges and tooltips on names', () => {
    nasState.dockerApps = [
      {
        name: 'dockerApps/nextcloud',
        appId: 'nextcloud',
        title: 'Nextcloud Hub',
        state: 'RUNNING',
        statusText: 'Up 2 days',
        composeYaml: '',
        composeFilePath: '',
        services: ['app'],
        containers: [],
        portMappings: ['8080:80/tcp'],
        volumes: [],
        webUiUrl: 'http://localhost:8080',
        iconUrl: 'https://example.com/nextcloud.png',
        totalCpuPercent: 1.5,
        totalMemoryMib: 256,
        createdAt: '2026-01-01',
        updatedAt: '2026-01-01',
      } as DockerApp,
      {
        name: 'dockerApps/jellyfin',
        appId: 'jellyfin',
        title: 'Jellyfin',
        state: 'STOPPED',
        statusText: 'Exited',
        composeYaml: '',
        composeFilePath: '',
        services: ['jellyfin'],
        containers: [],
        portMappings: ['8096:8096/tcp'],
        volumes: [],
        webUiUrl: 'http://localhost:8096',
        iconUrl: 'https://example.com/jellyfin.png',
        totalCpuPercent: 0,
        totalMemoryMib: 0,
        createdAt: '2026-01-01',
        updatedAt: '2026-01-01',
      } as DockerApp,
    ];

    const wrapper = mount(WidgetWebApps);
    expect(wrapper.text()).toContain('Web Applications');
    expect(wrapper.text()).toContain('1/2'); // 1 online of 2 apps

    const tiles = wrapper.findAll('.webapp-tile');
    expect(tiles).toHaveLength(2);

    // First tile should be running app (Nextcloud)
    const nextcloudTile = tiles[0];
    expect(nextcloudTile.text()).toContain('Nextcloud Hub');
    expect(nextcloudTile.text()).toContain(':8080');

    // Tooltip on name and icon
    const nameEl = nextcloudTile.find('.app-name');
    expect(nameEl.attributes('title')).toContain('Nextcloud Hub');
    expect(nameEl.attributes('title')).toContain(':8080');
    expect(nameEl.attributes('title')).toContain('Running');
    expect(nameEl.attributes('title')).toContain('Click to open Web UI');

    // Online status indicator
    expect(nextcloudTile.find('.status-online').exists()).toBe(true);

    // Second tile should be stopped app (Jellyfin)
    const jellyfinTile = tiles[1];
    expect(jellyfinTile.text()).toContain('Jellyfin');
    expect(jellyfinTile.text()).toContain(':8096');
    expect(jellyfinTile.find('.status-offline').exists()).toBe(true);
  });

  it('opens the web UI in a new tab when an app tile is clicked', async () => {
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);

    nasState.dockerApps = [
      {
        name: 'dockerApps/vaultwarden',
        appId: 'vaultwarden',
        title: 'Vaultwarden',
        state: 'RUNNING',
        statusText: 'Up 5 hours',
        composeYaml: '',
        composeFilePath: '',
        services: ['vaultwarden'],
        containers: [],
        portMappings: ['8081:80/tcp'],
        volumes: [],
        webUiUrl: 'http://localhost:8081',
        iconUrl: 'https://example.com/vaultwarden.png',
        totalCpuPercent: 0.2,
        totalMemoryMib: 64,
        createdAt: '2026-01-01',
        updatedAt: '2026-01-01',
      } as DockerApp,
    ];

    const wrapper = mount(WidgetWebApps);
    const tile = wrapper.find('.webapp-tile');
    await tile.trigger('click');

    expect(openSpy).toHaveBeenCalledTimes(1);
    expect(openSpy).toHaveBeenCalledWith(
      expect.stringContaining(':8081'),
      '_blank',
      'noopener,noreferrer'
    );
  });

  it('supports apps with multiple web UI portals via dropdown selector', async () => {
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);

    nasState.dockerApps = [
      {
        name: 'dockerApps/npm',
        appId: 'npm',
        title: 'Nginx Proxy Manager',
        state: 'RUNNING',
        statusText: 'Up 1 day',
        composeYaml: '',
        composeFilePath: '',
        services: ['app'],
        containers: [],
        portMappings: ['80:80/tcp', '81:81/tcp'],
        volumes: [],
        webUiUrl: 'http://localhost:81',
        portals: [
          { name: 'Admin UI', url: 'http://localhost:81', port: 81, protocol: 'http', path: '/' },
          { name: 'Public HTTP', url: 'http://localhost:80', port: 80, protocol: 'http', path: '/' },
        ],
        iconUrl: 'https://example.com/npm.png',
        totalCpuPercent: 0.1,
        totalMemoryMib: 50,
        createdAt: '2026-01-01',
        updatedAt: '2026-01-01',
      } as DockerApp,
    ];

    const wrapper = mount(WidgetWebApps);
    const multiBtn = wrapper.find('.multi-port-trigger');
    expect(multiBtn.exists()).toBe(true);
    expect(multiBtn.text()).toContain('+1');

    // Click multi-port trigger to open portal dropdown
    await multiBtn.trigger('click');

    const dropdown = wrapper.find('.portal-dropdown-menu');
    expect(dropdown.exists()).toBe(true);
    expect(dropdown.text()).toContain('Admin UI');
    expect(dropdown.text()).toContain('Public HTTP');

    // Click second portal item
    const menuItems = dropdown.findAll('.portal-menu-item');
    expect(menuItems).toHaveLength(2);
    await menuItems[1].trigger('click');

    expect(openSpy).toHaveBeenCalledWith(
      expect.stringContaining(':80'),
      '_blank',
      'noopener,noreferrer'
    );
  });

  it('falls back gracefully to fallback icon when image fails to load', async () => {
    nasState.dockerApps = [
      {
        name: 'dockerApps/broken-icon-app',
        appId: 'broken-icon-app',
        title: 'Broken Icon App',
        state: 'RUNNING',
        statusText: 'Up 1 day',
        composeYaml: '',
        composeFilePath: '',
        services: ['app'],
        containers: [],
        portMappings: ['3000:3000/tcp'],
        volumes: [],
        webUiUrl: 'http://localhost:3000',
        iconUrl: 'https://broken.invalid/img.png',
        totalCpuPercent: 0.1,
        totalMemoryMib: 50,
        createdAt: '2026-01-01',
        updatedAt: '2026-01-01',
      } as DockerApp,
    ];

    const wrapper = mount(WidgetWebApps);
    const img = wrapper.find('.app-img');
    expect(img.exists()).toBe(true);

    // Trigger error on primary image
    await img.trigger('error');
    // Trigger error on fallback image
    const fallbackImg = wrapper.find('.app-img');
    if (fallbackImg.exists()) {
      await fallbackImg.trigger('error');
    }

    // Now icon-fallback svg should be visible
    expect(wrapper.find('.icon-fallback').exists()).toBe(true);
  });
});
