import { describe, expect, it, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import UserGroupSelector from '../src/ui/components/UserGroupSelector.vue';
import { nasState } from '../src/ui/store';
import type { User, Group } from '../src/domain/models';

describe('UserGroupSelector Component', () => {
  const sampleUsers: User[] = [
    {
      name: 'users/alice',
      username: 'alice',
      uid: 1001,
      primaryGid: 1001,
      primaryGroup: 'alice',
      supplementaryGroups: ['nasusers'],
      fullName: 'Alice Wonder',
      homeDirectory: '/home/alice',
      shell: '/bin/bash',
      sshEnabled: true,
      sshPasswordAuth: true,
      sshAuthorizedKeys: [],
      locked: false,
      systemAccount: false,
      isAdmin: false,
    },
    {
      name: 'users/bob',
      username: 'bob',
      uid: 1002,
      primaryGid: 1002,
      primaryGroup: 'bob',
      supplementaryGroups: ['docker'],
      fullName: 'Robert Builder',
      homeDirectory: '/home/bob',
      shell: '/bin/bash',
      sshEnabled: false,
      sshPasswordAuth: false,
      sshAuthorizedKeys: [],
      locked: false,
      systemAccount: false,
      isAdmin: false,
    },
    {
      name: 'users/sigxcpu',
      username: 'sigxcpu',
      uid: 1000,
      primaryGid: 1000,
      primaryGroup: 'sigxcpu',
      supplementaryGroups: ['wheel', 'sudo'],
      fullName: 'System Administrator',
      homeDirectory: '/home/sigxcpu',
      shell: '/bin/zsh',
      sshEnabled: true,
      sshPasswordAuth: true,
      sshAuthorizedKeys: [],
      locked: false,
      systemAccount: false,
      isAdmin: true,
    },
  ];

  const sampleGroups: Group[] = [
    {
      name: 'groups/nasusers',
      groupName: 'nasusers',
      gid: 1001,
      members: ['alice', 'bob'],
      systemGroup: false,
    },
    {
      name: 'groups/family',
      groupName: 'family',
      gid: 1002,
      members: ['alice'],
      systemGroup: false,
    },
    {
      name: 'groups/docker',
      groupName: 'docker',
      gid: 998,
      members: ['bob', 'sigxcpu'],
      systemGroup: true,
    },
  ];

  beforeEach(() => {
    nasState.users = [...sampleUsers];
    nasState.groups = [...sampleGroups];
  });

  describe('Single-Select Mode', () => {
    it('displays selected single user and opens dropdown on toggle', async () => {
      const wrapper = mount(UserGroupSelector, {
        props: {
          modelValue: 'sigxcpu',
          type: 'user',
        },
      });

      expect(wrapper.text()).toContain('sigxcpu');
      expect(wrapper.find('.dropdown-menu').exists()).toBe(false);

      await wrapper.find('.user-group-selector').trigger('click');
      expect(wrapper.find('.dropdown-menu').exists()).toBe(true);
    });

    it('filters users using fuzzy search', async () => {
      const wrapper = mount(UserGroupSelector, {
        props: {
          modelValue: '',
          type: 'user',
        },
      });

      await wrapper.find('.user-group-selector').trigger('click');
      const searchInput = wrapper.find('.dropdown-search-input');

      // Search by partial name / username "rob" (matches Robert / bob)
      await searchInput.setValue('rob');
      const options = wrapper.findAll('.option-item');
      expect(options.length).toBeGreaterThanOrEqual(1);
      expect(wrapper.text()).toContain('bob');
      expect(wrapper.text()).toContain('Robert Builder');
    });

    it('emits update:modelValue when an option is clicked', async () => {
      const wrapper = mount(UserGroupSelector, {
        props: {
          modelValue: '',
          type: 'user',
        },
      });

      await wrapper.find('.user-group-selector').trigger('click');
      const firstOpt = wrapper.find('.option-item');
      await firstOpt.trigger('click');

      expect(wrapper.emitted('update:modelValue')).toBeTruthy();
      expect(wrapper.emitted('update:modelValue')![0][0]).toBe('alice');
      expect(wrapper.find('.dropdown-menu').exists()).toBe(false);
    });

    it('allows clearing single selection', async () => {
      const wrapper = mount(UserGroupSelector, {
        props: {
          modelValue: 'alice',
          type: 'user',
        },
      });

      const clearBtn = wrapper.find('.btn-clear');
      expect(clearBtn.exists()).toBe(true);
      await clearBtn.trigger('click');

      expect(wrapper.emitted('update:modelValue')).toBeTruthy();
      expect(wrapper.emitted('update:modelValue')![0][0]).toBe('');
    });
  });

  describe('Multi-Select Mode', () => {
    it('parses comma-separated string into chips', () => {
      const wrapper = mount(UserGroupSelector, {
        props: {
          modelValue: 'alice, bob',
          type: 'user',
          multiple: true,
        },
      });

      const chips = wrapper.findAll('.selector-chip');
      expect(chips.length).toBe(2);
      expect(chips[0].text()).toContain('alice');
      expect(chips[1].text()).toContain('bob');
    });

    it('removes a chip and emits updated comma-separated string', async () => {
      const wrapper = mount(UserGroupSelector, {
        props: {
          modelValue: 'alice, bob',
          type: 'user',
          multiple: true,
        },
      });

      const removeBtns = wrapper.findAll('.chip-remove-btn');
      await removeBtns[0].trigger('click');

      expect(wrapper.emitted('update:modelValue')).toBeTruthy();
      expect(wrapper.emitted('update:modelValue')![0][0]).toBe('bob');
    });

    it('adds an option when clicked from dropdown and keeps existing selections', async () => {
      const wrapper = mount(UserGroupSelector, {
        props: {
          modelValue: 'alice',
          type: 'user',
          multiple: true,
        },
      });

      await wrapper.find('.chip-search-input').trigger('focus');
      expect(wrapper.find('.dropdown-menu').exists()).toBe(true);

      // Find 'bob' option
      const options = wrapper.findAll('.option-item');
      const bobOpt = options.find((o) => o.text().includes('bob'));
      expect(bobOpt).toBeTruthy();

      await bobOpt!.trigger('click');
      expect(wrapper.emitted('update:modelValue')).toBeTruthy();
      expect(wrapper.emitted('update:modelValue')![0][0]).toBe('alice, bob');
    });

    it('allows typing a custom user/group and pressing Enter', async () => {
      const wrapper = mount(UserGroupSelector, {
        props: {
          modelValue: 'alice',
          type: 'group',
          multiple: true,
          allowCustom: true,
        },
      });

      const input = wrapper.find('.chip-search-input');
      await input.setValue('customgroup');
      await input.trigger('keydown', { key: 'Enter' });

      expect(wrapper.emitted('update:modelValue')).toBeTruthy();
      expect(wrapper.emitted('update:modelValue')![0][0]).toBe('alice, customgroup');
    });
  });

  describe('Group Mode', () => {
    it('displays groups with GID and member details in dropdown', async () => {
      const wrapper = mount(UserGroupSelector, {
        props: {
          modelValue: 'nasusers',
          type: 'group',
        },
      });

      await wrapper.find('.user-group-selector').trigger('click');
      expect(wrapper.text()).toContain('nasusers');
      expect(wrapper.text()).toContain('GID: 1001');
      expect(wrapper.text()).toContain('family');
      expect(wrapper.text()).toContain('docker');
    });
  });
});
