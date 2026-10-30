<script setup lang="ts">
import { computed, ref } from 'vue';
import {
  mdiCheck,
  mdiChip,
  mdiClose,
  mdiConsole,
  mdiContentCopy,
  mdiCpu64Bit,
  mdiHarddisk,
  mdiLanConnect,
  mdiLinux,
  mdiMemory,
  mdiRefresh,
  mdiServer,
  mdiSpeedometer,
  mdiThermometer,
} from '@mdi/js';
import { formatBytes, formatUptime } from '../../domain/models';
import MdiIcon from './MdiIcon.vue';
import { nasState, notify, refreshTelemetry } from '../store';

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'openTerminal'): void;
}>();

const telemetry = computed(() => nasState.systemTelemetry);
const isRefreshing = ref(false);
const copied = ref(false);

const host = computed(() => telemetry.value?.host);
const cpu = computed(() => telemetry.value?.cpu);
const mem = computed(() => telemetry.value?.memory);

const primaryInterface = computed(() => {
  const list = telemetry.value?.network || [];
  if (list.length === 0) return null;
  return list.find((n) => n.isDefault) || list.find((n) => n.isUp && n.name !== 'lo') || list[0];
});

const totalStorageBytes = computed(() =>
  nasState.pools.reduce((acc, p) => acc + (p.sizeBytes || 0), 0)
);
const usedStorageBytes = computed(() =>
  nasState.pools.reduce((acc, p) => acc + (p.allocatedBytes || 0), 0)
);

function getTempBadgeClass(temp: number): string {
  if (temp <= 0) return 'badge-neutral';
  if (temp < 65) return 'badge-success';
  if (temp < 80) return 'badge-warning';
  return 'badge-danger';
}

async function handleRefresh() {
  isRefreshing.value = true;
  try {
    await refreshTelemetry();
    notify('success', 'System telemetry refreshed');
  } catch (err: any) {
    notify('error', err?.message || 'Failed to refresh telemetry');
  } finally {
    isRefreshing.value = false;
  }
}

async function copySystemSummary() {
  const h = host.value;
  const c = cpu.value;
  const m = mem.value;
  const net = primaryInterface.value;

  const gpusText = h?.gpus && h.gpus.length > 0 ? h.gpus.join(', ') : 'None detected';
  const ipsText = net?.ipAddresses?.join(', ') || 'N/A';

  const summary = [
    '================= System Hardware & Platform Summary =================',
    `Host:         ${h?.hostname || 'N/A'}`,
    `OS:           ${h?.osName || 'Linux'} ${h?.osVersion || ''} (${h?.architecture || 'x86_64'})`,
    `Kernel:       ${h?.kernelVersion || 'N/A'}`,
    `Uptime:       ${formatUptime(h?.uptimeSeconds || 0)} (${h?.uptimeSeconds || 0}s)`,
    `Motherboard:  ${h?.motherboardVendor || ''} ${h?.motherboardName || 'Custom Board'} ${h?.motherboardVersion ? '(' + h.motherboardVersion + ')' : ''}`.trim(),
    `BIOS / UEFI:  ${h?.biosVendor || 'UEFI'} ${h?.biosVersion || ''} ${h?.biosDate ? '(' + h.biosDate + ')' : ''}`.trim(),
    `CPU:          ${c?.model || 'N/A'}`,
    `Topology:     ${c?.physicalCores || 0} Cores · ${c?.logicalCores || 0} Threads`,
    `Load Averages:${c?.load1?.toFixed(2) || '0.00'} (1m), ${c?.load5?.toFixed(2) || '0.00'} (5m), ${c?.load15?.toFixed(2) || '0.00'} (15m)`,
    `Temperature:  ${(c?.temperatureCelsius || 0) > 0 ? c?.temperatureCelsius?.toFixed(1) + ' °C' : 'N/A'}`,
    `Memory (RAM): ${formatBytes(m?.usedBytes || 0)} used / ${formatBytes(m?.totalBytes || 0)} total (Free: ${formatBytes(m?.freeBytes || 0)})`,
    `ZFS ARC:      ${formatBytes(m?.zfsArcBytes || 0)} (Max: ${formatBytes(m?.zfsArcMaxBytes || 0)})`,
    `Storage:      ${formatBytes(usedStorageBytes.value)} allocated across ${nasState.pools.length} pool(s)`,
    `Network:      ${net?.name || 'N/A'} (Speed: ${net?.speedMbps || 0} Mbps, IP: ${ipsText}, MAC: ${net?.macAddress || 'N/A'})`,
    `GPU(s):       ${gpusText}`,
    '======================================================================',
  ].join('\n');

  try {
    await navigator.clipboard.writeText(summary);
    copied.value = true;
    notify('info', 'System summary copied to clipboard');
    setTimeout(() => {
      copied.value = false;
    }, 2500);
  } catch (err: any) {
    notify('error', 'Failed to copy system summary');
  }
}
</script>

