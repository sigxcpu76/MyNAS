<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue';
import { mdiClose } from '@mdi/js';
import MdiIcon from './MdiIcon.vue';

const props = withDefaults(
  defineProps<{
    modelValue?: boolean;
    title?: string;
    subtitle?: string;
    maxHeight?: string;
    showCloseButton?: boolean;
  }>(),
  {
    modelValue: false,
    title: '',
    subtitle: '',
    maxHeight: '88vh',
    showCloseButton: true,
  }
);

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void;
  (e: 'close'): void;
}>();

const translateY = ref(0);
const isDragging = ref(false);
const startY = ref(0);
const currentY = ref(0);

function close() {
  emit('update:modelValue', false);
  emit('close');
}

function onTouchStart(e: TouchEvent) {
  if (e.touches.length !== 1) return;
  startY.value = e.touches[0].clientY;
  currentY.value = e.touches[0].clientY;
  isDragging.value = true;
}

function onTouchMove(e: TouchEvent) {
  if (!isDragging.value || e.touches.length !== 1) return;
  currentY.value = e.touches[0].clientY;
  const delta = currentY.value - startY.value;
  // Only allow dragging downwards
  if (delta > 0) {
    translateY.value = delta;
    if (e.cancelable) e.preventDefault();
  } else {
    translateY.value = 0;
  }
}

function onTouchEnd() {
  if (!isDragging.value) return;
  isDragging.value = false;
  const delta = currentY.value - startY.value;
  if (delta > 50) {
    close();
  }
  translateY.value = 0;
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.modelValue) {
    close();
  }
}

let prevDismissOverlay: (() => boolean) | undefined;

watch(
  () => props.modelValue,
  (val) => {
    translateY.value = 0;
    if (typeof document !== 'undefined') {
      if (val) {
        document.body.style.overflow = 'hidden';
      } else {
        document.body.style.overflow = '';
      }
    }
  }
);

onMounted(() => {
  if (typeof window !== 'undefined') {
    window.addEventListener('keydown', handleKeydown);
    prevDismissOverlay = (window as any).nanDismissTopOverlay;
    (window as any).nanDismissTopOverlay = () => {
      if (props.modelValue) {
        close();
        return true;
      }
      return typeof prevDismissOverlay === 'function' ? prevDismissOverlay() : false;
    };
  }
});

onUnmounted(() => {
  if (typeof window !== 'undefined') {
    window.removeEventListener('keydown', handleKeydown);
    if (prevDismissOverlay) {
      (window as any).nanDismissTopOverlay = prevDismissOverlay;
    }
    document.body.style.overflow = '';
  }
});
</script>

<template>
  <Teleport to="body">
    <Transition name="sheet-fade">
      <div v-if="modelValue" class="bottom-sheet-backdrop" @click.self="close">
        <div
          class="bottom-sheet-surface"
          :style="{
            maxHeight: maxHeight,
            transform: isDragging && translateY > 0 ? `translateY(${translateY}px)` : undefined,
            transition: isDragging ? 'none' : 'transform 0.28s cubic-bezier(0.16, 1, 0.3, 1)',
          }"
        >
          <!-- Drag Handle Header (Touch Gesture Zone) -->
          <div
            class="bottom-sheet-drag-area"
            @touchstart="onTouchStart"
            @touchmove="onTouchMove"
            @touchend="onTouchEnd"
          >
            <div class="drag-handle-pill" />
          </div>

          <!-- Title Bar -->
          <div
            v-if="title || $slots.header"
            class="bottom-sheet-header"
            @touchstart="onTouchStart"
            @touchmove="onTouchMove"
            @touchend="onTouchEnd"
          >
            <slot name="header">
              <div class="bottom-sheet-header-text">
                <h3 class="bottom-sheet-title">{{ title }}</h3>
                <p v-if="subtitle" class="bottom-sheet-subtitle">{{ subtitle }}</p>
              </div>
              <div class="bottom-sheet-header-actions">
                <slot name="header-actions" />
                <button
                  v-if="showCloseButton"
                  class="btn btn-xs btn-ghost btn-icon"
                  aria-label="Close sheet"
                  @click="close"
                >
                  <MdiIcon :path="mdiClose" size="18" />
                </button>
              </div>
            </slot>
          </div>

          <!-- Scrollable Body Content -->
          <div class="bottom-sheet-body">
            <slot />
          </div>

          <!-- Optional Sticky Footer -->
          <div v-if="$slots.footer" class="bottom-sheet-footer">
            <slot name="footer" />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.bottom-sheet-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1200;
  background: rgba(0, 0, 0, 0.65);
  backdrop-filter: blur(4px);
  -webkit-backdrop-filter: blur(4px);
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
}

.bottom-sheet-surface {
  width: 100%;
  background: var(--bg-surface, #1e293b);
  border-top-left-radius: 20px;
  border-top-right-radius: 20px;
  border-top: 1px solid var(--border-color, #334155);
  box-shadow: 0 -10px 25px -5px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
  padding-bottom: calc(16px + env(safe-area-inset-bottom, 0px));
  touch-action: pan-y;
  will-change: transform;
}

.bottom-sheet-drag-area {
  width: 100%;
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 14px 0 8px 0;
  min-height: 28px;
  cursor: grab;
}

.drag-handle-pill {
  width: 36px;
  height: 4px;
  border-radius: 999px;
  background: var(--border-color-strong, #64748b);
  opacity: 0.8;
}

.bottom-sheet-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 16px 12px 16px;
  border-bottom: 1px solid var(--border-color, #334155);
  gap: 12px;
}

.bottom-sheet-header-text {
  flex: 1;
  min-width: 0;
}

.bottom-sheet-title {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.bottom-sheet-subtitle {
  margin: 2px 0 0 0;
  font-size: 11px;
  color: var(--text-secondary, #94a3b8);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.bottom-sheet-header-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.bottom-sheet-body {
  flex: 1;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
  padding: 16px;
}

.bottom-sheet-footer {
  padding: 12px 16px 0 16px;
  border-top: 1px solid var(--border-color, #334155);
}

.sheet-fade-enter-active,
.sheet-fade-leave-active {
  transition: opacity 0.25s ease;
}

.sheet-fade-enter-from,
.sheet-fade-leave-to {
  opacity: 0;
}

.sheet-fade-enter-active .bottom-sheet-surface {
  transition: transform 0.28s cubic-bezier(0.16, 1, 0.3, 1);
}

.sheet-fade-leave-active .bottom-sheet-surface {
  transition: transform 0.22s cubic-bezier(0.4, 0, 1, 1);
}

.sheet-fade-enter-from .bottom-sheet-surface,
.sheet-fade-leave-to .bottom-sheet-surface {
  transform: translateY(100%);
}
</style>
