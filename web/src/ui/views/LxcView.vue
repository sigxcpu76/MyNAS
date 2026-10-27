<script setup lang="ts">
import { onMounted, onUnmounted, reactive, ref } from 'vue';
import {
  mdiArchiveOutline,
  mdiContentCopy,
  mdiRefresh,
  mdiTrashCan,
  mdiUpload,
} from '@mdi/js';
import MdiIcon from '../components/MdiIcon.vue';
import SkeletonBox from '../components/SkeletonBox.vue';
import SortableTh from '../components/SortableTh.vue';
import FileDropzone from '../components/FileDropzone.vue';
import { useTableSort } from '../composables/useTableSort';
import { services } from '../../container';
import { nasState, notify, refreshLxcTemplates as storeRefreshTemplates } from '../store';
import { confirmDelete } from '../modal';
import { generateNanosecondId, toZfsSafeSlug } from '../../domain/id';
import { formatBytes, type LxcContainer, type StorageImage } from '../../domain/models';

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

const showCreate = ref(false);
const form = reactive({
  containerName: '',
  containerId: '',
  distribution: 'debian',
  release: 'bookworm',
  architecture: 'amd64',
  memoryLimitMib: 1024,
  cpuCores: 2,
  unprivileged: true,
  autostart: true,
  templateArchive: '',
});

function onContainerNameInput() {
  form.containerId = toZfsSafeSlug(form.containerName);
}

function onDistributionChange() {
  if (!form.containerName) {
    form.containerId = toZfsSafeSlug(`${form.distribution}-${form.release}`);
  }
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
    form.unprivileged = true;
    form.autostart = true;
    form.templateArchive = '';
    showCreate.value = true;
  }
}

async function refreshLxc() {
  nasState.lxcContainers = await services.lxc.listContainers();
}

let pollTimer: ReturnType<typeof setInterval> | null = null;

onMounted(async () => {
  await Promise.all([refreshLxc(), refreshTemplates()]);
  pollTimer = setInterval(async () => {
    try {
      nasState.lxcContainers = await services.lxc.listContainers();
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

// Templates Repository Actions
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

async function handleCreate() {
  try {
    const targetId = toZfsSafeSlug(form.containerId || form.containerName || `${form.distribution}-${form.release}`) || 'container';
    const created = await services.lxc.createContainer(targetId, {
      containerName: form.containerName || targetId,
      distribution: form.distribution,
      release: form.release,
      architecture: form.architecture,
      memoryLimitMib: form.memoryLimitMib,
      cpuCores: form.cpuCores,
      unprivileged: form.unprivileged,
      autostart: form.autostart,
      templateArchive: form.templateArchive.trim() || undefined,
    });
    notify('success', `Created LXC system container ${created.containerName || targetId}`);
    form.containerName = '';
    form.containerId = '';
    form.templateArchive = '';
    showCreate.value = false;
    await refreshLxc();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create LXC container');
  }
}

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

/** Tooltip for the name cell: storage paths are kept out of the table itself. */
function lxcPathsTitle(lx: LxcContainer): string {
  const lines = [`Rootfs: ${lx.rootfsPath}`];
  if (lx.templateArchive) lines.push(`Template: ${lx.templateArchive}`);
  return lines.join('\n');
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
    await refreshLxc();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete LXC container');
  }
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
          <input v-model.number="form.cpuCores" type="number" min="1" max="32" class="input mono" />
        </div>
        <div class="form-group">
          <label>Memory Limit (MiB)</label>
          <input v-model.number="form.memoryLimitMib" type="number" min="128" step="128" class="input mono" />
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
          <input
            v-model="form.templateArchive"
            class="input mono"
            placeholder="/mnt/tank/lxc-templates/debian-12.tar.xz or custom archive path"
          />
        </div>
      </div>

      <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 14px">
        <div style="display: flex; gap: 16px; font-size: 12.5px">
          <label style="display: flex; align-items: center; gap: 6px">
            <input v-model="form.unprivileged" type="checkbox" /> Unprivileged
          </label>
          <label style="display: flex; align-items: center; gap: 6px">
            <input v-model="form.autostart" type="checkbox" /> Autostart
          </label>
        </div>
        <button type="submit" class="btn btn-primary">Create Container</button>
      </div>
    </form>

    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <SortableTh :sort="lxcSort" sort-key="name">Name</SortableTh>
            <SortableTh :sort="lxcSort" sort-key="state">State</SortableTh>
            <SortableTh :sort="lxcSort" sort-key="distro">Distro / Release / Arch</SortableTh>
            <SortableTh :sort="lxcSort" sort-key="ipv4">IPv4 Address</SortableTh>
            <SortableTh :sort="lxcSort" sort-key="resources">Resources</SortableTh>
            <th>Actions</th>
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
              <td><SkeletonBox width="100px" height="14px" /></td>
              <td><SkeletonBox width="95px" height="14px" /></td>
              <td><SkeletonBox width="110px" height="14px" /></td>
              <td>
                <div style="display: inline-flex; gap: 6px">
                  <SkeletonBox width="26px" height="26px" rounded="4px" />
                  <SkeletonBox width="26px" height="26px" rounded="4px" />
                </div>
              </td>
            </tr>
          </template>

          <tr v-else-if="nasState.lxcContainers.length === 0">
            <td colspan="6" style="text-align: center; color: var(--text-secondary); padding: 36px">
              No containers yet.
            </td>
          </tr>

          <tr v-for="lx in lxcSort.sorted.value" :key="lx.name">
            <td :title="lxcPathsTitle(lx)">
              <div style="font-weight: 600">{{ lx.containerName }}</div>
              <div class="mono" style="font-size: 11px; color: var(--text-muted)">{{ lx.name.replace('lxcContainers/', '') }}</div>
            </td>
            <td>
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
              <span v-if="lx.unprivileged" class="badge badge-info" style="margin-left: 4px">
                Unpriv
              </span>
            </td>
            <td class="mono">
              {{ lx.distribution }} / {{ lx.release }} ({{ lx.architecture }})
            </td>
            <td class="mono" style="color: var(--success-text)">
              {{ lx.ipv4Address || '—' }}
            </td>
            <td class="mono" style="font-size: 12px">
              {{ lx.cpuCores }} Cores · {{ lx.memoryLimitMib }} MiB
            </td>
            <td>
              <div style="display: flex; gap: 6px; flex-wrap: wrap">
                <button
                  v-if="lx.state !== 'RUNNING'"
                  class="btn btn-success btn-sm"
                  @click="handleAction(lx.name, 'start')"
                >
                  Start
                </button>
                <button
                  v-if="lx.state === 'RUNNING'"
                  class="btn btn-sm"
                  @click="handleAction(lx.name, 'freeze')"
                >
                  Freeze
                </button>
                <button
                  v-if="lx.state !== 'STOPPED'"
                  class="btn btn-sm"
                  @click="handleAction(lx.name, 'stop')"
                >
                  Stop
                </button>
                <button class="btn btn-danger btn-sm" @click="handleDelete(lx)">
                  Delete
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
</template>
