<script setup lang="ts">
import { computed } from 'vue';
import {
  mdiPower,
  mdiDatabaseCheck,
  mdiConsole,
  mdiInformationOutline,
  mdiLogout,
  mdiCellphone,
  mdiMonitor,
  mdiRefresh,
} from '@mdi/js';
import MdiIcon from '../../components/MdiIcon.vue';
import BottomSheet from '../../components/BottomSheet.vue';
import { nasState, refreshAllData } from '../../store';
import { useDevice, type DeviceViewMode } from '../../composables/useDevice';
import { useClientApp } from '../../composables/useClientApp';

const props = defineProps<{
  modelValue: boolean;
}>();

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void;
  (e: 'open-terminal'): void;
  (e: 'open-system-details'): void;
  (e: 'open-pool-modal'): void;
  (e: 'logout'): void;
}>();

const { viewMode, setViewMode } = useDevice();
const { clientApp } = useClientApp();

const hostname = computed(() => nasState.systemTelemetry?.host?.hostname || 'NaN Appliance');
const uptime = computed(() => {
  const s = nasState.systemTelemetry?.host?.uptimeSeconds || 0;
  const days = Math.floor(s / 86400);
  const hours = Math.floor((s % 86400) / 3600);
  const mins = Math.floor((s % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
});

function handleTerminal() {
  emit('update:modelValue', false);
  emit('open-terminal');
}

function handleDetails() {
  emit('update:modelValue', false);
  emit('open-system-details');
}

function handlePool() {
  emit('update:modelValue', false);
  emit('open-pool-modal');
}

function handleLogout() {
  emit('update:modelValue', false);
  emit('logout');
}

function selectMode(mode: DeviceViewMode) {
  setViewMode(mode);
}
</script>

<template>
  <BottomSheet
    :model-value="modelValue"
    title="System & Power"
    :subtitle="`${hostname} • Uptime ${uptime}`"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <div class="power-sheet-content">
      <!-- Quick Status Card -->
      <div class="status-summary-box">
        <div class="status-pill status-online">
          <span class="pulse-dot" />
          <span>Host Online</span>
        </div>
        <div class="host-specs">
          <span>OS: {{ nasState.systemTelemetry?.host?.osName || 'Debian / Proxmox' }}</span>
          <span>Kernel: {{ nasState.systemTelemetry?.host?.kernelVersion || '-' }}</span>
          <span class="mono" :style="{ color: clientApp.isNative ? '#4ade80' : 'inherit' }">Client: {{ clientApp.label }}</span>
        </div>
      </div>

      <!-- Quick Actions Grid -->
      <div class="action-tiles-grid">
        <button class="action-tile" @click="handleTerminal">
          <div class="tile-icon" style="background: rgba(56, 189, 248, 0.15); color: #38bdf8">
            <MdiIcon :path="mdiConsole" size="22" />
          </div>
          <span class="tile-label">Host Shell</span>
          <span class="tile-sub">Terminal</span>
        </button>

        <button class="action-tile" @click="handlePool">
          <div class="tile-icon" style="background: rgba(168, 85, 247, 0.15); color: #c084fc">
            <MdiIcon :path="mdiDatabaseCheck" size="22" />
          </div>
          <span class="tile-label">Main Pool</span>
          <span class="tile-sub">{{ nasState.systemConfig?.mainPool?.replace(/^zfsPools\//, '') || 'Configure' }}</span>
        </button>

        <button class="action-tile" @click="handleDetails">
          <div class="tile-icon" style="background: rgba(74, 222, 128, 0.15); color: #4ade80">
            <MdiIcon :path="mdiInformationOutline" size="22" />
          </div>
          <span class="tile-label">Specs</span>
          <span class="tile-sub">Hardware</span>
        </button>

        <button class="action-tile" @click="refreshAllData">
          <div class="tile-icon" style="background: rgba(251, 191, 36, 0.15); color: #fbbf24">
            <MdiIcon :path="mdiRefresh" size="22" :class="{ 'spin-icon': nasState.loading }" />
          </div>
          <span class="tile-label">Refresh</span>
          <span class="tile-sub">Telemetry</span>
        </button>
      </div>

      <!-- Viewport Mode Preference -->
      <div class="section-card">
        <div class="section-card-title">View Mode Preference</div>
        <div class="viewmode-toggle-group">
          <button
            class="viewmode-btn"
            :class="{ active: viewMode === 'auto' }"
            @click="selectMode('auto')"
          >
            Auto (Screen)
          </button>
          <button
            class="viewmode-btn"
            :class="{ active: viewMode === 'mobile' }"
            @click="selectMode('mobile')"
          >
            <MdiIcon :path="mdiCellphone" size="14" style="margin-right: 4px" />
            Mobile
          </button>
          <button
            class="viewmode-btn"
            :class="{ active: viewMode === 'desktop' }"
            @click="selectMode('desktop')"
          >
            <MdiIcon :path="mdiMonitor" size="14" style="margin-right: 4px" />
            Desktop
          </button>
        </div>
      </div>

      <!-- Account & Sign Out -->
      <div v-if="nasState.authenticated && nasState.currentUser" class="account-card">
        <div class="account-info">
          <div class="account-name">{{ nasState.currentUser.fullName || nasState.currentUser.username }}</div>
          <div class="account-role">{{ nasState.currentUser.isAdmin ? 'Administrator' : 'Standard User' }}</div>
        </div>
        <button class="btn btn-sm btn-danger" @click="handleLogout">
          <MdiIcon :path="mdiLogout" size="15" style="margin-right: 4px" />
          Sign Out
        </button>
      </div>
    </div>
  </BottomSheet>
</template>

<style scoped>
.power-sheet-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.status-summary-box {
  background: var(--bg-surface-elevated, #243248);
  border: 1px solid var(--border-color, #334155);
  border-radius: 12px;
  padding: 12px 14px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--accent-success, #4ade80);
}

.pulse-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--accent-success, #4ade80);
  box-shadow: 0 0 8px var(--accent-success, #4ade80);
}

.host-specs {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
}

.action-tiles-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
}

.action-tile {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 12px 6px;
  background: var(--bg-surface-elevated, #243248);
  border: 1px solid var(--border-color, #334155);
  border-radius: 12px;
  color: var(--text-primary, #f8fafc);
  cursor: pointer;
  touch-action: manipulation;
  transition: transform 0.15s ease, background 0.15s ease;
}

.action-tile:active {
  transform: scale(0.96);
  background: var(--bg-hover, #334155);
}

.tile-icon {
  width: 42px;
  height: 42px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 6px;
}

.tile-label {
  font-size: 12px;
  font-weight: 600;
  text-align: center;
}

.tile-sub {
  font-size: 10px;
  color: var(--text-secondary, #94a3b8);
  text-align: center;
  margin-top: 1px;
  max-width: 65px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.section-card {
  background: var(--bg-surface-elevated, #243248);
  border: 1px solid var(--border-color, #334155);
  border-radius: 12px;
  padding: 12px 14px;
}

.section-card-title {
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--text-secondary, #94a3b8);
  margin-bottom: 8px;
}

.viewmode-toggle-group {
  display: flex;
  background: var(--bg-base, #0f172a);
  border-radius: 8px;
  padding: 3px;
  gap: 3px;
}

.viewmode-btn {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 6px 8px;
  font-size: 11px;
  font-weight: 600;
  background: transparent;
  border: none;
  border-radius: 6px;
  color: var(--text-secondary, #94a3b8);
  cursor: pointer;
  transition: all 0.15s ease;
}

.viewmode-btn.active {
  background: var(--accent-primary, #38bdf8);
  color: #0f172a;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
}

.account-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--bg-surface-elevated, #243248);
  border: 1px solid var(--border-color, #334155);
  border-radius: 12px;
  padding: 12px 14px;
}

.account-name {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
}

.account-role {
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
}
</style>
