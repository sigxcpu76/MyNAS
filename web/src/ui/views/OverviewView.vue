<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue';
import {
  mdiCheck,
  mdiClose,
  mdiDrag,
  mdiMinus,
  mdiPlus,
  mdiRestore,
  mdiTune,
  mdiViewGridOutline,
} from '@mdi/js';
import { services } from '../../container';
import { confirmModal, nasState, refreshTelemetry } from '../store';
import OverviewSkeleton from '../components/OverviewSkeleton.vue';
import AddWidgetModal from '../dashboard/AddWidgetModal.vue';
import {
  DEFAULT_DASHBOARD_LAYOUT,
  BALANCED_DASHBOARD_LAYOUT,
  loadDashboardLayout,
  normalizeToGridLayout,
  syncPreferencesFromBackend,
  persistPreferencesToBackend,
  getWidgetCatalogItem,
  type DashboardWidgetConfig,
} from '../dashboard/dashboardConfig';
import {
  GRID_COLS,
  GRID_GAP,
  GRID_ROW_HEIGHT,
  compactLayout,
  findOpenSlot,
  moveGridItem,
  resizeGridItem,
} from '../dashboard/gridEngine';

// Modular Widgets
import WidgetKpis from '../dashboard/widgets/WidgetKpis.vue';
import WidgetGpu from '../dashboard/widgets/WidgetGpu.vue';
import WidgetPools from '../dashboard/widgets/WidgetPools.vue';
import WidgetCpu from '../dashboard/widgets/WidgetCpu.vue';
import WidgetMemory from '../dashboard/widgets/WidgetMemory.vue';
import WidgetSystem from '../dashboard/widgets/WidgetSystem.vue';
import WidgetNetwork from '../dashboard/widgets/WidgetNetwork.vue';
import WidgetDisks from '../dashboard/widgets/WidgetDisks.vue';
import WidgetDiskIo from '../dashboard/widgets/WidgetDiskIo.vue';
import WidgetZpoolIo from '../dashboard/widgets/WidgetZpoolIo.vue';
import WidgetWorkloads from '../dashboard/widgets/WidgetWorkloads.vue';
import WidgetVms from '../dashboard/widgets/WidgetVms.vue';
import WidgetApps from '../dashboard/widgets/WidgetApps.vue';
import WidgetWebApps from '../dashboard/widgets/WidgetWebApps.vue';
import WidgetShares from '../dashboard/widgets/WidgetShares.vue';
import WidgetSensors from '../dashboard/widgets/WidgetSensors.vue';

// ---------------------------------------------------------------------------
// Telemetry
// ---------------------------------------------------------------------------
const telemetry = computed(() => nasState.systemTelemetry);
export type { TrafficPoint } from '../store';
const trafficHistory = computed(() => nasState.networkTrafficHistory);
const gpuHistory = computed(() => nasState.gpuHistory);

let telemetryPollInterval: ReturnType<typeof setInterval> | null = null;

onMounted(() => {
  syncBackendPreferences();
  refreshTelemetry();
  telemetryPollInterval = setInterval(() => {
    if (!nasState.liveSyncActive) {
      refreshTelemetry();
    }
  }, 4000);
});

onBeforeUnmount(() => {
  if (telemetryPollInterval) {
    clearInterval(telemetryPollInterval);
    telemetryPollInterval = null;
  }
  cleanupPointerListeners();
});

// ---------------------------------------------------------------------------
// Grafana-Style Fluid 24-Column Grid State
// ---------------------------------------------------------------------------
const isEditMode = ref(false);
const showAddModal = ref(false);
const dashboardLayout = ref<DashboardWidgetConfig[]>(loadDashboardLayout());
const gridRef = ref<HTMLElement | null>(null);

async function syncBackendPreferences() {
  const remote = await syncPreferencesFromBackend(services);
  if (remote) {
    dashboardLayout.value = remote.layout;
  }
}

watch(
  () => nasState.userPreferences,
  (prefs) => {
    if (prefs?.dashboard && Array.isArray(prefs.dashboard.layout) && prefs.dashboard.layout.length > 0) {
      const normalized = normalizeToGridLayout(
        prefs.dashboard.layout,
        prefs.dashboard.columnCount || 3
      );
      if (normalized.length > 0) {
        dashboardLayout.value = normalized;
      }
    }
  },
  { deep: true }
);

