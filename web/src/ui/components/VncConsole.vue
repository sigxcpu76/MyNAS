<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import {
  mdiAspectRatio,
  mdiChevronDown,
  mdiClose,
  mdiFullscreen,
  mdiFullscreenExit,
  mdiKeyboardOutline,
  mdiOpenInNew,
  mdiPause,
  mdiPlay,
  mdiPower,
  mdiPowerCycle,
  mdiRefresh,
  mdiRestart,
  mdiRestartAlert,
  mdiSend,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import {
  RfbConsoleAdapter,
  type ScaleMode,
} from '../../adapters/novnc/rfbConsoleAdapter';
import { services } from '../../container';
import { askModal, notify } from '../store';

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
const vmState = ref<'RUNNING' | 'STOPPED' | 'PAUSED' | 'UNKNOWN'>('RUNNING');

let adapter: RfbConsoleAdapter | null = null;

async function fetchVmState() {
  try {
    const vm = await services.kvm.getVirtualMachine(props.vmName);
    if (vm && vm.state) {
      vmState.value = vm.state as any;
    }
  } catch {
    try {
      const list = await services.kvm.listVirtualMachines();
      const found = list.find((v) => v.name === props.vmName || v.vmName === props.vmName);
      if (found && found.state) {
        vmState.value = found.state as any;
      }
    } catch {}
  }
}

async function handleVmPowerAction(
  action: 'start' | 'stop' | 'reboot' | 'reset' | 'pause' | 'resume',
  event?: MouseEvent
) {
  try {
    if (action === 'start') {
      await services.kvm.startVirtualMachine(props.vmName);
      notify('success', `Started VM ${props.vmName}`);
      vmState.value = 'RUNNING';
      setTimeout(() => handleReconnect(), 1500);
    } else if (action === 'stop') {
      let force = false;
      if (event && (event.shiftKey || event.altKey)) {
        force = true;
      } else {
        const res = await askModal({
          title: `Shutdown VM: ${props.vmName}`,
          message: `Are you sure you want to shut down "${props.vmName}"?`,
          details: 'ACPI shutdown requests the guest operating system to flush filesystem buffers and power off cleanly.',
          confirmText: 'Shutdown',
          tone: 'warning',
          checkbox: {
            label: 'Force shutdown immediately (virsh destroy)',
            hint: 'Halts the VM process immediately. Use if the guest OS is hung, stuck, or unresponsive.',
            checked: false,
          },
        });
        if (!res.confirmed) return;
        force = Boolean(res.checkboxChecked);
      }
      await services.kvm.stopVirtualMachine(props.vmName, force);
      notify('success', `${force ? 'Force stopped' : 'Shut down'} VM ${props.vmName}`);
      vmState.value = 'STOPPED';
    } else if (action === 'reboot') {
      const res = await askModal({
        title: `Reboot VM: ${props.vmName}`,
        message: `Are you sure you want to reboot "${props.vmName}"?`,
        details: 'ACPI reboot sends a restart signal to the guest operating system.',
        confirmText: 'Reboot',
        tone: 'warning',
        icon: mdiRestart,
      });
      if (!res.confirmed) return;
      await services.kvm.rebootVirtualMachine(props.vmName, false);
      notify('success', `Rebooted VM ${props.vmName}`);
      vmState.value = 'RUNNING';
      setTimeout(() => handleReconnect(), 2000);
    } else if (action === 'reset') {
      const res = await askModal({
        title: `Hard Reset VM: ${props.vmName}`,
        message: `Are you sure you want to hard reset "${props.vmName}"?`,
        details: 'Power cycles virtual hardware instantly without waiting for guest OS response. Any unsaved data may be lost.',
        confirmText: 'Hard Reset',
        tone: 'danger',
        icon: mdiPowerCycle,
      });
      if (!res.confirmed) return;
      await services.kvm.resetVirtualMachine(props.vmName);
      notify('success', `Hard reset VM ${props.vmName}`);
      vmState.value = 'RUNNING';
      setTimeout(() => handleReconnect(), 2000);
    } else if (action === 'pause') {
      await services.kvm.pauseVirtualMachine(props.vmName);
      notify('success', `Paused VM ${props.vmName}`);
      vmState.value = 'PAUSED';
    } else if (action === 'resume') {
      await services.kvm.resumeVirtualMachine(props.vmName);
      notify('success', `Resumed VM ${props.vmName}`);
      vmState.value = 'RUNNING';
    }
    await fetchVmState();
  } catch (err: any) {
    notify('error', err?.message || `Failed to execute ${action} on VM`);
  }
}

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
  setTimeout(() => {
    adapter?.triggerResize();
  }, 50);
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
  fetchVmState();
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
        <h3 class="console-title">{{ vmName }}</h3>
        <span
          class="badge badge-sm"
          :class="{
            'badge-success': statusText === 'CONNECTED',
            'badge-info': statusText === 'CONNECTING',
            'badge-danger': statusText === 'ERROR' || statusText === 'DISCONNECTED',
          }"
          :title="statusDetail + (vncPort ? ` (Port: ${vncPort})` : '')"
        >
          {{ statusText }}
        </span>
      </div>

      <div class="header-actions">
        <!-- VM Power Controls -->
        <div class="toolbar-group">
          <!-- Start (if stopped) -->
          <button
            v-if="vmState === 'STOPPED'"
            class="btn btn-sm btn-icon btn-success"
            title="Start VM"
            @click="handleVmPowerAction('start')"
          >
            <MdiIcon :path="mdiPlay" size="16" />
          </button>

          <!-- Resume (if paused) -->
          <button
            v-if="vmState === 'PAUSED'"
            class="btn btn-sm btn-icon btn-success"
            title="Resume VM"
            @click="handleVmPowerAction('resume')"
          >
            <MdiIcon :path="mdiPlay" size="16" />
          </button>

          <!-- Pause (if running) -->
          <button
            v-if="vmState === 'RUNNING'"
            class="btn btn-sm btn-icon btn-secondary"
            title="Pause VM"
            @click="handleVmPowerAction('pause')"
          >
            <MdiIcon :path="mdiPause" size="16" />
          </button>

          <!-- Reboot (if running) -->
          <button
            v-if="vmState === 'RUNNING'"
            class="btn btn-sm btn-icon btn-secondary"
            title="Reboot VM (Clean ACPI Restart)"
            @click="handleVmPowerAction('reboot')"
          >
            <MdiIcon :path="mdiRestart" size="16" />
          </button>

          <!-- Hard Reset (if running) -->
          <button
            v-if="vmState === 'RUNNING'"
            class="btn btn-sm btn-icon btn-danger"
            title="Hard Reset VM (Power Cycle)"
            @click="handleVmPowerAction('reset')"
          >
            <MdiIcon :path="mdiPowerCycle" size="16" />
          </button>

          <!-- Shutdown (if running) -->
          <button
            v-if="vmState === 'RUNNING'"
            class="btn btn-sm btn-icon btn-warning"
            title="Shutdown VM"
            @click="handleVmPowerAction('stop', $event)"
          >
            <MdiIcon :path="mdiPower" size="16" />
          </button>
        </div>

        <div class="toolbar-divider"></div>

        <!-- Scale Mode Selector -->
        <div class="scale-select-wrapper" title="Viewport Scaling Mode">
          <MdiIcon :path="mdiAspectRatio" size="14" class="scale-icon" />
          <select
            v-model="scaleMode"
            class="scale-select mono"
            @change="handleScaleChange"
          >
            <option value="fit">Fit</option>
            <option value="1:1">100%</option>
            <option value="125">125%</option>
            <option value="150">150%</option>
            <option value="200">200%</option>
          </select>
          <MdiIcon :path="mdiChevronDown" size="12" class="scale-chevron" />
        </div>

        <!-- Send Keys Dropdown (Linux TTY & Virtual Terminals) -->
        <div ref="keysMenuRef" class="keys-dropdown-container">
          <button
            class="btn btn-secondary btn-sm btn-icon"
            type="button"
            title="Send Keys (Ctrl+Alt+Del, Alt+F1..F3, Ctrl+Alt+F1..F3)"
            @click.stop="showKeysMenu = !showKeysMenu"
          >
            <MdiIcon :path="mdiKeyboardOutline" size="16" />
          </button>

          <div v-if="showKeysMenu" class="keys-dropdown-menu" @click.stop>
            <div class="keys-menu-group-header">System</div>
            <button type="button" class="keys-menu-item" @click="sendCtrlAltDel(); showKeysMenu = false">
              <kbd class="shortcut-kbd">Ctrl + Alt + Del</kbd>
              <span class="shortcut-desc">Security / Task Manager</span>
            </button>

            <div class="keys-menu-divider"></div>

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

        <!-- Ctrl+Alt+Del Quick Action Button -->
        <button
          class="btn btn-secondary btn-sm btn-icon"
          title="Send Ctrl+Alt+Del"
          @click="sendCtrlAltDel"
        >
          <MdiIcon :path="mdiRestartAlert" size="16" />
        </button>

        <!-- Reconnect Button -->
        <button
          class="btn btn-sm btn-icon"
          :class="{
            'btn-warning': statusText === 'DISCONNECTED' || statusText === 'ERROR',
            'btn-secondary': statusText === 'CONNECTED' || statusText === 'CONNECTING',
          }"
          :disabled="reconnecting || statusText === 'CONNECTING'"
          title="Reconnect to Console"
          @click="handleReconnect"
        >
          <MdiIcon
            :path="mdiRefresh"
            size="16"
            :class="{ 'spin-icon': reconnecting || statusText === 'CONNECTING' }"
          />
        </button>

        <!-- Fullscreen Button -->
        <button
          class="btn btn-secondary btn-sm btn-icon"
          :title="isFullscreen ? 'Exit Fullscreen' : 'Enter Fullscreen'"
          @click="toggleFullscreen"
        >
          <MdiIcon :path="isFullscreen ? mdiFullscreenExit : mdiFullscreen" size="16" />
        </button>

        <!-- Detach Button (only shown when not standalone) -->
        <button
          v-if="!standalone"
          class="btn btn-secondary btn-sm btn-icon"
          title="Detach into Separate Window"
          @click="emit('detach')"
        >
          <MdiIcon :path="mdiOpenInNew" size="16" />
        </button>
      </div>

      <!-- Close / Exit: Always locked to Top Right -->
      <button
        class="btn btn-secondary btn-sm btn-icon vnc-top-close-btn"
        :title="standalone ? 'Close Window' : 'Close Console'"
        aria-label="Close Console"
        @click="emit('close')"
      >
        <MdiIcon :path="mdiClose" size="18" />
      </button>
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
          class="btn btn-secondary btn-sm btn-icon"
          :disabled="statusText !== 'CONNECTED'"
          title="Send Text + Enter"
        >
          <MdiIcon :path="mdiSend" size="14" />
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
  height: 100%;
  flex: 1;
  min-height: 0;
  gap: 10px;
}

