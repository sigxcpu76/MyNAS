<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import {
  mdiArchiveOutline,
  mdiClose,
  mdiConsole,
  mdiContentCopy,
  mdiFileDocumentOutline,
  mdiFolder,
  mdiInformationOutline,
  mdiLayers,
  mdiPause,
  mdiPencil,
  mdiPlay,
  mdiPlus,
  mdiRefresh,
  mdiRestart,
  mdiSpeedometer,
  mdiStop,
  mdiTrashCan,
  mdiTrashCanOutline,
  mdiUpload,
} from '@mdi/js';
import MdiIcon from '../components/MdiIcon.vue';
import SkeletonBox from '../components/SkeletonBox.vue';
import SortableTh from '../components/SortableTh.vue';
import FileDropzone from '../components/FileDropzone.vue';
import PathSelector from '../components/PathSelector.vue';
import DockerTerminalModal, { type TerminalContainerOption } from '../components/DockerTerminalModal.vue';
import { useTableSort } from '../composables/useTableSort';
import { services } from '../../container';
import { nasState, notify, refreshLxcTemplates as storeRefreshTemplates } from '../store';
import { confirmDelete } from '../modal';
import { toZfsSafeSlug } from '../../domain/id';
import { formatBytes, type LxcContainer, type LxcMountPoint, type StorageImage } from '../../domain/models';

/** Converts a dotted IPv4 address into a sortable integer (null when absent/invalid). */
function ipv4ToNumber(ip: string | undefined): number | null {
  const parts = (ip || '').trim().split('/')[0].split('.');
  if (parts.length !== 4) return null;
  let n = 0;
  for (const part of parts) {
    const octet = Number(part);
    if (!Number.isInteger(octet) || octet < 0 || octet > 255) return null;
    n = n * 256 + octet;
  }
  return n;
}

const lxcSort = useTableSort(() => nasState.lxcContainers, {
  name: (lx) => lx.containerName,
  state: (lx) => lx.state,
  autostart: (lx) => (lx.autostart ? 1 : 0),
  distro: (lx) => `${lx.distribution} / ${lx.release} (${lx.architecture})`,
  ipv4: (lx) => ipv4ToNumber(lx.ipv4Address),
  // Primary key: CPU cores, secondary: memory limit (MiB)
  resources: (lx) => lx.cpuCores * 1e9 + lx.memoryLimitMib,
});

const templateSort = useTableSort(() => nasState.lxcTemplates, {
  name: (t) => t.name,
  size: (t) => t.sizeBytes,
  modified: (t) => (t.modTime ? new Date(t.modTime) : null),
});

// =============================================================================
// Create Container State
// =============================================================================
const showCreate = ref(false);
const form = reactive({
  containerName: '',
  containerId: '',
  distribution: 'debian',
  release: 'bookworm',
  architecture: 'amd64',
  memoryLimitMib: 1024,
  cpuCores: 2,
  networkBridge: 'lxcbr0',
  unprivileged: true,
  autostart: true,
  gpuEnabled: false,
  description: '',
  templateArchive: '',
  mountPoints: [] as LxcMountPoint[],
});

function onContainerNameInput() {
  form.containerId = toZfsSafeSlug(form.containerName);
}

function onDistributionChange() {
  if (!form.containerName) {
    form.containerId = toZfsSafeSlug(`${form.distribution}-${form.release}`);
  }
}

function addCreateMount() {
  form.mountPoints.push({
    hostPath: '',
    containerPath: '',
    readOnly: false,
  });
}

function removeCreateMount(index: number) {
  form.mountPoints.splice(index, 1);
}

function toggleCreate() {
  if (showCreate.value) {
    showCreate.value = false;
  } else {
    form.containerName = 'Debian Container';
    form.distribution = 'debian';
    form.release = 'bookworm';
    form.containerId = toZfsSafeSlug(form.containerName);
    form.architecture = 'amd64';
    form.memoryLimitMib = 1024;
    form.cpuCores = 2;
    form.networkBridge = 'lxcbr0';
    form.unprivileged = true;
    form.autostart = true;
    form.gpuEnabled = false;
    form.description = '';
    form.templateArchive = '';
    form.mountPoints = [];
    showCreate.value = true;
  }
}

async function handleCreate() {
  try {
    const targetId = toZfsSafeSlug(form.containerId || form.containerName || `${form.distribution}-${form.release}`) || 'container';
    const validMounts = form.mountPoints.filter((mp) => mp.hostPath.trim() && mp.containerPath.trim());
    const created = await services.lxc.createContainer(targetId, {
      containerName: form.containerName || targetId,
      distribution: form.distribution,
      release: form.release,
      architecture: form.architecture,
      memoryLimitMib: form.memoryLimitMib,
      cpuCores: form.cpuCores,
      networkBridge: form.networkBridge.trim() || 'lxcbr0',
      unprivileged: form.unprivileged,
      autostart: form.autostart,
      gpuEnabled: form.gpuEnabled,
      description: form.description.trim(),
      templateArchive: form.templateArchive.trim() || undefined,
      mountPoints: validMounts,
    });
    notify('success', `Created LXC system container ${created.containerName || targetId}`);
    form.containerName = '';
    form.containerId = '';
    form.templateArchive = '';
    form.mountPoints = [];
    showCreate.value = false;
    await refreshLxc();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create LXC container');
  }
}

// =============================================================================
// Autostart Toggle
// =============================================================================
const togglingAutostart = ref<Record<string, boolean>>({});

async function toggleAutostart(lx: LxcContainer) {
  if (togglingAutostart.value[lx.name]) return;
  togglingAutostart.value[lx.name] = true;
  try {
    const updated = await services.lxc.updateContainer(
      lx.name,
      { autostart: !lx.autostart },
      ['autostart']
    );
    lx.autostart = updated.autostart;
    notify('success', `Autostart ${updated.autostart ? 'enabled' : 'disabled'} for ${lx.containerName}`);
    if (inspectedContainer.value && inspectedContainer.value.name === lx.name) {
      inspectedContainer.value.autostart = updated.autostart;
    }
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update autostart setting');
  } finally {
    togglingAutostart.value[lx.name] = false;
  }
}