const existingTypes = computed(() => dashboardLayout.value.map((w) => w.type));

function getWidgetComponent(type: string) {
  switch (type) {
    case 'kpis':
      return WidgetKpis;
    case 'gpu':
      return WidgetGpu;
    case 'pools':
      return WidgetPools;
    case 'cpu':
      return WidgetCpu;
    case 'memory':
      return WidgetMemory;
    case 'system':
      return WidgetSystem;
    case 'network':
      return WidgetNetwork;
    case 'disks':
      return WidgetDisks;
    case 'disk_io':
      return WidgetDiskIo;
    case 'zpool_io':
      return WidgetZpoolIo;
    case 'workloads':
      return WidgetWorkloads;
    case 'vms':
      return WidgetVms;
    case 'apps':
      return WidgetApps;
    case 'webapps':
      return WidgetWebApps;
    case 'shares':
      return WidgetShares;
    case 'sensors':
      return WidgetSensors;
    default:
      return null;
  }
}

function getWidgetName(type: string): string {
  return getWidgetCatalogItem(type)?.name || type;
}

function persistLayout() {
  persistPreferencesToBackend(services, dashboardLayout.value, 3);
}

function removeWidget(id: string) {
  const next = dashboardLayout.value.filter((w) => w.id !== id);
  dashboardLayout.value = compactLayout(next, GRID_COLS);
  persistLayout();
}

function handleAddWidget(type: string) {
  const cat = getWidgetCatalogItem(type);
  const w = cat?.defaultW ?? 8;
  const h = cat?.defaultH ?? 6;
  const pos = findOpenSlot(dashboardLayout.value, w, h, GRID_COLS);

  const newWidget: DashboardWidgetConfig = {
    id: `widget-${type}-${Date.now()}`,
    type,
    x: pos.x,
    y: pos.y,
    w,
    h,
    lane: cat?.defaultLane ?? 0,
  };

  dashboardLayout.value = compactLayout([...dashboardLayout.value, newWidget], GRID_COLS);
  persistLayout();
}

function stepWidgetSize(id: string, deltaW: number, deltaH: number) {
  const target = dashboardLayout.value.find((w) => w.id === id);
  if (!target) return;
  const cat = getWidgetCatalogItem(target.type);
  const minW = cat?.minW ?? 4;
  const minH = cat?.minH ?? 3;
  dashboardLayout.value = resizeGridItem(
    dashboardLayout.value,
    id,
    target.w + deltaW,
    target.h + deltaH,
    minW,
    minH,
    GRID_COLS
  );
  persistLayout();
}

async function resetToDefault() {
  const ok = await confirmModal({
    title: 'Reset Dashboard to Default?',
    message: 'Your custom widget arrangement will be replaced with the default dashboard layout.',
    tone: 'warning',
    confirmText: 'Reset Layout',
  });
  if (!ok) return;
  dashboardLayout.value = JSON.parse(JSON.stringify(DEFAULT_DASHBOARD_LAYOUT));
  persistLayout();
}

async function applyBalancedLayout() {
  const ok = await confirmModal({
    title: 'Apply Balanced Layout?',
    message: 'Your current widget arrangement will be replaced with the balanced preset layout.',
    tone: 'info',
    confirmText: 'Apply Balanced Layout',
  });
  if (!ok) return;
  dashboardLayout.value = JSON.parse(JSON.stringify(BALANCED_DASHBOARD_LAYOUT));
  persistLayout();
}

// ---------------------------------------------------------------------------
// Interactive Pointer Drag-to-Move & Drag-to-Resize
// ---------------------------------------------------------------------------
interface ActiveInteraction {
  mode: 'drag' | 'resize-se' | 'resize-e' | 'resize-s';
  widgetId: string;
  startClientX: number;
  startClientY: number;
  originX: number;
  originY: number;
  originW: number;
  originH: number;
  colStepPx: number;
  rowStepPx: number;
}

const interaction = ref<ActiveInteraction | null>(null);

const activePlaceholder = computed(() => {
  if (!interaction.value) return null;
  const w = dashboardLayout.value.find((item) => item.id === interaction.value?.widgetId);
  if (!w) return null;
  return { x: w.x, y: w.y, w: w.w, h: w.h };
});

