<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type * as echarts from 'echarts';
import {
  mdiVideo3dVariant,
  mdiThermometer,
  mdiFlash,
  mdiFan,
  mdiSpeedometer,
  mdiMemory,
} from '@mdi/js';
import { formatBytes, type GpuTelemetry } from '../../../domain/models';
import { nasState, type GpuHistoryPoint } from '../../store';
import { useChartPalette } from '../chartPalette';
import EChart from '../../components/EChart.vue';
import MdiIcon from '../../components/MdiIcon.vue';

const props = defineProps<{
  gpuHistory?: GpuHistoryPoint[];
}>();

const telemetry = computed(() => nasState.systemTelemetry);
const gpus = computed<GpuTelemetry[]>(() => telemetry.value?.sensors?.gpus || []);
const pciGpus = computed<string[]>(() => telemetry.value?.host?.gpus || []);
const chartPalette = useChartPalette();

const selectedGpuIndex = ref<number>(0);

watch(
  gpus,
  (list) => {
    if (list.length > 0 && !list.some((g) => g.index === selectedGpuIndex.value)) {
      selectedGpuIndex.value = list[0].index;
    }
  },
  { immediate: true }
);

const activeGpu = computed<GpuTelemetry | null>(() => {
  if (gpus.value.length === 0) return null;
  return gpus.value.find((g) => g.index === selectedGpuIndex.value) || gpus.value[0];
});

const vramPercent = computed(() => {
  const g = activeGpu.value;
  if (!g) return 0;
  if (g.memoryTotalBytes > 0) {
    return Math.min(100, Math.max(0, (g.memoryUsedBytes / g.memoryTotalBytes) * 100));
  }
  return Math.min(100, Math.max(0, g.memoryUtilizationPercent || 0));
});

const powerPercent = computed(() => {
  const g = activeGpu.value;
  if (!g || !g.powerCapWatts || g.powerCapWatts <= 0) return 0;
  return Math.min(100, Math.max(0, (g.powerWatts / g.powerCapWatts) * 100));
});

const historyPoints = computed(() => {
  const src = props.gpuHistory && props.gpuHistory.length > 0 ? props.gpuHistory : nasState.gpuHistory || [];
  if (src.length > 0) return src;
  const g = activeGpu.value;
  if (!g) return [];
  return [
    {
      time: 'now',
      byGpu: {
        [g.index]: {
          util: g.utilizationPercent || 0,
          memPct: parseFloat(vramPercent.value.toFixed(1)),
          temp: g.temperatureCelsius || 0,
          power: g.powerWatts || 0,
          enc: g.encoderUtilizationPercent || 0,
          dec: g.decoderUtilizationPercent || 0,
        },
      },
    },
  ];
});

