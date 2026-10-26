<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import {
  mdiArchiveOutline,
  mdiContentCopy,
  mdiDatabaseCheck,
  mdiRefresh,
  mdiTrashCan,
  mdiUpload,
} from '@mdi/js';
import MdiIcon from '../components/MdiIcon.vue';
import SkeletonBox from '../components/SkeletonBox.vue';
import { services } from '../../container';
import { confirmModal, nasState, notify, refreshLxcTemplates as storeRefreshTemplates } from '../store';
import { generateNanosecondId, toZfsSafeSlug } from '../../domain/id';
import { type StorageImage } from '../../domain/models';

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

onMounted(async () => {
  await Promise.all([refreshLxc(), refreshTemplates()]);
});

// Templates Repository Actions
const templateFileInput = ref<HTMLInputElement | null>(null);
const isUploadingTemplate = ref(false);

function triggerTemplateUpload() {
  templateFileInput.value?.click();
}

async function refreshTemplates() {
  try {
    await storeRefreshTemplates();
  } catch (err: any) {
    console.error('Failed to refresh templates:', err);
  }
}

async function handleUploadTemplate(e: Event) {
  const target = e.target as HTMLInputElement;
  const file = target.files?.[0];
  if (!file) return;

  isUploadingTemplate.value = true;
  try {
    const uploaded = await services.lxc.uploadTemplate(file);
    notify('success', `Uploaded template ${uploaded.name} (${uploaded.sizeFormatted})`);
    await refreshTemplates();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to upload LXC template');
  } finally {
    isUploadingTemplate.value = false;
    if (templateFileInput.value) templateFileInput.value.value = '';
  }
}

