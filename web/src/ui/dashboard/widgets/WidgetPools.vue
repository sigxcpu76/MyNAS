<script setup lang="ts">
import { ref } from 'vue';
import type * as echarts from 'echarts';
import { formatBytes } from '../../../domain/models';
import { nasState } from '../../store';
import { navigateTo } from '../../router';
import { useChartPalette } from '../chartPalette';
import EChart from '../../components/EChart.vue';
import SkeletonBox from '../../components/SkeletonBox.vue';

const chartPalette = useChartPalette();
const poolChartModes = ref<Record<string, 'usage' | 'frag'>>({});

function getPoolChartMode(poolName: string): 'usage' | 'frag' {
  return poolChartModes.value[poolName] || 'usage';
}

function setPoolChartMode(poolName: string, mode: 'usage' | 'frag') {
  poolChartModes.value[poolName] = mode;
}

function getPoolUsageChartOptions(allocated: number, free: number, total: number): echarts.EChartsOption {
  const p = chartPalette.value;
  const pct = total > 0 ? Math.round((allocated / total) * 100) : 0;

  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      backgroundColor: p.tooltipBg,
      borderColor: p.tooltipBorder,
      textStyle: { color: p.textColor, fontSize: 11 },
      formatter: (params: any) => {
        return `<strong>${params.name}</strong><br/>${formatBytes(params.value)} (${params.percent}%)`;
      },
    },
    title: {
      text: `${pct}%`,
      subtext: 'Used',
      left: 'center',
      top: '30%',
      textStyle: {
        color: p.textColor,
        fontSize: 15,
        fontWeight: 'bold',
      },
      subtextStyle: {
        color: p.subtextColor,
        fontSize: 10,
      },
    },
    series: [
      {
        name: 'Pool Usage',
        type: 'pie',
        radius: ['52%', '76%'],
        center: ['50%', '50%'],
        avoidLabelOverlap: false,
        label: { show: false },
        labelLine: { show: false },
        emphasis: {
          scale: true,
          scaleSize: 4,
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
      textStyle: { color: p.textColor, fontSize: 11 },
      formatter: (params: any) => {
        return `<strong>${params.name}</strong><br/>${params.value}%`;
      },
    },
    title: {
      text: `${frag}%`,
      subtext: 'Frag',
      left: 'center',
      top: '30%',
      textStyle: {
        color: fragColor,
        fontSize: 15,
        fontWeight: 'bold',
      },
      subtextStyle: {
        color: p.subtextColor,
        fontSize: 10,
      },
    },
    series: [
      {
        name: 'Fragmentation',
        type: 'pie',
        radius: ['52%', '76%'],
        center: ['50%', '50%'],
        avoidLabelOverlap: false,
        label: { show: false },
        labelLine: { show: false },
        emphasis: {
          scale: true,
          scaleSize: 4,
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
</script>

<template>
  <div class="pools-widget">
    <div class="pools-header">
      <h2 style="font-size: 18px; font-weight: 700; margin: 0">Pools</h2>
      <button class="btn btn-sm btn-primary" @click="navigateTo('zfs')">
        Manage Pools
      </button>
    </div>

    <!-- Moving Skeleton Placeholders for Storage Pools when Loading -->
    <div v-if="nasState.loading && nasState.pools.length === 0" class="pool-cards-grid">
      <div v-for="p in 2" :key="p" class="card pool-telemetry-card">
        <div class="card-header" style="border-bottom: 1px solid var(--border-subtle); padding-bottom: 8px; margin-bottom: 10px">
          <div style="display: flex; align-items: center; gap: 8px">
            <SkeletonBox width="110px" height="18px" />
            <SkeletonBox width="60px" height="20px" rounded="10px" />
          </div>
          <SkeletonBox width="65px" height="20px" rounded="10px" />
        </div>
        <div class="pool-compact-body">
          <div class="pool-info-col" style="gap: 8px">
            <SkeletonBox v-for="j in 6" :key="j" width="100%" height="13px" />
          </div>
          <div class="pool-col-divider"></div>
          <div class="pool-chart-col">
            <div style="position: relative; width: 90px; height: 90px; display: flex; align-items: center; justify-content: center">
              <SkeletonBox width="90px" height="90px" circle />
              <div style="position: absolute; width: 50px; height: 50px; border-radius: 50%; background: var(--bg-base)"></div>
            </div>
            <div style="display: flex; gap: 8px; margin-top: 8px">
              <SkeletonBox width="50px" height="11px" />
              <SkeletonBox width="50px" height="11px" />
            </div>
          </div>
        </div>
      </div>
    </div>

    <div v-else-if="nasState.pools.length === 0" class="card" style="text-align: center; padding: 36px 20px">
      <div style="font-size: 16px; font-weight: 600; margin-bottom: 16px">No pools configured.</div>
      <button class="btn btn-primary" @click="navigateTo('zfs')">Configure Pools</button>
    </div>

    <div v-else class="pool-cards-grid">
      <div
        v-for="pool in nasState.pools"
        :key="pool.name"
        class="card pool-telemetry-card"
      >
        <!-- Pool Card Header -->
        <div class="card-header" style="border-bottom: 1px solid var(--border-subtle); padding-bottom: 8px; margin-bottom: 10px">
          <div style="display: flex; align-items: center; gap: 8px">
            <strong class="mono" style="font-size: 15px; color: var(--accent-primary)">
              {{ pool.poolName || pool.name.replace(/^zfsPools\//, '') }}
            </strong>
            <span class="badge badge-neutral" style="font-size: 10.5px; padding: 1px 6px">{{ pool.vdevLayout || 'stripe' }}</span>
            <span class="badge badge-info" style="font-size: 10.5px; padding: 1px 6px" v-if="pool.compression">{{ pool.compression }}</span>
          </div>
          <span
            class="badge mono"
            :class="pool.state === 'ONLINE' ? 'badge-success' : 'badge-warning'"
            style="font-size: 10.5px; font-weight: 600; padding: 2px 7px"
          >
            {{ pool.state }}
          </span>
        </div>

        <!-- Pool Compact Body: Left Half Info, Right Half Chart -->
        <div class="pool-compact-body">
          <!-- Left Half: Pool Info & Key Metrics -->
          <div class="pool-info-col">
            <div class="pool-stat-row">
              <span class="stat-label">Total Size</span>
              <span class="stat-value mono">{{ formatBytes(pool.sizeBytes) }}</span>
            </div>
            <div class="pool-stat-row">
              <span class="stat-label">Allocated</span>
              <span class="stat-value mono" style="color: var(--accent-primary)">{{ formatBytes(pool.allocatedBytes) }}</span>
            </div>
            <div class="pool-stat-row">
              <span class="stat-label">Free Space</span>
              <span class="stat-value mono" style="color: var(--text-secondary)">{{ formatBytes(pool.freeBytes) }}</span>
            </div>
            <div class="pool-stat-row">
              <span class="stat-label">Fragmentation</span>
              <span class="stat-value mono" :style="{ color: (pool.fragmentationPercent || 0) > 40 ? 'var(--danger)' : (pool.fragmentationPercent || 0) > 20 ? 'var(--warning)' : 'var(--success)' }">
                {{ pool.fragmentationPercent || 0 }}%
              </span>
            </div>
            <div class="pool-stat-row">
              <span class="stat-label">Dedup Ratio</span>
              <span class="stat-value mono">{{ pool.dedupRatio ? `${pool.dedupRatio}x` : '1.00x' }}</span>
            </div>
            <div class="pool-stat-row">
              <span class="stat-label">Scrub</span>
              <span class="stat-value mono stat-truncate" :title="pool.lastScrubStatus || 'No scrub errors recorded'">
                {{ pool.lastScrubStatus || 'No errors' }}
              </span>
            </div>
            <div class="pool-stat-row">
              <span class="stat-label">Mount</span>
              <span class="stat-value mono stat-truncate" :title="pool.mountpoint || `/mnt/${pool.poolName || pool.name.replace(/^zfsPools\//, '')}`">
                {{ pool.mountpoint || `/mnt/${pool.poolName || pool.name.replace(/^zfsPools\//, '')}` }}
              </span>
            </div>
          </div>

          <!-- Vertical Subtle Divider -->
          <div class="pool-col-divider"></div>

          <!-- Right Half: Pie / Donut Chart with Usage/Frag Toggle -->
          <div class="pool-chart-col">
            <div class="chart-header-row">
              <div class="mini-pill-toggle">
                <button
                  type="button"
                  class="pill-btn"
                  :class="{ active: getPoolChartMode(pool.name) === 'usage' }"
                  @click="setPoolChartMode(pool.name, 'usage')"
                >
                  Usage
                </button>
                <button
                  type="button"
                  class="pill-btn"
                  :class="{ active: getPoolChartMode(pool.name) === 'frag' }"
                  @click="setPoolChartMode(pool.name, 'frag')"
                >
                  Frag
                </button>
              </div>
            </div>

            <div style="width: 100%; display: flex; flex-direction: column; align-items: center">
              <EChart
                v-if="getPoolChartMode(pool.name) === 'usage'"
                :options="getPoolUsageChartOptions(pool.allocatedBytes || 0, pool.freeBytes || 0, pool.sizeBytes || 0)"
                height="115px"
              />
              <EChart
                v-else
                :options="getPoolFragChartOptions(pool.fragmentationPercent || 0)"
                height="115px"
              />

              <div v-if="getPoolChartMode(pool.name) === 'usage'" class="pool-chart-legend">
                <span class="legend-item">
                  <span class="dot" style="background: var(--accent-primary)"></span>
                  {{ formatBytes(pool.allocatedBytes) }}
                </span>
                <span class="legend-item">
                  <span class="dot" style="background: var(--border-strong)"></span>
                  {{ formatBytes(pool.freeBytes) }}
                </span>
              </div>
              <div v-else class="pool-chart-legend">
                <span class="legend-item">
                  <span
                    class="dot"
                    :style="{ background: (pool.fragmentationPercent || 0) > 30 ? 'var(--warning)' : 'var(--info)' }"
                  ></span>
                  {{ pool.fragmentationPercent || 0 }}%
                </span>
                <span class="legend-item">
                  <span class="dot" style="background: var(--success)"></span>
                  {{ 100 - (pool.fragmentationPercent || 0) }}%
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pools-widget {
  width: 100%;
}

.pools-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.pool-cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 14px;
}

.pool-telemetry-card {
  display: flex;
  flex-direction: column;
  padding: 14px 16px;
}

.pool-compact-body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 1px 180px;
  gap: 14px;
  align-items: center;
}

.pool-info-col {
  display: flex;
  flex-direction: column;
  gap: 5px;
  min-width: 0;
}

.pool-stat-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 11.5px;
  gap: 8px;
}

.stat-label {
  color: var(--text-secondary);
  font-size: 11px;
  white-space: nowrap;
}

.stat-value {
  font-weight: 600;
  color: var(--text-primary);
  font-size: 11.5px;
  text-align: right;
}

.stat-truncate {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 170px;
}

.pool-col-divider {
  width: 1px;
  align-self: stretch;
  background: var(--border-subtle);
}

.pool-chart-col {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-width: 170px;
}

.chart-header-row {
  display: flex;
  justify-content: center;
  width: 100%;
  margin-bottom: 2px;
}

.mini-pill-toggle {
  display: inline-flex;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  padding: 1px;
  gap: 2px;
}

.pill-btn {
  background: transparent;
  border: none;
  border-radius: 10px;
  padding: 1px 8px;
  font-size: 10.5px;
  font-weight: 500;
  color: var(--text-muted);
  cursor: pointer;
  transition: all 0.15s ease;
}

.pill-btn:hover {
  color: var(--text-primary);
}

.pill-btn.active {
  background: var(--bg-surface);
  color: var(--accent-primary);
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.2);
}

.pool-chart-legend {
  display: flex;
  justify-content: center;
  gap: 10px;
  margin-top: 2px;
  font-size: 11px;
  font-family: var(--font-mono, monospace);
}

.legend-item {
  display: flex;
  align-items: center;
  gap: 5px;
  color: var(--text-secondary);
}

.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  display: inline-block;
}

@media (max-width: 580px) {
  .pool-compact-body {
    grid-template-columns: 1fr;
    gap: 12px;
  }
  .pool-col-divider {
    display: none;
  }
}
</style>
