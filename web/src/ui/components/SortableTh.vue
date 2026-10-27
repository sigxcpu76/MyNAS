<script setup lang="ts">
import { computed } from 'vue';
import { mdiMenuDown, mdiMenuUp, mdiMenuSwap } from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import type { TableSortState } from '../composables/useTableSort';

/**
 * Clickable, keyboard-accessible table header cell that toggles sorting
 * for a column managed by `useTableSort`.
 */
const props = defineProps<{
  sort: TableSortState;
  sortKey: string;
}>();

const active = computed(() => props.sort.key.value === props.sortKey);
const ariaSort = computed(() =>
  active.value ? (props.sort.dir.value === 'asc' ? 'ascending' : 'descending') : 'none'
);
const icon = computed(() =>
  !active.value ? mdiMenuSwap : props.sort.dir.value === 'asc' ? mdiMenuUp : mdiMenuDown
);
</script>

<template>
  <th
    class="sortable-th"
    :class="{ 'is-sorted': active }"
    :aria-sort="ariaSort"
    tabindex="0"
    @click="sort.toggle(sortKey)"
    @keydown.enter.prevent="sort.toggle(sortKey)"
    @keydown.space.prevent="sort.toggle(sortKey)"
  >
    <span class="sortable-th-inner">
      <span><slot /></span>
      <MdiIcon :path="icon" size="16" class="sortable-th-icon" />
    </span>
  </th>
</template>
