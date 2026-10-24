<script setup lang="ts">
import { computed, ref } from 'vue';
import {
  mdiAlertCircle,
  mdiCheckCircle,
  mdiClockOutline,
  mdiClose,
  mdiDatabase,
  mdiFlash,
  mdiHarddisk,
  mdiInformationOutline,
  mdiMagnify,
  mdiRefresh,
  mdiServer,
  mdiShieldCheck,
  mdiSpeedometer,
  mdiThermometer,
} from '@mdi/js';
import { formatBytes, type Disk } from '../../domain/models';
import MdiIcon from '../components/MdiIcon.vue';
import { nasState, notify, refreshAllData } from '../store';
import { navigateTo } from '../router';

const searchQuery = ref('');
const selectedDisk = ref<Disk | null>(null);
const modalTab = ref<'smart' | 'hardware' | 'partitions'>('smart');
const smartSearch = ref('');

const filteredDisks = computed(() => {
  const q = searchQuery.value.trim().toLowerCase();
  // Exclude virtual ZFS volumes (zvols) which are managed in Storage > Volumes
  const nonZvolDisks = nasState.disks.filter(
    (d) => !d.diskId.startsWith('zd') && d.transport !== 'zvol' && !d.devicePath.includes('/zd')
  );
  if (!q) return nonZvolDisks;
  return nonZvolDisks.filter((d) =>
    d.diskId.toLowerCase().includes(q) ||
    d.model.toLowerCase().includes(q) ||
    d.vendor.toLowerCase().includes(q) ||
    d.serial.toLowerCase().includes(q) ||
    d.type.toLowerCase().includes(q) ||
    d.devicePath.toLowerCase().includes(q) ||
    d.owningZfsPool.toLowerCase().includes(q)
  );
});

const filteredSmartParams = computed(() => {
  if (!selectedDisk.value?.smartParameters) return [];
  const q = smartSearch.value.trim().toLowerCase();
  if (!q) return selectedDisk.value.smartParameters;
  return selectedDisk.value.smartParameters.filter((p) =>
    p.name.toLowerCase().includes(q) ||
    p.rawValue.toLowerCase().includes(q) ||
    p.type.toLowerCase().includes(q) ||
    p.status.toLowerCase().includes(q) ||
    (p.description && p.description.toLowerCase().includes(q))
  );
});

function getTempBadgeClass(temp: number) {
  if (temp <= 0) return 'badge-neutral';
  if (temp < 42) return 'badge-info';
  if (temp < 55) return 'badge-warning';
  return 'badge-danger';
}

function getSmartBadgeClass(status: string) {
  const s = status.toUpperCase();
  if (s === 'PASSED') return 'badge-success';
  if (s === 'FAILED') return 'badge-danger';
  return 'badge-neutral';
}

function getTypeBadgeClass(type: string) {
  const t = type.toUpperCase();
  if (t === 'NVME') return 'badge-purple';
  if (t === 'SSD') return 'badge-info';
  if (t === 'HDD') return 'badge-warning';
  return 'badge-neutral';
}

function goToPool(poolName: string) {
  if (!poolName) return;
  nasState.selectedPoolName = poolName.startsWith('zfsPools/') ? poolName : `zfsPools/${poolName}`;
  navigateTo('storage-pools');
}
</script>

