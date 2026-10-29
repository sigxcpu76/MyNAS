<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from 'vue';
import {
  mdiArrowLeft,
  mdiArrowRight,
  mdiCheck,
  mdiClose,
  mdiDrag,
  mdiPlus,
  mdiRestore,
  mdiTune,
} from '@mdi/js';
import type { NetworkInterfaceTelemetry } from '../../domain/models';
import { nasState, refreshTelemetry } from '../store';
import OverviewSkeleton from '../components/OverviewSkeleton.vue';
import AddWidgetModal from '../dashboard/AddWidgetModal.vue';
import {
  DEFAULT_DASHBOARD_LAYOUT,
  loadDashboardLayout,
  saveDashboardLayout,
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

onMounted(() => {
  if (!nasState.systemTelemetry) {
    refreshTelemetry();
  }
});

// ---------------------------------------------------------------------------
// Dashboard Customization & Layout State
// ---------------------------------------------------------------------------
const isEditMode = ref(false);
const showAddModal = ref(false);
const dashboardLayout = ref<DashboardWidgetConfig[]>(loadDashboardLayout());

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
    default:
      return null;
  }
}

function getWidgetCatalogItem(type: string) {
  return WIDGET_CATALOG.find((c) => c.type === type);
}

function getAllowedSpans(type: string): number[] {
  const item = getWidgetCatalogItem(type);
  return item?.allowedColSpans || [4, 6, 8, 12];
}

function getSpanLabel(span: number): string {
  switch (span) {
    case 3:
      return '1/4';
    case 4:
      return '1/3';
    case 6:
      return '1/2';
    case 8:
      return '2/3';
    case 12:
      return 'Full';
    default:
      return `${span}c`;
  }
}

function persistLayout() {
  saveDashboardLayout(dashboardLayout.value);
}

function setWidgetColSpan(index: number, colSpan: number) {
  if (dashboardLayout.value[index]) {
    dashboardLayout.value[index].colSpan = colSpan;
    persistLayout();
  }
}

function removeWidget(index: number) {
  dashboardLayout.value.splice(index, 1);
  persistLayout();
}

function moveWidget(index: number, direction: -1 | 1) {
  const newIndex = index + direction;
  if (newIndex < 0 || newIndex >= dashboardLayout.value.length) return;
  const item = dashboardLayout.value.splice(index, 1)[0];
  dashboardLayout.value.splice(newIndex, 0, item);
  persistLayout();
}

function handleAddWidget(type: string) {
  const cat = getWidgetCatalogItem(type);
  const defaultSpan = cat?.defaultColSpan || 6;
  const newWidget: DashboardWidgetConfig = {
    id: `widget-${type}-${Date.now()}`,
    type,
    colSpan: defaultSpan,
  };
  dashboardLayout.value.push(newWidget);
  persistLayout();
}

function resetToDefault() {
  dashboardLayout.value = JSON.parse(JSON.stringify(DEFAULT_DASHBOARD_LAYOUT));
  persistLayout();
}

// ---------------------------------------------------------------------------
// HTML5 Drag-and-Drop Reordering
// ---------------------------------------------------------------------------
const draggedIndex = ref<number | null>(null);
const dragOverIndex = ref<number | null>(null);

function onDragStart(index: number, event: DragEvent) {
  if (!isEditMode.value) return;
  draggedIndex.value = index;
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', String(index));
  }
}

function onDragOver(index: number, event: DragEvent) {
  if (!isEditMode.value || draggedIndex.value === null) return;
  event.preventDefault();
  dragOverIndex.value = index;
}

function onDrop(index: number) {
  if (!isEditMode.value || draggedIndex.value === null) return;
  if (draggedIndex.value !== index) {
    const item = dashboardLayout.value.splice(draggedIndex.value, 1)[0];
    dashboardLayout.value.splice(index, 0, item);
    persistLayout();
  }
  draggedIndex.value = null;
  dragOverIndex.value = null;
}

