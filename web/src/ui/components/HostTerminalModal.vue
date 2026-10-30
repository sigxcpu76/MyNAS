<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import MdiIcon from './MdiIcon.vue';
import { services } from '../../container';
import {
  mdiConsole,
  mdiClose,
  mdiFullscreen,
  mdiFullscreenExit,
  mdiRefresh,
  mdiTrashCanOutline,
} from '@mdi/js';
import { nasState } from '../store';

const emit = defineEmits<{
  (e: 'close'): void;
}>();

const status = ref<'CONNECTING' | 'CONNECTED' | 'DISCONNECTED' | 'ERROR'>('CONNECTING');
const isFullscreen = ref(false);
const terminalContainer = ref<HTMLDivElement | null>(null);

let term: Terminal | null = null;
let fitAddon: FitAddon | null = null;
let ws: WebSocket | null = null;
let resizeObserver: ResizeObserver | null = null;

const currentUser = computed(() => nasState.currentUser);
const hostname = computed(() => nasState.systemTelemetry?.host?.hostname || 'NAS Host');

function getWebSocketUrl(): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  const token = services.auth.getToken() ?? '';
  return `${protocol}//${host}/ws/host/terminal?access_token=${encodeURIComponent(token)}`;
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
  if (typeof window !== 'undefined' && window.ResizeObserver) {
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
    term.writeln(`\x1b[36mConnecting to NAS host shell as \x1b[1;37m${currentUser.value?.username || 'user'}\x1b[0;36m...\x1b[0m\r\n`);
  }

  const url = getWebSocketUrl();
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
    term?.writeln('\r\n\x1b[33m[Host session disconnected. Press Reconnect to start a new session.]\x1b[0m\r\n');
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

onMounted(() => {
  initTerminal();
  connect();
  window.addEventListener('resize', handleWindowResize);
});

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleWindowResize);
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
  fitAddon = null;
});
</script>

