<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import {
  mdiChevronDown,
  mdiFullscreen,
  mdiFullscreenExit,
  mdiKeyboardOutline,
  mdiOpenInNew,
  mdiRefresh,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import {
  RfbConsoleAdapter,
  type ScaleMode,
} from '../../adapters/novnc/rfbConsoleAdapter';
import { services } from '../../container';

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
const keysMenuRef = ref<HTMLDivElement | null>(null);
const statusText = ref<'CONNECTING' | 'CONNECTED' | 'DISCONNECTED' | 'ERROR'>('CONNECTING');
const statusDetail = ref('Connecting to live QEMU VNC server...');
const quickText = ref('');
const isFullscreen = ref(false);
const scaleMode = ref<ScaleMode>('fit');
const showKeysMenu = ref(false);
const reconnecting = ref(false);

let adapter: RfbConsoleAdapter | null = null;

function sendCtrlAltDel() {
  adapter?.sendCtrlAltDel();
  adapter?.focus();
}

function sendShortcut(
  shortcut: 'alt-f1' | 'alt-f2' | 'alt-f3' | 'ctrl-alt-f1' | 'ctrl-alt-f2' | 'ctrl-alt-f3'
) {
  adapter?.sendLinuxShortcut(shortcut);
  showKeysMenu.value = false;
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

function handleDocumentClick(e: MouseEvent) {
  if (keysMenuRef.value && !keysMenuRef.value.contains(e.target as Node)) {
    showKeysMenu.value = false;
  }
}

async function handleReconnect() {
  if (reconnecting.value) return;
  reconnecting.value = true;
  statusText.value = 'CONNECTING';
  statusDetail.value = 'Reconnecting to live QEMU VNC server...';

  try {
    // Request a fresh console session token for this VM
    const freshSession = await services.kvm.openConsole(props.vmName);
    if (adapter) {
      adapter.updateCredentials(freshSession.token, freshSession.websocketUrl);
      adapter.connect();
    } else if (screenRef.value) {
      initAdapter(freshSession.token, freshSession.websocketUrl);
    }
  } catch (err: any) {
    // If requesting fresh token fails, attempt reconnect with existing credentials
    if (adapter) {
      adapter.connect();
    } else {
      statusText.value = 'ERROR';
      statusDetail.value = err?.message || 'Failed to reconnect';
    }
  } finally {
    reconnecting.value = false;
  }
}

function initAdapter(token: string, wsUrl?: string) {
  if (!screenRef.value) return;
  adapter = new RfbConsoleAdapter({
    target: screenRef.value,
    vmName: props.vmName,
    token,
    websocketUrl: wsUrl || props.websocketUrl,
    scaleMode: scaleMode.value,
    crisp: false,
    onStatusChange: (status, detail) => {
      statusText.value = status;
      if (detail) statusDetail.value = detail;
    },
  });
  adapter.connect();
}

onMounted(() => {
  document.addEventListener('fullscreenchange', handleFullscreenChange);
  document.addEventListener('click', handleDocumentClick);
  initAdapter(props.token, props.websocketUrl);
});

onBeforeUnmount(() => {
  document.removeEventListener('fullscreenchange', handleFullscreenChange);
  document.removeEventListener('click', handleDocumentClick);
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

        <!-- Send Keys Dropdown (Linux TTY & Virtual Terminals) -->
        <div ref="keysMenuRef" class="keys-dropdown-container">
          <button
            class="btn btn-secondary btn-sm"
            type="button"
            title="Send Linux virtual console keys (Alt+F1..F3, Ctrl+Alt+F1..F3)"
            @click.stop="showKeysMenu = !showKeysMenu"
          >
            <MdiIcon :path="mdiKeyboardOutline" size="14" style="margin-right: 4px" />
            <span>Send Keys</span>
            <MdiIcon :path="mdiChevronDown" size="13" style="margin-left: 2px; opacity: 0.8" />
          </button>

          <div v-if="showKeysMenu" class="keys-dropdown-menu" @click.stop>
            <div class="keys-menu-group-header">Linux Console / Installer (Alt+Fn)</div>
            <button type="button" class="keys-menu-item" @click="sendShortcut('alt-f1')">
              <kbd class="shortcut-kbd">Alt + F1</kbd>
              <span class="shortcut-desc">TTY 1 (Main / Installer)</span>
            </button>
            <button type="button" class="keys-menu-item" @click="sendShortcut('alt-f2')">
              <kbd class="shortcut-kbd">Alt + F2</kbd>
              <span class="shortcut-desc">TTY 2 (Shell Console)</span>
            </button>
            <button type="button" class="keys-menu-item" @click="sendShortcut('alt-f3')">
              <kbd class="shortcut-kbd">Alt + F3</kbd>
              <span class="shortcut-desc">TTY 3 (Debug Log)</span>
            </button>

            <div class="keys-menu-divider"></div>

            <div class="keys-menu-group-header">Desktop / X11 (Ctrl+Alt+Fn)</div>
            <button type="button" class="keys-menu-item" @click="sendShortcut('ctrl-alt-f1')">
              <kbd class="shortcut-kbd">Ctrl + Alt + F1</kbd>
              <span class="shortcut-desc">VT 1 (GDM / Console)</span>
            </button>
            <button type="button" class="keys-menu-item" @click="sendShortcut('ctrl-alt-f2')">
              <kbd class="shortcut-kbd">Ctrl + Alt + F2</kbd>
              <span class="shortcut-desc">VT 2 (Desktop / GUI)</span>
            </button>
            <button type="button" class="keys-menu-item" @click="sendShortcut('ctrl-alt-f3')">
              <kbd class="shortcut-kbd">Ctrl + Alt + F3</kbd>
              <span class="shortcut-desc">VT 3</span>
            </button>
          </div>
        </div>

        <!-- Reconnect Button -->
        <button
          class="btn btn-sm"
          :class="{
            'btn-warning': statusText === 'DISCONNECTED' || statusText === 'ERROR',
            'btn-secondary': statusText === 'CONNECTED' || statusText === 'CONNECTING',
          }"
          :disabled="reconnecting || statusText === 'CONNECTING'"
          title="Reconnect to live QEMU VNC server"
          @click="handleReconnect"
        >
          <MdiIcon
            :path="mdiRefresh"
            size="14"
            :class="{ 'spin-icon': reconnecting || statusText === 'CONNECTING' }"
            style="margin-right: 4px"
          />
          <span>{{ reconnecting ? 'Reconnecting...' : 'Reconnect' }}</span>
        </button>

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

      <!-- Disconnected / Error Overlay with Direct Reconnect Prompt -->
      <div
        v-if="statusText === 'DISCONNECTED' || statusText === 'ERROR'"
        class="vnc-disconnected-overlay"
      >
        <div class="disconnected-card">
          <MdiIcon :path="mdiRefresh" size="30" style="color: var(--warning-text, #f59e0b); margin-bottom: 8px" />
          <h4 class="disconnected-title">VNC Connection Closed</h4>
          <p class="disconnected-detail">{{ statusDetail }}</p>
          <button
            class="btn btn-primary btn-sm"
            :disabled="reconnecting"
            @click="handleReconnect"
          >
            <MdiIcon :path="mdiRefresh" size="14" :class="{ 'spin-icon': reconnecting }" style="margin-right: 5px" />
            {{ reconnecting ? 'Reconnecting...' : 'Reconnect Now' }}
          </button>
        </div>
      </div>
    </div>

    <!-- Footer: Linux TTY Shortcut Pills and Quick Text Bar -->
    <div class="vnc-footer">
      <div class="linux-shortcuts-bar">
        <span class="shortcuts-tag">Linux TTY:</span>
        <div class="shortcuts-pills">
          <button
            type="button"
            class="btn-shortcut"
            title="Send Alt+F1 (TTY1 - Main / Installer)"
            @click="sendShortcut('alt-f1')"
          >
            Alt+F1
          </button>
          <button
            type="button"
            class="btn-shortcut"
            title="Send Alt+F2 (TTY2 - Shell Prompt)"
            @click="sendShortcut('alt-f2')"
          >
            Alt+F2
          </button>
          <button
            type="button"
            class="btn-shortcut"
            title="Send Alt+F3 (TTY3 - Log)"
            @click="sendShortcut('alt-f3')"
          >
            Alt+F3
          </button>
          <span class="shortcuts-divider"></span>
          <button
            type="button"
            class="btn-shortcut"
            title="Send Ctrl+Alt+F1 (VT1 - Console)"
            @click="sendShortcut('ctrl-alt-f1')"
          >
            Ctrl+Alt+F1
          </button>
          <button
            type="button"
            class="btn-shortcut"
            title="Send Ctrl+Alt+F2 (VT2 - Desktop / GUI)"
            @click="sendShortcut('ctrl-alt-f2')"
          >
            Ctrl+Alt+F2
          </button>
          <button
            type="button"
            class="btn-shortcut"
            title="Send Ctrl+Alt+F3 (VT3)"
            @click="sendShortcut('ctrl-alt-f3')"
          >
            Ctrl+Alt+F3
          </button>
        </div>
      </div>

      <form
        class="quick-text-form"
        @submit.prevent="sendQuickText"
      >
        <input
          v-model="quickText"
          type="text"
          class="input mono input-sm"
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

.keys-dropdown-container {
  position: relative;
  display: inline-block;
}

.keys-dropdown-menu {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-strong, #334155);
  border-radius: 8px;
  box-shadow: 0 10px 25px rgba(0, 0, 0, 0.45);
  z-index: 1000;
  min-width: 270px;
  padding: 6px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.keys-menu-group-header {
  font-size: 10.5px;
  font-weight: 700;
  color: var(--text-muted, #94a3b8);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  padding: 6px 10px 3px 10px;
}

.keys-menu-divider {
  height: 1px;
  background: var(--border-subtle, #334155);
  margin: 4px 0;
}

.keys-menu-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  background: transparent;
  border: none;
  border-radius: 6px;
  padding: 6px 10px;
  color: var(--text-primary, #f8fafc);
  cursor: pointer;
  text-align: left;
  transition: background 0.15s ease;
}

.keys-menu-item:hover {
  background: var(--bg-surface-hover, #334155);
}

.shortcut-kbd {
  font-family: monospace;
  font-size: 11.5px;
  font-weight: 600;
  background: var(--bg-primary, #0f172a);
  border: 1px solid var(--border-color, #475569);
  border-radius: 4px;
  padding: 2px 6px;
  color: var(--accent-primary, #38bdf8);
}

.shortcut-desc {
  font-size: 11.5px;
  color: var(--text-secondary, #94a3b8);
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
  height: calc(100vh - 150px);
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

.vnc-disconnected-overlay {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.75);
  backdrop-filter: blur(3px);
  display: flex;
  justify-content: center;
  align-items: center;
  z-index: 20;
}

.disconnected-card {
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-strong, #334155);
  border-radius: 10px;
  padding: 24px 32px;
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  max-width: 380px;
  box-shadow: 0 12px 30px rgba(0, 0, 0, 0.5);
}

.disconnected-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary, #f8fafc);
}

.disconnected-detail {
  margin: 8px 0 16px 0;
  font-size: 12.5px;
  color: var(--text-secondary, #94a3b8);
  font-family: monospace;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

.spin-icon {
  animation: spin 1s linear infinite;
}

.vnc-footer {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.linux-shortcuts-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.shortcuts-tag {
  font-size: 11px;
  font-weight: 700;
  color: var(--text-muted, #94a3b8);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.shortcuts-pills {
  display: flex;
  align-items: center;
  gap: 5px;
  flex-wrap: wrap;
}

.btn-shortcut {
  background: var(--bg-secondary, #1e293b);
  border: 1px solid var(--border-color, #334155);
  color: var(--text-secondary, #cbd5e1);
  font-family: monospace;
  font-size: 11px;
  font-weight: 600;
  padding: 3px 8px;
  border-radius: 5px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.btn-shortcut:hover {
  background: var(--bg-surface-hover, #334155);
  border-color: var(--accent-primary, #38bdf8);
  color: var(--accent-primary, #38bdf8);
}

.shortcuts-divider {
  display: inline-block;
  width: 1px;
  height: 14px;
  background: var(--border-subtle, #334155);
  margin: 0 4px;
}

.quick-text-form {
  display: flex;
  gap: 8px;
  align-items: center;
}
</style>
