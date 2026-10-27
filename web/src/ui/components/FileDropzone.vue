<script setup lang="ts">
import { ref } from 'vue';
import { mdiCloudUploadOutline, mdiFileUploadOutline } from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import { formatBytes } from '../../domain/models';

export interface UploadProgressInfo {
  percent: number;
  loaded: number;
  total: number;
  filename: string;
}

const props = withDefaults(
  defineProps<{
    accept?: string;
    label?: string;
    sublabel?: string;
    icon?: string;
    isUploading?: boolean;
    progress?: UploadProgressInfo | null;
    disabled?: boolean;
  }>(),
  {
    accept: '',
    label: 'Drag & drop file here, or click to browse',
    sublabel: '',
    icon: '',
    isUploading: false,
    progress: null,
    disabled: false,
  }
);

const emit = defineEmits<{
  (e: 'file-selected', file: File): void;
  (e: 'invalid-file', filename: string, reason: string): void;
}>();

const fileInputRef = ref<HTMLInputElement | null>(null);
const isDragOver = ref(false);
let dragCounter = 0;

function openFileDialog() {
  if (props.disabled || props.isUploading) return;
  fileInputRef.value?.click();
}

defineExpose({
  openFileDialog,
});

function matchesAccept(file: File, acceptStr: string): boolean {
  if (!acceptStr) return true;
  const allowed = acceptStr
    .split(',')
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean);
  const fileName = file.name.toLowerCase();

  return allowed.some((ext) => {
    if (ext.startsWith('.')) {
      return fileName.endsWith(ext);
    }
    // MIME type check
    return file.type === ext || (ext.endsWith('/*') && file.type.startsWith(ext.slice(0, -1)));
  });
}

function processFile(file: File) {
  if (props.accept && !matchesAccept(file, props.accept)) {
    emit(
      'invalid-file',
      file.name,
      `Invalid file type for "${file.name}". Supported formats: ${props.accept}`
    );
    return;
  }
  emit('file-selected', file);
}

function onFileInputChange(e: Event) {
  const target = e.target as HTMLInputElement;
  const file = target.files?.[0];
  if (file) {
    processFile(file);
  }
  if (fileInputRef.value) {
    fileInputRef.value.value = '';
  }
}

function onDragEnter(e: DragEvent) {
  e.preventDefault();
  if (props.disabled || props.isUploading) return;
  dragCounter++;
  if (e.dataTransfer) {
    e.dataTransfer.dropEffect = 'copy';
  }
  isDragOver.value = true;
}

function onDragOver(e: DragEvent) {
  e.preventDefault();
  if (props.disabled || props.isUploading) return;
  if (e.dataTransfer) {
    e.dataTransfer.dropEffect = 'copy';
  }
}

function onDragLeave(e: DragEvent) {
  e.preventDefault();
  dragCounter--;
  if (dragCounter <= 0) {
    dragCounter = 0;
    isDragOver.value = false;
  }
}

function onDrop(e: DragEvent) {
  e.preventDefault();
  dragCounter = 0;
  isDragOver.value = false;
  if (props.disabled || props.isUploading) return;
  const file = e.dataTransfer?.files?.[0];
  if (file) {
    processFile(file);
  }
}
</script>

