<script setup lang="ts">
import { nasState } from '../../store';
import { navigateTo } from '../../router';
import SkeletonBox from '../../components/SkeletonBox.vue';
</script>

<template>
  <div class="workloads-widget">
    <div class="workloads-grid">
      <!-- Virtual Machines -->
      <div class="card workload-subcard">
        <div class="card-header">
          <h3 class="card-title">Virtual Machines</h3>
          <button class="btn btn-sm" @click="navigateTo('kvm')">Manage VMs</button>
        </div>
        <div v-if="nasState.loading && nasState.vms.length === 0" style="padding: 6px 0; display: flex; flex-direction: column; gap: 10px">
          <div v-for="s in 3" :key="s" style="display: flex; justify-content: space-between; align-items: center; padding: 6px 0; border-bottom: 1px solid var(--border-subtle)">
            <div style="display: flex; flex-direction: column; gap: 4px">
              <SkeletonBox width="110px" height="15px" />
              <SkeletonBox width="150px" height="11px" />
            </div>
            <SkeletonBox width="60px" height="20px" rounded="10px" />
          </div>
        </div>
        <div v-else-if="nasState.vms.length === 0" class="card-subtitle" style="padding: 12px 0">
          No virtual machines created yet.
        </div>
        <div
          v-for="vm in nasState.vms.slice(0, 4)"
          :key="vm.name"
          style="display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid var(--border-subtle)"
        >
          <div>
            <div style="font-weight: 600">{{ vm.vmName }}</div>
            <div class="card-subtitle mono">{{ vm.vcpus }} vCPU · {{ vm.memoryMib }} MiB · VNC :{{ vm.vncPort }}</div>
          </div>
          <span class="badge" :class="vm.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'">
            {{ vm.state }}
          </span>
        </div>
      </div>

      <!-- Apps & Containers -->
      <div class="card workload-subcard">
        <div class="card-header">
          <h3 class="card-title">Apps & Containers</h3>
          <button class="btn btn-sm" @click="navigateTo('docker')">Manage Apps</button>
        </div>
        <div v-if="nasState.loading && nasState.dockerApps.length === 0 && nasState.lxcContainers.length === 0" style="padding: 6px 0; display: flex; flex-direction: column; gap: 10px">
          <div v-for="s in 3" :key="s" style="display: flex; justify-content: space-between; align-items: center; padding: 6px 0; border-bottom: 1px solid var(--border-subtle)">
            <div style="display: flex; flex-direction: column; gap: 4px">
              <SkeletonBox width="120px" height="15px" />
              <SkeletonBox width="140px" height="11px" />
            </div>
            <SkeletonBox width="65px" height="20px" rounded="10px" />
          </div>
        </div>
        <div v-else-if="nasState.dockerApps.length === 0 && nasState.lxcContainers.length === 0" class="card-subtitle" style="padding: 12px 0">
          No apps or containers running.
        </div>
        <div
          v-for="app in nasState.dockerApps.slice(0, 3)"
          :key="app.name"
          style="display: flex; justify-content: space-between; padding: 7px 0; border-bottom: 1px solid var(--border-subtle)"
        >
          <div>
            <strong>[App] {{ app.title || app.appId }}</strong>
            <div class="card-subtitle mono">{{ app.containers.length }} containers · {{ app.statusText }}</div>
          </div>
          <span class="badge" :class="app.state === 'RUNNING' ? 'badge-success' : app.state === 'DEPLOYING' ? 'badge-info' : app.state === 'PARTIALLY_RUNNING' ? 'badge-warning' : app.state === 'ERROR' ? 'badge-danger' : 'badge-neutral'">
            {{ app.state }}
          </span>
        </div>
        <div
          v-for="lx in nasState.lxcContainers.slice(0, 2)"
          :key="lx.name"
          style="display: flex; justify-content: space-between; padding: 7px 0; border-bottom: 1px solid var(--border-subtle)"
        >
          <div>
            <strong>[LXC] {{ lx.containerName }}</strong>
            <div class="card-subtitle mono">{{ lx.distribution }} ({{ lx.ipv4Address || 'DHCP' }})</div>
          </div>
          <span class="badge" :class="lx.state === 'RUNNING' ? 'badge-success' : 'badge-neutral'">
            {{ lx.state }}
          </span>
        </div>
      </div>

      <!-- File Shares -->
      <div class="card workload-subcard">
        <div class="card-header">
          <h3 class="card-title">Shares</h3>
          <button class="btn btn-sm" @click="navigateTo('shares')">Manage Shares</button>
        </div>
        <div v-if="nasState.loading && nasState.cifsShares.length === 0 && nasState.webdavShares.length === 0" style="padding: 6px 0; display: flex; flex-direction: column; gap: 10px">
          <div v-for="s in 3" :key="s" style="display: flex; justify-content: space-between; align-items: center; padding: 6px 0; border-bottom: 1px solid var(--border-subtle)">
            <div style="display: flex; flex-direction: column; gap: 4px">
              <SkeletonBox width="130px" height="15px" />
              <SkeletonBox width="160px" height="11px" />
            </div>
            <SkeletonBox width="60px" height="20px" rounded="10px" />
          </div>
        </div>
        <div v-else-if="nasState.cifsShares.length === 0 && nasState.webdavShares.length === 0" class="card-subtitle" style="padding: 12px 0">
          No file shares configured.
        </div>
        <div
          v-for="cs in nasState.cifsShares.slice(0, 3)"
          :key="cs.name"
          style="display: flex; justify-content: space-between; padding: 7px 0; border-bottom: 1px solid var(--border-subtle)"
        >
          <div>
            <strong>[SMB] //nas/{{ cs.shareName }}</strong>
            <div class="card-subtitle mono">{{ cs.path }}</div>
          </div>
          <span class="badge badge-info">{{ cs.readOnly ? 'RO' : 'RW' }}</span>
        </div>
        <div
          v-for="ws in nasState.webdavShares.slice(0, 2)"
          :key="ws.name"
          style="display: flex; justify-content: space-between; padding: 7px 0; border-bottom: 1px solid var(--border-subtle)"
        >
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

<style scoped>
.workloads-widget {
  width: 100%;
}

.workloads-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 14px;
}

.workload-subcard {
  height: 100%;
  display: flex;
  flex-direction: column;
}
</style>
