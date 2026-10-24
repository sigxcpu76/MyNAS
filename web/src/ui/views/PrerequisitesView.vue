<script setup lang="ts">
import { computed, ref } from 'vue';
import { services } from '../../container';
import { nasState, notify } from '../store';

const checking = ref(false);
const categoryFilter = ref('ALL');

const filteredPrerequisites = computed(() => {
  if (categoryFilter.value === 'ALL') return nasState.prerequisites;
  return nasState.prerequisites.filter((p) => p.category === categoryFilter.value);
});

const installedCount = computed(
  () => nasState.prerequisites.filter((p) => p.status === 'INSTALLED').length
);
const fallbackCount = computed(
  () =>
    nasState.prerequisites.filter(
      (p) => p.status === 'FALLBACK_ACTIVE' || p.fallbackMode
    ).length
);

async function handleCheckPrerequisites() {
  checking.value = true;
  try {
    const summary = await services.prerequisites.checkPrerequisites();
    nasState.prerequisites = summary.prerequisites;
    notify(
      'success',
      `Re-verified ${summary.prerequisites.length} prerequisites (${summary.installedCount} native, ${summary.missingCount} fallback/missing)`
    );
  } catch (err: any) {
    notify('error', err?.message || 'Failed to check prerequisites');
  } finally {
    checking.value = false;
  }
}
</script>

<template>
  <div style="display: flex; flex-direction: column; gap: 18px">
    <div class="card">
      <div class="card-header">
        <div>
          <h2 class="card-title" style="font-size: 17px">
            External System Prerequisites & Kernel Module Verification
          </h2>
          <p class="card-subtitle">
            Verifies host binaries (`zpool`, `zfs`, `virsh`, `qemu-system-x86_64`, `docker`,
            `lxc-ls`, `smbd`, `sshd`, `websockify`), libraries (`libvirt`), and kernel modules
            (`zfs`, `kvm`).
          </p>
        </div>

        <div style="display: flex; align-items: center; gap: 10px">
          <select v-model="categoryFilter" class="select">
            <option value="ALL">All Categories ({{ nasState.prerequisites.length }})</option>
            <option value="STORAGE">STORAGE (ZFS)</option>
            <option value="VIRTUALIZATION">VIRTUALIZATION (KVM/noVNC)</option>
            <option value="CONTAINERS">CONTAINERS (Docker/LXC)</option>
            <option value="SHARING">SHARING (CIFS/WebDAV)</option>
            <option value="SECURITY">SECURITY (OpenSSH/PAM)</option>
          </select>

          <button
            class="btn btn-primary"
            :disabled="checking"
            @click="handleCheckPrerequisites"
          >
            {{ checking ? 'Probing System...' : 'Re-verify System Prerequisites' }}
          </button>
        </div>
      </div>

      <div style="display: flex; gap: 14px; margin-bottom: 16px; flex-wrap: wrap">
        <span class="badge badge-success">
          Native Installed: {{ installedCount }}
        </span>
        <span class="badge badge-warning">
          Hybrid Stateful Fallback Active: {{ fallbackCount }}
        </span>
        <span class="badge badge-info">
          Endpoint: POST /v1/prerequisites:check
        </span>
      </div>

      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>Resource Name / Component</th>
              <th>Category</th>
              <th>Kind</th>
              <th>Binary / Module Path</th>
              <th>Status</th>
              <th>Version & Details</th>
              <th>Required By</th>
              <th>Installation Hint</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in filteredPrerequisites" :key="item.name">
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
              <td class="mono" style="font-size: 12.5px; color: var(--accent-primary)">
                {{ item.binaryOrPath }}
              </td>
              <td>
                <span
                  class="badge"
                  :class="
                    item.status === 'INSTALLED'
                      ? 'badge-success'
                      : item.status === 'FALLBACK_ACTIVE'
                      ? 'badge-warning'
                      : 'badge-danger'
                  "
                >
                  {{ item.status }}
                </span>
              </td>
              <td>
                <div v-if="item.version" class="mono" style="font-size: 12px">
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
                    style="font-size: 10.5px"
                  >
                    {{ req }}
                  </span>
                </div>
              </td>
              <td>
                <code
                  class="mono"
                  style="
                    font-size: 11.5px;
                    background: var(--bg-base);
                    padding: 3px 7px;
                    border-radius: 4px;
                    border: 1px solid var(--border-subtle);
                    display: inline-block;
                  "
                >
                  {{ item.installHint }}
                </code>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
