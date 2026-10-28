<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import {
  mdiCheck,
  mdiClose,
  mdiCog,
  mdiContentCopy,
  mdiDelete,
  mdiFolderNetwork,
  mdiHelpCircleOutline,
  mdiInformationOutline,
  mdiLanConnect,
  mdiLock,
  mdiLockOpenVariantOutline,
  mdiOpenInNew,
  mdiPencil,
  mdiPlus,
  mdiWeb,
} from '@mdi/js';
import { services } from '../../container';
import type { CifsGlobalConfig, CifsShare, WebdavShare } from '../../domain/models';
import MdiIcon from '../components/MdiIcon.vue';
import SkeletonBox from '../components/SkeletonBox.vue';
import SortableTh from '../components/SortableTh.vue';
import PathSelector from '../components/PathSelector.vue';
import UserGroupSelector from '../components/UserGroupSelector.vue';
import { useTableSort } from '../composables/useTableSort';
import { getModuleAvailability, nasState, notify } from '../store';
import { confirmDelete } from '../modal';
import { navigateTo } from '../router';

const props = withDefaults(
  defineProps<{
    activeSubtab?: 'cifs' | 'webdav' | 'all';
  }>(),
  {
    activeSubtab: undefined,
  }
);

const currentSubtab = computed<'cifs' | 'webdav' | 'all'>(() => {
  if (props.activeSubtab) return props.activeSubtab;
  if (nasState.activeTab === 'shares-webdav') return 'webdav';
  if (nasState.activeTab === 'shares-cifs') return 'cifs';
  return 'all';
});

const cifsSort = useTableSort(
  () => nasState.cifsShares,
  {
    share: (s: CifsShare) => s.shareName,
    path: (s: CifsShare) => s.path + ' ' + (s.comment || ''),
    acl: (s: CifsShare) => (s.validUsers || []).join(', '),
    mode: (s: CifsShare) => (s.readOnly ? 'ro' : 'rw'),
    features: (s: CifsShare) => [s.shadowCopy ? 'shadow' : '', s.timeMachine ? 'tm' : '', s.recycleBin ? 'recycle' : '', s.createMask || '', s.directoryMask || ''].join(' '),
  }
);

