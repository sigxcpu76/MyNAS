<script setup lang="ts">
import { computed, ref } from 'vue';
import type * as echarts from 'echarts';
import { mdiChevronDown, mdiChevronUp } from '@mdi/js';
import { formatBytes, type ZpoolIoTelemetry } from '../../../domain/models';
import { nasState, type IoRatePoint } from '../../store';
import { useChartPalette } from '../chartPalette';
import EChart from '../../components/EChart.vue';
import MdiIcon from '../../components/MdiIcon.vue';

const props = defineProps<{
  ioHistory?: IoRatePoint[];
}>();

const selectedPool = ref<string>('__all__');
const showPools = ref(false);
const telemetry = computed(() => nasState.systemTelemetry);
const chartPalette = useChartPalette();

const zpools = computed<ZpoolIoTelemetry[]>(() => telemetry.value?.zpoolIo || []);

const poolCapacityMap = computed(() => {
  const map = new Map<string, { alloc: number; free: number }>();
  for (const p of nasState.pools || []) {
    const clean = p.poolName || p.name.replace(/^zfsPools\//, '');
    map.set(clean, { alloc: p.allocatedBytes || 0, free: p.freeBytes || 0 });
  }
  return map;
});

const activeStats = computed(() => {
  const list = zpools.value;
  if (list.length === 0) return null;

  if (selectedPool.value !== '__all__') {
    const found = list.find((z) => z.poolName === selectedPool.value);
    if (found) {
      return {
        label: found.poolName,
        readRate: found.readRateBytesPerSec,
        writeRate: found.writeRateBytesPerSec,
        readIops: found.readIops,
        writeIops: found.writeIops,
        readBytes: found.readBytes,
        writeBytes: found.writeBytes,
        waitQueue: found.waitQueue,
        activeQueue: found.activeQueue,
      };
    }
  }

  let readRate = 0;
  let writeRate = 0;
  let readIops = 0;
  let writeIops = 0;
  let readBytes = 0;
  let writeBytes = 0;
  let waitQueue = 0;
  let activeQueue = 0;

  for (const z of list) {
    readRate += z.readRateBytesPerSec || 0;
    writeRate += z.writeRateBytesPerSec || 0;
    readIops += z.readIops || 0;
    writeIops += z.writeIops || 0;
    readBytes += z.readBytes || 0;
    writeBytes += z.writeBytes || 0;
    waitQueue += z.waitQueue || 0;
    activeQueue += z.activeQueue || 0;
  }

  return {
    label: `All Pools (${list.length})`,
    readRate,
    writeRate,
    readIops,
    writeIops,
    readBytes,
    writeBytes,
    waitQueue,
    activeQueue,
  };
});

const historyPoints = computed(() => {
  if (props.ioHistory && props.ioHistory.length > 0) {
    return props.ioHistory;
  }
  return nasState.zpoolIoHistory || [];
});

const zpoolIoChartOptions = computed<echarts.EChartsOption>(() => {
  const p = chartPalette.value;
  const history = historyPoints.value;
  const times = history.map((pt) => pt.time);
  const key = selectedPool.value;
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
  <div class="card zpool-io-card">
    <div class="card-header" style="border-bottom: 1px solid var(--border-subtle); padding-bottom: 8px; margin-bottom: 12px">
      <div style="display: flex; align-items: center; gap: 8px">
        <h3 class="card-title">Zpool IOStat</h3>
        <span v-if="zpools.length > 0" class="badge badge-neutral mono">{{ zpools.length }} Pools</span>
      </div>
      <select
        v-if="zpools.length > 0"
        v-model="selectedPool"
        class="form-control mono pool-select"
        aria-label="Filter ZFS pool I/O"
      >
        <option value="__all__">All Pools</option>
        <option v-for="z in zpools" :key="z.poolName" :value="z.poolName">{{ z.poolName }}</option>
      </select>
    </div>

    <div v-if="!activeStats" class="card-subtitle" style="padding: 24px 0; text-align: center">
      No imported ZFS pools reporting kstat I/O.
    </div>

    <div v-else style="display: flex; flex-direction: column; gap: 12px">
      <div class="io-summary-header">
        <div class="io-summary-top">
          <strong class="mono" style="font-size: 14px; color: var(--accent-primary)">
            {{ activeStats.label }}
          </strong>
          <span class="mono" style="font-size: 11px; color: var(--text-secondary)">
            Q: {{ activeStats.waitQueue }} wait · {{ activeStats.activeQueue }} active
          </span>
        </div>

        <div class="system-throughput-pills mono">
          <div class="throughput-pill">
            <span class="pill-label">Pool Read</span>
            <strong class="pill-rate" style="color: var(--success)">
              {{ formatBytes(activeStats.readRate) }}/s
            </strong>
            <span class="pill-total">{{ activeStats.readIops }} IOPS · {{ formatBytes(activeStats.readBytes) }}</span>
          </div>
          <div class="throughput-pill">
            <span class="pill-label">Pool Write</span>
            <strong class="pill-rate" style="color: var(--accent-primary)">
              {{ formatBytes(activeStats.writeRate) }}/s
            </strong>
            <span class="pill-total">{{ activeStats.writeIops }} IOPS · {{ formatBytes(activeStats.writeBytes) }}</span>
          </div>
        </div>
      </div>

      <div class="traffic-chart-wrapper">
        <div class="traffic-chart-header">
          <span class="chart-section-title">ZFS Pool I/O Throughput</span>
          <span class="chart-pulse-indicator">
            <span class="pulse-dot"></span> /proc/spl/kstat/zfs
          </span>
        </div>
        <EChart
          :options="zpoolIoChartOptions"
          :not-merge="false"
          height="160px"
        />
      </div>

      <div class="io-list-section">
        <button
          type="button"
          class="io-list-toggle"
          :aria-expanded="showPools"
          @click="showPools = !showPools"
        >
          <span>Pools ({{ zpools.length }})</span>
          <MdiIcon :path="showPools ? mdiChevronUp : mdiChevronDown" size="16" />
        </button>

        <div v-if="showPools" class="io-rows">
          <button
            v-for="z in zpools"
            :key="z.poolName"
            type="button"
            class="io-row mono"
            :class="{ active: selectedPool === z.poolName }"
            @click="selectedPool = selectedPool === z.poolName ? '__all__' : z.poolName"
          >
            <div class="io-row-left">
              <span class="pool-name">{{ z.poolName }}</span>
              <span v-if="poolCapacityMap.has(z.poolName)" class="cap-badge">
                {{ formatBytes(poolCapacityMap.get(z.poolName)!.alloc) }} / {{ formatBytes(poolCapacityMap.get(z.poolName)!.free) }} free
              </span>
              <span class="queue-badge">
                Q {{ z.waitQueue }}/{{ z.activeQueue }}
              </span>
            </div>
            <div class="io-row-metrics">
              <span class="metric-item">
                <span class="metric-dir">R</span>
                <span class="metric-val" style="color: var(--success)">{{ formatBytes(z.readRateBytesPerSec) }}/s</span>
                <span class="metric-iops">({{ z.readIops }})</span>
              </span>
              <span class="metric-item">
                <span class="metric-dir">W</span>
                <span class="metric-val" style="color: var(--accent-primary)">{{ formatBytes(z.writeRateBytesPerSec) }}/s</span>
                <span class="metric-iops">({{ z.writeIops }})</span>
              </span>
            </div>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.zpool-io-card {
  height: auto;
  display: flex;
  flex-direction: column;
}

.pool-select {
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
  flex-wrap: wrap;
  min-width: 0;
}

.pool-name {
  font-weight: 600;
  color: var(--text-primary);
}

.cap-badge,
.queue-badge {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
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
