<script setup lang="ts">
import { computed, ref } from 'vue';
import {
  mdiFlash,
  mdiThermometer,
  mdiFan,
  mdiExpansionCard,
  mdiVideo3dVariant,
  mdiCpu64Bit,
  mdiInformationOutline,
  mdiAlertCircle,
  mdiHarddisk,
} from '@mdi/js';
import { formatBytes } from '../../../domain/models';
import type { HardwareSensorItem, GpuTelemetry } from '../../../domain/models';
import { nasState } from '../../store';
import MdiIcon from '../../components/MdiIcon.vue';

const telemetry = computed(() => nasState.systemTelemetry);
const sensors = computed(() => telemetry.value?.sensors);
const powerMeter = computed(() => sensors.value?.powerMeter);
const gpus = computed<GpuTelemetry[]>(() => sensors.value?.gpus || []);
const temperatures = computed<HardwareSensorItem[]>(() => sensors.value?.temperatures || []);
const fans = computed<HardwareSensorItem[]>(() => sensors.value?.fans || []);
const lmSensorsInstalled = computed(() => sensors.value?.lmSensorsInstalled ?? false);

// Active category filter for temperatures
const selectedCategory = ref<string>('ALL');

const categories = computed(() => {
  const set = new Set<string>();
  temperatures.value.forEach((t) => {
    if (t.category && t.category !== 'POWER' && t.category !== 'FAN') {
      set.add(t.category);
    }
  });
  return ['ALL', ...Array.from(set).sort()];
});

const filteredTemperatures = computed(() => {
  const list = temperatures.value.filter((t) => t.category !== 'POWER' && t.category !== 'FAN');
  if (selectedCategory.value === 'ALL') {
    return list;
  }
  return list.filter((t) => t.category === selectedCategory.value);
});

function getTempBadgeClass(temp: number): string {
  if (temp <= 0) return 'badge-neutral';
  if (temp < 60) return 'badge-success';
  if (temp < 80) return 'badge-warning';
  return 'badge-danger';
}

function getCategoryIcon(cat: string): string {
  switch (cat) {
    case 'CPU':
      return mdiCpu64Bit;
    case 'GPU':
      return mdiVideo3dVariant;
    case 'PCI':
      return mdiExpansionCard;
    case 'STORAGE':
      return mdiHarddisk;
    default:
      return mdiThermometer;
  }
}
</script>

