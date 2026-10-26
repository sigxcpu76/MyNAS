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
  DEFAULT_COMPOSE_MEMORY_LIMIT,
  DEFAULT_COMPOSE_CPU_LIMIT,
  formToComposeYaml,
  generateRandomHighPort,
  generateUid,
} from '../../domain/composeModel';
import { nasState, notify } from '../store';
import { generateNanosecondId, toZfsSafeSlug } from '../../domain/id';
import { isDotDataset } from '../../domain/models';
import CodeMirrorEditor from './CodeMirrorEditor.vue';
import MdiIcon from './MdiIcon.vue';
import PathSelector from './PathSelector.vue';
import { services } from '../../container';
import {
  mdiAlert,
  mdiAlertCircle,
  mdiCheckCircle,
  mdiClose,
  mdiContentCopy,
  mdiContentSave,
  mdiDiceMultipleOutline,
  mdiDocker,
  mdiHarddisk,
  mdiLanConnect,
  mdiLoading,
  mdiOpenInNew,
  mdiPlus,
  mdiRocketLaunchOutline,
  mdiStar,
} from '@mdi/js';

const props = defineProps<{
  initialAppId?: string;
  initialTitle?: string;
  initialComposeYaml?: string;
  isEditMode?: boolean;
  isSaving?: boolean;
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

// Resource Limit Select Options (128m, 256m, 512m, 1g, 2g, 4g, 8g, 16g, 32g & 0.5, 1.0, 2, 4, 8, 16, 32)
const MEMORY_LIMIT_SELECT_OPTIONS = [
  { value: '128m', label: '128m (128 MB)' },
  { value: '256m', label: '256m (256 MB)' },
  { value: '512m', label: '512m (512 MB — Default)' },
  { value: '1g', label: '1g (1 GB)' },
  { value: '2g', label: '2g (2 GB)' },
  { value: '4g', label: '4g (4 GB)' },
  { value: '8g', label: '8g (8 GB)' },
  { value: '16g', label: '16g (16 GB)' },
  { value: '32g', label: '32g (32 GB)' },
];

const CPU_LIMIT_SELECT_OPTIONS = [
  { value: '0.5', label: '0.5 (Half Core)' },
  { value: '1.0', label: '1.0 (1 Core — Default)' },
  { value: '2', label: '2 (2 Cores)' },
  { value: '4', label: '4 (4 Cores)' },
  { value: '8', label: '8 (8 Cores)' },
  { value: '16', label: '16 (16 Cores)' },
  { value: '32', label: '32 (32 Cores)' },
];

function isCustomMemoryLimit(val?: string): boolean {
  if (!val) return false;
  return !MEMORY_LIMIT_SELECT_OPTIONS.some((o) => o.value === val);
}

function isCustomCpuLimit(val?: string): boolean {
  if (!val) return false;
  return !CPU_LIMIT_SELECT_OPTIONS.some((o) => o.value === val) && val !== '1';
}

// Ensure active service has memoryLimit and cpuLimit initialized
watch(
  activeService,
  (svc) => {
    if (svc) {
      if (!svc.memoryLimit) svc.memoryLimit = DEFAULT_COMPOSE_MEMORY_LIMIT;
      if (!svc.cpuLimit) svc.cpuLimit = DEFAULT_COMPOSE_CPU_LIMIT;
      else if (svc.cpuLimit === '1') svc.cpuLimit = '1.0';
    }
  },
  { immediate: true }
);

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
      if (isDotDataset(d)) continue;
      if (d.mountpoint && d.mountpoint !== 'none' && d.mountpoint !== 'legacy') {
        paths.push(d.mountpoint);
      } else {
        paths.push(`/nas/${cleanPool}/${d.name.split('/').pop()}`);
      }
    }
  }
  return paths;
});