const nvtopChartOptions = computed<echarts.EChartsOption>(() => {
  const p = chartPalette.value;
  const g = activeGpu.value;
  const idx = g?.index ?? 0;
  const hist = historyPoints.value;
  const times = hist.map((pt) => pt.time);
  const utilSeries = hist.map((pt) => pt.byGpu[idx]?.util ?? 0);
  const memSeries = hist.map((pt) => pt.byGpu[idx]?.memPct ?? 0);
  const tempSeries = hist.map((pt) => pt.byGpu[idx]?.temp ?? 0);

  return {
    backgroundColor: 'transparent',
    animation: true,
    animationDuration: 400,
    animationDurationUpdate: 400,
    tooltip: {
      trigger: 'axis',
      backgroundColor: p.tooltipBg,
      borderColor: p.tooltipBorder,
      textStyle: { color: p.textColor, fontSize: 11.5 },
      axisPointer: {
        type: 'cross',
        lineStyle: { color: p.subtextColor, width: 1, type: 'dashed' },
      },
      formatter: (params: any) => {
        if (!Array.isArray(params) || params.length === 0) return '';
        let html = `<strong>${params[0].axisValue}</strong><br/>`;
        for (const item of params) {
          const unit = item.seriesName === 'Temp' ? '°C' : '%';
          html += `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${item.color};margin-right:6px"></span>`;
          html += `${item.seriesName}: <strong>${Number(item.value).toFixed(1)}${unit}</strong><br/>`;
        }
        return html;
      },
    },
    legend: {
      data: ['GPU %', 'VRAM %', 'Temp'],
      top: 0,
      right: 4,
      icon: 'roundRect',
      itemWidth: 10,
      itemHeight: 6,
      textStyle: { color: p.subtextColor, fontSize: 10.5 },
    },
    grid: {
      top: 24,
      right: 36,
      bottom: 20,
      left: 36,
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: times,
      axisLine: { lineStyle: { color: p.gridBorder } },
      axisLabel: { color: p.subtextColor, fontSize: 10 },
      axisTick: { show: false },
    },
    yAxis: [
      {
        type: 'value',
        min: 0,
        max: 100,
        splitNumber: 4,
        axisLabel: {
          color: p.subtextColor,
          fontSize: 10,
          formatter: '{value}%',
        },
        splitLine: { lineStyle: { color: p.gridBorder, type: 'dashed' } },
      },
      {
        type: 'value',
        min: 0,
        max: 100,
        splitNumber: 4,
        axisLabel: {
          color: p.subtextColor,
          fontSize: 10,
          formatter: '{value}°',
        },
        splitLine: { show: false },
      },
    ],
    series: [
      {
        name: 'GPU %',
        type: 'line',
        yAxisIndex: 0,
        data: utilSeries,
        smooth: 0.25,
        showSymbol: false,
        lineStyle: { width: 2, color: '#22c55e' },
        itemStyle: { color: '#22c55e' },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(34, 197, 94, 0.28)' },
              { offset: 1, color: 'rgba(34, 197, 94, 0.02)' },
            ],
          },
        },
      },
      {
        name: 'VRAM %',
        type: 'line',
        yAxisIndex: 0,
        data: memSeries,
        smooth: 0.25,
        showSymbol: false,
        lineStyle: { width: 2, color: '#38bdf8' },
        itemStyle: { color: '#38bdf8' },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(56, 189, 248, 0.22)' },
              { offset: 1, color: 'rgba(56, 189, 248, 0.02)' },
            ],
          },
        },
      },
      {
        name: 'Temp',
        type: 'line',
        yAxisIndex: 1,
        data: tempSeries,
        smooth: 0.25,
        showSymbol: false,
        lineStyle: { width: 1.5, type: 'dashed', color: '#f59e0b' },
        itemStyle: { color: '#f59e0b' },
      },
    ],
  };
});

function getTempBadgeClass(temp: number): string {
  if (temp <= 0) return 'badge-neutral';
  if (temp < 60) return 'badge-success';
  if (temp < 80) return 'badge-warning';
  return 'badge-danger';
}

function getUtilColor(pct: number): string {
  if (pct >= 90) return 'var(--danger)';
  if (pct >= 70) return 'var(--warning)';
  return '#22c55e';
}

function formatProcessShortName(cmd: string): string {
  if (!cmd) return 'unknown';
  const clean = cmd.trim().split(/\s+/)[0];
  const parts = clean.split('/');
  return parts[parts.length - 1] || clean;
}
</script>

