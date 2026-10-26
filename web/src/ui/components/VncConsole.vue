<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import {
  mdiFullscreen,
  mdiFullscreenExit,
  mdiOpenInNew,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import {
  RfbConsoleAdapter,
  type ScaleMode,
} from '../../adapters/novnc/rfbConsoleAdapter';

const props = withDefaults(
  defineProps<{
    vmName: string;
    token: string;
    vncPort?: number;
    websocketUrl?: string;
    standalone?: boolean;
  }>(),
  {
    vncPort: 5900,
    websocketUrl: undefined,
    standalone: false,
  }
);

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'detach'): void;
}>();

const screenRef = ref<HTMLDivElement | null>(null);
const wrapperRef = ref<HTMLDivElement | null>(null);
const statusText = ref<'CONNECTING' | 'CONNECTED' | 'DISCONNECTED' | 'ERROR'>('CONNECTING');
const statusDetail = ref('Connecting to live QEMU VNC server...');
const quickText = ref('');
const isFullscreen = ref(false);
const scaleMode = ref<ScaleMode>('fit');

let adapter: RfbConsoleAdapter | null = null;

function sendCtrlAltDel() {
  adapter?.sendCtrlAltDel();
  adapter?.focus();
}

function sendQuickText() {
  if (!adapter || !quickText.value) return;
  adapter.sendText(quickText.value);
  adapter.sendEnter();
  quickText.value = '';
  adapter.focus();
}

function handleScaleChange() {
  adapter?.setScaleMode(scaleMode.value);
}

function toggleFullscreen() {
  const target = wrapperRef.value || screenRef.value;
  if (!target) return;
  if (!document.fullscreenElement) {
    target.requestFullscreen().then(() => {
      isFullscreen.value = true;
    }).catch(() => {});
  } else {
    document.exitFullscreen().then(() => {
      isFullscreen.value = false;
    }).catch(() => {});
  }
}

function handleFullscreenChange() {
  isFullscreen.value = !!document.fullscreenElement;
}

onMounted(() => {
  document.addEventListener('fullscreenchange', handleFullscreenChange);
  if (screenRef.value) {
    adapter = new RfbConsoleAdapter({
      target: screenRef.value,
      vmName: props.vmName,
      token: props.token,
      websocketUrl: props.websocketUrl,
      scaleMode: scaleMode.value,
      crisp: false,
      onStatusChange: (status, detail) => {
        statusText.value = status;
        if (detail) statusDetail.value = detail;
      },
    });
    adapter.connect();
  }
});

onBeforeUnmount(() => {
  document.removeEventListener('fullscreenchange', handleFullscreenChange);
  adapter?.disconnect();
  adapter = null;
});
</script>

