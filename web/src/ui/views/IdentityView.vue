<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { mdiDelete, mdiPencil } from '@mdi/js';
import { services } from '../../container';
import type { Group, User } from '../../domain/models';
import MdiIcon from '../components/MdiIcon.vue';
import SkeletonBox from '../components/SkeletonBox.vue';
import SortableTh from '../components/SortableTh.vue';
import UserGroupSelector from '../components/UserGroupSelector.vue';
import { useTableSort } from '../composables/useTableSort';
import { nasState, notify } from '../store';
import { confirmDelete } from '../modal';

const userSort = useTableSort(
  () => {
    if (nasState.includeSystemAccounts) {
      return nasState.users;
    }
    return nasState.users.filter((u) => u.uid >= 1000 && u.uid <= 10000);
  },
  {
    username: (u: User) => u.username,
    groups: (u: User) => u.primaryGroup,
    // Disabled SSH sorts first, then enabled users by number of authorized keys
    ssh: (u: User) => (u.sshEnabled ? 1 + (u.sshAuthorizedKeys || []).length : 0),
    status: (u: User) => (u.locked ? 'LOCKED' : 'ACTIVE'),
  }
);

const groupSort = useTableSort(
  () => {
    if (nasState.includeSystemGroups) {
      return nasState.groups;
    }
    return nasState.groups.filter((g) => g.gid >= 1000 && g.gid <= 10000);
  },
  {
    name: (g: Group) => g.groupName,
    gid: (g: Group) => g.gid,
    members: (g: Group) => (g.members || []).length,
  }
);

function getNextAvailableUid(): number {
  const used = new Set(nasState.users.map((u) => u.uid));
  for (let uid = 1000; uid <= 10000; uid++) {
    if (!used.has(uid)) return uid;
  }
  const max = Math.max(1000, ...Array.from(used));
  return max + 1;
}

function getNextAvailableGid(): number {
  const used = new Set(nasState.groups.map((g) => g.gid));
  for (let gid = 1000; gid <= 10000; gid++) {
    if (!used.has(gid)) return gid;
  }
  const max = Math.max(1000, ...Array.from(used));
  return max + 1;
}

const showCreateUser = ref(false);
const userForm = reactive({
  userId: '',
  fullName: '',
  uid: 1000,
  primaryGroup: '',
  supplementaryGroupsText: '',
  homeDirectory: '',
  shell: '/bin/bash',
  password: '',
  smbPassword: '',
  smbEnabled: true,
  sshEnabled: true,
  sshPasswordAuth: true,
  sshAuthorizedKeysText: '',
  locked: false,
  isAdmin: false,
});

const editingUser = ref<User | null>(null);
const editUserForm = reactive({
  fullName: '',
  primaryGroup: '',
  supplementaryGroupsText: '',
  shell: '/bin/bash',
  password: '',
  smbPassword: '',
  smbEnabled: false,
  sshEnabled: true,
  sshPasswordAuth: true,
  sshAuthorizedKeysText: '',
  locked: false,
  isAdmin: false,
});

const showCreateGroup = ref(false);
const groupForm = reactive({
  groupId: '',
  gid: 1000,
  membersText: '',
});

const duplicateUidUser = computed(() => {
  if (!userForm.uid) return null;
  return nasState.users.find((u) => u.uid === userForm.uid) || null;
});

const duplicateGidGroup = computed(() => {
  if (!groupForm.gid) return null;
  return nasState.groups.find((g) => g.gid === groupForm.gid) || null;
});

function toggleCreateUser() {
  showCreateUser.value = !showCreateUser.value;
  if (showCreateUser.value) {
    userForm.userId = '';
    userForm.fullName = '';
    userForm.uid = getNextAvailableUid();
    userForm.primaryGroup = nasState.groups.find((g) => g.gid >= 1000 && g.gid <= 10000)?.groupName || '';
    userForm.supplementaryGroupsText = '';
    userForm.homeDirectory = '';
    userForm.shell = '/bin/bash';
    userForm.password = '';
    userForm.smbPassword = '';
    userForm.sshAuthorizedKeysText = '';
    userForm.locked = false;
    userForm.isAdmin = false;
  }
}

function toggleCreateGroup() {
  showCreateGroup.value = !showCreateGroup.value;
  if (showCreateGroup.value) {
    groupForm.groupId = '';
    groupForm.gid = getNextAvailableGid();
    groupForm.membersText = '';
  }
}