<template>
  <div class="card nvtop-widget-card">
    <!-- Top Header -->
    <div class="card-header nvtop-header">
      <div class="nvtop-title-group">
        <MdiIcon :path="mdiVideo3dVariant" size="18" style="color: #22c55e" />
        <h3 class="card-title">GPU Monitor</h3>
        <span class="nvtop-pill mono">nvtop</span>
      </div>

      <!-- GPU Tabs when multiple GPUs present -->
      <div v-if="gpus.length > 1" class="gpu-tabs">
        <button
          v-for="g in gpus"
          :key="g.index"
          type="button"
          class="gpu-tab-btn mono"
          :class="{ active: activeGpu && activeGpu.index === g.index }"
          @click="selectedGpuIndex = g.index"
        >
          GPU {{ g.index }}
        </button>
      </div>
      <div v-else-if="activeGpu" class="nvtop-meta-pills">
        <span class="badge badge-info mono">{{ activeGpu.vendor }}</span>
        <span v-if="activeGpu.driverVersion" class="badge badge-neutral mono">
          v{{ activeGpu.driverVersion }}
        </span>
      </div>
    </div>

    <!-- Active GPU Content -->
    <template v-if="activeGpu">
      <!-- Device Strip -->
      <div class="nvtop-device-strip">
        <div class="device-identity">
          <span class="device-idx mono">GPU {{ activeGpu.index }}</span>
          <strong class="device-name" :title="activeGpu.name">{{ activeGpu.name }}</strong>
          <span v-if="activeGpu.pciBusId" class="device-pci mono">{{ activeGpu.pciBusId }}</span>
        </div>

        <div class="device-stat-chips">
          <span
            v-if="activeGpu.temperatureCelsius > 0"
            class="badge mono stat-chip"
            :class="getTempBadgeClass(activeGpu.temperatureCelsius)"
            title="GPU Temperature"
          >
            <MdiIcon :path="mdiThermometer" size="12" />
            {{ activeGpu.temperatureCelsius.toFixed(0) }}°C
          </span>
          <span
            v-if="activeGpu.powerWatts > 0"
            class="badge badge-warning mono stat-chip"
            title="Power Draw / Limit"
          >
            <MdiIcon :path="mdiFlash" size="12" />
            {{ activeGpu.powerWatts.toFixed(1) }}W<template v-if="activeGpu.powerCapWatts && activeGpu.powerCapWatts > 0">/{{ activeGpu.powerCapWatts.toFixed(0) }}W</template>
          </span>
          <span
            v-if="activeGpu.fanSpeedPercent !== undefined && activeGpu.fanSpeedPercent > 0"
            class="badge badge-neutral mono stat-chip"
            title="Fan Speed"
          >
            <MdiIcon :path="mdiFan" size="12" />
            {{ activeGpu.fanSpeedPercent }}%
          </span>
          <span
            v-if="activeGpu.clockGraphicsMhz && activeGpu.clockGraphicsMhz > 0"
            class="badge badge-neutral mono stat-chip clock-chip"
            title="Graphics / Memory Clock"
          >
            <MdiIcon :path="mdiSpeedometer" size="12" />
            {{ activeGpu.clockGraphicsMhz }}MHz<template v-if="activeGpu.clockMemoryMhz && activeGpu.clockMemoryMhz > 0"> / {{ activeGpu.clockMemoryMhz }}MHz</template>
          </span>
        </div>
      </div>

      <!-- NVTOP Bar Meters -->
      <div class="nvtop-bars-grid">
        <div class="nvtop-bar-row">
          <span class="bar-label mono">GPU</span>
          <div class="nvtop-track">
            <div
              class="nvtop-fill"
              :style="{
                width: `${Math.min(100, Math.max(0, activeGpu.utilizationPercent || 0))}%`,
                background: getUtilColor(activeGpu.utilizationPercent || 0),
              }"
            ></div>
          </div>
          <span class="bar-val mono">{{ (activeGpu.utilizationPercent || 0).toFixed(0) }}%</span>
        </div>

        <div class="nvtop-bar-row">
          <span class="bar-label mono">MEM</span>
          <div class="nvtop-track">
            <div
              class="nvtop-fill"
              :style="{
                width: `${vramPercent}%`,
                background: '#38bdf8',
              }"
            ></div>
          </div>
          <span class="bar-val mono" :title="`${vramPercent.toFixed(1)}%`">
            {{ formatBytes(activeGpu.memoryUsedBytes) }} / {{ formatBytes(activeGpu.memoryTotalBytes) }}
          </span>
        </div>

        <div v-if="activeGpu.powerCapWatts && activeGpu.powerCapWatts > 0" class="nvtop-bar-row nvtop-bar-extra">
          <span class="bar-label mono">PWR</span>
          <div class="nvtop-track">
            <div
              class="nvtop-fill"
              :style="{
                width: `${powerPercent}%`,
                background: '#f59e0b',
              }"
            ></div>
          </div>
          <span class="bar-val mono">{{ activeGpu.powerWatts.toFixed(0) }}W / {{ activeGpu.powerCapWatts.toFixed(0) }}W</span>
        </div>

        <div
          v-if="(activeGpu.encoderUtilizationPercent || 0) > 0 || (activeGpu.decoderUtilizationPercent || 0) > 0"
          class="nvtop-bar-row nvtop-bar-extra"
        >
          <span class="bar-label mono">E/D</span>
          <div class="nvtop-track">
            <div
              class="nvtop-fill"
              :style="{
                width: `${Math.min(100, Math.max(activeGpu.encoderUtilizationPercent || 0, activeGpu.decoderUtilizationPercent || 0))}%`,
                background: '#a855f7',
              }"
            ></div>
          </div>
          <span class="bar-val mono">
            {{ (activeGpu.encoderUtilizationPercent || 0).toFixed(0) }}% / {{ (activeGpu.decoderUtilizationPercent || 0).toFixed(0) }}%
          </span>
        </div>
      </div>

      <!-- Rolling NVTOP Graph (Flexes to fill available height!) -->
      <div class="nvtop-chart-area">
        <EChart :options="nvtopChartOptions" height="100%" :not-merge="false" />
      </div>

      <!-- Active GPU Processes Table -->
      <div class="nvtop-procs-section">
        <div v-if="activeGpu.processes && activeGpu.processes.length > 0" class="nvtop-procs-table-wrap">
          <table class="nvtop-procs-table mono">
            <thead>
              <tr>
                <th>PID</th>
                <th>TYPE</th>
                <th style="text-align: right">GPU MEM</th>
                <th>COMMAND</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="proc in activeGpu.processes" :key="`${proc.pid}-${proc.name}`">
                <td>{{ proc.pid }}</td>
                <td>
                  <span class="proc-type-pill">{{ proc.processType || 'C' }}</span>
                </td>
                <td style="text-align: right">{{ formatBytes(proc.memoryUsedBytes) }}</td>
                <td class="proc-cmd" :title="proc.name">{{ formatProcessShortName(proc.name) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="nvtop-procs-empty mono">
          <MdiIcon :path="mdiMemory" size="13" />
          <span>No active GPU compute/graphics processes</span>
        </div>
      </div>
    </template>

    <!-- Empty State when no live GPU telemetry -->
    <div v-else class="nvtop-empty-state">
      <MdiIcon :path="mdiVideo3dVariant" size="32" style="color: var(--text-muted); opacity: 0.6" />
      <div class="empty-title">No Live GPU Telemetry Detected</div>
      <div v-if="pciGpus.length > 0" class="pci-gpu-list">
        <div v-for="(g, i) in pciGpus" :key="i" class="pci-gpu-item mono">{{ g }}</div>
      </div>
      <div class="empty-hint">
        Install NVIDIA drivers (<code>nvidia-smi</code>) or AMD <code>amdgpu</code> kernel module for live nvtop metrics.
      </div>
    </div>
  </div>
</template>

<style scoped>
.nvtop-widget-card {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  gap: 8px;
  overflow: hidden;
}

.nvtop-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  border-bottom: 1px solid var(--border-subtle);
  padding-bottom: 8px;
  margin-bottom: 0;
  flex-shrink: 0;
  flex-wrap: wrap;
}

.nvtop-title-group {
  display: flex;
  align-items: center;
  gap: 7px;
}

.nvtop-pill {
  font-size: 10px;
  font-weight: 700;
  padding: 1px 6px;
  border-radius: 4px;
  background: rgba(34, 197, 94, 0.14);
  color: #22c55e;
  border: 1px solid rgba(34, 197, 94, 0.3);
  letter-spacing: 0.04em;
}

.gpu-tabs {
  display: flex;
  gap: 4px;
}

.gpu-tab-btn {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
  font-size: 11px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 6px;
  cursor: pointer;
}

.gpu-tab-btn.active {
  background: rgba(34, 197, 94, 0.16);
  border-color: #22c55e;
  color: #22c55e;
}

.nvtop-meta-pills {
  display: flex;
  gap: 5px;
  align-items: center;
}

.nvtop-device-strip {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  flex-shrink: 0;
}

.device-identity {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.device-idx {
  font-size: 10.5px;
  font-weight: 700;
  color: var(--text-muted);
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  padding: 1px 5px;
  border-radius: 4px;
  flex-shrink: 0;
}

.device-name {
  font-size: 12.5px;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.device-pci {
  font-size: 10.5px;
  color: var(--text-muted);
}

.device-stat-chips {
  display: flex;
  align-items: center;
  gap: 5px;
  flex-wrap: wrap;
}

.stat-chip {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 10.5px;
  padding: 2px 6px;
}

.nvtop-bars-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px 14px;
  padding: 8px 10px;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  flex-shrink: 0;
}

.nvtop-bar-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.bar-label {
  font-size: 10.5px;
  font-weight: 700;
  color: var(--text-secondary);
  width: 28px;
  flex-shrink: 0;
}

.nvtop-track {
  flex: 1;
  height: 8px;
  background: rgba(148, 163, 184, 0.16);
  border-radius: 4px;
  overflow: hidden;
  min-width: 40px;
}

.nvtop-fill {
  height: 100%;
  border-radius: 4px;
  transition: width 0.35s ease;
}

.bar-val {
  font-size: 11px;
  color: var(--text-primary);
  white-space: nowrap;
  flex-shrink: 0;
}

.nvtop-chart-area {
  flex: 1 1 0;
  min-height: 110px;
  width: 100%;
  position: relative;
}

.nvtop-procs-section {
  flex-shrink: 0;
  border-top: 1px solid var(--border-subtle);
  padding-top: 6px;
}

.nvtop-procs-table-wrap {
  max-height: 110px;
  overflow-y: auto;
}

.nvtop-procs-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 11px;
}

.nvtop-procs-table th {
  text-align: left;
  font-size: 10px;
  color: var(--text-muted);
  font-weight: 600;
  padding: 2px 6px 4px;
  border-bottom: 1px solid var(--border-subtle);
}

.nvtop-procs-table td {
  padding: 3px 6px;
  color: var(--text-primary);
  border-bottom: 1px solid rgba(148, 163, 184, 0.08);
  white-space: nowrap;
}

.proc-cmd {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.proc-type-pill {
  font-size: 9.5px;
  font-weight: 700;
  padding: 0 5px;
  border-radius: 3px;
  background: rgba(56, 189, 248, 0.15);
  color: #38bdf8;
}

.nvtop-procs-empty {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: var(--text-muted);
}

.nvtop-empty-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  gap: 8px;
  padding: 16px;
}

.empty-title {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--text-primary);
}

.pci-gpu-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 100%;
  max-width: 420px;
}

.pci-gpu-item {
  font-size: 11px;
  padding: 4px 8px;
  border-radius: 6px;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
}

.empty-hint {
  font-size: 11.5px;
  color: var(--text-muted);
  max-width: 380px;
}

/* Container-responsive scaling for Grafana fluid grid */
@container (max-width: 460px) {
  .nvtop-bars-grid {
    grid-template-columns: 1fr;
  }
  .device-pci,
  .clock-chip {
    display: none;
  }
}
</style>
