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
      orient: 'vertical',
      right: '4%',
      top: 'middle',
      icon: 'circle',
      textStyle: {
        color: p.textColor,
        fontSize: 12,
      },
      formatter: (name: string) => {
        let val = 0;
        if (name === 'Services & Apps') val = servicesBytes;
        else if (name === 'ZFS ARC Cache') val = zfsArcBytes;
        else if (name === 'Buffers & Cache') val = buffersAndCache;
        else if (name === 'Free RAM') val = freeBytes;
        const pct = Math.round((val / total) * 100);
        return `${name}: ${formatBytes(val)} (${pct}%)`;
      },
    },
    title: {
      text: formatBytes(total),
      subtext: 'Total RAM',
      left: '30%',
      top: '40%',
      textStyle: {
        color: p.textColor,
        fontSize: 16,
        fontWeight: 'bold',
      },
      subtextStyle: {
        color: p.subtextColor,
        fontSize: 11,
      },
    },
    series: [
      {
        name: 'Memory Breakdown',
        type: 'pie',
        radius: ['52%', '78%'],
        center: ['30%', '50%'],
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

    <!-- RAM Breakdown EChart with Legend -->
    <EChart
      :options="ramChartOptions"
      height="220px"
    />

    <!-- Memory Summary Footer -->
    <div class="ram-summary-footer">
      <div class="ram-summary-item">
        <span class="detail-name">Services & Apps</span>
        <strong class="mono" style="color: var(--accent-primary)">
          {{ formatBytes(telemetry?.memory?.servicesBytes || 0) }}
        </strong>
      </div>
      <div class="ram-summary-item">
        <span class="detail-name">ZFS ARC Cache</span>
        <strong class="mono" style="color: var(--info)">
          {{ formatBytes(telemetry?.memory?.zfsArcBytes || 0) }}
        </strong>
      </div>
      <div class="ram-summary-item">
        <span class="detail-name">Buffers / Cache</span>
        <strong class="mono" style="color: var(--warning)">
          {{ formatBytes(Math.max(0, (telemetry?.memory?.buffersBytes || 0) + (telemetry?.memory?.cachedBytes || 0) - (telemetry?.memory?.zfsArcBytes || 0))) }}
        </strong>
      </div>
      <div class="ram-summary-item">
        <span class="detail-name">Free RAM</span>
        <strong class="mono" style="color: var(--success)">
          {{ formatBytes(telemetry?.memory?.freeBytes || 0) }}
        </strong>
      </div>
    </div>
  </div>
</template>

<style scoped>
.memory-widget-card {
  height: 100%;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
}

.ram-summary-footer {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px solid var(--border-subtle);
}

.ram-summary-item {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.detail-name {
  font-size: 10.5px;
  color: var(--text-secondary);
}

@media (max-width: 480px) {
  .ram-summary-footer {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