function measureColStep(): number {
  const el = gridRef.value;
  if (!el) return 48;
  const width = el.clientWidth;
  return (width + GRID_GAP) / GRID_COLS;
}

function startInteraction(
  mode: ActiveInteraction['mode'],
  widget: DashboardWidgetConfig,
  event: PointerEvent
) {
  if (!isEditMode.value) return;
  if (event.button !== 0) return;

  event.preventDefault();
  interaction.value = {
    mode,
    widgetId: widget.id,
    startClientX: event.clientX,
    startClientY: event.clientY,
    originX: widget.x,
    originY: widget.y,
    originW: widget.w,
    originH: widget.h,
    colStepPx: Math.max(12, measureColStep()),
    rowStepPx: GRID_ROW_HEIGHT + GRID_GAP,
  };

  window.addEventListener('pointermove', onPointerMove);
  window.addEventListener('pointerup', onPointerUp);
  window.addEventListener('pointercancel', onPointerUp);
}

function onPointerMove(event: PointerEvent) {
  const act = interaction.value;
  if (!act) return;

  const dx = event.clientX - act.startClientX;
  const dy = event.clientY - act.startClientY;
  const deltaCols = Math.round(dx / act.colStepPx);
  const deltaRows = Math.round(dy / act.rowStepPx);

  const current = dashboardLayout.value.find((w) => w.id === act.widgetId);
  if (!current) return;

  if (act.mode === 'drag') {
    const nextX = Math.max(0, Math.min(GRID_COLS - current.w, act.originX + deltaCols));
    const nextY = Math.max(0, act.originY + deltaRows);
    if (nextX !== current.x || nextY !== current.y) {
      dashboardLayout.value = moveGridItem(
        dashboardLayout.value,
        act.widgetId,
        nextX,
        nextY,
        GRID_COLS,
        true
      );
    }
  } else {
    const cat = getWidgetCatalogItem(current.type);
    const minW = cat?.minW ?? 4;
    const minH = cat?.minH ?? 3;
    const targetW =
      act.mode === 'resize-s' ? act.originW : act.originW + deltaCols;
    const targetH =
      act.mode === 'resize-e' ? act.originH : act.originH + deltaRows;
    const clampedW = Math.max(minW, Math.min(GRID_COLS - current.x, targetW));
    const clampedH = Math.max(minH, targetH);

    if (clampedW !== current.w || clampedH !== current.h) {
      dashboardLayout.value = resizeGridItem(
        dashboardLayout.value,
        act.widgetId,
        clampedW,
        clampedH,
        minW,
        minH,
        GRID_COLS
      );
    }
  }
}

function onPointerUp() {
  if (!interaction.value) return;
  interaction.value = null;
  cleanupPointerListeners();
  // Settle layout cleanly with vertical gravity and persist
  dashboardLayout.value = compactLayout(dashboardLayout.value, GRID_COLS);
  persistLayout();
}

function cleanupPointerListeners() {
  window.removeEventListener('pointermove', onPointerMove);
  window.removeEventListener('pointerup', onPointerUp);
  window.removeEventListener('pointercancel', onPointerUp);
}

function getPanelGridStyle(widget: DashboardWidgetConfig) {
  return {
    gridColumn: `${widget.x + 1} / span ${widget.w}`,
    gridRow: `${widget.y + 1} / span ${widget.h}`,
  };
}
</script>