<template>
  <div class="card sensors-widget-card">
    <!-- Header -->
    <div class="card-header sensors-header">
      <div style="display: flex; align-items: center; gap: 8px">
        <MdiIcon :path="mdiThermometer" size="20" style="color: var(--accent-primary)" />
        <h3 class="card-title">Hardware Sensors & Power</h3>
      </div>
      <div class="header-badges">
        <span
          v-if="powerMeter && powerMeter.powerWatts > 0"
          class="badge badge-warning power-pill"
          :title="`Adapter: ${powerMeter.adapter} • Device: ${powerMeter.deviceName}`"
        >
          <MdiIcon :path="mdiFlash" size="14" style="margin-right: 3px;" />
          <strong>{{ powerMeter.powerWatts.toFixed(1) }} W</strong>
        </span>
        <span
          v-if="lmSensorsInstalled"
          class="badge badge-success"
          title="lm-sensors is active on this system"
        >
          lm-sensors active
        </span>
        <span
          v-else
          class="badge badge-neutral"
          title="Install lm-sensors package for full motherboard & PCI sensor support"
        >
          lm-sensors not detected
        </span>
      </div>
    </div>

    <!-- Power Meter Card (if present) -->
    <div v-if="powerMeter && powerMeter.powerWatts > 0" class="power-meter-banner">
      <div class="power-icon-wrapper">
        <MdiIcon :path="mdiFlash" size="28" style="color: #f59e0b" />
      </div>
      <div class="power-details">
        <div class="power-title">
          <span>System Power Meter</span>
          <span class="mono power-adapter">({{ powerMeter.adapter }})</span>
        </div>
        <div class="power-meta">
          Device: <code class="mono">{{ powerMeter.deviceName }}</code>
          <span v-if="powerMeter.intervalSeconds > 0"> • Interval: {{ powerMeter.intervalSeconds }}s</span>
        </div>
      </div>
      <div class="power-value-box">
        <span class="power-number mono">{{ powerMeter.powerWatts.toFixed(1) }}</span>
        <span class="power-unit">Watts</span>
      </div>
    </div>

    <!-- Dedicated GPUs Section (NVIDIA / AMD) -->
    <div v-if="gpus.length > 0" class="gpus-section">
      <div class="section-title">
        <MdiIcon :path="mdiVideo3dVariant" size="16" style="color: var(--accent-primary)" />
        <span>Dedicated GPU Accelerators</span>
        <span class="badge badge-neutral mono">{{ gpus.length }} {{ gpus.length === 1 ? 'GPU' : 'GPUs' }}</span>
      </div>
      <div class="gpus-grid">
        <div v-for="gpu in gpus" :key="gpu.index" class="gpu-card">
          <div class="gpu-header">
            <div style="display: flex; align-items: center; gap: 6px">
              <strong style="font-size: 13px">{{ gpu.name }}</strong>
              <span class="badge badge-info mono" style="font-size: 10px; padding: 1px 5px">
                {{ gpu.vendor }}
              </span>
            </div>
            <span
              v-if="gpu.temperatureCelsius > 0"
              class="badge mono"
              :class="getTempBadgeClass(gpu.temperatureCelsius)"
            >
              {{ gpu.temperatureCelsius.toFixed(1) }}°C
            </span>
          </div>

          <div class="gpu-metrics">
            <div class="gpu-metric-row">
              <span class="metric-label">Utilization:</span>
              <div class="metric-bar-wrapper">
                <div class="progress-track" style="height: 6px; flex: 1">
                  <div
                    class="progress-fill"
                    :style="{
                      width: `${Math.min(100, Math.max(0, gpu.utilizationPercent))}%`,
                      background: gpu.utilizationPercent > 85 ? 'var(--danger)' : 'var(--accent-primary)'
                    }"
                  ></div>
                </div>
                <span class="mono metric-val" style="width: 42px; text-align: right">
                  {{ gpu.utilizationPercent.toFixed(0) }}%
                </span>
              </div>
            </div>

            <div v-if="gpu.memoryTotalBytes > 0" class="gpu-metric-row">
              <span class="metric-label">VRAM:</span>
              <div class="metric-bar-wrapper">
                <div class="progress-track" style="height: 6px; flex: 1">
                  <div
                    class="progress-fill"
                    :style="{
                      width: `${Math.min(100, Math.max(0, (gpu.memoryUsedBytes / gpu.memoryTotalBytes) * 100))}%`,
                      background: 'var(--info)'
                    }"
                  ></div>
                </div>
                <span class="mono metric-val" style="font-size: 11px">
                  {{ formatBytes(gpu.memoryUsedBytes) }} / {{ formatBytes(gpu.memoryTotalBytes) }}
                </span>
              </div>
            </div>

            <div class="gpu-sub-stats">
              <span v-if="gpu.powerWatts > 0" class="sub-stat">
                Power: <strong class="mono">{{ gpu.powerWatts.toFixed(1) }} W</strong>
              </span>
              <span v-if="gpu.fanSpeedPercent !== undefined && gpu.fanSpeedPercent >= 0" class="sub-stat">
                Fan: <strong class="mono">{{ gpu.fanSpeedPercent }}%</strong>
              </span>
              <span v-if="gpu.driverVersion" class="sub-stat mono" style="color: var(--text-muted)">
                Driver: {{ gpu.driverVersion }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Temperature Sensors Grid -->
    <div class="temps-section">
      <div class="section-header-bar">
        <div class="section-title">
          <MdiIcon :path="mdiThermometer" size="16" style="color: var(--accent-primary)" />
          <span>Component Temperatures</span>
          <span class="badge badge-neutral mono">{{ filteredTemperatures.length }} Sensors</span>
        </div>
        <!-- Category Filter Tabs -->
        <div v-if="categories.length > 2" class="category-tabs">
          <button
            v-for="cat in categories"
            :key="cat"
            class="tab-btn"
            :class="{ active: selectedCategory === cat }"
            @click="selectedCategory = cat"
          >
            {{ cat }}
          </button>
        </div>
      </div>

      <div v-if="filteredTemperatures.length === 0" class="empty-hint">
        <span v-if="lmSensorsInstalled">No thermal readings currently reported for this category.</span>
        <span v-else>
          Install <code class="mono">lm-sensors</code> (<code class="mono">sudo apt install lm-sensors</code>) to view all motherboard, expansion card, and storage sensors.
        </span>
      </div>

      <div v-else class="sensors-grid">
        <div
          v-for="item in filteredTemperatures"
          :key="`${item.chip}-${item.label}`"
          class="sensor-item-tile"
        >
          <div class="sensor-icon-col">
            <MdiIcon :path="getCategoryIcon(item.category)" size="16" style="color: var(--text-secondary)" />
          </div>
          <div class="sensor-info-col">
            <div class="sensor-label-row">
              <span class="sensor-name" :title="item.label">{{ item.label }}</span>
              <span v-if="item.alarm" class="badge badge-danger" title="Sensor Threshold Alarm">ALARM</span>
            </div>
            <div class="sensor-sub-row">
              <span class="sensor-chip mono" :title="item.chip">{{ item.chip }}</span>
              <span v-if="item.adapter" class="sensor-adapter mono">{{ item.adapter }}</span>
            </div>
          </div>
          <div class="sensor-val-col">
            <span class="badge mono temp-badge" :class="getTempBadgeClass(item.currentValue)">
              {{ item.currentValue.toFixed(1) }}{{ item.unit || '°C' }}
            </span>
            <span v-if="item.critThreshold && item.critThreshold > 0 && item.critThreshold < 150" class="sensor-crit mono">
              Crit: {{ item.critThreshold.toFixed(0) }}°C
            </span>
          </div>
        </div>
      </div>
    </div>

    <!-- System Fans Section (if present) -->
    <div v-if="fans.length > 0" class="fans-section">
      <div class="section-title">
        <MdiIcon :path="mdiFan" size="16" style="color: var(--accent-primary)" />
        <span>Chassis & Cooling Fans</span>
        <span class="badge badge-neutral mono">{{ fans.length }} Fans</span>
      </div>
      <div class="fans-grid">
        <div v-for="fan in fans" :key="`${fan.chip}-${fan.label}`" class="fan-tile">
          <div class="fan-meta">
            <MdiIcon :path="mdiFan" size="16" :class="{ 'fan-spinning': fan.currentValue > 0 }" />
            <span class="fan-label">{{ fan.label }}</span>
          </div>
          <div class="fan-rpm">
            <strong class="mono" :style="{ color: fan.currentValue > 0 ? 'var(--accent-primary)' : 'var(--text-muted)' }">
              {{ fan.currentValue > 0 ? `${Math.round(fan.currentValue)} RPM` : 'Stopped' }}
            </strong>
          </div>
        </div>
      </div>
    </div>

    <!-- lm-sensors installation hint (if not installed) -->
    <div v-if="!lmSensorsInstalled" class="lm-hint-card">
      <MdiIcon :path="mdiInformationOutline" size="18" style="color: var(--accent-primary); flex-shrink: 0" />
      <div class="lm-hint-text">
        <strong>Hardware Sensors Integration</strong>: Install <code class="mono">lm-sensors</code> on the host system to automatically unlock full ACPI power meters, expansion card temperatures, and fan monitoring.
      </div>
    </div>
  </div>
</template>

<style scoped>
.sensors-widget-card {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.sensors-header {
  border-bottom: 1px solid var(--border-subtle);
  padding-bottom: 8px;
  margin-bottom: 2px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px;
}

.header-badges {
  display: flex;
  align-items: center;
  gap: 8px;
}

.power-pill {
  display: inline-flex;
  align-items: center;
  font-size: 12px;
  padding: 3px 8px;
}

/* Power Meter Banner */
.power-meter-banner {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 12px 16px;
  background: linear-gradient(135deg, rgba(245, 158, 11, 0.12) 0%, rgba(245, 158, 11, 0.04) 100%);
  border: 1px solid rgba(245, 158, 11, 0.35);
  border-radius: 8px;
}

.power-icon-wrapper {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border-radius: 8px;
  background: rgba(245, 158, 11, 0.15);
  flex-shrink: 0;
}

.power-details {
  flex: 1;
  min-width: 0;
}

.power-title {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--text-primary);
  display: flex;
  align-items: center;
  gap: 6px;
}

.power-adapter {
  font-size: 11px;
  color: var(--text-secondary);
}

.power-meta {
  font-size: 11.5px;
  color: var(--text-secondary);
  margin-top: 3px;
}

.power-value-box {
  text-align: right;
  flex-shrink: 0;
}

.power-number {
  font-size: 26px;
  font-weight: 700;
  color: #f59e0b;
  display: block;
  line-height: 1.1;
}

.power-unit {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--text-secondary);
}

