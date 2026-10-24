<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { services } from '../../container';
import type { VirtualMachine, VirtualMachineConsoleSession } from '../../domain/models';
import { nasState, notify } from '../store';
import NoVncModal from '../components/NoVncModal.vue';

const showCreateVm = ref(false);
const activeConsoleSession = ref<VirtualMachineConsoleSession | null>(null);
const editingVm = ref<VirtualMachine | null>(null);

const vmForm = reactive({
  vmId: '',
  vcpus: 4,
  memoryMib: 4096,
  diskPath: '/dev/zvol/tank/vm-root',
  diskSizeGib: 40,
  isoImage: '/tank/isos/debian-12.7.0-amd64-netinst.iso',
  networkBridge: 'virbr0',
  osVariant: 'debian12',
  autostart: true,
  description: '',
});

const editVmForm = reactive({
  vcpus: 2,
  memoryMib: 2048,
  autostart: false,
  description: '',
});

const availableZvols = computed(() => {
  const list: string[] = [];
  for (const vols of Object.values(nasState.volumesByPool)) {
    for (const v of vols) {
      if (v.devicePath) list.push(v.devicePath);
    }
  }
  return list;
});

async function refreshVms() {
  nasState.vms = await services.kvm.listVirtualMachines();
}

async function handleCreateVm() {
  try {
    const created = await services.kvm.createVirtualMachine(vmForm.vmId, {
      vcpus: vmForm.vcpus,
      memoryMib: vmForm.memoryMib,
      diskPath: vmForm.diskPath,
      diskSizeGib: vmForm.diskSizeGib,
      isoImage: vmForm.isoImage,
      networkBridge: vmForm.networkBridge,
      osVariant: vmForm.osVariant,
      autostart: vmForm.autostart,
      description: vmForm.description,
    });
    notify('success', `Provisioned KVM Virtual Machine ${created.vmName}`);
    vmForm.vmId = '';
    showCreateVm.value = false;
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create VM');
  }
}

function startEditVm(vm: VirtualMachine) {
  editingVm.value = vm;
  editVmForm.vcpus = vm.vcpus;
  editVmForm.memoryMib = vm.memoryMib;
  editVmForm.autostart = vm.autostart;
  editVmForm.description = vm.description;
}

async function handleSaveVmEdit() {
  if (!editingVm.value) return;
  try {
    await services.kvm.updateVirtualMachine(
      editingVm.value.name,
      {
        vcpus: editVmForm.vcpus,
        memoryMib: editVmForm.memoryMib,
        autostart: editVmForm.autostart,
        description: editVmForm.description,
      },
      ['vcpus', 'memory_mib', 'autostart', 'description']
    );
    notify('success', `Updated VM ${editingVm.value.vmName}`);
    editingVm.value = null;
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update VM');
  }
}

async function handleVmPower(vmName: string, action: 'start' | 'stop' | 'reboot') {
  try {
    if (action === 'start') {
      await services.kvm.startVirtualMachine(vmName);
    } else if (action === 'stop') {
      await services.kvm.stopVirtualMachine(vmName, false);
    } else {
      await services.kvm.rebootVirtualMachine(vmName);
    }
    notify('success', `Executed :${action} on ${vmName}`);
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || `Failed to ${action} VM`);
  }
}

async function handleOpenConsole(vm: VirtualMachine) {
  try {
    const session = await services.kvm.openConsole(vm.name);
    activeConsoleSession.value = session;
  } catch (err: any) {
    notify('error', err?.message || 'Failed to open noVNC console');
  }
}

async function handleDeleteVm(vmName: string) {
  try {
    await services.kvm.deleteVirtualMachine(vmName);
    notify('info', `Deleted VM ${vmName}`);
    await refreshVms();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete VM');
  }
}
</script>

