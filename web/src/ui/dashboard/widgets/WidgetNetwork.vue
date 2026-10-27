<script setup lang="ts">
import { computed } from 'vue';
import type * as echarts from 'echarts';
import { formatBytes, type NetworkInterfaceTelemetry } from '../../../domain/models';
import { nasState } from '../../store';
import { useChartPalette } from '../chartPalette';
import EChart from '../../components/EChart.vue';

export interface TrafficPoint {
  time: string;
  rxRate: number;
  txRate: number;
}

const props = defineProps<{
  trafficHistory?: TrafficPoint[];
}>();

const telemetry = computed(() => nasState.systemTelemetry);
const chartPalette = useChartPalette();

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

const historyPoints = computed(() => props.trafficHistory || []);

const networkTrafficChartOptions = computed<echarts.EChartsOption>(() => {
  const p = chartPalette.value;
  const history = historyPoints.value;
  const times = history.map((pt) => pt.time);
  const inRates = history.map((pt) => pt.rxRate);
  const outRates = history.map((pt) => pt.txRate);

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
</script>

<template>
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
</template>

<style scoped>
.system-network-card {
  height: 100%;
  display: flex;
  flex-direction: column;
}

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
  .system-iface-header {
    flex-direction: column;
  }
}
</style>
