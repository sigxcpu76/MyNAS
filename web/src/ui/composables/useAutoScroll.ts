import { nextTick, ref } from 'vue';

export function useAutoScroll(threshold = 40) {
  const containerRef = ref<HTMLElement | null>(null);
  const autoScroll = ref(true);

  async function scrollToBottom(force = false) {
    await nextTick();
    if (!containerRef.value) return;
    if (force || autoScroll.value) {
      containerRef.value.scrollTop = containerRef.value.scrollHeight;
    }
  }

  function onScroll() {
    if (!containerRef.value) return;
    const el = containerRef.value;
    const isAtBottom = el.scrollHeight - el.scrollTop - el.clientHeight <= threshold;
    autoScroll.value = isAtBottom;
  }

  function toggleAutoScroll() {
    autoScroll.value = !autoScroll.value;
    if (autoScroll.value) {
      scrollToBottom(true);
    }
  }

  return {
    containerRef,
    autoScroll,
    scrollToBottom,
    onScroll,
    toggleAutoScroll,
  };
}