<template>
  <div style="display: flex; flex-direction: column; gap: 20px">
    <div class="card">
      <div class="card-header">
        <div>
          <h2 class="card-title" style="font-size: 17px">
            KVM Virtual Machines (`virtualMachines/{virtual_machine}`) — libvirt + noVNC
          </h2>
          <p class="card-subtitle">
            Hardware-accelerated QEMU/KVM domains backed by ZFS ZVols or QCOW2 images, with embedded
            WebSocket RFB 003.008 noVNC graphical consoles (`:openConsole`).
          </p>
        </div>

        <button class="btn btn-primary" @click="showCreateVm = !showCreateVm">
          {{ showCreateVm ? 'Cancel' : '+ Provision Virtual Machine' }}
        </button>
      </div>

      <!-- Create VM Form -->
      <form
        v-if="showCreateVm"
        @submit.prevent="handleCreateVm"
        style="
          background: var(--bg-base);
          border: 1px solid var(--border-strong);
          border-radius: 8px;
          padding: 16px;
          margin-bottom: 18px;
        "
      >
        <div class="form-grid">
          <div class="form-group">
            <label>VM Domain ID (`virtual_machine_id`)</label>
            <input v-model="vmForm.vmId" class="input mono" placeholder="debian-router" required />
          </div>
          <div class="form-group">
            <label>vCPUs</label>
            <input v-model.number="vmForm.vcpus" type="number" min="1" max="64" class="input mono" required />
          </div>
          <div class="form-group">
            <label>Memory (MiB)</label>
            <input v-model.number="vmForm.memoryMib" type="number" min="256" step="256" class="input mono" required />
          </div>
          <div class="form-group">
            <label>Disk Size (GiB)</label>
            <input v-model.number="vmForm.diskSizeGib" type="number" min="1" class="input mono" required />
          </div>
          <div class="form-group">
            <label>Backing ZVol or QCOW2 Path</label>
            <input
              v-model="vmForm.diskPath"
              list="zvol-list"
              class="input mono"
              placeholder="/dev/zvol/tank/vm-disk"
              required
            />
            <datalist id="zvol-list">
              <option v-for="zpath in availableZvols" :key="zpath" :value="zpath" />
            </datalist>
          </div>
          <div class="form-group">
            <label>Boot ISO Image Path</label>
            <input v-model="vmForm.isoImage" class="input mono" placeholder="/tank/isos/debian.iso" />
          </div>
          <div class="form-group">
            <label>Network Bridge</label>
            <input v-model="vmForm.networkBridge" class="input mono" placeholder="virbr0" />
          </div>
          <div class="form-group">
            <label>OS Profile (`os_variant`)</label>
            <select v-model="vmForm.osVariant" class="select">
              <option value="debian12">Debian 12 (Bookworm)</option>
              <option value="ubuntu24.04">Ubuntu 24.04 LTS</option>
              <option value="almalinux9">AlmaLinux / RHEL 9</option>
              <option value="freebsd14.0">FreeBSD 14 / OPNsense</option>
              <option value="win2k22">Windows Server 2022</option>
            </select>
          </div>
        </div>
        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 12px">
          <label style="display: flex; align-items: center; gap: 6px; font-size: 12.5px">
            <input v-model="vmForm.autostart" type="checkbox" /> Autostart domain with libvirtd
          </label>
          <button type="submit" class="btn btn-primary">Deploy KVM Domain</button>
        </div>
      </form>

      <!-- Edit VM Inline Form -->
      <div
        v-if="editingVm"
        style="
          background: rgba(56, 189, 248, 0.08);
          border: 1px solid var(--accent-primary);
          border-radius: 8px;
          padding: 14px;
          margin-bottom: 16px;
        "
      >
        <div style="font-weight: 600; margin-bottom: 10px">
          PATCH {{ editingVm.name }} (`updateMask=vcpus,memory_mib,autostart,description`)
        </div>
        <div class="form-grid">
          <div class="form-group">
            <label>vCPUs</label>
            <input v-model.number="editVmForm.vcpus" type="number" min="1" class="input mono" />
          </div>
          <div class="form-group">
            <label>Memory (MiB)</label>
            <input v-model.number="editVmForm.memoryMib" type="number" min="256" step="256" class="input mono" />
          </div>
          <div class="form-group">
            <label>Description</label>
            <input v-model="editVmForm.description" class="input" />
          </div>
        </div>
        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 10px">
          <label style="font-size: 12px; display: flex; align-items: center; gap: 6px">
            <input v-model="editVmForm.autostart" type="checkbox" /> Autostart on Host Boot
          </label>
          <div style="display: flex; gap: 8px">
            <button class="btn btn-sm" @click="editingVm = null">Cancel</button>
            <button class="btn btn-primary btn-sm" @click="handleSaveVmEdit">Save PATCH</button>
          </div>
        </div>
      </div>

      <!-- VM Table -->
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>VM Domain / UUID</th>
              <th>State</th>
              <th>Compute (vCPU / RAM)</th>
              <th>Storage Disk & ISO</th>
              <th>Network & VNC</th>
              <th>Actions & noVNC Console</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="vm in nasState.vms" :key="vm.name">
              <td>
                <div style="font-weight: 600; font-size: 14px">{{ vm.vmName }}</div>
                <div class="mono" style="font-size: 11px; color: var(--text-muted)">
                  {{ vm.uuid || vm.name }}
                </div>
                <div v-if="vm.description" style="font-size: 11.5px; color: var(--text-secondary)">
                  {{ vm.description }}
                </div>
              </td>
              <td>
                <span
                  class="badge"
                  :class="vm.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'"
                >
                  {{ vm.state }}
                </span>
                <span v-if="vm.autostart" class="badge badge-info" style="margin-left: 5px">
                  Autostart
                </span>
              </td>
              <td class="mono">
                {{ vm.vcpus }} vCPUs · {{ vm.memoryMib }} MiB
                <div style="font-size: 11px; color: var(--text-secondary)">
                  OS: {{ vm.osVariant }}
                </div>
              </td>
              <td>
                <div class="mono" style="font-size: 12px; color: var(--accent-primary)">
                  {{ vm.diskPath }} ({{ vm.diskSizeGib }} GiB)
                </div>
                <div v-if="vm.isoImage" class="mono" style="font-size: 11px; color: var(--text-secondary)">
                  ISO: {{ vm.isoImage }}
                </div>
              </td>
              <td class="mono" style="font-size: 12px">
                <div>Bridge: {{ vm.networkBridge }}</div>
                <div style="color: #34d399">
                  VNC {{ vm.vncDisplay || ':0' }} (TCP {{ vm.vncPort }})
                </div>
              </td>
              <td>
                <div style="display: flex; gap: 6px; flex-wrap: wrap">
                  <button class="btn btn-primary btn-sm" @click="handleOpenConsole(vm)">
                    noVNC Console
                  </button>
                  <button
                    v-if="vm.state !== 'RUNNING'"
                    class="btn btn-success btn-sm"
                    @click="handleVmPower(vm.name, 'start')"
                  >
                    `:start`
                  </button>
                  <button
                    v-if="vm.state === 'RUNNING'"
                    class="btn btn-sm"
                    @click="handleVmPower(vm.name, 'reboot')"
                  >
                    `:reboot`
                  </button>
                  <button
                    v-if="vm.state === 'RUNNING'"
                    class="btn btn-sm"
                    @click="handleVmPower(vm.name, 'stop')"
                  >
                    `:stop`
                  </button>
                  <button class="btn btn-sm" @click="startEditVm(vm)">Edit</button>
                  <button class="btn btn-danger btn-sm" @click="handleDeleteVm(vm.name)">
                    Del
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <NoVncModal
      v-if="activeConsoleSession"
      :session="activeConsoleSession"
      @close="activeConsoleSession = null"
    />
  </div>
</template>
