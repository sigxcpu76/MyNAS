<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(
  defineProps<{
    capacityPercent: number;
    fragmentationPercent: number;
    size?: number;
  }>(),
  {
    size: 140,
  }
);

// Geometry
const center = 70;
const outerRadius = 56;
const innerRadius = 42;
const outerCircumference = 2 * Math.PI * outerRadius; // ~351.86
const innerCircumference = 2 * Math.PI * innerRadius; // ~263.89

const capClamped = computed(() => Math.min(100, Math.max(0, props.capacityPercent || 0)));
const fragClamped = computed(() => Math.min(100, Math.max(0, props.fragmentationPercent || 0)));

const outerDashOffset = computed(
  () => outerCircumference * (1 - capClamped.value / 100)
);
const innerDashOffset = computed(
  () => innerCircumference * (1 - fragClamped.value / 100)
);

// Colors
const capacityColor = computed(() => {
  if (capClamped.value >= 90) return 'var(--danger)';
  if (capClamped.value >= 75) return 'var(--warning)';
  return 'var(--accent-primary)';
});

const fragmentationColor = computed(() => {
  if (fragClamped.value >= 60) return 'var(--warning)';
  if (fragClamped.value >= 30) return 'var(--accent-secondary)';
  return 'var(--purple-text)';
});
</script>

<template>
  <div class="zfs-gauge-wrapper" :style="{ width: `${size}px` }">
    <svg :viewBox="`0 0 140 140`" class="zfs-gauge-svg">
      <!-- Background Tracks -->
      <circle
        :cx="center"
        :cy="center"
        :r="outerRadius"
        fill="none"
        stroke="var(--gauge-track)"
        stroke-width="9"
      />
      <circle
        :cx="center"
        :cy="center"
        :r="innerRadius"
        fill="none"
        stroke="var(--gauge-track)"
        stroke-width="8"
      />

      <!-- Animated Value Rings (Rotated -90deg to start at 12 o'clock) -->
      <g :transform="`rotate(-90 ${center} ${center})`">
        <!-- Outer Ring: Capacity / Usage -->
        <circle
          :cx="center"
          :cy="center"
          :r="outerRadius"
          fill="none"
          :stroke="capacityColor"
          stroke-width="9"
          stroke-linecap="round"
          :stroke-dasharray="outerCircumference"
          :stroke-dashoffset="outerDashOffset"
          class="gauge-ring"
        />

        <!-- Inner Ring: Fragmentation -->
        <circle
          :cx="center"
          :cy="center"
          :r="innerRadius"
          fill="none"
          :stroke="fragmentationColor"
          stroke-width="8"
          stroke-linecap="round"
          :stroke-dasharray="innerCircumference"
          :stroke-dashoffset="innerDashOffset"
          class="gauge-ring"
        />
      </g>

      <!-- Center Text -->
      <text
        :x="center"
        :y="center - 4"
        text-anchor="middle"
        dominant-baseline="central"
        class="gauge-text-val"
      >
        {{ capClamped }}%
      </text>
      <text
        :x="center"
        :y="center + 14"
        text-anchor="middle"
        dominant-baseline="central"
        class="gauge-text-label"
      >
        USED
      </text>
      <text
        :x="center"
        :y="center + 27"
        text-anchor="middle"
        dominant-baseline="central"
        class="gauge-text-frag"
        :style="{ fill: fragmentationColor }"
      >
        {{ fragClamped }}% FRAG
      </text>
    </svg>

    <!-- Legend -->
    <div class="gauge-legend">
      <div class="legend-item" title="Capacity / Allocation">
        <span class="legend-dot" :style="{ background: capacityColor }"></span>
        <span class="legend-label">Usage:</span>
        <span class="legend-val mono">{{ capClamped }}%</span>
      </div>
      <div class="legend-item" title="ZFS Pool Fragmentation">
        <span class="legend-dot" :style="{ background: fragmentationColor }"></span>
        <span class="legend-label">Frag:</span>
        <span class="legend-val mono">{{ fragClamped }}%</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.zfs-gauge-wrapper {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  user-select: none;
}

.zfs-gauge-svg {
  width: 100%;
  height: auto;
  overflow: visible;
}

.gauge-ring {
  transition: stroke-dashoffset 0.6s cubic-bezier(0.4, 0, 0.2, 1), stroke 0.3s ease;
}

.gauge-text-val {
  font-family: inherit;
  font-size: 20px;
  font-weight: 800;
  fill: var(--text-primary);
}

.gauge-text-label {
  font-family: inherit;
  font-size: 8.5px;
  font-weight: 700;
  letter-spacing: 0.8px;
  fill: var(--text-secondary);
}

.gauge-text-frag {
  font-family: inherit;
  font-size: 9.5px;
  font-weight: 700;
  letter-spacing: 0.4px;
}

.gauge-legend {
  display: flex;
  justify-content: center;
  gap: 12px;
  margin-top: 6px;
  font-size: 11px;
}

.legend-item {
  display: flex;
  align-items: center;
  gap: 4px;
}

.legend-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
  flex-shrink: 0;
}

.legend-label {
  color: var(--text-secondary);
}

.legend-val {
  color: var(--text-primary);
  font-weight: 600;
}
</style>
