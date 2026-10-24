<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import {
  type ComposeAppFormModel,
  type ComposeEnvModel,
  type ComposePortModel,
  type ComposeServiceModel,
  type ComposeTopLevelNetwork,
  type ComposeTopLevelVolume,
  type ComposeVolumeMountModel,
  composeYamlToForm,
  createDefaultService,
  formToComposeYaml,
  generateUid,
} from '../../domain/composeModel';
import { nasState, notify } from '../store';
import CodeMirrorEditor from './CodeMirrorEditor.vue';
import MdiIcon from './MdiIcon.vue';
import PathSelector from './PathSelector.vue';
import { mdiContentCopy, mdiDocker, mdiHarddisk, mdiLanConnect } from '@mdi/js';

const props = defineProps<{
  initialAppId?: string;
  initialTitle?: string;
  initialComposeYaml?: string;
  isEditMode?: boolean;
}>();

const emit = defineEmits<{
  (e: 'save', data: { appId: string; title: string; composeYaml: string }): void;
  (e: 'cancel'): void;
}>();

// Editor View Mode: 'form' (default user-friendly) vs 'raw' (on demand)
const editorMode = ref<'form' | 'raw'>('form');

// Form Model State
const form = reactive<ComposeAppFormModel>(
  composeYamlToForm(
    props.initialComposeYaml || '',
    props.initialAppId || '',
    props.initialTitle || ''
  )
);

// Raw YAML buffer for raw mode
const rawYaml = ref(props.initialComposeYaml || formToComposeYaml(form));

// Navigation within Form Editor
const activeServiceId = ref(form.services[0]?.id || '');
const currentTopTab = ref<'services' | 'networks' | 'volumes'>('services');
const serviceSubTab = ref<'general' | 'ports' | 'env' | 'volumes' | 'resources' | 'networks'>('general');

// Bulk environment paste state
const showBulkEnv = ref(false);
const bulkEnvText = ref('');

// Computed active service
const activeService = computed(() => {
  return form.services.find((s) => s.id === activeServiceId.value) || form.services[0];
});

// Other services available for depends_on
const otherServices = computed(() => {
  if (!activeService.value) return [];
  return form.services
    .filter((s) => s.id !== activeService.value.id)
    .map((s) => s.name.trim())
    .filter(Boolean);
});

// Top-level network names
const availableNetworkNames = computed(() => {
  return form.networks.map((n) => n.name.trim()).filter(Boolean);
});

// ZFS datasets suggestion list from store
const zfsDatasetPaths = computed(() => {
  const paths: string[] = [];
  for (const [pool, datasets] of Object.entries(nasState.datasetsByPool || {})) {
    const cleanPool = pool.replace(/^zfsPools\//, '');
    for (const d of datasets) {
      if (d.mountpoint && d.mountpoint !== 'none' && d.mountpoint !== 'legacy') {
        paths.push(d.mountpoint);
      } else {
        paths.push(`/${cleanPool}/${d.name.split('/').pop()}`);
      }
    }
  }
  return paths;
});

const appsPoolName = computed(() => nasState.dockerConfig?.pool || 'tank');
const appsPoolAppdataDir = computed(
  () => nasState.dockerConfig?.appdataDataset || `/${appsPoolName.value}/appdata`
);

function defaultAppdataMount(appId: string, subfolder = 'data'): string {
  const cleanId = appId.trim() || 'app';
  return `${appsPoolAppdataDir.value}/${cleanId}/${subfolder}`;
}

// Watch mode changes: sync form <-> rawYaml
watch(editorMode, (newMode) => {
  if (newMode === 'raw') {
    rawYaml.value = formToComposeYaml(form);
  } else {
    try {
      const parsed = composeYamlToForm(rawYaml.value, form.appId, form.title);
      form.services = parsed.services;
      form.networks = parsed.networks;
      form.volumes = parsed.volumes;
      if (!form.services.some((s) => s.id === activeServiceId.value)) {
        activeServiceId.value = form.services[0]?.id || '';
      }
    } catch (err: any) {
      notify('error', `Failed to parse YAML into form: ${err?.message || 'Invalid syntax'}`);
    }
  }
});

// Service management
function addService() {
  const name = `service-${form.services.length + 1}`;
  const svc = createDefaultService(name);
  form.services.push(svc);
  activeServiceId.value = svc.id;
  currentTopTab.value = 'services';
}

function removeService(id: string) {
  if (form.services.length <= 1) {
    notify('error', 'An App must have at least one service');
    return;
  }
  const idx = form.services.findIndex((s) => s.id === id);
  if (idx !== -1) {
    form.services.splice(idx, 1);
    activeServiceId.value = form.services[0].id;
  }
}

function duplicateService(svc: ComposeServiceModel) {
  const cloned: ComposeServiceModel = JSON.parse(JSON.stringify(svc));
  cloned.id = generateUid();
  cloned.name = `${svc.name}-copy`;
  cloned.containerName = svc.containerName ? `${svc.containerName}-copy` : '';
  form.services.push(cloned);
  activeServiceId.value = cloned.id;
}

// Ports
function addPort(svc: ComposeServiceModel) {
  svc.ports.push({ hostPort: '', containerPort: '', protocol: 'tcp' });
}

function removePort(svc: ComposeServiceModel, idx: number) {
  svc.ports.splice(idx, 1);
}

// Environment
function addEnv(svc: ComposeServiceModel) {
  svc.environment.push({ key: '', value: '' });
}

function removeEnv(svc: ComposeServiceModel, idx: number) {
  svc.environment.splice(idx, 1);
}

function openBulkEnvModal() {
  if (!activeService.value) return;
  bulkEnvText.value = activeService.value.environment
    .map((e) => `${e.key}=${e.value}`)
    .join('\n');
  showBulkEnv.value = true;
}

function applyBulkEnv() {
  if (!activeService.value) return;
  const lines = bulkEnvText.value.split('\n');
  const envs: ComposeEnvModel[] = [];
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;
    const idx = trimmed.indexOf('=');
    if (idx !== -1) {
      envs.push({
        key: trimmed.substring(0, idx).trim(),
        value: trimmed.substring(idx + 1).trim(),
      });
    } else {
      envs.push({ key: trimmed, value: '' });
    }
  }
  activeService.value.environment = envs;
  showBulkEnv.value = false;
  notify('info', `Imported ${envs.length} environment variables`);
}