/* Section titles */
.section-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 8px;
}

.section-header-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 8px;
}

.category-tabs {
  display: flex;
  align-items: center;
  gap: 4px;
  background: rgba(0, 0, 0, 0.15);
  padding: 2px;
  border-radius: 6px;
}

.tab-btn {
  border: none;
  background: transparent;
  color: var(--text-secondary);
  font-size: 10.5px;
  font-weight: 600;
  padding: 3px 8px;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.tab-btn.active {
  background: var(--accent-primary);
  color: #fff;
}

/* GPUs Grid */
.gpus-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 10px;
}

.gpu-card {
  padding: 10px 12px;
  background: rgba(0, 0, 0, 0.2);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.gpu-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.gpu-metrics {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.gpu-metric-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 11.5px;
}

.metric-label {
  color: var(--text-secondary);
  width: 65px;
  flex-shrink: 0;
}

.metric-bar-wrapper {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 6px;
}

.gpu-sub-stats {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  font-size: 11px;
  color: var(--text-secondary);
  margin-top: 2px;
}

/* Sensors Grid */
.sensors-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 8px;
}

.sensor-item-tile {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  background: rgba(0, 0, 0, 0.15);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
}

.sensor-icon-col {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  flex-shrink: 0;
}

.sensor-info-col {
  flex: 1;
  min-width: 0;
}

