<script setup lang="ts">
import { computed } from 'vue';
import type * as echarts from 'echarts';
import { nasState } from '../../store';
import { useChartPalette } from '../chartPalette';
import EChart from '../../components/EChart.vue';

const telemetry = computed(() => nasState.systemTelemetry);
const chartPalette = useChartPalette();

const packageTemps = computed<number[]>(() => {
  const temps = telemetry.value?.cpu?.packageTemperatures;
  if (Array.isArray(temps) && temps.length > 0) {
    return temps;
  }
  const t = telemetry.value?.cpu?.temperatureCelsius;
  return t && t > 0 ? [t] : [];
});

function getTempClass(temp: number): string {
  if (temp <= 0) return 'badge-neutral';
  if (temp < 65) return 'badge-success';
  if (temp < 80) return 'badge-warning';
  return 'badge-danger';
}

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
</script>

<template>
  <div class="card cpu-widget-card">
    <div class="card-header">
      <h3 class="card-title">Processor</h3>
      <!-- Multiple CPU package temperature badges -->
      <div v-if="packageTemps.length > 1" style="display: flex; gap: 6px; align-items: center; flex-wrap: wrap">
        <span
          v-for="(t, idx) in packageTemps"
          :key="idx"
          class="badge"
          :class="getTempClass(t)"
          style="font-size: 11.5px; font-weight: 600"
          :title="`CPU Socket ${idx} Package Temperature`"
        >
          CPU{{ idx }}: {{ t.toFixed(1) }} °C
        </span>
      </div>
      <!-- Single package fallback badge -->
      <span
        v-else-if="(telemetry?.cpu?.temperatureCelsius || 0) > 0"
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
    <div
      class="cpu-metrics-bar"
      :style="packageTemps.length > 1 ? { gridTemplateColumns: `repeat(${3 + packageTemps.length}, 1fr)` } : {}"
    >
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
      <template v-if="packageTemps.length > 1">
        <div v-for="(t, idx) in packageTemps" :key="idx" class="metric-pill">
          <span class="pill-label">CPU{{ idx }} Temp</span>
          <strong
            class="mono pill-val"
            :style="{ color: t > 75 ? 'var(--danger)' : 'var(--text-primary)' }"
          >
            {{ t > 0 ? `${t.toFixed(1)}°C` : 'N/A' }}
          </strong>
        </div>
      </template>
      <div v-else class="metric-pill">
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
</template>

<style scoped>
.cpu-widget-card {
  height: 100%;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
}

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

@media (max-width: 480px) {
  .cpu-metrics-bar {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
