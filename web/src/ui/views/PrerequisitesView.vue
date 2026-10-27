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
import SortableTh from '../components/SortableTh.vue';
import { useTableSort } from '../composables/useTableSort';
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

const missingSort = useTableSort(missingList, {
  name: (p: Prerequisite) => p.displayName,
  category: (p: Prerequisite) => p.category,
  kind: (p: Prerequisite) => p.kind,
  target: (p: Prerequisite) => p.binaryOrPath,
  status: (p: Prerequisite) => p.status,
  requiredBy: (p: Prerequisite) => (p.requiredBy || []).length,
});

const installedSort = useTableSort(installedList, {
  name: (p: Prerequisite) => p.displayName,
  category: (p: Prerequisite) => p.category,
  kind: (p: Prerequisite) => p.kind,
  target: (p: Prerequisite) => p.binaryOrPath,
  requiredBy: (p: Prerequisite) => (p.requiredBy || []).length,
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
            <h2 class="card-title" style="font-size: 18px">Prerequisites</h2>
            <span
              v-if="linuxFlavor"
              class="badge badge-neutral"
              style="display: inline-flex; align-items: center; gap: 5px; font-weight: 600"
            >
              <MdiIcon :path="mdiLinux" size="14" />
              {{ linuxFlavor.prettyName || linuxFlavor.name }} ({{ linuxFlavor.packageManager }})
            </span>
          </div>
        </div>

        <div style="display: flex; align-items: center; gap: 10px; flex-wrap: wrap">
          <select v-model="categoryFilter" class="select">
            <option value="ALL">All Categories</option>
            <option value="STORAGE">Storage</option>
            <option value="VIRTUALIZATION">Virtualization</option>
            <option value="CONTAINERS">Containers</option>
            <option value="SHARING">Sharing</option>
            <option value="SECURITY">Security</option>
          </select>

          <button
            class="btn btn-primary"
            :disabled="checking"
            @click="loadReport(true)"
          >
            <MdiIcon :path="mdiRefresh" size="16" :class="{ 'spin-icon': checking }" />
            {{ checking ? 'Checking...' : 'Re-check' }}
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
          {{ isFullyCompliant ? 'All Installed' : `Action Required: ${missingCount} Missing` }}
        </span>

        <span class="badge badge-success">
          Installed: {{ installedCount }}
        </span>

        <span
          class="badge"
          :class="missingCount > 0 ? 'badge-warning' : 'badge-neutral'"
        >
          Missing: {{ missingCount }}
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
          <span>Missing</span>
          <span
            v-if="missingCount > 0"
            class="count-badge"
            style="background: var(--danger); color: #fff"
          >
            {{ missingCount }}
          </span>
          <span
            v-else
            class="count-badge"
            style="background: var(--success-bg); color: var(--success-text); border: 1px solid var(--success-border)"
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
          <span>Installed</span>
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
              <MdiIcon :path="mdiLinux" size="20" style="color: var(--accent-primary)" />
              <span style="font-weight: 700; font-size: 14px">
                Install Command
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
            Run as root (or with sudo) on the host to install all missing dependencies.
          </div>
        </div>

        <!-- Fully Compliant Banner when 0 missing -->
        <div v-if="missingList.length === 0" class="compliant-banner">
          <MdiIcon :path="mdiCheckCircle" size="36" style="color: var(--success-text)" />
          <div>
            <h3 style="margin: 0; font-size: 16px; color: var(--success-text)">
              All prerequisites are installed.
            </h3>
          </div>
        </div>

        <!-- Table of Missing Items -->
        <div v-else class="table-wrap" style="margin-top: 14px">
          <table class="data-table">
            <thead>
              <tr>
                <SortableTh :sort="missingSort" sort-key="name">Name</SortableTh>
                <SortableTh :sort="missingSort" sort-key="category">Category</SortableTh>
                <SortableTh :sort="missingSort" sort-key="kind">Kind</SortableTh>
                <SortableTh :sort="missingSort" sort-key="target">Target</SortableTh>
                <SortableTh :sort="missingSort" sort-key="status">Status</SortableTh>
                <SortableTh :sort="missingSort" sort-key="requiredBy">Required By</SortableTh>
                <th>Install</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in missingSort.sorted.value" :key="item.name">
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
                <td class="mono" style="font-size: 12.5px; color: var(--danger-text)">
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
                <SortableTh :sort="installedSort" sort-key="name">Name</SortableTh>
                <SortableTh :sort="installedSort" sort-key="category">Category</SortableTh>
                <SortableTh :sort="installedSort" sort-key="kind">Kind</SortableTh>
                <SortableTh :sort="installedSort" sort-key="target">Path / Target</SortableTh>
                <th>Status</th>
                <th>Version & Details</th>
                <SortableTh :sort="installedSort" sort-key="requiredBy">Required By</SortableTh>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in installedSort.sorted.value" :key="item.name">
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
                <td class="mono" style="font-size: 12.5px; color: var(--success-text)">
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
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-left: 4px solid var(--accent-primary);
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
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 12px 14px;
  overflow-x: auto;
}

.terminal-command code {
  color: var(--accent-primary);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
  line-height: 1.45;
  white-space: pre-wrap;
}

.compliant-banner {
  display: flex;
  align-items: center;
  gap: 16px;
  background: var(--success-bg);
  border: 1px solid var(--success-border);
  border-radius: 8px;
  padding: 18px 22px;
  margin-bottom: 14px;
}

.code-pill {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 11.5px;
  background: var(--warning-bg);
  color: var(--warning-text);
  padding: 4px 8px;
  border-radius: 4px;
  border: 1px solid var(--warning-border);
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
