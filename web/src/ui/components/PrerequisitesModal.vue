<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import {
  mdiAlertCircle,
  mdiCheck,
  mdiCheckCircle,
  mdiClose,
  mdiContentCopy,
  mdiLinux,
  mdiRefresh,
  mdiShieldAlert,
  mdiShieldCheck,
  mdiShieldRemove,
} from '@mdi/js';
import { services } from '../../container';
import type { Prerequisite, PrerequisiteSummary } from '../../domain/models';
import MdiIcon from './MdiIcon.vue';
import { nasState, notify } from '../store';

const emit = defineEmits<{
  (e: 'close'): void;
}>();

const checking = ref(false);
const copied = ref(false);
const categoryFilter = ref('ALL');
const activeTab = ref<'missing' | 'installed'>('missing');
const summary = ref<PrerequisiteSummary | null>(null);

const missingList = computed(() => {
  const items = summary.value?.missingPrerequisites ?? nasState.prerequisites.filter((p) => p.status !== 'INSTALLED');
  if (categoryFilter.value === 'ALL') return items;
  return items.filter((p) => p.category === categoryFilter.value);
});

const installedList = computed(() => {
  const items = summary.value?.installedPrerequisites ?? nasState.prerequisites.filter((p) => p.status === 'INSTALLED');
  if (categoryFilter.value === 'ALL') return items;
  return items.filter((p) => p.category === categoryFilter.value);
});

const installedCount = computed(() => {
  return summary.value?.installedCount ?? nasState.prerequisites.filter((p) => p.status === 'INSTALLED').length;
});

const missingCount = computed(() => {
  return summary.value?.missingCount ?? nasState.prerequisites.filter((p) => p.status !== 'INSTALLED').length;
});

const isStorageMissing = computed(() => {
  const allMissing = summary.value?.missingPrerequisites ?? nasState.prerequisites.filter((p) => p.status !== 'INSTALLED');
  return allMissing.some((p) => p.category === 'STORAGE' || p.name.includes('zfs') || p.name.includes('zpool'));
});

const statusLevel = computed<'green' | 'amber' | 'red'>(() => {
  if (nasState.prerequisites.length === 0 && !summary.value) return 'amber';
  if (missingCount.value === 0 && installedCount.value > 0) return 'green';
  if (isStorageMissing.value || installedCount.value === 0) return 'red';
  return 'amber';
});

const linuxFlavor = computed(() => summary.value?.linuxFlavor);

// Auto-switch to installed tab if 0 missing
onMounted(() => {
  if (missingCount.value === 0 && installedCount.value > 0) {
    activeTab.value = 'installed';
  }
  loadReport(false);
});

async function loadReport(showToast = false) {
  checking.value = true;
  try {
    const res = await services.prerequisites.checkPrerequisites();
    summary.value = res;
    nasState.prerequisites = res.prerequisites;
    if (res.missingCount === 0) {
      activeTab.value = 'installed';
    } else {
      activeTab.value = 'missing';
    }
    if (showToast) {
      notify(
        'success',
        `Re-checked ${res.prerequisites.length} prerequisites (${res.installedCount} installed, ${res.missingCount} missing)`
      );
    }
  } catch (err: any) {
    if (showToast) {
      notify('error', err?.message || 'Failed to check prerequisites');
    }
  } finally {
    checking.value = false;
  }
}

async function copyRemediation() {
  const script = summary.value?.remediationScript || generateFallbackScript();
  if (!script) return;
  try {
    await navigator.clipboard.writeText(script);
    copied.value = true;
    notify('info', 'Fix instructions copied to clipboard');
    setTimeout(() => {
      copied.value = false;
    }, 2500);
  } catch {
    notify('error', 'Failed to copy command to clipboard');
  }
}

function generateFallbackScript(): string {
  const allMissing = summary.value?.missingPrerequisites ?? nasState.prerequisites.filter((p) => p.status !== 'INSTALLED');
  const packages = new Set<string>();
  for (const p of allMissing) {
    if (p.installHint) {
      const match = p.installHint.match(/(?:install|add)\s+([a-zA-Z0-9_\- ]+)/i);
      if (match && match[1]) {
        match[1].trim().split(/\s+/).forEach((pkg) => packages.add(pkg));
      }
    }
  }
  if (packages.size > 0) {
    return `sudo apt-get update && sudo apt-get install -y ${Array.from(packages).join(' ')}`;
  }
  return '';
}
</script>

