<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import {
  mdiAlertCircle,
  mdiCheck,
  mdiCheckCircle,
  mdiContentCopy,
  mdiLinux,
  mdiRefresh,
} from '@mdi/js';
import { services } from '../../container';
import type { Prerequisite, PrerequisiteSummary } from '../../domain/models';
import MdiIcon from '../components/MdiIcon.vue';
import { nasState, notify } from '../store';

const checking = ref(false);
const categoryFilter = ref('ALL');
const activeTab = ref<'missing' | 'installed'>('missing');
const copied = ref(false);
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

const isFullyCompliant = computed(() => {
  if (summary.value && summary.value.isFullyCompliant !== undefined) {
    return summary.value.isFullyCompliant;
  }
  return missingCount.value === 0 && installedCount.value > 0;
});

const linuxFlavor = computed(() => summary.value?.linuxFlavor);

async function loadReport(showToast = false) {
  checking.value = true;
  try {
    const res = await services.prerequisites.checkPrerequisites();
    summary.value = res;
    nasState.prerequisites = res.prerequisites;
    if (showToast) {
      notify(
        'success',
        `Re-verified ${res.prerequisites.length} prerequisites (${res.installedCount} installed, ${res.missingCount} missing)`
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
  if (!summary.value?.remediationScript) return;
  try {
    await navigator.clipboard.writeText(summary.value.remediationScript);
    copied.value = true;
    notify('info', 'Remediation command copied to clipboard');
    setTimeout(() => {
      copied.value = false;
    }, 2500);
  } catch {
    notify('error', 'Failed to copy command to clipboard');
  }
}

onMounted(() => {
  loadReport(false);
});
</script>

<template>
  <div style="display: flex; flex-direction: column; gap: 18px">
    <!-- Top Configuration & Actions Card -->
    <div class="card">
      <div class="card-header" style="flex-wrap: wrap; gap: 14px">
        <div>
          <div style="display: flex; align-items: center; gap: 8px">
            <h2 class="card-title" style="font-size: 18px">
              Host System Prerequisites & Linux Compliance
            </h2>
            <span
              v-if="linuxFlavor"
              class="badge badge-neutral"
              style="display: inline-flex; align-items: center; gap: 5px; font-weight: 600"
            >
              <MdiIcon :path="mdiLinux" size="14" />
              {{ linuxFlavor.prettyName || linuxFlavor.name }} ({{ linuxFlavor.packageManager }})
            </span>
          </div>
          <p class="card-subtitle">
            Detects host distribution, package manager, virtualization kernel modules, hypervisors, and storage tools.
          </p>
        </div>

        <div style="display: flex; align-items: center; gap: 10px; flex-wrap: wrap">
          <select v-model="categoryFilter" class="select">
            <option value="ALL">All Categories</option>
            <option value="STORAGE">STORAGE (ZFS, S.M.A.R.T, NVMe)</option>
            <option value="VIRTUALIZATION">VIRTUALIZATION (KVM, QEMU, libvirt, OVMF, vTPM)</option>
            <option value="CONTAINERS">CONTAINERS (Docker, LXC)</option>
            <option value="SHARING">SHARING (CIFS/Samba, WebDAV)</option>
            <option value="SECURITY">SECURITY (OpenSSH)</option>
          </select>

          <button
            class="btn btn-primary"
            :disabled="checking"
            @click="loadReport(true)"
          >
            <MdiIcon :path="mdiRefresh" size="16" :class="{ 'spin-icon': checking }" />
            {{ checking ? 'Probing System...' : 'Re-verify Prerequisites' }}
          </button>
        </div>
      </div>

      <!-- Compliance Metrics Bar -->
      <div style="display: flex; gap: 12px; margin-bottom: 18px; flex-wrap: wrap; align-items: center">
        <span
          class="badge"
          :class="isFullyCompliant ? 'badge-success' : 'badge-danger'"
          style="font-size: 13px; font-weight: 700; padding: 4px 10px; display: inline-flex; align-items: center; gap: 6px"
        >
          <MdiIcon :path="isFullyCompliant ? mdiCheckCircle : mdiAlertCircle" size="16" />
          {{ isFullyCompliant ? '100% Fully Compliant' : `Action Required: ${missingCount} Missing` }}
        </span>

        <span class="badge badge-success">
          Installed & Verified: {{ installedCount }}
        </span>

        <span
          class="badge"
          :class="missingCount > 0 ? 'badge-warning' : 'badge-neutral'"
        >
          Missing / Fallback: {{ missingCount }}
        </span>

        <span v-if="summary?.hybridFallbackEnabled" class="badge badge-info">
          Hybrid Stateful Sandbox Fallback Active
        </span>
      </div>

      <!-- Navigation Tabs: Split Report into Missing (First Page) and Installed (Second Page) -->
      <div class="tabs-bar">
        <button
          class="tab-item"
          :class="{ active: activeTab === 'missing' }"
          @click="activeTab = 'missing'"
        >
          <MdiIcon :path="mdiAlertCircle" size="16" />
          <span>Page 1: Missing & Remediation</span>
          <span
            v-if="missingCount > 0"
            class="count-badge"
            style="background: #ef4444; color: #fff"
          >
            {{ missingCount }}
          </span>
          <span
            v-else
            class="count-badge"
            style="background: rgba(34, 197, 94, 0.2); color: #22c55e"
          >
            0
          </span>
        </button>

        <button
          class="tab-item"
          :class="{ active: activeTab === 'installed' }"
          @click="activeTab = 'installed'"
        >
          <MdiIcon :path="mdiCheckCircle" size="16" />
          <span>Page 2: Installed & Compliant</span>
          <span class="count-badge">
            {{ installedCount }}
          </span>
        </button>
      </div>

      <!-- PAGE 1: MISSING & REMEDIATION (First Page) -->
      <div v-if="activeTab === 'missing'" style="padding-top: 18px">
        <!-- If Missing Count > 0: Remediation Script Box -->
        <div v-if="summary?.remediationScript" class="remediation-box">
          <div class="remediation-header">
            <div style="display: flex; align-items: center; gap: 8px">
              <MdiIcon :path="mdiLinux" size="20" style="color: #60a5fa" />
              <span style="font-weight: 700; font-size: 14px">
                Compliance Remediation Command for {{ linuxFlavor?.prettyName || 'Host Distribution' }}
              </span>
            </div>
            <button
              class="btn btn-sm btn-secondary"
              style="display: inline-flex; align-items: center; gap: 6px"
              @click="copyRemediation"
            >
              <MdiIcon :path="copied ? mdiCheck : mdiContentCopy" size="15" />
              {{ copied ? 'Copied to Clipboard!' : 'Copy Command' }}
            </button>
          </div>
          <div class="terminal-command">
            <code>{{ summary.remediationScript }}</code>
          </div>
          <div style="margin-top: 8px; font-size: 12px; color: var(--text-secondary)">
            Run this command as root (or with sudo) on your host to install all missing dependencies and enable compliance.
          </div>
        </div>

        <!-- Fully Compliant Banner when 0 missing -->
        <div v-if="missingList.length === 0" class="compliant-banner">
          <MdiIcon :path="mdiCheckCircle" size="36" style="color: #22c55e" />
          <div>
            <h3 style="margin: 0 0 4px 0; font-size: 16px; color: #22c55e">
              System Is 100% Compliant!
            </h3>
            <p style="margin: 0; font-size: 13px; color: var(--text-secondary)">
              All prerequisite packages, kernel modules, libraries, and binaries are present on
              <strong style="color: var(--text-primary)">{{ linuxFlavor?.prettyName || 'the host' }}</strong>.
              All NAS virtualization, container, storage, and sharing capabilities are ready.
            </p>
          </div>
        </div>

        <!-- Table of Missing Items -->
        <div v-else class="table-wrap" style="margin-top: 14px">
          <table class="data-table">
            <thead>
              <tr>
                <th>Component / Prerequisite</th>
                <th>Category</th>
                <th>Kind</th>
                <th>Target Identifier</th>
                <th>Status</th>
                <th>Required By Features</th>
                <th>Exact Distro Installation Instruction</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in missingList" :key="item.name">
                <td>
                  <div style="font-weight: 600">{{ item.displayName }}</div>
                  <div class="mono" style="font-size: 11px; color: var(--text-muted)">
                    {{ item.name }}
                  </div>
                </td>
                <td>
                  <span class="badge badge-neutral">{{ item.category }}</span>
                </td>
                <td class="mono" style="font-size: 12px">{{ item.kind }}</td>
                <td class="mono" style="font-size: 12.5px; color: #f87171">
                  {{ item.binaryOrPath }}
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
                  <div style="display: flex; flex-wrap: wrap; gap: 4px">
                    <span
                      v-for="req in item.requiredBy"
                      :key="req"
                      class="badge badge-neutral"
                      style="font-size: 11px"
                    >
                      {{ req }}
                    </span>
                  </div>
                </td>
                <td>
                  <code class="code-pill">
                    {{ item.installHint }}
                  </code>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- PAGE 2: INSTALLED & COMPLIANT (Second Page) -->
      <div v-if="activeTab === 'installed'" style="padding-top: 18px">
        <div class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>Component / Prerequisite</th>
                <th>Category</th>
                <th>Kind</th>
                <th>Resolved Path / Target</th>
                <th>Status</th>
                <th>Version & Details</th>
                <th>Required By Features</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in installedList" :key="item.name">
                <td>
                  <div style="font-weight: 600">{{ item.displayName }}</div>
                  <div class="mono" style="font-size: 11px; color: var(--text-muted)">
                    {{ item.name }}
                  </div>
                </td>
                <td>
                  <span class="badge badge-neutral">{{ item.category }}</span>
                </td>
                <td class="mono" style="font-size: 12px">{{ item.kind }}</td>
                <td class="mono" style="font-size: 12.5px; color: #22c55e">
                  {{ item.binaryOrPath }}
                </td>
                <td>
                  <span class="badge badge-success">
                    INSTALLED
                  </span>
                </td>
                <td>
                  <div v-if="item.version" class="mono" style="font-size: 12px; font-weight: 600">
                    {{ item.version }}
                  </div>
                  <div style="font-size: 12px; color: var(--text-secondary)">
                    {{ item.details }}
                  </div>
                </td>
                <td>
                  <div style="display: flex; flex-wrap: wrap; gap: 4px">
                    <span
                      v-for="req in item.requiredBy"
                      :key="req"
                      class="badge badge-neutral"
                      style="font-size: 11px"
                    >
                      {{ req }}
                    </span>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tabs-bar {
  display: flex;
  gap: 8px;
  border-bottom: 2px solid var(--border-subtle);
  padding: 0 4px;
}

.tab-item {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  margin-bottom: -2px;
  padding: 10px 18px;
  color: var(--text-secondary);
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.18s ease;
}

.tab-item:hover {
  color: var(--text-primary);
  background: rgba(255, 255, 255, 0.03);
}

.tab-item.active {
  color: var(--accent-primary, #3b82f6);
  border-bottom-color: var(--accent-primary, #3b82f6);
}

.count-badge {
  font-size: 11px;
  font-weight: 700;
  padding: 2px 7px;
  border-radius: 9999px;
  background: var(--bg-surface-elevated, rgba(255, 255, 255, 0.1));
  color: var(--text-primary);
}

.remediation-box {
  background: #0f172a;
  border: 1px solid #1e293b;
  border-left: 4px solid #3b82f6;
  border-radius: 8px;
  padding: 14px 18px;
  margin-bottom: 16px;
}

.remediation-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}

.terminal-command {
  background: #020617;
  border: 1px solid #1e293b;
  border-radius: 6px;
  padding: 12px 14px;
  overflow-x: auto;
}

.terminal-command code {
  color: #38bdf8;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
  line-height: 1.45;
  white-space: pre-wrap;
}

.compliant-banner {
  display: flex;
  align-items: center;
  gap: 16px;
  background: rgba(34, 197, 94, 0.08);
  border: 1px solid rgba(34, 197, 94, 0.25);
  border-radius: 8px;
  padding: 18px 22px;
  margin-bottom: 14px;
}

.code-pill {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 11.5px;
  background: var(--bg-base, #0b1120);
  color: #fbbf24;
  padding: 4px 8px;
  border-radius: 4px;
  border: 1px solid var(--border-subtle);
  display: inline-block;
  white-space: pre-wrap;
}

.spin-icon {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
</style>
