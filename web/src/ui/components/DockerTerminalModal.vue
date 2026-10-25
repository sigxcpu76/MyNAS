<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import MdiIcon from './MdiIcon.vue';
import {
  mdiConsole,
  mdiClose,
  mdiFullscreen,
  mdiFullscreenExit,
  mdiRefresh,
  mdiTrashCanOutline,
} from '@mdi/js';

export interface TerminalContainerOption {
  id: string;
  name: string;
  serviceName?: string;
  state?: string;
}

const props = defineProps<{
  appTitle: string;
  containers: TerminalContainerOption[];
  initialContainerId?: string;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
}>();

const selectedContainerId = ref(
  props.initialContainerId || props.containers[0]?.id || props.containers[0]?.name || ''
);
const selectedShell = ref('/bin/sh');
const status = ref<'CONNECTING' | 'CONNECTED' | 'DISCONNECTED' | 'ERROR'>('CONNECTING');
const isFullscreen = ref(false);
const terminalContainer = ref<HTMLDivElement | null>(null);

let term: Terminal | null = null;
let fitAddon: FitAddon | null = null;
let ws: WebSocket | null = null;
let resizeObserver: ResizeObserver | null = null;

const currentContainer = computed(() => {
  return props.containers.find(
    (c) => c.id === selectedContainerId.value || c.name === selectedContainerId.value
  );
});

function getWebSocketUrl(containerId: string, shell: string): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  return `${protocol}//${host}/ws/docker/terminal/${encodeURIComponent(containerId)}?shell=${encodeURIComponent(shell)}`;
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
    // Ignore resize error if ws closing
  }
}

function initTerminal() {
  if (!terminalContainer.value) return;

  // Cleanup existing terminal if any
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

  // Send keystrokes directly to websocket
  term.onData((data) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(data);
    }
  });

  // Fit initially
  try {
    fitAddon.fit();
  } catch {
    // Ignore initial fit race
  }

  // Setup ResizeObserver for responsive resizing
  if (window.ResizeObserver) {
    resizeObserver = new ResizeObserver(() => {
      if (fitAddon && term) {
        try {
          fitAddon.fit();
          sendResize();
        } catch {
          // Ignore fit errors on detached DOM
        }
      }
    });
    resizeObserver.observe(terminalContainer.value);
  }
}

