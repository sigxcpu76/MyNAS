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

const canvasRef = ref<HTMLCanvasElement | null>(null);
const statusText = ref('CONNECTING');
const statusDetail = ref('Negotiating RFB 003.008...');
const quickCmd = ref('');
let adapter: RfbConsoleAdapter | null = null;

function mapKeyToKeysym(e: KeyboardEvent): number {
  if (e.key === 'Enter') return 0xff0d;
  if (e.key === 'Backspace') return 0xff08;
  if (e.key === 'Tab') return 0xff09;
  if (e.key === 'Escape') return 0xff1b;
  if (e.key.length === 1) return e.key.charCodeAt(0);
  return e.keyCode || 0;
}

function onCanvasKeyDown(e: KeyboardEvent) {
  if (!adapter) return;
  if (['Backspace', 'Tab', 'Enter', ' '].includes(e.key)) {
    e.preventDefault();
  }
  const sym = mapKeyToKeysym(e);
  adapter.sendKeyEvent(sym, true, e.key);
}

function onCanvasKeyUp(e: KeyboardEvent) {
  if (!adapter) return;
  const sym = mapKeyToKeysym(e);
  adapter.sendKeyEvent(sym, false);
}

function onCanvasMouseMove(e: MouseEvent) {
  if (!adapter || !canvasRef.value) return;
  const rect = canvasRef.value.getBoundingClientRect();
  adapter.sendPointerEvent(e.clientX - rect.left, e.clientY - rect.top, e.buttons);
}

function sendCtrlAltDel() {
  adapter?.sendCtrlAltDel();
  canvasRef.value?.focus();
}

function sendQuickCommand() {
  if (!adapter || !quickCmd.value.trim()) return;
  for (const ch of quickCmd.value) {
    adapter.sendKeyEvent(ch.charCodeAt(0), true, ch);
    adapter.sendKeyEvent(ch.charCodeAt(0), false);
  }
  adapter.sendKeyEvent(0xff0d, true, 'Enter');
  adapter.sendKeyEvent(0xff0d, false);
  quickCmd.value = '';
  canvasRef.value?.focus();
}

onMounted(() => {
  if (canvasRef.value) {
    adapter = new RfbConsoleAdapter({
      canvas: canvasRef.value,
      vmName: props.session.vmName,
      token: props.session.token,
      websocketUrl: props.session.websocketUrl,
      onStatusChange: (status, detail) => {
        statusText.value = status;
        if (detail) statusDetail.value = detail;
      },
    });
    adapter.connect();
    canvasRef.value.focus();
  }
});

onBeforeUnmount(() => {
  adapter?.disconnect();
});
</script>

<template>
  <div class="modal-backdrop" @click.self="emit('close')">
    <div class="modal-card" style="max-width: 880px">
      <div class="card-header">
        <div>
          <div style="display: flex; align-items: center; gap: 10px">
            <h3 class="card-title">noVNC / RFB 003.008 Graphical Console — {{ session.vmName }}</h3>
            <span
              class="badge"
              :class="statusText === 'CONNECTED' ? 'badge-success' : 'badge-info'"
            >
              {{ statusText }}
            </span>
          </div>
          <p class="card-subtitle mono" style="margin-top: 3px">
            WebSocket: {{ session.websocketUrl || `/ws/novnc/${session.vmName}` }} | VNC Port:
            {{ session.vncPort }} | Resolution: {{ session.resolution || '1024x768' }}
          </p>
        </div>

        <div style="display: flex; gap: 8px">
          <button class="btn btn-danger btn-sm" @click="sendCtrlAltDel">
            Send Ctrl+Alt+Del
          </button>
          <button class="btn btn-sm" @click="emit('close')">Close Console</button>
        </div>
      </div>

      <div
        style="
          display: flex;
          justify-content: center;
          background: #03060c;
          border: 1px solid var(--border-strong);
          border-radius: 8px;
          padding: 8px;
          margin-bottom: 12px;
        "
      >
        <canvas
          ref="canvasRef"
          width="800"
          height="430"
          tabindex="0"
          style="outline: none; cursor: crosshair; max-width: 100%; border-radius: 4px"
          @keydown="onCanvasKeyDown"
          @keyup="onCanvasKeyUp"
          @mousemove="onCanvasMouseMove"
        ></canvas>
      </div>

      <form
        @submit.prevent="sendQuickCommand"
        style="display: flex; gap: 8px; align-items: center"
      >
        <input
          v-model="quickCmd"
          type="text"
          class="input mono"
          style="flex: 1"
          placeholder="Send keystrokes to guest console (e.g. uname -a, ip a, free -h, clear)..."
        />
        <button type="submit" class="btn btn-primary btn-sm">Send Keys + Enter</button>
      </form>
    </div>
  </div>
</template>