const editingGroup = ref<Group | null>(null);
const editGroupMembersText = ref('');

async function refreshIdentity() {
  const [users, groups] = await Promise.all([
    services.identity.listUsers(nasState.includeSystemAccounts),
    services.identity.listGroups(nasState.includeSystemGroups),
  ]);
  nasState.users = users;
  nasState.groups = groups;
}

async function handleCreateUser() {
  if (duplicateUidUser.value) {
    notify('error', `UID ${userForm.uid} is already used by user "${duplicateUidUser.value.username}".`);
    return;
  }
  if (userForm.uid < 1000 || userForm.uid > 10000) {
    notify('error', 'UID must be between 1000 and 10000.');
    return;
  }
  try {
    const supplementaryGroups = userForm.supplementaryGroupsText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    const sshAuthorizedKeys = userForm.sshAuthorizedKeysText
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean);

    const created = await services.identity.createUser(userForm.userId, {
      uid: userForm.uid,
      primaryGroup: userForm.primaryGroup,
      supplementaryGroups,
      fullName: userForm.fullName,
      homeDirectory: userForm.homeDirectory || `/home/${userForm.userId.trim()}`,
      shell: userForm.shell,
      password: userForm.password,
      sshEnabled: userForm.sshEnabled,
      sshPasswordAuth: userForm.sshPasswordAuth,
      sshAuthorizedKeys,
      locked: userForm.locked,
      isAdmin: userForm.isAdmin,
      smbEnabled: userForm.smbEnabled,
      smbPassword: userForm.smbPassword || userForm.password,
    });
    notify('success', `Created Linux user ${created.username} (UID ${created.uid})`);
    userForm.userId = '';
    userForm.fullName = '';
    userForm.password = '';
    userForm.smbPassword = '';
    userForm.sshAuthorizedKeysText = '';
    showCreateUser.value = false;
    await refreshIdentity();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create user');
  }
}

function startEditUser(u: User) {
  editingUser.value = u;
  editUserForm.fullName = u.fullName;
  editUserForm.primaryGroup = u.primaryGroup;
  editUserForm.supplementaryGroupsText = (u.supplementaryGroups || []).join(', ');
  editUserForm.shell = u.shell || '/bin/bash';
  editUserForm.password = '';
  editUserForm.smbPassword = '';
  editUserForm.smbEnabled = Boolean(u.smbEnabled);
  editUserForm.sshEnabled = u.sshEnabled;
  editUserForm.sshPasswordAuth = u.sshPasswordAuth;
  editUserForm.sshAuthorizedKeysText = (u.sshAuthorizedKeys || []).join('\n');
  editUserForm.locked = u.locked;
  editUserForm.isAdmin = u.isAdmin;
}

async function handleSaveUserEdit() {
  if (!editingUser.value) return;
  try {
    const supplementaryGroups = editUserForm.supplementaryGroupsText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    const sshAuthorizedKeys = editUserForm.sshAuthorizedKeysText
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean);

    const mask = [
      'full_name',
      'primary_group',
      'supplementary_groups',
      'shell',
      'ssh_enabled',
      'ssh_password_auth',
      'ssh_authorized_keys',
      'locked',
      'is_admin',
      'smb_enabled',
    ];
    const updates: Partial<User> = {
      fullName: editUserForm.fullName,
      primaryGroup: editUserForm.primaryGroup,
      supplementaryGroups,
      shell: editUserForm.shell,
      sshEnabled: editUserForm.sshEnabled,
      sshPasswordAuth: editUserForm.sshPasswordAuth,
      sshAuthorizedKeys,
      locked: editUserForm.locked,
      isAdmin: editUserForm.isAdmin,
      smbEnabled: editUserForm.smbEnabled,
    };
    if (editUserForm.password) {
      updates.password = editUserForm.password;
      mask.push('password');
    }
    if (editUserForm.smbPassword) {
      updates.smbPassword = editUserForm.smbPassword;
      mask.push('smb_password');
    }

    await services.identity.updateUser(editingUser.value.name, updates, mask);
    notify('success', `Updated Linux user & SSH policy for ${editingUser.value.username}`);
    editingUser.value = null;
    await refreshIdentity();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update user');
  }
}