<template>
  <div class="modal-backdrop" @click.self="emit('close')">
    <div class="modal-card modal-card-large system-modal">
      <!-- Modal Header -->
      <div class="modal-header">
        <div style="display: flex; align-items: center; gap: 12px">
          <div class="system-icon-badge">
            <MdiIcon :path="mdiServer" size="24" />
          </div>
          <div>
            <div style="display: flex; align-items: center; gap: 8px">
              <h3 class="modal-title" style="margin: 0; font-size: 18px">System Information</h3>
              <span class="badge badge-success mono">
                {{ host?.hostname || 'nan-nas' }}
              </span>
              <span class="badge badge-neutral mono">
                {{ host?.architecture || 'x86_64' }}
              </span>
            </div>
            <p class="mono" style="font-size: 11.5px; color: var(--text-secondary); margin-top: 3px; margin-bottom: 0">
              {{ host?.osName || 'Linux' }} {{ host?.osVersion || '' }} · Kernel {{ host?.kernelVersion || '' }}
            </p>
          </div>
        </div>

        <div style="display: flex; align-items: center; gap: 8px">
          <button
            class="btn btn-sm"
            :disabled="isRefreshing"
            title="Refresh System Telemetry"
            @click="handleRefresh"
          >
            <MdiIcon :path="mdiRefresh" size="14" :class="{ 'spin-icon': isRefreshing }" style="margin-right: 4px" />
            {{ isRefreshing ? 'Refreshing...' : 'Refresh' }}
          </button>
          <button
            class="btn btn-sm btn-secondary"
            title="Open interactive host shell terminal"
            @click="emit('openTerminal')"
          >
            <MdiIcon :path="mdiConsole" size="14" style="margin-right: 4px" />
            Host Shell
          </button>
          <button
            class="btn btn-sm btn-secondary"
            title="Copy system specifications to clipboard"
            @click="copySystemSummary"
          >
            <MdiIcon :path="copied ? mdiCheck : mdiContentCopy" size="14" style="margin-right: 4px" />
            {{ copied ? 'Copied!' : 'Copy Summary' }}
          </button>
          <button class="btn btn-sm btn-icon" title="Close" aria-label="Close" @click="emit('close')">
            <MdiIcon :path="mdiClose" size="18" />
          </button>
        </div>
      </div>

      <!-- Modal Body -->
      <div class="modal-scroll-body system-details-grid">
        <!-- 1. Platform & Motherboard -->
        <div class="details-section">
          <div class="section-title">
            <MdiIcon :path="mdiChip" size="16" class="section-icon" />
            <span>Platform & Firmware</span>
          </div>
          <div class="details-card">
            <div class="spec-row">
              <span class="spec-label">Host Name:</span>
              <span class="spec-val mono font-semibold">{{ host?.hostname || 'N/A' }}</span>
            </div>
            <div class="spec-row">
              <span class="spec-label">Motherboard:</span>
              <span class="spec-val">
                {{ host?.motherboardVendor || '' }} {{ host?.motherboardName || 'Custom Platform' }}
                <span v-if="host?.motherboardVersion" class="mono text-muted" style="font-size: 11px">
                  ({{ host.motherboardVersion }})
                </span>
              </span>
            </div>
            <div class="spec-row">
              <span class="spec-label">BIOS / UEFI:</span>
              <span class="spec-val mono">
                {{ host?.biosVendor || 'UEFI' }} {{ host?.biosVersion || '' }}
                <span v-if="host?.biosDate" class="text-muted" style="font-size: 11px">
                  · {{ host.biosDate }}
                </span>
              </span>
            </div>
            <div class="spec-row">
              <span class="spec-label">System Uptime:</span>
              <span class="spec-val mono" style="color: var(--accent-primary); font-weight: 600">
                {{ formatUptime(host?.uptimeSeconds || 0) }}
                <span class="text-muted" style="font-weight: 400; font-size: 11px">
                  ({{ host?.uptimeSeconds?.toLocaleString() || 0 }} seconds)
                </span>
              </span>
            </div>
          </div>
        </div>

        <!-- 2. Processor (CPU) -->
        <div class="details-section">
          <div class="section-title">
            <MdiIcon :path="mdiCpu64Bit" size="16" class="section-icon" />
            <span>Processor (CPU)</span>
          </div>
          <div class="details-card">
            <div class="spec-row">
              <span class="spec-label">Processor:</span>
              <span class="spec-val font-semibold">{{ cpu?.model || 'Detecting CPU Model...' }}</span>
            </div>
            <div class="spec-row">
              <span class="spec-label">Cores & Threads:</span>
              <span class="spec-val mono">
                {{ cpu?.physicalCores || 0 }} Physical Cores · {{ cpu?.logicalCores || 0 }} Logical Threads
              </span>
            </div>
            <div class="spec-row">
              <span class="spec-label">Load Averages:</span>
              <span class="spec-val mono">
                {{ cpu?.load1?.toFixed(2) || '0.00' }} (1m) · {{ cpu?.load5?.toFixed(2) || '0.00' }} (5m) · {{ cpu?.load15?.toFixed(2) || '0.00' }} (15m)
              </span>
            </div>
            <div class="spec-row">
              <span class="spec-label">Usage & Thermal:</span>
              <span class="spec-val" style="display: inline-flex; align-items: center; gap: 8px; flex-wrap: wrap">
                <span class="mono font-semibold">{{ (cpu?.usagePercent || 0).toFixed(1) }}% Active</span>
                <template v-if="(cpu?.packageTemperatures || []).length > 1">
                  <span
                    v-for="(t, idx) in cpu?.packageTemperatures"
                    :key="idx"
                    class="badge"
                    :class="getTempBadgeClass(t)"
                    style="font-size: 11px"
                    :title="`CPU Socket ${idx} Package Temperature`"
                  >
                    <MdiIcon :path="mdiThermometer" size="12" style="margin-right: 2px" />
                    CPU{{ idx }}: {{ t.toFixed(1) }} °C
                  </span>
                </template>
                <span
                  v-else-if="(cpu?.temperatureCelsius || 0) > 0"
                  class="badge"
                  :class="getTempBadgeClass(cpu?.temperatureCelsius || 0)"
                  style="font-size: 11px"
                >
                  <MdiIcon :path="mdiThermometer" size="12" style="margin-right: 2px" />
                  {{ cpu?.temperatureCelsius.toFixed(1) }} °C
                </span>
              </span>
            </div>
          </div>
        </div>

        <!-- 3. Memory & ZFS ARC -->
        <div class="details-section">
          <div class="section-title">
            <MdiIcon :path="mdiMemory" size="16" class="section-icon" />
            <span>Memory (RAM) & Caching</span>
          </div>
          <div class="details-card">
            <div class="spec-row">
              <span class="spec-label">Total Installed:</span>
              <span class="spec-val mono font-semibold">{{ formatBytes(mem?.totalBytes || 0) }}</span>
            </div>
            <div class="spec-row">
              <span class="spec-label">Allocated (Used):</span>
              <span class="spec-val mono">
                {{ formatBytes(mem?.usedBytes || 0) }}
                <span class="text-muted" style="font-size: 11px">
                  ({{ mem?.totalBytes ? Math.round(((mem.usedBytes || 0) / mem.totalBytes) * 100) : 0 }}% of total)
                </span>
              </span>
            </div>
            <div class="spec-row">
              <span class="spec-label">Services & Apps:</span>
              <span class="spec-val mono">{{ formatBytes(mem?.servicesBytes || ((mem?.usedBytes || 0) - (mem?.zfsArcBytes || 0))) }}</span>
            </div>
            <div class="spec-row">
              <span class="spec-label">ZFS ARC Cache:</span>
              <span class="spec-val mono">
                {{ formatBytes(mem?.zfsArcBytes || 0) }}
                <span v-if="mem?.zfsArcMaxBytes" class="text-muted" style="font-size: 11px">
                  (Max limit: {{ formatBytes(mem.zfsArcMaxBytes) }})
                </span>
              </span>
            </div>
            <div class="spec-row">
              <span class="spec-label">Free Available:</span>
              <span class="spec-val mono text-success" style="color: var(--success); font-weight: 600">
                {{ formatBytes(mem?.freeBytes || 0) }}
              </span>
            </div>
          </div>
        </div>

        <!-- 4. Operating System & Kernel -->
        <div class="details-section">
          <div class="section-title">
            <MdiIcon :path="mdiLinux" size="16" class="section-icon" />
            <span>Operating System & Kernel</span>
          </div>
          <div class="details-card">
            <div class="spec-row">
              <span class="spec-label">Distribution:</span>
              <span class="spec-val font-semibold">{{ host?.osName || 'Linux' }} {{ host?.osVersion || '' }}</span>
            </div>
            <div class="spec-row">
              <span class="spec-label">Kernel Version:</span>
              <span class="spec-val mono">{{ host?.kernelVersion || 'N/A' }}</span>
            </div>
            <div class="spec-row">
              <span class="spec-label">Architecture:</span>
              <span class="spec-val mono">{{ host?.architecture || 'x86_64' }}</span>
            </div>
            <div class="spec-row">
              <span class="spec-label">Active Modules:</span>
              <span class="spec-val" style="display: flex; gap: 4px; flex-wrap: wrap">
                <span
                  v-for="mod in ['ZFS', 'KVM', 'Docker', 'LXC', 'CIFS', 'WebDAV']"
                  :key="mod"
                  class="badge badge-neutral"
                  style="font-size: 10.5px; padding: 1px 6px"
                >
                  {{ mod }}
                </span>
              </span>
            </div>
          </div>
        </div>

        <!-- 5. Primary Network Interface -->
        <div class="details-section" v-if="primaryInterface">
          <div class="section-title">
            <MdiIcon :path="mdiLanConnect" size="16" class="section-icon" />
            <span>Primary Network Interface ({{ primaryInterface.name }})</span>
          </div>
          <div class="details-card">
            <div class="spec-row">
              <span class="spec-label">Interface:</span>
              <span class="spec-val mono font-semibold" style="display: inline-flex; align-items: center; gap: 6px">
                {{ primaryInterface.name }}
                <span class="badge" :class="primaryInterface.isUp ? 'badge-success' : 'badge-neutral'" style="font-size: 10px; padding: 1px 5px">
                  {{ primaryInterface.isUp ? 'ONLINE' : 'DOWN' }}
                </span>
              </span>
            </div>
            <div class="spec-row">
              <span class="spec-label">Link Speed:</span>
              <span class="spec-val mono">
                {{ primaryInterface.speedMbps ? `${primaryInterface.speedMbps} Mbps` : 'Auto-negotiated' }}
              </span>
            </div>
            <div class="spec-row">
              <span class="spec-label">IP Addresses:</span>
              <span class="spec-val mono" style="display: flex; gap: 4px; flex-wrap: wrap">
                <span
                  v-for="ip in primaryInterface.ipAddresses"
                  :key="ip"
                  class="ip-chip"
                >
                  {{ ip }}
                </span>
              </span>
            </div>
            <div class="spec-row">
              <span class="spec-label">MAC Address:</span>
              <span class="spec-val mono">{{ primaryInterface.macAddress || 'N/A' }}</span>
            </div>
            <div class="spec-row">
              <span class="spec-label">Transfer Rates:</span>
              <span class="spec-val mono" style="font-size: 11.5px">
                RX: <strong style="color: var(--success)">{{ formatBytes(primaryInterface.rxRateBytesPerSec) }}/s</strong>
                · TX: <strong style="color: var(--accent-primary)">{{ formatBytes(primaryInterface.txRateBytesPerSec) }}/s</strong>
              </span>
            </div>
          </div>
        </div>

        <!-- 6. Storage & Graphics Accelerators -->
        <div class="details-section">
          <div class="section-title">
            <MdiIcon :path="mdiHarddisk" size="16" class="section-icon" />
            <span>Storage & Accelerators</span>
          </div>
          <div class="details-card">
            <div class="spec-row">
              <span class="spec-label">Storage Pools:</span>
              <span class="spec-val mono">
                {{ nasState.pools.length }} Active Pool(s) ({{ formatBytes(usedStorageBytes) }} used / {{ formatBytes(totalStorageBytes) }} total)
              </span>
            </div>
            <div class="spec-row" v-if="host?.gpus && host.gpus.length > 0">
              <span class="spec-label">Graphics (GPU):</span>
              <div class="spec-val">
                <div v-for="(gpu, idx) in host.gpus" :key="idx" class="mono" style="font-size: 12px; margin-bottom: 2px">
                  {{ gpu }}
                </div>
              </div>
            </div>
            <div class="spec-row" v-else>
              <span class="spec-label">Graphics:</span>
              <span class="spec-val text-muted">Integrated / Headless</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.system-modal {
  max-width: 840px;
  width: 95vw;
  max-height: 88vh;
  display: flex;
  flex-direction: column;
}

.system-icon-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  border-radius: 10px;
  background: var(--bg-surface-elevated, var(--bg-surface));
  border: 1px solid var(--border-subtle);
  color: var(--accent-primary);
  flex-shrink: 0;
}

.system-details-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 16px;
  padding: 16px 20px;
}

.details-section {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--text-secondary);
}

.section-icon {
  color: var(--accent-primary);
}

.details-card {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.spec-row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  font-size: 12.5px;
  line-height: 1.45;
}

.spec-label {
  width: 115px;
  flex-shrink: 0;
  font-weight: 600;
  color: var(--text-secondary);
}

.spec-val {
  color: var(--text-primary);
  word-break: break-word;
  flex: 1;
}

.ip-chip {
  display: inline-block;
  background: var(--bg-surface-elevated, rgba(255, 255, 255, 0.05));
  border: 1px solid var(--border-subtle);
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 11px;
}

.spin-icon {
  animation: spin 0.85s linear infinite;
  display: inline-block;
}
</style>