<template>
  <div style="display: flex; flex-direction: column; gap: 20px; width: 100%; max-width: 100%; min-width: 0">
    <!-- Header Card -->
    <div class="card" style="padding: 18px 20px">
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 14px">
        <div>
          <div style="display: flex; align-items: center; gap: 10px">
            <MdiIcon :path="mdiHarddisk" size="24" style="color: var(--accent-primary)" />
            <h2 class="card-title" style="font-size: 19px; margin: 0">Storage Disks</h2>
            <span class="badge badge-info" style="font-size: 12px">
              {{ nasState.disks.length }} {{ nasState.disks.length === 1 ? 'Drive' : 'Drives' }}
            </span>
          </div>
          <p class="card-subtitle" style="margin-top: 4px; font-size: 13px">
            Physical drives discovered from Linux <code class="mono">/sys/block</code>, hardware telemetry, S.M.A.R.T. health, and ZFS pool allocation.
          </p>
        </div>

        <div style="display: flex; align-items: center; gap: 10px">
          <!-- Search input -->
          <div class="search-input-wrap">
            <MdiIcon :path="mdiMagnify" size="16" class="search-icon" />
            <input
              v-model="searchQuery"
              class="input search-input"
              placeholder="Filter disks..."
            />
          </div>

          <button class="btn btn-sm" :disabled="nasState.loading" @click="refreshAllData">
            <MdiIcon :path="mdiRefresh" size="14" style="margin-right: 4px" />
            Scan
          </button>
        </div>
      </div>
    </div>

    <!-- Disks Table Card -->
    <div class="card" style="padding: 18px 20px">
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th style="min-width: 140px">Device</th>
              <th style="min-width: 220px">Model / Vendor</th>
              <th style="min-width: 90px">Type</th>
              <th style="min-width: 100px">Capacity</th>
              <th style="min-width: 100px">Temp</th>
              <th style="min-width: 110px">S.M.A.R.T.</th>
              <th style="min-width: 140px">Owning ZFS Pool</th>
              <th style="min-width: 160px">Partitions</th>
              <th style="min-width: 90px; text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="filteredDisks.length === 0">
              <td colspan="9" style="text-align: center; color: var(--text-secondary); padding: 36px">
                <div v-if="searchQuery">
                  No storage disks match filter "<strong>{{ searchQuery }}</strong>".
                </div>
                <div v-else>
                  No physical or virtual storage drives detected on the host.
                </div>
              </td>
            </tr>

            <tr v-for="d in filteredDisks" :key="d.diskId">
              <!-- Device -->
              <td>
                <div style="display: flex; align-items: center; gap: 8px">
                  <MdiIcon
                    :path="d.type === 'NVME' ? mdiFlash : (d.rotational ? mdiHarddisk : mdiServer)"
                    size="18"
                    :style="{ color: d.type === 'NVME' ? '#c084fc' : (d.rotational ? '#f59e0b' : '#38bdf8') }"
                  />
                  <div>
                    <div class="mono" style="font-weight: 700; font-size: 13px; color: var(--text-primary)">
                      {{ d.diskId }}
                    </div>
                    <div class="mono" style="font-size: 11px; color: var(--text-secondary)">
                      {{ d.devicePath }}
                    </div>
                  </div>
                </div>
              </td>

              <!-- Model / Vendor -->
              <td>
                <div style="font-weight: 600; font-size: 13px">
                  {{ d.model || d.vendor || '-' }}
                </div>
                <div v-if="d.serial" class="mono" style="font-size: 11px; color: var(--text-secondary)">
                  S/N: {{ d.serial }}
                </div>
              </td>

              <!-- Type / Transport -->
              <td>
                <div style="display: flex; flex-direction: column; gap: 3px; align-items: flex-start">
                  <span class="badge" :class="getTypeBadgeClass(d.type)" style="font-size: 10.5px">
                    {{ d.type }}
                  </span>
                  <span v-if="d.transport" class="mono" style="font-size: 10px; color: var(--text-muted)">
                    {{ d.transport.toUpperCase() }}
                  </span>
                </div>
              </td>

              <!-- Capacity -->
              <td>
                <div class="mono" style="font-weight: 700; font-size: 13px">
                  {{ formatBytes(d.sizeBytes) }}
                </div>
                <div class="mono" style="font-size: 10.5px; color: var(--text-secondary)">
                  {{ d.logicalBlockSize }}B / {{ d.physicalBlockSize }}B
                </div>
              </td>

              <!-- Temperature -->
              <td>
                <div v-if="d.temperatureCelsius > 0" style="display: flex; align-items: center; gap: 4px">
                  <MdiIcon :path="mdiThermometer" size="14" style="color: #38bdf8" />
                  <span class="badge mono" :class="getTempBadgeClass(d.temperatureCelsius)" style="font-size: 11px">
                    {{ d.temperatureCelsius.toFixed(1) }}°C
                  </span>
                </div>
                <span v-else class="mono" style="color: var(--text-muted); font-size: 11px">-</span>
              </td>

              <!-- S.M.A.R.T. Health -->
              <td>
                <div style="display: flex; flex-direction: column; gap: 2px">
                  <span class="badge mono" :class="getSmartBadgeClass(d.smartStatus)" style="font-size: 10.5px">
                    {{ d.smartStatus }}
                  </span>
                  <span v-if="d.healthPercentage > 0 && d.healthPercentage < 100" class="mono" style="font-size: 10px; color: var(--text-secondary)">
                    Health: {{ d.healthPercentage }}%
                  </span>
                </div>
              </td>

              <!-- Owning ZFS Pool -->
              <td>
                <button
                  v-if="d.owningZfsPool"
                  class="btn btn-sm mono"
                  style="font-size: 11px; padding: 3px 8px; border-color: rgba(56, 189, 248, 0.4); background: rgba(56, 189, 248, 0.1); color: var(--accent-primary)"
                  title="Click to view owning ZFS Pool"
                  @click="goToPool(d.owningZfsPool)"
                >
                  <MdiIcon :path="mdiDatabase" size="13" style="margin-right: 4px" />
                  {{ d.owningZfsPool }}
                </button>
                <span v-else class="mono" style="color: var(--text-muted); font-size: 11.5px">
                  Unallocated
                </span>
              </td>

              <!-- Partitions -->
              <td>
                <div v-if="d.partitions && d.partitions.length > 0" style="display: flex; flex-wrap: wrap; gap: 4px">
                  <span
                    v-for="p in d.partitions"
                    :key="p.name"
                    class="badge mono"
                    :class="p.owningZfsPool ? 'badge-info' : 'badge-neutral'"
                    style="font-size: 10px"
                    :title="`${p.devicePath} (${formatBytes(p.sizeBytes)})${p.mountpoint ? ' mounted at ' + p.mountpoint : ''}${p.owningZfsPool ? ' [ZFS: ' + p.owningZfsPool + ']' : ''}`"
                  >
                    {{ p.name }} ({{ formatBytes(p.sizeBytes) }})
                  </span>
                </div>
                <span v-else class="mono" style="color: var(--text-muted); font-size: 11px">
                  Whole disk
                </span>
              </td>

              <!-- Actions -->
              <td style="text-align: right">
                <button
                  class="btn btn-sm"
                  title="View Hardware & S.M.A.R.T. Details"
                  @click="selectedDisk = d"
                >
                  <MdiIcon :path="mdiInformationOutline" size="14" style="margin-right: 4px" />
                  Details
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Disk Inspector Modal -->
    <div v-if="selectedDisk" class="modal-overlay" @click.self="selectedDisk = null">
      <div class="modal-container" style="max-width: 860px; width: 100%">
        <div class="modal-header">
          <div style="display: flex; align-items: center; gap: 10px">
            <MdiIcon
              :path="selectedDisk.type === 'NVME' ? mdiFlash : (selectedDisk.rotational ? mdiHarddisk : mdiServer)"
              size="22"
              :style="{ color: selectedDisk.type === 'NVME' ? '#c084fc' : (selectedDisk.rotational ? '#f59e0b' : '#38bdf8') }"
            />
            <div>
              <h3 class="card-title" style="margin: 0; font-size: 15px">
                Disk Inspector: <span class="mono">{{ selectedDisk.diskId }}</span>
              </h3>
              <div class="mono" style="font-size: 11px; color: var(--text-secondary)">
                {{ selectedDisk.model || selectedDisk.vendor || selectedDisk.devicePath }}
              </div>
            </div>
          </div>
          <button class="btn btn-sm btn-icon" @click="selectedDisk = null">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>

        <!-- Modal Sub-Navigation Tabs -->
        <div class="modal-nav-tabs">
          <button
            class="modal-nav-tab"
            :class="{ active: modalTab === 'smart' }"
            @click="modalTab = 'smart'"
          >
            <MdiIcon :path="mdiSpeedometer" size="15" style="margin-right: 6px" />
            S.M.A.R.T. Telemetry
            <span class="badge badge-neutral" style="font-size: 10px; margin-left: 6px">
              {{ selectedDisk.smartParameters?.length || 0 }}
            </span>
          </button>
          <button
            class="modal-nav-tab"
            :class="{ active: modalTab === 'hardware' }"
            @click="modalTab = 'hardware'"
          >
            <MdiIcon :path="mdiInformationOutline" size="15" style="margin-right: 6px" />
            Hardware & Sysfs
          </button>
          <button
            class="modal-nav-tab"
            :class="{ active: modalTab === 'partitions' }"
            @click="modalTab = 'partitions'"
          >
            <MdiIcon :path="mdiDatabase" size="15" style="margin-right: 6px" />
            Partitions
            <span class="badge badge-neutral" style="font-size: 10px; margin-left: 6px">
              {{ selectedDisk.partitions?.length || 0 }}
            </span>
          </button>
        </div>

        <div class="modal-body" style="display: flex; flex-direction: column; gap: 16px; max-height: 70vh; overflow-y: auto">
          <!-- Overview Stats Grid (Always Visible) -->
          <div class="info-stat-row">
            <div class="stat-col">
              <span class="stat-label">Capacity</span>
              <span class="stat-value text-sky mono">{{ formatBytes(selectedDisk.sizeBytes) }}</span>
            </div>
            <div class="stat-col">
              <span class="stat-label">S.M.A.R.T. Health</span>
              <span class="stat-value mono" :class="selectedDisk.smartStatus === 'PASSED' ? 'text-emerald' : 'text-sky'">
                {{ selectedDisk.smartStatus }}
                <span v-if="selectedDisk.healthPercentage > 0 && selectedDisk.healthPercentage < 100" style="font-size: 11px; color: var(--text-secondary)">
                  ({{ selectedDisk.healthPercentage }}%)
                </span>
              </span>
            </div>
            <div class="stat-col">
              <span class="stat-label">Temperature</span>
              <span class="stat-value text-sky mono">
                {{ selectedDisk.temperatureCelsius > 0 ? `${selectedDisk.temperatureCelsius.toFixed(1)}°C` : 'N/A' }}
              </span>
            </div>
            <div v-if="selectedDisk.powerOnHours" class="stat-col">
              <span class="stat-label">Power-On Time</span>
              <span class="stat-value mono" style="font-size: 13px" :title="`${(selectedDisk.powerOnHours / 24).toFixed(1)} operating days`">
                {{ selectedDisk.powerOnHours.toLocaleString() }} hrs
              </span>
            </div>
            <div v-if="selectedDisk.totalBytesWritten" class="stat-col">
              <span class="stat-label">Total Written</span>
              <span class="stat-value mono" style="font-size: 13px; color: #a78bfa">
                {{ formatBytes(selectedDisk.totalBytesWritten) }}
              </span>
            </div>
            <div v-if="selectedDisk.powerCycleCount" class="stat-col">
              <span class="stat-label">Power Cycles</span>
              <span class="stat-value mono" style="font-size: 13px">
                {{ selectedDisk.powerCycleCount }}
              </span>
            </div>
          </div>

          <!-- TAB 1: S.M.A.R.T. PARAMETERS -->
          <div v-if="modalTab === 'smart'" style="display: flex; flex-direction: column; gap: 12px">
            <div style="display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap">
              <div style="font-weight: 600; font-size: 13px; display: flex; align-items: center; gap: 6px">
                <span>Diagnostic Attributes & Parameters</span>
                <span class="badge badge-neutral mono" style="font-size: 10.5px">
                  {{ filteredSmartParams.length }} of {{ selectedDisk.smartParameters?.length || 0 }}
                </span>
              </div>
              <div class="search-input-wrap">
                <MdiIcon :path="mdiMagnify" size="14" class="search-icon" />
                <input
                  v-model="smartSearch"
                  class="input search-input"
                  style="width: 220px; height: 30px; font-size: 11.5px"
                  placeholder="Filter parameters..."
                />
              </div>
            </div>

            <!-- Parameters Table -->
            <div v-if="filteredSmartParams.length > 0" class="table-wrap" style="max-height: 380px; overflow-y: auto">
              <table class="data-table">
                <thead>
                  <tr>
                    <th style="min-width: 45px">ID</th>
                    <th style="min-width: 220px">Parameter Name</th>
                    <th style="min-width: 150px">Raw Value</th>
                    <th style="min-width: 110px">Normalized</th>
                    <th style="min-width: 80px">Status</th>
                    <th style="min-width: 85px">Type</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="sp in filteredSmartParams" :key="sp.id + '-' + sp.name">
                    <!-- ID -->
                    <td class="mono font-semibold" style="color: var(--text-secondary); font-size: 11px">
                      #{{ sp.id.toString().padStart(2, '0') }}
                    </td>

                    <!-- Name & Description -->
                    <td>
                      <div class="mono font-semibold" style="font-size: 12px; color: var(--text-primary)">
                        {{ sp.name }}
                      </div>
                      <div v-if="sp.description" style="font-size: 10.5px; color: var(--text-secondary); line-height: 1.3; margin-top: 2px">
                        {{ sp.description }}
                      </div>
                    </td>

                    <!-- Raw Value -->
                    <td>
                      <span class="mono font-semibold" style="color: var(--accent-primary); font-size: 12px">
                        {{ sp.rawValue || '-' }}
                      </span>
                    </td>

                    <!-- Normalized (Val / Worst / Thresh) -->
                    <td class="mono" style="font-size: 11px; color: var(--text-secondary)">
                      <span v-if="sp.value && sp.value !== 'N/A'">
                        {{ sp.value }}
                        <span v-if="sp.worst && sp.worst !== '-'"> / {{ sp.worst }}</span>
                        <span v-if="sp.threshold && sp.threshold !== '-'" style="color: var(--text-muted)"> (T: {{ sp.threshold }})</span>
                      </span>
                      <span v-else>-</span>
                    </td>

                    <!-- Status -->
                    <td>
                      <span
                        class="badge mono"
                        :class="sp.status === 'OK' || sp.status === 'PASSED' ? 'badge-success' : (sp.status === 'WARNING' ? 'badge-warning' : 'badge-danger')"
                        style="font-size: 10px"
                      >
                        {{ sp.status }}
                      </span>
                    </td>

                    <!-- Type -->
                    <td>
                      <span class="badge mono badge-neutral" style="font-size: 10px">
                        {{ sp.type || 'Standard' }}
                      </span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-else style="color: var(--text-secondary); font-size: 12px; padding: 24px; text-align: center; background: var(--bg-base); border-radius: 6px">
              <span v-if="smartSearch">No parameters match filter "<strong>{{ smartSearch }}</strong>".</span>
              <span v-else>No S.M.A.R.T. parameters reported for this storage device.</span>
            </div>
          </div>

          <!-- TAB 2: HARDWARE & SYSFS -->
          <div v-else-if="modalTab === 'hardware'" style="background: var(--bg-base); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 14px">
            <div style="font-weight: 600; font-size: 13px; margin-bottom: 12px; color: var(--text-primary)">
              Linux <code class="mono">/sys/block/{{ selectedDisk.diskId }}</code> Attributes
            </div>
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px 20px; font-size: 12px">
              <div><span style="color: var(--text-secondary)">Device Path:</span> <span class="mono font-semibold" style="margin-left: 6px">{{ selectedDisk.devicePath }}</span></div>
              <div><span style="color: var(--text-secondary)">Model:</span> <span class="font-semibold" style="margin-left: 6px">{{ selectedDisk.model || '-' }}</span></div>
              <div><span style="color: var(--text-secondary)">Vendor:</span> <span class="font-semibold" style="margin-left: 6px">{{ selectedDisk.vendor || '-' }}</span></div>
              <div><span style="color: var(--text-secondary)">Serial:</span> <span class="mono font-semibold" style="margin-left: 6px">{{ selectedDisk.serial || '-' }}</span></div>
              <div><span style="color: var(--text-secondary)">Firmware:</span> <span class="mono font-semibold" style="margin-left: 6px">{{ selectedDisk.firmwareRev || '-' }}</span></div>
              <div><span style="color: var(--text-secondary)">Type:</span> <span class="mono font-semibold" style="margin-left: 6px">{{ selectedDisk.type }} (Rotational: {{ selectedDisk.rotational }})</span></div>
              <div><span style="color: var(--text-secondary)">Transport:</span> <span class="mono font-semibold" style="margin-left: 6px">{{ selectedDisk.transport || '-' }}</span></div>
              <div><span style="color: var(--text-secondary)">Block Sizes:</span> <span class="mono font-semibold" style="margin-left: 6px">Logical: {{ selectedDisk.logicalBlockSize }}B / Physical: {{ selectedDisk.physicalBlockSize }}B</span></div>
              <div><span style="color: var(--text-secondary)">Removable:</span> <span class="mono font-semibold" style="margin-left: 6px">{{ selectedDisk.removable ? 'Yes' : 'No' }}</span></div>
              <div><span style="color: var(--text-secondary)">Kernel State:</span> <span class="mono font-semibold" style="margin-left: 6px">{{ selectedDisk.state || 'running' }}</span></div>
              <div v-if="selectedDisk.owningZfsPool" style="grid-column: span 2">
                <span style="color: var(--text-secondary)">Owning ZFS Pool:</span>
                <span class="mono" style="color: var(--accent-primary); font-weight: 700; margin-left: 6px">{{ selectedDisk.owningZfsPool }}</span>
              </div>
            </div>
          </div>

          <!-- TAB 3: PARTITIONS -->
          <div v-else-if="modalTab === 'partitions'">
            <div style="font-weight: 600; font-size: 13px; margin-bottom: 8px">
              Partitions Table ({{ selectedDisk.partitions?.length || 0 }})
            </div>
            <div v-if="selectedDisk.partitions && selectedDisk.partitions.length > 0" class="table-wrap">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>Partition</th>
                    <th>Path</th>
                    <th>Size</th>
                    <th>Mountpoint</th>
                    <th>ZFS Pool</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="p in selectedDisk.partitions" :key="p.name">
                    <td class="mono font-semibold">{{ p.name }}</td>
                    <td class="mono" style="font-size: 11px">{{ p.devicePath }}</td>
                    <td class="mono">{{ formatBytes(p.sizeBytes) }}</td>
                    <td class="mono" style="font-size: 11px">{{ p.mountpoint || '-' }}</td>
                    <td class="mono" style="color: var(--accent-primary)">{{ p.owningZfsPool || '-' }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-else style="color: var(--text-secondary); font-size: 12px; padding: 12px; background: var(--bg-base); border-radius: 6px">
              No partitions found. This device is managed as a whole block storage disk.
            </div>
          </div>
        </div>

        <div style="padding: 14px 20px; border-top: 1px solid var(--border-subtle); display: flex; justify-content: flex-end">
          <button class="btn btn-sm" @click="selectedDisk = null">Close</button>
        </div>
      </div>
    </div>
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
  color: var(--text-secondary);
  pointer-events: none;
}