// Volumes
function addVolumeMount(svc: ComposeServiceModel) {
  svc.volumes.push({ source: '', target: '', mode: 'rw' });
}

function removeVolumeMount(svc: ComposeServiceModel, idx: number) {
  svc.volumes.splice(idx, 1);
}

// Top-Level Networks
function addTopLevelNetwork() {
  form.networks.push({
    id: generateUid(),
    name: `net-${form.networks.length + 1}`,
    driver: 'bridge',
    internal: false,
  });
}

function removeTopLevelNetwork(idx: number) {
  form.networks.splice(idx, 1);
}

// Top-Level Volumes
function addTopLevelVolume() {
  form.volumes.push({
    id: generateUid(),
    name: `vol-${form.volumes.length + 1}`,
    driver: 'local',
    external: false,
  });
}

function removeTopLevelVolume(idx: number) {
  form.volumes.splice(idx, 1);
}

// Device passthrough
function addDevice(svc: ComposeServiceModel) {
  svc.devices.push('/dev/dri:/dev/dri');
}

function removeDevice(svc: ComposeServiceModel, idx: number) {
  svc.devices.splice(idx, 1);
}

// Submission
function handleSave() {
  if (!form.appId.trim()) {
    notify('error', 'App ID is required');
    return;
  }

  // Validate that every service has a name and image
  for (const s of form.services) {
    if (!s.name.trim()) {
      notify('error', 'All services must have a valid service name');
      return;
    }
    if (!s.image.trim()) {
      notify('error', `Service "${s.name}" is missing a container image tag`);
      return;
    }
  }

  const finalYaml = editorMode.value === 'raw' ? rawYaml.value : formToComposeYaml(form);
  emit('save', {
    appId: form.appId.trim(),
    title: form.title.trim() || form.appId.trim(),
    composeYaml: finalYaml,
  });
}

function copyYaml() {
  const yamlOut = editorMode.value === 'raw' ? rawYaml.value : formToComposeYaml(form);
  navigator.clipboard.writeText(yamlOut);
  notify('info', 'Compose YAML copied to clipboard');
}
</script>