<template>
  <OverviewSkeleton v-if="!telemetry && (!nasState.initialLoaded || nasState.loading)" />
  <div v-else class="overview-dashboard">
    <!-- Top Action Bar / Edit Mode Banner -->
    <div v-if="isEditMode" class="dashboard-edit-banner">
      <div class="edit-banner-info">
        <div class="banner-icon-box">
          <svg viewBox="0 0 24 24" class="banner-icon"><path :d="mdiTune" /></svg>
        </div>
        <div>
          <div class="banner-title">Customize Dashboard</div>
          <div class="banner-subtitle">
            Fluid 24-column Grafana grid. Drag panel headers to reposition and drag corners or edges to scale any widget.
          </div>
        </div>
      </div>
      <div class="edit-banner-actions">
        <button class="btn btn-sm btn-secondary add-widget-btn" @click="showAddModal = true">
          <svg viewBox="0 0 24 24" class="btn-svg"><path :d="mdiPlus" /></svg>
          Add Widget
        </button>
        <button
          class="btn btn-sm btn-secondary balanced-layout-btn"
          @click="applyBalancedLayout"
          title="Apply balanced preset layout"
        >
          <svg viewBox="0 0 24 24" class="btn-svg"><path :d="mdiViewGridOutline" /></svg>
          Balanced Setup
        </button>
        <button
          class="btn btn-sm btn-secondary reset-layout-btn"
          @click="resetToDefault"
          title="Reset layout to default"
        >
          <svg viewBox="0 0 24 24" class="btn-svg"><path :d="mdiRestore" /></svg>
          Reset Default
        </button>
        <button class="btn btn-sm btn-primary done-edit-btn" @click="isEditMode = false">
          <svg viewBox="0 0 24 24" class="btn-svg"><path :d="mdiCheck" /></svg>
          Done Editing
        </button>
      </div>
    </div>

    <div v-else class="dashboard-top-bar">
      <button class="btn btn-sm btn-secondary edit-dashboard-btn" @click="isEditMode = true">
        <svg viewBox="0 0 24 24" class="btn-svg"><path :d="mdiTune" /></svg>
        Customize Dashboard
      </button>
    </div>

    <!-- Fluid Grafana-Style 24-Column Grid Canvas -->
    <div
      ref="gridRef"
      class="grafana-grid"
      :class="{
        editing: isEditMode,
        'is-interacting': Boolean(interaction),
      }"
    >
      <!-- Live Ghost Drop / Resize Placeholder -->
      <div
        v-if="activePlaceholder"
        class="grid-placeholder"
        :style="{
          gridColumn: `${activePlaceholder.x + 1} / span ${activePlaceholder.w}`,
          gridRow: `${activePlaceholder.y + 1} / span ${activePlaceholder.h}`,
        }"
      >
        <span class="placeholder-dims mono">
          {{ activePlaceholder.w }} × {{ activePlaceholder.h }}
        </span>
      </div>

      <!-- Fluid Scalable Widget Panels -->
      <div
        v-for="widget in dashboardLayout"
        :key="widget.id"
        class="widget-wrapper grid-panel"
        :class="{
          editing: isEditMode,
          'is-active-interaction': interaction?.widgetId === widget.id,
        }"
        :style="getPanelGridStyle(widget)"
        :data-widget-id="widget.id"
        :data-widget-type="widget.type"
      >
        <!-- Edit Mode Panel Toolbar (Drag Handle + Quick Scale + Remove) -->
        <div
          v-if="isEditMode"
          class="widget-panel-toolbar"
          @pointerdown="startInteraction('drag', widget, $event)"
        >
          <div class="toolbar-left">
            <span class="drag-grip" title="Drag to move widget">
              <svg viewBox="0 0 24 24" class="tool-svg"><path :d="mdiDrag" /></svg>
            </span>
            <span class="widget-type-name">{{ getWidgetName(widget.type) }}</span>
            <span class="size-badge mono">{{ widget.w }}×{{ widget.h }}</span>
          </div>

          <div class="toolbar-right" @pointerdown.stop>
            <div class="size-step-group" title="Adjust width">
              <button
                type="button"
                class="panel-tool-btn shrink-w-btn"
                title="Decrease width"
                @click="stepWidgetSize(widget.id, -2, 0)"
              >
                <svg viewBox="0 0 24 24" class="tool-svg"><path :d="mdiMinus" /></svg>
              </button>
              <span class="axis-tag mono">W</span>
              <button
                type="button"
                class="panel-tool-btn grow-w-btn"
                title="Increase width"
                @click="stepWidgetSize(widget.id, 2, 0)"
              >
                <svg viewBox="0 0 24 24" class="tool-svg"><path :d="mdiPlus" /></svg>
              </button>
            </div>

            <div class="size-step-group" title="Adjust height">
              <button
                type="button"
                class="panel-tool-btn shrink-h-btn"
                title="Decrease height"
                @click="stepWidgetSize(widget.id, 0, -1)"
              >
                <svg viewBox="0 0 24 24" class="tool-svg"><path :d="mdiMinus" /></svg>
              </button>
              <span class="axis-tag mono">H</span>
              <button
                type="button"
                class="panel-tool-btn grow-h-btn"
                title="Increase height"
                @click="stepWidgetSize(widget.id, 0, 1)"
              >
                <svg viewBox="0 0 24 24" class="tool-svg"><path :d="mdiPlus" /></svg>
              </button>
            </div>

            <button
              type="button"
              class="panel-tool-btn remove-btn"
              title="Remove widget"
              @click="removeWidget(widget.id)"
            >
              <svg viewBox="0 0 24 24" class="tool-svg"><path :d="mdiClose" /></svg>
            </button>
          </div>
        </div>

        <!-- Scalable Widget Body -->
        <div class="grid-panel-body">
          <component
            :is="getWidgetComponent(widget.type)"
            v-if="getWidgetComponent(widget.type)"
            :traffic-history="widget.type === 'network' ? trafficHistory : undefined"
            :gpu-history="widget.type === 'gpu' ? gpuHistory : undefined"
          />
        </div>

        <!-- Grafana-style Resize Handles in Edit Mode -->
        <template v-if="isEditMode">
          <div
            class="resize-handle resize-handle-e"
            title="Drag right edge to resize width"
            @pointerdown.stop="startInteraction('resize-e', widget, $event)"
          ></div>
          <div
            class="resize-handle resize-handle-s"
            title="Drag bottom edge to resize height"
            @pointerdown.stop="startInteraction('resize-s', widget, $event)"
          ></div>
          <div
            class="resize-handle resize-handle-se"
            title="Drag corner to resize panel"
            @pointerdown.stop="startInteraction('resize-se', widget, $event)"
          ></div>
        </template>
      </div>
    </div>

    <!-- Add Widget Catalog Modal -->
    <AddWidgetModal
      :open="showAddModal"
      :existing-widget-types="existingTypes"
      @close="showAddModal = false"
      @add="handleAddWidget"
    />
  </div>
