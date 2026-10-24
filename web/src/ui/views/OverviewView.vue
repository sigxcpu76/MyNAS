<script setup lang="ts">
import { computed } from 'vue';
import { formatBytes } from '../../domain/models';
import { nasState } from '../store';
import { navigateTo } from '../router';

const totalStorageBytes = computed(() =>
  nasState.pools.reduce((acc, p) => acc + (p.sizeBytes || 0), 0)
);
const usedStorageBytes = computed(() =>
  nasState.pools.reduce((acc, p) => acc + (p.allocatedBytes || 0), 0)
);
const runningVms = computed(() =>
  nasState.vms.filter((v) => v.state === 'RUNNING').length
);
const runningApps = computed(() =>
  nasState.dockerApps.filter((a) => a.state === 'RUNNING').length
);
const runningLxc = computed(() =>
  nasState.lxcContainers.filter((c) => c.state === 'RUNNING').length
);
const activeShares = computed(
  () =>
    nasState.cifsShares.filter((s) => s.enabled).length +
    nasState.webdavShares.filter((s) => s.enabled).length
);
const installedPrereqs = computed(
  () => nasState.prerequisites.filter((p) => p.status === 'INSTALLED').length
);
const fallbackPrereqs = computed(
  () =>
    nasState.prerequisites.filter(
      (p) => p.status === 'FALLBACK_ACTIVE' || p.fallbackMode
    ).length
);
</script>