const appsPoolName = computed(() => nasState.dockerConfig?.pool || nasState.systemConfig?.mainPool || 'tank');
const appsPoolAppdataDir = computed(
  () => nasState.dockerConfig?.appdataDataset || `/nas/${appsPoolName.value}/apps`
);

function defaultAppdataMount(appId: string, subfolder = 'data'): string {
  const cleanId = toZfsSafeSlug(appId.trim()) || 'app';
  return `${appsPoolAppdataDir.value}/${cleanId}/${subfolder}`;
}

const commonContainerPaths = [
  '/config',
  '/data',
  '/media',
  '/app/data',
  '/var/www/html',
  '/var/lib/postgresql/data',
  '/var/lib/mysql',
  '/etc/letsencrypt',
  '/downloads',
  '/etc/pihole',
];

function isNamedVolumeMount(source: string): boolean {
  if (!source) return false;
  return !source.startsWith('/') && !source.startsWith('.');
}

function setMountType(mount: ComposeVolumeMountModel, type: 'bind' | 'volume') {
  if (type === 'volume') {
    if (form.volumes.length === 0) {
      addTopLevelVolume();
    }
    mount.source = form.volumes[0]?.name || `${form.appId || 'app'}_data`;
  } else {
    mount.source = defaultAppdataMount(
      form.appId,
      mount.target ? mount.target.split('/').filter(Boolean).pop() || 'data' : 'data'
    );
  }
}