async function handleDeleteTemplate(tmpl: StorageImage) {
  const confirmed = await confirmModal({
    title: 'Delete LXC Template',
    tone: 'danger',
    message: `Are you sure you want to delete template archive "${tmpl.name}"?`,
    details: `This will permanently remove ${tmpl.path} (${tmpl.sizeFormatted}) from the main pool.`,
    confirmText: 'Delete Template',
  });
  if (!confirmed) return;

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

async function handleDelete(name: string) {
  const shortName = name.replace(/^lxcContainers\//, '');
  const confirmed = await confirmModal({
    title: 'Delete LXC Container',
    tone: 'danger',
    message: `Are you sure you want to permanently delete container "${shortName}"?`,
    details: 'The container will be stopped, and its rootfs and container configuration will be deleted.',
    confirmText: 'Delete Container',
  });
  if (!confirmed) return;

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
        <h2 class="card-title" style="font-size: 17px">
          LXC Linux System Containers (`lxcContainers/{lxc_container}`)
        </h2>
        <p class="card-subtitle">
          Lightweight unprivileged OS containers backed by ZFS rootfs datasets (`lxc-create`,
          `lxc-start`, `lxc-stop`, `lxc-freeze`).
        </p>
      </div>

      <button class="btn btn-primary" @click="toggleCreate">
        {{ showCreate ? 'Cancel' : '+ Create LXC Container' }}
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
          <small style="color: var(--text-muted); font-size: 11px">Human readable name</small>
        </div>
        <div class="form-group">
          <label>Container ID (`lxc_container_id`)</label>
          <input
            v-model="form.containerId"
            class="input mono"
            placeholder="e.g. debian-web-server"
            required
          />
          <small style="color: var(--text-muted); font-size: 11px">ZFS-safe slug computed from entity name</small>
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
          <label>CPU Cores (`lxc.cgroup2.cpuset.cpus`)</label>
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
              <option value="">-- Select from Templates Repository --</option>
              <option v-for="tmpl in nasState.lxcTemplates" :key="tmpl.path" :value="tmpl.path">
                {{ tmpl.name }} ({{ tmpl.sizeFormatted }})
              </option>
            </select>
          </div>
          <input
            v-model="form.templateArchive"
            class="input mono"
            placeholder="/nas/tank/lxc-templates/debian-12.tar.xz or custom archive path"
          />
          <div style="font-size: 11px; color: var(--text-muted); margin-top: 4px">
            Select a template from main pool repository (<code class="mono">/nas/{{ nasState.systemConfig?.mainPool || 'tank' }}/lxc-templates</code>) or specify custom archive path.
          </div>
        </div>
      </div>

      <div style="background: rgba(245, 158, 11, 0.08); border: 1px solid rgba(245, 158, 11, 0.3); border-radius: 6px; padding: 8px 12px; margin-top: 12px; font-size: 12px; display: flex; align-items: center; justify-content: space-between">
        <div>
          <span style="color: var(--text-secondary)">Rootfs Storage:</span>
          <span class="badge badge-main-pool" style="margin-left: 6px; display: inline-flex; align-items: center; gap: 4px">
            <MdiIcon :path="mdiDatabaseCheck" size="13" /> MAIN POOL ({{ nasState.systemConfig?.mainPool || 'tank' }})
          </span>
          <span class="mono" style="margin-left: 8px; color: var(--text-muted)">
            /nas/{{ nasState.systemConfig?.mainPool || 'tank' }}/containers/{{ form.containerId || '&lt;id&gt;' }}/rootfs
          </span>
        </div>
      </div>

      <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 14px">
        <div style="display: flex; gap: 16px; font-size: 12.5px">
          <label style="display: flex; align-items: center; gap: 6px">
            <input v-model="form.unprivileged" type="checkbox" /> Unprivileged UID Mapping
          </label>
          <label style="display: flex; align-items: center; gap: 6px">
            <input v-model="form.autostart" type="checkbox" /> Autostart (`lxc.start.auto = 1`)
          </label>
        </div>
        <button type="submit" class="btn btn-primary">Provision LXC Container</button>
      </div>
    </form>

    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <th>LXC Container</th>
            <th>State</th>
            <th>Distro / Release / Arch</th>
            <th>IPv4 Address</th>
            <th>Rootfs Path</th>
            <th>Resources</th>
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
              <td><SkeletonBox width="140px" height="14px" /></td>
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
            <td colspan="7" style="text-align: center; color: var(--text-secondary); padding: 36px">
              No LXC system containers found. Click <strong>"Create Container"</strong> above to provision one.
            </td>
          </tr>

          <tr v-for="lx in nasState.lxcContainers" :key="lx.name">
            <td>
              <div style="font-weight: 600">{{ lx.containerName }}</div>
              <div class="mono" style="font-size: 11px; color: var(--text-muted)">{{ lx.name }}</div>
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
              <div style="color: var(--accent-primary)">{{ lx.rootfsPath }}</div>
              <div v-if="lx.templateArchive" style="font-size: 11px; color: var(--text-muted); margin-top: 2px; display: inline-flex; align-items: center; gap: 4px">
                <MdiIcon :path="mdiArchiveOutline" size="13" /> {{ lx.templateArchive }}
              </div>
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
                  `:start`
                </button>
                <button
                  v-if="lx.state === 'RUNNING'"
                  class="btn btn-sm"
                  @click="handleAction(lx.name, 'freeze')"
                >
                  `:freeze`
                </button>
                <button
                  v-if="lx.state !== 'STOPPED'"
                  class="btn btn-sm"
                  @click="handleAction(lx.name, 'stop')"
                >
                  `:stop`
                </button>
                <button class="btn btn-danger btn-sm" @click="handleDelete(lx.name)">
                  Del
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
        <h2 class="card-title" style="font-size: 17px; display: flex; align-items: center; gap: 8px">
          <MdiIcon :path="mdiArchiveOutline" size="20" style="color: var(--accent-primary)" />
          LXC Templates Repository
        </h2>
        <p class="card-subtitle">
          OS rootfs template archives stored on main pool dataset
          <code class="mono">/nas/{{ nasState.systemConfig?.mainPool || 'tank' }}/lxc-templates</code>.
        </p>
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
          {{ isUploadingTemplate ? 'Uploading Template...' : 'Upload Template' }}
        </button>
      </div>
    </div>

    <!-- Templates Table -->
    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <th>Template Archive Name</th>
            <th>File Size</th>
            <th>Absolute Path</th>
            <th>Last Modified</th>
            <th style="text-align: right">Actions</th>
          </tr>
        </thead>
        <tbody>
          <!-- Skeleton Placeholders when Loading -->
          <template v-if="nasState.loading && nasState.lxcTemplates.length === 0">
            <tr v-for="r in 2" :key="r">
              <td><SkeletonBox width="160px" height="15px" /></td>
              <td><SkeletonBox width="70px" height="15px" /></td>
              <td><SkeletonBox width="260px" height="15px" /></td>
              <td><SkeletonBox width="120px" height="15px" /></td>
              <td style="text-align: right"><SkeletonBox width="60px" height="24px" rounded="4px" /></td>
            </tr>
          </template>

          <tr v-else-if="nasState.lxcTemplates.length === 0">
            <td colspan="5" style="text-align: center; color: var(--text-secondary); padding: 36px">
              No template archives found in repository. Click <strong>"Upload Template"</strong> to upload a rootfs archive (.tar.gz, .tar.xz, .tar.zst).
            </td>
          </tr>

          <tr v-for="tmpl in nasState.lxcTemplates" :key="tmpl.name">
            <td style="font-weight: 600">
              <div style="display: flex; align-items: center; gap: 8px">
                <MdiIcon :path="mdiArchiveOutline" size="16" style="color: var(--accent-primary)" />
                <span class="mono">{{ tmpl.name }}</span>
              </div>
            </td>
            <td class="mono">{{ tmpl.sizeFormatted }}</td>
            <td class="mono" style="font-size: 11px; color: var(--text-secondary)">
              {{ tmpl.path }}
            </td>
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
