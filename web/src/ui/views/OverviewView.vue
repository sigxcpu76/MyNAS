<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue';
import {
  mdiArrowDown,
  mdiArrowLeft,
  mdiArrowRight,
  mdiArrowUp,
  mdiCheck,
  mdiClose,
  mdiDrag,
  mdiPlus,
  mdiRestore,
  mdiTune,
  mdiViewColumn,
} from '@mdi/js';
import type { NetworkInterfaceTelemetry } from '../../domain/models';
import { services } from '../../container';
import { nasState, refreshTelemetry } from '../store';
import OverviewSkeleton from '../components/OverviewSkeleton.vue';
import AddWidgetModal from '../dashboard/AddWidgetModal.vue';
import {
  DEFAULT_DASHBOARD_LAYOUT,
  BALANCED_DASHBOARD_LAYOUT,
  loadDashboardLayout,
  loadDashboardColumnCount,
  saveDashboardLayout,
  syncPreferencesFromBackend,
  persistPreferencesToBackend,
  WIDGET_CATALOG,
  type DashboardWidgetConfig,
} from '../dashboard/dashboardConfig';

// Import Modular Widgets
import WidgetKpis from '../dashboard/widgets/WidgetKpis.vue';
import WidgetPools from '../dashboard/widgets/WidgetPools.vue';
import WidgetCpu from '../dashboard/widgets/WidgetCpu.vue';
import WidgetMemory from '../dashboard/widgets/WidgetMemory.vue';
import WidgetSystem from '../dashboard/widgets/WidgetSystem.vue';
import WidgetNetwork from '../dashboard/widgets/WidgetNetwork.vue';
import WidgetDisks from '../dashboard/widgets/WidgetDisks.vue';
import WidgetWorkloads from '../dashboard/widgets/WidgetWorkloads.vue';
import WidgetVms from '../dashboard/widgets/WidgetVms.vue';
import WidgetApps from '../dashboard/widgets/WidgetApps.vue';
import WidgetWebApps from '../dashboard/widgets/WidgetWebApps.vue';
import WidgetShares from '../dashboard/widgets/WidgetShares.vue';
import WidgetSensors from '../dashboard/widgets/WidgetSensors.vue';

// ---------------------------------------------------------------------------
// Telemetry (Reactive Stream)
// ---------------------------------------------------------------------------
const telemetry = computed(() => nasState.systemTelemetry);

// ---------------------------------------------------------------------------
// System Network Interface & Traffic History
// ---------------------------------------------------------------------------
export type { TrafficPoint } from '../store';

const trafficHistory = computed(() => nasState.networkTrafficHistory);

const systemInterface = computed<NetworkInterfaceTelemetry | null>(() => {
  const list = telemetry.value?.network || [];
  if (list.length === 0) return null;

  const def = list.find((n) => n.isDefault);
  if (def) return def;

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

  const activeWithIp = list.find(
    (n) => n.isUp && n.ipAddresses && n.ipAddresses.length > 0 && n.name !== 'lo'
  );
  if (activeWithIp) return activeWithIp;

  return list.find((n) => n.name !== 'lo') || list[0] || null;
});

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
});

// ---------------------------------------------------------------------------
// Dashboard Customization & Fluid Multi-Column Waterfall Lanes
// ---------------------------------------------------------------------------
const isEditMode = ref(false);
const showAddModal = ref(false);
const columnCount = ref<number>(loadDashboardColumnCount());
const dashboardLayout = ref<DashboardWidgetConfig[]>(loadDashboardLayout());

function setColumnCount(count: number) {
  columnCount.value = count;
  persistLayout();
}

async function syncBackendPreferences() {
  const remote = await syncPreferencesFromBackend(services);
  if (remote) {
    dashboardLayout.value = remote.layout;
    columnCount.value = remote.columnCount;
  }
}

