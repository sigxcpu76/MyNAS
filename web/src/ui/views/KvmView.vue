<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { services } from '../../container';
import {
  formatBytes,
  type VirtualMachine,
  type VirtualMachineCloudInit,
  type VirtualMachineConsoleSession,
  type VirtualMachineDisk,
  type VirtualMachineNetworkInterface,
  type VirtualMachineSnapshot,
} from '../../domain/models';
import { confirmModal, nasState, notify } from '../store';
import NoVncModal from '../components/NoVncModal.vue';
import MdiIcon from '../components/MdiIcon.vue';
import { generateNanosecondId, generateVMDiskName, generateZvolDiskPath } from '../../domain/id';
import {
  mdiClose,
  mdiCog,
  mdiConsole,
  mdiContentCopy,
  mdiDatabase,
  mdiEye,
  mdiHarddisk,
  mdiInformationOutline,
  mdiMagnify,
  mdiNetwork,
  mdiPause,
  mdiPlay,
  mdiPlus,
  mdiPower,
  mdiRefresh,
  mdiRestore,
  mdiServer,
  mdiShieldCheck,
  mdiSpeedometer,
  mdiStop,
  mdiTrashCan,
} from '@mdi/js';

// State
const searchQuery = ref('');
const activeConsoleSession = ref<VirtualMachineConsoleSession | null>(null);
const inspectingVm = ref<VirtualMachine | null>(null);
const inspectorTab = ref<'summary' | 'hardware' | 'cloudinit' | 'snapshots'>('summary');
const cloningVm = ref<VirtualMachine | null>(null);
const showCreateWizard = ref(false);
const wizardStep = ref(1);

// Snapshot state
const snapshotsList = ref<VirtualMachineSnapshot[]>([]);
const showTakeSnapshotModal = ref(false);
const snapshotForm = reactive({
  snapshotId: '',
  description: '',
  includeRam: false,
});

// Cloud-Init Form for Inspector
const ciForm = reactive({
  user: '',
  password: '',
  sshPublicKeys: '',
  ipConfig: '',
});

watch(inspectingVm, (vm) => {
  if (vm?.cloudInit) {
    ciForm.user = vm.cloudInit.user || '';
    ciForm.password = vm.cloudInit.password || '';
    ciForm.sshPublicKeys = (vm.cloudInit.sshPublicKeys || []).join('\n');
    ciForm.ipConfig = vm.cloudInit.ipConfig || '';
  } else {
    ciForm.user = '';
    ciForm.password = '';
    ciForm.sshPublicKeys = '';
    ciForm.ipConfig = '';
  }
}, { immediate: true });

// Media insertion modal
const showMediaModal = ref(false);
const mediaForm = reactive({
  isoPath: '',
});

// Add Hardware Modals
const showAddDiskModal = ref(false);
const diskForm = reactive({
  bus: 'scsi',
  diskPath: '',
  sizeGib: 32,
  cacheMode: 'none',
  discard: true,
  iothread: true,
});

const showAddNicModal = ref(false);
const nicForm = reactive({
  model: 'virtio',
  bridge: 'virbr0',
  vlanTag: 0,
});

// Clone Form
const cloneForm = reactive({
  newId: '',
  newName: '',
  targetPool: 'tank',
  linkedClone: true,
});

// Create Wizard Form
const wizardForm = reactive({
  // Step 1: General
  vmId: '',
  vmName: '',
  osVariant: 'debian12',
  description: '',
  autostart: true,
  startupDelaySeconds: 0,

  // Step 2: System
  machineType: 'q35',
  biosType: 'ovmf',
  tpmVersion: 'v2.0',
  secureBoot: true,

  // Step 3: Disks
  diskBus: 'scsi',
  diskPath: '',
  diskSizeGib: 32,
  diskCache: 'none',
  diskDiscard: true,
  diskIothread: true,

  // Step 4: CPU
  sockets: 1,
  cores: 2,
  cpuType: 'host',
  numa: false,

  // Step 5: Memory
  memoryMib: 4096,
  balloonMinMib: 2048,
  hugepages: 'off',

  // Step 6: Network
  netModel: 'virtio',
  netBridge: 'virbr0',
  vlanTag: 0,
  firewall: true,

  // Step 7: Cloud-Init
  ciUser: 'admin',
  ciPassword: '',
  ciSshKeys: '',
  ciIpConfig: 'dhcp',
});

// Computed properties
const filteredVms = computed(() => {
  const q = searchQuery.value.trim().toLowerCase();
  if (!q) return nasState.vms;
  return nasState.vms.filter(
    (vm) =>
      vm.vmName.toLowerCase().includes(q) ||
      vm.name.toLowerCase().includes(q) ||
      (vm.description || '').toLowerCase().includes(q) ||
      (vm.guestOsName || '').toLowerCase().includes(q) ||
      (vm.guestIpAddresses || []).some((ip) => ip.includes(q))
  );
});

const runningCount = computed(() => nasState.vms.filter((v) => v.state === 'RUNNING').length);
const totalVcpus = computed(() => nasState.vms.reduce((acc, v) => acc + (v.vcpus || 0), 0));
const totalMemoryMib = computed(() => nasState.vms.reduce((acc, v) => acc + (v.memoryMib || 0), 0));

const availableZvols = computed(() => {
  const list: string[] = [];
  for (const vols of Object.values(nasState.volumesByPool)) {
    for (const v of vols) {
      if (v.devicePath) list.push(v.devicePath);
    }
  }
  return list;
});

const availablePools = computed(() => {
  return nasState.pools.map((p) => p.poolName || p.name.split('/').pop() || 'tank');
});

const mainPool = computed(() => nasState.systemConfig?.mainPool || 'tank');

const suggestedDiskName = computed(() => {
  return generateVMDiskName(wizardForm.vmId || 'vm', 0);
});

const suggestedDiskPath = computed(() => {
  return generateZvolDiskPath(mainPool.value, wizardForm.vmId || 'vm', 0);
});

function openCreateWizard() {
  wizardForm.vmId = generateNanosecondId();
  wizardForm.vmName = `vm-${wizardForm.vmId}`;
  wizardForm.osVariant = 'debian12';
  wizardForm.description = '';
  wizardForm.autostart = true;
  wizardForm.startupDelaySeconds = 0;
  wizardForm.machineType = 'q35';
  wizardForm.biosType = 'ovmf';
  wizardForm.tpmVersion = 'v2.0';
  wizardForm.secureBoot = true;
  wizardForm.diskBus = 'scsi';
  wizardForm.diskPath = suggestedDiskPath.value;
  wizardForm.diskSizeGib = 32;
  wizardForm.diskCache = 'none';
  wizardForm.diskDiscard = true;
  wizardForm.diskIothread = true;
  wizardForm.sockets = 1;
  wizardForm.cores = 2;
  wizardForm.cpuType = 'host';
  wizardForm.numa = false;
  wizardForm.memoryMib = 4096;
  wizardForm.balloonMinMib = 2048;
  wizardForm.hugepages = 'off';
  wizardForm.netModel = 'virtio';
  wizardForm.netBridge = 'virbr0';
  wizardForm.vlanTag = 0;
  wizardForm.firewall = true;
  wizardStep.value = 1;
  showCreateWizard.value = true;
}

function openTakeSnapshotModal() {
  snapshotForm.snapshotId = generateNanosecondId();
  snapshotForm.description = '';
  snapshotForm.includeRam = false;
  showTakeSnapshotModal.value = true;
}

function openAddDiskModal() {
  const currentDisksCount = inspectingVm.value?.disks?.length || 0;
  diskForm.bus = 'scsi';
  diskForm.diskPath = generateZvolDiskPath(mainPool.value, inspectingVm.value?.name || 'vm', currentDisksCount);
  diskForm.sizeGib = 32;
  diskForm.cacheMode = 'none';
  diskForm.discard = true;
  diskForm.iothread = true;
  showAddDiskModal.value = true;
}

// Lifecycle
onMounted(async () => {
  await refreshVms();
});

