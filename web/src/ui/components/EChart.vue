<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, computed } from 'vue';
import * as echarts from 'echarts';
import { activeThemeId, isDarkMode } from '../theme';

const props = withDefaults(
  defineProps<{
    options: echarts.EChartsOption;
    height?: string;
    loading?: boolean;
  }>(),
  {
    height: '200px',
    loading: false,
  }
);

const chartRef = ref<HTMLDivElement | null>(null);
let chartInstance: echarts.ECharts | null = null;
let resizeObserver: ResizeObserver | null = null;

function isCanvasSupported(): boolean {
  if (typeof document === 'undefined') return false;
  // In jsdom test environment, skip canvas to prevent not-implemented warnings
  if (typeof navigator !== 'undefined' && navigator.userAgent && navigator.userAgent.includes('jsdom')) {
    return false;
  }
  try {
    const canvas = document.createElement('canvas');
    return Boolean(canvas.getContext && canvas.getContext('2d'));
  } catch {
    return false;
  }
}

function initChart() {
  if (!chartRef.value || !isCanvasSupported()) return;

  if (chartInstance) {
    chartInstance.dispose();
    chartInstance = null;
  }

  try {
    chartInstance = echarts.init(chartRef.value, isDarkMode.value ? 'dark' : undefined, {
      renderer: 'canvas',
    });

    if (props.options) {
      chartInstance.setOption(props.options, true);
    }

    if (props.loading) {
      chartInstance.showLoading();
    }
  } catch (err) {
    // Graceful fallback for headless or test environments lacking canvas 2d context
  }
}

onMounted(() => {
  initChart();

  if (chartRef.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => {
      chartInstance?.resize();
    });
    resizeObserver.observe(chartRef.value);
  }
});

watch(
  () => props.options,
  (newOpts) => {
    if (chartInstance && newOpts) {
      chartInstance.setOption(newOpts, true);
    }
  },
  { deep: true }
);

watch(
  () => props.loading,
  (isLoading) => {
    if (!chartInstance) return;
    if (isLoading) {
      chartInstance.showLoading();
    } else {
      chartInstance.hideLoading();
    }
  }
);

watch(activeThemeId, () => {
  // Re-initialize chart with new theme mode and styles
  initChart();
});

onBeforeUnmount(() => {
  if (resizeObserver) {
    resizeObserver.disconnect();
    resizeObserver = null;
  }
  if (chartInstance) {
    chartInstance.dispose();
    chartInstance = null;
  }
});
</script>

<template>
  <div ref="chartRef" :style="{ width: '100%', height }" class="echart-container"></div>
</template>

<style scoped>
.echart-container {
  min-height: 120px;
}
</style>
