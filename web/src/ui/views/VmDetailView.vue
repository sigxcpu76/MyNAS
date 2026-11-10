<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import {
  mdiCamera,
  mdiClose,
  mdiDisc,
  mdiHarddisk,
  mdiNetwork,
  mdiOpenInNew,
  mdiPencil,
  mdiPlus,
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
import { useNetworkBridges } from '../utils/networkBridges';

const { defaultBridge, bridgeOptionsFor } = useNetworkBridges();

const props = withDefaults(
  defineProps<{
    vmName: string;
    activeSubTab?: 'summary' | 'console' | 'configuration' | 'hardware' | 'snapshots';
  }>(),
  {
    activeSubTab: 'summary',
  }
);

const emit = defineEmits<{
  (e: 'back'): void;
  (e: 'update:activeSubTab', tab: 'summary' | 'console' | 'configuration' | 'hardware' | 'snapshots'): void;
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

function popoutConsole() {
  const clean = cleanVmName.value;
  const tokenParam = consoleSession.value?.token ? `&token=${encodeURIComponent(consoleSession.value.token)}` : '';
  const portParam = consoleSession.value?.vncPort ? `&port=${consoleSession.value.vncPort}` : '';
  const url = `/vnc-console?vm=${encodeURIComponent(clean)}${tokenParam}${portParam}`;
  window.open(
    url,
    `vnc_${clean}`,
    'width=1024,height=768,location=no,toolbar=no,menubar=no,status=no,resizable=yes'
  );
}

// -----------------------------------------------------------------------------
// Autostart Toggle
// -----------------------------------------------------------------------------
const isTogglingAutostart = ref(false);

async function toggleAutostart() {
  if (!vm.value || isTogglingAutostart.value) return;
  isTogglingAutostart.value = true;
  try {
    const updated = await services.kvm.updateVirtualMachine(
      vm.value.name,
      { autostart: !vm.value.autostart },
      ['autostart']
    );
    vm.value.autostart = updated.autostart;
    hwForm.autostart = updated.autostart;
    notify('success', `Autostart ${updated.autostart ? 'enabled' : 'disabled'} for ${cleanVmName.value}`);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to toggle autostart');
  } finally {
    isTogglingAutostart.value = false;
  }
}

// -----------------------------------------------------------------------------
// VM Hardware & Cloud-Init Configuration
// -----------------------------------------------------------------------------
const isSavingConfig = ref(false);
const hwForm = reactive({
  vmName: '',
  description: '',
  autostart: false,
  sockets: 1,
  cores: 2,
  cpuType: 'host',
  numa: false,
  memoryMib: 4096,
  hugepages: 'off',
  machineType: 'i440fx',
  biosType: 'seabios',
  tpmVersion: 'none',
  secureBoot: false,
  bootOrder: 'hd,cdrom',
  displayType: 'virtio-gpu',
});

const isSavingCloudInit = ref(false);
const ciForm = reactive({
  user: '',
  password: '',
  ipConfig: '',
  sshPublicKeys: '',
});

watch(
  vm,
  (v, oldV) => {
    if (v && v.name !== oldV?.name) {
      hwForm.vmName = v.vmName || cleanVmName.value;
      hwForm.description = v.description || '';
      hwForm.autostart = Boolean(v.autostart);
      hwForm.sockets = v.sockets || 1;
      hwForm.cores = v.cores || v.vcpus || 2;
      hwForm.cpuType = v.cpuType || 'host';
      hwForm.numa = Boolean(v.numa);
      hwForm.memoryMib = v.memoryMib || 2048;
      hwForm.hugepages = v.hugepages || 'off';
      let mach = v.machineType || 'i440fx';
      if (mach === 'pc' || mach.startsWith('pc-i440fx')) mach = 'i440fx';
      else if (mach.startsWith('pc-q35')) mach = 'q35';
      hwForm.machineType = mach;
      hwForm.biosType = v.biosType || 'seabios';
      hwForm.tpmVersion = v.tpmVersion || 'none';
      hwForm.secureBoot = Boolean(v.secureBoot);
      hwForm.bootOrder = v.bootOrder || 'hd,cdrom';
      hwForm.displayType = v.displayType || 'virtio-gpu';

      ciForm.user = v.cloudInit?.user || '';
      ciForm.password = v.cloudInit?.password || '';
      ciForm.ipConfig = v.cloudInit?.ipConfig || '';
      ciForm.sshPublicKeys = (v.cloudInit?.sshPublicKeys || []).join('\n');
    }
  },
  { immediate: true }
);

async function handleSaveHardwareConfig() {
  if (!vm.value || isSavingConfig.value) return;
  isSavingConfig.value = true;
  const vcpus = Math.max(1, Number(hwForm.sockets) || 1) * Math.max(1, Number(hwForm.cores) || 1);
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Save Configuration`, async () => {
      await services.kvm.updateVirtualMachine(
        vm.value!.name,
        {
          vmName: hwForm.vmName.trim() || cleanVmName.value,
          description: hwForm.description.trim(),
          autostart: hwForm.autostart,
          vcpus,
          sockets: hwForm.sockets,
          cores: hwForm.cores,
          cpuType: hwForm.cpuType,
          numa: hwForm.numa,
          memoryMib: hwForm.memoryMib,
          balloonMinMib: hwForm.memoryMib,
          hugepages: hwForm.hugepages,
          machineType: hwForm.machineType,
          biosType: hwForm.biosType,
          tpmVersion: hwForm.tpmVersion,
          secureBoot: hwForm.secureBoot,
          bootOrder: hwForm.bootOrder,
          displayType: hwForm.displayType,
        },
        [
          'vm_name',
          'description',
          'autostart',
          'vcpus',
          'sockets',
          'cores',
          'cpu_type',
          'numa',
          'memory_mib',
          'balloon_min_mib',
          'hugepages',
          'machine_type',
          'bios_type',
          'tpm_version',
          'secure_boot',
          'boot_order',
          'display_type',
        ]
      );
    });
    notify('success', `Saved configuration for "${cleanVmName.value}"`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to save configuration: ${err.message}`);
  } finally {
    isSavingConfig.value = false;
  }
}

