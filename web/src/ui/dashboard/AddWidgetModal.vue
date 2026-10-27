<script setup lang="ts">
import { computed, ref } from 'vue';
import { mdiClose, mdiPlus, mdiCheck } from '@mdi/js';
import { WIDGET_CATALOG, type WidgetCatalogItem } from './dashboardConfig';

const props = defineProps<{
  open: boolean;
  existingWidgetTypes: string[];
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'add', widgetType: string): void;
}>();

const selectedCategory = ref<string>('all');

const filteredCatalog = computed(() => {
  if (selectedCategory.value === 'all') {
    return WIDGET_CATALOG;
  }
  return WIDGET_CATALOG.filter((item) => item.category === selectedCategory.value);
});

function isAlreadyAdded(type: string): boolean {
  return props.existingWidgetTypes.includes(type);
}

function handleAdd(type: string) {
  emit('add', type);
}

function getColSpanLabel(span: number): string {
  switch (span) {
    case 3:
      return '1/4 Width';
    case 4:
      return '1/3 Width';
    case 6:
      return 'Half Width';
    case 8:
      return '2/3 Width';
    case 12:
      return 'Full Width';
    default:
      return `${span} Cols`;
  }
}
</script>

<template>
  <div v-if="open" class="modal-backdrop" @click.self="emit('close')">
    <div class="modal-dialog">
      <div class="modal-header">
        <div class="header-titles">
          <h2 class="modal-title">Add Widget to Dashboard</h2>
          <p class="modal-subtitle">Choose from real-time telemetry, storage pools, workloads, and system cards.</p>
        </div>
        <button class="btn-icon" @click="emit('close')" title="Close">
          <svg viewBox="0 0 24 24" class="icon-svg"><path :d="mdiClose" /></svg>
        </button>
      </div>

      <!-- Category Filter Pills -->
      <div class="category-filters">
        <button
          type="button"
          class="cat-pill"
          :class="{ active: selectedCategory === 'all' }"
          @click="selectedCategory = 'all'"
        >
          All ({{ WIDGET_CATALOG.length }})
        </button>
        <button
          type="button"
          class="cat-pill"
          :class="{ active: selectedCategory === 'system' }"
          @click="selectedCategory = 'system'"
        >
          System
        </button>
        <button
          type="button"
          class="cat-pill"
          :class="{ active: selectedCategory === 'storage' }"
          @click="selectedCategory = 'storage'"
        >
          Storage
        </button>
        <button
          type="button"
          class="cat-pill"
          :class="{ active: selectedCategory === 'network' }"
          @click="selectedCategory = 'network'"
        >
          Network
        </button>
        <button
          type="button"
          class="cat-pill"
          :class="{ active: selectedCategory === 'workloads' }"
          @click="selectedCategory = 'workloads'"
        >
          Workloads
        </button>
      </div>

      <!-- Catalog Grid -->
      <div class="catalog-grid">
        <div
          v-for="item in filteredCatalog"
          :key="item.type"
          class="catalog-card"
        >
          <div class="catalog-card-icon">
            <svg viewBox="0 0 24 24" class="widget-icon-svg">
              <path :d="item.icon" />
            </svg>
          </div>

          <div class="catalog-card-body">
            <div class="catalog-card-header">
              <span class="widget-name">{{ item.name }}</span>
              <span class="badge badge-neutral" style="font-size: 10.5px; text-transform: capitalize">
                {{ item.category }}
              </span>
            </div>
            <p class="widget-desc">{{ item.description }}</p>

            <div class="catalog-card-footer">
              <span class="span-tag">
                Default: {{ getColSpanLabel(item.defaultColSpan) }}
              </span>

              <button
                type="button"
                class="btn btn-sm"
                :class="isAlreadyAdded(item.type) ? 'btn-secondary' : 'btn-primary'"
                @click="handleAdd(item.type)"
              >
                <svg viewBox="0 0 24 24" style="width: 14px; height: 14px; fill: currentColor">
                  <path :d="isAlreadyAdded(item.type) ? mdiPlus : mdiPlus" />
                </svg>
                <span>{{ isAlreadyAdded(item.type) ? 'Add Another' : 'Add Widget' }}</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <div class="modal-footer">
        <button class="btn btn-secondary" @click="emit('close')">
          Done
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.65);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1050;
  padding: 20px;
}

.modal-dialog {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  width: 100%;
  max-width: 720px;
  max-height: 85vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 16px 36px rgba(0, 0, 0, 0.4);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 18px 20px 14px 20px;
  border-bottom: 1px solid var(--border-subtle);
}

.header-titles {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.modal-title {
  font-size: 18px;
  font-weight: 700;
  margin: 0;
  color: var(--text-primary);
}

.modal-subtitle {
  font-size: 12.5px;
  color: var(--text-secondary);
  margin: 0;
}

.category-filters {
  display: flex;
  gap: 8px;
  padding: 12px 20px;
  border-bottom: 1px solid var(--border-subtle);
  background: var(--bg-base);
  flex-wrap: wrap;
}

.cat-pill {
  background: transparent;
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 500;
  padding: 4px 12px;
  border-radius: 16px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.cat-pill:hover {
  border-color: var(--border-strong);
  color: var(--text-primary);
}

.cat-pill.active {
  background: var(--accent-primary);
  border-color: var(--accent-primary);
  color: #ffffff;
  font-weight: 600;
}

.catalog-grid {
  padding: 16px 20px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 12px;
  flex: 1;
}

.catalog-card {
  display: flex;
  gap: 14px;
  padding: 12px 14px;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  align-items: center;
  transition: border-color 0.15s ease;
}

.catalog-card:hover {
  border-color: var(--accent-primary);
}

.catalog-card-icon {
  width: 44px;
  height: 44px;
  border-radius: 8px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.widget-icon-svg {
  width: 24px;
  height: 24px;
  fill: var(--accent-primary);
}

.catalog-card-body {
  display: flex;
  flex-direction: column;
  gap: 4px;
  flex: 1;
  min-width: 0;
}

.catalog-card-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.widget-name {
  font-weight: 600;
  font-size: 14.5px;
  color: var(--text-primary);
}

.widget-desc {
  font-size: 12px;
  color: var(--text-secondary);
  margin: 0;
  line-height: 1.4;
}

.catalog-card-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 4px;
}

.span-tag {
  font-size: 11px;
  color: var(--text-muted);
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  padding: 12px 20px;
  border-top: 1px solid var(--border-subtle);
  background: var(--bg-base);
  border-bottom-left-radius: 12px;
  border-bottom-right-radius: 12px;
}

.btn-icon {
  background: transparent;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 4px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.btn-icon:hover {
  color: var(--text-primary);
  background: var(--bg-base);
}

.icon-svg {
  width: 20px;
  height: 20px;
  fill: currentColor;
}
</style>
