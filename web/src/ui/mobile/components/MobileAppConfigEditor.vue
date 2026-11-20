<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import {
  mdiAlertCircle,
  mdiChevronDown,
  mdiChevronUp,
  mdiClose,
  mdiCodeBraces,
  mdiContentCopy,
  mdiContentSave,
  mdiCpu64Bit,
  mdiDeleteOutline,
  mdiFormSelect,
  mdiHarddisk,
  mdiLanConnect,
  mdiMagnify,
  mdiPlus,
  mdiServerNetwork,
  mdiTune,
  mdiUpdate,
  mdiVariable,
} from '@mdi/js';
import MdiIcon from '../../components/MdiIcon.vue';
import CodeMirrorEditor from '../../components/CodeMirrorEditor.vue';
import {
  type ComposeAppFormModel,
  composeYamlToForm,
  createDefaultAppForm,
  createDefaultService,
  formToComposeYaml,
  generateRandomHighPort,
} from '../../../domain/composeModel';
import type { DockerApp } from '../../../domain/models';
import { services } from '../../../container';
import { confirmModal } from '../../modal';
import { nasState, notify, refreshAllData, runTrackedTask } from '../../store';

const props = defineProps<{
  app: DockerApp;
}>();

const editorMode = ref<'form' | 'yaml'>('form');
const yamlParseError = ref('');
const isSaving = ref(false);
const isUpgrading = ref(false);
const envSearch = ref('');
const showBulkEnv = ref(false);
const bulkEnvText = ref('');