<template>
  <div class="compose-editor" style="display: flex; flex-direction: column; flex: 1; min-height: 0">
    <!-- Top Metadata Bar -->
    <div style="background: var(--bg-card); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 14px 16px; margin-bottom: 16px">
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px">
        <div style="display: flex; gap: 14px; flex: 1; min-width: 320px">
          <div style="flex: 1">
            <label class="form-label" style="margin-bottom: 4px; font-size: 12px">
              App ID *
            </label>
            <input
              v-model="form.appId"
              type="text"
              class="form-input mono"
              :disabled="props.isEditMode"
              placeholder="e.g. nextcloud, plex, jellyfin"
              style="padding: 6px 10px; font-size: 13px"
            />
          </div>
          <div style="flex: 1">
            <label class="form-label" style="margin-bottom: 4px; font-size: 12px">
              Display Title
            </label>
            <input
              v-model="form.title"
              type="text"
              class="form-input"
              placeholder="e.g. Nextcloud Private Cloud"
              style="padding: 6px 10px; font-size: 13px"
            />
          </div>
        </div>

        <!-- Mode Toggle: Form vs Raw YAML -->
        <div style="display: flex; align-items: center; gap: 8px">
          <div style="display: flex; background: rgba(0, 0, 0, 0.25); border: 1px solid var(--border-subtle); border-radius: 6px; overflow: hidden">
            <button
              class="btn btn-sm"
              :style="{
                background: editorMode === 'form' ? 'var(--accent-primary)' : 'transparent',
                color: editorMode === 'form' ? '#fff' : 'var(--text-secondary)',
                border: 'none',
                borderRadius: 0,
                padding: '6px 14px',
                fontWeight: 600,
              }"
              @click="editorMode = 'form'"
            >
              Form
            </button>
            <button
              class="btn btn-sm"
              :style="{
                background: editorMode === 'raw' ? 'var(--accent-primary)' : 'transparent',
                color: editorMode === 'raw' ? '#fff' : 'var(--text-secondary)',
                border: 'none',
                borderRadius: 0,
                padding: '6px 14px',
                fontWeight: 600,
              }"
              @click="editorMode = 'raw'"
            >
              YAML
            </button>
          </div>

          <button class="btn btn-sm" title="Copy YAML" @click="copyYaml">
            <MdiIcon :path="mdiContentCopy" size="14" />
          </button>
        </div>
      </div>
    </div>

    <!-- ===================================================================== -->
    <!-- VIEW A: Form-Based Compose Editor                                     -->
    <!-- ===================================================================== -->
    <div v-if="editorMode === 'form'">
      <!-- Top Level Entities Navigation (Services, Networks, Volumes) -->
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; border-bottom: 1px solid var(--border-subtle); padding-bottom: 8px">
        <div style="display: flex; gap: 8px; align-items: center">
          <button
            class="btn btn-sm"
            :class="currentTopTab === 'services' ? 'btn-primary' : ''"
            style="display: inline-flex; align-items: center; gap: 6px"
            @click="currentTopTab = 'services'"
          >
            <MdiIcon :path="mdiDocker" size="14" />
            <span>Services ({{ form.services.length }})</span>
          </button>
          <button
            class="btn btn-sm"
            :class="currentTopTab === 'networks' ? 'btn-primary' : ''"
            style="display: inline-flex; align-items: center; gap: 6px"
            @click="currentTopTab = 'networks'"
          >
            <MdiIcon :path="mdiLanConnect" size="14" />
            <span>Networks ({{ form.networks.length }})</span>
          </button>
          <button
            class="btn btn-sm"
            :class="currentTopTab === 'volumes' ? 'btn-primary' : ''"
            style="display: inline-flex; align-items: center; gap: 6px"
            @click="currentTopTab = 'volumes'"
          >
            <MdiIcon :path="mdiHarddisk" size="14" />
            <span>Volumes ({{ form.volumes.length }})</span>
          </button>
        </div>

        <button
          v-if="currentTopTab === 'services'"
          class="btn btn-sm btn-success"
          style="padding: 4px 10px; font-size: 12px"
          @click="addService"
        >
          + Add Service
        </button>
        <button
          v-else-if="currentTopTab === 'networks'"
          class="btn btn-sm btn-success"
          style="padding: 4px 10px; font-size: 12px"
          @click="addTopLevelNetwork"
        >
          + Add Network
        </button>
        <button
          v-else-if="currentTopTab === 'volumes'"
          class="btn btn-sm btn-success"
          style="padding: 4px 10px; font-size: 12px"
          @click="addTopLevelVolume"
        >
          + Add Named Volume
        </button>
      </div>

      <!-- TAB 1: Services Editor -->
      <div v-if="currentTopTab === 'services'">
        <!-- Service Pills / Tabs -->
        <div style="display: flex; gap: 6px; margin-bottom: 14px; overflow-x: auto; padding-bottom: 4px">
          <div
            v-for="s in form.services"
            :key="s.id"
            style="
              display: flex;
              align-items: center;
              border-radius: 6px;
              border: 1px solid var(--border-subtle);
              background: var(--bg-card);
              overflow: hidden;
            "
            :style="{
              borderColor: activeServiceId === s.id ? 'var(--accent-primary)' : 'var(--border-subtle)',
              background: activeServiceId === s.id ? 'rgba(59, 130, 246, 0.12)' : 'var(--bg-card)',
            }"
          >
            <button
              class="btn btn-sm"
              style="border: none; background: transparent; padding: 6px 12px; font-weight: 600"
              :style="{ color: activeServiceId === s.id ? 'var(--accent-primary)' : 'var(--text-primary)' }"
              @click="activeServiceId = s.id"
            >
              {{ s.name || 'unnamed' }}
            </button>
            <button
              v-if="form.services.length > 1"
              style="
                border: none;
                background: transparent;
                color: var(--text-secondary);
                padding: 6px 8px;
                cursor: pointer;
                font-size: 11px;
              "
              title="Remove service"
              @click="removeService(s.id)"
            >
              ✕
            </button>
          </div>
        </div>

        <!-- Active Service Configuration Panel -->
        <div v-if="activeService" class="card" style="padding: 16px; border: 1px solid var(--border-subtle)">
          <!-- Service Header & Quick Actions -->
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px">
            <div style="display: flex; align-items: center; gap: 10px">
              <span style="font-size: 16px; font-weight: 700; color: var(--text-primary)">
                Service: {{ activeService.name }}
              </span>
              <span class="mono" style="font-size: 12px; color: var(--text-secondary)">
                {{ activeService.image || 'no image specified' }}
              </span>
            </div>

            <div style="display: flex; gap: 6px">
              <button
                class="btn btn-sm"
                title="Duplicate service"
                @click="duplicateService(activeService)"
              >
                ⎘ Duplicate
              </button>
            </div>
          </div>

          <!-- Service Sub-Tabs (General, Ports, Env, Volumes, Resources, Networks) -->
          <div style="display: flex; gap: 4px; margin-bottom: 16px; border-bottom: 1px solid var(--border-subtle); padding-bottom: 6px">
            <button
              class="btn btn-sm"
              :class="serviceSubTab === 'general' ? 'btn-primary' : ''"
              style="padding: 4px 10px; font-size: 12px"
              @click="serviceSubTab = 'general'"
            >
              General
            </button>
            <button
              class="btn btn-sm"
              :class="serviceSubTab === 'ports' ? 'btn-primary' : ''"
              style="padding: 4px 10px; font-size: 12px"
              @click="serviceSubTab = 'ports'"
            >
              Ports ({{ activeService.ports.length }})
            </button>
            <button
              class="btn btn-sm"
              :class="serviceSubTab === 'env' ? 'btn-primary' : ''"
              style="padding: 4px 10px; font-size: 12px"
              @click="serviceSubTab = 'env'"
            >
              Environment ({{ activeService.environment.length }})
            </button>
            <button
              class="btn btn-sm"
              :class="serviceSubTab === 'volumes' ? 'btn-primary' : ''"
              style="padding: 4px 10px; font-size: 12px"
              @click="serviceSubTab = 'volumes'"
            >
              Storage & Volumes ({{ activeService.volumes.length }})
            </button>
            <button
              class="btn btn-sm"
              :class="serviceSubTab === 'resources' ? 'btn-primary' : ''"
              style="padding: 4px 10px; font-size: 12px"
              @click="serviceSubTab = 'resources'"
            >
              Resources & Hardware
            </button>
            <button
              class="btn btn-sm"
              :class="serviceSubTab === 'networks' ? 'btn-primary' : ''"
              style="padding: 4px 10px; font-size: 12px"
              @click="serviceSubTab = 'networks'"
            >
              Networks & Dependencies
            </button>
          </div>

          <!-- Sub-Tab: General -->
          <div v-if="serviceSubTab === 'general'">
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 14px; margin-bottom: 12px">
              <div>
                <label class="form-label">Service Key *</label>
                <input
                  v-model="activeService.name"
                  type="text"
                  class="form-input mono"
                  placeholder="e.g. web, db, redis"
                />
                <span style="font-size: 11px; color: var(--text-secondary)">
                  Key name under <code>services:</code> in the compose file.
                </span>
              </div>
              <div>
                <label class="form-label">Container Name Override</label>
                <input
                  v-model="activeService.containerName"
                  type="text"
                  class="form-input mono"
                  placeholder="Optional, e.g. nextcloud-web"
                />
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 14px; margin-bottom: 12px">
              <div>
                <label class="form-label">Container Image *</label>
                <input
                  v-model="activeService.image"
                  type="text"
                  class="form-input mono"
                  placeholder="e.g. postgres:16-alpine, nextcloud:apache"
                />
              </div>
              <div>
                <label class="form-label">Restart Policy</label>
                <select v-model="activeService.restart" class="form-input">
                  <option value="unless-stopped">unless-stopped</option>
                  <option value="always">always</option>
                  <option value="on-failure">on-failure</option>
                  <option value="no">no</option>
                </select>
              </div>
            </div>

            <div>
              <label class="form-label">Command / Entrypoint Override</label>
              <input
                v-model="activeService.command"
                type="text"
                class="form-input mono"
                placeholder="e.g. python -m app, redis-server --appendonly yes"
              />
            </div>
          </div>

          <!-- Sub-Tab: Ports -->
          <div v-else-if="serviceSubTab === 'ports'">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px">
              <span style="font-size: 12.5px; color: var(--text-secondary)">
                Expose container network ports to host interfaces.
              </span>
              <button class="btn btn-sm btn-primary" @click="addPort(activeService)">
                + Add Port
              </button>
            </div>

            <div v-if="activeService.ports.length === 0" style="padding: 24px; text-align: center; color: var(--text-secondary); font-size: 13px">
              No port mappings defined for this service.
            </div>

            <table v-else class="table" style="width: 100%; border-collapse: collapse">
              <thead>
                <tr style="text-align: left; font-size: 11.5px; color: var(--text-secondary); border-bottom: 1px solid var(--border-subtle)">
                  <th style="padding: 6px 8px">HOST PORT</th>
                  <th style="padding: 6px 8px">CONTAINER PORT</th>
                  <th style="padding: 6px 8px">PROTOCOL</th>
                  <th style="padding: 6px 8px; width: 40px"></th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(p, idx) in activeService.ports" :key="idx" style="border-bottom: 1px solid var(--border-subtle)">
                  <td style="padding: 6px 8px">
                    <input
                      v-model="p.hostPort"
                      type="text"
                      class="form-input mono"
                      placeholder="e.g. 8080"
                      style="padding: 4px 8px; font-size: 12.5px"
                    />
                  </td>
                  <td style="padding: 6px 8px">
                    <input
                      v-model="p.containerPort"
                      type="text"
                      class="form-input mono"
                      placeholder="e.g. 80"
                      style="padding: 4px 8px; font-size: 12.5px"
                    />
                  </td>
                  <td style="padding: 6px 8px">
                    <select v-model="p.protocol" class="form-input" style="padding: 4px 8px; font-size: 12.5px">
                      <option value="tcp">TCP</option>
                      <option value="udp">UDP</option>
                    </select>
                  </td>
                  <td style="padding: 6px 8px; text-align: center">
                    <button class="btn btn-sm btn-danger" style="padding: 2px 6px" @click="removePort(activeService, idx)">
                      ✕
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- Sub-Tab: Environment -->
          <div v-else-if="serviceSubTab === 'env'">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px">
              <span style="font-size: 12.5px; color: var(--text-secondary)">
                Configure environment variables passed to the container process.
              </span>
              <div style="display: flex; gap: 6px">
                <button class="btn btn-sm" @click="openBulkEnvModal">
                  <MdiIcon :path="mdiContentCopy" size="14" style="margin-right: 4px" />
                  Bulk .env Edit
                </button>
                <button class="btn btn-sm btn-primary" @click="addEnv(activeService)">
                  + Add Variable
                </button>
              </div>
            </div>

            <div v-if="activeService.environment.length === 0" style="padding: 24px; text-align: center; color: var(--text-secondary); font-size: 13px">
              No environment variables defined for this service.
            </div>

            <table v-else class="table" style="width: 100%; border-collapse: collapse">
              <thead>
                <tr style="text-align: left; font-size: 11.5px; color: var(--text-secondary); border-bottom: 1px solid var(--border-subtle)">
                  <th style="padding: 6px 8px; width: 40%">KEY</th>
                  <th style="padding: 6px 8px; width: 55%">VALUE</th>
                  <th style="padding: 6px 8px; width: 5%"></th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(e, idx) in activeService.environment" :key="idx" style="border-bottom: 1px solid var(--border-subtle)">
                  <td style="padding: 6px 8px">
                    <input
                      v-model="e.key"
                      type="text"
                      class="form-input mono"
                      placeholder="e.g. POSTGRES_PASSWORD"
                      style="padding: 4px 8px; font-size: 12px"
                    />
                  </td>
                  <td style="padding: 6px 8px">
                    <input
                      v-model="e.value"
                      type="text"
                      class="form-input mono"
                      placeholder="e.g. mysecretpass"
                      style="padding: 4px 8px; font-size: 12px"
                    />
                  </td>
                  <td style="padding: 6px 8px; text-align: center">
                    <button class="btn btn-sm btn-danger" style="padding: 2px 6px" @click="removeEnv(activeService, idx)">
                      ✕
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- Sub-Tab: Volumes -->
          <div v-else-if="serviceSubTab === 'volumes'">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px">
              <span style="font-size: 12.5px; color: var(--text-secondary)">
                Bind mount host paths (e.g. ZFS datasets) or persistent named volumes.
              </span>
              <button class="btn btn-sm btn-primary" @click="addVolumeMount(activeService)">
                + Add Storage Mount
              </button>
            </div>

            <div v-if="activeService.volumes.length === 0" style="padding: 24px; text-align: center; color: var(--text-secondary); font-size: 13px">
              No storage volumes mounted for this service.
            </div>

            <table v-else class="table" style="width: 100%; border-collapse: collapse">
              <thead>
                <tr style="text-align: left; font-size: 11.5px; color: var(--text-secondary); border-bottom: 1px solid var(--border-subtle)">
                  <th style="padding: 6px 8px; width: 45%">HOST PATH / NAMED VOLUME</th>
                  <th style="padding: 6px 8px; width: 40%">CONTAINER DESTINATION</th>
                  <th style="padding: 6px 8px; width: 10%">MODE</th>
                  <th style="padding: 6px 8px; width: 5%"></th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(v, idx) in activeService.volumes" :key="idx" style="border-bottom: 1px solid var(--border-subtle)">
                  <td style="padding: 6px 8px">
                    <div style="display: flex; flex-direction: column; gap: 4px">
                      <PathSelector
                        v-model="v.source"
                        :placeholder="defaultAppdataMount(form.appId, 'data')"
                        :root-hint="appsPoolAppdataDir"
                      />
                      <div style="display: flex; gap: 6px; align-items: center">
                        <button
                          type="button"
                          class="btn-link"
                          style="font-size: 10.5px; color: var(--accent-primary); background: none; border: none; padding: 0; cursor: pointer; text-decoration: underline"
                          title="Quick-set to Docker owned ZFS appdata pool volume"
                          @click="
                            v.source = defaultAppdataMount(
                              form.appId,
                              v.target
                                ? v.target.split('/').filter(Boolean).pop() || 'data'
                                : 'data'
                            )
                          "
                        >
                          Set to App Pool Volume ({{ appsPoolName }})
                        </button>
                        <span v-if="form.volumes.length > 0" style="color: var(--text-secondary); font-size: 10px">|</span>
                        <select
                          v-if="form.volumes.length > 0"
                          style="font-size: 10.5px; padding: 1px 4px; background: none; border: 1px solid var(--border-subtle); color: var(--text-secondary); border-radius: 4px"
                          @change="
                            (e: any) => {
                              if (e.target.value) {
                                v.source = e.target.value;
                                e.target.value = '';
                              }
                            }
                          "
                        >
                          <option value="">Attach Named Volume...</option>
                          <option v-for="vol in form.volumes" :key="vol.id" :value="vol.name">
                            {{ vol.name }}
                          </option>
                        </select>
                      </div>
                    </div>
                  </td>
                  <td style="padding: 6px 8px">
                    <input
                      v-model="v.target"
                      type="text"
                      class="form-input mono"
                      placeholder="e.g. /var/www/html"
                      style="padding: 4px 8px; font-size: 12px"
                    />
                  </td>
                  <td style="padding: 6px 8px">
                    <select v-model="v.mode" class="form-input" style="padding: 4px 8px; font-size: 12px">
                      <option value="rw">Read/Write (rw)</option>
                      <option value="ro">Read-Only (ro)</option>
                    </select>
                  </td>
                  <td style="padding: 6px 8px; text-align: center">
                    <button class="btn btn-sm btn-danger" style="padding: 2px 6px" @click="removeVolumeMount(activeService, idx)">
                      ✕
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>

            <!-- Autocomplete datalist for host ZFS datasets -->
            <datalist id="zfs-mount-options">
              <option v-for="d in zfsDatasetPaths" :key="d" :value="d" />
              <option v-for="vol in form.volumes" :key="vol.id" :value="vol.name" />
            </datalist>
          </div>

          <!-- Sub-Tab: Resources & Hardware -->
          <div v-else-if="serviceSubTab === 'resources'">
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 14px; margin-bottom: 16px">
              <div>
                <label class="form-label">Memory Limit</label>
                <input
                  v-model="activeService.memoryLimit"
                  type="text"
                  class="form-input mono"
                  placeholder="e.g. 512m, 1g, 2g"
                />
                <span style="font-size: 11px; color: var(--text-secondary)">
                  Maps to <code>mem_limit</code> in Compose.
                </span>
              </div>
              <div>
                <label class="form-label">CPU Limit</label>
                <input
                  v-model="activeService.cpuLimit"
                  type="text"
                  class="form-input mono"
                  placeholder="e.g. 1.0, 2.5"
                />
                <span style="font-size: 11px; color: var(--text-secondary)">
                  Fractional CPU cores (maps to <code>cpus</code>).
                </span>
              </div>
            </div>

            <div style="margin-bottom: 16px; padding: 12px; background: rgba(0, 0, 0, 0.2); border-radius: 6px">
              <label style="display: flex; align-items: center; gap: 8px; cursor: pointer; font-size: 13.5px; font-weight: 600">
                <input v-model="activeService.privileged" type="checkbox" />
                Privileged Mode (<code>privileged: true</code>)
              </label>
              <p style="font-size: 11.5px; color: var(--text-secondary); margin: 4px 0 0 24px">
                Grants extended container privileges (useful for Docker-in-Docker, direct hardware access, or network tunnel adapters).
              </p>
            </div>

            <!-- Hardware Devices -->
            <div>
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
                <label class="form-label" style="margin-bottom: 0">Hardware Device Passthrough (<code>devices:</code>)</label>
                <button class="btn btn-sm" @click="addDevice(activeService)">
                  + Add Device
                </button>
              </div>
              <div v-if="activeService.devices.length === 0" style="font-size: 12px; color: var(--text-secondary); padding: 8px 0">
                No direct host device passthrough defined.
              </div>
              <div v-for="(dev, idx) in activeService.devices" :key="idx" style="display: flex; gap: 8px; margin-bottom: 6px">
                <input
                  v-model="activeService.devices[idx]"
                  type="text"
                  class="form-input mono"
                  placeholder="e.g. /dev/dri:/dev/dri (Intel/AMD GPU)"
                  style="font-size: 12px"
                />
                <button class="btn btn-sm btn-danger" @click="removeDevice(activeService, idx)">✕</button>
              </div>
            </div>
          </div>

          <!-- Sub-Tab: Networks & Dependencies -->
          <div v-else-if="serviceSubTab === 'networks'">
            <div style="margin-bottom: 16px">
              <label class="form-label">Network Mode</label>
              <select v-model="activeService.networkMode" class="form-input">
                <option value="">Default (bridge)</option>
                <option value="host">host (share host network stack)</option>
                <option value="none">none (isolated)</option>
              </select>
            </div>

            <!-- Depends On -->
            <div style="margin-bottom: 16px">
              <label class="form-label">Depends On Services (<code>depends_on:</code>)</label>
              <div v-if="otherServices.length === 0" style="font-size: 12px; color: var(--text-secondary)">
                No other services defined in this App stack.
              </div>
              <div v-else style="display: flex; gap: 12px; flex-wrap: wrap">
                <label
                  v-for="svcName in otherServices"
                  :key="svcName"
                  style="display: flex; align-items: center; gap: 6px; font-size: 13px; cursor: pointer"
                >
                  <input
                    type="checkbox"
                    :value="svcName"
                    :checked="activeService.dependsOn.includes(svcName)"
                    @change="(e: any) => {
                      if (e.target.checked) {
                        activeService.dependsOn.push(svcName);
                      } else {
                        activeService.dependsOn = activeService.dependsOn.filter((d) => d !== svcName);
                      }
                    }"
                  />
                  <code>{{ svcName }}</code>
                </label>
              </div>
            </div>

            <!-- Connect to top-level networks -->
            <div v-if="availableNetworkNames.length > 0">
              <label class="form-label">Custom Networks (<code>networks:</code>)</label>
              <div style="display: flex; gap: 12px; flex-wrap: wrap">
                <label
                  v-for="netName in availableNetworkNames"
                  :key="netName"
                  style="display: flex; align-items: center; gap: 6px; font-size: 13px; cursor: pointer"
                >
                  <input
                    type="checkbox"
                    :value="netName"
                    :checked="activeService.networks.includes(netName)"
                    @change="(e: any) => {
                      if (e.target.checked) {
                        activeService.networks.push(netName);
                      } else {
                        activeService.networks = activeService.networks.filter((n) => n !== netName);
                      }
                    }"
                  />
                  <code>{{ netName }}</code>
                </label>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- TAB 2: Top-Level Networks Editor -->
      <div v-else-if="currentTopTab === 'networks'" class="card" style="padding: 16px">
        <div style="margin-bottom: 12px; font-size: 12.5px; color: var(--text-secondary)">
          Declare application-scoped custom Docker networks (top-level <code>networks:</code> in Compose).
        </div>

        <div v-if="form.networks.length === 0" style="padding: 24px; text-align: center; color: var(--text-secondary); font-size: 13px">
          No custom networks defined. Services will use default bridge networking.
        </div>

        <table v-else class="table" style="width: 100%; border-collapse: collapse">
          <thead>
            <tr style="text-align: left; font-size: 11.5px; color: var(--text-secondary); border-bottom: 1px solid var(--border-subtle)">
              <th style="padding: 6px 8px">NETWORK NAME</th>
              <th style="padding: 6px 8px">DRIVER</th>
              <th style="padding: 6px 8px">INTERNAL ONLY</th>
              <th style="padding: 6px 8px; width: 40px"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(net, idx) in form.networks" :key="net.id" style="border-bottom: 1px solid var(--border-subtle)">
              <td style="padding: 6px 8px">
                <input v-model="net.name" type="text" class="form-input mono" placeholder="e.g. backend" style="padding: 4px 8px; font-size: 12px" />
              </td>
              <td style="padding: 6px 8px">
                <select v-model="net.driver" class="form-input" style="padding: 4px 8px; font-size: 12px">
                  <option value="bridge">bridge</option>
                  <option value="overlay">overlay</option>
                  <option value="macvlan">macvlan</option>
                </select>
              </td>
              <td style="padding: 6px 8px">
                <label style="display: flex; align-items: center; gap: 6px; font-size: 12px; cursor: pointer">
                  <input v-model="net.internal" type="checkbox" />
                  Isolate from external routing
                </label>
              </td>
              <td style="padding: 6px 8px; text-align: center">
                <button class="btn btn-sm btn-danger" style="padding: 2px 6px" @click="removeTopLevelNetwork(idx)">✕</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- TAB 3: Top-Level Volumes Editor -->
      <div v-else-if="currentTopTab === 'volumes'" class="card" style="padding: 16px">
        <div style="margin-bottom: 12px; font-size: 12.5px; color: var(--text-secondary)">
          Declare persistent named volumes managed by Docker (top-level <code>volumes:</code> in Compose).
        </div>

        <div v-if="form.volumes.length === 0" style="padding: 24px; text-align: center; color: var(--text-secondary); font-size: 13px">
          No top-level named volumes defined.
        </div>

        <table v-else class="table" style="width: 100%; border-collapse: collapse">
          <thead>
            <tr style="text-align: left; font-size: 11.5px; color: var(--text-secondary); border-bottom: 1px solid var(--border-subtle)">
              <th style="padding: 6px 8px">VOLUME NAME</th>
              <th style="padding: 6px 8px">DRIVER</th>
              <th style="padding: 6px 8px">EXTERNAL</th>
              <th style="padding: 6px 8px; width: 40px"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(vol, idx) in form.volumes" :key="vol.id" style="border-bottom: 1px solid var(--border-subtle)">
              <td style="padding: 6px 8px">
                <input v-model="vol.name" type="text" class="form-input mono" placeholder="e.g. db_data" style="padding: 4px 8px; font-size: 12px" />
              </td>
              <td style="padding: 6px 8px">
                <input v-model="vol.driver" type="text" class="form-input mono" placeholder="local" style="padding: 4px 8px; font-size: 12px" />
              </td>
              <td style="padding: 6px 8px">
                <label style="display: flex; align-items: center; gap: 6px; font-size: 12px; cursor: pointer">
                  <input v-model="vol.external" type="checkbox" />
                  Pre-existing volume
                </label>
              </td>
              <td style="padding: 6px 8px; text-align: center">
                <button class="btn btn-sm btn-danger" style="padding: 2px 6px" @click="removeTopLevelVolume(idx)">✕</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- ===================================================================== -->
    <!-- VIEW B: Raw Docker Compose YAML (On Demand)                           -->
    <!-- ===================================================================== -->
    <div v-else class="card" style="padding: 16px; flex: 1; display: flex; flex-direction: column; min-height: 520px; width: 100%">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; flex-wrap: wrap; gap: 8px">
        <div>
          <label class="form-label" style="margin-bottom: 0; font-size: 13.5px">
            Raw Docker Compose YAML Specification
          </label>
          <div style="font-size: 12px; color: var(--text-secondary); margin-top: 2px">
            Powered by CodeMirror 6 with syntax highlighting, inline validation, and context-aware completion.
          </div>
        </div>
        <div style="display: flex; align-items: center; gap: 8px">
          <span class="badge badge-info" style="font-size: 11px">Auto-Complete: Ctrl+Space</span>
          <button class="btn btn-sm" @click="copyYaml">
            <MdiIcon :path="mdiContentCopy" size="14" style="margin-right: 4px" />
            Copy YAML
          </button>
        </div>
      </div>
      <div style="flex: 1; min-height: 480px; width: 100%; display: flex; flex-direction: column">
        <CodeMirrorEditor
          v-model="rawYaml"
          height="100%"
          min-height="480px"
          placeholder="services:\n  app:\n    image: nginx:alpine\n    ports:\n      - '8080:80'\n"
        />
      </div>
    </div>

    <!-- Action Buttons -->
    <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 18px; border-top: 1px solid var(--border-subtle); padding-top: 14px">
      <button class="btn" @click="emit('cancel')">Cancel</button>
      <button class="btn btn-primary" @click="handleSave">
        {{ props.isEditMode ? '💾 Save & Redeploy Stack' : '🚀 Deploy Application' }}
      </button>
    </div>

    <!-- Bulk .env Paste Modal -->
    <div v-if="showBulkEnv" class="modal-backdrop" @click.self="showBulkEnv = false">
      <div class="modal-card" style="max-width: 600px">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px">
          <h4 style="font-size: 16px; font-weight: 700; color: var(--text-primary)">
            Bulk Import Environment Variables (.env)
          </h4>
          <button class="btn btn-sm" @click="showBulkEnv = false">✕</button>
        </div>
        <p style="font-size: 12px; color: var(--text-secondary); margin-bottom: 10px">
          Paste lines in <code>KEY=VALUE</code> format. Lines starting with <code>#</code> are treated as comments.
        </p>
        <textarea
          v-model="bulkEnvText"
          rows="10"
          class="form-input mono"
          placeholder="POSTGRES_DB=nextcloud&#10;POSTGRES_USER=nextcloud&#10;POSTGRES_PASSWORD=secret"
          style="font-size: 12px; line-height: 1.4"
        ></textarea>
        <div style="display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px">
          <button class="btn btn-sm" @click="showBulkEnv = false">Cancel</button>
          <button class="btn btn-sm btn-primary" @click="applyBulkEnv">Apply Variables</button>
        </div>
      </div>
    </div>
  </div>
</template>