watch(
  () => nasState.userPreferences,
  (prefs) => {
    if (prefs?.dashboard) {
      if (
        typeof prefs.dashboard.columnCount === 'number' &&
        (prefs.dashboard.columnCount === 2 || prefs.dashboard.columnCount === 3)
      ) {
        columnCount.value = prefs.dashboard.columnCount;
      }
      if (Array.isArray(prefs.dashboard.layout) && prefs.dashboard.layout.length > 0) {
        dashboardLayout.value = prefs.dashboard.layout.map((w) => ({
          id: w.id,
          type: w.type,
          lane: w.lane,
        }));
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

function getWidgetCatalogItem(type: string) {
  return WIDGET_CATALOG.find((c) => c.type === type);
}

function getWidgetName(type: string): string {
  return getWidgetCatalogItem(type)?.name || type;
}

// Full-width top ribbon widgets (KPIs)
const topRibbonWidgets = computed(() =>
  dashboardLayout.value.filter((w) => w.lane === -1)
);

// Column Lane widgets
function getLaneWidgets(laneIndex: number): DashboardWidgetConfig[] {
  return dashboardLayout.value.filter((w) => w.lane === laneIndex);
}

function persistLayout() {
  persistPreferencesToBackend(services, dashboardLayout.value, columnCount.value);
}

function removeWidget(id: string) {
  const idx = dashboardLayout.value.findIndex((w) => w.id === id);
  if (idx !== -1) {
    dashboardLayout.value.splice(idx, 1);
    persistLayout();
  }
}

function shiftWidgetOrder(id: string, direction: -1 | 1) {
  const widget = dashboardLayout.value.find((w) => w.id === id);
  if (!widget) return;

  const laneWidgets = getLaneWidgets(widget.lane);
  const laneIndex = laneWidgets.findIndex((w) => w.id === id);
  const targetLaneIndex = laneIndex + direction;

  if (targetLaneIndex < 0 || targetLaneIndex >= laneWidgets.length) return;

  const targetWidget = laneWidgets[targetLaneIndex];
  const globalIdxA = dashboardLayout.value.findIndex((w) => w.id === id);
  const globalIdxB = dashboardLayout.value.findIndex((w) => w.id === targetWidget.id);

  if (globalIdxA !== -1 && globalIdxB !== -1) {
    const item = dashboardLayout.value.splice(globalIdxA, 1)[0];
    dashboardLayout.value.splice(globalIdxB, 0, item);
    persistLayout();
  }
}

function shiftWidgetLane(id: string, direction: -1 | 1) {
  const widget = dashboardLayout.value.find((w) => w.id === id);
  if (!widget) return;

  let newLane = widget.lane + direction;
  if (newLane < 0) newLane = 0;
  if (newLane >= columnCount.value) newLane = columnCount.value - 1;

  if (newLane !== widget.lane) {
    widget.lane = newLane;
    persistLayout();
  }
}

function moveToLane(id: string, targetLane: number) {
  const widget = dashboardLayout.value.find((w) => w.id === id);
  if (widget) {
    widget.lane = targetLane;
    persistLayout();
  }
}

function handleAddWidget(type: string) {
  const cat = getWidgetCatalogItem(type);
  let targetLane = cat?.defaultLane ?? 0;

  if (type === 'kpis') {
    targetLane = -1;
  } else if (targetLane >= columnCount.value) {
    targetLane = columnCount.value - 1;
  } else if (targetLane === -1) {
    // If not kpis but defaultLane is -1, pick lane with fewest widgets
    let minCount = 9999;
    for (let c = 0; c < columnCount.value; c++) {
      const count = getLaneWidgets(c).length;
      if (count < minCount) {
        minCount = count;
        targetLane = c;
      }
    }
  }

  const newWidget: DashboardWidgetConfig = {
    id: `widget-${type}-${Date.now()}`,
    type,
    lane: targetLane,
  };

  dashboardLayout.value.push(newWidget);
  persistLayout();
}

function resetToDefault() {
  dashboardLayout.value = JSON.parse(JSON.stringify(DEFAULT_DASHBOARD_LAYOUT));
  persistLayout();
}

function applyBalancedLayout() {
  dashboardLayout.value = JSON.parse(JSON.stringify(BALANCED_DASHBOARD_LAYOUT));
  persistLayout();
}

// ---------------------------------------------------------------------------
// HTML5 Drag-and-Drop Reordering Between Lanes and Within Lanes
// ---------------------------------------------------------------------------
const draggedWidgetId = ref<string | null>(null);
const dragOverLane = ref<number | null>(null);
const dragOverWidgetId = ref<string | null>(null);

function onCardDragStart(id: string, event: DragEvent) {
  if (!isEditMode.value) return;
  draggedWidgetId.value = id;
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', id);
  }
}

function onCardDragOver(lane: number, targetId: string) {
  if (!isEditMode.value || !draggedWidgetId.value) return;
  dragOverLane.value = lane;
  dragOverWidgetId.value = targetId;
}

function onCardDrop(targetLane: number, targetId: string) {
  if (!isEditMode.value || !draggedWidgetId.value) return;
  if (draggedWidgetId.value === targetId) {
    draggedWidgetId.value = null;
    dragOverLane.value = null;
    dragOverWidgetId.value = null;
    return;
  }

  const sourceIdx = dashboardLayout.value.findIndex((w) => w.id === draggedWidgetId.value);
  if (sourceIdx === -1) return;

  const item = dashboardLayout.value.splice(sourceIdx, 1)[0];
  item.lane = targetLane;

  const targetIdx = dashboardLayout.value.findIndex((w) => w.id === targetId);
  if (targetIdx !== -1) {
    dashboardLayout.value.splice(targetIdx, 0, item);
  } else {
    dashboardLayout.value.push(item);
  }

  persistLayout();
  draggedWidgetId.value = null;
  dragOverLane.value = null;
  dragOverWidgetId.value = null;
}

function onLaneDragOver(lane: number) {
  if (!isEditMode.value || !draggedWidgetId.value) return;
  dragOverLane.value = lane;
}

function onLaneDrop(targetLane: number) {
  if (!isEditMode.value || !draggedWidgetId.value) return;
  const sourceIdx = dashboardLayout.value.findIndex((w) => w.id === draggedWidgetId.value);
  if (sourceIdx !== -1) {
    const item = dashboardLayout.value.splice(sourceIdx, 1)[0];
    item.lane = targetLane;
    dashboardLayout.value.push(item);
    persistLayout();
  }
  draggedWidgetId.value = null;
  dragOverLane.value = null;
  dragOverWidgetId.value = null;
}

function onDragEnd() {
  draggedWidgetId.value = null;
  dragOverLane.value = null;
  dragOverWidgetId.value = null;
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
          <div class="banner-subtitle">Fluid waterfall lanes adapt to natural content height. Drag cards across columns without clipped items or dead space.</div>
        </div>
      </div>
      <div class="edit-banner-actions">
        <!-- Columns Selector -->
        <div class="column-count-toggle" title="Switch layout column count">
          <svg viewBox="0 0 24 24" class="btn-svg"><path :d="mdiViewColumn" /></svg>
          <button
            type="button"
            class="col-toggle-btn"
            :class="{ active: columnCount === 2 }"
            @click="setColumnCount(2)"
          >
            2 Columns
          </button>
          <button
            type="button"
            class="col-toggle-btn"
            :class="{ active: columnCount === 3 }"
            @click="setColumnCount(3)"
          >
            3 Columns
          </button>
        </div>

        <button class="btn btn-sm btn-secondary add-widget-btn" @click="showAddModal = true">
          <svg viewBox="0 0 24 24" class="btn-svg"><path :d="mdiPlus" /></svg>
          Add Widget
        </button>
        <button class="btn btn-sm btn-secondary balanced-layout-btn" @click="applyBalancedLayout" title="Apply balanced layout across all lanes">
          <svg viewBox="0 0 24 24" class="btn-svg"><path :d="mdiViewColumn" /></svg>
          Balanced Setup
        </button>
        <button class="btn btn-sm btn-secondary reset-layout-btn" @click="resetToDefault" title="Reset layout to default">
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

    <!-- 1. Top Ribbon Section: Full-Width Ribbon (KPIs) -->
    <div v-if="topRibbonWidgets.length > 0" class="dashboard-top-ribbon">
      <div
        v-for="widget in topRibbonWidgets"
        :key="widget.id"
        class="widget-wrapper ribbon-card-wrapper"
        :class="{
          editing: isEditMode,
          'is-dragging': draggedWidgetId === widget.id,
        }"
        :draggable="isEditMode"
        @dragstart="onCardDragStart(widget.id, $event)"
        @dragend="onDragEnd"
      >
        <!-- Top Ribbon Toolbar in Edit Mode -->
        <div v-if="isEditMode" class="widget-lane-toolbar ribbon-toolbar">
          <div class="toolbar-left">
            <span class="drag-grip" title="Drag widget">
              <svg viewBox="0 0 24 24" class="tool-svg"><path :d="mdiDrag" /></svg>
            </span>
            <span class="widget-type-name">{{ getWidgetName(widget.type) }}</span>
            <span class="lane-badge">Top Ribbon</span>
          </div>
          <div class="toolbar-right">
            <button
              class="lane-tool-btn move-down-btn"
              @click="moveToLane(widget.id, 0)"
              title="Move down into Column 1"
            >
              <svg viewBox="0 0 24 24" class="tool-svg"><path :d="mdiArrowDown" /></svg>
              <span>Move to Columns</span>
            </button>
            <button
              class="lane-tool-btn remove-btn"
              @click="removeWidget(widget.id)"
              title="Remove widget"
            >
              <svg viewBox="0 0 24 24" class="tool-svg"><path :d="mdiClose" /></svg>
            </button>
          </div>
        </div>

        <!-- Render Component -->
        <component
          :is="getWidgetComponent(widget.type)"
          v-if="getWidgetComponent(widget.type)"
          :traffic-history="widget.type === 'network' ? trafficHistory : undefined"
        />
      </div>
    </div>

    <!-- 2. Multi-Column Fluid Waterfall Lanes -->
    <div class="dashboard-lanes" :class="`columns-${columnCount}`">
      <div
        v-for="colIdx in columnCount"
        :key="colIdx - 1"
        class="dashboard-lane"
        :class="{
          'lane-drag-over': dragOverLane === colIdx - 1,
          'editing': isEditMode
        }"
        @dragover.prevent="onLaneDragOver(colIdx - 1)"
        @drop.prevent="onLaneDrop(colIdx - 1)"
      >
        <!-- Column Header in Edit Mode -->
        <div v-if="isEditMode" class="lane-header-bar">
          <span class="lane-title">Column {{ colIdx }}</span>
          <span class="lane-count-tag">{{ getLaneWidgets(colIdx - 1).length }} widgets</span>
        </div>

        <!-- Empty Lane Placeholder in Edit Mode -->
        <div
          v-if="isEditMode && getLaneWidgets(colIdx - 1).length === 0"
          class="empty-lane-placeholder"
        >
          Drop widgets here to add to Column {{ colIdx }}
        </div>

        <!-- Widgets in this Column -->
        <div
          v-for="(widget, itemIdx) in getLaneWidgets(colIdx - 1)"
          :key="widget.id"
          class="widget-wrapper lane-widget-card"
          :class="{
            editing: isEditMode,
            'is-dragging': draggedWidgetId === widget.id,
            'is-drag-over': dragOverWidgetId === widget.id,
          }"
          :draggable="isEditMode"
          @dragstart="onCardDragStart(widget.id, $event)"
          @dragend="onDragEnd"
          @dragover.stop.prevent="onCardDragOver(colIdx - 1, widget.id)"
          @drop.stop="onCardDrop(colIdx - 1, widget.id)"
        >
          <!-- Card Toolbar in Edit Mode -->
          <div v-if="isEditMode" class="widget-lane-toolbar">
            <div class="toolbar-left">
              <span class="drag-grip" title="Drag to reorder or move to another column">
                <svg viewBox="0 0 24 24" class="tool-svg"><path :d="mdiDrag" /></svg>
              </span>
              <span class="widget-type-name">{{ getWidgetName(widget.type) }}</span>
              <span class="lane-badge">Col {{ colIdx }}</span>
            </div>

            <div class="toolbar-right">
              <!-- Move Left to previous column -->
              <button
                class="lane-tool-btn move-left-btn"
                :disabled="colIdx - 1 === 0"
                @click="shiftWidgetLane(widget.id, -1)"
                title="Move left to previous column"
              >
                <svg viewBox="0 0 24 24" class="tool-svg"><path :d="mdiArrowLeft" /></svg>
              </button>

              <!-- Move Up in current column -->
              <button
                class="lane-tool-btn move-up-btn"
                :disabled="itemIdx === 0"
                @click="shiftWidgetOrder(widget.id, -1)"
                title="Move up in column"
              >
                <svg viewBox="0 0 24 24" class="tool-svg"><path :d="mdiArrowUp" /></svg>
              </button>

              <!-- Move Down in current column -->
              <button
                class="lane-tool-btn move-down-btn"
                :disabled="itemIdx === getLaneWidgets(colIdx - 1).length - 1"
                @click="shiftWidgetOrder(widget.id, 1)"
                title="Move down in column"
              >
                <svg viewBox="0 0 24 24" class="tool-svg"><path :d="mdiArrowDown" /></svg>
              </button>

              <!-- Move Right to next column -->
              <button
                class="lane-tool-btn move-right-btn"
                :disabled="colIdx - 1 === columnCount - 1"
                @click="shiftWidgetLane(widget.id, 1)"
                title="Move right to next column"
              >
                <svg viewBox="0 0 24 24" class="tool-svg"><path :d="mdiArrowRight" /></svg>
              </button>

              <!-- Remove Widget -->
              <button
                class="lane-tool-btn remove-btn"
                @click="removeWidget(widget.id)"
                title="Remove widget"
              >
                <svg viewBox="0 0 24 24" class="tool-svg"><path :d="mdiClose" /></svg>
              </button>
            </div>
          </div>

          <!-- Render Modular Widget Component -->
          <component
            :is="getWidgetComponent(widget.type)"
            v-if="getWidgetComponent(widget.type)"
            :traffic-history="widget.type === 'network' ? trafficHistory : undefined"
          />
        </div>
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
  gap: 16px;
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
  padding: 12px 18px;
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

/* Column Count Toggle */
.column-count-toggle {
  display: inline-flex;
  align-items: center;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 2px 4px;
  gap: 4px;
  color: var(--text-secondary);
}

.col-toggle-btn {
  background: transparent;
  border: none;
  border-radius: 6px;
  padding: 3px 8px;
  font-size: 11px;
  font-weight: 600;
  color: var(--text-muted);
  cursor: pointer;
  transition: all 0.15s ease;
}

.col-toggle-btn:hover {
  color: var(--text-primary);
}

.col-toggle-btn.active {
  background: var(--bg-surface);
  color: var(--accent-primary);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.2);
}

