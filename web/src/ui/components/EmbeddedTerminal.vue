<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import MdiIcon from './MdiIcon.vue';
import { services } from '../../container';
import {
  mdiConsole,
  mdiOpenInNew,
  mdiRefresh,
  mdiTrashCanOutline,
} from '@mdi/js';

const props = withDefaults(
  defineProps<{
    containerId: string;
    containerName?: string;
    endpointType?: 'lxc' | 'docker';
    initialShell?: string;
  }>(),
  {
    endpointType: 'lxc',
    initialShell: '/bin/sh',
  }
);

const emit = defineEmits<{
  (e: 'popout'): void;
}>();

const selectedShell = ref(props.initialShell);
const status = ref<'CONNECTING' | 'CONNECTED' | 'DISCONNECTED' | 'ERROR'>('CONNECTING');
const terminalContainer = ref<HTMLDivElement | null>(null);

let term: Terminal | null = null;
let fitAddon: FitAddon | null = null;
let ws: WebSocket | null = null;
let resizeObserver: ResizeObserver | null = null;

function getWebSocketUrl(id: string, shell: string): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  const basePath = props.endpointType === 'lxc' ? '/ws/lxc/terminal/' : '/ws/docker/terminal/';
  const token = services.auth.getToken() ?? '';
  return `${protocol}//${host}${basePath}${encodeURIComponent(id)}?shell=${encodeURIComponent(shell)}&access_token=${encodeURIComponent(token)}`;
}

function sendResize() {
  if (!term || !ws || ws.readyState !== WebSocket.OPEN) return;
  try {
    const payload = JSON.stringify({
      type: 'resize',
      cols: term.cols,
      rows: term.rows,
    });
    ws.send(payload);
  } catch {
    // Ignore error when socket is closing
  }
}

function initTerminal() {
  if (!terminalContainer.value) return;

  if (term) {
    term.dispose();
    term = null;
  }

  terminalContainer.value.innerHTML = '';

  term = new Terminal({
    cursorBlink: true,
    cursorStyle: 'block',
    fontSize: 13,
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
    lineHeight: 1.25,
    theme: {
      background: '#0a0f1d',
      foreground: '#e2e8f0',
      cursor: '#38bdf8',
      cursorAccent: '#0a0f1d',
      selectionBackground: 'rgba(56, 189, 248, 0.35)',
      selectionForeground: '#ffffff',
      black: '#1e293b',
      red: '#ef4444',
      green: '#10b981',
      yellow: '#f59e0b',
      blue: '#3b82f6',
      magenta: '#ec4899',
      cyan: '#06b6d4',
      white: '#f8fafc',
      brightBlack: '#64748b',
      brightRed: '#f87171',
      brightGreen: '#34d399',
      brightYellow: '#fbbf24',
      brightBlue: '#60a5fa',
      brightMagenta: '#f472b6',
      brightCyan: '#22d3ee',
      brightWhite: '#ffffff',
    },
  });

  fitAddon = new FitAddon();
  term.loadAddon(fitAddon);
  term.open(terminalContainer.value);

  term.onData((data) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(data);
    }
  });

  try {
    fitAddon.fit();
  } catch {
    // Ignore fit on initial race
  }

  if (window.ResizeObserver) {
    resizeObserver = new ResizeObserver(() => {
      if (fitAddon && term) {
        try {
          fitAddon.fit();
          sendResize();
        } catch {
          // Ignore
        }
      }
    });
    resizeObserver.observe(terminalContainer.value);
  }
}

function connect() {
  if (!props.containerId) return;

  if (ws) {
    ws.onclose = null;
    ws.onerror = null;
    ws.onmessage = null;
    ws.close();
    ws = null;
  }

  status.value = 'CONNECTING';

  if (term) {
    term.reset();
    term.writeln(`\x1b[36mConnecting to ${props.containerName || props.containerId} (${selectedShell.value})...\x1b[0m\r\n`);
  }

  const url = getWebSocketUrl(props.containerId, selectedShell.value);
  ws = new WebSocket(url);
  ws.binaryType = 'arraybuffer';

  ws.onopen = () => {
    status.value = 'CONNECTED';
    if (fitAddon && term) {
      try {
        fitAddon.fit();
        sendResize();
      } catch {
        // Ignore
      }
    }
    term?.focus();
  };

  ws.onmessage = (event) => {
    if (!term) return;
    if (typeof event.data === 'string') {
      term.write(event.data);
    } else if (event.data instanceof ArrayBuffer) {
      term.write(new Uint8Array(event.data));
    }
  };

  ws.onerror = () => {
    status.value = 'ERROR';
    term?.writeln('\r\n\x1b[31;1m[Terminal connection error]\x1b[0m\r\n');
  };

  ws.onclose = () => {
    if (status.value !== 'ERROR') {
      status.value = 'DISCONNECTED';
    }
    term?.writeln('\r\n\x1b[33m[Session disconnected. Click Reconnect to try again.]\x1b[0m\r\n');
  };
}