async function refreshVms() {
  try {
    nasState.vms = await services.kvm.listVirtualMachines();
    if (inspectingVm.value) {
      const updated = nasState.vms.find((v) => v.name === inspectingVm.value?.name);
      if (updated) {
        inspectingVm.value = updated;
        if (inspectorTab.value === 'snapshots') {
          await loadSnapshots(updated.name);
        }
      }
    }
  } catch (err: any) {
    notify('error', err?.message || 'Failed to refresh virtual machines');
  }
}

async function openInspector(vm: VirtualMachine, tab: 'summary' | 'hardware' | 'cloudinit' | 'snapshots' = 'summary') {
  inspectingVm.value = vm;
  inspectorTab.value = tab;
  if (tab === 'snapshots') {
    await loadSnapshots(vm.name);
  }
}

async function loadSnapshots(vmName: string) {
  try {
    snapshotsList.value = await services.kvm.listSnapshots(vmName);
  } catch (err: any) {
    snapshotsList.value = [];
  }
}

// Power Actions
async function handlePowerAction(vm: VirtualMachine, action: 'start' | 'stop' | 'reboot' | 'reset' | 'pause' | 'resume') {
  try {
    if (action === 'start') {
      await services.kvm.startVirtualMachine(vm.name);
    } else if (action === 'stop') {
      await services.kvm.stopVirtualMachine(vm.name, false);
    } else if (action === 'reboot') {
      await services.kvm.rebootVirtualMachine(vm.name);
    } else if (action === 'reset') {
      await services.kvm.resetVirtualMachine(vm.name);
    } else if (action === 'pause') {
      await services.kvm.pauseVirtualMachine(vm.name);
    } else if (action === 'resume') {
      await services.kvm.resumeVirtualMachine(vm.name);
    }
    notify('success', `Executed :${action} on VM ${vm.vmName}`);
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || `Failed to execute ${action} on VM`);
  }
}

async function handleOpenConsole(vm: VirtualMachine) {
  try {
    const session = await services.kvm.openConsole(vm.name);
    activeConsoleSession.value = session;
  } catch (err: any) {
    notify('error', err?.message || 'Failed to open noVNC console session');
  }
}

// Snapshot Actions
async function handleTakeSnapshot() {
  if (!inspectingVm.value || !snapshotForm.snapshotId.trim()) return;
  try {
    await services.kvm.createSnapshot(
      inspectingVm.value.name,
      snapshotForm.snapshotId.trim(),
      snapshotForm.description,
      snapshotForm.includeRam
    );
    notify('success', `Created snapshot "${snapshotForm.snapshotId}"`);
    showTakeSnapshotModal.value = false;
    snapshotForm.snapshotId = '';
    snapshotForm.description = '';
    snapshotForm.includeRam = false;
    await loadSnapshots(inspectingVm.value.name);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create snapshot');
  }
}

async function handleRollbackSnapshot(snap: VirtualMachineSnapshot) {
  if (!inspectingVm.value) return;
  const confirmed = await confirmModal({
    title: 'Rollback Virtual Machine Snapshot',
    tone: 'warning',
    message: `Rollback VM "${inspectingVm.value.vmName}" to snapshot "${snap.snapshotId}"?`,
    details: 'Current unsaved memory state and any disk writes since the snapshot was taken will be lost.',
    confirmText: 'Rollback Snapshot',
  });
  if (!confirmed) return;

  try {
    await services.kvm.rollbackSnapshot(snap.name);
    notify('success', `Rolled back to snapshot "${snap.snapshotId}"`);
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to rollback snapshot');
  }
}

async function handleDeleteSnapshot(snap: VirtualMachineSnapshot) {
  if (!inspectingVm.value) return;
  const confirmed = await confirmModal({
    title: 'Delete VM Snapshot',
    tone: 'danger',
    message: `Are you sure you want to delete snapshot "${snap.snapshotId}"?`,
    details: 'This snapshot differential will be removed from the virtual machine storage volume.',
    confirmText: 'Delete Snapshot',
  });
  if (!confirmed) return;

  try {
    await services.kvm.deleteSnapshot(snap.name);
    notify('info', `Deleted snapshot "${snap.snapshotId}"`);
    await loadSnapshots(inspectingVm.value.name);
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete snapshot');
  }
}

// Clone VM
function openCloneModal(vm: VirtualMachine) {
  cloningVm.value = vm;
  cloneForm.newId = generateNanosecondId();
  cloneForm.newName = `${vm.vmName} (Clone)`;
  cloneForm.targetPool = mainPool.value || 'tank';
  cloneForm.linkedClone = true;
}

async function handleCloneVm() {
  if (!cloningVm.value || !cloneForm.newId.trim()) return;
  try {
    await services.kvm.cloneVirtualMachine(
      cloningVm.value.name,
      cloneForm.newId.trim(),
      cloneForm.newName,
      cloneForm.targetPool,
      cloneForm.linkedClone
    );
    notify('success', `Cloned VM to ${cloneForm.newId}`);
    cloningVm.value = null;
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to clone VM');
  }
}

// Media Actions
async function handleAttachMedia() {
  if (!inspectingVm.value || !mediaForm.isoPath.trim()) return;
  try {
    await services.kvm.attachMedia(inspectingVm.value.name, mediaForm.isoPath.trim());
    notify('success', `Mounted ISO image ${mediaForm.isoPath}`);
    showMediaModal.value = false;
    mediaForm.isoPath = '';
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to mount media');
  }
}

async function handleEjectMedia() {
  if (!inspectingVm.value) return;
  try {
    await services.kvm.ejectMedia(inspectingVm.value.name);
    notify('info', 'Ejected CD/DVD media');
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to eject media');
  }
}

// Add Hardware
async function handleAddDisk() {
  if (!inspectingVm.value) return;
  const currentDisks = inspectingVm.value.disks ? [...inspectingVm.value.disks] : [];
  const nextId = `${diskForm.bus}${currentDisks.length}`;
  const diskPath = diskForm.diskPath.trim();

  const newDisk: VirtualMachineDisk = {
    id: nextId,
    bus: diskForm.bus,
    diskPath,
    sizeBytes: diskForm.sizeGib * 1024 * 1024 * 1024,
    cacheMode: diskForm.cacheMode,
    discard: diskForm.discard,
    iothread: diskForm.iothread,
    backup: true,
  };
  currentDisks.push(newDisk);

  try {
    await services.kvm.updateVirtualMachine(
      inspectingVm.value.name,
      { disks: currentDisks },
      ['disks']
    );
    notify('success', `Added virtual disk ${nextId}`);
    showAddDiskModal.value = false;
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to add hard disk');
  }
}

async function handleAddNic() {
  if (!inspectingVm.value) return;
  const currentNics = inspectingVm.value.networkInterfaces ? [...inspectingVm.value.networkInterfaces] : [];
  const nextId = `net${currentNics.length}`;

  const newNic: VirtualMachineNetworkInterface = {
    id: nextId,
    model: nicForm.model,
    bridge: nicForm.bridge,
    vlanTag: nicForm.vlanTag > 0 ? nicForm.vlanTag : undefined,
    firewall: true,
  };
  currentNics.push(newNic);

  try {
    await services.kvm.updateVirtualMachine(
      inspectingVm.value.name,
      { networkInterfaces: currentNics },
      ['network_interfaces']
    );
    notify('success', `Added network interface ${nextId}`);
    showAddNicModal.value = false;
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to add network device');
  }
}

// Save Cloud-Init
async function handleSaveCloudInit() {
  if (!inspectingVm.value) return;
  const sshKeys = ciForm.sshPublicKeys
    .split('\n')
    .map((k) => k.trim())
    .filter(Boolean);

  const ci: VirtualMachineCloudInit = {
    user: ciForm.user,
    password: ciForm.password,
    sshPublicKeys: sshKeys,
    ipConfig: ciForm.ipConfig,
  };

  try {
    await services.kvm.updateVirtualMachine(
      inspectingVm.value.name,
      { cloudInit: ci },
      ['cloud_init']
    );
    notify('success', 'Updated Cloud-Init guest provisioning configuration');
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to save Cloud-Init configuration');
  }
}