function onDragEnd() {
  draggedIndex.value = null;
  dragOverIndex.value = null;
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
          <div class="banner-subtitle">Resize with width pills, drag or use arrows to reorder, or add new telemetry widgets.</div>
        </div>
      </div>
      <div class="edit-banner-actions">
        <button class="btn btn-sm btn-secondary add-widget-btn" @click="showAddModal = true">
          <svg viewBox="0 0 24 24" class="btn-svg"><path :d="mdiPlus" /></svg>
          Add Widget
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

    <!-- Responsive 12-Column Dashboard Grid -->
    <div class="dashboard-grid">
      <div
        v-for="(widget, index) in dashboardLayout"
        :key="widget.id"
        class="widget-wrapper"
        :class="[
          `col-span-${widget.colSpan || 12}`,
          {
            editing: isEditMode,
            'is-dragging': draggedIndex === index,
            'is-drag-over': dragOverIndex === index,
          },
        ]"
        :draggable="isEditMode"
        @dragstart="onDragStart(index, $event)"
        @dragover="onDragOver(index, $event)"
        @drop="onDrop(index)"
        @dragend="onDragEnd"
      >
        <!-- Edit Mode Header Toolbar -->
        <div v-if="isEditMode" class="widget-edit-toolbar">
          <div class="edit-left">
            <span class="drag-handle" title="Drag to reorder">
              <svg viewBox="0 0 24 24" class="toolbar-svg"><path :d="mdiDrag" /></svg>
            </span>
            <span class="widget-type-label">
              {{ getWidgetCatalogItem(widget.type)?.name || widget.type }}
            </span>
          </div>

          <div class="edit-center">
            <!-- ColSpan Resizing Pills -->
            <div class="span-pill-group">
              <button
                v-for="span in getAllowedSpans(widget.type)"
                :key="span"
                type="button"
                class="span-btn"
                :class="{ active: widget.colSpan === span }"
                @click="setWidgetColSpan(index, span)"
                :title="`Set width to ${getSpanLabel(span)} (${span} columns)`"
              >
                {{ getSpanLabel(span) }}
              </button>
            </div>
          </div>

          <div class="edit-right">
            <!-- Move Left / Move Right Buttons -->
            <button
              class="toolbar-btn move-left-btn"
              :disabled="index === 0"
              @click="moveWidget(index, -1)"
              title="Move left/up"
            >
              <svg viewBox="0 0 24 24" class="toolbar-svg"><path :d="mdiArrowLeft" /></svg>
            </button>
            <button
              class="toolbar-btn move-right-btn"
              :disabled="index === dashboardLayout.length - 1"
              @click="moveWidget(index, 1)"
              title="Move right/down"
            >
              <svg viewBox="0 0 24 24" class="toolbar-svg"><path :d="mdiArrowRight" /></svg>
            </button>
            <!-- Remove Widget Button -->
            <button
              class="toolbar-btn remove-btn"
              @click="removeWidget(index)"
              title="Remove widget"
            >
              <svg viewBox="0 0 24 24" class="toolbar-svg"><path :d="mdiClose" /></svg>
            </button>
          </div>
        </div>

        <!-- Render Modular Widget -->
        <component
          :is="getWidgetComponent(widget.type)"
          v-if="getWidgetComponent(widget.type)"
          :traffic-history="widget.type === 'network' ? trafficHistory : undefined"
        />
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
  border-radius: 10px;
  padding: 12px 18px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.15);
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
  background: rgba(122, 162, 247, 0.12);
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

/* 12-Column Responsive CSS Grid */
.dashboard-grid {
  display: grid;
  grid-template-columns: repeat(12, 1fr);
  gap: 16px;
  width: 100%;
}

.widget-wrapper {
  min-width: 0;
  display: flex;
  flex-direction: column;
  position: relative;
  transition: transform 0.15s ease, box-shadow 0.15s ease, opacity 0.15s ease;
}

/* Column Spans */
.col-span-3 {
  grid-column: span 3;
}

.col-span-4 {
  grid-column: span 4;
}

.col-span-6 {
  grid-column: span 6;
}

.col-span-8 {
  grid-column: span 8;
}

.col-span-12 {
  grid-column: span 12;
}

/* Edit Mode Styles */
.widget-wrapper.editing {
  border: 1px dashed var(--accent-primary);
  border-radius: 10px;
  padding: 8px;
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

.widget-edit-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 5px 8px;
  margin-bottom: 8px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
  gap: 8px;
  flex-wrap: wrap;
}

.edit-left,
.edit-center,
.edit-right {
  display: flex;
  align-items: center;
  gap: 6px;
}

.drag-handle {
  cursor: grab;
  color: var(--text-secondary);
  display: flex;
  align-items: center;
  padding: 2px;
}

.drag-handle:active {
  cursor: grabbing;
}

.widget-type-label {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--text-primary);
}

.span-pill-group {
  display: inline-flex;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  padding: 1px;
  gap: 2px;
}

.span-btn {
  background: transparent;
  border: none;
  border-radius: 10px;
  padding: 2px 7px;
  font-size: 10.5px;
  font-weight: 500;
  color: var(--text-muted);
  cursor: pointer;
  transition: all 0.15s ease;
}

.span-btn:hover {
  color: var(--text-primary);
}

.span-btn.active {
  background: var(--bg-surface);
  color: var(--accent-primary);
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.2);
}

.toolbar-btn {
  background: transparent;
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 3px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.15s ease;
}

.toolbar-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.toolbar-btn:not(:disabled):hover {
  color: var(--text-primary);
  border-color: var(--border-strong);
}

.toolbar-btn.remove-btn:hover {
  color: var(--danger, #f7768e);
  border-color: var(--danger, #f7768e);
}

.toolbar-svg {
  width: 15px;
  height: 15px;
  fill: currentColor;
}

/* Responsive Grid Adjustments */
@media (max-width: 1024px) {
  .col-span-3 {
    grid-column: span 6;
  }
  .col-span-4 {
    grid-column: span 6;
  }
  .col-span-6 {
    grid-column: span 6;
  }
  .col-span-8 {
    grid-column: span 12;
  }
  .col-span-12 {
    grid-column: span 12;
  }
}

@media (max-width: 768px) {
  .dashboard-grid {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .widget-wrapper {
    grid-column: span 12 !important;
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