async function handleDeleteUser(u: User) {
  const name = u.name;
  const shortName = u.username || name.replace(/^users\//, '');
  const res = await confirmDelete({
    kind: 'User',
    name: shortName,
    details: 'The Linux user account will be permanently removed.',
    confirmText: 'Delete User',
    checkbox: {
      label: 'Also permanently remove user home directory and files',
      checked: true,
      hint: 'If unchecked, files belonging to this UID will remain on the filesystem.',
    },
  });
  if (!res.confirmed) return;

  const removeHome = !!res.checkboxChecked;

  try {
    await services.identity.deleteUser(name, removeHome);
    notify('info', `Deleted user ${shortName}`);
    await refreshIdentity();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete user');
  }
}

async function handleCreateGroup() {
  if (duplicateGidGroup.value) {
    notify('error', `GID ${groupForm.gid} is already used by group "${duplicateGidGroup.value.groupName}".`);
    return;
  }
  if (groupForm.gid < 1000 || groupForm.gid > 10000) {
    notify('error', 'GID must be between 1000 and 10000.');
    return;
  }
  try {
    const members = groupForm.membersText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    const created = await services.identity.createGroup(groupForm.groupId, {
      gid: groupForm.gid,
      members,
    });
    notify('success', `Created Linux group ${created.groupName} (GID ${created.gid})`);
    groupForm.groupId = '';
    groupForm.membersText = '';
    showCreateGroup.value = false;
    await refreshIdentity();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create group');
  }
}

function startEditGroup(g: Group) {
  editingGroup.value = g;
  editGroupMembersText.value = (g.members || []).join(', ');
}

async function handleSaveGroupEdit() {
  if (!editingGroup.value) return;
  try {
    const members = editGroupMembersText.value
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    await services.identity.updateGroup(editingGroup.value.name, { members }, ['members']);
    notify('success', `Updated group ${editingGroup.value.groupName}`);
    editingGroup.value = null;
    await refreshIdentity();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update group');
  }
}

async function handleDeleteGroup(g: Group) {
  const name = g.name;
  const shortName = g.groupName || name.replace(/^groups\//, '');
  const res = await confirmDelete({
    kind: 'Group',
    name: shortName,
    details: 'The group will be removed from the Linux host.',
    confirmText: 'Delete Group',
  });
  if (!res.confirmed) return;

  try {
    await services.identity.deleteGroup(name);
    notify('info', `Deleted group ${shortName}`);
    await refreshIdentity();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete group');
  }
}
</script>

<template>
  <div style="display: flex; flex-direction: column; gap: 20px">
    <!-- Linux Users + SSH Access Card -->
    <div class="card">
      <div class="card-header">
        <div>
          <h2 class="card-title" style="font-size: 17px">Users</h2>
        </div>

        <div style="display: flex; align-items: center; gap: 12px">
          <label style="font-size: 12px; display: flex; align-items: center; gap: 6px">
            <input
              v-model="nasState.includeSystemAccounts"
              type="checkbox"
              @change="refreshIdentity"
            />
            Show System Accounts
          </label>
          <button class="btn btn-primary" @click="toggleCreateUser">
            {{ showCreateUser ? 'Cancel' : '+ Create User' }}
          </button>
        </div>
      </div>

      <!-- Create User Form -->
      <form
        v-if="showCreateUser"
        @submit.prevent="handleCreateUser"
        style="
          background: var(--bg-base);
          border: 1px solid var(--border-strong);
          border-radius: 8px;
          padding: 16px;
          margin-bottom: 18px;
        "
      >
        <div class="form-grid">
          <div class="form-group">
            <label>Username</label>
            <input v-model="userForm.userId" class="input mono" placeholder="devops" required />
          </div>
          <div class="form-group">
            <label>Full Name</label>
            <input v-model="userForm.fullName" class="input" placeholder="DevOps Engineer" />
          </div>
          <div class="form-group">
            <label>UID</label>
            <input
              v-model.number="userForm.uid"
              type="number"
              min="1000"
              max="10000"
              class="input mono"
              :class="{ 'input-error': duplicateUidUser }"
              required
            />
            <div v-if="duplicateUidUser" style="color: var(--danger); font-size: 11px; margin-top: 4px">
              UID {{ userForm.uid }} is already in use by {{ duplicateUidUser.username }}
            </div>
          </div>
          <div class="form-group">
            <label>Primary Group</label>
            <UserGroupSelector
              v-model="userForm.primaryGroup"
              type="group"
              placeholder="Select primary group..."
            />
          </div>
          <div class="form-group">
            <label>Supplementary Groups (comma-separated)</label>
            <UserGroupSelector
              v-model="userForm.supplementaryGroupsText"
              type="group"
              :multiple="true"
              placeholder="Select supplementary groups..."
            />
          </div>
          <div class="form-group">
            <label>Login Shell</label>
            <select v-model="userForm.shell" class="select">
              <option value="/bin/bash">/bin/bash</option>
              <option value="/bin/zsh">/bin/zsh</option>
              <option value="/bin/sh">/bin/sh</option>
              <option value="/usr/sbin/nologin">/usr/sbin/nologin (No Shell)</option>
            </select>
          </div>
          <div class="form-group">
            <label>Password</label>
            <input v-model="userForm.password" type="password" class="input" placeholder="••••••••" />
          </div>
          <div class="form-group">
            <label>Samba / SMB Password</label>
            <input v-model="userForm.smbPassword" type="password" class="input" placeholder="Leave blank to match system password" />
          </div>
        </div>

        <div class="form-group" style="margin-top: 12px">
          <label>SSH Public Keys (one per line)</label>
          <textarea
            v-model="userForm.sshAuthorizedKeysText"
            rows="2"
            class="textarea mono"
            placeholder="ssh-ed25519 AAAAC3NzaC... user@host"
          ></textarea>
        </div>

        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 12px; flex-wrap: wrap; gap: 12px">
          <div style="display: flex; gap: 16px; font-size: 12.5px; flex-wrap: wrap">
            <label style="display: flex; align-items: center; gap: 6px">
              <input v-model="userForm.smbEnabled" type="checkbox" /> Enable Samba / SMB
            </label>
            <label style="display: flex; align-items: center; gap: 6px">
              <input v-model="userForm.sshEnabled" type="checkbox" /> Enable SSH Login
            </label>
            <label style="display: flex; align-items: center; gap: 6px">
              <input v-model="userForm.sshPasswordAuth" type="checkbox" /> Allow SSH Password Auth
            </label>
            <label style="display: flex; align-items: center; gap: 6px">
              <input v-model="userForm.isAdmin" type="checkbox" /> NAS Admin
            </label>
          </div>
          <button type="submit" class="btn btn-primary">Create User</button>
        </div>
      </form>

      <!-- Edit User & SSH Keys Modal/Panel -->
      <div
        v-if="editingUser"
        style="
          background: rgba(56, 189, 248, 0.08);
          border: 1px solid var(--accent-primary);
          border-radius: 8px;
          padding: 16px;
          margin-bottom: 16px;
        "
      >
        <div style="font-weight: 600; margin-bottom: 10px">
          Edit {{ editingUser.username }}
        </div>
        <div class="form-grid">
          <div class="form-group">
            <label>Full Name</label>
            <input v-model="editUserForm.fullName" class="input" />
          </div>
          <div class="form-group">
            <label>Primary Group</label>
            <UserGroupSelector
              v-model="editUserForm.primaryGroup"
              type="group"
            />
          </div>
          <div class="form-group">
            <label>Supplementary Groups</label>
            <UserGroupSelector
              v-model="editUserForm.supplementaryGroupsText"
              type="group"
              :multiple="true"
            />
          </div>
          <div class="form-group">
            <label>Login Shell</label>
            <select v-model="editUserForm.shell" class="select">
              <option value="/bin/bash">/bin/bash</option>
              <option value="/bin/zsh">/bin/zsh</option>
              <option value="/bin/sh">/bin/sh</option>
              <option value="/usr/sbin/nologin">/usr/sbin/nologin</option>
            </select>
          </div>
          <div class="form-group">
            <label>Home Directory</label>
            <input :value="editingUser.homeDirectory" class="input mono" readonly />
          </div>
          <div class="form-group">
            <label>New Password (leave blank to keep current)</label>
            <input v-model="editUserForm.password" type="password" class="input" />
          </div>
          <div class="form-group">
            <label>New Samba / SMB Password</label>
            <input v-model="editUserForm.smbPassword" type="password" class="input" placeholder="Leave blank to keep current" />
          </div>
        </div>

        <div class="form-group" style="margin-top: 10px">
          <label>SSH Public Keys (one per line)</label>
          <textarea
            v-model="editUserForm.sshAuthorizedKeysText"
            rows="3"
            class="textarea mono"
            placeholder="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... user@host"
          ></textarea>
        </div>

        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 12px">
          <div style="display: flex; gap: 16px; font-size: 12px">
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="editUserForm.sshEnabled" type="checkbox" /> SSH Enabled
            </label>
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="editUserForm.sshPasswordAuth" type="checkbox" /> SSH Password Auth
            </label>
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="editUserForm.smbEnabled" type="checkbox" /> SMB Access
            </label>
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="editUserForm.locked" type="checkbox" /> Lock Account
            </label>
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="editUserForm.isAdmin" type="checkbox" /> NAS Admin
            </label>
          </div>
          <div style="display: flex; gap: 8px">
            <button class="btn btn-sm" @click="editingUser = null">Cancel</button>
            <button class="btn btn-primary btn-sm" @click="handleSaveUserEdit">
              Save
            </button>
          </div>
        </div>
      </div>

      <!-- Users Table -->
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <SortableTh :sort="userSort" sort-key="username">Username / UID</SortableTh>
              <SortableTh :sort="userSort" sort-key="groups">Groups</SortableTh>
              <SortableTh :sort="userSort" sort-key="ssh">SSH Access & Keys</SortableTh>
              <SortableTh :sort="userSort" sort-key="status">Status</SortableTh>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <!-- Moving Skeleton Placeholders when Loading -->
            <template v-if="nasState.loading && nasState.users.length === 0">
              <tr v-for="r in 4" :key="r">
                <td>
                  <div style="display: flex; flex-direction: column; gap: 4px">
                    <SkeletonBox width="110px" height="15px" />
                    <SkeletonBox width="130px" height="11px" />
                  </div>
                </td>
                <td>
                  <div style="display: flex; flex-direction: column; gap: 4px">
                    <SkeletonBox width="120px" height="13px" />
                    <SkeletonBox width="160px" height="11px" />
                  </div>
                </td>
                <td><SkeletonBox width="90px" height="20px" rounded="10px" /></td>
                <td><SkeletonBox width="65px" height="20px" rounded="10px" /></td>
                <td>
                  <div style="display: inline-flex; gap: 6px">
                    <SkeletonBox width="26px" height="26px" rounded="4px" />
                    <SkeletonBox width="26px" height="26px" rounded="4px" />
                  </div>
                </td>
              </tr>
            </template>

            <tr v-else-if="nasState.users.length === 0">
              <td colspan="5" style="text-align: center; color: var(--text-secondary); padding: 32px">
                No users.
              </td>
            </tr>

            <tr v-for="u in userSort.sorted.value" :key="u.name">
              <td>
                <div style="font-weight: 600">
                  {{ u.username }}
                  <span v-if="u.isAdmin" class="badge badge-info" style="margin-left: 4px">
                    Admin
                  </span>
                </div>
                <div class="mono" style="font-size: 11px; color: var(--text-muted)">
                  UID {{ u.uid }} · {{ u.fullName || u.name }}
                </div>
              </td>
              <td class="mono" style="font-size: 12px">
                <div>Primary: {{ u.primaryGroup }} ({{ u.primaryGid }})</div>
                <div style="font-size: 11px; color: var(--text-secondary)">
                  Groups: {{ (u.supplementaryGroups || []).join(', ') || '—' }}
                </div>
              </td>
              <td>
                <span class="badge" :class="u.sshEnabled ? 'badge-success' : 'badge-neutral'">
                  SSH: {{ u.sshEnabled ? 'ENABLED' : 'DISABLED' }}
                </span>
                <span
                  v-if="u.sshEnabled"
                  class="badge badge-info"
                  style="margin-left: 4px"
                >
                  {{ (u.sshAuthorizedKeys || []).length }} Keys
                </span>
                <div
                  v-if="u.sshEnabled"
                  style="font-size: 11px; color: var(--text-secondary); margin-top: 2px"
                >
                  Password Auth: {{ u.sshPasswordAuth ? 'Allowed' : 'Keys Only' }}
                </div>
              </td>
              <td>
                <span class="badge" :class="u.locked ? 'badge-danger' : 'badge-success'">
                  {{ u.locked ? 'LOCKED' : 'ACTIVE' }}
                </span>
                <span class="badge" :class="u.smbEnabled ? 'badge-purple' : 'badge-neutral'" style="margin-left: 4px">
                  SMB: {{ u.smbEnabled ? 'ON' : 'OFF' }}
                </span>
              </td>
              <td>
                <div style="display: flex; gap: 6px">
                  <button
                    class="btn btn-sm btn-icon"
                    title="Edit User & SSH Keys"
                    aria-label="Edit User & SSH Keys"
                    @click="startEditUser(u)"
                  >
                    <MdiIcon :path="mdiPencil" size="14" />
                  </button>
                  <button
                    class="btn btn-danger btn-sm btn-icon"
                    title="Delete User"
                    aria-label="Delete User"
                    @click="handleDeleteUser(u)"
                  >
                    <MdiIcon :path="mdiDelete" size="14" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Linux Groups Card -->
    <div class="card">
      <div class="card-header">
        <div>
          <h3 class="card-title">Groups</h3>
        </div>

        <div style="display: flex; align-items: center; gap: 12px">
          <label style="font-size: 12px; display: flex; align-items: center; gap: 6px">
            <input
              v-model="nasState.includeSystemGroups"
              type="checkbox"
              @change="refreshIdentity"
            />
            Show System Groups
          </label>
          <button class="btn btn-primary btn-sm" @click="toggleCreateGroup">
            {{ showCreateGroup ? 'Cancel' : '+ Create Group' }}
          </button>
        </div>
      </div>

      <form
        v-if="showCreateGroup"
        @submit.prevent="handleCreateGroup"
        style="
          background: var(--bg-base);
          border: 1px solid var(--border-strong);
          border-radius: 8px;
          padding: 14px;
          margin-bottom: 14px;
        "
      >
        <div class="form-grid">
          <div class="form-group">
            <label>Group Name</label>
            <input v-model="groupForm.groupId" class="input mono" placeholder="media-editors" required />
          </div>
          <div class="form-group">
            <label>GID</label>
            <input
              v-model.number="groupForm.gid"
              type="number"
              min="1000"
              max="10000"
              class="input mono"
              :class="{ 'input-error': duplicateGidGroup }"
              required
            />
            <div v-if="duplicateGidGroup" style="color: var(--danger); font-size: 11px; margin-top: 4px">
              GID {{ groupForm.gid }} is already in use by group "{{ duplicateGidGroup.groupName }}"
            </div>
          </div>
          <div class="form-group">
            <label>Members (comma-separated usernames)</label>
            <UserGroupSelector
              v-model="groupForm.membersText"
              type="user"
              :multiple="true"
              placeholder="Select group members..."
            />
          </div>
        </div>
        <div style="margin-top: 10px; display: flex; justify-content: flex-end">
          <button type="submit" class="btn btn-primary btn-sm">Create Group</button>
        </div>
      </form>

      <!-- Edit Group Inline -->
      <div
        v-if="editingGroup"
        style="
          background: rgba(56, 189, 248, 0.08);
          border: 1px solid var(--accent-primary);
          border-radius: 8px;
          padding: 12px;
          margin-bottom: 12px;
          display: flex;
          align-items: center;
          gap: 10px;
        "
      >
        <span class="mono" style="font-size: 12px">
          Members of {{ editingGroup.groupName }}:
        </span>
        <div style="flex: 1">
          <UserGroupSelector
            v-model="editGroupMembersText"
            type="user"
            :multiple="true"
            placeholder="Select group members..."
          />
        </div>
        <button class="btn btn-primary btn-sm" @click="handleSaveGroupEdit">Save</button>
        <button class="btn btn-sm" @click="editingGroup = null">Cancel</button>
      </div>

      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <SortableTh :sort="groupSort" sort-key="name">Group Name</SortableTh>
              <SortableTh :sort="groupSort" sort-key="gid">GID</SortableTh>
              <SortableTh :sort="groupSort" sort-key="members">Members</SortableTh>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="g in groupSort.sorted.value" :key="g.name">
              <td class="mono" style="font-weight: 600">{{ g.groupName }}</td>
              <td class="mono">{{ g.gid }}</td>
              <td>
                <div style="display: flex; gap: 5px; flex-wrap: wrap">
                  <span
                    v-for="m in g.members"
                    :key="m"
                    class="badge badge-neutral"
                  >
                    {{ m }}
                  </span>
                  <span v-if="!g.members || g.members.length === 0" class="card-subtitle">
                    No members
                  </span>
                </div>
              </td>
              <td>
                <div style="display: flex; gap: 6px">
                  <button
                    class="btn btn-sm btn-icon"
                    title="Edit Group Members"
                    aria-label="Edit Group Members"
                    @click="startEditGroup(g)"
                  >
                    <MdiIcon :path="mdiPencil" size="14" />
                  </button>
                  <button
                    class="btn btn-danger btn-sm btn-icon"
                    title="Delete Group"
                    aria-label="Delete Group"
                    @click="handleDeleteGroup(g)"
                  >
                    <MdiIcon :path="mdiDelete" size="14" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
