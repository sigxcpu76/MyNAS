<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { services } from '../../container';
import {
  formatBytes,
  formatRelativeTime,
  formatAbsoluteTime,
  type NetworkInterfaceTelemetry,
  type StorageImage,
  type VirtualMachine,
  type VirtualMachineCloudInit,
  type VirtualMachineConsoleSession,
  type VirtualMachineDisk,
  type VirtualMachineNetworkInterface,
  type VirtualMachineSnapshot,
} from '../../domain/models';
import { askModal, confirmModal, nasState, notify, refreshIsos as storeRefreshIsos } from '../store';
import { confirmDelete } from '../modal';
import NoVncModal from '../components/NoVncModal.vue';
import MdiIcon from '../components/MdiIcon.vue';
import SkeletonBox from '../components/SkeletonBox.vue';
import SortableTh from '../components/SortableTh.vue';
import FileDropzone from '../components/FileDropzone.vue';
import PathSelector from '../components/PathSelector.vue';
import { useTableSort } from '../composables/useTableSort';
import { generateNanosecondId, generateVMDiskFilePath, generateVMDiskName, generateZvolDiskPath, toZfsSafeSlug } from '../../domain/id';
import {
  mdiAlertCircleOutline,
  mdiClose,
  mdiCog,
  mdiConsole,
  mdiContentCopy,
  mdiDatabase,
  mdiDisc,
  mdiEye,
  mdiHarddisk,
  mdiInformationOutline,
  mdiMagnify,
  mdiNetwork,
  mdiPause,
  mdiPencil,
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
  mdiUpload,
} from '@mdi/js';

// State
const searchQuery = ref('');
const activeConsoleSession = ref<VirtualMachineConsoleSession | null>(null);
const inspectingVm = ref<VirtualMachine | null>(null);
const inspectorTab = ref<'summary' | 'hardware' | 'cloudinit' | 'snapshots'>('summary');
const cloningVm = ref<VirtualMachine | null>(null);
const showCreateWizard = ref(false);

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

// Edit Hardware Modals
const showEditCpuModal = ref(false);
const editCpuForm = reactive({
  vcpus: 2,
  sockets: 1,
  cores: 2,
  cpuType: 'host',
  numa: false,
});

const showEditMemoryModal = ref(false);
const editMemoryForm = reactive({
  memoryMib: 4096,
  hugepages: 'off',
});

const showEditPlatformModal = ref(false);
const editPlatformForm = reactive({
  machineType: 'i440fx',
  biosType: 'seabios',
  tpmVersion: 'none',
  secureBoot: false,
});

const showEditDisplayModal = ref(false);
const editDisplayForm = reactive({
  displayType: 'virtio-gpu',
});

const showEditDiskModal = ref(false);
const editingDiskId = ref('');
const editDiskForm = reactive({
  bus: 'scsi',
  diskPath: '',
  sizeGib: 32,
  cacheMode: 'none',
  discard: true,
  iothread: true,
});

const showEditNicModal = ref(false);
const editingNicId = ref('');
const editNicForm = reactive({
  model: 'virtio',
  bridge: 'br0',
  vlanTag: 0,
  macAddress: '',
  firewall: true,
});

// Modal error & saving states
const isSavingDisk = ref(false);
const editDiskError = ref('');
const isSavingNic = ref(false);
const editNicError = ref('');
const isSavingCpu = ref(false);
const editCpuError = ref('');
const isSavingMemory = ref(false);
const editMemoryError = ref('');
const isSavingPlatform = ref(false);
const editPlatformError = ref('');
const isSavingDisplay = ref(false);
const editDisplayError = ref('');

// Clone Form
const cloneForm = reactive({
  newId: '',
  newName: '',
  targetPool: 'tank',
  linkedClone: true,
});

export interface WizardDisk {
  id: string;
  bus: string;
  diskPath: string;
  sizeGib: number;
  cacheMode: string;
  discard: boolean;
  iothread: boolean;
}

export interface WizardNic {
  id: string;
  model: string;
  bridge: string;
  vlanTag: number;
  firewall: boolean;
}

// Create VM Hardware Form
const wizardForm = reactive({
  vmId: '',
  vmName: '',
  osVariant: 'debian12',
  description: '',
  autostart: true,
  startupDelaySeconds: 0,

  machineType: 'q35',
  biosType: 'seabios',
  tpmVersion: 'none',
  secureBoot: false,
  bootIso: '',

  sockets: 1,
  cores: 2,
  cpuType: 'host',
  numa: false,

  memoryMib: 4096,
  hugepages: 'off',

  displayType: 'virtio-gpu',

  disks: [] as WizardDisk[],
  networkInterfaces: [] as WizardNic[],

  showCloudInit: false,
  ciUser: 'admin',
  ciPassword: '',
  ciSshKeys: '',
  ciIpConfig: 'dhcp',
});

const wizardEditModal = ref<'cpu' | 'memory' | 'platform' | 'display' | 'disk' | 'nic' | null>(null);
const editingWizardDiskIndex = ref<number>(-1);
const editingWizardNicIndex = ref<number>(-1);

const editingDiskBuffer = reactive<WizardDisk>({
  id: 'sda',
  bus: 'scsi',
  diskPath: '',
  sizeGib: 32,
  cacheMode: 'none',
  discard: true,
  iothread: false,
});

const editingNicBuffer = reactive<WizardNic>({
  id: 'net0',
  model: 'virtio',
  bridge: 'br0',
  vlanTag: 0,
  firewall: true,
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
      visibleGuestIps(vm).some((ip) => ip.includes(q))
  );
});

/** IPv6 link-local addresses (fe80::/10) are noise on IPv4 LANs, so hide them. */
function isIpv6LinkLocal(ip: string): boolean {
  return /^fe[89ab][0-9a-f]:/i.test(ip.trim());
}

function visibleGuestIps(vm: { guestIpAddresses?: string[] }): string[] {
  return (vm.guestIpAddresses || []).filter((ip) => !isIpv6LinkLocal(ip));
}

const vmSort = useTableSort(filteredVms, {
  name: (vm) => vm.vmName,
  vcpus: (vm) => vm.vcpus,
  memory: (vm) => vm.memoryMib,
  ip: (vm) => visibleGuestIps(vm)[0],
});

const isoSort = useTableSort(() => nasState.isos, {
  name: (iso) => iso.name,
  size: (iso) => iso.sizeBytes,
  modTime: (iso) => (iso.modTime ? new Date(iso.modTime) : null),
});

const snapshotSort = useTableSort(snapshotsList, {
  id: (s) => s.snapshotId,
  vmState: (s) => s.vmState || 'STOPPED',
  includeRam: (s) => !!s.includeRam,
  created: (s) => (s.createTime ? new Date(s.createTime) : null),
  description: (s) => s.description,
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
  return generateVMDiskName(wizardForm.vmId || wizardForm.vmName || 'vm', 0);
});

const suggestedDiskPath = computed(() => {
  return generateZvolDiskPath(mainPool.value, wizardForm.vmId || wizardForm.vmName || 'vm', 0);
});

const defaultBridge = computed<string>(() => {
  const net = nasState.systemTelemetry?.network;
  if (!net || !Array.isArray(net)) return 'br0';
  const br = net.find((n: NetworkInterfaceTelemetry) => n.name === 'br0' || n.name === 'vmbr0');
  if (br) return br.name;
  const def = net.find((n: NetworkInterfaceTelemetry) => n.isDefault);
  if (def) return def.name;
  const firstEth = net.find(
    (n: NetworkInterfaceTelemetry) =>
      !n.name.startsWith('lo') &&
      !n.name.startsWith('docker') &&
      !n.name.startsWith('br-') &&
      !n.name.startsWith('veth') &&
      !n.name.startsWith('lxc')
  );
  if (firstEth) return firstEth.name;
  return 'br0';
});

const socketOptions = [1, 2, 3, 4];

const hostLogicalCores = computed<number>(() => {
  const n = nasState.systemTelemetry?.cpu?.logicalCores;
  return n && n > 0 ? Math.min(n, 128) : 16;
});

function coreOptionsFor(current: number): number[] {
  const opts = Array.from({ length: hostLogicalCores.value }, (_, i) => i + 1);
  if (current && !opts.includes(current)) opts.push(current);
  return opts.sort((a, b) => a - b);
}

interface BridgeOption {
  name: string;
  label: string;
}

const hostBridgeInterfaces = computed<BridgeOption[]>(() => {
  const net = nasState.systemTelemetry?.network;
  if (!net || !Array.isArray(net)) return [];
  return net
    .filter(
      (n: NetworkInterfaceTelemetry) =>
        !n.name.startsWith('lo') &&
        !n.name.startsWith('docker') &&
        !n.name.startsWith('br-') &&
        !n.name.startsWith('veth') &&
        !n.name.startsWith('vnet') &&
        !n.name.startsWith('tap') &&
        !n.name.startsWith('lxc')
    )
    .map((n: NetworkInterfaceTelemetry) => {
      const ip = n.ipAddresses?.find((a) => !a.includes(':')) || n.ipAddresses?.[0];
      const parts = [n.isUp ? 'up' : 'down'];
      if (ip) parts.push(ip);
      if (n.isDefault) parts.push('default route');
      return { name: n.name, label: `${n.name} (${parts.join(', ')})` };
    });
});

function bridgeOptionsFor(current: string): BridgeOption[] {
  const opts = [...hostBridgeInterfaces.value];
  if (current && !opts.some((o) => o.name === current)) {
    opts.unshift({ name: current, label: `${current} (not detected on host)` });
  }
  return opts;
}

function onVmNameInput() {
  const oldId = wizardForm.vmId;
  const newId = toZfsSafeSlug(wizardForm.vmName);
  wizardForm.vmId = newId;

  // Auto-update default disk paths if they followed the previous auto-generated name
  wizardForm.disks.forEach((d, i) => {
    const oldExpected = generateZvolDiskPath(mainPool.value, oldId, i);
    if (!d.diskPath || d.diskPath === oldExpected) {
      d.diskPath = generateZvolDiskPath(mainPool.value, newId, i);
    }
  });
}

function onOsVariantChange() {
  if (wizardForm.osVariant === 'win2k22') {
    wizardForm.machineType = 'q35';
    wizardForm.biosType = 'ovmf';
    wizardForm.tpmVersion = 'v2.0';
    wizardForm.secureBoot = true;
  } else if (wizardForm.osVariant.startsWith('freebsd')) {
    wizardForm.machineType = 'q35';
    wizardForm.biosType = 'ovmf';
    wizardForm.tpmVersion = 'none';
    wizardForm.secureBoot = false;
  } else {
    wizardForm.machineType = 'q35';
    wizardForm.biosType = 'seabios';
    wizardForm.tpmVersion = 'none';
    wizardForm.secureBoot = false;
  }
}

function openCreateWizard() {
  wizardForm.vmName = 'Debian Server';
  wizardForm.vmId = toZfsSafeSlug(wizardForm.vmName);
  wizardForm.osVariant = 'debian12';
  wizardForm.description = '';
  wizardForm.autostart = true;
  wizardForm.startupDelaySeconds = 0;
  wizardForm.machineType = 'q35';
  wizardForm.biosType = 'seabios';
  wizardForm.tpmVersion = 'none';
  wizardForm.secureBoot = false;
  wizardForm.bootIso = nasState.isos.length > 0 ? nasState.isos[0].path : '';
  wizardForm.sockets = 1;
  wizardForm.cores = 2;
  wizardForm.cpuType = 'host';
  wizardForm.numa = false;
  wizardForm.memoryMib = 4096;
  wizardForm.hugepages = 'off';
  wizardForm.displayType = 'virtio-gpu';

  wizardForm.disks = [
    {
      id: 'sda',
      bus: 'scsi',
      diskPath: generateZvolDiskPath(mainPool.value, wizardForm.vmId, 0),
      sizeGib: 32,
      cacheMode: 'none',
      discard: true,
      iothread: false,
    },
  ];

  wizardForm.networkInterfaces = [
    {
      id: 'net0',
      model: 'virtio',
      bridge: defaultBridge.value || 'br0',
      vlanTag: 0,
      firewall: true,
    },
  ];

  wizardForm.showCloudInit = false;
  wizardForm.ciUser = 'admin';
  wizardForm.ciPassword = '';
  wizardForm.ciSshKeys = '';
  wizardForm.ciIpConfig = 'dhcp';

  wizardEditModal.value = null;
  showCreateWizard.value = true;
}