const initialPoolName = computed(() =>
  (nasState.dockerConfig?.pool || nasState.systemConfig?.mainPool || 'tank')
    .replace(/^zfsPools\//, '')
    .trim()
);

function parseInitialForm(app: DockerApp): { model: ComposeAppFormModel; error: string } {
  const cleanId = app.appId || app.name.replace(/^dockerApps\//, '');
  try {
    return {
      model: composeYamlToForm(app.composeYaml || '', cleanId, app.title || cleanId, initialPoolName.value),
      error: '',
    };
  } catch (err: any) {
    return {
      model: createDefaultAppForm(cleanId, app.title || cleanId),
      error: err?.message || String(err),
    };
  }
}

const initial = parseInitialForm(props.app);
if (initial.error) {
  yamlParseError.value = initial.error;
  editorMode.value = 'yaml';
}

const form = reactive<ComposeAppFormModel>(initial.model);
const rawYaml = ref(props.app.composeYaml || formToComposeYaml(form));
const activeServiceId = ref(form.services[0]?.id || '');

// Accordion open state (General open by default; others expand on tap)
const openSections = reactive<Record<string, boolean>>({
  general: true,
  ports: false,
  env: false,
  volumes: false,
  resources: false,
  networks: false,
});

function toggleSection(key: string) {
  openSections[key] = !openSections[key];
}

// Re-initialize only when switching to a different app
watch(
  () => props.app.name,
  () => {
    const next = parseInitialForm(props.app);
    Object.assign(form, next.model);
    rawYaml.value = props.app.composeYaml || formToComposeYaml(form);
    yamlParseError.value = next.error;
    editorMode.value = next.error ? 'yaml' : 'form';
    activeServiceId.value = form.services[0]?.id || '';
  }
);

const activeService = computed(() => {
  return form.services.find((s) => s.id === activeServiceId.value) || form.services[0];
});

const filteredEnvIndices = computed(() => {
  if (!activeService.value) return [];
  const q = envSearch.value.trim().toLowerCase();
  return activeService.value.environment
    .map((e, idx) => ({ e, idx }))
    .filter(({ e }) => !q || e.key.toLowerCase().includes(q) || e.value.toLowerCase().includes(q));
});

const otherServiceNames = computed(() => {
  if (!activeService.value) return [];
  return form.services
    .filter((s) => s.id !== activeService.value!.id && s.name.trim())
    .map((s) => s.name.trim());
});

const MEMORY_PRESETS = ['256m', '512m', '1g', '2g', '4g', '8g'];
const CPU_PRESETS = ['0.5', '1.0', '2.0', '4.0'];

function switchMode(mode: 'form' | 'yaml') {
  if (mode === editorMode.value) return;
  if (mode === 'yaml') {
    rawYaml.value = formToComposeYaml(form);
    yamlParseError.value = '';
    editorMode.value = 'yaml';
  } else {
    try {
      const cleanId = props.app.appId || props.app.name.replace(/^dockerApps\//, '');
      const parsed = composeYamlToForm(rawYaml.value, cleanId, form.title || props.app.title || cleanId, initialPoolName.value);
      Object.assign(form, parsed);
      if (!form.services.find((s) => s.id === activeServiceId.value)) {
        activeServiceId.value = form.services[0]?.id || '';
      }
      yamlParseError.value = '';
      editorMode.value = 'form';
    } catch (err: any) {
      yamlParseError.value = `YAML parse error: ${err?.message || err}`;
      notify('error', 'Fix YAML syntax errors before switching to Form view');
    }
  }
}

function addService() {
  const idx = form.services.length + 1;
  const svc = createDefaultService(`service${idx}`);
  form.services.push(svc);
  activeServiceId.value = svc.id;
  openSections.general = true;
}

async function removeActiveService() {
  if (!activeService.value || form.services.length <= 1) return;
  const target = activeService.value;
  const ok = await confirmModal({
    title: `Remove service "${target.name}"?`,
    message: 'This service will be removed from the compose configuration when saved.',
    confirmText: 'Remove',
    tone: 'danger',
  });
  if (!ok) return;
  const idx = form.services.findIndex((s) => s.id === target.id);
  if (idx >= 0) {
    form.services.splice(idx, 1);
    activeServiceId.value = form.services[0]?.id || '';
  }
}

function addPort() {
  if (!activeService.value) return;
  activeService.value.ports.push({
    hostPort: String(generateRandomHighPort()),
    containerPort: '80',
    protocol: 'tcp',
    publishToHost: true,
  });
  openSections.ports = true;
}

function removePort(idx: number) {
  activeService.value?.ports.splice(idx, 1);
}

function addEnv() {
  if (!activeService.value) return;
  activeService.value.environment.push({ key: '', value: '' });
  openSections.env = true;
}

function removeEnv(idx: number) {
  activeService.value?.environment.splice(idx, 1);
}

function applyBulkEnv() {
  if (!activeService.value || !bulkEnvText.value.trim()) {
    showBulkEnv.value = false;
    return;
  }
  const lines = bulkEnvText.value.split(/\r?\n/);
  for (const raw of lines) {
    const line = raw.trim();
    if (!line || line.startsWith('#')) continue;
    const eq = line.indexOf('=');
    if (eq <= 0) continue;
    const key = line.slice(0, eq).trim();
    const val = line.slice(eq + 1).trim();
    const existing = activeService.value.environment.find((e) => e.key === key);
    if (existing) existing.value = val;
    else activeService.value.environment.push({ key, value: val });
  }
  bulkEnvText.value = '';
  showBulkEnv.value = false;
}

function addVolume() {
  if (!activeService.value) return;
  const cleanId = props.app.appId || props.app.name.replace(/^dockerApps\//, '');
  activeService.value.volumes.push({
    type: 'bind',
    source: `/mnt/${initialPoolName.value}/apps/${cleanId}/data`,
    target: '/data',
    mode: 'rw',
  });
  openSections.volumes = true;
}

function removeVolume(idx: number) {
  activeService.value?.volumes.splice(idx, 1);
}

function toggleServiceNetwork(netName: string) {
  if (!activeService.value) return;
  const idx = activeService.value.networks.indexOf(netName);
  if (idx >= 0) activeService.value.networks.splice(idx, 1);
  else activeService.value.networks.push(netName);
}

function toggleServiceDep(depName: string) {
  if (!activeService.value) return;
  const idx = activeService.value.dependsOn.indexOf(depName);
  if (idx >= 0) activeService.value.dependsOn.splice(idx, 1);
  else activeService.value.dependsOn.push(depName);
}

async function copyYaml() {
  try {
    await navigator.clipboard.writeText(rawYaml.value);
    notify('success', 'Compose YAML copied');
  } catch {
    notify('error', 'Failed to copy YAML');
  }
}

async function handleUpgrade() {
  if (isUpgrading.value) return;
  const ok = await confirmModal({
    title: `Upgrade "${props.app.title || props.app.appId}"?`,
    message: 'Pull latest container images and recreate containers.',
    confirmText: 'Upgrade',
    tone: 'primary',
  });
  if (!ok) return;
  isUpgrading.value = true;
  try {
    await runTrackedTask(`App "${props.app.title || props.app.appId}": Upgrade`, async () => {
      await services.apps.upgradeApp(props.app.name, true);
    });
    notify('success', 'App upgraded successfully');
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Upgrade failed: ${err?.message || err}`);
  } finally {
    isUpgrading.value = false;
  }
}

async function handleSave() {
  if (isSaving.value) return;
  let finalYaml = rawYaml.value;
  if (editorMode.value === 'form') {
    for (const s of form.services) {
      if (!s.name.trim()) {
        notify('error', 'Every service must have a name');
        return;
      }
      if (!s.image.trim()) {
        notify('error', `Service "${s.name}" requires a container image`);
        return;
      }
    }
    const targetRestart = form.autoStart ? 'unless-stopped' : 'no';
    for (const s of form.services) {
      if (!s.restart || s.restart === 'no' || s.restart === 'unless-stopped') {
        s.restart = targetRestart;
      }
    }
    finalYaml = formToComposeYaml(form);
    rawYaml.value = finalYaml;
  }

  isSaving.value = true;
  try {
    const title = form.title.trim() || props.app.title || props.app.appId;
    await runTrackedTask(`App "${title}": Update Configuration`, async () => {
      await services.apps.updateApp(props.app.name, finalYaml, title);
    });
    notify('success', 'Configuration saved and stack redeployed');
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to save configuration: ${err?.message || err}`);
  } finally {
    isSaving.value = false;
  }
}
</script>

<template>
  <div class="m-cfg-root">
    <!-- Mode Switcher (Form | YAML) -->
    <div class="m-segmented">
      <button
        type="button"
        class="m-seg-btn"
        :class="{ active: editorMode === 'form' }"
        @click="switchMode('form')"
      >
        <MdiIcon :path="mdiFormSelect" size="16" />
        <span>Guided Form</span>
      </button>
      <button
        type="button"
        class="m-seg-btn"
        :class="{ active: editorMode === 'yaml' }"
        @click="switchMode('yaml')"
      >
        <MdiIcon :path="mdiCodeBraces" size="16" />
        <span>Raw YAML</span>
      </button>
    </div>

    <!-- Parse Error Banner -->
    <div v-if="yamlParseError" class="m-alert-banner">
      <MdiIcon :path="mdiAlertCircle" size="18" style="flex-shrink: 0" />
      <span>{{ yamlParseError }}</span>
    </div>

    <!-- Catalog Update Banner -->
    <div v-if="app.updateAvailable" class="m-update-card">
      <div class="m-update-info">
        <MdiIcon :path="mdiUpdate" size="18" style="color: #fbbf24; flex-shrink: 0" />
        <div class="m-update-text">
          <div class="m-update-title">Update Available{{ app.latestVersion ? `: ${app.latestVersion}` : '' }}</div>
          <div v-if="app.latestImage" class="m-update-sub mono">{{ app.latestImage }}</div>
        </div>
      </div>
      <button
        type="button"
        class="btn btn-xs btn-primary"
        :disabled="isUpgrading"
        @click="handleUpgrade"
      >
        {{ isUpgrading ? 'Upgrading...' : 'Upgrade' }}
      </button>
    </div>

    <!-- FORM MODE -->
    <template v-if="editorMode === 'form'">
      <!-- App Metadata Card -->
      <div class="m-card">
        <div class="m-field">
          <label class="m-label">Display Name</label>
          <input
            v-model="form.title"
            type="text"
            class="m-input"
            placeholder="Application title"
          />
        </div>

        <label class="m-toggle-row">
          <div class="m-toggle-text">
            <span class="m-toggle-title">Start Automatically</span>
            <span class="m-toggle-sub">Restart containers automatically on boot</span>
          </div>
          <input v-model="form.autoStart" type="checkbox" class="m-switch" />
        </label>
      </div>

      <!-- Service Selector Strip -->
      <div class="m-service-bar">
        <div class="m-service-pills">
          <button
            v-for="svc in form.services"
            :key="svc.id"
            type="button"
            class="m-service-pill"
            :class="{ active: activeService?.id === svc.id }"
            @click="activeServiceId = svc.id"
          >
            {{ svc.name || 'unnamed' }}
          </button>
          <button type="button" class="m-service-pill m-service-add" @click="addService">
            <MdiIcon :path="mdiPlus" size="14" />
            <span>Service</span>
          </button>
        </div>
      </div>

      <!-- Active Service Accordions -->
      <template v-if="activeService">
        <!-- 1. General -->
        <div class="m-accordion">
          <button type="button" class="m-acc-header" @click="toggleSection('general')">
            <div class="m-acc-left">
              <MdiIcon :path="mdiTune" size="18" style="color: var(--accent-primary)" />
              <span class="m-acc-title">General &amp; Image</span>
            </div>
            <MdiIcon :path="openSections.general ? mdiChevronUp : mdiChevronDown" size="20" />
          </button>

          <div v-if="openSections.general" class="m-acc-body">
            <div class="m-field">
              <label class="m-label">Container Image *</label>
              <input
                v-model="activeService.image"
                type="text"
                class="m-input mono"
                placeholder="e.g. jellyfin/jellyfin:latest"
              />
            </div>

            <div class="m-field">
              <label class="m-label">Service Key *</label>
              <input
                v-model="activeService.name"
                type="text"
                class="m-input mono"
                placeholder="e.g. web, db"
              />
            </div>

            <div class="m-field">
              <label class="m-label">Container Name</label>
              <input
                v-model="activeService.containerName"
                type="text"
                class="m-input mono"
                placeholder="Optional override"
              />
            </div>

            <div class="m-field">
              <label class="m-label">Hostname</label>
              <input
                v-model="activeService.hostname"
                type="text"
                class="m-input mono"
                placeholder="Optional hostname"
              />
            </div>

            <div class="m-row-2">
              <div class="m-field">
                <label class="m-label">Restart Policy</label>
                <select v-model="activeService.restart" class="m-input">
                  <option value="unless-stopped">unless-stopped</option>
                  <option value="always">always</option>
                  <option value="on-failure">on-failure</option>
                  <option value="no">no</option>
                </select>
              </div>

              <div class="m-field">
                <label class="m-label">Network Mode</label>
                <select v-model="activeService.networkMode" class="m-input">
                  <option value="bridge">bridge</option>
                  <option value="host">host</option>
                  <option value="none">none</option>
                </select>
              </div>
            </div>

            <div class="m-field">
              <label class="m-label">Command Override</label>
              <input
                v-model="activeService.command"
                type="text"
                class="m-input mono"
                placeholder="Optional startup command"
              />
            </div>

            <button
              v-if="form.services.length > 1"
              type="button"
              class="btn btn-sm btn-danger m-full-btn"
              @click="removeActiveService"
            >
              <MdiIcon :path="mdiDeleteOutline" size="16" />
              <span>Remove Service "{{ activeService.name }}"</span>
            </button>
          </div>
        </div>

        <!-- 2. Ports -->
        <div class="m-accordion">
          <button type="button" class="m-acc-header" @click="toggleSection('ports')">
            <div class="m-acc-left">
              <MdiIcon :path="mdiServerNetwork" size="18" style="color: #4ade80" />
              <span class="m-acc-title">Ports</span>
              <span class="m-count-badge">{{ activeService.ports.length }}</span>
            </div>
            <MdiIcon :path="openSections.ports ? mdiChevronUp : mdiChevronDown" size="20" />
          </button>

          <div v-if="openSections.ports" class="m-acc-body">
            <div v-if="activeService.ports.length === 0" class="m-empty-hint">
              No ports mapped for this service.
            </div>

            <div v-for="(p, idx) in activeService.ports" :key="idx" class="m-item-card">
              <div class="m-item-card-head">
                <span class="m-item-idx">Port #{{ idx + 1 }}</span>
                <div class="m-item-head-actions">
                  <button
                    type="button"
                    class="m-proto-pill"
                    @click="p.protocol = p.protocol === 'tcp' ? 'udp' : 'tcp'"
                  >
                    {{ (p.protocol || 'tcp').toUpperCase() }}
                  </button>
                  <button
                    type="button"
                    class="m-icon-del"
                    aria-label="Remove port"
                    @click="removePort(idx)"
                  >
                    <MdiIcon :path="mdiClose" size="16" />
                  </button>
                </div>
              </div>

              <div class="m-row-2">
                <div class="m-field">
                  <label class="m-label">Host Port</label>
                  <input v-model="p.hostPort" type="text" inputmode="numeric" class="m-input mono" placeholder="8080" />
                </div>
                <div class="m-field">
                  <label class="m-label">Container Port</label>
                  <input v-model="p.containerPort" type="text" inputmode="numeric" class="m-input mono" placeholder="80" />
                </div>
              </div>
            </div>

            <button type="button" class="m-dashed-add" @click="addPort">
              <MdiIcon :path="mdiPlus" size="16" />
              <span>Add Port Mapping</span>
            </button>
          </div>
        </div>

        <!-- 3. Environment Variables -->
        <div class="m-accordion">
          <button type="button" class="m-acc-header" @click="toggleSection('env')">
            <div class="m-acc-left">
              <MdiIcon :path="mdiVariable" size="18" style="color: #fbbf24" />
              <span class="m-acc-title">Environment Variables</span>
              <span class="m-count-badge">{{ activeService.environment.length }}</span>
            </div>
            <MdiIcon :path="openSections.env ? mdiChevronUp : mdiChevronDown" size="20" />
          </button>

          <div v-if="openSections.env" class="m-acc-body">
            <div v-if="activeService.environment.length > 4" class="m-search-box">
              <MdiIcon :path="mdiMagnify" size="16" style="color: var(--text-secondary)" />
              <input
                v-model="envSearch"
                type="search"
                class="m-search-input"
                placeholder="Filter variables..."
              />
            </div>

            <div v-if="activeService.environment.length === 0" class="m-empty-hint">
              No environment variables configured.
            </div>

            <div v-for="item in filteredEnvIndices" :key="item.idx" class="m-item-card">
              <div class="m-env-key-row">
                <input
                  v-model="item.e.key"
                  type="text"
                  class="m-input mono m-env-key"
                  placeholder="KEY"
                />
                <button
                  type="button"
                  class="m-icon-del"
                  aria-label="Remove variable"
                  @click="removeEnv(item.idx)"
                >
                  <MdiIcon :path="mdiClose" size="16" />
                </button>
              </div>
              <input
                v-model="item.e.value"
                type="text"
                class="m-input mono"
                placeholder="value"
              />
            </div>

            <div v-if="showBulkEnv" class="m-item-card">
              <label class="m-label">Paste KEY=VALUE lines</label>
              <textarea
                v-model="bulkEnvText"
                rows="5"
                class="m-input mono"
                placeholder="TZ=UTC&#10;PUID=1000"
              />
              <div class="m-row-2">
                <button type="button" class="btn btn-sm btn-secondary" @click="showBulkEnv = false">
                  Cancel
                </button>
                <button type="button" class="btn btn-sm btn-primary" @click="applyBulkEnv">
                  Apply Lines
                </button>
              </div>
            </div>

            <div class="m-row-2">
              <button type="button" class="m-dashed-add" @click="addEnv">
                <MdiIcon :path="mdiPlus" size="16" />
                <span>Add Variable</span>
              </button>
              <button type="button" class="m-dashed-add" @click="showBulkEnv = !showBulkEnv">
                <span>Bulk Paste</span>
              </button>
            </div>
          </div>
        </div>

        <!-- 4. Storage & Volumes -->
        <div class="m-accordion">
          <button type="button" class="m-acc-header" @click="toggleSection('volumes')">
            <div class="m-acc-left">
              <MdiIcon :path="mdiHarddisk" size="18" style="color: #c084fc" />
              <span class="m-acc-title">Storage &amp; Volumes</span>
              <span class="m-count-badge">{{ activeService.volumes.length }}</span>
            </div>
            <MdiIcon :path="openSections.volumes ? mdiChevronUp : mdiChevronDown" size="20" />
          </button>

          <div v-if="openSections.volumes" class="m-acc-body">
            <div v-if="activeService.volumes.length === 0" class="m-empty-hint">
              No persistent mounts configured.
            </div>

            <div v-for="(vol, idx) in activeService.volumes" :key="idx" class="m-item-card">
              <div class="m-item-card-head">
                <div class="m-item-head-actions">
                  <select v-model="vol.type" class="m-mini-select">
                    <option value="bind">Host Path</option>
                    <option value="volume">Named Volume</option>
                    <option value="config">Config</option>
                  </select>
                  <button
                    type="button"
                    class="m-proto-pill"
                    @click="vol.mode = vol.mode === 'ro' ? 'rw' : 'ro'"
                  >
                    {{ (vol.mode || 'rw').toUpperCase() }}
                  </button>
                </div>
                <button
                  type="button"
                  class="m-icon-del"
                  aria-label="Remove mount"
                  @click="removeVolume(idx)"
                >
                  <MdiIcon :path="mdiClose" size="16" />
                </button>
              </div>

              <div class="m-field">
                <label class="m-label">{{ vol.type === 'volume' ? 'Volume Name' : vol.type === 'config' ? 'Config Name' : 'Host Path' }}</label>
                <input v-model="vol.source" type="text" class="m-input mono" placeholder="/mnt/tank/apps/..." />
              </div>

              <div class="m-field">
                <label class="m-label">Container Path</label>
                <input v-model="vol.target" type="text" class="m-input mono" placeholder="/config" />
              </div>
            </div>

            <button type="button" class="m-dashed-add" @click="addVolume">
              <MdiIcon :path="mdiPlus" size="16" />
              <span>Add Volume Mount</span>
            </button>
          </div>
        </div>

        <!-- 5. Resources & Hardware -->
        <div class="m-accordion">
          <button type="button" class="m-acc-header" @click="toggleSection('resources')">
            <div class="m-acc-left">
              <MdiIcon :path="mdiCpu64Bit" size="18" style="color: #38bdf8" />
              <span class="m-acc-title">Resources &amp; Hardware</span>
            </div>
            <MdiIcon :path="openSections.resources ? mdiChevronUp : mdiChevronDown" size="20" />
          </button>

          <div v-if="openSections.resources" class="m-acc-body">
            <div class="m-field">
              <label class="m-label">Memory Limit</label>
              <div class="m-chip-row">
                <button
                  v-for="preset in MEMORY_PRESETS"
                  :key="preset"
                  type="button"
                  class="m-preset-chip"
                  :class="{ active: activeService.memoryLimit === preset }"
                  @click="activeService.memoryLimit = preset"
                >
                  {{ preset }}
                </button>
              </div>
              <input
                v-model="activeService.memoryLimit"
                type="text"
                class="m-input mono"
                placeholder="e.g. 512m, 2g (blank = unlimited)"
              />
            </div>

            <div class="m-field">
              <label class="m-label">CPU Limit (Cores)</label>
              <div class="m-chip-row">
                <button
                  v-for="preset in CPU_PRESETS"
                  :key="preset"
                  type="button"
                  class="m-preset-chip"
                  :class="{ active: activeService.cpuLimit === preset }"
                  @click="activeService.cpuLimit = preset"
                >
                  {{ preset }}
                </button>
              </div>
              <input
                v-model="activeService.cpuLimit"
                type="text"
                class="m-input mono"
                placeholder="e.g. 1.0, 2.0 (blank = unlimited)"
              />
            </div>

            <label class="m-toggle-row">
              <div class="m-toggle-text">
                <span class="m-toggle-title">Privileged Container</span>
                <span class="m-toggle-sub">Grant extended host kernel access</span>
              </div>
              <input v-model="activeService.privileged" type="checkbox" class="m-switch" />
            </label>

            <label class="m-toggle-row">
              <div class="m-toggle-text">
                <span class="m-toggle-title">GPU Passthrough</span>
                <span class="m-toggle-sub">Expose host GPU to container</span>
              </div>
              <input v-model="activeService.gpuEnabled" type="checkbox" class="m-switch" />
            </label>
          </div>
        </div>

        <!-- 6. Networks & Dependencies (shown when applicable) -->
        <div v-if="form.networks.length > 0 || otherServiceNames.length > 0" class="m-accordion">
          <button type="button" class="m-acc-header" @click="toggleSection('networks')">
            <div class="m-acc-left">
              <MdiIcon :path="mdiLanConnect" size="18" style="color: #818cf8" />
              <span class="m-acc-title">Networks &amp; Dependencies</span>
            </div>
            <MdiIcon :path="openSections.networks ? mdiChevronUp : mdiChevronDown" size="20" />
          </button>

          <div v-if="openSections.networks" class="m-acc-body">
            <div v-if="form.networks.length > 0" class="m-field">
              <label class="m-label">Attached Networks</label>
              <div class="m-chip-row">
                <button
                  v-for="net in form.networks"
                  :key="net.id"
                  type="button"
                  class="m-preset-chip"
                  :class="{ active: activeService.networks.includes(net.name) }"
                  @click="toggleServiceNetwork(net.name)"
                >
                  {{ net.name }}
                </button>
              </div>
            </div>

            <div v-if="otherServiceNames.length > 0" class="m-field">
              <label class="m-label">Depends On</label>
              <div class="m-chip-row">
                <button
                  v-for="dep in otherServiceNames"
                  :key="dep"
                  type="button"
                  class="m-preset-chip"
                  :class="{ active: activeService.dependsOn.includes(dep) }"
                  @click="toggleServiceDep(dep)"
                >
                  {{ dep }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </template>
    </template>

    <!-- RAW YAML MODE -->
    <div v-else class="m-yaml-card">
      <div class="m-yaml-toolbar">
        <span class="m-label" style="margin: 0">docker-compose.yml</span>
        <button type="button" class="btn btn-xs btn-ghost" @click="copyYaml">
          <MdiIcon :path="mdiContentCopy" size="14" />
          <span>Copy</span>
        </button>
      </div>
      <div class="m-yaml-editor-wrap">
        <CodeMirrorEditor
          v-model="rawYaml"
          mode="docker-compose"
          height="52vh"
          min-height="300px"
        />
      </div>
    </div>

    <!-- Sticky Bottom Save Bar -->
    <div class="m-sticky-save-bar">
      <button
        type="button"
        class="btn btn-primary m-save-btn"
        :disabled="isSaving"
        @click="handleSave"
      >
        <MdiIcon :path="mdiContentSave" size="18" />
        <span>{{ isSaving ? 'Saving & Redeploying...' : 'Save & Redeploy' }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.m-cfg-root {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding-bottom: 76px;
}

.m-segmented {
  display: flex;
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 12px;
  padding: 4px;
  gap: 4px;
}

.m-seg-btn {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  height: 38px;
  border-radius: 8px;
  border: none;
  background: transparent;
  color: var(--text-secondary, #94a3b8);
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
  touch-action: manipulation;
}

.m-seg-btn.active {
  background: var(--accent-primary, #38bdf8);
  color: #0f172a;
}

.m-alert-banner {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 12px;
  background: rgba(239, 68, 68, 0.14);
  border: 1px solid rgba(239, 68, 68, 0.35);
  color: #fca5a5;
  font-size: 12px;
}

.m-update-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 12px;
  background: rgba(251, 191, 36, 0.12);
  border: 1px solid rgba(251, 191, 36, 0.35);
}

.m-update-info {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.m-update-text {
  min-width: 0;
}

.m-update-title {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
}

.m-update-sub {
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.m-card {
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 14px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.m-service-bar {
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
  scrollbar-width: none;
}

.m-service-bar::-webkit-scrollbar {
  display: none;
}

.m-service-pills {
  display: flex;
  gap: 8px;
  padding: 2px 0;
}

.m-service-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 36px;
  padding: 0 14px;
  border-radius: 999px;
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  color: var(--text-secondary, #94a3b8);
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
  cursor: pointer;
  touch-action: manipulation;
}

.m-service-pill.active {
  background: var(--accent-primary, #38bdf8);
  border-color: var(--accent-primary, #38bdf8);
  color: #0f172a;
}

.m-service-add {
  border-style: dashed;
  color: var(--accent-primary, #38bdf8);
}

.m-accordion {
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 14px;
  overflow: hidden;
}

.m-acc-header {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px;
  background: transparent;
  border: none;
  color: var(--text-primary, #f8fafc);
  cursor: pointer;
  touch-action: manipulation;
}

.m-acc-left {
  display: flex;
  align-items: center;
  gap: 10px;
}

.m-acc-title {
  font-size: 14px;
  font-weight: 700;
}

.m-count-badge {
  font-size: 11px;
  font-weight: 700;
  padding: 2px 7px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.08);
  color: var(--text-secondary, #94a3b8);
}

.m-acc-body {
  padding: 0 14px 14px 14px;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  padding-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.m-field {
  display: flex;
  flex-direction: column;
  gap: 5px;
  min-width: 0;
}

.m-label {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.4px;
  color: var(--text-secondary, #94a3b8);
}

.m-input {
  width: 100%;
  min-height: 42px;
  padding: 8px 12px;
  border-radius: 10px;
  border: 1px solid var(--border-color, #334155);
  background: var(--bg-base, #0f172a);
  color: var(--text-primary, #f8fafc);
  font-size: 15px;
  box-sizing: border-box;
}

.m-input:focus {
  outline: none;
  border-color: var(--accent-primary, #38bdf8);
}

.m-row-2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

.m-toggle-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 0;
  cursor: pointer;
}

.m-toggle-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.m-toggle-title {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
}

.m-toggle-sub {
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
}

.m-switch {
  width: 22px;
  height: 22px;
  accent-color: var(--accent-primary, #38bdf8);
  flex-shrink: 0;
}

.m-item-card {
  background: var(--bg-surface-elevated, #243248);
  border: 1px solid var(--border-color, #334155);
  border-radius: 12px;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.m-item-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.m-item-idx {
  font-size: 11px;
  font-weight: 700;
  color: var(--text-secondary, #94a3b8);
  text-transform: uppercase;
}

.m-item-head-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.m-proto-pill {
  height: 28px;
  padding: 0 10px;
  border-radius: 6px;
  border: 1px solid rgba(56, 189, 248, 0.35);
  background: rgba(56, 189, 248, 0.14);
  color: var(--accent-primary, #38bdf8);
  font-size: 11px;
  font-weight: 700;
  cursor: pointer;
}

.m-mini-select {
  height: 28px;
  padding: 0 8px;
  border-radius: 6px;
  border: 1px solid var(--border-color, #334155);
  background: var(--bg-base, #0f172a);
  color: var(--text-primary, #f8fafc);
  font-size: 12px;
}

.m-icon-del {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  border: none;
  background: rgba(239, 68, 68, 0.14);
  color: #f87171;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  flex-shrink: 0;
}

.m-env-key-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.m-env-key {
  font-weight: 700;
}

.m-search-box {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  height: 38px;
  border-radius: 10px;
  background: var(--bg-base, #0f172a);
  border: 1px solid var(--border-color, #334155);
}

.m-search-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  color: var(--text-primary, #f8fafc);
  font-size: 14px;
}

.m-dashed-add {
  width: 100%;
  height: 40px;
  border-radius: 10px;
  border: 1px dashed var(--border-color, #475569);
  background: transparent;
  color: var(--accent-primary, #38bdf8);
  font-size: 13px;
  font-weight: 700;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  cursor: pointer;
  touch-action: manipulation;
}

.m-chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 4px;
}

.m-preset-chip {
  height: 32px;
  padding: 0 12px;
  border-radius: 999px;
  border: 1px solid var(--border-color, #334155);
  background: var(--bg-base, #0f172a);
  color: var(--text-secondary, #94a3b8);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  touch-action: manipulation;
}

.m-preset-chip.active {
  background: var(--accent-primary, #38bdf8);
  border-color: var(--accent-primary, #38bdf8);
  color: #0f172a;
}

.m-empty-hint {
  font-size: 12px;
  color: var(--text-secondary, #94a3b8);
  text-align: center;
  padding: 8px 0;
}

.m-full-btn {
  width: 100%;
  height: 40px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
}

.m-yaml-card {
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 14px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.m-yaml-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.m-yaml-editor-wrap {
  border-radius: 10px;
  overflow: hidden;
  border: 1px solid var(--border-color, #334155);
}

.m-sticky-save-bar {
  position: fixed;
  left: 0;
  right: 0;
  bottom: calc(58px + env(safe-area-inset-bottom, 0px));
  z-index: 90;
  padding: 10px 12px;
  background: var(--bg-surface, #1e293b);
  border-top: 1px solid var(--border-color, #334155);
  box-shadow: 0 -6px 20px rgba(0, 0, 0, 0.35);
}

.m-save-btn {
  width: 100%;
  height: 44px;
  border-radius: 12px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 700;
}
</style>