// Delete VM
async function handleDeleteVm(vm: VirtualMachine) {
  const confirmed = await confirmModal({
    title: 'Delete Virtual Machine',
    tone: 'danger',
    message: `Are you sure you want to delete virtual machine "${vm.vmName}"?`,
    details: 'This will undefine the QEMU/KVM domain and remove all associated XML configurations, disks mappings, and NVRAM state.',
    confirmText: 'Delete VM',
  });
  if (!confirmed) return;

  try {
    await services.kvm.deleteVirtualMachine(vm.name);
    notify('info', `Deleted virtual machine ${vm.vmName}`);
    if (inspectingVm.value?.name === vm.name) {
      inspectingVm.value = null;
    }
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete VM');
  }
}

// Wizard submit
async function handleWizardSubmit() {
  if (!wizardForm.vmId.trim()) {
    notify('error', 'VM Domain ID is required');
    return;
  }

  const vcpus = wizardForm.sockets * wizardForm.cores;
  const diskPath = wizardForm.diskPath.trim();
  const primaryDisk: VirtualMachineDisk = {
    id: `${wizardForm.diskBus}0`,
    bus: wizardForm.diskBus,
    diskPath,
    sizeBytes: wizardForm.diskSizeGib * 1024 * 1024 * 1024,
    cacheMode: wizardForm.diskCache,
    discard: wizardForm.diskDiscard,
    iothread: wizardForm.diskIothread,
    backup: true,
  };

  const primaryNic: VirtualMachineNetworkInterface = {
    id: 'net0',
    model: wizardForm.netModel,
    bridge: wizardForm.netBridge,
    vlanTag: wizardForm.vlanTag > 0 ? wizardForm.vlanTag : undefined,
    firewall: wizardForm.firewall,
  };

  const sshKeys = wizardForm.ciSshKeys
    .split('\n')
    .map((k) => k.trim())
    .filter(Boolean);

  try {
    const created = await services.kvm.createVirtualMachine(wizardForm.vmId.trim(), {
      vmName: wizardForm.vmName || wizardForm.vmId.trim(),
      description: wizardForm.description,
      osVariant: wizardForm.osVariant,
      autostart: wizardForm.autostart,
      startupDelaySeconds: wizardForm.startupDelaySeconds,
      sockets: wizardForm.sockets,
      cores: wizardForm.cores,
      vcpus,
      cpuType: wizardForm.cpuType,
      numa: wizardForm.numa,
      memoryMib: wizardForm.memoryMib,
      balloonMinMib: wizardForm.balloonMinMib,
      hugepages: wizardForm.hugepages,
      machineType: wizardForm.machineType,
      biosType: wizardForm.biosType,
      tpmVersion: wizardForm.tpmVersion,
      secureBoot: wizardForm.secureBoot,
      disks: [primaryDisk],
      networkInterfaces: [primaryNic],
      cloudInit: {
        user: wizardForm.ciUser,
        password: wizardForm.ciPassword,
        sshPublicKeys: sshKeys,
        ipConfig: wizardForm.ciIpConfig,
      },
    });

    notify('success', `Successfully deployed Proxmox-grade VM ${created.vmName}`);
    showCreateWizard.value = false;
    wizardStep.value = 1;
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to deploy virtual machine');
  }
}
</script>

