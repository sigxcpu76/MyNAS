<script setup lang="ts">
import { computed } from 'vue';
import { formatBytes, formatUptime } from '../../../domain/models';
import { nasState } from '../../store';

const telemetry = computed(() => nasState.systemTelemetry);
</script>

<template>
  <div class="card fastfetch-card">
    <div class="card-header" style="border-bottom: 1px solid var(--border-subtle); padding-bottom: 8px; margin-bottom: 12px">
      <h3 class="card-title">System</h3>
      <span class="badge badge-neutral mono">{{ telemetry?.host?.architecture || 'x86_64' }}</span>
    </div>

    <div class="fastfetch-layout">
      <!-- Key-Value Rows -->
      <div class="fastfetch-info">
        <div class="fetch-row">
          <span class="fetch-key">Host:</span>
          <span class="fetch-val mono" style="font-weight: 600; color: var(--accent-primary)">
            {{ telemetry?.host?.hostname || 'nan-nas' }}
          </span>
        </div>

        <div class="fetch-row">
          <span class="fetch-key">OS:</span>
          <span class="fetch-val">{{ telemetry?.host?.osName || 'Linux' }} {{ telemetry?.host?.osVersion || '' }}</span>
        </div>

        <div class="fetch-row">
          <span class="fetch-key">Kernel:</span>
          <span class="fetch-val mono">{{ telemetry?.host?.kernelVersion || 'Linux' }} ({{ telemetry?.host?.architecture || 'x86_64' }})</span>
        </div>

        <div class="fetch-row">
          <span class="fetch-key">Uptime:</span>
          <span class="fetch-val mono">{{ formatUptime(telemetry?.host?.uptimeSeconds || 0) }}</span>
        </div>

        <div class="fetch-row">
          <span class="fetch-key">Motherboard:</span>
          <span class="fetch-val">
            {{ telemetry?.host?.motherboardVendor || '' }} {{ telemetry?.host?.motherboardName || 'Custom Platform' }}
            <span v-if="telemetry?.host?.motherboardVersion" class="mono" style="font-size: 11px; color: var(--text-secondary)">
              ({{ telemetry?.host?.motherboardVersion }})
            </span>
          </span>
        </div>

        <div class="fetch-row">
          <span class="fetch-key">BIOS:</span>
          <span class="fetch-val mono">
            {{ telemetry?.host?.biosVendor || 'UEFI' }} {{ telemetry?.host?.biosVersion || '' }}
            <span v-if="telemetry?.host?.biosDate" style="color: var(--text-secondary); font-size: 11px">
              ({{ telemetry?.host?.biosDate }})
            </span>
          </span>
        </div>

        <div class="fetch-row">
          <span class="fetch-key">CPU:</span>
          <span class="fetch-val">{{ telemetry?.cpu?.model || 'Unknown Processor' }}</span>
        </div>

        <div class="fetch-row" v-if="telemetry?.host?.gpus && telemetry.host.gpus.length > 0">
          <span class="fetch-key">GPU:</span>
          <div class="fetch-val">
            <div v-for="(gpu, idx) in telemetry.host.gpus" :key="idx" style="margin-bottom: 2px">
              {{ gpu }}
            </div>
          </div>
        </div>

        <div class="fetch-row">
          <span class="fetch-key">Memory:</span>
          <span class="fetch-val mono">
            {{ formatBytes(telemetry?.memory?.usedBytes || 0) }} / {{ formatBytes(telemetry?.memory?.totalBytes || 0) }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.fastfetch-card {
  font-family: inherit;
  height: 100%;
}

.fastfetch-layout {
  display: flex;
  flex-direction: column;
}

.fastfetch-info {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.fetch-row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  font-size: 13px;
  line-height: 1.4;
}

.fetch-key {
  width: 95px;
  flex-shrink: 0;
  font-weight: 600;
  color: var(--accent-primary);
  text-align: right;
}

.fetch-val {
  color: var(--text-primary);
  word-break: break-word;
}
</style>