.sensor-label-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.sensor-name {
  font-size: 12px;
  font-weight: 500;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 140px;
}

.sensor-sub-row {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 10px;
  color: var(--text-muted);
  margin-top: 1px;
}

.sensor-chip {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100px;
}

.sensor-adapter {
  color: var(--text-muted);
}

.sensor-val-col {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  flex-shrink: 0;
}

.temp-badge {
  font-size: 11px;
  padding: 2px 6px;
}

.sensor-crit {
  font-size: 9.5px;
  color: var(--text-muted);
  margin-top: 2px;
}

/* Fans Grid */
.fans-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 8px;
}

.fan-tile {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 7px 10px;
  background: rgba(0, 0, 0, 0.15);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  font-size: 11.5px;
}

.fan-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-secondary);
}

.fan-spinning {
  color: var(--accent-primary);
  animation: fan-spin 2s linear infinite;
}

@keyframes fan-spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

.empty-hint {
  padding: 16px;
  text-align: center;
  font-size: 12px;
  color: var(--text-muted);
  background: rgba(0, 0, 0, 0.1);
  border-radius: 6px;
}

.lm-hint-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  background: rgba(56, 189, 248, 0.08);
  border: 1px solid rgba(56, 189, 248, 0.25);
  border-radius: 6px;
  font-size: 11.5px;
  color: var(--text-secondary);
}

.lm-hint-text code {
  color: var(--accent-primary);
  background: rgba(0, 0, 0, 0.25);
  padding: 1px 4px;
  border-radius: 3px;
}
</style>