</template>

<style scoped>
.overview-dashboard {
  display: flex;
  flex-direction: column;
  gap: 12px;
  position: relative;
  width: 100%;
}

/* Dashboard Top Bar */
.dashboard-top-bar {
  display: flex;
  justify-content: flex-end;
  align-items: center;
}

.edit-dashboard-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

/* Edit Mode Banner */
.dashboard-edit-banner {
  background: var(--bg-surface);
  border: 1px solid var(--accent-primary);
  border-radius: 12px;
  padding: 12px 16px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}

.edit-banner-info {
  display: flex;
  align-items: center;
  gap: 12px;
}

.banner-icon-box {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  background: var(--accent-bg-subtle);
  border: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.banner-icon {
  width: 20px;
  height: 20px;
  fill: var(--accent-primary);
}

.banner-title {
  font-weight: 700;
  font-size: 15px;
  color: var(--text-primary);
}

.banner-subtitle {
  font-size: 12px;
  color: var(--text-secondary);
}

.edit-banner-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.btn-svg {
  width: 16px;
  height: 16px;
  fill: currentColor;
}

/* Fluid 24-Column Grafana Grid */
.grafana-grid {
  display: grid;
  grid-template-columns: repeat(24, minmax(0, 1fr));
  grid-auto-rows: 48px;
  gap: 12px;
  width: 100%;
  position: relative;
  align-items: stretch;
}

.grafana-grid.editing {
  background-image: radial-gradient(rgba(148, 163, 184, 0.22) 1px, transparent 1px);
  background-size: calc(100% / 24) 60px;
  padding: 4px;
  border-radius: 12px;
  min-height: 480px;
}

.grafana-grid.is-interacting {
  user-select: none;
}

.grid-placeholder {
  border: 2px dashed var(--accent-primary);
  background: rgba(122, 162, 247, 0.1);
  border-radius: 12px;
  pointer-events: none;
  z-index: 1;
  display: flex;
  align-items: flex-end;
  justify-content: flex-end;
  padding: 8px 10px;
}

.placeholder-dims {
  font-size: 11px;
  font-weight: 700;
  color: var(--accent-primary);
  background: var(--bg-surface);
  border: 1px solid var(--accent-primary);
  padding: 1px 6px;
  border-radius: 4px;
}

/* Scalable Grid Panel */
.grid-panel {
  position: relative;
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  min-width: 0;
  container-type: inline-size;
  border-radius: 12px;
  transition: box-shadow 0.15s ease, outline-color 0.15s ease;
  z-index: 2;
}

.grid-panel.editing {
  outline: 1px dashed var(--border-strong);
  outline-offset: 2px;
}

.grid-panel.editing:hover {
  outline-color: var(--accent-primary);
}

.grid-panel.is-active-interaction {
  z-index: 20;
  outline: 2px solid var(--accent-primary);
  box-shadow: 0 12px 28px rgba(0, 0, 0, 0.35);
}

/* Edit Mode Toolbar */
.widget-panel-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  padding: 5px 8px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-bottom: none;
  border-top-left-radius: 10px;
  border-top-right-radius: 10px;
  cursor: grab;
  flex-shrink: 0;
  user-select: none;
}

