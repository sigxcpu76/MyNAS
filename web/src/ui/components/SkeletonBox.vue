<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(
  defineProps<{
    width?: string;
    height?: string;
    circle?: boolean;
    rounded?: string;
    animate?: boolean;
    count?: number;
  }>(),
  {
    width: '100%',
    height: '16px',
    circle: false,
    rounded: '',
    animate: true,
    count: 1,
  }
);

const computedStyle = computed(() => {
  const s: Record<string, string> = {
    width: props.width,
    height: props.height,
  };
  if (props.circle) {
    s.borderRadius = '50%';
    s.width = props.height; // make it a circle based on height
    s.flexShrink = '0';
  } else if (props.rounded) {
    s.borderRadius = props.rounded;
  }
  return s;
});
</script>

<template>
  <template v-if="count === 1">
    <div
      class="skeleton-box"
      :class="{ 'no-animation': !animate }"
      :style="computedStyle"
    ></div>
  </template>
  <div v-else style="display: flex; flex-direction: column; gap: 8px; width: 100%">
    <div
      v-for="i in count"
      :key="i"
      class="skeleton-box"
      :class="{ 'no-animation': !animate }"
      :style="{
        ...computedStyle,
        width: i === count && count > 1 ? '75%' : computedStyle.width,
      }"
    ></div>
  </div>
</template>

<style scoped>
.skeleton-box {
  background: linear-gradient(
    90deg,
    var(--skeleton-base, rgba(255, 255, 255, 0.05)) 0%,
    var(--skeleton-highlight, rgba(255, 255, 255, 0.13)) 50%,
    var(--skeleton-base, rgba(255, 255, 255, 0.05)) 100%
  );
  background-size: 200% 100%;
  animation: skeleton-shimmer 1.6s infinite ease-in-out;
  border-radius: 6px;
  display: inline-block;
  vertical-align: middle;
  line-height: 1;
}

.skeleton-box.no-animation {
  animation: none;
  background: var(--skeleton-base, rgba(255, 255, 255, 0.05));
}
</style>
