<script setup lang="ts">
import { computed, ref } from 'vue';
import {
  mdiAlertCircle,
  mdiCamera,
  mdiCheckCircle,
  mdiConsole,
  mdiCubeOutline,
  mdiDatabase,
  mdiDatabaseCheck,
  mdiDatabaseSync,
  mdiLayers,
  mdiMonitor,
  mdiOpenInNew,
  mdiPlay,
  mdiPower,
  mdiRefresh,
  mdiRestart,
  mdiServer,
  mdiSnowflake,
  mdiStop,
  mdiTrashCanOutline,
  mdiUpdate,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import { services } from '../../container';
import {
  nasState,
  notify,
  refreshAllData,
  runTrackedTask,
  selectResource,
} from '../store';
import { confirmDelete } from '../modal';
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

async function popoutVmConsole() {
  if (!currentVm.value) return;
  const clean = currentVm.value.name.replace(/^virtualMachines\//, '');
  try {
    const s = await services.kvm.openConsole(clean);
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
  });
  if (!confirmed.confirmed) return;
  isBusy.value = true;
  try {
    await runTrackedTask(`App "${appDisplayName}": Delete`, async () => {
      await services.apps.deleteApp(app.name, false);
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
      await services.system.updateSystemConfig({
        mainPool: name,
      });
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
  const confirmed = await confirmDelete({
    kind: 'Destroy ZFS Pool',
    name,
    details: 'CRITICAL DANGER: ALL DATA, DATASETS, AND VOLUMES ON THIS POOL WILL BE PERMANENTLY DESTROYED.',
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
    <!-- Ribbon Left: Resource Identity & Live Badges -->
    <div class="ribbon-identity">
      <!-- VM Identity -->
      <template v-if="currentVm">
        <MdiIcon :path="mdiMonitor" size="18" style="color: var(--accent-primary)" />
        <span class="ribbon-name">{{ currentVm.vmName }}</span>
        <span class="ribbon-type-tag mono">VM</span>
        <span
          class="badge mono font-xs"
          :class="currentVm.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'"
        >
          {{ currentVm.state }}
        </span>
        <span
          v-if="currentVm.state === 'RUNNING' && currentVm.guestCpuPercent !== undefined"
          class="ribbon-metric mono font-xs"
        >
          CPU: {{ currentVm.guestCpuPercent.toFixed(1) }}%
        </span>
        <span
          v-if="currentVm.state === 'RUNNING' && currentVm.guestMemoryUsedMib"
          class="ribbon-metric mono font-xs"
        >
          RAM: {{ currentVm.guestMemoryUsedMib >= 1024 ? (currentVm.guestMemoryUsedMib / 1024).toFixed(2) + ' GiB' : currentVm.guestMemoryUsedMib + ' MiB' }}
        </span>
      </template>

      <!-- LXC Identity -->
      <template v-else-if="currentLxc">
        <MdiIcon :path="mdiCubeOutline" size="18" style="color: var(--accent-secondary)" />
        <span class="ribbon-name">{{ currentLxc.containerName || currentLxc.name.replace('lxcContainers/', '') }}</span>
        <span class="ribbon-type-tag mono">LXC</span>
        <span
          class="badge mono font-xs"
          :class="currentLxc.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'"
        >
          {{ currentLxc.state }}
        </span>
        <span
          v-if="currentLxc.state === 'RUNNING' && currentLxc.cpuPercent !== undefined"
          class="ribbon-metric mono font-xs"
        >
          CPU: {{ currentLxc.cpuPercent.toFixed(1) }}%
        </span>
        <span
          v-if="currentLxc.state === 'RUNNING' && currentLxc.memoryUsedMib"
          class="ribbon-metric mono font-xs"
        >
          RAM: {{ currentLxc.memoryUsedMib >= 1024 ? (currentLxc.memoryUsedMib / 1024).toFixed(2) + ' GiB' : currentLxc.memoryUsedMib + ' MiB' }}
        </span>
      </template>

      <!-- Pool Identity -->
      <template v-else-if="currentPool">
        <MdiIcon :path="mdiDatabase" size="18" style="color: var(--warning-text)" />
        <span class="ribbon-name">{{ currentPool.poolName || currentPool.name }}</span>
        <span class="ribbon-type-tag mono">ZFS POOL</span>
        <span
          class="badge mono font-xs"
          :class="currentPool.state === 'ONLINE' ? 'badge-success' : 'badge-danger'"
        >
          {{ currentPool.state || 'ONLINE' }}
        </span>
      </template>

      <!-- Docker Identity -->
      <template v-else-if="currentDocker">
        <MdiIcon :path="mdiLayers" size="18" style="color: var(--info-text)" />
        <span class="ribbon-name">{{ currentDocker.title || currentDocker.appId || currentDocker.name.replace(/^dockerApps\//, '') }}</span>
        <span class="ribbon-type-tag mono">APP</span>
        <span
          class="badge mono font-xs"
          :class="
            currentDocker.state === 'RUNNING'
              ? 'badge-success'
              : currentDocker.state === 'PARTIALLY_RUNNING'
              ? 'badge-warning'
              : 'badge-neutral'
          "
        >
          {{ currentDocker.state || 'STOPPED' }}
        </span>
        <span
          v-if="currentDocker.state === 'RUNNING' && currentDocker.totalCpuPercent !== undefined"
          class="ribbon-metric mono font-xs"
        >
          CPU: {{ currentDocker.totalCpuPercent.toFixed(1) }}%
        </span>
        <span
          v-if="currentDocker.state === 'RUNNING' && currentDocker.totalMemoryMib"
          class="ribbon-metric mono font-xs"
        >
          RAM: {{ currentDocker.totalMemoryMib >= 1024 ? (currentDocker.totalMemoryMib / 1024).toFixed(2) + ' GiB' : currentDocker.totalMemoryMib + ' MiB' }}
        </span>
      </template>

      <!-- Node Default Identity -->
      <template v-else>
        <MdiIcon :path="mdiServer" size="18" style="color: var(--accent-primary)" />
        <span class="ribbon-name">{{ nasState.systemTelemetry?.host?.hostname || 'nas01' }}</span>
        <span class="ribbon-type-tag mono">NODE</span>
        <span class="badge badge-success mono font-xs">ONLINE</span>
        <span v-if="nasState.systemTelemetry?.host?.uptimeSeconds" class="ribbon-metric mono font-xs">
          Uptime: {{ Math.floor(nasState.systemTelemetry.host.uptimeSeconds / 86400) }}d {{ Math.floor((nasState.systemTelemetry.host.uptimeSeconds % 86400) / 3600) }}h
        </span>
      </template>
    </div>

    <!-- Ribbon Right: Contextual Action Buttons -->
    <div class="ribbon-actions">
      <!-- VM Action Buttons -->
      <template v-if="currentVm">
        <button
          v-if="currentVm.state !== 'RUNNING'"
          class="btn btn-sm btn-success"
          :disabled="isBusy"
          title="Start Virtual Machine"
          @click="startVm"
        >
          <MdiIcon :path="mdiPlay" size="14" />
          <span>Start</span>
        </button>

        <button
          v-if="currentVm.state === 'RUNNING'"
          class="btn btn-sm btn-secondary"
          :disabled="isBusy"
          title="Send ACPI Shutdown Signal"
          @click="shutdownVm"
        >
          <MdiIcon :path="mdiPower" size="14" />
          <span>Shutdown</span>
        </button>

        <button
          v-if="currentVm.state === 'RUNNING'"
          class="btn btn-sm btn-danger"
          :disabled="isBusy"
          title="Force Power Off (Hard Stop)"
          @click="forceStopVm"
        >
          <MdiIcon :path="mdiStop" size="14" />
          <span>Stop</span>
        </button>

        <button
          v-if="currentVm.state === 'RUNNING'"
          class="btn btn-sm btn-secondary"
          :disabled="isBusy"
          title="Reboot VM"
          @click="rebootVm"
        >
          <MdiIcon :path="mdiRestart" size="14" />
          <span>Reboot</span>
        </button>

        <!-- Pop-out Console -->
        <button
          class="btn btn-sm btn-primary"
          title="Open Live noVNC Console in New Dedicated Window"
          @click="popoutVmConsole"
        >
          <MdiIcon :path="mdiOpenInNew" size="14" />
          <span>Console Pop-out</span>
        </button>

        <button
          class="btn btn-sm btn-danger"
          :disabled="isBusy"
          title="Delete Virtual Machine"
          @click="deleteVm"
        >
          <MdiIcon :path="mdiTrashCanOutline" size="14" />
          <span>Delete</span>
        </button>
      </template>

      <!-- LXC Action Buttons -->
      <template v-else-if="currentLxc">
        <button
          v-if="currentLxc.state !== 'RUNNING'"
          class="btn btn-sm btn-success"
          :disabled="isBusy"
          title="Start LXC Container"
          @click="startLxc"
        >
          <MdiIcon :path="mdiPlay" size="14" />
          <span>Start</span>
        </button>

        <button
          v-if="currentLxc.state === 'RUNNING'"
          class="btn btn-sm btn-secondary"
          :disabled="isBusy"
          title="Stop LXC Container"
          @click="stopLxc"
        >
          <MdiIcon :path="mdiStop" size="14" />
          <span>Stop</span>
        </button>

        <button
          v-if="currentLxc.state === 'RUNNING'"
          class="btn btn-sm btn-secondary"
          :disabled="isBusy"
          title="Restart Container"
          @click="restartLxc"
        >
          <MdiIcon :path="mdiRestart" size="14" />
          <span>Reboot</span>
        </button>

        <button
          v-if="currentLxc.state === 'RUNNING' || currentLxc.state === 'FROZEN'"
          class="btn btn-sm btn-secondary"
          :disabled="isBusy"
          :title="currentLxc.state === 'FROZEN' ? 'Unfreeze Container' : 'Freeze Container'"
          @click="toggleFreezeLxc"
        >
          <MdiIcon :path="mdiSnowflake" size="14" />
          <span>{{ currentLxc.state === 'FROZEN' ? 'Unfreeze' : 'Freeze' }}</span>
        </button>

        <button
          v-if="currentLxc.state === 'RUNNING'"
          class="btn btn-sm btn-primary"
          title="Open Interactive Shell Terminal"
          @click="emit('open-lxc-terminal', currentLxc)"
        >
          <MdiIcon :path="mdiConsole" size="14" />
          <span>Terminal</span>
        </button>

        <button
          class="btn btn-sm btn-danger"
          :disabled="isBusy"
          title="Delete LXC Container"
          @click="deleteLxc"
        >
          <MdiIcon :path="mdiTrashCanOutline" size="14" />
          <span>Delete</span>
        </button>
      </template>

      <!-- Storage Pool Action Buttons -->
      <template v-else-if="currentPool">
        <button
          class="btn btn-sm btn-secondary"
          :disabled="isBusy"
          title="Start ZFS Pool Data Integrity Scrub"
          @click="scrubPool"
        >
          <MdiIcon :path="mdiDatabaseSync" size="14" />
          <span>Scrub</span>
        </button>

        <button
          v-if="!isMainPool"
          class="btn btn-sm btn-secondary"
          :disabled="isBusy"
          title="Designate this pool as the system Main Pool"
          @click="setMainPool"
        >
          <MdiIcon :path="mdiDatabaseCheck" size="14" />
          <span>Set Main Pool</span>
        </button>

        <button
          class="btn btn-sm btn-secondary"
          :disabled="isBusy"
          title="Safely unmount and export pool"
          @click="exportPool"
        >
          <span>Export</span>
        </button>

        <button
          class="btn btn-sm btn-danger"
          :disabled="isBusy"
          title="Permanently destroy pool"
          @click="destroyPool"
        >
          <MdiIcon :path="mdiTrashCanOutline" size="14" />
          <span>Destroy</span>
        </button>
      </template>

      <!-- Docker App Action Buttons -->
      <template v-else-if="currentDocker">
        <button
          v-if="currentDocker.state !== 'RUNNING'"
          class="btn btn-sm btn-success"
          :disabled="isBusy"
          title="Start Application Stack"
          @click="startDocker"
        >
          <MdiIcon :path="mdiPlay" size="14" />
          <span>Start</span>
        </button>

        <button
          v-if="currentDocker.state === 'RUNNING' || currentDocker.state === 'PARTIALLY_RUNNING'"
          class="btn btn-sm btn-secondary"
          :disabled="isBusy"
          title="Stop Application Stack"
          @click="stopDocker"
        >
          <MdiIcon :path="mdiStop" size="14" />
          <span>Stop</span>
        </button>

        <button
          v-if="currentDocker.state === 'RUNNING' || currentDocker.state === 'PARTIALLY_RUNNING'"
          class="btn btn-sm btn-secondary"
          :disabled="isBusy"
          title="Restart Application Stack"
          @click="restartDocker"
        >
          <MdiIcon :path="mdiRestart" size="14" />
          <span>Restart</span>
        </button>

        <a
          v-if="currentDocker.webUiUrl"
          :href="currentDocker.webUiUrl"
          target="_blank"
          rel="noopener noreferrer"
          class="btn btn-sm btn-primary"
          title="Open Web UI Portal"
        >
          <MdiIcon :path="mdiOpenInNew" size="14" />
          <span>Web UI</span>
        </a>

        <button
          class="btn btn-sm btn-secondary"
          :disabled="isBusy"
          title="Pull latest images and update app"
          @click="upgradeDocker"
        >
          <MdiIcon :path="mdiUpdate" size="14" />
          <span>Upgrade</span>
        </button>

        <button
          class="btn btn-sm btn-danger"
          :disabled="isBusy"
          title="Delete Application Stack"
          @click="deleteDocker"
        >
          <MdiIcon :path="mdiTrashCanOutline" size="14" />
          <span>Delete</span>
        </button>
      </template>

      <!-- Node / Default Action Buttons -->
      <template v-else>
        <button
          class="btn btn-sm btn-secondary"
          title="Open Host Shell Terminal"
          @click="emit('open-host-terminal')"
        >
          <MdiIcon :path="mdiConsole" size="14" />
          <span class="hide-mobile">Shell</span>
        </button>

        <button
          class="btn btn-sm btn-secondary"
          title="Open Detached Host Terminal Window"
          @click="popoutHostTerminal"
        >
          <MdiIcon :path="mdiOpenInNew" size="14" />
          <span class="hide-mobile">Shell Pop-out</span>
        </button>

        <button
          class="btn btn-sm btn-secondary"
          title="Inspect Host Specs & Hardware"
          @click="emit('open-system-details')"
        >
          <span class="hide-mobile">Host Specs</span>
        </button>

        <button
          class="btn btn-sm btn-secondary"
          title="Inspect Host System Prerequisites"
          @click="emit('open-prerequisites')"
        >
          <span class="hide-mobile">Prerequisites</span>
        </button>

        <button
          class="btn btn-sm btn-ghost"
          :disabled="nasState.loading"
          title="Refresh All Telemetry"
          @click="refreshAllData"
        >
          <MdiIcon :path="mdiRefresh" size="14" :class="{ 'spin-icon': nasState.loading }" />
        </button>
      </template>
    </div>
  </div>
</template>

<style scoped>
.action-ribbon {
  height: 42px;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  flex-shrink: 0;
  gap: 12px;
}

.ribbon-identity {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  overflow: hidden;
}

.ribbon-name {
  font-weight: 700;
  font-size: 14px;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ribbon-type-tag {
  font-size: 10px;
  background: var(--bg-elevated);
  color: var(--text-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  padding: 1px 5px;
}

.ribbon-metric {
  color: var(--text-secondary);
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  padding: 1px 6px;
}

.ribbon-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.font-xs {
  font-size: 10.5px;
}
</style>