.widget-panel-toolbar:active {
  cursor: grabbing;
}

.toolbar-left {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.drag-grip {
  display: inline-flex;
  align-items: center;
  color: var(--text-muted);
  flex-shrink: 0;
}

.widget-type-name {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.size-badge {
  font-size: 10px;
  font-weight: 600;
  color: var(--accent-primary);
  background: var(--accent-bg-subtle);
  padding: 1px 5px;
  border-radius: 4px;
  flex-shrink: 0;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.size-step-group {
  display: inline-flex;
  align-items: center;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 1px;
  gap: 2px;
}

.axis-tag {
  font-size: 9.5px;
  font-weight: 700;
  color: var(--text-muted);
  padding: 0 2px;
}

.panel-tool-btn {
  background: transparent;
  border: none;
  color: var(--text-secondary);
  width: 20px;
  height: 20px;
  border-radius: 4px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  padding: 0;
}

.panel-tool-btn:hover {
  background: var(--bg-surface);
  color: var(--text-primary);
}

.panel-tool-btn.remove-btn {
  border: 1px solid var(--border-subtle);
  background: var(--bg-base);
  width: 22px;
  height: 22px;
}

.panel-tool-btn.remove-btn:hover {
  background: rgba(239, 68, 68, 0.16);
  border-color: var(--danger);
  color: var(--danger);
}

.tool-svg {
  width: 14px;
  height: 14px;
  fill: currentColor;
}

/* Scalable Widget Body */
.grid-panel-body {
  flex: 1 1 0;
  min-height: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.grid-panel.editing .grid-panel-body :deep(> .card) {
  border-top-left-radius: 0;
  border-top-right-radius: 0;
}

/* Make every widget card scale smoothly to fill its Grafana grid cell */
.grid-panel-body :deep(> .card) {
  height: 100%;
  width: 100%;
  display: flex;
  flex-direction: column;
  min-height: 0;
  margin: 0;
  overflow-y: auto;
  overflow-x: hidden;
}

/* Resize Handles (Grafana style) */
.resize-handle {
  position: absolute;
  z-index: 15;
}

.resize-handle-e {
  top: 32px;
  right: -4px;
  bottom: 14px;
  width: 8px;
  cursor: ew-resize;
}

.resize-handle-s {
  left: 14px;
  right: 14px;
  bottom: -4px;
  height: 8px;
  cursor: ns-resize;
}

.resize-handle-se {
  right: 2px;
  bottom: 2px;
  width: 16px;
  height: 16px;
  cursor: nwse-resize;
}

.resize-handle-se::after {
  content: '';
  position: absolute;
  right: 3px;
  bottom: 3px;
  width: 9px;
  height: 9px;
  border-right: 2.5px solid var(--accent-primary);
  border-bottom: 2.5px solid var(--accent-primary);
  border-bottom-right-radius: 2px;
  opacity: 0.75;
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.resize-handle-se:hover::after {
  opacity: 1;
  transform: scale(1.15);
}

/* Responsive fallback for tablets/narrow viewports */
@media (max-width: 900px) {
  .grafana-grid {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .grid-panel {
    min-height: 260px;
  }

  .grid-panel[data-widget-type='kpis'] {
    min-height: auto;
  }
}
</style>
