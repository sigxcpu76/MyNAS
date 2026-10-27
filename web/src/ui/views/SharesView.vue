<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { services } from '../../container';
import type { CifsGlobalConfig, CifsShare, WebdavShare } from '../../domain/models';
import SkeletonBox from '../components/SkeletonBox.vue';
import SortableTh from '../components/SortableTh.vue';
import PathSelector from '../components/PathSelector.vue';
import UserGroupSelector from '../components/UserGroupSelector.vue';
import { useTableSort } from '../composables/useTableSort';
import { getModuleAvailability, nasState, notify } from '../store';
import { confirmDelete } from '../modal';

const cifsSort = useTableSort(
  () => nasState.cifsShares,
  {
    share: (s: CifsShare) => s.shareName,
    acl: (s: CifsShare) => (s.validUsers || []).join(', '),
    mode: (s: CifsShare) => s.readOnly,
  }
);

const webdavSort = useTableSort(
  () => nasState.webdavShares,
  {
    endpoint: (w: WebdavShare) => w.urlPrefix,
    identity: (w: WebdavShare) => (w.allowedUsers || []).join(', '),
    access: (w: WebdavShare) => w.readOnly,
  }
);

const cifsModuleStatus = computed(() => getModuleAvailability('cifs'));

// Samba [global] configuration modal
const showGlobalSettings = ref(false);
const globalForm = reactive<CifsGlobalConfig>({
  name: 'cifsGlobalConfig',
  workgroup: 'WORKGROUP',
  serverString: 'NAS Samba Server',
  security: 'user',
  mapToGuest: 'Bad User',
  guestAccount: 'nobody',
  maxLogSize: 50,
  disableSpoolss: true,
  fruitAapl: true,
  wsddEnabled: true,
});

const showCreateCifs = ref(false);
const cifsForm = reactive({
  shareId: '',
  path: '/mnt/tank/media',
  comment: 'Shared Storage',
  validUsersText: '',
  validGroupsText: '',
  readListText: '',
  writeListText: '',
  forceUser: '',
  forceGroup: '',
  guestAccount: 'nobody',
  readOnly: false,
  guestOk: false,
  guestOnly: false,
  browseable: true,
  createMask: '0664',
  directoryMask: '0775',
  forceCreateMode: '0664',
  forceDirectoryMode: '0775',
  shadowCopy: true,
  timeMachine: false,
  recycleBin: true,
  enabled: true,
});

const editingCifs = ref<CifsShare | null>(null);
const editCifsForm = reactive({
  path: '',
  comment: '',
  validUsersText: '',
  validGroupsText: '',
  readListText: '',
  writeListText: '',
  forceUser: '',
  forceGroup: '',
  guestAccount: '',
  readOnly: false,
  guestOk: false,
  guestOnly: false,
  browseable: true,
  createMask: '0664',
  directoryMask: '0775',
  forceCreateMode: '0664',
  forceDirectoryMode: '0775',
  shadowCopy: false,
  timeMachine: false,
  recycleBin: false,
  enabled: true,
});

const showCreateWebdav = ref(false);
const webdavForm = reactive({
  shareId: '',
  path: '/mnt/tank/documents',
  description: 'Documents',
  allowedUsersText: 'admin, nasuser',
  allowedGroupsText: 'nasusers',
  readOnly: false,
  enabled: true,
});

const editingWebdav = ref<WebdavShare | null>(null);
const editWebdavForm = reactive({
  path: '',
  description: '',
  allowedUsersText: '',
  allowedGroupsText: '',
  readOnly: false,
  enabled: true,
});

async function refreshShares() {
  try {
    const [cifs, webdav, globalCfg] = await Promise.all([
      services.shares.listCifsShares(),
      services.shares.listWebdavShares(),
      services.shares.getCifsGlobalConfig().catch(() => null),
    ]);
    nasState.cifsShares = cifs;
    nasState.webdavShares = webdav;
    if (globalCfg) {
      nasState.cifsGlobalConfig = globalCfg;
      Object.assign(globalForm, globalCfg);
    }
  } catch (err: any) {
    notify('error', err?.message || 'Failed to refresh shares');
  }
}

onMounted(() => {
  refreshShares();
});

