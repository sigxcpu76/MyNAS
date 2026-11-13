<script setup lang="ts">
import { computed, ref } from 'vue';
import type * as echarts from 'echarts';
import { mdiChevronDown, mdiChevronUp } from '@mdi/js';
import { formatBytes, type DiskIoTelemetry } from '../../../domain/models';
import { nasState, type IoRatePoint } from '../../store';
import { useChartPalette } from '../chartPalette';
import EChart from '../../components/EChart.vue';
import MdiIcon from '../../components/MdiIcon.vue';

const props = defineProps<{
  ioHistory?: IoRatePoint[];
}>();

const selectedDisk = ref<string>('__all__');
const showDevices = ref(false);
const telemetry = computed(() => nasState.systemTelemetry);
const chartPalette = useChartPalette();

const disks = computed<DiskIoTelemetry[]>(() => telemetry.value?.diskIo || []);

const activeStats = computed(() => {
  const list = disks.value;
  if (list.length === 0) return null;

  if (selectedDisk.value !== '__all__') {
    const found = list.find((d) => d.name === selectedDisk.value);
    if (found) {
      return {
        label: found.name,
        readRate: found.readRateBytesPerSec,
        writeRate: found.writeRateBytesPerSec,
        readIops: found.readIops,
        writeIops: found.writeIops,
        readBytes: found.readBytes,
        writeBytes: found.writeBytes,
        ioInProgress: found.ioInProgress,
        utilPercent: found.utilizationPercent,
      };
    }
  }

  let readRate = 0;
  let writeRate = 0;
  let readIops = 0;
  let writeIops = 0;
  let readBytes = 0;
  let writeBytes = 0;
  let ioInProgress = 0;
  let maxUtil = 0;

  for (const d of list) {
    readRate += d.readRateBytesPerSec || 0;
    writeRate += d.writeRateBytesPerSec || 0;
    readIops += d.readIops || 0;
    writeIops += d.writeIops || 0;
    readBytes += d.readBytes || 0;
    writeBytes += d.writeBytes || 0;
    ioInProgress += d.ioInProgress || 0;
    if ((d.utilizationPercent || 0) > maxUtil) {
      maxUtil = d.utilizationPercent || 0;
    }
  }

  return {
    label: `All Disks (${list.length})`,
    readRate,
    writeRate,
    readIops,
    writeIops,
    readBytes,
    writeBytes,
    ioInProgress,
    utilPercent: maxUtil,
  };
});

const historyPoints = computed(() => {
  if (props.ioHistory && props.ioHistory.length > 0) {
    return props.ioHistory;
  }
  return nasState.diskIoHistory || [];
});

