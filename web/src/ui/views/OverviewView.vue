<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from 'vue';
import type * as echarts from 'echarts';
import { formatBytes, formatUptime, type NetworkInterfaceTelemetry } from '../../domain/models';
import { nasState, refreshTelemetry } from '../store';
import { navigateTo } from '../router';
import { isDarkMode } from '../theme';
import EChart from '../components/EChart.vue';
import OverviewSkeleton from '../components/OverviewSkeleton.vue';
import SkeletonBox from '../components/SkeletonBox.vue';

// ---------------------------------------------------------------------------
// Telemetry & Periodic Polling
// ---------------------------------------------------------------------------
const telemetry = computed(() => nasState.systemTelemetry);
let pollTimer: ReturnType<typeof setInterval> | null = null;

// ---------------------------------------------------------------------------
// System Network Interface & Traffic History
// ---------------------------------------------------------------------------
interface TrafficPoint {
  time: string;
  rxRate: number; // Inbound (bytes/sec)
  txRate: number; // Outbound (bytes/sec)
}

const trafficHistory = ref<TrafficPoint[]>([]);
const MAX_HISTORY_POINTS = 16;

const systemInterface = computed<NetworkInterfaceTelemetry | null>(() => {
  const list = telemetry.value?.network || [];
  if (list.length === 0) return null;

  // 1. Explicit default route interface marked by backend
  const def = list.find((n) => n.isDefault);
  if (def) return def;

  // 2. Physical active adapter with IP (avoid loopback and virtual bridges)
  const activePhysical = list.find(
    (n) =>
      n.isUp &&
      n.ipAddresses &&
      n.ipAddresses.length > 0 &&
      !n.name.startsWith('docker') &&
      !n.name.startsWith('br-') &&
      !n.name.startsWith('vmnet') &&
      !n.name.startsWith('veth') &&
      !n.name.startsWith('virbr') &&
      n.name !== 'lo'
  );
  if (activePhysical) return activePhysical;

  // 3. Any active interface with an IP
  const activeWithIp = list.find(
    (n) => n.isUp && n.ipAddresses && n.ipAddresses.length > 0 && n.name !== 'lo'
  );
  if (activeWithIp) return activeWithIp;

  // 4. Fallback to first non-loopback interface
  return list.find((n) => n.name !== 'lo') || list[0] || null;
});

function recordTrafficSample() {
  const iface = systemInterface.value;
  const now = new Date();
  const timeStr = now.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  const rx = iface?.rxRateBytesPerSec || 0;
  const tx = iface?.txRateBytesPerSec || 0;

  trafficHistory.value.push({
    time: timeStr,
    rxRate: rx,
    txRate: tx,
  });

  if (trafficHistory.value.length > MAX_HISTORY_POINTS) {
    trafficHistory.value.shift();
  }
}

onMounted(() => {
  // Pre-seed traffic history with recent baseline timestamps
  const now = Date.now();
  const iface = systemInterface.value;
  const initRx = iface?.rxRateBytesPerSec || 0;
  const initTx = iface?.txRateBytesPerSec || 0;

  for (let i = 8; i >= 0; i--) {
    const t = new Date(now - i * 4000);
    trafficHistory.value.push({
      time: t.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
      rxRate: i === 0 ? initRx : 0,
      txRate: i === 0 ? initTx : 0,
    });
  }

  refreshTelemetry();
  pollTimer = setInterval(async () => {
    await refreshTelemetry();
    recordTrafficSample();
  }, 4000);
});

onBeforeUnmount(() => {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
});

// ---------------------------------------------------------------------------
// KPI Summaries
// ---------------------------------------------------------------------------
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

