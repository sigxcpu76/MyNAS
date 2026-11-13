<script setup lang="ts">
import { ref } from 'vue';
import type * as echarts from 'echarts';
import { formatBytes, type ZfsPool } from '../../../domain/models';
import { nasState, selectResource } from '../../store';
import { navigateTo } from '../../router';
import { useChartPalette } from '../chartPalette';
import EChart from '../../components/EChart.vue';
import SkeletonBox from '../../components/SkeletonBox.vue';

const chartPalette = useChartPalette();
const poolChartModes = ref<Record<string, 'usage' | 'frag'>>({});

function openPool(pool: ZfsPool) {
  const clean = pool.poolName || pool.name.replace(/^zfsPools\//, '');
  navigateTo('storage-pools', false, `pool:${clean}`);
}

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
        top: '26%',
        textStyle: {
          color: p.textColor,
          fontSize: 13,
          fontWeight: 'bold',
        },
        subtextStyle: {
          color: p.subtextColor,
          fontSize: 9,
        },
      },
      series: [
        {
          name: 'Pool Usage',
          type: 'pie',
          radius: ['56%', '84%'],
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
      top: '26%',
      textStyle: {
        color: fragColor,
        fontSize: 13,
        fontWeight: 'bold',
      },
      subtextStyle: {
        color: p.subtextColor,
        fontSize: 9,
      },
    },
    series: [
      {
        name: 'Fragmentation',
        type: 'pie',
        radius: ['56%', '84%'],
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
      <button class="btn btn-primary" @click="navigateTo('storage-pools')">Configure Pools</button>
    </div>

    <div v-else class="pool-cards-grid">
      <div
        v-for="pool in nasState.pools"
        :key="pool.name"
        class="card pool-telemetry-card"
      >
        <!-- Pool Card Header -->
        <div
          class="card-header"
          style="border-bottom: 1px solid var(--border-subtle); padding-bottom: 6px; margin-bottom: 8px; cursor: pointer"
          @click="openPool(pool)"
        >
          <div style="display: flex; align-items: center; gap: 6px; min-width: 0">
            <strong class="mono" style="font-size: 14px; color: var(--accent-primary)">
              {{ pool.poolName || pool.name.replace(/^zfsPools\//, '') }}
            </strong>
            <span class="mono font-xs" style="color: var(--text-secondary); opacity: 0.85">{{ pool.vdevLayout || 'stripe' }}</span>
          </div>
          <span
            class="badge mono"
            :class="pool.state === 'ONLINE' ? 'badge-success' : 'badge-warning'"
            style="font-size: 10px; font-weight: 600; padding: 1px 6px"
          >
            {{ pool.state }}
          </span>
        </div>

        <!-- Pool Compact Body: Left Info, Right Mini Donut -->
        <div class="pool-compact-body">
          <div class="pool-info-col">
            <div class="pool-cap-line mono">
              <span style="color: var(--accent-primary); font-weight: 600">{{ formatBytes(pool.allocatedBytes) }}</span>
              <span style="color: var(--text-muted)"> / {{ formatBytes(pool.sizeBytes) }}</span>
              <span class="pool-free-chip">{{ formatBytes(pool.freeBytes) }} free</span>
            </div>

            <div class="pool-meta-chips mono">
              <span
                class="meta-chip"
                :style="{ color: (pool.fragmentationPercent || 0) > 40 ? 'var(--danger)' : (pool.fragmentationPercent || 0) > 20 ? 'var(--warning)' : 'var(--success)' }"
              >
                Frag {{ pool.fragmentationPercent || 0 }}%
              </span>
              <span class="meta-chip">
                Dedup {{ pool.dedupRatio ? `${pool.dedupRatio}x` : '1x' }}
              </span>
            </div>

            <div class="pool-stat-row">
              <span class="stat-label">Mount</span>
              <span class="stat-value mono stat-truncate" :title="pool.mountpoint || `/mnt/${pool.poolName || pool.name.replace(/^zfsPools\//, '')}`">
                {{ pool.mountpoint || `/mnt/${pool.poolName || pool.name.replace(/^zfsPools\//, '')}` }}
              </span>
            </div>
            <div class="pool-stat-row">
              <span class="stat-label">Scrub</span>
              <span class="stat-value mono stat-truncate" :title="pool.lastScrubStatus || 'No scrub errors recorded'">
                {{ pool.lastScrubStatus || 'No errors' }}
              </span>
            </div>
          </div>

          <div class="pool-chart-col">
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

            <EChart
              v-if="getPoolChartMode(pool.name) === 'usage'"
              :options="getPoolUsageChartOptions(pool.allocatedBytes || 0, pool.freeBytes || 0, pool.sizeBytes || 0)"
              height="84px"
            />
            <EChart
              v-else
              :options="getPoolFragChartOptions(pool.fragmentationPercent || 0)"
              height="84px"
            />
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

.pool-cards-grid {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.pool-telemetry-card {
  display: flex;
  flex-direction: column;
  padding: 10px 12px;
}

.pool-compact-body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 104px;
  gap: 10px;
  align-items: center;
}

.pool-info-col {
  display: flex;
  flex-direction: column;
  gap: 5px;
  min-width: 0;
}

.pool-cap-line {
  font-size: 12px;
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 4px;
}

.pool-free-chip {
  font-size: 10.5px;
  color: var(--text-secondary);
  margin-left: auto;
}

.pool-meta-chips {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.meta-chip {
  font-size: 10.5px;
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
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
  gap: 2px;
  width: 104px;
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
  border: 1px solid var(--border-subtle);
  box-shadow: none;
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