<template>
  <div class="modal-backdrop" @click.self="emit('close')">
    <div class="modal-card modal-card-xlarge">
      <!-- Modal Header -->
      <div class="modal-header" style="border-bottom: 1px solid var(--border-subtle); padding-bottom: 14px; margin-bottom: 16px">
        <div style="display: flex; align-items: center; gap: 12px">
          <div
            class="status-icon-wrapper"
            :class="`status-${statusLevel}`"
          >
            <MdiIcon
              :path="statusLevel === 'green' ? mdiShieldCheck : statusLevel === 'amber' ? mdiShieldAlert : mdiShieldRemove"
              size="26"
            />
          </div>
          <div>
            <div style="display: flex; align-items: center; gap: 8px">
              <h3 class="modal-title" style="margin: 0; font-size: 18px">System Prerequisites & Host Compliance</h3>
              <span
                class="badge"
                :class="statusLevel === 'green' ? 'badge-success' : statusLevel === 'amber' ? 'badge-warning' : 'badge-danger'"
              >
                {{ statusLevel === 'green' ? 'Compliant' : statusLevel === 'amber' ? `${missingCount} Missing` : 'Critical Missing' }}
              </span>
            </div>
            <p class="modal-subtitle" style="margin: 3px 0 0 0">
              Underlying Linux packages, virtualization hypervisors, and storage daemons required for NAS operations
            </p>
          </div>
        </div>

        <button class="btn btn-sm btn-icon" @click="emit('close')">
          <MdiIcon :path="mdiClose" size="18" />
        </button>
      </div>

      <div class="modal-scroll-body">
        <!-- Linux Distro & Overall Health Bar -->
        <div
          class="prereq-banner"
          :class="`banner-${statusLevel}`"
        >
          <div style="display: flex; align-items: center; gap: 10px">
            <MdiIcon :path="mdiLinux" size="24" style="flex-shrink: 0" />
            <div>
              <strong style="font-size: 13.5px">
                {{ linuxFlavor?.prettyName || 'Linux Host Platform' }}
              </strong>
              <div style="font-size: 12px; opacity: 0.85; margin-top: 2px">
                {{ installedCount }} of {{ installedCount + missingCount }} prerequisites verified and operational.
              </div>
            </div>
          </div>

          <div style="display: flex; align-items: center; gap: 8px">
            <button
              class="btn btn-sm btn-secondary"
              :disabled="checking"
              @click="loadReport(true)"
            >
              <MdiIcon :path="mdiRefresh" size="14" :class="{ 'spin-icon': checking }" style="margin-right: 4px" />
              {{ checking ? 'Re-checking...' : 'Re-check Prerequisites' }}
            </button>
          </div>
        </div>

        <!-- Fix Instructions Box (shown when any prerequisites are missing) -->
        <div v-if="missingCount > 0" class="remediation-box">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
            <div style="display: flex; align-items: center; gap: 8px">
              <MdiIcon :path="mdiAlertCircle" size="18" style="color: var(--warning-text)" />
              <strong style="font-size: 13.5px; color: var(--text-primary)">
                Instructions to Fix Missing Prerequisites
              </strong>
            </div>
            <button
              v-if="summary?.remediationScript || generateFallbackScript()"
              class="btn btn-sm btn-primary"
              @click="copyRemediation"
            >
              <MdiIcon :path="copied ? mdiCheck : mdiContentCopy" size="14" style="margin-right: 4px" />
              {{ copied ? 'Copied Command!' : 'Copy Fix Command' }}
            </button>
          </div>

          <p style="font-size: 12.5px; color: var(--text-secondary); margin: 0 0 10px 0">
            Open an SSH session or root terminal on this host and execute the package installation command below.
            Once complete, click <strong>Re-check Prerequisites</strong> to verify.
          </p>

          <div class="code-terminal mono">
            <code>{{ summary?.remediationScript || generateFallbackScript() || '# Detecting package commands...' }}</code>
          </div>
        </div>

        <div v-else class="card" style="padding: 16px; margin-bottom: 16px; background: var(--success-bg); border-color: var(--success-border)">
          <div style="display: flex; align-items: center; gap: 10px">
            <MdiIcon :path="mdiCheckCircle" size="22" style="color: var(--success-text); flex-shrink: 0" />
            <div>
              <strong style="color: var(--success-text)">All Host Prerequisites Are Installed</strong>
              <div style="font-size: 12px; color: var(--text-secondary); margin-top: 2px">
                Storage, virtualization, containers, and network sharing utilities are ready.
              </div>
            </div>
          </div>
        </div>

        <!-- Filter & Tab Switcher -->
        <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 10px; margin-bottom: 12px">
          <div style="display: flex; gap: 6px">
            <button
              class="btn btn-sm"
              :class="activeTab === 'missing' ? 'btn-primary' : 'btn-secondary'"
              @click="activeTab = 'missing'"
            >
              Missing / Degraded ({{ missingCount }})
            </button>
            <button
              class="btn btn-sm"
              :class="activeTab === 'installed' ? 'btn-primary' : 'btn-secondary'"
              @click="activeTab = 'installed'"
            >
              Installed ({{ installedCount }})
            </button>
          </div>

          <div style="display: flex; align-items: center; gap: 8px">
            <span style="font-size: 12px; color: var(--text-secondary)">Category:</span>
            <select
              v-model="categoryFilter"
              class="form-control"
              style="width: 160px; padding: 4px 8px; font-size: 12px"
            >
              <option value="ALL">All Categories</option>
              <option value="STORAGE">Storage (ZFS)</option>
              <option value="VIRTUALIZATION">Virtualization (KVM)</option>
              <option value="CONTAINERS">Containers (Docker/LXC)</option>
              <option value="SHARING">File Sharing (CIFS/WebDAV)</option>
              <option value="SECURITY">Security & PAM</option>
            </select>
          </div>
        </div>

        <!-- Prerequisites Table -->
        <div class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th style="width: 25%">Component</th>
                <th style="width: 14%">Category</th>
                <th style="width: 12%">Kind</th>
                <th style="width: 14%">Status</th>
                <th style="width: 35%">Installation Hint / Fix Command</th>
              </tr>
            </thead>
            <tbody>
              <template v-if="activeTab === 'missing'">
                <tr v-if="missingList.length === 0">
                  <td colspan="5" style="text-align: center; padding: 24px; color: var(--text-secondary)">
                    No missing prerequisites in this category.
                  </td>
                </tr>
                <tr v-for="item in missingList" :key="item.name">
                  <td>
                    <div style="font-weight: 600">{{ item.displayName }}</div>
                    <div class="mono" style="font-size: 11px; color: var(--text-secondary)">
                      {{ item.binaryOrPath }}
                    </div>
                  </td>
                  <td>
                    <span class="badge badge-neutral">{{ item.category }}</span>
                  </td>
                  <td>
                    <span class="mono" style="font-size: 11.5px">{{ item.kind }}</span>
                  </td>
                  <td>
                    <span
                      class="badge"
                      :class="item.status === 'FALLBACK_ACTIVE' ? 'badge-warning' : 'badge-danger'"
                    >
                      {{ item.status }}
                    </span>
                  </td>
                  <td>
                    <div class="mono" style="font-size: 11.5px; color: var(--accent-primary); word-break: break-all">
                      {{ item.installHint || 'Check Linux package manager' }}
                    </div>
                    <div v-if="item.requiredBy && item.requiredBy.length > 0" style="font-size: 11px; color: var(--text-muted); margin-top: 2px">
                      Required by: {{ item.requiredBy.join(', ') }}
                    </div>
                  </td>
                </tr>
              </template>

              <template v-else>
                <tr v-if="installedList.length === 0">
                  <td colspan="5" style="text-align: center; padding: 24px; color: var(--text-secondary)">
                    No installed prerequisites in this category.
                  </td>
                </tr>
                <tr v-for="item in installedList" :key="item.name">
                  <td>
                    <div style="font-weight: 600">{{ item.displayName }}</div>
                    <div class="mono" style="font-size: 11px; color: var(--text-secondary)">
                      {{ item.binaryOrPath }}
                    </div>
                  </td>
                  <td>
                    <span class="badge badge-neutral">{{ item.category }}</span>
                  </td>
                  <td>
                    <span class="mono" style="font-size: 11.5px">{{ item.kind }}</span>
                  </td>
                  <td>
                    <span class="badge badge-success">INSTALLED</span>
                  </td>
                  <td>
                    <span class="mono" style="font-size: 11.5px; color: var(--text-muted)">
                      {{ item.version ? `v${item.version}` : 'Verified on host' }}
                    </span>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Modal Footer -->
      <div class="modal-footer" style="display: flex; justify-content: space-between; align-items: center; border-top: 1px solid var(--border-subtle); padding-top: 14px; margin-top: 14px">
        <span style="font-size: 12px; color: var(--text-secondary)">
          Verified by NaN Host Checker Engine
        </span>
        <div style="display: flex; gap: 8px">
          <button class="btn btn-secondary" @click="emit('close')">
            Close
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.status-icon-wrapper {
  width: 42px;
  height: 42px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.status-green {
  background: var(--success-bg);
  color: var(--success-text);
  border: 1px solid var(--success-border);
}

.status-amber {
  background: var(--warning-bg);
  color: var(--warning-text);
  border: 1px solid var(--warning-border);
}

.status-red {
  background: var(--danger-bg);
  color: var(--danger-text);
  border: 1px solid var(--danger-border);
}

.prereq-banner {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  border-radius: 8px;
  margin-bottom: 14px;
}

.banner-green {
  background: var(--success-bg);
  border: 1px solid var(--success-border);
  color: var(--text-primary);
}

.banner-amber {
  background: var(--warning-bg);
  border: 1px solid var(--warning-border);
  color: var(--text-primary);
}

.banner-red {
  background: var(--danger-bg);
  border: 1px solid var(--danger-border);
  color: var(--text-primary);
}

.remediation-box {
  background: var(--bg-base);
  border: 1px solid var(--warning-border);
  border-radius: 8px;
  padding: 14px 16px;
  margin-bottom: 16px;
}

.code-terminal {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 10px 14px;
  overflow-x: auto;
  font-size: 12.5px;
  color: var(--accent-primary);
}

.spin-icon {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
