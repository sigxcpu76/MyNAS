<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
import {
  mdiAlert,
  mdiAlertCircle,
  mdiClose,
  mdiHelpCircle,
  mdiInformation,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import { activeModal, closeActiveModal } from '../modal';

const confirmBtnRef = ref<HTMLButtonElement | null>(null);
const promptInputRef = ref<HTMLInputElement | null>(null);

const iconPath = computed(() => {
  if (!activeModal.value) return mdiHelpCircle;
  if (activeModal.value.icon) return activeModal.value.icon;
  switch (activeModal.value.tone) {
    case 'danger':
      return mdiAlertCircle;
    case 'warning':
      return mdiAlert;
    case 'info':
      return mdiInformation;
    default:
      return mdiHelpCircle;
  }
});

const iconColor = computed(() => {
  if (!activeModal.value) return 'var(--accent-primary)';
  switch (activeModal.value.tone) {
    case 'danger':
      return '#ef4444';
    case 'warning':
      return '#f59e0b';
    case 'info':
      return '#38bdf8';
    default:
      return 'var(--accent-primary)';
  }
});

const iconBg = computed(() => {
  if (!activeModal.value) return 'rgba(56, 189, 248, 0.12)';
  switch (activeModal.value.tone) {
    case 'danger':
      return 'rgba(239, 68, 68, 0.15)';
    case 'warning':
      return 'rgba(245, 158, 11, 0.15)';
    case 'info':
      return 'rgba(56, 189, 248, 0.12)';
    default:
      return 'rgba(56, 189, 248, 0.12)';
  }
});

const confirmBtnClass = computed(() => {
  if (!activeModal.value) return 'btn btn-primary';
  switch (activeModal.value.tone) {
    case 'danger':
      return 'btn btn-danger';
    case 'warning':
      return 'btn btn-warning';
    default:
      return 'btn btn-primary';
  }
});

function handleCancel() {
  closeActiveModal(false);
}

function handleConfirm() {
  if (activeModal.value?.prompt?.required && !activeModal.value.inputValue.trim()) {
    return;
  }
  closeActiveModal(true);
}

function handleKeydown(e: KeyboardEvent) {
  if (!activeModal.value) return;
  if (e.key === 'Escape') {
    e.preventDefault();
    handleCancel();
  } else if (e.key === 'Enter' && !e.shiftKey) {
    // If not in a textarea
    const target = e.target as HTMLElement;
    if (target.tagName !== 'TEXTAREA') {
      e.preventDefault();
      handleConfirm();
    }
  }
}

watch(
  () => activeModal.value?.isOpen,
  (open) => {
    if (open) {
      nextTick(() => {
        if (activeModal.value?.prompt) {
          promptInputRef.value?.focus();
        } else {
          confirmBtnRef.value?.focus();
        }
      });
    }
  }
);

onMounted(() => {
  window.addEventListener('keydown', handleKeydown);
});

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown);
});
</script>