.vnc-console-root.is-standalone-root {
  height: 100vh;
  box-sizing: border-box;
  padding: 12px;
  background: var(--bg-primary, #0f172a);
}

.vnc-header {
  position: relative;
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  flex-shrink: 0;
  padding-right: 36px;
  min-height: 28px;
}

.vnc-top-close-btn {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  right: 0;
  width: 28px;
  height: 28px;
  min-width: 28px;
  max-height: 28px;
  padding: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  z-index: 10;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 10px;
}

.console-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.toolbar-group {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.toolbar-divider {
  width: 1px;
  height: 18px;
  background: var(--border-color, #334155);
  margin: 0 4px;
  align-self: center;
}

.header-actions .btn-icon,
.btn-icon {
  width: 28px;
  height: 28px;
  min-width: 28px;
  max-height: 28px;
  padding: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
}

.scale-select-wrapper {
  position: relative;
  display: inline-flex;
  align-items: center;
  background: var(--bg-secondary, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 6px;
  height: 28px;
  box-sizing: border-box;
  padding: 0 4px 0 7px;
  gap: 3px;
  transition: border-color 0.15s ease;
}

.scale-select-wrapper:hover {
  border-color: var(--accent-primary, #38bdf8);
}

.scale-icon {
  color: var(--text-secondary, #94a3b8);
  pointer-events: none;
  flex-shrink: 0;
}

.scale-select {
  appearance: none;
  -webkit-appearance: none;
  -moz-appearance: none;
  background: transparent !important;
  background-color: transparent !important;
  border: none !important;
  box-shadow: none !important;
  outline: none !important;
  color: var(--text-primary, #f8fafc) !important;
  font-size: 12px;
  font-family: inherit;
  line-height: 26px;
  height: 26px;
  padding: 0 16px 0 2px !important;
  margin: 0 !important;
  width: auto !important;
  cursor: pointer;
  box-sizing: border-box;
}

.scale-select option {
  background: var(--bg-surface, #1e293b);
  color: var(--text-primary, #f8fafc);
}

.scale-chevron {
  position: absolute;
  right: 5px;
  pointer-events: none;
  opacity: 0.6;
}

.keys-dropdown-container {
  position: relative;
  display: inline-flex;
  align-items: center;
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
  min-width: 340px;
  width: max-content;
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
  white-space: nowrap;
  gap: 16px;
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
  white-space: nowrap;
}

.shortcut-desc {
  font-size: 11.5px;
  color: var(--text-secondary, #94a3b8);
  white-space: nowrap;
}

.vnc-wrapper {
  background: #000;
  border: 1px solid var(--border-strong, #334155);
  border-radius: 8px;
  overflow: hidden;
  display: flex;
  justify-content: center;
  align-items: center;
  position: relative;
  flex: 1;
  min-height: 280px;
  width: 100%;
  height: 100%;
}

.vnc-wrapper.scale-fit-mode {
  overflow: hidden;
}

.vnc-wrapper.scale-manual-mode {
  overflow: auto;
}

.vnc-wrapper.is-standalone {
  flex: 1;
  min-height: 0;
  max-height: none;
  height: 100%;
}

.vnc-wrapper:fullscreen {
  width: 100vw;
  height: 100vh;
  max-height: 100vh;
  border: none;
  border-radius: 0;
}

.vnc-screen-container {
  width: 100%;
  height: 100%;
  flex: 1;
  min-height: 0;
  display: flex;
  justify-content: center;
  align-items: center;
  outline: none;
  user-select: none;
}

.scale-fit-mode .vnc-screen-container {
  width: 100%;
  height: 100%;
}

.scale-fit-mode .vnc-screen-container :deep(canvas) {
  display: block;
}

.scale-manual-mode .vnc-screen-container {
  width: 100%;
  height: 100%;
  display: flex;
  justify-content: center;
  align-items: center;
}

.vnc-disconnected-overlay {
  position: absolute;
  inset: 0;
  background: rgba(15, 23, 42, 0.85);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 10;
}

.disconnected-card {
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-strong, #334155);
  border-radius: 10px;
  padding: 24px 32px;
  text-align: center;
  max-width: 420px;
  box-shadow: 0 12px 30px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
  align-items: center;
}

.disconnected-title {
  margin: 0 0 6px 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
}

.disconnected-detail {
  margin: 0 0 16px 0;
  font-size: 13px;
  color: var(--text-secondary);
  line-height: 1.4;
}

.vnc-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  background: var(--bg-surface, #1e293b);
  border: 1px solid var(--border-color, #334155);
  border-radius: 8px;
  padding: 8px 12px;
  flex-shrink: 0;
}

.linux-shortcuts-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.shortcuts-tag {
  font-size: 11.5px;
  font-weight: 700;
  color: var(--text-muted, #94a3b8);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.shortcuts-pills {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.btn-shortcut {
  background: var(--bg-primary, #0f172a);
  border: 1px solid var(--border-color, #334155);
  border-radius: 5px;
  color: var(--text-primary, #f8fafc);
  font-family: monospace;
  font-size: 11.5px;
  font-weight: 500;
  padding: 3px 8px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.btn-shortcut:hover {
  background: var(--bg-surface-hover, #334155);
  border-color: var(--accent-primary, #38bdf8);
  color: var(--accent-primary, #38bdf8);
}

.shortcuts-divider {
  width: 1px;
  height: 16px;
  background: var(--border-color, #334155);
  margin: 0 2px;
}

.quick-text-form {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  max-width: 460px;
}

.spin-icon {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
</style>
