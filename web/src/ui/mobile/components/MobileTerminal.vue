<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import {
  mdiClose,
  mdiRefresh,
  mdiTrashCanOutline,
  mdiArrowUp,
  mdiArrowDown,
  mdiArrowLeft,
  mdiArrowRight,
  mdiConsole,
} from '@mdi/js';
import MdiIcon from '../../components/MdiIcon.vue';
import { services } from '../../../container';
import { nasState } from '../../store';
import { TERMINAL_FONT_FAMILY, ensureTerminalFontsLoaded } from '../../utils/terminalFont';

const props = withDefaults(
  defineProps<{
    title?: string;
    endpoint?: string;
    lxcId?: string;
    containerId?: string;
  }>(),
  {
    title: '',
    endpoint: '',
    lxcId: '',
    containerId: '',
  }
);

const emit = defineEmits<{
  (e: 'close'): void;
}>();

const status = ref<'CONNECTING' | 'CONNECTED' | 'DISCONNECTED' | 'ERROR'>('CONNECTING');
const terminalContainer = ref<HTMLDivElement | null>(null);
const isCtrlActive = ref(false);

const viewportHeight = ref(
  typeof window !== 'undefined' && window.visualViewport ? `${window.visualViewport.height}px` : '100%'
);

function handleViewportResize() {
  if (typeof window !== 'undefined' && window.visualViewport) {
    viewportHeight.value = `${window.visualViewport.height}px`;
    fitAddon?.fit();
    sendResize();
  }
}

let term: Terminal | null = null;
let fitAddon: FitAddon | null = null;
let ws: WebSocket | null = null;
let resizeObserver: ResizeObserver | null = null;

const hostname = computed(() => nasState.systemTelemetry?.host?.hostname || 'Host');

const displayTitle = computed(() => {
  if (props.title) return props.title;
  if (props.lxcId) return `LXC: ${props.lxcId}`;
  if (props.containerId) return `Container: ${props.containerId}`;
  return 'Host Shell';
});

function getWebSocketUrl(): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  const token = services.auth.getToken() ?? '';
  const tokenParam = token ? `access_token=${encodeURIComponent(token)}` : '';
  const shellParam = 'shell=' + encodeURIComponent('/bin/bash');
  const params = [shellParam];
  if (tokenParam) params.push(tokenParam);
  const q = params.join('&');

  if (props.lxcId) {
    return `${protocol}//${host}/ws/lxc/terminal/${encodeURIComponent(props.lxcId)}?${q}`;
  }
  if (props.containerId) {
    return `${protocol}//${host}/ws/docker/terminal/${encodeURIComponent(props.containerId)}?${q}`;
  }
  if (props.endpoint) {
    return `${protocol}//${host}${props.endpoint}${props.endpoint.includes('?') ? '&' : '?'}${tokenParam}`;
  }
  return `${protocol}//${host}/ws/host/terminal${tokenParam ? '?' + tokenParam : ''}`;
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
    // Ignore resize error
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
    customGlyphs: true,
    fontSize: 12,
    fontFamily: TERMINAL_FONT_FAMILY,
    lineHeight: 1.0,
    theme: {
      background: '#090d16',
      foreground: '#e2e8f0',
      cursor: '#38bdf8',
      cursorAccent: '#090d16',
      selectionBackground: 'rgba(56, 189, 248, 0.3)',
      black: '#1e293b',
      red: '#ef4444',
      green: '#22c55e',
      yellow: '#eab308',
      blue: '#3b82f6',
      magenta: '#a855f7',
      cyan: '#06b6d4',
      white: '#f8fafc',
    },
    allowProposedApi: true,
    scrollback: 1000,
  });

  fitAddon = new FitAddon();
  term.loadAddon(fitAddon);
  term.open(terminalContainer.value);

  nextTick(() => {
    try {
      fitAddon?.fit();
    } catch {}
  });

  ensureTerminalFontsLoaded(() => {
    if (term && fitAddon) {
      try {
        term.options.fontFamily = TERMINAL_FONT_FAMILY;
        fitAddon.fit();
        sendResize();
      } catch {}
    }
  });

  term.onData((data) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      if (isCtrlActive.value && data.length === 1) {
        // Handle sticky CTRL modifier
        isCtrlActive.value = false;
        const code = data.toUpperCase().charCodeAt(0);
        if (code >= 64 && code <= 95) {
          ws.send(String.fromCharCode(code - 64));
          return;
        }
      }
      ws.send(data);
    }
  });

  connectWebSocket();
}

function connectWebSocket() {
  if (ws) {
    ws.close();
    ws = null;
  }

  status.value = 'CONNECTING';

  try {
    const url = getWebSocketUrl();
    ws = new WebSocket(url);
    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
      status.value = 'CONNECTED';
      sendResize();
      term?.focus();
    };

    ws.onmessage = (event) => {
      if (typeof event.data === 'string') {
        try {
          const msg = JSON.parse(event.data);
          if (msg.type === 'stdout' && msg.data) {
            term?.write(msg.data);
            return;
          }
        } catch {
          term?.write(event.data);
        }
      } else if (event.data instanceof ArrayBuffer) {
        term?.write(new Uint8Array(event.data));
      }
    };

    ws.onerror = () => {
      status.value = 'ERROR';
      term?.write('\r\n\x1b[31m[WebSocket Connection Error]\x1b[0m\r\n');
    };

    ws.onclose = () => {
      status.value = 'DISCONNECTED';
      term?.write('\r\n\x1b[33m[Session Disconnected]\x1b[0m\r\n');
    };
  } catch (err) {
    status.value = 'ERROR';
  }
}

function sendKey(seq: string) {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(seq);
    term?.focus();
  }
}

function sendCtrlC() {
  sendKey('\x03');
}

