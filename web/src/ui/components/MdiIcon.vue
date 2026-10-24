<script setup lang="ts">
withDefaults(
  defineProps<{
    path: string;
    size?: number | string;
    color?: string;
  }>(),
  {
    size: 16,
    color: 'currentColor',
  }
);
</script>

<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    :style="{
      display: 'inline-block',
      verticalAlign: '-2px',
      flexShrink: 0,
      fill: color,
    }"
  >
    <path :d="path" />
  </svg>
</template>