function reconnect() {
  connect();
}

function clearScreen() {
  term?.clear();
  term?.focus();
}

watch(
  () => [props.containerId, selectedShell.value],
  () => {
    connect();
  }
);

onMounted(() => {
  nextTick(() => {
    initTerminal();
    connect();
  });
});

onBeforeUnmount(() => {
  if (resizeObserver) {
    resizeObserver.disconnect();
    resizeObserver = null;
  }
  if (ws) {
    ws.onclose = null;
    ws.onerror = null;
    ws.onmessage = null;
    ws.close();
    ws = null;
  }
  if (term) {
    term.dispose();
    term = null;
  }
});
</script>

<template>
  <div class="embedded-terminal-wrapper">
    <!-- Terminal Header Toolbar -->
    <div class="terminal-toolbar">
      <div class="toolbar-left">
        <MdiIcon :path="mdiConsole" size="16" style="color: var(--accent-primary)" />
        <span class="mono font-xs title-label">{{ containerName || containerId }}</span>
        <span
          class="badge mono font-xs"
          :class="{
            'badge-success': status === 'CONNECTED',
            'badge-warning': status === 'CONNECTING',
            'badge-neutral': status === 'DISCONNECTED',
            'badge-danger': status === 'ERROR',
          }"
        >
          {{ status }}
        </span>
      </div>

      <div class="toolbar-right">
        <!-- Shell Selector -->
        <select v-model="selectedShell" class="form-select font-xs shell-select">
          <option value="/bin/sh">/bin/sh</option>
          <option value="/bin/bash">/bin/bash</option>
          <option value="/bin/ash">/bin/ash</option>
        </select>

        <!-- Reconnect -->
        <button
          class="btn btn-xs btn-secondary"
          title="Reconnect Shell"
          @click="reconnect"
        >
          <MdiIcon :path="mdiRefresh" size="13" />
          <span class="hide-mobile">Reconnect</span>
        </button>

        <!-- Clear -->
        <button
          class="btn btn-xs btn-secondary"
          title="Clear Screen"
          @click="clearScreen"
        >
          <MdiIcon :path="mdiTrashCanOutline" size="13" />
          <span class="hide-mobile">Clear</span>
        </button>

        <!-- Pop-out Window -->
        <button
          class="btn btn-xs btn-primary"
          title="Open in Detached Pop-out Window"
          @click="emit('popout')"
        >
          <MdiIcon :path="mdiOpenInNew" size="13" />
          <span>Pop-out</span>
        </button>
      </div>
    </div>

    <!-- Terminal Canvas -->
    <div ref="terminalContainer" class="terminal-canvas-box"></div>
  </div>
</template>

<style scoped>
.embedded-terminal-wrapper {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 520px;
  background: #0a0f1d;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-subtle);
  overflow: hidden;
}

.terminal-toolbar {
  height: 38px;
  background: var(--bg-surface-elevated, #161f30);
  border-bottom: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  flex-shrink: 0;
  gap: 8px;
}

.toolbar-left,
.toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.title-label {
  font-weight: 600;
  color: var(--text-primary);
}

.shell-select {
  height: 24px;
  padding: 0 8px;
  font-size: 11px;
}

.terminal-canvas-box {
  flex: 1;
  width: 100%;
  min-height: 480px;
  background: #0a0f1d;
  padding: 8px;
  box-sizing: border-box;
  overflow: hidden;
}

.terminal-canvas-box :deep(.xterm) {
  height: 100%;
  padding: 0;
}

.terminal-canvas-box :deep(.xterm-viewport) {
  overflow-y: auto;
}

@media (max-width: 640px) {
  .hide-mobile {
    display: none;
  }
}
</style>
