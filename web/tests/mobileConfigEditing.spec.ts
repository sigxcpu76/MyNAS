import { describe, it, expect, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { nasState } from '../src/ui/store';
import { navigateTo } from '../src/ui/router';
import MobileShell from '../src/ui/mobile/MobileShell.vue';
import MobileKvmView from '../src/ui/mobile/views/MobileKvmView.vue';
import MobilePowerSheet from '../src/ui/mobile/components/MobilePowerSheet.vue';
import MobileAppConfigEditor from '../src/ui/mobile/components/MobileAppConfigEditor.vue';

const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] || null,
    setItem: (key: string, value: string) => {
      store[key] = value.toString();
    },
    removeItem: (key: string) => {
      delete store[key];
    },
    clear: () => {
      store = {};
    },
  };
})();

Object.defineProperty(window, 'localStorage', {
  value: localStorageMock,
  writable: true,
});

describe('Mobile UI configuration editing', () => {
  beforeEach(() => {
    localStorageMock.clear();
    nasState.activeTab = 'overview';
    nasState.selectedResourceId = 'node:local';
    nasState.dockerSubTab = 'summary';
    nasState.vmSubTab = 'summary';
    nasState.lxcSubTab = 'summary';
    nasState.poolSubTab = 'summary';
    nasState.systemConfig = {
      name: 'systemConfig',
      mainPool: 'zfsPools/tank',
    } as any;
    nasState.pools = [
      {
        name: 'zfsPools/tank',
        poolName: 'tank',
        state: 'ONLINE',
        sizeBytes: 4 * 1024 ** 4,
        allocatedBytes: 1 * 1024 ** 4,
        freeBytes: 3 * 1024 ** 4,
        fragmentationPercent: 5,
        dedupRatio: 1,
      } as any,
    ];
    nasState.dockerApps = [
      {
        name: 'dockerApps/jellyfin',
        appId: 'jellyfin',
        title: 'Jellyfin',
        state: 'RUNNING',
        composeYaml: 'services:\n  jellyfin:\n    image: jellyfin/jellyfin:latest\n',
        containers: [],
      } as any,
    ];
    nasState.vms = [
      {
        name: 'virtualMachines/debian-vm',
        vmName: 'debian-vm',
        state: 'RUNNING',
        vcpus: 2,
        memoryMib: 4096,
      } as any,
    ];
    nasState.lxcContainers = [
      {
        name: 'lxcContainers/pihole',
        containerName: 'pihole',
        state: 'RUNNING',
        distribution: 'debian',
      } as any,
    ];
  });

  const detailStubs = {
    MobileAppConfigEditor: { template: '<div class="stub-mobile-app-config" />', props: ['app'] },
    DockerDetailView: { template: '<div class="stub-docker-detail" />', props: ['appName', 'activeSubTab'] },
    VmDetailView: { template: '<div class="stub-vm-detail" />', props: ['vmName', 'activeSubTab'] },
    LxcDetailView: { template: '<div class="stub-lxc-detail" />', props: ['containerName', 'activeSubTab'] },
    PoolDetailView: { template: '<div class="stub-pool-detail" />', props: ['poolName', 'activeSubTab'] },
    MobileOverview: { template: '<div class="stub-mobile-overview" />' },
    MobileAppsView: { template: '<div class="stub-mobile-apps" />' },
    MobileStorageView: { template: '<div class="stub-mobile-storage" />' },
    MobileKvmView: { template: '<div class="stub-mobile-kvm" />' },
    MobilePowerSheet: { template: '<div />' },
    MobileNavDrawer: { template: '<div />' },
  };

  it('renders MobileAppConfigEditor on configuration tab and DockerDetailView on summary tab', async () => {
    navigateTo('docker', false, 'docker:jellyfin');
    nasState.dockerSubTab = 'configuration';
    const wrapper = mount(MobileShell, { global: { stubs: detailStubs } });

    expect(wrapper.find('.mobile-detail-header').exists()).toBe(true);
    expect(wrapper.find('.mobile-detail-title').text()).toBe('Jellyfin');
    expect(wrapper.find('.stub-mobile-app-config').exists()).toBe(true);

    nasState.dockerSubTab = 'summary';
    await wrapper.vm.$nextTick();
    expect(wrapper.find('.stub-docker-detail').exists()).toBe(true);

    await wrapper.find('.mobile-detail-back-btn').trigger('click');
    expect(nasState.selectedResourceId).toBe('node:docker');
    expect(wrapper.find('.stub-mobile-apps').exists()).toBe(true);
  });

  it('renders mobile accordion sections and Form/YAML switch in MobileAppConfigEditor', async () => {
    const wrapper = mount(MobileAppConfigEditor, {
      props: { app: nasState.dockerApps[0] },
      global: {
        stubs: {
          CodeMirrorEditor: { template: '<textarea class="stub-cm" />', props: ['modelValue'] },
        },
      },
    });

    expect(wrapper.find('.m-segmented').exists()).toBe(true);
    expect(wrapper.find('.m-sticky-save-bar').exists()).toBe(true);
    expect(wrapper.findAll('.m-accordion').length).toBeGreaterThanOrEqual(5);

    // Switch to Raw YAML
    const yamlBtn = wrapper.findAll('.m-seg-btn')[1];
    await yamlBtn.trigger('click');
    expect(wrapper.find('.stub-cm').exists()).toBe(true);

    // Switch back to Guided Form
    const formBtn = wrapper.findAll('.m-seg-btn')[0];
    await formBtn.trigger('click');
    expect(wrapper.findAll('.m-accordion').length).toBeGreaterThanOrEqual(5);
  });

  it('renders VmDetailView, LxcDetailView, and PoolDetailView in MobileShell', async () => {
    navigateTo('kvm', false, 'vm:debian-vm');
    nasState.vmSubTab = 'configuration';
    const wrapper = mount(MobileShell, { global: { stubs: detailStubs } });
    expect(wrapper.find('.stub-vm-detail').exists()).toBe(true);

    navigateTo('lxc', false, 'lxc:pihole');
    nasState.lxcSubTab = 'configuration';
    await wrapper.vm.$nextTick();
    expect(wrapper.find('.stub-lxc-detail').exists()).toBe(true);

    navigateTo('storage-pools', false, 'pool:tank');
    nasState.poolSubTab = 'config';
    await wrapper.vm.$nextTick();
    expect(wrapper.find('.stub-pool-detail').exists()).toBe(true);
  });

  it('navigates to VM and LXC configuration from MobileKvmView Config buttons', async () => {
    nasState.activeTab = 'kvm';
    const wrapper = mount(MobileKvmView);

    const vmConfigBtn = wrapper.findAll('button').find((b) => b.text().includes('Config'));
    expect(vmConfigBtn).toBeDefined();
    await vmConfigBtn!.trigger('click');
    expect(nasState.selectedResourceId).toBe('vm:debian-vm');
    expect(nasState.vmSubTab).toBe('configuration');

    const lxcTabBtn = wrapper.findAll('.workload-tab')[1];
    await lxcTabBtn.trigger('click');
    const lxcConfigBtn = wrapper.findAll('button').find((b) => b.text().includes('Config'));
    expect(lxcConfigBtn).toBeDefined();
    await lxcConfigBtn!.trigger('click');
    expect(nasState.selectedResourceId).toBe('lxc:pihole');
    expect(nasState.lxcSubTab).toBe('configuration');
  });

  it('allows opening Main Pool modal from MobilePowerSheet even when mainPool is set', async () => {
    const wrapper = mount(MobilePowerSheet, {
      props: { modelValue: true },
      global: {
        stubs: {
          BottomSheet: { template: '<div><slot /></div>' },
        },
      },
    });

    const poolBtn = wrapper.findAll('button.action-tile').find((b) => b.text().includes('Main Pool'));
    expect(poolBtn).toBeDefined();
    await poolBtn!.trigger('click');
    expect(wrapper.emitted('open-pool-modal')).toBeTruthy();
  });
});