<template>
  <div style="display: flex; flex-direction: column; gap: 20px">
    <!-- Top Summary Banner -->
    <div class="grid-4">
      <div class="card">
        <div class="card-header" style="margin-bottom: 6px">
          <span class="card-subtitle">Virtual Machines</span>
          <span class="badge badge-success">{{ runningCount }} / {{ nasState.vms.length }} Running</span>
        </div>
        <div style="font-size: 24px; font-weight: 700; color: var(--text-primary)">
          {{ nasState.vms.length }}
          <span style="font-size: 13px; color: var(--text-secondary); font-weight: normal; margin-left: 6px">
            Proxmox KVM Domains
          </span>
        </div>
      </div>

      <div class="card">
        <div class="card-header" style="margin-bottom: 6px">
          <span class="card-subtitle">vCPU Allocation</span>
          <MdiIcon :path="mdiSpeedometer" size="18" style="color: var(--accent-primary)" />
        </div>
        <div style="font-size: 24px; font-weight: 700; color: var(--text-primary)">
          {{ totalVcpus }}
          <span style="font-size: 13px; color: var(--text-secondary); font-weight: normal; margin-left: 6px">
            Assigned Cores
          </span>
        </div>
      </div>

      <div class="card">
        <div class="card-header" style="margin-bottom: 6px">
          <span class="card-subtitle">RAM Allocated</span>
          <MdiIcon :path="mdiServer" size="18" style="color: var(--accent-primary)" />
        </div>
        <div style="font-size: 24px; font-weight: 700; color: var(--text-primary)">
          {{ formatBytes(totalMemoryMib * 1024 * 1024) }}
          <span style="font-size: 13px; color: var(--text-secondary); font-weight: normal; margin-left: 6px">
            Memory
          </span>
        </div>
      </div>

      <div class="card">
        <div class="card-header" style="margin-bottom: 6px">
          <span class="card-subtitle">Hypervisor Engine</span>
          <span class="badge badge-neutral mono">QEMU / Libvirt</span>
        </div>
        <div style="font-size: 14px; font-weight: 600; color: var(--text-primary); margin-top: 4px">
          ZFS ZVols + noVNC + QGA
        </div>
        <p class="card-subtitle" style="margin-top: 4px">
          Proxmox VE Parity Architecture
        </p>
      </div>
    </div>

    <!-- Main Toolbar & VM Table -->
    <div class="card">
      <div class="card-header">
        <div>
          <h2 class="card-title" style="font-size: 17px">
            KVM Virtual Machines (`virtualMachines/{virtual_machine}`)
          </h2>
          <p class="card-subtitle">
            Enterprise QEMU/KVM virtual machines with UEFI OVMF, TPM 2.0, multi-disk ZVol topologies, Cloud-Init, and live snapshots.
          </p>
        </div>

        <div style="display: flex; gap: 10px; align-items: center">
          <div class="search-input-wrap">
            <MdiIcon :path="mdiMagnify" size="16" class="search-icon" />
            <input
              v-model="searchQuery"
              class="input input-sm mono"
              placeholder="Filter VMs, IPs, OS..."
              style="padding-left: 28px; width: 220px"
            />
          </div>

          <button class="btn btn-sm" @click="refreshVms">
            <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" /> Refresh
          </button>

          <button class="btn btn-sm btn-primary" @click="openCreateWizard">
            <MdiIcon :path="mdiPlus" size="15" style="margin-right: 4px" /> Create VM
          </button>
        </div>
      </div>

      <!-- VM Table -->
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>Status</th>
              <th>Name & ID</th>
              <th>Processors</th>
              <th>Memory</th>
              <th>Disks</th>
              <th>IP Addresses (QGA)</th>
              <th>Firmware / System</th>
              <th style="text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="vm in filteredVms" :key="vm.name">
              <!-- State -->
              <td>
                <span
                  class="badge mono"
                  :class="vm.state === 'RUNNING' ? 'badge-success' : (vm.state === 'PAUSED' ? 'badge-warning' : 'badge-neutral')"
                  style="display: inline-flex; align-items: center; gap: 5px"
                >
                  <span
                    v-if="vm.state === 'RUNNING'"
                    style="width: 7px; height: 7px; border-radius: 50%; background: var(--success); display: inline-block"
                  ></span>
                  {{ vm.state }}
                </span>
              </td>

              <!-- Name & ID -->
              <td>
                <div class="font-semibold" style="color: var(--text-primary); cursor: pointer" @click="openInspector(vm)">
                  {{ vm.vmName }}
                </div>
                <div class="mono text-muted" style="font-size: 11px">
                  {{ vm.name.replace('virtualMachines/', '') }}
                  <span v-if="vm.isTemplate" class="badge badge-warning" style="font-size: 9px; margin-left: 4px">TEMPLATE</span>
                </div>
              </td>

              <!-- CPU -->
              <td class="mono" style="font-size: 12px">
                {{ vm.vcpus }} vCPU
                <span v-if="vm.sockets && vm.cores" class="text-muted" style="font-size: 11px">
                  ({{ vm.sockets }}s / {{ vm.cores }}c)
                </span>
                <div class="text-muted" style="font-size: 10px">{{ vm.cpuType || 'host' }}</div>
              </td>

              <!-- Memory -->
              <td class="mono" style="font-size: 12px">
                {{ formatBytes(vm.memoryMib * 1024 * 1024) }}
                <div v-if="vm.balloonMinMib && vm.balloonMinMib !== vm.memoryMib" class="text-muted" style="font-size: 10px">
                  Min: {{ formatBytes(vm.balloonMinMib * 1024 * 1024) }}
                </div>
              </td>

              <!-- Disks -->
              <td class="mono" style="font-size: 11px">
                <span v-if="vm.disks && vm.disks.length > 0">
                  {{ vm.disks.length }} drive(s)
                  <div class="text-muted" style="font-size: 10px">
                    {{ vm.disks[0].id }} ({{ formatBytes(vm.disks[0].sizeBytes) }})
                  </div>
                </span>
                <span v-else-if="vm.diskPath">
                  {{ vm.diskPath.split('/').pop() }}
                </span>
                <span v-else class="text-muted">-</span>
              </td>

              <!-- IP Addresses -->
              <td>
                <div v-if="vm.guestIpAddresses && vm.guestIpAddresses.length > 0" style="display: flex; flex-direction: column; gap: 2px">
                  <span
                    v-for="ip in vm.guestIpAddresses.slice(0, 2)"
                    :key="ip"
                    class="badge mono badge-neutral"
                    style="font-size: 10px; width: fit-content"
                  >
                    {{ ip }}
                  </span>
                  <span v-if="vm.guestIpAddresses.length > 2" class="text-muted" style="font-size: 10px">
                    +{{ vm.guestIpAddresses.length - 2 }} more
                  </span>
                </div>
                <span v-else class="text-muted" style="font-size: 11px">
                  {{ vm.state === 'RUNNING' ? 'Querying QGA...' : 'Offline' }}
                </span>
              </td>

              <!-- Firmware -->
              <td class="mono" style="font-size: 11px">
                <div>{{ (vm.biosType || 'seabios').toUpperCase() }} ({{ vm.machineType || 'q35' }})</div>
                <div v-if="vm.tpmVersion === 'v2.0'" style="color: var(--accent-primary); font-size: 10px">
                  vTPM 2.0 active
                </div>
              </td>

              <!-- Actions -->
              <td style="text-align: right">
                <div style="display: inline-flex; gap: 6px; align-items: center">
                  <!-- Start/Resume -->
                  <button
                    v-if="vm.state === 'STOPPED'"
                    class="btn btn-xs btn-success"
                    title="Start VM"
                    @click="handlePowerAction(vm, 'start')"
                  >
                    <MdiIcon :path="mdiPlay" size="13" />
                  </button>
                  <button
                    v-else-if="vm.state === 'PAUSED'"
                    class="btn btn-xs btn-primary"
                    title="Resume VM"
                    @click="handlePowerAction(vm, 'resume')"
                  >
                    <MdiIcon :path="mdiPlay" size="13" />
                  </button>

                  <!-- Pause -->
                  <button
                    v-if="vm.state === 'RUNNING'"
                    class="btn btn-xs"
                    title="Pause VM"
                    @click="handlePowerAction(vm, 'pause')"
                  >
                    <MdiIcon :path="mdiPause" size="13" />
                  </button>

                  <!-- Reboot -->
                  <button
                    v-if="vm.state === 'RUNNING'"
                    class="btn btn-xs"
                    title="Reboot VM"
                    @click="handlePowerAction(vm, 'reboot')"
                  >
                    <MdiIcon :path="mdiRefresh" size="13" />
                  </button>

                  <!-- Shutdown -->
                  <button
                    v-if="vm.state === 'RUNNING'"
                    class="btn btn-xs btn-warning"
                    title="Shutdown VM"
                    @click="handlePowerAction(vm, 'stop')"
                  >
                    <MdiIcon :path="mdiPower" size="13" />
                  </button>

                  <!-- Console -->
                  <button
                    class="btn btn-xs btn-primary"
                    :disabled="vm.state !== 'RUNNING'"
                    title="Open noVNC Console"
                    @click="handleOpenConsole(vm)"
                  >
                    <MdiIcon :path="mdiConsole" size="13" style="margin-right: 3px" /> Console
                  </button>

                  <!-- Clone -->
                  <button
                    class="btn btn-xs"
                    title="Clone VM"
                    @click="openCloneModal(vm)"
                  >
                    <MdiIcon :path="mdiContentCopy" size="13" />
                  </button>

                  <!-- Inspect / Hardware -->
                  <button
                    class="btn btn-xs"
                    title="Inspect Hardware & Snapshots"
                    @click="openInspector(vm)"
                  >
                    <MdiIcon :path="mdiCog" size="13" />
                  </button>

                  <!-- Delete -->
                  <button
                    class="btn btn-xs btn-danger"
                    title="Delete VM"
                    @click="handleDeleteVm(vm)"
                  >
                    <MdiIcon :path="mdiTrashCan" size="13" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- PROXMOX-STYLE VM INSPECTOR MODAL -->
    <!-- ======================================================================= -->
    <div v-if="inspectingVm" class="modal-backdrop" @click.self="inspectingVm = null">
      <div class="modal-card" style="max-width: 960px; width: 95vw; max-height: 90vh; display: flex; flex-direction: column">
        <!-- Modal Header -->
        <div style="padding: 16px 20px; border-bottom: 1px solid var(--border-subtle); display: flex; justify-content: space-between; align-items: center">
          <div style="display: flex; align-items: center; gap: 12px">
            <MdiIcon :path="mdiServer" size="24" style="color: var(--accent-primary)" />
            <div>
              <div style="display: flex; align-items: center; gap: 8px">
                <h3 style="margin: 0; font-size: 17px; font-weight: 700">
                  {{ inspectingVm.vmName }}
                </h3>
                <span
                  class="badge mono"
                  :class="inspectingVm.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'"
                  style="font-size: 11px"
                >
                  {{ inspectingVm.state }}
                </span>
              </div>
              <div class="mono text-muted" style="font-size: 11px">
                {{ inspectingVm.name }} (UUID: {{ inspectingVm.uuid || 'N/A' }})
              </div>
            </div>
          </div>

          <!-- Quick Action Buttons inside Inspector Header -->
          <div style="display: flex; gap: 8px; align-items: center">
            <button
              v-if="inspectingVm.state !== 'RUNNING'"
              class="btn btn-sm btn-success"
              @click="handlePowerAction(inspectingVm, 'start')"
            >
              <MdiIcon :path="mdiPlay" size="14" style="margin-right: 4px" /> Start
            </button>
            <button
              v-else
              class="btn btn-sm btn-warning"
              @click="handlePowerAction(inspectingVm, 'stop')"
            >
              <MdiIcon :path="mdiPower" size="14" style="margin-right: 4px" /> Shutdown
            </button>
            <button
              class="btn btn-sm btn-primary"
              :disabled="inspectingVm.state !== 'RUNNING'"
              @click="handleOpenConsole(inspectingVm)"
            >
              <MdiIcon :path="mdiConsole" size="14" style="margin-right: 4px" /> Console
            </button>
            <button class="btn btn-sm" @click="inspectingVm = null">
              <MdiIcon :path="mdiClose" size="16" />
            </button>
          </div>
        </div>

        <!-- Navigation Tabs -->
        <div class="modal-nav-tabs">
          <button
            class="modal-nav-tab"
            :class="{ active: inspectorTab === 'summary' }"
            @click="inspectorTab = 'summary'"
          >
            <MdiIcon :path="mdiSpeedometer" size="15" style="margin-right: 6px" /> Summary
          </button>
          <button
            class="modal-nav-tab"
            :class="{ active: inspectorTab === 'hardware' }"
            @click="inspectorTab = 'hardware'"
          >
            <MdiIcon :path="mdiHarddisk" size="15" style="margin-right: 6px" /> Hardware
          </button>
          <button
            class="modal-nav-tab"
            :class="{ active: inspectorTab === 'cloudinit' }"
            @click="inspectorTab = 'cloudinit'"
          >
            <MdiIcon :path="mdiNetwork" size="15" style="margin-right: 6px" /> Cloud-Init
          </button>
          <button
            class="modal-nav-tab"
            :class="{ active: inspectorTab === 'snapshots' }"
            @click="inspectorTab = 'snapshots'; loadSnapshots(inspectingVm.name)"
          >
            <MdiIcon :path="mdiRestore" size="15" style="margin-right: 6px" /> Snapshots
          </button>
        </div>

        <!-- Tab Content Body -->
        <div style="padding: 20px; overflow-y: auto; flex: 1">
          <!-- ========================================== -->
          <!-- TAB 1: SUMMARY -->
          <!-- ========================================== -->
          <div v-if="inspectorTab === 'summary'" style="display: flex; flex-direction: column; gap: 16px">
            <!-- Telemetry Gauges -->
            <div style="display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 14px">
              <div style="background: var(--bg-base); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 14px">
                <div class="card-subtitle">CPU Usage (QEMU/QGA)</div>
                <div style="font-size: 22px; font-weight: 700; color: var(--text-primary); margin-top: 4px">
                  {{ inspectingVm.guestCpuPercent ? inspectingVm.guestCpuPercent.toFixed(1) + '%' : '-' }}
                </div>
                <div class="text-muted" style="font-size: 11px; margin-top: 4px">
                  Allocated: {{ inspectingVm.vcpus }} vCPU ({{ inspectingVm.cpuType || 'host' }})
                </div>
              </div>

              <div style="background: var(--bg-base); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 14px">
                <div class="card-subtitle">Memory Usage</div>
                <div style="font-size: 22px; font-weight: 700; color: var(--text-primary); margin-top: 4px">
                  {{ inspectingVm.guestMemoryUsedMib ? formatBytes(inspectingVm.guestMemoryUsedMib * 1024 * 1024) : '-' }}
                  <span style="font-size: 13px; color: var(--text-secondary); font-weight: normal">
                    / {{ formatBytes(inspectingVm.memoryMib * 1024 * 1024) }}
                  </span>
                </div>
                <div class="text-muted" style="font-size: 11px; margin-top: 4px">
                  Ballooning Target: {{ formatBytes((inspectingVm.balloonMinMib || inspectingVm.memoryMib) * 1024 * 1024) }}
                </div>
              </div>

              <div style="background: var(--bg-base); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 14px">
                <div class="card-subtitle">QEMU Guest Agent</div>
                <div style="font-size: 22px; font-weight: 700; margin-top: 4px" :style="{ color: inspectingVm.agentRunning ? 'var(--success-text)' : 'var(--text-secondary)' }">
                  {{ inspectingVm.agentRunning ? 'RUNNING' : 'DISCONNECTED' }}
                </div>
                <div class="text-muted" style="font-size: 11px; margin-top: 4px">
                  OS: {{ inspectingVm.guestOsName || inspectingVm.osVariant || 'Unknown' }}
                </div>
              </div>
            </div>

            <!-- Guest Network IP Addresses -->
            <div style="background: var(--bg-base); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 16px">
              <div style="font-weight: 600; font-size: 13px; margin-bottom: 10px; color: var(--text-primary)">
                Guest Network IP Addresses (Reported by QGA)
              </div>
              <div v-if="inspectingVm.guestIpAddresses && inspectingVm.guestIpAddresses.length > 0" style="display: flex; flex-wrap: wrap; gap: 8px">
                <code
                  v-for="ip in inspectingVm.guestIpAddresses"
                  :key="ip"
                  class="mono"
                  style="font-size: 12px; background: var(--bg-surface); padding: 5px 10px; border-radius: 4px; border: 1px solid var(--border-subtle)"
                >
                  {{ ip }}
                </code>
              </div>
              <div v-else class="text-muted" style="font-size: 12px">
                No IP addresses reported yet. Ensure the <code class="mono">qemu-guest-agent</code> package is running inside the guest.
              </div>
            </div>
          </div>

          <!-- ========================================== -->
          <!-- TAB 2: HARDWARE (PROXMOX-STYLE HARDWARE TABLE) -->
          <!-- ========================================== -->
          <div v-else-if="inspectorTab === 'hardware'">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px">
              <div style="font-weight: 600; font-size: 14px">
                Hardware Configuration
              </div>
              <div style="display: flex; gap: 8px">
                <button class="btn btn-xs" @click="openAddDiskModal">
                  <MdiIcon :path="mdiPlus" size="13" /> Add Hard Disk
                </button>
                <button class="btn btn-xs" @click="showAddNicModal = true">
                  <MdiIcon :path="mdiPlus" size="13" /> Add Network Device
                </button>
              </div>
            </div>

            <div class="table-wrap">
              <table class="data-table">
                <thead>
                  <tr>
                    <th style="width: 200px">Device</th>
                    <th>Configuration</th>
                    <th style="text-align: right; width: 120px">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  <!-- Processors -->
                  <tr>
                    <td class="font-semibold">Processors</td>
                    <td class="mono" style="font-size: 12px">
                      {{ inspectingVm.vcpus }} vCPU ({{ inspectingVm.sockets || 1 }} sockets, {{ inspectingVm.cores || inspectingVm.vcpus }} cores) [Type: {{ inspectingVm.cpuType || 'host' }}]
                    </td>
                    <td style="text-align: right"><span class="text-muted">-</span></td>
                  </tr>

                  <!-- Memory -->
                  <tr>
                    <td class="font-semibold">Memory</td>
                    <td class="mono" style="font-size: 12px">
                      {{ formatBytes(inspectingVm.memoryMib * 1024 * 1024) }}
                      (Ballooning Min: {{ formatBytes((inspectingVm.balloonMinMib || inspectingVm.memoryMib) * 1024 * 1024) }}, Hugepages: {{ inspectingVm.hugepages || 'off' }})
                    </td>
                    <td style="text-align: right"><span class="text-muted">-</span></td>
                  </tr>

                  <!-- Machine & BIOS -->
                  <tr>
                    <td class="font-semibold">Machine & BIOS</td>
                    <td class="mono" style="font-size: 12px">
                      BIOS: {{ (inspectingVm.biosType || 'seabios').toUpperCase() }} | Machine: {{ inspectingVm.machineType || 'q35' }} | TPM: {{ inspectingVm.tpmVersion || 'none' }}
                    </td>
                    <td style="text-align: right"><span class="text-muted">-</span></td>
                  </tr>

                  <!-- Hard Disks -->
                  <tr v-for="d in inspectingVm.disks" :key="d.id">
                    <td class="font-semibold">
                      Hard Disk (<code class="mono">{{ d.id }}</code>)
                    </td>
                    <td class="mono" style="font-size: 12px">
                      {{ d.diskPath }} ({{ formatBytes(d.sizeBytes) }}, Bus: {{ d.bus }}, Cache: {{ d.cacheMode || 'none' }}, Discard: {{ d.discard ? 'on' : 'off' }}, IOThread: {{ d.iothread ? 'on' : 'off' }})
                    </td>
                    <td style="text-align: right">
                      <span class="badge badge-success" style="font-size: 10px">ACTIVE</span>
                    </td>
                  </tr>

                  <!-- CD/DVD Drive -->
                  <tr>
                    <td class="font-semibold">CD/DVD Drive</td>
                    <td class="mono" style="font-size: 12px">
                      <span v-if="inspectingVm.cdromIsoImage || inspectingVm.isoImage">
                        {{ inspectingVm.cdromIsoImage || inspectingVm.isoImage }}
                      </span>
                      <span v-else class="text-muted">Empty (No media inserted)</span>
                    </td>
                    <td style="text-align: right">
                      <button
                        v-if="inspectingVm.cdromIsoImage || inspectingVm.isoImage"
                        class="btn btn-xs btn-warning"
                        @click="handleEjectMedia"
                      >
                        Eject
                      </button>
                      <button
                        v-else
                        class="btn btn-xs btn-primary"
                        @click="showMediaModal = true"
                      >
                        Insert Media
                      </button>
                    </td>
                  </tr>

                  <!-- Network Devices -->
                  <tr v-for="nic in inspectingVm.networkInterfaces" :key="nic.id">
                    <td class="font-semibold">
                      Network (<code class="mono">{{ nic.id }}</code>)
                    </td>
                    <td class="mono" style="font-size: 12px">
                      Bridge: {{ nic.bridge }}, Model: {{ nic.model }}, MAC: {{ nic.macAddress || 'auto' }}
                      <span v-if="nic.vlanTag" style="color: var(--accent-primary)">, VLAN: {{ nic.vlanTag }}</span>
                    </td>
                    <td style="text-align: right">
                      <span class="badge badge-neutral" style="font-size: 10px">BR: {{ nic.bridge }}</span>
                    </td>
                  </tr>

                  <!-- Display -->
                  <tr>
                    <td class="font-semibold">Display / Graphics</td>
                    <td class="mono" style="font-size: 12px">
                      {{ inspectingVm.displayType || 'virtio-gpu' }} (VNC Display: {{ inspectingVm.vncDisplay || '127.0.0.1:5900' }})
                    </td>
                    <td style="text-align: right"><span class="text-muted">-</span></td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- ========================================== -->
          <!-- TAB 3: CLOUD-INIT -->
          <!-- ========================================== -->
          <div v-else-if="inspectorTab === 'cloudinit'" style="display: flex; flex-direction: column; gap: 14px">
            <div style="font-weight: 600; font-size: 14px">
              Cloud-Init Guest Provisioning (`NoCloud` Automation)
            </div>
            <p class="card-subtitle">
              Automate user provisioning, SSH public keys, and static/DHCP IP network configuration inside Cloud-Init enabled images.
            </p>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 14px">
              <div class="form-group">
                <label>Default Admin User</label>
                <input
                  v-model="ciForm.user"
                  class="input mono"
                  placeholder="admin"
                />
              </div>

              <div class="form-group">
                <label>Password (Optional)</label>
                <input
                  v-model="ciForm.password"
                  type="password"
                  class="input mono"
                  placeholder="••••••••"
                />
              </div>
            </div>

            <div class="form-group">
              <label>SSH Authorized Public Keys</label>
              <textarea
                v-model="ciForm.sshPublicKeys"
                class="input mono"
                rows="3"
                placeholder="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA..."
              ></textarea>
            </div>

            <div class="form-group">
              <label>IP Configuration (CIDR or "dhcp")</label>
              <input
                v-model="ciForm.ipConfig"
                class="input mono"
                placeholder="ip=192.168.1.100/24,gw=192.168.1.1 or dhcp"
              />
            </div>

            <div>
              <button class="btn btn-primary" @click="handleSaveCloudInit">
                Save Cloud-Init Configuration
              </button>
            </div>
          </div>

          <!-- ========================================== -->
          <!-- TAB 4: SNAPSHOTS -->
          <!-- ========================================== -->
          <div v-else-if="inspectorTab === 'snapshots'">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px">
              <div style="font-weight: 600; font-size: 14px">
                Virtual Machine Snapshots
              </div>
              <button class="btn btn-sm btn-primary" @click="openTakeSnapshotModal">
                <MdiIcon :path="mdiPlus" size="14" style="margin-right: 4px" /> Take Snapshot
              </button>
            </div>

            <div v-if="snapshotsList.length > 0" class="table-wrap">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>Snapshot ID</th>
                    <th>VM State</th>
                    <th>RAM Included</th>
                    <th>Created</th>
                    <th>Description</th>
                    <th style="text-align: right">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="s in snapshotsList" :key="s.snapshotId">
                    <td class="mono font-semibold" style="color: var(--accent-primary)">
                      {{ s.snapshotId }}
                    </td>
                    <td>
                      <span class="badge mono" :class="s.vmState === 'RUNNING' ? 'badge-success' : 'badge-neutral'" style="font-size: 10px">
                        {{ s.vmState || 'STOPPED' }}
                      </span>
                    </td>
                    <td class="mono" style="font-size: 11px">
                      {{ s.includeRam ? 'Yes (Live RAM)' : 'No (Disk only)' }}
                    </td>
                    <td class="mono" style="font-size: 11px">
                      {{ s.createTime ? new Date(s.createTime).toLocaleString() : '-' }}
                    </td>
                    <td style="font-size: 12px">
                      {{ s.description || '-' }}
                    </td>
                    <td style="text-align: right">
                      <div style="display: inline-flex; gap: 6px">
                        <button
                          class="btn btn-xs btn-primary"
                          title="Rollback to this snapshot"
                          @click="handleRollbackSnapshot(s)"
                        >
                          <MdiIcon :path="mdiRestore" size="13" style="margin-right: 2px" /> Rollback
                        </button>
                        <button
                          class="btn btn-xs btn-danger"
                          title="Delete snapshot"
                          @click="handleDeleteSnapshot(s)"
                        >
                          <MdiIcon :path="mdiTrashCan" size="13" />
                        </button>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-else style="padding: 24px; text-align: center; color: var(--text-secondary); background: var(--bg-base); border-radius: 6px">
              No snapshots created yet for this virtual machine.
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- TAKE SNAPSHOT MODAL -->
    <!-- ======================================================================= -->
    <div v-if="showTakeSnapshotModal" class="modal-backdrop" @click.self="showTakeSnapshotModal = false">
      <div class="modal-card" style="max-width: 480px">
        <div class="modal-header">
          <h3 class="modal-title">Take Virtual Machine Snapshot</h3>
          <button class="btn btn-sm" @click="showTakeSnapshotModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <form @submit.prevent="handleTakeSnapshot" style="padding: 16px; display: flex; flex-direction: column; gap: 14px">
          <div class="form-group">
            <label>Snapshot Name / ID</label>
            <input v-model="snapshotForm.snapshotId" class="input mono" placeholder="clean-install-v1" required />
          </div>
          <div class="form-group">
            <label>Description (Optional)</label>
            <input v-model="snapshotForm.description" class="input" placeholder="Before major upgrade" />
          </div>
          <label style="display: flex; align-items: center; gap: 8px; font-size: 13px">
            <input v-model="snapshotForm.includeRam" type="checkbox" /> Include live RAM memory state (Live snapshot)
          </label>
          <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 10px">
            <button type="button" class="btn" @click="showTakeSnapshotModal = false">Cancel</button>
            <button type="submit" class="btn btn-primary">Take Snapshot</button>
          </div>
        </form>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- INSERT MEDIA MODAL -->
    <!-- ======================================================================= -->
    <div v-if="showMediaModal" class="modal-backdrop" @click.self="showMediaModal = false">
      <div class="modal-card" style="max-width: 480px">
        <div class="modal-header">
          <h3 class="modal-title">Insert CD/DVD Media (ISO)</h3>
          <button class="btn btn-sm" @click="showMediaModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <form @submit.prevent="handleAttachMedia" style="padding: 16px; display: flex; flex-direction: column; gap: 14px">
          <div class="form-group">
            <label>ISO Image Path</label>
            <input v-model="mediaForm.isoPath" class="input mono" placeholder="/tank/isos/debian-12.iso" required />
          </div>
          <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 10px">
            <button type="button" class="btn" @click="showMediaModal = false">Cancel</button>
            <button type="submit" class="btn btn-primary">Mount Media</button>
          </div>
        </form>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- ADD HARD DISK MODAL -->
    <!-- ======================================================================= -->
    <div v-if="showAddDiskModal" class="modal-backdrop" @click.self="showAddDiskModal = false">
      <div class="modal-card" style="max-width: 500px">
        <div class="modal-header">
          <h3 class="modal-title">Add Virtual Hard Disk</h3>
          <button class="btn btn-sm" @click="showAddDiskModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <form @submit.prevent="handleAddDisk" style="padding: 16px; display: flex; flex-direction: column; gap: 14px">
          <div class="form-grid">
            <div class="form-group">
              <label>Bus Device</label>
              <select v-model="diskForm.bus" class="select mono">
                <option value="scsi">VirtIO SCSI (Recommended)</option>
                <option value="virtio">VirtIO Block</option>
                <option value="sata">SATA</option>
                <option value="nvme">NVMe</option>
              </select>
            </div>
            <div class="form-group">
              <label>Size (GiB)</label>
              <input v-model.number="diskForm.sizeGib" type="number" min="1" class="input mono" required />
            </div>
          </div>
          <div class="form-group">
            <label>Storage Path (ZFS ZVol or QCOW2)</label>
            <input
              v-model="diskForm.diskPath"
              list="zvol-list-add"
              class="input mono"
              placeholder="/dev/zvol/tank/vm-disk1"
            />
            <datalist id="zvol-list-add">
              <option v-for="zpath in availableZvols" :key="zpath" :value="zpath" />
            </datalist>
          </div>
          <div style="display: flex; gap: 16px; font-size: 13px">
            <label style="display: flex; align-items: center; gap: 6px">
              <input v-model="diskForm.discard" type="checkbox" /> Discard / TRIM
            </label>
            <label style="display: flex; align-items: center; gap: 6px">
              <input v-model="diskForm.iothread" type="checkbox" /> IOThread
            </label>
          </div>
          <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 10px">
            <button type="button" class="btn" @click="showAddDiskModal = false">Cancel</button>
            <button type="submit" class="btn btn-primary">Add Disk</button>
          </div>
        </form>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- ADD NETWORK DEVICE MODAL -->
    <!-- ======================================================================= -->
    <div v-if="showAddNicModal" class="modal-backdrop" @click.self="showAddNicModal = false">
      <div class="modal-card" style="max-width: 480px">
        <div class="modal-header">
          <h3 class="modal-title">Add Network Device</h3>
          <button class="btn btn-sm" @click="showAddNicModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <form @submit.prevent="handleAddNic" style="padding: 16px; display: flex; flex-direction: column; gap: 14px">
          <div class="form-group">
            <label>Model</label>
            <select v-model="nicForm.model" class="select mono">
              <option value="virtio">VirtIO (paravirtualized)</option>
              <option value="e1000">Intel E1000</option>
              <option value="rtl8139">Realtek RTL8139</option>
            </select>
          </div>
          <div class="form-group">
            <label>Bridge</label>
            <input v-model="nicForm.bridge" class="input mono" placeholder="virbr0" required />
          </div>
          <div class="form-group">
            <label>VLAN Tag (0 = Untagged)</label>
            <input v-model.number="nicForm.vlanTag" type="number" min="0" max="4094" class="input mono" />
          </div>
          <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 10px">
            <button type="button" class="btn" @click="showAddNicModal = false">Cancel</button>
            <button type="submit" class="btn btn-primary">Add Device</button>
          </div>
        </form>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- CLONE VM MODAL -->
    <!-- ======================================================================= -->
    <div v-if="cloningVm" class="modal-backdrop" @click.self="cloningVm = null">
      <div class="modal-card" style="max-width: 500px">
        <div class="modal-header">
          <h3 class="modal-title">Clone Virtual Machine: {{ cloningVm.vmName }}</h3>
          <button class="btn btn-sm" @click="cloningVm = null">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <form @submit.prevent="handleCloneVm" style="padding: 16px; display: flex; flex-direction: column; gap: 14px">
          <div class="form-group">
            <label>New VM Domain ID</label>
            <input v-model="cloneForm.newId" class="input mono" required />
          </div>
          <div class="form-group">
            <label>New VM Display Name</label>
            <input v-model="cloneForm.newName" class="input" />
          </div>
          <div class="form-group">
            <label>Target ZFS Storage Pool</label>
            <select v-model="cloneForm.targetPool" class="select mono">
              <option v-for="p in availablePools" :key="p" :value="p">{{ p }}</option>
            </select>
          </div>
          <label style="display: flex; align-items: center; gap: 8px; font-size: 13px">
            <input v-model="cloneForm.linkedClone" type="checkbox" />
            Linked Clone (Instant zero-copy snapshot clone)
          </label>
          <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 10px">
            <button type="button" class="btn" @click="cloningVm = null">Cancel</button>
            <button type="submit" class="btn btn-primary">Execute Clone</button>
          </div>
        </form>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- PROXMOX-STYLE MULTI-STEP CREATION WIZARD -->
    <!-- ======================================================================= -->
    <div v-if="showCreateWizard" class="modal-backdrop" @click.self="showCreateWizard = false">
      <div class="modal-card" style="max-width: 780px; width: 95vw; display: flex; flex-direction: column">
        <!-- Wizard Header -->
        <div class="modal-header">
          <div>
            <h3 class="modal-title">Create Virtual Machine — Proxmox VE KVM Wizard</h3>
            <div class="text-muted" style="font-size: 12px; margin-top: 2px">
              Step {{ wizardStep }} of 7:
              <span v-if="wizardStep === 1">General Settings</span>
              <span v-else-if="wizardStep === 2">System & Firmware (UEFI / TPM)</span>
              <span v-else-if="wizardStep === 3">Storage Disks (ZFS ZVol)</span>
              <span v-else-if="wizardStep === 4">CPU & Topology</span>
              <span v-else-if="wizardStep === 5">Memory & Ballooning</span>
              <span v-else-if="wizardStep === 6">Network Interfaces</span>
              <span v-else-if="wizardStep === 7">Cloud-Init & Confirm</span>
            </div>
          </div>
          <button class="btn btn-sm" @click="showCreateWizard = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>

        <!-- Wizard Step Indicators -->
        <div style="display: flex; border-bottom: 1px solid var(--border-subtle); background: var(--bg-base)">
          <div
            v-for="s in [1, 2, 3, 4, 5, 6, 7]"
            :key="s"
            style="flex: 1; padding: 10px 4px; text-align: center; font-size: 11px; font-weight: 600; cursor: pointer"
            :style="{
              color: wizardStep === s ? 'var(--accent-primary)' : 'var(--text-secondary)',
              borderBottom: wizardStep === s ? '2px solid var(--accent-primary)' : '2px solid transparent',
            }"
            @click="wizardStep = s"
          >
            {{ ['General', 'System', 'Disks', 'CPU', 'RAM', 'Network', 'Confirm'][s - 1] }}
          </div>
        </div>

        <!-- Wizard Step Body -->
        <div style="padding: 20px; overflow-y: auto; max-height: 60vh">
          <!-- Step 1: General -->
          <div v-if="wizardStep === 1" class="form-grid">
            <div class="form-group">
              <label>VM Domain ID (`virtual_machine_id`)</label>
              <input v-model="wizardForm.vmId" class="input mono" placeholder="1761411800123456789" required />
              <small style="color: var(--text-muted); font-size: 11px">Unique nanosecond timestamp ID</small>
            </div>
            <div class="form-group">
              <label>Display Name</label>
              <input v-model="wizardForm.vmName" class="input" placeholder="Ubuntu Production Server" />
            </div>
            <div class="form-group">
              <label>Operating System</label>
              <select v-model="wizardForm.osVariant" class="select mono">
                <option value="debian12">Debian 12 (Bookworm)</option>
                <option value="ubuntu24.04">Ubuntu 24.04 LTS</option>
                <option value="almalinux9">AlmaLinux / RHEL 9</option>
                <option value="freebsd14.0">FreeBSD 14 / OPNsense</option>
                <option value="win2k22">Windows Server 2022 / Windows 11</option>
              </select>
            </div>
            <div class="form-group">
              <label>Description</label>
              <input v-model="wizardForm.description" class="input" placeholder="Application node" />
            </div>
            <div class="form-group" style="grid-column: span 2">
              <label style="display: flex; align-items: center; gap: 8px; font-size: 13px">
                <input v-model="wizardForm.autostart" type="checkbox" /> Autostart domain when host boots
              </label>
            </div>
          </div>

          <!-- Step 2: System -->
          <div v-if="wizardStep === 2" class="form-grid">
            <div class="form-group">
              <label>Machine Type</label>
              <select v-model="wizardForm.machineType" class="select mono">
                <option value="q35">q35 (Modern PCIe Architecture)</option>
                <option value="i440fx">i440fx (Legacy PCI)</option>
              </select>
            </div>
            <div class="form-group">
              <label>BIOS / Firmware</label>
              <select v-model="wizardForm.biosType" class="select mono">
                <option value="ovmf">OVMF (UEFI - Modern, Secure Boot)</option>
                <option value="seabios">SeaBIOS (Legacy BIOS)</option>
              </select>
            </div>
            <div class="form-group">
              <label>vTPM 2.0 Security</label>
              <select v-model="wizardForm.tpmVersion" class="select mono">
                <option value="v2.0">TPM 2.0 (Enabled via swtpm - Win 11)</option>
                <option value="none">Disabled</option>
              </select>
            </div>
            <div class="form-group">
              <label>Secure Boot</label>
              <select v-model="wizardForm.secureBoot" class="select mono">
                <option :value="true">Enabled</option>
                <option :value="false">Disabled</option>
              </select>
            </div>
          </div>

          <!-- Step 3: Disks -->
          <div v-if="wizardStep === 3" class="form-grid">
            <div class="form-group">
              <label>Bus Interface</label>
              <select v-model="wizardForm.diskBus" class="select mono">
                <option value="scsi">VirtIO SCSI (Recommended)</option>
                <option value="virtio">VirtIO Block</option>
                <option value="sata">SATA</option>
                <option value="nvme">NVMe</option>
              </select>
            </div>
            <div class="form-group">
              <label>Disk Size (GiB)</label>
              <input v-model.number="wizardForm.diskSizeGib" type="number" min="1" class="input mono" required />
            </div>
            <div class="form-group" style="grid-column: span 2">
              <label>Backing Storage Path (Raw image, QCOW2 or ZFS ZVol)</label>
              <input
                v-model="wizardForm.diskPath"
                list="zvol-list-wiz"
                class="input mono"
                :placeholder="suggestedDiskPath"
              />
              <datalist id="zvol-list-wiz">
                <option v-for="zpath in availableZvols" :key="zpath" :value="zpath" />
              </datalist>
              <small style="color: var(--text-muted); font-size: 11px">Standard disk naming: <code class="mono">{{ suggestedDiskName }}</code></small>
            </div>
            <div class="form-group">
              <label style="display: flex; align-items: center; gap: 8px; font-size: 13px">
                <input v-model="wizardForm.diskDiscard" type="checkbox" /> Discard / TRIM
              </label>
            </div>
            <div class="form-group">
              <label style="display: flex; align-items: center; gap: 8px; font-size: 13px">
                <input v-model="wizardForm.diskIothread" type="checkbox" /> Dedicated IOThread
              </label>
            </div>
          </div>

          <!-- Step 4: CPU -->
          <div v-if="wizardStep === 4" class="form-grid">
            <div class="form-group">
              <label>Sockets</label>
              <input v-model.number="wizardForm.sockets" type="number" min="1" class="input mono" required />
            </div>
            <div class="form-group">
              <label>Cores</label>
              <input v-model.number="wizardForm.cores" type="number" min="1" class="input mono" required />
            </div>
            <div class="form-group">
              <label>CPU Type / Model</label>
              <select v-model="wizardForm.cpuType" class="select mono">
                <option value="host">host (Pass-through host CPU features)</option>
                <option value="kvm64">kvm64 (Safe standard KVM)</option>
                <option value="x86-64-v2-AES">x86-64-v2-AES</option>
                <option value="EPYC">EPYC</option>
              </select>
            </div>
            <div class="form-group">
              <label>NUMA Topology</label>
              <select v-model="wizardForm.numa" class="select mono">
                <option :value="false">Disabled</option>
                <option :value="true">Enabled</option>
              </select>
            </div>
          </div>

          <!-- Step 5: Memory -->
          <div v-if="wizardStep === 5" class="form-grid">
            <div class="form-group">
              <label>Memory (MiB)</label>
              <input v-model.number="wizardForm.memoryMib" type="number" min="512" step="256" class="input mono" required />
            </div>
            <div class="form-group">
              <label>Minimum Memory (Ballooning MiB)</label>
              <input v-model.number="wizardForm.balloonMinMib" type="number" min="256" step="256" class="input mono" required />
            </div>
          </div>

          <!-- Step 6: Network -->
          <div v-if="wizardStep === 6" class="form-grid">
            <div class="form-group">
              <label>Model</label>
              <select v-model="wizardForm.netModel" class="select mono">
                <option value="virtio">VirtIO (paravirtualized)</option>
                <option value="e1000">Intel E1000</option>
                <option value="rtl8139">Realtek RTL8139</option>
              </select>
            </div>
            <div class="form-group">
              <label>Bridge</label>
              <input v-model="wizardForm.netBridge" class="input mono" placeholder="virbr0" required />
            </div>
            <div class="form-group">
              <label>VLAN Tag (0 = Untagged)</label>
              <input v-model.number="wizardForm.vlanTag" type="number" min="0" max="4094" class="input mono" />
            </div>
          </div>

          <!-- Step 7: Cloud-Init & Confirm -->
          <div v-if="wizardStep === 7" style="display: flex; flex-direction: column; gap: 14px">
            <div class="form-grid">
              <div class="form-group">
                <label>Cloud-Init User</label>
                <input v-model="wizardForm.ciUser" class="input mono" />
              </div>
              <div class="form-group">
                <label>Password</label>
                <input v-model="wizardForm.ciPassword" type="password" class="input mono" />
              </div>
              <div class="form-group" style="grid-column: span 2">
                <label>SSH Public Keys</label>
                <textarea v-model="wizardForm.ciSshKeys" class="input mono" rows="2" placeholder="ssh-ed25519 ..."></textarea>
              </div>
            </div>

            <!-- Confirmation Summary Card -->
            <div style="background: var(--bg-base); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 14px; font-size: 12px">
              <div style="font-weight: 700; margin-bottom: 8px">Summary Preview</div>
              <div><strong>VM Domain:</strong> <span class="mono">{{ wizardForm.vmId }}</span></div>
              <div><strong>Topology:</strong> <span class="mono">{{ wizardForm.sockets * wizardForm.cores }} vCPUs ({{ wizardForm.sockets }}s/{{ wizardForm.cores }}c) [{{ wizardForm.cpuType }}]</span></div>
              <div><strong>Memory:</strong> <span class="mono">{{ formatBytes(wizardForm.memoryMib * 1024 * 1024) }} (Balloon: {{ formatBytes(wizardForm.balloonMinMib * 1024 * 1024) }})</span></div>
              <div><strong>System:</strong> <span class="mono">{{ wizardForm.machineType }} / {{ wizardForm.biosType.toUpperCase() }} (TPM: {{ wizardForm.tpmVersion }})</span></div>
              <div><strong>Disk:</strong> <span class="mono">{{ wizardForm.diskBus }} ({{ wizardForm.diskSizeGib }} GiB)</span></div>
              <div><strong>Network:</strong> <span class="mono">{{ wizardForm.netModel }} on {{ wizardForm.netBridge }}</span></div>
            </div>
          </div>
        </div>

        <!-- Wizard Navigation Footer -->
        <div style="padding: 14px 20px; border-top: 1px solid var(--border-subtle); display: flex; justify-content: space-between; align-items: center">
          <button
            type="button"
            class="btn"
            :disabled="wizardStep === 1"
            @click="wizardStep--"
          >
            Previous
          </button>

          <div style="display: flex; gap: 10px">
            <button
              v-if="wizardStep < 7"
              type="button"
              class="btn btn-primary"
              @click="wizardStep++"
            >
              Next
            </button>
            <button
              v-else
              type="button"
              class="btn btn-primary"
              @click="handleWizardSubmit"
            >
              Deploy Virtual Machine
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- NOVNC MODAL COMPONENT -->
    <!-- ======================================================================= -->
    <NoVncModal
      v-if="activeConsoleSession"
      :session="activeConsoleSession"
      @close="activeConsoleSession = null"
    />
  </div>
</template>

<style scoped>
.search-input-wrap {
  position: relative;
  display: flex;
  align-items: center;
}

.search-icon {
  position: absolute;
  left: 9px;
  color: var(--text-muted);
  pointer-events: none;
}

.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(4px);
}

.modal-card {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.5);
}

.modal-header {
  padding: 14px 20px;
  border-bottom: 1px solid var(--border-subtle);
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.modal-title {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  color: var(--text-primary);
}

.modal-nav-tabs {
  display: flex;
  border-bottom: 1px solid var(--border-subtle);
  background: var(--bg-base);
  padding: 0 16px;
  gap: 8px;
}

.modal-nav-tab {
  display: flex;
  align-items: center;
  padding: 10px 14px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  cursor: pointer;
  transition: all 0.15s ease;
}

.modal-nav-tab:hover {
  color: var(--text-primary);
}

.modal-nav-tab.active {
  color: var(--accent-primary);
  border-bottom-color: var(--accent-primary);
}
</style>
