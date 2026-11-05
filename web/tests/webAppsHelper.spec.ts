import { describe, expect, it } from 'vitest';
import {
  collectWebApps,
  extractAppWebUis,
  extractPortFromUrl,
  getAppSlug,
  parsePortString,
  resolveWebUrl,
} from '../src/ui/dashboard/webAppsHelper';
import type { CatalogApp, DockerApp } from '../src/domain/models';

describe('webAppsHelper', () => {
  it('resolves localhost and 0.0.0.0 URLs to window hostname', () => {
    // In vitest/jsdom, location.hostname defaults to 'localhost' unless mocked
    expect(resolveWebUrl('http://localhost:8080/')).toContain(':8080/');
    expect(resolveWebUrl('http://0.0.0.0:9000')).toContain(':9000');
    expect(resolveWebUrl('http://192.168.1.50:8080/')).toBe('http://192.168.1.50:8080/');
  });

  it('extracts port numbers from URLs correctly', () => {
    expect(extractPortFromUrl('http://localhost:8080')).toBe('8080');
    expect(extractPortFromUrl('http://192.168.1.1:3000/dashboard')).toBe('3000');
    expect(extractPortFromUrl('https://example.com/')).toBe('443');
    expect(extractPortFromUrl('http://example.com/')).toBe('80');
    expect(extractPortFromUrl('invalid-url')).toBe('');
  });

  it('parses various docker port strings into host and container ports', () => {
    expect(parsePortString('8080:80/tcp')).toEqual({
      hostPort: '8080',
      containerPort: '80',
      protocol: 'tcp',
    });
    expect(parsePortString('0.0.0.0:8096->8096/tcp')).toEqual({
      hostPort: '8096',
      containerPort: '8096',
      protocol: 'tcp',
    });
    expect(parsePortString('127.0.0.1:3000:3000')).toEqual({
      hostPort: '3000',
      containerPort: '3000',
      protocol: 'tcp',
    });
    // Ignore UDP
    expect(parsePortString('53:53/udp')).toBeNull();
  });

  it('generates clean slugs for community dashboard icons', () => {
    expect(getAppSlug('Nextcloud')).toBe('nextcloud');
    expect(getAppSlug('Jellyfin Media Server')).toBe('jellyfin-media-server');
    expect(getAppSlug('Home_Assistant')).toBe('home_assistant');
  });

  it('extracts web UIs from an app with explicit webUiUrl and portals', () => {
    const app: DockerApp = {
      name: 'dockerApps/nextcloud',
      appId: 'nextcloud',
      title: 'Nextcloud Hub',
      state: 'RUNNING',
      statusText: 'Up 2 days',
      composeYaml: '',
      composeFilePath: '',
      services: ['app', 'db'],
      containers: [],
      portMappings: ['8080:80/tcp'],
      volumes: [],
      webUiUrl: 'http://localhost:8080',
      portals: [
        {
          name: 'Main Portal',
          url: 'http://localhost:8080',
          port: 8080,
          protocol: 'http',
          path: '/',
        },
      ],
      iconUrl: 'https://example.com/nextcloud.png',
      totalCpuPercent: 2.5,
      totalMemoryMib: 512,
      createdAt: '2026-01-01',
      updatedAt: '2026-01-01',
    };

    const result = extractAppWebUis(app);
    expect(result).not.toBeNull();
    expect(result?.title).toBe('Nextcloud Hub');
    expect(result?.primaryPort).toBe('8080');
    expect(result?.isRunning).toBe(true);
    expect(result?.iconUrl).toBe('https://example.com/nextcloud.png');
    expect(result?.tooltip).toContain('Nextcloud Hub');
    expect(result?.tooltip).toContain(':8080');
    expect(result?.tooltip).toContain('Running');
  });

  it('extracts web UIs from portMappings when webUiUrl is not explicitly set', () => {
    const app: DockerApp = {
      name: 'dockerApps/jellyfin',
      appId: 'jellyfin',
      title: 'Jellyfin',
      state: 'RUNNING',
      statusText: 'Up 1 day',
      composeYaml: '',
      composeFilePath: '',
      services: ['jellyfin'],
      containers: [],
      portMappings: ['8096:8096/tcp'],
      volumes: [],
      webUiUrl: '',
      totalCpuPercent: 1.0,
      totalMemoryMib: 256,
      createdAt: '2026-01-01',
      updatedAt: '2026-01-01',
    };

    const result = extractAppWebUis(app);
    expect(result).not.toBeNull();
    expect(result?.primaryPort).toBe('8096');
    expect(result?.primaryUrl).toContain(':8096');
    expect(result?.displayPorts).toContain(':8096');
  });

  it('excludes apps that have no exposed Web UI ports', () => {
    const app: DockerApp = {
      name: 'dockerApps/db-only',
      appId: 'db-only',
      title: 'PostgreSQL DB',
      state: 'RUNNING',
      statusText: 'Up 3 days',
      composeYaml: '',
      composeFilePath: '',
      services: ['db'],
      containers: [],
      portMappings: ['5432:5432/tcp'], // Non-web port
      volumes: [],
      webUiUrl: '',
      totalCpuPercent: 0.5,
      totalMemoryMib: 128,
      createdAt: '2026-01-01',
      updatedAt: '2026-01-01',
    };

    const result = extractAppWebUis(app);
    expect(result).toBeNull();
  });

  it('resolves icon from catalog if app iconUrl is missing', () => {
    const app: DockerApp = {
      name: 'dockerApps/plex',
      appId: 'plex',
      title: 'Plex Media Server',
      state: 'STOPPED',
      statusText: 'Exited',
      composeYaml: '',
      composeFilePath: '',
      services: ['plex'],
      containers: [],
      portMappings: ['32400:32400/tcp'],
      volumes: [],
      webUiUrl: 'http://localhost:32400/web',
      totalCpuPercent: 0,
      totalMemoryMib: 0,
      createdAt: '2026-01-01',
      updatedAt: '2026-01-01',
    };

    const catalog: CatalogApp[] = [
      {
        appId: 'plex',
        title: 'Plex',
        description: '',
        category: 'Media',
        categories: ['Media'],
        iconUrl: 'https://example.com/plex-catalog.png',
        home: '',
        train: 'community',
        version: '1.0',
        appVersion: '1.0',
        tags: [],
        screenshots: [],
        sources: [],
        containerImage: 'plex:latest',
        composeYaml: '',
        defaultPort: 32400,
      },
    ];

    const result = extractAppWebUis(app, catalog);
    expect(result).not.toBeNull();
    expect(result?.iconUrl).toBe('https://example.com/plex-catalog.png');
    expect(result?.isRunning).toBe(false);
    expect(result?.tooltip).toContain('Stopped');
  });

  it('collects and sorts web applications with running apps first', () => {
    const apps: DockerApp[] = [
      {
        name: 'dockerApps/vaultwarden',
        appId: 'vaultwarden',
        title: 'Vaultwarden',
        state: 'STOPPED',
        statusText: 'Exited',
        composeYaml: '',
        composeFilePath: '',
        services: ['vaultwarden'],
        containers: [],
        portMappings: ['8081:80/tcp'],
        volumes: [],
        webUiUrl: 'http://localhost:8081',
        totalCpuPercent: 0,
        totalMemoryMib: 0,
        createdAt: '2026-01-01',
        updatedAt: '2026-01-01',
      },
      {
        name: 'dockerApps/nextcloud',
        appId: 'nextcloud',
        title: 'Nextcloud',
        state: 'RUNNING',
        statusText: 'Up 1 day',
        composeYaml: '',
        composeFilePath: '',
        services: ['app'],
        containers: [],
        portMappings: ['8080:80/tcp'],
        volumes: [],
        webUiUrl: 'http://localhost:8080',
        totalCpuPercent: 1.2,
        totalMemoryMib: 256,
        createdAt: '2026-01-01',
        updatedAt: '2026-01-01',
      },
    ];

    const collected = collectWebApps(apps);
    expect(collected).toHaveLength(2);
    // Running app (Nextcloud) must come before stopped app (Vaultwarden)
    expect(collected[0].title).toBe('Nextcloud');
    expect(collected[1].title).toBe('Vaultwarden');
  });
});
