import { beforeEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import CommandPalette from '../src/ui/components/CommandPalette.vue';
import { nasState } from '../src/ui/store';
import type { DockerApp, LxcContainer, VirtualMachine, ZfsPool } from '../src/domain/models';

describe('CommandPalette Multi-word, Compound & Fuzzy Subsequence Search', () => {
  beforeEach(() => {
    // Populate mock resources
    nasState.dockerApps = [
      {
        name: 'dockerApps/quasar',
        appId: 'quasar',
        title: 'Quasar',
        state: 'RUNNING',
        containers: [{ id: 'c1', name: 'quasar-web', state: 'running' }],
      } as unknown as DockerApp,
      {
        name: 'dockerApps/nginx-proxy-manager',
        appId: 'nginx-proxy-manager',
        title: 'Nginx Proxy Manager',
        state: 'RUNNING',
        containers: [{ id: 'c2', name: 'npm-app', state: 'running' }],
      } as unknown as DockerApp,
    ];

    nasState.vms = [
      {
        name: 'virtualMachines/windows11',
        vmName: 'windows11',
        uuid: 'vm-11',
        state: 'RUNNING',
        vcpus: 4,
        memoryMib: 8192,
      } as unknown as VirtualMachine,
    ];

    nasState.lxcContainers = [
      {
        name: 'lxcContainers/ct100',
        containerName: 'ct100',
        state: 'RUNNING',
        ipv4Address: '192.168.27.100',
      } as unknown as LxcContainer,
    ];

    nasState.pools = [
      {
        name: 'zfsPools/tank',
        poolName: 'tank',
        state: 'ONLINE',
        capacityPercent: 42,
      } as unknown as ZfsPool,
    ];
  });

  it('filters resource and shows Quasar when searching "qua"', async () => {
    const wrapper = mount(CommandPalette, {
      props: {
        modelValue: true,
      },
    });

    const input = wrapper.find('input.palette-input');
    await input.setValue('qua');

    const titles = wrapper.findAll('.item-title').map((el) => el.text());
    expect(titles.length).toBeGreaterThan(0);
    // Base resource Quasar should be at index 0 because single token boosts Resources
    expect(titles[0]).toBe('Quasar');
    // Contextual actions also present
    expect(titles).toContain('Quasar: Console / Terminal');
    expect(titles).toContain('Quasar: Logs');
  });

  it('fuzzy matches "qualo" to "Quasar: Logs" at index 0', async () => {
    const wrapper = mount(CommandPalette, {
      props: {
        modelValue: true,
      },
    });

    const input = wrapper.find('input.palette-input');
    await input.setValue('qualo');

    const titles = wrapper.findAll('.item-title').map((el) => el.text());
    expect(titles.length).toBeGreaterThan(0);
    expect(titles[0]).toBe('Quasar: Logs');
  });

  it('fuzzy matches "quaterm" to "Quasar: Console / Terminal" at index 0', async () => {
    const wrapper = mount(CommandPalette, {
      props: {
        modelValue: true,
      },
    });

    const input = wrapper.find('input.palette-input');
    await input.setValue('quaterm');

    const titles = wrapper.findAll('.item-title').map((el) => el.text());
    expect(titles.length).toBeGreaterThan(0);
    expect(titles[0]).toBe('Quasar: Console / Terminal');
  });

  it('fuzzy matches "quacon" to "Quasar: Console / Terminal" at index 0', async () => {
    const wrapper = mount(CommandPalette, {
      props: {
        modelValue: true,
      },
    });

    const input = wrapper.find('input.palette-input');
    await input.setValue('quacon');

    const titles = wrapper.findAll('.item-title').map((el) => el.text());
    expect(titles.length).toBeGreaterThan(0);
    expect(titles[0]).toBe('Quasar: Console / Terminal');
  });

  it('selects Quasar: Console / Terminal when searching two words "qua term"', async () => {
    const wrapper = mount(CommandPalette, {
      props: {
        modelValue: true,
      },
    });

    const input = wrapper.find('input.palette-input');
    await input.setValue('qua term');

    const titles = wrapper.findAll('.item-title').map((el) => el.text());
    expect(titles.length).toBeGreaterThan(0);
    expect(titles[0]).toBe('Quasar: Console / Terminal');
    expect(titles).not.toContain('Quasar');
    expect(titles).not.toContain('Quasar: Logs');
  });

  it('selects Quasar: Console / Terminal when searching two words "qua con"', async () => {
    const wrapper = mount(CommandPalette, {
      props: {
        modelValue: true,
      },
    });

    const input = wrapper.find('input.palette-input');
    await input.setValue('qua con');

    const titles = wrapper.findAll('.item-title').map((el) => el.text());
    expect(titles.length).toBeGreaterThan(0);
    expect(titles[0]).toBe('Quasar: Console / Terminal');
  });

  it('supports inverted word order like "term qua"', async () => {
    const wrapper = mount(CommandPalette, {
      props: {
        modelValue: true,
      },
    });

    const input = wrapper.find('input.palette-input');
    await input.setValue('term qua');

    const titles = wrapper.findAll('.item-title').map((el) => el.text());
    expect(titles.length).toBeGreaterThan(0);
    expect(titles[0]).toBe('Quasar: Console / Terminal');
  });

  it('fuzzy matches "wincon" to "windows11: Console"', async () => {
    const wrapper = mount(CommandPalette, {
      props: {
        modelValue: true,
      },
    });

    const input = wrapper.find('input.palette-input');
    await input.setValue('wincon');

    const titles = wrapper.findAll('.item-title').map((el) => el.text());
    expect(titles[0]).toBe('windows11: Console');
  });

  it('fuzzy matches "ctterm" to "ct100: Terminal / Console"', async () => {
    const wrapper = mount(CommandPalette, {
      props: {
        modelValue: true,
      },
    });

    const input = wrapper.find('input.palette-input');
    await input.setValue('ctterm');

    const titles = wrapper.findAll('.item-title').map((el) => el.text());
    expect(titles[0]).toBe('ct100: Terminal / Console');
  });

  it('fuzzy matches acronym "npm" to "Nginx Proxy Manager"', async () => {
    const wrapper = mount(CommandPalette, {
      props: {
        modelValue: true,
      },
    });

    const input = wrapper.find('input.palette-input');
    await input.setValue('npm');

    const titles = wrapper.findAll('.item-title').map((el) => el.text());
    expect(titles[0]).toBe('Nginx Proxy Manager');
  });

  it('executing "qualo" switches to docker and sets dockerSubTab to logs', async () => {
    const wrapper = mount(CommandPalette, {
      props: {
        modelValue: true,
      },
    });

    const input = wrapper.find('input.palette-input');
    await input.setValue('qualo');

    const firstItem = wrapper.find('.palette-item');
    await firstItem.trigger('click');

    expect(nasState.selectedResourceId).toBe('docker:quasar');
    expect(nasState.activeTab).toBe('docker');
    expect(nasState.dockerSubTab).toBe('logs');
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([false]);
  });
});