function createAndAttachNamedVolume(mount: ComposeVolumeMountModel) {
  const volName = `${form.appId || 'app'}_vol_${form.volumes.length + 1}`;
  form.volumes.push({
    id: generateUid(),
    name: volName,
    driver: 'local',
    external: false,
  });
  mount.source = volName;
  notify('info', `Created named volume "${volName}"`);
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
const allContainerPorts = computed(() => {
  const result: Array<{
    serviceName: string;
    containerPort: string;
    hostPort: string;
    protocol: string;
    publishToHost: boolean;
  }> = [];
  const seen = new Set<string>();
  for (const s of form.services) {
    for (const p of s.ports) {
      const cp = p.containerPort.trim();
      if (cp && !seen.has(cp)) {
        seen.add(cp);
        result.push({
          serviceName: s.name,
          containerPort: cp,
          hostPort: p.hostPort.trim(),
          protocol: p.protocol,
          publishToHost: p.publishToHost !== false,
        });
      }
    }
  }
  return result;
});

function toggleManagementPort(containerPort: string) {
  const clean = containerPort.trim();
  if (form.managementPort === clean) {
    form.managementPort = '';
  } else {
    form.managementPort = clean;
    if (form.portals && form.portals.length > 0) {
      form.portals[0].port = clean;
    }
  }
}

watch(
  () => form.managementPort,
  (newVal) => {
    const clean = (newVal || '').trim();
    if (clean && form.portals && form.portals.length > 0) {
      form.portals[0].port = clean;
    }
  }
);

function addPort(svc: ComposeServiceModel) {
  const randHost = generateRandomHighPort();
  svc.ports.push({ hostPort: String(randHost), containerPort: '', protocol: 'tcp', publishToHost: true });
}

function removePort(svc: ComposeServiceModel, idx: number) {
  svc.ports.splice(idx, 1);
}

// Preemptive Host Port Availability Tracking
interface PortCheckResult {
  available?: boolean;
  inUseBy?: string;
  checking?: boolean;
  isDuplicate?: boolean;
}

const portCheckCache = reactive<Record<string, PortCheckResult>>({});
let checkDebounceTimer: any = null;

function schedulePortAvailabilityCheck() {
  clearTimeout(checkDebounceTimer);
  checkDebounceTimer = setTimeout(async () => {
    const portEntries: Array<{ portNum: number; proto: string; key: string }> = [];
    const portCountByKey = new Map<string, number>();

    for (const svc of form.services) {
      for (const p of svc.ports) {
        if (p.publishToHost !== false && p.hostPort && p.hostPort.trim()) {
          const num = parseInt(p.hostPort.trim(), 10);
          if (!isNaN(num) && num > 0 && num <= 65535) {
            const proto = p.protocol || 'tcp';
            const key = `${num}/${proto}`;
            portEntries.push({ portNum: num, proto, key });
            portCountByKey.set(key, (portCountByKey.get(key) || 0) + 1);
          }
        }
      }
    }

    const portsToQueryByProto: Record<string, number[]> = { tcp: [], udp: [] };
    for (const entry of portEntries) {
      const isDup = (portCountByKey.get(entry.key) || 0) > 1;
      if (isDup) {
        portCheckCache[entry.key] = { isDuplicate: true };
      } else {
        if (!portCheckCache[entry.key] || portCheckCache[entry.key].isDuplicate) {
          portCheckCache[entry.key] = { checking: true };
        }
        const list = portsToQueryByProto[entry.proto] || (portsToQueryByProto[entry.proto] = []);
        if (!list.includes(entry.portNum)) {
          list.push(entry.portNum);
        }
      }
    }

    for (const [proto, ports] of Object.entries(portsToQueryByProto)) {
      if (ports.length === 0) continue;
      try {
        const statuses = await services.apps.checkHostPorts(ports, proto, form.appId);
        for (const st of statuses) {
          const key = `${st.port}/${st.protocol}`;
          if (!portCountByKey.has(key) || (portCountByKey.get(key) || 0) <= 1) {
            portCheckCache[key] = {
              available: st.available,
              inUseBy: st.inUseBy,
              checking: false,
              isDuplicate: false,
            };
          }
        }
      } catch {
        for (const pNum of ports) {
          const key = `${pNum}/${proto}`;
          if (portCheckCache[key]?.checking) {
            delete portCheckCache[key];
          }
        }
      }
    }
  }, 350);
}

function getHostPortStatus(p: ComposePortModel): PortCheckResult | null {
  if (p.publishToHost === false || !p.hostPort || !p.hostPort.trim()) return null;
  const num = parseInt(p.hostPort.trim(), 10);
  if (isNaN(num) || num <= 0 || num > 65535) return null;
  const key = `${num}/${p.protocol || 'tcp'}`;
  return portCheckCache[key] || null;
}

watch(
  () =>
    form.services
      .map((s) => s.ports.map((p) => `${p.publishToHost}:${p.hostPort}:${p.protocol}`).join(','))
      .join(';'),
  () => {
    schedulePortAvailabilityCheck();
  },
  { immediate: true }
);

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

// Local submission state
const isSubmitting = ref(false);

watch(
  () => props.isSaving,
  (val) => {
    if (!val) {
      isSubmitting.value = false;
    }
  }
);

// Submission
function handleSave() {
  if (isSubmitting.value || props.isSaving) return;

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

  // Preemptive check: block submission if there are known host port conflicts
  if (editorMode.value === 'form') {
    for (const s of form.services) {
      for (const p of s.ports) {
        if (p.publishToHost !== false && p.hostPort && p.hostPort.trim()) {
          const st = getHostPortStatus(p);
          if (st?.isDuplicate) {
            notify('error', `Host port ${p.hostPort} is mapped multiple times in this application`);
            return;
          }
          if (st?.available === false) {
            notify('error', `Host port ${p.hostPort}/${p.protocol || 'tcp'} is already in use by ${st.inUseBy || 'another service'}`);
            return;
          }
        }
      }
    }
  }

  isSubmitting.value = true;
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
          <div style="display: flex; background: var(--bg-elevated); border: 1px solid var(--border-subtle); border-radius: 6px; overflow: hidden">
            <button
              class="btn btn-sm"
              :style="{
                background: editorMode === 'form' ? 'var(--accent-primary)' : 'transparent',
                color: editorMode === 'form' ? 'var(--accent-contrast)' : 'var(--text-secondary)',
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
                color: editorMode === 'raw' ? 'var(--accent-contrast)' : 'var(--text-secondary)',
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
              <MdiIcon :path="mdiClose" size="13" />
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
                <MdiIcon :path="mdiContentCopy" size="13" style="margin-right: 4px" />
                Duplicate
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
            <!-- Web UI Management Port Banner / Selector -->
            <div
              v-if="allContainerPorts.length > 0"
              style="display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 14px; margin-bottom: 14px; background: rgba(56, 189, 248, 0.08); border: 1px solid rgba(56, 189, 248, 0.25); border-radius: 6px"
            >
              <div style="display: flex; align-items: center; gap: 8px">
                <MdiIcon :path="mdiOpenInNew" size="18" style="color: var(--accent-primary); flex-shrink: 0" />
                <div>
                  <div style="font-size: 12.5px; font-weight: 600; color: var(--text-primary)">
                    Open Web UI Management Port
                  </div>
                  <div style="font-size: 11.5px; color: var(--text-secondary)">
                    Pick which container internal port provides the web management interface.
                  </div>
                </div>
              </div>
              <div style="display: flex; align-items: center; gap: 8px">
                <select
                  v-model="form.managementPort"
                  class="form-input mono"
                  style="padding: 4px 10px; font-size: 12px; min-width: 220px"
                >
                  <option value="">Auto-detect / Default</option>
                  <option
                    v-for="cp in allContainerPorts"
                    :key="cp.containerPort"
                    :value="cp.containerPort"
                  >
                    Port {{ cp.containerPort }} ({{ cp.serviceName }} : {{ cp.publishToHost ? (cp.hostPort || 'auto') : 'internal' }} → {{ cp.containerPort }})
                  </option>
                </select>
              </div>
            </div>

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
                  <th style="padding: 6px 8px; width: 120px">MAP TO HOST</th>
                  <th style="padding: 6px 8px">HOST PORT</th>
                  <th style="padding: 6px 8px; width: 140px">CONTAINER PORT</th>
                  <th style="padding: 6px 8px; width: 100px">PROTOCOL</th>
                  <th style="padding: 6px 8px; width: 90px; text-align: center">WEB UI</th>
                  <th style="padding: 6px 8px; width: 40px"></th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(p, idx) in activeService.ports" :key="idx" style="border-bottom: 1px solid var(--border-subtle)">
                  <td style="padding: 6px 8px; vertical-align: middle">
                    <label style="display: inline-flex; align-items: center; gap: 6px; cursor: pointer; font-size: 12px; margin: 0">
                      <input
                        type="checkbox"
                        :checked="p.publishToHost !== false"
                        @change="p.publishToHost = ($event.target as HTMLInputElement).checked; if (!p.publishToHost) { p.hostPort = ''; } else if (!p.hostPort) { p.hostPort = String(generateRandomHighPort()); }"
                      />
                      <span :style="{ color: p.publishToHost !== false ? 'var(--text-primary)' : 'var(--text-secondary)' }">
                        {{ p.publishToHost !== false ? 'Publish' : 'Internal' }}
                      </span>
                    </label>
                  </td>
                  <td style="padding: 6px 8px; vertical-align: middle">
                    <div v-if="p.publishToHost !== false" style="display: flex; align-items: center; gap: 6px; flex-wrap: wrap">
                      <div style="display: flex; align-items: center; gap: 4px">
                        <input
                          v-model="p.hostPort"
                          type="text"
                          class="form-input mono"
                          placeholder="32000+"
                          style="padding: 4px 8px; font-size: 12.5px; width: 95px"
                        />
                        <button
                          type="button"
                          class="btn btn-sm btn-ghost"
                          style="padding: 2px 5px; font-size: 11.5px; line-height: 1"
                          title="Pick another random high port (32000+)"
                          @click="p.hostPort = String(generateRandomHighPort())"
                        >
                          <MdiIcon :path="mdiDiceMultipleOutline" size="14" />
                        </button>
                      </div>
                      <!-- Preemptive port availability check indicator -->
                      <div v-if="getHostPortStatus(p)" style="display: inline-flex; align-items: center">
                        <span
                          v-if="getHostPortStatus(p)?.isDuplicate"
                          class="badge badge-warning"
                          style="padding: 2px 7px; font-size: 11px; display: inline-flex; align-items: center; gap: 4px"
                          title="Port duplicate: mapped multiple times in this application"
                        >
                          <MdiIcon :path="mdiAlert" size="13" /> Duplicate
                        </span>
                        <span
                          v-else-if="getHostPortStatus(p)?.checking"
                          class="badge badge-neutral"
                          style="padding: 2px 7px; font-size: 11px"
                        >
                          Checking...
                        </span>
                        <span
                          v-else-if="getHostPortStatus(p)?.available"
                          class="badge badge-success"
                          style="padding: 2px 7px; font-size: 11px; display: inline-flex; align-items: center; gap: 4px"
                          title="Host port is free and ready to bind"
                        >
                          <MdiIcon :path="mdiCheckCircle" size="13" /> Available
                        </span>
                        <span
                          v-else-if="getHostPortStatus(p)?.available === false"
                          class="badge badge-danger"
                          style="padding: 2px 7px; font-size: 11px; display: inline-flex; align-items: center; gap: 4px"
                          :title="`Port conflict: in use by ${getHostPortStatus(p)?.inUseBy}`"
                        >
                          <MdiIcon :path="mdiAlertCircle" size="13" /> In Use: {{ getHostPortStatus(p)?.inUseBy }}
                        </span>
                      </div>
                    </div>
                    <div v-else>
                      <span class="badge" style="background: rgba(148, 163, 184, 0.15); color: var(--text-secondary); font-size: 11px; padding: 3px 8px">
                        Internal only (expose)
                      </span>
                    </div>
                  </td>
                  <td style="padding: 6px 8px; vertical-align: middle">
                    <input
                      v-model="p.containerPort"
                      type="text"
                      class="form-input mono"
                      placeholder="e.g. 80"
                      style="padding: 4px 8px; font-size: 12.5px"
                    />
                  </td>
                  <td style="padding: 6px 8px; vertical-align: middle">
                    <select v-model="p.protocol" class="form-input" style="padding: 4px 8px; font-size: 12.5px">
                      <option value="tcp">TCP</option>
                      <option value="udp">UDP</option>
                    </select>
                  </td>
                  <td style="padding: 6px 8px; text-align: center; vertical-align: middle">
                    <button
                      type="button"
                      class="btn btn-sm"
                      :class="form.managementPort === p.containerPort.trim() && p.containerPort.trim() ? 'btn-primary' : 'btn-ghost'"
                      style="padding: 2px 8px; font-size: 11px; white-space: nowrap; display: inline-flex; align-items: center"
                      :title="form.managementPort === p.containerPort.trim() ? 'Active Web UI Port' : 'Set as Web UI Management Port'"
                      :disabled="!p.containerPort.trim()"
                      @click="toggleManagementPort(p.containerPort)"
                    >
                      <MdiIcon
                        v-if="form.managementPort === p.containerPort.trim() && p.containerPort.trim()"
                        :path="mdiStar"
                        size="13"
                        style="margin-right: 3px"
                      />
                      {{ form.managementPort === p.containerPort.trim() && p.containerPort.trim() ? 'Primary' : 'Set UI' }}
                    </button>
                  </td>
                  <td style="padding: 6px 8px; text-align: center; vertical-align: middle">
                    <button class="btn btn-sm btn-danger" style="padding: 2px 6px" @click="removePort(activeService, idx)">
                      <MdiIcon :path="mdiClose" size="13" />
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
                      <MdiIcon :path="mdiClose" size="13" />
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- Sub-Tab: Volumes -->
          <div v-else-if="serviceSubTab === 'volumes'">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; flex-wrap: wrap; gap: 8px">
              <div>
                <span style="font-size: 13px; font-weight: 600; color: var(--text-primary)">
                  Storage & Volume Mounts
                </span>
                <div style="font-size: 11.5px; color: var(--text-secondary); margin-top: 2px">
                  Mount ZFS datasets (under <code class="mono" style="color: var(--accent-primary)">/nas/{{ appsPoolName }}</code>) or persistent Docker named volumes into the container.
                </div>
              </div>
              <button class="btn btn-sm btn-primary" style="display: inline-flex; align-items: center; gap: 4px" @click="addVolumeMount(activeService)">
                <MdiIcon :path="mdiPlus" size="14" />
                <span>Add Storage Mount</span>
              </button>
            </div>

            <div v-if="activeService.volumes.length === 0" style="padding: 32px 16px; text-align: center; border: 1px dashed var(--border-subtle); border-radius: 8px; color: var(--text-secondary); font-size: 13px">
              <div>No storage volumes mounted for this service.</div>
              <div style="font-size: 11.5px; margin-top: 4px; opacity: 0.8">Click "Add Storage Mount" above to connect ZFS datasets, app data, or named volumes.</div>
            </div>

            <div v-else style="display: flex; flex-direction: column; gap: 12px">
              <div
                v-for="(v, idx) in activeService.volumes"
                :key="idx"
                style="background: rgba(255,255,255,0.02); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 12px; display: flex; flex-direction: column; gap: 10px"
              >
                <!-- Top Header of Volume Card: Type Selector, Presets & Remove -->
                <div style="display: flex; justify-content: space-between; align-items: center; gap: 8px; flex-wrap: wrap">
                  <!-- Type Pill / Switcher -->
                  <div style="display: flex; align-items: center; gap: 6px">
                    <span style="font-size: 11px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase">Volume Type:</span>
                    <div style="display: inline-flex; border: 1px solid var(--border-strong); border-radius: 6px; overflow: hidden; background: var(--bg-surface)">
                      <button
                        type="button"
                        class="btn-sm"
                        :style="{
                          padding: '3px 10px',
                          fontSize: '11.5px',
                          border: 'none',
                          cursor: 'pointer',
                          background: !isNamedVolumeMount(v.source) ? 'var(--accent-primary)' : 'transparent',
                          color: !isNamedVolumeMount(v.source) ? 'var(--accent-contrast)' : 'var(--text-secondary)',
                          fontWeight: !isNamedVolumeMount(v.source) ? '600' : 'normal',
                        }"
                        @click="setMountType(v, 'bind')"
                      >
                        Host ZFS Dataset / Path
                      </button>
                      <button
                        type="button"
                        class="btn-sm"
                        :style="{
                          padding: '3px 10px',
                          fontSize: '11.5px',
                          border: 'none',
                          cursor: 'pointer',
                          background: isNamedVolumeMount(v.source) ? 'var(--accent-primary)' : 'transparent',
                          color: isNamedVolumeMount(v.source) ? 'var(--accent-contrast)' : 'var(--text-secondary)',
                          fontWeight: isNamedVolumeMount(v.source) ? '600' : 'normal',
                        }"
                        @click="setMountType(v, 'volume')"
                      >
                        Named Volume
                      </button>
                    </div>
                  </div>

                  <!-- Quick Presets Dropdown (for host bind) -->
                  <div v-if="!isNamedVolumeMount(v.source)" style="display: flex; align-items: center; gap: 6px">
                    <label style="font-size: 11px; color: var(--text-secondary)">Preset:</label>
                    <select
                      class="input mono"
                      style="font-size: 11px; padding: 2px 6px; height: 26px"
                      @change="(e: any) => { if (e.target.value) { v.source = e.target.value; e.target.value = ''; } }"
                    >
                      <option value="">Choose preset / dataset...</option>
                      <optgroup label="App Data Presets">
                        <option :value="defaultAppdataMount(form.appId, 'data')">
                          App Data (/nas/{{ appsPoolName }}/apps/{{ form.appId || 'app' }}/data)
                        </option>
                        <option :value="defaultAppdataMount(form.appId, 'config')">
                          App Config (/nas/{{ appsPoolName }}/apps/{{ form.appId || 'app' }}/config)
                        </option>
                      </optgroup>
                      <optgroup label="Shared Media & Downloads">
                        <option :value="`/nas/${appsPoolName}/media`">Media (/nas/{{ appsPoolName }}/media)</option>
                        <option :value="`/nas/${appsPoolName}/downloads`">Downloads (/nas/{{ appsPoolName }}/downloads)</option>
                      </optgroup>
                      <optgroup v-if="zfsDatasetPaths.length > 0" label="Existing ZFS Datasets">
                        <option v-for="d in zfsDatasetPaths" :key="d" :value="d">
                          {{ d }}
                        </option>
                      </optgroup>
                    </select>
                  </div>

                  <button
                    type="button"
                    class="btn btn-sm btn-icon btn-danger"
                    style="padding: 2px 6px"
                    title="Remove Mount"
                    aria-label="Remove Mount"
                    @click="removeVolumeMount(activeService, idx)"
                  >
                    <MdiIcon :path="mdiClose" size="13" />
                  </button>
                </div>

                <!-- Middle Grid: Source, Target, Mode -->
                <div style="display: grid; grid-template-columns: 1fr 1fr 140px; gap: 10px; align-items: flex-start">
                  <!-- Source Column -->
                  <div>
                    <label style="display: block; font-size: 11px; font-weight: 600; color: var(--text-secondary); margin-bottom: 4px">
                      {{ isNamedVolumeMount(v.source) ? 'NAMED VOLUME' : 'HOST PATH / ZFS DATASET' }}
                    </label>

                    <!-- Case A: Named Volume Selector -->
                    <div v-if="isNamedVolumeMount(v.source)" style="display: flex; gap: 6px; align-items: center">
                      <select v-model="v.source" class="form-input mono" style="font-size: 12px; flex: 1; padding: 4px 8px">
                        <option v-for="vol in form.volumes" :key="vol.id" :value="vol.name">
                          {{ vol.name }}
                        </option>
                      </select>
                      <button
                        type="button"
                        class="btn btn-sm"
                        style="padding: 4px 8px; font-size: 11.5px; white-space: nowrap"
                        title="Create new named volume"
                        @click="createAndAttachNamedVolume(v)"
                      >
                        + New Volume
                      </button>
                    </div>

                    <!-- Case B: Host Path / ZFS Dataset Selector -->
                    <div v-else>
                      <PathSelector
                        v-model="v.source"
                        :placeholder="defaultAppdataMount(form.appId, 'data')"
                        :root-hint="`/nas/${appsPoolName}`"
                      />
                    </div>
                  </div>

                  <!-- Container Target Column -->
                  <div>
                    <label style="display: block; font-size: 11px; font-weight: 600; color: var(--text-secondary); margin-bottom: 4px">
                      CONTAINER DESTINATION PATH
                    </label>
                    <input
                      v-model="v.target"
                      type="text"
                      class="form-input mono"
                      placeholder="e.g. /config or /data"
                      list="container-target-options"
                      style="font-size: 12px; padding: 4px 8px; width: 100%"
                    />
                  </div>

                  <!-- Mode Column -->
                  <div>
                    <label style="display: block; font-size: 11px; font-weight: 600; color: var(--text-secondary); margin-bottom: 4px">
                      ACCESS MODE
                    </label>
                    <select v-model="v.mode" class="form-input" style="font-size: 12px; padding: 4px 8px; width: 100%">
                      <option value="rw">Read-Write (rw)</option>
                      <option value="ro">Read-Only (ro)</option>
                    </select>
                  </div>
                </div>
              </div>
            </div>

            <!-- Datalist for Container Target suggestions -->
            <datalist id="container-target-options">
              <option v-for="target in commonContainerPaths" :key="target" :value="target" />
            </datalist>
          </div>

          <!-- Sub-Tab: Resources & Hardware -->
          <div v-else-if="serviceSubTab === 'resources'">
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 14px; margin-bottom: 16px">
              <div>
                <label class="form-label">Memory Limit</label>
                <select
                  v-model="activeService.memoryLimit"
                  class="form-input mono"
                >
                  <option v-for="opt in MEMORY_LIMIT_SELECT_OPTIONS" :key="opt.value" :value="opt.value">
                    {{ opt.label }}
                  </option>
                  <option
                    v-if="isCustomMemoryLimit(activeService.memoryLimit)"
                    :value="activeService.memoryLimit"
                  >
                    {{ activeService.memoryLimit }} (Custom)
                  </option>
                </select>
                <span style="font-size: 11px; color: var(--text-secondary)">
                  Maps to <code>mem_limit</code> in Compose (default 512m).
                </span>
              </div>
              <div>
                <label class="form-label">CPU Limit</label>
                <select
                  v-model="activeService.cpuLimit"
                  class="form-input mono"
                >
                  <option v-for="opt in CPU_LIMIT_SELECT_OPTIONS" :key="opt.value" :value="opt.value">
                    {{ opt.label }}
                  </option>
                  <option
                    v-if="isCustomCpuLimit(activeService.cpuLimit)"
                    :value="activeService.cpuLimit"
                  >
                    {{ activeService.cpuLimit }} Cores (Custom)
                  </option>
                </select>
                <span style="font-size: 11px; color: var(--text-secondary)">
                  Fractional CPU cores (maps to <code>cpus</code>, default 1.0).
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
                <button class="btn btn-sm btn-danger" @click="removeDevice(activeService, idx)">
                  <MdiIcon :path="mdiClose" size="13" />
                </button>
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
                <button class="btn btn-sm btn-danger" style="padding: 2px 6px" @click="removeTopLevelNetwork(idx)">
                  <MdiIcon :path="mdiClose" size="13" />
                </button>
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
                <button class="btn btn-sm btn-danger" style="padding: 2px 6px" @click="removeTopLevelVolume(idx)">
                  <MdiIcon :path="mdiClose" size="13" />
                </button>
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
      <button class="btn" :disabled="props.isSaving || isSubmitting" @click="emit('cancel')">Cancel</button>
      <button
        class="btn btn-primary"
        :disabled="props.isSaving || isSubmitting"
        style="display: inline-flex; align-items: center; gap: 6px"
        @click="handleSave"
      >
        <MdiIcon v-if="props.isSaving || isSubmitting" :path="mdiLoading" class="spin-icon" size="15" />
        <MdiIcon v-else :path="props.isEditMode ? mdiContentSave : mdiRocketLaunchOutline" size="15" />
        <span>
          {{
            (props.isSaving || isSubmitting)
              ? (props.isEditMode ? 'Saving & Redeploying...' : 'Deploying Application (pulling & starting)...')
              : (props.isEditMode ? 'Save & Redeploy Stack' : 'Deploy Application')
          }}
        </span>
      </button>
    </div>

    <!-- Bulk .env Paste Modal -->
    <div v-if="showBulkEnv" class="modal-backdrop" @click.self="showBulkEnv = false">
      <div class="modal-card" style="max-width: 600px">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px">
          <h4 style="font-size: 16px; font-weight: 700; color: var(--text-primary)">
            Bulk Import Environment Variables (.env)
          </h4>
          <button class="btn btn-sm" @click="showBulkEnv = false">
            <MdiIcon :path="mdiClose" size="14" />
          </button>
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