function connect() {
  if (!selectedContainerId.value) return;

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
    term.writeln(`\x1b[36mConnecting to container \x1b[1;37m${currentContainer.value?.name || selectedContainerId.value}\x1b[0;36m with \x1b[1;33m${selectedShell.value}\x1b[0;36m...\x1b[0m\r\n`);
  }

  const url = getWebSocketUrl(selectedContainerId.value, selectedShell.value);
  ws = new WebSocket(url);
  ws.binaryType = 'arraybuffer';

  ws.onopen = () => {
    status.value = 'CONNECTED';
    if (fitAddon && term) {
      try {
        fitAddon.fit();
        sendResize();
      } catch {
        // Ignore fit error
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
    term?.writeln('\r\n\x1b[33m[Session disconnected. Press Reconnect or change shell to retry.]\x1b[0m\r\n');
  };
}

function reconnect() {
  connect();
}

function clearScreen() {
  term?.clear();
  term?.focus();
}

function toggleFullscreen() {
  isFullscreen.value = !isFullscreen.value;
  nextTick(() => {
    if (fitAddon && term) {
      fitAddon.fit();
      sendResize();
      term.focus();
    }
  });
}

watch(selectedContainerId, () => {
  connect();
});

watch(selectedShell, () => {
  connect();
});

onMounted(() => {
  initTerminal();
  connect();
  window.addEventListener('resize', handleWindowResize);
});

function handleWindowResize() {
  if (fitAddon && term) {
    try {
      fitAddon.fit();
      sendResize();
    } catch {
      // Ignore
    }
  }
}

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleWindowResize);
  if (resizeObserver) {
    resizeObserver.disconnect();
    resizeObserver = null;
  }
  if (ws) {
    ws.onclose = null;
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
  <div class="modal-backdrop" @click.self="emit('close')">
    <div
      class="modal-card docker-terminal-card"
      :class="{ 'docker-terminal-fullscreen': isFullscreen }"
    >
      <!-- Terminal Header / Toolbar -->
      <div class="card-header" style="padding: 12px 18px; border-bottom: 1px solid var(--border-subtle); display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 12px">
        <div style="display: flex; align-items: center; gap: 10px; flex-wrap: wrap">
          <div style="display: flex; align-items: center; gap: 8px">
            <span style="color: #38bdf8; display: inline-flex">
              <MdiIcon :path="mdiConsole" size="20" />
            </span>
            <h3 class="card-title" style="margin: 0; font-size: 15px">
              Terminal: {{ currentContainer?.name || currentContainer?.id || appTitle }}
            </h3>
          </div>

          <!-- Connection Status Badge -->
          <span
            class="badge"
            :class="{
              'badge-success': status === 'CONNECTED',
              'badge-info': status === 'CONNECTING',
              'badge-neutral': status === 'DISCONNECTED',
              'badge-danger': status === 'ERROR',
            }"
            style="font-size: 11px; padding: 2px 7px"
          >
            {{ status }}
          </span>

          <!-- App & Container context tag -->
          <span class="mono" style="font-size: 11px; color: var(--text-secondary)">
            {{ appTitle }}
          </span>
        </div>

        <!-- Controls: Container selector, Shell selector, Actions -->
        <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap">
          <!-- Container dropdown if multiple -->
          <div v-if="containers.length > 1" style="display: flex; align-items: center; gap: 4px">
            <label style="font-size: 11.5px; color: var(--text-secondary)">Container:</label>
            <select
              v-model="selectedContainerId"
              class="input mono"
              style="padding: 3px 8px; font-size: 11.5px; height: 28px"
            >
              <option
                v-for="c in containers"
                :key="c.id || c.name"
                :value="c.id || c.name"
              >
                {{ c.name }} ({{ c.serviceName || 'service' }})
              </option>
            </select>
          </div>

          <!-- Shell selector -->
          <div style="display: flex; align-items: center; gap: 4px">
            <label style="font-size: 11.5px; color: var(--text-secondary)">Shell:</label>
            <select
              v-model="selectedShell"
              class="input mono"
              style="padding: 3px 8px; font-size: 11.5px; height: 28px"
            >
              <option value="/bin/sh">/bin/sh (POSIX)</option>
              <option value="/bin/bash">/bin/bash</option>
              <option value="/bin/ash">/bin/ash</option>
              <option value="sh">sh</option>
            </select>
          </div>

          <!-- Clear Button -->
          <button
            class="btn btn-sm"
            style="padding: 3px 8px; height: 28px"
            title="Clear terminal screen"
            aria-label="Clear terminal screen"
            @click="clearScreen"
          >
            <MdiIcon :path="mdiTrashCanOutline" size="14" />
            <span style="font-size: 11.5px; margin-left: 3px">Clear</span>
          </button>

          <!-- Reconnect Button -->
          <button
            class="btn btn-sm"
            style="padding: 3px 8px; height: 28px"
            title="Reconnect session"
            aria-label="Reconnect session"
            @click="reconnect"
          >
            <MdiIcon :path="mdiRefresh" size="14" />
            <span style="font-size: 11.5px; margin-left: 3px">Reconnect</span>
          </button>

          <!-- Fullscreen Toggle -->
          <button
            class="btn btn-sm btn-icon"
            style="height: 28px; width: 28px; padding: 0"
            :title="isFullscreen ? 'Exit Fullscreen' : 'Fullscreen'"
            :aria-label="isFullscreen ? 'Exit Fullscreen' : 'Fullscreen'"
            @click="toggleFullscreen"
          >
            <MdiIcon :path="isFullscreen ? mdiFullscreenExit : mdiFullscreen" size="16" />
          </button>

          <!-- Close Modal -->
          <button
            class="btn btn-sm btn-icon"
            style="height: 28px; width: 28px; padding: 0"
            title="Close Terminal"
            aria-label="Close Terminal"
            @click="emit('close')"
          >
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>
      </div>

      <!-- Terminal Body Canvas / Container -->
      <div class="docker-terminal-body-wrapper">
        <div ref="terminalContainer" class="docker-terminal-canvas" />
      </div>

      <!-- Quick status footer -->
      <div class="docker-terminal-footer">
        <span class="mono" style="font-size: 11px; color: var(--text-secondary)">
          {{ currentContainer?.name || selectedContainerId }} &bull; {{ selectedShell }} &bull; Interactive PTY (VT100/xterm)
        </span>
        <span style="font-size: 11px; color: var(--text-secondary)">
          Press <kbd style="background: rgba(255,255,255,0.08); padding: 1px 4px; border-radius: 3px; font-size: 10px">Ctrl+C</kbd> or type <code class="mono" style="color: #38bdf8">exit</code> to end session
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.docker-terminal-card {
  max-width: 980px;
  width: 95vw;
  display: flex;
  flex-direction: column;
  background: #070b14;
  border: 1px solid var(--border-strong);
  border-radius: 10px;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.6);
  overflow: hidden;
  transition: all 0.2s ease;
}

.docker-terminal-fullscreen {
  max-width: 100vw !important;
  width: 100vw !important;
  height: 100vh !important;
  border-radius: 0 !important;
  border: none !important;
  margin: 0 !important;
  inset: 0 !important;
}

.docker-terminal-body-wrapper {
  background: #0a0f1d;
  padding: 10px 12px;
  flex: 1;
  min-height: 480px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.docker-terminal-fullscreen .docker-terminal-body-wrapper {
  min-height: calc(100vh - 90px);
}

.docker-terminal-canvas {
  flex: 1;
  width: 100%;
  height: 100%;
  min-height: 460px;
}

.docker-terminal-fullscreen .docker-terminal-canvas {
  min-height: calc(100vh - 110px);
}

.docker-terminal-footer {
  padding: 6px 16px;
  background: #070b14;
  border-top: 1px solid var(--border-subtle);
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

/* Ensure xterm viewport fills properly */
:deep(.xterm) {
  height: 100%;
  padding: 4px;
}

:deep(.xterm .xterm-viewport) {
  background-color: #0a0f1d !important;
}
</style>