async function handleSaveCloudInit() {
  if (!vm.value || isSavingCloudInit.value) return;
  isSavingCloudInit.value = true;
  const sshKeys = ciForm.sshPublicKeys
    .split('\n')
    .map((k) => k.trim())
    .filter(Boolean);
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Save Cloud-Init`, async () => {
      await services.kvm.updateVirtualMachine(
        vm.value!.name,
        {
          cloudInit: {
            user: ciForm.user.trim(),
            password: ciForm.password,
            ipConfig: ciForm.ipConfig.trim(),
            sshPublicKeys: sshKeys,
          },
        },
        ['cloud_init']
      );
    });
    notify('success', `Saved Cloud-Init settings for "${cleanVmName.value}"`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to save Cloud-Init: ${err.message}`);
  } finally {
    isSavingCloudInit.value = false;
  }
}

// Add Hardware Modals (Disk / NIC / CD-ROM)
const showAddDiskModal = ref(false);
const newDisk = reactive({
  bus: 'scsi',
  diskPath: '',
  sizeGib: 32,
  cacheMode: 'none',
  discard: true,
});

function openAddDisk() {
  const idx = (vm.value?.disks || []).length;
  const pool = (nasState.systemConfig?.mainPool || nasState.pools[0]?.poolName || 'tank').replace(/^zfsPools\//, '');
  newDisk.bus = 'scsi';
  newDisk.diskPath = `/dev/zvol/${pool}/vm-${cleanVmName.value}-disk-${idx}`;
  newDisk.sizeGib = 32;
  newDisk.cacheMode = 'none';
  newDisk.discard = true;
  showAddDiskModal.value = true;
}

async function submitAddDisk() {
  if (!vm.value || !newDisk.diskPath.trim() || isBusy.value) return;
  isBusy.value = true;
  const existing = vm.value.disks || [];
  const nextId = `sd${String.fromCharCode(97 + existing.length)}`;
  const updated = [
    ...existing,
    {
      id: nextId,
      bus: newDisk.bus,
      diskPath: newDisk.diskPath.trim(),
      sizeBytes: newDisk.sizeGib * 1024 * 1024 * 1024,
      cacheMode: newDisk.cacheMode,
      discard: newDisk.discard,
      iothread: true,
      backup: true,
    },
  ];
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Attach Disk`, async () => {
      await services.kvm.updateVirtualMachine(vm.value!.name, { disks: updated }, ['disks']);
    });
    notify('success', `Attached virtual disk (${newDisk.sizeGib} GiB)`);
    showAddDiskModal.value = false;
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to attach disk: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleRemoveDisk(idx: number) {
  if (!vm.value || isBusy.value) return;
  const target = (vm.value.disks || [])[idx];
  if (!target) return;
  const confirmed = await confirmDelete({
    kind: 'Virtual Disk Attachment',
    name: target.diskPath,
    details: 'The disk mapping will be detached from this virtual machine.',
    confirmText: 'Detach Disk',
  });
  if (!confirmed.confirmed) return;
  isBusy.value = true;
  const updated = (vm.value.disks || []).filter((_, i) => i !== idx);
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Detach Disk`, async () => {
      await services.kvm.updateVirtualMachine(vm.value!.name, { disks: updated }, ['disks']);
    });
    notify('info', `Detached disk ${target.diskPath}`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to detach disk: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

const showEditDiskModal = ref(false);
const editingDiskIndex = ref(-1);
const editDiskForm = reactive({
  bus: 'scsi',
  diskPath: '',
  sizeGib: 32,
  cacheMode: 'none',
  discard: true,
});

function openEditDisk(idx: number) {
  const disk = (vm.value?.disks || [])[idx];
  if (!disk) return;
  editingDiskIndex.value = idx;
  editDiskForm.bus = disk.bus || 'scsi';
  editDiskForm.diskPath = disk.diskPath || '';
  editDiskForm.sizeGib = Math.round((disk.sizeBytes || 0) / (1024 * 1024 * 1024)) || 32;
  editDiskForm.cacheMode = disk.cacheMode || 'none';
  editDiskForm.discard = disk.discard ?? true;
  showEditDiskModal.value = true;
}

async function submitEditDisk() {
  if (!vm.value || editingDiskIndex.value < 0 || isBusy.value) return;
  isBusy.value = true;
  const disks = [...(vm.value.disks || [])];
  disks[editingDiskIndex.value] = {
    ...disks[editingDiskIndex.value],
    bus: editDiskForm.bus,
    cacheMode: editDiskForm.cacheMode,
    discard: editDiskForm.discard,
  };
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Edit Disk`, async () => {
      await services.kvm.updateVirtualMachine(vm.value!.name, { disks }, ['disks']);
    });
    notify('success', 'Updated disk configuration');
    showEditDiskModal.value = false;
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to update disk: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

const showAddNicModal = ref(false);
const newNic = reactive({
  model: 'virtio',
  bridge: defaultBridge.value || 'br0',
  vlanTag: 0,
});
const addNicForm = newNic;

function openAddNic() {
  newNic.model = 'virtio';
  newNic.bridge = defaultBridge.value || 'br0';
  newNic.vlanTag = 0;
  showAddNicModal.value = true;
}

const showEditNicModal = ref(false);
const editingNicIndex = ref(-1);
const editNicForm = reactive({
  model: 'virtio',
  bridge: defaultBridge.value || 'br0',
  vlanTag: 0,
});

function openEditNic(idx: number) {
  const nic = (vm.value?.networkInterfaces || [])[idx];
  if (!nic) return;
  editingNicIndex.value = idx;
  editNicForm.model = nic.model || 'virtio';
  editNicForm.bridge = nic.bridge || defaultBridge.value || 'br0';
  editNicForm.vlanTag = nic.vlanTag || 0;
  showEditNicModal.value = true;
}

async function submitEditNic() {
  if (!vm.value || editingNicIndex.value < 0 || isBusy.value) return;
  isBusy.value = true;
  const nics = [...(vm.value.networkInterfaces || [])];
  nics[editingNicIndex.value] = {
    ...nics[editingNicIndex.value],
    model: editNicForm.model,
    bridge: editNicForm.bridge.trim() || 'br0',
    vlanTag: editNicForm.vlanTag > 0 ? editNicForm.vlanTag : undefined,
  };
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Edit NIC`, async () => {
      await services.kvm.updateVirtualMachine(vm.value!.name, { networkInterfaces: nics }, ['network_interfaces']);
    });
    notify('success', 'Updated network interface');
    showEditNicModal.value = false;
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to update network interface: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

// CD-ROM Media (Insert / Swap / Eject)
const showMediaModal = ref(false);
const activeMediaDeviceId = ref('');
const selectedMediaIso = ref('');

function openMediaDialog(deviceId: string, currentIso?: string) {
  activeMediaDeviceId.value = deviceId;
  selectedMediaIso.value = currentIso || nasState.isos[0]?.path || '';
  showMediaModal.value = true;
}

async function handleAttachMedia() {
  if (!vm.value || !selectedMediaIso.value.trim() || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Insert ISO`, async () => {
      await services.kvm.attachMedia(vm.value!.name, selectedMediaIso.value.trim(), activeMediaDeviceId.value);
    });
    notify('success', 'ISO media mounted');
    showMediaModal.value = false;
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to mount media: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleEjectMedia(deviceId: string) {
  if (!vm.value || isBusy.value) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Eject ISO`, async () => {
      await services.kvm.ejectMedia(vm.value!.name, deviceId);
    });
    notify('success', 'ISO media ejected');
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to eject media: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function submitAddNic() {
  if (!vm.value || isBusy.value) return;
  isBusy.value = true;
  const existing = vm.value.networkInterfaces || [];
  const updated = [
    ...existing,
    {
      id: `net${existing.length}`,
      model: newNic.model,
      bridge: newNic.bridge.trim() || 'br0',
      vlanTag: newNic.vlanTag > 0 ? newNic.vlanTag : undefined,
    },
  ];
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Add NIC`, async () => {
      await services.kvm.updateVirtualMachine(vm.value!.name, { networkInterfaces: updated }, ['network_interfaces']);
    });
    notify('success', `Added network interface net${existing.length}`);
    showAddNicModal.value = false;
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to add network interface: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function handleRemoveNic(idx: number) {
  if (!vm.value || isBusy.value) return;
  const updated = (vm.value.networkInterfaces || []).filter((_, i) => i !== idx);
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${cleanVmName.value}": Remove NIC`, async () => {
      await services.kvm.updateVirtualMachine(vm.value!.name, { networkInterfaces: updated }, ['network_interfaces']);
    });
    notify('info', 'Removed network interface');
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to remove network interface: ${err.message}`);
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
  <div class="vm-detail-view" :class="{ 'is-console-tab': props.activeSubTab === 'console' }">
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
                  class="badge font-xs"
                  :class="vm?.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'"
                  style="align-self: flex-start"
                >
                  {{ vm?.state || 'STOPPED' }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Real CPU Usage</span>
                <span class="metric-val">
                  {{ vm?.state === 'RUNNING' && vm?.guestCpuPercent !== undefined ? vm.guestCpuPercent.toFixed(1) + '%' : '-' }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Real Memory Used (RSS)</span>
                <span class="metric-val">
                  {{ vm?.state === 'RUNNING' && vm?.guestMemoryUsedMib ? (vm.guestMemoryUsedMib >= 1024 ? (vm.guestMemoryUsedMib / 1024).toFixed(2) + ' GiB' : vm.guestMemoryUsedMib + ' MiB') : '-' }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Guest Agent</span>
                <span class="metric-val" :style="{ color: vm?.agentRunning ? 'var(--success-text)' : 'var(--text-muted)' }">
                  {{ vm?.agentRunning ? 'Active & Responding' : 'Not installed / Inactive' }}
                </span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Guest OS</span>
                <span class="metric-val">{{ vm?.guestOsName || 'Linux (QEMU/KVM)' }}</span>
              </div>
              <div class="metric-item">
                <span class="metric-label">Autostart on Boot</span>
                <button
                  type="button"
                  class="badge interactive font-xs"
                  :class="vm?.autostart ? 'badge-success' : 'badge-neutral'"
                  style="align-self: flex-start; cursor: pointer"
                  :disabled="isTogglingAutostart"
                  @click="toggleAutostart"
                >
                  {{ vm?.autostart ? 'Enabled (On Boot)' : 'Disabled (Manual)' }}
                </button>
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
          <div class="card-header" style="display: flex; justify-content: space-between; align-items: center">
            <h3 class="card-title">Virtual Disks & Storage</h3>
            <button class="btn btn-xs btn-secondary" @click="emit('update:activeSubTab', 'configuration')">
              Edit Configuration
            </button>
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
                  <td class="font-xs"><span class="badge badge-neutral">{{ d.bus }}</span></td>
                  <td class="mono font-sm" style="font-weight: 600">{{ d.diskPath }}</td>
                  <td class="font-sm">{{ Math.round((d.sizeBytes || 0) / (1024 * 1024 * 1024)) }} GiB</td>
                  <td class="font-xs text-muted">{{ d.cacheMode || 'none' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    <!-- Subtab 2: Configuration (Hardware, Devices & Cloud-Init) -->
    <div v-else-if="props.activeSubTab === 'configuration' || props.activeSubTab === 'hardware'" class="vm-tab-content">
      <div class="grid-2">
        <!-- Card 1: Compute, Memory, Boot & Platform Configuration -->
        <div class="card detail-card">
          <div class="card-header">
            <div>
              <h3 class="card-title">Compute, Memory & Platform</h3>
              <p class="card-subtitle">Configure processors, RAM allocation, firmware and boot settings</p>
            </div>
          </div>
          <div class="card-body">
            <form style="display: flex; flex-direction: column; gap: 14px" @submit.prevent="handleSaveHardwareConfig">
              <div class="grid-2" style="grid-template-columns: 1fr 1fr; gap: 12px">
                <div class="form-group">
                  <label class="form-label">VM Name</label>
                  <input v-model="hwForm.vmName" type="text" class="form-input" required />
                </div>
                <div class="form-group">
                  <label class="form-label">Description</label>
                  <input v-model="hwForm.description" type="text" class="form-input" placeholder="Optional notes" />
                </div>
                <div class="form-group">
                  <label class="form-label">CPU Sockets</label>
                  <input v-model.number="hwForm.sockets" type="number" min="1" max="8" class="form-input" />
                </div>
                <div class="form-group">
                  <label class="form-label">Cores per Socket</label>
                  <input v-model.number="hwForm.cores" type="number" min="1" max="64" class="form-input" />
                </div>
                <div class="form-group">
                  <label class="form-label">CPU Model</label>
                  <select v-model="hwForm.cpuType" class="form-input">
                    <option value="host">host (Passthrough)</option>
                    <option value="kvm64">kvm64 (Compatible)</option>
                    <option value="max">max (All Features)</option>
                  </select>
                </div>
                <div class="form-group">
                  <label class="form-label">Memory (MiB)</label>
                  <input v-model.number="hwForm.memoryMib" type="number" min="256" step="256" class="form-input" />
                </div>
                <div class="form-group">
                  <label class="form-label">Machine Type</label>
                  <select v-model="hwForm.machineType" class="form-input">
                    <option value="i440fx">i440fx (Standard PC)</option>
                    <option value="q35">q35 (PCIe Native)</option>
                  </select>
                </div>
                <div class="form-group">
                  <label class="form-label">BIOS / Firmware</label>
                  <select v-model="hwForm.biosType" class="form-input">
                    <option value="seabios">SeaBIOS (Legacy BIOS)</option>
                    <option value="ovmf">OVMF (UEFI)</option>
                  </select>
                </div>
                <div class="form-group">
                  <label class="form-label">Boot Order</label>
                  <select v-model="hwForm.bootOrder" class="form-input">
                    <option value="hd,cdrom">Hard Disk, then CD-ROM</option>
                    <option value="cdrom,hd">CD-ROM, then Hard Disk</option>
                    <option value="network,hd,cdrom">Network (PXE), then Hard Disk</option>
                  </select>
                </div>
                <div class="form-group">
                  <label class="form-label">Display Adapter</label>
                  <select v-model="hwForm.displayType" class="form-input">
                    <option value="virtio-gpu">VirtIO-GPU</option>
                    <option value="std">Standard VGA</option>
                    <option value="qxl">SPICE QXL</option>
                    <option value="none">Headless (Serial Only)</option>
                  </select>
                </div>
                <div class="form-group">
                  <label class="form-label">TPM Emulator</label>
                  <select v-model="hwForm.tpmVersion" class="form-input">
                    <option value="none">None (Disabled)</option>
                    <option value="v2.0">TPM 2.0 (swtpm)</option>
                  </select>
                </div>
                <div class="form-group">
                  <label class="form-label">Hugepages</label>
                  <select v-model="hwForm.hugepages" class="form-input">
                    <option value="off">Off (Standard 4KB)</option>
                    <option value="2MB">2MB Hugepages</option>
                    <option value="1GB">1GB Hugepages</option>
                  </select>
                </div>
              </div>

              <div style="display: flex; gap: 18px; flex-wrap: wrap; margin-top: 4px">
                <label style="display: flex; align-items: center; gap: 6px; font-size: 13px; cursor: pointer">
                  <input v-model="hwForm.autostart" type="checkbox" />
                  <span>Autostart on Boot</span>
                </label>
                <label style="display: flex; align-items: center; gap: 6px; font-size: 13px; cursor: pointer">
                  <input v-model="hwForm.numa" type="checkbox" />
                  <span>Enable NUMA</span>
                </label>
                <label
                  v-if="hwForm.biosType === 'ovmf'"
                  style="display: flex; align-items: center; gap: 6px; font-size: 13px; cursor: pointer"
                >
                  <input v-model="hwForm.secureBoot" type="checkbox" />
                  <span>UEFI Secure Boot</span>
                </label>
              </div>

              <div style="display: flex; justify-content: flex-end; padding-top: 10px; border-top: 1px solid var(--border-subtle)">
                <button type="submit" class="btn btn-sm btn-primary" :disabled="isSavingConfig">
                  {{ isSavingConfig ? 'Saving...' : 'Save Configuration' }}
                </button>
              </div>
            </form>
          </div>
        </div>

        <!-- Card 2: Cloud-Init Provisioning -->
        <div class="card detail-card">
          <div class="card-header">
            <div>
              <h3 class="card-title">Cloud-Init Guest Provisioning</h3>
              <p class="card-subtitle">Automated user credentials, SSH keys and network configuration</p>
            </div>
          </div>
          <div class="card-body">
            <form style="display: flex; flex-direction: column; gap: 14px" @submit.prevent="handleSaveCloudInit">
              <div class="grid-2" style="grid-template-columns: 1fr 1fr; gap: 12px">
                <div class="form-group">
                  <label class="form-label">Default User</label>
                  <input v-model="ciForm.user" type="text" class="form-input" placeholder="e.g. ubuntu, debian, root" />
                </div>
                <div class="form-group">
                  <label class="form-label">Password</label>
                  <input v-model="ciForm.password" type="password" class="form-input" placeholder="Leave blank to keep unchanged" />
                </div>
              </div>
              <div class="form-group">
                <label class="form-label">IP Configuration</label>
                <input v-model="ciForm.ipConfig" type="text" class="form-input mono" placeholder="ip=dhcp or ip=192.168.1.50/24,gw=192.168.1.1" />
              </div>
              <div class="form-group">
                <label class="form-label">SSH Public Keys (one per line)</label>
                <textarea v-model="ciForm.sshPublicKeys" rows="4" class="form-input mono" placeholder="ssh-ed25519 AAAA..."></textarea>
              </div>
              <div style="display: flex; justify-content: flex-end; padding-top: 10px; border-top: 1px solid var(--border-subtle)">
                <button type="submit" class="btn btn-sm btn-primary" :disabled="isSavingCloudInit">
                  {{ isSavingCloudInit ? 'Saving...' : 'Save Cloud-Init' }}
                </button>
              </div>
            </form>
          </div>
        </div>
      </div>

      <!-- Attached Storage & Network Hardware Inventory -->
      <div class="card">
        <div class="card-header" style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 10px">
          <div>
            <h3 class="card-title">Attached Disks, Media & Network Interfaces</h3>
            <p class="card-subtitle">Manage virtual block devices, ISO drives and network adapters</p>
          </div>
          <div style="display: flex; gap: 8px">
            <button class="btn btn-sm btn-secondary" @click="openAddDisk">
              <MdiIcon :path="mdiPlus" size="14" />
              <span>Add Disk</span>
            </button>
            <button class="btn btn-sm btn-secondary" @click="openAddNic">
              <MdiIcon :path="mdiPlus" size="14" />
              <span>Add NIC</span>
            </button>
          </div>
        </div>
        <div class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th style="width: 160px">Component</th>
                <th>Configuration & Details</th>
                <th style="width: 120px">Bus / Model</th>
                <th style="width: 90px; text-align: right">Actions</th>
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
                <td class="font-sm">
                  <span class="mono">{{ d.diskPath }}</span> ({{ Math.round((d.sizeBytes || 0) / (1024 * 1024 * 1024)) }} GiB, cache: {{ d.cacheMode || 'none' }}{{ d.discard ? ', discard: on' : '' }})
                </td>
                <td><span class="badge badge-neutral font-xs">{{ d.bus }}</span></td>
                <td style="text-align: right">
                  <div style="display: inline-flex; gap: 4px; justify-content: flex-end">
                    <button class="btn btn-xs btn-secondary" title="Edit Disk Configuration" @click="openEditDisk(idx)">
                      <MdiIcon :path="mdiPencil" size="13" />
                    </button>
                    <button class="btn btn-xs btn-danger" title="Detach Disk" @click="handleRemoveDisk(idx)">
                      <MdiIcon :path="mdiTrashCanOutline" size="13" />
                    </button>
                  </div>
                </td>
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
                <td><span class="badge badge-neutral font-xs">{{ cd.bus || 'ide' }}</span></td>
                <td style="text-align: right">
                  <div style="display: inline-flex; gap: 4px; justify-content: flex-end">
                    <button
                      v-if="cd.isoPath"
                      class="btn btn-xs btn-secondary"
                      title="Eject CD-ROM Media"
                      @click="handleEjectMedia(cd.id || String(idx))"
                    >
                      Eject
                    </button>
                    <button
                      class="btn btn-xs btn-secondary"
                      :title="cd.isoPath ? 'Swap ISO Image' : 'Insert ISO Image'"
                      @click="openMediaDialog(cd.id || String(idx), cd.isoPath)"
                    >
                      {{ cd.isoPath ? 'Swap' : 'Insert' }}
                    </button>
                  </div>
                </td>
              </tr>

              <!-- Network Interfaces -->
              <tr v-for="(nic, idx) in vm?.networkInterfaces || []" :key="`nic-${idx}`">
                <td style="font-weight: 600">
                  <div style="display: flex; align-items: center; gap: 6px">
                    <MdiIcon :path="mdiNetwork" size="16" style="color: var(--accent-primary)" />
                    <span>Network (net{{ idx }})</span>
                  </div>
                </td>
                <td class="font-sm">
                  Bridge: <span class="mono">{{ nic.bridge }}</span><span v-if="nic.macAddress"> · MAC: <span class="mono">{{ nic.macAddress }}</span></span>
                  <span v-if="nic.vlanTag"> · VLAN: <span class="mono">{{ nic.vlanTag }}</span></span>
                </td>
                <td><span class="badge badge-neutral font-xs">{{ nic.model || 'virtio' }}</span></td>
                <td style="text-align: right">
                  <div style="display: inline-flex; gap: 4px; justify-content: flex-end">
                    <button class="btn btn-xs btn-secondary" title="Edit Network Interface" @click="openEditNic(idx)">
                      <MdiIcon :path="mdiPencil" size="13" />
                    </button>
                    <button class="btn btn-xs btn-danger" title="Remove Network Interface" @click="handleRemoveNic(idx)">
                      <MdiIcon :path="mdiTrashCanOutline" size="13" />
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Subtab 3: Integrated Console -->
    <div v-else-if="props.activeSubTab === 'console'" class="vm-tab-content vm-console-tab-content">
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
          <span class="font-sm" style="font-weight: 600">{{ cleanVmName }} Console (Port: {{ consoleSession.vncPort }})</span>
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
            @close="emit('update:activeSubTab', 'summary')"
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
                    <span>{{ snap.snapshotId }}</span>
                  </div>
                </td>
                <td class="text-secondary font-sm">{{ snap.description || '-' }}</td>
                <td class="font-sm text-muted">{{ snap.createTime || '-' }}</td>
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
                    <button class="btn btn-xs btn-danger" title="Delete" @click="handleDeleteSnapshot(snap)">
                      <MdiIcon :path="mdiTrashCanOutline" size="13" />
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Add Disk Modal -->
    <div v-if="showAddDiskModal" class="modal-overlay" @click.self="showAddDiskModal = false">
      <div class="modal-container" style="max-width: 480px">
        <div class="modal-header">
          <h3 class="modal-title">Add Virtual Disk: {{ cleanVmName }}</h3>
          <button class="btn btn-xs btn-ghost" @click="showAddDiskModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <div class="modal-body" style="display: flex; flex-direction: column; gap: 14px; padding: 16px 20px">
          <div class="form-group">
            <label class="form-label">Disk Device / ZVol Path</label>
            <input v-model="newDisk.diskPath" type="text" class="form-input mono" required />
          </div>
          <div class="grid-2" style="grid-template-columns: 1fr 1fr; gap: 12px">
            <div class="form-group">
              <label class="form-label">Size (GiB)</label>
              <input v-model.number="newDisk.sizeGib" type="number" min="1" class="form-input" />
            </div>
            <div class="form-group">
              <label class="form-label">Controller Bus</label>
              <select v-model="newDisk.bus" class="form-input">
                <option value="scsi">VirtIO SCSI</option>
                <option value="virtio">VirtIO Block</option>
                <option value="sata">SATA</option>
              </select>
            </div>
          </div>
        </div>
        <div class="modal-footer" style="padding: 12px 20px; display: flex; justify-content: flex-end; gap: 8px">
          <button class="btn btn-secondary btn-sm" @click="showAddDiskModal = false">Cancel</button>
          <button class="btn btn-primary btn-sm" :disabled="!newDisk.diskPath.trim() || isBusy" @click="submitAddDisk">
            Attach Disk
          </button>
        </div>
      </div>
    </div>

    <!-- Add NIC Modal -->
    <div v-if="showAddNicModal" class="modal-overlay" @click.self="showAddNicModal = false">
      <div class="modal-container" style="max-width: 440px">
        <div class="modal-header">
          <h3 class="modal-title">Add Network Interface: {{ cleanVmName }}</h3>
          <button class="btn btn-xs btn-ghost" @click="showAddNicModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <div class="modal-body" style="display: flex; flex-direction: column; gap: 14px; padding: 16px 20px">
          <div class="form-group">
            <label class="form-label">Host Network Bridge</label>
            <select v-model="newNic.bridge" class="form-input mono">
              <option v-for="b in bridgeOptionsFor(newNic.bridge)" :key="b.name" :value="b.name">
                {{ b.label }}
              </option>
            </select>
          </div>
          <div class="form-group">
            <label class="form-label">Adapter Model</label>
            <select v-model="newNic.model" class="form-input">
              <option value="virtio">VirtIO (Paravirtualized)</option>
              <option value="e1000e">Intel e1000e</option>
              <option value="rtl8139">Realtek RTL8139</option>
            </select>
          </div>
          <div class="form-group">
            <label class="form-label">VLAN Tag (Optional)</label>
            <input v-model.number="addNicForm.vlanTag" type="number" min="0" max="4094" class="form-input" placeholder="0 (Native / No VLAN)" />
          </div>
        </div>
        <div class="modal-footer" style="padding: 12px 20px; display: flex; justify-content: flex-end; gap: 8px">
          <button class="btn btn-secondary btn-sm" @click="showAddNicModal = false">Cancel</button>
          <button class="btn btn-primary btn-sm" :disabled="isBusy" @click="submitAddNic">
            Add NIC
          </button>
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

    <!-- Edit Disk Modal -->
    <div v-if="showEditDiskModal" class="modal-overlay" @click.self="showEditDiskModal = false">
      <div class="modal-container" style="max-width: 460px">
        <div class="modal-header">
          <h3 class="modal-title">Edit Virtual Disk: {{ editDiskForm.diskPath }}</h3>
          <button class="btn btn-xs btn-ghost" @click="showEditDiskModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <div class="modal-body" style="display: flex; flex-direction: column; gap: 14px; padding: 16px 20px">
          <div class="form-group">
            <label class="form-label">Controller Bus</label>
            <select v-model="editDiskForm.bus" class="form-input">
              <option value="scsi">VirtIO SCSI</option>
              <option value="virtio">VirtIO Block</option>
              <option value="sata">SATA</option>
            </select>
          </div>
          <div class="form-group">
            <label class="form-label">Cache Mode</label>
            <select v-model="editDiskForm.cacheMode" class="form-input">
              <option value="none">none (Direct I/O — Recommended)</option>
              <option value="writeback">writeback (Fast Async)</option>
              <option value="writethrough">writethrough (Safe Sync)</option>
            </select>
          </div>
          <div class="form-group" style="display: flex; align-items: center; gap: 8px">
            <input id="edit-discard-cb" v-model="editDiskForm.discard" type="checkbox" />
            <label for="edit-discard-cb" style="margin: 0; font-size: 13px; cursor: pointer">Enable Discard / TRIM</label>
          </div>
        </div>
        <div class="modal-footer" style="padding: 12px 20px; display: flex; justify-content: flex-end; gap: 8px">
          <button class="btn btn-secondary btn-sm" @click="showEditDiskModal = false">Cancel</button>
          <button class="btn btn-primary btn-sm" :disabled="isBusy" @click="submitEditDisk">
            Save Disk
          </button>
        </div>
      </div>
    </div>

    <!-- Edit NIC Modal -->
    <div v-if="showEditNicModal" class="modal-overlay" @click.self="showEditNicModal = false">
      <div class="modal-container" style="max-width: 440px">
        <div class="modal-header">
          <h3 class="modal-title">Edit Network Interface</h3>
          <button class="btn btn-xs btn-ghost" @click="showEditNicModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <div class="modal-body" style="display: flex; flex-direction: column; gap: 14px; padding: 16px 20px">
          <div class="form-group">
            <label class="form-label">Host Network Bridge</label>
            <select v-model="editNicForm.bridge" class="form-input mono">
              <option v-for="b in bridgeOptionsFor(editNicForm.bridge)" :key="b.name" :value="b.name">
                {{ b.label }}
              </option>
            </select>
          </div>
          <div class="form-group">
            <label class="form-label">Adapter Model</label>
            <select v-model="editNicForm.model" class="form-input">
              <option value="virtio">VirtIO (Paravirtualized)</option>
              <option value="e1000e">Intel e1000e</option>
              <option value="rtl8139">Realtek RTL8139</option>
            </select>
          </div>
          <div class="form-group">
            <label class="form-label">VLAN Tag (Optional)</label>
            <input v-model.number="editNicForm.vlanTag" type="number" min="0" max="4094" class="form-input" placeholder="0 (Native / No VLAN)" />
          </div>
        </div>
        <div class="modal-footer" style="padding: 12px 20px; display: flex; justify-content: flex-end; gap: 8px">
          <button class="btn btn-secondary btn-sm" @click="showEditNicModal = false">Cancel</button>
          <button class="btn btn-primary btn-sm" :disabled="isBusy" @click="submitEditNic">
            Save Interface
          </button>
        </div>
      </div>
    </div>

    <!-- Insert / Swap CD-ROM Media Modal -->
    <div v-if="showMediaModal" class="modal-overlay" @click.self="showMediaModal = false">
      <div class="modal-container" style="max-width: 500px">
        <div class="modal-header">
          <h3 class="modal-title">Mount CD-ROM ISO Media</h3>
          <button class="btn btn-xs btn-ghost" @click="showMediaModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <div class="modal-body" style="display: flex; flex-direction: column; gap: 14px; padding: 16px 20px">
          <div v-if="nasState.isos.length > 0" class="form-group">
            <label class="form-label">Select from Storage Repository</label>
            <select v-model="selectedMediaIso" class="form-input mono">
              <option v-for="iso in nasState.isos" :key="iso.path" :value="iso.path">
                {{ iso.name }} ({{ iso.sizeFormatted }})
              </option>
            </select>
          </div>
          <div class="form-group">
            <label class="form-label">Custom ISO Image Path</label>
            <input
              v-model="selectedMediaIso"
              type="text"
              class="form-input mono"
              placeholder="/mnt/tank/isos/debian-12.iso"
            />
          </div>
        </div>
        <div class="modal-footer" style="padding: 12px 20px; display: flex; justify-content: flex-end; gap: 8px">
          <button class="btn btn-secondary btn-sm" @click="showMediaModal = false">Cancel</button>
          <button class="btn btn-primary btn-sm" :disabled="!selectedMediaIso.trim() || isBusy" @click="handleAttachMedia">
            Mount ISO Image
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

.vm-detail-view.is-console-tab {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
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

.vm-console-tab-content {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.embedded-vnc-card {
  padding: 0;
  overflow: hidden;
  flex: 1;
  min-height: max(560px, calc(100vh - 160px));
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
  flex-shrink: 0;
}

.vnc-body-wrap {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 10px;
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
  font-size: 13px;
}

.font-xs {
  font-size: 11.5px;
}
</style>
