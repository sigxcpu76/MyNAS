<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { mdiCheckCircle, mdiAlertCircle, mdiClose, mdiDelete, mdiKeyVariant, mdiPencil, mdiPlus } from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import { services } from '../../container';
import { notify } from '../store';
import type { DockerRegistry } from '../../domain/models';

const emit = defineEmits<{ (e: 'close'): void }>();

const presets = [
  { label: 'Docker Hub', server: 'docker.io', hint: 'Use a Docker Hub personal access token (read-only is enough). Authenticated pulls avoid anonymous rate limits.' },
  { label: 'GitHub (GHCR)', server: 'ghcr.io', hint: 'Use a GitHub personal access token (classic) with the read:packages scope.' },
  { label: 'Quay.io', server: 'quay.io', hint: 'Use a Quay robot account or encrypted password.' },
  { label: 'GitLab', server: 'registry.gitlab.com', hint: 'Use a deploy token or personal access token with read_registry.' },
  { label: 'Custom', server: '', hint: 'Registry host, optionally with port, e.g. registry.example.com:5000.' },
];

const registries = ref<DockerRegistry[]>([]);
const loading = ref(false);
const saving = ref(false);
const formError = ref('');
const editing = ref<DockerRegistry | null>(null);
const showForm = ref(false);
const form = reactive({ preset: 'docker.io', server: 'docker.io', username: '', password: '', description: '' });

const activeHint = computed(
  () => presets.find((p) => p.server === form.preset)?.hint ?? presets[presets.length - 1].hint
);