function addWizardDisk() {
  const idx = wizardForm.disks.length;
  const letter = String.fromCharCode(97 + (idx % 26));
  wizardForm.disks.push({
    id: `sd${letter}`,
    bus: 'scsi',
    diskPath: generateZvolDiskPath(mainPool.value, wizardForm.vmId || 'vm', idx),
    sizeGib: 32,
    cacheMode: 'none',
    discard: true,
    iothread: false,
  });
}

function removeWizardDisk(index: number) {
  wizardForm.disks.splice(index, 1);
}

function openEditWizardDisk(index: number) {
  editingWizardDiskIndex.value = index;
  const d = wizardForm.disks[index];
  Object.assign(editingDiskBuffer, d);
  wizardEditModal.value = 'disk';
}

function saveEditWizardDisk() {
  if (editingWizardDiskIndex.value >= 0 && editingWizardDiskIndex.value < wizardForm.disks.length) {
    Object.assign(wizardForm.disks[editingWizardDiskIndex.value], editingDiskBuffer);
  }
  wizardEditModal.value = null;
}

function addWizardNic() {
  const idx = wizardForm.networkInterfaces.length;
  wizardForm.networkInterfaces.push({
    id: `net${idx}`,
    model: 'virtio',
    bridge: defaultBridge.value || 'br0',
    vlanTag: 0,
    firewall: true,
  });
}

function removeWizardNic(index: number) {
  wizardForm.networkInterfaces.splice(index, 1);
}

function openEditWizardNic(index: number) {
  editingWizardNicIndex.value = index;
  const nic = wizardForm.networkInterfaces[index];
  Object.assign(editingNicBuffer, nic);
  wizardEditModal.value = 'nic';
}

function saveEditWizardNic() {
  if (editingWizardNicIndex.value >= 0 && editingWizardNicIndex.value < wizardForm.networkInterfaces.length) {
    Object.assign(wizardForm.networkInterfaces[editingWizardNicIndex.value], editingNicBuffer);
  }
  wizardEditModal.value = null;
}

