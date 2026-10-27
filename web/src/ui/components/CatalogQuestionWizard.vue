<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import {
  mdiAlertCircleOutline,
  mdiCheck,
  mdiClose,
  mdiCodeBraces,
  mdiCog,
  mdiContentCopy,
  mdiDatabase,
  mdiDocker,
  mdiEye,
  mdiEyeOff,
  mdiFolderOutline,
  mdiHarddisk,
  mdiInformationOutline,
  mdiKeyVariant,
  mdiOpenInNew,
  mdiPencil,
  mdiRefresh,
  mdiRocketLaunchOutline,
  mdiServerNetwork,
  mdiShieldAccount,
  mdiTune,
} from '@mdi/js';
import type { CatalogApp, CatalogQuestion, CatalogQuestionGroup } from '../../domain/models';
import {
  categorizeQuestion,
  generateComposeFromQuestions,
  getAllAppQuestions,
  getQuestionsDefaultAnswers,
  organizeQuestionGroups,
} from '../../domain/catalogQuestions';
import CodeMirrorEditor from './CodeMirrorEditor.vue';
import MdiIcon from './MdiIcon.vue';
import PathSelector from './PathSelector.vue';
import { notify } from '../store';
import { toZfsSafeSlug } from '../../domain/id';

const props = withDefaults(
  defineProps<{
    app: CatalogApp;
    appsPool?: string;
    isDeploying?: boolean;
  }>(),
  {
    appsPool: 'tank',
    isDeploying: false,
  }
);

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'deploy', data: { appId: string; title: string; composeYaml: string }): void;
  (e: 'openInEditor', data: { appId: string; title: string; composeYaml: string }): void;
  (e: 'skipWizard'): void;
}>();

// Form answers state
const answers = reactive<Record<string, string>>(
  getQuestionsDefaultAnswers(props.app, props.appsPool)
);

// Password visibility state per variable
const showPassword = reactive<Record<string, boolean>>({});

// Tab navigation state
const groups = computed<CatalogQuestionGroup[]>(() => organizeQuestionGroups(props.app));
const activeTab = ref<string>(groups.value[0]?.name || 'Network & Web Portal');

// Split preview state
const showLivePreview = ref(true);
const isCopied = ref(false);

// Active group questions
const activeGroupQuestions = computed<CatalogQuestion[]>(() => {
  const grp = groups.value.find((g) => g.name === activeTab.value);
  return grp ? grp.questions : [];
});

const totalQuestionsCount = computed(() => getAllAppQuestions(props.app).length);

// Live compiled Compose YAML
const compiledComposeYaml = computed<string>(() => {
  return generateComposeFromQuestions(props.app, answers, props.appsPool);
});

// Primary Web UI Port (if defined)
const activeWebPort = computed<number | null>(() => {
  for (const q of getAllAppQuestions(props.app)) {
    const vLower = q.variable.toLowerCase();
    if (q.type === 'int' && (vLower.includes('port') || vLower.includes('web_port'))) {
      const val = parseInt(answers[q.variable] || q.defaultValue, 10);
      if (!isNaN(val) && val > 0) return val;
    }
  }
  return props.app.defaultPort && props.app.defaultPort > 0 ? props.app.defaultPort : null;
});

function getGroupIcon(name: string): string {
  switch (name) {
    case 'Network & Web Portal':
      return mdiServerNetwork;
    case 'Persistent Storage & Datasets':
      return mdiHarddisk;
    case 'Application Configuration & Credentials':
      return mdiCog;
    case 'User Permissions & System':
      return mdiShieldAccount;
    default:
      return mdiTune;
  }
}

function isPasswordVariable(q: CatalogQuestion): boolean {
  const vLower = q.variable.toLowerCase();
  return (
    vLower.includes('password') ||
    vLower.includes('secret') ||
    vLower.includes('token') ||
    vLower.includes('key')
  );
}

function toggleShowPassword(variable: string) {
  showPassword[variable] = !showPassword[variable];
}