async function load() {
  loading.value = true;
  try {
    registries.value = await services.apps.listRegistries();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to load registries');
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editing.value = null;
  formError.value = '';
  const unused = presets.find((p) => p.server && !registries.value.some((r) => r.server === p.server));
  form.preset = unused?.server ?? '';
  form.server = unused?.server ?? '';
  form.username = '';
  form.password = '';
  form.description = '';
  showForm.value = true;
}

function openEdit(r: DockerRegistry) {
  editing.value = r;
  formError.value = '';
  form.preset = presets.some((p) => p.server === r.server) ? r.server : '';
  form.server = r.server;
  form.username = r.username;
  form.password = '';
  form.description = r.description;
  showForm.value = true;
}

function onPresetChange() {
  if (form.preset) form.server = form.preset;
}

async function save() {
  formError.value = '';
  saving.value = true;
  try {
    const saved = await services.apps.saveRegistry(
      { server: form.server, username: form.username, password: form.password, description: form.description },
      editing.value?.name
    );
    notify('success', `Logged in to ${saved.server} as ${saved.username}`);
    showForm.value = false;
    await load();
  } catch (err: any) {
    formError.value = err?.message || 'Failed to save registry credentials';
  } finally {
    saving.value = false;
  }
}

async function remove(r: DockerRegistry) {
  if (!window.confirm(`Remove stored credentials for ${r.server}? Pulls from it will be anonymous.`)) return;
  try {
    await services.apps.deleteRegistry(r.name);
    notify('success', `Removed credentials for ${r.server}`);
    await load();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to remove registry');
  }
}

function serverLabel(server: string): string {
  return presets.find((p) => p.server === server)?.label ?? server;
}

onMounted(load);
</script>

<template>
  <div class="modal-backdrop">
    <div class="modal-card" style="max-width: 760px; width: 100%">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; padding-bottom: 12px; border-bottom: 1px solid var(--border-subtle)">
        <div style="display: flex; align-items: center; gap: 10px">
          <MdiIcon :path="mdiKeyVariant" size="22" style="color: var(--accent-primary)" />
          <h3 style="font-size: 18px; font-weight: 700; color: var(--text-primary); margin: 0">Container Registries</h3>
        </div>
        <button class="btn btn-sm" style="padding: 2px 6px" aria-label="Close" @click="emit('close')">
          <MdiIcon :path="mdiClose" size="18" />
        </button>
      </div>

      <p style="font-size: 13px; color: var(--text-secondary); margin: 0 0 14px; line-height: 1.5">
        Credentials used when apps pull images. Logins are verified before saving and stored on the system dataset.
        Passwords are never shown again.
      </p>

      <template v-if="!showForm">
        <div v-if="loading" style="padding: 20px; text-align: center; color: var(--text-secondary)">
          <span class="spinner-sm"></span>
        </div>
        <div
          v-else-if="registries.length === 0"
          style="padding: 24px 16px; text-align: center; border: 1px dashed var(--border-subtle); border-radius: 8px; color: var(--text-secondary); font-size: 13px"
        >
          No registry credentials yet. Image pulls are anonymous and subject to rate limits.
        </div>
        <table v-else class="data-table" style="width: 100%; border-collapse: collapse; font-size: 12.5px">
          <thead>
            <tr style="text-align: left; font-size: 11px; color: var(--text-secondary); text-transform: uppercase; border-bottom: 1px solid var(--border-subtle)">
              <th style="padding: 8px">Registry</th>
              <th style="padding: 8px">Username</th>
              <th style="padding: 8px">Status</th>
              <th style="padding: 8px; text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in registries" :key="r.name" style="border-bottom: 1px solid var(--border-subtle)">
              <td style="padding: 8px">
                <div style="font-weight: 600; color: var(--text-primary)">{{ serverLabel(r.server) }}</div>
                <div class="mono" style="font-size: 11px; color: var(--text-secondary)">{{ r.server }}</div>
                <div v-if="r.description" style="font-size: 11px; color: var(--text-secondary)">{{ r.description }}</div>
              </td>
              <td class="mono" style="padding: 8px">{{ r.username }}</td>
              <td style="padding: 8px">
                <span v-if="r.lastLoginStatus === 'ok'" class="badge badge-success" style="display: inline-flex; align-items: center; gap: 4px">
                  <MdiIcon :path="mdiCheckCircle" size="12" /> Verified
                </span>
                <span v-else class="badge badge-warning" style="display: inline-flex; align-items: center; gap: 4px" :title="r.lastLoginStatus">
                  <MdiIcon :path="mdiAlertCircle" size="12" /> {{ r.lastLoginStatus || 'Unverified' }}
                </span>
              </td>
              <td style="padding: 8px; text-align: right; white-space: nowrap">
                <button class="btn btn-sm btn-icon" title="Edit" aria-label="Edit" @click="openEdit(r)">
                  <MdiIcon :path="mdiPencil" size="15" />
                </button>
                <button class="btn btn-sm btn-icon btn-danger" style="margin-left: 6px" title="Remove" aria-label="Remove" @click="remove(r)">
                  <MdiIcon :path="mdiDelete" size="15" />
                </button>
              </td>
            </tr>
          </tbody>
        </table>

        <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 16px">
          <button class="btn" @click="emit('close')">Close</button>
          <button class="btn btn-primary" style="display: inline-flex; align-items: center; gap: 4px" @click="openCreate">
            <MdiIcon :path="mdiPlus" size="16" /> Add Registry
          </button>
        </div>
      </template>

      <form v-else @submit.prevent="save">
        <div v-if="formError" class="alert alert-danger" style="margin-bottom: 12px; padding: 10px 12px; border-radius: 6px; background: rgba(239, 68, 68, 0.12); color: var(--danger); font-size: 12.5px; white-space: pre-wrap">
          {{ formError }}
        </div>

        <div class="form-group">
          <label>Registry</label>
          <select v-model="form.preset" class="input" :disabled="saving" @change="onPresetChange">
            <option v-for="p in presets" :key="p.label" :value="p.server">{{ p.label }}{{ p.server ? ` (${p.server})` : '' }}</option>
          </select>
        </div>
        <div v-if="!form.preset" class="form-group">
          <label>Server</label>
          <input v-model="form.server" class="input mono" placeholder="registry.example.com:5000" :disabled="saving" required />
        </div>
        <div class="form-group">
          <label>Username</label>
          <input v-model="form.username" class="input mono" autocomplete="off" :disabled="saving" required />
        </div>
        <div class="form-group">
          <label>{{ editing ? 'Password / Access Token (leave empty to keep current)' : 'Password / Access Token' }}</label>
          <input v-model="form.password" class="input mono" type="password" autocomplete="new-password" :disabled="saving" :required="!editing" />
          <div style="font-size: 11.5px; color: var(--text-secondary); margin-top: 4px">{{ activeHint }}</div>
        </div>
        <div class="form-group">
          <label>Description (optional)</label>
          <input v-model="form.description" class="input" :disabled="saving" />
        </div>

        <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 16px">
          <button type="button" class="btn" :disabled="saving" @click="showForm = false">Back</button>
          <button type="submit" class="btn btn-primary" :disabled="saving" style="display: inline-flex; align-items: center; gap: 6px">
            <span v-if="saving" class="spinner-sm"></span>
            {{ saving ? 'Verifying login...' : editing ? 'Save & Verify' : 'Add & Verify' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>
