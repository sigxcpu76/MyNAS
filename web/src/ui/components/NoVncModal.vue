<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import type { VirtualMachineConsoleSession } from '../../domain/models';
import { RfbConsoleAdapter } from '../../adapters/novnc/rfbConsoleAdapter';

const props = defineProps<{
  session: VirtualMachineConsoleSession;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
}>();

const screenRef = ref<HTMLDivElement | null>(null);
const statusText = ref<'CONNECTING' | 'CONNECTED' | 'DISCONNECTED' | 'ERROR'>('CONNECTING');
const statusDetail = ref('Connecting to live QEMU VNC server...');
const quickText = ref('');
const isFullscreen = ref(false);
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

function toggleFullscreen() {
  if (!screenRef.value) return;
  if (!document.fullscreenElement) {
    screenRef.value.requestFullscreen().then(() => {
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
      vmName: props.session.vmName,
      token: props.session.token,
      websocketUrl: props.session.websocketUrl,
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
  <div class="modal-backdrop" @click.self="emit('close')">
    <div class="modal-card" style="max-width: 1024px; width: 95vw">
      <div class="card-header">
        <div>
          <div style="display: flex; align-items: center; gap: 10px">
            <h3 class="card-title">Live QEMU VNC Console — {{ session.vmName }}</h3>
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
          <p class="card-subtitle mono" style="margin-top: 3px">
            {{ statusDetail }}
            <span v-if="session.vncPort"> | Port: {{ session.vncPort }}</span>
          </p>
        </div>

        <div style="display: flex; gap: 8px">
          <button class="btn btn-secondary btn-sm" @click="toggleFullscreen" title="Toggle Fullscreen">
            {{ isFullscreen ? 'Exit Fullscreen' : 'Fullscreen' }}
          </button>
          <button class="btn btn-danger btn-sm" @click="sendCtrlAltDel">
            Send Ctrl+Alt+Del
          </button>
          <button class="btn btn-sm" @click="emit('close')">Close Console</button>
        </div>
      </div>

      <div class="vnc-wrapper">
        <div ref="screenRef" class="vnc-screen-container" tabindex="0"></div>
      </div>

      <form
        @submit.prevent="sendQuickText"
        style="display: flex; gap: 8px; align-items: center; margin-top: 10px"
      >
        <input
          v-model="quickText"
          type="text"
          class="input mono"
          style="flex: 1"
          placeholder="Paste or type text to send directly to guest machine..."
        />
        <button type="submit" class="btn btn-secondary btn-sm" :disabled="statusText !== 'CONNECTED'">
          Send Text + Enter
        </button>
      </form>
    </div>
  </div>
</template>

<style scoped>
.vnc-wrapper {
  background: #000;
  border: 1px solid var(--border-strong);
  border-radius: 8px;
  overflow: hidden;
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 480px;
}

.vnc-screen-container {
  width: 100%;
  height: 560px;
  display: flex;
  justify-content: center;
  align-items: center;
  outline: none;
  background: #000;
}

.vnc-screen-container:fullscreen {
  width: 100vw;
  height: 100vh;
  display: flex;
  justify-content: center;
  align-items: center;
  background: #000;
}

:deep(.vnc-screen-container canvas) {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  outline: none;
}
</style>
