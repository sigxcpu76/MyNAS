<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import {
  mdiAlertCircle,
  mdiCheck,
  mdiCheckCircle,
  mdiClose,
  mdiDatabase,
  mdiDatabaseCheck,
  mdiDownload,
  mdiRefresh,
} from '@mdi/js';
import { services } from '../../container';
import type { DiscoveredPool } from '../../domain/models';
import MdiIcon from './MdiIcon.vue';
import { confirmModal, nasState, notify, refreshAllData } from '../store';

const emit = defineEmits<{
  (e: 'close'): void;
}>();

const loading = ref(false);
const selecting = ref(false);
const manualPoolName = ref('');
const discoveredPools = ref<DiscoveredPool[]>([]);

const mainPool = computed(() => nasState.systemConfig?.mainPool || '');

const allPools = computed(() => {
  const map = new Map<string, DiscoveredPool>();
  for (const p of discoveredPools.value) {
    map.set(p.poolName, p);
  }
  for (const p of nasState.pools) {
    const name = p.poolName || p.name.replace(/^zfsPools\//, '');
    if (name && !map.has(name)) {
      map.set(name, {
        poolName: name,
        hasNasSystem: false,
        configVersion: '1.0',
        lastModified: '',
        isImported: true,
        datasetPath: `${name}/.nas-system`,
        mountpoint: `/${name}/.nas-system`,
      });
    }
  }
  return Array.from(map.values());
});

async function loadDiscoveredPools() {
  loading.value = true;
  try {
    const [list, zfsPools] = await Promise.all([
      services.system.discoverPools().catch(() => []),
      services.zfs.listPools().catch(() => []),
    ]);
    discoveredPools.value = list;
    if (zfsPools.length > 0) {
      nasState.pools = zfsPools;
    }
  } catch (err: any) {
    notify('error', err?.message || 'Failed to discover pools');
  } finally {
    loading.value = false;
  }
}

async function handleSelectPool(poolName: string) {
  const clean = poolName.trim();
  if (!clean) return;

  const current = mainPool.value;
  if (current && current !== clean) {
    const confirmed = await confirmModal({
      title: 'Switch Main Storage Pool',
      tone: 'warning',
      message: `Switch Main Storage Pool from "${current}" to "${clean}"?`,
      details: `System configuration, Docker appdata root, and LXC container root will now reside on "${clean}".`,
      confirmText: 'Switch Main Pool',
    });
    if (!confirmed) return;
  }

  selecting.value = true;
  try {
    const updated = await services.system.selectMainPool(clean, true);
    nasState.systemConfig = updated;
    notify('success', `Configured "${clean}" as Main Storage Pool. Modules enabled!`);
    await refreshAllData();
    emit('close');
  } catch (err: any) {
    notify('error', err?.message || 'Failed to select main pool');
  } finally {
    selecting.value = false;
  }
}

onMounted(() => {
  loadDiscoveredPools();
});
</script>

<template>
  <div class="modal-backdrop">
    <div class="modal-card">
      <div class="modal-header">
        <div style="display: flex; align-items: center; gap: 10px">
          <MdiIcon :path="mdiDatabase" size="22" style="color: var(--accent-primary, #3b82f6)" />
          <div>
            <h3 class="modal-title">Select Main Storage Pool</h3>
          </div>
        </div>
        <button class="btn btn-sm btn-icon" @click="emit('close')">
          <MdiIcon :path="mdiClose" size="18" />
        </button>
      </div>

      <div class="modal-body">
        <div class="info-banner">
          <MdiIcon :path="mdiAlertCircle" size="20" style="color: var(--accent-primary); flex-shrink: 0" />
          <div style="font-size: 13px; color: var(--text-secondary)">
            Until a main pool is selected, non-ZFS services (KVM, Docker, LXC, CIFS, WebDAV) remain paused.
            Selecting an imported pool that previously hosted NAS datasets (<code>.nas-system</code>) will
            <strong>automatically re-discover and restore</strong> all configuration.
          </div>
        </div>

        <div style="display: flex; justify-content: space-between; align-items: center; margin: 16px 0 10px 0">
          <span style="font-weight: 700; font-size: 13px; text-transform: uppercase; letter-spacing: 0.5px; color: var(--text-muted)">
            Available Storage Pools ({{ allPools.length }})
          </span>
          <button class="btn btn-sm btn-secondary" :disabled="loading" @click="loadDiscoveredPools">
            <MdiIcon :path="mdiRefresh" size="14" :class="{ 'spin-icon': loading }" />
            Scan Pools
          </button>
        </div>

        <div v-if="allPools.length === 0" style="text-align: center; padding: 32px 16px; background: rgba(0,0,0,0.2); border-radius: 8px">
          <MdiIcon :path="mdiDatabase" size="36" style="color: var(--text-muted); margin-bottom: 8px" />
          <div style="font-weight: 600; font-size: 14px">No ZFS pools found</div>
          <p style="font-size: 12.5px; color: var(--text-secondary); margin-top: 4px">
            Please create a new ZFS pool in the Storage &gt; Pools view first.
          </p>
        </div>

        <div v-else class="pool-list">
          <div
            v-for="p in allPools"
            :key="p.poolName"
            class="pool-item"
            :class="{ active: mainPool === p.poolName }"
          >
            <div style="display: flex; align-items: center; gap: 12px">
              <MdiIcon
                :path="mainPool === p.poolName ? mdiDatabaseCheck : mdiDatabase"
                size="24"
                :style="mainPool === p.poolName ? 'color: var(--accent-primary)' : 'color: var(--text-muted)'"
              />
              <div>
                <div style="display: flex; align-items: center; gap: 8px">
                  <span style="font-weight: 700; font-size: 15px">{{ p.poolName }}</span>
                  <span v-if="mainPool === p.poolName" class="badge badge-success" style="font-size: 11px">
                    Active Main Pool
                  </span>
                  <span v-else-if="p.hasNasSystem" class="badge badge-info" style="font-size: 11px">
                    Contains .nas-system
                  </span>
                  <span v-if="!p.isImported" class="badge badge-warning" style="font-size: 11px">
                    Importable Pool
                  </span>
                </div>
                <div class="mono" style="font-size: 11.5px; color: var(--text-secondary); margin-top: 2px">
                  Dataset: {{ p.datasetPath }} · Mountpoint: {{ p.mountpoint || (`/mnt/${p.poolName}/.nas-system`) }}
                </div>
              </div>
            </div>

            <div>
              <span
                v-if="mainPool === p.poolName"
                style="display: inline-flex; align-items: center; gap: 4px; font-size: 12px; color: var(--success-text); font-weight: 600; padding: 4px 8px"
              >
                <MdiIcon :path="mdiCheck" size="16" />
                Active
              </span>
              <button
                v-else
                class="btn btn-sm btn-primary"
                :disabled="selecting"
                style="display: inline-flex; align-items: center; gap: 6px"
                @click="handleSelectPool(p.poolName)"
              >
                <MdiIcon :path="p.hasNasSystem ? mdiDownload : mdiCheck" size="14" />
                {{ p.hasNasSystem ? 'Restore as Main Pool' : 'Select as Main Pool' }}
              </button>
            </div>
          </div>
        </div>

        <!-- Manual Pool Name Input -->
        <div style="margin-top: 16px; padding: 12px 14px; background: rgba(0,0,0,0.15); border: 1px dashed var(--border-subtle); border-radius: 8px">
          <div style="font-size: 12px; font-weight: 600; color: var(--text-secondary); margin-bottom: 8px">
            Or specify pool name directly:
          </div>
          <div style="display: flex; gap: 8px">
            <input
              v-model="manualPoolName"
              class="input mono"
              style="flex: 1"
              placeholder="e.g. mypool"
              @keydown.enter.prevent="handleSelectPool(manualPoolName)"
            />
            <button
              class="btn btn-sm btn-primary"
              :disabled="selecting || !manualPoolName.trim()"
              @click="handleSelectPool(manualPoolName)"
            >
              Select Pool
            </button>
          </div>
        </div>
      </div>

      <div class="modal-footer">
        <button class="btn btn-secondary" @click="emit('close')">
          Close
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1050;
  backdrop-filter: blur(4px);
  padding: 16px;
}

