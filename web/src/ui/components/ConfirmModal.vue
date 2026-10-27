<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
import {
  mdiAlert,
  mdiAlertCircle,
  mdiCheck,
  mdiClose,
  mdiContentCopy,
  mdiHelpCircle,
  mdiInformation,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import { activeModal, closeActiveModal } from '../modal';

const confirmBtnRef = ref<HTMLButtonElement | null>(null);
const promptInputRef = ref<HTMLInputElement | null>(null);
const phraseInputRef = ref<HTMLInputElement | null>(null);
const copiedPhrase = ref(false);

async function copyPhrase() {
  if (!activeModal.value?.confirmPhrase) return;
  try {
    if (navigator?.clipboard?.writeText) {
      await navigator.clipboard.writeText(activeModal.value.confirmPhrase);
    } else {
      const textarea = document.createElement('textarea');
      textarea.value = activeModal.value.confirmPhrase;
      textarea.style.position = 'fixed';
      textarea.style.opacity = '0';
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand('copy');
      document.body.removeChild(textarea);
    }
    copiedPhrase.value = true;
    setTimeout(() => {
      copiedPhrase.value = false;
    }, 2000);
  } catch {
    // Fallback if clipboard API is restricted
  }
}

const phraseMatches = computed(
  () => !activeModal.value?.confirmPhrase || activeModal.value.confirmPhraseValue === activeModal.value.confirmPhrase
);

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
      return 'var(--danger-text)';
    case 'warning':
      return 'var(--warning-text)';
    case 'info':
      return 'var(--info-text)';
    default:
      return 'var(--accent-primary)';
  }
});

const iconBg = computed(() => {
  if (!activeModal.value) return 'var(--accent-bg-subtle)';
  switch (activeModal.value.tone) {
    case 'danger':
      return 'var(--danger-bg)';
    case 'warning':
      return 'var(--warning-bg)';
    case 'info':
      return 'var(--info-bg)';
    default:
      return 'var(--accent-bg-subtle)';
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
  if (!phraseMatches.value) return;
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
        if (activeModal.value?.confirmPhrase) {
          phraseInputRef.value?.focus();
        } else if (activeModal.value?.prompt) {
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
          class="modal-close-btn"
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

        <!-- Type-to-confirm safety gate -->
        <div v-if="activeModal.confirmPhrase" class="confirm-prompt-group">
          <div class="confirm-prompt-header">
            <span class="confirm-prompt-label">
              Type <code class="mono confirm-phrase" title="Click to copy" @click="copyPhrase">{{ activeModal.confirmPhrase }}</code> to confirm
            </span>
            <button
              type="button"
              class="phrase-copy-btn"
              :title="copiedPhrase ? 'Copied to clipboard' : 'Copy phrase'"
              @click="copyPhrase"
            >
              <MdiIcon :path="copiedPhrase ? mdiCheck : mdiContentCopy" size="13" />
              <span>{{ copiedPhrase ? 'Copied' : 'Copy' }}</span>
            </button>
          </div>
          <input
            id="confirm-phrase-input"
            ref="phraseInputRef"
            v-model="activeModal.confirmPhraseValue"
            type="text"
            class="input mono"
            autocomplete="off"
            autocapitalize="off"
            spellcheck="false"
            :placeholder="activeModal.confirmPhrase"
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
          :disabled="!phraseMatches"
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

.modal-close-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border-radius: 6px;
  background: transparent;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.15s ease;
}

.modal-close-btn:hover {
  background: rgba(255, 255, 255, 0.08);
  color: var(--text-primary);
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
  background: var(--danger-bg);
  border: 1px solid var(--danger-border);
  color: var(--danger-text);
}

.confirm-details-box.tone-warning {
  background: var(--warning-bg);
  border: 1px solid var(--warning-border);
  color: var(--warning-text);
}

.confirm-details-box.tone-info,
.confirm-details-box.tone-primary {
  background: var(--info-bg);
  border: 1px solid var(--info-border);
  color: var(--text-primary);
}

.confirm-prompt-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.confirm-prompt-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.confirm-prompt-label {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-secondary);
}

.confirm-phrase {
  color: var(--danger-text);
  background: var(--danger-bg);
  border: 1px solid var(--danger-border);
  padding: 1px 6px;
  border-radius: 4px;
  cursor: pointer;
  user-select: all;
  transition: filter 0.15s ease;
}

.confirm-phrase:hover {
  filter: brightness(1.2);
}

.phrase-copy-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  font-weight: 500;
  padding: 2px 7px;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.15s ease;
  line-height: 1.4;
}

.phrase-copy-btn:hover {
  background: rgba(255, 255, 255, 0.09);
  color: var(--text-primary);
  border-color: var(--border-strong);
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
