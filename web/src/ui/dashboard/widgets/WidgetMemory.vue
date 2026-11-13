<script setup lang="ts">
import { computed } from 'vue';
import type * as echarts from 'echarts';
import { formatBytes } from '../../../domain/models';
import { nasState } from '../../store';
import { useChartPalette } from '../chartPalette';
import EChart from '../../components/EChart.vue';

const telemetry = computed(() => nasState.systemTelemetry);
const chartPalette = useChartPalette();

const ramChartOptions = computed<echarts.EChartsOption>(() => {
  const p = chartPalette.value;
  const mem = telemetry.value?.memory;

  if (!mem || mem.totalBytes <= 0) {
    return {
      backgroundColor: 'transparent',
      title: {
        text: 'Loading...',
        left: 'center',
        top: 'middle',
        textStyle: { color: p.subtextColor, fontSize: 13 },
      },
    };
  }

  const servicesBytes = mem.servicesBytes || Math.max(0, (mem.usedBytes || 0) - (mem.zfsArcBytes || 0));
  const zfsArcBytes = mem.zfsArcBytes || 0;
  const buffersAndCache = Math.max(0, (mem.buffersBytes || 0) + (mem.cachedBytes || 0) - zfsArcBytes);
  const freeBytes = mem.freeBytes || 0;
  const total = mem.totalBytes || 1;

  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      backgroundColor: p.tooltipBg,
      borderColor: p.tooltipBorder,
      textStyle: { color: p.textColor, fontSize: 12 },
      formatter: (params: any) => {
        return `<strong>${params.name}</strong><br/>${formatBytes(params.value)} (${params.percent}%)`;
      },
    },
    legend: {
      show: false,
    },
    title: {
      text: formatBytes(total),
      subtext: 'Total RAM',
      left: 'center',
      top: '35%',
      textStyle: {
        color: p.textColor,
        fontSize: 15,
        fontWeight: 'bold',
      },
      subtextStyle: {
        color: p.subtextColor,
        fontSize: 10.5,
      },
    },
    series: [
      {
        name: 'Memory Breakdown',
        type: 'pie',
        radius: ['58%', '86%'],
        center: ['50%', '50%'],
        avoidLabelOverlap: false,
        label: { show: false },
        labelLine: { show: false },
        emphasis: {
          scale: true,
          scaleSize: 6,
        },
        data: [
          {
            value: servicesBytes,
            name: 'Services & Apps',
            itemStyle: { color: p.ramServices },
          },
          {
            value: zfsArcBytes,
            name: 'ZFS ARC Cache',
            itemStyle: { color: p.ramArc },
          },
          {
            value: buffersAndCache,
            name: 'Buffers & Cache',
            itemStyle: { color: p.ramBuffers },
          },
          {
            value: freeBytes,
            name: 'Free RAM',
            itemStyle: { color: p.ramFree },
          },
        ],
      },
    ],
  };
});
</script>

<template>
  <div class="card memory-widget-card">
    <div class="card-header">
      <div>
        <h3 class="card-title">Memory</h3>
        <p class="card-subtitle">
          {{ formatBytes(telemetry?.memory?.availableBytes || 0) }} Available · {{ formatBytes(telemetry?.memory?.totalBytes || 0) }} Total
        </p>
      </div>
      <span class="badge badge-info">
        {{ telemetry?.memory?.totalBytes ? Math.round(((telemetry.memory.usedBytes || 0) / telemetry.memory.totalBytes) * 100) : 0 }}% In-Use
      </span>
    </div>

    <!-- Centered RAM Breakdown Donut -->
    <EChart
      :options="ramChartOptions"
      height="155px"
    />

    <!-- Clean 2x2 Memory Breakdown Grid -->
    <div class="ram-summary-footer">
      <div class="ram-summary-item">
        <span class="detail-name">
          <span class="legend-dot" :style="{ background: chartPalette.ramServices }"></span>
          Services & Apps
        </span>
        <strong class="mono detail-val" style="color: var(--accent-primary)">
          {{ formatBytes(telemetry?.memory?.servicesBytes || Math.max(0, (telemetry?.memory?.usedBytes || 0) - (telemetry?.memory?.zfsArcBytes || 0))) }}
        </strong>
      </div>
      <div class="ram-summary-item">
        <span class="detail-name">
          <span class="legend-dot" :style="{ background: chartPalette.ramArc }"></span>
          ZFS ARC Cache
        </span>
        <strong class="mono detail-val" style="color: var(--info)">
          {{ formatBytes(telemetry?.memory?.zfsArcBytes || 0) }}
        </strong>
      </div>
      <div class="ram-summary-item">
        <span class="detail-name">
          <span class="legend-dot" :style="{ background: chartPalette.ramBuffers }"></span>
          Buffers / Cache
        </span>
        <strong class="mono detail-val" style="color: var(--warning)">
          {{ formatBytes(Math.max(0, (telemetry?.memory?.buffersBytes || 0) + (telemetry?.memory?.cachedBytes || 0) - (telemetry?.memory?.zfsArcBytes || 0))) }}
        </strong>
      </div>
      <div class="ram-summary-item">
        <span class="detail-name">
          <span class="legend-dot" :style="{ background: chartPalette.ramFree }"></span>
          Free RAM
        </span>
        <strong class="mono detail-val" style="color: var(--success)">
          {{ formatBytes(telemetry?.memory?.freeBytes || 0) }}
        </strong>
      </div>
    </div>
  </div>
</template>

<style scoped>
.memory-widget-card {
  height: auto;
  display: flex;
  flex-direction: column;
}

.ram-summary-footer {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin-top: 8px;
  padding-top: 10px;
  border-top: 1px solid var(--border-subtle);
}

.ram-summary-item {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 7px 10px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
}

.detail-name {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.legend-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.detail-val {
  font-size: 12px;
  white-space: nowrap;
  flex-shrink: 0;
}

@media (max-width: 420px) {
  .ram-summary-footer {
    grid-template-columns: 1fr;
  }
}
</style>
