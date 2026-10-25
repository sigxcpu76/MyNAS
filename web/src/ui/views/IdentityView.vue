<script setup lang="ts">
import { reactive, ref } from 'vue';
import { services } from '../../container';
import type { Group, User } from '../../domain/models';
import { askModal, confirmModal, nasState, notify } from '../store';

const showCreateUser = ref(false);
const userForm = reactive({
  userId: '',
  fullName: '',
  uid: 1005,
  primaryGroup: 'nasusers',
  supplementaryGroupsText: 'docker, kvm',
  homeDirectory: '',
  shell: '/bin/bash',
  password: '',
  sshEnabled: true,
  sshPasswordAuth: true,
  sshAuthorizedKeysText: 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAINasApplianceKey admin@nas',
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
  sshEnabled: true,
  sshPasswordAuth: true,
  sshAuthorizedKeysText: '',
  locked: false,
  isAdmin: false,
});

const showCreateGroup = ref(false);
const groupForm = reactive({
  groupId: '',
  gid: 2010,
  membersText: 'admin',
});

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
    });
    notify('success', `Created Linux user ${created.username} (UID ${created.uid})`);
    userForm.userId = '';
    userForm.password = '';
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
    };
    if (editUserForm.password) {
      updates.password = editUserForm.password;
      mask.push('password');
    }

    await services.identity.updateUser(editingUser.value.name, updates, mask);
    notify('success', `Updated Linux user & SSH policy for ${editingUser.value.username}`);
    editingUser.value = null;
    await refreshIdentity();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update user');
  }
}

