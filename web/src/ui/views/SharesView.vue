<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { services } from '../../container';
import type { CifsShare, WebdavShare } from '../../domain/models';
import SkeletonBox from '../components/SkeletonBox.vue';
import SortableTh from '../components/SortableTh.vue';
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

const showCreateCifs = ref(false);
const cifsForm = reactive({
  shareId: '',
  path: '/nas/tank/media',
  comment: 'Shared ZFS Dataset',
  validUsersText: 'admin, nasuser',
  validGroupsText: 'nasusers',
  readOnly: false,
  guestOk: false,
  browseable: true,
  createMask: '0664',
  directoryMask: '0775',
  enabled: true,
});

const editingCifs = ref<CifsShare | null>(null);
const editCifsForm = reactive({
  path: '',
  comment: '',
  validUsersText: '',
  validGroupsText: '',
  readOnly: false,
  guestOk: false,
  enabled: true,
});

const showCreateWebdav = ref(false);
const webdavForm = reactive({
  shareId: '',
  path: '/nas/tank/documents',
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
  const [cifs, webdav] = await Promise.all([
    services.shares.listCifsShares(),
    services.shares.listWebdavShares(),
  ]);
  nasState.cifsShares = cifs;
  nasState.webdavShares = webdav;
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

    const created = await services.shares.createCifsShare(cifsForm.shareId, {
      path: cifsForm.path,
      comment: cifsForm.comment,
      validUsers,
      validGroups,
      readOnly: cifsForm.readOnly,
      guestOk: cifsForm.guestOk,
      browseable: cifsForm.browseable,
      createMask: cifsForm.createMask,
      directoryMask: cifsForm.directoryMask,
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
  editCifsForm.readOnly = share.readOnly;
  editCifsForm.guestOk = share.guestOk;
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

    await services.shares.updateCifsShare(
      editingCifs.value.name,
      {
        path: editCifsForm.path,
        comment: editCifsForm.comment,
        validUsers,
        validGroups,
        readOnly: editCifsForm.readOnly,
        guestOk: editCifsForm.guestOk,
        enabled: editCifsForm.enabled,
      },
      ['path', 'comment', 'valid_users', 'valid_groups', 'read_only', 'guest_ok', 'enabled']
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
        <button
          class="btn btn-primary btn-sm"
          :disabled="!cifsModuleStatus.enabled"
          @click="showCreateCifs = !showCreateCifs"
        >
          {{ !cifsModuleStatus.enabled ? 'CIFS Disabled (smbd missing)' : showCreateCifs ? 'Cancel' : '+ Add CIFS Share' }}
        </button>
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
          font-size: 12.5px;
        "
      >
        CIFS / Samba module is disabled because the underlying <code>smbd</code> daemon binary was not detected on the host.
      </div>

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
            <label>Path</label>
            <input v-model="cifsForm.path" class="input mono" placeholder="/nas/tank/media" required />
          </div>
          <div class="form-group">
            <label>Valid Users (comma-separated)</label>
            <input v-model="cifsForm.validUsersText" class="input mono" placeholder="admin, nasuser" />
          </div>
          <div class="form-group">
            <label>Valid Groups (comma-separated)</label>
            <input v-model="cifsForm.validGroupsText" class="input mono" placeholder="nasusers" />
          </div>
          <div class="form-group">
            <label>Create / Directory Mask</label>
            <div style="display: flex; gap: 6px">
              <input v-model="cifsForm.createMask" class="input mono" style="width: 50%" />
              <input v-model="cifsForm.directoryMask" class="input mono" style="width: 50%" />
            </div>
          </div>
          <div class="form-group">
            <label>Comment</label>
            <input v-model="cifsForm.comment" class="input" placeholder="Network Media Share" />
          </div>
        </div>
        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 10px">
          <div style="display: flex; gap: 14px; font-size: 12px">
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="cifsForm.readOnly" type="checkbox" /> Read-Only
            </label>
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="cifsForm.guestOk" type="checkbox" /> Guest OK
            </label>
          </div>
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
        <div style="font-weight: 600; margin-bottom: 8px">
          Edit {{ editingCifs.shareName }}
        </div>
        <div class="form-grid">
          <div class="form-group">
            <label>Path</label>
            <input v-model="editCifsForm.path" class="input mono" />
          </div>
          <div class="form-group">
            <label>Valid Users</label>
            <input v-model="editCifsForm.validUsersText" class="input mono" />
          </div>
          <div class="form-group">
            <label>Valid Groups</label>
            <input v-model="editCifsForm.validGroupsText" class="input mono" />
          </div>
        </div>
        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 10px">
          <div style="display: flex; gap: 14px; font-size: 12px">
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="editCifsForm.readOnly" type="checkbox" /> Read-Only
            </label>
            <label style="display: flex; align-items: center; gap: 5px">
              <input v-model="editCifsForm.guestOk" type="checkbox" /> Guest OK
            </label>
          </div>
          <div style="display: flex; gap: 6px">
            <button class="btn btn-sm" @click="editingCifs = null">Cancel</button>
            <button class="btn btn-primary btn-sm" @click="handleSaveCifsEdit">Save</button>
          </div>
        </div>
      </div>

      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <SortableTh :sort="cifsSort" sort-key="share">SMB Share</SortableTh>
              <SortableTh :sort="cifsSort" sort-key="acl">Access ACLs</SortableTh>
              <SortableTh :sort="cifsSort" sort-key="mode">Mode / Masks</SortableTh>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <!-- Moving Skeleton Placeholders when Loading -->
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
                No SMB shares.
              </td>
            </tr>

            <tr v-for="s in cifsSort.sorted.value" :key="s.name">
              <td>
                <div class="mono" style="font-weight: 600">//nas/{{ s.shareName }}</div>
                <div style="font-size: 11px; color: var(--text-secondary)">{{ s.comment }}</div>
              </td>
              <td class="mono" style="font-size: 11.5px">
                <div>Users: {{ (s.validUsers || []).join(', ') || 'any' }}</div>
                <div>Groups: {{ (s.validGroups || []).join(', ') || '—' }}</div>
              </td>
              <td>
                <span class="badge" :class="s.readOnly ? 'badge-warning' : 'badge-success'">
                  {{ s.readOnly ? 'RO' : 'RW' }}
                </span>
                <span v-if="s.guestOk" class="badge badge-info" style="margin-left: 4px">
                  Guest
                </span>
                <div class="mono" style="font-size: 10.5px; color: var(--text-muted); margin-top: 3px">
                  {{ s.createMask }} / {{ s.directoryMask }}
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
            <input v-model="webdavForm.path" class="input mono" placeholder="/nas/tank/documents" required />
          </div>
          <div class="form-group">
            <label>Allowed Users</label>
            <input v-model="webdavForm.allowedUsersText" class="input mono" placeholder="admin, nasuser" />
          </div>
          <div class="form-group">
            <label>Allowed Groups</label>
            <input v-model="webdavForm.allowedGroupsText" class="input mono" placeholder="nasusers" />
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
            <input v-model="editWebdavForm.path" class="input mono" />
          </div>
          <div class="form-group">
            <label>Allowed Users</label>
            <input v-model="editWebdavForm.allowedUsersText" class="input mono" />
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
            <!-- Moving Skeleton Placeholders when Loading -->
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
                No WebDAV shares.
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
</template>
