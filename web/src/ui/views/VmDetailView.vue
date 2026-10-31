<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import {
  mdiAlertCircle,
  mdiArrowLeft,
  mdiCamera,
  mdiCheckCircle,
  mdiClose,
  mdiCpu64Bit,
  mdiDelete,
  mdiDisc,
  mdiHarddisk,
  mdiInformationOutline,
  mdiMemory,
  mdiMonitor,
  mdiNetwork,
  mdiOpenInNew,
  mdiPlay,
  mdiPlus,
  mdiPower,
  mdiRefresh,
  mdiRestart,
  mdiRestore,
  mdiStop,
  mdiTrashCanOutline,
} from '@mdi/js';
import MdiIcon from '../components/MdiIcon.vue';
import VncConsole from '../components/VncConsole.vue';
import { services } from '../../container';
import {
  formatBytes,
  type VirtualMachine,
  type VirtualMachineConsoleSession,
  type VirtualMachineSnapshot,
} from '../../domain/models';
import {
  nasState,
  notify,
  refreshAllData,
  runTrackedTask,
} from '../store';
import { confirmDelete } from '../modal';

const props = withDefaults(
  defineProps<{
    vmName: string;
    activeSubTab?: 'summary' | 'hardware' | 'console' | 'snapshots';
  }>(),
  {
    activeSubTab: 'summary',
  }
);

const emit = defineEmits<{
  (e: 'back'): void;
}>();

const isBusy = ref(false);

