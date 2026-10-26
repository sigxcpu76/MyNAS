<script setup lang="ts">
import SkeletonBox from './SkeletonBox.vue';

withDefaults(
  defineProps<{
    count?: number;
    type?: 'app' | 'pool' | 'vm' | 'share';
  }>(),
  {
    count: 3,
    type: 'app',
  }
);
</script>

<template>
  <div class="cards-skeleton-grid">
    <div v-for="i in count" :key="i" class="card card-skeleton-item">
      <!-- Header row: Icon/Avatar + Title/Subtitle + Status Badge -->
      <div class="card-header" style="margin-bottom: 14px">
        <div style="display: flex; align-items: center; gap: 12px; min-width: 0">
          <SkeletonBox width="42px" height="42px" rounded="8px" />
          <div style="display: flex; flex-direction: column; gap: 6px">
            <SkeletonBox width="130px" height="16px" />
            <SkeletonBox width="90px" height="12px" />
          </div>
        </div>
        <SkeletonBox width="65px" height="22px" rounded="12px" />
      </div>

      <!-- Content lines / stats -->
      <div style="display: flex; flex-direction: column; gap: 10px; margin-bottom: 16px">
        <div style="display: flex; justify-content: space-between; align-items: center">
          <SkeletonBox width="80px" height="13px" />
          <SkeletonBox width="110px" height="13px" />
        </div>
        <div style="display: flex; justify-content: space-between; align-items: center">
          <SkeletonBox width="95px" height="13px" />
          <SkeletonBox width="75px" height="13px" />
        </div>
        <SkeletonBox width="100%" height="8px" rounded="4px" />
      </div>

      <!-- Action buttons footer -->
      <div style="display: flex; justify-content: space-between; align-items: center; border-top: 1px solid var(--border-subtle); padding-top: 12px">
        <SkeletonBox width="80px" height="13px" />
        <div style="display: flex; gap: 8px">
          <SkeletonBox width="60px" height="28px" rounded="6px" />
          <SkeletonBox width="60px" height="28px" rounded="6px" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.cards-skeleton-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 18px;
  width: 100%;
}

.card-skeleton-item {
  padding: 18px;
}
</style>