async function handleSaveGlobalSettings() {
  try {
    const updated = await services.shares.updateCifsGlobalConfig({
      workgroup: globalForm.workgroup.trim() || 'WORKGROUP',
      serverString: globalForm.serverString.trim(),
      security: globalForm.security.trim() || 'user',
      mapToGuest: globalForm.mapToGuest,
      guestAccount: globalForm.guestAccount.trim() || 'nobody',
      maxLogSize: Number(globalForm.maxLogSize) || 50,
      disableSpoolss: globalForm.disableSpoolss,
      fruitAapl: globalForm.fruitAapl,
      wsddEnabled: globalForm.wsddEnabled,
    });
    nasState.cifsGlobalConfig = updated;
    Object.assign(globalForm, updated);
    notify('success', 'Samba [global] settings saved and applied');
    showGlobalSettings.value = false;
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update Samba settings');
  }
}

async function handleCreateCifs() {
  try {
    const validUsers = cifsForm.validUsersText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    const validGroups = cifsForm.validGroupsText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    const readList = cifsForm.readListText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    const writeList = cifsForm.writeListText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);

    const created = await services.shares.createCifsShare(cifsForm.shareId, {
      path: cifsForm.path.trim(),
      comment: cifsForm.comment.trim(),
      validUsers,
      validGroups,
      readList,
      writeList,
      forceUser: cifsForm.forceUser.trim(),
      forceGroup: cifsForm.forceGroup.trim(),
      guestAccount: cifsForm.guestAccount.trim(),
      readOnly: cifsForm.readOnly,
      guestOk: cifsForm.guestOk,
      guestOnly: cifsForm.guestOnly,
      browseable: cifsForm.browseable,
      createMask: cifsForm.createMask.trim() || '0664',
      directoryMask: cifsForm.directoryMask.trim() || '0775',
      forceCreateMode: cifsForm.forceCreateMode.trim() || '0664',
      forceDirectoryMode: cifsForm.forceDirectoryMode.trim() || '0775',
      shadowCopy: cifsForm.shadowCopy,
      timeMachine: cifsForm.timeMachine,
      recycleBin: cifsForm.recycleBin,
      enabled: cifsForm.enabled,
    });
    notify('success', `Created CIFS share [${created.shareName}]`);
    cifsForm.shareId = '';
    showCreateCifs.value = false;
    await refreshShares();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create CIFS share');
  }
}

function startEditCifs(share: CifsShare) {
  editingCifs.value = share;
  editCifsForm.path = share.path;
  editCifsForm.comment = share.comment;
  editCifsForm.validUsersText = (share.validUsers || []).join(', ');
  editCifsForm.validGroupsText = (share.validGroups || []).join(', ');
  editCifsForm.readListText = (share.readList || []).join(', ');
  editCifsForm.writeListText = (share.writeList || []).join(', ');
  editCifsForm.forceUser = share.forceUser || '';
  editCifsForm.forceGroup = share.forceGroup || '';
  editCifsForm.guestAccount = share.guestAccount || '';
  editCifsForm.readOnly = share.readOnly;
  editCifsForm.guestOk = share.guestOk;
  editCifsForm.guestOnly = share.guestOnly;
  editCifsForm.browseable = share.browseable;
  editCifsForm.createMask = share.createMask || '0664';
  editCifsForm.directoryMask = share.directoryMask || '0775';
  editCifsForm.forceCreateMode = share.forceCreateMode || '0664';
  editCifsForm.forceDirectoryMode = share.forceDirectoryMode || '0775';
  editCifsForm.shadowCopy = share.shadowCopy;
  editCifsForm.timeMachine = share.timeMachine;
  editCifsForm.recycleBin = share.recycleBin;
  editCifsForm.enabled = share.enabled;
}

