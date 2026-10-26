import { beforeEach, describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import PrerequisitesTopBarButton from '../src/ui/components/PrerequisitesTopBarButton.vue';
import PrerequisitesModal from '../src/ui/components/PrerequisitesModal.vue';
import { nasState } from '../src/ui/store';
import type { Prerequisite } from '../src/domain/models';

const store = new Map<string, string>();
const localStorageMock = {
  getItem: (key: string) => store.get(key) ?? null,
  setItem: (key: string, val: string) => store.set(key, String(val)),
  removeItem: (key: string) => store.delete(key),
  clear: () => store.clear(),
};

Object.defineProperty(window, 'localStorage', {
  value: localStorageMock,
  writable: true,
});

describe('Prerequisites Top Bar Button & Modal', () => {
  beforeEach(() => {
    localStorageMock.clear();
    nasState.initialLoaded = true;
  });

  describe('PrerequisitesTopBarButton', () => {
    it('displays GREEN status when all prerequisites are installed', () => {
      nasState.prerequisites = [
        { name: 'prerequisites/zfs', displayName: 'ZFS', category: 'STORAGE', status: 'INSTALLED' } as Prerequisite,
        { name: 'prerequisites/qemu', displayName: 'QEMU', category: 'VIRTUALIZATION', status: 'INSTALLED' } as Prerequisite,
      ];

      const wrapper = mount(PrerequisitesTopBarButton);
      expect(wrapper.classes()).toContain('btn-green');
      expect(wrapper.find('.pill-green').text()).toBe('OK');
      expect(wrapper.attributes('title')).toContain('All 2 components verified');
    });

    it('displays AMBER status when non-critical prerequisites are missing', () => {
      nasState.prerequisites = [
        { name: 'prerequisites/zfs', displayName: 'ZFS', category: 'STORAGE', status: 'INSTALLED' } as Prerequisite,
        { name: 'prerequisites/docker', displayName: 'Docker', category: 'CONTAINERS', status: 'MISSING' } as Prerequisite,
        { name: 'prerequisites/lxc', displayName: 'LXC', category: 'CONTAINERS', status: 'FALLBACK_ACTIVE' } as Prerequisite,
      ];

      const wrapper = mount(PrerequisitesTopBarButton);
      expect(wrapper.classes()).toContain('btn-amber');
      expect(wrapper.find('.pill-amber').text()).toBe('2 Missing');
      expect(wrapper.attributes('title')).toContain('2 missing');
    });

    it('displays RED status when critical storage prerequisite (ZFS) is missing', () => {
      nasState.prerequisites = [
        { name: 'prerequisites/zfs', displayName: 'ZFS', category: 'STORAGE', status: 'MISSING' } as Prerequisite,
        { name: 'prerequisites/docker', displayName: 'Docker', category: 'CONTAINERS', status: 'INSTALLED' } as Prerequisite,
      ];

      const wrapper = mount(PrerequisitesTopBarButton);
      expect(wrapper.classes()).toContain('btn-red');
      expect(wrapper.find('.pill-red').text()).toBe('1 Missing');
      expect(wrapper.attributes('title')).toContain('Critical Storage/System required');
    });

    it('emits click event on user click', async () => {
      const wrapper = mount(PrerequisitesTopBarButton);
      await wrapper.trigger('click');
      expect(wrapper.emitted('click')).toHaveLength(1);
    });
  });

  describe('PrerequisitesModal', () => {
    it('renders fix instructions and remediation command when prerequisites are missing', () => {
      nasState.prerequisites = [
        {
          name: 'prerequisites/zfs',
          displayName: 'ZFS on Linux',
          category: 'STORAGE',
          kind: 'KERNEL_MODULE',
          binaryOrPath: '/sbin/zpool',
          status: 'MISSING',
          installHint: 'apt-get install -y zfsutils-linux',
        } as Prerequisite,
        {
          name: 'prerequisites/qemu',
          displayName: 'QEMU-KVM',
          category: 'VIRTUALIZATION',
          kind: 'BINARY',
          binaryOrPath: '/usr/bin/qemu-system-x86_64',
          status: 'MISSING',
          installHint: 'apt-get install -y qemu-system-x86',
        } as Prerequisite,
      ];

      const wrapper = mount(PrerequisitesModal);

      // Verify modal headers and status
      expect(wrapper.text()).toContain('System Prerequisites & Host Compliance');
      expect(wrapper.text()).toContain('Instructions to Fix Missing Prerequisites');
      expect(wrapper.text()).toContain('Copy Fix Command');

      // Verify missing items table
      expect(wrapper.text()).toContain('ZFS on Linux');
      expect(wrapper.text()).toContain('QEMU-KVM');
      expect(wrapper.text()).toContain('apt-get install -y zfsutils-linux');
    });

    it('emits close event when close button is clicked', async () => {
      nasState.prerequisites = [];
      const wrapper = mount(PrerequisitesModal);

      const closeBtn = wrapper.find('.btn-icon');
      await closeBtn.trigger('click');
      expect(wrapper.emitted('close')).toHaveLength(1);
    });
  });
});