function sendCtrlD() {
  sendKey('\x04');
}

function sendTab() {
  sendKey('\t');
}

function sendEscape() {
  sendKey('\x1b');
}

function sendUp() {
  sendKey('\x1b[A');
}

function sendDown() {
  sendKey('\x1b[B');
}

function clearScreen() {
  term?.clear();
  sendKey('\x0c');
}

onMounted(() => {
  initTerminal();

  if (typeof window !== 'undefined' && window.visualViewport) {
    window.visualViewport.addEventListener('resize', handleViewportResize);
  }

  if (terminalContainer.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => {
      try {
        fitAddon?.fit();
        sendResize();
      } catch {}
    });
    resizeObserver.observe(terminalContainer.value);
  }
});

onBeforeUnmount(() => {
  if (typeof window !== 'undefined' && window.visualViewport) {
    window.visualViewport.removeEventListener('resize', handleViewportResize);
  }
  if (resizeObserver) {
    resizeObserver.disconnect();
    resizeObserver = null;
  }
  if (ws) {
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
  <div class="mobile-terminal-wrapper" :style="{ height: viewportHeight }">
    <!-- Header -->
    <div class="mobile-terminal-header">
      <div class="header-left">
        <MdiIcon :path="mdiConsole" size="16" style="color: var(--accent-primary)" />
        <span class="header-title">{{ displayTitle }}</span>
        <span class="status-indicator" :class="status.toLowerCase()">
          {{ status }}
        </span>
      </div>

      <div class="header-actions">
        <button class="btn btn-xs btn-ghost btn-icon" title="Clear Screen" @click="clearScreen">
          <MdiIcon :path="mdiTrashCanOutline" size="16" />
        </button>
        <button class="btn btn-xs btn-ghost btn-icon" title="Reconnect" @click="connectWebSocket">
          <MdiIcon :path="mdiRefresh" size="16" />
        </button>
        <button class="btn btn-xs btn-ghost btn-icon" title="Close" @click="emit('close')">
          <MdiIcon :path="mdiClose" size="18" />
        </button>
      </div>
    </div>

    <!-- Terminal Canvas -->
    <div ref="terminalContainer" class="mobile-terminal-canvas" />

    <!-- Mobile Virtual Accessory Key Bar -->
    <div class="mobile-accessory-bar">
      <button class="acc-key" @click="sendEscape">ESC</button>
      <button class="acc-key" @click="sendTab">TAB</button>
      <button
        class="acc-key"
        :class="{ active: isCtrlActive }"
        @click="isCtrlActive = !isCtrlActive"
      >
        CTRL
      </button>
      <button class="acc-key key-accent" @click="sendCtrlC">^C</button>
      <button class="acc-key" @click="sendCtrlD">^D</button>
      <button class="acc-key" @click="sendKey('|')">|</button>
      <button class="acc-key" @click="sendKey('/')">/</button>
      <button class="acc-key" @click="sendKey('~')">~</button>
      <button class="acc-key" @click="sendKey('-')">-</button>
      <button class="acc-key key-icon" @click="sendUp">
        <MdiIcon :path="mdiArrowUp" size="14" />
      </button>
      <button class="acc-key key-icon" @click="sendDown">
        <MdiIcon :path="mdiArrowDown" size="14" />
      </button>
      <button class="acc-key key-icon" @click="sendKey('\x1b[D')">
        <MdiIcon :path="mdiArrowLeft" size="14" />
      </button>
      <button class="acc-key key-icon" @click="sendKey('\x1b[C')">
        <MdiIcon :path="mdiArrowRight" size="14" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.mobile-terminal-wrapper {
  position: fixed;
  inset: 0;
  z-index: 1300;
  background: #090d16;
  display: flex;
  flex-direction: column;
  padding-top: env(safe-area-inset-top, 0px);
  padding-bottom: env(safe-area-inset-bottom, 0px);
}

.mobile-terminal-header {
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  background: #111827;
  border-bottom: 1px solid #1f2937;
  flex-shrink: 0;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.header-title {
  font-size: 13px;
  font-weight: 700;
  color: #f8fafc;
}

.status-indicator {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 999px;
  font-weight: 600;
  text-transform: uppercase;
}

.status-indicator.connected {
  background: rgba(34, 197, 94, 0.2);
  color: #4ade80;
}

.status-indicator.connecting {
  background: rgba(234, 179, 8, 0.2);
  color: #facc15;
}

.status-indicator.disconnected,
.status-indicator.error {
  background: rgba(239, 68, 68, 0.2);
  color: #f87171;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}

.mobile-terminal-canvas {
  flex: 1;
  overflow: hidden;
  padding: 4px 6px;
  background: #090d16;
}

.mobile-accessory-bar {
  height: 42px;
  display: flex;
  align-items: center;
  gap: 4px;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
  padding: 4px 8px;
  background: #111827;
  border-top: 1px solid #1f2937;
  flex-shrink: 0;
  scrollbar-width: none;
}

.mobile-accessory-bar::-webkit-scrollbar {
  display: none;
}

.acc-key {
  height: 32px;
  min-width: 36px;
  padding: 0 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 6px;
  color: #f1f5f9;
  font-size: 11px;
  font-weight: 700;
  font-family: ui-monospace, monospace;
  cursor: pointer;
  touch-action: manipulation;
  flex-shrink: 0;
  user-select: none;
  -webkit-user-select: none;
}

.acc-key:active {
  background: #38bdf8;
  color: #0f172a;
}

.acc-key.active {
  background: #38bdf8;
  color: #0f172a;
  border-color: #38bdf8;
}

.acc-key.key-accent {
  color: #ef4444;
}

.acc-key.key-icon {
  padding: 0 6px;
}
</style>