/* 1. Top Ribbon Section (KPIs) */
.dashboard-top-ribbon {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.ribbon-card-wrapper {
  width: 100%;
  height: auto;
}

/* 2. Fluid Multi-Column Waterfall Lanes */
.dashboard-lanes {
  display: flex;
  gap: 16px;
  align-items: flex-start;
  width: 100%;
}

.dashboard-lane {
  flex: 1 1 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
  transition: background 0.15s ease;
}

.dashboard-lane.editing {
  background: rgba(122, 162, 247, 0.02);
  border: 1px dashed var(--border-subtle);
  border-radius: 10px;
  padding: 8px;
}

.dashboard-lane.lane-drag-over {
  outline: 2px dashed var(--accent-primary);
  background: rgba(122, 162, 247, 0.06);
}

.lane-header-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 8px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
}

.lane-title {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--text-secondary);
}

.lane-count-tag {
  font-size: 10.5px;
  color: var(--text-muted);
  font-family: var(--font-mono, monospace);
}

.empty-lane-placeholder {
  padding: 32px 16px;
  text-align: center;
  font-size: 12px;
  color: var(--text-muted);
  border: 1px dashed var(--border-subtle);
  border-radius: 8px;
}

/* Cards inside Lanes */
.lane-widget-card {
  width: 100%;
  height: auto;
  display: flex;
  flex-direction: column;
  transition: transform 0.15s ease, opacity 0.15s ease;
}

