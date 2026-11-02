<script setup lang="ts">
import WidgetVms from './WidgetVms.vue';
import WidgetApps from './WidgetApps.vue';
import WidgetShares from './WidgetShares.vue';
</script>

<template>
  <div class="workloads-unified-widget">
    <div class="workloads-grid">
      <WidgetVms />
      <WidgetApps />
      <WidgetShares />
    </div>
  </div>
</template>

<style scoped>
.workloads-unified-widget {
  width: 100%;
  height: 100%;
}

.workloads-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 14px;
  height: 100%;
}
</style>