function openWizardEdit(component: 'cpu' | 'memory' | 'platform' | 'display') {
  wizardEditModal.value = component;
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

const transitioningVms = reactive<Record<string, 'starting' | 'stopping' | 'rebooting'>>({});
let pollTimer: ReturnType<typeof setInterval> | null = null;
let fastPollTimer: ReturnType<typeof setTimeout> | null = null;

function triggerFastPolling(attempts = 20) {
  if (fastPollTimer) {
    clearTimeout(fastPollTimer);
    fastPollTimer = null;
  }
  let count = 0;
  const poll = async () => {
    count++;
    await refreshVms(true);
    if (Object.keys(transitioningVms).length > 0 && count < attempts) {
      fastPollTimer = setTimeout(poll, 1000);
    }
  };
  fastPollTimer = setTimeout(poll, 1000);
}

// Lifecycle
onMounted(async () => {
  await Promise.all([refreshVms(), refreshIsos()]);
  pollTimer = setInterval(async () => {
    await refreshVms(true);
  }, 3000);
});

onUnmounted(() => {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
  if (fastPollTimer) {
    clearTimeout(fastPollTimer);
    fastPollTimer = null;
  }
});

async function refreshVms(silent: boolean | Event = false) {
  const isSilent = typeof silent === 'boolean' ? silent : false;
  try {
    const list = await services.kvm.listVirtualMachines();
    nasState.vms = list;

    // Check if any transitioning VM reached its expected state
    for (const [name, target] of Object.entries(transitioningVms)) {
      const found = list.find((v) => v.name === name);
      if (!found) {
        delete transitioningVms[name];
      } else if (target === 'stopping' && found.state === 'STOPPED') {
        delete transitioningVms[name];
      } else if (target === 'starting' && found.state === 'RUNNING') {
        delete transitioningVms[name];
      }
    }

    if (inspectingVm.value) {
      const updated = list.find((v) => v.name === inspectingVm.value?.name);
      if (updated) {
        inspectingVm.value = updated;
        if (inspectorTab.value === 'snapshots') {
          await loadSnapshots(updated.name);
        }
      }
    }
  } catch (err: any) {
    if (!isSilent) {
      notify('error', err?.message || 'Failed to refresh virtual machines');
    }
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
async function handlePowerAction(
  vm: VirtualMachine,
  action: 'start' | 'stop' | 'reboot' | 'reset' | 'pause' | 'resume',
  event?: MouseEvent
) {
  try {
    if (action === 'start') {
      transitioningVms[vm.name] = 'starting';
      triggerFastPolling();
      await services.kvm.startVirtualMachine(vm.name);
      notify('success', `Started VM ${vm.vmName}`);
    } else if (action === 'stop') {
      let force = false;
      if (event && (event.shiftKey || event.altKey)) {
        force = true;
      } else {
        const res = await askModal({
          title: `Shutdown VM: ${vm.vmName}`,
          message: `Are you sure you want to shut down "${vm.vmName}"?`,
          details: 'ACPI shutdown requests the guest operating system to flush filesystem buffers and power off cleanly.',
          confirmText: 'Shutdown',
          tone: 'warning',
          checkbox: {
            label: 'Force shutdown immediately (virsh destroy)',
            hint: 'Halts the VM process immediately. Use if the guest OS is hung, stuck, or unresponsive.',
            checked: false,
          },
        });
        if (!res.confirmed) return;
        force = Boolean(res.checkboxChecked);
      }
      transitioningVms[vm.name] = 'stopping';
      triggerFastPolling();
      await services.kvm.stopVirtualMachine(vm.name, force);
      notify('success', `${force ? 'Force stopped' : 'Shut down'} VM ${vm.vmName}`);
    } else if (action === 'reboot') {
      let force = false;
      if (event && (event.shiftKey || event.altKey)) {
        force = true;
      } else {
        const res = await askModal({
          title: `Reboot VM: ${vm.vmName}`,
          message: `Are you sure you want to reboot "${vm.vmName}"?`,
          details: 'ACPI reboot sends a restart signal to the guest operating system.',
          confirmText: 'Reboot',
          tone: 'warning',
          checkbox: {
            label: 'Force reset immediately (virsh reset)',
            hint: 'Hard resets / power-cycles the virtual motherboard without waiting for guest OS response.',
            checked: false,
          },
        });
        if (!res.confirmed) return;
        force = Boolean(res.checkboxChecked);
      }
      transitioningVms[vm.name] = 'rebooting';
      triggerFastPolling();
      await services.kvm.rebootVirtualMachine(vm.name, force);
      notify('success', `${force ? 'Force reset' : 'Rebooted'} VM ${vm.vmName}`);
    } else if (action === 'reset') {
      transitioningVms[vm.name] = 'rebooting';
      triggerFastPolling();
      await services.kvm.resetVirtualMachine(vm.name);
      notify('success', `Hard reset VM ${vm.vmName}`);
    } else if (action === 'pause') {
      await services.kvm.pauseVirtualMachine(vm.name);
      notify('success', `Paused VM ${vm.vmName}`);
    } else if (action === 'resume') {
      await services.kvm.resumeVirtualMachine(vm.name);
      notify('success', `Resumed VM ${vm.vmName}`);
    }
    await refreshVms(true);
  } catch (err: any) {
    delete transitioningVms[vm.name];
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
  const res = await confirmDelete({
    kind: 'Snapshot',
    name: snap.snapshotId,
    details: 'This snapshot will be permanently removed from the virtual machine.',
    confirmText: 'Delete Snapshot',
  });
  if (!res.confirmed) return;

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

// ISO Repository Actions
const isoFileInput = ref<HTMLInputElement | null>(null);
const isoDropzoneRef = ref<any>(null);
const isUploadingIso = ref(false);
const isoUploadProgress = ref<{ percent: number; loaded: number; total: number; filename: string } | null>(null);

function triggerIsoUpload() {
  if (isoDropzoneRef.value) {
    isoDropzoneRef.value.openFileDialog();
  } else {
    isoFileInput.value?.click();
  }
}

async function refreshIsos() {
  try {
    await storeRefreshIsos();
  } catch (err: any) {
    console.error('Failed to refresh ISOs:', err);
  }
}

async function uploadIsoFile(file: File) {
  if (!file) return;

  isUploadingIso.value = true;
  isoUploadProgress.value = {
    percent: 0,
    loaded: 0,
    total: file.size,
    filename: file.name,
  };

  try {
    const uploaded = await services.kvm.uploadIso(file, (p) => {
      isoUploadProgress.value = {
        percent: p.percent,
        loaded: p.loaded,
        total: p.total,
        filename: file.name,
      };
    });
    notify('success', `Uploaded ISO ${uploaded.name} (${uploaded.sizeFormatted})`);
    await refreshIsos();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to upload ISO');
  } finally {
    isUploadingIso.value = false;
    isoUploadProgress.value = null;
    if (isoFileInput.value) isoFileInput.value.value = '';
  }
}

async function handleUploadIso(e: Event) {
  const target = e.target as HTMLInputElement;
  const file = target.files?.[0];
  if (file) {
    await uploadIsoFile(file);
  }
}

function handleInvalidIsoFile(filename: string, reason: string) {
  notify('error', reason);
}

async function handleDeleteIso(iso: StorageImage) {
  const res = await confirmDelete({
    kind: 'ISO',
    name: iso.name,
    details: `This will permanently remove ${iso.path} (${iso.sizeFormatted}) from the main pool.`,
    confirmText: 'Delete ISO',
  });
  if (!res.confirmed) return;

  try {
    await services.kvm.deleteIso(iso.name);
    notify('info', `Deleted ISO ${iso.name}`);
    await refreshIsos();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete ISO');
  }
}

function copyIsoPath(path: string) {
  navigator.clipboard.writeText(path);
  notify('info', `Copied path to clipboard: ${path}`);
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

// Edit / Detach Disks
function openEditDiskModal(disk: VirtualMachineDisk) {
  if (!inspectingVm.value) return;
  editingDiskId.value = disk.id;
  editDiskError.value = '';
  isSavingDisk.value = false;
  editDiskForm.bus = disk.bus || 'scsi';
  editDiskForm.diskPath = disk.diskPath || '';
  editDiskForm.sizeGib = disk.sizeBytes ? Math.round(disk.sizeBytes / (1024 * 1024 * 1024)) : 32;
  editDiskForm.cacheMode = disk.cacheMode || 'none';
  editDiskForm.discard = disk.discard ?? true;
  editDiskForm.iothread = disk.iothread ?? true;
  showEditDiskModal.value = true;
}

async function handleSaveEditDisk() {
  if (!inspectingVm.value) return;
  editDiskError.value = '';
  const currentDisks = inspectingVm.value.disks ? [...inspectingVm.value.disks] : [];
  const idx = currentDisks.findIndex((d) => d.id === editingDiskId.value);
  if (idx === -1) {
    editDiskError.value = `Selected disk "${editingDiskId.value}" not found on VM "${inspectingVm.value.vmName}"`;
    notify('error', editDiskError.value);
    return;
  }
  const updatedDisk: VirtualMachineDisk = {
    ...currentDisks[idx],
    bus: editDiskForm.bus,
    diskPath: editDiskForm.diskPath.trim(),
    sizeBytes: editDiskForm.sizeGib * 1024 * 1024 * 1024,
    cacheMode: editDiskForm.cacheMode,
    discard: editDiskForm.discard,
    iothread: editDiskForm.iothread,
  };
  currentDisks[idx] = updatedDisk;

  isSavingDisk.value = true;
  try {
    await services.kvm.updateVirtualMachine(
      inspectingVm.value.name,
      { disks: currentDisks },
      ['disks']
    );
    notify('success', `Updated hard disk ${editingDiskId.value}`);
    showEditDiskModal.value = false;
    await refreshVms();
  } catch (err: any) {
    const msg = err?.message || 'Failed to update hard disk';
    editDiskError.value = msg;
    notify('error', msg);
  } finally {
    isSavingDisk.value = false;
  }
}

async function handleDeleteDisk(disk: VirtualMachineDisk) {
  if (!inspectingVm.value) return;
  const ok = window.confirm(`Are you sure you want to detach disk ${disk.id} (${disk.diskPath}) from ${inspectingVm.value.vmName}? The underlying storage volume will not be destroyed.`);
  if (!ok) return;

  const currentDisks = (inspectingVm.value.disks || []).filter((d) => d.id !== disk.id);
  try {
    await services.kvm.updateVirtualMachine(
      inspectingVm.value.name,
      { disks: currentDisks },
      ['disks']
    );
    notify('success', `Detached disk ${disk.id}`);
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to detach disk');
  }
}

// Edit / Remove Network Interfaces
function openEditNicModal(nic: VirtualMachineNetworkInterface) {
  if (!inspectingVm.value) return;
  editingNicId.value = nic.id;
  editNicError.value = '';
  isSavingNic.value = false;
  editNicForm.model = nic.model || 'virtio';
  editNicForm.bridge = nic.bridge || 'br0';
  editNicForm.vlanTag = nic.vlanTag || 0;
  editNicForm.macAddress = nic.macAddress || '';
  editNicForm.firewall = nic.firewall ?? true;
  showEditNicModal.value = true;
}

async function handleSaveEditNic() {
  if (!inspectingVm.value) return;
  editNicError.value = '';
  const currentNics = inspectingVm.value.networkInterfaces ? [...inspectingVm.value.networkInterfaces] : [];
  const idx = currentNics.findIndex((n) => n.id === editingNicId.value);
  if (idx === -1) {
    editNicError.value = `Selected network interface "${editingNicId.value}" not found on VM "${inspectingVm.value.vmName}"`;
    notify('error', editNicError.value);
    return;
  }
  const updatedNic: VirtualMachineNetworkInterface = {
    ...currentNics[idx],
    model: editNicForm.model,
    bridge: editNicForm.bridge,
    vlanTag: editNicForm.vlanTag > 0 ? editNicForm.vlanTag : undefined,
    macAddress: editNicForm.macAddress.trim() || undefined,
    firewall: editNicForm.firewall,
  };
  currentNics[idx] = updatedNic;

  isSavingNic.value = true;
  try {
    await services.kvm.updateVirtualMachine(
      inspectingVm.value.name,
      { networkInterfaces: currentNics },
      ['network_interfaces']
    );
    notify('success', `Updated network interface ${editingNicId.value}`);
    showEditNicModal.value = false;
    await refreshVms();
  } catch (err: any) {
    const msg = err?.message || 'Failed to update network interface';
    editNicError.value = msg;
    notify('error', msg);
  } finally {
    isSavingNic.value = false;
  }
}

async function handleDeleteNic(nic: VirtualMachineNetworkInterface) {
  if (!inspectingVm.value) return;
  const ok = window.confirm(`Are you sure you want to remove network interface ${nic.id} (${nic.bridge}) from ${inspectingVm.value.vmName}?`);
  if (!ok) return;

  const currentNics = (inspectingVm.value.networkInterfaces || []).filter((n) => n.id !== nic.id);
  try {
    await services.kvm.updateVirtualMachine(
      inspectingVm.value.name,
      { networkInterfaces: currentNics },
      ['network_interfaces']
    );
    notify('success', `Removed network interface ${nic.id}`);
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to remove network interface');
  }
}

// Hardware Edit Handlers
function openEditCpuModal() {
  if (!inspectingVm.value) return;
  editCpuError.value = '';
  isSavingCpu.value = false;
  editCpuForm.sockets = inspectingVm.value.sockets || 1;
  editCpuForm.cores = inspectingVm.value.cores || inspectingVm.value.vcpus || 1;
  editCpuForm.vcpus = inspectingVm.value.vcpus || (editCpuForm.sockets * editCpuForm.cores);
  editCpuForm.cpuType = inspectingVm.value.cpuType || 'host';
  editCpuForm.numa = Boolean(inspectingVm.value.numa);
  showEditCpuModal.value = true;
}

async function handleUpdateCpu() {
  if (!inspectingVm.value) return;
  editCpuError.value = '';
  const vcpus = editCpuForm.sockets * editCpuForm.cores;
  isSavingCpu.value = true;
  try {
    await services.kvm.updateVirtualMachine(
      inspectingVm.value.name,
      {
        vcpus,
        sockets: editCpuForm.sockets,
        cores: editCpuForm.cores,
        cpuType: editCpuForm.cpuType,
        numa: editCpuForm.numa,
      },
      ['vcpus', 'sockets', 'cores', 'cpu_type', 'numa']
    );
    notify('success', `Updated CPU configuration for ${inspectingVm.value.vmName}`);
    showEditCpuModal.value = false;
    await refreshVms();
  } catch (err: any) {
    const msg = err?.message || 'Failed to update CPU settings';
    editCpuError.value = msg;
    notify('error', msg);
  } finally {
    isSavingCpu.value = false;
  }
}

function openEditMemoryModal() {
  if (!inspectingVm.value) return;
  editMemoryError.value = '';
  isSavingMemory.value = false;
  editMemoryForm.memoryMib = inspectingVm.value.memoryMib || 2048;
  editMemoryForm.hugepages = inspectingVm.value.hugepages || 'off';
  showEditMemoryModal.value = true;
}

async function handleUpdateMemory() {
  if (!inspectingVm.value) return;
  editMemoryError.value = '';
  isSavingMemory.value = true;
  try {
    await services.kvm.updateVirtualMachine(
      inspectingVm.value.name,
      {
        memoryMib: editMemoryForm.memoryMib,
        balloonMinMib: editMemoryForm.memoryMib,
        hugepages: editMemoryForm.hugepages,
      },
      ['memory_mib', 'balloon_min_mib', 'hugepages']
    );
    notify('success', `Updated memory configuration for ${inspectingVm.value.vmName}`);
    showEditMemoryModal.value = false;
    await refreshVms();
  } catch (err: any) {
    const msg = err?.message || 'Failed to update memory settings';
    editMemoryError.value = msg;
    notify('error', msg);
  } finally {
    isSavingMemory.value = false;
  }
}

function openEditPlatformModal() {
  if (!inspectingVm.value) return;
  editPlatformError.value = '';
  isSavingPlatform.value = false;
  let mach = inspectingVm.value.machineType || 'i440fx';
  if (mach === 'pc' || mach.startsWith('pc-i440fx')) {
    mach = 'i440fx';
  } else if (mach.startsWith('pc-q35')) {
    mach = 'q35';
  }
  editPlatformForm.machineType = mach;
  editPlatformForm.biosType = inspectingVm.value.biosType || 'seabios';
  editPlatformForm.tpmVersion = inspectingVm.value.tpmVersion || 'none';
  editPlatformForm.secureBoot = Boolean(inspectingVm.value.secureBoot);
  showEditPlatformModal.value = true;
}

async function handleUpdatePlatform() {
  if (!inspectingVm.value) return;
  editPlatformError.value = '';
  isSavingPlatform.value = true;
  try {
    await services.kvm.updateVirtualMachine(
      inspectingVm.value.name,
      {
        machineType: editPlatformForm.machineType,
        biosType: editPlatformForm.biosType,
        tpmVersion: editPlatformForm.tpmVersion,
        secureBoot: editPlatformForm.secureBoot,
      },
      ['machine_type', 'bios_type', 'tpm_version', 'secure_boot']
    );
    notify('success', `Updated platform / BIOS configuration for ${inspectingVm.value.vmName}`);
    showEditPlatformModal.value = false;
    await refreshVms();
  } catch (err: any) {
    const msg = err?.message || 'Failed to update platform settings';
    editPlatformError.value = msg;
    notify('error', msg);
  } finally {
    isSavingPlatform.value = false;
  }
}

function openEditDisplayModal() {
  if (!inspectingVm.value) return;
  editDisplayError.value = '';
  isSavingDisplay.value = false;
  editDisplayForm.displayType = inspectingVm.value.displayType || 'virtio-gpu';
  showEditDisplayModal.value = true;
}

async function handleUpdateDisplay() {
  if (!inspectingVm.value) return;
  editDisplayError.value = '';
  isSavingDisplay.value = true;
  try {
    await services.kvm.updateVirtualMachine(
      inspectingVm.value.name,
      {
        displayType: editDisplayForm.displayType,
      },
      ['display_type']
    );
    notify('success', `Updated display configuration for ${inspectingVm.value.vmName}`);
    showEditDisplayModal.value = false;
    await refreshVms();
  } catch (err: any) {
    const msg = err?.message || 'Failed to update display settings';
    editDisplayError.value = msg;
    notify('error', msg);
  } finally {
    isSavingDisplay.value = false;
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
  const res = await confirmDelete({
    kind: 'Virtual Machine',
    name: vm.vmName,
    details: 'The domain definition, disk mappings and NVRAM state will be removed.',
    confirmText: 'Delete VM',
  });
  if (!res.confirmed) return;

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
  const targetId = toZfsSafeSlug(wizardForm.vmId || wizardForm.vmName) || 'vm';
  const vcpus = wizardForm.sockets * wizardForm.cores;

  const disks: VirtualMachineDisk[] = wizardForm.disks.map((d, i) => {
    const diskPath = d.diskPath.trim() || generateZvolDiskPath(mainPool.value, targetId, i);
    return {
      id: d.id || `sd${String.fromCharCode(97 + i)}`,
      bus: d.bus,
      diskPath,
      sizeBytes: d.sizeGib * 1024 * 1024 * 1024,
      cacheMode: d.cacheMode,
      discard: d.discard,
      iothread: d.iothread,
      backup: true,
    };
  });

  const nics: VirtualMachineNetworkInterface[] = wizardForm.networkInterfaces.map((nic, i) => ({
    id: nic.id || `net${i}`,
    model: nic.model,
    bridge: nic.bridge,
    vlanTag: nic.vlanTag > 0 ? nic.vlanTag : undefined,
    firewall: nic.firewall,
  }));

  const sshKeys = wizardForm.ciSshKeys
    .split('\n')
    .map((k) => k.trim())
    .filter(Boolean);

  try {
    const created = await services.kvm.createVirtualMachine(targetId, {
      vmName: wizardForm.vmName || targetId,
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
      balloonMinMib: wizardForm.memoryMib,
      hugepages: wizardForm.hugepages,
      machineType: wizardForm.machineType,
      biosType: wizardForm.biosType,
      tpmVersion: wizardForm.tpmVersion,
      secureBoot: wizardForm.secureBoot,
      cdromIsoImage: wizardForm.bootIso || undefined,
      displayType: wizardForm.displayType || 'virtio-gpu',
      disks,
      networkInterfaces: nics,
      cloudInit: {
        user: wizardForm.ciUser,
        password: wizardForm.ciPassword,
        sshPublicKeys: sshKeys,
        ipConfig: wizardForm.ciIpConfig,
      },
    });

    notify('success', `Created VM ${created.vmName}`);
    if (wizardForm.bootIso) {
      try {
        await services.kvm.attachMedia(created.name, wizardForm.bootIso);
      } catch (mediaErr: any) {
        console.warn('Failed to attach boot media:', mediaErr);
      }
    }
    showCreateWizard.value = false;
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to deploy virtual machine');
  }
}
</script>

<template>
  <div style="display: flex; flex-direction: column; gap: 20px">
    <!-- Top Summary Banner -->
    <div class="grid-3">
      <div class="card">
        <div class="card-header" style="margin-bottom: 6px">
          <span class="card-subtitle">Virtual Machines</span>
          <span class="badge badge-success">{{ runningCount }} / {{ nasState.vms.length }} Running</span>
        </div>
        <div style="font-size: 24px; font-weight: 700; color: var(--text-primary)">
          {{ nasState.vms.length }}
        </div>
      </div>

      <div class="card">
        <div class="card-header" style="margin-bottom: 6px">
          <span class="card-subtitle">vCPU Allocation</span>
          <MdiIcon :path="mdiSpeedometer" size="18" style="color: var(--accent-primary)" />
        </div>
        <div style="font-size: 24px; font-weight: 700; color: var(--text-primary)">
          {{ totalVcpus }}
        </div>
      </div>

      <div class="card">
        <div class="card-header" style="margin-bottom: 6px">
          <span class="card-subtitle">RAM Allocated</span>
          <MdiIcon :path="mdiServer" size="18" style="color: var(--accent-primary)" />
        </div>
        <div style="font-size: 24px; font-weight: 700; color: var(--text-primary)">
          {{ formatBytes(totalMemoryMib * 1024 * 1024) }}
        </div>
      </div>
    </div>

    <!-- Main Toolbar & VM Table -->
    <div class="card">
      <div class="card-header">
        <div>
          <h2 class="card-title" style="font-size: 17px">Virtual Machines</h2>
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

          <button class="btn btn-sm" @click="refreshVms()">
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
              <SortableTh :sort="vmSort" sort-key="name">Name</SortableTh>
              <SortableTh :sort="vmSort" sort-key="vcpus">vCPUs</SortableTh>
              <SortableTh :sort="vmSort" sort-key="memory">Memory</SortableTh>
              <SortableTh :sort="vmSort" sort-key="ip">IP Addresses</SortableTh>
              <th style="text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <!-- Moving Skeleton Placeholders when Loading -->
            <template v-if="nasState.loading && nasState.vms.length === 0">
              <tr v-for="r in 4" :key="r">
                <td><SkeletonBox width="140px" height="15px" /></td>
                <td><SkeletonBox width="30px" height="14px" /></td>
                <td><SkeletonBox width="60px" height="14px" /></td>
                <td><SkeletonBox width="110px" height="14px" /></td>
                <td style="text-align: right">
                  <div style="display: inline-flex; gap: 6px; justify-content: flex-end">
                    <SkeletonBox width="26px" height="26px" rounded="4px" />
                    <SkeletonBox width="65px" height="26px" rounded="4px" />
                    <SkeletonBox width="26px" height="26px" rounded="4px" />
                  </div>
                </td>
              </tr>
            </template>

            <tr v-else-if="filteredVms.length === 0">
              <td colspan="5" style="text-align: center; color: var(--text-secondary); padding: 36px">
                <div v-if="searchQuery">
                  No virtual machines match filter "<strong>{{ searchQuery }}</strong>".
                </div>
                <div v-else>
                  No virtual machines created yet. Click <strong>"Create VM"</strong> to get started.
                </div>
              </td>
            </tr>

            <tr
              v-for="vm in vmSort.sorted.value"
              :key="vm.name"
              class="vm-row"
              :class="`vm-row-${(vm.state || 'unknown').toLowerCase()}`"
              :title="vm.state"
            >
              <!-- Name (row tint conveys power state) -->
              <td>
                <div class="font-semibold" style="color: var(--text-primary); cursor: pointer" @click="openInspector(vm)">
                  {{ vm.vmName }}
                  <span v-if="vm.isTemplate" class="badge badge-warning" style="font-size: 9px; margin-left: 4px">TEMPLATE</span>
                  <span
                    v-if="transitioningVms[vm.name]"
                    class="badge mono badge-warning"
                    style="font-size: 9px; margin-left: 6px"
                  >
                    {{ transitioningVms[vm.name] === 'stopping' ? 'SHUTTING DOWN' : transitioningVms[vm.name] === 'starting' ? 'STARTING' : 'REBOOTING' }}
                  </span>
                  <span
                    v-else-if="vm.state === 'STOPPED'"
                    class="badge mono badge-neutral"
                    style="font-size: 9px; margin-left: 6px"
                  >
                    STOPPED
                  </span>
                </div>
                <div
                  v-if="vm.name.replace('virtualMachines/', '') !== vm.vmName"
                  class="mono text-muted"
                  style="font-size: 11px"
                >
                  {{ vm.name.replace('virtualMachines/', '') }}
                </div>
              </td>

              <!-- vCPUs (total threads) -->
              <td class="mono" style="font-size: 12px">{{ vm.vcpus }}</td>

              <!-- Memory -->
              <td class="mono" style="font-size: 12px; white-space: nowrap">
                {{ formatBytes(vm.memoryMib * 1024 * 1024) }}
              </td>

              <!-- IP Addresses -->
              <td>
                <div v-if="visibleGuestIps(vm).length > 0" style="display: flex; flex-direction: column; gap: 2px">
                  <span
                    v-for="ip in visibleGuestIps(vm).slice(0, 2)"
                    :key="ip"
                    class="badge mono badge-neutral"
                    style="font-size: 10px; width: fit-content"
                  >
                    {{ ip }}
                  </span>
                  <span v-if="visibleGuestIps(vm).length > 2" class="text-muted" style="font-size: 10px">
                    +{{ visibleGuestIps(vm).length - 2 }} more
                  </span>
                </div>
                <span v-else class="text-muted" style="font-size: 11px">
                  {{ vm.state !== 'RUNNING' ? 'Offline' : (vm.guestIpAddresses?.length ? '—' : 'Querying...') }}
                </span>
              </td>

              <!-- Actions -->
              <td style="text-align: right">
                <div style="display: inline-flex; gap: 6px; align-items: center">
                  <!-- Start/Resume -->
                  <button
                    v-if="vm.state === 'STOPPED'"
                    class="btn btn-xs btn-success"
                    :disabled="Boolean(transitioningVms[vm.name])"
                    title="Start VM"
                    @click="handlePowerAction(vm, 'start')"
                  >
                    <MdiIcon :path="mdiPlay" size="13" />
                  </button>
                  <button
                    v-else-if="vm.state === 'PAUSED'"
                    class="btn btn-xs btn-primary"
                    :disabled="Boolean(transitioningVms[vm.name])"
                    title="Resume VM"
                    @click="handlePowerAction(vm, 'resume')"
                  >
                    <MdiIcon :path="mdiPlay" size="13" />
                  </button>

                  <!-- Pause -->
                  <button
                    v-if="vm.state === 'RUNNING'"
                    class="btn btn-xs"
                    :disabled="Boolean(transitioningVms[vm.name])"
                    title="Pause VM"
                    @click="handlePowerAction(vm, 'pause')"
                  >
                    <MdiIcon :path="mdiPause" size="13" />
                  </button>

                  <!-- Reboot -->
                  <button
                    v-if="vm.state === 'RUNNING'"
                    class="btn btn-xs"
                    :disabled="Boolean(transitioningVms[vm.name])"
                    title="Reboot VM (Hold Shift to Force Reset)"
                    @click="handlePowerAction(vm, 'reboot', $event)"
                  >
                    <MdiIcon :path="mdiRefresh" size="13" />
                  </button>

                  <!-- Shutdown -->
                  <button
                    v-if="vm.state === 'RUNNING'"
                    class="btn btn-xs btn-warning"
                    :disabled="Boolean(transitioningVms[vm.name])"
                    title="Shutdown VM (Hold Shift to Force Stop)"
                    @click="handlePowerAction(vm, 'stop', $event)"
                  >
                    <MdiIcon :path="mdiPower" size="13" />
                  </button>

                  <!-- Console -->
                  <button
                    class="btn btn-xs btn-primary"
                    :disabled="vm.state !== 'RUNNING'"
                    title="Open Console"
                    @click="handleOpenConsole(vm)"
                  >
                    <MdiIcon :path="mdiConsole" size="13" />
                  </button>

                  <!-- Clone -->
                  <button
                    class="btn btn-xs"
                    title="Clone VM"
                    @click="openCloneModal(vm)"
                  >
                    <MdiIcon :path="mdiContentCopy" size="13" />
                  </button>

                  <!-- Edit / Hardware Inspector -->
                  <button
                    class="btn btn-xs"
                    title="Edit VM & Hardware"
                    @click="openInspector(vm)"
                  >
                    <MdiIcon :path="mdiPencil" size="13" />
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
    <!-- ISOS REPOSITORY CARD -->
    <!-- ======================================================================= -->
    <div class="card">
      <div class="card-header">
        <div>
          <h2 class="card-title" style="font-size: 17px">ISOs</h2>
        </div>

        <div style="display: flex; gap: 10px; align-items: center">
          <input
            ref="isoFileInput"
            type="file"
            accept=".iso,.img"
            style="display: none"
            @change="handleUploadIso"
          />
          <button class="btn btn-sm" @click="refreshIsos">
            <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" /> Refresh
          </button>
          <button
            class="btn btn-sm btn-primary"
            :disabled="isUploadingIso"
            @click="triggerIsoUpload"
          >
            <MdiIcon :path="mdiUpload" size="14" style="margin-right: 4px" />
            {{ isUploadingIso && isoUploadProgress ? `Uploading ${isoUploadProgress.percent}%...` : 'Upload ISO' }}
          </button>
        </div>
      </div>

      <!-- Drag & Drop ISO Dropzone -->
      <div style="margin: 0 16px 16px 16px">
        <FileDropzone
          ref="isoDropzoneRef"
          accept=".iso,.img"
          label="Drag & drop ISO file here, or click to browse"
          sublabel="Supports .iso and .img installer images"
          :icon="mdiDisc"
          :is-uploading="isUploadingIso"
          :progress="isoUploadProgress"
          @file-selected="uploadIsoFile"
          @invalid-file="handleInvalidIsoFile"
        />
      </div>

      <!-- ISO Table -->
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <SortableTh :sort="isoSort" sort-key="name">ISO Image Name</SortableTh>
              <SortableTh :sort="isoSort" sort-key="size">File Size</SortableTh>
              <SortableTh :sort="isoSort" sort-key="modTime">Last Modified</SortableTh>
              <th style="text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <!-- Active Upload Row -->
            <tr v-if="isUploadingIso && isoUploadProgress">
              <td colspan="4" style="padding: 14px 16px; background: rgba(59, 130, 246, 0.04)">
                <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; font-size: 12px">
                  <span class="mono" style="font-weight: 600">{{ isoUploadProgress.filename }} (Uploading)</span>
                  <span class="mono">{{ formatBytes(isoUploadProgress.loaded) }} / {{ formatBytes(isoUploadProgress.total) }} ({{ isoUploadProgress.percent }}%)</span>
                </div>
                <div class="progress-bar" style="height: 6px">
                  <div class="progress-fill" :style="{ width: `${isoUploadProgress.percent}%` }"></div>
                </div>
              </td>
            </tr>

            <!-- Skeleton Placeholders when Loading -->
            <template v-if="nasState.loading && nasState.isos.length === 0">
              <tr v-for="r in 2" :key="r">
                <td><SkeletonBox width="160px" height="15px" /></td>
                <td><SkeletonBox width="70px" height="15px" /></td>
                <td><SkeletonBox width="120px" height="15px" /></td>
                <td style="text-align: right"><SkeletonBox width="60px" height="24px" rounded="4px" /></td>
              </tr>
            </template>

            <tr v-else-if="nasState.isos.length === 0">
              <td colspan="4" style="text-align: center; color: var(--text-secondary); padding: 36px">
                No ISO images found in repository. Click <strong>"Upload ISO"</strong> to upload an installer image (.iso, .img).
              </td>
            </tr>

            <tr v-for="iso in isoSort.sorted.value" :key="iso.name">
              <td style="font-weight: 600">
                <div style="display: flex; align-items: center; gap: 8px">
                  <MdiIcon :path="mdiDisc" size="16" style="color: var(--accent-primary)" />
                  <span class="mono" :title="iso.path">{{ iso.name }}</span>
                </div>
              </td>
              <td class="mono">{{ iso.sizeFormatted }}</td>
              <td
                class="mono"
                style="font-size: 11px; color: var(--text-muted); white-space: nowrap"
                :title="formatAbsoluteTime(iso.modTime)"
              >
                {{ formatRelativeTime(iso.modTime) }}
              </td>
              <td style="text-align: right">
                <div style="display: inline-flex; gap: 6px">
                  <button
                    class="btn btn-xs"
                    title="Copy absolute path"
                    @click="copyIsoPath(iso.path)"
                  >
                    <MdiIcon :path="mdiContentCopy" size="13" />
                  </button>
                  <button
                    class="btn btn-xs btn-danger"
                    title="Delete ISO"
                    @click="handleDeleteIso(iso)"
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
    <div v-if="inspectingVm" class="modal-backdrop">
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
                  v-if="transitioningVms[inspectingVm.name]"
                  class="badge mono badge-warning"
                  style="font-size: 11px"
                >
                  {{ transitioningVms[inspectingVm.name] === 'stopping' ? 'SHUTTING DOWN...' : transitioningVms[inspectingVm.name] === 'starting' ? 'STARTING...' : 'REBOOTING...' }}
                </span>
                <span
                  v-else
                  class="badge mono"
                  :class="inspectingVm.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'"
                  style="font-size: 11px"
                >
                  {{ inspectingVm.state }}
                </span>
              </div>
              <div class="mono text-muted" style="font-size: 11px">
                {{ inspectingVm.name.replace('virtualMachines/', '') }} (UUID: {{ inspectingVm.uuid || 'N/A' }})
              </div>
            </div>
          </div>

          <!-- Quick Action Buttons inside Inspector Header -->
          <div style="display: flex; gap: 8px; align-items: center">
            <button
              v-if="inspectingVm.state !== 'RUNNING'"
              class="btn btn-sm btn-success"
              :disabled="Boolean(transitioningVms[inspectingVm.name])"
              @click="handlePowerAction(inspectingVm, 'start', $event)"
            >
              <MdiIcon :path="mdiPlay" size="14" style="margin-right: 4px" /> Start
            </button>
            <template v-else>
              <button
                class="btn btn-sm"
                :disabled="Boolean(transitioningVms[inspectingVm.name])"
                title="Reboot VM (Hold Shift to Force Reset)"
                @click="handlePowerAction(inspectingVm, 'reboot', $event)"
              >
                <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" /> Reboot
              </button>
              <button
                class="btn btn-sm btn-warning"
                :disabled="Boolean(transitioningVms[inspectingVm.name])"
                title="Shutdown VM (Hold Shift to Force Stop)"
                @click="handlePowerAction(inspectingVm, 'stop', $event)"
              >
                <MdiIcon :path="mdiPower" size="14" style="margin-right: 4px" /> Shutdown
              </button>
            </template>
            <button
              class="btn btn-sm btn-primary"
              :disabled="inspectingVm.state !== 'RUNNING' || Boolean(transitioningVms[inspectingVm.name])"
              @click="handleOpenConsole(inspectingVm)"
            >
              <MdiIcon :path="mdiConsole" size="14" style="margin-right: 4px" /> Console
            </button>
            <button class="modal-close-btn" type="button" aria-label="Close" @click="inspectingVm = null">
              <MdiIcon :path="mdiClose" size="18" />
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
                <div class="card-subtitle">CPU Usage</div>
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
                  Fixed Dedicated Memory
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
                IP Addresses
              </div>
              <div v-if="visibleGuestIps(inspectingVm).length > 0" style="display: flex; flex-wrap: wrap; gap: 8px">
                <code
                  v-for="ip in visibleGuestIps(inspectingVm)"
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
                <button class="btn btn-xs" @click="nicForm.bridge = defaultBridge; showAddNicModal = true">
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
                    <th style="text-align: right; width: 140px">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  <!-- Processors -->
                  <tr @dblclick="openEditCpuModal" style="cursor: pointer" title="Double click to edit processors">
                    <td class="font-semibold">Processors</td>
                    <td class="mono" style="font-size: 12px">
                      {{ inspectingVm.vcpus }} vCPU ({{ inspectingVm.sockets || 1 }} sockets, {{ inspectingVm.cores || inspectingVm.vcpus }} cores) [Type: {{ inspectingVm.cpuType || 'host' }}]
                      <span v-if="inspectingVm.numa" class="badge badge-neutral" style="margin-left: 6px; font-size: 10px">NUMA</span>
                    </td>
                    <td style="text-align: right">
                      <button class="btn btn-xs" @click.stop="openEditCpuModal">
                        <MdiIcon :path="mdiPencil" size="12" /> Edit
                      </button>
                    </td>
                  </tr>

                  <!-- Memory -->
                  <tr @dblclick="openEditMemoryModal" style="cursor: pointer" title="Double click to edit memory">
                    <td class="font-semibold">Memory</td>
                    <td class="mono" style="font-size: 12px">
                      {{ formatBytes(inspectingVm.memoryMib * 1024 * 1024) }}
                      (Fixed, Hugepages: {{ inspectingVm.hugepages || 'off' }})
                    </td>
                    <td style="text-align: right">
                      <button class="btn btn-xs" @click.stop="openEditMemoryModal">
                        <MdiIcon :path="mdiPencil" size="12" /> Edit
                      </button>
                    </td>
                  </tr>

                  <!-- Machine & BIOS -->
                  <tr @dblclick="openEditPlatformModal" style="cursor: pointer" title="Double click to edit machine & BIOS">
                    <td class="font-semibold">Machine & BIOS</td>
                    <td class="mono" style="font-size: 12px">
                      BIOS: {{ (inspectingVm.biosType || 'seabios').toUpperCase() }} | Machine: {{ inspectingVm.machineType || 'q35' }} | TPM: {{ inspectingVm.tpmVersion || 'none' }}
                      <span v-if="inspectingVm.secureBoot" class="badge badge-neutral" style="margin-left: 6px; font-size: 10px">Secure Boot</span>
                    </td>
                    <td style="text-align: right">
                      <button class="btn btn-xs" @click.stop="openEditPlatformModal">
                        <MdiIcon :path="mdiPencil" size="12" /> Edit
                      </button>
                    </td>
                  </tr>

                  <!-- Hard Disks -->
                  <tr
                    v-for="d in inspectingVm.disks"
                    :key="d.id"
                    @dblclick="openEditDiskModal(d)"
                    style="cursor: pointer"
                    title="Double click to edit hard disk"
                  >
                    <td class="font-semibold">
                      Hard Disk (<code class="mono">{{ d.id }}</code>)
                    </td>
                    <td class="mono" style="font-size: 12px">
                      {{ d.diskPath }} ({{ formatBytes(d.sizeBytes) }}, Bus: {{ d.bus }}, Cache: {{ d.cacheMode || 'none' }}, Discard: {{ d.discard ? 'on' : 'off' }}, IOThread: {{ d.iothread ? 'on' : 'off' }})
                    </td>
                    <td style="text-align: right">
                      <div style="display: inline-flex; gap: 6px; align-items: center">
                        <button class="btn btn-xs" @click.stop="openEditDiskModal(d)" title="Edit disk">
                          <MdiIcon :path="mdiPencil" size="12" /> Edit
                        </button>
                        <button class="btn btn-xs btn-danger" @click.stop="handleDeleteDisk(d)" title="Detach disk">
                          <MdiIcon :path="mdiTrashCan" size="12" /> Detach
                        </button>
                      </div>
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
                  <tr
                    v-for="nic in inspectingVm.networkInterfaces"
                    :key="nic.id"
                    @dblclick="openEditNicModal(nic)"
                    style="cursor: pointer"
                    title="Double click to edit network device"
                  >
                    <td class="font-semibold">
                      Network (<code class="mono">{{ nic.id }}</code>)
                    </td>
                    <td class="mono" style="font-size: 12px">
                      Bridge: {{ nic.bridge }}, Model: {{ nic.model }}, MAC: {{ nic.macAddress || 'auto' }}
                      <span v-if="nic.vlanTag" style="color: var(--accent-primary)">, VLAN: {{ nic.vlanTag }}</span>
                    </td>
                    <td style="text-align: right">
                      <div style="display: inline-flex; gap: 6px; align-items: center">
                        <button class="btn btn-xs" @click.stop="openEditNicModal(nic)" title="Edit network device">
                          <MdiIcon :path="mdiPencil" size="12" /> Edit
                        </button>
                        <button class="btn btn-xs btn-danger" @click.stop="handleDeleteNic(nic)" title="Remove network device">
                          <MdiIcon :path="mdiTrashCan" size="12" /> Remove
                        </button>
                      </div>
                    </td>
                  </tr>

                  <!-- Display -->
                  <tr @dblclick="openEditDisplayModal" style="cursor: pointer" title="Double click to edit display">
                    <td class="font-semibold">Display / Graphics</td>
                    <td class="mono" style="font-size: 12px">
                      {{ inspectingVm.displayType || 'virtio-gpu' }} (VNC Display: {{ inspectingVm.vncDisplay || '127.0.0.1:5900' }})
                    </td>
                    <td style="text-align: right">
                      <button class="btn btn-xs" @click.stop="openEditDisplayModal">
                        <MdiIcon :path="mdiPencil" size="12" /> Edit
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- ========================================== -->
          <!-- TAB 3: CLOUD-INIT -->
          <!-- ========================================== -->
          <div v-else-if="inspectorTab === 'cloudinit'" style="display: flex; flex-direction: column; gap: 14px">
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
                Snapshots
              </div>
              <button class="btn btn-sm btn-primary" @click="openTakeSnapshotModal">
                <MdiIcon :path="mdiPlus" size="14" style="margin-right: 4px" /> Take Snapshot
              </button>
            </div>

            <div v-if="snapshotsList.length > 0" class="table-wrap">
              <table class="data-table">
                <thead>
                  <tr>
                    <SortableTh :sort="snapshotSort" sort-key="id">Snapshot ID</SortableTh>
                    <SortableTh :sort="snapshotSort" sort-key="vmState">VM State</SortableTh>
                    <SortableTh :sort="snapshotSort" sort-key="includeRam">RAM Included</SortableTh>
                    <SortableTh :sort="snapshotSort" sort-key="created">Created</SortableTh>
                    <SortableTh :sort="snapshotSort" sort-key="description">Description</SortableTh>
                    <th style="text-align: right">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="s in snapshotSort.sorted.value" :key="s.snapshotId">
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
              No snapshots yet.
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- TAKE SNAPSHOT MODAL -->
    <!-- ======================================================================= -->
    <div v-if="showTakeSnapshotModal" class="modal-backdrop">
      <div class="modal-card" style="max-width: 480px">
        <div class="modal-header">
          <h3 class="modal-title">Take Snapshot</h3>
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
            <input v-model="snapshotForm.includeRam" type="checkbox" /> Include RAM state
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
    <div v-if="showMediaModal" class="modal-backdrop">
      <div class="modal-card" style="max-width: 480px">
        <div class="modal-header">
          <h3 class="modal-title">Insert Media</h3>
          <button class="btn btn-sm" @click="showMediaModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <form @submit.prevent="handleAttachMedia" style="padding: 16px; display: flex; flex-direction: column; gap: 14px">
          <div v-if="nasState.isos.length > 0" class="form-group">
            <label>ISO</label>
            <select v-model="mediaForm.isoPath" class="select mono">
              <option value="">-- Choose an ISO --</option>
              <option v-for="iso in nasState.isos" :key="iso.path" :value="iso.path">
                {{ iso.name }} ({{ iso.sizeFormatted }})
              </option>
            </select>
          </div>
          <div class="form-group">
            <label>ISO Image Path</label>
            <PathSelector
              v-model="mediaForm.isoPath"
              placeholder="/mnt/tank/isos/debian-12.iso"
              root-hint="/mnt"
              required
            />
            <small style="color: var(--text-muted); font-size: 11px">
              Or browse / enter a custom path on the host.
            </small>
          </div>
          <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 10px">
            <button type="button" class="btn" @click="showMediaModal = false">Cancel</button>
            <button type="submit" class="btn btn-primary" :disabled="!mediaForm.isoPath.trim()">Mount Media</button>
          </div>
        </form>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- ADD HARD DISK MODAL -->
    <!-- ======================================================================= -->
    <div v-if="showAddDiskModal" class="modal-backdrop">
      <div class="modal-card" style="max-width: 500px">
        <div class="modal-header">
          <h3 class="modal-title">Add Hard Disk</h3>
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
    <div v-if="showAddNicModal" class="modal-backdrop">
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
            <label>Bridge / LAN Interface</label>
            <select v-model="nicForm.bridge" class="select mono" required>
              <option v-for="b in bridgeOptionsFor(nicForm.bridge)" :key="b.name" :value="b.name">{{ b.label }}</option>
            </select>
            <span class="helper">Host bridge (e.g. vmbr0) or physical interface (e.g. ens3) for direct LAN bridging</span>
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
    <!-- EDIT HARD DISK MODAL -->
    <!-- ======================================================================= -->
    <div v-if="showEditDiskModal" class="modal-backdrop">
      <div class="modal-card" style="max-width: 500px">
        <div class="modal-header">
          <h3 class="modal-title">Edit Hard Disk ({{ editingDiskId }})</h3>
          <button class="btn btn-sm" @click="showEditDiskModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <form @submit.prevent="handleSaveEditDisk" style="padding: 16px; display: flex; flex-direction: column; gap: 14px">
          <div
            v-if="editDiskError"
            style="
              background: var(--btn-danger-bg);
              border: 1px solid var(--btn-danger-border);
              color: var(--btn-danger-text);
              padding: 10px 14px;
              border-radius: 6px;
              font-size: 13px;
              display: flex;
              align-items: center;
              gap: 8px;
            "
          >
            <MdiIcon :path="mdiAlertCircleOutline" size="16" />
            <span>{{ editDiskError }}</span>
          </div>

          <div class="form-grid">
            <div class="form-group">
              <label>Bus Device</label>
              <select v-model="editDiskForm.bus" class="select mono">
                <option value="scsi">VirtIO SCSI (Recommended)</option>
                <option value="virtio">VirtIO Block</option>
                <option value="sata">SATA</option>
                <option value="nvme">NVMe</option>
              </select>
            </div>
            <div class="form-group">
              <label>Size (GiB)</label>
              <input v-model.number="editDiskForm.sizeGib" type="number" min="1" class="input mono" required />
            </div>
          </div>
          <div class="form-group">
            <label>Storage Path (ZFS ZVol or QCOW2)</label>
            <input
              v-model="editDiskForm.diskPath"
              list="zvol-list-edit"
              class="input mono"
              placeholder="/dev/zvol/tank/vm-disk1"
              required
            />
            <datalist id="zvol-list-edit">
              <option v-for="zpath in availableZvols" :key="zpath" :value="zpath" />
            </datalist>
          </div>
          <div class="form-group">
            <label>Cache Mode</label>
            <select v-model="editDiskForm.cacheMode" class="select mono">
              <option value="none">none (Default / Direct I/O)</option>
              <option value="writeback">writeback</option>
              <option value="writethrough">writethrough</option>
              <option value="unsafe">unsafe</option>
            </select>
          </div>
          <div style="display: flex; gap: 16px; font-size: 13px">
            <label style="display: flex; align-items: center; gap: 6px">
              <input v-model="editDiskForm.discard" type="checkbox" /> Discard / TRIM
            </label>
            <label style="display: flex; align-items: center; gap: 6px">
              <input v-model="editDiskForm.iothread" type="checkbox" /> IOThread
            </label>
          </div>
          <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 10px">
            <button type="button" class="btn" :disabled="isSavingDisk" @click="showEditDiskModal = false">Cancel</button>
            <button type="submit" class="btn btn-primary" :disabled="isSavingDisk">
              <span v-if="isSavingDisk" class="spinner-sm" style="margin-right: 6px"></span>
              {{ isSavingDisk ? 'Saving...' : 'Save Changes' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- EDIT NETWORK DEVICE MODAL -->
    <!-- ======================================================================= -->
    <div v-if="showEditNicModal" class="modal-backdrop">
      <div class="modal-card" style="max-width: 480px">
        <div class="modal-header">
          <h3 class="modal-title">Edit Network Device ({{ editingNicId }})</h3>
          <button class="btn btn-sm" @click="showEditNicModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <form @submit.prevent="handleSaveEditNic" style="padding: 16px; display: flex; flex-direction: column; gap: 14px">
          <div
            v-if="editNicError"
            style="
              background: var(--btn-danger-bg);
              border: 1px solid var(--btn-danger-border);
              color: var(--btn-danger-text);
              padding: 10px 14px;
              border-radius: 6px;
              font-size: 13px;
              display: flex;
              align-items: center;
              gap: 8px;
            "
          >
            <MdiIcon :path="mdiAlertCircleOutline" size="16" />
            <span>{{ editNicError }}</span>
          </div>

          <div class="form-group">
            <label>Model</label>
            <select v-model="editNicForm.model" class="select mono">
              <option value="virtio">VirtIO (paravirtualized)</option>
              <option value="e1000">Intel E1000</option>
              <option value="rtl8139">Realtek RTL8139</option>
            </select>
          </div>
          <div class="form-group">
            <label>Bridge / LAN Interface</label>
            <select v-model="editNicForm.bridge" class="select mono" required>
              <option v-for="b in bridgeOptionsFor(editNicForm.bridge)" :key="b.name" :value="b.name">{{ b.label }}</option>
            </select>
            <span class="helper">Host bridge (e.g. vmbr0) or physical interface (e.g. ens3) for direct LAN bridging</span>
          </div>
          <div class="form-group">
            <label>VLAN Tag (0 = Untagged)</label>
            <input v-model.number="editNicForm.vlanTag" type="number" min="0" max="4094" class="input mono" />
          </div>
          <div class="form-group">
            <label>MAC Address (optional / blank to preserve)</label>
            <input v-model="editNicForm.macAddress" type="text" placeholder="52:54:00:xx:xx:xx" class="input mono" />
          </div>
          <div style="display: flex; gap: 16px; font-size: 13px">
            <label style="display: flex; align-items: center; gap: 6px">
              <input v-model="editNicForm.firewall" type="checkbox" /> MAC Spoofing Filter
            </label>
          </div>
          <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 10px">
            <button type="button" class="btn" :disabled="isSavingNic" @click="showEditNicModal = false">Cancel</button>
            <button type="submit" class="btn btn-primary" :disabled="isSavingNic">
              <span v-if="isSavingNic" class="spinner-sm" style="margin-right: 6px"></span>
              {{ isSavingNic ? 'Saving...' : 'Save Changes' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- EDIT PROCESSORS MODAL -->
    <!-- ======================================================================= -->
    <div v-if="showEditCpuModal" class="modal-backdrop">
      <div class="modal-card" style="max-width: 480px">
        <div class="modal-header">
          <h3 class="modal-title">Edit Processors ({{ inspectingVm?.vmName }})</h3>
          <button class="btn btn-sm" @click="showEditCpuModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <form @submit.prevent="handleUpdateCpu" style="padding: 16px; display: flex; flex-direction: column; gap: 14px">
          <div
            v-if="editCpuError"
            style="
              background: var(--btn-danger-bg);
              border: 1px solid var(--btn-danger-border);
              color: var(--btn-danger-text);
              padding: 10px 14px;
              border-radius: 6px;
              font-size: 13px;
              display: flex;
              align-items: center;
              gap: 8px;
            "
          >
            <MdiIcon :path="mdiAlertCircleOutline" size="16" />
            <span>{{ editCpuError }}</span>
          </div>

          <div class="form-grid">
            <div class="form-group">
              <label>Sockets</label>
              <select v-model.number="editCpuForm.sockets" class="select mono">
                <option v-for="n in socketOptions" :key="n" :value="n">{{ n }}</option>
              </select>
            </div>
            <div class="form-group">
              <label>Cores (per socket)</label>
              <select v-model.number="editCpuForm.cores" class="select mono">
                <option v-for="n in coreOptionsFor(editCpuForm.cores)" :key="n" :value="n">{{ n }}</option>
              </select>
            </div>
          </div>
          <div class="form-group">
            <label>Total vCPUs</label>
            <input :value="editCpuForm.sockets * editCpuForm.cores" class="input mono" disabled />
          </div>
          <div class="form-group">
            <label>CPU Type / Emulation Model</label>
            <select v-model="editCpuForm.cpuType" class="select mono">
              <option value="host">host (Pass-through host CPU flags - Recommended)</option>
              <option value="kvm64">kvm64 (Default QEMU 64-bit)</option>
              <option value="qemu64">qemu64 (Legacy QEMU)</option>
              <option value="x86-64-v2-AES">x86-64-v2-AES (Modern Generic x86-64)</option>
              <option value="EPYC">EPYC (AMD EPYC)</option>
            </select>
          </div>
          <label style="display: flex; align-items: center; gap: 8px; font-size: 13px">
            <input v-model="editCpuForm.numa" type="checkbox" />
            Enable NUMA (Non-Uniform Memory Access)
          </label>
          <div class="text-muted" style="font-size: 11px">
            Note: Changes to CPU topology and model take effect on next VM restart.
          </div>
          <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 10px">
            <button type="button" class="btn" :disabled="isSavingCpu" @click="showEditCpuModal = false">Cancel</button>
            <button type="submit" class="btn btn-primary" :disabled="isSavingCpu">
              <span v-if="isSavingCpu" class="spinner-sm" style="margin-right: 6px"></span>
              {{ isSavingCpu ? 'Saving...' : 'Save Changes' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- EDIT MEMORY MODAL -->
    <!-- ======================================================================= -->
    <div v-if="showEditMemoryModal" class="modal-backdrop">
      <div class="modal-card" style="max-width: 480px">
        <div class="modal-header">
          <h3 class="modal-title">Edit Memory ({{ inspectingVm?.vmName }})</h3>
          <button class="btn btn-sm" @click="showEditMemoryModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <form @submit.prevent="handleUpdateMemory" style="padding: 16px; display: flex; flex-direction: column; gap: 14px">
          <div
            v-if="editMemoryError"
            style="
              background: var(--btn-danger-bg);
              border: 1px solid var(--btn-danger-border);
              color: var(--btn-danger-text);
              padding: 10px 14px;
              border-radius: 6px;
              font-size: 13px;
              display: flex;
              align-items: center;
              gap: 8px;
            "
          >
            <MdiIcon :path="mdiAlertCircleOutline" size="16" />
            <span>{{ editMemoryError }}</span>
          </div>

          <div class="form-group">
            <label>Memory Size (MiB)</label>
            <input v-model.number="editMemoryForm.memoryMib" type="number" min="256" step="256" class="input mono" required />
            <span class="helper">{{ formatBytes(editMemoryForm.memoryMib * 1024 * 1024) }}</span>
          </div>
          <div class="form-group">
            <label>Hugepages</label>
            <select v-model="editMemoryForm.hugepages" class="select mono">
              <option value="off">off (Standard 4KB paging)</option>
              <option value="2MB">2MB (Pre-allocated Hugepages)</option>
              <option value="1GB">1GB (Gigabyte Hugepages)</option>
            </select>
          </div>
          <div class="text-muted" style="font-size: 11px">
            Note: Memory modifications take effect immediately or on next VM restart.
          </div>
          <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 10px">
            <button type="button" class="btn" :disabled="isSavingMemory" @click="showEditMemoryModal = false">Cancel</button>
            <button type="submit" class="btn btn-primary" :disabled="isSavingMemory">
              <span v-if="isSavingMemory" class="spinner-sm" style="margin-right: 6px"></span>
              {{ isSavingMemory ? 'Saving...' : 'Save Changes' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- EDIT PLATFORM / MACHINE & BIOS MODAL -->
    <!-- ======================================================================= -->
    <div v-if="showEditPlatformModal" class="modal-backdrop">
      <div class="modal-card" style="max-width: 500px">
        <div class="modal-header">
          <h3 class="modal-title">Edit Machine & BIOS ({{ inspectingVm?.vmName }})</h3>
          <button class="btn btn-sm" @click="showEditPlatformModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <form @submit.prevent="handleUpdatePlatform" style="padding: 16px; display: flex; flex-direction: column; gap: 14px">
          <div
            v-if="editPlatformError"
            style="
              background: var(--btn-danger-bg);
              border: 1px solid var(--btn-danger-border);
              color: var(--btn-danger-text);
              padding: 10px 14px;
              border-radius: 6px;
              font-size: 13px;
              display: flex;
              align-items: center;
              gap: 8px;
            "
          >
            <MdiIcon :path="mdiAlertCircleOutline" size="16" />
            <span>{{ editPlatformError }}</span>
          </div>

          <div class="form-group">
            <label>Machine Chipset Architecture</label>
            <select v-model="editPlatformForm.machineType" class="select mono">
              <option value="q35">q35 (Modern PCIe / SATA Architecture - Recommended)</option>
              <option value="i440fx">i440fx (Legacy PCI / IDE Architecture)</option>
            </select>
          </div>
          <div class="form-group">
            <label>BIOS / Firmware</label>
            <select v-model="editPlatformForm.biosType" class="select mono">
              <option value="ovmf">OVMF (UEFI - Modern GPT Boot)</option>
              <option value="seabios">SeaBIOS (Standard Legacy MBR/BIOS)</option>
            </select>
          </div>
          <div class="form-group">
            <label>vTPM (Trusted Platform Module)</label>
            <select v-model="editPlatformForm.tpmVersion" class="select mono">
              <option value="none">None (Disabled)</option>
              <option value="v2.0">v2.0 (Required for Windows 11)</option>
            </select>
          </div>
          <label style="display: flex; align-items: center; gap: 8px; font-size: 13px" :style="{ opacity: editPlatformForm.biosType === 'ovmf' ? 1 : 0.5 }">
            <input v-model="editPlatformForm.secureBoot" type="checkbox" :disabled="editPlatformForm.biosType !== 'ovmf'" />
            Enable UEFI Secure Boot (Requires OVMF)
          </label>
          <div class="text-muted" style="font-size: 11px">
            Note: Changing machine chipset or BIOS type requires a full VM restart and can alter device bus addresses.
          </div>
          <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 10px">
            <button type="button" class="btn" :disabled="isSavingPlatform" @click="showEditPlatformModal = false">Cancel</button>
            <button type="submit" class="btn btn-primary" :disabled="isSavingPlatform">
              <span v-if="isSavingPlatform" class="spinner-sm" style="margin-right: 6px"></span>
              {{ isSavingPlatform ? 'Saving...' : 'Save Changes' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- EDIT DISPLAY MODAL -->
    <!-- ======================================================================= -->
    <div v-if="showEditDisplayModal" class="modal-backdrop">
      <div class="modal-card" style="max-width: 480px">
        <div class="modal-header">
          <h3 class="modal-title">Edit Display Adapter ({{ inspectingVm?.vmName }})</h3>
          <button class="btn btn-sm" @click="showEditDisplayModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <form @submit.prevent="handleUpdateDisplay" style="padding: 16px; display: flex; flex-direction: column; gap: 14px">
          <div
            v-if="editDisplayError"
            style="
              background: var(--btn-danger-bg);
              border: 1px solid var(--btn-danger-border);
              color: var(--btn-danger-text);
              padding: 10px 14px;
              border-radius: 6px;
              font-size: 13px;
              display: flex;
              align-items: center;
              gap: 8px;
            "
          >
            <MdiIcon :path="mdiAlertCircleOutline" size="16" />
            <span>{{ editDisplayError }}</span>
          </div>

          <div class="form-group">
            <label>Display Device / GPU</label>
            <select v-model="editDisplayForm.displayType" class="select mono">
              <option value="virtio-gpu">VirtIO GPU (virtio-gpu - High Performance 2D/3D)</option>
              <option value="vga">Standard VGA (Compatibility)</option>
              <option value="qxl">QXL (SPICE / Enhanced 2D)</option>
              <option value="bochs">Bochs (Basic Display)</option>
              <option value="none">None (Headless / Serial Only)</option>
            </select>
          </div>
          <div class="text-muted" style="font-size: 11px">
            Note: Changes to display adapter take effect on next VM restart.
          </div>
          <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 10px">
            <button type="button" class="btn" :disabled="isSavingDisplay" @click="showEditDisplayModal = false">Cancel</button>
            <button type="submit" class="btn btn-primary" :disabled="isSavingDisplay">
              <span v-if="isSavingDisplay" class="spinner-sm" style="margin-right: 6px"></span>
              {{ isSavingDisplay ? 'Saving...' : 'Save Changes' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- CLONE VM MODAL -->
    <!-- ======================================================================= -->
    <div v-if="cloningVm" class="modal-backdrop">
      <div class="modal-card" style="max-width: 500px">
        <div class="modal-header">
          <h3 class="modal-title">Clone {{ cloningVm.vmName }}</h3>
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
            <label>Target Pool</label>
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
            <button type="submit" class="btn btn-primary">Clone</button>
          </div>
        </form>
      </div>
    </div>

    <!-- ======================================================================= -->
    <!-- NEW VM CREATION: HARDWARE CONFIGURATION TABLE MODAL -->
    <!-- ======================================================================= -->
    <div v-if="showCreateWizard" class="modal-backdrop">
      <div class="modal-card" style="max-width: 880px; width: 95vw; max-height: 92vh; display: flex; flex-direction: column">
        <!-- Modal Header -->
        <div class="modal-header" style="flex-shrink: 0">
          <div style="display: flex; align-items: center; gap: 12px">
            <div style="width: 36px; height: 36px; border-radius: 8px; background: rgba(56, 189, 248, 0.12); display: flex; align-items: center; justify-content: center; flex-shrink: 0">
              <MdiIcon :path="mdiServer" size="20" style="color: var(--accent-primary)" />
            </div>
            <div>
              <h3 class="modal-title">Create Virtual Machine</h3>
              <div class="text-muted" style="font-size: 12px; margin-top: 2px">
                Configure hardware and system resources for the new guest domain
              </div>
            </div>
          </div>
          <button class="modal-close-btn" type="button" aria-label="Close" @click="showCreateWizard = false">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>

        <!-- Modal Body -->
        <div style="flex: 1; min-height: 0; padding: 20px; overflow-y: auto; display: flex; flex-direction: column; gap: 16px">
          <!-- General VM info bar -->
          <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 14px; padding: 14px 16px; background: rgba(255, 255, 255, 0.02); border: 1px solid var(--border-subtle); border-radius: 8px">
            <div class="form-group" style="margin: 0">
              <label>VM Name</label>
              <input
                v-model="wizardForm.vmName"
                class="input"
                placeholder="e.g. Debian Server"
                @input="onVmNameInput"
                required
              />
            </div>
            <div class="form-group" style="margin: 0">
              <label>VM Domain ID</label>
              <input
                v-model="wizardForm.vmId"
                class="input mono"
                placeholder="e.g. debian-server"
                required
              />
            </div>
            <div class="form-group" style="margin: 0">
              <label>Operating System</label>
              <select v-model="wizardForm.osVariant" class="select mono" @change="onOsVariantChange">
                <option value="debian12">Debian 12 (Bookworm)</option>
                <option value="ubuntu24.04">Ubuntu 24.04 LTS</option>
                <option value="almalinux9">AlmaLinux / RHEL 9</option>
                <option value="freebsd14.0">FreeBSD 14 / OPNsense</option>
                <option value="win2k22">Windows Server 2022 / 11</option>
                <option value="generic">Generic Linux 6.x</option>
              </select>
            </div>
            <div class="form-group" style="margin: 0; display: flex; flex-direction: column; justify-content: flex-end; padding-bottom: 6px">
              <label style="display: flex; align-items: center; gap: 8px; font-size: 13px; margin: 0; cursor: pointer">
                <input v-model="wizardForm.autostart" type="checkbox" /> Autostart on boot
              </label>
            </div>
          </div>

          <!-- Hardware Configuration Table Card -->
          <div>
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px">
              <div style="font-weight: 600; font-size: 14px">
                Hardware Configuration
              </div>
              <div style="display: flex; gap: 8px">
                <button class="btn btn-xs" type="button" @click="addWizardDisk">
                  <MdiIcon :path="mdiPlus" size="13" /> Add Hard Disk
                </button>
                <button class="btn btn-xs" type="button" @click="addWizardNic">
                  <MdiIcon :path="mdiPlus" size="13" /> Add Network Device
                </button>
              </div>
            </div>

            <div class="table-wrap">
              <table class="data-table">
                <thead>
                  <tr>
                    <th style="width: 190px">Device</th>
                    <th>Configuration</th>
                    <th style="text-align: right; width: 130px">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  <!-- Processors -->
                  <tr style="cursor: pointer" title="Click to edit processors" @click="openWizardEdit('cpu')">
                    <td class="font-semibold">Processors</td>
                    <td class="mono" style="font-size: 12px">
                      {{ wizardForm.sockets * wizardForm.cores }} vCPU ({{ wizardForm.sockets }} sockets, {{ wizardForm.cores }} cores) [Type: {{ wizardForm.cpuType }}]
                      <span v-if="wizardForm.numa" class="badge badge-neutral" style="margin-left: 6px; font-size: 10px">NUMA</span>
                    </td>
                    <td style="text-align: right">
                      <button class="btn btn-xs" type="button" @click.stop="openWizardEdit('cpu')">
                        <MdiIcon :path="mdiPencil" size="12" /> Edit
                      </button>
                    </td>
                  </tr>

                  <!-- Memory -->
                  <tr style="cursor: pointer" title="Click to edit memory" @click="openWizardEdit('memory')">
                    <td class="font-semibold">Memory</td>
                    <td class="mono" style="font-size: 12px">
                      {{ formatBytes(wizardForm.memoryMib * 1024 * 1024) }}
                      (Fixed, Hugepages: {{ wizardForm.hugepages }})
                    </td>
                    <td style="text-align: right">
                      <button class="btn btn-xs" type="button" @click.stop="openWizardEdit('memory')">
                        <MdiIcon :path="mdiPencil" size="12" /> Edit
                      </button>
                    </td>
                  </tr>

                  <!-- Machine & BIOS -->
                  <tr style="cursor: pointer" title="Click to edit machine & BIOS" @click="openWizardEdit('platform')">
                    <td class="font-semibold">Machine & BIOS</td>
                    <td class="mono" style="font-size: 12px">
                      BIOS: {{ wizardForm.biosType.toUpperCase() }} | Machine: {{ wizardForm.machineType }} | TPM: {{ wizardForm.tpmVersion }}
                      <span v-if="wizardForm.secureBoot" class="badge badge-neutral" style="margin-left: 6px; font-size: 10px">Secure Boot</span>
                    </td>
                    <td style="text-align: right">
                      <button class="btn btn-xs" type="button" @click.stop="openWizardEdit('platform')">
                        <MdiIcon :path="mdiPencil" size="12" /> Edit
                      </button>
                    </td>
                  </tr>

                  <!-- Hard Disks -->
                  <tr v-for="(disk, idx) in wizardForm.disks" :key="disk.id">
                    <td class="font-semibold">
                      Hard Disk (<code class="mono">{{ disk.id }}</code>)
                    </td>
                    <td class="mono" style="font-size: 12px">
                      {{ disk.diskPath }} ({{ disk.sizeGib }} GiB, Bus: {{ disk.bus }}, Cache: {{ disk.cacheMode }}, Discard: {{ disk.discard ? 'on' : 'off' }}, IOThread: {{ disk.iothread ? 'on' : 'off' }})
                    </td>
                    <td style="text-align: right">
                      <div style="display: inline-flex; gap: 4px">
                        <button class="btn btn-xs" type="button" @click.stop="openEditWizardDisk(idx)">
                          <MdiIcon :path="mdiPencil" size="12" /> Edit
                        </button>
                        <button
                          v-if="wizardForm.disks.length > 1"
                          class="btn btn-xs btn-danger"
                          type="button"
                          title="Remove disk"
                          @click.stop="removeWizardDisk(idx)"
                        >
                          <MdiIcon :path="mdiTrashCan" size="12" />
                        </button>
                      </div>
                    </td>
                  </tr>

                  <!-- CD/DVD Drive (Select ISO image) -->
                  <tr>
                    <td class="font-semibold">CD/DVD Drive</td>
                    <td style="font-size: 12px">
                      <select v-model="wizardForm.bootIso" class="select select-sm mono" style="width: 100%; max-width: 440px">
                        <option value="">-- No Boot Media / PXE Network Boot --</option>
                        <option v-for="iso in nasState.isos" :key="iso.path" :value="iso.path">
                          {{ iso.name }} ({{ iso.sizeFormatted }})
                        </option>
                      </select>
                    </td>
                    <td style="text-align: right">
                      <button
                        v-if="wizardForm.bootIso"
                        class="btn btn-xs"
                        type="button"
                        title="Clear ISO image selection"
                        @click="wizardForm.bootIso = ''"
                      >
                        <MdiIcon :path="mdiClose" size="12" /> Clear
                      </button>
                      <span v-else class="text-muted" style="font-size: 11px">No ISO</span>
                    </td>
                  </tr>

                  <!-- Network Interfaces -->
                  <tr v-for="(nic, idx) in wizardForm.networkInterfaces" :key="nic.id">
                    <td class="font-semibold">
                      Network (<code class="mono">{{ nic.id }}</code>)
                    </td>
                    <td class="mono" style="font-size: 12px">
                      Bridge: {{ nic.bridge }}, Model: {{ nic.model }}
                      <span v-if="nic.vlanTag" style="color: var(--accent-primary)">, VLAN: {{ nic.vlanTag }}</span>
                      <span v-if="nic.firewall" style="color: var(--text-muted)">, Firewall: on</span>
                    </td>
                    <td style="text-align: right">
                      <div style="display: inline-flex; gap: 4px">
                        <button class="btn btn-xs" type="button" @click.stop="openEditWizardNic(idx)">
                          <MdiIcon :path="mdiPencil" size="12" /> Edit
                        </button>
                        <button
                          v-if="wizardForm.networkInterfaces.length > 1"
                          class="btn btn-xs btn-danger"
                          type="button"
                          title="Remove network interface"
                          @click.stop="removeWizardNic(idx)"
                        >
                          <MdiIcon :path="mdiTrashCan" size="12" />
                        </button>
                      </div>
                    </td>
                  </tr>

                  <!-- Display -->
                  <tr style="cursor: pointer" title="Click to edit display" @click="openWizardEdit('display')">
                    <td class="font-semibold">Display / Graphics</td>
                    <td class="mono" style="font-size: 12px">
                      {{ wizardForm.displayType || 'virtio-gpu' }} (VNC Display: Auto)
                    </td>
                    <td style="text-align: right">
                      <button class="btn btn-xs" type="button" @click.stop="openWizardEdit('display')">
                        <MdiIcon :path="mdiPencil" size="12" /> Edit
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- Cloud-Init Optional Collapsible -->
          <div style="background: rgba(255, 255, 255, 0.02); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 12px 16px">
            <div
              style="display: flex; justify-content: space-between; align-items: center; cursor: pointer; user-select: none"
              @click="wizardForm.showCloudInit = !wizardForm.showCloudInit"
            >
              <div style="display: flex; align-items: center; gap: 8px; font-weight: 600; font-size: 13px">
                <MdiIcon :path="mdiCog" size="16" style="color: var(--text-secondary)" />
                <span>Cloud-Init Configuration (Optional)</span>
              </div>
              <button type="button" class="btn btn-xs">
                {{ wizardForm.showCloudInit ? 'Collapse' : 'Expand' }}
              </button>
            </div>
            <div v-if="wizardForm.showCloudInit" class="form-grid" style="margin-top: 14px; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px">
              <div class="form-group">
                <label>Cloud-Init User</label>
                <input v-model="wizardForm.ciUser" class="input mono" placeholder="admin" />
              </div>
              <div class="form-group">
                <label>Password</label>
                <input v-model="wizardForm.ciPassword" type="password" class="input mono" placeholder="Leave empty for key-only" />
              </div>
              <div class="form-group" style="grid-column: span 2">
                <label>SSH Public Keys</label>
                <textarea v-model="wizardForm.ciSshKeys" class="input mono" rows="2" placeholder="ssh-ed25519 AAAA..."></textarea>
              </div>
            </div>
          </div>
        </div>

        <!-- Modal Footer -->
        <div style="flex-shrink: 0; padding: 14px 20px; border-top: 1px solid var(--border-subtle); display: flex; justify-content: space-between; align-items: center">
          <button type="button" class="btn" @click="showCreateWizard = false">
            Cancel
          </button>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="!wizardForm.vmName || !wizardForm.vmId"
            @click="handleWizardSubmit"
          >
            Create Virtual Machine
          </button>
        </div>
      </div>
    </div>

    <!-- WIZARD SUB-MODAL: CPU -->
    <div v-if="wizardEditModal === 'cpu'" class="modal-backdrop" style="z-index: 1050">
      <div class="modal-card" style="max-width: 480px">
        <div class="modal-header">
          <h3 class="modal-title">Edit Processors</h3>
          <button class="modal-close-btn" type="button" aria-label="Close" @click="wizardEditModal = null">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>
        <div class="modal-body form-grid">
          <div class="form-group">
            <label>Sockets</label>
            <select v-model.number="wizardForm.sockets" class="select mono">
              <option v-for="n in socketOptions" :key="n" :value="n">{{ n }}</option>
            </select>
          </div>
          <div class="form-group">
            <label>Cores</label>
            <select v-model.number="wizardForm.cores" class="select mono">
              <option v-for="n in coreOptionsFor(wizardForm.cores)" :key="n" :value="n">{{ n }}</option>
            </select>
          </div>
          <div class="form-group">
            <label>CPU Type / Model</label>
            <select v-model="wizardForm.cpuType" class="select mono">
              <option value="host">host (Pass-through host CPU)</option>
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
        <div class="modal-footer">
          <button type="button" class="btn btn-primary" @click="wizardEditModal = null">Done</button>
        </div>
      </div>
    </div>

    <!-- WIZARD SUB-MODAL: MEMORY -->
    <div v-if="wizardEditModal === 'memory'" class="modal-backdrop" style="z-index: 1050">
      <div class="modal-card" style="max-width: 480px">
        <div class="modal-header">
          <h3 class="modal-title">Edit Memory</h3>
          <button class="modal-close-btn" type="button" aria-label="Close" @click="wizardEditModal = null">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>
        <div class="modal-body form-grid">
          <div class="form-group" style="grid-column: span 2">
            <label>Memory Size (MiB)</label>
            <input v-model.number="wizardForm.memoryMib" type="number" min="512" step="256" class="input mono" required />
            <span class="helper">{{ formatBytes(wizardForm.memoryMib * 1024 * 1024) }}</span>
          </div>
          <div class="form-group" style="grid-column: span 2">
            <label>Hugepages</label>
            <select v-model="wizardForm.hugepages" class="select mono">
              <option value="off">Disabled (Standard 4KiB pages)</option>
              <option value="2M">2 MiB Hugepages</option>
              <option value="1G">1 GiB Hugepages</option>
            </select>
          </div>
        </div>
        <div class="modal-footer">
          <button type="button" class="btn btn-primary" @click="wizardEditModal = null">Done</button>
        </div>
      </div>
    </div>

    <!-- WIZARD SUB-MODAL: PLATFORM / BIOS -->
    <div v-if="wizardEditModal === 'platform'" class="modal-backdrop" style="z-index: 1050">
      <div class="modal-card" style="max-width: 480px">
        <div class="modal-header">
          <h3 class="modal-title">Edit Machine & BIOS</h3>
          <button class="modal-close-btn" type="button" aria-label="Close" @click="wizardEditModal = null">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>
        <div class="modal-body form-grid">
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
              <option value="v2.0">TPM 2.0 (Required for Windows 11)</option>
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
        <div class="modal-footer">
          <button type="button" class="btn btn-primary" @click="wizardEditModal = null">Done</button>
        </div>
      </div>
    </div>

    <!-- WIZARD SUB-MODAL: HARD DISK -->
    <div v-if="wizardEditModal === 'disk'" class="modal-backdrop" style="z-index: 1050">
      <div class="modal-card" style="max-width: 520px">
        <div class="modal-header">
          <h3 class="modal-title">Edit Hard Disk ({{ editingDiskBuffer.id }})</h3>
          <button class="modal-close-btn" type="button" aria-label="Close" @click="wizardEditModal = null">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>
        <div class="modal-body form-grid">
          <div class="form-group">
            <label>Bus Interface</label>
            <select v-model="editingDiskBuffer.bus" class="select mono">
              <option value="scsi">VirtIO SCSI (Recommended)</option>
              <option value="virtio">VirtIO Block</option>
              <option value="sata">SATA</option>
              <option value="nvme">NVMe</option>
            </select>
          </div>
          <div class="form-group">
            <label>Disk Size (GiB)</label>
            <input v-model.number="editingDiskBuffer.sizeGib" type="number" min="1" class="input mono" required />
          </div>
          <div class="form-group" style="grid-column: span 2">
            <label>Storage Path (Raw, QCOW2 or ZVol)</label>
            <input
              v-model="editingDiskBuffer.diskPath"
              list="zvol-list-wiz"
              class="input mono"
              :placeholder="suggestedDiskPath"
            />
            <datalist id="zvol-list-wiz">
              <option v-for="zpath in availableZvols" :key="zpath" :value="zpath" />
            </datalist>
          </div>
          <div class="form-group">
            <label>Cache Mode</label>
            <select v-model="editingDiskBuffer.cacheMode" class="select mono">
              <option value="none">none (Direct I/O - Recommended)</option>
              <option value="writethrough">writethrough</option>
              <option value="writeback">writeback</option>
              <option value="unsafe">unsafe</option>
            </select>
          </div>
          <div class="form-group" style="display: flex; flex-direction: column; justify-content: center; gap: 8px">
            <label style="display: flex; align-items: center; gap: 8px; font-size: 13px; margin: 0; cursor: pointer">
              <input v-model="editingDiskBuffer.discard" type="checkbox" /> Discard / TRIM
            </label>
            <label style="display: flex; align-items: center; gap: 8px; font-size: 13px; margin: 0; cursor: pointer">
              <input v-model="editingDiskBuffer.iothread" type="checkbox" /> Dedicated IOThread
            </label>
          </div>
        </div>
        <div class="modal-footer">
          <button type="button" class="btn" @click="wizardEditModal = null">Cancel</button>
          <button type="button" class="btn btn-primary" @click="saveEditWizardDisk">Save</button>
        </div>
      </div>
    </div>

    <!-- WIZARD SUB-MODAL: NETWORK -->
    <div v-if="wizardEditModal === 'nic'" class="modal-backdrop" style="z-index: 1050">
      <div class="modal-card" style="max-width: 480px">
        <div class="modal-header">
          <h3 class="modal-title">Edit Network Device ({{ editingNicBuffer.id }})</h3>
          <button class="modal-close-btn" type="button" aria-label="Close" @click="wizardEditModal = null">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>
        <div class="modal-body form-grid">
          <div class="form-group">
            <label>Model</label>
            <select v-model="editingNicBuffer.model" class="select mono">
              <option value="virtio">VirtIO (paravirtualized)</option>
              <option value="e1000">Intel E1000</option>
              <option value="rtl8139">Realtek RTL8139</option>
            </select>
          </div>
          <div class="form-group">
            <label>Bridge / LAN Interface</label>
            <select v-model="editingNicBuffer.bridge" class="select mono" required>
              <option v-for="b in bridgeOptionsFor(editingNicBuffer.bridge)" :key="b.name" :value="b.name">{{ b.label }}</option>
            </select>
          </div>
          <div class="form-group">
            <label>VLAN Tag (0 = Untagged)</label>
            <input v-model.number="editingNicBuffer.vlanTag" type="number" min="0" max="4094" class="input mono" />
          </div>
          <div class="form-group" style="display: flex; align-items: center; padding-top: 20px">
            <label style="display: flex; align-items: center; gap: 8px; font-size: 13px; margin: 0; cursor: pointer">
              <input v-model="editingNicBuffer.firewall" type="checkbox" /> MAC Spoofing Filter
            </label>
          </div>
        </div>
        <div class="modal-footer">
          <button type="button" class="btn" @click="wizardEditModal = null">Cancel</button>
          <button type="button" class="btn btn-primary" @click="saveEditWizardNic">Save</button>
        </div>
      </div>
    </div>

    <!-- WIZARD SUB-MODAL: DISPLAY -->
    <div v-if="wizardEditModal === 'display'" class="modal-backdrop" style="z-index: 1050">
      <div class="modal-card" style="max-width: 440px">
        <div class="modal-header">
          <h3 class="modal-title">Edit Display / Graphics</h3>
          <button class="modal-close-btn" type="button" aria-label="Close" @click="wizardEditModal = null">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>
        <div class="modal-body form-grid">
          <div class="form-group" style="grid-column: span 2">
            <label>Graphics Controller</label>
            <select v-model="wizardForm.displayType" class="select mono">
              <option value="virtio-gpu">virtio-gpu (Recommended, 3D accelerated)</option>
              <option value="qxl">qxl (SPICE / 2D optimized)</option>
              <option value="std">std (Standard VGA compatible)</option>
              <option value="cirrus">cirrus (Legacy)</option>
            </select>
          </div>
        </div>
        <div class="modal-footer">
          <button type="button" class="btn btn-primary" @click="wizardEditModal = null">Done</button>
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
/* VM power state is conveyed by a faded row tint instead of a Status column */
.vm-row td:first-child {
  box-shadow: inset 3px 0 0 var(--vm-state-color, transparent);
}

.vm-row {
  background: linear-gradient(
    90deg,
    color-mix(in srgb, var(--vm-state-color, transparent) 16%, transparent) 0%,
    color-mix(in srgb, var(--vm-state-color, transparent) 5%, transparent) 45%,
    transparent 100%
  );
}

.vm-row-running {
  --vm-state-color: var(--success, #22c55e);
}

.vm-row-stopped,
.vm-row-shutoff,
.vm-row-crashed {
  --vm-state-color: var(--danger, #ef4444);
}

.vm-row-paused {
  --vm-state-color: var(--warning, #f59e0b);
}

.vm-row td:last-child {
  white-space: nowrap;
}

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
