<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import {
  mdiAccountGroup,
  mdiArrowLeft,
  mdiCamera,
  mdiCircle,
  mdiClose,
  mdiCogOutline,
  mdiConsole,
  mdiContentCopy,
  mdiCubeOutline,
  mdiDatabase,
  mdiDatabaseCheck,
  mdiDatabaseSync,
  mdiExport,
  mdiFileDocumentOutline,
  mdiFolderNetwork,
  mdiFolderOutline,
  mdiHarddisk,
  mdiInformationOutline,
  mdiLayers,
  mdiMonitor,
  mdiOpenInNew,
  mdiPause,
  mdiPlay,
  mdiPower,
  mdiRefresh,
  mdiRestart,
  mdiServer,
  mdiSnowflake,
  mdiStop,
  mdiTrashCanOutline,
  mdiUpdate,
  mdiWeb,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import { services } from '../../container';
import {
  nasState,
  type NavTab,
  notify,
  refreshAllData,
  runTrackedTask,
  selectResource,
} from '../store';
import { navigateTo } from '../router';
import { confirmDelete, confirmModal } from '../modal';
import { computeStorageBlastRadius } from '../utils/storageBlastRadius';
import { toZfsSafeSlug } from '../../domain/id';
import type { VirtualMachine, LxcContainer, ZfsPool, DockerApp } from '../../domain/models';

const emit = defineEmits<{
  (e: 'open-host-terminal'): void;
  (e: 'open-system-details'): void;
  (e: 'open-prerequisites'): void;
  (e: 'open-vm-console', vm: VirtualMachine): void;
  (e: 'open-lxc-terminal', ct: LxcContainer): void;
}>();

const selectedId = computed(() => nasState.selectedResourceId || 'node:local');

// Target item resolution
const currentVm = computed<VirtualMachine | null>(() => {
  if (!selectedId.value.startsWith('vm:')) return null;
  const name = selectedId.value.replace('vm:', '');
  return (
    nasState.vms.find(
      (v) =>
        v.name === name ||
        v.name === `virtualMachines/${name}` ||
        v.vmName === name
    ) || null
  );
});

const currentLxc = computed<LxcContainer | null>(() => {
  if (!selectedId.value.startsWith('lxc:')) return null;
  const name = selectedId.value.replace('lxc:', '');
  return (
    nasState.lxcContainers.find(
      (c) =>
        c.name === name ||
        c.name === `lxcContainers/${name}` ||
        c.containerName === name
    ) || null
  );
});

const currentPool = computed<ZfsPool | null>(() => {
  if (!selectedId.value.startsWith('pool:')) return null;
  const name = selectedId.value.replace('pool:', '');
  return (
    nasState.pools.find(
      (p) =>
        p.name === name ||
        p.name === `zfsPools/${name}` ||
        (p.poolName || p.name.replace(/^zfsPools\//, '')) === name
    ) || null
  );
});

const currentDocker = computed<DockerApp | null>(() => {
  if (!selectedId.value.startsWith('docker:')) return null;
  const name = selectedId.value.replace('docker:', '');
  return (
    nasState.dockerApps.find(
      (a) =>
        a.name === name ||
        a.name === `dockerApps/${name}` ||
        a.appId === name ||
        a.title === name
    ) || null
  );
});

const isNode = computed(
  () =>
    selectedId.value === 'node:local' ||
    selectedId.value === 'node:disks' ||
    selectedId.value === 'node:shares' ||
    selectedId.value === 'node:identity'
);

const isBusy = ref(false);

// -----------------------------------------------------------------------------
// VM Operations
// -----------------------------------------------------------------------------
async function startVm() {
  if (!currentVm.value || isBusy.value) return;
  const vm = currentVm.value;
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${vm.vmName}": Start`, async () => {
      await services.kvm.startVirtualMachine(vm.name);
    });
    notify('success', `Virtual machine "${vm.vmName}" started`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to start VM: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function shutdownVm() {
  if (!currentVm.value || isBusy.value) return;
  const vm = currentVm.value;
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${vm.vmName}": ACPI Shutdown`, async () => {
      await services.kvm.stopVirtualMachine(vm.name, false);
    });
    notify('info', `Shutdown signal sent to "${vm.vmName}"`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to stop VM: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function forceStopVm() {
  if (!currentVm.value || isBusy.value) return;
  const vm = currentVm.value;
  const ok = await confirmModal({
    title: `Force Power Off "${vm.vmName}"?`,
    message: 'Forcibly terminating power is equivalent to pulling the power plug. Unsaved data and guest filesystems may become corrupted.',
    tone: 'danger',
    confirmText: 'Force Power Off',
  });
  if (!ok) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${vm.vmName}": Force Power Off`, async () => {
      await services.kvm.stopVirtualMachine(vm.name, true);
    });
    notify('info', `Virtual machine "${vm.vmName}" power terminated`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to force stop VM: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function rebootVm() {
  if (!currentVm.value || isBusy.value) return;
  const vm = currentVm.value;
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${vm.vmName}": Reboot`, async () => {
      await services.kvm.rebootVirtualMachine(vm.name, false);
    });
    notify('info', `Reboot signal sent to "${vm.vmName}"`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to reboot VM: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function deleteVm() {
  if (!currentVm.value || isBusy.value) return;
  const vm = currentVm.value;
  const confirmed = await confirmDelete({
    kind: 'Virtual Machine',
    name: vm.vmName,
    details: 'The virtual machine definition and guest configuration will be removed.',
    confirmText: 'Delete VM',
  });
  if (!confirmed.confirmed) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${vm.vmName}": Delete`, async () => {
      await services.kvm.deleteVirtualMachine(vm.name);
    });
    notify('warning', `Virtual machine "${vm.vmName}" deleted`);
    selectResource('node:vms');
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to delete VM: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

function popoutVmConsole() {
  if (!currentVm.value) return;
  const clean = currentVm.value.name.replace(/^virtualMachines\//, '');
  const url = `/vnc-console?vm=${encodeURIComponent(clean)}`;
  window.open(
    url,
    `vnc_${clean}`,
    'width=1024,height=768,location=no,toolbar=no,menubar=no,status=no,resizable=yes'
  );
}

async function pauseVm() {
  if (!currentVm.value || isBusy.value) return;
  const vm = currentVm.value;
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${vm.vmName}": Pause`, async () => {
      await services.kvm.pauseVirtualMachine(vm.name);
    });
    notify('info', `Virtual machine "${vm.vmName}" paused`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to pause VM: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function resumeVm() {
  if (!currentVm.value || isBusy.value) return;
  const vm = currentVm.value;
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${vm.vmName}": Resume`, async () => {
      await services.kvm.resumeVirtualMachine(vm.name);
    });
    notify('success', `Virtual machine "${vm.vmName}" resumed`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to resume VM: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

// Clone VM Modal State
const showCloneVmModal = ref(false);
const cloneForm = reactive({
  targetVmName: '',
  targetVmId: '',
  fullClone: true,
});

function openCloneVm() {
  if (!currentVm.value) return;
  cloneForm.targetVmName = `${currentVm.value.vmName}-clone`;
  cloneForm.targetVmId = toZfsSafeSlug(`${currentVm.value.vmName}-clone`);
  cloneForm.fullClone = true;
  showCloneVmModal.value = true;
}

async function submitCloneVm() {
  if (!currentVm.value || !cloneForm.targetVmName.trim() || isBusy.value) return;
  const vm = currentVm.value;
  const targetId = toZfsSafeSlug(cloneForm.targetVmId.trim() || cloneForm.targetVmName.trim());
  isBusy.value = true;
  try {
    await runTrackedTask(`VM "${vm.vmName}": Clone to "${cloneForm.targetVmName}"`, async () => {
      const targetPool =
        nasState.systemConfig?.mainPool ||
        nasState.pools[0]?.poolName ||
        nasState.pools[0]?.name.replace(/^zfsPools\//, '') ||
        'tank';
      await services.kvm.cloneVirtualMachine(
        vm.name,
        targetId,
        cloneForm.targetVmName.trim(),
        targetPool,
        !cloneForm.fullClone
      );
    });
    notify('success', `Cloned VM "${vm.vmName}" to "${cloneForm.targetVmName}"`);
    showCloneVmModal.value = false;
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Clone failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

function popoutLxcTerminal() {
  if (!currentLxc.value) return;
  const clean = currentLxc.value.name.replace(/^lxcContainers\//, '');
  const url = `/terminal?lxc=${encodeURIComponent(clean)}`;
  window.open(
    url,
    `lxc_terminal_${clean}`,
    'width=1000,height=650,location=no,toolbar=no,menubar=no,status=no,resizable=yes'
  );
}

function handleNav(tab: NavTab) {
  navigateTo(tab);
}

// -----------------------------------------------------------------------------
// LXC Operations
// -----------------------------------------------------------------------------
async function startLxc() {
  if (!currentLxc.value || isBusy.value) return;
  const ct = currentLxc.value;
  isBusy.value = true;
  try {
    await runTrackedTask(`LXC "${ct.containerName || ct.name}": Start`, async () => {
      await services.lxc.startContainer(ct.name);
    });
    notify('success', `Container "${ct.containerName || ct.name}" started`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to start container: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function stopLxc() {
  if (!currentLxc.value || isBusy.value) return;
  const ct = currentLxc.value;
  isBusy.value = true;
  try {
    await runTrackedTask(`LXC "${ct.containerName || ct.name}": Stop`, async () => {
      await services.lxc.stopContainer(ct.name);
    });
    notify('info', `Container "${ct.containerName || ct.name}" stopped`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to stop container: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function restartLxc() {
  if (!currentLxc.value || isBusy.value) return;
  const ct = currentLxc.value;
  isBusy.value = true;
  try {
    await runTrackedTask(`LXC "${ct.containerName || ct.name}": Reboot`, async () => {
      await services.lxc.restartContainer(ct.name);
    });
    notify('info', `Container "${ct.containerName || ct.name}" restarted`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to restart container: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function toggleFreezeLxc() {
  if (!currentLxc.value || isBusy.value) return;
  const ct = currentLxc.value;
  const isFrozen = ct.state === 'FROZEN';
  isBusy.value = true;
  try {
    await runTrackedTask(`LXC "${ct.containerName || ct.name}": ${isFrozen ? 'Unfreeze' : 'Freeze'}`, async () => {
      if (isFrozen) {
        await services.lxc.startContainer(ct.name);
      } else {
        await services.lxc.freezeContainer(ct.name);
      }
    });
    notify('info', `Container "${ct.containerName || ct.name}" ${isFrozen ? 'unfrozen' : 'frozen'}`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Action failed: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function deleteLxc() {
  if (!currentLxc.value || isBusy.value) return;
  const ct = currentLxc.value;
  const clean = ct.containerName || ct.name.replace(/^lxcContainers\//, '');
  const confirmed = await confirmDelete({
    kind: 'LXC Container',
    name: clean,
    details: 'The container root filesystem, state, and host configuration will be destroyed.',
    confirmText: 'Delete Container',
  });
  if (!confirmed.confirmed) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`LXC "${clean}": Delete`, async () => {
      await services.lxc.deleteContainer(ct.name);
    });
    notify('warning', `LXC container "${clean}" deleted`);
    selectResource('node:lxc');
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to delete container: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

// -----------------------------------------------------------------------------
// Docker Operations
// -----------------------------------------------------------------------------
async function startDocker() {
  if (!currentDocker.value || isBusy.value) return;
  const app = currentDocker.value;
  const appDisplayName = app.title || app.appId || app.name.replace(/^dockerApps\//, '');
  isBusy.value = true;
  try {
    await runTrackedTask(`App "${appDisplayName}": Start`, async () => {
      await services.apps.startApp(app.name);
    });
    notify('success', `Application "${appDisplayName}" started`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to start app: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function stopDocker() {
  if (!currentDocker.value || isBusy.value) return;
  const app = currentDocker.value;
  const appDisplayName = app.title || app.appId || app.name.replace(/^dockerApps\//, '');
  isBusy.value = true;
  try {
    await runTrackedTask(`App "${appDisplayName}": Stop`, async () => {
      await services.apps.stopApp(app.name);
    });
    notify('info', `Application "${appDisplayName}" stopped`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to stop app: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function restartDocker() {
  if (!currentDocker.value || isBusy.value) return;
  const app = currentDocker.value;
  const appDisplayName = app.title || app.appId || app.name.replace(/^dockerApps\//, '');
  isBusy.value = true;
  try {
    await runTrackedTask(`App "${appDisplayName}": Restart`, async () => {
      await services.apps.restartApp(app.name);
    });
    notify('info', `Application "${appDisplayName}" restarted`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to restart app: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function deleteDocker() {
  if (!currentDocker.value || isBusy.value) return;
  const app = currentDocker.value;
  const appDisplayName = app.title || app.appId || app.name.replace(/^dockerApps\//, '');
  const confirmed = await confirmDelete({
    kind: 'Docker Application',
    name: appDisplayName,
    details: 'The Docker Compose service stack, containers and virtual network will be removed.',
    confirmText: 'Delete App',
    checkbox: {
      label: 'Also permanently delete application persistent volumes and data (-v)',
      checked: false,
    },
  });
  if (!confirmed.confirmed) return;
  const purgeVolumes = Boolean(confirmed.checkboxChecked);
  isBusy.value = true;
  try {
    await runTrackedTask(`App "${appDisplayName}": Delete`, async () => {
      await services.apps.deleteApp(app.name, purgeVolumes);
    });
    notify('warning', `Application "${appDisplayName}" deleted`);
    selectResource('node:docker');
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to delete app: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function upgradeDocker() {
  if (!currentDocker.value || isBusy.value) return;
  const app = currentDocker.value;
  const appDisplayName = app.title || app.appId || app.name.replace(/^dockerApps\//, '');
  isBusy.value = true;
  try {
    await runTrackedTask(`App "${appDisplayName}": Upgrade`, async () => {
      await services.apps.upgradeApp(app.name, true);
    });
    notify('success', `Upgrade completed for "${appDisplayName}"`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to upgrade app: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

// -----------------------------------------------------------------------------
// Storage Pool Operations
// -----------------------------------------------------------------------------
const isMainPool = computed(() => {
  if (!currentPool.value) return false;
  const name = currentPool.value.poolName || currentPool.value.name.replace(/^zfsPools\//, '');
  return nasState.systemConfig?.mainPool === name;
});

async function scrubPool() {
  if (!currentPool.value || isBusy.value) return;
  const pool = currentPool.value;
  isBusy.value = true;
  try {
    await runTrackedTask(`ZFS Pool "${pool.poolName || pool.name}": Start Scrub`, async () => {
      await services.zfs.scrubPool(pool.name);
    });
    notify('success', `Scrub started on pool "${pool.poolName || pool.name}"`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to scrub pool: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function setMainPool() {
  if (!currentPool.value || isBusy.value) return;
  const pool = currentPool.value;
  const name = pool.poolName || pool.name.replace(/^zfsPools\//, '');
  isBusy.value = true;
  try {
    await runTrackedTask(`System: Set Main Pool to "${name}"`, async () => {
      await services.system.selectMainPool(name, true);
    });
    notify('success', `Designated "${name}" as main storage pool`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to set main pool: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function exportPool() {
  if (!currentPool.value || isBusy.value) return;
  const pool = currentPool.value;
  const name = pool.poolName || pool.name.replace(/^zfsPools\//, '');
  const confirmed = await confirmDelete({
    kind: 'Export ZFS Pool',
    name,
    details: isMainPool.value
      ? 'WARNING: This is the MAIN POOL. Exporting it will pause Apps and Containers!'
      : 'The pool will be unmounted and safely detached. You can re-import it anytime.',
    confirmText: 'Export Pool',
  });
  if (!confirmed.confirmed) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`ZFS Pool "${name}": Export`, async () => {
      await services.zfs.exportPool(pool.name);
    });
    notify('info', `Pool "${name}" exported`);
    selectResource('node:pools');
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to export pool: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

async function destroyPool() {
  if (!currentPool.value || isBusy.value) return;
  const pool = currentPool.value;
  const name = pool.poolName || pool.name.replace(/^zfsPools\//, '');
  const blast = computeStorageBlastRadius(name);
  const details = blast.hasDependencies
    ? `CRITICAL DANGER: ALL DATA, DATASETS, AND VOLUMES ON THIS POOL WILL BE PERMANENTLY DESTROYED.\n\nACTIVE DEPENDENCIES DETECTED:\n${blast.summaryWarning}`
    : 'CRITICAL DANGER: ALL DATA, DATASETS, AND VOLUMES ON THIS POOL WILL BE PERMANENTLY DESTROYED.';
  const confirmed = await confirmDelete({
    kind: 'Pool',
    name,
    details,
    confirmText: 'Destroy Pool Permanently',
  });
  if (!confirmed.confirmed) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`ZFS Pool "${name}": Destroy`, async () => {
      await services.zfs.deletePool(pool.name);
    });
    notify('warning', `ZFS pool "${name}" destroyed`);
    selectResource('node:pools');
    await refreshAllData();
  } catch (err: any) {
    notify('error', `Failed to destroy pool: ${err.message}`);
  } finally {
    isBusy.value = false;
  }
}

function popoutHostTerminal() {
  window.open(
    '/terminal',
    'host_terminal',
    'width=1000,height=650,location=no,toolbar=no,menubar=no,status=no,resizable=yes'
  );
}
</script>

<template>
  <div class="action-ribbon">
    <!-- Ribbon Left: Resource Identity, Status Bullet & Live Telemetry -->
    <div class="ribbon-identity">
      <!-- VM Identity -->
      <template v-if="currentVm">
        <button
          class="btn btn-xs btn-ghost ribbon-back-btn"
          title="Back to Virtual Machines"
          @click="selectResource('node:vms'); handleNav('kvm')"
        >
          <MdiIcon :path="mdiArrowLeft" size="15" />
        </button>
        <MdiIcon :path="mdiMonitor" size="16" style="color: var(--accent-primary)" />
        <MdiIcon
          :path="mdiCircle"
          size="9"
          :style="{
            color:
              currentVm.state === 'RUNNING'
                ? 'var(--success)'
                : currentVm.state === 'PAUSED'
                ? 'var(--warning)'
                : 'var(--text-muted)',
          }"
          :title="`Status: ${(currentVm.state || 'STOPPED').toLowerCase()}`"
        />
        <span class="ribbon-name">{{ currentVm.vmName }}</span>
        <span
          v-if="currentVm.state === 'RUNNING' && currentVm.guestCpuPercent !== undefined"
          class="ribbon-metric font-xs"
        >
          CPU: {{ currentVm.guestCpuPercent.toFixed(1) }}%
        </span>
        <span
          v-if="currentVm.state === 'RUNNING' && currentVm.guestMemoryUsedMib"
          class="ribbon-metric font-xs"
        >
          RAM: {{ currentVm.guestMemoryUsedMib >= 1024 ? (currentVm.guestMemoryUsedMib / 1024).toFixed(2) + ' GiB' : currentVm.guestMemoryUsedMib + ' MiB' }}
        </span>
      </template>

      <!-- LXC Identity -->
      <template v-else-if="currentLxc">
        <button
          class="btn btn-xs btn-ghost ribbon-back-btn"
          title="Back to LXC Containers"
          @click="selectResource('node:lxc'); handleNav('lxc')"
        >
          <MdiIcon :path="mdiArrowLeft" size="15" />
        </button>
        <MdiIcon :path="mdiCubeOutline" size="16" style="color: var(--accent-secondary)" />
        <MdiIcon
          v-if="currentLxc.state === 'FROZEN'"
          :path="mdiSnowflake"
          size="11"
          style="color: var(--info)"
          title="Status: frozen"
        />
        <MdiIcon
          v-else
          :path="mdiCircle"
          size="9"
          :style="{
            color: currentLxc.state === 'RUNNING' ? 'var(--success)' : 'var(--text-muted)',
          }"
          :title="`Status: ${(currentLxc.state || 'STOPPED').toLowerCase()}`"
        />
        <span class="ribbon-name">{{ currentLxc.containerName || currentLxc.name.replace('lxcContainers/', '') }}</span>
        <span
          v-if="currentLxc.state === 'RUNNING' && currentLxc.cpuPercent !== undefined"
          class="ribbon-metric font-xs"
        >
          CPU: {{ currentLxc.cpuPercent.toFixed(1) }}%
        </span>
        <span
          v-if="currentLxc.state === 'RUNNING' && currentLxc.memoryUsedMib"
          class="ribbon-metric font-xs"
        >
          RAM: {{ currentLxc.memoryUsedMib >= 1024 ? (currentLxc.memoryUsedMib / 1024).toFixed(2) + ' GiB' : currentLxc.memoryUsedMib + ' MiB' }}
        </span>
      </template>

      <!-- Pool Identity -->
      <template v-else-if="currentPool">
        <button
          class="btn btn-xs btn-ghost ribbon-back-btn"
          title="Back to Storage Pools"
          @click="selectResource('node:pools'); handleNav('storage-pools')"
        >
          <MdiIcon :path="mdiArrowLeft" size="15" />
        </button>
        <MdiIcon :path="mdiDatabase" size="16" style="color: var(--warning-text)" />
        <MdiIcon
          :path="mdiCircle"
          size="9"
          :style="{
            color: currentPool.state === 'ONLINE' ? 'var(--success)' : 'var(--danger)',
          }"
          :title="`Status: ${(currentPool.state || 'ONLINE').toLowerCase()}`"
        />
        <span class="ribbon-name">{{ currentPool.poolName || currentPool.name }}</span>
      </template>

      <!-- Docker Identity -->
      <template v-else-if="currentDocker">
        <button
          class="btn btn-xs btn-ghost ribbon-back-btn"
          title="Back to Docker Applications"
          @click="selectResource('node:docker'); handleNav('docker')"
        >
          <MdiIcon :path="mdiArrowLeft" size="15" />
        </button>
        <MdiIcon :path="mdiLayers" size="16" style="color: var(--info-text)" />
        <MdiIcon
          :path="mdiCircle"
          size="9"
          :style="{
            color:
              currentDocker.state === 'RUNNING'
                ? 'var(--success)'
                : currentDocker.state === 'PARTIALLY_RUNNING'
                ? 'var(--warning)'
                : 'var(--text-muted)',
          }"
          :title="`Status: ${(currentDocker.state || 'STOPPED').toLowerCase()}`"
        />
        <span class="ribbon-name">{{ currentDocker.title || currentDocker.appId || currentDocker.name.replace(/^dockerApps\//, '') }}</span>
        <span
          v-if="currentDocker.state === 'RUNNING' && currentDocker.totalCpuPercent !== undefined"
          class="ribbon-metric font-xs"
        >
          CPU: {{ currentDocker.totalCpuPercent.toFixed(1) }}%
        </span>
        <span
          v-if="currentDocker.state === 'RUNNING' && currentDocker.totalMemoryMib"
          class="ribbon-metric font-xs"
        >
          RAM: {{ currentDocker.totalMemoryMib >= 1024 ? (currentDocker.totalMemoryMib / 1024).toFixed(2) + ' GiB' : currentDocker.totalMemoryMib + ' MiB' }}
        </span>
      </template>

      <!-- Collection / Node Section Identity -->
      <template v-else-if="nasState.activeTab === 'kvm'">
        <MdiIcon :path="mdiMonitor" size="16" style="color: var(--accent-primary)" />
        <span class="ribbon-name">Virtual Machines</span>
      </template>
      <template v-else-if="nasState.activeTab === 'lxc'">
        <MdiIcon :path="mdiCubeOutline" size="16" style="color: var(--accent-secondary)" />
        <span class="ribbon-name">LXC Containers</span>
      </template>
      <template v-else-if="nasState.activeTab === 'docker'">
        <MdiIcon :path="mdiLayers" size="16" style="color: var(--info-text)" />
        <span class="ribbon-name">Applications</span>
      </template>
      <template
        v-else-if="
          nasState.activeTab === 'storage-pools' ||
          nasState.activeTab === 'storage-disks' ||
          nasState.activeTab === 'storage-datasets' ||
          nasState.activeTab === 'storage-volumes' ||
          nasState.activeTab === 'zfs'
        "
      >
        <MdiIcon :path="mdiDatabase" size="16" style="color: var(--warning-text)" />
        <span class="ribbon-name">Storage</span>
      </template>
      <template
        v-else-if="
          nasState.activeTab === 'shares' ||
          nasState.activeTab === 'shares-cifs' ||
          nasState.activeTab === 'shares-webdav'
        "
      >
        <MdiIcon :path="mdiFolderNetwork" size="16" style="color: var(--accent-primary)" />
        <span class="ribbon-name">Network Shares</span>
      </template>
      <template v-else-if="nasState.activeTab === 'identity'">
        <MdiIcon :path="mdiAccountGroup" size="16" style="color: var(--accent-secondary)" />
        <span class="ribbon-name">Users & Groups</span>
      </template>
      <template v-else>
        <MdiIcon :path="mdiServer" size="16" style="color: var(--accent-primary)" />
        <span class="ribbon-name">{{ nasState.systemTelemetry?.host?.hostname || 'nas01' }}</span>
        <span v-if="nasState.systemTelemetry?.host?.uptimeSeconds" class="ribbon-metric font-xs">
          Uptime: {{ Math.floor(nasState.systemTelemetry.host.uptimeSeconds / 86400) }}d {{ Math.floor((nasState.systemTelemetry.host.uptimeSeconds % 86400) / 3600) }}h
        </span>
      </template>
    </div>

    <!-- Ribbon Center: Unified Component & Section Tabs -->
    <div class="ribbon-tabs">
      <!-- VM Subtabs -->
      <template v-if="currentVm">
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.vmSubTab === 'summary' }"
          @click="nasState.vmSubTab = 'summary'"
        >
          <MdiIcon :path="mdiInformationOutline" size="14" />
          <span>Summary</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.vmSubTab === 'console' }"
          @click="nasState.vmSubTab = 'console'"
        >
          <MdiIcon :path="mdiConsole" size="14" />
          <span>Console</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.vmSubTab === 'configuration' || nasState.vmSubTab === 'hardware' }"
          @click="nasState.vmSubTab = 'configuration'"
        >
          <MdiIcon :path="mdiCogOutline" size="14" />
          <span>Configuration</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.vmSubTab === 'snapshots' }"
          @click="nasState.vmSubTab = 'snapshots'"
        >
          <MdiIcon :path="mdiCamera" size="14" />
          <span>Snapshots</span>
        </button>
      </template>

      <!-- LXC Subtabs -->
      <template v-else-if="currentLxc">
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.lxcSubTab === 'summary' }"
          @click="nasState.lxcSubTab = 'summary'"
        >
          <MdiIcon :path="mdiInformationOutline" size="14" />
          <span>Summary</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.lxcSubTab === 'console' || nasState.lxcSubTab === 'terminal' }"
          @click="nasState.lxcSubTab = 'console'"
        >
          <MdiIcon :path="mdiConsole" size="14" />
          <span>Console</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.lxcSubTab === 'configuration' || nasState.lxcSubTab === 'mounts' }"
          @click="nasState.lxcSubTab = 'configuration'"
        >
          <MdiIcon :path="mdiCogOutline" size="14" />
          <span>Configuration</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.lxcSubTab === 'logs' }"
          @click="nasState.lxcSubTab = 'logs'"
        >
          <MdiIcon :path="mdiFileDocumentOutline" size="14" />
          <span>Logs</span>
        </button>
      </template>

      <!-- Docker App Subtabs -->
      <template v-else-if="currentDocker">
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.dockerSubTab === 'summary' }"
          @click="nasState.dockerSubTab = 'summary'"
        >
          <MdiIcon :path="mdiInformationOutline" size="14" />
          <span>Summary</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.dockerSubTab === 'console' }"
          @click="nasState.dockerSubTab = 'console'"
        >
          <MdiIcon :path="mdiConsole" size="14" />
          <span>Console</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.dockerSubTab === 'configuration' || nasState.dockerSubTab === 'compose' }"
          @click="nasState.dockerSubTab = 'configuration'"
        >
          <MdiIcon :path="mdiCogOutline" size="14" />
          <span>Configuration</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.dockerSubTab === 'containers' }"
          @click="nasState.dockerSubTab = 'containers'"
        >
          <MdiIcon :path="mdiLayers" size="14" />
          <span>Containers</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.dockerSubTab === 'logs' }"
          @click="nasState.dockerSubTab = 'logs'"
        >
          <MdiIcon :path="mdiFileDocumentOutline" size="14" />
          <span>Logs</span>
        </button>
      </template>

      <!-- Pool Subtabs -->
      <template v-else-if="currentPool">
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.poolSubTab === 'summary' }"
          @click="nasState.poolSubTab = 'summary'"
        >
          <MdiIcon :path="mdiInformationOutline" size="14" />
          <span>Summary</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.poolSubTab === 'datasets' }"
          @click="nasState.poolSubTab = 'datasets'"
        >
          <MdiIcon :path="mdiFolderOutline" size="14" />
          <span>Datasets</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.poolSubTab === 'volumes' }"
          @click="nasState.poolSubTab = 'volumes'"
        >
          <MdiIcon :path="mdiHarddisk" size="14" />
          <span>Volumes</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.poolSubTab === 'topology' }"
          @click="nasState.poolSubTab = 'topology'"
        >
          <MdiIcon :path="mdiLayers" size="14" />
          <span>Topology</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.poolSubTab === 'config' }"
          @click="nasState.poolSubTab = 'config'"
        >
          <MdiIcon :path="mdiCogOutline" size="14" />
          <span>Configuration</span>
        </button>
      </template>

      <!-- Storage Collection Subtabs -->
      <template
        v-else-if="
          nasState.activeTab === 'storage-pools' ||
          nasState.activeTab === 'storage-disks' ||
          nasState.activeTab === 'storage-datasets' ||
          nasState.activeTab === 'storage-volumes' ||
          nasState.activeTab === 'zfs'
        "
      >
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.activeTab === 'storage-pools' || nasState.activeTab === 'zfs' }"
          @click="handleNav('storage-pools')"
        >
          <span>Pools ({{ nasState.pools.length }})</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.activeTab === 'storage-disks' }"
          @click="handleNav('storage-disks')"
        >
          <span>Disks ({{ nasState.disks.length }})</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.activeTab === 'storage-datasets' }"
          @click="handleNav('storage-datasets')"
        >
          <span>Datasets</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.activeTab === 'storage-volumes' }"
          @click="handleNav('storage-volumes')"
        >
          <span>Volumes</span>
        </button>
      </template>

      <!-- Shares Collection Subtabs -->
      <template
        v-else-if="
          nasState.activeTab === 'shares' ||
          nasState.activeTab === 'shares-cifs' ||
          nasState.activeTab === 'shares-webdav'
        "
      >
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.activeTab === 'shares' || nasState.activeTab === 'shares-cifs' }"
          @click="handleNav('shares-cifs')"
        >
          <span>SMB / CIFS ({{ nasState.cifsShares.length }})</span>
        </button>
        <button
          class="ribbon-tab-btn"
          :class="{ active: nasState.activeTab === 'shares-webdav' }"
          @click="handleNav('shares-webdav')"
        >
          <span>WebDAV ({{ nasState.webdavShares.length }})</span>
        </button>
      </template>
    </div>

    <!-- Ribbon Right: Unified Contextual Action Buttons (Icon-Only with Tooltips) -->
    <div class="ribbon-actions">
      <!-- VM Action Buttons -->
      <template v-if="currentVm">
        <button
          v-if="currentVm.state === 'PAUSED'"
          class="btn btn-sm btn-icon btn-success"
          :disabled="isBusy"
          title="Resume Virtual Machine"
          aria-label="Resume"
          @click="resumeVm"
        >
          <MdiIcon :path="mdiPlay" size="15" />
        </button>
        <button
          v-else-if="currentVm.state !== 'RUNNING'"
          class="btn btn-sm btn-icon btn-success"
          :disabled="isBusy"
          title="Start Virtual Machine"
          aria-label="Start"
          @click="startVm"
        >
          <MdiIcon :path="mdiPlay" size="15" />
        </button>

        <button
          v-if="currentVm.state === 'RUNNING'"
          class="btn btn-sm btn-icon btn-secondary"
          :disabled="isBusy"
          title="Pause Virtual Machine"
          aria-label="Pause"
          @click="pauseVm"
        >
          <MdiIcon :path="mdiPause" size="15" />
        </button>

        <button
          v-if="currentVm.state === 'RUNNING'"
          class="btn btn-sm btn-icon btn-secondary"
          :disabled="isBusy"
          title="Shutdown (Send ACPI Signal)"
          aria-label="Shutdown"
          @click="shutdownVm"
        >
          <MdiIcon :path="mdiPower" size="15" />
        </button>

        <button
          v-if="currentVm.state === 'RUNNING'"
          class="btn btn-sm btn-icon btn-secondary"
          :disabled="isBusy"
          title="Stop (Force Power Off)"
          aria-label="Stop"
          @click="forceStopVm"
        >
          <MdiIcon :path="mdiStop" size="15" />
        </button>

        <button
          v-if="currentVm.state === 'RUNNING'"
          class="btn btn-sm btn-icon btn-secondary"
          :disabled="isBusy"
          title="Reboot Virtual Machine"
          aria-label="Reboot"
          @click="rebootVm"
        >
          <MdiIcon :path="mdiRestart" size="15" />
        </button>

        <button
          class="btn btn-sm btn-icon btn-secondary"
          :disabled="isBusy"
          title="Clone Virtual Machine"
          aria-label="Clone"
          @click="openCloneVm"
        >
          <MdiIcon :path="mdiContentCopy" size="15" />
        </button>

        <button
          class="btn btn-sm btn-icon btn-secondary"
          title="Console Pop-out (Open Live noVNC in New Window)"
          aria-label="Console Pop-out"
          @click="popoutVmConsole"
        >
          <MdiIcon :path="mdiOpenInNew" size="15" />
        </button>

        <button
          class="btn btn-sm btn-icon btn-danger"
          :disabled="isBusy"
          title="Delete Virtual Machine"
          aria-label="Delete"
          @click="deleteVm"
        >
          <MdiIcon :path="mdiTrashCanOutline" size="15" />
        </button>
      </template>

      <!-- LXC Action Buttons -->
      <template v-else-if="currentLxc">
        <button
          v-if="currentLxc.state !== 'RUNNING' && currentLxc.state !== 'FROZEN'"
          class="btn btn-sm btn-icon btn-success"
          :disabled="isBusy"
          title="Start LXC Container"
          aria-label="Start"
          @click="startLxc"
        >
          <MdiIcon :path="mdiPlay" size="15" />
        </button>

        <button
          v-if="currentLxc.state === 'RUNNING' || currentLxc.state === 'FROZEN'"
          class="btn btn-sm btn-icon btn-secondary"
          :disabled="isBusy"
          :title="currentLxc.state === 'FROZEN' ? 'Unfreeze Container' : 'Freeze Container'"
          :aria-label="currentLxc.state === 'FROZEN' ? 'Unfreeze' : 'Freeze'"
          @click="toggleFreezeLxc"
        >
          <MdiIcon :path="currentLxc.state === 'FROZEN' ? mdiPlay : mdiPause" size="15" />
        </button>

        <button
          v-if="currentLxc.state === 'RUNNING' || currentLxc.state === 'FROZEN'"
          class="btn btn-sm btn-icon btn-secondary"
          :disabled="isBusy"
          title="Stop LXC Container"
          aria-label="Stop"
          @click="stopLxc"
        >
          <MdiIcon :path="mdiStop" size="15" />
        </button>

        <button
          v-if="currentLxc.state === 'RUNNING'"
          class="btn btn-sm btn-icon btn-secondary"
          :disabled="isBusy"
          title="Reboot LXC Container"
          aria-label="Reboot"
          @click="restartLxc"
        >
          <MdiIcon :path="mdiRestart" size="15" />
        </button>

        <button
          v-if="currentLxc.state === 'RUNNING'"
          class="btn btn-sm btn-icon btn-secondary"
          title="Console Pop-out (Open Detached Shell Terminal Window)"
          aria-label="Console Pop-out"
          @click="popoutLxcTerminal"
        >
          <MdiIcon :path="mdiOpenInNew" size="15" />
        </button>

        <button
          class="btn btn-sm btn-icon btn-danger"
          :disabled="isBusy"
          title="Delete LXC Container"
          aria-label="Delete"
          @click="deleteLxc"
        >
          <MdiIcon :path="mdiTrashCanOutline" size="15" />
        </button>
      </template>

      <!-- Storage Pool Action Buttons -->
      <template v-else-if="currentPool">
        <button
          class="btn btn-sm btn-icon btn-secondary"
          :disabled="isBusy"
          title="Scrub Pool (Start Data Integrity Scrub)"
          aria-label="Scrub"
          @click="scrubPool"
        >
          <MdiIcon :path="mdiDatabaseSync" size="15" />
        </button>

        <button
          v-if="!isMainPool"
          class="btn btn-sm btn-icon btn-secondary"
          :disabled="isBusy"
          title="Set as Main Pool"
          aria-label="Set Main Pool"
          @click="setMainPool"
        >
          <MdiIcon :path="mdiDatabaseCheck" size="15" />
        </button>

        <button
          class="btn btn-sm btn-icon btn-secondary"
          :disabled="isBusy"
          title="Export Pool (Safely Unmount and Export)"
          aria-label="Export"
          @click="exportPool"
        >
          <MdiIcon :path="mdiExport" size="15" />
        </button>

        <button
          class="btn btn-sm btn-icon btn-danger"
          :disabled="isBusy"
          title="Destroy Pool Permanently"
          aria-label="Destroy"
          @click="destroyPool"
        >
          <MdiIcon :path="mdiTrashCanOutline" size="15" />
        </button>
      </template>

      <!-- Docker App Action Buttons -->
      <template v-else-if="currentDocker">
        <button
          v-if="currentDocker.state !== 'RUNNING'"
          class="btn btn-sm btn-icon btn-success"
          :disabled="isBusy"
          title="Start Application Stack"
          aria-label="Start"
          @click="startDocker"
        >
          <MdiIcon :path="mdiPlay" size="15" />
        </button>

        <button
          v-if="currentDocker.state === 'RUNNING' || currentDocker.state === 'PARTIALLY_RUNNING'"
          class="btn btn-sm btn-icon btn-secondary"
          :disabled="isBusy"
          title="Stop Application Stack"
          aria-label="Stop"
          @click="stopDocker"
        >
          <MdiIcon :path="mdiStop" size="15" />
        </button>

        <button
          v-if="currentDocker.state === 'RUNNING' || currentDocker.state === 'PARTIALLY_RUNNING'"
          class="btn btn-sm btn-icon btn-secondary"
          :disabled="isBusy"
          title="Restart Application Stack"
          aria-label="Restart"
          @click="restartDocker"
        >
          <MdiIcon :path="mdiRestart" size="15" />
        </button>

        <a
          v-if="currentDocker.webUiUrl"
          :href="currentDocker.webUiUrl"
          target="_blank"
          rel="noopener noreferrer"
          class="btn btn-sm btn-icon btn-primary"
          title="Open Web UI Portal"
          aria-label="Web UI"
        >
          <MdiIcon :path="mdiOpenInNew" size="15" />
        </a>

        <button
          class="btn btn-sm btn-icon btn-secondary"
          :disabled="isBusy"
          title="Upgrade Application (Pull Latest Images & Redeploy)"
          aria-label="Upgrade"
          @click="upgradeDocker"
        >
          <MdiIcon :path="mdiUpdate" size="15" />
        </button>

        <button
          class="btn btn-sm btn-icon btn-danger"
          :disabled="isBusy"
          title="Delete Application Stack"
          aria-label="Delete"
          @click="deleteDocker"
        >
          <MdiIcon :path="mdiTrashCanOutline" size="15" />
        </button>
      </template>
    </div>

    <!-- Clone VM Modal -->
    <div v-if="showCloneVmModal && currentVm" class="modal-overlay" @click.self="showCloneVmModal = false">
      <div class="modal-container" style="max-width: 460px">
        <div class="modal-header">
          <h3 class="modal-title">Clone VM: {{ currentVm.vmName }}</h3>
          <button class="btn btn-xs btn-ghost" @click="showCloneVmModal = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
        <div class="modal-body" style="display: flex; flex-direction: column; gap: 14px; padding: 16px 20px">
          <div style="display: flex; flex-direction: column; gap: 6px">
            <label style="font-size: 12px; font-weight: 600; color: var(--text-secondary)">Target VM Name</label>
            <input
              v-model="cloneForm.targetVmName"
              type="text"
              class="input"
              placeholder="e.g. debian-server-clone"
              @input="cloneForm.targetVmId = toZfsSafeSlug(cloneForm.targetVmName)"
            />
          </div>
          <label style="display: flex; align-items: center; gap: 8px; font-size: 13px; cursor: pointer">
            <input v-model="cloneForm.fullClone" type="checkbox" />
            <span>Full Clone (independent copy of virtual disks)</span>
          </label>
        </div>
        <div class="modal-footer" style="padding: 12px 20px; display: flex; justify-content: flex-end; gap: 8px">
          <button class="btn btn-sm btn-secondary" @click="showCloneVmModal = false">Cancel</button>
          <button class="btn btn-sm btn-primary" :disabled="!cloneForm.targetVmName.trim() || isBusy" @click="submitCloneVm">
            Clone VM
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.action-ribbon {
  min-height: 44px;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  flex-shrink: 0;
  gap: 14px;
  flex-wrap: wrap;
}

.ribbon-identity {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.ribbon-back-btn {
  padding: 3px 5px;
  color: var(--text-secondary);
}

.ribbon-name {
  font-weight: 700;
  font-size: 14.5px;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ribbon-metric {
  color: var(--text-secondary);
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  padding: 1px 7px;
  font-size: 11.5px;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.ribbon-tabs {
  display: flex;
  align-items: center;
  gap: 4px;
  overflow-x: auto;
  flex: 1;
}

.ribbon-tab-btn {
  background: transparent;
  border: 1px solid transparent;
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 500;
  padding: 5px 11px;
  border-radius: 5px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  transition: all 0.12s ease;
  white-space: nowrap;
}

.ribbon-tab-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.ribbon-tab-btn.active {
  background: var(--accent-bg-subtle);
  color: var(--accent-primary);
  border-color: var(--accent-primary);
  font-weight: 600;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

.ribbon-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
  margin-left: auto;
  padding: 2px 4px;
}

.ribbon-actions .btn {
  height: 28px;
  padding: 0 10px;
  font-size: 12px;
  font-weight: 600;
  border-radius: 5px;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.12);
  transition: all 0.12s ease;
}

.ribbon-actions .btn:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.2);
}

.ribbon-actions .btn-success {
  background: var(--success);
  color: #fff;
  border-color: var(--success);
}

.ribbon-actions .btn-danger {
  background: rgba(239, 68, 68, 0.15);
  color: var(--danger);
  border-color: rgba(239, 68, 68, 0.35);
}

.ribbon-actions .btn-danger:hover:not(:disabled) {
  background: var(--danger);
  color: #fff;
}

.ribbon-actions .btn-secondary {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  color: var(--text-primary);
}

.ribbon-actions .btn-secondary:hover:not(:disabled) {
  background: var(--bg-hover);
  border-color: var(--border-hover, var(--text-muted));
}

.font-xs {
  font-size: 11.5px;
}
</style>