// ---------------------------------------------------------------------------
// Theme-Aware Chart Color Palette
// ---------------------------------------------------------------------------
const chartPalette = computed(() => {
  const dark = isDarkMode.value;
  return {
    textColor: dark ? '#c0caf5' : '#1f2328',
    subtextColor: dark ? '#a9b1d6' : '#57606a',
    borderColor: dark ? '#24283b' : '#d0d7de',
    tooltipBg: dark ? '#1f2335' : '#ffffff',
    tooltipBorder: dark ? '#3b4261' : '#d0d7de',

    // ZFS Pool Usage
    allocated: dark ? '#7aa2f7' : '#0969da',
    free: dark ? '#292e42' : '#eaeef2',

    // ZFS Pool Frag
    fragHigh: dark ? '#f7768e' : '#cf222e',
    fragMed: dark ? '#e0af68' : '#bf8700',
    fragLow: dark ? '#7dcfff' : '#0969da',
    contiguous: dark ? '#9ece6a' : '#1a7f37',

    // RAM Slices
    ramServices: dark ? '#7aa2f7' : '#0969da',
    ramArc: dark ? '#7dcfff' : '#0550ae',
    ramBuffers: dark ? '#e0af68' : '#9a6700',
    ramFree: dark ? '#9ece6a' : '#1a7f37',

    // CPU Gauge
    cpuLow: dark ? '#9ece6a' : '#1a7f37',
    cpuMed: dark ? '#e0af68' : '#bf8700',
    cpuHigh: dark ? '#f7768e' : '#cf222e',
    gaugeBg: dark ? '#292e42' : '#eaeef2',

    // Network Traffic Graph
    netIn: dark ? '#9ece6a' : '#1a7f37',
    netInArea: dark ? 'rgba(158, 206, 106, 0.22)' : 'rgba(26, 127, 55, 0.16)',
    netOut: dark ? '#7aa2f7' : '#0969da',
    netOutArea: dark ? 'rgba(122, 162, 247, 0.22)' : 'rgba(9, 105, 218, 0.16)',
    gridBorder: dark ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.08)',
  };
});

// ---------------------------------------------------------------------------
// 1. ZFS Storage Pools: ECharts Options per Pool
// ---------------------------------------------------------------------------
function getPoolUsageChartOptions(allocated: number, free: number, total: number): echarts.EChartsOption {
  const p = chartPalette.value;
  const pct = total > 0 ? Math.round((allocated / total) * 100) : 0;

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
    title: {
      text: `${pct}%`,
      subtext: 'Used',
      left: 'center',
      top: '38%',
      textStyle: {
        color: p.textColor,
        fontSize: 18,
        fontWeight: 'bold',
      },
      subtextStyle: {
        color: p.subtextColor,
        fontSize: 11,
      },
    },
    series: [
      {
        name: 'Pool Usage',
        type: 'pie',
        radius: ['58%', '82%'],
        center: ['50%', '50%'],
        avoidLabelOverlap: false,
        label: { show: false },
        labelLine: { show: false },
        emphasis: {
          scale: true,
          scaleSize: 5,
        },
        data: [
          {
            value: allocated,
            name: 'Allocated',
            itemStyle: { color: p.allocated },
          },
          {
            value: free,
            name: 'Free',
            itemStyle: { color: p.free },
          },
        ],
      },
    ],
  };
}

function getPoolFragChartOptions(fragPercent: number): echarts.EChartsOption {
  const p = chartPalette.value;
  const frag = Math.min(100, Math.max(0, fragPercent || 0));
  const contiguous = 100 - frag;
  const fragColor = frag > 40 ? p.fragHigh : frag > 20 ? p.fragMed : p.fragLow;

  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      backgroundColor: p.tooltipBg,
      borderColor: p.tooltipBorder,
      textStyle: { color: p.textColor, fontSize: 12 },
      formatter: (params: any) => {
        return `<strong>${params.name}</strong><br/>${params.value}%`;
      },
    },
    title: {
      text: `${frag}%`,
      subtext: 'Frag',
      left: 'center',
      top: '38%',
      textStyle: {
        color: fragColor,
        fontSize: 18,
        fontWeight: 'bold',
      },
      subtextStyle: {
        color: p.subtextColor,
        fontSize: 11,
      },
    },
    series: [
      {
        name: 'Fragmentation',
        type: 'pie',
        radius: ['58%', '82%'],
        center: ['50%', '50%'],
        avoidLabelOverlap: false,
        label: { show: false },
        labelLine: { show: false },
        emphasis: {
          scale: true,
          scaleSize: 5,
        },
        data: [
          {
            value: frag,
            name: 'Fragmented',
            itemStyle: { color: fragColor },
          },
          {
            value: contiguous,
            name: 'Contiguous',
            itemStyle: { color: p.contiguous },
          },
        ],
      },
    ],
  };
}

// ---------------------------------------------------------------------------
// 2. RAM Breakdown: ECharts Donut Chart with Legend
// ---------------------------------------------------------------------------
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

  const servicesBytes = mem.servicesBytes || 0;
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