async function handleSaveCifsEdit() {
  if (!editingCifs.value) return;
  try {
    const validUsers = editCifsForm.validUsersText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    const validGroups = editCifsForm.validGroupsText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    const readList = editCifsForm.readListText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    const writeList = editCifsForm.writeListText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);

    await services.shares.updateCifsShare(
      editingCifs.value.name,
      {
        path: editCifsForm.path.trim(),
        comment: editCifsForm.comment.trim(),
        validUsers,
        validGroups,
        readList,
        writeList,
        forceUser: editCifsForm.forceUser.trim(),
        forceGroup: editCifsForm.forceGroup.trim(),
        guestAccount: editCifsForm.guestAccount.trim(),
        readOnly: editCifsForm.readOnly,
        guestOk: editCifsForm.guestOk,
        guestOnly: editCifsForm.guestOnly,
        browseable: editCifsForm.browseable,
        createMask: editCifsForm.createMask.trim(),
        directoryMask: editCifsForm.directoryMask.trim(),
        forceCreateMode: editCifsForm.forceCreateMode.trim(),
        forceDirectoryMode: editCifsForm.forceDirectoryMode.trim(),
        shadowCopy: editCifsForm.shadowCopy,
        timeMachine: editCifsForm.timeMachine,
        recycleBin: editCifsForm.recycleBin,
        enabled: editCifsForm.enabled,
      },
      [
        'path',
        'comment',
        'valid_users',
        'valid_groups',
        'read_list',
        'write_list',
        'force_user',
        'force_group',
        'guest_account',
        'read_only',
        'guest_ok',
        'guest_only',
        'browseable',
        'create_mask',
        'directory_mask',
        'force_create_mode',
        'force_directory_mode',
        'shadow_copy',
        'time_machine',
        'recycle_bin',
        'enabled',
      ]
    );
    notify('success', `Updated CIFS share [${editingCifs.value.shareName}]`);
    editingCifs.value = null;
    await refreshShares();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update CIFS share');
  }
}

async function handleDeleteCifs(share: CifsShare) {
  const name = share.name;
  const shortName = share.shareName || name.replace(/^cifsShares\//, '');
  const res = await confirmDelete({
    kind: 'SMB Share',
    name: shortName,
    details: 'The SMB export will be removed; files on disk remain intact.',
    confirmText: 'Delete Share',
  });
  if (!res.confirmed) return;

  try {
    await services.shares.deleteCifsShare(name);
    notify('info', `Deleted CIFS share ${shortName}`);
    await refreshShares();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete CIFS share');
  }
}

async function handleCreateWebdav() {
  try {
    const allowedUsers = webdavForm.allowedUsersText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    const allowedGroups = webdavForm.allowedGroupsText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);

    const created = await services.shares.createWebdavShare(webdavForm.shareId, {
      path: webdavForm.path,
      description: webdavForm.description,
      allowedUsers,
      allowedGroups,
      readOnly: webdavForm.readOnly,
      enabled: webdavForm.enabled,
    });
    notify('success', `Published WebDAV endpoint ${created.urlPrefix}`);
    webdavForm.shareId = '';
    showCreateWebdav.value = false;
    await refreshShares();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create WebDAV share');
  }
}

function startEditWebdav(share: WebdavShare) {
  editingWebdav.value = share;
  editWebdavForm.path = share.path;
  editWebdavForm.description = share.description;
  editWebdavForm.allowedUsersText = (share.allowedUsers || []).join(', ');
  editWebdavForm.allowedGroupsText = (share.allowedGroups || []).join(', ');
  editWebdavForm.readOnly = share.readOnly;
  editWebdavForm.enabled = share.enabled;
}

async function handleSaveWebdavEdit() {
  if (!editingWebdav.value) return;
  try {
    const allowedUsers = editWebdavForm.allowedUsersText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    const allowedGroups = editWebdavForm.allowedGroupsText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);

    await services.shares.updateWebdavShare(
      editingWebdav.value.name,
      {
        path: editWebdavForm.path,
        description: editWebdavForm.description,
        allowedUsers,
        allowedGroups,
        readOnly: editWebdavForm.readOnly,
        enabled: editWebdavForm.enabled,
      },
      ['path', 'description', 'allowed_users', 'allowed_groups', 'read_only', 'enabled']
    );
    notify('success', `Updated WebDAV share ${editingWebdav.value.urlPrefix}`);
    editingWebdav.value = null;
    await refreshShares();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update WebDAV share');
  }
}

