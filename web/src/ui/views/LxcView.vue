<script setup lang="ts">
import { reactive, ref } from 'vue';
import { services } from '../../container';
import { nasState, notify } from '../store';

const showCreate = ref(false);
const form = reactive({
  containerId: '',
  distribution: 'debian',
  release: 'bookworm',
  architecture: 'amd64',
  memoryLimitMib: 1024,
  cpuCores: 2,
  unprivileged: true,
  autostart: true,
});

async function refreshLxc() {
  nasState.lxcContainers = await services.lxc.listContainers();
}

async function handleCreate() {
  try {
    const created = await services.lxc.createContainer(form.containerId, {
      distribution: form.distribution,
      release: form.release,
      architecture: form.architecture,
      memoryLimitMib: form.memoryLimitMib,
      cpuCores: form.cpuCores,
      unprivileged: form.unprivileged,
      autostart: form.autostart,
    });
    notify('success', `Created LXC system container ${created.containerName}`);
    form.containerId = '';
    showCreate.value = false;
    await refreshLxc();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to create LXC container');
  }
}

async function handleAction(name: string, verb: 'start' | 'stop' | 'freeze') {
  try {
    if (verb === 'start') await services.lxc.startContainer(name);
    else if (verb === 'stop') await services.lxc.stopContainer(name);
    else await services.lxc.freezeContainer(name);
    notify('success', `LXC :${verb} succeeded on ${name}`);
    await refreshLxc();
  } catch (err: any) {
    notify('error', err?.message || `Failed to ${verb} LXC container`);
  }
}

async function handleDelete(name: string) {
  try {
    await services.lxc.deleteContainer(name, true);
    notify('info', `Deleted LXC container ${name}`);
    await refreshLxc();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete LXC container');
  }
}
</script>

<template>
  <div class="card">
    <div class="card-header">
      <div>
        <h2 class="card-title" style="font-size: 17px">
          LXC Linux System Containers (`lxcContainers/{lxc_container}`)
        </h2>
        <p class="card-subtitle">
          Lightweight unprivileged OS containers backed by ZFS rootfs datasets (`lxc-create`,
          `lxc-start`, `lxc-stop`, `lxc-freeze`).
        </p>
      </div>

      <button class="btn btn-primary" @click="showCreate = !showCreate">
        {{ showCreate ? 'Cancel' : '+ Create LXC Container' }}
      </button>
    </div>

    <form
      v-if="showCreate"
      @submit.prevent="handleCreate"
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
          <label>Container Name (`lxc_container_id`)</label>
          <input v-model="form.containerId" class="input mono" placeholder="wireguard-gw" required />
        </div>
        <div class="form-group">
          <label>Distribution</label>
          <select v-model="form.distribution" class="select">
            <option value="debian">debian</option>
            <option value="ubuntu">ubuntu</option>
            <option value="alpine">alpine</option>
            <option value="archlinux">archlinux</option>
          </select>
        </div>
        <div class="form-group">
          <label>Release</label>
          <input v-model="form.release" class="input mono" placeholder="bookworm / noble / 3.20" required />
        </div>
        <div class="form-group">
          <label>Architecture</label>
          <select v-model="form.architecture" class="select">
            <option value="amd64">amd64</option>
            <option value="arm64">arm64</option>
          </select>
        </div>
        <div class="form-group">
          <label>CPU Cores (`lxc.cgroup2.cpuset.cpus`)</label>
          <input v-model.number="form.cpuCores" type="number" min="1" max="32" class="input mono" />
        </div>
        <div class="form-group">
          <label>Memory Limit (MiB)</label>
          <input v-model.number="form.memoryLimitMib" type="number" min="128" step="128" class="input mono" />
        </div>
      </div>

      <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 12px">
        <div style="display: flex; gap: 16px; font-size: 12.5px">
          <label style="display: flex; align-items: center; gap: 6px">
            <input v-model="form.unprivileged" type="checkbox" /> Unprivileged UID Mapping
          </label>
          <label style="display: flex; align-items: center; gap: 6px">
            <input v-model="form.autostart" type="checkbox" /> Autostart (`lxc.start.auto = 1`)
          </label>
        </div>
        <button type="submit" class="btn btn-primary">Provision LXC Container</button>
      </div>
    </form>

    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <th>LXC Container</th>
            <th>State</th>
            <th>Distro / Release / Arch</th>
            <th>IPv4 Address</th>
            <th>Rootfs Path</th>
            <th>Resources</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="lx in nasState.lxcContainers" :key="lx.name">
            <td>
              <div style="font-weight: 600">{{ lx.containerName }}</div>
              <div class="mono" style="font-size: 11px; color: var(--text-muted)">{{ lx.name }}</div>
            </td>
            <td>
              <span
                class="badge"
                :class="
                  lx.state === 'RUNNING'
                    ? 'badge-success'
                    : lx.state === 'FROZEN'
                    ? 'badge-warning'
                    : 'badge-neutral'
                "
              >
                {{ lx.state }}
              </span>
              <span v-if="lx.unprivileged" class="badge badge-info" style="margin-left: 4px">
                Unpriv
              </span>
            </td>
            <td class="mono">
              {{ lx.distribution }} / {{ lx.release }} ({{ lx.architecture }})
            </td>
            <td class="mono" style="color: #34d399">
              {{ lx.ipv4Address || '—' }}
            </td>
            <td class="mono" style="font-size: 12px; color: var(--accent-primary)">
              {{ lx.rootfsPath }}
            </td>
            <td class="mono" style="font-size: 12px">
              {{ lx.cpuCores }} Cores · {{ lx.memoryLimitMib }} MiB
            </td>
            <td>
              <div style="display: flex; gap: 6px; flex-wrap: wrap">
                <button
                  v-if="lx.state !== 'RUNNING'"
                  class="btn btn-success btn-sm"
                  @click="handleAction(lx.name, 'start')"
                >
                  `:start`
                </button>
                <button
                  v-if="lx.state === 'RUNNING'"
                  class="btn btn-sm"
                  @click="handleAction(lx.name, 'freeze')"
                >
                  `:freeze`
                </button>
                <button
                  v-if="lx.state !== 'STOPPED'"
                  class="btn btn-sm"
                  @click="handleAction(lx.name, 'stop')"
                >
                  `:stop`
                </button>
                <button class="btn btn-danger btn-sm" @click="handleDelete(lx.name)">
                  Del
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