<template>
  <div style="display: flex; flex-direction: column; gap: 20px">
    <!-- Top KPI Row -->
    <div class="grid-4">
      <div class="card">
        <div class="card-header" style="margin-bottom: 6px">
          <span class="card-subtitle">ZFS Storage Pools</span>
          <span class="badge badge-success">{{ nasState.pools.length }} Pools</span>
        </div>
        <div style="font-size: 22px; font-weight: 700">
          {{ formatBytes(usedStorageBytes) }}
          <span style="font-size: 13px; color: var(--text-secondary); font-weight: 400">
            / {{ formatBytes(totalStorageBytes) }}
          </span>
        </div>
        <p class="card-subtitle" style="margin-top: 6px">
          Snapshots, ZVols & LZ4/ZSTD datasets
        </p>
      </div>

      <div class="card">
        <div class="card-header" style="margin-bottom: 6px">
          <span class="card-subtitle">KVM Hypervisor (libvirt)</span>
          <span class="badge badge-info">{{ runningVms }} / {{ nasState.vms.length }} Running</span>
        </div>
        <div style="font-size: 22px; font-weight: 700">
          {{ nasState.vms.length }} Virtual Machines
        </div>
        <p class="card-subtitle" style="margin-top: 6px">
          Hardware virtualization + HTML5 noVNC
        </p>
      </div>

      <div class="card">
        <div class="card-header" style="margin-bottom: 6px">
          <span class="card-subtitle">Apps & Containers</span>
          <span class="badge badge-success">
            {{ runningApps + runningLxc }} Active
          </span>
        </div>
        <div style="font-size: 22px; font-weight: 700">
          {{ nasState.dockerApps.length }} Apps · {{ nasState.lxcContainers.length }} LXC
        </div>
        <p class="card-subtitle" style="margin-top: 6px">
          Application stacks & Linux containers
        </p>
      </div>

      <div class="card">
        <div class="card-header" style="margin-bottom: 6px">
          <span class="card-subtitle">Shares & Linux Identity</span>
          <span class="badge badge-info">{{ activeShares }} Shares</span>
        </div>
        <div style="font-size: 22px; font-weight: 700">
          {{ nasState.users.length }} Users · {{ nasState.groups.length }} Groups
        </div>
        <p class="card-subtitle" style="margin-top: 6px">
          Samba CIFS, WebDAV (`/dav/...`) & OpenSSH keys
        </p>
      </div>
    </div>

    <!-- Middle 2-Column Section: ZFS Pools & KVM VMs -->
    <div class="grid-2">
      <div class="card">
        <div class="card-header">
          <div>
            <h3 class="card-title">ZFS Storage Pool Telemetry</h3>
            <p class="card-subtitle">Capacity, vdev redundancy & scrub status</p>
          </div>
          <button class="btn btn-sm" @click="navigateTo('zfs')">
            Manage ZFS
          </button>
        </div>

        <div v-if="nasState.pools.length === 0" class="card-subtitle">
          No ZFS pools configured.
        </div>

        <div
          v-for="pool in nasState.pools"
          :key="pool.name"
          style="
            padding: 12px;
            background: var(--bg-base);
            border: 1px solid var(--border-subtle);
            border-radius: 8px;
            margin-bottom: 10px;
          "
        >
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
            <div>
              <strong class="mono" style="font-size: 14px">{{ pool.poolName || pool.name }}</strong>
              <span class="badge badge-neutral" style="margin-left: 8px">{{ pool.vdevLayout }}</span>
              <span class="badge badge-info" style="margin-left: 6px">{{ pool.compression }}</span>
            </div>
            <span
              class="badge"
              :class="pool.state === 'ONLINE' ? 'badge-success' : 'badge-warning'"
            >
              {{ pool.state }}
            </span>
          </div>

          <div class="progress-bar" style="margin-bottom: 6px">
            <div
              class="progress-fill"
              :style="{ width: `${Math.min(100, pool.capacityPercent || 0)}%` }"
            ></div>
          </div>

          <div style="display: flex; justify-content: space-between; font-size: 12px; color: var(--text-secondary)">
            <span>Used: {{ formatBytes(pool.allocatedBytes) }} ({{ pool.capacityPercent }}%)</span>
            <span>Free: {{ formatBytes(pool.freeBytes) }}</span>
            <span>Frag: {{ pool.fragmentationPercent }}% | Dedup: {{ pool.dedupRatio }}x</span>
          </div>
        </div>
      </div>

      <div class="card">
        <div class="card-header">
          <div>
            <h3 class="card-title">System Prerequisites Health</h3>
            <p class="card-subtitle">
              {{ installedPrereqs }} Native Installed · {{ fallbackPrereqs }} Hybrid Fallback Active
            </p>
          </div>
          <button class="btn btn-sm" @click="navigateTo('prerequisites')">
            Inspect Prerequisites
          </button>
        </div>

        <div class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>Component</th>
                <th>Category</th>
                <th>Kind</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="prereq in nasState.prerequisites.slice(0, 7)" :key="prereq.name">
                <td>
                  <strong>{{ prereq.displayName }}</strong>
                  <div class="mono" style="font-size: 11px; color: var(--text-secondary)">
                    {{ prereq.binaryOrPath }}
                  </div>
                </td>
                <td><span class="badge badge-neutral">{{ prereq.category }}</span></td>
                <td class="mono" style="font-size: 12px">{{ prereq.kind }}</td>
                <td>
                  <span
                    class="badge"
                    :class="
                      prereq.status === 'INSTALLED'
                        ? 'badge-success'
                        : prereq.status === 'FALLBACK_ACTIVE'
                        ? 'badge-warning'
                        : 'badge-danger'
                    "
                  >
                    {{ prereq.status }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Bottom Quick Overview: Active Workloads & Shares -->
    <div class="grid-3">
      <div class="card">
        <div class="card-header">
          <h3 class="card-title">KVM Virtual Machines</h3>
          <button class="btn btn-sm" @click="navigateTo('kvm')">Open KVM</button>
        </div>
        <div v-for="vm in nasState.vms" :key="vm.name" style="display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid var(--border-subtle)">
          <div>
            <div style="font-weight: 600">{{ vm.vmName }}</div>
            <div class="card-subtitle mono">{{ vm.vcpus }} vCPU · {{ vm.memoryMib }} MiB · VNC :{{ vm.vncPort }}</div>
          </div>
          <span class="badge" :class="vm.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'">
            {{ vm.state }}
          </span>
        </div>
      </div>

      <div class="card">
        <div class="card-header">
          <h3 class="card-title">Apps & LXC Containers</h3>
          <button class="btn btn-sm" @click="navigateTo('docker')">Manage Apps</button>
        </div>
        <div v-for="app in nasState.dockerApps.slice(0, 3)" :key="app.name" style="display: flex; justify-content: space-between; padding: 7px 0; border-bottom: 1px solid var(--border-subtle)">
          <div>
            <strong>[App] {{ app.title || app.appId }}</strong>
            <div class="card-subtitle mono">{{ app.containers.length }} containers · {{ app.statusText }}</div>
          </div>
          <span class="badge" :class="app.state === 'RUNNING' ? 'badge-success' : app.state === 'PARTIALLY_RUNNING' ? 'badge-warning' : 'badge-neutral'">
            {{ app.state }}
          </span>
        </div>
        <div v-for="lx in nasState.lxcContainers.slice(0, 3)" :key="lx.name" style="display: flex; justify-content: space-between; padding: 7px 0; border-bottom: 1px solid var(--border-subtle)">
          <div>
            <strong>[LXC] {{ lx.containerName }}</strong>
            <div class="card-subtitle mono">{{ lx.distribution }} {{ lx.release }} ({{ lx.ipv4Address || 'DHCP' }})</div>
          </div>
          <span class="badge" :class="lx.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'">
            {{ lx.state }}
          </span>
        </div>
      </div>

      <div class="card">
        <div class="card-header">
          <h3 class="card-title">CIFS & WebDAV Shares</h3>
          <button class="btn btn-sm" @click="navigateTo('shares')">Shares</button>
        </div>
        <div v-for="cs in nasState.cifsShares" :key="cs.name" style="display: flex; justify-content: space-between; padding: 7px 0; border-bottom: 1px solid var(--border-subtle)">
          <div>
            <strong>[SMB] //nas/{{ cs.shareName }}</strong>
            <div class="card-subtitle mono">{{ cs.path }}</div>
          </div>
          <span class="badge badge-info">{{ cs.readOnly ? 'RO' : 'RW' }}</span>
        </div>
        <div v-for="ws in nasState.webdavShares" :key="ws.name" style="display: flex; justify-content: space-between; padding: 7px 0; border-bottom: 1px solid var(--border-subtle)">
          <div>
            <strong>[WebDAV] {{ ws.urlPrefix }}</strong>
            <div class="card-subtitle mono">{{ ws.path }}</div>
          </div>
          <span class="badge badge-info">{{ ws.readOnly ? 'RO' : 'RW' }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