<template>
  <div class="vnc-console-root" :class="{ 'is-standalone-root': standalone }">
    <!-- Header / Control Toolbar -->
    <div class="vnc-header">
      <div class="header-left">
        <div style="display: flex; align-items: center; gap: 10px; flex-wrap: wrap">
          <h3 class="console-title">Live QEMU VNC Console — {{ vmName }}</h3>
          <span
            class="badge"
            :class="{
              'badge-success': statusText === 'CONNECTED',
              'badge-info': statusText === 'CONNECTING',
              'badge-danger': statusText === 'ERROR' || statusText === 'DISCONNECTED',
            }"
          >
            {{ statusText }}
          </span>
        </div>
        <p class="console-subtitle mono">
          {{ statusDetail }}
          <span v-if="vncPort"> | Port: {{ vncPort }}</span>
        </p>
      </div>

      <div class="header-actions">
        <!-- Scale Mode Selector -->
        <div class="vnc-control-pill" title="Viewport Scaling Mode">
          <span class="control-label">Scale:</span>
          <select
            v-model="scaleMode"
            class="select-inline mono"
            @change="handleScaleChange"
          >
            <option value="fit">Fit Window</option>
            <option value="1:1">100% (1:1)</option>
            <option value="125">125%</option>
            <option value="150">150%</option>
            <option value="200">200%</option>
          </select>
        </div>

        <!-- Fullscreen Button -->
        <button
          class="btn btn-secondary btn-sm"
          :title="isFullscreen ? 'Exit Fullscreen' : 'Enter Fullscreen'"
          @click="toggleFullscreen"
        >
          <MdiIcon :path="isFullscreen ? mdiFullscreenExit : mdiFullscreen" size="14" style="margin-right: 4px" />
          <span>{{ isFullscreen ? 'Exit Fullscreen' : 'Fullscreen' }}</span>
        </button>

        <!-- Ctrl+Alt+Del -->
        <button class="btn btn-danger btn-sm" title="Send Ctrl+Alt+Del" @click="sendCtrlAltDel">
          Send Ctrl+Alt+Del
        </button>

        <!-- Detach Button (only shown when not standalone) -->
        <button
          v-if="!standalone"
          class="btn btn-secondary btn-sm"
          title="Detach console into independent window"
          @click="emit('detach')"
        >
          <MdiIcon :path="mdiOpenInNew" size="14" style="margin-right: 4px" />
          <span>Detach</span>
        </button>

        <!-- Close / Exit -->
        <button class="btn btn-sm" @click="emit('close')">
          {{ standalone ? 'Close Window' : 'Close Console' }}
        </button>
      </div>
    </div>

    <!-- Screen Canvas Viewport -->
    <div
      ref="wrapperRef"
      class="vnc-wrapper"
      :class="{
        'is-standalone': standalone,
        'scale-fit-mode': scaleMode === 'fit',
        'scale-manual-mode': scaleMode !== 'fit',
      }"
    >
      <div
        ref="screenRef"
        class="vnc-screen-container"
        tabindex="0"
      ></div>
    </div>

    <!-- Quick Text Input Bar -->
    <form
      class="vnc-footer"
      @submit.prevent="sendQuickText"
    >
      <input
        v-model="quickText"
        type="text"
        class="input mono"
        style="flex: 1"
        placeholder="Paste or type text to send directly to guest machine..."
      />
      <button
        type="submit"
        class="btn btn-secondary btn-sm"
        :disabled="statusText !== 'CONNECTED'"
      >
        Send Text + Enter
      </button>
    </form>
  </div>
</template>

<style scoped>
.vnc-console-root {
  display: flex;
  flex-direction: column;
  width: 100%;
  gap: 10px;
}

.vnc-console-root.is-standalone-root {
  height: 100vh;
  box-sizing: border-box;
  padding: 12px;
  background: var(--bg-primary, #0f172a);
}

.vnc-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}

.console-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
}

.console-subtitle {
  margin: 3px 0 0 0;
  font-size: 12px;
  color: var(--text-secondary);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.vnc-control-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--bg-secondary, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 6px;
  padding: 2px 8px;
  height: 28px;
}

.control-label {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.select-inline {
  background: transparent;
  color: var(--text-primary);
  border: none;
  outline: none;
  font-size: 12px;
  cursor: pointer;
}

.select-inline option {
  background: #1e293b;
  color: #f8fafc;
}

.vnc-wrapper {
  background: #000;
  border: 1px solid var(--border-strong, #334155);
  border-radius: 8px;
  overflow: auto;
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 480px;
  height: 580px;
  max-height: calc(85vh - 130px);
  position: relative;
}

.vnc-wrapper.is-standalone {
  flex: 1;
  min-height: 360px;
  height: calc(100vh - 125px);
  max-height: none;
}

.vnc-wrapper:fullscreen {
  width: 100vw;
  height: 100vh;
  max-height: 100vh;
  border-radius: 0;
  border: none;
}

.vnc-screen-container {
  display: flex;
  justify-content: center;
  align-items: center;
  outline: none;
  background: #000;
  margin: auto;
}

.vnc-wrapper.scale-fit-mode .vnc-screen-container {
  width: 100%;
  height: 100%;
}

.vnc-wrapper.scale-fit-mode :deep(canvas) {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  margin: auto;
}

.vnc-wrapper.scale-manual-mode .vnc-screen-container {
  width: auto;
  height: auto;
}

.vnc-wrapper.scale-manual-mode :deep(canvas) {
  max-width: none !important;
  max-height: none !important;
}

.vnc-footer {
  display: flex;
  gap: 8px;
  align-items: center;
}
</style>
