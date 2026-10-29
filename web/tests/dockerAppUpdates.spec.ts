import { describe, expect, it, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { nextTick } from 'vue';
import { mapDockerApp, HttpDockerAppAdapter } from '../src/adapters/http/httpAdapters';
import { AipClient } from '../src/adapters/http/aipClient';
import { services } from '../src/container';
import { nasState } from '../src/ui/store';
import DockerView from '../src/ui/views/DockerView.vue';
import type { DockerApp } from '../src/domain/models';

describe('Docker App Catalog Metadata & One-Click Upgrades', () => {
  const sampleAppWithUpdate: DockerApp = {
    name: 'dockerApps/esphome',
    appId: 'esphome',
    title: 'ESPHome',
    state: 'RUNNING',
    statusText: '1/1 running',
    autoStart: true,
    composeYaml: `x-nas-catalog-id: esphome
x-nas-catalog-source: stable
x-nas-catalog-version: 2026.10.0
services:
  esphome:
    image: ghcr.io/esphome/esphome:2026.10.0
`,
    composeFilePath: '/nas/tank/apps/esphome/compose.yaml',
    services: ['esphome'],
    containers: [
      {
        containerId: 'c1',
        name: 'esphome-1',
        serviceName: 'esphome',
        image: 'ghcr.io/esphome/esphome:2026.10.0',
        state: 'RUNNING',
        status: 'Up 2 hours',
        cpuPercent: 0.5,
        memoryMib: 120,
        ports: ['36052:80'],
        createdAt: '2026-10-29T10:00:00Z',
      },
    ],
    portMappings: ['36052:80'],
    volumes: ['/mnt/tank/apps/esphome:/data'],
    webUiUrl: 'http://localhost:36052',
    catalogAppId: 'esphome',
    catalogSource: 'stable',
    catalogVersion: '2026.10.0',
    updateAvailable: true,
    latestVersion: '2026.11.0',
    latestImage: 'ghcr.io/esphome/esphome:2026.11.0',
    totalCpuPercent: 0.5,
    totalMemoryMib: 120,
    createdAt: '2026-10-29T10:00:00Z',
    updatedAt: '2026-10-29T10:00:00Z',
  };

  beforeEach(() => {
    vi.restoreAllMocks();
    nasState.apps = [sampleAppWithUpdate];
    nasState.dockerConfig = {
      dataset: 'tank/apps',
      pool: 'tank',
      mountpoint: '/mnt/tank/apps',
      appdataPath: '/mnt/tank/apps',
      dockerDataRoot: '/mnt/tank/docker',
      availableDatasets: ['tank/apps'],
      availablePools: ['tank'],
      datasetSelected: true,
      poolSelected: true,
    };
  });

  it('maps catalog metadata and update status correctly in mapDockerApp', () => {
    const raw = {
      name: 'dockerApps/esphome',
      app_id: 'esphome',
      title: 'ESPHome',
      state: 'RUNNING',
      catalog_app_id: 'esphome',
      catalog_source: 'stable',
      catalog_version: '2026.10.0',
      update_available: true,
      latest_version: '2026.11.0',
      latest_image: 'ghcr.io/esphome/esphome:2026.11.0',
      total_cpu_percent: 1.2,
      total_memory_mib: 150,
      containers: [],
    };

    const mapped = mapDockerApp(raw);
    expect(mapped.catalogAppId).toBe('esphome');
    expect(mapped.catalogSource).toBe('stable');
    expect(mapped.catalogVersion).toBe('2026.10.0');
    expect(mapped.updateAvailable).toBe(true);
    expect(mapped.latestVersion).toBe('2026.11.0');
    expect(mapped.latestImage).toBe('ghcr.io/esphome/esphome:2026.11.0');
  });

  it('calls customMethod upgrade with take_snapshot flag in HttpDockerAppAdapter', async () => {
    const mockClient = {
      customMethod: vi.fn().mockResolvedValue({
        name: 'dockerApps/esphome',
        app_id: 'esphome',
        state: 'DEPLOYING',
        status_text: 'Upgrading stack (pulling latest images & restarting)...',
      }),
    } as unknown as AipClient;

    const adapter = new HttpDockerAppAdapter(mockClient);
    const result = await adapter.upgradeApp('dockerApps/esphome', true);

    expect(mockClient.customMethod).toHaveBeenCalledWith(
      'dockerApps/esphome',
      'upgrade',
      { take_snapshot: true }
    );
    expect(result.appId).toBe('esphome');
    expect(result.state).toBe('DEPLOYING');
  });

  it('renders Update Available badge and opens Upgrade modal in DockerView', async () => {
    vi.spyOn(services.apps, 'listApps').mockResolvedValue([sampleAppWithUpdate]);
    vi.spyOn(services.apps, 'getDockerConfig').mockResolvedValue(nasState.dockerConfig!);
    const upgradeSpy = vi.spyOn(services.apps, 'upgradeApp').mockResolvedValue({
      ...sampleAppWithUpdate,
      state: 'DEPLOYING',
      statusText: 'Upgrading stack...',
    });

    const wrapper = mount(DockerView);
    await nextTick();
    await nextTick();

    // Check that Update Available badge is rendered
    expect(wrapper.text()).toContain('Update: 2026.11.0');

    // Click the Upgrade action button
    const upgradeBtn = wrapper.find('button[aria-label="Upgrade ESPHome"]');
    expect(upgradeBtn.exists()).toBe(true);
    await upgradeBtn.trigger('click');
    await nextTick();

    // Check modal contents
    expect(wrapper.text()).toContain('Upgrade: ESPHome');
    expect(wrapper.text()).toContain('Create pre-upgrade ZFS snapshot');
    expect(wrapper.text()).toContain('Installed: 2026.10.0 → Latest: 2026.11.0');

    // Confirm upgrade
    const confirmBtn = wrapper.findAll('button').find((b) => b.text().includes('Confirm Upgrade'));
    expect(confirmBtn).toBeDefined();
    await confirmBtn!.trigger('click');
    await nextTick();

    expect(upgradeSpy).toHaveBeenCalledWith('dockerApps/esphome', true);
  });
});