.search-input {
  padding-left: 30px;
  height: 32px;
  font-size: 12px;
  width: 190px;
}

.badge-purple {
  background: rgba(168, 85, 247, 0.15);
  color: #c084fc;
  border: 1px solid rgba(168, 85, 247, 0.3);
}

.modal-nav-tabs {
  display: flex;
  gap: 4px;
  padding: 0 20px;
  border-bottom: 1px solid var(--border-subtle);
  background: rgba(15, 23, 42, 0.4);
}

.modal-nav-tab {
  display: flex;
  align-items: center;
  padding: 10px 14px;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-secondary);
  background: transparent;
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

.info-stat-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: 8px;
  background: rgba(0, 0, 0, 0.2);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 10px 12px;
}

.stat-col {
  display: flex;
  flex-direction: column;
}

.stat-label {
  font-size: 10px;
  font-weight: 600;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.stat-value {
  font-size: 13.5px;
  font-weight: 700;
  margin-top: 2px;
}

.text-sky {
  color: #38bdf8;
}

.text-emerald {
  color: #34d399;
}

.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.75);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal-container {
  background: var(--bg-surface);
  border: 1px solid var(--border-strong);
  border-radius: 12px;
  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
  max-height: 85vh;
  overflow: hidden;
}

.modal-header {
  padding: 16px 20px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid var(--border-subtle);
}

.modal-body {
  padding: 20px;
  overflow-y: auto;
}
</style>