<template>
  <div class="modal-backdrop" @click.self="emit('close')">
    <div
      class="modal-card host-terminal-card"
      :class="{ 'host-terminal-fullscreen': isFullscreen }"
    >
      <!-- Header -->
      <div class="card-header host-terminal-header">
        <div style="display: flex; align-items: center; gap: 12px; min-width: 0; flex-wrap: wrap;">
          <div style="display: flex; align-items: center; gap: 8px;">
            <MdiIcon :path="mdiConsole" size="20" style="color: var(--accent-primary)" />
            <h3 class="modal-title" style="margin: 0; font-size: 15px; color: #f8fafc; font-weight: 600;">
              Host Terminal
            </h3>
          </div>

          <div style="display: flex; align-items: center; gap: 6px;">
            <span class="badge badge-neutral mono" style="font-size: 11px;">
              {{ hostname }}
            </span>
            <span class="badge badge-info mono" style="font-size: 11px;">
              {{ currentUser?.username || 'root' }}
            </span>
          </div>

          <!-- Connection Status Pill -->
          <span
            v-if="status === 'CONNECTED'"
            class="badge badge-success"
            style="font-size: 10.5px; padding: 2px 7px;"
          >
            CONNECTED
          </span>
          <span
            v-else-if="status === 'CONNECTING'"
            class="badge badge-neutral"
            style="font-size: 10.5px; padding: 2px 7px; display: inline-flex; align-items: center; gap: 4px;"
          >
            <span class="spinner" style="width: 8px; height: 8px; border-width: 1.5px"></span>
            CONNECTING
          </span>
          <span
            v-else-if="status === 'DISCONNECTED'"
            class="badge badge-warning"
            style="font-size: 10.5px; padding: 2px 7px;"
          >
            DISCONNECTED
          </span>
          <span
            v-else
            class="badge badge-danger"
            style="font-size: 10.5px; padding: 2px 7px;"
          >
            ERROR
          </span>
        </div>

        <!-- Terminal Actions -->
        <div style="display: flex; align-items: center; gap: 6px;">
          <button
            class="btn btn-sm btn-icon"
            style="color: var(--text-secondary); background: transparent; border: 1px solid var(--border-subtle); height: 28px; width: 28px; padding: 0"
            title="Clear terminal screen"
            aria-label="Clear terminal screen"
            @click="clearScreen"
          >
            <MdiIcon :path="mdiTrashCanOutline" size="15" />
          </button>

          <button
            class="btn btn-sm btn-icon"
            style="color: var(--text-secondary); background: transparent; border: 1px solid var(--border-subtle); height: 28px; width: 28px; padding: 0"
            title="Reconnect terminal session"
            aria-label="Reconnect terminal session"
            @click="reconnect"
          >
            <MdiIcon :path="mdiRefresh" size="15" />
          </button>

          <button
            class="btn btn-sm btn-icon"
            style="color: var(--text-secondary); background: transparent; border: 1px solid var(--border-subtle); height: 28px; width: 28px; padding: 0"
            :title="isFullscreen ? 'Exit Fullscreen' : 'Fullscreen'"
            :aria-label="isFullscreen ? 'Exit Fullscreen' : 'Fullscreen'"
            @click="toggleFullscreen"
          >
            <MdiIcon :path="isFullscreen ? mdiFullscreenExit : mdiFullscreen" size="15" />
          </button>
        </div>

        <!-- Close Button: ALWAYS in top right corner -->
        <button
          class="btn btn-sm btn-icon terminal-top-close-btn"
          title="Close Terminal"
          aria-label="Close terminal"
          @click="emit('close')"
        >
          <MdiIcon :path="mdiClose" size="18" />
        </button>
      </div>

      <!-- Terminal Body Canvas -->
      <div class="host-terminal-body-wrapper">
        <div ref="terminalContainer" class="host-terminal-canvas" />
      </div>

      <!-- Footer Info -->
      <div class="host-terminal-footer">
        <span class="mono" style="font-size: 11px; color: var(--text-secondary);">
          Host Shell: Logged in as <strong style="color: #f8fafc">{{ currentUser?.username }}</strong> (shell from <code>/etc/passwd</code>)
        </span>
        <span class="mono hide-mobile" style="font-size: 11px; color: var(--text-tertiary, #64748b);">
          Tip: Ctrl+Shift+C / Ctrl+Shift+V to copy &amp; paste
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.host-terminal-card {
  position: relative;
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

.host-terminal-header {
  position: relative;
  padding: 10px 48px 10px 16px;
  border-bottom: 1px solid var(--border-subtle);
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  background: #070b14;
  min-height: 48px;
}

.terminal-top-close-btn {
  position: absolute;
  top: 10px;
  right: 12px;
  height: 28px;
  width: 28px;
  padding: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
  z-index: 10;
}

.terminal-top-close-btn:hover {
  background: var(--bg-surface, #1e293b);
  color: #fff;
  border-color: var(--border-strong, #475569);
}

.host-terminal-fullscreen {
  max-width: 100vw !important;
  width: 100vw !important;
  height: 100vh !important;
  border-radius: 0 !important;
  border: none !important;
  margin: 0 !important;
  inset: 0 !important;
}

.host-terminal-body-wrapper {
  background: #0a0f1d;
  padding: 10px 12px;
  flex: 1;
  min-height: 480px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.host-terminal-fullscreen .host-terminal-body-wrapper {
  min-height: calc(100vh - 90px);
}

.host-terminal-canvas {
  flex: 1;
  width: 100%;
  height: 100%;
  min-height: 460px;
}

.host-terminal-fullscreen .host-terminal-canvas {
  min-height: calc(100vh - 110px);
}

.host-terminal-footer {
  padding: 6px 16px;
  background: #070b14;
  border-top: 1px solid var(--border-subtle);
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

:deep(.xterm) {
  height: 100%;
  padding: 4px;
}

:deep(.xterm .xterm-viewport) {
  background-color: #0a0f1d !important;
}
</style>