// ---------------------------------------------------------------------------
// 3. CPU Telemetry: ECharts Gauge / Meter
// ---------------------------------------------------------------------------
const cpuUsageChartOptions = computed<echarts.EChartsOption>(() => {
  const p = chartPalette.value;
  const cpu = telemetry.value?.cpu;
  const usage = cpu ? parseFloat(cpu.usagePercent.toFixed(1)) : 0;

  return {
    backgroundColor: 'transparent',
    tooltip: {
      formatter: '{a} <br/>{c}%',
      backgroundColor: p.tooltipBg,
      borderColor: p.tooltipBorder,
      textStyle: { color: p.textColor, fontSize: 12 },
    },
    series: [
      {
        name: 'CPU Utilization',
        type: 'gauge',
        startAngle: 180,
        endAngle: 0,
        center: ['50%', '70%'],
        radius: '95%',
        min: 0,
        max: 100,
        splitNumber: 5,
        axisLine: {
          lineStyle: {
            width: 12,
            color: [
              [0.6, p.cpuLow],
              [0.85, p.cpuMed],
              [1, p.cpuHigh],
            ],
          },
        },
        pointer: {
          icon: 'path://M12.8,0.7l12,40.1H0.7L12.8,0.7z',
          length: '14%',
          width: 8,
          offsetCenter: [0, '-50%'],
          itemStyle: {
            color: 'auto',
          },
        },
        axisTick: {
          length: 6,
          lineStyle: {
            color: 'auto',
            width: 1,
          },
        },
        splitLine: {
          length: 12,
          lineStyle: {
            color: 'auto',
            width: 2,
          },
        },
        axisLabel: {
          color: p.subtextColor,
          fontSize: 10,
          distance: -20,
        },
        title: {
          offsetCenter: [0, '-10%'],
          fontSize: 12,
          color: p.subtextColor,
        },
        detail: {
          fontSize: 22,
          fontWeight: 'bold',
          offsetCenter: [0, '-28%'],
          valueAnimation: true,
          formatter: (value: number) => `${value.toFixed(1)}%`,
          color: p.textColor,
        },
        data: [
          {
            value: usage,
            name: 'CPU Load',
          },
        ],
      },
    ],
  };
});

// ---------------------------------------------------------------------------
// 4. Network Traffic (In/Out) Throughput History Chart
// ---------------------------------------------------------------------------
const networkTrafficChartOptions = computed<echarts.EChartsOption>(() => {
  const p = chartPalette.value;
  const times = trafficHistory.value.map((pt) => pt.time);
  const inRates = trafficHistory.value.map((pt) => pt.rxRate);
  const outRates = trafficHistory.value.map((pt) => pt.txRate);

  return {
    backgroundColor: 'transparent',
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
      data: ['Inbound (RX / In)', 'Outbound (TX / Out)'],
      top: 0,
      right: 0,
      icon: 'roundRect',
      itemWidth: 12,
      itemHeight: 8,
      textStyle: {
        color: p.textColor,
        fontSize: 11,
      },
    },
    grid: {
      top: 26,
      right: 12,
      bottom: 22,
      left: 60,
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: times,
      axisLine: { lineStyle: { color: p.borderColor } },
      axisLabel: { color: p.subtextColor, fontSize: 10 },
      splitLine: { show: false },
    },
    yAxis: {
      type: 'value',
      min: 0,
      splitLine: {
        lineStyle: { color: p.gridBorder, type: 'dashed' },
      },
      axisLabel: {
        color: p.subtextColor,
        fontSize: 10,
        formatter: (val: number) => `${formatBytes(val)}/s`,
      },
    },
    series: [
      {
        name: 'Inbound (RX / In)',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2, color: p.netIn },
        itemStyle: { color: p.netIn },
        areaStyle: {
          color: p.netInArea,
        },
        data: inRates,
      },
      {
        name: 'Outbound (TX / Out)',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2, color: p.netOut },
        itemStyle: { color: p.netOut },
        areaStyle: {
          color: p.netOutArea,
        },
        data: outRates,
      },
    ],
  };
});

// Temperature Status Class Helper
function getTempClass(temp: number): string {
  if (temp <= 0) return 'badge-neutral';
  if (temp < 65) return 'badge-success';
  if (temp < 80) return 'badge-warning';
  return 'badge-danger';
}
</script>