const diskIoChartOptions = computed<echarts.EChartsOption>(() => {
  const p = chartPalette.value;
  const history = historyPoints.value;
  const times = history.map((pt) => pt.time);
  const key = selectedDisk.value;
  const readRates = history.map((pt) =>
    key === '__all__' ? pt.readRate : (pt.byKey?.[key]?.readRate ?? 0)
  );
  const writeRates = history.map((pt) =>
    key === '__all__' ? pt.writeRate : (pt.byKey?.[key]?.writeRate ?? 0)
  );

  const maxVal = Math.max(...readRates, ...writeRates, 0);
  const baselineMax = maxVal < 65536 ? 65536 : undefined;

  return {
    backgroundColor: 'transparent',
    animation: true,
    animationDuration: 1000,
    animationDurationUpdate: 1000,
    animationEasing: 'linear',
    animationEasingUpdate: 'linear',
    tooltip: {
      trigger: 'axis',
      backgroundColor: p.tooltipBg,
      borderColor: p.tooltipBorder,
      textStyle: { color: p.textColor, fontSize: 12 },
      axisPointer: {
        type: 'cross',
        lineStyle: { color: p.subtextColor, width: 1, type: 'dashed' },
      },
      formatter: (params: any) => {
        if (!Array.isArray(params) || params.length === 0) return '';
        const time = params[0].axisValue;
        let html = `<strong>${time}</strong><br/>`;
        for (const item of params) {
          html += `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${item.color};margin-right:6px"></span>`;
          html += `${item.seriesName}: <strong>${formatBytes(item.value)}/s</strong><br/>`;
        }
        return html;
      },
    },
    legend: {
      data: ['Read', 'Write'],
      top: 0,
      right: 0,
      icon: 'roundRect',
      itemWidth: 12,
      itemHeight: 8,
      textStyle: { color: p.textColor, fontSize: 11 },
    },
    grid: { top: 26, right: 12, bottom: 22, left: 60 },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: times,
      axisLine: { lineStyle: { color: p.borderColor } },
      axisLabel: {
        color: p.subtextColor,
        fontSize: 10,
        showMaxLabel: true,
        showMinLabel: true,
        interval: (index: number) => index % 6 === 0 || index === times.length - 1,
      },
      splitLine: { show: false },
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: baselineMax,
      splitLine: { lineStyle: { color: p.gridBorder, type: 'dashed' } },
      axisLabel: {
        color: p.subtextColor,
        fontSize: 10,
        formatter: (val: number) => `${formatBytes(val)}/s`,
      },
    },
    series: [
      {
        name: 'Read',
        type: 'line',
        smooth: 0.25,
        showSymbol: false,
        lineStyle: { width: 2, color: p.netIn },
        itemStyle: { color: p.netIn },
        areaStyle: { color: p.netInArea },
        data: readRates,
      },
      {
        name: 'Write',
        type: 'line',
        smooth: 0.25,
        showSymbol: false,
        lineStyle: { width: 2, color: p.netOut },
        itemStyle: { color: p.netOut },
        areaStyle: { color: p.netOutArea },
        data: writeRates,
      },
    ],
  };
});
</script>

<template>
  <div class="card disk-io-card">
    <div class="card-header" style="border-bottom: 1px solid var(--border-subtle); padding-bottom: 8px; margin-bottom: 12px">
      <div style="display: flex; align-items: center; gap: 8px">
        <h3 class="card-title">Disk I/O</h3>
        <span v-if="disks.length > 0" class="badge badge-neutral mono">{{ disks.length }} Devices</span>
      </div>
      <select
        v-if="disks.length > 0"
        v-model="selectedDisk"
        class="form-control mono disk-select"
        aria-label="Filter disk I/O device"
      >
        <option value="__all__">All Disks</option>
        <option v-for="d in disks" :key="d.name" :value="d.name">{{ d.name }}</option>
      </select>
    </div>

    <div v-if="!activeStats" class="card-subtitle" style="padding: 24px 0; text-align: center">
      Reading /proc/diskstats telemetry...
    </div>

    <div v-else style="display: flex; flex-direction: column; gap: 12px">
      <div class="io-summary-header">
        <div class="io-summary-top">
          <strong class="mono" style="font-size: 14px; color: var(--accent-primary)">
            {{ activeStats.label }}
          </strong>
          <span class="mono" style="font-size: 11px; color: var(--text-secondary)">
            Q: {{ activeStats.ioInProgress }} · Util: {{ activeStats.utilPercent.toFixed(1) }}%
          </span>
        </div>

        <div class="system-throughput-pills mono">
          <div class="throughput-pill">
            <span class="pill-label">Read</span>
            <strong class="pill-rate" style="color: var(--success)">
              {{ formatBytes(activeStats.readRate) }}/s
            </strong>
            <span class="pill-total">{{ activeStats.readIops }} IOPS · {{ formatBytes(activeStats.readBytes) }}</span>
          </div>
          <div class="throughput-pill">
            <span class="pill-label">Write</span>
            <strong class="pill-rate" style="color: var(--accent-primary)">
              {{ formatBytes(activeStats.writeRate) }}/s
            </strong>
            <span class="pill-total">{{ activeStats.writeIops }} IOPS · {{ formatBytes(activeStats.writeBytes) }}</span>
          </div>
        </div>
      </div>

      <div class="traffic-chart-wrapper">
        <div class="traffic-chart-header">
          <span class="chart-section-title">Read / Write Throughput History</span>
          <span class="chart-pulse-indicator">
            <span class="pulse-dot"></span> /proc/diskstats
          </span>
        </div>
        <EChart
          :options="diskIoChartOptions"
          :not-merge="false"
          height="160px"
        />
      </div>

      <div class="io-list-section">
        <button
          type="button"
          class="io-list-toggle"
          :aria-expanded="showDevices"
          @click="showDevices = !showDevices"
        >
          <span>Devices ({{ disks.length }})</span>
          <MdiIcon :path="showDevices ? mdiChevronUp : mdiChevronDown" size="16" />
        </button>

        <div v-if="showDevices" class="io-rows">
          <button
            v-for="d in disks"
            :key="d.name"
            type="button"
            class="io-row mono"
            :class="{ active: selectedDisk === d.name }"
            @click="selectedDisk = selectedDisk === d.name ? '__all__' : d.name"
          >
            <div class="io-row-left">
              <span class="dev-name">{{ d.name }}</span>
              <span
                class="util-badge"
                :class="d.utilizationPercent >= 80 ? 'util-high' : d.utilizationPercent >= 40 ? 'util-med' : ''"
              >
                {{ d.utilizationPercent.toFixed(0) }}%
              </span>
            </div>
            <div class="io-row-metrics">
              <span class="metric-item">
                <span class="metric-dir">R</span>
                <span class="metric-val" style="color: var(--success)">{{ formatBytes(d.readRateBytesPerSec) }}/s</span>
                <span class="metric-iops">({{ d.readIops }})</span>
              </span>
              <span class="metric-item">
                <span class="metric-dir">W</span>
                <span class="metric-val" style="color: var(--accent-primary)">{{ formatBytes(d.writeRateBytesPerSec) }}/s</span>
                <span class="metric-iops">({{ d.writeIops }})</span>
              </span>
            </div>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.disk-io-card {
  height: auto;
  display: flex;
  flex-direction: column;
}