function generateRandomPassword(variable: string) {
  const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()_+';
  let pass = '';
  for (let i = 0; i < 20; i++) {
    pass += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  answers[variable] = pass;
  showPassword[variable] = true;
  notify('info', 'Generated secure random password');
}

const cleanAppId = computed(() => toZfsSafeSlug(props.app.appId || props.app.title) || 'app');

function resetStoragePath(variable: string) {
  const parts = variable.split('.');
  const leaf = parts[parts.length - 1] || 'data';
  answers[variable] = `/mnt/${props.appsPool}/apps/${cleanAppId.value}/${leaf}`;
}

async function copyYaml() {
  try {
    await navigator.clipboard.writeText(compiledComposeYaml.value);
    isCopied.value = true;
    notify('success', 'Compose YAML copied to clipboard');
    setTimeout(() => {
      isCopied.value = false;
    }, 2000);
  } catch {
    notify('error', 'Failed to copy YAML');
  }
}

function handleOpenInEditor() {
  emit('openInEditor', {
    appId: cleanAppId.value,
    title: props.app.title || props.app.appId,
    composeYaml: compiledComposeYaml.value,
  });
}

function handleDeploy() {
  emit('deploy', {
    appId: cleanAppId.value,
    title: props.app.title || props.app.appId,
    composeYaml: compiledComposeYaml.value,
  });
}

// Ensure active tab points to a valid group if groups change
watch(
  groups,
  (newGroups) => {
    if (newGroups.length > 0 && !newGroups.some((g) => g.name === activeTab.value)) {
      activeTab.value = newGroups[0].name;
    }
  },
  { immediate: true }
);
</script>

<template>
  <div class="modal-backdrop">
    <div class="modal-card modal-card-xlarge wizard-card">
      <!-- Modal Header -->
      <div class="wizard-header">
        <div style="display: flex; align-items: center; gap: 14px">
          <!-- App Icon -->
          <div class="app-icon-wrap">
            <img
              v-if="app.iconUrl"
              :src="app.iconUrl"
              :alt="app.title"
              class="app-icon-img"
              @error="($event.target as HTMLElement).style.display = 'none'"
            />
            <MdiIcon :path="mdiDocker" size="28" style="color: var(--accent-primary)" />
          </div>

          <div>
            <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap">
              <h2 style="font-size: 19px; font-weight: 700; color: var(--text-primary); margin: 0">
                {{ app.title || app.appId }}
              </h2>
              <span class="badge badge-info" style="font-size: 11px">
                {{ app.train || 'community' }}
              </span>
              <span v-if="app.appVersion || app.version" class="badge badge-neutral" style="font-size: 11px">
                v{{ app.appVersion || app.version }}
              </span>
              <span v-if="activeWebPort" class="badge badge-info" style="font-size: 11px">
                Portal Port: {{ activeWebPort }}
              </span>
            </div>
          </div>
        </div>

        <div style="display: flex; align-items: center; gap: 8px">
          <button
            class="btn btn-sm"
            :class="{ 'btn-primary': showLivePreview }"
            style="font-size: 12px; display: inline-flex; align-items: center; gap: 6px"
            title="Toggle YAML preview"
            @click="showLivePreview = !showLivePreview"
          >
            <MdiIcon :path="mdiCodeBraces" size="16" />
            <span>{{ showLivePreview ? 'Hide YAML Preview' : 'Show YAML Preview' }}</span>
          </button>
          <button class="btn btn-sm btn-icon" style="padding: 4px" @click="emit('close')">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>
      </div>

      <!-- App Description Banner -->
      <div v-if="app.description" class="wizard-desc-banner">
        <MdiIcon :path="mdiInformationOutline" size="16" style="color: var(--accent-primary); flex-shrink: 0; margin-top: 1px" />
        <span style="font-size: 12.5px; color: var(--text-secondary); line-height: 1.45">
          {{ app.description }}
        </span>
        <a
          v-if="app.home"
          :href="app.home"
          target="_blank"
          rel="noopener noreferrer"
          class="btn btn-sm"
          style="margin-left: auto; padding: 2px 8px; font-size: 11.5px; display: inline-flex; align-items: center; gap: 4px; flex-shrink: 0"
        >
          <span>Homepage</span>
          <MdiIcon :path="mdiOpenInNew" size="13" />
        </a>
      </div>

      <!-- Main Layout Body (Tabs + Split Preview) -->
      <div class="wizard-content-area">
        <!-- Left Side: Wizard Form Tabs & Fields -->
        <div class="wizard-form-pane">
          <!-- Top Horizontal Category Navigation Bar -->
          <div class="wizard-nav-tabs">
            <button
              v-for="grp in groups"
              :key="grp.name"
              class="nav-tab-btn"
              :class="{ active: activeTab === grp.name }"
              @click="activeTab = grp.name"
            >
              <MdiIcon :path="getGroupIcon(grp.name)" size="16" />
              <span>{{ grp.name }}</span>
              <span class="tab-badge">{{ grp.questions.length }}</span>
            </button>
          </div>

          <!-- Active Group Header Info -->
          <div class="group-info-bar">
            <div>
              <div style="font-size: 13.5px; font-weight: 600; color: var(--text-primary)">
                {{ activeTab }}
              </div>
              <div
                v-if="groups.find((g) => g.name === activeTab)?.description"
                style="font-size: 12px; color: var(--text-secondary); margin-top: 1px"
              >
                {{ groups.find((g) => g.name === activeTab)?.description }}
              </div>
            </div>
            <div style="font-size: 11.5px; color: var(--text-secondary)">
              {{ activeGroupQuestions.length }} {{ activeGroupQuestions.length === 1 ? 'parameter' : 'parameters' }}
            </div>
          </div>

          <!-- Questions Form List -->
          <div class="questions-scroll-list">
            <div
              v-for="q in activeGroupQuestions"
              :key="q.variable"
              class="question-card"
            >
              <!-- Question Label & Type Badges -->
              <div class="question-header">
                <div style="display: flex; align-items: center; gap: 6px">
                  <label class="question-label">
                    {{ q.label || q.variable }}
                  </label>
                  <span v-if="q.required" class="required-star" title="Required parameter">*</span>
                </div>
                <div style="display: flex; align-items: center; gap: 6px">
                  <span class="badge badge-neutral" style="font-size: 10px; text-transform: uppercase">
                    {{ q.type }}
                  </span>
                  <span v-if="q.mountPath" class="badge badge-success" style="font-size: 10px">
                    mount: {{ q.mountPath }}
                  </span>
                </div>
              </div>

              <!-- Question Description / Tooltip -->
              <p v-if="q.description" class="question-desc">
                {{ q.description }}
              </p>

              <!-- Control Input Switcher -->
              <div class="question-input-wrap">
                <!-- 1. INT / PORT -->
                <div v-if="q.type === 'int'" style="display: flex; align-items: center; gap: 10px">
                  <input
                    v-model="answers[q.variable]"
                    type="number"
                    class="input"
                    style="max-width: 200px"
                    :placeholder="q.defaultValue || '0'"
                  />
                  <span v-if="q.variable.toLowerCase().includes('port')" style="font-size: 12px; color: var(--text-secondary)">
                    Host Port → Container Port:
                    <strong style="color: var(--accent-primary)">{{ app.defaultPort || answers[q.variable] || q.defaultValue }}</strong>
                  </span>
                </div>

                <!-- 2. HOSTPATH / STORAGE -->
                <div v-else-if="q.type === 'hostpath'" style="display: flex; flex-direction: column; gap: 6px">
                  <div style="display: flex; gap: 8px; align-items: center">
                    <div style="flex: 1">
                      <PathSelector
                        v-model="answers[q.variable]"
                        :placeholder="`/mnt/${appsPool}/apps/${cleanAppId}/...`"
                        :root-hint="`/mnt/${appsPool}`"
                      />
                    </div>
                    <button
                      class="btn btn-sm"
                      style="padding: 6px 10px; font-size: 11.5px; white-space: nowrap"
                      title="Reset path to default dataset under /mnt/<pool>/apps/"
                      @click="resetStoragePath(q.variable)"
                    >
                      <MdiIcon :path="mdiRefresh" size="14" />
                      <span>Default</span>
                    </button>
                  </div>
                  <div style="font-size: 11.5px; color: var(--text-secondary); display: flex; align-items: center; gap: 6px">
                    <MdiIcon :path="mdiHarddisk" size="13" style="color: var(--accent-primary)" />
                    <span>Mounted in container at: <code>{{ q.mountPath || '/data' }}</code></span>
                  </div>
                </div>

                <!-- 3. BOOLEAN -->
                <div v-else-if="q.type === 'boolean'" style="display: flex; align-items: center; gap: 8px">
                  <label class="switch-wrap">
                    <input
                      type="checkbox"
                      :checked="answers[q.variable] === 'true' || answers[q.variable] === '1'"
                      @change="answers[q.variable] = ($event.target as HTMLInputElement).checked ? 'true' : 'false'"
                    />
                    <span class="switch-slider"></span>
                  </label>
                  <span style="font-size: 12.5px; color: var(--text-primary); font-weight: 500">
                    {{ (answers[q.variable] === 'true' || answers[q.variable] === '1') ? 'Enabled' : 'Disabled' }}
                  </span>
                </div>

                <!-- 4. SELECT / ENUM -->
                <div v-else-if="q.type === 'select' || (q.options && q.options.length > 0)">
                  <select v-model="answers[q.variable]" class="input" style="max-width: 320px">
                    <option v-for="opt in q.options" :key="opt" :value="opt">
                      {{ opt }}
                    </option>
                  </select>
                </div>

                <!-- 5. STRING / PASSWORD -->
                <div v-else>
                  <div v-if="isPasswordVariable(q)" style="display: flex; align-items: center; gap: 8px; max-width: 480px">
                    <div style="position: relative; flex: 1">
                      <input
                        v-model="answers[q.variable]"
                        :type="showPassword[q.variable] ? 'text' : 'password'"
                        class="input"
                        style="width: 100%; padding-right: 36px"
                        :placeholder="q.defaultValue || 'Enter password...'"
                      />
                      <button
                        type="button"
                        class="btn-icon"
                        style="position: absolute; right: 6px; top: 50%; transform: translateY(-50%); background: transparent; border: none; color: var(--text-secondary); cursor: pointer; padding: 4px"
                        :title="showPassword[q.variable] ? 'Hide password' : 'Show password'"
                        @click="toggleShowPassword(q.variable)"
                      >
                        <MdiIcon :path="showPassword[q.variable] ? mdiEyeOff : mdiEye" size="16" />
                      </button>
                    </div>
                    <button
                      type="button"
                      class="btn btn-sm"
                      style="padding: 6px 10px; font-size: 11.5px; display: inline-flex; align-items: center; gap: 4px; white-space: nowrap"
                      title="Generate secure 20-character password"
                      @click="generateRandomPassword(q.variable)"
                    >
                      <MdiIcon :path="mdiKeyVariant" size="14" />
                      <span>Generate</span>
                    </button>
                  </div>
                  <input
                    v-else
                    v-model="answers[q.variable]"
                    type="text"
                    class="input"
                    style="max-width: 480px"
                    :placeholder="q.defaultValue || 'Enter value...'"
                  />
                </div>
              </div>
            </div>

            <!-- Empty group state -->
            <div v-if="activeGroupQuestions.length === 0" class="empty-group-box">
              <MdiIcon :path="mdiTune" size="24" style="color: var(--text-secondary); margin-bottom: 6px" />
              <p style="font-size: 13px; color: var(--text-secondary); margin: 0">
                No parameters. Defaults will be used.
              </p>
            </div>
          </div>
        </div>

        <!-- Right Side: Live Docker Compose YAML Preview Split Pane -->
        <div v-if="showLivePreview" class="wizard-preview-pane">
          <div class="preview-header">
            <div style="display: flex; align-items: center; gap: 6px">
              <MdiIcon :path="mdiCodeBraces" size="16" style="color: var(--accent-primary)" />
              <span style="font-size: 12.5px; font-weight: 600; color: var(--text-primary)">
                Compose YAML
              </span>
            </div>
            <button
              class="btn btn-sm"
              style="padding: 3px 8px; font-size: 11.5px; display: inline-flex; align-items: center; gap: 4px"
              @click="copyYaml"
            >
              <MdiIcon :path="isCopied ? mdiCheck : mdiContentCopy" size="13" />
              <span>{{ isCopied ? 'Copied!' : 'Copy YAML' }}</span>
            </button>
          </div>

          <div class="preview-editor-body">
            <CodeMirrorEditor
              :model-value="compiledComposeYaml"
              :read-only="true"
              height="100%"
              min-height="300px"
            />
          </div>
        </div>
      </div>

      <!-- Modal Footer Action Bar -->
      <div class="wizard-footer">
        <button
          class="btn btn-sm"
          style="font-size: 12px; color: var(--text-secondary)"
          title="Bypass configuration wizard and load raw Compose template directly"
          @click="emit('skipWizard')"
        >
          Skip Wizard & Open Raw YAML
        </button>

        <div style="display: flex; align-items: center; gap: 10px">
          <button class="btn btn-sm" @click="emit('close')">
            Cancel
          </button>
          <button
            class="btn btn-sm"
            style="display: inline-flex; align-items: center; gap: 6px; font-size: 12.5px"
            title="Open generated stack in visual Compose Editor to customize services, environment, or labels"
            @click="handleOpenInEditor"
          >
            <MdiIcon :path="mdiPencil" size="15" />
            <span>Customize in Editor</span>
          </button>
          <button
            class="btn btn-sm btn-primary"
            :disabled="isDeploying"
            style="display: inline-flex; align-items: center; gap: 6px; font-size: 12.5px; font-weight: 600; padding: 6px 14px"
            @click="handleDeploy"
          >
            <MdiIcon
              :path="isDeploying ? mdiRefresh : mdiRocketLaunchOutline"
              size="16"
              :class="{ 'spin-icon': isDeploying }"
            />
            <span>{{ isDeploying ? 'Deploying Stack...' : 'Deploy App Now' }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.wizard-card {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  height: 92vh;
  max-height: 92vh;
}

.wizard-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

.app-icon-wrap {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  background: rgba(56, 189, 248, 0.1);
  border: 1px solid rgba(56, 189, 248, 0.2);
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  position: relative;
  flex-shrink: 0;
}

.app-icon-img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  position: absolute;
  top: 0;
  left: 0;
}

.wizard-desc-banner {
  background: rgba(15, 23, 42, 0.65);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 8px 12px;
  margin-top: 10px;
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.wizard-content-area {
  display: flex;
  flex: 1;
  min-height: 0;
  gap: 14px;
  margin-top: 12px;
  overflow: hidden;
}

.wizard-form-pane {
  flex: 1.2;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  background: rgba(0, 0, 0, 0.2);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  overflow: hidden;
}

.wizard-nav-tabs {
  display: flex;
  background: rgba(0, 0, 0, 0.35);
  border-bottom: 1px solid var(--border-subtle);
  overflow-x: auto;
  flex-shrink: 0;
}

.nav-tab-btn {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 10px 14px;
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  color: var(--text-secondary);
  font-size: 12.5px;
  font-weight: 500;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.15s ease;
}

.nav-tab-btn:hover {
  color: var(--text-primary);
  background: rgba(255, 255, 255, 0.03);
}

.nav-tab-btn.active {
  color: var(--accent-primary);
  border-bottom-color: var(--accent-primary);
  background: rgba(56, 189, 248, 0.06);
  font-weight: 600;
}

.tab-badge {
  background: rgba(255, 255, 255, 0.08);
  color: var(--text-secondary);
  font-size: 10.5px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: 10px;
}

.nav-tab-btn.active .tab-badge {
  background: rgba(56, 189, 248, 0.2);
  color: var(--accent-primary);
}

.group-info-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 14px;
  background: rgba(255, 255, 255, 0.015);
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

.questions-scroll-list {
  flex: 1;
  overflow-y: auto;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.question-card {
  background: rgba(255, 255, 255, 0.025);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 12px 14px;
  transition: border-color 0.15s ease;
}

.question-card:hover {
  border-color: rgba(255, 255, 255, 0.18);
}

.question-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
}

.question-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.required-star {
  color: var(--danger-text);
  font-weight: 700;
}

.question-desc {
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.4;
  margin: 0 0 10px 0;
}

.question-input-wrap {
  margin-top: 4px;
}

.empty-group-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  text-align: center;
}

/* Right Pane Preview */
.wizard-preview-pane {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  overflow: hidden;
}

.preview-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 14px;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

.preview-editor-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.wizard-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: 14px;
  border-top: 1px solid var(--border-subtle);
  margin-top: 12px;
  flex-shrink: 0;
}

/* Switch styling */
.switch-wrap {
  position: relative;
  display: inline-block;
  width: 36px;
  height: 20px;
}

.switch-wrap input {
  opacity: 0;
  width: 0;
  height: 0;
}

.switch-slider {
  position: absolute;
  cursor: pointer;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: var(--border-strong);
  transition: 0.2s;
  border-radius: 20px;
}

.switch-slider:before {
  position: absolute;
  content: "";
  height: 14px;
  width: 14px;
  left: 3px;
  bottom: 3px;
  background-color: white;
  transition: 0.2s;
  border-radius: 50%;
}

input:checked + .switch-slider {
  background-color: var(--accent-primary);
}

input:checked + .switch-slider:before {
  transform: translateX(16px);
}
</style>