.widget-wrapper.editing {
  border: 1px dashed var(--accent-primary);
  border-radius: 10px;
  padding: 6px;
  background: rgba(122, 162, 247, 0.02);
}

.widget-wrapper.is-dragging {
  opacity: 0.35;
  transform: scale(0.98);
}

.widget-wrapper.is-drag-over {
  outline: 2px solid var(--accent-primary);
  outline-offset: 4px;
}

/* Card Toolbars in Edit Mode */
.widget-lane-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 4px 8px;
  margin-bottom: 6px;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.15);
  gap: 8px;
  flex-wrap: wrap;
}

.toolbar-left,
.toolbar-right {
  display: flex;
  align-items: center;
  gap: 6px;
}

.drag-grip {
  cursor: grab;
  color: var(--text-secondary);
  display: flex;
  align-items: center;
  padding: 2px;
}

.drag-grip:active {
  cursor: grabbing;
}

.widget-type-name {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--text-primary);
}

.lane-badge {
  font-size: 10px;
  font-family: var(--font-mono, monospace);
  font-weight: 600;
  background: var(--bg-tertiary);
  color: var(--accent-primary);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  padding: 1px 5px;
}

.lane-tool-btn {
  background: transparent;
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 3px 5px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  transition: all 0.15s ease;
}

.lane-tool-btn:disabled {
  opacity: 0.25;
  cursor: not-allowed;
}

.lane-tool-btn:not(:disabled):hover {
  color: var(--text-primary);
  border-color: var(--border-strong);
  background: var(--bg-surface-elevated, rgba(255, 255, 255, 0.04));
}

.lane-tool-btn.remove-btn:hover {
  color: var(--danger, #f7768e);
  border-color: var(--danger, #f7768e);
}

.tool-svg {
  width: 14px;
  height: 14px;
  fill: currentColor;
}

/* Responsive Breakpoints */
@media (max-width: 1024px) {
  .dashboard-lanes.columns-3 .dashboard-lane:nth-child(3) {
    display: none;
  }
}

@media (max-width: 768px) {
  .dashboard-lanes {
    flex-direction: column;
    gap: 16px;
  }

  .dashboard-lane {
    width: 100%;
  }

  .dashboard-edit-banner {
    flex-direction: column;
    align-items: flex-start;
  }

  .edit-banner-actions {
    width: 100%;
    justify-content: flex-start;
  }
}
</style>