async function handleDeleteWebdav(share: WebdavShare) {
  const name = share.name;
  const shortName = name.replace(/^webdavShares\//, '');
  const res = await confirmDelete({
    kind: 'WebDAV Share',
    name: share.urlPrefix || shortName,
    details: 'The WebDAV endpoint will be removed; files on disk remain intact.',
    confirmText: 'Delete Share',
  });
  if (!res.confirmed) return;

  try {
    await services.shares.deleteWebdavShare(name);
    notify('info', `Deleted WebDAV share ${shortName}`);
    await refreshShares();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete WebDAV share');
  }
}
</script>

<template>
  <div class="grid-2">
    <!-- CIFS / Samba Shares Card -->
    <div class="card">
      <div class="card-header">
        <div>
          <h3 class="card-title">SMB Shares</h3>
        </div>
        <div style="display: flex; gap: 8px">
          <button
            class="btn btn-sm"
            :disabled="!cifsModuleStatus.enabled"
            @click="showGlobalSettings = true"
          >
            ⚙ Samba Settings
          </button>
          <button
            class="btn btn-primary btn-sm"
            :disabled="!cifsModuleStatus.enabled"
            @click="showCreateCifs = !showCreateCifs"
          >
            {{ !cifsModuleStatus.enabled ? 'CIFS Disabled' : showCreateCifs ? 'Cancel' : '+ Add SMB Share' }}
          </button>
        </div>
      </div>

      <div
        v-if="!cifsModuleStatus.enabled"
        style="
          padding: 12px;
          margin-bottom: 12px;
          border-radius: 8px;
          background: var(--danger-bg);
          border: 1px solid var(--danger-border);
          color: var(--danger-text);
          font-size: 12px;
        "
      >
        CIFS / Samba module is disabled because the underlying <code>smbd</code> daemon binary was not detected on the host.
      </div>

      <!-- Create SMB Share Form -->
      <form
        v-if="showCreateCifs"
        @submit.prevent="handleCreateCifs"
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
            <label>Share Name</label>
            <input v-model="cifsForm.shareId" class="input mono" placeholder="media" required />
          </div>
          <div class="form-group">
            <label>Filesystem Path</label>
            <PathSelector
              v-model="cifsForm.path"
              placeholder="/mnt/tank/media"
              required
            />
          </div>
          <div class="form-group">
            <label>Comment / Description</label>
            <input v-model="cifsForm.comment" class="input" placeholder="Media Library" />
          </div>
          <div class="form-group">
            <label>Valid Users (comma-separated)</label>
            <UserGroupSelector
              v-model="cifsForm.validUsersText"
              type="user"
              :multiple="true"
              placeholder="Select users..."
            />
          </div>
          <div class="form-group">
            <label>Valid Groups (comma-separated)</label>
            <UserGroupSelector
              v-model="cifsForm.validGroupsText"
              type="group"
              :multiple="true"
              placeholder="Select groups..."
            />
          </div>
          <div class="form-group">
            <label>Force User / Force Group</label>
            <div style="display: flex; gap: 6px">
              <div style="width: 50%">
                <UserGroupSelector
                  v-model="cifsForm.forceUser"
                  type="user"
                  placeholder="user (optional)"
                />
              </div>
              <div style="width: 50%">
                <UserGroupSelector
                  v-model="cifsForm.forceGroup"
                  type="group"
                  placeholder="group (optional)"
                />
              </div>
            </div>
          </div>
          <div class="form-group">
            <label>Read-Only List / Write List</label>
            <div style="display: flex; gap: 6px">
              <div style="width: 50%">
                <UserGroupSelector
                  v-model="cifsForm.readListText"
                  type="user"
                  :multiple="true"
                  placeholder="read-only users"
                />
              </div>
              <div style="width: 50%">
                <UserGroupSelector
                  v-model="cifsForm.writeListText"
                  type="user"
                  :multiple="true"
                  placeholder="write users"
                />
              </div>
            </div>
          </div>
          <div class="form-group">
            <label>Create / Directory Mask</label>
            <div style="display: flex; gap: 6px">
              <input v-model="cifsForm.createMask" class="input mono" placeholder="0664" style="width: 50%" />
              <input v-model="cifsForm.directoryMask" class="input mono" placeholder="0775" style="width: 50%" />
            </div>
          </div>
          <div class="form-group">
            <label>Guest User Mapping</label>
            <UserGroupSelector
              v-model="cifsForm.guestAccount"
              type="user"
              placeholder="nobody"
            />
          </div>
        </div>

        <div style="display: flex; flex-direction: column; gap: 8px; margin-top: 12px; padding-top: 10px; border-top: 1px solid var(--border-color)">
          <div style="display: flex; gap: 14px; font-size: 12px; flex-wrap: wrap">
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="cifsForm.browseable" type="checkbox" /> Browseable (Visible in Network)
            </label>
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="cifsForm.readOnly" type="checkbox" /> Read-Only
            </label>
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="cifsForm.guestOk" type="checkbox" /> Guest OK
            </label>
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="cifsForm.guestOnly" type="checkbox" /> Guest Only
            </label>
          </div>
          <div style="display: flex; gap: 14px; font-size: 12px; flex-wrap: wrap">
            <label style="display: flex; align-items: center; gap: 5px" title="Exposes ZFS snapshots as Windows Previous Versions (vfs_shadow_copy2)">
              <input v-model="cifsForm.shadowCopy" type="checkbox" /> ZFS Snapshots (Shadow Copy)
            </label>
            <label style="display: flex; align-items: center; gap: 5px" title="Apple Time Machine backup target (vfs_fruit)">
              <input v-model="cifsForm.timeMachine" type="checkbox" /> macOS Time Machine
            </label>
            <label style="display: flex; align-items: center; gap: 5px" title="Moves deleted files to .recycle instead of permanent deletion">
              <input v-model="cifsForm.recycleBin" type="checkbox" /> Recycle Bin
            </label>
          </div>
        </div>

        <div style="display: flex; justify-content: flex-end; margin-top: 12px">
          <button type="submit" class="btn btn-primary btn-sm">Create SMB Share</button>
        </div>
      </form>

      <!-- Edit CIFS Inline Panel -->
      <div
        v-if="editingCifs"
        style="
          background: rgba(56, 189, 248, 0.08);
          border: 1px solid var(--accent-primary);
          border-radius: 8px;
          padding: 12px;
          margin-bottom: 14px;
        "
      >
        <div style="font-weight: 600; margin-bottom: 8px; font-size: 13px">
          Edit SMB Share: {{ editingCifs.shareName }}
        </div>
        <div class="form-grid">
          <div class="form-group">
            <label>Path</label>
            <PathSelector
              v-model="editCifsForm.path"
              placeholder="/mnt/tank/media"
              required
            />
          </div>
          <div class="form-group">
            <label>Comment</label>
            <input v-model="editCifsForm.comment" class="input" />
          </div>
          <div class="form-group">
            <label>Valid Users</label>
            <UserGroupSelector
              v-model="editCifsForm.validUsersText"
              type="user"
              :multiple="true"
              placeholder="Select users..."
            />
          </div>
          <div class="form-group">
            <label>Valid Groups</label>
            <UserGroupSelector
              v-model="editCifsForm.validGroupsText"
              type="group"
              :multiple="true"
              placeholder="Select groups..."
            />
          </div>
          <div class="form-group">
            <label>Force User / Force Group</label>
            <div style="display: flex; gap: 6px">
              <div style="width: 50%">
                <UserGroupSelector
                  v-model="editCifsForm.forceUser"
                  type="user"
                  placeholder="user (optional)"
                />
              </div>
              <div style="width: 50%">
                <UserGroupSelector
                  v-model="editCifsForm.forceGroup"
                  type="group"
                  placeholder="group (optional)"
                />
              </div>
            </div>
          </div>
          <div class="form-group">
            <label>Read-Only List / Write List</label>
            <div style="display: flex; gap: 6px">
              <div style="width: 50%">
                <UserGroupSelector
                  v-model="editCifsForm.readListText"
                  type="user"
                  :multiple="true"
                  placeholder="read-only users"
                />
              </div>
              <div style="width: 50%">
                <UserGroupSelector
                  v-model="editCifsForm.writeListText"
                  type="user"
                  :multiple="true"
                  placeholder="write users"
                />
              </div>
            </div>
          </div>
          <div class="form-group">
            <label>Create / Directory Mask</label>
            <div style="display: flex; gap: 6px">
              <input v-model="editCifsForm.createMask" class="input mono" style="width: 50%" />
              <input v-model="editCifsForm.directoryMask" class="input mono" style="width: 50%" />
            </div>
          </div>
          <div class="form-group">
            <label>Guest User Mapping</label>
            <UserGroupSelector
              v-model="editCifsForm.guestAccount"
              type="user"
              placeholder="nobody"
            />
          </div>
        </div>

        <div style="display: flex; flex-direction: column; gap: 8px; margin-top: 10px; padding-top: 8px; border-top: 1px solid rgba(56, 189, 248, 0.2)">
          <div style="display: flex; gap: 14px; font-size: 12px; flex-wrap: wrap">
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="editCifsForm.browseable" type="checkbox" /> Browseable
            </label>
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="editCifsForm.readOnly" type="checkbox" /> Read-Only
            </label>
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="editCifsForm.guestOk" type="checkbox" /> Guest OK
            </label>
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="editCifsForm.guestOnly" type="checkbox" /> Guest Only
            </label>
          </div>
          <div style="display: flex; gap: 14px; font-size: 12px; flex-wrap: wrap">
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="editCifsForm.shadowCopy" type="checkbox" /> ZFS Snapshots
            </label>
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="editCifsForm.timeMachine" type="checkbox" /> macOS Time Machine
            </label>
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="editCifsForm.recycleBin" type="checkbox" /> Recycle Bin
            </label>
          </div>
        </div>

        <div style="display: flex; justify-content: flex-end; gap: 6px; margin-top: 10px">
          <button class="btn btn-sm" @click="editingCifs = null">Cancel</button>
          <button class="btn btn-primary btn-sm" @click="handleSaveCifsEdit">Save Share</button>
        </div>
      </div>

      <!-- CIFS Shares Table -->
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <SortableTh :sort="cifsSort" sort-key="share">SMB Share</SortableTh>
              <SortableTh :sort="cifsSort" sort-key="acl">Access ACLs</SortableTh>
              <SortableTh :sort="cifsSort" sort-key="mode">Permissions & Features</SortableTh>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <template v-if="nasState.loading && nasState.cifsShares.length === 0">
              <tr v-for="r in 3" :key="r">
                <td>
                  <div style="display: flex; flex-direction: column; gap: 4px">
                    <SkeletonBox width="140px" height="15px" />
                    <SkeletonBox width="190px" height="11px" />
                  </div>
                </td>
                <td>
                  <div style="display: flex; flex-direction: column; gap: 4px">
                    <SkeletonBox width="110px" height="13px" />
                    <SkeletonBox width="90px" height="13px" />
                  </div>
                </td>
                <td>
                  <div style="display: flex; gap: 6px">
                    <SkeletonBox width="45px" height="20px" rounded="10px" />
                    <SkeletonBox width="45px" height="20px" rounded="10px" />
                  </div>
                </td>
                <td>
                  <div style="display: inline-flex; gap: 6px">
                    <SkeletonBox width="26px" height="26px" rounded="4px" />
                    <SkeletonBox width="26px" height="26px" rounded="4px" />
                  </div>
                </td>
              </tr>
            </template>

            <tr v-else-if="nasState.cifsShares.length === 0">
              <td colspan="4" style="text-align: center; color: var(--text-secondary); padding: 32px">
                No SMB shares configured.
              </td>
            </tr>

            <tr v-for="s in cifsSort.sorted.value" :key="s.name">
              <td>
                <div style="display: flex; align-items: center; gap: 6px">
                  <span class="mono" style="font-weight: 600">//nas/{{ s.shareName }}</span>
                  <span v-if="!s.browseable" class="badge badge-warning" style="font-size: 10px">Hidden</span>
                </div>
                <div style="font-size: 11px; color: var(--text-secondary)">{{ s.comment || s.path }}</div>
              </td>
              <td class="mono" style="font-size: 11.5px">
                <div>Users: {{ (s.validUsers || []).join(', ') || 'all valid' }}</div>
                <div>Groups: {{ (s.validGroups || []).join(', ') || '—' }}</div>
                <div v-if="s.forceUser || s.forceGroup" style="color: var(--text-muted); font-size: 10.5px">
                  Force: {{ s.forceUser || '—' }} / {{ s.forceGroup || '—' }}
                </div>
              </td>
              <td>
                <div style="display: flex; gap: 4px; flex-wrap: wrap; margin-bottom: 3px">
                  <span class="badge" :class="s.readOnly ? 'badge-warning' : 'badge-success'">
                    {{ s.readOnly ? 'RO' : 'RW' }}
                  </span>
                  <span v-if="s.guestOk" class="badge badge-info">
                    Guest{{ s.guestOnly ? ' Only' : '' }}
                  </span>
                  <span v-if="s.shadowCopy" class="badge badge-purple" title="ZFS Snapshots">
                    Snapshots
                  </span>
                  <span v-if="s.timeMachine" class="badge badge-info" title="macOS Time Machine">
                    Time Machine
                  </span>
                  <span v-if="s.recycleBin" class="badge badge-secondary" title="Recycle Bin">
                    Recycle
                  </span>
                </div>
                <div class="mono" style="font-size: 10.5px; color: var(--text-muted)">
                  Mask: {{ s.createMask || '0664' }} / {{ s.directoryMask || '0775' }}
                </div>
              </td>
              <td>
                <div style="display: flex; gap: 6px">
                  <button class="btn btn-sm" @click="startEditCifs(s)">Edit</button>
                  <button class="btn btn-danger btn-sm" @click="handleDeleteCifs(s)">
                    Del
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- WebDAV Shares Card -->
    <div class="card">
      <div class="card-header">
        <div>
          <h3 class="card-title">WebDAV Shares</h3>
        </div>
        <button class="btn btn-primary btn-sm" @click="showCreateWebdav = !showCreateWebdav">
          {{ showCreateWebdav ? 'Cancel' : '+ Add WebDAV Share' }}
        </button>
      </div>

      <form
        v-if="showCreateWebdav"
        @submit.prevent="handleCreateWebdav"
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
            <label>Share ID</label>
            <input v-model="webdavForm.shareId" class="input mono" placeholder="documents" required />
          </div>
          <div class="form-group">
            <label>Path</label>
            <PathSelector
              v-model="webdavForm.path"
              placeholder="/mnt/tank/documents"
              required
            />
          </div>
          <div class="form-group">
            <label>Allowed Users</label>
            <UserGroupSelector
              v-model="webdavForm.allowedUsersText"
              type="user"
              :multiple="true"
              placeholder="Select allowed users..."
            />
          </div>
          <div class="form-group">
            <label>Allowed Groups</label>
            <UserGroupSelector
              v-model="webdavForm.allowedGroupsText"
              type="group"
              :multiple="true"
              placeholder="Select allowed groups..."
            />
          </div>
        </div>
        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 10px">
          <label style="font-size: 12px; display: flex; align-items: center; gap: 5px">
            <input v-model="webdavForm.readOnly" type="checkbox" /> Read-Only
          </label>
          <button type="submit" class="btn btn-primary btn-sm">Publish WebDAV Share</button>
        </div>
      </form>

      <!-- Edit WebDAV Inline Panel -->
      <div
        v-if="editingWebdav"
        style="
          background: rgba(56, 189, 248, 0.08);
          border: 1px solid var(--accent-primary);
          border-radius: 8px;
          padding: 12px;
          margin-bottom: 14px;
        "
      >
        <div style="font-weight: 600; margin-bottom: 8px">
          Edit {{ editingWebdav.urlPrefix }}
        </div>
        <div class="form-grid">
          <div class="form-group">
            <label>Path</label>
            <PathSelector
              v-model="editWebdavForm.path"
              placeholder="/mnt/tank/documents"
              required
            />
          </div>
          <div class="form-group">
            <label>Allowed Users</label>
            <UserGroupSelector
              v-model="editWebdavForm.allowedUsersText"
              type="user"
              :multiple="true"
            />
          </div>
          <div class="form-group">
            <label>Description</label>
            <input v-model="editWebdavForm.description" class="input" />
          </div>
        </div>
        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 10px">
          <label style="font-size: 12px; display: flex; align-items: center; gap: 5px">
            <input v-model="editWebdavForm.readOnly" type="checkbox" /> Read-Only
          </label>
          <div style="display: flex; gap: 6px">
            <button class="btn btn-sm" @click="editingWebdav = null">Cancel</button>
            <button class="btn btn-primary btn-sm" @click="handleSaveWebdavEdit">Save</button>
          </div>
        </div>
      </div>

      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <SortableTh :sort="webdavSort" sort-key="endpoint">Endpoint URL</SortableTh>
              <SortableTh :sort="webdavSort" sort-key="identity">Allowed Identity</SortableTh>
              <SortableTh :sort="webdavSort" sort-key="access">Access</SortableTh>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <template v-if="nasState.loading && nasState.webdavShares.length === 0">
              <tr v-for="r in 3" :key="r">
                <td>
                  <div style="display: flex; flex-direction: column; gap: 4px">
                    <SkeletonBox width="100px" height="15px" />
                    <SkeletonBox width="170px" height="11px" />
                  </div>
                </td>
                <td>
                  <div style="display: flex; flex-direction: column; gap: 4px">
                    <SkeletonBox width="110px" height="13px" />
                    <SkeletonBox width="80px" height="13px" />
                  </div>
                </td>
                <td><SkeletonBox width="60px" height="20px" rounded="10px" /></td>
                <td>
                  <div style="display: inline-flex; gap: 6px">
                    <SkeletonBox width="26px" height="26px" rounded="4px" />
                    <SkeletonBox width="26px" height="26px" rounded="4px" />
                  </div>
                </td>
              </tr>
            </template>

            <tr v-else-if="nasState.webdavShares.length === 0">
              <td colspan="4" style="text-align: center; color: var(--text-secondary); padding: 32px">
                No WebDAV shares configured.
              </td>
            </tr>

            <tr v-for="w in webdavSort.sorted.value" :key="w.name">
              <td>
                <div class="mono" style="font-weight: 600; color: var(--success-text)">{{ w.urlPrefix }}</div>
                <div style="font-size: 11px; color: var(--text-secondary)">{{ w.description }}</div>
              </td>
              <td class="mono" style="font-size: 11.5px">
                <div>Users: {{ (w.allowedUsers || []).join(', ') || 'authenticated' }}</div>
                <div>Groups: {{ (w.allowedGroups || []).join(', ') || '—' }}</div>
              </td>
              <td>
                <span class="badge" :class="w.readOnly ? 'badge-warning' : 'badge-success'">
                  {{ w.readOnly ? 'RO' : 'RW' }}
                </span>
              </td>
              <td>
                <div style="display: flex; gap: 6px">
                  <button class="btn btn-sm" @click="startEditWebdav(w)">Edit</button>
                  <button class="btn btn-danger btn-sm" @click="handleDeleteWebdav(w)">
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

  <!-- Samba [global] Configuration Modal -->
  <div v-if="showGlobalSettings" class="modal-backdrop" @click.self="showGlobalSettings = false">
    <div class="modal" style="max-width: 600px">
      <div class="modal-header">
        <h3 class="modal-title">Samba [global] Configuration</h3>
        <button class="modal-close" @click="showGlobalSettings = false">×</button>
      </div>
      <form @submit.prevent="handleSaveGlobalSettings">
        <div class="modal-body" style="display: flex; flex-direction: column; gap: 14px">
          <div class="form-grid">
            <div class="form-group">
              <label>Workgroup</label>
              <input v-model="globalForm.workgroup" class="input mono" placeholder="WORKGROUP" required />
            </div>
            <div class="form-group">
              <label>Server String</label>
              <input v-model="globalForm.serverString" class="input" placeholder="NAS Samba Server" />
            </div>
            <div class="form-group">
              <label>Security Mode</label>
              <select v-model="globalForm.security" class="select mono">
                <option value="user">user (Standard)</option>
              </select>
            </div>
            <div class="form-group">
              <label>Map to Guest</label>
              <select v-model="globalForm.mapToGuest" class="select mono">
                <option value="Bad User">Bad User (Allow Guest Fallback)</option>
                <option value="Never">Never (Strict Authentication)</option>
              </select>
            </div>
            <div class="form-group">
              <label>Guest System Account</label>
              <UserGroupSelector
                v-model="globalForm.guestAccount"
                type="user"
                placeholder="nobody"
                :include-system="true"
              />
            </div>
            <div class="form-group">
              <label>Max Log Size (KB)</label>
              <input v-model.number="globalForm.maxLogSize" type="number" class="input mono" min="10" />
            </div>
          </div>

          <div style="display: flex; flex-direction: column; gap: 8px; font-size: 12.5px; padding-top: 10px; border-top: 1px solid var(--border-color)">
            <label style="display: flex; align-items: center; gap: 6px">
              <input v-model="globalForm.disableSpoolss" type="checkbox" /> Disable Spoolss & Printing Services
            </label>
            <label style="display: flex; align-items: center; gap: 6px">
              <input v-model="globalForm.fruitAapl" type="checkbox" /> Apple AAPL Extensions (Enhanced macOS Finder Support)
            </label>
            <label style="display: flex; align-items: center; gap: 6px">
              <input v-model="globalForm.wsddEnabled" type="checkbox" /> Windows Network Discovery (WSDD Daemon)
            </label>
          </div>
        </div>
        <div class="modal-footer">
          <button type="button" class="btn btn-sm" @click="showGlobalSettings = false">Cancel</button>
          <button type="submit" class="btn btn-primary btn-sm">Save & Apply Global Config</button>
        </div>
      </form>
    </div>
  </div>
</template>