<template>
  <OverviewSkeleton v-if="!telemetry && (!nasState.initialLoaded || nasState.loading)" />
  <div v-else class="overview-dashboard">
    <!-- Top KPI Row -->
    <div class="grid-4">
      <div class="card">
        <div class="card-header" style="margin-bottom: 6px">
          <span class="card-subtitle">Storage</span>
          <span class="badge badge-success">{{ nasState.pools.length }} Pools</span>
        </div>
        <div style="font-size: 22px; font-weight: 700">
          {{ formatBytes(usedStorageBytes) }}
          <span style="font-size: 13px; color: var(--text-secondary); font-weight: 400">
            / {{ formatBytes(totalStorageBytes) }}
          </span>
        </div>
      </div>

      <div class="card">
        <div class="card-header" style="margin-bottom: 6px">
          <span class="card-subtitle">Processor & Load</span>
          <span
            v-if="(telemetry?.cpu?.temperatureCelsius || 0) > 0"
            class="badge"
            :class="getTempClass(telemetry?.cpu?.temperatureCelsius || 0)"
          >
            {{ telemetry?.cpu?.temperatureCelsius.toFixed(1) }} °C
          </span>
        </div>
        <div style="font-size: 22px; font-weight: 700">
          {{ (telemetry?.cpu?.usagePercent || 0).toFixed(1) }}%
          <span style="font-size: 13px; color: var(--text-secondary); font-weight: 400">
            load avg: {{ telemetry?.cpu?.load1?.toFixed(2) || '0.00' }}
          </span>
        </div>
        <p class="card-subtitle" style="margin-top: 6px">
          {{ telemetry?.cpu?.logicalCores || 0 }} Logical Threads ({{ telemetry?.cpu?.physicalCores || 0 }} Cores)
        </p>
      </div>

      <div class="card">
        <div class="card-header" style="margin-bottom: 6px">
          <span class="card-subtitle">Memory (RAM)</span>
          <span class="badge badge-info">
            {{ formatBytes(telemetry?.memory?.totalBytes || 0) }} Total
          </span>
        </div>
        <div style="font-size: 22px; font-weight: 700">
          {{ formatBytes(telemetry?.memory?.usedBytes || 0) }}
          <span style="font-size: 13px; color: var(--text-secondary); font-weight: 400">
            Used ({{ telemetry?.memory?.totalBytes ? Math.round(((telemetry.memory.usedBytes || 0) / telemetry.memory.totalBytes) * 100) : 0 }}%)
          </span>
        </div>
        <p class="card-subtitle" style="margin-top: 6px">
          ZFS ARC: {{ formatBytes(telemetry?.memory?.zfsArcBytes || 0) }} · Free: {{ formatBytes(telemetry?.memory?.freeBytes || 0) }}
        </p>
      </div>

      <div class="card">
        <div class="card-header" style="margin-bottom: 6px">
          <span class="card-subtitle">Workloads & Shares</span>
          <span class="badge badge-success">
            {{ runningVms + runningApps + runningLxc }} Active
          </span>
        </div>
        <div style="font-size: 22px; font-weight: 700">
          {{ nasState.vms.length }} VMs · {{ nasState.dockerApps.length }} Apps
        </div>
        <p class="card-subtitle" style="margin-top: 6px">
          {{ activeShares }} Shares · {{ nasState.users.length }} Users
        </p>
      </div>
    </div>

    <!-- ZFS Storage Pools: One Card Per Pool with ECharts Pie Charts -->
    <div>
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px">
        <h2 style="font-size: 18px; font-weight: 700; margin: 0">Pools</h2>
        <button class="btn btn-sm btn-primary" @click="navigateTo('zfs')">
          Manage Pools
        </button>
      </div>

      <!-- Moving Skeleton Placeholders for Storage Pools when Loading -->
      <div v-if="nasState.loading && nasState.pools.length === 0" class="pool-cards-grid">
        <div v-for="p in 2" :key="p" class="card pool-telemetry-card">
          <div class="card-header" style="border-bottom: 1px solid var(--border-subtle); padding-bottom: 10px; margin-bottom: 12px">
            <div style="display: flex; align-items: center; gap: 8px">
              <SkeletonBox width="110px" height="18px" />
              <SkeletonBox width="60px" height="20px" rounded="10px" />
            </div>
            <SkeletonBox width="65px" height="20px" rounded="10px" />
          </div>
          <div class="pool-charts-container">
            <div class="pool-chart-box">
              <SkeletonBox width="90px" height="13px" style="margin-bottom: 10px" />
              <div class="donut-skeleton-wrap" style="position: relative; width: 110px; height: 110px; display: flex; align-items: center; justify-content: center">
                <SkeletonBox width="110px" height="110px" circle />
                <div style="position: absolute; width: 60px; height: 60px; border-radius: 50%; background: var(--bg-base)"></div>
              </div>
              <div style="display: flex; gap: 10px; margin-top: 10px">
                <SkeletonBox width="70px" height="12px" />
                <SkeletonBox width="70px" height="12px" />
              </div>
            </div>
            <div class="pool-chart-box">
              <SkeletonBox width="95px" height="13px" style="margin-bottom: 10px" />
              <div class="donut-skeleton-wrap" style="position: relative; width: 110px; height: 110px; display: flex; align-items: center; justify-content: center">
                <SkeletonBox width="110px" height="110px" circle />
                <div style="position: absolute; width: 60px; height: 60px; border-radius: 50%; background: var(--bg-base)"></div>
              </div>
              <div style="display: flex; gap: 10px; margin-top: 10px">
                <SkeletonBox width="70px" height="12px" />
                <SkeletonBox width="70px" height="12px" />
              </div>
            </div>
          </div>
        </div>
      </div>

      <div v-else-if="nasState.pools.length === 0" class="card" style="text-align: center; padding: 36px 20px">
        <div style="font-size: 16px; font-weight: 600; margin-bottom: 16px">No pools configured.</div>
        <button class="btn btn-primary" @click="navigateTo('zfs')">Configure Pools</button>
      </div>

      <div class="pool-cards-grid">
        <div
          v-for="pool in nasState.pools"
          :key="pool.name"
          class="card pool-telemetry-card"
        >
          <!-- Pool Card Header -->
          <div class="card-header" style="border-bottom: 1px solid var(--border-subtle); padding-bottom: 10px; margin-bottom: 12px">
            <div style="display: flex; align-items: center; gap: 8px">
              <strong class="mono" style="font-size: 16px; color: var(--accent-primary)">
                {{ pool.poolName || pool.name }}
              </strong>
              <span class="badge badge-neutral">{{ pool.vdevLayout || 'stripe' }}</span>
              <span class="badge badge-info" v-if="pool.compression">{{ pool.compression }}</span>
            </div>
            <span
              class="badge"
              :class="pool.state === 'ONLINE' ? 'badge-success' : 'badge-warning'"
            >
              {{ pool.state }}
            </span>
          </div>

          <!-- Pool Charts: Two Pie Charts (Usage & Fragmentation) -->
          <div class="pool-charts-container">
            <!-- Usage Pie Chart -->
            <div class="pool-chart-box">
              <div class="chart-label">Storage Usage</div>
              <EChart
                :options="getPoolUsageChartOptions(pool.allocatedBytes || 0, pool.freeBytes || 0, pool.sizeBytes || 0)"
                height="150px"
              />
              <div class="chart-legend-row">
                <span class="legend-item">
                  <span class="dot" style="background: var(--accent-primary)"></span>
                  Used: {{ formatBytes(pool.allocatedBytes) }}
                </span>
                <span class="legend-item">
                  <span class="dot" style="background: var(--border-strong)"></span>
                  Free: {{ formatBytes(pool.freeBytes) }}
                </span>
              </div>
            </div>

            <!-- Fragmentation Pie Chart -->
            <div class="pool-chart-box">
              <div class="chart-label">Fragmentation</div>
              <EChart
                :options="getPoolFragChartOptions(pool.fragmentationPercent || 0)"
                height="150px"
              />
              <div class="chart-legend-row">
                <span class="legend-item">
                  <span
                    class="dot"
                    :style="{ background: (pool.fragmentationPercent || 0) > 30 ? 'var(--warning)' : 'var(--info)' }"
                  ></span>
                  Frag: {{ pool.fragmentationPercent || 0 }}%
                </span>
                <span class="legend-item">
                  <span class="dot" style="background: var(--success)"></span>
                  Contiguous: {{ 100 - (pool.fragmentationPercent || 0) }}%
                </span>
              </div>
            </div>
          </div>

          <!-- Pool Details Footer -->
          <div class="pool-details-footer">
            <div class="detail-cell">
              <span class="detail-name">Total Size</span>
              <strong class="mono">{{ formatBytes(pool.sizeBytes) }}</strong>
            </div>
            <div class="detail-cell">
              <span class="detail-name">Dedup Ratio</span>
              <strong class="mono">{{ pool.dedupRatio ? `${pool.dedupRatio}x` : '1.00x' }}</strong>
            </div>
            <div class="detail-cell">
              <span class="detail-name">Scrub</span>
              <span class="mono" style="font-size: 11px">{{ pool.lastScrubStatus || 'Never' }}</span>
            </div>
            <div class="detail-cell">
              <span class="detail-name">Mount</span>
              <span class="mono" style="font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap" :title="pool.mountpoint || 'Default'">
                {{ pool.mountpoint || 'Default' }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Middle Row: CPU Load & Temperature + RAM Breakdown -->
    <div class="grid-2">
      <!-- CPU Telemetry Card -->
      <div class="card">
        <div class="card-header">
          <h3 class="card-title">Processor</h3>
          <span
            v-if="(telemetry?.cpu?.temperatureCelsius || 0) > 0"
            class="badge"
            :class="getTempClass(telemetry?.cpu?.temperatureCelsius || 0)"
            style="font-size: 13px; font-weight: 600"
          >
            {{ telemetry?.cpu?.temperatureCelsius.toFixed(1) }} °C
          </span>
        </div>

        <div style="margin-bottom: 12px">
          <div style="font-weight: 600; font-size: 14px; margin-bottom: 2px">
            {{ telemetry?.cpu?.model || 'Detecting Processor Model...' }}
          </div>
          <div class="card-subtitle mono" style="font-size: 12px">
            {{ telemetry?.cpu?.physicalCores || 0 }} Physical Cores · {{ telemetry?.cpu?.logicalCores || 0 }} Logical Threads
          </div>
        </div>

        <!-- CPU Gauge EChart -->
        <EChart
          :options="cpuUsageChartOptions"
          height="160px"
        />

        <!-- Load Averages & Thermal Details -->
        <div class="cpu-metrics-bar">
          <div class="metric-pill">
            <span class="pill-label">Load 1m</span>
            <strong class="mono pill-val">{{ telemetry?.cpu?.load1?.toFixed(2) || '0.00' }}</strong>
          </div>
          <div class="metric-pill">
            <span class="pill-label">Load 5m</span>
            <strong class="mono pill-val">{{ telemetry?.cpu?.load5?.toFixed(2) || '0.00' }}</strong>
          </div>
          <div class="metric-pill">
            <span class="pill-label">Load 15m</span>
            <strong class="mono pill-val">{{ telemetry?.cpu?.load15?.toFixed(2) || '0.00' }}</strong>
          </div>
          <div class="metric-pill">
            <span class="pill-label">CPU Temp</span>
            <strong
              class="mono pill-val"
              :style="{ color: (telemetry?.cpu?.temperatureCelsius || 0) > 75 ? 'var(--danger)' : 'var(--text-primary)' }"
            >
              {{ (telemetry?.cpu?.temperatureCelsius || 0) > 0 ? `${telemetry?.cpu?.temperatureCelsius.toFixed(1)}°C` : 'N/A' }}
            </strong>
          </div>
        </div>
      </div>

      <!-- RAM Information Card with Donut Chart and Legend -->
      <div class="card">
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
    </div>

    <!-- Static System & Hardware Info + Network Interfaces -->
    <div class="grid-2">
      <!-- Hardware Info Card -->
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

      <!-- Network Interface & Bandwidth Card (System Interface Only + In/Out Traffic Graph) -->
      <div class="card system-network-card">
        <div class="card-header" style="border-bottom: 1px solid var(--border-subtle); padding-bottom: 8px; margin-bottom: 12px">
          <h3 class="card-title">Network</h3>
          <span
            v-if="systemInterface"
            class="badge"
            :class="systemInterface.isUp ? 'badge-success' : 'badge-neutral'"
          >
            {{ systemInterface.isUp ? 'ONLINE' : 'DOWN' }}
          </span>
        </div>

        <div v-if="!systemInterface" class="card-subtitle" style="padding: 24px 0; text-align: center">
          Scanning system network interface...
        </div>

        <div v-else style="display: flex; flex-direction: column; gap: 12px">
          <!-- System Interface Details & Live Rate Badges -->
          <div class="system-iface-header">
            <div>
              <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 4px">
                <strong class="mono" style="font-size: 16px; color: var(--accent-primary)">
                  {{ systemInterface.name }}
                </strong>
                <span v-if="systemInterface.speedMbps && systemInterface.speedMbps > 0" class="badge badge-neutral mono">
                  {{ systemInterface.speedMbps }} Mbps
                </span>
              </div>

              <!-- IP Addresses Chips -->
              <div class="mono system-ips">
                <span v-if="systemInterface.ipAddresses.length === 0" style="color: var(--text-muted)">
                  No IP assigned
                </span>
                <span
                  v-for="ip in systemInterface.ipAddresses"
                  :key="ip"
                  class="ip-chip"
                >
                  {{ ip }}
                </span>
              </div>

              <div v-if="systemInterface.macAddress" class="mono" style="font-size: 11.5px; color: var(--text-secondary); margin-top: 4px">
                MAC: {{ systemInterface.macAddress }}
              </div>
            </div>

            <!-- In/Out Throughput Rate Badges -->
            <div class="system-throughput-pills mono">
              <div class="throughput-pill pill-rx">
                <span class="pill-label">Inbound (RX)</span>
                <strong class="pill-rate" style="color: var(--success)">
                  {{ formatBytes(systemInterface.rxRateBytesPerSec) }}/s
                </strong>
                <span class="pill-total">Total: {{ formatBytes(systemInterface.rxBytes) }}</span>
              </div>
              <div class="throughput-pill pill-tx">
                <span class="pill-label">Outbound (TX)</span>
                <strong class="pill-rate" style="color: var(--accent-primary)">
                  {{ formatBytes(systemInterface.txRateBytesPerSec) }}/s
                </strong>
                <span class="pill-total">Total: {{ formatBytes(systemInterface.txBytes) }}</span>
              </div>
            </div>
          </div>

          <!-- Real-Time In/Out Traffic Line Graph -->
          <div class="traffic-chart-wrapper">
            <div class="traffic-chart-header">
              <span class="chart-section-title">In / Out Throughput History</span>
              <span class="chart-pulse-indicator">
                <span class="pulse-dot"></span> Live
              </span>
            </div>
            <EChart
              :options="networkTrafficChartOptions"
              height="165px"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- Bottom Quick Overview: Workloads & Active Shares -->
    <div class="grid-3">
      <div class="card">
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

      <div class="card">
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
          <span class="badge" :class="app.state === 'RUNNING' ? 'badge-success' : app.state === 'PARTIALLY_RUNNING' ? 'badge-warning' : 'badge-neutral'">
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

      <div class="card">
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
.overview-dashboard {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* Pool Cards Grid */
.pool-cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(420px, 1fr));
  gap: 16px;
}