.modal-card {
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-subtle, #334155);
  border-radius: 12px;
  width: 100%;
  max-width: 650px;
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.modal-header {
  padding: 18px 20px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid var(--border-subtle, #334155);
}

.modal-title {
  margin: 0;
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
}

.modal-subtitle {
  margin: 2px 0 0 0;
  font-size: 12.5px;
  color: var(--text-secondary, #94a3b8);
}

.modal-body {
  padding: 20px;
  max-height: 70vh;
  overflow-y: auto;
}

.info-banner {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  background: rgba(59, 130, 246, 0.08);
  border: 1px solid rgba(59, 130, 246, 0.25);
  border-radius: 8px;
  padding: 14px 16px;
}

.pool-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.pool-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 16px;
  background: var(--bg-base, #0f172a);
  border: 1px solid var(--border-subtle, #334155);
  border-radius: 8px;
  transition: all 0.18s ease;
}

.pool-item:hover {
  border-color: var(--accent-primary, #3b82f6);
}

.pool-item.active {
  border-color: rgba(34, 197, 94, 0.5);
  background: rgba(34, 197, 94, 0.04);
}

.modal-footer {
  padding: 14px 20px;
  display: flex;
  justify-content: flex-end;
  border-top: 1px solid var(--border-subtle, #334155);
  background: rgba(15, 23, 42, 0.5);
}

.spin-icon {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