const webdavSort = useTableSort(
  () => nasState.webdavShares,
  {
    endpoint: (w: WebdavShare) => w.urlPrefix,
    path: (w: WebdavShare) => w.path + ' ' + (w.description || ''),
    identity: (w: WebdavShare) => (w.allowedUsers || []).join(', '),
    access: (w: WebdavShare) => (w.readOnly ? 'ro' : 'rw'),
    status: (w: WebdavShare) => (w.enabled ? 1 : 0),
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
const lastDerivedWebdavSlug = ref('');
const copiedWebdavUrl = ref<string | null>(null);
const connectingWebdav = ref<WebdavShare | null>(null);
const connectActiveTab = ref<'windows' | 'macos' | 'linux' | 'browser'>('windows');

const webdavForm = reactive({
  shareId: '',
  path: '',
  description: '',
  allowedUsersText: '',
  allowedGroupsText: '',
  readOnly: false,
  enabled: true,
});

watch(
  () => webdavForm.path,
  (newPath) => {
    if (!newPath) return;
    const trimmed = newPath.replace(/\/+$/, '');
    const segments = trimmed.split('/').filter(Boolean);
    if (segments.length > 0) {
      const slug = segments[segments.length - 1];
      if (!webdavForm.shareId || webdavForm.shareId === lastDerivedWebdavSlug.value) {
        webdavForm.shareId = slug;
        lastDerivedWebdavSlug.value = slug;
      }
    }
  }
);

function getWebdavShortName(share: WebdavShare): string {
  return (
    share.name.replace(/^webdavShares\//, '') ||
    (share.urlPrefix ? share.urlPrefix.replace(/^\/dav\//, '') : '')
  );
}

const currentHostname = computed(() =>
  typeof window !== 'undefined' ? window.location.hostname || 'localhost' : 'localhost'
);

function getWebdavMountUrl(share: WebdavShare): string {
  const host = currentHostname.value;
  const shortName = getWebdavShortName(share);
  return `http://${host}:8081/dav/${shortName}/`;
}

async function copyWebdavMountUrl(share: WebdavShare) {
  const url = getWebdavMountUrl(share);
  try {
    await navigator.clipboard.writeText(url);
    copiedWebdavUrl.value = share.name;
    setTimeout(() => {
      if (copiedWebdavUrl.value === share.name) {
        copiedWebdavUrl.value = null;
      }
    }, 2000);
    notify('info', 'WebDAV mount URL copied to clipboard');
  } catch {
    notify('error', 'Failed to copy mount URL');
  }
}

function openConnectModal(share: WebdavShare) {
  connectingWebdav.value = share;
  connectActiveTab.value = 'windows';
}

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
      maxLogSize: globalForm.maxLogSize || 50,
      disableSpoolss: globalForm.disableSpoolss,
      fruitAapl: globalForm.fruitAapl,
      wsddEnabled: globalForm.wsddEnabled,
    });
    nasState.cifsGlobalConfig = updated;
    Object.assign(globalForm, updated);
    showGlobalSettings.value = false;
    notify('success', 'Samba [global] settings saved');
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
      path: cifsForm.path,
      comment: cifsForm.comment,
      validUsers,
      validGroups,
      readList,
      writeList,
      forceUser: cifsForm.forceUser.trim() || undefined,
      forceGroup: cifsForm.forceGroup.trim() || undefined,
      guestAccount: cifsForm.guestAccount.trim() || undefined,
      readOnly: cifsForm.readOnly,
      guestOk: cifsForm.guestOk,
      guestOnly: cifsForm.guestOnly,
      browseable: cifsForm.browseable,
      createMask: cifsForm.createMask.trim() || undefined,
      directoryMask: cifsForm.directoryMask.trim() || undefined,
      forceCreateMode: cifsForm.forceCreateMode.trim() || undefined,
      forceDirectoryMode: cifsForm.forceDirectoryMode.trim() || undefined,
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
  editCifsForm.comment = share.comment || '';
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
        path: editCifsForm.path,
        comment: editCifsForm.comment,
        validUsers,
        validGroups,
        readList,
        writeList,
        forceUser: editCifsForm.forceUser.trim() || undefined,
        forceGroup: editCifsForm.forceGroup.trim() || undefined,
        guestAccount: editCifsForm.guestAccount.trim() || undefined,
        readOnly: editCifsForm.readOnly,
        guestOk: editCifsForm.guestOk,
        guestOnly: editCifsForm.guestOnly,
        browseable: editCifsForm.browseable,
        createMask: editCifsForm.createMask.trim() || undefined,
        directoryMask: editCifsForm.directoryMask.trim() || undefined,
        forceCreateMode: editCifsForm.forceCreateMode.trim() || undefined,
        forceDirectoryMode: editCifsForm.forceDirectoryMode.trim() || undefined,
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
  <div class="shares-view-container">
    <!-- Sub-navigation Pill Bar -->
    <div class="shares-subnav">
      <div class="subnav-pill-group">
        <button
          type="button"
          class="subnav-pill"
          :class="{ active: currentSubtab === 'cifs' || (currentSubtab === 'all' && nasState.activeTab !== 'shares-webdav') }"
          @click="navigateTo('shares-cifs')"
        >
          <MdiIcon :path="mdiFolderNetwork" size="16" />
          <span>Windows (SMB / CIFS)</span>
          <span class="subnav-badge">{{ nasState.cifsShares.length }}</span>
        </button>
        <button
          type="button"
          class="subnav-pill"
          :class="{ active: currentSubtab === 'webdav' }"
          @click="navigateTo('shares-webdav')"
        >
          <MdiIcon :path="mdiWeb" size="16" />
          <span>WebDAV</span>
          <span class="subnav-badge">{{ nasState.webdavShares.length }}</span>
        </button>
      </div>
    </div>

    <!-- CIFS / Samba Shares Card (Wide Table) -->
    <div v-if="currentSubtab === 'cifs' || currentSubtab === 'all'" class="card shares-wide-card">
      <div class="card-header">
        <div>
          <h3 class="card-title">SMB / CIFS Shares</h3>
          <p class="card-subtitle" style="margin-top: 2px">
            High-performance network file sharing for Windows, macOS, and Linux clients via Samba.
          </p>
        </div>
        <div style="display: flex; gap: 8px; flex-wrap: wrap">
          <button
            class="btn btn-sm btn-secondary"
            :disabled="!cifsModuleStatus.enabled"
            @click="showGlobalSettings = true"
          >
            <MdiIcon :path="mdiCog" size="14" style="margin-right: 4px" />
            Samba Settings
          </button>
          <button
            class="btn btn-primary btn-sm"
            :disabled="!cifsModuleStatus.enabled"
            @click="showCreateCifs = !showCreateCifs"
          >
            <MdiIcon :path="showCreateCifs ? mdiClose : mdiPlus" size="14" style="margin-right: 4px" />
            {{ !cifsModuleStatus.enabled ? 'CIFS Disabled' : showCreateCifs ? 'Cancel' : 'Add SMB Share' }}
          </button>
        </div>
      </div>

      <div
        v-if="!cifsModuleStatus.enabled"
        style="
          padding: 12px;
          margin-bottom: 14px;
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
          padding: 16px;
          margin-bottom: 16px;
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

        <div style="display: flex; justify-content: flex-end; margin-top: 14px">
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
          padding: 14px;
          margin-bottom: 16px;
        "
      >
        <div style="font-weight: 600; margin-bottom: 10px; font-size: 14px">
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

        <div style="display: flex; justify-content: flex-end; gap: 6px; margin-top: 12px">
          <button class="btn btn-sm" @click="editingCifs = null">Cancel</button>
          <button class="btn btn-primary btn-sm" @click="handleSaveCifsEdit">Save Share</button>
        </div>
      </div>

      <!-- CIFS Shares Wide Table -->
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <SortableTh :sort="cifsSort" sort-key="share">SMB Share</SortableTh>
              <SortableTh :sort="cifsSort" sort-key="path">Filesystem Path & Description</SortableTh>
              <SortableTh :sort="cifsSort" sort-key="acl">Access Control</SortableTh>
              <SortableTh :sort="cifsSort" sort-key="mode">Permissions</SortableTh>
              <SortableTh :sort="cifsSort" sort-key="features">VFS Features & Masks</SortableTh>
              <th style="width: 90px; text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <template v-if="nasState.loading && nasState.cifsShares.length === 0">
              <tr v-for="r in 3" :key="r">
                <td>
                  <div style="display: flex; flex-direction: column; gap: 4px">
                    <SkeletonBox width="140px" height="15px" />
                    <SkeletonBox width="80px" height="11px" />
                  </div>
                </td>
                <td>
                  <div style="display: flex; flex-direction: column; gap: 4px">
                    <SkeletonBox width="180px" height="13px" />
                    <SkeletonBox width="120px" height="11px" />
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
                  <SkeletonBox width="120px" height="16px" />
                </td>
                <td style="text-align: right">
                  <div style="display: inline-flex; gap: 6px; justify-content: flex-end">
                    <SkeletonBox width="28px" height="28px" rounded="4px" />
                    <SkeletonBox width="28px" height="28px" rounded="4px" />
                  </div>
                </td>
              </tr>
            </template>

            <tr v-else-if="nasState.cifsShares.length === 0">
              <td colspan="6" style="text-align: center; color: var(--text-secondary); padding: 36px">
                No SMB shares configured.
              </td>
            </tr>

            <tr v-for="s in cifsSort.sorted.value" :key="s.name">
              <td>
                <div style="display: flex; align-items: center; gap: 6px">
                  <span class="mono" style="font-weight: 600; font-size: 13.5px; color: var(--accent-primary)">
                    //nas/{{ s.shareName }}
                  </span>
                  <span v-if="!s.browseable" class="badge badge-warning" style="font-size: 10px">Hidden</span>
                </div>
              </td>
              <td>
                <div class="mono" style="font-size: 12px; color: var(--text-primary)">{{ s.path }}</div>
                <div v-if="s.comment" style="font-size: 11px; color: var(--text-secondary); margin-top: 2px">
                  {{ s.comment }}
                </div>
              </td>
              <td class="mono" style="font-size: 11.5px">
                <div>
                  <span style="color: var(--text-secondary)">Users:</span>
                  <strong>{{ (s.validUsers || []).join(', ') || 'all valid' }}</strong>
                </div>
                <div>
                  <span style="color: var(--text-secondary)">Groups:</span>
                  {{ (s.validGroups || []).join(', ') || '—' }}
                </div>
                <div v-if="s.forceUser || s.forceGroup" style="color: var(--text-muted); font-size: 10.5px">
                  Force: {{ s.forceUser || '—' }} / {{ s.forceGroup || '—' }}
                </div>
              </td>
              <td>
                <div style="display: flex; gap: 4px; flex-wrap: wrap">
                  <span class="badge" :class="s.readOnly ? 'badge-warning' : 'badge-success'">
                    {{ s.readOnly ? 'RO' : 'RW' }}
                  </span>
                  <span v-if="s.guestOk" class="badge badge-info">
                    Guest{{ s.guestOnly ? ' Only' : '' }}
                  </span>
                </div>
              </td>
              <td>
                <div style="display: flex; gap: 4px; flex-wrap: wrap; margin-bottom: 3px">
                  <span v-if="s.shadowCopy" class="badge badge-purple" title="ZFS Snapshots (Windows Previous Versions)">
                    Snapshots
                  </span>
                  <span v-if="s.timeMachine" class="badge badge-info" title="macOS Time Machine Target">
                    Time Machine
                  </span>
                  <span v-if="s.recycleBin" class="badge badge-secondary" title="Recycle Bin Enabled">
                    Recycle
                  </span>
                </div>
                <div class="mono" style="font-size: 10.5px; color: var(--text-muted)">
                  Mask: {{ s.createMask || '0664' }} / {{ s.directoryMask || '0775' }}
                </div>
              </td>
              <td style="text-align: right">
                <div style="display: inline-flex; gap: 6px; justify-content: flex-end">
                  <button
                    class="btn btn-sm btn-icon"
                    title="Edit SMB Share"
                    aria-label="Edit SMB Share"
                    @click="startEditCifs(s)"
                  >
                    <MdiIcon :path="mdiPencil" size="14" />
                  </button>
                  <button
                    class="btn btn-danger btn-sm btn-icon"
                    title="Delete SMB Share"
                    aria-label="Delete SMB Share"
                    @click="handleDeleteCifs(s)"
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

    <!-- WebDAV Shares Card (Wide Table) -->
    <div v-if="currentSubtab === 'webdav' || currentSubtab === 'all'" class="card shares-wide-card">
      <div class="card-header">
        <div>
          <h3 class="card-title">WebDAV Endpoints</h3>
          <p class="card-subtitle" style="margin-top: 2px">
            HTTP / HTTPS file sharing endpoints for Nextcloud, mobile devices, and remote syncing.
          </p>
        </div>
        <button class="btn btn-primary btn-sm" @click="showCreateWebdav = !showCreateWebdav">
          <MdiIcon :path="showCreateWebdav ? mdiClose : mdiPlus" size="14" style="margin-right: 4px" />
          {{ showCreateWebdav ? 'Cancel' : 'Add WebDAV Share' }}
        </button>
      </div>

      <form
        v-if="showCreateWebdav"
        @submit.prevent="handleCreateWebdav"
        style="
          background: var(--bg-base);
          border: 1px solid var(--border-strong);
          border-radius: 8px;
          padding: 16px;
          margin-bottom: 16px;
        "
      >
        <div class="form-grid">
          <div class="form-group">
            <label>Share ID</label>
            <input v-model="webdavForm.shareId" class="input mono" placeholder="documents" required />
          </div>
          <div class="form-group">
            <label>Filesystem Path</label>
            <PathSelector
              v-model="webdavForm.path"
              placeholder="/mnt/tank/documents"
              required
            />
          </div>
          <div class="form-group">
            <label>Description</label>
            <input v-model="webdavForm.description" class="input" placeholder="Documents Store" />
          </div>
          <div class="form-group">
            <label>Allowed Users</label>
            <UserGroupSelector
              v-model="webdavForm.allowedUsersText"
              type="user"
              :multiple="true"
              placeholder="Select allowed users (leave empty for all users)..."
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

          <div class="form-group" style="grid-column: 1 / -1">
            <label style="font-weight: 600; margin-bottom: 6px; display: block">Access Type</label>
            <div class="access-type-selector">
              <button
                type="button"
                class="access-type-card"
                :class="{ selected: !webdavForm.readOnly }"
                @click="webdavForm.readOnly = false"
              >
                <div style="display: flex; align-items: center; gap: 8px">
                  <MdiIcon :path="mdiLockOpenVariantOutline" size="18" style="color: #22c55e" />
                  <span style="font-weight: 600; font-size: 13px">Read-Write (Full Access)</span>
                </div>
                <div style="font-size: 11.5px; color: var(--text-secondary); margin-top: 4px">
                  Allowed users can read, upload, edit, rename, and delete files.
                </div>
              </button>
              <button
                type="button"
                class="access-type-card"
                :class="{ selected: webdavForm.readOnly }"
                @click="webdavForm.readOnly = true"
              >
                <div style="display: flex; align-items: center; gap: 8px">
                  <MdiIcon :path="mdiLock" size="18" style="color: #eab308" />
                  <span style="font-weight: 600; font-size: 13px">Read-Only</span>
                </div>
                <div style="font-size: 11.5px; color: var(--text-secondary); margin-top: 4px">
                  Safe browsing and downloads. Blocks uploads, modifications, and deletes.
                </div>
              </button>
            </div>
          </div>
        </div>
        <div style="display: flex; justify-content: flex-end; align-items: center; margin-top: 14px">
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
          padding: 14px;
          margin-bottom: 16px;
        "
      >
        <div style="font-weight: 600; margin-bottom: 10px; font-size: 14px">
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
            <label>Allowed Groups</label>
            <UserGroupSelector
              v-model="editWebdavForm.allowedGroupsText"
              type="group"
              :multiple="true"
            />
          </div>
          <div class="form-group">
            <label>Description</label>
            <input v-model="editWebdavForm.description" class="input" />
          </div>

          <div class="form-group" style="grid-column: 1 / -1">
            <label style="font-weight: 600; margin-bottom: 6px; display: block">Access Type</label>
            <div class="access-type-selector">
              <button
                type="button"
                class="access-type-card"
                :class="{ selected: !editWebdavForm.readOnly }"
                @click="editWebdavForm.readOnly = false"
              >
                <div style="display: flex; align-items: center; gap: 8px">
                  <MdiIcon :path="mdiLockOpenVariantOutline" size="18" style="color: #22c55e" />
                  <span style="font-weight: 600; font-size: 13px">Read-Write (Full Access)</span>
                </div>
                <div style="font-size: 11.5px; color: var(--text-secondary); margin-top: 4px">
                  Allowed users can read, upload, edit, rename, and delete files.
                </div>
              </button>
              <button
                type="button"
                class="access-type-card"
                :class="{ selected: editWebdavForm.readOnly }"
                @click="editWebdavForm.readOnly = true"
              >
                <div style="display: flex; align-items: center; gap: 8px">
                  <MdiIcon :path="mdiLock" size="18" style="color: #eab308" />
                  <span style="font-weight: 600; font-size: 13px">Read-Only</span>
                </div>
                <div style="font-size: 11.5px; color: var(--text-secondary); margin-top: 4px">
                  Safe browsing and downloads. Blocks uploads, modifications, and deletes.
                </div>
              </button>
            </div>
          </div>
        </div>
        <div style="display: flex; justify-content: flex-end; align-items: center; margin-top: 14px; gap: 8px">
          <button class="btn btn-sm" @click="editingWebdav = null">Cancel</button>
          <button class="btn btn-primary btn-sm" @click="handleSaveWebdavEdit">Save</button>
        </div>
      </div>

      <!-- WebDAV Shares Wide Table -->
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <SortableTh :sort="webdavSort" sort-key="endpoint">WebDAV Endpoint</SortableTh>
              <SortableTh :sort="webdavSort" sort-key="path">Target Path & Description</SortableTh>
              <SortableTh :sort="webdavSort" sort-key="identity">Allowed Identity</SortableTh>
              <SortableTh :sort="webdavSort" sort-key="access">Permissions</SortableTh>
              <SortableTh :sort="webdavSort" sort-key="status">Status</SortableTh>
              <th style="width: 90px; text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <template v-if="nasState.loading && nasState.webdavShares.length === 0">
              <tr v-for="r in 3" :key="r">
                <td>
                  <div style="display: flex; flex-direction: column; gap: 4px">
                    <SkeletonBox width="140px" height="15px" />
                    <SkeletonBox width="80px" height="11px" />
                  </div>
                </td>
                <td>
                  <div style="display: flex; flex-direction: column; gap: 4px">
                    <SkeletonBox width="180px" height="13px" />
                    <SkeletonBox width="120px" height="11px" />
                  </div>
                </td>
                <td>
                  <div style="display: flex; flex-direction: column; gap: 4px">
                    <SkeletonBox width="110px" height="13px" />
                    <SkeletonBox width="80px" height="13px" />
                  </div>
                </td>
                <td><SkeletonBox width="45px" height="20px" rounded="10px" /></td>
                <td><SkeletonBox width="55px" height="20px" rounded="10px" /></td>
                <td style="text-align: right">
                  <div style="display: inline-flex; gap: 6px; justify-content: flex-end">
                    <SkeletonBox width="28px" height="28px" rounded="4px" />
                    <SkeletonBox width="28px" height="28px" rounded="4px" />
                  </div>
                </td>
              </tr>
            </template>

            <tr v-else-if="nasState.webdavShares.length === 0">
              <td colspan="6" style="text-align: center; color: var(--text-secondary); padding: 36px">
                No WebDAV shares configured.
              </td>
            </tr>

            <tr v-for="w in webdavSort.sorted.value" :key="w.name">
              <td>
                <div class="endpoint-cell-box">
                  <div style="display: flex; align-items: center; gap: 8px">
                    <span class="mono" style="font-weight: 600; font-size: 13.5px; color: var(--accent-primary)">
                      /dav/{{ getWebdavShortName(w) }}
                    </span>
                    <button
                      class="btn btn-sm btn-icon"
                      title="Client Connect Guide"
                      aria-label="Client Connect Guide"
                      style="padding: 2px 6px; height: 22px"
                      @click="openConnectModal(w)"
                    >
                      <MdiIcon :path="mdiLanConnect" size="13" />
                    </button>
                  </div>
                  <div class="endpoint-url-row">
                    <a
                      :href="getWebdavMountUrl(w)"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="endpoint-url-text"
                      title="Open WebDAV Auto-Index"
                    >
                      {{ getWebdavMountUrl(w) }}
                    </a>
                    <button
                      class="btn btn-sm btn-icon"
                      :title="copiedWebdavUrl === w.name ? 'Copied!' : 'Copy Mount URL'"
                      aria-label="Copy Mount URL"
                      style="padding: 1px 5px; height: 20px"
                      @click="copyWebdavMountUrl(w)"
                    >
                      <MdiIcon :path="copiedWebdavUrl === w.name ? mdiCheck : mdiContentCopy" size="12" />
                    </button>
                    <a
                      :href="getWebdavMountUrl(w)"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="btn btn-sm btn-icon"
                      title="Open in Browser (Auto-Index)"
                      aria-label="Open in Browser"
                      style="padding: 1px 5px; height: 20px; color: var(--text-secondary)"
                    >
                      <MdiIcon :path="mdiOpenInNew" size="12" />
                    </a>
                  </div>
                </div>
              </td>
              <td>
                <div class="mono" style="font-size: 12px; color: var(--text-primary)">{{ w.path }}</div>
                <div v-if="w.description" style="font-size: 11px; color: var(--text-secondary); margin-top: 2px">
                  {{ w.description }}
                </div>
              </td>
              <td class="mono" style="font-size: 11.5px">
                <div>
                  <span style="color: var(--text-secondary)">Users:</span>
                  <strong>{{ (w.allowedUsers || []).join(', ') || 'All Authenticated' }}</strong>
                </div>
                <div>
                  <span style="color: var(--text-secondary)">Groups:</span>
                  {{ (w.allowedGroups || []).join(', ') || '—' }}
                </div>
              </td>
              <td>
                <span class="badge" :class="w.readOnly ? 'badge-warning' : 'badge-success'">
                  {{ w.readOnly ? 'Read-Only' : 'Read-Write' }}
                </span>
              </td>
              <td>
                <span class="badge" :class="w.enabled ? 'badge-success' : 'badge-neutral'">
                  {{ w.enabled ? 'ONLINE' : 'DISABLED' }}
                </span>
              </td>
              <td style="text-align: right">
                <div style="display: inline-flex; gap: 6px; justify-content: flex-end">
                  <button
                    class="btn btn-sm btn-icon"
                    title="Client Connect Guide"
                    aria-label="Client Connect Guide"
                    @click="openConnectModal(w)"
                  >
                    <MdiIcon :path="mdiLanConnect" size="14" />
                  </button>
                  <button
                    class="btn btn-sm btn-icon"
                    title="Edit WebDAV Share"
                    aria-label="Edit WebDAV Share"
                    @click="startEditWebdav(w)"
                  >
                    <MdiIcon :path="mdiPencil" size="14" />
                  </button>
                  <button
                    class="btn btn-danger btn-sm btn-icon"
                    title="Delete WebDAV Share"
                    aria-label="Delete WebDAV Share"
                    @click="handleDeleteWebdav(w)"
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

  <!-- WebDAV Client Connect Guide Modal -->
  <div v-if="connectingWebdav" class="modal-backdrop" @click.self="connectingWebdav = null">
    <div class="modal" style="max-width: 680px">
      <div class="modal-header">
        <div style="display: flex; align-items: center; gap: 8px">
          <MdiIcon :path="mdiLanConnect" size="20" style="color: var(--accent-primary)" />
          <h3 class="modal-title">Connect to WebDAV Share: {{ getWebdavShortName(connectingWebdav) }}</h3>
        </div>
        <button class="modal-close" @click="connectingWebdav = null">×</button>
      </div>
      <div class="modal-body" style="display: flex; flex-direction: column; gap: 16px">
        <!-- Mount URL Banner -->
        <div style="background: var(--bg-base); border: 1px solid var(--border-strong); border-radius: 8px; padding: 12px">
          <div style="font-size: 11px; text-transform: uppercase; color: var(--text-secondary); font-weight: 600; margin-bottom: 4px">
            Dedicated WebDAV Port 8081 Mount URL
          </div>
          <div style="display: flex; align-items: center; justify-content: space-between; gap: 8px">
            <span class="mono" style="font-size: 13px; color: var(--accent-primary); word-break: break-all; font-weight: 600">
              {{ getWebdavMountUrl(connectingWebdav) }}
            </span>
            <div style="display: flex; gap: 6px; flex-shrink: 0">
              <button class="btn btn-sm" @click="copyWebdavMountUrl(connectingWebdav)">
                <MdiIcon :path="copiedWebdavUrl === connectingWebdav.name ? mdiCheck : mdiContentCopy" size="14" style="margin-right: 4px" />
                {{ copiedWebdavUrl === connectingWebdav.name ? 'Copied' : 'Copy' }}
              </button>
              <a :href="getWebdavMountUrl(connectingWebdav)" target="_blank" rel="noopener noreferrer" class="btn btn-sm btn-primary">
                <MdiIcon :path="mdiOpenInNew" size="14" style="margin-right: 4px" />
                Browse Auto-Index
              </a>
            </div>
          </div>
        </div>

        <!-- OS Guide Tabs -->
        <div style="display: flex; border-bottom: 1px solid var(--border-subtle); gap: 8px">
          <button
            class="connect-tab-btn"
            :class="{ active: connectActiveTab === 'windows' }"
            @click="connectActiveTab = 'windows'"
          >
            Windows
          </button>
          <button
            class="connect-tab-btn"
            :class="{ active: connectActiveTab === 'macos' }"
            @click="connectActiveTab = 'macos'"
          >
            macOS
          </button>
          <button
            class="connect-tab-btn"
            :class="{ active: connectActiveTab === 'linux' }"
            @click="connectActiveTab = 'linux'"
          >
            Linux
          </button>
          <button
            class="connect-tab-btn"
            :class="{ active: connectActiveTab === 'browser' }"
            @click="connectActiveTab = 'browser'"
          >
            Nextcloud & Mobile
          </button>
        </div>

        <!-- Windows Tab -->
        <div v-if="connectActiveTab === 'windows'" style="display: flex; flex-direction: column; gap: 12px">
          <div>
            <div style="font-weight: 600; font-size: 13px; margin-bottom: 4px">Method 1: Map Network Drive (File Explorer)</div>
            <ol style="padding-left: 20px; font-size: 12.5px; color: var(--text-secondary); display: flex; flex-direction: column; gap: 4px">
              <li>Open File Explorer and right-click <strong>This PC</strong> &rarr; select <strong>Map network drive...</strong></li>
              <li>Choose a drive letter (e.g. <code>Z:</code>).</li>
              <li>In the Folder field, paste: <code class="mono">{{ getWebdavMountUrl(connectingWebdav) }}</code></li>
              <li>Check <strong>Connect using different credentials</strong> and click <strong>Finish</strong>.</li>
              <li>Enter your Linux system username and password when prompted.</li>
            </ol>
          </div>
          <div>
            <div style="font-weight: 600; font-size: 13px; margin-bottom: 4px">Method 2: Command Prompt / PowerShell</div>
            <div style="background: var(--bg-base); border: 1px solid var(--border-subtle); border-radius: 6px; padding: 10px; font-family: monospace; font-size: 12px; color: var(--accent-primary)">
              net use Z: {{ getWebdavMountUrl(connectingWebdav) }} /user:&lt;username&gt;
            </div>
          </div>
        </div>

        <!-- macOS Tab -->
        <div v-if="connectActiveTab === 'macos'" style="display: flex; flex-direction: column; gap: 12px">
          <div>
            <div style="font-weight: 600; font-size: 13px; margin-bottom: 4px">Connect via Finder</div>
            <ol style="padding-left: 20px; font-size: 12.5px; color: var(--text-secondary); display: flex; flex-direction: column; gap: 4px">
              <li>In Finder, click menu <strong>Go</strong> &rarr; <strong>Connect to Server...</strong> (or press <kbd>⌘</kbd> + <kbd>K</kbd>).</li>
              <li>In the Server Address field, paste: <code class="mono">{{ getWebdavMountUrl(connectingWebdav) }}</code></li>
              <li>Click <strong>Connect</strong>.</li>
              <li>Select <strong>Registered User</strong> and enter your Linux credentials.</li>
            </ol>
          </div>
        </div>

        <!-- Linux Tab -->
        <div v-if="connectActiveTab === 'linux'" style="display: flex; flex-direction: column; gap: 12px">
          <div>
            <div style="font-weight: 600; font-size: 13px; margin-bottom: 4px">Method 1: Mount with davfs2</div>
            <div style="background: var(--bg-base); border: 1px solid var(--border-subtle); border-radius: 6px; padding: 10px; font-family: monospace; font-size: 12px; color: var(--accent-primary)">
              sudo mkdir -p /mnt/webdav<br>
              sudo mount -t davfs {{ getWebdavMountUrl(connectingWebdav) }} /mnt/webdav
            </div>
          </div>
          <div>
            <div style="font-weight: 600; font-size: 13px; margin-bottom: 4px">Method 2: GNOME Files / Nautilus</div>
            <ol style="padding-left: 20px; font-size: 12.5px; color: var(--text-secondary); display: flex; flex-direction: column; gap: 4px">
              <li>Open Files (Nautilus) and click <strong>Other Locations</strong>.</li>
              <li>In Connect to Server, enter: <code class="mono">dav://{{ currentHostname }}:8081/dav/{{ getWebdavShortName(connectingWebdav) }}/</code></li>
              <li>Click <strong>Connect</strong> and enter credentials.</li>
            </ol>
          </div>
        </div>

        <!-- Nextcloud & Mobile Tab -->
        <div v-if="connectActiveTab === 'browser'" style="display: flex; flex-direction: column; gap: 12px">
          <div>
            <div style="font-weight: 600; font-size: 13px; margin-bottom: 4px">Auto-Index Web File Browser</div>
            <p style="font-size: 12.5px; color: var(--text-secondary); margin-bottom: 8px">
              Opening the WebDAV URL in any browser serves an interactive auto-index directory view with instant file filtering, size display, and direct download links.
            </p>
          </div>
          <div>
            <div style="font-weight: 600; font-size: 13px; margin-bottom: 4px">Nextcloud External Storage</div>
            <div style="background: var(--bg-base); border: 1px solid var(--border-subtle); border-radius: 6px; padding: 10px; font-size: 12px; color: var(--text-secondary); display: flex; flex-direction: column; gap: 4px">
              <div><strong>Storage type:</strong> WebDAV</div>
              <div><strong>Host:</strong> {{ currentHostname }} (Port: 8081)</div>
              <div><strong>URL / Remote subfolder:</strong> /dav/{{ getWebdavShortName(connectingWebdav) }}/</div>
              <div><strong>Authentication:</strong> Log-in credentials, save in session</div>
            </div>
          </div>
        </div>
      </div>
      <div class="modal-footer" style="display: flex; justify-content: flex-end; padding: 12px 20px">
        <button class="btn btn-sm" @click="connectingWebdav = null">Close</button>
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

<style scoped>
.shares-view-container {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
}

.shares-subnav {
  display: flex;
  justify-content: flex-start;
  margin-bottom: 2px;
}

.subnav-pill-group {
  display: inline-flex;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 3px;
  gap: 4px;
}

.subnav-pill {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  background: transparent;
  border: none;
  border-radius: 6px;
  padding: 6px 14px;
  font-size: 13px;
  font-weight: 500;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.15s ease;
}

.subnav-pill:hover {
  color: var(--text-primary);
}

.subnav-pill.active {
  background: var(--bg-tertiary);
  color: var(--accent-primary);
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.15);
}

.subnav-badge {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  padding: 1px 7px;
  font-size: 11px;
  color: var(--text-primary);
  font-family: var(--font-mono, monospace);
}

.shares-wide-card {
  width: 100%;
}

.access-type-selector {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 10px;
}

.access-type-card {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  text-align: left;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 10px 14px;
  cursor: pointer;
  transition: all 0.15s ease;
  width: 100%;
}

.access-type-card:hover {
  border-color: var(--accent-primary);
  background: var(--bg-tertiary);
}

.access-type-card.selected {
  border-color: var(--accent-primary);
  background: rgba(56, 189, 248, 0.08);
  box-shadow: 0 0 0 1px var(--accent-primary);
}

.endpoint-cell-box {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.endpoint-url-row {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 2px 6px;
  max-width: fit-content;
}

.endpoint-url-text {
  font-family: var(--font-mono, monospace);
  font-size: 11.5px;
  color: var(--accent-primary);
  font-weight: 500;
  text-decoration: none;
}

.endpoint-url-text:hover {
  text-decoration: underline;
}

.connect-tab-btn {
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  padding: 8px 14px;
  font-size: 13px;
  font-weight: 500;
  color: var(--text-secondary);
  cursor: pointer;
}

.connect-tab-btn.active {
  color: var(--accent-primary);
  border-bottom-color: var(--accent-primary);
  font-weight: 600;
}
</style>