.pool-telemetry-card {
  display: flex;
  flex-direction: column;
}

.pool-charts-container {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  align-items: center;
  margin-bottom: 12px;
}

.pool-chart-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 10px 6px 8px 6px;
}

.chart-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 4px;
}

.chart-legend-row {
  display: flex;
  flex-direction: column;
  gap: 3px;
  font-size: 11px;
  margin-top: 6px;
  font-family: var(--font-mono, monospace);
}

.legend-item {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-secondary);
}

.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
}

.pool-details-footer {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
  padding-top: 10px;
  border-top: 1px solid var(--border-subtle);
  background: var(--bg-base);
  padding: 10px;
  border-radius: 6px;
}

.detail-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.detail-name {
  font-size: 11px;
  color: var(--text-secondary);
}

/* CPU Metrics Bar */
.cpu-metrics-bar {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
  margin-top: 12px;
}

.metric-pill {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 8px 10px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.pill-label {
  font-size: 10px;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.pill-val {
  font-size: 14px;
}

/* RAM Summary Footer */
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

/* Hardware Info Card */
.fastfetch-card {
  font-family: inherit;
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

/* System Network Interface & Traffic Graph */
.system-iface-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 14px;
  flex-wrap: wrap;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 12px 14px;
}

.system-ips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  font-size: 12px;
}

.ip-chip {
  background: var(--accent-bg-subtle, rgba(122, 162, 247, 0.12));
  color: var(--text-primary);
  border: 1px solid var(--border-subtle);
  padding: 1px 6px;
  border-radius: 4px;
}

.system-throughput-pills {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.throughput-pill {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 8px 12px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 140px;
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

@media (max-width: 900px) {
  .pool-cards-grid {
    grid-template-columns: 1fr;
  }
  .system-iface-header {
    flex-direction: column;
  }
}
</style>
