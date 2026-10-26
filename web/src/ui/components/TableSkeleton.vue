<script setup lang="ts">
import SkeletonBox from './SkeletonBox.vue';

withDefaults(
  defineProps<{
    rows?: number;
    columns?: number;
    colWidths?: string[];
  }>(),
  {
    rows: 4,
    columns: 6,
    colWidths: () => [],
  }
);
</script>

<template>
  <div class="table-skeleton-wrap">
    <div v-for="r in rows" :key="r" class="table-skeleton-row">
      <div
        v-for="c in columns"
        :key="c"
        class="table-skeleton-cell"
        :style="{ width: colWidths[c - 1] || 'auto', flex: colWidths[c - 1] ? 'none' : '1' }"
      >
        <SkeletonBox
          :width="c === 1 ? '110px' : c === columns ? '70px' : '85%'"
          :height="c === columns ? '26px' : '15px'"
          :rounded="c === columns ? '6px' : '4px'"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.table-skeleton-wrap {
  display: flex;
  flex-direction: column;
  width: 100%;
}

.table-skeleton-row {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 14px 12px;
  border-bottom: 1px solid var(--border-subtle);
}

.table-skeleton-row:last-child {
  border-bottom: none;
}

.table-skeleton-cell {
  display: flex;
  align-items: center;
  min-width: 0;
}
</style>
