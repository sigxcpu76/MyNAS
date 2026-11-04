<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { services } from '../../container';

const terminalContainer = ref<HTMLDivElement | null>(null);
const connectionStatus = ref<'CONNECTING' | 'CONNECTED' | 'DISCONNECTED' | 'ERROR'>('DISCONNECTED');
const statusMessage = ref('');

const params = typeof window !== 'undefined' ? new URLSearchParams(window.location.search) : new URLSearchParams();
const lxcId = params.get('lxc') || '';
const containerId = params.get('container') || '';
const initialShell = params.get('shell') || '/bin/bash';
const selectedShell = ref(initialShell);

const terminalType = ref<'lxc' | 'docker' | 'host'>(
  lxcId ? 'lxc' : containerId ? 'docker' : 'host'
);

const targetId = ref(lxcId || containerId || 'host');

const title = ref(
  terminalType.value === 'lxc'
    ? `LXC Terminal — ${lxcId}`
    : terminalType.value === 'docker'
    ? `Docker Terminal — ${containerId}`
    : 'Host Shell (root)'
);

let term: Terminal | null = null;
let fitAddon: FitAddon | null = null;
let ws: WebSocket | null = null;
let resizeObserver: ResizeObserver | null = null;

function getWebSocketUrl(): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  const token = services.auth.getToken() ?? '';
  const tokenParam = token ? `access_token=${encodeURIComponent(token)}` : '';

  if (terminalType.value === 'lxc') {
    const q = [`shell=${encodeURIComponent(selectedShell.value)}`];
    if (tokenParam) q.push(tokenParam);
    return `${protocol}//${host}/ws/lxc/terminal/${encodeURIComponent(lxcId)}?${q.join('&')}`;
  } else if (terminalType.value === 'docker') {
    const q = [`shell=${encodeURIComponent(selectedShell.value)}`];
    if (tokenParam) q.push(tokenParam);
    return `${protocol}//${host}/ws/docker/terminal/${encodeURIComponent(containerId)}?${q.join('&')}`;
  } else {
    return `${protocol}//${host}/ws/host/terminal${tokenParam ? '?' + tokenParam : ''}`;
  }
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
    // Ignore
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
  } catch {}

  if (typeof window !== 'undefined' && window.ResizeObserver) {
    resizeObserver = new ResizeObserver(() => {
      if (fitAddon && term) {
        try {
          fitAddon.fit();
          sendResize();
        } catch {}
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

  connectionStatus.value = 'CONNECTING';

  if (term) {
    term.reset();
    term.writeln(`\x1b[36mConnecting to ${title.value}...\x1b[0m\r\n`);
  }

  const url = getWebSocketUrl();
  ws = new WebSocket(url);
  ws.binaryType = 'arraybuffer';

  ws.onopen = () => {
    connectionStatus.value = 'CONNECTED';
    if (fitAddon && term) {
      try {
        fitAddon.fit();
        sendResize();
      } catch {}
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
    connectionStatus.value = 'ERROR';
    statusMessage.value = 'Connection error occurred.';
    term?.writeln('\r\n\x1b[31m[WebSocket connection error]\x1b[0m');
  };

  ws.onclose = (event) => {
    connectionStatus.value = 'DISCONNECTED';
    statusMessage.value = `Session closed (code: ${event.code}).`;
    term?.writeln(`\r\n\x1b[33m[Session closed with code ${event.code}]\x1b[0m\r\n`);
  };
}

function handleShellChange(e: Event) {
  selectedShell.value = (e.target as HTMLSelectElement).value;
  connect();
}

function clearScreen() {
  term?.clear();
}

function handleClose() {
  if (typeof window !== 'undefined') {
    if (window.opener) {
      window.close();
    } else {
      window.location.href = '/overview';
    }
  }
}

onMounted(async () => {
  await nextTick();
  initTerminal();
  connect();
});

onBeforeUnmount(() => {
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
  <div class="standalone-terminal-container">
    <div class="terminal-standalone-header">
      <div class="terminal-header-title">
        <svg style="width: 18px; height: 18px; fill: currentColor; color: var(--accent-primary)" viewBox="0 0 24 24">
          <path d="M20,19V7H4V19H20M20,3A2,2 0 0,1 22,5V19A2,2 0 0,1 20,21H4A2,2 0 0,1 2,19V5A2,2 0 0,1 4,3H20M13,17V15H18V17H13M9.58,13L5.5,8.91L6.91,7.5L12.41,13L6.91,18.5L5.5,17.09L9.58,13Z" />
        </svg>
        <span class="header-text">{{ title }}</span>
        <span
          class="badge"
          :class="{
            'badge-success': connectionStatus === 'CONNECTED',
            'badge-warning': connectionStatus === 'CONNECTING',
            'badge-danger': connectionStatus === 'ERROR' || connectionStatus === 'DISCONNECTED',
          }"
          style="font-size: 11px; text-transform: uppercase"
        >
          {{ connectionStatus }}
        </span>
      </div>

      <div class="terminal-header-actions">
        <!-- Shell selector for containers/LXC -->
        <select
          v-if="terminalType !== 'host'"
          :value="selectedShell"
          class="select select-xs"
          style="max-width: 130px; font-family: monospace"
          title="Shell environment"
          @change="handleShellChange"
        >
          <option value="/bin/bash">/bin/bash</option>
          <option value="/bin/sh">/bin/sh</option>
          <option value="/bin/ash">/bin/ash</option>
          <option value="/bin/zsh">/bin/zsh</option>
        </select>

        <button class="btn btn-xs btn-secondary" title="Clear terminal screen" @click="clearScreen">
          Clear
        </button>

        <button
          v-if="connectionStatus !== 'CONNECTED' && connectionStatus !== 'CONNECTING'"
          class="btn btn-xs btn-primary"
          title="Reconnect to session"
          @click="connect"
        >
          Reconnect
        </button>

        <button class="btn btn-xs btn-secondary" title="Close window" @click="handleClose">
          Close
        </button>
      </div>
    </div>

    <div class="terminal-standalone-body">
      <div ref="terminalContainer" class="terminal-canvas" />
    </div>
  </div>
</template>

<style scoped>
.standalone-terminal-container {
  width: 100vw;
  height: 100vh;
  box-sizing: border-box;
  background: #0a0f1d;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.terminal-standalone-header {
  height: 44px;
  background: var(--bg-surface, #1e293b);
  border-bottom: 1px solid var(--border-color, #334155);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  user-select: none;
}

.terminal-header-title {
  display: flex;
  align-items: center;
  gap: 10px;
}

.header-text {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--text-primary, #f8fafc);
  white-space: nowrap;
}

.terminal-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.terminal-standalone-body {
  flex: 1;
  position: relative;
  overflow: hidden;
  padding: 8px;
  background: #0a0f1d;
}

.terminal-canvas {
  width: 100%;
  height: 100%;
}

:deep(.xterm) {
  height: 100%;
  padding: 4px;
}

:deep(.xterm-viewport) {
  overflow-y: auto;
}
</style>
