import { describe, expect, it, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import IdentityView from '../src/ui/views/IdentityView.vue';
import { nasState } from '../src/ui/store';
import type { User, Group } from '../src/domain/models';

describe('IdentityView Component', () => {
  beforeEach(() => {
    nasState.includeSystemAccounts = false;
    nasState.includeSystemGroups = false;
    nasState.users = [
      {
        name: 'users/root',
        username: 'root',
        uid: 0,
        primaryGid: 0,
        primaryGroup: 'root',
        supplementaryGroups: [],
        fullName: 'root',
        homeDirectory: '/root',
        shell: '/bin/bash',
        sshEnabled: true,
        sshPasswordAuth: true,
        sshAuthorizedKeys: [],
        locked: false,
        systemAccount: true,
        isAdmin: true,
      },
      {
        name: 'users/sigxcpu',
        username: 'sigxcpu',
        uid: 1000,
        primaryGid: 1000,
        primaryGroup: 'sigxcpu',
        supplementaryGroups: ['users'],
        fullName: 'sigxcpu',
        homeDirectory: '/home/sigxcpu',
        shell: '/bin/bash',
        sshEnabled: true,
        sshPasswordAuth: true,
        sshAuthorizedKeys: [],
        locked: false,
        systemAccount: false,
        isAdmin: true,
      },
      {
        name: 'users/libvirt-qemu',
        username: 'libvirt-qemu',
        uid: 64055,
        primaryGid: 993,
        primaryGroup: 'kvm',
        supplementaryGroups: [],
        fullName: 'Libvirt Qemu',
        homeDirectory: '/var/lib/libvirt',
        shell: '/usr/sbin/nologin',
        sshEnabled: false,
        sshPasswordAuth: false,
        sshAuthorizedKeys: [],
        locked: false,
        systemAccount: true,
        isAdmin: false,
      },
    ];

    nasState.groups = [
      {
        name: 'groups/users',
        groupName: 'users',
        gid: 100,
        members: ['sigxcpu'],
        systemGroup: true,
      },
      {
        name: 'groups/sigxcpu',
        groupName: 'sigxcpu',
        gid: 1000,
        members: [],
        systemGroup: false,
      },
      {
        name: 'groups/libvirt-qemu',
        groupName: 'libvirt-qemu',
        gid: 64055,
        members: ['libvirt-qemu'],
        systemGroup: true,
      },
    ];
  });

  it('filters out UIDs < 1000 and > 10000 from the users table by default', () => {
    const wrapper = mount(IdentityView);
    const text = wrapper.text();

    expect(text).toContain('sigxcpu');
    expect(text).not.toContain('root');
    expect(text).not.toContain('libvirt-qemu');
  });

  it('filters out GIDs < 1000 and > 10000 from the groups table by default', () => {
    const wrapper = mount(IdentityView);
    const text = wrapper.text();

    expect(text).toContain('sigxcpu');
    // Group 'users' (GID 100) and 'libvirt-qemu' (GID 64055) should not be shown
    const groupRows = wrapper.findAll('.data-table tbody tr');
    const groupText = groupRows.map((r) => r.text()).join(' ');
    expect(groupText).not.toContain('GID 100');
    expect(groupText).not.toContain('64055');
  });

  it('does not pre-populate with invented users like "admin" or fake keys', async () => {
    const wrapper = mount(IdentityView);

    // Open Create User form
    const createBtn = wrapper.findAll('button').find((b) => b.text().includes('Create User'));
    expect(createBtn).toBeTruthy();
    await createBtn!.trigger('click');

    const inputs = wrapper.findAll('input');
    const userInputsValues = inputs.map((i) => (i.element as HTMLInputElement).value);
    expect(userInputsValues).not.toContain('admin');
    expect(userInputsValues).not.toContain('nasusers');

    const textareas = wrapper.findAll('textarea');
    const textareaValues = textareas.map((t) => (t.element as HTMLTextAreaElement).value);
    expect(textareaValues.some((v) => v.includes('admin@nas'))).toBe(false);

    // Open Create Group form
    const createGroupBtn = wrapper.findAll('button').find((b) => b.text().includes('Create Group'));
    expect(createGroupBtn).toBeTruthy();
    await createGroupBtn!.trigger('click');

    const groupInputsValues = wrapper.findAll('input').map((i) => (i.element as HTMLInputElement).value);
    expect(groupInputsValues).not.toContain('admin');
  });

  it('dynamically suggests next available UID and GID >= 1000 and detects duplicates', async () => {
    const wrapper = mount(IdentityView);

    // Open Create User form
    const createBtn = wrapper.findAll('button').find((b) => b.text().includes('Create User'));
    await createBtn!.trigger('click');

    // UID 1000 is used by sigxcpu, so next available is 1001
    const uidInput = wrapper.find('input[type="number"]');
    expect((uidInput.element as HTMLInputElement).value).toBe('1001');

    // Setting it to 1000 triggers duplicate warning
    await uidInput.setValue('1000');
    expect(wrapper.text()).toContain('UID 1000 is already in use by sigxcpu');
  });
});