<template>
  <div
    v-if="activeModal && activeModal.isOpen"
    class="modal-backdrop"
    @click.self="handleCancel"
  >
    <div class="modal-card confirm-modal-card" role="dialog" aria-modal="true">
      <!-- Header -->
      <div class="confirm-modal-header">
        <div class="confirm-icon-box" :style="{ backgroundColor: iconBg }">
          <MdiIcon :path="iconPath" size="24" :style="{ color: iconColor }" />
        </div>
        <div style="flex: 1; min-width: 0">
          <h3 class="confirm-title">
            {{ activeModal.title || (activeModal.alertOnly ? 'Notice' : 'Confirmation') }}
          </h3>
        </div>
        <button
          class="btn-icon btn-sm"
          type="button"
          aria-label="Close"
          @click="handleCancel"
        >
          <MdiIcon :path="mdiClose" size="18" />
        </button>
      </div>

      <!-- Body -->
      <div class="confirm-modal-body">
        <div class="confirm-message">
          {{ activeModal.message }}
        </div>

        <div v-if="activeModal.details" class="confirm-details-box" :class="`tone-${activeModal.tone || 'primary'}`">
          {{ activeModal.details }}
        </div>

        <!-- Optional Prompt Input -->
        <div v-if="activeModal.prompt" class="confirm-prompt-group">
          <label v-if="activeModal.prompt.label" class="confirm-prompt-label">
            {{ activeModal.prompt.label }}
          </label>
          <input
            ref="promptInputRef"
            v-model="activeModal.inputValue"
            type="text"
            class="input mono"
            :placeholder="activeModal.prompt.placeholder || ''"
            :required="activeModal.prompt.required"
          />
        </div>

        <!-- Optional Checkbox Option -->
        <div v-if="activeModal.checkbox" class="confirm-checkbox-row">
          <label class="checkbox-label-wrapper">
            <input
              v-model="activeModal.checkboxValue"
              type="checkbox"
              class="confirm-checkbox"
            />
            <div class="checkbox-text-group">
              <span class="checkbox-main-label">{{ activeModal.checkbox.label }}</span>
              <span v-if="activeModal.checkbox.hint" class="checkbox-hint">
                {{ activeModal.checkbox.hint }}
              </span>
            </div>
          </label>
        </div>
      </div>

      <!-- Footer Buttons -->
      <div class="confirm-modal-footer">
        <button
          v-if="!activeModal.alertOnly"
          type="button"
          class="btn"
          @click="handleCancel"
        >
          {{ activeModal.cancelText || 'Cancel' }}
        </button>
        <button
          ref="confirmBtnRef"
          type="button"
          :class="confirmBtnClass"
          @click="handleConfirm"
        >
          {{ activeModal.confirmText }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.confirm-modal-card {
  max-width: 480px;
  padding: 20px 22px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  animation: modalPopIn 0.16s ease-out;
}

@keyframes modalPopIn {
  from {
    opacity: 0;
    transform: scale(0.96) translateY(-4px);
  }
  to {
    opacity: 1;
    transform: scale(1) translateY(0);
  }
}

.confirm-modal-header {
  display: flex;
  align-items: center;
  gap: 14px;
}

.confirm-icon-box {
  width: 42px;
  height: 42px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.confirm-title {
  margin: 0;
  font-size: 17px;
  font-weight: 700;
  color: var(--text-primary);
  line-height: 1.3;
}

.confirm-modal-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.confirm-message {
  font-size: 14px;
  line-height: 1.5;
  color: var(--text-primary);
  white-space: pre-line;
}

.confirm-details-box {
  font-size: 12.5px;
  line-height: 1.45;
  padding: 10px 14px;
  border-radius: 8px;
  white-space: pre-line;
}

.confirm-details-box.tone-danger {
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.35);
  color: #fca5a5;
}

.confirm-details-box.tone-warning {
  background: rgba(245, 158, 11, 0.1);
  border: 1px solid rgba(245, 158, 11, 0.35);
  color: #fde68a;
}

.confirm-details-box.tone-info,
.confirm-details-box.tone-primary {
  background: rgba(56, 189, 248, 0.08);
  border: 1px solid rgba(56, 189, 248, 0.25);
  color: var(--text-secondary);
}

.confirm-prompt-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.confirm-prompt-label {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-secondary);
}

.confirm-checkbox-row {
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 10px 12px;
}

.checkbox-label-wrapper {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  cursor: pointer;
  user-select: none;
}

.confirm-checkbox {
  margin-top: 3px;
  cursor: pointer;
  accent-color: var(--accent-primary);
  width: 16px;
  height: 16px;
}

.checkbox-text-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.checkbox-main-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.checkbox-hint {
  font-size: 11.5px;
  color: var(--text-secondary);
}

.confirm-modal-footer {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 10px;
  margin-top: 4px;
}
</style>