async function handleDeleteUser(name: string) {
  const shortName = name.replace(/^users\//, '');
  const res = await askModal({
    title: 'Delete Linux User',
    tone: 'danger',
    message: `Are you sure you want to delete user "${shortName}"?`,
    details: 'The user account will be permanently removed from Linux PAM and /etc/passwd.',
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

async function handleDeleteGroup(name: string) {
  const shortName = name.replace(/^groups\//, '');
  const confirmed = await confirmModal({
    title: 'Delete Linux Group',
    tone: 'danger',
    message: `Are you sure you want to delete group "${shortName}"?`,
    details: 'The group will be removed from /etc/group on the Linux host.',
    confirmText: 'Delete Group',
  });
  if (!confirmed) return;

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
          <h2 class="card-title" style="font-size: 17px">
            Linux Users & OpenSSH Access Control (`users/{user}`)
          </h2>
          <p class="card-subtitle">
            Manage `/etc/passwd` accounts, UID/GID membership, login shells, SSH daemon access, and
            `~/.ssh/authorized_keys` public keys.
          </p>
        </div>

        <div style="display: flex; align-items: center; gap: 12px">
          <label style="font-size: 12px; display: flex; align-items: center; gap: 6px">
            <input
              v-model="nasState.includeSystemAccounts"
              type="checkbox"
              @change="refreshIdentity"
            />
            Show System Accounts (`uid < 1000`)
          </label>
          <button class="btn btn-primary" @click="showCreateUser = !showCreateUser">
            {{ showCreateUser ? 'Cancel' : '+ Create Linux User' }}
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
            <label>Username (`user_id`)</label>
            <input v-model="userForm.userId" class="input mono" placeholder="devops" required />
          </div>
          <div class="form-group">
            <label>Full Name (GECOS)</label>
            <input v-model="userForm.fullName" class="input" placeholder="DevOps Engineer" />
          </div>
          <div class="form-group">
            <label>UID</label>
            <input v-model.number="userForm.uid" type="number" min="1000" class="input mono" />
          </div>
          <div class="form-group">
            <label>Primary Group</label>
            <input v-model="userForm.primaryGroup" class="input mono" placeholder="nasusers" />
          </div>
          <div class="form-group">
            <label>Supplementary Groups (comma-separated)</label>
            <input v-model="userForm.supplementaryGroupsText" class="input mono" placeholder="docker, kvm, sudo" />
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
        </div>

        <div class="form-group" style="margin-top: 12px">
          <label>SSH Public Keys (`~/.ssh/authorized_keys`, one key per line)</label>
          <textarea
            v-model="userForm.sshAuthorizedKeysText"
            rows="2"
            class="textarea mono"
            placeholder="ssh-ed25519 AAAAC3NzaC... user@host"
          ></textarea>
        </div>

        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 12px; flex-wrap: wrap; gap: 12px">
          <div style="display: flex; gap: 16px; font-size: 12.5px">
            <label style="display: flex; align-items: center; gap: 6px">
              <input v-model="userForm.sshEnabled" type="checkbox" /> Enable SSH Login (`AllowUsers`)
            </label>
            <label style="display: flex; align-items: center; gap: 6px">
              <input v-model="userForm.sshPasswordAuth" type="checkbox" /> Allow SSH Password Auth
            </label>
            <label style="display: flex; align-items: center; gap: 6px">
              <input v-model="userForm.isAdmin" type="checkbox" /> NAS Admin (`sudo`)
            </label>
          </div>
          <button type="submit" class="btn btn-primary">Create User & Write SSH Keys</button>
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
          PATCH {{ editingUser.name }} — Identity, Groups & OpenSSH `authorized_keys`
        </div>
        <div class="form-grid">
          <div class="form-group">
            <label>Full Name</label>
            <input v-model="editUserForm.fullName" class="input" />
          </div>
          <div class="form-group">
            <label>Primary Group</label>
            <input v-model="editUserForm.primaryGroup" class="input mono" />
          </div>
          <div class="form-group">
            <label>Supplementary Groups</label>
            <input v-model="editUserForm.supplementaryGroupsText" class="input mono" />
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
            <label>New Password (leave blank to keep current)</label>
            <input v-model="editUserForm.password" type="password" class="input" />
          </div>
        </div>

        <div class="form-group" style="margin-top: 10px">
          <label>SSH Public Keys (`~/.ssh/authorized_keys`)</label>
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
              <input v-model="editUserForm.locked" type="checkbox" /> Lock Account (`usermod -L`)
            </label>
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="editUserForm.isAdmin" type="checkbox" /> NAS Admin
            </label>
          </div>
          <div style="display: flex; gap: 8px">
            <button class="btn btn-sm" @click="editingUser = null">Cancel</button>
            <button class="btn btn-primary btn-sm" @click="handleSaveUserEdit">
              Save User & SSH Policy
            </button>
          </div>
        </div>
      </div>

      <!-- Users Table -->
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>Username / UID</th>
              <th>Groups</th>
              <th>Home & Shell</th>
              <th>SSH Access & Keys</th>
              <th>Status</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="u in nasState.users" :key="u.name">
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
              <td class="mono" style="font-size: 12px">
                <div>{{ u.homeDirectory }}</div>
                <div style="font-size: 11px; color: var(--text-secondary)">{{ u.shell }}</div>
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
              </td>
              <td>
                <div style="display: flex; gap: 6px">
                  <button class="btn btn-sm" @click="startEditUser(u)">
                    Edit / SSH Keys
                  </button>
                  <button class="btn btn-danger btn-sm" @click="handleDeleteUser(u.name)">
                    Del
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
          <h3 class="card-title">Linux System Groups (`groups/{group}`)</h3>
          <p class="card-subtitle">Manage POSIX group GIDs and user memberships (`/etc/group`)</p>
        </div>

        <button class="btn btn-primary btn-sm" @click="showCreateGroup = !showCreateGroup">
          {{ showCreateGroup ? 'Cancel' : '+ Create Group' }}
        </button>
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
            <label>Group Name (`group_id`)</label>
            <input v-model="groupForm.groupId" class="input mono" placeholder="media-editors" required />
          </div>
          <div class="form-group">
            <label>GID</label>
            <input v-model.number="groupForm.gid" type="number" min="100" class="input mono" />
          </div>
          <div class="form-group">
            <label>Members (comma-separated usernames)</label>
            <input v-model="groupForm.membersText" class="input mono" placeholder="admin, nasuser" />
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
        <input v-model="editGroupMembersText" class="input mono" style="flex: 1" />
        <button class="btn btn-primary btn-sm" @click="handleSaveGroupEdit">Save PATCH</button>
        <button class="btn btn-sm" @click="editingGroup = null">Cancel</button>
      </div>

      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>Group Name</th>
              <th>GID</th>
              <th>Members</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="g in nasState.groups" :key="g.name">
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
                  <button class="btn btn-sm" @click="startEditGroup(g)">Edit Members</button>
                  <button class="btn btn-danger btn-sm" @click="handleDeleteGroup(g.name)">
                    Del
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