.disk-select {
  width: auto;
  min-width: 120px;
  padding: 3px 8px;
  font-size: 12px;
  height: 28px;
}

.io-summary-header {
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 10px 12px;
}

.io-summary-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.system-throughput-pills {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.throughput-pill {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 6px 10px;
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.pill-label {
  font-size: 10.5px;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.pill-rate {
  font-size: 14.5px;
}

.pill-total {
  font-size: 11px;
  color: var(--text-secondary);
}

.traffic-chart-wrapper {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 10px 14px 6px 14px;
  display: flex;
  flex-direction: column;
}

.traffic-chart-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 2px;
}

.chart-section-title {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.chart-pulse-indicator {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: var(--success);
  font-weight: 600;
  font-family: var(--font-mono, monospace);
}

.pulse-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--success);
  box-shadow: 0 0 6px var(--success);
}

.io-list-section {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.io-list-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 6px 10px;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  color: var(--text-secondary);
  font-size: 11.5px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  cursor: pointer;
  transition: border-color 0.15s ease, color 0.15s ease;
}

.io-list-toggle:hover {
  border-color: var(--accent-primary);
  color: var(--text-primary);
}

.io-rows {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.io-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
  width: 100%;
  padding: 7px 10px;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  color: var(--text-primary);
  font-size: 11.5px;
  font-variant-numeric: tabular-nums;
  text-align: left;
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease;
}

.io-row:hover {
  border-color: var(--accent-primary);
  background: var(--accent-bg-subtle, rgba(122, 162, 247, 0.08));
}

.io-row.active {
  border-color: var(--accent-primary);
  background: var(--accent-bg-subtle, rgba(122, 162, 247, 0.14));
}

.io-row-left {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.dev-name {
  font-weight: 600;
  color: var(--text-primary);
}

.util-badge {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
}

.util-badge.util-med {
  color: var(--warning, #e0af68);
  border-color: var(--warning, #e0af68);
}

.util-badge.util-high {
  color: var(--danger, #f7768e);
  border-color: var(--danger, #f7768e);
}

.io-row-metrics {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.metric-item {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
}

.metric-dir {
  font-size: 10px;
  font-weight: 700;
  color: var(--text-secondary);
}

.metric-val {
  font-weight: 600;
}

.metric-iops {
  font-size: 10.5px;
  color: var(--text-secondary);
}
</style>