<template>
  <div
    class="file-dropzone"
    :class="{
      'drag-over': isDragOver,
      'is-uploading': isUploading,
      'is-disabled': disabled,
    }"
    @click="openFileDialog"
    @dragenter="onDragEnter"
    @dragover="onDragOver"
    @dragleave="onDragLeave"
    @drop="onDrop"
  >
    <input
      ref="fileInputRef"
      type="file"
      :accept="accept"
      style="display: none"
      @change="onFileInputChange"
    />

    <!-- Upload in progress state -->
    <div v-if="isUploading && progress" class="dropzone-uploading">
      <div class="dropzone-uploading-header">
        <div class="dropzone-uploading-title">
          <MdiIcon :path="mdiFileUploadOutline" size="20" class="uploading-icon pulse" />
          <span class="mono filename">{{ progress.filename }}</span>
          <span class="uploading-tag">Uploading...</span>
        </div>
        <div class="mono progress-numbers">
          {{ formatBytes(progress.loaded) }} / {{ formatBytes(progress.total) }}
          <span class="percent-badge">({{ progress.percent }}%)</span>
        </div>
      </div>
      <div class="dropzone-progress-bar">
        <div class="dropzone-progress-fill" :style="{ width: `${progress.percent}%` }"></div>
      </div>
    </div>

    <!-- Default idle / drag-over state -->
    <div v-else class="dropzone-idle">
      <div class="dropzone-icon-wrap" :class="{ 'bounce': isDragOver }">
        <MdiIcon :path="icon || mdiCloudUploadOutline" size="32" class="dropzone-icon" />
      </div>
      <div class="dropzone-text">
        <div class="dropzone-label">
          <template v-if="isDragOver">
            <span class="highlight-text">Drop file to upload immediately</span>
          </template>
          <template v-else>
            {{ label }}
          </template>
        </div>
        <div v-if="sublabel" class="dropzone-sublabel">
          {{ sublabel }}
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.file-dropzone {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  border: 2px dashed var(--border-strong);
  border-radius: 10px;
  padding: 18px 24px;
  background: rgba(255, 255, 255, 0.01);
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  user-select: none;
  position: relative;
  outline: none;
}

.file-dropzone:hover:not(.is-disabled):not(.is-uploading) {
  border-color: var(--accent-primary);
  background: var(--accent-bg-subtle);
}

.file-dropzone.drag-over {
  border-color: var(--accent-primary);
  border-style: solid;
  background: var(--accent-bg-subtle);
  box-shadow: 0 0 16px rgba(122, 162, 247, 0.25);
  transform: translateY(-1px);
}

.file-dropzone.is-uploading {
  border-color: var(--accent-primary);
  background: var(--bg-surface);
  cursor: default;
  padding: 16px 20px;
}

.file-dropzone.is-disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.dropzone-idle {
  display: flex;
  align-items: center;
  gap: 16px;
  width: 100%;
  justify-content: center;
}

.dropzone-icon-wrap {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: 10px;
  background: var(--accent-bg-subtle);
  color: var(--accent-primary);
  flex-shrink: 0;
  transition: transform 0.2s ease;
}

.dropzone-icon-wrap.bounce {
  transform: scale(1.15) translateY(-2px);
}

.dropzone-text {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.dropzone-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.highlight-text {
  color: var(--accent-primary);
  font-weight: 700;
}

.dropzone-sublabel {
  font-size: 11px;
  color: var(--text-secondary);
}

/* Uploading State */
.dropzone-uploading {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 100%;
}

.dropzone-uploading-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
  flex-wrap: wrap;
  gap: 8px;
}

.dropzone-uploading-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}

.uploading-icon {
  color: var(--accent-primary);
}

.pulse {
  animation: pulse 1.5s infinite ease-in-out;
}

@keyframes pulse {
  0%, 100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.5;
    transform: scale(1.1);
  }
}

.filename {
  color: var(--text-primary);
  font-weight: 600;
  max-width: 400px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.uploading-tag {
  font-size: 11px;
  background: var(--accent-bg-subtle);
  color: var(--accent-primary);
  padding: 2px 8px;
  border-radius: 4px;
  font-weight: 500;
}

.progress-numbers {
  font-size: 12px;
  color: var(--text-secondary);
}

.percent-badge {
  font-weight: 700;
  color: var(--accent-primary);
  margin-left: 4px;
}

.dropzone-progress-bar {
  width: 100%;
  height: 8px;
  background: var(--bg-hover);
  border-radius: 4px;
  overflow: hidden;
}

.dropzone-progress-fill {
  height: 100%;
  background: var(--accent-primary);
  border-radius: 4px;
  transition: width 0.2s ease;
}
</style>