const cleanVmName = computed(() => {
  return props.vmName.replace(/^virtualMachines\//, '').trim();
});

const vm = computed<VirtualMachine | null>(() => {
  const target = cleanVmName.value;
  return (
    nasState.vms.find(
      (v) => v.name === target || v.name === `virtualMachines/${target}` || v.vmName === target
    ) || null
  );
});

// Live VNC Console Session
const consoleSession = ref<VirtualMachineConsoleSession | null>(null);
const isLoadingConsole = ref(false);
const consoleError = ref('');

async function loadConsoleSession() {
  if (!vm.value) return;
  isLoadingConsole.value = true;
  consoleError.value = '';
  try {
    const s = await services.kvm.openConsole(cleanVmName.value);
    consoleSession.value = s;
  } catch (err: any) {
    consoleError.value = err?.message || 'Failed to open QEMU live console';
    notify('error', consoleError.value);
  } finally {
    isLoadingConsole.value = false;
  }
}

watch(
  () => props.activeSubTab,
  (tab) => {
    if (tab === 'console' && !consoleSession.value) {
      loadConsoleSession();
    }
  },
  { immediate: true }
);

async function popoutConsole() {
  const clean = cleanVmName.value;
  try {
    let s = consoleSession.value;
    if (!s) {
      s = await services.kvm.openConsole(clean);
      consoleSession.value = s;
    }
    const tokenParam = s?.token ? `&token=${encodeURIComponent(s.token)}` : '';
    const portParam = s?.vncPort ? `&port=${s.vncPort}` : '';
    const url = `/vnc-console?vm=${encodeURIComponent(clean)}${tokenParam}${portParam}`;
    window.open(
      url,
      `vnc_${clean}`,
      'width=1024,height=768,location=no,toolbar=no,menubar=no,status=no,resizable=yes'
    );
  } catch {
    const url = `/vnc-console?vm=${encodeURIComponent(clean)}`;
    window.open(
      url,
      `vnc_${clean}`,
      'width=1024,height=768,location=no,toolbar=no,menubar=no,status=no,resizable=yes'
    );
  }
}

// -----------------------------------------------------------------------------
// VM Power Controls
// -----------------------------------------------------------------------------
async function handleStart() {
  if (!vm.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Start`, async () => {
      await services.kvm.startVirtualMachine(vm.value!.name);
    });
    notify('success', `Virtual machine "${cleanVmName.value}" started`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Start failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleShutdown() {
  if (!vm.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": ACPI Shutdown`, async () => {
      await services.kvm.stopVirtualMachine(vm.value!.name, false);
    });
    notify('info', `Shutdown signal sent to "${cleanVmName.value}"`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Shutdown failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleForceStop() {
  if (!vm.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Force Stop`, async () => {
      await services.kvm.stopVirtualMachine(vm.value!.name, true);
    });
    notify('info', `Virtual machine "${cleanVmName.value}" terminated`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Stop failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleReboot() {
  if (!vm.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Reboot`, async () => {
      await services.kvm.rebootVirtualMachine(vm.value!.name, false);
    });
    notify('info', `Reboot signal sent to "${cleanVmName.value}"`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Reboot failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleDeleteVm() {
  if (!vm.value || isBusy.value) return;
  const confirmed = await confirmDelete({
    kind: 'Virtual Machine',
    name: cleanVmName.value,
    details: 'The virtual machine definition and guest configuration will be removed.',
    confirmText: 'Delete VM',
  });
  if (!confirmed.confirmed) return;

  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Delete`, async () => {
      await services.kvm.deleteVirtualMachine(vm.value!.name);
    });
    notify('warning', `Virtual machine "${cleanVmName.value}" deleted`);
    emit('back');
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Delete failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

// -----------------------------------------------------------------------------
// Snapshots Management
// -----------------------------------------------------------------------------
const snapshotsList = ref<VirtualMachineSnapshot[]>([]);
const isLoadingSnapshots = ref(false);
const showSnapshotModal = ref(false);
const snapshotForm = reactive({
  id: '',
  description: '',
  includeRam: false,
});

async function loadSnapshots() {
  if (!vm.value) return;
  isLoadingSnapshots.value = true;
  try {
    snapshotsList.value = await services.kvm.listSnapshots(vm.value.name);
  } catch {
    snapshotsList.value = [];
  } finally {
    isLoadingSnapshots.value = false;
  }
}

watch(() => props.activeSubTab, (tab) => {
  if (tab === 'snapshots') {
    loadSnapshots();
  }
});

async function submitSnapshot() {
  if (!snapshotForm.id.trim() || !vm.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Create Snapshot "${snapshotForm.id}"`, async () => {
      await services.kvm.createSnapshot(
        vm.value!.name,
        snapshotForm.id.trim(),
        snapshotForm.description.trim(),
        snapshotForm.includeRam
      );
    });
    notify('success', `Snapshot "${snapshotForm.id}" created`);
    showSnapshotModal.value = false;
    snapshotForm.id = '';
    snapshotForm.description = '';
    await loadSnapshots();
  } catch (err: any) {
    notify('error', `Failed to create snapshot: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleRollbackSnapshot(snap: VirtualMachineSnapshot) {
  if (!vm.value || isBusy.value) return;
  const confirmed = await confirmDelete({
    kind: 'Snapshot Rollback',
    name: snap.snapshotId,
    details: 'Current disk state will be reverted to this snapshot point.',
    confirmText: 'Rollback Snapshot',
  });
  if (!confirmed.confirmed) return;

  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Rollback Snapshot "${snap.snapshotId}"`, async () => {
      await services.kvm.rollbackSnapshot(snap.name);
    });
    notify('success', `Reverted VM to snapshot "${snap.snapshotId}"`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Rollback failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleDeleteSnapshot(snap: VirtualMachineSnapshot) {
  if (!vm.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Delete Snapshot "${snap.snapshotId}"`, async () => {
      await services.kvm.deleteSnapshot(snap.name);
    });
    notify('info', `Snapshot "${snap.snapshotId}" deleted`);
    await loadSnapshots();
  } catch (err: any) {
    notify('error', `Failed to delete snapshot: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}
</script>

<template>
  <div class="vm-detail-view">
    <!-- Subtab 1: Summary -->
    <div v-if="props.activeSubTab === 'summary'" class="vm-tab-content">
      <div class="grid-2">
        <!-- Live Real-Time Telemetry Card -->
        <div class="card detail-card">
          <div class="card-header">
            <h3 class="card-title">Real-Time Guest Telemetry</h3>
          </div>
          <div class="card-body">
            <div class="metrics-grid">
              <div class="metric-item">
                <span class="metric-label">Power State</span>
                <span
                  class="badge mono font-xs"
                  :class="vm?.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'"
                  style="align-self: flex-start"
                >
                  {{ vm?.state || 'STOPPED' }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Real CPU Usage</span>
                <span class="metric-val mono">
                  {{ vm?.state === 'RUNNING' && vm?.guestCpuPercent !== undefined ? vm.guestCpuPercent.toFixed(1) + '%' : '-' }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Real Memory Used (RSS)</span>
                <span class="metric-val mono">
                  {{ vm?.state === 'RUNNING' && vm?.guestMemoryUsedMib ? (vm.guestMemoryUsedMib >= 1024 ? (vm.guestMemoryUsedMib / 1024).toFixed(2) + ' GiB' : vm.guestMemoryUsedMib + ' MiB') : '-' }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Guest Agent</span>
                <span class="metric-val mono" :style="{ color: vm?.agentRunning ? 'var(--success-text)' : 'var(--text-muted)' }">
                  {{ vm?.agentRunning ? 'Active & Responding' : 'Not installed / Inactive' }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Guest OS</span>
                <span class="metric-val">{{ vm?.guestOsName || 'Linux (QEMU/KVM)' }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Autostart on Boot</span>
                <span class="metric-val mono">{{ vm?.autostart ? 'Enabled' : 'Disabled' }}</span>
              </div>
            </div>

            <!-- Guest Network IP Addresses -->
            <div style="margin-top: 18px">
              <h4 class="font-sm" style="font-weight: 600; margin-bottom: 6px">Assigned Network IPs</h4>
              <div v-if="!vm?.guestIpAddresses || vm.guestIpAddresses.length === 0" class="text-muted font-sm">
                No active IP addresses reported by QEMU guest agent.
              </div>
              <div v-else style="display: flex; flex-wrap: wrap; gap: 6px">
                <span v-for="ip in vm.guestIpAddresses" :key="ip" class="badge badge-info mono font-xs">
                  {{ ip }}
                </span>
              </div>
            </div>
          </div>
        </div>

        <!-- Disks & Attached Storage Summary Card -->
        <div class="card detail-card">
          <div class="card-header">
            <h3 class="card-title">Virtual Disks & Storage</h3>
          </div>
          <div class="card-body">
            <div v-if="!vm?.disks || vm.disks.length === 0" class="empty-state">
              No virtual hard disks attached.
            </div>
            <table v-else class="data-table">
              <thead>
                <tr>
                  <th>Bus</th>
                  <th>Path / Device</th>
                  <th>Size</th>
                  <th>Cache</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(d, idx) in vm.disks" :key="idx">
                  <td class="mono font-xs"><span class="badge badge-neutral mono">{{ d.bus }}</span></td>
                  <td class="mono font-sm" style="font-weight: 600">{{ d.diskPath }}</td>
                  <td class="mono font-sm">{{ Math.round((d.sizeBytes || 0) / (1024 * 1024 * 1024)) }} GiB</td>
                  <td class="mono font-xs text-muted">{{ d.cacheMode || 'none' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    <!-- Subtab 2: Hardware -->
    <div v-else-if="props.activeSubTab === 'hardware'" class="vm-tab-content">
      <div class="card">
        <div class="card-header">
          <h3 class="card-title">Hardware Inventory</h3>
          <p class="card-subtitle">Attached hardware components, network interfaces and controller buses</p>
        </div>
        <div class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th style="width: 140px">Component</th>
                <th>Configuration & Details</th>
                <th style="width: 120px">Bus / Interface</th>
              </tr>
            </thead>
            <tbody>
              <!-- Disks -->
              <tr v-for="(d, idx) in vm?.disks || []" :key="`disk-${idx}`">
                <td style="font-weight: 600">
                  <div style="display: flex; align-items: center; gap: 6px">
                    <MdiIcon :path="mdiHarddisk" size="16" style="color: var(--warning-text)" />
                    <span>Hard Disk ({{ idx }})</span>
                  </div>
                </td>
                <td class="mono font-sm">
                  {{ d.diskPath }} ({{ Math.round((d.sizeBytes || 0) / (1024 * 1024 * 1024)) }} GiB, cache: {{ d.cacheMode || 'none' }}{{ d.discard ? ', discard: on' : '' }})
                </td>
                <td><span class="badge badge-neutral mono font-xs">{{ d.bus }}</span></td>
              </tr>

              <!-- CD-ROM / ISO -->
              <tr v-for="(cd, idx) in vm?.cdroms || []" :key="`cd-${idx}`">
                <td style="font-weight: 600">
                  <div style="display: flex; align-items: center; gap: 6px">
                    <MdiIcon :path="mdiDisc" size="16" style="color: var(--accent-secondary)" />
                    <span>CD-ROM Drive ({{ idx }})</span>
                  </div>
                </td>
                <td class="mono font-sm">
                  {{ cd.isoPath || 'empty (no media)' }}
                </td>
                <td><span class="badge badge-neutral mono font-xs">{{ cd.bus || 'ide' }}</span></td>
              </tr>

              <!-- Network Interfaces -->
              <tr v-for="(nic, idx) in vm?.networkInterfaces || []" :key="`nic-${idx}`">
                <td style="font-weight: 600">
                  <div style="display: flex; align-items: center; gap: 6px">
                    <MdiIcon :path="mdiNetwork" size="16" style="color: var(--accent-primary)" />
                    <span>Network (net{{ idx }})</span>
                  </div>
                </td>
                <td class="mono font-sm">
                  mac={{ nic.macAddress }}, bridge={{ nic.bridge }}
                </td>
                <td><span class="badge badge-neutral mono font-xs">{{ nic.model || 'virtio' }}</span></td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Subtab 3: Integrated Console -->
    <div v-else-if="props.activeSubTab === 'console'" class="vm-tab-content">
      <div v-if="isLoadingConsole" class="card" style="padding: 48px; text-align: center">
        <div class="spinner" style="width: 28px; height: 28px; border-width: 3px; margin: 0 auto"></div>
        <p style="margin-top: 12px; font-size: 13px; color: var(--text-secondary)">Connecting to live QEMU/noVNC console...</p>
      </div>
      <div v-else-if="consoleError" class="card" style="padding: 24px">
        <h3 class="card-title" style="color: var(--danger-text)">Console Connection Failed</h3>
        <p class="font-sm text-secondary" style="margin-top: 8px">{{ consoleError }}</p>
        <div style="margin-top: 14px">
          <button class="btn btn-sm btn-primary" @click="loadConsoleSession">Retry Connection</button>
        </div>
      </div>
      <div v-else-if="consoleSession" class="card embedded-vnc-card">
        <div class="vnc-toolbar">
          <span class="font-sm mono" style="font-weight: 600">{{ cleanVmName }} Console (Port: {{ consoleSession.vncPort }})</span>
          <button class="btn btn-xs btn-primary" @click="popoutConsole">
            <MdiIcon :path="mdiOpenInNew" size="13" style="margin-right: 4px" />
            Pop-out Window
          </button>
        </div>
        <div class="vnc-body-wrap">
          <VncConsole
            :vm-name="consoleSession.vmName"
            :token="consoleSession.token"
            :vnc-port="consoleSession.vncPort"
            :websocket-url="consoleSession.websocketUrl"
            :standalone="false"
            @detach="popoutConsole"
          />
        </div>
      </div>
    </div>

    <!-- Subtab 4: Snapshots -->
    <div v-else-if="props.activeSubTab === 'snapshots'" class="vm-tab-content">
      <div class="card">
        <div class="card-header">
          <div>
            <h3 class="card-title">Virtual Machine Snapshots</h3>
            <p class="card-subtitle">Point-in-time state captures and RAM memory dumps</p>
          </div>
          <button class="btn btn-sm btn-primary" @click="showSnapshotModal = true">
            <MdiIcon :path="mdiCamera" size="14" style="margin-right: 4px" />
            Take Snapshot
          </button>
        </div>
        <div class="table-wrap">
          <div v-if="snapshotsList.length === 0" class="empty-state">
            No snapshots recorded for {{ cleanVmName }}.
          </div>
          <table v-else class="data-table">
            <thead>
              <tr>
                <th>Snapshot Name</th>
                <th>Description</th>
                <th>Created At</th>
                <th>RAM State</th>
                <th style="width: 140px; text-align: center">Action</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="snap in snapshotsList" :key="snap.name">
                <td style="font-weight: 600">
                  <div style="display: flex; align-items: center; gap: 6px">
                    <MdiIcon :path="mdiCamera" size="14" style="color: var(--accent-primary)" />
                    <span class="mono">{{ snap.snapshotId }}</span>
                  </div>
                </td>
                <td class="text-secondary font-sm">{{ snap.description || '-' }}</td>
                <td class="mono font-sm text-muted">{{ snap.createTime || '-' }}</td>
                <td>
                  <span class="badge font-xs" :class="snap.includeRam ? 'badge-success' : 'badge-neutral'">
                    {{ snap.includeRam ? 'Memory Dump' : 'Disk Only' }}
                  </span>
                </td>
                <td style="text-align: center">
                  <div style="display: flex; gap: 6px; justify-content: center">
                    <button class="btn btn-xs btn-secondary" title="Rollback" @click="handleRollbackSnapshot(snap)">
                      Rollback
                    </button>
                    <button class="btn btn-xs btn-ghost" title="Delete" @click="handleDeleteSnapshot(snap)">
                      <MdiIcon :path="mdiDelete" size="13" style="color: var(--danger-text)" />
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Take Snapshot Modal -->
    <div v-if="showSnapshotModal" class="modal-overlay" @click.self="showSnapshotModal = false">
      <div class="modal-container" style="max-width: 500px">
        <div class="modal-header">
          <h3 class="modal-title">Take Snapshot: {{ cleanVmName }}</h3>
          <button class="btn btn-xs btn-ghost" @click="showSnapshotModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <div class="modal-body" style="display: flex; flex-direction: column; gap: 14px; padding: 16px 20px">
          <div class="form-group">
            <label class="form-label">Snapshot Name</label>
            <input
              v-model="snapshotForm.id"
              type="text"
              class="form-input"
              placeholder="e.g. pre-upgrade, backup-20261031"
            />
          </div>
          <div class="form-group">
            <label class="form-label">Description (Optional)</label>
            <textarea
              v-model="snapshotForm.description"
              class="form-input"
              rows="2"
              placeholder="Reason for snapshot..."
            ></textarea>
          </div>
          <div class="form-group" style="display: flex; align-items: center; gap: 8px">
            <input id="snap-ram-cb" v-model="snapshotForm.includeRam" type="checkbox" />
            <label for="snap-ram-cb" style="margin: 0; font-size: 13px; cursor: pointer">Include RAM memory state (Live Snapshot)</label>
          </div>
        </div>
        <div class="modal-footer" style="padding: 12px 20px; display: flex; justify-content: flex-end; gap: 8px">
          <button class="btn btn-secondary btn-sm" @click="showSnapshotModal = false">Cancel</button>
          <button class="btn btn-primary btn-sm" :disabled="!snapshotForm.id.trim() || isBusy" @click="submitSnapshot">
            Create Snapshot
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.vm-detail-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
}

.vm-header-card {
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.vm-header-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 14px;
}

.vm-title {
  margin: 0;
  font-size: 18px;
  font-weight: 700;
  color: var(--text-primary);
}

.vm-metrics-line {
  margin-top: 2px;
}

.vm-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.vm-subtabs {
  display: flex;
  align-items: center;
  gap: 4px;
  border-top: 1px solid var(--border-subtle);
  padding-top: 12px;
  margin-top: 4px;
  overflow-x: auto;
}

.subtab-item {
  background: transparent;
  border: 1px solid transparent;
  color: var(--text-secondary);
  font-size: 12.5px;
  font-weight: 500;
  padding: 5px 12px;
  border-radius: 4px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.subtab-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.subtab-item.active {
  background: var(--accent-bg-subtle);
  color: var(--accent-primary);
  border-color: var(--accent-primary);
  font-weight: 600;
}

.vm-tab-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.grid-2 {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 16px;
}

.detail-card {
  padding: 18px 20px;
}

.metrics-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

.metric-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.metric-label {
  font-size: 11px;
  color: var(--text-muted);
}

.metric-val {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.embedded-vnc-card {
  padding: 0;
  overflow: hidden;
  height: 600px;
  display: flex;
  flex-direction: column;
}

.vnc-toolbar {
  height: 38px;
  background: var(--sidebar-bg);
  border-bottom: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 14px;
}

.vnc-body-wrap {
  flex: 1;
  min-height: 0;
}

.empty-state {
  padding: 32px;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
}

.form-input {
  background: var(--input-bg);
  border: 1px solid var(--border-subtle);
  color: var(--text-primary);
  border-radius: 4px;
  padding: 6px 10px;
  font-size: 13px;
  outline: none;
}

.font-sm {
  font-size: 12px;
}

.font-xs {
  font-size: 10.5px;
}
</style>