// =============================================================================
// Container Lifecycle & Actions
// =============================================================================
async function handleAction(name: string, verb: 'start' | 'stop' | 'freeze') {
  try {
    if (verb === 'start') await services.lxc.startContainer(name);
    else if (verb === 'stop') await services.lxc.stopContainer(name);
    else await services.lxc.freezeContainer(name);
    notify('success', `LXC :${verb} succeeded on ${name}`);
    await refreshLxc();
  } catch (err: any) {
    notify('error', err?.message || `Failed to ${verb} LXC container`);
  }
}

async function handleRestart(lx: LxcContainer) {
  try {
    await services.lxc.restartContainer(lx.name);
    notify('success', `Restarted LXC container ${lx.containerName}`);
    await refreshLxc();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to restart LXC container');
  }
}

async function handleDelete(lx: LxcContainer) {
  const name = lx.name;
  const shortName = name.replace(/^lxcContainers\//, '');
  const res = await confirmDelete({
    kind: 'Container',
    name: lx.containerName || shortName,
    details: 'The container will be stopped and its rootfs and configuration deleted.',
    confirmText: 'Delete Container',
  });
  if (!res.confirmed) return;

  try {
    await services.lxc.deleteContainer(name, true);
    notify('info', `Deleted LXC container ${shortName}`);
    if (inspectedContainer.value && inspectedContainer.value.name === name) {
      isInspectModalOpen.value = false;
    }
    await refreshLxc();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete LXC container');
  }
}

// =============================================================================
// Edit Container Modal
// =============================================================================
const isEditModalOpen = ref(false);
const isSavingEdit = ref(false);
const editingContainer = ref<LxcContainer | null>(null);
const editForm = reactive({
  containerName: '',
  description: '',
  cpuCores: 2,
  memoryLimitMib: 1024,
  networkBridge: 'lxcbr0',
  autostart: true,
  unprivileged: true,
  gpuEnabled: false,
  mountPoints: [] as LxcMountPoint[],
});

function openEditModal(lx: LxcContainer) {
  editingContainer.value = lx;
  editForm.containerName = lx.containerName || lx.name.replace(/^lxcContainers\//, '');
  editForm.description = lx.description || '';
  editForm.cpuCores = lx.cpuCores || 2;
  editForm.memoryLimitMib = lx.memoryLimitMib || 1024;
  editForm.networkBridge = lx.networkBridge || 'lxcbr0';
  editForm.autostart = Boolean(lx.autostart);
  editForm.unprivileged = Boolean(lx.unprivileged);
  editForm.gpuEnabled = Boolean(lx.gpuEnabled);
  editForm.mountPoints = (lx.mountPoints || []).map((mp) => ({
    hostPath: mp.hostPath,
    containerPath: mp.containerPath,
    readOnly: Boolean(mp.readOnly),
  }));
  isEditModalOpen.value = true;
}

function addEditMount() {
  editForm.mountPoints.push({
    hostPath: '',
    containerPath: '',
    readOnly: false,
  });
}

function removeEditMount(index: number) {
  editForm.mountPoints.splice(index, 1);
}

async function handleSaveEdit() {
  if (!editingContainer.value) return;
  isSavingEdit.value = true;
  try {
    const validMounts = editForm.mountPoints.filter((mp) => mp.hostPath.trim() && mp.containerPath.trim());
    const updated = await services.lxc.updateContainer(editingContainer.value.name, {
      containerName: editForm.containerName.trim(),
      description: editForm.description.trim(),
      cpuCores: editForm.cpuCores,
      memoryLimitMib: editForm.memoryLimitMib,
      networkBridge: editForm.networkBridge.trim() || 'lxcbr0',
      autostart: editForm.autostart,
      unprivileged: editForm.unprivileged,
      gpuEnabled: editForm.gpuEnabled,
      mountPoints: validMounts,
    });
    notify('success', `Updated configuration for ${updated.containerName}`);
    isEditModalOpen.value = false;
    await refreshLxc();
    if (inspectedContainer.value && inspectedContainer.value.name === updated.name) {
      inspectedContainer.value = updated;
    }
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update container configuration');
  } finally {
    isSavingEdit.value = false;
  }
}

// =============================================================================
// Container Inspector Modal & Logs
// =============================================================================
const isInspectModalOpen = ref(false);
const inspectedContainer = ref<LxcContainer | null>(null);
const inspectActiveTab = ref<'overview' | 'mounts' | 'logs'>('overview');
const logTail = ref(100);
const logsContent = ref('');
const isLoadingLogs = ref(false);

function openInspectModal(lx: LxcContainer, defaultTab: 'overview' | 'mounts' | 'logs' = 'overview') {
  inspectedContainer.value = lx;
  inspectActiveTab.value = defaultTab;
  isInspectModalOpen.value = true;
  if (defaultTab === 'logs') {
    fetchLogs();
  }
}

async function fetchLogs() {
  if (!inspectedContainer.value) return;
  isLoadingLogs.value = true;
  try {
    const logs = await services.lxc.getContainerLogs(inspectedContainer.value.name, logTail.value);
    logsContent.value = logs || 'No log entries available for this container.';
  } catch (err: any) {
    logsContent.value = `Failed to fetch logs: ${err?.message || 'Unknown error'}`;
  } finally {
    isLoadingLogs.value = false;
  }
}

function copyLogs() {
  navigator.clipboard.writeText(logsContent.value);
  notify('info', 'Logs copied to clipboard');
}

// =============================================================================
// Interactive Web Terminal Modal
// =============================================================================
const isTerminalOpen = ref(false);
const activeTerminalContainer = ref<LxcContainer | null>(null);

function openTerminal(lx: LxcContainer) {
  if (lx.state !== 'RUNNING') {
    notify('info', `Container "${lx.containerName}" is ${lx.state}. Start the container to access console.`);
    return;
  }
  activeTerminalContainer.value = lx;
  isTerminalOpen.value = true;
}

const terminalContainerOptions = computed<TerminalContainerOption[]>(() => {
  if (!activeTerminalContainer.value) return [];
  const id = activeTerminalContainer.value.name.replace(/^lxcContainers\//, '');
  return [
    {
      id,
      name: activeTerminalContainer.value.containerName || id,
      state: activeTerminalContainer.value.state,
    },
  ];
});

// =============================================================================
// Polling & Lifecycle
// =============================================================================
async function refreshLxc() {
  nasState.lxcContainers = await services.lxc.listContainers();
}

let pollTimer: ReturnType<typeof setInterval> | null = null;

onMounted(async () => {
  await Promise.all([refreshLxc(), refreshTemplates()]);
  pollTimer = setInterval(async () => {
    try {
      nasState.lxcContainers = await services.lxc.listContainers();
      if (inspectedContainer.value) {
        const found = nasState.lxcContainers.find((c) => c.name === inspectedContainer.value?.name);
        if (found) inspectedContainer.value = found;
      }
    } catch {
      // background polling error ignored
    }
  }, 3000);
});

onUnmounted(() => {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
});

// =============================================================================
// Templates Repository
// =============================================================================
const templateFileInput = ref<HTMLInputElement | null>(null);
const templateDropzoneRef = ref<any>(null);
const isUploadingTemplate = ref(false);
const templateUploadProgress = ref<{ percent: number; loaded: number; total: number; filename: string } | null>(null);

function triggerTemplateUpload() {
  if (templateDropzoneRef.value) {
    templateDropzoneRef.value.openFileDialog();
  } else {
    templateFileInput.value?.click();
  }
}

async function refreshTemplates() {
  try {
    await storeRefreshTemplates();
  } catch (err: any) {
    console.error('Failed to refresh templates:', err);
  }
}

async function uploadTemplateFile(file: File) {
  if (!file) return;

  isUploadingTemplate.value = true;
  templateUploadProgress.value = {
    percent: 0,
    loaded: 0,
    total: file.size,
    filename: file.name,
  };

  try {
    const uploaded = await services.lxc.uploadTemplate(file, (p) => {
      templateUploadProgress.value = {
        percent: p.percent,
        loaded: p.loaded,
        total: p.total,
        filename: file.name,
      };
    });
    notify('success', `Uploaded template ${uploaded.name} (${uploaded.sizeFormatted})`);
    await refreshTemplates();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to upload LXC template');
  } finally {
    isUploadingTemplate.value = false;
    templateUploadProgress.value = null;
    if (templateFileInput.value) templateFileInput.value.value = '';
  }
}

async function handleUploadTemplate(e: Event) {
  const target = e.target as HTMLInputElement;
  const file = target.files?.[0];
  if (file) {
    await uploadTemplateFile(file);
  }
}

function handleInvalidTemplateFile(filename: string, reason: string) {
  notify('error', reason);
}

async function handleDeleteTemplate(tmpl: StorageImage) {
  const res = await confirmDelete({
    kind: 'Template',
    name: tmpl.name,
    details: `This will permanently remove ${tmpl.path} (${tmpl.sizeFormatted}) from the main pool.`,
    confirmText: 'Delete Template',
  });
  if (!res.confirmed) return;

  try {
    await services.lxc.deleteTemplate(tmpl.name);
    notify('info', `Deleted LXC template ${tmpl.name}`);
    await refreshTemplates();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete LXC template');
  }
}

function useTemplateForContainer(tmpl: StorageImage) {
  form.templateArchive = tmpl.path;
  if (!showCreate.value) {
    toggleCreate();
    form.templateArchive = tmpl.path;
  }
  notify('info', `Selected template: ${tmpl.name}`);
}

function copyTemplatePath(path: string) {
  navigator.clipboard.writeText(path);
  notify('info', `Copied path to clipboard: ${path}`);
}
</script>

<template>
  <div class="card">
    <div class="card-header">
      <div>
        <h2 class="card-title" style="font-size: 17px">LXC Containers</h2>
      </div>

      <button class="btn btn-primary" @click="toggleCreate">
        {{ showCreate ? 'Cancel' : '+ Create Container' }}
      </button>
    </div>

    <!-- Create Container Form -->
    <form
      v-if="showCreate"
      @submit.prevent="handleCreate"
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
          <label>Container Name</label>
          <input
            v-model="form.containerName"
            class="input"
            placeholder="e.g. Debian Web Server"
            @input="onContainerNameInput"
          />
        </div>
        <div class="form-group">
          <label>Container ID</label>
          <input
            v-model="form.containerId"
            class="input mono"
            placeholder="e.g. debian-web-server"
            required
          />
          <small style="color: var(--text-muted); font-size: 11px">Derived from container name</small>
        </div>
        <div class="form-group">
          <label>Distribution</label>
          <select v-model="form.distribution" class="select" @change="onDistributionChange">
            <option value="debian">debian</option>
            <option value="ubuntu">ubuntu</option>
            <option value="alpine">alpine</option>
            <option value="archlinux">archlinux</option>
          </select>
        </div>
        <div class="form-group">
          <label>Release</label>
          <input v-model="form.release" class="input mono" placeholder="bookworm / noble / 3.20" required />
        </div>
        <div class="form-group">
          <label>Architecture</label>
          <select v-model="form.architecture" class="select">
            <option value="amd64">amd64</option>
            <option value="arm64">arm64</option>
          </select>
        </div>
        <div class="form-group">
          <label>CPU Cores</label>
          <input v-model.number="form.cpuCores" type="number" min="1" max="64" class="input mono" />
        </div>
        <div class="form-group">
          <label>Memory Limit (MiB)</label>
          <input v-model.number="form.memoryLimitMib" type="number" min="128" step="128" class="input mono" />
        </div>
        <div class="form-group">
          <label>Network Bridge</label>
          <input v-model="form.networkBridge" class="input mono" placeholder="e.g. lxcbr0 or br0" />
        </div>
        <div class="form-group" style="grid-column: span 2">
          <label>Description (Optional)</label>
          <input v-model="form.description" class="input" placeholder="e.g. Production microservice container" />
        </div>
        <div class="form-group" style="grid-column: span 2">
          <label>Template Archive (.tar.gz / .tar.xz / .tar.zst) (Optional)</label>
          <div v-if="nasState.lxcTemplates.length > 0" style="margin-bottom: 6px">
            <select
              class="select mono"
              :value="form.templateArchive"
              @change="(e) => form.templateArchive = (e.target as HTMLSelectElement).value"
            >
              <option value="">-- Select a template --</option>
              <option v-for="tmpl in nasState.lxcTemplates" :key="tmpl.path" :value="tmpl.path">
                {{ tmpl.name }} ({{ tmpl.sizeFormatted }})
              </option>
            </select>
          </div>
          <PathSelector
            v-model="form.templateArchive"
            placeholder="/mnt/tank/lxc-templates/debian-12.tar.xz or custom archive path"
            root-hint="/mnt"
          />
        </div>
      </div>

      <!-- Storage Mounts Section -->
      <div style="margin-top: 14px; padding-top: 12px; border-top: 1px solid var(--border-subtle)">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
          <div>
            <h4 style="font-size: 13px; font-weight: 600; margin: 0; color: var(--text-primary)">
              Storage Bind Mounts
            </h4>
            <span style="font-size: 11px; color: var(--text-muted)">
              Map host ZFS datasets or directories into the container filesystem
            </span>
          </div>
          <button type="button" class="btn btn-xs" @click="addCreateMount">
            <MdiIcon :path="mdiPlus" size="13" style="margin-right: 4px" /> Add Mount Point
          </button>
        </div>

        <div v-if="form.mountPoints.length === 0" style="font-size: 12px; color: var(--text-muted); font-style: italic; padding: 4px 0">
          No bind mounts configured.
        </div>

        <div v-else style="display: flex; flex-direction: column; gap: 8px; margin-bottom: 8px">
          <div
            v-for="(mp, idx) in form.mountPoints"
            :key="idx"
            style="display: grid; grid-template-columns: 1fr 1fr auto auto; gap: 8px; align-items: center"
          >
            <input
              v-model="mp.hostPath"
              class="input mono"
              style="font-size: 12px"
              placeholder="Host Path, e.g. /mnt/tank/media"
              required
            />
            <input
              v-model="mp.containerPath"
              class="input mono"
              style="font-size: 12px"
              placeholder="Container Path, e.g. /media"
              required
            />
            <label style="display: flex; align-items: center; gap: 4px; font-size: 12px; white-space: nowrap">
              <input v-model="mp.readOnly" type="checkbox" /> Read Only
            </label>
            <button
              type="button"
              class="btn btn-xs btn-danger"
              style="padding: 4px"
              title="Remove Mount"
              @click="removeCreateMount(idx)"
            >
              <MdiIcon :path="mdiTrashCanOutline" size="14" />
            </button>
          </div>
        </div>
      </div>

      <!-- Flags & Actions -->
      <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 14px; padding-top: 10px; border-top: 1px solid var(--border-subtle)">
        <div style="display: flex; gap: 16px; font-size: 12.5px; flex-wrap: wrap">
          <label style="display: flex; align-items: center; gap: 6px; cursor: pointer">
            <input v-model="form.unprivileged" type="checkbox" /> Unprivileged
          </label>
          <label style="display: flex; align-items: center; gap: 6px; cursor: pointer">
            <input v-model="form.autostart" type="checkbox" /> Autostart on boot
          </label>
          <label style="display: flex; align-items: center; gap: 6px; cursor: pointer">
            <input v-model="form.gpuEnabled" type="checkbox" /> GPU Acceleration (/dev/dri, nvidia)
          </label>
        </div>
        <button type="submit" class="btn btn-primary">Create Container</button>
      </div>
    </form>

    <!-- LXC Containers Table -->
    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <SortableTh :sort="lxcSort" sort-key="name">Name</SortableTh>
            <SortableTh :sort="lxcSort" sort-key="state">State</SortableTh>
            <SortableTh :sort="lxcSort" sort-key="autostart">Autostart</SortableTh>
            <SortableTh :sort="lxcSort" sort-key="distro">Distro / Release / Arch</SortableTh>
            <SortableTh :sort="lxcSort" sort-key="ipv4">IPv4 Address</SortableTh>
            <SortableTh :sort="lxcSort" sort-key="resources">Resources</SortableTh>
            <th style="text-align: right">Actions</th>
          </tr>
        </thead>
        <tbody>
          <!-- Moving Skeleton Placeholders when Loading -->
          <template v-if="nasState.loading && nasState.lxcContainers.length === 0">
            <tr v-for="r in 3" :key="r">
              <td>
                <div style="display: flex; flex-direction: column; gap: 4px">
                  <SkeletonBox width="130px" height="15px" />
                  <SkeletonBox width="90px" height="11px" />
                </div>
              </td>
              <td><SkeletonBox width="65px" height="20px" rounded="10px" /></td>
              <td><SkeletonBox width="60px" height="20px" rounded="10px" /></td>
              <td><SkeletonBox width="100px" height="14px" /></td>
              <td><SkeletonBox width="95px" height="14px" /></td>
              <td><SkeletonBox width="110px" height="14px" /></td>
              <td style="text-align: right">
                <div style="display: inline-flex; gap: 6px">
                  <SkeletonBox width="26px" height="26px" rounded="4px" />
                  <SkeletonBox width="26px" height="26px" rounded="4px" />
                </div>
              </td>
            </tr>
          </template>

          <tr v-else-if="nasState.lxcContainers.length === 0">
            <td colspan="7" style="text-align: center; color: var(--text-secondary); padding: 36px">
              No containers yet. Click "+ Create Container" above or upload a template below.
            </td>
          </tr>

          <tr v-for="lx in lxcSort.sorted.value" :key="lx.name">
            <!-- Name & ID -->
            <td>
              <div
                style="font-weight: 600; cursor: pointer; color: var(--text-primary)"
                @click="openInspectModal(lx, 'overview')"
              >
                {{ lx.containerName }}
              </div>
              <div class="mono" style="font-size: 11px; color: var(--text-muted)">
                {{ lx.name.replace('lxcContainers/', '') }}
              </div>
              <div v-if="lx.description" style="font-size: 11px; color: var(--text-secondary); margin-top: 2px">
                {{ lx.description }}
              </div>
            </td>

            <!-- State & Badges -->
            <td>
              <div style="display: flex; align-items: center; gap: 4px; flex-wrap: wrap">
                <span
                  class="badge"
                  :class="
                    lx.state === 'RUNNING'
                      ? 'badge-success'
                      : lx.state === 'FROZEN'
                      ? 'badge-warning'
                      : 'badge-neutral'
                  "
                >
                  {{ lx.state }}
                </span>
                <span v-if="lx.unprivileged" class="badge badge-info" title="Unprivileged user namespace">
                  Unpriv
                </span>
                <span v-if="lx.gpuEnabled" class="badge" style="background: rgba(16, 185, 129, 0.15); color: #10b981; border: 1px solid rgba(16, 185, 129, 0.3)" title="GPU Acceleration Enabled">
                  GPU
                </span>
              </div>
            </td>

            <!-- Autostart Toggle Badge -->
            <td>
              <button
                type="button"
                class="badge"
                :class="lx.autostart ? 'badge-success' : 'badge-neutral'"
                style="cursor: pointer; border: 1px solid var(--border-subtle); display: inline-flex; align-items: center; gap: 4px; font-size: 11px; padding: 2px 8px; transition: all 0.15s ease"
                :title="`Click to ${lx.autostart ? 'disable' : 'enable'} autostart on host boot`"
                :disabled="togglingAutostart[lx.name]"
                @click="toggleAutostart(lx)"
              >
                <span
                  style="display: inline-block; width: 6px; height: 6px; border-radius: 50%"
                  :style="{ background: lx.autostart ? 'var(--success)' : 'var(--text-muted)' }"
                ></span>
                {{ lx.autostart ? 'On Boot' : 'Manual' }}
              </button>
            </td>

            <!-- Distro / Release / Arch -->
            <td class="mono">
              {{ lx.distribution }} / {{ lx.release }} ({{ lx.architecture }})
            </td>

            <!-- IPv4 Address -->
            <td class="mono" style="color: var(--success-text)">
              {{ lx.ipv4Address || '—' }}
            </td>

            <!-- Resources -->
            <td class="mono" style="font-size: 12px">
              <div>{{ lx.cpuCores }} Cores · {{ lx.memoryLimitMib }} MiB</div>
              <div v-if="lx.mountPoints && lx.mountPoints.length > 0" style="font-size: 11px; color: var(--text-muted)">
                {{ lx.mountPoints.length }} bind mount{{ lx.mountPoints.length > 1 ? 's' : '' }}
              </div>
            </td>

            <!-- Actions Toolbar -->
            <td style="text-align: right">
              <div style="display: inline-flex; gap: 4px; flex-wrap: wrap; justify-content: flex-end">
                <!-- Start Button -->
                <button
                  v-if="lx.state !== 'RUNNING'"
                  class="btn btn-xs btn-success"
                  title="Start Container"
                  @click="handleAction(lx.name, 'start')"
                >
                  <MdiIcon :path="mdiPlay" size="13" />
                </button>

                <!-- Restart Button -->
                <button
                  v-if="lx.state === 'RUNNING'"
                  class="btn btn-xs"
                  title="Restart Container"
                  @click="handleRestart(lx)"
                >
                  <MdiIcon :path="mdiRestart" size="13" />
                </button>

                <!-- Freeze / Unfreeze -->
                <button
                  v-if="lx.state === 'RUNNING'"
                  class="btn btn-xs"
                  title="Freeze Container (pause execution)"
                  @click="handleAction(lx.name, 'freeze')"
                >
                  <MdiIcon :path="mdiPause" size="13" />
                </button>
                <button
                  v-else-if="lx.state === 'FROZEN'"
                  class="btn btn-xs btn-success"
                  title="Unfreeze Container"
                  @click="handleAction(lx.name, 'start')"
                >
                  <MdiIcon :path="mdiPlay" size="13" />
                </button>

                <!-- Stop Button -->
                <button
                  v-if="lx.state !== 'STOPPED'"
                  class="btn btn-xs"
                  title="Stop Container"
                  @click="handleAction(lx.name, 'stop')"
                >
                  <MdiIcon :path="mdiStop" size="13" />
                </button>

                <!-- Terminal Button -->
                <button
                  class="btn btn-xs btn-primary"
                  :disabled="lx.state !== 'RUNNING'"
                  title="Open Web Terminal Console"
                  @click="openTerminal(lx)"
                >
                  <MdiIcon :path="mdiConsole" size="13" />
                </button>

                <!-- Logs Button -->
                <button
                  class="btn btn-xs"
                  title="View Container Logs"
                  @click="openInspectModal(lx, 'logs')"
                >
                  <MdiIcon :path="mdiFileDocumentOutline" size="13" />
                </button>

                <!-- Edit Button -->
                <button
                  class="btn btn-xs"
                  title="Edit Hardware & Settings"
                  @click="openEditModal(lx)"
                >
                  <MdiIcon :path="mdiPencil" size="13" />
                </button>

                <!-- Inspect Button -->
                <button
                  class="btn btn-xs"
                  title="Inspect Container Details"
                  @click="openInspectModal(lx, 'overview')"
                >
                  <MdiIcon :path="mdiInformationOutline" size="13" />
                </button>

                <!-- Delete Button -->
                <button
                  class="btn btn-xs btn-danger"
                  title="Delete Container"
                  @click="handleDelete(lx)"
                >
                  <MdiIcon :path="mdiTrashCan" size="13" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>

  <!-- LXC Templates Repository Card -->
  <div class="card" style="margin-top: 20px">
    <div class="card-header">
      <div>
        <h2 class="card-title" style="font-size: 17px">Templates</h2>
      </div>

      <div style="display: flex; gap: 10px; align-items: center">
        <input
          ref="templateFileInput"
          type="file"
          accept=".tar.gz,.tar.xz,.tar.zst,.tar,.tgz"
          style="display: none"
          @change="handleUploadTemplate"
        />
        <button class="btn btn-sm" @click="refreshTemplates">
          <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" /> Refresh
        </button>
        <button
          class="btn btn-sm btn-primary"
          :disabled="isUploadingTemplate"
          @click="triggerTemplateUpload"
        >
          <MdiIcon :path="mdiUpload" size="14" style="margin-right: 4px" />
          {{ isUploadingTemplate && templateUploadProgress ? `Uploading ${templateUploadProgress.percent}%...` : 'Upload Template' }}
        </button>
      </div>
    </div>

    <!-- Drag & Drop Template Dropzone -->
    <div style="margin: 0 16px 16px 16px">
      <FileDropzone
        ref="templateDropzoneRef"
        accept=".tar.gz,.tar.xz,.tar.zst,.tar,.tgz"
        label="Drag & drop template archive here, or click to browse"
        sublabel="Supports .tar.gz, .tar.xz, .tar.zst, .tar, .tgz templates"
        :icon="mdiArchiveOutline"
        :is-uploading="isUploadingTemplate"
        :progress="templateUploadProgress"
        @file-selected="uploadTemplateFile"
        @invalid-file="handleInvalidTemplateFile"
      />
    </div>

    <!-- Templates Table -->
    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <SortableTh :sort="templateSort" sort-key="name">Template Archive Name</SortableTh>
            <SortableTh :sort="templateSort" sort-key="size">File Size</SortableTh>
            <SortableTh :sort="templateSort" sort-key="modified">Last Modified</SortableTh>
            <th style="text-align: right">Actions</th>
          </tr>
        </thead>
        <tbody>
          <!-- Active Upload Row -->
          <tr v-if="isUploadingTemplate && templateUploadProgress">
            <td colspan="4" style="padding: 14px 16px; background: rgba(59, 130, 246, 0.04)">
              <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; font-size: 12px">
                <span class="mono" style="font-weight: 600">{{ templateUploadProgress.filename }} (Uploading)</span>
                <span class="mono">{{ formatBytes(templateUploadProgress.loaded) }} / {{ formatBytes(templateUploadProgress.total) }} ({{ templateUploadProgress.percent }}%)</span>
              </div>
              <div class="progress-bar" style="height: 6px">
                <div class="progress-fill" :style="{ width: `${templateUploadProgress.percent}%` }"></div>
              </div>
            </td>
          </tr>

          <!-- Skeleton Placeholders when Loading -->
          <template v-if="nasState.loading && nasState.lxcTemplates.length === 0">
            <tr v-for="r in 2" :key="r">
              <td><SkeletonBox width="160px" height="15px" /></td>
              <td><SkeletonBox width="70px" height="15px" /></td>
              <td><SkeletonBox width="120px" height="15px" /></td>
              <td style="text-align: right"><SkeletonBox width="60px" height="24px" rounded="4px" /></td>
            </tr>
          </template>

          <tr v-else-if="nasState.lxcTemplates.length === 0">
            <td colspan="4" style="text-align: center; color: var(--text-secondary); padding: 36px">
              No templates yet. Upload a .tar.gz, .tar.xz or .tar.zst archive.
            </td>
          </tr>

          <tr v-for="tmpl in templateSort.sorted.value" :key="tmpl.name">
            <td style="font-weight: 600">
              <div style="display: flex; align-items: center; gap: 8px">
                <MdiIcon :path="mdiArchiveOutline" size="16" style="color: var(--accent-primary)" />
                <span class="mono" :title="tmpl.path">{{ tmpl.name }}</span>
              </div>
            </td>
            <td class="mono">{{ tmpl.sizeFormatted }}</td>
            <td class="mono" style="font-size: 11px; color: var(--text-muted)">
              {{ tmpl.modTime ? new Date(tmpl.modTime).toLocaleString() : '—' }}
            </td>
            <td style="text-align: right">
              <div style="display: inline-flex; gap: 6px">
                <button
                  class="btn btn-xs btn-primary"
                  title="Use template to create container"
                  @click="useTemplateForContainer(tmpl)"
                >
                  Use Template
                </button>
                <button
                  class="btn btn-xs"
                  title="Copy absolute path"
                  @click="copyTemplatePath(tmpl.path)"
                >
                  <MdiIcon :path="mdiContentCopy" size="13" />
                </button>
                <button
                  class="btn btn-xs btn-danger"
                  title="Delete Template"
                  @click="handleDeleteTemplate(tmpl)"
                >
                  <MdiIcon :path="mdiTrashCan" size="13" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>

  <!-- ======================================================================= -->
  <!-- MODAL 1: Edit LXC Container                                            -->
  <!-- ======================================================================= -->
  <div v-if="isEditModalOpen && editingContainer" class="modal-backdrop">
    <div class="modal-card modal-card-large">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px">
        <div>
          <h3 style="font-size: 18px; font-weight: 700; color: var(--text-primary); margin: 0">
            Edit Container: {{ editingContainer.containerName }}
          </h3>
          <span class="mono" style="font-size: 11px; color: var(--text-muted)">
            {{ editingContainer.name }}
          </span>
        </div>
        <button class="btn btn-sm" style="padding: 2px 6px" @click="isEditModalOpen = false">
          <MdiIcon :path="mdiClose" size="18" />
        </button>
      </div>

      <form @submit.prevent="handleSaveEdit">
        <div class="form-grid">
          <div class="form-group">
            <label>Container Name</label>
            <input v-model="editForm.containerName" class="input" required />
          </div>
          <div class="form-group">
            <label>Network Bridge</label>
            <input v-model="editForm.networkBridge" class="input mono" placeholder="lxcbr0" />
          </div>
          <div class="form-group">
            <label>CPU Cores</label>
            <input v-model.number="editForm.cpuCores" type="number" min="1" max="64" class="input mono" required />
          </div>
          <div class="form-group">
            <label>Memory Limit (MiB)</label>
            <input v-model.number="editForm.memoryLimitMib" type="number" min="128" step="128" class="input mono" required />
          </div>
          <div class="form-group" style="grid-column: span 2">
            <label>Description</label>
            <input v-model="editForm.description" class="input" placeholder="e.g. Media server container" />
          </div>
        </div>

        <!-- Storage Mounts in Edit Modal -->
        <div style="margin-top: 14px; padding-top: 12px; border-top: 1px solid var(--border-subtle)">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
            <div>
              <h4 style="font-size: 13px; font-weight: 600; margin: 0; color: var(--text-primary)">
                Storage Bind Mounts
              </h4>
              <span style="font-size: 11px; color: var(--text-muted)">
                Map host datasets or folders into container mount paths
              </span>
            </div>
            <button type="button" class="btn btn-xs" @click="addEditMount">
              <MdiIcon :path="mdiPlus" size="13" style="margin-right: 4px" /> Add Mount Point
            </button>
          </div>

          <div v-if="editForm.mountPoints.length === 0" style="font-size: 12px; color: var(--text-muted); font-style: italic; padding: 4px 0">
            No bind mounts configured.
          </div>

          <div v-else style="display: flex; flex-direction: column; gap: 8px; margin-bottom: 12px">
            <div
              v-for="(mp, idx) in editForm.mountPoints"
              :key="idx"
              style="display: grid; grid-template-columns: 1fr 1fr auto auto; gap: 8px; align-items: center"
            >
              <input
                v-model="mp.hostPath"
                class="input mono"
                style="font-size: 12px"
                placeholder="Host Path, e.g. /mnt/tank/media"
                required
              />
              <input
                v-model="mp.containerPath"
                class="input mono"
                style="font-size: 12px"
                placeholder="Container Path, e.g. /media"
                required
              />
              <label style="display: flex; align-items: center; gap: 4px; font-size: 12px; white-space: nowrap">
                <input v-model="mp.readOnly" type="checkbox" /> Read Only
              </label>
              <button
                type="button"
                class="btn btn-xs btn-danger"
                style="padding: 4px"
                title="Remove Mount"
                @click="removeEditMount(idx)"
              >
                <MdiIcon :path="mdiTrashCanOutline" size="14" />
              </button>
            </div>
          </div>
        </div>

        <!-- Checkboxes -->
        <div style="display: flex; gap: 16px; margin-top: 14px; padding-top: 12px; border-top: 1px solid var(--border-subtle); flex-wrap: wrap">
          <label style="display: flex; align-items: center; gap: 6px; font-size: 13px; cursor: pointer">
            <input v-model="editForm.autostart" type="checkbox" /> Autostart on boot
          </label>
          <label style="display: flex; align-items: center; gap: 6px; font-size: 13px; cursor: pointer">
            <input v-model="editForm.unprivileged" type="checkbox" /> Unprivileged container
          </label>
          <label style="display: flex; align-items: center; gap: 6px; font-size: 13px; cursor: pointer">
            <input v-model="editForm.gpuEnabled" type="checkbox" /> GPU Passthrough (/dev/dri, nvidia)
          </label>
        </div>

        <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 18px">
          <button type="button" class="btn btn-secondary" @click="isEditModalOpen = false">
            Cancel
          </button>
          <button type="submit" class="btn btn-primary" :disabled="isSavingEdit">
            {{ isSavingEdit ? 'Saving...' : 'Save Changes' }}
          </button>
        </div>
      </form>
    </div>
  </div>

  <!-- ======================================================================= -->
  <!-- MODAL 2: LXC Container Inspector (Overview, Mounts, Logs)               -->
  <!-- ======================================================================= -->
  <div v-if="isInspectModalOpen && inspectedContainer" class="modal-backdrop">
    <div class="modal-card modal-card-large">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px; flex-shrink: 0">
        <div>
          <div style="display: flex; align-items: center; gap: 8px">
            <h3 style="font-size: 18px; font-weight: 700; color: var(--text-primary); margin: 0">
              {{ inspectedContainer.containerName }}
            </h3>
            <span
              class="badge"
              :class="
                inspectedContainer.state === 'RUNNING'
                  ? 'badge-success'
                  : inspectedContainer.state === 'FROZEN'
                  ? 'badge-warning'
                  : 'badge-neutral'
              "
            >
              {{ inspectedContainer.state }}
            </span>
            <button
              type="button"
              class="badge"
              :class="inspectedContainer.autostart ? 'badge-success' : 'badge-neutral'"
              style="cursor: pointer; border: 1px solid var(--border-subtle); display: inline-flex; align-items: center; gap: 4px; font-size: 11px; padding: 2px 8px; transition: all 0.15s ease"
              :title="`Click to ${inspectedContainer.autostart ? 'disable' : 'enable'} autostart on host boot`"
              @click="toggleAutostart(inspectedContainer)"
            >
              <span
                style="display: inline-block; width: 6px; height: 6px; border-radius: 50%"
                :style="{ background: inspectedContainer.autostart ? 'var(--success)' : 'var(--text-muted)' }"
              ></span>
              {{ inspectedContainer.autostart ? 'Autostart: On Boot' : 'Autostart: Manual' }}
            </button>
          </div>
          <p class="mono" style="font-size: 12px; color: var(--text-secondary); margin-top: 4px; margin-bottom: 0">
            {{ inspectedContainer.name }}
          </p>
        </div>

        <div style="display: flex; gap: 8px; align-items: center">
          <button
            class="btn btn-sm btn-primary"
            :disabled="inspectedContainer.state !== 'RUNNING'"
            title="Open Interactive Terminal"
            @click="openTerminal(inspectedContainer)"
          >
            <MdiIcon :path="mdiConsole" size="14" style="margin-right: 4px" /> Terminal
          </button>
          <button class="btn btn-sm" style="padding: 2px 6px" @click="isInspectModalOpen = false">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>
      </div>

      <!-- Inspector Tabs -->
      <div style="display: flex; gap: 8px; border-bottom: 1px solid var(--border-subtle); padding-bottom: 10px; margin-bottom: 14px; flex-shrink: 0">
        <button
          class="btn btn-sm"
          :class="inspectActiveTab === 'overview' ? 'btn-primary' : ''"
          @click="inspectActiveTab = 'overview'"
        >
          <MdiIcon :path="mdiSpeedometer" size="14" style="margin-right: 4px" />
          Overview
        </button>
        <button
          class="btn btn-sm"
          :class="inspectActiveTab === 'mounts' ? 'btn-primary' : ''"
          @click="inspectActiveTab = 'mounts'"
        >
          <MdiIcon :path="mdiFolder" size="14" style="margin-right: 4px" />
          Mount Points ({{ inspectedContainer.mountPoints?.length || 0 }})
        </button>
        <button
          class="btn btn-sm"
          :class="inspectActiveTab === 'logs' ? 'btn-primary' : ''"
          @click="inspectActiveTab = 'logs'; fetchLogs()"
        >
          <MdiIcon :path="mdiFileDocumentOutline" size="14" style="margin-right: 4px" />
          Logs
        </button>
      </div>

      <!-- Tab 1: Overview -->
      <div v-if="inspectActiveTab === 'overview'" style="display: flex; flex-direction: column; gap: 14px">
        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 12px">
          <div class="card" style="padding: 12px; background: var(--bg-surface)">
            <div style="font-size: 11px; color: var(--text-muted); text-transform: uppercase">Distribution</div>
            <div class="mono" style="font-size: 14px; font-weight: 600; margin-top: 4px">
              {{ inspectedContainer.distribution }} / {{ inspectedContainer.release }}
            </div>
            <div class="mono" style="font-size: 11px; color: var(--text-secondary)">{{ inspectedContainer.architecture }}</div>
          </div>

          <div class="card" style="padding: 12px; background: var(--bg-surface)">
            <div style="font-size: 11px; color: var(--text-muted); text-transform: uppercase">IPv4 Address</div>
            <div class="mono" style="font-size: 14px; font-weight: 600; margin-top: 4px; color: var(--success-text)">
              {{ inspectedContainer.ipv4Address || 'No IP assigned' }}
            </div>
            <div class="mono" style="font-size: 11px; color: var(--text-secondary)">Bridge: {{ inspectedContainer.networkBridge || 'lxcbr0' }}</div>
          </div>

          <div class="card" style="padding: 12px; background: var(--bg-surface)">
            <div style="font-size: 11px; color: var(--text-muted); text-transform: uppercase">Hardware Resources</div>
            <div class="mono" style="font-size: 14px; font-weight: 600; margin-top: 4px">
              {{ inspectedContainer.cpuCores }} vCPUs · {{ inspectedContainer.memoryLimitMib }} MiB RAM
            </div>
            <div style="font-size: 11px; color: var(--text-secondary)">
              {{ inspectedContainer.unprivileged ? 'Unprivileged Container' : 'Privileged Container' }}
            </div>
          </div>

          <div class="card" style="padding: 12px; background: var(--bg-surface)">
            <div style="font-size: 11px; color: var(--text-muted); text-transform: uppercase">GPU Passthrough</div>
            <div style="font-size: 14px; font-weight: 600; margin-top: 4px" :style="{ color: inspectedContainer.gpuEnabled ? 'var(--success-text)' : 'var(--text-muted)' }">
              {{ inspectedContainer.gpuEnabled ? 'Enabled (/dev/dri, nvidia)' : 'Disabled' }}
            </div>
          </div>
        </div>

        <div class="card" style="padding: 14px; background: var(--bg-surface)">
          <div style="font-weight: 600; font-size: 13px; margin-bottom: 8px">Storage Configuration</div>
          <div style="display: flex; flex-direction: column; gap: 6px; font-size: 12px">
            <div>
              <span style="color: var(--text-muted)">Rootfs Path: </span>
              <span class="mono">{{ inspectedContainer.rootfsPath }}</span>
            </div>
            <div v-if="inspectedContainer.templateArchive">
              <span style="color: var(--text-muted)">Base Template: </span>
              <span class="mono">{{ inspectedContainer.templateArchive }}</span>
            </div>
            <div v-if="inspectedContainer.description">
              <span style="color: var(--text-muted)">Description: </span>
              <span>{{ inspectedContainer.description }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Tab 2: Mount Points -->
      <div v-else-if="inspectActiveTab === 'mounts'">
        <div v-if="!inspectedContainer.mountPoints || inspectedContainer.mountPoints.length === 0" style="text-align: center; color: var(--text-secondary); padding: 32px">
          No bind mount points configured for this container.
        </div>
        <div v-else class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>Host Storage Path</th>
                <th>Container Mount Path</th>
                <th>Access Mode</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(mp, i) in inspectedContainer.mountPoints" :key="i">
                <td class="mono" style="font-size: 12px">{{ mp.hostPath }}</td>
                <td class="mono" style="font-size: 12px">{{ mp.containerPath }}</td>
                <td>
                  <span class="badge" :class="mp.readOnly ? 'badge-warning' : 'badge-success'">
                    {{ mp.readOnly ? 'Read Only' : 'Read / Write' }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Tab 3: Container Logs -->
      <div v-else-if="inspectActiveTab === 'logs'" style="display: flex; flex-direction: column; height: 480px">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
          <div style="display: flex; gap: 8px; align-items: center">
            <span style="font-size: 12px; color: var(--text-muted)">Lines:</span>
            <select v-model.number="logTail" class="select" style="padding: 2px 8px; font-size: 12px; height: 28px" @change="fetchLogs">
              <option :value="50">50</option>
              <option :value="100">100</option>
              <option :value="200">200</option>
              <option :value="500">500</option>
            </select>
          </div>
          <div style="display: flex; gap: 8px">
            <button class="btn btn-sm" @click="fetchLogs" :disabled="isLoadingLogs">
              <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" />
              {{ isLoadingLogs ? 'Refreshing...' : 'Refresh' }}
            </button>
            <button class="btn btn-sm" @click="copyLogs">
              <MdiIcon :path="mdiContentCopy" size="14" style="margin-right: 4px" /> Copy
            </button>
          </div>
        </div>

        <pre
          class="mono"
          style="
            flex: 1;
            background: #090d16;
            color: #d1d5db;
            border: 1px solid var(--border-subtle);
            border-radius: 6px;
            padding: 12px;
            font-size: 12px;
            line-height: 1.5;
            overflow-y: auto;
            white-space: pre-wrap;
            margin: 0;
          "
        >{{ logsContent }}</pre>
      </div>
    </div>
  </div>

  <!-- ======================================================================= -->
  <!-- MODAL 3: Interactive Container Web Terminal                            -->
  <!-- ======================================================================= -->
  <DockerTerminalModal
    v-if="isTerminalOpen && activeTerminalContainer"
    :app-title="`LXC Terminal: ${activeTerminalContainer.containerName || activeTerminalContainer.name.replace('lxcContainers/', '')}`"
    :containers="terminalContainerOptions"
    :initial-container-id="activeTerminalContainer.name.replace('lxcContainers/', '')"
    endpoint-type="lxc"
    @close="isTerminalOpen = false; activeTerminalContainer = null"
  />
</template>
